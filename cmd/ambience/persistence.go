package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nelsong6/ambience/rngutil"
	"github.com/nelsong6/ambience/sim"
)

const defaultPersistInterval = 30 * time.Second

type persistenceStore interface {
	Load(context.Context) (*persistedAtmosphere, error)
	Save(context.Context, persistedAtmosphere) error
}

type fileStore struct {
	path string
}

type persistedTransition struct {
	From  sim.Config `json:"from"`
	To    sim.Config `json:"to"`
	Start int        `json:"start"`
	Dur   int        `json:"dur"`
}

type persistedAtmosphere struct {
	Version       int                  `json:"version"`
	SavedAt       time.Time            `json:"savedAt"`
	Type          string               `json:"type"`
	Seed          int64                `json:"seed"`
	SceneRNGState uint64               `json:"sceneRngState"`
	Config        sim.Config           `json:"config"`
	Effect        persistedEffectState `json:"effect"`
	CommandSeq    int64                `json:"commandSeq"`
	CurrentScene  Scene                `json:"currentScene"`
	NextScene     Scene                `json:"nextScene"`
	EntropyBytes  int64                `json:"entropyBytes"`
	Transition    persistedTransition  `json:"transition"`
}

func newPersistenceStoreFromEnv() (persistenceStore, time.Duration, error) {
	path := os.Getenv("AMBIENCE_PERSIST_PATH")
	if path == "" {
		return nil, 0, nil
	}
	interval := defaultPersistInterval
	if raw := os.Getenv("AMBIENCE_PERSIST_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("parse AMBIENCE_PERSIST_INTERVAL: %w", err)
		}
		if d <= 0 {
			return nil, 0, fmt.Errorf("AMBIENCE_PERSIST_INTERVAL must be > 0")
		}
		interval = d
	}
	return &fileStore{path: path}, interval, nil
}

func (f *fileStore) Load(_ context.Context) (*persistedAtmosphere, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var state persistedAtmosphere
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Version != 1 {
		return nil, fmt.Errorf("unsupported persisted state version %d", state.Version)
	}
	return &state, nil
}

func (f *fileStore) Save(_ context.Context, state persistedAtmosphere) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, f.path)
}

func persistLoop(ctx context.Context, interval time.Duration, store persistenceStore, a *atmosphere) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			savePersistedState(context.Background(), store, a)
			return
		case <-t.C:
			savePersistedState(ctx, store, a)
		}
	}
}

func savePersistedState(ctx context.Context, store persistenceStore, a *atmosphere) {
	if store == nil {
		return
	}
	if err := store.Save(ctx, a.persistedState()); err != nil {
		log.Printf("persist shared atmosphere: %v", err)
	}
}

func restoreSharedAtmosphere(ctx context.Context, store persistenceStore) *atmosphere {
	fresh := newAtmosphere(sim.Config{})
	if store == nil {
		return fresh
	}

	state, err := store.Load(ctx)
	if err != nil {
		log.Printf("restore shared atmosphere: %v; starting fresh", err)
		return fresh
	}
	if state == nil {
		log.Printf("shared atmosphere persistence enabled, no prior snapshot found")
		return fresh
	}

	if state.Effect.GridW <= 0 {
		state.Effect.GridW = gridW
	}
	if state.Effect.GridH <= 0 {
		state.Effect.GridH = gridH
	}

	sceneRNGState := state.SceneRNGState
	if sceneRNGState == 0 {
		sceneRNGState = uint64(state.Seed ^ 0x6d0f27bd0b5a3c11)
	}

	rt, err := newEffectRuntime(state.Type, state.Effect.GridW, state.Effect.GridH, state.Seed, state.Effect.Config)
	if err != nil {
		log.Printf("restore shared atmosphere: %v; starting fresh", err)
		return fresh
	}

	a := &atmosphere{
		effect:     rt,
		cfg:        state.Config,
		seed:       state.Seed,
		sceneRNG:   rngutil.NewFromState(sceneRNGState),
		commandSeq: state.CommandSeq,
		current:    state.CurrentScene,
		next:       state.NextScene,
		listeners:  make(map[chan Command]struct{}),
		lastSeen:   time.Now(),
	}
	if err := a.effect.RestorePersisted(state.Effect); err != nil {
		log.Printf("restore %s state: %v; starting fresh", state.Type, err)
		return fresh
	}
	a.entropyBytes = state.EntropyBytes
	a.transitionFrom = state.Transition.From
	a.transitionTo = state.Transition.To
	a.transitionStart = state.Transition.Start
	a.transitionDur = state.Transition.Dur
	if a.transitionDur > 0 {
		a.cfg = state.Transition.From
	}

	log.Printf("restored shared atmosphere from %s at tick %d", state.SavedAt.Format(time.RFC3339), a.effect.CurrentTick())
	return a
}

func (a *atmosphere) persistedState() persistedAtmosphere {
	a.mu.Lock()
	seed := a.seed
	sceneRNGState := a.sceneRNG.State()
	current := a.current
	next := a.next
	entropyBytes := a.entropyBytes
	commandSeq := a.commandSeq
	transition := persistedTransition{
		From:  a.transitionFrom,
		To:    a.transitionTo,
		Start: a.transitionStart,
		Dur:   a.transitionDur,
	}
	a.mu.Unlock()

	effectState, err := a.effect.Persisted()
	if err != nil {
		log.Printf("snapshot %s persisted state: %v", a.effect.Type(), err)
		effectState = persistedEffectState{}
	}
	effectConfig := a.cfg
	if len(effectState.Config) > 0 {
		if err := json.Unmarshal(effectState.Config, &effectConfig); err != nil {
			log.Printf("decode %s persisted config: %v", a.effect.Type(), err)
			effectConfig = a.cfg
		}
	}

	return persistedAtmosphere{
		Version:       1,
		SavedAt:       time.Now().UTC(),
		Type:          a.effect.Type(),
		Seed:          seed,
		SceneRNGState: sceneRNGState,
		Config:        effectConfig,
		Effect:        effectState,
		CommandSeq:    commandSeq,
		CurrentScene:  current,
		NextScene:     next,
		EntropyBytes:  entropyBytes,
		Transition:    transition,
	}
}
