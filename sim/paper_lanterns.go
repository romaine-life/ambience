package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type paperLantern struct {
	Row, Col       float64
	VRow, VCol     float64
	Color          color.RGBA
	Phase          float64
	Sway           float64
	Size           float64
	Background     bool
	FadeTicks      int
	FadeTotalTicks int
}

// PaperLanternsConfig tunes a calm sky-lantern release. Zero values are
// normalized by withDefaults so schema defaults, partial scene configs, and
// persisted restores all converge on the same effective config.
type PaperLanternsConfig struct {
	// INTRODUCTION
	IntroDur          int `json:"intro_dur"`
	IntroFirstDelay   int `json:"intro_first"`
	IntroClusterDelay int `json:"intro_cluster"`
	// ENDING
	EndingStop int `json:"ending_stop"`
	EndingTail int `json:"ending_tail"`
	// SPAWN/RHYTHM
	LoneEvery     int `json:"lone_every"`
	ReleaseGap    int `json:"release_gap"`
	MaxLanterns   int `json:"max"`
	ReleaseMin    int `json:"release_min"`
	ReleaseMax    int `json:"release_max"`
	ReleaseWindow int `json:"release_window"`
	// MOTION
	RiseSpeed   float64 `json:"rise"`
	SpeedJitter float64 `json:"rise_jit"`
	Wind        float64 `json:"wind"`
	WindJitter  float64 `json:"wind_jit"`
	Sway        float64 `json:"sway"`
	// SHAPE/FADE
	Size      float64 `json:"size"`
	FadeStart float64 `json:"fade_start"`
	FadeDur   int     `json:"fade_dur"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// DEPTH
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// EVENT CHANCES
	EmitChance      float64 `json:"emit_p"`
	ReleaseChance   float64 `json:"release_p"`
	WindDriftChance float64 `json:"wind_drift_p"`
	FadeChance      float64 `json:"fade_p"`
	QuietGapChance  float64 `json:"quiet_gap_p"`
	// EVENT MODIFIERS
	WindDriftDur      int     `json:"wind_drift_dur"`
	WindDriftStrength float64 `json:"wind_drift_str"`
	QuietGapDur       int     `json:"quiet_gap_dur"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 190
	}
	if c.IntroFirstDelay < 0 {
		c.IntroFirstDelay = 0
	}
	if c.IntroFirstDelay == 0 {
		c.IntroFirstDelay = 24
	}
	if c.IntroClusterDelay <= c.IntroFirstDelay {
		c.IntroClusterDelay = 105
	}
	if c.EndingStop < 0 {
		c.EndingStop = 0
	}
	if c.EndingTail <= 0 {
		c.EndingTail = 260
	}
	if c.LoneEvery <= 0 {
		c.LoneEvery = 95
	}
	if c.ReleaseGap <= 0 {
		c.ReleaseGap = 420
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 80
	}
	if c.ReleaseMin <= 0 {
		c.ReleaseMin = 5
	}
	if c.ReleaseMax <= 0 {
		c.ReleaseMax = 9
	}
	if c.ReleaseMax < c.ReleaseMin {
		c.ReleaseMin, c.ReleaseMax = c.ReleaseMax, c.ReleaseMin
	}
	if c.ReleaseWindow <= 0 {
		c.ReleaseWindow = 24
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.076
	}
	if c.Wind == 0 {
		c.Wind = 0.18
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
		c.WindJitter = 0.22
	}
	if c.Sway <= 0 {
		c.Sway = 0.55
	}
	if c.Size <= 0 {
		c.Size = 1.45
	}
	if c.FadeStart <= 0 {
		c.FadeStart = 0.28
	}
	if c.FadeStart > 0.9 {
		c.FadeStart = 0.9
	}
	if c.FadeDur <= 0 {
		c.FadeDur = 95
	}
	if c.Hue == 0 {
		c.Hue = 36
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.HueSpread == 0 {
		c.HueSpread = 11
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.76
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.46
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.78
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.Layers <= 0 {
		c.Layers = 2
	}
	if c.LayerBalance <= 0 {
		c.LayerBalance = 0.42
	}
	if c.WindDriftDur <= 0 {
		c.WindDriftDur = 210
	}
	if c.WindDriftStrength <= 0 {
		c.WindDriftStrength = 0.55
	}
	if c.QuietGapDur <= 0 {
		c.QuietGapDur = 300
	}
	return c
}

// PaperLanternsSchema describes the Paper Lanterns effect's tunable knobs.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name:           "paper-lanterns",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 40, Max: 480, Step: 10, Default: 190,
				Description: "Ticks spent from dark sky through the first cluster. Fire intro to preview."},
			{Key: "intro_first", Label: "first delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24, Trigger: "intro",
				Description: "Delay before the first lone lantern launches during the intro."},
			{Key: "intro_cluster", Label: "cluster delay", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 360, Step: 5, Default: 105,
				Description: "Delay before the intro release pulse establishes the festival rhythm."},
			{Key: "ending_stop", Label: "stop delay", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 0, Trigger: "ending",
				Description: "Ticks after ending fires before automatic releases stop."},
			{Key: "ending_tail", Label: "tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 60, Max: 900, Step: 15, Default: 260,
				Description: "Ticks for in-flight lanterns to rise and fade before the sky holds dark."},
			{Key: "lone_every", Label: "lone every", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 25, Max: 420, Step: 5, Default: 95,
				Description: "Average ticks between lone drifters outside cluster releases."},
			{Key: "release_gap", Label: "cluster gap", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 90, Max: 1200, Step: 15, Default: 420,
				Description: "Average ticks between scheduled release-pulse clusters."},
			{Key: "max", Label: "max lanterns", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 8, Max: 180, Step: 1, Default: 80,
				Description: "Maximum active lantern count."},
			{Key: "release_min", Label: "cluster min", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 5,
				Description: "Minimum lanterns queued by a release-pulse event."},
			{Key: "release_max", Label: "cluster max", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 2, Max: 20, Step: 1, Default: 9,
				Description: "Maximum lanterns queued by a release-pulse event."},
			{Key: "release_window", Label: "release win", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 90, Step: 1, Default: 24,
				Description: "Ticks over which a cluster is emitted instead of appearing at once."},
			{Key: "rise", Label: "rise speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.025, Max: 0.22, Step: 0.005, Default: 0.076,
				Description: "Rows per tick each lantern rises. Lower values read more buoyant."},
			{Key: "rise_jit", Label: "rise jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.02, Default: 0.28,
				Description: "Per-lantern speed variation at launch."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -1.5, Max: 1.5, Step: 0.05, Default: 0.18,
				Description: "Global horizontal drift bias. Negative bends left, positive bends right."},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 1.0, Step: 0.05, Default: 0.22,
				Description: "Per-lantern horizontal drift variation at launch."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1.8, Step: 0.05, Default: 0.55,
				Description: "Slow side-to-side breathing in each lantern path."},
			{Key: "size", Label: "size", Slot: SlotSpawn, Group: "shape", Type: KnobFloat, Min: 0.7, Max: 3.2, Step: 0.1, Default: 1.45,
				Description: "Lantern body size in low-resolution pixels."},
			{Key: "fade_start", Label: "fade top", Slot: SlotEnd, Group: "shape", Type: KnobFloat, Min: 0.08, Max: 0.65, Step: 0.01, Default: 0.28,
				Description: "Top-screen fraction where natural altitude fade begins."},
			{Key: "fade_dur", Label: "fade dur", Slot: SlotEventMod, Group: "lantern-fade", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 95,
				Description: "Ticks for a manually faded lantern to dim out."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 18, Max: 58, Step: 1, Default: 36,
				Description: "Lantern palette hue. The main scene knob for warm paper colors."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 11,
				Description: "Per-lantern hue variation across the warm palette."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 1, Step: 0.01, Default: 0.76,
				Description: "Paper and glow color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 0.8, Step: 0.01, Default: 0.46,
				Description: "Dim end of the lantern paper palette."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Bright end of the lantern paper palette."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 2,
				Description: "1 = single field. 2 = dimmer smaller background lanterns."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.05, Default: 0.42,
				Description: "Fraction of newly launched lanterns assigned to the background layer."},
			{Key: "emit_p", Label: "lantern emit", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lantern-emit",
				Description: "Per-tick chance of an extra single-lantern launch."},
			{Key: "release_p", Label: "release pulse", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.00025, Default: 0, Trigger: "release-pulse",
				Description: "Per-tick chance of an extra cluster release."},
			{Key: "wind_drift_p", Label: "wind drift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.00025, Default: 0, Trigger: "wind-drift",
				Description: "Per-tick chance of a temporary global drift-bias shift."},
			{Key: "fade_p", Label: "lantern fade", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lantern-fade",
				Description: "Per-tick chance to dim the highest in-flight lantern."},
			{Key: "quiet_gap_p", Label: "quiet gap", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.00025, Default: 0, Trigger: "quiet-gap",
				Description: "Per-tick chance of a temporary suppression window between releases."},
			{Key: "wind_drift_dur", Label: "drift dur", Slot: SlotEventMod, Group: "wind-drift", Type: KnobInt, Min: 30, Max: 600, Step: 10, Default: 210,
				Description: "Duration of a wind-drift bias shift."},
			{Key: "wind_drift_str", Label: "drift str", Slot: SlotEventMod, Group: "wind-drift", Type: KnobFloat, Min: 0.05, Max: 2.0, Step: 0.05, Default: 0.55,
				Description: "Magnitude of the temporary wind-drift bias."},
			{Key: "quiet_gap_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 60, Max: 1200, Step: 15, Default: 300,
				Description: "Duration of the quiet-gap release suppression window."},
		},
	}
}

