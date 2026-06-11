package main

import (
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

func TestDevObserveWaitsForTriggeredStatePredicate(t *testing.T) {
	s, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	if err := s.applyConfig(json.RawMessage(`{"ending_dur":2,"ending_linger":1,"ending_splashes":0}`)); err != nil {
		t.Fatalf("applyConfig: %v", err)
	}

	got, err := s.observe(context.Background(), devObserveRequest{
		Effect:    "rain",
		Session:   "observe-test",
		Trigger:   "ending",
		Lifecycle: string(sim.LifecycleRunning),
		MaxTicks:  60,
		HoldTicks: 2,
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !got.Applied || !got.Observed {
		t.Fatalf("applied/observed = %v/%v, want true/true", got.Applied, got.Observed)
	}
	if got.ObservedTick <= got.StartTick {
		t.Fatalf("observed tick = %d, start tick = %d", got.ObservedTick, got.StartTick)
	}
	if got.HeldUntilTick < got.ObservedTick+got.HoldTicks {
		t.Fatalf("held until tick = %d, observed tick = %d hold = %d", got.HeldUntilTick, got.ObservedTick, got.HoldTicks)
	}
	if got.ObservationID == "" || got.FrameURL == "" {
		t.Fatalf("missing frozen frame refs: id=%q url=%q", got.ObservationID, got.FrameURL)
	}
	if !hasAppliedEvent(got.AppliedEvents, "ending", got.StartTick) {
		t.Fatalf("ending not present in applied events: %+v", got.AppliedEvents)
	}
	if !hasAppliedEvent(got.MatchedEvents, "ending", got.StartTick) {
		t.Fatalf("ending not present in matched events: %+v", got.MatchedEvents)
	}
	if frame, ok := s.observationFrame(got.ObservationID); !ok || len(frame) == 0 {
		t.Fatalf("stored observation frame ok=%v rows=%d", ok, len(frame))
	}
}

func TestDevObserveTimesOutForUnreachedLifecycle(t *testing.T) {
	s, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	_, err = s.observe(context.Background(), devObserveRequest{
		Effect:    "rain",
		Session:   "observe-test",
		Lifecycle: string(sim.LifecycleEnded),
		MaxTicks:  3,
	})
	if err == nil {
		t.Fatal("observe unexpectedly succeeded")
	}
}

func TestServeDevSessionFrameReturnsStoredObservationPNG(t *testing.T) {
	session := "observe-frame-test"
	key := devSessionKey("rain", session, gridW, gridH)
	devSessions.Delete(key)
	defer devSessions.Delete(key)

	s, err := newDevSession("rain")
	if err != nil {
		t.Fatalf("newDevSession: %v", err)
	}
	s.observed = append(s.observed, devObservation{
		ID:    "frame1",
		Tick:  1,
		Frame: s.effect.Frame(),
	})
	devSessions.Store(key, s)

	req := httptest.NewRequest(http.MethodGet, "/dev/frame?session="+session+"&effect=rain&observation=frame1", nil)
	rec := httptest.NewRecorder()
	serveDevSessionFrame(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q, want image/png", got)
	}
	if _, err := png.Decode(rec.Body); err != nil {
		t.Fatalf("decode png: %v", err)
	}
}
