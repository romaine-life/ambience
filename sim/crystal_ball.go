package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	CrystalBallEventIntro      = "intro"
	CrystalBallEventEnding     = "ending"
	CrystalBallEventVisionForm = "vision-form"
	CrystalBallEventGlowPulse  = "glow-pulse"
	CrystalBallEventSwirl      = "swirl"
	CrystalBallEventClear      = "clear"

	crystalBallTicksPerSecond = 60
	crystalBallVisionDefaultP = 0.00055
)

// CrystalBallConfig tunes the contained crystal-ball effect. The public schema
// intentionally stays to the issue's five knobs: mist density, swirl speed,
// vision cadence, glow-pulse strength, and scene hue.
type CrystalBallConfig struct {
	Swirl        float64 `json:"swirl"`
	MistRate     float64 `json:"mistRate"`
	VisionChance float64 `json:"visionChance"`
	GlowPulse    float64 `json:"glowPulse"`
	Hue          float64 `json:"hue"`
}

func (c CrystalBallConfig) withDefaults() CrystalBallConfig {
	if c.Swirl <= 0 {
		c.Swirl = 0.72
	}
	c.Swirl = math.Max(0.08, math.Min(2.2, c.Swirl))
	if c.MistRate <= 0 {
		c.MistRate = 0.74
	}
	c.MistRate = math.Max(0.12, math.Min(1.7, c.MistRate))
	if c.VisionChance <= 0 {
		c.VisionChance = crystalBallVisionDefaultP
	}
	c.VisionChance = math.Max(0.00005, math.Min(0.004, c.VisionChance))
	if c.GlowPulse <= 0 {
		c.GlowPulse = 0.64
	}
	c.GlowPulse = math.Max(0.08, math.Min(1.6, c.GlowPulse))
	if c.Hue == 0 {
		c.Hue = 276
	}
	c.Hue = math.Mod(c.Hue+360, 360)
	return c
}

// NormalizeCrystalBallConfig returns cfg with defaults and clamps applied.
func NormalizeCrystalBallConfig(cfg CrystalBallConfig) CrystalBallConfig {
	return cfg.withDefaults()
}

func CrystalBallSchema() EffectSchema {
	return EffectSchema{
		Name:           "crystal-ball",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "mistRate", Label: "mist rate", Slot: SlotSpawn, Group: "orb", Type: KnobFloat, Min: 0.18, Max: 1.45, Step: 0.05, Default: 0.74, Trigger: CrystalBallEventIntro,
				Description: "Density of the contained mist wisps; fire intro to gather the mist."},
			{Key: "swirl", Label: "swirl", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.12, Max: 1.8, Step: 0.05, Default: 0.72, Trigger: CrystalBallEventSwirl,
				Description: "Speed of the internal spiral; higher values tighten the slow hypnotic turn."},
			{Key: "visionChance", Label: "vision chance", Slot: SlotEvent, Group: "vision", Type: KnobFloat, Min: 0.00005, Max: 0.003, Step: 0.00005, Default: crystalBallVisionDefaultP, Trigger: CrystalBallEventVisionForm,
				Description: "Average cadence for a fleeting vision to coalesce from the mist."},
			{Key: "glowPulse", Label: "glow pulse", Slot: SlotEventMod, Group: "glow", Type: KnobFloat, Min: 0.12, Max: 1.4, Step: 0.05, Default: 0.64, Trigger: CrystalBallEventGlowPulse,
				Description: "Strength of the soft inner pulse when glow-pulse fires."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 276, Trigger: CrystalBallEventEnding,
				Description: "Mystic hue; fire ending to clear the mist and dim the ball."},
		},
	}
}

