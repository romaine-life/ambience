package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type RowboatConfig = ProceduralConfig
type RowboatState = ProceduralState
type RowboatSnapshot = ProceduralSnapshot
type RowboatPersistedState = ProceduralPersistedState

// Rowboat is a dedicated sim type for the rowboat effect.
type Rowboat struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    RowboatConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var rowboatDefaultsLocal = RowboatConfig{
	"intro_dur":     50,
	"intro_drift":   0.18,
	"ending_dur":    65,
	"ending_linger": 18,
	"ending_ripple": 0.08,
	"waterline":     0.58,
	"drift_speed":   0.08,
	"bob_amp":       1.20,
	"wave_amp":      1.60,
	"wave_freq":     0.16,
	"ripple":        0.24,
	"reflection":    0.22,
	"boat_len":      14.0,
	"boat_height":   3.5,
	"hue":           206,
	"hue_sp":        16,
	"sat":           0.36,
	"lmin":          0.16,
	"lmax":          0.82,
	"wake_p":        0.0,
	"drift_p":       0.0,
	"calm_p":        0.0,
	"wake_dur":      40,
	"wake_mult":     1.85,
	"drift_dur":     58,
	"drift_push":    1.30,
	"calm_dur":      72,
	"calm_mult":     0.50,
}

