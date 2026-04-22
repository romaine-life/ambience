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

var beachDefaults = ProceduralConfig{
	"intro_dur":       55,
	"intro_tide":      0.18,
	"ending_dur":      65,
	"ending_linger":   18,
	"ending_wet":      0.10,
	"shoreline":       0.58,
	"tide_amp":        6.0,
	"wave_amp":        2.4,
	"wave_freq":       0.18,
	"speed":           0.10,
	"slope":           0.16,
	"foam":            0.36,
	"shimmer":         0.22,
	"hue":             198,
	"hue_sp":          16,
	"sat":             0.50,
	"lmin":            0.28,
	"lmax":            0.82,
	"high_tide_p":     0.0,
	"low_tide_p":      0.0,
	"foam_burst_p":    0.0,
	"high_tide_dur":   60,
	"high_tide_push":  1.40,
	"low_tide_dur":    58,
	"low_tide_pull":   1.20,
	"foam_burst_dur":  34,
	"foam_burst_mult": 1.90,
}

var campfireDefaults = ProceduralConfig{
	"intro_dur":     45,
	"intro_glow":    0.14,
	"ending_dur":    60,
	"ending_linger": 24,
	"ending_glow":   0.08,
	"flame_height":  14.0,
	"flame_width":   10.0,
	"flame_speed":   0.12,
	"flicker":       0.72,
	"ember_rate":    0.26,
	"ember_speed":   0.62,
	"glow":          0.54,
	"hue":           24,
	"hue_sp":        18,
	"sat":           0.82,
	"lmin":          0.32,
	"lmax":          0.94,
	"crackle_p":     0.0,
	"lull_p":        0.0,
	"crackle_dur":   36,
	"crackle_mult":  1.85,
	"lull_dur":      68,
	"lull_mult":     0.55,
}

var windmillDefaults = ProceduralConfig{
	"intro_dur":     45,
	"intro_turn":    0.12,
	"ending_dur":    60,
	"ending_linger": 20,
	"ending_turn":   0.05,
	"turn_speed":    0.08,
	"blade_len":     14.0,
	"blade_width":   1.8,
	"tower_height":  20.0,
	"tower_width":   6.0,
	"horizon":       0.72,
	"glow":          0.18,
	"hue":           28,
	"hue_sp":        18,
	"sat":           0.42,
	"lmin":          0.18,
	"lmax":          0.82,
	"gust_p":        0.0,
	"lull_p":        0.0,
	"gust_dur":      50,
	"gust_mult":     1.90,
	"lull_dur":      72,
	"lull_mult":     0.45,
}

var lighthouseDefaults = ProceduralConfig{
	"intro_dur":        50,
	"intro_beam":       0.16,
	"ending_dur":       65,
	"ending_linger":    18,
	"ending_beam":      0.08,
	"sweep_speed":      0.08,
	"beam_width":       0.22,
	"beam_softness":    0.42,
	"tower_height":     22.0,
	"tower_width":      6.5,
	"horizon":          0.74,
	"haze":             0.14,
	"glow":             0.22,
	"hue":              214,
	"hue_sp":           18,
	"sat":              0.34,
	"lmin":             0.12,
	"lmax":             0.84,
	"bright_pass_p":    0.0,
	"fog_thicken_p":    0.0,
	"calm_p":           0.0,
	"bright_pass_dur":  42,
	"bright_pass_mult": 1.75,
	"fog_thicken_dur":  72,
	"fog_thicken_mult": 1.85,
	"calm_dur":         64,
	"calm_mult":        0.55,
}

var rowboatDefaults = ProceduralConfig{
	"intro_dur":     50,
	"intro_drift":   0.18,
	"ending_dur":    65,
	"ending_linger": 18,
	"ending_ripple": 0.08,
	"waterline":     0.58,
	"drift_speed":   0.08,
	"bob_amp":       1.20,
	"wave_amp":      1.60,
	"wave_freq":     0.16,
	"ripple":        0.24,
	"reflection":    0.22,
	"boat_len":      14.0,
	"boat_height":   3.5,
	"hue":           206,
	"hue_sp":        16,
	"sat":           0.36,
	"lmin":          0.16,
	"lmax":          0.82,
	"wake_p":        0.0,
	"drift_p":       0.0,
	"calm_p":        0.0,
	"wake_dur":      40,
	"wake_mult":     1.85,
	"drift_dur":     58,
	"drift_push":    1.30,
	"calm_dur":      72,
	"calm_mult":     0.50,
}

var underwaterDefaults = ProceduralConfig{
	"intro_dur":          55,
	"intro_reveal":       0.14,
	"ending_dur":         70,
	"ending_linger":      22,
	"ending_murk":        0.08,
	"density":            0.28,
	"rise_speed":         0.42,
	"drift":              0.10,
	"sway":               0.54,
	"weed_height":        20.0,
	"weed_count":         11.0,
	"caustics":           0.30,
	"depth":              0.56,
	"hue":                192,
	"hue_sp":             18,
	"sat":                0.42,
	"lmin":               0.12,
	"lmax":               0.82,
	"bubble_burst_p":     0.0,
	"current_shift_p":    0.0,
	"calm_p":             0.0,
	"bubble_burst_dur":   38,
	"bubble_burst_mult":  1.90,
	"current_shift_dur":  62,
	"current_shift_push": 1.20,
	"calm_dur":           74,
	"calm_mult":          0.55,
}

var volcanoDefaults = ProceduralConfig{
	"intro_dur":     55,
	"intro_glow":    0.12,
	"ending_dur":    70,
	"ending_linger": 22,
	"ending_embers": 0.08,
	"horizon":       0.80,
	"cone_height":   26.0,
	"cone_width":    44.0,
	"crater_width":  8.0,
	"glow":          0.28,
	"smoke":         0.22,
	"ash":           0.18,
	"hue":           18,
	"hue_sp":        18,
	"sat":           0.78,
	"lmin":          0.08,
	"lmax":          0.88,
	"eruption_p":    0.0,
	"smolder_p":     0.0,
	"flare_p":       0.0,
	"eruption_dur":  42,
	"eruption_mult": 2.20,
	"smolder_dur":   72,
	"smolder_mult":  1.60,
	"flare_dur":     28,
	"flare_mult":    1.90,
}

var trainDefaults = ProceduralConfig{
	"intro_dur":      50,
	"intro_cue":      0.12,
	"ending_dur":     70,
	"ending_linger":  18,
	"ending_clear":   0.08,
	"horizon":        0.74,
	"track_row":      0.82,
	"train_height":   4.5,
	"car_count":      6.0,
	"car_gap":        1.0,
	"speed":          1.05,
	"smoke":          0.18,
	"headlight":      0.24,
	"hue":            220,
	"hue_sp":         20,
	"sat":            0.34,
	"lmin":           0.08,
	"lmax":           0.86,
	"pass_p":         0.0,
	"express_p":      0.0,
	"quiet_gap_p":    0.0,
	"pass_dur":       72,
	"pass_mult":      1.15,
	"express_dur":    46,
	"express_mult":   1.90,
	"quiet_gap_dur":  90,
	"quiet_gap_mult": 0.45,
}