// CrystalBallState is the wire/persisted state for the crystal-ball effect.
// Lifecycle is derived from timers and BallDark at snapshot time and is ignored
// on restore.
type CrystalBallState struct {
	Tick           int       `json:"tick"`
	IntroTicks     int       `json:"introTicks"`
	IntroTotal     int       `json:"introTotal"`
	EndingTicks    int       `json:"endingTicks"`
	EndingTotal    int       `json:"endingTotal"`
	ClearTicks     int       `json:"clearTicks"`
	ClearTotal     int       `json:"clearTotal"`
	VisionTicks    int       `json:"visionTicks"`
	VisionTotal    int       `json:"visionTotal"`
	VisionKind     int       `json:"visionKind"`
	VisionSeed     uint64    `json:"visionSeed,omitempty"`
	GlowTicks      int       `json:"glowTicks"`
	GlowTotal      int       `json:"glowTotal"`
	SwirlTicks     int       `json:"swirlTicks"`
	SwirlTotal     int       `json:"swirlTotal"`
	VisionCooldown int       `json:"visionCooldown"`
	BallDark       bool      `json:"ballDark"`
	MistSeed       uint64    `json:"mistSeed,omitempty"`
	Lifecycle      Lifecycle `json:"lifecycle"`
	RNGState       uint64    `json:"rngState,omitempty"`
}

// CrystalBallSnapshot is the wire shape returned by Snapshot().
type CrystalBallSnapshot struct {
	CrystalBallState
}

// CrystalBallPersistedState is the on-disk shape returned by
// SnapshotPersistedState().
type CrystalBallPersistedState struct {
	CrystalBallState
}

// CrystalBall is a contained orb on a stand with a deterministic internal mist
// spiral and fleeting vision forms.
type CrystalBall struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  CrystalBallConfig
	tick int

	introTicks     int
	introTotal     int
	endingTicks    int
	endingTotal    int
	clearTicks     int
	clearTotal     int
	visionTicks    int
	visionTotal    int
	visionKind     int
	visionSeed     uint64
	glowTicks      int
	glowTotal      int
	swirlTicks     int
	swirlTotal     int
	visionCooldown int
	ballDark       bool
	mistSeed       uint64

	log []LogEntry
}

func NewCrystalBall(w, h int, seed int64, cfg CrystalBallConfig) *CrystalBall {
	rng := rngutil.New(seed)
	b := &CrystalBall{
		W:          w,
		H:          h,
		rng:        rng,
		cfg:        cfg.withDefaults(),
		visionKind: -1,
		mistSeed:   rng.Uint64(),
	}
	b.scheduleNextVisionLocked()
	return b
}

func (b *CrystalBall) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.W = w
	b.H = h
}

func (b *CrystalBall) SetConfig(cfg CrystalBallConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cfg = cfg.withDefaults()
	if b.visionCooldown <= 0 && b.visionTicks <= 0 && !b.ballDark && b.endingTicks <= 0 {
		b.scheduleNextVisionLocked()
	}
}

func (b *CrystalBall) EffectiveConfig() CrystalBallConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

func (b *CrystalBall) CurrentTick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tick
}

func (b *CrystalBall) PerturbRNG(delta int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rng.Mix(delta)
}

func (b *CrystalBall) Snapshot() CrystalBallSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return CrystalBallSnapshot{CrystalBallState: b.snapshotStateLocked()}
}

func (b *CrystalBall) RestoreSnapshot(s CrystalBallSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.CrystalBallState)
}

func (b *CrystalBall) SnapshotPersistedState() CrystalBallPersistedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return CrystalBallPersistedState{CrystalBallState: b.snapshotStateLocked()}
}

func (b *CrystalBall) RestorePersistedState(s CrystalBallPersistedState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.CrystalBallState)
}

func (b *CrystalBall) snapshotStateLocked() CrystalBallState {
	return CrystalBallState{
		Tick:           b.tick,
		IntroTicks:     b.introTicks,
		IntroTotal:     b.introTotal,
		EndingTicks:    b.endingTicks,
		EndingTotal:    b.endingTotal,
		ClearTicks:     b.clearTicks,
		ClearTotal:     b.clearTotal,
		VisionTicks:    b.visionTicks,
		VisionTotal:    b.visionTotal,
		VisionKind:     b.visionKind,
		VisionSeed:     b.visionSeed,
		GlowTicks:      b.glowTicks,
		GlowTotal:      b.glowTotal,
		SwirlTicks:     b.swirlTicks,
		SwirlTotal:     b.swirlTotal,
		VisionCooldown: b.visionCooldown,
		BallDark:       b.ballDark,
		MistSeed:       b.mistSeed,
		Lifecycle:      b.lifecycleLocked(),
		RNGState:       b.rng.State(),
	}
}

