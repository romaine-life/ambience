package sim

import "testing"

func TestLavaLampSchema(t *testing.T) {
	schema := LavaLampSchema()
	if schema.Name != "lava-lamp" {
		t.Fatalf("schema name = %q, want lava-lamp", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected lava-lamp schema knobs")
	}
}

func TestLavaLampSnapshotRestore(t *testing.T) {
	l := NewLavaLamp(160, 80, 42, nil)
	if !l.TriggerEvent("blob-rise") {
		t.Fatal("expected blob-rise trigger to succeed")
	}
	if !l.TriggerEvent("quiet-flow") {
		t.Fatal("expected quiet-flow trigger to succeed")
	}
	l.Step()

	snap := l.Snapshot()
	if snap.Timers["blob_rise"] <= 0 {
		t.Fatal("expected blob_rise timer in snapshot")
	}
	if snap.Timers["quiet_flow"] <= 0 {
		t.Fatal("expected quiet_flow timer in snapshot")
	}

	restored := NewLavaLamp(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["blob_rise"] != snap.Timers["blob_rise"] {
		t.Fatalf("restored blob_rise timer = %d, want %d", again.Timers["blob_rise"], snap.Timers["blob_rise"])
	}
	if again.Timers["quiet_flow"] != snap.Timers["quiet_flow"] {
		t.Fatalf("restored quiet_flow timer = %d, want %d", again.Timers["quiet_flow"], snap.Timers["quiet_flow"])
	}
}

func TestLavaLampIntroSuppressesEvents(t *testing.T) {
	l := NewLavaLamp(160, 80, 99, LavaLampConfig{
		"intro_dur":   30,
		"blob_rise_p": 0.999,
	})
	if !l.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	for i := 0; i < 5; i++ {
		l.Step()
	}
	snap := l.Snapshot()
	if snap.Timers["intro"] <= 0 {
		t.Fatal("expected intro timer to remain active")
	}
	if snap.Timers["blob_rise"] != 0 {
		t.Fatalf("blob_rise should be suppressed during intro, got %d", snap.Timers["blob_rise"])
	}
}
