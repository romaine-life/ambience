package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type StarfieldConfig = ProceduralConfig

// StarfieldState is the shared procedural timers/values state plus the
// derived lifecycle observer field (recomputed at snapshot time, ignored on
// restore).
type StarfieldState struct {
	ProceduralState
	Lifecycle Lifecycle `json:"lifecycle"`
}

// StarfieldSnapshot is the wire shape returned by Snapshot().
type StarfieldSnapshot struct {
	StarfieldState
	RNGState uint64 `json:"rngState,omitempty"`
}

// StarfieldPersistedState is the on-disk shape returned by
// SnapshotPersistedState().
type StarfieldPersistedState struct {
	StarfieldState
	RNGState uint64 `json:"rngState"`
}

// Starfield is a dedicated sim type for the starfield effect, replacing the
// kind="starfield" branch of the old Procedural type.
type Starfield struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    StarfieldConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var starfieldDefaultsLocal = StarfieldConfig{
	"intro_dur":          50,
	"intro_density":      0.08,
	"ending_dur":         60,
	"ending_linger":      16,
	"ending_density":     0.03,
	"density":            0.22,
	"speed":              0.12,
	"drift":              0.04,
	"layers":             3,
	"size":               1.0,
	"hue":                218,
	"hue_sp":             18,
	"sat":                0.18,
	"lmin":               0.55,
	"lmax":               0.95,
	"shooting_star_p":    0.0,
	"twinkle_burst_p":    0.0,
	"shooting_star_dur":  26,
	"shooting_star_mult": 1.8,
	"twinkle_burst_dur":  42,
	"twinkle_burst_mult": 1.7,
}

func StarfieldSchema() EffectSchema {
	return EffectSchema{
		Name: "starfield",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent populating the star layers from sparse points into the full field."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.01, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Starting fraction of the full star density before the field finishes blooming in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent dimming the starfield back toward near-darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 16,
				Description: "Extra quiet ticks after the fade so the last points can linger."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.3, Step: 0.01, Default: 0.03,
				Description: "How much of the far starfield remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Base star density across the full field."},
			{Key: "speed", Label: "parallax", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "How quickly the parallax layers drift across the scene."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: -0.25, Max: 0.25, Step: 0.01, Default: 0.04,
				Description: "Baseline horizontal drift direction for the starfield."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of parallax layers."},
			{Key: "size", Label: "star size", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.5, Max: 2.5, Step: 0.1, Default: 1,
				Description: "Pixel size of the nearest stars."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 190, Max: 250, Step: 1, Default: 218,
				Description: "Base star tint. Lower values lean blue-cyan; higher values lean violet."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 18,
				Description: "Variation in the star tint across the field."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.01, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Overall star color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.85, Step: 0.01, Default: 0.55,
				Description: "Minimum lightness used for the dimmest background stars."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1.0, Step: 0.01, Default: 0.95,
				Description: "Maximum lightness used for the nearest stars and bright accents."},
			{Key: "shooting_star_p", Label: "shooting star", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "shooting-star",
				Description: "Per-tick chance of a rare shooting star crossing the field."},
			{Key: "twinkle_burst_p", Label: "twinkle burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "twinkle-burst",
				Description: "Per-tick chance of a brief field-wide brightening."},
			{Key: "shooting_star_dur", Label: "shoot dur", Slot: SlotEventMod, Group: "shooting-star", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 26,
				Description: "How long a shooting star remains visible."},
			{Key: "shooting_star_mult", Label: "shoot x", Slot: SlotEventMod, Group: "shooting-star", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.8,
				Description: "Brightness multiplier for the shooting star accent."},
			{Key: "twinkle_burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "twinkle-burst", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 42,
				Description: "Duration of the twinkle burst brightening window."},
			{Key: "twinkle_burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "twinkle-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.7,
				Description: "Brightness multiplier applied during a twinkle burst."},
		},
	}
}

func defaultStarfieldConfig() StarfieldConfig { return cloneConfig(starfieldDefaultsLocal) }

func mergeStarfieldDefaults(cfg StarfieldConfig) StarfieldConfig {
	out := defaultStarfieldConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = starfieldDefaultsLocal["intro_dur"]
	}
	out["intro_density"] = clamp01(out["intro_density"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = starfieldDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_density"] = clamp01(out["ending_density"])
	if out["density"] <= 0 {
		out["density"] = starfieldDefaultsLocal["density"]
	}
	if out["speed"] <= 0 {
		out["speed"] = starfieldDefaultsLocal["speed"]
	}
	if out["layers"] < 1 {
		out["layers"] = starfieldDefaultsLocal["layers"]
	}
	if out["size"] <= 0 {
		out["size"] = starfieldDefaultsLocal["size"]
	}
	if out["hue"] == 0 {
		out["hue"] = starfieldDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = starfieldDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = starfieldDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = starfieldDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["shooting_star_dur"] <= 0 {
		out["shooting_star_dur"] = starfieldDefaultsLocal["shooting_star_dur"]
	}
	if out["shooting_star_mult"] <= 0 {
		out["shooting_star_mult"] = starfieldDefaultsLocal["shooting_star_mult"]
	}
	if out["twinkle_burst_dur"] <= 0 {
		out["twinkle_burst_dur"] = starfieldDefaultsLocal["twinkle_burst_dur"]
	}
	if out["twinkle_burst_mult"] <= 0 {
		out["twinkle_burst_mult"] = starfieldDefaultsLocal["twinkle_burst_mult"]
	}
	return out
}

func NewStarfield(w, h int, seed int64, cfg StarfieldConfig) *Starfield {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Starfield{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeStarfieldDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (s *Starfield) Resize(w, h int) {
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

func (s *Starfield) SetConfig(cfg StarfieldConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = mergeStarfieldDefaults(cfg)
}

func (s *Starfield) EffectiveConfig() StarfieldConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.cfg)
}

func (s *Starfield) Snapshot() StarfieldSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StarfieldSnapshot{
		StarfieldState: s.snapshotStateLocked(),
		RNGState:       s.rng.State(),
	}
}

func (s *Starfield) RestoreSnapshot(snap StarfieldSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		s.rng.SetState(snap.RNGState)
	}
}

func (s *Starfield) SnapshotPersistedState() StarfieldPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StarfieldPersistedState{
		StarfieldState: s.snapshotStateLocked(),
		RNGState:       s.rng.State(),
	}
}

