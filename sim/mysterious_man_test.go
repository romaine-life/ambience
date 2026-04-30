package sim

import "testing"

func TestMysteriousManSchema(t *testing.T) {
	schema := MysteriousManSchema()
	if schema.Name != "mysterious-man" {
		t.Fatalf("schema name = %q, want mysterious-man", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected mysterious-man schema knobs")
	}
}

func TestMysteriousManSnapshotRestore(t *testing.T) {
	p := NewMysteriousMan(160, 80, 21, nil)
	if !p.TriggerEvent("inhale") {
		t.Fatal("expected inhale trigger to succeed")
	}
	if !p.TriggerEvent("exhale") {
		t.Fatal("expected exhale trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["inhale"] <= 0 {
		t.Fatal("expected inhale timer in snapshot")
	}
	if snap.Timers["exhale"] <= 0 {
		t.Fatal("expected exhale timer in snapshot")
	}

	restored := NewMysteriousMan(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["inhale"] != snap.Timers["inhale"] {
		t.Fatalf("restored inhale timer = %d, want %d", again.Timers["inhale"], snap.Timers["inhale"])
	}
	if again.Timers["exhale"] != snap.Timers["exhale"] {
		t.Fatalf("restored exhale timer = %d, want %d", again.Timers["exhale"], snap.Timers["exhale"])
	}
}
