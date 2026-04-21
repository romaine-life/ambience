package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nelsong6/ambience/sim"
)

type devEffectRunner interface {
	Type() string
	Step()
	CurrentTick() int
	Snapshot(seed int64) any
	ConfigData() any
	SetConfigQuery(url.Values)
	TriggerEvent(string) bool
	DrainLog() []sim.LogEntry
}

type devSession struct {
	mu sync.Mutex

	effect     string
	seed       int64
	runner     devEffectRunner
	commandSeq int64
	listeners  map[chan Command]struct{}
	lastSeen   time.Time
	cancel     context.CancelFunc
}

type rainDevRunner struct {
	sim *sim.Rain
}

type firefliesDevRunner struct {
	sim *sim.Fireflies
}

var devSessions sync.Map // key "<effect>\n<session>" => *devSession

func newDevSession(effect string) (*devSession, error) {
	effect = normalizeDevEffect(effect)
	seed := time.Now().UnixNano()
	runner, err := newDevEffectRunner(effect, seed)
	if err != nil {
		return nil, err
	}
	return &devSession{
		effect:    effect,
		seed:      seed,
		runner:    runner,
		listeners: make(map[chan Command]struct{}),
		lastSeen:  time.Now(),
	}, nil
}

func normalizeDevEffect(effect string) string {
	effect = strings.TrimSpace(strings.ToLower(effect))
	if effect == "" {
		return "rain"
	}
	return effect
}

func newDevEffectRunner(effect string, seed int64) (devEffectRunner, error) {
	switch normalizeDevEffect(effect) {
	case "rain":
		return &rainDevRunner{sim: sim.NewRain(gridW, gridH, seed, sim.Config{})}, nil
	case "fireflies":
		return &firefliesDevRunner{sim: sim.NewFireflies(gridW, gridH, seed, sim.FirefliesConfig{})}, nil
	default:
		return nil, fmt.Errorf("unknown effect %q", effect)
	}
}

