package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type BeachConfig = ProceduralConfig

// BeachState is the shared procedural timer/value state plus the derived
// lifecycle contract field (computed at snapshot time, never restored).
type BeachState struct {
	ProceduralState
	Lifecycle Lifecycle `json:"lifecycle"`
}

// BeachSnapshot is the client-facing wire snapshot for Beach.
type BeachSnapshot struct {
	BeachState
	RNGState uint64 `json:"rngState,omitempty"`
}

// BeachPersistedState is the server-side resume state for Beach.
type BeachPersistedState struct {
	BeachState
	RNGState uint64 `json:"rngState"`
}

// Beach is a dedicated sim type for the beach effect, replacing the
// kind="beach" branch of the old Procedural type.
type Beach struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    BeachConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var beachDefaultsLocal = BeachConfig{
	"intro_dur":       55,
	"intro_tide":      0.18,
	"ending_dur":      65,
	"ending_linger":   18,
	"ending_wet":      0.10,
	"shoreline":       0.58,
	"tide_amp":        6.0,
	"wave_amp":        2.4,
	"wave_freq":       0.18,
	"speed":           0.10,
	"slope":           0.16,
	"foam":            0.36,
	"shimmer":         0.22,
	"hue":             198,
	"hue_sp":          16,
	"sat":             0.50,
	"lmin":            0.28,
	"lmax":            0.82,
	"high_tide_p":     0.0,
	"low_tide_p":      0.0,
	"foam_burst_p":    0.0,
	"high_tide_dur":   60,
	"high_tide_push":  1.40,
	"low_tide_dur":    58,
	"low_tide_pull":   1.20,
	"foam_burst_dur":  34,
	"foam_burst_mult": 1.90,
}

