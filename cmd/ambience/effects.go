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
	"dust": {
		Type:       "dust",
		Schema:     sim.DustSchema,
		NewRuntime: newDustRuntime,
	},
	"fireflies": {
		Type:       "fireflies",
		Schema:     sim.FirefliesSchema,
		NewRuntime: newFirefliesRuntime,
	},
	"waterfall": {
		Type:       "waterfall",
		Schema:     sim.WaterfallSchema,
		NewRuntime: newWaterfallRuntime,
	},
	"snow": {
		Type:       "snow",
		Schema:     sim.SnowSchema,
		NewRuntime: newSnowRuntime,
	},
	"autumn-leaves": {
		Type:       "autumn-leaves",
		Schema:     sim.AutumnLeavesSchema,
		NewRuntime: newAutumnLeavesRuntime,
	},
	"starfield": {
		Type:       "starfield",
		Schema:     sim.StarfieldSchema,
		NewRuntime: newStarfieldRuntime,
	},
	"aurora": {
		Type:       "aurora",
		Schema:     sim.AuroraSchema,
		NewRuntime: newAuroraRuntime,
	},
	"wheat-field": {
		Type:       "wheat-field",
		Schema:     sim.WheatFieldSchema,
		NewRuntime: newWheatFieldRuntime,
	},
	"beach": {
		Type:       "beach",
		Schema:     sim.BeachSchema,
		NewRuntime: newBeachRuntime,
	},
	"campfire": {
		Type:       "campfire",
		Schema:     sim.CampfireSchema,
		NewRuntime: newCampfireRuntime,
	},
	"windmill": {
		Type:       "windmill",
		Schema:     sim.WindmillSchema,
		NewRuntime: newWindmillRuntime,
	},
	"lighthouse": {
		Type:       "lighthouse",
		Schema:     sim.LighthouseSchema,
		NewRuntime: newLighthouseRuntime,
	},
	"rowboat": {
		Type:       "rowboat",
		Schema:     sim.RowboatSchema,
		NewRuntime: newRowboatRuntime,
	},
	"underwater": {
		Type:       "underwater",
		Schema:     sim.UnderwaterSchema,
		NewRuntime: newUnderwaterRuntime,
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

type dustRuntime struct {
	sim *sim.Dust
}

type firefliesRuntime struct {
	sim *sim.Fireflies
}

type waterfallRuntime struct {
	sim *sim.Waterfall
}

type proceduralRuntime struct {
	kind string
	sim  *sim.Procedural
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

func newDustRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.DustConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode dust config: %w", err)
		}
	}
	return &dustRuntime{sim: sim.NewDust(w, h, seed, parsed)}, nil
}

func newFirefliesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.FirefliesConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode fireflies config: %w", err)
		}
	}
	return &firefliesRuntime{sim: sim.NewFireflies(w, h, seed, parsed)}, nil
}

func newWaterfallRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.WaterfallConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode waterfall config: %w", err)
		}
	}
	return &waterfallRuntime{sim: sim.NewWaterfall(w, h, seed, parsed)}, nil
}

func newProceduralRuntime(kind string, w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.ProceduralConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode %s config: %w", kind, err)
		}
	}
	return &proceduralRuntime{
		kind: kind,
		sim:  sim.NewProcedural(kind, w, h, seed, parsed),
	}, nil
}

func newSnowRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("snow", w, h, seed, cfg)
}

func newAutumnLeavesRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("autumn-leaves", w, h, seed, cfg)
}

func newStarfieldRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("starfield", w, h, seed, cfg)
}

func newAuroraRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("aurora", w, h, seed, cfg)
}

func newWheatFieldRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("wheat-field", w, h, seed, cfg)
}

func newBeachRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("beach", w, h, seed, cfg)
}

func newCampfireRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("campfire", w, h, seed, cfg)
}

func newWindmillRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("windmill", w, h, seed, cfg)
}

func newLighthouseRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("lighthouse", w, h, seed, cfg)
}

func newRowboatRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("rowboat", w, h, seed, cfg)
}

func newUnderwaterRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	return newProceduralRuntime("underwater", w, h, seed, cfg)
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

func (d *dustRuntime) Type() string { return "dust" }

func (d *dustRuntime) Schema() sim.EffectSchema { return sim.DustSchema() }

func (d *dustRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(d.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := d.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  d.sim.W,
		GridH:  d.sim.H,
	}, nil
}

func (d *dustRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := d.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DustSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode dust snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (d.sim.W != s.GridW || d.sim.H != s.GridH) {
		d.sim.Resize(s.GridW, s.GridH)
	}
	d.sim.RestoreSnapshot(state)
	return nil
}

