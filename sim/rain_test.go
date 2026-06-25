package sim

import (
	"image/color"
	"math"
	"testing"
)

var staticRed = color.RGBA{255, 100, 100, 255}

// pearson returns the linear correlation of two equal-length samples in [-1,1].
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
	num := n*sxy - sx*sy
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den == 0 {
		return 0
	}
	return num / den
}

// TestRainCueCoherence pins the physical relationships between a drop's cues so
// a regression in any single axis fails here instead of waiting for a human to
// notice it on screen. Real rain: bigger drops fall faster (Gunn–Kinzer), and
// "bigger" must show up in EVERY cue together — a fast drop is also thicker,
// longer-streaked, brighter and nearer. Each of these was, at some point, silently
// wrong (uniform speed; fast drops rendered thinner; etc.).
func TestRainCueCoherence(t *testing.T) {
	r := NewRain(640, 360, 7, Config{Layers: 2})
	const n = 4000
	for i := 0; i < n; i++ {
		r.spawnDropAt(r.rng.Float64() * float64(r.W))
	}
	if len(r.drops) != n {
		t.Fatalf("spawned %d drops, want %d", len(r.drops), n)
	}

	speed := make([]float64, n)
	width := make([]float64, n)
	streak := make([]float64, n)
	bright := make([]float64, n)
	depth := make([]float64, n)
	for i, d := range r.drops {
		speed[i] = d.vRow
		width[i] = float64(d.widthCells)
		streak[i] = float64(d.streakLen)
		bright[i] = 0.299*float64(d.Color.R) + 0.587*float64(d.Color.G) + 0.114*float64(d.Color.B)
		depth[i] = d.depth
	}

	// Speed must genuinely vary (Gunn–Kinzer spread), not sit at one value.
	lo, hi := speed[0], speed[0]
	for _, v := range speed {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi/lo < 2.0 {
		t.Errorf("fall-speed spread = %.2fx (max %.2f / min %.2f), want >= 2x — drops look uniform", hi/lo, hi, lo)
	}

	// Every cue must move with apparent size together; signs are the physics.
	checks := []struct {
		name string
		corr float64
		want float64 // required sign
		min  float64 // required magnitude
	}{
		{"faster drops are thicker", pearson(speed, width), +1, 0.2},
		{"faster drops have longer streaks", pearson(speed, streak), +1, 0.3},
		{"faster drops are brighter", pearson(speed, bright), +1, 0.15},
		{"faster drops are nearer (lower depth)", pearson(speed, depth), -1, 0.2},
	}
	for _, c := range checks {
		if c.corr*c.want <= 0 || math.Abs(c.corr) < c.min {
			t.Errorf("%s: correlation = %+.2f, want sign %+.0f and |r| >= %.2f", c.name, c.corr, c.want, c.min)
		}
	}
}

func TestOverlayLayerIsANearDepthSubsetOfTheMainField(t *testing.T) {
	countLit := func(g [][]Pixel) int {
		n := 0
		for _, row := range g {
			for _, p := range row {
				if p.Filled {
					n++
				}
			}
		}
		return n
	}

	// Overlay off (the default): nothing qualifies, the overlay frame is empty,
	// and the main field still rains — the feature is inert when the lever is 0.
	off := NewRain(80, 45, 7, Config{Overlay: 0, SpawnEvery: 1, SpawnBurst: 4})
	for i := 0; i < 200; i++ {
		off.Step()
	}
	if lit := countLit(off.OverlayGridCopy()); lit != 0 {
		t.Fatalf("overlay off: expected an empty overlay frame, got %d lit pixels", lit)
	}
	if countLit(off.GridCopy()) == 0 {
		t.Fatalf("overlay off: expected rain in the main field")
	}

	// Overlay 1 (every drop is at/nearer than depth 1): the overlay frame is
	// non-empty, and every lit overlay pixel is also present (and at least as
	// bright) in the main field. The promoted drops still paint into the main
	// grid exactly as before, so the back field is unchanged — purely additive.
	on := NewRain(80, 45, 7, Config{Overlay: 1, SpawnEvery: 1, SpawnBurst: 4})
	for i := 0; i < 200; i++ {
		on.Step()
	}
	ov := on.OverlayGridCopy()
	main := on.GridCopy()
	if countLit(ov) == 0 {
		t.Fatalf("overlay on: expected a non-empty overlay frame")
	}
	for y := range ov {
		for x := range ov[y] {
			op := ov[y][x]
			if !op.Filled {
				continue
			}
			mp := main[y][x]
			if !mp.Filled || mp.C.R < op.C.R || mp.C.G < op.C.G || mp.C.B < op.C.B {
				t.Fatalf("overlay pixel (%d,%d)=%v not covered by main field %v", x, y, op.C, mp.C)
			}
		}
	}
}

func TestNewRainAppliesDefaults(t *testing.T) {
	r := NewRain(10, 10, 1, Config{})
	if r.cfg.SpawnEvery == 0 {
		t.Errorf("expected default SpawnEvery")
	}
	if r.cfg.FadeFactor == 0 {
		t.Errorf("expected default FadeFactor")
	}
	if r.cfg.Hue == 0 {
		t.Errorf("expected default Hue")
	}
	if r.cfg.StreakLen == 0 {
		t.Errorf("expected default StreakLen")
	}
	if r.cfg.Speed == 0 {
		t.Errorf("expected default Speed")
	}
	if r.cfg.Layers == 0 {
		t.Errorf("expected default Layers")
	}
	if r.cfg.LayerBalance == 0 {
		t.Errorf("expected default LayerBalance")
	}
	if r.cfg.Speed < 1.5 || r.cfg.Speed > 2.0 {
		t.Errorf("expected 60 Hz rain default speed in faster foreground range, got %.2f", r.cfg.Speed)
	}
	if r.cfg.SpawnEvery != 3 || r.cfg.SpawnBurst != 4 {
		t.Errorf("expected restrained foreground rain defaults, got spawnEvery=%d burst=%d", r.cfg.SpawnEvery, r.cfg.SpawnBurst)
	}
	if r.cfg.FrontSpeed < 40 {
		t.Errorf("expected front-plane speed default to model near-window rain, got %.2f", r.cfg.FrontSpeed)
	}
}

func TestEndingDefaultsStillAllowExplicitZeroKnobs(t *testing.T) {
	r := NewRain(10, 10, 1, Config{
		EndingDur:      25,
		EndingLinger:   0,
		EndingSplashes: 0,
	})
	if r.cfg.EndingDur != 25 {
		t.Fatalf("ending dur = %d, want 25", r.cfg.EndingDur)
	}
	if r.cfg.EndingLinger != 0 {
		t.Fatalf("ending linger = %d, want 0", r.cfg.EndingLinger)
	}
	if r.cfg.EndingSplashes != 0 {
		t.Fatalf("ending splashes = %d, want 0", r.cfg.EndingSplashes)
	}
}

// TestStepClearsGridEachTick verifies the clear-before-repaint invariant:
// a cell manually set outside the drop system is gone after one Step.
func TestStepClearsGridEachTick(t *testing.T) {
	r := NewRain(10, 10, 1, Config{SpawnEvery: 1000000}) // never spawn
	r.Grid[3][3] = Pixel{Filled: true, C: staticRed}
	r.Step()
	if r.Grid[3][3].Filled {
		t.Errorf("expected grid cleared each tick; (3,3) still filled")
	}
}

// TestStepPaintsDropHeadAndTrail injects a drop and verifies a streak is
// painted after one Step.
func TestStepPaintsDropHeadAndTrail(t *testing.T) {
	r := NewRain(20, 20, 1, Config{
		Speed:      1,
		StreakLen:  4,
		FadeFactor: 0.9,
		SpawnEvery: 1000000, // disable spawns
	})
	r.drops = append(r.drops, drop{
		Row: 0, Col: 10,
		Color: staticRed, vRow: 1, vCol: 0,
		streakLen: 4,
	})
	r.Step()
	filled := 0
	for y := 0; y < r.H; y++ {
		if r.Grid[y][10].Filled {
			filled++
		}
	}
	if filled < 2 {
		t.Errorf("expected multiple cells filled in column 10 (streak), got %d", filled)
	}
}

func TestFastDropPaintsContiguousGridTrail(t *testing.T) {
	r := NewRain(20, 20, 1, Config{
		Speed:      3,
		StreakLen:  5,
		FadeFactor: 0.9,
		SpawnEvery: 1000000, // disable spawns
	})
	r.drops = append(r.drops, drop{
		Row: 0, Col: 10,
		Color: staticRed, vRow: 3, vCol: 0,
		streakLen: 5,
	})
	r.Step()

	for y := 0; y <= 3; y++ {
		if !r.Grid[y][10].Filled {
			t.Fatalf("expected fast drop to paint contiguous row %d in column 10", y)
		}
	}
}

func TestDefaultRainBuildsFullerWeatherField(t *testing.T) {
	r := NewRain(160, 80, 2, Config{
		SheetDensity:  0.6,
		SheetStrength: 0.3,
		SheetLength:   11,
		SheetSpeed:    1.2,
	})
	for i := 0; i < 160; i++ {
		r.Step()
	}

	filled := countFilledPixels(r.Grid)
	if filled < 1000 {
		t.Fatalf("default rain filled %d grid cells after warmup, want at least 1000", filled)
	}
}

func TestRainSheetBuildsTextureWithoutForegroundDrops(t *testing.T) {
	r := NewRain(80, 40, 2, Config{
		SpawnEvery:    1,
		SheetDensity:  0.8,
		SheetStrength: 0.5,
		SheetLength:   10,
		SheetSpeed:    1.5,
	})
	r.calmTicks = 2
	r.Step()

	filled := countFilledPixels(r.Grid)
	if filled < 300 {
		t.Fatalf("rain sheet filled %d grid cells, want at least 300", filled)
	}
	if len(r.drops) != 0 {
		t.Fatalf("foreground drops = %d, want 0 while calm suppresses spawning", len(r.drops))
	}
}

func TestFrontPlaneBuildsNearWindowStreaksWithoutTrackedDrops(t *testing.T) {
	r := NewRain(80, 40, 2, Config{
		SpawnEvery:    1000000,
		SheetDensity:  0,
		FrontDensity:  0.8,
		FrontStrength: 0.7,
		FrontLength:   24,
		FrontSpeed:    54,
	})
	r.calmTicks = 2
	for i := 0; i < 4; i++ {
		r.Step()
	}

	filled := countFilledPixels(r.Grid)
	if filled < 50 {
		t.Fatalf("front-plane rain filled %d grid cells, want at least 50", filled)
	}
	if len(r.drops) != 0 {
		t.Fatalf("tracked drops = %d, want 0 for procedural front-plane streaks", len(r.drops))
	}
}

func TestFrontPlaneDoesNotReuseExactPathSet(t *testing.T) {
	r := NewRain(120, 60, 2, Config{
		Wind:          0,
		SpawnEvery:    1000000,
		SheetDensity:  0,
		FrontDensity:  0.7,
		FrontStrength: 0.8,
		FrontLength:   20,
		FrontSpeed:    30,
	})
	r.calmTicks = 20
	for i := 0; i < 6; i++ {
		r.Step()
	}
	first := filledColumns(r.Grid)
	for i := 0; i < 8; i++ {
		r.Step()
	}
	second := filledColumns(r.Grid)

	if len(first) < 4 || len(second) < 4 {
		t.Fatalf("front-plane columns too sparse: first=%d second=%d", len(first), len(second))
	}
	if sameIntSet(first, second) {
		t.Fatalf("front-plane reused the same occupied column set; want changing event paths")
	}
}

func TestFrontPlaneDirectionStaysCoherentWithRainField(t *testing.T) {
	r := NewRain(120, 60, 2, Config{
		Wind:          0.18,
		SpawnEvery:    1000000,
		SheetDensity:  0,
		FrontDensity:  0.8,
		FrontStrength: 0.8,
		FrontLength:   24,
		FrontSpeed:    42,
	})

	wind := r.currentWind()
	birthTick := uint64(12)
	birthHash := hash64(birthTick*0x9e3779b97f4a7c15 + 0x8f1bbcdcaf1476d9)
	for i := 0; i < 12; i++ {
		h2 := hash64(birthHash + uint64(i)*0xd6e8feb86659fd93 + 0x1f83d9abfb41bd6b)
		eventWind := wind + (hashUnit(h2)*2-1)*0.035
		if diff := math.Abs(eventWind - wind); diff > 0.036 {
			t.Fatalf("front-plane event wind drift = %.3f, want coherent with rain field", diff)
		}
	}
}

func TestHslToRGBBasicAnchors(t *testing.T) {
	cases := []struct {
		h, s, l             float64
		wantR, wantG, wantB uint8
	}{
		{0, 1, 0.5, 255, 0, 0},
		{120, 1, 0.5, 0, 255, 0},
		{240, 1, 0.5, 0, 0, 255},
		{0, 0, 0, 0, 0, 0},
		{0, 0, 1, 255, 255, 255},
	}
	for _, tc := range cases {
		got := hslToRGB(tc.h, tc.s, tc.l)
		if got.R != tc.wantR || got.G != tc.wantG || got.B != tc.wantB {
			t.Errorf("hslToRGB(%v,%v,%v) = %v; want R=%d G=%d B=%d",
				tc.h, tc.s, tc.l, got, tc.wantR, tc.wantG, tc.wantB)
		}
	}
}

// TestIntroRampsProceduralLayers guards the "it just started raining" entrance:
// the procedural sheet + front plane are tick-driven and used to render at full
// density the instant a client joined, so triggering an intro only thinned the
// foreground drops while the bulk of the field popped in fully formed. The intro
// must ramp the WHOLE field from near-empty.
func TestIntroRampsProceduralLayers(t *testing.T) {
	r := NewRain(80, 45, 11, Config{
		SheetDensity:  0.8,
		SheetStrength: 0.6,
		FrontDensity:  0.4,
		FrontStrength: 0.6,
		SpawnEvery:    100000, // suppress foreground drops so we measure the procedural layers
		IntroDur:      60,
	})

	if !r.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to be recognized")
	}
	// Right after the trigger the field must be (near) empty, not a full storm.
	atTrigger := countFilledPixels(r.Grid)

	// A few ticks in: still early in the ramp, sparse.
	for i := 0; i < 5; i++ {
		r.Step()
	}
	early := countFilledPixels(r.Grid)

	// Near the end of the intro: the field should be substantially fuller.
	for r.CurrentTick() < 58 {
		r.Step()
	}
	late := countFilledPixels(r.Grid)

	if atTrigger > 40 {
		t.Fatalf("field not near-empty at intro trigger: %d filled pixels", atTrigger)
	}
	if late <= early*3 {
		t.Fatalf("procedural field did not ramp: early=%d late=%d (want late >> early)", early, late)
	}
}

