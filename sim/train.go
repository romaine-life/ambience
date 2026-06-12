package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type TrainConfig = ProceduralConfig

// TrainState is the shared procedural state plus the lifecycle observer
// contract field. Lifecycle is derived at snapshot time and never restored.
type TrainState struct {
	ProceduralState
	Lifecycle Lifecycle `json:"lifecycle"`
}

type TrainSnapshot struct {
	TrainState
	RNGState uint64 `json:"rngState,omitempty"`
}

type TrainPersistedState struct {
	TrainState
	RNGState uint64 `json:"rngState"`
}

// Train is a dedicated sim type for the train effect: a horizontal locomotive
// pull that crosses the frame at long intervals, with the horizon mostly
// static between passes.
type Train struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    TrainConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var trainDefaultsLocal = TrainConfig{
	"intro_dur":          60,
	"intro_glow":         0.4,
	"ending_dur":         70,
	"ending_linger":      24,
	"ending_glow":        0.1,
	"horizon":            0.7,
	"track_y":            0.78,
	"loco_len":           7,
	"car_len":            6,
	"cars":               3,
	"train_height":       5,
	"light_glow":         0.45,
	"smoke":              0.32,
	"cue_lead":           14,
	"tail_linger":        12,
	"hue":                220,
	"hue_sp":             18,
	"sat":                0.42,
	"lmin":               0.1,
	"lmax":               0.78,
	"pass_p":             0.0,
	"express_p":          0.0,
	"quiet_p":            0.0,
	"pass_dur":           160,
	"express_dur":        110,
	"express_speed_mult": 1.7,
	"quiet_dur":          240,
	"quiet_mult":         0.15,
}

func TrainSchema() EffectSchema {
	return EffectSchema{
		Name: "train",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent easing the first headlight cue and rumble into the empty frame."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.4,
				Description: "Starting fraction of the lead-light glow before the scene settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent fading any in-flight train and clearing the residual dust."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24,
				Description: "Extra quiet ticks after the last train has cleared the frame."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.1,
				Description: "Residual headlight or steam glow strength near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0.5, Max: 0.86, Step: 0.01, Default: 0.7,
				Description: "Vertical position of the distant horizon line."},
			{Key: "track_y", Label: "track y", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0.55, Max: 0.92, Step: 0.01, Default: 0.78,
				Description: "Vertical position of the rail line that the train rides on."},
			{Key: "loco_len", Label: "loco len", Slot: SlotLever, Group: "train", Type: KnobFloat, Min: 4, Max: 14, Step: 0.5, Default: 7,
				Description: "Length of the locomotive silhouette in grid cells."},
			{Key: "car_len", Label: "car len", Slot: SlotLever, Group: "train", Type: KnobFloat, Min: 3, Max: 14, Step: 0.5, Default: 6,
				Description: "Length of each trailing car silhouette in grid cells."},
			{Key: "cars", Label: "cars", Slot: SlotLever, Group: "train", Type: KnobInt, Min: 0, Max: 8, Step: 1, Default: 3,
				Description: "Number of trailing cars pulled behind the locomotive."},
			{Key: "train_height", Label: "train height", Slot: SlotLever, Group: "train", Type: KnobFloat, Min: 2, Max: 10, Step: 0.5, Default: 5,
				Description: "Visible height of the train silhouette above the rail."},
			{Key: "light_glow", Label: "light glow", Slot: SlotLever, Group: "train", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Headlight halo intensity at the front of the locomotive."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "train", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.32,
				Description: "Trailing smoke / steam plume drifting up off the locomotive."},
			{Key: "cue_lead", Label: "cue lead", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 0, Max: 60, Step: 1, Default: 14,
				Description: "Ticks of distant headlight / vibration cue before the locomotive enters the frame."},
			{Key: "tail_linger", Label: "tail linger", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 0, Max: 60, Step: 1, Default: 12,
				Description: "Ticks of residual dust or steam glow after the last car clears the frame."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 220,
				Description: "Base sky hue. Lower values lean dawn pinks; higher values lean deeper blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between sky bands and trackside terrain."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.1,
				Description: "Minimum lightness used for shadows and trackside silhouette."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for sky glow and headlight halo."},
			{Key: "pass_p", Label: "pass", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "pass",
				Description: "Per-tick chance of an ordinary train passing through the frame."},
			{Key: "express_p", Label: "express", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "express",
				Description: "Per-tick chance of a brighter, faster express crossing the frame."},
			{Key: "quiet_p", Label: "quiet gap", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-gap",
				Description: "Per-tick chance of a long quiet window where passes are rare."},
			{Key: "pass_dur", Label: "pass dur", Slot: SlotEventMod, Group: "pass", Type: KnobInt, Min: 40, Max: 400, Step: 5, Default: 160,
				Description: "Total duration of a pass: cue, sweep, and tail combined."},
			{Key: "express_dur", Label: "express dur", Slot: SlotEventMod, Group: "express", Type: KnobInt, Min: 30, Max: 320, Step: 5, Default: 110,
				Description: "Total duration of an express crossing."},
			{Key: "express_speed_mult", Label: "express x", Slot: SlotEventMod, Group: "express", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.7,
				Description: "Headlight and smoke intensity multiplier applied during an express."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 50, Max: 800, Step: 10, Default: 240,
				Description: "Duration of the quiet-gap window."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobFloat, Min: 0.0, Max: 1, Step: 0.05, Default: 0.15,
				Description: "Probability multiplier applied to pass and express odds while quiet-gap is active."},
		},
	}
}

