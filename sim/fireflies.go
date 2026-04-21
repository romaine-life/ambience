package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type firefly struct {
	Row, Col   float64
	VRow, VCol float64
	Color      color.RGBA
	Phase      float64
	BlinkRate  float64
	background bool
}

// FirefliesConfig tunes the drifting-light simulation used by the Fireflies
// effect. Zero values fall back to sensible defaults via withDefaults.
type FirefliesConfig struct {
	// MOTION
	Drift  float64 `json:"drift"`
	Wander float64 `json:"wander"`
	// DENSITY
	SpawnEvery   int `json:"spawn"`
	MaxFireflies int `json:"max"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// DEPTH
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// EVENT CHANCES
	BlinkBurstChance   float64 `json:"blink_burst_p"`
	ClusterShiftChance float64 `json:"cluster_shift_p"`
	CalmChance         float64 `json:"calm_p"`
	// EVENT MODIFIERS
	BlinkBurstDur   int     `json:"blink_burst_dur"`
	BlinkBurstMult  float64 `json:"blink_burst_mult"`
	ClusterShiftDur int     `json:"cluster_shift_dur"`
	ClusterPull     float64 `json:"cluster_pull"`
	CalmDur         int     `json:"calm_dur"`
}

func (c FirefliesConfig) withDefaults() FirefliesConfig {
	if c.Drift <= 0 {
		c.Drift = 0.18
	}
	if c.Wander <= 0 {
		c.Wander = 0.4
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 3
	}
	if c.MaxFireflies <= 0 {
		c.MaxFireflies = 44
	}
	if c.Hue == 0 {
		c.Hue = 72
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 18
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.55
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.45
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.9
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.Layers <= 0 {
		c.Layers = 2
	}
	if c.LayerBalance <= 0 {
		c.LayerBalance = 0.45
	}
	if c.BlinkBurstDur <= 0 {
		c.BlinkBurstDur = 55
	}
	if c.BlinkBurstMult <= 0 {
		c.BlinkBurstMult = 1.6
	}
	if c.ClusterShiftDur <= 0 {
		c.ClusterShiftDur = 75
	}
	if c.ClusterPull <= 0 {
		c.ClusterPull = 0.65
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 60
	}
	return c
}

// FirefliesSchema describes the Fireflies effect's tunable knobs for the dev UI.
func FirefliesSchema() EffectSchema {
	return EffectSchema{
		Name: "fireflies",
		Knobs: []Knob{
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Base movement speed for drifting fireflies. Higher values make the field feel more active."},
			{Key: "wander", Label: "wander", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.05, Max: 1.0, Step: 0.05, Default: 0.4,
				Description: "Per-step randomness in the movement vector. Higher values produce looser drifting paths."},
			{Key: "spawn", Label: "spawn 1/", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 20, Step: 1, Default: 3,
				Description: "One-in-N chance of adding a new firefly each tick while under the cap."},
			{Key: "max", Label: "max lights", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 4, Max: 120, Step: 1, Default: 44,
				Description: "Maximum number of active fireflies in the field."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 20, Max: 140, Step: 1, Default: 72,
				Description: "Base hue of the glow. Lower values warm toward yellow; higher values lean green."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Variation in glow hue across the firefly field."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Glow saturation. Lower values soften the lights toward pale lantern tones."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.45,
				Description: "Minimum firefly lightness."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.98, Step: 0.01, Default: 0.9,
				Description: "Maximum firefly lightness."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 2,
				Description: "1 = single layer. 2 = brighter foreground and dimmer background drift."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.05, Default: 0.45,
				Description: "Fraction of lights assigned to the background layer."},
			{Key: "blink_burst_p", Label: "blink burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blink-burst",
				Description: "Per-tick chance of a brief field-wide brightness surge."},
			{Key: "cluster_shift_p", Label: "cluster shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "cluster-shift",
				Description: "Per-tick chance of the field gathering toward a new focus point."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter low-activity period."},
			{Key: "blink_burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "blink-burst", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55,
				Description: "Typical blink-burst duration in ticks (jittered by +/-30%)."},
			{Key: "blink_burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "blink-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.6,
				Description: "Brightness multiplier during a blink burst."},
			{Key: "cluster_shift_dur", Label: "cluster dur", Slot: SlotEventMod, Group: "cluster-shift", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 75,
				Description: "How long the current cluster focus stays active."},
			{Key: "cluster_pull", Label: "cluster pull", Slot: SlotEventMod, Group: "cluster-shift", Type: KnobFloat, Min: 0.05, Max: 2, Step: 0.05, Default: 0.65,
				Description: "Strength of the pull toward the active cluster focus."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60,
				Description: "Duration of the quieter low-activity window."},
		},
	}
}

type FirefliesState struct {
	Tick              int     `json:"tick"`
	BlinkBurstTicks   int     `json:"blinkBurstTicks"`
	CalmTicks         int     `json:"calmTicks"`
	ClusterShiftTicks int     `json:"clusterShiftTicks"`
	ClusterRow        float64 `json:"clusterRow"`
	ClusterCol        float64 `json:"clusterCol"`
}

type Firefly struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	Color      RGB     `json:"color"`
	Phase      float64 `json:"phase"`
	BlinkRate  float64 `json:"blinkRate"`
	Background bool    `json:"background"`
}

type FirefliesSnapshot struct {
	FirefliesState
	Fireflies []Firefly `json:"fireflies"`
}

type FirefliesPersistedState struct {
	FirefliesState
	RNGState  uint64     `json:"rngState"`
	Fireflies []Firefly  `json:"fireflies"`
}

// Fireflies is a drifting-light simulation used for the Fireflies effect.
type Fireflies struct {
	mu sync.Mutex

	W, H      int
	Grid      [][]Pixel
	fireflies []firefly
	rng       *rngutil.RNG
	cfg       FirefliesConfig
	tick      int

	blinkBurstTicks   int
	calmTicks         int
	clusterShiftTicks int
	clusterRow        float64
	clusterCol        float64

	log []LogEntry
}

func NewFireflies(w, h int, seed int64, cfg FirefliesConfig) *Fireflies {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	c := cfg.withDefaults()
	return &Fireflies{
		W:          w,
		H:          h,
		Grid:       grid,
		rng:        rngutil.New(seed),
		cfg:        c,
		clusterRow: float64(h) * 0.5,
		clusterCol: float64(w) * 0.5,
	}
}

func (f *Fireflies) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if w == f.W && h == f.H {
		return
	}
	f.W = w
	f.H = h
	f.Grid = make([][]Pixel, h)
	for i := range f.Grid {
		f.Grid[i] = make([]Pixel, w)
	}
	if f.clusterRow > float64(h) {
		f.clusterRow = float64(h) * 0.5
	}
	if f.clusterCol > float64(w) {
		f.clusterCol = float64(w) * 0.5
	}
}

func (f *Fireflies) SetConfig(cfg FirefliesConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	newCfg := cfg.withDefaults()
	if f.cfg.Drift > 0 && newCfg.Drift != f.cfg.Drift {
		ratio := newCfg.Drift / f.cfg.Drift
		for i := range f.fireflies {
			f.fireflies[i].VRow *= ratio
			f.fireflies[i].VCol *= ratio
		}
	}
	f.cfg = newCfg
}

func (f *Fireflies) EffectiveConfig() FirefliesConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

func (f *Fireflies) SnapshotState() FirefliesState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshotStateLocked()
}

func (f *Fireflies) Snapshot() FirefliesSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return FirefliesSnapshot{
		FirefliesState: f.snapshotStateLocked(),
		Fireflies:      f.copyFirefliesLocked(),
	}
}

func (f *Fireflies) RestoreState(s FirefliesState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreStateLocked(s)
}

func (f *Fireflies) RestoreSnapshot(s FirefliesSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreStateLocked(s.FirefliesState)
	f.restoreFirefliesLocked(s.Fireflies)
}

func (f *Fireflies) SnapshotPersistedState() FirefliesPersistedState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return FirefliesPersistedState{
		FirefliesState: f.snapshotStateLocked(),
		RNGState:       f.rng.State(),
		Fireflies:      f.copyFirefliesLocked(),
	}
}

func (f *Fireflies) RestorePersistedState(s FirefliesPersistedState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreStateLocked(s.FirefliesState)
	if s.RNGState != 0 {
		f.rng.SetState(s.RNGState)
	}
	f.restoreFirefliesLocked(s.Fireflies)
}

func (f *Fireflies) FirefliesCopy() []Firefly {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.copyFirefliesLocked()
}

func (f *Fireflies) RestoreFireflies(list []Firefly) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreFirefliesLocked(list)
}

func (f *Fireflies) CurrentTick() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tick
}

func (f *Fireflies) PerturbRNG(delta int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rng.Mix(delta)
}

func (f *Fireflies) TriggerEvent(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch name {
	case "blink-burst":
		f.blinkBurstTicks = jitterInt(f.rng, f.cfg.BlinkBurstDur, 0.3)
		f.appendLog("blink-burst", fmt.Sprintf("triggered (dur=%d, x%.2f)", f.blinkBurstTicks, f.cfg.BlinkBurstMult))
	case "cluster-shift":
		f.clusterShiftTicks = jitterInt(f.rng, f.cfg.ClusterShiftDur, 0.3)
		f.clusterRow = f.rng.Float64() * float64(f.H)
		f.clusterCol = f.rng.Float64() * float64(f.W)
		f.appendLog("cluster-shift", fmt.Sprintf("triggered (dur=%d, row=%.0f col=%.0f)", f.clusterShiftTicks, f.clusterRow, f.clusterCol))
	case "calm":
		f.calmTicks = jitterInt(f.rng, f.cfg.CalmDur, 0.3)
		f.appendLog("calm", fmt.Sprintf("triggered (dur=%d)", f.calmTicks))
	default:
		return false
	}
	return true
}

func (f *Fireflies) DrainLog() []LogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.log) == 0 {
		return nil
	}
	out := f.log
	f.log = nil
	return out
}

func (f *Fireflies) appendLog(kind, desc string) {
	f.log = append(f.log, LogEntry{Tick: f.tick, Type: kind, Desc: desc})
	if len(f.log) > 200 {
		f.log = f.log[len(f.log)-200:]
	}
}

func (f *Fireflies) Step() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tick++

	if f.blinkBurstTicks > 0 {
		f.blinkBurstTicks--
	}
	if f.calmTicks > 0 {
		f.calmTicks--
	}
	if f.clusterShiftTicks > 0 {
		f.clusterShiftTicks--
	}

	if f.blinkBurstTicks == 0 && f.rng.Float64() < f.cfg.BlinkBurstChance {
		f.blinkBurstTicks = jitterInt(f.rng, f.cfg.BlinkBurstDur, 0.3)
		f.appendLog("blink-burst", fmt.Sprintf("started (dur=%d, x%.2f)", f.blinkBurstTicks, f.cfg.BlinkBurstMult))
	}
	if f.clusterShiftTicks == 0 && f.rng.Float64() < f.cfg.ClusterShiftChance {
		f.clusterShiftTicks = jitterInt(f.rng, f.cfg.ClusterShiftDur, 0.3)
		f.clusterRow = f.rng.Float64() * float64(f.H)
		f.clusterCol = f.rng.Float64() * float64(f.W)
		f.appendLog("cluster-shift", fmt.Sprintf("started (dur=%d, row=%.0f col=%.0f)", f.clusterShiftTicks, f.clusterRow, f.clusterCol))
	}
	if f.calmTicks == 0 && f.rng.Float64() < f.cfg.CalmChance {
		f.calmTicks = jitterInt(f.rng, f.cfg.CalmDur, 0.3)
		f.appendLog("calm", fmt.Sprintf("started (dur=%d)", f.calmTicks))
	}

	for y := range f.Grid {
		for x := range f.Grid[y] {
			f.Grid[y][x] = Pixel{}
		}
	}

	spawnEvery := f.cfg.SpawnEvery
	if f.calmTicks > 0 {
		spawnEvery *= 2
	}
	if spawnEvery < 1 {
		spawnEvery = 1
	}
	if len(f.fireflies) < f.cfg.MaxFireflies && f.rng.Intn(spawnEvery) == 0 {
		f.spawnFirefly()
	}

	for i := range f.fireflies {
		ff := &f.fireflies[i]
		f.stepFirefly(ff)
		f.paintFirefly(*ff)
	}
}

func (f *Fireflies) snapshotStateLocked() FirefliesState {
	return FirefliesState{
		Tick:              f.tick,
		BlinkBurstTicks:   f.blinkBurstTicks,
		CalmTicks:         f.calmTicks,
		ClusterShiftTicks: f.clusterShiftTicks,
		ClusterRow:        f.clusterRow,
		ClusterCol:        f.clusterCol,
	}
}

func (f *Fireflies) restoreStateLocked(s FirefliesState) {
	f.tick = s.Tick
	f.blinkBurstTicks = s.BlinkBurstTicks
	f.calmTicks = s.CalmTicks
	f.clusterShiftTicks = s.ClusterShiftTicks
	f.clusterRow = s.ClusterRow
	f.clusterCol = s.ClusterCol
}

func (f *Fireflies) copyFirefliesLocked() []Firefly {
	out := make([]Firefly, len(f.fireflies))
	for i, ff := range f.fireflies {
		out[i] = Firefly{
			Row:        ff.Row,
			Col:        ff.Col,
			VRow:       ff.VRow,
			VCol:       ff.VCol,
			Color:      RGB{R: ff.Color.R, G: ff.Color.G, B: ff.Color.B},
			Phase:      ff.Phase,
			BlinkRate:  ff.BlinkRate,
			Background: ff.background,
		}
	}
	return out
}

func (f *Fireflies) restoreFirefliesLocked(list []Firefly) {
	f.fireflies = make([]firefly, len(list))
	for i, ff := range list {
		f.fireflies[i] = firefly{
			Row:        ff.Row,
			Col:        ff.Col,
			VRow:       ff.VRow,
			VCol:       ff.VCol,
			Color:      color.RGBA{R: ff.Color.R, G: ff.Color.G, B: ff.Color.B, A: 255},
			Phase:      ff.Phase,
			BlinkRate:  ff.BlinkRate,
			background: ff.Background,
		}
	}
}

func (f *Fireflies) stepFirefly(ff *firefly) {
	wander := f.cfg.Wander * 0.02
	ff.VCol += (f.rng.Float64()*2 - 1) * wander
	ff.VRow += (f.rng.Float64()*2 - 1) * wander * 0.7
	maxSpeed := f.cfg.Drift * 2.2
	if ff.background {
		maxSpeed *= 0.7
	}
	if ff.VCol > maxSpeed {
		ff.VCol = maxSpeed
	}
	if ff.VCol < -maxSpeed {
		ff.VCol = -maxSpeed
	}
	if ff.VRow > maxSpeed {
		ff.VRow = maxSpeed
	}
	if ff.VRow < -maxSpeed {
		ff.VRow = -maxSpeed
	}
	if f.clusterShiftTicks > 0 && f.cfg.ClusterPull > 0 {
		ff.VCol += (f.clusterCol - ff.Col) * f.cfg.ClusterPull * 0.0008
		ff.VRow += (f.clusterRow - ff.Row) * f.cfg.ClusterPull * 0.0005
	}
	ff.Col += ff.VCol
	ff.Row += ff.VRow
	for ff.Col < 0 {
		ff.Col += float64(f.W)
	}
	for ff.Col >= float64(f.W) {
		ff.Col -= float64(f.W)
	}
	for ff.Row < 0 {
		ff.Row += float64(f.H)
	}
	for ff.Row >= float64(f.H) {
		ff.Row -= float64(f.H)
	}
	ff.Phase += ff.BlinkRate
}

func (f *Fireflies) paintFirefly(ff firefly) {
	gr := int(math.Round(ff.Row))
	gc := int(math.Round(ff.Col))
	if gr < 0 || gr >= f.H || gc < 0 || gc >= f.W {
		return
	}
	base := (math.Sin(ff.Phase) + 1) * 0.5
	glow := 0.15 + 0.85*base*base
	if f.blinkBurstTicks > 0 {
		glow *= f.cfg.BlinkBurstMult
	}
	if f.calmTicks > 0 {
		glow *= 0.7
	}
	if ff.background {
		glow *= 0.75
	}
	glow = clamp01(glow)
	c := ff.Color
	c.R = uint8(float64(c.R) * glow)
	c.G = uint8(float64(c.G) * glow)
	c.B = uint8(float64(c.B) * glow)
	f.Grid[gr][gc] = Pixel{Filled: true, C: c}
}

func (f *Fireflies) spawnFirefly() {
	isBG := f.cfg.Layers >= 2 && f.rng.Float64() < f.cfg.LayerBalance
	speed := f.cfg.Drift * (0.55 + f.rng.Float64()*0.9)
	if isBG {
		speed *= 0.6
	}
	hue := math.Mod(f.cfg.Hue+(f.rng.Float64()*2-1)*f.cfg.HueSpread+360, 360)
	lightness := f.cfg.LightnessMin + f.rng.Float64()*(f.cfg.LightnessMax-f.cfg.LightnessMin)
	if isBG {
		lightness *= 0.82
	}
	col := hslToRGB(hue, f.cfg.Saturation, lightness)
	f.fireflies = append(f.fireflies, firefly{
		Row:        f.rng.Float64() * float64(f.H),
		Col:        f.rng.Float64() * float64(f.W),
		VRow:       (f.rng.Float64()*2 - 1) * speed * 0.5,
		VCol:       (f.rng.Float64()*2 - 1) * speed,
		Color:      col,
		Phase:      f.rng.Float64() * 2 * math.Pi,
		BlinkRate:  0.04 + f.rng.Float64()*0.07,
		background: isBG,
	})
}
