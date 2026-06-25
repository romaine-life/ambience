package main

import "testing"

// TestRainJoinsFresh pins the per-effect join contract: rain is steady-state, so
// it declares FreshJoin (clients start it from its intro at the live edge — no
// freeze, delay 0). Every other registered effect defaults to "restore" (replay
// the snapshot as-is, keep the playback delay) because its current frame is its
// accumulated history.
func TestRainJoinsFresh(t *testing.T) {
	if got := effectJoinMode("rain"); got != "fresh" {
		t.Fatalf("rain joinMode = %q, want \"fresh\"", got)
	}
	if got := playbackDelayTicksFor("rain"); got != 0 {
		t.Fatalf("rain playback delay = %d ticks, want 0 (live edge)", got)
	}
	if restorePlaybackDelayTicks <= 0 {
		t.Fatalf("restore playback delay = %d, want a positive delay for event sync", restorePlaybackDelayTicks)
	}

	for effectType := range effectRegistry {
		if effectType == "rain" {
			continue
		}
		if mode := effectJoinMode(effectType); mode != "restore" {
			t.Errorf("effect %q joinMode = %q, want \"restore\" (only rain opts into fresh today)", effectType, mode)
		}
		if delay := playbackDelayTicksFor(effectType); delay != restorePlaybackDelayTicks {
			t.Errorf("effect %q playback delay = %d, want restore default %d", effectType, delay, restorePlaybackDelayTicks)
		}
	}

	// Unknown effects fall back to the safe restore default.
	if got := effectJoinMode("does-not-exist"); got != "restore" {
		t.Fatalf("unknown effect joinMode = %q, want \"restore\"", got)
	}
}
