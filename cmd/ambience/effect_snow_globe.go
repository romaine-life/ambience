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
		Type:         "snow-globe",
		Schema:       sim.SnowGlobeSchema,
		NewRuntime:   newSnowGlobeRuntime,
		NewScene:     generateSnowGlobeScene,
		NewNearScene: generateSnowGlobeSceneNear,
	})
}

type snowGlobeRuntime struct {
	sim *sim.SnowGlobe
}

func newSnowGlobeRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.SnowGlobeConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode snow-globe config: %w", err)
		}
	}
	return &snowGlobeRuntime{sim: sim.NewSnowGlobe(w, h, seed, parsed)}, nil
}

func (r *snowGlobeRuntime) Type() string { return "snow-globe" }

func (r *snowGlobeRuntime) Schema() sim.EffectSchema { return sim.SnowGlobeSchema() }

func (r *snowGlobeRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *snowGlobeRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.SnowGlobeSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode snow-globe snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *snowGlobeRuntime) Persisted() (persistedEffectState, error) {
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

func (r *snowGlobeRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.SnowGlobePersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode snow-globe persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *snowGlobeRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *snowGlobeRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *snowGlobeRuntime) Step() { r.sim.Step() }

func (r *snowGlobeRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *snowGlobeRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *snowGlobeRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.SnowGlobeConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode snow-globe config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *snowGlobeRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *snowGlobeRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 8
}

func (r *snowGlobeRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.SnowGlobeConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode snow-globe transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode snow-globe transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpSnowGlobeConfig(sim.NormalizeSnowGlobeConfig(from), sim.NormalizeSnowGlobeConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func generateSnowGlobeScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	cfg := sim.SnowGlobeConfig{
		SettleRate:      0.78 + rng.Float64()*0.55,
		SnowVolume:      0.50 + rng.Float64()*0.45,
		ShakeCadence:    40 + rng.Float64()*18,
		SwirlTurbulence: 0.70 + rng.Float64()*0.85,
		Scene:           rng.Intn(4),
		IntroDur:        90,
		EndingDur:       300,
	}
	cfg = sim.NormalizeSnowGlobeConfig(cfg)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForSnowGlobeConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateSnowGlobeSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateSnowGlobeScene(rng, startedAt, durationTicks)
	var prev, target sim.SnowGlobeConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	prev = sim.NormalizeSnowGlobeConfig(prev)
	target = sim.NormalizeSnowGlobeConfig(target)
	if target.Scene == prev.Scene {
		target.Scene = (prev.Scene + 1 + rng.Intn(3)) % 4
	}
	cfg := lerpSnowGlobeConfig(prev, target, math.Max(0.45, clampUnit(variation)))
	cfg.Scene = target.Scene
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForSnowGlobeConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func lerpSnowGlobeConfig(a, b sim.SnowGlobeConfig, t float64) sim.SnowGlobeConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(math.Round(float64(y-x)*t)) }
	scene := a.Scene
	if t >= 0.5 {
		scene = b.Scene
	}
	return sim.NormalizeSnowGlobeConfig(sim.SnowGlobeConfig{
		SettleRate:      lf(a.SettleRate, b.SettleRate),
		SnowVolume:      lf(a.SnowVolume, b.SnowVolume),
		ShakeCadence:    lf(a.ShakeCadence, b.ShakeCadence),
		SwirlTurbulence: lf(a.SwirlTurbulence, b.SwirlTurbulence),
		Scene:           scene,
		IntroDur:        li(a.IntroDur, b.IntroDur),
		EndingDur:       li(a.EndingDur, b.EndingDur),
	})
}

func nameForSnowGlobeConfig(cfg sim.SnowGlobeConfig) string {
	cfg = sim.NormalizeSnowGlobeConfig(cfg)
	names := []string{"cabin", "lone-pine", "village", "lighthouse"}
	scene := cfg.Scene
	if scene < 0 || scene >= len(names) {
		scene = 0
	}
	switch {
	case cfg.SnowVolume >= 0.86:
		return "deep-snow-" + names[scene]
	case cfg.SwirlTurbulence >= 1.28:
		return "shaken-" + names[scene]
	default:
		return "quiet-" + names[scene]
	}
}
