package sim

import (
	"reflect"
	"testing"
)

func TestPaperLanternsSchema(t *testing.T) {
	schema := PaperLanternsSchema()
	if schema.Name != "paper-lanterns" {
		t.Fatalf("schema name = %q, want paper-lanterns", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("paper-lanterns ending should hold a terminal dark sky")
	}
	want := map[string]bool{
		"intro":         false,
		"ending":        false,
		"lantern-emit":  false,
		"release-pulse": false,
		"wind-drift":    false,
		"lantern-fade":  false,
		"quiet-gap":     false,
	}
	for _, knob := range schema.Knobs {
		if _, ok := want[knob.Trigger]; ok {
			want[knob.Trigger] = true
		}
	}
	for trigger, seen := range want {
		if !seen {
			t.Fatalf("schema missing trigger %q", trigger)
		}
	}
}

func TestPaperLanternsReleasePulseSnapshotReplay(t *testing.T) {
	cfg := PaperLanternsConfig{
		ReleaseMin:    5,
		ReleaseMax:    5,
		ReleaseWindow: 5,
		MaxLanterns:   20,
		LoneEvery:     1000,
		ReleaseGap:    1000,
	}
	src := NewPaperLanterns(80, 42, 123, cfg)
	if !src.TriggerEvent("release-pulse") {
		t.Fatal("release-pulse trigger rejected")
	}
	for i := 0; i < 6; i++ {
		src.Step()
	}
	snap := src.Snapshot()
	if got := len(snap.Lanterns); got < 5 {
		t.Fatalf("active lanterns after release = %d, want at least 5", got)
	}
	if filled := countFilled(src.GridCopy()); filled < 15 {
		t.Fatalf("filled pixels after release = %d, want visible lantern cluster", filled)
	}

	dst := NewPaperLanterns(80, 42, 999, PaperLanternsConfig{})
	dst.RestoreSnapshot(snap)
	for i := 0; i < 30; i++ {
		src.Step()
		dst.Step()
	}
	if src.CurrentTick() != dst.CurrentTick() {
		t.Fatalf("restored tick = %d, want %d", dst.CurrentTick(), src.CurrentTick())
	}
	if !reflect.DeepEqual(src.GridCopy(), dst.GridCopy()) {
		t.Fatal("restored sim diverged from source frame")
	}
}

func TestPaperLanternsEndingHoldsDark(t *testing.T) {
	p := NewPaperLanterns(64, 36, 55, PaperLanternsConfig{
		ReleaseMin:    6,
		ReleaseMax:    6,
		ReleaseWindow: 1,
		EndingStop:    0,
		EndingTail:    25,
	})
	if !p.TriggerEvent("release-pulse") {
		t.Fatal("release-pulse trigger rejected")
	}
	for i := 0; i < 3; i++ {
		p.Step()
	}
	if countFilled(p.GridCopy()) == 0 {
		t.Fatal("expected lanterns before ending")
	}
	if !p.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("lifecycle after ending trigger = %q, want ending", got)
	}
	for i := 0; i < 35; i++ {
		p.Step()
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle after ending tail = %q, want ended", got)
	}
	if filled := countFilled(p.GridCopy()); filled != 0 {
		t.Fatalf("terminal dark sky filled pixels = %d, want 0", filled)
	}
	for i := 0; i < 20; i++ {
		p.Step()
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold: %q", got)
	}
	if filled := countFilled(p.GridCopy()); filled != 0 {
		t.Fatalf("terminal dark sky did not hold, filled pixels = %d", filled)
	}
}
