package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// RainOnWindowConfig tunes the rain-on-window effect. The effect is a single
// glass pane: a static blurred light field behind the glass, a subtle frame,
// and droplets that nucleate, grow, merge, and run downward.
type RainOnWindowConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroDensity float64 `json:"intro_density"`
	// ENDING
	EndingDur int `json:"ending_dur"`
	TrackLife int `json:"track_life"`
	// DROPLETS
	Nucleation   float64 `json:"nucleation"`
	GrowRate     float64 `json:"grow"`
	CriticalMass float64 `json:"critical"`
	MergeFactor  float64 `json:"merge"`
	MaxDrops     int     `json:"max_drops"`
	// MOTION
	FallSpeed  float64 `json:"fall_speed"`
	Wind       float64 `json:"wind"`
	WindJitter float64 `json:"wind_jit"`
	// BACKGROUND / GLASS
	GlowHue      float64 `json:"glow_hue"`
	GlowSat      float64 `json:"glow_sat"`
	GlowLight    float64 `json:"glow_light"`
	GlassTint    float64 `json:"glass_tint"`
	FrameDark    float64 `json:"frame"`
	DropContrast float64 `json:"drop_contrast"`
	// EVENT CHANCES
	FormChance  float64 `json:"form_p"`
	MergeChance float64 `json:"merge_p"`
	FallChance  float64 `json:"fall_p"`
	GustChance  float64 `json:"gust_p"`
	QuietChance float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	GustDur      int     `json:"gust_dur"`
	GustStrength float64 `json:"gust_strength"`
	QuietDur     int     `json:"quiet_dur"`
}

func (c RainOnWindowConfig) withDefaults() RainOnWindowConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 150
	}
	if c.IntroDensity <= 0 {
		c.IntroDensity = 0.42
	}
	c.IntroDensity = clamp01(c.IntroDensity)
	if c.EndingDur <= 0 {
		c.EndingDur = 260
	}
	if c.TrackLife <= 0 {
		c.TrackLife = 190
	}
	if c.Nucleation <= 0 {
		c.Nucleation = 0.16
	}
	c.Nucleation = clamp01(c.Nucleation)
	if c.GrowRate <= 0 {
		c.GrowRate = 0.034
	}
	if c.CriticalMass <= 0 {
		c.CriticalMass = 1.75
	}
	if c.MergeFactor <= 0 {
		c.MergeFactor = 1.08
	}
	if c.MaxDrops <= 0 {
		c.MaxDrops = 120
	}
	if c.MaxDrops < 4 {
		c.MaxDrops = 4
	}
	if c.MaxDrops > 320 {
		c.MaxDrops = 320
	}
	if c.FallSpeed <= 0 {
		c.FallSpeed = 0.56
	}
	if c.WindJitter < 0 {
		c.WindJitter = 0
	}
	if c.GlowHue == 0 {
		c.GlowHue = 42
	}
	if c.GlowSat <= 0 {
		c.GlowSat = 0.56
	}
	if c.GlowLight <= 0 {
		c.GlowLight = 0.36
	}
	if c.GlassTint <= 0 {
		c.GlassTint = 0.32
	}
	if c.FrameDark <= 0 {
		c.FrameDark = 0.62
	}
	if c.DropContrast <= 0 {
		c.DropContrast = 0.72
	}
	if c.FormChance < 0 {
		c.FormChance = 0
	}
	if c.MergeChance < 0 {
		c.MergeChance = 0
	}
	if c.FallChance < 0 {
		c.FallChance = 0
	}
	if c.GustChance < 0 {
		c.GustChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.GustDur <= 0 {
		c.GustDur = 150
	}
	if c.GustStrength <= 0 {
		c.GustStrength = 0.85
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 420
	}
	return c
}

