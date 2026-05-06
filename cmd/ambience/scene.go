// Scene is a time-bounded effect configuration. The atmosphere keeps a
// single-slot lookahead (current + next) and transitions when current's
// DurationTicks elapses. Each scene is freshly generated from the atmosphere's
// RNG, so entropy contributed via AddEntropy naturally biases future scene
// generation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/nelsong6/ambience/rngutil"
	"github.com/nelsong6/ambience/sim"
)

type Scene struct {
	Name          string          `json:"name"`
	Config        json.RawMessage `json:"config,omitempty"`
	DurationTicks int             `json:"durationTicks"`
	StartedAtTick int             `json:"startedAtTick"`
}

// Remaining returns ticks left before transition. Clamps at zero.
func (s Scene) Remaining(currentTick int) int {
	r := s.StartedAtTick + s.DurationTicks - currentTick
	if r < 0 {
		return 0
	}
	return r
}

func generateEffectScene(effectType string, rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	if def, ok := lookupEffectDefinition(effectType); ok && def.NewScene != nil {
		return def.NewScene(rng, startedAt, durationTicks)
	}
	return generateSchemaScene(effectType, rng, startedAt, durationTicks)
}

func generateSchemaScene(effectType string, rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	configData := randomEffectSceneConfig(effectType, rng)
	return Scene{
		Name:          nameForEffectScene(effectType, rng),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func randomEffectSceneConfig(effectType string, rng *rngutil.RNG) json.RawMessage {
	schema, ok := schemaForEffect(effectType)
	if !ok {
		return nil
	}
	data, err := randomizedDevConfig(schema, rng.Int63())
	if err != nil {
		return nil
	}
	return data
}

// generateRainScene produces a Scene using rng. Duration is randomized across
// 1–4 hours (36k–144k ticks at 10 Hz). The config ranges are kept within
// sim-safe bounds so any generated scene is guaranteed to look reasonable.
func generateRainScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	_ = durationTicks
	hue := rng.Float64() * 360
	hueSpread := 10 + rng.Float64()*50     // 10–60°
	sat := 0.4 + rng.Float64()*0.5         // 0.4–0.9
	lmin := 0.2 + rng.Float64()*0.3        // 0.2–0.5
	lmax := lmin + 0.2 + rng.Float64()*0.3 // lmin + 0.2..0.5

	speed := 0.6 + rng.Float64()*1.4  // 0.6–2.0
	spawnEvery := 3 + rng.Intn(8)     // 3–10
	spawnBurst := 1 + rng.Intn(3)     // 1–3
	streak := 3 + rng.Intn(8)         // 3–10
	fade := 0.80 + rng.Float64()*0.15 // 0.80–0.95
	wind := -0.4 + rng.Float64()*0.8  // -0.4..+0.4
	windJit := rng.Float64() * 0.3
	speedJit := rng.Float64() * 0.3

	layers := 2 + rng.Intn(3)               // 2–4
	layerBalance := 0.3 + rng.Float64()*0.4 // 0.3–0.7

	// Event chances kept low; transitions should feel natural at 10 Hz.
	downpourP := 0.0005 + rng.Float64()*0.001
	calmP := 0.0005 + rng.Float64()*0.001
	gustP := 0.0005 + rng.Float64()*0.001
	splashP := 0.001 + rng.Float64()*0.002

	cfg := sim.Config{
		Wind:           wind,
		WindJitter:     windJit,
		Speed:          speed,
		SpeedJitter:    speedJit,
		StreakLen:      streak,
		FadeFactor:     fade,
		SpawnEvery:     spawnEvery,
		SpawnBurst:     spawnBurst,
		Hue:            hue,
		HueSpread:      hueSpread,
		Saturation:     sat,
		LightnessMin:   lmin,
		LightnessMax:   lmax,
		Layers:         layers,
		LayerBalance:   layerBalance,
		DownpourChance: downpourP,
		CalmChance:     calmP,
		GustChance:     gustP,
		SplashChance:   splashP,
		// Event modifiers fall through to withDefaults().
	}
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForRainConfig(cfg),
		Config:        configData,
		DurationTicks: sceneDurationTicks(rng),
		StartedAtTick: startedAt,
	}
}

