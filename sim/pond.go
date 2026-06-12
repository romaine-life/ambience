package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// Pond is a still pond with a small flock of ducks that circle their own
// chosen centers on the water. The server picks dive/quack/gust/calm events
// and integrates duck angles; clients run the same runtime locally so the
// ducks stay roughly in sync even though pixel-level wake noise drifts.

type pondDuck struct {
	CX, CY     float64 // circle center in normalized [0,1] frame coords
	Radius     float64 // circle radius in pixels
	Angle      float64 // current angle in radians
	AngularVel float64 // radians per tick
	Hue        float64 // base body hue
	DiveLeft   int     // ticks of dive remaining (0 = on surface)
	DiveTotal  int     // total dive duration when started
}

type pondRipple struct {
	X, Y    float64
	Radius  float64
	Life    int
	MaxLife int
}

// PondConfig tunes the Pond prototype used in isolated dev sessions.
type PondConfig struct {
	// INTRODUCTION
	IntroDur   int     `json:"intro_dur"`
	IntroDrift float64 `json:"intro_drift"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingFade   float64 `json:"ending_fade"`
	// LEVERS — pond
	Waterline  float64 `json:"waterline"`
	WaveAmp    float64 `json:"wave_amp"`
	WaveFreq   float64 `json:"wave_freq"`
	RippleLife int     `json:"ripple_life"`
	// LEVERS — ducks
	DuckCount int     `json:"duck_count"`
	CircleMin float64 `json:"circle_min"`
	CircleMax float64 `json:"circle_max"`
	SwimSpeed float64 `json:"swim_speed"`
	WakeRate  int     `json:"wake_rate"`
	// LEVERS — color
	WaterHue   float64 `json:"water_hue"`
	DuckHue    float64 `json:"duck_hue"`
	HueSpread  float64 `json:"hue_sp"`
	Saturation float64 `json:"sat"`
	LightMin   float64 `json:"lmin"`
	LightMax   float64 `json:"lmax"`
	// EVENT CHANCES
	DiveChance  float64 `json:"dive_p"`
	QuackChance float64 `json:"quack_p"`
	GustChance  float64 `json:"gust_p"`
	CalmChance  float64 `json:"calm_p"`
	// EVENT MODIFIERS
	DiveDur    int     `json:"dive_dur"`
	QuackBurst int     `json:"quack_burst"`
	GustDur    int     `json:"gust_dur"`
	GustMult   float64 `json:"gust_mult"`
	CalmDur    int     `json:"calm_dur"`
	CalmMult   float64 `json:"calm_mult"`
}

func (c PondConfig) withDefaults() PondConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 60
	}
	if c.IntroDrift < 0 {
		c.IntroDrift = 0
	}
	c.IntroDrift = clamp01(c.IntroDrift)
	if c.IntroDrift == 0 {
		c.IntroDrift = 0.20
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 70
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	if c.EndingFade <= 0 {
		c.EndingFade = 0.12
	}
	c.EndingFade = clamp01(c.EndingFade)
	if c.Waterline <= 0 {
		c.Waterline = 0.42
	}
	c.Waterline = clamp01(c.Waterline)
	if c.WaveAmp < 0 {
		c.WaveAmp = 0
	}
	if c.WaveAmp == 0 {
		c.WaveAmp = 0.9
	}
	if c.WaveFreq <= 0 {
		c.WaveFreq = 0.18
	}
	if c.RippleLife <= 0 {
		c.RippleLife = 28
	}
	if c.DuckCount <= 0 {
		c.DuckCount = 3
	}
	if c.DuckCount > 12 {
		c.DuckCount = 12
	}
	if c.CircleMin <= 0 {
		c.CircleMin = 6
	}
	if c.CircleMax <= 0 {
		c.CircleMax = 14
	}
	if c.CircleMax < c.CircleMin {
		c.CircleMin, c.CircleMax = c.CircleMax, c.CircleMin
	}
	if c.SwimSpeed <= 0 {
		c.SwimSpeed = 0.06
	}
	if c.WakeRate <= 0 {
		c.WakeRate = 4
	}
	if c.WaterHue == 0 {
		c.WaterHue = 198
	}
	if c.DuckHue == 0 {
		c.DuckHue = 44
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.42
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.14
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.78
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.DiveChance < 0 {
		c.DiveChance = 0
	}
	if c.QuackChance < 0 {
		c.QuackChance = 0
	}
	if c.GustChance < 0 {
		c.GustChance = 0
	}
	if c.CalmChance < 0 {
		c.CalmChance = 0
	}
	if c.DiveDur <= 0 {
		c.DiveDur = 24
	}
	if c.QuackBurst <= 0 {
		c.QuackBurst = 2
	}
	if c.GustDur <= 0 {
		c.GustDur = 40
	}
	if c.GustMult <= 0 {
		c.GustMult = 1.8
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 70
	}
	if c.CalmMult <= 0 || c.CalmMult > 1 {
		c.CalmMult = 0.45
	}
	return c
}

// PondSchema describes the Pond effect's tunable knobs for the dev UI.
func PondSchema() EffectSchema {
	return EffectSchema{
		Name: "pond",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent fading the ducks and water highlights in at scene start."},
			{Key: "intro_drift", Label: "intro drift", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.20,
				Description: "Starting fraction of the final swim speed and wake density during the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent slowing the ducks and dimming the pond before the scene clears."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks after the visible motion has mostly faded."},
			{Key: "ending_fade", Label: "ending fade", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Residual motion fraction remaining at the very end of the outro."},
			{Key: "waterline", Label: "waterline", Slot: SlotLever, Group: "pond", Type: KnobFloat, Min: 0.2, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Vertical position of the pond surface as a fraction of the frame height."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "pond", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.1, Default: 0.9,
				Description: "Amplitude of the gentle surface undulation."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "pond", Type: KnobFloat, Min: 0.05, Max: 0.4, Step: 0.01, Default: 0.18,
				Description: "Horizontal frequency of the surface wiggle."},
			{Key: "ripple_life", Label: "ripple life", Slot: SlotLever, Group: "pond", Type: KnobInt, Min: 8, Max: 80, Step: 2, Default: 28,
				Description: "How many ticks a duck wake or quack ripple takes to fade out."},
			{Key: "duck_count", Label: "ducks", Slot: SlotLever, Group: "ducks", Type: KnobInt, Min: 1, Max: 12, Step: 1, Default: 3,
				Description: "Number of ducks circling on the pond."},
			{Key: "circle_min", Label: "circle min", Slot: SlotLever, Group: "ducks", Type: KnobFloat, Min: 2, Max: 18, Step: 0.5, Default: 6,
				Description: "Smallest circle radius any duck may have, in pixels."},
			{Key: "circle_max", Label: "circle max", Slot: SlotLever, Group: "ducks", Type: KnobFloat, Min: 4, Max: 30, Step: 0.5, Default: 14,
				Description: "Largest circle radius any duck may have, in pixels."},
			{Key: "swim_speed", Label: "swim speed", Slot: SlotLever, Group: "ducks", Type: KnobFloat, Min: 0.01, Max: 0.2, Step: 0.005, Default: 0.06,
				Description: "Base angular speed each duck advances around its circle each tick."},
			{Key: "wake_rate", Label: "wake 1/", Slot: SlotLever, Group: "ducks", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 4,
				Description: "One-in-N per-tick chance per duck of leaving a fresh wake ripple behind it."},
			{Key: "water_hue", Label: "water hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 240, Step: 1, Default: 198,
				Description: "Base hue of the pond water. Lower values lean teal; higher values lean deeper blue."},
			{Key: "duck_hue", Label: "duck hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 80, Step: 1, Default: 44,
				Description: "Base hue of the ducks. Lower values lean brown; higher values lean yellow."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Variation between water, sky reflection, and duck plumage tones."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.85, Step: 0.02, Default: 0.42,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.4, Step: 0.01, Default: 0.14,
				Description: "Minimum lightness used for the deepest water and duck shadow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.45, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for sky reflection and duck highlight."},
			{Key: "dive_p", Label: "dive", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "dive",
				Description: "Per-tick chance of a duck ducking under for a few seconds and resurfacing."},
			{Key: "quack_p", Label: "quack", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quack",
				Description: "Per-tick chance of a quack — a small burst of ripples at one duck's position."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a wind gust that briefly speeds the ducks up."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a calm interval that briefly slows the ducks down."},
			{Key: "dive_dur", Label: "dive dur", Slot: SlotEventMod, Group: "dive", Type: KnobInt, Min: 6, Max: 90, Step: 2, Default: 24,
				Description: "Duration of a duck's dive in ticks before it pops back up."},
			{Key: "quack_burst", Label: "quack burst", Slot: SlotEventMod, Group: "quack", Type: KnobInt, Min: 1, Max: 6, Step: 1, Default: 2,
				Description: "Number of extra ripples emitted on each quack."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 40,
				Description: "Duration of a wind gust window in ticks."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Swim-speed multiplier applied while a gust is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70,
				Description: "Duration of a calm window in ticks."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Swim-speed multiplier applied while a calm is active."},
		},
	}
}

// PondDuck is the serializable shape of one circling duck.
type PondDuck struct {
	CX         float64 `json:"cx"`
	CY         float64 `json:"cy"`
	Radius     float64 `json:"r"`
	Angle      float64 `json:"a"`
	AngularVel float64 `json:"av"`
	Hue        float64 `json:"hue"`
	DiveLeft   int     `json:"diveLeft,omitempty"`
	DiveTotal  int     `json:"diveTotal,omitempty"`
}

// PondRipple is the serializable shape of one expanding surface ripple.
type PondRipple struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Radius  float64 `json:"radius"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
}