// RainOnWindowSchema describes the rain-on-window effect's tunable knobs.
func RainOnWindowSchema() EffectSchema {
	return EffectSchema{
		Name:           "rain-on-window",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 30, Max: 600, Step: 10, Default: 150, Trigger: "intro",
				Description: "Ticks spent ramping from a nearly dry pane into the resting rain cadence."},
			{Key: "intro_density", Label: "intro drops", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.42,
				Description: "How quickly first droplets appear during the intro ramp."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 60, Max: 900, Step: 20, Default: 260, Trigger: "ending",
				Description: "Ticks spent stopping new drops, letting existing drops finish, and fading the glow."},
			{Key: "track_life", Label: "track life", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 40, Max: 480, Step: 10, Default: 190,
				Description: "How long residual wet tracks remain visible after drops slide past."},

			{Key: "nucleation", Label: "nucleation", Slot: SlotSpawn, Group: "droplets", Type: KnobFloat, Min: 0.01, Max: 0.7, Step: 0.01, Default: 0.16,
				Description: "Per-tick chance that fresh beads appear on the pane."},
			{Key: "grow", Label: "growth", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 0.006, Max: 0.12, Step: 0.002, Default: 0.034,
				Description: "Condensation rate: small droplets swell until gravity wins."},
			{Key: "critical", Label: "fall mass", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 0.65, Max: 3.6, Step: 0.05, Default: 1.75,
				Description: "Droplet mass threshold where beads begin running downward."},
			{Key: "merge", Label: "merge reach", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 0.65, Max: 1.8, Step: 0.05, Default: 1.08,
				Description: "Touch radius multiplier for combining adjacent droplets."},
			{Key: "max_drops", Label: "max drops", Slot: SlotLever, Group: "droplets", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 120,
				Description: "Upper bound for live droplets before old tiny beads are culled."},

			{Key: "fall_speed", Label: "fall speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.12, Max: 1.8, Step: 0.02, Default: 0.56,
				Description: "Base downward speed once droplets exceed the fall threshold."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -1.5, Max: 1.5, Step: 0.05, Default: 0,
				Description: "Steady sideways skew applied to falling tracks."},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.05, Default: 0.18,
				Description: "Per-drop variation in sideways drift."},

			{Key: "glow_hue", Label: "glow hue", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 20, Max: 320, Step: 1, Default: 42,
				Description: "Scene palette knob: amber interior through blue city-street glow."},
			{Key: "glow_sat", Label: "glow sat", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.56,
				Description: "Saturation of the blurred light behind the glass."},
			{Key: "glow_light", Label: "glow light", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 0.08, Max: 0.78, Step: 0.01, Default: 0.36,
				Description: "Brightness of the diffuse background light field."},
			{Key: "glass_tint", Label: "glass tint", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.32,
				Description: "Cool film over the pane; higher values feel wetter and dimmer."},
			{Key: "frame", Label: "frame", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 0.2, Max: 0.9, Step: 0.01, Default: 0.62,
				Description: "Darkness of the subtle window-frame silhouette."},
			{Key: "drop_contrast", Label: "drop contrast", Slot: SlotLever, Group: "background", Type: KnobFloat, Min: 0.25, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Brightness and rim contrast of the water beads."},

			{Key: "form_p", Label: "drop form", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.004, Step: 0.0001, Default: 0, Trigger: "drop-form",
				Description: "Per-tick chance of a visible fresh nucleation beat."},
			{Key: "merge_p", Label: "drop merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.0001, Default: 0, Trigger: "drop-merge",
				Description: "Per-tick chance of forcing nearby droplets to combine."},
			{Key: "fall_p", Label: "drop fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.0001, Default: 0, Trigger: "drop-fall",
				Description: "Per-tick chance of a droplet crossing the gravity threshold."},
			{Key: "gust_p", Label: "wind gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.002, Step: 0.0001, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance of a short sideways skew and edge-biased spawns."},
			{Key: "quiet_p", Label: "quiet pane", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.002, Step: 0.0001, Default: 0, Trigger: "quiet-pane",
				Description: "Per-tick chance of a long sparse interval with few new drops."},

			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind gust", Type: KnobInt, Min: 30, Max: 480, Step: 10, Default: 150,
				Description: "Typical duration of a wind-gust skew."},
			{Key: "gust_strength", Label: "gust strength", Slot: SlotEventMod, Group: "wind gust", Type: KnobFloat, Min: 0.15, Max: 2.2, Step: 0.05, Default: 0.85,
				Description: "Sideways push added during a wind gust."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet pane", Type: KnobInt, Min: 90, Max: 1200, Step: 30, Default: 420,
				Description: "Typical length of a sparse quiet-pane interval."},
		},
	}
}

// RainOnWindowDrop is the wire form of an active droplet.
type RainOnWindowDrop struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Mass    float64 `json:"mass"`
	Radius  float64 `json:"radius"`
	VX      float64 `json:"vx"`
	VY      float64 `json:"vy"`
	Falling bool    `json:"falling"`
	Age     int     `json:"age"`
}

// RainOnWindowTrack is the wire form of a fading vertical wet track.
type RainOnWindowTrack struct {
	X       float64 `json:"x"`
	Y0      float64 `json:"y0"`
	Y1      float64 `json:"y1"`
	Width   float64 `json:"width"`
	Skew    float64 `json:"skew"`
	Life    int     `json:"life"`
	MaxLife int     `json:"maxLife"`
}

