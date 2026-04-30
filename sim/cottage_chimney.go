package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type CottageChimneyConfig = ProceduralConfig
type CottageChimneyState = ProceduralState
type CottageChimneySnapshot = ProceduralSnapshot
type CottageChimneyPersistedState = ProceduralPersistedState

// CottageChimney is a dedicated sim type for the cottage-chimney-smoke effect.
// A single warm-lit cottage silhouette at night with a slow rising plume from
// the chimney. The server decides discrete events (puff emission, gusts, lamp
// flickers, embers, quiet nights); clients render the steady plume locally.
type CottageChimney struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    CottageChimneyConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var cottageChimneyDefaultsLocal = CottageChimneyConfig{
	"intro_dur":         60,
	"intro_glow":        0.05,
	"intro_first_puff":  30,
	"ending_dur":        80,
	"ending_linger":     30,
	"ending_glow":       0.05,
	"ending_puffs":      3,
	"puff_rate":         0.18,
	"plume_rise":        0.55,
	"plume_drift":       0.12,
	"plume_width":       3.6,
	"plume_softness":    0.45,
	"plume_top":         0.06,
	"cottage_width":     34.0,
	"cottage_height":    18.0,
	"roof_pitch":        0.55,
	"chimney_height":    7.0,
	"horizon":           0.78,
	"window_glow":       0.78,
	"window_hue":        46,
	"hue":               222,
	"hue_sp":            18,
	"sat":               0.36,
	"lmin":              0.06,
	"lmax":              0.32,
	"gust_p":            0.0,
	"flicker_p":         0.0,
	"ember_p":           0.0,
	"quiet_p":           0.0,
	"gust_dur":          54,
	"gust_drift_mult":   2.4,
	"flicker_dur":       18,
	"flicker_mult":      0.45,
	"ember_dur":         42,
	"quiet_dur":         220,
	"quiet_rate_mult":   0.35,
}

