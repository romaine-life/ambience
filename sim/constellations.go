package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	constellationsIntroTicks   = 180
	constellationsEndingTicks  = 240
	constellationsQuietTicks   = 600
	constellationsShimmerTicks = 100
)

// ConstellationsConfig tunes the constellation-forming effect. FigureSet is a
// scene selector: 0=zodiac, 1=northern winter, 2=summer triangle, 3=mythic.
type ConstellationsConfig struct {
	Twinkle       float64 `json:"twinkle"`
	StarRate      float64 `json:"starRate"`
	DrawChance    float64 `json:"drawChance"`
	FigureShimmer float64 `json:"figureShimmer"`
	FigureSet     int     `json:"figureSet"`
}

type ConstellationPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ConstellationInstance struct {
	Set           int                  `json:"set"`
	Figure        int                  `json:"figure"`
	Name          string               `json:"name"`
	Seed          uint64               `json:"seed"`
	Scale         float64              `json:"scale"`
	OffsetX       float64              `json:"offsetX"`
	OffsetY       float64              `json:"offsetY"`
	Rotation      float64              `json:"rotation"`
	Points        []ConstellationPoint `json:"points,omitempty"`
	Edges         [][2]int             `json:"edges,omitempty"`
	Phase         string               `json:"phase"`
	PhaseTicks    int                  `json:"phaseTicks"`
	PhaseTotal    int                  `json:"phaseTotal"`
	DrawTicks     int                  `json:"drawTicks"`
	HoldTicks     int                  `json:"holdTicks"`
	DissolveTicks int                  `json:"dissolveTicks"`
	Progress      float64              `json:"progress"`
	Fade          float64              `json:"fade"`
	ShimmerTicks  int                  `json:"shimmerTicks,omitempty"`
	ShimmerTotal  int                  `json:"shimmerTotal,omitempty"`
}

type ConstellationsState struct {
	Tick        int                    `json:"tick"`
	IntroTicks  int                    `json:"introTicks,omitempty"`
	IntroTotal  int                    `json:"introTotal,omitempty"`
	EndingTicks int                    `json:"endingTicks,omitempty"`
	EndingTotal int                    `json:"endingTotal,omitempty"`
	Ended       bool                   `json:"ended,omitempty"`
	QuietTicks  int                    `json:"quietTicks,omitempty"`
	SkySeed     uint64                 `json:"skySeed"`
	Figure      *ConstellationInstance `json:"figure,omitempty"`
	Lifecycle   Lifecycle              `json:"lifecycle"`
}

type ConstellationsSnapshot struct {
	ConstellationsState
	RNGState uint64 `json:"rngState,omitempty"`
}

type ConstellationsPersistedState struct {
	ConstellationsState
	RNGState uint64 `json:"rngState"`
}

type Constellations struct {
	mu sync.Mutex

	W, H int

	rng         *rngutil.RNG
	cfg         ConstellationsConfig
	tick        int
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	ended       bool
	quietTicks  int
	skySeed     uint64
	figure      *ConstellationInstance
	log         []LogEntry
}

func ConstellationsSchema() EffectSchema {
	return EffectSchema{
		Name:           "constellations",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "starRate", Label: "star rate", Slot: SlotSpawn, Group: "sky", Type: KnobFloat, Min: 0.05, Max: 1.2, Step: 0.05, Default: 0.55, Trigger: "intro",
				Description: "Density of the sparse background stars as the sky emerges."},
			{Key: "twinkle", Label: "twinkle", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0, Max: 1.5, Step: 0.05, Default: 0.55,
				Description: "Strength of the gentle brightness drift on background and figure stars."},
			{Key: "drawChance", Label: "draw chance", Slot: SlotEvent, Group: "figure", Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.00005, Default: 0.00055, Trigger: "figure-draw",
				Description: "Per-tick chance that a new constellation starts drawing while the sky is quiet."},
			{Key: "figureShimmer", Label: "figure shimmer", Slot: SlotEventMod, Group: "figure", Type: KnobFloat, Min: 0, Max: 1.5, Step: 0.05, Default: 0.6, Trigger: "dissolve",
				Description: "Strength of the shimmer that rides over the active figure before it fades."},
			{Key: "figureSet", Label: "figure set", Slot: SlotEnd, Group: "scene", Type: KnobInt, Min: 0, Max: 3, Step: 1, Default: 0, Trigger: "ending",
				Description: "Scene selector: 0=zodiac, 1=northern winter, 2=summer triangle, 3=mythic."},
		},
	}
}

