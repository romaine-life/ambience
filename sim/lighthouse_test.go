package sim

import "testing"

func TestLighthouseSchema(t *testing.T) {
	schema := LighthouseSchema()
	if schema.Name != "lighthouse" {
		t.Fatalf("schema name = %q, want lighthouse", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected lighthouse schema knobs")
	}
}

func TestLighthouseSnapshotRestore(t *testing.T) {
	p := NewLighthouse(160, 80, 55, nil)
	if !p.TriggerEvent("bright-pass") {
		t.Fatal("expected bright-pass trigger to succeed")
	}
	if !p.TriggerEvent("fog-thicken") {
		t.Fatal("expected fog-thicken trigger to succeed")
	}
	p.Step()

	snap := p.Snapshot()
	if snap.Timers["bright-pass"] <= 0 {
		t.Fatal("expected bright-pass timer in snapshot")
	}
	if snap.Timers["fog-thicken"] <= 0 {
		t.Fatal("expected fog-thicken timer in snapshot")
	}
	if snap.Values["bright_gain"] <= 1 {
		t.Fatal("expected bright gain value in snapshot")
	}
	if snap.Values["fog_gain"] <= 1 {
		t.Fatal("expected fog gain value in snapshot")
	}

	restored := NewLighthouse(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["bright-pass"] != snap.Timers["bright-pass"] {
		t.Fatalf("restored bright-pass timer = %d, want %d", again.Timers["bright-pass"], snap.Timers["bright-pass"])
	}
	if again.Timers["fog-thicken"] != snap.Timers["fog-thicken"] {
		t.Fatalf("restored fog-thicken timer = %d, want %d", again.Timers["fog-thicken"], snap.Timers["fog-thicken"])
	}
	if again.Values["bright_gain"] != snap.Values["bright_gain"] {
		t.Fatalf("restored bright gain = %f, want %f", again.Values["bright_gain"], snap.Values["bright_gain"])
	}
	if again.Values["fog_gain"] != snap.Values["fog_gain"] {
		t.Fatalf("restored fog gain = %f, want %f", again.Values["fog_gain"], snap.Values["fog_gain"])
	}
}
