package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// AutumnLeavesConfig keeps the lightweight ProceduralConfig shape so the
// browser-first prototype doesn't need a typed-struct cleanup pass yet.
type AutumnLeavesConfig = ProceduralConfig

type AutumnLeavesState = ProceduralState
type AutumnLeavesSnapshot = ProceduralSnapshot
type AutumnLeavesPersistedState = ProceduralPersistedState

// AutumnLeaves is a dedicated sim type for the autumn-leaves effect,
// replacing the kind="autumn-leaves" branch of the old Procedural type.
type AutumnLeaves struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    AutumnLeavesConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var autumnLeavesDefaultsLocal = AutumnLeavesConfig{
	"intro_dur":      55,
	"intro_density":  0.12,
	"ending_dur":     60,
	"ending_linger":  18,
	"ending_density": 0.04,
	"density":        0.24,
	"speed":          0.44,
	"drift":          0.18,
	"sway":           0.86,
	"layers":         2,
	"size":           1.2,
	"hue":            28,
	"hue_sp":         24,
	"sat":            0.62,
	"lmin":           0.38,
	"lmax":           0.78,
	"gust_p":         0.0,
	"lull_p":         0.0,
	"swirl_p":        0.0,
	"gust_dur":       48,
	"gust_mult":      1.9,
	"lull_dur":       72,
	"lull_mult":      0.35,
	"swirl_dur":      52,
	"swirl_pull":     1.15,
}

func AutumnLeavesSchema() EffectSchema {
	return EffectSchema{
		Name: "autumn-leaves",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent building from a few drifting leaves into the full fall."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.02, Default: 0.12,
				Description: "Starting fraction of the full leaf field before the fall settles in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent tapering the leaf detachments back toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 18,
				Description: "Extra quiet ticks for the last airborne leaves to settle out."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.02, Default: 0.04,
				Description: "How much low-level drift remains at the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.05, Max: 0.75, Step: 0.01, Default: 0.24,
				Description: "Base number of drifting leaves across the field."},
			{Key: "speed", Label: "fall speed", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.44,
				Description: "How quickly leaves drop through the scene."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: -0.8, Max: 0.8, Step: 0.01, Default: 0.18,
				Description: "Baseline sideways carry applied to the leaf field."},
			{Key: "sway", Label: "flutter", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.8, Step: 0.02, Default: 0.86,
				Description: "How much the leaves wobble and flutter on the way down."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "fall", Type: KnobInt, Min: 1, Max: 3, Step: 1, Default: 2,
				Description: "Number of leaf depth layers."},
			{Key: "size", Label: "leaf size", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.5, Max: 3, Step: 0.1, Default: 1.2,
				Description: "Pixel size of the nearer leaf blocks."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 70, Step: 1, Default: 28,
				Description: "Base leaf hue. Lower values warm toward red; higher values lean gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 50, Step: 1, Default: 24,
				Description: "Variation in leaf color across the field."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.62,
				Description: "Overall leaf saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.8, Step: 0.01, Default: 0.38,
				Description: "Minimum lightness used for distant leaves and background tones."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for the brightest near leaves."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a stronger wind push across the leaf field."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the leaf fall thinning into a quieter stretch."},
			{Key: "swirl_p", Label: "swirl", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "swirl",
				Description: "Per-tick chance of a circular eddy tugging leaves into a swirl."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 48,
				Description: "Typical gust duration in ticks (jittered by +/-30%)."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.9,
				Description: "How strongly a gust bends the leaf field sideways."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the lower-density lull window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Density multiplier applied while lull is active."},
			{Key: "swirl_dur", Label: "swirl dur", Slot: SlotEventMod, Group: "swirl", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 52,
				Description: "How long the swirl eddy stays active."},
			{Key: "swirl_pull", Label: "swirl pull", Slot: SlotEventMod, Group: "swirl", Type: KnobFloat, Min: 0.1, Max: 2.5, Step: 0.05, Default: 1.15,
				Description: "Strength of the circular pull during a swirl event."},
		},
	}
}

func defaultAutumnLeavesConfig() AutumnLeavesConfig { return cloneConfig(autumnLeavesDefaultsLocal) }