func defaultConstellationsConfig() ConstellationsConfig {
	return ConstellationsConfig{
		Twinkle:       0.55,
		StarRate:      0.55,
		DrawChance:    0.00055,
		FigureShimmer: 0.6,
		FigureSet:     0,
	}
}

func NormalizeConstellationsConfig(cfg ConstellationsConfig) ConstellationsConfig {
	def := defaultConstellationsConfig()
	if cfg.Twinkle == 0 {
		cfg.Twinkle = def.Twinkle
	}
	if cfg.StarRate == 0 {
		cfg.StarRate = def.StarRate
	}
	if cfg.DrawChance == 0 {
		cfg.DrawChance = def.DrawChance
	}
	if cfg.FigureShimmer == 0 {
		cfg.FigureShimmer = def.FigureShimmer
	}
	if cfg.Twinkle < 0 {
		cfg.Twinkle = 0
	}
	if cfg.Twinkle > 1.5 {
		cfg.Twinkle = 1.5
	}
	if cfg.StarRate < 0.05 {
		cfg.StarRate = 0.05
	}
	if cfg.StarRate > 1.2 {
		cfg.StarRate = 1.2
	}
	if cfg.DrawChance < 0 {
		cfg.DrawChance = 0
	}
	if cfg.DrawChance > 0.003 {
		cfg.DrawChance = 0.003
	}
	if cfg.FigureShimmer < 0 {
		cfg.FigureShimmer = 0
	}
	if cfg.FigureShimmer > 1.5 {
		cfg.FigureShimmer = 1.5
	}
	if cfg.FigureSet < 0 {
		cfg.FigureSet = 0
	}
	if cfg.FigureSet >= len(constellationFigureSets) {
		cfg.FigureSet = len(constellationFigureSets) - 1
	}
	return cfg
}

func NewConstellations(w, h int, seed int64, cfg ConstellationsConfig) *Constellations {
	rng := rngutil.New(seed)
	skySeed := rng.Uint64()
	return &Constellations{
		W:       w,
		H:       h,
		rng:     rng,
		cfg:     NormalizeConstellationsConfig(cfg),
		skySeed: skySeed,
	}
}

func (c *Constellations) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.W = w
	c.H = h
}

func (c *Constellations) SetConfig(cfg ConstellationsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = NormalizeConstellationsConfig(cfg)
}

func (c *Constellations) EffectiveConfig() ConstellationsConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

func (c *Constellations) Snapshot() ConstellationsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ConstellationsSnapshot{
		ConstellationsState: c.snapshotStateLocked(),
		RNGState:            c.rng.State(),
	}
}

func (c *Constellations) RestoreSnapshot(s ConstellationsSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.ConstellationsState)
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

func (c *Constellations) SnapshotPersistedState() ConstellationsPersistedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ConstellationsPersistedState{
		ConstellationsState: c.snapshotStateLocked(),
		RNGState:            c.rng.State(),
	}
}

func (c *Constellations) RestorePersistedState(s ConstellationsPersistedState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreStateLocked(s.ConstellationsState)
	if s.RNGState != 0 {
		c.rng.SetState(s.RNGState)
	}
}

func (c *Constellations) snapshotStateLocked() ConstellationsState {
	return ConstellationsState{
		Tick:        c.tick,
		IntroTicks:  c.introTicks,
		IntroTotal:  c.introTotal,
		EndingTicks: c.endingTicks,
		EndingTotal: c.endingTotal,
		Ended:       c.ended,
		QuietTicks:  c.quietTicks,
		SkySeed:     c.skySeed,
		Figure:      cloneConstellationInstance(c.figure),
		Lifecycle:   c.lifecycleLocked(),
	}
}

