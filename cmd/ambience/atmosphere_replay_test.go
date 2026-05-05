package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nelsong6/ambience/sim"
)

func TestAtmosphereReplayAfterLastEventID(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	a.broadcast(Command{Kind: "config", Tick: 10, Data: mustJSON(t, sim.Config{Wind: 1})})
	a.broadcast(Command{Kind: "trigger", Tick: 11, Event: "gust"})
	a.broadcast(Command{Kind: "clock", Tick: 12, Data: mustJSON(t, clockData{Tick: 12})})

	a.mu.Lock()
	replay, ok := a.replayAfterLocked("1")
	a.mu.Unlock()
	if !ok {
		t.Fatal("expected replay to be available")
	}
	if len(replay) != 2 {
		t.Fatalf("replay length = %d, want 2", len(replay))
	}
	if replay[0].ID != "2" || replay[1].ID != "3" {
		t.Fatalf("replay IDs = %q/%q, want 2/3", replay[0].ID, replay[1].ID)
	}
}

func TestAtmosphereReplayStartsAfterSnapshotID(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	a.broadcast(Command{Kind: "metric", Tick: 21})
	a.broadcast(Command{Kind: "trigger", Tick: 22, Event: "gust"})

	a.mu.Lock()
	replay, ok := a.replayAfterLocked("0")
	a.mu.Unlock()
	if !ok {
		t.Fatal("expected replay from snapshot ID to be available")
	}
	if len(replay) != 2 || replay[0].ID != "1" || replay[1].ID != "2" {
		t.Fatalf("replay = %+v", replay)
	}
}

func TestAtmosphereReplayRejectsStaleLastEventID(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	for i := 0; i < authorityReplayBufferSize+2; i++ {
		a.broadcast(Command{Kind: "metric", Tick: i})
	}

	a.mu.Lock()
	replay, ok := a.replayAfterLocked("1")
	a.mu.Unlock()
	if ok {
		t.Fatalf("expected stale replay to be rejected, got %+v", replay)
	}
}

func TestStreamAtmosphereReplaysAfterLastEventID(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	a.broadcast(Command{Kind: "config", Tick: 10, Data: mustJSON(t, sim.Config{Wind: 1})})
	a.broadcast(Command{Kind: "trigger", Tick: 11, Event: "gust"})
	a.broadcast(Command{Kind: "clock", Tick: 12, Data: mustJSON(t, clockData{Tick: 12})})

	body := streamAtmosphereOnce(t, a, "1")
	if strings.Contains(body, `"kind":"snapshot"`) {
		t.Fatalf("stream replay included fresh snapshot:\n%s", body)
	}
	for _, want := range []string{"id: 2", `"kind":"trigger"`, `"event":"gust"`, "id: 3", `"kind":"clock"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body missing %q:\n%s", want, body)
		}
	}
}

func TestStreamAtmosphereFlushesEmptyReplay(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	a.broadcast(Command{Kind: "metric", Tick: 1})

	body := streamAtmosphereOnce(t, a, "1")
	if !strings.Contains(body, ": replay-current") {
		t.Fatalf("stream body missing replay-current comment:\n%s", body)
	}
	if strings.Contains(body, `"kind":"snapshot"`) {
		t.Fatalf("current replay included fresh snapshot:\n%s", body)
	}
}

func TestStreamAtmosphereFallsBackToSnapshotForStaleLastEventID(t *testing.T) {
	a := newAtmosphereWithEffectAndSeed("rain", 1)
	for i := 0; i < authorityReplayBufferSize+2; i++ {
		a.broadcast(Command{Kind: "metric", Tick: i})
	}

	body := streamAtmosphereOnce(t, a, "1")
	if !strings.Contains(body, `"kind":"snapshot"`) {
		t.Fatalf("stream body missing fresh snapshot:\n%s", body)
	}
	if !strings.Contains(body, "id: "+strconv.Itoa(authorityReplayBufferSize+2)) {
		t.Fatalf("snapshot frame did not use current command ID:\n%s", body)
	}
}

func streamAtmosphereOnce(t *testing.T, a *atmosphere, lastID string) string {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	rec := httptest.NewRecorder()

	streamAtmosphere(rec, req, a)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec.Body.String()
}
