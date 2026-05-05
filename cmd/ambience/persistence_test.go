package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nelsong6/ambience/sim"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	want := persistedAtmosphere{
		Version:       1,
		Type:          "rain",
		Seed:          123,
		SceneRNGState: 456,
		Effect: persistedEffectState{
			GridW: 160,
			GridH: 80,
		},
		CommandSeq:   11,
		EntropyBytes: 7,
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}
	want.Seed = 789
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("load returned nil state")
	}
	if got.Version != want.Version || got.Seed != want.Seed || got.SceneRNGState != want.SceneRNGState || got.CommandSeq != want.CommandSeq {
		t.Fatalf("round trip mismatch: got %+v want %+v", *got, want)
	}
}

func TestSharedAtmospherePersistenceRoundTrip(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.broadcast(Command{Kind: "metric", Tick: 1})
	a.AddEntropy([]byte("hello"))
	for i := 0; i < 5; i++ {
		a.effect.Step()
	}
	a.rotateScene(a.effect.CurrentTick())
	a.applyTransition(a.effect.CurrentTick())

	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	got := restored.snapshot()
	want := a.snapshot()

	if got.Tick != want.Tick {
		t.Fatalf("tick = %d, want %d", got.Tick, want.Tick)
	}
	if got.EntropyBytes != want.EntropyBytes {
		t.Fatalf("entropy = %d, want %d", got.EntropyBytes, want.EntropyBytes)
	}
	if restored.currentCommandID() != a.currentCommandID() {
		t.Fatalf("command id = %q, want %q", restored.currentCommandID(), a.currentCommandID())
	}
	if got.CurrentScene.Name != want.CurrentScene.Name {
		t.Fatalf("current scene = %q, want %q", got.CurrentScene.Name, want.CurrentScene.Name)
	}
	if got.NextScene.Name != want.NextScene.Name {
		t.Fatalf("next scene = %q, want %q", got.NextScene.Name, want.NextScene.Name)
	}
	if !configsEqualJSON(got.CurrentScene.Config, want.CurrentScene.Config) {
		t.Fatalf("current scene config = %s, want %s", got.CurrentScene.Config, want.CurrentScene.Config)
	}
	if !configsEqualJSON(got.NextScene.Config, want.NextScene.Config) {
		t.Fatalf("next scene config = %s, want %s", got.NextScene.Config, want.NextScene.Config)
	}

	var gotState sim.RainSnapshot
	if err := json.Unmarshal(got.State, &gotState); err != nil {
		t.Fatalf("decode restored state: %v", err)
	}
	var wantState sim.RainSnapshot
	if err := json.Unmarshal(want.State, &wantState); err != nil {
		t.Fatalf("decode original state: %v", err)
	}
	if len(gotState.Drops) != len(wantState.Drops) {
		t.Fatalf("drops = %d, want %d", len(gotState.Drops), len(wantState.Drops))
	}
	if len(gotState.Splashes) != len(wantState.Splashes) {
		t.Fatalf("splashes = %d, want %d", len(gotState.Splashes), len(wantState.Splashes))
	}
}
