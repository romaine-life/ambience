package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// CaveCrystals is a slow-arc effect: in a dim cave, crystal nuclei seed at
// random points along an uneven floor and grow upward in discrete steps over
// the lifetime of a scene. The server is authoritative about *when* nuclei
// appear, *when* a step fires, and *which* crystal pops in fully formed.
// Clients run a local replica so the angular silhouettes stay in sync; the
// inner sparkle noise drifts per client.
//
// The long-arc nature is the design point: nucleation is rare, growth steps
// happen on a slow cadence, and the field fills out gradually across the
// whole scene before resetting via the ending lifecycle.

// Crystal is one growing crystal along the cave floor.
type Crystal struct {
	Col     int     `json:"col"`
	Anchor  int     `json:"anchor"`
	Step    int     `json:"step"`
	MaxStep int     `json:"max_step"`
	Hue     float64 `json:"hue"`
	Tilt    float64 `json:"tilt"`
	Phase   int     `json:"phase"`
}

// CaveCrystalsConfig tunes the cave-crystals effect.
type CaveCrystalsConfig struct {
	// INTRODUCTION
	IntroDur     int `json:"intro_dur"`
	IntroCluster int `json:"intro_cluster"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingDim    float64 `json:"ending_dim"`
	// LEVERS — field
	MaxCrystals int     `json:"max_crystals"`
	Floor       float64 `json:"floor"`
	FloorRough  float64 `json:"floor_rough"`
	// LEVERS — growth
	NucleateEvery int     `json:"nucleate_every"`
	GrowthEvery   int     `json:"growth_every"`
	GrowthSteps   int     `json:"growth_steps"`
	StepHeight    float64 `json:"step_height"`
	BaseWidth     float64 `json:"base_width"`
	Tilt          float64 `json:"tilt"`
	Sparkle       float64 `json:"sparkle"`
	// LEVERS — color
	Hue       float64 `json:"hue"`
	HueSpread float64 `json:"hue_sp"`
	Sat       float64 `json:"sat"`
	LightMin  float64 `json:"lmin"`
	LightMax  float64 `json:"lmax"`
	CaveLight float64 `json:"cave_light"`
	Glow      float64 `json:"glow"`
	// EVENT CHANCES
	PopChance     float64 `json:"pop_p"`
	BurstChance   float64 `json:"burst_p"`
	QuietChance   float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	BurstDur  int     `json:"burst_dur"`
	BurstMult float64 `json:"burst_mult"`
	QuietDur  int     `json:"quiet_dur"`
	QuietMult float64 `json:"quiet_mult"`
}

func (c CaveCrystalsConfig) withDefaults() CaveCrystalsConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 80
	}
	if c.IntroCluster <= 0 {
		c.IntroCluster = 4
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 90
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	if c.EndingLinger == 0 {
		c.EndingLinger = 40
	}
	if c.EndingDim <= 0 {
		c.EndingDim = 0.35
	}
	c.EndingDim = clamp01(c.EndingDim)
	if c.MaxCrystals <= 0 {
		c.MaxCrystals = 22
	}
	if c.MaxCrystals > 64 {
		c.MaxCrystals = 64
	}
	if c.Floor <= 0 {
		c.Floor = 0.82
	}
	if c.FloorRough < 0 {
		c.FloorRough = 0
	}
	if c.NucleateEvery <= 0 {
		c.NucleateEvery = 80
	}
	if c.GrowthEvery <= 0 {
		c.GrowthEvery = 60
	}
	if c.GrowthSteps <= 0 {
		c.GrowthSteps = 6
	}
	if c.GrowthSteps > 12 {
		c.GrowthSteps = 12
	}
	if c.StepHeight <= 0 {
		c.StepHeight = 1.6
	}
	if c.BaseWidth <= 0 {
		c.BaseWidth = 1.4
	}
	if c.Tilt < 0 {
		c.Tilt = 0
	}
	if c.Sparkle < 0 {
		c.Sparkle = 0
	}
	if c.Hue == 0 {
		c.Hue = 268
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.Sat <= 0 {
		c.Sat = 0.62
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.32
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.86
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.CaveLight <= 0 {
		c.CaveLight = 0.06
	}
	if c.Glow < 0 {
		c.Glow = 0
	}
	if c.PopChance < 0 {
		c.PopChance = 0
	}
	if c.BurstChance < 0 {
		c.BurstChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.BurstDur <= 0 {
		c.BurstDur = 36
	}
	if c.BurstMult <= 0 {
		c.BurstMult = 1.8
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 120
	}
	if c.QuietMult <= 0 {
		c.QuietMult = 0.4
	}
	return c
}

// CaveCrystalsSchema describes the cave-crystals effect's tunable knobs for
// the dev UI.
func CaveCrystalsSchema() EffectSchema {
	return EffectSchema{
		Name: "cave-crystals",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80, Trigger: "intro",
				Description: "Ticks spent seeding the first cluster of nuclei before steady-state growth takes over."},
			{Key: "intro_cluster", Label: "intro cluster", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 4,
				Description: "Number of nuclei dropped during the intro to anchor the visual."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 20, Max: 320, Step: 5, Default: 90, Trigger: "ending",
				Description: "Ticks spent finishing in-progress growth and dimming the cave."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 40,
				Description: "Extra still ticks after growth stops so the silhouette can hold the frame."},
			{Key: "ending_dim", Label: "ending dim", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "How dark the cave fades to during the outro. 0 = no dimming, 1 = total black-out."},
			{Key: "max_crystals", Label: "max crystals", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 4, Max: 64, Step: 1, Default: 22,
				Description: "Maximum number of crystals that can occupy the floor at once."},
			{Key: "floor", Label: "floor", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.55, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Vertical position of the cave floor as a fraction of the frame height."},
			{Key: "floor_rough", Label: "floor rough", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0, Max: 6, Step: 0.5, Default: 1.5,
				Description: "Per-column jitter applied to the floor silhouette so it reads as uneven cave rock."},
			{Key: "nucleate_every", Label: "nucleate 1/", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 10, Max: 600, Step: 5, Default: 80,
				Description: "Average ticks between new nuclei seeding on the floor. Higher = rarer."},
			{Key: "growth_every", Label: "growth 1/", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 5, Max: 600, Step: 5, Default: 60,
				Description: "Average ticks between growth pulses. Lower = crystals reach full size faster."},
			{Key: "growth_steps", Label: "growth steps", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 2, Max: 12, Step: 1, Default: 6,
				Description: "How many discrete steps a crystal goes through from nucleus to fully grown."},
			{Key: "step_height", Label: "step height", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Pixels added to the silhouette per growth step."},
			{Key: "base_width", Label: "base width", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.4,
				Description: "Half-width of a fully grown crystal at its base, in pixels."},
			{Key: "tilt", Label: "tilt", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.16,
				Description: "Maximum lean per crystal so the cluster doesn't read as perfectly vertical."},
			{Key: "sparkle", Label: "sparkle", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Brightness of facet sparkles that flash for a few ticks after each growth pulse."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 268,
				Description: "Base crystal hue. 268 reads as amethyst; 200 cyan-quartz; 60 glowstone-yellow."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 80, Step: 1, Default: 24,
				Description: "Per-crystal hue variation around the base hue."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.02, Default: 0.62,
				Description: "Crystal saturation. Lower values lean toward pale obsidian."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.32,
				Description: "Minimum lightness used for crystal cores."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used for sparkles and faceted highlights."},
			{Key: "cave_light", Label: "cave light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.06,
				Description: "Ambient lightness of the cave background. Lower = darker cave."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Faint lighting that fully grown crystals cast onto the surrounding floor."},
			{Key: "pop_p", Label: "pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "crystal-pop",
				Description: "Per-tick chance of a fully formed crystal popping in without growth steps."},
			{Key: "burst_p", Label: "sparkle burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "sparkle-burst",
				Description: "Per-tick chance of an extra sparkle wash brightening the existing cluster."},
			{Key: "quiet_p", Label: "quiet cave", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "quiet-cave",
				Description: "Per-tick chance of a long suppression window where growth slows and nothing new spawns."},
			{Key: "burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "sparkle-burst", Type: KnobInt, Min: 8, Max: 160, Step: 4, Default: 36,
				Description: "Duration of a sparkle-burst window in ticks."},
			{Key: "burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "sparkle-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Sparkle brightness multiplier applied during a burst."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-cave", Type: KnobInt, Min: 30, Max: 600, Step: 10, Default: 120,
				Description: "Duration of the quiet-cave suppression window."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-cave", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Growth and nucleation multiplier applied during a quiet-cave window."},
		},
	}
}

// CaveCrystalsState is the wire/persisted snapshot of the cave field.
type CaveCrystalsState struct {
	Tick          int       `json:"tick"`
	Crystals      []Crystal `json:"crystals"`
	SinceNucleate int       `json:"sinceNucleate"`
	SinceGrowth   int       `json:"sinceGrowth"`
	IntroTicks    int       `json:"introTicks"`
	IntroTotal    int       `json:"introTotal"`
	IntroSeeded   int       `json:"introSeeded"`
	EndingTicks   int       `json:"endingTicks"`
	EndingTotal   int       `json:"endingTotal"`
	EndingFade    int       `json:"endingFade"`
	BurstTicks    int       `json:"burstTicks"`
	QuietTicks    int       `json:"quietTicks"`
	RNGState      uint64    `json:"rngState,omitempty"`
}

type CaveCrystalsSnapshot struct {
	CaveCrystalsState
}

type CaveCrystalsPersistedState struct {
	CaveCrystalsState
}

// CaveCrystals is the authoritative server-side cave field.
type CaveCrystals struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  CaveCrystalsConfig
	tick int

	crystals []Crystal

	sinceNucleate int
	sinceGrowth   int

	introTicks  int
	introTotal  int
	introSeeded int

	endingTicks int
	endingTotal int
	endingFade  int

	burstTicks int
	quietTicks int

	log []LogEntry
}

func NewCaveCrystals(w, h int, seed int64, cfg CaveCrystalsConfig) *CaveCrystals {
	c := &CaveCrystals{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	return c
}

func (c *CaveCrystals) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.W = w
	c.H = h
}

func (c *CaveCrystals) SetConfig(cfg CaveCrystalsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg.withDefaults()
	if len(c.crystals) > c.cfg.MaxCrystals {
		c.crystals = c.crystals[:c.cfg.MaxCrystals]
	}
}

func (c *CaveCrystals) EffectiveConfig() CaveCrystalsConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

func (c *CaveCrystals) CurrentTick() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tick
}

func (c *CaveCrystals) PerturbRNG(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Mix(delta)
}

func (c *CaveCrystals) DrainLog() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.log) == 0 {
		return nil
	}
	out := c.log
	c.log = nil
	return out
}

func (c *CaveCrystals) appendLog(kind, desc string) {
	c.log = append(c.log, LogEntry{Tick: c.tick, Type: kind, Desc: desc})
	if len(c.log) > 200 {
		c.log = c.log[len(c.log)-200:]
	}
}

func (c *CaveCrystals) Snapshot() CaveCrystalsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CaveCrystalsSnapshot{CaveCrystalsState: c.snapshotStateLocked(true)}
}

func (c *CaveCrystals) RestoreSnapshot(s CaveCrystalsSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CaveCrystalsState)
}

func (c *CaveCrystals) SnapshotPersistedState() CaveCrystalsPersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CaveCrystalsPersistedState{CaveCrystalsState: c.snapshotStateLocked(true)}
}

func (c *CaveCrystals) RestorePersistedState(s CaveCrystalsPersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CaveCrystalsState)
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

func (c *CaveCrystals) snapshotStateLocked(includeRNG bool) CaveCrystalsState {
	out := CaveCrystalsState{
		Tick:          c.tick,
		SinceNucleate: c.sinceNucleate,
		SinceGrowth:   c.sinceGrowth,
		IntroTicks:    c.introTicks,
		IntroTotal:    c.introTotal,
		IntroSeeded:   c.introSeeded,
		EndingTicks:   c.endingTicks,
		EndingTotal:   c.endingTotal,
		EndingFade:    c.endingFade,
		BurstTicks:    c.burstTicks,
		QuietTicks:    c.quietTicks,
	}
	if len(c.crystals) > 0 {
		out.Crystals = make([]Crystal, len(c.crystals))
		copy(out.Crystals, c.crystals)
	}
	if includeRNG {
		out.RNGState = c.rng.State()
	}
	return out
}

func (c *CaveCrystals) restoreStateLocked(s CaveCrystalsState) {
	c.tick = s.Tick
	if len(s.Crystals) > 0 {
		c.crystals = make([]Crystal, len(s.Crystals))
		copy(c.crystals, s.Crystals)
	} else {
		c.crystals = nil
	}
	c.sinceNucleate = s.SinceNucleate
	c.sinceGrowth = s.SinceGrowth
	c.introTicks = s.IntroTicks
	c.introTotal = s.IntroTotal
	c.introSeeded = s.IntroSeeded
	c.endingTicks = s.EndingTicks
	c.endingTotal = s.EndingTotal
	c.endingFade = s.EndingFade
	c.burstTicks = s.BurstTicks
	c.quietTicks = s.QuietTicks
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (c *CaveCrystals) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "nucleus-spawn":
		if cr, ok := c.spawnNucleusLocked(); ok {
			c.appendLog("nucleus-spawn", fmt.Sprintf("col=%d hue=%.0f", cr.Col, cr.Hue))
		} else {
			c.appendLog("nucleus-spawn", "skipped (full)")
		}
	case "growth-pulse":
		idx, ok := c.pickGrowingLocked()
		if !ok {
			c.appendLog("growth-pulse", "skipped (none growing)")
			return true
		}
		c.crystals[idx].Step++
		c.crystals[idx].Phase = 0
		c.appendLog("growth-pulse", fmt.Sprintf("crystal %d -> step %d/%d", idx+1, c.crystals[idx].Step, c.crystals[idx].MaxStep))
	case "crystal-pop":
		c.popCrystalLocked("triggered")
	case "sparkle-burst":
		c.startBurstLocked("triggered")
	case "quiet-cave":
		c.startQuietLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, cluster=%d)", c.introTotal, c.cfg.IntroCluster))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", c.endingFade, c.endingTotal-c.endingFade))
	default:
		return false
	}
	return true
}

func (c *CaveCrystals) startIntroLocked() {
	c.crystals = nil
	c.introTotal = c.cfg.IntroDur
	if c.introTotal <= 0 {
		c.introTotal = 80
	}
	c.introTicks = c.introTotal
	c.introSeeded = 0
	c.endingTicks = 0
	c.endingTotal = 0
	c.endingFade = 0
	c.burstTicks = 0
	c.quietTicks = 0
	c.sinceNucleate = 0
	c.sinceGrowth = 0
}

func (c *CaveCrystals) startEndingLocked() {
	c.introTicks = 0
	c.introTotal = 0
	c.introSeeded = 0
	c.burstTicks = 0
	c.quietTicks = 0
	c.endingFade = c.cfg.EndingDur
	if c.endingFade <= 0 {
		c.endingFade = 90
	}
	linger := c.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	c.endingTotal = c.endingFade + linger
	if c.endingTotal < 1 {
		c.endingTotal = c.endingFade
	}
	c.endingTicks = c.endingTotal
}

func (c *CaveCrystals) startBurstLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.BurstDur, 0.3)
	c.burstTicks = dur
	c.appendLog("sparkle-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, c.cfg.BurstMult))
}

func (c *CaveCrystals) startQuietLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.QuietDur, 0.3)
	c.quietTicks = dur
	c.appendLog("quiet-cave", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, c.cfg.QuietMult))
}

// spawnNucleusLocked seeds a new crystal. Returns the placed crystal and ok=true on success.
// Avoids stacking new nuclei directly on top of existing ones so the field reads as a spread.
func (c *CaveCrystals) spawnNucleusLocked() (Crystal, bool) {
	if c.W <= 0 || c.H <= 0 {
		return Crystal{}, false
	}
	if len(c.crystals) >= c.cfg.MaxCrystals {
		return Crystal{}, false
	}
	col := -1
	minSpacing := 3
	if c.cfg.MaxCrystals > 0 {
		ideal := c.W / (c.cfg.MaxCrystals + 1)
		if ideal > minSpacing {
			minSpacing = ideal
		}
	}
	for attempt := 0; attempt < 8; attempt++ {
		candidate := c.rng.Intn(c.W)
		ok := true
		for _, ex := range c.crystals {
			if absInt(ex.Col-candidate) < minSpacing {
				ok = false
				break
			}
		}
		if ok {
			col = candidate
			break
		}
	}
	if col < 0 {
		col = c.rng.Intn(c.W)
	}
	cr := Crystal{
		Col:     col,
		Anchor:  c.rng.Intn(3) - 1,
		Step:    1,
		MaxStep: c.cfg.GrowthSteps - c.rng.Intn(max(1, c.cfg.GrowthSteps/3)),
		Hue:     math.Mod(c.cfg.Hue+(c.rng.Float64()*2-1)*c.cfg.HueSpread+360, 360),
		Tilt:    (c.rng.Float64()*2 - 1) * c.cfg.Tilt,
		Phase:   0,
	}
	if cr.MaxStep < 2 {
		cr.MaxStep = 2
	}
	c.crystals = append(c.crystals, cr)
	return cr, true
}

func (c *CaveCrystals) pickGrowingLocked() (int, bool) {
	growing := make([]int, 0, len(c.crystals))
	for i, cr := range c.crystals {
		if cr.Step < cr.MaxStep {
			growing = append(growing, i)
		}
	}
	if len(growing) == 0 {
		return 0, false
	}
	return growing[c.rng.Intn(len(growing))], true
}

func (c *CaveCrystals) popCrystalLocked(verb string) {
	cr, ok := c.spawnNucleusLocked()
	if !ok {
		c.appendLog("crystal-pop", "skipped (full)")
		return
	}
	cr.Step = cr.MaxStep
	c.crystals[len(c.crystals)-1] = cr
	c.appendLog("crystal-pop", fmt.Sprintf("%s col=%d hue=%.0f", verb, cr.Col, cr.Hue))
}

func (c *CaveCrystals) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++

	for i := range c.crystals {
		if c.crystals[i].Phase < 1<<30 {
			c.crystals[i].Phase++
		}
	}

	if c.endingTicks > 0 {
		c.endingTicks--
		// Continue advancing growth at half rate during ending so in-flight
		// crystals can finish their current step, but no new nuclei spawn.
		c.sinceGrowth++
		growEvery := c.cfg.GrowthEvery * 2
		if growEvery < 1 {
			growEvery = 1
		}
		if c.sinceGrowth >= growEvery {
			c.sinceGrowth = 0
			if idx, ok := c.pickGrowingLocked(); ok {
				c.crystals[idx].Step++
				c.crystals[idx].Phase = 0
			}
		}
		if c.endingTicks == 0 {
			c.startIntroLocked()
			c.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, cluster=%d)", c.introTotal, c.cfg.IntroCluster))
		}
		return
	}

	if c.introTicks > 0 {
		c.introTicks--
		// Distribute the intro cluster evenly across the intro window so the
		// first few crystals appear quickly to anchor the visual.
		target := c.cfg.IntroCluster
		if target > c.cfg.MaxCrystals {
			target = c.cfg.MaxCrystals
		}
		if target > 0 && c.introTotal > 0 {
			elapsed := c.introTotal - c.introTicks
			wantSeeded := (elapsed*target + c.introTotal/2) / c.introTotal
			if wantSeeded > target {
				wantSeeded = target
			}
			for c.introSeeded < wantSeeded && len(c.crystals) < c.cfg.MaxCrystals {
				if cr, ok := c.spawnNucleusLocked(); ok {
					c.crystals[len(c.crystals)-1].Step = c.crystals[len(c.crystals)-1].MaxStep / 2
					c.appendLog("nucleus-spawn", fmt.Sprintf("intro col=%d hue=%.0f", cr.Col, cr.Hue))
					c.introSeeded++
				} else {
					break
				}
			}
		}
		return
	}

	c.sinceNucleate++
	c.sinceGrowth++

	nucleateEvery := c.cfg.NucleateEvery
	growEvery := c.cfg.GrowthEvery
	if c.quietTicks > 0 {
		c.quietTicks--
		mult := c.cfg.QuietMult
		if mult <= 0 {
			mult = 1
		}
		nucleateEvery = int(math.Round(float64(nucleateEvery) / mult))
		growEvery = int(math.Round(float64(growEvery) / mult))
	}
	if nucleateEvery < 1 {
		nucleateEvery = 1
	}
	if growEvery < 1 {
		growEvery = 1
	}

	if c.burstTicks > 0 {
		c.burstTicks--
	}

	if c.sinceNucleate >= nucleateEvery && len(c.crystals) < c.cfg.MaxCrystals {
		c.sinceNucleate = 0
		if cr, ok := c.spawnNucleusLocked(); ok {
			c.appendLog("nucleus-spawn", fmt.Sprintf("col=%d hue=%.0f", cr.Col, cr.Hue))
		}
	}

	if c.sinceGrowth >= growEvery {
		c.sinceGrowth = 0
		if idx, ok := c.pickGrowingLocked(); ok {
			c.crystals[idx].Step++
			c.crystals[idx].Phase = 0
			c.appendLog("growth-pulse", fmt.Sprintf("crystal %d -> step %d/%d", idx+1, c.crystals[idx].Step, c.crystals[idx].MaxStep))
		}
	}

	if c.cfg.PopChance > 0 && c.rng.Float64() < c.cfg.PopChance {
		c.popCrystalLocked("started")
	}
	if c.cfg.BurstChance > 0 && c.burstTicks <= 0 && c.rng.Float64() < c.cfg.BurstChance {
		c.startBurstLocked("started")
	}
	if c.cfg.QuietChance > 0 && c.quietTicks <= 0 && c.rng.Float64() < c.cfg.QuietChance {
		c.startQuietLocked("started")
	}
}

// GridCopy renders the current frame. The cave background, uneven floor, and
// each crystal silhouette are drawn deterministically from the crystals slice
// so server and client replicas converge on the same field for any given
// snapshot.
func (c *CaveCrystals) GridCopy() [][]Pixel {
	c.mu.Lock()
	defer c.mu.Unlock()
	grid := make([][]Pixel, c.H)
	for y := range grid {
		grid[y] = make([]Pixel, c.W)
	}
	if c.W <= 0 || c.H <= 0 {
		return grid
	}
	cfg := c.cfg

	dim := 1.0
	if c.endingTicks > 0 && c.endingFade > 0 {
		fadeProgress := 1.0 - float64(c.endingTicks)/float64(c.endingFade)
		if fadeProgress > 1 {
			fadeProgress = 1
		}
		if fadeProgress < 0 {
			fadeProgress = 0
		}
		dim = 1 - fadeProgress*cfg.EndingDim
	}
	if c.introTicks > 0 && c.introTotal > 0 {
		introProgress := 1.0 - float64(c.introTicks)/float64(c.introTotal)
		if introProgress < 0.2 {
			dim *= 0.6 + introProgress*2.0
		}
	}

	caveTop := hslToRGB(cfg.Hue+8, cfg.Sat*0.18, cfg.CaveLight*0.35*dim)
	caveBottom := hslToRGB(cfg.Hue+8, cfg.Sat*0.22, cfg.CaveLight*1.05*dim)
	for y := 0; y < c.H; y++ {
		t := 0.0
		if c.H > 1 {
			t = float64(y) / float64(c.H-1)
		}
		row := mixColor(caveTop, caveBottom, t)
		for x := 0; x < c.W; x++ {
			grid[y][x] = Pixel{Filled: true, C: row}
		}
	}

	floor := int(math.Round(float64(c.H-1) * cfg.Floor))
	if floor < 1 {
		floor = c.H - 1
	}
	if floor >= c.H {
		floor = c.H - 1
	}
	floorTops := make([]int, c.W)
	for x := 0; x < c.W; x++ {
		jitter := 0.0
		if cfg.FloorRough > 0 {
			jitter = (math.Sin(float64(x)*0.37)+math.Sin(float64(x)*0.13+1.3))*cfg.FloorRough*0.45 +
				(math.Sin(float64(x)*0.07+0.6))*cfg.FloorRough*0.25
		}
		top := floor + int(math.Round(jitter))
		if top < 1 {
			top = 1
		}
		if top >= c.H {
			top = c.H - 1
		}
		floorTops[x] = top
	}
	floorBase := hslToRGB(cfg.Hue+12, cfg.Sat*0.16, math.Min(cfg.LightMin*0.75, cfg.CaveLight*2.4)*dim)
	floorEdge := hslToRGB(cfg.Hue+10, cfg.Sat*0.24, math.Min(cfg.LightMin, cfg.CaveLight*3.4)*dim)
	for x := 0; x < c.W; x++ {
		top := floorTops[x]
		for y := top; y < c.H; y++ {
			col := floorBase
			if y == top {
				col = floorEdge
			}
			grid[y][x] = Pixel{Filled: true, C: col}
		}
	}

	if cfg.Glow > 0 {
		for _, cr := range c.crystals {
			if cr.Step < cr.MaxStep {
				continue
			}
			top := floorTops[clampInt(cr.Col, 0, c.W-1)]
			radius := 4 + int(math.Round(float64(cr.MaxStep)*0.4))
			for dy := -radius; dy <= 1; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					gy := top + dy
					gx := cr.Col + dx
					if gy < 0 || gy >= c.H || gx < 0 || gx >= c.W {
						continue
					}
					dist := math.Hypot(float64(dx), float64(dy))
					falloff := math.Max(0, 1-dist/float64(radius+1))
					if falloff <= 0 {
						continue
					}
					weight := cfg.Glow * 0.18 * falloff * dim
					base := grid[gy][gx].C
					tint := hslToRGB(cr.Hue, cfg.Sat*0.6, cfg.LightMin+(cfg.LightMax-cfg.LightMin)*0.35)
					grid[gy][gx] = Pixel{Filled: true, C: blendRGBA(base, tint, weight)}
				}
			}
		}
	}

	burstGain := 1.0
	if c.burstTicks > 0 && cfg.BurstMult > 1 {
		burstGain = cfg.BurstMult
	}

	for ci, cr := range c.crystals {
		if cr.Step <= 0 {
			continue
		}
		baseTop := floorTops[clampInt(cr.Col, 0, c.W-1)]
		baseTop += cr.Anchor
		if baseTop < 0 {
			baseTop = 0
		}
		if baseTop >= c.H {
			baseTop = c.H - 1
		}
		stepFrac := math.Min(1, float64(cr.Step)/math.Max(1, float64(cr.MaxStep)))
		height := int(math.Round(cfg.StepHeight * float64(cr.Step)))
		if height < 1 {
			height = 1
		}
		baseHalf := math.Max(0, cfg.BaseWidth*stepFrac)
		drawCrystal(grid, cr, baseTop, height, baseHalf, cfg, dim, ci)
	}

	if cfg.Sparkle > 0 || c.burstTicks > 0 {
		for ci, cr := range c.crystals {
			if cr.Step <= 0 {
				continue
			}
			baseTop := floorTops[clampInt(cr.Col, 0, c.W-1)] + cr.Anchor
			height := int(math.Round(cfg.StepHeight * float64(cr.Step)))
			if height < 1 {
				height = 1
			}
			tipY := baseTop - height
			tipX := cr.Col + int(math.Round(cr.Tilt*float64(height)))
			fade := 1.0 - math.Min(1, float64(cr.Phase)/12.0)
			intensity := cfg.Sparkle * fade * burstGain * dim
			if intensity <= 0 {
				continue
			}
			seed := uint64(ci*131 + cr.Col*7 + c.tick/3)
			placeSparkle(grid, tipX, tipY, intensity, cr.Hue, cfg, seed)
			placeSparkle(grid, tipX-1, tipY+1, intensity*0.65, cr.Hue, cfg, seed^13)
			placeSparkle(grid, tipX+1, tipY+1, intensity*0.65, cr.Hue, cfg, seed^29)
		}
	}

	return grid
}

func drawCrystal(grid [][]Pixel, cr Crystal, baseTop int, height int, baseHalf float64, cfg CaveCrystalsConfig, dim float64, idx int) {
	if height < 1 {
		return
	}
	core := hslToRGB(cr.Hue, cfg.Sat, cfg.LightMin+(cfg.LightMax-cfg.LightMin)*0.55*dim)
	bright := hslToRGB(cr.Hue, math.Min(1, cfg.Sat*1.05), cfg.LightMax*0.95*dim)
	dark := hslToRGB(math.Mod(cr.Hue+340, 360), cfg.Sat*0.85, cfg.LightMin*0.85*dim)
	tipColor := hslToRGB(cr.Hue, cfg.Sat*0.92, math.Min(1, cfg.LightMax*dim))
	for ty := 0; ty < height; ty++ {
		yratio := 0.0
		if height > 1 {
			yratio = float64(ty) / float64(height-1)
		}
		half := int(math.Round(math.Max(0, baseHalf*(1-yratio*0.92))))
		gy := baseTop - ty
		if gy < 0 || gy >= len(grid) {
			continue
		}
		offset := int(math.Round(cr.Tilt * float64(ty)))
		cx := cr.Col + offset
		paintPixel(grid, cx, gy, core)
		for dx := 1; dx <= half; dx++ {
			c := core
			if dx == half {
				if (idx+dx+ty)%2 == 0 {
					c = bright
				} else {
					c = dark
				}
			}
			paintPixel(grid, cx-dx, gy, c)
			paintPixel(grid, cx+dx, gy, c)
		}
		if ty == height-1 {
			paintPixel(grid, cx, gy, tipColor)
		}
	}
}

func placeSparkle(grid [][]Pixel, x, y int, intensity float64, hue float64, cfg CaveCrystalsConfig, seed uint64) {
	if intensity <= 0 {
		return
	}
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[0]) {
		return
	}
	wave := math.Sin(float64(seed) * 0.97)
	bright := cfg.LightMax*0.9 + 0.1*wave
	if bright > 1 {
		bright = 1
	}
	c := hslToRGB(hue, cfg.Sat*0.7, bright)
	prev := grid[y][x].C
	grid[y][x] = Pixel{Filled: true, C: blendRGBA(prev, c, math.Min(1, intensity))}
}

func blendRGBA(a, b color.RGBA, t float64) color.RGBA {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return color.RGBA{R: b.R, G: b.G, B: b.B, A: 255}
	}
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: 255,
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
