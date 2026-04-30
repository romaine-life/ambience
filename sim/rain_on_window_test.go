package sim

import "testing"

func TestRainOnWindowSpawnsAndFalls(t *testing.T) {
	r := NewRainOnWindow(160, 80, 1, RainOnWindowConfig{
		SpawnRate:     1.5,
		MaxDrops:      40,
		GrowthRate:    0.4,
		FallThreshold: 1.5,
		FallSpeed:     0.3,
		Gravity:       0.02,
		TrailLife:     30,
	})
	for i := 0; i < 5; i++ {
		r.Step()
	}
	snap := r.Snapshot()
	if len(snap.Droplets) == 0 {
		t.Fatalf("expected droplets after 5 ticks, got 0")
	}
	// March long enough that some pinned drops cross the fall threshold.
	for i := 0; i < 80; i++ {
		r.Step()
	}
	snap = r.Snapshot()
	anyFalling := false
	for _, d := range snap.Droplets {
		if d.Falling {
			anyFalling = true
			break
		}
	}
	if !anyFalling && len(snap.Tracks) == 0 {
		t.Fatalf("expected at least one falling drop or track after long sim, got %d droplets / %d tracks",
			len(snap.Droplets), len(snap.Tracks))
	}
}

func TestRainOnWindowSchemaContainsTriggers(t *testing.T) {
	schema := RainOnWindowSchema()
	if schema.Name != "rain-on-window" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{"wind-gust": true, "quiet-pane": true, "intro": true, "ending": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestRainOnWindowGustEvent(t *testing.T) {
	r := NewRainOnWindow(160, 80, 7, RainOnWindowConfig{GustDur: 20, GustStrength: 0.5})
	if !r.TriggerEvent("wind-gust") {
		t.Fatalf("expected wind-gust trigger to be accepted")
	}
	snap := r.Snapshot()
	if snap.GustTicks <= 0 {
		t.Fatalf("expected gust ticks > 0 after trigger, got %d", snap.GustTicks)
	}
	if snap.GustSide == 0 {
		t.Fatalf("expected gust side ±1 after trigger, got %d", snap.GustSide)
	}
}

func TestRainOnWindowIntroAndEnding(t *testing.T) {
	r := NewRainOnWindow(160, 80, 13, RainOnWindowConfig{IntroDur: 30, EndingDur: 30, EndingLinger: 5})
	r.TriggerEvent("intro")
	if r.Snapshot().IntroTicks <= 0 {
		t.Fatalf("expected intro_ticks > 0 after intro trigger")
	}
	r.TriggerEvent("ending")
	snap := r.Snapshot()
	if snap.IntroTicks != 0 {
		t.Fatalf("expected intro to be canceled by ending, got intro_ticks=%d", snap.IntroTicks)
	}
	if snap.EndingTicks <= 0 {
		t.Fatalf("expected ending_ticks > 0 after ending trigger, got %d", snap.EndingTicks)
	}
}

func TestRainOnWindowSnapshotRoundTrip(t *testing.T) {
	a := NewRainOnWindow(160, 80, 21, RainOnWindowConfig{SpawnRate: 1, GrowthRate: 0.2})
	for i := 0; i < 30; i++ {
		a.Step()
	}
	snap := a.Snapshot()
	b := NewRainOnWindow(160, 80, 0, RainOnWindowConfig{})
	b.RestoreSnapshot(snap)
	bSnap := b.Snapshot()
	if bSnap.Tick != snap.Tick {
		t.Fatalf("tick mismatch after restore: got %d want %d", bSnap.Tick, snap.Tick)
	}
	if len(bSnap.Droplets) != len(snap.Droplets) {
		t.Fatalf("droplet count mismatch after restore: got %d want %d",
			len(bSnap.Droplets), len(snap.Droplets))
	}
}
