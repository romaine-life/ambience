package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// CaveCrystals grows a slow field of faceted crystals along a cave floor.
// The server is authoritative about when nuclei spawn and when each crystal
// grows another step, so clients running their own replicas stay in sync on
// the long-arc shape of the field even though the inner sparkle noise drifts
// per client.
//
// Field growth is intentionally slow: a healthy default (~10 min cross-effect
// cadence) should fill the field over the lifetime of the running effect, so
// each fresh runtime begins on a bare floor and ends on a packed grove.

// CaveCrystalsConfig tunes the cave-crystals prototype used in isolated dev
// sessions. See CaveCrystalsSchema for the full knob inventory.
type CaveCrystalsConfig struct {
	// INTRODUCTION
	IntroDur    int     `json:"intro_dur"`
	IntroBurst  int     `json:"intro_burst"`
	IntroGrowth float64 `json:"intro_growth"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingDim    float64 `json:"ending_dim"`
	// LEVERS — cave
	Baseline    float64 `json:"baseline"`
	FloorJitter float64 `json:"floor_jitter"`
	Ambient     float64 `json:"ambient"`
	// LEVERS — field
	MaxCrystals   int     `json:"max_crystals"`
	MaxGrowth     int     `json:"max_growth"`
	CrystalHeight float64 `json:"crystal_height"`
	CrystalWidth  float64 `json:"crystal_width"`
	// LEVERS — color
	Hue       float64 `json:"hue"`
	HueSpread float64 `json:"hue_sp"`
	Sat       float64 `json:"sat"`
	LightMin  float64 `json:"lmin"`
	LightMax  float64 `json:"lmax"`
	// EVENT CHANCES
	NucleusChance float64 `json:"nucleus_p"`
	GrowthChance  float64 `json:"growth_p"`
	PopChance     float64 `json:"pop_p"`
	BurstChance   float64 `json:"burst_p"`
	QuietChance   float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	SparkleDur    int     `json:"sparkle_dur"`
	BurstSparkles int     `json:"burst_sparkles"`
	PopGrowth     float64 `json:"pop_growth"`
	QuietDur      int     `json:"quiet_dur"`
	QuietMult     float64 `json:"quiet_mult"`
}

func (c CaveCrystalsConfig) withDefaults() CaveCrystalsConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 60
	}
	if c.IntroBurst < 0 {
		c.IntroBurst = 0
	}
	if c.IntroBurst > 200 {
		c.IntroBurst = 200
	}
	c.IntroGrowth = clamp01(c.IntroGrowth)
	if c.EndingDur <= 0 {
		c.EndingDur = 80
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	c.EndingDim = clamp01(c.EndingDim)
	if c.Baseline <= 0 {
		c.Baseline = 0.82
	}
	if c.FloorJitter < 0 {
		c.FloorJitter = 0
	}
	if c.Ambient < 0 {
		c.Ambient = 0
	}
	if c.MaxCrystals <= 0 {
		c.MaxCrystals = 28
	}
	if c.MaxCrystals > 120 {
		c.MaxCrystals = 120
	}
	if c.MaxGrowth <= 0 {
		c.MaxGrowth = 4
	}
	if c.MaxGrowth > 8 {
		c.MaxGrowth = 8
	}
	if c.CrystalHeight <= 0 {
		c.CrystalHeight = 7
	}
	if c.CrystalWidth <= 0 {
		c.CrystalWidth = 2.4
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
		c.LightMin = 0.18
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.86
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.NucleusChance < 0 {
		c.NucleusChance = 0
	}
	if c.GrowthChance < 0 {
		c.GrowthChance = 0
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
	if c.SparkleDur <= 0 {
		c.SparkleDur = 14
	}
	if c.BurstSparkles <= 0 {
		c.BurstSparkles = 8
	}
	if c.PopGrowth <= 0 {
		c.PopGrowth = 0.85
	}
	if c.PopGrowth > 1 {
		c.PopGrowth = 1
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 90
	}
	if c.QuietMult <= 0 || c.QuietMult > 1 {
		c.QuietMult = 0.25
	}
	return c
}

// CaveCrystalsSchema describes the cave-crystals effect's tunable knobs for
// the dev UI.
func CaveCrystalsSchema() EffectSchema {
	return EffectSchema{
		Name: "cave-crystals",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent seeding the floor with the first nuclei before normal growth takes over."},
			{Key: "intro_burst", Label: "intro burst", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 24, Step: 1, Default: 5,
				Description: "Number of small crystals seeded immediately when intro fires so the floor isn't bare."},
			{Key: "intro_growth", Label: "intro growth", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Starting growth step for intro-burst crystals, as a fraction of max growth."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks during which new nuclei stop and in-progress crystals finish their last growth step."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 30,
				Description: "Extra still ticks after growth ends so the field can hold the frame as the cave dims."},
			{Key: "ending_dim", Label: "ending dim", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.18,
				Description: "Residual crystal brightness at the end of the outro."},
			{Key: "baseline", Label: "baseline", Slot: SlotLever, Group: "cave", Type: KnobFloat, Min: 0.55, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Vertical position of the cave floor as a fraction of frame height."},
			{Key: "floor_jitter", Label: "floor rough", Slot: SlotLever, Group: "cave", Type: KnobFloat, Min: 0, Max: 4, Step: 0.1, Default: 1.4,
				Description: "Per-column jitter applied to the cave floor silhouette."},
			{Key: "ambient", Label: "ambient", Slot: SlotLever, Group: "cave", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Faint ambient glow above the floor, so the cave isn't pitch-black."},
			{Key: "max_crystals", Label: "max field", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 4, Max: 80, Step: 1, Default: 28,
				Description: "Cap on the number of crystals seeded into the field over the effect's lifetime."},
			{Key: "max_growth", Label: "max grow", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 4,
				Description: "Maximum growth steps a single crystal can reach. Higher values stretch the long-arc growth window."},
			{Key: "crystal_height", Label: "height", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 3, Max: 14, Step: 0.5, Default: 7,
				Description: "Approximate full-grown crystal height in pixels."},
			{Key: "crystal_width", Label: "width", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 1, Max: 5, Step: 0.2, Default: 2.4,
				Description: "Approximate fully-grown crystal half-width in pixels."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 268,
				Description: "Base crystal hue. Around 270 gives amethyst, 200 gives quartz, 150 glowstone, 0 obsidian."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Per-crystal variation around the base hue."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.62,
				Description: "Crystal-color saturation. Lower values lean toward dim stone tones."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for crystal silhouettes and the cave floor."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.45, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used for sparkles and crystal highlights."},
			{Key: "nucleus_p", Label: "nucleus", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0.004, Trigger: "nucleus-spawn",
				Description: "Per-tick chance of seeding a new crystal nucleus on the floor."},
			{Key: "growth_p", Label: "growth", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.001, Default: 0.012, Trigger: "growth-pulse",
				Description: "Per-tick chance of one existing crystal advancing a growth step."},
			{Key: "pop_p", Label: "pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.005, Step: 0.0001, Default: 0, Trigger: "crystal-pop",
				Description: "Per-tick chance of a fully-formed crystal popping in. Rare — meant as an accent, not the default."},
			{Key: "burst_p", Label: "sparkle burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "sparkle-burst",
				Description: "Per-tick chance of an extra sparkle burst around an existing crystal cluster."},
			{Key: "quiet_p", Label: "quiet cave", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "quiet-cave",
				Description: "Per-tick chance of a long suppression window during which growth almost stops."},
			{Key: "sparkle_dur", Label: "sparkle dur", Slot: SlotEventMod, Group: "sparkle", Type: KnobInt, Min: 4, Max: 60, Step: 2, Default: 14,
				Description: "Ticks each crystal sparkles after a growth pulse before settling."},
			{Key: "burst_sparkles", Label: "burst count", Slot: SlotEventMod, Group: "sparkle", Type: KnobInt, Min: 1, Max: 24, Step: 1, Default: 8,
				Description: "Approximate number of crystals lit up by a sparkle burst event."},
			{Key: "pop_growth", Label: "pop grown", Slot: SlotEventMod, Group: "pop", Type: KnobFloat, Min: 0.5, Max: 1, Step: 0.05, Default: 0.85,
				Description: "Initial growth fraction (0..1) for a popped crystal — closer to 1 means it appears fully formed."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet", Type: KnobInt, Min: 20, Max: 400, Step: 5, Default: 90,
				Description: "Duration of a quiet-cave window in ticks."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Growth/nucleus probability multiplier while quiet-cave is active."},
		},
	}
}

// CrystalSnap is the persisted shape of a single crystal in the field.
type CrystalSnap struct {
	Col          int     `json:"col"`
	Growth       int     `json:"g"`
	Variant      int     `json:"v"`
	HueOffset    float64 `json:"h"`
	SparkleTicks int     `json:"s,omitempty"`
	Big          bool    `json:"big,omitempty"`
}

// CaveCrystalsState is the wire/persisted snapshot of the cave-crystals field.
type CaveCrystalsState struct {
	Tick        int           `json:"tick"`
	Crystals    []CrystalSnap `json:"crystals,omitempty"`
	IntroTicks  int           `json:"introTicks"`
	IntroTotal  int           `json:"introTotal"`
	EndingTicks int           `json:"endingTicks"`
	EndingTotal int           `json:"endingTotal"`
	EndingFade  int           `json:"endingFade"`
	Lifecycle   Lifecycle     `json:"lifecycle"`
	QuietTicks  int           `json:"quietTicks"`
	RNGState    uint64        `json:"rngState,omitempty"`
}

type CaveCrystalsSnapshot struct {
	CaveCrystalsState
}

type CaveCrystalsPersistedState struct {
	CaveCrystalsState
}

type caveCrystal struct {
	col          int
	growth       int
	variant      int
	hueOffset    float64
	sparkleTicks int
	big          bool
}

// CaveCrystals is the authoritative server-side crystal field.
type CaveCrystals struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  CaveCrystalsConfig
	tick int

	crystals []caveCrystal

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int
	quietTicks  int

	log []LogEntry
}

func NewCaveCrystals(w, h int, seed int64, cfg CaveCrystalsConfig) *CaveCrystals {
	return &CaveCrystals{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
}

func (c *CaveCrystals) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if w == c.W && h == c.H {
		return
	}
	scaleX := float64(w) / math.Max(1, float64(c.W))
	c.W = w
	c.H = h
	for i := range c.crystals {
		c.crystals[i].col = int(math.Round(float64(c.crystals[i].col) * scaleX))
		if c.crystals[i].col < 0 {
			c.crystals[i].col = 0
		}
		if c.crystals[i].col >= c.W {
			c.crystals[i].col = c.W - 1
		}
	}
}

func (c *CaveCrystals) SetConfig(cfg CaveCrystalsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg.withDefaults()
	// Trim any per-crystal growth that exceeds the new max so a tightened
	// MaxGrowth doesn't render past-cap shapes.
	for i := range c.crystals {
		if c.crystals[i].growth > c.cfg.MaxGrowth {
			c.crystals[i].growth = c.cfg.MaxGrowth
		}
	}
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
	return CaveCrystalsSnapshot{CaveCrystalsState: c.snapshotStateLocked()}
}

func (c *CaveCrystals) RestoreSnapshot(s CaveCrystalsSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CaveCrystalsState)
}

func (c *CaveCrystals) SnapshotPersistedState() CaveCrystalsPersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CaveCrystalsPersistedState{CaveCrystalsState: c.snapshotStateLocked()}
}

func (c *CaveCrystals) RestorePersistedState(s CaveCrystalsPersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.CaveCrystalsState)
}

func (c *CaveCrystals) snapshotStateLocked() CaveCrystalsState {
	out := CaveCrystalsState{
		Tick:        c.tick,
		IntroTicks:  c.introTicks,
		IntroTotal:  c.introTotal,
		EndingTicks: c.endingTicks,
		EndingTotal: c.endingTotal,
		EndingFade:  c.endingFade,
		Lifecycle:   c.lifecycleLocked(),
		QuietTicks:  c.quietTicks,
		RNGState:    c.rng.State(),
	}
	if len(c.crystals) > 0 {
		out.Crystals = make([]CrystalSnap, len(c.crystals))
		for i, cr := range c.crystals {
			out.Crystals[i] = CrystalSnap{
				Col:          cr.col,
				Growth:       cr.growth,
				Variant:      cr.variant,
				HueOffset:    cr.hueOffset,
				SparkleTicks: cr.sparkleTicks,
				Big:          cr.big,
			}
		}
	}
	return out
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// the field's counters. The outro is non-terminal: when the ending window
// expires Step auto-restarts the intro, so lifecycle passes through intro
// back to running (the schema declares ending_terminal: false by omission).
func (c *CaveCrystals) lifecycleLocked() Lifecycle {
	switch {
	case c.introTicks > 0:
		return LifecycleIntro
	case c.endingTicks > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (c *CaveCrystals) restoreStateLocked(s CaveCrystalsState) {
	c.tick = s.Tick
	c.introTicks = s.IntroTicks
	c.introTotal = s.IntroTotal
	c.endingTicks = s.EndingTicks
	c.endingTotal = s.EndingTotal
	c.endingFade = s.EndingFade
	c.quietTicks = s.QuietTicks
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
	c.crystals = c.crystals[:0]
	for _, cs := range s.Crystals {
		c.crystals = append(c.crystals, caveCrystal{
			col:          cs.Col,
			growth:       cs.Growth,
			variant:      cs.Variant,
			hueOffset:    cs.HueOffset,
			sparkleTicks: cs.SparkleTicks,
			big:          cs.Big,
		})
	}
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (c *CaveCrystals) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "nucleus-spawn":
		if c.spawnNucleusLocked(0) {
			c.appendLog("nucleus-spawn", fmt.Sprintf("seeded (count=%d)", len(c.crystals)))
		} else {
			c.appendLog("nucleus-spawn", "skipped (field full)")
		}
	case "growth-pulse":
		if idx, ok := c.pickGrowingLocked(); ok {
			c.advanceGrowthLocked(idx)
			c.appendLog("growth-pulse", fmt.Sprintf("crystal %d -> %d/%d", idx+1, c.crystals[idx].growth, c.cfg.MaxGrowth))
		} else {
			c.appendLog("growth-pulse", "skipped (no growing crystals)")
		}
	case "crystal-pop":
		startGrowth := int(math.Round(float64(c.cfg.MaxGrowth) * c.cfg.PopGrowth))
		if startGrowth < 1 {
			startGrowth = 1
		}
		if c.spawnNucleusLocked(startGrowth) {
			c.crystals[len(c.crystals)-1].big = true
			c.crystals[len(c.crystals)-1].sparkleTicks = c.cfg.SparkleDur * 2
			c.appendLog("crystal-pop", fmt.Sprintf("popped (g=%d/%d)", startGrowth, c.cfg.MaxGrowth))
		} else {
			c.appendLog("crystal-pop", "skipped (field full)")
		}
	case "sparkle-burst":
		c.startBurstLocked("triggered")
	case "quiet-cave":
		c.startQuietLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, burst=%d)", c.introTotal, c.cfg.IntroBurst))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", c.endingFade, c.endingTotal-c.endingFade))
	default:
		return false
	}
	return true
}

func (c *CaveCrystals) startBurstLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.SparkleDur, 0.3)
	count := c.cfg.BurstSparkles
	if count > len(c.crystals) {
		count = len(c.crystals)
	}
	if count == 0 {
		c.appendLog("sparkle-burst", verb+" (skipped — no crystals)")
		return
	}
	for i := 0; i < count; i++ {
		idx := c.rng.Intn(len(c.crystals))
		if c.crystals[idx].sparkleTicks < dur {
			c.crystals[idx].sparkleTicks = dur
		}
	}
	c.appendLog("sparkle-burst", fmt.Sprintf("%s (lit=%d, dur=%d)", verb, count, dur))
}

func (c *CaveCrystals) startQuietLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.QuietDur, 0.3)
	c.quietTicks = dur
	c.appendLog("quiet-cave", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, c.cfg.QuietMult))
}

func (c *CaveCrystals) startIntroLocked() {
	c.crystals = c.crystals[:0]
	c.introTotal = c.cfg.IntroDur
	if c.introTotal <= 0 {
		c.introTotal = 60
	}
	c.introTicks = c.introTotal
	c.endingTicks = 0
	c.endingTotal = 0
	c.endingFade = 0
	c.quietTicks = 0
	startGrowth := int(math.Round(float64(c.cfg.MaxGrowth) * c.cfg.IntroGrowth))
	if startGrowth < 0 {
		startGrowth = 0
	}
	for i := 0; i < c.cfg.IntroBurst; i++ {
		c.spawnNucleusLocked(startGrowth)
	}
}

func (c *CaveCrystals) startEndingLocked() {
	c.introTicks = 0
	c.introTotal = 0
	c.quietTicks = 0
	c.endingFade = c.cfg.EndingDur
	if c.endingFade <= 0 {
		c.endingFade = 80
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

// spawnNucleusLocked seeds a new crystal at a random floor column. Returns
// false if the field is at MaxCrystals.
func (c *CaveCrystals) spawnNucleusLocked(startGrowth int) bool {
	if len(c.crystals) >= c.cfg.MaxCrystals {
		return false
	}
	if c.W <= 0 {
		return false
	}
	if startGrowth < 0 {
		startGrowth = 0
	}
	if startGrowth > c.cfg.MaxGrowth {
		startGrowth = c.cfg.MaxGrowth
	}
	col := c.rng.Intn(c.W)
	hueOffset := (c.rng.Float64()*2 - 1) * c.cfg.HueSpread
	c.crystals = append(c.crystals, caveCrystal{
		col:          col,
		growth:       startGrowth,
		variant:      c.rng.Intn(3),
		hueOffset:    hueOffset,
		sparkleTicks: c.cfg.SparkleDur,
	})
	return true
}

// pickGrowingLocked picks a crystal that hasn't reached MaxGrowth yet.
func (c *CaveCrystals) pickGrowingLocked() (int, bool) {
	growable := make([]int, 0, len(c.crystals))
	for i, cr := range c.crystals {
		if cr.growth < c.cfg.MaxGrowth {
			growable = append(growable, i)
		}
	}
	if len(growable) == 0 {
		return 0, false
	}
	return growable[c.rng.Intn(len(growable))], true
}

func (c *CaveCrystals) advanceGrowthLocked(idx int) {
	if idx < 0 || idx >= len(c.crystals) {
		return
	}
	if c.crystals[idx].growth >= c.cfg.MaxGrowth {
		return
	}
	c.crystals[idx].growth++
	c.crystals[idx].sparkleTicks = c.cfg.SparkleDur
}

func (c *CaveCrystals) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++

	// Lifecycle decrement.
	if c.endingTicks > 0 {
		c.endingTicks--
		// When ending finishes, intro auto-restarts so the next scene window
		// reads as a fresh field instead of a residue.
		if c.endingTicks == 0 {
			c.startIntroLocked()
			c.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, burst=%d)", c.introTotal, c.cfg.IntroBurst))
		}
	} else if c.introTicks > 0 {
		c.introTicks--
	}

	if c.quietTicks > 0 {
		c.quietTicks--
	}

	// Decay sparkle ticks across the field every step regardless of phase
	// so client/server sparkle counters stay coherent.
	for i := range c.crystals {
		if c.crystals[i].sparkleTicks > 0 {
			c.crystals[i].sparkleTicks--
		}
	}

	if c.endingTicks > 0 {
		// Ending phase: no new nuclei, but in-progress crystals can still
		// finish their current step.
		c.maybeFinishStepLocked()
		return
	}

	if c.introTicks > 0 {
		c.maybeIntroSpawnLocked()
		return
	}

	mult := 1.0
	if c.quietTicks > 0 {
		mult = c.cfg.QuietMult
	}
	if c.cfg.NucleusChance > 0 && c.rng.Float64() < c.cfg.NucleusChance*mult {
		if c.spawnNucleusLocked(0) {
			c.appendLog("nucleus-spawn", fmt.Sprintf("seeded (count=%d)", len(c.crystals)))
		}
	}
	if c.cfg.GrowthChance > 0 && c.rng.Float64() < c.cfg.GrowthChance*mult {
		if idx, ok := c.pickGrowingLocked(); ok {
			c.advanceGrowthLocked(idx)
		}
	}
	if c.cfg.PopChance > 0 && c.rng.Float64() < c.cfg.PopChance*mult {
		startGrowth := int(math.Round(float64(c.cfg.MaxGrowth) * c.cfg.PopGrowth))
		if startGrowth < 1 {
			startGrowth = 1
		}
		if c.spawnNucleusLocked(startGrowth) {
			c.crystals[len(c.crystals)-1].big = true
			c.crystals[len(c.crystals)-1].sparkleTicks = c.cfg.SparkleDur * 2
			c.appendLog("crystal-pop", fmt.Sprintf("popped (g=%d/%d)", startGrowth, c.cfg.MaxGrowth))
		}
	}
	if c.cfg.BurstChance > 0 && c.rng.Float64() < c.cfg.BurstChance*mult {
		c.startBurstLocked("started")
	}
	if c.cfg.QuietChance > 0 && c.quietTicks <= 0 && c.rng.Float64() < c.cfg.QuietChance {
		c.startQuietLocked("started")
	}
}

// maybeIntroSpawnLocked thinly seeds the field across the intro window so
// it reads as growing rather than appearing fully-formed. About one nucleus
// per N ticks where N keeps total intro spawns near MaxCrystals/2.
func (c *CaveCrystals) maybeIntroSpawnLocked() {
	if len(c.crystals) >= c.cfg.MaxCrystals {
		return
	}
	target := c.cfg.MaxCrystals / 2
	if target < 2 {
		target = 2
	}
	cadence := math.Max(1, float64(c.introTotal)/float64(max(1, target)))
	if c.rng.Float64() < 1.0/cadence {
		c.spawnNucleusLocked(0)
	}
}

// maybeFinishStepLocked lets one in-progress crystal finish its current
// growth step during the ending window so the field doesn't freeze
// mid-shimmer when ending fires.
func (c *CaveCrystals) maybeFinishStepLocked() {
	if c.cfg.EndingDur <= 0 || c.endingFade <= 0 {
		return
	}
	if c.rng.Float64() > 1.0/float64(max(1, c.cfg.EndingDur/4)) {
		return
	}
	if idx, ok := c.pickGrowingLocked(); ok {
		c.advanceGrowthLocked(idx)
	}
}

// GridCopy paints the cave + crystal field into a fresh pixel grid. Called
// by the WASM frame() bridge and the server-side Frame() runtime adapter.
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

	dim := 1.0
	if c.endingTicks > 0 && c.endingTotal > 0 {
		// Phase progress at 1 = far into ending. Map to dim factor that
		// settles at ending_dim.
		p := phaseProgress(c.endingTotal, c.endingTicks)
		dim = 1 - (1-c.cfg.EndingDim)*p
	}
	if c.introTicks > 0 && c.introTotal > 0 {
		p := phaseProgress(c.introTotal, c.introTicks)
		// Brightness ramps in: starts at intro_growth fraction, lands at 1.
		base := c.cfg.IntroGrowth
		dim *= base + (1-base)*p
	}
	if dim < 0 {
		dim = 0
	}

	c.paintCaveLocked(grid, dim)
	c.paintFloorLocked(grid, dim)
	for _, cr := range c.crystals {
		c.paintCrystalLocked(grid, cr, dim)
	}
	return grid
}

func (c *CaveCrystals) paintCaveLocked(grid [][]Pixel, dim float64) {
	hue := c.cfg.Hue
	sat := c.cfg.Sat * 0.18
	floor := c.floorBaseRow()
	ambient := clamp01(c.cfg.Ambient * dim)
	for y := 0; y < c.H; y++ {
		t := float64(y) / math.Max(1, float64(c.H-1))
		// Top of the cave is darker; the slice just above the floor picks up
		// a faint lavender-ish wash.
		light := c.cfg.LightMin * (0.18 + 0.55*t)
		light *= dim
		// Add a faint glow within a few rows of the floor based on ambient.
		if ambient > 0 && y < floor {
			distance := math.Max(0, float64(floor-y))
			glow := ambient * math.Exp(-distance/math.Max(2, float64(c.H)*0.2))
			light = math.Min(1, light+glow)
		}
		fillRow(grid, y, hslToRGB(math.Mod(hue+360, 360), sat, clamp01(light)))
	}
}

func (c *CaveCrystals) paintFloorLocked(grid [][]Pixel, dim float64) {
	hue := c.cfg.Hue
	sat := c.cfg.Sat * 0.32
	base := c.floorBaseRow()
	jitter := c.cfg.FloorJitter
	for x := 0; x < c.W; x++ {
		offset := 0.0
		if jitter > 0 {
			offset = math.Sin(float64(x)*0.42)*jitter*0.4 +
				math.Sin(float64(x)*0.13+1.7)*jitter*0.6
		}
		topY := base + int(math.Round(offset))
		if topY < 0 {
			topY = 0
		}
		if topY >= c.H {
			topY = c.H - 1
		}
		for y := topY; y < c.H; y++ {
			t := float64(y-topY) / math.Max(1, float64(c.H-topY))
			light := (c.cfg.LightMin*0.5 + (c.cfg.LightMax-c.cfg.LightMin)*0.06*(1-t)) * dim
			paintPixel(grid, x, y, hslToRGB(math.Mod(hue+340, 360), sat, clamp01(light)))
		}
	}
}

// floorBaseRow returns the un-jittered cave-floor row.
func (c *CaveCrystals) floorBaseRow() int {
	row := int(math.Round(float64(c.H-1) * c.cfg.Baseline))
	if row < 1 {
		row = 1
	}
	if row > c.H-1 {
		row = c.H - 1
	}
	return row
}

func (c *CaveCrystals) paintCrystalLocked(grid [][]Pixel, cr caveCrystal, dim float64) {
	if cr.growth <= 0 || c.cfg.MaxGrowth <= 0 {
		return
	}
	hue := math.Mod(c.cfg.Hue+cr.hueOffset+360, 360)
	sat := c.cfg.Sat
	growth := float64(cr.growth) / float64(c.cfg.MaxGrowth)
	if growth > 1 {
		growth = 1
	}
	height := math.Max(1, math.Round(c.cfg.CrystalHeight*growth))
	halfWidth := math.Max(0, c.cfg.CrystalWidth*growth*0.5)
	if cr.big {
		height *= 1.3
		halfWidth *= 1.3
	}
	col := cr.col
	floorRow := c.floorBaseRow()
	tipRow := floorRow - int(height)
	if tipRow < 0 {
		tipRow = 0
	}
	bodyLight := c.cfg.LightMin + (c.cfg.LightMax-c.cfg.LightMin)*0.55
	edgeLight := c.cfg.LightMin + (c.cfg.LightMax-c.cfg.LightMin)*0.82
	tipLight := c.cfg.LightMax
	bodyLight *= dim
	edgeLight *= dim
	tipLight *= dim
	for row := tipRow; row <= floorRow; row++ {
		// Crystal silhouette tapers from full half-width at the floor to 0
		// at the tip with a slight bell curve so it reads as faceted rather
		// than as a triangle.
		t := float64(floorRow-row) / math.Max(1, float64(floorRow-tipRow))
		shape := math.Pow(1-t, 0.85)
		half := int(math.Round(halfWidth * shape))
		// Variant 1: chunky base. Variant 2: slim. Variant 0: standard.
		switch cr.variant {
		case 1:
			half = max(half, int(math.Round(halfWidth*0.4)))
		case 2:
			if half > 0 {
				half--
			}
		}
		for dx := -half; dx <= half; dx++ {
			x := col + dx
			edge := dx == -half || dx == half
			light := bodyLight
			if edge && half > 0 {
				light = edgeLight
			}
			paintPixel(grid, x, row, hslToRGB(hue, sat, clamp01(light)))
		}
	}
	// Tip highlight + stem facet line for readability.
	paintPixel(grid, col, tipRow, hslToRGB(hue, clamp01(sat*0.85), clamp01(tipLight)))
	if cr.big {
		paintPixel(grid, col-1, tipRow+1, hslToRGB(hue, clamp01(sat*0.7), clamp01(tipLight*0.85)))
		paintPixel(grid, col+1, tipRow+1, hslToRGB(hue, clamp01(sat*0.7), clamp01(tipLight*0.85)))
	}

	// Sparkle pixels around the crystal when sparkleTicks > 0. A handful of
	// short-lived cells around the body. Deterministic per crystal+tick so
	// client/server replicas stay coherent.
	if cr.sparkleTicks > 0 {
		count := 1 + cr.sparkleTicks/6
		seed := uint32(cr.col*131 + cr.variant*97 + c.tick*53)
		for i := 0; i < count; i++ {
			seed = seed*1664525 + 1013904223
			dx := int(seed%5) - 2
			seed = seed*1664525 + 1013904223
			dy := int(seed%5) - 4
			x := col + dx
			y := tipRow + dy
			paintPixel(grid, x, y, hslToRGB(hue, clamp01(sat*0.9), clamp01(tipLight)))
		}
	}
}

func fillRow(grid [][]Pixel, y int, c color.RGBA) {
	if y < 0 || y >= len(grid) {
		return
	}
	row := grid[y]
	for x := range row {
		row[x] = Pixel{Filled: true, C: c}
	}
}
