package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type paperLantern struct {
	X, Y      float64
	VX, VY    float64
	W, H      float64
	HueOffset float64
	Light     float64
	Age       int
	SwayPhase float64
	SwaySpeed float64
	SwayAmp   float64
	Layer     int
	FadeLeft  int
	FadeTotal int
}

// PaperLanternsConfig tunes the paper-lantern release effect used in isolated
// dev sessions and shared effect rotation.
type PaperLanternsConfig struct {
	// INTRODUCTION
	IntroFirstDelay   int `json:"intro_first"`
	IntroClusterDelay int `json:"intro_cluster"`
	// ENDING
	EndingStop int `json:"ending_stop"`
	TailLength int `json:"tail_len"`
	// LEVERS - motion / density
	Wind         float64 `json:"wind"`
	WindDrift    float64 `json:"wind_drift"`
	RiseSpeed    float64 `json:"rise_speed"`
	SpeedJitter  float64 `json:"speed_jit"`
	Sway         float64 `json:"sway"`
	SpawnChance  float64 `json:"emit_p"`
	MaxLanterns  int     `json:"max"`
	LanternSize  float64 `json:"size"`
	Glow         float64 `json:"glow"`
	FadeAltitude float64 `json:"fade_alt"`
	// LEVERS - color
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// EVENT CHANCES
	ReleaseChance float64 `json:"release_p"`
	WindChance    float64 `json:"wind_p"`
	QuietChance   float64 `json:"quiet_p"`
	FadeChance    float64 `json:"fade_p"`
	// EVENT MODIFIERS
	ClusterMin    int     `json:"cluster_min"`
	ClusterMax    int     `json:"cluster_max"`
	ReleaseWindow int     `json:"release_window"`
	WindDur       int     `json:"wind_dur"`
	WindStrength  float64 `json:"wind_str"`
	QuietDur      int     `json:"quiet_dur"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroFirstDelay <= 0 {
		c.IntroFirstDelay = 45
	}
	if c.IntroClusterDelay <= 0 {
		c.IntroClusterDelay = 150
	}
	if c.IntroClusterDelay < c.IntroFirstDelay {
		c.IntroClusterDelay = c.IntroFirstDelay + 1
	}
	if c.EndingStop < 0 {
		c.EndingStop = 0
	}
	if c.EndingStop == 0 {
		c.EndingStop = 18
	}
	if c.TailLength <= 0 {
		c.TailLength = 420
	}
	if c.WindDrift < 0 {
		c.WindDrift = 0
	}
	if c.WindDrift == 0 {
		c.WindDrift = 0.05
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.18
	}
	if c.SpeedJitter < 0 {
		c.SpeedJitter = 0
	}
	if c.SpeedJitter == 0 {
		c.SpeedJitter = 0.25
	}
	if c.Sway < 0 {
		c.Sway = 0
	}
	if c.Sway == 0 {
		c.Sway = 0.22
	}
	if c.SpawnChance < 0 {
		c.SpawnChance = 0
	}
	if c.SpawnChance == 0 {
		c.SpawnChance = 0.006
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 72
	}
	if c.MaxLanterns > 240 {
		c.MaxLanterns = 240
	}
	if c.LanternSize <= 0 {
		c.LanternSize = 3.2
	}
	if c.Glow <= 0 {
		c.Glow = 0.72
	}
	c.Glow = clamp01(c.Glow)
	if c.FadeAltitude <= 0 {
		c.FadeAltitude = 0.28
	}
	c.FadeAltitude = clamp01(c.FadeAltitude)
	if c.Hue == 0 {
		c.Hue = 34
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.HueSpread == 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.78
	}
	c.Saturation = clamp01(c.Saturation)
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.18
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.88
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.ReleaseChance < 0 {
		c.ReleaseChance = 0
	}
	if c.ReleaseChance == 0 {
		c.ReleaseChance = 0.0008
	}
	if c.WindChance < 0 {
		c.WindChance = 0
	}
	if c.WindChance == 0 {
		c.WindChance = 0.00025
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.QuietChance == 0 {
		c.QuietChance = 0.00012
	}
	if c.FadeChance < 0 {
		c.FadeChance = 0
	}
	if c.ClusterMin <= 0 {
		c.ClusterMin = 5
	}
	if c.ClusterMax <= 0 {
		c.ClusterMax = 10
	}
	if c.ClusterMax < c.ClusterMin {
		c.ClusterMin, c.ClusterMax = c.ClusterMax, c.ClusterMin
	}
	if c.ClusterMax > 24 {
		c.ClusterMax = 24
	}
	if c.ReleaseWindow <= 0 {
		c.ReleaseWindow = 22
	}
	if c.WindDur <= 0 {
		c.WindDur = 260
	}
	if c.WindStrength <= 0 {
		c.WindStrength = 0.09
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 460
	}
	return c
}

// PaperLanternsSchema describes the paper-lanterns effect's tunable knobs for
// the dev UI.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name: "paper-lanterns",
		Knobs: []Knob{
			{Key: "intro_first", Label: "first delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks before the first lone lantern launches during the intro."},
			{Key: "intro_cluster", Label: "cluster delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 420, Step: 5, Default: 150,
				Description: "Ticks before the first release pulse establishes the festival rhythm."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.22, Max: 0.22, Step: 0.01, Default: 0.035,
				Description: "Baseline horizontal drift. Positive leans right; negative leans left."},
			{Key: "wind_drift", Label: "drift span", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.18, Step: 0.01, Default: 0.05,
				Description: "How far wind-drift events can bend lantern paths sideways."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.06, Max: 0.42, Step: 0.01, Default: 0.18,
				Description: "Vertical rise speed in grid cells per tick. Lower values feel buoyant."},
			{Key: "speed_jit", Label: "speed jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.65, Step: 0.01, Default: 0.25,
				Description: "Per-lantern rise speed variation so clusters do not move like rockets."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.01, Default: 0.22,
				Description: "Gentle side-to-side oscillation applied to each lantern."},
			{Key: "emit_p", Label: "lantern emit", Slot: SlotEvent, Group: "cadence", Type: KnobFloat, Min: 0, Max: 0.04, Step: 0.0005, Default: 0.006, Trigger: "lantern-emit",
				Description: "Per-tick chance of a lone drifter launching between release pulses."},
			{Key: "release_p", Label: "release pulse", Slot: SlotEvent, Group: "cadence", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0008, Trigger: "release-pulse",
				Description: "Per-tick chance of a clustered lantern release."},
			{Key: "wind_p", Label: "wind drift", Slot: SlotEvent, Group: "weather", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.00025, Trigger: "wind-drift",
				Description: "Per-tick chance that the global drift bias shifts sideways."},
			{Key: "quiet_p", Label: "quiet gap", Slot: SlotEvent, Group: "cadence", Type: KnobFloat, Min: 0, Max: 0.006, Step: 0.0002, Default: 0.00012, Trigger: "quiet-gap",
				Description: "Per-tick chance of a long suppression window with no new launches."},
			{Key: "fade_p", Label: "fade cue", Slot: SlotEvent, Group: "cadence", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "lantern-fade",
				Description: "Optional chance of cueing one high lantern to dim early."},
			{Key: "max", Label: "max lanterns", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 8, Max: 160, Step: 1, Default: 72,
				Description: "Maximum active lanterns allowed in the sky."},
			{Key: "size", Label: "size", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 2, Max: 7, Step: 0.1, Default: 3.2,
				Description: "Base lantern body size in low-resolution pixels."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Strength of the warm halo around each paper body."},
			{Key: "fade_alt", Label: "fade altitude", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.05, Max: 0.55, Step: 0.01, Default: 0.28,
				Description: "Top-of-frame band where lanterns begin dimming out."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 20, Max: 70, Step: 1, Default: 34,
				Description: "Lantern paper palette. Lower values glow orange; higher values glow gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Color variation across lanterns within a release."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Overall saturation of paper and flame tones."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.45, Step: 0.01, Default: 0.18,
				Description: "Minimum body lightness used for dim distant lanterns."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.45, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Maximum lightness for fresh lanterns and flame centers."},
			{Key: "cluster_min", Label: "cluster min", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 1, Max: 18, Step: 1, Default: 5,
				Description: "Minimum lanterns emitted by one release pulse."},
			{Key: "cluster_max", Label: "cluster max", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 2, Max: 24, Step: 1, Default: 10,
				Description: "Maximum lanterns emitted by one release pulse."},
			{Key: "release_window", Label: "pulse window", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 1, Max: 90, Step: 1, Default: 22,
				Description: "Ticks over which a cluster release is spread."},
			{Key: "wind_dur", Label: "wind dur", Slot: SlotEventMod, Group: "wind", Type: KnobInt, Min: 30, Max: 720, Step: 10, Default: 260,
				Description: "Duration of a wind-drift bias shift."},
			{Key: "wind_str", Label: "wind strength", Slot: SlotEventMod, Group: "wind", Type: KnobFloat, Min: 0.02, Max: 0.28, Step: 0.01, Default: 0.09,
				Description: "Additional sideways drift applied by a wind-drift event."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet", Type: KnobInt, Min: 60, Max: 1200, Step: 10, Default: 460,
				Description: "Duration of the quiet-gap suppression window."},
			{Key: "ending_stop", Label: "release stop", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 18, Trigger: "ending",
				Description: "Ticks reserved for releases to shut down at the start of the outro."},
			{Key: "tail_len", Label: "tail length", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 30, Max: 1200, Step: 10, Default: 420,
				Description: "Ticks allowed for in-flight lanterns to finish rising before the sky clears."},
		},
	}
}

// PaperLantern is the serializable shape of one in-flight lantern.
type PaperLantern struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	VX        float64 `json:"vx"`
	VY        float64 `json:"vy"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
	HueOffset float64 `json:"hueOffset"`
	Light     float64 `json:"light"`
	Age       int     `json:"age"`
	SwayPhase float64 `json:"swayPhase"`
	SwaySpeed float64 `json:"swaySpeed"`
	SwayAmp   float64 `json:"swayAmp"`
	Layer     int     `json:"layer"`
	FadeLeft  int     `json:"fadeLeft,omitempty"`
	FadeTotal int     `json:"fadeTotal,omitempty"`
}