type PaperLanternsState struct {
	Tick                 int       `json:"tick"`
	IntroTicks           int       `json:"introTicks"`
	IntroTotal           int       `json:"introTotal"`
	IntroFirstLaunched   bool      `json:"introFirstLaunched"`
	IntroClusterLaunched bool      `json:"introClusterLaunched"`
	EndingTicks          int       `json:"endingTicks"`
	EndingTotal          int       `json:"endingTotal"`
	Ended                bool      `json:"ended"`
	QuietGapTicks        int       `json:"quietGapTicks"`
	WindDriftTicks       int       `json:"windDriftTicks"`
	WindShift            float64   `json:"windShift"`
	ReleaseQueue         int       `json:"releaseQueue"`
	ReleaseWindowTicks   int       `json:"releaseWindowTicks"`
	NextLoneIn           int       `json:"nextLoneIn"`
	NextReleaseIn        int       `json:"nextReleaseIn"`
	Lifecycle            Lifecycle `json:"lifecycle"`
}

type PaperLantern struct {
	Row            float64 `json:"row"`
	Col            float64 `json:"col"`
	VRow           float64 `json:"vRow"`
	VCol           float64 `json:"vCol"`
	Color          RGB     `json:"color"`
	Phase          float64 `json:"phase"`
	Sway           float64 `json:"sway"`
	Size           float64 `json:"size"`
	Background     bool    `json:"background"`
	FadeTicks      int     `json:"fadeTicks,omitempty"`
	FadeTotalTicks int     `json:"fadeTotalTicks,omitempty"`
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

// PaperLanterns is a slow, buoyant pixel-grid sky-lantern release.
type PaperLanterns struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	introTicks           int
	introTotal           int
	introFirstLaunched   bool
	introClusterLaunched bool
	endingTicks          int
	endingTotal          int
	ended                bool

	quietGapTicks      int
	windDriftTicks     int
	windShift          float64
	releaseQueue       int
	releaseWindowTicks int
	nextLoneIn         int
	nextReleaseIn      int

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	c := cfg.withDefaults()
	return &PaperLanterns{
		W:             w,
		H:             h,
		Grid:          grid,
		rng:           rngutil.New(seed),
		cfg:           c,
		nextLoneIn:    1,
		nextReleaseIn: max(20, c.ReleaseGap/4),
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
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, w)
	}
	p.lanterns = p.lanterns[:0]
}

