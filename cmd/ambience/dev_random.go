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
	return json.Marshal(cfg)
}

func stabilizeRandomizedDevConfig(effect string, cfg map[string]any) {
	if effect != "rain" {
		return
	}
	clampFloatMin(cfg, "speed", 2.1)
	clampIntMin(cfg, "streak", 10)
	clampIntMax(cfg, "spawn", 2)
	clampIntMin(cfg, "burst", 6)
	clampIntMin(cfg, "layers", 2)
	clampFloatMin(cfg, "lbal", 0.5)
}

func clampFloatMin(cfg map[string]any, key string, min float64) {
	if v, ok := cfg[key].(float64); ok && v < min {
		cfg[key] = min
	}
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
