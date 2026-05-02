package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// CaveCrystals is a long-arc effect: in a dim cave, angular crystals
// nucleate on the floor and grow incrementally over the lifetime of the
// scene. The server is authoritative about which crystals exist, where
// they sit, and which growth step they're on; clients run a local replica
// so the field stays synced even when sparkle particles drift per client.
//
// Per-crystal lifecycle:
//
//	step 0      — invisible nucleus seed
//	step 1..N-1 — growing (each step bumps height by one chunk)
//	step N      — fully grown
//
// The scene-wide lifecycle wraps around the per-crystal state:
//
//	intro   — fast-spawn a small cluster so the cave isn't empty
//	steady  — slow nucleation + slow growth, with rare events
//	ending  — stop spawning, finish in-progress steps, dim to silhouette
//	(after ending the field auto-resets and a fresh intro begins)

// CaveCrystal is one crystal's authoritative state. The list is broadcast
// in full on snapshot so a join-in-progress client sees the same cave the
// authority is rendering.
type CaveCrystal struct {
	X       float64 `json:"x"`             // floor column (0..W)
	BaseRow int     `json:"row"`           // soil row this crystal stands on
	Step    int     `json:"s"`             // current growth step (0..MaxStep)
	MaxStep int     `json:"m"`             // target growth step
	Height  float64 `json:"h"`             // target full height in pixels at MaxStep
	Width   float64 `json:"w"`             // half-width of the silhouette base, in cells
	Tilt    float64 `json:"t"`             // -1..1 horizontal lean per growth row
	HueOff  float64 `json:"hu"`            // per-crystal hue offset (degrees)
	Timer   int     `json:"tm"`            // ticks until next growth step
	Sparkle int     `json:"sp,omitempty"`  // sparkle ticks remaining (decoration)
	Popped  bool    `json:"pop,omitempty"` // true if this crystal was a pop-in event
}