func sceneDurationTicks(rng *rngutil.RNG) int {
	// Duration: 1–4 hours at 10 Hz. AMBIENCE_SCENE_TICKS overrides for local
	// testing, e.g. set to 300 for 30 s scenes to watch transitions fire.
	const ticksPerHour = 36000
	dur := ticksPerHour + rng.Intn(3*ticksPerHour)
	if v, err := strconv.Atoi(os.Getenv("AMBIENCE_SCENE_TICKS")); err == nil && v > 0 {
		dur = v
	}
	return dur
}

// nameFor derives a short human-readable descriptor from a generated config.
// Format: `<hue>-<pace>-<density>` e.g. `warm-fast-drizzle`,
// `cool-calm-downpour`. Used in logs and the / status panel.
func nameForRainConfig(cfg sim.Config) string {
	var hueName string
	switch {
	case cfg.Hue < 45 || cfg.Hue >= 340:
		hueName = "red"
	case cfg.Hue < 90:
		hueName = "amber"
	case cfg.Hue < 160:
		hueName = "green"
	case cfg.Hue < 210:
		hueName = "cyan"
	case cfg.Hue < 270:
		hueName = "blue"
	default:
		hueName = "violet"
	}

	var paceName string
	switch {
	case cfg.Speed < 0.9:
		paceName = "slow"
	case cfg.Speed < 1.4:
		paceName = "steady"
	default:
		paceName = "fast"
	}

	var densityName string
	switch {
	case cfg.SpawnEvery >= 8:
		densityName = "drizzle"
	case cfg.SpawnEvery >= 5:
		densityName = "rain"
	default:
		densityName = "downpour"
	}

	return fmt.Sprintf("%s-%s-%s", hueName, paceName, densityName)
}

var sceneNameAdjectives = []string{
	"amber",
	"blue",
	"bright",
	"calm",
	"dim",
	"drifting",
	"faint",
	"glowing",
	"green",
	"hushed",
	"late",
	"low",
	"midnight",
	"quiet",
	"silver",
	"slow",
	"soft",
	"violet",
	"warm",
}

var effectSceneSubjects = map[string][]string{
	"aurora":         {"aurora", "skyfire", "polar-lights", "northern-lights", "light-curtain"},
	"autumn-leaves":  {"autumn-leaves", "fall-leaves", "leaf-fall"},
	"beach":          {"beach", "shore", "tide", "surf"},
	"burning-trees":  {"burning-trees", "forest-fire", "ember-woods"},
	"campfire":       {"campfire", "firelight", "embers", "hearth"},
	"cave-crystals":  {"cave-crystals", "geode", "crystal-cave", "amethyst-grotto"},
	"dust":           {"dust", "motes", "haze"},
	"fireflies":      {"fireflies", "glowflies", "lantern-bugs"},
	"lighthouse":     {"lighthouse", "beacon", "harbor-light"},
	"mysterious-man": {"mysterious-man", "stranger", "silhouette"},
	"rowboat":        {"rowboat", "skiff", "small-boat"},
	"sand":           {"sand", "dunes", "drift-sand"},
	"snow":           {"snow", "snowfall", "flurries"},
	"starfield":      {"starfield", "stars", "night-sky"},
	"tetris":         {"tetris", "falling-blocks", "tetrominoes"},
	"train":          {"train", "railway", "locomotive"},
	"underwater":     {"underwater", "deepwater", "reef"},
	"volcano":        {"volcano", "caldera", "lava"},
	"water-pipe":     {"water-pipe", "pipe-flow", "spout"},
	"waterfall":      {"waterfall", "falls", "cascade"},
	"wheat-field":    {"wheat-field", "grain-field", "wheat"},
	"windmill":       {"windmill", "mill", "sails"},
}

func nameForEffectScene(effectType string, rng *rngutil.RNG) string {
	adjective := sceneNameAdjectives[rng.Intn(len(sceneNameAdjectives))]
	subjects := effectSceneSubjects[effectType]
	if len(subjects) == 0 {
		subjects = []string{effectType}
	}
	subject := subjects[rng.Intn(len(subjects))]
	return fmt.Sprintf("%s-%s", adjective, subject)
}
