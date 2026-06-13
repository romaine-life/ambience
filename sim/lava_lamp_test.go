package sim

import (
	"reflect"
	"testing"
)

func TestLavaLampSchemaContainsTriggers(t *testing.T) {
	schema := LavaLampSchema()
	if schema.Name != "lava-lamp" {
		t.Fatalf("schema name = %q, want lava-lamp", schema.Name)
	}
	want := map[string]bool{
		"intro":       true,
		"ending":      true,
		"blob-rise":   true,
		"blob-merge":  true,
		"blob-split":  true,
		"surface-pop": true,
		"quiet-flow":  true,
	}
	for _, knob := range schema.Knobs {
		if knob.Trigger != "" {
			delete(want, knob.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
	if !schema.EndingTerminal {
		t.Fatal("lava-lamp ending should be terminal")
	}
}

func TestLavaLampBlobRiseAndSurfacePop(t *testing.T) {
	l := NewLavaLamp(120, 70, 1, LavaLampConfig{
		MinBlobs: 3,
		MaxBlobs: 4,
	})
	if !l.TriggerEvent("blob-rise") {
		t.Fatal("blob-rise trigger rejected")
	}
	snap := l.Snapshot()
	if countLavaBlobsInMode(snap.Blobs, LavaBlobRising) == 0 {
		t.Fatalf("blob-rise did not create a rising blob: %#v", snap.Blobs)
	}

	if !l.TriggerEvent("surface-pop") {
		t.Fatal("surface-pop trigger rejected")
	}
	snap = l.Snapshot()
	if countLavaBlobsInMode(snap.Blobs, LavaBlobSurface) == 0 {
		t.Fatalf("surface-pop did not flatten a blob: %#v", snap.Blobs)
	}
}

func TestLavaLampSnapshotRoundTrip(t *testing.T) {
	cfg := LavaLampConfig{
		MinBlobs:         3,
		MaxBlobs:         5,
		BlobRiseChance:   0,
		MergeChance:      0,
		SplitChance:      0,
		SurfacePopChance: 0,
		QuietFlowChance:  0,
	}
	a := NewLavaLamp(96, 54, 0x5151, cfg)
	a.TriggerEvent("blob-rise")
	a.TriggerEvent("quiet-flow")
	for i := 0; i < 30; i++ {
		a.Step()
	}

	b := NewLavaLamp(96, 54, 999, cfg)
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatalf("restored snapshot differs\na: %#v\nb: %#v", a.Snapshot(), b.Snapshot())
	}
	for i := 0; i < 40; i++ {
		a.Step()
		b.Step()
		if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
			t.Fatalf("snapshot mismatch after step %d", i+1)
		}
		if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
			t.Fatalf("frame mismatch after step %d", i+1)
		}
	}
}

func TestLavaLampEndingHoldsSettledBase(t *testing.T) {
	l := NewLavaLamp(120, 70, 7, LavaLampConfig{
		MinBlobs:         3,
		MaxBlobs:         3,
		EndingDur:        8,
		EndingSettle:     8,
		BlobRiseChance:   0,
		MergeChance:      0,
		SplitChance:      0,
		SurfacePopChance: 0,
		QuietFlowChance:  0,
	})
	l.TriggerEvent("blob-rise")
	for i := 0; i < 10; i++ {
		l.Step()
	}
	if !l.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 40; i++ {
		l.Step()
	}
	snap := l.Snapshot()
	if snap.Lifecycle != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", snap.Lifecycle)
	}
	for _, b := range snap.Blobs {
		if b.Mode != LavaBlobBase {
			t.Fatalf("blob %d mode = %q, want base after terminal ending", b.ID, b.Mode)
		}
	}
	for i := 0; i < 25; i++ {
		l.Step()
		if got := l.Snapshot().Lifecycle; got != LifecycleEnded {
			t.Fatalf("terminal ending did not hold after %d extra ticks: %q", i+1, got)
		}
	}
}

func countLavaBlobsInMode(blobs []LavaBlob, mode string) int {
	n := 0
	for _, b := range blobs {
		if b.Mode == mode {
			n++
		}
	}
	return n
}
