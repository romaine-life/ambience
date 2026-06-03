package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "slimes",
		Schema:     sim.SlimesSchema,
		NewRuntime: newSlimesRuntime,
	})
}

type slimesRuntime struct {
	sim *sim.Slimes
}

func newSlimesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.SlimesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode slimes config: %w", err)
		}
	}
	return &slimesRuntime{sim: sim.NewSlimes(w, h, seed, parsed)}, nil
}

func (s *slimesRuntime) Type() string { return "slimes" }

func (s *slimesRuntime) Schema() sim.EffectSchema { return sim.SlimesSchema() }

func (s *slimesRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(s.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := s.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  s.sim.W,
		GridH:  s.sim.H,
	}, nil
}

func (s *slimesRuntime) Restore(env effectEnvelope) error {
	if len(env.Config) > 0 {
		if err := s.ApplyConfig(env.Config); err != nil {
			return err
		}
	}
	if len(env.State) == 0 {
		return nil
	}
	var state sim.SlimesSnapshot
	if err := json.Unmarshal(env.State, &state); err != nil {
		return fmt.Errorf("decode slimes snapshot: %w", err)
	}
	if env.GridW > 0 && env.GridH > 0 && (s.sim.W != env.GridW || s.sim.H != env.GridH) {
		s.sim.Resize(env.GridW, env.GridH)
	}
	s.sim.RestoreSnapshot(state)
	return nil
}

func (s *slimesRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(s.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(s.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  s.sim.W,
		GridH:  s.sim.H,
	}, nil
}

func (s *slimesRuntime) RestorePersisted(env persistedEffectState) error {
	if len(env.Config) > 0 {
		if err := s.ApplyConfig(env.Config); err != nil {
			return err
		}
	}
	if len(env.State) == 0 {
		return nil
	}
	var state sim.SlimesPersistedState
	if err := json.Unmarshal(env.State, &state); err != nil {
		return fmt.Errorf("decode slimes persisted state: %w", err)
	}
	if env.GridW > 0 && env.GridH > 0 && (s.sim.W != env.GridW || s.sim.H != env.GridH) {
		s.sim.Resize(env.GridW, env.GridH)
	}
	s.sim.RestorePersistedState(state)
	return nil
}

func (s *slimesRuntime) Trigger(name string) bool { return s.sim.TriggerEvent(name) }

func (s *slimesRuntime) Step() { s.sim.Step() }

func (s *slimesRuntime) CurrentTick() int { return s.sim.CurrentTick() }

func (s *slimesRuntime) DrainLog() []sim.LogEntry { return s.sim.DrainLog() }

func (s *slimesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.SlimesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode slimes config: %w", err)
		}
	}
	s.sim.SetConfig(cfg)
	return nil
}

func (s *slimesRuntime) AddEntropy(delta int64) { s.sim.PerturbRNG(delta) }
