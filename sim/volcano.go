package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type VolcanoConfig = ProceduralConfig
type VolcanoState = ProceduralState
type VolcanoSnapshot = ProceduralSnapshot
type VolcanoPersistedState = ProceduralPersistedState

// Volcano is a dedicated sim type for the volcano effect.
type Volcano struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    VolcanoConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var volcanoDefaultsLocal = VolcanoConfig{
	"intro_dur":       55,
	"intro_glow":      0.16,
	"ending_dur":      70,
	"ending_linger":   22,
	"ending_glow":     0.10,
	"horizon":         0.86,
	"cone_height":     28.0,
	"cone_width":      46.0,
	"crater_width":    8.0,
	"slope_jitter":    1.6,
	"glow":            0.55,
	"smoke":           0.32,
	"smoke_height":    18.0,
	"hue":             18,
	"hue_sp":          16,
	"sat":             0.78,
	"lmin":            0.18,
	"lmax":            0.92,
	"eruption_p":      0.0,
	"smolder_p":       0.0,
	"flare_p":         0.0,
	"eruption_dur":    80,
	"eruption_height": 28.0,
	"eruption_mult":   2.4,
	"smolder_dur":     80,
	"smolder_mult":    0.55,
	"flare_dur":       24,
	"flare_mult":      1.85,
}

