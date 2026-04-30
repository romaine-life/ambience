package sim

import "testing"

func TestWheatFieldSchema(t *testing.T) {
	schema := WheatFieldSchema()
	if schema.Name != "wheat-field" {
		t.Fatalf("schema name = %q, want wheat-field", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected wheat-field schema knobs")
	}
}

func TestWheatFieldSnapshotRestore(t *testing.T) {
	p := NewWheatField(160, 80, 88, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["gust"] <= 0 {
		t.Fatal("expected gust timer in snapshot")
	}
	if snap.Values["gust_push"] == 0 {
		t.Fatal("expected gust push value in snapshot")
	}

	restored := NewWheatField(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Values["gust_push"] != snap.Values["gust_push"] {
		t.Fatalf("restored gust push = %f, want %f", again.Values["gust_push"], snap.Values["gust_push"])
	}
}