// RainOnWindowSnapshot is the server/client wire state for rain-on-window.
type RainOnWindowSnapshot struct {
	Tick        int                 `json:"tick"`
	Lifecycle   Lifecycle           `json:"lifecycle"`
	IntroTicks  int                 `json:"introTicks,omitempty"`
	IntroTotal  int                 `json:"introTotal,omitempty"`
	EndingTicks int                 `json:"endingTicks,omitempty"`
	EndingTotal int                 `json:"endingTotal,omitempty"`
	Ended       bool                `json:"ended,omitempty"`
	QuietTicks  int                 `json:"quietTicks,omitempty"`
	GustTicks   int                 `json:"gustTicks,omitempty"`
	GustWind    float64             `json:"gustWind,omitempty"`
	GustBias    int                 `json:"gustBias,omitempty"`
	RNGState    uint64              `json:"rngState,omitempty"`
	Drops       []RainOnWindowDrop  `json:"drops"`
	Tracks      []RainOnWindowTrack `json:"tracks"`
}

// RainOnWindowPersistedState is the restart-safe state for rain-on-window.
type RainOnWindowPersistedState struct {
	RainOnWindowSnapshot
}

// RainOnWindow is the authoritative pixel-grid simulation.
type RainOnWindow struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng *rngutil.RNG
	cfg RainOnWindowConfig

	tick        int
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	ended       bool
	quietTicks  int
	gustTicks   int
	gustWind    float64
	gustBias    int

	drops  []RainOnWindowDrop
	tracks []RainOnWindowTrack
	log    []LogEntry
}

func NewRainOnWindow(w, h int, seed int64, cfg RainOnWindowConfig) *RainOnWindow {
	r := &RainOnWindow{
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	r.Resize(w, h)
	r.mu.Lock()
	r.seedInitialPaneLocked()
	r.renderLocked()
	r.mu.Unlock()
	return r
}

func (r *RainOnWindow) Resize(w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	r.W = w
	r.H = h
	r.Grid = make([][]Pixel, h)
	for y := range r.Grid {
		r.Grid[y] = make([]Pixel, w)
	}
	r.renderLocked()
}

func (r *RainOnWindow) EffectiveConfig() RainOnWindowConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *RainOnWindow) SetConfig(cfg RainOnWindowConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg.withDefaults()
	for i := range r.drops {
		r.drops[i].Radius = rainWindowRadius(r.drops[i].Mass)
		if r.drops[i].Falling {
			r.drops[i].VY = r.fallVelocityLocked(r.drops[i].Mass)
		}
	}
	r.renderLocked()
}

func (r *RainOnWindow) Snapshot() RainOnWindowSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(true)
}

func (r *RainOnWindow) RestoreSnapshot(snap RainOnWindowSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreSnapshotLocked(snap)
	r.renderLocked()
}

func (r *RainOnWindow) SnapshotPersistedState() RainOnWindowPersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainOnWindowPersistedState{RainOnWindowSnapshot: r.snapshotLocked(true)}
}

func (r *RainOnWindow) RestorePersistedState(state RainOnWindowPersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreSnapshotLocked(state.RainOnWindowSnapshot)
	r.renderLocked()
}

func (r *RainOnWindow) snapshotLocked(includeRNG bool) RainOnWindowSnapshot {
	drops := make([]RainOnWindowDrop, len(r.drops))
	copy(drops, r.drops)
	tracks := make([]RainOnWindowTrack, len(r.tracks))
	copy(tracks, r.tracks)
	out := RainOnWindowSnapshot{
		Tick:        r.tick,
		Lifecycle:   r.lifecycleLocked(),
		IntroTicks:  r.introTicks,
		IntroTotal:  r.introTotal,
		EndingTicks: r.endingTicks,
		EndingTotal: r.endingTotal,
		Ended:       r.ended,
		QuietTicks:  r.quietTicks,
		GustTicks:   r.gustTicks,
		GustWind:    r.gustWind,
		GustBias:    r.gustBias,
		Drops:       drops,
		Tracks:      tracks,
	}
	if includeRNG {
		out.RNGState = r.rng.State()
	}
	return out
}

func (r *RainOnWindow) restoreSnapshotLocked(s RainOnWindowSnapshot) {
	r.tick = s.Tick
	r.introTicks = s.IntroTicks
	r.introTotal = s.IntroTotal
	r.endingTicks = s.EndingTicks
	r.endingTotal = s.EndingTotal
	r.ended = s.Ended || s.Lifecycle == LifecycleEnded
	r.quietTicks = s.QuietTicks
	r.gustTicks = s.GustTicks
	r.gustWind = s.GustWind
	r.gustBias = s.GustBias
	r.drops = append(r.drops[:0], s.Drops...)
	for i := range r.drops {
		if r.drops[i].Radius <= 0 {
			r.drops[i].Radius = rainWindowRadius(r.drops[i].Mass)
		}
	}
	r.tracks = append(r.tracks[:0], s.Tracks...)
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}
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

