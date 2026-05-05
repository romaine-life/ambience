package sim

import (
	"image/color"
	"math"
)

func copyPixelGrid(src [][]Pixel) [][]Pixel {
	out := make([][]Pixel, len(src))
	for y := range src {
		out[y] = make([]Pixel, len(src[y]))
		copy(out[y], src[y])
	}
	return out
}

func (a *Aurora) GridCopy() [][]Pixel {
	a.mu.Lock()
	defer a.mu.Unlock()
	return copyPixelGrid(a.Grid)
}

func (a *AutumnLeaves) GridCopy() [][]Pixel {
	a.mu.Lock()
	defer a.mu.Unlock()
	return copyPixelGrid(a.Grid)
}

func (b *Beach) GridCopy() [][]Pixel {
	b.mu.Lock()
	defer b.mu.Unlock()
	return copyPixelGrid(b.Grid)
}

func (c *Campfire) GridCopy() [][]Pixel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return copyPixelGrid(c.Grid)
}

func (d *Dust) GridCopy() [][]Pixel {
	d.mu.Lock()
	defer d.mu.Unlock()
	return copyPixelGrid(d.Grid)
}

func (f *Fireflies) GridCopy() [][]Pixel {
	f.mu.Lock()
	defer f.mu.Unlock()
	return copyPixelGrid(f.Grid)
}

func (l *Lighthouse) GridCopy() [][]Pixel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return copyPixelGrid(l.Grid)
}

func (m *MysteriousMan) GridCopy() [][]Pixel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copyPixelGrid(m.Grid)
}

func (r *Rowboat) GridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyPixelGrid(r.Grid)
}

func (s *Sand) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyPixelGrid(s.Grid)
}

func (s *Snow) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyPixelGrid(s.Grid)
}

func (s *Starfield) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyPixelGrid(s.Grid)
}

func (t *Train) GridCopy() [][]Pixel {
	t.mu.Lock()
	defer t.mu.Unlock()
	return copyPixelGrid(t.Grid)
}

func (u *Underwater) GridCopy() [][]Pixel {
	u.mu.Lock()
	defer u.mu.Unlock()
	return copyPixelGrid(u.Grid)
}

func (v *Volcano) GridCopy() [][]Pixel {
	v.mu.Lock()
	defer v.mu.Unlock()
	return copyPixelGrid(v.Grid)
}

func (p *WaterPipe) GridCopy() [][]Pixel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return copyPixelGrid(p.Grid)
}

func (w *Waterfall) GridCopy() [][]Pixel {
	w.mu.Lock()
	defer w.mu.Unlock()
	return copyPixelGrid(w.Grid)
}

func (w *WheatField) GridCopy() [][]Pixel {
	w.mu.Lock()
	defer w.mu.Unlock()
	return copyPixelGrid(w.Grid)
}

func (w *Windmill) GridCopy() [][]Pixel {
	w.mu.Lock()
	defer w.mu.Unlock()
	return copyPixelGrid(w.Grid)
}

func (b *BurningTrees) GridCopy() [][]Pixel {
	b.mu.Lock()
	defer b.mu.Unlock()
	grid := make([][]Pixel, b.H)
	for y := range grid {
		grid[y] = make([]Pixel, b.W)
	}
	if b.W <= 0 || b.H <= 0 || len(b.states) == 0 {
		return grid
	}
	cfg := b.cfg
	base := int(math.Round(float64(b.H-1) * cfg.Baseline))
	if base < 1 {
		base = b.H - 1
	}
	treeStep := float64(b.W) / float64(len(b.states)+1)
	for i, state := range b.states {
		cx := int(math.Round(float64(i+1) * treeStep))
		h := int(math.Round(float64(b.H) * (cfg.TreeMinH + (cfg.TreeMaxH-cfg.TreeMinH)*0.55)))
		if h < 3 {
			h = 3
		}
		half := int(math.Round(cfg.TreeWidth * float64(b.W) / 160))
		if half < 2 {
			half = 2
		}
		trunk := h / 3
		canopyTop := base - h
		trunkTop := base - trunk
		for y := trunkTop; y <= base && y >= 0 && y < b.H; y++ {
			paintPixel(grid, cx, y, color.RGBA{96, 68, 38, 255})
		}
		for y := canopyTop; y <= trunkTop && y >= 0 && y < b.H; y++ {
			width := half - int(math.Abs(float64(y-(canopyTop+trunkTop)/2))*float64(half)/math.Max(1, float64(trunkTop-canopyTop)))
			if width < 1 {
				width = 1
			}
			for x := cx - width; x <= cx+width; x++ {
				switch state {
				case BTreeStateAlive:
					paintPixel(grid, x, y, hslToRGB(cfg.CanopyHue, cfg.Sat, cfg.LightMin+(cfg.LightMax-cfg.LightMin)*0.45))
				case BTreeStateIgniting, BTreeStateBurning:
					paintPixel(grid, x, y, hslToRGB(cfg.FlameHue+float64((x+y)%5)*cfg.HueSpread/5, cfg.Sat, cfg.LightMax))
				case BTreeStateAshing, BTreeStateAsh:
					paintPixel(grid, x, y, color.RGBA{74, 66, 58, 255})
				}
			}
		}
	}
	return grid
}

func (t *Tetris) GridCopy() [][]Pixel {
	t.mu.Lock()
	defer t.mu.Unlock()
	grid := make([][]Pixel, t.H)
	for y := range grid {
		grid[y] = make([]Pixel, t.W)
	}
	if t.W <= 0 || t.H <= 0 || t.boardW <= 0 || t.boardH <= 0 {
		return grid
	}
	cell := t.H / (t.boardH + 2)
	if cell < 1 {
		cell = 1
	}
	boardPxW := t.boardW * cell
	left := (t.W - boardPxW) / 2
	top := (t.H - t.boardH*cell) / 2
	paintCell := func(row, col int, kind byte, hue float64) {
		if row < 0 || row >= t.boardH || col < 0 || col >= t.boardW || kind == 0 {
			return
		}
		c := hslToRGB(hue, t.cfg.Saturation, (t.cfg.LightMin+t.cfg.LightMax)/2)
		for py := 0; py < cell; py++ {
			for px := 0; px < cell; px++ {
				paintPixel(grid, left+col*cell+px, top+row*cell+py, c)
			}
		}
	}
	for row := 0; row < t.boardH; row++ {
		for col := 0; col < t.boardW; col++ {
			i := row*t.boardW + col
			if i < len(t.cells) && t.cells[i] != 0 {
				paintCell(row, col, t.cells[i], t.hues[i])
			}
		}
	}
	if t.active != nil {
		if int(t.active.Kind) >= len(tetrisPieceHues) ||
			int(t.active.Kind) >= len(tetrisShapes) ||
			t.active.Rotation < 0 ||
			t.active.Rotation >= len(tetrisShapes[t.active.Kind]) {
			return grid
		}
		hue := math.Mod(t.cfg.Hue+tetrisPieceHues[t.active.Kind], 360)
		for _, off := range tetrisShapes[t.active.Kind][t.active.Rotation] {
			paintCell(t.active.Row+off[0], t.active.Col+off[1], t.active.Kind, hue)
		}
	}
	return grid
}

func paintPixel(grid [][]Pixel, x, y int, c color.RGBA) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	grid[y][x] = Pixel{Filled: true, C: c}
}
