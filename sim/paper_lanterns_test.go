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
	if len(schema.Knobs) == 0 {
		t.Fatal("expected paper-lanterns schema knobs")
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

func TestPaperLanternsReleasePulseSpawnsCluster(t *testing.T) {
	p := NewPaperLanterns(96, 54, 11, PaperLanternsConfig{
		ClusterMin:    5,
		ClusterMax:    5,
		ReleaseWindow: 1,
		MaxLanterns:   32,
		SpawnChance:   0.000001,
		ReleaseChance: 0.000001,
		WindChance:    0.000001,
		QuietChance:   0.000001,
	})
	before := len(p.Snapshot().Lanterns)
	if !p.TriggerEvent("release-pulse") {
		t.Fatal("expected release-pulse trigger to succeed")
	}
	p.Step()
	after := len(p.Snapshot().Lanterns)
	if got := after - before; got != 5 {
		t.Fatalf("pulse spawned %d lanterns, want 5", got)
	}
	for i := 0; i < 30; i++ {
		p.Step()
	}
	if countWarmPixels(p.GridCopy()) == 0 {
		t.Fatal("expected release pulse to paint warm lantern pixels")
	}
}

func TestPaperLanternsSnapshotRestoreRoundTrip(t *testing.T) {
	p := NewPaperLanterns(96, 54, 42, PaperLanternsConfig{
		ClusterMin:    6,
		ClusterMax:    6,
		ReleaseWindow: 3,
		MaxLanterns:   48,
		SpawnChance:   0.000001,
		ReleaseChance: 0.000001,
		WindChance:    0.000001,
		QuietChance:   0.000001,
	})
	for _, event := range []string{"release-pulse", "wind-drift", "quiet-gap"} {
		if !p.TriggerEvent(event) {
			t.Fatalf("expected %s trigger to succeed", event)
		}
	}
	for i := 0; i < 5; i++ {
		p.Step()
	}

	snap := p.Snapshot()
	restored := NewPaperLanterns(96, 54, 7, PaperLanternsConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	got := restored.Snapshot()

	if !reflect.DeepEqual(got.PaperLanternsState, snap.PaperLanternsState) {
		t.Fatalf("restored state mismatch\ngot:  %#v\nwant: %#v", got.PaperLanternsState, snap.PaperLanternsState)
	}
	if !reflect.DeepEqual(got.Lanterns, snap.Lanterns) {
		t.Fatalf("restored lanterns mismatch\ngot:  %#v\nwant: %#v", got.Lanterns, snap.Lanterns)
	}
	if !reflect.DeepEqual(restored.GridCopy(), p.GridCopy()) {
		t.Fatal("restored grid mismatch")
	}
}

func TestPaperLanternsEndingClearsAndHolds(t *testing.T) {
	p := NewPaperLanterns(80, 45, 9, PaperLanternsConfig{
		EndingStop:    1,
		TailLength:    3,
		MaxLanterns:   18,
		SpawnChance:   0.000001,
		ReleaseChance: 0.000001,
		WindChance:    0.000001,
		QuietChance:   0.000001,
	})
	if len(p.Snapshot().Lanterns) == 0 {
		t.Fatal("expected initial in-flight lanterns")
	}
	if !p.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to succeed")
	}
	for i := 0; i < 8; i++ {
		p.Step()
	}
	snap := p.Snapshot()
	if !snap.Ended {
		t.Fatal("expected ending state to hold")
	}
	if len(snap.Lanterns) != 0 {
		t.Fatalf("lantern count after ending = %d, want 0", len(snap.Lanterns))
	}
	if got := countWarmPixels(p.GridCopy()); got != 0 {
		t.Fatalf("warm lantern pixels after ending = %d, want 0", got)
	}
	for i := 0; i < 4; i++ {
		p.Step()
	}
	if len(p.Snapshot().Lanterns) != 0 {
		t.Fatal("ending did not hold dark")
	}
}

func countWarmPixels(grid [][]Pixel) int {
	n := 0
	for y := range grid {
		for x := range grid[y] {
			c := grid[y][x].C
			if c.R > 70 && c.G > 35 && c.R > c.B*2 {
				n++
			}
		}
	}
	return n
}