func CottageChimneySchema() EffectSchema {
	return EffectSchema{
		Name: "cottage-chimney",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent warming the window glow up to its target before the first puff."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.01, Default: 0.05,
				Description: "Starting fraction of the window glow before the cottage warms up."},
			{Key: "intro_first_puff", Label: "first puff", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 30, Trigger: "puff",
				Description: "Ticks after the window lights before the first chimney puff leaves."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 320, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent dimming the window glow as the cottage settles into full dark."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 30,
				Description: "Extra quiet ticks after the window glow has gone."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.05,
				Description: "Residual window glow at the end of the outro."},
			{Key: "ending_puffs", Label: "ending puffs", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 8, Step: 1, Default: 3,
				Description: "Number of slow final puffs that leave the chimney during the outro."},
			{Key: "puff_rate", Label: "puff rate", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Average puffs per second leaving the chimney during steady state."},
			{Key: "plume_rise", Label: "rise speed", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: 0.1, Max: 1.5, Step: 0.05, Default: 0.55,
				Description: "Vertical speed of each puff as it climbs above the chimney."},
			{Key: "plume_drift", Label: "wind drift", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.02, Default: 0.12,
				Description: "Steady horizontal wind drift applied to puffs as they rise."},
			{Key: "plume_width", Label: "plume width", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: 1.5, Max: 7.5, Step: 0.1, Default: 3.6,
				Description: "Base width of each smoke puff in cells."},
			{Key: "plume_softness", Label: "plume soft", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.05, Default: 0.45,
				Description: "How softly each puff falls off into the surrounding air."},
			{Key: "plume_top", Label: "plume top", Slot: SlotLever, Group: "plume", Type: KnobFloat, Min: 0.0, Max: 0.45, Step: 0.01, Default: 0.06,
				Description: "Fraction of the canvas at the top where puffs fade to nothing."},
			{Key: "cottage_width", Label: "cottage w", Slot: SlotLever, Group: "cottage", Type: KnobFloat, Min: 18, Max: 56, Step: 1, Default: 34,
				Description: "Width of the cottage silhouette in cells."},
			{Key: "cottage_height", Label: "cottage h", Slot: SlotLever, Group: "cottage", Type: KnobFloat, Min: 10, Max: 28, Step: 1, Default: 18,
				Description: "Height of the cottage silhouette in cells."},
			{Key: "roof_pitch", Label: "roof pitch", Slot: SlotLever, Group: "cottage", Type: KnobFloat, Min: 0.2, Max: 0.9, Step: 0.02, Default: 0.55,
				Description: "How steeply the gable roof rises above the cottage body."},
			{Key: "chimney_height", Label: "chimney h", Slot: SlotLever, Group: "cottage", Type: KnobFloat, Min: 3, Max: 12, Step: 0.5, Default: 7,
				Description: "Height of the chimney above the roofline."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "cottage", Type: KnobFloat, Min: 0.55, Max: 0.92, Step: 0.01, Default: 0.78,
				Description: "Vertical position of the ground line under the cottage."},
			{Key: "window_glow", Label: "window glow", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1.0, Step: 0.02, Default: 0.78,
				Description: "Brightness of the warm-lit window — the scene-palette knob."},
			{Key: "window_hue", Label: "window hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 46,
				Description: "Hue of the window glow. Warm yellows around 40-60; candle amber lower; cool blues higher."},
			{Key: "hue", Label: "sky hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 280, Step: 1, Default: 222,
				Description: "Base night-sky hue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 18,
				Description: "Variation between upper night sky and the horizon."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.36,
				Description: "Overall night-sky saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.02, Max: 0.3, Step: 0.01, Default: 0.06,
				Description: "Minimum lightness used for the deepest part of the sky."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.6, Step: 0.01, Default: 0.32,
				Description: "Maximum lightness used near the horizon."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance of a stronger sideways wind gust bending the plume."},
			{Key: "flicker_p", Label: "flicker", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.04, Step: 0.001, Default: 0, Trigger: "lamp-flicker",
				Description: "Per-tick chance of the window glow dipping briefly."},
			{Key: "ember_p", Label: "ember", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "embers",
				Description: "Per-tick chance of a bright spark drifting up alongside a puff."},
			{Key: "quiet_p", Label: "quiet", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "quiet-night",
				Description: "Per-tick chance of a long quiet window with thinner emission."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind-gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 54,
				Description: "Duration of a wind-gust window."},
			{Key: "gust_drift_mult", Label: "gust drift x", Slot: SlotEventMod, Group: "wind-gust", Type: KnobFloat, Min: 1.1, Max: 4.0, Step: 0.1, Default: 2.4,
				Description: "Drift multiplier applied to the plume during a gust."},
			{Key: "flicker_dur", Label: "flicker dur", Slot: SlotEventMod, Group: "lamp-flicker", Type: KnobInt, Min: 4, Max: 90, Step: 2, Default: 18,
				Description: "Duration of the window-glow dip during a flicker."},
			{Key: "flicker_mult", Label: "flicker x", Slot: SlotEventMod, Group: "lamp-flicker", Type: KnobFloat, Min: 0.1, Max: 0.95, Step: 0.05, Default: 0.45,
				Description: "Glow multiplier applied while the window flickers."},
			{Key: "ember_dur", Label: "ember dur", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 10, Max: 120, Step: 2, Default: 42,
				Description: "Duration of the bright-spark drift up the plume."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-night", Type: KnobInt, Min: 60, Max: 600, Step: 10, Default: 220,
				Description: "Duration of the long suppression window."},
			{Key: "quiet_rate_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-night", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Puff-rate multiplier applied while the cottage settles into a quiet night."},
		},
	}
}

func defaultCottageChimneyConfig() CottageChimneyConfig {
	return cloneConfig(cottageChimneyDefaultsLocal)
}

func mergeCottageChimneyDefaults(cfg CottageChimneyConfig) CottageChimneyConfig {
	out := defaultCottageChimneyConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = cottageChimneyDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["intro_first_puff"] < 0 {
		out["intro_first_puff"] = 0
	}
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = cottageChimneyDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["ending_puffs"] < 0 {
		out["ending_puffs"] = 0
	}
	if out["puff_rate"] <= 0 {
		out["puff_rate"] = cottageChimneyDefaultsLocal["puff_rate"]
	}
	if out["plume_rise"] <= 0 {
		out["plume_rise"] = cottageChimneyDefaultsLocal["plume_rise"]
	}
	if out["plume_width"] <= 0 {
		out["plume_width"] = cottageChimneyDefaultsLocal["plume_width"]
	}
	if out["plume_softness"] <= 0 {
		out["plume_softness"] = cottageChimneyDefaultsLocal["plume_softness"]
	}
	if out["plume_top"] < 0 {
		out["plume_top"] = 0
	}
	if out["cottage_width"] <= 0 {
		out["cottage_width"] = cottageChimneyDefaultsLocal["cottage_width"]
	}
	if out["cottage_height"] <= 0 {
		out["cottage_height"] = cottageChimneyDefaultsLocal["cottage_height"]
	}
	if out["roof_pitch"] <= 0 {
		out["roof_pitch"] = cottageChimneyDefaultsLocal["roof_pitch"]
	}
	if out["chimney_height"] <= 0 {
		out["chimney_height"] = cottageChimneyDefaultsLocal["chimney_height"]
	}
	if out["horizon"] <= 0 {
		out["horizon"] = cottageChimneyDefaultsLocal["horizon"]
	}
	if out["window_glow"] <= 0 {
		out["window_glow"] = cottageChimneyDefaultsLocal["window_glow"]
	}
	if out["window_hue"] < 0 {
		out["window_hue"] = cottageChimneyDefaultsLocal["window_hue"]
	}
	if out["hue"] == 0 {
		out["hue"] = cottageChimneyDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = cottageChimneyDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = cottageChimneyDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = cottageChimneyDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = cottageChimneyDefaultsLocal["gust_dur"]
	}
	if out["gust_drift_mult"] <= 0 {
		out["gust_drift_mult"] = cottageChimneyDefaultsLocal["gust_drift_mult"]
	}
	if out["flicker_dur"] <= 0 {
		out["flicker_dur"] = cottageChimneyDefaultsLocal["flicker_dur"]
	}
	if out["flicker_mult"] <= 0 {
		out["flicker_mult"] = cottageChimneyDefaultsLocal["flicker_mult"]
	}
	if out["ember_dur"] <= 0 {
		out["ember_dur"] = cottageChimneyDefaultsLocal["ember_dur"]
	}
	if out["quiet_dur"] <= 0 {
		out["quiet_dur"] = cottageChimneyDefaultsLocal["quiet_dur"]
	}
	if out["quiet_rate_mult"] <= 0 {
		out["quiet_rate_mult"] = cottageChimneyDefaultsLocal["quiet_rate_mult"]
	}
	return out
}

func NewCottageChimney(w, h int, seed int64, cfg CottageChimneyConfig) *CottageChimney {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &CottageChimney{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeCottageChimneyDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (c *CottageChimney) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if w == c.W && h == c.H {
		return
	}
	c.W = w
	c.H = h
	c.Grid = make([][]Pixel, h)
	for i := range c.Grid {
		c.Grid[i] = make([]Pixel, w)
	}
}

func (c *CottageChimney) SetConfig(cfg CottageChimneyConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = mergeCottageChimneyDefaults(cfg)
}

func (c *CottageChimney) EffectiveConfig() CottageChimneyConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneConfig(c.cfg)
}

func (c *CottageChimney) Snapshot() CottageChimneySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CottageChimneySnapshot{ProceduralState: c.snapshotStateLocked()}
}