func (r *RainOnWindow) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "drop-form":
		n := 2 + r.rng.Intn(3)
		made := 0
		for i := 0; i < n; i++ {
			if r.spawnDropLocked(false) {
				made++
			}
		}
		r.appendLogLocked("drop-form", fmt.Sprintf("%d new droplets", made))
	case "drop-merge":
		if !r.forceMergeLocked() {
			r.seedTouchingPairLocked()
			r.forceMergeLocked()
		}
	case "drop-fall":
		r.forceFallLocked("triggered")
	case "wind-gust":
		r.startGustLocked("triggered")
	case "quiet-pane":
		r.startQuietLocked("triggered")
	case "intro":
		r.startIntroLocked()
		r.appendLogLocked("intro", fmt.Sprintf("dry pane ramp (dur=%d)", r.introTotal))
	case "ending":
		r.startEndingLocked()
		r.appendLogLocked("ending", fmt.Sprintf("fade and drain (dur=%d)", r.endingTotal))
	default:
		return false
	}
	r.renderLocked()
	return true
}

func (r *RainOnWindow) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++

	if r.ended {
		r.renderLocked()
		return
	}

	if r.introTicks > 0 {
		r.introTicks--
	}
	if r.endingTicks > 0 {
		r.endingTicks--
		if r.endingTicks == 0 {
			r.ended = true
			r.drops = r.drops[:0]
			r.tracks = r.tracks[:0]
			r.quietTicks = 0
			r.gustTicks = 0
			r.gustWind = 0
			r.gustBias = 0
			r.appendLogLocked("ending", "terminal dim pane")
			r.renderLocked()
			return
		}
	}
	if r.quietTicks > 0 {
		r.quietTicks--
	}
	if r.gustTicks > 0 {
		r.gustTicks--
		if r.gustTicks == 0 {
			r.gustWind = 0
			r.gustBias = 0
		}
	}

	r.advanceTracksLocked()
	r.advanceDropsLocked()
	r.mergeTouchingDropsLocked(true)
	r.rollSpawnLocked()
	r.rollEventsLocked()
	r.trimDropsLocked()
	r.renderLocked()
}

func (r *RainOnWindow) GridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyPixelGrid(r.Grid)
}