func (b *CrystalBall) restoreStateLocked(s CrystalBallState) {
	b.tick = s.Tick
	b.introTicks = s.IntroTicks
	b.introTotal = s.IntroTotal
	b.endingTicks = s.EndingTicks
	b.endingTotal = s.EndingTotal
	b.clearTicks = s.ClearTicks
	b.clearTotal = s.ClearTotal
	b.visionTicks = s.VisionTicks
	b.visionTotal = s.VisionTotal
	b.visionKind = s.VisionKind
	b.visionSeed = s.VisionSeed
	b.glowTicks = s.GlowTicks
	b.glowTotal = s.GlowTotal
	b.swirlTicks = s.SwirlTicks
	b.swirlTotal = s.SwirlTotal
	b.visionCooldown = s.VisionCooldown
	b.ballDark = s.BallDark
	b.mistSeed = s.MistSeed
	if b.mistSeed == 0 {
		b.mistSeed = b.rng.Uint64()
	}
	if s.RNGState != 0 {
		b.rng.SetState(s.RNGState)
	}
	if b.visionKind < 0 || b.visionKind > 3 {
		b.visionKind = -1
	}
}

func (b *CrystalBall) lifecycleLocked() Lifecycle {
	switch {
	case b.introTicks > 0:
		return LifecycleIntro
	case b.endingTicks > 0:
		return LifecycleEnding
	case b.ballDark:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (b *CrystalBall) TriggerEvent(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch name {
	case CrystalBallEventIntro:
		b.startIntroLocked()
	case CrystalBallEventEnding:
		b.startEndingLocked()
	case CrystalBallEventVisionForm:
		if b.ballDark || b.endingTicks > 0 {
			return true
		}
		b.startVisionLocked("triggered")
	case CrystalBallEventGlowPulse:
		if b.ballDark || b.endingTicks > 0 {
			return true
		}
		b.startGlowPulseLocked("triggered")
	case CrystalBallEventSwirl:
		if b.ballDark || b.endingTicks > 0 {
			return true
		}
		b.startSwirlLocked("triggered")
	case CrystalBallEventClear:
		if b.ballDark || b.endingTicks > 0 {
			return true
		}
		b.startClearLocked("triggered")
	default:
		return false
	}
	return true
}

func (b *CrystalBall) Step() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tick++
	prevEnding := b.endingTicks
	prevVision := b.visionTicks
	b.decTimersLocked()

	if prevEnding > 0 && b.endingTicks <= 0 {
		b.finishEndingLocked()
		return
	}
	if prevVision > 0 && b.visionTicks <= 0 {
		b.visionTotal = 0
		b.visionKind = -1
		b.visionSeed = 0
		if !b.ballDark && b.endingTicks <= 0 {
			b.scheduleNextVisionLocked()
		}
	}
	if b.ballDark || b.endingTicks > 0 || b.introTicks > 0 || b.clearTicks > 0 {
		return
	}
	if b.visionCooldown > 0 {
		return
	}
	if b.visionTicks <= 0 {
		b.startVisionLocked("started")
	}
}

func (b *CrystalBall) decTimersLocked() {
	dec := func(v *int) {
		if *v > 0 {
			*v--
		}
	}
	dec(&b.introTicks)
	dec(&b.endingTicks)
	dec(&b.clearTicks)
	dec(&b.visionTicks)
	dec(&b.glowTicks)
	dec(&b.swirlTicks)
	dec(&b.visionCooldown)
	if b.introTicks <= 0 {
		b.introTotal = 0
	}
	if b.clearTicks <= 0 {
		b.clearTotal = 0
	}
	if b.glowTicks <= 0 {
		b.glowTotal = 0
	}
	if b.swirlTicks <= 0 {
		b.swirlTotal = 0
	}
}

func (b *CrystalBall) startIntroLocked() {
	b.ballDark = false
	b.endingTicks = 0
	b.endingTotal = 0
	b.clearTicks = 0
	b.clearTotal = 0
	b.introTotal = 180
	b.introTicks = b.introTotal
	b.startGlowPulseLocked("intro")
	b.scheduleNextVisionLocked()
	b.appendLogLocked(CrystalBallEventIntro, fmt.Sprintf("mist gathered (dur=%d)", b.introTotal))
}

func (b *CrystalBall) startEndingLocked() {
	b.ballDark = false
	b.introTicks = 0
	b.introTotal = 0
	b.clearTicks = 0
	b.clearTotal = 0
	b.visionTicks = 0
	b.visionTotal = 0
	b.visionKind = -1
	b.visionSeed = 0
	b.visionCooldown = 0
	b.endingTotal = 300
	b.endingTicks = b.endingTotal
	b.appendLogLocked(CrystalBallEventEnding, fmt.Sprintf("clearing to terminal dim (dur=%d)", b.endingTotal))
}

func (b *CrystalBall) finishEndingLocked() {
	b.ballDark = true
	b.endingTotal = 0
	b.glowTicks = 0
	b.glowTotal = 0
	b.swirlTicks = 0
	b.swirlTotal = 0
	b.appendLogLocked(CrystalBallEventClear, "mist cleared; orb dark")
}

func (b *CrystalBall) startVisionLocked(verb string) {
	dur := jitterInt(b.rng, 260, 0.22)
	b.visionTicks = dur
	b.visionTotal = dur
	b.visionKind = b.rng.Intn(4)
	b.visionSeed = b.rng.Uint64()
	b.visionCooldown = 0
	b.appendLogLocked(CrystalBallEventVisionForm, fmt.Sprintf("%s (shape=%d, dur=%d)", verb, b.visionKind, dur))
}

func (b *CrystalBall) startGlowPulseLocked(verb string) {
	base := 70 + int(math.Round(70*b.cfg.GlowPulse))
	dur := jitterInt(b.rng, base, 0.12)
	b.glowTicks = dur
	b.glowTotal = dur
	b.appendLogLocked(CrystalBallEventGlowPulse, fmt.Sprintf("%s (dur=%d, strength=%.2f)", verb, dur, b.cfg.GlowPulse))
}

func (b *CrystalBall) startSwirlLocked(verb string) {
	dur := jitterInt(b.rng, 180, 0.18)
	b.swirlTicks = dur
	b.swirlTotal = dur
	b.appendLogLocked(CrystalBallEventSwirl, fmt.Sprintf("%s (dur=%d, swirl=%.2f)", verb, dur, b.cfg.Swirl))
}

func (b *CrystalBall) startClearLocked(verb string) {
	dur := jitterInt(b.rng, 210, 0.16)
	b.clearTicks = dur
	b.clearTotal = dur
	b.visionTicks = 0
	b.visionTotal = 0
	b.visionKind = -1
	b.visionSeed = 0
	b.appendLogLocked(CrystalBallEventClear, fmt.Sprintf("%s (dur=%d)", verb, dur))
}

func (b *CrystalBall) scheduleNextVisionLocked() {
	if b.cfg.VisionChance <= 0 {
		b.visionCooldown = 0
		return
	}
	base := (20 + b.rng.Intn(21)) * crystalBallTicksPerSecond
	scale := math.Sqrt(crystalBallVisionDefaultP / b.cfg.VisionChance)
	delay := int(math.Round(float64(base) * scale))
	b.visionCooldown = max(5*crystalBallTicksPerSecond, min(120*crystalBallTicksPerSecond, delay))
}

func (b *CrystalBall) appendLogLocked(kind, desc string) {
	b.log = append(b.log, LogEntry{Tick: b.tick, Type: kind, Desc: desc})
	if len(b.log) > 200 {
		b.log = b.log[len(b.log)-200:]
	}
}

func (b *CrystalBall) DrainLog() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) == 0 {
		return nil
	}
	out := b.log
	b.log = nil
	return out
}