func mergeAutumnLeavesDefaults(cfg AutumnLeavesConfig) AutumnLeavesConfig {
	out := defaultAutumnLeavesConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = autumnLeavesDefaultsLocal["intro_dur"]
	}
	out["intro_density"] = clamp01(out["intro_density"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = autumnLeavesDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_density"] = clamp01(out["ending_density"])
	if out["density"] <= 0 {
		out["density"] = autumnLeavesDefaultsLocal["density"]
	}
	if out["speed"] <= 0 {
		out["speed"] = autumnLeavesDefaultsLocal["speed"]
	}
	if out["layers"] < 1 {
		out["layers"] = autumnLeavesDefaultsLocal["layers"]
	}
	if out["size"] <= 0 {
		out["size"] = autumnLeavesDefaultsLocal["size"]
	}
	if out["hue"] == 0 {
		out["hue"] = autumnLeavesDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = autumnLeavesDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = autumnLeavesDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = autumnLeavesDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = autumnLeavesDefaultsLocal["gust_dur"]
	}
	if out["gust_mult"] <= 0 {
		out["gust_mult"] = autumnLeavesDefaultsLocal["gust_mult"]
	}
	if out["lull_dur"] <= 0 {
		out["lull_dur"] = autumnLeavesDefaultsLocal["lull_dur"]
	}
	if out["lull_mult"] <= 0 {
		out["lull_mult"] = autumnLeavesDefaultsLocal["lull_mult"]
	}
	if out["swirl_dur"] <= 0 {
		out["swirl_dur"] = autumnLeavesDefaultsLocal["swirl_dur"]
	}
	if out["swirl_pull"] <= 0 {
		out["swirl_pull"] = autumnLeavesDefaultsLocal["swirl_pull"]
	}
	return out
}

// NewAutumnLeaves builds an AutumnLeaves instance.
func NewAutumnLeaves(w, h int, seed int64, cfg AutumnLeavesConfig) *AutumnLeaves {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &AutumnLeaves{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeAutumnLeavesDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (a *AutumnLeaves) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if w == a.W && h == a.H {
		return
	}
	a.W = w
	a.H = h
	a.Grid = make([][]Pixel, h)
	for i := range a.Grid {
		a.Grid[i] = make([]Pixel, w)
	}
}

func (a *AutumnLeaves) SetConfig(cfg AutumnLeavesConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = mergeAutumnLeavesDefaults(cfg)
}

func (a *AutumnLeaves) EffectiveConfig() AutumnLeavesConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneConfig(a.cfg)
}

func (a *AutumnLeaves) Snapshot() AutumnLeavesSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AutumnLeavesSnapshot{
		ProceduralState: a.snapshotStateLocked(),
		RNGState:        a.rng.State(),
	}
}

func (a *AutumnLeaves) RestoreSnapshot(snap AutumnLeavesSnapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		a.rng.SetState(snap.RNGState)
	}
}

func (a *AutumnLeaves) SnapshotPersistedState() AutumnLeavesPersistedState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AutumnLeavesPersistedState{
		ProceduralState: a.snapshotStateLocked(),
		RNGState:        a.rng.State(),
	}
}

func (a *AutumnLeaves) RestorePersistedState(ps AutumnLeavesPersistedState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		a.rng.SetState(ps.RNGState)
	}
}

func (a *AutumnLeaves) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   a.tick,
		Timers: cloneTimerMap(a.timers),
		Values: cloneValueMap(a.values),
	}
}

func (a *AutumnLeaves) restoreStateLocked(state ProceduralState) {
	a.tick = state.Tick
	a.timers = cloneTimerMap(state.Timers)
	if a.timers == nil {
		a.timers = make(map[string]int)
	}
	a.values = cloneValueMap(state.Values)
	if a.values == nil {
		a.values = make(map[string]float64)
	}
}

func (a *AutumnLeaves) CurrentTick() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tick
}

func (a *AutumnLeaves) PerturbRNG(delta int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rng.Mix(delta)
}

func (a *AutumnLeaves) DrainLog() []LogEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.log) == 0 {
		return nil
	}
	out := a.log
	a.log = nil
	return out
}

func (a *AutumnLeaves) appendLog(kind, desc string) {
	a.log = append(a.log, LogEntry{Tick: a.tick, Type: kind, Desc: desc})
	if len(a.log) > 200 {
		a.log = a.log[len(a.log)-200:]
	}
}

