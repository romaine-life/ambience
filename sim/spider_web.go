package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	spiderWebTicksPerSecond = 60
	spiderWebSpokes         = 12
	spiderWebRings          = 7
)

const (
	spiderWebPaletteDawn = iota
	spiderWebPaletteMoon
	spiderWebPaletteAutumn
	spiderWebPaletteMisty
)

// SpiderWebConfig tunes the dewy spider-web effect. The named knobs match the
// verification/dev schema; lifecycle durations are exposed only to make intro
// and ending triggers observable and configurable.
type SpiderWebConfig struct {
	DropletShimmer float64 `json:"dropletShimmer"`
	GlintRate      float64 `json:"glintRate"`
	MoveChance     float64 `json:"moveChance"`
	WebSway        float64 `json:"webSway"`
	Palette        int     `json:"palette"`
	IntroDur       int     `json:"introDur"`
	EndingDur      int     `json:"endingDur"`
}

func (c SpiderWebConfig) withDefaults() SpiderWebConfig {
	if c.DropletShimmer <= 0 {
		c.DropletShimmer = 1.0
	}
	c.DropletShimmer = math.Max(0.15, math.Min(2.5, c.DropletShimmer))
	if c.GlintRate <= 0 {
		c.GlintRate = 0.75
	}
	c.GlintRate = math.Max(0.08, math.Min(1.8, c.GlintRate))
	if c.MoveChance <= 0 {
		c.MoveChance = 0.034
	}
	c.MoveChance = math.Max(0, math.Min(0.16, c.MoveChance))
	if c.WebSway <= 0 {
		c.WebSway = 0.55
	}
	c.WebSway = math.Max(0, math.Min(1.8, c.WebSway))
	if c.Palette < spiderWebPaletteDawn {
		c.Palette = spiderWebPaletteDawn
	}
	if c.Palette > spiderWebPaletteMisty {
		c.Palette = spiderWebPaletteMisty
	}
	if c.IntroDur <= 0 {
		c.IntroDur = 120
	}
	c.IntroDur = max(15, min(360, c.IntroDur))
	if c.EndingDur <= 0 {
		c.EndingDur = 240
	}
	c.EndingDur = max(30, min(720, c.EndingDur))
	return c
}

// NormalizeSpiderWebConfig returns cfg with defaults and clamps applied.
func NormalizeSpiderWebConfig(cfg SpiderWebConfig) SpiderWebConfig {
	return cfg.withDefaults()
}

func SpiderWebSchema() EffectSchema {
	return EffectSchema{
		Name:           "spider-web",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "glintRate", Label: "glint rate", Slot: SlotSpawn, Group: "dew", Type: KnobFloat, Min: 0.1, Max: 1.8, Step: 0.05, Default: 0.75, Trigger: "glint",
				Description: "Dew bead density and autonomous glints per second."},
			{Key: "dropletShimmer", Label: "droplet shimmer", Slot: SlotLever, Group: "dew", Type: KnobFloat, Min: 0.15, Max: 2.5, Step: 0.05, Default: 1.0,
				Description: "Continuous bead twinkle as light shifts across the web."},
			{Key: "moveChance", Label: "move chance", Slot: SlotEvent, Group: "spider", Type: KnobFloat, Min: 0, Max: 0.16, Step: 0.002, Default: 0.034, Trigger: "reposition",
				Description: "Per-second chance of a small spider reposition or catch wrap."},
			{Key: "webSway", Label: "web sway", Slot: SlotEventMod, Group: "breeze", Type: KnobFloat, Min: 0, Max: 1.8, Step: 0.05, Default: 0.55, Trigger: "web-sway",
				Description: "Strength of breeze gusts bending the silk and droplets."},
			{Key: "palette", Label: "palette", Slot: SlotLever, Group: "scene", Type: KnobInt, Min: 0, Max: 3, Step: 1, Default: 0,
				Description: "Lighting: 0 dawn dew, 1 moonlit silver, 2 autumn gold, 3 misty."},
			{Key: "introDur", Label: "intro dur", Slot: SlotSpawn, Group: "lifecycle", Type: KnobInt, Min: 30, Max: 360, Step: 10, Default: 120, Trigger: "intro",
				Description: "Ticks spent catching first light before the web settles."},
			{Key: "endingDur", Label: "ending dur", Slot: SlotEnd, Group: "lifecycle", Type: KnobInt, Min: 60, Max: 720, Step: 10, Default: 240, Trigger: "ending",
				Description: "Ticks for the light to fade into a held still web."},
		},
	}
}

type SpiderWebDroplet struct {
	Ring       int     `json:"ring"`
	Spoke      int     `json:"spoke"`
	Offset     float64 `json:"offset,omitempty"`
	Size       float64 `json:"size,omitempty"`
	Phase      float64 `json:"phase,omitempty"`
	GlintTicks int     `json:"glintTicks,omitempty"`
}

