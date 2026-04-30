package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type WindmillConfig = ProceduralConfig
type WindmillState = ProceduralState
type WindmillSnapshot = ProceduralSnapshot
type WindmillPersistedState = ProceduralPersistedState

// Windmill is a dedicated sim type for the windmill effect.
type Windmill struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    WindmillConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var windmillDefaultsLocal = WindmillConfig{
	"intro_dur":     45,
	"intro_turn":    0.12,
	"ending_dur":    60,
	"ending_linger": 20,
	"ending_turn":   0.05,
	"turn_speed":    0.08,
	"blade_len":     14.0,
	"blade_width":   1.8,
	"tower_height":  20.0,
	"tower_width":   6.0,
	"horizon":       0.72,
	"glow":          0.18,
	"hue":           28,
	"hue_sp":        18,
	"sat":           0.42,
	"lmin":          0.18,
	"lmax":          0.82,
	"gust_p":        0.0,
	"lull_p":        0.0,
	"gust_dur":      50,
	"gust_mult":     1.90,
	"lull_dur":      72,
	"lull_mult":     0.45,
}

func WindmillSchema() EffectSchema {
	return EffectSchema{
		Name: "windmill",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks spent easing the blades from stillness into a readable turn."},
			{Key: "intro_turn", Label: "intro turn", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Starting fraction of the final rotation speed before the mill settles into motion."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent coasting the blades back down toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks after the blades have mostly stopped."},
			{Key: "ending_turn", Label: "ending turn", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.01, Max: 0.35, Step: 0.01, Default: 0.05,
				Description: "Residual blade motion near the end of the outro."},
			{Key: "turn_speed", Label: "turn speed", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.02, Max: 0.25, Step: 0.01, Default: 0.08,
				Description: "Base blade rotation speed."},
			{Key: "blade_len", Label: "blade len", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 6, Max: 22, Step: 0.5, Default: 14,
				Description: "Length of the windmill blades."},
			{Key: "blade_width", Label: "blade width", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.8,
				Description: "Thickness of each blade arm."},
			{Key: "tower_height", Label: "tower height", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 10, Max: 30, Step: 0.5, Default: 20,
				Description: "Height of the windmill tower above the hill."},
			{Key: "tower_width", Label: "tower width", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 3, Max: 10, Step: 0.5, Default: 6,
				Description: "Width of the windmill tower silhouette."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.56, Max: 0.86, Step: 0.01, Default: 0.72,
				Description: "Height of the ground line and hill in frame."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Strength of the dusk haze and tiny warm window glow."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 10, Max: 240, Step: 1, Default: 28,
				Description: "Base sky hue spanning cool night blues through warm dusk."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 18,
				Description: "Variation between the upper sky and the horizon glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Overall sky and glow saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the upper sky and dark ground."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for the horizon and glow."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of the blades briefly spinning faster."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the wind settling into a slower turn."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50,
				Description: "Duration of a faster-turning gust."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Rotation multiplier applied during a gust."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the calmer slower-turning window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Rotation multiplier applied while lull is active."},
		},
	}
}

func defaultWindmillConfig() WindmillConfig { return cloneConfig(windmillDefaultsLocal) }

func mergeWindmillDefaults(cfg WindmillConfig) WindmillConfig {
	out := defaultWindmillConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = windmillDefaultsLocal["intro_dur"]
	}
	out["intro_turn"] = clamp01(out["intro_turn"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = windmillDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_turn"] = clamp01(out["ending_turn"])
	if out["turn_speed"] <= 0 {
		out["turn_speed"] = windmillDefaultsLocal["turn_speed"]
	}
	if out["blade_len"] <= 0 {
		out["blade_len"] = windmillDefaultsLocal["blade_len"]
	}
	if out["blade_width"] <= 0 {
		out["blade_width"] = windmillDefaultsLocal["blade_width"]
	}
	if out["tower_height"] <= 0 {
		out["tower_height"] = windmillDefaultsLocal["tower_height"]
	}
	if out["tower_width"] <= 0 {
		out["tower_width"] = windmillDefaultsLocal["tower_width"]
	}
	if out["horizon"] <= 0 {
		out["horizon"] = windmillDefaultsLocal["horizon"]
	}
	if out["glow"] <= 0 {
		out["glow"] = windmillDefaultsLocal["glow"]
	}
	if out["hue"] == 0 {
		out["hue"] = windmillDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = windmillDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = windmillDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = windmillDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = windmillDefaultsLocal["gust_dur"]
	}
	if out["gust_mult"] <= 0 {
		out["gust_mult"] = windmillDefaultsLocal["gust_mult"]
	}
	if out["lull_dur"] <= 0 {
		out["lull_dur"] = windmillDefaultsLocal["lull_dur"]
	}
	if out["lull_mult"] <= 0 {
		out["lull_mult"] = windmillDefaultsLocal["lull_mult"]
	}
	return out
}

func NewWindmill(w, h int, seed int64, cfg WindmillConfig) *Windmill {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Windmill{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeWindmillDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (w *Windmill) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if width == w.W && height == w.H {
		return
	}
	w.W = width
	w.H = height
	w.Grid = make([][]Pixel, height)
	for i := range w.Grid {
		w.Grid[i] = make([]Pixel, width)
	}
}

func (w *Windmill) SetConfig(cfg WindmillConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg = mergeWindmillDefaults(cfg)
}

func (w *Windmill) EffectiveConfig() WindmillConfig {
	w.mu.Lock()
	defer w.mu.Unlock()
	return cloneConfig(w.cfg)
}

func (w *Windmill) Snapshot() WindmillSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WindmillSnapshot{ProceduralState: w.snapshotStateLocked()}
}

func (w *Windmill) RestoreSnapshot(snap WindmillSnapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restoreStateLocked(snap.ProceduralState)
}

func (w *Windmill) SnapshotPersistedState() WindmillPersistedState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WindmillPersistedState{
		ProceduralState: w.snapshotStateLocked(),
		RNGState:        w.rng.State(),
	}
}

