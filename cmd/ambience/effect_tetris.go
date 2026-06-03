package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "tetris",
		Schema:     sim.TetrisSchema,
		NewRuntime: newTetrisRuntime,
	})
}

type tetrisRuntime struct {
	sim *sim.Tetris
}

func newTetrisRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.TetrisConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode tetris config: %w", err)
		}
	}
	return &tetrisRuntime{sim: sim.NewTetris(w, h, seed, parsed)}, nil
}

func (t *tetrisRuntime) Type() string { return "tetris" }

func (t *tetrisRuntime) Schema() sim.EffectSchema { return sim.TetrisSchema() }

func (t *tetrisRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(t.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := t.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  t.sim.W,
		GridH:  t.sim.H,
	}, nil
}

func (t *tetrisRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := t.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.TetrisSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode tetris snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (t.sim.W != s.GridW || t.sim.H != s.GridH) {
		t.sim.Resize(s.GridW, s.GridH)
	}
	t.sim.RestoreSnapshot(state)
	return nil
}

func (t *tetrisRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(t.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(t.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  t.sim.W,
		GridH:  t.sim.H,
	}, nil
}

func (t *tetrisRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := t.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.TetrisPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode tetris persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (t.sim.W != s.GridW || t.sim.H != s.GridH) {
		t.sim.Resize(s.GridW, s.GridH)
	}
	t.sim.RestorePersistedState(state)
	return nil
}

func (t *tetrisRuntime) Trigger(name string) bool { return t.sim.TriggerEvent(name) }

func (t *tetrisRuntime) Step() { t.sim.Step() }

func (t *tetrisRuntime) CurrentTick() int { return t.sim.CurrentTick() }

func (t *tetrisRuntime) DrainLog() []sim.LogEntry { return t.sim.DrainLog() }

func (t *tetrisRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.TetrisConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode tetris config: %w", err)
		}
	}
	t.sim.SetConfig(cfg)
	return nil
}

func (t *tetrisRuntime) AddEntropy(delta int64) { t.sim.PerturbRNG(delta) }
