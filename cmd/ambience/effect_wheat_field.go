package main

import "github.com/nelsong6/ambience/sim"

func init() {
	register(newProceduralEffectDef("wheat-field", sim.WheatFieldSchema))
}
