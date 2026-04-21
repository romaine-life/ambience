package sim

import (
	"math"
	"testing"
)

func TestNewWaterfallAppliesDefaults(t *testing.T) {
	w := NewWaterfall(20, 12, 1, WaterfallConfig{})
	cfg := w.EffectiveConfig()
	if cfg.Width <= 0 {
		t.Fatal("expected default sheet width")
	}
	if cfg.PoolY <= 0 || cfg.PoolSpan <= 0 {
		t.Fatal("expected default pool geometry")
	}
	if cfg.MistSpawnEvery <= 0 || cfg.RippleEvery <= 0 {
		t.Fatal("expected default accent cadence")
	}
}

func TestWaterfallStepPaintsGrid(t *testing.T) {
	w := NewWaterfall(48, 28, 1, WaterfallConfig{
		MistSpawnEvery:  1,
		MaxMist:         8,
		RippleEvery:     1,
		MaxRipples:      4,
		SurgeChance:     0,
		CalmChance:      0,
		MistBurstChance: 0,
	})
	for i := 0; i < 6; i++ {
		w.Step()
	}

	filled := 0
	for y := range w.Grid {
		for x := range w.Grid[y] {
			if w.Grid[y][x].Filled {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("expected waterfall step to paint at least one pixel")
	}
}

func TestWaterfallSnapshotRestoreRoundTrip(t *testing.T) {
	w := NewWaterfall(48, 30, 42, WaterfallConfig{
		MistSpawnEvery:  1,
		MaxMist:         12,
		RippleEvery:     1,
		MaxRipples:      6,
		SurgeChance:     0,
		CalmChance:      0,
		MistBurstChance: 0,
	})
	for i := 0; i < 8; i++ {
		w.Step()
	}
	if !w.TriggerEvent("surge") {
		t.Fatal("expected surge trigger to succeed")
	}
	if !w.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	snap := w.Snapshot()

	restored := NewWaterfall(48, 30, 9, WaterfallConfig{})
	restored.SetConfig(w.EffectiveConfig())
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
	if len(got.Mists) != len(snap.Mists) {
		t.Fatalf("mist count = %d, want %d", len(got.Mists), len(snap.Mists))
	}
	if len(got.Ripples) != len(snap.Ripples) {
		t.Fatalf("ripple count = %d, want %d", len(got.Ripples), len(snap.Ripples))
	}
}

func TestWaterfallSheetWobbleDriftsDownward(t *testing.T) {
	w := NewWaterfall(72, 36, 1, WaterfallConfig{
		Width:           6,
		Wobble:          4.5,
		Speed:           1,
		PoolY:           0.8,
		MistSpawnEvery:  99,
		MaxMist:         1,
		RippleEvery:     99,
		MaxRipples:      1,
		SurgeChance:     0,
		CalmChance:      0,
		MistBurstChance: 0,
	})

	before := waterfallSheetCentersAtTick(w, 0)
	after := waterfallSheetCentersAtTick(w, 15)
	shift := bestWaterfallCenterShift(before, after, 6)
	if shift <= 0 {
		t.Fatalf("sheet wobble shift = %d, want downward drift", shift)
	}
}

func waterfallSheetCentersAtTick(w *Waterfall, tick int) []float64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tick = tick
	w.clearGridLocked()
	w.paintSheetLocked()

	surface := w.poolRowLocked()
	centers := make([]float64, surface)
	for y := 0; y < surface; y++ {
		sum := 0.0
		count := 0.0
		for x := range w.Grid[y] {
			if !w.Grid[y][x].Filled {
				continue
			}
			sum += float64(x)
			count++
		}
		if count == 0 {
			centers[y] = math.NaN()
			continue
		}
		centers[y] = sum / count
	}
	return centers
}

func bestWaterfallCenterShift(before, after []float64, maxShift int) int {
	bestShift := 0
	bestScore := math.Inf(1)
	for shift := -maxShift; shift <= maxShift; shift++ {
		score := 0.0
		pairs := 0
		for y := range before {
			y2 := y + shift
			if y2 < 0 || y2 >= len(after) {
				continue
			}
			if math.IsNaN(before[y]) || math.IsNaN(after[y2]) {
				continue
			}
			diff := before[y] - after[y2]
			score += diff * diff
			pairs++
		}
		if pairs == 0 {
			continue
		}
		score /= float64(pairs)
		if score < bestScore {
			bestScore = score
			bestShift = shift
		}
	}
	return bestShift
}
