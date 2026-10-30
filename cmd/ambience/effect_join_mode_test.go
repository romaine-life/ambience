package main

import "testing"

// TestRainJoinsFresh pins the per-effect join contract: rain is steady-state, so
// it declares FreshJoin (clients start it from its intro instead of jolting on
// mid-storm). Every other registered effect defaults to "restore" (replay the
// snapshot as-is) because its current frame is its accumulated history.
func TestRainJoinsFresh(t *testing.T) {
	if got := effectJoinMode("rain"); got != "fresh" {
		t.Fatalf("rain joinMode = %q, want \"fresh\"", got)
	}
	for effectType := range effectRegistry {
		if effectType == "rain" {
			continue
		}
		if mode := effectJoinMode(effectType); mode != "restore" {
			t.Errorf("effect %q joinMode = %q, want \"restore\" (only rain opts into fresh today)", effectType, mode)
		}
	}

	// Unknown effects fall back to the safe restore default.
	if got := effectJoinMode("does-not-exist"); got != "restore" {
		t.Fatalf("unknown effect joinMode = %q, want \"restore\"", got)
	}
}

// TestPlaybackDelayIsPositive keeps a command buffer for every effect: clients
// render this far behind the world tick so commands land on the same tick
// everywhere.
func TestPlaybackDelayIsPositive(t *testing.T) {
	if playbackDelayTicks <= 0 {
		t.Fatalf("playback delay = %d, want a positive delay for event sync", playbackDelayTicks)
	}
}
