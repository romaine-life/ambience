package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "rowboat",
		Schema:     sim.RowboatSchema,
		NewRuntime: newRowboatRuntime,
	})
}

type rowboatRuntime struct {
	sim *sim.Rowboat
}

func newRowboatRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.RowboatConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode rowboat config: %w", err)
		}
	}
	return &rowboatRuntime{sim: sim.NewRowboat(w, h, seed, parsed)}, nil
}

func (r *rowboatRuntime) Type() string { return "rowboat" }

func (r *rowboatRuntime) Schema() sim.EffectSchema { return sim.RowboatSchema() }

func (r *rowboatRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *rowboatRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RowboatSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rowboat snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *rowboatRuntime) Persisted() (persistedEffectState, error) {
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

func (r *rowboatRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RowboatPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rowboat persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *rowboatRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *rowboatRuntime) Step() { r.sim.Step() }

func (r *rowboatRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rowboatRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *rowboatRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.RowboatConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rowboat config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *rowboatRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
