// Package sim implements the ambient effect simulations.
//
// Each effect is a grid of Pixels evolved by a Step() function. The server
// ticks the current effect and broadcasts snapshots; clients render whatever
// they receive without running the simulation themselves.
package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// Pixel is one cell in the grid. Filled=false means transparent/empty.
type Pixel struct {
	Filled bool
	C      color.RGBA
}

// drop is one active raindrop. Its Row/Col are continuous; each tick the drop
// advances by (vRow, vCol) and re-paints its head + trail of streakLen cells
// into the grid. Per-drop fields carry the jitter/layer state picked at spawn.
type drop struct {
	Row, Col   float64
	Color      color.RGBA
	vRow, vCol float64 // movement vector per tick
	streakLen  int     // trail length (including head) for this drop
	background bool    // background-layer drop (dimmer, shorter, slower)
}

// Config tunes Rain. Every knob is a continuous spectrum so the dev UI can
// expose them as sliders. Zero values fall back to sensible defaults via
// withDefaults().
type Config struct {
	// MOTION
	Wind        float64 `json:"wind"`
	WindJitter  float64 `json:"wind_jit"`
	Speed       float64 `json:"speed"`
	SpeedJitter float64 `json:"speed_jit"`
	// SHAPE
	StreakLen  int     `json:"streak"`
	FadeFactor float64 `json:"fade"`
	// SPAWN
	SpawnEvery int `json:"spawn"`
	SpawnBurst int `json:"burst"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// DEPTH
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// LEVERS
	HueDriftAmp  float64 `json:"hue_drift"`
	WindDriftAmp float64 `json:"wind_drift"`
	// EVENT CHANCES
	DownpourChance float64 `json:"downpour_p"`
	CalmChance     float64 `json:"calm_p"`
	GustChance     float64 `json:"gust_p"`
	SplashChance   float64 `json:"splash_p"`
	// EVENT MODIFIERS
	DownpourDur  int     `json:"downpour_dur"`
	DownpourMult float64 `json:"downpour_mult"`
	CalmDur      int     `json:"calm_dur"`
	GustDur      int     `json:"gust_dur"`
	GustStrength float64 `json:"gust_str"`
	SplashSize   int     `json:"splash_size"`
}

func (c Config) withDefaults() Config {
	if c.Speed <= 0 {
		c.Speed = 1.0
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 5
	}
	if c.SpawnBurst <= 0 {
		c.SpawnBurst = 1
	}
	if c.StreakLen <= 0 {
		c.StreakLen = 5
	}
	if c.FadeFactor <= 0 {
		c.FadeFactor = 0.88
	}
	if c.Hue == 0 {
		c.Hue = 210
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.6
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.55
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.85
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.Layers <= 0 {
		c.Layers = 1
	}
	if c.DownpourDur <= 0 {
		c.DownpourDur = 60
	}
	if c.DownpourMult <= 0 {
		c.DownpourMult = 4
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 50
	}
	if c.GustDur <= 0 {
		c.GustDur = 30
	}
	if c.GustStrength <= 0 {
		c.GustStrength = 1.5
	}
	if c.SplashSize <= 0 {
		c.SplashSize = 4
	}
	return c
}

// hslToRGB converts HSL to RGBA (alpha=255). h in degrees [0, 360); s, l in [0, 1].
func hslToRGB(h, s, l float64) color.RGBA {
	c := (1 - math.Abs(2*l-1)) * s
	hp := h / 60
	x := c * (1 - math.Abs(math.Mod(hp, 2)-1))
	var rp, gp, bp float64
	switch {
	case hp < 1:
		rp, gp, bp = c, x, 0
	case hp < 2:
		rp, gp, bp = x, c, 0
	case hp < 3:
		rp, gp, bp = 0, c, x
	case hp < 4:
		rp, gp, bp = 0, x, c
	case hp < 5:
		rp, gp, bp = x, 0, c
	default:
		rp, gp, bp = c, 0, x
	}
	m := l - c/2
	return color.RGBA{
		R: uint8(math.Round(clamp01(rp+m) * 255)),
		G: uint8(math.Round(clamp01(gp+m) * 255)),
		B: uint8(math.Round(clamp01(bp+m) * 255)),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// RainSchema describes Rain's tunable knobs for the dev UI. Every current
// knob is spawn-config (set per drop at creation). When we add events
// (downpour, splash, calm) they'll gain SlotEvent / SlotEventMod entries.
func RainSchema() EffectSchema {
	return EffectSchema{
		Name: "rain",
		Knobs: []Knob{
			// All tuning knobs are live-applicable — sliding a control affects
			// the running sim. Two flavors, signaled in the description:
			// "rescales in-flight" (affects existing drops immediately, e.g.
			// speed) vs "next drop onward" (applies at spawn time; rain turns
			// over in a few seconds so visible propagation is quick).
			// The old SlotSpawn bucket is empty for Rain — future effects may
			// still use it for genuine once-at-effect-start values.
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -3, Max: 3, Step: 0.1, Default: 0,
				Description: "Slope of the rain: cols sideways per row of descent. 0 = straight down, ±1 = 45°. Next drop onward."},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Per-drop random variation in wind (± this fraction of base). Adds organic scatter. Next drop onward."},
			{Key: "speed", Label: "speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.1, Default: 1.0,
				Description: "Base rows descended per tick. Rescales every in-flight drop proportionally."},
			{Key: "speed_jit", Label: "speed jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Per-drop random variation in speed (± this fraction of base). Next drop onward."},
			{Key: "streak", Label: "streak len", Slot: SlotLever, Group: "shape", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 5,
				Description: "Pixels painted behind each drop's head, tracing a visible streak. Applied at paint time."},
			{Key: "fade", Label: "fade", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.5, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Brightness multiplier per position along a streak. 1.0 = uniform, 0.5 = sharp tail fade. Applied at paint time."},
			{Key: "spawn", Label: "spawn 1/", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 5,
				Description: "Rolls 1 in N per tick for a new drop. Smaller = denser rain."},
			{Key: "burst", Label: "burst max", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 1,
				Description: "Max drops emitted per spawn event. 1 = no clumping; higher = drops in clusters."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 210,
				Description: "Base hue on the color wheel in degrees (0=red, 120=green, 240=blue). Next drop onward."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 180, Step: 1, Default: 0,
				Description: "Per-drop hue variation (± degrees). Larger = more color variety within the rain. Next drop onward."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.6,
				Description: "Color saturation. 0 = grayscale, 1 = fully vivid. Next drop onward."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.55,
				Description: "Minimum lightness for drop colors. Lower = allows darker drops. Next drop onward."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.85,
				Description: "Maximum lightness for drop colors. Higher = allows brighter drops. Next drop onward."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 1,
				Description: "1 = single layer. 2 = adds a dimmer/shorter/slower background layer for parallax depth. Next drop onward."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Fraction of drops assigned to the background layer. Ignored unless layers=2. Next drop onward."},
			{Key: "hue_drift", Label: "hue drift", Slot: SlotLever, Group: "drift", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 0,
				Description: "Amplitude (±degrees) the base hue slowly wanders over ~30s cycles. 0 = static."},
			{Key: "wind_drift", Label: "wind drift", Slot: SlotLever, Group: "drift", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Amplitude the effective wind sways around base. 0 = static; creates gentle direction changes."},

			// DISCRETE EVENTS — per-tick probability of firing.
			{Key: "downpour_p", Label: "downpour", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "downpour",
				Description: "Per-tick probability of starting a downpour (temporary dense rain burst)."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick probability of a calm event — drops stop spawning for a while."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick probability of a wind gust — a sudden sideways push for a stretch of time."},
			{Key: "splash_p", Label: "splash", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.002, Default: 0, Trigger: "splash",
				Description: "Per-tick probability of a splash — an expanding radial ring at a random point."},

			// EVENT MODIFIERS — typical per-event values (each event randomizes ±30%).
			{Key: "downpour_dur", Label: "downpour dur", Slot: SlotEventMod, Group: "downpour", Type: KnobInt, Min: 10, Max: 300, Step: 10, Default: 60,
				Description: "Typical downpour duration in ticks (actual value jitters ±30%)."},
			{Key: "downpour_mult", Label: "downpour ×", Slot: SlotEventMod, Group: "downpour", Type: KnobFloat, Min: 1.5, Max: 10, Step: 0.5, Default: 4,
				Description: "Spawn-rate multiplier during a downpour. 4 = four times denser than baseline."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 300, Step: 10, Default: 50,
				Description: "Typical calm duration in ticks (spawning pauses for this long ±30%)."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 5, Max: 100, Step: 5, Default: 30,
				Description: "Typical gust duration in ticks (how long the wind push lasts ±30%)."},
			{Key: "gust_str", Label: "gust strength", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.1, Default: 1.5,
				Description: "Magnitude of the extra wind added during a gust. Sign is random per event."},
			{Key: "splash_size", Label: "splash size", Slot: SlotEventMod, Group: "splash", Type: KnobInt, Min: 2, Max: 12, Step: 1, Default: 4,
				Description: "Max splash-ring radius in pixels (actual jitters ±30%)."},
		},
	}
}

// splashInstance is an active splash event — a small radial ring that
// expands outward from (row, col) over maxAge ticks and fades as it grows.
type splashInstance struct {
	row, col  int
	age       int
	maxAge    int
	maxRadius int
	color     color.RGBA
}

// LogEntry is one event-occurrence record. Produced when discrete events
// fire (probabilistic or triggered) so the dev UI can show a live log.
type LogEntry struct {
	Tick int    `json:"tick"`
	Type string `json:"type"`
	Desc string `json:"desc"`
}

// Rain is the rain simulation.
// Thread-safety: Step, SetConfig, Trigger*, and DrainLog are all safe to
// call from multiple goroutines. The mutex serializes them. Grid reads for
// rendering happen inside Step's critical section and the resulting snapshot
// is immutable after that.
type Rain struct {
	mu sync.Mutex

	W, H  int
	Grid  [][]Pixel
	drops []drop
	rng   *rngutil.RNG
	cfg   Config

	// lever state
	tick int

	// event state
	downpourTicks int
	downpourMult  float64
	calmTicks     int
	gustTicks     int
	gustWind      float64
	splashes      []splashInstance

	// log ring — most recent events, bounded. DrainLog returns + clears.
	log []LogEntry
}

// NewRain builds a Rain sim. Zero Config gets sensible defaults.
func NewRain(w, h int, seed int64, cfg Config) *Rain {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Rain{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
}

// SetConfig updates the live sim's config without resetting drop/event/tick
// state. Used by dev sessions when the user tweaks a knob — spawn-config
// changes take effect on subsequent drops; lever/event changes apply to the
// running simulation immediately.
//
// Speed is a lever: when it changes, rescale every in-flight drop's velocity
// by the ratio so the slider actually tunes visible rain instead of waiting
// for the old drops to fall off-screen.
func (r *Rain) SetConfig(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newCfg := cfg.withDefaults()
	if r.cfg.Speed > 0 && newCfg.Speed != r.cfg.Speed {
		ratio := newCfg.Speed / r.cfg.Speed
		for i := range r.drops {
			r.drops[i].vRow *= ratio
			r.drops[i].vCol *= ratio
		}
	}
	r.cfg = newCfg
}

// PerturbRNG folds external entropy (e.g. keystroke-derived bytes from
// connected clients) into the sim's RNG without resetting it. Next random
// draw consumes from the perturbed stream — future decisions will differ
// from what they'd have been without the perturbation.
func (r *Rain) PerturbRNG(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Mix(delta)
}

// Resize changes the sim's grid dimensions. Existing drops are dropped
// (re-spawn naturally). Event-timer state is preserved. Safe to call
// concurrently with Step.
func (r *Rain) Resize(w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w == r.W && h == r.H {
		return
	}
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	r.W = w
	r.H = h
	r.Grid = grid
	r.drops = r.drops[:0]
	r.splashes = r.splashes[:0]
}

// State is the subset of Rain state a snapshot exposes to clients so they
// can initialize a matching local replica.
type State struct {
	Tick          int     `json:"tick"`
	DownpourTicks int     `json:"downpourTicks"`
	DownpourMult  float64 `json:"downpourMult"`
	CalmTicks     int     `json:"calmTicks"`
	GustTicks     int     `json:"gustTicks"`
	GustWind      float64 `json:"gustWind"`
}

// PersistedState is the server-side subset of Rain state needed to resume
// the atmosphere after a process restart.
type PersistedState struct {
	State
	RNGState uint64   `json:"rngState"`
	Drops    []Drop   `json:"drops"`
	Splashes []Splash `json:"splashes"`
}

// RGB is the wire form of a color — lowercase keys and no alpha, shared
// across all effect snapshots. Internal sim state uses color.RGBA; this
// type only exists at the JSON boundary.
type RGB struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

// Drop is the wire form of an in-flight raindrop, emitted in snapshots so
// joining clients can replicate mid-simulation instead of starting with an
// empty air column.
type Drop struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	Color      RGB     `json:"color"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	StreakLen  int     `json:"streakLen"`
	Background bool    `json:"background"`
}

