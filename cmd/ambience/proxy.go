package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	edgeEntropyBufferLimit  = 256 * 1024
	edgeEntropyFlushEvery   = 2 * time.Second
	edgeForwardTimeout      = 1 * time.Second
	edgeSnapshotPollEvery   = 5 * time.Second
	edgeSnapshotTimeout     = 3 * time.Second
	edgeReconnectDelay      = 1 * time.Second
	edgeSchemaLookupTimeout = 1 * time.Second
	edgeMaxSSEFrame         = 4 * 1024 * 1024
	edgeReplayBufferSize    = 512
	edgeReadyFreshness      = 20 * time.Second
)

type authorityProxy struct {
	proxy   *httputil.ReverseProxy
	entropy *entropyForwarder
	mirror  *authorityMirror
	client  *http.Client
	baseURL *url.URL
}

func newAuthorityProxy(ctx context.Context, rawURL string) (*authorityProxy, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse AMBIENCE_AUTHORITY_URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("AMBIENCE_AUTHORITY_URL must include scheme and host")
	}

	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Context().Err() != nil {
			return
		}
		log.Printf("authority proxy %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "authority unavailable", http.StatusBadGateway)
	}

	return &authorityProxy{
		proxy:   proxy,
		entropy: newEntropyForwarder(ctx, baseURL),
		mirror:  newAuthorityMirror(ctx, baseURL),
		client:  &http.Client{},
		baseURL: baseURL,
	}, nil
}

func registerEdgeRoutes(mux *http.ServeMux, proxy *authorityProxy) {
	mux.HandleFunc("/og-image.png", proxy.serveHTTP)
	mux.HandleFunc("/effects/", proxy.serveHTTP)
	mux.HandleFunc("/snapshot", cors(proxy.serveSnapshot))
	mux.HandleFunc("/events", cors(proxy.serveEvents))
	mux.HandleFunc("/entropy", cors(proxy.serveEntropy))
	mux.HandleFunc("/control-auth", proxy.serveHTTP)
	mux.HandleFunc("/config", proxy.serveHTTP)
	mux.HandleFunc("/trigger/", proxy.serveHTTP)
	mux.HandleFunc("/dev/snapshot", proxy.serveHTTP)
	mux.HandleFunc("/dev/events", proxy.serveHTTP)
	mux.HandleFunc("/dev/config", proxy.serveHTTP)
	mux.HandleFunc("/dev/randomize", proxy.serveHTTP)
	mux.HandleFunc("/dev/observe", proxy.serveHTTP)
	mux.HandleFunc("/dev/frame", proxy.serveHTTP)
	mux.HandleFunc("/dev/trigger/", proxy.serveHTTP)
}

func (p *authorityProxy) ready() bool {
	return p.mirror.ready()
}

func (p *authorityProxy) socialImageVersion() string {
	if p == nil || p.mirror == nil {
		return ""
	}
	snap, _, ok := p.mirror.snapshot()
	if !ok {
		return ""
	}
	return socialImageVersion(snap)
}

func (p *authorityProxy) effectSchemaExists(effect string) (bool, error) {
	if p == nil || p.client == nil || p.baseURL == nil {
		return false, fmt.Errorf("authority proxy is not configured for schema lookup")
	}
	effect = strings.TrimSpace(effect)
	if effect == "" {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), edgeSchemaLookupTimeout)
	defer cancel()

	schemaURL := p.baseURL.ResolveReference(&url.URL{
		Path: "/effects/" + url.PathEscape(effect) + "/schema",
	}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, schemaURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("authority schema lookup returned %s", resp.Status)
	}
}