// PondState is the wire/persisted scalar shape of the pond runtime.
type PondState struct {
	Tick        int       `json:"tick"`
	IntroTicks  int       `json:"introTicks"`
	IntroTotal  int       `json:"introTotal"`
	EndingTicks int       `json:"endingTicks"`
	EndingTotal int       `json:"endingTotal"`
	Lifecycle   Lifecycle `json:"lifecycle"`
	EndingFade  int       `json:"endingFade"`
	GustTicks   int       `json:"gustTicks"`
	GustGain    float64   `json:"gustGain"`
	CalmTicks   int       `json:"calmTicks"`
}

type PondSnapshot struct {
	PondState
	RNGState uint64       `json:"rngState,omitempty"`
	Ducks    []PondDuck   `json:"ducks"`
	Ripples  []PondRipple `json:"ripples"`
}

type PondPersistedState struct {
	PondState
	RNGState uint64       `json:"rngState"`
	Ducks    []PondDuck   `json:"ducks"`
	Ripples  []PondRipple `json:"ripples"`
}

// Pond is the authoritative server-side pond simulation.
type Pond struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel
	rng  *rngutil.RNG
	cfg  PondConfig
	tick int

	ducks   []pondDuck
	ripples []pondRipple

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int
	gustTicks   int
	gustGain    float64
	calmTicks   int

	log []LogEntry
}

