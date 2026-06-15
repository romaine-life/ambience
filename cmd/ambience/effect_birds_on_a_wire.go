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
		Type:         "birds-on-a-wire",
		Schema:       sim.BirdsOnAWireSchema,
		NewRuntime:   newBirdsOnAWireRuntime,
		NewScene:     generateBirdsOnAWireScene,
		NewNearScene: generateBirdsOnAWireSceneNear,
	})
}

type birdsOnAWireRuntime struct {
	sim *sim.BirdsOnAWire
}

func newBirdsOnAWireRuntime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.BirdsOnAWireConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode birds-on-a-wire config: %w", err)
		}
	}
	return &birdsOnAWireRuntime{sim: sim.NewBirdsOnAWire(w, h, seed, parsed)}, nil
}

func (r *birdsOnAWireRuntime) Type() string { return "birds-on-a-wire" }

func (r *birdsOnAWireRuntime) Schema() sim.EffectSchema { return sim.BirdsOnAWireSchema() }

func (r *birdsOnAWireRuntime) Snapshot() (effectEnvelope, error) {
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

func (r *birdsOnAWireRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BirdsOnAWireSnapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode birds-on-a-wire snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *birdsOnAWireRuntime) Persisted() (persistedEffectState, error) {
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

func (r *birdsOnAWireRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.BirdsOnAWirePersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode birds-on-a-wire persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *birdsOnAWireRuntime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *birdsOnAWireRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *birdsOnAWireRuntime) Step() { r.sim.Step() }

func (r *birdsOnAWireRuntime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *birdsOnAWireRuntime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *birdsOnAWireRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.BirdsOnAWireConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode birds-on-a-wire config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *birdsOnAWireRuntime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }

func (r *birdsOnAWireRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 5
}

func (r *birdsOnAWireRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.BirdsOnAWireConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode birds-on-a-wire transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode birds-on-a-wire transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpBirdsOnAWireConfig(normalizeBirdsOnAWireConfig(from), normalizeBirdsOnAWireConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type birdsOnAWirePalette struct {
	name string
	cfg  sim.BirdsOnAWireConfig
}

var birdsOnAWirePalettes = []birdsOnAWirePalette{
	{
		name: "morning wire",
		cfg: sim.BirdsOnAWireConfig{
			SkyHue: 208, SkySat: 0.34, TopLight: 0.24, HorizonLight: 0.62,
			MaxBirds: 8, IntroTarget: 4, ArrivalEvery: 260, PairChance: 0.12,
			TakeoffEvery: 720, FlockChance: 0.18, QuietDur: 900,
		},
	},
	{
		name: "evening commute",
		cfg: sim.BirdsOnAWireConfig{
			SkyHue: 32, SkySat: 0.62, TopLight: 0.19, HorizonLight: 0.62,
			MaxBirds: 16, IntroTarget: 7, ArrivalEvery: 125, PairChance: 0.34,
			BobChance: 0.004, TakeoffEvery: 480, FlockChance: 0.28,
		},
	},
	{
		name: "overcast lull",
		cfg: sim.BirdsOnAWireConfig{
			SkyHue: 220, SkySat: 0.08, TopLight: 0.26, HorizonLight: 0.46,
			MaxBirds: 6, IntroTarget: 3, ArrivalEvery: 420, PairChance: 0.08,
			TakeoffEvery: 940, FlockChance: 0.12, QuietDur: 1320, QuietArrival: 0.06,
		},
	},
	{
		name: "telephone row",
		cfg: sim.BirdsOnAWireConfig{
			SkyHue: 42, SkySat: 0.48, TopLight: 0.18, HorizonLight: 0.58,
			WireCount: 2, WireY: 0.36, WireSag: 3.8, MaxBirds: 18, PerchSpacing: 4.5,
			ArrivalEvery: 150, PairChance: 0.26, TakeoffEvery: 560,
		},
	},
}

func generateBirdsOnAWireScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := birdsOnAWirePalettes[rng.Intn(len(birdsOnAWirePalettes))]
	cfg := randomBirdsOnAWireConfig(rng, palette.cfg)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          palette.name,
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateBirdsOnAWireSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateBirdsOnAWireScene(rng, startedAt, durationTicks)
	var prev, target sim.BirdsOnAWireConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpBirdsOnAWireConfig(normalizeBirdsOnAWireConfig(prev), normalizeBirdsOnAWireConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	random.Config = configData
	random.Name = nameForBirdsOnAWireConfig(cfg)
	return random
}

func randomBirdsOnAWireConfig(rng *rngutil.RNG, base sim.BirdsOnAWireConfig) sim.BirdsOnAWireConfig {
	cfg := normalizeBirdsOnAWireConfig(base)
	jf := func(v, spread float64) float64 {
		return v * (1 + spread*(rng.Float64()*2-1))
	}
	ji := func(v int, spread float64) int {
		return max(1, int(math.Round(float64(v)*(1+spread*(rng.Float64()*2-1)))))
	}
	cfg.SkyHue = math.Mod(cfg.SkyHue+(rng.Float64()*10-5)+360, 360)
	cfg.SkySat = clampUnit(jf(cfg.SkySat, 0.16))
	cfg.TopLight = clampUnit(jf(cfg.TopLight, 0.12))
	cfg.HorizonLight = clampUnit(jf(cfg.HorizonLight, 0.10))
	cfg.WireSag = jf(cfg.WireSag, 0.18)
	cfg.MaxBirds = ji(cfg.MaxBirds, 0.18)
	cfg.IntroTarget = ji(cfg.IntroTarget, 0.20)
	cfg.ArrivalEvery = ji(cfg.ArrivalEvery, 0.22)
	cfg.PairChance = clampUnit(jf(cfg.PairChance, 0.25))
	cfg.TakeoffEvery = ji(cfg.TakeoffEvery, 0.20)
	cfg.FlockChance = clampUnit(jf(cfg.FlockChance, 0.22))
	cfg.QuietDur = ji(cfg.QuietDur, 0.18)
	cfg.QuietArrival = clampUnit(jf(cfg.QuietArrival, 0.20))
	return normalizeBirdsOnAWireConfig(cfg)
}

func nameForBirdsOnAWireConfig(cfg sim.BirdsOnAWireConfig) string {
	cfg = normalizeBirdsOnAWireConfig(cfg)
	switch {
	case cfg.WireCount > 1:
		return "telephone row"
	case cfg.SkySat < 0.16:
		return "overcast lull"
	case cfg.SkyHue >= 180 && cfg.SkyHue <= 240:
		return "morning wire"
	case cfg.MaxBirds >= 14 || cfg.ArrivalEvery < 160:
		return "evening commute"
	default:
		return "evening wire"
	}
}

func normalizeBirdsOnAWireConfig(cfg sim.BirdsOnAWireConfig) sim.BirdsOnAWireConfig {
	return sim.NewBirdsOnAWire(1, 1, 1, cfg).EffectiveConfig()
}

func lerpBirdsOnAWireConfig(a, b sim.BirdsOnAWireConfig, t float64) sim.BirdsOnAWireConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.BirdsOnAWireConfig{
		IntroDur:          li(a.IntroDur, b.IntroDur),
		IntroLandEvery:    li(a.IntroLandEvery, b.IntroLandEvery),
		IntroTarget:       li(a.IntroTarget, b.IntroTarget),
		EndingDur:         li(a.EndingDur, b.EndingDur),
		OutroTakeoffEvery: li(a.OutroTakeoffEvery, b.OutroTakeoffEvery),
		ResidualLife:      li(a.ResidualLife, b.ResidualLife),
		SkyHue:            lerpAngle(a.SkyHue, b.SkyHue, t),
		SkySat:            lf(a.SkySat, b.SkySat),
		TopLight:          lf(a.TopLight, b.TopLight),
		HorizonLight:      lf(a.HorizonLight, b.HorizonLight),
		HorizonY:          lf(a.HorizonY, b.HorizonY),
		WireY:             lf(a.WireY, b.WireY),
		WireSag:           lf(a.WireSag, b.WireSag),
		WireCount:         li(a.WireCount, b.WireCount),
		MaxBirds:          li(a.MaxBirds, b.MaxBirds),
		PerchSpacing:      lf(a.PerchSpacing, b.PerchSpacing),
		ArrivalEvery:      li(a.ArrivalEvery, b.ArrivalEvery),
		PairChance:        lf(a.PairChance, b.PairChance),
		BobChance:         lf(a.BobChance, b.BobChance),
		TakeoffEvery:      li(a.TakeoffEvery, b.TakeoffEvery),
		FlockChance:       lf(a.FlockChance, b.FlockChance),
		QuietDur:          li(a.QuietDur, b.QuietDur),
		QuietArrival:      lf(a.QuietArrival, b.QuietArrival),
	}
}
