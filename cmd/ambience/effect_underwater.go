package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "underwater",
		Schema:     sim.UnderwaterSchema,
		NewRuntime: newUnderwaterRuntime,
	})
}

type underwaterRuntime struct {
	sim *sim.Underwater
}

func newUnderwaterRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.UnderwaterConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode underwater config: %w", err)
		}
	}
	return &underwaterRuntime{sim: sim.NewUnderwater(w, h, seed, parsed)}, nil
}

func (r *underwaterRuntime) Type() string { return "underwater" }

func (r *underwaterRuntime) Schema() sim.EffectSchema { return sim.UnderwaterSchema() }

func (r *underwaterRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *underwaterRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.UnderwaterSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode underwater snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *underwaterRuntime) Persisted() (persistedEffectState, error) {
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

func (r *underwaterRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.UnderwaterPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode underwater persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *underwaterRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *underwaterRuntime) Step() { r.sim.Step() }

func (r *underwaterRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *underwaterRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *underwaterRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.UnderwaterConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode underwater config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *underwaterRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
