package sim

import "testing"

func TestStarfieldSchema(t *testing.T) {
	schema := StarfieldSchema()
	if schema.Name != "starfield" {
		t.Fatalf("schema name = %q, want starfield", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected starfield schema knobs")
	}
}

func TestStarfieldSnapshotRestore(t *testing.T) {
	p := NewStarfield(160, 80, 12, nil)
	if !p.TriggerEvent("shooting-star") {
		t.Fatal("expected shooting-star trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["shooting-star"] <= 0 {
		t.Fatal("expected shooting-star timer in snapshot")
	}

	restored := NewStarfield(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shooting-star"] != snap.Timers["shooting-star"] {
		t.Fatalf("restored shooting-star timer = %d, want %d", again.Timers["shooting-star"], snap.Timers["shooting-star"])
	}
}
