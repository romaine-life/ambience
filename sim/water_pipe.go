package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type waterPipeDroplet struct {
	Row, Col   float64
	VRow, VCol float64
	Life       int
	MaxLife    int
	Color      color.RGBA
}

type waterPipeRipple struct {
	Col      float64
	Radius   float64
	Speed    float64
	Life     int
	MaxLife  int
	Strength float64
}

type waterPipeRunoff struct {
	Col      float64
	Vel      float64
	Life     int
	MaxLife  int
	Strength float64
	Side     int // -1 left, +1 right
}

// WaterPipeConfig tunes the pipe-into-pool effect used in isolated dev sessions.
type WaterPipeConfig struct {
	// INTRODUCTION
	IntroDur  int     `json:"intro_dur"`
	IntroDrip float64 `json:"intro_drip"`
	IntroFill float64 `json:"intro_fill"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
	// PIPE / BASIN GEOMETRY
	PipeX       float64 `json:"pipe_x"`
	PipeY       float64 `json:"pipe_y"`
	PipeWidth   float64 `json:"pipe_width"`
	StreamWidth float64 `json:"stream_width"`
	BasinY      float64 `json:"basin_y"`
	BasinSpan   float64 `json:"basin_span"`
	BasinDepth  float64 `json:"basin_depth"`
	WallThick   float64 `json:"wall_thick"`
	// FLOW / OVERFLOW
	Inflow         float64 `json:"inflow"`
	Drain          float64 `json:"drain"`
	OverflowSpeed  float64 `json:"overflow_speed"`
	OverflowFade   float64 `json:"overflow_fade"`
	SplatterChance float64 `json:"splatter_p"`
	MaxDroplets    int     `json:"droplet_max"`
	RippleEvery    int     `json:"ripple_every"`
	MaxRipples     int     `json:"ripple_max"`
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
	DryUpChance float64 `json:"dry_p"`
	// EVENT MODIFIERS
	SurgeDur  int     `json:"surge_dur"`
	SurgeMult float64 `json:"surge_mult"`
	DryDur    int     `json:"dry_dur"`
	DryMult   float64 `json:"dry_mult"`
}

func (c WaterPipeConfig) withDefaults() WaterPipeConfig {
	if c.IntroDur == 0 && c.IntroDrip == 0 && c.IntroFill == 0 {
		c.IntroDur = 70
		c.IntroDrip = 0.12
		c.IntroFill = 0.05
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 70
		}
		if c.IntroDrip <= 0 {
			c.IntroDrip = 0.12
		}
		if c.IntroFill < 0 {
			c.IntroFill = 0
		}
	}
	c.IntroDrip = clamp01(c.IntroDrip)
	c.IntroFill = clamp01(c.IntroFill)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingResidue == 0 {
		c.EndingDur = 70
		c.EndingLinger = 30
		c.EndingResidue = 0.18
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
		c.PipeX = 0.32
	}
	c.PipeX = clamp01(c.PipeX)
	if c.PipeY <= 0 {
		c.PipeY = 0.18
	}
	c.PipeY = clamp01(c.PipeY)
	if c.PipeWidth <= 0 {
		c.PipeWidth = 6
	}
	if c.StreamWidth <= 0 {
		c.StreamWidth = 3
	}
	if c.BasinY <= 0 {
		c.BasinY = 0.66
	}
	c.BasinY = clamp01(c.BasinY)
	if c.BasinSpan <= 0 {
		c.BasinSpan = 0.34
	}
	c.BasinSpan = clamp01(c.BasinSpan)
	if c.BasinDepth <= 0 {
		c.BasinDepth = 8
	}
	if c.WallThick <= 0 {
		c.WallThick = 1
	}
	if c.Inflow <= 0 {
		c.Inflow = 1.0
	}
	if c.Drain < 0 {
		c.Drain = 0
	}
	if c.OverflowSpeed <= 0 {
		c.OverflowSpeed = 0.55
	}
	if c.OverflowFade <= 0 {
		c.OverflowFade = 0.045
	}
	if c.SplatterChance < 0 {
		c.SplatterChance = 0
	}
	if c.MaxDroplets <= 0 {
		c.MaxDroplets = 36
	}
	if c.RippleEvery <= 0 {
		c.RippleEvery = 7
	}
	if c.MaxRipples <= 0 {
		c.MaxRipples = 8
	}
	if c.Hue == 0 {
		c.Hue = 198
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 14
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.55
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.42
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.84
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.PipeHue == 0 {
		c.PipeHue = 28
	}
	if c.PipeLight <= 0 {
		c.PipeLight = 0.34
	}
	c.PipeLight = clamp01(c.PipeLight)
	if c.SurgeDur <= 0 {
		c.SurgeDur = 60
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 1.8
	}
	if c.DryDur <= 0 {
		c.DryDur = 70
	}
	if c.DryMult <= 0 {
		c.DryMult = 0.25
	}
	return c
}

// WaterPipeSchema describes the Water Pipe effect's tunable knobs for the dev UI.
func WaterPipeSchema() EffectSchema {
	return EffectSchema{
		Name: "water-pipe",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent ramping from a slow drip into the full pour."},
			{Key: "intro_drip", Label: "intro drip", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.12,
				Description: "Initial pour fraction during the intro before the stream reaches full inflow."},
			{Key: "intro_fill", Label: "intro fill", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.05,
				Description: "How full the basin starts when the intro fires (0 = empty, 1 = at the brim)."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering the pour back down to a final drip."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 30,
				Description: "Extra quiet ticks for ripples and runoff to settle after the source cuts off."},
			{Key: "ending_residue", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.18,
				Description: "Fraction of pool fill that lingers as the source dries up."},
			{Key: "pipe_x", Label: "pipe x", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.32,
				Description: "Horizontal position of the pipe spout as a fraction of the canvas."},
			{Key: "pipe_y", Label: "pipe y", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Vertical position of the pipe lip as a fraction from the top of the canvas."},
			{Key: "pipe_width", Label: "pipe width", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 3, Max: 16, Step: 0.5, Default: 6,
				Description: "Outside diameter of the pipe spout in cells."},
			{Key: "stream_width", Label: "stream width", Slot: SlotLever, Group: "pipe", Type: KnobFloat, Min: 1, Max: 10, Step: 0.25, Default: 3,
				Description: "Thickness of the falling stream coming out of the pipe."},
			{Key: "basin_y", Label: "basin y", Slot: SlotLever, Group: "basin", Type: KnobFloat, Min: 0.45, Max: 0.9, Step: 0.01, Default: 0.66,
				Description: "Vertical position of the basin's brim as a fraction from the top of the canvas."},
			{Key: "basin_span", Label: "basin span", Slot: SlotLever, Group: "basin", Type: KnobFloat, Min: 0.12, Max: 0.7, Step: 0.01, Default: 0.34,
				Description: "Horizontal span of the basin opening relative to canvas width."},
			{Key: "basin_depth", Label: "basin depth", Slot: SlotLever, Group: "basin", Type: KnobFloat, Min: 3, Max: 24, Step: 0.5, Default: 8,
				Description: "Depth of the basin in cells, including its walls."},
			{Key: "wall_thick", Label: "wall thick", Slot: SlotLever, Group: "basin", Type: KnobFloat, Min: 1, Max: 4, Step: 0.5, Default: 1,
				Description: "Thickness of the basin walls in cells."},
			{Key: "inflow", Label: "inflow", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.1, Max: 3, Step: 0.05, Default: 1,
				Description: "How fast the pour adds water to the basin per tick."},
			{Key: "drain", Label: "drain", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.005, Default: 0,
				Description: "Passive drain rate — small values let a steady inflow stabilize below the brim."},
			{Key: "overflow_speed", Label: "overflow speed", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.1, Max: 2, Step: 0.05, Default: 0.55,
				Description: "How fast the overflow runs along the floor away from the basin."},
			{Key: "overflow_fade", Label: "overflow fade", Slot: SlotLever, Group: "flow", Type: KnobFloat, Min: 0.005, Max: 0.2, Step: 0.005, Default: 0.045,
				Description: "How quickly the overflow tapers as it streams across the floor."},
			{Key: "splatter_p", Label: "splatter", Slot: SlotLever, Group: "accents", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.42,
				Description: "Per-tick chance of a stray droplet flying off the impact zone."},
			{Key: "droplet_max", Label: "max droplets", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 4, Max: 96, Step: 1, Default: 36,
				Description: "Maximum live droplets bouncing around the pipe and basin."},
			{Key: "ripple_every", Label: "ripple every", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 7,
				Description: "Typical cadence between basin ripple beats, in ticks."},
			{Key: "ripple_max", Label: "max ripples", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 8,
				Description: "Maximum live ripple fronts expanding across the basin."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 240, Step: 1, Default: 198,
				Description: "Base hue of the water and accents."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 14,
				Description: "Variation in hue across the stream and surface highlights."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Overall color saturation of the water."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.42,
				Description: "Minimum lightness used for the basin body and shadowed water."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.98, Step: 0.01, Default: 0.84,
				Description: "Maximum lightness used for highlights and froth."},
			{Key: "pipe_hue", Label: "pipe hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 28,
				Description: "Base hue used for the pipe and basin walls."},
			{Key: "pipe_light", Label: "pipe light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.34,
				Description: "Base lightness of the pipe and basin walls."},
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surge",
				Description: "Per-tick chance of a temporary increase in inflow rate."},
			{Key: "dry_p", Label: "dry-up", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "dry-up",
				Description: "Per-tick chance of the pipe drying up to a near-pause."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60,
				Description: "Typical surge duration in ticks (jittered ±30%)."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.8,
				Description: "Inflow multiplier applied while a surge is active."},
			{Key: "dry_dur", Label: "dry dur", Slot: SlotEventMod, Group: "dry-up", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70,
				Description: "Typical dry-up duration in ticks (jittered ±30%)."},
			{Key: "dry_mult", Label: "dry x", Slot: SlotEventMod, Group: "dry-up", Type: KnobFloat, Min: 0.0, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Inflow multiplier applied while a dry-up is active (0 = pipe shut)."},
		},
	}
}

type WaterPipeState struct {
	Tick           int     `json:"tick"`
	SurgeTicks     int     `json:"surgeTicks"`
	DryTicks       int     `json:"dryTicks"`
	IntroTicks     int     `json:"introTicks"`
	IntroTotal     int     `json:"introTotal"`
	EndingTicks    int     `json:"endingTicks"`
	EndingTotal    int     `json:"endingTotal"`
	EndingFade     int     `json:"endingFade"`
	RippleCooldown int     `json:"rippleCooldown"`
	Fill           float64 `json:"fill"`
}

type WaterPipeDroplet struct {
	Row     float64 `json:"row"`
	Col     float64 `json:"col"`
	VRow    float64 `json:"vRow"`
	VCol    float64 `json:"vCol"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
	Color   RGB     `json:"color"`
}

