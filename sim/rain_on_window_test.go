package sim

import (
	"reflect"
	"testing"
)

func TestRainOnWindowSchemaContainsIssueEvents(t *testing.T) {
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

func TestRainOnWindowEventsFormMergeAndFall(t *testing.T) {
	r := NewRainOnWindow(64, 40, 12, RainOnWindowConfig{
		Nucleation:   0.01,
		GrowRate:     0.08,
		CriticalMass: 1.0,
		MergeFactor:  1.6,
		FallSpeed:    0.7,
	})
	if !r.TriggerEvent("drop-form") {
		t.Fatal("drop-form trigger rejected")
	}
	formed := len(r.Snapshot().Drops)
	if formed < 2 {
		t.Fatalf("drop-form produced %d drops, want at least 2", formed)
	}
	if !r.TriggerEvent("drop-merge") {
		t.Fatal("drop-merge trigger rejected")
	}
	merged := len(r.Snapshot().Drops)
	if merged >= formed {
		t.Fatalf("drop-merge left %d drops, want fewer than %d", merged, formed)
	}
	if !r.TriggerEvent("drop-fall") {
		t.Fatal("drop-fall trigger rejected")
	}
	for i := 0; i < 6; i++ {
		r.Step()
	}
	snap := r.Snapshot()
	if !rainWindowAnyFalling(snap.Drops) {
		t.Fatalf("expected a falling drop after drop-fall, got %+v", snap.Drops)
	}
	if len(snap.Tracks) == 0 {
		t.Fatal("expected falling drop to leave a wet track")
	}
}

func TestRainOnWindowDropletsNaturallyGrowAndFall(t *testing.T) {
	r := NewRainOnWindow(64, 40, 99, RainOnWindowConfig{
		Nucleation:   0.75,
		GrowRate:     0.16,
		CriticalMass: 0.7,
		MergeFactor:  1.35,
		FallSpeed:    0.62,
		MaxDrops:     80,
	})
	for i := 0; i < 40; i++ {
		r.Step()
	}
	snap := r.Snapshot()
	if len(snap.Drops) == 0 {
		t.Fatal("expected active droplets after high nucleation window")
	}
	if !rainWindowAnyFalling(snap.Drops) {
		t.Fatalf("expected at least one naturally falling droplet, got %+v", snap.Drops)
	}
	if len(snap.Tracks) == 0 {
		t.Fatal("expected natural falling droplets to create tracks")
	}
}

func TestRainOnWindowSnapshotRoundTrip(t *testing.T) {
	src := NewRainOnWindow(72, 44, 123, RainOnWindowConfig{
		Nucleation:   0.4,
		GrowRate:     0.06,
		CriticalMass: 1.1,
		GustDur:      80,
		QuietDur:     90,
	})
	for _, event := range []string{"drop-form", "wind-gust", "quiet-pane", "drop-fall"} {
		if !src.TriggerEvent(event) {
			t.Fatalf("%s trigger rejected", event)
		}
	}
	for i := 0; i < 14; i++ {
		src.Step()
	}

	dst := NewRainOnWindow(72, 44, 1, RainOnWindowConfig{
		Nucleation:   0.4,
		GrowRate:     0.06,
		CriticalMass: 1.1,
		GustDur:      80,
		QuietDur:     90,
	})
	dst.RestoreSnapshot(src.Snapshot())
	if !reflect.DeepEqual(src.Snapshot(), dst.Snapshot()) {
		t.Fatal("snapshot mismatch immediately after restore")
	}
	if !reflect.DeepEqual(src.GridCopy(), dst.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
	for i := 0; i < 12; i++ {
		src.Step()
		dst.Step()
	}
	if !reflect.DeepEqual(src.Snapshot(), dst.Snapshot()) {
		t.Fatal("snapshot mismatch after synchronized steps")
	}
	if !reflect.DeepEqual(src.GridCopy(), dst.GridCopy()) {
		t.Fatal("grid mismatch after synchronized steps")
	}
}

func TestRainOnWindowEndingHoldsTerminalDimPane(t *testing.T) {
	r := NewRainOnWindow(64, 40, 44, RainOnWindowConfig{EndingDur: 8, TrackLife: 40})
	baseline := rainWindowBrightness(r.GridCopy())
	r.TriggerEvent("drop-form")
	r.TriggerEvent("drop-fall")
	if !r.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 16; i++ {
		r.Step()
	}
	snap := r.Snapshot()
	if snap.Lifecycle != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", snap.Lifecycle)
	}
	if len(snap.Drops) != 0 || len(snap.Tracks) != 0 {
		t.Fatalf("terminal pane retained drops=%d tracks=%d", len(snap.Drops), len(snap.Tracks))
	}
	terminal := rainWindowBrightness(r.GridCopy())
	if terminal >= baseline*0.35 {
		t.Fatalf("terminal brightness %.2f should be well below baseline %.2f", terminal, baseline)
	}
	for i := 0; i < 12; i++ {
		r.Step()
	}
	if got := r.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if !r.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := r.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func rainWindowAnyFalling(drops []RainOnWindowDrop) bool {
	for _, d := range drops {
		if d.Falling {
			return true
		}
	}
	return false
}