func (c *CottageChimney) RestoreSnapshot(snap CottageChimneySnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(snap.ProceduralState)
}

func (c *CottageChimney) SnapshotPersistedState() CottageChimneyPersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CottageChimneyPersistedState{
		ProceduralState: c.snapshotStateLocked(),
		RNGState:        c.rng.State(),
	}
}

func (c *CottageChimney) RestorePersistedState(ps CottageChimneyPersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		c.rng.SetState(ps.RNGState)
	}
}

func (c *CottageChimney) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   c.tick,
		Timers: cloneTimerMap(c.timers),
		Values: cloneValueMap(c.values),
	}
}

func (c *CottageChimney) restoreStateLocked(state ProceduralState) {
	c.tick = state.Tick
	c.timers = cloneTimerMap(state.Timers)
	if c.timers == nil {
		c.timers = make(map[string]int)
	}
	c.values = cloneValueMap(state.Values)
	if c.values == nil {
		c.values = make(map[string]float64)
	}
}

func (c *CottageChimney) CurrentTick() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tick
}

func (c *CottageChimney) PerturbRNG(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Mix(delta)
}

func (c *CottageChimney) DrainLog() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.log) == 0 {
		return nil
	}
	out := c.log
	c.log = nil
	return out
}

func (c *CottageChimney) appendLog(kind, desc string) {
	c.log = append(c.log, LogEntry{Tick: c.tick, Type: kind, Desc: desc})
	if len(c.log) > 200 {
		c.log = c.log[len(c.log)-200:]
	}
}

