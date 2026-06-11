package sim

import "testing"

func TestPaperLanternsSchemaContainsTriggers(t *testing.T) {
	schema := PaperLanternsSchema()
	if schema.Name != "paper-lanterns" {
		t.Fatalf("schema name = %q, want paper-lanterns", schema.Name)
	}
	want := map[string]bool{
		"lantern-emit":  true,
		"release-pulse": true,
		"wind-drift":    true,
		"lantern-fade":  true,
		"quiet-gap":     true,
		"intro":         true,
		"ending":        true,
	}
	for _, knob := range schema.Knobs {
		if knob.Trigger != "" {
			delete(want, knob.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func TestPaperLanternsReleasePulseQueuesCluster(t *testing.T) {
	p := NewPaperLanterns(80, 48, 4, PaperLanternsConfig{
		ClusterMin:     5,
		ClusterMax:     5,
		ReleaseSpacing: 1,
		MaxLanterns:    5,
		LoneEmitChance: 0,
		ReleaseChance:  0,
		QuietChance:    0,
	})
	if !p.TriggerEvent("release-pulse") {
		t.Fatal("expected release-pulse trigger")
	}
	for i := 0; i < 12; i++ {
		p.Step()
	}
	if got := len(p.LanternsCopy()); got != 5 {
		t.Fatalf("lantern count = %d, want 5", got)
	}
}

func TestPaperLanternsSnapshotRoundTrip(t *testing.T) {
	p := NewPaperLanterns(80, 48, 9, PaperLanternsConfig{
		ClusterMin:     4,
		ClusterMax:     4,
		ReleaseSpacing: 1,
		LoneEmitChance: 0,
		ReleaseChance:  0,
		QuietChance:    0,
	})
	p.TriggerEvent("release-pulse")
	p.TriggerEvent("wind-drift")
	for i := 0; i < 8; i++ {
		p.Step()
	}
	snap := p.Snapshot()

	restored := NewPaperLanterns(20, 10, 1, PaperLanternsConfig{})
	restored.SetConfig(p.EffectiveConfig())
	restored.Resize(80, 48)
	restored.RestoreSnapshot(snap)

	got := restored.Snapshot()
	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.WindBiasTicks != snap.WindBiasTicks {
		t.Fatalf("windBiasTicks = %d, want %d", got.WindBiasTicks, snap.WindBiasTicks)
	}
	if len(got.Lanterns) != len(snap.Lanterns) {
		t.Fatalf("lantern count = %d, want %d", len(got.Lanterns), len(snap.Lanterns))
	}
}

func TestPaperLanternsEndingHoldsDarkSky(t *testing.T) {
	p := NewPaperLanterns(80, 48, 12, PaperLanternsConfig{
		ClusterMin:     6,
		ClusterMax:     6,
		ReleaseSpacing: 1,
		RiseSpeed:      0.18,
		EndingTail:     40,
		LoneEmitChance: 0,
		ReleaseChance:  0,
		QuietChance:    0,
	})
	p.TriggerEvent("release-pulse")
	for i := 0; i < 10; i++ {
		p.Step()
	}
	if len(p.LanternsCopy()) == 0 {
		t.Fatal("expected lanterns before ending")
	}
	if !p.TriggerEvent("ending") {
		t.Fatal("expected ending trigger")
	}
	for i := 0; i < 120; i++ {
		p.Step()
	}
	state := p.SnapshotState()
	if state.Lifecycle != paperLanternsLifecycleEnded {
		t.Fatalf("lifecycle = %q, want %q", state.Lifecycle, paperLanternsLifecycleEnded)
	}
	if got := len(p.LanternsCopy()); got != 0 {
		t.Fatalf("lanterns after ending = %d, want 0", got)
	}
	if bright := countPaperLanternBrightPixels(p.GridCopy()); bright != 0 {
		t.Fatalf("bright pixels after ending = %d, want dark sky", bright)
	}
}

func TestPaperLanternsPaintsReadableLanterns(t *testing.T) {
	p := NewPaperLanterns(96, 54, 15, PaperLanternsConfig{
		ClusterMin:     8,
		ClusterMax:     8,
		ReleaseSpacing: 1,
		LoneEmitChance: 0,
		ReleaseChance:  0,
		QuietChance:    0,
	})
	p.TriggerEvent("release-pulse")
	for i := 0; i < 30; i++ {
		p.Step()
	}
	if bright := countPaperLanternBrightPixels(p.GridCopy()); bright < 8 {
		t.Fatalf("warm bright lantern pixels = %d, want >= 8", bright)
	}
}

func countPaperLanternBrightPixels(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, px := range row {
			if px.Filled && px.C.R > 115 && px.C.G > 65 && px.C.B < 100 {
				count++
			}
		}
	}
	return count
}
