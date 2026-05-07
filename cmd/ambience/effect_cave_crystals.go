package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "cave-crystals",
		Schema:     sim.CaveCrystalsSchema,
		NewRuntime: newCaveCrystalsRuntime,
	})
}

type caveCrystalsRuntime struct {
	sim *sim.CaveCrystals
}

func newCaveCrystalsRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.CaveCrystalsConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode cave-crystals config: %w", err)
		}
	}
	return &caveCrystalsRuntime{sim: sim.NewCaveCrystals(w, h, seed, parsed)}, nil
}

func (c *caveCrystalsRuntime) Type() string { return "cave-crystals" }

func (c *caveCrystalsRuntime) Schema() sim.EffectSchema { return sim.CaveCrystalsSchema() }

func (c *caveCrystalsRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(c.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := c.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  c.sim.W,
		GridH:  c.sim.H,
	}, nil
}

func (c *caveCrystalsRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := c.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CaveCrystalsSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cave-crystals snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (c.sim.W != s.GridW || c.sim.H != s.GridH) {
		c.sim.Resize(s.GridW, s.GridH)
	}
	c.sim.RestoreSnapshot(state)
	return nil
}

func (c *caveCrystalsRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(c.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(c.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  c.sim.W,
		GridH:  c.sim.H,
	}, nil
}

func (c *caveCrystalsRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := c.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.CaveCrystalsPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode cave-crystals persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (c.sim.W != s.GridW || c.sim.H != s.GridH) {
		c.sim.Resize(s.GridW, s.GridH)
	}
	c.sim.RestorePersistedState(state)
	return nil
}

func (c *caveCrystalsRuntime) Trigger(name string) bool { return c.sim.TriggerEvent(name) }

func (c *caveCrystalsRuntime) Step() { c.sim.Step() }

func (c *caveCrystalsRuntime) CurrentTick() int { return c.sim.CurrentTick() }

func (c *caveCrystalsRuntime) DrainLog() []sim.LogEntry { return c.sim.DrainLog() }

func (c *caveCrystalsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.CaveCrystalsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode cave-crystals config: %w", err)
		}
	}
	c.sim.SetConfig(cfg)
	return nil
}

func (c *caveCrystalsRuntime) AddEntropy(delta int64) { c.sim.PerturbRNG(delta) }