func (p *PaperLanterns) SetConfig(cfg PaperLanternsConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg.withDefaults()
	if p.nextLoneIn <= 0 {
		p.nextLoneIn = p.jitterTicksLocked(p.cfg.LoneEvery, 0.25)
	}
	if p.nextReleaseIn <= 0 {
		p.nextReleaseIn = p.jitterTicksLocked(p.cfg.ReleaseGap, 0.25)
	}
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

func (p *PaperLanterns) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "intro":
		p.startIntroLocked()
	case "ending":
		p.startEndingLocked()
	case "lantern-emit":
		ok := p.spawnLanternLocked(false)
		if ok {
			p.appendLog("lantern-emit", "single lantern launched")
		} else {
			p.appendLog("lantern-emit", "launch skipped at cap")
		}
	case "release-pulse":
		p.quietGapTicks = 0
		count := p.startReleasePulseLocked("triggered")
		p.appendLog("release-pulse", fmt.Sprintf("cluster queued (%d lanterns)", count))
	case "wind-drift":
		p.startWindDriftLocked("triggered")
	case "lantern-fade":
		if p.fadeHighestLanternLocked() {
			p.appendLog("lantern-fade", "highest lantern dimming")
		} else {
			p.appendLog("lantern-fade", "no lantern to fade")
		}
	case "quiet-gap":
		p.quietGapTicks = p.jitterTicksLocked(p.cfg.QuietGapDur, 0.25)
		p.releaseQueue = 0
		p.releaseWindowTicks = 0
		p.appendLog("quiet-gap", fmt.Sprintf("release suppression (%d ticks)", p.quietGapTicks))
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
	if p.quietGapTicks > 0 {
		p.quietGapTicks--
	}
	if p.windDriftTicks > 0 {
		p.windDriftTicks--
		if p.windDriftTicks == 0 {
			p.windShift = 0
		}
	}

	p.advanceLifecycleLocked()
	p.rollChanceEventsLocked()
	p.emitQueuedReleaseLocked()
	if p.autoSpawnsAllowedLocked() {
		p.advanceSchedulersLocked()
	}
	p.stepLanternsLocked()
	p.paintFrameLocked()
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

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	return PaperLanternsState{
		Tick:                 p.tick,
		IntroTicks:           p.introTicks,
		IntroTotal:           p.introTotal,
		IntroFirstLaunched:   p.introFirstLaunched,
		IntroClusterLaunched: p.introClusterLaunched,
		EndingTicks:          p.endingTicks,
		EndingTotal:          p.endingTotal,
		Ended:                p.ended,
		QuietGapTicks:        p.quietGapTicks,
		WindDriftTicks:       p.windDriftTicks,
		WindShift:            p.windShift,
		ReleaseQueue:         p.releaseQueue,
		ReleaseWindowTicks:   p.releaseWindowTicks,
		NextLoneIn:           p.nextLoneIn,
		NextReleaseIn:        p.nextReleaseIn,
		Lifecycle:            p.lifecycleLocked(),
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.introFirstLaunched = s.IntroFirstLaunched
	p.introClusterLaunched = s.IntroClusterLaunched
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.ended = s.Ended
	p.quietGapTicks = s.QuietGapTicks
	p.windDriftTicks = s.WindDriftTicks
	p.windShift = s.WindShift
	p.releaseQueue = s.ReleaseQueue
	p.releaseWindowTicks = s.ReleaseWindowTicks
	p.nextLoneIn = s.NextLoneIn
	p.nextReleaseIn = s.NextReleaseIn
	if p.nextLoneIn <= 0 {
		p.nextLoneIn = p.jitterTicksLocked(p.cfg.LoneEvery, 0.25)
	}
	if p.nextReleaseIn <= 0 {
		p.nextReleaseIn = p.jitterTicksLocked(p.cfg.ReleaseGap, 0.25)
	}
}

func (p *PaperLanterns) lifecycleLocked() Lifecycle {
	switch {
	case p.ended:
		return LifecycleEnded
	case p.endingTicks > 0:
		return LifecycleEnding
	case p.introTicks > 0:
		return LifecycleIntro
	default:
		return LifecycleRunning
	}
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			Row:            l.Row,
			Col:            l.Col,
			VRow:           l.VRow,
			VCol:           l.VCol,
			Color:          RGB{R: l.Color.R, G: l.Color.G, B: l.Color.B},
			Phase:          l.Phase,
			Sway:           l.Sway,
			Size:           l.Size,
			Background:     l.Background,
			FadeTicks:      l.FadeTicks,
			FadeTotalTicks: l.FadeTotalTicks,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			Row:            l.Row,
			Col:            l.Col,
			VRow:           l.VRow,
			VCol:           l.VCol,
			Color:          color.RGBA{R: l.Color.R, G: l.Color.G, B: l.Color.B, A: 255},
			Phase:          l.Phase,
			Sway:           l.Sway,
			Size:           l.Size,
			Background:     l.Background,
			FadeTicks:      l.FadeTicks,
			FadeTotalTicks: l.FadeTotalTicks,
		}
	}
}

