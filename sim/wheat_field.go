package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type WheatFieldConfig = ProceduralConfig
type WheatFieldState = ProceduralState
type WheatFieldSnapshot = ProceduralSnapshot
type WheatFieldPersistedState = ProceduralPersistedState

// WheatField is a dedicated sim type for the wheat-field effect, replacing
// the kind="wheat-field" branch of the old Procedural type.
type WheatField struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    WheatFieldConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var wheatFieldDefaultsLocal = WheatFieldConfig{
	"intro_dur":     60,
	"intro_breeze":  0.16,
	"ending_dur":    70,
	"ending_linger": 20,
	"ending_sway":   0.08,
	"density":       0.48,
	"speed":         0.12,
	"drift":         0.16,
	"sway":          0.68,
	"wave_freq":     0.18,
	"field_top":     0.62,
	"stalk_h":       18,
	"layers":        3,
	"hue":           46,
	"hue_sp":        18,
	"sat":           0.64,
	"lmin":          0.30,
	"lmax":          0.76,
	"gust_p":        0.0,
	"calm_p":        0.0,
	"gust_dur":      50,
	"gust_mult":     1.85,
	"calm_dur":      72,
	"calm_mult":     0.40,
}

func WheatFieldSchema() EffectSchema {
	return EffectSchema{
		Name: "wheat-field",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent spreading motion through the field from near-stillness into full waves."},
			{Key: "intro_breeze", Label: "intro breeze", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.08, Max: 0.5, Step: 0.02, Default: 0.16,
				Description: "Starting fraction of the full sway before the waves finish arriving."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent damping the field back toward calm."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 140, Step: 5, Default: 20,
				Description: "Extra quiet ticks for the last residual motion to settle out."},
			{Key: "ending_sway", Label: "ending sway", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.04, Max: 0.28, Step: 0.02, Default: 0.08,
				Description: "Residual sway fraction that remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.24, Max: 0.92, Step: 0.02, Default: 0.48,
				Description: "How densely the field is packed with visible stalk highlights."},
			{Key: "speed", Label: "wave speed", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "How quickly the broad waves travel through the field."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Preferred direction of the wave travel. Positive values push right."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.25, Max: 1.35, Step: 0.02, Default: 0.68,
				Description: "How far the stalk tips lean and recover."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.06, Max: 0.35, Step: 0.01, Default: 0.18,
				Description: "Horizontal frequency of the passing field waves."},
			{Key: "field_top", Label: "field top", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.54, Max: 0.74, Step: 0.01, Default: 0.62,
				Description: "Where the top of the wheat band sits in the frame."},
			{Key: "stalk_h", Label: "stalk height", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 8, Max: 28, Step: 1, Default: 18,
				Description: "Apparent height of the stalks rising above the field base."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of depth layers used to build the field."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 38, Max: 66, Step: 1, Default: 46,
				Description: "Base wheat hue. Lower values warm toward amber; higher values lean green-gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 32, Step: 1, Default: 18,
				Description: "Variation across stalk highlights and shadow bands."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.32, Max: 0.95, Step: 0.01, Default: 0.64,
				Description: "Overall color saturation of the field."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.18, Max: 0.55, Step: 0.01, Default: 0.30,
				Description: "Minimum lightness used for deeper shadowed wheat."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.55, Max: 0.92, Step: 0.01, Default: 0.76,
				Description: "Maximum lightness used for sunstruck stalk tips."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a stronger wind wave crossing the field."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the field settling into a quieter, lighter sway."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50,
				Description: "Typical duration of a stronger wind pulse."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.85,
				Description: "Sway multiplier applied during a gust."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the quieter low-amplitude window."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Sway multiplier applied while calm is active."},
		},
	}
}

func defaultWheatFieldConfig() WheatFieldConfig { return cloneConfig(wheatFieldDefaultsLocal) }

func mergeWheatFieldDefaults(cfg WheatFieldConfig) WheatFieldConfig {
	out := defaultWheatFieldConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = wheatFieldDefaultsLocal["intro_dur"]
	}
	out["intro_breeze"] = clamp01(out["intro_breeze"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = wheatFieldDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_sway"] = clamp01(out["ending_sway"])
	if out["density"] <= 0 {
		out["density"] = wheatFieldDefaultsLocal["density"]
	}
	if out["speed"] <= 0 {
		out["speed"] = wheatFieldDefaultsLocal["speed"]
	}
	if out["sway"] <= 0 {
		out["sway"] = wheatFieldDefaultsLocal["sway"]
	}
	if out["wave_freq"] <= 0 {
		out["wave_freq"] = wheatFieldDefaultsLocal["wave_freq"]
	}
	if out["field_top"] <= 0 {
		out["field_top"] = wheatFieldDefaultsLocal["field_top"]
	}
	if out["stalk_h"] <= 0 {
		out["stalk_h"] = wheatFieldDefaultsLocal["stalk_h"]
	}
	if out["layers"] < 1 {
		out["layers"] = wheatFieldDefaultsLocal["layers"]
	}
	if out["hue"] == 0 {
		out["hue"] = wheatFieldDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = wheatFieldDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = wheatFieldDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = wheatFieldDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = wheatFieldDefaultsLocal["gust_dur"]
	}
	if out["gust_mult"] <= 0 {
		out["gust_mult"] = wheatFieldDefaultsLocal["gust_mult"]
	}
	if out["calm_dur"] <= 0 {
		out["calm_dur"] = wheatFieldDefaultsLocal["calm_dur"]
	}
	if out["calm_mult"] <= 0 {
		out["calm_mult"] = wheatFieldDefaultsLocal["calm_mult"]
	}
	return out
}

func NewWheatField(w, h int, seed int64, cfg WheatFieldConfig) *WheatField {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &WheatField{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeWheatFieldDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (s *WheatField) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if w == s.W && h == s.H {
		return
	}
	s.W = w
	s.H = h
	s.Grid = make([][]Pixel, h)
	for i := range s.Grid {
		s.Grid[i] = make([]Pixel, w)
	}
}

func (s *WheatField) SetConfig(cfg WheatFieldConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = mergeWheatFieldDefaults(cfg)
}

func (s *WheatField) EffectiveConfig() WheatFieldConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.cfg)
}