func (b *CrystalBall) Frame() [][]Pixel {
	return b.GridCopy()
}

func (b *CrystalBall) GridCopy() [][]Pixel {
	b.mu.Lock()
	defer b.mu.Unlock()

	grid := newPixelGrid(b.W, b.H)
	if b.W <= 0 || b.H <= 0 {
		return grid
	}

	mist, glow, dim := b.renderLevelsLocked()
	cx := float64(b.W-1) / 2
	cy := float64(b.H) * 0.42
	rx := math.Max(6, math.Min(float64(b.W)*0.30, float64(b.H)*0.34))
	ry := math.Max(6, math.Min(float64(b.H)*0.33, float64(b.W)*0.28))

	b.paintCrystalBackgroundLocked(grid)
	b.paintCrystalStandLocked(grid, cx, cy, rx, ry, dim)
	b.paintCrystalOrbBodyLocked(grid, cx, cy, rx, ry, glow, dim)
	b.paintCrystalMistLocked(grid, cx, cy, rx, ry, mist, glow)
	b.paintCrystalVisionLocked(grid, cx, cy, rx, ry, mist, glow)
	b.paintCrystalOrbRimLocked(grid, cx, cy, rx, ry, dim)
	return grid
}

func (b *CrystalBall) renderLevelsLocked() (mist, glow, dim float64) {
	dim = 1
	mist = b.cfg.MistRate
	breath := 0.5 + 0.5*math.Sin(float64(b.tick)*0.026)
	glow = 0.46 + 0.28*breath
	if b.glowTicks > 0 && b.glowTotal > 0 {
		p := phaseProgress(b.glowTotal, b.glowTicks)
		glow += b.cfg.GlowPulse * math.Sin(math.Pi*p) * 0.58
	}
	if b.introTicks > 0 && b.introTotal > 0 {
		p := phaseProgress(b.introTotal, b.introTicks)
		dim *= 0.18 + 0.82*p
		mist *= 0.08 + 0.92*p
		glow *= 0.18 + 0.82*p
	}
	if b.clearTicks > 0 && b.clearTotal > 0 {
		p := phaseProgress(b.clearTotal, b.clearTicks)
		e := math.Sin(math.Pi * p)
		mist *= 1 - 0.86*e
		glow *= 1 - 0.42*e
	}
	if b.endingTicks > 0 && b.endingTotal > 0 {
		p := phaseProgress(b.endingTotal, b.endingTicks)
		dim *= 1 - 0.88*p
		mist *= 1 - p
		glow *= 1 - 0.94*p
	}
	if b.ballDark {
		dim = 0.18
		mist = 0
		glow = 0.02
	}
	return math.Max(0, mist), math.Max(0, glow), math.Max(0, dim)
}

