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
