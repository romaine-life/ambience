package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	LavaBlobBase    = "base"
	LavaBlobRising  = "rising"
	LavaBlobFalling = "falling"
	LavaBlobSurface = "surface"
)

// LavaLampConfig tunes a classic lava-lamp silhouette: a dim bottle, warm
// base glow, and a handful of viscous blobs that rise, merge, split, and sink.
type LavaLampConfig struct {
	// INTRODUCTION
	IntroDur        int `json:"intro_dur"`
	IntroFirstDelay int `json:"intro_first"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingSettle int     `json:"ending_settle"`
	EndingGlow   float64 `json:"ending_glow"`
	// BLOB RHYTHM
	MinBlobs    int     `json:"min_blobs"`
	MaxBlobs    int     `json:"max_blobs"`
	BlobMin     float64 `json:"blob_min"`
	BlobMax     float64 `json:"blob_max"`
	DetachEvery int     `json:"detach_every"`
	SurfaceHold int     `json:"surface_hold"`
	MergeDist   float64 `json:"merge_dist"`
	SplitMin    float64 `json:"split_min"`
	// MOTION
	RiseSpeed float64 `json:"rise"`
	FallSpeed float64 `json:"fall"`
	Drift     float64 `json:"drift"`
	Viscosity float64 `json:"viscosity"`
	// PALETTE
	Hue         float64 `json:"hue"`
	HueSpread   float64 `json:"hue_sp"`
	Saturation  float64 `json:"sat"`
	LiquidLight float64 `json:"liquid_light"`
	BlobLight   float64 `json:"blob_light"`
	GlassLight  float64 `json:"glass_light"`
	HeatGlow    float64 `json:"heat_glow"`
	// EVENT CHANCES
	BlobRiseChance   float64 `json:"blob_rise_p"`
	MergeChance      float64 `json:"merge_p"`
	SplitChance      float64 `json:"split_p"`
	SurfacePopChance float64 `json:"surface_pop_p"`
	QuietFlowChance  float64 `json:"quiet_flow_p"`
	// EVENT MODIFIERS
	QuietDur  int     `json:"quiet_dur"`
	QuietMult float64 `json:"quiet_mult"`
}

func (c LavaLampConfig) withDefaults() LavaLampConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 180
	}
	if c.IntroFirstDelay <= 0 {
		c.IntroFirstDelay = 55
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 170
	}
	if c.EndingSettle <= 0 {
		c.EndingSettle = 190
	}
	if c.EndingGlow <= 0 {
		c.EndingGlow = 0.08
	}
	c.EndingGlow = clamp01(c.EndingGlow)
	if c.MinBlobs <= 0 {
		c.MinBlobs = 3
	}
	if c.MaxBlobs <= 0 {
		c.MaxBlobs = 5
	}
	if c.MinBlobs > 8 {
		c.MinBlobs = 8
	}
	if c.MaxBlobs > 9 {
		c.MaxBlobs = 9
	}
	if c.MaxBlobs < c.MinBlobs {
		c.MinBlobs, c.MaxBlobs = c.MaxBlobs, c.MinBlobs
	}
	if c.BlobMin <= 0 {
		c.BlobMin = 5.1
	}
	if c.BlobMax <= 0 {
		c.BlobMax = 10.6
	}
	if c.BlobMax < c.BlobMin {
		c.BlobMin, c.BlobMax = c.BlobMax, c.BlobMin
	}
	if c.DetachEvery <= 0 {
		c.DetachEvery = 220
	}
	if c.SurfaceHold <= 0 {
		c.SurfaceHold = 48
	}
	if c.MergeDist <= 0 {
		c.MergeDist = 0.86
	}
	if c.SplitMin <= 0 {
		c.SplitMin = 8.2
	}
	if c.RiseSpeed <= 0 {
		c.RiseSpeed = 0.058
	}
	if c.FallSpeed <= 0 {
		c.FallSpeed = 0.043
	}
	if c.Drift <= 0 {
		c.Drift = 0.10
	}
	if c.Viscosity <= 0 {
		c.Viscosity = 0.038
	}
	if c.Hue == 0 {
		c.Hue = 7
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.HueSpread == 0 {
		c.HueSpread = 8
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.88
	}
	c.Saturation = clamp01(c.Saturation)
	if c.LiquidLight <= 0 {
		c.LiquidLight = 0.10
	}
	c.LiquidLight = clamp01(c.LiquidLight)
	if c.BlobLight <= 0 {
		c.BlobLight = 0.62
	}
	c.BlobLight = clamp01(c.BlobLight)
	if c.GlassLight <= 0 {
		c.GlassLight = 0.22
	}
	c.GlassLight = clamp01(c.GlassLight)
	if c.HeatGlow <= 0 {
		c.HeatGlow = 0.84
	}
	c.HeatGlow = clamp01(c.HeatGlow)
	if c.BlobRiseChance < 0 {
		c.BlobRiseChance = 0
	}
	if c.MergeChance < 0 {
		c.MergeChance = 0
	}
	if c.SplitChance < 0 {
		c.SplitChance = 0
	}
	if c.SurfacePopChance < 0 {
		c.SurfacePopChance = 0
	}
	if c.QuietFlowChance < 0 {
		c.QuietFlowChance = 0
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 420
	}
	if c.QuietMult <= 0 {
		c.QuietMult = 0.28
	}
	c.QuietMult = clamp01(c.QuietMult)
	return c
}

// LavaLampSchema describes the Lava Lamp effect's tunable knobs.
func LavaLampSchema() EffectSchema {
	return EffectSchema{
		Name:           "lava-lamp",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 30, Max: 520, Step: 10, Default: 180, Trigger: "intro",
				Description: "Ticks spent warming the bottle from dim glass into steady flow."},
			{Key: "intro_first", Label: "first blob", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 5, Max: 220, Step: 5, Default: 55,
				Description: "Delay before the first blob detaches during the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 40, Max: 520, Step: 10, Default: 170, Trigger: "ending",
				Description: "Ticks spent fading the heat source and liquid glow."},
			{Key: "ending_settle", Label: "settle", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 40, Max: 620, Step: 10, Default: 190,
				Description: "Extra ticks for in-flight blobs to sink back to the base."},
			{Key: "ending_glow", Label: "residual", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual base glow held after the terminal ending settles."},
			{Key: "min_blobs", Label: "min blobs", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 2, Max: 6, Step: 1, Default: 3,
				Description: "Minimum blob count kept in the lamp."},
			{Key: "max_blobs", Label: "max blobs", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 3, Max: 8, Step: 1, Default: 5,
				Description: "Maximum blob count before merges become likely."},
			{Key: "blob_min", Label: "blob min", Slot: SlotSpawn, Group: "shape", Type: KnobFloat, Min: 3.8, Max: 8.2, Step: 0.1, Default: 5.1,
				Description: "Smallest blob radius in grid cells."},
			{Key: "blob_max", Label: "blob max", Slot: SlotSpawn, Group: "shape", Type: KnobFloat, Min: 7, Max: 15, Step: 0.1, Default: 10.6,
				Description: "Largest ordinary blob radius before split events become readable."},
			{Key: "detach_every", Label: "detach every", Slot: SlotLever, Group: "rhythm", Type: KnobInt, Min: 80, Max: 720, Step: 10, Default: 220,
				Description: "Typical rest ticks before a base blob slowly detaches and rises."},
			{Key: "surface_hold", Label: "surface hold", Slot: SlotEventMod, Group: "surface-pop", Type: KnobInt, Min: 12, Max: 140, Step: 4, Default: 48,
				Description: "Ticks a blob spends flattened against the top before sinking."},
			{Key: "merge_dist", Label: "merge dist", Slot: SlotEventMod, Group: "blob-merge", Type: KnobFloat, Min: 0.55, Max: 1.35, Step: 0.01, Default: 0.86,
				Description: "Distance multiplier used to decide whether adjacent blobs can merge."},
			{Key: "split_min", Label: "split min", Slot: SlotEventMod, Group: "blob-split", Type: KnobFloat, Min: 6, Max: 16, Step: 0.1, Default: 8.2,
				Description: "Minimum radius for a blob to split into two slower blobs."},
			{Key: "rise", Label: "rise speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.018, Max: 0.16, Step: 0.002, Default: 0.058,
				Description: "Upward rows per tick for warm rising blobs."},
			{Key: "fall", Label: "fall speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.014, Max: 0.13, Step: 0.002, Default: 0.043,
				Description: "Downward rows per tick for cooling sinking blobs."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.01, Max: 0.45, Step: 0.01, Default: 0.10,
				Description: "Slow sideways wander applied inside the bottle silhouette."},
			{Key: "viscosity", Label: "viscosity", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.012, Max: 0.12, Step: 0.002, Default: 0.038,
				Description: "How slowly blob velocity eases toward the current rise or sink speed."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 4, Max: 165, Step: 1, Default: 7,
				Description: "Scene palette knob: red, blue, green, or in-between goo colors."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "palette", Type: KnobFloat, Min: 0, Max: 34, Step: 1, Default: 8,
				Description: "Per-blob hue variation around the main palette color."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.25, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Saturation of blobs, base heat, and liquid tint."},
			{Key: "liquid_light", Label: "liquid", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.03, Max: 0.28, Step: 0.01, Default: 0.10,
				Description: "Dark liquid lightness inside the glass bottle."},
			{Key: "blob_light", Label: "blob light", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.25, Max: 0.88, Step: 0.01, Default: 0.62,
				Description: "Lightness of the suspended lava blobs."},
			{Key: "glass_light", Label: "glass", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.08, Max: 0.55, Step: 0.01, Default: 0.22,
				Description: "Brightness of the bottle outline and shoulder highlights."},
			{Key: "heat_glow", Label: "heat glow", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.08, Max: 1, Step: 0.01, Default: 0.84,
				Description: "Strength of the warm lamp base glow."},
			{Key: "blob_rise_p", Label: "blob rise", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.002, Step: 0.00005, Default: 0.00045, Trigger: "blob-rise",
				Description: "Per-tick chance of an extra base blob detaching."},
			{Key: "merge_p", Label: "merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.0015, Step: 0.00005, Default: 0.00025, Trigger: "blob-merge",
				Description: "Per-tick chance that nearby blobs combine into one larger blob."},
			{Key: "split_p", Label: "split", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.0015, Step: 0.00005, Default: 0.00022, Trigger: "blob-split",
				Description: "Per-tick chance a large moving blob splits into two smaller blobs."},
			{Key: "surface_pop_p", Label: "surface pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.001, Step: 0.00005, Default: 0.00012, Trigger: "surface-pop",
				Description: "Per-tick chance the highest blob flattens at the top and starts sinking."},
			{Key: "quiet_flow_p", Label: "quiet flow", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.0008, Step: 0.00002, Default: 0.00004, Trigger: "quiet-flow",
				Description: "Per-tick chance of a long low-detachment settling window."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobInt, Min: 120, Max: 1400, Step: 20, Default: 420,
				Description: "Duration of a quiet-flow suppression window."},
			{Key: "quiet_mult", Label: "quiet mult", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobFloat, Min: 0.02, Max: 0.75, Step: 0.01, Default: 0.28,
				Description: "Detachment probability multiplier while quiet-flow is active."},
		},
	}
}

type LavaBlob struct {
	ID           int     `json:"id"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	VX           float64 `json:"vx"`
	VY           float64 `json:"vy"`
	R            float64 `json:"r"`
	Mode         string  `json:"mode"`
	Phase        float64 `json:"phase"`
	RestTicks    int     `json:"restTicks,omitempty"`
	SurfaceTicks int     `json:"surfaceTicks,omitempty"`
	HueOffset    float64 `json:"hueOffset"`
	Age          int     `json:"age"`
}

