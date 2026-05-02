package sim

import "testing"

func TestLavaLampSchema(t *testing.T) {
	schema := LavaLampSchema()
	if schema.Name != "lava-lamp" {
		t.Fatalf("schema name = %q, want lava-lamp", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected lava-lamp schema knobs")
	}
}

func TestLavaLampSnapshotRestore(t *testing.T) {
	l := NewLavaLamp(160, 80, 21, nil)
	if !l.TriggerEvent("blob-rise") {
		t.Fatal("expected blob-rise trigger to succeed")
	}
	if !l.TriggerEvent("quiet-flow") {
		t.Fatal("expected quiet-flow trigger to succeed")
	}
	l.Step()

	snap := l.Snapshot()
	if snap.Timers["rise"] <= 0 {
		t.Fatal("expected rise timer in snapshot")
	}
	if snap.Timers["quiet_flow"] <= 0 {
		t.Fatal("expected quiet_flow timer in snapshot")
	}

	restored := NewLavaLamp(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["rise"] != snap.Timers["rise"] {
		t.Fatalf("restored rise timer = %d, want %d", again.Timers["rise"], snap.Timers["rise"])
	}
	if again.Timers["quiet_flow"] != snap.Timers["quiet_flow"] {
		t.Fatalf("restored quiet_flow timer = %d, want %d", again.Timers["quiet_flow"], snap.Timers["quiet_flow"])
	}
}

func TestLavaLampLifecycleEvents(t *testing.T) {
	l := NewLavaLamp(160, 80, 9, nil)
	if !l.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	if l.Snapshot().Timers["intro"] <= 0 {
		t.Fatal("expected intro timer after trigger")
	}
	if !l.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to succeed")
	}
	snap := l.Snapshot()
	if snap.Timers["intro"] != 0 {
		t.Fatalf("intro should clear after ending, got %d", snap.Timers["intro"])
	}
	if snap.Timers["ending"] <= 0 {
		t.Fatal("expected ending timer after trigger")
	}
}

func TestLavaLampUnknownTrigger(t *testing.T) {
	l := NewLavaLamp(160, 80, 1, nil)
	if l.TriggerEvent("not-a-real-event") {
		t.Fatal("expected unknown trigger to return false")
	}
}
