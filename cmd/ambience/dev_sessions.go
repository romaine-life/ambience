package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type devSnapshotData struct {
	Type          string          `json:"type"`
	Tick          int             `json:"tick"`
	Config        json.RawMessage `json:"config"`
	State         json.RawMessage `json:"state"`
	Seed          int64           `json:"seed"`
	GridW         int             `json:"gridW"`
	GridH         int             `json:"gridH"`
	AppliedEvents []appliedEvent  `json:"appliedEvents"`
}

// appliedEvent records a lifecycle/trigger event actually applied to this dev
// session's sim (drained from the effect log). Surfaced in the dev snapshot so
// an external observer (the glimmung verifier) can mechanically confirm a fired
// trigger reached the session it is screenshotting — rather than inferring it
// from a single frame, which a pristine, never-triggered sim can coincidentally
// match (e.g. an "intro" resting look equals an untouched sim's resting look).
type appliedEvent struct {
	Tick  int    `json:"tick"`
	Event string `json:"event"`
}

// devAppliedEventsCap bounds the per-session applied-event ring.
const devAppliedEventsCap = 32

type devSession struct {
	mu sync.Mutex

	seed       int64
	effect     effectRuntime
	commandSeq int64
	listeners  map[chan Command]struct{}
	lastSeen   time.Time
	cancel     context.CancelFunc
	applied    []appliedEvent
}

// recordApplied appends an applied event to the bounded ring.
func (s *devSession) recordApplied(tick int, event string) {
	s.mu.Lock()
	s.applied = append(s.applied, appliedEvent{Tick: tick, Event: event})
	if len(s.applied) > devAppliedEventsCap {
		s.applied = s.applied[len(s.applied)-devAppliedEventsCap:]
	}
	s.mu.Unlock()
}

// appliedEventsCopy returns a snapshot copy of the applied-event ring.
func (s *devSession) appliedEventsCopy() []appliedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.applied) == 0 {
		return nil
	}
	out := make([]appliedEvent, len(s.applied))
	copy(out, s.applied)
	return out
}

var devSessions sync.Map // key "<effect>\n<session>" => *devSession

func normalizeDevEffect(effect string) string {
	effect = strings.TrimSpace(strings.ToLower(effect))
	if effect == "" {
		return "rain"
	}
	return effect
}

