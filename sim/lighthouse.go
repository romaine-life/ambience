package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type LighthouseConfig = ProceduralConfig
type LighthouseState = ProceduralState
type LighthouseSnapshot = ProceduralSnapshot
type LighthousePersistedState = ProceduralPersistedState

// Lighthouse is a dedicated sim type for the lighthouse effect.
type Lighthouse struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    LighthouseConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var lighthouseDefaultsLocal = LighthouseConfig{
	"intro_dur":        50,
	"intro_beam":       0.16,
	"ending_dur":       65,
	"ending_linger":    18,
	"ending_beam":      0.08,
	"sweep_speed":      0.08,
	"beam_width":       0.22,
	"beam_softness":    0.42,
	"tower_height":     22.0,
	"tower_width":      6.5,
	"horizon":          0.74,
	"haze":             0.14,
	"glow":             0.22,
	"hue":              214,
	"hue_sp":           18,
	"sat":              0.34,
	"lmin":             0.12,
	"lmax":             0.84,
	"bright_pass_p":    0.0,
	"fog_thicken_p":    0.0,
	"calm_p":           0.0,
	"bright_pass_dur":  42,
	"bright_pass_mult": 1.75,
	"fog_thicken_dur":  72,
	"fog_thicken_mult": 1.85,
	"calm_dur":         64,
	"calm_mult":        0.55,
}

