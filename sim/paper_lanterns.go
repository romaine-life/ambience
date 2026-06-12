package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type paperLantern struct {
	Row, Col     float64
	VRow, VCol   float64
	Width        float64
	Height       float64
	Hue          float64
	Saturation   float64
	Lightness    float64
	Phase        float64
	WobbleRate   float64
	WindResponse float64
	Age          int
}

// PaperLanternsConfig tunes the sky-lantern release simulation.
type PaperLanternsConfig struct {
	// INTRODUCTION
	IntroFirstDelay   int `json:"intro_first"`
	IntroClusterDelay int `json:"intro_cluster"`
	// ENDING
	EndingStop int `json:"ending_stop"`
	EndingTail int `json:"ending_tail"`
	// SPAWN
	EmitChance      float64 `json:"emit_p"`
	MaxLanterns     int     `json:"max"`
	ReleaseInterval int     `json:"release_interval"`
	ReleaseJitter   float64 `json:"release_jit"`
	PulseWindow     int     `json:"pulse_window"`
	ClusterMin      int     `json:"cluster_min"`
	ClusterMax      int     `json:"cluster_max"`
	QuietGapDur     int     `json:"quiet_gap_dur"`
	// MOTION
	RiseSpeed    float64 `json:"rise"`
	RiseJitter   float64 `json:"rise_jit"`
	WindDrift    float64 `json:"wind"`
	Sway         float64 `json:"sway"`
	WindShiftDur int     `json:"wind_shift_dur"`
	WindShiftStr float64 `json:"wind_shift_str"`
	// SHAPE
	Size      float64 `json:"size"`
	FadeStart float64 `json:"fade_start"`
	Glow      float64 `json:"glow"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// EVENT CHANCES
	ReleaseChance   float64 `json:"release_p"`
	WindShiftChance float64 `json:"wind_p"`
	QuietGapChance  float64 `json:"quiet_p"`
	// MANUAL EVENTS
	IntroEvent   int `json:"intro_event"`
	EndingEvent  int `json:"ending_event"`
	EmitEvent    int `json:"emit_event"`
	ReleaseEvent int `json:"release_event"`
	WindEvent    int `json:"wind_event"`
	FadeEvent    int `json:"fade_event"`
	QuietEvent   int `json:"quiet_event"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroFirstDelay <= 0 {
		c.IntroFirstDelay = 30
	}
	if c.IntroClusterDelay <= 0 {
		c.IntroClusterDelay = 150
	}
	if c.EndingStop < 0 {
		c.EndingStop = 0
	}
	if c.EndingTail <= 0 {
		c.EndingTail = 1800
	}
	if c.EmitChance <= 0 {
		c.EmitChance = 0.012
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 72
	}
	if c.ReleaseInterval <= 0 {
		c.ReleaseInterval = 420
	}
	if c.ReleaseJitter <= 0 {
		c.ReleaseJitter = 0.28
	}
	if c.ReleaseJitter > 0.9 {
		c.ReleaseJitter = 0.9
	}
	if c.PulseWindow <= 0 {
		c.PulseWindow = 24
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
	if c.QuietGapDur <= 0 {
		c.QuietGapDur = 180
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.12
	}
	if c.RiseJitter <= 0 {
		c.RiseJitter = 0.24
	}
	if c.RiseJitter > 0.8 {
		c.RiseJitter = 0.8
	}
	if c.Sway < 0 {
		c.Sway = 0
	}
	if c.Sway == 0 {
		c.Sway = 0.32
	}
	if c.WindShiftDur <= 0 {
		c.WindShiftDur = 180
	}
	if c.WindShiftStr <= 0 {
		c.WindShiftStr = 0.08
	}
	if c.Size <= 0 {
		c.Size = 1
	}
	if c.FadeStart <= 0 {
		c.FadeStart = 0.26
	}
	c.FadeStart = clamp01(c.FadeStart)
	if c.Glow <= 0 {
		c.Glow = 0.62
	}
	c.Glow = clamp01(c.Glow)
	if c.Hue == 0 {
		c.Hue = 38
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 14
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.72
	}
	c.Saturation = clamp01(c.Saturation)
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.36
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.82
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	c.LightnessMin = clamp01(c.LightnessMin)
	c.LightnessMax = clamp01(c.LightnessMax)
	if c.ReleaseChance < 0 {
		c.ReleaseChance = 0
	}
	if c.WindShiftChance < 0 {
		c.WindShiftChance = 0
	}
	if c.QuietGapChance < 0 {
		c.QuietGapChance = 0
	}
	return c
}

// PaperLanternsSchema describes the Paper Lanterns effect's dev knobs.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name:           "paper-lanterns",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_first", Label: "first delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 30,
				Description: "Ticks before the first lone lantern appears during the intro."},
			{Key: "intro_cluster", Label: "first cluster", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 30, Max: 360, Step: 5, Default: 150,
				Description: "Ticks before the intro release pulse establishes the festival rhythm."},
			{Key: "ending_stop", Label: "release stop", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 0,
				Description: "Ticks after ending starts before new releases are fully suppressed."},
			{Key: "ending_tail", Label: "tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 120, Max: 3600, Step: 30, Default: 1800,
				Description: "Maximum outro ticks for in-flight lanterns to rise and fade out."},
			{Key: "emit_p", Label: "lone emit", Slot: SlotSpawn, Group: "spawn", Type: KnobFloat, Min: 0, Max: 0.06, Step: 0.001, Default: 0.012,
				Description: "Per-tick chance of a lone lantern drifting up between clusters."},
			{Key: "max", Label: "max", Slot: SlotSpawn, Group: "spawn", Type: KnobInt, Min: 8, Max: 160, Step: 1, Default: 72,
				Description: "Maximum number of active lanterns."},
			{Key: "release_interval", Label: "release every", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 120, Max: 1200, Step: 30, Default: 420,
				Description: "Nominal ticks between automatic cluster releases."},
			{Key: "release_jit", Label: "release jitter", Slot: SlotEventMod, Group: "release-pulse", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.01, Default: 0.28,
				Description: "Random timing variation for automatic release intervals."},
			{Key: "pulse_window", Label: "pulse window", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 90, Step: 1, Default: 24,
				Description: "Ticks over which a cluster's lanterns are staggered."},
			{Key: "cluster_min", Label: "cluster min", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 5,
				Description: "Minimum lantern count in a release pulse."},
			{Key: "cluster_max", Label: "cluster max", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 20, Step: 1, Default: 9,
				Description: "Maximum lantern count in a release pulse."},
			{Key: "quiet_gap_dur", Label: "quiet gap", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 60, Max: 1500, Step: 30, Default: 180,
				Description: "Release suppression window after a cluster or quiet-gap trigger."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.04, Max: 0.28, Step: 0.005, Default: 0.12,
				Description: "Base upward speed. Lower values feel floatier and less rocket-like."},
			{Key: "rise_jit", Label: "rise jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.01, Default: 0.24,
				Description: "Per-lantern rise speed variation picked at spawn."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.18, Max: 0.18, Step: 0.005, Default: 0.035,
				Description: "Global horizontal drift bias. Positive carries lanterns right."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1.2, Step: 0.02, Default: 0.32,
				Description: "Gentle per-lantern side-to-side floating motion."},
			{Key: "wind_shift_dur", Label: "wind dur", Slot: SlotEventMod, Group: "wind-drift", Type: KnobInt, Min: 30, Max: 600, Step: 10, Default: 180,
				Description: "Duration of a wind-drift bias shift."},
			{Key: "wind_shift_str", Label: "wind shift", Slot: SlotEventMod, Group: "wind-drift", Type: KnobFloat, Min: 0.01, Max: 0.3, Step: 0.005, Default: 0.08,
				Description: "Extra sideways carry during a wind-drift event."},
			{Key: "size", Label: "size", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.6, Max: 2, Step: 0.05, Default: 1,
				Description: "Lantern body size in low-resolution pixels."},
			{Key: "fade_start", Label: "fade altitude", Slot: SlotEnd, Group: "shape", Type: KnobFloat, Min: 0.08, Max: 0.55, Step: 0.01, Default: 0.26,
				Description: "Top-of-scene fraction where lanterns begin dimming out."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.15, Max: 1, Step: 0.01, Default: 0.62,
				Description: "Soft warm halo strength around each lantern."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 18, Max: 62, Step: 1, Default: 38,
				Description: "Lantern palette hue. Lower warms orange; higher leans golden."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 45, Step: 1, Default: 14,
				Description: "Per-lantern palette variation inside a release."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Paper and glow color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.36,
				Description: "Dimmest paper shade used by the lantern bodies."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 0.98, Step: 0.01, Default: 0.82,
				Description: "Brightest warm glow and paper highlight."},
			{Key: "release_p", Label: "release chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0,
				Description: "Extra per-tick chance of a cluster release beyond the interval timer."},
			{Key: "wind_p", Label: "wind chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0,
				Description: "Per-tick chance of a temporary global wind drift shift."},
			{Key: "quiet_p", Label: "quiet chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0,
				Description: "Per-tick chance of suppressing cluster releases for a long quiet gap."},
			{Key: "intro_event", Label: "intro", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "intro",
				Description: "Restart from a dark sky, then launch a first lantern and intro cluster."},
			{Key: "ending_event", Label: "ending", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "ending",
				Description: "Stop releases and let in-flight lanterns fade to a dark sky."},
			{Key: "emit_event", Label: "lantern emit", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "lantern-emit",
				Description: "Launch one lantern from the bottom edge."},
			{Key: "release_event", Label: "release pulse", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "release-pulse",
				Description: "Launch a staggered cluster of paper lanterns."},
			{Key: "wind_event", Label: "wind drift", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "wind-drift",
				Description: "Shift the global wind bias for a short while."},
			{Key: "fade_event", Label: "lantern fade", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "lantern-fade",
				Description: "Push the highest lantern into its top-edge fade."},
			{Key: "quiet_event", Label: "quiet gap", Slot: SlotEvent, Group: "manual", Type: KnobInt, Min: 0, Max: 0, Step: 1, Default: 0, Trigger: "quiet-gap",
				Description: "Suppress cluster releases for a long quiet window."},
		},
	}
}

