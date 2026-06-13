package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type CottageChimneyConfig = ProceduralConfig

type CottageSmokePuff struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
	Age   int     `json:"age"`
	Life  int     `json:"life"`
	Size  float64 `json:"size"`
	Phase float64 `json:"phase"`
}

type CottageEmber struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
	Age   int     `json:"age"`
	Life  int     `json:"life"`
	Phase float64 `json:"phase"`
}

type CottageChimneyState struct {
	Tick      int                `json:"tick"`
	Puffs     []CottageSmokePuff `json:"puffs,omitempty"`
	Embers    []CottageEmber     `json:"embers,omitempty"`
	Timers    map[string]int     `json:"timers,omitempty"`
	Values    map[string]float64 `json:"values,omitempty"`
	Ended     bool               `json:"ended,omitempty"`
	Lifecycle Lifecycle          `json:"lifecycle"`
}

type CottageChimneySnapshot struct {
	CottageChimneyState
	RNGState uint64 `json:"rngState,omitempty"`
}

type CottageChimneyPersistedState struct {
	CottageChimneyState
	RNGState uint64 `json:"rngState"`
}

type CottageChimney struct {
	mu sync.Mutex

	W, H int

	rng    *rngutil.RNG
	cfg    CottageChimneyConfig
	tick   int
	puffs  []CottageSmokePuff
	embers []CottageEmber
	timers map[string]int
	values map[string]float64
	ended  bool
	log    []LogEntry
}

var cottageChimneyDefaults = CottageChimneyConfig{
	"intro_dur":        90,
	"first_puff_delay": 34,
	"ending_dur":       90,
	"ending_tail":      130,
	"final_puffs":      3,
	"puff_every":       30,
	"puff_life":        185,
	"puff_size":        2.35,
	"plume_width":      16,
	"rise":             0.145,
	"wind":             0.052,
	"wander":           0.34,
	"smoke_hue":        218,
	"smoke_sat":        0.16,
	"smoke_light":      0.45,
	"window_hue":       40,
	"window_sat":       0.86,
	"window_light":     0.68,
	"window_glow":      0.78,
	"sky_hue":          226,
	"sky_sat":          0.36,
	"sky_light":        0.065,
	"gust_p":           0,
	"flicker_p":        0,
	"ember_p":          0,
	"quiet_p":          0,
	"gust_dur":         95,
	"gust_strength":    0.22,
	"puff_scatter":     0.72,
	"flicker_dur":      42,
	"flicker_depth":    0.34,
	"ember_life":       78,
	"quiet_dur":        260,
	"quiet_mult":       0.38,
}

