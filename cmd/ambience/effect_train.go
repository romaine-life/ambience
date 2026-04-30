package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "train",
		Schema:     sim.TrainSchema,
		NewRuntime: newTrainRuntime,
	})
}

type trainRuntime struct {
	sim *sim.Train
}

func newTrainRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.TrainConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode train config: %w", err)
		}
	}
	return &trainRuntime{sim: sim.NewTrain(w, h, seed, parsed)}, nil
}

func (r *trainRuntime) Type() string { return "train" }

func (r *trainRuntime) Schema() sim.EffectSchema { return sim.TrainSchema() }

func (r *trainRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *trainRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.TrainSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode train snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *trainRuntime) Persisted() (persistedEffectState, error) {
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

func (r *trainRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.TrainPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode train persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *trainRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *trainRuntime) Step() { r.sim.Step() }

func (r *trainRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *trainRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *trainRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.TrainConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode train config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *trainRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
