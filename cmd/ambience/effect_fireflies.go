package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "fireflies",
		Schema:     sim.FirefliesSchema,
		NewRuntime: newFirefliesRuntime,
	})
}

type firefliesRuntime struct {
	sim *sim.Fireflies
}

func newFirefliesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.FirefliesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode fireflies config: %w", err)
		}
	}
	return &firefliesRuntime{sim: sim.NewFireflies(w, h, seed, parsed)}, nil
}

func (f *firefliesRuntime) Type() string { return "fireflies" }

func (f *firefliesRuntime) Schema() sim.EffectSchema { return sim.FirefliesSchema() }

func (f *firefliesRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(f.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := f.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  f.sim.W,
		GridH:  f.sim.H,
	}, nil
}

func (f *firefliesRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := f.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.FirefliesSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode fireflies snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (f.sim.W != s.GridW || f.sim.H != s.GridH) {
		f.sim.Resize(s.GridW, s.GridH)
	}
	f.sim.RestoreSnapshot(state)
	return nil
}

func (f *firefliesRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(f.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(f.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  f.sim.W,
		GridH:  f.sim.H,
	}, nil
}

func (f *firefliesRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := f.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.FirefliesPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode fireflies persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (f.sim.W != s.GridW || f.sim.H != s.GridH) {
		f.sim.Resize(s.GridW, s.GridH)
	}
	f.sim.RestorePersistedState(state)
	return nil
}

func (f *firefliesRuntime) Trigger(name string) bool { return f.sim.TriggerEvent(name) }

func (f *firefliesRuntime) Step() { f.sim.Step() }

func (f *firefliesRuntime) CurrentTick() int { return f.sim.CurrentTick() }

func (f *firefliesRuntime) DrainLog() []sim.LogEntry { return f.sim.DrainLog() }

func (f *firefliesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.FirefliesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode fireflies config: %w", err)
		}
	}
	f.sim.SetConfig(cfg)
	return nil
}

func (f *firefliesRuntime) AddEntropy(delta int64) { f.sim.PerturbRNG(delta) }
