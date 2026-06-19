package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:         "constellations",
		Schema:       sim.ConstellationsSchema,
		NewRuntime:   newConstellationsRuntime,
		NewScene:     generateConstellationsScene,
		NewNearScene: generateConstellationsSceneNear,
	})
}

type constellationsRuntime struct {
	sim *sim.Constellations
}

func newConstellationsRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.ConstellationsConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode constellations config: %w", err)
		}
	}
	return &constellationsRuntime{sim: sim.NewConstellations(w, h, seed, parsed)}, nil
}

func (r *constellationsRuntime) Type() string { return "constellations" }

func (r *constellationsRuntime) Schema() sim.EffectSchema { return sim.ConstellationsSchema() }

func (r *constellationsRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *constellationsRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.ConstellationsSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode constellations snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *constellationsRuntime) Persisted() (persistedEffectState, error) {
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

func (r *constellationsRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.ConstellationsPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode constellations persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *constellationsRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *constellationsRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *constellationsRuntime) Step() { r.sim.Step() }

func (r *constellationsRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *constellationsRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *constellationsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.ConstellationsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode constellations config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *constellationsRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func generateConstellationsScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	cfg := randomConstellationsConfig(rng, rng.Intn(len(constellationSceneNames)))
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForConstellationsConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateConstellationsSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateConstellationsScene(rng, startedAt, durationTicks)
	var prev, target sim.ConstellationsConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	prev = sim.NormalizeConstellationsConfig(prev)
	target = sim.NormalizeConstellationsConfig(target)
	t := clampUnit(variation)
	cfg := sim.ConstellationsConfig{
		Twinkle:       lerpConstellationsFloat(prev.Twinkle, target.Twinkle, t),
		StarRate:      lerpConstellationsFloat(prev.StarRate, target.StarRate, t),
		DrawChance:    lerpConstellationsFloat(prev.DrawChance, target.DrawChance, t),
		FigureShimmer: lerpConstellationsFloat(prev.FigureShimmer, target.FigureShimmer, t),
		FigureSet:     nextConstellationsFigureSet(rng, prev.FigureSet),
	}
	cfg = sim.NormalizeConstellationsConfig(cfg)
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForConstellationsConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func randomConstellationsConfig(rng *rngutil.RNG, figureSet int) sim.ConstellationsConfig {
	return sim.ConstellationsConfig{
		Twinkle:       0.3 + rng.Float64()*0.65,
		StarRate:      0.32 + rng.Float64()*0.56,
		DrawChance:    0.00035 + rng.Float64()*0.00055,
		FigureShimmer: 0.35 + rng.Float64()*0.8,
		FigureSet:     figureSet,
	}
}

func nextConstellationsFigureSet(rng *rngutil.RNG, previous int) int {
	if len(constellationSceneNames) <= 1 {
		return 0
	}
	previous = sim.NormalizeConstellationsConfig(sim.ConstellationsConfig{FigureSet: previous}).FigureSet
	next := rng.Intn(len(constellationSceneNames) - 1)
	if next >= previous {
		next++
	}
	return next
}

func lerpConstellationsFloat(a, b, t float64) float64 {
	return a + (b-a)*t
}

func nameForConstellationsConfig(cfg sim.ConstellationsConfig) string {
	cfg = sim.NormalizeConstellationsConfig(cfg)
	set := cfg.FigureSet
	if set < 0 || set >= len(constellationSceneNames) {
		set = 0
	}
	pace := "slow"
	if cfg.DrawChance > 0.0007 {
		pace = "forming"
	} else if cfg.DrawChance < 0.00045 {
		pace = "quiet"
	}
	return fmt.Sprintf("%s-%s", constellationSceneNames[set], pace)
}

var constellationSceneNames = []string{
	"zodiac",
	"northern-winter",
	"summer-triangle",
	"mythic",
}