func CottageChimneySchema() EffectSchema {
	return EffectSchema{
		Name:           "cottage-chimney",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 90, Trigger: "intro",
				Description: "Ticks spent warming the window from a dark cottage into the steady home-light."},
			{Key: "first_puff_delay", Label: "first puff", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 34,
				Description: "Delay before the first smoke puff during intro, so the plume begins after the lamp is lit."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 90, Trigger: "ending",
				Description: "Ticks spent dimming the window toward the terminal dark-cottage resting state."},
			{Key: "ending_tail", Label: "tail length", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 130,
				Description: "Extra time for the final chimney puffs to drift and fade after the lamp goes out."},
			{Key: "final_puffs", Label: "final puffs", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 8, Step: 1, Default: 3,
				Description: "Number of last puffs released as the cottage settles into night."},
			{Key: "puff_every", Label: "puff every", Slot: SlotSpawn, Group: "smoke", Type: KnobInt, Min: 12, Max: 90, Step: 1, Default: 30, Trigger: "puff-emit",
				Description: "Steady-state cadence between slow chimney puffs."},
			{Key: "puff_life", Label: "puff life", Slot: SlotSpawn, Group: "smoke", Type: KnobInt, Min: 70, Max: 320, Step: 5, Default: 185,
				Description: "How long each smoke puff survives before fading out near the upper edge."},
			{Key: "puff_size", Label: "puff size", Slot: SlotSpawn, Group: "smoke", Type: KnobFloat, Min: 1, Max: 5, Step: 0.05, Default: 2.35,
				Description: "Base radius of each emitted puff at low resolution."},
			{Key: "plume_width", Label: "plume width", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 4, Max: 34, Step: 0.5, Default: 16,
				Description: "Horizontal spread of the chimney plume as puffs age."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.04, Max: 0.32, Step: 0.005, Default: 0.145,
				Description: "Upward speed of smoke puffs. Lower values feel sleepy and meditative."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.18, Max: 0.24, Step: 0.005, Default: 0.052,
				Description: "Constant sideways drift bias for the plume."},
			{Key: "wander", Label: "wander", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.01, Default: 0.34,
				Description: "Small sine-wave meander applied as puffs rise."},
			{Key: "smoke_hue", Label: "smoke hue", Slot: SlotLever, Group: "smoke color", Type: KnobFloat, Min: 190, Max: 260, Step: 1, Default: 218,
				Description: "Cool night hue mixed into the smoke."},
			{Key: "smoke_sat", Label: "smoke sat", Slot: SlotLever, Group: "smoke color", Type: KnobFloat, Min: 0, Max: 0.42, Step: 0.01, Default: 0.16,
				Description: "Smoke saturation. Lower values read as soft gray."},
			{Key: "smoke_light", Label: "smoke light", Slot: SlotLever, Group: "smoke color", Type: KnobFloat, Min: 0.2, Max: 0.75, Step: 0.01, Default: 0.45,
				Description: "Smoke lightness before fade is applied."},
			{Key: "window_hue", Label: "window hue", Slot: SlotLever, Group: "window palette", Type: KnobFloat, Min: 26, Max: 220, Step: 1, Default: 40,
				Description: "Scene-palette knob for the lit window: amber, yellow, or cold blue."},
			{Key: "window_sat", Label: "window sat", Slot: SlotLever, Group: "window palette", Type: KnobFloat, Min: 0.12, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Saturation of the window glow."},
			{Key: "window_light", Label: "window light", Slot: SlotLever, Group: "window palette", Type: KnobFloat, Min: 0.28, Max: 0.92, Step: 0.01, Default: 0.68,
				Description: "Window lightness at full steady glow."},
			{Key: "window_glow", Label: "window glow", Slot: SlotLever, Group: "window palette", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Size and intensity of the warm halo around the window."},
			{Key: "sky_hue", Label: "sky hue", Slot: SlotLever, Group: "night sky", Type: KnobFloat, Min: 205, Max: 255, Step: 1, Default: 226,
				Description: "Base hue of the static night sky behind the cottage."},
			{Key: "sky_sat", Label: "sky sat", Slot: SlotLever, Group: "night sky", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.36,
				Description: "Saturation of the quiet night backdrop."},
			{Key: "sky_light", Label: "sky light", Slot: SlotLever, Group: "night sky", Type: KnobFloat, Min: 0.025, Max: 0.16, Step: 0.005, Default: 0.065,
				Description: "Overall night-sky lightness."},
			{Key: "gust_p", Label: "wind gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.00025, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance of a brief sideways gust bending the smoke plume."},
			{Key: "flicker_p", Label: "lamp flicker", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.00025, Default: 0, Trigger: "lamp-flicker",
				Description: "Per-tick chance of the window glow dipping and recovering."},
			{Key: "ember_p", Label: "embers", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.006, Step: 0.00025, Default: 0, Trigger: "embers",
				Description: "Per-tick chance of a rare bright spark drifting up with a puff."},
			{Key: "quiet_p", Label: "quiet night", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.006, Step: 0.00025, Default: 0, Trigger: "quiet-night",
				Description: "Per-tick chance of a long, thinner-emission quiet window."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind-gust", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 95,
				Description: "Duration of a wind-gust pulse."},
			{Key: "gust_strength", Label: "gust str", Slot: SlotEventMod, Group: "wind-gust", Type: KnobFloat, Min: 0.04, Max: 0.55, Step: 0.01, Default: 0.22,
				Description: "Extra drift bias applied at the gust peak."},
			{Key: "puff_scatter", Label: "scatter", Slot: SlotEventMod, Group: "puff-emit", Type: KnobFloat, Min: 0, Max: 2.2, Step: 0.05, Default: 0.72,
				Description: "Spawn-time spread and velocity variation for individual puffs."},
			{Key: "flicker_dur", Label: "flicker dur", Slot: SlotEventMod, Group: "lamp-flicker", Type: KnobInt, Min: 8, Max: 120, Step: 4, Default: 42,
				Description: "Duration of a window-glow dip and recovery."},
			{Key: "flicker_depth", Label: "flicker depth", Slot: SlotEventMod, Group: "lamp-flicker", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.34,
				Description: "How far the window glow dips during a lamp-flicker event."},
			{Key: "ember_life", Label: "ember life", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 20, Max: 160, Step: 5, Default: 78,
				Description: "Lifetime of rare sparks rising in the smoke."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-night", Type: KnobInt, Min: 80, Max: 520, Step: 10, Default: 260,
				Description: "Duration of a thin-smoke quiet-night suppression window."},
			{Key: "quiet_mult", Label: "quiet mult", Slot: SlotEventMod, Group: "quiet-night", Type: KnobFloat, Min: 0.1, Max: 0.85, Step: 0.05, Default: 0.38,
				Description: "Emission multiplier during quiet-night. Lower values create wider gaps."},
		},
	}
}

func defaultCottageChimneyConfig() CottageChimneyConfig {
	return cloneConfig(cottageChimneyDefaults)
}

