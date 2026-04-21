package sim

import "testing"

func TestNewDustAppliesDefaults(t *testing.T) {
	d := NewDust(24, 14, 1, DustConfig{})
	cfg := d.EffectiveConfig()
	if cfg.Drift == 0 {
		t.Fatal("expected default drift")
	}
	if cfg.SpawnEvery <= 0 || cfg.MaxDust <= 0 {
		t.Fatal("expected default dust density config")
	}
	if cfg.GustDur <= 0 || cfg.GustFront <= 0 {
		t.Fatal("expected default gust config")
	}
}

func TestDustStepPaintsGrid(t *testing.T) {
	d := NewDust(40, 20, 1, DustConfig{
		SpawnEvery: 1,
		SpawnBurst: 2,
		MaxDust:    16,
		GustChance: 0,
		CalmChance: 0,
	})
	for i := 0; i < 8; i++ {
		d.Step()
	}

	filled := 0
	for y := range d.Grid {
		for x := range d.Grid[y] {
			if d.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected dust step to paint at least one pixel")
	}
}

func TestDustSnapshotRestoreRoundTrip(t *testing.T) {
	d := NewDust(48, 24, 42, DustConfig{
		SpawnEvery: 1,
		SpawnBurst: 2,
		MaxDust:    18,
		GustChance: 0,
		CalmChance: 0,
	})
	for i := 0; i < 6; i++ {
		d.Step()
	}
	if !d.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	d.Step()
	snap := d.Snapshot()

	restored := NewDust(48, 24, 9, DustConfig{})
	restored.SetConfig(d.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	got := restored.Snapshot()

	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.GustTicks != snap.GustTicks {
		t.Fatalf("gust ticks = %d, want %d", got.GustTicks, snap.GustTicks)
	}
	if got.IntroTicks != snap.IntroTicks {
		t.Fatalf("intro ticks = %d, want %d", got.IntroTicks, snap.IntroTicks)
	}
	if len(got.Motes) != len(snap.Motes) {
		t.Fatalf("mote count = %d, want %d", len(got.Motes), len(snap.Motes))
	}
}
