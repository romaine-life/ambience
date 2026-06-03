package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type AuroraConfig = ProceduralConfig
type AuroraState = ProceduralState
type AuroraSnapshot = ProceduralSnapshot
type AuroraPersistedState = ProceduralPersistedState

// Aurora is a dedicated sim type for the aurora effect, replacing the
// kind="aurora" branch of the old Procedural type.
type Aurora struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    AuroraConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var auroraDefaultsLocal = AuroraConfig{
	"intro_dur":     70,
	"intro_glow":    0.18,
	"ending_dur":    80,
	"ending_linger": 20,
	"ending_glow":   0.05,
	"intensity":     0.56,
	"speed":         0.11,
	"drift":         0.08,
	"bands":         3,
	"thickness":     9,
	"wave_amp":      6,
	"wave_freq":     0.16,
	"curtain_len":   15,
	"hue":           138,
	"hue_sp":        26,
	"sat":           0.72,
	"lmin":          0.20,
	"lmax":          0.74,
	"brighten_p":    0.0,
	"shift_p":       0.0,
	"fade_p":        0.0,
	"brighten_dur":  42,
	"brighten_mult": 1.45,
	"shift_dur":     64,
	"shift_amt":     1.10,
	"fade_dur":      58,
	"fade_mult":     0.60,
}

func AuroraSchema() EffectSchema {
	return EffectSchema{
		Name: "aurora",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 260, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent blooming from a faint horizon glow into the full aurora."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.01, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Starting brightness fraction before the ribbons fully form."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 260, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent dimming and narrowing back toward a dark sky."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks for the last faint glow to hang over the horizon."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.05,
				Description: "Residual brightness fraction that remains near the end of the outro."},
			{Key: "intensity", Label: "intensity", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.05, Max: 1.2, Step: 0.01, Default: 0.56,
				Description: "Overall luminance of the aurora ribbons."},
			{Key: "speed", Label: "motion", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.02, Max: 0.45, Step: 0.01, Default: 0.11,
				Description: "How quickly the ribbons undulate across the sky."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Baseline sideways drift for the whole veil field."},
			{Key: "bands", Label: "bands", Slot: SlotLever, Group: "sky", Type: KnobInt, Min: 1, Max: 5, Step: 1, Default: 3,
				Description: "Number of main aurora ribbons."},
			{Key: "thickness", Label: "thickness", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 2, Max: 18, Step: 0.5, Default: 9,
				Description: "Vertical thickness of each bright ribbon core."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 1, Max: 18, Step: 0.5, Default: 6,
				Description: "How far each ribbon arches and sways."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.04, Max: 0.4, Step: 0.01, Default: 0.16,
				Description: "Horizontal frequency of the ribbon arches."},
			{Key: "curtain_len", Label: "curtain", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 2, Max: 28, Step: 0.5, Default: 15,
				Description: "How far the glow trails downward from each ribbon."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 100, Max: 220, Step: 1, Default: 138,
				Description: "Base aurora hue. Lower values lean green; higher values tip toward cyan-violet."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 26,
				Description: "Variation between the different bands and edge glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Overall color saturation of the sky glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.20,
				Description: "Minimum lightness used for the faint outer glow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 0.95, Step: 0.01, Default: 0.74,
				Description: "Maximum lightness used for the brightest ribbon centers."},
			{Key: "brighten_p", Label: "brighten", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "brighten",
				Description: "Per-tick chance of the sky blooming into a brighter curtain."},
			{Key: "shift_p", Label: "shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "shift",
				Description: "Per-tick chance of the aurora sliding into a new wave alignment."},
			{Key: "fade_p", Label: "fade", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "fade",
				Description: "Per-tick chance of the ribbons briefly thinning into a dimmer phase."},
			{Key: "brighten_dur", Label: "bright dur", Slot: SlotEventMod, Group: "brighten", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 42,
				Description: "How long a brighten bloom lasts."},
			{Key: "brighten_mult", Label: "bright x", Slot: SlotEventMod, Group: "brighten", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.45,
				Description: "Brightness multiplier applied during a bloom."},
			{Key: "shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "shift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 64,
				Description: "How long the shifted alignment remains active."},
			{Key: "shift_amt", Label: "shift amt", Slot: SlotEventMod, Group: "shift", Type: KnobFloat, Min: 0.1, Max: 3, Step: 0.05, Default: 1.1,
				Description: "How strongly a shift event pulls the ribbons into a new phase."},
			{Key: "fade_dur", Label: "fade dur", Slot: SlotEventMod, Group: "fade", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "How long the dimmer phase lasts."},
			{Key: "fade_mult", Label: "fade x", Slot: SlotEventMod, Group: "fade", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.6,
				Description: "Brightness multiplier applied during a fade event."},
		},
	}
}

