package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type paperLantern struct {
	Row       float64
	Col       float64
	VRow      float64
	VCol      float64
	Hue       float64
	Lightness float64
	Phase     float64
	Age       int
}

// PaperLanternsConfig tunes the Paper Lanterns effect. Zero values fall back
// to calm festival defaults, except fields whose schema default is explicitly
// zero such as ending_stop.
type PaperLanternsConfig struct {
	// INTRODUCTION
	IntroDelay   int `json:"intro_delay"`
	IntroCluster int `json:"intro_cluster"`
	// ENDING
	EndingStop int `json:"ending_stop"`
	EndingTail int `json:"ending_tail"`
	// SPAWN / RELEASE
	SpawnEvery    int     `json:"spawn"`
	MaxLanterns   int     `json:"max"`
	ReleaseChance float64 `json:"release_p"`
	QuietGap      int     `json:"quiet_gap"`
	ClusterMin    int     `json:"cluster_min"`
	ClusterMax    int     `json:"cluster_max"`
	ClusterWindow int     `json:"cluster_window"`
	// MOTION
	RiseSpeed       float64 `json:"rise"`
	SpeedJitter     float64 `json:"speed_jit"`
	WindDrift       float64 `json:"wind"`
	WindShiftChance float64 `json:"wind_shift_p"`
	WindShiftRange  float64 `json:"wind_shift"`
	// SHAPE / FADE
	FadeStart float64 `json:"fade_start"`
	Glow      float64 `json:"glow"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	FlameHue     float64 `json:"flame_hue"`
}

func (c PaperLanternsConfig) withDefaults() PaperLanternsConfig {
	if c.IntroDelay <= 0 {
		c.IntroDelay = 45
	}
	if c.IntroCluster <= 0 {
		c.IntroCluster = 150
	}
	if c.EndingStop < 0 {
		c.EndingStop = 0
	}
	if c.EndingTail <= 0 {
		c.EndingTail = 900
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 95
	}
	if c.MaxLanterns <= 0 {
		c.MaxLanterns = 72
	}
	if c.MaxLanterns > 180 {
		c.MaxLanterns = 180
	}
	if c.ReleaseChance < 0 {
		c.ReleaseChance = 0
	}
	if c.ReleaseChance == 0 {
		c.ReleaseChance = 0.0012
	}
	if c.QuietGap <= 0 {
		c.QuietGap = 620
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
	if c.ClusterWindow <= 0 {
		c.ClusterWindow = 44
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.105
	}
	if c.SpeedJitter < 0 {
		c.SpeedJitter = 0
	}
	if c.SpeedJitter == 0 {
		c.SpeedJitter = 0.26
	}
	if c.WindShiftChance < 0 {
		c.WindShiftChance = 0
	}
	if c.WindShiftChance == 0 {
		c.WindShiftChance = 0.00065
	}
	if c.WindShiftRange <= 0 {
		c.WindShiftRange = 0.055
	}
	if c.FadeStart <= 0 {
		c.FadeStart = 0.32
	}
	c.FadeStart = clamp01(c.FadeStart)
	if c.Glow <= 0 {
		c.Glow = 0.58
	}
	if c.Hue == 0 {
		c.Hue = 38
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.72
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.48
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.78
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.FlameHue == 0 {
		c.FlameHue = 24
	}
	return c
}

// PaperLanternsSchema describes Paper Lanterns' dev controls.
func PaperLanternsSchema() EffectSchema {
	return EffectSchema{
		Name:           "paper-lanterns",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_delay", Label: "first wait", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks after intro before the first lone lantern lifts from the bottom."},
			{Key: "intro_cluster", Label: "first cluster", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 360, Step: 5, Default: 150,
				Description: "Ticks after intro before the first release pulse establishes the festival rhythm."},
			{Key: "ending_stop", Label: "release stop", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 0,
				Description: "Ticks after ending starts before new releases are fully suppressed."},
			{Key: "ending_tail", Label: "tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 120, Max: 1800, Step: 30, Default: 900, Trigger: "ending",
				Description: "Maximum outro tail while in-flight lanterns rise and the sky returns to dark."},
			{Key: "spawn", Label: "lone 1/", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 30, Max: 240, Step: 5, Default: 95, Trigger: "lantern-emit",
				Description: "One-in-N tick chance of a lone lantern between release pulses."},
			{Key: "max", Label: "max lanterns", Slot: SlotSpawn, Group: "release", Type: KnobInt, Min: 8, Max: 180, Step: 1, Default: 72,
				Description: "Maximum number of active lanterns allowed in the sky."},
			{Key: "release_p", Label: "release pulse", Slot: SlotEvent, Group: "release", Type: KnobFloat, Min: 0, Max: 0.006, Step: 0.0001, Default: 0.0012, Trigger: "release-pulse",
				Description: "Per-tick chance of a clustered release when the quiet gap has expired."},
			{Key: "quiet_gap", Label: "quiet gap", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 120, Max: 1800, Step: 30, Default: 620, Trigger: "quiet-gap",
				Description: "Suppression window after a release pulse so clusters feel occasional."},
			{Key: "cluster_min", Label: "cluster min", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 1, Max: 18, Step: 1, Default: 5,
				Description: "Smallest number of lanterns in a release pulse."},
			{Key: "cluster_max", Label: "cluster max", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 1, Max: 24, Step: 1, Default: 10,
				Description: "Largest number of lanterns in a release pulse."},
			{Key: "cluster_window", Label: "cluster window", Slot: SlotEventMod, Group: "release", Type: KnobInt, Min: 5, Max: 120, Step: 5, Default: 44,
				Description: "Ticks over which a pulse's lanterns are emitted."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.035, Max: 0.22, Step: 0.005, Default: 0.105,
				Description: "Base upward buoyancy. Higher values rise faster, but still drift rather than launch."},
			{Key: "speed_jit", Label: "speed jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.7, Step: 0.01, Default: 0.26,
				Description: "Per-lantern variation in rise speed."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -0.12, Max: 0.12, Step: 0.005, Default: 0.0,
				Description: "Global horizontal drift bias. Negative bends left; positive bends right."},
			{Key: "wind_shift_p", Label: "wind shift", Slot: SlotEvent, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.004, Step: 0.0001, Default: 0.00065, Trigger: "wind-drift",
				Description: "Per-tick chance that the global wind bias eases toward a new side drift."},
			{Key: "wind_shift", Label: "wind range", Slot: SlotEventMod, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.16, Step: 0.005, Default: 0.055,
				Description: "Maximum random offset applied by a wind-drift event."},
			{Key: "fade_start", Label: "fade start", Slot: SlotEnd, Group: "fade", Type: KnobFloat, Min: 0.08, Max: 0.6, Step: 0.01, Default: 0.32,
				Description: "Fraction of frame height from the top where lanterns begin fading out."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.05, Max: 1.0, Step: 0.01, Default: 0.58,
				Description: "Soft warm halo around each paper rectangle."},
			{Key: "hue", Label: "paper hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 12, Max: 58, Step: 1, Default: 38,
				Description: "Base lantern paper hue; this is the main palette knob."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 38, Step: 1, Default: 12,
				Description: "Hue variation across lanterns in the same release."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1.0, Step: 0.01, Default: 0.72,
				Description: "Paper color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.18, Max: 0.85, Step: 0.01, Default: 0.48,
				Description: "Dim end of the lantern paper brightness range."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.96, Step: 0.01, Default: 0.78,
				Description: "Bright end of the lantern paper brightness range."},
			{Key: "flame_hue", Label: "flame hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 45, Step: 1, Default: 24,
				Description: "Hue of the tiny center flame visible inside each paper lantern."},
		},
	}
}

type PaperLanternsState struct {
	Tick              int       `json:"tick"`
	Lifecycle         Lifecycle `json:"lifecycle"`
	IntroTicks        int       `json:"introTicks"`
	IntroTotal        int       `json:"introTotal"`
	IntroFirstTicks   int       `json:"introFirstTicks"`
	IntroClusterTicks int       `json:"introClusterTicks"`
	EndingTicks       int       `json:"endingTicks"`
	EndingTotal       int       `json:"endingTotal"`
	ReleaseStopTicks  int       `json:"releaseStopTicks"`
	Ended             bool      `json:"ended"`
	Wind              float64   `json:"wind"`
	WindTarget        float64   `json:"windTarget"`
	QuietTicks        int       `json:"quietTicks"`
	ClusterTicks      int       `json:"clusterTicks"`
	ClusterRemaining  int       `json:"clusterRemaining"`
}

type PaperLantern struct {
	Row       float64 `json:"row"`
	Col       float64 `json:"col"`
	VRow      float64 `json:"vRow"`
	VCol      float64 `json:"vCol"`
	Hue       float64 `json:"hue"`
	Lightness float64 `json:"lightness"`
	Phase     float64 `json:"phase"`
	Age       int     `json:"age"`
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

// PaperLanterns simulates warm paper lanterns rising slowly through a dark sky.
type PaperLanterns struct {
	mu sync.Mutex

	W, H     int
	Grid     [][]Pixel
	lanterns []paperLantern
	rng      *rngutil.RNG
	cfg      PaperLanternsConfig
	tick     int

	introTicks        int
	introTotal        int
	introFirstTicks   int
	introClusterTicks int
	endingTicks       int
	endingTotal       int
	releaseStopTicks  int
	ended             bool

	wind             float64
	windTarget       float64
	quietTicks       int
	clusterTicks     int
	clusterRemaining int

	log []LogEntry
}

func NewPaperLanterns(w, h int, seed int64, cfg PaperLanternsConfig) *PaperLanterns {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	c := cfg.withDefaults()
	p := &PaperLanterns{
		W:          w,
		H:          h,
		Grid:       grid,
		rng:        rngutil.New(seed),
		cfg:        c,
		wind:       c.WindDrift,
		windTarget: c.WindDrift,
	}
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
	newCfg := cfg.withDefaults()
	if p.cfg.RiseSpeed > 0 && newCfg.RiseSpeed != p.cfg.RiseSpeed {
		ratio := newCfg.RiseSpeed / p.cfg.RiseSpeed
		for i := range p.lanterns {
			p.lanterns[i].VRow *= ratio
		}
	}
	if newCfg.MaxLanterns < len(p.lanterns) {
		p.lanterns = append([]paperLantern(nil), p.lanterns[len(p.lanterns)-newCfg.MaxLanterns:]...)
	}
	p.windTarget += newCfg.WindDrift - p.cfg.WindDrift
	p.cfg = newCfg
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
	case "intro":
		p.startIntroLocked()
	case "ending":
		p.startEndingLocked()
	case "lantern-emit":
		p.ended = false
		p.spawnLanternLocked(true)
	case "release-pulse":
		p.ended = false
		p.startReleasePulseLocked(true)
	case "wind-drift":
		p.shiftWindLocked(true)
	case "lantern-fade":
		p.fadeHighestLanternLocked(true)
	case "quiet-gap":
		p.quietTicks = max(p.quietTicks, p.cfg.QuietGap)
		p.appendLog("quiet-gap", fmt.Sprintf("started (dur=%d)", p.quietTicks))
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

func (p *PaperLanterns) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (p *PaperLanterns) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	if p.ended {
		p.paintFrameLocked()
		return
	}

	if p.quietTicks > 0 {
		p.quietTicks--
	}

	p.stepLifecycleLocked()
	p.stepWindLocked()
	p.maybeStartEventsLocked()
	p.stepClusterLocked()
	p.stepLanternsLocked()
	p.paintFrameLocked()
}

func (p *PaperLanterns) snapshotStateLocked() PaperLanternsState {
	return PaperLanternsState{
		Tick:              p.tick,
		Lifecycle:         p.lifecycleLocked(),
		IntroTicks:        p.introTicks,
		IntroTotal:        p.introTotal,
		IntroFirstTicks:   p.introFirstTicks,
		IntroClusterTicks: p.introClusterTicks,
		EndingTicks:       p.endingTicks,
		EndingTotal:       p.endingTotal,
		ReleaseStopTicks:  p.releaseStopTicks,
		Ended:             p.ended,
		Wind:              p.wind,
		WindTarget:        p.windTarget,
		QuietTicks:        p.quietTicks,
		ClusterTicks:      p.clusterTicks,
		ClusterRemaining:  p.clusterRemaining,
	}
}

func (p *PaperLanterns) restoreStateLocked(s PaperLanternsState) {
	p.tick = s.Tick
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.introFirstTicks = s.IntroFirstTicks
	p.introClusterTicks = s.IntroClusterTicks
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.releaseStopTicks = s.ReleaseStopTicks
	p.ended = s.Ended
	p.wind = s.Wind
	p.windTarget = s.WindTarget
	p.quietTicks = s.QuietTicks
	p.clusterTicks = s.ClusterTicks
	p.clusterRemaining = s.ClusterRemaining
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
			Row:       l.Row,
			Col:       l.Col,
			VRow:      l.VRow,
			VCol:      l.VCol,
			Hue:       l.Hue,
			Lightness: l.Lightness,
			Phase:     l.Phase,
			Age:       l.Age,
		}
	}
	return out
}

func (p *PaperLanterns) restoreLanternsLocked(list []PaperLantern) {
	p.lanterns = make([]paperLantern, len(list))
	for i, l := range list {
		p.lanterns[i] = paperLantern{
			Row:       l.Row,
			Col:       l.Col,
			VRow:      l.VRow,
			VCol:      l.VCol,
			Hue:       l.Hue,
			Lightness: l.Lightness,
			Phase:     l.Phase,
			Age:       l.Age,
		}
	}
}

func (p *PaperLanterns) startIntroLocked() {
	p.ended = false
	p.lanterns = nil
	p.endingTicks = 0
	p.endingTotal = 0
	p.releaseStopTicks = 0
	p.clusterTicks = 0
	p.clusterRemaining = 0
	p.quietTicks = 0
	p.introFirstTicks = max(0, p.cfg.IntroDelay)
	p.introClusterTicks = max(1, p.cfg.IntroCluster)
	p.introTotal = max(1, p.introClusterTicks+p.cfg.ClusterWindow)
	p.introTicks = p.introTotal
	p.appendLog("intro", fmt.Sprintf("started (first=%d, cluster=%d)", p.introFirstTicks, p.introClusterTicks))
}

func (p *PaperLanterns) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.introFirstTicks = 0
	p.introClusterTicks = 0
	p.clusterTicks = 0
	p.clusterRemaining = 0
	p.releaseStopTicks = max(0, p.cfg.EndingStop)
	p.endingTotal = max(1, p.cfg.EndingTail)
	p.endingTicks = p.endingTotal
	p.ended = false
	p.appendLog("ending", fmt.Sprintf("started (stop=%d, tail=%d)", p.releaseStopTicks, p.endingTotal))
}

func (p *PaperLanterns) stepLifecycleLocked() {
	if p.introFirstTicks > 0 {
		p.introFirstTicks--
		if p.introFirstTicks == 0 {
			p.spawnLanternLocked(true)
		}
	}
	if p.introClusterTicks > 0 {
		p.introClusterTicks--
		if p.introClusterTicks == 0 {
			p.startReleasePulseLocked(true)
		}
	}
	if p.introTicks > 0 {
		p.introTicks--
	}
	if p.releaseStopTicks > 0 {
		p.releaseStopTicks--
	}
	if p.endingTicks > 0 {
		p.endingTicks--
		if p.endingTicks == 0 {
			p.lanterns = nil
			p.ended = true
			p.appendLog("lantern-fade", "outro tail complete")
		}
	}
}

func (p *PaperLanterns) releasesAllowedLocked() bool {
	if p.ended {
		return false
	}
	if p.endingTicks > 0 && p.releaseStopTicks <= 0 {
		return false
	}
	return true
}

func (p *PaperLanterns) maybeStartEventsLocked() {
	if !p.releasesAllowedLocked() || p.introTicks > 0 {
		return
	}
	if p.rng.Float64() < p.cfg.WindShiftChance {
		p.shiftWindLocked(true)
	}
	if p.quietTicks == 0 && p.rng.Float64() < p.cfg.ReleaseChance {
		p.startReleasePulseLocked(true)
		return
	}
	if len(p.lanterns) < p.cfg.MaxLanterns && p.rng.Intn(p.cfg.SpawnEvery) == 0 {
		p.spawnLanternLocked(true)
	}
}

func (p *PaperLanterns) stepWindLocked() {
	target := p.cfg.WindDrift + (p.windTarget - p.cfg.WindDrift)
	p.wind += (target - p.wind) * 0.018
}

func (p *PaperLanterns) shiftWindLocked(log bool) {
	span := math.Abs(p.cfg.WindShiftRange)
	p.windTarget = p.cfg.WindDrift + (p.rng.Float64()*2-1)*span
	if log {
		p.appendLog("wind-drift", fmt.Sprintf("target=%.3f", p.windTarget))
	}
}

func (p *PaperLanterns) startReleasePulseLocked(log bool) {
	if !p.releasesAllowedLocked() {
		return
	}
	count := p.cfg.ClusterMin
	if p.cfg.ClusterMax > p.cfg.ClusterMin {
		count += p.rng.Intn(p.cfg.ClusterMax - p.cfg.ClusterMin + 1)
	}
	room := p.cfg.MaxLanterns - len(p.lanterns) - p.clusterRemaining
	if room < count {
		count = room
	}
	if count <= 0 {
		return
	}
	p.clusterRemaining += count
	p.clusterTicks = max(p.clusterTicks, max(1, p.cfg.ClusterWindow))
	p.quietTicks = max(p.quietTicks, p.cfg.QuietGap)
	if log {
		p.appendLog("release-pulse", fmt.Sprintf("cluster queued (count=%d, window=%d)", count, p.clusterTicks))
		p.appendLog("quiet-gap", fmt.Sprintf("started (dur=%d)", p.quietTicks))
	}
}

func (p *PaperLanterns) stepClusterLocked() {
	if p.clusterRemaining <= 0 {
		p.clusterTicks = 0
		return
	}
	if !p.releasesAllowedLocked() {
		p.clusterRemaining = 0
		p.clusterTicks = 0
		return
	}
	if p.clusterTicks <= 0 {
		p.clusterTicks = 1
	}
	spawnNow := p.clusterTicks <= p.clusterRemaining
	if !spawnNow {
		gap := max(1, p.clusterTicks/p.clusterRemaining)
		spawnNow = p.rng.Intn(gap) == 0
	}
	if spawnNow {
		p.spawnLanternLocked(true)
		p.clusterRemaining--
	}
	p.clusterTicks--
	if p.clusterTicks <= 0 && p.clusterRemaining > 0 {
		for p.clusterRemaining > 0 {
			p.spawnLanternLocked(true)
			p.clusterRemaining--
		}
	}
}

func (p *PaperLanterns) spawnLanternLocked(log bool) {
	if len(p.lanterns) >= p.cfg.MaxLanterns || p.W <= 0 || p.H <= 0 {
		return
	}
	speedJitter := 1 + (p.rng.Float64()*2-1)*p.cfg.SpeedJitter
	if speedJitter < 0.25 {
		speedJitter = 0.25
	}
	hue := math.Mod(p.cfg.Hue+(p.rng.Float64()*2-1)*p.cfg.HueSpread+360, 360)
	light := p.cfg.LightnessMin + p.rng.Float64()*(p.cfg.LightnessMax-p.cfg.LightnessMin)
	l := paperLantern{
		Row:       float64(p.H) + 2 + p.rng.Float64()*3,
		Col:       p.rng.Float64() * float64(max(1, p.W-1)),
		VRow:      -p.cfg.RiseSpeed * speedJitter,
		VCol:      p.wind*0.65 + (p.rng.Float64()*2-1)*0.012,
		Hue:       hue,
		Lightness: light,
		Phase:     p.rng.Float64() * math.Pi * 2,
	}
	p.lanterns = append(p.lanterns, l)
	if log {
		p.appendLog("lantern-emit", fmt.Sprintf("launched (row=%.1f col=%.1f)", l.Row, l.Col))
	}
}

func (p *PaperLanterns) seedInitialLanternsLocked() {
	if p.W <= 0 || p.H <= 0 || p.cfg.MaxLanterns <= 0 {
		return
	}
	count := min(6, max(2, p.cfg.MaxLanterns/12))
	if count > p.cfg.MaxLanterns {
		count = p.cfg.MaxLanterns
	}
	for i := 0; i < count; i++ {
		p.spawnLanternLocked(false)
		idx := len(p.lanterns) - 1
		if idx < 0 {
			return
		}
		p.lanterns[idx].Row = float64(p.H) * (0.22 + p.rng.Float64()*0.72)
		p.lanterns[idx].Col = p.rng.Float64() * float64(max(1, p.W-1))
		p.lanterns[idx].Age = 48 + p.rng.Intn(180)
	}
}

func (p *PaperLanterns) stepLanternsLocked() {
	out := p.lanterns[:0]
	for i := range p.lanterns {
		l := p.lanterns[i]
		l.Age++
		l.Phase += 0.045 + 0.01*math.Sin(float64(l.Age)*0.013)
		l.VCol += (p.wind - l.VCol) * 0.012
		l.VCol += math.Sin(l.Phase*0.7) * 0.0007
		l.Row += l.VRow + math.Sin(l.Phase)*0.006
		l.Col += l.VCol
		if l.Col < -8 || l.Col > float64(p.W)+8 || l.Row < -8 {
			p.appendLog("lantern-fade", fmt.Sprintf("faded (row=%.1f col=%.1f)", l.Row, l.Col))
			continue
		}
		out = append(out, l)
	}
	p.lanterns = out
}

func (p *PaperLanterns) fadeHighestLanternLocked(log bool) {
	if len(p.lanterns) == 0 {
		if log {
			p.appendLog("lantern-fade", "no lanterns in flight")
		}
		return
	}
	best := 0
	for i := 1; i < len(p.lanterns); i++ {
		if p.lanterns[i].Row < p.lanterns[best].Row {
			best = i
		}
	}
	l := p.lanterns[best]
	p.lanterns = append(p.lanterns[:best], p.lanterns[best+1:]...)
	if log {
		p.appendLog("lantern-fade", fmt.Sprintf("manual fade (row=%.1f col=%.1f)", l.Row, l.Col))
	}
}

func (p *PaperLanterns) paintFrameLocked() {
	p.clearGridLocked()
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
	alpha := p.lanternAlphaLocked(l)
	if alpha <= 0.02 {
		return
	}
	cx := int(math.Round(l.Col))
	cy := int(math.Round(l.Row))
	paper := hslToRGB(l.Hue, clamp01(p.cfg.Saturation), clamp01(l.Lightness))
	rim := hslToRGB(l.Hue, clamp01(p.cfg.Saturation*0.82), clamp01(l.Lightness+0.12))
	glow := hslToRGB(l.Hue, clamp01(p.cfg.Saturation*0.85), clamp01(l.Lightness*0.72))
	flameLight := 0.62 + 0.16*(0.5+0.5*math.Sin(l.Phase*2.1))
	flame := hslToRGB(p.cfg.FlameHue, 0.92, clamp01(flameLight))

	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			dist := math.Sqrt(float64(dx*dx + dy*dy))
			if dist > 3.2 {
				continue
			}
			strength := (1 - dist/3.3) * p.cfg.Glow * alpha * 0.45
			if strength <= 0 {
				continue
			}
			paintPaperLanternPixel(p.Grid, cx+dx, cy+dy, scalePaperLanternColor(glow, strength))
		}
	}

	for dy := -2; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			edge := dx == -1 || dx == 1 || dy == -2 || dy == 1
			strength := alpha * 0.82
			c := paper
			if edge {
				strength = alpha
				c = rim
			}
			paintPaperLanternPixel(p.Grid, cx+dx, cy+dy, scalePaperLanternColor(c, strength))
		}
	}
	paintPaperLanternPixel(p.Grid, cx, cy, scalePaperLanternColor(flame, alpha))
	paintPaperLanternPixel(p.Grid, cx, cy+1, scalePaperLanternColor(flame, alpha*0.65))
}

func (p *PaperLanterns) lanternAlphaLocked(l paperLantern) float64 {
	alpha := 1.0
	fadeY := math.Max(1, float64(p.H)*p.cfg.FadeStart)
	if l.Row < fadeY {
		alpha *= clamp01(l.Row / fadeY)
	}
	if l.Age < 24 {
		alpha *= float64(l.Age) / 24
	}
	if p.endingTicks > 0 && p.endingTotal > 0 {
		tail := math.Min(180, float64(p.endingTotal)*0.35)
		if tail > 0 && float64(p.endingTicks) < tail {
			alpha *= clamp01(float64(p.endingTicks) / tail)
		}
	}
	return clamp01(alpha)
}

func paintPaperLanternPixel(grid [][]Pixel, x, y int, c color.RGBA) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cell := &grid[y][x]
	if !cell.Filled {
		*cell = Pixel{Filled: true, C: c}
		return
	}
	if c.R > cell.C.R {
		cell.C.R = c.R
	}
	if c.G > cell.C.G {
		cell.C.G = c.G
	}
	if c.B > cell.C.B {
		cell.C.B = c.B
	}
	cell.C.A = 255
}

func scalePaperLanternColor(c color.RGBA, amount float64) color.RGBA {
	amount = clamp01(amount)
	return color.RGBA{
		R: uint8(float64(c.R) * amount),
		G: uint8(float64(c.G) * amount),
		B: uint8(float64(c.B) * amount),
		A: 255,
	}
}

func (p *PaperLanterns) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}