var mysteriousManDefaults = ProceduralConfig{
	"intro_dur":          55,
	"intro_reveal":       0.08,
	"ending_dur":         70,
	"ending_linger":      20,
	"ending_shadow":      0.08,
	"figure_x":           0.38,
	"figure_scale":       1.0,
	"lean":               0.08,
	"contrast":           0.24,
	"ember":              0.24,
	"hold_angle":         0.22,
	"smoke":              0.16,
	"rise_speed":         0.72,
	"drift":              0.08,
	"hue":                216,
	"hue_sp":             18,
	"sat":                0.18,
	"lmin":               0.04,
	"lmax":               0.88,
	"inhale_p":           0.0,
	"exhale_p":           0.0,
	"ash_fall_p":         0.0,
	"lighter_flick_p":    0.0,
	"inhale_dur":         34,
	"inhale_mult":        1.80,
	"exhale_dur":         46,
	"exhale_mult":        1.65,
	"ash_fall_dur":       24,
	"ash_fall_mult":      1.40,
	"lighter_flick_dur":  20,
	"lighter_flick_mult": 2.25,
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

func BeachSchema() EffectSchema {
	return EffectSchema{
		Name: "beach",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent establishing the first advancing tide rhythm from a calmer shoreline."},
			{Key: "intro_tide", Label: "intro tide", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Starting fraction of the full tide motion before the shoreline settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent easing the waterline back out toward a calmer shore."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 140, Step: 5, Default: 18,
				Description: "Extra quiet ticks for wet sand and foam remnants to fade after the retreat."},
			{Key: "ending_wet", Label: "ending wet", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.10,
				Description: "Residual shoreline motion and wet-sand presence near the end of the outro."},
			{Key: "shoreline", Label: "shoreline", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.42, Max: 0.76, Step: 0.01, Default: 0.58,
				Description: "Base shoreline height in the frame."},
			{Key: "tide_amp", Label: "tide amp", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 1, Max: 14, Step: 0.5, Default: 6,
				Description: "How far the tide line advances and retreats."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.5, Max: 6, Step: 0.1, Default: 2.4,
				Description: "Height of the small shoreline ripples layered on top of the tide."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.05, Max: 0.35, Step: 0.01, Default: 0.18,
				Description: "Horizontal frequency of the shoreline wiggle."},
			{Key: "speed", Label: "tide speed", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.02, Max: 0.3, Step: 0.01, Default: 0.10,
				Description: "How quickly the tide rhythm moves in and out."},
			{Key: "slope", Label: "shore slope", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: -0.35, Max: 0.35, Step: 0.01, Default: 0.16,
				Description: "Diagonal slant of the shoreline across the frame."},
			{Key: "foam", Label: "foam", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.36,
				Description: "Brightness and thickness of the foam edge at the waterline."},
			{Key: "shimmer", Label: "shimmer", Slot: SlotLever, Group: "shore", Type: KnobFloat, Min: 0.02, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Strength of the water-surface shimmer away from the foam line."},
			{Key: "hue", Label: "water hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 220, Step: 1, Default: 198,
				Description: "Base water hue. Lower values lean teal; higher values lean deep blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 16,
				Description: "Variation across water and foam accents."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.9, Step: 0.01, Default: 0.50,
				Description: "Overall water color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.6, Step: 0.01, Default: 0.28,
				Description: "Minimum lightness used for deeper water and wet sand."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for foam and bright shore reflections."},
			{Key: "high_tide_p", Label: "high tide", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "high-tide",
				Description: "Per-tick chance of the shoreline pushing farther inland for a while."},
			{Key: "low_tide_p", Label: "low tide", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "low-tide",
				Description: "Per-tick chance of the shoreline pulling farther back down the beach."},
			{Key: "foam_burst_p", Label: "foam burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "foam-burst",
				Description: "Per-tick chance of a brighter foamy wash crossing the edge."},
			{Key: "high_tide_dur", Label: "high dur", Slot: SlotEventMod, Group: "high-tide", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60,
				Description: "Duration of the stronger high-tide push."},
			{Key: "high_tide_push", Label: "high push", Slot: SlotEventMod, Group: "high-tide", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.4,
				Description: "Additional inward shoreline offset during high tide."},
			{Key: "low_tide_dur", Label: "low dur", Slot: SlotEventMod, Group: "low-tide", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "Duration of the lower-tide retreat."},
			{Key: "low_tide_pull", Label: "low pull", Slot: SlotEventMod, Group: "low-tide", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.2,
				Description: "Additional outward shoreline offset during low tide."},
			{Key: "foam_burst_dur", Label: "foam dur", Slot: SlotEventMod, Group: "foam-burst", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 34,
				Description: "Duration of a brighter foamy edge."},
			{Key: "foam_burst_mult", Label: "foam x", Slot: SlotEventMod, Group: "foam-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Brightness multiplier applied during a foam burst."},
		},
	}
}

func CampfireSchema() EffectSchema {
	return EffectSchema{
		Name: "campfire",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks spent catching from a small glow into a stable flame."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Starting fraction of the final campfire intensity before the flame catches fully."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent collapsing the flame down toward ember glow."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24,
				Description: "Extra ember time after the flame has mostly died back."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual flame and ember intensity near the end of the outro."},
			{Key: "flame_height", Label: "flame height", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 6, Max: 24, Step: 0.5, Default: 14,
				Description: "Overall flame height above the logs."},
			{Key: "flame_width", Label: "flame width", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 4, Max: 18, Step: 0.5, Default: 10,
				Description: "Width of the flame body and ember bed."},
			{Key: "flame_speed", Label: "flame speed", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 0.04, Max: 0.3, Step: 0.01, Default: 0.12,
				Description: "How quickly the flame shape flickers and rolls."},
			{Key: "flicker", Label: "flicker", Slot: SlotLever, Group: "flame", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.72,
				Description: "Side-to-side and height variation inside the flame body."},
			{Key: "ember_rate", Label: "ember rate", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.26,
				Description: "How many embers rise from the fire during the steady state."},
			{Key: "ember_speed", Label: "ember speed", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.1, Max: 1.4, Step: 0.02, Default: 0.62,
				Description: "How quickly embers travel upward and fade."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "embers", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.54,
				Description: "Strength of the warm localized light cast around the campfire."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 8, Max: 50, Step: 1, Default: 24,
				Description: "Base flame hue. Lower values lean redder; higher values lean more yellow-orange."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 18,
				Description: "Variation between coal reds, orange mids, and bright flame tips."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.82,
				Description: "Overall color saturation of the fire and ember tones."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 0.7, Step: 0.01, Default: 0.32,
				Description: "Minimum lightness used for darker coals, logs, and outer flame edges."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.94,
				Description: "Maximum lightness used for the hottest flame cores and bright embers."},
			{Key: "crackle_p", Label: "crackle", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "crackle",
				Description: "Per-tick chance of a brighter crackle that throws extra embers."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the flame briefly settling into a lower, calmer burn."},
			{Key: "crackle_dur", Label: "crackle dur", Slot: SlotEventMod, Group: "crackle", Type: KnobInt, Min: 8, Max: 160, Step: 4, Default: 36,
				Description: "Duration of the brighter crackling burst."},
			{Key: "crackle_mult", Label: "crackle x", Slot: SlotEventMod, Group: "crackle", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Intensity multiplier applied while a crackle burst is active."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 68,
				Description: "Duration of the quieter lower-flame window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Flame intensity multiplier applied while lull is active."},
		},
	}
}

func WindmillSchema() EffectSchema {
	return EffectSchema{
		Name: "windmill",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 45, Trigger: "intro",
				Description: "Ticks spent easing the blades from stillness into a readable turn."},
			{Key: "intro_turn", Label: "intro turn", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Starting fraction of the final rotation speed before the mill settles into motion."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "ending",
				Description: "Ticks spent coasting the blades back down toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks after the blades have mostly stopped."},
			{Key: "ending_turn", Label: "ending turn", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.01, Max: 0.35, Step: 0.01, Default: 0.05,
				Description: "Residual blade motion near the end of the outro."},
			{Key: "turn_speed", Label: "turn speed", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.02, Max: 0.25, Step: 0.01, Default: 0.08,
				Description: "Base blade rotation speed."},
			{Key: "blade_len", Label: "blade len", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 6, Max: 22, Step: 0.5, Default: 14,
				Description: "Length of the windmill blades."},
			{Key: "blade_width", Label: "blade width", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.8,
				Description: "Thickness of each blade arm."},
			{Key: "tower_height", Label: "tower height", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 10, Max: 30, Step: 0.5, Default: 20,
				Description: "Height of the windmill tower above the hill."},
			{Key: "tower_width", Label: "tower width", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 3, Max: 10, Step: 0.5, Default: 6,
				Description: "Width of the windmill tower silhouette."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.56, Max: 0.86, Step: 0.01, Default: 0.72,
				Description: "Height of the ground line and hill in frame."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "mill", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Strength of the dusk haze and tiny warm window glow."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 10, Max: 240, Step: 1, Default: 28,
				Description: "Base sky hue spanning cool night blues through warm dusk."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 28, Step: 1, Default: 18,
				Description: "Variation between the upper sky and the horizon glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.42,
				Description: "Overall sky and glow saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the upper sky and dark ground."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for the horizon and glow."},
			{Key: "gust_p", Label: "gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "gust",
				Description: "Per-tick chance of the blades briefly spinning faster."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of the wind settling into a slower turn."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "gust", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50,
				Description: "Duration of a faster-turning gust."},
			{Key: "gust_mult", Label: "gust x", Slot: SlotEventMod, Group: "gust", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Rotation multiplier applied during a gust."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the calmer slower-turning window."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Rotation multiplier applied while lull is active."},
		},
	}
}

func LighthouseSchema() EffectSchema {
	return EffectSchema{
		Name: "lighthouse",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent bringing the first sweep up from a dim narrow beam."},
			{Key: "intro_beam", Label: "intro beam", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Starting fraction of the full beam presence before the lighthouse settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent fading the sweep down toward darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 18,
				Description: "Extra quiet ticks after the beam has mostly faded."},
			{Key: "ending_beam", Label: "ending beam", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual beam presence near the end of the outro."},
			{Key: "sweep_speed", Label: "sweep speed", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.2, Step: 0.01, Default: 0.08,
				Description: "How quickly the beam sweeps back across the scene."},
			{Key: "beam_width", Label: "beam width", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.06, Max: 0.45, Step: 0.01, Default: 0.22,
				Description: "Angular width of the sweeping light wedge."},
			{Key: "beam_softness", Label: "beam soft", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.05, Default: 0.42,
				Description: "Soft falloff from the beam core into surrounding haze."},
			{Key: "tower_height", Label: "tower height", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 10, Max: 30, Step: 0.5, Default: 22,
				Description: "Height of the lighthouse silhouette above the horizon."},
			{Key: "tower_width", Label: "tower width", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 3, Max: 10, Step: 0.5, Default: 6.5,
				Description: "Width of the lighthouse tower silhouette."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "tower", Type: KnobFloat, Min: 0.56, Max: 0.86, Step: 0.01, Default: 0.74,
				Description: "Height of the horizon and coastline in frame."},
			{Key: "haze", Label: "haze", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Base atmospheric haze around the beam and horizon."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "beam", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.22,
				Description: "Strength of the lamp glow near the tower head."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 240, Step: 1, Default: 214,
				Description: "Base night-sky hue. Lower values lean teal; higher values lean deeper blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between upper sky, beam haze, and horizon glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.34,
				Description: "Overall scene saturation for sky and beam haze."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Minimum lightness used for the darkest sky and sea areas."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.84,
				Description: "Maximum lightness used for the beam and horizon glow."},
			{Key: "bright_pass_p", Label: "bright pass", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "bright-pass",
				Description: "Per-tick chance of a brighter beam pass cutting across the scene."},
			{Key: "fog_thicken_p", Label: "fog thicken", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "fog-thicken",
				Description: "Per-tick chance of the air thickening into a hazier softer sweep."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter lower-intensity interval between brighter passes."},
			{Key: "bright_pass_dur", Label: "bright dur", Slot: SlotEventMod, Group: "bright-pass", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 42,
				Description: "Duration of a brighter beam pass."},
			{Key: "bright_pass_mult", Label: "bright x", Slot: SlotEventMod, Group: "bright-pass", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.75,
				Description: "Brightness multiplier applied during a bright pass."},
			{Key: "fog_thicken_dur", Label: "fog dur", Slot: SlotEventMod, Group: "fog-thicken", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the thicker fog window."},
			{Key: "fog_thicken_mult", Label: "fog x", Slot: SlotEventMod, Group: "fog-thicken", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Haze multiplier applied while fog thickening is active."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 64,
				Description: "Duration of the quieter lower-beam interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Beam intensity multiplier applied while calm is active."},
		},
	}
}

func RowboatSchema() EffectSchema {
	return EffectSchema{
		Name: "rowboat",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent easing the boat and its first ripples into view."},
			{Key: "intro_drift", Label: "intro drift", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Starting fraction of the final drift and bob motion before the scene settles."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 65, Trigger: "ending",
				Description: "Ticks spent flattening the ripples and easing the boat toward stillness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 18,
				Description: "Extra quiet ticks after the visible motion has mostly faded."},
			{Key: "ending_ripple", Label: "ending ripple", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual motion and ripple presence near the end of the outro."},
			{Key: "waterline", Label: "waterline", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.42, Max: 0.76, Step: 0.01, Default: 0.58,
				Description: "Height of the lake surface in the frame."},
			{Key: "drift_speed", Label: "drift speed", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.02, Max: 0.2, Step: 0.01, Default: 0.08,
				Description: "How quickly the boat drifts and the wave phase rolls underneath it."},
			{Key: "bob_amp", Label: "bob amp", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.1, Default: 1.2,
				Description: "Vertical bobbing amplitude of the hull."},
			{Key: "wave_amp", Label: "wave amp", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.2, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Amplitude of the wider surface undulation behind the boat."},
			{Key: "wave_freq", Label: "wave freq", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.35, Step: 0.01, Default: 0.16,
				Description: "Horizontal frequency of the lake surface wiggle."},
			{Key: "ripple", Label: "ripple", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.24,
				Description: "Strength and number of the local ripples around the boat."},
			{Key: "reflection", Label: "reflection", Slot: SlotLever, Group: "lake", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Visibility of the boat and ripple reflection in the water."},
			{Key: "boat_len", Label: "boat len", Slot: SlotLever, Group: "boat", Type: KnobFloat, Min: 6, Max: 24, Step: 0.5, Default: 14,
				Description: "Length of the rowboat hull."},
			{Key: "boat_height", Label: "boat height", Slot: SlotLever, Group: "boat", Type: KnobFloat, Min: 1, Max: 8, Step: 0.5, Default: 3.5,
				Description: "Height of the rowboat silhouette above the waterline."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 230, Step: 1, Default: 206,
				Description: "Base water and sky hue. Lower values lean teal; higher values lean deeper blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 24, Step: 1, Default: 16,
				Description: "Variation between the upper sky, water, and reflected highlights."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.36,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.08, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Minimum lightness used for darker water and hull shadow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.35, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for sky glow and water highlights."},
			{Key: "wake_p", Label: "wake", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wake",
				Description: "Per-tick chance of a more pronounced wake rippling out behind the boat."},
			{Key: "drift_p", Label: "drift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "drift",
				Description: "Per-tick chance of the boat being pushed gently farther to one side."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of the lake briefly flattening into calmer motion."},
			{Key: "wake_dur", Label: "wake dur", Slot: SlotEventMod, Group: "wake", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 40,
				Description: "Duration of the more pronounced wake window."},
			{Key: "wake_mult", Label: "wake x", Slot: SlotEventMod, Group: "wake", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Ripple multiplier applied while wake is active."},
			{Key: "drift_dur", Label: "drift dur", Slot: SlotEventMod, Group: "drift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 58,
				Description: "Duration of the stronger side drift window."},
			{Key: "drift_push", Label: "drift push", Slot: SlotEventMod, Group: "drift", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.3,
				Description: "Additional sideways push applied during a drift event."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the calmer low-motion interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.5,
				Description: "Motion and ripple multiplier applied while calm is active."},
		},
	}
}

func UnderwaterSchema() EffectSchema {
	return EffectSchema{
		Name: "underwater",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent brightening out of murk before the full underwater scene settles in."},
			{Key: "intro_reveal", Label: "intro reveal", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Starting fraction of the final bubble, caustic, and sway activity."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent dimming the water column back toward still murk."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks after the main motion has mostly faded."},
			{Key: "ending_murk", Label: "ending murk", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual underwater activity and light left near the end of the outro."},
			{Key: "density", Label: "bubble density", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.28,
				Description: "Base density of drifting bubbles across the scene."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.02, Default: 0.42,
				Description: "How quickly bubbles rise through the water column."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.01, Default: 0.10,
				Description: "Baseline sideways carry applied to bubbles and suspended particles."},
			{Key: "sway", Label: "sway", Slot: SlotLever, Group: "seaweed", Type: KnobFloat, Min: 0.05, Max: 1.4, Step: 0.02, Default: 0.54,
				Description: "How much the seaweed and finer particles sway with the water."},
			{Key: "weed_height", Label: "weed height", Slot: SlotLever, Group: "seaweed", Type: KnobFloat, Min: 8, Max: 30, Step: 0.5, Default: 20,
				Description: "Height of the tallest seaweed fronds."},
			{Key: "weed_count", Label: "weed count", Slot: SlotLever, Group: "seaweed", Type: KnobInt, Min: 4, Max: 18, Step: 1, Default: 11,
				Description: "Number of seaweed clumps anchored along the bottom."},
			{Key: "caustics", Label: "caustics", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.30,
				Description: "Strength of the shifting light bands and shafts near the surface."},
			{Key: "depth", Label: "depth", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0.15, Max: 0.95, Step: 0.01, Default: 0.56,
				Description: "How murky and deep the water feels overall."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 220, Step: 1, Default: 192,
				Description: "Base water hue. Lower values lean greener; higher values lean bluer."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between the light shafts, water body, and seaweed glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.8, Step: 0.01, Default: 0.42,
				Description: "Overall underwater color saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Minimum lightness used for deep water and seabed shadows."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.25, Max: 0.95, Step: 0.01, Default: 0.82,
				Description: "Maximum lightness used for bubbles and surface caustics."},
			{Key: "bubble_burst_p", Label: "bubble burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "bubble-burst",
				Description: "Per-tick chance of a denser burst of bubbles rising through the frame."},
			{Key: "current_shift_p", Label: "current shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "current-shift",
				Description: "Per-tick chance of the water current leaning harder to one side."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of bubble activity and sway easing into a quieter interval."},
			{Key: "bubble_burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 38,
				Description: "Duration of a denser bubble burst."},
			{Key: "bubble_burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "bubble-burst", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Bubble density multiplier applied during a bubble burst."},
			{Key: "current_shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "current-shift", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 62,
				Description: "Duration of the stronger sideways current window."},
			{Key: "current_shift_push", Label: "shift push", Slot: SlotEventMod, Group: "current-shift", Type: KnobFloat, Min: 0.2, Max: 3, Step: 0.05, Default: 1.2,
				Description: "Additional horizontal push applied during a current shift."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 74,
				Description: "Duration of the calmer low-motion underwater interval."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Bubble and sway multiplier applied while calm is active."},
		},
	}
}

func VolcanoSchema() EffectSchema {
	return EffectSchema{
		Name: "volcano",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent building the crater glow and first smoke before the full scene settles."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Starting fraction of the final crater glow and smoke intensity."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent tapering eruptions and heat back toward a quieter mountain."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 22,
				Description: "Extra quiet ticks for the last embers and smoke wisps to fade."},
			{Key: "ending_embers", Label: "ending embers", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Residual glow and ember activity near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "cone", Type: KnobFloat, Min: 0.62, Max: 0.9, Step: 0.01, Default: 0.8,
				Description: "Height of the ground plane where the volcano sits."},
			{Key: "cone_height", Label: "cone height", Slot: SlotLever, Group: "cone", Type: KnobFloat, Min: 12, Max: 38, Step: 0.5, Default: 26,
				Description: "Height of the volcano silhouette above the horizon."},
			{Key: "cone_width", Label: "cone width", Slot: SlotLever, Group: "cone", Type: KnobFloat, Min: 20, Max: 70, Step: 1, Default: 44,
				Description: "Width of the volcano base silhouette."},
			{Key: "crater_width", Label: "crater width", Slot: SlotLever, Group: "cone", Type: KnobFloat, Min: 3, Max: 16, Step: 0.5, Default: 8,
				Description: "Width of the glowing crater opening near the peak."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.28,
				Description: "Baseline strength of the crater glow and hot haze."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0.02, Max: 0.7, Step: 0.01, Default: 0.22,
				Description: "Amount of ambient smoke drifting up from the crater."},
			{Key: "ash", Label: "ash", Slot: SlotLever, Group: "vent", Type: KnobFloat, Min: 0.02, Max: 0.7, Step: 0.01, Default: 0.18,
				Description: "Amount of ember and ash flecks visible around the vent."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 1, Max: 50, Step: 1, Default: 18,
				Description: "Base lava hue. Lower values lean red; higher values lean gold-orange."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 18,
				Description: "Variation between crater glow, sparks, and horizon haze."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Overall lava and ember saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.03, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Minimum lightness used for dark sky and mountain tones."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Maximum lightness used for the hottest sparks and crater core."},
			{Key: "eruption_p", Label: "eruption", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "eruption",
				Description: "Per-tick chance of a more forceful eruption burst."},
			{Key: "smolder_p", Label: "smolder", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "smolder",
				Description: "Per-tick chance of the crater settling into a hotter smokier smolder."},
			{Key: "flare_p", Label: "flare", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "flare",
				Description: "Per-tick chance of a short sharp flare at the crater mouth."},
			{Key: "eruption_dur", Label: "eruption dur", Slot: SlotEventMod, Group: "eruption", Type: KnobInt, Min: 10, Max: 180, Step: 5, Default: 42,
				Description: "Duration of the higher-energy eruption window."},
			{Key: "eruption_mult", Label: "eruption x", Slot: SlotEventMod, Group: "eruption", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 2.2,
				Description: "Heat and spark multiplier applied during an eruption."},
			{Key: "smolder_dur", Label: "smolder dur", Slot: SlotEventMod, Group: "smolder", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the hotter smokier smolder interval."},
			{Key: "smolder_mult", Label: "smolder x", Slot: SlotEventMod, Group: "smolder", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.6,
				Description: "Smoke and glow multiplier applied while smolder is active."},
			{Key: "flare_dur", Label: "flare dur", Slot: SlotEventMod, Group: "flare", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 28,
				Description: "Duration of the brighter vent flare."},
			{Key: "flare_mult", Label: "flare x", Slot: SlotEventMod, Group: "flare", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Crater brightness multiplier applied during a flare."},
		},
	}
}

func TrainSchema() EffectSchema {
	return EffectSchema{
		Name: "train",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent building the distant cue lights and horizon before the first full pass."},
			{Key: "intro_cue", Label: "intro cue", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Starting fraction of the final headlight and horizon presence during the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent clearing the line after the final cars have passed."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 18,
				Description: "Extra quiet ticks for residual smoke, glow, and rail haze to fade."},
			{Key: "ending_clear", Label: "ending clear", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "How much dim horizon light remains near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0.58, Max: 0.86, Step: 0.01, Default: 0.74,
				Description: "Height of the far horizon behind the track."},
			{Key: "track_row", Label: "track row", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0.68, Max: 0.92, Step: 0.01, Default: 0.82,
				Description: "Vertical placement of the rails and train body."},
			{Key: "train_height", Label: "train height", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 3, Max: 8, Step: 0.5, Default: 4.5,
				Description: "Overall locomotive and carriage height in grid cells."},
			{Key: "car_count", Label: "car count", Slot: SlotLever, Group: "scene", Type: KnobInt, Min: 3, Max: 12, Step: 1, Default: 6,
				Description: "Baseline number of train cars trailing the locomotive."},
			{Key: "car_gap", Label: "car gap", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.5, Max: 3, Step: 0.1, Default: 1,
				Description: "Spacing between the locomotive and successive cars."},
			{Key: "speed", Label: "speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.5, Max: 2.5, Step: 0.05, Default: 1.05,
				Description: "Baseline traversal speed of the train across the scene."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.02, Max: 0.7, Step: 0.01, Default: 0.18,
				Description: "Amount of exhaust haze or steam trailing above the locomotive."},
			{Key: "headlight", Label: "headlight", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.02, Max: 0.8, Step: 0.01, Default: 0.24,
				Description: "Strength of the lead light glow during a pass."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 180, Max: 260, Step: 1, Default: 220,
				Description: "Base scene hue. Lower values warm toward dusk; higher values cool toward midnight blue."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 20,
				Description: "Variation between sky, headlight haze, and rail highlights."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.08, Max: 0.8, Step: 0.01, Default: 0.34,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.03, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Minimum lightness used for the ground, train body, and deep sky."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used for headlight bloom and window glints."},
			{Key: "pass_p", Label: "pass", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "pass",
				Description: "Per-tick chance of a standard train pass starting."},
			{Key: "express_p", Label: "express", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "express",
				Description: "Per-tick chance of a faster brighter express pass."},
			{Key: "quiet_gap_p", Label: "quiet gap", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-gap",
				Description: "Per-tick chance of a quieter interval before the next passing train."},
			{Key: "pass_dur", Label: "pass dur", Slot: SlotEventMod, Group: "pass", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 72,
				Description: "Duration of the standard pass window, including any lingering smoke."},
			{Key: "pass_mult", Label: "pass x", Slot: SlotEventMod, Group: "pass", Type: KnobFloat, Min: 1.0, Max: 2.5, Step: 0.05, Default: 1.15,
				Description: "Speed and presence multiplier applied during a standard pass."},
			{Key: "express_dur", Label: "express dur", Slot: SlotEventMod, Group: "express", Type: KnobInt, Min: 15, Max: 180, Step: 5, Default: 46,
				Description: "Duration of the faster express pass window."},
			{Key: "express_mult", Label: "express x", Slot: SlotEventMod, Group: "express", Type: KnobFloat, Min: 1.1, Max: 3, Step: 0.05, Default: 1.9,
				Description: "Speed and brightness multiplier applied during an express pass."},
			{Key: "quiet_gap_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 90,
				Description: "Duration of the quieter interval between passes."},
			{Key: "quiet_gap_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-gap", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Ambient haze and motion multiplier applied while quiet-gap is active."},
		},
	}
}

func MysteriousManSchema() EffectSchema {
	return EffectSchema{
		Name: "mysterious-man",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent revealing the ember and silhouette from near-total darkness."},
			{Key: "intro_reveal", Label: "intro reveal", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Starting fraction of the final ember and silhouette visibility during the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent dimming the ember and letting the last smoke dissolve."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks for the final smoke traces and silhouette falloff."},
			{Key: "ending_shadow", Label: "ending shadow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "How much silhouette and ember residue remains near the end of the outro."},
			{Key: "figure_x", Label: "figure x", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.2, Max: 0.7, Step: 0.01, Default: 0.38,
				Description: "Horizontal placement of the figure in the frame."},
			{Key: "figure_scale", Label: "scale", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.85, Max: 1.6, Step: 0.05, Default: 1,
				Description: "Overall figure size."},
			{Key: "lean", Label: "lean", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: -0.4, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "Forward or backward lean of the silhouette posture."},
			{Key: "contrast", Label: "contrast", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.14, Max: 0.7, Step: 0.01, Default: 0.24,
				Description: "How strongly the figure silhouette separates from the dark background."},
			{Key: "ember", Label: "ember", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.12, Max: 0.8, Step: 0.01, Default: 0.24,
				Description: "Baseline brightness of the cigarette ember."},
			{Key: "hold_angle", Label: "hold angle", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: -0.8, Max: 0.8, Step: 0.02, Default: 0.22,
				Description: "Angle of the hand and cigarette near the face."},
			{Key: "smoke", Label: "smoke", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.08, Max: 0.6, Step: 0.01, Default: 0.16,
				Description: "Amount of smoke visible around the ember and face."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.2, Max: 1.6, Step: 0.02, Default: 0.72,
				Description: "Vertical lift applied to the smoke wisps."},
			{Key: "drift", Label: "drift", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.01, Default: 0.08,
				Description: "Sideways drift of the smoke. Positive drifts right, negative drifts left."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 180, Max: 260, Step: 1, Default: 216,
				Description: "Base scene hue. Lower values warm toward sodium light; higher values cool toward blue noir."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0, Max: 40, Step: 1, Default: 18,
				Description: "Variation between shadow tones, smoke tint, and ember-adjacent light."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.04, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.02, Max: 0.3, Step: 0.01, Default: 0.04,
				Description: "Minimum lightness used for the darkest background and coat tones."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.3, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Maximum lightness used for ember, lighter flick, and the brightest smoke edges."},
			{Key: "inhale_p", Label: "inhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "inhale",
				Description: "Per-tick chance of the ember brightening for a deeper inhale."},
			{Key: "exhale_p", Label: "exhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "exhale",
				Description: "Per-tick chance of a visible smoke plume leaving the figure."},
			{Key: "ash_fall_p", Label: "ash fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "ash-fall",
				Description: "Per-tick chance of a small ash fleck breaking away from the cigarette."},
			{Key: "lighter_flick_p", Label: "lighter flick", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lighter-flick",
				Description: "Per-tick chance of a brief brighter warm flash near the face and hand."},
			{Key: "inhale_dur", Label: "inhale dur", Slot: SlotEventMod, Group: "inhale", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 34,
				Description: "Duration of the inhale emphasis window."},
			{Key: "inhale_mult", Label: "inhale x", Slot: SlotEventMod, Group: "inhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Ember multiplier applied during an inhale."},
			{Key: "exhale_dur", Label: "exhale dur", Slot: SlotEventMod, Group: "exhale", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 46,
				Description: "Duration of the exhale plume window."},
			{Key: "exhale_mult", Label: "exhale x", Slot: SlotEventMod, Group: "exhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.65,
				Description: "Smoke plume multiplier applied during an exhale."},
			{Key: "ash_fall_dur", Label: "ash dur", Slot: SlotEventMod, Group: "ash-fall", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 24,
				Description: "Duration of the falling ash fleck event."},
			{Key: "ash_fall_mult", Label: "ash x", Slot: SlotEventMod, Group: "ash-fall", Type: KnobFloat, Min: 1.0, Max: 3, Step: 0.05, Default: 1.4,
				Description: "Intensity multiplier applied to the ash-fall event."},
			{Key: "lighter_flick_dur", Label: "flick dur", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobInt, Min: 10, Max: 100, Step: 5, Default: 20,
				Description: "Duration of the lighter flick highlight."},
			{Key: "lighter_flick_mult", Label: "flick x", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 2.25,
				Description: "Brightness multiplier applied during the lighter flick."},
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
	case "beach":
		return cloneProceduralConfig(beachDefaults)
	case "campfire":
		return cloneProceduralConfig(campfireDefaults)
	case "windmill":
		return cloneProceduralConfig(windmillDefaults)
	case "lighthouse":
		return cloneProceduralConfig(lighthouseDefaults)
	case "rowboat":
		return cloneProceduralConfig(rowboatDefaults)
	case "underwater":
		return cloneProceduralConfig(underwaterDefaults)
	case "volcano":
		return cloneProceduralConfig(volcanoDefaults)
	case "train":
		return cloneProceduralConfig(trainDefaults)
	case "mysterious-man":
		return cloneProceduralConfig(mysteriousManDefaults)
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
	case "beach":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = beachDefaults["intro_dur"]
		}
		out["intro_tide"] = clamp01(out["intro_tide"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = beachDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_wet"] = clamp01(out["ending_wet"])
		if out["shoreline"] <= 0 {
			out["shoreline"] = beachDefaults["shoreline"]
		}
		if out["tide_amp"] <= 0 {
			out["tide_amp"] = beachDefaults["tide_amp"]
		}
		if out["wave_amp"] <= 0 {
			out["wave_amp"] = beachDefaults["wave_amp"]
		}
		if out["wave_freq"] <= 0 {
			out["wave_freq"] = beachDefaults["wave_freq"]
		}
		if out["speed"] <= 0 {
			out["speed"] = beachDefaults["speed"]
		}
		if out["foam"] <= 0 {
			out["foam"] = beachDefaults["foam"]
		}
		if out["shimmer"] <= 0 {
			out["shimmer"] = beachDefaults["shimmer"]
		}
		if out["hue"] == 0 {
			out["hue"] = beachDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = beachDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = beachDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = beachDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["high_tide_dur"] <= 0 {
			out["high_tide_dur"] = beachDefaults["high_tide_dur"]
		}
		if out["high_tide_push"] <= 0 {
			out["high_tide_push"] = beachDefaults["high_tide_push"]
		}
		if out["low_tide_dur"] <= 0 {
			out["low_tide_dur"] = beachDefaults["low_tide_dur"]
		}
		if out["low_tide_pull"] <= 0 {
			out["low_tide_pull"] = beachDefaults["low_tide_pull"]
		}
		if out["foam_burst_dur"] <= 0 {
			out["foam_burst_dur"] = beachDefaults["foam_burst_dur"]
		}
		if out["foam_burst_mult"] <= 0 {
			out["foam_burst_mult"] = beachDefaults["foam_burst_mult"]
		}
	case "campfire":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = campfireDefaults["intro_dur"]
		}
		out["intro_glow"] = clamp01(out["intro_glow"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = campfireDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_glow"] = clamp01(out["ending_glow"])
		if out["flame_height"] <= 0 {
			out["flame_height"] = campfireDefaults["flame_height"]
		}
		if out["flame_width"] <= 0 {
			out["flame_width"] = campfireDefaults["flame_width"]
		}
		if out["flame_speed"] <= 0 {
			out["flame_speed"] = campfireDefaults["flame_speed"]
		}
		if out["flicker"] <= 0 {
			out["flicker"] = campfireDefaults["flicker"]
		}
		if out["ember_rate"] <= 0 {
			out["ember_rate"] = campfireDefaults["ember_rate"]
		}
		if out["ember_speed"] <= 0 {
			out["ember_speed"] = campfireDefaults["ember_speed"]
		}
		if out["glow"] <= 0 {
			out["glow"] = campfireDefaults["glow"]
		}
		if out["hue"] == 0 {
			out["hue"] = campfireDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = campfireDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = campfireDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = campfireDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["crackle_dur"] <= 0 {
			out["crackle_dur"] = campfireDefaults["crackle_dur"]
		}
		if out["crackle_mult"] <= 0 {
			out["crackle_mult"] = campfireDefaults["crackle_mult"]
		}
		if out["lull_dur"] <= 0 {
			out["lull_dur"] = campfireDefaults["lull_dur"]
		}
		if out["lull_mult"] <= 0 {
			out["lull_mult"] = campfireDefaults["lull_mult"]
		}
	case "windmill":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = windmillDefaults["intro_dur"]
		}
		out["intro_turn"] = clamp01(out["intro_turn"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = windmillDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_turn"] = clamp01(out["ending_turn"])
		if out["turn_speed"] <= 0 {
			out["turn_speed"] = windmillDefaults["turn_speed"]
		}
		if out["blade_len"] <= 0 {
			out["blade_len"] = windmillDefaults["blade_len"]
		}
		if out["blade_width"] <= 0 {
			out["blade_width"] = windmillDefaults["blade_width"]
		}
		if out["tower_height"] <= 0 {
			out["tower_height"] = windmillDefaults["tower_height"]
		}
		if out["tower_width"] <= 0 {
			out["tower_width"] = windmillDefaults["tower_width"]
		}
		if out["horizon"] <= 0 {
			out["horizon"] = windmillDefaults["horizon"]
		}
		if out["glow"] <= 0 {
			out["glow"] = windmillDefaults["glow"]
		}
		if out["hue"] == 0 {
			out["hue"] = windmillDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = windmillDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = windmillDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = windmillDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["gust_dur"] <= 0 {
			out["gust_dur"] = windmillDefaults["gust_dur"]
		}
		if out["gust_mult"] <= 0 {
			out["gust_mult"] = windmillDefaults["gust_mult"]
		}
		if out["lull_dur"] <= 0 {
			out["lull_dur"] = windmillDefaults["lull_dur"]
		}
		if out["lull_mult"] <= 0 {
			out["lull_mult"] = windmillDefaults["lull_mult"]
		}
	case "lighthouse":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = lighthouseDefaults["intro_dur"]
		}
		out["intro_beam"] = clamp01(out["intro_beam"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = lighthouseDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_beam"] = clamp01(out["ending_beam"])
		if out["sweep_speed"] <= 0 {
			out["sweep_speed"] = lighthouseDefaults["sweep_speed"]
		}
		if out["beam_width"] <= 0 {
			out["beam_width"] = lighthouseDefaults["beam_width"]
		}
		if out["beam_softness"] <= 0 {
			out["beam_softness"] = lighthouseDefaults["beam_softness"]
		}
		if out["tower_height"] <= 0 {
			out["tower_height"] = lighthouseDefaults["tower_height"]
		}
		if out["tower_width"] <= 0 {
			out["tower_width"] = lighthouseDefaults["tower_width"]
		}
		if out["horizon"] <= 0 {
			out["horizon"] = lighthouseDefaults["horizon"]
		}
		if out["haze"] <= 0 {
			out["haze"] = lighthouseDefaults["haze"]
		}
		if out["glow"] <= 0 {
			out["glow"] = lighthouseDefaults["glow"]
		}
		if out["hue"] == 0 {
			out["hue"] = lighthouseDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = lighthouseDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = lighthouseDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = lighthouseDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["bright_pass_dur"] <= 0 {
			out["bright_pass_dur"] = lighthouseDefaults["bright_pass_dur"]
		}
		if out["bright_pass_mult"] <= 0 {
			out["bright_pass_mult"] = lighthouseDefaults["bright_pass_mult"]
		}
		if out["fog_thicken_dur"] <= 0 {
			out["fog_thicken_dur"] = lighthouseDefaults["fog_thicken_dur"]
		}
		if out["fog_thicken_mult"] <= 0 {
			out["fog_thicken_mult"] = lighthouseDefaults["fog_thicken_mult"]
		}
		if out["calm_dur"] <= 0 {
			out["calm_dur"] = lighthouseDefaults["calm_dur"]
		}
		if out["calm_mult"] <= 0 {
			out["calm_mult"] = lighthouseDefaults["calm_mult"]
		}
	case "rowboat":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = rowboatDefaults["intro_dur"]
		}
		out["intro_drift"] = clamp01(out["intro_drift"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = rowboatDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_ripple"] = clamp01(out["ending_ripple"])
		if out["waterline"] <= 0 {
			out["waterline"] = rowboatDefaults["waterline"]
		}
		if out["drift_speed"] <= 0 {
			out["drift_speed"] = rowboatDefaults["drift_speed"]
		}
		if out["bob_amp"] <= 0 {
			out["bob_amp"] = rowboatDefaults["bob_amp"]
		}
		if out["wave_amp"] <= 0 {
			out["wave_amp"] = rowboatDefaults["wave_amp"]
		}
		if out["wave_freq"] <= 0 {
			out["wave_freq"] = rowboatDefaults["wave_freq"]
		}
		if out["ripple"] <= 0 {
			out["ripple"] = rowboatDefaults["ripple"]
		}
		if out["reflection"] <= 0 {
			out["reflection"] = rowboatDefaults["reflection"]
		}
		if out["boat_len"] <= 0 {
			out["boat_len"] = rowboatDefaults["boat_len"]
		}
		if out["boat_height"] <= 0 {
			out["boat_height"] = rowboatDefaults["boat_height"]
		}
		if out["hue"] == 0 {
			out["hue"] = rowboatDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = rowboatDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = rowboatDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = rowboatDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["wake_dur"] <= 0 {
			out["wake_dur"] = rowboatDefaults["wake_dur"]
		}
		if out["wake_mult"] <= 0 {
			out["wake_mult"] = rowboatDefaults["wake_mult"]
		}
		if out["drift_dur"] <= 0 {
			out["drift_dur"] = rowboatDefaults["drift_dur"]
		}
		if out["drift_push"] <= 0 {
			out["drift_push"] = rowboatDefaults["drift_push"]
		}
		if out["calm_dur"] <= 0 {
			out["calm_dur"] = rowboatDefaults["calm_dur"]
		}
		if out["calm_mult"] <= 0 {
			out["calm_mult"] = rowboatDefaults["calm_mult"]
		}
	case "underwater":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = underwaterDefaults["intro_dur"]
		}
		out["intro_reveal"] = clamp01(out["intro_reveal"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = underwaterDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_murk"] = clamp01(out["ending_murk"])
		if out["density"] <= 0 {
			out["density"] = underwaterDefaults["density"]
		}
		if out["rise_speed"] <= 0 {
			out["rise_speed"] = underwaterDefaults["rise_speed"]
		}
		if out["sway"] <= 0 {
			out["sway"] = underwaterDefaults["sway"]
		}
		if out["weed_height"] <= 0 {
			out["weed_height"] = underwaterDefaults["weed_height"]
		}
		if out["weed_count"] < 1 {
			out["weed_count"] = underwaterDefaults["weed_count"]
		}
		if out["caustics"] <= 0 {
			out["caustics"] = underwaterDefaults["caustics"]
		}
		if out["depth"] <= 0 {
			out["depth"] = underwaterDefaults["depth"]
		}
		if out["hue"] == 0 {
			out["hue"] = underwaterDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = underwaterDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = underwaterDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = underwaterDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["bubble_burst_dur"] <= 0 {
			out["bubble_burst_dur"] = underwaterDefaults["bubble_burst_dur"]
		}
		if out["bubble_burst_mult"] <= 0 {
			out["bubble_burst_mult"] = underwaterDefaults["bubble_burst_mult"]
		}
		if out["current_shift_dur"] <= 0 {
			out["current_shift_dur"] = underwaterDefaults["current_shift_dur"]
		}
		if out["current_shift_push"] <= 0 {
			out["current_shift_push"] = underwaterDefaults["current_shift_push"]
		}
		if out["calm_dur"] <= 0 {
			out["calm_dur"] = underwaterDefaults["calm_dur"]
		}
		if out["calm_mult"] <= 0 {
			out["calm_mult"] = underwaterDefaults["calm_mult"]
		}
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
		out["ending_embers"] = clamp01(out["ending_embers"])
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
		if out["glow"] <= 0 {
			out["glow"] = volcanoDefaults["glow"]
		}
		if out["smoke"] <= 0 {
			out["smoke"] = volcanoDefaults["smoke"]
		}
		if out["ash"] <= 0 {
			out["ash"] = volcanoDefaults["ash"]
		}
		if out["hue"] == 0 {
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
	case "train":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = trainDefaults["intro_dur"]
		}
		out["intro_cue"] = clamp01(out["intro_cue"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = trainDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_clear"] = clamp01(out["ending_clear"])
		if out["horizon"] <= 0 {
			out["horizon"] = trainDefaults["horizon"]
		}
		if out["track_row"] <= 0 {
			out["track_row"] = trainDefaults["track_row"]
		}
		if out["train_height"] <= 0 {
			out["train_height"] = trainDefaults["train_height"]
		}
		if out["car_count"] < 3 {
			out["car_count"] = trainDefaults["car_count"]
		}
		if out["car_gap"] <= 0 {
			out["car_gap"] = trainDefaults["car_gap"]
		}
		if out["speed"] <= 0 {
			out["speed"] = trainDefaults["speed"]
		}
		if out["smoke"] <= 0 {
			out["smoke"] = trainDefaults["smoke"]
		}
		if out["headlight"] <= 0 {
			out["headlight"] = trainDefaults["headlight"]
		}
		if out["hue"] <= 0 {
			out["hue"] = trainDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = trainDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = trainDefaults["lmin"]
		}
		if out["lmax"] <= 0 {
			out["lmax"] = trainDefaults["lmax"]
		}
		if out["lmax"] < out["lmin"] {
			out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
		}
		if out["pass_dur"] <= 0 {
			out["pass_dur"] = trainDefaults["pass_dur"]
		}
		if out["pass_mult"] <= 0 {
			out["pass_mult"] = trainDefaults["pass_mult"]
		}
		if out["express_dur"] <= 0 {
			out["express_dur"] = trainDefaults["express_dur"]
		}
		if out["express_mult"] <= 0 {
			out["express_mult"] = trainDefaults["express_mult"]
		}
		if out["quiet_gap_dur"] <= 0 {
			out["quiet_gap_dur"] = trainDefaults["quiet_gap_dur"]
		}
		if out["quiet_gap_mult"] <= 0 {
			out["quiet_gap_mult"] = trainDefaults["quiet_gap_mult"]
		}
	case "mysterious-man":
		if out["intro_dur"] <= 0 {
			out["intro_dur"] = mysteriousManDefaults["intro_dur"]
		}
		out["intro_reveal"] = clamp01(out["intro_reveal"])
		if out["ending_dur"] <= 0 {
			out["ending_dur"] = mysteriousManDefaults["ending_dur"]
		}
		if out["ending_linger"] < 0 {
			out["ending_linger"] = 0
		}
		out["ending_shadow"] = clamp01(out["ending_shadow"])
		if out["figure_x"] <= 0 {
			out["figure_x"] = mysteriousManDefaults["figure_x"]
		}
		if out["figure_scale"] <= 0 {
			out["figure_scale"] = mysteriousManDefaults["figure_scale"]
		}
		if out["contrast"] <= 0 {
			out["contrast"] = mysteriousManDefaults["contrast"]
		}
		if out["ember"] <= 0 {
			out["ember"] = mysteriousManDefaults["ember"]
		}
		if out["smoke"] <= 0 {
			out["smoke"] = mysteriousManDefaults["smoke"]
		}
		if out["rise_speed"] <= 0 {
			out["rise_speed"] = mysteriousManDefaults["rise_speed"]
		}
		if out["hue"] <= 0 {
			out["hue"] = mysteriousManDefaults["hue"]
		}
		if out["hue_sp"] < 0 {
			out["hue_sp"] = 0
		}
		if out["sat"] <= 0 {
			out["sat"] = mysteriousManDefaults["sat"]
		}
		if out["lmin"] <= 0 {
			out["lmin"] = mysteriousManDefaults["lmin"]
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
		if out["exhale_mult"] <= 0 {
			out["exhale_mult"] = mysteriousManDefaults["exhale_mult"]
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
	case "beach":
		switch name {
		case "high-tide":
			p.startBeachHighTideLocked("triggered")
		case "low-tide":
			p.startBeachLowTideLocked("triggered")
		case "foam-burst":
			p.startBeachFoamBurstLocked("triggered")
		case "intro":
			p.startBeachIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, tide=%.2f)", p.timers["intro"], p.cfg["intro_tide"]))
		case "ending":
			p.startBeachEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "campfire":
		switch name {
		case "crackle":
			p.startCampfireCrackleLocked("triggered")
		case "lull":
			p.startCampfireLullLocked("triggered")
		case "intro":
			p.startCampfireIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", p.timers["intro"], p.cfg["intro_glow"]))
		case "ending":
			p.startCampfireEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "windmill":
		switch name {
		case "gust":
			p.startWindmillGustLocked("triggered")
		case "lull":
			p.startWindmillLullLocked("triggered")
		case "intro":
			p.startWindmillIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, turn=%.2f)", p.timers["intro"], p.cfg["intro_turn"]))
		case "ending":
			p.startWindmillEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "lighthouse":
		switch name {
		case "bright-pass":
			p.startLighthouseBrightPassLocked("triggered")
		case "fog-thicken":
			p.startLighthouseFogThickenLocked("triggered")
		case "calm":
			p.startLighthouseCalmLocked("triggered")
		case "intro":
			p.startLighthouseIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, beam=%.2f)", p.timers["intro"], p.cfg["intro_beam"]))
		case "ending":
			p.startLighthouseEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "rowboat":
		switch name {
		case "wake":
			p.startRowboatWakeLocked("triggered")
		case "drift":
			p.startRowboatDriftLocked("triggered")
		case "calm":
			p.startRowboatCalmLocked("triggered")
		case "intro":
			p.startRowboatIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, drift=%.2f)", p.timers["intro"], p.cfg["intro_drift"]))
		case "ending":
			p.startRowboatEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
	case "underwater":
		switch name {
		case "bubble-burst":
			p.startUnderwaterBubbleBurstLocked("triggered")
		case "current-shift":
			p.startUnderwaterCurrentShiftLocked("triggered")
		case "calm":
			p.startUnderwaterCalmLocked("triggered")
		case "intro":
			p.startUnderwaterIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, reveal=%.2f)", p.timers["intro"], p.cfg["intro_reveal"]))
		case "ending":
			p.startUnderwaterEndingLocked()
			p.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", p.intCfg("ending_dur"), p.intCfg("ending_linger")))
		default:
			return false
		}
		return true
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
	case "train":
		switch name {
		case "pass":
			p.startTrainPassLocked("triggered")
		case "express":
			p.startTrainExpressLocked("triggered")
		case "quiet-gap":
			p.startTrainQuietGapLocked("triggered")
		case "intro":
			p.startTrainIntroLocked()
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, cue=%.2f)", p.timers["intro"], p.cfg["intro_cue"]))
		case "ending":
			p.startTrainEndingLocked()
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
			p.appendLog("intro", fmt.Sprintf("started (dur=%d, reveal=%.2f)", p.timers["intro"], p.cfg["intro_reveal"]))
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
	case "beach":
		p.stepBeachLocked()
	case "campfire":
		p.stepCampfireLocked()
	case "windmill":
		p.stepWindmillLocked()
	case "lighthouse":
		p.stepLighthouseLocked()
	case "rowboat":
		p.stepRowboatLocked()
	case "underwater":
		p.stepUnderwaterLocked()
	case "volcano":
		p.stepVolcanoLocked()
	case "train":
		p.stepTrainLocked()
	case "mysterious-man":
		p.stepMysteriousManLocked()
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

func (p *Procedural) startBeachHighTideLocked(verb string) {
	p.timers["high-tide"] = jitterInt(p.rng, p.intCfg("high_tide_dur"), 0.3)
	p.timers["low-tide"] = 0
	p.values["tide_bias"] = p.cfg["high_tide_push"] * (0.65 + p.rng.Float64()*0.55)
	p.appendLog("high-tide", fmt.Sprintf("%s (dur=%d, bias=+%.2f)", verb, p.timers["high-tide"], p.values["tide_bias"]))
}

func (p *Procedural) startBeachLowTideLocked(verb string) {
	p.timers["low-tide"] = jitterInt(p.rng, p.intCfg("low_tide_dur"), 0.3)
	p.timers["high-tide"] = 0
	p.values["tide_bias"] = -p.cfg["low_tide_pull"] * (0.65 + p.rng.Float64()*0.55)
	p.appendLog("low-tide", fmt.Sprintf("%s (dur=%d, bias=%.2f)", verb, p.timers["low-tide"], p.values["tide_bias"]))
}

func (p *Procedural) startBeachFoamBurstLocked(verb string) {
	p.timers["foam-burst"] = jitterInt(p.rng, p.intCfg("foam_burst_dur"), 0.3)
	p.values["foam_gain"] = p.cfg["foam_burst_mult"] * (0.85 + p.rng.Float64()*0.35)
	p.appendLog("foam-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["foam-burst"], p.values["foam_gain"]))
}

func (p *Procedural) startBeachIntroLocked() {
	p.timers["high-tide"] = 0
	p.timers["low-tide"] = 0
	p.timers["foam-burst"] = 0
	p.timers["ending"] = 0
	p.values["tide_bias"] = 0
	p.values["foam_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startBeachEndingLocked() {
	p.timers["intro"] = 0
	p.timers["high-tide"] = 0
	p.timers["low-tide"] = 0
	p.timers["foam-burst"] = 0
	p.values["tide_bias"] = 0
	p.values["foam_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepBeachLocked() {
	if p.timers["high-tide"] <= 0 && p.timers["low-tide"] <= 0 {
		p.values["tide_bias"] = 0
	}
	if p.timers["foam-burst"] <= 0 {
		p.values["foam_gain"] = 1
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
	if p.timers["high-tide"] <= 0 && p.timers["low-tide"] <= 0 && p.cfg["high_tide_p"] > 0 && p.rng.Float64() < p.cfg["high_tide_p"] {
		p.startBeachHighTideLocked("started")
	}
	if p.timers["low-tide"] <= 0 && p.timers["high-tide"] <= 0 && p.cfg["low_tide_p"] > 0 && p.rng.Float64() < p.cfg["low_tide_p"] {
		p.startBeachLowTideLocked("started")
	}
	if p.timers["foam-burst"] <= 0 && p.cfg["foam_burst_p"] > 0 && p.rng.Float64() < p.cfg["foam_burst_p"] {
		p.startBeachFoamBurstLocked("started")
	}
}

func (p *Procedural) startCampfireCrackleLocked(verb string) {
	p.timers["crackle"] = jitterInt(p.rng, p.intCfg("crackle_dur"), 0.3)
	p.values["crackle_gain"] = p.cfg["crackle_mult"] * (0.75 + p.rng.Float64()*0.50)
	p.appendLog("crackle", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["crackle"], p.values["crackle_gain"]))
}

func (p *Procedural) startCampfireLullLocked(verb string) {
	p.timers["lull"] = jitterInt(p.rng, p.intCfg("lull_dur"), 0.3)
	p.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lull"], p.cfg["lull_mult"]))
}

func (p *Procedural) startCampfireIntroLocked() {
	p.timers["crackle"] = 0
	p.timers["lull"] = 0
	p.timers["ending"] = 0
	p.values["crackle_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startCampfireEndingLocked() {
	p.timers["intro"] = 0
	p.timers["crackle"] = 0
	p.timers["lull"] = 0
	p.values["crackle_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepCampfireLocked() {
	if p.timers["crackle"] <= 0 {
		p.values["crackle_gain"] = 1
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
	if p.timers["crackle"] <= 0 && p.cfg["crackle_p"] > 0 && p.rng.Float64() < p.cfg["crackle_p"] {
		p.startCampfireCrackleLocked("started")
	}
	if p.timers["lull"] <= 0 && p.cfg["lull_p"] > 0 && p.rng.Float64() < p.cfg["lull_p"] {
		p.startCampfireLullLocked("started")
	}
}

func (p *Procedural) startWindmillGustLocked(verb string) {
	p.timers["gust"] = jitterInt(p.rng, p.intCfg("gust_dur"), 0.3)
	p.values["gust_gain"] = p.cfg["gust_mult"] * (0.75 + p.rng.Float64()*0.45)
	p.appendLog("gust", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["gust"], p.values["gust_gain"]))
}

func (p *Procedural) startWindmillLullLocked(verb string) {
	p.timers["lull"] = jitterInt(p.rng, p.intCfg("lull_dur"), 0.3)
	p.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lull"], p.cfg["lull_mult"]))
}

func (p *Procedural) startWindmillIntroLocked() {
	p.timers["gust"] = 0
	p.timers["lull"] = 0
	p.timers["ending"] = 0
	p.values["gust_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startWindmillEndingLocked() {
	p.timers["intro"] = 0
	p.timers["gust"] = 0
	p.timers["lull"] = 0
	p.values["gust_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepWindmillLocked() {
	if p.timers["gust"] <= 0 {
		p.values["gust_gain"] = 1
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
		p.startWindmillGustLocked("started")
	}
	if p.timers["lull"] <= 0 && p.cfg["lull_p"] > 0 && p.rng.Float64() < p.cfg["lull_p"] {
		p.startWindmillLullLocked("started")
	}
}

func (p *Procedural) startLighthouseBrightPassLocked(verb string) {
	p.timers["bright-pass"] = jitterInt(p.rng, p.intCfg("bright_pass_dur"), 0.3)
	p.values["bright_gain"] = p.cfg["bright_pass_mult"] * (0.8 + p.rng.Float64()*0.4)
	p.appendLog("bright-pass", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["bright-pass"], p.values["bright_gain"]))
}

func (p *Procedural) startLighthouseFogThickenLocked(verb string) {
	p.timers["fog-thicken"] = jitterInt(p.rng, p.intCfg("fog_thicken_dur"), 0.3)
	p.timers["calm"] = 0
	p.values["fog_gain"] = p.cfg["fog_thicken_mult"] * (0.8 + p.rng.Float64()*0.45)
	p.appendLog("fog-thicken", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["fog-thicken"], p.values["fog_gain"]))
}

func (p *Procedural) startLighthouseCalmLocked(verb string) {
	p.timers["calm"] = jitterInt(p.rng, p.intCfg("calm_dur"), 0.3)
	p.timers["fog-thicken"] = 0
	p.values["fog_gain"] = 1
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["calm"], p.cfg["calm_mult"]))
}

func (p *Procedural) startLighthouseIntroLocked() {
	p.timers["bright-pass"] = 0
	p.timers["fog-thicken"] = 0
	p.timers["calm"] = 0
	p.timers["ending"] = 0
	p.values["bright_gain"] = 1
	p.values["fog_gain"] = 1
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startLighthouseEndingLocked() {
	p.timers["intro"] = 0
	p.timers["bright-pass"] = 0
	p.timers["fog-thicken"] = 0
	p.timers["calm"] = 0
	p.values["bright_gain"] = 1
	p.values["fog_gain"] = 1
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepLighthouseLocked() {
	if p.timers["bright-pass"] <= 0 {
		p.values["bright_gain"] = 1
	}
	if p.timers["fog-thicken"] <= 0 {
		p.values["fog_gain"] = 1
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
	if p.timers["bright-pass"] <= 0 && p.cfg["bright_pass_p"] > 0 && p.rng.Float64() < p.cfg["bright_pass_p"] {
		p.startLighthouseBrightPassLocked("started")
	}
	if p.timers["fog-thicken"] <= 0 && p.timers["calm"] <= 0 && p.cfg["fog_thicken_p"] > 0 && p.rng.Float64() < p.cfg["fog_thicken_p"] {
		p.startLighthouseFogThickenLocked("started")
	}
	if p.timers["calm"] <= 0 && p.timers["fog-thicken"] <= 0 && p.cfg["calm_p"] > 0 && p.rng.Float64() < p.cfg["calm_p"] {
		p.startLighthouseCalmLocked("started")
	}
}

func (p *Procedural) startRowboatWakeLocked(verb string) {
	p.timers["wake"] = jitterInt(p.rng, p.intCfg("wake_dur"), 0.3)
	p.values["wake_gain"] = p.cfg["wake_mult"] * (0.8 + p.rng.Float64()*0.45)
	p.appendLog("wake", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["wake"], p.values["wake_gain"]))
}

func (p *Procedural) startRowboatDriftLocked(verb string) {
	p.timers["drift"] = jitterInt(p.rng, p.intCfg("drift_dur"), 0.3)
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["drift_push"] = sign * p.cfg["drift_push"] * (0.65 + p.rng.Float64()*0.55)
	p.appendLog("drift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["drift"], p.values["drift_push"]))
}

func (p *Procedural) startRowboatCalmLocked(verb string) {
	p.timers["calm"] = jitterInt(p.rng, p.intCfg("calm_dur"), 0.3)
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["calm"], p.cfg["calm_mult"]))
}

func (p *Procedural) startRowboatIntroLocked() {
	p.timers["wake"] = 0
	p.timers["drift"] = 0
	p.timers["calm"] = 0
	p.timers["ending"] = 0
	p.values["wake_gain"] = 1
	p.values["drift_push"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startRowboatEndingLocked() {
	p.timers["intro"] = 0
	p.timers["wake"] = 0
	p.timers["drift"] = 0
	p.timers["calm"] = 0
	p.values["wake_gain"] = 1
	p.values["drift_push"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepRowboatLocked() {
	if p.timers["wake"] <= 0 {
		p.values["wake_gain"] = 1
	}
	if p.timers["drift"] <= 0 {
		p.values["drift_push"] = 0
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
	if p.timers["wake"] <= 0 && p.cfg["wake_p"] > 0 && p.rng.Float64() < p.cfg["wake_p"] {
		p.startRowboatWakeLocked("started")
	}
	if p.timers["drift"] <= 0 && p.cfg["drift_p"] > 0 && p.rng.Float64() < p.cfg["drift_p"] {
		p.startRowboatDriftLocked("started")
	}
	if p.timers["calm"] <= 0 && p.cfg["calm_p"] > 0 && p.rng.Float64() < p.cfg["calm_p"] {
		p.startRowboatCalmLocked("started")
	}
}

func (p *Procedural) startUnderwaterBubbleBurstLocked(verb string) {
	p.timers["bubble-burst"] = jitterInt(p.rng, p.intCfg("bubble_burst_dur"), 0.3)
	p.values["bubble_gain"] = p.cfg["bubble_burst_mult"] * (0.8 + p.rng.Float64()*0.45)
	p.appendLog("bubble-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["bubble-burst"], p.values["bubble_gain"]))
}

func (p *Procedural) startUnderwaterCurrentShiftLocked(verb string) {
	p.timers["current-shift"] = jitterInt(p.rng, p.intCfg("current_shift_dur"), 0.3)
	p.timers["calm"] = 0
	sign := 1.0
	if p.rng.Float64() < 0.5 {
		sign = -1
	}
	p.values["current_push"] = sign * p.cfg["current_shift_push"] * (0.55 + p.rng.Float64()*0.55)
	p.appendLog("current-shift", fmt.Sprintf("%s (dur=%d, push=%+.2f)", verb, p.timers["current-shift"], p.values["current_push"]))
}

func (p *Procedural) startUnderwaterCalmLocked(verb string) {
	p.timers["calm"] = jitterInt(p.rng, p.intCfg("calm_dur"), 0.3)
	p.timers["current-shift"] = 0
	p.values["current_push"] = 0
	p.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["calm"], p.cfg["calm_mult"]))
}

func (p *Procedural) startUnderwaterIntroLocked() {
	p.timers["bubble-burst"] = 0
	p.timers["current-shift"] = 0
	p.timers["calm"] = 0
	p.timers["ending"] = 0
	p.values["bubble_gain"] = 1
	p.values["current_push"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startUnderwaterEndingLocked() {
	p.timers["intro"] = 0
	p.timers["bubble-burst"] = 0
	p.timers["current-shift"] = 0
	p.timers["calm"] = 0
	p.values["bubble_gain"] = 1
	p.values["current_push"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepUnderwaterLocked() {
	if p.timers["bubble-burst"] <= 0 {
		p.values["bubble_gain"] = 1
	}
	if p.timers["current-shift"] <= 0 {
		p.values["current_push"] = 0
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
	if p.timers["bubble-burst"] <= 0 && p.cfg["bubble_burst_p"] > 0 && p.rng.Float64() < p.cfg["bubble_burst_p"] {
		p.startUnderwaterBubbleBurstLocked("started")
	}
	if p.timers["current-shift"] <= 0 && p.timers["calm"] <= 0 && p.cfg["current_shift_p"] > 0 && p.rng.Float64() < p.cfg["current_shift_p"] {
		p.startUnderwaterCurrentShiftLocked("started")
	}
	if p.timers["calm"] <= 0 && p.timers["current-shift"] <= 0 && p.cfg["calm_p"] > 0 && p.rng.Float64() < p.cfg["calm_p"] {
		p.startUnderwaterCalmLocked("started")
	}
}

func (p *Procedural) startVolcanoEruptionLocked(verb string) {
	p.timers["eruption"] = jitterInt(p.rng, p.intCfg("eruption_dur"), 0.3)
	p.values["eruption_gain"] = p.cfg["eruption_mult"] * (0.8 + p.rng.Float64()*0.45)
	p.values["eruption_total"] = float64(p.timers["eruption"])
	p.values["eruption_seed"] = p.rng.Float64() * 1000
	p.values["eruption_dir"] = 1
	if p.rng.Float64() < 0.5 {
		p.values["eruption_dir"] = -1
	}
	p.appendLog("eruption", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["eruption"], p.values["eruption_gain"]))
}

func (p *Procedural) startVolcanoSmolderLocked(verb string) {
	p.timers["smolder"] = jitterInt(p.rng, p.intCfg("smolder_dur"), 0.3)
	p.values["smolder_gain"] = p.cfg["smolder_mult"] * (0.8 + p.rng.Float64()*0.4)
	p.appendLog("smolder", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["smolder"], p.values["smolder_gain"]))
}

func (p *Procedural) startVolcanoFlareLocked(verb string) {
	p.timers["flare"] = jitterInt(p.rng, p.intCfg("flare_dur"), 0.3)
	p.values["flare_gain"] = p.cfg["flare_mult"] * (0.85 + p.rng.Float64()*0.35)
	p.appendLog("flare", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["flare"], p.values["flare_gain"]))
}

func (p *Procedural) startVolcanoIntroLocked() {
	p.timers["eruption"] = 0
	p.timers["smolder"] = 0
	p.timers["flare"] = 0
	p.timers["ending"] = 0
	p.values["eruption_gain"] = 1
	p.values["smolder_gain"] = 1
	p.values["flare_gain"] = 1
	p.values["eruption_total"] = 0
	p.values["eruption_seed"] = 0
	p.values["eruption_dir"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startVolcanoEndingLocked() {
	p.timers["intro"] = 0
	p.timers["eruption"] = 0
	p.timers["smolder"] = 0
	p.timers["flare"] = 0
	p.values["eruption_gain"] = 1
	p.values["smolder_gain"] = 1
	p.values["flare_gain"] = 1
	p.values["eruption_total"] = 0
	p.values["eruption_seed"] = 0
	p.values["eruption_dir"] = 0
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
		p.values["eruption_total"] = 0
		p.values["eruption_seed"] = 0
		p.values["eruption_dir"] = 0
	}
	if p.timers["smolder"] <= 0 {
		p.values["smolder_gain"] = 1
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
	if p.timers["eruption"] <= 0 && p.cfg["eruption_p"] > 0 && p.rng.Float64() < p.cfg["eruption_p"] {
		p.startVolcanoEruptionLocked("started")
	}
	if p.timers["smolder"] <= 0 && p.cfg["smolder_p"] > 0 && p.rng.Float64() < p.cfg["smolder_p"] {
		p.startVolcanoSmolderLocked("started")
	}
	if p.timers["flare"] <= 0 && p.cfg["flare_p"] > 0 && p.rng.Float64() < p.cfg["flare_p"] {
		p.startVolcanoFlareLocked("started")
	}
}

func (p *Procedural) startTrainPassLocked(verb string) {
	p.timers["express"] = 0
	p.timers["quiet-gap"] = 0
	p.timers["pass"] = jitterInt(p.rng, p.intCfg("pass_dur"), 0.3)
	p.values["pass_gain"] = p.cfg["pass_mult"] * (0.9 + p.rng.Float64()*0.3)
	p.values["express_gain"] = 1
	p.values["train_total"] = float64(p.timers["pass"])
	p.values["train_speed"] = p.cfg["speed"] * p.values["pass_gain"]
	p.values["train_dir"] = 1
	if p.rng.Float64() < 0.5 {
		p.values["train_dir"] = -1
	}
	baseCars := p.intCfg("car_count")
	p.values["train_cars"] = float64(max(3, baseCars-1+p.rng.Intn(3)))
	p.values["train_gap"] = math.Max(0.5, p.cfg["car_gap"]*(0.9+p.rng.Float64()*0.25))
	p.values["train_seed"] = p.rng.Float64() * 1000
	p.values["smoke_gain"] = p.cfg["smoke"] * (0.8 + p.rng.Float64()*0.5)
	p.values["headlight_gain"] = p.cfg["headlight"] * (0.9 + p.rng.Float64()*0.35)
	p.appendLog("pass", fmt.Sprintf("%s (dur=%d, dir=%+.0f, cars=%d)", verb, p.timers["pass"], p.values["train_dir"], int(p.values["train_cars"])))
}

func (p *Procedural) startTrainExpressLocked(verb string) {
	p.timers["pass"] = 0
	p.timers["quiet-gap"] = 0
	p.timers["express"] = jitterInt(p.rng, p.intCfg("express_dur"), 0.3)
	p.values["pass_gain"] = 1
	p.values["express_gain"] = p.cfg["express_mult"] * (0.95 + p.rng.Float64()*0.35)
	p.values["train_total"] = float64(p.timers["express"])
	p.values["train_speed"] = p.cfg["speed"] * p.values["express_gain"]
	p.values["train_dir"] = 1
	if p.rng.Float64() < 0.5 {
		p.values["train_dir"] = -1
	}
	baseCars := p.intCfg("car_count")
	p.values["train_cars"] = float64(max(3, baseCars-2+p.rng.Intn(3)))
	p.values["train_gap"] = math.Max(0.5, p.cfg["car_gap"]*(0.75+p.rng.Float64()*0.15))
	p.values["train_seed"] = p.rng.Float64() * 1000
	p.values["smoke_gain"] = p.cfg["smoke"] * (0.7 + p.rng.Float64()*0.35)
	p.values["headlight_gain"] = p.cfg["headlight"] * (1.15 + p.rng.Float64()*0.55)
	p.appendLog("express", fmt.Sprintf("%s (dur=%d, dir=%+.0f, cars=%d)", verb, p.timers["express"], p.values["train_dir"], int(p.values["train_cars"])))
}

func (p *Procedural) startTrainQuietGapLocked(verb string) {
	p.timers["quiet-gap"] = jitterInt(p.rng, p.intCfg("quiet_gap_dur"), 0.3)
	p.appendLog("quiet-gap", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["quiet-gap"], p.cfg["quiet_gap_mult"]))
}

func (p *Procedural) startTrainIntroLocked() {
	p.timers["pass"] = 0
	p.timers["express"] = 0
	p.timers["quiet-gap"] = 0
	p.timers["ending"] = 0
	p.values["pass_gain"] = 1
	p.values["express_gain"] = 1
	p.values["train_total"] = 0
	p.values["train_speed"] = 0
	p.values["train_dir"] = 0
	p.values["train_cars"] = 0
	p.values["train_gap"] = 0
	p.values["train_seed"] = 0
	p.values["smoke_gain"] = 0
	p.values["headlight_gain"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
}

func (p *Procedural) startTrainEndingLocked() {
	p.timers["intro"] = 0
	p.timers["pass"] = 0
	p.timers["express"] = 0
	p.timers["quiet-gap"] = 0
	p.values["pass_gain"] = 1
	p.values["express_gain"] = 1
	p.values["train_total"] = 0
	p.values["train_speed"] = 0
	p.values["train_dir"] = 0
	p.values["train_cars"] = 0
	p.values["train_gap"] = 0
	p.values["train_seed"] = 0
	p.values["smoke_gain"] = 0
	p.values["headlight_gain"] = 0
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
}

func (p *Procedural) stepTrainLocked() {
	if p.timers["pass"] <= 0 && p.timers["express"] <= 0 {
		p.values["pass_gain"] = 1
		p.values["express_gain"] = 1
		p.values["train_total"] = 0
		p.values["train_speed"] = 0
		p.values["train_dir"] = 0
		p.values["train_cars"] = 0
		p.values["train_gap"] = 0
		p.values["train_seed"] = 0
		p.values["smoke_gain"] = 0
		p.values["headlight_gain"] = 0
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
	if p.timers["quiet-gap"] > 0 {
		return
	}
	if p.timers["pass"] <= 0 && p.timers["express"] <= 0 && p.cfg["express_p"] > 0 && p.rng.Float64() < p.cfg["express_p"] {
		p.startTrainExpressLocked("started")
		return
	}
	if p.timers["pass"] <= 0 && p.timers["express"] <= 0 && p.cfg["pass_p"] > 0 && p.rng.Float64() < p.cfg["pass_p"] {
		p.startTrainPassLocked("started")
		return
	}
	if p.timers["pass"] <= 0 && p.timers["express"] <= 0 && p.timers["quiet-gap"] <= 0 && p.cfg["quiet_gap_p"] > 0 && p.rng.Float64() < p.cfg["quiet_gap_p"] {
		p.startTrainQuietGapLocked("started")
	}
}

func (p *Procedural) startMysteriousManInhaleLocked(verb string) {
	p.timers["exhale"] = 0
	p.timers["inhale"] = jitterInt(p.rng, p.intCfg("inhale_dur"), 0.3)
	p.values["inhale_gain"] = p.cfg["inhale_mult"] * (0.85 + p.rng.Float64()*0.35)
	p.values["exhale_gain"] = 1
	p.appendLog("inhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["inhale"], p.values["inhale_gain"]))
}

func (p *Procedural) startMysteriousManExhaleLocked(verb string) {
	p.timers["inhale"] = 0
	p.timers["exhale"] = jitterInt(p.rng, p.intCfg("exhale_dur"), 0.3)
	p.values["inhale_gain"] = 1
	p.values["exhale_gain"] = p.cfg["exhale_mult"] * (0.85 + p.rng.Float64()*0.35)
	p.values["exhale_total"] = float64(p.timers["exhale"])
	p.values["exhale_seed"] = p.rng.Float64() * 1000
	p.values["exhale_dir"] = 1
	if p.rng.Float64() < 0.5 {
		p.values["exhale_dir"] = -1
	}
	p.appendLog("exhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["exhale"], p.values["exhale_gain"]))
}

func (p *Procedural) startMysteriousManAshFallLocked(verb string) {
	p.timers["ash-fall"] = jitterInt(p.rng, p.intCfg("ash_fall_dur"), 0.3)
	p.values["ash_gain"] = p.cfg["ash_fall_mult"] * (0.8 + p.rng.Float64()*0.35)
	p.values["ash_seed"] = p.rng.Float64() * 1000
	p.values["ash_total"] = float64(p.timers["ash-fall"])
	p.appendLog("ash-fall", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["ash-fall"], p.values["ash_gain"]))
}

func (p *Procedural) startMysteriousManLighterFlickLocked(verb string) {
	p.timers["lighter-flick"] = jitterInt(p.rng, p.intCfg("lighter_flick_dur"), 0.3)
	p.values["lighter_gain"] = p.cfg["lighter_flick_mult"] * (0.9 + p.rng.Float64()*0.4)
	p.appendLog("lighter-flick", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lighter-flick"], p.values["lighter_gain"]))
}

func (p *Procedural) startMysteriousManIntroLocked() {
	p.timers["inhale"] = 0
	p.timers["exhale"] = 0
	p.timers["ash-fall"] = 0
	p.timers["lighter-flick"] = 0
	p.timers["ending"] = 0
	p.values["inhale_gain"] = 1
	p.values["exhale_gain"] = 1
	p.values["exhale_total"] = 0
	p.values["lighter_gain"] = 1
	p.values["exhale_seed"] = 0
	p.values["exhale_dir"] = 0
	p.values["ash_gain"] = 1
	p.values["ash_seed"] = 0
	p.values["ash_total"] = 0
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
	p.values["exhale_total"] = 0
	p.values["lighter_gain"] = 1
	p.values["exhale_seed"] = 0
	p.values["exhale_dir"] = 0
	p.values["ash_gain"] = 1
	p.values["ash_seed"] = 0
	p.values["ash_total"] = 0
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
		p.values["exhale_total"] = 0
		p.values["exhale_seed"] = 0
		p.values["exhale_dir"] = 0
	}
	if p.timers["ash-fall"] <= 0 {
		p.values["ash_gain"] = 1
		p.values["ash_seed"] = 0
		p.values["ash_total"] = 0
	}
	if p.timers["lighter-flick"] <= 0 {
		p.values["lighter_gain"] = 1
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
	if p.timers["inhale"] <= 0 && p.timers["exhale"] <= 0 && p.cfg["inhale_p"] > 0 && p.rng.Float64() < p.cfg["inhale_p"] {
		p.startMysteriousManInhaleLocked("started")
		return
	}
	if p.timers["exhale"] <= 0 && p.timers["inhale"] <= 0 && p.cfg["exhale_p"] > 0 && p.rng.Float64() < p.cfg["exhale_p"] {
		p.startMysteriousManExhaleLocked("started")
		return
	}
	if p.timers["ash-fall"] <= 0 && p.cfg["ash_fall_p"] > 0 && p.rng.Float64() < p.cfg["ash_fall_p"] {
		p.startMysteriousManAshFallLocked("started")
	}
	if p.timers["lighter-flick"] <= 0 && p.cfg["lighter_flick_p"] > 0 && p.rng.Float64() < p.cfg["lighter_flick_p"] {
		p.startMysteriousManLighterFlickLocked("started")
	}
}
