package sim

import "testing"

func TestProceduralGridCopiesAreVisible(t *testing.T) {
	tests := []struct {
		name string
		grid func() [][]Pixel
	}{
		{name: "aurora", grid: func() [][]Pixel {
			s := NewAurora(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "autumn-leaves", grid: func() [][]Pixel {
			s := NewAutumnLeaves(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "beach", grid: func() [][]Pixel {
			s := NewBeach(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "burning-trees", grid: func() [][]Pixel {
			s := NewBurningTrees(160, 80, 1, BurningTreesConfig{})
			s.Step()
			return s.GridCopy()
		}},
		{name: "campfire", grid: func() [][]Pixel {
			s := NewCampfire(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "lighthouse", grid: func() [][]Pixel {
			s := NewLighthouse(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "mysterious-man", grid: func() [][]Pixel {
			s := NewMysteriousMan(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "rowboat", grid: func() [][]Pixel {
			s := NewRowboat(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "snow", grid: func() [][]Pixel {
			s := NewSnow(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "starfield", grid: func() [][]Pixel {
			s := NewStarfield(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "train", grid: func() [][]Pixel {
			s := NewTrain(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "underwater", grid: func() [][]Pixel {
			s := NewUnderwater(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "volcano", grid: func() [][]Pixel {
			s := NewVolcano(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "wheat-field", grid: func() [][]Pixel {
			s := NewWheatField(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
		{name: "windmill", grid: func() [][]Pixel {
			s := NewWindmill(160, 80, 1, nil)
			s.Step()
			return s.GridCopy()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countFilled(tt.grid()); got < 1000 {
				t.Fatalf("filled pixels = %d, want visible procedural frame", got)
			}
		})
	}
}

func countFilled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled {
				count++
			}
		}
	}
	return count
}

func TestRestoredProceduralRenderersHaveEffectStructure(t *testing.T) {
	tests := []struct {
		name string
		grid func() [][]Pixel
		want func(*testing.T, [][]Pixel)
	}{
		{name: "aurora", grid: steppedAuroraGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "upper green bands", countRegion(g, 0, 0, gridWOf(g), gridHOf(g)/2, isGreenishBright), 400)
			assertAtLeast(t, "dark sky", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isVeryDark), 1000)
		}},
		{name: "autumn-leaves", grid: steppedAutumnLeavesGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "warm leaves", countRegion(g, 0, 0, gridWOf(g), gridHOf(g)*3/4, isWarmLeaf), 120)
			assertAtLeast(t, "ground band", countRegion(g, 0, gridHOf(g)*3/4, gridWOf(g), gridHOf(g), isEarthy), 1000)
		}},
		{name: "beach", grid: steppedBeachGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "warm sky", countRegion(g, 0, 0, gridWOf(g), gridHOf(g)/3, isWarmSky), 1000)
			assertAtLeast(t, "middle ocean", countRegion(g, 0, gridHOf(g)/3, gridWOf(g), gridHOf(g)*2/3, isBlue), 1200)
			assertAtLeast(t, "lower sand", countRegion(g, 0, gridHOf(g)*2/3, gridWOf(g), gridHOf(g), isSand), 1200)
			assertAtLeast(t, "foam line", countRegion(g, 0, gridHOf(g)/2, gridWOf(g), gridHOf(g)*3/4, isBright), 80)
		}},
		{name: "campfire", grid: steppedCampfireGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "central flame", countRegion(g, gridWOf(g)/2-18, gridHOf(g)*2/3, gridWOf(g)/2+18, gridHOf(g), isFire), 30)
			assertAtLeast(t, "dark surround", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isVeryDark), 1000)
		}},
		{name: "lighthouse", grid: steppedLighthouseGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "sweeping beam", countRegion(g, gridWOf(g)/5, 0, gridWOf(g), gridHOf(g)*2/3, isBeam), 120)
			assertAtLeast(t, "tower", countRegion(g, gridWOf(g)/10, gridHOf(g)/2, gridWOf(g)/4, gridHOf(g), isTower), 80)
		}},
		{name: "mysterious-man", grid: steppedMysteriousManGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "cigarette ember", countRegion(g, gridWOf(g)/2, gridHOf(g)/2, gridWOf(g)*2/3, gridHOf(g)*3/4, isRedEye), 2)
			assertAtLeast(t, "black figure", countRegion(g, gridWOf(g)/2-20, gridHOf(g)/3, gridWOf(g)/2+20, gridHOf(g)*4/5, isAlmostBlack), 200)
		}},
		{name: "rowboat", grid: steppedRowboatGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "brown boat", countRegion(g, gridWOf(g)/4, gridHOf(g)/2, gridWOf(g)/2, gridHOf(g)*2/3, isBoatBrown), 20)
			assertAtLeast(t, "water field", countRegion(g, 0, gridHOf(g)/2, gridWOf(g), gridHOf(g), isBlue), 1200)
		}},
		{name: "snow", grid: steppedSnowGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "snow ground", countRegion(g, 0, gridHOf(g)*4/5, gridWOf(g), gridHOf(g), isSnowGround), 1000)
			assertAtLeast(t, "dark trees", countRegion(g, 0, gridHOf(g)/2, gridWOf(g), gridHOf(g)*4/5, isVeryDarkBlue), 100)
		}},
		{name: "starfield", grid: steppedStarfieldGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "stars", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isStar), 35)
			assertAtMost(t, "stars are sparse", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isStar), 1200)
			assertAtLeast(t, "dark space", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isVeryDark), 4000)
		}},
		{name: "train", grid: steppedTrainGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "train body", countRegion(g, 0, gridHOf(g)/2, gridWOf(g), gridHOf(g)*3/4, isTrainBody), 80)
			assertAtLeast(t, "track", countRegion(g, 0, gridHOf(g)*2/3-4, gridWOf(g), gridHOf(g)*2/3+8, isTrack), 20)
		}},
		{name: "underwater", grid: steppedUnderwaterGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "seaweed", countRegion(g, 0, gridHOf(g)*2/3, gridWOf(g), gridHOf(g), isSeaweed), 80)
			assertAtLeast(t, "bubbles", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isBubble), 40)
			assertAtLeast(t, "water body", countRegion(g, 0, 0, gridWOf(g), gridHOf(g), isBlue), 2000)
		}},
		{name: "volcano", grid: steppedVolcanoGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "lava", countRegion(g, gridWOf(g)/3, gridHOf(g)/4, gridWOf(g)*2/3, gridHOf(g), isFire), 120)
			assertAtLeast(t, "dark cone", countRegion(g, gridWOf(g)/4, gridHOf(g)/3, gridWOf(g)*3/4, gridHOf(g)*5/6, isRock), 600)
		}},
		{name: "wheat-field", grid: steppedWheatFieldGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "wheat stalks", countRegion(g, 0, gridHOf(g)/2, gridWOf(g), gridHOf(g), isWheat), 1000)
			assertAtLeast(t, "sky", countRegion(g, 0, 0, gridWOf(g), gridHOf(g)/3, isSky), 1000)
		}},
		{name: "windmill", grid: steppedWindmillGrid, want: func(t *testing.T, g [][]Pixel) {
			assertAtLeast(t, "dark blades", countRegion(g, gridWOf(g)/3, gridHOf(g)/4, gridWOf(g)*3/4, gridHOf(g)*2/3, isBlade), 60)
			assertAtLeast(t, "dark hill", countRegion(g, 0, gridHOf(g)*2/3, gridWOf(g), gridHOf(g), isGroundGreen), 1000)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want(t, tt.grid())
		})
	}
}