func LighthouseSchema() EffectSchema {
	return EffectSchema{
		Name: "lighthouse",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent bringing the first sweep up from a dim narrow beam."},
			{Key: "intro_beam", Label: "intro beam", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Starting fraction of the full beam presence before the lighthouse settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent fading the sweep down toward darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 18,
				Description: "Extra quiet ticks after the beam has mostly faded."},
			{Key: "ending_beam", Label: "ending beam", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual beam presence near the end of the outro."},
			{Key: "sweep_speed", Label: "sweep speed", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.2, Step: 0.01, Default: 0.08,
				Description: "How quickly the beam sweeps back across the scene."},
			{Key: "beam_width", Label: "beam width", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.06, Max: 0.45, Step: 0.01, Default: 0.22,
				Description: "Angular width of the sweeping light wedge."},
			{Key: "beam_softness", Label: "beam soft", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.05, Default: 0.42,
				Description: "Soft falloff from the beam core into surrounding haze."},
			{Key: "tower_height", Label: "tower height", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 10, Max: 30, Step: 0.5, Default: 22,
				Description: "Height of the lighthouse silhouette above the horizon."},
			{Key: "tower_width", Label: "tower width", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 3, Max: 10, Step: 0.5, Default: 6.5,
				Description: "Width of the lighthouse tower silhouette."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 0.56, Max: 0.86, Step: 0.01, Default: 0.74,
				Description: "Height of the horizon and coastline in frame."},
			{Key: "haze", Label: "haze", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Base atmospheric haze around the beam and horizon."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.22,
				Description: "Strength of the lamp glow near the tower head."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 240, Step: 1, Default: 214,
				Description: "Base night-sky hue. Lower values lean teal; higher values lean deeper blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between upper sky, beam haze, and horizon glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.34,
				Description: "Overall scene saturation for sky and beam haze."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Minimum lightness used for the darkest sky and sea areas."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.84,
				Description: "Maximum lightness used for the beam and horizon glow."},
			{Key: "bright_pass_p", Label: "bright pass", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "bright-pass",
				Description: "Per-tick chance of a brighter beam pass cutting across the scene."},
			{Key: "fog_thicken_p", Label: "fog thicken", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "fog-thicken",
				Description: "Per-tick chance of the air thickening into a hazier softer sweep."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter lower-intensity interval between brighter passes."},
			{Key: "bright_pass_dur", Label: "bright dur", Slot: SlotEventMod, Group: "bright-pass", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 42,
				Description: "Duration of a brighter beam pass."},
			{Key: "bright_pass_mult", Label: "bright x", Slot: SlotEventMod, Group: "bright-pass", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.75,
				Description: "Brightness multiplier applied during a bright pass."},
			{Key: "fog_thicken_dur", Label: "fog dur", Slot: SlotEventMod, Group: "fog-thicken", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the thicker fog window."},
			{Key: "fog_thicken_mult", Label: "fog x", Slot: SlotEventMod, Group: "fog-thicken", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Haze multiplier applied while fog thickening is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 64,
				Description: "Duration of the quieter lower-beam interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Beam intensity multiplier applied while calm is active."},
		},
	}
}

func defaultLighthouseConfig() LighthouseConfig { return cloneConfig(lighthouseDefaultsLocal) }

func mergeLighthouseDefaults(cfg LighthouseConfig) LighthouseConfig {
	out := defaultLighthouseConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = lighthouseDefaultsLocal["intro_dur"]
	}
	out["intro_beam"] = clamp01(out["intro_beam"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = lighthouseDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_beam"] = clamp01(out["ending_beam"])
	if out["sweep_speed"] <= 0 {
		out["sweep_speed"] = lighthouseDefaultsLocal["sweep_speed"]
	}
	if out["beam_width"] <= 0 {
		out["beam_width"] = lighthouseDefaultsLocal["beam_width"]
	}
	if out["beam_softness"] <= 0 {
		out["beam_softness"] = lighthouseDefaultsLocal["beam_softness"]
	}
	if out["tower_height"] <= 0 {
		out["tower_height"] = lighthouseDefaultsLocal["tower_height"]
	}
	if out["tower_width"] <= 0 {
		out["tower_width"] = lighthouseDefaultsLocal["tower_width"]
	}
	if out["horizon"] <= 0 {
		out["horizon"] = lighthouseDefaultsLocal["horizon"]
	}
	if out["haze"] <= 0 {
		out["haze"] = lighthouseDefaultsLocal["haze"]
	}
	if out["glow"] <= 0 {
		out["glow"] = lighthouseDefaultsLocal["glow"]
	}
	if out["hue"] == 0 {
		out["hue"] = lighthouseDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = lighthouseDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = lighthouseDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = lighthouseDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["bright_pass_dur"] <= 0 {
		out["bright_pass_dur"] = lighthouseDefaultsLocal["bright_pass_dur"]
	}
	if out["bright_pass_mult"] <= 0 {
		out["bright_pass_mult"] = lighthouseDefaultsLocal["bright_pass_mult"]
	}
	if out["fog_thicken_dur"] <= 0 {
		out["fog_thicken_dur"] = lighthouseDefaultsLocal["fog_thicken_dur"]
	}
	if out["fog_thicken_mult"] <= 0 {
		out["fog_thicken_mult"] = lighthouseDefaultsLocal["fog_thicken_mult"]
	}
	if out["calm_dur"] <= 0 {
		out["calm_dur"] = lighthouseDefaultsLocal["calm_dur"]
	}
	if out["calm_mult"] <= 0 {
		out["calm_mult"] = lighthouseDefaultsLocal["calm_mult"]
	}
	return out
}

func NewLighthouse(w, h int, seed int64, cfg LighthouseConfig) *Lighthouse {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Lighthouse{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeLighthouseDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (l *Lighthouse) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if w == l.W && h == l.H {
		return
	}
	l.W = w
	l.H = h
	l.Grid = make([][]Pixel, h)
	for i := range l.Grid {
		l.Grid[i] = make([]Pixel, w)
	}
}

func (l *Lighthouse) SetConfig(cfg LighthouseConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg = mergeLighthouseDefaults(cfg)
}

func (l *Lighthouse) EffectiveConfig() LighthouseConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	return cloneConfig(l.cfg)
}

func (l *Lighthouse) Snapshot() LighthouseSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LighthouseSnapshot{
		ProceduralState: l.snapshotStateLocked(),
		RNGState:        l.rng.State(),
	}
}

func (l *Lighthouse) RestoreSnapshot(snap LighthouseSnapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		l.rng.SetState(snap.RNGState)
	}
}

func (l *Lighthouse) SnapshotPersistedState() LighthousePersistedState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LighthousePersistedState{
		ProceduralState: l.snapshotStateLocked(),
		RNGState:        l.rng.State(),
	}
}

func (l *Lighthouse) RestorePersistedState(ps LighthousePersistedState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		l.rng.SetState(ps.RNGState)
	}
}

func (l *Lighthouse) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   l.tick,
		Timers: cloneTimerMap(l.timers),
		Values: cloneValueMap(l.values),
	}
}

func (l *Lighthouse) restoreStateLocked(state ProceduralState) {
	l.tick = state.Tick
	l.timers = cloneTimerMap(state.Timers)
	if l.timers == nil {
		l.timers = make(map[string]int)
	}
	l.values = cloneValueMap(state.Values)
	if l.values == nil {
		l.values = make(map[string]float64)
	}
}

func (l *Lighthouse) CurrentTick() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tick
}

func (l *Lighthouse) PerturbRNG(delta int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rng.Mix(delta)
}

func (l *Lighthouse) DrainLog() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.log) == 0 {
		return nil
	}
	out := l.log
	l.log = nil
	return out
}

