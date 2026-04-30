package sim

import "testing"

func TestCottageChimneySchema(t *testing.T) {
	schema := CottageChimneySchema()
	if schema.Name != "cottage-chimney" {
		t.Fatalf("schema name = %q, want cottage-chimney", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected cottage-chimney schema knobs")
	}
}

func TestCottageChimneySnapshotRestore(t *testing.T) {
	c := NewCottageChimney(160, 80, 91, nil)
	if !c.TriggerEvent("wind-gust") {
		t.Fatal("expected wind-gust trigger to succeed")
	}
	if !c.TriggerEvent("lamp-flicker") {
		t.Fatal("expected lamp-flicker trigger to succeed")
	}
	c.Step()

	snap := c.Snapshot()
	if snap.Timers["gust"] <= 0 {
		t.Fatal("expected gust timer in snapshot")
	}
	if snap.Timers["flicker"] <= 0 {
		t.Fatal("expected flicker timer in snapshot")
	}
	if snap.Values["gust_drift"] <= 0 {
		t.Fatal("expected gust drift value in snapshot")
	}

	restored := NewCottageChimney(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["gust"] != snap.Timers["gust"] {
		t.Fatalf("restored gust timer = %d, want %d", again.Timers["gust"], snap.Timers["gust"])
	}
	if again.Values["gust_drift"] != snap.Values["gust_drift"] {
		t.Fatalf("restored gust drift = %f, want %f", again.Values["gust_drift"], snap.Values["gust_drift"])
	}
}

func TestCottageChimneyIntroEmitsFirstPuff(t *testing.T) {
	c := NewCottageChimney(160, 80, 13, CottageChimneyConfig{
		"intro_dur":        20,
		"intro_first_puff": 5,
	})
	if !c.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	for i := 0; i < 8; i++ {
		c.Step()
	}
	snap := c.Snapshot()
	if snap.Values["puff_count"] < 1 {
		t.Fatalf("expected at least one puff after intro_first_puff window, got %v", snap.Values["puff_count"])
	}
}
