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
// Filled cells carry their current rendered color; trails emerge because
// Step() decays cell colors each tick until they cross a darkness threshold
// and get unfilled.
type Pixel struct {
	Filled bool
	C      color.RGBA
}

// drop is one active raindrop tracked outside the grid. Each drop lives
// across multiple ticks and marks the cell it passes through as filled with
// its head color; the drop leaves a decaying trail behind it in the grid.
// Row/Col are floats so Wind can take any continuous value.
type drop struct {
	Row, Col float64
	Color    color.RGBA
}

// Config tunes Rain behavior. Zero values fall back to sensible defaults
// via withDefaults(). All knobs are continuous spectrums (no discrete
// presets) so the dev UI can expose them as sliders.
type Config struct {
	// Wind shifts drops sideways per tick on a continuous spectrum. 0 =
	// vertical, positive = right-slanted, negative = left-slanted. Typical
	// slider range is [-2.0, 2.0]; at 1.0 the rain falls at 45°.
	Wind float64
	// SpawnEvery is the RNG denominator: a new drop spawns with probability
	// 1/SpawnEvery per tick. Smaller = denser rain.
	SpawnEvery int
	// FadeFactor dims every filled cell each tick (0.5 = very short trails,
	// 0.9 = long trails). Applied to R/G/B channels.
	FadeFactor float64
	// Hue is the base hue in degrees [0, 360). The drop palette (3 tones)
	// is generated from this hue at NewRain time.
	Hue float64
}

func (c Config) withDefaults() Config {
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 5
	}
	if c.FadeFactor <= 0 {
		c.FadeFactor = 0.65
	}
	// Hue default: cool blue-cyan (~210°). Wrapping handles negatives.
	if c.Hue == 0 {
		c.Hue = 210
	}
	return c
}

// paletteFromHue builds a 3-tone palette (pale, mid, dark) from a base hue.
// The three tones give drops visible brightness variation while keeping the
// whole effect tonally coherent.
func paletteFromHue(hue float64) []color.RGBA {
	h := math.Mod(hue, 360)
	if h < 0 {
		h += 360
	}
	return []color.RGBA{
		hslToRGB(h, 0.60, 0.85), // pale
		hslToRGB(h, 0.60, 0.70), // mid
		hslToRGB(h, 0.65, 0.55), // dark
	}
}

// hslToRGB converts HSL to RGBA (alpha=255). h is in degrees [0, 360);
// s and l are in [0, 1].
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
		R: uint8(math.Round((rp + m) * 255)),
		G: uint8(math.Round((gp + m) * 255)),
		B: uint8(math.Round((bp + m) * 255)),
		A: 255,
	}
}

// Rain is the rain simulation. Drops fall (with optional wind angle) leaving
// decaying-brightness trails behind them. At bottom or off the sides, drops
// are removed.
type Rain struct {
	W, H    int
	Grid    [][]Pixel
	drops   []drop
	rng     *rand.Rand
	cfg     Config
	palette []color.RGBA
}

// NewRain builds a Rain sim with dimensions, a seeded RNG, and a config.
// Pass a zero Config{} for defaults.
func NewRain(w, h int, seed int64, cfg Config) *Rain {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	cfg = cfg.withDefaults()
	return &Rain{
		W:       w,
		H:       h,
		Grid:    grid,
		rng:     rand.New(rand.NewSource(seed)),
		cfg:     cfg,
		palette: paletteFromHue(cfg.Hue),
	}
}

// Step advances the sim by one tick:
//  1. Decay every filled cell by FadeFactor; unfill when near-black.
//  2. With probability 1/SpawnEvery, spawn a new drop at a random column.
//  3. Move every active drop (1 row down, Wind cols sideways).
//     Mark the new cell as fully lit in the drop's color.
//     Drops leaving the grid are removed.
func (r *Rain) Step() {
	// 1. Fade existing cells.
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			p := &r.Grid[y][x]
			if !p.Filled {
				continue
			}
			p.C.R = uint8(float64(p.C.R) * r.cfg.FadeFactor)
			p.C.G = uint8(float64(p.C.G) * r.cfg.FadeFactor)
			p.C.B = uint8(float64(p.C.B) * r.cfg.FadeFactor)
			if p.C.R < 16 && p.C.G < 16 && p.C.B < 16 {
				p.Filled = false
			}
		}
	}

	// 2. Maybe spawn a new drop.
	if r.rng.Intn(r.cfg.SpawnEvery) == 0 {
		col := r.rng.Intn(r.W)
		c := r.palette[r.rng.Intn(len(r.palette))]
		r.drops = append(r.drops, drop{Row: 0, Col: float64(col), Color: c})
		r.Grid[0][col] = Pixel{Filled: true, C: c}
	}

	// 3. Move drops.
	alive := r.drops[:0]
	for _, d := range r.drops {
		d.Row++
		d.Col += r.cfg.Wind
		gr := int(d.Row)
		gc := int(math.Round(d.Col))
		if gr >= r.H || gc < 0 || gc >= r.W {
			continue
		}
		r.Grid[gr][gc] = Pixel{Filled: true, C: d.Color}
		alive = append(alive, d)
	}
	r.drops = alive
}