func (c *CottageChimney) intCfg(key string) int {
	return int(math.Round(c.cfg[key]))
}

func (c *CottageChimney) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "puff", "puff-emit":
		c.startPuffLocked("triggered")
	case "wind-gust", "gust":
		c.startGustLocked("triggered")
	case "lamp-flicker", "flicker":
		c.startFlickerLocked("triggered")
	case "embers", "ember":
		c.startEmberLocked("triggered")
	case "quiet-night", "quiet":
		c.startQuietLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", c.timers["intro"], c.cfg["intro_glow"]))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d, puffs=%d)", c.intCfg("ending_dur"), c.intCfg("ending_linger"), c.intCfg("ending_puffs")))
	default:
		return false
	}
	return true
}

func (c *CottageChimney) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++
	for key, value := range c.timers {
		if value > 0 {
			c.timers[key] = value - 1
		}
	}
	c.stepLocked()
}

func (c *CottageChimney) startPuffLocked(verb string) {
	// Each puff is recorded as the tick it left the chimney; the renderer
	// derives the puff's age from (tick - tick_emitted). Storing the
	// emission tick instead of a remaining-life timer keeps puff trajectories
	// deterministic across sim/render replays.
	c.values["last_puff_tick"] = float64(c.tick)
	c.values["puff_count"] = c.values["puff_count"] + 1
	if verb != "" {
		c.appendLog("puff", verb)
	}
}

func (c *CottageChimney) startGustLocked(verb string) {
	c.timers["gust"] = jitterInt(c.rng, c.intCfg("gust_dur"), 0.3)
	c.values["gust_drift"] = c.cfg["gust_drift_mult"] * (0.85 + c.rng.Float64()*0.3)
	c.appendLog("gust", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, c.timers["gust"], c.values["gust_drift"]))
}

func (c *CottageChimney) startFlickerLocked(verb string) {
	c.timers["flicker"] = jitterInt(c.rng, c.intCfg("flicker_dur"), 0.4)
	c.values["flicker_gain"] = c.cfg["flicker_mult"] * (0.7 + c.rng.Float64()*0.4)
	c.appendLog("flicker", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, c.timers["flicker"], c.values["flicker_gain"]))
}

func (c *CottageChimney) startEmberLocked(verb string) {
	c.timers["ember"] = jitterInt(c.rng, c.intCfg("ember_dur"), 0.3)
	c.values["ember_seed"] = float64(c.rng.Intn(1 << 30))
	c.appendLog("ember", fmt.Sprintf("%s (dur=%d)", verb, c.timers["ember"]))
}

func (c *CottageChimney) startQuietLocked(verb string) {
	c.timers["quiet"] = jitterInt(c.rng, c.intCfg("quiet_dur"), 0.25)
	c.appendLog("quiet", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, c.timers["quiet"], c.cfg["quiet_rate_mult"]))
}

