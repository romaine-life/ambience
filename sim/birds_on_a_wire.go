package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	BirdsOnAWireStateArriving  = "arriving"
	BirdsOnAWireStatePerched   = "perched"
	BirdsOnAWireStateDeparting = "departing"
	BirdsOnAWireStateGone      = "gone"
)

// BirdsOnAWireConfig tunes the birds-on-a-wire effect.
type BirdsOnAWireConfig struct {
	// INTRODUCTION
	IntroDur       int `json:"intro_dur"`
	IntroLandEvery int `json:"intro_land_every"`
	IntroTarget    int `json:"intro_target"`
	// ENDING
	EndingDur         int `json:"ending_dur"`
	OutroTakeoffEvery int `json:"outro_every"`
	ResidualLife      int `json:"residual_life"`
	// SCENE
	SkyHue       float64 `json:"sky_hue"`
	SkySat       float64 `json:"sky_sat"`
	TopLight     float64 `json:"top_light"`
	HorizonLight float64 `json:"horizon_light"`
	HorizonY     float64 `json:"horizon_y"`
	WireY        float64 `json:"wire_y"`
	WireSag      float64 `json:"wire_sag"`
	WireCount    int     `json:"wire_count"`
	// BIRDS
	MaxBirds     int     `json:"max_birds"`
	PerchSpacing float64 `json:"perch_spacing"`
	ArrivalEvery int     `json:"arrival_every"`
	PairChance   float64 `json:"pair_chance"`
	BobChance    float64 `json:"bob_chance"`
	TakeoffEvery int     `json:"takeoff_every"`
	FlockChance  float64 `json:"flock_chance"`
	QuietDur     int     `json:"quiet_dur"`
	QuietArrival float64 `json:"quiet_arrival"`
}

func (c BirdsOnAWireConfig) withDefaults() BirdsOnAWireConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 180
	}
	if c.IntroLandEvery <= 0 {
		c.IntroLandEvery = 28
	}
	if c.IntroTarget <= 0 {
		c.IntroTarget = 5
	}
	if c.IntroTarget > 24 {
		c.IntroTarget = 24
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 180
	}
	if c.OutroTakeoffEvery <= 0 {
		c.OutroTakeoffEvery = 26
	}
	if c.ResidualLife <= 0 {
		c.ResidualLife = 90
	}
	if c.SkyHue == 0 {
		c.SkyHue = 34
	}
	if c.SkySat <= 0 {
		c.SkySat = 0.52
	}
	c.SkySat = clamp01(c.SkySat)
	if c.TopLight <= 0 {
		c.TopLight = 0.20
	}
	c.TopLight = clamp01(c.TopLight)
	if c.HorizonLight <= 0 {
		c.HorizonLight = 0.56
	}
	c.HorizonLight = clamp01(c.HorizonLight)
	if c.HorizonY <= 0 {
		c.HorizonY = 0.78
	}
	if c.HorizonY < 0.55 {
		c.HorizonY = 0.55
	}
	if c.HorizonY > 0.94 {
		c.HorizonY = 0.94
	}
	if c.WireY <= 0 {
		c.WireY = 0.42
	}
	if c.WireY < 0.18 {
		c.WireY = 0.18
	}
	if c.WireY > 0.72 {
		c.WireY = 0.72
	}
	if c.WireSag <= 0 {
		c.WireSag = 3.2
	}
	if c.WireSag > 10 {
		c.WireSag = 10
	}
	if c.WireCount <= 0 {
		c.WireCount = 1
	}
	if c.WireCount > 2 {
		c.WireCount = 2
	}
	if c.MaxBirds <= 0 {
		c.MaxBirds = 12
	}
	if c.MaxBirds > 36 {
		c.MaxBirds = 36
	}
	if c.PerchSpacing <= 0 {
		c.PerchSpacing = 5.5
	}
	if c.PerchSpacing < 2.5 {
		c.PerchSpacing = 2.5
	}
	if c.ArrivalEvery <= 0 {
		c.ArrivalEvery = 190
	}
	if c.PairChance < 0 {
		c.PairChance = 0
	} else if c.PairChance == 0 {
		c.PairChance = 0.22
	}
	if c.PairChance > 1 {
		c.PairChance = 1
	}
	if c.BobChance < 0 {
		c.BobChance = 0
	}
	if c.BobChance == 0 {
		c.BobChance = 0.003
	}
	if c.TakeoffEvery <= 0 {
		c.TakeoffEvery = 540
	}
	if c.FlockChance < 0 {
		c.FlockChance = 0
	}
	if c.FlockChance == 0 {
		c.FlockChance = 0.24
	}
	if c.FlockChance > 1 {
		c.FlockChance = 1
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 720
	}
	if c.QuietArrival <= 0 {
		c.QuietArrival = 0.12
	}
	if c.QuietArrival > 1 {
		c.QuietArrival = 1
	}
	return c
}

