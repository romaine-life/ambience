package main

import "testing"

// Every Glimmung phase must resolve to a registry, and the union of slugs must
// match the retired shell scripts' slug set (the workflow re-registration binds
// each (phase, slug) to this binary).
func TestRegistryForAllPhases(t *testing.T) {
	wantCounts := map[string]int{
		"env-prep":    7,
		"test-plan":   6,
		"implement":   11,
		"env-destroy": 5,
	}
	for phase, count := range wantCounts {
		reg, ok := registryFor(phase)
		if !ok {
			t.Errorf("phase %q did not resolve to a registry", phase)
			continue
		}
		if got := len(reg.Slugs()); got != count {
			t.Errorf("phase %q has %d slugs, want %d (%v)", phase, got, count, reg.Slugs())
		}
	}
	if _, ok := registryFor("bogus"); ok {
		t.Error("unknown phase must not resolve")
	}
}
