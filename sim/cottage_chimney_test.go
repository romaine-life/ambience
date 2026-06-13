package sim

import (
	"reflect"
	"testing"
)

func TestCottageChimneySchemaContainsEvents(t *testing.T) {
	schema := CottageChimneySchema()
	if schema.Name != "cottage-chimney" {
		t.Fatalf("schema name = %q, want cottage-chimney", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("cottage-chimney ending should hold a terminal dark cottage state")
	}
	want := map[string]bool{
		"puff-emit":    true,
		"wind-gust":    true,
		"lamp-flicker": true,
		"embers":       true,
		"quiet-night":  true,
		"intro":        true,
		"ending":       true,
	}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestCottageChimneyDefaultFrameHasWindowAndSmoke(t *testing.T) {
	c := NewCottageChimney(96, 54, 12, nil)
	for i := 0; i < 90; i++ {
		c.Step()
	}
	snap := c.Snapshot()
	if len(snap.Puffs) == 0 {
		t.Fatal("expected steady state to maintain visible smoke puffs")
	}
	grid := c.GridCopy()
	if got := countWarmWindowPixels(grid); got < 6 {
		t.Fatalf("warm window pixels = %d, want at least 6", got)
	}
	if got := countUpperSmokePixels(grid); got < 12 {
		t.Fatalf("upper smoke pixels = %d, want at least 12", got)
	}
}

func TestCottageChimneySnapshotRoundTrip(t *testing.T) {
	cfg := CottageChimneyConfig{
		"puff_every":  18,
		"puff_life":   120,
		"wind":        0.08,
		"window_hue":  42,
		"window_glow": 0.7,
	}
	a := NewCottageChimney(80, 48, 99, cfg)
	for _, event := range []string{"puff-emit", "wind-gust", "embers", "lamp-flicker"} {
		if !a.TriggerEvent(event) {
			t.Fatalf("trigger %q rejected", event)
		}
	}
	for i := 0; i < 25; i++ {
		a.Step()
	}

	b := NewCottageChimney(80, 48, 1, cfg)
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
	for i := 0; i < 30; i++ {
		a.Step()
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch after restored sims advanced")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch after restored sims advanced")
	}
}

func TestCottageChimneyEndingHoldsDark(t *testing.T) {
	c := NewCottageChimney(96, 54, 44, CottageChimneyConfig{
		"ending_dur":  8,
		"ending_tail": 8,
		"final_puffs": 1,
	})
	if !c.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := c.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("after ending trigger lifecycle = %q, want ending", got)
	}
	for i := 0; i < 30; i++ {
		c.Step()
	}
	snap := c.Snapshot()
	if got := snap.Lifecycle; got != LifecycleEnded {
		t.Fatalf("post-ending lifecycle = %q, want ended", got)
	}
	if len(snap.Puffs) != 0 {
		t.Fatalf("terminal dark state should have no smoke puffs, got %d", len(snap.Puffs))
	}
	if got := countWarmWindowPixels(c.GridCopy()); got != 0 {
		t.Fatalf("terminal dark state has %d warm window pixels, want 0", got)
	}
	for i := 0; i < 20; i++ {
		c.Step()
	}
	if got := c.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal state did not hold, lifecycle = %q", got)
	}
	if !c.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from ended state")
	}
	if got := c.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func countWarmWindowPixels(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			if p.C.R > 115 && p.C.G > 75 && p.C.B < 95 {
				count++
			}
		}
	}
	return count
}

func countUpperSmokePixels(grid [][]Pixel) int {
	if len(grid) == 0 {
		return 0
	}
	limit := len(grid) / 2
	count := 0
	for y := 0; y < limit; y++ {
		for _, p := range grid[y] {
			if !p.Filled {
				continue
			}
			if p.C.B >= p.C.R && p.C.B >= p.C.G && p.C.R > 24 && p.C.G > 28 {
				count++
			}
		}
	}
	return count
}