func (d *dustRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(d.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(d.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  d.sim.W,
		GridH:  d.sim.H,
	}, nil
}

func (d *dustRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := d.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.DustPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode dust persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (d.sim.W != s.GridW || d.sim.H != s.GridH) {
		d.sim.Resize(s.GridW, s.GridH)
	}
	d.sim.RestorePersistedState(state)
	return nil
}

func (d *dustRuntime) Trigger(name string) bool { return d.sim.TriggerEvent(name) }

func (d *dustRuntime) Step() { d.sim.Step() }

func (d *dustRuntime) CurrentTick() int { return d.sim.CurrentTick() }

func (d *dustRuntime) DrainLog() []sim.LogEntry { return d.sim.DrainLog() }

func (d *dustRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.DustConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode dust config: %w", err)
		}
	}
	d.sim.SetConfig(cfg)
	return nil
}

func (d *dustRuntime) AddEntropy(delta int64) { d.sim.PerturbRNG(delta) }

func (f *firefliesRuntime) Type() string { return "fireflies" }

func (f *firefliesRuntime) Schema() sim.EffectSchema { return sim.FirefliesSchema() }

func (f *firefliesRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(f.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := f.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  f.sim.W,
		GridH:  f.sim.H,
	}, nil
}

func (f *firefliesRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := f.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.FirefliesSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode fireflies snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (f.sim.W != s.GridW || f.sim.H != s.GridH) {
		f.sim.Resize(s.GridW, s.GridH)
	}
	f.sim.RestoreSnapshot(state)
	return nil
}

func (f *firefliesRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(f.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(f.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  f.sim.W,
		GridH:  f.sim.H,
	}, nil
}

func (f *firefliesRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := f.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.FirefliesPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode fireflies persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (f.sim.W != s.GridW || f.sim.H != s.GridH) {
		f.sim.Resize(s.GridW, s.GridH)
	}
	f.sim.RestorePersistedState(state)
	return nil
}

func (f *firefliesRuntime) Trigger(name string) bool { return f.sim.TriggerEvent(name) }

func (f *firefliesRuntime) Step() { f.sim.Step() }

func (f *firefliesRuntime) CurrentTick() int { return f.sim.CurrentTick() }

func (f *firefliesRuntime) DrainLog() []sim.LogEntry { return f.sim.DrainLog() }

func (f *firefliesRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.FirefliesConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode fireflies config: %w", err)
		}
	}
	f.sim.SetConfig(cfg)
	return nil
}

func (f *firefliesRuntime) AddEntropy(delta int64) { f.sim.PerturbRNG(delta) }

func (w *waterfallRuntime) Type() string { return "waterfall" }

func (w *waterfallRuntime) Schema() sim.EffectSchema { return sim.WaterfallSchema() }

func (w *waterfallRuntime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(w.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := w.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  w.sim.W,
		GridH:  w.sim.H,
	}, nil
}

func (w *waterfallRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := w.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterfallSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode waterfall snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (w.sim.W != s.GridW || w.sim.H != s.GridH) {
		w.sim.Resize(s.GridW, s.GridH)
	}
	w.sim.RestoreSnapshot(state)
	return nil
}

func (w *waterfallRuntime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(w.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(w.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  w.sim.W,
		GridH:  w.sim.H,
	}, nil
}

func (w *waterfallRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := w.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.WaterfallPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode waterfall persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (w.sim.W != s.GridW || w.sim.H != s.GridH) {
		w.sim.Resize(s.GridW, s.GridH)
	}
	w.sim.RestorePersistedState(state)
	return nil
}

func (w *waterfallRuntime) Trigger(name string) bool { return w.sim.TriggerEvent(name) }

func (w *waterfallRuntime) Step() { w.sim.Step() }

func (w *waterfallRuntime) CurrentTick() int { return w.sim.CurrentTick() }

func (w *waterfallRuntime) DrainLog() []sim.LogEntry { return w.sim.DrainLog() }

func (w *waterfallRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.WaterfallConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode waterfall config: %w", err)
		}
	}
	w.sim.SetConfig(cfg)
	return nil
}

func (w *waterfallRuntime) AddEntropy(delta int64) { w.sim.PerturbRNG(delta) }

func (p *proceduralRuntime) Type() string { return p.kind }

func (p *proceduralRuntime) Schema() sim.EffectSchema {
	switch p.kind {
	case "snow":
		return sim.SnowSchema()
	case "autumn-leaves":
		return sim.AutumnLeavesSchema()
	case "starfield":
		return sim.StarfieldSchema()
	case "aurora":
		return sim.AuroraSchema()
	case "wheat-field":
		return sim.WheatFieldSchema()
	case "beach":
		return sim.BeachSchema()
	case "campfire":
		return sim.CampfireSchema()
	case "windmill":
		return sim.WindmillSchema()
	case "lighthouse":
		return sim.LighthouseSchema()
	case "rowboat":
		return sim.RowboatSchema()
	case "underwater":
		return sim.UnderwaterSchema()
	default:
		return sim.EffectSchema{}
	}
}

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
