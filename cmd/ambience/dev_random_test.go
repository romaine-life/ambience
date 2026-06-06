package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

func TestRandomizedDevConfigStaysWithinSchemaBounds(t *testing.T) {
	schemas := []sim.EffectSchema{
		sim.RainSchema(),
		sim.DustSchema(),
		sim.FirefliesSchema(),
		sim.WaterfallSchema(),
		sim.SnowSchema(),
		sim.AutumnLeavesSchema(),
		sim.StarfieldSchema(),
		sim.AuroraSchema(),
		sim.WheatFieldSchema(),
		sim.BeachSchema(),
		sim.CampfireSchema(),
		sim.WindmillSchema(),
		sim.LighthouseSchema(),
		sim.RowboatSchema(),
		sim.UnderwaterSchema(),
	}

	for i, schema := range schemas {
		data, err := randomizedDevConfig(schema, int64(12345+i))
		if err != nil {
			t.Fatalf("%s randomizedDevConfig: %v", schema.Name, err)
		}

		var values map[string]float64
		if err := json.Unmarshal(data, &values); err != nil {
			t.Fatalf("%s unmarshal randomized config: %v", schema.Name, err)
		}

		for _, knob := range schema.Knobs {
			got, ok := values[knob.Key]
			if !ok {
				t.Fatalf("%s missing randomized value for %q", schema.Name, knob.Key)
			}
			if got < knob.Min-1e-9 || got > knob.Max+1e-9 {
				t.Fatalf("%s %s = %v outside [%v, %v]", schema.Name, knob.Key, got, knob.Min, knob.Max)
			}
			if !valueAlignedToStep(knob, got) {
				t.Fatalf("%s %s = %v is not aligned to step %v from min %v", schema.Name, knob.Key, got, knob.Step, knob.Min)
			}
		}
	}
}

func TestRandomizedDevConfigChangesAtLeastOneKnob(t *testing.T) {
	schemas := []sim.EffectSchema{
		sim.RainSchema(),
		sim.DustSchema(),
		sim.FirefliesSchema(),
		sim.WaterfallSchema(),
		sim.SnowSchema(),
		sim.AutumnLeavesSchema(),
		sim.StarfieldSchema(),
		sim.AuroraSchema(),
		sim.WheatFieldSchema(),
		sim.BeachSchema(),
		sim.CampfireSchema(),
		sim.WindmillSchema(),
		sim.LighthouseSchema(),
		sim.RowboatSchema(),
		sim.UnderwaterSchema(),
	}

	for i, schema := range schemas {
		data, err := randomizedDevConfig(schema, int64(98765+i))
		if err != nil {
			t.Fatalf("%s randomizedDevConfig: %v", schema.Name, err)
		}

		var values map[string]float64
		if err := json.Unmarshal(data, &values); err != nil {
			t.Fatalf("%s unmarshal randomized config: %v", schema.Name, err)
		}

		changed := false
		for _, knob := range schema.Knobs {
			if math.Abs(values[knob.Key]-knob.Default) > 1e-9 {
				changed = true
				break
			}
		}
		if !changed {
			t.Fatalf("%s randomized config unexpectedly matched every default", schema.Name)
		}
	}
}

func TestRandomizedRainConfigKeepsReadableWeatherEnvelope(t *testing.T) {
	schema := sim.RainSchema()
	for seed := int64(1); seed <= 128; seed++ {
		data, err := randomizedDevConfig(schema, seed)
		if err != nil {
			t.Fatalf("randomizedDevConfig seed %d: %v", seed, err)
		}

		var values map[string]float64
		if err := json.Unmarshal(data, &values); err != nil {
			t.Fatalf("unmarshal randomized rain config seed %d: %v", seed, err)
		}

		if values["speed"] < 0.85 || values["speed"] > 1.35 || values["streak"] < 10 || values["spawn"] < 2 || values["spawn"] > 5 || values["burst"] < 3 || values["burst"] > 5 {
			t.Fatalf("seed %d produced rain outside the 60 Hz foreground envelope: %v", seed, values)
		}
		if values["speed_jit"] > 0.2 || values["wind"] < -0.4 || values["wind"] > 0.4 || values["wind_jit"] > 0.2 || values["wind_drift"] > 0.2 || values["fade"] < 0.88 {
			t.Fatalf("seed %d produced unstable rain motion: %v", seed, values)
		}
		if values["layers"] < 2 || values["lbal"] < 0.45 {
			t.Fatalf("seed %d produced rain without enough depth: %v", seed, values)
		}
		if values["sheet"] < 0.5 || values["sheet_len"] < 9 || values["sheet_speed"] > 1.5 {
			t.Fatalf("seed %d produced rain without enough atmospheric sheet: %v", seed, values)
		}
		if values["hue"] < 190 || values["hue"] > 240 || values["sat"] > 0.45 || values["lmax"] > 0.75 {
			t.Fatalf("seed %d produced rain outside restrained palette: %v", seed, values)
		}
	}
}

func valueAlignedToStep(knob sim.Knob, value float64) bool {
	if knob.Step <= 0 {
		if knob.Type == sim.KnobInt {
			return math.Abs(value-math.Round(value)) <= 1e-9
		}
		return true
	}
	steps := math.Round((value - knob.Min) / knob.Step)
	expected := knob.Min + steps*knob.Step
	if math.Abs(value-expected) > 1e-6 {
		return false
	}
	if knob.Type == sim.KnobInt {
		return math.Abs(value-math.Round(value)) <= 1e-9
	}
	return true
}