func BeachSchema() EffectSchema {
	return EffectSchema{
		Name: "beach",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent establishing the first advancing tide rhythm from a calmer shoreline."},
			{Key: "intro_tide", Label: "intro tide", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Starting fraction of the full tide motion before the shoreline settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent easing the waterline back out toward a calmer shore."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 140, Step: 5, Default: 18,
				Description: "Extra quiet ticks for wet sand and foam remnants to fade after the retreat."},
			{Key: "ending_wet", Label: "ending wet", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.10,
				Description: "Residual shoreline motion and wet-sand presence near the end of the outro."},
			{Key: "shoreline", Label: "shoreline", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.42, Max: 0.76, Step: 0.01, Default: 0.58,
				Description: "Base shoreline height in the frame."},
			{Key: "tide_amp", Label: "tide amp", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 1, Max: 14, Step: 0.5, Default: 6,
				Description: "How far the tide line advances and retreats."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.5, Max: 6, Step: 0.1, Default: 2.4,
				Description: "Height of the small shoreline ripples layered on top of the tide."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.05, Max: 0.35, Step: 0.01, Default: 0.18,
				Description: "Horizontal frequency of the shoreline wiggle."},
			{Key: "speed", Label: "tide speed", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.02, Max: 0.3, Step: 0.01, Default: 0.10,
				Description: "How quickly the tide rhythm moves in and out."},
			{Key: "slope", Label: "shore slope", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: -0.35, Max: 0.35, Step: 0.01, Default: 0.16,
				Description: "Diagonal slant of the shoreline across the frame."},
			{Key: "foam", Label: "foam", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.36,
				Description: "Brightness and thickness of the foam edge at the waterline."},
			{Key: "shimmer", Label: "shimmer", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Strength of the water-surface shimmer away from the foam line."},
			{Key: "hue", Label: "water hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 220, Step: 1, Default: 198,
				Description: "Base water hue. Lower values lean teal; higher values lean deep blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 16,
				Description: "Variation across water and foam accents."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.9, Step: 0.01, Default: 0.50,
				Description: "Overall water color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.6, Step: 0.01, Default: 0.28,
				Description: "Minimum lightness used for deeper water and wet sand."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for foam and bright shore reflections."},
			{Key: "high_tide_p", Label: "high tide", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "high-tide",
				Description: "Per-tick chance of the shoreline pushing farther inland for a while."},
			{Key: "low_tide_p", Label: "low tide", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "low-tide",
				Description: "Per-tick chance of the shoreline pulling farther back down the beach."},
			{Key: "foam_burst_p", Label: "foam burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "foam-burst",
				Description: "Per-tick chance of a brighter foamy wash crossing the edge."},
			{Key: "high_tide_dur", Label: "high dur", Slot: SlotEventMod, Group: "high-tide", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60,
				Description: "Duration of the stronger high-tide push."},
			{Key: "high_tide_push", Label: "high push", Slot: SlotEventMod, Group: "high-tide", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.4,
				Description: "Additional inward shoreline offset during high tide."},
			{Key: "low_tide_dur", Label: "low dur", Slot: SlotEventMod, Group: "low-tide", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "Duration of the lower-tide retreat."},
			{Key: "low_tide_pull", Label: "low pull", Slot: SlotEventMod, Group: "low-tide", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.2,
				Description: "Additional outward shoreline offset during low tide."},
			{Key: "foam_burst_dur", Label: "foam dur", Slot: SlotEventMod, Group: "foam-burst", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 34,
				Description: "Duration of a brighter foamy edge."},
			{Key: "foam_burst_mult", Label: "foam x", Slot: SlotEventMod, Group: "foam-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Brightness multiplier applied during a foam burst."},
		},
	}
}

func defaultBeachConfig() BeachConfig { return cloneConfig(beachDefaultsLocal) }

func mergeBeachDefaults(cfg BeachConfig) BeachConfig {
	out := defaultBeachConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = beachDefaultsLocal["intro_dur"]
	}
	out["intro_tide"] = clamp01(out["intro_tide"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = beachDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_wet"] = clamp01(out["ending_wet"])
	if out["shoreline"] <= 0 {
		out["shoreline"] = beachDefaultsLocal["shoreline"]
	}
	if out["tide_amp"] <= 0 {
		out["tide_amp"] = beachDefaultsLocal["tide_amp"]
	}
	if out["wave_amp"] <= 0 {
		out["wave_amp"] = beachDefaultsLocal["wave_amp"]
	}
	if out["wave_freq"] <= 0 {
		out["wave_freq"] = beachDefaultsLocal["wave_freq"]
	}
	if out["speed"] <= 0 {
		out["speed"] = beachDefaultsLocal["speed"]
	}
	if out["foam"] <= 0 {
		out["foam"] = beachDefaultsLocal["foam"]
	}
	if out["shimmer"] <= 0 {
		out["shimmer"] = beachDefaultsLocal["shimmer"]
	}
	if out["hue"] == 0 {
		out["hue"] = beachDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = beachDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = beachDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = beachDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["high_tide_dur"] <= 0 {
		out["high_tide_dur"] = beachDefaultsLocal["high_tide_dur"]
	}
	if out["high_tide_push"] <= 0 {
		out["high_tide_push"] = beachDefaultsLocal["high_tide_push"]
	}
	if out["low_tide_dur"] <= 0 {
		out["low_tide_dur"] = beachDefaultsLocal["low_tide_dur"]
	}
	if out["low_tide_pull"] <= 0 {
		out["low_tide_pull"] = beachDefaultsLocal["low_tide_pull"]
	}
	if out["foam_burst_dur"] <= 0 {
		out["foam_burst_dur"] = beachDefaultsLocal["foam_burst_dur"]
	}
	if out["foam_burst_mult"] <= 0 {
		out["foam_burst_mult"] = beachDefaultsLocal["foam_burst_mult"]
	}
	return out
}

func NewBeach(w, h int, seed int64, cfg BeachConfig) *Beach {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Beach{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeBeachDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (b *Beach) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if w == b.W && h == b.H {
		return
	}
	b.W = w
	b.H = h
	b.Grid = make([][]Pixel, h)
	for i := range b.Grid {
		b.Grid[i] = make([]Pixel, w)
	}
}

func (b *Beach) SetConfig(cfg BeachConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cfg = mergeBeachDefaults(cfg)
}

func (b *Beach) EffectiveConfig() BeachConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneConfig(b.cfg)
}

func (b *Beach) Snapshot() BeachSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BeachSnapshot{
		BeachState: b.snapshotStateLocked(),
		RNGState:   b.rng.State(),
	}
}

func (b *Beach) RestoreSnapshot(snap BeachSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		b.rng.SetState(snap.RNGState)
	}
}

func (b *Beach) SnapshotPersistedState() BeachPersistedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BeachPersistedState{
		BeachState: b.snapshotStateLocked(),
		RNGState:   b.rng.State(),
	}
}

func (b *Beach) RestorePersistedState(ps BeachPersistedState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		b.rng.SetState(ps.RNGState)
	}
}

func (b *Beach) snapshotStateLocked() BeachState {
	return BeachState{
		ProceduralState: ProceduralState{
			Tick:   b.tick,
			Timers: cloneTimerMap(b.timers),
			Values: cloneValueMap(b.values),
		},
		Lifecycle: b.lifecycleLocked(),
	}
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// the intro/ending timers. Beach's outro is non-terminal: when the ending
// timer expires the surf returns to steady state and automatic events
// resume, so lifecycle returns to running (the schema declares
// ending_terminal: false by omission).
func (b *Beach) lifecycleLocked() Lifecycle {
	switch {
	case b.timers["intro"] > 0:
		return LifecycleIntro
	case b.timers["ending"] > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (b *Beach) restoreStateLocked(state ProceduralState) {
	b.tick = state.Tick
	b.timers = cloneTimerMap(state.Timers)
	if b.timers == nil {
		b.timers = make(map[string]int)
	}
	b.values = cloneValueMap(state.Values)
	if b.values == nil {
		b.values = make(map[string]float64)
	}
}

func (b *Beach) CurrentTick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tick
}

func (b *Beach) PerturbRNG(delta int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rng.Mix(delta)
}

func (b *Beach) DrainLog() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) == 0 {
		return nil
	}
	out := b.log
	b.log = nil
	return out
}

