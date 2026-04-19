package sim

import (
	"image/color"
	"testing"
)

func TestStepFadesFilledCell(t *testing.T) {
	r := NewRain(10, 10, 1, Config{FadeFactor: 0.5})
	r.Grid[5][5] = Pixel{Filled: true, C: color.RGBA{200, 200, 200, 255}}

	r.Step()

	// Cell should still be filled but dimmer.
	if !r.Grid[5][5].Filled {
		t.Errorf("expected (5,5) still filled after 1 step")
	}
	if r.Grid[5][5].C.R >= 200 {
		t.Errorf("expected fade, got R=%d", r.Grid[5][5].C.R)
	}
}

func TestStepEventuallyDarkensFilledCellToEmpty(t *testing.T) {
	r := NewRain(10, 10, 1, Config{FadeFactor: 0.5})
	r.Grid[5][5] = Pixel{Filled: true, C: color.RGBA{200, 200, 200, 255}}

	// 10 fade steps at 0.5 should take 200 → ~0.2 → cleared.
	for i := 0; i < 10; i++ {
		r.Step()
	}

	if r.Grid[5][5].Filled {
		t.Errorf("expected (5,5) empty after repeated fade, got filled with %v", r.Grid[5][5].C)
	}
}

func TestNewRainAppliesDefaults(t *testing.T) {
	r := NewRain(10, 10, 1, Config{})
	if r.cfg.SpawnEvery == 0 {
		t.Errorf("expected default SpawnEvery to be set")
	}
	if r.cfg.FadeFactor == 0 {
		t.Errorf("expected default FadeFactor to be set")
	}
	if len(r.cfg.Palette) == 0 {
		t.Errorf("expected default palette to be set")
	}
}