type SpiderWebSnapshot struct {
	Tick         int                `json:"tick"`
	Lifecycle    Lifecycle          `json:"lifecycle"`
	Droplets     []SpiderWebDroplet `json:"droplets"`
	SpiderAngle  float64            `json:"spiderAngle"`
	SpiderRadius float64            `json:"spiderRadius"`
	MoveFromA    float64            `json:"moveFromA,omitempty"`
	MoveFromR    float64            `json:"moveFromR,omitempty"`
	MoveToA      float64            `json:"moveToA,omitempty"`
	MoveToR      float64            `json:"moveToR,omitempty"`
	MoveTicks    int                `json:"moveTicks,omitempty"`
	MoveTotal    int                `json:"moveTotal,omitempty"`
	WrapTicks    int                `json:"wrapTicks,omitempty"`
	WrapTotal    int                `json:"wrapTotal,omitempty"`
	IntroTicks   int                `json:"introTicks,omitempty"`
	IntroTotal   int                `json:"introTotal,omitempty"`
	EndingTicks  int                `json:"endingTicks,omitempty"`
	EndingTotal  int                `json:"endingTotal,omitempty"`
	SwayTicks    int                `json:"swayTicks,omitempty"`
	SwayTotal    int                `json:"swayTotal,omitempty"`
	SwayAmp      float64            `json:"swayAmp,omitempty"`
	SwayDir      float64            `json:"swayDir,omitempty"`
	Ended        bool               `json:"ended,omitempty"`
	RNGState     uint64             `json:"rngState,omitempty"`
}

type SpiderWebPersistedState struct {
	Tick         int                `json:"tick"`
	Droplets     []SpiderWebDroplet `json:"droplets"`
	SpiderAngle  float64            `json:"spiderAngle"`
	SpiderRadius float64            `json:"spiderRadius"`
	MoveFromA    float64            `json:"moveFromA,omitempty"`
	MoveFromR    float64            `json:"moveFromR,omitempty"`
	MoveToA      float64            `json:"moveToA,omitempty"`
	MoveToR      float64            `json:"moveToR,omitempty"`
	MoveTicks    int                `json:"moveTicks,omitempty"`
	MoveTotal    int                `json:"moveTotal,omitempty"`
	WrapTicks    int                `json:"wrapTicks,omitempty"`
	WrapTotal    int                `json:"wrapTotal,omitempty"`
	IntroTicks   int                `json:"introTicks,omitempty"`
	IntroTotal   int                `json:"introTotal,omitempty"`
	EndingTicks  int                `json:"endingTicks,omitempty"`
	EndingTotal  int                `json:"endingTotal,omitempty"`
	SwayTicks    int                `json:"swayTicks,omitempty"`
	SwayTotal    int                `json:"swayTotal,omitempty"`
	SwayAmp      float64            `json:"swayAmp,omitempty"`
	SwayDir      float64            `json:"swayDir,omitempty"`
	Ended        bool               `json:"ended,omitempty"`
	RNGState     uint64             `json:"rngState"`
}

type SpiderWeb struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng *rngutil.RNG
	cfg SpiderWebConfig

	tick         int
	droplets     []SpiderWebDroplet
	spiderAngle  float64
	spiderRadius float64
	moveFromA    float64
	moveFromR    float64
	moveToA      float64
	moveToR      float64
	moveTicks    int
	moveTotal    int
	wrapTicks    int
	wrapTotal    int
	introTicks   int
	introTotal   int
	endingTicks  int
	endingTotal  int
	swayTicks    int
	swayTotal    int
	swayAmp      float64
	swayDir      float64
	ended        bool
	log          []LogEntry
}

func NewSpiderWeb(w, h int, seed int64, cfg SpiderWebConfig) *SpiderWeb {
	cfg = cfg.withDefaults()
	e := &SpiderWeb{
		rng:          rngutil.New(seed),
		cfg:          cfg,
		spiderAngle:  -math.Pi / 2,
		spiderRadius: 0.06,
	}
	e.Resize(w, h)
	e.mu.Lock()
	e.reconcileDropletsLocked()
	e.renderLocked()
	e.mu.Unlock()
	return e
}

func (e *SpiderWeb) Resize(w, h int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e.W = w
	e.H = h
	e.Grid = make([][]Pixel, h)
	for y := range e.Grid {
		e.Grid[y] = make([]Pixel, w)
	}
	e.renderLocked()
}

func (e *SpiderWeb) EffectiveConfig() SpiderWebConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *SpiderWeb) SetConfig(cfg SpiderWebConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg.withDefaults()
	e.reconcileDropletsLocked()
	e.renderLocked()
}