func countFilledPixels(grid [][]Pixel) int {
	filled := 0
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Filled {
				filled++
			}
		}
	}
	return filled
}

func filledColumns(grid [][]Pixel) map[int]bool {
	cols := map[int]bool{}
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Filled {
				cols[x] = true
			}
		}
	}
	return cols
}

func sameIntSet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func TestPersistedStateRestoreRoundTrip(t *testing.T) {
	r := NewRain(20, 20, 42, Config{
		Speed:      1.2,
		StreakLen:  4,
		FadeFactor: 0.9,
		SpawnEvery: 2,
		SpawnBurst: 2,
	})
	for i := 0; i < 10; i++ {
		r.Step()
	}

	state := r.SnapshotPersistedState()

	restored := NewRain(20, 20, 1, Config{})
	restored.SetConfig(r.EffectiveConfig())
	restored.RestorePersistedState(state)

	got := restored.SnapshotPersistedState()
	if got.Tick != state.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, state.Tick)
	}
	if got.DownpourTicks != state.DownpourTicks || got.CalmTicks != state.CalmTicks || got.GustTicks != state.GustTicks {
		t.Fatalf("event timers changed: got %+v want %+v", got.State, state.State)
	}
	if got.RNGState != state.RNGState {
		t.Fatalf("rng state = %d, want %d", got.RNGState, state.RNGState)
	}
	if len(got.Drops) != len(state.Drops) {
		t.Fatalf("drops = %d, want %d", len(got.Drops), len(state.Drops))
	}
	if len(got.Splashes) != len(state.Splashes) {
		t.Fatalf("splashes = %d, want %d", len(got.Splashes), len(state.Splashes))
	}
}

