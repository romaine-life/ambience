package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "magic-portal",
		Schema:     sim.MagicPortalSchema,
		NewRuntime: newMagicPortalRuntime,
	})
}

type magicPortalRuntime struct {
	sim *sim.MagicPortal
}

func newMagicPortalRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.MagicPortalConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode magic-portal config: %w", err)
		}
	}
	return &magicPortalRuntime{sim: sim.NewMagicPortal(w, h, seed, parsed)}, nil
}

func (m *magicPortalRuntime) Type() string { return "magic-portal" }

func (m *magicPortalRuntime) Schema() sim.EffectSchema { return sim.MagicPortalSchema() }

func (m *magicPortalRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(m.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := m.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  m.sim.W,
		GridH:  m.sim.H,
	}, nil
}

func (m *magicPortalRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := m.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.MagicPortalSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode magic-portal snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (m.sim.W != s.GridW || m.sim.H != s.GridH) {
		m.sim.Resize(s.GridW, s.GridH)
	}
	m.sim.RestoreSnapshot(state)
	return nil
}

func (m *magicPortalRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(m.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(m.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  m.sim.W,
		GridH:  m.sim.H,
	}, nil
}

func (m *magicPortalRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := m.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.MagicPortalPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode magic-portal persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (m.sim.W != s.GridW || m.sim.H != s.GridH) {
		m.sim.Resize(s.GridW, s.GridH)
	}
	m.sim.RestorePersistedState(state)
	return nil
}

func (m *magicPortalRuntime) Trigger(name string) bool { return m.sim.TriggerEvent(name) }

func (m *magicPortalRuntime) Frame() [][]sim.Pixel { return m.sim.GridCopy() }

func (m *magicPortalRuntime) Step() { m.sim.Step() }

func (m *magicPortalRuntime) CurrentTick() int { return m.sim.CurrentTick() }

func (m *magicPortalRuntime) DrainLog() []sim.LogEntry { return m.sim.DrainLog() }

func (m *magicPortalRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.MagicPortalConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode magic-portal config: %w", err)
		}
	}
	m.sim.SetConfig(cfg)
	return nil
}

func (m *magicPortalRuntime) AddEntropy(delta int64) { m.sim.PerturbRNG(delta) }
