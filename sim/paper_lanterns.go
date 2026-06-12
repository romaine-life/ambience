package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type paperLantern struct {
	Row, Col    float64
	VRow, VCol  float64
	Body        color.RGBA
	Flame       color.RGBA
	Phase       float64
	FlickerRate float64
	Width       int
	Height      int
	Fading      bool
	Age         int
}

// PaperLanternsConfig tunes a slow sky-lantern release field.
type PaperLanternsConfig struct {
	// LIFECYCLE
	IntroFirstDelay   int `json:"intro_first_delay"`
	IntroClusterDelay int `json:"intro_cluster_delay"`
	EndingStopTicks   int `json:"ending_stop"`
	EndingTailTicks   int `json:"ending_tail"`
	// SPAWN
	EmitEvery    int `json:"emit_every"`
	MaxLanterns  int `json:"max"`
	PulseMin     int `json:"pulse_min"`
	PulseMax     int `json:"pulse_max"`
	PulseWindow  int `json:"pulse_window"`
	LanternWidth int `json:"lantern_w"`
	// MOTION
	RiseSpeed   float64 `json:"rise"`
	SpeedJitter float64 `json:"speed_jit"`
	Wind        float64 `json:"wind"`
	WindJitter  float64 `json:"wind_jit"`
	Wander      float64 `json:"wander"`
	Sway        float64 `json:"sway"`
	// END
	FadeAltitude float64 `json:"fade_alt"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	Glow         float64 `json:"glow"`
	// EVENT CHANCES
	ReleasePulseChance float64 `json:"release_pulse_p"`
	WindDriftChance    float64 `json:"wind_drift_p"`
	QuietGapChance     float64 `json:"quiet_gap_p"`
	// EVENT MODIFIERS
	WindDriftDur      int     `json:"wind_drift_dur"`
	WindDriftStrength float64 `json:"wind_drift_str"`
	QuietGapDur       int     `json:"quiet_gap_dur"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroFirstDelay <= 0 {
		c.IntroFirstDelay = 24
	}
	if c.IntroClusterDelay <= 0 {
		c.IntroClusterDelay = 96
	}
	if c.EndingStopTicks < 0 {
		c.EndingStopTicks = 0
	}
	if c.EndingTailTicks <= 0 {
		c.EndingTailTicks = 420
	}
	if c.EmitEvery <= 0 {
		c.EmitEvery = 50
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 56
	}
	if c.PulseMin <= 0 {
		c.PulseMin = 5
	}
	if c.PulseMax <= 0 {
		c.PulseMax = 9
	}
	if c.PulseMax < c.PulseMin {
		c.PulseMin, c.PulseMax = c.PulseMax, c.PulseMin
	}
	if c.PulseWindow <= 0 {
		c.PulseWindow = 20
	}
	if c.LanternWidth <= 0 {
		c.LanternWidth = 3
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.13
	}
	if c.SpeedJitter < 0 {
		c.SpeedJitter = 0
	}
	if c.SpeedJitter == 0 {
		c.SpeedJitter = 0.28
	}
	if c.WindJitter < 0 {
		c.WindJitter = 0
	}
	if c.WindJitter == 0 {
		c.WindJitter = 0.035
	}
	if c.Wander <= 0 {
		c.Wander = 0.55
	}
	if c.Sway <= 0 {
		c.Sway = 0.8
	}
	if c.FadeAltitude <= 0 {
		c.FadeAltitude = 0.28
	}
	c.FadeAltitude = clamp01(c.FadeAltitude)
	if c.Hue == 0 {
		c.Hue = 36
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.HueSpread == 0 {
		c.HueSpread = 14
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.74
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.42
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.82
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.Glow <= 0 {
		c.Glow = 0.42
	}
	if c.WindDriftDur <= 0 {
		c.WindDriftDur = 120
	}
	if c.WindDriftStrength <= 0 {
		c.WindDriftStrength = 0.16
	}
	if c.QuietGapDur <= 0 {
		c.QuietGapDur = 260
	}
	return c
}

// PaperLanternsSchema describes the Paper Lanterns effect's dev controls.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name:           "paper-lanterns",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_first_delay", Label: "first delay", Slot: SlotSpawn, Group: "intro", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 24, Trigger: "intro",
				Description: "Ticks before the first lone lantern launches during the intro."},
			{Key: "intro_cluster_delay", Label: "cluster delay", Slot: SlotSpawn, Group: "intro", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 96,
				Description: "Ticks from intro start to the first release pulse."},
			{Key: "ending_stop", Label: "stop delay", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 0, Trigger: "ending",
				Description: "Ticks after ending starts before all new releases are suppressed."},
			{Key: "ending_tail", Label: "tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 30, Max: 900, Step: 10, Default: 420,
				Description: "Maximum ticks for in-flight lanterns to rise out before the sky holds dark."},
			{Key: "emit_every", Label: "lone emit 1/", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 8, Max: 180, Step: 1, Default: 50, Trigger: "lantern-emit",
				Description: "One-in-N chance per tick for a lone lantern between clusters."},
			{Key: "max", Label: "max lanterns", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 4, Max: 140, Step: 1, Default: 56,
				Description: "Maximum number of in-flight lanterns."},
			{Key: "pulse_min", Label: "pulse min", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 2, Max: 16, Step: 1, Default: 5,
				Description: "Minimum lanterns launched by a release pulse."},
			{Key: "pulse_max", Label: "pulse max", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 3, Max: 22, Step: 1, Default: 9,
				Description: "Maximum lanterns launched by a release pulse."},
			{Key: "pulse_window", Label: "pulse window", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 1, Max: 80, Step: 1, Default: 20,
				Description: "Ticks over which a pulse cluster is released."},
			{Key: "lantern_w", Label: "lantern width", Slot: SlotSpawn, Group: "shape", Type: KnobInt, Min: 2, Max: 5, Step: 1, Default: 3,
				Description: "Pixel width of each lantern body; height is one pixel taller."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.04, Max: 0.32, Step: 0.01, Default: 0.13,
				Description: "Base upward speed. Lower values feel buoyant rather than rocket-like."},
			{Key: "speed_jit", Label: "rise jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.7, Step: 0.01, Default: 0.28,
				Description: "Per-lantern variation in upward speed."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.35, Max: 0.35, Step: 0.01, Default: 0,
				Description: "Global sideways drift bias."},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.18, Step: 0.005, Default: 0.035,
				Description: "Per-lantern wind variation at launch."},
			{Key: "wander", Label: "wander", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.05, Max: 1.5, Step: 0.05, Default: 0.55,
				Description: "Small per-tick drift wobble."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 2, Step: 0.05, Default: 0.8,
				Description: "Sinusoidal side-to-side lantern buoyancy."},
			{Key: "fade_alt", Label: "fade altitude", Slot: SlotEnd, Group: "fade", Type: KnobFloat, Min: 0.08, Max: 0.6, Step: 0.01, Default: 0.28, Trigger: "lantern-fade",
				Description: "Top fraction of the scene where lanterns dim before disappearing."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 18, Max: 58, Step: 1, Default: 36,
				Description: "Base paper-lantern palette hue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0, Max: 34, Step: 1, Default: 14,
				Description: "Variation across lantern paper colors."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.74,
				Description: "Warm color saturation for the paper glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.15, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Minimum lantern paper lightness."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.35, Max: 1, Step: 0.01, Default: 0.82,
				Description: "Maximum lantern paper lightness."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.42,
				Description: "Faint halo strength around each lantern."},
			{Key: "release_pulse_p", Label: "release pulse", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.003, Trigger: "release-pulse",
				Description: "Per-tick chance of a 5-10 lantern cluster release."},
			{Key: "wind_drift_p", Label: "wind drift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0008, Trigger: "wind-drift",
				Description: "Per-tick chance that the global drift bias shifts."},
			{Key: "quiet_gap_p", Label: "quiet gap", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0007, Trigger: "quiet-gap",
				Description: "Per-tick chance of suppressing cluster releases for a while."},
			{Key: "wind_drift_dur", Label: "wind dur", Slot: SlotEventMod, Group: "wind-drift", Type: KnobInt, Min: 20, Max: 360, Step: 5, Default: 120,
				Description: "How long a shifted wind bias is held."},
			{Key: "wind_drift_str", Label: "wind str", Slot: SlotEventMod, Group: "wind-drift", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Maximum extra sideways wind from a drift event."},
			{Key: "quiet_gap_dur", Label: "gap dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 40, Max: 900, Step: 10, Default: 260,
				Description: "How long release pulses are suppressed during a quiet gap."},
		},
	}
}

