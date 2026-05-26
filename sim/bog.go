package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// Bog is a still, murky bog whose only motion is methane bubbles welling up
// from the muck and popping at the surface. Authority decides when bubbles
// spawn and how fast they rise; clients run a local replica and ride the
// snapshot/RNG-state seam to stay coherent without per-bubble streaming.

type bogBubble struct {
	Row, Col float64
	BaseCol  float64
	VRow     float64
	Radius   float64
	Phase    float64
	Wobble   float64
}

type bogRipple struct {
	Col     float64
	Radius  float64
	Life    int
	MaxLife int
}

// BogConfig tunes the Bog prototype used in isolated dev sessions.
type BogConfig struct {
	// LEVERS — water
	Surface  float64 `json:"surface"`
	DepthDim float64 `json:"depth_dim"`
	Mist     float64 `json:"mist"`
	// LEVERS — bubbles
	SpawnEvery int     `json:"spawn"`
	MaxBubbles int     `json:"max"`
	Rise       float64 `json:"rise"`
	WobbleAmp  float64 `json:"wobble"`
	BubbleSize float64 `json:"size"`
	// LEVERS — color
	WaterHue   float64 `json:"water_hue"`
	MistHue    float64 `json:"mist_hue"`
	Saturation float64 `json:"sat"`
	LightMin   float64 `json:"lmin"`
	LightMax   float64 `json:"lmax"`
	// EVENT CHANCES
	GushChance  float64 `json:"gush_p"`
	CalmChance  float64 `json:"calm_p"`
	SurgeChance float64 `json:"surge_p"`
	// EVENT MODIFIERS
	GushCount int     `json:"gush_count"`
	GushDur   int     `json:"gush_dur"`
	CalmDur   int     `json:"calm_dur"`
	CalmMult  float64 `json:"calm_mult"`
	SurgeDur  int     `json:"surge_dur"`
	SurgeMult float64 `json:"surge_mult"`
}

func (c BogConfig) withDefaults() BogConfig {
	if c.Surface <= 0 {
		c.Surface = 0.36
	}
	c.Surface = clamp01(c.Surface)
	if c.DepthDim < 0 {
		c.DepthDim = 0
	}
	c.DepthDim = clamp01(c.DepthDim)
	if c.Mist < 0 {
		c.Mist = 0
	}
	c.Mist = clamp01(c.Mist)
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 4
	}
	if c.MaxBubbles <= 0 {
		c.MaxBubbles = 36
	}
	if c.Rise <= 0 {
		c.Rise = 0.14
	}
	if c.WobbleAmp < 0 {
		c.WobbleAmp = 0
	}
	if c.BubbleSize <= 0 {
		c.BubbleSize = 1.6
	}
	if c.WaterHue == 0 {
		c.WaterHue = 105
	}
	if c.MistHue == 0 {
		c.MistHue = 95
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.46
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.08
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.7
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.GushChance < 0 {
		c.GushChance = 0
	}
	if c.CalmChance < 0 {
		c.CalmChance = 0
	}
	if c.SurgeChance < 0 {
		c.SurgeChance = 0
	}
	if c.GushCount <= 0 {
		c.GushCount = 8
	}
	if c.GushDur <= 0 {
		c.GushDur = 18
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 80
	}
	if c.CalmMult <= 0 || c.CalmMult > 1 {
		c.CalmMult = 0.25
	}
	if c.SurgeDur <= 0 {
		c.SurgeDur = 50
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 1.8
	}
	return c
}

