package sim

import "testing"

func TestPaperLanternsSchemaDeclaresEvents(t *testing.T) {
	schema := PaperLanternsSchema()
	if schema.Name != "paper-lanterns" {
		t.Fatalf("schema name = %q, want paper-lanterns", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("paper-lanterns ending should hold a terminal dark sky")
	}
	want := map[string]bool{
		PaperLanternsEventLanternEmit:  false,
		PaperLanternsEventReleasePulse: false,
		PaperLanternsEventWindDrift:    false,
		PaperLanternsEventLanternFade:  false,
		PaperLanternsEventQuietGap:     false,
		PaperLanternsEventIntro:        false,
		PaperLanternsEventEnding:       false,
	}
	for _, k := range schema.Knobs {
		if _, ok := want[k.Trigger]; ok {
			want[k.Trigger] = true
		}
	}
	for trigger, seen := range want {
		if !seen {
			t.Fatalf("schema missing trigger %q", trigger)
		}
	}
}

func TestPaperLanternsReleasePulsePaintsWarmLanterns(t *testing.T) {
	p := NewPaperLanterns(64, 36, 7, PaperLanternsConfig{
		ClusterMin:    6,
		ClusterMax:    6,
		PulseWindow:   12,
		RiseSpeed:     0.18,
		ReleaseChance: 0.0001,
	})
	if !p.TriggerEvent(PaperLanternsEventReleasePulse) {
		t.Fatal("release-pulse trigger rejected")
	}
	for i := 0; i < 42; i++ {
		p.Step()
	}
	if got := len(p.LanternsCopy()); got < 6 {
		t.Fatalf("lantern count = %d, want at least 6", got)
	}
	if got := countWarmLanternPixels(p.GridCopy()); got < 20 {
		t.Fatalf("warm lantern pixels = %d, want visible paper lanterns", got)
	}
}

func TestPaperLanternsSnapshotRestoreRoundTrip(t *testing.T) {
	p := NewPaperLanterns(64, 36, 11, PaperLanternsConfig{
		SpawnEvery:    12,
		ClusterMin:    5,
		ClusterMax:    5,
		PulseWindow:   10,
		RiseSpeed:     0.16,
		ReleaseChance: 0.0001,
	})
	p.TriggerEvent(PaperLanternsEventReleasePulse)
	p.TriggerEvent(PaperLanternsEventWindDrift)
	for i := 0; i < 28; i++ {
		p.Step()
	}
	snap := p.Snapshot()

	restored := NewPaperLanterns(64, 36, 99, PaperLanternsConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.RestoreSnapshot(snap)
	got := restored.Snapshot()
	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if len(got.Lanterns) != len(snap.Lanterns) {
		t.Fatalf("lantern count = %d, want %d", len(got.Lanterns), len(snap.Lanterns))
	}
	if got.WindDriftTicks != snap.WindDriftTicks || got.WindBias != snap.WindBias {
		t.Fatalf("wind state = (%d, %.3f), want (%d, %.3f)", got.WindDriftTicks, got.WindBias, snap.WindDriftTicks, snap.WindBias)
	}
}

func TestPaperLanternsEndingHoldsDarkSky(t *testing.T) {
	p := NewPaperLanterns(64, 36, 13, PaperLanternsConfig{
		ClusterMin:    6,
		ClusterMax:    6,
		PulseWindow:   8,
		RiseSpeed:     0.22,
		EndingTail:    24,
		ReleaseChance: 0.0001,
	})
	p.TriggerEvent(PaperLanternsEventReleasePulse)
	for i := 0; i < 20; i++ {
		p.Step()
	}
	if countWarmLanternPixels(p.GridCopy()) == 0 {
		t.Fatal("expected visible lanterns before ending")
	}
	if !p.TriggerEvent(PaperLanternsEventEnding) {
		t.Fatal("ending trigger rejected")
	}
	if got := p.SnapshotState().Lifecycle; got != LifecycleEnding {
		t.Fatalf("after ending lifecycle = %q, want ending", got)
	}
	for i := 0; i < 32; i++ {
		p.Step()
	}
	if got := p.SnapshotState().Lifecycle; got != LifecycleEnded {
		t.Fatalf("resolved lifecycle = %q, want ended", got)
	}
	if got := countWarmLanternPixels(p.GridCopy()); got != 0 {
		t.Fatalf("warm lantern pixels after terminal ending = %d, want 0", got)
	}
	for i := 0; i < 10; i++ {
		p.Step()
	}
	if got := p.SnapshotState().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle after extra steps = %q, want ended", got)
	}
}

func countWarmLanternPixels(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled && p.C.R > 70 && p.C.G > 35 && int(p.C.R) > int(p.C.B)*2 {
				count++
			}
		}
	}
	return count
}