func (e *SpiderWeb) Snapshot() SpiderWebSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SpiderWebSnapshot{
		Tick:         e.tick,
		Lifecycle:    e.lifecycleLocked(),
		Droplets:     cloneSpiderWebDroplets(e.droplets),
		SpiderAngle:  e.spiderAngle,
		SpiderRadius: e.spiderRadius,
		MoveFromA:    e.moveFromA,
		MoveFromR:    e.moveFromR,
		MoveToA:      e.moveToA,
		MoveToR:      e.moveToR,
		MoveTicks:    e.moveTicks,
		MoveTotal:    e.moveTotal,
		WrapTicks:    e.wrapTicks,
		WrapTotal:    e.wrapTotal,
		IntroTicks:   e.introTicks,
		IntroTotal:   e.introTotal,
		EndingTicks:  e.endingTicks,
		EndingTotal:  e.endingTotal,
		SwayTicks:    e.swayTicks,
		SwayTotal:    e.swayTotal,
		SwayAmp:      e.swayAmp,
		SwayDir:      e.swayDir,
		Ended:        e.ended,
		RNGState:     e.rng.State(),
	}
}

func (e *SpiderWeb) RestoreSnapshot(snap SpiderWebSnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.restoreStateLocked(spiderWebStateFromSnapshot(snap))
	if snap.RNGState != 0 {
		e.rng.SetState(snap.RNGState)
	}
	e.renderLocked()
}

func (e *SpiderWeb) SnapshotPersistedState() SpiderWebPersistedState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SpiderWebPersistedState{
		Tick:         e.tick,
		Droplets:     cloneSpiderWebDroplets(e.droplets),
		SpiderAngle:  e.spiderAngle,
		SpiderRadius: e.spiderRadius,
		MoveFromA:    e.moveFromA,
		MoveFromR:    e.moveFromR,
		MoveToA:      e.moveToA,
		MoveToR:      e.moveToR,
		MoveTicks:    e.moveTicks,
		MoveTotal:    e.moveTotal,
		WrapTicks:    e.wrapTicks,
		WrapTotal:    e.wrapTotal,
		IntroTicks:   e.introTicks,
		IntroTotal:   e.introTotal,
		EndingTicks:  e.endingTicks,
		EndingTotal:  e.endingTotal,
		SwayTicks:    e.swayTicks,
		SwayTotal:    e.swayTotal,
		SwayAmp:      e.swayAmp,
		SwayDir:      e.swayDir,
		Ended:        e.ended,
		RNGState:     e.rng.State(),
	}
}

func (e *SpiderWeb) RestorePersistedState(state SpiderWebPersistedState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.restoreStateLocked(state)
	if state.RNGState != 0 {
		e.rng.SetState(state.RNGState)
	}
	e.renderLocked()
}

func (e *SpiderWeb) restoreStateLocked(state SpiderWebPersistedState) {
	e.tick = state.Tick
	e.droplets = cloneSpiderWebDroplets(state.Droplets)
	e.spiderAngle = state.SpiderAngle
	e.spiderRadius = state.SpiderRadius
	if e.spiderRadius == 0 {
		e.spiderRadius = 0.06
	}
	e.moveFromA = state.MoveFromA
	e.moveFromR = state.MoveFromR
	e.moveToA = state.MoveToA
	e.moveToR = state.MoveToR
	e.moveTicks = state.MoveTicks
	e.moveTotal = state.MoveTotal
	e.wrapTicks = state.WrapTicks
	e.wrapTotal = state.WrapTotal
	e.introTicks = state.IntroTicks
	e.introTotal = state.IntroTotal
	e.endingTicks = state.EndingTicks
	e.endingTotal = state.EndingTotal
	e.swayTicks = state.SwayTicks
	e.swayTotal = state.SwayTotal
	e.swayAmp = state.SwayAmp
	e.swayDir = state.SwayDir
	e.ended = state.Ended
	if e.introTicks > 0 && e.introTotal <= 0 {
		e.introTotal = e.cfg.IntroDur
	}
	if e.endingTicks > 0 && e.endingTotal <= 0 {
		e.endingTotal = e.cfg.EndingDur
	}
	if e.swayTicks > 0 && e.swayTotal <= 0 {
		e.swayTotal = e.swayTicks
	}
	if e.moveTicks > 0 && e.moveTotal <= 0 {
		e.moveTotal = e.moveTicks
	}
	e.reconcileDropletsLocked()
}

