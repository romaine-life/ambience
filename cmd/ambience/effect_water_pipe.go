package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "water-pipe",
		Schema:     sim.WaterPipeSchema,
		NewRuntime: newWaterPipeRuntime,
	})
}

type waterPipeRuntime struct {
	sim *sim.WaterPipe
}

func newWaterPipeRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.WaterPipeConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode water-pipe config: %w", err)
		}
	}
	return &waterPipeRuntime{sim: sim.NewWaterPipe(w, h, seed, parsed)}, nil
}

func (p *waterPipeRuntime) Type() string { return "water-pipe" }

func (p *waterPipeRuntime) Schema() sim.EffectSchema { return sim.WaterPipeSchema() }

func (p *waterPipeRuntime) Snapshot() (effectEnvelope, error) {
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

func (p *waterPipeRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterPipeSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode water-pipe snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestoreSnapshot(state)
	return nil
}

func (p *waterPipeRuntime) Persisted() (persistedEffectState, error) {
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

func (p *waterPipeRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterPipePersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode water-pipe persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestorePersistedState(state)
	return nil
}

func (p *waterPipeRuntime) Trigger(name string) bool { return p.sim.TriggerEvent(name) }

func (p *waterPipeRuntime) Step() { p.sim.Step() }

func (p *waterPipeRuntime) CurrentTick() int { return p.sim.CurrentTick() }

func (p *waterPipeRuntime) DrainLog() []sim.LogEntry { return p.sim.DrainLog() }

func (p *waterPipeRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.WaterPipeConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode water-pipe config: %w", err)
		}
	}
	p.sim.SetConfig(cfg)
	return nil
}

func (p *waterPipeRuntime) AddEntropy(delta int64) { p.sim.PerturbRNG(delta) }
