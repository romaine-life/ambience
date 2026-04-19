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

// Rain is the rain simulation.
type Rain struct {
	W, H  int
	Grid  [][]Pixel
	drops []drop
	rng   *rand.Rand
	cfg   Config
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

// Step advances the sim by one tick:
//  1. Clear the grid (drops repaint their trails each tick; no residual state).
//  2. Roll for spawn; if hit, emit 1..SpawnBurst new drops.
//  3. Advance every active drop; paint its head + streak trail into the grid.
//  4. Drop drops whose trail has fully exited the bottom.
func (r *Rain) Step() {
	// 1. Clear.
	for y := range r.Grid {
		for x := range r.Grid[y] {
			r.Grid[y][x] = Pixel{}
		}
	}

	// 2. Spawn.
	if r.rng.Intn(r.cfg.SpawnEvery) == 0 {
		burst := 1
		if r.cfg.SpawnBurst > 1 {
			burst = 1 + r.rng.Intn(r.cfg.SpawnBurst)
		}
		for i := 0; i < burst; i++ {
			r.spawnDrop()
		}
	}

	// 3 + 4. Advance + paint + cull.
	alive := r.drops[:0]
	for _, d := range r.drops {
		d.Row += d.vRow
		d.Col += d.vCol
		r.paintDrop(d)
		// Keep alive until the tail has cleared the bottom.
		tailRow := d.Row - float64(d.streakLen-1)*d.vRow
		if tailRow < float64(r.H) && d.Row > -float64(d.streakLen) {
			alive = append(alive, d)
		}
	}
	r.drops = alive
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

	// Motion jitter.
	sJit := (r.rng.Float64()*2 - 1) * r.cfg.SpeedJitter
	wJit := (r.rng.Float64()*2 - 1) * r.cfg.WindJitter
	effSpeed := r.cfg.Speed * (1 + sJit)
	effWind := r.cfg.Wind * (1 + wJit)
	if effSpeed < 0.1 {
		effSpeed = 0.1
	}
	// Background layer moves slower (parallax depth illusion).
	if isBG {
		effSpeed *= 0.6
	}

	// Color: hue base + jitter, lightness sampled from [min, max], saturation from cfg.
	hJit := (r.rng.Float64()*2 - 1) * r.cfg.HueSpread
	hue := math.Mod(r.cfg.Hue+hJit+360, 360)
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