// PaperLanternsState is the wire/persisted scalar shape of the lantern sim.
type PaperLanternsState struct {
	Tick             int     `json:"tick"`
	IntroTicks       int     `json:"introTicks"`
	IntroTotal       int     `json:"introTotal"`
	IntroFirstDone   bool    `json:"introFirstDone,omitempty"`
	IntroClusterDone bool    `json:"introClusterDone,omitempty"`
	EndingTicks      int     `json:"endingTicks"`
	EndingTotal      int     `json:"endingTotal"`
	EndingStop       int     `json:"endingStop"`
	QuietTicks       int     `json:"quietTicks"`
	PulseTicks       int     `json:"pulseTicks"`
	PulseTotal       int     `json:"pulseTotal"`
	PulseRemaining   int     `json:"pulseRemaining"`
	WindTicks        int     `json:"windTicks"`
	WindBias         float64 `json:"windBias"`
	WindTarget       float64 `json:"windTarget"`
	Ended            bool    `json:"ended,omitempty"`
}

type PaperLanternsSnapshot struct {
	PaperLanternsState
	RNGState uint64         `json:"rngState,omitempty"`
	Lanterns []PaperLantern `json:"lanterns"`
}

type PaperLanternsPersistedState struct {
	PaperLanternsState
	RNGState uint64         `json:"rngState"`
	Lanterns []PaperLantern `json:"lanterns"`
}

