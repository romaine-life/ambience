package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "mysterious-man",
		Schema:     sim.MysteriousManSchema,
		NewRuntime: newMysteriousManRuntime,
	})
}

type mysteriousManRuntime struct {
	sim *sim.MysteriousMan
}

func newMysteriousManRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.MysteriousManConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode mysterious-man config: %w", err)
		}
	}
	return &mysteriousManRuntime{sim: sim.NewMysteriousMan(w, h, seed, parsed)}, nil
}

func (r *mysteriousManRuntime) Type() string { return "mysterious-man" }

func (r *mysteriousManRuntime) Schema() sim.EffectSchema { return sim.MysteriousManSchema() }

func (r *mysteriousManRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *mysteriousManRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.MysteriousManSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode mysterious-man snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *mysteriousManRuntime) Persisted() (persistedEffectState, error) {
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

func (r *mysteriousManRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.MysteriousManPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode mysterious-man persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *mysteriousManRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *mysteriousManRuntime) Step() { r.sim.Step() }

func (r *mysteriousManRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *mysteriousManRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *mysteriousManRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.MysteriousManConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode mysterious-man config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *mysteriousManRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