// BogSchema describes the Bog effect's tunable knobs for the dev UI.
func BogSchema() EffectSchema {
	return EffectSchema{
		Name: "bog",
		Knobs: []Knob{
			{Key: "surface", Label: "surface", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.15, Max: 0.55, Step: 0.01, Default: 0.36,
				Description: "Vertical position of the bog surface as a fraction of the frame height."},
			{Key: "depth_dim", Label: "depth", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.55,
				Description: "How much darker the water gets toward the bottom of the bog."},
			{Key: "mist", Label: "mist", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.28,
				Description: "How heavy the haze above the surface reads. Higher values feel like fog."},
			{Key: "spawn", Label: "spawn 1/", Slot: SlotLever, Group: "bubbles", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 4,
				Description: "One-in-N per-tick chance of a new bubble welling up while under the cap."},
			{Key: "max", Label: "max bubbles", Slot: SlotLever, Group: "bubbles", Type: KnobInt, Min: 4, Max: 96, Step: 1, Default: 36,
				Description: "Maximum live bubbles climbing through the bog at once."},
			{Key: "rise", Label: "rise", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Rows per tick a bubble drifts upward toward the surface."},
			{Key: "wobble", Label: "wobble", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 0, Max: 3, Step: 0.1, Default: 1.2,
				Description: "Side-to-side sway amplitude of each bubble's path."},
			{Key: "size", Label: "size", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 1, Max: 3, Step: 0.25, Default: 1.6,
				Description: "Maximum bubble radius. Most bubbles are smaller; this caps the largest."},
			{Key: "water_hue", Label: "water hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 60, Max: 200, Step: 1, Default: 105,
				Description: "Base hue of the bog water. Lower values lean yellow-green; higher lean teal."},
			{Key: "mist_hue", Label: "mist hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 60, Max: 200, Step: 1, Default: 95,
				Description: "Base hue of the haze above the surface."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.02, Default: 0.46,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Minimum lightness for the deepest water and the night behind the mist."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 0.95, Step: 0.01, Default: 0.7,
				Description: "Maximum lightness used for surface highlights and bubble rims."},
			{Key: "gush_p", Label: "gush", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "bubble-burst",
				Description: "Per-tick chance of a localized burst that releases several bubbles together."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter window where bubbles barely surface."},
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surge",
				Description: "Per-tick chance of methane pressure briefly speeding bubbles up."},
			{Key: "gush_count", Label: "gush count", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobInt, Min: 2, Max: 24, Step: 1, Default: 8,
				Description: "Approximate number of bubbles released during a single gush."},
			{Key: "gush_dur", Label: "gush dur", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobInt, Min: 4, Max: 80, Step: 2, Default: 18,
				Description: "How long the gush keeps releasing extra bubbles, in ticks."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80,
				Description: "Duration of a quiet bog window, in ticks."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Spawn-rate multiplier applied while a calm is active."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 50,
				Description: "How long a surge keeps bubbles rising faster, in ticks."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Rise-speed multiplier applied to active bubbles during a surge."},
		},
	}
}

// BogState is the wire/persisted scalar shape of the bog runtime.
type BogState struct {
	Tick       int `json:"tick"`
	GushTicks  int `json:"gushTicks"`
	CalmTicks  int `json:"calmTicks"`
	SurgeTicks int `json:"surgeTicks"`
}

// BogBubble is the serializable shape of one live methane bubble.
type BogBubble struct {
	Row     float64 `json:"row"`
	Col     float64 `json:"col"`
	BaseCol float64 `json:"baseCol"`
	VRow    float64 `json:"vRow"`
	Radius  float64 `json:"r"`
	Phase   float64 `json:"phase"`
	Wobble  float64 `json:"wobble"`
}

// BogRipple is the serializable shape of one expanding surface ripple.
type BogRipple struct {
	Col     float64 `json:"col"`
	Radius  float64 `json:"radius"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
}

type BogSnapshot struct {
	BogState
	RNGState uint64      `json:"rngState,omitempty"`
	Bubbles  []BogBubble `json:"bubbles"`
	Ripples  []BogRipple `json:"ripples"`
}

type BogPersistedState struct {
	BogState
	RNGState uint64      `json:"rngState"`
	Bubbles  []BogBubble `json:"bubbles"`
	Ripples  []BogRipple `json:"ripples"`
}

// Bog is the authoritative server-side bog simulation. Clients run the same
// runtime locally; the only cross-replica truth is the snapshot/event seam.
type Bog struct {
	mu sync.Mutex

	W, H    int
	Grid    [][]Pixel
	bubbles []bogBubble
	ripples []bogRipple
	rng     *rngutil.RNG
	cfg     BogConfig
	tick    int

	gushTicks  int
	calmTicks  int
	surgeTicks int

	log []LogEntry
}

func NewBog(w, h int, seed int64, cfg BogConfig) *Bog {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Bog{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
}

func (b *Bog) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if w == b.W && h == b.H {
		return
	}
	b.W = w
	b.H = h
	b.Grid = make([][]Pixel, h)
	for i := range b.Grid {
		b.Grid[i] = make([]Pixel, w)
	}
}

func (b *Bog) SetConfig(cfg BogConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cfg = cfg.withDefaults()
}

func (b *Bog) EffectiveConfig() BogConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

func (b *Bog) CurrentTick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tick
}

func (b *Bog) PerturbRNG(delta int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rng.Mix(delta)
}

func (b *Bog) DrainLog() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) == 0 {
		return nil
	}
	out := b.log
	b.log = nil
	return out
}

func (b *Bog) appendLog(kind, desc string) {
	b.log = append(b.log, LogEntry{Tick: b.tick, Type: kind, Desc: desc})
	if len(b.log) > 200 {
		b.log = b.log[len(b.log)-200:]
	}
}

func (b *Bog) Snapshot() BogSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BogSnapshot{
		BogState: b.snapshotStateLocked(),
		RNGState: b.rng.State(),
		Bubbles:  b.copyBubblesLocked(),
		Ripples:  b.copyRipplesLocked(),
	}
}

func (b *Bog) RestoreSnapshot(s BogSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.BogState)
	if s.RNGState != 0 {
		b.rng.SetState(s.RNGState)
	}
	b.restoreBubblesLocked(s.Bubbles)
	b.restoreRipplesLocked(s.Ripples)
	b.paintFrameLocked()
}

func (b *Bog) SnapshotPersistedState() BogPersistedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BogPersistedState{
		BogState: b.snapshotStateLocked(),
		RNGState: b.rng.State(),
		Bubbles:  b.copyBubblesLocked(),
		Ripples:  b.copyRipplesLocked(),
	}
}

func (b *Bog) RestorePersistedState(s BogPersistedState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(s.BogState)
	if s.RNGState != 0 {
		b.rng.SetState(s.RNGState)
	}
	b.restoreBubblesLocked(s.Bubbles)
	b.restoreRipplesLocked(s.Ripples)
}

func (b *Bog) snapshotStateLocked() BogState {
	return BogState{
		Tick:       b.tick,
		GushTicks:  b.gushTicks,
		CalmTicks:  b.calmTicks,
		SurgeTicks: b.surgeTicks,
	}
}

func (b *Bog) restoreStateLocked(s BogState) {
	b.tick = s.Tick
	b.gushTicks = s.GushTicks
	b.calmTicks = s.CalmTicks
	b.surgeTicks = s.SurgeTicks
}

func (b *Bog) copyBubblesLocked() []BogBubble {
	out := make([]BogBubble, len(b.bubbles))
	for i, bb := range b.bubbles {
		out[i] = BogBubble{
			Row:     bb.Row,
			Col:     bb.Col,
			BaseCol: bb.BaseCol,
			VRow:    bb.VRow,
			Radius:  bb.Radius,
			Phase:   bb.Phase,
			Wobble:  bb.Wobble,
		}
	}
	return out
}

func (b *Bog) restoreBubblesLocked(list []BogBubble) {
	b.bubbles = make([]bogBubble, len(list))
	for i, bb := range list {
		b.bubbles[i] = bogBubble{
			Row:     bb.Row,
			Col:     bb.Col,
			BaseCol: bb.BaseCol,
			VRow:    bb.VRow,
			Radius:  bb.Radius,
			Phase:   bb.Phase,
			Wobble:  bb.Wobble,
		}
	}
}

func (b *Bog) copyRipplesLocked() []BogRipple {
	out := make([]BogRipple, len(b.ripples))
	for i, r := range b.ripples {
		out[i] = BogRipple{
			Col:     r.Col,
			Radius:  r.Radius,
			Life:    r.Life,
			MaxLife: r.MaxLife,
		}
	}
	return out
}

func (b *Bog) restoreRipplesLocked(list []BogRipple) {
	b.ripples = make([]bogRipple, len(list))
	for i, r := range list {
		b.ripples[i] = bogRipple{
			Col:     r.Col,
			Radius:  r.Radius,
			Life:    r.Life,
			MaxLife: r.MaxLife,
		}
	}
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (b *Bog) TriggerEvent(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch name {
	case "bubble-burst":
		b.gushTicks = jitterInt(b.rng, b.cfg.GushDur, 0.3)
		b.appendLog("bubble-burst", fmt.Sprintf("triggered (dur=%d, count~%d)", b.gushTicks, b.cfg.GushCount))
	case "calm":
		b.calmTicks = jitterInt(b.rng, b.cfg.CalmDur, 0.3)
		b.appendLog("calm", fmt.Sprintf("triggered (dur=%d, x%.2f)", b.calmTicks, b.cfg.CalmMult))
	case "surge":
		b.surgeTicks = jitterInt(b.rng, b.cfg.SurgeDur, 0.3)
		b.appendLog("surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", b.surgeTicks, b.cfg.SurgeMult))
	default:
		return false
	}
	return true
}

func (b *Bog) Step() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tick++
	if b.gushTicks > 0 {
		b.gushTicks--
	}
	if b.calmTicks > 0 {
		b.calmTicks--
	}
	if b.surgeTicks > 0 {
		b.surgeTicks--
	}

	if b.gushTicks == 0 && b.cfg.GushChance > 0 && b.rng.Float64() < b.cfg.GushChance {
		b.gushTicks = jitterInt(b.rng, b.cfg.GushDur, 0.3)
		b.appendLog("bubble-burst", fmt.Sprintf("started (dur=%d, count~%d)", b.gushTicks, b.cfg.GushCount))
	}
	if b.calmTicks == 0 && b.cfg.CalmChance > 0 && b.rng.Float64() < b.cfg.CalmChance {
		b.calmTicks = jitterInt(b.rng, b.cfg.CalmDur, 0.3)
		b.appendLog("calm", fmt.Sprintf("started (dur=%d)", b.calmTicks))
	}
	if b.surgeTicks == 0 && b.cfg.SurgeChance > 0 && b.rng.Float64() < b.cfg.SurgeChance {
		b.surgeTicks = jitterInt(b.rng, b.cfg.SurgeDur, 0.3)
		b.appendLog("surge", fmt.Sprintf("started (dur=%d, x%.2f)", b.surgeTicks, b.cfg.SurgeMult))
	}

	b.spawnBubblesLocked()
	b.stepBubblesLocked()
	b.stepRipplesLocked()
	b.paintFrameLocked()
}

func (b *Bog) spawnBubblesLocked() {
	if b.W <= 0 || b.H <= 0 {
		return
	}
	cadence := b.cfg.SpawnEvery
	if b.calmTicks > 0 {
		mult := 1.0 / math.Max(0.05, b.cfg.CalmMult)
		cadence = int(math.Round(float64(cadence) * mult))
	}
	if cadence < 1 {
		cadence = 1
	}
	if len(b.bubbles) < b.cfg.MaxBubbles && b.rng.Intn(cadence) == 0 {
		b.spawnBubbleLocked(-1)
	}
	if b.gushTicks > 0 && len(b.bubbles) < b.cfg.MaxBubbles {
		expected := float64(b.cfg.GushCount) / math.Max(1, float64(b.cfg.GushDur))
		n := int(math.Floor(expected))
		if b.rng.Float64() < expected-float64(n) {
			n++
		}
		col := b.rng.Intn(max(1, b.W))
		for i := 0; i < n && len(b.bubbles) < b.cfg.MaxBubbles; i++ {
			b.spawnBubbleLocked(col)
		}
	}
}

func (b *Bog) spawnBubbleLocked(anchorCol int) {
	if b.W <= 0 || b.H <= 0 {
		return
	}
	col := float64(b.rng.Intn(b.W))
	if anchorCol >= 0 {
		spread := math.Max(2, float64(b.W)*0.06)
		col = float64(anchorCol) + (b.rng.Float64()*2-1)*spread
		if col < 0 {
			col = 0
		}
		if col >= float64(b.W) {
			col = float64(b.W - 1)
		}
	}
	row := float64(b.H) - 1 - b.rng.Float64()*math.Max(1, float64(b.H)*0.08)
	radius := math.Max(1, b.cfg.BubbleSize*(0.55+b.rng.Float64()*0.55))
	wobble := b.cfg.WobbleAmp * (0.4 + b.rng.Float64()*0.8)
	b.bubbles = append(b.bubbles, bogBubble{
		Row:     row,
		Col:     col,
		BaseCol: col,
		VRow:    b.cfg.Rise * (0.65 + b.rng.Float64()*0.8),
		Radius:  radius,
		Phase:   b.rng.Float64() * 2 * math.Pi,
		Wobble:  wobble,
	})
}

func (b *Bog) stepBubblesLocked() {
	if len(b.bubbles) == 0 {
		return
	}
	surfaceRow := b.surfaceRowLocked()
	speedMult := 1.0
	if b.surgeTicks > 0 {
		speedMult = b.cfg.SurgeMult
	}
	alive := b.bubbles[:0]
	for _, bb := range b.bubbles {
		bb.Row -= bb.VRow * speedMult
		bb.Phase += 0.18
		bb.Col = bb.BaseCol + math.Sin(bb.Phase)*bb.Wobble
		if bb.Col < 0 {
			bb.Col = 0
		}
		if bb.Col >= float64(b.W) {
			bb.Col = float64(b.W - 1)
		}
		if bb.Row <= float64(surfaceRow) {
			life := 14 + int(math.Round(bb.Radius*6))
			b.ripples = append(b.ripples, bogRipple{
				Col:     bb.BaseCol,
				Radius:  0,
				Life:    life,
				MaxLife: life,
			})
			continue
		}
		alive = append(alive, bb)
	}
	b.bubbles = alive
}

func (b *Bog) stepRipplesLocked() {
	if len(b.ripples) == 0 {
		return
	}
	alive := b.ripples[:0]
	for _, r := range b.ripples {
		r.Radius += 0.45
		r.Life--
		if r.Life > 0 && r.Radius < float64(b.W) {
			alive = append(alive, r)
		}
	}
	b.ripples = alive
}

func (b *Bog) surfaceRowLocked() int {
	row := int(math.Round(float64(b.H-1) * b.cfg.Surface))
	if row < 1 {
		row = 1
	}
	if row >= b.H {
		row = b.H - 1
	}
	return row
}

func (b *Bog) paintFrameLocked() {
	for y := range b.Grid {
		for x := range b.Grid[y] {
			b.Grid[y][x] = Pixel{}
		}
	}
	if b.W <= 0 || b.H <= 0 {
		return
	}
	surface := b.surfaceRowLocked()
	b.paintMistLocked(surface)
	b.paintWaterLocked(surface)
	b.paintRipplesLocked(surface)
	b.paintBubblesLocked(surface)
}

func (b *Bog) paintMistLocked(surface int) {
	if surface <= 0 {
		return
	}
	hue := math.Mod(b.cfg.MistHue+360, 360)
	sat := clamp01(b.cfg.Saturation * 0.4)
	for y := 0; y < surface; y++ {
		t := 1.0
		if surface > 1 {
			t = float64(y) / float64(surface-1)
		}
		base := b.cfg.LightMin * (0.6 + 0.4*t)
		glow := b.cfg.Mist * (b.cfg.LightMax - b.cfg.LightMin) * 0.2 * t
		light := clamp01(base + glow)
		c := hslToRGB(hue, sat, light)
		for x := 0; x < b.W; x++ {
			b.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (b *Bog) paintWaterLocked(surface int) {
	if surface >= b.H {
		return
	}
	hue := math.Mod(b.cfg.WaterHue+360, 360)
	sat := clamp01(b.cfg.Saturation)
	depth := math.Max(1, float64(b.H-1-surface))
	for y := surface; y < b.H; y++ {
		d := float64(y-surface) / depth
		baseLight := b.cfg.LightMax*(1-d) + b.cfg.LightMin*d
		baseLight *= 1 - b.cfg.DepthDim*d
		baseLight = clamp01(baseLight)
		shimmer := math.Sin(float64(y)*0.31 + float64(b.tick)*0.05)
		for x := 0; x < b.W; x++ {
			ripple := math.Sin(float64(x)*0.13 + float64(y)*0.09 + float64(b.tick)*0.04)
			light := clamp01(baseLight + (b.cfg.LightMax-b.cfg.LightMin)*0.04*ripple + 0.015*shimmer)
			hueShift := math.Sin(float64(x)*0.07+float64(y)*0.05) * 6
			b.Grid[y][x] = Pixel{Filled: true, C: hslToRGB(math.Mod(hue+hueShift+360, 360), sat, light)}
		}
	}
	// Surface meniscus — a brighter band that gently undulates so the seam
	// between mist and water reads as a water surface rather than a hard line.
	highlight := hslToRGB(math.Mod(hue+12+360, 360), clamp01(sat*0.7), clamp01(b.cfg.LightMax*0.95))
	for x := 0; x < b.W; x++ {
		wave := math.Sin(float64(x)*0.42 + float64(b.tick)*0.07)
		row := surface + int(math.Round(wave*0.4))
		if row < surface {
			row = surface
		}
		if row >= b.H {
			row = b.H - 1
		}
		b.Grid[row][x] = Pixel{Filled: true, C: highlight}
	}
}

func (b *Bog) paintBubblesLocked(surface int) {
	hue := math.Mod(b.cfg.WaterHue+30+360, 360)
	for _, bb := range b.bubbles {
		col := int(math.Round(bb.Col))
		row := int(math.Round(bb.Row))
		if row <= surface || row >= b.H || col < 0 || col >= b.W {
			continue
		}
		body := hslToRGB(hue, clamp01(b.cfg.Saturation*0.55), clamp01(b.cfg.LightMin+(b.cfg.LightMax-b.cfg.LightMin)*0.65))
		paintPixel(b.Grid, col, row, body)
		if bb.Radius >= 1.4 {
			rim := hslToRGB(hue, clamp01(b.cfg.Saturation*0.7), clamp01(b.cfg.LightMax))
			for _, delta := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				x := col + delta[0]
				y := row + delta[1]
				if y <= surface || y >= b.H || x < 0 || x >= b.W {
					continue
				}
				paintPixel(b.Grid, x, y, rim)
			}
		}
		if bb.Radius >= 2.2 {
			pip := hslToRGB(hue, clamp01(b.cfg.Saturation*0.4), clamp01(b.cfg.LightMax*0.98))
			x := col - 1
			y := row - 1
			if y > surface && y < b.H && x >= 0 && x < b.W {
				paintPixel(b.Grid, x, y, pip)
			}
		}
	}
}

func (b *Bog) paintRipplesLocked(surface int) {
	if len(b.ripples) == 0 || surface < 0 || surface >= b.H {
		return
	}
	hue := math.Mod(b.cfg.WaterHue+18+360, 360)
	for _, r := range b.ripples {
		fade := clamp01(float64(r.Life) / math.Max(1, float64(r.MaxLife)))
		if fade <= 0 {
			continue
		}
		light := clamp01(b.cfg.LightMin + (b.cfg.LightMax-b.cfg.LightMin)*(0.4+0.55*fade))
		c := hslToRGB(hue, clamp01(b.cfg.Saturation*0.6), light)
		left := int(math.Round(r.Col - r.Radius))
		right := int(math.Round(r.Col + r.Radius))
		for _, x := range []int{left, right} {
			if x < 0 || x >= b.W {
				continue
			}
			paintPixel(b.Grid, x, surface, c)
		}
	}
}
