package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type dustMote struct {
	Row, Col   float64
	VRow, VCol float64
	Life       int
	MaxLife    int
	Trail      int
	Color      color.RGBA
	Background bool
}

// DustConfig tunes the drifting dust prototype used in isolated dev sessions.
type DustConfig struct {
	// INTRODUCTION
	IntroDur  int     `json:"intro_dur"`
	IntroHaze float64 `json:"intro_haze"`
	IntroPush float64 `json:"intro_push"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
	// LEVERS
	Drift        float64 `json:"drift"`
	Wander       float64 `json:"wander"`
	SpawnEvery   int     `json:"spawn"`
	SpawnBurst   int     `json:"burst"`
	MaxDust      int     `json:"max"`
	Trail        int     `json:"trail"`
	Fade         float64 `json:"fade"`
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// EVENT CHANCES
	GustChance float64 `json:"gust_p"`
	CalmChance float64 `json:"calm_p"`
	// EVENT MODIFIERS
	GustDur   int     `json:"gust_dur"`
	GustMult  float64 `json:"gust_mult"`
	GustFront float64 `json:"gust_front"`
	CalmDur   int     `json:"calm_dur"`
	CalmMult  float64 `json:"calm_mult"`
}

func (c DustConfig) withDefaults() DustConfig {
	if c.IntroDur == 0 && c.IntroHaze == 0 && c.IntroPush == 0 {
		c.IntroDur = 60
		c.IntroHaze = 0.12
		c.IntroPush = 1.5
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 60
		}
		if c.IntroHaze < 0 {
			c.IntroHaze = 0
		}
		if c.IntroPush <= 0 {
			c.IntroPush = 1.5
		}
	}
	c.IntroHaze = clamp01(c.IntroHaze)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingResidue == 0 {
		c.EndingDur = 60
		c.EndingLinger = 20
		c.EndingResidue = 0.08
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 60
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingResidue < 0 {
			c.EndingResidue = 0
		}
	}
	c.EndingResidue = clamp01(c.EndingResidue)
	if c.Drift == 0 {
		c.Drift = 0.45
	}
	if c.Wander <= 0 {
		c.Wander = 0.35
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 4
	}
	if c.SpawnBurst <= 0 {
		c.SpawnBurst = 2
	}
	if c.MaxDust <= 0 {
		c.MaxDust = 56
	}
	if c.Trail <= 0 {
		c.Trail = 3
	}
	if c.Fade <= 0 {
		c.Fade = 0.72
	}
	if c.Hue == 0 {
		c.Hue = 32
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 10
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.35
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.32
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.72
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
	if c.GustDur <= 0 {
		c.GustDur = 50
	}
	if c.GustMult <= 0 {
		c.GustMult = 1.8
	}
	if c.GustFront <= 0 {
		c.GustFront = 18
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 65
	}
	if c.CalmMult <= 0 {
		c.CalmMult = 0.4
	}
	return c
}

// DustSchema describes the Dust effect's tunable knobs for the dev UI.
func DustSchema() EffectSchema {
	return EffectSchema{
		Name: "dust",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent building from thin residue into a readable drifting dust field."},
			{Key: "intro_haze", Label: "intro haze", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.9, Step: 0.02, Default: 0.12,
				Description: "Starting dust-density fraction during the intro before the full field arrives."},
			{Key: "intro_push", Label: "first-hit x", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.5,
				Description: "Strength of the first gust front that establishes motion when intro fires."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent tapering the dust field back toward thin residue."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 20,
				Description: "Extra quiet ticks after the taper so the last streaks can drift out."},
			{Key: "ending_residue", Label: "residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.02, Default: 0.08,
				Description: "How much dust haze remains at the end of the outro instead of dropping to black."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -2.5, Max: 2.5, Step: 0.05, Default: 0.45,
				Description: "Base horizontal carry. Positive drifts right, negative drifts left."},
			{Key: "wander", Label: "wander", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Random jitter in the dust motion. Higher values loosen the field into rougher air."},
			{Key: "spawn", Label: "spawn 1/", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 20, Step: 1, Default: 4,
				Description: "One-in-N roll for a new dust burst each tick while under the particle cap."},
			{Key: "burst", Label: "burst max", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 2,
				Description: "Maximum motes emitted per successful spawn roll."},
			{Key: "max", Label: "max motes", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 4, Max: 160, Step: 1, Default: 56,
				Description: "Maximum number of live dust motes drifting across the field."},
			{Key: "trail", Label: "trail", Slot: SlotLever, Group: "shape", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 3,
				Description: "Pixels painted behind each mote along its current motion vector."},
			{Key: "fade", Label: "fade", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.35, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Brightness multiplier down each dust trail. Lower values give a softer tail."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 80, Step: 1, Default: 32,
				Description: "Base dust hue. Lower values warm toward rust; higher values lean pale straw."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 10,
				Description: "Variation in dust color across the field and haze."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.35,
				Description: "Overall color saturation of the dust."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.32,
				Description: "Minimum lightness used for the haze and dimmer motes."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.72,
				Description: "Maximum lightness used for the brightest dust streaks."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 2,
				Description: "1 = single layer. 2 = adds a dimmer background layer for more atmospheric depth."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.05, Default: 0.45,
				Description: "Fraction of dust motes assigned to the background layer when layers=2."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a stronger gust front sweeping through the field."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter low-density stretch."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 50,
				Description: "Typical gust duration in ticks (jittered by +/-30%)."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.8,
				Description: "Strength multiplier applied to the gust's lateral push."},
			{Key: "gust_front", Label: "front width", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 4, Max: 60, Step: 1, Default: 18,
				Description: "Vertical thickness of the active gust front band."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 65,
				Description: "Duration of the calmer low-density window."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Density multiplier applied while calm is active."},
		},
	}
}

type DustState struct {
	Tick        int     `json:"tick"`
	GustTicks   int     `json:"gustTicks"`
	CalmTicks   int     `json:"calmTicks"`
	GustCenter  float64 `json:"gustCenter"`
	GustPush    float64 `json:"gustPush"`
	IntroTicks  int     `json:"introTicks"`
	IntroTotal  int     `json:"introTotal"`
	EndingTicks int     `json:"endingTicks"`
	EndingTotal int     `json:"endingTotal"`
	EndingFade  int     `json:"endingFade"`
}

type DustMote struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	Life       int     `json:"life"`
	MaxLife    int     `json:"maxLife"`
	Trail      int     `json:"trail"`
	Color      RGB     `json:"color"`
	Background bool    `json:"background"`
}

type DustSnapshot struct {
	DustState
	RNGState uint64     `json:"rngState,omitempty"`
	Motes    []DustMote `json:"motes"`
}

type DustPersistedState struct {
	DustState
	RNGState uint64     `json:"rngState"`
	Motes    []DustMote `json:"motes"`
}

// Dust is a drifting field of horizontal dust motes for isolated dev sessions.
type Dust struct {
	mu sync.Mutex

	W, H  int
	Grid  [][]Pixel
	motes []dustMote
	rng   *rngutil.RNG
	cfg   DustConfig
	tick  int

	gustTicks   int
	calmTicks   int
	gustCenter  float64
	gustPush    float64
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int

	log []LogEntry
}

func NewDust(w, h int, seed int64, cfg DustConfig) *Dust {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Dust{
		W:          w,
		H:          h,
		Grid:       grid,
		rng:        rngutil.New(seed),
		cfg:        cfg.withDefaults(),
		gustCenter: float64(h) * 0.5,
	}
}

func (d *Dust) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if w == d.W && h == d.H {
		return
	}
	d.W = w
	d.H = h
	d.Grid = make([][]Pixel, h)
	for i := range d.Grid {
		d.Grid[i] = make([]Pixel, w)
	}
	if d.gustCenter >= float64(h) {
		d.gustCenter = float64(h) * 0.5
	}
}

func (d *Dust) SetConfig(cfg DustConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	newCfg := cfg.withDefaults()
	if newCfg.Drift != d.cfg.Drift {
		delta := newCfg.Drift - d.cfg.Drift
		for i := range d.motes {
			scale := 1.0
			if d.motes[i].Background {
				scale = 0.72
			}
			d.motes[i].VCol += delta * scale
		}
	}
	d.cfg = newCfg
}

func (d *Dust) EffectiveConfig() DustConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg
}

func (d *Dust) SnapshotState() DustState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snapshotStateLocked()
}

func (d *Dust) Snapshot() DustSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DustSnapshot{
		DustState: d.snapshotStateLocked(),
		RNGState:  d.rng.State(),
		Motes:     d.copyMotesLocked(),
	}
}

func (d *Dust) RestoreState(s DustState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restoreStateLocked(s)
}

func (d *Dust) RestoreSnapshot(s DustSnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restoreStateLocked(s.DustState)
	if s.RNGState != 0 {
		d.rng.SetState(s.RNGState)
	}
	d.restoreMotesLocked(s.Motes)
	d.rebuildGridLocked()
}

func (d *Dust) SnapshotPersistedState() DustPersistedState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DustPersistedState{
		DustState: d.snapshotStateLocked(),
		RNGState:  d.rng.State(),
		Motes:     d.copyMotesLocked(),
	}
}

func (d *Dust) RestorePersistedState(s DustPersistedState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restoreStateLocked(s.DustState)
	if s.RNGState != 0 {
		d.rng.SetState(s.RNGState)
	}
	d.restoreMotesLocked(s.Motes)
	d.rebuildGridLocked()
}

func (d *Dust) CurrentTick() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tick
}

func (d *Dust) PerturbRNG(delta int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rng.Mix(delta)
}

func (d *Dust) TriggerEvent(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch name {
	case "gust":
		d.startGustLocked(d.cfg.GustMult)
		d.appendLog("gust", fmt.Sprintf("triggered (dur=%d, row=%.0f, push=%+.2f)", d.gustTicks, d.gustCenter, d.gustPush))
	case "calm":
		d.calmTicks = jitterInt(d.rng, d.cfg.CalmDur, 0.3)
		d.appendLog("calm", fmt.Sprintf("triggered (dur=%d, x%.2f)", d.calmTicks, d.cfg.CalmMult))
	case "intro":
		d.startIntroductionLocked()
		d.appendLog("intro", fmt.Sprintf("started (dur=%d, haze=%.2f, push=%.2f)", d.introTotal, d.cfg.IntroHaze, d.cfg.IntroPush))
	case "ending":
		d.startEndingLocked()
		d.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d, residue=%.2f)", d.endingFade, d.endingTotal-d.endingFade, d.cfg.EndingResidue))
	default:
		return false
	}
	return true
}

func (d *Dust) DrainLog() []LogEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.log) == 0 {
		return nil
	}
	out := d.log
	d.log = nil
	return out
}

func (d *Dust) appendLog(kind, desc string) {
	d.log = append(d.log, LogEntry{Tick: d.tick, Type: kind, Desc: desc})
	if len(d.log) > 200 {
		d.log = d.log[len(d.log)-200:]
	}
}

func (d *Dust) Step() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tick++
	if d.gustTicks > 0 {
		d.gustTicks--
	} else {
		d.gustPush = 0
	}
	if d.calmTicks > 0 {
		d.calmTicks--
	}
	introActive := d.introTicks > 0
	endingActive := d.endingTicks > 0

	if !introActive && !endingActive {
		if d.gustTicks == 0 && d.rng.Float64() < d.cfg.GustChance {
			d.startGustLocked(d.cfg.GustMult)
			d.appendLog("gust", fmt.Sprintf("started (dur=%d, row=%.0f, push=%+.2f)", d.gustTicks, d.gustCenter, d.gustPush))
		}
		if d.calmTicks == 0 && d.rng.Float64() < d.cfg.CalmChance {
			d.calmTicks = jitterInt(d.rng, d.cfg.CalmDur, 0.3)
			d.appendLog("calm", fmt.Sprintf("started (dur=%d, x%.2f)", d.calmTicks, d.cfg.CalmMult))
		}
	}

	d.clearGridLocked()
	d.paintHazeLocked()
	d.spawnStepLocked()
	d.stepMotesLocked()
	d.paintMotesLocked()

	if introActive {
		d.introTicks--
		if d.introTicks < 0 {
			d.introTicks = 0
		}
	}
	if endingActive {
		d.endingTicks--
		if d.endingTicks < 0 {
			d.endingTicks = 0
		}
	}
}

func (d *Dust) snapshotStateLocked() DustState {
	return DustState{
		Tick:        d.tick,
		GustTicks:   d.gustTicks,
		CalmTicks:   d.calmTicks,
		GustCenter:  d.gustCenter,
		GustPush:    d.gustPush,
		IntroTicks:  d.introTicks,
		IntroTotal:  d.introTotal,
		EndingTicks: d.endingTicks,
		EndingTotal: d.endingTotal,
		EndingFade:  d.endingFade,
	}
}

func (d *Dust) restoreStateLocked(s DustState) {
	d.tick = s.Tick
	d.gustTicks = s.GustTicks
	d.calmTicks = s.CalmTicks
	d.gustCenter = s.GustCenter
	d.gustPush = s.GustPush
	d.introTicks = s.IntroTicks
	d.introTotal = s.IntroTotal
	d.endingTicks = s.EndingTicks
	d.endingTotal = s.EndingTotal
	d.endingFade = s.EndingFade
}

func (d *Dust) copyMotesLocked() []DustMote {
	out := make([]DustMote, len(d.motes))
	for i, m := range d.motes {
		out[i] = DustMote{
			Row:        m.Row,
			Col:        m.Col,
			VRow:       m.VRow,
			VCol:       m.VCol,
			Life:       m.Life,
			MaxLife:    m.MaxLife,
			Trail:      m.Trail,
			Color:      RGB{R: m.Color.R, G: m.Color.G, B: m.Color.B},
			Background: m.Background,
		}
	}
	return out
}

func (d *Dust) restoreMotesLocked(list []DustMote) {
	d.motes = make([]dustMote, len(list))
	for i, m := range list {
		d.motes[i] = dustMote{
			Row:        m.Row,
			Col:        m.Col,
			VRow:       m.VRow,
			VCol:       m.VCol,
			Life:       m.Life,
			MaxLife:    m.MaxLife,
			Trail:      m.Trail,
			Color:      color.RGBA{R: m.Color.R, G: m.Color.G, B: m.Color.B, A: 255},
			Background: m.Background,
		}
	}
}

func (d *Dust) rebuildGridLocked() {
	d.clearGridLocked()
	d.paintHazeLocked()
	d.paintMotesLocked()
}

func (d *Dust) startGustLocked(mult float64) {
	d.gustTicks = jitterInt(d.rng, d.cfg.GustDur, 0.3)
	if d.H > 1 {
		d.gustCenter = d.rng.Float64() * float64(d.H-1)
	} else {
		d.gustCenter = 0
	}
	dir := d.cfg.Drift
	if math.Abs(dir) < 0.05 {
		dir = 0.35
		if d.rng.Float64() < 0.5 {
			dir = -dir
		}
	}
	sign := 1.0
	if dir < 0 {
		sign = -1
	}
	d.gustPush = sign * math.Max(0.18, math.Abs(dir)) * mult * (0.7 + d.rng.Float64()*0.6)
}

func (d *Dust) startIntroductionLocked() {
	d.calmTicks = 0
	d.gustTicks = 0
	d.gustPush = 0
	d.motes = nil
	d.endingTicks = 0
	d.endingTotal = 0
	d.endingFade = 0
	d.introTotal = d.cfg.IntroDur
	if d.introTotal <= 0 {
		d.introTotal = 60
	}
	d.introTicks = d.introTotal
	d.startGustLocked(math.Max(0.2, d.cfg.IntroPush))
}

func (d *Dust) startEndingLocked() {
	d.introTicks = 0
	d.introTotal = 0
	d.calmTicks = 0
	d.gustTicks = 0
	d.gustPush = 0
	d.endingFade = d.cfg.EndingDur
	if d.endingFade <= 0 {
		d.endingFade = 60
	}
	linger := d.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	d.endingTotal = d.endingFade + linger
	if d.endingTotal < 1 {
		d.endingTotal = d.endingFade
	}
	d.endingTicks = d.endingTotal
}

func (d *Dust) densityLevelLocked() float64 {
	level := 1.0
	if d.gustTicks > 0 {
		level *= 1.25
	}
	if d.calmTicks > 0 {
		level *= d.cfg.CalmMult
	}
	if d.introTicks > 0 {
		progress := phaseProgress(d.introTotal, d.introTicks)
		level *= d.cfg.IntroHaze + (1-d.cfg.IntroHaze)*progress
	}
	if d.endingTicks > 0 {
		progress := phaseProgress(d.endingTotal, d.endingTicks)
		level *= 1 - (1-d.cfg.EndingResidue)*progress
	}
	if level < 0.05 {
		level = 0.05
	}
	return level
}

func (d *Dust) gustInfluenceAtRowLocked(row float64) float64 {
	if d.gustTicks <= 0 {
		return 0
	}
	half := math.Max(2, d.cfg.GustFront*0.5)
	dist := math.Abs(row - d.gustCenter)
	if dist >= half {
		return 0
	}
	return 1 - dist/half
}

func (d *Dust) clearGridLocked() {
	for y := range d.Grid {
		for x := range d.Grid[y] {
			d.Grid[y][x] = Pixel{}
		}
	}
}

func (d *Dust) spawnStepLocked() {
	if len(d.motes) >= d.cfg.MaxDust {
		return
	}
	level := d.densityLevelLocked()
	spawnEvery := int(math.Round(float64(d.cfg.SpawnEvery) / math.Max(0.15, level)))
	if spawnEvery < 1 {
		spawnEvery = 1
	}
	attempts := 1
	if level > 1 {
		attempts += int(math.Floor(level))
		if d.rng.Float64() < level-math.Floor(level) {
			attempts++
		}
	}
	for i := 0; i < attempts && len(d.motes) < d.cfg.MaxDust; i++ {
		if d.rng.Intn(spawnEvery) != 0 {
			continue
		}
		burst := 1
		if d.cfg.SpawnBurst > 1 {
			burst = 1 + d.rng.Intn(d.cfg.SpawnBurst)
		}
		if d.gustTicks > 0 && d.rng.Float64() < 0.35 {
			burst++
		}
		for j := 0; j < burst && len(d.motes) < d.cfg.MaxDust; j++ {
			d.spawnMoteLocked()
		}
	}
}

func (d *Dust) spawnRowLocked() float64 {
	if d.H <= 1 {
		return 0
	}
	if d.gustTicks > 0 {
		half := math.Max(2, d.cfg.GustFront*0.5)
		row := d.gustCenter + (d.rng.Float64()*2-1)*half*0.9
		if row < 0 {
			row = 0
		}
		if row > float64(d.H-1) {
			row = float64(d.H - 1)
		}
		return row
	}
	center := float64(d.H)*0.58 + math.Sin(float64(d.tick)*0.017)*float64(d.H)*0.08
	spread := math.Max(3, float64(d.H)*0.28)
	row := center + (d.rng.Float64()*2-1)*spread
	if row < 0 {
		row = 0
	}
	if row > float64(d.H-1) {
		row = float64(d.H - 1)
	}
	return row
}

func (d *Dust) spawnMoteLocked() {
	if d.W <= 0 || d.H <= 0 {
		return
	}
	isBG := d.cfg.Layers >= 2 && d.rng.Float64() < d.cfg.LayerBalance
	row := d.spawnRowLocked()
	gustInfluence := d.gustInfluenceAtRowLocked(row)
	drift := d.cfg.Drift
	if isBG {
		drift *= 0.72
	}
	vCol := drift*(0.7+d.rng.Float64()*0.6) + d.gustPush*gustInfluence*(0.45+d.rng.Float64()*0.25)
	vCol += (d.rng.Float64()*2 - 1) * d.cfg.Wander * 0.05
	vRow := (d.rng.Float64()*2 - 1) * (0.03 + d.cfg.Wander*0.12)
	if isBG {
		vRow *= 0.7
	}
	trail := d.cfg.Trail
	if isBG {
		trail = max(1, trail-1)
	}
	hue := math.Mod(d.cfg.Hue+(d.rng.Float64()*2-1)*d.cfg.HueSpread+360, 360)
	light := d.cfg.LightnessMin + d.rng.Float64()*(d.cfg.LightnessMax-d.cfg.LightnessMin)
	if isBG {
		light *= 0.78
	}
	c := hslToRGB(hue, d.cfg.Saturation, light)
	speed := math.Abs(vCol) + 0.15
	lifeBase := int(math.Round(float64(max(24, d.W)) / math.Max(0.12, speed)))
	if lifeBase < 18 {
		lifeBase = 18
	}
	if lifeBase > 260 {
		lifeBase = 260
	}
	life := jitterInt(d.rng, lifeBase, 0.3)
	edgePad := float64(trail + 2)
	col := d.rng.Float64() * float64(max(1, d.W-1))
	switch {
	case vCol > 0.08:
		col = -edgePad + d.rng.Float64()*edgePad
	case vCol < -0.08:
		col = float64(d.W-1) + d.rng.Float64()*edgePad
	}
	d.motes = append(d.motes, dustMote{
		Row:        row,
		Col:        col,
		VRow:       vRow,
		VCol:       vCol,
		Life:       life,
		MaxLife:    life,
		Trail:      trail,
		Color:      c,
		Background: isBG,
	})
}

func (d *Dust) stepMotesLocked() {
	if len(d.motes) == 0 {
		return
	}
	alive := d.motes[:0]
	for _, m := range d.motes {
		wander := d.cfg.Wander * 0.018
		if m.Background {
			wander *= 0.75
		}
		m.VCol += (d.rng.Float64()*2 - 1) * wander
		m.VRow += (d.rng.Float64()*2 - 1) * wander * 0.8
		target := d.cfg.Drift
		if m.Background {
			target *= 0.72
		}
		if d.calmTicks > 0 {
			target *= 0.88
		}
		if d.gustTicks > 0 {
			influence := d.gustInfluenceAtRowLocked(m.Row)
			target += d.gustPush * (0.55 + 0.45*influence)
			m.VRow += (d.rng.Float64()*2 - 1) * influence * d.cfg.Wander * 0.03
		}
		m.VCol += (target - m.VCol) * 0.18
		if m.VRow > 0.28 {
			m.VRow = 0.28
		}
		if m.VRow < -0.28 {
			m.VRow = -0.28
		}
		maxCol := math.Max(0.18, math.Abs(target)*2.4+0.15)
		if m.Background {
			maxCol *= 0.8
		}
		if m.VCol > maxCol {
			m.VCol = maxCol
		}
		if m.VCol < -maxCol {
			m.VCol = -maxCol
		}
		m.Col += m.VCol
		m.Row += m.VRow
		for m.Row < 0 {
			m.Row += float64(max(1, d.H))
		}
		for m.Row >= float64(d.H) {
			m.Row -= float64(max(1, d.H))
		}
		m.Life--
		if m.Life > 0 && m.Col >= -float64(m.Trail)-2 && m.Col < float64(d.W)+float64(m.Trail)+2 {
			alive = append(alive, m)
		}
	}
	d.motes = alive
}

func (d *Dust) paintHazeLocked() {
	level := d.densityLevelLocked()
	if level <= 0 || d.W <= 0 || d.H <= 0 {
		return
	}
	center := float64(d.H)*0.58 + math.Sin(float64(d.tick)*0.013)*float64(d.H)*0.04
	spread := math.Max(4, float64(d.H)*0.24)
	for y := 0; y < d.H; y++ {
		rowInfluence := 1 - math.Abs(float64(y)-center)/spread
		if rowInfluence <= 0 {
			continue
		}
		gustRow := d.gustInfluenceAtRowLocked(float64(y))
		for x := 0; x < d.W; x++ {
			wave := 0.5 + 0.5*math.Sin(float64(x)*0.09+float64(y)*0.17+float64(d.tick)*0.04)
			strength := rowInfluence * level * (0.02 + 0.05*wave)
			if gustRow > 0 {
				sweep := 0.6 + 0.4*math.Sin(float64(x)*0.12-float64(d.tick)*0.06)
				strength += gustRow * 0.04 * sweep
			}
			if strength < 0.028 {
				continue
			}
			hue := math.Mod(d.cfg.Hue-6+math.Sin(float64(x)*0.03)*d.cfg.HueSpread*0.2+360, 360)
			light := clamp01(d.cfg.LightnessMin * (0.18 + 0.6*strength))
			c := hslToRGB(hue, clamp01(d.cfg.Saturation*0.35), light)
			d.paintMax(y, x, c)
		}
	}
}

func (d *Dust) paintMotesLocked() {
	for _, m := range d.motes {
		fadeLife := clamp01(float64(m.Life) / float64(max(1, m.MaxLife)))
		if fadeLife <= 0 {
			continue
		}
		tail := max(1, m.Trail)
		for i := 0; i < tail; i++ {
			row := m.Row - float64(i)*m.VRow*0.75
			col := m.Col - float64(i)*m.VCol*0.75
			gr := int(math.Round(row))
			gc := int(math.Round(col))
			bright := math.Pow(d.cfg.Fade, float64(i)) * (0.35 + 0.65*fadeLife)
			if m.Background {
				bright *= 0.78
			}
			c := m.Color
			c.R = uint8(float64(c.R) * bright)
			c.G = uint8(float64(c.G) * bright)
			c.B = uint8(float64(c.B) * bright)
			d.paintMax(gr, gc, c)
		}
	}
}

func (d *Dust) paintMax(row, col int, c color.RGBA) {
	if row < 0 || row >= d.H || col < 0 || col >= d.W {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cur := d.Grid[row][col]
	if !cur.Filled {
		d.Grid[row][col] = Pixel{Filled: true, C: c}
		return
	}
	if c.R > cur.C.R {
		cur.C.R = c.R
	}
	if c.G > cur.C.G {
		cur.C.G = c.G
	}
	if c.B > cur.C.B {
		cur.C.B = c.B
	}
	cur.C.A = 255
	cur.Filled = true
	d.Grid[row][col] = cur
}
