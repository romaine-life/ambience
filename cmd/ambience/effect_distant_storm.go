package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "distant-storm",
		Schema:     sim.DistantStormSchema,
		NewRuntime: newDistantStormRuntime,
	})
}

type distantStormRuntime struct {
	sim *sim.DistantStorm
}

func newDistantStormRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.DistantStormConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode distant-storm config: %w", err)
		}
	}
	return &distantStormRuntime{sim: sim.NewDistantStorm(w, h, seed, parsed)}, nil
}

func (r *distantStormRuntime) Type() string { return "distant-storm" }

func (r *distantStormRuntime) Schema() sim.EffectSchema { return sim.DistantStormSchema() }

func (r *distantStormRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *distantStormRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DistantStormSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode distant-storm snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *distantStormRuntime) Persisted() (persistedEffectState, error) {
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

func (r *distantStormRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DistantStormPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode distant-storm persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *distantStormRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *distantStormRuntime) Step() { r.sim.Step() }

func (r *distantStormRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *distantStormRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *distantStormRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.DistantStormConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode distant-storm config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *distantStormRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