type WaterPipeRipple struct {
	Col      float64 `json:"col"`
	Radius   float64 `json:"radius"`
	Speed    float64 `json:"speed"`
	Life     int     `json:"life"`
	MaxLife  int     `json:"maxLife"`
	Strength float64 `json:"strength"`
}

type WaterPipeRunoff struct {
	Col      float64 `json:"col"`
	Vel      float64 `json:"vel"`
	Life     int     `json:"life"`
	MaxLife  int     `json:"maxLife"`
	Strength float64 `json:"strength"`
	Side     int     `json:"side"`
}

type WaterPipeSnapshot struct {
	WaterPipeState
	RNGState uint64             `json:"rngState,omitempty"`
	Droplets []WaterPipeDroplet `json:"droplets"`
	Ripples  []WaterPipeRipple  `json:"ripples"`
	Runoff   []WaterPipeRunoff  `json:"runoff"`
}

type WaterPipePersistedState struct {
	WaterPipeState
	RNGState uint64             `json:"rngState"`
	Droplets []WaterPipeDroplet `json:"droplets"`
	Ripples  []WaterPipeRipple  `json:"ripples"`
	Runoff   []WaterPipeRunoff  `json:"runoff"`
}

// WaterPipe is a stylized "pipe pouring into a basin, overflowing" prototype
// used in isolated dev sessions.
type WaterPipe struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	droplets []waterPipeDroplet
	ripples  []waterPipeRipple
	runoff   []waterPipeRunoff
	rng      *rngutil.RNG
	cfg      WaterPipeConfig
	tick     int

	surgeTicks     int
	dryTicks       int
	introTicks     int
	introTotal     int
	endingTicks    int
	endingTotal    int
	endingFade     int
	rippleCooldown int
	fill           float64

	log []LogEntry
}

