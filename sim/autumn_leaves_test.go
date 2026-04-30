package sim

import "testing"

func TestAutumnLeavesSchema(t *testing.T) {
	schema := AutumnLeavesSchema()
	if schema.Name != "autumn-leaves" {
		t.Fatalf("schema name = %q, want autumn-leaves", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected autumn leaves schema knobs")
	}
}

func TestAutumnLeavesSnapshotRestore(t *testing.T) {
	p := NewAutumnLeaves(160, 80, 99, nil)
	if !p.TriggerEvent("swirl") {
		t.Fatal("expected swirl trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Tick != 1 {
		t.Fatalf("tick = %d, want 1", snap.Tick)
	}
	if snap.Timers["swirl"] <= 0 {
		t.Fatal("expected swirl timer in snapshot")
	}

	restored := NewAutumnLeaves(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["swirl"] != snap.Timers["swirl"] {
		t.Fatalf("restored swirl timer = %d, want %d", again.Timers["swirl"], snap.Timers["swirl"])
	}
}
