package main

import (
	"encoding/json"
	"math"
	"math/rand"

	"github.com/romaine-life/ambience/sim"
)

const devRandomSeedMixer int64 = 0x5deece66d

func randomizedDevConfig(schema sim.EffectSchema, seed int64) (json.RawMessage, error) {
	rng := rand.New(rand.NewSource(seed ^ devRandomSeedMixer))
	cfg := make(map[string]any, len(schema.Knobs))
	for _, knob := range schema.Knobs {
		value := randomizedKnobValue(rng, knob)
		switch knob.Type {
		case sim.KnobInt:
			cfg[knob.Key] = int(math.Round(value))
		default:
			cfg[knob.Key] = value
		}
	}
	stabilizeRandomizedDevConfig(schema.Name, cfg)
	normalizeRandomizedLightBounds(cfg)
	return json.Marshal(cfg)
}

func stabilizeRandomizedDevConfig(effect string, cfg map[string]any) {
	if effect == "paper-lanterns" {
		clampIntMin(cfg, "lone_every", 55)
		clampIntMax(cfg, "lone_every", 130)
		clampIntMin(cfg, "release_gap", 240)
		clampIntMax(cfg, "release_gap", 520)
		clampIntMin(cfg, "max", 55)
		clampIntMin(cfg, "release_min", 5)
		clampIntMax(cfg, "release_min", 9)
		clampIntMin(cfg, "release_max", 7)
		clampIntMax(cfg, "release_max", 12)
		clampIntMin(cfg, "release_window", 12)
		clampIntMax(cfg, "release_window", 34)
		clampIntMax(cfg, "ending_stop", 20)
		clampIntMin(cfg, "ending_tail", 220)
		clampIntMax(cfg, "ending_tail", 360)
		clampFloatRange(cfg, "rise", 0.05, 0.11)
		clampFloatRange(cfg, "wind", -0.55, 0.65)
		clampFloatMax(cfg, "wind_jit", 0.35)
		clampFloatRange(cfg, "size", 1.1, 2.1)
		clampFloatRange(cfg, "hue", 24, 52)
		clampFloatMax(cfg, "hue_sp", 20)
		clampFloatMin(cfg, "lbal", 0.25)
		clampFloatMax(cfg, "lbal", 0.65)
		clampFloatMax(cfg, "emit_p", 0)
		clampFloatMax(cfg, "release_p", 0)
		clampFloatMax(cfg, "fade_p", 0)
		clampFloatMax(cfg, "quiet_gap_p", 0)
		clampFloatMax(cfg, "wind_drift_p", 0.0002)
		return
	}
	if effect != "rain" {
		return
	}
	clampFloatMin(cfg, "speed", 1.5)
	clampFloatMax(cfg, "speed", 2.4)
	clampFloatMax(cfg, "speed_jit", 0.2)
	clampIntMin(cfg, "streak", 10)
	clampFloatMin(cfg, "fade", 0.88)
	clampIntMin(cfg, "spawn", 2)
	clampIntMax(cfg, "spawn", 5)
	clampIntMin(cfg, "burst", 3)
	clampIntMax(cfg, "burst", 5)
	clampFloatRange(cfg, "wind", -0.4, 0.4)
	clampFloatMax(cfg, "wind_jit", 0.2)
	clampFloatMax(cfg, "wind_drift", 0.2)
	clampIntMin(cfg, "layers", 2)
	clampFloatMin(cfg, "lbal", 0.45)
	clampFloatMin(cfg, "sheet", 0.5)
	clampIntMin(cfg, "sheet_len", 9)
	clampFloatMin(cfg, "sheet_alpha", 0.25)
	clampFloatMax(cfg, "sheet_alpha", 0.45)
	clampFloatMin(cfg, "sheet_speed", 1.3)
	clampFloatMax(cfg, "sheet_speed", 2.0)
	clampFloatMin(cfg, "front", 0.25)
	clampFloatMax(cfg, "front", 0.55)
	clampFloatMin(cfg, "front_alpha", 0.35)
	clampFloatMax(cfg, "front_alpha", 0.7)
	clampIntMin(cfg, "front_len", 18)
	clampIntMax(cfg, "front_len", 32)
	clampFloatMin(cfg, "front_speed", 40)
	clampFloatMax(cfg, "front_speed", 72)
	clampRainHue(cfg)
	clampFloatMax(cfg, "hue_sp", 18)
	clampFloatMax(cfg, "sat", 0.45)
	clampFloatMax(cfg, "lmin", 0.45)
	clampFloatMax(cfg, "lmax", 0.75)
	clampFloatMax(cfg, "downpour_p", 0.0003)
	clampFloatMax(cfg, "calm_p", 0.0003)
	clampFloatMax(cfg, "gust_p", 0.0003)
	clampFloatMax(cfg, "splash_p", 0.0006)
	clampFloatMax(cfg, "downpour_mult", 4)
}

