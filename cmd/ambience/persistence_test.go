package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

	"github.com/nelsong6/ambience/sim"
)

// memoryStore is the in-memory persistenceStore used by tests that exercise
// the atmosphere round-trip without needing a real Cosmos client.
type memoryStore struct {
	saved *persistedAtmosphere
}

func (m *memoryStore) Load(_ context.Context) (*persistedAtmosphere, error) {
	if m.saved == nil {
		return nil, nil
	}
	clone := *m.saved
	return &clone, nil
}

func (m *memoryStore) Save(_ context.Context, state persistedAtmosphere) error {
	clone := state
	m.saved = &clone
	return nil
}

// fakeCosmosContainer fakes *azcosmos.ContainerClient at the cosmosContainer
// interface boundary. Tests use it to exercise cosmosStore without a real
// Cosmos account.
type fakeCosmosContainer struct {
	items map[string][]byte
}

func (f *fakeCosmosContainer) ReadItem(_ context.Context, _ azcosmos.PartitionKey, id string, _ *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	data, ok := f.items[id]
	if !ok {
		return azcosmos.ItemResponse{}, &azcore.ResponseError{StatusCode: http.StatusNotFound}
	}
	return azcosmos.ItemResponse{Value: data}, nil
}

func (f *fakeCosmosContainer) UpsertItem(_ context.Context, _ azcosmos.PartitionKey, item []byte, _ *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	var meta struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(item, &meta); err != nil {
		return azcosmos.ItemResponse{}, err
	}
	if f.items == nil {
		f.items = map[string][]byte{}
	}
	f.items[meta.ID] = item
	return azcosmos.ItemResponse{}, nil
}

func TestCosmosStoreLoadMissingReturnsNil(t *testing.T) {
	store := &cosmosStore{container: &fakeCosmosContainer{}, docID: "shared"}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil from empty store, got %+v", got)
	}
}

func TestCosmosStoreRoundTrip(t *testing.T) {
	store := &cosmosStore{container: &fakeCosmosContainer{}, docID: "shared"}
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

func TestCosmosStoreLoadRejectsUnknownVersion(t *testing.T) {
	fake := &fakeCosmosContainer{}
	store := &cosmosStore{container: fake, docID: "shared"}
	bad, err := json.Marshal(cosmosDoc{ID: "shared", State: persistedAtmosphere{Version: 999}})
	if err != nil {
		t.Fatalf("marshal bad doc: %v", err)
	}
	if _, err := fake.UpsertItem(context.Background(), azcosmos.NewPartitionKeyString("shared"), bad, nil); err != nil {
		t.Fatalf("seed fake: %v", err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("expected error loading unknown version, got nil")
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

	store := &memoryStore{}
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
