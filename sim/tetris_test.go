package sim

import (
	"encoding/json"
	"testing"
)

func TestNewTetrisAppliesDefaults(t *testing.T) {
	tt := NewTetris(160, 80, 1, TetrisConfig{})
	cfg := tt.EffectiveConfig()
	if cfg.BoardW <= 0 || cfg.BoardH <= 0 {
		t.Fatal("expected default board dimensions")
	}
	if cfg.FallEvery <= 0 || cfg.SpawnPause <= 0 {
		t.Fatal("expected default cadence config")
	}
	if cfg.IntroDur <= 0 || cfg.EndingDur <= 0 {
		t.Fatal("expected default intro/ending dur")
	}
}

func TestTetrisStepEventuallySpawnsAndSettles(t *testing.T) {
	tt := NewTetris(160, 80, 42, TetrisConfig{
		FallEvery:   1,
		SpawnPause:  1,
		LockDelay:   1,
		IntroFirst:  1,
		LullChance:  0,
		IntroHeight: 0,
	})
	settled := false
	for i := 0; i < 600 && !settled; i++ {
		tt.Step()
		snap := tt.Snapshot()
		for _, cell := range snap.Cells {
			if cell != 0 {
				settled = true
				break
			}
		}
	}
	if !settled {
		t.Fatal("expected at least one piece to settle into the board after 600 steps")
	}
}

func TestTetrisLoseStateTriggersEnding(t *testing.T) {
	tt := NewTetris(160, 80, 7, TetrisConfig{
		FallEvery:     1,
		SpawnPause:    0,
		LockDelay:     0,
		IntroFirst:    0,
		FillThreshold: 0.05, // very low — first piece settling tips it
	})
	for i := 0; i < 800; i++ {
		tt.Step()
		snap := tt.Snapshot()
		if snap.EndingTicks > 0 {
			return
		}
	}
	t.Fatal("expected lose-state ending within 800 ticks at fill_thresh=0.05")
}

func TestTetrisSnapshotRoundTrip(t *testing.T) {
	tt := NewTetris(160, 80, 1234, TetrisConfig{
		FallEvery:  1,
		SpawnPause: 0,
		LockDelay:  0,
		IntroFirst: 0,
	})
	for i := 0; i < 80; i++ {
		tt.Step()
	}
	snap := tt.Snapshot()

	// Verify JSON round-trip — important because the snapshot crosses the
	// SSE boundary and gets parsed by the browser.
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded TetrisSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	restored := NewTetris(160, 80, 0, TetrisConfig{})
	restored.SetConfig(tt.EffectiveConfig())
	restored.RestoreSnapshot(decoded)
	got := restored.Snapshot()

	if got.Tick != snap.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, snap.Tick)
	}
	if got.PieceIndex != snap.PieceIndex {
		t.Fatalf("pieceIndex = %d, want %d", got.PieceIndex, snap.PieceIndex)
	}
	if got.NextKind != snap.NextKind {
		t.Fatalf("nextKind = %d, want %d", got.NextKind, snap.NextKind)
	}
	if got.RNGState != snap.RNGState {
		t.Fatalf("rngState = %d, want %d", got.RNGState, snap.RNGState)
	}
	if len(got.Cells) != len(snap.Cells) {
		t.Fatalf("cells len = %d, want %d", len(got.Cells), len(snap.Cells))
	}
	for i := range got.Cells {
		if got.Cells[i] != snap.Cells[i] {
			t.Fatalf("cell[%d] = %d, want %d", i, got.Cells[i], snap.Cells[i])
		}
	}
	if (got.Active == nil) != (snap.Active == nil) {
		t.Fatalf("active mismatch: got=%v want=%v", got.Active, snap.Active)
	}
	if got.Active != nil && snap.Active != nil {
		if got.Active.Kind != snap.Active.Kind ||
			got.Active.Row != snap.Active.Row ||
			got.Active.Col != snap.Active.Col ||
			got.Active.Rotation != snap.Active.Rotation {
			t.Fatalf("active mismatch: got=%+v want=%+v", *got.Active, *snap.Active)
		}
	}
}

func TestTetrisTriggerEvents(t *testing.T) {
	tt := NewTetris(160, 80, 1, TetrisConfig{})
	if !tt.TriggerEvent("intro") {
		t.Fatal("expected intro trigger to succeed")
	}
	if !tt.TriggerEvent("lull") {
		t.Fatal("expected lull trigger to succeed")
	}
	if !tt.TriggerEvent("new-piece") {
		t.Fatal("expected new-piece trigger to succeed")
	}
	if !tt.TriggerEvent("ending") {
		t.Fatal("expected ending trigger to succeed")
	}
	if tt.TriggerEvent("does-not-exist") {
		t.Fatal("expected unknown trigger to fail")
	}
	logs := tt.DrainLog()
	if len(logs) == 0 {
		t.Fatal("expected trigger events to produce log entries")
	}
}
