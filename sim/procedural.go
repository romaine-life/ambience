package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// ProceduralConfig is a lightweight numeric config map used by browser-first
// scenic prototypes. The server owns event timing and snapshot/restore state;
// the browser owns the richer deterministic render derived from tick + seed.
type ProceduralConfig map[string]float64

type ProceduralState struct {
	Tick   int                `json:"tick"`
	Timers map[string]int     `json:"timers,omitempty"`
	Values map[string]float64 `json:"values,omitempty"`
}

type ProceduralSnapshot struct {
	ProceduralState
}

type ProceduralPersistedState struct {
	ProceduralState
	RNGState uint64 `json:"rngState"`
}

// Procedural hosts lightweight browser-first scenic prototypes. It tracks
// authoritative tick/event state so join-in-progress dev sessions can restore
// a consistent mood without the server needing a full particle simulation.
type Procedural struct {
	mu sync.Mutex

	Kind string
	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    ProceduralConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var snowDefaults = ProceduralConfig{
	"intro_dur":      60,
	"intro_density":  0.16,
	"ending_dur":     70,
	"ending_linger":  22,
	"ending_density": 0.08,
	"density":        0.32,
	"speed":          0.48,
	"drift":          0.08,
	"sway":           0.42,
	"layers":         3,
	"size":           1.0,
	"hue":            210,
	"hue_sp":         12,
	"sat":            0.16,
	"lmin":           0.74,
	"lmax":           0.98,
	"gust_p":         0.0,
	"calm_p":         0.0,
	"gust_dur":       55,
	"gust_mult":      1.85,
	"calm_dur":       80,
	"calm_mult":      0.42,
}

var autumnLeavesDefaults = ProceduralConfig{
	"intro_dur":      55,
	"intro_density":  0.12,
	"ending_dur":     60,
	"ending_linger":  18,
	"ending_density": 0.04,
	"density":        0.24,
	"speed":          0.44,
	"drift":          0.18,
	"sway":           0.86,
	"layers":         2,
	"size":           1.2,
	"hue":            28,
	"hue_sp":         24,
	"sat":            0.62,
	"lmin":           0.38,
	"lmax":           0.78,
	"gust_p":         0.0,
	"lull_p":         0.0,
	"swirl_p":        0.0,
	"gust_dur":       48,
	"gust_mult":      1.9,
	"lull_dur":       72,
	"lull_mult":      0.35,
	"swirl_dur":      52,
	"swirl_pull":     1.15,
}

var starfieldDefaults = ProceduralConfig{
	"intro_dur":          50,
	"intro_density":      0.08,
	"ending_dur":         60,
	"ending_linger":      16,
	"ending_density":     0.03,
	"density":            0.22,
	"speed":              0.12,
	"drift":              0.04,
	"layers":             3,
	"size":               1.0,
	"hue":                218,
	"hue_sp":             18,
	"sat":                0.18,
	"lmin":               0.55,
	"lmax":               0.95,
	"shooting_star_p":    0.0,
	"twinkle_burst_p":    0.0,
	"shooting_star_dur":  26,
	"shooting_star_mult": 1.8,
	"twinkle_burst_dur":  42,
	"twinkle_burst_mult": 1.7,
}

var auroraDefaults = ProceduralConfig{
	"intro_dur":     70,
	"intro_glow":    0.18,
	"ending_dur":    80,
	"ending_linger": 20,
	"ending_glow":   0.05,
	"intensity":     0.56,
	"speed":         0.11,
	"drift":         0.08,
	"bands":         3,
	"thickness":     9,
	"wave_amp":      6,
	"wave_freq":     0.16,
	"curtain_len":   15,
	"hue":           138,
	"hue_sp":        26,
	"sat":           0.72,
	"lmin":          0.20,
	"lmax":          0.74,
	"brighten_p":    0.0,
	"shift_p":       0.0,
	"fade_p":        0.0,
	"brighten_dur":  42,
	"brighten_mult": 1.45,
	"shift_dur":     64,
	"shift_amt":     1.10,
	"fade_dur":      58,
	"fade_mult":     0.60,
}

var wheatFieldDefaults = ProceduralConfig{
	"intro_dur":     60,
	"intro_breeze":  0.16,
	"ending_dur":    70,
	"ending_linger": 20,
	"ending_sway":   0.08,
	"density":       0.48,
	"speed":         0.12,
	"drift":         0.16,
	"sway":          0.68,
	"wave_freq":     0.18,
	"field_top":     0.62,
	"stalk_h":       18,
	"layers":        3,
	"hue":           46,
	"hue_sp":        18,
	"sat":           0.64,
	"lmin":          0.30,
	"lmax":          0.76,
	"gust_p":        0.0,
	"calm_p":        0.0,
	"gust_dur":      50,
	"gust_mult":     1.85,
	"calm_dur":      72,
	"calm_mult":     0.40,
}

func SnowSchema() EffectSchema {
	return EffectSchema{
		Name: "snow",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent ramping from a few first flakes into the full snowfall."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.02, Default: 0.16,
				Description: "Starting snowfall fraction before the full field settles in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering the snowfall down toward still air."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks after the taper so the last flakes can drift out."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.02, Default: 0.08,
				Description: "How much low-level snowfall remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.32,
				Description: "Base snowfall density across all layers."},
			{Key: "speed", Label: "fall speed", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.5, Step: 0.02, Default: 0.48,
				Description: "How quickly flakes descend through the field."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Baseline sideways carry. Positive drifts right, negative drifts left."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0, Max: 1.2, Step: 0.02, Default: 0.42,
				Description: "Side-to-side meander in each flake's path."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "fall", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of snowfall depth layers."},
			{Key: "size", Label: "flake size", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.5, Max: 2.5, Step: 0.1, Default: 1,
				Description: "Pixel size of the brightest foreground flakes."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 240, Step: 1, Default: 210,
				Description: "Base snow tint. Lower values warm toward dusk; higher values cool toward ice."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 12,
				Description: "Variation in the flake tint and sky reflection."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.01, Max: 0.45, Step: 0.01, Default: 0.16,
				Description: "Overall color saturation of the snow scene."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.9, Step: 0.01, Default: 0.74,
				Description: "Minimum lightness used for dim flakes and distant haze."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1.0, Step: 0.01, Default: 0.98,
				Description: "Maximum lightness used for the brightest near flakes."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a crosswind kicking the snowfall sideways."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the snowfall briefly thinning into stillness."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55,
				Description: "Typical gust duration in ticks (jittered by +/-30%)."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.85,
				Description: "How strongly a gust bends the snowfall sideways."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 80,
				Description: "Duration of the quieter low-density window."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.42,
				Description: "Density multiplier applied while calm is active."},
		},
	}
}