// PaperLanterns is the authoritative server-side paper-lantern release sim.
type PaperLanterns struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	introTicks       int
	introTotal       int
	introFirstDone   bool
	introClusterDone bool
	endingTicks      int
	endingTotal      int
	endingStop       int
	quietTicks       int
	pulseTicks       int
	pulseTotal       int
	pulseRemaining   int
	windTicks        int
	windBias         float64
	windTarget       float64
	ended            bool

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	p := &PaperLanterns{
		W:          w,
		H:          h,
		Grid:       grid,
		rng:        rngutil.New(seed),
		cfg:        cfg.withDefaults(),
		windTarget: cfg.withDefaults().Wind,
	}
	p.windBias = p.cfg.Wind
	p.seedInitialLanternsLocked()
	p.paintFrameLocked()
	return p
}

func (p *PaperLanterns) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if w == p.W && h == p.H {
		return
	}
	p.W = w
	p.H = h
	p.Grid = make([][]Pixel, h)
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, w)
	}
	p.paintFrameLocked()
}

func (p *PaperLanterns) SetConfig(cfg PaperLanternsConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg.withDefaults()
	if p.windTicks <= 0 {
		p.windTarget = p.cfg.Wind
	}
	if len(p.lanterns) > p.cfg.MaxLanterns {
		p.lanterns = append([]paperLantern(nil), p.lanterns[len(p.lanterns)-p.cfg.MaxLanterns:]...)
	}
	p.paintFrameLocked()
}

