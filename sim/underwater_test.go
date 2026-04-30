package sim

import "testing"

func TestUnderwaterSchema(t *testing.T) {
	schema := UnderwaterSchema()
	if schema.Name != "underwater" {
		t.Fatalf("schema name = %q, want underwater", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected underwater schema knobs")
	}
}

func TestUnderwaterSnapshotRestore(t *testing.T) {
	p := NewUnderwater(160, 80, 77, nil)
	if !p.TriggerEvent("bubble-burst") {
		t.Fatal("expected bubble-burst trigger to succeed")
	}
	if !p.TriggerEvent("current-shift") {
		t.Fatal("expected current-shift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["bubble-burst"] <= 0 {
		t.Fatal("expected bubble-burst timer in snapshot")
	}
	if snap.Timers["current-shift"] <= 0 {
		t.Fatal("expected current-shift timer in snapshot")
	}
	if snap.Values["bubble_gain"] <= 1 {
		t.Fatal("expected bubble gain value in snapshot")
	}
	if snap.Values["current_push"] == 0 {
		t.Fatal("expected current push value in snapshot")
	}

	restored := NewUnderwater(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["bubble-burst"] != snap.Timers["bubble-burst"] {
		t.Fatalf("restored bubble-burst timer = %d, want %d", again.Timers["bubble-burst"], snap.Timers["bubble-burst"])
	}
	if again.Timers["current-shift"] != snap.Timers["current-shift"] {
		t.Fatalf("restored current-shift timer = %d, want %d", again.Timers["current-shift"], snap.Timers["current-shift"])
	}
	if again.Values["bubble_gain"] != snap.Values["bubble_gain"] {
		t.Fatalf("restored bubble gain = %f, want %f", again.Values["bubble_gain"], snap.Values["bubble_gain"])
	}
	if again.Values["current_push"] != snap.Values["current_push"] {
		t.Fatalf("restored current push = %f, want %f", again.Values["current_push"], snap.Values["current_push"])
	}
}
