package main

import "github.com/nelsong6/ambience/sim"

func init() {
	register(newProceduralEffectDef("windmill", sim.WindmillSchema))
}