type PaperLanternsState struct {
	Tick              int       `json:"tick"`
	ReleaseTicks      int       `json:"releaseTicks"`
	QuietTicks        int       `json:"quietTicks"`
	PulseTicks        int       `json:"pulseTicks"`
	PendingPulse      int       `json:"pendingPulse"`
	WindShiftTicks    int       `json:"windShiftTicks"`
	WindBias          float64   `json:"windBias"`
	IntroTicks        int       `json:"introTicks"`
	IntroTotal        int       `json:"introTotal"`
	IntroFirstFired   bool      `json:"introFirstFired"`
	IntroClusterFired bool      `json:"introClusterFired"`
	EndingTicks       int       `json:"endingTicks"`
	EndingTotal       int       `json:"endingTotal"`
	Ended             bool      `json:"ended"`
	Lifecycle         Lifecycle `json:"lifecycle"`
}

type PaperLantern struct {
	Row          float64 `json:"row"`
	Col          float64 `json:"col"`
	VRow         float64 `json:"vRow"`
	VCol         float64 `json:"vCol"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Hue          float64 `json:"hue"`
	Saturation   float64 `json:"saturation"`
	Lightness    float64 `json:"lightness"`
	Phase        float64 `json:"phase"`
	WobbleRate   float64 `json:"wobbleRate"`
	WindResponse float64 `json:"windResponse"`
	Age          int     `json:"age"`
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

// PaperLanterns simulates calm paper lantern releases rising through a dark sky.
type PaperLanterns struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	releaseTicks      int
	quietTicks        int
	pulseTicks        int
	pendingPulse      int
	windShiftTicks    int
	windBias          float64
	introTicks        int
	introTotal        int
	introFirstFired   bool
	introClusterFired bool
	endingTicks       int
	endingTotal       int
	ended             bool

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	c := cfg.withDefaults()
	p := &PaperLanterns{
		W:     w,
		H:     h,
		Grid:  grid,
		rng:   rngutil.New(seed),
		cfg:   c,
		ended: false,
	}
	p.releaseTicks = max(1, jitterInt(p.rng, c.ReleaseInterval/2, c.ReleaseJitter))
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
	oldW, oldH := p.W, p.H
	p.W = w
	p.H = h
	p.Grid = make([][]Pixel, h)
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, w)
	}
	if oldW > 0 && oldH > 0 {
		colScale := float64(w) / float64(oldW)
		rowScale := float64(h) / float64(oldH)
		for i := range p.lanterns {
			p.lanterns[i].Col *= colScale
			p.lanterns[i].Row *= rowScale
			p.lanterns[i].VCol *= colScale
			p.lanterns[i].VRow *= rowScale
		}
	}
	p.paintFrameLocked()
}

func (p *PaperLanterns) SetConfig(cfg PaperLanternsConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg.withDefaults()
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

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
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
	case "lantern-emit":
		if p.ended {
			p.ended = false
		}
		if p.spawnLanternLocked() {
			p.appendLog("lantern-emit", "triggered")
		}
	case "release-pulse":
		if p.ended {
			p.ended = false
		}
		p.startReleasePulseLocked("triggered")
	case "wind-drift":
		p.startWindDriftLocked("triggered")
	case "lantern-fade":
		p.fadeHighestLanternLocked("triggered")
	case "quiet-gap":
		p.startQuietGapLocked("triggered")
	case "intro":
		p.startIntroLocked()
	case "ending":
		p.startEndingLocked()
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
	p.stepTimersLocked()
	if p.ended {
		p.paintFrameLocked()
		return
	}

	p.stepLifecycleLocked()
	p.stepEventScheduleLocked()
	p.stepLanternsLocked()
	p.paintFrameLocked()
}

func (p *PaperLanterns) stepTimersLocked() {
	for _, timer := range []*int{&p.releaseTicks, &p.quietTicks, &p.pulseTicks, &p.windShiftTicks} {
		if *timer > 0 {
			*timer--
		}
	}
	if p.windShiftTicks <= 0 {
		p.windBias = 0
	}
}

func (p *PaperLanterns) stepLifecycleLocked() {
	if p.introTicks > 0 {
		elapsed := p.introTotal - p.introTicks
		if !p.introFirstFired && elapsed >= p.cfg.IntroFirstDelay {
			p.spawnLanternLocked()
			p.introFirstFired = true
			p.appendLog("lantern-emit", "intro first lantern")
		}
		if !p.introClusterFired && elapsed >= p.cfg.IntroClusterDelay {
			p.startReleasePulseLocked("intro")
			p.introClusterFired = true
		}
		p.introTicks--
		if p.introTicks <= 0 {
			p.releaseTicks = p.nextReleaseTicksLocked()
			p.quietTicks = 0
			p.appendLog("intro", "resolved")
		}
	}

	if p.endingTicks > 0 {
		elapsed := p.endingTotal - p.endingTicks
		p.endingTicks--
		if elapsed >= p.cfg.EndingStop {
			p.pendingPulse = 0
			p.pulseTicks = 0
			p.releaseTicks = p.nextReleaseTicksLocked()
			p.quietTicks = max(p.quietTicks, p.cfg.QuietGapDur)
		}
		if p.endingTicks <= 0 || len(p.lanterns) == 0 {
			p.lanterns = nil
			p.endingTicks = 0
			p.ended = true
			p.appendLog("ending", "resolved")
		}
	}
}

func (p *PaperLanterns) stepEventScheduleLocked() {
	if p.introTicks > 0 || p.endingTicks > 0 {
		return
	}
	if p.pendingPulse > 0 {
		lanternsThisTick := 1
		if p.pulseTicks <= max(1, p.cfg.PulseWindow/3) && p.pendingPulse > 3 {
			lanternsThisTick = 2
		}
		for i := 0; i < lanternsThisTick && p.pendingPulse > 0; i++ {
			p.spawnLanternLocked()
			p.pendingPulse--
		}
		if p.pulseTicks <= 0 || p.pendingPulse <= 0 {
			p.pendingPulse = 0
			p.pulseTicks = 0
		}
	}
	if p.quietTicks <= 0 && len(p.lanterns) < p.cfg.MaxLanterns && p.rng.Float64() < p.cfg.EmitChance {
		p.spawnLanternLocked()
		p.appendLog("lantern-emit", "lone drifter")
	}
	if p.quietTicks <= 0 && p.pendingPulse <= 0 && (p.releaseTicks <= 0 || (p.cfg.ReleaseChance > 0 && p.rng.Float64() < p.cfg.ReleaseChance)) {
		p.startReleasePulseLocked("started")
	}
	if p.windShiftTicks <= 0 && p.cfg.WindShiftChance > 0 && p.rng.Float64() < p.cfg.WindShiftChance {
		p.startWindDriftLocked("started")
	}
	if p.quietTicks <= 0 && p.cfg.QuietGapChance > 0 && p.rng.Float64() < p.cfg.QuietGapChance {
		p.startQuietGapLocked("started")
	}
}

func (p *PaperLanterns) stepLanternsLocked() {
	if len(p.lanterns) == 0 {
		return
	}
	wind := p.currentWindLocked()
	dst := p.lanterns[:0]
	for i := range p.lanterns {
		l := p.lanterns[i]
		l.Age++
		l.Phase += l.WobbleRate
		bob := math.Sin(l.Phase*0.7) * p.cfg.RiseSpeed * 0.08
		l.Row += l.VRow + bob
		l.Col += l.VCol + wind*l.WindResponse + math.Sin(l.Phase)*p.cfg.Sway*0.035
		l.VCol += math.Sin(float64(p.tick)*0.012+l.Phase*0.35) * p.cfg.Sway * 0.0009
		if l.VCol > 0.11 {
			l.VCol = 0.11
		}
		if l.VCol < -0.11 {
			l.VCol = -0.11
		}
		for l.Col < -l.Width {
			l.Col += float64(p.W) + l.Width*2
		}
		for l.Col >= float64(p.W)+l.Width {
			l.Col -= float64(p.W) + l.Width*2
		}
		if l.Row < -l.Height-2 || p.lanternFadeLocked(l) <= 0.02 {
			p.appendLog("lantern-fade", "faded at altitude")
			continue
		}
		dst = append(dst, l)
	}
	p.lanterns = dst
}

func (p *PaperLanterns) startIntroLocked() {
	p.lanterns = nil
	p.ended = false
	p.endingTicks = 0
	p.endingTotal = 0
	p.pendingPulse = 0
	p.pulseTicks = 0
	p.quietTicks = 0
	p.windShiftTicks = 0
	p.windBias = 0
	p.introFirstFired = false
	p.introClusterFired = false
	total := p.cfg.IntroClusterDelay + max(1, p.cfg.PulseWindow) + 60
	if total < p.cfg.IntroFirstDelay+30 {
		total = p.cfg.IntroFirstDelay + 30
	}
	p.introTotal = total
	p.introTicks = total
	p.appendLog("intro", fmt.Sprintf("started (first=%d, cluster=%d)", p.cfg.IntroFirstDelay, p.cfg.IntroClusterDelay))
}

func (p *PaperLanterns) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.introFirstFired = false
	p.introClusterFired = false
	p.pendingPulse = 0
	p.pulseTicks = 0
	p.releaseTicks = p.nextReleaseTicksLocked()
	p.quietTicks = max(p.quietTicks, p.cfg.QuietGapDur)
	p.ended = false
	p.endingTotal = max(1, p.cfg.EndingTail)
	p.endingTicks = p.endingTotal
	p.appendLog("ending", fmt.Sprintf("started (stop=%d, tail=%d)", p.cfg.EndingStop, p.cfg.EndingTail))
}

func (p *PaperLanterns) startReleasePulseLocked(verb string) {
	count := p.cfg.ClusterMin
	if p.cfg.ClusterMax > p.cfg.ClusterMin {
		count += p.rng.Intn(p.cfg.ClusterMax - p.cfg.ClusterMin + 1)
	}
	p.pendingPulse += count
	p.pulseTicks = max(p.pulseTicks, p.cfg.PulseWindow)
	p.releaseTicks = p.nextReleaseTicksLocked()
	p.quietTicks = max(p.quietTicks, jitterInt(p.rng, p.cfg.QuietGapDur, 0.25))
	p.appendLog("release-pulse", fmt.Sprintf("%s (lanterns=%d, window=%d)", verb, count, p.cfg.PulseWindow))
}

func (p *PaperLanterns) startWindDriftLocked(verb string) {
	p.windShiftTicks = jitterInt(p.rng, p.cfg.WindShiftDur, 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.windBias = sign * p.cfg.WindShiftStr * (0.65 + p.rng.Float64()*0.55)
	p.appendLog("wind-drift", fmt.Sprintf("%s (dur=%d, bias=%+.3f)", verb, p.windShiftTicks, p.windBias))
}

func (p *PaperLanterns) startQuietGapLocked(verb string) {
	p.quietTicks = jitterInt(p.rng, p.cfg.QuietGapDur, 0.3)
	p.releaseTicks = max(p.releaseTicks, p.quietTicks)
	p.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d)", verb, p.quietTicks))
}

func (p *PaperLanterns) fadeHighestLanternLocked(verb string) {
	if len(p.lanterns) == 0 {
		return
	}
	best := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Row < p.lanterns[best].Row {
			best = i
		}
	}
	p.lanterns[best].Row = math.Min(p.lanterns[best].Row, float64(p.H)*p.cfg.FadeStart*0.35)
	p.appendLog("lantern-fade", verb)
}

func (p *PaperLanterns) spawnLanternLocked() bool {
	if p.W <= 0 || p.H <= 0 || len(p.lanterns) >= p.cfg.MaxLanterns {
		return false
	}
	size := p.cfg.Size * (0.85 + p.rng.Float64()*0.35)
	width := math.Max(2, math.Round(3*size))
	height := math.Max(3, math.Round(5*size))
	speed := p.cfg.RiseSpeed * (1 + p.cfg.RiseJitter*(p.rng.Float64()*2-1))
	if speed < 0.02 {
		speed = 0.02
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	colPad := width + 2
	col := colPad + p.rng.Float64()*math.Max(1, float64(p.W)-colPad*2)
	p.lanterns = append(p.lanterns, paperLantern{
		Row:          float64(p.H) + height + p.rng.Float64()*4,
		Col:          col,
		VRow:         -speed,
		VCol:         (p.rng.Float64()*2 - 1) * 0.025,
		Width:        width,
		Height:       height,
		Hue:          hue,
		Saturation:   p.cfg.Saturation,
		Lightness:    light,
		Phase:        p.rng.Float64() * 2 * math.Pi,
		WobbleRate:   0.025 + p.rng.Float64()*0.035,
		WindResponse: 0.55 + p.rng.Float64()*0.8,
	})
	return true
}

func (p *PaperLanterns) nextReleaseTicksLocked() int {
	return max(1, jitterInt(p.rng, p.cfg.ReleaseInterval, p.cfg.ReleaseJitter))
}

func (p *PaperLanterns) currentWindLocked() float64 {
	return p.cfg.WindDrift + p.windBias + math.Sin(float64(p.tick)*0.004)*p.cfg.Sway*0.01
}

func (p *PaperLanterns) lanternFadeLocked(l paperLantern) float64 {
	fadeStart := math.Max(1, float64(p.H)*p.cfg.FadeStart)
	if l.Row >= fadeStart {
		return 1
	}
	return clamp01(l.Row / fadeStart)
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	return PaperLanternsState{
		Tick:              p.tick,
		ReleaseTicks:      p.releaseTicks,
		QuietTicks:        p.quietTicks,
		PulseTicks:        p.pulseTicks,
		PendingPulse:      p.pendingPulse,
		WindShiftTicks:    p.windShiftTicks,
		WindBias:          p.windBias,
		IntroTicks:        p.introTicks,
		IntroTotal:        p.introTotal,
		IntroFirstFired:   p.introFirstFired,
		IntroClusterFired: p.introClusterFired,
		EndingTicks:       p.endingTicks,
		EndingTotal:       p.endingTotal,
		Ended:             p.ended,
		Lifecycle:         p.lifecycleLocked(),
	}
}

func (p *PaperLanterns) lifecycleLocked() Lifecycle {
	switch {
	case p.introTicks > 0:
		return LifecycleIntro
	case p.endingTicks > 0:
		return LifecycleEnding
	case p.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.releaseTicks = s.ReleaseTicks
	p.quietTicks = s.QuietTicks
	p.pulseTicks = s.PulseTicks
	p.pendingPulse = s.PendingPulse
	p.windShiftTicks = s.WindShiftTicks
	p.windBias = s.WindBias
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.introFirstFired = s.IntroFirstFired
	p.introClusterFired = s.IntroClusterFired
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.ended = s.Ended
	if p.releaseTicks <= 0 && !p.ended && p.introTicks <= 0 && p.endingTicks <= 0 {
		p.releaseTicks = p.nextReleaseTicksLocked()
	}
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			Row:          l.Row,
			Col:          l.Col,
			VRow:         l.VRow,
			VCol:         l.VCol,
			Width:        l.Width,
			Height:       l.Height,
			Hue:          l.Hue,
			Saturation:   l.Saturation,
			Lightness:    l.Lightness,
			Phase:        l.Phase,
			WobbleRate:   l.WobbleRate,
			WindResponse: l.WindResponse,
			Age:          l.Age,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			Row:          l.Row,
			Col:          l.Col,
			VRow:         l.VRow,
			VCol:         l.VCol,
			Width:        l.Width,
			Height:       l.Height,
			Hue:          l.Hue,
			Saturation:   l.Saturation,
			Lightness:    l.Lightness,
			Phase:        l.Phase,
			WobbleRate:   l.WobbleRate,
			WindResponse: l.WindResponse,
			Age:          l.Age,
		}
	}
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *PaperLanterns) paintFrameLocked() {
	p.paintSkyLocked()
	if p.ended {
		return
	}
	for _, l := range p.lanterns {
		p.paintLanternLocked(l)
	}
}

func (p *PaperLanterns) paintSkyLocked() {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	skyHue := math.Mod(p.cfg.Hue+220, 360)
	for y := 0; y < p.H; y++ {
		t := 0.0
		if p.H > 1 {
			t = float64(y) / float64(p.H-1)
		}
		c := hslToRGB(skyHue, 0.16, 0.025+0.045*t)
		for x := 0; x < p.W; x++ {
			p.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
	stars := max(8, p.W*p.H/1100)
	for i := 0; i < stars; i++ {
		x := (i*73 + 17) % max(1, p.W)
		yLimit := max(1, p.H*2/3)
		y := (i*41 + 11) % yLimit
		if ((x*31 + y*17 + i*13) % 7) == 0 {
			c := hslToRGB(math.Mod(p.cfg.Hue+30, 360), 0.18, 0.34)
			p.blendPixelLocked(x, y, c, 0.32)
		}
	}
}

func (p *PaperLanterns) paintLanternLocked(l paperLantern) {
	fade := p.lanternFadeLocked(l)
	if fade <= 0 {
		return
	}
	x0 := int(math.Round(l.Col - l.Width/2))
	y0 := int(math.Round(l.Row - l.Height/2))
	w := max(2, int(math.Round(l.Width)))
	h := max(3, int(math.Round(l.Height)))
	cx := x0 + w/2
	cy := y0 + h/2

	glowC := hslToRGB(l.Hue, clamp01(l.Saturation*0.95), clamp01(p.cfg.LightnessMax))
	rx := max(2, int(math.Round(l.Width*(1.8+p.cfg.Glow))))
	ry := max(2, int(math.Round(l.Height*(1.35+p.cfg.Glow*0.7))))
	for yy := cy - ry; yy <= cy+ry; yy++ {
		for xx := cx - rx; xx <= cx+rx; xx++ {
			dx := float64(xx-cx) / math.Max(1, float64(rx))
			dy := float64(yy-cy) / math.Max(1, float64(ry))
			d := dx*dx + dy*dy
			if d > 1 {
				continue
			}
			alpha := fade * p.cfg.Glow * 0.34 * (1 - d)
			p.blendPixelLocked(xx, yy, glowC, alpha)
		}
	}

	body := hslToRGB(l.Hue, clamp01(l.Saturation*0.55), clamp01(l.Lightness))
	edge := hslToRGB(math.Mod(l.Hue-5+360, 360), clamp01(l.Saturation*0.66), clamp01(l.Lightness*0.72))
	highlight := hslToRGB(math.Mod(l.Hue+4, 360), clamp01(l.Saturation*0.42), clamp01(p.cfg.LightnessMax*0.96))
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			alpha := fade * 0.84
			c := body
			if yy == 0 || yy == h-1 || xx == 0 || xx == w-1 {
				c = edge
				alpha = fade * 0.9
			}
			if xx == w/2 && yy > 0 && yy < h-1 {
				c = highlight
				alpha = fade * 0.78
			}
			p.blendPixelLocked(x0+xx, y0+yy, c, alpha)
		}
	}

	flame := hslToRGB(math.Mod(l.Hue-18+360, 360), 1, clamp01(p.cfg.LightnessMax))
	core := color.RGBA{R: 255, G: 238, B: 178, A: 255}
	p.blendPixelLocked(cx, y0+h-2, flame, fade)
	p.blendPixelLocked(cx, y0+h/2, core, fade*0.72)
	if h >= 5 {
		p.blendPixelLocked(cx, y0+h-3, core, fade*0.58)
	}
}

func (p *PaperLanterns) blendPixelLocked(x, y int, c color.RGBA, alpha float64) {
	if x < 0 || y < 0 || x >= p.W || y >= p.H {
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