func (r *RainOnWindow) lifecycleLocked() Lifecycle {
	switch {
	case r.introTicks > 0:
		return LifecycleIntro
	case r.endingTicks > 0:
		return LifecycleEnding
	case r.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (r *RainOnWindow) startIntroLocked() {
	r.ended = false
	r.endingTicks = 0
	r.endingTotal = 0
	r.quietTicks = 0
	r.gustTicks = 0
	r.gustWind = 0
	r.gustBias = 0
	r.drops = r.drops[:0]
	r.tracks = r.tracks[:0]
	r.introTotal = r.cfg.IntroDur
	r.introTicks = r.introTotal
	seed := 1 + int(math.Round(r.cfg.IntroDensity*6))
	for i := 0; i < seed; i++ {
		_ = r.spawnDropLocked(true)
	}
}

func (r *RainOnWindow) startEndingLocked() {
	r.introTicks = 0
	r.introTotal = 0
	r.endingTotal = r.cfg.EndingDur
	r.endingTicks = r.endingTotal
	r.quietTicks = r.endingTotal
	for i := range r.drops {
		r.startDropFallLocked(i)
	}
}

func (r *RainOnWindow) rollSpawnLocked() {
	if r.endingTicks > 0 || r.ended {
		return
	}
	prob := r.cfg.Nucleation
	if r.introTicks > 0 && r.introTotal > 0 {
		progress := 1 - float64(r.introTicks)/float64(r.introTotal)
		prob *= 1.4 + r.cfg.IntroDensity*3.0*(1-progress*0.45)
	}
	if r.quietTicks > 0 {
		prob *= 0.08
	}
	if prob > 0.95 {
		prob = 0.95
	}
	if r.rng.Float64() >= prob {
		return
	}
	count := 1
	if prob > 0.32 && r.rng.Float64() < prob {
		count++
	}
	for i := 0; i < count; i++ {
		_ = r.spawnDropLocked(false)
	}
}

func (r *RainOnWindow) rollEventsLocked() {
	if r.endingTicks > 0 || r.ended || r.introTicks > 0 {
		return
	}
	if r.cfg.FormChance > 0 && r.rng.Float64() < r.cfg.FormChance {
		if r.spawnDropLocked(false) {
			r.appendLogLocked("drop-form", "rolled fresh bead")
		}
	}
	if r.cfg.MergeChance > 0 && r.rng.Float64() < r.cfg.MergeChance {
		if r.forceMergeLocked() {
			r.appendLogLocked("drop-merge", "rolled contact")
		}
	}
	if r.cfg.FallChance > 0 && r.rng.Float64() < r.cfg.FallChance {
		r.forceFallLocked("rolled")
	}
	if r.cfg.GustChance > 0 && r.rng.Float64() < r.cfg.GustChance {
		r.startGustLocked("rolled")
	}
	if r.cfg.QuietChance > 0 && r.rng.Float64() < r.cfg.QuietChance {
		r.startQuietLocked("rolled")
	}
}

func (r *RainOnWindow) spawnDropLocked(intro bool) bool {
	if r.W <= 0 || r.H <= 0 || len(r.drops) >= r.cfg.MaxDrops {
		return false
	}
	marginX := math.Min(3, math.Max(0, float64(r.W-1)/8))
	marginY := math.Min(2, math.Max(0, float64(r.H-1)/10))
	xMin := marginX
	xMax := math.Max(xMin, float64(r.W-1)-marginX)
	yMin := marginY
	yMax := math.Max(yMin, float64(r.H-1)-marginY)

	x := xMin + r.rng.Float64()*(xMax-xMin)
	if r.gustBias != 0 && r.rng.Float64() < 0.72 {
		edge := math.Max(2, float64(r.W)*0.22)
		if r.gustBias < 0 {
			x = xMin + r.rng.Float64()*math.Min(edge, xMax-xMin)
		} else {
			x = xMax - r.rng.Float64()*math.Min(edge, xMax-xMin)
		}
	}
	y := yMin + r.rng.Float64()*(yMax-yMin)
	mass := 0.18 + r.rng.Float64()*0.42
	if intro {
		mass *= 0.55
	}
	windJit := 0.0
	if r.cfg.WindJitter > 0 {
		windJit = (r.rng.Float64()*2 - 1) * r.cfg.WindJitter * 0.035
	}
	d := RainOnWindowDrop{
		X:      x,
		Y:      y,
		Mass:   mass,
		Radius: rainWindowRadius(mass),
		VX:     windJit,
		Age:    0,
	}
	r.drops = append(r.drops, d)
	return true
}

func (r *RainOnWindow) seedInitialPaneLocked() {
	target := max(6, min(r.cfg.MaxDrops/3, max(1, r.W*r.H/420)))
	for i := 0; i < target; i++ {
		if !r.spawnDropLocked(false) {
			break
		}
		idx := len(r.drops) - 1
		r.drops[idx].Age = r.rng.Intn(90)
		r.drops[idx].Mass = 0.28 + r.rng.Float64()*r.cfg.CriticalMass*0.95
		r.drops[idx].Radius = rainWindowRadius(r.drops[idx].Mass)
		if r.rng.Float64() < 0.22 {
			r.startDropFallLocked(idx)
			trail := 1 + r.rng.Float64()*math.Max(2, float64(r.H)*0.18)
			r.addTrackLocked(r.drops[idx].X, r.drops[idx].Y-trail, r.drops[idx].Y, r.drops[idx].Radius*0.75, r.drops[idx].VX*8)
		}
	}
}

func (r *RainOnWindow) seedTouchingPairLocked() {
	if len(r.drops)+2 > r.cfg.MaxDrops {
		return
	}
	x := 2 + r.rng.Float64()*math.Max(1, float64(r.W-4))
	y := 2 + r.rng.Float64()*math.Max(1, float64(r.H-8))
	mass := r.cfg.CriticalMass * 0.38
	r.drops = append(r.drops,
		RainOnWindowDrop{X: x, Y: y, Mass: mass, Radius: rainWindowRadius(mass), Age: 8},
		RainOnWindowDrop{X: x + 0.7, Y: y + 0.15, Mass: mass * 0.9, Radius: rainWindowRadius(mass * 0.9), Age: 6},
	)
}

func (r *RainOnWindow) advanceDropsLocked() {
	out := r.drops[:0]
	for _, d := range r.drops {
		d.Age++
		if d.Falling {
			prevY := d.Y
			wind := (r.cfg.Wind + r.gustWind) * 0.055
			d.X += wind + d.VX
			d.Y += d.VY
			d.Mass += r.cfg.GrowRate * 0.18
			d.Radius = rainWindowRadius(d.Mass)
			r.addTrackLocked(d.X, prevY, d.Y, math.Max(0.65, d.Radius*0.82), wind*8)
			if d.Y-d.Radius <= float64(r.H)+2 && d.X >= -3 && d.X <= float64(r.W)+3 {
				out = append(out, d)
			}
			continue
		}

		growWave := 0.82 + 0.18*math.Sin(float64(d.Age)*0.23+d.X*0.17+d.Y*0.11)
		d.Mass += r.cfg.GrowRate * growWave
		d.Radius = rainWindowRadius(d.Mass)
		if d.Mass > r.cfg.CriticalMass*0.72 {
			d.Y += 0.012 + 0.018*(d.Mass/r.cfg.CriticalMass)
		}
		if d.Mass >= r.cfg.CriticalMass {
			d.Falling = true
			d.VY = r.fallVelocityLocked(d.Mass)
			r.appendLogLocked("drop-fall", fmt.Sprintf("mass %.2f", d.Mass))
		}
		out = append(out, d)
	}
	r.drops = out
}

func (r *RainOnWindow) advanceTracksLocked() {
	out := r.tracks[:0]
	for _, tr := range r.tracks {
		tr.Life--
		if tr.Life > 0 {
			out = append(out, tr)
		}
	}
	r.tracks = out
}

func (r *RainOnWindow) mergeTouchingDropsLocked(logEvents bool) int {
	merged := 0
	for i := 0; i < len(r.drops); i++ {
		for j := i + 1; j < len(r.drops); j++ {
			a := r.drops[i]
			b := r.drops[j]
			dx := a.X - b.X
			dy := a.Y - b.Y
			limit := (a.Radius + b.Radius) * r.cfg.MergeFactor
			if dx*dx+dy*dy > limit*limit {
				continue
			}
			r.drops[i] = rainWindowMergedDrop(a, b)
			if r.drops[i].Mass >= r.cfg.CriticalMass {
				r.startDropFallLocked(i)
			}
			r.drops = append(r.drops[:j], r.drops[j+1:]...)
			merged++
			j--
		}
	}
	if merged > 0 && logEvents {
		r.appendLogLocked("drop-merge", fmt.Sprintf("%d contact%s", merged, rainWindowPlural(merged)))
	}
	return merged
}

func (r *RainOnWindow) forceMergeLocked() bool {
	if len(r.drops) < 2 {
		return false
	}
	bestI, bestJ := 0, 1
	bestD := math.Inf(1)
	for i := 0; i < len(r.drops); i++ {
		for j := i + 1; j < len(r.drops); j++ {
			dx := r.drops[i].X - r.drops[j].X
			dy := r.drops[i].Y - r.drops[j].Y
			d := dx*dx + dy*dy
			if d < bestD {
				bestD = d
				bestI = i
				bestJ = j
			}
		}
	}
	r.drops[bestI] = rainWindowMergedDrop(r.drops[bestI], r.drops[bestJ])
	if r.drops[bestI].Mass >= r.cfg.CriticalMass {
		r.startDropFallLocked(bestI)
	}
	r.drops = append(r.drops[:bestJ], r.drops[bestJ+1:]...)
	r.appendLogLocked("drop-merge", "forced contact")
	return true
}

func (r *RainOnWindow) forceFallLocked(verb string) {
	if len(r.drops) == 0 {
		_ = r.spawnDropLocked(false)
	}
	if len(r.drops) == 0 {
		r.appendLogLocked("drop-fall", "skipped (full pane)")
		return
	}
	best := 0
	for i := range r.drops {
		if r.drops[i].Mass > r.drops[best].Mass {
			best = i
		}
	}
	if r.drops[best].Mass < r.cfg.CriticalMass {
		r.drops[best].Mass = r.cfg.CriticalMass * (1.02 + r.rng.Float64()*0.18)
		r.drops[best].Radius = rainWindowRadius(r.drops[best].Mass)
	}
	r.startDropFallLocked(best)
	r.appendLogLocked("drop-fall", fmt.Sprintf("%s (mass %.2f)", verb, r.drops[best].Mass))
}

func (r *RainOnWindow) startDropFallLocked(i int) {
	if i < 0 || i >= len(r.drops) {
		return
	}
	r.drops[i].Falling = true
	r.drops[i].VY = r.fallVelocityLocked(r.drops[i].Mass)
	if r.drops[i].VX == 0 && r.cfg.WindJitter > 0 {
		r.drops[i].VX = (r.rng.Float64()*2 - 1) * r.cfg.WindJitter * 0.04
	}
}

func (r *RainOnWindow) startGustLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.GustDur, 0.32)
	sign := 1.0
	if r.rng.Float64() < 0.5 {
		sign = -1
	}
	r.gustTicks = dur
	r.gustWind = sign * r.cfg.GustStrength * (0.8 + r.rng.Float64()*0.45)
	if r.gustWind < 0 {
		r.gustBias = -1
	} else {
		r.gustBias = 1
	}
	r.appendLogLocked("wind-gust", fmt.Sprintf("%s (dur=%d, skew=%.2f)", verb, dur, r.gustWind))
}