// Splash is the wire form of an active splash ring.
type Splash struct {
	Row       int `json:"row"`
	Col       int `json:"col"`
	Age       int `json:"age"`
	MaxAge    int `json:"maxAge"`
	MaxRadius int `json:"maxRadius"`
	Color     RGB `json:"color"`
}

// SnapshotState returns a copy of the event-timer state at this instant so
// a joining client can replicate the atmosphere.
func (r *Rain) SnapshotState() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{
		Tick:          r.tick,
		DownpourTicks: r.downpourTicks,
		DownpourMult:  r.downpourMult,
		CalmTicks:     r.calmTicks,
		GustTicks:     r.gustTicks,
		GustWind:      r.gustWind,
	}
}

// DropsCopy returns the active drops as wire-form Drop values. Caller owns
// the slice.
func (r *Rain) DropsCopy() []Drop {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Drop, len(r.drops))
	for i, d := range r.drops {
		out[i] = Drop{
			Row:        d.Row,
			Col:        d.Col,
			Color:      RGB{R: d.Color.R, G: d.Color.G, B: d.Color.B},
			VRow:       d.vRow,
			VCol:       d.vCol,
			StreakLen:  d.streakLen,
			Background: d.background,
		}
	}
	return out
}

// SplashesCopy returns the active splashes as wire-form Splash values.
func (r *Rain) SplashesCopy() []Splash {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Splash, len(r.splashes))
	for i, s := range r.splashes {
		out[i] = Splash{
			Row:       s.row,
			Col:       s.col,
			Age:       s.age,
			MaxAge:    s.maxAge,
			MaxRadius: s.maxRadius,
			Color:     RGB{R: s.color.R, G: s.color.G, B: s.color.B},
		}
	}
	return out
}