func TestTriggerIntroResetsAtmosphereAndSeedsLeadingDrops(t *testing.T) {
	r := NewRain(40, 20, 7, Config{
		SpawnEvery:  1000,
		IntroStyle:  1,
		IntroDur:    40,
		IntroOpen:   0.1,
		IntroSparse: 6,
		IntroSeed:   5,
	})
	r.drops = append(r.drops, drop{
		Row: 2, Col: 20,
		Color: staticRed, vRow: 1, vCol: 0,
		streakLen: 3,
	})
	r.splashes = append(r.splashes, splashInstance{row: 3, col: 3, maxAge: 4, maxRadius: 2})
	r.downpourTicks = 9
	r.calmTicks = 7
	r.gustTicks = 5
	r.gustWind = 1.5
	r.endingTicks = 11
	r.endingTotal = 20
	r.endingFade = 12
	r.endingSplashLeft = 2
	r.endingSplashTotal = 3

	if !r.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to be recognized")
	}

	state := r.SnapshotState()
	if state.IntroTicks != 40 || state.IntroTotal != 40 {
		t.Fatalf("intro state = %+v, want intro ticks/total 40", state)
	}
	if state.DownpourTicks != 0 || state.CalmTicks != 0 || state.GustTicks != 0 || state.GustWind != 0 {
		t.Fatalf("weather timers not cleared by intro: %+v", state)
	}
	if state.EndingTicks != 0 || state.EndingTotal != 0 || state.EndingFade != 0 || state.EndingSplashLeft != 0 || state.EndingSplashTotal != 0 {
		t.Fatalf("ending state not cleared by intro: %+v", state)
	}
	if len(r.splashes) != 0 {
		t.Fatalf("splashes = %d, want 0", len(r.splashes))
	}
	if len(r.drops) != 5 {
		t.Fatalf("seeded drops = %d, want 5", len(r.drops))
	}
	for _, d := range r.drops {
		if d.Col < 0 || d.Col > 4 {
			t.Fatalf("seeded intro drop col = %.2f, want within leading curtain", d.Col)
		}
	}
}

