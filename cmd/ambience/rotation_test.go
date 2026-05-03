package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nelsong6/ambience/rngutil"
	"github.com/nelsong6/ambience/sim"
)

func TestPickNextEffectAvoidsCurrent(t *testing.T) {
	rng := rngutil.New(1)
	pool := []string{"rain", "campfire", "aurora"}
	for i := 0; i < 64; i++ {
		got := pickNextEffect(rng, pool, "rain")
		if got == "rain" {
			t.Fatalf("pick %d returned current: %q", i, got)
		}
	}
}

func TestPickNextEffectSinglePoolReturnsCurrent(t *testing.T) {
	rng := rngutil.New(1)
	if got := pickNextEffect(rng, []string{"rain"}, "rain"); got != "rain" {
		t.Fatalf("single-element pool with current returned %q, want %q", got, "rain")
	}
}

func TestPickNextEffectEmptyPoolReturnsCurrent(t *testing.T) {
	rng := rngutil.New(1)
	if got := pickNextEffect(rng, nil, "rain"); got != "rain" {
		t.Fatalf("empty pool returned %q, want %q", got, "rain")
	}
}

func TestRotationPolicyAllowedFiltersUnknown(t *testing.T) {
	p := rotationPolicy{Allowed: []string{"rain", "definitely-not-an-effect", "campfire"}}
	got := p.resolvedAllowedEffects()
	want := []string{"campfire", "rain"}
	if len(got) != len(want) {
		t.Fatalf("resolvedAllowedEffects = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("resolvedAllowedEffects[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestRotationPolicyAllowedDefaultsToRegistry(t *testing.T) {
	p := rotationPolicy{}
	got := p.resolvedAllowedEffects()
	if len(got) == 0 {
		t.Fatal("expected non-empty default pool")
	}
	if len(got) != len(effectRegistry) {
		t.Fatalf("default pool len = %d, want %d (full registry)", len(got), len(effectRegistry))
	}
}

func TestMaybeRotateEffectDisabledByDefault(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	if rotated := a.maybeRotateEffect(1_000_000); rotated {
		t.Fatal("rotation fired with default (disabled) policy")
	}
}

func TestMaybeRotateEffectFiresAfterCadence(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain", "campfire"},
	})
	if rotated := a.maybeRotateEffect(5); rotated {
		t.Fatal("rotation fired before cadence elapsed")
	}
	if rotated := a.maybeRotateEffect(10); !rotated {
		t.Fatal("rotation did not fire at cadence elapsed")
	}
	snap := a.snapshot()
	if snap.Type == "rain" {
		t.Fatalf("expected effect to change after rotation; still %q", snap.Type)
	}
	if snap.Type != "campfire" {
		t.Fatalf("expected campfire after rotation; got %q", snap.Type)
	}
	if snap.CurrentScene.Name == "campfire" {
		t.Fatalf("current scene name = raw effect %q; want generated scene label", snap.CurrentScene.Name)
	}
	if snap.NextScene.Name == "campfire" {
		t.Fatalf("next scene name = raw effect %q; want generated scene label", snap.NextScene.Name)
	}
}

func TestNonRainAtmosphereStartsWithGeneratedSceneLabels(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("aurora", 123)
	snap := a.snapshot()
	if snap.CurrentScene.Name == "aurora" {
		t.Fatalf("current scene name = raw effect %q; want generated scene label", snap.CurrentScene.Name)
	}
	if snap.NextScene.Name == "aurora" {
		t.Fatalf("next scene name = raw effect %q; want generated scene label", snap.NextScene.Name)
	}
	if !strings.Contains(snap.CurrentScene.Name, "aurora") &&
		!strings.Contains(snap.CurrentScene.Name, "lights") &&
		!strings.Contains(snap.CurrentScene.Name, "skyfire") &&
		!strings.Contains(snap.CurrentScene.Name, "curtain") {
		t.Fatalf("aurora scene name %q does not read like an aurora variant", snap.CurrentScene.Name)
	}
}

func TestNonRainSceneRotationKeepsEffectSceneLabels(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("aurora", 123)
	before := a.snapshot()
	a.rotateScene(before.CurrentScene.DurationTicks)
	after := a.snapshot()

	if after.Type != "aurora" {
		t.Fatalf("type after scene rotation = %q, want aurora", after.Type)
	}
	if after.CurrentScene.Name != before.NextScene.Name {
		t.Fatalf("current scene after rotation = %q, want prior next %q", after.CurrentScene.Name, before.NextScene.Name)
	}
	if after.NextScene.Name == "aurora" {
		t.Fatalf("next scene name = raw effect %q; want generated scene label", after.NextScene.Name)
	}
	if after.NextScene.Config != (sim.Config{}) {
		t.Fatalf("non-rain next scene unexpectedly has rain config: %+v", after.NextScene.Config)
	}
}

func TestMaybeRotateEffectBroadcastsSnapshot(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain", "aurora"},
	})
	ch := a.addListener()
	defer a.removeListener(ch)

	rotated := a.maybeRotateEffect(10)
	if !rotated {
		t.Fatal("rotation did not fire")
	}

	deadline := time.After(time.Second)
	gotSnapshot := false
	for !gotSnapshot {
		select {
		case cmd := <-ch:
			if cmd.Kind == "snapshot" {
				gotSnapshot = true
			}
		case <-deadline:
			t.Fatal("no snapshot broadcast after rotation")
		}
	}
}

