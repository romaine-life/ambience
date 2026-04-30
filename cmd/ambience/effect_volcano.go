package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "volcano",
		Schema:     sim.VolcanoSchema,
		NewRuntime: newVolcanoRuntime,
	})
}

type volcanoRuntime struct {
	sim *sim.Volcano
}

func newVolcanoRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.VolcanoConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode volcano config: %w", err)
		}
	}
	return &volcanoRuntime{sim: sim.NewVolcano(w, h, seed, parsed)}, nil
}

func (r *volcanoRuntime) Type() string { return "volcano" }

func (r *volcanoRuntime) Schema() sim.EffectSchema { return sim.VolcanoSchema() }

func (r *volcanoRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *volcanoRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.VolcanoSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode volcano snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *volcanoRuntime) Persisted() (persistedEffectState, error) {
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

func (r *volcanoRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.VolcanoPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode volcano persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *volcanoRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *volcanoRuntime) Step() { r.sim.Step() }

func (r *volcanoRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *volcanoRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *volcanoRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.VolcanoConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode volcano config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *volcanoRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