func defaultAuroraConfig() AuroraConfig { return cloneConfig(auroraDefaultsLocal) }

func mergeAuroraDefaults(cfg AuroraConfig) AuroraConfig {
	out := defaultAuroraConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = auroraDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = auroraDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["intensity"] <= 0 {
		out["intensity"] = auroraDefaultsLocal["intensity"]
	}
	if out["speed"] <= 0 {
		out["speed"] = auroraDefaultsLocal["speed"]
	}
	if out["bands"] < 1 {
		out["bands"] = auroraDefaultsLocal["bands"]
	}
	if out["thickness"] <= 0 {
		out["thickness"] = auroraDefaultsLocal["thickness"]
	}
	if out["wave_amp"] <= 0 {
		out["wave_amp"] = auroraDefaultsLocal["wave_amp"]
	}
	if out["wave_freq"] <= 0 {
		out["wave_freq"] = auroraDefaultsLocal["wave_freq"]
	}
	if out["curtain_len"] <= 0 {
		out["curtain_len"] = auroraDefaultsLocal["curtain_len"]
	}
	if out["hue"] == 0 {
		out["hue"] = auroraDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = auroraDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = auroraDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = auroraDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["brighten_dur"] <= 0 {
		out["brighten_dur"] = auroraDefaultsLocal["brighten_dur"]
	}
	if out["brighten_mult"] <= 0 {
		out["brighten_mult"] = auroraDefaultsLocal["brighten_mult"]
	}
	if out["shift_dur"] <= 0 {
		out["shift_dur"] = auroraDefaultsLocal["shift_dur"]
	}
	if out["shift_amt"] <= 0 {
		out["shift_amt"] = auroraDefaultsLocal["shift_amt"]
	}
	if out["fade_dur"] <= 0 {
		out["fade_dur"] = auroraDefaultsLocal["fade_dur"]
	}
	if out["fade_mult"] <= 0 {
		out["fade_mult"] = auroraDefaultsLocal["fade_mult"]
	}
	return out
}

func NewAurora(w, h int, seed int64, cfg AuroraConfig) *Aurora {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Aurora{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeAuroraDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (a *Aurora) Resize(w, h int) {
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

func (a *Aurora) SetConfig(cfg AuroraConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = mergeAuroraDefaults(cfg)
}

func (a *Aurora) EffectiveConfig() AuroraConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneConfig(a.cfg)
}

func (a *Aurora) Snapshot() AuroraSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AuroraSnapshot{
		ProceduralState: a.snapshotStateLocked(),
		RNGState:        a.rng.State(),
	}
}

func (a *Aurora) RestoreSnapshot(snap AuroraSnapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		a.rng.SetState(snap.RNGState)
	}
}

func (a *Aurora) SnapshotPersistedState() AuroraPersistedState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AuroraPersistedState{
		ProceduralState: a.snapshotStateLocked(),
		RNGState:        a.rng.State(),
	}
}

func (a *Aurora) RestorePersistedState(ps AuroraPersistedState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		a.rng.SetState(ps.RNGState)
	}
}

func (a *Aurora) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   a.tick,
		Timers: cloneTimerMap(a.timers),
		Values: cloneValueMap(a.values),
	}
}

func (a *Aurora) restoreStateLocked(state ProceduralState) {
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

func (a *Aurora) CurrentTick() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tick
}

