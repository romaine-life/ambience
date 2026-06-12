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
		t.Fatal("paper-lanterns ending should be terminal")
	}
	triggers := map[string]bool{}
	for _, knob := range schema.Knobs {
		if knob.Trigger != "" {
			triggers[knob.Trigger] = true
		}
	}
	for _, want := range []string{"lantern-emit", "release-pulse", "wind-drift", "lantern-fade", "quiet-gap", "intro", "ending"} {
		if !triggers[want] {
			t.Fatalf("schema missing trigger %q", want)
		}
	}
}

func TestPaperLanternsReleasePulseLaunchesCluster(t *testing.T) {
	p := NewPaperLanterns(80, 48, 11, PaperLanternsConfig{
		PulseMin:           6,
		PulseMax:           6,
		PulseWindow:        1,
		ReleasePulseChance: 0,
	})
	if !p.TriggerEvent("release-pulse") {
		t.Fatal("release-pulse trigger rejected")
	}
	for i := 0; i < 3; i++ {
		p.Step()
	}
	if got := len(p.LanternsCopy()); got != 6 {
		t.Fatalf("lanterns after release pulse = %d, want 6", got)
	}
	if paperLanternGridBrightness(p.GridCopy()) == 0 {
		t.Fatal("release pulse did not paint visible lantern pixels")
	}
}

func TestPaperLanternsSnapshotRestoreDeterministic(t *testing.T) {
	cfg := PaperLanternsConfig{
		EmitEvery:          1000,
		ReleasePulseChance: 0,
		WindDriftChance:    0,
		QuietGapChance:     0,
		PulseMin:           5,
		PulseMax:           5,
		PulseWindow:        4,
	}
	a := NewPaperLanterns(80, 48, 99, cfg)
	b := NewPaperLanterns(80, 48, 1, cfg)
	a.TriggerEvent("release-pulse")
	a.TriggerEvent("wind-drift")
	for i := 0; i < 8; i++ {
		a.Step()
	}
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
	for i := 0; i < 20; i++ {
		a.Step()
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch after matched post-restore steps")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch after matched post-restore steps")
	}
}

func TestPaperLanternsIntroAndTerminalEnding(t *testing.T) {
	cfg := PaperLanternsConfig{
		IntroFirstDelay:    1,
		IntroClusterDelay:  4,
		EndingTailTicks:    18,
		EmitEvery:          1000,
		ReleasePulseChance: 0,
		WindDriftChance:    0,
		QuietGapChance:     0,
		PulseMin:           5,
		PulseMax:           5,
		PulseWindow:        1,
	}
	p := NewPaperLanterns(80, 48, 7, cfg)
	if !p.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected")
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("after intro lifecycle = %q, want intro", got)
	}
	for i := 0; i < 80 && p.Snapshot().Lifecycle != LifecycleRunning; i++ {
		p.Step()
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleRunning {
		t.Fatalf("intro did not resolve to running, got %q", got)
	}
	if len(p.LanternsCopy()) == 0 {
		t.Fatal("intro should leave visible in-flight lanterns")
	}
	if !p.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := p.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("after ending lifecycle = %q, want ending", got)
	}
	for i := 0; i < 80 && p.Snapshot().Lifecycle != LifecycleEnded; i++ {
		p.Step()
	}
	snap := p.Snapshot()
	if snap.Lifecycle != LifecycleEnded {
		t.Fatalf("ending did not resolve to ended, got %q", snap.Lifecycle)
	}
	if !snap.Dark {
		t.Fatal("terminal ending should report dark=true")
	}
	if got := paperLanternGridBrightness(p.GridCopy()); got != 0 {
		t.Fatalf("terminal ending brightness = %d, want 0", got)
	}
	for i := 0; i < 24; i++ {
		p.Step()
		if got := p.Snapshot().Lifecycle; got != LifecycleEnded {
			t.Fatalf("terminal ending did not hold: lifecycle %q after %d ticks", got, i+1)
		}
		if got := paperLanternGridBrightness(p.GridCopy()); got != 0 {
			t.Fatalf("terminal ending did not hold dark: brightness %d after %d ticks", got, i+1)
		}
	}
}

func paperLanternGridBrightness(grid [][]Pixel) int {
	total := 0
	for _, row := range grid {
		for _, px := range row {
			if px.Filled {
				total += int(px.C.R) + int(px.C.G) + int(px.C.B)
			}
		}
	}
	return total
}