func (b *Beach) appendLog(kind, desc string) {
	b.log = append(b.log, LogEntry{Tick: b.tick, Type: kind, Desc: desc})
	if len(b.log) > 200 {
		b.log = b.log[len(b.log)-200:]
	}
}

func (b *Beach) intCfg(key string) int {
	return int(math.Round(b.cfg[key]))
}

func (b *Beach) TriggerEvent(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch name {
	case "high-tide":
		b.startHighTideLocked("triggered")
	case "low-tide":
		b.startLowTideLocked("triggered")
	case "foam-burst":
		b.startFoamBurstLocked("triggered")
	case "intro":
		b.startIntroLocked()
		b.appendLog("intro", fmt.Sprintf("started (dur=%d, tide=%.2f)", b.timers["intro"], b.cfg["intro_tide"]))
	case "ending":
		b.startEndingLocked()
		b.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", b.intCfg("ending_dur"), b.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (b *Beach) Step() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tick++
	for key, value := range b.timers {
		if value > 0 {
			b.timers[key] = value - 1
		}
	}
	b.stepLocked()
}

func (b *Beach) startHighTideLocked(verb string) {
	b.timers["high-tide"] = jitterInt(b.rng, b.intCfg("high_tide_dur"), 0.3)
	b.timers["low-tide"] = 0
	b.values["tide_bias"] = b.cfg["high_tide_push"] * (0.65 + b.rng.Float64()*0.55)
	b.appendLog("high-tide", fmt.Sprintf("%s (dur=%d, bias=+%.2f)", verb, b.timers["high-tide"], b.values["tide_bias"]))
}

func (b *Beach) startLowTideLocked(verb string) {
	b.timers["low-tide"] = jitterInt(b.rng, b.intCfg("low_tide_dur"), 0.3)
	b.timers["high-tide"] = 0
	b.values["tide_bias"] = -b.cfg["low_tide_pull"] * (0.65 + b.rng.Float64()*0.55)
	b.appendLog("low-tide", fmt.Sprintf("%s (dur=%d, bias=%.2f)", verb, b.timers["low-tide"], b.values["tide_bias"]))
}

func (b *Beach) startFoamBurstLocked(verb string) {
	b.timers["foam-burst"] = jitterInt(b.rng, b.intCfg("foam_burst_dur"), 0.3)
	b.values["foam_gain"] = b.cfg["foam_burst_mult"] * (0.85 + b.rng.Float64()*0.35)
	b.appendLog("foam-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, b.timers["foam-burst"], b.values["foam_gain"]))
}

func (b *Beach) startIntroLocked() {
	b.timers["high-tide"] = 0
	b.timers["low-tide"] = 0
	b.timers["foam-burst"] = 0
	b.timers["ending"] = 0
	b.values["tide_bias"] = 0
	b.values["foam_gain"] = 1
	b.timers["intro"] = b.intCfg("intro_dur")
	b.values["intro_total"] = float64(b.timers["intro"])
}

func (b *Beach) startEndingLocked() {
	b.timers["intro"] = 0
	b.timers["high-tide"] = 0
	b.timers["low-tide"] = 0
	b.timers["foam-burst"] = 0
	b.values["tide_bias"] = 0
	b.values["foam_gain"] = 1
	endingTotal := b.intCfg("ending_dur") + max(0, b.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, b.intCfg("ending_dur"))
	}
	b.timers["ending"] = endingTotal
	b.values["ending_total"] = float64(endingTotal)
}

func (b *Beach) stepLocked() {
	if b.timers["high-tide"] <= 0 && b.timers["low-tide"] <= 0 {
		b.values["tide_bias"] = 0
	}
	if b.timers["foam-burst"] <= 0 {
		b.values["foam_gain"] = 1
	}
	if b.timers["intro"] <= 0 {
		delete(b.values, "intro_total")
	}
	if b.timers["ending"] <= 0 {
		delete(b.values, "ending_total")
	}
	if b.timers["intro"] > 0 || b.timers["ending"] > 0 {
		return
	}
	if b.timers["high-tide"] <= 0 && b.timers["low-tide"] <= 0 && b.cfg["high_tide_p"] > 0 && b.rng.Float64() < b.cfg["high_tide_p"] {
		b.startHighTideLocked("started")
	}
	if b.timers["low-tide"] <= 0 && b.timers["high-tide"] <= 0 && b.cfg["low_tide_p"] > 0 && b.rng.Float64() < b.cfg["low_tide_p"] {
		b.startLowTideLocked("started")
	}
	if b.timers["foam-burst"] <= 0 && b.cfg["foam_burst_p"] > 0 && b.rng.Float64() < b.cfg["foam_burst_p"] {
		b.startFoamBurstLocked("started")
	}
}
