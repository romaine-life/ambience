package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nelsong6/ambience/rngutil"
	"github.com/nelsong6/ambience/sim"
)

const sceneSeedMix int64 = 0x6d0f27bd0b5a3c11

func effectDisplayName(effectType string) string {
	return strings.ReplaceAll(strings.TrimSpace(effectType), "-", " ")
}

func effectHash(effectType string) int64 {
	var hash int64 = 1469598103934665603
	for _, ch := range strings.TrimSpace(effectType) {
		hash ^= int64(ch)
		hash *= 1099511628211
	}
	if hash < 0 {
		hash = -hash
	}
	if hash == 0 {
		hash = 1
	}
	return hash
}

func deriveEffectSeed(base int64, tick int, effectType string) int64 {
	seed := base ^ (int64(tick+1) * 1103515245) ^ effectHash(effectType)
	if seed == 0 {
		seed = effectHash(effectType)
	}
	return seed
}

func randomizedEffectConfig(effectType string, seed int64) (json.RawMessage, error) {
	schema, ok := schemaForEffect(effectType)
	if !ok {
		return nil, fmt.Errorf("unknown effect type %q", effectType)
	}
	return randomizedDevConfig(schema, seed)
}

func buildSharedEffectState(effectType string, seed int64, tick int) (effectRuntime, *rngutil.RNG, Scene, Scene, sim.Config, error) {
	switch effectType {
	case "rain":
		sceneRNG := rngutil.New(seed ^ sceneSeedMix)
		current := generateScene(sceneRNG, tick)
		next := generateScene(sceneRNG, 0)
		cfgData, _ := json.Marshal(current.Config)
		rt, err := newEffectRuntime(effectType, gridW, gridH, seed, cfgData)
		if err != nil {
			return nil, nil, Scene{}, Scene{}, sim.Config{}, err
		}
		return rt, sceneRNG, current, next, current.Config, nil
	default:
		cfgData, err := randomizedEffectConfig(effectType, seed)
		if err != nil {
			return nil, nil, Scene{}, Scene{}, sim.Config{}, err
		}
		rt, err := newEffectRuntime(effectType, gridW, gridH, seed, cfgData)
		if err != nil {
			return nil, nil, Scene{}, Scene{}, sim.Config{}, err
		}
		current := Scene{
			Name:          effectDisplayName(effectType),
			DurationTicks: 0,
			StartedAtTick: tick,
		}
		return rt, rngutil.New(seed ^ sceneSeedMix), current, Scene{}, sim.Config{}, nil
	}
}

func (a *atmosphere) broadcastSnapshot() {
	snap := a.snapshot()
	data, _ := json.Marshal(snap)
	a.broadcast(Command{
		Kind: "snapshot",
		Tick: snap.Tick,
		Data: data,
	})
}

func (a *atmosphere) switchEffect(effectType string) error {
	effectType = normalizeDevEffect(effectType)
	oldEffect, _ := a.currentEffectRuntime()
	tick := oldEffect.CurrentTick()

	a.mu.Lock()
	baseSeed := a.seed
	a.mu.Unlock()

	seed := deriveEffectSeed(baseSeed, tick, effectType)
	rt, sceneRNG, current, next, cfg, err := buildSharedEffectState(effectType, seed, tick)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.effect = rt
	a.effectVersion++
	a.seed = seed
	a.sceneRNG = sceneRNG
	a.cfg = cfg
	a.current = current
	a.next = next
	a.transitionFrom = sim.Config{}
	a.transitionTo = sim.Config{}
	a.transitionStart = 0
	a.transitionDur = 0
	a.mu.Unlock()

	a.broadcastSnapshot()
	return nil
}

func (s *devSession) broadcastSnapshot() {
	snap := s.snapshot()
	data, _ := json.Marshal(snap)
	effect, _ := s.currentEffectRuntime()
	s.broadcast(Command{
		Kind: "snapshot",
		Tick: effect.CurrentTick(),
		Data: data,
	})
}

func (s *devSession) switchEffect(effectType string) error {
	effectType = normalizeDevEffect(effectType)
	effect, _ := s.currentEffectRuntime()
	tick := effect.CurrentTick()

	s.mu.Lock()
	baseSeed := s.seed
	s.mu.Unlock()

	seed := deriveEffectSeed(baseSeed, tick, effectType)
	cfgData, err := randomizedEffectConfig(effectType, seed)
	if err != nil {
		return err
	}
	rt, err := newEffectRuntime(effectType, gridW, gridH, seed, cfgData)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.seed = seed
	s.effect = rt
	s.effectVersion++
	s.mu.Unlock()

	s.broadcastSnapshot()
	return nil
}