func NewPond(w, h int, seed int64, cfg PondConfig) *Pond {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	p := &Pond{
		W:        w,
		H:        h,
		Grid:     grid,
		rng:      rngutil.New(seed),
		cfg:      cfg.withDefaults(),
		gustGain: 1,
	}
	p.spawnDucksLocked()
	return p
}

func (p *Pond) Resize(w, h int) {
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
}

func (p *Pond) SetConfig(cfg PondConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := cfg.withDefaults()
	respawn := next.DuckCount != p.cfg.DuckCount ||
		next.CircleMin != p.cfg.CircleMin ||
		next.CircleMax != p.cfg.CircleMax ||
		next.SwimSpeed != p.cfg.SwimSpeed
	p.cfg = next
	if respawn {
		p.spawnDucksLocked()
	}
}

func (p *Pond) EffectiveConfig() PondConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *Pond) CurrentTick() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tick
}

func (p *Pond) PerturbRNG(delta int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rng.Mix(delta)
}

func (p *Pond) DrainLog() []LogEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.log) == 0 {
		return nil
	}
	out := p.log
	p.log = nil
	return out
}

func (p *Pond) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *Pond) Snapshot() PondSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PondSnapshot{
		PondState: p.snapshotStateLocked(),
		RNGState:  p.rng.State(),
		Ducks:     p.copyDucksLocked(),
		Ripples:   p.copyRipplesLocked(),
	}
}

