package sim

import "testing"

func TestTrainSchema(t *testing.T) {
	schema := TrainSchema()
	if schema.Name != "train" {
		t.Fatalf("schema name = %q, want train", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected train schema knobs")
	}
}

func TestTrainSnapshotRestore(t *testing.T) {
	tr := NewTrain(160, 80, 41, nil)
	if !tr.TriggerEvent("pass") {
		t.Fatal("expected pass trigger to succeed")
	}
	if !tr.TriggerEvent("quiet-gap") {
		t.Fatal("expected quiet-gap trigger to succeed")
	}
	tr.Step()

	snap := tr.Snapshot()
	if snap.Timers["pass"] <= 0 {
		t.Fatal("expected pass timer in snapshot")
	}
	if snap.Timers["quiet-gap"] <= 0 {
		t.Fatal("expected quiet-gap timer in snapshot")
	}
	if snap.Values["pass_total"] <= 0 {
		t.Fatal("expected pass_total in snapshot values")
	}
	dir := snap.Values["pass_dir"]
	if dir != 1 && dir != -1 {
		t.Fatalf("pass_dir = %f, want ±1", dir)
	}

	restored := NewTrain(160, 80, 9, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["pass"] != snap.Timers["pass"] {
		t.Fatalf("restored pass timer = %d, want %d", again.Timers["pass"], snap.Timers["pass"])
	}
	if again.Timers["quiet-gap"] != snap.Timers["quiet-gap"] {
		t.Fatalf("restored quiet-gap timer = %d, want %d", again.Timers["quiet-gap"], snap.Timers["quiet-gap"])
	}
	if again.Values["pass_dir"] != snap.Values["pass_dir"] {
		t.Fatalf("restored pass_dir = %f, want %f", again.Values["pass_dir"], snap.Values["pass_dir"])
	}
}

func TestTrainExpressCancelsPass(t *testing.T) {
	tr := NewTrain(160, 80, 17, nil)
	tr.TriggerEvent("pass")
	if !tr.TriggerEvent("express") {
		t.Fatal("expected express trigger to succeed")
	}
	snap := tr.Snapshot()
	if snap.Timers["pass"] != 0 {
		t.Fatalf("pass timer should clear when express starts; got %d", snap.Timers["pass"])
	}
	if snap.Timers["express"] <= 0 {
		t.Fatal("expected express timer to be active")
	}
}
