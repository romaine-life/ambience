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
	want := map[string]bool{"lightning": true, "squall": true, "calm": true, "intro": true, "ending": true}
	for _, k := range schema.Knobs {
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
	if !d.TriggerEvent("lightning") {
		t.Fatal("expected lightning trigger to be accepted")
	}
	snap := d.Snapshot()
	if snap.Timers["lightning"] <= 0 {
		t.Fatalf("expected lightning timer > 0 after trigger, got %d", snap.Timers["lightning"])
	}
	if snap.Values["lightning_gain"] <= 1 {
		t.Fatalf("expected lightning_gain > 1 after trigger, got %f", snap.Values["lightning_gain"])
	}
}

func TestDistantStormSnapshotRestore(t *testing.T) {
	d := NewDistantStorm(160, 80, 91, nil)
	if !d.TriggerEvent("squall") {
		t.Fatal("expected squall trigger to succeed")
	}
	if !d.TriggerEvent("lightning") {
		t.Fatal("expected lightning trigger to succeed")
	}
	d.Step()

	snap := d.Snapshot()
	if snap.Timers["squall"] <= 0 {
		t.Fatal("expected squall timer in snapshot")
	}

	restored := NewDistantStorm(160, 80, 7, nil)
	restored.RestoreSnapshot(snap)
	again := restored.Snapshot()
	if again.Timers["squall"] != snap.Timers["squall"] {
		t.Fatalf("restored squall timer = %d, want %d", again.Timers["squall"], snap.Timers["squall"])
	}
	if again.Values["squall_gain"] != snap.Values["squall_gain"] {
		t.Fatalf("restored squall_gain = %f, want %f", again.Values["squall_gain"], snap.Values["squall_gain"])
	}
}

func TestDistantStormIntroResets(t *testing.T) {
	d := NewDistantStorm(160, 80, 5, nil)
	d.TriggerEvent("squall")
	d.TriggerEvent("intro")
	snap := d.Snapshot()
	if snap.Timers["intro"] <= 0 {
		t.Fatalf("expected intro timer > 0, got %d", snap.Timers["intro"])
	}
	if snap.Timers["squall"] != 0 {
		t.Fatalf("expected squall timer to clear on intro, got %d", snap.Timers["squall"])
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
	horizon := int(math.Floor(float64(height) * 0.58))
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