func (l *Lighthouse) appendLog(kind, desc string) {
	l.log = append(l.log, LogEntry{Tick: l.tick, Type: kind, Desc: desc})
	if len(l.log) > 200 {
		l.log = l.log[len(l.log)-200:]
	}
}

func (l *Lighthouse) intCfg(key string) int {
	return int(math.Round(l.cfg[key]))
}

func (l *Lighthouse) TriggerEvent(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch name {
	case "bright-pass":
		l.startBrightPassLocked("triggered")
	case "fog-thicken":
		l.startFogThickenLocked("triggered")
	case "calm":
		l.startCalmLocked("triggered")
	case "intro":
		l.startIntroLocked()
		l.appendLog("intro", fmt.Sprintf("started (dur=%d, beam=%.2f)", l.timers["intro"], l.cfg["intro_beam"]))
	case "ending":
		l.startEndingLocked()
		l.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", l.intCfg("ending_dur"), l.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (l *Lighthouse) Step() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tick++
	for key, value := range l.timers {
		if value > 0 {
			l.timers[key] = value - 1
		}
	}
	l.stepLocked()
}

func (l *Lighthouse) startBrightPassLocked(verb string) {
	l.timers["bright-pass"] = jitterInt(l.rng, l.intCfg("bright_pass_dur"), 0.3)
	l.values["bright_gain"] = l.cfg["bright_pass_mult"] * (0.8 + l.rng.Float64()*0.4)
	l.appendLog("bright-pass", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["bright-pass"], l.values["bright_gain"]))
}

func (l *Lighthouse) startFogThickenLocked(verb string) {
	l.timers["fog-thicken"] = jitterInt(l.rng, l.intCfg("fog_thicken_dur"), 0.3)
	l.timers["calm"] = 0
	l.values["fog_gain"] = l.cfg["fog_thicken_mult"] * (0.8 + l.rng.Float64()*0.45)
	l.appendLog("fog-thicken", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["fog-thicken"], l.values["fog_gain"]))
}

func (l *Lighthouse) startCalmLocked(verb string) {
	l.timers["calm"] = jitterInt(l.rng, l.intCfg("calm_dur"), 0.3)
	l.timers["fog-thicken"] = 0
	l.values["fog_gain"] = 1
	l.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["calm"], l.cfg["calm_mult"]))
}

func (l *Lighthouse) startIntroLocked() {
	l.timers["bright-pass"] = 0
	l.timers["fog-thicken"] = 0
	l.timers["calm"] = 0
	l.timers["ending"] = 0
	l.values["bright_gain"] = 1
	l.values["fog_gain"] = 1
	l.timers["intro"] = l.intCfg("intro_dur")
	l.values["intro_total"] = float64(l.timers["intro"])
}

func (l *Lighthouse) startEndingLocked() {
	l.timers["intro"] = 0
	l.timers["bright-pass"] = 0
	l.timers["fog-thicken"] = 0
	l.timers["calm"] = 0
	l.values["bright_gain"] = 1
	l.values["fog_gain"] = 1
	endingTotal := l.intCfg("ending_dur") + max(0, l.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, l.intCfg("ending_dur"))
	}
	l.timers["ending"] = endingTotal
	l.values["ending_total"] = float64(endingTotal)
}

func (l *Lighthouse) stepLocked() {
	if l.timers["bright-pass"] <= 0 {
		l.values["bright_gain"] = 1
	}
	if l.timers["fog-thicken"] <= 0 {
		l.values["fog_gain"] = 1
	}
	if l.timers["intro"] <= 0 {
		delete(l.values, "intro_total")
	}
	if l.timers["ending"] <= 0 {
		delete(l.values, "ending_total")
	}
	if l.timers["intro"] > 0 || l.timers["ending"] > 0 {
		return
	}
	if l.timers["bright-pass"] <= 0 && l.cfg["bright_pass_p"] > 0 && l.rng.Float64() < l.cfg["bright_pass_p"] {
		l.startBrightPassLocked("started")
	}
	if l.timers["fog-thicken"] <= 0 && l.timers["calm"] <= 0 && l.cfg["fog_thicken_p"] > 0 && l.rng.Float64() < l.cfg["fog_thicken_p"] {
		l.startFogThickenLocked("started")
	}
	if l.timers["calm"] <= 0 && l.timers["fog-thicken"] <= 0 && l.cfg["calm_p"] > 0 && l.rng.Float64() < l.cfg["calm_p"] {
		l.startCalmLocked("started")
	}
}
