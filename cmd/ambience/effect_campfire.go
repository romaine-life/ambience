package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "campfire",
		Schema:     sim.CampfireSchema,
		NewRuntime: newCampfireRuntime,
	})
}

type campfireRuntime struct {
	sim *sim.Campfire
}

func newCampfireRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.CampfireConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode campfire config: %w", err)
		}
	}
	return &campfireRuntime{sim: sim.NewCampfire(w, h, seed, parsed)}, nil
}

func (r *campfireRuntime) Type() string { return "campfire" }

func (r *campfireRuntime) Schema() sim.EffectSchema { return sim.CampfireSchema() }

func (r *campfireRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *campfireRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CampfireSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode campfire snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *campfireRuntime) Persisted() (persistedEffectState, error) {
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

func (r *campfireRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CampfirePersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode campfire persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *campfireRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *campfireRuntime) Step() { r.sim.Step() }

func (r *campfireRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *campfireRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *campfireRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.CampfireConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode campfire config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *campfireRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