func mergeCottageChimneyDefaults(cfg CottageChimneyConfig) CottageChimneyConfig {
	out := defaultCottageChimneyConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = cottageChimneyDefaults["intro_dur"]
	}
	if out["first_puff_delay"] < 0 {
		out["first_puff_delay"] = 0
	}
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = cottageChimneyDefaults["ending_dur"]
	}
	if out["ending_tail"] < 0 {
		out["ending_tail"] = 0
	}
	if out["final_puffs"] < 0 {
		out["final_puffs"] = 0
	}
	if out["puff_every"] <= 0 {
		out["puff_every"] = cottageChimneyDefaults["puff_every"]
	}
	if out["puff_life"] <= 0 {
		out["puff_life"] = cottageChimneyDefaults["puff_life"]
	}
	if out["puff_size"] <= 0 {
		out["puff_size"] = cottageChimneyDefaults["puff_size"]
	}
	if out["plume_width"] <= 0 {
		out["plume_width"] = cottageChimneyDefaults["plume_width"]
	}
	if out["rise"] <= 0 {
		out["rise"] = cottageChimneyDefaults["rise"]
	}
	if out["wander"] < 0 {
		out["wander"] = 0
	}
	if out["smoke_hue"] == 0 {
		out["smoke_hue"] = cottageChimneyDefaults["smoke_hue"]
	}
	out["smoke_sat"] = clamp01(out["smoke_sat"])
	if out["smoke_light"] <= 0 {
		out["smoke_light"] = cottageChimneyDefaults["smoke_light"]
	}
	out["smoke_light"] = clamp01(out["smoke_light"])
	if out["window_hue"] == 0 {
		out["window_hue"] = cottageChimneyDefaults["window_hue"]
	}
	if out["window_sat"] <= 0 {
		out["window_sat"] = cottageChimneyDefaults["window_sat"]
	}
	out["window_sat"] = clamp01(out["window_sat"])
	if out["window_light"] <= 0 {
		out["window_light"] = cottageChimneyDefaults["window_light"]
	}
	out["window_light"] = clamp01(out["window_light"])
	if out["window_glow"] <= 0 {
		out["window_glow"] = cottageChimneyDefaults["window_glow"]
	}
	out["window_glow"] = clamp01(out["window_glow"])
	if out["sky_hue"] == 0 {
		out["sky_hue"] = cottageChimneyDefaults["sky_hue"]
	}
	if out["sky_sat"] <= 0 {
		out["sky_sat"] = cottageChimneyDefaults["sky_sat"]
	}
	out["sky_sat"] = clamp01(out["sky_sat"])
	if out["sky_light"] <= 0 {
		out["sky_light"] = cottageChimneyDefaults["sky_light"]
	}
	out["sky_light"] = clamp01(out["sky_light"])
	if out["gust_p"] < 0 {
		out["gust_p"] = 0
	}
	if out["flicker_p"] < 0 {
		out["flicker_p"] = 0
	}
	if out["ember_p"] < 0 {
		out["ember_p"] = 0
	}
	if out["quiet_p"] < 0 {
		out["quiet_p"] = 0
	}
	if out["gust_dur"] <= 0 {
		out["gust_dur"] = cottageChimneyDefaults["gust_dur"]
	}
	if out["gust_strength"] < 0 {
		out["gust_strength"] = 0
	}
	if out["puff_scatter"] < 0 {
		out["puff_scatter"] = 0
	}
	if out["flicker_dur"] <= 0 {
		out["flicker_dur"] = cottageChimneyDefaults["flicker_dur"]
	}
	if out["flicker_depth"] < 0 {
		out["flicker_depth"] = 0
	}
	out["flicker_depth"] = clamp01(out["flicker_depth"])
	if out["ember_life"] <= 0 {
		out["ember_life"] = cottageChimneyDefaults["ember_life"]
	}
	if out["quiet_dur"] <= 0 {
		out["quiet_dur"] = cottageChimneyDefaults["quiet_dur"]
	}
	if out["quiet_mult"] <= 0 {
		out["quiet_mult"] = cottageChimneyDefaults["quiet_mult"]
	}
	out["quiet_mult"] = clamp01(out["quiet_mult"])
	return out
}

func NewCottageChimney(w, h int, seed int64, cfg CottageChimneyConfig) *CottageChimney {
	c := &CottageChimney{
		W:      w,
		H:      h,
		rng:    rngutil.New(seed),
		cfg:    mergeCottageChimneyDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
	c.timers["puff"] = 1
	return c
}

func (c *CottageChimney) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.W = w
	c.H = h
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

func (c *CottageChimney) Snapshot() CottageChimneySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CottageChimneySnapshot{
		CottageChimneyState: c.snapshotStateLocked(),
		RNGState:            c.rng.State(),
	}
}

func (c *CottageChimney) RestoreSnapshot(s CottageChimneySnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CottageChimneyState)
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

func (c *CottageChimney) SnapshotPersistedState() CottageChimneyPersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CottageChimneyPersistedState{
		CottageChimneyState: c.snapshotStateLocked(),
		RNGState:            c.rng.State(),
	}
}

func (c *CottageChimney) RestorePersistedState(s CottageChimneyPersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CottageChimneyState)
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

