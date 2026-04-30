package sim

import "testing"

func TestNewSandAppliesDefaults(t *testing.T) {
	s := NewSand(40, 30, 1, SandConfig{})
	cfg := s.EffectiveConfig()
	if cfg.Flow <= 0 {
		t.Fatal("expected default flow")
	}
	if cfg.Gravity <= 0 || cfg.PipeW <= 0 || cfg.BinW <= 0 || cfg.BinH <= 0 {
		t.Fatal("expected default pipe/bin geometry")
	}
	if cfg.Talus <= 0 {
		t.Fatal("expected default talus angle")
	}
	if cfg.SurgeDur <= 0 || cfg.CalmDur <= 0 {
		t.Fatal("expected default surge/calm durations")
	}
}

func TestSandStepEmitsAndPaints(t *testing.T) {
	s := NewSand(80, 60, 7, SandConfig{
		Flow:        2.5,
		PipeW:       4,
		BinW:        40,
		BinH:        20,
		SurgeChance: 0,
		CalmChance:  0,
	})
	for i := 0; i < 30; i++ {
		s.Step()
	}
	if len(s.grains) == 0 {
		t.Fatal("expected at least one in-flight grain after 30 ticks of pour")
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
		t.Fatal("expected sand step to paint at least one pixel (pipe + bin)")
	}
}

func TestSandPileSettlesUnderPipe(t *testing.T) {
	s := NewSand(80, 60, 17, SandConfig{
		Flow:        3,
		PipeW:       4,
		BinW:        50,
		BinH:        24,
		Gravity:     0.12,
		SurgeChance: 0,
		CalmChance:  0,
	})
	for i := 0; i < 200; i++ {
		s.Step()
	}
	total := 0
	for _, h := range s.pile {
		total += h
	}
	if total < 50 {
		t.Fatalf("expected sand to accumulate in the pile after 200 ticks, got total height %d", total)
	}
}

func TestSandSnapshotRoundTrip(t *testing.T) {
	s := NewSand(64, 40, 11, SandConfig{
		Flow:        2,
		BinW:        40,
		BinH:        20,
		SurgeChance: 0,
		CalmChance:  0,
	})
	for i := 0; i < 40; i++ {
		s.Step()
	}
	if !s.TriggerEvent("surge") {
		t.Fatal("expected surge trigger to succeed")
	}
	s.Step()
	snap := s.Snapshot()

	restored := NewSand(64, 40, 99, SandConfig{})
	restored.SetConfig(s.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	got := restored.Snapshot()

	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.SurgeTicks != snap.SurgeTicks {
		t.Fatalf("surge ticks = %d, want %d", got.SurgeTicks, snap.SurgeTicks)
	}
	if len(got.Grains) != len(snap.Grains) {
		t.Fatalf("grain count = %d, want %d", len(got.Grains), len(snap.Grains))
	}
	if len(got.Pile) != len(snap.Pile) {
		t.Fatalf("pile len = %d, want %d", len(got.Pile), len(snap.Pile))
	}
	for i := range got.Pile {
		if got.Pile[i] != snap.Pile[i] {
			t.Fatalf("pile[%d] = %d, want %d", i, got.Pile[i], snap.Pile[i])
		}
	}
}

func TestSandTriggerIntroResetsPile(t *testing.T) {
	s := NewSand(64, 40, 5, SandConfig{
		Flow: 4,
		BinW: 40,
		BinH: 20,
	})
	for i := 0; i < 100; i++ {
		s.Step()
	}
	preTotal := 0
	for _, h := range s.pile {
		preTotal += h
	}
	if preTotal == 0 {
		t.Fatal("setup: expected some pile before intro retrigger")
	}
	if !s.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	postTotal := 0
	for _, h := range s.pile {
		postTotal += h
	}
	if postTotal != 0 {
		t.Fatalf("intro should clear the pile (intro_pile=0), got total %d", postTotal)
	}
	if s.introTicks != s.introTotal || s.introTotal <= 0 {
		t.Fatalf("intro ticks not initialized: ticks=%d total=%d", s.introTicks, s.introTotal)
	}
}
