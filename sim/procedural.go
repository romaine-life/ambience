package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

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

var mysteriousManDefaults = ProceduralConfig{
	"intro_dur":          70,
	"intro_glow":         0.10,
	"ending_dur":         85,
	"ending_linger":      24,
	"ending_glow":        0.06,
	"figure_x":           0.5,
	"figure_height":      30.0,
	"figure_width":       11.0,
	"silhouette":         0.92,
	"hat":                1.0,
	"shoulder":           1.0,
	"ember_x":            0.56,
	"ember_y":            0.62,
	"ember_brightness":   0.86,
	"ember_pulse":        0.34,
	"smoke_density":      0.42,
	"smoke_rise":         0.46,
	"smoke_drift":        0.18,
	"smoke_softness":     0.62,
	"hue":                22,
	"hue_sp":             10,
	"sat":                0.72,
	"lmin":               0.06,
	"lmax":               0.86,
	"inhale_p":           0.0,
	"exhale_p":           0.0,
	"ash_fall_p":         0.0,
	"lighter_flick_p":    0.0,
	"inhale_dur":         32,
	"inhale_mult":        1.85,
	"exhale_dur":         60,
	"exhale_plume":       1.40,
	"ash_fall_dur":       28,
	"ash_fall_mult":      1.30,
	"lighter_flick_dur":  20,
	"lighter_flick_mult": 2.40,
}

var volcanoDefaults = ProceduralConfig{
	"intro_dur":         55,
	"intro_glow":        0.16,
	"ending_dur":        70,
	"ending_linger":     22,
	"ending_glow":       0.10,
	"horizon":           0.86,
	"cone_height":       28.0,
	"cone_width":        46.0,
	"crater_width":      8.0,
	"slope_jitter":      1.6,
	"glow":              0.55,
	"smoke":             0.32,
	"smoke_height":      18.0,
	"hue":               18,
	"hue_sp":            16,
	"sat":               0.78,
	"lmin":              0.18,
	"lmax":              0.92,
	"eruption_p":        0.0,
	"smolder_p":         0.0,
	"flare_p":           0.0,
	"eruption_dur":      80,
	"eruption_height":   28.0,
	"eruption_mult":     2.4,
	"smolder_dur":       80,
	"smolder_mult":      0.55,
	"flare_dur":         24,
	"flare_mult":        1.85,
}