func (p *Pond) RestoreSnapshot(s PondSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.PondState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreDucksLocked(s.Ducks)
	p.restoreRipplesLocked(s.Ripples)
	p.paintFrameLocked()
}

func (p *Pond) SnapshotPersistedState() PondPersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PondPersistedState{
		PondState: p.snapshotStateLocked(),
		RNGState:  p.rng.State(),
		Ducks:     p.copyDucksLocked(),
		Ripples:   p.copyRipplesLocked(),
	}
}

func (p *Pond) RestorePersistedState(s PondPersistedState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.PondState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
	p.restoreDucksLocked(s.Ducks)
	p.restoreRipplesLocked(s.Ripples)
}

func (p *Pond) snapshotStateLocked() PondState {
	return PondState{
		Tick:        p.tick,
		IntroTicks:  p.introTicks,
		IntroTotal:  p.introTotal,
		EndingTicks: p.endingTicks,
		EndingTotal: p.endingTotal,
		Lifecycle:   p.lifecycleLocked(),
		EndingFade:  p.endingFade,
		GustTicks:   p.gustTicks,
		GustGain:    p.gustGain,
		CalmTicks:   p.calmTicks,
	}
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// the pond's internal counters. The outro is non-terminal: once endingTicks
// expires, automatic events resume, so lifecycle returns to running (the
// schema declares ending_terminal: false by omission).
func (p *Pond) lifecycleLocked() Lifecycle {
	switch {
	case p.introTicks > 0:
		return LifecycleIntro
	case p.endingTicks > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

func (p *Pond) restoreStateLocked(s PondState) {
	p.tick = s.Tick
	p.introTicks = s.IntroTicks
	p.introTotal = s.IntroTotal
	p.endingTicks = s.EndingTicks
	p.endingTotal = s.EndingTotal
	p.endingFade = s.EndingFade
	p.gustTicks = s.GustTicks
	p.gustGain = s.GustGain
	if p.gustGain == 0 {
		p.gustGain = 1
	}
	p.calmTicks = s.CalmTicks
}

func (p *Pond) copyDucksLocked() []PondDuck {
	out := make([]PondDuck, len(p.ducks))
	for i, d := range p.ducks {
		out[i] = PondDuck{
			CX:         d.CX,
			CY:         d.CY,
			Radius:     d.Radius,
			Angle:      d.Angle,
			AngularVel: d.AngularVel,
			Hue:        d.Hue,
			DiveLeft:   d.DiveLeft,
			DiveTotal:  d.DiveTotal,
		}
	}
	return out
}

func (p *Pond) restoreDucksLocked(list []PondDuck) {
	if len(list) == 0 {
		p.spawnDucksLocked()
		return
	}
	p.ducks = make([]pondDuck, len(list))
	for i, d := range list {
		p.ducks[i] = pondDuck{
			CX:         d.CX,
			CY:         d.CY,
			Radius:     d.Radius,
			Angle:      d.Angle,
			AngularVel: d.AngularVel,
			Hue:        d.Hue,
			DiveLeft:   d.DiveLeft,
			DiveTotal:  d.DiveTotal,
		}
	}
}

func (p *Pond) copyRipplesLocked() []PondRipple {
	out := make([]PondRipple, len(p.ripples))
	for i, r := range p.ripples {
		out[i] = PondRipple{
			X:       r.X,
			Y:       r.Y,
			Radius:  r.Radius,
			Life:    r.Life,
			MaxLife: r.MaxLife,
		}
	}
	return out
}

func (p *Pond) restoreRipplesLocked(list []PondRipple) {
	p.ripples = make([]pondRipple, len(list))
	for i, r := range list {
		p.ripples[i] = pondRipple{
			X:       r.X,
			Y:       r.Y,
			Radius:  r.Radius,
			Life:    r.Life,
			MaxLife: r.MaxLife,
		}
	}
}

func (p *Pond) spawnDucksLocked() {
	n := p.cfg.DuckCount
	if n <= 0 {
		n = 1
	}
	p.ducks = make([]pondDuck, n)
	for i := range p.ducks {
		p.ducks[i] = p.makeDuckLocked(i, n)
	}
}

func (p *Pond) makeDuckLocked(i, n int) pondDuck {
	// Spread duck centers across the pond horizontally with some jitter so
	// they don't line up perfectly.
	xFrac := (float64(i)+0.5)/float64(n) + (p.rng.Float64()-0.5)*0.08
	if xFrac < 0.05 {
		xFrac = 0.05
	}
	if xFrac > 0.95 {
		xFrac = 0.95
	}
	// Center vertically lives in the lower half (the pond), biased toward
	// just below the waterline so the circles read as on-water.
	yFrac := p.cfg.Waterline + 0.10 + p.rng.Float64()*0.30
	if yFrac > 0.92 {
		yFrac = 0.92
	}
	r := p.cfg.CircleMin + p.rng.Float64()*math.Max(0, p.cfg.CircleMax-p.cfg.CircleMin)
	dir := 1.0
	if p.rng.Float64() < 0.5 {
		dir = -1
	}
	vel := dir * p.cfg.SwimSpeed * (0.75 + p.rng.Float64()*0.5)
	hue := math.Mod(p.cfg.DuckHue+(p.rng.Float64()-0.5)*p.cfg.HueSpread+360, 360)
	return pondDuck{
		CX:         xFrac,
		CY:         yFrac,
		Radius:     r,
		Angle:      p.rng.Float64() * 2 * math.Pi,
		AngularVel: vel,
		Hue:        hue,
	}
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (p *Pond) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "dive":
		p.diveLocked("triggered")
	case "quack":
		p.quackLocked("triggered")
	case "gust":
		p.startGustLocked("triggered")
	case "calm":
		p.startCalmLocked("triggered")
	case "intro":
		p.startIntroLocked()
		p.appendLog("intro", fmt.Sprintf("started (dur=%d, drift=%.2f)", p.introTotal, p.cfg.IntroDrift))
	case "ending":
		p.startEndingLocked()
		p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.endingFade, p.endingTotal-p.endingFade))
	default:
		return false
	}
	return true
}

