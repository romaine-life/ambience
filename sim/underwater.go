package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type UnderwaterConfig = ProceduralConfig
type UnderwaterState = ProceduralState
type UnderwaterSnapshot = ProceduralSnapshot
type UnderwaterPersistedState = ProceduralPersistedState

// Underwater is a dedicated sim type for the underwater effect.
type Underwater struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    UnderwaterConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var underwaterDefaultsLocal = UnderwaterConfig{
	"intro_dur":          55,
	"intro_reveal":       0.14,
	"ending_dur":         70,
	"ending_linger":      22,
	"ending_murk":        0.08,
	"density":            0.28,
	"rise_speed":         0.42,
	"drift":              0.10,
	"sway":               0.54,
	"weed_height":        20.0,
	"weed_count":         11.0,
	"caustics":           0.30,
	"depth":              0.56,
	"hue":                192,
	"hue_sp":             18,
	"sat":                0.42,
	"lmin":               0.12,
	"lmax":               0.82,
	"bubble_burst_p":     0.0,
	"current_shift_p":    0.0,
	"calm_p":             0.0,
	"bubble_burst_dur":   38,
	"bubble_burst_mult":  1.90,
	"current_shift_dur":  62,
	"current_shift_push": 1.20,
	"calm_dur":           74,
	"calm_mult":          0.55,
}