func (s *Starfield) RestorePersistedState(ps StarfieldPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		s.rng.SetState(ps.RNGState)
	}
}

func (s *Starfield) snapshotStateLocked() StarfieldState {
	return StarfieldState{
		ProceduralState: ProceduralState{
			Tick:   s.tick,
			Timers: cloneTimerMap(s.timers),
			Values: cloneValueMap(s.values),
		},
		Lifecycle: s.lifecycleLocked(),
	}
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// the starfield's internal timers. The outro is non-terminal: once
// timers["ending"] expires, automatic events resume, so lifecycle returns to
// running (the schema declares ending_terminal: false by omission).
func (s *Starfield) lifecycleLocked() Lifecycle {
	switch {
	case s.timers["intro"] > 0:
		return LifecycleIntro
	case s.timers["ending"] > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (s *Starfield) restoreStateLocked(state ProceduralState) {
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

func (s *Starfield) CurrentTick() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

func (s *Starfield) PerturbRNG(delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng.Mix(delta)
}

func (s *Starfield) DrainLog() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return nil
	}
	out := s.log
	s.log = nil
	return out
}

func (s *Starfield) appendLog(kind, desc string) {
	s.log = append(s.log, LogEntry{Tick: s.tick, Type: kind, Desc: desc})
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *Starfield) intCfg(key string) int {
	return int(math.Round(s.cfg[key]))
}

func (s *Starfield) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "shooting-star":
		s.startShootingStarLocked("triggered")
	case "twinkle-burst":
		s.startTwinkleBurstLocked("triggered")
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

func (s *Starfield) Step() {
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

func (s *Starfield) startShootingStarLocked(verb string) {
	s.timers["shooting-star"] = jitterInt(s.rng, s.intCfg("shooting_star_dur"), 0.3)
	sign := 1.0
	if s.rng.Float64() < 0.5 {
		sign = -1
	}
	s.values["shooting_dir"] = sign
	s.values["shooting_row"] = 6 + s.rng.Float64()*math.Max(4, float64(s.H)/3)
	s.values["shooting_start"] = s.rng.Float64() * float64(max(1, s.W))
	s.appendLog("shooting-star", fmt.Sprintf("%s (dur=%d, dir=%+.0f)", verb, s.timers["shooting-star"], sign))
}

func (s *Starfield) startTwinkleBurstLocked(verb string) {
	s.timers["twinkle-burst"] = jitterInt(s.rng, s.intCfg("twinkle_burst_dur"), 0.3)
	s.appendLog("twinkle-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, s.timers["twinkle-burst"], s.cfg["twinkle_burst_mult"]))
}

func (s *Starfield) startIntroLocked() {
	s.timers["ending"] = 0
	s.timers["shooting-star"] = 0
	s.timers["twinkle-burst"] = 0
	s.timers["intro"] = s.intCfg("intro_dur")
	s.values["intro_total"] = float64(s.timers["intro"])
}

func (s *Starfield) startEndingLocked() {
	s.timers["intro"] = 0
	s.timers["shooting-star"] = 0
	s.timers["twinkle-burst"] = 0
	endingTotal := s.intCfg("ending_dur") + max(0, s.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, s.intCfg("ending_dur"))
	}
	s.timers["ending"] = endingTotal
	s.values["ending_total"] = float64(endingTotal)
}

func (s *Starfield) stepLocked() {
	if s.timers["intro"] <= 0 {
		delete(s.values, "intro_total")
	}
	if s.timers["ending"] <= 0 {
		delete(s.values, "ending_total")
	}
	if s.timers["intro"] > 0 || s.timers["ending"] > 0 {
		return
	}
	if s.timers["shooting-star"] <= 0 && s.cfg["shooting_star_p"] > 0 && s.rng.Float64() < s.cfg["shooting_star_p"] {
		s.startShootingStarLocked("started")
	}
	if s.timers["twinkle-burst"] <= 0 && s.cfg["twinkle_burst_p"] > 0 && s.rng.Float64() < s.cfg["twinkle_burst_p"] {
		s.startTwinkleBurstLocked("started")
	}
}