func NewWaterPipe(w, h int, seed int64, cfg WaterPipeConfig) *WaterPipe {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &WaterPipe{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
}

func (p *WaterPipe) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if width == p.W && height == p.H {
		return
	}
	p.W = width
	p.H = height
	p.Grid = make([][]Pixel, height)
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, width)
	}
}

func (p *WaterPipe) SetConfig(cfg WaterPipeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg.withDefaults()
}

func (p *WaterPipe) EffectiveConfig() WaterPipeConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *WaterPipe) SnapshotState() WaterPipeState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotStateLocked()
}

func (p *WaterPipe) Snapshot() WaterPipeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return WaterPipeSnapshot{
		WaterPipeState: p.snapshotStateLocked(),
		RNGState:       p.rng.State(),
		Droplets:       p.copyDropletsLocked(),
		Ripples:        p.copyRipplesLocked(),
		Runoff:         p.copyRunoffLocked(),
	}
}

func (p *WaterPipe) RestoreState(s WaterPipeState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s)
}

func (p *WaterPipe) RestoreSnapshot(s WaterPipeSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.WaterPipeState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreDropletsLocked(s.Droplets)
	p.restoreRipplesLocked(s.Ripples)
	p.restoreRunoffLocked(s.Runoff)
	p.paintFrameLocked()
}

func (p *WaterPipe) SnapshotPersistedState() WaterPipePersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return WaterPipePersistedState{
		WaterPipeState: p.snapshotStateLocked(),
		RNGState:       p.rng.State(),
		Droplets:       p.copyDropletsLocked(),
		Ripples:        p.copyRipplesLocked(),
		Runoff:         p.copyRunoffLocked(),
	}
}

func (p *WaterPipe) RestorePersistedState(s WaterPipePersistedState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.WaterPipeState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreDropletsLocked(s.Droplets)
	p.restoreRipplesLocked(s.Ripples)
	p.restoreRunoffLocked(s.Runoff)
}

func (p *WaterPipe) CurrentTick() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tick
}

func (p *WaterPipe) PerturbRNG(delta int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rng.Mix(delta)
}

func (p *WaterPipe) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "surge":
		p.surgeTicks = jitterInt(p.rng, p.cfg.SurgeDur, 0.3)
		p.appendLog("surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", p.surgeTicks, p.cfg.SurgeMult))
	case "dry-up":
		p.dryTicks = jitterInt(p.rng, p.cfg.DryDur, 0.3)
		p.appendLog("dry-up", fmt.Sprintf("triggered (dur=%d, x%.2f)", p.dryTicks, p.cfg.DryMult))
	case "intro":
		p.startIntroductionLocked()
		p.appendLog("intro", fmt.Sprintf("started (dur=%d, drip=%.2f)", p.introTotal, p.cfg.IntroDrip))
	case "ending":
		p.startEndingLocked()
		p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.endingFade, p.endingTotal-p.endingFade))
	default:
		return false
	}
	return true
}

func (p *WaterPipe) DrainLog() []LogEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.log) == 0 {
		return nil
	}
	out := p.log
	p.log = nil
	return out
}

func (p *WaterPipe) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *WaterPipe) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	if p.surgeTicks > 0 {
		p.surgeTicks--
	}
	if p.dryTicks > 0 {
		p.dryTicks--
	}
	if p.introTicks > 0 {
		p.introTicks--
	}
	if p.endingTicks > 0 {
		p.endingTicks--
	}
	if p.rippleCooldown > 0 {
		p.rippleCooldown--
	}

	if p.surgeTicks == 0 && p.rng.Float64() < p.cfg.SurgeChance {
		p.surgeTicks = jitterInt(p.rng, p.cfg.SurgeDur, 0.3)
		p.appendLog("surge", fmt.Sprintf("started (dur=%d, x%.2f)", p.surgeTicks, p.cfg.SurgeMult))
	}
	if p.dryTicks == 0 && p.rng.Float64() < p.cfg.DryUpChance {
		p.dryTicks = jitterInt(p.rng, p.cfg.DryDur, 0.3)
		p.appendLog("dry-up", fmt.Sprintf("started (dur=%d, x%.2f)", p.dryTicks, p.cfg.DryMult))
	}

	p.updateFillLocked()
	p.stepDropletsLocked()
	p.stepRipplesLocked()
	p.stepRunoffLocked()
	p.spawnRippleLocked()
	p.spawnRunoffLocked()
	p.spawnSplatterLocked()

	p.paintFrameLocked()
}