// CurrentTick returns the current sim tick number.
func (r *Rain) CurrentTick() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tick
}

// EffectiveConfig returns the sim's current config with all defaults applied.
// Used by atmospheres to share the effective values with clients via snapshot.
func (r *Rain) EffectiveConfig() Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// RestoreState overwrites the sim's tick + event-timer state from an external
// snapshot (e.g., the first SSE message to a joining client). Does not touch
// config or drops.
func (r *Rain) RestoreState(s State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tick = s.Tick
	r.downpourTicks = s.DownpourTicks
	r.downpourMult = s.DownpourMult
	r.calmTicks = s.CalmTicks
	r.gustTicks = s.GustTicks
	r.gustWind = s.GustWind
}

// SnapshotPersistedState returns the full server-side sim state needed to
// resume from disk, including in-flight particles and RNG state.
func (r *Rain) SnapshotPersistedState() PersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := PersistedState{
		State: State{
			Tick:          r.tick,
			DownpourTicks: r.downpourTicks,
			DownpourMult:  r.downpourMult,
			CalmTicks:     r.calmTicks,
			GustTicks:     r.gustTicks,
			GustWind:      r.gustWind,
		},
		RNGState: r.rng.State(),
		Drops:    make([]Drop, len(r.drops)),
		Splashes: make([]Splash, len(r.splashes)),
	}
	for i, d := range r.drops {
		out.Drops[i] = Drop{
			Row:        d.Row,
			Col:        d.Col,
			Color:      RGB{R: d.Color.R, G: d.Color.G, B: d.Color.B},
			VRow:       d.vRow,
			VCol:       d.vCol,
			StreakLen:  d.streakLen,
			Background: d.background,
		}
	}
	for i, s := range r.splashes {
		out.Splashes[i] = Splash{
			Row:       s.row,
			Col:       s.col,
			Age:       s.age,
			MaxAge:    s.maxAge,
			MaxRadius: s.maxRadius,
			Color:     RGB{R: s.color.R, G: s.color.G, B: s.color.B},
		}
	}
	return out
}

