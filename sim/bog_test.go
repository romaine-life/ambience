package sim

import "testing"

func TestBogSchemaContainsTriggers(t *testing.T) {
	schema := BogSchema()
	if schema.Name != "bog" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{"bubble-burst": true, "calm": true, "surge": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestBogStepSpawnsBubblesAndPaintsGrid(t *testing.T) {
	b := NewBog(60, 40, 7, BogConfig{
		SpawnEvery: 1,
		MaxBubbles: 12,
		Rise:       0.6,
		WobbleAmp:  0.5,
		BubbleSize: 1.5,
		Surface:    0.35,
	})
	for i := 0; i < 30; i++ {
		b.Step()
	}
	snap := b.Snapshot()
	if len(snap.Bubbles) == 0 {
		t.Fatal("expected bubbles to have spawned after 30 ticks")
	}
	// At least some bubbles should have risen out of the deepest row.
	bottom := float64(b.H - 1)
	rising := 0
	for _, bb := range snap.Bubbles {
		if bb.Row < bottom-0.5 {
			rising++
		}
	}
	if rising == 0 {
		t.Fatal("expected at least one bubble to have risen above the floor")
	}
	filled := 0
	for y := range b.Grid {
		for x := range b.Grid[y] {
			if b.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected painted pixels after stepping")
	}
}

func TestBogBubblePopAtSurfaceEmitsRipple(t *testing.T) {
	b := NewBog(40, 30, 11, BogConfig{
		SpawnEvery: 1,
		MaxBubbles: 6,
		Rise:       1.5, // fast rise so bubbles reach the surface quickly
		WobbleAmp:  0,
		BubbleSize: 1.5,
		Surface:    0.5,
	})
	for i := 0; i < 80; i++ {
		b.Step()
	}
	snap := b.Snapshot()
	if len(snap.Ripples) == 0 {
		t.Fatal("expected at least one ripple after bubbles popped at the surface")
	}
}

func TestBogTriggerEventAndSnapshotRoundTrip(t *testing.T) {
	b := NewBog(48, 32, 23, BogConfig{
		SpawnEvery: 1,
		MaxBubbles: 8,
		Rise:       0.4,
		WobbleAmp:  0.3,
		BubbleSize: 1.5,
		Surface:    0.4,
		GushDur:    20,
		CalmDur:    40,
		SurgeDur:   30,
	})
	for i := 0; i < 6; i++ {
		b.Step()
	}
	if !b.TriggerEvent("bubble-burst") {
		t.Fatal("expected bubble-burst trigger to succeed")
	}
	if !b.TriggerEvent("calm") {
		t.Fatal("expected calm trigger to succeed")
	}
	if !b.TriggerEvent("surge") {
		t.Fatal("expected surge trigger to succeed")
	}
	if b.TriggerEvent("nonsense") {
		t.Fatal("expected unknown trigger to be rejected")
	}
	snap := b.Snapshot()
	if snap.GushTicks <= 0 || snap.CalmTicks <= 0 || snap.SurgeTicks <= 0 {
		t.Fatalf("expected all event timers to be active, got %+v", snap.BogState)
	}

	restored := NewBog(48, 32, 9, BogConfig{})
	restored.SetConfig(b.EffectiveConfig())
	restored.RestoreSnapshot(snap)

	got := restored.Snapshot()
	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.GushTicks != snap.GushTicks {
		t.Fatalf("gushTicks = %d, want %d", got.GushTicks, snap.GushTicks)
	}
	if got.CalmTicks != snap.CalmTicks {
		t.Fatalf("calmTicks = %d, want %d", got.CalmTicks, snap.CalmTicks)
	}
	if got.SurgeTicks != snap.SurgeTicks {
		t.Fatalf("surgeTicks = %d, want %d", got.SurgeTicks, snap.SurgeTicks)
	}
	if len(got.Bubbles) != len(snap.Bubbles) {
		t.Fatalf("bubble count = %d, want %d", len(got.Bubbles), len(snap.Bubbles))
	}
	if got.RNGState != snap.RNGState {
		t.Fatalf("rng state mismatch after restore")
	}
}