func (p *PaperLanterns) EffectiveConfig() PaperLanternsConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *PaperLanterns) CurrentTick() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tick
}

func (p *PaperLanterns) PerturbRNG(delta int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rng.Mix(delta)
}

func (p *PaperLanterns) DrainLog() []LogEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.log) == 0 {
		return nil
	}
	out := p.log
	p.log = nil
	return out
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *PaperLanterns) Snapshot() PaperLanternsSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PaperLanternsSnapshot{
		PaperLanternsState: p.snapshotStateLocked(),
		RNGState:           p.rng.State(),
		Lanterns:           p.copyLanternsLocked(),
	}
}

func (p *PaperLanterns) RestoreSnapshot(s PaperLanternsSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.PaperLanternsState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreLanternsLocked(s.Lanterns)
	p.paintFrameLocked()
}

func (p *PaperLanterns) SnapshotPersistedState() PaperLanternsPersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PaperLanternsPersistedState{
		PaperLanternsState: p.snapshotStateLocked(),
		RNGState:           p.rng.State(),
		Lanterns:           p.copyLanternsLocked(),
	}
}

func (p *PaperLanterns) RestorePersistedState(s PaperLanternsPersistedState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.PaperLanternsState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreLanternsLocked(s.Lanterns)
	p.paintFrameLocked()
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	return PaperLanternsState{
		Tick:             p.tick,
		IntroTicks:       p.introTicks,
		IntroTotal:       p.introTotal,
		IntroFirstDone:   p.introFirstDone,
		IntroClusterDone: p.introClusterDone,
		EndingTicks:      p.endingTicks,
		EndingTotal:      p.endingTotal,
		EndingStop:       p.endingStop,
		QuietTicks:       p.quietTicks,
		PulseTicks:       p.pulseTicks,
		PulseTotal:       p.pulseTotal,
		PulseRemaining:   p.pulseRemaining,
		WindTicks:        p.windTicks,
		WindBias:         p.windBias,
		WindTarget:       p.windTarget,
		Ended:            p.ended,
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.introFirstDone = s.IntroFirstDone
	p.introClusterDone = s.IntroClusterDone
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.endingStop = s.EndingStop
	p.quietTicks = s.QuietTicks
	p.pulseTicks = s.PulseTicks
	p.pulseTotal = s.PulseTotal
	p.pulseRemaining = s.PulseRemaining
	p.windTicks = s.WindTicks
	p.windBias = s.WindBias
	p.windTarget = s.WindTarget
	if p.windTarget == 0 && p.windBias == 0 {
		p.windTarget = p.cfg.Wind
		p.windBias = p.cfg.Wind
	}
	p.ended = s.Ended
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			X:         l.X,
			Y:         l.Y,
			VX:        l.VX,
			VY:        l.VY,
			W:         l.W,
			H:         l.H,
			HueOffset: l.HueOffset,
			Light:     l.Light,
			Age:       l.Age,
			SwayPhase: l.SwayPhase,
			SwaySpeed: l.SwaySpeed,
			SwayAmp:   l.SwayAmp,
			Layer:     l.Layer,
			FadeLeft:  l.FadeLeft,
			FadeTotal: l.FadeTotal,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			X:         l.X,
			Y:         l.Y,
			VX:        l.VX,
			VY:        l.VY,
			W:         l.W,
			H:         l.H,
			HueOffset: l.HueOffset,
			Light:     l.Light,
			Age:       l.Age,
			SwayPhase: l.SwayPhase,
			SwaySpeed: l.SwaySpeed,
			SwayAmp:   l.SwayAmp,
			Layer:     l.Layer,
			FadeLeft:  l.FadeLeft,
			FadeTotal: l.FadeTotal,
		}
	}
}

