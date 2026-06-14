package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// RainOnWindowConfig tunes the rain-on-window effect. The authority owns the
// droplet state: drops nucleate, grow through condensation, merge when they
// touch, and fall once they cross the configured mass threshold.
type RainOnWindowConfig struct {
	// LIFECYCLE
	IntroDur     int     `json:"intro_dur"`
	IntroDensity float64 `json:"intro_density"`
	EndingDur    int     `json:"ending_dur"`
	EndingLinger int     `json:"ending_linger"`
	TrackLife    int     `json:"track_life"`
	// DROPLETS
	NucleationRate float64 `json:"nucleation"`
	GrowthRate     float64 `json:"growth"`
	FallThreshold  float64 `json:"fall_threshold"`
	FallSpeed      float64 `json:"fall_speed"`
	MergeRadius    float64 `json:"merge_radius"`
	DropCap        int     `json:"drop_cap"`
	DropContrast   float64 `json:"drop_contrast"`
	TrackStrength  float64 `json:"track_strength"`
	// BACKGROUND / GLASS
	BackgroundHue   float64 `json:"bg_hue"`
	BackgroundHueSp float64 `json:"bg_hue_sp"`
	Saturation      float64 `json:"sat"`
	Glow            float64 `json:"glow"`
	FrameStrength   float64 `json:"frame"`
	Wind            float64 `json:"wind"`
	// EVENT CHANCES
	DropFormChance  float64 `json:"drop_form_p"`
	DropMergeChance float64 `json:"drop_merge_p"`
	DropFallChance  float64 `json:"drop_fall_p"`
	WindGustChance  float64 `json:"wind_gust_p"`
	QuietPaneChance float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	DropFormBurst    int     `json:"drop_form_burst"`
	WindGustDur      int     `json:"wind_gust_dur"`
	WindGustStrength float64 `json:"wind_gust_strength"`
	QuietPaneDur     int     `json:"quiet_dur"`
	QuietPaneMult    float64 `json:"quiet_mult"`
}

func (c RainOnWindowConfig) withDefaults() RainOnWindowConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 90
	}
	if c.IntroDensity <= 0 {
		c.IntroDensity = 2.0
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 120
	}
	if c.EndingLinger <= 0 {
		c.EndingLinger = 70
	}
	if c.TrackLife <= 0 {
		c.TrackLife = 180
	}
	if c.NucleationRate <= 0 {
		c.NucleationRate = 0.16
	}
	if c.GrowthRate <= 0 {
		c.GrowthRate = 0.012
	}
	if c.FallThreshold <= 0 {
		c.FallThreshold = 2.35
	}
	if c.FallSpeed <= 0 {
		c.FallSpeed = 0.62
	}
	if c.MergeRadius <= 0 {
		c.MergeRadius = 0.86
	}
	if c.DropCap <= 0 {
		c.DropCap = 95
	}
	if c.DropCap > 220 {
		c.DropCap = 220
	}
	if c.DropContrast <= 0 {
		c.DropContrast = 0.78
	}
	if c.TrackStrength <= 0 {
		c.TrackStrength = 0.58
	}
	if c.BackgroundHue == 0 {
		c.BackgroundHue = 214
	}
	if c.BackgroundHueSp == 0 {
		c.BackgroundHueSp = 16
	}
	if c.BackgroundHueSp < 0 {
		c.BackgroundHueSp = 0
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.36
	}
	if c.Glow <= 0 {
		c.Glow = 0.62
	}
	if c.FrameStrength <= 0 {
		c.FrameStrength = 0.34
	}
	if c.DropFormBurst <= 0 {
		c.DropFormBurst = 3
	}
	if c.WindGustDur <= 0 {
		c.WindGustDur = 120
	}
	if c.WindGustStrength <= 0 {
		c.WindGustStrength = 1.15
	}
	if c.QuietPaneDur <= 0 {
		c.QuietPaneDur = 260
	}
	if c.QuietPaneMult <= 0 {
		c.QuietPaneMult = 0.08
	}
	if c.DropFormChance < 0 {
		c.DropFormChance = 0
	}
	if c.DropMergeChance < 0 {
		c.DropMergeChance = 0
	}
	if c.DropFallChance < 0 {
		c.DropFallChance = 0
	}
	if c.WindGustChance < 0 {
		c.WindGustChance = 0
	}
	if c.QuietPaneChance < 0 {
		c.QuietPaneChance = 0
	}
	c.IntroDensity = math.Max(0.1, c.IntroDensity)
	c.NucleationRate = math.Max(0, c.NucleationRate)
	c.GrowthRate = math.Max(0, c.GrowthRate)
	c.FallThreshold = math.Max(0.6, c.FallThreshold)
	c.FallSpeed = math.Max(0.05, c.FallSpeed)
	c.MergeRadius = math.Max(0.25, c.MergeRadius)
	c.DropContrast = clamp01(c.DropContrast)
	c.TrackStrength = clamp01(c.TrackStrength)
	c.Saturation = clamp01(c.Saturation)
	c.Glow = clamp01(c.Glow)
	c.FrameStrength = clamp01(c.FrameStrength)
	c.QuietPaneMult = clamp01(c.QuietPaneMult)
	return c
}

