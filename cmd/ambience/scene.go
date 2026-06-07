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
	"time"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

type Scene struct {
	Name          string          `json:"name"`
	Config        json.RawMessage `json:"config,omitempty"`
	DurationTicks int             `json:"durationTicks"`
	StartedAtTick int             `json:"startedAtTick"`
}

type scenePolicy struct {
	MinTicks        int     `json:"minTicks"`
	MaxTicks        int     `json:"maxTicks"`
	TransitionTicks int     `json:"transitionTicks"`
	Variation       float64 `json:"variation"`
}

func defaultScenePolicy() scenePolicy {
	p := scenePolicy{
		MinTicks:        ticksFor(time.Hour),
		MaxTicks:        ticksFor(4 * time.Hour),
		TransitionTicks: ticksFor(5 * time.Minute),
		Variation:       0.35,
	}
	if v, err := strconv.Atoi(os.Getenv("AMBIENCE_SCENE_TICKS")); err == nil && v > 0 {
		p.MinTicks = v
		p.MaxTicks = v
	}
	return p.normalized()
}

func (p scenePolicy) normalized() scenePolicy {
	if p.MinTicks <= 0 {
		p.MinTicks = ticksFor(time.Hour)
	}
	if p.MaxTicks <= 0 {
		p.MaxTicks = p.MinTicks
	}
	if p.MaxTicks < p.MinTicks {
		p.MinTicks, p.MaxTicks = p.MaxTicks, p.MinTicks
	}
	if p.TransitionTicks < 0 {
		p.TransitionTicks = 0
	}
	if p.Variation <= 0 {
		p.Variation = 0.35
	}
	if p.Variation > 1 {
		p.Variation = 1
	}
	return p
}

func (p scenePolicy) durationTicks(rng *rngutil.RNG) int {
	p = p.normalized()
	if p.MaxTicks <= p.MinTicks {
		return p.MinTicks
	}
	return p.MinTicks + rng.Intn(p.MaxTicks-p.MinTicks+1)
}

func (p scenePolicy) minMinutes() float64 {
	return float64(p.normalized().MinTicks) * float64(tickRate) / float64(time.Minute)
}

func (p scenePolicy) maxMinutes() float64 {
	return float64(p.normalized().MaxTicks) * float64(tickRate) / float64(time.Minute)
}

func (p scenePolicy) transitionMinutes() float64 {
	return float64(p.normalized().TransitionTicks) * float64(tickRate) / float64(time.Minute)
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

func generateInitialScene(effectType string, rng *rngutil.RNG, startedAt int, policy scenePolicy) Scene {
	return generateEffectScene(effectType, rng, startedAt, policy.durationTicks(rng))
}

func generateNextScene(effectType string, rng *rngutil.RNG, startedAt int, policy scenePolicy, previousConfig json.RawMessage) Scene {
	durationTicks := policy.durationTicks(rng)
	if def, ok := lookupEffectDefinition(effectType); ok && def.NewNearScene != nil && len(previousConfig) > 0 {
		return def.NewNearScene(rng, startedAt, durationTicks, previousConfig, policy.normalized().Variation)
	}
	return generateEffectScene(effectType, rng, startedAt, durationTicks)
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
// 1–4 hours. The config ranges are kept within sim-safe bounds so any
// generated scene is guaranteed to look reasonable.
func generateRainScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	hue := 204 + rng.Float64()*28            // cool blue/cyan rain family
	hueSpread := 4 + rng.Float64()*12        // 4–16°
	sat := 0.18 + rng.Float64()*0.24         // 0.18–0.42
	lmin := 0.28 + rng.Float64()*0.16        // 0.28–0.44
	lmax := lmin + 0.20 + rng.Float64()*0.16 // lmin + 0.20..0.36

	speed := 1.55 + rng.Float64()*0.75 // 1.55–2.30
	spawnEvery := 3 + rng.Intn(3)      // 3–5
	spawnBurst := 3 + rng.Intn(3)      // 3–5
	streak := 10 + rng.Intn(5)         // 10–14
	fade := 0.88 + rng.Float64()*0.08  // 0.88–0.96
	wind := -0.35 + rng.Float64()*0.7  // -0.35..+0.35
	windJit := rng.Float64() * 0.18
	speedJit := rng.Float64() * 0.18

	layers := 2
	layerBalance := 0.45 + rng.Float64()*0.20 // 0.45–0.65

	sheetDensity := 0.52 + rng.Float64()*0.24
	sheetStrength := 0.22 + rng.Float64()*0.16
	sheetLength := 9 + rng.Intn(6)
	sheetSpeed := 1.35 + rng.Float64()*0.55
	frontDensity := 0.28 + rng.Float64()*0.22
	frontStrength := 0.4 + rng.Float64()*0.25
	frontLength := 18 + rng.Intn(13)
	frontSpeed := 42 + rng.Float64()*28

	// Per-tick event chances stay low at 60 Hz; events should punctuate the
	// field rather than constantly re-shape it.
	downpourP := 0.00008 + rng.Float64()*0.00017
	calmP := 0.00008 + rng.Float64()*0.00017
	gustP := 0.00008 + rng.Float64()*0.00017
	splashP := 0.00016 + rng.Float64()*0.00034

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
		SheetDensity:   sheetDensity,
		SheetStrength:  sheetStrength,
		SheetLength:    sheetLength,
		SheetSpeed:     sheetSpeed,
		FrontDensity:   frontDensity,
		FrontStrength:  frontStrength,
		FrontLength:    frontLength,
		FrontSpeed:     frontSpeed,
		DownpourChance: downpourP,
		CalmChance:     calmP,
		GustChance:     gustP,
		SplashChance:   splashP,
		// Event modifiers fall through to withDefaults().
	}
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForRainConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateRainSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateRainScene(rng, startedAt, durationTicks)
	var prev, target sim.Config
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpConfig(sim.NormalizeConfig(prev), sim.NormalizeConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForRainConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func sceneDurationTicks(rng *rngutil.RNG) int {
	// Duration: 1–4 hours. AMBIENCE_SCENE_TICKS overrides for local testing,
	// e.g. set to 1800 for 30 s scenes to watch transitions fire at 60 Hz.
	ticksPerHour := ticksFor(time.Hour)
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
	case cfg.Speed < 1.4:
		paceName = "slow"
	case cfg.Speed < 2.0:
		paceName = "steady"
	default:
		paceName = "fast"
	}

	var densityName string
	switch {
	case cfg.SpawnEvery >= 5:
		densityName = "drizzle"
	case cfg.SpawnEvery >= 4:
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
	"cave-crystals":  {"cave-crystals", "geode", "crystal-grove", "shard-grove"},
	"distant-storm":  {"distant-storm", "horizon-storm", "quiet-horizon", "stormfront"},
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
