package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

func TestMagicPortalDevRoutes(t *testing.T) {
	if got, ok := devPageEffectFromPath("/dev/magic-portal"); !ok || got != "magic-portal" {
		t.Fatalf("devPageEffectFromPath(/dev/magic-portal) = %q, %v; want magic-portal, true", got, ok)
	}
	if got, ok := effectFromSchemaPath("/effects/magic-portal/schema"); !ok || got != "magic-portal" {
		t.Fatalf("effectFromSchemaPath(/effects/magic-portal/schema) = %q, %v; want magic-portal, true", got, ok)
	}
	session, err := newDevSession("magic-portal")
	if err != nil {
		t.Fatalf("newDevSession(magic-portal): %v", err)
	}
	if snap := session.snapshot(); snap.Type != "magic-portal" {
		t.Fatalf("snapshot type = %q, want magic-portal", snap.Type)
	}
}

func TestMagicPortalScenePresetsUseTunedPortalEnvelope(t *testing.T) {
	wantNames := map[string]bool{
		"arcane blue":   true,
		"infernal red":  true,
		"ancient amber": true,
		"dormant":       true,
	}
	if len(magicPortalScenePresets) != len(wantNames) {
		t.Fatalf("magic portal presets = %d, want %d", len(magicPortalScenePresets), len(wantNames))
	}

	for i, preset := range magicPortalScenePresets {
		if !wantNames[preset.name] {
			t.Fatalf("unexpected preset name %q", preset.name)
		}
		cfg := magicPortalSceneConfig(rngutil.New(int64(100+i)), preset)
		if cfg.PulsePeriod < preset.pulseMin || cfg.PulsePeriod > preset.pulseMax {
			t.Fatalf("%s pulse period = %d outside [%d,%d]", preset.name, cfg.PulsePeriod, preset.pulseMin, preset.pulseMax)
		}
		if cfg.EmberRate < preset.emberMin || cfg.EmberRate > preset.emberMax {
			t.Fatalf("%s ember rate = %d outside [%d,%d]", preset.name, cfg.EmberRate, preset.emberMin, preset.emberMax)
		}
		if cfg.PulsePeriod < 150 {
			t.Fatalf("%s pulse period = %d; pulse would read too strobing", preset.name, cfg.PulsePeriod)
		}
		if cfg.RuneCount < 12 || cfg.RuneCount > 18 || cfg.RingWidth < 2 || cfg.RingWidth > 3.3 {
			t.Fatalf("%s lost readable gate silhouette tuning: %+v", preset.name, cfg)
		}
		if angularDistance(cfg.Hue, preset.hue) > 7 {
			t.Fatalf("%s hue = %.2f too far from preset hue %.2f", preset.name, cfg.Hue, preset.hue)
		}
		if preset.dormant {
			if cfg.Sat > 0.2 || cfg.LightMax > 0.55 || cfg.Glow > 0.5 {
				t.Fatalf("dormant preset too bright/saturated: %+v", cfg)
			}
		} else if cfg.Sat < 0.65 || cfg.LightMax < 0.78 || cfg.Glow < 0.55 {
			t.Fatalf("%s preset too dim to read as active portal: %+v", preset.name, cfg)
		}
	}
}

func TestGenerateMagicPortalSceneReturnsNamedPresetConfig(t *testing.T) {
	for seed := int64(1); seed <= 32; seed++ {
		scene := generateMagicPortalScene(rngutil.New(seed), 17, 900)
		if scene.StartedAtTick != 17 || scene.DurationTicks != 900 {
			t.Fatalf("scene timing = start %d duration %d, want 17 / 900", scene.StartedAtTick, scene.DurationTicks)
		}
		var cfg sim.MagicPortalConfig
		if err := json.Unmarshal(scene.Config, &cfg); err != nil {
			t.Fatalf("unmarshal scene config seed %d: %v", seed, err)
		}
		if nameForMagicPortalConfig(cfg) != scene.Name {
			t.Fatalf("scene %q config classifies as %q: %+v", scene.Name, nameForMagicPortalConfig(cfg), cfg)
		}
	}
}

func TestMagicPortalSceneInterpolationDriftsHueAndCadence(t *testing.T) {
	rt := &magicPortalRuntime{sim: sim.NewMagicPortal(64, 36, 1, sim.MagicPortalConfig{})}
	from := sim.NormalizeMagicPortalConfig(sim.MagicPortalConfig{Hue: 350, Sat: 0.7, LightMin: 0.1, LightMax: 0.8, PulsePeriod: 200, EmberRate: 2})
	to := sim.NormalizeMagicPortalConfig(sim.MagicPortalConfig{Hue: 10, Sat: 0.8, LightMin: 0.12, LightMax: 0.9, PulsePeriod: 300, EmberRate: 4})
	fromData, _ := json.Marshal(from)
	toData, _ := json.Marshal(to)

	data, err := rt.InterpolateConfig(fromData, toData, 0.5)
	if err != nil {
		t.Fatalf("InterpolateConfig: %v", err)
	}
	var got sim.MagicPortalConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal interpolated config: %v", err)
	}
	if angularDistance(got.Hue, 0) > 0.001 {
		t.Fatalf("hue interpolation should cross 0 degrees, got %.4f", got.Hue)
	}
	if got.PulsePeriod != 250 {
		t.Fatalf("pulse period = %d, want 250", got.PulsePeriod)
	}
	if got.EmberRate != 3 {
		t.Fatalf("ember rate = %d, want 3", got.EmberRate)
	}
}

func angularDistance(a, b float64) float64 {
	d := math.Abs(normalizeMagicPortalHue(a) - normalizeMagicPortalHue(b))
	if d > 180 {
		return 360 - d
	}
	return d
}