func (p *WaterPipe) paintFrameLocked() {
	p.clearGridLocked()
	p.paintBasinLocked()
	p.paintPoolLocked()
	p.paintStreamLocked()
	p.paintImpactLocked()
	p.paintRipplesLocked()
	p.paintRunoffLocked()
	p.paintPipeLocked()
	p.paintDropletsLocked()
}

func (p *WaterPipe) snapshotStateLocked() WaterPipeState {
	return WaterPipeState{
		Tick:           p.tick,
		SurgeTicks:     p.surgeTicks,
		DryTicks:       p.dryTicks,
		IntroTicks:     p.introTicks,
		IntroTotal:     p.introTotal,
		EndingTicks:    p.endingTicks,
		EndingTotal:    p.endingTotal,
		EndingFade:     p.endingFade,
		RippleCooldown: p.rippleCooldown,
		Fill:           p.fill,
	}
}

func (p *WaterPipe) restoreStateLocked(s WaterPipeState) {
	p.tick = s.Tick
	p.surgeTicks = s.SurgeTicks
	p.dryTicks = s.DryTicks
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.endingFade = s.EndingFade
	p.rippleCooldown = s.RippleCooldown
	p.fill = s.Fill
}

func (p *WaterPipe) copyDropletsLocked() []WaterPipeDroplet {
	out := make([]WaterPipeDroplet, len(p.droplets))
	for i, d := range p.droplets {
		out[i] = WaterPipeDroplet{
			Row:     d.Row,
			Col:     d.Col,
			VRow:    d.VRow,
			VCol:    d.VCol,
			Life:    d.Life,
			MaxLife: d.MaxLife,
			Color:   RGB{R: d.Color.R, G: d.Color.G, B: d.Color.B},
		}
	}
	return out
}

func (p *WaterPipe) restoreDropletsLocked(list []WaterPipeDroplet) {
	p.droplets = make([]waterPipeDroplet, len(list))
	for i, d := range list {
		p.droplets[i] = waterPipeDroplet{
			Row:     d.Row,
			Col:     d.Col,
			VRow:    d.VRow,
			VCol:    d.VCol,
			Life:    d.Life,
			MaxLife: d.MaxLife,
			Color:   color.RGBA{R: d.Color.R, G: d.Color.G, B: d.Color.B, A: 255},
		}
	}
}

func (p *WaterPipe) copyRipplesLocked() []WaterPipeRipple {
	out := make([]WaterPipeRipple, len(p.ripples))
	for i, r := range p.ripples {
		out[i] = WaterPipeRipple{
			Col:      r.Col,
			Radius:   r.Radius,
			Speed:    r.Speed,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
		}
	}
	return out
}

func (p *WaterPipe) restoreRipplesLocked(list []WaterPipeRipple) {
	p.ripples = make([]waterPipeRipple, len(list))
	for i, r := range list {
		p.ripples[i] = waterPipeRipple{
			Col:      r.Col,
			Radius:   r.Radius,
			Speed:    r.Speed,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
		}
	}
}

func (p *WaterPipe) copyRunoffLocked() []WaterPipeRunoff {
	out := make([]WaterPipeRunoff, len(p.runoff))
	for i, r := range p.runoff {
		out[i] = WaterPipeRunoff{
			Col:      r.Col,
			Vel:      r.Vel,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
			Side:     r.Side,
		}
	}
	return out
}

func (p *WaterPipe) restoreRunoffLocked(list []WaterPipeRunoff) {
	p.runoff = make([]waterPipeRunoff, len(list))
	for i, r := range list {
		p.runoff[i] = waterPipeRunoff{
			Col:      r.Col,
			Vel:      r.Vel,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
			Side:     r.Side,
		}
	}
}

func (p *WaterPipe) startIntroductionLocked() {
	p.endingTicks = 0
	p.endingTotal = 0
	p.endingFade = 0
	p.introTotal = p.cfg.IntroDur
	if p.introTotal <= 0 {
		p.introTotal = 70
	}
	p.introTicks = p.introTotal
	p.fill = clamp01(p.cfg.IntroFill)
	p.rippleCooldown = 1
}

func (p *WaterPipe) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.endingFade = p.cfg.EndingDur
	if p.endingFade <= 0 {
		p.endingFade = 70
	}
	linger := p.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	p.endingTotal = p.endingFade + linger
	if p.endingTotal < 1 {
		p.endingTotal = p.endingFade
	}
	p.endingTicks = p.endingTotal
}

// flowLevelLocked returns the current effective inflow multiplier (event +
// lifecycle modifiers applied). 1.0 = baseline.
func (p *WaterPipe) flowLevelLocked() float64 {
	flow := 1.0
	if p.surgeTicks > 0 {
		flow *= p.cfg.SurgeMult
	}
	if p.dryTicks > 0 {
		flow *= p.cfg.DryMult
	}
	if p.introTicks > 0 {
		progress := phaseProgress(p.introTotal, p.introTicks)
		flow *= p.cfg.IntroDrip + (1-p.cfg.IntroDrip)*progress
	}
	if p.endingTicks > 0 {
		elapsed := p.endingTotal - p.endingTicks
		if elapsed < p.endingFade {
			fade := clamp01(float64(elapsed) / float64(max(1, p.endingFade-1)))
			flow *= 1 - 0.94*fade
		} else {
			flow *= 0.06
		}
	}
	if flow < 0 {
		flow = 0
	}
	return flow
}

