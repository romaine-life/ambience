package sim

import "testing"

func TestNewSandAppliesDefaults(t *testing.T) {
	s := NewSand(80, 50, 1, SandConfig{})
	cfg := s.EffectiveConfig()
	if cfg.PipeWidth <= 0 || cfg.StreamSpread <= 0 {
		t.Fatal("expected default pipe / stream spread")
	}
	if cfg.ContainerY <= 0 || cfg.ContainerSpan <= 0 || cfg.ContainerDepth <= 0 {
		t.Fatal("expected default container geometry")
	}
	if cfg.EmitRate <= 0 || cfg.Repose <= 0 || cfg.SettlePerTick <= 0 {
		t.Fatal("expected default flow / settle params")
	}
}

func TestSandStepGrowsPile(t *testing.T) {
	s := NewSand(96, 56, 1, SandConfig{
		EmitRate:    2.4,
		SurgeChance: 0,
		CalmChance:  0,
		MaxGrains:   160,
	})
	for i := 0; i < 200; i++ {
		s.Step()
	}
	state := s.SnapshotState()
	maxH := 0.0
	for _, h := range state.Pile {
		if h > maxH {
			maxH = h
		}
	}
	if maxH <= 0 {
		t.Fatalf("expected pile to accumulate after steady pour, got max=%.2f", maxH)
	}
	filled := 0
	for y := range s.Grid {
		for x := range s.Grid[y] {
			if s.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected sand step to paint pixels")
	}
}

func TestSandTriggerEventsAreRecognized(t *testing.T) {
	s := NewSand(64, 48, 3, SandConfig{})
	for _, name := range []string{"surge", "calm", "intro", "ending"} {
		if !s.TriggerEvent(name) {
			t.Fatalf("expected sand to recognize %q", name)
		}
	}
	if s.TriggerEvent("nope") {
		t.Fatal("expected unknown event to be rejected")
	}
}

func TestSandSnapshotRoundTrip(t *testing.T) {
	a := NewSand(72, 48, 9, SandConfig{EmitRate: 1.6})
	for i := 0; i < 60; i++ {
		a.Step()
	}
	snap := a.Snapshot()
	b := NewSand(72, 48, 9, SandConfig{})
	b.RestoreSnapshot(snap)
	if b.SnapshotState().Tick != a.SnapshotState().Tick {
		t.Fatal("expected restored sand to match tick")
	}
	if len(b.SnapshotState().Pile) != len(a.SnapshotState().Pile) {
		t.Fatal("expected restored pile length to match")
	}
}