type LavaLampState struct {
	Tick               int        `json:"tick"`
	Blobs              []LavaBlob `json:"blobs"`
	NextBlobID         int        `json:"nextBlobID"`
	IntroTicks         int        `json:"introTicks"`
	IntroTotal         int        `json:"introTotal"`
	IntroFirstLaunched bool       `json:"introFirstLaunched"`
	EndingTicks        int        `json:"endingTicks"`
	EndingTotal        int        `json:"endingTotal"`
	Ended              bool       `json:"ended"`
	QuietTicks         int        `json:"quietTicks"`
	Lifecycle          Lifecycle  `json:"lifecycle"`
	RNGState           uint64     `json:"rngState,omitempty"`
}

type LavaLampSnapshot struct {
	LavaLampState
}

type LavaLampPersistedState struct {
	LavaLampState
}

type LavaLamp struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  LavaLampConfig
	tick int

	blobs      []LavaBlob
	nextBlobID int

	introTicks         int
	introTotal         int
	introFirstLaunched bool
	endingTicks        int
	endingTotal        int
	ended              bool
	quietTicks         int

	log []LogEntry
}

func NewLavaLamp(w, h int, seed int64, cfg LavaLampConfig) *LavaLamp {
	l := &LavaLamp{
		W:          w,
		H:          h,
		rng:        rngutil.New(seed),
		cfg:        cfg.withDefaults(),
		nextBlobID: 1,
	}
	l.seedInitialBlobsLocked()
	return l
}

