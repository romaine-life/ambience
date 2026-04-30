package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "rain-on-window",
		Schema:     sim.RainOnWindowSchema,
		NewRuntime: newRainOnWindowRuntime,
	})
}

type rainOnWindowRuntime struct {
	sim *sim.RainOnWindow
}

func newRainOnWindowRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.RainOnWindowConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode rain-on-window config: %w", err)
		}
	}
	return &rainOnWindowRuntime{sim: sim.NewRainOnWindow(w, h, seed, parsed)}, nil
}

func (r *rainOnWindowRuntime) Type() string { return "rain-on-window" }

func (r *rainOnWindowRuntime) Schema() sim.EffectSchema { return sim.RainOnWindowSchema() }

func (r *rainOnWindowRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *rainOnWindowRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainOnWindowSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain-on-window snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *rainOnWindowRuntime) Persisted() (persistedEffectState, error) {
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

func (r *rainOnWindowRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainOnWindowPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain-on-window persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *rainOnWindowRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *rainOnWindowRuntime) Step() { r.sim.Step() }

func (r *rainOnWindowRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rainOnWindowRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *rainOnWindowRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.RainOnWindowConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rain-on-window config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *rainOnWindowRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
