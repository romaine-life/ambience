package sim

import "testing"

func TestMagicPortalSchemaContainsContractTriggers(t *testing.T) {
	schema := MagicPortalSchema()
	if schema.Name != "magic-portal" {
		t.Fatalf("schema name = %q, want magic-portal", schema.Name)
	}
	want := map[string]bool{
		"intro":       true,
		"pulse":       true,
		"power-surge": true,
		"ember-burst": true,
		"rune-shift":  true,
		"quiet-gate":  true,
		"ending":      true,
	}
	for _, knob := range schema.Knobs {
		if knob.Trigger != "" {
			delete(want, knob.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestMagicPortalEndingHoldsDormantGate(t *testing.T) {
	cfg := MagicPortalConfig{
		EndingSurge: 4,
		EndingDecay: 6,
		EmberTail:   8,
		BurstCount:  4,
		EmberLife:   8,
	}
	portal := NewMagicPortal(96, 64, 7, cfg)
	for i := 0; i < 4; i++ {
		portal.Step()
	}
	activeBright := countMagicPortalBright(portal.GridCopy())
	if activeBright < 80 {
		t.Fatalf("active bright pixels = %d, want readable portal", activeBright)
	}
	if !portal.TriggerEvent("ending") {
		t.Fatalf("ending trigger was not accepted")
	}
	for i := 0; i < 40; i++ {
		portal.Step()
	}
	snap := portal.Snapshot()
	if snap.LifeState != magicPortalDormant {
		t.Fatalf("life state after ending = %d, want dormant", snap.LifeState)
	}
	dormantBright := countMagicPortalBright(portal.GridCopy())
	if dormantBright > activeBright/8 {
		t.Fatalf("dormant bright pixels = %d, want <= %d from active %d", dormantBright, activeBright/8, activeBright)
	}
	for i := 0; i < 20; i++ {
		portal.Step()
	}
	if got := countMagicPortalBright(portal.GridCopy()); got != dormantBright {
		t.Fatalf("dormant frame changed bright count from %d to %d", dormantBright, got)
	}
}

func TestMagicPortalIntroRestoresReadableGateAfterEnding(t *testing.T) {
	cfg := MagicPortalConfig{
		IntroResolve: 4,
		IntroPulse:   6,
		EndingSurge:  3,
		EndingDecay:  4,
		EmberTail:    4,
		BurstCount:   3,
		EmberLife:    6,
	}
	portal := NewMagicPortal(96, 64, 11, cfg)
	portal.TriggerEvent("ending")
	for i := 0; i < 24; i++ {
		portal.Step()
	}
	if countMagicPortalBright(portal.GridCopy()) > 8 {
		t.Fatalf("gate should be dark before intro")
	}
	if !portal.TriggerEvent("intro") {
		t.Fatalf("intro trigger was not accepted")
	}
	for i := 0; i < 20; i++ {
		portal.Step()
	}
	if got := countMagicPortalBright(portal.GridCopy()); got < 80 {
		t.Fatalf("bright pixels after intro = %d, want readable portal", got)
	}
}

func TestMagicPortalSnapshotRoundTripPreservesEmbersAndEvents(t *testing.T) {
	cfg := MagicPortalConfig{
		BurstCount:      8,
		EmberLife:       40,
		SurgeDur:        24,
		RuneShiftDur:    18,
		EmberPeakChance: 0,
		SurgeChance:     0,
		BurstChance:     0,
		RuneShiftChance: 0,
		QuietChance:     0,
	}
	a := NewMagicPortal(80, 48, 123, cfg)
	b := NewMagicPortal(80, 48, 999, cfg)
	a.TriggerEvent("power-surge")
	a.TriggerEvent("ember-burst")
	a.TriggerEvent("rune-shift")
	for i := 0; i < 6; i++ {
		a.Step()
	}
	b.RestoreSnapshot(a.Snapshot())
	if countMagicPortalBright(a.GridCopy()) != countMagicPortalBright(b.GridCopy()) {
		t.Fatalf("restored portal bright count mismatch")
	}
	for i := 0; i < 12; i++ {
		a.Step()
		b.Step()
	}
	if !equalPixelGrid(a.GridCopy(), b.GridCopy()) {
		t.Fatalf("restored portal diverged after stepping")
	}
}

func countMagicPortalBright(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if !p.Filled {
				continue
			}
			if int(p.C.R)+int(p.C.G)+int(p.C.B) >= 90 {
				count++
			}
		}
	}
	return count
}

func equalPixelGrid(a, b [][]Pixel) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}