func defaultTrainConfig() TrainConfig { return cloneConfig(trainDefaultsLocal) }

func mergeTrainDefaults(cfg TrainConfig) TrainConfig {
	out := defaultTrainConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = trainDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = trainDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["horizon"] <= 0 {
		out["horizon"] = trainDefaultsLocal["horizon"]
	}
	if out["track_y"] <= 0 {
		out["track_y"] = trainDefaultsLocal["track_y"]
	}
	if out["track_y"] < out["horizon"] {
		out["track_y"] = out["horizon"] + 0.04
	}
	if out["loco_len"] <= 0 {
		out["loco_len"] = trainDefaultsLocal["loco_len"]
	}
	if out["car_len"] <= 0 {
		out["car_len"] = trainDefaultsLocal["car_len"]
	}
	if out["cars"] < 0 {
		out["cars"] = 0
	}
	if out["train_height"] <= 0 {
		out["train_height"] = trainDefaultsLocal["train_height"]
	}
	if out["light_glow"] <= 0 {
		out["light_glow"] = trainDefaultsLocal["light_glow"]
	}
	if out["smoke"] < 0 {
		out["smoke"] = 0
	}
	if out["cue_lead"] < 0 {
		out["cue_lead"] = 0
	}
	if out["tail_linger"] < 0 {
		out["tail_linger"] = 0
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = trainDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = trainDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = trainDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["pass_dur"] <= 0 {
		out["pass_dur"] = trainDefaultsLocal["pass_dur"]
	}
	if out["express_dur"] <= 0 {
		out["express_dur"] = trainDefaultsLocal["express_dur"]
	}
	if out["express_speed_mult"] <= 0 {
		out["express_speed_mult"] = trainDefaultsLocal["express_speed_mult"]
	}
	if out["quiet_dur"] <= 0 {
		out["quiet_dur"] = trainDefaultsLocal["quiet_dur"]
	}
	if out["quiet_mult"] < 0 {
		out["quiet_mult"] = 0
	}
	return out
}

func NewTrain(w, h int, seed int64, cfg TrainConfig) *Train {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Train{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeTrainDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (t *Train) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if w == t.W && h == t.H {
		return
	}
	t.W = w
	t.H = h
	t.Grid = make([][]Pixel, h)
	for i := range t.Grid {
		t.Grid[i] = make([]Pixel, w)
	}
}

func (t *Train) SetConfig(cfg TrainConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cfg = mergeTrainDefaults(cfg)
}

func (t *Train) EffectiveConfig() TrainConfig {
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneConfig(t.cfg)
}

func (t *Train) Snapshot() TrainSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TrainSnapshot{
		TrainState: t.snapshotStateLocked(),
		RNGState:   t.rng.State(),
	}
}

func (t *Train) RestoreSnapshot(snap TrainSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		t.rng.SetState(snap.RNGState)
	}
}

func (t *Train) SnapshotPersistedState() TrainPersistedState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TrainPersistedState{
		TrainState: t.snapshotStateLocked(),
		RNGState:   t.rng.State(),
	}
}

func (t *Train) RestorePersistedState(ps TrainPersistedState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		t.rng.SetState(ps.RNGState)
	}
}

func (t *Train) snapshotStateLocked() TrainState {
	return TrainState{
		ProceduralState: ProceduralState{
			Tick:   t.tick,
			Timers: cloneTimerMap(t.timers),
			Values: cloneValueMap(t.values),
		},
		Lifecycle: t.lifecycleLocked(),
	}
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// train's timers. The outro is non-terminal: once the ending timer expires,
// automatic pass/express/quiet rolls resume, so lifecycle returns to running
// (the schema declares ending_terminal: false by omission).
func (t *Train) lifecycleLocked() Lifecycle {
	switch {
	case t.timers["intro"] > 0:
		return LifecycleIntro
	case t.timers["ending"] > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (t *Train) restoreStateLocked(state ProceduralState) {
	t.tick = state.Tick
	t.timers = cloneTimerMap(state.Timers)
	if t.timers == nil {
		t.timers = make(map[string]int)
	}
	t.values = cloneValueMap(state.Values)
	if t.values == nil {
		t.values = make(map[string]float64)
	}
}

func (t *Train) CurrentTick() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tick
}

func (t *Train) PerturbRNG(delta int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rng.Mix(delta)
}

func (t *Train) DrainLog() []LogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.log) == 0 {
		return nil
	}
	out := t.log
	t.log = nil
	return out
}