func (c *CottageChimney) snapshotStateLocked() CottageChimneyState {
	puffs := make([]CottageSmokePuff, len(c.puffs))
	copy(puffs, c.puffs)
	embers := make([]CottageEmber, len(c.embers))
	copy(embers, c.embers)
	return CottageChimneyState{
		Tick:      c.tick,
		Puffs:     puffs,
		Embers:    embers,
		Timers:    cloneTimerMap(c.timers),
		Values:    cloneValueMap(c.values),
		Ended:     c.ended,
		Lifecycle: c.lifecycleLocked(),
	}
}

func (c *CottageChimney) lifecycleLocked() Lifecycle {
	switch {
	case c.timers["intro"] > 0:
		return LifecycleIntro
	case c.timers["ending"] > 0:
		return LifecycleEnding
	case c.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (c *CottageChimney) restoreStateLocked(s CottageChimneyState) {
	c.tick = s.Tick
	c.puffs = append(c.puffs[:0], s.Puffs...)
	c.embers = append(c.embers[:0], s.Embers...)
	c.timers = cloneTimerMap(s.Timers)
	if c.timers == nil {
		c.timers = make(map[string]int)
	}
	c.values = cloneValueMap(s.Values)
	if c.values == nil {
		c.values = make(map[string]float64)
	}
	c.ended = s.Ended
}

func (c *CottageChimney) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "puff-emit":
		c.emitPuffLocked("triggered")
		c.timers["puff"] = c.nextPuffDelayLocked()
	case "wind-gust":
		c.startWindGustLocked("triggered")
	case "lamp-flicker":
		c.startLampFlickerLocked("triggered")
	case "embers":
		c.emitPuffLocked("spark")
		c.emitEmberLocked()
		c.appendLog("embers", "spark released with chimney puff")
	case "quiet-night":
		c.startQuietNightLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, first puff=%d)", c.timers["intro"], c.intCfg("first_puff_delay")))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, tail=%d, final=%d)", c.intCfg("ending_dur"), c.intCfg("ending_tail"), c.intCfg("final_puffs")))
	default:
		return false
	}
	return true
}

func (c *CottageChimney) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++
	c.decrementTimersLocked()
	c.advanceParticlesLocked()
	c.stepLifecycleLocked()

	if c.ended || c.timers["ending"] > 0 {
		return
	}
	if c.timers["intro"] <= 0 {
		c.rollAmbientEventsLocked()
	}
	c.maybeEmitPuffLocked()
}

func (c *CottageChimney) intCfg(key string) int {
	return int(math.Round(c.cfg[key]))
}

func (c *CottageChimney) decrementTimersLocked() {
	for key, left := range c.timers {
		if left <= 1 {
			delete(c.timers, key)
			continue
		}
		c.timers[key] = left - 1
	}
}

func (c *CottageChimney) stepLifecycleLocked() {
	if c.timers["intro"] <= 0 {
		delete(c.values, "intro_total")
	}
	if c.timers["ending"] > 0 {
		c.emitFinalPuffsLocked()
		return
	}
	if _, wasEnding := c.values["ending_total"]; wasEnding {
		c.ended = true
		c.puffs = nil
		c.embers = nil
		delete(c.values, "ending_total")
		delete(c.values, "ending_fade")
		delete(c.values, "final_left")
		delete(c.timers, "final_gap")
		c.appendLog("ended", "window dark, smoke tail gone")
	}
}

func (c *CottageChimney) rollAmbientEventsLocked() {
	if c.cfg["gust_p"] > 0 && c.timers["wind_gust"] <= 0 && c.rng.Float64() < c.cfg["gust_p"] {
		c.startWindGustLocked("started")
	}
	if c.cfg["flicker_p"] > 0 && c.timers["lamp_flicker"] <= 0 && c.rng.Float64() < c.cfg["flicker_p"] {
		c.startLampFlickerLocked("started")
	}
	if c.cfg["ember_p"] > 0 && c.rng.Float64() < c.cfg["ember_p"] {
		c.emitEmberLocked()
		c.appendLog("embers", "spark rolled")
	}
	if c.cfg["quiet_p"] > 0 && c.timers["quiet_night"] <= 0 && c.rng.Float64() < c.cfg["quiet_p"] {
		c.startQuietNightLocked("started")
	}
}

func (c *CottageChimney) maybeEmitPuffLocked() {
	if c.timers["puff"] > 0 {
		return
	}
	if c.timers["intro"] > 0 && c.lampLevelLocked() < 0.58 {
		c.timers["puff"] = 2
		return
	}
	c.emitPuffLocked("emitted")
	c.timers["puff"] = c.nextPuffDelayLocked()
}

func (c *CottageChimney) nextPuffDelayLocked() int {
	every := max(1, c.intCfg("puff_every"))
	if c.timers["quiet_night"] > 0 {
		mult := math.Max(0.1, c.cfg["quiet_mult"])
		every = int(math.Round(float64(every) / mult))
	}
	jitter := 0.18
	if c.timers["wind_gust"] > 0 {
		jitter = 0.28
	}
	return jitterInt(c.rng, every, jitter)
}

