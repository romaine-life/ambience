package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// BurningTrees is a row of pixel-art trees along the bottom of the frame.
// Individual trees ignite, burn down through a flame stage, and resolve into
// charred stumps before the row eventually clears via the ending lifecycle.
//
// The server is authoritative about *which* tree is burning, *for how long*,
// and *which neighbor catches next*. Clients run a local replica using the
// same per-tree state so flames stay synced even though the inner pixel
// noise (flame turbulence, ember motion) drifts per client.
//
// Per-tree state is one of:
//   alive (0)    — healthy tree, full canopy
//   igniting (1) — flame is catching, canopy still mostly there
//   burning (2)  — full flame, canopy burning down
//   ashing (3)   — flame guttering out, only stumps + smoke
//   ash (4)      — burnt stump, no flame
//
// Each non-alive tree carries a phase timer that ticks down toward the next
// stage. ignite -> burn -> ash -> stump linger -> alive (during ending).

const (
	BTreeStateAlive byte = iota
	BTreeStateIgniting
	BTreeStateBurning
	BTreeStateAshing
	BTreeStateAsh
)

// BurningTreesConfig tunes the burning-trees prototype used in isolated dev
// sessions. See BurningTreesSchema for the full knob inventory.
type BurningTreesConfig struct {
	// INTRODUCTION
	IntroDur    int     `json:"intro_dur"`
	IntroGrowth float64 `json:"intro_growth"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingAsh    float64 `json:"ending_ash"`
	// LEVERS — grove
	TreeCount int     `json:"tree_count"`
	TreeWidth float64 `json:"tree_width"`
	TreeMinH  float64 `json:"tree_min_h"`
	TreeMaxH  float64 `json:"tree_max_h"`
	Baseline  float64 `json:"baseline"`
	Canopy    float64 `json:"canopy"`
	// LEVERS — fire
	IgniteDur int     `json:"ignite_dur"`
	BurnDur   int     `json:"burn_dur"`
	AshDur    int     `json:"ash_dur"`
	SpreadP   float64 `json:"spread_p"`
	FlameH    float64 `json:"flame_h"`
	Flicker   float64 `json:"flicker"`
	EmberRate float64 `json:"ember_rate"`
	Glow      float64 `json:"glow"`
	Smoke     float64 `json:"smoke"`
	// LEVERS — color (canopy / flame / ash)
	CanopyHue float64 `json:"canopy_hue"`
	FlameHue  float64 `json:"flame_hue"`
	HueSpread float64 `json:"hue_sp"`
	Sat       float64 `json:"sat"`
	LightMin  float64 `json:"lmin"`
	LightMax  float64 `json:"lmax"`
	// EVENT CHANCES
	IgniteChance float64 `json:"ignite_p"`
	FlareChance  float64 `json:"flare_p"`
	LullChance   float64 `json:"lull_p"`
	// EVENT MODIFIERS
	FlareDur  int     `json:"flare_dur"`
	FlareMult float64 `json:"flare_mult"`
	LullDur   int     `json:"lull_dur"`
	LullMult  float64 `json:"lull_mult"`
}

func (c BurningTreesConfig) withDefaults() BurningTreesConfig {
	if c.IntroDur == 0 && c.IntroGrowth == 0 {
		c.IntroDur = 60
		c.IntroGrowth = 0.18
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 60
		}
		if c.IntroGrowth < 0 {
			c.IntroGrowth = 0
		}
	}
	c.IntroGrowth = clamp01(c.IntroGrowth)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingAsh == 0 {
		c.EndingDur = 80
		c.EndingLinger = 30
		c.EndingAsh = 0.35
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 80
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingAsh < 0 {
			c.EndingAsh = 0
		}
	}
	c.EndingAsh = clamp01(c.EndingAsh)
	if c.TreeCount <= 0 {
		c.TreeCount = 9
	}
	if c.TreeCount > 24 {
		c.TreeCount = 24
	}
	if c.TreeWidth <= 0 {
		c.TreeWidth = 7
	}
	if c.TreeMinH <= 0 {
		c.TreeMinH = 8
	}
	if c.TreeMaxH <= 0 {
		c.TreeMaxH = 16
	}
	if c.TreeMaxH < c.TreeMinH {
		c.TreeMinH, c.TreeMaxH = c.TreeMaxH, c.TreeMinH
	}
	if c.Baseline <= 0 {
		c.Baseline = 0.86
	}
	if c.Canopy <= 0 {
		c.Canopy = 0.62
	}
	if c.IgniteDur <= 0 {
		c.IgniteDur = 30
	}
	if c.BurnDur <= 0 {
		c.BurnDur = 220
	}
	if c.AshDur <= 0 {
		c.AshDur = 80
	}
	if c.SpreadP < 0 {
		c.SpreadP = 0
	}
	if c.FlameH <= 0 {
		c.FlameH = 9
	}
	if c.Flicker <= 0 {
		c.Flicker = 0.7
	}
	if c.EmberRate < 0 {
		c.EmberRate = 0
	}
	if c.Glow <= 0 {
		c.Glow = 0.45
	}
	if c.Smoke < 0 {
		c.Smoke = 0
	}
	if c.CanopyHue == 0 {
		c.CanopyHue = 118
	}
	if c.FlameHue == 0 {
		c.FlameHue = 22
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
		c.LightMax = 0.82
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.IgniteChance < 0 {
		c.IgniteChance = 0
	}
	if c.FlareChance < 0 {
		c.FlareChance = 0
	}
	if c.LullChance < 0 {
		c.LullChance = 0
	}
	if c.FlareDur <= 0 {
		c.FlareDur = 36
	}
	if c.FlareMult <= 0 {
		c.FlareMult = 1.7
	}
	if c.LullDur <= 0 {
		c.LullDur = 60
	}
	if c.LullMult <= 0 {
		c.LullMult = 0.55
	}
	return c
}

// BurningTreesSchema describes the burning-trees effect's tunable knobs for
// the dev UI.
func BurningTreesSchema() EffectSchema {
	return EffectSchema{
		Name: "burning-trees",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent growing the row from sprouts into readable trees before fire is allowed."},
			{Key: "intro_growth", Label: "intro growth", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Starting canopy fraction during the intro before each tree finishes growing in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent extinguishing remaining flames and dimming the grove."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 30,
				Description: "Extra still ticks after the flames are out so charred stumps can hold the frame."},
			{Key: "ending_ash", Label: "ending ash", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Fraction of ash silhouettes that linger into the outro instead of dropping straight to soil."},
			{Key: "tree_count", Label: "tree count", Slot: SlotLever, Group: "grove", Type: KnobInt, Min: 3, Max: 24, Step: 1, Default: 9,
				Description: "Number of trees in the row. Lower values read as a copse; higher values read as a tree line."},
			{Key: "tree_width", Label: "tree width", Slot: SlotLever, Group: "grove", Type: KnobFloat, Min: 4, Max: 14, Step: 0.5, Default: 7,
				Description: "Horizontal cells each tree occupies in the row."},
			{Key: "tree_min_h", Label: "min height", Slot: SlotLever, Group: "grove", Type: KnobFloat, Min: 4, Max: 24, Step: 0.5, Default: 8,
				Description: "Minimum tree height in pixels above the soil."},
			{Key: "tree_max_h", Label: "max height", Slot: SlotLever, Group: "grove", Type: KnobFloat, Min: 6, Max: 32, Step: 0.5, Default: 16,
				Description: "Maximum tree height in pixels above the soil."},
			{Key: "baseline", Label: "baseline", Slot: SlotLever, Group: "grove", Type: KnobFloat, Min: 0.5, Max: 0.95, Step: 0.01, Default: 0.86,
				Description: "Vertical position of the soil line as a fraction of the frame height."},
			{Key: "canopy", Label: "canopy", Slot: SlotLever, Group: "grove", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.02, Default: 0.62,
				Description: "How dense the canopy reads; lower values feel sparse and skeletal."},
			{Key: "ignite_dur", Label: "ignite dur", Slot: SlotLever, Group: "fire", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 30,
				Description: "Ticks a tree spends in the early igniting phase before the full burn takes hold."},
			{Key: "burn_dur", Label: "burn dur", Slot: SlotLever, Group: "fire", Type: KnobInt, Min: 60, Max: 600, Step: 10, Default: 220,
				Description: "Ticks each tree stays in the bright burning phase before the canopy is consumed."},
			{Key: "ash_dur", Label: "ash dur", Slot: SlotLever, Group: "fire", Type: KnobInt, Min: 20, Max: 300, Step: 5, Default: 80,
				Description: "Ticks the dim guttering ashing phase lasts before only the bare stump remains."},
			{Key: "spread_p", Label: "spread", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 0, Max: 0.06, Step: 0.001, Default: 0.012,
				Description: "Per-tick chance per burning tree of igniting an adjacent neighbor."},
			{Key: "flame_h", Label: "flame height", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 3, Max: 22, Step: 0.5, Default: 9,
				Description: "How tall the flame plume rises above each burning tree's canopy."},
			{Key: "flicker", Label: "flicker", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.05, Default: 0.7,
				Description: "Side-to-side and height variation inside the flame body."},
			{Key: "ember_rate", Label: "embers", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.32,
				Description: "How many embers rise from each burning tree."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.02, Default: 0.45,
				Description: "Strength of the warm light cast around each burning tree."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "fire", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.45,
				Description: "How much smoke trails up from burning and ashing trees."},
			{Key: "canopy_hue", Label: "canopy hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 60, Max: 200, Step: 1, Default: 118,
				Description: "Base canopy hue for healthy trees. Lower values lean yellow-green; higher lean blue-green."},
			{Key: "flame_hue", Label: "flame hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 22,
				Description: "Base flame hue. Lower values lean redder; higher values lean orange-yellow."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 14,
				Description: "Variation across canopy and flame tones between trees."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.02, Default: 0.62,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for ash, charred trunks, and the outer flame edge."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.45, Max: 1, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for sunlit canopies and the hottest flame cores."},
			{Key: "ignite_p", Label: "ignite", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "ignite",
				Description: "Per-tick chance of a fresh tree catching fire on its own."},
			{Key: "flare_p", Label: "flare", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "flare",
				Description: "Per-tick chance of the active flames briefly intensifying together."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the active flames briefly dropping into a calmer burn."},
			{Key: "flare_dur", Label: "flare dur", Slot: SlotEventMod, Group: "flare", Type: KnobInt, Min: 8, Max: 160, Step: 4, Default: 36,
				Description: "Duration of a brighter flare across active flames."},
			{Key: "flare_mult", Label: "flare x", Slot: SlotEventMod, Group: "flare", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.7,
				Description: "Brightness multiplier applied while a flare is active."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60,
				Description: "Duration of the calmer lower-flame window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Brightness multiplier applied while a lull is active."},
		},
	}
}

// BurningTreesState is the wire/persisted snapshot of the burn row.
type BurningTreesState struct {
	Tick        int     `json:"tick"`
	States      []byte  `json:"states"`
	PhaseLeft   []int   `json:"phaseLeft"`
	PhaseTotal  []int   `json:"phaseTotal"`
	IntroTicks  int     `json:"introTicks"`
	IntroTotal  int     `json:"introTotal"`
	EndingTicks int     `json:"endingTicks"`
	EndingTotal int     `json:"endingTotal"`
	EndingFade  int     `json:"endingFade"`
	FlareTicks  int     `json:"flareTicks"`
	FlareGain   float64 `json:"flareGain"`
	LullTicks   int     `json:"lullTicks"`
	RNGState    uint64  `json:"rngState,omitempty"`
}

type BurningTreesSnapshot struct {
	BurningTreesState
}

type BurningTreesPersistedState struct {
	BurningTreesState
}

// BurningTrees is the authoritative server-side burn row.
type BurningTrees struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  BurningTreesConfig
	tick int

	states     []byte
	phaseLeft  []int
	phaseTotal []int

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int

	flareTicks int
	flareGain  float64
	lullTicks  int

	log []LogEntry
}

func NewBurningTrees(w, h int, seed int64, cfg BurningTreesConfig) *BurningTrees {
	bt := &BurningTrees{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	bt.resetTreesLocked()
	return bt
}

func (b *BurningTrees) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.W = w
	b.H = h
}

func (b *BurningTrees) SetConfig(cfg BurningTreesConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	next := cfg.withDefaults()
	if next.TreeCount != b.cfg.TreeCount {
		b.cfg = next
		b.resetTreesLocked()
		return
	}
	b.cfg = next
}

func (b *BurningTrees) EffectiveConfig() BurningTreesConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

func (b *BurningTrees) CurrentTick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tick
}

func (b *BurningTrees) PerturbRNG(delta int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rng.Mix(delta)
}

func (b *BurningTrees) DrainLog() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) == 0 {
		return nil
	}
	out := b.log
	b.log = nil
	return out
}

func (b *BurningTrees) appendLog(kind, desc string) {
	b.log = append(b.log, LogEntry{Tick: b.tick, Type: kind, Desc: desc})
	if len(b.log) > 200 {
		b.log = b.log[len(b.log)-200:]
	}
}

func (b *BurningTrees) Snapshot() BurningTreesSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BurningTreesSnapshot{BurningTreesState: b.snapshotStateLocked(false)}
}

func (b *BurningTrees) RestoreSnapshot(s BurningTreesSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.BurningTreesState)
}

func (b *BurningTrees) SnapshotPersistedState() BurningTreesPersistedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BurningTreesPersistedState{BurningTreesState: b.snapshotStateLocked(true)}
}

func (b *BurningTrees) RestorePersistedState(s BurningTreesPersistedState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.BurningTreesState)
	if s.RNGState != 0 {
		b.rng.SetState(s.RNGState)
	}
}

func (b *BurningTrees) snapshotStateLocked(includeRNG bool) BurningTreesState {
	states := make([]byte, len(b.states))
	copy(states, b.states)
	left := make([]int, len(b.phaseLeft))
	copy(left, b.phaseLeft)
	total := make([]int, len(b.phaseTotal))
	copy(total, b.phaseTotal)
	out := BurningTreesState{
		Tick:        b.tick,
		States:      states,
		PhaseLeft:   left,
		PhaseTotal:  total,
		IntroTicks:  b.introTicks,
		IntroTotal:  b.introTotal,
		EndingTicks: b.endingTicks,
		EndingTotal: b.endingTotal,
		EndingFade:  b.endingFade,
		FlareTicks:  b.flareTicks,
		FlareGain:   b.flareGain,
		LullTicks:   b.lullTicks,
	}
	if includeRNG {
		out.RNGState = b.rng.State()
	}
	return out
}

func (b *BurningTrees) restoreStateLocked(s BurningTreesState) {
	b.tick = s.Tick
	if len(s.States) > 0 {
		b.states = make([]byte, len(s.States))
		copy(b.states, s.States)
	} else {
		b.resetTreesLocked()
	}
	if len(s.PhaseLeft) == len(b.states) {
		b.phaseLeft = make([]int, len(s.PhaseLeft))
		copy(b.phaseLeft, s.PhaseLeft)
	} else {
		b.phaseLeft = make([]int, len(b.states))
	}
	if len(s.PhaseTotal) == len(b.states) {
		b.phaseTotal = make([]int, len(s.PhaseTotal))
		copy(b.phaseTotal, s.PhaseTotal)
	} else {
		b.phaseTotal = make([]int, len(b.states))
	}
	b.introTicks = s.IntroTicks
	b.introTotal = s.IntroTotal
	b.endingTicks = s.EndingTicks
	b.endingTotal = s.EndingTotal
	b.endingFade = s.EndingFade
	b.flareTicks = s.FlareTicks
	b.flareGain = s.FlareGain
	b.lullTicks = s.LullTicks
}

func (b *BurningTrees) resetTreesLocked() {
	n := b.cfg.TreeCount
	if n <= 0 {
		n = 1
	}
	b.states = make([]byte, n)
	b.phaseLeft = make([]int, n)
	b.phaseTotal = make([]int, n)
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (b *BurningTrees) TriggerEvent(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch name {
	case "ignite":
		idx, ok := b.pickHealthyTreeLocked()
		if !ok {
			b.appendLog("ignite", "skipped (no healthy trees)")
			return true
		}
		b.igniteLocked(idx)
		b.appendLog("ignite", fmt.Sprintf("tree %d (of %d)", idx+1, len(b.states)))
	case "flare":
		b.startFlareLocked("triggered")
	case "lull":
		b.startLullLocked("triggered")
	case "intro":
		b.startIntroLocked()
		b.appendLog("intro", fmt.Sprintf("started (dur=%d, growth=%.2f)", b.introTotal, b.cfg.IntroGrowth))
	case "ending":
		b.startEndingLocked()
		b.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", b.endingFade, b.endingTotal-b.endingFade))
	default:
		return false
	}
	return true
}

func (b *BurningTrees) pickHealthyTreeLocked() (int, bool) {
	healthy := make([]int, 0, len(b.states))
	for i, s := range b.states {
		if s == BTreeStateAlive {
			healthy = append(healthy, i)
		}
	}
	if len(healthy) == 0 {
		return 0, false
	}
	return healthy[b.rng.Intn(len(healthy))], true
}

func (b *BurningTrees) igniteLocked(idx int) {
	if idx < 0 || idx >= len(b.states) {
		return
	}
	if b.states[idx] != BTreeStateAlive {
		return
	}
	dur := jitterInt(b.rng, b.cfg.IgniteDur, 0.25)
	b.states[idx] = BTreeStateIgniting
	b.phaseLeft[idx] = dur
	b.phaseTotal[idx] = dur
}

func (b *BurningTrees) startFlareLocked(verb string) {
	dur := jitterInt(b.rng, b.cfg.FlareDur, 0.3)
	b.flareTicks = dur
	b.flareGain = math.Max(1, b.cfg.FlareMult*(0.85+b.rng.Float64()*0.3))
	b.appendLog("flare", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, b.flareGain))
}

func (b *BurningTrees) startLullLocked(verb string) {
	dur := jitterInt(b.rng, b.cfg.LullDur, 0.3)
	b.lullTicks = dur
	b.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, b.cfg.LullMult))
}

func (b *BurningTrees) startIntroLocked() {
	b.resetTreesLocked()
	b.introTotal = b.cfg.IntroDur
	if b.introTotal <= 0 {
		b.introTotal = 60
	}
	b.introTicks = b.introTotal
	b.endingTicks = 0
	b.endingTotal = 0
	b.endingFade = 0
	b.flareTicks = 0
	b.flareGain = 1
	b.lullTicks = 0
}

func (b *BurningTrees) startEndingLocked() {
	b.flareTicks = 0
	b.flareGain = 1
	b.lullTicks = 0
	b.introTicks = 0
	b.introTotal = 0
	b.endingFade = b.cfg.EndingDur
	if b.endingFade <= 0 {
		b.endingFade = 80
	}
	linger := b.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	b.endingTotal = b.endingFade + linger
	if b.endingTotal < 1 {
		b.endingTotal = b.endingFade
	}
	b.endingTicks = b.endingTotal
}

func (b *BurningTrees) Step() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tick++

	if len(b.states) != b.cfg.TreeCount {
		b.resetTreesLocked()
	}

	if b.endingTicks > 0 {
		b.endingTicks--
		if b.endingTicks == 0 {
			b.startIntroLocked()
			b.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, growth=%.2f)", b.introTotal, b.cfg.IntroGrowth))
		}
	} else if b.introTicks > 0 {
		b.introTicks--
	}

	if b.flareTicks > 0 {
		b.flareTicks--
		if b.flareTicks == 0 {
			b.flareGain = 1
		}
	}
	if b.lullTicks > 0 {
		b.lullTicks--
	}

	b.advanceTreesLocked()

	if b.endingTicks > 0 || b.introTicks > 0 {
		return
	}

	if b.cfg.IgniteChance > 0 && b.rng.Float64() < b.cfg.IgniteChance {
		if idx, ok := b.pickHealthyTreeLocked(); ok {
			b.igniteLocked(idx)
			b.appendLog("ignite", fmt.Sprintf("tree %d (rolled)", idx+1))
		}
	}
	if b.cfg.FlareChance > 0 && b.rng.Float64() < b.cfg.FlareChance {
		b.startFlareLocked("started")
	}
	if b.cfg.LullChance > 0 && b.rng.Float64() < b.cfg.LullChance {
		b.startLullLocked("started")
	}
	if b.cfg.SpreadP > 0 {
		b.rollSpreadLocked()
	}
}

func (b *BurningTrees) advanceTreesLocked() {
	for i := range b.states {
		if b.states[i] == BTreeStateAlive {
			continue
		}
		if b.phaseLeft[i] > 0 {
			b.phaseLeft[i]--
		}
		if b.phaseLeft[i] > 0 {
			continue
		}
		switch b.states[i] {
		case BTreeStateIgniting:
			dur := jitterInt(b.rng, b.cfg.BurnDur, 0.2)
			b.states[i] = BTreeStateBurning
			b.phaseLeft[i] = dur
			b.phaseTotal[i] = dur
		case BTreeStateBurning:
			dur := jitterInt(b.rng, b.cfg.AshDur, 0.25)
			b.states[i] = BTreeStateAshing
			b.phaseLeft[i] = dur
			b.phaseTotal[i] = dur
		case BTreeStateAshing:
			b.states[i] = BTreeStateAsh
			b.phaseLeft[i] = 0
			b.phaseTotal[i] = 0
		}
	}
}

func (b *BurningTrees) rollSpreadLocked() {
	for i, s := range b.states {
		if s != BTreeStateBurning {
			continue
		}
		// Spread chance ramps with how far into the burn we are — peaks
		// mid-burn, mirrors how a real fire passes heat to neighbors.
		progress := 0.5
		if b.phaseTotal[i] > 0 {
			progress = phaseProgress(b.phaseTotal[i], b.phaseLeft[i])
		}
		envelope := math.Sin(progress * math.Pi)
		if envelope <= 0 {
			continue
		}
		if b.rng.Float64() >= b.cfg.SpreadP*envelope {
			continue
		}
		neighbors := []int{i - 1, i + 1}
		// shuffle to avoid bias toward left/right
		if b.rng.Float64() < 0.5 {
			neighbors[0], neighbors[1] = neighbors[1], neighbors[0]
		}
		for _, n := range neighbors {
			if n < 0 || n >= len(b.states) {
				continue
			}
			if b.states[n] != BTreeStateAlive {
				continue
			}
			b.igniteLocked(n)
			b.appendLog("ignite", fmt.Sprintf("tree %d (spread)", n+1))
			break
		}
	}
}
