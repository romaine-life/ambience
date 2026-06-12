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
		Type:         "paper-lanterns",
		Schema:       sim.PaperLanternsSchema,
		NewRuntime:   newPaperLanternsRuntime,
		NewScene:     generatePaperLanternsScene,
		NewNearScene: generatePaperLanternsSceneNear,
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

func (p *paperLanternsRuntime) Type() string { return "paper-lanterns" }

func (p *paperLanternsRuntime) Schema() sim.EffectSchema { return sim.PaperLanternsSchema() }

func (p *paperLanternsRuntime) Snapshot() (effectEnvelope, error) {
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

func (p *paperLanternsRuntime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
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
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestoreSnapshot(state)
	return nil
}

func (p *paperLanternsRuntime) Persisted() (persistedEffectState, error) {
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

func (p *paperLanternsRuntime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := p.ApplyConfig(s.Config); err != nil {
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
	if s.GridW > 0 && s.GridH > 0 && (p.sim.W != s.GridW || p.sim.H != s.GridH) {
		p.sim.Resize(s.GridW, s.GridH)
	}
	p.sim.RestorePersistedState(state)
	return nil
}

func (p *paperLanternsRuntime) Trigger(name string) bool { return p.sim.TriggerEvent(name) }

func (p *paperLanternsRuntime) Frame() [][]sim.Pixel { return p.sim.GridCopy() }

func (p *paperLanternsRuntime) Step() { p.sim.Step() }

func (p *paperLanternsRuntime) CurrentTick() int { return p.sim.CurrentTick() }

func (p *paperLanternsRuntime) DrainLog() []sim.LogEntry { return p.sim.DrainLog() }

func (p *paperLanternsRuntime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.PaperLanternsConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode paper-lanterns config: %w", err)
		}
	}
	p.sim.SetConfig(cfg)
	return nil
}

func (p *paperLanternsRuntime) AddEntropy(delta int64) { p.sim.PerturbRNG(delta) }

func (p *paperLanternsRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 4
}

func (p *paperLanternsRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.PaperLanternsConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode paper-lanterns transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode paper-lanterns transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpPaperLanternsConfig(sim.NormalizePaperLanternsConfig(from), sim.NormalizePaperLanternsConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type paperLanternsPalette struct {
	name                      string
	hue, hueSpread            float64
	satMin, satMax            float64
	lightMin, lightMax        float64
	riseMin, riseMax          float64
	windMin, windMax          float64
	spawnMin, spawnMax        int
	clusterMin, clusterMax    int
	releaseMin, releaseMax    float64
	glowMin, glowMax          float64
	quietChance, windChance   float64
	layerBalanceMin, layerMax float64
}

var paperLanternsPalettes = []paperLanternsPalette{
	{
		name: "spirit's eve", hue: 38, hueSpread: 10,
		satMin: 0.56, satMax: 0.74, lightMin: 0.38, lightMax: 0.9,
		riseMin: 0.10, riseMax: 0.15, windMin: -0.28, windMax: 0.28,
		spawnMin: 32, spawnMax: 50, clusterMin: 5, clusterMax: 9,
		releaseMin: 0.0014, releaseMax: 0.0026, glowMin: 0.68, glowMax: 0.9,
		quietChance: 0.00015, windChance: 0.00035, layerBalanceMin: 0.28, layerMax: 0.42,
	},
	{
		name: "temple festival", hue: 30, hueSpread: 18,
		satMin: 0.64, satMax: 0.86, lightMin: 0.42, lightMax: 0.95,
		riseMin: 0.12, riseMax: 0.18, windMin: -0.18, windMax: 0.36,
		spawnMin: 24, spawnMax: 38, clusterMin: 6, clusterMax: 11,
		releaseMin: 0.0022, releaseMax: 0.0042, glowMin: 0.78, glowMax: 1.08,
		quietChance: 0.00008, windChance: 0.00045, layerBalanceMin: 0.30, layerMax: 0.46,
	},
	{
		name: "slow drift", hue: 44, hueSpread: 9,
		satMin: 0.45, satMax: 0.62, lightMin: 0.36, lightMax: 0.82,
		riseMin: 0.07, riseMax: 0.11, windMin: -0.12, windMax: 0.18,
		spawnMin: 46, spawnMax: 76, clusterMin: 4, clusterMax: 7,
		releaseMin: 0.0008, releaseMax: 0.0016, glowMin: 0.55, glowMax: 0.78,
		quietChance: 0.00022, windChance: 0.00025, layerBalanceMin: 0.34, layerMax: 0.52,
	},
	{
		name: "lantern flock", hue: 34, hueSpread: 14,
		satMin: 0.58, satMax: 0.82, lightMin: 0.40, lightMax: 0.94,
		riseMin: 0.11, riseMax: 0.16, windMin: -0.34, windMax: 0.34,
		spawnMin: 18, spawnMax: 30, clusterMin: 8, clusterMax: 14,
		releaseMin: 0.0028, releaseMax: 0.0052, glowMin: 0.72, glowMax: 1.0,
		quietChance: 0.00006, windChance: 0.0005, layerBalanceMin: 0.22, layerMax: 0.38,
	},
}

func generatePaperLanternsScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := paperLanternsPalettes[rng.Intn(len(paperLanternsPalettes))]
	cfg := randomPaperLanternsConfig(rng, palette)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForPaperLanternsConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generatePaperLanternsSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generatePaperLanternsScene(rng, startedAt, durationTicks)
	var prev, target sim.PaperLanternsConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpPaperLanternsConfig(sim.NormalizePaperLanternsConfig(prev), sim.NormalizePaperLanternsConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForPaperLanternsConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func randomPaperLanternsConfig(rng *rngutil.RNG, p paperLanternsPalette) sim.PaperLanternsConfig {
	intRange := func(minValue, maxValue int) int {
		if maxValue <= minValue {
			return minValue
		}
		return minValue + rng.Intn(maxValue-minValue+1)
	}
	floatRange := func(minValue, maxValue float64) float64 {
		if maxValue <= minValue {
			return minValue
		}
		return minValue + rng.Float64()*(maxValue-minValue)
	}
	clusterMin := intRange(p.clusterMin, max(p.clusterMin, p.clusterMax-3))
	clusterMax := intRange(max(clusterMin, p.clusterMax-2), p.clusterMax)
	return sim.PaperLanternsConfig{
		IntroFirstDelay:   intRange(12, 34),
		IntroClusterDelay: intRange(70, 115),
		EndingTail:        intRange(440, 720),
		SpawnEvery:        intRange(p.spawnMin, p.spawnMax),
		MaxLanterns:       intRange(58, 104),
		ClusterMin:        clusterMin,
		ClusterMax:        clusterMax,
		PulseWindow:       intRange(16, 30),
		RiseSpeed:         floatRange(p.riseMin, p.riseMax),
		SpeedJitter:       floatRange(0.16, 0.34),
		Wind:              floatRange(p.windMin, p.windMax),
		WindShift:         floatRange(0.32, 0.82),
		Sway:              floatRange(0.32, 0.72),
		Size:              floatRange(0.88, 1.18),
		FadeAltitude:      floatRange(0.22, 0.34),
		Glow:              floatRange(p.glowMin, p.glowMax),
		Layers:            2,
		LayerBalance:      floatRange(p.layerBalanceMin, p.layerMax),
		Hue:               math.Mod(p.hue+(rng.Float64()*2-1)*p.hueSpread*0.35+360, 360),
		HueSpread:         floatRange(math.Max(4, p.hueSpread*0.65), p.hueSpread*1.15),
		Saturation:        floatRange(p.satMin, p.satMax),
		LightnessMin:      p.lightMin,
		LightnessMax:      p.lightMax,
		ReleaseChance:     floatRange(p.releaseMin, p.releaseMax),
		WindDriftChance:   p.windChance * floatRange(0.75, 1.25),
		QuietChance:       p.quietChance * floatRange(0.7, 1.3),
		WindDriftDur:      intRange(140, 260),
		QuietDur:          intRange(340, 620),
		QuietMult:         floatRange(0.22, 0.48),
	}
}

func nameForPaperLanternsConfig(cfg sim.PaperLanternsConfig) string {
	palette := "amber"
	switch {
	case cfg.Hue < 29:
		palette = "saffron"
	case cfg.Hue < 39:
		palette = "candle"
	case cfg.Hue < 49:
		palette = "gold"
	default:
		palette = "pale"
	}
	pace := "drift"
	switch {
	case cfg.RiseSpeed < 0.1:
		pace = "slow"
	case cfg.RiseSpeed > 0.16:
		pace = "bright"
	}
	release := "lanterns"
	switch {
	case cfg.ReleaseChance > 0.0034 || cfg.ClusterMax >= 11:
		release = "flock"
	case cfg.ReleaseChance < 0.0014:
		release = "sparks"
	}
	return fmt.Sprintf("%s-%s-%s", palette, pace, release)
}

func lerpPaperLanternsConfig(a, b sim.PaperLanternsConfig, t float64) sim.PaperLanternsConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.PaperLanternsConfig{
		IntroFirstDelay:   li(a.IntroFirstDelay, b.IntroFirstDelay),
		IntroClusterDelay: li(a.IntroClusterDelay, b.IntroClusterDelay),
		EndingTail:        li(a.EndingTail, b.EndingTail),
		SpawnEvery:        li(a.SpawnEvery, b.SpawnEvery),
		MaxLanterns:       li(a.MaxLanterns, b.MaxLanterns),
		ClusterMin:        li(a.ClusterMin, b.ClusterMin),
		ClusterMax:        li(a.ClusterMax, b.ClusterMax),
		PulseWindow:       li(a.PulseWindow, b.PulseWindow),
		RiseSpeed:         lf(a.RiseSpeed, b.RiseSpeed),
		SpeedJitter:       lf(a.SpeedJitter, b.SpeedJitter),
		Wind:              lf(a.Wind, b.Wind),
		WindShift:         lf(a.WindShift, b.WindShift),
		Sway:              lf(a.Sway, b.Sway),
		Size:              lf(a.Size, b.Size),
		FadeAltitude:      lf(a.FadeAltitude, b.FadeAltitude),
		Glow:              lf(a.Glow, b.Glow),
		Layers:            li(a.Layers, b.Layers),
		LayerBalance:      lf(a.LayerBalance, b.LayerBalance),
		Hue:               lerpAngle(a.Hue, b.Hue, t),
		HueSpread:         lf(a.HueSpread, b.HueSpread),
		Saturation:        lf(a.Saturation, b.Saturation),
		LightnessMin:      lf(a.LightnessMin, b.LightnessMin),
		LightnessMax:      lf(a.LightnessMax, b.LightnessMax),
		ReleaseChance:     lf(a.ReleaseChance, b.ReleaseChance),
		WindDriftChance:   lf(a.WindDriftChance, b.WindDriftChance),
		QuietChance:       lf(a.QuietChance, b.QuietChance),
		WindDriftDur:      li(a.WindDriftDur, b.WindDriftDur),
		QuietDur:          li(a.QuietDur, b.QuietDur),
		QuietMult:         lf(a.QuietMult, b.QuietMult),
	}
}
