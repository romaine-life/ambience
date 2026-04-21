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