func (b *CrystalBall) paintCrystalBackgroundLocked(grid [][]Pixel) {
	top := hslToRGB(math.Mod(b.cfg.Hue+224, 360), 0.22, 0.045)
	bottom := hslToRGB(math.Mod(b.cfg.Hue+250, 360), 0.20, 0.085)
	for y := 0; y < b.H; y++ {
		t := float64(y) / math.Max(1, float64(b.H-1))
		fillRow(grid, y, mixColor(top, bottom, t))
	}
}

func (b *CrystalBall) paintCrystalStandLocked(grid [][]Pixel, cx, cy, rx, ry, dim float64) {
	baseTop := min(b.H-3, int(math.Round(cy+ry*0.82)))
	baseBottom := min(b.H-1, baseTop+max(4, int(math.Round(ry*0.42))))
	neckTop := min(b.H-1, int(math.Round(cy+ry*0.58)))
	neckBottom := min(b.H-1, baseTop+1)
	standHue := math.Mod(b.cfg.Hue+54, 360)
	neckHalf := max(2, int(math.Round(rx*0.16)))
	for y := neckTop; y <= neckBottom; y++ {
		t := float64(y-neckTop) / math.Max(1, float64(neckBottom-neckTop))
		c := hslToRGB(standHue, 0.23, clamp01((0.12+0.08*t)*dim))
		crystalFillRectAlpha(grid, int(math.Round(cx))-neckHalf, y, neckHalf*2+1, 1, c, 0.96)
	}
	topHalf := int(math.Round(rx * 0.72))
	bottomHalf := int(math.Round(rx * 0.52))
	for y := baseTop; y <= baseBottom; y++ {
		t := float64(y-baseTop) / math.Max(1, float64(baseBottom-baseTop))
		half := int(math.Round(float64(topHalf)*(1-t) + float64(bottomHalf)*t))
		light := (0.20 - 0.09*t) * dim
		c := hslToRGB(standHue, 0.28, clamp01(light))
		crystalFillRectAlpha(grid, int(math.Round(cx))-half, y, half*2+1, 1, c, 1)
		if y <= baseTop+1 {
			hi := hslToRGB(standHue+8, 0.22, clamp01(0.38*dim))
			crystalFillRectAlpha(grid, int(math.Round(cx))-topHalf, y, topHalf*2+1, 1, hi, 0.38)
		}
	}
	labelHalf := max(3, bottomHalf/3)
	labelY := baseTop + max(2, (baseBottom-baseTop)/2)
	label := hslToRGB(standHue+12, 0.20, clamp01(0.36*dim))
	crystalFillRectAlpha(grid, int(math.Round(cx))-labelHalf, labelY, labelHalf*2+1, 1, label, 0.48)
}

