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
	data, err := json.Marshal(lerpMagicPortalConfig(
		sim.NormalizeMagicPortalConfig(from),
		sim.NormalizeMagicPortalConfig(to),
		progress,
	))
	if err != nil {
		return nil, err
	}
	return data, nil
}

type magicPortalScenePreset struct {
	name       string
	hue        float64
	sat        float64
	lightMin   float64
	lightMax   float64
	dormant    bool
	pulseMin   int
	pulseMax   int
	emberMin   int
	emberMax   int
	pulseAmpLo float64
	pulseAmpHi float64
	glowLo     float64
	glowHi     float64
}

var magicPortalScenePresets = []magicPortalScenePreset{
	{name: "arcane blue", hue: 208, sat: 0.72, lightMin: 0.12, lightMax: 0.86, pulseMin: 180, pulseMax: 280, emberMin: 2, emberMax: 4, pulseAmpLo: 0.58, pulseAmpHi: 0.86, glowLo: 0.64, glowHi: 0.92},
	{name: "infernal red", hue: 7, sat: 0.84, lightMin: 0.10, lightMax: 0.88, pulseMin: 150, pulseMax: 235, emberMin: 3, emberMax: 5, pulseAmpLo: 0.66, pulseAmpHi: 0.98, glowLo: 0.70, glowHi: 1.02},
	{name: "ancient amber", hue: 42, sat: 0.76, lightMin: 0.11, lightMax: 0.84, pulseMin: 205, pulseMax: 320, emberMin: 2, emberMax: 4, pulseAmpLo: 0.50, pulseAmpHi: 0.78, glowLo: 0.58, glowHi: 0.84},
	{name: "dormant", hue: 218, sat: 0.12, lightMin: 0.08, lightMax: 0.48, dormant: true, pulseMin: 280, pulseMax: 420, emberMin: 1, emberMax: 2, pulseAmpLo: 0.26, pulseAmpHi: 0.48, glowLo: 0.24, glowHi: 0.46},
}

func generateMagicPortalScene(rng *rngutil.RNG, startedAt int, durationTicks int) Scene {
	preset := magicPortalScenePresets[rng.Intn(len(magicPortalScenePresets))]
	cfg := magicPortalSceneConfig(rng, preset)
	configData, _ := json.Marshal(cfg)
	if durationTicks <= 0 {
		durationTicks = sceneDurationTicks(rng)
	}
	return Scene{
		Name:          preset.name,
		Config:        configData,
		DurationTicks: durationTicks,
		StartedAtTick: startedAt,
	}
}

func generateMagicPortalSceneNear(rng *rngutil.RNG, startedAt int, durationTicks int, previousConfig json.RawMessage, variation float64) Scene {
	random := generateMagicPortalScene(rng, startedAt, durationTicks)
	var previous, target sim.MagicPortalConfig
	if err := json.Unmarshal(previousConfig, &previous); err != nil {
		return random
	}
	if err := json.Unmarshal(random.Config, &target); err != nil {
		return random
	}
	cfg := lerpMagicPortalConfig(
		sim.NormalizeMagicPortalConfig(previous),
		sim.NormalizeMagicPortalConfig(target),
		clampUnit(variation),
	)
	configData, _ := json.Marshal(cfg)
	return Scene{
		Name:          nameForMagicPortalConfig(cfg),
		Config:        configData,
		DurationTicks: random.DurationTicks,
		StartedAtTick: startedAt,
	}
}

func magicPortalSceneConfig(rng *rngutil.RNG, preset magicPortalScenePreset) sim.MagicPortalConfig {
	cfg := sim.MagicPortalConfig{
		IntroResolve:     magicPortalRangeInt(rng, 55, 95),
		IntroFull:        magicPortalRangeInt(rng, 150, 240),
		EndingDur:        magicPortalRangeInt(rng, 90, 140),
		FinalSurgeDur:    magicPortalRangeInt(rng, 34, 58),
		FinalSurgeInt:    magicPortalRangeFloat(rng, 1.5, 2.2),
		EndingEmberTail:  magicPortalRangeInt(rng, 60, 105),
		RingScale:        magicPortalRangeFloat(rng, 0.92, 1.12),
		RingWidth:        magicPortalRangeFloat(rng, 2.1, 3.2),
		RuneCount:        magicPortalRangeInt(rng, 12, 18),
		PulsePeriod:      magicPortalRangeInt(rng, preset.pulseMin, preset.pulseMax),
		PulseAmp:         magicPortalRangeFloat(rng, preset.pulseAmpLo, preset.pulseAmpHi),
		Glow:             magicPortalRangeFloat(rng, preset.glowLo, preset.glowHi),
		EmberRate:        magicPortalRangeInt(rng, preset.emberMin, preset.emberMax),
		Hue:              normalizeMagicPortalHue(preset.hue + magicPortalRangeFloat(rng, -6, 6)),
		Sat:              clampUnit(preset.sat + magicPortalRangeFloat(rng, -0.05, 0.05)),
		LightMin:         clampUnit(preset.lightMin + magicPortalRangeFloat(rng, -0.02, 0.02)),
		LightMax:         clampUnit(preset.lightMax + magicPortalRangeFloat(rng, -0.04, 0.04)),
		SurgeChance:      magicPortalRangeFloat(rng, 0.00008, 0.00028),
		EmberBurstChance: magicPortalRangeFloat(rng, 0.00035, 0.0012),
		RuneShiftChance:  magicPortalRangeFloat(rng, 0.00035, 0.0011),
		QuietChance:      magicPortalRangeFloat(rng, 0.00004, 0.00018),
		PulsePeakDur:     magicPortalRangeInt(rng, 22, 34),
		SurgeDur:         magicPortalRangeInt(rng, 72, 120),
		SurgeMult:        magicPortalRangeFloat(rng, 1.45, 1.9),
		BurstEmbers:      magicPortalRangeInt(rng, 8, 14),
		RuneShiftDur:     magicPortalRangeInt(rng, 42, 75),
		QuietDur:         magicPortalRangeInt(rng, 220, 360),
		QuietMult:        magicPortalRangeFloat(rng, 0.25, 0.42),
		EmberLife:        magicPortalRangeInt(rng, 72, 120),
	}
	if preset.dormant {
		cfg.SurgeChance = magicPortalRangeFloat(rng, 0.00002, 0.00009)
		cfg.EmberBurstChance = magicPortalRangeFloat(rng, 0.00012, 0.00045)
		cfg.RuneShiftChance = magicPortalRangeFloat(rng, 0.00015, 0.00055)
		cfg.QuietChance = magicPortalRangeFloat(rng, 0.00012, 0.00035)
		cfg.SurgeDur = magicPortalRangeInt(rng, 90, 150)
		cfg.SurgeMult = magicPortalRangeFloat(rng, 1.18, 1.5)
		cfg.BurstEmbers = magicPortalRangeInt(rng, 3, 7)
		cfg.QuietDur = magicPortalRangeInt(rng, 320, 580)
		cfg.QuietMult = magicPortalRangeFloat(rng, 0.18, 0.30)
		cfg.EmberLife = magicPortalRangeInt(rng, 55, 90)
	}
	return sim.NormalizeMagicPortalConfig(cfg)
}

