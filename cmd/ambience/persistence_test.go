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
	effect, _ := a.currentEffectRuntime()
	for i := 0; i < 5; i++ {
		effect.Step()
	}
	a.rotateScene(effect.CurrentTick())
	a.applyTransition(effect.CurrentTick())

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

func TestSharedAtmospherePersistenceRoundTripAfterEffectSwitch(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	if err := a.switchEffect("dust"); err != nil {
		t.Fatalf("switchEffect: %v", err)
	}

	store := &fileStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := restoreSharedAtmosphere(context.Background(), store)
	got := restored.snapshot()
	if got.Type != "dust" {
		t.Fatalf("restored type = %q, want dust", got.Type)
	}
	if got.CurrentScene.Name != "dust" {
		t.Fatalf("restored current scene = %q, want dust", got.CurrentScene.Name)
	}
}