func AutumnLeavesSchema() EffectSchema {
	return EffectSchema{
		Name: "autumn-leaves",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent building from a few drifting leaves into the full fall."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.02, Default: 0.12,
				Description: "Starting fraction of the full leaf field before the fall settles in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent tapering the leaf detachments back toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 18,
				Description: "Extra quiet ticks for the last airborne leaves to settle out."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.02, Default: 0.04,
				Description: "How much low-level drift remains at the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.05, Max: 0.75, Step: 0.01, Default: 0.24,
				Description: "Base number of drifting leaves across the field."},
			{Key: "speed", Label: "fall speed", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.44,
				Description: "How quickly leaves drop through the scene."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: -0.8, Max: 0.8, Step: 0.01, Default: 0.18,
				Description: "Baseline sideways carry applied to the leaf field."},
			{Key: "sway", Label: "flutter", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.1, Max: 1.8, Step: 0.02, Default: 0.86,
				Description: "How much the leaves wobble and flutter on the way down."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "fall", Type: KnobInt, Min: 1, Max: 3, Step: 1, Default: 2,
				Description: "Number of leaf depth layers."},
			{Key: "size", Label: "leaf size", Slot: SlotLever, Group: "fall", Type: KnobFloat, Min: 0.5, Max: 3, Step: 0.1, Default: 1.2,
				Description: "Pixel size of the nearer leaf blocks."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 70, Step: 1, Default: 28,
				Description: "Base leaf hue. Lower values warm toward red; higher values lean gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 50, Step: 1, Default: 24,
				Description: "Variation in leaf color across the field."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.62,
				Description: "Overall leaf saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.8, Step: 0.01, Default: 0.38,
				Description: "Minimum lightness used for distant leaves and background tones."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for the brightest near leaves."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a stronger wind push across the leaf field."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the leaf fall thinning into a quieter stretch."},
			{Key: "swirl_p", Label: "swirl", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "swirl",
				Description: "Per-tick chance of a circular eddy tugging leaves into a swirl."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 48,
				Description: "Typical gust duration in ticks (jittered by +/-30%)."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.9,
				Description: "How strongly a gust bends the leaf field sideways."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the lower-density lull window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Density multiplier applied while lull is active."},
			{Key: "swirl_dur", Label: "swirl dur", Slot: SlotEventMod, Group: "swirl", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 52,
				Description: "How long the swirl eddy stays active."},
			{Key: "swirl_pull", Label: "swirl pull", Slot: SlotEventMod, Group: "swirl", Type: KnobFloat, Min: 0.1, Max: 2.5, Step: 0.05, Default: 1.15,
				Description: "Strength of the circular pull during a swirl event."},
		},
	}
}