func (p *authorityProxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

func (p *authorityProxy) serveSnapshot(w http.ResponseWriter, _ *http.Request) {
	snap, _, ok := p.mirror.snapshot()
	if !ok {
		http.Error(w, "authority snapshot unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

func (p *authorityProxy) serveEvents(w http.ResponseWriter, r *http.Request) {
	p.mirror.stream(w, r)
}

func (p *authorityProxy) serveEntropy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	const maxBytes = 4096
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read entropy body", http.StatusBadRequest)
		return
	}
	if len(payload) > 0 {
		if err := p.entropy.forward(payload); err != nil {
			p.entropy.enqueue(payload)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type authorityMirror struct {
	ctx         context.Context
	client      *http.Client
	eventsURL   string
	snapshotURL string

	mu          sync.Mutex
	snap        snapshotData
	snapshotID  string
	hasSnapshot bool
	lastStream  time.Time
	replay      []Command
	listeners   map[chan Command]struct{}
}

func newAuthorityMirror(ctx context.Context, baseURL *url.URL) *authorityMirror {
	m := &authorityMirror{
		ctx:         ctx,
		client:      &http.Client{},
		eventsURL:   baseURL.ResolveReference(&url.URL{Path: "/events"}).String(),
		snapshotURL: baseURL.ResolveReference(&url.URL{Path: "/snapshot"}).String(),
		listeners:   make(map[chan Command]struct{}),
	}
	go func() {
		snap, err := m.fetchSnapshot()
		if err == nil {
			m.setSnapshot(snap, "")
		}
	}()
	go m.runEventsLoop()
	go m.runSnapshotPollLoop()
	return m
}

func (m *authorityMirror) ready() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasSnapshot {
		return false
	}
	if m.lastStream.IsZero() {
		return false
	}
	return time.Since(m.lastStream) <= edgeReadyFreshness
}

func (m *authorityMirror) snapshot() (snapshotData, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasSnapshot {
		return snapshotData{}, "", false
	}
	return cloneSnapshot(m.snap), m.snapshotID, true
}

func (m *authorityMirror) setSnapshot(snap snapshotData, snapshotID string) {
	m.mu.Lock()
	m.snap = cloneSnapshot(snap)
	if snapshotID != "" {
		m.snapshotID = snapshotID
		m.replay = nil
	}
	m.hasSnapshot = true
	m.mu.Unlock()
}

func (m *authorityMirror) noteStreamContact() {
	m.mu.Lock()
	m.lastStream = time.Now()
	m.mu.Unlock()
}

func (m *authorityMirror) addListener() chan Command {
	ch := make(chan Command, 64)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners[ch] = struct{}{}
	return ch
}

func (m *authorityMirror) addListenerLocked() chan Command {
	ch := make(chan Command, 64)
	m.listeners[ch] = struct{}{}
	return ch
}

func (m *authorityMirror) stream(w http.ResponseWriter, req *http.Request) {
	lastID := req.Header.Get("Last-Event-ID")
	snap, snapshotID, replay, replayable, ch, ok := m.beginStream(lastID)
	if !ok {
		http.Error(w, "authority snapshot unavailable", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	defer m.removeListener(ch)

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	if replayable {
		for _, cmd := range replay {
			if err := writeCommand(w, flusher, cmd); err != nil {
				return
			}
		}
		if len(replay) == 0 {
			if err := writeSSEComment(w, flusher, "replay-current"); err != nil {
				return
			}
		}
	} else {
		if err := writeSnapshotDataFrame(w, flusher, snap, snapshotID); err != nil {
			return
		}
	}

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if err := writeSSEComment(w, flusher, "keep-alive"); err != nil {
				return
			}
		case cmd, ok := <-ch:
			if !ok {
				return
			}
			if err := writeCommand(w, flusher, cmd); err != nil {
				return
			}
		}
	}
}

func (m *authorityMirror) runSnapshotPollLoop() {
	t := time.NewTicker(edgeSnapshotPollEvery)
	defer t.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
			snap, err := m.fetchSnapshot()
			if err != nil {
				continue
			}
			m.setSnapshot(snap, "")
		}
	}
}

func (m *authorityMirror) beginStream(lastID string) (snapshotData, string, []Command, bool, chan Command, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.hasSnapshot {
		return snapshotData{}, "", nil, false, nil, false
	}

	ch := m.addListenerLocked()
	snap := cloneSnapshot(m.snap)
	snapshotID := m.snapshotID
	replay, replayable := m.replayAfterLocked(lastID)
	return snap, snapshotID, replay, replayable, ch, true
}

func (m *authorityMirror) removeListener(ch chan Command) {
	if ch == nil {
		return
	}
	m.mu.Lock()
	delete(m.listeners, ch)
	m.mu.Unlock()
	close(ch)
}

func (m *authorityMirror) replayAfterLocked(lastID string) ([]Command, bool) {
	if lastID == "" {
		return nil, false
	}
	if lastID == m.snapshotID {
		return append([]Command(nil), m.replay...), true
	}
	for i := len(m.replay) - 1; i >= 0; i-- {
		if m.replay[i].ID == lastID {
			replay := append([]Command(nil), m.replay[i+1:]...)
			return replay, true
		}
	}
	return nil, false
}

func (m *authorityMirror) appendReplayLocked(cmd Command) {
	if cmd.ID == "" {
		return
	}
	m.replay = append(m.replay, cmd)
	if len(m.replay) > edgeReplayBufferSize {
		m.replay = append([]Command(nil), m.replay[len(m.replay)-edgeReplayBufferSize:]...)
	}
}

func (m *authorityMirror) broadcastLocked(cmd Command) {
	for ch := range m.listeners {
		select {
		case ch <- cmd:
		default:
		}
	}
}

func (m *authorityMirror) fetchSnapshot() (snapshotData, error) {
	ctx, cancel := context.WithTimeout(m.ctx, edgeSnapshotTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.snapshotURL, nil)
	if err != nil {
		return snapshotData{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return snapshotData{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return snapshotData{}, fmt.Errorf("authority /snapshot returned %s", resp.Status)
	}
	var snap snapshotData
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return snapshotData{}, err
	}
	return snap, nil
}

func (m *authorityMirror) runEventsLoop() {
	for {
		if m.ctx.Err() != nil {
			return
		}
		if err := m.consumeEvents(); err != nil && m.ctx.Err() == nil {
			log.Printf("edge authority stream: %v", err)
		}
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(edgeReconnectDelay):
		}
	}
}

func (m *authorityMirror) consumeEvents() error {
	req, err := http.NewRequestWithContext(m.ctx, http.MethodGet, m.eventsURL, nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("authority /events returned %s", resp.Status)
	}
	m.noteStreamContact()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), edgeMaxSSEFrame)

	var (
		currentID string
		dataLines []string
	)
	flush := func() error {
		if len(dataLines) == 0 {
			currentID = ""
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		id := currentID
		currentID = ""
		return m.handleEventPayload(id, []byte(payload))
	}

	for scanner.Scan() {
		m.noteStreamContact()
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				log.Printf("edge authority event decode: %v", err)
			}
		case strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "id:"):
			currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			part := strings.TrimPrefix(line, "data:")
			part = strings.TrimPrefix(part, " ")
			dataLines = append(dataLines, part)
		}
	}
	if err := flush(); err != nil {
		log.Printf("edge authority event decode: %v", err)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (m *authorityMirror) handleEventPayload(id string, payload []byte) error {
	var cmd Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	cmd.ID = id
	if cmd.Kind == "snapshot" {
		var snap snapshotData
		if err := json.Unmarshal(cmd.Data, &snap); err != nil {
			return err
		}
		m.setSnapshot(snap, cmd.ID)
		return nil
	}
	m.applyCommand(cmd)
	return nil
}

func (m *authorityMirror) applyCommand(cmd Command) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasSnapshot {
		return
	}
	if cmd.Tick > m.snap.Tick {
		m.snap.Tick = cmd.Tick
	}
	switch cmd.Kind {
	case "config":
		m.snap.Config = cloneRaw(cmd.Data)
		m.snap.CurrentScene.Config = cloneRaw(cmd.Data)
	case "scene":
		var data struct {
			Name          string               `json:"name"`
			Config        json.RawMessage      `json:"config"`
			DurationTicks int                  `json:"durationTicks"`
			StartedAtTick int                  `json:"startedAtTick"`
			NextName      string               `json:"nextName"`
			NextConfig    json.RawMessage      `json:"nextConfig"`
			ScenePolicy   *scenePolicyData     `json:"scenePolicy"`
			Transition    *transitionStateData `json:"transition"`
		}
		if err := json.Unmarshal(cmd.Data, &data); err == nil {
			m.snap.CurrentScene.Name = data.Name
			if len(data.Config) > 0 {
				m.snap.CurrentScene.Config = cloneRaw(data.Config)
			}
			m.snap.CurrentScene.DurationTicks = data.DurationTicks
			m.snap.CurrentScene.StartedAtTick = data.StartedAtTick
			m.snap.NextScene.Name = data.NextName
			if len(data.NextConfig) > 0 {
				m.snap.NextScene.Config = cloneRaw(data.NextConfig)
			}
			m.snap.SceneRemaining = data.DurationTicks
			if data.ScenePolicy != nil {
				m.snap.ScenePolicy = *data.ScenePolicy
			}
			if data.Transition != nil {
				m.snap.Transition = *data.Transition
			}
		}
	case "metric":
		var data struct {
			EntropyBytes   int64                `json:"entropyBytes"`
			SceneRemaining int                  `json:"sceneRemaining"`
			CurrentName    string               `json:"currentName"`
			NextName       string               `json:"nextName"`
			ScenePolicy    *scenePolicyData     `json:"scenePolicy"`
			Transition     *transitionStateData `json:"transition"`
		}
		if err := json.Unmarshal(cmd.Data, &data); err == nil {
			m.snap.EntropyBytes = data.EntropyBytes
			if data.SceneRemaining > 0 {
				m.snap.SceneRemaining = data.SceneRemaining
			}
			if data.CurrentName != "" {
				m.snap.CurrentScene.Name = data.CurrentName
			}
			if data.NextName != "" {
				m.snap.NextScene.Name = data.NextName
			}
			if data.ScenePolicy != nil {
				m.snap.ScenePolicy = *data.ScenePolicy
			}
			if data.Transition != nil {
				m.snap.Transition = *data.Transition
			}
		}
	}
	m.appendReplayLocked(cmd)
	m.broadcastLocked(cmd)
}