func (l *LavaLamp) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.W = w
	l.H = h
	l.clampBlobsToBottleLocked()
}

func (l *LavaLamp) SetConfig(cfg LavaLampConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev := l.cfg
	l.cfg = cfg.withDefaults()
	if l.cfg.MaxBlobs != prev.MaxBlobs || l.cfg.MinBlobs != prev.MinBlobs {
		l.trimBlobsLocked()
		l.ensureBlobCountLocked()
	}
	l.clampBlobSizesLocked()
	l.clampBlobsToBottleLocked()
}

func (l *LavaLamp) EffectiveConfig() LavaLampConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cfg
}

func (l *LavaLamp) CurrentTick() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tick
}

func (l *LavaLamp) PerturbRNG(delta int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rng.Mix(delta)
}

func (l *LavaLamp) DrainLog() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.log) == 0 {
		return nil
	}
	out := l.log
	l.log = nil
	return out
}

func (l *LavaLamp) appendLog(kind, desc string) {
	l.log = append(l.log, LogEntry{Tick: l.tick, Type: kind, Desc: desc})
	if len(l.log) > 200 {
		l.log = l.log[len(l.log)-200:]
	}
}

func (l *LavaLamp) Snapshot() LavaLampSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LavaLampSnapshot{LavaLampState: l.snapshotStateLocked(true)}
}

func (l *LavaLamp) RestoreSnapshot(s LavaLampSnapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(s.LavaLampState)
}

func (l *LavaLamp) SnapshotPersistedState() LavaLampPersistedState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LavaLampPersistedState{LavaLampState: l.snapshotStateLocked(true)}
}

func (l *LavaLamp) RestorePersistedState(s LavaLampPersistedState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(s.LavaLampState)
}

func (l *LavaLamp) snapshotStateLocked(includeRNG bool) LavaLampState {
	blobs := make([]LavaBlob, len(l.blobs))
	copy(blobs, l.blobs)
	out := LavaLampState{
		Tick:               l.tick,
		Blobs:              blobs,
		NextBlobID:         l.nextBlobID,
		IntroTicks:         l.introTicks,
		IntroTotal:         l.introTotal,
		IntroFirstLaunched: l.introFirstLaunched,
		EndingTicks:        l.endingTicks,
		EndingTotal:        l.endingTotal,
		Ended:              l.ended,
		QuietTicks:         l.quietTicks,
		Lifecycle:          l.lifecycleLocked(),
	}
	if includeRNG {
		out.RNGState = l.rng.State()
	}
	return out
}

func (l *LavaLamp) restoreStateLocked(s LavaLampState) {
	l.tick = s.Tick
	l.blobs = append([]LavaBlob(nil), s.Blobs...)
	l.nextBlobID = s.NextBlobID
	if l.nextBlobID <= 0 {
		for _, b := range l.blobs {
			if b.ID >= l.nextBlobID {
				l.nextBlobID = b.ID + 1
			}
		}
		if l.nextBlobID <= 0 {
			l.nextBlobID = 1
		}
	}
	l.introTicks = s.IntroTicks
	l.introTotal = s.IntroTotal
	l.introFirstLaunched = s.IntroFirstLaunched
	l.endingTicks = s.EndingTicks
	l.endingTotal = s.EndingTotal
	l.ended = s.Ended
	l.quietTicks = s.QuietTicks
	if s.RNGState != 0 {
		l.rng.SetState(s.RNGState)
	}
	for i := range l.blobs {
		if !validLavaBlobMode(l.blobs[i].Mode) {
			l.blobs[i].Mode = LavaBlobBase
		}
	}
	if len(l.blobs) == 0 {
		l.ensureBlobCountLocked()
	}
	l.trimBlobsLocked()
	l.clampBlobSizesLocked()
	l.clampBlobsToBottleLocked()
}

