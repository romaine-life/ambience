package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "burning-trees",
		Schema:     sim.BurningTreesSchema,
		NewRuntime: newBurningTreesRuntime,
	})
}

type burningTreesRuntime struct {
	sim *sim.BurningTrees
}

func newBurningTreesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.BurningTreesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode burning-trees config: %w", err)
		}
	}
	return &burningTreesRuntime{sim: sim.NewBurningTrees(w, h, seed, parsed)}, nil
}

func (b *burningTreesRuntime) Type() string { return "burning-trees" }

func (b *burningTreesRuntime) Schema() sim.EffectSchema { return sim.BurningTreesSchema() }

func (b *burningTreesRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(b.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := b.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  b.sim.W,
		GridH:  b.sim.H,
	}, nil
}

func (b *burningTreesRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := b.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BurningTreesSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode burning-trees snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (b.sim.W != s.GridW || b.sim.H != s.GridH) {
		b.sim.Resize(s.GridW, s.GridH)
	}
	b.sim.RestoreSnapshot(state)
	return nil
}

func (b *burningTreesRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(b.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(b.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  b.sim.W,
		GridH:  b.sim.H,
	}, nil
}

func (b *burningTreesRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := b.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BurningTreesPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode burning-trees persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (b.sim.W != s.GridW || b.sim.H != s.GridH) {
		b.sim.Resize(s.GridW, s.GridH)
	}
	b.sim.RestorePersistedState(state)
	return nil
}

func (b *burningTreesRuntime) Trigger(name string) bool { return b.sim.TriggerEvent(name) }

func (b *burningTreesRuntime) Step() { b.sim.Step() }

func (b *burningTreesRuntime) CurrentTick() int { return b.sim.CurrentTick() }

func (b *burningTreesRuntime) DrainLog() []sim.LogEntry { return b.sim.DrainLog() }

func (b *burningTreesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.BurningTreesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode burning-trees config: %w", err)
		}
	}
	b.sim.SetConfig(cfg)
	return nil
}

func (b *burningTreesRuntime) AddEntropy(delta int64) { b.sim.PerturbRNG(delta) }
