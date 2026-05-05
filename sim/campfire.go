package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type CampfireConfig = ProceduralConfig
type CampfireState = ProceduralState
type CampfireSnapshot = ProceduralSnapshot
type CampfirePersistedState = ProceduralPersistedState

// Campfire is a dedicated sim type for the campfire effect.
type Campfire struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    CampfireConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var campfireDefaultsLocal = CampfireConfig{
	"intro_dur":     45,
	"intro_glow":    0.14,
	"ending_dur":    60,
	"ending_linger": 24,
	"ending_glow":   0.08,
	"flame_height":  14.0,
	"flame_width":   10.0,
	"flame_speed":   0.12,
	"flicker":       0.72,
	"ember_rate":    0.26,
	"ember_speed":   0.62,
	"glow":          0.54,
	"hue":           24,
	"hue_sp":        18,
	"sat":           0.82,
	"lmin":          0.32,
	"lmax":          0.94,
	"crackle_p":     0.0,
	"lull_p":        0.0,
	"crackle_dur":   36,
	"crackle_mult":  1.85,
	"lull_dur":      68,
	"lull_mult":     0.55,
}

func CampfireSchema() EffectSchema {
	return EffectSchema{
		Name: "campfire",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks spent catching from a small glow into a stable flame."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Starting fraction of the final campfire intensity before the flame catches fully."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent collapsing the flame down toward ember glow."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24,
				Description: "Extra ember time after the flame has mostly died back."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual flame and ember intensity near the end of the outro."},
			{Key: "flame_height", Label: "flame height", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 6, Max: 24, Step: 0.5, Default: 14,
				Description: "Overall flame height above the logs."},
			{Key: "flame_width", Label: "flame width", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 4, Max: 18, Step: 0.5, Default: 10,
				Description: "Width of the flame body and ember bed."},
			{Key: "flame_speed", Label: "flame speed", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 0.04, Max: 0.3, Step: 0.01, Default: 0.12,
				Description: "How quickly the flame shape flickers and rolls."},
			{Key: "flicker", Label: "flicker", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.72,
				Description: "Side-to-side and height variation inside the flame body."},
			{Key: "ember_rate", Label: "ember rate", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.26,
				Description: "How many embers rise from the fire during the steady state."},
			{Key: "ember_speed", Label: "ember speed", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.62,
				Description: "How quickly embers travel upward and fade."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.54,
				Description: "Strength of the warm localized light cast around the campfire."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 50, Step: 1, Default: 24,
				Description: "Base flame hue. Lower values lean redder; higher values lean more yellow-orange."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 18,
				Description: "Variation between coal reds, orange mids, and bright flame tips."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.82,
				Description: "Overall color saturation of the fire and ember tones."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.32,
				Description: "Minimum lightness used for darker coals, logs, and outer flame edges."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.94,
				Description: "Maximum lightness used for the hottest flame cores and bright embers."},
			{Key: "crackle_p", Label: "crackle", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "crackle",
				Description: "Per-tick chance of a brighter crackle that throws extra embers."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the flame briefly settling into a lower, calmer burn."},
			{Key: "crackle_dur", Label: "crackle dur", Slot: SlotEventMod, Group: "crackle", Type: KnobInt, Min: 8, Max: 160, Step: 4, Default: 36,
				Description: "Duration of the brighter crackling burst."},
			{Key: "crackle_mult", Label: "crackle x", Slot: SlotEventMod, Group: "crackle", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Intensity multiplier applied while a crackle burst is active."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 68,
				Description: "Duration of the quieter lower-flame window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Flame intensity multiplier applied while lull is active."},
		},
	}
}

func defaultCampfireConfig() CampfireConfig { return cloneConfig(campfireDefaultsLocal) }

