package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:         "rain",
		Schema:       sim.RainSchema,
		NewRuntime:   newRainRuntime,
		NewScene:     generateRainScene,
		NewNearScene: generateRainSceneNear,
		// Rain is pure steady-state — independent transient drops, no coupling to
		// a past. Start it fresh on join so it reads as "it began raining".
		FreshJoin: true,
	})
}

type rainRuntime struct {
	sim *sim.Rain
}

func newRainRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.Config
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode rain config: %w", err)
		}
	}
	return &rainRuntime{sim: sim.NewRain(w, h, seed, parsed)}, nil
}

func (r *rainRuntime) Type() string { return "rain" }

func (r *rainRuntime) Schema() sim.EffectSchema { return sim.RainSchema() }

func (r *rainRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *rainRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *rainRuntime) Persisted() (persistedEffectState, error) {
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

func (r *rainRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *rainRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *rainRuntime) Step() { r.sim.Step() }

func (r *rainRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rainRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *rainRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.Config
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rain config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *rainRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *rainRuntime) SceneTransitionTicks(durationTicks int) int {
	return durationTicks / 2
}

func (r *rainRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.Config
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode rain transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode rain transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpConfig(sim.NormalizeConfig(from), sim.NormalizeConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}
