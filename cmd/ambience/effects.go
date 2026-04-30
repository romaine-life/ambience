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

// proceduralRuntime is the shared backing for every effect implemented as a
// sim.Procedural variant (snow, autumn-leaves, starfield, ...). The schema
// function is supplied per-effect so the runtime doesn't need a switch on
// kind — each new procedural effect plugs in via newProceduralEffectDef().
type proceduralRuntime struct {
	kind       string
	schemaFunc func() sim.EffectSchema
	sim        *sim.Procedural
}

func newProceduralRuntime(kind string, schemaFunc func() sim.EffectSchema, w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.ProceduralConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode %s config: %w", kind, err)
		}
	}
	return &proceduralRuntime{
		kind:       kind,
		schemaFunc: schemaFunc,
		sim:        sim.NewProcedural(kind, w, h, seed, parsed),
	}, nil
}

// newProceduralEffectDef wraps a procedural-backed effect for self-registration.
// A per-effect file becomes one init() line:
//
//	func init() { register(newProceduralEffectDef("snow", sim.SnowSchema)) }
func newProceduralEffectDef(kind string, schema func() sim.EffectSchema) effectDefinition {
	return effectDefinition{
		Type:   kind,
		Schema: schema,
		NewRuntime: func(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
			return newProceduralRuntime(kind, schema, w, h, seed, cfg)
		},
	}
}

func (p *proceduralRuntime) Type() string             { return p.kind }
func (p *proceduralRuntime) Schema() sim.EffectSchema { return p.schemaFunc() }

func (p *proceduralRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(p.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := p.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  p.sim.W,
		GridH:  p.sim.H,
	}, nil
}

func (p *proceduralRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.ProceduralSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode %s snapshot: %w", p.kind, err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestoreSnapshot(state)
	return nil
}

func (p *proceduralRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(p.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(p.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  p.sim.W,
		GridH:  p.sim.H,
	}, nil
}

func (p *proceduralRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.ProceduralPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode %s persisted state: %w", p.kind, err)
	}
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestorePersistedState(state)
	return nil
}

func (p *proceduralRuntime) Trigger(name string) bool { return p.sim.TriggerEvent(name) }

func (p *proceduralRuntime) Step() { p.sim.Step() }

func (p *proceduralRuntime) CurrentTick() int { return p.sim.CurrentTick() }

func (p *proceduralRuntime) DrainLog() []sim.LogEntry { return p.sim.DrainLog() }

func (p *proceduralRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.ProceduralConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode %s config: %w", p.kind, err)
		}
	}
	p.sim.SetConfig(cfg)
	return nil
}

func (p *proceduralRuntime) AddEntropy(delta int64) { p.sim.PerturbRNG(delta) }
