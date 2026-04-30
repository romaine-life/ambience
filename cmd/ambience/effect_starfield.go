package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "starfield",
		Schema:     sim.StarfieldSchema,
		NewRuntime: newStarfieldRuntime,
	})
}

type starfieldRuntime struct {
	sim *sim.Starfield
}

func newStarfieldRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.StarfieldConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode starfield config: %w", err)
		}
	}
	return &starfieldRuntime{sim: sim.NewStarfield(w, h, seed, parsed)}, nil
}

func (r *starfieldRuntime) Type() string { return "starfield" }

func (r *starfieldRuntime) Schema() sim.EffectSchema { return sim.StarfieldSchema() }

func (r *starfieldRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *starfieldRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.StarfieldSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode starfield snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *starfieldRuntime) Persisted() (persistedEffectState, error) {
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

func (r *starfieldRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.StarfieldPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode starfield persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *starfieldRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *starfieldRuntime) Step() { r.sim.Step() }

func (r *starfieldRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *starfieldRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *starfieldRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.StarfieldConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode starfield config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *starfieldRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