func VolcanoSchema() EffectSchema {
	return EffectSchema{
		Name: "volcano",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent kindling crater glow and smoke hints before pressure builds."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Starting fraction of the crater glow before the mountain settles into idle pressure."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering eruptions back toward a quiet simmer."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks for ash and embers to fall back to the cone."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Residual crater glow that remains near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 0.6, Max: 0.95, Step: 0.01, Default: 0.86,
				Description: "Where the mountain base sits in the frame."},
			{Key: "cone_height", Label: "cone height", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 12, Max: 44, Step: 1, Default: 28,
				Description: "Height of the volcano silhouette above the base."},
			{Key: "cone_width", Label: "cone width", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 22, Max: 70, Step: 1, Default: 46,
				Description: "Base width of the cone silhouette."},
			{Key: "crater_width", Label: "crater", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 4, Max: 18, Step: 0.5, Default: 8,
				Description: "Width of the glowing crater notch at the cone's summit."},
			{Key: "slope_jitter", Label: "slope rough", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 0, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Per-column jitter applied to the silhouette so the slopes read as rocky rather than perfect."},
			{Key: "glow", Label: "crater glow", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Strength of the warm glow rising out of the crater during idle."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.01, Default: 0.32,
				Description: "How much smoke continually rises from the crater during idle."},
			{Key: "smoke_height", Label: "smoke height", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 6, Max: 40, Step: 1, Default: 18,
				Description: "How far smoke trails rise above the crater before fading."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Base lava and crater hue. Lower values warm toward red; higher values lean orange."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 16,
				Description: "Variation across crater core, lava sparks, and smoke tinting."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Overall saturation of the lava and crater glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the cone silhouette and dim smoke."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Maximum lightness used for the brightest lava sparks at peak eruption."},
			{Key: "eruption_p", Label: "eruption", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "eruption",
				Description: "Per-tick chance of a full eruption blasting lava sparks and ash above the crater."},
			{Key: "smolder_p", Label: "smolder", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "smolder",
				Description: "Per-tick chance of the mountain settling into a quieter, dimmer simmer."},
			{Key: "flare_p", Label: "flare", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "flare",
				Description: "Per-tick chance of a brief crater flare without the full eruption arc."},
			{Key: "eruption_dur", Label: "eruption dur", Slot: SlotEventMod, Group: "eruption", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 80,
				Description: "Duration of an eruption in ticks (jittered by +/-30%)."},
			{Key: "eruption_height", Label: "eruption arc", Slot: SlotEventMod, Group: "eruption", Type: KnobFloat, Min: 8, Max: 60, Step: 1, Default: 28,
				Description: "How far above the crater lava sparks reach at peak eruption."},
			{Key: "eruption_mult", Label: "eruption x", Slot: SlotEventMod, Group: "eruption", Type: KnobFloat, Min: 1.1, Max: 4, Step: 0.05, Default: 2.4,
				Description: "Glow and spark multiplier applied while an eruption is active."},
			{Key: "smolder_dur", Label: "smolder dur", Slot: SlotEventMod, Group: "smolder", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80,
				Description: "Duration of the quieter smoldering window."},
			{Key: "smolder_mult", Label: "smolder x", Slot: SlotEventMod, Group: "smolder", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Glow and smoke multiplier applied while smolder is active."},
			{Key: "flare_dur", Label: "flare dur", Slot: SlotEventMod, Group: "flare", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 24,
				Description: "Duration of a brief crater flare."},
			{Key: "flare_mult", Label: "flare x", Slot: SlotEventMod, Group: "flare", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Glow multiplier applied during a flare."},
		},
	}
}

func defaultVolcanoConfig() VolcanoConfig { return cloneConfig(volcanoDefaultsLocal) }

func mergeVolcanoDefaults(cfg VolcanoConfig) VolcanoConfig {
	out := defaultVolcanoConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = volcanoDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = volcanoDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["horizon"] <= 0 {
		out["horizon"] = volcanoDefaultsLocal["horizon"]
	}
	if out["cone_height"] <= 0 {
		out["cone_height"] = volcanoDefaultsLocal["cone_height"]
	}
	if out["cone_width"] <= 0 {
		out["cone_width"] = volcanoDefaultsLocal["cone_width"]
	}
	if out["crater_width"] <= 0 {
		out["crater_width"] = volcanoDefaultsLocal["crater_width"]
	}
	if out["slope_jitter"] < 0 {
		out["slope_jitter"] = 0
	}
	if out["glow"] <= 0 {
		out["glow"] = volcanoDefaultsLocal["glow"]
	}
	if out["smoke"] < 0 {
		out["smoke"] = 0
	}
	if out["smoke_height"] <= 0 {
		out["smoke_height"] = volcanoDefaultsLocal["smoke_height"]
	}
	if out["hue"] < 0 {
		out["hue"] = volcanoDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = volcanoDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = volcanoDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = volcanoDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["eruption_dur"] <= 0 {
		out["eruption_dur"] = volcanoDefaultsLocal["eruption_dur"]
	}
	if out["eruption_height"] <= 0 {
		out["eruption_height"] = volcanoDefaultsLocal["eruption_height"]
	}
	if out["eruption_mult"] <= 0 {
		out["eruption_mult"] = volcanoDefaultsLocal["eruption_mult"]
	}
	if out["smolder_dur"] <= 0 {
		out["smolder_dur"] = volcanoDefaultsLocal["smolder_dur"]
	}
	if out["smolder_mult"] <= 0 {
		out["smolder_mult"] = volcanoDefaultsLocal["smolder_mult"]
	}
	if out["flare_dur"] <= 0 {
		out["flare_dur"] = volcanoDefaultsLocal["flare_dur"]
	}
	if out["flare_mult"] <= 0 {
		out["flare_mult"] = volcanoDefaultsLocal["flare_mult"]
	}
	return out
}

func NewVolcano(w, h int, seed int64, cfg VolcanoConfig) *Volcano {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Volcano{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeVolcanoDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (v *Volcano) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if w == v.W && h == v.H {
		return
	}
	v.W = w
	v.H = h
	v.Grid = make([][]Pixel, h)
	for i := range v.Grid {
		v.Grid[i] = make([]Pixel, w)
	}
}

func (v *Volcano) SetConfig(cfg VolcanoConfig) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cfg = mergeVolcanoDefaults(cfg)
}

func (v *Volcano) EffectiveConfig() VolcanoConfig {
	v.mu.Lock()
	defer v.mu.Unlock()
	return cloneConfig(v.cfg)
}

func (v *Volcano) Snapshot() VolcanoSnapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	return VolcanoSnapshot{
		ProceduralState: v.snapshotStateLocked(),
		RNGState:        v.rng.State(),
	}
}

func (v *Volcano) RestoreSnapshot(snap VolcanoSnapshot) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		v.rng.SetState(snap.RNGState)
	}
}

func (v *Volcano) SnapshotPersistedState() VolcanoPersistedState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return VolcanoPersistedState{
		ProceduralState: v.snapshotStateLocked(),
		RNGState:        v.rng.State(),
	}
}

func (v *Volcano) RestorePersistedState(ps VolcanoPersistedState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		v.rng.SetState(ps.RNGState)
	}
}

func (v *Volcano) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   v.tick,
		Timers: cloneTimerMap(v.timers),
		Values: cloneValueMap(v.values),
	}
}

func (v *Volcano) restoreStateLocked(state ProceduralState) {
	v.tick = state.Tick
	v.timers = cloneTimerMap(state.Timers)
	if v.timers == nil {
		v.timers = make(map[string]int)
	}
	v.values = cloneValueMap(state.Values)
	if v.values == nil {
		v.values = make(map[string]float64)
	}
}

func (v *Volcano) CurrentTick() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.tick
}