func (p *PaperLanterns) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "lantern-emit":
		if p.canEmitLocked() {
			p.spawnLanternLocked(false)
			p.appendLog("lantern-emit", "single lantern launched")
		} else {
			p.appendLog("lantern-emit", "suppressed")
		}
	case "release-pulse":
		if p.canEmitLocked() {
			p.startReleasePulseLocked("triggered")
		} else {
			p.appendLog("release-pulse", "suppressed")
		}
	case "wind-drift":
		p.startWindDriftLocked("triggered")
	case "lantern-fade":
		p.fadeOneLanternLocked("triggered")
	case "quiet-gap":
		p.startQuietGapLocked("triggered")
	case "intro":
		p.startIntroLocked()
		p.appendLog("intro", fmt.Sprintf("started (first=%d, cluster=%d)", p.cfg.IntroFirstDelay, p.cfg.IntroClusterDelay))
	case "ending":
		p.startEndingLocked()
		p.appendLog("ending", fmt.Sprintf("started (stop=%d, tail=%d)", p.endingStop, p.endingTotal-p.endingStop))
	default:
		return false
	}
	p.paintFrameLocked()
	return true
}

func (p *PaperLanterns) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	if p.windTicks > 0 {
		p.windTicks--
	} else {
		p.windTarget = p.cfg.Wind
	}
	p.windBias += (p.windTarget - p.windBias) * 0.018
	if p.quietTicks > 0 {
		p.quietTicks--
	}

	p.stepIntroLocked()
	if p.endingTicks <= 0 && !p.ended && p.introTicks <= 0 {
		p.maybeAutomaticEventsLocked()
	}
	p.stepPulseLocked()
	p.stepLanternsLocked()

	if p.endingTicks > 0 {
		p.endingTicks--
		if p.endingTicks <= 0 {
			p.lanterns = nil
			p.pulseTicks = 0
			p.pulseRemaining = 0
			p.ended = true
			p.appendLog("ending", "sky returned to dark")
		}
	}

	p.paintFrameLocked()
}

func (p *PaperLanterns) canEmitLocked() bool {
	return !p.ended && p.endingTicks <= 0 && len(p.lanterns) < p.cfg.MaxLanterns
}

func (p *PaperLanterns) maybeAutomaticEventsLocked() {
	if p.quietTicks > 0 {
		return
	}
	if p.cfg.SpawnChance > 0 && p.rng.Float64() < p.cfg.SpawnChance {
		p.spawnLanternLocked(false)
	}
	if p.pulseTicks <= 0 && p.cfg.ReleaseChance > 0 && p.rng.Float64() < p.cfg.ReleaseChance {
		p.startReleasePulseLocked("started")
	}
	if p.cfg.WindChance > 0 && p.rng.Float64() < p.cfg.WindChance {
		p.startWindDriftLocked("started")
	}
	if p.cfg.QuietChance > 0 && p.rng.Float64() < p.cfg.QuietChance {
		p.startQuietGapLocked("started")
	}
	if p.cfg.FadeChance > 0 && p.rng.Float64() < p.cfg.FadeChance {
		p.fadeOneLanternLocked("started")
	}
}

func (p *PaperLanterns) stepIntroLocked() {
	if p.introTicks <= 0 {
		return
	}
	elapsed := p.introTotal - p.introTicks
	if !p.introFirstDone && elapsed >= p.cfg.IntroFirstDelay {
		p.spawnLanternLocked(false)
		p.introFirstDone = true
		p.appendLog("lantern-emit", "intro first lantern")
	}
	if !p.introClusterDone && elapsed >= p.cfg.IntroClusterDelay {
		p.startReleasePulseLocked("intro")
		p.introClusterDone = true
	}
	p.introTicks--
	if p.introTicks <= 0 {
		p.introTicks = 0
		p.introTotal = 0
		p.introFirstDone = false
		p.introClusterDone = false
	}
}