func (p *WaterPipe) updateFillLocked() {
	flow := p.flowLevelLocked()
	inflow := p.cfg.Inflow * flow * 0.012 // ~tuned so a baseline pour fills in ~5s
	p.fill += inflow
	// Steady drain so a baseline pour can stabilize below the brim.
	p.fill -= p.cfg.Drain * 0.012
	// Endings let residual pool linger then drain to the configured residue.
	if p.endingTicks > 0 {
		progress := phaseProgress(p.endingTotal, p.endingTicks)
		target := p.cfg.EndingResidue
		p.fill = p.fill*(1-0.06*progress) + target*0.06*progress
	}
	if p.fill < 0 {
		p.fill = 0
	}
	if p.fill > 1.6 {
		p.fill = 1.6
	}
}

// pipeGeometry returns the pipe footprint in cell coordinates:
// (lipRow, leftCol, rightCol, lipCenterCol).
func (p *WaterPipe) pipeGeometryLocked() (int, int, int, int) {
	if p.W <= 0 || p.H <= 0 {
		return 0, 0, 0, 0
	}
	width := p.cfg.PipeWidth
	if width < 3 {
		width = 3
	}
	half := int(math.Round(width * 0.5))
	if half < 2 {
		half = 2
	}
	center := int(math.Round(p.cfg.PipeX * float64(p.W-1)))
	left := center - half
	right := center + half
	if left < 1 {
		left = 1
	}
	if right >= p.W-1 {
		right = p.W - 2
	}
	if right < left {
		right = left
	}
	lip := int(math.Round(p.cfg.PipeY * float64(p.H-1)))
	if lip < 2 {
		lip = 2
	}
	if lip > p.H-6 {
		lip = p.H - 6
	}
	return lip, left, right, center
}

func (p *WaterPipe) basinGeometryLocked() (int, int, int, int) {
	if p.W <= 0 || p.H <= 0 {
		return 0, 0, 0, 0
	}
	brim := int(math.Round(p.cfg.BasinY * float64(p.H-1)))
	if brim < 6 {
		brim = 6
	}
	if brim > p.H-3 {
		brim = p.H - 3
	}
	depth := int(math.Round(p.cfg.BasinDepth))
	if depth < 3 {
		depth = 3
	}
	if depth > p.H-brim-1 {
		depth = p.H - brim - 1
	}
	if depth < 2 {
		depth = 2
	}
	bottom := brim + depth
	half := int(math.Round(p.cfg.BasinSpan * float64(p.W) * 0.5))
	if half < 4 {
		half = 4
	}
	center := p.W / 2
	// Anchor basin under the pipe so the stream lands inside the rim.
	_, _, _, pipeCenter := p.pipeGeometryLocked()
	if pipeCenter > 0 {
		center = pipeCenter
	}
	left := center - half
	right := center + half
	if left < 1 {
		left = 1
	}
	if right >= p.W-1 {
		right = p.W - 2
	}
	return brim, bottom, left, right
}

func (p *WaterPipe) wallThickLocked() int {
	w := int(math.Round(p.cfg.WallThick))
	if w < 1 {
		w = 1
	}
	if w > 4 {
		w = 4
	}
	return w
}

func (p *WaterPipe) clearGridLocked() {
	for y := range p.Grid {
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{}
		}
	}
}

func (p *WaterPipe) stepDropletsLocked() {
	if len(p.droplets) == 0 {
		return
	}
	alive := p.droplets[:0]
	gravity := 0.085
	for _, d := range p.droplets {
		d.VRow += gravity
		d.Row += d.VRow
		d.Col += d.VCol
		d.Life--
		if d.Life > 0 && d.Row < float64(p.H)+1 && d.Col >= -2 && d.Col < float64(p.W)+2 {
			alive = append(alive, d)
		}
	}
	p.droplets = alive
}

func (p *WaterPipe) stepRipplesLocked() {
	if len(p.ripples) == 0 {
		return
	}
	alive := p.ripples[:0]
	for _, r := range p.ripples {
		r.Radius += r.Speed
		r.Life--
		if r.Life > 0 && r.Radius < float64(p.W) {
			alive = append(alive, r)
		}
	}
	p.ripples = alive
}

func (p *WaterPipe) stepRunoffLocked() {
	if len(p.runoff) == 0 {
		return
	}
	alive := p.runoff[:0]
	for _, r := range p.runoff {
		r.Col += r.Vel * float64(r.Side)
		r.Strength *= 1 - p.cfg.OverflowFade
		r.Life--
		if r.Life > 0 && r.Strength > 0.04 && r.Col >= -1 && r.Col <= float64(p.W)+1 {
			alive = append(alive, r)
		}
	}
	p.runoff = alive
}

