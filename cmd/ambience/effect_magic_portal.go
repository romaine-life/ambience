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
		Type:         "magic-portal",
		Schema:       sim.MagicPortalSchema,
		NewRuntime:   newMagicPortalRuntime,
		NewScene:     generateMagicPortalScene,
		NewNearScene: generateMagicPortalSceneNear,
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

func (m *magicPortalRuntime) SceneTransitionTicks(durationTicks int) int {
	if durationTicks <= 0 {
		return 0
	}
	return durationTicks / 3
}

func (m *magicPortalRuntime) InterpolateConfig(fromData, toData json.RawMessage, progress float64) (json.RawMessage, error) {
	var from, to sim.MagicPortalConfig
	if len(fromData) > 0 {
		if err := json.Unmarshal(fromData, &from); err != nil {
			return nil, fmt.Errorf("decode magic-portal transition from config: %w", err)
		}
	}
	if len(toData) > 0 {
		if err := json.Unmarshal(toData, &to); err != nil {
			return nil, fmt.Errorf("decode magic-portal transition to config: %w", err)
		}
	}
	data, err := json.Marshal(lerpMagicPortalConfig(normalizeMagicPortalConfig(from), normalizeMagicPortalConfig(to), progress))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type magicPortalPalette struct {
	name                         string
	hue, hueSpread               float64
	satMin, satMax               float64
	lightMin, lightMax           float64
	pulseMin, pulseMax           int
	ampMin, ampMax               float64
	glowMin, glowMax             float64
	emberMin, emberMax           int
	surgeChance, burstChance     float64
	runeShiftChance, quietChance float64
}

var magicPortalPalettes = []magicPortalPalette{
	{
		name: "arcane blue", hue: 208, hueSpread: 16,
		satMin: 0.64, satMax: 0.84, lightMin: 0.10, lightMax: 0.88,
		pulseMin: 185, pulseMax: 245, ampMin: 0.60, ampMax: 0.86, glowMin: 0.66, glowMax: 0.88,
		emberMin: 2, emberMax: 4, surgeChance: 0.00018, burstChance: 0.0009, runeShiftChance: 0.0008, quietChance: 0.00012,
	},
	{
		name: "infernal red", hue: 8, hueSpread: 18,
		satMin: 0.68, satMax: 0.90, lightMin: 0.10, lightMax: 0.86,
		pulseMin: 155, pulseMax: 220, ampMin: 0.70, ampMax: 0.96, glowMin: 0.74, glowMax: 0.98,
		emberMin: 3, emberMax: 6, surgeChance: 0.00025, burstChance: 0.0014, runeShiftChance: 0.0009, quietChance: 0.00008,
	},
	{
		name: "ancient amber", hue: 40, hueSpread: 14,
		satMin: 0.48, satMax: 0.68, lightMin: 0.12, lightMax: 0.82,
		pulseMin: 210, pulseMax: 285, ampMin: 0.52, ampMax: 0.76, glowMin: 0.56, glowMax: 0.78,
		emberMin: 2, emberMax: 4, surgeChance: 0.00014, burstChance: 0.00075, runeShiftChance: 0.0011, quietChance: 0.00016,
	},
	{
		name: "dormant", hue: 218, hueSpread: 10,
		satMin: 0.05, satMax: 0.18, lightMin: 0.08, lightMax: 0.55,
		pulseMin: 275, pulseMax: 380, ampMin: 0.22, ampMax: 0.44, glowMin: 0.24, glowMax: 0.48,
		emberMin: 1, emberMax: 2, surgeChance: 0.00006, burstChance: 0.00035, runeShiftChance: 0.00055, quietChance: 0.00035,
	},
}

func generateMagicPortalScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	palette := magicPortalPalettes[rng.Intn(len(magicPortalPalettes))]
	cfg := randomMagicPortalConfig(rng, palette)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          nameForMagicPortalConfig(cfg),
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateMagicPortalSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateMagicPortalScene(rng, startedAt, durationTicks)
	var prev, target sim.MagicPortalConfig
	if err := json.Unmarshal(previousConfig, &prev); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpMagicPortalConfig(normalizeMagicPortalConfig(prev), normalizeMagicPortalConfig(target), clampUnit(variation))
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForMagicPortalConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func randomMagicPortalConfig(rng *rngutil.RNG, p magicPortalPalette) sim.MagicPortalConfig {
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
	period := intRange(p.pulseMin, p.pulseMax)
	return sim.MagicPortalConfig{
		IntroResolve: intRange(55, 95),
		IntroFull:    intRange(max(120, period-35), period+45),

		EndingDur:       intRange(90, 145),
		FinalSurgeDur:   intRange(32, 58),
		FinalSurgeInt:   floatRange(1.45, 2.15),
		EndingEmberTail: intRange(55, 95),

		RingScale:   floatRange(0.90, 1.14),
		RingWidth:   floatRange(1.9, 3.2),
		RuneCount:   intRange(11, 18),
		PulsePeriod: period,
		PulseAmp:    floatRange(p.ampMin, p.ampMax),
		Glow:        floatRange(p.glowMin, p.glowMax),
		EmberRate:   intRange(p.emberMin, p.emberMax),

		Hue:      math.Mod(p.hue+(rng.Float64()*2-1)*p.hueSpread+360, 360),
		Sat:      floatRange(p.satMin, p.satMax),
		LightMin: p.lightMin,
		LightMax: p.lightMax,

		SurgeChance:      p.surgeChance * floatRange(0.75, 1.25),
		EmberBurstChance: p.burstChance * floatRange(0.75, 1.25),
		RuneShiftChance:  p.runeShiftChance * floatRange(0.75, 1.25),
		QuietChance:      p.quietChance * floatRange(0.75, 1.25),

		PulsePeakDur: intRange(max(18, period/10), max(28, period/6)),
		SurgeDur:     intRange(75, 130),
		SurgeMult:    floatRange(1.45, 2.05),
		BurstEmbers:  intRange(8, 16),
		RuneShiftDur: intRange(38, 72),
		QuietDur:     intRange(220, 380),
		QuietMult:    floatRange(0.22, 0.42),
		EmberLife:    intRange(70, 125),
	}
}

func nameForMagicPortalConfig(cfg sim.MagicPortalConfig) string {
	palette := "arcane"
	switch {
	case cfg.Sat < 0.25:
		palette = "dormant"
	case cfg.Hue < 25 || cfg.Hue >= 345:
		palette = "infernal"
	case cfg.Hue >= 25 && cfg.Hue < 60:
		palette = "ancient"
	case cfg.Hue >= 180 && cfg.Hue < 245:
		palette = "arcane"
	}
	pace := "breathing"
	switch {
	case cfg.PulsePeriod < 180:
		pace = "restless"
	case cfg.PulsePeriod > 285:
		pace = "sleeping"
	}
	return fmt.Sprintf("%s-%s-gate", palette, pace)
}

func normalizeMagicPortalConfig(cfg sim.MagicPortalConfig) sim.MagicPortalConfig {
	return sim.NewMagicPortal(1, 1, 1, cfg).EffectiveConfig()
}

func lerpMagicPortalConfig(a, b sim.MagicPortalConfig, t float64) sim.MagicPortalConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.MagicPortalConfig{
		IntroResolve: li(a.IntroResolve, b.IntroResolve),
		IntroFull:    li(a.IntroFull, b.IntroFull),

		EndingDur:       li(a.EndingDur, b.EndingDur),
		FinalSurgeDur:   li(a.FinalSurgeDur, b.FinalSurgeDur),
		FinalSurgeInt:   lf(a.FinalSurgeInt, b.FinalSurgeInt),
		EndingEmberTail: li(a.EndingEmberTail, b.EndingEmberTail),

		RingScale:   lf(a.RingScale, b.RingScale),
		RingWidth:   lf(a.RingWidth, b.RingWidth),
		RuneCount:   li(a.RuneCount, b.RuneCount),
		PulsePeriod: li(a.PulsePeriod, b.PulsePeriod),
		PulseAmp:    lf(a.PulseAmp, b.PulseAmp),
		Glow:        lf(a.Glow, b.Glow),
		EmberRate:   li(a.EmberRate, b.EmberRate),

		Hue:      lerpAngle(a.Hue, b.Hue, t),
		Sat:      lf(a.Sat, b.Sat),
		LightMin: lf(a.LightMin, b.LightMin),
		LightMax: lf(a.LightMax, b.LightMax),

		SurgeChance:      lf(a.SurgeChance, b.SurgeChance),
		EmberBurstChance: lf(a.EmberBurstChance, b.EmberBurstChance),
		RuneShiftChance:  lf(a.RuneShiftChance, b.RuneShiftChance),
		QuietChance:      lf(a.QuietChance, b.QuietChance),

		PulsePeakDur: li(a.PulsePeakDur, b.PulsePeakDur),
		SurgeDur:     li(a.SurgeDur, b.SurgeDur),
		SurgeMult:    lf(a.SurgeMult, b.SurgeMult),
		BurstEmbers:  li(a.BurstEmbers, b.BurstEmbers),
		RuneShiftDur: li(a.RuneShiftDur, b.RuneShiftDur),
		QuietDur:     li(a.QuietDur, b.QuietDur),
		QuietMult:    lf(a.QuietMult, b.QuietMult),
		EmberLife:    li(a.EmberLife, b.EmberLife),
	}
}
