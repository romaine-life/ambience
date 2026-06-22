package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/romaine-life/ambience/sim"
)

func withSharedControlTestState(t *testing.T, a *atmosphere) {
	t.Helper()
	prevShared := shared
	prevAuth := controlAuth
	shared = a
	controlAuth = newControlAuthenticator("", oidcControlAuthConfig{})
	t.Cleanup(func() {
		shared = prevShared
		controlAuth = prevAuth
	})
}

func TestServeSharedNextEffectRotates(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Allowed: []string{"rain", "campfire"},
	})
	withSharedControlTestState(t, a)

	req := httptest.NewRequest(http.MethodPost, "/next-effect", nil)
	rec := httptest.NewRecorder()
	serveSharedNextEffect(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if got := a.snapshot().Type; got != "campfire" {
		t.Fatalf("type after /next-effect = %q, want campfire", got)
	}
}

func TestServeSharedNextEffectRejectsNoAlternateEffect(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{
		Allowed: []string{"rain"},
	})
	withSharedControlTestState(t, a)

	req := httptest.NewRequest(http.MethodPost, "/next-effect", nil)
	rec := httptest.NewRecorder()
	serveSharedNextEffect(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := a.snapshot().Type; got != "rain" {
		t.Fatalf("type after rejected /next-effect = %q, want rain", got)
	}
}

func TestServeSharedNextEffectRequiresPost(t *testing.T) {
	withSharedControlTestState(t, newAtmosphere(sim.Config{}))

	req := httptest.NewRequest(http.MethodGet, "/next-effect", nil)
	rec := httptest.NewRecorder()
	serveSharedNextEffect(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestParseScenePolicyValuesUsesMinutesAndVariation(t *testing.T) {
	values := url.Values{}
	values.Set(sceneMinMinutesKey, "15")
	values.Set(sceneMaxMinutesKey, "45")
	values.Set(sceneTransitionMinutesKey, "7.5")
	values.Set(sceneVariationKey, "0.25")

	got, err := parseScenePolicyValues(values, defaultScenePolicy().data())
	if err != nil {
		t.Fatalf("parseScenePolicyValues: %v", err)
	}
	if got.MinTicks != ticksFor(15*time.Minute) || got.MaxTicks != ticksFor(45*time.Minute) {
		t.Fatalf("duration ticks = %d..%d, want %d..%d", got.MinTicks, got.MaxTicks, ticksFor(15*time.Minute), ticksFor(45*time.Minute))
	}
	if got.TransitionTicks != ticksFor(450*time.Second) {
		t.Fatalf("transition ticks = %d, want %d", got.TransitionTicks, ticksFor(450*time.Second))
	}
	if got.Variation != 0.25 {
		t.Fatalf("variation = %.2f, want 0.25", got.Variation)
	}
}

func TestParseRotationPolicyValuesUsesMinutesEnabledAndPool(t *testing.T) {
	values := url.Values{}
	values.Set(rotationEnabledKey, "0")
	values.Set(rotationCadenceMinutesKey, "45")
	values.Set(rotationAllowPrefix+"rain", "1")
	values.Set(rotationAllowPrefix+"campfire", "1")
	values.Set(rotationAllowPrefix+"aurora", "0")

	got, err := parseRotationPolicyValues(values, rotationPolicy{
		Enabled:      true,
		CadenceTicks: defaultRotationCadenceTicks,
	}.data())
	if err != nil {
		t.Fatalf("parseRotationPolicyValues: %v", err)
	}
	if got.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if got.CadenceTicks != ticksFor(45*time.Minute) {
		t.Fatalf("cadence ticks = %d, want %d", got.CadenceTicks, ticksFor(45*time.Minute))
	}
	if len(got.Allowed) != 2 || got.Allowed[0] != "campfire" || got.Allowed[1] != "rain" {
		t.Fatalf("allowed = %v, want [campfire rain]", got.Allowed)
	}
}

func TestParseRotationPolicyValuesRejectsEmptyPool(t *testing.T) {
	values := url.Values{}
	for _, effectType := range registeredEffectTypes() {
		values.Set(rotationAllowPrefix+effectType, "0")
	}

	if _, err := parseRotationPolicyValues(values, rotationPolicy{
		Enabled:      true,
		CadenceTicks: defaultRotationCadenceTicks,
	}.data()); err == nil {
		t.Fatal("expected empty effect pool error")
	}
}
