// Package sim implements the ambient effect simulations.
//
// Each effect is a grid of Pixels evolved by a Step() function. The server
// ticks the current effect and broadcasts snapshots; clients render whatever
// they receive without running the simulation themselves.
package sim

import (
	"image/color"
	"math"
	"math/rand"
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

	// Wind is slope (cols per row of descent). 0 = vertical, ±1 = 45°.
	Wind float64
	// WindJitter is the fractional per-drop variation in Wind. 0.25 means each
	// drop's effective Wind is Wind × (1 ± up-to-25%). Adds organic scatter.
	WindJitter float64
	// Speed is base rows descended per tick.
	Speed float64
	// SpeedJitter is per-drop fractional variation in Speed (same scheme).
	SpeedJitter float64

	// SHAPE

	// StreakLen is the pixel length of each drop's trail (head included).
	// 1 = single-pixel dots; higher = longer visible streaks.
	StreakLen int
	// FadeFactor is the brightness multiplier per position back from the head
	// along a streak. 1.0 = uniformly lit streak; 0.5 = steep tail fade.
	FadeFactor float64

	// SPAWN

	// SpawnEvery is the RNG denominator: spawn rolls 1 in SpawnEvery per tick.
	SpawnEvery int
	// SpawnBurst is the max drops emitted per spawn event. A single roll fires
	// a burst of 1..SpawnBurst drops (uniform random). 1 = no clumping.
	SpawnBurst int

	// COLOR

	// Hue is the base hue in degrees [0, 360).
	Hue float64
	// HueSpread is ± degrees each drop can deviate from Hue.
	HueSpread float64
	// Saturation of generated palette tones (0..1).
	Saturation float64
	// LightnessMin / LightnessMax define the lightness range drops are drawn
	// from. Each drop picks a random lightness in [min, max]. Larger spread =
	// more tonal variety within the rain.
	LightnessMin float64
	LightnessMax float64

	// DEPTH

	// Layers is 1 or 2. With 2, a background layer of dimmer/shorter/slower
	// drops is spawned alongside the foreground.
	Layers int
	// LayerBalance is the fraction of new drops that go into the background
	// layer (0..1). Ignored when Layers < 2.
	LayerBalance float64

	// LEVERS (continuous drift)

	// HueDriftAmp is the amplitude (±degrees) the base hue wanders over time
	// around its static Hue value. 0 = no drift.
	HueDriftAmp float64
	// WindDriftAmp is the amplitude the effective base wind wanders around
	// Wind (in cols/row). Creates gentle sway.
	WindDriftAmp float64

	// EVENTS (per-tick probability)

	// DownpourChance is the per-tick probability of a downpour event firing
	// (while one isn't already active). 0.001 = ~once per 10s at 10Hz.
	DownpourChance float64
	// CalmChance is the per-tick probability of a calm event (spawn pause).
	CalmChance float64
	// GustChance is the per-tick probability of a wind gust.
	GustChance float64
	// SplashChance is the per-tick probability of a splash event.
	SplashChance float64

	// EVENT MODIFIERS (typical per-event values; each event randomizes ±30%)

	// DownpourDur is the typical downpour duration in ticks.
	DownpourDur int
	// DownpourMult is the spawn-rate multiplier during a downpour.
	DownpourMult float64
	// CalmDur is the typical calm duration in ticks (no drops spawn).
	CalmDur int
	// GustDur is the typical gust duration in ticks.
	GustDur int
	// GustStrength is the magnitude of the wind delta added during a gust
	// (sign randomized per event).
	GustStrength float64
	// SplashSize is the typical splash radius in pixels.
	SplashSize int
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
			// motion
			{Key: "wind", Label: "wind", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: -3, Max: 3, Step: 0.1, Default: 0},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0},
			{Key: "speed", Label: "speed", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.1, Default: 1.0},
			{Key: "speed_jit", Label: "speed jitter", Slot: SlotSpawn, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0},
			// shape
			{Key: "streak", Label: "streak len", Slot: SlotSpawn, Group: "shape", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 5},
			{Key: "fade", Label: "fade", Slot: SlotSpawn, Group: "shape", Type: KnobFloat, Min: 0.5, Max: 1, Step: 0.01, Default: 0.88},
			// spawn rate
			{Key: "spawn", Label: "spawn 1/", Slot: SlotSpawn, Group: "density", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 5},
			{Key: "burst", Label: "burst max", Slot: SlotSpawn, Group: "density", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 1},
			// color
			{Key: "hue", Label: "hue", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 210},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 180, Step: 1, Default: 0},
			{Key: "sat", Label: "saturation", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.6},
			{Key: "lmin", Label: "light min", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.55},
			{Key: "lmax", Label: "light max", Slot: SlotSpawn, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.85},
			// depth
			{Key: "layers", Label: "layers", Slot: SlotSpawn, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 1},
			{Key: "lbal", Label: "bg balance", Slot: SlotSpawn, Group: "depth", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.4},

			// CONTINUOUS LEVERS — slow drift over time.
			{Key: "hue_drift", Label: "hue drift", Slot: SlotLever, Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 0},
			{Key: "wind_drift", Label: "wind drift", Slot: SlotLever, Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0},

			// DISCRETE EVENTS — per-tick probability of firing.
			{Key: "downpour_p", Label: "downpour", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0},
			{Key: "splash_p", Label: "splash", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.002, Default: 0},

			// EVENT MODIFIERS — typical per-event values (each event randomizes ±30%).
			{Key: "downpour_dur", Label: "downpour dur", Slot: SlotEventMod, Group: "downpour", Type: KnobInt, Min: 10, Max: 300, Step: 10, Default: 60},
			{Key: "downpour_mult", Label: "downpour ×", Slot: SlotEventMod, Group: "downpour", Type: KnobFloat, Min: 1.5, Max: 10, Step: 0.5, Default: 4},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 300, Step: 10, Default: 50},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 5, Max: 100, Step: 5, Default: 30},
			{Key: "gust_str", Label: "gust strength", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.1, Default: 1.5},
			{Key: "splash_size", Label: "splash size", Slot: SlotEventMod, Group: "splash", Type: KnobInt, Min: 2, Max: 12, Step: 1, Default: 4},
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

// Rain is the rain simulation.
type Rain struct {
	W, H  int
	Grid  [][]Pixel
	drops []drop
	rng   *rand.Rand
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
		rng:  rand.New(rand.NewSource(seed)),
		cfg:  cfg.withDefaults(),
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
	}
	if r.calmTicks == 0 && r.rng.Float64() < r.cfg.CalmChance {
		r.calmTicks = jitterInt(r.rng, r.cfg.CalmDur, 0.3)
	}
	if r.gustTicks == 0 && r.rng.Float64() < r.cfg.GustChance {
		r.gustTicks = jitterInt(r.rng, r.cfg.GustDur, 0.3)
		sign := 1.0
		if r.rng.Float64() < 0.5 {
			sign = -1
		}
		r.gustWind = sign * r.cfg.GustStrength * (0.7 + r.rng.Float64()*0.6)
	}
	if r.rng.Float64() < r.cfg.SplashChance {
		r.spawnSplash()
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
func jitterInt(rng *rand.Rand, base int, spread float64) int {
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
