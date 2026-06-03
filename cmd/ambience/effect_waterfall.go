package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "waterfall",
		Schema:     sim.WaterfallSchema,
		NewRuntime: newWaterfallRuntime,
	})
}

type waterfallRuntime struct {
	sim *sim.Waterfall
}

func newWaterfallRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.WaterfallConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode waterfall config: %w", err)
		}
	}
	return &waterfallRuntime{sim: sim.NewWaterfall(w, h, seed, parsed)}, nil
}

func (w *waterfallRuntime) Type() string { return "waterfall" }

func (w *waterfallRuntime) Schema() sim.EffectSchema { return sim.WaterfallSchema() }

func (w *waterfallRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(w.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := w.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  w.sim.W,
		GridH:  w.sim.H,
	}, nil
}

func (w *waterfallRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := w.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterfallSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode waterfall snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (w.sim.W != s.GridW || w.sim.H != s.GridH) {
		w.sim.Resize(s.GridW, s.GridH)
	}
	w.sim.RestoreSnapshot(state)
	return nil
}

func (w *waterfallRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(w.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(w.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  w.sim.W,
		GridH:  w.sim.H,
	}, nil
}

func (w *waterfallRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := w.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterfallPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode waterfall persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (w.sim.W != s.GridW || w.sim.H != s.GridH) {
		w.sim.Resize(s.GridW, s.GridH)
	}
	w.sim.RestorePersistedState(state)
	return nil
}

func (w *waterfallRuntime) Trigger(name string) bool { return w.sim.TriggerEvent(name) }

func (w *waterfallRuntime) Step() { w.sim.Step() }

func (w *waterfallRuntime) CurrentTick() int { return w.sim.CurrentTick() }

func (w *waterfallRuntime) DrainLog() []sim.LogEntry { return w.sim.DrainLog() }

func (w *waterfallRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.WaterfallConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode waterfall config: %w", err)
		}
	}
	w.sim.SetConfig(cfg)
	return nil
}

func (w *waterfallRuntime) AddEntropy(delta int64) { w.sim.PerturbRNG(delta) }