func lerpMagicPortalConfig(a, b sim.MagicPortalConfig, t float64) sim.MagicPortalConfig {
	t = clampUnit(t)
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return int(math.Round(float64(x) + float64(y-x)*t)) }
	return sim.NormalizeMagicPortalConfig(sim.MagicPortalConfig{
		IntroResolve:     li(a.IntroResolve, b.IntroResolve),
		IntroFull:        li(a.IntroFull, b.IntroFull),
		EndingDur:        li(a.EndingDur, b.EndingDur),
		FinalSurgeDur:    li(a.FinalSurgeDur, b.FinalSurgeDur),
		FinalSurgeInt:    lf(a.FinalSurgeInt, b.FinalSurgeInt),
		EndingEmberTail:  li(a.EndingEmberTail, b.EndingEmberTail),
		RingScale:        lf(a.RingScale, b.RingScale),
		RingWidth:        lf(a.RingWidth, b.RingWidth),
		RuneCount:        li(a.RuneCount, b.RuneCount),
		PulsePeriod:      li(a.PulsePeriod, b.PulsePeriod),
		PulseAmp:         lf(a.PulseAmp, b.PulseAmp),
		Glow:             lf(a.Glow, b.Glow),
		EmberRate:        li(a.EmberRate, b.EmberRate),
		Hue:              normalizeMagicPortalHue(lerpAngle(a.Hue, b.Hue, t)),
		Sat:              lf(a.Sat, b.Sat),
		LightMin:         lf(a.LightMin, b.LightMin),
		LightMax:         lf(a.LightMax, b.LightMax),
		SurgeChance:      lf(a.SurgeChance, b.SurgeChance),
		EmberBurstChance: lf(a.EmberBurstChance, b.EmberBurstChance),
		RuneShiftChance:  lf(a.RuneShiftChance, b.RuneShiftChance),
		QuietChance:      lf(a.QuietChance, b.QuietChance),
		PulsePeakDur:     li(a.PulsePeakDur, b.PulsePeakDur),
		SurgeDur:         li(a.SurgeDur, b.SurgeDur),
		SurgeMult:        lf(a.SurgeMult, b.SurgeMult),
		BurstEmbers:      li(a.BurstEmbers, b.BurstEmbers),
		RuneShiftDur:     li(a.RuneShiftDur, b.RuneShiftDur),
		QuietDur:         li(a.QuietDur, b.QuietDur),
		QuietMult:        lf(a.QuietMult, b.QuietMult),
		EmberLife:        li(a.EmberLife, b.EmberLife),
	})
}

func nameForMagicPortalConfig(cfg sim.MagicPortalConfig) string {
	cfg = sim.NormalizeMagicPortalConfig(cfg)
	if cfg.Sat < 0.24 || cfg.LightMax < 0.58 {
		return "dormant"
	}
	hue := normalizeMagicPortalHue(cfg.Hue)
	switch {
	case hue < 25 || hue >= 345:
		return "infernal red"
	case hue < 65:
		return "ancient amber"
	default:
		return "arcane blue"
	}
}

func magicPortalRangeFloat(rng *rngutil.RNG, min, max float64) float64 {
	if max <= min {
		return min
	}
	return min + rng.Float64()*(max-min)
}

func magicPortalRangeInt(rng *rngutil.RNG, min, max int) int {
	if max <= min {
		return min
	}
	return min + rng.Intn(max-min+1)
}

func normalizeMagicPortalHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}