func StarfieldSchema() EffectSchema {
	return EffectSchema{
		Name: "starfield",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent populating the star layers from sparse points into the full field."},
			{Key: "intro_density", Label: "intro density", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.01, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Starting fraction of the full star density before the field finishes blooming in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent dimming the starfield back toward near-darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 120, Step: 5, Default: 16,
				Description: "Extra quiet ticks after the fade so the last points can linger."},
			{Key: "ending_density", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.3, Step: 0.01, Default: 0.03,
				Description: "How much of the far starfield remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Base star density across the full field."},
			{Key: "speed", Label: "parallax", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "How quickly the parallax layers drift across the scene."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: -0.25, Max: 0.25, Step: 0.01, Default: 0.04,
				Description: "Baseline horizontal drift direction for the starfield."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of parallax layers."},
			{Key: "size", Label: "star size", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.5, Max: 2.5, Step: 0.1, Default: 1,
				Description: "Pixel size of the nearest stars."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 190, Max: 250, Step: 1, Default: 218,
				Description: "Base star tint. Lower values lean blue-cyan; higher values lean violet."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 18,
				Description: "Variation in the star tint across the field."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.01, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Overall star color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 0.85, Step: 0.01, Default: 0.55,
				Description: "Minimum lightness used for the dimmest background stars."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1.0, Step: 0.01, Default: 0.95,
				Description: "Maximum lightness used for the nearest stars and bright accents."},
			{Key: "shooting_star_p", Label: "shooting star", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "shooting-star",
				Description: "Per-tick chance of a rare shooting star crossing the field."},
			{Key: "twinkle_burst_p", Label: "twinkle burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "twinkle-burst",
				Description: "Per-tick chance of a brief field-wide brightening."},
			{Key: "shooting_star_dur", Label: "shoot dur", Slot: SlotEventMod, Group: "shooting-star", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 26,
				Description: "How long a shooting star remains visible."},
			{Key: "shooting_star_mult", Label: "shoot x", Slot: SlotEventMod, Group: "shooting-star", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.8,
				Description: "Brightness multiplier for the shooting star accent."},
			{Key: "twinkle_burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "twinkle-burst", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 42,
				Description: "Duration of the twinkle burst brightening window."},
			{Key: "twinkle_burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "twinkle-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.7,
				Description: "Brightness multiplier applied during a twinkle burst."},
		},
	}
}

func AuroraSchema() EffectSchema {
	return EffectSchema{
		Name: "aurora",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 260, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent blooming from a faint horizon glow into the full aurora."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.01, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Starting brightness fraction before the ribbons fully form."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 260, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent dimming and narrowing back toward a dark sky."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks for the last faint glow to hang over the horizon."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 0.4, Step: 0.01, Default: 0.05,
				Description: "Residual brightness fraction that remains near the end of the outro."},
			{Key: "intensity", Label: "intensity", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.05, Max: 1.2, Step: 0.01, Default: 0.56,
				Description: "Overall luminance of the aurora ribbons."},
			{Key: "speed", Label: "motion", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.02, Max: 0.45, Step: 0.01, Default: 0.11,
				Description: "How quickly the ribbons undulate across the sky."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Baseline sideways drift for the whole veil field."},
			{Key: "bands", Label: "bands", Slot: SlotLever, Group: "sky", Type: KnobInt, Min: 1, Max: 5, Step: 1, Default: 3,
				Description: "Number of main aurora ribbons."},
			{Key: "thickness", Label: "thickness", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 2, Max: 18, Step: 0.5, Default: 9,
				Description: "Vertical thickness of each bright ribbon core."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 1, Max: 18, Step: 0.5, Default: 6,
				Description: "How far each ribbon arches and sways."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.04, Max: 0.4, Step: 0.01, Default: 0.16,
				Description: "Horizontal frequency of the ribbon arches."},
			{Key: "curtain_len", Label: "curtain", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 2, Max: 28, Step: 0.5, Default: 15,
				Description: "How far the glow trails downward from each ribbon."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 100, Max: 220, Step: 1, Default: 138,
				Description: "Base aurora hue. Lower values lean green; higher values tip toward cyan-violet."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 26,
				Description: "Variation between the different bands and edge glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Overall color saturation of the sky glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.20,
				Description: "Minimum lightness used for the faint outer glow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 0.95, Step: 0.01, Default: 0.74,
				Description: "Maximum lightness used for the brightest ribbon centers."},
			{Key: "brighten_p", Label: "brighten", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "brighten",
				Description: "Per-tick chance of the sky blooming into a brighter curtain."},
			{Key: "shift_p", Label: "shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "shift",
				Description: "Per-tick chance of the aurora sliding into a new wave alignment."},
			{Key: "fade_p", Label: "fade", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "fade",
				Description: "Per-tick chance of the ribbons briefly thinning into a dimmer phase."},
			{Key: "brighten_dur", Label: "bright dur", Slot: SlotEventMod, Group: "brighten", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 42,
				Description: "How long a brighten bloom lasts."},
			{Key: "brighten_mult", Label: "bright x", Slot: SlotEventMod, Group: "brighten", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.45,
				Description: "Brightness multiplier applied during a bloom."},
			{Key: "shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "shift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 64,
				Description: "How long the shifted alignment remains active."},
			{Key: "shift_amt", Label: "shift amt", Slot: SlotEventMod, Group: "shift", Type: KnobFloat, Min: 0.1, Max: 3, Step: 0.05, Default: 1.1,
				Description: "How strongly a shift event pulls the ribbons into a new phase."},
			{Key: "fade_dur", Label: "fade dur", Slot: SlotEventMod, Group: "fade", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "How long the dimmer phase lasts."},
			{Key: "fade_mult", Label: "fade x", Slot: SlotEventMod, Group: "fade", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.6,
				Description: "Brightness multiplier applied during a fade event."},
		},
	}
}

