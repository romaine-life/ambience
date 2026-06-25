package main

import "testing"

// TestRainJoinsFresh pins the per-effect join contract: rain is steady-state, so
// it declares FreshJoin (clients start it from its intro, aligned to the
// playback buffer — eases in with no freeze). Every other registered effect
// defaults to "restore" (replay the snapshot as-is, hold it through the buffer)
// because its current frame is its accumulated history. The jitter buffer itself
// is shared — joinMode only changes how the client enters it.
func TestRainJoinsFresh(t *testing.T) {
	if got := effectJoinMode("rain"); got != "fresh" {
		t.Fatalf("rain joinMode = %q, want \"fresh\"", got)
	}
	if playbackBufferTicks <= 0 {
		t.Fatalf("playback buffer = %d ticks, want a positive jitter buffer for smooth playback", playbackBufferTicks)
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
