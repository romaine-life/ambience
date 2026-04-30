package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "wheat-field",
		Schema:     sim.WheatFieldSchema,
		NewRuntime: newWheatFieldRuntime,
	})
}

type wheatFieldRuntime struct {
	sim *sim.WheatField
}

func newWheatFieldRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.WheatFieldConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode wheat-field config: %w", err)
		}
	}
	return &wheatFieldRuntime{sim: sim.NewWheatField(w, h, seed, parsed)}, nil
}

func (r *wheatFieldRuntime) Type() string { return "wheat-field" }

func (r *wheatFieldRuntime) Schema() sim.EffectSchema { return sim.WheatFieldSchema() }

func (r *wheatFieldRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *wheatFieldRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WheatFieldSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode wheat-field snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *wheatFieldRuntime) Persisted() (persistedEffectState, error) {
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

func (r *wheatFieldRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WheatFieldPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode wheat-field persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *wheatFieldRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *wheatFieldRuntime) Step() { r.sim.Step() }

func (r *wheatFieldRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *wheatFieldRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *wheatFieldRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.WheatFieldConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode wheat-field config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *wheatFieldRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