func (c *Constellations) restoreStateLocked(s ConstellationsState) {
	c.tick = s.Tick
	c.introTicks = max(0, s.IntroTicks)
	c.introTotal = max(0, s.IntroTotal)
	c.endingTicks = max(0, s.EndingTicks)
	c.endingTotal = max(0, s.EndingTotal)
	c.ended = s.Ended
	c.quietTicks = max(0, s.QuietTicks)
	c.skySeed = s.SkySeed
	if c.skySeed == 0 {
		c.skySeed = c.rng.Uint64()
	}
	c.figure = cloneConstellationInstance(s.Figure)
}

func (c *Constellations) lifecycleLocked() Lifecycle {
	switch {
	case c.introTicks > 0:
		return LifecycleIntro
	case c.endingTicks > 0:
		return LifecycleEnding
	case c.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (c *Constellations) CurrentTick() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tick
}

func (c *Constellations) PerturbRNG(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Mix(delta)
}

func (c *Constellations) DrainLog() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.log) == 0 {
		return nil
	}
	out := c.log
	c.log = nil
	return out
}

func (c *Constellations) appendLog(kind, desc string) {
	c.log = append(c.log, LogEntry{Tick: c.tick, Type: kind, Desc: desc})
	if len(c.log) > 200 {
		c.log = c.log[len(c.log)-200:]
	}
}

func (c *Constellations) TriggerEvent(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "intro":
		c.startIntroLocked()
	case "ending":
		c.startEndingLocked()
	case "figure-draw":
		c.ended = false
		c.endingTicks = 0
		c.introTicks = 0
		c.quietTicks = 0
		c.startFigureLocked("triggered")
	case "shimmer":
		if c.figure == nil {
			c.ended = false
			c.startFigureLocked("triggered")
		}
		c.startShimmerLocked("triggered")
	case "dissolve":
		if c.figure != nil {
			c.startDissolveLocked(c.figure.DissolveTicks)
			c.appendLog("dissolve", fmt.Sprintf("started (%s)", c.figure.Name))
		} else {
			c.quietTicks = constellationsQuietTicks / 2
			c.appendLog("dissolve", "quiet sky")
		}
	case "quiet-sky":
		c.figure = nil
		c.endingTicks = 0
		c.introTicks = 0
		c.ended = false
		c.quietTicks = constellationsQuietTicks
		c.appendLog("quiet-sky", "cleared active figure")
	default:
		return false
	}
	return true
}

func (c *Constellations) Step() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tick++
	if c.quietTicks > 0 {
		c.quietTicks--
	}
	if c.introTicks > 0 {
		c.introTicks--
		if c.introTicks == 0 {
			c.introTotal = 0
			c.appendLog("intro", "settled into quiet sky")
		}
	}
	if c.endingTicks > 0 {
		c.stepFigureLocked()
		c.endingTicks--
		if c.endingTicks == 0 {
			c.endingTotal = 0
			c.ended = true
			c.figure = nil
			c.quietTicks = 0
			c.appendLog("ended", "terminal quiet sky")
		}
		return
	}
	if c.ended {
		return
	}

	c.stepFigureLocked()
	if c.introTicks > 0 || c.quietTicks > 0 || c.figure != nil {
		return
	}
	if c.cfg.DrawChance > 0 && c.rng.Float64() < c.cfg.DrawChance {
		c.startFigureLocked("started")
	}
}

func (c *Constellations) startIntroLocked() {
	c.ended = false
	c.figure = nil
	c.quietTicks = 0
	c.endingTicks = 0
	c.endingTotal = 0
	c.introTotal = constellationsIntroTicks
	c.introTicks = c.introTotal
	c.appendLog("intro", fmt.Sprintf("first stars emerging (%d ticks)", c.introTicks))
}

func (c *Constellations) startEndingLocked() {
	c.introTicks = 0
	c.introTotal = 0
	c.quietTicks = 0
	c.ended = false
	c.endingTotal = constellationsEndingTicks
	c.endingTicks = c.endingTotal
	if c.figure != nil {
		c.startDissolveLocked(c.endingTotal)
	}
	c.appendLog("ending", fmt.Sprintf("sky quieting (%d ticks)", c.endingTicks))
}