func (p *PaperLanterns) startIntroLocked() {
	p.ended = false
	p.endingTicks = 0
	p.endingTotal = 0
	p.introTotal = p.cfg.IntroDur
	p.introTicks = p.introTotal
	p.introFirstLaunched = false
	p.introClusterLaunched = false
	p.quietGapTicks = 0
	p.releaseQueue = 0
	p.releaseWindowTicks = 0
	p.lanterns = p.lanterns[:0]
	p.appendLog("intro", fmt.Sprintf("first=%d cluster=%d", p.cfg.IntroFirstDelay, p.cfg.IntroClusterDelay))
}

func (p *PaperLanterns) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.introFirstLaunched = false
	p.introClusterLaunched = false
	p.ended = false
	p.endingTotal = p.cfg.EndingStop + p.cfg.EndingTail
	if p.endingTotal <= 0 {
		p.endingTotal = 1
	}
	p.endingTicks = p.endingTotal
	p.releaseQueue = 0
	p.releaseWindowTicks = 0
	p.appendLog("ending", fmt.Sprintf("stop=%d tail=%d", p.cfg.EndingStop, p.cfg.EndingTail))
}

func (p *PaperLanterns) advanceLifecycleLocked() {
	if p.introTicks > 0 {
		elapsed := p.introTotal - p.introTicks
		if !p.introFirstLaunched && elapsed >= p.cfg.IntroFirstDelay {
			p.spawnLanternLocked(true)
			p.introFirstLaunched = true
		}
		if !p.introClusterLaunched && elapsed >= p.cfg.IntroClusterDelay {
			p.startReleasePulseLocked("intro")
			p.introClusterLaunched = true
		}
		p.introTicks--
		if p.introTicks == 0 {
			p.nextLoneIn = p.jitterTicksLocked(p.cfg.LoneEvery, 0.2)
			p.nextReleaseIn = p.jitterTicksLocked(p.cfg.ReleaseGap, 0.25)
		}
	}
	if p.endingTicks > 0 {
		p.endingTicks--
		if p.endingTicks == 0 {
			p.ended = true
			p.releaseQueue = 0
			p.releaseWindowTicks = 0
			p.lanterns = p.lanterns[:0]
		}
	}
}