func RowboatSchema() EffectSchema {
	return EffectSchema{
		Name: "rowboat",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent easing the boat and its first ripples into view."},
			{Key: "intro_drift", Label: "intro drift", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Starting fraction of the final drift and bob motion before the scene settles."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent flattening the ripples and easing the boat toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 18,
				Description: "Extra quiet ticks after the visible motion has mostly faded."},
			{Key: "ending_ripple", Label: "ending ripple", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual motion and ripple presence near the end of the outro."},
			{Key: "waterline", Label: "waterline", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.42, Max: 0.76, Step: 0.01, Default: 0.58,
				Description: "Height of the lake surface in the frame."},
			{Key: "drift_speed", Label: "drift speed", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.02, Max: 0.2, Step: 0.01, Default: 0.08,
				Description: "How quickly the boat drifts and the wave phase rolls underneath it."},
			{Key: "bob_amp", Label: "bob amp", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.1, Default: 1.2,
				Description: "Vertical bobbing amplitude of the hull."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.2, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Amplitude of the wider surface undulation behind the boat."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.35, Step: 0.01, Default: 0.16,
				Description: "Horizontal frequency of the lake surface wiggle."},
			{Key: "ripple", Label: "ripple", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.24,
				Description: "Strength and number of the local ripples around the boat."},
			{Key: "reflection", Label: "reflection", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Visibility of the boat and ripple reflection in the water."},
			{Key: "boat_len", Label: "boat len", Slot: SlotLever, Group: "boat", Type: KnobFloat, Min: 6, Max: 24, Step: 0.5, Default: 14,
				Description: "Length of the rowboat hull."},
			{Key: "boat_height", Label: "boat height", Slot: SlotLever, Group: "boat", Type: KnobFloat, Min: 1, Max: 8, Step: 0.5, Default: 3.5,
				Description: "Height of the rowboat silhouette above the waterline."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 230, Step: 1, Default: 206,
				Description: "Base water and sky hue. Lower values lean teal; higher values lean deeper blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 24, Step: 1, Default: 16,
				Description: "Variation between the upper sky, water, and reflected highlights."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.36,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.08, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Minimum lightness used for darker water and hull shadow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.35, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for sky glow and water highlights."},
			{Key: "wake_p", Label: "wake", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wake",
				Description: "Per-tick chance of a more pronounced wake rippling out behind the boat."},
			{Key: "drift_p", Label: "drift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "drift",
				Description: "Per-tick chance of the boat being pushed gently farther to one side."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the lake briefly flattening into calmer motion."},
			{Key: "wake_dur", Label: "wake dur", Slot: SlotEventMod, Group: "wake", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 40,
				Description: "Duration of the more pronounced wake window."},
			{Key: "wake_mult", Label: "wake x", Slot: SlotEventMod, Group: "wake", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Ripple multiplier applied while wake is active."},
			{Key: "drift_dur", Label: "drift dur", Slot: SlotEventMod, Group: "drift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "Duration of the stronger side drift window."},
			{Key: "drift_push", Label: "drift push", Slot: SlotEventMod, Group: "drift", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.3,
				Description: "Additional sideways push applied during a drift event."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the calmer low-motion interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.5,
				Description: "Motion and ripple multiplier applied while calm is active."},
		},
	}
}

func defaultRowboatConfig() RowboatConfig { return cloneConfig(rowboatDefaultsLocal) }

func mergeRowboatDefaults(cfg RowboatConfig) RowboatConfig {
	out := defaultRowboatConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = rowboatDefaultsLocal["intro_dur"]
	}
	out["intro_drift"] = clamp01(out["intro_drift"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = rowboatDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_ripple"] = clamp01(out["ending_ripple"])
	if out["waterline"] <= 0 {
		out["waterline"] = rowboatDefaultsLocal["waterline"]
	}
	if out["drift_speed"] <= 0 {
		out["drift_speed"] = rowboatDefaultsLocal["drift_speed"]
	}
	if out["bob_amp"] <= 0 {
		out["bob_amp"] = rowboatDefaultsLocal["bob_amp"]
	}
	if out["wave_amp"] <= 0 {
		out["wave_amp"] = rowboatDefaultsLocal["wave_amp"]
	}
	if out["wave_freq"] <= 0 {
		out["wave_freq"] = rowboatDefaultsLocal["wave_freq"]
	}
	if out["ripple"] <= 0 {
		out["ripple"] = rowboatDefaultsLocal["ripple"]
	}
	if out["reflection"] <= 0 {
		out["reflection"] = rowboatDefaultsLocal["reflection"]
	}
	if out["boat_len"] <= 0 {
		out["boat_len"] = rowboatDefaultsLocal["boat_len"]
	}
	if out["boat_height"] <= 0 {
		out["boat_height"] = rowboatDefaultsLocal["boat_height"]
	}
	if out["hue"] == 0 {
		out["hue"] = rowboatDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = rowboatDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = rowboatDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = rowboatDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["wake_dur"] <= 0 {
		out["wake_dur"] = rowboatDefaultsLocal["wake_dur"]
	}
	if out["wake_mult"] <= 0 {
		out["wake_mult"] = rowboatDefaultsLocal["wake_mult"]
	}
	if out["drift_dur"] <= 0 {
		out["drift_dur"] = rowboatDefaultsLocal["drift_dur"]
	}
	if out["drift_push"] <= 0 {
		out["drift_push"] = rowboatDefaultsLocal["drift_push"]
	}
	if out["calm_dur"] <= 0 {
		out["calm_dur"] = rowboatDefaultsLocal["calm_dur"]
	}
	if out["calm_mult"] <= 0 {
		out["calm_mult"] = rowboatDefaultsLocal["calm_mult"]
	}
	return out
}

func NewRowboat(w, h int, seed int64, cfg RowboatConfig) *Rowboat {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Rowboat{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeRowboatDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (r *Rowboat) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if w == r.W && h == r.H {
		return
	}
	r.W = w
	r.H = h
	r.Grid = make([][]Pixel, h)
	for i := range r.Grid {
		r.Grid[i] = make([]Pixel, w)
	}
}

func (r *Rowboat) SetConfig(cfg RowboatConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = mergeRowboatDefaults(cfg)
}

func (r *Rowboat) EffectiveConfig() RowboatConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneConfig(r.cfg)
}

func (r *Rowboat) Snapshot() RowboatSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RowboatSnapshot{
		ProceduralState: r.snapshotStateLocked(),
		RNGState:        r.rng.State(),
	}
}

func (r *Rowboat) RestoreSnapshot(snap RowboatSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		r.rng.SetState(snap.RNGState)
	}
}

func (r *Rowboat) SnapshotPersistedState() RowboatPersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RowboatPersistedState{
		ProceduralState: r.snapshotStateLocked(),
		RNGState:        r.rng.State(),
	}
}

func (r *Rowboat) RestorePersistedState(ps RowboatPersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		r.rng.SetState(ps.RNGState)
	}
}

func (r *Rowboat) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   r.tick,
		Timers: cloneTimerMap(r.timers),
		Values: cloneValueMap(r.values),
	}
}

func (r *Rowboat) restoreStateLocked(state ProceduralState) {
	r.tick = state.Tick
	r.timers = cloneTimerMap(state.Timers)
	if r.timers == nil {
		r.timers = make(map[string]int)
	}
	r.values = cloneValueMap(state.Values)
	if r.values == nil {
		r.values = make(map[string]float64)
	}
}

func (r *Rowboat) CurrentTick() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tick
}

