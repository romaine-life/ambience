package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// RainOnWindow is the contemplative "rain seen through a windowpane" prototype.
// Tiny droplets nucleate on the glass, slowly grow, then track downward once
// they exceed a critical mass — leaving thinning streaks behind. A diffuse
// glow band behind the pane carries the warm-interior vs. cool-city palette.
//
// The server is authoritative about lifecycle phases (intro/ending) and
// discrete event windows (wind-gust, quiet-pane). Per-droplet physics is
// shared between authority and clients via snapshots so a join-in-progress
// dev session sees roughly the same droplet positions as the server.

type windowDroplet struct {
	Row, Col float64
	Radius   float64
	VRow     float64
	VCol     float64
	Hue      float64
	PinRow   float64 // origin Y when this drop started falling (0 if not falling)
	Falling  bool
}

type windowTrack struct {
	Row, Col float64
	Strength float64
	Life     int
	MaxLife  int
}

// RainOnWindowConfig tunes the rain-on-window prototype used in isolated dev
// sessions. See RainOnWindowSchema for the full knob inventory.
type RainOnWindowConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroDensity float64 `json:"intro_density"`
	IntroBgRamp  float64 `json:"intro_bg"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
	// PANE / FRAME
	FrameThick float64 `json:"frame_thick"`
	PanePad    float64 `json:"pane_pad"`
	// SPAWN / GROWTH
	SpawnRate     float64 `json:"spawn_p"`
	MaxDrops      int     `json:"drop_max"`
	GrowthRate    float64 `json:"grow_rate"`
	MergeRadius   float64 `json:"merge_r"`
	MergeRate     float64 `json:"merge_p"`
	FallThreshold float64 `json:"fall_thresh"`
	// FALL
	FallSpeed float64 `json:"fall_speed"`
	Gravity   float64 `json:"gravity"`
	TrailLife int     `json:"trail_life"`
	// COLOR
	BgHue        float64 `json:"bg_hue"`
	BgSat        float64 `json:"bg_sat"`
	BgLight      float64 `json:"bg_light"`
	GlowHue      float64 `json:"glow_hue"`
	GlowStrength float64 `json:"glow"`
	FrameLight   float64 `json:"frame_light"`
	DropSat      float64 `json:"drop_sat"`
	DropLight    float64 `json:"drop_light"`
	HighlightL   float64 `json:"hi_light"`
	// EVENT CHANCES
	GustChance  float64 `json:"gust_p"`
	QuietChance float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	GustDur      int     `json:"gust_dur"`
	GustStrength float64 `json:"gust_str"`
	QuietDur     int     `json:"quiet_dur"`
	QuietMult    float64 `json:"quiet_mult"`
}

func (c RainOnWindowConfig) withDefaults() RainOnWindowConfig {
	if c.IntroDur == 0 && c.IntroDensity == 0 && c.IntroBgRamp == 0 {
		c.IntroDur = 60
		c.IntroDensity = 1.8
		c.IntroBgRamp = 0.35
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 60
		}
		if c.IntroDensity <= 0 {
			c.IntroDensity = 1.8
		}
		if c.IntroBgRamp < 0 {
			c.IntroBgRamp = 0
		}
	}
	c.IntroBgRamp = clamp01(c.IntroBgRamp)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingResidue == 0 {
		c.EndingDur = 80
		c.EndingLinger = 40
		c.EndingResidue = 0.25
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 80
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingResidue < 0 {
			c.EndingResidue = 0
		}
	}
	c.EndingResidue = clamp01(c.EndingResidue)
	if c.FrameThick <= 0 {
		c.FrameThick = 2
	}
	if c.PanePad < 0 {
		c.PanePad = 0
	}
	if c.SpawnRate < 0 {
		c.SpawnRate = 0
	}
	if c.MaxDrops <= 0 {
		c.MaxDrops = 80
	}
	if c.GrowthRate <= 0 {
		c.GrowthRate = 0.012
	}
	if c.MergeRadius <= 0 {
		c.MergeRadius = 1.6
	}
	if c.MergeRate < 0 {
		c.MergeRate = 0
	}
	if c.FallThreshold <= 0 {
		c.FallThreshold = 1.7
	}
	if c.FallSpeed <= 0 {
		c.FallSpeed = 0.18
	}
	if c.Gravity < 0 {
		c.Gravity = 0
	}
	if c.TrailLife <= 0 {
		c.TrailLife = 60
	}
	if c.BgHue == 0 {
		c.BgHue = 32
	}
	if c.BgSat <= 0 {
		c.BgSat = 0.42
	}
	if c.BgLight <= 0 {
		c.BgLight = 0.24
	}
	if c.GlowHue == 0 {
		c.GlowHue = 38
	}
	if c.GlowStrength <= 0 {
		c.GlowStrength = 0.6
	}
	if c.FrameLight <= 0 {
		c.FrameLight = 0.08
	}
	if c.DropSat <= 0 {
		c.DropSat = 0.18
	}
	if c.DropLight <= 0 {
		c.DropLight = 0.7
	}
	if c.HighlightL <= 0 {
		c.HighlightL = 0.92
	}
	if c.GustChance < 0 {
		c.GustChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.GustDur <= 0 {
		c.GustDur = 40
	}
	if c.GustStrength <= 0 {
		c.GustStrength = 0.6
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 80
	}
	if c.QuietMult <= 0 {
		c.QuietMult = 0.2
	}
	return c
}

// RainOnWindowSchema describes the rain-on-window effect's tunable knobs for
// the dev UI.
func RainOnWindowSchema() EffectSchema {
	return EffectSchema{
		Name: "rain-on-window",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent ramping nucleation density and background glow into steady-state."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.8,
				Description: "Spawn-rate multiplier during the intro to quickly establish the rain-on-window trope."},
			{Key: "intro_bg", Label: "intro bg ramp", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Starting background-glow strength during the intro (0 = dark, 1 = full)."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent suppressing new spawns and fading the background glow."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 40,
				Description: "Extra still ticks for existing tracks to finish before the pane resets."},
			{Key: "ending_residue", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Fraction of leftover droplets that linger as condensation through the outro."},
			{Key: "frame_thick", Label: "frame thick", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 0, Max: 6, Step: 0.5, Default: 2,
				Description: "Thickness of the silhouette window frame around the pane (0 = no frame)."},
			{Key: "pane_pad", Label: "pane pad", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 0, Max: 0.2, Step: 0.01, Default: 0.04,
				Description: "Padding between the canvas edge and the pane interior, as a fraction."},
			{Key: "spawn_p", Label: "spawn", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0, Max: 1.5, Step: 0.02, Default: 0.5,
				Description: "Per-tick expected number of new droplets nucleating on the pane (Poisson-like)."},
			{Key: "drop_max", Label: "max drops", Slot: SlotLever, Group: "drops", Type: KnobInt, Min: 8, Max: 240, Step: 4, Default: 80,
				Description: "Cap on simultaneously live droplets — keeps dense scenes from overloading."},
			{Key: "grow_rate", Label: "grow rate", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0.001, Max: 0.06, Step: 0.001, Default: 0.012,
				Description: "How fast pinned droplets accrete moisture per tick."},
			{Key: "merge_r", Label: "merge radius", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Maximum gap (in cells) at which two pinned droplets combine into one."},
			{Key: "merge_p", Label: "merge rate", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.5,
				Description: "Per-tick chance of an eligible pair of touching pinned droplets actually merging."},
			{Key: "fall_thresh", Label: "fall thresh", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 1, Max: 4, Step: 0.05, Default: 1.7,
				Description: "Critical droplet radius at which surface tension breaks and the drop tracks downward."},
			{Key: "fall_speed", Label: "fall speed", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0.04, Max: 1.2, Step: 0.02, Default: 0.18,
				Description: "Initial downward speed of a falling droplet, in cells per tick."},
			{Key: "gravity", Label: "gravity", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.001, Default: 0.012,
				Description: "Acceleration applied to falling droplets each tick (0 = constant speed)."},
			{Key: "trail_life", Label: "trail life", Slot: SlotLever, Group: "drops", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60,
				Description: "How long a falling droplet's residual track stays visible after it passes."},
			{Key: "bg_hue", Label: "bg hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 32,
				Description: "Base hue of the diffuse light field behind the pane."},
			{Key: "bg_sat", Label: "bg sat", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.42,
				Description: "Saturation of the background glow."},
			{Key: "bg_light", Label: "bg light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.24,
				Description: "Lightness floor for the background glow at the pane edges."},
			{Key: "glow_hue", Label: "glow hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 38,
				Description: "Hue of the central highlight inside the pane (warm interior vs. cool city)."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.6,
				Description: "Strength of the central highlight that suggests interior light or city haze."},
			{Key: "frame_light", Label: "frame light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Lightness of the silhouette frame around the pane."},
			{Key: "drop_sat", Label: "drop sat", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Saturation of the droplet bodies (kept low so they read as glass)."},
			{Key: "drop_light", Label: "drop light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1, Step: 0.02, Default: 0.7,
				Description: "Body lightness of pinned droplets."},
			{Key: "hi_light", Label: "highlight light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.6, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Lightness of the upper-left specular highlight on each droplet."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance of a wind gust briefly skewing droplets sideways."},
			{Key: "quiet_p", Label: "quiet", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-pane",
				Description: "Per-tick chance of a quiet-pane window where new drops are rare."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind-gust", Type: KnobInt, Min: 8, Max: 200, Step: 4, Default: 40,
				Description: "Duration of a wind-gust window in ticks."},
			{Key: "gust_str", Label: "gust strength", Slot: SlotEventMod, Group: "wind-gust", Type: KnobFloat, Min: 0.05, Max: 1.5, Step: 0.05, Default: 0.6,
				Description: "Sideways drift strength applied to droplets during a gust."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-pane", Type: KnobInt, Min: 20, Max: 400, Step: 10, Default: 80,
				Description: "Duration of a quiet-pane window in ticks."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-pane", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.2,
				Description: "Spawn-rate multiplier during a quiet-pane window."},
		},
	}
}

// RainOnWindowState is the wire/persisted shape of the lifecycle + event
// timers. Droplets and tracks live in a separate Snapshot wrapper.
type RainOnWindowState struct {
	Tick        int     `json:"tick"`
	GustTicks   int     `json:"gustTicks"`
	GustSide    int     `json:"gustSide"`
	QuietTicks  int     `json:"quietTicks"`
	IntroTicks  int     `json:"introTicks"`
	IntroTotal  int     `json:"introTotal"`
	EndingTicks int     `json:"endingTicks"`
	EndingTotal int     `json:"endingTotal"`
	EndingFade  int     `json:"endingFade"`
	GustWind    float64 `json:"gustWind"`
}

type RainOnWindowDroplet struct {
	Row     float64 `json:"row"`
	Col     float64 `json:"col"`
	Radius  float64 `json:"radius"`
	VRow    float64 `json:"vRow"`
	VCol    float64 `json:"vCol"`
	Hue     float64 `json:"hue"`
	PinRow  float64 `json:"pinRow"`
	Falling bool    `json:"falling"`
}

type RainOnWindowTrack struct {
	Row      float64 `json:"row"`
	Col      float64 `json:"col"`
	Strength float64 `json:"strength"`
	Life     int     `json:"life"`
	MaxLife  int     `json:"maxLife"`
}

type RainOnWindowSnapshot struct {
	RainOnWindowState
	Droplets []RainOnWindowDroplet `json:"droplets"`
	Tracks   []RainOnWindowTrack   `json:"tracks"`
}

type RainOnWindowPersistedState struct {
	RainOnWindowState
	RNGState uint64                `json:"rngState"`
	Droplets []RainOnWindowDroplet `json:"droplets"`
	Tracks   []RainOnWindowTrack   `json:"tracks"`
}

// RainOnWindow is the authoritative server-side rain-on-window sim.
type RainOnWindow struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  RainOnWindowConfig
	tick int

	droplets []windowDroplet
	tracks   []windowTrack

	gustTicks   int
	gustSide    int
	gustWind    float64
	quietTicks  int
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int

	log []LogEntry
}

func NewRainOnWindow(w, h int, seed int64, cfg RainOnWindowConfig) *RainOnWindow {
	return &RainOnWindow{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
}

func (r *RainOnWindow) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.W = w
	r.H = h
}

func (r *RainOnWindow) SetConfig(cfg RainOnWindowConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg.withDefaults()
}

func (r *RainOnWindow) EffectiveConfig() RainOnWindowConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *RainOnWindow) CurrentTick() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tick
}

func (r *RainOnWindow) PerturbRNG(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Mix(delta)
}

func (r *RainOnWindow) DrainLog() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) == 0 {
		return nil
	}
	out := r.log
	r.log = nil
	return out
}

func (r *RainOnWindow) appendLog(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

func (r *RainOnWindow) Snapshot() RainOnWindowSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainOnWindowSnapshot{
		RainOnWindowState: r.snapshotStateLocked(),
		Droplets:          r.copyDropletsLocked(),
		Tracks:            r.copyTracksLocked(),
	}
}

func (r *RainOnWindow) RestoreSnapshot(s RainOnWindowSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(s.RainOnWindowState)
	r.restoreDropletsLocked(s.Droplets)
	r.restoreTracksLocked(s.Tracks)
}

func (r *RainOnWindow) SnapshotPersistedState() RainOnWindowPersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainOnWindowPersistedState{
		RainOnWindowState: r.snapshotStateLocked(),
		RNGState:          r.rng.State(),
		Droplets:          r.copyDropletsLocked(),
		Tracks:            r.copyTracksLocked(),
	}
}

func (r *RainOnWindow) RestorePersistedState(s RainOnWindowPersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(s.RainOnWindowState)
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}
	r.restoreDropletsLocked(s.Droplets)
	r.restoreTracksLocked(s.Tracks)
}

func (r *RainOnWindow) snapshotStateLocked() RainOnWindowState {
	return RainOnWindowState{
		Tick:        r.tick,
		GustTicks:   r.gustTicks,
		GustSide:    r.gustSide,
		QuietTicks:  r.quietTicks,
		IntroTicks:  r.introTicks,
		IntroTotal:  r.introTotal,
		EndingTicks: r.endingTicks,
		EndingTotal: r.endingTotal,
		EndingFade:  r.endingFade,
		GustWind:    r.gustWind,
	}
}

func (r *RainOnWindow) restoreStateLocked(s RainOnWindowState) {
	r.tick = s.Tick
	r.gustTicks = s.GustTicks
	r.gustSide = s.GustSide
	r.quietTicks = s.QuietTicks
	r.introTicks = s.IntroTicks
	r.introTotal = s.IntroTotal
	r.endingTicks = s.EndingTicks
	r.endingTotal = s.EndingTotal
	r.endingFade = s.EndingFade
	r.gustWind = s.GustWind
}

func (r *RainOnWindow) copyDropletsLocked() []RainOnWindowDroplet {
	out := make([]RainOnWindowDroplet, len(r.droplets))
	for i, d := range r.droplets {
		out[i] = RainOnWindowDroplet{
			Row: d.Row, Col: d.Col, Radius: d.Radius,
			VRow: d.VRow, VCol: d.VCol, Hue: d.Hue,
			PinRow: d.PinRow, Falling: d.Falling,
		}
	}
	return out
}

func (r *RainOnWindow) restoreDropletsLocked(list []RainOnWindowDroplet) {
	r.droplets = make([]windowDroplet, len(list))
	for i, d := range list {
		r.droplets[i] = windowDroplet{
			Row: d.Row, Col: d.Col, Radius: d.Radius,
			VRow: d.VRow, VCol: d.VCol, Hue: d.Hue,
			PinRow: d.PinRow, Falling: d.Falling,
		}
	}
}

func (r *RainOnWindow) copyTracksLocked() []RainOnWindowTrack {
	out := make([]RainOnWindowTrack, len(r.tracks))
	for i, t := range r.tracks {
		out[i] = RainOnWindowTrack{
			Row: t.Row, Col: t.Col, Strength: t.Strength,
			Life: t.Life, MaxLife: t.MaxLife,
		}
	}
	return out
}

func (r *RainOnWindow) restoreTracksLocked(list []RainOnWindowTrack) {
	r.tracks = make([]windowTrack, len(list))
	for i, t := range list {
		r.tracks[i] = windowTrack{
			Row: t.Row, Col: t.Col, Strength: t.Strength,
			Life: t.Life, MaxLife: t.MaxLife,
		}
	}
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (r *RainOnWindow) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "wind-gust":
		r.startGustLocked("triggered")
	case "quiet-pane":
		r.startQuietLocked("triggered")
	case "intro":
		r.startIntroLocked()
		r.appendLog("intro", fmt.Sprintf("started (dur=%d, density x%.2f)", r.introTotal, r.cfg.IntroDensity))
	case "ending":
		r.startEndingLocked()
		r.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", r.endingFade, r.endingTotal-r.endingFade))
	default:
		return false
	}
	return true
}

func (r *RainOnWindow) startGustLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.GustDur, 0.3)
	r.gustTicks = dur
	if r.rng.Float64() < 0.5 {
		r.gustSide = -1
	} else {
		r.gustSide = 1
	}
	r.gustWind = float64(r.gustSide) * r.cfg.GustStrength
	r.appendLog("wind-gust", fmt.Sprintf("%s (dur=%d, side=%+d, strength=%.2f)", verb, dur, r.gustSide, r.cfg.GustStrength))
}

func (r *RainOnWindow) startQuietLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.QuietDur, 0.3)
	r.quietTicks = dur
	r.appendLog("quiet-pane", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, r.cfg.QuietMult))
}

func (r *RainOnWindow) startIntroLocked() {
	r.endingTicks = 0
	r.endingTotal = 0
	r.endingFade = 0
	r.introTotal = r.cfg.IntroDur
	if r.introTotal <= 0 {
		r.introTotal = 60
	}
	r.introTicks = r.introTotal
}

func (r *RainOnWindow) startEndingLocked() {
	r.introTicks = 0
	r.introTotal = 0
	r.endingFade = r.cfg.EndingDur
	if r.endingFade <= 0 {
		r.endingFade = 80
	}
	linger := r.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	r.endingTotal = r.endingFade + linger
	if r.endingTotal < 1 {
		r.endingTotal = r.endingFade
	}
	r.endingTicks = r.endingTotal
}

func (r *RainOnWindow) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++
	if r.gustTicks > 0 {
		r.gustTicks--
		if r.gustTicks == 0 {
			r.gustWind = 0
			r.gustSide = 0
		}
	}
	if r.quietTicks > 0 {
		r.quietTicks--
	}
	if r.introTicks > 0 {
		r.introTicks--
	}
	if r.endingTicks > 0 {
		r.endingTicks--
	}

	if r.gustTicks == 0 && r.endingTicks == 0 && r.cfg.GustChance > 0 && r.rng.Float64() < r.cfg.GustChance {
		r.startGustLocked("started")
	}
	if r.quietTicks == 0 && r.endingTicks == 0 && r.cfg.QuietChance > 0 && r.rng.Float64() < r.cfg.QuietChance {
		r.startQuietLocked("started")
	}

	r.spawnLocked()
	r.advanceDropletsLocked()
	r.advanceTracksLocked()
}

// activityLevel returns the current spawn-multiplier from the lifecycle and
// quiet-pane modifiers. 0 = no spawning, 1 = baseline, >1 during intro.
func (r *RainOnWindow) activityLevel() float64 {
	level := 1.0
	if r.quietTicks > 0 {
		level *= r.cfg.QuietMult
	}
	if r.introTicks > 0 {
		// Intro density boost ramps from full at the start to 1.0 by the end.
		progress := phaseProgress(r.introTotal, r.introTicks)
		level *= r.cfg.IntroDensity*(1-progress) + 1.0*progress
	}
	if r.endingTicks > 0 {
		elapsed := r.endingTotal - r.endingTicks
		if elapsed < r.endingFade {
			fade := clamp01(float64(elapsed) / float64(max(1, r.endingFade-1)))
			level *= 1 - fade
		} else {
			level *= 0
		}
	}
	if level < 0 {
		level = 0
	}
	return level
}

func (r *RainOnWindow) paneRectLocked() (rowMin, rowMax, colMin, colMax int) {
	pad := int(math.Round(r.cfg.PanePad * float64(min(r.W, r.H))))
	frame := int(math.Round(r.cfg.FrameThick))
	inset := pad + frame
	if inset < 0 {
		inset = 0
	}
	rowMin = inset
	rowMax = r.H - 1 - inset
	colMin = inset
	colMax = r.W - 1 - inset
	if rowMax <= rowMin {
		rowMin = 0
		rowMax = r.H - 1
	}
	if colMax <= colMin {
		colMin = 0
		colMax = r.W - 1
	}
	return
}

func (r *RainOnWindow) spawnLocked() {
	if len(r.droplets) >= r.cfg.MaxDrops {
		return
	}
	level := r.activityLevel()
	if level <= 0 {
		return
	}
	rate := r.cfg.SpawnRate * level
	if rate <= 0 {
		return
	}
	// Treat rate as expected drops per tick. Spawn a Bernoulli per integer
	// portion plus one extra Bernoulli on the fractional remainder.
	rowMin, rowMax, colMin, colMax := r.paneRectLocked()
	if rowMax-rowMin < 4 || colMax-colMin < 4 {
		return
	}
	whole := int(rate)
	frac := rate - float64(whole)
	expected := whole
	if r.rng.Float64() < frac {
		expected++
	}
	for i := 0; i < expected; i++ {
		if len(r.droplets) >= r.cfg.MaxDrops {
			break
		}
		col := float64(colMin) + r.rng.Float64()*float64(colMax-colMin)
		// Bias toward the windward edge during a gust so pane reads as wet
		// from one side.
		if r.gustTicks > 0 && r.cfg.GustStrength > 0 {
			edge := float64(colMin)
			if r.gustSide > 0 {
				edge = float64(colMax)
			}
			bias := 0.4 + 0.4*r.rng.Float64()
			col = col*(1-bias) + edge*bias
		}
		row := float64(rowMin) + r.rng.Float64()*float64(rowMax-rowMin)
		hueJitter := (r.rng.Float64()*2 - 1) * 8
		r.droplets = append(r.droplets, windowDroplet{
			Row:    row,
			Col:    col,
			Radius: 0.4 + r.rng.Float64()*0.4,
			Hue:    math.Mod(r.cfg.BgHue+hueJitter+360, 360),
		})
		r.appendLog("drop-form", fmt.Sprintf("at (%.0f,%.0f) r=%.2f", col, row, r.droplets[len(r.droplets)-1].Radius))
	}
}

func (r *RainOnWindow) advanceDropletsLocked() {
	if len(r.droplets) == 0 {
		return
	}
	_, rowMax, colMin, colMax := r.paneRectLocked()
	growBoost := 1.0
	if r.endingTicks > 0 {
		// During ending, suppress growth so existing drops finish their
		// tracks instead of fattening.
		elapsed := r.endingTotal - r.endingTicks
		fade := clamp01(float64(elapsed) / float64(max(1, r.endingFade-1)))
		growBoost = 1 - fade
	}
	alive := r.droplets[:0]
	for i := range r.droplets {
		d := r.droplets[i]
		if !d.Falling {
			// Pinned: accrete moisture; if past threshold, start falling.
			d.Radius += r.cfg.GrowthRate * growBoost * (0.6 + 0.8*r.rng.Float64())
			// Wind nudges pinned drops sideways slightly so the pane reads
			// as breezy during gusts.
			if r.gustTicks > 0 && r.cfg.GustStrength > 0 {
				d.Col += float64(r.gustSide) * 0.04 * r.cfg.GustStrength
			}
			if d.Radius >= r.cfg.FallThreshold {
				d.Falling = true
				d.PinRow = d.Row
				d.VRow = r.cfg.FallSpeed * (0.85 + 0.4*r.rng.Float64())
				d.VCol = r.gustWind * 0.6
				r.appendLog("drop-fall", fmt.Sprintf("at (%.0f,%.0f) r=%.2f", d.Col, d.Row, d.Radius))
			}
		} else {
			d.VRow += r.cfg.Gravity
			d.Row += d.VRow
			d.Col += d.VCol
			// Wind drift bleeds off so falling drops eventually return to
			// vertical.
			d.VCol *= 0.94
			// Drops shrink slightly as they leave moisture behind on the
			// pane.
			d.Radius -= r.cfg.GrowthRate * 0.6
			// Lay down a track point every couple cells of travel so the
			// streak is visible from rendering side without spamming
			// state.
			if int(math.Round(d.Row))%2 == 0 && r.rng.Float64() < 0.65 {
				life := r.cfg.TrailLife
				if life <= 0 {
					life = 60
				}
				strength := clamp01(d.Radius / math.Max(0.2, r.cfg.FallThreshold))
				r.tracks = append(r.tracks, windowTrack{
					Row:      d.Row,
					Col:      d.Col,
					Strength: strength,
					Life:     life,
					MaxLife:  life,
				})
			}
		}
		// Cull off-pane / vanished drops.
		if d.Radius <= 0.2 {
			continue
		}
		if d.Row > float64(rowMax)+1 {
			continue
		}
		if d.Col < float64(colMin)-1 || d.Col > float64(colMax)+1 {
			continue
		}
		alive = append(alive, d)
	}
	r.droplets = alive

	// Pinned/pinned merge step: walk the list and merge any two non-falling
	// drops that overlap within MergeRadius.
	if r.cfg.MergeRadius > 0 {
		merged := r.droplets[:0]
		used := make([]bool, len(r.droplets))
		for i := range r.droplets {
			if used[i] {
				continue
			}
			a := r.droplets[i]
			if !a.Falling {
				for j := i + 1; j < len(r.droplets); j++ {
					if used[j] {
						continue
					}
					b := r.droplets[j]
					if b.Falling {
						continue
					}
					dr := a.Row - b.Row
					dc := a.Col - b.Col
					if math.Hypot(dr, dc) > a.Radius+b.Radius+r.cfg.MergeRadius*0.4 {
						continue
					}
					if r.rng.Float64() > r.cfg.MergeRate {
						continue
					}
					// Merge area-wise so the combined drop is meaningfully
					// bigger.
					area := a.Radius*a.Radius + b.Radius*b.Radius
					a.Radius = math.Sqrt(area)
					a.Row = (a.Row*a.Radius + b.Row*b.Radius) / (a.Radius + b.Radius)
					a.Col = (a.Col*a.Radius + b.Col*b.Radius) / (a.Radius + b.Radius)
					a.Hue = (a.Hue + b.Hue) * 0.5
					used[j] = true
					r.appendLog("drop-merge", fmt.Sprintf("at (%.0f,%.0f) r=%.2f", a.Col, a.Row, a.Radius))
				}
			}
			used[i] = true
			merged = append(merged, a)
		}
		r.droplets = merged
	}
}

func (r *RainOnWindow) advanceTracksLocked() {
	if len(r.tracks) == 0 {
		return
	}
	alive := r.tracks[:0]
	for _, t := range r.tracks {
		t.Life--
		if t.Life > 0 {
			alive = append(alive, t)
		}
	}
	r.tracks = alive
}
