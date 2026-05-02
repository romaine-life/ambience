package main

import (
	"encoding/json"
	"fmt"

	"github.com/nelsong6/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "bog-bubbles",
		Schema:     sim.BogBubblesSchema,
		NewRuntime: newBogBubblesRuntime,
	})
}

type bogBubblesRuntime struct {
	sim *sim.BogBubbles
}

func newBogBubblesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.BogBubblesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode bog-bubbles config: %w", err)
		}
	}
	return &bogBubblesRuntime{sim: sim.NewBogBubbles(w, h, seed, parsed)}, nil
}

func (r *bogBubblesRuntime) Type() string { return "bog-bubbles" }

func (r *bogBubblesRuntime) Schema() sim.EffectSchema { return sim.BogBubblesSchema() }

func (r *bogBubblesRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *bogBubblesRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BogBubblesSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode bog-bubbles snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *bogBubblesRuntime) Persisted() (persistedEffectState, error) {
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

func (r *bogBubblesRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BogBubblesPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode bog-bubbles persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *bogBubblesRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *bogBubblesRuntime) Step() { r.sim.Step() }

func (r *bogBubblesRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *bogBubblesRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *bogBubblesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.BogBubblesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode bog-bubbles config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *bogBubblesRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