func (p *WaterPipe) spawnRippleLocked() {
	if len(p.ripples) >= p.cfg.MaxRipples {
		return
	}
	if p.rippleCooldown > 0 {
		return
	}
	flow := p.flowLevelLocked()
	if flow < 0.1 {
		return
	}
	cadence := float64(p.cfg.RippleEvery) / math.Max(0.25, flow)
	if cadence < 1 {
		cadence = 1
	}
	_, _, basinLeft, basinRight := p.basinGeometryLocked()
	_, _, _, pipeCenter := p.pipeGeometryLocked()
	basinCenter := float64((basinLeft + basinRight) / 2)
	col := basinCenter + (p.rng.Float64()*2-1)*1.2
	if pipeCenter > 0 {
		col = float64(pipeCenter) + (p.rng.Float64()*2-1)*1.2
	}
	life := jitterInt(p.rng, 22, 0.25)
	speed := (0.45 + p.rng.Float64()*0.45) * (0.85 + 0.2*flow)
	strength := clamp01(0.45 + 0.3*flow + p.rng.Float64()*0.2)
	p.ripples = append(p.ripples, waterPipeRipple{
		Col:      col,
		Radius:   0,
		Speed:    speed,
		Life:     life,
		MaxLife:  life,
		Strength: strength,
	})
	p.rippleCooldown = jitterInt(p.rng, int(math.Round(cadence)), 0.25)
}

func (p *WaterPipe) spawnRunoffLocked() {
	overflow := p.fill - 1.0
	if overflow <= 0 {
		return
	}
	if p.endingTicks > 0 && p.endingTotal-p.endingTicks >= p.endingFade {
		// During the linger phase keep spawning sparingly so existing
		// runoff has time to taper visually instead of jumping.
		if p.rng.Float64() > 0.25 {
			return
		}
	}
	flow := p.flowLevelLocked()
	intensity := math.Min(1, overflow/0.6)
	if p.rng.Float64() > 0.55+0.4*intensity {
		return
	}
	_, _, basinLeft, basinRight := p.basinGeometryLocked()
	for _, side := range []int{-1, 1} {
		col := float64(basinLeft)
		if side > 0 {
			col = float64(basinRight)
		}
		vel := p.cfg.OverflowSpeed * (0.85 + 0.4*p.rng.Float64()) * (0.7 + 0.3*flow)
		life := jitterInt(p.rng, 60, 0.3)
		strength := clamp01(0.55 + 0.45*intensity + p.rng.Float64()*0.15)
		p.runoff = append(p.runoff, waterPipeRunoff{
			Col:      col,
			Vel:      vel,
			Life:     life,
			MaxLife:  life,
			Strength: strength,
			Side:     side,
		})
	}
}

func (p *WaterPipe) spawnSplatterLocked() {
	flow := p.flowLevelLocked()
	if flow <= 0.05 {
		return
	}
	chance := p.cfg.SplatterChance * (0.5 + 0.6*flow)
	if p.rng.Float64() > chance {
		return
	}
	if len(p.droplets) >= p.cfg.MaxDroplets {
		return
	}
	brim, _, _, _ := p.basinGeometryLocked()
	_, _, _, pipeCenter := p.pipeGeometryLocked()
	col := float64(pipeCenter) + (p.rng.Float64()*2-1)*p.cfg.StreamWidth*0.7
	row := float64(brim) - 1 + p.rng.Float64()*1.5
	vCol := (p.rng.Float64()*2 - 1) * (0.5 + 0.4*flow)
	vRow := -(0.6 + p.rng.Float64()*0.5) * (0.7 + 0.3*flow)
	life := jitterInt(p.rng, 18, 0.4)
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread*0.5+360, 360)
	light := clamp01(p.cfg.LightnessMax * (0.85 + p.rng.Float64()*0.15))
	colr := hslToRGB(hue, clamp01(p.cfg.Saturation*0.85), light)
	p.droplets = append(p.droplets, waterPipeDroplet{
		Row:     row,
		Col:     col,
		VRow:    vRow,
		VCol:    vCol,
		Life:    life,
		MaxLife: life,
		Color:   colr,
	})
}

func (p *WaterPipe) paintBasinLocked() {
	brim, bottom, left, right := p.basinGeometryLocked()
	wall := p.wallThickLocked()
	wallHue := math.Mod(p.cfg.PipeHue+360, 360)
	wallC := hslToRGB(wallHue, 0.45, p.cfg.PipeLight)
	wallC2 := hslToRGB(wallHue, 0.35, clamp01(p.cfg.PipeLight*0.7))
	// Bottom slab.
	for y := bottom; y < bottom+wall && y < p.H; y++ {
		for x := left - wall; x <= right+wall && x < p.W; x++ {
			if x < 0 {
				continue
			}
			c := wallC
			if y > bottom {
				c = wallC2
			}
			p.paintMax(y, x, c)
		}
	}
	// Side walls (interior cells stay empty so the pool fills them).
	for y := brim; y <= bottom; y++ {
		for w := 0; w < wall; w++ {
			p.paintMax(y, left-1-w, wallC)
			p.paintMax(y, right+1+w, wallC)
		}
	}
	// Brim highlight on the outside corners.
	if brim-1 >= 0 {
		for w := 0; w < wall; w++ {
			highlight := hslToRGB(wallHue, 0.3, clamp01(p.cfg.PipeLight*1.4))
			p.paintMax(brim-1, left-1-w, highlight)
			p.paintMax(brim-1, right+1+w, highlight)
		}
	}
}

