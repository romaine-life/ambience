package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// SnowConfig keeps the lightweight ProceduralConfig shape so the legacy
// browser-first prototypes don't need a typed-struct cleanup pass yet. The
// per-file isolation is the win here — each new procedural effect now lives
// in its own dedicated Go file instead of mutating a 3000-line shared file.
type SnowConfig = ProceduralConfig

// SnowState mirrors ProceduralState; kept as an alias so snapshot wire
// shapes stay byte-identical with the legacy Procedural type.
type SnowState = ProceduralState

// SnowSnapshot is the wire shape returned by Snapshot().
type SnowSnapshot = ProceduralSnapshot

// SnowPersistedState is the on-disk shape returned by SnapshotPersistedState().
type SnowPersistedState = ProceduralPersistedState

// Snow is a dedicated sim type for the snow effect, replacing the kind="snow"
// branch of the old Procedural type.
type Snow struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    SnowConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var snowDefaults = SnowConfig{
	"intro_dur":      60,
	"intro_density":  0.16,
	"ending_dur":     70,
	"ending_linger":  22,
	"ending_density": 0.08,
	"density":        0.32,
	"speed":          0.48,
	"drift":          0.08,
	"sway":           0.42,
	"layers":         3,
	"size":           1.0,
	"hue":            210,
	"hue_sp":         12,
	"sat":            0.16,
	"lmin":           0.74,
	"lmax":           0.98,
	"gust_p":         0.0,
	"calm_p":         0.0,
	"gust_dur":       55,
	"gust_mult":      1.85,
	"calm_dur":       80,
	"calm_mult":      0.42,
}

func SnowSchema() EffectSchema {
	return EffectSchema{
		Name: "snow",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent ramping from a few first flakes into the full snowfall."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.16,
				Description: "Starting snowfall fraction before the full field settles in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering the snowfall down toward still air."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks after the taper so the last flakes can drift out."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.02, Default: 0.08,
				Description: "How much low-level snowfall remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.32,
				Description: "Base snowfall density across all layers."},
			{Key: "speed", Label: "fall speed", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.5, Step: 0.02, Default: 0.48,
				Description: "How quickly flakes descend through the field."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Baseline sideways carry. Positive drifts right, negative drifts left."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0, Max: 1.2, Step: 0.02, Default: 0.42,
				Description: "Side-to-side meander in each flake's path."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "fall", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of snowfall depth layers."},
			{Key: "size", Label: "flake size", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.5, Max: 2.5, Step: 0.1, Default: 1,
				Description: "Pixel size of the brightest foreground flakes."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 240, Step: 1, Default: 210,
				Description: "Base snow tint. Lower values warm toward dusk; higher values cool toward ice."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Variation in the flake tint and sky reflection."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.01, Max: 0.45, Step: 0.01, Default: 0.16,
				Description: "Overall color saturation of the snow scene."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.9, Step: 0.01, Default: 0.74,
				Description: "Minimum lightness used for dim flakes and distant haze."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1.0, Step: 0.01, Default: 0.98,
				Description: "Maximum lightness used for the brightest near flakes."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a crosswind kicking the snowfall sideways."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the snowfall briefly thinning into stillness."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55,
				Description: "Typical gust duration in ticks (jittered by +/-30%)."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.85,
				Description: "How strongly a gust bends the snowfall sideways."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 80,
				Description: "Duration of the quieter low-density window."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.42,
				Description: "Density multiplier applied while calm is active."},
		},
	}
}

func defaultSnowConfig() SnowConfig { return cloneConfig(snowDefaults) }