func WheatFieldSchema() EffectSchema {
	return EffectSchema{
		Name: "wheat-field",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent spreading motion through the field from near-stillness into full waves."},
			{Key: "intro_breeze", Label: "intro breeze", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.08, Max: 0.5, Step: 0.02, Default: 0.16,
				Description: "Starting fraction of the full sway before the waves finish arriving."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent damping the field back toward calm."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 140, Step: 5, Default: 20,
				Description: "Extra quiet ticks for the last residual motion to settle out."},
			{Key: "ending_sway", Label: "ending sway", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.04, Max: 0.28, Step: 0.02, Default: 0.08,
				Description: "Residual sway fraction that remains near the end of the outro."},
			{Key: "density", Label: "density", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.24, Max: 0.92, Step: 0.02, Default: 0.48,
				Description: "How densely the field is packed with visible stalk highlights."},
			{Key: "speed", Label: "wave speed", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "How quickly the broad waves travel through the field."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: -0.5, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Preferred direction of the wave travel. Positive values push right."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.25, Max: 1.35, Step: 0.02, Default: 0.68,
				Description: "How far the stalk tips lean and recover."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.06, Max: 0.35, Step: 0.01, Default: 0.18,
				Description: "Horizontal frequency of the passing field waves."},
			{Key: "field_top", Label: "field top", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 0.54, Max: 0.74, Step: 0.01, Default: 0.62,
				Description: "Where the top of the wheat band sits in the frame."},
			{Key: "stalk_h", Label: "stalk height", Slot: SlotLever, Group: "field", Type: KnobFloat, Min: 8, Max: 28, Step: 1, Default: 18,
				Description: "Apparent height of the stalks rising above the field base."},
			{Key: "layers", Label: "layers", Slot: SlotLever, Group: "field", Type: KnobInt, Min: 1, Max: 4, Step: 1, Default: 3,
				Description: "Number of depth layers used to build the field."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 38, Max: 66, Step: 1, Default: 46,
				Description: "Base wheat hue. Lower values warm toward amber; higher values lean green-gold."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 32, Step: 1, Default: 18,
				Description: "Variation across stalk highlights and shadow bands."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.32, Max: 0.95, Step: 0.01, Default: 0.64,
				Description: "Overall color saturation of the field."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.18, Max: 0.55, Step: 0.01, Default: 0.30,
				Description: "Minimum lightness used for deeper shadowed wheat."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.55, Max: 0.92, Step: 0.01, Default: 0.76,
				Description: "Maximum lightness used for sunstruck stalk tips."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of a stronger wind wave crossing the field."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the field settling into a quieter, lighter sway."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50,
				Description: "Typical duration of a stronger wind pulse."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 1.85,
				Description: "Sway multiplier applied during a gust."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the quieter low-amplitude window."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.4,
				Description: "Sway multiplier applied while calm is active."},
		},
	}
}