func TestMaybeRotateEffectSinglePoolNoOp(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain"},
	})
	if rotated := a.maybeRotateEffect(50); rotated {
		t.Fatal("rotation fired with single-effect pool")
	}
	if got := a.snapshot().Type; got != "rain" {
		t.Fatalf("type = %q after no-op rotation; want rain", got)
	}
}

func TestRotationStartTickResetsAfterRotation(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain", "campfire"},
	})
	a.maybeRotateEffect(10)
	a.mu.Lock()
	got := a.rotationStartTick
	a.mu.Unlock()
	if got != 0 {
		t.Fatalf("rotationStartTick after rotation = %d; want 0 (anchored to new effect's local tick)", got)
	}
}

func TestRotationStateSurvivesPersistRoundTrip(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain"},
	})
	a.mu.Lock()
	a.rotationStartTick = 42
	want := a.rotationStartTick
	a.mu.Unlock()

	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	restored.mu.Lock()
	got := restored.rotationStartTick
	restored.mu.Unlock()
	if got != want {
		t.Fatalf("rotationStartTick after restore = %d; want %d", got, want)
	}
}

func TestRestoreOldStateAnchorsRotationAtRestoredTick(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	for i := 0; i < 25; i++ {
		a.effect.Step()
	}
	state := a.persistedState()
	state.RotationStartTick = nil

	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphereWithPolicy(context.Background(), store, rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
	})
	restored.mu.Lock()
	got := restored.rotationStartTick
	restored.mu.Unlock()
	if got != restored.effect.CurrentTick() {
		t.Fatalf("rotationStartTick after old-state restore = %d; want current tick %d", got, restored.effect.CurrentTick())
	}
	if rotated := restored.maybeRotateEffect(restored.effect.CurrentTick()); rotated {
		t.Fatal("old-state restore rotated immediately instead of preserving restored effect")
	}
}

func TestRestoreWithPolicyStillPrefersSavedEffect(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("campfire", 123)
	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphereWithPolicy(context.Background(), store, rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain", "aurora"},
	})
	if got := restored.snapshot().Type; got != "campfire" {
		t.Fatalf("restored type = %q, want saved campfire", got)
	}
}

func TestRestoreReplacesLegacyRawNonRainSceneNames(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("aurora", 123)
	state := a.persistedState()
	state.CurrentScene.Name = "aurora"
	state.NextScene.Name = "aurora"
	state.NextScene.DurationTicks = 0

	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	snap := restored.snapshot()
	if snap.CurrentScene.Name == "aurora" {
		t.Fatalf("restored current scene name = raw effect %q; want generated scene label", snap.CurrentScene.Name)
	}
	if snap.NextScene.Name == "aurora" {
		t.Fatalf("restored next scene name = raw effect %q; want generated scene label", snap.NextScene.Name)
	}
	if snap.NextScene.DurationTicks != snap.CurrentScene.DurationTicks {
		t.Fatalf("restored next duration = %d, want current duration %d", snap.NextScene.DurationTicks, snap.CurrentScene.DurationTicks)
	}
}

func TestLoadRotationPolicyFromEnvDefault(t *testing.T) {
	t.Setenv("AMBIENCE_ROTATION_ENABLED", "")
	t.Setenv("AMBIENCE_ROTATION_CADENCE", "")
	t.Setenv("AMBIENCE_ROTATION_EFFECTS", "")
	p := loadRotationPolicyFromEnv()
	if !p.Enabled {
		t.Fatal("default policy disabled; want enabled")
	}
	if p.CadenceTicks != defaultRotationCadenceTicks {
		t.Fatalf("default cadence = %d; want %d", p.CadenceTicks, defaultRotationCadenceTicks)
	}
	if len(p.Allowed) != 0 {
		t.Fatalf("default allowed = %v; want empty (registry fallback)", p.Allowed)
	}
}

func TestLoadRotationPolicyFromEnvOverrides(t *testing.T) {
	t.Setenv("AMBIENCE_ROTATION_ENABLED", "false")
	t.Setenv("AMBIENCE_ROTATION_CADENCE", "30s")
	t.Setenv("AMBIENCE_ROTATION_EFFECTS", "rain, campfire ,aurora")
	p := loadRotationPolicyFromEnv()
	if p.Enabled {
		t.Fatal("expected disabled")
	}
	wantTicks := int(30 * time.Second / tickRate)
	if p.CadenceTicks != wantTicks {
		t.Fatalf("cadence ticks = %d; want %d", p.CadenceTicks, wantTicks)
	}
	if len(p.Allowed) != 3 || p.Allowed[0] != "rain" || p.Allowed[1] != "campfire" || p.Allowed[2] != "aurora" {
		t.Fatalf("allowed = %v; want [rain campfire aurora]", p.Allowed)
	}
}
