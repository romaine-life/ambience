package main

import (
	"encoding/json"
	"fmt"

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
	data, err := json.Marshal(lerpPaperLanternsConfig(normalizePaperLanternsConfig(from), normalizePaperLanternsConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type paperLanternsPalette struct {
	name                             string
	hueMin, hueMax                   float64
	satMin, satMax                   float64
	lightMin, lightMax               float64
	windMin, windMax                 float64
	loneMin, loneMax                 int
	releaseMin, releaseMax           int
	clusterMin, clusterMax           int
	riseMin, riseMax                 float64
	sizeMin, sizeMax                 float64
	layerBalanceMin, layerBalanceMax float64
}

var paperLanternsPalettes = []paperLanternsPalette{
	{
		name: "spirit's eve", hueMin: 30, hueMax: 42, satMin: 0.68, satMax: 0.86, lightMin: 0.42, lightMax: 0.78,
		windMin: -0.18, windMax: 0.32, loneMin: 80, loneMax: 125, releaseMin: 330, releaseMax: 510,
		clusterMin: 5, clusterMax: 9, riseMin: 0.064, riseMax: 0.088, sizeMin: 1.35, sizeMax: 1.75,
		layerBalanceMin: 0.36, layerBalanceMax: 0.52,
	},
	{
		name: "temple festival", hueMin: 22, hueMax: 34, satMin: 0.78, satMax: 0.94, lightMin: 0.48, lightMax: 0.86,
		windMin: 0.08, windMax: 0.55, loneMin: 55, loneMax: 90, releaseMin: 250, releaseMax: 380,
		clusterMin: 7, clusterMax: 12, riseMin: 0.074, riseMax: 0.105, sizeMin: 1.45, sizeMax: 2.05,
		layerBalanceMin: 0.28, layerBalanceMax: 0.46,
	},
	{
		name: "slow drift", hueMin: 38, hueMax: 52, satMin: 0.48, satMax: 0.66, lightMin: 0.36, lightMax: 0.68,
		windMin: -0.42, windMax: 0.18, loneMin: 120, loneMax: 190, releaseMin: 540, releaseMax: 840,
		clusterMin: 4, clusterMax: 7, riseMin: 0.044, riseMax: 0.066, sizeMin: 1.1, sizeMax: 1.55,
		layerBalanceMin: 0.46, layerBalanceMax: 0.68,
	},
	{
		name: "lantern flock", hueMin: 28, hueMax: 46, satMin: 0.62, satMax: 0.82, lightMin: 0.44, lightMax: 0.8,
		windMin: -0.12, windMax: 0.45, loneMin: 45, loneMax: 75, releaseMin: 210, releaseMax: 330,
		clusterMin: 9, clusterMax: 15, riseMin: 0.066, riseMax: 0.096, sizeMin: 1.25, sizeMax: 1.8,
		layerBalanceMin: 0.4, layerBalanceMax: 0.62,
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
		Name:          palette.name,
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
	cfg := lerpPaperLanternsConfig(normalizePaperLanternsConfig(prev), normalizePaperLanternsConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	random.Config = configData
	random.Name = nameForPaperLanternsConfig(cfg)
	return random
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
	clusterMin := intRange(p.clusterMin, p.clusterMax)
	clusterMax := clusterMin + intRange(1, 4)
	return sim.PaperLanternsConfig{
		IntroDur:          intRange(150, 230),
		IntroFirstDelay:   intRange(15, 38),
		IntroClusterDelay: intRange(85, 135),
		EndingStop:        0,
		EndingTail:        intRange(220, 360),

		LoneEvery:     intRange(p.loneMin, p.loneMax),
		ReleaseGap:    intRange(p.releaseMin, p.releaseMax),
		MaxLanterns:   intRange(60, 110),
		ReleaseMin:    clusterMin,
		ReleaseMax:    min(20, clusterMax),
		ReleaseWindow: intRange(16, 34),

		RiseSpeed:   floatRange(p.riseMin, p.riseMax),
		SpeedJitter: floatRange(0.18, 0.36),
		Wind:        floatRange(p.windMin, p.windMax),
		WindJitter:  floatRange(0.14, 0.3),
		Sway:        floatRange(0.42, 0.82),

		Size:      floatRange(p.sizeMin, p.sizeMax),
		FadeStart: floatRange(0.22, 0.36),
		FadeDur:   intRange(75, 125),

		Hue:          floatRange(p.hueMin, p.hueMax),
		HueSpread:    floatRange(7, 18),
		Saturation:   floatRange(p.satMin, p.satMax),
		LightnessMin: p.lightMin,
		LightnessMax: p.lightMax,

		Layers:       2,
		LayerBalance: floatRange(p.layerBalanceMin, p.layerBalanceMax),

		EmitChance:      0,
		ReleaseChance:   0,
		WindDriftChance: floatRange(0.00008, 0.00022),
		FadeChance:      0,
		QuietGapChance:  floatRange(0.00004, 0.00012),

		WindDriftDur:      intRange(150, 280),
		WindDriftStrength: floatRange(0.36, 0.75),
		QuietGapDur:       intRange(220, 420),
	}
}

func nameForPaperLanternsConfig(cfg sim.PaperLanternsConfig) string {
	palette := "amber"
	switch {
	case cfg.Hue < 28:
		palette = "temple"
	case cfg.Hue > 45:
		palette = "gold"
	case cfg.Saturation < 0.6:
		palette = "soft"
	}
	pace := "drift"
	switch {
	case cfg.ReleaseGap < 300:
		pace = "flock"
	case cfg.ReleaseGap > 560:
		pace = "slow"
	}
	return fmt.Sprintf("%s-%s-lanterns", palette, pace)
}

func normalizePaperLanternsConfig(cfg sim.PaperLanternsConfig) sim.PaperLanternsConfig {
	return sim.NewPaperLanterns(1, 1, 1, cfg).EffectiveConfig()
}

func lerpPaperLanternsConfig(a, b sim.PaperLanternsConfig, t float64) sim.PaperLanternsConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.PaperLanternsConfig{
		IntroDur:          li(a.IntroDur, b.IntroDur),
		IntroFirstDelay:   li(a.IntroFirstDelay, b.IntroFirstDelay),
		IntroClusterDelay: li(a.IntroClusterDelay, b.IntroClusterDelay),
		EndingStop:        li(a.EndingStop, b.EndingStop),
		EndingTail:        li(a.EndingTail, b.EndingTail),

		LoneEvery:     li(a.LoneEvery, b.LoneEvery),
		ReleaseGap:    li(a.ReleaseGap, b.ReleaseGap),
		MaxLanterns:   li(a.MaxLanterns, b.MaxLanterns),
		ReleaseMin:    li(a.ReleaseMin, b.ReleaseMin),
		ReleaseMax:    li(a.ReleaseMax, b.ReleaseMax),
		ReleaseWindow: li(a.ReleaseWindow, b.ReleaseWindow),

		RiseSpeed:   lf(a.RiseSpeed, b.RiseSpeed),
		SpeedJitter: lf(a.SpeedJitter, b.SpeedJitter),
		Wind:        lf(a.Wind, b.Wind),
		WindJitter:  lf(a.WindJitter, b.WindJitter),
		Sway:        lf(a.Sway, b.Sway),

		Size:      lf(a.Size, b.Size),
		FadeStart: lf(a.FadeStart, b.FadeStart),
		FadeDur:   li(a.FadeDur, b.FadeDur),

		Hue:          lerpAngle(a.Hue, b.Hue, t),
		HueSpread:    lf(a.HueSpread, b.HueSpread),
		Saturation:   lf(a.Saturation, b.Saturation),
		LightnessMin: lf(a.LightnessMin, b.LightnessMin),
		LightnessMax: lf(a.LightnessMax, b.LightnessMax),

		Layers:       li(a.Layers, b.Layers),
		LayerBalance: lf(a.LayerBalance, b.LayerBalance),

		EmitChance:      lf(a.EmitChance, b.EmitChance),
		ReleaseChance:   lf(a.ReleaseChance, b.ReleaseChance),
		WindDriftChance: lf(a.WindDriftChance, b.WindDriftChance),
		FadeChance:      lf(a.FadeChance, b.FadeChance),
		QuietGapChance:  lf(a.QuietGapChance, b.QuietGapChance),

		WindDriftDur:      li(a.WindDriftDur, b.WindDriftDur),
		WindDriftStrength: lf(a.WindDriftStrength, b.WindDriftStrength),
		QuietGapDur:       li(a.QuietGapDur, b.QuietGapDur),
	}
}
