package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:         "rain-on-window",
		Schema:       sim.RainOnWindowSchema,
		NewRuntime:   newRainOnWindowRuntime,
		NewScene:     generateRainOnWindowScene,
		NewNearScene: generateRainOnWindowSceneNear,
	})
}

type rainOnWindowRuntime struct {
	sim *sim.RainOnWindow
}

func newRainOnWindowRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.RainOnWindowConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode rain-on-window config: %w", err)
		}
	}
	return &rainOnWindowRuntime{sim: sim.NewRainOnWindow(w, h, seed, parsed)}, nil
}

func (r *rainOnWindowRuntime) Type() string { return "rain-on-window" }

func (r *rainOnWindowRuntime) Schema() sim.EffectSchema { return sim.RainOnWindowSchema() }

func (r *rainOnWindowRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(r.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := r.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  r.sim.W,
		GridH:  r.sim.H,
	}, nil
}

func (r *rainOnWindowRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainOnWindowSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain-on-window snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *rainOnWindowRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(r.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(r.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  r.sim.W,
		GridH:  r.sim.H,
	}, nil
}

func (r *rainOnWindowRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainOnWindowPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain-on-window persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *rainOnWindowRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *rainOnWindowRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *rainOnWindowRuntime) Step() { r.sim.Step() }

func (r *rainOnWindowRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rainOnWindowRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *rainOnWindowRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.RainOnWindowConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rain-on-window config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *rainOnWindowRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *rainOnWindowRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 4
}

func (r *rainOnWindowRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.RainOnWindowConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode rain-on-window transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode rain-on-window transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpRainOnWindowConfig(normalizeRainOnWindowConfig(from), normalizeRainOnWindowConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type rainOnWindowPalette struct {
	name                                            string
	hueMin, hueMax                                  float64
	satMin, satMax                                  float64
	lightMin, lightMax                              float64
	nucleationMin, nucleationMax                    float64
	growMin, growMax                                float64
	criticalMin, criticalMax                        float64
	fallMin, fallMax                                float64
	windMin, windMax                                float64
	glassMin, glassMax                              float64
	formChance, mergeChance, fallChance, gustChance float64
	quietChance                                     float64
}

var rainOnWindowPalettes = []rainOnWindowPalette{
	{
		name: "quiet city", hueMin: 205, hueMax: 224, satMin: 0.22, satMax: 0.42, lightMin: 0.20, lightMax: 0.34,
		nucleationMin: 0.07, nucleationMax: 0.14, growMin: 0.018, growMax: 0.032, criticalMin: 1.7, criticalMax: 2.4,
		fallMin: 0.34, fallMax: 0.58, windMin: -0.18, windMax: 0.22, glassMin: 0.30, glassMax: 0.48,
		formChance: 0.00010, mergeChance: 0.00008, fallChance: 0.00008, gustChance: 0.00005, quietChance: 0.00014,
	},
	{
		name: "evening downpour", hueMin: 34, hueMax: 52, satMin: 0.54, satMax: 0.78, lightMin: 0.36, lightMax: 0.56,
		nucleationMin: 0.24, nucleationMax: 0.40, growMin: 0.040, growMax: 0.068, criticalMin: 1.35, criticalMax: 1.95,
		fallMin: 0.58, fallMax: 0.92, windMin: -0.28, windMax: 0.32, glassMin: 0.36, glassMax: 0.58,
		formChance: 0.00028, mergeChance: 0.00022, fallChance: 0.00024, gustChance: 0.00012, quietChance: 0.00004,
	},
	{
		name: "neon street", hueMin: 268, hueMax: 306, satMin: 0.56, satMax: 0.88, lightMin: 0.30, lightMax: 0.50,
		nucleationMin: 0.15, nucleationMax: 0.27, growMin: 0.030, growMax: 0.052, criticalMin: 1.55, criticalMax: 2.15,
		fallMin: 0.44, fallMax: 0.76, windMin: -0.40, windMax: 0.48, glassMin: 0.42, glassMax: 0.68,
		formChance: 0.00018, mergeChance: 0.00018, fallChance: 0.00016, gustChance: 0.00016, quietChance: 0.00008,
	},
	{
		name: "gentle drizzle", hueMin: 46, hueMax: 68, satMin: 0.28, satMax: 0.52, lightMin: 0.26, lightMax: 0.42,
		nucleationMin: 0.09, nucleationMax: 0.18, growMin: 0.018, growMax: 0.036, criticalMin: 1.8, criticalMax: 2.6,
		fallMin: 0.28, fallMax: 0.52, windMin: -0.12, windMax: 0.16, glassMin: 0.24, glassMax: 0.44,
		formChance: 0.00012, mergeChance: 0.00008, fallChance: 0.00008, gustChance: 0.00004, quietChance: 0.00012,
	},
}

func generateRainOnWindowScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := rainOnWindowPalettes[rng.Intn(len(rainOnWindowPalettes))]
	cfg := randomRainOnWindowConfig(rng, palette)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          palette.name,
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateRainOnWindowSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateRainOnWindowScene(rng, startedAt, durationTicks)
	var prev, target sim.RainOnWindowConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpRainOnWindowConfig(normalizeRainOnWindowConfig(prev), normalizeRainOnWindowConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	random.Config = configData
	random.Name = nameForRainOnWindowConfig(cfg)
	return random
}

func randomRainOnWindowConfig(rng *rngutil.RNG, p rainOnWindowPalette) sim.RainOnWindowConfig {
	intRange := func(minValue, maxValue int) int {
		if maxValue <= minValue {
			return minValue
		}
		return minValue + rng.Intn(maxValue-minValue+1)
	}
	floatRange := func(minValue, maxValue float64) float64 {
		if maxValue <= minValue {
			return minValue
		}
		return minValue + rng.Float64()*(maxValue-minValue)
	}
	return sim.RainOnWindowConfig{
		IntroDur:     intRange(120, 230),
		IntroDensity: floatRange(0.32, 0.72),
		EndingDur:    intRange(220, 360),
		TrackLife:    intRange(140, 260),

		Nucleation:   floatRange(p.nucleationMin, p.nucleationMax),
		GrowRate:     floatRange(p.growMin, p.growMax),
		CriticalMass: floatRange(p.criticalMin, p.criticalMax),
		MergeFactor:  floatRange(1.0, 1.32),
		MaxDrops:     intRange(90, 160),

		FallSpeed:  floatRange(p.fallMin, p.fallMax),
		Wind:       floatRange(p.windMin, p.windMax),
		WindJitter: floatRange(0.10, 0.32),

		GlowHue:      floatRange(p.hueMin, p.hueMax),
		GlowSat:      floatRange(p.satMin, p.satMax),
		GlowLight:    floatRange(p.lightMin, p.lightMax),
		GlassTint:    floatRange(p.glassMin, p.glassMax),
		FrameDark:    floatRange(0.54, 0.72),
		DropContrast: floatRange(0.62, 0.88),

		FormChance:  p.formChance,
		MergeChance: p.mergeChance,
		FallChance:  p.fallChance,
		GustChance:  p.gustChance,
		QuietChance: p.quietChance,

		GustDur:      intRange(110, 230),
		GustStrength: floatRange(0.45, 1.15),
		QuietDur:     intRange(260, 620),
	}
}

func nameForRainOnWindowConfig(cfg sim.RainOnWindowConfig) string {
	palette := "warm"
	switch {
	case cfg.GlowHue >= 255 && cfg.GlowHue <= 325:
		palette = "neon"
	case cfg.GlowHue >= 185 && cfg.GlowHue < 255:
		palette = "city"
	case cfg.GlowHue >= 55 && cfg.GlowHue < 95:
		palette = "gold"
	}
	density := "drizzle"
	switch {
	case cfg.Nucleation >= 0.24:
		density = "downpour"
	case cfg.Nucleation >= 0.15:
		density = "rain"
	}
	return fmt.Sprintf("%s-%s-window", palette, density)
}

func normalizeRainOnWindowConfig(cfg sim.RainOnWindowConfig) sim.RainOnWindowConfig {
	return sim.NewRainOnWindow(1, 1, 1, cfg).EffectiveConfig()
}

func lerpRainOnWindowConfig(a, b sim.RainOnWindowConfig, t float64) sim.RainOnWindowConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.RainOnWindowConfig{
		IntroDur:     li(a.IntroDur, b.IntroDur),
		IntroDensity: lf(a.IntroDensity, b.IntroDensity),
		EndingDur:    li(a.EndingDur, b.EndingDur),
		TrackLife:    li(a.TrackLife, b.TrackLife),

		Nucleation:   lf(a.Nucleation, b.Nucleation),
		GrowRate:     lf(a.GrowRate, b.GrowRate),
		CriticalMass: lf(a.CriticalMass, b.CriticalMass),
		MergeFactor:  lf(a.MergeFactor, b.MergeFactor),
		MaxDrops:     li(a.MaxDrops, b.MaxDrops),

		FallSpeed:  lf(a.FallSpeed, b.FallSpeed),
		Wind:       lf(a.Wind, b.Wind),
		WindJitter: lf(a.WindJitter, b.WindJitter),

		GlowHue:      lerpAngle(a.GlowHue, b.GlowHue, t),
		GlowSat:      lf(a.GlowSat, b.GlowSat),
		GlowLight:    lf(a.GlowLight, b.GlowLight),
		GlassTint:    lf(a.GlassTint, b.GlassTint),
		FrameDark:    lf(a.FrameDark, b.FrameDark),
		DropContrast: lf(a.DropContrast, b.DropContrast),

		FormChance:  lf(a.FormChance, b.FormChance),
		MergeChance: lf(a.MergeChance, b.MergeChance),
		FallChance:  lf(a.FallChance, b.FallChance),
		GustChance:  lf(a.GustChance, b.GustChance),
		QuietChance: lf(a.QuietChance, b.QuietChance),

		GustDur:      li(a.GustDur, b.GustDur),
		GustStrength: lf(a.GustStrength, b.GustStrength),
		QuietDur:     li(a.QuietDur, b.QuietDur),
	}
}
