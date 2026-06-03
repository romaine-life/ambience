package main

import "github.com/romaine-life/ambience/sim"

func (r *rainRuntime) Frame() [][]sim.Pixel          { return r.sim.GridCopy() }
func (r *auroraRuntime) Frame() [][]sim.Pixel        { return r.sim.GridCopy() }
func (r *autumnLeavesRuntime) Frame() [][]sim.Pixel  { return r.sim.GridCopy() }
func (r *beachRuntime) Frame() [][]sim.Pixel         { return r.sim.GridCopy() }
func (r *bogRuntime) Frame() [][]sim.Pixel           { return r.sim.GridCopy() }
func (r *burningTreesRuntime) Frame() [][]sim.Pixel  { return r.sim.GridCopy() }
func (r *campfireRuntime) Frame() [][]sim.Pixel      { return r.sim.GridCopy() }
func (r *caveCrystalsRuntime) Frame() [][]sim.Pixel  { return r.sim.GridCopy() }
func (r *distantStormRuntime) Frame() [][]sim.Pixel  { return r.sim.GridCopy() }
func (r *dustRuntime) Frame() [][]sim.Pixel          { return r.sim.GridCopy() }
func (r *firefliesRuntime) Frame() [][]sim.Pixel     { return r.sim.GridCopy() }
func (r *lighthouseRuntime) Frame() [][]sim.Pixel    { return r.sim.GridCopy() }
func (r *mysteriousManRuntime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }
func (r *pondRuntime) Frame() [][]sim.Pixel          { return r.sim.GridCopy() }
func (r *rowboatRuntime) Frame() [][]sim.Pixel       { return r.sim.GridCopy() }
func (r *sandRuntime) Frame() [][]sim.Pixel          { return r.sim.GridCopy() }
func (r *slimesRuntime) Frame() [][]sim.Pixel        { return r.sim.GridCopy() }
func (r *snowRuntime) Frame() [][]sim.Pixel          { return r.sim.GridCopy() }
func (r *starfieldRuntime) Frame() [][]sim.Pixel     { return r.sim.GridCopy() }
func (r *tetrisRuntime) Frame() [][]sim.Pixel        { return r.sim.GridCopy() }
func (r *trainRuntime) Frame() [][]sim.Pixel         { return r.sim.GridCopy() }
func (r *underwaterRuntime) Frame() [][]sim.Pixel    { return r.sim.GridCopy() }
func (r *volcanoRuntime) Frame() [][]sim.Pixel       { return r.sim.GridCopy() }
func (r *waterPipeRuntime) Frame() [][]sim.Pixel     { return r.sim.GridCopy() }
func (r *waterfallRuntime) Frame() [][]sim.Pixel     { return r.sim.GridCopy() }
func (r *wheatFieldRuntime) Frame() [][]sim.Pixel    { return r.sim.GridCopy() }
func (r *windmillRuntime) Frame() [][]sim.Pixel      { return r.sim.GridCopy() }
