package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/nelsong6/ambience/sim"
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
		sim.VolcanoSchema(),
		sim.TrainSchema(),
		sim.MysteriousManSchema(),
		sim.BurningTreesSchema(),
		sim.SandSchema(),
		sim.TetrisSchema(),
		sim.WaterPipeSchema(),
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
		sim.VolcanoSchema(),
		sim.TrainSchema(),
		sim.MysteriousManSchema(),
		sim.BurningTreesSchema(),
		sim.SandSchema(),
		sim.TetrisSchema(),
		sim.WaterPipeSchema(),
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
