package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	paperLanternsLifecycleRunning = "running"
	paperLanternsLifecycleIntro   = "intro"
	paperLanternsLifecycleEnding  = "ending"
	paperLanternsLifecycleEnded   = "ended"
)

type paperLantern struct {
	Row, Col    float64
	Rise, Drift float64
	Hue         float64
	Lightness   float64
	Phase       float64
	Width       int
	Height      int
	Age         int
	FadeTicks   int
	FadeLogged  bool
	Foreground  bool
}

// PaperLanternsConfig tunes the paper-lantern release effect. Zero values
// fall back to a calm festival cadence.
type PaperLanternsConfig struct {
	// LIFECYCLE
	IntroDelay        int `json:"intro_delay"`
	IntroClusterDelay int `json:"intro_cluster_delay"`
	EndingStopDelay   int `json:"ending_stop"`
	EndingTail        int `json:"tail_len"`
	// MOTION
	RiseSpeed   float64 `json:"rise"`
	SpeedJitter float64 `json:"speed_jit"`
	Wind        float64 `json:"wind"`
	WindShift   float64 `json:"wind_shift"`
	Wander      float64 `json:"wander"`
	// SPAWN
	LoneEmitChance float64 `json:"emit_p"`
	ReleaseChance  float64 `json:"release_p"`
	QuietChance    float64 `json:"quiet_gap_p"`
	MaxLanterns    int     `json:"max"`
	ClusterMin     int     `json:"cluster_min"`
	ClusterMax     int     `json:"cluster_max"`
	ReleaseSpacing int     `json:"release_spacing"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// END / EVENTS
	FadeAltitude int `json:"fade_alt"`
	FadeDur      int `json:"fade_dur"`
	WindDriftDur int `json:"wind_drift_dur"`
	QuietGapDur  int `json:"quiet_gap_dur"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroDelay <= 0 {
		c.IntroDelay = 28
	}
	if c.IntroClusterDelay <= 0 {
		c.IntroClusterDelay = 120
	}
	if c.IntroClusterDelay < c.IntroDelay {
		c.IntroClusterDelay = c.IntroDelay
	}
	if c.EndingStopDelay < 0 {
		c.EndingStopDelay = 0
	}
	if c.EndingTail <= 0 {
		c.EndingTail = 420
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.14
	}
	if c.SpeedJitter <= 0 {
		c.SpeedJitter = 0.22
	}
	c.SpeedJitter = clamp01(c.SpeedJitter)
	if c.WindShift <= 0 {
		c.WindShift = 0.055
	}
	if c.Wander <= 0 {
		c.Wander = 0.035
	}
	if c.LoneEmitChance <= 0 {
		c.LoneEmitChance = 0.018
	}
	if c.ReleaseChance <= 0 {
		c.ReleaseChance = 0.0025
	}
	if c.QuietChance <= 0 {
		c.QuietChance = 0.0008
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 60
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
	if c.ReleaseSpacing <= 0 {
		c.ReleaseSpacing = 4
	}
	if c.Hue == 0 {
		c.Hue = 36
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.74
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.42
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.88
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.FadeAltitude <= 0 {
		c.FadeAltitude = 18
	}
	if c.FadeDur <= 0 {
		c.FadeDur = 70
	}
	if c.WindDriftDur <= 0 {
		c.WindDriftDur = 180
	}
	if c.QuietGapDur <= 0 {
		c.QuietGapDur = 260
	}
	return c
}

// PaperLanternsSchema describes the paper-lantern release effect's tunable
// knobs for the dev UI.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name: "paper-lanterns",
		Knobs: []Knob{
			{Key: "intro_delay", Label: "first lantern", Slot: SlotSpawn, Group: "intro", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 28, Trigger: "intro",
				Description: "Ticks from a dark sky to the first lone lantern during intro."},
			{Key: "intro_cluster_delay", Label: "first cluster", Slot: SlotSpawn, Group: "intro", Type: KnobInt, Min: 20, Max: 300, Step: 5, Default: 120,
				Description: "Ticks from intro start to the first release pulse."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.06, Max: 0.32, Step: 0.01, Default: 0.14,
				Description: "Base vertical lift per tick. Lower values feel more buoyant."},
			{Key: "speed_jit", Label: "rise jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.22,
				Description: "Per-lantern rise speed variation."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.18, Max: 0.18, Step: 0.01, Default: 0,
				Description: "Global sideways drift. Positive values bend lanterns right."},
			{Key: "wind_shift", Label: "wind shift", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.18, Step: 0.01, Default: 0.055,
				Description: "Maximum temporary drift bias from wind-drift events."},
			{Key: "wander", Label: "wander", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.12, Step: 0.005, Default: 0.035,
				Description: "Small sinusoidal side-to-side lantern wobble."},
			{Key: "emit_p", Label: "lone emit", Slot: SlotEvent, Group: "release", Type: KnobFloat, Min: 0, Max: 0.08, Step: 0.001, Default: 0.018, Trigger: "lantern-emit",
				Description: "Per-tick chance of a lone lantern between release pulses."},
			{Key: "release_p", Label: "release pulse", Slot: SlotEvent, Group: "release", Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0.0025, Trigger: "release-pulse",
				Description: "Per-tick chance of a clustered lantern release."},
			{Key: "quiet_gap_p", Label: "quiet gap", Slot: SlotEvent, Group: "release", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0.0008, Trigger: "quiet-gap",
				Description: "Per-tick chance of suppressing new releases for a while."},
			{Key: "max", Label: "max lights", Slot: SlotLever, Group: "release", Type: KnobInt, Min: 8, Max: 140, Step: 1, Default: 60,
				Description: "Maximum in-flight lanterns."},
			{Key: "cluster_min", Label: "cluster min", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 2, Max: 16, Step: 1, Default: 5,
				Description: "Minimum lanterns queued by a release pulse."},
			{Key: "cluster_max", Label: "cluster max", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 3, Max: 24, Step: 1, Default: 9,
				Description: "Maximum lanterns queued by a release pulse."},
			{Key: "release_spacing", Label: "spacing", Slot: SlotEventMod, Group: "release-pulse", Type: KnobInt, Min: 1, Max: 14, Step: 1, Default: 4,
				Description: "Average ticks between lanterns inside a release pulse."},
			{Key: "wind_drift_dur", Label: "wind dur", Slot: SlotEventMod, Group: "wind-drift", Type: KnobInt, Min: 30, Max: 420, Step: 5, Default: 180, Trigger: "wind-drift",
				Description: "Duration of a temporary wind-drift bias."},
			{Key: "quiet_gap_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 60, Max: 720, Step: 10, Default: 260,
				Description: "Duration of a release-suppression window."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 20, Max: 58, Step: 1, Default: 36,
				Description: "Base paper-lantern hue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "palette", Type: KnobFloat, Min: 0, Max: 32, Step: 1, Default: 12,
				Description: "Warm color variation across spawned lanterns."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.25, Max: 1, Step: 0.01, Default: 0.74,
				Description: "Overall lantern color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.18, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Dim edge lightness for the paper body."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.5, Max: 0.98, Step: 0.01, Default: 0.88,
				Description: "Bright flame and paper-core lightness."},
			{Key: "fade_alt", Label: "fade altitude", Slot: SlotEnd, Group: "fade", Type: KnobInt, Min: 4, Max: 40, Step: 1, Default: 18, Trigger: "lantern-fade",
				Description: "Rows from the top where lanterns begin fading."},
			{Key: "fade_dur", Label: "fade dur", Slot: SlotEnd, Group: "fade", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 70,
				Description: "Manual lantern-fade duration."},
			{Key: "ending_stop", Label: "stop delay", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 0, Trigger: "ending",
				Description: "Ticks after ending before queued releases are cancelled."},
			{Key: "tail_len", Label: "tail len", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 90, Max: 900, Step: 10, Default: 420,
				Description: "Ticks for in-flight lanterns to finish their rise before the sky rests dark."},
		},
	}
}