func mergeSnowDefaults(cfg SnowConfig) SnowConfig {
	out := defaultSnowConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = snowDefaults["intro_dur"]
	}
	out["intro_density"] = clamp01(out["intro_density"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = snowDefaults["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_density"] = clamp01(out["ending_density"])
	if out["density"] <= 0 {
		out["density"] = snowDefaults["density"]
	}
	if out["speed"] <= 0 {
		out["speed"] = snowDefaults["speed"]
	}
	if out["layers"] < 1 {
		out["layers"] = snowDefaults["layers"]
	}
	if out["size"] <= 0 {
		out["size"] = snowDefaults["size"]
	}
	if out["hue"] == 0 {
		out["hue"] = snowDefaults["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = snowDefaults["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = snowDefaults["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = snowDefaults["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = snowDefaults["gust_dur"]
	}
	if out["gust_mult"] <= 0 {
		out["gust_mult"] = snowDefaults["gust_mult"]
	}
	if out["calm_dur"] <= 0 {
		out["calm_dur"] = snowDefaults["calm_dur"]
	}
	if out["calm_mult"] <= 0 {
		out["calm_mult"] = snowDefaults["calm_mult"]
	}
	return out
}

// NewSnow builds a Snow instance with the given grid size, RNG seed and
// merged-with-defaults config.
func NewSnow(w, h int, seed int64, cfg SnowConfig) *Snow {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Snow{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeSnowDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (s *Snow) Resize(w, h int) {
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

func (s *Snow) SetConfig(cfg SnowConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = mergeSnowDefaults(cfg)
}

func (s *Snow) EffectiveConfig() SnowConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.cfg)
}

func (s *Snow) Snapshot() SnowSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SnowSnapshot{ProceduralState: s.snapshotStateLocked()}
}

func (s *Snow) RestoreSnapshot(snap SnowSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.ProceduralState)
}

func (s *Snow) SnapshotPersistedState() SnowPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SnowPersistedState{
		ProceduralState: s.snapshotStateLocked(),
		RNGState:        s.rng.State(),
	}
}

func (s *Snow) RestorePersistedState(ps SnowPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		s.rng.SetState(ps.RNGState)
	}
}

func (s *Snow) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   s.tick,
		Timers: cloneTimerMap(s.timers),
		Values: cloneValueMap(s.values),
	}
}

func (s *Snow) restoreStateLocked(state ProceduralState) {
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

func (s *Snow) CurrentTick() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

func (s *Snow) PerturbRNG(delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng.Mix(delta)
}

func (s *Snow) DrainLog() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return nil
	}
	out := s.log
	s.log = nil
	return out
}

func (s *Snow) appendLog(kind, desc string) {
	s.log = append(s.log, LogEntry{Tick: s.tick, Type: kind, Desc: desc})
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *Snow) intCfg(key string) int {
	return int(math.Round(s.cfg[key]))
}

func (s *Snow) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "gust":
		s.startGustLocked("triggered")
	case "calm":
		s.startCalmLocked("triggered")
	case "intro":
		s.startIntroLocked()
		s.appendLog("intro", fmt.Sprintf("started (dur=%d, density=%.2f)", s.timers["intro"], s.cfg["intro_density"]))
	case "ending":
		s.startEndingLocked()
		s.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", s.intCfg("ending_dur"), s.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (s *Snow) Step() {
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

func (s *Snow) startGustLocked(verb string) {
	s.timers["gust"] = jitterInt(s.rng, s.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if s.rng.Float64() < 0.5 {
		sign = -1
	}
	s.values["gust_push"] = sign * s.cfg["gust_mult"] * (0.45 + s.rng.Float64()*0.55)
	s.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, s.timers["gust"], s.values["gust_push"]))
}

func (s *Snow) startCalmLocked(verb string) {
	s.timers["calm"] = jitterInt(s.rng, s.intCfg("calm_dur"), 0.3)
	s.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, s.timers["calm"], s.cfg["calm_mult"]))
}

func (s *Snow) startIntroLocked() {
	s.timers["gust"] = 0
	s.timers["calm"] = 0
	s.timers["ending"] = 0
	s.values["gust_push"] = 0
	s.timers["intro"] = s.intCfg("intro_dur")
	s.values["intro_total"] = float64(s.timers["intro"])
}

func (s *Snow) startEndingLocked() {
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

func (s *Snow) stepLocked() {
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
