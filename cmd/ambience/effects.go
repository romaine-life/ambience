package main

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/nelsong6/ambience/sim"
)

// effectEnvelope is the generic server/client wire shape for one effect's
// own state. The outer snapshot envelope adds scene metadata, entropy, seed,
// and the effect type.
type effectEnvelope struct {
	Tick   int             `json:"tick"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	GridW  int             `json:"gridW"`
	GridH  int             `json:"gridH"`
}

type persistedEffectState struct {
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	GridW  int             `json:"gridW"`
	GridH  int             `json:"gridH"`
}

// effectRuntime is the server-side contract a concrete effect implements so
// the HTTP layer can work in terms of "an effect" rather than Rain fields.
type effectRuntime interface {
	Type() string
	Schema() sim.EffectSchema
	Snapshot() (effectEnvelope, error)
	Restore(effectEnvelope) error
	Persisted() (persistedEffectState, error)
	RestorePersisted(persistedEffectState) error
	Trigger(name string) bool

	Step()
	CurrentTick() int
	DrainLog() []sim.LogEntry
	ApplyConfig(json.RawMessage) error
	AddEntropy(int64)
}

type effectDefinition struct {
	Type       string
	Schema     func() sim.EffectSchema
	NewRuntime func(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error)
}

var effectRegistry = map[string]effectDefinition{
	"rain": {
		Type:       "rain",
		Schema:     sim.RainSchema,
		NewRuntime: newRainRuntime,
	},
}

func lookupEffectDefinition(effectType string) (effectDefinition, bool) {
	def, ok := effectRegistry[effectType]
	return def, ok
}

func mustNewEffectRuntime(effectType string, w, h int, seed int64, cfg json.RawMessage) effectRuntime {
	rt, err := newEffectRuntime(effectType, w, h, seed, cfg)
	if err != nil {
		panic(err)
	}
	return rt
}

func newEffectRuntime(effectType string, w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	def, ok := lookupEffectDefinition(effectType)
	if !ok {
		return nil, fmt.Errorf("unknown effect type %q", effectType)
	}
	return def.NewRuntime(w, h, seed, cfg)
}

func schemaForEffect(effectType string) (sim.EffectSchema, bool) {
	def, ok := lookupEffectDefinition(effectType)
	if !ok {
		return sim.EffectSchema{}, false
	}
	return def.Schema(), true
}

func parseEffectConfig(q url.Values, schema sim.EffectSchema) (json.RawMessage, error) {
	cfg := map[string]any{}
	for _, knob := range schema.Knobs {
		raw := q.Get(knob.Key)
		if raw == "" {
			continue
		}
		switch knob.Type {
		case sim.KnobInt:
			var n int
			if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
				return nil, fmt.Errorf("parse %s as int: %w", knob.Key, err)
			}
			cfg[knob.Key] = n
		case sim.KnobFloat:
			var f float64
			if _, err := fmt.Sscanf(raw, "%f", &f); err != nil {
				return nil, fmt.Errorf("parse %s as float: %w", knob.Key, err)
			}
			cfg[knob.Key] = f
		default:
			return nil, fmt.Errorf("unsupported knob type %q for %s", knob.Type, knob.Key)
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func cloneRaw(data json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), data...)
}

type rainRuntime struct {
	sim *sim.Rain
}

func newRainRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.Config
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode rain config: %w", err)
		}
	}
	return &rainRuntime{sim: sim.NewRain(w, h, seed, parsed)}, nil
}

func (r *rainRuntime) Type() string { return "rain" }

func (r *rainRuntime) Schema() sim.EffectSchema { return sim.RainSchema() }

func (r *rainRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *rainRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.RainSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *rainRuntime) Persisted() (persistedEffectState, error) {
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

func (r *rainRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.PersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode rain persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *rainRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *rainRuntime) Step() { r.sim.Step() }

func (r *rainRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *rainRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *rainRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.Config
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rain config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *rainRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