// RestorePersistedState overwrites the sim from a server-side persisted
// snapshot. Config is handled separately via SetConfig before this call.
func (r *Rain) RestorePersistedState(s PersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick = s.Tick
	r.downpourTicks = s.DownpourTicks
	r.downpourMult = s.DownpourMult
	r.calmTicks = s.CalmTicks
	r.gustTicks = s.GustTicks
	r.gustWind = s.GustWind
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}

	r.drops = make([]drop, len(s.Drops))
	for i, d := range s.Drops {
		r.drops[i] = drop{
			Row:        d.Row,
			Col:        d.Col,
			Color:      color.RGBA{R: d.Color.R, G: d.Color.G, B: d.Color.B, A: 255},
			vRow:       d.VRow,
			vCol:       d.VCol,
			streakLen:  d.StreakLen,
			background: d.Background,
		}
	}

	r.splashes = make([]splashInstance, len(s.Splashes))
	for i, sp := range s.Splashes {
		r.splashes[i] = splashInstance{
			row:       sp.Row,
			col:       sp.Col,
			age:       sp.Age,
			maxAge:    sp.MaxAge,
			maxRadius: sp.MaxRadius,
			color:     color.RGBA{R: sp.Color.R, G: sp.Color.G, B: sp.Color.B, A: 255},
		}
	}

	for y := range r.Grid {
		for x := range r.Grid[y] {
			r.Grid[y][x] = Pixel{}
		}
	}
	r.paintSplashes()
	for _, d := range r.drops {
		r.paintDrop(d)
	}
}

