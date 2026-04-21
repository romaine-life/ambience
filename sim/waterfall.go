package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type waterfallMist struct {
	Row, Col   float64
	VRow, VCol float64
	Life       int
	MaxLife    int
	Color      color.RGBA
}

type waterfallRipple struct {
	Col      float64
	Radius   float64
	Speed    float64
	Life     int
	MaxLife  int
	Strength float64
}

// WaterfallConfig tunes the calm waterfall prototype used in isolated dev sessions.
type WaterfallConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroTrickle float64 `json:"intro_trickle"`
	IntroMist    float64 `json:"intro_mist"`
	// ENDING
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	EndingMist   float64 `json:"ending_mist"`
	// LEVERS
	Width          float64 `json:"width"`
	Wobble         float64 `json:"wobble"`
	Speed          float64 `json:"speed"`
	PoolY          float64 `json:"pool_y"`
	PoolSpan       float64 `json:"pool_span"`
	MistSpawnEvery int     `json:"mist_spawn"`
	MaxMist        int     `json:"mist_max"`
	RippleEvery    int     `json:"ripple_every"`
	MaxRipples     int     `json:"ripple_max"`
	Hue            float64 `json:"hue"`
	HueSpread      float64 `json:"hue_sp"`
	Saturation     float64 `json:"sat"`
	LightnessMin   float64 `json:"lmin"`
	LightnessMax   float64 `json:"lmax"`
	// EVENT CHANCES
	SurgeChance     float64 `json:"surge_p"`
	CalmChance      float64 `json:"calm_p"`
	MistBurstChance float64 `json:"mist_burst_p"`
	// EVENT MODIFIERS
	SurgeDur      int     `json:"surge_dur"`
	SurgeMult     float64 `json:"surge_mult"`
	CalmDur       int     `json:"calm_dur"`
	CalmMult      float64 `json:"calm_mult"`
	MistBurstDur  int     `json:"mist_burst_dur"`
	MistBurstMult float64 `json:"mist_burst_mult"`
}

func (c WaterfallConfig) withDefaults() WaterfallConfig {
	if c.IntroDur == 0 && c.IntroTrickle == 0 && c.IntroMist == 0 {
		c.IntroDur = 60
		c.IntroTrickle = 0.18
		c.IntroMist = 0.25
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 60
		}
		if c.IntroTrickle <= 0 {
			c.IntroTrickle = 0.18
		}
		if c.IntroMist < 0 {
			c.IntroMist = 0
		}
	}
	c.IntroTrickle = clamp01(c.IntroTrickle)
	c.IntroMist = clamp01(c.IntroMist)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingMist == 0 {
		c.EndingDur = 60
		c.EndingLinger = 24
		c.EndingMist = 0.2
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 60
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingMist < 0 {
			c.EndingMist = 0
		}
	}
	c.EndingMist = clamp01(c.EndingMist)
	if c.Width <= 0 {
		c.Width = 7
	}
	if c.Wobble < 0 {
		c.Wobble = 0
	}
	if c.Wobble == 0 {
		c.Wobble = 1.8
	}
	if c.Speed <= 0 {
		c.Speed = 1.0
	}
	if c.PoolY <= 0 {
		c.PoolY = 0.72
	}
	c.PoolY = clamp01(c.PoolY)
	if c.PoolSpan <= 0 {
		c.PoolSpan = 0.34
	}
	c.PoolSpan = clamp01(c.PoolSpan)
	if c.MistSpawnEvery <= 0 {
		c.MistSpawnEvery = 2
	}
	if c.MaxMist <= 0 {
		c.MaxMist = 48
	}
	if c.RippleEvery <= 0 {
		c.RippleEvery = 8
	}
	if c.MaxRipples <= 0 {
		c.MaxRipples = 10
	}
	if c.Hue == 0 {
		c.Hue = 204
	}
	if c.HueSpread <= 0 {
		c.HueSpread = 12
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.48
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.45
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.82
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.SurgeDur <= 0 {
		c.SurgeDur = 55
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 1.6
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 70
	}
	if c.CalmMult <= 0 {
		c.CalmMult = 0.55
	}
	if c.MistBurstDur <= 0 {
		c.MistBurstDur = 40
	}
	if c.MistBurstMult <= 0 {
		c.MistBurstMult = 2.5
	}
	return c
}