func UnderwaterSchema() EffectSchema {
	return EffectSchema{
		Name: "underwater",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent brightening out of murk before the full underwater scene settles in."},
			{Key: "intro_reveal", Label: "intro reveal", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Starting fraction of the final bubble, caustic, and sway activity."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent dimming the water column back toward still murk."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks after the main motion has mostly faded."},
			{Key: "ending_murk", Label: "ending murk", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual underwater activity and light left near the end of the outro."},
			{Key: "density", Label: "bubble density", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.28,
				Description: "Base density of drifting bubbles across the scene."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.02, Default: 0.42,
				Description: "How quickly bubbles rise through the water column."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.01, Default: 0.10,
				Description: "Baseline sideways carry applied to bubbles and suspended particles."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "seaweed", Type: KnobFloat, Min: 0.05, Max: 1.4, Step: 0.02, Default: 0.54,
				Description: "How much the seaweed and finer particles sway with the water."},
			{Key: "weed_height", Label: "weed height", Slot: SlotLever, Group: "seaweed", Type: KnobFloat, Min: 8, Max: 30, Step: 0.5, Default: 20,
				Description: "Height of the tallest seaweed fronds."},
			{Key: "weed_count", Label: "weed count", Slot: SlotLever, Group: "seaweed", Type: KnobInt, Min: 4, Max: 18, Step: 1, Default: 11,
				Description: "Number of seaweed clumps anchored along the bottom."},
			{Key: "caustics", Label: "caustics", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.30,
				Description: "Strength of the shifting light bands and shafts near the surface."},
			{Key: "depth", Label: "depth", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0.15, Max: 0.95, Step: 0.01, Default: 0.56,
				Description: "How murky and deep the water feels overall."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 220, Step: 1, Default: 192,
				Description: "Base water hue. Lower values lean greener; higher values lean bluer."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between the light shafts, water body, and seaweed glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.42,
				Description: "Overall underwater color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Minimum lightness used for deep water and seabed shadows."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for bubbles and surface caustics."},
			{Key: "bubble_burst_p", Label: "bubble burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "bubble-burst",
				Description: "Per-tick chance of a denser burst of bubbles rising through the frame."},
			{Key: "current_shift_p", Label: "current shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "current-shift",
				Description: "Per-tick chance of the water current leaning harder to one side."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of bubble activity and sway easing into a quieter interval."},
			{Key: "bubble_burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 38,
				Description: "Duration of a denser bubble burst."},
			{Key: "bubble_burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Bubble density multiplier applied during a bubble burst."},
			{Key: "current_shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "current-shift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 62,
				Description: "Duration of the stronger sideways current window."},
			{Key: "current_shift_push", Label: "shift push", Slot: SlotEventMod, Group: "current-shift", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.2,
				Description: "Additional horizontal push applied during a current shift."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 74,
				Description: "Duration of the calmer low-motion underwater interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Bubble and sway multiplier applied while calm is active."},
		},
	}
}

func defaultUnderwaterConfig() UnderwaterConfig { return cloneConfig(underwaterDefaultsLocal) }

func mergeUnderwaterDefaults(cfg UnderwaterConfig) UnderwaterConfig {
	out := defaultUnderwaterConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = underwaterDefaultsLocal["intro_dur"]
	}
	out["intro_reveal"] = clamp01(out["intro_reveal"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = underwaterDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_murk"] = clamp01(out["ending_murk"])
	if out["density"] <= 0 {
		out["density"] = underwaterDefaultsLocal["density"]
	}
	if out["rise_speed"] <= 0 {
		out["rise_speed"] = underwaterDefaultsLocal["rise_speed"]
	}
	if out["sway"] <= 0 {
		out["sway"] = underwaterDefaultsLocal["sway"]
	}
	if out["weed_height"] <= 0 {
		out["weed_height"] = underwaterDefaultsLocal["weed_height"]
	}
	if out["weed_count"] < 1 {
		out["weed_count"] = underwaterDefaultsLocal["weed_count"]
	}
	if out["caustics"] <= 0 {
		out["caustics"] = underwaterDefaultsLocal["caustics"]
	}
	if out["depth"] <= 0 {
		out["depth"] = underwaterDefaultsLocal["depth"]
	}
	if out["hue"] == 0 {
		out["hue"] = underwaterDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = underwaterDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = underwaterDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = underwaterDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["bubble_burst_dur"] <= 0 {
		out["bubble_burst_dur"] = underwaterDefaultsLocal["bubble_burst_dur"]
	}
	if out["bubble_burst_mult"] <= 0 {
		out["bubble_burst_mult"] = underwaterDefaultsLocal["bubble_burst_mult"]
	}
	if out["current_shift_dur"] <= 0 {
		out["current_shift_dur"] = underwaterDefaultsLocal["current_shift_dur"]
	}
	if out["current_shift_push"] <= 0 {
		out["current_shift_push"] = underwaterDefaultsLocal["current_shift_push"]
	}
	if out["calm_dur"] <= 0 {
		out["calm_dur"] = underwaterDefaultsLocal["calm_dur"]
	}
	if out["calm_mult"] <= 0 {
		out["calm_mult"] = underwaterDefaultsLocal["calm_mult"]
	}
	return out
}

func NewUnderwater(w, h int, seed int64, cfg UnderwaterConfig) *Underwater {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Underwater{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeUnderwaterDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (u *Underwater) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if w == u.W && h == u.H {
		return
	}
	u.W = w
	u.H = h
	u.Grid = make([][]Pixel, h)
	for i := range u.Grid {
		u.Grid[i] = make([]Pixel, w)
	}
}

func (u *Underwater) SetConfig(cfg UnderwaterConfig) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cfg = mergeUnderwaterDefaults(cfg)
}

func (u *Underwater) EffectiveConfig() UnderwaterConfig {
	u.mu.Lock()
	defer u.mu.Unlock()
	return cloneConfig(u.cfg)
}

func (u *Underwater) Snapshot() UnderwaterSnapshot {
	u.mu.Lock()
	defer u.mu.Unlock()
	return UnderwaterSnapshot{ProceduralState: u.snapshotStateLocked()}
}

func (u *Underwater) RestoreSnapshot(snap UnderwaterSnapshot) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.restoreStateLocked(snap.ProceduralState)
}

func (u *Underwater) SnapshotPersistedState() UnderwaterPersistedState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return UnderwaterPersistedState{
		ProceduralState: u.snapshotStateLocked(),
		RNGState:        u.rng.State(),
	}
}

func (u *Underwater) RestorePersistedState(ps UnderwaterPersistedState) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		u.rng.SetState(ps.RNGState)
	}
}

func (u *Underwater) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   u.tick,
		Timers: cloneTimerMap(u.timers),
		Values: cloneValueMap(u.values),
	}
}

func (u *Underwater) restoreStateLocked(state ProceduralState) {
	u.tick = state.Tick
	u.timers = cloneTimerMap(state.Timers)
	if u.timers == nil {
		u.timers = make(map[string]int)
	}
	u.values = cloneValueMap(state.Values)
	if u.values == nil {
		u.values = make(map[string]float64)
	}
}

func (u *Underwater) CurrentTick() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.tick
}

func (u *Underwater) PerturbRNG(delta int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.rng.Mix(delta)
}

func (u *Underwater) DrainLog() []LogEntry {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.log) == 0 {
		return nil
	}
	out := u.log
	u.log = nil
	return out
}