// GridCopy returns a snapshot of the current grid. The caller owns the
// returned slice and can read it without holding any sim lock.
func (r *Rain) GridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]Pixel, len(r.Grid))
	for y := range r.Grid {
		out[y] = make([]Pixel, len(r.Grid[y]))
		copy(out[y], r.Grid[y])
	}
	return out
}

// TriggerEvent fires a discrete event immediately, bypassing probability.
// Returns true on recognized event names ("downpour", "calm", "gust", "splash").
func (r *Rain) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "downpour":
		r.downpourTicks = jitterInt(r.rng, r.cfg.DownpourDur, 0.3)
		r.downpourMult = r.cfg.DownpourMult
		r.appendLog("downpour", fmt.Sprintf("triggered (dur=%d, ×%.1f)", r.downpourTicks, r.downpourMult))
	case "calm":
		r.calmTicks = jitterInt(r.rng, r.cfg.CalmDur, 0.3)
		r.appendLog("calm", fmt.Sprintf("triggered (dur=%d)", r.calmTicks))
	case "gust":
		r.gustTicks = jitterInt(r.rng, r.cfg.GustDur, 0.3)
		sign := 1.0
		if r.rng.Float64() < 0.5 {
			sign = -1
		}
		r.gustWind = sign * r.cfg.GustStrength * (0.7 + r.rng.Float64()*0.6)
		r.appendLog("gust", fmt.Sprintf("triggered (dur=%d, wind=%+.2f)", r.gustTicks, r.gustWind))
	case "splash":
		r.spawnSplash()
		r.appendLog("splash", "triggered")
	default:
		return false
	}
	return true
}

// DrainLog returns and clears any log entries accumulated since the last drain.
func (r *Rain) DrainLog() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) == 0 {
		return nil
	}
	out := r.log
	r.log = nil
	return out
}