func cloneProceduralConfig(src ProceduralConfig) ProceduralConfig {
	if src == nil {
		return ProceduralConfig{}
	}
	out := make(ProceduralConfig, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneTimerMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(src))
	for k, v := range src {
		if v > 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneValueMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func proceduralDefaults(kind string) ProceduralConfig {
	switch kind {
	case "snow":
		return cloneProceduralConfig(snowDefaults)
	case "autumn-leaves":
		return cloneProceduralConfig(autumnLeavesDefaults)
	case "starfield":
		return cloneProceduralConfig(starfieldDefaults)
	case "aurora":
		return cloneProceduralConfig(auroraDefaults)
	case "wheat-field":
		return cloneProceduralConfig(wheatFieldDefaults)
	default:
		return ProceduralConfig{}
	}
}

func mergeProceduralDefaults(kind string, cfg ProceduralConfig) ProceduralConfig {
	out := proceduralDefaults(kind)
	for k, v := range cfg {
		out[k] = v
	}
	switch kind {
	case "snow":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = snowDefaults["intro_dur"]
		}
		out["intro_density"] = clamp01(out["intro_density"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = snowDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_density"] = clamp01(out["ending_density"])
		if out["density"] <= 0 {
			out["density"] = snowDefaults["density"]
		}
		if out["speed"] <= 0 {
			out["speed"] = snowDefaults["speed"]
		}
		if out["layers"] < 1 {
			out["layers"] = snowDefaults["layers"]
		}
		if out["size"] <= 0 {
			out["size"] = snowDefaults["size"]
		}
		if out["hue"] == 0 {
			out["hue"] = snowDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = snowDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = snowDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = snowDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["gust_dur"] <= 0 {
			out["gust_dur"] = snowDefaults["gust_dur"]
		}
		if out["gust_mult"] <= 0 {
			out["gust_mult"] = snowDefaults["gust_mult"]
		}
		if out["calm_dur"] <= 0 {
			out["calm_dur"] = snowDefaults["calm_dur"]
		}
		if out["calm_mult"] <= 0 {
			out["calm_mult"] = snowDefaults["calm_mult"]
		}
	case "autumn-leaves":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = autumnLeavesDefaults["intro_dur"]
		}
		out["intro_density"] = clamp01(out["intro_density"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = autumnLeavesDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_density"] = clamp01(out["ending_density"])
		if out["density"] <= 0 {
			out["density"] = autumnLeavesDefaults["density"]
		}
		if out["speed"] <= 0 {
			out["speed"] = autumnLeavesDefaults["speed"]
		}
		if out["layers"] < 1 {
			out["layers"] = autumnLeavesDefaults["layers"]
		}
		if out["size"] <= 0 {
			out["size"] = autumnLeavesDefaults["size"]
		}
		if out["hue"] == 0 {
			out["hue"] = autumnLeavesDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = autumnLeavesDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = autumnLeavesDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = autumnLeavesDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["gust_dur"] <= 0 {
			out["gust_dur"] = autumnLeavesDefaults["gust_dur"]
		}
		if out["gust_mult"] <= 0 {
			out["gust_mult"] = autumnLeavesDefaults["gust_mult"]
		}
		if out["lull_dur"] <= 0 {
			out["lull_dur"] = autumnLeavesDefaults["lull_dur"]
		}
		if out["lull_mult"] <= 0 {
			out["lull_mult"] = autumnLeavesDefaults["lull_mult"]
		}
		if out["swirl_dur"] <= 0 {
			out["swirl_dur"] = autumnLeavesDefaults["swirl_dur"]
		}
		if out["swirl_pull"] <= 0 {
			out["swirl_pull"] = autumnLeavesDefaults["swirl_pull"]
		}
	case "starfield":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = starfieldDefaults["intro_dur"]
		}
		out["intro_density"] = clamp01(out["intro_density"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = starfieldDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_density"] = clamp01(out["ending_density"])
		if out["density"] <= 0 {
			out["density"] = starfieldDefaults["density"]
		}
		if out["speed"] <= 0 {
			out["speed"] = starfieldDefaults["speed"]
		}
		if out["layers"] < 1 {
			out["layers"] = starfieldDefaults["layers"]
		}
		if out["size"] <= 0 {
			out["size"] = starfieldDefaults["size"]
		}
		if out["hue"] == 0 {
			out["hue"] = starfieldDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = starfieldDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = starfieldDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = starfieldDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["shooting_star_dur"] <= 0 {
			out["shooting_star_dur"] = starfieldDefaults["shooting_star_dur"]
		}
		if out["shooting_star_mult"] <= 0 {
			out["shooting_star_mult"] = starfieldDefaults["shooting_star_mult"]
		}
		if out["twinkle_burst_dur"] <= 0 {
			out["twinkle_burst_dur"] = starfieldDefaults["twinkle_burst_dur"]
		}
		if out["twinkle_burst_mult"] <= 0 {
			out["twinkle_burst_mult"] = starfieldDefaults["twinkle_burst_mult"]
		}
	case "aurora":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = auroraDefaults["intro_dur"]
		}
		out["intro_glow"] = clamp01(out["intro_glow"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = auroraDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_glow"] = clamp01(out["ending_glow"])
		if out["intensity"] <= 0 {
			out["intensity"] = auroraDefaults["intensity"]
		}
		if out["speed"] <= 0 {
			out["speed"] = auroraDefaults["speed"]
		}
		if out["bands"] < 1 {
			out["bands"] = auroraDefaults["bands"]
		}
		if out["thickness"] <= 0 {
			out["thickness"] = auroraDefaults["thickness"]
		}
		if out["wave_amp"] <= 0 {
			out["wave_amp"] = auroraDefaults["wave_amp"]
		}
		if out["wave_freq"] <= 0 {
			out["wave_freq"] = auroraDefaults["wave_freq"]
		}
		if out["curtain_len"] <= 0 {
			out["curtain_len"] = auroraDefaults["curtain_len"]
		}
		if out["hue"] == 0 {
			out["hue"] = auroraDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = auroraDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = auroraDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = auroraDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["brighten_dur"] <= 0 {
			out["brighten_dur"] = auroraDefaults["brighten_dur"]
		}
		if out["brighten_mult"] <= 0 {
			out["brighten_mult"] = auroraDefaults["brighten_mult"]
		}
		if out["shift_dur"] <= 0 {
			out["shift_dur"] = auroraDefaults["shift_dur"]
		}
		if out["shift_amt"] <= 0 {
			out["shift_amt"] = auroraDefaults["shift_amt"]
		}
		if out["fade_dur"] <= 0 {
			out["fade_dur"] = auroraDefaults["fade_dur"]
		}
		if out["fade_mult"] <= 0 {
			out["fade_mult"] = auroraDefaults["fade_mult"]
		}
	case "wheat-field":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = wheatFieldDefaults["intro_dur"]
		}
		out["intro_breeze"] = clamp01(out["intro_breeze"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = wheatFieldDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_sway"] = clamp01(out["ending_sway"])
		if out["density"] <= 0 {
			out["density"] = wheatFieldDefaults["density"]
		}
		if out["speed"] <= 0 {
			out["speed"] = wheatFieldDefaults["speed"]
		}
		if out["sway"] <= 0 {
			out["sway"] = wheatFieldDefaults["sway"]
		}
		if out["wave_freq"] <= 0 {
			out["wave_freq"] = wheatFieldDefaults["wave_freq"]
		}
		if out["field_top"] <= 0 {
			out["field_top"] = wheatFieldDefaults["field_top"]
		}
		if out["stalk_h"] <= 0 {
			out["stalk_h"] = wheatFieldDefaults["stalk_h"]
		}
		if out["layers"] < 1 {
			out["layers"] = wheatFieldDefaults["layers"]
		}
		if out["hue"] == 0 {
			out["hue"] = wheatFieldDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = wheatFieldDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = wheatFieldDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = wheatFieldDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["gust_dur"] <= 0 {
			out["gust_dur"] = wheatFieldDefaults["gust_dur"]
		}
		if out["gust_mult"] <= 0 {
			out["gust_mult"] = wheatFieldDefaults["gust_mult"]
		}
		if out["calm_dur"] <= 0 {
			out["calm_dur"] = wheatFieldDefaults["calm_dur"]
		}
		if out["calm_mult"] <= 0 {
			out["calm_mult"] = wheatFieldDefaults["calm_mult"]
		}
	}
	return out
}

func NewProcedural(kind string, w, h int, seed int64, cfg ProceduralConfig) *Procedural {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &Procedural{
		Kind:   kind,
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeProceduralDefaults(kind, cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (p *Procedural) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if w == p.W && h == p.H {
		return
	}
	p.W = w
	p.H = h
	p.Grid = make([][]Pixel, h)
	for i := range p.Grid {
		p.Grid[i] = make([]Pixel, w)
	}
}

func (p *Procedural) SetConfig(cfg ProceduralConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = mergeProceduralDefaults(p.Kind, cfg)
}

func (p *Procedural) EffectiveConfig() ProceduralConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return cloneProceduralConfig(p.cfg)
}

func (p *Procedural) Snapshot() ProceduralSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ProceduralSnapshot{
		ProceduralState: p.snapshotStateLocked(),
	}
}

func (p *Procedural) RestoreSnapshot(s ProceduralSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.ProceduralState)
}

func (p *Procedural) SnapshotPersistedState() ProceduralPersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ProceduralPersistedState{
		ProceduralState: p.snapshotStateLocked(),
		RNGState:        p.rng.State(),
	}
}

func (p *Procedural) RestorePersistedState(s ProceduralPersistedState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s.ProceduralState)
	if s.RNGState != 0 {
		p.rng.SetState(s.RNGState)
	}
}

func (p *Procedural) SnapshotState() ProceduralState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotStateLocked()
}

func (p *Procedural) RestoreState(s ProceduralState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreStateLocked(s)
}

func (p *Procedural) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   p.tick,
		Timers: cloneTimerMap(p.timers),
		Values: cloneValueMap(p.values),
	}
}

func (p *Procedural) restoreStateLocked(s ProceduralState) {
	p.tick = s.Tick
	p.timers = cloneTimerMap(s.Timers)
	if p.timers == nil {
		p.timers = make(map[string]int)
	}
	p.values = cloneValueMap(s.Values)
	if p.values == nil {
		p.values = make(map[string]float64)
	}
}

func (p *Procedural) CurrentTick() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tick
}

func (p *Procedural) PerturbRNG(delta int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rng.Mix(delta)
}

func (p *Procedural) TriggerEvent(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.Kind {
	case "snow":
		switch name {
		case "gust":
			p.startSnowGustLocked("triggered")
		case "calm":
			p.startSnowCalmLocked("triggered")
		case "intro":
			p.startSnowIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, density=%.2f)", p.timers["intro"], p.cfg["intro_density"]))
		case "ending":
			p.startSnowEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "autumn-leaves":
		switch name {
		case "gust":
			p.startAutumnGustLocked("triggered")
		case "lull":
			p.startAutumnLullLocked("triggered")
		case "swirl":
			p.startAutumnSwirlLocked("triggered")
		case "intro":
			p.startAutumnIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, density=%.2f)", p.timers["intro"], p.cfg["intro_density"]))
		case "ending":
			p.startAutumnEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "starfield":
		switch name {
		case "shooting-star":
			p.startStarfieldShootingStarLocked("triggered")
		case "twinkle-burst":
			p.startStarfieldTwinkleBurstLocked("triggered")
		case "intro":
			p.startStarfieldIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, density=%.2f)", p.timers["intro"], p.cfg["intro_density"]))
		case "ending":
			p.startStarfieldEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "aurora":
		switch name {
		case "brighten":
			p.startAuroraBrightenLocked("triggered")
		case "shift":
			p.startAuroraShiftLocked("triggered")
		case "fade":
			p.startAuroraFadeLocked("triggered")
		case "intro":
			p.startAuroraIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", p.timers["intro"], p.cfg["intro_glow"]))
		case "ending":
			p.startAuroraEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "wheat-field":
		switch name {
		case "gust":
			p.startWheatFieldGustLocked("triggered")
		case "calm":
			p.startWheatFieldCalmLocked("triggered")
		case "intro":
			p.startWheatFieldIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, breeze=%.2f)", p.timers["intro"], p.cfg["intro_breeze"]))
		case "ending":
			p.startWheatFieldEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	default:
		return false
	}
}

func (p *Procedural) DrainLog() []LogEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.log) == 0 {
		return nil
	}
	out := p.log
	p.log = nil
	return out
}

func (p *Procedural) appendLog(kind, desc string) {
	p.log = append(p.log, LogEntry{Tick: p.tick, Type: kind, Desc: desc})
	if len(p.log) > 200 {
		p.log = p.log[len(p.log)-200:]
	}
}

func (p *Procedural) Step() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tick++
	for key, value := range p.timers {
		if value > 0 {
			p.timers[key] = value - 1
		}
	}

	switch p.Kind {
	case "snow":
		p.stepSnowLocked()
	case "autumn-leaves":
		p.stepAutumnLeavesLocked()
	case "starfield":
		p.stepStarfieldLocked()
	case "aurora":
		p.stepAuroraLocked()
	case "wheat-field":
		p.stepWheatFieldLocked()
	}
}

func (p *Procedural) intCfg(key string) int {
	return int(math.Round(p.cfg[key]))
}

func (p *Procedural) startSnowGustLocked(verb string) {
	p.timers["gust"] = jitterInt(p.rng, p.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["gust_push"] = sign * p.cfg["gust_mult"] * (0.45 + p.rng.Float64()*0.55)
	p.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["gust"], p.values["gust_push"]))
}

func (p *Procedural) startSnowCalmLocked(verb string) {
	p.timers["calm"] = jitterInt(p.rng, p.intCfg("calm_dur"), 0.3)
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["calm"], p.cfg["calm_mult"]))
}