func (b *CrystalBall) paintCrystalOrbBodyLocked(grid [][]Pixel, cx, cy, rx, ry, glow, dim float64) {
	top := int(math.Floor(cy - ry - 1))
	bottom := int(math.Ceil(cy + ry + 1))
	left := int(math.Floor(cx - rx - 1))
	right := int(math.Ceil(cx + rx + 1))
	for y := max(0, top); y <= min(b.H-1, bottom); y++ {
		for x := max(0, left); x <= min(b.W-1, right); x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			d2 := dx*dx + dy*dy
			if d2 > 1 {
				continue
			}
			radial := 1 - math.Sqrt(d2)
			vertical := clamp01((float64(y) - (cy - ry)) / math.Max(1, 2*ry))
			light := (0.065 + 0.14*vertical + 0.28*radial + 0.16*glow) * dim
			sat := 0.32
			if b.cfg.Hue > 190 && b.cfg.Hue < 245 {
				sat = 0.25
			}
			c := hslToRGB(math.Mod(b.cfg.Hue+10*math.Sin(float64(x)*0.10), 360), sat, clamp01(light))
			alpha := 0.92
			if d2 > 0.78 {
				alpha = 0.78
			}
			crystalBlendPixel(grid, x, y, c, alpha)
		}
	}
}

