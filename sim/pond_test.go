package sim

import "testing"

func TestPondSchemaContainsTriggers(t *testing.T) {
	schema := PondSchema()
	if schema.Name != "pond" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{"intro": true, "ending": true, "dive": true, "quack": true, "gust": true, "calm": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestPondStepAdvancesDucksAndPaintsGrid(t *testing.T) {
	p := NewPond(80, 50, 11, PondConfig{
		DuckCount: 3,
		CircleMin: 5,
		CircleMax: 9,
		SwimSpeed: 0.08,
		Waterline: 0.4,
		WakeRate:  1,
	})
	startAngles := make([]float64, len(p.ducks))
	for i, d := range p.ducks {
		startAngles[i] = d.Angle
	}
	for i := 0; i < 40; i++ {
		p.Step()
	}
	snap := p.Snapshot()
	if len(snap.Ducks) != 3 {
		t.Fatalf("expected 3 ducks, got %d", len(snap.Ducks))
	}
	moved := 0
	for i, d := range snap.Ducks {
		if d.Angle != startAngles[i] {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("expected at least one duck's angle to change after 40 ticks")
	}
	filled := 0
	for y := range p.Grid {
		for x := range p.Grid[y] {
			if p.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected painted pixels after stepping")
	}
}

func TestPondDiveTriggerEmitsRipple(t *testing.T) {
	p := NewPond(60, 40, 17, PondConfig{
		DuckCount: 2,
		DiveDur:   12,
	})
	if !p.TriggerEvent("dive") {
		t.Fatal("expected dive trigger to succeed")
	}
	snap := p.Snapshot()
	if len(snap.Ripples) == 0 {
		t.Fatal("expected at least one ripple immediately after dive")
	}
	diving := 0
	for _, d := range snap.Ducks {
		if d.DiveLeft > 0 {
			diving++
		}
	}
	if diving != 1 {
		t.Fatalf("expected exactly one duck to be diving, got %d", diving)
	}
}

func TestPondTriggerEventAndSnapshotRoundTrip(t *testing.T) {
	p := NewPond(64, 40, 23, PondConfig{
		DuckCount: 2,
		GustDur:   30,
		CalmDur:   40,
		DiveDur:   18,
	})
	for i := 0; i < 5; i++ {
		p.Step()
	}
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	if !p.TriggerEvent("calm") {
		t.Fatal("expected calm trigger to succeed")
	}
	if !p.TriggerEvent("quack") {
		t.Fatal("expected quack trigger to succeed")
	}
	if p.TriggerEvent("nonsense") {
		t.Fatal("expected unknown trigger to be rejected")
	}
	snap := p.Snapshot()
	if snap.GustTicks <= 0 || snap.CalmTicks <= 0 {
		t.Fatalf("expected gust and calm timers active, got %+v", snap.PondState)
	}

	restored := NewPond(64, 40, 9, PondConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.RestoreSnapshot(snap)

	got := restored.Snapshot()
	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.GustTicks != snap.GustTicks {
		t.Fatalf("gustTicks = %d, want %d", got.GustTicks, snap.GustTicks)
	}
	if got.CalmTicks != snap.CalmTicks {
		t.Fatalf("calmTicks = %d, want %d", got.CalmTicks, snap.CalmTicks)
	}
	if len(got.Ducks) != len(snap.Ducks) {
		t.Fatalf("duck count = %d, want %d", len(got.Ducks), len(snap.Ducks))
	}
	if got.RNGState != snap.RNGState {
		t.Fatalf("rng state mismatch after restore")
	}
}
