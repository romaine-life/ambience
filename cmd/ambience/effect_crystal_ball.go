package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:         "crystal-ball",
		Schema:       sim.CrystalBallSchema,
		NewRuntime:   newCrystalBallRuntime,
		NewScene:     generateCrystalBallScene,
		NewNearScene: generateCrystalBallSceneNear,
	})
}

type crystalBallRuntime struct {
	sim *sim.CrystalBall
}

func newCrystalBallRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.CrystalBallConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode crystal-ball config: %w", err)
		}
	}
	return &crystalBallRuntime{sim: sim.NewCrystalBall(w, h, seed, parsed)}, nil
}

func (r *crystalBallRuntime) Type() string { return "crystal-ball" }

func (r *crystalBallRuntime) Schema() sim.EffectSchema { return sim.CrystalBallSchema() }

func (r *crystalBallRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *crystalBallRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CrystalBallSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode crystal-ball snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *crystalBallRuntime) Persisted() (persistedEffectState, error) {
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

func (r *crystalBallRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CrystalBallPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode crystal-ball persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *crystalBallRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *crystalBallRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *crystalBallRuntime) Step() { r.sim.Step() }

func (r *crystalBallRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *crystalBallRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *crystalBallRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.CrystalBallConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode crystal-ball config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *crystalBallRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *crystalBallRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 6
}

func (r *crystalBallRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.CrystalBallConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode crystal-ball transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode crystal-ball transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpCrystalBallConfig(sim.NormalizeCrystalBallConfig(from), sim.NormalizeCrystalBallConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type crystalBallPalette struct {
	name                       string
	hue, hueSpread             float64
	mistMin, mistMax           float64
	swirlMin, swirlMax         float64
	visionChanceMin, visionMax float64
	glowPulseMin, glowPulseMax float64
}

var crystalBallPalettes = []crystalBallPalette{
	{
		name: "violet seer", hue: 276, hueSpread: 14,
		mistMin: 0.66, mistMax: 0.98, swirlMin: 0.54, swirlMax: 0.94,
		visionChanceMin: 0.00045, visionMax: 0.00078, glowPulseMin: 0.56, glowPulseMax: 0.86,
	},
	{
		name: "emerald", hue: 142, hueSpread: 13,
		mistMin: 0.58, mistMax: 0.92, swirlMin: 0.48, swirlMax: 0.88,
		visionChanceMin: 0.00038, visionMax: 0.00068, glowPulseMin: 0.48, glowPulseMax: 0.76,
	},
	{
		name: "smoke grey", hue: 218, hueSpread: 9,
		mistMin: 0.42, mistMax: 0.72, swirlMin: 0.34, swirlMax: 0.66,
		visionChanceMin: 0.00024, visionMax: 0.00046, glowPulseMin: 0.28, glowPulseMax: 0.52,
	},
	{
		name: "starlit", hue: 228, hueSpread: 18,
		mistMin: 0.54, mistMax: 0.86, swirlMin: 0.66, swirlMax: 1.18,
		visionChanceMin: 0.00036, visionMax: 0.00072, glowPulseMin: 0.62, glowPulseMax: 0.96,
	},
}

func generateCrystalBallScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := crystalBallPalettes[rng.Intn(len(crystalBallPalettes))]
	cfg := randomCrystalBallConfig(rng, palette)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForCrystalBallConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateCrystalBallSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateCrystalBallScene(rng, startedAt, durationTicks)
	var prev, target sim.CrystalBallConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpCrystalBallConfig(sim.NormalizeCrystalBallConfig(prev), sim.NormalizeCrystalBallConfig(target), math.Max(0.35, clampUnit(variation)))
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForCrystalBallConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func randomCrystalBallConfig(rng *rngutil.RNG, p crystalBallPalette) sim.CrystalBallConfig {
	floatRange := func(minValue, maxValue float64) float64 {
		if maxValue <= minValue {
			return minValue
		}
		return minValue + rng.Float64()*(maxValue-minValue)
	}
	return sim.NormalizeCrystalBallConfig(sim.CrystalBallConfig{
		Swirl:        floatRange(p.swirlMin, p.swirlMax),
		MistRate:     floatRange(p.mistMin, p.mistMax),
		VisionChance: floatRange(p.visionChanceMin, p.visionMax),
		GlowPulse:    floatRange(p.glowPulseMin, p.glowPulseMax),
		Hue:          math.Mod(p.hue+(rng.Float64()*2-1)*p.hueSpread+360, 360),
	})
}

func nameForCrystalBallConfig(cfg sim.CrystalBallConfig) string {
	cfg = sim.NormalizeCrystalBallConfig(cfg)
	palette := "violet-seer"
	switch {
	case cfg.Hue >= 105 && cfg.Hue < 170:
		palette = "emerald"
	case cfg.Hue >= 190 && cfg.Hue < 245 && cfg.GlowPulse < 0.58:
		palette = "smoke-grey"
	case cfg.Hue >= 198 && cfg.Hue < 252:
		palette = "starlit"
	case cfg.Hue >= 250 && cfg.Hue < 310:
		palette = "violet-seer"
	}
	motion := "slow"
	switch {
	case cfg.Swirl >= 1.0:
		motion = "deep-swirl"
	case cfg.MistRate >= 0.9:
		motion = "heavy-mist"
	}
	return fmt.Sprintf("%s-%s-orb", palette, motion)
}

func lerpCrystalBallConfig(a, b sim.CrystalBallConfig, t float64) sim.CrystalBallConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	return sim.NormalizeCrystalBallConfig(sim.CrystalBallConfig{
		Swirl:        lf(a.Swirl, b.Swirl),
		MistRate:     lf(a.MistRate, b.MistRate),
		VisionChance: lf(a.VisionChance, b.VisionChance),
		GlowPulse:    lf(a.GlowPulse, b.GlowPulse),
		Hue:          lerpAngle(a.Hue, b.Hue, t),
	})
}