func (b *CrystalBall) paintCrystalMistLocked(grid [][]Pixel, cx, cy, rx, ry, mist, glow float64) {
	if mist <= 0 {
		return
	}
	swirl := b.cfg.Swirl
	if b.swirlTicks > 0 && b.swirlTotal > 0 {
		p := phaseProgress(b.swirlTotal, b.swirlTicks)
		swirl *= 1 + 1.25*math.Sin(math.Pi*p)
	}
	phase := float64(b.tick) * 0.019 * swirl
	arms := 3
	points := max(34, int(math.Round(72*mist)))
	for arm := 0; arm < arms; arm++ {
		offset := float64(arm) * math.Pi * 2 / float64(arms)
		for i := 0; i < points; i++ {
			t := float64(i) / math.Max(1, float64(points-1))
			radius := 0.09 + 0.74*t
			angle := offset + phase + t*5.6 + math.Sin(float64(b.tick)*0.011+float64(i)*0.17+offset)*0.36
			x := cx + math.Cos(angle)*rx*radius
			y := cy + math.Sin(angle)*ry*radius*0.72 + math.Cos(angle*0.7)*ry*0.05
			if !crystalInsideOrb(x, y, cx, cy, rx*0.90, ry*0.90) {
				continue
			}
			fade := math.Sin(math.Pi * t)
			alpha := clamp01((0.11 + 0.28*fade) * mist * (0.72 + 0.25*glow))
			hue := math.Mod(b.cfg.Hue+24*math.Sin(t*math.Pi*2+float64(arm)), 360)
			c := hslToRGB(hue, 0.24, clamp01(0.46+0.22*glow))
			crystalBlendPixel(grid, int(math.Round(x)), int(math.Round(y)), c, alpha)
			if i%7 == 0 {
				crystalBlendPixel(grid, int(math.Round(x))+1, int(math.Round(y)), c, alpha*0.44)
			}
		}
	}

	count := max(10, int(math.Round(rx*ry*0.045*mist)))
	for i := 0; i < count; i++ {
		baseAngle := crystalHash(b.mistSeed, 1000+i) * math.Pi * 2
		baseRadius := math.Sqrt(crystalHash(b.mistSeed, 2000+i)) * 0.78
		dir := 1.0
		if crystalHash(b.mistSeed, 3000+i) < 0.5 {
			dir = -1
		}
		angle := baseAngle + dir*phase*(0.55+crystalHash(b.mistSeed, 4000+i)*0.65)
		angle += math.Sin(float64(b.tick)*0.017+float64(i)) * 0.32
		radius := baseRadius * (0.90 + 0.10*math.Sin(float64(b.tick)*0.021+float64(i)*0.41))
		x := cx + math.Cos(angle)*rx*radius
		y := cy + math.Sin(angle)*ry*radius*0.76
		alpha := clamp01((0.12 + crystalHash(b.mistSeed, 5000+i)*0.18) * mist)
		hue := math.Mod(b.cfg.Hue+(crystalHash(b.mistSeed, 6000+i)*2-1)*34, 360)
		c := hslToRGB(hue, 0.16, clamp01(0.55+0.18*glow))
		crystalBlendPixel(grid, int(math.Round(x)), int(math.Round(y)), c, alpha)
		if crystalHash(b.mistSeed, 7000+i) > 0.86 {
			crystalBlendPixel(grid, int(math.Round(x))+1, int(math.Round(y)), c, alpha*0.54)
		}
	}
}