func steppedAuroraGrid() [][]Pixel {
	s := NewAurora(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedAutumnLeavesGrid() [][]Pixel {
	s := NewAutumnLeaves(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedBeachGrid() [][]Pixel {
	s := NewBeach(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedCampfireGrid() [][]Pixel {
	s := NewCampfire(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedLighthouseGrid() [][]Pixel {
	s := NewLighthouse(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedMysteriousManGrid() [][]Pixel {
	s := NewMysteriousMan(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedRowboatGrid() [][]Pixel {
	s := NewRowboat(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedSnowGrid() [][]Pixel {
	s := NewSnow(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedStarfieldGrid() [][]Pixel {
	s := NewStarfield(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedTrainGrid() [][]Pixel {
	s := NewTrain(160, 80, 1, nil)
	s.TriggerEvent("pass")
	for i := 0; i < 80; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedUnderwaterGrid() [][]Pixel {
	s := NewUnderwater(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedVolcanoGrid() [][]Pixel {
	s := NewVolcano(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedWheatFieldGrid() [][]Pixel {
	s := NewWheatField(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func steppedWindmillGrid() [][]Pixel {
	s := NewWindmill(160, 80, 1, nil)
	for i := 0; i < 20; i++ {
		s.Step()
	}
	return s.GridCopy()
}

func gridWOf(grid [][]Pixel) int {
	if len(grid) == 0 {
		return 0
	}
	return len(grid[0])
}

func gridHOf(grid [][]Pixel) int { return len(grid) }

func countRegion(grid [][]Pixel, x0, y0, x1, y1 int, pred func(Pixel) bool) int {
	count := 0
	for y := max(0, y0); y < min(len(grid), y1); y++ {
		for x := max(0, x0); x < min(len(grid[y]), x1); x++ {
			if pred(grid[y][x]) {
				count++
			}
		}
	}
	return count
}

func assertAtLeast(t *testing.T, label string, got, want int) {
	t.Helper()
	if got < want {
		t.Fatalf("%s = %d, want >= %d", label, got, want)
	}
}

func assertAtMost(t *testing.T, label string, got, want int) {
	t.Helper()
	if got > want {
		t.Fatalf("%s = %d, want <= %d", label, got, want)
	}
}

func brightness(p Pixel) int {
	if !p.Filled {
		return 0
	}
	return (int(p.C.R) + int(p.C.G) + int(p.C.B)) / 3
}

func isVeryDark(p Pixel) bool    { return p.Filled && brightness(p) < 32 }
func isAlmostBlack(p Pixel) bool { return p.Filled && brightness(p) < 18 }
func isBright(p Pixel) bool      { return p.Filled && brightness(p) > 150 }
func isStar(p Pixel) bool        { return p.Filled && brightness(p) > 155 }
func isGreenishBright(p Pixel) bool {
	return p.Filled && p.C.G > p.C.R+20 && p.C.G > p.C.B-10 && brightness(p) > 70
}
func isWarmLeaf(p Pixel) bool { return p.Filled && p.C.R > p.C.G && p.C.G > p.C.B && p.C.R > 95 }
func isEarthy(p Pixel) bool   { return p.Filled && p.C.R > 35 && p.C.G > 25 && p.C.B < 45 }
func isSand(p Pixel) bool {
	return p.Filled && p.C.R > 120 && p.C.G > 90 && p.C.B < 180 && p.C.R >= p.C.B+20 && p.C.G >= p.C.B+8
}
func isWarmSky(p Pixel) bool {
	return p.Filled && p.C.R > p.C.B && p.C.G > p.C.B && brightness(p) > 110
}
func isBlue(p Pixel) bool   { return p.Filled && p.C.B > p.C.R+8 && p.C.G >= p.C.R }
func isFire(p Pixel) bool   { return p.Filled && p.C.R > 120 && p.C.G > 35 && p.C.B < 150 }
func isBeam(p Pixel) bool   { return p.Filled && brightness(p) > 50 }
func isTower(p Pixel) bool  { return p.Filled && brightness(p) < 45 && p.C.B >= p.C.R }
func isRedEye(p Pixel) bool { return p.Filled && p.C.R > 120 && p.C.R >= p.C.B+20 && p.C.G >= p.C.B }
func isBoatBrown(p Pixel) bool {
	return p.Filled && p.C.R > 70 && p.C.G > 35 && p.C.G < 90 && p.C.B < 60
}
func isSnowGround(p Pixel) bool {
	return p.Filled && p.C.B >= p.C.R && brightness(p) > 30 && brightness(p) < 110
}
func isVeryDarkBlue(p Pixel) bool { return p.Filled && p.C.B >= p.C.R && brightness(p) < 45 }
func isTrack(p Pixel) bool        { return p.Filled && p.C.R > 65 && p.C.G > 45 && p.C.B < 70 }
func isTrainBody(p Pixel) bool {
	return p.Filled && brightness(p) < 70 && p.C.B >= p.C.R
}
func isSeaweed(p Pixel) bool {
	return p.Filled && p.C.G > p.C.R+4 && p.C.G >= p.C.B-8 && brightness(p) < 110
}
func isBubble(p Pixel) bool { return p.Filled && p.C.B > 110 && p.C.G > 90 && brightness(p) > 90 }
func isRock(p Pixel) bool {
	return p.Filled && p.C.R > 35 && p.C.R < 90 && p.C.G > 25 && p.C.G < 70 && p.C.B > 20 && p.C.B < 70
}
func isWheat(p Pixel) bool       { return p.Filled && p.C.R > 80 && p.C.G > 65 && p.C.B < 70 }
func isSky(p Pixel) bool         { return p.Filled && p.C.B > p.C.R && p.C.G > p.C.R }
func isBlade(p Pixel) bool       { return p.Filled && brightness(p) < 55 && p.C.B >= p.C.R }
func isGroundGreen(p Pixel) bool { return p.Filled && brightness(p) < 45 && p.C.B >= p.C.R }