func (v *Volcano) PerturbRNG(delta int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.rng.Mix(delta)
}

func (v *Volcano) DrainLog() []LogEntry {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.log) == 0 {
		return nil
	}
	out := v.log
	v.log = nil
	return out
}

func (v *Volcano) appendLog(kind, desc string) {
	v.log = append(v.log, LogEntry{Tick: v.tick, Type: kind, Desc: desc})
	if len(v.log) > 200 {
		v.log = v.log[len(v.log)-200:]
	}
}

func (v *Volcano) intCfg(key string) int {
	return int(math.Round(v.cfg[key]))
}

func (v *Volcano) TriggerEvent(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch name {
	case "eruption":
		v.startEruptionLocked("triggered")
	case "smolder":
		v.startSmolderLocked("triggered")
	case "flare":
		v.startFlareLocked("triggered")
	case "intro":
		v.startIntroLocked()
		v.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", v.timers["intro"], v.cfg["intro_glow"]))
	case "ending":
		v.startEndingLocked()
		v.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", v.intCfg("ending_dur"), v.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (v *Volcano) Step() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.tick++
	for key, value := range v.timers {
		if value > 0 {
			v.timers[key] = value - 1
		}
	}
	v.stepLocked()
}

func (v *Volcano) startEruptionLocked(verb string) {
	v.timers["eruption"] = jitterInt(v.rng, v.intCfg("eruption_dur"), 0.3)
	v.timers["smolder"] = 0
	v.values["eruption_gain"] = v.cfg["eruption_mult"] * (0.8 + v.rng.Float64()*0.45)
	v.values["eruption_seed"] = v.rng.Float64() * 1024
	v.appendLog("eruption", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, v.timers["eruption"], v.values["eruption_gain"]))
}

func (v *Volcano) startSmolderLocked(verb string) {
	v.timers["smolder"] = jitterInt(v.rng, v.intCfg("smolder_dur"), 0.3)
	v.timers["eruption"] = 0
	v.values["eruption_gain"] = 1
	v.appendLog("smolder", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, v.timers["smolder"], v.cfg["smolder_mult"]))
}

func (v *Volcano) startFlareLocked(verb string) {
	v.timers["flare"] = jitterInt(v.rng, v.intCfg("flare_dur"), 0.3)
	v.values["flare_gain"] = v.cfg["flare_mult"] * (0.85 + v.rng.Float64()*0.3)
	v.appendLog("flare", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, v.timers["flare"], v.values["flare_gain"]))
}

func (v *Volcano) startIntroLocked() {
	v.timers["eruption"] = 0
	v.timers["smolder"] = 0
	v.timers["flare"] = 0
	v.timers["ending"] = 0
	v.values["eruption_gain"] = 1
	v.values["flare_gain"] = 1
	v.timers["intro"] = v.intCfg("intro_dur")
	v.values["intro_total"] = float64(v.timers["intro"])
}

func (v *Volcano) startEndingLocked() {
	v.timers["intro"] = 0
	v.timers["eruption"] = 0
	v.timers["smolder"] = 0
	v.timers["flare"] = 0
	v.values["eruption_gain"] = 1
	v.values["flare_gain"] = 1
	endingTotal := v.intCfg("ending_dur") + max(0, v.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, v.intCfg("ending_dur"))
	}
	v.timers["ending"] = endingTotal
	v.values["ending_total"] = float64(endingTotal)
}

func (v *Volcano) stepLocked() {
	if v.timers["eruption"] <= 0 {
		v.values["eruption_gain"] = 1
	}
	if v.timers["flare"] <= 0 {
		v.values["flare_gain"] = 1
	}
	if v.timers["intro"] <= 0 {
		delete(v.values, "intro_total")
	}
	if v.timers["ending"] <= 0 {
		delete(v.values, "ending_total")
	}
	if v.timers["intro"] > 0 || v.timers["ending"] > 0 {
		return
	}
	if v.timers["eruption"] <= 0 && v.timers["smolder"] <= 0 && v.cfg["eruption_p"] > 0 && v.rng.Float64() < v.cfg["eruption_p"] {
		v.startEruptionLocked("started")
	}
	if v.timers["smolder"] <= 0 && v.timers["eruption"] <= 0 && v.cfg["smolder_p"] > 0 && v.rng.Float64() < v.cfg["smolder_p"] {
		v.startSmolderLocked("started")
	}
	if v.timers["flare"] <= 0 && v.cfg["flare_p"] > 0 && v.rng.Float64() < v.cfg["flare_p"] {
		v.startFlareLocked("started")
	}
}