func (p *Procedural) startSnowIntroLocked() {
	p.timers["gust"] = 0
	p.timers["calm"] = 0
	p.timers["ending"] = 0
	p.values["gust_push"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startSnowEndingLocked() {
	p.timers["intro"] = 0
	p.timers["gust"] = 0
	p.timers["calm"] = 0
	p.values["gust_push"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepSnowLocked() {
	if p.timers["gust"] <= 0 {
		p.values["gust_push"] = 0
	}
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
	}
	if p.timers["intro"] > 0 || p.timers["ending"] > 0 {
		return
	}
	if p.timers["gust"] <= 0 && p.cfg["gust_p"] > 0 && p.rng.Float64() < p.cfg["gust_p"] {
		p.startSnowGustLocked("started")
	}
	if p.timers["calm"] <= 0 && p.cfg["calm_p"] > 0 && p.rng.Float64() < p.cfg["calm_p"] {
		p.startSnowCalmLocked("started")
	}
}

func (p *Procedural) startAutumnGustLocked(verb string) {
	p.timers["gust"] = jitterInt(p.rng, p.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["gust_push"] = sign * p.cfg["gust_mult"] * (0.5 + p.rng.Float64()*0.7)
	p.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["gust"], p.values["gust_push"]))
}

func (p *Procedural) startAutumnLullLocked(verb string) {
	p.timers["lull"] = jitterInt(p.rng, p.intCfg("lull_dur"), 0.3)
	p.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lull"], p.cfg["lull_mult"]))
}

func (p *Procedural) startAutumnSwirlLocked(verb string) {
	p.timers["swirl"] = jitterInt(p.rng, p.intCfg("swirl_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["swirl_spin"] = sign * p.cfg["swirl_pull"] * (0.65 + p.rng.Float64()*0.45)
	p.values["swirl_row"] = float64(max(8, p.H/3)) + p.rng.Float64()*float64(max(1, p.H/2))
	p.values["swirl_col"] = p.rng.Float64() * float64(max(1, p.W))
	p.appendLog("swirl", fmt.Sprintf("%s (dur=%d, pull=%+.2f)", verb, p.timers["swirl"], p.values["swirl_spin"]))
}

func (p *Procedural) startAutumnIntroLocked() {
	p.timers["gust"] = 0
	p.timers["lull"] = 0
	p.timers["swirl"] = 0
	p.timers["ending"] = 0
	p.values["gust_push"] = 0
	p.values["swirl_spin"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startAutumnEndingLocked() {
	p.timers["intro"] = 0
	p.timers["gust"] = 0
	p.timers["lull"] = 0
	p.timers["swirl"] = 0
	p.values["gust_push"] = 0
	p.values["swirl_spin"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepAutumnLeavesLocked() {
	if p.timers["gust"] <= 0 {
		p.values["gust_push"] = 0
	}
	if p.timers["swirl"] <= 0 {
		p.values["swirl_spin"] = 0
	}
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
	}
	if p.timers["intro"] > 0 || p.timers["ending"] > 0 {
		return
	}
	if p.timers["gust"] <= 0 && p.cfg["gust_p"] > 0 && p.rng.Float64() < p.cfg["gust_p"] {
		p.startAutumnGustLocked("started")
	}
	if p.timers["lull"] <= 0 && p.cfg["lull_p"] > 0 && p.rng.Float64() < p.cfg["lull_p"] {
		p.startAutumnLullLocked("started")
	}
	if p.timers["swirl"] <= 0 && p.cfg["swirl_p"] > 0 && p.rng.Float64() < p.cfg["swirl_p"] {
		p.startAutumnSwirlLocked("started")
	}
}

func (p *Procedural) startStarfieldShootingStarLocked(verb string) {
	p.timers["shooting-star"] = jitterInt(p.rng, p.intCfg("shooting_star_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["shooting_dir"] = sign
	p.values["shooting_row"] = 6 + p.rng.Float64()*math.Max(4, float64(p.H)/3)
	p.values["shooting_start"] = p.rng.Float64() * float64(max(1, p.W))
	p.appendLog("shooting-star", fmt.Sprintf("%s (dur=%d, dir=%+.0f)", verb, p.timers["shooting-star"], sign))
}

func (p *Procedural) startStarfieldTwinkleBurstLocked(verb string) {
	p.timers["twinkle-burst"] = jitterInt(p.rng, p.intCfg("twinkle_burst_dur"), 0.3)
	p.appendLog("twinkle-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["twinkle-burst"], p.cfg["twinkle_burst_mult"]))
}

func (p *Procedural) startStarfieldIntroLocked() {
	p.timers["ending"] = 0
	p.timers["shooting-star"] = 0
	p.timers["twinkle-burst"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startStarfieldEndingLocked() {
	p.timers["intro"] = 0
	p.timers["shooting-star"] = 0
	p.timers["twinkle-burst"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepStarfieldLocked() {
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
	}
	if p.timers["intro"] > 0 || p.timers["ending"] > 0 {
		return
	}
	if p.timers["shooting-star"] <= 0 && p.cfg["shooting_star_p"] > 0 && p.rng.Float64() < p.cfg["shooting_star_p"] {
		p.startStarfieldShootingStarLocked("started")
	}
	if p.timers["twinkle-burst"] <= 0 && p.cfg["twinkle_burst_p"] > 0 && p.rng.Float64() < p.cfg["twinkle_burst_p"] {
		p.startStarfieldTwinkleBurstLocked("started")
	}
}

func (p *Procedural) startAuroraBrightenLocked(verb string) {
	p.timers["brighten"] = jitterInt(p.rng, p.intCfg("brighten_dur"), 0.3)
	p.values["brighten_gain"] = p.cfg["brighten_mult"] * (0.85 + p.rng.Float64()*0.35)
	p.appendLog("brighten", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["brighten"], p.values["brighten_gain"]))
}

func (p *Procedural) startAuroraShiftLocked(verb string) {
	p.timers["shift"] = jitterInt(p.rng, p.intCfg("shift_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["shift_push"] = sign * p.cfg["shift_amt"] * (0.55 + p.rng.Float64()*0.55)
	p.values["shift_seed"] = p.rng.Float64() * math.Pi * 2
	p.appendLog("shift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["shift"], p.values["shift_push"]))
}

func (p *Procedural) startAuroraFadeLocked(verb string) {
	p.timers["fade"] = jitterInt(p.rng, p.intCfg("fade_dur"), 0.3)
	p.appendLog("fade", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["fade"], p.cfg["fade_mult"]))
}

func (p *Procedural) startAuroraIntroLocked() {
	p.timers["brighten"] = 0
	p.timers["shift"] = 0
	p.timers["fade"] = 0
	p.timers["ending"] = 0
	p.values["brighten_gain"] = 0
	p.values["shift_push"] = 0
	p.values["shift_seed"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startAuroraEndingLocked() {
	p.timers["intro"] = 0
	p.timers["brighten"] = 0
	p.timers["shift"] = 0
	p.timers["fade"] = 0
	p.values["brighten_gain"] = 0
	p.values["shift_push"] = 0
	p.values["shift_seed"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepAuroraLocked() {
	if p.timers["brighten"] <= 0 {
		p.values["brighten_gain"] = 0
	}
	if p.timers["shift"] <= 0 {
		p.values["shift_push"] = 0
		p.values["shift_seed"] = 0
	}
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
	}
	if p.timers["intro"] > 0 || p.timers["ending"] > 0 {
		return
	}
	if p.timers["brighten"] <= 0 && p.cfg["brighten_p"] > 0 && p.rng.Float64() < p.cfg["brighten_p"] {
		p.startAuroraBrightenLocked("started")
	}
	if p.timers["shift"] <= 0 && p.cfg["shift_p"] > 0 && p.rng.Float64() < p.cfg["shift_p"] {
		p.startAuroraShiftLocked("started")
	}
	if p.timers["fade"] <= 0 && p.cfg["fade_p"] > 0 && p.rng.Float64() < p.cfg["fade_p"] {
		p.startAuroraFadeLocked("started")
	}
}

func (p *Procedural) startWheatFieldGustLocked(verb string) {
	p.timers["gust"] = jitterInt(p.rng, p.intCfg("gust_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.35 {
		sign = -1
	}
	p.values["gust_push"] = sign * p.cfg["gust_mult"] * (0.55 + p.rng.Float64()*0.55)
	p.appendLog("gust", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["gust"], p.values["gust_push"]))
}

func (p *Procedural) startWheatFieldCalmLocked(verb string) {
	p.timers["calm"] = jitterInt(p.rng, p.intCfg("calm_dur"), 0.3)
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["calm"], p.cfg["calm_mult"]))
}

func (p *Procedural) startWheatFieldIntroLocked() {
	p.timers["gust"] = 0
	p.timers["calm"] = 0
	p.timers["ending"] = 0
	p.values["gust_push"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startWheatFieldEndingLocked() {
	p.timers["intro"] = 0
	p.timers["gust"] = 0
	p.timers["calm"] = 0
	p.values["gust_push"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepWheatFieldLocked() {
	if p.timers["gust"] <= 0 {
		p.values["gust_push"] = 0
	}
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
	}
	if p.timers["intro"] > 0 || p.timers["ending"] > 0 {
		return
	}
	if p.timers["gust"] <= 0 && p.cfg["gust_p"] > 0 && p.rng.Float64() < p.cfg["gust_p"] {
		p.startWheatFieldGustLocked("started")
	}
	if p.timers["calm"] <= 0 && p.cfg["calm_p"] > 0 && p.rng.Float64() < p.cfg["calm_p"] {
		p.startWheatFieldCalmLocked("started")
	}
}
