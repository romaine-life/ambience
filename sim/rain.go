// Package sim implements the ambient effect simulations.
//
// Each effect is a grid of Pixels evolved by a Step() function. The server
// ticks the current effect and broadcasts snapshots; clients render whatever
// they receive without running the simulation themselves.
package sim

import (
	"image/color"
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
type drop struct {
	Row, Col int
	Color    color.RGBA
}

// Config tunes Rain behavior. Zero values fall back to sensible defaults
// via withDefaults().
type Config struct {
	// WindDir shifts drops sideways per tick: -1 left, 0 vertical, +1 right.
	WindDir int
	// SpawnEvery is the RNG denominator: a new drop spawns with probability
	// 1/SpawnEvery per tick. Smaller = denser rain.
	SpawnEvery int
	// FadeFactor dims every filled cell each tick (0.5 = very short trails,
	// 0.9 = long trails). Applied to R/G/B channels.
	FadeFactor float64
	// Palette is the pool of drop colors. At spawn time a random entry is
	// picked for each drop.
	Palette []color.RGBA
}

var defaultPalette = []color.RGBA{
	{180, 220, 255, 255},
	{140, 190, 240, 255},
	{100, 160, 220, 255},
}

func (c Config) withDefaults() Config {
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 5
	}
	if c.FadeFactor <= 0 {
		c.FadeFactor = 0.65
	}
	if len(c.Palette) == 0 {
		c.Palette = defaultPalette
	}
	return c
}

// Rain is the rain simulation. Drops fall (with optional wind angle) leaving
// decaying-brightness trails behind them. At bottom or off the sides, drops
// are removed.
type Rain struct {
	W, H  int
	Grid  [][]Pixel
	drops []drop
	rng   *rand.Rand
	cfg   Config
}

// NewRain builds a Rain sim with dimensions, a seeded RNG, and a config.
// Pass a zero Config{} for defaults.
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
//  1. Decay every filled cell by FadeFactor; unfill when near-black.
//  2. With probability 1/SpawnEvery, spawn a new drop at a random column.
//  3. Move every active drop (1 row down, WindDir cols sideways).
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
		c := r.cfg.Palette[r.rng.Intn(len(r.cfg.Palette))]
		r.drops = append(r.drops, drop{Row: 0, Col: col, Color: c})
		r.Grid[0][col] = Pixel{Filled: true, C: c}
	}

	// 3. Move drops.
	alive := r.drops[:0]
	for _, d := range r.drops {
		d.Row++
		d.Col += r.cfg.WindDir
		if d.Row >= r.H || d.Col < 0 || d.Col >= r.W {
			continue
		}
		r.Grid[d.Row][d.Col] = Pixel{Filled: true, C: d.Color}
		alive = append(alive, d)
	}
	r.drops = alive
}
