package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "paper-lanterns",
		Schema:     sim.PaperLanternsSchema,
		NewRuntime: newPaperLanternsRuntime,
	})
}

type paperLanternsRuntime struct {
	sim *sim.PaperLanterns
}

func newPaperLanternsRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.PaperLanternsConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode paper-lanterns config: %w", err)
		}
	}
	return &paperLanternsRuntime{sim: sim.NewPaperLanterns(w, h, seed, parsed)}, nil
}

func (p *paperLanternsRuntime) Type() string { return "paper-lanterns" }

func (p *paperLanternsRuntime) Schema() sim.EffectSchema { return sim.PaperLanternsSchema() }

func (p *paperLanternsRuntime) Snapshot() (effectEnvelope, error) {
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

func (p *paperLanternsRuntime) Restore(env effectEnvelope) error {
	if len(env.Config) > 0 {
		if err := p.ApplyConfig(env.Config); err != nil {
			return err
		}
	}
	if len(env.State) == 0 {
		return nil
	}
	var state sim.PaperLanternsSnapshot
	if err := json.Unmarshal(env.State, &state); err != nil {
		return fmt.Errorf("decode paper-lanterns snapshot: %w", err)
	}
	if env.GridW > 0 && env.GridH > 0 && (p.sim.W != env.GridW || p.sim.H != env.GridH) {
		p.sim.Resize(env.GridW, env.GridH)
	}
	p.sim.RestoreSnapshot(state)
	return nil
}

func (p *paperLanternsRuntime) Persisted() (persistedEffectState, error) {
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

func (p *paperLanternsRuntime) RestorePersisted(env persistedEffectState) error {
	if len(env.Config) > 0 {
		if err := p.ApplyConfig(env.Config); err != nil {
			return err
		}
	}
	if len(env.State) == 0 {
		return nil
	}
	var state sim.PaperLanternsPersistedState
	if err := json.Unmarshal(env.State, &state); err != nil {
		return fmt.Errorf("decode paper-lanterns persisted state: %w", err)
	}
	if env.GridW > 0 && env.GridH > 0 && (p.sim.W != env.GridW || p.sim.H != env.GridH) {
		p.sim.Resize(env.GridW, env.GridH)
	}
	p.sim.RestorePersistedState(state)
	return nil
}

func (p *paperLanternsRuntime) Trigger(name string) bool { return p.sim.TriggerEvent(name) }

func (p *paperLanternsRuntime) Step() { p.sim.Step() }

func (p *paperLanternsRuntime) CurrentTick() int { return p.sim.CurrentTick() }

func (p *paperLanternsRuntime) DrainLog() []sim.LogEntry { return p.sim.DrainLog() }

func (p *paperLanternsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.PaperLanternsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode paper-lanterns config: %w", err)
		}
	}
	p.sim.SetConfig(cfg)
	return nil
}

func (p *paperLanternsRuntime) AddEntropy(delta int64) { p.sim.PerturbRNG(delta) }

func (p *paperLanternsRuntime) Frame() [][]sim.Pixel { return p.sim.GridCopy() }