func (t *Train) appendLog(kind, desc string) {
	t.log = append(t.log, LogEntry{Tick: t.tick, Type: kind, Desc: desc})
	if len(t.log) > 200 {
		t.log = t.log[len(t.log)-200:]
	}
}

func (t *Train) intCfg(key string) int {
	return int(math.Round(t.cfg[key]))
}

func (t *Train) TriggerEvent(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch name {
	case "pass":
		t.startPassLocked("triggered")
	case "express":
		t.startExpressLocked("triggered")
	case "quiet-gap":
		t.startQuietGapLocked("triggered")
	case "intro":
		t.startIntroLocked()
		t.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", t.timers["intro"], t.cfg["intro_glow"]))
	case "ending":
		t.startEndingLocked()
		t.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", t.intCfg("ending_dur"), t.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (t *Train) Step() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tick++
	for key, value := range t.timers {
		if value > 0 {
			t.timers[key] = value - 1
		}
	}
	t.stepLocked()
}

func (t *Train) startPassLocked(verb string) {
	t.timers["pass"] = jitterInt(t.rng, t.intCfg("pass_dur"), 0.18)
	t.timers["express"] = 0
	t.values["pass_total"] = float64(t.timers["pass"])
	t.values["pass_dir"] = pickDirection(t.rng)
	delete(t.values, "express_total")
	delete(t.values, "express_dir")
	t.appendLog("pass", fmt.Sprintf("%s (dur=%d, dir=%+0.0f)", verb, t.timers["pass"], t.values["pass_dir"]))
}

func (t *Train) startExpressLocked(verb string) {
	t.timers["express"] = jitterInt(t.rng, t.intCfg("express_dur"), 0.18)
	t.timers["pass"] = 0
	t.values["express_total"] = float64(t.timers["express"])
	t.values["express_dir"] = pickDirection(t.rng)
	delete(t.values, "pass_total")
	delete(t.values, "pass_dir")
	t.appendLog("express", fmt.Sprintf("%s (dur=%d, dir=%+0.0f)", verb, t.timers["express"], t.values["express_dir"]))
}

func (t *Train) startQuietGapLocked(verb string) {
	t.timers["quiet-gap"] = jitterInt(t.rng, t.intCfg("quiet_dur"), 0.25)
	t.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, t.timers["quiet-gap"], t.cfg["quiet_mult"]))
}

func (t *Train) startIntroLocked() {
	t.timers["pass"] = 0
	t.timers["express"] = 0
	t.timers["quiet-gap"] = 0
	t.timers["ending"] = 0
	delete(t.values, "pass_total")
	delete(t.values, "pass_dir")
	delete(t.values, "express_total")
	delete(t.values, "express_dir")
	t.timers["intro"] = t.intCfg("intro_dur")
	t.values["intro_total"] = float64(t.timers["intro"])
}

func (t *Train) startEndingLocked() {
	t.timers["intro"] = 0
	t.timers["pass"] = 0
	t.timers["express"] = 0
	t.timers["quiet-gap"] = 0
	delete(t.values, "pass_total")
	delete(t.values, "pass_dir")
	delete(t.values, "express_total")
	delete(t.values, "express_dir")
	endingTotal := t.intCfg("ending_dur") + max(0, t.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, t.intCfg("ending_dur"))
	}
	t.timers["ending"] = endingTotal
	t.values["ending_total"] = float64(endingTotal)
}

func (t *Train) stepLocked() {
	if t.timers["pass"] <= 0 {
		delete(t.values, "pass_total")
		delete(t.values, "pass_dir")
	}
	if t.timers["express"] <= 0 {
		delete(t.values, "express_total")
		delete(t.values, "express_dir")
	}
	if t.timers["intro"] <= 0 {
		delete(t.values, "intro_total")
	}
	if t.timers["ending"] <= 0 {
		delete(t.values, "ending_total")
	}
	if t.timers["intro"] > 0 || t.timers["ending"] > 0 {
		return
	}
	quiet := t.timers["quiet-gap"] > 0
	quietMult := 1.0
	if quiet {
		quietMult = t.cfg["quiet_mult"]
	}
	if t.timers["pass"] <= 0 && t.timers["express"] <= 0 && t.cfg["pass_p"] > 0 && t.rng.Float64() < t.cfg["pass_p"]*quietMult {
		t.startPassLocked("started")
	}
	if t.timers["pass"] <= 0 && t.timers["express"] <= 0 && t.cfg["express_p"] > 0 && t.rng.Float64() < t.cfg["express_p"]*quietMult {
		t.startExpressLocked("started")
	}
	if !quiet && t.cfg["quiet_p"] > 0 && t.rng.Float64() < t.cfg["quiet_p"] {
		t.startQuietGapLocked("started")
	}
}

func pickDirection(rng *rngutil.RNG) float64 {
	if rng.Float64() < 0.5 {
		return -1
	}
	return 1
}
