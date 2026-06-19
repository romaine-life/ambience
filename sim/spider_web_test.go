package sim

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSpiderWebSchemaContainsRequiredKnobsAndLifecycle(t *testing.T) {
	schema := SpiderWebSchema()
	if schema.Name != "spider-web" {
		t.Fatalf("schema name = %q, want spider-web", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("spider-web ending should hold a terminal state")
	}

	wantKnobs := map[string]bool{
		"dropletShimmer": true,
		"glintRate":      true,
		"moveChance":     true,
		"webSway":        true,
		"palette":        true,
		"introDur":       true,
		"endingDur":      true,
	}
	wantTriggers := map[string]bool{
		"intro":      true,
		"ending":     true,
		"glint":      true,
		"reposition": true,
		"web-sway":   true,
	}
	for _, knob := range schema.Knobs {
		delete(wantKnobs, knob.Key)
		delete(wantTriggers, knob.Trigger)
	}
	if len(wantKnobs) != 0 {
		t.Fatalf("schema missing knobs: %v", wantKnobs)
	}
	if len(wantTriggers) != 0 {
		t.Fatalf("schema missing triggers: %v", wantTriggers)
	}
}

func TestSpiderWebDefaultFrameReadsAsDewyWeb(t *testing.T) {
	e := NewSpiderWeb(80, 48, 12, SpiderWebConfig{})
	grid := e.GridCopy()
	if len(grid) != 48 || len(grid[0]) != 80 {
		t.Fatalf("grid size = %dx%d, want 80x48", len(grid[0]), len(grid))
	}
	if bright := countSpiderWebBrightPixels(grid); bright < 45 {
		t.Fatalf("bright web/dew pixels = %d, want at least 45", bright)
	}
	if drops := len(e.Snapshot().Droplets); drops < 30 {
		t.Fatalf("droplets = %d, want at least 30", drops)
	}
}

func TestSpiderWebSnapshotRoundTripReplaysFrame(t *testing.T) {
	a := NewSpiderWeb(80, 48, 99, SpiderWebConfig{GlintRate: 1.2, DropletShimmer: 1.4, WebSway: 1.1})
	for _, trigger := range []string{"glint", "web-sway", "wrap-catch"} {
		if !a.TriggerEvent(trigger) {
			t.Fatalf("%s trigger rejected", trigger)
		}
	}
	for i := 0; i < 9; i++ {
		a.Step()
	}

	b := NewSpiderWeb(80, 48, 1, SpiderWebConfig{GlintRate: 1.2, DropletShimmer: 1.4, WebSway: 1.1})
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
		if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
			t.Fatalf("snapshot mismatch after replay step %d", i+1)
		}
		if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
			t.Fatalf("grid mismatch after replay step %d", i+1)
		}
	}
}

func TestSpiderWebPersistedRoundTripOmitsDerivedLifecycle(t *testing.T) {
	a := NewSpiderWeb(64, 40, 17, SpiderWebConfig{EndingDur: 12})
	if !a.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 5; i++ {
		a.Step()
	}
	state := a.SnapshotPersistedState()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("lifecycle")) {
		t.Fatalf("persisted state should not include derived lifecycle: %s", data)
	}

	b := NewSpiderWeb(64, 40, 1, SpiderWebConfig{EndingDur: 12})
	b.RestorePersistedState(state)
	if !reflect.DeepEqual(a.SnapshotPersistedState(), b.SnapshotPersistedState()) {
		t.Fatal("persisted state mismatch immediately after restore")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch after persisted restore")
	}
}

func TestSpiderWebEndingHoldsTerminalStillness(t *testing.T) {
	e := NewSpiderWeb(64, 40, 42, SpiderWebConfig{EndingDur: 30})
	if !e.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("lifecycle after ending trigger = %q, want ending", got)
	}
	for i := 0; i < 35; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	frame := e.GridCopy()
	for i := 0; i < 20; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if !reflect.DeepEqual(frame, e.GridCopy()) {
		t.Fatal("terminal stillness frame changed after ending completed")
	}
	if !e.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func TestSpiderWebWrapCatchTriggerLogs(t *testing.T) {
	e := NewSpiderWeb(64, 40, 7, SpiderWebConfig{})
	if !e.TriggerEvent("wrap-catch") {
		t.Fatal("wrap-catch trigger rejected")
	}
	snap := e.Snapshot()
	if snap.WrapTicks == 0 || snap.MoveTicks == 0 {
		t.Fatalf("wrap-catch did not start movement and wrap timers: %+v", snap)
	}
	log := e.DrainLog()
	if len(log) == 0 || log[len(log)-1].Type != "wrap-catch" {
		t.Fatalf("wrap-catch log missing: %+v", log)
	}
}

func countSpiderWebBrightPixels(grid [][]Pixel) int {
	n := 0
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			if int(p.C.R)+int(p.C.G)+int(p.C.B) >= 330 {
				n++
			}
		}
	}
	return n
}