func (r *RainOnWindow) startQuietLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.QuietDur, 0.28)
	r.quietTicks = dur
	r.appendLogLocked("quiet-pane", fmt.Sprintf("%s (dur=%d)", verb, dur))
}

func (r *RainOnWindow) fallVelocityLocked(mass float64) float64 {
	return r.cfg.FallSpeed * (0.72 + math.Sqrt(math.Max(0.1, mass))*0.34)
}

func (r *RainOnWindow) addTrackLocked(x, y0, y1, width, skew float64) {
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	life := r.cfg.TrackLife
	if r.endingTicks > 0 && r.endingTotal > 0 {
		life = min(life, max(12, r.endingTicks))
	}
	r.tracks = append(r.tracks, RainOnWindowTrack{
		X:       x,
		Y0:      y0,
		Y1:      y1,
		Width:   width,
		Skew:    skew,
		Life:    life,
		MaxLife: life,
	})
	if len(r.tracks) > 260 {
		r.tracks = r.tracks[len(r.tracks)-260:]
	}
}

func (r *RainOnWindow) trimDropsLocked() {
	for len(r.drops) > r.cfg.MaxDrops {
		worst := 0
		for i := range r.drops {
			if r.drops[i].Falling {
				continue
			}
			if r.drops[i].Mass < r.drops[worst].Mass || r.drops[worst].Falling {
				worst = i
			}
		}
		r.drops = append(r.drops[:worst], r.drops[worst+1:]...)
	}
}