func (c *Constellations) startFigureLocked(verb string) {
	setIndex := c.figureSetLocked()
	set := constellationFigureSets[setIndex]
	figureIndex := c.rng.Intn(len(set.Figures))
	tpl := set.Figures[figureIndex]
	draw := jitterInt(c.rng, 240, 0.22)
	hold := jitterInt(c.rng, 720, 0.35)
	dissolve := jitterInt(c.rng, 230, 0.25)
	c.figure = &ConstellationInstance{
		Set:           setIndex,
		Figure:        figureIndex,
		Name:          tpl.Name,
		Seed:          c.rng.Uint64(),
		Scale:         0.74 + c.rng.Float64()*0.2,
		OffsetX:       (c.rng.Float64()*2 - 1) * 0.045,
		OffsetY:       (c.rng.Float64()*2 - 1) * 0.055,
		Rotation:      (c.rng.Float64()*2 - 1) * 0.12,
		Points:        cloneConstellationPoints(tpl.Points),
		Edges:         cloneConstellationEdges(tpl.Edges),
		Phase:         "drawing",
		PhaseTotal:    draw,
		DrawTicks:     draw,
		HoldTicks:     hold,
		DissolveTicks: dissolve,
		Fade:          1,
	}
	c.appendLog("figure-draw", fmt.Sprintf("%s %s/%s (draw=%d hold=%d)", verb, set.Name, tpl.Name, draw, hold))
}

func (c *Constellations) startShimmerLocked(verb string) {
	if c.figure == nil {
		return
	}
	total := jitterInt(c.rng, constellationsShimmerTicks, 0.25)
	c.figure.ShimmerTotal = total
	c.figure.ShimmerTicks = total
	c.appendLog("shimmer", fmt.Sprintf("%s (%s, %d ticks)", verb, c.figure.Name, total))
}

func (c *Constellations) startDissolveLocked(total int) {
	if c.figure == nil {
		return
	}
	if total <= 0 {
		total = c.figure.DissolveTicks
	}
	if total <= 0 {
		total = 1
	}
	if c.figure.Progress < 1 {
		c.figure.Progress = 1
	}
	c.figure.Phase = "dissolving"
	c.figure.PhaseTicks = 0
	c.figure.PhaseTotal = total
	c.figure.Fade = 1
}

func (c *Constellations) stepFigureLocked() {
	if c.figure == nil {
		return
	}
	f := c.figure
	if f.ShimmerTicks > 0 {
		f.ShimmerTicks--
		if f.ShimmerTicks == 0 {
			f.ShimmerTotal = 0
		}
	}
	total := max(1, f.PhaseTotal)
	f.PhaseTicks++
	switch f.Phase {
	case "drawing":
		f.Progress = clamp01(float64(f.PhaseTicks) / float64(total))
		f.Fade = 1
		if f.PhaseTicks >= total {
			f.Phase = "holding"
			f.PhaseTicks = 0
			f.PhaseTotal = max(1, f.HoldTicks)
			f.Progress = 1
		}
	case "holding":
		f.Progress = 1
		f.Fade = 1
		if f.PhaseTicks >= total {
			c.startDissolveLocked(f.DissolveTicks)
		}
	case "dissolving":
		f.Progress = 1
		f.Fade = clamp01(1 - float64(f.PhaseTicks)/float64(total))
		if f.PhaseTicks >= total {
			c.appendLog("dissolve", fmt.Sprintf("completed (%s)", f.Name))
			c.figure = nil
		}
	default:
		f.Phase = "drawing"
		f.PhaseTicks = 0
		f.PhaseTotal = max(1, f.DrawTicks)
		f.Progress = 0
		f.Fade = 1
	}
}

func (c *Constellations) figureSetLocked() int {
	setIndex := c.cfg.FigureSet
	if setIndex < 0 {
		return 0
	}
	if setIndex >= len(constellationFigureSets) {
		return len(constellationFigureSets) - 1
	}
	return setIndex
}

func (c *Constellations) GridCopy() [][]Pixel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.renderLocked()
}

func (c *Constellations) Frame() [][]Pixel {
	return c.GridCopy()
}