func (u *Underwater) appendLog(kind, desc string) {
	u.log = append(u.log, LogEntry{Tick: u.tick, Type: kind, Desc: desc})
	if len(u.log) > 200 {
		u.log = u.log[len(u.log)-200:]
	}
}

func (u *Underwater) intCfg(key string) int {
	return int(math.Round(u.cfg[key]))
}

func (u *Underwater) TriggerEvent(name string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	switch name {
	case "bubble-burst":
		u.startBubbleBurstLocked("triggered")
	case "current-shift":
		u.startCurrentShiftLocked("triggered")
	case "calm":
		u.startCalmLocked("triggered")
	case "intro":
		u.startIntroLocked()
		u.appendLog("intro", fmt.Sprintf("started (dur=%d, reveal=%.2f)", u.timers["intro"], u.cfg["intro_reveal"]))
	case "ending":
		u.startEndingLocked()
		u.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", u.intCfg("ending_dur"), u.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (u *Underwater) Step() {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.tick++
	for key, value := range u.timers {
		if value > 0 {
			u.timers[key] = value - 1
		}
	}
	u.stepLocked()
}

func (u *Underwater) startBubbleBurstLocked(verb string) {
	u.timers["bubble-burst"] = jitterInt(u.rng, u.intCfg("bubble_burst_dur"), 0.3)
	u.values["bubble_gain"] = u.cfg["bubble_burst_mult"] * (0.8 + u.rng.Float64()*0.45)
	u.appendLog("bubble-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, u.timers["bubble-burst"], u.values["bubble_gain"]))
}

func (u *Underwater) startCurrentShiftLocked(verb string) {
	u.timers["current-shift"] = jitterInt(u.rng, u.intCfg("current_shift_dur"), 0.3)
	u.timers["calm"] = 0
	sign := 1.0
	if u.rng.Float64() < 0.5 {
		sign = -1
	}
	u.values["current_push"] = sign * u.cfg["current_shift_push"] * (0.55 + u.rng.Float64()*0.55)
	u.appendLog("current-shift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, u.timers["current-shift"], u.values["current_push"]))
}

func (u *Underwater) startCalmLocked(verb string) {
	u.timers["calm"] = jitterInt(u.rng, u.intCfg("calm_dur"), 0.3)
	u.timers["current-shift"] = 0
	u.values["current_push"] = 0
	u.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, u.timers["calm"], u.cfg["calm_mult"]))
}

func (u *Underwater) startIntroLocked() {
	u.timers["bubble-burst"] = 0
	u.timers["current-shift"] = 0
	u.timers["calm"] = 0
	u.timers["ending"] = 0
	u.values["bubble_gain"] = 1
	u.values["current_push"] = 0
	u.timers["intro"] = u.intCfg("intro_dur")
	u.values["intro_total"] = float64(u.timers["intro"])
}

func (u *Underwater) startEndingLocked() {
	u.timers["intro"] = 0
	u.timers["bubble-burst"] = 0
	u.timers["current-shift"] = 0
	u.timers["calm"] = 0
	u.values["bubble_gain"] = 1
	u.values["current_push"] = 0
	endingTotal := u.intCfg("ending_dur") + max(0, u.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, u.intCfg("ending_dur"))
	}
	u.timers["ending"] = endingTotal
	u.values["ending_total"] = float64(endingTotal)
}

func (u *Underwater) stepLocked() {
	if u.timers["bubble-burst"] <= 0 {
		u.values["bubble_gain"] = 1
	}
	if u.timers["current-shift"] <= 0 {
		u.values["current_push"] = 0
	}
	if u.timers["intro"] <= 0 {
		delete(u.values, "intro_total")
	}
	if u.timers["ending"] <= 0 {
		delete(u.values, "ending_total")
	}
	if u.timers["intro"] > 0 || u.timers["ending"] > 0 {
		return
	}
	if u.timers["bubble-burst"] <= 0 && u.cfg["bubble_burst_p"] > 0 && u.rng.Float64() < u.cfg["bubble_burst_p"] {
		u.startBubbleBurstLocked("started")
	}
	if u.timers["current-shift"] <= 0 && u.timers["calm"] <= 0 && u.cfg["current_shift_p"] > 0 && u.rng.Float64() < u.cfg["current_shift_p"] {
		u.startCurrentShiftLocked("started")
	}
	if u.timers["calm"] <= 0 && u.timers["current-shift"] <= 0 && u.cfg["calm_p"] > 0 && u.rng.Float64() < u.cfg["calm_p"] {
		u.startCalmLocked("started")
	}
}
