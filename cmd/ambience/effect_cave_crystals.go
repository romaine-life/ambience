package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "cave-crystals",
		Schema:     sim.CaveCrystalsSchema,
		NewRuntime: newCaveCrystalsRuntime,
	})
}

type caveCrystalsRuntime struct {
	sim *sim.CaveCrystals
}

func newCaveCrystalsRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.CaveCrystalsConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode cave-crystals config: %w", err)
		}
	}
	return &caveCrystalsRuntime{sim: sim.NewCaveCrystals(w, h, seed, parsed)}, nil
}

func (r *caveCrystalsRuntime) Type() string { return "cave-crystals" }

func (r *caveCrystalsRuntime) Schema() sim.EffectSchema { return sim.CaveCrystalsSchema() }

func (r *caveCrystalsRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *caveCrystalsRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CaveCrystalsSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cave-crystals snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *caveCrystalsRuntime) Persisted() (persistedEffectState, error) {
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

func (r *caveCrystalsRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CaveCrystalsPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cave-crystals persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *caveCrystalsRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *caveCrystalsRuntime) Step() { r.sim.Step() }

func (r *caveCrystalsRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *caveCrystalsRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *caveCrystalsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.CaveCrystalsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode cave-crystals config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *caveCrystalsRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
