package sim

import "testing"

func TestRowboatSchema(t *testing.T) {
	schema := RowboatSchema()
	if schema.Name != "rowboat" {
		t.Fatalf("schema name = %q, want rowboat", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected rowboat schema knobs")
	}
}

func TestRowboatSnapshotRestore(t *testing.T) {
	p := NewRowboat(160, 80, 66, nil)
	if !p.TriggerEvent("wake") {
		t.Fatal("expected wake trigger to succeed")
	}
	if !p.TriggerEvent("drift") {
		t.Fatal("expected drift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["wake"] <= 0 {
		t.Fatal("expected wake timer in snapshot")
	}
	if snap.Timers["drift"] <= 0 {
		t.Fatal("expected drift timer in snapshot")
	}
	if snap.Values["wake_gain"] <= 1 {
		t.Fatal("expected wake gain value in snapshot")
	}
	if snap.Values["drift_push"] == 0 {
		t.Fatal("expected drift push value in snapshot")
	}

	restored := NewRowboat(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["wake"] != snap.Timers["wake"] {
		t.Fatalf("restored wake timer = %d, want %d", again.Timers["wake"], snap.Timers["wake"])
	}
	if again.Timers["drift"] != snap.Timers["drift"] {
		t.Fatalf("restored drift timer = %d, want %d", again.Timers["drift"], snap.Timers["drift"])
	}
	if again.Values["wake_gain"] != snap.Values["wake_gain"] {
		t.Fatalf("restored wake gain = %f, want %f", again.Values["wake_gain"], snap.Values["wake_gain"])
	}
	if again.Values["drift_push"] != snap.Values["drift_push"] {
		t.Fatalf("restored drift push = %f, want %f", again.Values["drift_push"], snap.Values["drift_push"])
	}
}