// BirdsOnAWireSchema describes the birds-on-a-wire effect's tunable knobs.
func BirdsOnAWireSchema() EffectSchema {
	return EffectSchema{
		Name:           "birds-on-a-wire",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 40, Max: 420, Step: 10, Default: 180, Trigger: "intro",
				Description: "Ticks for an empty wire to fill toward the intro target."},
			{Key: "intro_land_every", Label: "intro arrivals", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 8, Max: 100, Step: 2, Default: 28,
				Description: "Spacing between early arrivals after intro starts."},
			{Key: "intro_target", Label: "intro target", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 1, Max: 18, Step: 1, Default: 5,
				Description: "Bird count the intro tries to establish quickly."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "outro", Type: KnobInt, Min: 40, Max: 420, Step: 10, Default: 180, Trigger: "ending",
				Description: "Ticks spent sending perched birds away before holding the empty wire."},
			{Key: "outro_every", Label: "outro cadence", Slot: SlotEnd, Group: "outro", Type: KnobInt, Min: 8, Max: 90, Step: 2, Default: 26,
				Description: "Takeoff spacing while the outro empties the wire."},
			{Key: "residual_life", Label: "residual life", Slot: SlotEnd, Group: "outro", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 90,
				Description: "Maximum extra perch time assigned to birds at outro start."},
			{Key: "sky_hue", Label: "sky hue", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 34,
				Description: "Base sky hue; warm dusk and cool dawn live here."},
			{Key: "sky_sat", Label: "sky sat", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.52,
				Description: "Sky saturation. Lower values read as overcast."},
			{Key: "top_light", Label: "top light", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.20,
				Description: "Brightness at the top of the sky gradient."},
			{Key: "horizon_light", Label: "horizon light", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.12, Max: 0.82, Step: 0.01, Default: 0.56,
				Description: "Brightness near the rooftop and treeline silhouette."},
			{Key: "horizon_y", Label: "horizon", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.55, Max: 0.94, Step: 0.01, Default: 0.78,
				Description: "Vertical position of the distant rooftop and treeline band."},
			{Key: "wire_y", Label: "wire height", Slot: SlotLever, Group: "wire", Type: KnobFloat, Min: 0.18, Max: 0.72, Step: 0.01, Default: 0.42,
				Description: "Vertical position of the upper wire."},
			{Key: "wire_sag", Label: "wire sag", Slot: SlotLever, Group: "wire", Type: KnobFloat, Min: 0.2, Max: 10, Step: 0.1, Default: 3.2,
				Description: "Center sag in grid cells."},
			{Key: "wire_count", Label: "wire count", Slot: SlotLever, Group: "wire", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 1,
				Description: "One power line or a lower telephone-row pair."},
			{Key: "max_birds", Label: "max birds", Slot: SlotLever, Group: "birds", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 12,
				Description: "Capacity cap for settled and incoming birds."},
			{Key: "perch_spacing", Label: "spacing", Slot: SlotLever, Group: "birds", Type: KnobFloat, Min: 2.5, Max: 12, Step: 0.25, Default: 5.5,
				Description: "Minimum space between selected perches on the same wire."},
			{Key: "arrival_every", Label: "arrival every", Slot: SlotEvent, Group: "arrivals", Type: KnobInt, Min: 30, Max: 900, Step: 10, Default: 190, Trigger: "bird-land",
				Description: "Average ticks between steady-state arriving birds."},
			{Key: "pair_chance", Label: "pair chance", Slot: SlotEventMod, Group: "arrivals", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.22,
				Description: "Chance that an arrival event brings a second bird."},
			{Key: "bob_chance", Label: "bob chance", Slot: SlotEvent, Group: "idle", Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0.003, Trigger: "bird-bob",
				Description: "Per-bird idle chance for a preen, shuffle, or bob."},
			{Key: "takeoff_every", Label: "takeoff every", Slot: SlotEvent, Group: "takeoff", Type: KnobInt, Min: 120, Max: 1800, Step: 20, Default: 540, Trigger: "single-takeoff",
				Description: "Average ticks between natural departure checks."},
			{Key: "flock_chance", Label: "flock chance", Slot: SlotEventMod, Group: "takeoff", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.24, Trigger: "flock-takeoff",
				Description: "Chance that a departure check startles a nearby cluster."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet", Type: KnobInt, Min: 120, Max: 2400, Step: 30, Default: 720, Trigger: "quiet-wire",
				Description: "Suppression window where the wire stays empty or near-empty."},
			{Key: "quiet_arrival", Label: "quiet arrivals", Slot: SlotEventMod, Group: "quiet", Type: KnobFloat, Min: 0.01, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Arrival-rate multiplier while quiet-wire is active."},
		},
	}
}