// RainOnWindowDrop is one active bead of water on the glass.
type RainOnWindowDrop struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Radius   float64 `json:"r"`
	Mass     float64 `json:"mass"`
	Growth   float64 `json:"growth"`
	Tone     float64 `json:"tone"`
	Age      int     `json:"age"`
	Falling  bool    `json:"falling,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Drift    float64 `json:"drift,omitempty"`
	TrailTop float64 `json:"trailTop,omitempty"`
}

// RainOnWindowTrack is the residual wet path left by a falling drop.
type RainOnWindowTrack struct {
	X         float64 `json:"x"`
	Y0        float64 `json:"y0"`
	Y1        float64 `json:"y1"`
	Width     float64 `json:"width"`
	Life      int     `json:"life"`
	TotalLife int     `json:"totalLife"`
	Tone      float64 `json:"tone"`
}

type RainOnWindowState struct {
	Tick        int                 `json:"tick"`
	Lifecycle   Lifecycle           `json:"lifecycle"`
	Drops       []RainOnWindowDrop  `json:"drops,omitempty"`
	Tracks      []RainOnWindowTrack `json:"tracks,omitempty"`
	IntroTicks  int                 `json:"introTicks,omitempty"`
	IntroTotal  int                 `json:"introTotal,omitempty"`
	EndingTicks int                 `json:"endingTicks,omitempty"`
	EndingTotal int                 `json:"endingTotal,omitempty"`
	Ended       bool                `json:"ended,omitempty"`
	WindTicks   int                 `json:"windTicks,omitempty"`
	WindTotal   int                 `json:"windTotal,omitempty"`
	WindBias    float64             `json:"windBias,omitempty"`
	QuietTicks  int                 `json:"quietTicks,omitempty"`
	QuietTotal  int                 `json:"quietTotal,omitempty"`
	RNGState    uint64              `json:"rngState,omitempty"`
}

// RainOnWindowSnapshot is the server/client wire state for rain-on-window.
type RainOnWindowSnapshot struct {
	RainOnWindowState
}

// RainOnWindowPersistedState is the restart-safe state for rain-on-window.
type RainOnWindowPersistedState struct {
	RainOnWindowState
}

// RainOnWindow is a contemplative pane-of-glass rain effect. The background is
// a fixed blurred light field; only water on the pane moves.
type RainOnWindow struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng *rngutil.RNG
	cfg RainOnWindowConfig

	tick        int
	drops       []RainOnWindowDrop
	tracks      []RainOnWindowTrack
	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	ended       bool
	windTicks   int
	windTotal   int
	windBias    float64
	quietTicks  int
	quietTotal  int
	log         []LogEntry
}

func NewRainOnWindow(w, h int, seed int64, cfg RainOnWindowConfig) *RainOnWindow {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e := &RainOnWindow{
		W:    w,
		H:    h,
		Grid: newPixelGrid(w, h),
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
	e.seedInitialDropsLocked()
	e.renderLocked()
	return e
}

// RainOnWindowSchema describes the rain-on-window effect's tunable knobs.
func RainOnWindowSchema() EffectSchema {
	return EffectSchema{
		Name:           "rain-on-window",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 90, Trigger: "intro",
				Description: "Ticks for a dry pane and faint glow to ramp into normal condensation."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.4, Max: 4, Step: 0.05, Default: 2.0,
				Description: "Extra first-drop nucleation during intro before settling into steady cadence."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 120, Trigger: "ending",
				Description: "Ticks spent stopping new drops, finishing tracks, and dimming the glow."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70,
				Description: "Extra fade time after the last drops are encouraged down the pane."},
			{Key: "track_life", Label: "track life", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 30, Max: 420, Step: 10, Default: 180,
				Description: "Ticks a wet track remains visible after a droplet has run through it."},
			{Key: "nucleation", Label: "nucleation", Slot: SlotSpawn, Group: "droplets", Type: KnobFloat, Min: 0.02, Max: 0.65, Step: 0.01, Default: 0.16,
				Description: "Per-tick chance that a new bead forms on the glass."},
			{Key: "growth", Label: "growth", Slot: SlotSpawn, Group: "droplets", Type: KnobFloat, Min: 0.002, Max: 0.04, Step: 0.001, Default: 0.012,
				Description: "How quickly stationary beads gain mass from condensation."},
			{Key: "fall_threshold", Label: "fall mass", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 1.2, Max: 4.5, Step: 0.05, Default: 2.35,
				Description: "Droplet radius at which gravity wins and the bead starts tracking downward."},
			{Key: "fall_speed", Label: "fall speed", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 0.12, Max: 1.8, Step: 0.02, Default: 0.62,
				Description: "Base downward speed for a droplet once it exceeds the fall threshold."},
			{Key: "merge_radius", Label: "merge radius", Slot: SlotLever, Group: "droplets", Type: KnobFloat, Min: 0.35, Max: 1.35, Step: 0.02, Default: 0.86,
				Trigger: "drop-merge", Description: "How close two beads must be before contact combines them."},
			{Key: "drop_cap", Label: "drop cap", Slot: SlotLever, Group: "droplets", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 95,
				Description: "Maximum active beads held by the pane before new nucleation is suppressed."},
			{Key: "drop_contrast", Label: "drop contrast", Slot: SlotLever, Group: "glass", Type: KnobFloat, Min: 0.15, Max: 1, Step: 0.02, Default: 0.78,
				Description: "Brightness and rim strength of bead highlights against the blurred scene."},
			{Key: "track_strength", Label: "track strength", Slot: SlotLever, Group: "glass", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.02, Default: 0.58,
				Description: "Visibility of vertical wet tracks left by falling droplets."},
			{Key: "bg_hue", Label: "bg hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 24, Max: 310, Step: 1, Default: 214,
				Description: "Palette knob for the blurred light field: cool city blues through warm interior glow."},
			{Key: "bg_hue_sp", Label: "bg spread", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0, Max: 80, Step: 1, Default: 16,
				Description: "Hue separation between the main glow and dim secondary city-light bands."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.05, Max: 0.85, Step: 0.01, Default: 0.36,
				Description: "Color saturation of the out-of-focus light behind the glass."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.12, Max: 1, Step: 0.02, Default: 0.62,
				Description: "Diffuse background light intensity behind the pane."},
			{Key: "frame", Label: "frame", Slot: SlotLever, Group: "glass", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.34,
				Description: "Subtle window-frame silhouette around the single pane."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -1, Max: 1, Step: 0.02, Default: 0,
				Description: "Persistent sideways drift applied to falling droplets."},
			{Key: "drop_form_p", Label: "drop form", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.015, Step: 0.0005, Default: 0, Trigger: "drop-form",
				Description: "Per-tick chance of an extra bead nucleating; trigger forms a visible small cluster."},
			{Key: "drop_merge_p", Label: "drop merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "drop-merge",
				Description: "Per-tick chance to encourage one nearby pair to combine."},
			{Key: "drop_fall_p", Label: "drop fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "drop-fall",
				Description: "Per-tick chance to push the heaviest bead over the fall threshold."},
			{Key: "wind_gust_p", Label: "wind gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance that tracks skew sideways and new drops bias to one edge."},
			{Key: "quiet_p", Label: "quiet pane", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0005, Default: 0, Trigger: "quiet-pane",
				Description: "Per-tick chance of a low-nucleation window where the pane briefly calms."},
			{Key: "drop_form_burst", Label: "form burst", Slot: SlotEventMod, Group: "drop form", Type: KnobInt, Min: 1, Max: 12, Step: 1, Default: 3,
				Description: "Number of beads created by a manual or chance drop-form event."},
			{Key: "wind_gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind gust", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 120,
				Description: "Duration of the sideways-skew gust interval."},
			{Key: "wind_gust_strength", Label: "gust strength", Slot: SlotEventMod, Group: "wind gust", Type: KnobFloat, Min: 0.15, Max: 2.5, Step: 0.05, Default: 1.15,
				Description: "Sideways bias applied to falling drops and new-drop edge selection during a gust."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet pane", Type: KnobInt, Min: 40, Max: 520, Step: 10, Default: 260,
				Description: "Duration of the suppressed-nucleation quiet-pane event."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet pane", Type: KnobFloat, Min: 0, Max: 0.45, Step: 0.01, Default: 0.08,
				Description: "Nucleation multiplier while quiet-pane is active."},
		},
	}
}

func (e *RainOnWindow) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.W = w
	e.H = h
	e.Grid = newPixelGrid(w, h)
	for i := range e.drops {
		e.drops[i].X = clampFloat(e.drops[i].X, 0, float64(w-1))
		e.drops[i].Y = clampFloat(e.drops[i].Y, 0, float64(h-1))
		e.drops[i].TrailTop = clampFloat(e.drops[i].TrailTop, 0, float64(h-1))
	}
	e.renderLocked()
}

func (e *RainOnWindow) EffectiveConfig() RainOnWindowConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *RainOnWindow) SetConfig(cfg RainOnWindowConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg.withDefaults()
	if len(e.drops) > e.cfg.DropCap {
		e.drops = append([]RainOnWindowDrop(nil), e.drops[:e.cfg.DropCap]...)
	}
	e.renderLocked()
}

func (e *RainOnWindow) Snapshot() RainOnWindowSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return RainOnWindowSnapshot{RainOnWindowState: e.snapshotStateLocked()}
}

func (e *RainOnWindow) RestoreSnapshot(snap RainOnWindowSnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.restoreStateLocked(snap.RainOnWindowState)
	e.renderLocked()
}

func (e *RainOnWindow) SnapshotPersistedState() RainOnWindowPersistedState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return RainOnWindowPersistedState{RainOnWindowState: e.snapshotStateLocked()}
}

func (e *RainOnWindow) RestorePersistedState(state RainOnWindowPersistedState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.restoreStateLocked(state.RainOnWindowState)
	e.renderLocked()
}

func (e *RainOnWindow) snapshotStateLocked() RainOnWindowState {
	drops := make([]RainOnWindowDrop, len(e.drops))
	copy(drops, e.drops)
	tracks := make([]RainOnWindowTrack, len(e.tracks))
	copy(tracks, e.tracks)
	return RainOnWindowState{
		Tick:        e.tick,
		Lifecycle:   e.lifecycleLocked(),
		Drops:       drops,
		Tracks:      tracks,
		IntroTicks:  e.introTicks,
		IntroTotal:  e.introTotal,
		EndingTicks: e.endingTicks,
		EndingTotal: e.endingTotal,
		Ended:       e.ended,
		WindTicks:   e.windTicks,
		WindTotal:   e.windTotal,
		WindBias:    e.windBias,
		QuietTicks:  e.quietTicks,
		QuietTotal:  e.quietTotal,
		RNGState:    e.rng.State(),
	}
}

func (e *RainOnWindow) restoreStateLocked(s RainOnWindowState) {
	e.tick = s.Tick
	e.drops = append([]RainOnWindowDrop(nil), s.Drops...)
	e.tracks = append([]RainOnWindowTrack(nil), s.Tracks...)
	e.introTicks = s.IntroTicks
	e.introTotal = s.IntroTotal
	e.endingTicks = s.EndingTicks
	e.endingTotal = s.EndingTotal
	e.ended = s.Ended || s.Lifecycle == LifecycleEnded
	e.windTicks = s.WindTicks
	e.windTotal = s.WindTotal
	e.windBias = s.WindBias
	e.quietTicks = s.QuietTicks
	e.quietTotal = s.QuietTotal
	if s.RNGState != 0 {
		e.rng.SetState(s.RNGState)
	}
	if len(e.drops) > e.cfg.DropCap {
		e.drops = e.drops[:e.cfg.DropCap]
	}
}

func (e *RainOnWindow) CurrentTick() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick
}

func (e *RainOnWindow) PerturbRNG(delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rng.Mix(delta)
}

func (e *RainOnWindow) DrainLog() []LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.log) == 0 {
		return nil
	}
	out := e.log
	e.log = nil
	return out
}

func (e *RainOnWindow) TriggerEvent(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch name {
	case "drop-form":
		n := e.formBurstLocked(true)
		e.appendLogLocked("drop-form", fmt.Sprintf("formed %d bead(s)", n))
	case "drop-merge":
		merged := e.forceMergeLocked()
		e.appendLogLocked("drop-merge", fmt.Sprintf("merged=%t drops=%d", merged, len(e.drops)))
	case "drop-fall":
		fell := e.forceFallLocked()
		e.appendLogLocked("drop-fall", fmt.Sprintf("started=%t falling=%d", fell, e.countFallingLocked()))
	case "wind-gust":
		e.startWindGustLocked("triggered")
	case "quiet-pane":
		e.startQuietPaneLocked("triggered")
	case "intro":
		e.startIntroLocked()
		e.appendLogLocked("intro", fmt.Sprintf("dry pane ramp (dur=%d)", e.introTotal))
	case "ending":
		e.startEndingLocked()
		e.appendLogLocked("ending", fmt.Sprintf("fade (dur=%d, linger=%d)", e.cfg.EndingDur, e.cfg.EndingLinger))
	default:
		return false
	}
	e.renderLocked()
	return true
}

func (e *RainOnWindow) Step() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.tick++
	wasEnding := e.endingTicks > 0
	e.tickTimersLocked()
	if wasEnding && e.endingTicks == 0 {
		e.ended = true
		e.drops = nil
		e.tracks = nil
		e.renderLocked()
		return
	}
	if e.ended {
		e.renderLocked()
		return
	}

	e.advanceDropsLocked()
	e.advanceTracksLocked()

	if e.endingTicks > 0 {
		e.renderLocked()
		return
	}

	e.rollEventChancesLocked()
	e.rollNucleationLocked()
	e.growStationaryDropsLocked()
	e.resolveMergesLocked(16, false)
	e.renderLocked()
}

func (e *RainOnWindow) GridCopy() [][]Pixel {
	e.mu.Lock()
	defer e.mu.Unlock()
	return copyPixelGrid(e.Grid)
}

func (e *RainOnWindow) tickTimersLocked() {
	if e.introTicks > 0 {
		e.introTicks--
	}
	if e.endingTicks > 0 {
		e.endingTicks--
	}
	if e.windTicks > 0 {
		e.windTicks--
		if e.windTicks == 0 {
			e.windBias = 0
			e.windTotal = 0
		}
	}
	if e.quietTicks > 0 {
		e.quietTicks--
		if e.quietTicks == 0 {
			e.quietTotal = 0
		}
	}
}

func (e *RainOnWindow) lifecycleLocked() Lifecycle {
	switch {
	case e.introTicks > 0:
		return LifecycleIntro
	case e.endingTicks > 0:
		return LifecycleEnding
	case e.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (e *RainOnWindow) appendLogLocked(kind, desc string) {
	e.log = append(e.log, LogEntry{Tick: e.tick, Type: kind, Desc: desc})
	if len(e.log) > 200 {
		e.log = e.log[len(e.log)-200:]
	}
}

func (e *RainOnWindow) seedInitialDropsLocked() {
	count := max(8, (e.W*e.H)/1050)
	count = min(count, max(8, e.cfg.DropCap*2/3))
	for i := 0; i < count; i++ {
		_ = e.spawnDropLocked(false)
	}
	for i := range e.drops {
		e.drops[i].Radius += e.rng.Float64() * 1.2
		e.drops[i].Mass = e.drops[i].Radius * e.drops[i].Radius
		if e.rng.Float64() < 0.08 {
			e.startDropFallLocked(i)
		}
	}
}

func (e *RainOnWindow) formBurstLocked(edgeAware bool) int {
	count := 0
	for i := 0; i < e.cfg.DropFormBurst; i++ {
		if e.spawnDropLocked(edgeAware) {
			count++
		}
	}
	return count
}

func (e *RainOnWindow) spawnDropLocked(edgeAware bool) bool {
	if len(e.drops) >= e.cfg.DropCap || e.W <= 0 || e.H <= 0 {
		return false
	}
	x := e.rng.Float64() * float64(max(1, e.W-1))
	if edgeAware && e.windTicks > 0 && math.Abs(e.windBias) > 0.01 {
		edgeWidth := math.Max(2, float64(e.W)*0.22)
		if e.windBias > 0 {
			x = e.rng.Float64() * edgeWidth
		} else {
			x = float64(e.W-1) - e.rng.Float64()*edgeWidth
		}
	}
	y := (0.06 + e.rng.Float64()*0.78) * float64(max(1, e.H-1))
	r := 0.42 + e.rng.Float64()*0.62
	growth := e.cfg.GrowthRate * (0.72 + e.rng.Float64()*0.62)
	e.drops = append(e.drops, RainOnWindowDrop{
		X:        x,
		Y:        y,
		Radius:   r,
		Mass:     r * r,
		Growth:   growth,
		Tone:     e.rng.Float64(),
		TrailTop: y,
	})
	return true
}

func (e *RainOnWindow) rollNucleationLocked() {
	chance := e.cfg.NucleationRate
	if e.introTicks > 0 {
		progress := phaseProgress(e.introTotal, e.introTicks)
		chance *= e.cfg.IntroDensity * (1.18 - 0.42*progress)
		if progress < 0.18 {
			chance += 0.24
		}
	}
	if e.quietTicks > 0 {
		chance *= e.cfg.QuietPaneMult
	}
	for chance > 0 {
		p := math.Min(1, chance)
		if e.rng.Float64() < p {
			_ = e.spawnDropLocked(true)
		}
		chance -= 1
	}
}

func (e *RainOnWindow) growStationaryDropsLocked() {
	for i := range e.drops {
		d := &e.drops[i]
		d.Age++
		if d.Falling {
			continue
		}
		quiet := 1.0
		if e.quietTicks > 0 {
			quiet = 0.45
		}
		d.Radius += d.Growth * quiet * (0.72 + 0.08*d.Radius)
		d.Mass = d.Radius * d.Radius
		if d.Radius >= e.cfg.FallThreshold {
			e.startDropFallLocked(i)
		}
	}
}

func (e *RainOnWindow) advanceDropsLocked() {
	if len(e.drops) == 0 {
		return
	}
	dst := e.drops[:0]
	for i := range e.drops {
		d := e.drops[i]
		d.Age++
		if d.Falling {
			wind := e.cfg.Wind*0.08 + e.windBias*0.12
			d.Speed += 0.003 + d.Mass*0.0008
			d.Y += d.Speed
			d.X += d.Drift + wind
			if d.Y > float64(e.H)+d.Radius || d.X < -6 || d.X > float64(e.W)+6 {
				e.addTrackLocked(d.X, d.TrailTop, math.Min(d.Y, float64(e.H-1)), math.Max(0.7, d.Radius*0.56), d.Tone)
				continue
			}
		}
		dst = append(dst, d)
	}
	e.drops = dst
}

func (e *RainOnWindow) advanceTracksLocked() {
	if len(e.tracks) == 0 {
		return
	}
	dst := e.tracks[:0]
	decay := 1
	if e.endingTicks > 0 {
		decay = 3
	}
	for _, tr := range e.tracks {
		tr.Life -= decay
		if tr.Life > 0 {
			dst = append(dst, tr)
		}
	}
	e.tracks = dst
}

func (e *RainOnWindow) resolveMergesLocked(limit int, logged bool) int {
	merged := 0
	for merged < limit {
		if !e.mergeOnePairLocked(logged) {
			return merged
		}
		merged++
	}
	return merged
}

func (e *RainOnWindow) mergeOnePairLocked(logged bool) bool {
	bestI, bestJ := -1, -1
	best := math.MaxFloat64
	for i := 0; i < len(e.drops); i++ {
		a := e.drops[i]
		for j := i + 1; j < len(e.drops); j++ {
			b := e.drops[j]
			dx := a.X - b.X
			dy := a.Y - b.Y
			dist := math.Hypot(dx, dy)
			limit := (a.Radius + b.Radius) * e.cfg.MergeRadius
			if dist <= limit && dist < best {
				best = dist
				bestI = i
				bestJ = j
			}
		}
	}
	if bestI < 0 {
		return false
	}
	a := e.drops[bestI]
	b := e.drops[bestJ]
	mass := math.Max(0.1, a.Mass+b.Mass)
	x := (a.X*a.Mass + b.X*b.Mass) / mass
	y := (a.Y*a.Mass + b.Y*b.Mass) / mass
	merged := RainOnWindowDrop{
		X:        x,
		Y:        y,
		Radius:   math.Min(5.5, math.Sqrt(mass)),
		Mass:     mass,
		Growth:   math.Max(a.Growth, b.Growth) * 1.03,
		Tone:     (a.Tone*a.Mass + b.Tone*b.Mass) / mass,
		Age:      max(a.Age, b.Age),
		Falling:  a.Falling || b.Falling,
		Speed:    math.Max(a.Speed, b.Speed),
		Drift:    (a.Drift + b.Drift) * 0.5,
		TrailTop: math.Min(nonZeroTrailTop(a), nonZeroTrailTop(b)),
	}
	if merged.TrailTop == 0 {
		merged.TrailTop = y
	}
	e.drops[bestI] = merged
	e.drops = append(e.drops[:bestJ], e.drops[bestJ+1:]...)
	if !merged.Falling && merged.Radius >= e.cfg.FallThreshold {
		e.startDropFallLocked(bestI)
	}
	if logged {
		e.appendLogLocked("drop-merge", fmt.Sprintf("combined radius %.1f", merged.Radius))
	}
	return true
}

func nonZeroTrailTop(d RainOnWindowDrop) float64 {
	if d.TrailTop != 0 {
		return d.TrailTop
	}
	return d.Y
}

func (e *RainOnWindow) forceMergeLocked() bool {
	if e.mergeOnePairLocked(true) {
		return true
	}
	if len(e.drops) == 0 {
		_ = e.spawnDropLocked(false)
	}
	if len(e.drops) < e.cfg.DropCap {
		base := e.drops[e.rng.Intn(len(e.drops))]
		r := 0.55 + e.rng.Float64()*0.45
		e.drops = append(e.drops, RainOnWindowDrop{
			X:        clampFloat(base.X+base.Radius*0.65, 0, float64(e.W-1)),
			Y:        clampFloat(base.Y+base.Radius*0.15, 0, float64(e.H-1)),
			Radius:   r,
			Mass:     r * r,
			Growth:   e.cfg.GrowthRate,
			Tone:     e.rng.Float64(),
			TrailTop: base.Y,
		})
	}
	return e.mergeOnePairLocked(true)
}

func (e *RainOnWindow) forceFallLocked() bool {
	if len(e.drops) == 0 {
		_ = e.spawnDropLocked(false)
	}
	best := -1
	bestMass := -1.0
	for i, d := range e.drops {
		if d.Falling {
			continue
		}
		if d.Mass > bestMass {
			best = i
			bestMass = d.Mass
		}
	}
	if best < 0 {
		return false
	}
	if e.drops[best].Radius < e.cfg.FallThreshold {
		e.drops[best].Radius = e.cfg.FallThreshold * (1.02 + e.rng.Float64()*0.08)
		e.drops[best].Mass = e.drops[best].Radius * e.drops[best].Radius
	}
	e.startDropFallLocked(best)
	return true
}

func (e *RainOnWindow) startDropFallLocked(i int) {
	if i < 0 || i >= len(e.drops) || e.drops[i].Falling {
		return
	}
	d := &e.drops[i]
	d.Falling = true
	d.Speed = e.cfg.FallSpeed * (0.82 + e.rng.Float64()*0.36) * (0.82 + math.Min(1.2, d.Mass*0.07))
	d.Drift = (e.rng.Float64()*2 - 1) * 0.045
	d.Drift += e.cfg.Wind * 0.035
	d.Drift += e.windBias * 0.06
	d.TrailTop = d.Y
}

func (e *RainOnWindow) addTrackLocked(x, y0, y1, width, tone float64) {
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	life := jitterInt(e.rng, e.cfg.TrackLife, 0.25)
	e.tracks = append(e.tracks, RainOnWindowTrack{
		X:         x,
		Y0:        clampFloat(y0, 0, float64(max(0, e.H-1))),
		Y1:        clampFloat(y1, 0, float64(max(0, e.H-1))),
		Width:     width,
		Life:      life,
		TotalLife: life,
		Tone:      tone,
	})
	if len(e.tracks) > 160 {
		e.tracks = e.tracks[len(e.tracks)-160:]
	}
}

func (e *RainOnWindow) rollEventChancesLocked() {
	if e.cfg.DropFormChance > 0 && e.rng.Float64() < e.cfg.DropFormChance {
		n := e.formBurstLocked(true)
		e.appendLogLocked("drop-form", fmt.Sprintf("rolled %d bead(s)", n))
	}
	if e.cfg.DropMergeChance > 0 && e.rng.Float64() < e.cfg.DropMergeChance {
		if e.forceMergeLocked() {
			e.appendLogLocked("drop-merge", "rolled")
		}
	}
	if e.cfg.DropFallChance > 0 && e.rng.Float64() < e.cfg.DropFallChance {
		if e.forceFallLocked() {
			e.appendLogLocked("drop-fall", "rolled")
		}
	}
	if e.cfg.WindGustChance > 0 && e.rng.Float64() < e.cfg.WindGustChance {
		e.startWindGustLocked("rolled")
	}
	if e.cfg.QuietPaneChance > 0 && e.rng.Float64() < e.cfg.QuietPaneChance {
		e.startQuietPaneLocked("rolled")
	}
}

func (e *RainOnWindow) startWindGustLocked(verb string) {
	e.windTotal = jitterInt(e.rng, e.cfg.WindGustDur, 0.25)
	e.windTicks = e.windTotal
	dir := 1.0
	if e.rng.Float64() < 0.5 {
		dir = -1
	}
	e.windBias = dir * e.cfg.WindGustStrength * (0.75 + e.rng.Float64()*0.5)
	for i := range e.drops {
		if e.drops[i].Falling {
			e.drops[i].Drift += e.windBias * 0.035
		}
	}
	e.appendLogLocked("wind-gust", fmt.Sprintf("%s (dur=%d, bias=%.2f)", verb, e.windTicks, e.windBias))
}

func (e *RainOnWindow) startQuietPaneLocked(verb string) {
	e.quietTotal = jitterInt(e.rng, e.cfg.QuietPaneDur, 0.2)
	e.quietTicks = e.quietTotal
	e.appendLogLocked("quiet-pane", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, e.quietTicks, e.cfg.QuietPaneMult))
}

func (e *RainOnWindow) startIntroLocked() {
	e.ended = false
	e.endingTicks = 0
	e.endingTotal = 0
	e.drops = nil
	e.tracks = nil
	e.windTicks = 0
	e.windTotal = 0
	e.windBias = 0
	e.quietTicks = 0
	e.quietTotal = 0
	e.introTotal = max(1, e.cfg.IntroDur)
	e.introTicks = e.introTotal
}

func (e *RainOnWindow) startEndingLocked() {
	e.introTicks = 0
	e.introTotal = 0
	e.quietTicks = 0
	e.quietTotal = 0
	e.ended = false
	e.endingTotal = max(1, e.cfg.EndingDur+e.cfg.EndingLinger)
	e.endingTicks = e.endingTotal
	for i := range e.drops {
		if !e.drops[i].Falling {
			e.startDropFallLocked(i)
		}
	}
}

func (e *RainOnWindow) countFallingLocked() int {
	count := 0
	for _, d := range e.drops {
		if d.Falling {
			count++
		}
	}
	return count
}

func (e *RainOnWindow) renderLocked() {
	if e.W < 1 || e.H < 1 {
		return
	}
	if len(e.Grid) != e.H || len(e.Grid[0]) != e.W {
		e.Grid = newPixelGrid(e.W, e.H)
	}
	level := e.sceneLevelLocked()
	for y := 0; y < e.H; y++ {
		for x := 0; x < e.W; x++ {
			e.Grid[y][x] = Pixel{Filled: true, C: e.backgroundColorLocked(x, y, level)}
		}
	}
	if !e.ended {
		for _, tr := range e.tracks {
			e.paintTrackLocked(tr.X, tr.Y0, tr.Y1, tr.Width, e.trackAlphaLocked(tr), tr.Tone)
		}
		for _, d := range e.drops {
			if d.Falling {
				e.paintTrackLocked(d.X, d.TrailTop, d.Y, math.Max(0.7, d.Radius*0.54), e.cfg.TrackStrength*0.78, d.Tone)
			}
		}
		for _, d := range e.drops {
			e.paintDropLocked(d)
		}
	}
	e.paintWindowFrameLocked(level)
}

func (e *RainOnWindow) sceneLevelLocked() float64 {
	if e.ended {
		return 0.055
	}
	level := 1.0
	if e.introTicks > 0 {
		p := smoothstep(phaseProgress(e.introTotal, e.introTicks))
		level = 0.16 + 0.84*p
	}
	if e.endingTicks > 0 {
		p := smoothstep(phaseProgress(e.endingTotal, e.endingTicks))
		level = math.Min(level, 0.055+0.945*(1-p))
	}
	return clamp01(level)
}

func (e *RainOnWindow) backgroundColorLocked(x, y int, level float64) color.RGBA {
	nx := 0.0
	ny := 0.0
	if e.W > 1 {
		nx = float64(x) / float64(e.W-1)
	}
	if e.H > 1 {
		ny = float64(y) / float64(e.H-1)
	}

	mainGlow := radialFalloff(nx, ny, 0.32, 0.42, 0.34, 0.42)
	sideGlow := radialFalloff(nx, ny, 0.73, 0.56, 0.18, 0.24)
	streetA := softBand(nx, 0.18, 0.035) * softBand(ny, 0.64, 0.26)
	streetB := softBand(nx, 0.55, 0.06) * softBand(ny, 0.46, 0.32)
	streetC := softBand(nx, 0.84, 0.045) * softBand(ny, 0.74, 0.18)
	city := streetA*0.42 + streetB*0.34 + streetC*0.28
	vignette := 1 - 0.55*radialFalloff(nx, ny, 0.5, 0.5, 0.78, 0.82)
	vertical := 0.12 + 0.12*(1-ny)

	hue := math.Mod(e.cfg.BackgroundHue+e.cfg.BackgroundHueSp*(0.42*sideGlow+0.26*streetC-0.18*mainGlow)+360, 360)
	light := 0.028 + vertical + e.cfg.Glow*(0.38*mainGlow+0.22*sideGlow+0.28*city)
	light *= 0.68 + 0.32*vignette
	light *= level
	return hslToRGB(hue, clamp01(e.cfg.Saturation), clamp01(light))
}

func (e *RainOnWindow) trackAlphaLocked(tr RainOnWindowTrack) float64 {
	if tr.TotalLife <= 0 {
		return 0
	}
	return e.cfg.TrackStrength * clamp01(float64(tr.Life)/float64(tr.TotalLife))
}

func (e *RainOnWindow) paintTrackLocked(x, y0, y1, width, alpha, tone float64) {
	if alpha <= 0 {
		return
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	top := max(0, int(math.Floor(y0)))
	bottom := min(e.H-1, int(math.Ceil(y1)))
	if bottom < top {
		return
	}
	radius := math.Max(0.6, width)
	light := hslToRGB(math.Mod(e.cfg.BackgroundHue+18+tone*e.cfg.BackgroundHueSp+360, 360), 0.13, 0.78)
	shadow := color.RGBA{R: 8, G: 10, B: 14, A: 255}
	for y := top; y <= bottom; y++ {
		progress := 0.0
		if bottom > top {
			progress = float64(y-top) / float64(bottom-top)
		}
		center := x + math.Sin(float64(y)*0.075+tone*6.2)*radius*0.28
		localAlpha := alpha * (0.62 + 0.38*progress)
		minX := int(math.Floor(center - radius - 1))
		maxX := int(math.Ceil(center + radius + 1))
		for px := minX; px <= maxX; px++ {
			dist := math.Abs(float64(px) - center)
			if dist > radius+0.9 {
				continue
			}
			core := clamp01(1 - dist/(radius+0.9))
			if dist > radius*0.75 {
				blendRainPixel(e.Grid, px, y, shadow, localAlpha*0.18*core)
			} else {
				blendRainPixel(e.Grid, px, y, light, localAlpha*0.36*core)
			}
		}
	}
}

func (e *RainOnWindow) paintDropLocked(d RainOnWindowDrop) {
	if d.Radius <= 0 {
		return
	}
	radius := d.Radius
	minX := int(math.Floor(d.X - radius - 1))
	maxX := int(math.Ceil(d.X + radius + 1))
	minY := int(math.Floor(d.Y - radius - 1))
	maxY := int(math.Ceil(d.Y + radius + 1))
	highlight := hslToRGB(math.Mod(e.cfg.BackgroundHue+26+d.Tone*e.cfg.BackgroundHueSp+360, 360), 0.12, 0.88)
	core := hslToRGB(math.Mod(e.cfg.BackgroundHue+10+360, 360), 0.10, 0.68)
	shadow := color.RGBA{R: 5, G: 7, B: 10, A: 255}
	alpha := e.cfg.DropContrast
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx := (float64(x) - d.X) / radius
			dy := (float64(y) - d.Y) / math.Max(0.7, radius*1.08)
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 1.12 {
				continue
			}
			edge := clamp01((dist - 0.72) / 0.4)
			body := clamp01(1 - dist*0.72)
			blendRainPixel(e.Grid, x, y, core, alpha*0.16*body)
			if edge > 0 {
				blendRainPixel(e.Grid, x, y, shadow, alpha*0.20*edge)
			}
			hx := d.X - radius*0.34
			hy := d.Y - radius*0.38
			hd := math.Hypot(float64(x)-hx, float64(y)-hy)
			if hd <= math.Max(0.6, radius*0.42) {
				ha := alpha * 0.52 * (1 - hd/math.Max(0.6, radius*0.42))
				blendRainPixel(e.Grid, x, y, highlight, ha)
			}
		}
	}
}

func (e *RainOnWindow) paintWindowFrameLocked(level float64) {
	frame := max(1, int(math.Round(math.Min(float64(e.W), float64(e.H))*0.014)))
	sill := max(frame+1, int(math.Round(float64(frame)*1.7)))
	alpha := e.cfg.FrameStrength * (0.72 + 0.28*level)
	c := color.RGBA{R: 5, G: 6, B: 8, A: 255}
	for y := 0; y < e.H; y++ {
		for x := 0; x < e.W; x++ {
			if x < frame || x >= e.W-frame || y < frame || y >= e.H-sill {
				blendRainPixel(e.Grid, x, y, c, alpha)
			}
		}
	}
}

func radialFalloff(x, y, cx, cy, rx, ry float64) float64 {
	if rx <= 0 || ry <= 0 {
		return 0
	}
	dx := (x - cx) / rx
	dy := (y - cy) / ry
	d := dx*dx + dy*dy
	return math.Exp(-d * 2.15)
}

func softBand(value, center, width float64) float64 {
	if width <= 0 {
		return 0
	}
	d := math.Abs(value-center) / width
	return math.Exp(-d * d)
}

func smoothstep(v float64) float64 {
	v = clamp01(v)
	return v * v * (3 - 2*v)
}

func blendRainPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	prev := grid[y][x].C
	if !grid[y][x].Filled {
		prev = color.RGBA{}
	}
	grid[y][x] = Pixel{Filled: true, C: color.RGBA{
		R: uint8(float64(prev.R)*(1-alpha) + float64(c.R)*alpha + 0.5),
		G: uint8(float64(prev.G)*(1-alpha) + float64(c.G)*alpha + 0.5),
		B: uint8(float64(prev.B)*(1-alpha) + float64(c.B)*alpha + 0.5),
		A: 255,
	}}
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func countRainOnWindowFilled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled && p.C != (color.RGBA{}) {
				count++
			}
		}
	}
	return count
}
