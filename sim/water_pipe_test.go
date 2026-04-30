package sim

import "testing"

func TestNewWaterPipeAppliesDefaults(t *testing.T) {
	p := NewWaterPipe(80, 50, 1, WaterPipeConfig{})
	cfg := p.EffectiveConfig()
	if cfg.PipeWidth <= 0 || cfg.StreamWidth <= 0 {
		t.Fatal("expected default pipe / stream widths")
	}
	if cfg.BasinY <= 0 || cfg.BasinSpan <= 0 || cfg.BasinDepth <= 0 {
		t.Fatal("expected default basin geometry")
	}
	if cfg.SurgeMult <= 0 || cfg.DryMult < 0 {
		t.Fatal("expected default event multipliers")
	}
}

func TestWaterPipeStepPaintsGridAndFills(t *testing.T) {
	p := NewWaterPipe(96, 56, 1, WaterPipeConfig{
		Inflow:      2.0,
		BasinDepth:  6,
		SurgeChance: 0,
		DryUpChance: 0,
	})
	for i := 0; i < 30; i++ {
		p.Step()
	}
	filled := 0
	for y := range p.Grid {
		for x := range p.Grid[y] {
			if p.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected water-pipe step to paint pixels")
	}
	if p.SnapshotState().Fill <= 0 {
		t.Fatal("expected basin fill to grow under steady inflow")
	}
}

func TestWaterPipeOverflowSpawnsRunoff(t *testing.T) {
	p := NewWaterPipe(96, 56, 7, WaterPipeConfig{
		Inflow:      3,
		BasinDepth:  4,
		SurgeChance: 0,
		DryUpChance: 0,
	})
	p.fill = 1.2
	saw := false
	for i := 0; i < 60; i++ {
		p.Step()
		if len(p.runoff) > 0 {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatal("expected overflow to spawn runoff streams")
	}
}

func TestWaterPipeSnapshotRestoreRoundTrip(t *testing.T) {
	p := NewWaterPipe(80, 48, 42, WaterPipeConfig{
		Inflow:      1.5,
		SurgeChance: 0,
		DryUpChance: 0,
	})
	for i := 0; i < 12; i++ {
		p.Step()
	}
	if !p.TriggerEvent("surge") {
		t.Fatal("expected surge trigger to succeed")
	}
	if !p.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	snap := p.Snapshot()

	restored := NewWaterPipe(80, 48, 9, WaterPipeConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	got := restored.Snapshot()

	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.SurgeTicks != snap.SurgeTicks {
		t.Fatalf("surge ticks = %d, want %d", got.SurgeTicks, snap.SurgeTicks)
	}
	if got.IntroTicks != snap.IntroTicks {
		t.Fatalf("intro ticks = %d, want %d", got.IntroTicks, snap.IntroTicks)
	}
	if got.Fill != snap.Fill {
		t.Fatalf("fill = %v, want %v", got.Fill, snap.Fill)
	}
	if len(got.Droplets) != len(snap.Droplets) {
		t.Fatalf("droplet count = %d, want %d", len(got.Droplets), len(snap.Droplets))
	}
	if len(got.Ripples) != len(snap.Ripples) {
		t.Fatalf("ripple count = %d, want %d", len(got.Ripples), len(snap.Ripples))
	}
	if len(got.Runoff) != len(snap.Runoff) {
		t.Fatalf("runoff count = %d, want %d", len(got.Runoff), len(snap.Runoff))
	}
}

func TestWaterPipeDryUpEventReducesFlow(t *testing.T) {
	p := NewWaterPipe(80, 48, 3, WaterPipeConfig{
		Inflow:      1,
		SurgeChance: 0,
		DryUpChance: 0,
		DryMult:     0.1,
		DryDur:      40,
	})
	p.Step()
	baseline := p.flowLevelLocked()
	if !p.TriggerEvent("dry-up") {
		t.Fatal("expected dry-up trigger to succeed")
	}
	p.Step()
	dried := p.flowLevelLocked()
	if dried >= baseline {
		t.Fatalf("expected dry-up to reduce flow: baseline=%v dried=%v", baseline, dried)
	}
}
