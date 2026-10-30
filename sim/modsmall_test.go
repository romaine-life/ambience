package sim

import (
	"math"
	"math/rand/v2"
	"testing"
)

// modSmall must be bit-identical to math.Mod: effects use it in place of
// math.Mod on their per-pixel paths and clients must keep rendering the same
// pixels.
func TestModSmallMatchesMathMod(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	check := func(v, m float64) {
		got, want := modSmall(v, m), math.Mod(v, m)
		if math.Float64bits(got) != math.Float64bits(want) && !(math.IsNaN(got) && math.IsNaN(want)) {
			t.Fatalf("modSmall(%v, %v) = %v, math.Mod = %v", v, m, got, want)
		}
	}
	for _, m := range []float64{2, 360} {
		for _, v := range []float64{0, math.Copysign(0, -1), m, 2 * m, 3 * m, math.Nextafter(m, 0), math.Nextafter(2*m, 0), math.Nextafter(3*m, 0), -1, -m, 5 * m, math.Inf(1), math.NaN(), 1e-300} {
			check(v, m)
		}
		for i := 0; i < 2_000_000; i++ {
			check((r.Float64()*5-1)*m, m)
		}
	}
}
