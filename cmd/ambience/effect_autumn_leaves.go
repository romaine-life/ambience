package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "autumn-leaves",
		Schema:     sim.AutumnLeavesSchema,
		NewRuntime: newAutumnLeavesRuntime,
	})
}

type autumnLeavesRuntime struct {
	sim *sim.AutumnLeaves
}

func newAutumnLeavesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.AutumnLeavesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode autumn-leaves config: %w", err)
		}
	}
	return &autumnLeavesRuntime{sim: sim.NewAutumnLeaves(w, h, seed, parsed)}, nil
}

func (r *autumnLeavesRuntime) Type() string { return "autumn-leaves" }

func (r *autumnLeavesRuntime) Schema() sim.EffectSchema { return sim.AutumnLeavesSchema() }

func (r *autumnLeavesRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *autumnLeavesRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.AutumnLeavesSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode autumn-leaves snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *autumnLeavesRuntime) Persisted() (persistedEffectState, error) {
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

func (r *autumnLeavesRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.AutumnLeavesPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode autumn-leaves persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *autumnLeavesRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *autumnLeavesRuntime) Step() { r.sim.Step() }

func (r *autumnLeavesRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *autumnLeavesRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *autumnLeavesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.AutumnLeavesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode autumn-leaves config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *autumnLeavesRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
