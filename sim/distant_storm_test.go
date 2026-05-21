package sim

import (
	"math"
	"testing"
)

func TestDistantStormSchema(t *testing.T) {
	schema := DistantStormSchema()
	if schema.Name != "distant-storm" {
		t.Fatalf("schema name = %q, want distant-storm", schema.Name)
	}
	if len(schema.Knobs) == 0 {
		t.Fatal("expected distant-storm schema knobs")
	}
	want := map[string]bool{
		"lightning-flash": true,
		"afterglow":       true,
		"cloud-drift":     true,
		"distant-thunder": true,
		"quiet-horizon":   true,
		"intro":           true,
		"ending":          true,
	}
	for _, k := range schema.Knobs {
		if k.Key == "rain_shaft" || k.Key == "squall_p" || k.Key == "calm_p" {
			t.Fatalf("schema exposes unrelated/noisy storm knob %q", k.Key)
		}
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestDistantStormLightningTrigger(t *testing.T) {
	d := NewDistantStorm(160, 80, 13, nil)
	if !d.TriggerEvent("lightning-flash") {
		t.Fatal("expected lightning-flash trigger to be accepted")
	}
	snap := d.Snapshot()
	if snap.Timers["lightning"] <= 0 {
		t.Fatalf("expected lightning timer > 0 after trigger, got %d", snap.Timers["lightning"])
	}
	if snap.Timers["afterglow"] <= 0 {
		t.Fatalf("expected afterglow timer > 0 after lightning-flash, got %d", snap.Timers["afterglow"])
	}
	if snap.Values["lightning_gain"] <= 1 {
		t.Fatalf("expected lightning_gain > 1 after trigger, got %f", snap.Values["lightning_gain"])
	}
}

func TestDistantStormIssueEvents(t *testing.T) {
	d := NewDistantStorm(160, 80, 21, nil)
	for _, event := range []string{"afterglow", "cloud-drift", "distant-thunder", "quiet-horizon"} {
		if !d.TriggerEvent(event) {
			t.Fatalf("expected %s trigger to be accepted", event)
		}
	}
	snap := d.Snapshot()
	if snap.Timers["afterglow"] <= 0 {
		t.Fatal("expected afterglow timer")
	}
	if snap.Timers["cloud-drift"] <= 0 {
		t.Fatal("expected cloud-drift timer")
	}
	if snap.Timers["distant-thunder"] <= 0 {
		t.Fatal("expected distant-thunder timer")
	}
	if snap.Timers["quiet-horizon"] <= 0 {
		t.Fatal("expected quiet-horizon timer")
	}
	if d.TriggerEvent("squall") || d.TriggerEvent("calm") || d.TriggerEvent("lightning") {
		t.Fatal("legacy event names should not be accepted")
	}
}

func TestDistantStormQuietHorizonSuppressesRandomLightning(t *testing.T) {
	d := NewDistantStorm(160, 80, 34, DistantStormConfig{"lightning_p": 1, "quiet_horizon_dur": 20})
	if !d.TriggerEvent("quiet-horizon") {
		t.Fatal("expected quiet-horizon trigger")
	}
	for i := 0; i < 5; i++ {
		d.Step()
	}
	snap := d.Snapshot()
	if snap.Timers["lightning"] != 0 {
		t.Fatalf("expected quiet-horizon to suppress random lightning, got timer %d", snap.Timers["lightning"])
	}
}

func TestDistantStormSnapshotRestore(t *testing.T) {
	d := NewDistantStorm(160, 80, 91, nil)
	if !d.TriggerEvent("cloud-drift") {
		t.Fatal("expected cloud-drift trigger to succeed")
	}
	if !d.TriggerEvent("lightning-flash") {
		t.Fatal("expected lightning-flash trigger to succeed")
	}
	d.Step()

	snap := d.Snapshot()
	if snap.Timers["cloud-drift"] <= 0 {
		t.Fatal("expected cloud-drift timer in snapshot")
	}

	restored := NewDistantStorm(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["cloud-drift"] != snap.Timers["cloud-drift"] {
		t.Fatalf("restored cloud-drift timer = %d, want %d", again.Timers["cloud-drift"], snap.Timers["cloud-drift"])
	}
	if again.Values["cloud_drift_bias"] != snap.Values["cloud_drift_bias"] {
		t.Fatalf("restored cloud_drift_bias = %f, want %f", again.Values["cloud_drift_bias"], snap.Values["cloud_drift_bias"])
	}
}

func TestDistantStormIntroResets(t *testing.T) {
	d := NewDistantStorm(160, 80, 5, nil)
	d.TriggerEvent("cloud-drift")
	d.TriggerEvent("distant-thunder")
	d.TriggerEvent("quiet-horizon")
	d.TriggerEvent("intro")
	snap := d.Snapshot()
	if snap.Timers["intro"] <= 0 {
		t.Fatalf("expected intro timer > 0, got %d", snap.Timers["intro"])
	}
	for _, key := range []string{"cloud-drift", "distant-thunder", "quiet-horizon"} {
		if snap.Timers[key] != 0 {
			t.Fatalf("expected %s timer to clear on intro, got %d", key, snap.Timers[key])
		}
	}
}

func TestDistantStormGridCopyPaintsAcrossHorizon(t *testing.T) {
	const height = 20
	const width = 40
	d := NewDistantStorm(width, height, 1, nil)
	grid := d.GridCopy()
	if len(grid) != height {
		t.Fatalf("expected %d rows, got %d", height, len(grid))
	}
	if len(grid[0]) != width {
		t.Fatalf("expected %d cols, got %d", width, len(grid[0]))
	}
	horizon := int(math.Floor(float64(height) * 0.72))
	above, below := 0, 0
	for y, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			if y < horizon {
				above++
			} else {
				below++
			}
		}
	}
	if above == 0 {
		t.Fatal("expected painted pixels above horizon (sky/cloud)")
	}
	if below == 0 {
		t.Fatal("expected painted pixels below horizon (sea)")
	}
}
