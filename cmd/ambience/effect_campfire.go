package main

import "github.com/nelsong6/ambience/sim"

func init() {
	register(newProceduralEffectDef("campfire", sim.CampfireSchema))
}
