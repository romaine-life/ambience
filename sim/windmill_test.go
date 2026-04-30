package sim

import "testing"

func TestWindmillSchema(t *testing.T) {
	schema := WindmillSchema()
	if schema.Name != "windmill" {
		t.Fatalf("schema name = %q, want windmill", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected windmill schema knobs")
	}
}

func TestWindmillSnapshotRestore(t *testing.T) {
	p := NewWindmill(160, 80, 44, nil)
	if !p.TriggerEvent("gust") {
		t.Fatal("expected gust trigger to succeed")
	}
	if !p.TriggerEvent("lull") {
		t.Fatal("expected lull trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["gust"] <= 0 {
		t.Fatal("expected gust timer in snapshot")
	}
	if snap.Timers["lull"] <= 0 {
		t.Fatal("expected lull timer in snapshot")
	}
	if snap.Values["gust_gain"] <= 1 {
		t.Fatal("expected gust gain value in snapshot")
	}

	restored := NewWindmill(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Timers["lull"] != snap.Timers["lull"] {
		t.Fatalf("restored lull timer = %d, want %d", again.Timers["lull"], snap.Timers["lull"])
	}
	if again.Values["gust_gain"] != snap.Values["gust_gain"] {
		t.Fatalf("restored gust gain = %f, want %f", again.Values["gust_gain"], snap.Values["gust_gain"])
	}
}
