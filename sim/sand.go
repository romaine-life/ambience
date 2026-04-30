package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// sandGrain is one in-flight grain falling from the pipe toward the pile.
// Continuous coords + velocity; settled grains move into the column heightmap
// rather than staying as particles.
type sandGrain struct {
	Row, Col   float64
	VRow, VCol float64
	Color      color.RGBA
}

// SandConfig tunes the Sand effect: sand pouring from a pipe into a container.
// Geometry knobs (pipe_x, bin_w, etc) are fractions/cell counts so the effect
// scales with the configured grid; defaults apply to a 160x80 grid but degrade
// gracefully on smaller dimensions.
type SandConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroTrickle float64 `json:"intro_trickle"`
	IntroPile    float64 `json:"intro_pile"`
	// GEOMETRY (also spawn slot — fixed at the start of a scene)
	PipeX float64 `json:"pipe_x"`
	PipeW int     `json:"pipe_w"`
	BinX  float64 `json:"bin_x"`
	BinW  int     `json:"bin_w"`
	BinH  int     `json:"bin_h"`
	// LEVERS — flow
	Flow    float64 `json:"flow"`
	Spread  float64 `json:"spread"`
	Gravity float64 `json:"gravity"`
	Jitter  float64 `json:"jitter"`
	// LEVERS — settling
	Talus int `json:"talus"`
	// LEVERS — color
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// EVENTS
	SurgeChance float64 `json:"surge_p"`
	CalmChance  float64 `json:"calm_p"`
	// EVENT MODIFIERS
	SurgeDur  int     `json:"surge_dur"`
	SurgeMult float64 `json:"surge_mult"`
	CalmDur   int     `json:"calm_dur"`
	CalmMult  float64 `json:"calm_mult"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
}

func (c SandConfig) withDefaults() SandConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 60
	}
	if c.IntroTrickle < 0 {
		c.IntroTrickle = 0
	}
	if c.IntroTrickle > 1 {
		c.IntroTrickle = 1
	}
	c.IntroPile = clamp01(c.IntroPile)

	if c.PipeX <= 0 {
		c.PipeX = 0.5
	}
	if c.PipeX > 1 {
		c.PipeX = 1
	}
	if c.PipeW <= 0 {
		c.PipeW = 6
	}
	if c.BinX <= 0 {
		c.BinX = 0.5
	}
	if c.BinX > 1 {
		c.BinX = 1
	}
	if c.BinW <= 0 {
		c.BinW = 90
	}
	if c.BinH <= 0 {
		c.BinH = 32
	}

	if c.Flow <= 0 {
		c.Flow = 1.4
	}
	if c.Spread < 0 {
		c.Spread = 0
	}
	if c.Gravity <= 0 {
		c.Gravity = 0.06
	}
	if c.Jitter < 0 {
		c.Jitter = 0
	}
	if c.Talus <= 0 {
		c.Talus = 2
	}

	if c.Hue == 0 {
		c.Hue = 36
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.55
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.42
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.78
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}

	if c.SurgeDur <= 0 {
		c.SurgeDur = 60
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 2.4
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 80
	}
	if c.CalmMult <= 0 {
		c.CalmMult = 0.4
	}

	if c.EndingDur <= 0 {
		c.EndingDur = 80
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	c.EndingResidue = clamp01(c.EndingResidue)
	return c
}

// NormalizeSandConfig exposes Sand's defaulting rules to callers outside the
// sim package (mirrors NormalizeConfig for Rain).
func NormalizeSandConfig(c SandConfig) SandConfig { return c.withDefaults() }

// SandSchema describes the Sand effect's tunable knobs for the dev UI.
func SandSchema() EffectSchema {
	return EffectSchema{
		Name: "sand",
		Knobs: []Knob{
			// INTRODUCTION
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent ramping the pour from a thin trickle up to full flow on entry."},
			{Key: "intro_trickle", Label: "trickle", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.15,
				Description: "Starting flow fraction at the very beginning of the intro before it ramps up."},
			{Key: "intro_pile", Label: "starting pile", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 0.95, Step: 0.02, Default: 0,
				Description: "Initial pile fill fraction. 0 = container starts empty."},
			// GEOMETRY (spawn-time fixed)
			{Key: "pipe_x", Label: "pipe x", Slot: SlotSpawn, Group: "geometry", Type: KnobFloat, Min: 0.1, Max: 0.9, Step: 0.02, Default: 0.5,
				Description: "Horizontal position of the pipe nozzle as a fraction across the canvas."},
			{Key: "pipe_w", Label: "pipe w", Slot: SlotSpawn, Group: "geometry", Type: KnobInt, Min: 2, Max: 16, Step: 1, Default: 6,
				Description: "Pipe nozzle width in cells. Wider pipes spread emission across more columns."},
			{Key: "bin_x", Label: "bin x", Slot: SlotSpawn, Group: "geometry", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.02, Default: 0.5,
				Description: "Horizontal position of the container's center as a fraction across the canvas."},
			{Key: "bin_w", Label: "bin w", Slot: SlotSpawn, Group: "geometry", Type: KnobInt, Min: 20, Max: 140, Step: 2, Default: 90,
				Description: "Container width in cells (outer; walls are the leftmost and rightmost cells)."},
			{Key: "bin_h", Label: "bin h", Slot: SlotSpawn, Group: "geometry", Type: KnobInt, Min: 8, Max: 60, Step: 1, Default: 32,
				Description: "Container interior height in cells. Taller bins hold more sand before overflow."},
			// LEVERS — flow
			{Key: "flow", Label: "flow", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.1, Max: 6, Step: 0.05, Default: 1.4,
				Description: "Average grains emitted from the pipe per tick."},
			{Key: "spread", Label: "spread", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.3,
				Description: "Lateral velocity jitter applied to each emitted grain so the stream isn't a perfect line."},
			{Key: "gravity", Label: "gravity", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.02, Max: 0.2, Step: 0.005, Default: 0.06,
				Description: "Vertical acceleration applied to each grain per tick. Higher values feel heavier."},
			{Key: "jitter", Label: "jitter", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.05,
				Description: "Per-tick lateral wobble on grains in flight. Loosens the stream into rougher fall."},
			// LEVERS — settling
			{Key: "talus", Label: "talus", Slot: SlotLever, Group: "settling", Type: KnobInt, Min: 1, Max: 6, Step: 1, Default: 2,
				Description: "Max height difference between adjacent pile columns before sand slides sideways."},
			// LEVERS — color
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 70, Step: 1, Default: 36,
				Description: "Base sand hue. Lower values warm toward rust; higher values lean pale straw."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 14,
				Description: "Per-grain hue variation so the pile reads as granular rather than uniform."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Overall color saturation of the sand."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Minimum lightness used for the dimmer grains."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for the brightest grains and the freshly fallen surface."},
			// EVENTS
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surge",
				Description: "Per-tick chance of a temporary increase in flow rate (a surge)."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a temporary reduction in flow rate."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60,
				Description: "Typical surge duration in ticks (jittered by +/-30%)."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1.1, Max: 5, Step: 0.1, Default: 2.4,
				Description: "Flow multiplier applied while a surge is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80,
				Description: "Typical calm duration in ticks (jittered by +/-30%)."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Flow multiplier applied while a calm is active."},
			// ENDING
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent tapering the pipe's flow back toward zero."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 40,
				Description: "Extra quiet ticks after the taper so airborne grains can finish settling."},
			{Key: "ending_residue", Label: "residue pile", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 1.0,
				Description: "Pile fraction kept at the end of the outro. 1 = pile stays intact, 0 = pile drains."},
		},
	}
}

// SandGrain is the JSON-serializable mirror of an in-flight grain for
// snapshot/restore.
type SandGrain struct {
	Row  float64 `json:"row"`
	Col  float64 `json:"col"`
	VRow float64 `json:"vRow"`
	VCol float64 `json:"vCol"`
	R    uint8   `json:"r"`
	G    uint8   `json:"g"`
	B    uint8   `json:"b"`
}

// SandState carries the small bookkeeping fields shared between snapshot and
// persisted-state envelopes.
type SandState struct {
	Tick        int     `json:"tick"`
	IntroTicks  int     `json:"introTicks"`
	IntroTotal  int     `json:"introTotal"`
	EndingTicks int     `json:"endingTicks"`
	EndingTotal int     `json:"endingTotal"`
	EndingFade  int     `json:"endingFade"`
	SurgeTicks  int     `json:"surgeTicks"`
	SurgeMult   float64 `json:"surgeMult"`
	CalmTicks   int     `json:"calmTicks"`
}

// SandSnapshot is the wire-shape clients consume to mirror the authority sim.
type SandSnapshot struct {
	SandState
	Pile   []int       `json:"pile"`
	Grains []SandGrain `json:"grains"`
}

type SandPersistedState struct {
	SandState
	RNGState uint64      `json:"rngState"`
	Pile     []int       `json:"pile"`
	Grains   []SandGrain `json:"grains"`
}

// Sand simulates sand pouring from a pipe into a container. Falling grains are
// tracked as particles; settled sand is a per-column heightmap with a talus
// relaxation pass each tick so piles read as granular rather than as a static
// trapezoid.
type Sand struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng  *rngutil.RNG
	cfg  SandConfig
	tick int

	grains []sandGrain
	pile   []int

	surgeTicks  int
	surgeMult   float64
	calmTicks   int
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int
	emitAcc     float64

	log []LogEntry
}

func NewSand(w, h int, seed int64, cfg SandConfig) *Sand {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	s := &Sand{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
		pile: make([]int, w),
	}
	s.seedInitialPileLocked()
	return s
}

func (s *Sand) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if w == s.W && h == s.H {
		return
	}
	s.W = w
	s.H = h
	s.Grid = make([][]Pixel, h)
	for i := range s.Grid {
		s.Grid[i] = make([]Pixel, w)
	}
	s.pile = make([]int, w)
	s.grains = nil
	s.seedInitialPileLocked()
}

func (s *Sand) SetConfig(cfg SandConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.cfg
	next := cfg.withDefaults()
	geometryChanged := next.PipeX != prev.PipeX || next.PipeW != prev.PipeW ||
		next.BinX != prev.BinX || next.BinW != prev.BinW || next.BinH != prev.BinH
	s.cfg = next
	if geometryChanged {
		// Geometry knobs are spawn-time: clearing in-flight grains and
		// rebuilding the pile is the cleanest way to re-establish a
		// readable pour after a layout change. Live levers (flow, hue,
		// gravity) keep continuity.
		s.grains = nil
		s.pile = make([]int, s.W)
		s.seedInitialPileLocked()
	}
}

func (s *Sand) EffectiveConfig() SandConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Sand) CurrentTick() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

func (s *Sand) PerturbRNG(delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng.Mix(delta)
}

func (s *Sand) DrainLog() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return nil
	}
	out := s.log
	s.log = nil
	return out
}

func (s *Sand) appendLog(kind, desc string) {
	s.log = append(s.log, LogEntry{Tick: s.tick, Type: kind, Desc: desc})
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *Sand) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "surge":
		s.startSurgeLocked()
		s.appendLog("surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", s.surgeTicks, s.surgeMult))
	case "calm":
		s.calmTicks = jitterInt(s.rng, s.cfg.CalmDur, 0.3)
		s.appendLog("calm", fmt.Sprintf("triggered (dur=%d, x%.2f)", s.calmTicks, s.cfg.CalmMult))
	case "intro":
		s.startIntroLocked()
		s.appendLog("intro", fmt.Sprintf("started (dur=%d, trickle=%.2f, pile=%.2f)", s.introTotal, s.cfg.IntroTrickle, s.cfg.IntroPile))
	case "ending":
		s.startEndingLocked()
		s.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d, residue=%.2f)", s.endingFade, s.endingTotal-s.endingFade, s.cfg.EndingResidue))
	default:
		return false
	}
	return true
}

func (s *Sand) startSurgeLocked() {
	s.surgeTicks = jitterInt(s.rng, s.cfg.SurgeDur, 0.3)
	s.surgeMult = s.cfg.SurgeMult
}

func (s *Sand) startIntroLocked() {
	s.surgeTicks = 0
	s.calmTicks = 0
	s.endingTicks = 0
	s.endingTotal = 0
	s.endingFade = 0
	s.grains = nil
	s.pile = make([]int, s.W)
	s.seedInitialPileLocked()
	s.introTotal = s.cfg.IntroDur
	if s.introTotal <= 0 {
		s.introTotal = 60
	}
	s.introTicks = s.introTotal
}

func (s *Sand) startEndingLocked() {
	s.introTicks = 0
	s.introTotal = 0
	s.surgeTicks = 0
	s.calmTicks = 0
	s.endingFade = s.cfg.EndingDur
	if s.endingFade <= 0 {
		s.endingFade = 80
	}
	linger := s.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	s.endingTotal = s.endingFade + linger
	if s.endingTotal < 1 {
		s.endingTotal = s.endingFade
	}
	s.endingTicks = s.endingTotal
}

// seedInitialPileLocked respects IntroPile by preloading the bin with a
// roughly flat layer of sand before the first tick. Used on construction and
// on intro restart so "starting pile" reads visibly.
func (s *Sand) seedInitialPileLocked() {
	if s.cfg.IntroPile <= 0 {
		return
	}
	binL, binR, binBottom, binTop := s.binBoundsLocked()
	if binR <= binL+1 {
		return
	}
	binH := binBottom - binTop + 1
	if binH < 1 {
		return
	}
	height := int(math.Round(float64(binH) * s.cfg.IntroPile))
	if height < 1 {
		return
	}
	if height > binH {
		height = binH
	}
	for c := binL + 1; c <= binR-1; c++ {
		s.pile[c] = height
	}
}

// binBoundsLocked computes the container's interior using current geometry
// knobs. binL/binR are the wall columns; sand lives in [binL+1, binR-1].
// binBottom is the floor row; binTop is the highest row sand can occupy
// inside the bin (i.e. the wall top).
func (s *Sand) binBoundsLocked() (binL, binR, binBottom, binTop int) {
	if s.W <= 0 || s.H <= 0 {
		return 0, 0, 0, 0
	}
	binW := s.cfg.BinW
	if binW < 4 {
		binW = 4
	}
	if binW > s.W {
		binW = s.W
	}
	center := int(math.Round(s.cfg.BinX * float64(s.W-1)))
	binL = center - binW/2
	binR = binL + binW - 1
	if binL < 0 {
		shift := -binL
		binL += shift
		binR += shift
	}
	if binR > s.W-1 {
		shift := binR - (s.W - 1)
		binL -= shift
		binR -= shift
	}
	if binL < 0 {
		binL = 0
	}
	if binR > s.W-1 {
		binR = s.W - 1
	}
	binBottom = s.H - 2
	if binBottom < 0 {
		binBottom = 0
	}
	binH := s.cfg.BinH
	if binH < 2 {
		binH = 2
	}
	if binH > binBottom {
		binH = binBottom
	}
	binTop = binBottom - binH + 1
	if binTop < 0 {
		binTop = 0
	}
	return binL, binR, binBottom, binTop
}

func (s *Sand) pipeBoundsLocked() (left, right, row int) {
	if s.W <= 0 {
		return 0, 0, 0
	}
	pipeW := s.cfg.PipeW
	if pipeW < 1 {
		pipeW = 1
	}
	if pipeW > s.W {
		pipeW = s.W
	}
	center := int(math.Round(s.cfg.PipeX * float64(s.W-1)))
	left = center - pipeW/2
	right = left + pipeW - 1
	if left < 0 {
		left = 0
		right = left + pipeW - 1
	}
	if right > s.W-1 {
		right = s.W - 1
		left = right - pipeW + 1
		if left < 0 {
			left = 0
		}
	}
	row = 4
	if s.H <= 8 {
		row = 1
	}
	return left, right, row
}

func (s *Sand) Step() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tick++

	if s.surgeTicks > 0 {
		s.surgeTicks--
		if s.surgeTicks == 0 {
			s.surgeMult = 0
		}
	}
	if s.calmTicks > 0 {
		s.calmTicks--
	}
	introActive := s.introTicks > 0
	endingActive := s.endingTicks > 0

	if !introActive && !endingActive {
		if s.surgeTicks == 0 && s.rng.Float64() < s.cfg.SurgeChance {
			s.startSurgeLocked()
			s.appendLog("surge", fmt.Sprintf("started (dur=%d, x%.2f)", s.surgeTicks, s.surgeMult))
		}
		if s.calmTicks == 0 && s.rng.Float64() < s.cfg.CalmChance {
			s.calmTicks = jitterInt(s.rng, s.cfg.CalmDur, 0.3)
			s.appendLog("calm", fmt.Sprintf("started (dur=%d, x%.2f)", s.calmTicks, s.cfg.CalmMult))
		}
	}

	s.emitGrainsLocked()
	s.advanceGrainsLocked()
	s.relaxPileLocked()
	s.applyEndingDrainLocked()
	s.repaintGridLocked()

	if introActive {
		s.introTicks--
		if s.introTicks < 0 {
			s.introTicks = 0
		}
	}
	if endingActive {
		s.endingTicks--
		if s.endingTicks < 0 {
			s.endingTicks = 0
		}
	}
}

func (s *Sand) currentFlowLocked() float64 {
	flow := s.cfg.Flow
	if s.surgeTicks > 0 && s.surgeMult > 0 {
		flow *= s.surgeMult
	}
	if s.calmTicks > 0 {
		flow *= s.cfg.CalmMult
	}
	if s.introTicks > 0 {
		progress := phaseProgress(s.introTotal, s.introTicks)
		flow *= s.cfg.IntroTrickle + (1-s.cfg.IntroTrickle)*progress
	}
	if s.endingTicks > 0 {
		fade := s.endingFade
		if fade <= 0 {
			fade = s.endingTotal
		}
		fadeLeft := s.endingTicks - (s.endingTotal - fade)
		if fadeLeft <= 0 {
			return 0
		}
		progress := phaseProgress(fade, fadeLeft)
		flow *= 1 - progress
	}
	if flow < 0 {
		flow = 0
	}
	return flow
}

func (s *Sand) emitGrainsLocked() {
	if s.W <= 0 || s.H <= 0 {
		return
	}
	flow := s.currentFlowLocked()
	if flow <= 0 {
		return
	}
	s.emitAcc += flow
	emit := int(math.Floor(s.emitAcc))
	s.emitAcc -= float64(emit)
	if emit <= 0 {
		return
	}
	left, right, row := s.pipeBoundsLocked()
	width := float64(right - left + 1)
	for i := 0; i < emit; i++ {
		col := float64(left) + s.rng.Float64()*width
		// jittered start row just below the pipe rim
		startRow := float64(row) + 1 + s.rng.Float64()*0.5
		vCol := (s.rng.Float64()*2 - 1) * s.cfg.Spread
		vRow := 0.05 + s.rng.Float64()*0.05
		hue := math.Mod(s.cfg.Hue+(s.rng.Float64()*2-1)*s.cfg.HueSpread+360, 360)
		light := s.cfg.LightnessMin + s.rng.Float64()*(s.cfg.LightnessMax-s.cfg.LightnessMin)
		c := hslToRGB(hue, s.cfg.Saturation, light)
		s.grains = append(s.grains, sandGrain{
			Row:   startRow,
			Col:   col,
			VRow:  vRow,
			VCol:  vCol,
			Color: c,
		})
	}
	if len(s.grains) > 4096 {
		// Hard cap to keep the sim bounded if a misconfig sets flow huge.
		s.grains = s.grains[len(s.grains)-4096:]
	}
}

func (s *Sand) advanceGrainsLocked() {
	if len(s.grains) == 0 {
		return
	}
	binL, binR, binBottom, _ := s.binBoundsLocked()
	alive := s.grains[:0]
	for _, g := range s.grains {
		// Apply gravity + jitter.
		g.VRow += s.cfg.Gravity
		if s.cfg.Jitter > 0 {
			g.VCol += (s.rng.Float64()*2 - 1) * s.cfg.Jitter
		}
		// Cap velocities so we don't tunnel through the pile.
		if g.VRow > 0.95 {
			g.VRow = 0.95
		}
		if g.VCol > 0.6 {
			g.VCol = 0.6
		} else if g.VCol < -0.6 {
			g.VCol = -0.6
		}
		nextCol := g.Col + g.VCol
		nextRow := g.Row + g.VRow

		// Wall collisions: keep grains inside the bin's interior. Bounce
		// damped so the stream can ride down a wall without piling outside.
		if nextCol <= float64(binL) {
			nextCol = float64(binL) + 0.5
			g.VCol = math.Abs(g.VCol) * 0.3
		}
		if nextCol >= float64(binR) {
			nextCol = float64(binR) - 0.5
			g.VCol = -math.Abs(g.VCol) * 0.3
		}

		col := int(math.Floor(nextCol + 0.5))
		if col < 0 {
			col = 0
		}
		if col >= s.W {
			col = s.W - 1
		}

		// If the column is outside the bin (corner cases), drop the grain.
		if col <= binL || col >= binR {
			continue
		}

		// Settle when we have reached the top of the pile in this column or
		// the floor below the bin.
		settleRow := binBottom - s.pile[col]
		if int(math.Floor(nextRow)) >= settleRow {
			s.depositLocked(col)
			continue
		}
		if int(math.Floor(nextRow)) >= s.H-1 {
			// Off-bottom-of-canvas safeguard.
			s.depositLocked(col)
			continue
		}

		g.Row = nextRow
		g.Col = nextCol
		alive = append(alive, g)
	}
	s.grains = alive
}

// depositLocked adds a single grain to the pile at the given column. If the
// column is already at the cap (above the bin top), it nudges the grain
// sideways to a neighbor with room — that's how surge events overflow the
// container by spreading a wide mound rather than building straight up.
func (s *Sand) depositLocked(col int) {
	binL, binR, _, binTop := s.binBoundsLocked()
	if col <= binL || col >= binR {
		return
	}
	maxPile := s.H - 1 - binTop
	if maxPile < 1 {
		maxPile = 1
	}
	target := col
	if s.pile[target] >= maxPile {
		// Try to slide outward to a neighbor with room.
		for offset := 1; offset < binR-binL; offset++ {
			leftCand := col - offset
			rightCand := col + offset
			if leftCand > binL && s.pile[leftCand] < maxPile {
				target = leftCand
				break
			}
			if rightCand < binR && s.pile[rightCand] < maxPile {
				target = rightCand
				break
			}
		}
		if s.pile[target] >= maxPile {
			return
		}
	}
	s.pile[target]++
}

// relaxPileLocked runs a small number of talus passes over the pile so that
// any column whose height exceeds its neighbor by more than the configured
// talus angle moves one unit sideways. Multiple passes per tick let a fresh
// mound spread quickly into a readable angle of repose.
func (s *Sand) relaxPileLocked() {
	binL, binR, _, _ := s.binBoundsLocked()
	if binR <= binL+2 {
		return
	}
	talus := s.cfg.Talus
	if talus < 1 {
		talus = 1
	}
	const passes = 4
	for p := 0; p < passes; p++ {
		moved := false
		// Sweep direction alternates each pass to avoid bias.
		var step int
		var startCol, endCol int
		if p%2 == 0 {
			step = 1
			startCol = binL + 1
			endCol = binR - 1
		} else {
			step = -1
			startCol = binR - 1
			endCol = binL + 1
		}
		for c := startCol; (step == 1 && c <= endCol) || (step == -1 && c >= endCol); c += step {
			h := s.pile[c]
			if h <= 0 {
				continue
			}
			leftC := c - 1
			rightC := c + 1
			leftDiff := 0
			rightDiff := 0
			if leftC > binL {
				leftDiff = h - s.pile[leftC]
			}
			if rightC < binR {
				rightDiff = h - s.pile[rightC]
			}
			if leftDiff > talus || rightDiff > talus {
				// Slide toward the steeper side.
				if leftDiff >= rightDiff && leftC > binL {
					s.pile[c]--
					s.pile[leftC]++
					moved = true
				} else if rightC < binR {
					s.pile[c]--
					s.pile[rightC]++
					moved = true
				}
			}
		}
		if !moved {
			break
		}
	}
}

// applyEndingDrainLocked pulls the pile toward EndingResidue while the outro
// is active so a residue<1 setting visibly drains the bin.
func (s *Sand) applyEndingDrainLocked() {
	if s.endingTicks <= 0 {
		return
	}
	if s.cfg.EndingResidue >= 1 {
		return
	}
	binL, binR, binBottom, binTop := s.binBoundsLocked()
	if binR <= binL+1 {
		return
	}
	binH := binBottom - binTop + 1
	if binH < 1 {
		return
	}
	progress := phaseProgress(s.endingTotal, s.endingTicks)
	scale := 1 - (1-s.cfg.EndingResidue)*progress
	if scale < 0 {
		scale = 0
	}
	for c := binL + 1; c <= binR-1; c++ {
		target := int(math.Round(float64(s.pile[c]) * scale))
		if target < 0 {
			target = 0
		}
		if target < s.pile[c] {
			s.pile[c]--
		}
	}
}

func (s *Sand) repaintGridLocked() {
	for y := range s.Grid {
		for x := range s.Grid[y] {
			s.Grid[y][x] = Pixel{}
		}
	}
	s.paintBinLocked()
	s.paintPipeLocked()
	s.paintPileLocked()
	s.paintGrainsLocked()
}

func (s *Sand) paintBinLocked() {
	binL, binR, binBottom, binTop := s.binBoundsLocked()
	if binR <= binL || binBottom <= binTop {
		return
	}
	wall := color.RGBA{R: 64, G: 56, B: 48, A: 255}
	rim := color.RGBA{R: 96, G: 80, B: 64, A: 255}
	// Walls.
	for r := binTop; r <= binBottom; r++ {
		s.paintCell(r, binL, wall)
		s.paintCell(r, binR, wall)
	}
	// Floor.
	for c := binL; c <= binR; c++ {
		s.paintCell(binBottom+1, c, wall)
	}
	// Rim highlight at the top of each wall.
	s.paintCell(binTop, binL, rim)
	s.paintCell(binTop, binR, rim)
}

func (s *Sand) paintPipeLocked() {
	left, right, row := s.pipeBoundsLocked()
	if right < left {
		return
	}
	body := color.RGBA{R: 96, G: 88, B: 80, A: 255}
	rim := color.RGBA{R: 128, G: 116, B: 104, A: 255}
	dark := color.RGBA{R: 56, G: 50, B: 44, A: 255}
	// Pipe body — three rows tall, with rim and shadow lines for depth.
	if row >= 2 {
		for c := left; c <= right; c++ {
			s.paintCell(row-2, c, rim)
		}
	}
	for c := left; c <= right; c++ {
		s.paintCell(row-1, c, body)
		s.paintCell(row, c, dark)
	}
}

func (s *Sand) paintPileLocked() {
	binL, binR, binBottom, _ := s.binBoundsLocked()
	for c := binL + 1; c <= binR-1; c++ {
		h := s.pile[c]
		if h <= 0 {
			continue
		}
		for k := 0; k < h; k++ {
			row := binBottom - k
			if row < 0 || row >= s.H {
				break
			}
			// Deterministic per-cell hue/light jitter so the pile reads as
			// granular without needing per-cell color storage.
			n := pileNoise(c, row)
			hue := math.Mod(s.cfg.Hue+(n[0]*2-1)*s.cfg.HueSpread+360, 360)
			light := s.cfg.LightnessMin + n[1]*(s.cfg.LightnessMax-s.cfg.LightnessMin)
			// Top row of pile is brighter to read as freshly fallen.
			if k == h-1 {
				light = math.Min(s.cfg.LightnessMax, light+0.08)
			}
			s.paintCell(row, c, hslToRGB(hue, s.cfg.Saturation, light))
		}
	}
}

func (s *Sand) paintGrainsLocked() {
	for _, g := range s.grains {
		row := int(math.Floor(g.Row + 0.5))
		col := int(math.Floor(g.Col + 0.5))
		s.paintCell(row, col, g.Color)
	}
}

func (s *Sand) paintCell(row, col int, c color.RGBA) {
	if row < 0 || row >= s.H || col < 0 || col >= s.W {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	s.Grid[row][col] = Pixel{Filled: true, C: c}
}

// pileNoise returns two stable [0,1) values per cell so the pile gets visual
// texture without any RNG state. Cheap integer hash; values are good enough
// for "salt" jitter, not statistical work.
func pileNoise(col, row int) [2]float64 {
	h := uint32(col*73856093) ^ uint32(row*19349663)
	h ^= h >> 13
	h *= 0x85ebca6b
	h ^= h >> 16
	a := float64(h&0xffff) / 65536.0
	b := float64((h>>16)&0xffff) / 65536.0
	return [2]float64{a, b}
}

// SnapshotState returns the lightweight scalar bookkeeping (no grains/pile).
func (s *Sand) SnapshotState() SandState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotStateLocked()
}

func (s *Sand) Snapshot() SandSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SandSnapshot{
		SandState: s.snapshotStateLocked(),
		Pile:      s.copyPileLocked(),
		Grains:    s.copyGrainsLocked(),
	}
}

func (s *Sand) RestoreState(state SandState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(state)
}

func (s *Sand) RestoreSnapshot(snap SandSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.SandState)
	s.restorePileLocked(snap.Pile)
	s.restoreGrainsLocked(snap.Grains)
}

func (s *Sand) SnapshotPersistedState() SandPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SandPersistedState{
		SandState: s.snapshotStateLocked(),
		RNGState:  s.rng.State(),
		Pile:      s.copyPileLocked(),
		Grains:    s.copyGrainsLocked(),
	}
}

func (s *Sand) RestorePersistedState(state SandPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(state.SandState)
	if state.RNGState != 0 {
		s.rng.SetState(state.RNGState)
	}
	s.restorePileLocked(state.Pile)
	s.restoreGrainsLocked(state.Grains)
}

func (s *Sand) snapshotStateLocked() SandState {
	return SandState{
		Tick:        s.tick,
		IntroTicks:  s.introTicks,
		IntroTotal:  s.introTotal,
		EndingTicks: s.endingTicks,
		EndingTotal: s.endingTotal,
		EndingFade:  s.endingFade,
		SurgeTicks:  s.surgeTicks,
		SurgeMult:   s.surgeMult,
		CalmTicks:   s.calmTicks,
	}
}

func (s *Sand) restoreStateLocked(state SandState) {
	s.tick = state.Tick
	s.introTicks = state.IntroTicks
	s.introTotal = state.IntroTotal
	s.endingTicks = state.EndingTicks
	s.endingTotal = state.EndingTotal
	s.endingFade = state.EndingFade
	s.surgeTicks = state.SurgeTicks
	s.surgeMult = state.SurgeMult
	s.calmTicks = state.CalmTicks
}

func (s *Sand) copyPileLocked() []int {
	out := make([]int, len(s.pile))
	copy(out, s.pile)
	return out
}

func (s *Sand) restorePileLocked(pile []int) {
	if len(pile) == 0 {
		return
	}
	if len(pile) == s.W {
		copy(s.pile, pile)
		return
	}
	// If the snapshot was captured at a different width, copy what fits.
	n := len(pile)
	if n > s.W {
		n = s.W
	}
	for i := 0; i < n; i++ {
		s.pile[i] = pile[i]
	}
}

func (s *Sand) copyGrainsLocked() []SandGrain {
	out := make([]SandGrain, len(s.grains))
	for i, g := range s.grains {
		out[i] = SandGrain{
			Row:  g.Row,
			Col:  g.Col,
			VRow: g.VRow,
			VCol: g.VCol,
			R:    g.Color.R,
			G:    g.Color.G,
			B:    g.Color.B,
		}
	}
	return out
}

func (s *Sand) restoreGrainsLocked(grains []SandGrain) {
	s.grains = make([]sandGrain, len(grains))
	for i, g := range grains {
		s.grains[i] = sandGrain{
			Row:   g.Row,
			Col:   g.Col,
			VRow:  g.VRow,
			VCol:  g.VCol,
			Color: color.RGBA{R: g.R, G: g.G, B: g.B, A: 255},
		}
	}
}