type PaperLanternsState struct {
	Tick                int       `json:"tick"`
	Lifecycle           Lifecycle `json:"lifecycle"`
	IntroTicks          int       `json:"introTicks,omitempty"`
	IntroFirstTicks     int       `json:"introFirstTicks,omitempty"`
	IntroClusterTicks   int       `json:"introClusterTicks,omitempty"`
	IntroFirstEmitted   bool      `json:"introFirstEmitted,omitempty"`
	IntroClusterEmitted bool      `json:"introClusterEmitted,omitempty"`
	EndingStopTicks     int       `json:"endingStopTicks,omitempty"`
	EndingTicks         int       `json:"endingTicks,omitempty"`
	PulseQueue          int       `json:"pulseQueue,omitempty"`
	PulseTicks          int       `json:"pulseTicks,omitempty"`
	PulseCenter         float64   `json:"pulseCenter,omitempty"`
	WindBias            float64   `json:"windBias,omitempty"`
	WindDriftTicks      int       `json:"windDriftTicks,omitempty"`
	QuietGapTicks       int       `json:"quietGapTicks,omitempty"`
	Dark                bool      `json:"dark"`
}

type PaperLantern struct {
	Row         float64 `json:"row"`
	Col         float64 `json:"col"`
	VRow        float64 `json:"vRow"`
	VCol        float64 `json:"vCol"`
	Body        RGB     `json:"body"`
	Flame       RGB     `json:"flame"`
	Phase       float64 `json:"phase"`
	FlickerRate float64 `json:"flickerRate"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Fading      bool    `json:"fading"`
	Age         int     `json:"age"`
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

// PaperLanterns simulates glowing paper lanterns rising through a dark sky.
type PaperLanterns struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	lifecycle Lifecycle

	introTicks          int
	introFirstTicks     int
	introClusterTicks   int
	introFirstEmitted   bool
	introClusterEmitted bool
	endingStopTicks     int
	endingTicks         int

	pulseQueue  int
	pulseTicks  int
	pulseCenter float64

	windBias       float64
	windDriftTicks int
	quietGapTicks  int

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for y := range grid {
		grid[y] = make([]Pixel, w)
	}
	return &PaperLanterns{
		W:         w,
		H:         h,
		Grid:      grid,
		rng:       rngutil.New(seed),
		cfg:       cfg.withDefaults(),
		lifecycle: LifecycleRunning,
	}
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
	for y := range p.Grid {
		p.Grid[y] = make([]Pixel, w)
	}
}

func (p *PaperLanterns) SetConfig(cfg PaperLanternsConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := cfg.withDefaults()
	if p.cfg.RiseSpeed > 0 && next.RiseSpeed != p.cfg.RiseSpeed {
		ratio := next.RiseSpeed / p.cfg.RiseSpeed
		for i := range p.lanterns {
			p.lanterns[i].VRow *= ratio
		}
	}
	p.cfg = next
}

func (p *PaperLanterns) EffectiveConfig() PaperLanternsConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
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

func (p *PaperLanterns) RestoreSnapshot(snap PaperLanternsSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(snap.PaperLanternsState)
	if snap.RNGState != 0 {
		p.rng.SetState(snap.RNGState)
	}
	p.restoreLanternsLocked(snap.Lanterns)
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

func (p *PaperLanterns) RestorePersistedState(ps PaperLanternsPersistedState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(ps.PaperLanternsState)
	if ps.RNGState != 0 {
		p.rng.SetState(ps.RNGState)
	}
	p.restoreLanternsLocked(ps.Lanterns)
	p.paintFrameLocked()
}

func (p *PaperLanterns) LanternsCopy() []PaperLantern {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyLanternsLocked()
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

func (p *PaperLanterns) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "lantern-emit":
		p.spawnLanternLocked(p.rng.Float64()*float64(max(1, p.W)), false)
		p.appendLog("lantern-emit", "triggered")
	case "release-pulse":
		p.startReleasePulseLocked("triggered")
	case "wind-drift":
		p.startWindDriftLocked("triggered")
	case "lantern-fade":
		p.forceLanternFadeLocked()
	case "quiet-gap":
		p.startQuietGapLocked("triggered")
	case "intro":
		p.startIntroLocked()
		p.appendLog("intro", fmt.Sprintf("started (first=%d, cluster=%d)", p.introFirstTicks, p.introClusterTicks))
	case "ending":
		p.startEndingLocked()
		p.appendLog("ending", fmt.Sprintf("started (stop=%d, tail=%d)", p.endingStopTicks, p.endingTicks))
	default:
		return false
	}
	p.paintFrameLocked()
	return true
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

func (p *PaperLanterns) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	p.stepTimersLocked()
	p.stepLifecycleLocked()
	p.stepEventsLocked()
	p.stepPulseLocked()
	p.stepLanternsLocked()
	p.paintFrameLocked()
}

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	life := p.lifecycle
	if life == "" {
		life = LifecycleRunning
	}
	return PaperLanternsState{
		Tick:                p.tick,
		Lifecycle:           life,
		IntroTicks:          p.introTicks,
		IntroFirstTicks:     p.introFirstTicks,
		IntroClusterTicks:   p.introClusterTicks,
		IntroFirstEmitted:   p.introFirstEmitted,
		IntroClusterEmitted: p.introClusterEmitted,
		EndingStopTicks:     p.endingStopTicks,
		EndingTicks:         p.endingTicks,
		PulseQueue:          p.pulseQueue,
		PulseTicks:          p.pulseTicks,
		PulseCenter:         p.pulseCenter,
		WindBias:            p.windBias,
		WindDriftTicks:      p.windDriftTicks,
		QuietGapTicks:       p.quietGapTicks,
		Dark:                life == LifecycleEnded && len(p.lanterns) == 0,
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.lifecycle = s.Lifecycle
	if p.lifecycle == "" {
		p.lifecycle = LifecycleRunning
	}
	p.introTicks = s.IntroTicks
	p.introFirstTicks = s.IntroFirstTicks
	p.introClusterTicks = s.IntroClusterTicks
	p.introFirstEmitted = s.IntroFirstEmitted
	p.introClusterEmitted = s.IntroClusterEmitted
	p.endingStopTicks = s.EndingStopTicks
	p.endingTicks = s.EndingTicks
	p.pulseQueue = s.PulseQueue
	p.pulseTicks = s.PulseTicks
	p.pulseCenter = s.PulseCenter
	p.windBias = s.WindBias
	p.windDriftTicks = s.WindDriftTicks
	p.quietGapTicks = s.QuietGapTicks
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			Row:         l.Row,
			Col:         l.Col,
			VRow:        l.VRow,
			VCol:        l.VCol,
			Body:        RGB{R: l.Body.R, G: l.Body.G, B: l.Body.B},
			Flame:       RGB{R: l.Flame.R, G: l.Flame.G, B: l.Flame.B},
			Phase:       l.Phase,
			FlickerRate: l.FlickerRate,
			Width:       l.Width,
			Height:      l.Height,
			Fading:      l.Fading,
			Age:         l.Age,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			Row:         l.Row,
			Col:         l.Col,
			VRow:        l.VRow,
			VCol:        l.VCol,
			Body:        color.RGBA{R: l.Body.R, G: l.Body.G, B: l.Body.B, A: 255},
			Flame:       color.RGBA{R: l.Flame.R, G: l.Flame.G, B: l.Flame.B, A: 255},
			Phase:       l.Phase,
			FlickerRate: l.FlickerRate,
			Width:       l.Width,
			Height:      l.Height,
			Fading:      l.Fading,
			Age:         l.Age,
		}
	}
}

func (p *PaperLanterns) stepTimersLocked() {
	if p.introTicks > 0 {
		p.introTicks--
	}
	if p.introFirstTicks > 0 {
		p.introFirstTicks--
	}
	if p.introClusterTicks > 0 {
		p.introClusterTicks--
	}
	if p.endingStopTicks > 0 {
		p.endingStopTicks--
	}
	if p.endingTicks > 0 {
		p.endingTicks--
	}
	if p.windDriftTicks > 0 {
		p.windDriftTicks--
	}
	if p.quietGapTicks > 0 {
		p.quietGapTicks--
	}
	if p.pulseTicks > 0 {
		p.pulseTicks--
	}
	if p.windDriftTicks == 0 && p.windBias != 0 {
		p.windBias *= 0.985
		if math.Abs(p.windBias) < 0.002 {
			p.windBias = 0
		}
	}
}

func (p *PaperLanterns) stepLifecycleLocked() {
	switch p.lifecycle {
	case LifecycleIntro:
		if !p.introFirstEmitted && p.introFirstTicks <= 0 {
			p.spawnLanternLocked(p.rng.Float64()*float64(max(1, p.W)), false)
			p.introFirstEmitted = true
			p.appendLog("lantern-emit", "intro first lantern")
		}
		if !p.introClusterEmitted && p.introClusterTicks <= 0 {
			p.startReleasePulseLocked("intro")
			p.introClusterEmitted = true
		}
		if p.introTicks <= 0 && p.pulseQueue <= 0 {
			p.lifecycle = LifecycleRunning
			p.appendLog("intro", "resolved")
		}
	case LifecycleEnding:
		if p.endingStopTicks <= 0 {
			p.pulseQueue = 0
			p.pulseTicks = 0
		}
		if (p.endingStopTicks <= 0 && len(p.lanterns) == 0) || p.endingTicks <= 0 {
			p.lanterns = nil
			p.lifecycle = LifecycleEnded
			p.endingTicks = 0
			p.endingStopTicks = 0
			p.appendLog("ending", "resolved dark")
		}
	}
}

func (p *PaperLanterns) stepEventsLocked() {
	if p.lifecycle == LifecycleEnded || p.lifecycle == LifecycleIntro {
		return
	}
	releasesStopped := p.lifecycle == LifecycleEnding && p.endingStopTicks <= 0
	if !releasesStopped && len(p.lanterns) < p.cfg.MaxLanterns {
		emitEvery := p.cfg.EmitEvery
		if p.quietGapTicks > 0 {
			emitEvery *= 2
		}
		if emitEvery < 1 {
			emitEvery = 1
		}
		if p.rng.Intn(emitEvery) == 0 {
			p.spawnLanternLocked(p.rng.Float64()*float64(max(1, p.W)), false)
			p.appendLog("lantern-emit", "started")
		}
	}
	if p.lifecycle == LifecycleEnding {
		return
	}
	if p.pulseQueue <= 0 && p.quietGapTicks <= 0 && p.cfg.ReleasePulseChance > 0 && p.rng.Float64() < p.cfg.ReleasePulseChance {
		p.startReleasePulseLocked("started")
	}
	if p.windDriftTicks <= 0 && p.cfg.WindDriftChance > 0 && p.rng.Float64() < p.cfg.WindDriftChance {
		p.startWindDriftLocked("started")
	}
	if p.quietGapTicks <= 0 && p.cfg.QuietGapChance > 0 && p.rng.Float64() < p.cfg.QuietGapChance {
		p.startQuietGapLocked("started")
	}
}

func (p *PaperLanterns) stepPulseLocked() {
	if p.pulseQueue <= 0 || p.lifecycle == LifecycleEnded {
		return
	}
	if p.lifecycle == LifecycleEnding && p.endingStopTicks <= 0 {
		p.pulseQueue = 0
		p.pulseTicks = 0
		return
	}
	remainingTicks := max(1, p.pulseTicks+1)
	launches := int(math.Ceil(float64(p.pulseQueue) / float64(remainingTicks)))
	if launches < 1 {
		launches = 1
	}
	for i := 0; i < launches && p.pulseQueue > 0 && len(p.lanterns) < p.cfg.MaxLanterns; i++ {
		spread := math.Max(4, float64(p.W)*0.12)
		center := p.pulseCenter
		if center == 0 {
			center = p.rng.Float64() * float64(max(1, p.W))
		}
		col := center + (p.rng.Float64()*2-1)*spread
		p.spawnLanternLocked(col, true)
		p.pulseQueue--
	}
	if p.pulseQueue <= 0 {
		p.pulseTicks = 0
	}
}

func (p *PaperLanterns) stepLanternsLocked() {
	dst := p.lanterns[:0]
	for i := range p.lanterns {
		l := p.lanterns[i]
		l.Age++
		l.Phase += l.FlickerRate
		wind := p.cfg.Wind + p.windBias
		l.VCol += (wind-l.VCol)*0.012 + (p.rng.Float64()*2-1)*p.cfg.Wander*0.0016
		l.Col += l.VCol + math.Sin(l.Phase)*p.cfg.Sway*0.012
		l.Row += l.VRow * (0.96 + 0.04*math.Sin(l.Phase*0.7))
		fadeStart := math.Max(1, float64(p.H)*p.cfg.FadeAltitude)
		if l.Row <= fadeStart && !l.Fading {
			l.Fading = true
			p.appendLog("lantern-fade", "started")
		}
		if l.Row < -float64(l.Height+1) || l.Col < -8 || l.Col > float64(p.W+8) {
			p.appendLog("lantern-fade", "completed")
			continue
		}
		dst = append(dst, l)
	}
	p.lanterns = dst
}

func (p *PaperLanterns) startReleasePulseLocked(verb string) {
	count := p.cfg.PulseMin
	if p.cfg.PulseMax > p.cfg.PulseMin {
		count += p.rng.Intn(p.cfg.PulseMax - p.cfg.PulseMin + 1)
	}
	p.pulseQueue += count
	p.pulseTicks = jitterInt(p.rng, p.cfg.PulseWindow, 0.25)
	p.pulseCenter = p.rng.Float64() * float64(max(1, p.W))
	p.appendLog("release-pulse", fmt.Sprintf("%s (count=%d, window=%d)", verb, count, p.pulseTicks))
}

func (p *PaperLanterns) startWindDriftLocked(verb string) {
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.windBias = sign * p.cfg.WindDriftStrength * (0.45 + p.rng.Float64()*0.75)
	p.windDriftTicks = jitterInt(p.rng, p.cfg.WindDriftDur, 0.3)
	p.appendLog("wind-drift", fmt.Sprintf("%s (dur=%d, bias=%+.2f)", verb, p.windDriftTicks, p.windBias))
}

func (p *PaperLanterns) startQuietGapLocked(verb string) {
	p.quietGapTicks = jitterInt(p.rng, p.cfg.QuietGapDur, 0.25)
	p.pulseQueue = 0
	p.pulseTicks = 0
	p.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d)", verb, p.quietGapTicks))
}

func (p *PaperLanterns) startIntroLocked() {
	p.lifecycle = LifecycleIntro
	p.lanterns = nil
	p.pulseQueue = 0
	p.pulseTicks = 0
	p.quietGapTicks = 0
	p.endingTicks = 0
	p.endingStopTicks = 0
	p.introFirstTicks = max(0, p.cfg.IntroFirstDelay)
	p.introClusterTicks = max(p.introFirstTicks+1, p.cfg.IntroClusterDelay)
	p.introTicks = p.introClusterTicks + max(1, p.cfg.PulseWindow) + 30
	p.introFirstEmitted = false
	p.introClusterEmitted = false
}

func (p *PaperLanterns) startEndingLocked() {
	if p.lifecycle == LifecycleEnded {
		return
	}
	p.lifecycle = LifecycleEnding
	p.introTicks = 0
	p.introFirstTicks = 0
	p.introClusterTicks = 0
	p.introFirstEmitted = false
	p.introClusterEmitted = false
	p.quietGapTicks = 0
	p.endingStopTicks = max(0, p.cfg.EndingStopTicks)
	p.endingTicks = max(1, p.cfg.EndingTailTicks)
	if p.endingStopTicks <= 0 {
		p.pulseQueue = 0
		p.pulseTicks = 0
	}
}

func (p *PaperLanterns) forceLanternFadeLocked() {
	if len(p.lanterns) == 0 {
		p.spawnLanternLocked(p.rng.Float64()*float64(max(1, p.W)), false)
	}
	fadeRow := math.Max(1, float64(p.H)*p.cfg.FadeAltitude*0.6)
	for i := range p.lanterns {
		if p.lanterns[i].Row > fadeRow {
			p.lanterns[i].Row = fadeRow
			p.lanterns[i].Fading = true
			p.appendLog("lantern-fade", "triggered")
			return
		}
	}
	p.lanterns[0].Fading = true
	p.appendLog("lantern-fade", "triggered")
}

func (p *PaperLanterns) spawnLanternLocked(col float64, cluster bool) {
	if p.lifecycle == LifecycleEnded || len(p.lanterns) >= p.cfg.MaxLanterns || p.W <= 0 || p.H <= 0 {
		return
	}
	width := max(2, p.cfg.LanternWidth)
	height := width + 1
	speedJitter := 1 + (p.rng.Float64()*2-1)*p.cfg.SpeedJitter
	if speedJitter < 0.35 {
		speedJitter = 0.35
	}
	rise := -p.cfg.RiseSpeed * speedJitter
	wind := p.cfg.Wind + p.windBias + (p.rng.Float64()*2-1)*p.cfg.WindJitter
	if cluster {
		wind *= 0.75
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	body := hslToRGB(hue, clamp01(p.cfg.Saturation), clamp01(light))
	flame := hslToRGB(math.Mod(hue-10+360, 360), clamp01(p.cfg.Saturation*0.9), clamp01(p.cfg.LightnessMax+(1-p.cfg.LightnessMax)*0.65))
	p.lanterns = append(p.lanterns, paperLantern{
		Row:         float64(p.H - 1),
		Col:         col,
		VRow:        rise,
		VCol:        wind,
		Body:        body,
		Flame:       flame,
		Phase:       p.rng.Float64() * 2 * math.Pi,
		FlickerRate: 0.05 + p.rng.Float64()*0.06,
		Width:       width,
		Height:      height,
	})
}

func (p *PaperLanterns) paintFrameLocked() {
	for y := range p.Grid {
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{}
		}
	}
	for _, l := range p.lanterns {
		p.paintLanternLocked(l)
	}
}

func (p *PaperLanterns) paintLanternLocked(l paperLantern) {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	fadeStart := math.Max(1, float64(p.H)*p.cfg.FadeAltitude)
	alpha := 1.0
	if l.Row < fadeStart {
		alpha = clamp01(l.Row / fadeStart)
	}
	if p.lifecycle == LifecycleEnding && p.endingTicks > 0 && p.cfg.EndingTailTicks > 0 {
		alpha = math.Min(alpha, clamp01(float64(p.endingTicks)/float64(p.cfg.EndingTailTicks)*1.6))
	}
	if alpha <= 0 {
		return
	}
	flicker := 0.82 + 0.18*(0.5+0.5*math.Sin(l.Phase))
	body := scaleRGBA(l.Body, alpha*flicker)
	flame := scaleRGBA(l.Flame, alpha*(0.72+0.28*math.Sin(l.Phase*1.7)))
	halo := scaleRGBA(l.Body, alpha*p.cfg.Glow*0.28)
	shadow := scaleRGBA(l.Body, alpha*flicker*0.58)

	x0 := int(math.Round(l.Col)) - l.Width/2
	y0 := int(math.Round(l.Row)) - l.Height/2
	for y := y0 - 1; y <= y0+l.Height; y++ {
		for x := x0 - 1; x <= x0+l.Width; x++ {
			if x < x0 || x >= x0+l.Width || y < y0 || y >= y0+l.Height {
				p.paintMaxLocked(x, y, halo)
			}
		}
	}
	for y := 0; y < l.Height; y++ {
		for x := 0; x < l.Width; x++ {
			c := body
			if y == 0 || y == l.Height-1 || x == 0 || x == l.Width-1 {
				c = shadow
			}
			p.paintMaxLocked(x0+x, y0+y, c)
		}
	}
	p.paintMaxLocked(x0+l.Width/2, y0+l.Height/2, flame)
	if l.Width >= 4 {
		p.paintMaxLocked(x0+l.Width/2-1, y0+l.Height/2, scaleRGBA(flame, 0.72))
	}
}

func (p *PaperLanterns) paintMaxLocked(x, y int, c color.RGBA) {
	if x < 0 || y < 0 || y >= p.H || x >= p.W {
		return
	}
	dst := &p.Grid[y][x]
	if !dst.Filled {
		*dst = Pixel{Filled: true, C: c}
		return
	}
	if c.R > dst.C.R {
		dst.C.R = c.R
	}
	if c.G > dst.C.G {
		dst.C.G = c.G
	}
	if c.B > dst.C.B {
		dst.C.B = c.B
	}
	dst.C.A = 255
}

func scaleRGBA(c color.RGBA, a float64) color.RGBA {
	a = clamp01(a)
	return color.RGBA{
		R: uint8(math.Round(float64(c.R) * a)),
		G: uint8(math.Round(float64(c.G) * a)),
		B: uint8(math.Round(float64(c.B) * a)),
		A: 255,
	}
}
