package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "cottage-chimney",
		Schema:     sim.CottageChimneySchema,
		NewRuntime: newCottageChimneyRuntime,
	})
}

type cottageChimneyRuntime struct {
	sim *sim.CottageChimney
}

func newCottageChimneyRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.CottageChimneyConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode cottage-chimney config: %w", err)
		}
	}
	return &cottageChimneyRuntime{sim: sim.NewCottageChimney(w, h, seed, parsed)}, nil
}

func (r *cottageChimneyRuntime) Type() string { return "cottage-chimney" }

func (r *cottageChimneyRuntime) Schema() sim.EffectSchema { return sim.CottageChimneySchema() }

func (r *cottageChimneyRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *cottageChimneyRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CottageChimneySnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cottage-chimney snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *cottageChimneyRuntime) Persisted() (persistedEffectState, error) {
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

func (r *cottageChimneyRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CottageChimneyPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cottage-chimney persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *cottageChimneyRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *cottageChimneyRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *cottageChimneyRuntime) Step() { r.sim.Step() }

func (r *cottageChimneyRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *cottageChimneyRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *cottageChimneyRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.CottageChimneyConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode cottage-chimney config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *cottageChimneyRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