func (c *Constellations) renderLocked() [][]Pixel {
	grid := newPixelGrid(c.W, c.H)
	if c.W <= 0 || c.H <= 0 {
		return grid
	}
	c.renderSkyLocked(grid)
	if c.figure != nil {
		c.renderFigureLocked(grid, c.figure)
	}
	return grid
}

func (c *Constellations) renderSkyLocked(grid [][]Pixel) {
	level := c.skyLevelLocked()
	top := hslToRGB(226, 0.58, 0.025+0.018*level)
	bottom := hslToRGB(222, 0.5, 0.065+0.035*level)
	for y := 0; y < c.H; y++ {
		t := 0.0
		if c.H > 1 {
			t = float64(y) / float64(c.H-1)
		}
		rowColor := mixConstellationColor(top, bottom, t)
		for x := 0; x < c.W; x++ {
			grid[y][x] = Pixel{Filled: true, C: rowColor}
		}
	}

	count := int(math.Round(float64(c.W*c.H) * (0.004 + 0.012*c.cfg.StarRate) * (0.18 + 0.82*level)))
	if level > 0.05 {
		count = max(8, count)
	}
	for i := 0; i < count; i++ {
		x := int(constellationHash(c.skySeed, i*5+1) * float64(c.W))
		y := int(constellationHash(c.skySeed, i*5+2) * float64(c.H))
		if x >= c.W {
			x = c.W - 1
		}
		if y >= c.H {
			y = c.H - 1
		}
		sizeRoll := constellationHash(c.skySeed, i*5+3)
		phase := constellationHash(c.skySeed, i*5+4) * math.Pi * 2
		twinkle := 1 + c.cfg.Twinkle*0.32*math.Sin(float64(c.tick)*0.028*(0.35+sizeRoll)+phase)
		base := 0.35 + constellationHash(c.skySeed, i*5+5)*0.65
		alpha := clamp01((0.25 + 0.55*base) * level * twinkle)
		light := clamp01(0.58 + 0.36*base*twinkle)
		col := hslToRGB(216+(base-0.5)*24, 0.12+0.08*base, light)
		blendConstellationPixel(grid, x, y, col, alpha)
		if sizeRoll > 0.94 && c.W >= 48 && c.H >= 28 {
			halo := hslToRGB(215, 0.1, 0.72)
			blendConstellationPixel(grid, x+1, y, halo, alpha*0.22)
			blendConstellationPixel(grid, x-1, y, halo, alpha*0.18)
		}
	}
}

func (c *Constellations) skyLevelLocked() float64 {
	level := 1.0
	if c.introTicks > 0 {
		level = 0.12 + 0.88*constellationPhaseProgress(c.introTotal, c.introTicks)
	}
	if c.endingTicks > 0 {
		level = 1 - 0.86*constellationPhaseProgress(c.endingTotal, c.endingTicks)
	}
	if c.ended {
		level = 0.12
	}
	if c.quietTicks > 0 {
		level *= 0.6
	}
	return clamp01(level)
}