func (a *Aurora) PerturbRNG(delta int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rng.Mix(delta)
}

func (a *Aurora) DrainLog() []LogEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.log) == 0 {
		return nil
	}
	out := a.log
	a.log = nil
	return out
}

func (a *Aurora) appendLog(kind, desc string) {
	a.log = append(a.log, LogEntry{Tick: a.tick, Type: kind, Desc: desc})
	if len(a.log) > 200 {
		a.log = a.log[len(a.log)-200:]
	}
}

func (a *Aurora) intCfg(key string) int {
	return int(math.Round(a.cfg[key]))
}

func (a *Aurora) TriggerEvent(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch name {
	case "brighten":
		a.startBrightenLocked("triggered")
	case "shift":
		a.startShiftLocked("triggered")
	case "fade":
		a.startFadeLocked("triggered")
	case "intro":
		a.startIntroLocked()
		a.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", a.timers["intro"], a.cfg["intro_glow"]))
	case "ending":
		a.startEndingLocked()
		a.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", a.intCfg("ending_dur"), a.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (a *Aurora) Step() {
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

func (a *Aurora) startBrightenLocked(verb string) {
	a.timers["brighten"] = jitterInt(a.rng, a.intCfg("brighten_dur"), 0.3)
	a.values["brighten_gain"] = a.cfg["brighten_mult"] * (0.85 + a.rng.Float64()*0.35)
	a.appendLog("brighten", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, a.timers["brighten"], a.values["brighten_gain"]))
}

func (a *Aurora) startShiftLocked(verb string) {
	a.timers["shift"] = jitterInt(a.rng, a.intCfg("shift_dur"), 0.3)
	sign := 1.0
	if a.rng.Float64() < 0.5 {
		sign = -1
	}
	a.values["shift_push"] = sign * a.cfg["shift_amt"] * (0.55 + a.rng.Float64()*0.55)
	a.values["shift_seed"] = a.rng.Float64() * math.Pi * 2
	a.appendLog("shift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, a.timers["shift"], a.values["shift_push"]))
}

func (a *Aurora) startFadeLocked(verb string) {
	a.timers["fade"] = jitterInt(a.rng, a.intCfg("fade_dur"), 0.3)
	a.appendLog("fade", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, a.timers["fade"], a.cfg["fade_mult"]))
}

func (a *Aurora) startIntroLocked() {
	a.timers["brighten"] = 0
	a.timers["shift"] = 0
	a.timers["fade"] = 0
	a.timers["ending"] = 0
	a.values["brighten_gain"] = 0
	a.values["shift_push"] = 0
	a.values["shift_seed"] = 0
	a.timers["intro"] = a.intCfg("intro_dur")
	a.values["intro_total"] = float64(a.timers["intro"])
}

func (a *Aurora) startEndingLocked() {
	a.timers["intro"] = 0
	a.timers["brighten"] = 0
	a.timers["shift"] = 0
	a.timers["fade"] = 0
	a.values["brighten_gain"] = 0
	a.values["shift_push"] = 0
	a.values["shift_seed"] = 0
	endingTotal := a.intCfg("ending_dur") + max(0, a.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, a.intCfg("ending_dur"))
	}
	a.timers["ending"] = endingTotal
	a.values["ending_total"] = float64(endingTotal)
}

func (a *Aurora) stepLocked() {
	if a.timers["brighten"] <= 0 {
		a.values["brighten_gain"] = 0
	}
	if a.timers["shift"] <= 0 {
		a.values["shift_push"] = 0
		a.values["shift_seed"] = 0
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
	if a.timers["brighten"] <= 0 && a.cfg["brighten_p"] > 0 && a.rng.Float64() < a.cfg["brighten_p"] {
		a.startBrightenLocked("started")
	}
	if a.timers["shift"] <= 0 && a.cfg["shift_p"] > 0 && a.rng.Float64() < a.cfg["shift_p"] {
		a.startShiftLocked("started")
	}
	if a.timers["fade"] <= 0 && a.cfg["fade_p"] > 0 && a.rng.Float64() < a.cfg["fade_p"] {
		a.startFadeLocked("started")
	}
}
