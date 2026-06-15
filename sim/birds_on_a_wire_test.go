package sim

import (
	"reflect"
	"testing"
)

func TestBirdsOnAWireSchemaContainsIssueEvents(t *testing.T) {
	schema := BirdsOnAWireSchema()
	if schema.Name != "birds-on-a-wire" {
		t.Fatalf("schema name = %q, want birds-on-a-wire", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("birds-on-a-wire ending should hold an empty terminal wire")
	}
	want := map[string]bool{
		"intro":          true,
		"ending":         true,
		"bird-land":      true,
		"bird-bob":       true,
		"flock-takeoff":  true,
		"single-takeoff": true,
		"quiet-wire":     true,
	}
	for _, knob := range schema.Knobs {
		delete(want, knob.Trigger)
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestBirdsOnAWireLandBobAndSingleTakeoff(t *testing.T) {
	e := NewBirdsOnAWire(96, 54, 12, BirdsOnAWireConfig{
		ArrivalEvery: 900,
		TakeoffEvery: 900,
		MaxBirds:     6,
	})
	if !e.TriggerEvent("bird-land") {
		t.Fatal("bird-land trigger rejected")
	}
	stepUntilBirdState(t, e, BirdsOnAWireStatePerched, 220)
	if got := countBirdsInState(e.Snapshot(), BirdsOnAWireStatePerched); got == 0 {
		t.Fatal("expected at least one perched bird after landing")
	}
	if !e.TriggerEvent("bird-bob") {
		t.Fatal("bird-bob trigger rejected")
	}
	if !snapshotHasBob(e.Snapshot()) {
		t.Fatal("expected a perched bird to carry bob ticks after bird-bob")
	}
	if !e.TriggerEvent("single-takeoff") {
		t.Fatal("single-takeoff trigger rejected")
	}
	if got := countBirdsInState(e.Snapshot(), BirdsOnAWireStateDeparting); got == 0 {
		t.Fatal("expected one bird to be departing after single-takeoff")
	}
}

func TestBirdsOnAWireFlockAndQuietWire(t *testing.T) {
	e := NewBirdsOnAWire(120, 64, 44, BirdsOnAWireConfig{
		MaxBirds:     8,
		ArrivalEvery: 900,
		TakeoffEvery: 900,
		PairChance:   -1,
	})
	for i := 0; i < 5; i++ {
		if !e.TriggerEvent("bird-land") {
			t.Fatalf("bird-land trigger %d rejected", i+1)
		}
	}
	stepUntilPerchedCount(t, e, 5, 260)
	if !e.TriggerEvent("flock-takeoff") {
		t.Fatal("flock-takeoff trigger rejected")
	}
	if got := countBirdsInState(e.Snapshot(), BirdsOnAWireStateDeparting); got < 2 {
		t.Fatalf("departing birds after flock-takeoff = %d, want at least 2", got)
	}
	stepUntilPerchedCount(t, e, 2, 260)
	if !e.TriggerEvent("quiet-wire") {
		t.Fatal("quiet-wire trigger rejected")
	}
	snap := e.Snapshot()
	if snap.QuietTicks == 0 {
		t.Fatal("quiet-wire did not start a suppression window")
	}
	if snap.ArrivalTicks <= e.EffectiveConfig().ArrivalEvery {
		t.Fatalf("quiet-wire arrival timer = %d, want longer than base %d", snap.ArrivalTicks, e.EffectiveConfig().ArrivalEvery)
	}
}

func TestBirdsOnAWireSnapshotRoundTrip(t *testing.T) {
	cfg := BirdsOnAWireConfig{ArrivalEvery: 900, TakeoffEvery: 900, PairChance: -1, WireCount: 2}
	a := NewBirdsOnAWire(96, 54, 99, cfg)
	for i := 0; i < 3; i++ {
		if !a.TriggerEvent("bird-land") {
			t.Fatalf("bird-land trigger %d rejected", i+1)
		}
	}
	for i := 0; i < 90; i++ {
		a.Step()
	}
	a.TriggerEvent("quiet-wire")
	for i := 0; i < 12; i++ {
		a.Step()
	}

	b := NewBirdsOnAWire(96, 54, 1, cfg)
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch immediately after restore")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
	for i := 0; i < 30; i++ {
		a.Step()
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch after stepping restored sims")
	}
}

func TestBirdsOnAWireEndingHoldsEmptyWire(t *testing.T) {
	e := NewBirdsOnAWire(96, 54, 77, BirdsOnAWireConfig{
		EndingDur:         24,
		OutroTakeoffEvery: 4,
		ResidualLife:      8,
		ArrivalEvery:      900,
		TakeoffEvery:      900,
		PairChance:        -1,
	})
	for i := 0; i < 4; i++ {
		e.TriggerEvent("bird-land")
	}
	stepUntilPerchedCount(t, e, 4, 260)
	if !e.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnding {
		t.Fatalf("lifecycle after ending trigger = %q, want ending", got)
	}
	for i := 0; i < 80; i++ {
		e.Step()
	}
	snap := e.Snapshot()
	if got := snap.Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	if len(snap.Birds) != 0 {
		t.Fatalf("terminal wire has %d birds, want none", len(snap.Birds))
	}
	for i := 0; i < 20; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if len(e.Snapshot().Birds) != 0 {
		t.Fatal("birds reappeared while terminal ending was holding")
	}
	if !e.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}

func stepUntilBirdState(t *testing.T, e *BirdsOnAWire, state string, maxTicks int) {
	t.Helper()
	for i := 0; i < maxTicks; i++ {
		if countBirdsInState(e.Snapshot(), state) > 0 {
			return
		}
		e.Step()
	}
	t.Fatalf("no bird reached state %q within %d ticks", state, maxTicks)
}

func stepUntilPerchedCount(t *testing.T, e *BirdsOnAWire, want, maxTicks int) {
	t.Helper()
	for i := 0; i < maxTicks; i++ {
		if countBirdsInState(e.Snapshot(), BirdsOnAWireStatePerched) >= want {
			return
		}
		e.Step()
	}
	t.Fatalf("perched bird count never reached %d within %d ticks; snapshot=%+v", want, maxTicks, e.Snapshot())
}

func countBirdsInState(snap BirdsOnAWireSnapshot, state string) int {
	count := 0
	for _, b := range snap.Birds {
		if b.State == state {
			count++
		}
	}
	return count
}

func snapshotHasBob(snap BirdsOnAWireSnapshot) bool {
	for _, b := range snap.Birds {
		if b.State == BirdsOnAWireStatePerched && b.BobTicks > 0 {
			return true
		}
	}
	return false
}