func spiderWebStateFromSnapshot(snap SpiderWebSnapshot) SpiderWebPersistedState {
	return SpiderWebPersistedState{
		Tick:         snap.Tick,
		Droplets:     snap.Droplets,
		SpiderAngle:  snap.SpiderAngle,
		SpiderRadius: snap.SpiderRadius,
		MoveFromA:    snap.MoveFromA,
		MoveFromR:    snap.MoveFromR,
		MoveToA:      snap.MoveToA,
		MoveToR:      snap.MoveToR,
		MoveTicks:    snap.MoveTicks,
		MoveTotal:    snap.MoveTotal,
		WrapTicks:    snap.WrapTicks,
		WrapTotal:    snap.WrapTotal,
		IntroTicks:   snap.IntroTicks,
		IntroTotal:   snap.IntroTotal,
		EndingTicks:  snap.EndingTicks,
		EndingTotal:  snap.EndingTotal,
		SwayTicks:    snap.SwayTicks,
		SwayTotal:    snap.SwayTotal,
		SwayAmp:      snap.SwayAmp,
		SwayDir:      snap.SwayDir,
		Ended:        snap.Ended,
		RNGState:     snap.RNGState,
	}
}

func (e *SpiderWeb) CurrentTick() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick
}

func (e *SpiderWeb) PerturbRNG(delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rng.Mix(delta)
}

func (e *SpiderWeb) DrainLog() []LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.log) == 0 {
		return nil
	}
	out := e.log
	e.log = nil
	return out
}

func (e *SpiderWeb) TriggerEvent(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch name {
	case "intro":
		e.startIntroLocked()
	case "ending":
		e.startEndingLocked()
	case "glint":
		if e.ended {
			return false
		}
		e.startGlintLocked("triggered")
	case "reposition":
		if e.ended {
			return false
		}
		e.startRepositionLocked("triggered")
	case "wrap-catch":
		if e.ended {
			return false
		}
		e.startWrapLocked("triggered")
	case "web-sway":
		if e.ended {
			return false
		}
		e.startSwayLocked("triggered")
	default:
		return false
	}
	e.renderLocked()
	return true
}

func (e *SpiderWeb) Step() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.tick++
	if e.ended {
		e.renderLocked()
		return
	}
	for i := range e.droplets {
		if e.droplets[i].GlintTicks > 0 {
			e.droplets[i].GlintTicks--
		}
	}
	if e.introTicks > 0 {
		e.introTicks--
		if e.introTicks == 0 {
			e.introTotal = 0
		}
	}
	if e.swayTicks > 0 {
		e.swayTicks--
		if e.swayTicks == 0 {
			e.swayTotal = 0
			e.swayAmp = 0
			e.swayDir = 0
		}
	}
	if e.moveTicks > 0 {
		e.moveTicks--
		if e.moveTicks == 0 {
			e.spiderAngle = e.moveToA
			e.spiderRadius = e.moveToR
			e.moveTotal = 0
		}
	}
	if e.wrapTicks > 0 {
		e.wrapTicks--
		if e.wrapTicks == 0 {
			e.wrapTotal = 0
		}
	}
	if e.endingTicks > 0 {
		e.endingTicks--
		if e.endingTicks == 0 {
			e.endingTotal = 0
			e.moveTicks = 0
			e.moveTotal = 0
			e.wrapTicks = 0
			e.wrapTotal = 0
			e.swayTicks = 0
			e.swayTotal = 0
			for i := range e.droplets {
				e.droplets[i].GlintTicks = 0
			}
			e.ended = true
			e.appendLogLocked("ending", "web stilled")
		}
	}

	if !e.ended && e.endingTicks == 0 {
		e.rollGlintLocked()
		e.rollMovementLocked()
		e.rollSwayLocked()
	}

	e.renderLocked()
}

func (e *SpiderWeb) GridCopy() [][]Pixel {
	e.mu.Lock()
	defer e.mu.Unlock()
	return copyPixelGrid(e.Grid)
}

func (e *SpiderWeb) Frame() [][]Pixel {
	return e.GridCopy()
}

