package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "lighthouse",
		Schema:     sim.LighthouseSchema,
		NewRuntime: newLighthouseRuntime,
	})
}

type lighthouseRuntime struct {
	sim *sim.Lighthouse
}

func newLighthouseRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.LighthouseConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode lighthouse config: %w", err)
		}
	}
	return &lighthouseRuntime{sim: sim.NewLighthouse(w, h, seed, parsed)}, nil
}

func (r *lighthouseRuntime) Type() string { return "lighthouse" }

func (r *lighthouseRuntime) Schema() sim.EffectSchema { return sim.LighthouseSchema() }

func (r *lighthouseRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *lighthouseRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.LighthouseSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode lighthouse snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *lighthouseRuntime) Persisted() (persistedEffectState, error) {
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

func (r *lighthouseRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.LighthousePersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode lighthouse persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *lighthouseRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *lighthouseRuntime) Step() { r.sim.Step() }

func (r *lighthouseRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *lighthouseRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *lighthouseRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.LighthouseConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode lighthouse config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *lighthouseRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