func (p *PaperLanterns) stepPulseLocked() {
	if p.pulseTicks <= 0 || p.pulseRemaining <= 0 {
		p.pulseTicks = 0
		p.pulseRemaining = 0
		p.pulseTotal = 0
		return
	}
	if !p.canEmitLocked() {
		p.pulseTicks = 0
		p.pulseRemaining = 0
		return
	}
	perTick := int(math.Ceil(float64(p.pulseRemaining) / float64(max(1, p.pulseTicks))))
	if perTick < 1 {
		perTick = 1
	}
	for i := 0; i < perTick && p.pulseRemaining > 0 && len(p.lanterns) < p.cfg.MaxLanterns; i++ {
		p.spawnLanternLocked(true)
		p.pulseRemaining--
	}
	p.pulseTicks--
	if p.pulseTicks <= 0 && p.pulseRemaining > 0 {
		for p.pulseRemaining > 0 && len(p.lanterns) < p.cfg.MaxLanterns {
			p.spawnLanternLocked(true)
			p.pulseRemaining--
		}
	}
}

func (p *PaperLanterns) stepLanternsLocked() {
	if len(p.lanterns) == 0 {
		return
	}
	alive := p.lanterns[:0]
	for _, l := range p.lanterns {
		l.Age++
		layerScale := 1.0
		if l.Layer > 0 {
			layerScale = 0.72
		}
		targetVX := p.windBias*layerScale + math.Sin(l.SwayPhase+float64(l.Age)*l.SwaySpeed)*p.cfg.Sway*0.018*layerScale
		l.VX += (targetVX - l.VX) * 0.045
		l.X += l.VX + math.Sin(l.SwayPhase+float64(l.Age)*l.SwaySpeed*0.7)*l.SwayAmp*0.018
		l.Y -= l.VY
		if l.FadeLeft > 0 {
			l.FadeLeft--
			if l.FadeLeft <= 0 {
				p.appendLog("lantern-fade", "lantern dimmed")
				continue
			}
		}
		if l.Y < -l.H-2 {
			p.appendLog("lantern-fade", "lantern reached top")
			continue
		}
		if l.X < -24 || l.X > float64(p.W)+24 {
			p.appendLog("lantern-fade", "lantern drifted out")
			continue
		}
		alive = append(alive, l)
	}
	p.lanterns = alive
}

func (p *PaperLanterns) startIntroLocked() {
	p.lanterns = nil
	p.ended = false
	p.endingTicks = 0
	p.endingTotal = 0
	p.endingStop = 0
	p.quietTicks = 0
	p.pulseTicks = 0
	p.pulseTotal = 0
	p.pulseRemaining = 0
	p.introFirstDone = false
	p.introClusterDone = false
	p.introTotal = max(p.cfg.IntroFirstDelay, p.cfg.IntroClusterDelay) + max(1, p.cfg.ReleaseWindow)
	p.introTicks = p.introTotal
}

func (p *PaperLanterns) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.introFirstDone = false
	p.introClusterDone = false
	p.quietTicks = 0
	p.pulseTicks = 0
	p.pulseTotal = 0
	p.pulseRemaining = 0
	p.endingStop = max(0, p.cfg.EndingStop)
	p.endingTotal = p.endingStop + max(1, p.cfg.TailLength)
	p.endingTicks = p.endingTotal
	p.ended = false
}

func (p *PaperLanterns) startReleasePulseLocked(verb string) {
	count := p.cfg.ClusterMin
	if p.cfg.ClusterMax > p.cfg.ClusterMin {
		count += p.rng.Intn(p.cfg.ClusterMax - p.cfg.ClusterMin + 1)
	}
	p.pulseTicks = max(1, jitterInt(p.rng, p.cfg.ReleaseWindow, 0.2))
	p.pulseTotal = count
	p.pulseRemaining = count
	p.appendLog("release-pulse", fmt.Sprintf("%s (count=%d, window=%d)", verb, count, p.pulseTicks))
}

func (p *PaperLanterns) startWindDriftLocked(verb string) {
	dir := 1.0
	if p.rng.Float64() < 0.5 {
		dir = -1
	}
	strength := p.cfg.WindStrength
	if strength <= 0 {
		strength = p.cfg.WindDrift
	}
	p.windTicks = jitterInt(p.rng, p.cfg.WindDur, 0.35)
	p.windTarget = p.cfg.Wind + dir*strength*(0.65+p.rng.Float64()*0.7)
	p.appendLog("wind-drift", fmt.Sprintf("%s (dur=%d, target=%+.2f)", verb, p.windTicks, p.windTarget))
}