func TestPersistedStatePreservesIntroProgress(t *testing.T) {
	r := NewRain(20, 20, 9, Config{
		SpawnEvery: 1000,
		IntroDur:   30,
		IntroSeed:  2,
	})
	if !r.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to be recognized")
	}
	r.Step()

	state := r.SnapshotPersistedState()
	restored := NewRain(20, 20, 1, Config{})
	restored.SetConfig(r.EffectiveConfig())
	restored.RestorePersistedState(state)

	got := restored.SnapshotState()
	if got.IntroTicks != state.IntroTicks || got.IntroTotal != state.IntroTotal {
		t.Fatalf("intro state = %+v, want ticks=%d total=%d", got, state.IntroTicks, state.IntroTotal)
	}
}

func TestTriggerEndingStartsOutroWithoutHardReset(t *testing.T) {
	r := NewRain(40, 20, 11, Config{
		SpawnEvery:     1000,
		EndingStyle:    3,
		EndingDur:      30,
		EndingLinger:   15,
		EndingSplashes: 4,
	})
	r.drops = append(r.drops, drop{
		Row: 2, Col: 20,
		Color: staticRed, vRow: 1, vCol: 0,
		streakLen: 3,
	})
	r.splashes = append(r.splashes, splashInstance{row: 3, col: 3, maxAge: 4, maxRadius: 2})
	r.downpourTicks = 9
	r.calmTicks = 7
	r.gustTicks = 5
	r.gustWind = 1.5
	r.introTicks = 13
	r.introTotal = 20

	if !r.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to be recognized")
	}

	state := r.SnapshotState()
	if state.EndingTicks != 45 || state.EndingTotal != 45 || state.EndingFade != 30 {
		t.Fatalf("ending state = %+v, want ending ticks/total 45 fade 30", state)
	}
	if state.EndingSplashLeft != 4 || state.EndingSplashTotal != 4 {
		t.Fatalf("ending splash budget = %+v, want 4", state)
	}
	if state.DownpourTicks != 0 || state.CalmTicks != 0 || state.GustTicks != 0 || state.GustWind != 0 {
		t.Fatalf("weather timers not cleared by ending: %+v", state)
	}
	if state.IntroTicks != 0 || state.IntroTotal != 0 {
		t.Fatalf("intro state not cleared by ending: %+v", state)
	}
	if len(r.drops) != 1 {
		t.Fatalf("ending should keep live drops, got %d", len(r.drops))
	}
	if len(r.splashes) != 1 {
		t.Fatalf("ending should keep live splashes, got %d", len(r.splashes))
	}
}