func (c *Constellations) renderFigureLocked(grid [][]Pixel, f *ConstellationInstance) {
	points := c.figureScreenPoints(f)
	if len(points) == 0 {
		return
	}
	shimmerLevel := 0.0
	if f.ShimmerTicks > 0 && f.ShimmerTotal > 0 {
		shimmerLevel = math.Sin(constellationPhaseProgress(f.ShimmerTotal, f.ShimmerTicks)*math.Pi) * c.cfg.FigureShimmer
	}
	lineColor := hslToRGB(214, 0.2, clamp01(0.68+0.16*shimmerLevel))
	starColor := hslToRGB(46, 0.24, clamp01(0.86+0.08*shimmerLevel))
	lineAlpha := clamp01((0.52 + 0.18*shimmerLevel) * f.Fade)

	totalLen := 0.0
	edgeLens := make([]float64, len(f.Edges))
	for i, edge := range f.Edges {
		if edge[0] < 0 || edge[0] >= len(points) || edge[1] < 0 || edge[1] >= len(points) {
			continue
		}
		a, b := points[edge[0]], points[edge[1]]
		edgeLens[i] = math.Hypot(b.X-a.X, b.Y-a.Y)
		totalLen += edgeLens[i]
	}
	remaining := totalLen * clamp01(f.Progress)
	for i, edge := range f.Edges {
		if remaining <= 0 {
			break
		}
		if edge[0] < 0 || edge[0] >= len(points) || edge[1] < 0 || edge[1] >= len(points) || edgeLens[i] <= 0 {
			continue
		}
		a, b := points[edge[0]], points[edge[1]]
		portion := math.Min(1, remaining/edgeLens[i])
		end := ConstellationPoint{
			X: a.X + (b.X-a.X)*portion,
			Y: a.Y + (b.Y-a.Y)*portion,
		}
		drawConstellationLine(grid, a.X, a.Y, end.X, end.Y, lineColor, lineAlpha)
		remaining -= edgeLens[i]
	}

	pointAlpha := clamp01((0.34 + 0.66*f.Progress) * f.Fade)
	for i, p := range points {
		local := 1 + c.cfg.Twinkle*0.16*math.Sin(float64(c.tick)*0.035+float64(i)*1.7+float64(f.Seed%97))
		alpha := clamp01(pointAlpha * local)
		x := int(math.Round(p.X))
		y := int(math.Round(p.Y))
		blendConstellationPixel(grid, x, y, starColor, alpha)
		if alpha > 0.72 && c.W >= 50 && c.H >= 30 {
			halo := hslToRGB(45, 0.18, 0.78)
			blendConstellationPixel(grid, x+1, y, halo, alpha*0.24)
			blendConstellationPixel(grid, x, y+1, halo, alpha*0.16)
		}
	}
}

func (c *Constellations) figureScreenPoints(f *ConstellationInstance) []ConstellationPoint {
	out := make([]ConstellationPoint, len(f.Points))
	cosR := math.Cos(f.Rotation)
	sinR := math.Sin(f.Rotation)
	cx := 0.5 + f.OffsetX
	cy := 0.49 + f.OffsetY
	for i, p := range f.Points {
		x := (p.X - 0.5) * f.Scale
		y := (p.Y - 0.5) * f.Scale
		rx := x*cosR - y*sinR
		ry := x*sinR + y*cosR
		sx := (cx + rx) * float64(max(1, c.W-1))
		sy := (cy + ry) * float64(max(1, c.H-1))
		out[i] = ConstellationPoint{
			X: clampFloat(sx, 1, float64(max(1, c.W-2))),
			Y: clampFloat(sy, 1, float64(max(1, c.H-2))),
		}
	}
	return out
}

func cloneConstellationInstance(src *ConstellationInstance) *ConstellationInstance {
	if src == nil {
		return nil
	}
	out := *src
	out.Points = cloneConstellationPoints(src.Points)
	out.Edges = cloneConstellationEdges(src.Edges)
	return &out
}

func cloneConstellationPoints(src []ConstellationPoint) []ConstellationPoint {
	if len(src) == 0 {
		return nil
	}
	out := make([]ConstellationPoint, len(src))
	copy(out, src)
	return out
}

func cloneConstellationEdges(src [][2]int) [][2]int {
	if len(src) == 0 {
		return nil
	}
	out := make([][2]int, len(src))
	copy(out, src)
	return out
}

func constellationPhaseProgress(total, left int) float64 {
	if total <= 1 || left <= 0 {
		return 1
	}
	elapsed := total - left
	if elapsed <= 0 {
		return 0
	}
	return clamp01(float64(elapsed) / float64(total-1))
}

func constellationHash(seed uint64, index int) float64 {
	z := seed + uint64(index+1)*0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return float64(z>>11) * (1.0 / (1 << 53))
}

func clampFloat(v, lo, hi float64) float64 {
	if hi < lo {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func blendConstellationPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	if alpha >= 1 || !grid[y][x].Filled {
		grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}}
		return
	}
	prev := grid[y][x].C
	grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(prev.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(prev.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(prev.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func drawConstellationLine(grid [][]Pixel, x0, y0, x1, y1 float64, c color.RGBA, alpha float64) {
	steps := int(math.Ceil(math.Max(math.Abs(x1-x0), math.Abs(y1-y0)) * 1.35))
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(x0 + (x1-x0)*t))
		y := int(math.Round(y0 + (y1-y0)*t))
		blendConstellationPixel(grid, x, y, c, alpha)
	}
}

func mixConstellationColor(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: 255,
	}
}

