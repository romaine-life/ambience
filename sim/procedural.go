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
