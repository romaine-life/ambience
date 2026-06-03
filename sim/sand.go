package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type sandGrain struct {
	Row, Col   float64
	VRow, VCol float64
	Life       int
	MaxLife    int
	Color      color.RGBA
	Bright     float64
}

// SandConfig tunes the pipe-into-container sand prototype used in isolated dev
// sessions. The granular feel is intentional: emitted grains fall as discrete
// particles, then accumulate as a per-column pile that slides toward its angle
// of repose.
type SandConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroTrickle float64 `json:"intro_trickle"`
	IntroPile    float64 `json:"intro_pile"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
	// PIPE / CONTAINER GEOMETRY
	PipeX          float64 `json:"pipe_x"`
	PipeY          float64 `json:"pipe_y"`
	PipeWidth      float64 `json:"pipe_width"`
	StreamSpread   float64 `json:"stream_spread"`
	ContainerY     float64 `json:"container_y"`
	ContainerSpan  float64 `json:"container_span"`
	ContainerDepth float64 `json:"container_depth"`
	WallThick      float64 `json:"wall_thick"`
	// FLOW
	EmitRate      float64 `json:"emit_rate"`
	Gravity       float64 `json:"gravity"`
	Drag          float64 `json:"drag"`
	Spread        float64 `json:"spread"`
	Splatter      float64 `json:"splatter_p"`
	MaxGrains     int     `json:"grain_max"`
	Repose        float64 `json:"repose"`
	SettlePerTick int     `json:"settle"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	PipeHue      float64 `json:"pipe_hue"`
	PipeLight    float64 `json:"pipe_light"`
	// EVENT CHANCES
	SurgeChance float64 `json:"surge_p"`
	CalmChance  float64 `json:"calm_p"`
	// EVENT MODIFIERS
	SurgeDur  int     `json:"surge_dur"`
	SurgeMult float64 `json:"surge_mult"`
	CalmDur   int     `json:"calm_dur"`
	CalmMult  float64 `json:"calm_mult"`
}

func (c SandConfig) withDefaults() SandConfig {
	if c.IntroDur == 0 && c.IntroTrickle == 0 && c.IntroPile == 0 {
		c.IntroDur = 70
		c.IntroTrickle = 0.18
		c.IntroPile = 0.05
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 70
		}
		if c.IntroTrickle <= 0 {
			c.IntroTrickle = 0.18
		}
		if c.IntroPile < 0 {
			c.IntroPile = 0
		}
	}
	c.IntroTrickle = clamp01(c.IntroTrickle)
	c.IntroPile = clamp01(c.IntroPile)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingResidue == 0 {
		c.EndingDur = 70
		c.EndingLinger = 40
		c.EndingResidue = 0.4
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 70
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingResidue < 0 {
			c.EndingResidue = 0
		}
	}
	c.EndingResidue = clamp01(c.EndingResidue)
	if c.PipeX <= 0 {
		c.PipeX = 0.5
	}
	c.PipeX = clamp01(c.PipeX)
	if c.PipeY <= 0 {
		c.PipeY = 0.16
	}
	c.PipeY = clamp01(c.PipeY)
	if c.PipeWidth <= 0 {
		c.PipeWidth = 6
	}
	if c.StreamSpread <= 0 {
		c.StreamSpread = 1.4
	}
	if c.ContainerY <= 0 {
		c.ContainerY = 0.62
	}
	c.ContainerY = clamp01(c.ContainerY)
	if c.ContainerSpan <= 0 {
		c.ContainerSpan = 0.42
	}
	c.ContainerSpan = clamp01(c.ContainerSpan)
	if c.ContainerDepth <= 0 {
		c.ContainerDepth = 16
	}
	if c.WallThick <= 0 {
		c.WallThick = 1
	}
	if c.EmitRate <= 0 {
		c.EmitRate = 1.6
	}
	if c.Gravity <= 0 {
		c.Gravity = 0.085
	}
	if c.Drag < 0 {
		c.Drag = 0
	}
	if c.Spread < 0 {
		c.Spread = 0
	}
	if c.Splatter < 0 {
		c.Splatter = 0
	}
	if c.MaxGrains <= 0 {
		c.MaxGrains = 96
	}
	if c.Repose <= 0 {
		c.Repose = 1.6
	}
	if c.SettlePerTick <= 0 {
		c.SettlePerTick = 6
	}
	if c.Hue == 0 {
		c.Hue = 38
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.6
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.36
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.78
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.PipeHue == 0 {
		c.PipeHue = 22
	}
	if c.PipeLight <= 0 {
		c.PipeLight = 0.32
	}
	c.PipeLight = clamp01(c.PipeLight)
	if c.SurgeDur <= 0 {
		c.SurgeDur = 60
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 1.9
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 70
	}
	if c.CalmMult <= 0 {
		c.CalmMult = 0.35
	}
	return c
}