type PaperLantern struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	Rise       float64 `json:"rise"`
	Drift      float64 `json:"drift"`
	Hue        float64 `json:"hue"`
	Lightness  float64 `json:"lightness"`
	Phase      float64 `json:"phase"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Age        int     `json:"age"`
	FadeTicks  int     `json:"fadeTicks"`
	FadeLogged bool    `json:"fadeLogged"`
	Foreground bool    `json:"foreground"`
}

type PaperLanternsState struct {
	Tick                 int     `json:"tick"`
	WindBias             float64 `json:"windBias"`
	WindBiasTicks        int     `json:"windBiasTicks"`
	QuietTicks           int     `json:"quietTicks"`
	ReleaseQueue         int     `json:"releaseQueue"`
	ReleaseWait          int     `json:"releaseWait"`
	Lifecycle            string  `json:"lifecycle"`
	IntroTicks           int     `json:"introTicks"`
	IntroFirstLaunched   bool    `json:"introFirstLaunched"`
	IntroClusterLaunched bool    `json:"introClusterLaunched"`
	EndingTicks          int     `json:"endingTicks"`
	EndingStopApplied    bool    `json:"endingStopApplied"`
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

// PaperLanterns simulates clustered sky-lantern releases on a low-resolution
// pixel grid.
type PaperLanterns struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	windBias      float64
	windBiasTicks int
	quietTicks    int
	releaseQueue  int
	releaseWait   int

	lifecycle            string
	introTicks           int
	introFirstLaunched   bool
	introClusterLaunched bool
	endingTicks          int
	endingStopApplied    bool

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &PaperLanterns{
		W:         w,
		H:         h,
		Grid:      grid,
		rng:       rngutil.New(seed),
		cfg:       cfg.withDefaults(),
		lifecycle: paperLanternsLifecycleRunning,
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
	for i := range p.lanterns {
		if p.lanterns[i].Col > float64(w-1) {
			p.lanterns[i].Col = float64(max(0, w-1))
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

func (p *PaperLanterns) LanternsCopy() []PaperLantern {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyLanternsLocked()
}

func (p *PaperLanterns) RestoreLanterns(list []PaperLantern) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreLanternsLocked(list)
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

func (p *PaperLanterns) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "lantern-emit":
		p.spawnLanternLocked("triggered")
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
	case "ending":
		p.startEndingLocked()
	default:
		return false
	}
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
	p.stepLifecycleLocked()
	p.stepTimersLocked()
	p.stepReleaseQueueLocked()

	if p.lifecycle == paperLanternsLifecycleRunning && p.quietTicks == 0 {
		if p.releaseQueue == 0 && p.rng.Float64() < p.cfg.ReleaseChance {
			p.startReleasePulseLocked("started")
		}
		if p.rng.Float64() < p.cfg.LoneEmitChance {
			p.spawnLanternLocked("started")
		}
		if p.rng.Float64() < p.cfg.QuietChance {
			p.startQuietGapLocked("started")
		}
	}
	if p.lifecycle != paperLanternsLifecycleEnded && p.windBiasTicks == 0 && p.rng.Float64() < 0.0015 {
		p.startWindDriftLocked("started")
	}

	p.stepLanternsLocked()
	p.paintFrameLocked()
}

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (p *PaperLanterns) stepLifecycleLocked() {
	switch p.lifecycle {
	case paperLanternsLifecycleIntro:
		p.introTicks++
		if !p.introFirstLaunched && p.introTicks >= p.cfg.IntroDelay {
			p.spawnLanternLocked("intro")
			p.introFirstLaunched = true
		}
		if !p.introClusterLaunched && p.introTicks >= p.cfg.IntroClusterDelay {
			p.startReleasePulseLocked("intro")
			p.introClusterLaunched = true
			p.lifecycle = paperLanternsLifecycleRunning
		}
	case paperLanternsLifecycleEnding:
		p.endingTicks++
		if !p.endingStopApplied && p.endingTicks >= p.cfg.EndingStopDelay {
			p.releaseQueue = 0
			p.releaseWait = 0
			p.endingStopApplied = true
		}
		if p.endingTicks >= p.cfg.EndingTail && len(p.lanterns) == 0 {
			p.lifecycle = paperLanternsLifecycleEnded
		}
	case "":
		p.lifecycle = paperLanternsLifecycleRunning
	}
}

func (p *PaperLanterns) stepTimersLocked() {
	if p.windBiasTicks > 0 {
		p.windBiasTicks--
		if p.windBiasTicks == 0 {
			p.windBias = 0
		}
	}
	if p.quietTicks > 0 {
		p.quietTicks--
	}
	if p.releaseWait > 0 {
		p.releaseWait--
	}
}

func (p *PaperLanterns) stepReleaseQueueLocked() {
	if p.releaseQueue <= 0 || p.releaseWait > 0 {
		return
	}
	if p.lifecycle == paperLanternsLifecycleEnded || (p.lifecycle == paperLanternsLifecycleEnding && p.endingStopApplied) {
		p.releaseQueue = 0
		return
	}
	p.spawnLanternLocked("release-pulse")
	p.releaseQueue--
	p.releaseWait = jitterInt(p.rng, p.cfg.ReleaseSpacing, 0.45)
}

func (p *PaperLanterns) stepLanternsLocked() {
	if len(p.lanterns) == 0 {
		if p.lifecycle == paperLanternsLifecycleEnding && p.endingTicks >= p.cfg.EndingTail {
			p.lifecycle = paperLanternsLifecycleEnded
		}
		return
	}
	effectiveWind := p.cfg.Wind + p.windBias
	dst := p.lanterns[:0]
	fadeRow := p.fadeStartRowLocked()
	for i := range p.lanterns {
		l := p.lanterns[i]
		l.Age++
		wobble := math.Sin(float64(p.tick)*0.035+l.Phase) * p.cfg.Wander
		l.Row -= l.Rise
		l.Col += effectiveWind + l.Drift + wobble
		if l.FadeTicks > 0 {
			l.FadeTicks--
		}
		if !l.FadeLogged && l.Row <= fadeRow {
			l.FadeLogged = true
			p.appendLog("lantern-fade", fmt.Sprintf("dimming near row %.0f", l.Row))
		}
		alpha := p.lanternAlphaLocked(l)
		offTop := l.Row < -float64(l.Height+2)
		offSide := l.Col < -8 || l.Col > float64(p.W+8)
		if alpha > 0.02 && !offTop && !offSide {
			dst = append(dst, l)
		}
	}
	p.lanterns = dst
	if p.lifecycle == paperLanternsLifecycleEnding && p.endingTicks >= p.cfg.EndingTail && len(p.lanterns) == 0 {
		p.lifecycle = paperLanternsLifecycleEnded
	}
}

func (p *PaperLanterns) spawnLanternLocked(reason string) {
	if p.lifecycle == paperLanternsLifecycleEnded {
		return
	}
	if len(p.lanterns) >= p.cfg.MaxLanterns || p.W <= 0 || p.H <= 0 {
		return
	}
	margin := 4
	if p.W < 12 {
		margin = 1
	}
	usable := max(1, p.W-margin*2)
	speedJit := 1 + (p.rng.Float64()*2-1)*p.cfg.SpeedJitter
	if speedJit < 0.25 {
		speedJit = 0.25
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	foreground := p.rng.Float64() > 0.28
	width := 3
	height := 4
	if foreground && p.rng.Float64() > 0.42 {
		width = 4
		height = 5
	}
	if p.W < 48 {
		width = max(2, width-1)
	}
	l := paperLantern{
		Row:        float64(p.H) + 2 + p.rng.Float64()*5,
		Col:        float64(margin) + p.rng.Float64()*float64(usable),
		Rise:       p.cfg.RiseSpeed * speedJit,
		Drift:      (p.rng.Float64()*2 - 1) * 0.018,
		Hue:        hue,
		Lightness:  light,
		Phase:      p.rng.Float64() * 2 * math.Pi,
		Width:      width,
		Height:     height,
		Foreground: foreground,
	}
	if !foreground {
		l.Rise *= 0.82
		l.Width = max(2, l.Width-1)
	}
	p.lanterns = append(p.lanterns, l)
	p.appendLog("lantern-emit", fmt.Sprintf("%s (row=%.0f col=%.0f)", reason, l.Row, l.Col))
}

func (p *PaperLanterns) startReleasePulseLocked(reason string) {
	if p.lifecycle == paperLanternsLifecycleEnded {
		return
	}
	span := max(0, p.cfg.ClusterMax-p.cfg.ClusterMin)
	count := p.cfg.ClusterMin
	if span > 0 {
		count += p.rng.Intn(span + 1)
	}
	room := p.cfg.MaxLanterns - len(p.lanterns) - p.releaseQueue
	if room < count {
		count = room
	}
	if count <= 0 {
		return
	}
	p.releaseQueue += count
	p.releaseWait = 0
	p.appendLog("release-pulse", fmt.Sprintf("%s (%d lanterns)", reason, count))
}

func (p *PaperLanterns) startWindDriftLocked(reason string) {
	p.windBiasTicks = jitterInt(p.rng, p.cfg.WindDriftDur, 0.3)
	p.windBias = (p.rng.Float64()*2 - 1) * p.cfg.WindShift
	p.appendLog("wind-drift", fmt.Sprintf("%s (dur=%d drift=%+.3f)", reason, p.windBiasTicks, p.windBias))
}

func (p *PaperLanterns) forceLanternFadeLocked() {
	if len(p.lanterns) == 0 {
		p.spawnLanternLocked("fade-preview")
		if len(p.lanterns) > 0 {
			p.lanterns[len(p.lanterns)-1].Row = p.fadeStartRowLocked()
		}
	}
	if len(p.lanterns) == 0 {
		return
	}
	target := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Row < p.lanterns[target].Row {
			target = i
		}
	}
	p.lanterns[target].FadeTicks = p.cfg.FadeDur
	p.lanterns[target].FadeLogged = true
	p.appendLog("lantern-fade", fmt.Sprintf("triggered (dur=%d)", p.cfg.FadeDur))
}

func (p *PaperLanterns) startQuietGapLocked(reason string) {
	p.quietTicks = jitterInt(p.rng, p.cfg.QuietGapDur, 0.35)
	p.releaseQueue = 0
	p.releaseWait = 0
	p.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d)", reason, p.quietTicks))
}

func (p *PaperLanterns) startIntroLocked() {
	p.lanterns = nil
	p.quietTicks = 0
	p.releaseQueue = 0
	p.releaseWait = 0
	p.windBias = 0
	p.windBiasTicks = 0
	p.lifecycle = paperLanternsLifecycleIntro
	p.introTicks = 0
	p.introFirstLaunched = false
	p.introClusterLaunched = false
	p.endingTicks = 0
	p.endingStopApplied = false
	p.appendLog("intro", "dark sky, waiting for first lantern")
	p.paintFrameLocked()
}

func (p *PaperLanterns) startEndingLocked() {
	if p.lifecycle == paperLanternsLifecycleEnded {
		return
	}
	p.lifecycle = paperLanternsLifecycleEnding
	p.endingTicks = 0
	p.endingStopApplied = false
	if p.cfg.EndingStopDelay == 0 {
		p.releaseQueue = 0
		p.releaseWait = 0
		p.endingStopApplied = true
	}
	p.appendLog("ending", "release stopped; lanterns finish their climb")
}

func (p *PaperLanterns) paintFrameLocked() {
	p.paintSkyLocked()
	for _, l := range p.lanterns {
		alpha := p.lanternAlphaLocked(l)
		if alpha > 0 {
			p.paintLanternLocked(l, alpha)
		}
	}
}

func (p *PaperLanterns) paintSkyLocked() {
	for y := range p.Grid {
		t := 0.0
		if p.H > 1 {
			t = float64(y) / float64(p.H-1)
		}
		c := color.RGBA{
			R: uint8(3 + 8*t),
			G: uint8(5 + 5*t),
			B: uint8(13 + 12*t),
			A: 255,
		}
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
	starCount := max(8, p.W*p.H/900)
	for i := 0; i < starCount; i++ {
		x := int(p.staticHash(uint32(i*97+11)) * float64(max(1, p.W)))
		yLimit := max(1, p.H*2/3)
		y := int(p.staticHash(uint32(i*131+17)) * float64(yLimit))
		if x >= 0 && x < p.W && y >= 0 && y < p.H {
			b := uint8(35 + 30*p.staticHash(uint32(i*37+5)))
			p.Grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: b, G: b, B: b + 12, A: 255}}
		}
	}
}

func (p *PaperLanterns) paintLanternLocked(l paperLantern, alpha float64) {
	cx := int(math.Round(l.Col))
	top := int(math.Round(l.Row))
	w := max(2, l.Width)
	h := max(3, l.Height)
	left := cx - w/2
	body := hslToRGB(l.Hue, p.cfg.Saturation*0.65, clamp01(l.Lightness*0.84))
	rim := hslToRGB(l.Hue, p.cfg.Saturation, clamp01(p.cfg.LightnessMax*0.72))
	core := hslToRGB(math.Mod(l.Hue-7+360, 360), clamp01(p.cfg.Saturation+0.12), clamp01(p.cfg.LightnessMax))
	flame := color.RGBA{R: 255, G: 226, B: 118, A: 255}

	glowAlpha := alpha * 0.22
	for gy := top - 1; gy <= top+h; gy++ {
		for gx := left - 1; gx <= left+w; gx++ {
			if gx < 0 || gy < 0 || gy >= p.H || gx >= p.W {
				continue
			}
			dist := math.Abs(float64(gx-cx))*0.8 + math.Abs(float64(gy-(top+h/2)))
			if dist <= 2.2 {
				p.blendPixel(gx, gy, core, glowAlpha*(1-dist/3.0))
			}
		}
	}

	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			x := left + dx
			y := top + dy
			if x < 0 || y < 0 || y >= p.H || x >= p.W {
				continue
			}
			edge := dx == 0 || dx == w-1 || dy == 0 || dy == h-1
			localAlpha := alpha * 0.9
			c := body
			if edge {
				c = rim
				localAlpha *= 0.85
			}
			p.blendPixel(x, y, c, localAlpha)
		}
	}

	flameX := cx
	flameY := top + max(1, h-2)
	p.blendPixel(flameX, flameY, flame, alpha)
	if h >= 5 {
		p.blendPixel(flameX, flameY-1, core, alpha*0.65)
	}
}

func (p *PaperLanterns) lanternAlphaLocked(l paperLantern) float64 {
	alpha := 1.0
	fadeStart := p.fadeStartRowLocked()
	if l.Row <= fadeStart {
		alpha *= clamp01(l.Row / math.Max(1, fadeStart))
	}
	if l.FadeTicks > 0 {
		alpha = math.Min(alpha, clamp01(float64(l.FadeTicks)/float64(max(1, p.cfg.FadeDur))))
	}
	if p.lifecycle == paperLanternsLifecycleEnding && p.cfg.EndingTail > 0 {
		hold := int(math.Round(float64(p.cfg.EndingTail) * 0.62))
		if p.endingTicks > hold {
			alpha *= clamp01(float64(p.cfg.EndingTail-p.endingTicks) / float64(max(1, p.cfg.EndingTail-hold)))
		}
	}
	return clamp01(alpha)
}

func (p *PaperLanterns) fadeStartRowLocked() float64 {
	return float64(min(max(2, p.cfg.FadeAltitude), max(2, p.H-1)))
}

func (p *PaperLanterns) blendPixel(x, y int, c color.RGBA, alpha float64) {
	if alpha <= 0 || x < 0 || y < 0 || y >= p.H || x >= p.W {
		return
	}
	alpha = clamp01(alpha)
	dst := p.Grid[y][x].C
	p.Grid[y][x] = Pixel{
		Filled: true,
		C: color.RGBA{
			R: uint8(float64(dst.R)*(1-alpha) + float64(c.R)*alpha),
			G: uint8(float64(dst.G)*(1-alpha) + float64(c.G)*alpha),
			B: uint8(float64(dst.B)*(1-alpha) + float64(c.B)*alpha),
			A: 255,
		},
	}
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	lifecycle := p.lifecycle
	if lifecycle == "" {
		lifecycle = paperLanternsLifecycleRunning
	}
	return PaperLanternsState{
		Tick:                 p.tick,
		WindBias:             p.windBias,
		WindBiasTicks:        p.windBiasTicks,
		QuietTicks:           p.quietTicks,
		ReleaseQueue:         p.releaseQueue,
		ReleaseWait:          p.releaseWait,
		Lifecycle:            lifecycle,
		IntroTicks:           p.introTicks,
		IntroFirstLaunched:   p.introFirstLaunched,
		IntroClusterLaunched: p.introClusterLaunched,
		EndingTicks:          p.endingTicks,
		EndingStopApplied:    p.endingStopApplied,
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.windBias = s.WindBias
	p.windBiasTicks = s.WindBiasTicks
	p.quietTicks = s.QuietTicks
	p.releaseQueue = s.ReleaseQueue
	p.releaseWait = s.ReleaseWait
	p.lifecycle = s.Lifecycle
	if p.lifecycle == "" {
		p.lifecycle = paperLanternsLifecycleRunning
	}
	p.introTicks = s.IntroTicks
	p.introFirstLaunched = s.IntroFirstLaunched
	p.introClusterLaunched = s.IntroClusterLaunched
	p.endingTicks = s.EndingTicks
	p.endingStopApplied = s.EndingStopApplied
}

func (p *PaperLanterns) copyLanternsLocked() []PaperLantern {
	out := make([]PaperLantern, len(p.lanterns))
	for i, l := range p.lanterns {
		out[i] = PaperLantern{
			Row:        l.Row,
			Col:        l.Col,
			Rise:       l.Rise,
			Drift:      l.Drift,
			Hue:        l.Hue,
			Lightness:  l.Lightness,
			Phase:      l.Phase,
			Width:      l.Width,
			Height:     l.Height,
			Age:        l.Age,
			FadeTicks:  l.FadeTicks,
			FadeLogged: l.FadeLogged,
			Foreground: l.Foreground,
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
			Rise:       l.Rise,
			Drift:      l.Drift,
			Hue:        l.Hue,
			Lightness:  l.Lightness,
			Phase:      l.Phase,
			Width:      l.Width,
			Height:     l.Height,
			Age:        l.Age,
			FadeTicks:  l.FadeTicks,
			FadeLogged: l.FadeLogged,
			Foreground: l.Foreground,
		}
		if p.lanterns[i].Width <= 0 {
			p.lanterns[i].Width = 3
		}
		if p.lanterns[i].Height <= 0 {
			p.lanterns[i].Height = 4
		}
	}
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *PaperLanterns) staticHash(n uint32) float64 {
	n ^= n >> 16
	n *= 0x7feb352d
	n ^= n >> 15
	n *= 0x846ca68b
	n ^= n >> 16
	return float64(n&0xffffff) / float64(0x1000000)
}
