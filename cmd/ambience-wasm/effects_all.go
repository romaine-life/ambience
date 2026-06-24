//go:build js && wasm && !rainonly

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/romaine-life/ambience/sim"
)

// Full effect registry — every effect the server's own site/runtime can render.
// This is the default build (no extra tags), so the produced ambience.wasm is
// the complete all-effects bundle the ambience site serves at /ambience.wasm.
//
// The rain-scoped client artifact (served at /ambience-rain.wasm for vendoring
// consumers like chess) is built with `-tags rainonly`, which swaps in
// effects_rainonly.go and lets the linker drop every unreferenced sim.New*.

const wasmLavaLampEffect = "lava-lamp"

func supportedEffects(js.Value, []js.Value) any {
	effects := []any{
		"aurora",
		"autumn-leaves",
		"beach",
		"birds-on-a-wire",
		"bog",
		"burning-trees",
		"campfire",
		"cave-crystals",
		"constellations",
		"cottage-chimney",
		"distant-storm",
		"dust",
		"fireflies",
		wasmLavaLampEffect,
		"lighthouse",
		"magic-portal",
		"mysterious-man",
		"paper-lanterns",
		"pond",
		"rain",
		"rain-on-window",
		"rowboat",
		"sand",
		"slimes",
		"snow",
		"snow-globe",
		"spider-web",
		"starfield",
		"tetris",
		"train",
		"underwater",
		"volcano",
		"water-pipe",
		"waterfall",
		"wheat-field",
		"windmill",
	}
	return js.ValueOf(effects)
}

func makeRuntime(kind string, w, h int, seed int64, cfg json.RawMessage) (*runtime, error) {
	switch kind {
	case "aurora":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewAurora)
	case "autumn-leaves":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewAutumnLeaves)
	case "beach":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewBeach)
	case "birds-on-a-wire":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewBirdsOnAWire)
	case "bog":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewBog)
	case "burning-trees":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewBurningTrees)
	case "campfire":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewCampfire)
	case "cave-crystals":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewCaveCrystals)
	case "constellations":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewConstellations)
	case "cottage-chimney":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewCottageChimney)
	case "distant-storm":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewDistantStorm)
	case "dust":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewDust)
	case "fireflies":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewFireflies)
	case wasmLavaLampEffect:
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewLavaLamp)
	case "lighthouse":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewLighthouse)
	case "magic-portal":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewMagicPortal)
	case "mysterious-man":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewMysteriousMan)
	case "paper-lanterns":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewPaperLanterns)
	case "pond":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewPond)
	case "rain":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewRain)
	case "rain-on-window":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewRainOnWindow)
	case "rowboat":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewRowboat)
	case "sand":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewSand)
	case "slimes":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewSlimes)
	case "snow":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewSnow)
	case "snow-globe":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewSnowGlobe)
	case "spider-web":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewSpiderWeb)
	case "starfield":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewStarfield)
	case "tetris":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewTetris)
	case "train":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewTrain)
	case "underwater":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewUnderwater)
	case "volcano":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewVolcano)
	case "water-pipe":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewWaterPipe)
	case "waterfall":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewWaterfall)
	case "wheat-field":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewWheatField)
	case "windmill":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewWindmill)
	default:
		return nil, fmt.Errorf("unsupported effect %q", kind)
	}
}