func (c *CottageChimney) startIntroLocked() {
	c.timers["gust"] = 0
	c.timers["flicker"] = 0
	c.timers["ember"] = 0
	c.timers["quiet"] = 0
	c.timers["ending"] = 0
	c.values["gust_drift"] = 0
	c.values["flicker_gain"] = 1
	c.timers["intro"] = c.intCfg("intro_dur")
	c.values["intro_total"] = float64(c.timers["intro"])
	first := c.intCfg("intro_first_puff")
	if first < 0 {
		first = 0
	}
	c.timers["intro_puff_wait"] = first
}

func (c *CottageChimney) startEndingLocked() {
	c.timers["intro"] = 0
	c.timers["intro_puff_wait"] = 0
	c.timers["gust"] = 0
	c.timers["flicker"] = 0
	c.timers["ember"] = 0
	c.timers["quiet"] = 0
	c.values["gust_drift"] = 0
	c.values["flicker_gain"] = 1
	endingTotal := c.intCfg("ending_dur") + max(0, c.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, c.intCfg("ending_dur"))
	}
	c.timers["ending"] = endingTotal
	c.values["ending_total"] = float64(endingTotal)
	c.values["ending_puffs_left"] = math.Max(0, math.Round(c.cfg["ending_puffs"]))
	// Spread the remaining outro puffs across the fade window.
	endingFade := max(1, c.intCfg("ending_dur"))
	if c.values["ending_puffs_left"] > 0 {
		c.timers["ending_puff_wait"] = endingFade / int(c.values["ending_puffs_left"]+1)
	}
}

func (c *CottageChimney) stepLocked() {
	if c.timers["gust"] <= 0 {
		c.values["gust_drift"] = 0
	}
	if c.timers["flicker"] <= 0 {
		c.values["flicker_gain"] = 1
	}
	if c.timers["intro"] <= 0 {
		delete(c.values, "intro_total")
	}

	if c.timers["intro"] > 0 {
		// Window glow warms up; first puff held until the wait timer expires.
		if c.timers["intro_puff_wait"] <= 0 {
			c.startPuffLocked("intro")
			// Don't re-emit during the rest of intro; let the steady-state path take over once intro ends.
			c.timers["intro_puff_wait"] = c.timers["intro"] + 1
		}
		return
	}
	if c.timers["ending"] > 0 {
		// Periodic outro puffs, then silence.
		if c.values["ending_puffs_left"] > 0 && c.timers["ending_puff_wait"] <= 0 {
			c.startPuffLocked("ending")
			c.values["ending_puffs_left"]--
			endingFade := max(1, c.intCfg("ending_dur"))
			c.timers["ending_puff_wait"] = endingFade / (int(c.values["ending_puffs_left"]) + 2)
		}
		return
	}
	if c.timers["ending"] <= 0 {
		delete(c.values, "ending_total")
		delete(c.values, "ending_puffs_left")
	}

	// Steady state: probabilistic puff emission, scaled down during quiet nights.
	rate := c.cfg["puff_rate"]
	if c.timers["quiet"] > 0 {
		rate *= c.cfg["quiet_rate_mult"]
	}
	if rate > 0 {
		// puff_rate is "puffs per second"; sim runs at 10 Hz, so per-tick chance is rate/10.
		perTick := rate / 10
		if perTick > 1 {
			perTick = 1
		}
		if c.rng.Float64() < perTick {
			c.startPuffLocked("")
		}
	}

	if c.timers["gust"] <= 0 && c.cfg["gust_p"] > 0 && c.rng.Float64() < c.cfg["gust_p"] {
		c.startGustLocked("started")
	}
	if c.timers["flicker"] <= 0 && c.cfg["flicker_p"] > 0 && c.rng.Float64() < c.cfg["flicker_p"] {
		c.startFlickerLocked("started")
	}
	if c.timers["ember"] <= 0 && c.cfg["ember_p"] > 0 && c.rng.Float64() < c.cfg["ember_p"] {
		c.startEmberLocked("started")
	}
	if c.timers["quiet"] <= 0 && c.cfg["quiet_p"] > 0 && c.rng.Float64() < c.cfg["quiet_p"] {
		c.startQuietLocked("started")
	}
}
