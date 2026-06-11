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

func (r *paperLanternsRuntime) Type() string { return "paper-lanterns" }

func (r *paperLanternsRuntime) Schema() sim.EffectSchema { return sim.PaperLanternsSchema() }

func (r *paperLanternsRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *paperLanternsRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PaperLanternsSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode paper-lanterns snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *paperLanternsRuntime) Persisted() (persistedEffectState, error) {
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

func (r *paperLanternsRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PaperLanternsPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode paper-lanterns persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *paperLanternsRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *paperLanternsRuntime) Step() { r.sim.Step() }

func (r *paperLanternsRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *paperLanternsRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *paperLanternsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.PaperLanternsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode paper-lanterns config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *paperLanternsRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
