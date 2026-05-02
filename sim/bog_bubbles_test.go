package sim

import "testing"

func TestBogBubblesSchema(t *testing.T) {
	schema := BogBubblesSchema()
	if schema.Name != "bog-bubbles" {
		t.Fatalf("schema name = %q, want bog-bubbles", schema.Name)
	}
	want := map[string]bool{"intro": true, "ending": true, "methane-burst": true, "quiet-bog": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestBogBubblesSnapshotRestore(t *testing.T) {
	b := NewBogBubbles(160, 80, 17, nil)
	if !b.TriggerEvent("methane-burst") {
		t.Fatal("expected methane-burst trigger to succeed")
	}
	b.Step()

	snap := b.Snapshot()
	if snap.Timers["methane-burst"] <= 0 {
		t.Fatal("expected methane-burst timer in snapshot")
	}
	if snap.Values["spawn_gain"] <= 1 {
		t.Fatalf("expected spawn_gain > 1 during burst, got %f", snap.Values["spawn_gain"])
	}

	restored := NewBogBubbles(160, 80, 0, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["methane-burst"] != snap.Timers["methane-burst"] {
		t.Fatalf("restored methane-burst timer = %d, want %d", again.Timers["methane-burst"], snap.Timers["methane-burst"])
	}
	if again.Values["spawn_gain"] != snap.Values["spawn_gain"] {
		t.Fatalf("restored spawn_gain = %f, want %f", again.Values["spawn_gain"], snap.Values["spawn_gain"])
	}
}

func TestBogBubblesQuietCancelsBurst(t *testing.T) {
	b := NewBogBubbles(160, 80, 5, nil)
	if !b.TriggerEvent("methane-burst") {
		t.Fatal("expected methane-burst trigger to succeed")
	}
	if !b.TriggerEvent("quiet-bog") {
		t.Fatal("expected quiet-bog trigger to succeed")
	}
	snap := b.Snapshot()
	if snap.Timers["methane-burst"] != 0 {
		t.Fatalf("expected methane-burst timer cleared after quiet-bog, got %d", snap.Timers["methane-burst"])
	}
	if snap.Timers["quiet-bog"] <= 0 {
		t.Fatalf("expected quiet-bog timer set, got %d", snap.Timers["quiet-bog"])
	}
}

func TestBogBubblesIntroEndingLifecycle(t *testing.T) {
	b := NewBogBubbles(160, 80, 9, BogBubblesConfig{"intro_dur": 12, "ending_dur": 8, "ending_linger": 4})
	if !b.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	if got := b.Snapshot().Timers["intro"]; got <= 0 {
		t.Fatalf("expected intro timer > 0 after trigger, got %d", got)
	}
	if !b.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to succeed")
	}
	snap := b.Snapshot()
	if snap.Timers["intro"] != 0 {
		t.Fatalf("expected intro timer cleared after ending, got %d", snap.Timers["intro"])
	}
	if snap.Timers["ending"] <= 0 {
		t.Fatalf("expected ending timer > 0 after trigger, got %d", snap.Timers["ending"])
	}
}
