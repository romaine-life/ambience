package main

import (
	"context"
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
		GridW:         160,
		GridH:         80,
		EntropyBytes:  7,
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
	if got.Version != want.Version || got.Seed != want.Seed || got.SceneRNGState != want.SceneRNGState {
		t.Fatalf("round trip mismatch: got %+v want %+v", *got, want)
	}
}

func TestSharedAtmospherePersistenceRoundTrip(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.AddEntropy([]byte("hello"))
	for i := 0; i < 5; i++ {
		a.sim.Step()
	}
	a.rotateScene(a.sim.CurrentTick())
	a.applyTransition(a.sim.CurrentTick())

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
	if got.CurrentScene.Name != want.CurrentScene.Name {
		t.Fatalf("current scene = %q, want %q", got.CurrentScene.Name, want.CurrentScene.Name)
	}
	if got.NextScene.Name != want.NextScene.Name {
		t.Fatalf("next scene = %q, want %q", got.NextScene.Name, want.NextScene.Name)
	}
	if len(got.Drops) != len(want.Drops) {
		t.Fatalf("drops = %d, want %d", len(got.Drops), len(want.Drops))
	}
	if len(got.Splashes) != len(want.Splashes) {
		t.Fatalf("splashes = %d, want %d", len(got.Splashes), len(want.Splashes))
	}
}
