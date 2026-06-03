package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/romaine-life/ambience/rngutil"
	"github.com/romaine-life/ambience/sim"
)

const defaultPersistInterval = 30 * time.Second

type persistenceStore interface {
	Load(context.Context) (*persistedAtmosphere, error)
	Save(context.Context, persistedAtmosphere) error
}

type persistedTransition struct {
	From  json.RawMessage `json:"from,omitempty"`
	To    json.RawMessage `json:"to,omitempty"`
	Start int             `json:"start"`
	Dur   int             `json:"dur"`
}

type persistedAtmosphere struct {
	Version       int                  `json:"version"`
	SavedAt       time.Time            `json:"savedAt"`
	Type          string               `json:"type"`
	Seed          int64                `json:"seed"`
	SceneRNGState uint64               `json:"sceneRngState"`
	Config        json.RawMessage      `json:"config,omitempty"`
	Effect        persistedEffectState `json:"effect"`
	CommandSeq    int64                `json:"commandSeq"`
	CurrentScene  Scene                `json:"currentScene"`
	NextScene     Scene                `json:"nextScene"`
	EntropyBytes  int64                `json:"entropyBytes"`
	Transition    persistedTransition  `json:"transition"`
	// RotationStartTick is the tick on the active effect runtime at which
	// the current effect became current — preserved across restarts so
	// rotation cadence isn't reset by a pod restart. Old persisted blobs
	// without this field are detected during restore and anchored to the
	// restored effect's current tick, preserving the running effect instead
	// of immediately rotating away from it.
	RotationStartTick *int `json:"rotationStartTick,omitempty"`
}

func newPersistenceStoreFromEnv() (persistenceStore, time.Duration, error) {
	store, err := newCosmosStoreFromEnv()
	if err != nil {
		return nil, 0, err
	}
	if store == nil {
		return nil, 0, nil
	}
	interval := defaultPersistInterval
	if raw := os.Getenv("AMBIENCE_COSMOS_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("parse AMBIENCE_COSMOS_INTERVAL: %w", err)
		}
		if d <= 0 {
			return nil, 0, fmt.Errorf("AMBIENCE_COSMOS_INTERVAL must be > 0")
		}
		interval = d
	}
	return store, interval, nil
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
	return restoreSharedAtmosphereWithPolicy(ctx, store, rotationPolicy{})
}

func restoreSharedAtmosphereWithPolicy(ctx context.Context, store persistenceStore, policy rotationPolicy) *atmosphere {
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
		cfg:        cloneRaw(state.Config),
		seed:       state.Seed,
		sceneRNG:   rngutil.NewFromState(sceneRNGState),
		commandSeq: state.CommandSeq,
		current:    state.CurrentScene,
		next:       state.NextScene,
		listeners:  make(map[chan Command]struct{}),
		lastSeen:   time.Now(),
	}
	if len(a.cfg) == 0 {
		a.cfg = cloneRaw(state.Effect.Config)
	}
	a.normalizeRestoredSceneLabels(state.Type)
	a.normalizeRestoredSceneConfigs(state.Type)
	if err := a.effect.RestorePersisted(state.Effect); err != nil {
		log.Printf("restore %s state: %v; starting fresh", state.Type, err)
		return fresh
	}
	a.entropyBytes = state.EntropyBytes
	a.transitionFrom = cloneRaw(state.Transition.From)
	a.transitionTo = cloneRaw(state.Transition.To)
	a.transitionStart = state.Transition.Start
	a.transitionDur = state.Transition.Dur
	if state.RotationStartTick != nil {
		a.rotationStartTick = *state.RotationStartTick
	} else if policy.Enabled {
		a.rotationStartTick = a.effect.CurrentTick()
	}
	if a.transitionDur > 0 {
		a.cfg = cloneRaw(state.Transition.From)
	}

	log.Printf("restored shared atmosphere from %s at tick %d", state.SavedAt.Format(time.RFC3339), a.effect.CurrentTick())
	return a
}

func (a *atmosphere) normalizeRestoredSceneLabels(effectType string) {
	currentDur := a.current.DurationTicks
	if currentDur <= 0 {
		currentDur = defaultRotationCadenceTicks
	}
	if a.current.Name == "" || a.current.Name == effectType {
		a.current = generateEffectScene(effectType, a.sceneRNG, a.current.StartedAtTick, currentDur)
	} else {
		a.current.DurationTicks = currentDur
	}

	nextDur := a.next.DurationTicks
	if nextDur <= 0 {
		nextDur = currentDur
	}
	if a.next.Name == "" || a.next.Name == effectType {
		a.next = generateEffectScene(effectType, a.sceneRNG, a.next.StartedAtTick, nextDur)
	} else {
		a.next.DurationTicks = nextDur
	}
}

func (a *atmosphere) normalizeRestoredSceneConfigs(effectType string) {
	if len(a.current.Config) == 0 {
		if len(a.cfg) > 0 {
			a.current.Config = cloneRaw(a.cfg)
		}
	}
	if len(a.next.Config) == 0 {
		a.next = generateEffectScene(effectType, a.sceneRNG, a.next.StartedAtTick, a.next.DurationTicks)
	}
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
		From:  cloneRaw(a.transitionFrom),
		To:    cloneRaw(a.transitionTo),
		Start: a.transitionStart,
		Dur:   a.transitionDur,
	}
	rotationStartTick := a.rotationStartTick
	rotationStartTickPtr := &rotationStartTick
	a.mu.Unlock()

	effectState, err := a.effect.Persisted()
	if err != nil {
		log.Printf("snapshot %s persisted state: %v", a.effect.Type(), err)
		effectState = persistedEffectState{}
	}
	return persistedAtmosphere{
		Version:           1,
		SavedAt:           time.Now().UTC(),
		Type:              a.effect.Type(),
		Seed:              seed,
		SceneRNGState:     sceneRNGState,
		Config:            cloneRaw(a.cfg),
		Effect:            effectState,
		CommandSeq:        commandSeq,
		CurrentScene:      current,
		NextScene:         next,
		EntropyBytes:      entropyBytes,
		Transition:        transition,
		RotationStartTick: rotationStartTickPtr,
	}
}