func (s *WheatField) Snapshot() WheatFieldSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return WheatFieldSnapshot{ProceduralState: s.snapshotStateLocked()}
}

func (s *WheatField) RestoreSnapshot(snap WheatFieldSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.ProceduralState)
}

func (s *WheatField) SnapshotPersistedState() WheatFieldPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return WheatFieldPersistedState{
		ProceduralState: s.snapshotStateLocked(),
		RNGState:        s.rng.State(),
	}
}

func (s *WheatField) RestorePersistedState(ps WheatFieldPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		s.rng.SetState(ps.RNGState)
	}
}

func (s *WheatField) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   s.tick,
		Timers: cloneTimerMap(s.timers),
		Values: cloneValueMap(s.values),
	}
}

func (s *WheatField) restoreStateLocked(state ProceduralState) {
	s.tick = state.Tick
	s.timers = cloneTimerMap(state.Timers)
	if s.timers == nil {
		s.timers = make(map[string]int)
	}
	s.values = cloneValueMap(state.Values)
	if s.values == nil {
		s.values = make(map[string]float64)
	}
}

func (s *WheatField) CurrentTick() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

func (s *WheatField) PerturbRNG(delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng.Mix(delta)
}

func (s *WheatField) DrainLog() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return nil
	}
	out := s.log
	s.log = nil
	return out
}

func (s *WheatField) appendLog(kind, desc string) {
	s.log = append(s.log, LogEntry{Tick: s.tick, Type: kind, Desc: desc})
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *WheatField) intCfg(key string) int {
	return int(math.Round(s.cfg[key]))
}

func (s *WheatField) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "gust":
		s.startGustLocked("triggered")
	case "calm":
		s.startCalmLocked("triggered")
	case "intro":
		s.startIntroLocked()
		s.appendLog("intro", fmt.Sprintf("started (dur=%d, breeze=%.2f)", s.timers["intro"], s.cfg["intro_breeze"]))
	case "ending":
		s.startEndingLocked()
		s.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", s.intCfg("ending_dur"), s.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (s *WheatField) Step() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tick++
	for key, value := range s.timers {
		if value > 0 {
			s.timers[key] = value - 1
		}
	}
	s.stepLocked()
}

func (s *WheatField) startGustLocked(verb string) {
	s.timers["gust"] = jitterInt(s.rng, s.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if s.rng.Float64() < 0.35 {
		sign = -1
	}
	s.values["gust_push"] = sign * s.cfg["gust_mult"] * (0.55 + s.rng.Float64()*0.55)
	s.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, s.timers["gust"], s.values["gust_push"]))
}

func (s *WheatField) startCalmLocked(verb string) {
	s.timers["calm"] = jitterInt(s.rng, s.intCfg("calm_dur"), 0.3)
	s.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, s.timers["calm"], s.cfg["calm_mult"]))
}

func (s *WheatField) startIntroLocked() {
	s.timers["gust"] = 0
	s.timers["calm"] = 0
	s.timers["ending"] = 0
	s.values["gust_push"] = 0
	s.timers["intro"] = s.intCfg("intro_dur")
	s.values["intro_total"] = float64(s.timers["intro"])
}

func (s *WheatField) startEndingLocked() {
	s.timers["intro"] = 0
	s.timers["gust"] = 0
	s.timers["calm"] = 0
	s.values["gust_push"] = 0
	endingTotal := s.intCfg("ending_dur") + max(0, s.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, s.intCfg("ending_dur"))
	}
	s.timers["ending"] = endingTotal
	s.values["ending_total"] = float64(endingTotal)
}

func (s *WheatField) stepLocked() {
	if s.timers["gust"] <= 0 {
		s.values["gust_push"] = 0
	}
	if s.timers["intro"] <= 0 {
		delete(s.values, "intro_total")
	}
	if s.timers["ending"] <= 0 {
		delete(s.values, "ending_total")
	}
	if s.timers["intro"] > 0 || s.timers["ending"] > 0 {
		return
	}
	if s.timers["gust"] <= 0 && s.cfg["gust_p"] > 0 && s.rng.Float64() < s.cfg["gust_p"] {
		s.startGustLocked("started")
	}
	if s.timers["calm"] <= 0 && s.cfg["calm_p"] > 0 && s.rng.Float64() < s.cfg["calm_p"] {
		s.startCalmLocked("started")
	}
}
