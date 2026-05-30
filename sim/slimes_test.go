package sim

import "testing"

func TestSlimesSchemaContainsTriggers(t *testing.T) {
	schema := SlimesSchema()
	if schema.Name != "slimes" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{"hop": true, "wave": true, "calm": true, "big-hop": true, "intro": true, "ending": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestSlimesHopFlow(t *testing.T) {
	s := NewSlimes(120, 60, 1, SlimesConfig{
		SlimeCount: 5,
		HopPower:   2.0,
		Gravity:    0.25,
		SquashDur:  4,
	})
	if !s.TriggerEvent("hop") {
		t.Fatal("expected hop trigger to succeed")
	}
	snap := s.Snapshot()
	hopper := -1
	for i, sl := range snap.Slimes {
		if sl.State == SlimeStateHopping {
			hopper = i
			break
		}
	}
	if hopper < 0 {
		t.Fatalf("expected one slime to be hopping after trigger, got %+v", snap.Slimes)
	}
	// March the sim long enough for the hop to land and the squash to finish.
	for i := 0; i < 60; i++ {
		s.Step()
	}
	post := s.Snapshot()
	if post.Slimes[hopper].State != SlimeStateSitting {
		t.Fatalf("expected slime %d to be sitting after hop+land, got state %d", hopper, post.Slimes[hopper].State)
	}
}

func TestSlimesStepPaintsGrid(t *testing.T) {
	s := NewSlimes(80, 40, 9, SlimesConfig{
		SlimeCount: 4,
		HopChance:  0.0,
		Baseline:   0.7,
	})
	for i := 0; i < 4; i++ {
		s.Step()
	}
	grid := s.GridCopy()
	if len(grid) != 40 {
		t.Fatalf("grid height = %d, want 40", len(grid))
	}
	filled := 0
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected painted meadow + slime pixels")
	}
}

func TestSlimesIntroResetsAndShrinks(t *testing.T) {
	s := NewSlimes(80, 40, 3, SlimesConfig{SlimeCount: 4, IntroDur: 20, IntroGrowth: 0.2})
	s.TriggerEvent("hop")
	s.TriggerEvent("intro")
	snap := s.Snapshot()
	for i, sl := range snap.Slimes {
		if sl.State != SlimeStateSitting {
			t.Fatalf("slime %d not sitting after intro: state %d", i, sl.State)
		}
	}
	if snap.IntroTicks <= 0 {
		t.Fatalf("expected intro_ticks > 0 after intro trigger, got %d", snap.IntroTicks)
	}
}

func TestSlimesSnapshotRoundTrip(t *testing.T) {
	s := NewSlimes(96, 48, 17, SlimesConfig{
		SlimeCount: 6,
		HopPower:   1.5,
		Gravity:    0.2,
		SquashDur:  6,
		WaveDur:    40,
		CalmDur:    50,
		BigHopDur:  30,
	})
	for i := 0; i < 8; i++ {
		s.Step()
	}
	if !s.TriggerEvent("wave") {
		t.Fatal("wave trigger should succeed")
	}
	if !s.TriggerEvent("calm") {
		t.Fatal("calm trigger should succeed")
	}
	if !s.TriggerEvent("big-hop") {
		t.Fatal("big-hop trigger should succeed")
	}
	if s.TriggerEvent("nonsense") {
		t.Fatal("unknown trigger should be rejected")
	}
	snap := s.Snapshot()
	if snap.WaveTicks <= 0 || snap.CalmTicks <= 0 || snap.BigHopTicks <= 0 {
		t.Fatalf("expected all event timers active, got %+v", snap.SlimesState)
	}

	restored := NewSlimes(96, 48, 0, SlimesConfig{})
	restored.SetConfig(s.EffectiveConfig())
	restored.RestoreSnapshot(snap)

	got := restored.Snapshot()
	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.WaveTicks != snap.WaveTicks {
		t.Fatalf("waveTicks = %d, want %d", got.WaveTicks, snap.WaveTicks)
	}
	if got.CalmTicks != snap.CalmTicks {
		t.Fatalf("calmTicks = %d, want %d", got.CalmTicks, snap.CalmTicks)
	}
	if got.BigHopTicks != snap.BigHopTicks {
		t.Fatalf("bigHopTicks = %d, want %d", got.BigHopTicks, snap.BigHopTicks)
	}
	if len(got.Slimes) != len(snap.Slimes) {
		t.Fatalf("slime count = %d, want %d", len(got.Slimes), len(snap.Slimes))
	}
	if got.RNGState != snap.RNGState {
		t.Fatal("rng state mismatch after restore")
	}
}
