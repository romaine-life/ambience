package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

func withSharedControlTestState(t *testing.T, a *atmosphere) {
	t.Helper()
	prevShared := shared
	prevAuth := controlAuth
	shared = a
	controlAuth = newControlAuthenticator("", microsoftControlAuthConfig{})
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
