package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "dust",
		Schema:     sim.DustSchema,
		NewRuntime: newDustRuntime,
	})
}

type dustRuntime struct {
	sim *sim.Dust
}

func newDustRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.DustConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode dust config: %w", err)
		}
	}
	return &dustRuntime{sim: sim.NewDust(w, h, seed, parsed)}, nil
}

func (d *dustRuntime) Type() string { return "dust" }

func (d *dustRuntime) Schema() sim.EffectSchema { return sim.DustSchema() }

func (d *dustRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(d.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := d.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  d.sim.W,
		GridH:  d.sim.H,
	}, nil
}

func (d *dustRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := d.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DustSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode dust snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (d.sim.W != s.GridW || d.sim.H != s.GridH) {
		d.sim.Resize(s.GridW, s.GridH)
	}
	d.sim.RestoreSnapshot(state)
	return nil
}

func (d *dustRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(d.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(d.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  d.sim.W,
		GridH:  d.sim.H,
	}, nil
}

func (d *dustRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := d.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DustPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode dust persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (d.sim.W != s.GridW || d.sim.H != s.GridH) {
		d.sim.Resize(s.GridW, s.GridH)
	}
	d.sim.RestorePersistedState(state)
	return nil
}

func (d *dustRuntime) Trigger(name string) bool { return d.sim.TriggerEvent(name) }

func (d *dustRuntime) Step() { d.sim.Step() }

func (d *dustRuntime) CurrentTick() int { return d.sim.CurrentTick() }

func (d *dustRuntime) DrainLog() []sim.LogEntry { return d.sim.DrainLog() }

func (d *dustRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.DustConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode dust config: %w", err)
		}
	}
	d.sim.SetConfig(cfg)
	return nil
}

func (d *dustRuntime) AddEntropy(delta int64) { d.sim.PerturbRNG(delta) }