func (p *WaterPipe) paintPoolLocked() {
	brim, bottom, left, right := p.basinGeometryLocked()
	if right <= left || bottom <= brim {
		return
	}
	depth := bottom - brim
	level := clamp01(p.fill)
	surface := bottom - int(math.Round(level*float64(depth)))
	if surface > bottom {
		surface = bottom
	}
	if surface < brim {
		surface = brim
	}
	hue := math.Mod(p.cfg.Hue+360, 360)
	for y := surface; y < bottom; y++ {
		dist := float64(y-surface) / float64(max(1, bottom-surface-1))
		shimmer := 0.7 + 0.3*math.Sin(float64(y)*0.31+float64(p.tick)*0.08)
		light := clamp01(p.cfg.LightnessMin*(0.55+0.4*dist) + (p.cfg.LightnessMax-p.cfg.LightnessMin)*0.15*shimmer)
		c := hslToRGB(math.Mod(hue-6+360, 360), clamp01(p.cfg.Saturation*0.85), light)
		for x := left; x <= right; x++ {
			p.paintMax(y, x, c)
		}
	}
	// Surface line — slightly brighter band.
	if surface >= brim && surface <= bottom {
		light := clamp01(p.cfg.LightnessMax * 0.85)
		c := hslToRGB(math.Mod(hue+2+360, 360), clamp01(p.cfg.Saturation*0.7), light)
		for x := left; x <= right; x++ {
			wave := math.Sin(float64(x)*0.42+float64(p.tick)*0.12) * 0.3
			row := surface + int(math.Round(wave))
			if row < brim {
				row = brim
			}
			if row > bottom {
				row = bottom
			}
			p.paintMax(row, x, c)
		}
	}
}

func (p *WaterPipe) paintStreamLocked() {
	lipRow, pipeLeft, pipeRight, pipeCenter := p.pipeGeometryLocked()
	brim, bottom, basinLeft, basinRight := p.basinGeometryLocked()
	flow := p.flowLevelLocked()
	if flow <= 0.02 {
		return
	}
	// Stream lands at the pipe center, flowing into the pool surface (or basin
	// floor when nearly empty).
	depth := bottom - brim
	level := clamp01(p.fill)
	surface := bottom - int(math.Round(level*float64(depth)))
	if surface < brim+1 {
		surface = brim + 1
	}
	streamTop := lipRow + 1
	streamBottom := surface
	if streamBottom <= streamTop {
		return
	}
	width := math.Max(1, p.cfg.StreamWidth*flow)
	hue := math.Mod(p.cfg.Hue+360, 360)
	for y := streamTop; y < streamBottom; y++ {
		progress := float64(y-streamTop) / float64(max(1, streamBottom-streamTop-1))
		// A faint sway makes it read as falling water rather than a pasted column.
		sway := math.Sin(float64(y)*0.55-float64(p.tick)*0.18) * 0.6 * width * 0.18
		rowCenter := float64(pipeCenter) + sway
		half := math.Max(0.6, width*0.5)
		start := int(math.Floor(rowCenter - half))
		end := int(math.Ceil(rowCenter + half))
		if start < 0 {
			start = 0
		}
		if end >= p.W {
			end = p.W - 1
		}
		for x := start; x <= end; x++ {
			dist := math.Abs((float64(x)+0.5)-rowCenter) / half
			if dist > 1.05 {
				continue
			}
			edge := clamp01(1 - dist*dist)
			pulse := 0.7 + 0.3*math.Sin(progress*9-float64(p.tick)*0.36+float64(x)*0.4)
			intensity := edge * pulse
			if intensity < 0.1 {
				continue
			}
			h := math.Mod(hue+math.Sin(progress*2+float64(x)*0.1)*p.cfg.HueSpread*0.5+360, 360)
			light := clamp01(p.cfg.LightnessMin + (p.cfg.LightnessMax-p.cfg.LightnessMin)*(0.35+0.6*intensity))
			c := hslToRGB(h, p.cfg.Saturation, light)
			p.paintMax(y, x, c)
		}
	}
	// A bright lip drip just below the pipe spout so the eye reads "source".
	for x := pipeLeft; x <= pipeRight; x++ {
		if math.Abs(float64(x-pipeCenter)) > p.cfg.StreamWidth*0.55 {
			continue
		}
		c := hslToRGB(hue, clamp01(p.cfg.Saturation*0.6), clamp01(p.cfg.LightnessMax*0.95))
		p.paintMax(lipRow+1, x, c)
	}
	_ = basinLeft
	_ = basinRight
}

func (p *WaterPipe) paintImpactLocked() {
	flow := p.flowLevelLocked()
	if flow <= 0.05 {
		return
	}
	brim, bottom, _, _ := p.basinGeometryLocked()
	depth := bottom - brim
	level := clamp01(p.fill)
	surface := bottom - int(math.Round(level*float64(depth)))
	if surface < brim {
		surface = brim
	}
	if surface > bottom {
		surface = bottom
	}
	_, _, _, pipeCenter := p.pipeGeometryLocked()
	radius := int(math.Round(math.Max(2, p.cfg.StreamWidth*flow*0.7)))
	hue := math.Mod(p.cfg.Hue+360, 360)
	for dx := -radius; dx <= radius; dx++ {
		x := pipeCenter + dx
		if x < 0 || x >= p.W {
			continue
		}
		dist := math.Abs(float64(dx)) / float64(radius+1)
		if dist > 1 {
			continue
		}
		foam := clamp01((1 - dist*dist) * (0.65 + 0.25*flow))
		light := clamp01(p.cfg.LightnessMin + (p.cfg.LightnessMax-p.cfg.LightnessMin)*(0.6+0.4*foam))
		c := hslToRGB(math.Mod(hue+10+360, 360), clamp01(p.cfg.Saturation*0.4), light)
		p.paintMax(surface, x, c)
		if surface-1 >= 0 {
			p.paintMax(surface-1, x, c)
		}
	}
}