func schemaForEffect(effect string) (sim.EffectSchema, bool) {
	switch normalizeDevEffect(effect) {
	case "rain":
		return sim.RainSchema(), true
	case "fireflies":
		return sim.FirefliesSchema(), true
	default:
		return sim.EffectSchema{}, false
	}
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

func devSessionKey(effect, sessionID string) string {
	return normalizeDevEffect(effect) + "\n" + sessionID
}

func getOrCreateDevSession(effect, sessionID string) (*devSession, error) {
	key := devSessionKey(effect, sessionID)
	if v, ok := devSessions.Load(key); ok {
		s := v.(*devSession)
		s.mu.Lock()
		s.lastSeen = time.Now()
		s.mu.Unlock()
		return s, nil
	}
	s, err := newDevSession(effect)
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
			s.runner.Step()
			for _, entry := range s.runner.DrainLog() {
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

func (s *devSession) snapshot() any {
	return s.runner.Snapshot(s.seed)
}

func (s *devSession) setConfigQuery(values url.Values) {
	s.runner.SetConfigQuery(values)
	data, _ := json.Marshal(s.runner.ConfigData())
	s.broadcast(Command{
		Kind: "config",
		Tick: s.runner.CurrentTick(),
		Data: data,
	})
}

func (s *devSession) triggerEvent(event string) bool {
	return s.runner.TriggerEvent(event)
}

func writeDevSnapshotFrame(w http.ResponseWriter, flusher http.Flusher, s *devSession) error {
	data, _ := json.Marshal(s.snapshot())
	return writeCommand(w, flusher, Command{
		ID:   s.currentCommandID(),
		Kind: "snapshot",
		Tick: s.runner.CurrentTick(),
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
	effect := normalizeDevEffect(req.URL.Query().Get("effect"))
	s, err := getOrCreateDevSession(effect, sessionID)
	if err != nil {
		return nil, "", "", err
	}
	return s, effect, sessionID, nil
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
	s.setConfigQuery(req.URL.Query())
	w.WriteHeader(http.StatusNoContent)
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
	effect := normalizeDevEffect(req.URL.Query().Get("effect"))
	key := devSessionKey(effect, sessionID)
	v, ok := devSessions.Load(key)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	s := v.(*devSession)
	if !s.triggerEvent(event) {
		http.Error(w, "unknown event: "+event, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *rainDevRunner) Type() string { return "rain" }

func (r *rainDevRunner) Step() { r.sim.Step() }

func (r *rainDevRunner) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rainDevRunner) Snapshot(seed int64) any {
	state := r.sim.SnapshotState()
	return map[string]any{
		"type":         r.Type(),
		"tick":         state.Tick,
		"config":       r.sim.EffectiveConfig(),
		"seed":         seed,
		"downpourLeft": state.DownpourTicks,
		"downpourMult": state.DownpourMult,
		"calmLeft":     state.CalmTicks,
		"gustLeft":     state.GustTicks,
		"gustWind":     state.GustWind,
		"gridW":        r.sim.W,
		"gridH":        r.sim.H,
		"drops":        r.sim.DropsCopy(),
		"splashes":     r.sim.SplashesCopy(),
	}
}

func (r *rainDevRunner) ConfigData() any { return r.sim.EffectiveConfig() }

func (r *rainDevRunner) SetConfigQuery(values url.Values) {
	r.sim.SetConfig(parseRainConfig(values))
}

func (r *rainDevRunner) TriggerEvent(event string) bool { return r.sim.TriggerEvent(event) }

func (r *rainDevRunner) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (f *firefliesDevRunner) Type() string { return "fireflies" }

func (f *firefliesDevRunner) Step() { f.sim.Step() }

func (f *firefliesDevRunner) CurrentTick() int { return f.sim.CurrentTick() }

func (f *firefliesDevRunner) Snapshot(seed int64) any {
	state := f.sim.SnapshotState()
	return map[string]any{
		"type":             f.Type(),
		"tick":             state.Tick,
		"config":           f.sim.EffectiveConfig(),
		"seed":             seed,
		"blinkBurstLeft":   state.BlinkBurstTicks,
		"calmLeft":         state.CalmTicks,
		"clusterShiftLeft": state.ClusterShiftTicks,
		"clusterRow":       state.ClusterRow,
		"clusterCol":       state.ClusterCol,
		"gridW":            f.sim.W,
		"gridH":            f.sim.H,
		"fireflies":        f.sim.FirefliesCopy(),
	}
}

func (f *firefliesDevRunner) ConfigData() any { return f.sim.EffectiveConfig() }

func (f *firefliesDevRunner) SetConfigQuery(values url.Values) {
	f.sim.SetConfig(parseFirefliesConfig(values))
}

func (f *firefliesDevRunner) TriggerEvent(event string) bool { return f.sim.TriggerEvent(event) }

func (f *firefliesDevRunner) DrainLog() []sim.LogEntry { return f.sim.DrainLog() }

func parseRainConfig(q url.Values) sim.Config {
	cfg := sim.Config{}
	getFloat(q, "wind", &cfg.Wind)
	getFloat(q, "wind_jit", &cfg.WindJitter)
	getFloat(q, "speed", &cfg.Speed)
	getFloat(q, "speed_jit", &cfg.SpeedJitter)
	getInt(q, "streak", &cfg.StreakLen)
	getFloat(q, "fade", &cfg.FadeFactor)
	getInt(q, "spawn", &cfg.SpawnEvery)
	getInt(q, "burst", &cfg.SpawnBurst)
	getFloat(q, "hue", &cfg.Hue)
	getFloat(q, "hue_sp", &cfg.HueSpread)
	getFloat(q, "sat", &cfg.Saturation)
	getFloat(q, "lmin", &cfg.LightnessMin)
	getFloat(q, "lmax", &cfg.LightnessMax)
	getInt(q, "layers", &cfg.Layers)
	getFloat(q, "lbal", &cfg.LayerBalance)
	getFloat(q, "hue_drift", &cfg.HueDriftAmp)
	getFloat(q, "wind_drift", &cfg.WindDriftAmp)
	getFloat(q, "downpour_p", &cfg.DownpourChance)
	getFloat(q, "calm_p", &cfg.CalmChance)
	getFloat(q, "gust_p", &cfg.GustChance)
	getFloat(q, "splash_p", &cfg.SplashChance)
	getInt(q, "downpour_dur", &cfg.DownpourDur)
	getFloat(q, "downpour_mult", &cfg.DownpourMult)
	getInt(q, "calm_dur", &cfg.CalmDur)
	getInt(q, "gust_dur", &cfg.GustDur)
	getFloat(q, "gust_str", &cfg.GustStrength)
	getInt(q, "splash_size", &cfg.SplashSize)
	return cfg
}

func parseFirefliesConfig(q url.Values) sim.FirefliesConfig {
	cfg := sim.FirefliesConfig{}
	getFloat(q, "drift", &cfg.Drift)
	getFloat(q, "wander", &cfg.Wander)
	getInt(q, "spawn", &cfg.SpawnEvery)
	getInt(q, "max", &cfg.MaxFireflies)
	getFloat(q, "hue", &cfg.Hue)
	getFloat(q, "hue_sp", &cfg.HueSpread)
	getFloat(q, "sat", &cfg.Saturation)
	getFloat(q, "lmin", &cfg.LightnessMin)
	getFloat(q, "lmax", &cfg.LightnessMax)
	getInt(q, "layers", &cfg.Layers)
	getFloat(q, "lbal", &cfg.LayerBalance)
	getFloat(q, "blink_burst_p", &cfg.BlinkBurstChance)
	getFloat(q, "cluster_shift_p", &cfg.ClusterShiftChance)
	getFloat(q, "calm_p", &cfg.CalmChance)
	getInt(q, "blink_burst_dur", &cfg.BlinkBurstDur)
	getFloat(q, "blink_burst_mult", &cfg.BlinkBurstMult)
	getInt(q, "cluster_shift_dur", &cfg.ClusterShiftDur)
	getFloat(q, "cluster_pull", &cfg.ClusterPull)
	getInt(q, "calm_dur", &cfg.CalmDur)
	return cfg
}
