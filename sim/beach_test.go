package sim

import "testing"

func TestBeachSchema(t *testing.T) {
	schema := BeachSchema()
	if schema.Name != "beach" {
		t.Fatalf("schema name = %q, want beach", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected beach schema knobs")
	}
}

func TestBeachSnapshotRestore(t *testing.T) {
	p := NewBeach(160, 80, 91, nil)
	if !p.TriggerEvent("foam-burst") {
		t.Fatal("expected foam-burst trigger to succeed")
	}
	if !p.TriggerEvent("high-tide") {
		t.Fatal("expected high-tide trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["foam-burst"] <= 0 {
		t.Fatal("expected foam-burst timer in snapshot")
	}
	if snap.Timers["high-tide"] <= 0 {
		t.Fatal("expected high-tide timer in snapshot")
	}

	restored := NewBeach(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["foam-burst"] != snap.Timers["foam-burst"] {
		t.Fatalf("restored foam-burst timer = %d, want %d", again.Timers["foam-burst"], snap.Timers["foam-burst"])
	}
	if again.Values["tide_bias"] != snap.Values["tide_bias"] {
		t.Fatalf("restored tide bias = %f, want %f", again.Values["tide_bias"], snap.Values["tide_bias"])
	}
}
