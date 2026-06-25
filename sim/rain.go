// Package sim implements the ambient effect simulations.
//
// Each effect is a grid of Pixels evolved by a Step() function. The server
// ticks the current effect and broadcasts snapshots; clients render whatever
// they receive without running the simulation themselves.
package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// Pixel is one cell in the grid. Filled=false means transparent/empty.
type Pixel struct {
	Filled bool
	C      color.RGBA
}

// drop is one active raindrop. Its Row/Col are continuous; each tick the drop
// advances by (vRow, vCol) and re-paints its head + trail of streakLen cells
// into the grid. Per-drop fields carry the jitter/layer state picked at spawn.
type drop struct {
	Row, Col   float64
	Color      color.RGBA
	vRow, vCol float64 // movement vector per tick
	streakLen  int     // trail length (including head) for this drop
	background bool    // far half of the depth field (kept for any layer-keyed code)
	depth      float64 // synthetic distance 0=near .. 1=far
	widthCells int     // brush width in grid cells (>=1); near drops wider
	dMm        float64 // physical drop diameter (mm) — provenance for audit/introspection
}

// Config tunes Rain. Every knob is a continuous spectrum so the dev UI can
// expose them as sliders. Zero values fall back to sensible defaults via
// withDefaults().
type Config struct {
	// MOTION
	Wind        float64 `json:"wind"`
	WindJitter  float64 `json:"wind_jit"`
	Speed       float64 `json:"speed"`
	SpeedJitter float64 `json:"speed_jit"`
	// INTRODUCTION
	IntroStyle  int     `json:"intro_style"`
	IntroDur    int     `json:"intro_dur"`
	IntroSparse float64 `json:"intro_sparse"`
	IntroOpen   float64 `json:"intro_open"`
	IntroSeed   int     `json:"intro_seed"`
	// ENDING
	EndingStyle    int `json:"ending_style"`
	EndingDur      int `json:"ending_dur"`
	EndingLinger   int `json:"ending_linger"`
	EndingSplashes int `json:"ending_splashes"`
	// SHAPE
	StreakLen  int     `json:"streak"`
	FadeFactor float64 `json:"fade"`
	DropWidth  float64 `json:"drop_width"`
	// SPAWN
	SpawnEvery int `json:"spawn"`
	SpawnBurst int `json:"burst"`
	// COLOR
	Hue          float64 `json:"hue"`
	HueSpread    float64 `json:"hue_sp"`
	Saturation   float64 `json:"sat"`
	LightnessMin float64 `json:"lmin"`
	LightnessMax float64 `json:"lmax"`
	// DEPTH
	Layers       int     `json:"layers"`
	LayerBalance float64 `json:"lbal"`
	// Overlay is a near-depth cutoff in [0,1]: drops whose synthetic depth is at
	// or below it are ALSO emitted (unchanged) via OverlayGridCopy, a separate
	// frame a consumer composites IN FRONT of its own UI so the nearest drops
	// cross over the page. 0 = none. Those drops still paint into the main grid
	// exactly as before, so the back field is identical whatever this is set to.
	Overlay float64 `json:"overlay"`
	// TEXTURE
	SheetDensity  float64 `json:"sheet"`
	SheetStrength float64 `json:"sheet_alpha"`
	SheetLength   int     `json:"sheet_len"`
	SheetSpeed    float64 `json:"sheet_speed"`
	FrontDensity  float64 `json:"front"`
	FrontStrength float64 `json:"front_alpha"`
	FrontLength   int     `json:"front_len"`
	FrontSpeed    float64 `json:"front_speed"`
	// LEVERS
	HueDriftAmp  float64 `json:"hue_drift"`
	WindDriftAmp float64 `json:"wind_drift"`
	// EVENT CHANCES
	DownpourChance float64 `json:"downpour_p"`
	CalmChance     float64 `json:"calm_p"`
	GustChance     float64 `json:"gust_p"`
	SplashChance   float64 `json:"splash_p"`
	// EVENT MODIFIERS
	DownpourDur  int     `json:"downpour_dur"`
	DownpourMult float64 `json:"downpour_mult"`
	CalmDur      int     `json:"calm_dur"`
	GustDur      int     `json:"gust_dur"`
	GustStrength float64 `json:"gust_str"`
	SplashSize   int     `json:"splash_size"`
}

// NormalizeConfig applies Rain's defaulting rules so callers outside the sim
// package can reason about the effective config without constructing a Rain.
func NormalizeConfig(c Config) Config {
	return c.withDefaults()
}

func (c Config) withDefaults() Config {
	if c.Speed <= 0 {
		c.Speed = 1.8
	}
	if c.SpawnEvery <= 0 {
		c.SpawnEvery = 3
	}
	if c.SpawnBurst <= 0 {
		c.SpawnBurst = 4
	}
	if c.StreakLen <= 0 {
		c.StreakLen = 12
	}
	if c.DropWidth <= 0 {
		// Width tracks apparent drop diameter: the biggest, nearest drops are
		// this many cells wide and taper to 1 for distant drizzle. Real rain's
		// big drops fall faster (Gunn–Kinzer) AND are larger, so fast streaks
		// should be thick, not thin wisps. Set to 1 for uniform one-cell rain.
		c.DropWidth = 3
	}
	if c.IntroDur <= 0 {
		c.IntroDur = 360
	}
	if c.IntroSparse < 1 {
		c.IntroSparse = 8
	}
	if c.IntroOpen <= 0 {
		c.IntroOpen = 0.08
	}
	c.IntroOpen = clamp01(c.IntroOpen)
	if c.IntroSeed < 0 {
		c.IntroSeed = 0
	}
	if c.IntroSeed == 0 {
		c.IntroSeed = 4
	}
	if c.EndingStyle == 0 && c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingSplashes == 0 {
		c.EndingDur = 360
		c.EndingLinger = 120
		c.EndingSplashes = 3
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 360
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingSplashes < 0 {
			c.EndingSplashes = 0
		}
	}
	if c.FadeFactor <= 0 {
		c.FadeFactor = 0.91
	}
	if c.Hue == 0 {
		c.Hue = 214
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.32
	}
	if c.LightnessMin <= 0 {
		c.LightnessMin = 0.34
	}
	if c.LightnessMax <= 0 {
		c.LightnessMax = 0.68
	}
	if c.LightnessMax < c.LightnessMin {
		c.LightnessMin, c.LightnessMax = c.LightnessMax, c.LightnessMin
	}
	if c.Layers <= 0 {
		c.Layers = 2
	}
	if c.LayerBalance <= 0 {
		c.LayerBalance = 0.55
	}
	// Overlay defaults to 0 (off) — a zero value is meaningful, so only bound it.
	c.Overlay = clamp01(c.Overlay)
	if c.SheetStrength <= 0 {
		c.SheetStrength = 0.3
	}
	if c.SheetLength <= 0 {
		c.SheetLength = 11
	}
	if c.SheetSpeed <= 0 {
		c.SheetSpeed = 1.65
	}
	if c.FrontStrength <= 0 {
		c.FrontStrength = 0.55
	}
	if c.FrontLength <= 0 {
		c.FrontLength = 24
	}
	if c.FrontSpeed <= 0 {
		c.FrontSpeed = 54
	}
	if c.DownpourDur <= 0 {
		c.DownpourDur = 360
	}
	if c.DownpourMult <= 0 {
		c.DownpourMult = 3
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 300
	}
	if c.GustDur <= 0 {
		c.GustDur = 180
	}
	if c.GustStrength <= 0 {
		c.GustStrength = 1.5
	}
	if c.SplashSize <= 0 {
		c.SplashSize = 4
	}
	return c
}