// SandSchema describes the Sand effect's tunable knobs for the dev UI.
func SandSchema() EffectSchema {
	return EffectSchema{
		Name: "sand",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent ramping from a thin trickle into the full pour."},
			{Key: "intro_trickle", Label: "intro trickle", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Initial emission fraction during the intro before the stream reaches full rate."},
			{Key: "intro_pile", Label: "intro pile", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.05,
				Description: "How full the container starts when the intro fires (0 = empty, 1 = at the brim)."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering the pour back down to a final trickle."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 40,
				Description: "Extra quiet ticks for final grains and slope sliding to settle after the source cuts off."},
			{Key: "ending_residue", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.4,
				Description: "Fraction of pile that remains as the source dries up — 0 fully drains, 1 keeps the pile."},
			{Key: "pipe_x", Label: "pipe x", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.5,
				Description: "Horizontal position of the pipe spout as a fraction of the canvas."},
			{Key: "pipe_y", Label: "pipe y", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Vertical position of the pipe lip as a fraction from the top of the canvas."},
			{Key: "pipe_width", Label: "pipe width", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 3, Max: 16, Step: 0.5, Default: 6,
				Description: "Outside diameter of the pipe spout in cells."},
			{Key: "stream_spread", Label: "stream spread", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 0.2, Max: 4, Step: 0.1, Default: 1.4,
				Description: "Lateral spread of grains as they leave the spout. Larger values widen the falling column."},
			{Key: "container_y", Label: "container y", Slot: SlotLever, Group: "container", Type: KnobFloat, Min: 0.4, Max: 0.9, Step: 0.01, Default: 0.62,
				Description: "Vertical position of the container's brim as a fraction from the top of the canvas."},
			{Key: "container_span", Label: "container span", Slot: SlotLever, Group: "container", Type: KnobFloat, Min: 0.15, Max: 0.8, Step: 0.01, Default: 0.42,
				Description: "Horizontal span of the container opening relative to canvas width."},
			{Key: "container_depth", Label: "depth", Slot: SlotLever, Group: "container", Type: KnobFloat, Min: 4, Max: 36, Step: 1, Default: 16,
				Description: "Depth of the container in cells."},
			{Key: "wall_thick", Label: "wall thick", Slot: SlotLever, Group: "container", Type: KnobFloat, Min: 1, Max: 4, Step: 0.5, Default: 1,
				Description: "Thickness of the container walls in cells."},
			{Key: "emit_rate", Label: "emit rate", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.1, Max: 5, Step: 0.05, Default: 1.6,
				Description: "Average number of grains emitted from the pipe per tick at baseline flow."},
			{Key: "gravity", Label: "gravity", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.02, Max: 0.25, Step: 0.005, Default: 0.085,
				Description: "Acceleration applied to falling grains each tick. Higher values make grains fall faster."},
			{Key: "drag", Label: "drag", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.04,
				Description: "Air resistance damping on each grain's velocity. Smooths heavy spread."},
			{Key: "spread", Label: "side jitter", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.01, Default: 0.06,
				Description: "Per-tick lateral wobble applied to falling grains."},
			{Key: "splatter_p", Label: "splatter", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.18,
				Description: "Per-tick chance of a stray grain bouncing off the impact zone."},
			{Key: "grain_max", Label: "max grains", Slot: SlotLever, Group: "flow", Type: KnobInt, Min: 8, Max: 320, Step: 4, Default: 96,
				Description: "Maximum live grains tumbling in the air at once."},
			{Key: "repose", Label: "repose", Slot: SlotLever, Group: "settle", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Maximum height step the pile tolerates between adjacent columns. Lower = flatter pile."},
			{Key: "settle", Label: "settle/tick", Slot: SlotLever, Group: "settle", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 6,
				Description: "How many slope-sliding passes to apply per tick. Higher = pile flattens faster."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 14, Max: 56, Step: 1, Default: 38,
				Description: "Base hue of the sand, kept inside a warm-earth band so randomized sessions still read as sand."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Variation in grain hue across the stream and pile."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.6,
				Description: "Overall color saturation of the sand."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.36,
				Description: "Minimum lightness used for the deeper pile body."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.98, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for the brightest grains and surface ridge."},
			{Key: "pipe_hue", Label: "pipe hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 22,
				Description: "Base hue used for the pipe and container walls."},
			{Key: "pipe_light", Label: "pipe light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.32,
				Description: "Base lightness of the pipe and container walls."},
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surge",
				Description: "Per-tick chance of a temporary increase in emission rate."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a temporary reduction in flow rate."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60,
				Description: "Typical surge duration in ticks (jittered ±30%)."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.9,
				Description: "Emission multiplier applied while a surge is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70,
				Description: "Typical calm duration in ticks (jittered ±30%)."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Emission multiplier applied while a calm is active (0 = pipe shut)."},
		},
	}
}

type SandState struct {
	Tick        int       `json:"tick"`
	SurgeTicks  int       `json:"surgeTicks"`
	CalmTicks   int       `json:"calmTicks"`
	IntroTicks  int       `json:"introTicks"`
	IntroTotal  int       `json:"introTotal"`
	EndingTicks int       `json:"endingTicks"`
	EndingTotal int       `json:"endingTotal"`
	EndingFade  int       `json:"endingFade"`
	Pile        []float64 `json:"pile"`
	PileLeft    int       `json:"pileLeft"`
}

type SandGrain struct {
	Row     float64 `json:"row"`
	Col     float64 `json:"col"`
	VRow    float64 `json:"vRow"`
	VCol    float64 `json:"vCol"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
	Color   RGB     `json:"color"`
	Bright  float64 `json:"bright"`
}

type SandSnapshot struct {
	SandState
	RNGState uint64      `json:"rngState,omitempty"`
	Grains   []SandGrain `json:"grains"`
}

type SandPersistedState struct {
	SandState
	RNGState uint64      `json:"rngState"`
	Grains   []SandGrain `json:"grains"`
}

// Sand is a stylized "pipe pouring granular material into a container"
// prototype used in isolated dev sessions. Falling grains are individual
// particles; once they impact the pile or container floor, they are
// accumulated into a per-column heightmap that slides toward repose.
type Sand struct {
	mu sync.Mutex

	W, H   int
	Grid   [][]Pixel
	grains []sandGrain
	rng    *rngutil.RNG
	cfg    SandConfig
	tick   int

	surgeTicks  int
	calmTicks   int
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int

	// pile is the per-column heightmap of accumulated sand inside the
	// container, indexed by [col - pileLeft]. Stored in the same coordinate
	// frame as the grid so paint/spawn math reads naturally.
	pile     []float64
	pileLeft int

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
	}
	s.resetPileLocked()
	return s
}

func (s *Sand) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if width == s.W && height == s.H {
		return
	}
	s.W = width
	s.H = height
	s.Grid = make([][]Pixel, height)
	for i := range s.Grid {
		s.Grid[i] = make([]Pixel, width)
	}
	s.resetPileLocked()
}

func (s *Sand) SetConfig(cfg SandConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.cfg
	s.cfg = cfg.withDefaults()
	if prev.ContainerY != s.cfg.ContainerY ||
		prev.ContainerSpan != s.cfg.ContainerSpan ||
		prev.ContainerDepth != s.cfg.ContainerDepth ||
		prev.PipeX != s.cfg.PipeX ||
		prev.PipeWidth != s.cfg.PipeWidth {
		s.resetPileLocked()
	}
}

func (s *Sand) EffectiveConfig() SandConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

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
		RNGState:  s.rng.State(),
		Grains:    s.copyGrainsLocked(),
	}
}

func (s *Sand) RestoreState(st SandState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(st)
}

func (s *Sand) RestoreSnapshot(snap SandSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.SandState)
	if snap.RNGState != 0 {
		s.rng.SetState(snap.RNGState)
	}
	s.restoreGrainsLocked(snap.Grains)
	s.paintFrameLocked()
}

func (s *Sand) SnapshotPersistedState() SandPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SandPersistedState{
		SandState: s.snapshotStateLocked(),
		RNGState:  s.rng.State(),
		Grains:    s.copyGrainsLocked(),
	}
}

func (s *Sand) RestorePersistedState(st SandPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(st.SandState)
	if st.RNGState != 0 {
		s.rng.SetState(st.RNGState)
	}
	s.restoreGrainsLocked(st.Grains)
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

func (s *Sand) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "surge":
		s.surgeTicks = jitterInt(s.rng, s.cfg.SurgeDur, 0.3)
		s.appendLog("surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", s.surgeTicks, s.cfg.SurgeMult))
	case "calm":
		s.calmTicks = jitterInt(s.rng, s.cfg.CalmDur, 0.3)
		s.appendLog("calm", fmt.Sprintf("triggered (dur=%d, x%.2f)", s.calmTicks, s.cfg.CalmMult))
	case "intro":
		s.startIntroductionLocked()
		s.appendLog("intro", fmt.Sprintf("started (dur=%d, trickle=%.2f)", s.introTotal, s.cfg.IntroTrickle))
	case "ending":
		s.startEndingLocked()
		s.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", s.endingFade, s.endingTotal-s.endingFade))
	default:
		return false
	}
	return true
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

func (s *Sand) Step() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tick++
	if s.surgeTicks > 0 {
		s.surgeTicks--
	}
	if s.calmTicks > 0 {
		s.calmTicks--
	}
	if s.introTicks > 0 {
		s.introTicks--
	}
	if s.endingTicks > 0 {
		s.endingTicks--
	}

	if s.surgeTicks == 0 && s.rng.Float64() < s.cfg.SurgeChance {
		s.surgeTicks = jitterInt(s.rng, s.cfg.SurgeDur, 0.3)
		s.appendLog("surge", fmt.Sprintf("started (dur=%d, x%.2f)", s.surgeTicks, s.cfg.SurgeMult))
	}
	if s.calmTicks == 0 && s.rng.Float64() < s.cfg.CalmChance {
		s.calmTicks = jitterInt(s.rng, s.cfg.CalmDur, 0.3)
		s.appendLog("calm", fmt.Sprintf("started (dur=%d, x%.2f)", s.calmTicks, s.cfg.CalmMult))
	}

	s.spawnGrainsLocked()
	s.stepGrainsLocked()
	s.settlePileLocked()
	s.applyEndingDrainLocked()

	s.paintFrameLocked()
}

func (s *Sand) paintFrameLocked() {
	s.clearGridLocked()
	s.paintContainerLocked()
	s.paintPileLocked()
	s.paintPipeLocked()
	s.paintGrainsLocked()
}

func (s *Sand) snapshotStateLocked() SandState {
	pile := make([]float64, len(s.pile))
	copy(pile, s.pile)
	return SandState{
		Tick:        s.tick,
		SurgeTicks:  s.surgeTicks,
		CalmTicks:   s.calmTicks,
		IntroTicks:  s.introTicks,
		IntroTotal:  s.introTotal,
		EndingTicks: s.endingTicks,
		EndingTotal: s.endingTotal,
		EndingFade:  s.endingFade,
		Pile:        pile,
		PileLeft:    s.pileLeft,
	}
}

func (s *Sand) restoreStateLocked(st SandState) {
	s.tick = st.Tick
	s.surgeTicks = st.SurgeTicks
	s.calmTicks = st.CalmTicks
	s.introTicks = st.IntroTicks
	s.introTotal = st.IntroTotal
	s.endingTicks = st.EndingTicks
	s.endingTotal = st.EndingTotal
	s.endingFade = st.EndingFade
	if len(st.Pile) > 0 {
		s.pile = append([]float64(nil), st.Pile...)
		s.pileLeft = st.PileLeft
	} else {
		s.resetPileLocked()
	}
}

func (s *Sand) copyGrainsLocked() []SandGrain {
	out := make([]SandGrain, len(s.grains))
	for i, g := range s.grains {
		out[i] = SandGrain{
			Row:     g.Row,
			Col:     g.Col,
			VRow:    g.VRow,
			VCol:    g.VCol,
			Life:    g.Life,
			MaxLife: g.MaxLife,
			Color:   RGB{R: g.Color.R, G: g.Color.G, B: g.Color.B},
			Bright:  g.Bright,
		}
	}
	return out
}

func (s *Sand) restoreGrainsLocked(list []SandGrain) {
	s.grains = make([]sandGrain, len(list))
	for i, g := range list {
		s.grains[i] = sandGrain{
			Row:     g.Row,
			Col:     g.Col,
			VRow:    g.VRow,
			VCol:    g.VCol,
			Life:    g.Life,
			MaxLife: g.MaxLife,
			Color:   color.RGBA{R: g.Color.R, G: g.Color.G, B: g.Color.B, A: 255},
			Bright:  g.Bright,
		}
	}
}

func (s *Sand) startIntroductionLocked() {
	s.endingTicks = 0
	s.endingTotal = 0
	s.endingFade = 0
	s.introTotal = s.cfg.IntroDur
	if s.introTotal <= 0 {
		s.introTotal = 70
	}
	s.introTicks = s.introTotal
	s.grains = nil
	s.resetPileLocked()
	if s.cfg.IntroPile > 0 {
		s.seedPileLocked(s.cfg.IntroPile)
	}
}

func (s *Sand) startEndingLocked() {
	s.introTicks = 0
	s.introTotal = 0
	s.endingFade = s.cfg.EndingDur
	if s.endingFade <= 0 {
		s.endingFade = 70
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

// flowLevelLocked returns the current effective emission multiplier (event +
// lifecycle modifiers applied). 1.0 = baseline.
func (s *Sand) flowLevelLocked() float64 {
	flow := 1.0
	if s.surgeTicks > 0 {
		flow *= s.cfg.SurgeMult
	}
	if s.calmTicks > 0 {
		flow *= s.cfg.CalmMult
	}
	if s.introTicks > 0 {
		progress := phaseProgress(s.introTotal, s.introTicks)
		flow *= s.cfg.IntroTrickle + (1-s.cfg.IntroTrickle)*progress
	}
	if s.endingTicks > 0 {
		elapsed := s.endingTotal - s.endingTicks
		if elapsed < s.endingFade {
			fade := clamp01(float64(elapsed) / float64(max(1, s.endingFade-1)))
			flow *= 1 - 0.94*fade
		} else {
			flow *= 0.0
		}
	}
	if flow < 0 {
		flow = 0
	}
	return flow
}

func (s *Sand) pipeGeometryLocked() (lipRow, leftCol, rightCol, centerCol int) {
	if s.W <= 0 || s.H <= 0 {
		return 0, 0, 0, 0
	}
	width := s.cfg.PipeWidth
	if width < 3 {
		width = 3
	}
	half := int(math.Round(width * 0.5))
	if half < 2 {
		half = 2
	}
	center := int(math.Round(s.cfg.PipeX * float64(s.W-1)))
	left := center - half
	right := center + half
	if left < 1 {
		left = 1
	}
	if right >= s.W-1 {
		right = s.W - 2
	}
	if right < left {
		right = left
	}
	lip := int(math.Round(s.cfg.PipeY * float64(s.H-1)))
	if lip < 2 {
		lip = 2
	}
	if lip > s.H-8 {
		lip = s.H - 8
	}
	return lip, left, right, center
}

func (s *Sand) containerGeometryLocked() (brim, bottom, leftCol, rightCol int) {
	if s.W <= 0 || s.H <= 0 {
		return 0, 0, 0, 0
	}
	br := int(math.Round(s.cfg.ContainerY * float64(s.H-1)))
	if br < 8 {
		br = 8
	}
	if br > s.H-4 {
		br = s.H - 4
	}
	depth := int(math.Round(s.cfg.ContainerDepth))
	if depth < 3 {
		depth = 3
	}
	if depth > s.H-br-1 {
		depth = s.H - br - 1
	}
	if depth < 2 {
		depth = 2
	}
	bot := br + depth
	half := int(math.Round(s.cfg.ContainerSpan * float64(s.W) * 0.5))
	if half < 4 {
		half = 4
	}
	center := s.W / 2
	_, _, _, pipeCenter := s.pipeGeometryLocked()
	if pipeCenter > 0 {
		center = pipeCenter
	}
	left := center - half
	right := center + half
	if left < 1 {
		left = 1
	}
	if right >= s.W-1 {
		right = s.W - 2
	}
	return br, bot, left, right
}

func (s *Sand) wallThickLocked() int {
	w := int(math.Round(s.cfg.WallThick))
	if w < 1 {
		w = 1
	}
	if w > 4 {
		w = 4
	}
	return w
}

func (s *Sand) resetPileLocked() {
	_, _, left, right := s.containerGeometryLocked()
	cols := right - left + 1
	if cols < 1 {
		cols = 1
	}
	s.pile = make([]float64, cols)
	s.pileLeft = left
}

func (s *Sand) seedPileLocked(fillFraction float64) {
	_, bottom, left, right := s.containerGeometryLocked()
	brim, _, _, _ := s.containerGeometryLocked()
	depth := float64(bottom - brim)
	if depth <= 0 {
		return
	}
	level := clamp01(fillFraction) * depth
	cols := right - left + 1
	if cols <= 0 {
		return
	}
	if len(s.pile) != cols || s.pileLeft != left {
		s.pile = make([]float64, cols)
		s.pileLeft = left
	}
	for i := range s.pile {
		// Slight initial mound under the pipe for a natural look.
		dist := math.Abs(float64(i) - float64(cols-1)*0.5)
		falloff := math.Max(0, 1-dist/(float64(cols)*0.5+0.001))
		s.pile[i] = level * (0.7 + 0.3*falloff)
	}
}

func (s *Sand) clearGridLocked() {
	for y := range s.Grid {
		for x := range s.Grid[y] {
			s.Grid[y][x] = Pixel{}
		}
	}
}

func (s *Sand) spawnGrainsLocked() {
	flow := s.flowLevelLocked()
	if flow <= 0.001 {
		return
	}
	if len(s.grains) >= s.cfg.MaxGrains {
		return
	}
	rate := s.cfg.EmitRate * flow
	if rate <= 0 {
		return
	}
	whole := int(math.Floor(rate))
	frac := rate - float64(whole)
	count := whole
	if s.rng.Float64() < frac {
		count++
	}
	if count <= 0 {
		return
	}
	lipRow, _, _, pipeCenter := s.pipeGeometryLocked()
	for i := 0; i < count && len(s.grains) < s.cfg.MaxGrains; i++ {
		s.spawnOneGrainLocked(lipRow, pipeCenter)
	}
}

func (s *Sand) spawnOneGrainLocked(lipRow, pipeCenter int) {
	col := float64(pipeCenter) + (s.rng.Float64()*2-1)*math.Max(0.4, s.cfg.StreamSpread*0.5)
	row := float64(lipRow + 1)
	vCol := (s.rng.Float64()*2 - 1) * 0.18 * s.cfg.StreamSpread
	vRow := 0.35 + s.rng.Float64()*0.25
	hue := math.Mod(s.cfg.Hue+(s.rng.Float64()*2-1)*s.cfg.HueSpread+360, 360)
	light := s.cfg.LightnessMin + s.rng.Float64()*(s.cfg.LightnessMax-s.cfg.LightnessMin)
	c := hslToRGB(hue, s.cfg.Saturation, light)
	bright := 0.75 + s.rng.Float64()*0.25
	maxLife := jitterInt(s.rng, max(40, s.H*2), 0.25)
	s.grains = append(s.grains, sandGrain{
		Row:     row,
		Col:     col,
		VRow:    vRow,
		VCol:    vCol,
		Life:    maxLife,
		MaxLife: maxLife,
		Color:   c,
		Bright:  bright,
	})
}

func (s *Sand) stepGrainsLocked() {
	if len(s.grains) == 0 {
		return
	}
	alive := s.grains[:0]
	gravity := s.cfg.Gravity
	drag := s.cfg.Drag
	jitter := s.cfg.Spread * 0.18
	_, bottom, left, right := s.containerGeometryLocked()
	wallTop := bottom // floor inside the container is at row bottom
	for _, g := range s.grains {
		g.VRow += gravity
		if drag > 0 {
			g.VCol *= 1 - drag*0.4
		}
		if jitter > 0 {
			g.VCol += (s.rng.Float64()*2 - 1) * jitter
		}
		g.Row += g.VRow
		g.Col += g.VCol
		g.Life--

		gridCol := int(math.Round(g.Col))
		// Pile collision: if the grain enters the column above the pile
		// surface, deposit it.
		if gridCol >= left && gridCol <= right {
			idx := gridCol - s.pileLeft
			if idx >= 0 && idx < len(s.pile) {
				surfaceRow := float64(wallTop) - s.pile[idx]
				if g.Row >= surfaceRow {
					s.depositGrainLocked(g, idx)
					continue
				}
			}
			// Side wall bounce inside the container — keep the grain alive
			// but redirect its lateral velocity so it doesn't pop through.
			if gridCol == left-1 || gridCol == right+1 {
				g.VCol = math.Abs(g.VCol)
				if gridCol == right+1 {
					g.VCol = -g.VCol
				}
			}
		} else {
			// Outside the container span — let it fall off the canvas.
		}

		if g.Life <= 0 || g.Row >= float64(s.H+2) {
			continue
		}
		alive = append(alive, g)
	}
	s.grains = alive

	// Spawn an occasional splatter grain when the stream is impacting the
	// pile, to sell the granular feel.
	if s.cfg.Splatter > 0 && s.flowLevelLocked() > 0.05 && len(s.grains) < s.cfg.MaxGrains {
		if s.rng.Float64() < s.cfg.Splatter*0.15 {
			s.spawnSplatterLocked()
		}
	}
}

func (s *Sand) depositGrainLocked(g sandGrain, pileIdx int) {
	// Add a unit of pile height at this column. Grains are depicted as
	// roughly one cell tall, so each deposit raises the column by ~1.
	if pileIdx < 0 || pileIdx >= len(s.pile) {
		return
	}
	s.pile[pileIdx] += 1.0
	// Cap at container depth so we don't grow indefinitely.
	brim, bottom, _, _ := s.containerGeometryLocked()
	maxH := float64(bottom-brim) + 2
	if s.pile[pileIdx] > maxH {
		s.pile[pileIdx] = maxH
	}
}

func (s *Sand) settlePileLocked() {
	if len(s.pile) <= 1 {
		return
	}
	repose := s.cfg.Repose
	if repose < 0.5 {
		repose = 0.5
	}
	passes := s.cfg.SettlePerTick
	if passes < 1 {
		passes = 1
	}
	for p := 0; p < passes; p++ {
		moved := false
		// Alternate sweep direction so the pile doesn't bias one way.
		if (p+s.tick)%2 == 0 {
			for i := 0; i < len(s.pile)-1; i++ {
				if s.tryFlowLocked(i, i+1, repose) {
					moved = true
				}
			}
			for i := len(s.pile) - 1; i > 0; i-- {
				if s.tryFlowLocked(i, i-1, repose) {
					moved = true
				}
			}
		} else {
			for i := len(s.pile) - 1; i > 0; i-- {
				if s.tryFlowLocked(i, i-1, repose) {
					moved = true
				}
			}
			for i := 0; i < len(s.pile)-1; i++ {
				if s.tryFlowLocked(i, i+1, repose) {
					moved = true
				}
			}
		}
		if !moved {
			break
		}
	}
}

// tryFlowLocked transfers a half-unit from src to dst when the height step
// exceeds the configured angle of repose. Returns whether material moved.
func (s *Sand) tryFlowLocked(src, dst int, repose float64) bool {
	if src < 0 || src >= len(s.pile) || dst < 0 || dst >= len(s.pile) {
		return false
	}
	delta := s.pile[src] - s.pile[dst]
	if delta <= repose {
		return false
	}
	move := (delta - repose) * 0.5
	if move < 0.05 {
		return false
	}
	s.pile[src] -= move
	s.pile[dst] += move
	return true
}

func (s *Sand) applyEndingDrainLocked() {
	if s.endingTicks <= 0 {
		return
	}
	progress := phaseProgress(s.endingTotal, s.endingTicks)
	target := clamp01(s.cfg.EndingResidue)
	// Slowly converge each column toward target * starting-pile-height. We
	// approximate "starting" by reading a per-tick sample of the maximum and
	// blending toward it.
	for i := range s.pile {
		s.pile[i] = s.pile[i]*(1-0.04*progress) + target*s.pile[i]*0.04*progress
	}
}

func (s *Sand) spawnSplatterLocked() {
	if len(s.grains) >= s.cfg.MaxGrains {
		return
	}
	_, bottom, left, right := s.containerGeometryLocked()
	if right <= left {
		return
	}
	_, _, _, pipeCenter := s.pipeGeometryLocked()
	idx := pipeCenter - s.pileLeft
	if idx < 0 || idx >= len(s.pile) {
		return
	}
	row := float64(bottom) - s.pile[idx]
	col := float64(pipeCenter) + (s.rng.Float64()*2-1)*s.cfg.StreamSpread*1.4
	vRow := -(0.35 + s.rng.Float64()*0.3)
	vCol := (s.rng.Float64()*2 - 1) * 0.55
	hue := math.Mod(s.cfg.Hue+(s.rng.Float64()*2-1)*s.cfg.HueSpread*0.7+360, 360)
	light := clamp01(s.cfg.LightnessMax * (0.85 + s.rng.Float64()*0.15))
	c := hslToRGB(hue, clamp01(s.cfg.Saturation*0.85), light)
	maxLife := jitterInt(s.rng, 22, 0.3)
	s.grains = append(s.grains, sandGrain{
		Row:     row,
		Col:     col,
		VRow:    vRow,
		VCol:    vCol,
		Life:    maxLife,
		MaxLife: maxLife,
		Color:   c,
		Bright:  0.95,
	})
}

func (s *Sand) paintContainerLocked() {
	brim, bottom, left, right := s.containerGeometryLocked()
	wall := s.wallThickLocked()
	wallHue := math.Mod(s.cfg.PipeHue+360, 360)
	wallC := hslToRGB(wallHue, 0.4, s.cfg.PipeLight)
	wallC2 := hslToRGB(wallHue, 0.32, clamp01(s.cfg.PipeLight*0.7))
	for y := bottom; y < bottom+wall && y < s.H; y++ {
		for x := left - wall; x <= right+wall && x < s.W; x++ {
			if x < 0 {
				continue
			}
			c := wallC
			if y > bottom {
				c = wallC2
			}
			s.paintMax(y, x, c)
		}
	}
	for y := brim; y <= bottom; y++ {
		for w := 0; w < wall; w++ {
			s.paintMax(y, left-1-w, wallC)
			s.paintMax(y, right+1+w, wallC)
		}
	}
	if brim-1 >= 0 {
		highlight := hslToRGB(wallHue, 0.3, clamp01(s.cfg.PipeLight*1.4))
		for w := 0; w < wall; w++ {
			s.paintMax(brim-1, left-1-w, highlight)
			s.paintMax(brim-1, right+1+w, highlight)
		}
	}
}

func (s *Sand) paintPileLocked() {
	if len(s.pile) == 0 {
		return
	}
	_, bottom, left, right := s.containerGeometryLocked()
	if right <= left {
		return
	}
	hue := math.Mod(s.cfg.Hue+360, 360)
	for i, h := range s.pile {
		col := s.pileLeft + i
		if col < left || col > right {
			continue
		}
		if h <= 0 {
			continue
		}
		topRow := bottom - int(math.Round(h))
		if topRow < 0 {
			topRow = 0
		}
		for y := topRow; y <= bottom; y++ {
			depth := float64(bottom - y)
			frac := 0.0
			if h > 0 {
				frac = depth / h
			}
			// Bright at the surface, slightly darker as we go down.
			ridge := 1.0 - clamp01(frac)
			grain := 0.5 + 0.5*math.Sin(float64(col)*0.81+depth*0.37+float64(s.tick)*0.0)
			localHue := math.Mod(hue+(grain-0.5)*s.cfg.HueSpread*0.6+360, 360)
			light := clamp01(s.cfg.LightnessMin + (s.cfg.LightnessMax-s.cfg.LightnessMin)*(0.25+0.55*ridge+0.18*grain))
			c := hslToRGB(localHue, clamp01(s.cfg.Saturation*0.92), light)
			s.paintMax(y, col, c)
		}
	}
}

func (s *Sand) paintPipeLocked() {
	lipRow, left, right, _ := s.pipeGeometryLocked()
	wallHue := math.Mod(s.cfg.PipeHue+360, 360)
	body := hslToRGB(wallHue, 0.55, s.cfg.PipeLight)
	rim := hslToRGB(wallHue, 0.45, clamp01(s.cfg.PipeLight*1.5))
	shade := hslToRGB(wallHue, 0.45, clamp01(s.cfg.PipeLight*0.65))
	for y := 0; y <= lipRow; y++ {
		for x := left; x <= right; x++ {
			c := body
			if x == left || x == right {
				c = shade
			}
			s.paintMax(y, x, c)
		}
	}
	for x := left - 1; x <= right+1; x++ {
		if x < 0 || x >= s.W {
			continue
		}
		s.paintMax(lipRow, x, rim)
	}
}

func (s *Sand) paintGrainsLocked() {
	for _, g := range s.grains {
		fade := clamp01(float64(g.Life) / float64(max(1, g.MaxLife)))
		if fade <= 0 {
			continue
		}
		row := int(math.Round(g.Row))
		col := int(math.Round(g.Col))
		bright := g.Bright * (0.5 + 0.5*fade)
		c := g.Color
		c.R = uint8(float64(c.R) * bright)
		c.G = uint8(float64(c.G) * bright)
		c.B = uint8(float64(c.B) * bright)
		s.paintMax(row, col, c)
	}
}

func (s *Sand) paintMax(row, col int, c color.RGBA) {
	if row < 0 || row >= s.H || col < 0 || col >= s.W {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cur := s.Grid[row][col]
	if !cur.Filled {
		s.Grid[row][col] = Pixel{Filled: true, C: c}
		return
	}
	if c.R > cur.C.R {
		cur.C.R = c.R
	}
	if c.G > cur.C.G {
		cur.C.G = c.G
	}
	if c.B > cur.C.B {
		cur.C.B = c.B
	}
	cur.C.A = 255
	cur.Filled = true
	s.Grid[row][col] = cur
}