// BirdsOnAWireBird is one bird entity in the server/client wire state.
type BirdsOnAWireBird struct {
	ID       int     `json:"id"`
	State    string  `json:"state"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	StartX   float64 `json:"startX,omitempty"`
	StartY   float64 `json:"startY,omitempty"`
	TargetX  float64 `json:"targetX,omitempty"`
	TargetY  float64 `json:"targetY,omitempty"`
	Wire     int     `json:"wire,omitempty"`
	Facing   int     `json:"facing,omitempty"`
	Age      int     `json:"age,omitempty"`
	T        int     `json:"t,omitempty"`
	Duration int     `json:"duration,omitempty"`
	Phase    float64 `json:"phase,omitempty"`
	BobTicks int     `json:"bobTicks,omitempty"`
	Perched  int     `json:"perched,omitempty"`
	DepartAt int     `json:"departAt,omitempty"`
}

// BirdsOnAWireSnapshot is the server/client wire state for birds-on-a-wire.
type BirdsOnAWireSnapshot struct {
	Tick         int                `json:"tick"`
	Lifecycle    Lifecycle          `json:"lifecycle"`
	Birds        []BirdsOnAWireBird `json:"birds"`
	IntroTicks   int                `json:"introTicks,omitempty"`
	IntroTarget  int                `json:"introTarget,omitempty"`
	EndingTicks  int                `json:"endingTicks,omitempty"`
	QuietTicks   int                `json:"quietTicks,omitempty"`
	ArrivalTicks int                `json:"arrivalTicks,omitempty"`
	TakeoffTicks int                `json:"takeoffTicks,omitempty"`
	Ended        bool               `json:"ended,omitempty"`
	NextID       int                `json:"nextId,omitempty"`
	RNGState     uint64             `json:"rngState,omitempty"`
}

// BirdsOnAWirePersistedState is the restart-safe state for birds-on-a-wire.
type BirdsOnAWirePersistedState struct {
	Tick         int                `json:"tick"`
	Birds        []BirdsOnAWireBird `json:"birds"`
	IntroTicks   int                `json:"introTicks,omitempty"`
	IntroTarget  int                `json:"introTarget,omitempty"`
	EndingTicks  int                `json:"endingTicks,omitempty"`
	QuietTicks   int                `json:"quietTicks,omitempty"`
	ArrivalTicks int                `json:"arrivalTicks,omitempty"`
	TakeoffTicks int                `json:"takeoffTicks,omitempty"`
	Ended        bool               `json:"ended,omitempty"`
	NextID       int                `json:"nextId,omitempty"`
	RNGState     uint64             `json:"rngState"`
}

// BirdsOnAWire is the authoritative pixel-grid simulation for birds-on-a-wire.
type BirdsOnAWire struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng *rngutil.RNG
	cfg BirdsOnAWireConfig

	tick         int
	birds        []BirdsOnAWireBird
	introTicks   int
	introTarget  int
	endingTicks  int
	quietTicks   int
	arrivalTicks int
	takeoffTicks int
	ended        bool
	nextID       int
	log          []LogEntry
}

func NewBirdsOnAWire(w, h int, seed int64, cfg BirdsOnAWireConfig) *BirdsOnAWire {
	cfg = cfg.withDefaults()
	e := &BirdsOnAWire{
		rng:          rngutil.New(seed),
		cfg:          cfg,
		arrivalTicks: 1,
		takeoffTicks: cfg.TakeoffEvery,
		introTarget:  cfg.IntroTarget,
		nextID:       1,
	}
	e.Resize(w, h)
	return e
}

func (e *BirdsOnAWire) Resize(w, h int) {
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
	e.retargetBirdsLocked()
	e.renderLocked()
}

func (e *BirdsOnAWire) EffectiveConfig() BirdsOnAWireConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *BirdsOnAWire) SetConfig(cfg BirdsOnAWireConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg.withDefaults()
	if e.introTarget > e.cfg.IntroTarget {
		e.introTarget = e.cfg.IntroTarget
	}
	e.retargetBirdsLocked()
	e.renderLocked()
}

func (e *BirdsOnAWire) Snapshot() BirdsOnAWireSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return BirdsOnAWireSnapshot{
		Tick:         e.tick,
		Lifecycle:    e.lifecycleLocked(),
		Birds:        cloneBirdsOnAWireBirds(e.birds),
		IntroTicks:   e.introTicks,
		IntroTarget:  e.introTarget,
		EndingTicks:  e.endingTicks,
		QuietTicks:   e.quietTicks,
		ArrivalTicks: e.arrivalTicks,
		TakeoffTicks: e.takeoffTicks,
		Ended:        e.ended,
		NextID:       e.nextID,
		RNGState:     e.rng.State(),
	}
}

func (e *BirdsOnAWire) RestoreSnapshot(snap BirdsOnAWireSnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick = snap.Tick
	e.birds = cloneBirdsOnAWireBirds(snap.Birds)
	e.introTicks = snap.IntroTicks
	e.introTarget = snap.IntroTarget
	e.endingTicks = snap.EndingTicks
	e.quietTicks = snap.QuietTicks
	e.arrivalTicks = snap.ArrivalTicks
	e.takeoffTicks = snap.TakeoffTicks
	e.ended = snap.Ended || snap.Lifecycle == LifecycleEnded
	e.nextID = snap.NextID
	if e.nextID <= 0 {
		e.nextID = nextBirdsOnAWireID(e.birds)
	}
	if e.arrivalTicks <= 0 {
		e.arrivalTicks = 1
	}
	if e.takeoffTicks <= 0 {
		e.takeoffTicks = e.cfg.TakeoffEvery
	}
	if snap.RNGState != 0 {
		e.rng.SetState(snap.RNGState)
	}
	e.retargetBirdsLocked()
	e.renderLocked()
}

func (e *BirdsOnAWire) SnapshotPersistedState() BirdsOnAWirePersistedState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return BirdsOnAWirePersistedState{
		Tick:         e.tick,
		Birds:        cloneBirdsOnAWireBirds(e.birds),
		IntroTicks:   e.introTicks,
		IntroTarget:  e.introTarget,
		EndingTicks:  e.endingTicks,
		QuietTicks:   e.quietTicks,
		ArrivalTicks: e.arrivalTicks,
		TakeoffTicks: e.takeoffTicks,
		Ended:        e.ended,
		NextID:       e.nextID,
		RNGState:     e.rng.State(),
	}
}

func (e *BirdsOnAWire) RestorePersistedState(state BirdsOnAWirePersistedState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick = state.Tick
	e.birds = cloneBirdsOnAWireBirds(state.Birds)
	e.introTicks = state.IntroTicks
	e.introTarget = state.IntroTarget
	e.endingTicks = state.EndingTicks
	e.quietTicks = state.QuietTicks
	e.arrivalTicks = state.ArrivalTicks
	e.takeoffTicks = state.TakeoffTicks
	e.ended = state.Ended
	e.nextID = state.NextID
	if e.nextID <= 0 {
		e.nextID = nextBirdsOnAWireID(e.birds)
	}
	if e.arrivalTicks <= 0 {
		e.arrivalTicks = 1
	}
	if e.takeoffTicks <= 0 {
		e.takeoffTicks = e.cfg.TakeoffEvery
	}
	if state.RNGState != 0 {
		e.rng.SetState(state.RNGState)
	}
	e.retargetBirdsLocked()
	e.renderLocked()
}

func (e *BirdsOnAWire) CurrentTick() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick
}

func (e *BirdsOnAWire) PerturbRNG(delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rng.Mix(delta)
}

func (e *BirdsOnAWire) DrainLog() []LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.log) == 0 {
		return nil
	}
	out := e.log
	e.log = nil
	return out
}

func (e *BirdsOnAWire) TriggerEvent(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	var ok bool
	switch name {
	case "intro":
		e.startIntroLocked()
		ok = true
	case "ending":
		e.startEndingLocked()
		ok = true
	case "bird-land":
		ok = e.spawnArrivalGroupLocked(true)
	case "bird-bob":
		ok = e.startRandomBobLocked(true)
		if !ok {
			e.appendLogLocked("bird-bob", "empty wire stayed still")
			ok = true
		}
	case "single-takeoff":
		ok = e.startSingleTakeoffLocked(true)
		if !ok {
			e.appendLogLocked("single-takeoff", "empty wire stayed still")
			ok = true
		}
	case "flock-takeoff":
		ok = e.startFlockTakeoffLocked(true)
		if !ok {
			e.appendLogLocked("flock-takeoff", "empty wire stayed still")
			ok = true
		}
	case "quiet-wire":
		e.startQuietLocked()
		ok = true
	default:
		return false
	}
	e.renderLocked()
	return ok
}

func (e *BirdsOnAWire) Step() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick++

	if e.introTicks > 0 {
		e.introTicks--
		if e.introTicks == 0 {
			e.introTarget = 0
		}
	}
	if e.quietTicks > 0 {
		e.quietTicks--
	}

	e.advanceBirdsLocked()
	e.trimGoneBirdsLocked()

	if e.endingTicks > 0 {
		e.runEndingLocked()
	} else if !e.ended {
		e.rollArrivalsLocked()
		e.rollTakeoffsLocked()
		e.rollIdleLocked()
		if e.introTicks == 0 && e.introTarget > 0 && e.countSettlingLocked() >= e.introTarget {
			e.introTarget = 0
		}
	}

	e.renderLocked()
}

func (e *BirdsOnAWire) GridCopy() [][]Pixel {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]Pixel, len(e.Grid))
	for y := range e.Grid {
		out[y] = make([]Pixel, len(e.Grid[y]))
		copy(out[y], e.Grid[y])
	}
	return out
}

func (e *BirdsOnAWire) lifecycleLocked() Lifecycle {
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

func (e *BirdsOnAWire) startIntroLocked() {
	e.ended = false
	e.endingTicks = 0
	e.quietTicks = 0
	e.birds = nil
	e.introTicks = e.cfg.IntroDur
	e.introTarget = e.cfg.IntroTarget
	e.arrivalTicks = 1
	e.takeoffTicks = e.cfg.TakeoffEvery
	e.appendLogLocked("intro", fmt.Sprintf("wire reset; first arrivals target %d", e.introTarget))
}

func (e *BirdsOnAWire) startEndingLocked() {
	e.introTicks = 0
	e.introTarget = 0
	e.quietTicks = 0
	e.endingTicks = e.cfg.EndingDur
	e.arrivalTicks = e.cfg.ArrivalEvery
	e.ended = false
	for i := range e.birds {
		if e.birds[i].State == BirdsOnAWireStatePerched {
			e.birds[i].DepartAt = e.tick + e.rng.Intn(max(1, e.cfg.ResidualLife+1))
		}
	}
	e.appendLogLocked("ending", fmt.Sprintf("emptying wire over %d ticks", e.endingTicks))
}

func (e *BirdsOnAWire) startQuietLocked() {
	e.quietTicks = e.cfg.QuietDur
	e.arrivalTicks = max(1, int(float64(e.cfg.ArrivalEvery)/math.Max(0.02, e.cfg.QuietArrival)))
	e.startClusterTakeoffLocked(int(math.Ceil(float64(e.countPerchedLocked())*0.65)), true)
	e.appendLogLocked("quiet-wire", fmt.Sprintf("arrivals suppressed for %d ticks", e.quietTicks))
}

func (e *BirdsOnAWire) runEndingLocked() {
	if e.tick%max(1, e.cfg.OutroTakeoffEvery) == 0 {
		if e.countPerchedLocked() > 1 && e.rng.Float64() < 0.35 {
			e.startClusterTakeoffLocked(2, false)
		} else {
			e.startSingleTakeoffLocked(false)
		}
	}
	for i := range e.birds {
		if e.birds[i].State == BirdsOnAWireStatePerched && e.birds[i].DepartAt > 0 && e.tick >= e.birds[i].DepartAt {
			e.startDepartingLocked(i)
		}
	}
	e.endingTicks--
	if e.endingTicks <= 0 {
		e.endingTicks = 0
		e.birds = nil
		e.ended = true
		e.appendLogLocked("ending", "wire empty")
	}
}

func (e *BirdsOnAWire) rollArrivalsLocked() {
	e.arrivalTicks--
	if e.arrivalTicks > 0 {
		return
	}
	base := e.currentArrivalEveryLocked()
	if e.canAcceptBirdLocked() {
		e.spawnArrivalGroupLocked(false)
	}
	e.arrivalTicks = e.jitterTicksLocked(base, 0.45)
}

func (e *BirdsOnAWire) rollTakeoffsLocked() {
	e.takeoffTicks--
	if e.takeoffTicks > 0 {
		return
	}
	if e.countPerchedLocked() > 0 {
		if e.countPerchedLocked() >= 3 && e.rng.Float64() < e.cfg.FlockChance {
			e.startFlockTakeoffLocked(false)
		} else {
			e.startSingleTakeoffLocked(false)
		}
	}
	e.takeoffTicks = e.jitterTicksLocked(e.cfg.TakeoffEvery, 0.5)
}

func (e *BirdsOnAWire) rollIdleLocked() {
	for i := range e.birds {
		if e.birds[i].State != BirdsOnAWireStatePerched || e.birds[i].BobTicks > 0 {
			continue
		}
		if e.rng.Float64() < e.cfg.BobChance {
			e.startBobLocked(i, false)
		}
	}
}

func (e *BirdsOnAWire) currentArrivalEveryLocked() int {
	if e.introTarget > 0 && e.countSettlingLocked() < e.introTarget {
		return e.cfg.IntroLandEvery
	}
	if e.quietTicks > 0 {
		return max(1, int(float64(e.cfg.ArrivalEvery)/math.Max(0.02, e.cfg.QuietArrival)))
	}
	return e.cfg.ArrivalEvery
}

func (e *BirdsOnAWire) spawnArrivalGroupLocked(manual bool) bool {
	if e.ended || e.endingTicks > 0 {
		return false
	}
	count := 1
	if e.rng.Float64() < e.cfg.PairChance {
		count = 2
	}
	spawned := 0
	for i := 0; i < count; i++ {
		if e.spawnBirdLocked() {
			spawned++
		}
	}
	if spawned > 0 && manual {
		e.appendLogLocked("bird-land", fmt.Sprintf("%d bird arrival triggered", spawned))
	}
	return spawned > 0
}

func (e *BirdsOnAWire) spawnBirdLocked() bool {
	if !e.canAcceptBirdLocked() {
		return false
	}
	x, wire, ok := e.pickFreePerchLocked()
	if !ok {
		return false
	}
	targetY := e.wireYAtLocked(x, wire) - 1
	fromLeft := e.rng.Intn(2) == 0
	if x < float64(e.W)*0.35 {
		fromLeft = true
	} else if x > float64(e.W)*0.65 {
		fromLeft = false
	}
	startX := -5.0
	facing := 1
	if !fromLeft {
		startX = float64(e.W + 5)
		facing = -1
	}
	startY := targetY - (6 + e.rng.Float64()*10)
	dist := math.Hypot(x-startX, targetY-startY)
	dur := int(math.Max(42, math.Min(150, dist*1.8)))
	b := BirdsOnAWireBird{
		ID:       e.nextID,
		State:    BirdsOnAWireStateArriving,
		X:        startX,
		Y:        startY,
		StartX:   startX,
		StartY:   startY,
		TargetX:  x,
		TargetY:  targetY,
		Wire:     wire,
		Facing:   facing,
		Duration: dur,
		Phase:    e.rng.Float64() * math.Pi * 2,
	}
	e.nextID++
	e.birds = append(e.birds, b)
	return true
}

func (e *BirdsOnAWire) pickFreePerchLocked() (float64, int, bool) {
	if e.W < 8 {
		return float64(e.W) / 2, 0, true
	}
	margin := math.Min(7, math.Max(2, float64(e.W)*0.08))
	for attempt := 0; attempt < 80; attempt++ {
		wire := e.rng.Intn(e.cfg.WireCount)
		x := margin + e.rng.Float64()*math.Max(1, float64(e.W)-1-2*margin)
		if e.perchFreeLocked(x, wire) {
			return x, wire, true
		}
	}
	return 0, 0, false
}

func (e *BirdsOnAWire) perchFreeLocked(x float64, wire int) bool {
	spacing := e.cfg.PerchSpacing
	for _, b := range e.birds {
		if b.State == BirdsOnAWireStateDeparting || b.State == BirdsOnAWireStateGone || b.Wire != wire {
			continue
		}
		bx := b.TargetX
		if b.State == BirdsOnAWireStatePerched {
			bx = b.X
		}
		if math.Abs(bx-x) < spacing {
			return false
		}
	}
	return true
}

func (e *BirdsOnAWire) canAcceptBirdLocked() bool {
	return e.countActiveBirdsLocked() < e.cfg.MaxBirds
}

func (e *BirdsOnAWire) countActiveBirdsLocked() int {
	n := 0
	for _, b := range e.birds {
		if b.State != BirdsOnAWireStateDeparting && b.State != BirdsOnAWireStateGone {
			n++
		}
	}
	return n
}

func (e *BirdsOnAWire) countSettlingLocked() int {
	n := 0
	for _, b := range e.birds {
		if b.State == BirdsOnAWireStateArriving || b.State == BirdsOnAWireStatePerched {
			n++
		}
	}
	return n
}

func (e *BirdsOnAWire) countPerchedLocked() int {
	n := 0
	for _, b := range e.birds {
		if b.State == BirdsOnAWireStatePerched {
			n++
		}
	}
	return n
}

func (e *BirdsOnAWire) startRandomBobLocked(manual bool) bool {
	for attempt := 0; attempt < len(e.birds)*2; attempt++ {
		i := e.rng.Intn(len(e.birds))
		if e.birds[i].State == BirdsOnAWireStatePerched {
			e.startBobLocked(i, manual)
			return true
		}
	}
	return false
}

func (e *BirdsOnAWire) startBobLocked(i int, manual bool) {
	e.birds[i].BobTicks = 34 + e.rng.Intn(26)
	if e.rng.Float64() < 0.25 {
		e.birds[i].Facing *= -1
		if e.birds[i].Facing == 0 {
			e.birds[i].Facing = 1
		}
	}
	if manual {
		e.appendLogLocked("bird-bob", fmt.Sprintf("bird %d shuffled", e.birds[i].ID))
	}
}

func (e *BirdsOnAWire) startSingleTakeoffLocked(manual bool) bool {
	perched := e.perchedIndicesLocked()
	if len(perched) == 0 {
		return false
	}
	i := perched[e.rng.Intn(len(perched))]
	id := e.birds[i].ID
	e.startDepartingLocked(i)
	if manual {
		e.appendLogLocked("single-takeoff", fmt.Sprintf("bird %d lifted off", id))
	}
	return true
}

func (e *BirdsOnAWire) startFlockTakeoffLocked(manual bool) bool {
	if e.countPerchedLocked() == 0 {
		return false
	}
	n := 2 + e.rng.Intn(3)
	ok := e.startClusterTakeoffLocked(n, false)
	if ok && manual {
		e.appendLogLocked("flock-takeoff", "nearby birds startled together")
	}
	return ok
}

func (e *BirdsOnAWire) startClusterTakeoffLocked(limit int, broad bool) bool {
	perched := e.perchedIndicesLocked()
	if len(perched) == 0 || limit <= 0 {
		return false
	}
	if limit > len(perched) {
		limit = len(perched)
	}
	anchor := e.birds[perched[e.rng.Intn(len(perched))]]
	selected := 0
	radius := 18.0
	if broad {
		radius = float64(e.W)
	}
	for _, idx := range perched {
		if selected >= limit {
			break
		}
		b := e.birds[idx]
		if broad || (b.Wire == anchor.Wire && math.Abs(b.X-anchor.X) <= radius) {
			e.startDepartingLocked(idx)
			selected++
		}
	}
	if selected == 0 {
		e.startDepartingLocked(perched[0])
		selected = 1
	}
	return selected > 0
}

func (e *BirdsOnAWire) perchedIndicesLocked() []int {
	out := make([]int, 0, len(e.birds))
	for i, b := range e.birds {
		if b.State == BirdsOnAWireStatePerched {
			out = append(out, i)
		}
	}
	return out
}

func (e *BirdsOnAWire) startDepartingLocked(i int) {
	b := &e.birds[i]
	b.State = BirdsOnAWireStateDeparting
	b.StartX = b.X
	b.StartY = b.Y
	b.T = 0
	b.Duration = 48 + e.rng.Intn(48)
	if b.Facing == 0 {
		b.Facing = 1
	}
	if e.rng.Float64() < 0.35 {
		b.Facing *= -1
	}
	b.TargetX = float64(e.W + 6)
	if b.Facing < 0 {
		b.TargetX = -6
	}
	b.TargetY = b.Y - (10 + e.rng.Float64()*10)
	b.BobTicks = 0
	b.DepartAt = 0
}

func (e *BirdsOnAWire) advanceBirdsLocked() {
	for i := range e.birds {
		b := &e.birds[i]
		b.Age++
		switch b.State {
		case BirdsOnAWireStateArriving, BirdsOnAWireStateDeparting:
			b.T++
			t := clamp01(float64(b.T) / float64(max(1, b.Duration)))
			s := t * t * (3 - 2*t)
			b.X = b.StartX + (b.TargetX-b.StartX)*s
			b.Y = b.StartY + (b.TargetY-b.StartY)*s - math.Sin(t*math.Pi)*2.6
			if b.T >= b.Duration {
				if b.State == BirdsOnAWireStateArriving {
					b.State = BirdsOnAWireStatePerched
					b.X = b.TargetX
					b.Y = e.wireYAtLocked(b.TargetX, b.Wire) - 1
					b.T = 0
					b.Perched = 0
					b.Duration = 0
					b.BobTicks = 10 + e.rng.Intn(18)
					e.appendLogLocked("bird-land", fmt.Sprintf("bird %d settled", b.ID))
				} else {
					b.State = BirdsOnAWireStateGone
				}
			}
		case BirdsOnAWireStatePerched:
			b.Perched++
			b.TargetY = e.wireYAtLocked(b.X, b.Wire) - 1
			b.Y = b.TargetY
			if b.BobTicks > 0 {
				b.BobTicks--
			}
		}
	}
}

func (e *BirdsOnAWire) trimGoneBirdsLocked() {
	out := e.birds[:0]
	for _, b := range e.birds {
		if b.State != BirdsOnAWireStateGone {
			out = append(out, b)
		}
	}
	e.birds = out
}

func (e *BirdsOnAWire) retargetBirdsLocked() {
	for i := range e.birds {
		b := &e.birds[i]
		if b.Wire >= e.cfg.WireCount {
			b.Wire = e.cfg.WireCount - 1
		}
		if b.Wire < 0 {
			b.Wire = 0
		}
		if b.State == BirdsOnAWireStateArriving || b.State == BirdsOnAWireStatePerched {
			b.TargetY = e.wireYAtLocked(b.TargetX, b.Wire) - 1
			if b.State == BirdsOnAWireStatePerched {
				b.Y = e.wireYAtLocked(b.X, b.Wire) - 1
			}
		}
	}
}

func (e *BirdsOnAWire) jitterTicksLocked(base int, spread float64) int {
	if base <= 1 {
		return 1
	}
	f := float64(base) * (1 + spread*(e.rng.Float64()*2-1))
	return max(1, int(math.Round(f)))
}

func (e *BirdsOnAWire) appendLogLocked(kind, desc string) {
	e.log = append(e.log, LogEntry{Tick: e.tick, Type: kind, Desc: desc})
	if len(e.log) > 200 {
		e.log = e.log[len(e.log)-200:]
	}
}

func (e *BirdsOnAWire) renderLocked() {
	for y := 0; y < e.H; y++ {
		t := 0.0
		if e.H > 1 {
			t = float64(y) / float64(e.H-1)
		}
		light := e.cfg.TopLight + (e.cfg.HorizonLight-e.cfg.TopLight)*math.Pow(t, 1.35)
		sat := e.cfg.SkySat * (0.92 - 0.18*t)
		hue := e.cfg.SkyHue + 7*t
		c := hslToRGB(hue, clamp01(sat), clamp01(light))
		for x := 0; x < e.W; x++ {
			e.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
	e.paintSilhouetteLocked()
	e.paintWiresLocked()
	for _, b := range e.birds {
		if b.State == BirdsOnAWireStateDeparting || b.State == BirdsOnAWireStateArriving {
			e.paintFlyingBirdLocked(b)
		} else if b.State == BirdsOnAWireStatePerched {
			e.paintPerchedBirdLocked(b)
		}
	}
}

func (e *BirdsOnAWire) paintSilhouetteLocked() {
	if e.H <= 0 || e.W <= 0 {
		return
	}
	horizon := int(math.Round(e.cfg.HorizonY * float64(e.H-1)))
	if horizon < 0 {
		horizon = 0
	}
	dark := hslToRGB(e.cfg.SkyHue+205, 0.18, math.Max(0.035, e.cfg.TopLight*0.24))
	for x := 0; x < e.W; x++ {
		wave := math.Sin(float64(x)*0.19+0.8) + 0.55*math.Sin(float64(x)*0.47)
		top := horizon + int(math.Round(1.5+wave*1.8))
		seg := x % 19
		if seg >= 3 && seg <= 9 {
			roof := 3 - int(math.Abs(float64(seg-6)))
			top -= roof
		}
		if top < horizon-4 {
			top = horizon - 4
		}
		if top > e.H-1 {
			top = e.H - 1
		}
		for y := top; y < e.H; y++ {
			e.setPixelLocked(x, y, dark)
		}
	}
}

func (e *BirdsOnAWire) paintWiresLocked() {
	wire := color.RGBA{R: 21, G: 18, B: 20, A: 255}
	for w := 0; w < e.cfg.WireCount; w++ {
		prevY := int(math.Round(e.wireYAtLocked(0, w)))
		for x := 0; x < e.W; x++ {
			y := int(math.Round(e.wireYAtLocked(float64(x), w)))
			e.setPixelLocked(x, y, wire)
			if math.Abs(float64(y-prevY)) > 0.5 {
				e.setPixelLocked(x, prevY, wire)
			}
			prevY = y
		}
	}
}

func (e *BirdsOnAWire) paintPerchedBirdLocked(b BirdsOnAWireBird) {
	c := color.RGBA{R: 17, G: 15, B: 18, A: 255}
	x := int(math.Round(b.X))
	y := int(math.Round(b.Y))
	if b.BobTicks > 0 {
		y -= int(math.Round(math.Sin(float64(b.BobTicks)*0.42) * 0.7))
	}
	f := b.Facing
	if f == 0 {
		f = 1
	}
	e.setPixelLocked(x, y-1, c)
	e.setPixelLocked(x-f, y-1, c)
	e.setPixelLocked(x, y-2, c)
	e.setPixelLocked(x+f, y-2, c)
	e.setPixelLocked(x-2*f, y-1, c)
	if b.BobTicks > 12 {
		e.setPixelLocked(x-f, y-3, c)
	}
	e.setPixelLocked(x, y, c)
}

func (e *BirdsOnAWire) paintFlyingBirdLocked(b BirdsOnAWireBird) {
	c := color.RGBA{R: 16, G: 14, B: 18, A: 255}
	x := int(math.Round(b.X))
	y := int(math.Round(b.Y))
	f := b.Facing
	if f == 0 {
		f = 1
	}
	flap := math.Sin(float64(e.tick)*0.38 + b.Phase)
	wingY := -1
	if flap < 0 {
		wingY = 1
	}
	e.setPixelLocked(x, y, c)
	e.setPixelLocked(x+f, y, c)
	e.setPixelLocked(x-f, y+wingY, c)
	e.setPixelLocked(x-2*f, y+wingY*2, c)
	e.setPixelLocked(x+2*f, y-wingY, c)
}

func (e *BirdsOnAWire) setPixelLocked(x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= e.W || y >= e.H {
		return
	}
	e.Grid[y][x] = Pixel{Filled: true, C: c}
}

func (e *BirdsOnAWire) wireYAtLocked(x float64, wire int) float64 {
	if e.H <= 1 {
		return 0
	}
	t := 0.5
	if e.W > 1 {
		t = x / float64(e.W-1)
	}
	curve := 1 - math.Pow(2*t-1, 2)
	y := e.cfg.WireY*float64(e.H-1) + e.cfg.WireSag*curve + float64(wire)*math.Max(5, float64(e.H)*0.13)
	maxY := math.Max(0, float64(e.H)-3)
	if y > maxY {
		y = maxY
	}
	if y < 2 {
		y = 2
	}
	return y
}

func cloneBirdsOnAWireBirds(src []BirdsOnAWireBird) []BirdsOnAWireBird {
	if len(src) == 0 {
		return nil
	}
	out := make([]BirdsOnAWireBird, len(src))
	copy(out, src)
	return out
}

func nextBirdsOnAWireID(birds []BirdsOnAWireBird) int {
	next := 1
	for _, b := range birds {
		if b.ID >= next {
			next = b.ID + 1
		}
	}
	return next
}

func countBirdsOnAWireFilled(grid [][]Pixel) int {
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
