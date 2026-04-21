package sim

import "testing"

func TestNewFirefliesAppliesDefaults(t *testing.T) {
	f := NewFireflies(16, 12, 1, FirefliesConfig{})
	cfg := f.EffectiveConfig()
	if cfg.Drift <= 0 {
		t.Fatal("expected default drift")
	}
	if cfg.SpawnEvery <= 0 {
		t.Fatal("expected default spawn cadence")
	}
	if cfg.MaxFireflies <= 0 {
		t.Fatal("expected default max fireflies")
	}
	if cfg.LightnessMax <= 0 || cfg.LightnessMin <= 0 {
		t.Fatal("expected default lightness range")
	}
}

func TestFirefliesStepPaintsGrid(t *testing.T) {
	f := NewFireflies(20, 10, 1, FirefliesConfig{
		SpawnEvery:         1,
		MaxFireflies:       4,
		Drift:              0.2,
		Wander:             0.2,
		BlinkBurstChance:   0,
		ClusterShiftChance: 0,
		CalmChance:         0,
	})
	for i := 0; i < 8; i++ {
		f.Step()
	}

	filled := 0
	for y := range f.Grid {
		for x := range f.Grid[y] {
			if f.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected at least one painted firefly pixel")
	}
}

func TestFirefliesTriggerEventAndRestoreRoundTrip(t *testing.T) {
	f := NewFireflies(24, 16, 42, FirefliesConfig{
		SpawnEvery:         1,
		MaxFireflies:       6,
		Drift:              0.22,
		Wander:             0.3,
		BlinkBurstChance:   0,
		ClusterShiftChance: 0,
		CalmChance:         0,
	})
	for i := 0; i < 6; i++ {
		f.Step()
	}
	if !f.TriggerEvent("blink-burst") {
		t.Fatal("expected blink-burst trigger to succeed")
	}
	state := f.SnapshotState()
	fireflies := f.FirefliesCopy()

	restored := NewFireflies(24, 16, 9, FirefliesConfig{})
	restored.SetConfig(f.EffectiveConfig())
	restored.RestoreState(state)
	restored.RestoreFireflies(fireflies)

	got := restored.SnapshotState()
	if got.Tick != state.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, state.Tick)
	}
	if got.BlinkBurstTicks != state.BlinkBurstTicks {
		t.Fatalf("blink burst ticks = %d, want %d", got.BlinkBurstTicks, state.BlinkBurstTicks)
	}
	if len(restored.FirefliesCopy()) != len(fireflies) {
		t.Fatalf("firefly count = %d, want %d", len(restored.FirefliesCopy()), len(fireflies))
	}
}