func validLavaBlobMode(mode string) bool {
	switch mode {
	case LavaBlobBase, LavaBlobRising, LavaBlobFalling, LavaBlobSurface:
		return true
	default:
		return false
	}
}

func (l *LavaLamp) lifecycleLocked() Lifecycle {
	switch {
	case l.introTicks > 0:
		return LifecycleIntro
	case l.endingTicks > 0:
		return LifecycleEnding
	case l.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (l *LavaLamp) TriggerEvent(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch name {
	case "blob-rise":
		if b, ok := l.detachOneLocked(true); ok {
			l.appendLog("blob-rise", fmt.Sprintf("blob %d detached", b.ID))
		} else {
			l.appendLog("blob-rise", "skipped (no base blob)")
		}
	case "blob-merge":
		if merged := l.forceMergeLocked(); merged != 0 {
			l.appendLog("blob-merge", fmt.Sprintf("blob %d absorbed a neighbor", merged))
		} else {
			l.appendLog("blob-merge", "skipped (no adjacent blobs)")
		}
	case "blob-split":
		if split := l.forceSplitLocked(); split != 0 {
			l.appendLog("blob-split", fmt.Sprintf("blob %d split", split))
		} else {
			l.appendLog("blob-split", "skipped (no large blob)")
		}
	case "surface-pop":
		if popped := l.forceSurfacePopLocked(); popped != 0 {
			l.appendLog("surface-pop", fmt.Sprintf("blob %d flattened", popped))
		} else {
			l.appendLog("surface-pop", "skipped (no moving blob)")
		}
	case "quiet-flow":
		l.startQuietLocked()
		l.appendLog("quiet-flow", fmt.Sprintf("settling window (%d ticks)", l.quietTicks))
	case "intro":
		l.startIntroLocked()
		l.appendLog("intro", fmt.Sprintf("warm-up (dur=%d, first=%d)", l.introTotal, l.cfg.IntroFirstDelay))
	case "ending":
		l.startEndingLocked()
		l.appendLog("ending", fmt.Sprintf("cool-down (dur=%d)", l.endingTotal))
	default:
		return false
	}
	return true
}

func (l *LavaLamp) Step() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tick++
	l.trimBlobsLocked()
	if !l.ended {
		l.ensureBlobCountLocked()
	}

	if l.introTicks > 0 {
		elapsed := l.introTotal - l.introTicks
		if !l.introFirstLaunched && elapsed >= l.cfg.IntroFirstDelay {
			if _, ok := l.detachOneLocked(false); ok {
				l.introFirstLaunched = true
			}
		}
		l.introTicks--
		if l.introTicks == 0 {
			l.ended = false
		}
	}
	if l.endingTicks > 0 {
		l.endingTicks--
		if l.endingTicks == 0 {
			l.finishEndingLocked()
		}
	}
	if l.quietTicks > 0 {
		l.quietTicks--
	}
	if l.lifecycleLocked() == LifecycleRunning && l.quietTicks == 0 {
		l.ensureSuspendedFlowLocked()
	}

	for i := range l.blobs {
		l.advanceBlobLocked(i)
	}

	if l.lifecycleLocked() == LifecycleRunning {
		l.rollAutonomousEventsLocked()
	}
}

func (l *LavaLamp) rollAutonomousEventsLocked() {
	quietMult := 1.0
	if l.quietTicks > 0 {
		quietMult = l.cfg.QuietMult
	}
	if l.cfg.BlobRiseChance > 0 && l.rng.Float64() < l.cfg.BlobRiseChance*quietMult {
		if b, ok := l.detachOneLocked(true); ok {
			l.appendLog("blob-rise", fmt.Sprintf("blob %d detached", b.ID))
		}
	}
	if l.cfg.MergeChance > 0 && l.rng.Float64() < l.cfg.MergeChance {
		if id := l.forceMergeLocked(); id != 0 {
			l.appendLog("blob-merge", fmt.Sprintf("blob %d absorbed a neighbor", id))
		}
	}
	if l.cfg.SplitChance > 0 && l.rng.Float64() < l.cfg.SplitChance {
		if id := l.forceSplitLocked(); id != 0 {
			l.appendLog("blob-split", fmt.Sprintf("blob %d split", id))
		}
	}
	if l.cfg.SurfacePopChance > 0 && l.rng.Float64() < l.cfg.SurfacePopChance {
		if id := l.forceSurfacePopLocked(); id != 0 {
			l.appendLog("surface-pop", fmt.Sprintf("blob %d flattened", id))
		}
	}
	if l.cfg.QuietFlowChance > 0 && l.rng.Float64() < l.cfg.QuietFlowChance {
		l.startQuietLocked()
		l.appendLog("quiet-flow", fmt.Sprintf("settling window (%d ticks)", l.quietTicks))
	}
}

func (l *LavaLamp) seedInitialBlobsLocked() {
	l.blobs = nil
	target := l.cfg.MinBlobs + l.rng.Intn(max(1, l.cfg.MaxBlobs-l.cfg.MinBlobs+1))
	_, top, bottom, _, body := lavaBottleGeometry(l.W, l.H)
	for len(l.blobs) < target {
		l.blobs = append(l.blobs, l.newBaseBlobLocked())
	}
	for i := range l.blobs {
		b := &l.blobs[i]
		switch i % 3 {
		case 0:
			b.Mode = LavaBlobRising
			b.Y = bottom - (bottom-top)*(0.18+0.42*l.rng.Float64())
			b.VY = -l.cfg.RiseSpeed * (0.6 + l.rng.Float64()*0.5)
		case 1:
			b.Mode = LavaBlobFalling
			b.Y = top + (bottom-top)*(0.18+0.45*l.rng.Float64())
			b.VY = l.cfg.FallSpeed * (0.5 + l.rng.Float64()*0.5)
		default:
			b.Mode = LavaBlobBase
			b.Y = bottom - b.R*0.35
			b.RestTicks = jitterInt(l.rng, l.cfg.DetachEvery, 0.55)
		}
		b.X += (l.rng.Float64()*2 - 1) * body * 0.28
	}
	l.clampBlobsToBottleLocked()
}

func (l *LavaLamp) ensureBlobCountLocked() {
	for len(l.blobs) < l.cfg.MinBlobs {
		l.blobs = append(l.blobs, l.newBaseBlobLocked())
	}
}

func (l *LavaLamp) trimBlobsLocked() {
	if len(l.blobs) <= l.cfg.MaxBlobs {
		return
	}
	l.blobs = l.blobs[:l.cfg.MaxBlobs]
}

func (l *LavaLamp) ensureSuspendedFlowLocked() {
	target := l.cfg.MinBlobs - 1
	if target < 2 {
		target = 2
	}
	if target > 4 {
		target = 4
	}
	for l.countSuspendedLocked() < target {
		if _, ok := l.detachOneLocked(true); !ok {
			return
		}
	}
}

func (l *LavaLamp) countSuspendedLocked() int {
	count := 0
	for _, b := range l.blobs {
		if b.Mode != LavaBlobBase {
			count++
		}
	}
	return count
}

func (l *LavaLamp) clampBlobSizesLocked() {
	for i := range l.blobs {
		if l.blobs[i].R < l.cfg.BlobMin {
			l.blobs[i].R = l.cfg.BlobMin
		}
		if l.blobs[i].R > l.cfg.BlobMax*1.6 {
			l.blobs[i].R = l.cfg.BlobMax * 1.6
		}
	}
}

func (l *LavaLamp) newBaseBlobLocked() LavaBlob {
	cx, _, bottom, _, body := lavaBottleGeometry(l.W, l.H)
	r := l.cfg.BlobMin + l.rng.Float64()*math.Max(0.1, l.cfg.BlobMax-l.cfg.BlobMin)
	if body > 0 {
		r = math.Min(r, body*0.42)
	}
	id := l.nextBlobID
	l.nextBlobID++
	return LavaBlob{
		ID:        id,
		X:         cx + (l.rng.Float64()*2-1)*body*0.22,
		Y:         bottom - r*0.35,
		R:         r,
		Mode:      LavaBlobBase,
		Phase:     l.rng.Float64() * math.Pi * 2,
		RestTicks: jitterInt(l.rng, l.cfg.DetachEvery, 0.55),
		HueOffset: (l.rng.Float64()*2 - 1) * l.cfg.HueSpread,
	}
}

func (l *LavaLamp) detachOneLocked(force bool) (LavaBlob, bool) {
	if l.ended || l.endingTicks > 0 {
		return LavaBlob{}, false
	}
	best := -1
	for i := range l.blobs {
		if l.blobs[i].Mode != LavaBlobBase {
			continue
		}
		if !force && l.blobs[i].RestTicks > 0 {
			continue
		}
		if best < 0 || l.blobs[i].RestTicks < l.blobs[best].RestTicks {
			best = i
		}
	}
	if best < 0 && len(l.blobs) < l.cfg.MaxBlobs {
		l.blobs = append(l.blobs, l.newBaseBlobLocked())
		best = len(l.blobs) - 1
	}
	if best < 0 {
		return LavaBlob{}, false
	}
	b := &l.blobs[best]
	b.Mode = LavaBlobRising
	b.RestTicks = 0
	b.SurfaceTicks = 0
	b.VY = -l.cfg.RiseSpeed * (0.4 + l.rng.Float64()*0.35)
	b.VX += (l.rng.Float64()*2 - 1) * l.cfg.Drift
	b.Age = 0
	return *b, true
}

func (l *LavaLamp) forceMergeLocked() int {
	if len(l.blobs) < 2 {
		return 0
	}
	bestI, bestJ := -1, -1
	bestScore := math.MaxFloat64
	for i := 0; i < len(l.blobs); i++ {
		for j := i + 1; j < len(l.blobs); j++ {
			if l.blobs[i].Mode == LavaBlobBase && l.blobs[j].Mode == LavaBlobBase {
				continue
			}
			d := math.Hypot(l.blobs[i].X-l.blobs[j].X, l.blobs[i].Y-l.blobs[j].Y)
			limit := (l.blobs[i].R + l.blobs[j].R) * l.cfg.MergeDist
			score := d / math.Max(1, limit)
			if score < bestScore {
				bestScore = score
				bestI, bestJ = i, j
			}
		}
	}
	if bestI < 0 || bestJ < 0 || bestScore > 1.45 {
		return 0
	}
	a := &l.blobs[bestI]
	b := l.blobs[bestJ]
	areaA := a.R * a.R
	areaB := b.R * b.R
	total := areaA + areaB
	a.X = (a.X*areaA + b.X*areaB) / total
	a.Y = (a.Y*areaA + b.Y*areaB) / total
	a.VX = (a.VX*areaA + b.VX*areaB) / total
	a.VY = (a.VY*areaA + b.VY*areaB) / total
	a.R = math.Min(l.cfg.BlobMax*1.55, math.Sqrt(total)*0.96)
	if a.Mode == LavaBlobBase && b.Mode != LavaBlobBase {
		a.Mode = b.Mode
	}
	a.HueOffset = (a.HueOffset + b.HueOffset) / 2
	l.blobs = append(l.blobs[:bestJ], l.blobs[bestJ+1:]...)
	return a.ID
}

func (l *LavaLamp) forceSplitLocked() int {
	if len(l.blobs) >= l.cfg.MaxBlobs {
		return 0
	}
	best := -1
	for i := range l.blobs {
		if l.blobs[i].R < l.cfg.SplitMin || l.blobs[i].Mode == LavaBlobBase {
			continue
		}
		if best < 0 || l.blobs[i].R > l.blobs[best].R {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	parent := &l.blobs[best]
	childR := math.Max(l.cfg.BlobMin, parent.R*0.68)
	parent.R = childR
	parent.X -= childR * 0.42
	parent.VX -= l.cfg.Drift * (0.35 + l.rng.Float64()*0.3)
	child := *parent
	child.ID = l.nextBlobID
	l.nextBlobID++
	child.X += childR * 1.05
	child.VX = -parent.VX * 0.7
	child.VY += (l.rng.Float64()*2 - 1) * l.cfg.Viscosity
	child.Phase += math.Pi * (0.75 + l.rng.Float64()*0.5)
	child.HueOffset += (l.rng.Float64()*2 - 1) * l.cfg.HueSpread * 0.35
	l.blobs = append(l.blobs, child)
	return parent.ID
}

func (l *LavaLamp) forceSurfacePopLocked() int {
	best := -1
	for i := range l.blobs {
		if l.blobs[i].Mode == LavaBlobBase || l.blobs[i].Mode == LavaBlobSurface {
			continue
		}
		if best < 0 || l.blobs[i].Y < l.blobs[best].Y {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	_, top, _, _, _ := lavaBottleGeometry(l.W, l.H)
	b := &l.blobs[best]
	b.Mode = LavaBlobSurface
	b.Y = top + b.R*0.32
	b.VY = 0
	b.SurfaceTicks = jitterInt(l.rng, l.cfg.SurfaceHold, 0.35)
	return b.ID
}

func (l *LavaLamp) startQuietLocked() {
	l.quietTicks = jitterInt(l.rng, l.cfg.QuietDur, 0.28)
}

func (l *LavaLamp) startIntroLocked() {
	l.introTotal = l.cfg.IntroDur
	l.introTicks = l.introTotal
	l.introFirstLaunched = false
	l.endingTicks = 0
	l.endingTotal = 0
	l.ended = false
	l.quietTicks = 0
	l.blobs = nil
	l.ensureBlobCountLocked()
	for i := range l.blobs {
		l.blobs[i].Mode = LavaBlobBase
		l.blobs[i].VY = 0
		l.blobs[i].VX = 0
		l.blobs[i].RestTicks = l.cfg.IntroFirstDelay + i*jitterInt(l.rng, l.cfg.DetachEvery/3, 0.5)
	}
}

func (l *LavaLamp) startEndingLocked() {
	l.introTicks = 0
	l.introTotal = 0
	l.introFirstLaunched = false
	l.ended = false
	l.quietTicks = 0
	l.endingTotal = l.cfg.EndingDur + l.cfg.EndingSettle
	l.endingTicks = l.endingTotal
	for i := range l.blobs {
		if l.blobs[i].Mode != LavaBlobBase {
			l.blobs[i].Mode = LavaBlobFalling
			l.blobs[i].SurfaceTicks = 0
			if l.blobs[i].VY < 0 {
				l.blobs[i].VY = 0
			}
		}
	}
}

func (l *LavaLamp) finishEndingLocked() {
	l.ended = true
	l.introTicks = 0
	l.endingTicks = 0
	_, _, bottom, _, _ := lavaBottleGeometry(l.W, l.H)
	l.ensureBlobCountLocked()
	for i := range l.blobs {
		l.blobs[i].Mode = LavaBlobBase
		l.blobs[i].Y = bottom - l.blobs[i].R*0.35
		l.blobs[i].VY = 0
		l.blobs[i].VX = 0
		l.blobs[i].RestTicks = l.cfg.DetachEvery
		l.blobs[i].SurfaceTicks = 0
	}
}

func (l *LavaLamp) advanceBlobLocked(i int) {
	if i < 0 || i >= len(l.blobs) {
		return
	}
	b := &l.blobs[i]
	b.Age++
	cx, top, bottom, _, body := lavaBottleGeometry(l.W, l.H)
	heat := l.heatLevelLocked()
	quietMult := 1.0
	if l.quietTicks > 0 {
		quietMult = l.cfg.QuietMult
	}

	switch b.Mode {
	case LavaBlobBase:
		b.Y += (bottom - b.R*0.35 - b.Y) * 0.08
		b.X += (cx + math.Sin(float64(l.tick)*0.009+b.Phase)*body*0.10 - b.X) * 0.015
		b.VX *= 0.85
		b.VY *= 0.85
		if l.lifecycleLocked() == LifecycleRunning {
			if b.RestTicks > 0 {
				b.RestTicks--
			}
			if b.RestTicks <= 0 && l.rng.Float64() < quietMult {
				_, _ = l.detachOneLocked(false)
			}
		}
	case LavaBlobRising:
		targetV := -l.cfg.RiseSpeed * (0.55 + heat*0.65) * (0.86 + 0.18*math.Sin(float64(l.tick)*0.011+b.Phase))
		b.VY += (targetV - b.VY) * l.cfg.Viscosity
		targetX := cx + math.Sin(float64(l.tick)*0.008+b.Phase)*body*l.cfg.Drift
		b.VX += (targetX - b.X) * l.cfg.Viscosity * 0.08
		b.X += b.VX
		b.Y += b.VY
		if b.Y-b.R <= top+1 {
			b.Mode = LavaBlobSurface
			b.Y = top + b.R*0.32
			b.VY = 0
			b.SurfaceTicks = jitterInt(l.rng, l.cfg.SurfaceHold, 0.25)
		}
	case LavaBlobSurface:
		b.Y += (top + b.R*0.32 - b.Y) * 0.12
		b.X += (cx + math.Sin(float64(l.tick)*0.006+b.Phase)*body*0.15 - b.X) * 0.02
		if b.SurfaceTicks > 0 {
			b.SurfaceTicks--
		}
		if b.SurfaceTicks <= 0 || l.endingTicks > 0 {
			b.Mode = LavaBlobFalling
			b.VY = l.cfg.FallSpeed * 0.35
		}
	case LavaBlobFalling:
		targetV := l.cfg.FallSpeed * (0.75 + (1-heat)*0.8)
		if l.endingTicks > 0 || l.ended {
			targetV *= 1.75
		}
		b.VY += (targetV - b.VY) * l.cfg.Viscosity
		targetX := cx + math.Sin(float64(l.tick)*0.006+b.Phase)*body*l.cfg.Drift*0.65
		b.VX += (targetX - b.X) * l.cfg.Viscosity * 0.06
		b.X += b.VX
		b.Y += b.VY
		if b.Y+b.R*0.42 >= bottom {
			b.Mode = LavaBlobBase
			b.Y = bottom - b.R*0.35
			b.VY = 0
			b.VX *= 0.25
			b.RestTicks = jitterInt(l.rng, l.cfg.DetachEvery, 0.55)
		}
	}

	hw := lavaHalfWidthAt(l.W, l.H, b.Y) - b.R*0.55
	if hw < 1 {
		hw = 1
	}
	if b.X < cx-hw {
		b.X = cx - hw
		b.VX *= -0.2
	}
	if b.X > cx+hw {
		b.X = cx + hw
		b.VX *= -0.2
	}
	if b.Y < top+b.R*0.25 {
		b.Y = top + b.R*0.25
	}
	if b.Y > bottom-b.R*0.25 {
		b.Y = bottom - b.R*0.25
	}
}

func (l *LavaLamp) heatLevelLocked() float64 {
	switch {
	case l.introTicks > 0:
		p := phaseProgress(l.introTotal, l.introTicks)
		return clamp01(0.12 + 0.88*p)
	case l.endingTicks > 0:
		p := phaseProgress(l.endingTotal, l.endingTicks)
		return clamp01(1 - (1-l.cfg.EndingGlow)*p)
	case l.ended:
		return l.cfg.EndingGlow
	default:
		return 1
	}
}

func (l *LavaLamp) clampBlobsToBottleLocked() {
	cx, top, bottom, _, _ := lavaBottleGeometry(l.W, l.H)
	for i := range l.blobs {
		b := &l.blobs[i]
		if b.R <= 0 {
			b.R = l.cfg.BlobMin
		}
		if b.Y < top+b.R*0.25 {
			b.Y = top + b.R*0.25
		}
		if b.Y > bottom-b.R*0.25 {
			b.Y = bottom - b.R*0.25
		}
		hw := lavaHalfWidthAt(l.W, l.H, b.Y) - b.R*0.55
		if hw < 1 {
			hw = 1
		}
		if b.X < cx-hw {
			b.X = cx - hw
		}
		if b.X > cx+hw {
			b.X = cx + hw
		}
	}
}

func (l *LavaLamp) GridCopy() [][]Pixel {
	l.mu.Lock()
	defer l.mu.Unlock()
	grid := make([][]Pixel, l.H)
	for y := range grid {
		grid[y] = make([]Pixel, l.W)
	}
	l.renderLocked(grid)
	return grid
}

func (l *LavaLamp) renderLocked(grid [][]Pixel) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	w, h := len(grid[0]), len(grid)
	heat := l.heatLevelLocked()
	cx, top, bottom, neck, body := lavaBottleGeometry(w, h)
	bg := hslToRGB(245, 0.18, 0.035)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			grid[y][x] = Pixel{Filled: true, C: bg}
		}
	}

	for y := max(0, int(top)-4); y < min(h, int(bottom)+8); y++ {
		yy := float64(y) + 0.5
		hw := lavaHalfWidthAt(w, h, yy)
		for x := max(0, int(cx-hw)-4); x < min(w, int(cx+hw)+5); x++ {
			xx := float64(x) + 0.5
			d := math.Abs(xx-cx) - hw
			if d <= 0 {
				v := clamp01((yy - top) / math.Max(1, bottom-top))
				light := l.cfg.LiquidLight*(0.72+0.28*(1-v)) + heat*0.045*(1-v*0.65)
				liquid := hslToRGB(l.cfg.Hue+22, l.cfg.Saturation*0.38, clamp01(light))
				lavaBlendPixel(grid, x, y, liquid, 0.94)
			}
			if math.Abs(d) < 1.45 {
				glass := hslToRGB(l.cfg.Hue+12, 0.18, l.cfg.GlassLight*(0.65+0.35*heat))
				lavaBlendPixel(grid, x, y, glass, 0.78)
			}
		}
	}

	l.paintHeatGlowLocked(grid, cx, bottom, body, heat)
	for _, b := range l.blobs {
		l.paintBlobLocked(grid, b, heat)
	}
	l.paintBottleCapsLocked(grid, cx, top, bottom, neck, body, heat)
}

func (l *LavaLamp) paintHeatGlowLocked(grid [][]Pixel, cx, bottom, body, heat float64) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	glow := hslToRGB(34, 0.95, 0.48)
	core := hslToRGB(42, 1.0, 0.68)
	for y := max(0, int(bottom-body*0.55)); y < h; y++ {
		for x := max(0, int(cx-body*1.55)); x < min(w, int(cx+body*1.55)); x++ {
			dx := (float64(x) + 0.5 - cx) / math.Max(1, body*1.25)
			dy := (float64(y) + 0.5 - bottom) / math.Max(1, body*0.60)
			d := math.Sqrt(dx*dx + dy*dy)
			if d > 1.35 {
				continue
			}
			alpha := (1 - d/1.35) * l.cfg.HeatGlow * heat
			lavaBlendPixel(grid, x, y, glow, alpha*0.75)
			if d < 0.38 {
				lavaBlendPixel(grid, x, y, core, alpha*0.55)
			}
		}
	}
}

func (l *LavaLamp) paintBlobLocked(grid [][]Pixel, b LavaBlob, heat float64) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	rx, ry := b.R, b.R
	switch b.Mode {
	case LavaBlobBase:
		rx *= 1.45
		ry *= 0.52
	case LavaBlobSurface:
		rx *= 1.65
		ry *= 0.43
	}
	c := hslToRGB(l.cfg.Hue+b.HueOffset+math.Sin(float64(l.tick)*0.004+b.Phase)*l.cfg.HueSpread*0.18, l.cfg.Saturation, l.cfg.BlobLight*(0.45+0.55*heat))
	hot := hslToRGB(l.cfg.Hue+b.HueOffset+8, math.Min(1, l.cfg.Saturation*1.05), math.Min(0.95, l.cfg.BlobLight+0.18*heat))
	x0, x1 := int(b.X-rx*1.35), int(b.X+rx*1.35)+1
	y0, y1 := int(b.Y-ry*1.35), int(b.Y+ry*1.35)+1
	for y := max(0, y0); y < min(h, y1); y++ {
		for x := max(0, x0); x < min(w, x1); x++ {
			xx := float64(x) + 0.5
			yy := float64(y) + 0.5
			if !lavaInsideBottle(w, h, xx, yy) {
				continue
			}
			dx := (xx - b.X) / math.Max(0.1, rx)
			dy := (yy - b.Y) / math.Max(0.1, ry)
			d := math.Sqrt(dx*dx + dy*dy)
			if d > 1.34 {
				continue
			}
			if d <= 1 {
				alpha := 0.84 + 0.12*(1-d)
				lavaBlendPixel(grid, x, y, c, alpha)
				if d < 0.45 {
					lavaBlendPixel(grid, x, y, hot, (0.45-d)*0.6)
				}
			} else {
				lavaBlendPixel(grid, x, y, c, (1.34-d)/0.34*0.34)
			}
		}
	}
}