func (b *CrystalBall) paintCrystalVisionLocked(grid [][]Pixel, cx, cy, rx, ry, mist, glow float64) {
	if b.visionTicks <= 0 || b.visionTotal <= 0 || b.visionKind < 0 {
		return
	}
	p := phaseProgress(b.visionTotal, b.visionTicks)
	envelope := math.Sin(math.Pi * p)
	if envelope <= 0 {
		return
	}
	localX := cx + (crystalHash(b.visionSeed, 10)-0.5)*rx*0.18
	localY := cy + (crystalHash(b.visionSeed, 11)-0.5)*ry*0.12
	size := math.Max(3, math.Min(rx, ry)*0.25)
	alpha := clamp01((0.42 + 0.22*glow) * envelope * (0.78 + 0.18*mist))
	c := hslToRGB(math.Mod(b.cfg.Hue+36, 360), 0.18, clamp01(0.72+0.18*glow))
	paint := func(x, y int, a float64) {
		px := int(math.Round(localX)) + x
		py := int(math.Round(localY)) + y
		if crystalInsideOrb(float64(px), float64(py), cx, cy, rx*0.82, ry*0.82) {
			crystalBlendPixel(grid, px, py, c, alpha*a)
		}
	}

	switch b.visionKind {
	case 0:
		half := max(2, int(math.Round(size)))
		for x := -half; x <= half; x++ {
			t := float64(x) / math.Max(1, float64(half))
			y := int(math.Round(math.Sin((t+1)*math.Pi) * size * 0.28))
			paint(x, y, 0.9)
			paint(x, -y, 0.62)
		}
		paint(0, 0, 1)
		paint(0, 1, 0.75)
	case 1:
		r := max(3, int(math.Round(size)))
		for a := -70; a <= 100; a += 12 {
			ang := float64(a) * math.Pi / 180
			x := int(math.Round(math.Cos(ang) * float64(r) * 0.62))
			y := int(math.Round(math.Sin(ang) * float64(r)))
			paint(x, y, 0.88)
			paint(x+1, y, 0.48)
		}
	case 2:
		h := max(5, int(math.Round(size*1.25)))
		for y := -h; y <= h/2; y++ {
			width := max(1, int(math.Round(float64(h-y)*0.10)))
			for x := -width; x <= width; x++ {
				paint(x, y, 0.78)
			}
		}
		for x := -h / 3; x <= h/3; x++ {
			paint(x, -h-1+absInt(x)/2, 0.88)
		}
	default:
		r := max(3, int(math.Round(size)))
		for i := -r; i <= r; i++ {
			paint(i, 0, 0.82)
			paint(0, i, 0.82)
			if absInt(i) <= r/2 {
				paint(i, i, 0.60)
				paint(i, -i, 0.60)
			}
		}
	}
}

func (b *CrystalBall) paintCrystalOrbRimLocked(grid [][]Pixel, cx, cy, rx, ry, dim float64) {
	top := int(math.Floor(cy - ry - 2))
	bottom := int(math.Ceil(cy + ry + 2))
	left := int(math.Floor(cx - rx - 2))
	right := int(math.Ceil(cx + rx + 2))
	rim := hslToRGB(math.Mod(b.cfg.Hue+10, 360), 0.20, clamp01(0.58*dim))
	edge := hslToRGB(math.Mod(b.cfg.Hue+24, 360), 0.12, clamp01(0.82*dim))
	for y := max(0, top); y <= min(b.H-1, bottom); y++ {
		for x := max(0, left); x <= min(b.W-1, right); x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			d2 := dx*dx + dy*dy
			if d2 >= 0.88 && d2 <= 1.08 {
				alpha := 0.36
				if d2 > 0.98 {
					alpha = 0.46
				}
				crystalBlendPixel(grid, x, y, rim, alpha)
			}
		}
	}
	highlightLen := int(math.Round(rx * 0.86))
	for i := 0; i < highlightLen; i++ {
		t := float64(i) / math.Max(1, float64(highlightLen-1))
		x := int(math.Round(cx - rx*0.50 + t*rx*0.72))
		y := int(math.Round(cy - ry*0.56 + math.Sin(t*math.Pi)*ry*0.08))
		if crystalInsideOrb(float64(x), float64(y), cx, cy, rx, ry) {
			crystalBlendPixel(grid, x, y, edge, 0.34)
			if i%4 == 0 {
				crystalBlendPixel(grid, x, y+1, edge, 0.13)
			}
		}
	}
}

func crystalInsideOrb(x, y, cx, cy, rx, ry float64) bool {
	dx := (x - cx) / rx
	dy := (y - cy) / ry
	return dx*dx+dy*dy <= 1
}

func crystalFillRectAlpha(grid [][]Pixel, x0, y0, w, h int, c color.RGBA, alpha float64) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			crystalBlendPixel(grid, x, y, c, alpha)
		}
	}
}

func crystalBlendPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	if alpha >= 1 || !grid[y][x].Filled {
		grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}}
		return
	}
	prev := grid[y][x].C
	grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(prev.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(prev.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(prev.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func crystalHash(seed uint64, salt int) float64 {
	x := seed + uint64(salt)*0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return float64(x&0x1fffffffffffff) / float64(1<<53)
}
