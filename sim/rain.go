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
type Pixel struct {
	Filled bool
	C      color.RGBA
}

// Rain is the MVP effect: single-pixel-wide streaks fall from the top and
// exit the bottom. No diagonals, no accumulation — the simplest thing that
// proves the end-to-end shared-world pipe works.
type Rain struct {
	W, H int
	Grid [][]Pixel
	rng  *rand.Rand
}

// NewRain builds a Rain sim with the given grid dimensions and RNG seed.
// The RNG is used for spawn probability and spawn column; callers can feed
// entropy into it later via Reseed.
func NewRain(w, h int, seed int64) *Rain {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Rain{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rand.New(rand.NewSource(seed)),
	}
}

// Step advances the sim by one tick: may spawn a new streak at the top row,
// then falls every filled cell one row. Cells at the bottom row vanish.
func (r *Rain) Step() {
	if r.rng.Intn(10) == 0 {
		col := r.rng.Intn(r.W)
		if !r.Grid[0][col].Filled {
			r.Grid[0][col] = Pixel{
				Filled: true,
				C:      color.RGBA{180, 220, 255, 255},
			}
		}
	}
	for row := r.H - 1; row >= 0; row-- {
		for c := 0; c < r.W; c++ {
			if !r.Grid[row][c].Filled {
				continue
			}
			if row == r.H-1 {
				r.Grid[row][c] = Pixel{}
				continue
			}
			if !r.Grid[row+1][c].Filled {
				r.Grid[row+1][c] = r.Grid[row][c]
				r.Grid[row][c] = Pixel{}
			}
		}
	}
}