func (r *Rowboat) PerturbRNG(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Mix(delta)
}

func (r *Rowboat) DrainLog() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) == 0 {
		return nil
	}
	out := r.log
	r.log = nil
	return out
}

func (r *Rowboat) appendLog(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

func (r *Rowboat) intCfg(key string) int {
	return int(math.Round(r.cfg[key]))
}

func (r *Rowboat) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "wake":
		r.startWakeLocked("triggered")
	case "drift":
		r.startDriftLocked("triggered")
	case "calm":
		r.startCalmLocked("triggered")
	case "intro":
		r.startIntroLocked()
		r.appendLog("intro", fmt.Sprintf("started (dur=%d, drift=%.2f)", r.timers["intro"], r.cfg["intro_drift"]))
	case "ending":
		r.startEndingLocked()
		r.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", r.intCfg("ending_dur"), r.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (r *Rowboat) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++
	for key, value := range r.timers {
		if value > 0 {
			r.timers[key] = value - 1
		}
	}
	r.stepLocked()
}

func (r *Rowboat) startWakeLocked(verb string) {
	r.timers["wake"] = jitterInt(r.rng, r.intCfg("wake_dur"), 0.3)
	r.values["wake_gain"] = r.cfg["wake_mult"] * (0.8 + r.rng.Float64()*0.45)
	r.appendLog("wake", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, r.timers["wake"], r.values["wake_gain"]))
}

func (r *Rowboat) startDriftLocked(verb string) {
	r.timers["drift"] = jitterInt(r.rng, r.intCfg("drift_dur"), 0.3)
	sign := 1.0
	if r.rng.Float64() < 0.5 {
		sign = -1
	}
	r.values["drift_push"] = sign * r.cfg["drift_push"] * (0.65 + r.rng.Float64()*0.55)
	r.appendLog("drift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, r.timers["drift"], r.values["drift_push"]))
}

func (r *Rowboat) startCalmLocked(verb string) {
	r.timers["calm"] = jitterInt(r.rng, r.intCfg("calm_dur"), 0.3)
	r.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, r.timers["calm"], r.cfg["calm_mult"]))
}

func (r *Rowboat) startIntroLocked() {
	r.timers["wake"] = 0
	r.timers["drift"] = 0
	r.timers["calm"] = 0
	r.timers["ending"] = 0
	r.values["wake_gain"] = 1
	r.values["drift_push"] = 0
	r.timers["intro"] = r.intCfg("intro_dur")
	r.values["intro_total"] = float64(r.timers["intro"])
}

func (r *Rowboat) startEndingLocked() {
	r.timers["intro"] = 0
	r.timers["wake"] = 0
	r.timers["drift"] = 0
	r.timers["calm"] = 0
	r.values["wake_gain"] = 1
	r.values["drift_push"] = 0
	endingTotal := r.intCfg("ending_dur") + max(0, r.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, r.intCfg("ending_dur"))
	}
	r.timers["ending"] = endingTotal
	r.values["ending_total"] = float64(endingTotal)
}

func (r *Rowboat) stepLocked() {
	if r.timers["wake"] <= 0 {
		r.values["wake_gain"] = 1
	}
	if r.timers["drift"] <= 0 {
		r.values["drift_push"] = 0
	}
	if r.timers["intro"] <= 0 {
		delete(r.values, "intro_total")
	}
	if r.timers["ending"] <= 0 {
		delete(r.values, "ending_total")
	}
	if r.timers["intro"] > 0 || r.timers["ending"] > 0 {
		return
	}
	if r.timers["wake"] <= 0 && r.cfg["wake_p"] > 0 && r.rng.Float64() < r.cfg["wake_p"] {
		r.startWakeLocked("started")
	}
	if r.timers["drift"] <= 0 && r.cfg["drift_p"] > 0 && r.rng.Float64() < r.cfg["drift_p"] {
		r.startDriftLocked("started")
	}
	if r.timers["calm"] <= 0 && r.cfg["calm_p"] > 0 && r.rng.Float64() < r.cfg["calm_p"] {
		r.startCalmLocked("started")
	}
}
