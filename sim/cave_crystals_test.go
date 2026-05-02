package sim

import "testing"

func TestCaveCrystalsNucleusGrowsThroughSteps(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 1, CaveCrystalsConfig{
		MaxSteps:   3,
		GrowthDur:  4,
		NucleateP:  1, // make sure we get spawns regardless of intro
		MaxCount:   4,
		IntroDur:   1,
		IntroSeed:  1,
	})
	if !cc.TriggerEvent("nucleus-spawn") {
		t.Fatalf("expected nucleus-spawn to be accepted")
	}
	snap := cc.Snapshot()
	if len(snap.Crystals) == 0 {
		t.Fatalf("expected at least one crystal after nucleus-spawn, got 0")
	}
	// Find a crystal we can advance and march it through every growth step.
	idx := -1
	for i, cr := range snap.Crystals {
		if cr.Step < cr.MaxStep {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("expected at least one growing crystal, got %v", snap.Crystals)
	}
	for i := 0; i < 200 && cc.Snapshot().Crystals[idx].Step < cc.Snapshot().Crystals[idx].MaxStep; i++ {
		cc.Step()
	}
	post := cc.Snapshot()
	if len(post.Crystals) <= idx {
		t.Fatalf("crystal %d disappeared during growth (have %d crystals)", idx, len(post.Crystals))
	}
	if post.Crystals[idx].Step != post.Crystals[idx].MaxStep {
		t.Fatalf("expected crystal %d to reach max step (%d), got %d", idx, post.Crystals[idx].MaxStep, post.Crystals[idx].Step)
	}
}

func TestCaveCrystalsCrystalPopLandsFullyGrown(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 7, CaveCrystalsConfig{MaxSteps: 5, MaxCount: 8})
	if !cc.TriggerEvent("crystal-pop") {
		t.Fatalf("expected crystal-pop to be accepted")
	}
	snap := cc.Snapshot()
	popped := -1
	for i, cr := range snap.Crystals {
		if cr.Popped {
			popped = i
			break
		}
	}
	if popped < 0 {
		t.Fatalf("expected one popped crystal, got %v", snap.Crystals)
	}
	if snap.Crystals[popped].Step != snap.Crystals[popped].MaxStep {
		t.Fatalf("popped crystal should be at max step, got %d/%d", snap.Crystals[popped].Step, snap.Crystals[popped].MaxStep)
	}
}

func TestCaveCrystalsEndingResetsField(t *testing.T) {
	cc := NewCaveCrystals(160, 80, 3, CaveCrystalsConfig{
		MaxCount:     6,
		IntroDur:     50,
		IntroSeed:    3,
		EndingDur:    2,
		EndingLinger: 1,
	})
	for i := 0; i < 5; i++ {
		cc.TriggerEvent("nucleus-spawn")
	}
	preCount := len(cc.Snapshot().Crystals)
	if preCount == 0 {
		t.Fatalf("expected crystals before ending")
	}
	cc.TriggerEvent("ending")
	// March through the ending window. EndingDur=2 + EndingLinger=1 = 3
	// ticks, so on the 3rd Step the ending finishes and an intro
	// auto-restart resets the crystal list.
	for i := 0; i < 4; i++ {
		cc.Step()
	}
	post := cc.Snapshot()
	if post.IntroTicks <= 0 {
		t.Fatalf("expected intro to auto-restart after ending, got introTicks=%d", post.IntroTicks)
	}
	if post.EndingTicks != 0 {
		t.Fatalf("expected ending window to be cleared, got endingTicks=%d", post.EndingTicks)
	}
	if len(post.Crystals) >= preCount {
		t.Fatalf("expected ending+intro to reset the crystal list (was %d, now %d)", preCount, len(post.Crystals))
	}
}

func TestCaveCrystalsSchemaContainsTriggers(t *testing.T) {
	schema := CaveCrystalsSchema()
	if schema.Name != "cave-crystals" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{
		"nucleus-spawn": false, // not surfaced via knob trigger; tested via TriggerEvent
		"crystal-pop":   true,
		"sparkle-burst": true,
		"quiet-cave":    true,
		"intro":         true,
		"ending":        true,
	}
	got := map[string]bool{}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			got[k.Trigger] = true
		}
	}
	for trig, requireKnob := range want {
		if !requireKnob {
			continue
		}
		if !got[trig] {
			t.Fatalf("schema missing knob-bound trigger %q", trig)
		}
	}
	// And the bare TriggerEvent surface accepts every event name we promise.
	cc := NewCaveCrystals(120, 60, 1, CaveCrystalsConfig{})
	for _, evt := range []string{"nucleus-spawn", "crystal-pop", "growth-pulse", "sparkle-burst", "quiet-cave", "intro", "ending"} {
		if !cc.TriggerEvent(evt) {
			t.Fatalf("TriggerEvent(%q) returned false", evt)
		}
	}
	if cc.TriggerEvent("does-not-exist") {
		t.Fatalf("TriggerEvent should return false for unknown event")
	}
}