// CaveCrystalsConfig tunes the cave-crystals prototype.
type CaveCrystalsConfig struct {
	// INTRODUCTION
	IntroDur   int `json:"intro_dur"`
	IntroSeed  int `json:"intro_seed"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingResid  float64 `json:"ending_resid"`
	// LEVERS — field
	MaxCount  int     `json:"max_count"`
	Baseline  float64 `json:"baseline"`
	FloorBump float64 `json:"floor_bump"`
	// LEVERS — growth
	NucleateP    float64 `json:"nucleate_p"`
	GrowthDur    int     `json:"growth_dur"`
	MaxSteps     int     `json:"max_steps"`
	MinSize      float64 `json:"min_size"`
	MaxSize      float64 `json:"max_size"`
	WidthRatio   float64 `json:"width_ratio"`
	SparkleDur   int     `json:"sparkle_dur"`
	// LEVERS — color
	Hue       float64 `json:"hue"`
	HueSpread float64 `json:"hue_sp"`
	Sat       float64 `json:"sat"`
	LightMin  float64 `json:"lmin"`
	LightMax  float64 `json:"lmax"`
	Glow      float64 `json:"glow"`
	// EVENT CHANCES
	PopChance     float64 `json:"pop_p"`
	BurstChance   float64 `json:"burst_p"`
	QuietChance   float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	PopSizeMult    float64 `json:"pop_mult"`
	BurstDur       int     `json:"burst_dur"`
	BurstCount     int     `json:"burst_count"`
	QuietDur       int     `json:"quiet_dur"`
	QuietGrowthMul float64 `json:"quiet_mult"`
}

func (c CaveCrystalsConfig) withDefaults() CaveCrystalsConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 80
	}
	if c.IntroSeed < 0 {
		c.IntroSeed = 0
	}
	if c.IntroSeed == 0 {
		c.IntroSeed = 4
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 90
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	c.EndingResid = clamp01(c.EndingResid)
	if c.EndingResid == 0 {
		c.EndingResid = 0.18
	}
	if c.MaxCount <= 0 {
		c.MaxCount = 32
	}
	if c.MaxCount > 96 {
		c.MaxCount = 96
	}
	if c.Baseline <= 0 {
		c.Baseline = 0.82
	}
	if c.FloorBump < 0 {
		c.FloorBump = 0
	}
	if c.FloorBump == 0 {
		c.FloorBump = 1.5
	}
	if c.NucleateP < 0 {
		c.NucleateP = 0
	}
	if c.NucleateP == 0 {
		c.NucleateP = 0.012
	}
	if c.GrowthDur <= 0 {
		c.GrowthDur = 110
	}
	if c.MaxSteps <= 0 {
		c.MaxSteps = 5
	}
	if c.MinSize <= 0 {
		c.MinSize = 4
	}
	if c.MaxSize <= 0 {
		c.MaxSize = 12
	}
	if c.MaxSize < c.MinSize {
		c.MinSize, c.MaxSize = c.MaxSize, c.MinSize
	}
	if c.WidthRatio <= 0 {
		c.WidthRatio = 0.35
	}
	if c.SparkleDur <= 0 {
		c.SparkleDur = 12
	}
	if c.Hue == 0 {
		c.Hue = 280
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.Sat <= 0 {
		c.Sat = 0.55
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.22
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.78
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.Glow < 0 {
		c.Glow = 0
	}
	if c.Glow == 0 {
		c.Glow = 0.4
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
	if c.PopSizeMult <= 0 {
		c.PopSizeMult = 1.4
	}
	if c.BurstDur <= 0 {
		c.BurstDur = 30
	}
	if c.BurstCount <= 0 {
		c.BurstCount = 4
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 200
	}
	if c.QuietGrowthMul <= 0 {
		c.QuietGrowthMul = 0.3
	}
	if c.QuietGrowthMul > 1 {
		c.QuietGrowthMul = 1
	}
	return c
}

// CaveCrystalsSchema describes the cave-crystals effect's tunable knobs.
func CaveCrystalsSchema() EffectSchema {
	return EffectSchema{
		Name: "cave-crystals",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80, Trigger: "intro",
				Description: "Ticks spent dropping the initial cluster of nuclei before the slow steady growth takes over."},
			{Key: "intro_seed", Label: "intro seed", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 4,
				Description: "Number of crystal nuclei the intro plants up front so the cave doesn't read as empty."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 90, Trigger: "ending",
				Description: "Ticks spent dimming the field toward silhouette while in-progress crystals finish their last step."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 40,
				Description: "Extra still ticks after the dim-out so the dark cave can hold the frame before reset."},
			{Key: "ending_resid", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Fraction of crystal lightness that lingers as silhouette as the cave goes dark."},
			{Key: "max_count", Label: "max count", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 4, Max: 96, Step: 2, Default: 32,
				Description: "Cap on the number of crystals in the field. Higher values pack the floor more densely."},
			{Key: "baseline", Label: "baseline", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.5, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Vertical position of the cave floor as a fraction of the frame height."},
			{Key: "floor_bump", Label: "floor bump", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0, Max: 4, Step: 0.5, Default: 1.5,
				Description: "Maximum vertical wobble of the cave floor — small values read as flat, larger as a lumpy cavern."},
			{Key: "nucleate_p", Label: "nucleate", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0, Max: 0.08, Step: 0.001, Default: 0.012,
				Description: "Per-tick chance of a brand-new crystal seeding at a random floor point."},
			{Key: "growth_dur", Label: "growth dur", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 20, Max: 600, Step: 5, Default: 110,
				Description: "Ticks between each growth step on a single crystal — slower values feel like geology, faster like time-lapse."},
			{Key: "max_steps", Label: "max steps", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 2, Max: 8, Step: 1, Default: 5,
				Description: "Number of discrete growth steps a crystal cycles through before reaching full size."},
			{Key: "min_size", Label: "min size", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 2, Max: 16, Step: 0.5, Default: 4,
				Description: "Minimum target height in pixels for a fully-grown crystal."},
			{Key: "max_size", Label: "max size", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 4, Max: 28, Step: 0.5, Default: 12,
				Description: "Maximum target height in pixels for a fully-grown crystal."},
			{Key: "width_ratio", Label: "width", Slot: SlotLever, Group: "growth", Type: KnobFloat, Min: 0.15, Max: 0.7, Step: 0.02, Default: 0.35,
				Description: "Crystal half-width relative to its full height. Lower values lean shard-like, higher lean stout."},
			{Key: "sparkle_dur", Label: "sparkle dur", Slot: SlotLever, Group: "growth", Type: KnobInt, Min: 4, Max: 60, Step: 1, Default: 12,
				Description: "Ticks a sparkle decoration sticks around after a growth step or burst."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 340, Step: 1, Default: 280,
				Description: "Base crystal hue. Lower values lean teal/cyan; mid values lean violet; higher lean magenta."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 24,
				Description: "Hue variation across crystals. Small values feel uniform, large values feel like a mixed-mineral seam."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.02, Default: 0.55,
				Description: "Overall crystal saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.22,
				Description: "Minimum lightness used for the dimmest crystal edges and floor silhouette."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for the brightest crystal cores and sparkle accents."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.4,
				Description: "Strength of the soft cool light cast around fully-grown crystals."},
			{Key: "pop_p", Label: "pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "crystal-pop",
				Description: "Per-tick chance of a fully-grown crystal popping into existence at once."},
			{Key: "burst_p", Label: "sparkle burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "sparkle-burst",
				Description: "Per-tick chance of an extra burst of sparkles around an existing crystal cluster."},
			{Key: "quiet_p", Label: "quiet cave", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "quiet-cave",
				Description: "Per-tick chance of a long suppression window where new spawns stop and growth crawls."},
			{Key: "pop_mult", Label: "pop x", Slot: SlotEventMod, Group: "crystal-pop", Type: KnobFloat, Min: 1, Max: 2.5, Step: 0.05, Default: 1.4,
				Description: "Size multiplier applied to a popped-in crystal versus the baseline target size."},
			{Key: "burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "sparkle-burst", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 30,
				Description: "How long the sparkles in a burst remain visible."},
			{Key: "burst_count", Label: "burst count", Slot: SlotEventMod, Group: "sparkle-burst", Type: KnobInt, Min: 1, Max: 12, Step: 1, Default: 4,
				Description: "How many crystals get sparkled at once during a burst."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-cave", Type: KnobInt, Min: 20, Max: 600, Step: 10, Default: 200,
				Description: "Duration of the quiet-cave suppression window."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-cave", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.3,
				Description: "Growth-rate multiplier applied while the cave is quiet."},
		},
	}
}

// CaveCrystalsState is the wire/persisted snapshot of the cave field.
type CaveCrystalsState struct {
	Tick         int           `json:"tick"`
	Crystals     []CaveCrystal `json:"crystals"`
	IntroTicks   int           `json:"introTicks"`
	IntroTotal   int           `json:"introTotal"`
	EndingTicks  int           `json:"endingTicks"`
	EndingTotal  int           `json:"endingTotal"`
	EndingFade   int           `json:"endingFade"`
	QuietTicks   int           `json:"quietTicks"`
	BurstTicks   int           `json:"burstTicks"`
	FloorProfile []float64     `json:"floor"`
	RNGState     uint64        `json:"rngState,omitempty"`
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

	crystals []CaveCrystal
	floor    []float64

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int
	quietTicks  int
	burstTicks  int

	log []LogEntry
}

func NewCaveCrystals(w, h int, seed int64, cfg CaveCrystalsConfig) *CaveCrystals {
	cc := &CaveCrystals{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	cc.regenFloorLocked()
	cc.startIntroLocked()
	return cc
}

func (c *CaveCrystals) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.W = w
	c.H = h
	c.regenFloorLocked()
}

func (c *CaveCrystals) SetConfig(cfg CaveCrystalsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg.withDefaults()
	if len(c.crystals) > c.cfg.MaxCount {
		c.crystals = c.crystals[:c.cfg.MaxCount]
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
	return CaveCrystalsSnapshot{CaveCrystalsState: c.snapshotStateLocked(false)}
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
	crystals := make([]CaveCrystal, len(c.crystals))
	copy(crystals, c.crystals)
	floor := make([]float64, len(c.floor))
	copy(floor, c.floor)
	out := CaveCrystalsState{
		Tick:         c.tick,
		Crystals:     crystals,
		IntroTicks:   c.introTicks,
		IntroTotal:   c.introTotal,
		EndingTicks:  c.endingTicks,
		EndingTotal:  c.endingTotal,
		EndingFade:   c.endingFade,
		QuietTicks:   c.quietTicks,
		BurstTicks:   c.burstTicks,
		FloorProfile: floor,
	}
	if includeRNG {
		out.RNGState = c.rng.State()
	}
	return out
}

func (c *CaveCrystals) restoreStateLocked(s CaveCrystalsState) {
	c.tick = s.Tick
	if len(s.Crystals) > 0 {
		c.crystals = make([]CaveCrystal, len(s.Crystals))
		copy(c.crystals, s.Crystals)
	} else {
		c.crystals = nil
	}
	c.introTicks = s.IntroTicks
	c.introTotal = s.IntroTotal
	c.endingTicks = s.EndingTicks
	c.endingTotal = s.EndingTotal
	c.endingFade = s.EndingFade
	c.quietTicks = s.QuietTicks
	c.burstTicks = s.BurstTicks
	if len(s.FloorProfile) > 0 {
		c.floor = make([]float64, len(s.FloorProfile))
		copy(c.floor, s.FloorProfile)
	} else {
		c.regenFloorLocked()
	}
}

func (c *CaveCrystals) regenFloorLocked() {
	if c.W <= 0 {
		c.floor = nil
		return
	}
	c.floor = make([]float64, c.W)
	bump := c.cfg.FloorBump
	for x := 0; x < c.W; x++ {
		// Smooth low-frequency wobble — not RNG-driven, so the floor stays
		// stable across snapshot/restore even without persisting the curve.
		t := float64(x) / math.Max(1, float64(c.W))
		w := math.Sin(t*math.Pi*1.6+0.7)*0.55 + math.Sin(t*math.Pi*4.1+1.9)*0.3
		c.floor[x] = w * bump
	}
}

// floorRowAt returns the soil row this column sits on, derived from the
// pre-baked floor profile + base baseline.
func (c *CaveCrystals) floorRowAt(x float64) int {
	if c.W <= 0 || c.H <= 0 {
		return 0
	}
	col := int(math.Round(x))
	if col < 0 {
		col = 0
	}
	if col >= c.W {
		col = c.W - 1
	}
	bump := 0.0
	if col < len(c.floor) {
		bump = c.floor[col]
	}
	base := math.Floor(float64(c.H) * c.cfg.Baseline)
	row := int(base + bump)
	if row < 1 {
		row = 1
	}
	if row >= c.H {
		row = c.H - 1
	}
	return row
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (c *CaveCrystals) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "nucleus-spawn":
		if c.spawnCrystalLocked(false) {
			c.appendLog("nucleus-spawn", fmt.Sprintf("seeded (count=%d/%d)", len(c.crystals), c.cfg.MaxCount))
		} else {
			c.appendLog("nucleus-spawn", "skipped (field full)")
		}
	case "crystal-pop":
		if c.spawnCrystalLocked(true) {
			c.appendLog("crystal-pop", fmt.Sprintf("popped (count=%d/%d)", len(c.crystals), c.cfg.MaxCount))
		} else {
			c.appendLog("crystal-pop", "skipped (field full)")
		}
	case "growth-pulse":
		if idx, ok := c.pickGrowingLocked(); ok {
			c.advanceCrystalLocked(idx, "triggered")
		} else {
			c.appendLog("growth-pulse", "skipped (none growing)")
		}
	case "sparkle-burst":
		c.startSparkleBurstLocked("triggered")
	case "quiet-cave":
		c.startQuietLocked("triggered")
	case "intro":
		c.startIntroLocked()
		c.appendLog("intro", fmt.Sprintf("started (dur=%d, seed=%d)", c.introTotal, c.cfg.IntroSeed))
	case "ending":
		c.startEndingLocked()
		c.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", c.endingFade, c.endingTotal-c.endingFade))
	default:
		return false
	}
	return true
}

func (c *CaveCrystals) startIntroLocked() {
	c.crystals = c.crystals[:0]
	c.endingTicks = 0
	c.endingTotal = 0
	c.endingFade = 0
	c.quietTicks = 0
	c.burstTicks = 0
	c.introTotal = c.cfg.IntroDur
	if c.introTotal <= 0 {
		c.introTotal = 80
	}
	c.introTicks = c.introTotal
	// Drop a couple of nuclei at intro entry so the first frame isn't
	// stark empty floor — the rest of the seed cluster fills in over
	// the intro window.
	burst := c.cfg.IntroSeed / 2
	if burst < 1 {
		burst = 1
	}
	for i := 0; i < burst; i++ {
		c.spawnCrystalLocked(false)
	}
}

func (c *CaveCrystals) startEndingLocked() {
	c.introTicks = 0
	c.introTotal = 0
	c.quietTicks = 0
	c.burstTicks = 0
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

func (c *CaveCrystals) startSparkleBurstLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.BurstDur, 0.25)
	c.burstTicks = dur
	count := c.cfg.BurstCount
	if count < 1 {
		count = 1
	}
	if count > len(c.crystals) {
		count = len(c.crystals)
	}
	for i := 0; i < count && len(c.crystals) > 0; i++ {
		idx := c.rng.Intn(len(c.crystals))
		c.crystals[idx].Sparkle = dur
	}
	c.appendLog("sparkle-burst", fmt.Sprintf("%s (dur=%d, count=%d)", verb, dur, count))
}

func (c *CaveCrystals) startQuietLocked(verb string) {
	dur := jitterInt(c.rng, c.cfg.QuietDur, 0.3)
	c.quietTicks = dur
	c.appendLog("quiet-cave", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, c.cfg.QuietGrowthMul))
}

// spawnCrystalLocked seeds one crystal at a random floor column. If popped
// is true the crystal lands fully grown with a size bump (crystal-pop event).
func (c *CaveCrystals) spawnCrystalLocked(popped bool) bool {
	if c.W <= 0 || c.H <= 0 {
		return false
	}
	if len(c.crystals) >= c.cfg.MaxCount {
		return false
	}
	x := c.rng.Float64() * float64(c.W)
	hueJ := (c.rng.Float64()*2 - 1) * c.cfg.HueSpread
	tilt := (c.rng.Float64()*2 - 1) * 0.4
	heightFrac := c.rng.Float64()
	target := c.cfg.MinSize + (c.cfg.MaxSize-c.cfg.MinSize)*heightFrac
	if popped {
		target *= c.cfg.PopSizeMult
	}
	width := math.Max(1, target*c.cfg.WidthRatio*(0.7+c.rng.Float64()*0.6))
	cr := CaveCrystal{
		X:       x,
		BaseRow: c.floorRowAt(x),
		Step:    0,
		MaxStep: max(1, c.cfg.MaxSteps),
		Height:  target,
		Width:   width,
		Tilt:    tilt,
		HueOff:  hueJ,
		Timer:   c.cfg.GrowthDur,
		Popped:  popped,
	}
	if popped {
		cr.Step = cr.MaxStep
		cr.Sparkle = c.cfg.SparkleDur * 2
	}
	c.crystals = append(c.crystals, cr)
	return true
}

// pickGrowingLocked picks a random crystal that hasn't reached its full size.
func (c *CaveCrystals) pickGrowingLocked() (int, bool) {
	candidates := make([]int, 0, len(c.crystals))
	for i, cr := range c.crystals {
		if cr.Step < cr.MaxStep {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[c.rng.Intn(len(candidates))], true
}

// advanceCrystalLocked bumps one crystal's growth step and resets its timer.
func (c *CaveCrystals) advanceCrystalLocked(idx int, source string) {
	if idx < 0 || idx >= len(c.crystals) {
		return
	}
	cr := &c.crystals[idx]
	if cr.Step >= cr.MaxStep {
		return
	}
	cr.Step++
	cr.Timer = c.crystalGrowthIntervalLocked()
	cr.Sparkle = c.cfg.SparkleDur
	if source != "" {
		c.appendLog("growth-pulse", fmt.Sprintf("crystal %d → step %d/%d (%s)", idx+1, cr.Step, cr.MaxStep, source))
	}
}

// crystalGrowthIntervalLocked returns the per-crystal timer reset value
// after a growth step, factoring in the quiet-cave slowdown.
func (c *CaveCrystals) crystalGrowthIntervalLocked() int {
	dur := c.cfg.GrowthDur
	if c.quietTicks > 0 && c.cfg.QuietGrowthMul > 0 {
		dur = int(math.Round(float64(dur) / c.cfg.QuietGrowthMul))
	}
	if dur < 1 {
		dur = 1
	}
	return jitterInt(c.rng, dur, 0.2)
}

func (c *CaveCrystals) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++

	if c.endingTicks > 0 {
		c.endingTicks--
		if c.endingTicks == 0 {
			c.startIntroLocked()
			c.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, seed=%d)", c.introTotal, c.cfg.IntroSeed))
		}
	} else if c.introTicks > 0 {
		c.introTicks--
	}

	if c.quietTicks > 0 {
		c.quietTicks--
	}
	if c.burstTicks > 0 {
		c.burstTicks--
	}

	c.advanceSparklesLocked()
	c.advanceGrowthLocked()

	if c.endingTicks > 0 {
		// During the outro we never seed and we've already advanced any
		// in-progress growth above. Skip the regular event/spawn rolls.
		return
	}

	// Intro spawns: distribute the seed cluster across the intro window so
	// the first batch isn't a single popcorn frame.
	if c.introTicks > 0 && len(c.crystals) < c.cfg.IntroSeed {
		spawnRate := float64(c.cfg.IntroSeed) / math.Max(1, float64(c.introTotal))
		if c.rng.Float64() < spawnRate*1.4 {
			c.spawnCrystalLocked(false)
		}
		return
	}

	// Steady-state: nucleate, roll for events.
	nucP := c.cfg.NucleateP
	if c.quietTicks > 0 {
		nucP *= c.cfg.QuietGrowthMul * 0.6
	}
	if nucP > 0 && c.rng.Float64() < nucP {
		c.spawnCrystalLocked(false)
	}
	if c.cfg.PopChance > 0 && c.rng.Float64() < c.cfg.PopChance {
		if c.spawnCrystalLocked(true) {
			c.appendLog("crystal-pop", fmt.Sprintf("rolled (count=%d/%d)", len(c.crystals), c.cfg.MaxCount))
		}
	}
	if c.cfg.BurstChance > 0 && c.rng.Float64() < c.cfg.BurstChance {
		c.startSparkleBurstLocked("rolled")
	}
	if c.cfg.QuietChance > 0 && c.quietTicks <= 0 && c.rng.Float64() < c.cfg.QuietChance {
		c.startQuietLocked("rolled")
	}
}

func (c *CaveCrystals) advanceGrowthLocked() {
	for i := range c.crystals {
		cr := &c.crystals[i]
		if cr.Step >= cr.MaxStep {
			continue
		}
		if cr.Timer > 0 {
			cr.Timer--
		}
		if cr.Timer > 0 {
			continue
		}
		c.advanceCrystalLocked(i, "")
	}
}

func (c *CaveCrystals) advanceSparklesLocked() {
	for i := range c.crystals {
		cr := &c.crystals[i]
		if cr.Sparkle > 0 {
			cr.Sparkle--
		}
	}
}