func (l *LavaLamp) paintBottleCapsLocked(grid [][]Pixel, cx, top, bottom, neck, body, heat float64) {
	h := len(grid)
	if h == 0 {
		return
	}
	glass := hslToRGB(l.cfg.Hue+14, 0.16, l.cfg.GlassLight*(0.55+0.45*heat))
	metal := hslToRGB(232, 0.11, 0.10+0.07*heat)
	hot := hslToRGB(35, 0.94, 0.38+0.16*heat)
	lavaFillRect(grid, int(cx-neck)-2, int(top)-4, int(neck*2)+4, 3, metal, 0.9)
	lavaFillRect(grid, int(cx-neck)-1, int(top)-1, int(neck*2)+2, 2, glass, 0.75)
	lavaFillRect(grid, int(cx-body)-4, int(bottom), int(body*2)+8, 3, metal, 0.95)
	lavaFillRect(grid, int(cx-body*0.56), int(bottom)+1, int(body*1.12), 2, hot, 0.45*heat)
	lavaFillRect(grid, int(cx-body*0.86), min(h-3, int(bottom)+4), int(body*1.72), 3, metal, 0.9)
}

func lavaFillRect(grid [][]Pixel, x0, y0, ww, hh int, c color.RGBA, alpha float64) {
	for y := max(0, y0); y < min(len(grid), y0+hh); y++ {
		for x := max(0, x0); x < min(len(grid[y]), x0+ww); x++ {
			lavaBlendPixel(grid, x, y, c, alpha)
		}
	}
}

func lavaBlendPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) || alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	dst := grid[y][x].C
	if !grid[y][x].Filled {
		dst = color.RGBA{}
	}
	inv := 1 - alpha
	grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(dst.R)*inv + float64(c.R)*alpha + 0.5),
		G: uint8(float64(dst.G)*inv + float64(c.G)*alpha + 0.5),
		B: uint8(float64(dst.B)*inv + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func lavaInsideBottle(w, h int, x, y float64) bool {
	_, top, bottom, _, _ := lavaBottleGeometry(w, h)
	if y < top || y > bottom {
		return false
	}
	cx, _, _, _, _ := lavaBottleGeometry(w, h)
	return math.Abs(x-cx) <= lavaHalfWidthAt(w, h, y)
}

func lavaBottleGeometry(w, h int) (cx, top, bottom, neck, body float64) {
	cx = float64(w) * 0.52
	top = math.Max(1, float64(h)*0.07)
	bottom = math.Max(top+8, float64(h)*0.88)
	neck = math.Max(3, float64(w)*0.055)
	body = math.Max(neck+4, math.Min(float64(w)*0.20, float64(h)*0.30))
	return
}

func lavaHalfWidthAt(w, h int, y float64) float64 {
	_, top, bottom, neck, body := lavaBottleGeometry(w, h)
	if y <= top || y >= bottom {
		return neck
	}
	t := clamp01((y - top) / math.Max(1, bottom-top))
	switch {
	case t < 0.16:
		return neck
	case t < 0.30:
		p := smoothstep((t - 0.16) / 0.14)
		return neck + (body-neck)*p
	case t > 0.90:
		p := smoothstep((t - 0.90) / 0.10)
		return body * (1 - 0.10*p)
	default:
		return body * (0.98 + 0.04*math.Sin((t-0.30)/0.60*math.Pi))
	}
}

func smoothstep(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}