func (c *CottageChimney) startIntroLocked() {
	c.ended = false
	c.puffs = nil
	c.embers = nil
	c.timers = make(map[string]int)
	c.values = make(map[string]float64)
	total := max(1, c.intCfg("intro_dur"))
	c.timers["intro"] = total
	c.values["intro_total"] = float64(total)
	c.timers["puff"] = max(0, c.intCfg("first_puff_delay"))
}

func (c *CottageChimney) startEndingLocked() {
	c.timers["intro"] = 0
	c.timers["wind_gust"] = 0
	c.timers["lamp_flicker"] = 0
	c.timers["quiet_night"] = 0
	delete(c.values, "intro_total")
	delete(c.values, "gust_total")
	delete(c.values, "flicker_total")
	delete(c.values, "quiet_total")
	c.ended = false
	fade := max(1, c.intCfg("ending_dur"))
	tail := max(0, c.intCfg("ending_tail"))
	total := max(1, fade+tail)
	c.timers["ending"] = total
	c.values["ending_total"] = float64(total)
	c.values["ending_fade"] = float64(fade)
	finalPuffs := max(0, c.intCfg("final_puffs"))
	if finalPuffs > 0 {
		c.emitPuffLocked("final")
		c.values["final_left"] = float64(finalPuffs - 1)
		c.timers["final_gap"] = max(6, c.intCfg("puff_every")/2)
	} else {
		c.values["final_left"] = 0
	}
}

func (c *CottageChimney) emitFinalPuffsLocked() {
	left := int(math.Round(c.values["final_left"]))
	if left <= 0 || c.timers["final_gap"] > 0 {
		return
	}
	c.emitPuffLocked("final")
	c.values["final_left"] = float64(left - 1)
	c.timers["final_gap"] = max(6, c.intCfg("puff_every")/2)
}

func (c *CottageChimney) startWindGustLocked(verb string) {
	dur := jitterInt(c.rng, max(1, c.intCfg("gust_dur")), 0.25)
	dir := 1.0
	if c.rng.Float64() < 0.35 {
		dir = -1
	}
	strength := c.cfg["gust_strength"] * (0.75 + c.rng.Float64()*0.5) * dir
	c.timers["wind_gust"] = dur
	c.values["gust_total"] = float64(dur)
	c.values["gust_strength"] = strength
	c.appendLog("wind-gust", fmt.Sprintf("%s (dur=%d, drift=%+.2f)", verb, dur, strength))
}

func (c *CottageChimney) startLampFlickerLocked(verb string) {
	dur := jitterInt(c.rng, max(1, c.intCfg("flicker_dur")), 0.25)
	depth := clamp01(c.cfg["flicker_depth"] * (0.8 + c.rng.Float64()*0.4))
	c.timers["lamp_flicker"] = dur
	c.values["flicker_total"] = float64(dur)
	c.values["flicker_depth"] = depth
	c.appendLog("lamp-flicker", fmt.Sprintf("%s (dur=%d, depth=%.2f)", verb, dur, depth))
}

func (c *CottageChimney) startQuietNightLocked(verb string) {
	dur := jitterInt(c.rng, max(1, c.intCfg("quiet_dur")), 0.2)
	c.timers["quiet_night"] = dur
	c.values["quiet_total"] = float64(dur)
	c.appendLog("quiet-night", fmt.Sprintf("%s (dur=%d, emission=%.2f)", verb, dur, c.cfg["quiet_mult"]))
}

func (c *CottageChimney) emitPuffLocked(verb string) {
	x, y := cottageChimneyMouth(c.W, c.H)
	scatter := c.cfg["puff_scatter"]
	if c.timers["wind_gust"] > 0 {
		scatter *= 1.35
	}
	life := jitterInt(c.rng, max(1, c.intCfg("puff_life")), 0.18)
	size := c.cfg["puff_size"] * (0.78 + c.rng.Float64()*0.46)
	p := CottageSmokePuff{
		X:     float64(x) + (c.rng.Float64()*2-1)*scatter,
		Y:     float64(y),
		VX:    (c.rng.Float64()*2 - 1) * scatter * 0.018,
		VY:    -c.cfg["rise"] * (0.82 + c.rng.Float64()*0.36),
		Age:   0,
		Life:  life,
		Size:  size,
		Phase: c.rng.Float64() * math.Pi * 2,
	}
	c.puffs = append(c.puffs, p)
	if len(c.puffs) > 80 {
		c.puffs = append([]CottageSmokePuff(nil), c.puffs[len(c.puffs)-80:]...)
	}
	if verb != "" {
		c.appendLog("puff-emit", fmt.Sprintf("%s (life=%d)", verb, life))
	}
}

func (c *CottageChimney) emitEmberLocked() {
	x, y := cottageChimneyMouth(c.W, c.H)
	life := jitterInt(c.rng, max(1, c.intCfg("ember_life")), 0.28)
	e := CottageEmber{
		X:     float64(x) + (c.rng.Float64()*2-1)*1.4,
		Y:     float64(y) + c.rng.Float64()*1.2,
		VX:    c.currentWindLocked()*0.45 + (c.rng.Float64()*2-1)*0.035,
		VY:    -c.cfg["rise"] * (1.15 + c.rng.Float64()*0.8),
		Age:   0,
		Life:  life,
		Phase: c.rng.Float64() * math.Pi * 2,
	}
	c.embers = append(c.embers, e)
	if len(c.embers) > 24 {
		c.embers = append([]CottageEmber(nil), c.embers[len(c.embers)-24:]...)
	}
}

