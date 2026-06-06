package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDevPageEffectFromPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/dev", want: "rain", wantOK: true},
		{path: "/dev/", want: "rain", wantOK: true},
		{path: "/dev/aurora", want: "aurora", wantOK: true},
		{path: "/dev/beach", want: "beach", wantOK: true},
		{path: "/dev/campfire", want: "campfire", wantOK: true},
		{path: "/dev/dust", want: "dust", wantOK: true},
		{path: "/dev/autumn-leaves", want: "autumn-leaves", wantOK: true},
		{path: "/dev/fireflies", want: "fireflies", wantOK: true},
		{path: "/dev/lighthouse", want: "lighthouse", wantOK: true},
		{path: "/dev/rowboat", want: "rowboat", wantOK: true},
		{path: "/dev/snow", want: "snow", wantOK: true},
		{path: "/dev/starfield", want: "starfield", wantOK: true},
		{path: "/dev/underwater", want: "underwater", wantOK: true},
		{path: "/dev/waterfall", want: "waterfall", wantOK: true},
		{path: "/dev/windmill", want: "windmill", wantOK: true},
		{path: "/dev/wheat-field", want: "wheat-field", wantOK: true},
		{path: "/dev/unknown", wantOK: false},
		{path: "/dev/fireflies/extra", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := devPageEffectFromPath(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("devPageEffectFromPath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("devPageEffectFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestEffectFromSchemaPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/effects/rain/schema", want: "rain", wantOK: true},
		{path: "/effects/aurora/schema", want: "aurora", wantOK: true},
		{path: "/effects/beach/schema", want: "beach", wantOK: true},
		{path: "/effects/campfire/schema", want: "campfire", wantOK: true},
		{path: "/effects/autumn-leaves/schema", want: "autumn-leaves", wantOK: true},
		{path: "/effects/dust/schema", want: "dust", wantOK: true},
		{path: "/effects/fireflies/schema", want: "fireflies", wantOK: true},
		{path: "/effects/lighthouse/schema", want: "lighthouse", wantOK: true},
		{path: "/effects/rowboat/schema", want: "rowboat", wantOK: true},
		{path: "/effects/snow/schema", want: "snow", wantOK: true},
		{path: "/effects/starfield/schema", want: "starfield", wantOK: true},
		{path: "/effects/underwater/schema", want: "underwater", wantOK: true},
		{path: "/effects/waterfall/schema", want: "waterfall", wantOK: true},
		{path: "/effects/windmill/schema", want: "windmill", wantOK: true},
		{path: "/effects/wheat-field/schema", want: "wheat-field", wantOK: true},
		{path: "/effects/unknown/schema", wantOK: false},
		{path: "/effects/fireflies/not-schema", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := effectFromSchemaPath(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("effectFromSchemaPath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("effectFromSchemaPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestEffectFromPagePath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{path: "/effects/rain", want: "rain", wantOK: true},
		{path: "/effects/aurora", want: "aurora", wantOK: true},
		{path: "/effects/unknown", wantOK: false},
		{path: "/effects/rain/schema", wantOK: false},
		{path: "/effects/rain/extra", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := effectFromPagePath(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("effectFromPagePath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("effectFromPagePath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestServeEffectsRouteServesEffectPageWithEmbeds(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/effects/snow", nil)
	req.Host = "ambience.dev.romaine.life"
	rec := httptest.NewRecorder()

	serveEffectsRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<meta property="og:title" content="ambience effect - snow">`,
		`<meta property="og:url" content="https://ambience.dev.romaine.life/effects/snow">`,
		`<meta property="og:image" content="https://ambience.dev.romaine.life/og-image.png?effect=snow&amp;page=effect">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`url=/dev/snow`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("effect page missing %q in:\n%s", want, body)
		}
	}
}

func TestServeEffectsRouteKeepsSchemaJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/effects/snow/schema", nil)
	rec := httptest.NewRecorder()

	serveEffectsRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(rec.Body.String(), `"name":"snow"`) {
		t.Fatalf("schema response missing snow name: %s", rec.Body.String())
	}
}

func TestNewDevSessionFirefliesSnapshotType(t *testing.T) {
	session, err := newDevSession("fireflies")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "fireflies" {
		t.Fatalf("snapshot type = %q, want fireflies", snap.Type)
	}
}

func TestNewDevSessionWithGridSnapshotDimensions(t *testing.T) {
	session, err := newDevSessionWithGrid("rain", 320, 180)
	if err != nil {
		t.Fatalf("newDevSessionWithGrid: %v", err)
	}
	snap := session.snapshot()
	if snap.GridW != 320 || snap.GridH != 180 {
		t.Fatalf("snapshot grid = %dx%d, want 320x180", snap.GridW, snap.GridH)
	}
}

func TestNewDevSessionDustSnapshotType(t *testing.T) {
	session, err := newDevSession("dust")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "dust" {
		t.Fatalf("snapshot type = %q, want dust", snap.Type)
	}
}

func TestNewDevSessionWaterfallSnapshotType(t *testing.T) {
	session, err := newDevSession("waterfall")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "waterfall" {
		t.Fatalf("snapshot type = %q, want waterfall", snap.Type)
	}
}

func TestNewDevSessionSnowSnapshotType(t *testing.T) {
	session, err := newDevSession("snow")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "snow" {
		t.Fatalf("snapshot type = %q, want snow", snap.Type)
	}
}

func TestNewDevSessionAuroraSnapshotType(t *testing.T) {
	session, err := newDevSession("aurora")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "aurora" {
		t.Fatalf("snapshot type = %q, want aurora", snap.Type)
	}
}

func TestNewDevSessionBeachSnapshotType(t *testing.T) {
	session, err := newDevSession("beach")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "beach" {
		t.Fatalf("snapshot type = %q, want beach", snap.Type)
	}
}

func TestNewDevSessionCampfireSnapshotType(t *testing.T) {
	session, err := newDevSession("campfire")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "campfire" {
		t.Fatalf("snapshot type = %q, want campfire", snap.Type)
	}
}

func TestNewDevSessionWindmillSnapshotType(t *testing.T) {
	session, err := newDevSession("windmill")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "windmill" {
		t.Fatalf("snapshot type = %q, want windmill", snap.Type)
	}
}

func TestNewDevSessionLighthouseSnapshotType(t *testing.T) {
	session, err := newDevSession("lighthouse")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "lighthouse" {
		t.Fatalf("snapshot type = %q, want lighthouse", snap.Type)
	}
}

func TestNewDevSessionRowboatSnapshotType(t *testing.T) {
	session, err := newDevSession("rowboat")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "rowboat" {
		t.Fatalf("snapshot type = %q, want rowboat", snap.Type)
	}
}

func TestNewDevSessionUnderwaterSnapshotType(t *testing.T) {
	session, err := newDevSession("underwater")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "underwater" {
		t.Fatalf("snapshot type = %q, want underwater", snap.Type)
	}
}

func TestNewDevSessionWheatFieldSnapshotType(t *testing.T) {
	session, err := newDevSession("wheat-field")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "wheat-field" {
		t.Fatalf("snapshot type = %q, want wheat-field", snap.Type)
	}
}

func TestNewDevSessionAutumnLeavesSnapshotType(t *testing.T) {
	session, err := newDevSession("autumn-leaves")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "autumn-leaves" {
		t.Fatalf("snapshot type = %q, want autumn-leaves", snap.Type)
	}
}

func TestNewDevSessionStarfieldSnapshotType(t *testing.T) {
	session, err := newDevSession("starfield")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	snap := session.snapshot()
	if snap.Type != "starfield" {
		t.Fatalf("snapshot type = %q, want starfield", snap.Type)
	}
}

func TestDevSessionRandomizeConfigChangesSnapshotConfig(t *testing.T) {
	session, err := newDevSession("dust")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}

	before := session.snapshot()
	time.Sleep(time.Nanosecond)
	if _, err := session.randomizeConfig(99); err != nil {
		t.Fatalf("randomizeConfig: %v", err)
	}
	after := session.snapshot()

	if configsEqualJSON(before.Config, after.Config) {
		t.Fatal("expected randomized config to differ from previous session config")
	}
}

// TestDevSessionRecordsAppliedEvent verifies that a triggered event registers
// in the session's appliedEvents (the mechanical "trigger applied" signal the
// verifier asserts) and does not leak into an unrelated session.
func TestDevSessionRecordsAppliedEvent(t *testing.T) {
	s, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	if !s.triggerEvent("ending") {
		t.Fatalf("triggerEvent(ending) returned false")
	}
	// Drain the way the run loop would; "ending" is an explicit lifecycle event
	// (never auto-fired), so it must appear after the next tick.
	var found bool
	for i := 0; i < 5 && !found; i++ {
		s.stepAndBroadcast()
		for _, e := range s.snapshot().AppliedEvents {
			if e.Event == "ending" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("ending never registered in appliedEvents: %+v", s.snapshot().AppliedEvents)
	}

	other, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession other: %v", err)
	}
	other.stepAndBroadcast()
	for _, e := range other.snapshot().AppliedEvents {
		if e.Event == "ending" {
			t.Fatalf("isolated session leaked applied event 'ending'")
		}
	}
}

// TestDevSessionAppliedEventsRingBounded verifies the applied-event ring is
// bounded so a long-lived dev session cannot grow snapshots without limit.
func TestDevSessionAppliedEventsRingBounded(t *testing.T) {
	s, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	for i := 0; i < devAppliedEventsCap+10; i++ {
		s.recordApplied(i, "pulse")
	}
	if got := len(s.snapshot().AppliedEvents); got != devAppliedEventsCap {
		t.Fatalf("applied ring len = %d, want %d", got, devAppliedEventsCap)
	}
	// Oldest entries are evicted: the first retained tick should be 10.
	if first := s.snapshot().AppliedEvents[0].Tick; first != 10 {
		t.Fatalf("oldest retained tick = %d, want 10", first)
	}
}