func devPageEffectCandidateFromPath(path string) (string, bool) {
	switch path {
	case "/dev", "/dev/":
		return "rain", true
	}
	if !strings.HasPrefix(path, "/dev/") {
		return "", false
	}
	rest := strings.TrimPrefix(path, "/dev/")
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

func effectFromSchemaPath(path string) (string, bool) {
	if !strings.HasPrefix(path, "/effects/") || !strings.HasSuffix(path, "/schema") {
		return "", false
	}
	rest := strings.TrimPrefix(path, "/effects/")
	rest = strings.TrimSuffix(rest, "/schema")
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	if _, ok := schemaForEffect(rest); !ok {
		return "", false
	}
	return rest, true
}

func effectFromPagePath(path string) (string, bool) {
	if !strings.HasPrefix(path, "/effects/") {
		return "", false
	}
	rest := strings.TrimPrefix(path, "/effects/")
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	if _, ok := schemaForEffect(rest); !ok {
		return "", false
	}
	return rest, true
}

func devPageEffectFromPath(path string) (string, bool) {
	effect, ok := devPageEffectCandidateFromPath(path)
	if !ok {
		return "", false
	}
	if _, ok := schemaForEffect(effect); !ok {
		return "", false
	}
	return effect, true
}

func serveEffectsRoute(w http.ResponseWriter, req *http.Request) {
	if _, ok := effectFromSchemaPath(req.URL.Path); ok {
		serveEffectSchema(w, req)
		return
	}
	serveEffectPage(w, req)
}

func serveEffectSchema(w http.ResponseWriter, req *http.Request) {
	effect, ok := effectFromSchemaPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	schema, ok := schemaForEffect(effect)
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(schema)
}

func serveEffectPage(w http.ResponseWriter, req *http.Request) {
	effect, ok := effectFromPagePath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	meta := socialPageMeta{
		Title:       effectSocialTitle(effect),
		Description: effectSocialDescription(effect),
		URL:         absoluteRequestURL(req, "/effects/"+effect, ""),
		Image:       absoluteRequestURL(req, "/og-image.png", "effect="+url.QueryEscape(effect)+"&page=effect"),
	}
	body := renderEffectPage(effect, meta)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func renderEffectPage(effect string, meta socialPageMeta) string {
	title := html.EscapeString(meta.Title)
	description := html.EscapeString(meta.Description)
	effectPath := "/dev/" + url.PathEscape(effect)
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"UTF-8\">\n" +
		"<title>" + title + "</title>\n" +
		"<meta name=\"description\" content=\"" + description + "\">\n" +
		renderSocialMeta(meta) + "\n" +
		"<meta http-equiv=\"refresh\" content=\"0; url=" + html.EscapeString(effectPath) + "\">\n" +
		"</head>\n<body>\n" +
		"<p><a href=\"" + html.EscapeString(effectPath) + "\">Open " + html.EscapeString(effect) + " controls</a></p>\n" +
		"</body>\n</html>\n"
}

func newDevSession(effectType string) (*devSession, error) {
	effectType = normalizeDevEffect(effectType)
	seed := freshSeed()
	effect, err := newEffectRuntime(effectType, gridW, gridH, seed, nil)
	if err != nil {
		return nil, err
	}
	cfg, err := randomizedDevConfig(effect.Schema(), seed)
	if err != nil {
		return nil, err
	}
	if err := effect.ApplyConfig(cfg); err != nil {
		return nil, err
	}
	return &devSession{
		seed:      seed,
		effect:    effect,
		listeners: make(map[chan Command]struct{}),
		lastSeen:  time.Now(),
	}, nil
}

func configsEqualJSON(left, right json.RawMessage) bool {
	var a any
	if err := json.Unmarshal(left, &a); err != nil {
		return false
	}
	var b any
	if err := json.Unmarshal(right, &b); err != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}

func devSessionKey(effectType, sessionID string) string {
	return normalizeDevEffect(effectType) + "\n" + sessionID
}

func getOrCreateDevSession(effectType, sessionID string) (*devSession, error) {
	key := devSessionKey(effectType, sessionID)
	if v, ok := devSessions.Load(key); ok {
		s := v.(*devSession)
		s.mu.Lock()
		s.lastSeen = time.Now()
		s.mu.Unlock()
		return s, nil
	}
	s, err := newDevSession(effectType)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	actual, loaded := devSessions.LoadOrStore(key, s)
	if loaded {
		cancel()
		existing := actual.(*devSession)
		existing.mu.Lock()
		existing.lastSeen = time.Now()
		existing.mu.Unlock()
		return existing, nil
	}
	go s.run(ctx)
	return s, nil
}

func sweepDevSessions() {
	t := time.NewTicker(devSessionSweep)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		devSessions.Range(func(k, v any) bool {
			s := v.(*devSession)
			s.mu.Lock()
			empty := len(s.listeners) == 0
			idle := now.Sub(s.lastSeen) > devSessionIdle
			s.mu.Unlock()
			if empty && idle {
				if s.cancel != nil {
					s.cancel()
				}
				devSessions.Delete(k)
			}
			return true
		})
	}
}

func (s *devSession) run(ctx context.Context) {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.stepAndBroadcast()
		}
	}
}

// stepAndBroadcast advances the sim one tick, records every drained event into
// the applied-event ring, and broadcasts it to listeners. Split out so the
// drain/record path is unit-testable without driving the ticker goroutine.
func (s *devSession) stepAndBroadcast() {
	s.effect.Step()
	for _, entry := range s.effect.DrainLog() {
		s.recordApplied(entry.Tick, entry.Type)
		s.broadcast(Command{
			Kind:  "trigger",
			Tick:  entry.Tick,
			Event: entry.Type,
			Desc:  entry.Desc,
		})
	}
}

func (s *devSession) addListener() chan Command {
	ch := make(chan Command, 32)
	s.mu.Lock()
	s.listeners[ch] = struct{}{}
	s.lastSeen = time.Now()
	s.mu.Unlock()
	return ch
}

func (s *devSession) removeListener(ch chan Command) {
	s.mu.Lock()
	delete(s.listeners, ch)
	s.lastSeen = time.Now()
	s.mu.Unlock()
	close(ch)
}

