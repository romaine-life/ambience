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
	return renderProceduralGrid("aurora", a.W, a.H, a.tick, a.cfg, a.timers, a.values, a.rng.State())
}

func (a *AutumnLeaves) GridCopy() [][]Pixel {
	a.mu.Lock()
	defer a.mu.Unlock()
	return renderProceduralGrid("autumn-leaves", a.W, a.H, a.tick, a.cfg, a.timers, a.values, a.rng.State())
}

func (b *Beach) GridCopy() [][]Pixel {
	b.mu.Lock()
	defer b.mu.Unlock()
	return renderProceduralGrid("beach", b.W, b.H, b.tick, b.cfg, b.timers, b.values, b.rng.State())
}

func (b *Bog) GridCopy() [][]Pixel {
	b.mu.Lock()
	defer b.mu.Unlock()
	return copyPixelGrid(b.Grid)
}

func (c *Campfire) GridCopy() [][]Pixel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return renderProceduralGrid("campfire", c.W, c.H, c.tick, c.cfg, c.timers, c.values, c.rng.State())
}

func (d *DistantStorm) GridCopy() [][]Pixel {
	d.mu.Lock()
	defer d.mu.Unlock()
	return renderProceduralGrid("distant-storm", d.W, d.H, d.tick, d.cfg, d.timers, d.values, d.rng.State())
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
	return renderProceduralGrid("lighthouse", l.W, l.H, l.tick, l.cfg, l.timers, l.values, l.rng.State())
}

func (m *MysteriousMan) GridCopy() [][]Pixel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return renderProceduralGrid("mysterious-man", m.W, m.H, m.tick, m.cfg, m.timers, m.values, m.rng.State())
}

func (r *Rowboat) GridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return renderProceduralGrid("rowboat", r.W, r.H, r.tick, r.cfg, r.timers, r.values, r.rng.State())
}

func (s *Sand) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyPixelGrid(s.Grid)
}

func (s *Snow) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return renderProceduralGrid("snow", s.W, s.H, s.tick, s.cfg, s.timers, s.values, s.rng.State())
}

func (s *Starfield) GridCopy() [][]Pixel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return renderProceduralGrid("starfield", s.W, s.H, s.tick, s.cfg, s.timers, s.values, s.rng.State())
}

func (t *Train) GridCopy() [][]Pixel {
	t.mu.Lock()
	defer t.mu.Unlock()
	return renderProceduralGrid("train", t.W, t.H, t.tick, t.cfg, t.timers, t.values, t.rng.State())
}

func (u *Underwater) GridCopy() [][]Pixel {
	u.mu.Lock()
	defer u.mu.Unlock()
	return renderProceduralGrid("underwater", u.W, u.H, u.tick, u.cfg, u.timers, u.values, u.rng.State())
}

func (v *Volcano) GridCopy() [][]Pixel {
	v.mu.Lock()
	defer v.mu.Unlock()
	return renderProceduralGrid("volcano", v.W, v.H, v.tick, v.cfg, v.timers, v.values, v.rng.State())
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
	return renderProceduralGrid("wheat-field", w.W, w.H, w.tick, w.cfg, w.timers, w.values, w.rng.State())
}

func (w *Windmill) GridCopy() [][]Pixel {
	w.mu.Lock()
	defer w.mu.Unlock()
	return renderProceduralGrid("windmill", w.W, w.H, w.tick, w.cfg, w.timers, w.values, w.rng.State())
}

func proceduralGridCopy(kind string, w, h, tick int, cfg ProceduralConfig) [][]Pixel {
	grid := make([][]Pixel, h)
	for y := range grid {
		grid[y] = make([]Pixel, w)
	}
	if w <= 0 || h <= 0 {
		return grid
	}
	kindHash := uint32(0)
	for _, r := range kind {
		kindHash = kindHash*31 + uint32(r)
	}
	hue := cfg["hue"]
	if hue == 0 {
		hue = float64(kindHash % 360)
	}
	sat := cfg["sat"]
	if sat <= 0 {
		sat = 0.45
	}
	lmin := cfg["lmin"]
	if lmin <= 0 {
		lmin = 0.12
	}
	lmax := cfg["lmax"]
	if lmax <= 0 {
		lmax = 0.72
	}
	if lmax < lmin {
		lmin, lmax = lmax, lmin
	}
	for y := 0; y < h; y++ {
		yr := 0.0
		if h > 1 {
			yr = float64(y) / float64(h-1)
		}
		for x := 0; x < w; x++ {
			wave := math.Sin((float64(x)+float64(tick)*(0.12+float64(kindHash&7)*0.018))*(0.09+float64((kindHash>>4)&7)*0.01)+float64(kindHash)*0.001) +
				math.Sin((float64(y)-float64(tick)*(0.09+float64((kindHash>>8)&7)*0.018))*(0.13+float64((kindHash>>12)&7)*0.012)+float64(kindHash)*0.0007)
			sparkle := math.Sin((float64(x)*(13+float64(kindHash&5)) + float64(y)*(23+float64((kindHash>>3)&7)) + float64(tick) + float64(kindHash)) * 0.071)
			band := math.Sin((float64(x) + float64(y) + float64(tick)*0.2) * (0.04 + float64((kindHash>>16)&7)*0.006))
			light := clamp01(lmin + (lmax-lmin)*(0.25+0.42*yr+0.16*wave+0.08*band+0.1*math.Max(0, sparkle)))
			c := hslToRGB(math.Mod(hue+float64(kindHash%70)-35+wave*16+sparkle*8+360, 360), clamp01(sat), light)
			grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
	count := max(8, int(math.Floor(float64(w*h)*0.012)))
	for i := 0; i < count; i++ {
		x := int((uint32(i)*2654435761 + kindHash + uint32(tick/6)*97) % uint32(w))
		y := int((uint32(i)*2246822519 + (kindHash >> 1) + uint32(tick/5)*131) % uint32(h))
		c := hslToRGB(math.Mod(hue+float64((i*17)%60)-30+360, 360), clamp01(sat*1.1), clamp01(lmax))
		paintPixel(grid, x, y, c)
	}
	return grid
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
		h := int(math.Round(cfg.TreeMinH + (cfg.TreeMaxH-cfg.TreeMinH)*0.55))
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