func MysteriousManSchema() EffectSchema {
	return EffectSchema{
		Name: "mysterious-man",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent revealing the figure from full darkness as the first ember catches."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.10,
				Description: "Starting fraction of the final ember intensity before the silhouette resolves."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 85, Trigger: "ending",
				Description: "Ticks spent fading the ember and silhouette back toward darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24,
				Description: "Extra quiet ticks after the ember has mostly faded so the last smoke plume can thin out."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.06,
				Description: "Residual ember and silhouette presence near the end of the outro."},
			{Key: "figure_x", Label: "figure x", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.01, Default: 0.5,
				Description: "Horizontal position of the figure as a fraction of the frame width."},
			{Key: "figure_height", Label: "figure height", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 18, Max: 48, Step: 1, Default: 30,
				Description: "Vertical extent of the silhouette from shoulders to feet."},
			{Key: "figure_width", Label: "figure width", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 6, Max: 18, Step: 1, Default: 11,
				Description: "Horizontal extent of the silhouette body and shoulders."},
			{Key: "silhouette", Label: "silhouette", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.4, Max: 1.0, Step: 0.02, Default: 0.92,
				Description: "How dark the figure reads against the scene. Lower values let some shape detail show through."},
			{Key: "hat", Label: "hat", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 1.0,
				Description: "Adds a brimmed hat to the silhouette outline. Set to 0 to drop the hat."},
			{Key: "shoulder", Label: "shoulders", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 1.0,
				Description: "Adds a coat-shoulder bulge to the silhouette outline."},
			{Key: "ember_x", Label: "ember x", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.3, Max: 0.85, Step: 0.01, Default: 0.56,
				Description: "Horizontal position of the cigarette ember as a fraction of the frame width."},
			{Key: "ember_y", Label: "ember y", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.3, Max: 0.85, Step: 0.01, Default: 0.62,
				Description: "Vertical position of the cigarette ember as a fraction of the frame height."},
			{Key: "ember_brightness", Label: "ember bright", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.2, Max: 1.2, Step: 0.02, Default: 0.86,
				Description: "Steady-state brightness of the cigarette ember."},
			{Key: "ember_pulse", Label: "ember pulse", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.02, Default: 0.34,
				Description: "How much the ember breathes between dim and bright in the steady loop."},
			{Key: "smoke_density", Label: "smoke", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.05, Max: 1.0, Step: 0.02, Default: 0.42,
				Description: "Number of smoke puffs in the air at any one time."},
			{Key: "smoke_rise", Label: "rise speed", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.02, Default: 0.46,
				Description: "How quickly smoke puffs lift away from the ember."},
			{Key: "smoke_drift", Label: "drift", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Sideways carry on the smoke. Positive drifts right, negative drifts left."},
			{Key: "smoke_softness", Label: "softness", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.2, Max: 1.2, Step: 0.02, Default: 0.62,
				Description: "How softly smoke puffs fade. Higher values blur the edges into the dark."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 22,
				Description: "Base ember hue. Lower values lean redder; higher values lean orange-warm."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 10,
				Description: "Variation across ember core, halo, and reflected warmth on the figure."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Saturation of the ember and warm reflected accents."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.0, Max: 0.4, Step: 0.01, Default: 0.06,
				Description: "Minimum lightness used for the dark scene and silhouette body."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used for the brightest ember pixel."},
			{Key: "inhale_p", Label: "inhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "inhale",
				Description: "Per-tick chance of an inhale brightening the ember while smoke briefly compresses."},
			{Key: "exhale_p", Label: "exhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "exhale",
				Description: "Per-tick chance of an exhale releasing a visible smoke plume from the figure."},
			{Key: "ash_fall_p", Label: "ash fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "ash-fall",
				Description: "Per-tick chance of a small ash fleck breaking off the cigarette."},
			{Key: "lighter_flick_p", Label: "lighter flick", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "lighter-flick",
				Description: "Per-tick chance of a rare brighter ember catch like a lighter flicking on."},
			{Key: "inhale_dur", Label: "inhale dur", Slot: SlotEventMod, Group: "inhale", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 32,
				Description: "Duration of an inhale event in ticks."},
			{Key: "inhale_mult", Label: "inhale x", Slot: SlotEventMod, Group: "inhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Ember brightness multiplier applied while an inhale is active."},
			{Key: "exhale_dur", Label: "exhale dur", Slot: SlotEventMod, Group: "exhale", Type: KnobInt, Min: 10, Max: 160, Step: 2, Default: 60,
				Description: "Duration of an exhale plume in ticks."},
			{Key: "exhale_plume", Label: "plume size", Slot: SlotEventMod, Group: "exhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.4,
				Description: "Size and density multiplier of the exhaled smoke plume."},
			{Key: "ash_fall_dur", Label: "ash dur", Slot: SlotEventMod, Group: "ash-fall", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 28,
				Description: "How many ticks an ash fleck remains visible as it falls."},
			{Key: "ash_fall_mult", Label: "ash x", Slot: SlotEventMod, Group: "ash-fall", Type: KnobFloat, Min: 1.05, Max: 2.5, Step: 0.05, Default: 1.3,
				Description: "Brightness multiplier on the ash fleck while it falls."},
			{Key: "lighter_flick_dur", Label: "flick dur", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobInt, Min: 8, Max: 60, Step: 2, Default: 20,
				Description: "Duration of a lighter-flick brighter catch in ticks."},
			{Key: "lighter_flick_mult", Label: "flick x", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobFloat, Min: 1.2, Max: 4, Step: 0.05, Default: 2.4,
				Description: "Ember brightness multiplier applied during a lighter flick."},
		},
	}
}