func (p *Pond) diveLocked(verb string) {
	if len(p.ducks) == 0 {
		return
	}
	surface := p.ducks[:0:0]
	for i, d := range p.ducks {
		if d.DiveLeft == 0 {
			surface = append(surface, p.ducks[i])
		}
	}
	if len(surface) == 0 {
		return
	}
	// Pick a random surface duck (by scanning until we hit one).
	target := p.rng.Intn(len(surface))
	count := 0
	for i := range p.ducks {
		if p.ducks[i].DiveLeft != 0 {
			continue
		}
		if count == target {
			dur := jitterInt(p.rng, p.cfg.DiveDur, 0.25)
			p.ducks[i].DiveLeft = dur
			p.ducks[i].DiveTotal = dur
			x, y := p.duckPos(p.ducks[i])
			p.emitRippleLocked(x, y, dur+8)
			p.appendLog("dive", fmt.Sprintf("%s duck %d (dur=%d)", verb, i+1, dur))
			return
		}
		count++
	}
}

func (p *Pond) quackLocked(verb string) {
	if len(p.ducks) == 0 {
		return
	}
	idx := p.rng.Intn(len(p.ducks))
	x, y := p.duckPos(p.ducks[idx])
	burst := p.cfg.QuackBurst
	if burst <= 0 {
		burst = 1
	}
	life := p.cfg.RippleLife
	if life <= 0 {
		life = 24
	}
	for i := 0; i < burst; i++ {
		offsetX := (p.rng.Float64() - 0.5) * 2
		offsetY := (p.rng.Float64() - 0.5) * 1
		p.emitRippleLocked(x+offsetX, y+offsetY, life+i*4)
	}
	p.appendLog("quack", fmt.Sprintf("%s duck %d (burst=%d)", verb, idx+1, burst))
}

func (p *Pond) startGustLocked(verb string) {
	dur := jitterInt(p.rng, p.cfg.GustDur, 0.3)
	p.gustTicks = dur
	p.gustGain = math.Max(1, p.cfg.GustMult*(0.85+p.rng.Float64()*0.3))
	p.appendLog("gust", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, p.gustGain))
}

