package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:         "spider-web",
		Schema:       sim.SpiderWebSchema,
		NewRuntime:   newSpiderWebRuntime,
		NewScene:     generateSpiderWebScene,
		NewNearScene: generateSpiderWebSceneNear,
	})
}

type spiderWebRuntime struct {
	sim *sim.SpiderWeb
}

func newSpiderWebRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.SpiderWebConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode spider-web config: %w", err)
		}
	}
	return &spiderWebRuntime{sim: sim.NewSpiderWeb(w, h, seed, parsed)}, nil
}

func (r *spiderWebRuntime) Type() string { return "spider-web" }

func (r *spiderWebRuntime) Schema() sim.EffectSchema { return sim.SpiderWebSchema() }

func (r *spiderWebRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *spiderWebRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.SpiderWebSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode spider-web snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *spiderWebRuntime) Persisted() (persistedEffectState, error) {
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

func (r *spiderWebRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.SpiderWebPersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode spider-web persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *spiderWebRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *spiderWebRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *spiderWebRuntime) Step() { r.sim.Step() }

func (r *spiderWebRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *spiderWebRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *spiderWebRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.SpiderWebConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode spider-web config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *spiderWebRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *spiderWebRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 10
}

func (r *spiderWebRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.SpiderWebConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode spider-web transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode spider-web transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpSpiderWebConfig(sim.NormalizeSpiderWebConfig(from), sim.NormalizeSpiderWebConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func generateSpiderWebScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := rng.Intn(4)
	cfg := sim.SpiderWebConfig{
		Palette:   palette,
		IntroDur:  120,
		EndingDur: 240,
	}
	switch palette {
	case 1:
		cfg.DropletShimmer = 0.70 + rng.Float64()*0.45
		cfg.GlintRate = 0.42 + rng.Float64()*0.42
		cfg.MoveChance = 0.014 + rng.Float64()*0.024
		cfg.WebSway = 0.18 + rng.Float64()*0.42
	case 2:
		cfg.DropletShimmer = 0.82 + rng.Float64()*0.62
		cfg.GlintRate = 0.56 + rng.Float64()*0.58
		cfg.MoveChance = 0.020 + rng.Float64()*0.038
		cfg.WebSway = 0.42 + rng.Float64()*0.62
	case 3:
		cfg.DropletShimmer = 0.34 + rng.Float64()*0.54
		cfg.GlintRate = 0.30 + rng.Float64()*0.48
		cfg.MoveChance = 0.010 + rng.Float64()*0.026
		cfg.WebSway = 0.12 + rng.Float64()*0.36
	default:
		cfg.DropletShimmer = 1.02 + rng.Float64()*0.62
		cfg.GlintRate = 0.76 + rng.Float64()*0.62
		cfg.MoveChance = 0.020 + rng.Float64()*0.034
		cfg.WebSway = 0.32 + rng.Float64()*0.56
	}
	cfg = sim.NormalizeSpiderWebConfig(cfg)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForSpiderWebConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateSpiderWebSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateSpiderWebScene(rng, startedAt, durationTicks)
	var prev, target sim.SpiderWebConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	prev = sim.NormalizeSpiderWebConfig(prev)
	target = sim.NormalizeSpiderWebConfig(target)
	if target.Palette == prev.Palette && rng.Float64() < 0.55 {
		target.Palette = (prev.Palette + 1 + rng.Intn(3)) % 4
	}
	t := math.Max(0.25, clampUnit(variation))
	cfg := lerpSpiderWebConfig(prev, target, t)
	if rng.Float64() < 0.30+0.50*t {
		cfg.Palette = target.Palette
	} else {
		cfg.Palette = prev.Palette
	}
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForSpiderWebConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func lerpSpiderWebConfig(a, b sim.SpiderWebConfig, t float64) sim.SpiderWebConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(math.Round(float64(y-x)*t)) }
	palette := a.Palette
	if t >= 0.5 {
		palette = b.Palette
	}
	return sim.NormalizeSpiderWebConfig(sim.SpiderWebConfig{
		DropletShimmer: lf(a.DropletShimmer, b.DropletShimmer),
		GlintRate:      lf(a.GlintRate, b.GlintRate),
		MoveChance:     lf(a.MoveChance, b.MoveChance),
		WebSway:        lf(a.WebSway, b.WebSway),
		Palette:        palette,
		IntroDur:       li(a.IntroDur, b.IntroDur),
		EndingDur:      li(a.EndingDur, b.EndingDur),
	})
}

func nameForSpiderWebConfig(cfg sim.SpiderWebConfig) string {
	cfg = sim.NormalizeSpiderWebConfig(cfg)
	base := []string{"dawn-dew", "moonlit-silver", "autumn-gold", "misty"}
	palette := cfg.Palette
	if palette < 0 || palette >= len(base) {
		palette = 0
	}
	switch {
	case cfg.GlintRate >= 1.15:
		return "jeweled-" + base[palette]
	case cfg.WebSway >= 0.95:
		return "breezy-" + base[palette]
	default:
		return base[palette]
	}
}