func (p *PaperLanterns) autoSpawnsAllowedLocked() bool {
	if p.ended || p.introTicks > 0 || p.quietGapTicks > 0 {
		return false
	}
	if p.endingTicks <= 0 {
		return true
	}
	elapsed := p.endingTotal - p.endingTicks
	return elapsed < p.cfg.EndingStop
}

func (p *PaperLanterns) rollChanceEventsLocked() {
	if p.ended {
		return
	}
	if p.autoSpawnsAllowedLocked() && p.cfg.EmitChance > 0 && p.rng.Float64() < p.cfg.EmitChance {
		p.spawnLanternLocked(false)
		p.appendLog("lantern-emit", "chance launch")
	}
	if p.autoSpawnsAllowedLocked() && p.cfg.ReleaseChance > 0 && p.rng.Float64() < p.cfg.ReleaseChance {
		count := p.startReleasePulseLocked("chance")
		p.appendLog("release-pulse", fmt.Sprintf("chance cluster (%d lanterns)", count))
	}
	if p.windDriftTicks == 0 && p.cfg.WindDriftChance > 0 && p.rng.Float64() < p.cfg.WindDriftChance {
		p.startWindDriftLocked("chance")
	}
	if len(p.lanterns) > 0 && p.cfg.FadeChance > 0 && p.rng.Float64() < p.cfg.FadeChance {
		if p.fadeHighestLanternLocked() {
			p.appendLog("lantern-fade", "chance fade")
		}
	}
	if p.quietGapTicks == 0 && p.cfg.QuietGapChance > 0 && p.rng.Float64() < p.cfg.QuietGapChance {
		p.quietGapTicks = p.jitterTicksLocked(p.cfg.QuietGapDur, 0.25)
		p.releaseQueue = 0
		p.releaseWindowTicks = 0
		p.appendLog("quiet-gap", fmt.Sprintf("chance quiet (%d ticks)", p.quietGapTicks))
	}
}

func (p *PaperLanterns) advanceSchedulersLocked() {
	p.nextLoneIn--
	if p.nextLoneIn <= 0 {
		p.spawnLanternLocked(false)
		p.nextLoneIn = p.jitterTicksLocked(p.cfg.LoneEvery, 0.35)
	}
	p.nextReleaseIn--
	if p.nextReleaseIn <= 0 {
		p.startReleasePulseLocked("scheduled")
		p.nextReleaseIn = p.jitterTicksLocked(p.cfg.ReleaseGap, 0.28)
	}
}