func cloneSnapshot(snap snapshotData) snapshotData {
	cloned := snap
	cloned.Config = cloneRaw(snap.Config)
	cloned.State = cloneRaw(snap.State)
	return cloned
}

type entropyForwarder struct {
	ctx        context.Context
	client     *http.Client
	entropyURL string

	mu      sync.Mutex
	pending [][]byte
	bytes   int
}

func newEntropyForwarder(ctx context.Context, baseURL *url.URL) *entropyForwarder {
	entropyURL := baseURL.ResolveReference(&url.URL{Path: "/entropy"}).String()
	f := &entropyForwarder{
		ctx:        ctx,
		client:     &http.Client{},
		entropyURL: entropyURL,
	}
	go f.run()
	return f
}

func (f *entropyForwarder) run() {
	t := time.NewTicker(edgeEntropyFlushEvery)
	defer t.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-t.C:
			f.flush()
		}
	}
}

func (f *entropyForwarder) forward(payload []byte) error {
	ctx, cancel := context.WithTimeout(f.ctx, edgeForwardTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.entropyURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("authority /entropy returned %s", resp.Status)
	}
	return nil
}

func (f *entropyForwarder) enqueue(payload []byte) {
	if len(payload) == 0 {
		return
	}
	chunk := append([]byte(nil), payload...)
	if len(chunk) > edgeEntropyBufferLimit {
		chunk = chunk[len(chunk)-edgeEntropyBufferLimit:]
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for len(f.pending) > 0 && f.bytes+len(chunk) > edgeEntropyBufferLimit {
		dropped := f.pending[0]
		f.pending = f.pending[1:]
		f.bytes -= len(dropped)
	}
	if f.bytes+len(chunk) > edgeEntropyBufferLimit {
		log.Printf("edge entropy buffer full, dropping %d bytes", len(chunk))
		return
	}
	f.pending = append(f.pending, chunk)
	f.bytes += len(chunk)
}

func (f *entropyForwarder) flush() {
	for {
		payload := f.peek()
		if len(payload) == 0 {
			return
		}
		if err := f.forward(payload); err != nil {
			return
		}
		f.pop()
	}
}

func (f *entropyForwarder) peek() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return nil
	}
	return append([]byte(nil), f.pending[0]...)
}

func (f *entropyForwarder) pop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return
	}
	f.bytes -= len(f.pending[0])
	f.pending = f.pending[1:]
}