func (w *Windmill) RestorePersistedState(ps WindmillPersistedState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		w.rng.SetState(ps.RNGState)
	}
}

func (w *Windmill) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   w.tick,
		Timers: cloneTimerMap(w.timers),
		Values: cloneValueMap(w.values),
	}
}

func (w *Windmill) restoreStateLocked(state ProceduralState) {
	w.tick = state.Tick
	w.timers = cloneTimerMap(state.Timers)
	if w.timers == nil {
		w.timers = make(map[string]int)
	}
	w.values = cloneValueMap(state.Values)
	if w.values == nil {
		w.values = make(map[string]float64)
	}
}

func (w *Windmill) CurrentTick() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tick
}

func (w *Windmill) PerturbRNG(delta int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rng.Mix(delta)
}

func (w *Windmill) DrainLog() []LogEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.log) == 0 {
		return nil
	}
	out := w.log
	w.log = nil
	return out
}

func (w *Windmill) appendLog(kind, desc string) {
	w.log = append(w.log, LogEntry{Tick: w.tick, Type: kind, Desc: desc})
	if len(w.log) > 200 {
		w.log = w.log[len(w.log)-200:]
	}
}

func (w *Windmill) intCfg(key string) int {
	return int(math.Round(w.cfg[key]))
}

func (w *Windmill) TriggerEvent(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch name {
	case "gust":
		w.startGustLocked("triggered")
	case "lull":
		w.startLullLocked("triggered")
	case "intro":
		w.startIntroLocked()
		w.appendLog("intro", fmt.Sprintf("started (dur=%d, turn=%.2f)", w.timers["intro"], w.cfg["intro_turn"]))
	case "ending":
		w.startEndingLocked()
		w.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", w.intCfg("ending_dur"), w.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (w *Windmill) Step() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tick++
	for key, value := range w.timers {
		if value > 0 {
			w.timers[key] = value - 1
		}
	}
	w.stepLocked()
}

func (w *Windmill) startGustLocked(verb string) {
	w.timers["gust"] = jitterInt(w.rng, w.intCfg("gust_dur"), 0.3)
	w.values["gust_gain"] = w.cfg["gust_mult"] * (0.75 + w.rng.Float64()*0.45)
	w.appendLog("gust", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, w.timers["gust"], w.values["gust_gain"]))
}

func (w *Windmill) startLullLocked(verb string) {
	w.timers["lull"] = jitterInt(w.rng, w.intCfg("lull_dur"), 0.3)
	w.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, w.timers["lull"], w.cfg["lull_mult"]))
}

func (w *Windmill) startIntroLocked() {
	w.timers["gust"] = 0
	w.timers["lull"] = 0
	w.timers["ending"] = 0
	w.values["gust_gain"] = 1
	w.timers["intro"] = w.intCfg("intro_dur")
	w.values["intro_total"] = float64(w.timers["intro"])
}

func (w *Windmill) startEndingLocked() {
	w.timers["intro"] = 0
	w.timers["gust"] = 0
	w.timers["lull"] = 0
	w.values["gust_gain"] = 1
	endingTotal := w.intCfg("ending_dur") + max(0, w.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, w.intCfg("ending_dur"))
	}
	w.timers["ending"] = endingTotal
	w.values["ending_total"] = float64(endingTotal)
}

func (w *Windmill) stepLocked() {
	if w.timers["gust"] <= 0 {
		w.values["gust_gain"] = 1
	}
	if w.timers["intro"] <= 0 {
		delete(w.values, "intro_total")
	}
	if w.timers["ending"] <= 0 {
		delete(w.values, "ending_total")
	}
	if w.timers["intro"] > 0 || w.timers["ending"] > 0 {
		return
	}
	if w.timers["gust"] <= 0 && w.cfg["gust_p"] > 0 && w.rng.Float64() < w.cfg["gust_p"] {
		w.startGustLocked("started")
	}
	if w.timers["lull"] <= 0 && w.cfg["lull_p"] > 0 && w.rng.Float64() < w.cfg["lull_p"] {
		w.startLullLocked("started")
	}
}