func (c *CottageChimney) advanceParticlesLocked() {
	wind := c.currentWindLocked()
	alivePuffs := c.puffs[:0]
	for _, p := range c.puffs {
		p.Age++
		ageT := 0.0
		if p.Life > 0 {
			ageT = clamp01(float64(p.Age) / float64(p.Life))
		}
		wander := math.Sin(float64(c.tick)*0.025+p.Phase+float64(p.Age)*0.04) * c.cfg["wander"] * (0.08 + ageT*0.22)
		p.X += p.VX + wind*(0.38+ageT*0.9) + wander
		p.Y += p.VY * (1 - 0.32*ageT)
		p.Size += 0.004 + c.cfg["plume_width"]*0.00065
		if p.Age < p.Life && p.Y > -p.Size*3 && p.X > -p.Size*5 && p.X < float64(c.W)+p.Size*5 {
			alivePuffs = append(alivePuffs, p)
		}
	}
	c.puffs = alivePuffs

	aliveEmbers := c.embers[:0]
	for _, e := range c.embers {
		e.Age++
		e.X += e.VX + wind*0.18 + math.Sin(e.Phase+float64(e.Age)*0.11)*0.035
		e.Y += e.VY
		if e.Age < e.Life && e.Y >= 0 && e.X >= -2 && e.X < float64(c.W)+2 {
			aliveEmbers = append(aliveEmbers, e)
		}
	}
	c.embers = aliveEmbers
}

func (c *CottageChimney) currentWindLocked() float64 {
	wind := c.cfg["wind"]
	if c.timers["wind_gust"] > 0 {
		total := int(math.Round(c.values["gust_total"]))
		progress := proceduralPhaseProgress(total, c.timers["wind_gust"])
		wind += c.values["gust_strength"] * math.Sin(progress*math.Pi)
	}
	return wind
}

func (c *CottageChimney) lampLevelLocked() float64 {
	return cottageLampLevel(c.tick, c.cfg, c.timers, c.values, c.ended)
}

func (c *CottageChimney) GridCopy() [][]Pixel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return renderCottageChimneyFrame(c.W, c.H, c.tick, c.cfg, c.timers, c.values, c.puffs, c.embers, c.ended)
}

func renderCottageChimneyFrame(w, h, tick int, cfg CottageChimneyConfig, timers map[string]int, values map[string]float64, puffs []CottageSmokePuff, embers []CottageEmber, ended bool) [][]Pixel {
	grid := make([][]Pixel, h)
	for y := range grid {
		grid[y] = make([]Pixel, w)
	}
	if w <= 0 || h <= 0 {
		return grid
	}
	cfg = mergeCottageChimneyDefaults(cfg)
	skyHue := math.Mod(cfg["sky_hue"]+360, 360)
	skySat := clamp01(cfg["sky_sat"])
	top := hslToRGB(skyHue, skySat, clamp01(cfg["sky_light"]*0.62))
	bottom := hslToRGB(math.Mod(skyHue-8+360, 360), skySat*0.9, clamp01(cfg["sky_light"]*1.45))
	for y := 0; y < h; y++ {
		t := 0.0
		if h > 1 {
			t = float64(y) / float64(h-1)
		}
		c := mixColor(top, bottom, t)
		for x := 0; x < w; x++ {
			grid[y][x] = Pixel{Filled: true, C: c}
		}
	}

	paintCottageStars(grid, tick)
	baseY := cottageBaseY(w, h)
	ground := hslToRGB(math.Mod(skyHue+122, 360), 0.18, clamp01(cfg["sky_light"]*0.54))
	for y := baseY; y < h; y++ {
		shade := mixColor(ground, color.RGBA{R: 0, G: 0, B: 0, A: 255}, float64(y-baseY)/float64(max(1, h-baseY))*0.45)
		for x := 0; x < w; x++ {
			grid[y][x] = Pixel{Filled: true, C: shade}
		}
	}

	smoke := hslToRGB(cfg["smoke_hue"], clamp01(cfg["smoke_sat"]), clamp01(cfg["smoke_light"]))
	for _, p := range puffs {
		paintCottageSmokePuff(grid, p, smoke, cfg["plume_width"])
	}

	paintCottageSilhouette(grid, cfg, tick, timers, values, ended)
	for _, e := range embers {
		paintCottageEmber(grid, e)
	}
	return grid
}

