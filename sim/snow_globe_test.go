package sim

import (
	"reflect"
	"testing"
)

func TestSnowGlobeSchemaContainsLifecycleAndShakeTriggers(t *testing.T) {
	schema := SnowGlobeSchema()
	if schema.Name != "snow-globe" {
		t.Fatalf("schema name = %q, want snow-globe", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("snow-globe ending should hold a terminal settled state")
	}
	want := map[string]bool{"intro": true, "ending": true, "shake": true}
	for _, knob := range schema.Knobs {
		delete(want, knob.Trigger)
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestSnowGlobeFrameHasContentAndSceneKnobChangesFrame(t *testing.T) {
	cabin := NewSnowGlobe(64, 40, 11, SnowGlobeConfig{Scene: 0})
	if filled := countSnowGlobeFilled(cabin.Frame()); filled < 400 {
		t.Fatalf("default frame has %d filled pixels, want a visible globe", filled)
	}
	lighthouse := NewSnowGlobe(64, 40, 11, SnowGlobeConfig{Scene: 3})
	if reflect.DeepEqual(cabin.Frame(), lighthouse.Frame()) {
		t.Fatal("scene knob did not change the rendered inner scene")
	}
}

func TestSnowGlobeSnapshotRestoreRoundTrip(t *testing.T) {
	a := NewSnowGlobe(64, 40, 42, SnowGlobeConfig{
		SettleRate:      1.2,
		SnowVolume:      0.9,
		ShakeCadence:    35,
		SwirlTurbulence: 1.4,
		Scene:           2,
	})
	if !a.TriggerEvent("shake") {
		t.Fatal("shake trigger rejected")
	}
	for i := 0; i < 12; i++ {
		a.Step()
	}

	b := NewSnowGlobe(64, 40, 7, SnowGlobeConfig{
		SettleRate:      1.2,
		SnowVolume:      0.9,
		ShakeCadence:    35,
		SwirlTurbulence: 1.4,
		Scene:           2,
	})
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch immediately after restore")
	}
	if !reflect.DeepEqual(a.Frame(), b.Frame()) {
		t.Fatal("frame mismatch immediately after restore")
	}
	for i := 0; i < 30; i++ {
		a.Step()
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch after replayed steps")
	}
	if !reflect.DeepEqual(a.Frame(), b.Frame()) {
		t.Fatal("frame mismatch after replayed steps")
	}
}

func TestSnowGlobePersistedRoundTripAndTerminalEnding(t *testing.T) {
	a := NewSnowGlobe(64, 40, 88, SnowGlobeConfig{EndingDur: 120})
	a.TriggerEvent("shake")
	for i := 0; i < 20; i++ {
		a.Step()
	}
	persisted := a.SnapshotPersistedState()

	b := NewSnowGlobe(64, 40, 1, SnowGlobeConfig{EndingDur: 120})
	b.RestorePersistedState(persisted)
	if !reflect.DeepEqual(persisted, b.SnapshotPersistedState()) {
		t.Fatal("persisted state mismatch after restore")
	}
	if !b.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := b.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("lifecycle = %q, want ending", got)
	}
	for i := 0; i < 700 && b.Snapshot().Lifecycle != LifecycleEnded; i++ {
		b.Step()
	}
	if got := b.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	for i := 0; i < 20; i++ {
		b.Step()
	}
	if got := b.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if filled := countSnowGlobeFilled(b.Frame()); filled == 0 {
		t.Fatal("terminal settled globe frame is empty")
	}
	if !b.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := b.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func TestSnowGlobeDevEventsAndAutonomousShake(t *testing.T) {
	g := NewSnowGlobe(48, 32, 99, SnowGlobeConfig{ShakeCadence: 5})
	for _, event := range []string{"shake", "settle", "still", "swirl"} {
		if !g.TriggerEvent(event) {
			t.Fatalf("%s trigger rejected", event)
		}
	}
	if g.TriggerEvent("nope") {
		t.Fatal("unknown trigger accepted")
	}

	g = NewSnowGlobe(48, 32, 99, SnowGlobeConfig{ShakeCadence: 5})
	for i := 0; i < 700; i++ {
		if g.Snapshot().Timers["swirl"] > 0 {
			return
		}
		g.Step()
	}
	t.Fatal("autonomous shake did not start within the expected cadence window")
}

func countSnowGlobeFilled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled {
				count++
			}
		}
	}
	return count
}
