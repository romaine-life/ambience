package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
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

func TestRotationPolicyAllowedEmptyMeansRegistryFallback(t *testing.T) {
	p := rotationPolicy{}
	got := p.resolvedAllowedEffects()
	if len(got) == 0 {
		t.Fatal("expected non-empty registry fallback pool")
	}
	if len(got) != len(effectRegistry) {
		t.Fatalf("fallback pool len = %d, want %d (full registry)", len(got), len(effectRegistry))
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
	promotedConfig := cloneRaw(before.NextScene.Config)
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
	if len(after.CurrentScene.Config) == 0 || len(after.NextScene.Config) == 0 {
		t.Fatalf("non-rain scenes missing configs: current=%s next=%s", after.CurrentScene.Config, after.NextScene.Config)
	}
	if !configsEqualJSON(after.Config, promotedConfig) {
		t.Fatalf("runtime config did not advance to promoted scene config\ngot: %s\nwant: %s", after.Config, promotedConfig)
	}
}

func TestRainSceneRotationKeepsDriftCapability(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 123)
	before := a.snapshot()
	a.rotateScene(before.CurrentScene.DurationTicks)

	a.mu.Lock()
	dur := a.transitionDur
	from := cloneRaw(a.transitionFrom)
	to := cloneRaw(a.transitionTo)
	a.mu.Unlock()

	if dur <= 0 {
		t.Fatalf("rain transition duration = %d; want positive drift window", dur)
	}
	if len(from) == 0 || len(to) == 0 {
		t.Fatalf("rain transition missing configs: from=%s to=%s", from, to)
	}
	if !configsEqualJSON(to, before.NextScene.Config) {
		t.Fatalf("rain transition target = %s, want promoted scene config %s", to, before.NextScene.Config)
	}
}

func TestRainTransitionInterpolatesTextureAndFrontPlane(t *testing.T) {
	from := sim.NormalizeConfig(sim.Config{
		Speed:          1.6,
		SpawnEvery:     5,
		SpawnBurst:     3,
		StreakLen:      10,
		SheetDensity:   0.52,
		SheetStrength:  0.24,
		SheetLength:    9,
		SheetSpeed:     1.3,
		FrontDensity:   0.28,
		FrontStrength:  0.4,
		FrontLength:    18,
		FrontSpeed:     42,
		DownpourChance: 0.00008,
	})
	to := sim.NormalizeConfig(sim.Config{
		Speed:          2.2,
		SpawnEvery:     3,
		SpawnBurst:     5,
		StreakLen:      14,
		SheetDensity:   0.76,
		SheetStrength:  0.38,
		SheetLength:    14,
		SheetSpeed:     1.9,
		FrontDensity:   0.5,
		FrontStrength:  0.65,
		FrontLength:    30,
		FrontSpeed:     70,
		DownpourChance: 0.00025,
	})

	got := lerpConfig(from, to, 0.5)
	if got.SheetDensity <= 0 || got.FrontDensity <= 0 {
		t.Fatalf("interpolated rain lost texture/front layers: %+v", got)
	}
	if got.SheetDensity == from.SheetDensity || got.SheetDensity == to.SheetDensity {
		t.Fatalf("sheet density did not interpolate: got %.3f from %.3f to %.3f", got.SheetDensity, from.SheetDensity, to.SheetDensity)
	}
	if got.FrontSpeed == from.FrontSpeed || got.FrontSpeed == to.FrontSpeed {
		t.Fatalf("front speed did not interpolate: got %.3f from %.3f to %.3f", got.FrontSpeed, from.FrontSpeed, to.FrontSpeed)
	}
}

func TestRainSceneVariationKeepsGeneratedSceneNearCurrent(t *testing.T) {
	rng := rngutil.New(99)
	base := generateRainScene(rng, 0, ticksFor(time.Hour))
	var baseCfg sim.Config
	if err := json.Unmarshal(base.Config, &baseCfg); err != nil {
		t.Fatalf("decode base config: %v", err)
	}

	near := generateRainSceneNear(rng, 0, ticksFor(time.Hour), base.Config, 0.2)
	var nearCfg sim.Config
	if err := json.Unmarshal(near.Config, &nearCfg); err != nil {
		t.Fatalf("decode near config: %v", err)
	}

	if diff := absFloat(nearCfg.Speed - baseCfg.Speed); diff > 0.75*0.21 {
		t.Fatalf("near speed drift = %.3f, want within 20%% of full generated speed envelope", diff)
	}
	if diff := absFloat(nearCfg.FrontSpeed - baseCfg.FrontSpeed); diff > 28*0.21 {
		t.Fatalf("near front speed drift = %.3f, want within 20%% of full generated front-speed envelope", diff)
	}
}