func (p *WaterPipe) paintRipplesLocked() {
	if len(p.ripples) == 0 {
		return
	}
	brim, bottom, left, right := p.basinGeometryLocked()
	depth := bottom - brim
	level := clamp01(p.fill)
	surface := bottom - int(math.Round(level*float64(depth)))
	if surface < brim {
		surface = brim
	}
	if surface > bottom {
		surface = bottom
	}
	hue := math.Mod(p.cfg.Hue+360, 360)
	for _, r := range p.ripples {
		fade := clamp01(float64(r.Life) / float64(max(1, r.MaxLife)))
		if fade <= 0 {
			continue
		}
		for x := left; x <= right; x++ {
			wave := math.Abs(math.Abs(float64(x)-r.Col) - r.Radius)
			if wave > 0.85 {
				continue
			}
			bright := r.Strength * fade * (1 - wave/0.85)
			light := clamp01(p.cfg.LightnessMin*0.85 + (p.cfg.LightnessMax-p.cfg.LightnessMin)*(0.25+0.6*bright))
			c := hslToRGB(math.Mod(hue-6+360, 360), clamp01(p.cfg.Saturation*0.7), light)
			p.paintMax(surface, x, c)
		}
	}
}

func (p *WaterPipe) paintRunoffLocked() {
	if len(p.runoff) == 0 {
		return
	}
	_, bottom, basinLeft, basinRight := p.basinGeometryLocked()
	wall := p.wallThickLocked()
	floor := bottom + wall // top of overflow band
	if floor >= p.H {
		floor = p.H - 1
	}
	hue := math.Mod(p.cfg.Hue+360, 360)
	for _, r := range p.runoff {
		fade := clamp01(float64(r.Life) / float64(max(1, r.MaxLife)))
		intensity := r.Strength * fade
		if intensity <= 0.02 {
			continue
		}
		col := int(math.Round(r.Col))
		if r.Side > 0 && col < basinRight {
			col = basinRight + 1
		}
		if r.Side < 0 && col > basinLeft {
			col = basinLeft - 1
		}
		if col < 0 || col >= p.W {
			continue
		}
		light := clamp01(p.cfg.LightnessMin*0.85 + (p.cfg.LightnessMax-p.cfg.LightnessMin)*(0.3+0.6*intensity))
		c := hslToRGB(math.Mod(hue-4+360, 360), clamp01(p.cfg.Saturation*0.75), light)
		p.paintMax(floor, col, c)
		if floor+1 < p.H && intensity > 0.3 {
			dim := c
			dim.R = uint8(float64(dim.R) * 0.75)
			dim.G = uint8(float64(dim.G) * 0.75)
			dim.B = uint8(float64(dim.B) * 0.75)
			p.paintMax(floor+1, col, dim)
		}
		// Soft trailing edge so streams read as flow, not a single moving dot.
		trail := int(math.Round(2 + 3*intensity))
		for t := 1; t <= trail; t++ {
			tcol := col - r.Side*t
			if tcol < 0 || tcol >= p.W {
				continue
			}
			tfade := intensity * (1 - float64(t)/float64(trail+1))
			if tfade <= 0.05 {
				continue
			}
			tlight := clamp01(p.cfg.LightnessMin + (p.cfg.LightnessMax-p.cfg.LightnessMin)*(0.2+0.5*tfade))
			tc := hslToRGB(math.Mod(hue-8+360, 360), clamp01(p.cfg.Saturation*0.65), tlight)
			p.paintMax(floor, tcol, tc)
		}
	}
}

func (p *WaterPipe) paintPipeLocked() {
	lipRow, left, right, _ := p.pipeGeometryLocked()
	wallHue := math.Mod(p.cfg.PipeHue+360, 360)
	body := hslToRGB(wallHue, 0.55, p.cfg.PipeLight)
	rim := hslToRGB(wallHue, 0.45, clamp01(p.cfg.PipeLight*1.5))
	shade := hslToRGB(wallHue, 0.45, clamp01(p.cfg.PipeLight*0.65))
	// Pipe body extends from the top edge down to the lip.
	for y := 0; y <= lipRow; y++ {
		for x := left; x <= right; x++ {
			c := body
			if x == left || x == right {
				c = shade
			}
			p.paintMax(y, x, c)
		}
	}
	// Pipe rim — a brighter horizontal band right at the lip.
	for x := left; x <= right; x++ {
		p.paintMax(lipRow, x, rim)
	}
	// Optional bottom lip overhang for shape.
	if lipRow+1 < p.H {
		for x := left - 1; x <= right+1; x++ {
			if x < 0 || x >= p.W {
				continue
			}
			p.paintMax(lipRow, x, rim)
		}
	}
}

func (p *WaterPipe) paintDropletsLocked() {
	for _, d := range p.droplets {
		fade := clamp01(float64(d.Life) / float64(max(1, d.MaxLife)))
		if fade <= 0 {
			continue
		}
		row := int(math.Round(d.Row))
		col := int(math.Round(d.Col))
		c := d.Color
		scale := 0.3 + 0.7*fade
		c.R = uint8(float64(c.R) * scale)
		c.G = uint8(float64(c.G) * scale)
		c.B = uint8(float64(c.B) * scale)
		p.paintMax(row, col, c)
	}
}

func (p *WaterPipe) paintMax(row, col int, c color.RGBA) {
	if row < 0 || row >= p.H || col < 0 || col >= p.W {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cur := p.Grid[row][col]
	if !cur.Filled {
		p.Grid[row][col] = Pixel{Filled: true, C: c}
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
	p.Grid[row][col] = cur
}
