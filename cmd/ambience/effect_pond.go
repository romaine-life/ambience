package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "pond",
		Schema:     sim.PondSchema,
		NewRuntime: newPondRuntime,
	})
}

type pondRuntime struct {
	sim *sim.Pond
}

func newPondRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.PondConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode pond config: %w", err)
		}
	}
	return &pondRuntime{sim: sim.NewPond(w, h, seed, parsed)}, nil
}

func (p *pondRuntime) Type() string { return "pond" }

func (p *pondRuntime) Schema() sim.EffectSchema { return sim.PondSchema() }

func (p *pondRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(p.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := p.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  p.sim.W,
		GridH:  p.sim.H,
	}, nil
}

func (p *pondRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PondSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode pond snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestoreSnapshot(state)
	return nil
}

func (p *pondRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(p.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(p.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  p.sim.W,
		GridH:  p.sim.H,
	}, nil
}

func (p *pondRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PondPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode pond persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestorePersistedState(state)
	return nil
}

func (p *pondRuntime) Trigger(name string) bool { return p.sim.TriggerEvent(name) }

func (p *pondRuntime) Step() { p.sim.Step() }

func (p *pondRuntime) CurrentTick() int { return p.sim.CurrentTick() }

func (p *pondRuntime) DrainLog() []sim.LogEntry { return p.sim.DrainLog() }

func (p *pondRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.PondConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode pond config: %w", err)
		}
	}
	p.sim.SetConfig(cfg)
	return nil
}

func (p *pondRuntime) AddEntropy(delta int64) { p.sim.PerturbRNG(delta) }
