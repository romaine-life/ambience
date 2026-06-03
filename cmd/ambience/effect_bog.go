package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "bog",
		Schema:     sim.BogSchema,
		NewRuntime: newBogRuntime,
	})
}

type bogRuntime struct {
	sim *sim.Bog
}

func newBogRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.BogConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode bog config: %w", err)
		}
	}
	return &bogRuntime{sim: sim.NewBog(w, h, seed, parsed)}, nil
}

func (b *bogRuntime) Type() string { return "bog" }

func (b *bogRuntime) Schema() sim.EffectSchema { return sim.BogSchema() }

func (b *bogRuntime) Snapshot() (effectEnvelope, error) {
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

func (b *bogRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := b.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BogSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode bog snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (b.sim.W != s.GridW || b.sim.H != s.GridH) {
		b.sim.Resize(s.GridW, s.GridH)
	}
	b.sim.RestoreSnapshot(state)
	return nil
}

func (b *bogRuntime) Persisted() (persistedEffectState, error) {
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

func (b *bogRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := b.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BogPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode bog persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (b.sim.W != s.GridW || b.sim.H != s.GridH) {
		b.sim.Resize(s.GridW, s.GridH)
	}
	b.sim.RestorePersistedState(state)
	return nil
}

func (b *bogRuntime) Trigger(name string) bool { return b.sim.TriggerEvent(name) }

func (b *bogRuntime) Step() { b.sim.Step() }

func (b *bogRuntime) CurrentTick() int { return b.sim.CurrentTick() }

func (b *bogRuntime) DrainLog() []sim.LogEntry { return b.sim.DrainLog() }

func (b *bogRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.BogConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode bog config: %w", err)
		}
	}
	b.sim.SetConfig(cfg)
	return nil
}

func (b *bogRuntime) AddEntropy(delta int64) { b.sim.PerturbRNG(delta) }
