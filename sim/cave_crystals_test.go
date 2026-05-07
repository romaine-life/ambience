package sim

import "testing"

func TestCaveCrystalsSchemaContainsTriggers(t *testing.T) {
	schema := CaveCrystalsSchema()
	if schema.Name != "cave-crystals" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{
		"nucleus-spawn": true,
		"growth-pulse":  true,
		"crystal-pop":   true,
		"sparkle-burst": true,
		"quiet-cave":    true,
		"intro":         true,
		"ending":        true,
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

func TestCaveCrystalsNucleusSpawnSeeds(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 1, CaveCrystalsConfig{MaxCrystals: 4, MaxGrowth: 3})
	if !cc.TriggerEvent("nucleus-spawn") {
		t.Fatalf("expected nucleus-spawn to be accepted")
	}
	snap := cc.Snapshot()
	if len(snap.Crystals) != 1 {
		t.Fatalf("expected 1 crystal after nucleus-spawn, got %d", len(snap.Crystals))
	}
	if snap.Crystals[0].Growth != 0 {
		t.Fatalf("expected new nucleus to start at growth=0, got %d", snap.Crystals[0].Growth)
	}
}

func TestCaveCrystalsGrowthPulseAdvances(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 2, CaveCrystalsConfig{MaxCrystals: 4, MaxGrowth: 3})
	cc.TriggerEvent("nucleus-spawn")
	for i := 0; i < 3; i++ {
		if !cc.TriggerEvent("growth-pulse") {
			t.Fatalf("expected growth-pulse to be accepted")
		}
	}
	snap := cc.Snapshot()
	if snap.Crystals[0].Growth != 3 {
		t.Fatalf("expected growth to reach max=3, got %d", snap.Crystals[0].Growth)
	}
	// Past max growth, nothing should advance.
	cc.TriggerEvent("growth-pulse")
	snap = cc.Snapshot()
	if snap.Crystals[0].Growth != 3 {
		t.Fatalf("growth advanced past max: %d", snap.Crystals[0].Growth)
	}
}

func TestCaveCrystalsIntroResetsField(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 3, CaveCrystalsConfig{MaxCrystals: 6, MaxGrowth: 2, IntroBurst: 2, IntroGrowth: 0.5})
	cc.TriggerEvent("nucleus-spawn")
	cc.TriggerEvent("nucleus-spawn")
	cc.TriggerEvent("growth-pulse")
	if !cc.TriggerEvent("intro") {
		t.Fatalf("expected intro to be accepted")
	}
	snap := cc.Snapshot()
	if snap.IntroTicks <= 0 {
		t.Fatalf("expected introTicks > 0 after intro, got %d", snap.IntroTicks)
	}
	if len(snap.Crystals) != 2 {
		t.Fatalf("expected intro to reset to intro_burst=2 crystals, got %d", len(snap.Crystals))
	}
	for _, cr := range snap.Crystals {
		// 0.5 of MaxGrowth=2 = 1
		if cr.Growth != 1 {
			t.Fatalf("expected intro crystals at growth=1 (0.5*max), got %d", cr.Growth)
		}
	}
}

func TestCaveCrystalsCapsAtMaxCrystals(t *testing.T) {
	cc := NewCaveCrystals(80, 40, 4, CaveCrystalsConfig{MaxCrystals: 3, MaxGrowth: 2})
	for i := 0; i < 10; i++ {
		cc.TriggerEvent("nucleus-spawn")
	}
	snap := cc.Snapshot()
	if len(snap.Crystals) != 3 {
		t.Fatalf("expected field to cap at MaxCrystals=3, got %d", len(snap.Crystals))
	}
}

func TestCaveCrystalsSnapshotRestoreRoundtrip(t *testing.T) {
	src := NewCaveCrystals(120, 60, 5, CaveCrystalsConfig{MaxCrystals: 4, MaxGrowth: 3})
	src.TriggerEvent("nucleus-spawn")
	src.TriggerEvent("growth-pulse")
	src.TriggerEvent("nucleus-spawn")
	for i := 0; i < 5; i++ {
		src.Step()
	}
	snap := src.Snapshot()

	dst := NewCaveCrystals(120, 60, 99, CaveCrystalsConfig{MaxCrystals: 4, MaxGrowth: 3})
	dst.RestoreSnapshot(snap)
	dstSnap := dst.Snapshot()

	if len(dstSnap.Crystals) != len(snap.Crystals) {
		t.Fatalf("restore lost crystals: src=%d dst=%d", len(snap.Crystals), len(dstSnap.Crystals))
	}
	for i, src := range snap.Crystals {
		got := dstSnap.Crystals[i]
		if got.Col != src.Col || got.Growth != src.Growth || got.Variant != src.Variant {
			t.Fatalf("crystal %d mismatch after restore: src=%+v dst=%+v", i, src, got)
		}
	}
	if dstSnap.Tick != snap.Tick {
		t.Fatalf("tick mismatch after restore: src=%d dst=%d", snap.Tick, dstSnap.Tick)
	}
}

func TestCaveCrystalsEndingHaltsNewNuclei(t *testing.T) {
	cc := NewCaveCrystals(120, 60, 6, CaveCrystalsConfig{
		MaxCrystals:   8,
		MaxGrowth:     2,
		NucleusChance: 1, // forces a nucleus every tick if allowed
	})
	cc.TriggerEvent("ending")
	before := len(cc.Snapshot().Crystals)
	for i := 0; i < 20; i++ {
		cc.Step()
	}
	after := cc.Snapshot()
	if len(after.Crystals) != before {
		t.Fatalf("ending should suppress new nuclei: before=%d after=%d", before, len(after.Crystals))
	}
}

func TestCaveCrystalsGridCopyHasContent(t *testing.T) {
	cc := NewCaveCrystals(48, 24, 7, CaveCrystalsConfig{MaxCrystals: 4, MaxGrowth: 2, IntroBurst: 2, IntroGrowth: 1})
	cc.TriggerEvent("intro")
	grid := cc.GridCopy()
	if len(grid) != 24 || len(grid[0]) != 48 {
		t.Fatalf("grid shape mismatch: %dx%d", len(grid), len(grid[0]))
	}
	filled := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatalf("expected GridCopy to paint pixels, got empty grid")
	}
}
