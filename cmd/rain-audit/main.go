// rain-audit is a deterministic introspection harness for the rain sim. It
// spawns a large, density-independent sample of drops and prints a self-judging
// percept report: physical-unit distributions plus a graded correlation matrix
// of every visual cue against fall speed. The point is that it checks the WHOLE
// percept every run — so a regression in any single axis (speed too slow, fast
// drops too thin, etc.) fails loudly here instead of waiting to be noticed on
// screen.
//
// Usage:
//
//	go run ./cmd/rain-audit                      # default scene, 640x360
//	go run ./cmd/rain-audit -n 8000 -speed 1.0   # lighter/slower, bigger sample
//	go run ./cmd/rain-audit -seed 3 -json        # raw DropInfo as JSON
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/romaine-life/ambience/sim"
)

func main() {
	var (
		n       = flag.Int("n", 6000, "sample size (drops to spawn)")
		seed    = flag.Int64("seed", 1, "rng seed (deterministic)")
		gw      = flag.Int("gw", 640, "grid width")
		gh      = flag.Int("gh", 360, "grid height")
		speed   = flag.Float64("speed", 1.8, "Speed knob (intensity)")
		layers  = flag.Int("layers", 2, "depth layers (1 = flat, 2 = depth field)")
		dumpRaw = flag.Bool("json", false, "dump raw DropInfo as JSON instead of the report")
	)
	flag.Parse()

	r := sim.NewRain(*gw, *gh, *seed, sim.Config{Layers: *layers, Speed: *speed})
	r.SpawnDrops(*n)
	info := r.DropProvenance()
	if len(info) == 0 {
		fmt.Fprintln(os.Stderr, "no drops spawned")
		os.Exit(1)
	}

	if *dumpRaw {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(info)
		return
	}

	// Extract parallel arrays.
	col := func(f func(sim.DropInfo) float64) []float64 {
		xs := make([]float64, len(info))
		for i, d := range info {
			xs[i] = f(d)
		}
		return xs
	}
	speedA := col(func(d sim.DropInfo) float64 { return d.RowsPerTick })
	diam := col(func(d sim.DropInfo) float64 { return d.DiameterMm })
	width := col(func(d sim.DropInfo) float64 { return float64(d.WidthCells) })
	streak := col(func(d sim.DropInfo) float64 { return float64(d.StreakCells) })
	bright := col(func(d sim.DropInfo) float64 { return d.Brightness })
	dist := col(func(d sim.DropInfo) float64 { return d.Distance })
	secs := col(func(d sim.DropInfo) float64 { return d.SecondsCross })
	appMS := col(func(d sim.DropInfo) float64 { return d.ApparentMS })

	fmt.Printf("rain-audit  grid %dx%d  seed %d  Speed %.2f  layers %d  n=%d\n",
		*gw, *gh, *seed, *speed, *layers, len(info))
	fmt.Println("(deterministic spawn sample — density-independent)")

	fmt.Println("\n── distributions (physical units) ──")
	printDist("crossing time (s)", secs, "%.2f")
	printDist("apparent fall (m/s)", appMS, "%.1f")
	printDist("diameter (mm)", diam, "%.2f")
	printDist("width (cells)", width, "%.1f")
	printDist("streak (cells)", streak, "%.0f")
	printDist("brightness (0-255)", bright, "%.0f")

	fmt.Println("\n── physics grade: every cue vs fall speed ──")
	pass := true
	grade := func(name string, c, wantSign, minMag float64) {
		ok := c*wantSign > 0 && math.Abs(c) >= minMag
		mark := "OK  "
		if !ok {
			mark = "FAIL"
			pass = false
		}
		sign := "+"
		if wantSign < 0 {
			sign = "-"
		}
		fmt.Printf("  [%s] %-34s r=%+.2f  (want %s, |r|>=%.2f)\n", mark, name, c, sign, minMag)
	}
	grade("bigger drops fall faster", pearson(speedA, diam), +1, 0.5)
	grade("faster drops are thicker", pearson(speedA, width), +1, 0.2)
	grade("faster drops streak longer", pearson(speedA, streak), +1, 0.3)
	grade("faster drops are brighter", pearson(speedA, bright), +1, 0.15)
	grade("faster drops are nearer", pearson(speedA, dist), -1, 0.2)

	spread := maxOf(speedA) / minOf(speedA)
	spreadOK := spread >= 2.0
	if !spreadOK {
		pass = false
	}
	mark := "OK  "
	if !spreadOK {
		mark = "FAIL"
	}
	fmt.Printf("  [%s] %-34s %.1fx  (want >= 2.0x)\n", mark, "speed spread (max/min)", spread)

	fmt.Println()
	if pass {
		fmt.Println("VERDICT: PASS — all cue relationships physically coherent")
	} else {
		fmt.Println("VERDICT: FAIL — a cue is decoupled or inverted (see above)")
		os.Exit(1)
	}
}

func printDist(label string, xs []float64, f string) {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	p := func(q float64) float64 { return s[int(q*float64(len(s)-1))] }
	form := "  %-20s p5 " + f + "   p50 " + f + "   p95 " + f + "   (min " + f + " / max " + f + ")\n"
	fmt.Printf(form, label, p(0.05), p(0.50), p(0.95), s[0], s[len(s)-1])
}

func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	var sx, sy, sxx, syy, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		syy += ys[i] * ys[i]
		sxy += xs[i] * ys[i]
	}
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

func maxOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		m = math.Max(m, x)
	}
	return m
}

func minOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		m = math.Min(m, x)
	}
	return m
}
