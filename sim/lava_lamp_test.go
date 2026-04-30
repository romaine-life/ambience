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
	if snap.Timers["quiet"] <= 0 {
		t.Fatal("expected quiet timer in snapshot")
	}

	restored := NewLavaLamp(160, 80, 99, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["rise"] != snap.Timers["rise"] {
		t.Fatalf("restored rise timer = %d, want %d", again.Timers["rise"], snap.Timers["rise"])
	}
	if again.Timers["quiet"] != snap.Timers["quiet"] {
		t.Fatalf("restored quiet timer = %d, want %d", again.Timers["quiet"], snap.Timers["quiet"])
	}
}

func TestLavaLampLifecycleTriggers(t *testing.T) {
	l := NewLavaLamp(80, 40, 5, nil)
	if !l.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	snap := l.Snapshot()
	if snap.Timers["intro"] <= 0 {
		t.Fatal("expected intro timer set after intro trigger")
	}
	if !l.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to succeed")
	}
	snap = l.Snapshot()
	if snap.Timers["ending"] <= 0 {
		t.Fatal("expected ending timer set after ending trigger")
	}
	if snap.Timers["intro"] != 0 {
		t.Fatal("intro timer should clear when ending triggers")
	}
}
