package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "lava-lamp",
		Schema:     sim.LavaLampSchema,
		NewRuntime: newLavaLampRuntime,
	})
}

type lavaLampRuntime struct {
	sim *sim.LavaLamp
}

func newLavaLampRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.LavaLampConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode lava-lamp config: %w", err)
		}
	}
	return &lavaLampRuntime{sim: sim.NewLavaLamp(w, h, seed, parsed)}, nil
}

func (l *lavaLampRuntime) Type() string { return "lava-lamp" }

func (l *lavaLampRuntime) Schema() sim.EffectSchema { return sim.LavaLampSchema() }

func (l *lavaLampRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(l.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := l.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  l.sim.W,
		GridH:  l.sim.H,
	}, nil
}

func (l *lavaLampRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := l.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.LavaLampSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode lava-lamp snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (l.sim.W != s.GridW || l.sim.H != s.GridH) {
		l.sim.Resize(s.GridW, s.GridH)
	}
	l.sim.RestoreSnapshot(state)
	return nil
}

func (l *lavaLampRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(l.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(l.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  l.sim.W,
		GridH:  l.sim.H,
	}, nil
}

func (l *lavaLampRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := l.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.LavaLampPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode lava-lamp persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (l.sim.W != s.GridW || l.sim.H != s.GridH) {
		l.sim.Resize(s.GridW, s.GridH)
	}
	l.sim.RestorePersistedState(state)
	return nil
}

func (l *lavaLampRuntime) Trigger(name string) bool { return l.sim.TriggerEvent(name) }

func (l *lavaLampRuntime) Frame() [][]sim.Pixel { return l.sim.GridCopy() }

func (l *lavaLampRuntime) Step() { l.sim.Step() }

func (l *lavaLampRuntime) CurrentTick() int { return l.sim.CurrentTick() }

func (l *lavaLampRuntime) DrainLog() []sim.LogEntry { return l.sim.DrainLog() }

func (l *lavaLampRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.LavaLampConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode lava-lamp config: %w", err)
		}
	}
	l.sim.SetConfig(cfg)
	return nil
}

func (l *lavaLampRuntime) AddEntropy(delta int64) { l.sim.PerturbRNG(delta) }
