package sim

import "testing"

func TestSnowSchema(t *testing.T) {
	schema := SnowSchema()
	if schema.Name != "snow" {
		t.Fatalf("schema name = %q, want snow", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected snow schema knobs")
	}
}

func TestProceduralSnowSnapshotRestore(t *testing.T) {
	p := NewProcedural("snow", 160, 80, 42, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	if !p.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Tick != 1 {
		t.Fatalf("tick = %d, want 1", snap.Tick)
	}
	if snap.Timers["intro"] <= 0 {
		t.Fatal("expected intro timer in snapshot")
	}

	restored := NewProcedural("snow", 160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Tick != snap.Tick {
		t.Fatalf("restored tick = %d, want %d", again.Tick, snap.Tick)
	}
	if again.Timers["intro"] != snap.Timers["intro"] {
		t.Fatalf("restored intro timer = %d, want %d", again.Timers["intro"], snap.Timers["intro"])
	}
}