func (r *RainOnWindow) appendLogLocked(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

func (r *RainOnWindow) renderLocked() {
	if len(r.Grid) != r.H {
		return
	}
	level := r.backgroundLevelLocked()
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			r.Grid[y][x] = Pixel{Filled: true, C: r.backgroundColorLocked(x, y, level)}
		}
	}
	r.paintFrameLocked()
	for _, tr := range r.tracks {
		r.paintTrackLocked(tr, level)
	}
	for _, d := range r.drops {
		r.paintDropLocked(d, level)
	}
}

func (r *RainOnWindow) backgroundLevelLocked() float64 {
	if r.ended {
		return 0.05
	}
	level := 1.0
	if r.introTicks > 0 && r.introTotal > 0 {
		progress := 1 - float64(r.introTicks)/float64(r.introTotal)
		level = 0.18 + 0.82*rainWindowSmoothstep(progress)
	}
	if r.endingTicks > 0 && r.endingTotal > 0 {
		progress := float64(r.endingTicks) / float64(r.endingTotal)
		level = math.Min(level, 0.06+0.94*rainWindowSmoothstep(progress))
	}
	return clamp01(level)
}

func (r *RainOnWindow) backgroundColorLocked(x, y int, level float64) color.RGBA {
	nx := 0.0
	ny := 0.0
	if r.W > 1 {
		nx = float64(x) / float64(r.W-1)
	}
	if r.H > 1 {
		ny = float64(y) / float64(r.H-1)
	}
	center := rainWindowBlob(nx, ny, 0.38, 0.46, 0.42, 0.34)
	side := rainWindowBlob(nx, ny, 0.77, 0.28, 0.24, 0.18)
	low := rainWindowBlob(nx, ny, 0.58, 0.76, 0.52, 0.22)
	vignette := 0.65 + 0.35*(1-math.Hypot(nx-0.5, ny-0.5)/0.72)
	if vignette < 0.45 {
		vignette = 0.45
	}
	light := r.cfg.GlowLight * level * clamp01(0.16+0.72*center+0.42*side+0.26*low) * vignette
	hue := r.cfg.GlowHue + 14*side - 10*ny + 6*math.Sin(nx*math.Pi)
	sat := clamp01(r.cfg.GlowSat * (0.72 + 0.24*center + 0.12*side))
	c := hslToRGB(hue, sat, clamp01(light))
	tint := hslToRGB(210, 0.18, clamp01(0.035+0.095*r.cfg.GlassTint*level))
	return rainWindowBlend(c, tint, 0.18+0.25*r.cfg.GlassTint)
}

func (r *RainOnWindow) paintFrameLocked() {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	thick := max(1, min(r.W, r.H)/28)
	midX := r.W / 2
	midY := int(math.Round(float64(r.H) * 0.56))
	alpha := clamp01(r.cfg.FrameDark)
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			onBorder := x < thick || y < thick || x >= r.W-thick || y >= r.H-thick
			onMullion := rainWindowAbsInt(x-midX) <= thick/2
			onRail := r.H >= 36 && rainWindowAbsInt(y-midY) <= thick/2
			if !onBorder && !onMullion && !onRail {
				continue
			}
			r.Grid[y][x].C = rainWindowDarken(r.Grid[y][x].C, alpha)
		}
	}
}

