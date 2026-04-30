package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "sand",
		Schema:     sim.SandSchema,
		NewRuntime: newSandRuntime,
	})
}

type sandRuntime struct {
	sim *sim.Sand
}

func newSandRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.SandConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode sand config: %w", err)
		}
	}
	return &sandRuntime{sim: sim.NewSand(w, h, seed, parsed)}, nil
}

func (s *sandRuntime) Type() string { return "sand" }

func (s *sandRuntime) Schema() sim.EffectSchema { return sim.SandSchema() }

func (s *sandRuntime) Snapshot() (effectEnvelope, error) {
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

func (s *sandRuntime) Restore(env effectEnvelope) error {
	if len(env.Config) > 0 {
		if err := s.ApplyConfig(env.Config); err != nil {
			return err
		}
	}
	if len(env.State) == 0 {
		return nil
	}
	var state sim.SandSnapshot
	if err := json.Unmarshal(env.State, &state); err != nil {
		return fmt.Errorf("decode sand snapshot: %w", err)
	}
	if env.GridW > 0 && env.GridH > 0 && (s.sim.W != env.GridW || s.sim.H != env.GridH) {
		s.sim.Resize(env.GridW, env.GridH)
	}
	s.sim.RestoreSnapshot(state)
	return nil
}

func (s *sandRuntime) Persisted() (persistedEffectState, error) {
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

func (s *sandRuntime) RestorePersisted(p persistedEffectState) error {
	if len(p.Config) > 0 {
		if err := s.ApplyConfig(p.Config); err != nil {
			return err
		}
	}
	if len(p.State) == 0 {
		return nil
	}
	var state sim.SandPersistedState
	if err := json.Unmarshal(p.State, &state); err != nil {
		return fmt.Errorf("decode sand persisted state: %w", err)
	}
	if p.GridW > 0 && p.GridH > 0 && (s.sim.W != p.GridW || s.sim.H != p.GridH) {
		s.sim.Resize(p.GridW, p.GridH)
	}
	s.sim.RestorePersistedState(state)
	return nil
}

func (s *sandRuntime) Trigger(name string) bool { return s.sim.TriggerEvent(name) }

func (s *sandRuntime) Step() { s.sim.Step() }

func (s *sandRuntime) CurrentTick() int { return s.sim.CurrentTick() }

func (s *sandRuntime) DrainLog() []sim.LogEntry { return s.sim.DrainLog() }

func (s *sandRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.SandConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode sand config: %w", err)
		}
	}
	s.sim.SetConfig(cfg)
	return nil
}

func (s *sandRuntime) AddEntropy(delta int64) { s.sim.PerturbRNG(delta) }
