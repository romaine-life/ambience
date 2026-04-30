package sim

import "testing"

func TestCampfireSchema(t *testing.T) {
	schema := CampfireSchema()
	if schema.Name != "campfire" {
		t.Fatalf("schema name = %q, want campfire", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected campfire schema knobs")
	}
}

func TestCampfireSnapshotRestore(t *testing.T) {
	p := NewCampfire(160, 80, 33, nil)
	if !p.TriggerEvent("crackle") {
		t.Fatal("expected crackle trigger to succeed")
	}
	if !p.TriggerEvent("lull") {
		t.Fatal("expected lull trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["crackle"] <= 0 {
		t.Fatal("expected crackle timer in snapshot")
	}
	if snap.Timers["lull"] <= 0 {
		t.Fatal("expected lull timer in snapshot")
	}
	if snap.Values["crackle_gain"] <= 1 {
		t.Fatal("expected crackle gain value in snapshot")
	}

	restored := NewCampfire(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["crackle"] != snap.Timers["crackle"] {
		t.Fatalf("restored crackle timer = %d, want %d", again.Timers["crackle"], snap.Timers["crackle"])
	}
	if again.Timers["lull"] != snap.Timers["lull"] {
		t.Fatalf("restored lull timer = %d, want %d", again.Timers["lull"], snap.Timers["lull"])
	}
	if again.Values["crackle_gain"] != snap.Values["crackle_gain"] {
		t.Fatalf("restored crackle gain = %f, want %f", again.Values["crackle_gain"], snap.Values["crackle_gain"])
	}
}