func (r *RainOnWindow) paintTrackLocked(tr RainOnWindowTrack, level float64) {
	if tr.MaxLife <= 0 || tr.Life <= 0 {
		return
	}
	y0 := int(math.Floor(math.Min(tr.Y0, tr.Y1))) - 1
	y1 := int(math.Ceil(math.Max(tr.Y0, tr.Y1))) + 1
	if y0 < 0 {
		y0 = 0
	}
	if y1 >= r.H {
		y1 = r.H - 1
	}
	span := math.Max(1, tr.Y1-tr.Y0)
	life := float64(tr.Life) / float64(tr.MaxLife)
	baseAlpha := clamp01(0.28 * life * (0.4 + 0.6*level))
	col := hslToRGB(205, 0.18, 0.72)
	for y := y0; y <= y1; y++ {
		t := (float64(y) - tr.Y0) / span
		x := tr.X + tr.Skew*t
		radius := math.Max(0.55, tr.Width)
		for ix := int(math.Floor(x - radius - 1)); ix <= int(math.Ceil(x+radius+1)); ix++ {
			if ix < 0 || ix >= r.W {
				continue
			}
			dist := math.Abs(float64(ix) - x)
			if dist > radius+0.8 {
				continue
			}
			alpha := baseAlpha * clamp01(1-dist/(radius+0.8))
			r.Grid[y][ix].C = rainWindowBlend(r.Grid[y][ix].C, col, alpha)
		}
	}
}

func (r *RainOnWindow) paintDropLocked(d RainOnWindowDrop, level float64) {
	if d.Radius <= 0 {
		return
	}
	radius := math.Max(0.65, d.Radius)
	x0 := int(math.Floor(d.X - radius - 1))
	x1 := int(math.Ceil(d.X + radius + 1))
	y0 := int(math.Floor(d.Y - radius - 1))
	y1 := int(math.Ceil(d.Y + radius + 1))
	shine := hslToRGB(205, 0.22, 0.86)
	rim := hslToRGB(212, 0.28, 0.16)
	for y := y0; y <= y1; y++ {
		if y < 0 || y >= r.H {
			continue
		}
		for x := x0; x <= x1; x++ {
			if x < 0 || x >= r.W {
				continue
			}
			dx := float64(x) - d.X
			dy := float64(y) - d.Y
			dist := math.Hypot(dx, dy)
			if dist > radius+0.85 {
				continue
			}
			core := clamp01(1 - dist/(radius+0.85))
			alpha := core * (0.26 + 0.46*r.cfg.DropContrast) * (0.55 + 0.45*level)
			if dx < 0 && dy < 0 {
				alpha += 0.20 * core * r.cfg.DropContrast
			}
			r.Grid[y][x].C = rainWindowBlend(r.Grid[y][x].C, shine, clamp01(alpha))
			if dist > radius*0.58 || dy > radius*0.22 {
				r.Grid[y][x].C = rainWindowBlend(r.Grid[y][x].C, rim, clamp01(0.16*core*r.cfg.DropContrast))
			}
		}
	}
}

func rainWindowRadius(mass float64) float64 {
	return 0.42 + math.Sqrt(math.Max(0.05, mass))*0.38
}

func rainWindowMergedDrop(a, b RainOnWindowDrop) RainOnWindowDrop {
	mass := a.Mass + b.Mass
	if mass <= 0 {
		mass = 0.1
	}
	x := (a.X*a.Mass + b.X*b.Mass) / mass
	y := (a.Y*a.Mass + b.Y*b.Mass) / mass
	out := RainOnWindowDrop{
		X:       x,
		Y:       y,
		Mass:    mass,
		Radius:  rainWindowRadius(mass),
		VX:      (a.VX*a.Mass + b.VX*b.Mass) / mass,
		VY:      math.Max(a.VY, b.VY),
		Falling: a.Falling || b.Falling,
		Age:     max(a.Age, b.Age),
	}
	return out
}

func rainWindowBlob(x, y, cx, cy, sx, sy float64) float64 {
	dx := (x - cx) / math.Max(0.001, sx)
	dy := (y - cy) / math.Max(0.001, sy)
	return math.Exp(-(dx*dx + dy*dy) * 2.4)
}

func rainWindowBlend(base, over color.RGBA, alpha float64) color.RGBA {
	alpha = clamp01(alpha)
	inv := 1 - alpha
	return color.RGBA{
		R: uint8(math.Round(float64(base.R)*inv + float64(over.R)*alpha)),
		G: uint8(math.Round(float64(base.G)*inv + float64(over.G)*alpha)),
		B: uint8(math.Round(float64(base.B)*inv + float64(over.B)*alpha)),
		A: 255,
	}
}

func rainWindowDarken(c color.RGBA, alpha float64) color.RGBA {
	alpha = clamp01(alpha)
	scale := 1 - alpha
	return color.RGBA{
		R: uint8(math.Round(float64(c.R) * scale)),
		G: uint8(math.Round(float64(c.G) * scale)),
		B: uint8(math.Round(float64(c.B) * scale)),
		A: 255,
	}
}

func rainWindowPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func rainWindowSmoothstep(v float64) float64 {
	v = clamp01(v)
	return v * v * (3 - 2*v)
}

func rainWindowAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func rainWindowBrightness(grid [][]Pixel) float64 {
	var sum float64
	var count int
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			sum += float64(p.C.R) + float64(p.C.G) + float64(p.C.B)
			count += 3
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