func (e *SpiderWeb) lifecycleLocked() Lifecycle {
	switch {
	case e.introTicks > 0:
		return LifecycleIntro
	case e.endingTicks > 0:
		return LifecycleEnding
	case e.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (e *SpiderWeb) startIntroLocked() {
	e.ended = false
	e.endingTicks = 0
	e.endingTotal = 0
	e.introTicks = e.cfg.IntroDur
	e.introTotal = e.cfg.IntroDur
	e.spiderRadius = 0.06
	e.spiderAngle = -math.Pi / 2
	e.moveTicks = 0
	e.moveTotal = 0
	e.wrapTicks = 0
	e.wrapTotal = 0
	e.appendLogLocked("intro", fmt.Sprintf("dew catches first light (dur=%d)", e.introTicks))
}

func (e *SpiderWeb) startEndingLocked() {
	e.introTicks = 0
	e.introTotal = 0
	e.endingTicks = e.cfg.EndingDur
	e.endingTotal = e.cfg.EndingDur
	e.ended = false
	e.appendLogLocked("ending", fmt.Sprintf("light fading (dur=%d)", e.endingTicks))
}

func (e *SpiderWeb) startGlintLocked(verb string) {
	if len(e.droplets) == 0 {
		return
	}
	count := 1
	if e.cfg.GlintRate > 1.15 && e.rng.Float64() < 0.55 {
		count = 2
	}
	for i := 0; i < count; i++ {
		idx := e.rng.Intn(len(e.droplets))
		e.droplets[idx].GlintTicks = 22 + e.rng.Intn(30)
	}
	e.appendLogLocked("glint", fmt.Sprintf("%s (%d bead)", verb, count))
}

func (e *SpiderWeb) startRepositionLocked(verb string) {
	a, r := e.currentSpiderPolarLocked()
	e.moveFromA = a
	e.moveFromR = r
	if r > 0.24 && e.rng.Float64() < 0.55 {
		e.moveToR = 0.06 + e.rng.Float64()*0.05
		e.moveToA = -math.Pi/2 + (e.rng.Float64()-0.5)*0.55
	} else {
		spoke := e.rng.Intn(spiderWebSpokes)
		e.moveToA = spiderWebAngle(spoke) + (e.rng.Float64()-0.5)*0.18
		e.moveToR = 0.10 + e.rng.Float64()*0.23
	}
	e.moveTotal = 80 + e.rng.Intn(80)
	e.moveTicks = e.moveTotal
	e.appendLogLocked("reposition", fmt.Sprintf("%s (dur=%d)", verb, e.moveTotal))
}

func (e *SpiderWeb) startWrapLocked(verb string) {
	a, r := e.currentSpiderPolarLocked()
	e.moveFromA = a
	e.moveFromR = r
	spoke := e.rng.Intn(spiderWebSpokes)
	e.moveToA = spiderWebAngle(spoke) + (e.rng.Float64()-0.5)*0.16
	e.moveToR = 0.33 + e.rng.Float64()*0.30
	e.moveTotal = 70 + e.rng.Intn(65)
	e.moveTicks = e.moveTotal
	e.wrapTotal = 210 + e.rng.Intn(100)
	e.wrapTicks = e.wrapTotal
	e.appendLogLocked("wrap-catch", fmt.Sprintf("%s (wrap=%d)", verb, e.wrapTotal))
}

func (e *SpiderWeb) startSwayLocked(verb string) {
	e.swayTotal = 90 + e.rng.Intn(150)
	e.swayTicks = e.swayTotal
	dir := 1.0
	if e.rng.Float64() < 0.5 {
		dir = -1
	}
	e.swayDir = dir
	e.swayAmp = (0.7 + e.rng.Float64()*0.75) * e.cfg.WebSway
	e.appendLogLocked("web-sway", fmt.Sprintf("%s (amp=%.2f)", verb, e.swayAmp))
}

func (e *SpiderWeb) rollGlintLocked() {
	p := e.cfg.GlintRate / float64(spiderWebTicksPerSecond)
	if e.rng.Float64() < p {
		e.startGlintLocked("started")
	}
}

func (e *SpiderWeb) rollMovementLocked() {
	if e.moveTicks > 0 || e.wrapTicks > 0 {
		return
	}
	p := e.cfg.MoveChance / float64(spiderWebTicksPerSecond)
	if e.rng.Float64() >= p {
		return
	}
	if e.rng.Float64() < 0.25 {
		e.startWrapLocked("started")
	} else {
		e.startRepositionLocked("started")
	}
}

func (e *SpiderWeb) rollSwayLocked() {
	if e.swayTicks > 0 || e.cfg.WebSway <= 0 {
		return
	}
	p := 0.00005 * e.cfg.WebSway
	if e.rng.Float64() < p {
		e.startSwayLocked("started")
	}
}

func (e *SpiderWeb) reconcileDropletsLocked() {
	target := e.desiredDropletCountLocked()
	if target < 0 {
		target = 0
	}
	if target > spiderWebSpokes*spiderWebRings {
		target = spiderWebSpokes * spiderWebRings
	}
	if len(e.droplets) > target {
		e.droplets = e.droplets[:target]
		return
	}
	used := map[[2]int]bool{}
	for _, d := range e.droplets {
		if d.Ring >= 1 && d.Ring <= spiderWebRings && d.Spoke >= 0 && d.Spoke < spiderWebSpokes {
			used[[2]int{d.Ring, d.Spoke}] = true
		}
	}
	for len(e.droplets) < target {
		var ring, spoke int
		for attempt := 0; attempt < 80; attempt++ {
			ring = 1 + e.rng.Intn(spiderWebRings)
			spoke = e.rng.Intn(spiderWebSpokes)
			if !used[[2]int{ring, spoke}] {
				break
			}
		}
		if used[[2]int{ring, spoke}] {
			for r := 1; r <= spiderWebRings && used[[2]int{ring, spoke}]; r++ {
				for s := 0; s < spiderWebSpokes; s++ {
					if !used[[2]int{r, s}] {
						ring, spoke = r, s
						break
					}
				}
			}
			if used[[2]int{ring, spoke}] {
				return
			}
		}
		used[[2]int{ring, spoke}] = true
		e.droplets = append(e.droplets, SpiderWebDroplet{
			Ring:   ring,
			Spoke:  spoke,
			Offset: (e.rng.Float64() - 0.5) * 0.105,
			Size:   0.75 + e.rng.Float64()*0.8,
			Phase:  e.rng.Float64() * math.Pi * 2,
		})
	}
}

func (e *SpiderWeb) desiredDropletCountLocked() int {
	norm := (e.cfg.GlintRate - 0.1) / 1.7
	norm = clamp01(norm)
	density := 0.34 + norm*0.44
	return int(math.Round(float64(spiderWebSpokes*spiderWebRings) * density))
}

func (e *SpiderWeb) currentSpiderPolarLocked() (float64, float64) {
	if e.moveTicks <= 0 || e.moveTotal <= 0 {
		return e.spiderAngle, e.spiderRadius
	}
	progress := 1 - float64(e.moveTicks)/float64(e.moveTotal)
	progress = easeInOutSpiderWeb(progress)
	return lerpSpiderWebAngle(e.moveFromA, e.moveToA, progress), e.moveFromR + (e.moveToR-e.moveFromR)*progress
}

func (e *SpiderWeb) appendLogLocked(kind, desc string) {
	e.log = append(e.log, LogEntry{Tick: e.tick, Type: kind, Desc: desc})
	if len(e.log) > 200 {
		e.log = e.log[len(e.log)-200:]
	}
}

func (e *SpiderWeb) renderLocked() {
	if e.W <= 0 || e.H <= 0 || len(e.Grid) != e.H {
		return
	}
	pal := spiderWebPalette(e.cfg.Palette)
	level := e.visibilityLocked()
	for y := 0; y < e.H; y++ {
		t := 0.0
		if e.H > 1 {
			t = float64(y) / float64(e.H-1)
		}
		bg := mixColor(pal.Top, pal.Bottom, t)
		if e.ended {
			bg = mixColor(bg, color.RGBA{}, 0.36)
		}
		bg = scaleSpiderWebColor(bg, 0.58+0.42*level)
		for x := 0; x < e.W; x++ {
			e.Grid[y][x] = Pixel{Filled: true, C: bg}
		}
	}
	if e.W < 3 || e.H < 3 {
		return
	}

	cx, cy, rx, ry := e.webBoundsLocked()
	sway := e.swayOffsetLocked()
	lineAlpha := 0.34 * level
	if e.ended {
		lineAlpha *= 0.74
	}
	for s := 0; s < spiderWebSpokes; s++ {
		a := spiderWebAngle(s)
		x0, y0 := e.webPointLocked(0.05, a, cx, cy, rx, ry, sway)
		x1, y1 := e.webPointLocked(1.02, a, cx, cy, rx, ry, sway)
		drawSpiderWebLine(e.Grid, x0, y0, x1, y1, pal.Web, lineAlpha)
	}
	for ring := 1; ring <= spiderWebRings; ring++ {
		r := spiderWebRingNorm(ring)
		alpha := lineAlpha * (0.72 + 0.28*float64(ring)/spiderWebRings)
		var prevX, prevY, firstX, firstY float64
		for s := 0; s <= spiderWebSpokes; s++ {
			a := spiderWebAngle(s % spiderWebSpokes)
			x, y := e.webPointLocked(r, a, cx, cy, rx, ry, sway)
			if s == 0 {
				firstX, firstY = x, y
			} else {
				drawSpiderWebLine(e.Grid, prevX, prevY, x, y, pal.Web, alpha)
			}
			prevX, prevY = x, y
		}
		drawSpiderWebLine(e.Grid, prevX, prevY, firstX, firstY, pal.Web, alpha)
	}

	e.paintDropletsLocked(pal, cx, cy, rx, ry, sway, level)
	e.paintSpiderLocked(pal, cx, cy, rx, ry, sway, level)
}

func (e *SpiderWeb) visibilityLocked() float64 {
	if e.ended {
		return 0.34
	}
	level := 1.0
	if e.introTicks > 0 {
		total := max(1, e.introTotal)
		p := 1 - float64(e.introTicks)/float64(total)
		level = 0.12 + 0.88*easeInOutSpiderWeb(p)
	}
	if e.endingTicks > 0 {
		total := max(1, e.endingTotal)
		p := float64(e.endingTicks) / float64(total)
		level = 0.28 + 0.72*easeInOutSpiderWeb(p)
	}
	return clamp01(level)
}

func (e *SpiderWeb) webBoundsLocked() (float64, float64, float64, float64) {
	cx := float64(e.W-1) * 0.52
	cy := float64(e.H-1) * 0.47
	rx := math.Max(3, float64(e.W)*0.46)
	ry := math.Max(3, float64(e.H)*0.44)
	return cx, cy, rx, ry
}

func (e *SpiderWeb) swayOffsetLocked() float64 {
	if e.ended {
		return 0
	}
	base := math.Sin(float64(e.tick)*0.025) * e.cfg.WebSway * 0.28
	if e.swayTicks <= 0 || e.swayTotal <= 0 {
		return base
	}
	p := 1 - float64(e.swayTicks)/float64(e.swayTotal)
	envelope := math.Sin(math.Pi * p)
	gust := math.Sin(p*math.Pi*2.8) * envelope * e.swayAmp * e.swayDir
	return base + gust
}

func (e *SpiderWeb) webPointLocked(r, angle, cx, cy, rx, ry, sway float64) (float64, float64) {
	tick := e.tick
	if e.ended {
		tick = 0
	}
	warp := 1 + 0.025*math.Sin(angle*3+float64(tick)*0.011)
	x := cx + math.Cos(angle)*rx*r*warp + sway*r*r*2.4
	y := cy + math.Sin(angle)*ry*r*(1+0.018*math.Cos(angle*2)) + sway*r*0.35
	return x, y
}

func (e *SpiderWeb) paintDropletsLocked(pal spiderWebColors, cx, cy, rx, ry, sway, level float64) {
	for _, d := range e.droplets {
		if d.Ring < 1 || d.Ring > spiderWebRings || d.Spoke < 0 || d.Spoke >= spiderWebSpokes {
			continue
		}
		r := spiderWebRingNorm(d.Ring)
		a := spiderWebAngle(d.Spoke) + d.Offset
		x, y := e.webPointLocked(r, a, cx, cy, rx, ry, sway)
		shimmer := 0.5 + 0.5*math.Sin(float64(e.tick)*0.065*e.cfg.DropletShimmer+d.Phase)
		if e.ended {
			shimmer = 0.18 + 0.12*math.Sin(d.Phase)
		}
		alpha := (0.32 + 0.38*shimmer) * level
		glint := 0.0
		if d.GlintTicks > 0 {
			total := 52.0
			p := 1 - float64(d.GlintTicks)/total
			p = math.Max(0, math.Min(1, p))
			glint = math.Sin(math.Pi*p) * (0.70 + 0.25*d.Size) * level
		}
		main := mixColor(pal.Dew, pal.Glint, glint)
		ix, iy := int(math.Round(x)), int(math.Round(y))
		blendSpiderWebPixel(e.Grid, ix, iy, main, math.Min(1, alpha+glint))
		if d.Size > 1.18 || glint > 0.28 {
			halo := scaleSpiderWebColor(pal.Glint, 0.82)
			blendSpiderWebPixel(e.Grid, ix+1, iy, halo, 0.18*level+glint*0.35)
			blendSpiderWebPixel(e.Grid, ix, iy-1, halo, 0.12*level+glint*0.22)
		}
	}
}

func (e *SpiderWeb) paintSpiderLocked(pal spiderWebColors, cx, cy, rx, ry, sway, level float64) {
	a, r := e.currentSpiderPolarLocked()
	x, y := e.webPointLocked(r, a, cx, cy, rx, ry, sway)
	ix, iy := int(math.Round(x)), int(math.Round(y))
	body := pal.Spider
	if level < 0.75 {
		body = mixColor(body, pal.Web, 0.22*(1-level))
	}
	paintPixel(e.Grid, ix, iy, body)
	paintPixel(e.Grid, ix, iy+1, body)
	paintPixel(e.Grid, ix, iy-1, mixColor(body, pal.Web, 0.20))
	for _, leg := range [][2]int{{-1, -1}, {-2, -1}, {1, -1}, {2, -1}, {-1, 1}, {-2, 1}, {1, 1}, {2, 1}} {
		blendSpiderWebPixel(e.Grid, ix+leg[0], iy+leg[1], body, 0.78*level)
	}
	if e.wrapTicks > 0 && e.wrapTotal > 0 {
		p := 1 - float64(e.wrapTicks)/float64(e.wrapTotal)
		pulse := 0.45 + 0.55*math.Sin(math.Pi*4*p)*math.Sin(math.Pi*p)
		catch := mixColor(pal.Dew, pal.Glint, 0.45+0.35*pulse)
		blendSpiderWebPixel(e.Grid, ix+1, iy, catch, 0.72*level)
		blendSpiderWebPixel(e.Grid, ix+1, iy+1, catch, 0.56*level)
		blendSpiderWebPixel(e.Grid, ix+2, iy, catch, 0.38*level)
		drawSpiderWebLine(e.Grid, x, y, x+2.2, y+0.5, pal.Web, 0.52*level)
	}
}

type spiderWebColors struct {
	Top    color.RGBA
	Bottom color.RGBA
	Web    color.RGBA
	Dew    color.RGBA
	Glint  color.RGBA
	Spider color.RGBA
}

func spiderWebPalette(palette int) spiderWebColors {
	switch palette {
	case spiderWebPaletteMoon:
		return spiderWebColors{
			Top:    hslToRGB(228, 0.26, 0.065),
			Bottom: hslToRGB(206, 0.20, 0.14),
			Web:    hslToRGB(208, 0.16, 0.68),
			Dew:    hslToRGB(210, 0.20, 0.82),
			Glint:  hslToRGB(215, 0.08, 0.96),
			Spider: hslToRGB(226, 0.12, 0.05),
		}
	case spiderWebPaletteAutumn:
		return spiderWebColors{
			Top:    hslToRGB(236, 0.20, 0.08),
			Bottom: hslToRGB(34, 0.44, 0.23),
			Web:    hslToRGB(42, 0.18, 0.62),
			Dew:    hslToRGB(50, 0.52, 0.72),
			Glint:  hslToRGB(43, 0.85, 0.88),
			Spider: hslToRGB(24, 0.26, 0.045),
		}
	case spiderWebPaletteMisty:
		return spiderWebColors{
			Top:    hslToRGB(192, 0.15, 0.13),
			Bottom: hslToRGB(142, 0.12, 0.20),
			Web:    hslToRGB(178, 0.12, 0.64),
			Dew:    hslToRGB(174, 0.24, 0.74),
			Glint:  hslToRGB(168, 0.18, 0.92),
			Spider: hslToRGB(196, 0.10, 0.055),
		}
	default:
		return spiderWebColors{
			Top:    hslToRGB(210, 0.32, 0.095),
			Bottom: hslToRGB(42, 0.40, 0.22),
			Web:    hslToRGB(205, 0.18, 0.62),
			Dew:    hslToRGB(188, 0.42, 0.72),
			Glint:  hslToRGB(48, 0.78, 0.86),
			Spider: hslToRGB(218, 0.12, 0.045),
		}
	}
}

func spiderWebRingNorm(ring int) float64 {
	return 0.17 + float64(ring-1)*0.83/float64(max(1, spiderWebRings-1))
}

func spiderWebAngle(spoke int) float64 {
	return -math.Pi/2 + float64(spoke)*2*math.Pi/float64(spiderWebSpokes)
}

func drawSpiderWebLine(grid [][]Pixel, x0, y0, x1, y1 float64, c color.RGBA, alpha float64) {
	steps := int(math.Ceil(math.Max(math.Abs(x1-x0), math.Abs(y1-y0)) * 1.35))
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(x0 + (x1-x0)*t))
		y := int(math.Round(y0 + (y1-y0)*t))
		blendSpiderWebPixel(grid, x, y, c, alpha)
	}
}

func blendSpiderWebPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	if alpha >= 1 || !grid[y][x].Filled {
		grid[y][x] = Pixel{Filled: true, C: c}
		return
	}
	base := grid[y][x].C
	grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(base.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(base.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(base.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func scaleSpiderWebColor(c color.RGBA, f float64) color.RGBA {
	f = math.Max(0, f)
	return color.RGBA{
		R: uint8(math.Min(255, float64(c.R)*f)),
		G: uint8(math.Min(255, float64(c.G)*f)),
		B: uint8(math.Min(255, float64(c.B)*f)),
		A: 255,
	}
}

func easeInOutSpiderWeb(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}

func lerpSpiderWebAngle(a, b, t float64) float64 {
	d := math.Mod(b-a+math.Pi*3, math.Pi*2) - math.Pi
	return a + d*t
}

func cloneSpiderWebDroplets(src []SpiderWebDroplet) []SpiderWebDroplet {
	if len(src) == 0 {
		return nil
	}
	out := make([]SpiderWebDroplet, len(src))
	copy(out, src)
	return out
}

func countSpiderWebFilled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled && p.C != (color.RGBA{}) {
				count++
			}
		}
	}
	return count
}
