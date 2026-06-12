package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	PaperLanternsEventLanternEmit  = "lantern-emit"
	PaperLanternsEventReleasePulse = "release-pulse"
	PaperLanternsEventWindDrift    = "wind-drift"
	PaperLanternsEventLanternFade  = "lantern-fade"
	PaperLanternsEventQuietGap     = "quiet-gap"
	PaperLanternsEventIntro        = "intro"
	PaperLanternsEventEnding       = "ending"
)

type paperLantern struct {
	Row, Col   float64
	VRow, VCol float64
	Hue        float64
	Lightness  float64
	Phase      float64
	SwayAmp    float64
	Size       float64
	Age        int
	Background bool
}

// PaperLanternsConfig tunes a slow sky-lantern release. Hue is the primary
// scene knob: motion stays calm while palettes shift the festival mood.
type PaperLanternsConfig struct {
	// INTRODUCTION
	IntroFirstDelay   int `json:"intro_first_delay"`
	IntroClusterDelay int `json:"intro_cluster_delay"`
	// ENDING
	EndingTail int `json:"ending_tail"`
	// SPAWN / RELEASE
	SpawnEvery  int `json:"spawn"`
	MaxLanterns int `json:"max"`
	ClusterMin  int `json:"cluster_min"`
	ClusterMax  int `json:"cluster_max"`
	PulseWindow int `json:"pulse_window"`
	// MOTION
	RiseSpeed   float64 `json:"rise"`
	SpeedJitter float64 `json:"rise_jit"`
	Wind        float64 `json:"wind"`
	WindShift   float64 `json:"wind_shift"`
	Sway        float64 `json:"sway"`
	// SHAPE / DEPTH
	Size         float64 `json:"size"`
	FadeAltitude float64 `json:"fade_alt"`
	Glow         float64 `json:"glow"`
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// EVENT CHANCES
	ReleaseChance   float64 `json:"release_p"`
	WindDriftChance float64 `json:"wind_p"`
	QuietChance     float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	WindDriftDur int     `json:"wind_dur"`
	QuietDur     int     `json:"quiet_dur"`
	QuietMult    float64 `json:"quiet_mult"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroFirstDelay <= 0 {
		c.IntroFirstDelay = 18
	}
	if c.IntroClusterDelay <= 0 {
		c.IntroClusterDelay = 82
	}
	if c.IntroClusterDelay < c.IntroFirstDelay+5 {
		c.IntroClusterDelay = c.IntroFirstDelay + 5
	}
	if c.EndingTail <= 0 {
		c.EndingTail = 520
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 34
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 72
	}
	if c.MaxLanterns > 220 {
		c.MaxLanterns = 220
	}
	if c.ClusterMin <= 0 {
		c.ClusterMin = 5
	}
	if c.ClusterMax <= 0 {
		c.ClusterMax = 9
	}
	if c.ClusterMax < c.ClusterMin {
		c.ClusterMin, c.ClusterMax = c.ClusterMax, c.ClusterMin
	}
	if c.ClusterMin < 1 {
		c.ClusterMin = 1
	}
	if c.ClusterMax > 24 {
		c.ClusterMax = 24
	}
	if c.PulseWindow <= 0 {
		c.PulseWindow = 20
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.13
	}
	if c.RiseSpeed > 0.5 {
		c.RiseSpeed = 0.5
	}
	if c.SpeedJitter <= 0 {
		c.SpeedJitter = 0.22
	}
	if c.SpeedJitter > 1 {
		c.SpeedJitter = 1
	}
	if c.WindShift <= 0 {
		c.WindShift = 0.55
	}
	if c.Sway <= 0 {
		c.Sway = 0.48
	}
	if c.Size <= 0 {
		c.Size = 1
	}
	if c.Size > 2.4 {
		c.Size = 2.4
	}
	if c.FadeAltitude <= 0 {
		c.FadeAltitude = 0.28
	}
	if c.FadeAltitude > 0.75 {
		c.FadeAltitude = 0.75
	}
	if c.Glow <= 0 {
		c.Glow = 0.78
	}
	if c.Glow > 1.6 {
		c.Glow = 1.6
	}
	if c.Layers <= 0 {
		c.Layers = 2
	}
	if c.Layers > 2 {
		c.Layers = 2
	}
	if c.LayerBalance <= 0 {
		c.LayerBalance = 0.34
	}
	c.LayerBalance = clamp01(c.LayerBalance)
	if c.Hue == 0 && c.Saturation == 0 && c.LightnessMin == 0 && c.LightnessMax == 0 {
		c.Hue = 36
		c.Saturation = 0.66
		c.LightnessMin = 0.42
		c.LightnessMax = 0.9
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.66
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.42
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.9
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.ReleaseChance <= 0 {
		c.ReleaseChance = 0.0022
	}
	if c.WindDriftChance < 0 {
		c.WindDriftChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.WindDriftDur <= 0 {
		c.WindDriftDur = 190
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 420
	}
	if c.QuietMult <= 0 || c.QuietMult > 1 {
		c.QuietMult = 0.35
	}
	return c
}

func NormalizePaperLanternsConfig(c PaperLanternsConfig) PaperLanternsConfig {
	return c.withDefaults()
}

func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name:           "paper-lanterns",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_event", Label: "intro", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: PaperLanternsEventIntro,
				Description: "Preview the first lone lantern followed by the opening release."},
			{Key: "intro_first_delay", Label: "first delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 18,
				Description: "Ticks before the first lone lantern appears during intro."},
			{Key: "intro_cluster_delay", Label: "cluster delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 360, Step: 5, Default: 82,
				Description: "Ticks before the intro release pulse follows the first lantern."},
			{Key: "ending_event", Label: "ending", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: PaperLanternsEventEnding,
				Description: "Stop new releases and let the sky clear to the terminal dark state."},
			{Key: "ending_tail", Label: "tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 90, Max: 1200, Step: 10, Default: 520,
				Description: "Ticks allowed for in-flight lanterns to rise out and fade."},
			{Key: "fade_event", Label: "fade top", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: PaperLanternsEventLanternFade,
				Description: "Nudge the highest lantern into the top fade band for dev preview."},
			{Key: "spawn", Label: "lone 1/", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 8, Max: 120, Step: 1, Default: 34, Trigger: PaperLanternsEventLanternEmit,
				Description: "One-in-N cadence for lone drifters; trigger launches one now."},
			{Key: "max", Label: "max lights", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 8, Max: 180, Step: 1, Default: 72,
				Description: "Maximum active lanterns in the sky."},
			{Key: "cluster_min", Label: "cluster min", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 2, Max: 18, Step: 1, Default: 5,
				Description: "Minimum lanterns emitted by a release pulse."},
			{Key: "cluster_max", Label: "cluster max", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 3, Max: 24, Step: 1, Default: 9,
				Description: "Maximum lanterns emitted by a release pulse."},
			{Key: "pulse_window", Label: "pulse window", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 4, Max: 80, Step: 2, Default: 20,
				Description: "Ticks over which a release pulse staggers its lanterns."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.04, Max: 0.34, Step: 0.01, Default: 0.13,
				Description: "Vertical buoyancy. Higher values rise faster but stay non-rocketlike."},
			{Key: "rise_jit", Label: "rise jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.02, Default: 0.22,
				Description: "Per-lantern variation in rise speed."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -1.2, Max: 1.2, Step: 0.02, Default: 0,
				Description: "Baseline horizontal drift. Positive bends right; negative bends left."},
			{Key: "wind_shift", Label: "wind shift", Slot: SlotEventMod, Group: "wind-drift", Type: KnobFloat, Min: 0.05, Max: 1.4, Step: 0.05, Default: 0.55,
				Description: "Maximum temporary wind bias when wind-drift fires."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1.4, Step: 0.05, Default: 0.48,
				Description: "Gentle per-lantern side-to-side meander."},
			{Key: "size", Label: "size", Slot: SlotSpawn, Group: "shape", Type: KnobFloat, Min: 0.6, Max: 2.2, Step: 0.05, Default: 1,
				Description: "Paper rectangle size before per-lantern jitter."},
			{Key: "fade_alt", Label: "fade altitude", Slot: SlotEnd, Group: "shape", Type: KnobFloat, Min: 0.08, Max: 0.55, Step: 0.01, Default: 0.28,
				Description: "Fraction of scene height where lanterns start fading near the top."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.15, Max: 1.5, Step: 0.05, Default: 0.78,
				Description: "Soft halo strength around each paper lantern."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 2,
				Description: "1 = single depth; 2 adds smaller dim background lanterns."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0.05, Max: 0.85, Step: 0.05, Default: 0.34,
				Description: "Fraction of lanterns assigned to the dimmer background layer."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 18, Max: 58, Step: 1, Default: 36,
				Description: "Base paper-lantern hue: amber through candle gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Per-lantern hue variation inside the warm paper palette."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 1, Step: 0.01, Default: 0.66,
				Description: "Overall warmth and color intensity of the lantern paper."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.18, Max: 0.75, Step: 0.01, Default: 0.42,
				Description: "Minimum paper lightness for dimmer distant lanterns."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.35, Max: 0.98, Step: 0.01, Default: 0.9,
				Description: "Maximum paper and flame brightness."},
			{Key: "release_p", Label: "release", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0022, Trigger: PaperLanternsEventReleasePulse,
				Description: "Per-tick chance of a clustered release pulse."},
			{Key: "wind_p", Label: "wind drift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: PaperLanternsEventWindDrift,
				Description: "Per-tick chance that the global drift bias shifts for a while."},
			{Key: "quiet_p", Label: "quiet gap", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: PaperLanternsEventQuietGap,
				Description: "Per-tick chance of a long release-suppression window."},
			{Key: "wind_dur", Label: "wind dur", Slot: SlotEventMod, Group: "wind-drift", Type: KnobInt, Min: 30, Max: 600, Step: 10, Default: 190,
				Description: "Duration of a temporary wind-drift bias."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 90, Max: 1200, Step: 10, Default: 420,
				Description: "Duration of the release-suppression quiet gap."},
			{Key: "quiet_mult", Label: "quiet lone", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Lone-lantern density multiplier during a quiet gap."},
		},
	}
}

type PaperLanternsState struct {
	Tick                 int       `json:"tick"`
	WindDriftTicks       int       `json:"windDriftTicks"`
	WindBias             float64   `json:"windBias"`
	QuietTicks           int       `json:"quietTicks"`
	ReleaseTicks         int       `json:"releaseTicks"`
	ReleaseWindow        int       `json:"releaseWindow"`
	ReleaseLeft          int       `json:"releaseLeft"`
	ReleaseTotal         int       `json:"releaseTotal"`
	ReleaseCenter        float64   `json:"releaseCenter"`
	ReleaseSpread        float64   `json:"releaseSpread"`
	IntroTicks           int       `json:"introTicks"`
	IntroTotal           int       `json:"introTotal"`
	IntroFirstLeft       int       `json:"introFirstLeft"`
	IntroClusterLeft     int       `json:"introClusterLeft"`
	IntroFirstLaunched   bool      `json:"introFirstLaunched"`
	IntroClusterLaunched bool      `json:"introClusterLaunched"`
	EndingTicks          int       `json:"endingTicks"`
	EndingTotal          int       `json:"endingTotal"`
	Ended                bool      `json:"ended"`
	Lifecycle            Lifecycle `json:"lifecycle"`
}

type PaperLantern struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	Hue        float64 `json:"hue"`
	Lightness  float64 `json:"lightness"`
	Phase      float64 `json:"phase"`
	SwayAmp    float64 `json:"swayAmp"`
	Size       float64 `json:"size"`
	Age        int     `json:"age"`
	Background bool    `json:"background"`
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

type PaperLanterns struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	windDriftTicks int
	windBias       float64
	quietTicks     int

	releaseTicks  int
	releaseWindow int
	releaseLeft   int
	releaseTotal  int
	releaseCenter float64
	releaseSpread float64

	introTicks           int
	introTotal           int
	introFirstLeft       int
	introClusterLeft     int
	introFirstLaunched   bool
	introClusterLaunched bool

	endingTicks int
	endingTotal int
	ended       bool

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	p := &PaperLanterns{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
	p.paintBackgroundLocked()
	return p
}

func (p *PaperLanterns) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	oldW, oldH := p.W, p.H
	if w == oldW && h == oldH {
		return
	}
	p.W = w
	p.H = h
	p.Grid = make([][]Pixel, h)
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, w)
	}
	if oldW > 0 && oldH > 0 {
		xScale := float64(w) / float64(oldW)
		yScale := float64(h) / float64(oldH)
		for i := range p.lanterns {
			p.lanterns[i].Col *= xScale
			p.lanterns[i].Row *= yScale
		}
		p.releaseCenter *= xScale
		p.releaseSpread *= xScale
	}
	p.rebuildGridLocked()
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
	p.rebuildGridLocked()
}

func (p *PaperLanterns) EffectiveConfig() PaperLanternsConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *PaperLanterns) SnapshotState() PaperLanternsState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotStateLocked()
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

func (p *PaperLanterns) RestoreState(s PaperLanternsState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s)
	p.rebuildGridLocked()
}

func (p *PaperLanterns) RestoreSnapshot(s PaperLanternsSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.PaperLanternsState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreLanternsLocked(s.Lanterns)
	p.rebuildGridLocked()
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
	p.rebuildGridLocked()
}

func (p *PaperLanterns) LanternsCopy() []PaperLantern {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyLanternsLocked()
}

func (p *PaperLanterns) RestoreLanterns(list []PaperLantern) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreLanternsLocked(list)
	p.rebuildGridLocked()
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

func (p *PaperLanterns) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case PaperLanternsEventLanternEmit:
		p.ended = false
		if p.spawnLanternLocked(false) {
			p.appendLog(PaperLanternsEventLanternEmit, "triggered")
		} else {
			p.appendLog(PaperLanternsEventLanternEmit, "triggered (sky full)")
		}
	case PaperLanternsEventReleasePulse:
		p.ended = false
		p.startReleasePulseLocked("triggered")
	case PaperLanternsEventWindDrift:
		p.startWindDriftLocked("triggered")
	case PaperLanternsEventLanternFade:
		p.nudgeLanternFadeLocked()
	case PaperLanternsEventQuietGap:
		p.startQuietGapLocked("triggered")
	case PaperLanternsEventIntro:
		p.startIntroLocked()
		p.appendLog(PaperLanternsEventIntro, fmt.Sprintf("started (first=%d, cluster=%d)", p.cfg.IntroFirstDelay, p.cfg.IntroClusterDelay))
	case PaperLanternsEventEnding:
		p.startEndingLocked()
		p.appendLog(PaperLanternsEventEnding, fmt.Sprintf("started (tail=%d)", p.endingTotal))
	default:
		return false
	}
	p.rebuildGridLocked()
	return true
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *PaperLanterns) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	if p.ended {
		p.paintBackgroundLocked()
		return
	}

	if p.windDriftTicks > 0 {
		p.windDriftTicks--
		if p.windDriftTicks == 0 {
			p.windBias = 0
		}
	}
	if p.quietTicks > 0 {
		p.quietTicks--
	}

	introActive := p.introTicks > 0
	endingActive := p.endingTicks > 0

	if introActive {
		p.stepIntroLocked()
	}
	if !endingActive {
		p.stepReleasePulseLocked()
	}
	if !introActive && !endingActive {
		p.stepEventsLocked()
		p.stepLoneEmitLocked()
	}

	p.stepLanternsLocked()
	p.rebuildGridLocked()

	if introActive {
		p.introTicks--
		if p.introTicks <= 0 {
			p.introTicks = 0
			p.introTotal = 0
			p.introFirstLeft = 0
			p.introClusterLeft = 0
		}
	}
	if endingActive {
		p.endingTicks--
		if p.endingTicks <= 0 {
			p.endingTicks = 0
			p.endingTotal = 0
			p.ended = true
			p.releaseTicks = 0
			p.releaseLeft = 0
			p.quietTicks = 0
			p.windDriftTicks = 0
			p.windBias = 0
			p.lanterns = nil
			p.paintBackgroundLocked()
		}
	}
}

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	return PaperLanternsState{
		Tick:                 p.tick,
		WindDriftTicks:       p.windDriftTicks,
		WindBias:             p.windBias,
		QuietTicks:           p.quietTicks,
		ReleaseTicks:         p.releaseTicks,
		ReleaseWindow:        p.releaseWindow,
		ReleaseLeft:          p.releaseLeft,
		ReleaseTotal:         p.releaseTotal,
		ReleaseCenter:        p.releaseCenter,
		ReleaseSpread:        p.releaseSpread,
		IntroTicks:           p.introTicks,
		IntroTotal:           p.introTotal,
		IntroFirstLeft:       p.introFirstLeft,
		IntroClusterLeft:     p.introClusterLeft,
		IntroFirstLaunched:   p.introFirstLaunched,
		IntroClusterLaunched: p.introClusterLaunched,
		EndingTicks:          p.endingTicks,
		EndingTotal:          p.endingTotal,
		Ended:                p.ended,
		Lifecycle:            p.lifecycleLocked(),
	}
}

func (p *PaperLanterns) lifecycleLocked() Lifecycle {
	switch {
	case p.ended:
		return LifecycleEnded
	case p.introTicks > 0:
		return LifecycleIntro
	case p.endingTicks > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.windDriftTicks = s.WindDriftTicks
	p.windBias = s.WindBias
	p.quietTicks = s.QuietTicks
	p.releaseTicks = s.ReleaseTicks
	p.releaseWindow = s.ReleaseWindow
	p.releaseLeft = s.ReleaseLeft
	p.releaseTotal = s.ReleaseTotal
	p.releaseCenter = s.ReleaseCenter
	p.releaseSpread = s.ReleaseSpread
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.introFirstLeft = s.IntroFirstLeft
	p.introClusterLeft = s.IntroClusterLeft
	p.introFirstLaunched = s.IntroFirstLaunched
	p.introClusterLaunched = s.IntroClusterLaunched
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.ended = s.Ended
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			Row:        l.Row,
			Col:        l.Col,
			VRow:       l.VRow,
			VCol:       l.VCol,
			Hue:        l.Hue,
			Lightness:  l.Lightness,
			Phase:      l.Phase,
			SwayAmp:    l.SwayAmp,
			Size:       l.Size,
			Age:        l.Age,
			Background: l.Background,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			Row:        l.Row,
			Col:        l.Col,
			VRow:       l.VRow,
			VCol:       l.VCol,
			Hue:        l.Hue,
			Lightness:  l.Lightness,
			Phase:      l.Phase,
			SwayAmp:    l.SwayAmp,
			Size:       l.Size,
			Age:        l.Age,
			Background: l.Background,
		}
	}
}

func (p *PaperLanterns) startIntroLocked() {
	p.ended = false
	p.endingTicks = 0
	p.endingTotal = 0
	p.releaseTicks = 0
	p.releaseLeft = 0
	p.quietTicks = 0
	p.windDriftTicks = 0
	p.windBias = 0
	p.lanterns = nil
	p.introFirstLeft = p.cfg.IntroFirstDelay
	p.introClusterLeft = p.cfg.IntroClusterDelay
	p.introFirstLaunched = false
	p.introClusterLaunched = false
	p.introTotal = p.cfg.IntroClusterDelay + p.cfg.PulseWindow + 40
	if p.introTotal < p.cfg.IntroFirstDelay+20 {
		p.introTotal = p.cfg.IntroFirstDelay + 20
	}
	p.introTicks = p.introTotal
}

func (p *PaperLanterns) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.releaseTicks = 0
	p.releaseLeft = 0
	p.quietTicks = 0
	p.endingTotal = p.cfg.EndingTail
	if p.endingTotal <= 0 {
		p.endingTotal = 520
	}
	p.endingTicks = p.endingTotal
	p.ended = false
}

func (p *PaperLanterns) stepIntroLocked() {
	if !p.introFirstLaunched {
		if p.introFirstLeft <= 0 {
			p.spawnLanternLocked(false)
			p.introFirstLaunched = true
		} else {
			p.introFirstLeft--
		}
	}
	if !p.introClusterLaunched {
		if p.introClusterLeft <= 0 {
			p.startReleasePulseLocked("intro")
			p.introClusterLaunched = true
		} else {
			p.introClusterLeft--
		}
	}
}

func (p *PaperLanterns) stepEventsLocked() {
	if p.releaseTicks == 0 && p.quietTicks == 0 && p.rng.Float64() < p.cfg.ReleaseChance {
		p.startReleasePulseLocked("started")
	}
	if p.windDriftTicks == 0 && p.cfg.WindDriftChance > 0 && p.rng.Float64() < p.cfg.WindDriftChance {
		p.startWindDriftLocked("started")
	}
	if p.quietTicks == 0 && p.cfg.QuietChance > 0 && p.rng.Float64() < p.cfg.QuietChance {
		p.startQuietGapLocked("started")
	}
}

func (p *PaperLanterns) stepLoneEmitLocked() {
	if len(p.lanterns) >= p.cfg.MaxLanterns {
		return
	}
	spawnEvery := p.cfg.SpawnEvery
	if p.quietTicks > 0 {
		spawnEvery = int(math.Round(float64(spawnEvery) / math.Max(0.05, p.cfg.QuietMult)))
	}
	if spawnEvery < 1 {
		spawnEvery = 1
	}
	if p.rng.Intn(spawnEvery) == 0 {
		p.spawnLanternLocked(false)
	}
}

func (p *PaperLanterns) startReleasePulseLocked(verb string) {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	count := p.cfg.ClusterMin
	if p.cfg.ClusterMax > p.cfg.ClusterMin {
		count += p.rng.Intn(p.cfg.ClusterMax - p.cfg.ClusterMin + 1)
	}
	p.releaseWindow = max(1, p.cfg.PulseWindow)
	p.releaseTicks = p.releaseWindow
	p.releaseTotal = count
	p.releaseLeft = count
	p.releaseCenter = p.rng.Float64() * float64(max(1, p.W-1))
	p.releaseSpread = math.Max(4, float64(p.W)*(0.10+p.rng.Float64()*0.16))
	p.spawnFromPulseLocked()
	p.appendLog(PaperLanternsEventReleasePulse, fmt.Sprintf("%s (count=%d, window=%d)", verb, count, p.releaseWindow))
}

func (p *PaperLanterns) stepReleasePulseLocked() {
	if p.releaseTicks <= 0 {
		p.releaseLeft = 0
		return
	}
	if p.releaseLeft > 0 {
		elapsed := p.releaseWindow - p.releaseTicks
		spacing := max(1, p.releaseWindow/max(1, p.releaseTotal))
		if elapsed%spacing == 0 || p.releaseTicks <= p.releaseLeft {
			p.spawnFromPulseLocked()
		}
	}
	p.releaseTicks--
	if p.releaseTicks <= 0 {
		p.releaseTicks = 0
		p.releaseLeft = 0
	}
}

func (p *PaperLanterns) spawnFromPulseLocked() {
	if p.releaseLeft <= 0 {
		return
	}
	spawns := 1
	if p.releaseLeft > 4 && p.rng.Float64() < 0.22 {
		spawns = 2
	}
	for i := 0; i < spawns && p.releaseLeft > 0; i++ {
		if p.spawnLanternLocked(true) {
			p.releaseLeft--
		} else {
			return
		}
	}
}

func (p *PaperLanterns) startWindDriftLocked(verb string) {
	p.windDriftTicks = jitterInt(p.rng, p.cfg.WindDriftDur, 0.25)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.windBias = sign * p.cfg.WindShift * (0.45 + p.rng.Float64()*0.55)
	p.appendLog(PaperLanternsEventWindDrift, fmt.Sprintf("%s (dur=%d, bias=%+.2f)", verb, p.windDriftTicks, p.windBias))
}

func (p *PaperLanterns) startQuietGapLocked(verb string) {
	p.quietTicks = jitterInt(p.rng, p.cfg.QuietDur, 0.25)
	p.releaseTicks = 0
	p.releaseLeft = 0
	p.appendLog(PaperLanternsEventQuietGap, fmt.Sprintf("%s (dur=%d, lone=%.2f)", verb, p.quietTicks, p.cfg.QuietMult))
}

func (p *PaperLanterns) nudgeLanternFadeLocked() {
	if len(p.lanterns) == 0 {
		p.appendLog(PaperLanternsEventLanternFade, "triggered (no lanterns)")
		return
	}
	idx := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Row < p.lanterns[idx].Row {
			idx = i
		}
	}
	fadeStart := p.fadeStartLocked()
	if fadeStart < 1 {
		fadeStart = 1
	}
	if p.lanterns[idx].Row > fadeStart*0.35 {
		p.lanterns[idx].Row = fadeStart * 0.35
	}
	if p.lanterns[idx].VRow > -p.cfg.RiseSpeed {
		p.lanterns[idx].VRow = -p.cfg.RiseSpeed
	}
	p.appendLog(PaperLanternsEventLanternFade, fmt.Sprintf("triggered (row=%.1f)", p.lanterns[idx].Row))
}

func (p *PaperLanterns) spawnLanternLocked(cluster bool) bool {
	if p.W <= 0 || p.H <= 0 || len(p.lanterns) >= p.cfg.MaxLanterns {
		return false
	}
	background := p.cfg.Layers >= 2 && p.rng.Float64() < p.cfg.LayerBalance
	size := p.cfg.Size * (0.86 + p.rng.Float64()*0.32)
	if background {
		size *= 0.72
	}
	col := p.rng.Float64() * float64(max(1, p.W-1))
	if cluster {
		col = p.releaseCenter + (p.rng.Float64()*2-1)*p.releaseSpread*0.5
		if col < 0 {
			col = p.rng.Float64() * math.Min(4, float64(p.W))
		}
		if col > float64(p.W-1) {
			col = float64(p.W-1) - p.rng.Float64()*math.Min(4, float64(p.W))
		}
	}
	speed := p.cfg.RiseSpeed * (1 + p.cfg.SpeedJitter*(p.rng.Float64()*2-1))
	if speed < p.cfg.RiseSpeed*0.35 {
		speed = p.cfg.RiseSpeed * 0.35
	}
	if background {
		speed *= 0.76
	}
	wind := p.currentWindLocked()
	vCol := wind*0.055 + (p.rng.Float64()*2-1)*0.018
	if background {
		vCol *= 0.75
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	if background {
		light *= 0.82
	}
	p.lanterns = append(p.lanterns, paperLantern{
		Row:        float64(p.H) + size*(1.6+p.rng.Float64()*1.8),
		Col:        col,
		VRow:       -speed,
		VCol:       vCol,
		Hue:        hue,
		Lightness:  light,
		Phase:      p.rng.Float64() * 2 * math.Pi,
		SwayAmp:    p.cfg.Sway * (0.45 + p.rng.Float64()*0.75),
		Size:       size,
		Age:        0,
		Background: background,
	})
	return true
}

func (p *PaperLanterns) stepLanternsLocked() {
	if len(p.lanterns) == 0 {
		return
	}
	alive := p.lanterns[:0]
	for _, l := range p.lanterns {
		l.Age++
		targetWind := p.currentWindLocked() * 0.055
		if l.Background {
			targetWind *= 0.75
		}
		l.VCol += (targetWind - l.VCol) * 0.018
		l.VCol += (p.rng.Float64()*2 - 1) * 0.0025
		maxDrift := 0.16 + math.Abs(targetWind)*1.8
		if l.VCol > maxDrift {
			l.VCol = maxDrift
		}
		if l.VCol < -maxDrift {
			l.VCol = -maxDrift
		}
		sway := math.Sin(float64(l.Age)*0.035+l.Phase) * l.SwayAmp * 0.022
		l.Col += l.VCol + sway
		l.Row += l.VRow
		if p.endingTicks > 0 && p.endingTotal > 0 {
			progress := phaseProgress(p.endingTotal, p.endingTicks)
			l.VRow -= 0.0005 * progress
		}
		bodyH := p.lanternHeightLocked(l)
		offscreenSide := l.Col < -float64(bodyH+6) || l.Col > float64(p.W+bodyH+6)
		offscreenTop := l.Row < -float64(bodyH+2)
		if !offscreenSide && !offscreenTop {
			alive = append(alive, l)
		}
	}
	p.lanterns = alive
}

func (p *PaperLanterns) currentWindLocked() float64 {
	breath := 0.11 * math.Sin(float64(p.tick)*0.006+0.8)
	return p.cfg.Wind + p.windBias + breath
}

func (p *PaperLanterns) fadeStartLocked() float64 {
	return math.Max(2, float64(p.H)*p.cfg.FadeAltitude)
}

func (p *PaperLanterns) lanternHeightLocked(l paperLantern) int {
	h := int(math.Round(4.2 * l.Size))
	if h < 3 {
		h = 3
	}
	if h > 9 {
		h = 9
	}
	return h
}

func (p *PaperLanterns) lanternAlphaLocked(l paperLantern) float64 {
	alpha := 1.0
	fadeStart := p.fadeStartLocked()
	if l.Row < fadeStart {
		alpha *= clamp01(l.Row / fadeStart)
	}
	if l.Age < 18 {
		alpha *= clamp01(float64(l.Age) / 18)
	}
	if p.endingTicks > 0 && p.endingTotal > 0 {
		progress := phaseProgress(p.endingTotal, p.endingTicks)
		if progress > 0.78 {
			alpha *= clamp01(1 - (progress-0.78)/0.22)
		}
	}
	if l.Background {
		alpha *= 0.72
	}
	return clamp01(alpha)
}

func (p *PaperLanterns) rebuildGridLocked() {
	p.paintBackgroundLocked()
	for _, l := range p.lanterns {
		p.paintLanternLocked(l)
	}
}

func (p *PaperLanterns) paintBackgroundLocked() {
	for y := range p.Grid {
		yr := 0.0
		if p.H > 1 {
			yr = float64(y) / float64(p.H-1)
		}
		c := hslToRGB(228, 0.34, 0.022+0.026*yr)
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (p *PaperLanterns) paintLanternLocked(l paperLantern) {
	alpha := p.lanternAlphaLocked(l)
	if alpha <= 0.01 {
		return
	}
	cx := int(math.Round(l.Col))
	cy := int(math.Round(l.Row))
	bodyH := p.lanternHeightLocked(l)
	bodyW := int(math.Round(float64(bodyH) * 0.68))
	if bodyW < 2 {
		bodyW = 2
	}
	if bodyW > 7 {
		bodyW = 7
	}

	glow := hslToRGB(l.Hue, clamp01(p.cfg.Saturation*0.92), clamp01(l.Lightness*0.78))
	glowRX := bodyW + 3
	glowRY := bodyH + 3
	for dy := -glowRY; dy <= glowRY; dy++ {
		for dx := -glowRX; dx <= glowRX; dx++ {
			nx := float64(dx) / float64(max(1, glowRX))
			ny := float64(dy) / float64(max(1, glowRY))
			dist := math.Sqrt(nx*nx + ny*ny)
			if dist > 1 {
				continue
			}
			strength := math.Pow(1-dist, 1.7) * p.cfg.Glow * 0.42 * alpha
			if strength <= 0.015 {
				continue
			}
			p.paintMax(cy+dy, cx+dx, scaleRGBA(glow, strength))
		}
	}

	paper := hslToRGB(l.Hue, clamp01(p.cfg.Saturation*0.78), clamp01(l.Lightness))
	edge := hslToRGB(l.Hue, clamp01(p.cfg.Saturation*0.9), clamp01(l.Lightness*0.62))
	for dy := -bodyH / 2; dy <= bodyH/2; dy++ {
		widthAtRow := bodyW
		if dy == -bodyH/2 || dy == bodyH/2 {
			widthAtRow = max(1, bodyW-1)
		}
		for dx := -widthAtRow / 2; dx <= widthAtRow/2; dx++ {
			isEdge := dy == -bodyH/2 || dy == bodyH/2 || dx == -widthAtRow/2 || dx == widthAtRow/2
			c := paper
			strength := alpha
			if isEdge {
				c = edge
				strength *= 0.82
			}
			p.paintMax(cy+dy, cx+dx, scaleRGBA(c, strength))
		}
	}

	flame := hslToRGB(30, 0.95, clamp01(p.cfg.LightnessMax))
	flameCore := hslToRGB(48, 0.9, 0.96)
	fy := cy + max(0, bodyH/5)
	p.paintMax(fy, cx, scaleRGBA(flameCore, alpha))
	p.paintMax(fy+1, cx, scaleRGBA(flame, alpha*0.78))
	if bodyW >= 4 {
		p.paintMax(fy, cx-1, scaleRGBA(flame, alpha*0.46))
		p.paintMax(fy, cx+1, scaleRGBA(flame, alpha*0.46))
	}
}

func (p *PaperLanterns) paintMax(row, col int, c color.RGBA) {
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

func scaleRGBA(c color.RGBA, alpha float64) color.RGBA {
	alpha = clamp01(alpha)
	return color.RGBA{
		R: uint8(math.Round(float64(c.R) * alpha)),
		G: uint8(math.Round(float64(c.G) * alpha)),
		B: uint8(math.Round(float64(c.B) * alpha)),
		A: 255,
	}
}