func clampFloatMin(cfg map[string]any, key string, min float64) {
	if v, ok := cfg[key].(float64); ok && v < min {
		cfg[key] = min
	}
}

func clampFloatMax(cfg map[string]any, key string, max float64) {
	if v, ok := cfg[key].(float64); ok && v > max {
		cfg[key] = max
	}
}

func clampFloatRange(cfg map[string]any, key string, min, max float64) {
	if v, ok := cfg[key].(float64); ok {
		if v < min {
			cfg[key] = min
		} else if v > max {
			cfg[key] = max
		}
	}
}

func clampRainHue(cfg map[string]any) {
	v, ok := cfg["hue"].(float64)
	if !ok || (v >= 190 && v <= 240) {
		return
	}
	cfg["hue"] = 204 + math.Mod(math.Abs(v), 28)
}

func normalizeRandomizedLightBounds(cfg map[string]any) {
	lmin, okMin := cfg["lmin"].(float64)
	lmax, okMax := cfg["lmax"].(float64)
	if !okMin || !okMax || lmax >= lmin {
		return
	}
	cfg["lmin"], cfg["lmax"] = lmax, lmin
}

func clampIntMin(cfg map[string]any, key string, min int) {
	if v, ok := cfg[key].(int); ok && v < min {
		cfg[key] = min
	}
}

func clampIntMax(cfg map[string]any, key string, max int) {
	if v, ok := cfg[key].(int); ok && v > max {
		cfg[key] = max
	}
}

func randomizedKnobValue(rng *rand.Rand, knob sim.Knob) float64 {
	min := knob.Min
	max := knob.Max
	if max < min {
		min, max = max, min
	}
	if max == min {
		return min
	}

	step := knob.Step
	switch knob.Type {
	case sim.KnobInt:
		if step < 1 {
			step = 1
		}
	default:
		if step <= 0 {
			step = 0.01
		}
	}

	fraction := randomizedKnobFraction(rng, knob, min, max)
	value := min + (max-min)*fraction
	value = quantizeKnobValue(value, min, max, step, knob.Type)
	return avoidImplicitDefaultCollision(value, min, max, step, knob)
}

func randomizedKnobFraction(rng *rand.Rand, knob sim.Knob, min, max float64) float64 {
	sample := fullRangeKnobFraction(rng, knob)
	if knob.Default < min || knob.Default > max || max <= min {
		return clampUnit(sample)
	}

	biasChance, biasSpan := knobBiasProfile(knob)
	if rng.Float64() >= biasChance {
		return clampUnit(sample)
	}

	defaultNorm := (knob.Default - min) / (max - min)
	low := math.Max(0, defaultNorm-biasSpan*0.5)
	high := math.Min(1, defaultNorm+biasSpan*0.5)
	if high <= low {
		return clampUnit(defaultNorm)
	}
	return low + rng.Float64()*(high-low)
}

func fullRangeKnobFraction(rng *rand.Rand, knob sim.Knob) float64 {
	switch knob.Slot {
	case sim.SlotEvent:
		return math.Pow(rng.Float64(), 2.4)
	case sim.SlotSpawn, sim.SlotEnd:
		return 0.05 + 0.9*rng.Float64()
	default:
		return rng.Float64()
	}
}

func knobBiasProfile(knob sim.Knob) (chance, span float64) {
	switch knob.Slot {
	case sim.SlotEvent:
		return 0.55, 0.12
	case sim.SlotSpawn, sim.SlotEnd:
		return 0.45, 0.4
	case sim.SlotEventMod:
		return 0.35, 0.5
	default:
		return 0.35, 0.45
	}
}

func quantizeKnobValue(value, min, max, step float64, knobType sim.KnobType) float64 {
	if step > 0 {
		value = min + math.Round((value-min)/step)*step
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	if knobType == sim.KnobInt {
		return math.Round(value)
	}
	return math.Round(value*1e6) / 1e6
}

func avoidImplicitDefaultCollision(value, min, max, step float64, knob sim.Knob) float64 {
	if knob.Default == 0 || value != 0 || max <= 0 || step <= 0 {
		return value
	}

	replacement := step
	if min < 0 && max > 0 && knob.Default < 0 {
		replacement = -step
	}
	if replacement < min {
		replacement = min
	}
	if replacement > max {
		replacement = max
	}
	return quantizeKnobValue(replacement, min, max, step, knob.Type)
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
