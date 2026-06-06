package sim

import (
	"image/color"
	"testing"
)

var staticRed = color.RGBA{255, 100, 100, 255}

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
	r.Step()

	filled := countFilledPixels(r.Grid)
	if filled < 80 {
		t.Fatalf("front-plane rain filled %d grid cells, want at least 80", filled)
	}
	if len(r.drops) != 0 {
		t.Fatalf("tracked drops = %d, want 0 for procedural front-plane streaks", len(r.drops))
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