func (a *AutumnLeaves) intCfg(key string) int {
	return int(math.Round(a.cfg[key]))
}

func (a *AutumnLeaves) TriggerEvent(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch name {
	case "gust":
		a.startGustLocked("triggered")
	case "lull":
		a.startLullLocked("triggered")
	case "swirl":
		a.startSwirlLocked("triggered")
	case "intro":
		a.startIntroLocked()
		a.appendLog("intro", fmt.Sprintf("started (dur=%d, density=%.2f)", a.timers["intro"], a.cfg["intro_density"]))
	case "ending":
		a.startEndingLocked()
		a.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", a.intCfg("ending_dur"), a.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (a *AutumnLeaves) Step() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tick++
	for key, value := range a.timers {
		if value > 0 {
			a.timers[key] = value - 1
		}
	}
	a.stepLocked()
}

func (a *AutumnLeaves) startGustLocked(verb string) {
	a.timers["gust"] = jitterInt(a.rng, a.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if a.rng.Float64() < 0.5 {
		sign = -1
	}
	a.values["gust_push"] = sign * a.cfg["gust_mult"] * (0.5 + a.rng.Float64()*0.7)
	a.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, a.timers["gust"], a.values["gust_push"]))
}

func (a *AutumnLeaves) startLullLocked(verb string) {
	a.timers["lull"] = jitterInt(a.rng, a.intCfg("lull_dur"), 0.3)
	a.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, a.timers["lull"], a.cfg["lull_mult"]))
}

func (a *AutumnLeaves) startSwirlLocked(verb string) {
	a.timers["swirl"] = jitterInt(a.rng, a.intCfg("swirl_dur"), 0.3)
	sign := 1.0
	if a.rng.Float64() < 0.5 {
		sign = -1
	}
	a.values["swirl_spin"] = sign * a.cfg["swirl_pull"] * (0.65 + a.rng.Float64()*0.45)
	a.values["swirl_row"] = float64(max(8, a.H/3)) + a.rng.Float64()*float64(max(1, a.H/2))
	a.values["swirl_col"] = a.rng.Float64() * float64(max(1, a.W))
	a.appendLog("swirl", fmt.Sprintf("%s (dur=%d, pull=%+.2f)", verb, a.timers["swirl"], a.values["swirl_spin"]))
}

func (a *AutumnLeaves) startIntroLocked() {
	a.timers["gust"] = 0
	a.timers["lull"] = 0
	a.timers["swirl"] = 0
	a.timers["ending"] = 0
	a.values["gust_push"] = 0
	a.values["swirl_spin"] = 0
	a.timers["intro"] = a.intCfg("intro_dur")
	a.values["intro_total"] = float64(a.timers["intro"])
}

func (a *AutumnLeaves) startEndingLocked() {
	a.timers["intro"] = 0
	a.timers["gust"] = 0
	a.timers["lull"] = 0
	a.timers["swirl"] = 0
	a.values["gust_push"] = 0
	a.values["swirl_spin"] = 0
	endingTotal := a.intCfg("ending_dur") + max(0, a.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, a.intCfg("ending_dur"))
	}
	a.timers["ending"] = endingTotal
	a.values["ending_total"] = float64(endingTotal)
}

func (a *AutumnLeaves) stepLocked() {
	if a.timers["gust"] <= 0 {
		a.values["gust_push"] = 0
	}
	if a.timers["swirl"] <= 0 {
		a.values["swirl_spin"] = 0
	}
	if a.timers["intro"] <= 0 {
		delete(a.values, "intro_total")
	}
	if a.timers["ending"] <= 0 {
		delete(a.values, "ending_total")
	}
	if a.timers["intro"] > 0 || a.timers["ending"] > 0 {
		return
	}
	if a.timers["gust"] <= 0 && a.cfg["gust_p"] > 0 && a.rng.Float64() < a.cfg["gust_p"] {
		a.startGustLocked("started")
	}
	if a.timers["lull"] <= 0 && a.cfg["lull_p"] > 0 && a.rng.Float64() < a.cfg["lull_p"] {
		a.startLullLocked("started")
	}
	if a.timers["swirl"] <= 0 && a.cfg["swirl_p"] > 0 && a.rng.Float64() < a.cfg["swirl_p"] {
		a.startSwirlLocked("started")
	}
}