func (r *Rain) appendLog(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

// Step advances the sim by one tick. Flow:
//  1. Tick bookkeeping; decrement active event timers.
//  2. Roll for new events (downpour, calm, gust, splash) if not already active.
//  3. Clear the grid.
//  4. Paint active splashes.
//  5. Roll for drop spawn (respecting calm / downpour multiplier).
//  6. Advance every active drop; paint its head + streak trail.
//  7. Cull drops whose trail has fully exited the bottom.
//  8. Age/remove expired splashes.
func (r *Rain) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++

	// 1. Decrement active event timers.
	if r.downpourTicks > 0 {
		r.downpourTicks--
	}
	if r.calmTicks > 0 {
		r.calmTicks--
	}
	if r.gustTicks > 0 {
		r.gustTicks--
	} else {
		r.gustWind = 0
	}

	// 2. Roll for new events.
	if r.downpourTicks == 0 && r.rng.Float64() < r.cfg.DownpourChance {
		r.downpourTicks = jitterInt(r.rng, r.cfg.DownpourDur, 0.3)
		r.downpourMult = r.cfg.DownpourMult
		r.appendLog("downpour", fmt.Sprintf("started (dur=%d, ×%.1f)", r.downpourTicks, r.downpourMult))
	}
	if r.calmTicks == 0 && r.rng.Float64() < r.cfg.CalmChance {
		r.calmTicks = jitterInt(r.rng, r.cfg.CalmDur, 0.3)
		r.appendLog("calm", fmt.Sprintf("started (dur=%d)", r.calmTicks))
	}
	if r.gustTicks == 0 && r.rng.Float64() < r.cfg.GustChance {
		r.gustTicks = jitterInt(r.rng, r.cfg.GustDur, 0.3)
		sign := 1.0
		if r.rng.Float64() < 0.5 {
			sign = -1
		}
		r.gustWind = sign * r.cfg.GustStrength * (0.7 + r.rng.Float64()*0.6)
		r.appendLog("gust", fmt.Sprintf("started (dur=%d, wind=%+.2f)", r.gustTicks, r.gustWind))
	}
	if r.rng.Float64() < r.cfg.SplashChance {
		r.spawnSplash()
		r.appendLog("splash", "fired")
	}

	// 3. Clear.
	for y := range r.Grid {
		for x := range r.Grid[y] {
			r.Grid[y][x] = Pixel{}
		}
	}

	// 4. Paint splashes.
	r.paintSplashes()

	// 5. Spawn drops — unless in a calm period. Downpour multiplies spawn rate.
	effectiveSpawn := r.cfg.SpawnEvery
	if r.downpourTicks > 0 && r.downpourMult > 1 {
		effectiveSpawn = int(float64(r.cfg.SpawnEvery) / r.downpourMult)
		if effectiveSpawn < 1 {
			effectiveSpawn = 1
		}
	}
	if r.calmTicks == 0 && r.rng.Intn(effectiveSpawn) == 0 {
		burst := 1
		if r.cfg.SpawnBurst > 1 {
			burst = 1 + r.rng.Intn(r.cfg.SpawnBurst)
		}
		for i := 0; i < burst; i++ {
			r.spawnDrop()
		}
	}

	// 6 + 7. Advance + paint + cull drops.
	alive := r.drops[:0]
	for _, d := range r.drops {
		d.Row += d.vRow
		d.Col += d.vCol
		r.paintDrop(d)
		tailRow := d.Row - float64(d.streakLen-1)*d.vRow
		if tailRow < float64(r.H) && d.Row > -float64(d.streakLen) {
			alive = append(alive, d)
		}
	}
	r.drops = alive

	// 8. Age splashes; drop expired.
	splashesAlive := r.splashes[:0]
	for _, s := range r.splashes {
		s.age++
		if s.age < s.maxAge {
			splashesAlive = append(splashesAlive, s)
		}
	}
	r.splashes = splashesAlive
}

// jitterInt returns an int in [base*(1-spread), base*(1+spread)], uniform.
func jitterInt(rng *rngutil.RNG, base int, spread float64) int {
	f := float64(base) * (1 + spread*(rng.Float64()*2-1))
	n := int(math.Round(f))
	if n < 1 {
		n = 1
	}
	return n
}

// currentHue returns the base hue drifted by the HueDriftAmp lever, using a
// slow sine with fixed period (~30s at 10Hz). 0 amplitude = static.
func (r *Rain) currentHue() float64 {
	base := r.cfg.Hue
	if r.cfg.HueDriftAmp > 0 {
		base += r.cfg.HueDriftAmp * math.Sin(float64(r.tick)*0.02)
	}
	return math.Mod(base+360, 360)
}

// currentWind returns the base wind drifted by WindDriftAmp plus any active
// gust delta.
func (r *Rain) currentWind() float64 {
	w := r.cfg.Wind
	if r.cfg.WindDriftAmp > 0 {
		w += r.cfg.WindDriftAmp * math.Sin(float64(r.tick)*0.013+1.7)
	}
	w += r.gustWind
	return w
}

