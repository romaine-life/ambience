package sim

import (
	"reflect"
	"testing"
)

func TestConstellationsSchema(t *testing.T) {
	schema := ConstellationsSchema()
	if schema.Name != "constellations" {
		t.Fatalf("schema name = %q, want constellations", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("constellations ending should be terminal")
	}
	want := map[string]bool{
		"twinkle":       false,
		"starRate":      false,
		"drawChance":    false,
		"figureShimmer": false,
		"figureSet":     false,
	}
	for _, knob := range schema.Knobs {
		if _, ok := want[knob.Key]; ok {
			want[knob.Key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("missing schema knob %q", key)
		}
	}
}

func TestConstellationsFigureDrawPaintsLines(t *testing.T) {
	c := NewConstellations(96, 54, 42, ConstellationsConfig{DrawChance: 0, FigureSet: 1})
	if !c.TriggerEvent("figure-draw") {
		t.Fatal("figure-draw trigger rejected")
	}
	for i := 0; i < 260; i++ {
		c.Step()
	}
	snap := c.Snapshot()
	if snap.Figure == nil || snap.Figure.Progress < 1 {
		t.Fatalf("expected a fully drawn figure, got %#v", snap.Figure)
	}
	if bright := countConstellationBrightPixels(c.GridCopy()); bright < 30 {
		t.Fatalf("expected drawn constellation bright pixels, got %d", bright)
	}
}

func TestConstellationsSnapshotRestoreRoundTrip(t *testing.T) {
	src := NewConstellations(96, 54, 12, ConstellationsConfig{FigureSet: 3, DrawChance: 0})
	src.TriggerEvent("figure-draw")
	src.TriggerEvent("shimmer")
	for i := 0; i < 25; i++ {
		src.Step()
	}

	snap := src.Snapshot()
	dst := NewConstellations(96, 54, 99, nilConstellationsConfig())
	dst.RestoreSnapshot(snap)
	if got := dst.Snapshot(); !reflect.DeepEqual(got, snap) {
		t.Fatalf("snapshot mismatch after restore\ngot:  %#v\nwant: %#v", got, snap)
	}
	if !reflect.DeepEqual(src.GridCopy(), dst.GridCopy()) {
		t.Fatal("restored grid differs from source")
	}

	for i := 0; i < 20; i++ {
		src.Step()
		dst.Step()
	}
	if !reflect.DeepEqual(src.Snapshot(), dst.Snapshot()) {
		t.Fatal("post-restore snapshots diverged")
	}
	if !reflect.DeepEqual(src.GridCopy(), dst.GridCopy()) {
		t.Fatal("post-restore grids diverged")
	}
}

func TestConstellationsPersistedRoundTripAndTerminalEnding(t *testing.T) {
	src := NewConstellations(80, 48, 77, ConstellationsConfig{FigureSet: 2, DrawChance: 0})
	src.TriggerEvent("figure-draw")
	for i := 0; i < 12; i++ {
		src.Step()
	}
	persisted := src.SnapshotPersistedState()

	dst := NewConstellations(80, 48, 1, nilConstellationsConfig())
	dst.RestorePersistedState(persisted)
	if got := dst.SnapshotPersistedState(); !reflect.DeepEqual(got, persisted) {
		t.Fatalf("persisted mismatch after restore\ngot:  %#v\nwant: %#v", got, persisted)
	}

	if !dst.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := dst.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("after ending lifecycle = %q, want ending", got)
	}
	for i := 0; i < constellationsEndingTicks; i++ {
		dst.Step()
	}
	if got := dst.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("after ending lifecycle = %q, want ended", got)
	}
	for i := 0; i < 30; i++ {
		dst.Step()
	}
	if got := dst.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold: got %q", got)
	}
}

func countConstellationBrightPixels(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			if int(p.C.R)+int(p.C.G)+int(p.C.B) > 410 {
				count++
			}
		}
	}
	return count
}

func nilConstellationsConfig() ConstellationsConfig {
	return ConstellationsConfig{}
}