func cottageLampLevel(tick int, cfg CottageChimneyConfig, timers map[string]int, values map[string]float64, ended bool) float64 {
	if ended {
		return 0
	}
	level := 1.0
	if timers["intro"] > 0 {
		total := int(math.Round(values["intro_total"]))
		p := proceduralPhaseProgress(total, timers["intro"])
		level = p * p * (3 - 2*p)
	}
	if timers["ending"] > 0 {
		total := int(math.Round(values["ending_total"]))
		fade := int(math.Round(values["ending_fade"]))
		elapsed := total - timers["ending"]
		if fade <= 0 || elapsed >= fade {
			level = 0
		} else {
			p := clamp01(float64(elapsed) / float64(fade))
			p = p * p * (3 - 2*p)
			level *= 1 - p
		}
	}
	if timers["lamp_flicker"] > 0 {
		total := int(math.Round(values["flicker_total"]))
		p := proceduralPhaseProgress(total, timers["lamp_flicker"])
		env := math.Sin(p * math.Pi)
		flutter := 0.68 + 0.32*math.Sin(float64(tick)*0.71)
		level *= 1 - clamp01(values["flicker_depth"])*env*flutter
	}
	return clamp01(level)
}

func cottageBaseY(w, h int) int {
	if h <= 0 {
		return 0
	}
	return max(1, h-max(4, h/10))
}

func cottageGeometry(w, h int) (bodyLeft, bodyTop, bodyRight, baseY, roofPeakY, chimneyLeft, chimneyTop, chimneyW, chimneyH int) {
	baseY = cottageBaseY(w, h)
	bodyW := clampInt(int(math.Round(float64(w)*0.42)), 18, max(18, w-8))
	bodyH := clampInt(int(math.Round(float64(h)*0.25)), 8, max(8, h/3))
	center := w / 2
	bodyLeft = center - bodyW/2
	bodyRight = bodyLeft + bodyW
	bodyTop = baseY - bodyH
	roofH := clampInt(int(math.Round(float64(h)*0.16)), 5, max(5, h/4))
	roofPeakY = bodyTop - roofH
	chimneyW = max(2, bodyW/12)
	chimneyH = max(5, roofH)
	chimneyLeft = center - bodyW/4
	chimneyTop = roofPeakY + max(1, roofH/5)
	return
}

func cottageChimneyMouth(w, h int) (x, y int) {
	_, _, _, _, _, chimneyLeft, chimneyTop, chimneyW, _ := cottageGeometry(w, h)
	return chimneyLeft + chimneyW/2, chimneyTop - 1
}

func paintCottageStars(grid [][]Pixel, tick int) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	limit := int(float64(h) * 0.58)
	for y := 1; y < limit; y++ {
		for x := 0; x < w; x++ {
			hash := uint32(x*73856093) ^ uint32(y*19349663) ^ 0x6a09e667
			if hash%137 != 0 {
				continue
			}
			twinkle := 0.62 + 0.22*math.Sin(float64(tick)*0.015+float64(hash%53))
			star := color.RGBA{
				R: uint8(128 + (hash % 72)),
				G: uint8(132 + (hash % 68)),
				B: uint8(152 + (hash % 84)),
				A: 255,
			}
			blendCottagePixel(grid, x, y, star, 0.22*twinkle)
		}
	}
}

func paintCottageSmokePuff(grid [][]Pixel, p CottageSmokePuff, smoke color.RGBA, plumeWidth float64) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	if p.Life <= 0 {
		return
	}
	ageT := clamp01(float64(p.Age) / float64(p.Life))
	fade := math.Pow(1-ageT, 0.72)
	if p.Y < float64(h)*0.18 {
		fade *= clamp01(p.Y / math.Max(1, float64(h)*0.18))
	}
	if fade <= 0.01 {
		return
	}
	rx := math.Max(1.4, p.Size*(0.72+ageT*1.25)+plumeWidth*ageT*0.055)
	ry := math.Max(1.2, p.Size*(0.62+ageT*1.05))
	minX := int(math.Floor(p.X - rx - 1))
	maxX := int(math.Ceil(p.X + rx + 1))
	minY := int(math.Floor(p.Y - ry - 1))
	maxY := int(math.Ceil(p.Y + ry + 1))
	for y := minY; y <= maxY; y++ {
		if y < 0 || y >= h {
			continue
		}
		for x := minX; x <= maxX; x++ {
			if x < 0 || x >= w {
				continue
			}
			nx := (float64(x) - p.X) / rx
			ny := (float64(y) - p.Y) / ry
			d2 := nx*nx + ny*ny
			if d2 > 1.18 {
				continue
			}
			core := 1 - d2/1.18
			strength := clamp01((0.16 + 0.42*fade) * (0.36 + 0.64*core))
			blendCottagePixel(grid, x, y, smoke, strength)
		}
	}
}