func (r *Rain) spawnSplash() {
	if r.cfg.SplashSize <= 0 {
		return
	}
	radius := jitterInt(r.rng, r.cfg.SplashSize, 0.3)
	hue := math.Mod(r.currentHue()+(r.rng.Float64()*2-1)*r.cfg.HueSpread+360, 360)
	c := hslToRGB(hue, r.cfg.Saturation, r.cfg.LightnessMax)
	r.splashes = append(r.splashes, splashInstance{
		row:       r.rng.Intn(r.H),
		col:       r.rng.Intn(r.W),
		maxAge:    radius * 2,
		maxRadius: radius,
		color:     c,
	})
}

func (r *Rain) paintSplashes() {
	for _, s := range r.splashes {
		t := float64(s.age) / float64(s.maxAge) // 0..1
		radius := t * float64(s.maxRadius)
		alpha := 1 - t
		c := s.color
		c.R = uint8(float64(c.R) * alpha)
		c.G = uint8(float64(c.G) * alpha)
		c.B = uint8(float64(c.B) * alpha)
		// Plot a ring at the current radius.
		steps := int(2 * math.Pi * radius)
		if steps < 8 {
			steps = 8
		}
		for i := 0; i < steps; i++ {
			theta := 2 * math.Pi * float64(i) / float64(steps)
			gc := s.col + int(math.Round(radius*math.Cos(theta)))
			gr := s.row + int(math.Round(radius*math.Sin(theta)))
			if gr < 0 || gr >= r.H || gc < 0 || gc >= r.W {
				continue
			}
			r.Grid[gr][gc] = Pixel{Filled: true, C: c}
		}
	}
}

// paintDrop lays down StreakLen cells from the drop's head backward along its
// motion vector. Brightness decays by FadeFactor per position from the head.
func (r *Rain) paintDrop(d drop) {
	for i := 0; i < d.streakLen; i++ {
		row := d.Row - float64(i)*d.vRow
		col := d.Col - float64(i)*d.vCol
		gr := int(math.Floor(row))
		gc := int(math.Round(col))
		if gr < 0 || gr >= r.H || gc < 0 || gc >= r.W {
			continue
		}
		brightness := math.Pow(r.cfg.FadeFactor, float64(i))
		c := d.Color
		c.R = uint8(float64(c.R) * brightness)
		c.G = uint8(float64(c.G) * brightness)
		c.B = uint8(float64(c.B) * brightness)
		r.Grid[gr][gc] = Pixel{Filled: true, C: c}
	}
}

// spawnDrop rolls per-drop jitter (speed, wind, hue, lightness) + layer,
// computes the motion vector + color, and appends the drop to the list.
func (r *Rain) spawnDrop() {
	isBG := r.cfg.Layers >= 2 && r.rng.Float64() < r.cfg.LayerBalance

	// Motion jitter. Wind uses currentWind() so lever drift + gust events apply.
	sJit := (r.rng.Float64()*2 - 1) * r.cfg.SpeedJitter
	wJit := (r.rng.Float64()*2 - 1) * r.cfg.WindJitter
	effSpeed := r.cfg.Speed * (1 + sJit)
	effWind := r.currentWind() + wJit*r.cfg.Wind // jitter relative to static base magnitude
	if effSpeed < 0.1 {
		effSpeed = 0.1
	}
	// Background layer moves slower (parallax depth illusion).
	if isBG {
		effSpeed *= 0.6
	}

	// Color: hue base (possibly drifted) + jitter, lightness sampled from [min, max].
	hJit := (r.rng.Float64()*2 - 1) * r.cfg.HueSpread
	hue := math.Mod(r.currentHue()+hJit+360, 360)
	t := r.rng.Float64()
	lightness := r.cfg.LightnessMin + t*(r.cfg.LightnessMax-r.cfg.LightnessMin)
	if isBG {
		lightness *= 0.65
	}
	c := hslToRGB(hue, r.cfg.Saturation, lightness)

	streak := r.cfg.StreakLen
	if isBG {
		streak = streak / 2
		if streak < 2 {
			streak = 2
		}
	}

	col := r.rng.Float64() * float64(r.W)
	r.drops = append(r.drops, drop{
		Row:        0,
		Col:        col,
		Color:      c,
		vRow:       effSpeed,
		vCol:       effWind * effSpeed,
		streakLen:  streak,
		background: isBG,
	})
}