func (p *PaperLanterns) startQuietGapLocked(verb string) {
	p.quietTicks = jitterInt(p.rng, p.cfg.QuietDur, 0.25)
	p.pulseTicks = 0
	p.pulseRemaining = 0
	p.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d)", verb, p.quietTicks))
}

func (p *PaperLanterns) fadeOneLanternLocked(verb string) {
	if len(p.lanterns) == 0 {
		p.appendLog("lantern-fade", verb+" (none)")
		return
	}
	target := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Y < p.lanterns[target].Y {
			target = i
		}
	}
	dur := jitterInt(p.rng, 54, 0.25)
	p.lanterns[target].FadeLeft = dur
	p.lanterns[target].FadeTotal = dur
	p.appendLog("lantern-fade", fmt.Sprintf("%s lantern %d (dur=%d)", verb, target+1, dur))
}

func (p *PaperLanterns) seedInitialLanternsLocked() {
	if p.W <= 0 || p.H <= 0 || p.cfg.MaxLanterns <= 0 {
		return
	}
	n := max(3, min(12, p.W/34))
	if n > p.cfg.MaxLanterns {
		n = p.cfg.MaxLanterns
	}
	for i := 0; i < n; i++ {
		p.spawnLanternAtLocked(p.rng.Float64()*float64(max(1, p.W-1)), float64(p.H)*(0.22+p.rng.Float64()*0.88), false)
	}
}

func (p *PaperLanterns) spawnLanternLocked(cluster bool) {
	if p.W <= 0 || p.H <= 0 || len(p.lanterns) >= p.cfg.MaxLanterns {
		return
	}
	x := p.rng.Float64() * float64(max(1, p.W-1))
	if cluster && len(p.lanterns) > 0 && p.rng.Float64() < 0.45 {
		x = p.lanterns[len(p.lanterns)-1].X + (p.rng.Float64()*2-1)*float64(p.W)*0.07
	}
	y := float64(p.H) + 2 + p.rng.Float64()*4
	p.spawnLanternAtLocked(x, y, cluster)
}

func (p *PaperLanterns) spawnLanternAtLocked(x, y float64, cluster bool) {
	if len(p.lanterns) >= p.cfg.MaxLanterns {
		return
	}
	layer := 0
	if p.rng.Float64() < 0.34 {
		layer = 1
	}
	layerScale := 1.0
	if layer > 0 {
		layerScale = 0.78
	}
	size := p.cfg.LanternSize * (0.82 + p.rng.Float64()*0.38) * layerScale
	if size < 1.5 {
		size = 1.5
	}
	w := math.Max(2, math.Round(size))
	h := math.Max(3, math.Round(size*1.45))
	rise := p.cfg.RiseSpeed * (1 + (p.rng.Float64()*2-1)*p.cfg.SpeedJitter) * layerScale
	if rise < 0.025 {
		rise = 0.025
	}
	drift := p.cfg.Wind*(0.55+p.rng.Float64()*0.5)*layerScale + (p.rng.Float64()*2-1)*0.018
	if cluster {
		drift += (p.rng.Float64()*2 - 1) * 0.012
	}
	p.lanterns = append(p.lanterns, paperLantern{
		X:         x,
		Y:         y,
		VX:        drift,
		VY:        rise,
		W:         w,
		H:         h,
		HueOffset: (p.rng.Float64()*2 - 1) * p.cfg.HueSpread,
		Light:     0.58 + p.rng.Float64()*0.42,
		SwayPhase: p.rng.Float64() * 2 * math.Pi,
		SwaySpeed: 0.025 + p.rng.Float64()*0.035,
		SwayAmp:   0.35 + p.rng.Float64()*0.8,
		Layer:     layer,
	})
}

func (p *PaperLanterns) paintFrameLocked() {
	p.clearGridLocked()
	p.paintSkyLocked()
	for _, l := range p.lanterns {
		p.paintLanternLocked(l)
	}
}