func VolcanoSchema() EffectSchema {
	return EffectSchema{
		Name: "volcano",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent kindling crater glow and smoke hints before pressure builds."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Starting fraction of the crater glow before the mountain settles into idle pressure."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering eruptions back toward a quiet simmer."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks for ash and embers to fall back to the cone."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Residual crater glow that remains near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 0.6, Max: 0.95, Step: 0.01, Default: 0.86,
				Description: "Where the mountain base sits in the frame."},
			{Key: "cone_height", Label: "cone height", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 12, Max: 44, Step: 1, Default: 28,
				Description: "Height of the volcano silhouette above the base."},
			{Key: "cone_width", Label: "cone width", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 22, Max: 70, Step: 1, Default: 46,
				Description: "Base width of the cone silhouette."},
			{Key: "crater_width", Label: "crater", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 4, Max: 18, Step: 0.5, Default: 8,
				Description: "Width of the glowing crater notch at the cone's summit."},
			{Key: "slope_jitter", Label: "slope rough", Slot: SlotLever, Group: "mountain", Type: KnobFloat, Min: 0, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Per-column jitter applied to the silhouette so the slopes read as rocky rather than perfect."},
			{Key: "glow", Label: "crater glow", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Strength of the warm glow rising out of the crater during idle."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.01, Default: 0.32,
				Description: "How much smoke continually rises from the crater during idle."},
			{Key: "smoke_height", Label: "smoke height", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 6, Max: 40, Step: 1, Default: 18,
				Description: "How far smoke trails rise above the crater before fading."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Base lava and crater hue. Lower values warm toward red; higher values lean orange."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 16,
				Description: "Variation across crater core, lava sparks, and smoke tinting."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Overall saturation of the lava and crater glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the cone silhouette and dim smoke."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Maximum lightness used for the brightest lava sparks at peak eruption."},
			{Key: "eruption_p", Label: "eruption", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "eruption",
				Description: "Per-tick chance of a full eruption blasting lava sparks and ash above the crater."},
			{Key: "smolder_p", Label: "smolder", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "smolder",
				Description: "Per-tick chance of the mountain settling into a quieter, dimmer simmer."},
			{Key: "flare_p", Label: "flare", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "flare",
				Description: "Per-tick chance of a brief crater flare without the full eruption arc."},
			{Key: "eruption_dur", Label: "eruption dur", Slot: SlotEventMod, Group: "eruption", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 80,
				Description: "Duration of an eruption in ticks (jittered by +/-30%)."},
			{Key: "eruption_height", Label: "eruption arc", Slot: SlotEventMod, Group: "eruption", Type: KnobFloat, Min: 8, Max: 60, Step: 1, Default: 28,
				Description: "How far above the crater lava sparks reach at peak eruption."},
			{Key: "eruption_mult", Label: "eruption x", Slot: SlotEventMod, Group: "eruption", Type: KnobFloat, Min: 1.1, Max: 4, Step: 0.05, Default: 2.4,
				Description: "Glow and spark multiplier applied while an eruption is active."},
			{Key: "smolder_dur", Label: "smolder dur", Slot: SlotEventMod, Group: "smolder", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80,
				Description: "Duration of the quieter smoldering window."},
			{Key: "smolder_mult", Label: "smolder x", Slot: SlotEventMod, Group: "smolder", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Glow and smoke multiplier applied while smolder is active."},
			{Key: "flare_dur", Label: "flare dur", Slot: SlotEventMod, Group: "flare", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 24,
				Description: "Duration of a brief crater flare."},
			{Key: "flare_mult", Label: "flare x", Slot: SlotEventMod, Group: "flare", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Glow multiplier applied during a flare."},
		},
	}
}

func proceduralDefaults(kind string) ProceduralConfig {
	switch kind {
	case "volcano":
		return cloneConfig(volcanoDefaults)
	case "mysterious-man":
		return cloneConfig(mysteriousManDefaults)
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
	case "volcano":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = volcanoDefaults["intro_dur"]
		}
		out["intro_glow"] = clamp01(out["intro_glow"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = volcanoDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_glow"] = clamp01(out["ending_glow"])
		if out["horizon"] <= 0 {
			out["horizon"] = volcanoDefaults["horizon"]
		}
		if out["cone_height"] <= 0 {
			out["cone_height"] = volcanoDefaults["cone_height"]
		}
		if out["cone_width"] <= 0 {
			out["cone_width"] = volcanoDefaults["cone_width"]
		}
		if out["crater_width"] <= 0 {
			out["crater_width"] = volcanoDefaults["crater_width"]
		}
		if out["slope_jitter"] < 0 {
			out["slope_jitter"] = 0
		}
		if out["glow"] <= 0 {
			out["glow"] = volcanoDefaults["glow"]
		}
		if out["smoke"] < 0 {
			out["smoke"] = 0
		}
		if out["smoke_height"] <= 0 {
			out["smoke_height"] = volcanoDefaults["smoke_height"]
		}
		if out["hue"] < 0 {
			out["hue"] = volcanoDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = volcanoDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = volcanoDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = volcanoDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["eruption_dur"] <= 0 {
			out["eruption_dur"] = volcanoDefaults["eruption_dur"]
		}
		if out["eruption_height"] <= 0 {
			out["eruption_height"] = volcanoDefaults["eruption_height"]
		}
		if out["eruption_mult"] <= 0 {
			out["eruption_mult"] = volcanoDefaults["eruption_mult"]
		}
		if out["smolder_dur"] <= 0 {
			out["smolder_dur"] = volcanoDefaults["smolder_dur"]
		}
		if out["smolder_mult"] <= 0 {
			out["smolder_mult"] = volcanoDefaults["smolder_mult"]
		}
		if out["flare_dur"] <= 0 {
			out["flare_dur"] = volcanoDefaults["flare_dur"]
		}
		if out["flare_mult"] <= 0 {
			out["flare_mult"] = volcanoDefaults["flare_mult"]
		}
	case "mysterious-man":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = mysteriousManDefaults["intro_dur"]
		}
		out["intro_glow"] = clamp01(out["intro_glow"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = mysteriousManDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_glow"] = clamp01(out["ending_glow"])
		if out["figure_x"] <= 0 {
			out["figure_x"] = mysteriousManDefaults["figure_x"]
		}
		if out["figure_height"] <= 0 {
			out["figure_height"] = mysteriousManDefaults["figure_height"]
		}
		if out["figure_width"] <= 0 {
			out["figure_width"] = mysteriousManDefaults["figure_width"]
		}
		out["silhouette"] = clamp01(out["silhouette"])
		if out["silhouette"] <= 0 {
			out["silhouette"] = mysteriousManDefaults["silhouette"]
		}
		out["hat"] = clamp01(out["hat"])
		out["shoulder"] = clamp01(out["shoulder"])
		if out["ember_x"] <= 0 {
			out["ember_x"] = mysteriousManDefaults["ember_x"]
		}
		if out["ember_y"] <= 0 {
			out["ember_y"] = mysteriousManDefaults["ember_y"]
		}
		if out["ember_brightness"] <= 0 {
			out["ember_brightness"] = mysteriousManDefaults["ember_brightness"]
		}
		if out["ember_pulse"] < 0 {
			out["ember_pulse"] = 0
		}
		if out["smoke_density"] < 0 {
			out["smoke_density"] = 0
		}
		if out["smoke_rise"] <= 0 {
			out["smoke_rise"] = mysteriousManDefaults["smoke_rise"]
		}
		if out["smoke_softness"] <= 0 {
			out["smoke_softness"] = mysteriousManDefaults["smoke_softness"]
		}
		if out["hue"] < 0 {
			out["hue"] = mysteriousManDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = mysteriousManDefaults["sat"]
		}
		if out["lmin"] < 0 {
			out["lmin"] = 0
		}
		if out["lmax"] <= 0 {
			out["lmax"] = mysteriousManDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["inhale_dur"] <= 0 {
			out["inhale_dur"] = mysteriousManDefaults["inhale_dur"]
		}
		if out["inhale_mult"] <= 0 {
			out["inhale_mult"] = mysteriousManDefaults["inhale_mult"]
		}
		if out["exhale_dur"] <= 0 {
			out["exhale_dur"] = mysteriousManDefaults["exhale_dur"]
		}
		if out["exhale_plume"] <= 0 {
			out["exhale_plume"] = mysteriousManDefaults["exhale_plume"]
		}
		if out["ash_fall_dur"] <= 0 {
			out["ash_fall_dur"] = mysteriousManDefaults["ash_fall_dur"]
		}
		if out["ash_fall_mult"] <= 0 {
			out["ash_fall_mult"] = mysteriousManDefaults["ash_fall_mult"]
		}
		if out["lighter_flick_dur"] <= 0 {
			out["lighter_flick_dur"] = mysteriousManDefaults["lighter_flick_dur"]
		}
		if out["lighter_flick_mult"] <= 0 {
			out["lighter_flick_mult"] = mysteriousManDefaults["lighter_flick_mult"]
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
	return cloneConfig(p.cfg)
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
	case "volcano":
		switch name {
		case "eruption":
			p.startVolcanoEruptionLocked("triggered")
		case "smolder":
			p.startVolcanoSmolderLocked("triggered")
		case "flare":
			p.startVolcanoFlareLocked("triggered")
		case "intro":
			p.startVolcanoIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", p.timers["intro"], p.cfg["intro_glow"]))
		case "ending":
			p.startVolcanoEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "mysterious-man":
		switch name {
		case "inhale":
			p.startMysteriousManInhaleLocked("triggered")
		case "exhale":
			p.startMysteriousManExhaleLocked("triggered")
		case "ash-fall":
			p.startMysteriousManAshFallLocked("triggered")
		case "lighter-flick":
			p.startMysteriousManLighterFlickLocked("triggered")
		case "intro":
			p.startMysteriousManIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", p.timers["intro"], p.cfg["intro_glow"]))
		case "ending":
			p.startMysteriousManEndingLocked()
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
	case "volcano":
		p.stepVolcanoLocked()
	case "mysterious-man":
		p.stepMysteriousManLocked()
	}
}

func (p *Procedural) intCfg(key string) int {
	return int(math.Round(p.cfg[key]))
}










func (p *Procedural) startVolcanoEruptionLocked(verb string) {
	p.timers["eruption"] = jitterInt(p.rng, p.intCfg("eruption_dur"), 0.3)
	p.timers["smolder"] = 0
	p.values["eruption_gain"] = p.cfg["eruption_mult"] * (0.8 + p.rng.Float64()*0.45)
	p.values["eruption_seed"] = p.rng.Float64() * 1024
	p.appendLog("eruption", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["eruption"], p.values["eruption_gain"]))
}

func (p *Procedural) startVolcanoSmolderLocked(verb string) {
	p.timers["smolder"] = jitterInt(p.rng, p.intCfg("smolder_dur"), 0.3)
	p.timers["eruption"] = 0
	p.values["eruption_gain"] = 1
	p.appendLog("smolder", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["smolder"], p.cfg["smolder_mult"]))
}

func (p *Procedural) startVolcanoFlareLocked(verb string) {
	p.timers["flare"] = jitterInt(p.rng, p.intCfg("flare_dur"), 0.3)
	p.values["flare_gain"] = p.cfg["flare_mult"] * (0.85 + p.rng.Float64()*0.3)
	p.appendLog("flare", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["flare"], p.values["flare_gain"]))
}

func (p *Procedural) startVolcanoIntroLocked() {
	p.timers["eruption"] = 0
	p.timers["smolder"] = 0
	p.timers["flare"] = 0
	p.timers["ending"] = 0
	p.values["eruption_gain"] = 1
	p.values["flare_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startVolcanoEndingLocked() {
	p.timers["intro"] = 0
	p.timers["eruption"] = 0
	p.timers["smolder"] = 0
	p.timers["flare"] = 0
	p.values["eruption_gain"] = 1
	p.values["flare_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepVolcanoLocked() {
	if p.timers["eruption"] <= 0 {
		p.values["eruption_gain"] = 1
	}
	if p.timers["flare"] <= 0 {
		p.values["flare_gain"] = 1
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
	if p.timers["eruption"] <= 0 && p.timers["smolder"] <= 0 && p.cfg["eruption_p"] > 0 && p.rng.Float64() < p.cfg["eruption_p"] {
		p.startVolcanoEruptionLocked("started")
	}
	if p.timers["smolder"] <= 0 && p.timers["eruption"] <= 0 && p.cfg["smolder_p"] > 0 && p.rng.Float64() < p.cfg["smolder_p"] {
		p.startVolcanoSmolderLocked("started")
	}
	if p.timers["flare"] <= 0 && p.cfg["flare_p"] > 0 && p.rng.Float64() < p.cfg["flare_p"] {
		p.startVolcanoFlareLocked("started")
	}
}

func (p *Procedural) startMysteriousManInhaleLocked(verb string) {
	p.timers["inhale"] = jitterInt(p.rng, p.intCfg("inhale_dur"), 0.25)
	p.values["inhale_gain"] = p.cfg["inhale_mult"] * (0.8 + p.rng.Float64()*0.4)
	p.appendLog("inhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["inhale"], p.values["inhale_gain"]))
}

func (p *Procedural) startMysteriousManExhaleLocked(verb string) {
	p.timers["exhale"] = jitterInt(p.rng, p.intCfg("exhale_dur"), 0.25)
	p.values["exhale_gain"] = p.cfg["exhale_plume"] * (0.85 + p.rng.Float64()*0.35)
	p.values["exhale_seed"] = p.rng.Float64() * 1024
	p.appendLog("exhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["exhale"], p.values["exhale_gain"]))
}

func (p *Procedural) startMysteriousManAshFallLocked(verb string) {
	p.timers["ash-fall"] = jitterInt(p.rng, p.intCfg("ash_fall_dur"), 0.3)
	p.values["ash_gain"] = p.cfg["ash_fall_mult"] * (0.85 + p.rng.Float64()*0.3)
	p.values["ash_seed"] = p.rng.Float64() * 1024
	p.appendLog("ash-fall", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["ash-fall"], p.values["ash_gain"]))
}

func (p *Procedural) startMysteriousManLighterFlickLocked(verb string) {
	p.timers["lighter-flick"] = jitterInt(p.rng, p.intCfg("lighter_flick_dur"), 0.25)
	p.values["flick_gain"] = p.cfg["lighter_flick_mult"] * (0.85 + p.rng.Float64()*0.3)
	p.appendLog("lighter-flick", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lighter-flick"], p.values["flick_gain"]))
}

func (p *Procedural) startMysteriousManIntroLocked() {
	p.timers["inhale"] = 0
	p.timers["exhale"] = 0
	p.timers["ash-fall"] = 0
	p.timers["lighter-flick"] = 0
	p.timers["ending"] = 0
	p.values["inhale_gain"] = 1
	p.values["exhale_gain"] = 1
	p.values["ash_gain"] = 1
	p.values["flick_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startMysteriousManEndingLocked() {
	p.timers["intro"] = 0
	p.timers["inhale"] = 0
	p.timers["exhale"] = 0
	p.timers["ash-fall"] = 0
	p.timers["lighter-flick"] = 0
	p.values["inhale_gain"] = 1
	p.values["exhale_gain"] = 1
	p.values["ash_gain"] = 1
	p.values["flick_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepMysteriousManLocked() {
	if p.timers["inhale"] <= 0 {
		p.values["inhale_gain"] = 1
	}
	if p.timers["exhale"] <= 0 {
		p.values["exhale_gain"] = 1
	}
	if p.timers["ash-fall"] <= 0 {
		p.values["ash_gain"] = 1
	}
	if p.timers["lighter-flick"] <= 0 {
		p.values["flick_gain"] = 1
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
	if p.timers["inhale"] <= 0 && p.cfg["inhale_p"] > 0 && p.rng.Float64() < p.cfg["inhale_p"] {
		p.startMysteriousManInhaleLocked("started")
	}
	if p.timers["exhale"] <= 0 && p.cfg["exhale_p"] > 0 && p.rng.Float64() < p.cfg["exhale_p"] {
		p.startMysteriousManExhaleLocked("started")
	}
	if p.timers["ash-fall"] <= 0 && p.cfg["ash_fall_p"] > 0 && p.rng.Float64() < p.cfg["ash_fall_p"] {
		p.startMysteriousManAshFallLocked("started")
	}
	if p.timers["lighter-flick"] <= 0 && p.cfg["lighter_flick_p"] > 0 && p.rng.Float64() < p.cfg["lighter_flick_p"] {
		p.startMysteriousManLighterFlickLocked("started")
	}
}