func mergeCampfireDefaults(cfg CampfireConfig) CampfireConfig {
	out := defaultCampfireConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = campfireDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = campfireDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["flame_height"] <= 0 {
		out["flame_height"] = campfireDefaultsLocal["flame_height"]
	}
	if out["flame_width"] <= 0 {
		out["flame_width"] = campfireDefaultsLocal["flame_width"]
	}
	if out["flame_speed"] <= 0 {
		out["flame_speed"] = campfireDefaultsLocal["flame_speed"]
	}
	if out["flicker"] <= 0 {
		out["flicker"] = campfireDefaultsLocal["flicker"]
	}
	if out["ember_rate"] <= 0 {
		out["ember_rate"] = campfireDefaultsLocal["ember_rate"]
	}
	if out["ember_speed"] <= 0 {
		out["ember_speed"] = campfireDefaultsLocal["ember_speed"]
	}
	if out["glow"] <= 0 {
		out["glow"] = campfireDefaultsLocal["glow"]
	}
	if out["hue"] == 0 {
		out["hue"] = campfireDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = campfireDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = campfireDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = campfireDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["crackle_dur"] <= 0 {
		out["crackle_dur"] = campfireDefaultsLocal["crackle_dur"]
	}
	if out["crackle_mult"] <= 0 {
		out["crackle_mult"] = campfireDefaultsLocal["crackle_mult"]
	}
	if out["lull_dur"] <= 0 {
		out["lull_dur"] = campfireDefaultsLocal["lull_dur"]
	}
	if out["lull_mult"] <= 0 {
		out["lull_mult"] = campfireDefaultsLocal["lull_mult"]
	}
	return out
}

func NewCampfire(w, h int, seed int64, cfg CampfireConfig) *Campfire {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Campfire{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeCampfireDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (c *Campfire) Resize(w, h int) {
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

func (c *Campfire) SetConfig(cfg CampfireConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = mergeCampfireDefaults(cfg)
}

func (c *Campfire) EffectiveConfig() CampfireConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneConfig(c.cfg)
}

func (c *Campfire) Snapshot() CampfireSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CampfireSnapshot{
		ProceduralState: c.snapshotStateLocked(),
		RNGState:        c.rng.State(),
	}
}

func (c *Campfire) RestoreSnapshot(snap CampfireSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		c.rng.SetState(snap.RNGState)
	}
}

func (c *Campfire) SnapshotPersistedState() CampfirePersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CampfirePersistedState{
		ProceduralState: c.snapshotStateLocked(),
		RNGState:        c.rng.State(),
	}
}

func (c *Campfire) RestorePersistedState(ps CampfirePersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		c.rng.SetState(ps.RNGState)
	}
}

func (c *Campfire) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   c.tick,
		Timers: cloneTimerMap(c.timers),
		Values: cloneValueMap(c.values),
	}
}

func (c *Campfire) restoreStateLocked(state ProceduralState) {
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

func (c *Campfire) CurrentTick() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tick
}

func (c *Campfire) PerturbRNG(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Mix(delta)
}

func (c *Campfire) DrainLog() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.log) == 0 {
		return nil
	}
	out := c.log
	c.log = nil
	return out
}

func (c *Campfire) appendLog(kind, desc string) {
	c.log = append(c.log, LogEntry{Tick: c.tick, Type: kind, Desc: desc})
	if len(c.log) > 200 {
		c.log = c.log[len(c.log)-200:]
	}
}

func (c *Campfire) intCfg(key string) int {
	return int(math.Round(c.cfg[key]))
}

func (c *Campfire) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "crackle":
		c.startCrackleLocked("triggered")
	case "lull":
		c.startLullLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", c.timers["intro"], c.cfg["intro_glow"]))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", c.intCfg("ending_dur"), c.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (c *Campfire) Step() {
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

func (c *Campfire) startCrackleLocked(verb string) {
	c.timers["crackle"] = jitterInt(c.rng, c.intCfg("crackle_dur"), 0.3)
	c.values["crackle_gain"] = c.cfg["crackle_mult"] * (0.75 + c.rng.Float64()*0.50)
	c.appendLog("crackle", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, c.timers["crackle"], c.values["crackle_gain"]))
}

func (c *Campfire) startLullLocked(verb string) {
	c.timers["lull"] = jitterInt(c.rng, c.intCfg("lull_dur"), 0.3)
	c.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, c.timers["lull"], c.cfg["lull_mult"]))
}

func (c *Campfire) startIntroLocked() {
	c.timers["crackle"] = 0
	c.timers["lull"] = 0
	c.timers["ending"] = 0
	c.values["crackle_gain"] = 1
	c.timers["intro"] = c.intCfg("intro_dur")
	c.values["intro_total"] = float64(c.timers["intro"])
}

func (c *Campfire) startEndingLocked() {
	c.timers["intro"] = 0
	c.timers["crackle"] = 0
	c.timers["lull"] = 0
	c.values["crackle_gain"] = 1
	endingTotal := c.intCfg("ending_dur") + max(0, c.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, c.intCfg("ending_dur"))
	}
	c.timers["ending"] = endingTotal
	c.values["ending_total"] = float64(endingTotal)
}

func (c *Campfire) stepLocked() {
	if c.timers["crackle"] <= 0 {
		c.values["crackle_gain"] = 1
	}
	if c.timers["intro"] <= 0 {
		delete(c.values, "intro_total")
	}
	if c.timers["ending"] <= 0 {
		delete(c.values, "ending_total")
	}
	if c.timers["intro"] > 0 || c.timers["ending"] > 0 {
		return
	}
	if c.timers["crackle"] <= 0 && c.cfg["crackle_p"] > 0 && c.rng.Float64() < c.cfg["crackle_p"] {
		c.startCrackleLocked("started")
	}
	if c.timers["lull"] <= 0 && c.cfg["lull_p"] > 0 && c.rng.Float64() < c.cfg["lull_p"] {
		c.startLullLocked("started")
	}
}