func (p *PaperLanterns) clearGridLocked() {
	for y := range p.Grid {
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{}
		}
	}
}

func (p *PaperLanterns) paintSkyLocked() {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	top := hslToRGB(232, 0.34, 0.025)
	bottom := hslToRGB(222, 0.26, 0.055)
	for y := 0; y < p.H; y++ {
		t := 0.0
		if p.H > 1 {
			t = float64(y) / float64(p.H-1)
		}
		c := mixRGBA(top, bottom, t)
		for x := 0; x < p.W; x++ {
			p.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (p *PaperLanterns) paintLanternLocked(l paperLantern) {
	vis := p.lanternVisibility(l)
	if vis <= 0 {
		return
	}
	cx := int(math.Round(l.X))
	top := int(math.Round(l.Y))
	bodyW := max(2, int(math.Round(l.W)))
	bodyH := max(3, int(math.Round(l.H)))
	left := cx - bodyW/2
	hue := math.Mod(p.cfg.Hue+l.HueOffset+360, 360)
	bodyLight := p.cfg.LightnessMin + l.Light*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	if l.Layer > 0 {
		bodyLight *= 0.82
	}
	body := hslToRGB(hue, p.cfg.Saturation, clamp01(bodyLight))
	edge := hslToRGB(hue-5, p.cfg.Saturation*0.72, clamp01(bodyLight*0.62))
	flame := hslToRGB(hue+18, 0.96, clamp01(p.cfg.LightnessMax))
	core := hslToRGB(52, 0.98, 0.94)

	radius := int(math.Round(float64(max(bodyW, bodyH))*1.35 + 2))
	for yy := top - radius/2; yy <= top+bodyH+radius/2; yy++ {
		for xx := left - radius; xx <= left+bodyW+radius; xx++ {
			dx := (float64(xx) + 0.5 - l.X) / math.Max(1, float64(radius))
			dy := (float64(yy) + 0.5 - (float64(top) + float64(bodyH)*0.55)) / math.Max(1, float64(radius))
			dist := math.Sqrt(dx*dx + dy*dy*1.4)
			if dist > 1 {
				continue
			}
			alpha := (1 - dist) * p.cfg.Glow * vis * 0.28
			glow := hslToRGB(hue, p.cfg.Saturation*0.88, clamp01(bodyLight*0.65))
			p.blendPixel(xx, yy, glow, alpha)
		}
	}

	for yy := 0; yy < bodyH; yy++ {
		for xx := 0; xx < bodyW; xx++ {
			alpha := vis * 0.88
			c := body
			if yy == 0 || yy == bodyH-1 || xx == 0 || xx == bodyW-1 {
				c = edge
				alpha *= 0.92
			}
			if yy > bodyH/2 && xx > 0 && xx < bodyW-1 {
				alpha *= 1.05
			}
			p.blendPixel(left+xx, top+yy, c, alpha)
		}
	}

	fx := left + bodyW/2
	fy := top + max(1, bodyH*2/3)
	p.blendPixel(fx, fy, flame, vis)
	if bodyW >= 4 {
		p.blendPixel(fx-1, fy, flame, vis*0.55)
	}
	if bodyH >= 5 {
		p.blendPixel(fx, fy-1, core, vis*0.72)
	}
}

func (p *PaperLanterns) lanternVisibility(l paperLantern) float64 {
	vis := 1.0
	if l.Age < 24 {
		vis *= clamp01(float64(l.Age+1) / 24)
	}
	fadeBand := math.Max(1, float64(p.H)*p.cfg.FadeAltitude)
	if l.Y < fadeBand {
		vis *= clamp01(l.Y / fadeBand)
	}
	if l.FadeTotal > 0 {
		vis *= clamp01(float64(l.FadeLeft) / float64(l.FadeTotal))
	}
	if l.Layer > 0 {
		vis *= 0.78
	}
	return clamp01(vis)
}

func (p *PaperLanterns) blendPixel(x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= p.H || x < 0 || x >= p.W {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	if alpha >= 1 || !p.Grid[y][x].Filled {
		p.Grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}}
		return
	}
	prev := p.Grid[y][x].C
	p.Grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(prev.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(prev.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(prev.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func mixRGBA(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: 255,
	}
}