type constellationFigureSet struct {
	Name    string
	Figures []constellationTemplate
}

type constellationTemplate struct {
	Name   string
	Points []ConstellationPoint
	Edges  [][2]int
}

var constellationFigureSets = []constellationFigureSet{
	{
		Name: "zodiac",
		Figures: []constellationTemplate{
			{
				Name:   "aries",
				Points: []ConstellationPoint{{0.18, 0.48}, {0.34, 0.38}, {0.52, 0.43}, {0.68, 0.35}, {0.82, 0.47}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}},
			},
			{
				Name:   "leo",
				Points: []ConstellationPoint{{0.18, 0.62}, {0.34, 0.53}, {0.44, 0.37}, {0.58, 0.34}, {0.55, 0.52}, {0.69, 0.64}, {0.84, 0.55}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {4, 1}},
			},
			{
				Name:   "scorpio",
				Points: []ConstellationPoint{{0.15, 0.36}, {0.31, 0.41}, {0.47, 0.49}, {0.61, 0.6}, {0.72, 0.72}, {0.84, 0.65}, {0.9, 0.5}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}},
			},
		},
	},
	{
		Name: "northern-winter",
		Figures: []constellationTemplate{
			{
				Name:   "orion",
				Points: []ConstellationPoint{{0.32, 0.22}, {0.67, 0.24}, {0.43, 0.45}, {0.51, 0.47}, {0.59, 0.49}, {0.36, 0.76}, {0.68, 0.75}},
				Edges:  [][2]int{{0, 2}, {2, 3}, {3, 4}, {4, 1}, {2, 5}, {4, 6}, {0, 1}, {5, 6}},
			},
			{
				Name:   "cassiopeia",
				Points: []ConstellationPoint{{0.15, 0.45}, {0.32, 0.32}, {0.48, 0.54}, {0.66, 0.36}, {0.85, 0.5}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}},
			},
			{
				Name:   "taurus",
				Points: []ConstellationPoint{{0.2, 0.24}, {0.38, 0.4}, {0.5, 0.5}, {0.64, 0.39}, {0.82, 0.22}, {0.52, 0.68}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {2, 5}},
			},
		},
	},
	{
		Name: "summer-triangle",
		Figures: []constellationTemplate{
			{
				Name:   "summer-triangle",
				Points: []ConstellationPoint{{0.2, 0.72}, {0.5, 0.2}, {0.82, 0.67}, {0.48, 0.52}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 0}, {1, 3}},
			},
			{
				Name:   "cygnus",
				Points: []ConstellationPoint{{0.48, 0.18}, {0.5, 0.36}, {0.52, 0.55}, {0.55, 0.78}, {0.22, 0.46}, {0.78, 0.47}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {4, 2}, {2, 5}},
			},
			{
				Name:   "lyra",
				Points: []ConstellationPoint{{0.3, 0.2}, {0.43, 0.45}, {0.62, 0.38}, {0.72, 0.6}, {0.52, 0.72}, {0.35, 0.58}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 1}},
			},
		},
	},
	{
		Name: "mythic",
		Figures: []constellationTemplate{
			{
				Name:   "crown",
				Points: []ConstellationPoint{{0.2, 0.55}, {0.3, 0.38}, {0.43, 0.48}, {0.52, 0.3}, {0.62, 0.49}, {0.75, 0.38}, {0.85, 0.56}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {0, 6}},
			},
			{
				Name:   "dragon",
				Points: []ConstellationPoint{{0.16, 0.32}, {0.28, 0.45}, {0.42, 0.36}, {0.54, 0.52}, {0.67, 0.45}, {0.79, 0.6}, {0.88, 0.42}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}},
			},
			{
				Name:   "pegasus",
				Points: []ConstellationPoint{{0.26, 0.28}, {0.62, 0.25}, {0.72, 0.6}, {0.36, 0.66}, {0.18, 0.5}, {0.82, 0.42}},
				Edges:  [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {3, 4}, {1, 5}},
			},
		},
	},
}
