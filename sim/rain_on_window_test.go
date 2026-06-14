package sim

import (
	"reflect"
	"testing"
)

func TestRainOnWindowSchemaContainsIssueTriggers(t *testing.T) {
	schema := RainOnWindowSchema()
	if schema.Name != "rain-on-window" {
		t.Fatalf("schema name = %q, want rain-on-window", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("rain-on-window ending should hold a terminal dim pane")
	}
	want := map[string]bool{
		"drop-form":  true,
		"drop-merge": true,
		"drop-fall":  true,
		"wind-gust":  true,
		"quiet-pane": true,
		"intro":      true,
		"ending":     true,
	}
	for _, knob := range schema.Knobs {
		delete(want, knob.Trigger)
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestRainOnWindowDefaultFrameHasPaneAndDrops(t *testing.T) {
	e := NewRainOnWindow(64, 36, 1, RainOnWindowConfig{})
	if filled := countRainOnWindowFilled(e.GridCopy()); filled < 64*36 {
		t.Fatalf("default pane filled pixels = %d, want full blurred background", filled)
	}
	snap := e.Snapshot()
	if len(snap.Drops) < 8 {
		t.Fatalf("default drops = %d, want seeded droplets", len(snap.Drops))
	}
	if brightness := rainOnWindowAverageBrightness(e.GridCopy()); brightness <= 18 {
		t.Fatalf("default frame too dark: average brightness %.2f", brightness)
	}
}

func TestRainOnWindowDropsGrowThenFall(t *testing.T) {
	e := NewRainOnWindow(64, 36, 7, RainOnWindowConfig{
		NucleationRate: 0.01,
		GrowthRate:     0.08,
		FallThreshold:  1.35,
		FallSpeed:      0.8,
		DropCap:        20,
	})
	e.TriggerEvent("intro")
	for i := 0; i < 90; i++ {
		e.Step()
	}
	snap := e.Snapshot()
	if len(snap.Drops) == 0 && len(snap.Tracks) == 0 {
		t.Fatal("expected droplets or residual tracks after growth window")
	}
	falling := 0
	for _, d := range snap.Drops {
		if d.Falling {
			falling++
		}
	}
	if falling == 0 && len(snap.Tracks) == 0 {
		t.Fatalf("no drop crossed the fall threshold: drops=%d tracks=%d", len(snap.Drops), len(snap.Tracks))
	}
}

func TestRainOnWindowManualMergeCombinesTouchingDrops(t *testing.T) {
	e := NewRainOnWindow(64, 36, 11, RainOnWindowConfig{DropCap: 20})
	e.mu.Lock()
	e.drops = []RainOnWindowDrop{
		{X: 20, Y: 12, Radius: 1.2, Mass: 1.44, Growth: 0.01, Tone: 0.2, TrailTop: 12},
		{X: 21, Y: 12.2, Radius: 1.1, Mass: 1.21, Growth: 0.01, Tone: 0.4, TrailTop: 12.2},
	}
	e.mu.Unlock()
	before := len(e.Snapshot().Drops)
	if !e.TriggerEvent("drop-merge") {
		t.Fatal("drop-merge trigger rejected")
	}
	after := len(e.Snapshot().Drops)
	if after >= before {
		t.Fatalf("drop-merge did not reduce active drop count: before=%d after=%d", before, after)
	}
}

func TestRainOnWindowSnapshotRoundTrip(t *testing.T) {
	a := NewRainOnWindow(64, 36, 99, RainOnWindowConfig{
		DropFormBurst:   4,
		WindGustDur:     22,
		QuietPaneDur:    18,
		DropFallChance:  0.003,
		DropMergeChance: 0.003,
	})
	for _, event := range []string{"drop-form", "drop-merge", "drop-fall", "wind-gust", "quiet-pane"} {
		if !a.TriggerEvent(event) {
			t.Fatalf("%s trigger rejected", event)
		}
	}
	for i := 0; i < 25; i++ {
		a.Step()
	}

	b := NewRainOnWindow(64, 36, 1, RainOnWindowConfig{
		DropFormBurst:   4,
		WindGustDur:     22,
		QuietPaneDur:    18,
		DropFallChance:  0.003,
		DropMergeChance: 0.003,
	})
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch immediately after restore")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
	for i := 0; i < 20; i++ {
		a.Step()
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch after post-restore steps")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch after post-restore steps")
	}
}

func TestRainOnWindowEndingHoldsTerminalDimPane(t *testing.T) {
	e := NewRainOnWindow(64, 36, 44, RainOnWindowConfig{EndingDur: 8, EndingLinger: 4})
	before := rainOnWindowAverageBrightness(e.GridCopy())
	if !e.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 20; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	if drops := len(e.Snapshot().Drops); drops != 0 {
		t.Fatalf("terminal state has %d drops, want dry pane", drops)
	}
	after := rainOnWindowAverageBrightness(e.GridCopy())
	if after >= before*0.45 {
		t.Fatalf("terminal pane did not fade enough: before %.2f after %.2f", before, after)
	}
	for i := 0; i < 8; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if !e.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func rainOnWindowAverageBrightness(grid [][]Pixel) float64 {
	var sum float64
	var count int
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			sum += (float64(p.C.R) + float64(p.C.G) + float64(p.C.B)) / 3
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