func (p *Pond) startCalmLocked(verb string) {
	dur := jitterInt(p.rng, p.cfg.CalmDur, 0.3)
	p.calmTicks = dur
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, p.cfg.CalmMult))
}

func (p *Pond) startIntroLocked() {
	p.introTotal = p.cfg.IntroDur
	if p.introTotal <= 0 {
		p.introTotal = 60
	}
	p.introTicks = p.introTotal
	p.endingTicks = 0
	p.endingTotal = 0
	p.endingFade = 0
	p.gustTicks = 0
	p.gustGain = 1
	p.calmTicks = 0
	for i := range p.ducks {
		p.ducks[i].DiveLeft = 0
		p.ducks[i].DiveTotal = 0
	}
}

func (p *Pond) startEndingLocked() {
	p.introTicks = 0
	p.introTotal = 0
	p.gustTicks = 0
	p.gustGain = 1
	p.calmTicks = 0
	p.endingFade = p.cfg.EndingDur
	if p.endingFade <= 0 {
		p.endingFade = 70
	}
	linger := p.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	p.endingTotal = p.endingFade + linger
	if p.endingTotal < 1 {
		p.endingTotal = p.endingFade
	}
	p.endingTicks = p.endingTotal
}

func (p *Pond) emitRippleLocked(x, y float64, life int) {
	if life <= 0 {
		life = 1
	}
	p.ripples = append(p.ripples, pondRipple{
		X:       x,
		Y:       y,
		Radius:  0,
		Life:    life,
		MaxLife: life,
	})
	// Cap total ripples so a runaway quack/dive event can't grow the
	// snapshot indefinitely.
	if len(p.ripples) > 240 {
		p.ripples = p.ripples[len(p.ripples)-240:]
	}
}

func (p *Pond) duckPos(d pondDuck) (float64, float64) {
	cx := d.CX * float64(p.W-1)
	cy := d.CY * float64(p.H-1)
	x := cx + math.Cos(d.Angle)*d.Radius
	// Vertical stretch so circles look like ellipses on the water surface.
	y := cy + math.Sin(d.Angle)*d.Radius*0.45
	return x, y
}

func (p *Pond) motionLevelLocked() float64 {
	level := 1.0
	if p.gustTicks > 0 {
		level *= p.gustGain
	}
	if p.calmTicks > 0 {
		level *= p.cfg.CalmMult
	}
	if p.introTicks > 0 && p.introTotal > 0 {
		level *= p.cfg.IntroDrift + (1-p.cfg.IntroDrift)*phaseProgress(p.introTotal, p.introTicks)
	}
	if p.endingTicks > 0 && p.endingTotal > 0 {
		level *= 1 - (1-p.cfg.EndingFade)*phaseProgress(p.endingTotal, p.endingTicks)
	}
	return math.Max(0.04, level)
}

func (p *Pond) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	if p.introTicks > 0 {
		p.introTicks--
	}
	if p.endingTicks > 0 {
		p.endingTicks--
	}
	if p.gustTicks > 0 {
		p.gustTicks--
		if p.gustTicks == 0 {
			p.gustGain = 1
		}
	}
	if p.calmTicks > 0 {
		p.calmTicks--
	}

	motion := p.motionLevelLocked()
	for i := range p.ducks {
		d := &p.ducks[i]
		if d.DiveLeft > 0 {
			d.DiveLeft--
			if d.DiveLeft == 0 {
				x, y := p.duckPos(*d)
				p.emitRippleLocked(x, y, p.cfg.RippleLife)
				p.appendLog("dive", fmt.Sprintf("duck %d resurfaced", i+1))
			}
			continue
		}
		d.Angle += d.AngularVel * motion
		// Periodically drop a wake ripple behind the duck.
		if p.cfg.WakeRate > 0 && p.rng.Intn(p.cfg.WakeRate) == 0 {
			x, y := p.duckPos(*d)
			// trail point a little behind the swim direction
			wakeAngle := d.Angle - math.Copysign(math.Pi/2, d.AngularVel)
			tx := x - math.Cos(wakeAngle)*0.6
			ty := y - math.Sin(wakeAngle)*0.6*0.45
			life := int(math.Round(float64(p.cfg.RippleLife) * (0.35 + 0.4*motion)))
			if life < 4 {
				life = 4
			}
			p.emitRippleLocked(tx, ty, life)
		}
	}

	// Roll for random events only when no lifecycle is gating them.
	if p.introTicks <= 0 && p.endingTicks <= 0 {
		if p.cfg.DiveChance > 0 && p.rng.Float64() < p.cfg.DiveChance {
			p.diveLocked("rolled")
		}
		if p.cfg.QuackChance > 0 && p.rng.Float64() < p.cfg.QuackChance {
			p.quackLocked("rolled")
		}
		if p.gustTicks == 0 && p.cfg.GustChance > 0 && p.rng.Float64() < p.cfg.GustChance {
			p.startGustLocked("started")
		}
		if p.calmTicks == 0 && p.cfg.CalmChance > 0 && p.rng.Float64() < p.cfg.CalmChance {
			p.startCalmLocked("started")
		}
	}

	p.stepRipplesLocked()
	p.paintFrameLocked()
}

