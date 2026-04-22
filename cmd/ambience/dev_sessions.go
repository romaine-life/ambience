package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type devSnapshotData struct {
	Type   string          `json:"type"`
	Tick   int             `json:"tick"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	Seed   int64           `json:"seed"`
	GridW  int             `json:"gridW"`
	GridH  int             `json:"gridH"`
}

type devSession struct {
	mu sync.Mutex

	seed          int64
	effect        effectRuntime
	effectVersion int64
	commandSeq    int64
	listeners     map[chan Command]struct{}
	lastSeen      time.Time
	cancel        context.CancelFunc
}

var devSessions sync.Map // key "<session>" => *devSession

func normalizeDevEffect(effect string) string {
	effect = strings.TrimSpace(strings.ToLower(effect))
	if effect == "" {
		return "rain"
	}
	return effect
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

func devPageEffectFromPath(path string) (string, bool) {
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
	if _, ok := schemaForEffect(rest); !ok {
		return "", false
	}
	return rest, true
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

func newDevSession(effectType string) (*devSession, error) {
	effectType = normalizeDevEffect(effectType)
	seed := deriveEffectSeed(time.Now().UnixNano(), 0, effectType)
	cfg, err := randomizedEffectConfig(effectType, seed)
	if err != nil {
		return nil, err
	}
	effect, err := newEffectRuntime(effectType, gridW, gridH, seed, cfg)
	if err != nil {
		return nil, err
	}
	return &devSession{
		seed:          seed,
		effect:        effect,
		effectVersion: 1,
		listeners:     make(map[chan Command]struct{}),
		lastSeen:      time.Now(),
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
	_ = effectType
	return strings.TrimSpace(sessionID)
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
			effect, version := s.currentEffectRuntime()
			effect.Step()
			if !s.effectVersionIs(version) {
				continue
			}
			for _, entry := range effect.DrainLog() {
				if !s.effectVersionIs(version) {
					break
				}
				s.broadcast(Command{
					Kind:  "trigger",
					Tick:  entry.Tick,
					Event: entry.Type,
					Desc:  entry.Desc,
				})
			}
		}
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

func (s *devSession) currentEffectRuntime() (effectRuntime, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effect, s.effectVersion
}

func (s *devSession) effectVersionIs(version int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effectVersion == version
}

func (s *devSession) snapshot() devSnapshotData {
	s.mu.Lock()
	seed := s.seed
	effect := s.effect
	s.mu.Unlock()

	effectSnap, err := effect.Snapshot()
	if err != nil {
		return devSnapshotData{
			Type: effect.Type(),
			Seed: seed,
		}
	}
	return devSnapshotData{
		Type:   effect.Type(),
		Tick:   effectSnap.Tick,
		Config: cloneRaw(effectSnap.Config),
		State:  cloneRaw(effectSnap.State),
		Seed:   seed,
		GridW:  effectSnap.GridW,
		GridH:  effectSnap.GridH,
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
	effect, _ := s.currentEffectRuntime()
	if err := effect.ApplyConfig(data); err != nil {
		return err
	}
	s.broadcast(Command{
		Kind: "config",
		Tick: effect.CurrentTick(),
		Data: cloneRaw(data),
	})
	return nil
}

func (s *devSession) randomizeConfig(seed int64) (json.RawMessage, error) {
	effect, _ := s.currentEffectRuntime()
	current, err := effect.Snapshot()
	if err != nil {
		return nil, err
	}
	for attempt := range 6 {
		cfg, err := randomizedDevConfig(effect.Schema(), seed+int64(attempt)*7919)
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
	effect, _ := s.currentEffectRuntime()
	return effect.Trigger(event)
}

func writeDevSnapshotFrame(w http.ResponseWriter, flusher http.Flusher, s *devSession) error {
	data, _ := json.Marshal(s.snapshot())
	effect, _ := s.currentEffectRuntime()
	return writeCommand(w, flusher, Command{
		ID:   s.currentCommandID(),
		Kind: "snapshot",
		Tick: effect.CurrentTick(),
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
	cfg, err := s.randomizeConfig(time.Now().UnixNano())
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

func serveDevSessionEffect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rawEffect := strings.TrimSpace(req.URL.Query().Get("effect"))
	if rawEffect == "" {
		http.Error(w, "effect param required", http.StatusBadRequest)
		return
	}
	s, effectType, _, err := devSessionFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.switchEffect(effectType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