func paintCottageSilhouette(grid [][]Pixel, cfg CottageChimneyConfig, tick int, timers map[string]int, values map[string]float64, ended bool) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	bodyLeft, bodyTop, bodyRight, baseY, roofPeakY, chimneyLeft, chimneyTop, chimneyW, chimneyH := cottageGeometry(w, h)
	skyHue := cfg["sky_hue"]
	body := hslToRGB(math.Mod(skyHue+18, 360), 0.18, clamp01(cfg["sky_light"]*0.43))
	roof := hslToRGB(math.Mod(skyHue+10, 360), 0.22, clamp01(cfg["sky_light"]*0.30))
	edge := hslToRGB(math.Mod(skyHue+20, 360), 0.12, clamp01(cfg["sky_light"]*0.72))

	fillRect(grid, chimneyLeft, chimneyTop, chimneyW, chimneyH, roof)
	if chimneyTop >= 0 {
		fillRect(grid, chimneyLeft-1, chimneyTop, chimneyW+2, 1, edge)
	}

	fillRect(grid, bodyLeft, bodyTop, bodyRight-bodyLeft, baseY-bodyTop, body)
	if bodyTop >= 0 {
		fillRect(grid, bodyLeft, bodyTop, bodyRight-bodyLeft, 1, edge)
	}
	roofSpan := bodyRight - bodyLeft + max(4, (bodyRight-bodyLeft)/5)
	center := (bodyLeft + bodyRight) / 2
	for y := roofPeakY; y <= bodyTop+1; y++ {
		if y < 0 || y >= h {
			continue
		}
		p := 0.0
		if bodyTop+1 > roofPeakY {
			p = float64(y-roofPeakY) / float64(bodyTop+1-roofPeakY)
		}
		half := int(math.Round(float64(roofSpan) * 0.5 * p))
		for x := center - half; x <= center+half; x++ {
			paintPixel(grid, x, y, roof)
		}
	}

	doorW := max(3, (bodyRight-bodyLeft)/8)
	doorH := max(5, (baseY-bodyTop)*2/3)
	doorX := bodyLeft + max(2, (bodyRight-bodyLeft)/7)
	doorY := baseY - doorH
	door := hslToRGB(math.Mod(skyHue+30, 360), 0.16, clamp01(cfg["sky_light"]*0.22))
	fillRect(grid, doorX, doorY, doorW, doorH, door)
	if doorH > 4 {
		paintPixel(grid, doorX+doorW-1, doorY+doorH/2, edge)
	}

	level := cottageLampLevel(tick, cfg, timers, values, ended)
	if level <= 0.01 {
		return
	}
	winW := max(4, (bodyRight-bodyLeft)/7)
	winH := max(3, (baseY-bodyTop)/3)
	winX := bodyLeft + (bodyRight-bodyLeft)*58/100
	winY := bodyTop + (baseY-bodyTop)*34/100
	window := hslToRGB(cfg["window_hue"], clamp01(cfg["window_sat"]), clamp01(0.12+cfg["window_light"]*level))
	glow := hslToRGB(cfg["window_hue"], clamp01(cfg["window_sat"]*0.82), clamp01(cfg["window_light"]*0.86))
	glowR := int(math.Round(float64(max(winW, winH)) * (1.4 + cfg["window_glow"]*1.4)))
	for y := winY - glowR; y <= winY+winH+glowR; y++ {
		for x := winX - glowR; x <= winX+winW+glowR; x++ {
			cx := float64(winX) + float64(winW-1)/2
			cy := float64(winY) + float64(winH-1)/2
			d := math.Hypot((float64(x)-cx)/float64(max(1, glowR)), (float64(y)-cy)/float64(max(1, glowR)))
			if d > 1 {
				continue
			}
			strength := cfg["window_glow"] * level * math.Pow(1-d, 1.5) * 0.62
			blendCottagePixel(grid, x, y, glow, strength)
		}
	}
	fillRect(grid, winX, winY, winW, winH, window)
	bar := mixColor(window, color.RGBA{R: 0, G: 0, B: 0, A: 255}, 0.48)
	if winW >= 5 {
		fillRect(grid, winX+winW/2, winY, 1, winH, bar)
	}
	if winH >= 4 {
		fillRect(grid, winX, winY+winH/2, winW, 1, bar)
	}
}

func paintCottageEmber(grid [][]Pixel, e CottageEmber) {
	if e.Life <= 0 {
		return
	}
	ageT := clamp01(float64(e.Age) / float64(e.Life))
	strength := math.Pow(1-ageT, 0.8)
	if strength <= 0 {
		return
	}
	c := hslToRGB(34+math.Sin(e.Phase)*8, 0.9, 0.66+0.16*strength)
	x := int(math.Round(e.X))
	y := int(math.Round(e.Y))
	blendCottagePixel(grid, x, y, c, 0.92*strength)
	halo := hslToRGB(28, 0.75, 0.48)
	blendCottagePixel(grid, x-1, y, halo, 0.24*strength)
	blendCottagePixel(grid, x+1, y, halo, 0.24*strength)
	blendCottagePixel(grid, x, y-1, halo, 0.18*strength)
}

func fillRect(grid [][]Pixel, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			paintPixel(grid, xx, yy, c)
		}
	}
}

func blendCottagePixel(grid [][]Pixel, x, y int, c color.RGBA, strength float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	strength = clamp01(strength)
	if strength <= 0 {
		return
	}
	base := grid[y][x].C
	grid[y][x] = Pixel{Filled: true, C: mixColor(base, c, strength)}
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