func (p *PaperLanterns) emitQueuedReleaseLocked() {
	if p.releaseQueue <= 0 {
		p.releaseQueue = 0
		p.releaseWindowTicks = 0
		return
	}
	if p.ended || p.quietGapTicks > 0 {
		return
	}
	window := max(1, p.releaseWindowTicks)
	perTick := int(math.Ceil(float64(p.releaseQueue) / float64(window)))
	for i := 0; i < perTick && p.releaseQueue > 0; i++ {
		p.spawnLanternLocked(true)
		p.releaseQueue--
	}
	if p.releaseWindowTicks > 0 {
		p.releaseWindowTicks--
	}
}

func (p *PaperLanterns) startReleasePulseLocked(reason string) int {
	span := p.cfg.ReleaseMax - p.cfg.ReleaseMin + 1
	count := p.cfg.ReleaseMin
	if span > 1 {
		count += p.rng.Intn(span)
	}
	p.releaseQueue += count
	p.releaseWindowTicks = max(p.releaseWindowTicks, p.cfg.ReleaseWindow)
	if reason != "scheduled" && reason != "intro" {
		p.nextReleaseIn = p.jitterTicksLocked(p.cfg.ReleaseGap, 0.25)
	}
	return count
}

func (p *PaperLanterns) startWindDriftLocked(reason string) {
	sign := 1.0
	if p.rng.Intn(2) == 0 {
		sign = -1
	}
	p.windShift = sign * p.cfg.WindDriftStrength * (0.55 + p.rng.Float64()*0.9)
	p.windDriftTicks = p.jitterTicksLocked(p.cfg.WindDriftDur, 0.25)
	p.appendLog("wind-drift", fmt.Sprintf("%s (dur=%d, bias=%+.2f)", reason, p.windDriftTicks, p.windShift))
}

func (p *PaperLanterns) fadeHighestLanternLocked() bool {
	if len(p.lanterns) == 0 {
		return false
	}
	idx := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Row < p.lanterns[idx].Row {
			idx = i
		}
	}
	p.lanterns[idx].FadeTotalTicks = p.cfg.FadeDur
	p.lanterns[idx].FadeTicks = p.cfg.FadeDur
	return true
}

