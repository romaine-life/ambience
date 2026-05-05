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
	Frame() [][]sim.Pixel

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

// effectRegistry is populated at package init time by each effect_*.go file's
// init() calling register(). Splitting registration across per-effect files
// removes the central insertion point that used to make every new-effect PR
// merge-conflict against another in flight.
var effectRegistry = map[string]effectDefinition{}

func register(def effectDefinition) {
	if _, dup := effectRegistry[def.Type]; dup {
		panic(fmt.Sprintf("effect %q registered twice", def.Type))
	}
	effectRegistry[def.Type] = def
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
