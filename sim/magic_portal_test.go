package sim

import (
	"math"
	"reflect"
	"testing"
)

func TestMagicPortalSchemaContainsTriggers(t *testing.T) {
	schema := MagicPortalSchema()
	if schema.Name != "magic-portal" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{
		MagicPortalEventPulse:      true,
		MagicPortalEventPowerSurge: true,
		MagicPortalEventEmberBurst: true,
		MagicPortalEventRuneShift:  true,
		MagicPortalEventQuietGate:  true,
		MagicPortalEventIntro:      true,
		MagicPortalEventEnding:     true,
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

func TestMagicPortalEndingTerminalGateDark(t *testing.T) {
	p := NewMagicPortal(96, 54, 3, MagicPortalConfig{
		EndingDur:       24,
		FinalSurgeDur:   10,
		EndingEmberTail: 12,
		EmberLife:       12,
		BurstEmbers:     6,
	})
	for i := 0; i < 8; i++ {
		p.Step()
	}
	before := magicPortalFrameBrightness(p.GridCopy())
	if before < 40000 {
		t.Fatalf("expected readable lit portal before ending, brightness=%d", before)
	}
	if !p.TriggerEvent(MagicPortalEventEnding) {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 80; i++ {
		p.Step()
	}
	snap := p.Snapshot()
	if !snap.GateDark {
		t.Fatalf("expected gateDark=true after outro, got %+v", snap.MagicPortalState)
	}
	after := magicPortalFrameBrightness(p.GridCopy())
	if after >= before/4 {
		t.Fatalf("terminal dark frame too bright: after=%d before=%d", after, before)
	}
	for i := 0; i < 24; i++ {
		p.Step()
	}
	if !p.Snapshot().GateDark {
		t.Fatal("gateDark did not hold after terminal ending state")
	}
	held := magicPortalFrameBrightness(p.GridCopy())
	if held != after {
		t.Fatalf("terminal dark frame changed after hold: held=%d after=%d", held, after)
	}
}

func TestMagicPortalPowerSurgeTransientReturnsToBaseline(t *testing.T) {
	p := NewMagicPortal(96, 54, 5, MagicPortalConfig{
		PulsePeriod: 1000,
		SurgeDur:    24,
		SurgeMult:   2.4,
		BurstEmbers: 5,
		EmberLife:   16,
	})
	for i := 0; i < 60; i++ {
		p.Step()
	}
	baseline := magicPortalFrameBrightness(p.GridCopy())
	if !p.TriggerEvent(MagicPortalEventPowerSurge) {
		t.Fatal("power-surge trigger rejected")
	}
	for i := 0; i < 12; i++ {
		p.Step()
	}
	surge := magicPortalFrameBrightness(p.GridCopy())
	if surge <= baseline {
		t.Fatalf("expected surge to brighten portal: baseline=%d surge=%d", baseline, surge)
	}
	for i := 0; i < 90; i++ {
		p.Step()
	}
	settled := magicPortalFrameBrightness(p.GridCopy())
	if math.Abs(float64(settled-baseline)) > float64(baseline)*0.12 {
		t.Fatalf("surge did not return near baseline: baseline=%d settled=%d", baseline, settled)
	}
	if p.Snapshot().GateDark {
		t.Fatal("transient surge should not set terminal gateDark")
	}
}

func TestMagicPortalSnapshotRoundTrip(t *testing.T) {
	p := NewMagicPortal(90, 50, 11, MagicPortalConfig{
		PulsePeriod: 80,
		BurstEmbers: 9,
		QuietDur:    40,
	})
	for i := 0; i < 12; i++ {
		p.Step()
	}
	p.TriggerEvent(MagicPortalEventPulse)
	p.TriggerEvent(MagicPortalEventRuneShift)
	p.TriggerEvent(MagicPortalEventQuietGate)
	for i := 0; i < 6; i++ {
		p.Step()
	}
	snap := p.Snapshot()

	restored := NewMagicPortal(90, 50, 99, MagicPortalConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	if !reflect.DeepEqual(p.Snapshot(), restored.Snapshot()) {
		t.Fatalf("snapshot mismatch after restore\nauthority=%+v\nrestored=%+v", p.Snapshot(), restored.Snapshot())
	}
	if !reflect.DeepEqual(p.GridCopy(), restored.GridCopy()) {
		t.Fatal("grid mismatch after restore")
	}
	for i := 0; i < 12; i++ {
		p.Step()
		restored.Step()
	}
	if !reflect.DeepEqual(p.Snapshot(), restored.Snapshot()) {
		t.Fatal("snapshot mismatch after post-restore stepping")
	}
	if !reflect.DeepEqual(p.GridCopy(), restored.GridCopy()) {
		t.Fatal("grid mismatch after post-restore stepping")
	}
}

func magicPortalFrameBrightness(grid [][]Pixel) int {
	total := 0
	for y := range grid {
		for x := range grid[y] {
			c := grid[y][x].C
			total += int(c.R) + int(c.G) + int(c.B)
		}
	}
	return total
}
