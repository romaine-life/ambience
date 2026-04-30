package sim

import "testing"

func TestAuroraSchema(t *testing.T) {
	schema := AuroraSchema()
	if schema.Name != "aurora" {
		t.Fatalf("schema name = %q, want aurora", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected aurora schema knobs")
	}
}

func TestAuroraSnapshotRestore(t *testing.T) {
	p := NewAurora(160, 80, 77, nil)
	if !p.TriggerEvent("shift") {
		t.Fatal("expected shift trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["shift"] <= 0 {
		t.Fatal("expected shift timer in snapshot")
	}
	if snap.Values["shift_push"] == 0 {
		t.Fatal("expected shift push value in snapshot")
	}

	restored := NewAurora(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["shift"] != snap.Timers["shift"] {
		t.Fatalf("restored shift timer = %d, want %d", again.Timers["shift"], snap.Timers["shift"])
	}
	if again.Values["shift_push"] != snap.Values["shift_push"] {
		t.Fatalf("restored shift push = %f, want %f", again.Values["shift_push"], snap.Values["shift_push"])
	}
}