func TestEndingDirectionalSpawnNarrowsToLane(t *testing.T) {
	r := NewRain(40, 20, 5, Config{EndingStyle: 1})
	r.spawnEndingDrop(0.8)
	if len(r.drops) != 1 {
		t.Fatalf("drops = %d, want 1", len(r.drops))
	}
	if r.drops[0].Col < 0 || r.drops[0].Col > 8 {
		t.Fatalf("ending drop col = %.2f, want near left lane", r.drops[0].Col)
	}
}

func TestPersistedStatePreservesEndingProgress(t *testing.T) {
	r := NewRain(20, 20, 15, Config{
		SpawnEvery:     1000,
		EndingDur:      24,
		EndingLinger:   10,
		EndingSplashes: 5,
	})
	if !r.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to be recognized")
	}
	for i := 0; i < 12; i++ {
		r.Step()
	}

	state := r.SnapshotPersistedState()
	restored := NewRain(20, 20, 1, Config{})
	restored.SetConfig(r.EffectiveConfig())
	restored.RestorePersistedState(state)

	got := restored.SnapshotState()
	if got.EndingTicks != state.EndingTicks || got.EndingTotal != state.EndingTotal || got.EndingFade != state.EndingFade {
		t.Fatalf("ending state = %+v, want ticks=%d total=%d fade=%d", got, state.EndingTicks, state.EndingTotal, state.EndingFade)
	}
	if got.EndingSplashLeft != state.EndingSplashLeft || got.EndingSplashTotal != state.EndingSplashTotal {
		t.Fatalf("ending splash state = %+v, want left=%d total=%d", got, state.EndingSplashLeft, state.EndingSplashTotal)
	}
}