func (s *devSession) broadcast(cmd Command) {
	s.mu.Lock()
	if cmd.ID == "" {
		s.commandSeq++
		cmd.ID = strconv.FormatInt(s.commandSeq, 10)
	}
	defer s.mu.Unlock()
	for ch := range s.listeners {
		select {
		case ch <- cmd:
		default:
		}
	}
}

func (s *devSession) currentCommandID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strconv.FormatInt(s.commandSeq, 10)
}

func (s *devSession) snapshot() devSnapshotData {
	s.mu.Lock()
	seed := s.seed
	s.mu.Unlock()

	effectSnap, err := s.effect.Snapshot()
	if err != nil {
		return devSnapshotData{
			Type: s.effect.Type(),
			Seed: seed,
		}
	}
	return devSnapshotData{
		Type:          s.effect.Type(),
		Tick:          effectSnap.Tick,
		Config:        cloneRaw(effectSnap.Config),
		State:         cloneRaw(effectSnap.State),
		Seed:          seed,
		GridW:         effectSnap.GridW,
		GridH:         effectSnap.GridH,
		AppliedEvents: s.appliedEventsCopy(),
	}
}

func (s *devSession) setConfigQuery(values url.Values) error {
	data, err := parseEffectConfig(values, s.effect.Schema())
	if err != nil {
		return err
	}
	return s.applyConfig(data)
}

func (s *devSession) applyConfig(data json.RawMessage) error {
	if err := s.effect.ApplyConfig(data); err != nil {
		return err
	}
	s.broadcast(Command{
		Kind: "config",
		Tick: s.effect.CurrentTick(),
		Data: cloneRaw(data),
	})
	return nil
}

func (s *devSession) randomizeConfig(seed int64) (json.RawMessage, error) {
	current, err := s.effect.Snapshot()
	if err != nil {
		return nil, err
	}
	for attempt := range 6 {
		cfg, err := randomizedDevConfig(s.effect.Schema(), seed+int64(attempt)*7919)
		if err != nil {
			return nil, err
		}
		if attempt == 5 || !configsEqualJSON(cfg, current.Config) {
			if err := s.applyConfig(cfg); err != nil {
				return nil, err
			}
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("randomize config: exhausted attempts")
}

func (s *devSession) triggerEvent(event string) bool {
	return s.effect.Trigger(event)
}

func writeDevSnapshotFrame(w http.ResponseWriter, flusher http.Flusher, s *devSession) error {
	data, _ := json.Marshal(s.snapshot())
	return writeCommand(w, flusher, Command{
		ID:   s.currentCommandID(),
		Kind: "snapshot",
		Tick: s.effect.CurrentTick(),
		Data: data,
	})
}

func streamDevSession(w http.ResponseWriter, req *http.Request, s *devSession) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	ch := s.addListener()
	defer s.removeListener(ch)
	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	if err := writeDevSnapshotFrame(w, flusher, s); err != nil {
		return
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

func devSessionFromRequest(req *http.Request) (*devSession, string, string, error) {
	sessionID := strings.TrimSpace(req.URL.Query().Get("session"))
	if sessionID == "" {
		return nil, "", "", fmt.Errorf("session param required")
	}
	effectType := normalizeDevEffect(req.URL.Query().Get("effect"))
	s, err := getOrCreateDevSession(effectType, sessionID)
	if err != nil {
		return nil, "", "", err
	}
	return s, effectType, sessionID, nil
}

func serveDevSessionSnapshot(w http.ResponseWriter, req *http.Request) {
	s, _, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.snapshot())
}

func serveDevSessionEvents(w http.ResponseWriter, req *http.Request) {
	s, _, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	streamDevSession(w, req, s)
}

func serveDevSessionConfig(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	s, _, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.setConfigQuery(req.URL.Query()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func serveDevSessionRandomize(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	s, _, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.randomizeConfig(freshSeed())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(cfg)
}

func serveDevSessionTrigger(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(req.URL.Path, "/dev/trigger/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "usage: /dev/trigger/<session>/<event>?effect=<name>", http.StatusBadRequest)
		return
	}
	sessionID, event := parts[0], parts[1]
	effectType := normalizeDevEffect(req.URL.Query().Get("effect"))
	s, err := getOrCreateDevSession(effectType, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.triggerEvent(event) {
		http.Error(w, "unknown event: "+event, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
