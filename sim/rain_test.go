package sim

import (
	"image/color"
	"testing"
)

func TestStepFadesFilledCell(t *testing.T) {
	r := NewRain(10, 10, 1, Config{FadeFactor: 0.5})
	r.Grid[5][5] = Pixel{Filled: true, C: color.RGBA{200, 200, 200, 255}}

	r.Step()

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
	if r.cfg.Hue == 0 {
		t.Errorf("expected default Hue to be set")
	}
	if len(r.palette) == 0 {
		t.Errorf("expected palette generated from hue")
	}
}

func TestPaletteFromHueVariesByHue(t *testing.T) {
	redish := paletteFromHue(0)
	blueish := paletteFromHue(210)
	if redish[0] == blueish[0] {
		t.Errorf("different hues should produce different palettes, got identical pale tone")
	}
}

func TestHslToRGBBasicAnchors(t *testing.T) {
	cases := []struct {
		h, s, l      float64
		wantR, wantG, wantB uint8
	}{
		{0, 1, 0.5, 255, 0, 0},     // pure red
		{120, 1, 0.5, 0, 255, 0},   // pure green
		{240, 1, 0.5, 0, 0, 255},   // pure blue
		{0, 0, 0, 0, 0, 0},         // black
		{0, 0, 1, 255, 255, 255},   // white
	}
	for _, tc := range cases {
		got := hslToRGB(tc.h, tc.s, tc.l)
		if got.R != tc.wantR || got.G != tc.wantG || got.B != tc.wantB {
			t.Errorf("hslToRGB(%v, %v, %v) = %v; want R=%d G=%d B=%d", tc.h, tc.s, tc.l, got, tc.wantR, tc.wantG, tc.wantB)
		}
	}
}