func (p *PaperLanterns) spawnLanternLocked(cluster bool) bool {
	if len(p.lanterns) >= p.cfg.MaxLanterns || p.W <= 0 || p.H <= 0 {
		return false
	}
	background := p.cfg.Layers >= 2 && p.rng.Float64() < p.cfg.LayerBalance
	speed := p.cfg.RiseSpeed * (1 + (p.rng.Float64()*2-1)*p.cfg.SpeedJitter)
	if speed < p.cfg.RiseSpeed*0.35 {
		speed = p.cfg.RiseSpeed * 0.35
	}
	if background {
		speed *= 0.72
	}
	wind := (p.cfg.Wind + p.windShift) * 0.045
	wind += (p.rng.Float64()*2 - 1) * p.cfg.WindJitter * 0.045
	if cluster {
		wind += (p.rng.Float64()*2 - 1) * 0.018
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	if background {
		light *= 0.82
	}
	body := hslToRGB(hue, clamp01(p.cfg.Saturation), clamp01(light))
	size := p.cfg.Size * (0.85 + p.rng.Float64()*0.3)
	if background {
		size *= 0.78
	}
	row := float64(p.H-1) + p.rng.Float64()*2
	col := p.rng.Float64() * float64(p.W)
	p.lanterns = append(p.lanterns, paperLantern{
		Row:        row,
		Col:        col,
		VRow:       -speed,
		VCol:       wind,
		Color:      body,
		Phase:      p.rng.Float64() * 2 * math.Pi,
		Sway:       0.55 + p.rng.Float64()*0.9,
		Size:       size,
		Background: background,
	})
	return true
}

func (p *PaperLanterns) stepLanternsLocked() {
	wind := (p.cfg.Wind + p.windShift) * 0.002
	out := p.lanterns[:0]
	for i := range p.lanterns {
		l := p.lanterns[i]
		sway := math.Sin(l.Phase) * p.cfg.Sway * l.Sway * 0.025
		l.VCol += wind
		if l.VCol > 0.16 {
			l.VCol = 0.16
		} else if l.VCol < -0.16 {
			l.VCol = -0.16
		}
		l.Row += l.VRow
		l.Col += l.VCol + sway
		l.Phase += 0.025 + 0.018*l.Sway
		for l.Col < -2 {
			l.Col += float64(p.W + 4)
		}
		for l.Col >= float64(p.W+2) {
			l.Col -= float64(p.W + 4)
		}
		if l.FadeTicks > 0 {
			l.FadeTicks--
		}
		if l.FadeTotalTicks > 0 && l.FadeTicks <= 0 {
			continue
		}
		if l.Row < -4 {
			continue
		}
		out = append(out, l)
	}
	p.lanterns = out
}

func (p *PaperLanterns) paintFrameLocked() {
	p.clearGridLocked()
	if p.ended {
		return
	}
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

func (p *PaperLanterns) paintLanternLocked(l paperLantern) {
	x := int(math.Round(l.Col))
	y := int(math.Round(l.Row))
	fade := p.altitudeFadeLocked(l.Row)
	if l.FadeTotalTicks > 0 {
		fade *= clamp01(float64(l.FadeTicks) / float64(max(1, l.FadeTotalTicks)))
	}
	if p.endingTicks > 0 && p.endingTotal > 0 {
		elapsed := p.endingTotal - p.endingTicks
		if elapsed >= p.cfg.EndingStop {
			tailElapsed := elapsed - p.cfg.EndingStop
			tail := max(1, p.cfg.EndingTail)
			fade *= clamp01(1 - float64(tailElapsed)/float64(tail))
		}
	}
	if l.Background {
		fade *= 0.72
	}
	if fade <= 0.03 {
		return
	}
	size := int(math.Round(l.Size))
	if size < 1 {
		size = 1
	}
	bodyW := max(1, size+1)
	bodyH := max(2, size+2)
	left := x - bodyW/2
	top := y - bodyH/2

	glow := scaleRGBA(l.Color, 0.18*fade)
	for gy := top - 1; gy <= top+bodyH; gy++ {
		for gx := left - 1; gx <= left+bodyW; gx++ {
			if gx >= left && gx < left+bodyW && gy >= top && gy < top+bodyH {
				continue
			}
			p.blendPixelLocked(gx, gy, glow)
		}
	}

	body := scaleRGBA(l.Color, fade)
	rim := scaleRGBA(l.Color, 0.62*fade)
	for gy := top; gy < top+bodyH; gy++ {
		for gx := left; gx < left+bodyW; gx++ {
			c := body
			if gx == left || gx == left+bodyW-1 || gy == top || gy == top+bodyH-1 {
				c = rim
			}
			p.blendPixelLocked(gx, gy, c)
		}
	}

	flamePulse := 0.72 + 0.28*math.Sin(l.Phase*2.7)
	flame := hslToRGB(32, 0.96, 0.78)
	p.blendPixelLocked(x, top+bodyH/2, scaleRGBA(flame, fade*flamePulse))
	if bodyH >= 4 {
		ember := hslToRGB(18, 0.95, 0.55)
		p.blendPixelLocked(x, top+bodyH/2+1, scaleRGBA(ember, fade*0.65))
	}
}

func (p *PaperLanterns) altitudeFadeLocked(row float64) float64 {
	if p.H <= 0 {
		return 0
	}
	topBand := math.Max(1, float64(p.H)*p.cfg.FadeStart)
	if row <= topBand {
		return clamp01(row / topBand)
	}
	if row > float64(p.H) {
		return clamp01(1 - (row-float64(p.H))*0.5)
	}
	return 1
}

func (p *PaperLanterns) blendPixelLocked(x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= p.W || y >= p.H {
		return
	}
	dst := &p.Grid[y][x]
	if !dst.Filled {
		dst.Filled = true
		dst.C = c
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

func scaleRGBA(c color.RGBA, f float64) color.RGBA {
	f = clamp01(f)
	return color.RGBA{
		R: uint8(float64(c.R) * f),
		G: uint8(float64(c.G) * f),
		B: uint8(float64(c.B) * f),
		A: 255,
	}
}

func (p *PaperLanterns) jitterTicksLocked(base int, spread float64) int {
	if base <= 1 {
		return 1
	}
	return jitterInt(p.rng, base, spread)
}