// hslToRGB converts HSL to RGBA (alpha=255). h in degrees [0, 360); s, l in [0, 1].
func hslToRGB(h, s, l float64) color.RGBA {
	c := (1 - math.Abs(2*l-1)) * s
	hp := h / 60
	x := c * (1 - math.Abs(math.Mod(hp, 2)-1))
	var rp, gp, bp float64
	switch {
	case hp < 1:
		rp, gp, bp = c, x, 0
	case hp < 2:
		rp, gp, bp = x, c, 0
	case hp < 3:
		rp, gp, bp = 0, c, x
	case hp < 4:
		rp, gp, bp = 0, x, c
	case hp < 5:
		rp, gp, bp = x, 0, c
	default:
		rp, gp, bp = c, 0, x
	}
	m := l - c/2
	return color.RGBA{
		R: uint8(math.Round(clamp01(rp+m) * 255)),
		G: uint8(math.Round(clamp01(gp+m) * 255)),
		B: uint8(math.Round(clamp01(bp+m) * 255)),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// RainSchema describes Rain's tunable knobs for the dev UI.
func RainSchema() EffectSchema {
	return EffectSchema{
		Name: "rain",
		Knobs: []Knob{
			// All tuning knobs are live-applicable — sliding a control affects
			// the running sim. Two flavors, signaled in the description:
			// "rescales in-flight" (affects existing drops immediately, e.g.
			// speed) vs "next drop onward" (applies at spawn time; rain turns
			// over in a few seconds so visible propagation is quick).
			// Introductions belong in the spawn slot: they define how the
			// effect arrives, rather than how the steady-state body behaves.
			{Key: "intro_style", Label: "intro style", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 3, Step: 1, Default: 0,
				Description: "Start-pattern selector: 0=full drizzle, 1=left curtain, 2=center bloom, 3=right curtain. Fire intro to preview."},
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 60, Max: 1440, Step: 30, Default: 360, Trigger: "intro",
				Description: "Ticks the introduction spends ramping from sparse first drops into full rain. Fire button previews the current setup."},
			{Key: "intro_sparse", Label: "intro sparse", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 1, Max: 20, Step: 0.5, Default: 8,
				Description: "How sparse the very first intro drops are relative to steady-state density. 1 = already full, larger = gentler build-in."},
			{Key: "intro_open", Label: "intro open", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.01, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Fraction of the screen initially active for curtain-style intros before the rainy area expands."},
			{Key: "intro_seed", Label: "intro seed", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 4,
				Description: "Drops injected immediately when intro fires so the scene starts with a readable first beat."},
			// Endings belong in the end slot: they shape how the field resolves.
			{Key: "ending_style", Label: "ending style", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 3, Step: 1, Default: 0,
				Description: "Outro selector: 0=full taper, 1=left lane, 2=center thread, 3=right lane. Fire ending to preview."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 60, Max: 1440, Step: 30, Default: 360, Trigger: "ending",
				Description: "Ticks spent thinning the rain before the field settles. Fire button previews the current outro setup."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 720, Step: 30, Default: 120,
				Description: "Extra quiet ticks after spawns stop so the last drops and splashes can resolve before the next state."},
			{Key: "ending_splashes", Label: "ending splashes", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 12, Step: 1, Default: 3,
				Description: "Residual splash beats spread across the outro so the rain can stop before the scene fully settles."},
			{Key: "wind", Label: "wind", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: -3, Max: 3, Step: 0.1, Default: 0,
				Description: "Slope of the rain: cols sideways per row of descent. 0 = straight down, ±1 = 45°. Next drop onward."},
			{Key: "wind_jit", Label: "wind jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Per-drop random variation in wind (± this fraction of base). Adds organic scatter. Next drop onward."},
			{Key: "speed", Label: "speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Base rows descended per tick. Rescales every in-flight drop proportionally."},
			{Key: "speed_jit", Label: "speed jitter", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Per-drop random variation in speed (± this fraction of base). Next drop onward."},
			{Key: "streak", Label: "streak len", Slot: SlotLever, Group: "shape", Type: KnobInt, Min: 1, Max: 16, Step: 1, Default: 12,
				Description: "Pixels painted behind each drop's head, tracing a visible streak. Applied at paint time."},
			{Key: "fade", Label: "fade", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 0.5, Max: 1, Step: 0.01, Default: 0.91,
				Description: "Brightness multiplier per position along a streak. 1.0 = uniform, 0.5 = sharp tail fade. Applied at paint time."},
			{Key: "drop_width", Label: "drop width", Slot: SlotLever, Group: "shape", Type: KnobFloat, Min: 1, Max: 5, Step: 0.5, Default: 3,
				Description: "Cells wide for the biggest, nearest drops (needs layers ≥ 2), tapering to 1 for distant drizzle — so fast heavy drops read thick and far drops thin. 1 = uniform one-cell rain. Next drop onward."},
			{Key: "spawn", Label: "spawn 1/", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 30, Step: 1, Default: 3,
				Description: "Rolls 1 in N per tick for a new drop. Smaller = denser rain."},
			{Key: "burst", Label: "burst max", Slot: SlotLever, Group: "density", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 4,
				Description: "Max drops emitted per spawn event. 1 = no clumping; higher = drops in clusters."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 214,
				Description: "Base hue on the color wheel in degrees (0=red, 120=green, 240=blue). Next drop onward."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 8,
				Description: "Per-drop hue variation (± degrees). Larger = more color variety within the rain. Next drop onward."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.32,
				Description: "Color saturation. 0 = grayscale, 1 = fully vivid. Next drop onward."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.34,
				Description: "Minimum lightness for drop colors. Lower = allows darker drops. Next drop onward."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.95, Step: 0.01, Default: 0.68,
				Description: "Maximum lightness for drop colors. Higher = allows brighter drops. Next drop onward."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "depth", Type: KnobInt, Min: 1, Max: 2, Step: 1, Default: 2,
				Description: "1 = single layer. 2 = adds a dimmer/shorter/slower background layer for parallax depth. Next drop onward."},
			{Key: "lbal", Label: "bg balance", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Fraction of drops assigned to the background layer. Ignored unless layers=2. Next drop onward."},
			{Key: "overlay", Label: "overlay", Slot: SlotLever, Group: "depth", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Near-depth cutoff: drops at or nearer than this (by synthetic depth, 0=near..1=far) ALSO render in a front plane a consumer can composite over its own UI, so the nearest drops cross in front of the page. 0 = none. The same drops still render in the main field unchanged."},
			{Key: "sheet", Label: "sheet", Slot: SlotLever, Group: "texture", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.6,
				Description: "Procedural background rain texture density. 0 = only foreground drops; higher = fuller rain field."},
			{Key: "sheet_alpha", Label: "sheet alpha", Slot: SlotLever, Group: "texture", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.3,
				Description: "Brightness of the procedural rain sheet before foreground drops are painted."},
			{Key: "sheet_len", Label: "sheet len", Slot: SlotLever, Group: "texture", Type: KnobInt, Min: 2, Max: 20, Step: 1, Default: 11,
				Description: "Streak length for the procedural background sheet."},
			{Key: "sheet_speed", Label: "sheet speed", Slot: SlotLever, Group: "texture", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.05, Default: 1.65,
				Description: "Rows per tick for the procedural background sheet."},
			{Key: "front", Label: "front", Slot: SlotLever, Group: "front plane", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Near-window rain streak density. This layer represents rain crossing the screen plane, not slow falling particles."},
			{Key: "front_alpha", Label: "front alpha", Slot: SlotLever, Group: "front plane", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Brightness of front-plane rain flashes."},
			{Key: "front_len", Label: "front len", Slot: SlotLever, Group: "front plane", Type: KnobInt, Min: 4, Max: 48, Step: 1, Default: 24,
				Description: "Streak length for near-window rain flashes."},
			{Key: "front_speed", Label: "front speed", Slot: SlotLever, Group: "front plane", Type: KnobFloat, Min: 8, Max: 100, Step: 1, Default: 54,
				Description: "Rows per tick for front-plane rain. High values model monitor-height rain crossing in only a few frames."},
			{Key: "hue_drift", Label: "hue drift", Slot: SlotLever, Group: "drift", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 0,
				Description: "Amplitude (±degrees) the base hue slowly wanders over ~30s cycles. 0 = static."},
			{Key: "wind_drift", Label: "wind drift", Slot: SlotLever, Group: "drift", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0,
				Description: "Amplitude the effective wind sways around base. 0 = static; creates gentle direction changes."},

			// DISCRETE EVENTS — per-tick probability of firing.
			{Key: "downpour_p", Label: "downpour", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.001, Step: 0.00005, Default: 0, Trigger: "downpour",
				Description: "Per-tick probability of starting a downpour (temporary dense rain burst)."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.001, Step: 0.00005, Default: 0, Trigger: "calm",
				Description: "Per-tick probability of a calm event — drops stop spawning for a while."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.001, Step: 0.00005, Default: 0, Trigger: "gust",
				Description: "Per-tick probability of a wind gust — a sudden sideways push for a stretch of time."},
			{Key: "splash_p", Label: "splash", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.0001, Default: 0, Trigger: "splash",
				Description: "Per-tick probability of a splash — an expanding radial ring at a random point."},

			// EVENT MODIFIERS — typical per-event values (each event randomizes ±30%).
			{Key: "downpour_dur", Label: "downpour dur", Slot: SlotEventMod, Group: "downpour", Type: KnobInt, Min: 60, Max: 1800, Step: 30, Default: 360,
				Description: "Typical downpour duration in ticks (actual value jitters ±30%)."},
			{Key: "downpour_mult", Label: "downpour ×", Slot: SlotEventMod, Group: "downpour", Type: KnobFloat, Min: 1.5, Max: 10, Step: 0.5, Default: 3,
				Description: "Spawn-rate multiplier during a downpour. 3 = three times denser than baseline."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 60, Max: 1800, Step: 30, Default: 300,
				Description: "Typical calm duration in ticks (spawning pauses for this long ±30%)."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 30, Max: 600, Step: 30, Default: 180,
				Description: "Typical gust duration in ticks (how long the wind push lasts ±30%)."},
			{Key: "gust_str", Label: "gust strength", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 0.3, Max: 3, Step: 0.1, Default: 1.5,
				Description: "Magnitude of the extra wind added during a gust. Sign is random per event."},
			{Key: "splash_size", Label: "splash size", Slot: SlotEventMod, Group: "splash", Type: KnobInt, Min: 2, Max: 12, Step: 1, Default: 4,
				Description: "Max splash-ring radius in pixels (actual jitters ±30%)."},
		},
	}
}

// splashInstance is an active splash event — a small radial ring that
// expands outward from (row, col) over maxAge ticks and fades as it grows.
type splashInstance struct {
	row, col  int
	age       int
	maxAge    int
	maxRadius int
	color     color.RGBA
}

// LogEntry is one event-occurrence record. Produced when discrete events
// fire (probabilistic or triggered) so the dev UI can show a live log.
type LogEntry struct {
	Tick int    `json:"tick"`
	Type string `json:"type"`
	Desc string `json:"desc"`
}

// Rain is the rain simulation.
// Thread-safety: Step, SetConfig, Trigger*, and DrainLog are all safe to
// call from multiple goroutines. The mutex serializes them. Grid reads for
// rendering happen inside Step's critical section and the resulting snapshot
// is immutable after that.
type Rain struct {
	mu sync.Mutex

	W, H  int
	Grid  [][]Pixel
	drops []drop
	rng   *rngutil.RNG
	cfg   Config

	// lever state
	tick int

	// event state
	downpourTicks     int
	downpourMult      float64
	calmTicks         int
	gustTicks         int
	gustWind          float64
	splashes          []splashInstance
	introTicks        int
	introTotal        int
	endingTicks       int
	endingTotal       int
	endingFade        int
	endingSplashLeft  int
	endingSplashTotal int

	// log ring — most recent events, bounded. DrainLog returns + clears.
	log []LogEntry
}

// NewRain builds a Rain sim. Zero Config gets sensible defaults.
func NewRain(w, h int, seed int64, cfg Config) *Rain {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Rain{
		W:    w,
		H:    h,
		Grid: grid,
		rng:  rngutil.New(seed),
		cfg:  cfg.withDefaults(),
	}
}

// SetConfig updates the live sim's config without resetting drop/event/tick
// state. Used by dev sessions when the user tweaks a knob — spawn-config
// changes take effect on subsequent drops; lever/event changes apply to the
// running simulation immediately.
//
// Speed is a lever: when it changes, rescale every in-flight drop's velocity
// by the ratio so the slider actually tunes visible rain instead of waiting
// for the old drops to fall off-screen.
func (r *Rain) SetConfig(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newCfg := cfg.withDefaults()
	if r.cfg.Speed > 0 && newCfg.Speed != r.cfg.Speed {
		ratio := newCfg.Speed / r.cfg.Speed
		for i := range r.drops {
			r.drops[i].vRow *= ratio
			r.drops[i].vCol *= ratio
		}
	}
	r.cfg = newCfg
}

// PerturbRNG folds external entropy (e.g. keystroke-derived bytes from
// connected clients) into the sim's RNG without resetting it. Next random
// draw consumes from the perturbed stream — future decisions will differ
// from what they'd have been without the perturbation.
func (r *Rain) PerturbRNG(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Mix(delta)
}

// Resize changes the sim's grid dimensions. Existing drops are dropped
// (re-spawn naturally). Event-timer state is preserved. Safe to call
// concurrently with Step.
func (r *Rain) Resize(w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w == r.W && h == r.H {
		return
	}
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	r.W = w
	r.H = h
	r.Grid = grid
	r.drops = r.drops[:0]
	r.splashes = r.splashes[:0]
}

// State is the subset of Rain state a snapshot exposes to clients so they
// can initialize a matching local replica.
type State struct {
	Tick              int       `json:"tick"`
	DownpourTicks     int       `json:"downpourTicks"`
	DownpourMult      float64   `json:"downpourMult"`
	CalmTicks         int       `json:"calmTicks"`
	GustTicks         int       `json:"gustTicks"`
	GustWind          float64   `json:"gustWind"`
	IntroTicks        int       `json:"introTicks"`
	IntroTotal        int       `json:"introTotal"`
	EndingTicks       int       `json:"endingTicks"`
	EndingTotal       int       `json:"endingTotal"`
	Lifecycle         Lifecycle `json:"lifecycle"`
	EndingFade        int       `json:"endingFade"`
	EndingSplashLeft  int       `json:"endingSplashLeft"`
	EndingSplashTotal int       `json:"endingSplashTotal"`
}

// PersistedState is the server-side subset of Rain state needed to resume
// the atmosphere after a process restart.
type PersistedState struct {
	State
	RNGState uint64   `json:"rngState"`
	Drops    []Drop   `json:"drops"`
	Splashes []Splash `json:"splashes"`
}

// RGB is the wire form of a color — lowercase keys and no alpha, shared
// across all effect snapshots. Internal sim state uses color.RGBA; this
// type only exists at the JSON boundary.
type RGB struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

// Drop is the wire form of an in-flight raindrop, emitted in snapshots so
// joining clients can replicate mid-simulation instead of starting with an
// empty air column.
type Drop struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	Color      RGB     `json:"color"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	StreakLen  int     `json:"streakLen"`
	Background bool    `json:"background"`
	Depth      float64 `json:"depth,omitempty"`
	WidthCells int     `json:"widthCells,omitempty"`
}

// Splash is the wire form of an active splash ring.
type Splash struct {
	Row       int `json:"row"`
	Col       int `json:"col"`
	Age       int `json:"age"`
	MaxAge    int `json:"maxAge"`
	MaxRadius int `json:"maxRadius"`
	Color     RGB `json:"color"`
}

// RainSnapshot is the browser/client-facing state dump used to drop a replica
// into the current simulation without waiting for new particles to spawn.
type RainSnapshot struct {
	State
	RNGState uint64   `json:"rngState,omitempty"`
	Drops    []Drop   `json:"drops"`
	Splashes []Splash `json:"splashes"`
}

// SnapshotState returns a copy of the event-timer state at this instant so
// a joining client can replicate the atmosphere.
func (r *Rain) SnapshotState() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{
		Tick:              r.tick,
		DownpourTicks:     r.downpourTicks,
		DownpourMult:      r.downpourMult,
		CalmTicks:         r.calmTicks,
		GustTicks:         r.gustTicks,
		GustWind:          r.gustWind,
		IntroTicks:        r.introTicks,
		IntroTotal:        r.introTotal,
		EndingTicks:       r.endingTicks,
		EndingTotal:       r.endingTotal,
		Lifecycle:         r.lifecycleLocked(),
		EndingFade:        r.endingFade,
		EndingSplashLeft:  r.endingSplashLeft,
		EndingSplashTotal: r.endingSplashTotal,
	}
}

// lifecycleLocked derives the effect-generic lifecycle contract value from
// rain's internal counters. Rain's outro is non-terminal: once the ending
// fade completes, automatic events resume, so lifecycle returns to running
// (the schema declares ending_terminal: false by omission).
func (r *Rain) lifecycleLocked() Lifecycle {
	switch {
	case r.introTicks > 0:
		return LifecycleIntro
	case r.endingTicks > 0:
		return LifecycleEnding
	default:
		return LifecycleRunning
	}
}

// Snapshot returns the full client-facing wire state for the current Rain sim.
func (r *Rain) Snapshot() RainSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainSnapshot{
		State:    r.snapshotStateLocked(),
		RNGState: r.rng.State(),
		Drops:    r.copyDropsLocked(),
		Splashes: r.copySplashesLocked(),
	}
}

// DropsCopy returns the active drops as wire-form Drop values. Caller owns
// the slice.
func (r *Rain) DropsCopy() []Drop {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyDropsLocked()
}

// SplashesCopy returns the active splashes as wire-form Splash values.
func (r *Rain) SplashesCopy() []Splash {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copySplashesLocked()
}

// CurrentTick returns the current sim tick number.
func (r *Rain) CurrentTick() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tick
}

// EffectiveConfig returns the sim's current config with all defaults applied.
// Used by atmospheres to share the effective values with clients via snapshot.
func (r *Rain) EffectiveConfig() Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// RestoreState overwrites the sim's tick + event-timer state from an external
// snapshot (e.g., the first SSE message to a joining client). Does not touch
// config or drops.
func (r *Rain) RestoreState(s State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tick = s.Tick
	r.downpourTicks = s.DownpourTicks
	r.downpourMult = s.DownpourMult
	r.calmTicks = s.CalmTicks
	r.gustTicks = s.GustTicks
	r.gustWind = s.GustWind
	r.introTicks = s.IntroTicks
	r.introTotal = s.IntroTotal
	r.endingTicks = s.EndingTicks
	r.endingTotal = s.EndingTotal
	r.endingFade = s.EndingFade
	r.endingSplashLeft = s.EndingSplashLeft
	r.endingSplashTotal = s.EndingSplashTotal
}

// RestoreSnapshot overwrites the sim's client-facing state, including active
// drops and splashes, from a full wire snapshot.
func (r *Rain) RestoreSnapshot(s RainSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(s.State)
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}
	r.restoreParticlesLocked(s.Drops, s.Splashes)
}

// SnapshotPersistedState returns the full server-side sim state needed to
// resume from disk, including in-flight particles and RNG state.
func (r *Rain) SnapshotPersistedState() PersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := PersistedState{
		State:    r.snapshotStateLocked(),
		RNGState: r.rng.State(),
		Drops:    r.copyDropsLocked(),
		Splashes: r.copySplashesLocked(),
	}
	return out
}

// RestorePersistedState overwrites the sim from a server-side persisted
// snapshot. Config is handled separately via SetConfig before this call.
func (r *Rain) RestorePersistedState(s PersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.restoreStateLocked(s.State)
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}
	r.restoreParticlesLocked(s.Drops, s.Splashes)
}

func (r *Rain) snapshotStateLocked() State {
	return State{
		Tick:              r.tick,
		DownpourTicks:     r.downpourTicks,
		DownpourMult:      r.downpourMult,
		CalmTicks:         r.calmTicks,
		GustTicks:         r.gustTicks,
		GustWind:          r.gustWind,
		IntroTicks:        r.introTicks,
		IntroTotal:        r.introTotal,
		EndingTicks:       r.endingTicks,
		EndingTotal:       r.endingTotal,
		Lifecycle:         r.lifecycleLocked(),
		EndingFade:        r.endingFade,
		EndingSplashLeft:  r.endingSplashLeft,
		EndingSplashTotal: r.endingSplashTotal,
	}
}

func (r *Rain) copyDropsLocked() []Drop {
	out := make([]Drop, len(r.drops))
	for i, d := range r.drops {
		out[i] = Drop{
			Row:        d.Row,
			Col:        d.Col,
			Color:      RGB{R: d.Color.R, G: d.Color.G, B: d.Color.B},
			VRow:       d.vRow,
			VCol:       d.vCol,
			StreakLen:  d.streakLen,
			Background: d.background,
			Depth:      d.depth,
			WidthCells: d.widthCells,
		}
	}
	return out
}

func (r *Rain) copySplashesLocked() []Splash {
	out := make([]Splash, len(r.splashes))
	for i, s := range r.splashes {
		out[i] = Splash{
			Row:       s.row,
			Col:       s.col,
			Age:       s.age,
			MaxAge:    s.maxAge,
			MaxRadius: s.maxRadius,
			Color:     RGB{R: s.color.R, G: s.color.G, B: s.color.B},
		}
	}
	return out
}

func (r *Rain) restoreStateLocked(s State) {
	r.tick = s.Tick
	r.downpourTicks = s.DownpourTicks
	r.downpourMult = s.DownpourMult
	r.calmTicks = s.CalmTicks
	r.gustTicks = s.GustTicks
	r.gustWind = s.GustWind
	r.introTicks = s.IntroTicks
	r.introTotal = s.IntroTotal
	r.endingTicks = s.EndingTicks
	r.endingTotal = s.EndingTotal
	r.endingFade = s.EndingFade
	r.endingSplashLeft = s.EndingSplashLeft
	r.endingSplashTotal = s.EndingSplashTotal
}

func (r *Rain) restoreParticlesLocked(drops []Drop, splashes []Splash) {
	r.drops = make([]drop, len(drops))
	for i, d := range drops {
		r.drops[i] = drop{
			Row:        d.Row,
			Col:        d.Col,
			Color:      color.RGBA{R: d.Color.R, G: d.Color.G, B: d.Color.B, A: 255},
			vRow:       d.VRow,
			vCol:       d.VCol,
			streakLen:  d.StreakLen,
			background: d.Background,
			depth:      d.Depth,
			widthCells: d.WidthCells,
		}
	}

	r.splashes = make([]splashInstance, len(splashes))
	for i, sp := range splashes {
		r.splashes[i] = splashInstance{
			row:       sp.Row,
			col:       sp.Col,
			age:       sp.Age,
			maxAge:    sp.MaxAge,
			maxRadius: sp.MaxRadius,
			color:     color.RGBA{R: sp.Color.R, G: sp.Color.G, B: sp.Color.B, A: 255},
		}
	}

	r.repaintLocked()
}

// GridCopy returns a snapshot of the current grid. The caller owns the
// returned slice and can read it without holding any sim lock.
func (r *Rain) GridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]Pixel, len(r.Grid))
	for y := range r.Grid {
		out[y] = make([]Pixel, len(r.Grid[y]))
		copy(out[y], r.Grid[y])
	}
	return out
}

// OverlayGridCopy returns a fresh transparent grid holding only the drops at or
// nearer than the Overlay depth cutoff (Config.Overlay). It is computed on
// demand — Step() and the main grid are untouched — so a consumer can composite
// this frame ABOVE its own UI while the very same drops still paint into
// GridCopy's back field. With Overlay 0 nothing qualifies and the grid is empty,
// so reading it never changes the rain.
func (r *Rain) OverlayGridCopy() [][]Pixel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]Pixel, r.H)
	for y := range out {
		out[y] = make([]Pixel, r.W)
	}
	if r.cfg.Overlay <= 0 {
		return out
	}
	for _, d := range r.drops {
		if d.depth <= r.cfg.Overlay {
			r.paintDropInto(out, d)
		}
	}
	return out
}

func (r *Rain) repaintLocked() {
	for y := range r.Grid {
		for x := range r.Grid[y] {
			r.Grid[y][x] = Pixel{}
		}
	}
	r.paintSheet()
	r.paintSplashes()
	for _, d := range r.drops {
		r.paintDrop(d)
	}
	r.paintFrontPlane()
}

// TriggerEvent fires a discrete event immediately, bypassing probability.
// Returns true on recognized event names ("downpour", "calm", "gust", "splash",
// "intro", "ending").
func (r *Rain) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "intro":
		r.startIntroductionLocked()
		// Repaint now so the new (near-empty) intro frame is visible on the very
		// next render even if Step() won't run for a while. A just-joined client
		// sits behind its playback-delay buffer and does not step immediately;
		// without this it would keep showing the stale full-storm grid the
		// snapshot restore painted, then jump — the opposite of easing in.
		r.repaintLocked()
		r.appendLog("intro", fmt.Sprintf("started (%s, dur=%d)", introStyleName(r.cfg.IntroStyle), r.introTotal))
	case "ending":
		r.startEndingLocked()
		r.appendLog("ending", fmt.Sprintf("started (%s, taper=%d, linger=%d, splashes=%d)", endingStyleName(r.cfg.EndingStyle), r.endingFade, r.endingTotal-r.endingFade, r.endingSplashTotal))
	case "downpour":
		r.downpourTicks = jitterInt(r.rng, r.cfg.DownpourDur, 0.3)
		r.downpourMult = r.cfg.DownpourMult
		r.appendLog("downpour", fmt.Sprintf("triggered (dur=%d, ×%.1f)", r.downpourTicks, r.downpourMult))
	case "calm":
		r.calmTicks = jitterInt(r.rng, r.cfg.CalmDur, 0.3)
		r.appendLog("calm", fmt.Sprintf("triggered (dur=%d)", r.calmTicks))
	case "gust":
		r.gustTicks = jitterInt(r.rng, r.cfg.GustDur, 0.3)
		sign := 1.0
		if r.rng.Float64() < 0.5 {
			sign = -1
		}
		r.gustWind = sign * r.cfg.GustStrength * (0.7 + r.rng.Float64()*0.6)
		r.appendLog("gust", fmt.Sprintf("triggered (dur=%d, wind=%+.2f)", r.gustTicks, r.gustWind))
	case "splash":
		r.spawnSplash()
		r.appendLog("splash", "triggered")
	default:
		return false
	}
	return true
}

// DrainLog returns and clears any log entries accumulated since the last drain.
func (r *Rain) DrainLog() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) == 0 {
		return nil
	}
	out := r.log
	r.log = nil
	return out
}

func (r *Rain) appendLog(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

// Step advances the sim by one tick. Flow:
//  1. Tick bookkeeping; decrement active event timers.
//  2. Roll for new events (downpour, calm, gust, splash) if not already active.
//  3. Clear the grid.
//  4. Paint active splashes.
//  5. Roll for drop spawn (respecting calm / downpour multiplier).
//  6. Advance every active drop; paint its head + streak trail.
//  7. Cull drops whose trail has fully exited the bottom.
//  8. Age/remove expired splashes.
//  9. Repaint from the final post-step particle state.
func (r *Rain) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++

	// 1. Decrement active event timers.
	if r.downpourTicks > 0 {
		r.downpourTicks--
	}
	if r.calmTicks > 0 {
		r.calmTicks--
	}
	if r.gustTicks > 0 {
		r.gustTicks--
	} else {
		r.gustWind = 0
	}

	// 2. Roll for new events unless a manual lifecycle beat is currently
	// establishing or resolving the scene. Those phases should read on their own.
	if r.introTicks == 0 && r.endingTicks == 0 {
		if r.downpourTicks == 0 && r.rng.Float64() < r.cfg.DownpourChance {
			r.downpourTicks = jitterInt(r.rng, r.cfg.DownpourDur, 0.3)
			r.downpourMult = r.cfg.DownpourMult
			r.appendLog("downpour", fmt.Sprintf("started (dur=%d, ×%.1f)", r.downpourTicks, r.downpourMult))
		}
		if r.calmTicks == 0 && r.rng.Float64() < r.cfg.CalmChance {
			r.calmTicks = jitterInt(r.rng, r.cfg.CalmDur, 0.3)
			r.appendLog("calm", fmt.Sprintf("started (dur=%d)", r.calmTicks))
		}
		if r.gustTicks == 0 && r.rng.Float64() < r.cfg.GustChance {
			r.gustTicks = jitterInt(r.rng, r.cfg.GustDur, 0.3)
			sign := 1.0
			if r.rng.Float64() < 0.5 {
				sign = -1
			}
			r.gustWind = sign * r.cfg.GustStrength * (0.7 + r.rng.Float64()*0.6)
			r.appendLog("gust", fmt.Sprintf("started (dur=%d, wind=%+.2f)", r.gustTicks, r.gustWind))
		}
		if r.rng.Float64() < r.cfg.SplashChance {
			r.spawnSplash()
			r.appendLog("splash", "fired")
		}
	}

	// 3. Clear.
	for y := range r.Grid {
		for x := range r.Grid[y] {
			r.Grid[y][x] = Pixel{}
		}
	}

	// 4. Paint splashes.
	r.paintSplashes()

	// 5. Spawn drops. Manual lifecycle beats temporarily own the spawn pattern;
	// otherwise steady-state rules apply.
	if r.introTicks > 0 {
		r.stepIntroduction()
	} else if r.endingTicks > 0 {
		r.stepEnding()
	} else {
		effectiveSpawn := r.cfg.SpawnEvery
		if r.downpourTicks > 0 && r.downpourMult > 1 {
			effectiveSpawn = int(float64(r.cfg.SpawnEvery) / r.downpourMult)
			if effectiveSpawn < 1 {
				effectiveSpawn = 1
			}
		}
		if r.calmTicks == 0 && r.rng.Intn(effectiveSpawn) == 0 {
			burst := 1
			if r.cfg.SpawnBurst > 1 {
				burst = 1 + r.rng.Intn(r.cfg.SpawnBurst)
			}
			for i := 0; i < burst; i++ {
				r.spawnDrop()
			}
		}
	}

	// 6 + 7. Advance + paint + cull drops.
	alive := r.drops[:0]
	for _, d := range r.drops {
		d.Row += d.vRow
		d.Col += d.vCol
		r.paintDrop(d)
		trailRowStep, _ := d.trailStep()
		tailRow := d.Row - float64(d.streakLen-1)*trailRowStep
		if tailRow < float64(r.H) && d.Row > -float64(d.streakLen) {
			alive = append(alive, d)
		}
	}
	r.drops = alive

	// 8. Age splashes; drop expired.
	splashesAlive := r.splashes[:0]
	for _, s := range r.splashes {
		s.age++
		if s.age < s.maxAge {
			splashesAlive = append(splashesAlive, s)
		}
	}
	r.splashes = splashesAlive

	r.repaintLocked()
}

// jitterInt returns an int in [base*(1-spread), base*(1+spread)], uniform.
func jitterInt(rng *rngutil.RNG, base int, spread float64) int {
	f := float64(base) * (1 + spread*(rng.Float64()*2-1))
	n := int(math.Round(f))
	if n < 1 {
		n = 1
	}
	return n
}

// currentHue returns the base hue drifted by the HueDriftAmp lever. The
// coefficient is calibrated for the 60 Hz rain baseline; 0 amplitude = static.
func (r *Rain) currentHue() float64 {
	base := r.cfg.Hue
	if r.cfg.HueDriftAmp > 0 {
		base += r.cfg.HueDriftAmp * math.Sin(float64(r.tick)*0.0035)
	}
	return math.Mod(base+360, 360)
}

// currentWind returns the base wind drifted by WindDriftAmp plus any active
// gust delta.
func (r *Rain) currentWind() float64 {
	w := r.cfg.Wind
	if r.cfg.WindDriftAmp > 0 {
		w += r.cfg.WindDriftAmp * math.Sin(float64(r.tick)*0.0022+1.7)
	}
	w += r.gustWind
	return w
}

// layerIntensity scales the procedural rain layers (sheet + front plane) during
// the intro/ending lifecycle beats so the WHOLE field ramps, not just the
// foreground drops. The procedural layers are tick-driven and would otherwise
// render at full density the instant a client joins — defeating the "it just
// started raining" entrance. Returns 1 in steady state (no change).
func (r *Rain) layerIntensity() float64 {
	switch {
	case r.introTicks > 0:
		// 0 at the first intro tick, ramping to 1 as the intro completes —
		// matches the foreground spawn ramp in stepIntroduction.
		return phaseProgress(r.introTotal, r.introTicks)
	case r.endingTicks > 0:
		// Fade the procedural field out across the ending's fade window; gone
		// through the trailing linger so the last drops resolve over bare grid.
		if r.endingFade <= 1 {
			return 0
		}
		elapsed := r.endingTotal - r.endingTicks
		if elapsed >= r.endingFade {
			return 0
		}
		return clamp01(1 - float64(elapsed)/float64(r.endingFade-1))
	default:
		return 1
	}
}

func introStyle(style int) int {
	switch style {
	case 1, 2, 3:
		return style
	default:
		return 0
	}
}

func introStyleName(style int) string {
	switch introStyle(style) {
	case 1:
		return "left-curtain"
	case 2:
		return "center-bloom"
	case 3:
		return "right-curtain"
	default:
		return "full-drizzle"
	}
}

func phaseProgress(total, left int) float64 {
	if left <= 1 || total <= 1 {
		return 1
	}
	elapsed := total - left
	if elapsed <= 0 {
		return 0
	}
	return clamp01(float64(elapsed) / float64(total-1))
}

func endingStyle(style int) int {
	switch style {
	case 1, 2, 3:
		return style
	default:
		return 0
	}
}

func endingStyleName(style int) string {
	switch endingStyle(style) {
	case 1:
		return "left-lane"
	case 2:
		return "center-thread"
	case 3:
		return "right-lane"
	default:
		return "full-taper"
	}
}

func (r *Rain) introColumnRange(progress float64) (float64, float64) {
	if r.W <= 1 {
		return 0, float64(r.W)
	}
	if introStyle(r.cfg.IntroStyle) == 0 {
		return 0, float64(r.W)
	}
	openFrac := clamp01(r.cfg.IntroOpen)
	if openFrac <= 0 {
		openFrac = 0.08
	}
	widthFrac := openFrac + (1-openFrac)*clamp01(progress)
	width := widthFrac * float64(r.W)
	if width < 1 {
		width = 1
	}
	switch introStyle(r.cfg.IntroStyle) {
	case 1:
		return 0, math.Min(float64(r.W), width)
	case 2:
		center := float64(r.W) / 2
		half := width / 2
		return math.Max(0, center-half), math.Min(float64(r.W), center+half)
	case 3:
		return math.Max(0, float64(r.W)-width), float64(r.W)
	default:
		return 0, float64(r.W)
	}
}

func (r *Rain) endingColumnRange(progress float64) (float64, float64) {
	if r.W <= 1 {
		return 0, float64(r.W)
	}
	if endingStyle(r.cfg.EndingStyle) == 0 {
		return 0, float64(r.W)
	}
	width := (1 - clamp01(progress)) * float64(r.W)
	if width < 1 {
		width = 1
	}
	switch endingStyle(r.cfg.EndingStyle) {
	case 1:
		return 0, math.Min(float64(r.W), width)
	case 2:
		center := float64(r.W) / 2
		half := width / 2
		return math.Max(0, center-half), math.Min(float64(r.W), center+half)
	case 3:
		return math.Max(0, float64(r.W)-width), float64(r.W)
	default:
		return 0, float64(r.W)
	}
}

// refGridH is the grid height the rain config was calibrated against (see
// docs/rain-visual-model.md): at 180 rows a tracked-drop speed of ~1.8
// rows/tick crosses the viewport in ~1.7s. Fall speed and streak/sheet/front
// lengths are expressed in that reference and scaled by resScale() so the
// screen-relative motion and proportions stay constant at any grid resolution.
// Raising the grid then only makes the pixels finer — it must not slow the rain
// or shorten the streaks (a regression we hit when the grid went 180→360).
const refGridH = 180

func (r *Rain) resScale() float64 {
	// Only scale up for grids taller than the reference. Smaller grids (tests,
	// the terminal) keep the config's literal cell values, so this never shrinks
	// or slows rain below what a scene declares — it just stops a finer grid from
	// making it crawl.
	if r.H <= refGridH {
		return 1
	}
	return float64(r.H) / refGridH
}

// lerpDepth interpolates a perceptual factor from near (t=0) to far (t=1).
func lerpDepth(near, far, t float64) float64 { return near + (far-near)*t }

// refRainMm is the reference raindrop diameter (mm). ~1.3mm is near the
// median-volume diameter of moderate rain.
const refRainMm = 1.3

// Physical scale anchoring. Drop fall speed is derived from real terminal
// velocity (m/s) projected onto the viewport, instead of an arbitrary
// rows/tick. worldHeightM is the one genuinely artistic choice: how many metres
// of falling rain the viewport spans. ~4.5m reads as rain seen across a scene —
// a reference 1.3mm drop (~4.9 m/s) then crosses in ~0.9s, the biggest drops in
// ~0.5s, distant drizzle in ~2-3s. Smaller = closer/faster rain. Because the
// projection multiplies by grid height, fall speed is automatically
// resolution-independent (a finer grid only thins the pixels). refSpeedKnob is
// the Speed value that means "nominal intensity" (×1); the scene Speed knob
// scales around it.
const (
	clientFPS    = 60.0
	worldHeightM = 4.5
	refSpeedKnob = 1.8
)

// gunnKinzer returns raindrop terminal velocity (m/s) for diameter d (mm) using
// the Gunn–Kinzer (1949) empirical fit: ~2 m/s at 0.5mm rising to ~9 m/s near
// 5mm. This is what makes big drops fall visibly faster than small ones.
func gunnKinzer(dMm float64) float64 {
	v := 9.65 - 10.3*math.Exp(-0.6*dMm)
	if v < 0.5 {
		v = 0.5
	}
	return v
}

// ── Introspection / audit surface ───────────────────────────────────────────

// DropInfo is the full physical provenance of one drop in human-meaningful
// units, for the rain-audit harness and the /rain/debug endpoint. It exposes the
// inputs (diameter, distance) and the derived percept (apparent velocity in m/s,
// seconds to cross the screen) so the physics chain is verifiable end-to-end
// instead of inferred from raw rows/tick.
type DropInfo struct {
	DiameterMm   float64 `json:"diameterMm"`
	Distance     float64 `json:"distance"`     // 0 near .. 1 far
	TerminalMS   float64 `json:"terminalMS"`   // free-fall terminal velocity (Gunn–Kinzer)
	ApparentMS   float64 `json:"apparentMS"`   // on-screen apparent fall speed, m/s
	RowsPerTick  float64 `json:"rowsPerTick"`  // raw sim velocity
	SecondsCross float64 `json:"secondsCross"` // time to fall the full viewport
	WidthCells   int     `json:"widthCells"`
	StreakCells  int     `json:"streakCells"`
	Brightness   float64 `json:"brightness"` // luminance 0..255
}

// DropProvenance returns the physical provenance of every live drop. Read-only.
func (r *Rain) DropProvenance() []DropInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := float64(r.H)
	out := make([]DropInfo, len(r.drops))
	for i, d := range r.drops {
		lum := 0.299*float64(d.Color.R) + 0.587*float64(d.Color.G) + 0.114*float64(d.Color.B)
		var apparentMS, secCross float64
		if h > 0 {
			apparentMS = d.vRow * worldHeightM * clientFPS / h
		}
		if d.vRow > 0 {
			secCross = h / (d.vRow * clientFPS)
		}
		out[i] = DropInfo{
			DiameterMm:   d.dMm,
			Distance:     d.depth,
			TerminalMS:   gunnKinzer(d.dMm),
			ApparentMS:   apparentMS,
			RowsPerTick:  d.vRow,
			SecondsCross: secCross,
			WidthCells:   d.widthCells,
			StreakCells:  d.streakLen,
			Brightness:   lum,
		}
	}
	return out
}

// SpawnDrops spawns n drops immediately (bypassing the spawn-rate roll) so an
// audit can build a large, density-independent statistical sample without
// running the clock. Introspection/test surface only.
func (r *Rain) SpawnDrops(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < n; i++ {
		r.spawnDropAt(r.rng.Float64() * float64(r.W))
	}
}

func (r *Rain) spawnDropAt(col float64) {
	col = math.Max(0, math.Min(float64(r.W)-1, col))
	rs := r.resScale()

	// Synthetic depth. With layering on, each drop draws a distance t in [0,1]
	// (0 = nearest, 1 = farthest), and apparent size, fall speed, streak length
	// and brightness all derive from it together — so a slow drop is *visibly* a
	// distant drop (smaller, dimmer, shorter), not arbitrarily slow. The physics:
	// apparent angular size and angular velocity both fall off with distance, and
	// nearer drops also tend to be the larger, faster-falling ones, so near drops
	// are big/fast/long/bright and far drops the opposite. Bias toward the far
	// field (sqrt) so the scene is mostly mid/background rain with a few bold near
	// streaks rather than an even split.
	depthOn := r.cfg.Layers >= 2

	// Two independent physical axes drive realistic variety:
	//   sizeT  — drop diameter. Marshall–Palmer: lots of small drops, few large,
	//            so the sample is biased small.
	//   depthT — distance from the viewer.
	// Terminal velocity climbs steeply with diameter (Gunn–Kinzer), so big drops
	// genuinely fall faster — that, not just distance, is what gives real rain
	// its mix of fast bold streaks and slow faint drizzle. Apparent (on-screen)
	// velocity = terminal velocity / distance; apparent diameter = size / distance.
	sizeT, depthT := 0.0, 0.0
	dMm, distFall := refRainMm, 1.0
	if depthOn {
		sizeT = math.Pow(r.rng.Float64(), 1.3)   // Marshall–Palmer: biased small
		depthT = r.rng.Float64()                 // distance from viewer
		dMm = lerpDepth(0.5, 4.5, sizeT)         // drop diameter
		distFall = lerpDepth(1.0, 0.65, depthT)  // apparent angular falloff with distance
	}
	vTerm := gunnKinzer(dMm)                              // real terminal velocity, m/s
	speedMul := (vTerm / gunnKinzer(refRainMm)) * distFall // vs reference drop (for streak)
	sizeMul := (dMm / refRainMm) * distFall              // apparent diameter on screen
	brightF := 1.0
	if depthOn {
		brightF = (0.45 + 0.55*distFall) * (0.45 + 0.55*sizeT)
	}

	// Motion jitter. Wind uses currentWind() so lever drift + gust events apply.
	sJit := (r.rng.Float64()*2 - 1) * r.cfg.SpeedJitter
	wJit := (r.rng.Float64()*2 - 1) * r.cfg.WindJitter
	// Physical fall speed: apparent m/s (terminal velocity / distance) projected
	// onto the grid — (m/s) / worldHeight * rows / ticks-per-second. The Speed
	// knob scales intensity around its nominal value. Multiplying by grid height
	// makes this resolution-independent without resScale.
	intensity := r.cfg.Speed / refSpeedKnob
	effSpeed := (vTerm * distFall) * intensity * (1 + sJit) * (float64(r.H) / (worldHeightM * clientFPS))
	effWind := r.currentWind() + wJit*r.cfg.Wind // jitter relative to static base magnitude
	if effSpeed < 0.05 {
		effSpeed = 0.05
	}

	// Color: hue base + jitter, lightness sampled from [min,max], dimmed by depth+size.
	hJit := (r.rng.Float64()*2 - 1) * r.cfg.HueSpread
	hue := math.Mod(r.currentHue()+hJit+360, 360)
	lt := r.rng.Float64()
	lightness := (r.cfg.LightnessMin + lt*(r.cfg.LightnessMax-r.cfg.LightnessMin)) * brightF
	c := hslToRGB(hue, r.cfg.Saturation, lightness)

	// Streak length is motion blur — proportional to apparent velocity, so fast
	// near drops draw long streaks and slow far ones short ones.
	streak := int(math.Round(float64(r.cfg.StreakLen) * rs * speedMul))
	if streak < 1 {
		streak = 1
	}

	// Width stays in cells (a finer grid = thinner drops). DropWidth > 1 fattens
	// the largest, nearest drops toward that many cells.
	width := 1
	if depthOn && r.cfg.DropWidth > 1 {
		wf := clamp01(sizeMul / (4.5 / refRainMm)) // 0..1 against the nearest, biggest drop
		width = int(math.Round(1 + (r.cfg.DropWidth-1)*wf))
		if width < 1 {
			width = 1
		}
	}

	r.drops = append(r.drops, drop{
		Row:        0,
		Col:        col,
		Color:      c,
		vRow:       effSpeed,
		vCol:       effWind * effSpeed,
		streakLen:  streak,
		background: depthT >= 0.55,
		depth:      depthT,
		widthCells: width,
		dMm:        dMm,
	})
}

func (r *Rain) spawnIntroDrop(progress float64) {
	minCol, maxCol := r.introColumnRange(progress)
	col := minCol
	if maxCol > minCol {
		col += r.rng.Float64() * (maxCol - minCol)
	}
	r.spawnDropAt(col)
}

func (r *Rain) spawnEndingDrop(progress float64) {
	minCol, maxCol := r.endingColumnRange(progress)
	col := minCol
	if maxCol > minCol {
		col += r.rng.Float64() * (maxCol - minCol)
	}
	r.spawnDropAt(col)
}

func (r *Rain) startIntroductionLocked() {
	r.downpourTicks = 0
	r.downpourMult = 0
	r.calmTicks = 0
	r.gustTicks = 0
	r.gustWind = 0
	r.splashes = nil
	r.drops = nil
	r.endingTicks = 0
	r.endingTotal = 0
	r.endingFade = 0
	r.endingSplashLeft = 0
	r.endingSplashTotal = 0
	r.introTotal = r.cfg.IntroDur
	if r.introTotal <= 0 {
		r.introTotal = 60
	}
	r.introTicks = r.introTotal
	for i := 0; i < r.cfg.IntroSeed; i++ {
		r.spawnIntroDrop(0)
	}
}

func (r *Rain) stepIntroduction() {
	progress := phaseProgress(r.introTotal, r.introTicks)
	sparse := r.cfg.IntroSparse
	if sparse < 1 {
		sparse = 1
	}
	factor := 1 + (sparse-1)*(1-progress)
	effectiveSpawn := int(math.Round(float64(r.cfg.SpawnEvery) * factor))
	if effectiveSpawn < 1 {
		effectiveSpawn = 1
	}
	if r.rng.Intn(effectiveSpawn) == 0 {
		burst := 1
		if r.cfg.SpawnBurst > 1 {
			burst = 1 + r.rng.Intn(r.cfg.SpawnBurst)
		}
		for i := 0; i < burst; i++ {
			r.spawnIntroDrop(progress)
		}
	}
	r.introTicks--
}

func (r *Rain) startEndingLocked() {
	r.introTicks = 0
	r.introTotal = 0
	r.downpourTicks = 0
	r.downpourMult = 0
	r.calmTicks = 0
	r.gustTicks = 0
	r.gustWind = 0
	r.endingFade = r.cfg.EndingDur
	if r.endingFade <= 0 {
		r.endingFade = 60
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
	r.endingSplashTotal = r.cfg.EndingSplashes
	if r.endingSplashTotal < 0 {
		r.endingSplashTotal = 0
	}
	r.endingSplashLeft = r.endingSplashTotal
}

func (r *Rain) stepEnding() {
	totalProgress := phaseProgress(r.endingTotal, r.endingTicks)
	if r.endingSplashLeft > 0 && r.endingSplashTotal > 0 {
		targetDone := int(math.Floor(math.Pow(totalProgress, 1.8) * float64(r.endingSplashTotal)))
		done := r.endingSplashTotal - r.endingSplashLeft
		for done < targetDone && r.endingSplashLeft > 0 {
			r.spawnSplash()
			r.endingSplashLeft--
			done++
		}
	}

	elapsed := r.endingTotal - r.endingTicks
	if elapsed < r.endingFade {
		fadeProgress := clamp01(float64(elapsed) / float64(max(1, r.endingFade-1)))
		factor := 1 + 18*fadeProgress*fadeProgress
		effectiveSpawn := int(math.Round(float64(r.cfg.SpawnEvery) * factor))
		if effectiveSpawn < 1 {
			effectiveSpawn = 1
		}
		if r.rng.Intn(effectiveSpawn) == 0 {
			r.spawnEndingDrop(fadeProgress)
		}
	}

	r.endingTicks--
	if r.endingTicks < 0 {
		r.endingTicks = 0
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (r *Rain) spawnSplash() {
	if r.cfg.SplashSize <= 0 {
		return
	}
	baseRadius := int(math.Round(float64(r.cfg.SplashSize) * r.resScale()))
	if baseRadius < 1 {
		baseRadius = 1
	}
	radius := jitterInt(r.rng, baseRadius, 0.3)
	hue := math.Mod(r.currentHue()+(r.rng.Float64()*2-1)*r.cfg.HueSpread+360, 360)
	c := hslToRGB(hue, r.cfg.Saturation, r.cfg.LightnessMax)
	r.splashes = append(r.splashes, splashInstance{
		row:       r.rng.Intn(r.H),
		col:       r.rng.Intn(r.W),
		maxAge:    radius * 2,
		maxRadius: radius,
		color:     c,
	})
}

func (r *Rain) paintSplashes() {
	for _, s := range r.splashes {
		t := float64(s.age) / float64(s.maxAge) // 0..1
		radius := t * float64(s.maxRadius)
		alpha := 1 - t
		c := s.color
		c.R = uint8(float64(c.R) * alpha)
		c.G = uint8(float64(c.G) * alpha)
		c.B = uint8(float64(c.B) * alpha)
		// Plot a ring at the current radius.
		steps := int(2 * math.Pi * radius)
		if steps < 8 {
			steps = 8
		}
		for i := 0; i < steps; i++ {
			theta := 2 * math.Pi * float64(i) / float64(steps)
			gc := s.col + int(math.Round(radius*math.Cos(theta)))
			gr := s.row + int(math.Round(radius*math.Sin(theta)))
			if gr < 0 || gr >= r.H || gc < 0 || gc >= r.W {
				continue
			}
			r.paintPixelMax(gr, gc, c)
		}
	}
}

func (r *Rain) paintSheet() {
	if r.cfg.SheetDensity <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	rs := r.resScale()
	length := int(math.Round(float64(r.cfg.SheetLength) * rs))
	if minL := int(math.Round(2 * rs)); length < minL {
		length = minL
	}
	if maxL := int(math.Round(40 * rs)); length > maxL {
		length = maxL
	}
	if length < 2 {
		length = 2
	}
	strength := clamp01(r.cfg.SheetStrength)
	if strength <= 0 {
		return
	}
	speed := r.cfg.SheetSpeed
	if speed <= 0 {
		speed = 1
	}
	speed *= rs
	intensity := r.layerIntensity()
	if intensity <= 0 {
		return
	}
	streams := int(math.Round(clamp01(r.cfg.SheetDensity) * float64(r.W) * 0.85 * intensity))
	if streams < 1 {
		return
	}
	span := float64(r.H + length*2)
	wind := r.currentWind()
	rowStep, colStep := normalizedMotion(1, wind)
	for i := 0; i < streams; i++ {
		h0 := hash64(uint64(i)*0x9e3779b97f4a7c15 + 0x6a09e667f3bcc909)
		h1 := hash64(uint64(i)*0xc2b2ae3d27d4eb4f + 0xbb67ae8584caa73b)
		h2 := hash64(uint64(i)*0x165667b19e3779f9 + 0x3c6ef372fe94f82b)
		phase := hashUnit(h0) * span
		streamSpeed := speed * (0.75 + hashUnit(h1)*0.5)
		headRow := math.Mod(phase+float64(r.tick)*streamSpeed, span) - float64(length)
		baseCol := hashUnit(h2) * float64(r.W)
		hue := math.Mod(r.currentHue()+(hashUnit(h1)*2-1)*r.cfg.HueSpread*0.5+360, 360)
		light := r.cfg.LightnessMin + hashUnit(h0)*(r.cfg.LightnessMax-r.cfg.LightnessMin)
		base := hslToRGB(hue, r.cfg.Saturation*0.55, light)
		for j := 0; j < length; j++ {
			row := headRow - float64(j)*rowStep
			col := baseCol + row*wind - float64(j)*colStep
			gr := int(math.Floor(row))
			if gr < 0 || gr >= r.H {
				continue
			}
			gc := wrapInt(int(math.Round(col)), r.W)
			tail := 1 - float64(j)/float64(length)
			brightness := strength * (0.35 + 0.65*tail) * (0.75 + hashUnit(h2)*0.25)
			c := base
			c.R = uint8(float64(c.R) * brightness)
			c.G = uint8(float64(c.G) * brightness)
			c.B = uint8(float64(c.B) * brightness)
			r.paintPixelMax(gr, gc, c)
		}
	}
}

func (r *Rain) paintFrontPlane() {
	if r.cfg.FrontDensity <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	rs := r.resScale()
	length := int(math.Round(float64(r.cfg.FrontLength) * rs))
	if minL := int(math.Round(4 * rs)); length < minL {
		length = minL
	}
	if maxL := int(math.Round(64 * rs)); length > maxL {
		length = maxL
	}
	if length < 4 {
		length = 4
	}
	strength := clamp01(r.cfg.FrontStrength)
	if strength <= 0 {
		return
	}
	speed := r.cfg.FrontSpeed
	if speed <= 0 {
		speed = 54
	}
	speed *= rs
	wind := r.currentWind()
	life := int(math.Ceil((float64(r.H)+float64(length)*2)/speed)) + 1
	if life < 3 {
		life = 3
	}
	if life > 18 {
		life = 18
	}
	eventsPerTick := clamp01(r.cfg.FrontDensity) * float64(r.W) * 0.038 * r.layerIntensity()
	for age := 0; age <= life; age++ {
		birthTick := r.tick - age
		if birthTick < 0 {
			continue
		}
		birthHash := hash64(uint64(birthTick)*0x9e3779b97f4a7c15 + 0x8f1bbcdcaf1476d9)
		eventCount := int(math.Floor(eventsPerTick))
		if hashUnit(hash64(birthHash+0x632be59bd9b4e019)) < eventsPerTick-float64(eventCount) {
			eventCount++
		}
		if eventCount == 0 && eventsPerTick > 0 && hashUnit(birthHash) < eventsPerTick {
			eventCount = 1
		}
		for i := 0; i < eventCount; i++ {
			h0 := hash64(birthHash + uint64(i)*0x94d049bb133111eb + 0xa54ff53a5f1d36f1)
			h1 := hash64(birthHash + uint64(i)*0xbf58476d1ce4e5b9 + 0x510e527fade682d1)
			h2 := hash64(birthHash + uint64(i)*0xd6e8feb86659fd93 + 0x1f83d9abfb41bd6b)
			h3 := hash64(birthHash + uint64(i)*0x9e3779b97f4a7c15 + 0x243f6a8885a308d3)
			eventSpeed := speed * (0.68 + hashUnit(h0)*0.64)
			subFrame := hashUnit(h1) * eventSpeed
			headRow := -float64(length) + subFrame + float64(age)*eventSpeed
			eventWind := wind + (hashUnit(h2)*2-1)*0.035
			rowStep, colStep := normalizedMotion(1, eventWind)
			baseCol := hashUnit(h3) * float64(r.W)
			hue := math.Mod(r.currentHue()+(hashUnit(h1)*2-1)*r.cfg.HueSpread*0.35+360, 360)
			light := r.cfg.LightnessMax + (1-r.cfg.LightnessMax)*0.25
			base := hslToRGB(hue, r.cfg.Saturation*0.45, light)
			exposureLen := length + int(math.Round(math.Min(eventSpeed*0.65, float64(length)*1.5)))
			if maxExp := int(math.Round(72 * rs)); exposureLen > maxExp {
				exposureLen = maxExp
			}
			for j := 0; j < exposureLen; j++ {
				row := headRow - float64(j)*rowStep
				col := baseCol + row*eventWind - float64(j)*colStep
				gr := int(math.Floor(row))
				if gr < 0 || gr >= r.H {
					continue
				}
				gc := int(math.Round(col))
				if gc < 0 || gc >= r.W {
					continue
				}
				tail := math.Pow(1-float64(j)/float64(exposureLen), 1.7)
				brightness := strength * (0.08 + 0.62*tail) * (0.85 + hashUnit(h0)*0.15)
				c := base
				c.R = uint8(float64(c.R) * brightness)
				c.G = uint8(float64(c.G) * brightness)
				c.B = uint8(float64(c.B) * brightness)
				r.paintPixelMax(gr, gc, c)
			}
		}
	}
}

// paintDrop lays down StreakLen cells from the drop's head backward along its
// motion vector. Brightness decays by FadeFactor per position from the head.
func (r *Rain) paintDrop(d drop) {
	r.paintDropInto(r.Grid, d)
}

// paintDropInto paints a drop's brush-width streak into an arbitrary grid with
// the same max-blend as the main field. OverlayGridCopy reuses it to build the
// near-plane frame from the nearest drops alone.
func (r *Rain) paintDropInto(grid [][]Pixel, d drop) {
	rowStep, colStep := d.trailStep()
	// Width is stamped perpendicular to motion (rotate the unit motion vector
	// 90°). width 1 reduces to the classic single-cell streak.
	w := d.widthCells
	if w < 1 {
		w = 1
	}
	perpRow, perpCol := -colStep, rowStep
	half := float64(w-1) / 2
	for i := 0; i < d.streakLen; i++ {
		row := d.Row - float64(i)*rowStep
		col := d.Col - float64(i)*colStep
		brightness := math.Pow(r.cfg.FadeFactor, float64(i))
		c := d.Color
		c.R = uint8(float64(c.R) * brightness)
		c.G = uint8(float64(c.G) * brightness)
		c.B = uint8(float64(c.B) * brightness)
		for k := 0; k < w; k++ {
			off := float64(k) - half
			gr := int(math.Floor(row + off*perpRow))
			gc := int(math.Round(col + off*perpCol))
			if gr < 0 || gr >= r.H || gc < 0 || gc >= r.W {
				continue
			}
			r.paintPixelMaxInto(grid, gr, gc, c)
		}
	}
}

func (d drop) trailStep() (float64, float64) {
	return normalizedMotion(d.vRow, d.vCol)
}

func normalizedMotion(vRow, vCol float64) (float64, float64) {
	length := math.Hypot(vRow, vCol)
	if length < 0.0001 {
		return 1, 0
	}
	return vRow / length, vCol / length
}

func hash64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func hashUnit(x uint64) float64 {
	return float64(x>>11) * (1.0 / 9007199254740992.0)
}

func wrapInt(v, limit int) int {
	if limit <= 0 {
		return 0
	}
	v %= limit
	if v < 0 {
		v += limit
	}
	return v
}

func (r *Rain) paintPixelMax(row, col int, c color.RGBA) {
	r.paintPixelMaxInto(r.Grid, row, col, c)
}

func (r *Rain) paintPixelMaxInto(grid [][]Pixel, row, col int, c color.RGBA) {
	if row < 0 || row >= r.H || col < 0 || col >= r.W {
		return
	}
	p := &grid[row][col]
	if !p.Filled {
		*p = Pixel{Filled: true, C: c}
		return
	}
	if c.R > p.C.R {
		p.C.R = c.R
	}
	if c.G > p.C.G {
		p.C.G = c.G
	}
	if c.B > p.C.B {
		p.C.B = c.B
	}
	p.C.A = 255
}

// spawnDrop rolls per-drop jitter (speed, wind, hue, lightness) + layer,
// computes the motion vector + color, and appends the drop to the list.
func (r *Rain) spawnDrop() {
	r.spawnDropAt(r.rng.Float64() * float64(r.W))
}