// WaterfallSchema describes the Waterfall effect's tunable knobs for the dev UI.
func WaterfallSchema() EffectSchema {
	return EffectSchema{
		Name: "waterfall",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent ramping from a trickle into the full waterfall sheet."},
			{Key: "intro_trickle", Label: "intro trickle", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.05, Default: 0.18,
				Description: "Starting flow fraction during the intro before the sheet reaches full width."},
			{Key: "intro_mist", Label: "intro mist", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.25,
				Description: "How much mist is already present at the beginning of the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent tapering the sheet back down toward a trickle."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 24,
				Description: "Extra quiet ticks for ripples and mist to settle after the sheet weakens."},
			{Key: "ending_mist", Label: "ending mist", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.2,
				Description: "Fraction of mist that remains near the end of the outro."},
			{Key: "width", Label: "sheet width", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 2, Max: 24, Step: 0.5, Default: 7,
				Description: "Base width of the waterfall sheet before events widen or narrow it."},
			{Key: "wobble", Label: "sheet wobble", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0, Max: 5, Step: 0.1, Default: 1.8,
				Description: "Side-to-side shimmer along the falling sheet."},
			{Key: "speed", Label: "flow speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.2, Max: 2.5, Step: 0.05, Default: 1,
				Description: "How quickly the sheet shimmer, mist rise, and pool accents move."},
			{Key: "pool_y", Label: "pool height", Slot: SlotLever, Group: "pool", Type: KnobFloat, Min: 0.45, Max: 0.9, Step: 0.01, Default: 0.72,
				Description: "Vertical position of the pool surface as a fraction of the screen height."},
			{Key: "pool_span", Label: "pool span", Slot: SlotLever, Group: "pool", Type: KnobFloat, Min: 0.12, Max: 0.75, Step: 0.01, Default: 0.34,
				Description: "Horizontal span of the visible pool beneath the waterfall."},
			{Key: "mist_spawn", Label: "mist 1/", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 1, Max: 12, Step: 1, Default: 2,
				Description: "One-in-N roll for new mist particles around the impact zone."},
			{Key: "mist_max", Label: "max mist", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 4, Max: 120, Step: 1, Default: 48,
				Description: "Maximum live mist particles drifting near the pool."},
			{Key: "ripple_every", Label: "ripple every", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 8,
				Description: "Typical cadence between pool ripple beats, in ticks."},
			{Key: "ripple_max", Label: "max ripples", Slot: SlotLever, Group: "accents", Type: KnobInt, Min: 1, Max: 24, Step: 1, Default: 10,
				Description: "Maximum live ripple fronts expanding across the pool."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 240, Step: 1, Default: 204,
				Description: "Base hue of the water and mist accents."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 12,
				Description: "Variation in hue across the sheet and mist accents."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.48,
				Description: "Overall color saturation of the water scene."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.45,
				Description: "Minimum lightness used for the pool body and darker parts of the sheet."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.98, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for highlights, foam, and bright mist."},
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surge",
				Description: "Per-tick chance of a broader, louder surge in the waterfall sheet."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the falls easing down into a quieter period."},
			{Key: "mist_burst_p", Label: "mist burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "mist-burst",
				Description: "Per-tick chance of the impact zone throwing off extra mist."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 55,
				Description: "Typical surge duration in ticks (jittered by +/-30%)."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.6,
				Description: "Flow multiplier applied while a surge is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70,
				Description: "Duration of a quieter, thinner waterfall period."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Flow multiplier applied while calm is active."},
			{Key: "mist_burst_dur", Label: "mist dur", Slot: SlotEventMod, Group: "mist-burst", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 40,
				Description: "Duration of the extra-mist burst event."},
			{Key: "mist_burst_mult", Label: "mist x", Slot: SlotEventMod, Group: "mist-burst", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 2.5,
				Description: "Mist density multiplier while a burst is active."},
		},
	}
}

type WaterfallState struct {
	Tick           int `json:"tick"`
	SurgeTicks     int `json:"surgeTicks"`
	CalmTicks      int `json:"calmTicks"`
	MistBurstTicks int `json:"mistBurstTicks"`
	IntroTicks     int `json:"introTicks"`
	IntroTotal     int `json:"introTotal"`
	EndingTicks    int `json:"endingTicks"`
	EndingTotal    int `json:"endingTotal"`
	EndingFade     int `json:"endingFade"`
	RippleCooldown int `json:"rippleCooldown"`
}

type WaterfallMist struct {
	Row     float64 `json:"row"`
	Col     float64 `json:"col"`
	VRow    float64 `json:"vRow"`
	VCol    float64 `json:"vCol"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
	Color   RGB     `json:"color"`
}

type WaterfallRipple struct {
	Col      float64 `json:"col"`
	Radius   float64 `json:"radius"`
	Speed    float64 `json:"speed"`
	Life     int     `json:"life"`
	MaxLife  int     `json:"maxLife"`
	Strength float64 `json:"strength"`
}

type WaterfallSnapshot struct {
	WaterfallState
	Mists   []WaterfallMist   `json:"mists"`
	Ripples []WaterfallRipple `json:"ripples"`
}

type WaterfallPersistedState struct {
	WaterfallState
	RNGState uint64            `json:"rngState"`
	Mists    []WaterfallMist   `json:"mists"`
	Ripples  []WaterfallRipple `json:"ripples"`
}

// Waterfall is a serene scenic waterfall prototype for isolated dev sessions.
type Waterfall struct {
	mu sync.Mutex

	W, H    int
	Grid    [][]Pixel
	mists   []waterfallMist
	ripples []waterfallRipple
	rng     *rngutil.RNG
	cfg     WaterfallConfig
	tick    int

	surgeTicks     int
	calmTicks      int
	mistBurstTicks int
	introTicks     int
	introTotal     int
	endingTicks    int
	endingTotal    int
	endingFade     int
	rippleCooldown int

	log []LogEntry
}

func NewWaterfall(w, h int, seed int64, cfg WaterfallConfig) *Waterfall {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Waterfall{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
}

func (w *Waterfall) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if width == w.W && height == w.H {
		return
	}
	w.W = width
	w.H = height
	w.Grid = make([][]Pixel, height)
	for i := range w.Grid {
		w.Grid[i] = make([]Pixel, width)
	}
}

func (w *Waterfall) SetConfig(cfg WaterfallConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()
	newCfg := cfg.withDefaults()
	if w.cfg.Speed > 0 && newCfg.Speed != w.cfg.Speed {
		ratio := newCfg.Speed / w.cfg.Speed
		for i := range w.mists {
			w.mists[i].VRow *= ratio
			w.mists[i].VCol *= ratio
		}
		for i := range w.ripples {
			w.ripples[i].Speed *= 0.7 + 0.3*ratio
		}
	}
	w.cfg = newCfg
}

func (w *Waterfall) EffectiveConfig() WaterfallConfig {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cfg
}

func (w *Waterfall) SnapshotState() WaterfallState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.snapshotStateLocked()
}

func (w *Waterfall) Snapshot() WaterfallSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WaterfallSnapshot{
		WaterfallState: w.snapshotStateLocked(),
		Mists:          w.copyMistsLocked(),
		Ripples:        w.copyRipplesLocked(),
	}
}

func (w *Waterfall) RestoreState(s WaterfallState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restoreStateLocked(s)
}

func (w *Waterfall) RestoreSnapshot(s WaterfallSnapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restoreStateLocked(s.WaterfallState)
	w.restoreMistsLocked(s.Mists)
	w.restoreRipplesLocked(s.Ripples)
}

func (w *Waterfall) SnapshotPersistedState() WaterfallPersistedState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WaterfallPersistedState{
		WaterfallState: w.snapshotStateLocked(),
		RNGState:       w.rng.State(),
		Mists:          w.copyMistsLocked(),
		Ripples:        w.copyRipplesLocked(),
	}
}

func (w *Waterfall) RestorePersistedState(s WaterfallPersistedState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restoreStateLocked(s.WaterfallState)
	if s.RNGState != 0 {
		w.rng.SetState(s.RNGState)
	}
	w.restoreMistsLocked(s.Mists)
	w.restoreRipplesLocked(s.Ripples)
}

func (w *Waterfall) CurrentTick() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tick
}

func (w *Waterfall) PerturbRNG(delta int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rng.Mix(delta)
}

func (w *Waterfall) TriggerEvent(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch name {
	case "surge":
		w.surgeTicks = jitterInt(w.rng, w.cfg.SurgeDur, 0.3)
		w.spawnRippleLocked(w.flowLevelLocked())
		w.appendLog("surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", w.surgeTicks, w.cfg.SurgeMult))
	case "calm":
		w.calmTicks = jitterInt(w.rng, w.cfg.CalmDur, 0.3)
		w.appendLog("calm", fmt.Sprintf("triggered (dur=%d, x%.2f)", w.calmTicks, w.cfg.CalmMult))
	case "mist-burst":
		w.mistBurstTicks = jitterInt(w.rng, w.cfg.MistBurstDur, 0.3)
		w.spawnRippleLocked(w.flowLevelLocked())
		w.appendLog("mist-burst", fmt.Sprintf("triggered (dur=%d, x%.2f)", w.mistBurstTicks, w.cfg.MistBurstMult))
	case "intro":
		w.startIntroductionLocked()
		w.appendLog("intro", fmt.Sprintf("started (dur=%d, trickle=%.2f)", w.introTotal, w.cfg.IntroTrickle))
	case "ending":
		w.startEndingLocked()
		w.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", w.endingFade, w.endingTotal-w.endingFade))
	default:
		return false
	}
	return true
}

func (w *Waterfall) DrainLog() []LogEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.log) == 0 {
		return nil
	}
	out := w.log
	w.log = nil
	return out
}

func (w *Waterfall) appendLog(kind, desc string) {
	w.log = append(w.log, LogEntry{Tick: w.tick, Type: kind, Desc: desc})
	if len(w.log) > 200 {
		w.log = w.log[len(w.log)-200:]
	}
}

func (w *Waterfall) Step() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tick++
	if w.surgeTicks > 0 {
		w.surgeTicks--
	}
	if w.calmTicks > 0 {
		w.calmTicks--
	}
	if w.mistBurstTicks > 0 {
		w.mistBurstTicks--
	}
	if w.introTicks > 0 {
		w.introTicks--
	}
	if w.endingTicks > 0 {
		w.endingTicks--
	}
	if w.rippleCooldown > 0 {
		w.rippleCooldown--
	}

	if w.surgeTicks == 0 && w.rng.Float64() < w.cfg.SurgeChance {
		w.surgeTicks = jitterInt(w.rng, w.cfg.SurgeDur, 0.3)
		w.spawnRippleLocked(w.flowLevelLocked())
		w.appendLog("surge", fmt.Sprintf("started (dur=%d, x%.2f)", w.surgeTicks, w.cfg.SurgeMult))
	}
	if w.calmTicks == 0 && w.rng.Float64() < w.cfg.CalmChance {
		w.calmTicks = jitterInt(w.rng, w.cfg.CalmDur, 0.3)
		w.appendLog("calm", fmt.Sprintf("started (dur=%d, x%.2f)", w.calmTicks, w.cfg.CalmMult))
	}
	if w.mistBurstTicks == 0 && w.rng.Float64() < w.cfg.MistBurstChance {
		w.mistBurstTicks = jitterInt(w.rng, w.cfg.MistBurstDur, 0.3)
		w.spawnRippleLocked(w.flowLevelLocked())
		w.appendLog("mist-burst", fmt.Sprintf("started (dur=%d, x%.2f)", w.mistBurstTicks, w.cfg.MistBurstMult))
	}

	w.stepMistsLocked()
	w.stepRipplesLocked()
	w.stepRippleSpawnerLocked()
	w.stepMistSpawnerLocked()
	w.clearGridLocked()
	w.paintPoolLocked()
	w.paintSheetLocked()
	w.paintImpactLocked()
	w.paintRipplesLocked()
	w.paintMistsLocked()
}

func (w *Waterfall) snapshotStateLocked() WaterfallState {
	return WaterfallState{
		Tick:           w.tick,
		SurgeTicks:     w.surgeTicks,
		CalmTicks:      w.calmTicks,
		MistBurstTicks: w.mistBurstTicks,
		IntroTicks:     w.introTicks,
		IntroTotal:     w.introTotal,
		EndingTicks:    w.endingTicks,
		EndingTotal:    w.endingTotal,
		EndingFade:     w.endingFade,
		RippleCooldown: w.rippleCooldown,
	}
}

func (w *Waterfall) restoreStateLocked(s WaterfallState) {
	w.tick = s.Tick
	w.surgeTicks = s.SurgeTicks
	w.calmTicks = s.CalmTicks
	w.mistBurstTicks = s.MistBurstTicks
	w.introTicks = s.IntroTicks
	w.introTotal = s.IntroTotal
	w.endingTicks = s.EndingTicks
	w.endingTotal = s.EndingTotal
	w.endingFade = s.EndingFade
	w.rippleCooldown = s.RippleCooldown
}

func (w *Waterfall) copyMistsLocked() []WaterfallMist {
	out := make([]WaterfallMist, len(w.mists))
	for i, m := range w.mists {
		out[i] = WaterfallMist{
			Row:     m.Row,
			Col:     m.Col,
			VRow:    m.VRow,
			VCol:    m.VCol,
			Life:    m.Life,
			MaxLife: m.MaxLife,
			Color:   RGB{R: m.Color.R, G: m.Color.G, B: m.Color.B},
		}
	}
	return out
}

func (w *Waterfall) restoreMistsLocked(list []WaterfallMist) {
	w.mists = make([]waterfallMist, len(list))
	for i, m := range list {
		w.mists[i] = waterfallMist{
			Row:     m.Row,
			Col:     m.Col,
			VRow:    m.VRow,
			VCol:    m.VCol,
			Life:    m.Life,
			MaxLife: m.MaxLife,
			Color:   color.RGBA{R: m.Color.R, G: m.Color.G, B: m.Color.B, A: 255},
		}
	}
}

func (w *Waterfall) copyRipplesLocked() []WaterfallRipple {
	out := make([]WaterfallRipple, len(w.ripples))
	for i, r := range w.ripples {
		out[i] = WaterfallRipple{
			Col:      r.Col,
			Radius:   r.Radius,
			Speed:    r.Speed,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
		}
	}
	return out
}

func (w *Waterfall) restoreRipplesLocked(list []WaterfallRipple) {
	w.ripples = make([]waterfallRipple, len(list))
	for i, r := range list {
		w.ripples[i] = waterfallRipple{
			Col:      r.Col,
			Radius:   r.Radius,
			Speed:    r.Speed,
			Life:     r.Life,
			MaxLife:  r.MaxLife,
			Strength: r.Strength,
		}
	}
}

func (w *Waterfall) startIntroductionLocked() {
	w.endingTicks = 0
	w.endingTotal = 0
	w.endingFade = 0
	w.introTotal = w.cfg.IntroDur
	if w.introTotal <= 0 {
		w.introTotal = 60
	}
	w.introTicks = w.introTotal
	w.rippleCooldown = 1
}

func (w *Waterfall) startEndingLocked() {
	w.introTicks = 0
	w.introTotal = 0
	w.endingFade = w.cfg.EndingDur
	if w.endingFade <= 0 {
		w.endingFade = 60
	}
	linger := w.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	w.endingTotal = w.endingFade + linger
	if w.endingTotal < 1 {
		w.endingTotal = w.endingFade
	}
	w.endingTicks = w.endingTotal
}

func (w *Waterfall) flowLevelLocked() float64 {
	flow := 1.0
	if w.surgeTicks > 0 {
		flow *= w.cfg.SurgeMult
	}
	if w.calmTicks > 0 {
		flow *= w.cfg.CalmMult
	}
	if w.introTicks > 0 {
		progress := phaseProgress(w.introTotal, w.introTicks)
		flow *= w.cfg.IntroTrickle + (1-w.cfg.IntroTrickle)*progress
	}
	if w.endingTicks > 0 {
		elapsed := w.endingTotal - w.endingTicks
		if elapsed < w.endingFade {
			fade := clamp01(float64(elapsed) / float64(max(1, w.endingFade-1)))
			flow *= 1 - 0.88*fade
		} else {
			flow *= 0.12
		}
	}
	if flow < 0.05 {
		flow = 0.05
	}
	return flow
}

func (w *Waterfall) mistLevelLocked() float64 {
	level := 1.0
	if w.surgeTicks > 0 {
		level *= 1.25
	}
	if w.calmTicks > 0 {
		level *= 0.65
	}
	if w.mistBurstTicks > 0 {
		level *= w.cfg.MistBurstMult
	}
	if w.introTicks > 0 {
		progress := phaseProgress(w.introTotal, w.introTicks)
		level *= w.cfg.IntroMist + (1-w.cfg.IntroMist)*progress
	}
	if w.endingTicks > 0 {
		progress := phaseProgress(w.endingTotal, w.endingTicks)
		level *= 1 - (1-w.cfg.EndingMist)*progress
	}
	if level < 0.05 {
		level = 0.05
	}
	return level
}

func (w *Waterfall) poolRowLocked() int {
	if w.H <= 0 {
		return 0
	}
	row := int(math.Round(w.cfg.PoolY * float64(w.H-1)))
	if row < 6 {
		row = 6
	}
	if row > w.H-4 {
		row = w.H - 4
	}
	return row
}

func (w *Waterfall) poolBoundsLocked() (int, int) {
	center := int(math.Round(float64(w.W) * 0.5))
	half := int(math.Round(w.cfg.PoolSpan * float64(w.W) * 0.5))
	if half < 4 {
		half = 4
	}
	left := center - half
	right := center + half
	if left < 0 {
		left = 0
	}
	if right >= w.W {
		right = w.W - 1
	}
	return left, right
}

func (w *Waterfall) clearGridLocked() {
	for y := range w.Grid {
		for x := range w.Grid[y] {
			w.Grid[y][x] = Pixel{}
		}
	}
}

func (w *Waterfall) stepMistsLocked() {
	if len(w.mists) == 0 {
		return
	}
	speedScale := 0.75 + 0.4*w.cfg.Speed
	alive := w.mists[:0]
	for _, m := range w.mists {
		m.VCol += (w.rng.Float64()*2 - 1) * 0.015
		if m.VCol > 0.4 {
			m.VCol = 0.4
		}
		if m.VCol < -0.4 {
			m.VCol = -0.4
		}
		m.Row += m.VRow * speedScale
		m.Col += m.VCol
		m.VRow *= 0.99
		m.Life--
		if m.Life > 0 && m.Row >= -2 && m.Row < float64(w.H) && m.Col >= -2 && m.Col < float64(w.W)+2 {
			alive = append(alive, m)
		}
	}
	w.mists = alive
}

func (w *Waterfall) stepRipplesLocked() {
	if len(w.ripples) == 0 {
		return
	}
	alive := w.ripples[:0]
	for _, r := range w.ripples {
		r.Radius += r.Speed
		r.Life--
		if r.Life > 0 && r.Radius < float64(w.W) {
			alive = append(alive, r)
		}
	}
	w.ripples = alive
}

func (w *Waterfall) stepRippleSpawnerLocked() {
	if len(w.ripples) >= w.cfg.MaxRipples {
		return
	}
	if w.rippleCooldown > 0 {
		return
	}
	flow := w.flowLevelLocked()
	cadence := float64(w.cfg.RippleEvery)
	if flow > 0 {
		cadence /= math.Max(0.25, flow)
	}
	if w.endingTicks > 0 && w.endingTotal-w.endingTicks >= w.endingFade {
		cadence *= 2
	}
	if cadence < 1 {
		cadence = 1
	}
	w.spawnRippleLocked(flow)
	w.rippleCooldown = jitterInt(w.rng, int(math.Round(cadence)), 0.25)
}

func (w *Waterfall) stepMistSpawnerLocked() {
	if len(w.mists) >= w.cfg.MaxMist {
		return
	}
	level := w.mistLevelLocked()
	spawnEvery := int(math.Round(float64(w.cfg.MistSpawnEvery) / math.Max(0.2, level)))
	if spawnEvery < 1 {
		spawnEvery = 1
	}
	attempts := 1
	if level > 1 {
		attempts += int(math.Floor(level))
		if w.rng.Float64() < level-math.Floor(level) {
			attempts++
		}
	}
	if w.endingTicks > 0 && w.endingTotal-w.endingTicks >= w.endingFade {
		spawnEvery *= 3
		attempts = 1
	}
	for i := 0; i < attempts && len(w.mists) < w.cfg.MaxMist; i++ {
		if w.rng.Intn(spawnEvery) == 0 {
			w.spawnMistLocked(level)
		}
	}
}

func (w *Waterfall) spawnRippleLocked(flow float64) {
	if len(w.ripples) >= w.cfg.MaxRipples {
		return
	}
	center := float64(w.W) * 0.5
	col := center + (w.rng.Float64()*2-1)*w.cfg.Width*math.Max(0.35, flow)*0.35
	life := jitterInt(w.rng, 18, 0.25)
	speed := (0.5 + w.rng.Float64()*0.55) * (0.8 + 0.25*math.Max(0.5, flow))
	strength := clamp01(0.45 + w.rng.Float64()*0.35 + (flow-1)*0.12)
	w.ripples = append(w.ripples, waterfallRipple{
		Col:      col,
		Radius:   0,
		Speed:    speed,
		Life:     life,
		MaxLife:  life,
		Strength: strength,
	})
}

func (w *Waterfall) spawnMistLocked(level float64) {
	if len(w.mists) >= w.cfg.MaxMist {
		return
	}
	center := float64(w.W) * 0.5
	flow := w.flowLevelLocked()
	surface := float64(w.poolRowLocked())
	col := center + (w.rng.Float64()*2-1)*w.cfg.Width*math.Max(0.35, flow)*0.6
	row := surface - 1 - w.rng.Float64()*2
	vRow := -(0.12 + w.rng.Float64()*0.22) * (0.8 + 0.35*w.cfg.Speed)
	vCol := (w.rng.Float64()*2 - 1) * (0.08 + 0.1*math.Max(0.5, level) + w.cfg.Wobble*0.02)
	life := jitterInt(w.rng, 22, 0.35)
	hue := math.Mod(w.cfg.Hue+(w.rng.Float64()*2-1)*w.cfg.HueSpread*0.35+360, 360)
	light := clamp01(w.cfg.LightnessMax * (0.88 + w.rng.Float64()*0.12))
	colr := hslToRGB(hue, clamp01(w.cfg.Saturation*0.45), light)
	w.mists = append(w.mists, waterfallMist{
		Row:     row,
		Col:     col,
		VRow:    vRow,
		VCol:    vCol,
		Life:    life,
		MaxLife: life,
		Color:   colr,
	})
}

func (w *Waterfall) paintPoolLocked() {
	surface := w.poolRowLocked()
	left, right := w.poolBoundsLocked()
	depth := w.H - surface
	if depth > 10 {
		depth = 10
	}
	if depth < 3 {
		depth = 3
	}
	center := float64(w.W) * 0.5
	half := math.Max(1, float64(right-left)/2)
	for y := surface; y < w.H && y < surface+depth; y++ {
		rowDepth := 1 - float64(y-surface)/float64(depth)
		for x := left; x <= right; x++ {
			edge := 1 - math.Abs(float64(x)-center)/half
			if edge <= 0 {
				continue
			}
			shimmer := 0.72 + 0.28*math.Sin(float64(x)*0.13+float64(y)*0.27+float64(w.tick)*0.07*w.cfg.Speed)
			light := clamp01(w.cfg.LightnessMin*0.22 + (w.cfg.LightnessMax-w.cfg.LightnessMin)*0.28*edge*rowDepth*shimmer)
			c := hslToRGB(math.Mod(w.cfg.Hue-8+360, 360), clamp01(w.cfg.Saturation*0.9), light)
			w.paintMax(y, x, c)
		}
	}
}

func (w *Waterfall) paintSheetLocked() {
	surface := w.poolRowLocked()
	if surface <= 0 {
		return
	}
	center := float64(w.W) * 0.5
	flow := w.flowLevelLocked()
	width := math.Max(1, w.cfg.Width*flow)
	for y := 0; y < surface; y++ {
		progress := float64(y) / float64(max(1, surface-1))
		// Let the sheet's bend drift downward so the wobble reinforces the fall.
		rowCenter := center + math.Sin(progress*5.1-float64(w.tick)*0.05*w.cfg.Speed)*w.cfg.Wobble*0.55
		rowWidth := width * (0.86 + 0.32*progress)
		half := math.Max(0.6, rowWidth*0.5)
		start := int(math.Floor(rowCenter - half - 1))
		end := int(math.Ceil(rowCenter + half + 1))
		if start < 0 {
			start = 0
		}
		if end >= w.W {
			end = w.W - 1
		}
		for x := start; x <= end; x++ {
			dist := math.Abs((float64(x)+0.5)-rowCenter) / half
			if dist > 1.1 {
				continue
			}
			edge := clamp01(1 - dist*dist)
			pulse := 0.72 + 0.28*math.Sin(progress*11-float64(w.tick)*0.22*w.cfg.Speed+float64(x)*0.35)
			intensity := edge * pulse
			if intensity < 0.08 {
				continue
			}
			hue := math.Mod(w.cfg.Hue+math.Sin(progress*3+float64(x)*0.1)*w.cfg.HueSpread+360, 360)
			light := clamp01(w.cfg.LightnessMin + (w.cfg.LightnessMax-w.cfg.LightnessMin)*(0.3+0.7*intensity))
			c := hslToRGB(hue, w.cfg.Saturation, light)
			w.paintMax(y, x, c)
		}
	}
}

func (w *Waterfall) paintImpactLocked() {
	surface := w.poolRowLocked()
	center := int(math.Round(float64(w.W) * 0.5))
	flow := w.flowLevelLocked()
	level := w.mistLevelLocked()
	radius := int(math.Round(math.Max(2, w.cfg.Width*flow*0.6)))
	for dx := -radius; dx <= radius; dx++ {
		x := center + dx
		dist := math.Abs(float64(dx)) / float64(radius+1)
		if dist > 1 {
			continue
		}
		foam := clamp01((1 - dist*dist) * (0.65 + 0.2*math.Max(0.5, level)))
		light := clamp01(w.cfg.LightnessMin + (w.cfg.LightnessMax-w.cfg.LightnessMin)*(0.55+0.45*foam))
		c := hslToRGB(math.Mod(w.cfg.Hue-16+360, 360), clamp01(w.cfg.Saturation*0.25), light)
		w.paintMax(surface, x, c)
		w.paintMax(surface-1, x, c)
		if surface+1 < w.H && dx%2 == 0 {
			dim := c
			dim.R = uint8(float64(dim.R) * 0.8)
			dim.G = uint8(float64(dim.G) * 0.8)
			dim.B = uint8(float64(dim.B) * 0.8)
			w.paintMax(surface+1, x, dim)
		}
	}
}

func (w *Waterfall) paintRipplesLocked() {
	if len(w.ripples) == 0 {
		return
	}
	surface := w.poolRowLocked()
	left, right := w.poolBoundsLocked()
	for _, r := range w.ripples {
		fade := clamp01(float64(r.Life) / float64(max(1, r.MaxLife)))
		if fade <= 0 {
			continue
		}
		for x := left; x <= right; x++ {
			wave := math.Abs(math.Abs(float64(x)-r.Col) - r.Radius)
			if wave > 0.8 {
				continue
			}
			bright := r.Strength * fade * (1 - wave/0.8)
			light := clamp01(w.cfg.LightnessMin*0.85 + (w.cfg.LightnessMax-w.cfg.LightnessMin)*(0.25+0.55*bright))
			c := hslToRGB(math.Mod(w.cfg.Hue-10+360, 360), clamp01(w.cfg.Saturation*0.7), light)
			w.paintMax(surface, x, c)
			if surface+1 < w.H && bright > 0.45 {
				dim := c
				dim.R = uint8(float64(dim.R) * 0.75)
				dim.G = uint8(float64(dim.G) * 0.75)
				dim.B = uint8(float64(dim.B) * 0.75)
				w.paintMax(surface+1, x, dim)
			}
		}
	}
}

func (w *Waterfall) paintMistsLocked() {
	for _, m := range w.mists {
		fade := clamp01(float64(m.Life) / float64(max(1, m.MaxLife)))
		if fade <= 0 {
			continue
		}
		row := int(math.Round(m.Row))
		col := int(math.Round(m.Col))
		c := m.Color
		scale := 0.25 + 0.75*fade
		c.R = uint8(float64(c.R) * scale)
		c.G = uint8(float64(c.G) * scale)
		c.B = uint8(float64(c.B) * scale)
		w.paintMax(row, col, c)
		if fade > 0.7 {
			side := col
			if m.VCol >= 0 {
				side++
			} else {
				side--
			}
			dim := c
			dim.R = uint8(float64(dim.R) * 0.65)
			dim.G = uint8(float64(dim.G) * 0.65)
			dim.B = uint8(float64(dim.B) * 0.65)
			w.paintMax(row, side, dim)
		}
	}
}

func (w *Waterfall) paintMax(row, col int, c color.RGBA) {
	if row < 0 || row >= w.H || col < 0 || col >= w.W {
		return
	}
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cur := w.Grid[row][col]
	if !cur.Filled {
		w.Grid[row][col] = Pixel{Filled: true, C: c}
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
	w.Grid[row][col] = cur
}