func (p *Pond) stepRipplesLocked() {
	if len(p.ripples) == 0 {
		return
	}
	alive := p.ripples[:0]
	for _, r := range p.ripples {
		r.Radius += 0.35
		r.Life--
		if r.Life > 0 && r.Radius < float64(p.W) {
			alive = append(alive, r)
		}
	}
	p.ripples = alive
}

func (p *Pond) surfaceRowLocked() int {
	row := int(math.Round(float64(p.H-1) * p.cfg.Waterline))
	if row < 1 {
		row = 1
	}
	if row >= p.H {
		row = p.H - 1
	}
	return row
}

func (p *Pond) paintFrameLocked() {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	for y := range p.Grid {
		for x := range p.Grid[y] {
			p.Grid[y][x] = Pixel{}
		}
	}
	surface := p.surfaceRowLocked()
	p.paintSkyLocked(surface)
	p.paintWaterLocked(surface)
	p.paintRipplesLocked(surface)
	p.paintDucksLocked(surface)
}

func (p *Pond) paintSkyLocked(surface int) {
	if surface <= 0 {
		return
	}
	hue := math.Mod(p.cfg.WaterHue+p.cfg.HueSpread*0.6+360, 360)
	sat := clamp01(p.cfg.Saturation * 0.5)
	for y := 0; y < surface; y++ {
		t := 1.0
		if surface > 1 {
			t = float64(y) / float64(surface-1)
		}
		light := clamp01(p.cfg.LightMax*(0.55+0.35*t) + p.cfg.LightMin*0.1)
		c := hslToRGB(hue, sat, light)
		for x := 0; x < p.W; x++ {
			p.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (p *Pond) paintWaterLocked(surface int) {
	if surface >= p.H {
		return
	}
	hue := math.Mod(p.cfg.WaterHue+360, 360)
	sat := clamp01(p.cfg.Saturation)
	depth := math.Max(1, float64(p.H-1-surface))
	motion := p.motionLevelLocked()
	for y := surface; y < p.H; y++ {
		d := float64(y-surface) / depth
		baseLight := p.cfg.LightMin + (p.cfg.LightMax-p.cfg.LightMin)*(0.55-0.35*d)
		baseLight = clamp01(baseLight)
		shimmer := math.Sin(float64(y)*0.27 + float64(p.tick)*0.05)
		for x := 0; x < p.W; x++ {
			wave := math.Sin(float64(x)*p.cfg.WaveFreq + float64(p.tick)*0.06)
			light := clamp01(baseLight + (p.cfg.LightMax-p.cfg.LightMin)*0.05*wave*motion + 0.02*shimmer)
			hueShift := math.Sin(float64(x)*0.05+float64(y)*0.04) * p.cfg.HueSpread * 0.15
			p.Grid[y][x] = Pixel{Filled: true, C: hslToRGB(math.Mod(hue+hueShift+360, 360), sat, light)}
		}
	}
	// Surface meniscus — a brighter waterline so the seam reads as water,
	// not just a flat color change.
	highlight := hslToRGB(math.Mod(hue-6+360, 360), clamp01(sat*0.7), clamp01(p.cfg.LightMax*0.95))
	for x := 0; x < p.W; x++ {
		wave := math.Sin(float64(x)*p.cfg.WaveFreq + float64(p.tick)*0.08)
		row := surface + int(math.Round(wave*p.cfg.WaveAmp*motion*0.5))
		if row < surface {
			row = surface
		}
		if row >= p.H {
			row = p.H - 1
		}
		p.Grid[row][x] = Pixel{Filled: true, C: highlight}
	}
}

func (p *Pond) paintRipplesLocked(surface int) {
	if len(p.ripples) == 0 {
		return
	}
	hue := math.Mod(p.cfg.WaterHue+12+360, 360)
	for _, r := range p.ripples {
		fade := clamp01(float64(r.Life) / math.Max(1, float64(r.MaxLife)))
		if fade <= 0 {
			continue
		}
		light := clamp01(p.cfg.LightMin + (p.cfg.LightMax-p.cfg.LightMin)*(0.5+0.45*fade))
		c := hslToRGB(hue, clamp01(p.cfg.Saturation*0.6), light)
		// Paint an ellipse rim (squashed vertically so it reads as a
		// circular ripple on a tilted water surface).
		left := int(math.Round(r.X - r.Radius))
		right := int(math.Round(r.X + r.Radius))
		top := int(math.Round(r.Y - r.Radius*0.45))
		bottom := int(math.Round(r.Y + r.Radius*0.45))
		for _, x := range []int{left, right} {
			if x < 0 || x >= p.W {
				continue
			}
			y := int(math.Round(r.Y))
			if y > surface && y < p.H {
				paintPixel(p.Grid, x, y, c)
			}
		}
		for _, y := range []int{top, bottom} {
			if y <= surface || y >= p.H {
				continue
			}
			x := int(math.Round(r.X))
			if x >= 0 && x < p.W {
				paintPixel(p.Grid, x, y, c)
			}
		}
	}
}

func (p *Pond) paintDucksLocked(surface int) {
	if len(p.ducks) == 0 {
		return
	}
	sat := clamp01(p.cfg.Saturation * 0.9)
	bodyLight := clamp01(p.cfg.LightMin + (p.cfg.LightMax-p.cfg.LightMin)*0.6)
	headLight := clamp01(p.cfg.LightMin + (p.cfg.LightMax-p.cfg.LightMin)*0.78)
	billLight := clamp01(p.cfg.LightMax * 0.92)
	for i, d := range p.ducks {
		if d.DiveLeft > 0 {
			// Diving — just show a small bubble / disturbance, no body.
			x, y := p.duckPos(d)
			col := int(math.Round(x))
			row := int(math.Round(y))
			if row <= surface || row >= p.H || col < 0 || col >= p.W {
				continue
			}
			pip := hslToRGB(math.Mod(p.cfg.WaterHue+360, 360), clamp01(p.cfg.Saturation*0.5), clamp01(p.cfg.LightMax*0.9))
			paintPixel(p.Grid, col, row, pip)
			continue
		}
		x, y := p.duckPos(d)
		col := int(math.Round(x))
		row := int(math.Round(y))
		if row <= surface || row >= p.H || col < 0 || col >= p.W {
			continue
		}
		hue := math.Mod(d.Hue+360, 360)
		body := hslToRGB(hue, sat, bodyLight)
		head := hslToRGB(math.Mod(hue+8+360, 360), clamp01(sat*0.85), headLight)
		bill := hslToRGB(math.Mod(hue-12+360, 360), clamp01(sat*0.9), billLight)
		// Body sits at row; head one row up; bill one column ahead in swim direction.
		paintPixel(p.Grid, col, row, body)
		paintPixel(p.Grid, col-1, row, body)
		paintPixel(p.Grid, col, row-1, head)
		// Bill direction follows the swim tangent.
		swimDir := 1
		// Tangent of the circle: -sin(angle) horizontally; sign of AngularVel decides direction.
		tang := -math.Sin(d.Angle) * math.Copysign(1, d.AngularVel)
		if tang < 0 {
			swimDir = -1
		}
		paintPixel(p.Grid, col+swimDir, row-1, bill)
		_ = i
	}
}