func TestGeneratedRainScenesStayInWeatherFieldRange(t *testing.T) {
	rng := rngutil.New(44)
	for i := 0; i < 64; i++ {
		scene := generateRainScene(rng, i*100, ticksFor(time.Hour))
		var cfg sim.Config
		if err := json.Unmarshal(scene.Config, &cfg); err != nil {
			t.Fatalf("decode generated rain config %d: %v", i, err)
		}
		if cfg.Speed < 1.5 || cfg.Speed > 2.35 || cfg.SpawnEvery < 3 || cfg.SpawnEvery > 5 || cfg.SpawnBurst < 3 || cfg.SpawnBurst > 5 || cfg.StreakLen < 10 {
			t.Fatalf("generated rain outside 60 Hz foreground range %d: %+v", i, cfg)
		}
		if cfg.Layers < 2 || cfg.LayerBalance < 0.45 {
			t.Fatalf("generated rain lacks background depth %d: %+v", i, cfg)
		}
		if cfg.SheetDensity < 0.5 || cfg.SheetStrength <= 0 || cfg.SheetLength < 9 || cfg.SheetSpeed < 1.3 {
			t.Fatalf("generated rain lacks atmospheric sheet %d: %+v", i, cfg)
		}
		if cfg.FrontDensity < 0.25 || cfg.FrontStrength < 0.35 || cfg.FrontLength < 18 || cfg.FrontSpeed < 40 {
			t.Fatalf("generated rain lacks near-window front plane %d: %+v", i, cfg)
		}
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
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

func TestRotateToNextEffectIgnoresAutomaticRotationGate(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      false,
		CadenceTicks: 10_000,
		Allowed:      []string{"rain", "campfire"},
	})
	before := a.snapshot()

	if rotated := a.rotateToNextEffect(); !rotated {
		t.Fatal("next effect did not rotate")
	}
	after := a.snapshot()
	if got := after.Type; got != "campfire" {
		t.Fatalf("type after next effect = %q, want campfire", got)
	}
	if after.Seed == before.Seed {
		t.Fatalf("seed after next effect = %d, want fresh runtime seed", after.Seed)
	}
}

func TestRotateToNextEffectSinglePoolNoOp(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Allowed: []string{"rain"},
	})

	if rotated := a.rotateToNextEffect(); rotated {
		t.Fatal("next effect rotated with single-effect pool")
	}
	if got := a.snapshot().Type; got != "rain" {
		t.Fatalf("type after no-op next effect = %q, want rain", got)
	}
}

func TestRotateToNextEffectBroadcastsSnapshot(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Allowed: []string{"rain", "aurora"},
	})
	ch := a.addListener()
	defer a.removeListener(ch)

	if rotated := a.rotateToNextEffect(); !rotated {
		t.Fatal("next effect did not rotate")
	}

	deadline := time.After(time.Second)
	for {
		select {
		case cmd := <-ch:
			if cmd.Kind == "snapshot" {
				return
			}
		case <-deadline:
			t.Fatal("no snapshot broadcast after next effect")
		}
	}
}

func TestCrossEffectRotationInitializesRuntimeWithSceneConfig(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Enabled:      true,
		CadenceTicks: 10,
		Allowed:      []string{"rain", "campfire"},
	})
	if rotated := a.maybeRotateEffect(10); !rotated {
		t.Fatal("rotation did not fire")
	}
	snap := a.snapshot()
	if snap.Type != "campfire" {
		t.Fatalf("type after rotation = %q, want campfire", snap.Type)
	}
	if len(snap.CurrentScene.Config) == 0 {
		t.Fatal("current scene missing config")
	}
	if !configsEqualJSON(snap.Config, snap.CurrentScene.Config) {
		t.Fatalf("runtime config does not match current scene config\ngot: %s\nwant: %s", snap.Config, snap.CurrentScene.Config)
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

	store := &memoryStore{}
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

	store := &memoryStore{}
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
	store := &memoryStore{}
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

	store := &memoryStore{}
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

func TestPersistedScenesRestoreWithConfigs(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("aurora", 123)
	before := a.snapshot()
	store := &memoryStore{}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	after := restored.snapshot()
	if !configsEqualJSON(after.CurrentScene.Config, before.CurrentScene.Config) {
		t.Fatalf("current scene config after restore = %s, want %s", after.CurrentScene.Config, before.CurrentScene.Config)
	}
	if !configsEqualJSON(after.NextScene.Config, before.NextScene.Config) {
		t.Fatalf("next scene config after restore = %s, want %s", after.NextScene.Config, before.NextScene.Config)
	}
}

func TestLegacySceneRestoreFillsMissingConfigs(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("aurora", 123)
	state := a.persistedState()
	state.CurrentScene.Config = nil
	state.NextScene.Config = nil

	store := &memoryStore{}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	snap := restored.snapshot()
	if len(snap.CurrentScene.Config) == 0 {
		t.Fatal("restored current scene missing migrated config")
	}
	if len(snap.NextScene.Config) == 0 {
		t.Fatal("restored next scene missing migrated config")
	}
}

func TestAtmosphereSceneCodeHasNoRainBranch(t *testing.T) {
	files := []string{"atmosphere.go", "scene.go", "persistence.go"}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, forbidden := range []string{`effectType == "rain"`, `effectType != "rain"`, `hasRainConfig`} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s still contains rain-specific scene branch %q", file, forbidden)
			}
		}
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
		t.Fatalf("default allowed = %v; want empty for all registered effects", p.Allowed)
	}
	resolved := p.resolvedAllowedEffects()
	if len(resolved) <= 1 {
		t.Fatalf("resolved default allowed = %v; want multiple registered effects", resolved)
	}
	if !slices.Contains(resolved, "rain") || !slices.Contains(resolved, "campfire") {
		t.Fatalf("resolved default allowed = %v; want registered effects including rain and campfire", resolved)
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
