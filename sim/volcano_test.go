package sim

import "testing"

func TestVolcanoSchema(t *testing.T) {
	schema := VolcanoSchema()
	if schema.Name != "volcano" {
		t.Fatalf("schema name = %q, want volcano", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected volcano schema knobs")
	}
}

func TestVolcanoSnapshotRestore(t *testing.T) {
	p := NewVolcano(160, 80, 18, nil)
	if !p.TriggerEvent("eruption") {
		t.Fatal("expected eruption trigger to succeed")
	}
	if !p.TriggerEvent("flare") {
		t.Fatal("expected flare trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["eruption"] <= 0 {
		t.Fatal("expected eruption timer in snapshot")
	}
	if snap.Timers["flare"] <= 0 {
		t.Fatal("expected flare timer in snapshot")
	}

	restored := NewVolcano(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["eruption"] != snap.Timers["eruption"] {
		t.Fatalf("restored eruption timer = %d, want %d", again.Timers["eruption"], snap.Timers["eruption"])
	}
	if again.Timers["flare"] != snap.Timers["flare"] {
		t.Fatalf("restored flare timer = %d, want %d", again.Timers["flare"], snap.Timers["flare"])
	}
}
