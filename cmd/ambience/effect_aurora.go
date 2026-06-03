package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "aurora",
		Schema:     sim.AuroraSchema,
		NewRuntime: newAuroraRuntime,
	})
}

type auroraRuntime struct {
	sim *sim.Aurora
}

func newAuroraRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.AuroraConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode aurora config: %w", err)
		}
	}
	return &auroraRuntime{sim: sim.NewAurora(w, h, seed, parsed)}, nil
}

func (r *auroraRuntime) Type() string { return "aurora" }

func (r *auroraRuntime) Schema() sim.EffectSchema { return sim.AuroraSchema() }

func (r *auroraRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *auroraRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.AuroraSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode aurora snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *auroraRuntime) Persisted() (persistedEffectState, error) {
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

func (r *auroraRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.AuroraPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode aurora persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *auroraRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *auroraRuntime) Step() { r.sim.Step() }

func (r *auroraRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *auroraRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *auroraRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.AuroraConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode aurora config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *auroraRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
