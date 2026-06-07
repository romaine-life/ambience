package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/romaine-life/ambience/sim"
)

const (
	sceneMinMinutesKey        = "scene_min_m"
	sceneMaxMinutesKey        = "scene_max_m"
	sceneTransitionMinutesKey = "scene_transition_m"
	sceneVariationKey         = "scene_variation"
)

func sharedEffectSchema(req *http.Request) (string, sim.EffectSchema, error) {
	if shared == nil {
		return "", sim.EffectSchema{}, fmt.Errorf("shared atmosphere unavailable")
	}
	snap := shared.snapshot()
	effectType := strings.TrimSpace(strings.ToLower(snap.Type))
	if effectType == "" {
		return "", sim.EffectSchema{}, fmt.Errorf("shared atmosphere unavailable")
	}
	if requested := strings.TrimSpace(strings.ToLower(req.URL.Query().Get("effect"))); requested != "" && requested != effectType {
		return "", sim.EffectSchema{}, fmt.Errorf("shared atmosphere is %s, not %s", effectType, requested)
	}
	schema, ok := schemaForEffect(effectType)
	if !ok {
		return "", sim.EffectSchema{}, fmt.Errorf("unknown shared effect %q", effectType)
	}
	return effectType, schema, nil
}

func serveSharedConfig(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !controlAuth.require(w, req) {
		return
	}
	_, schema, err := sharedEffectSchema(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	values := req.URL.Query()
	if hasEffectConfigValues(values, schema) {
		data, err := parseEffectConfig(values, schema)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := shared.setConfigRaw(data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if hasScenePolicyValues(values) {
		policy, err := parseScenePolicyValues(values, shared.scenePolicySnapshot())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		shared.setScenePolicy(policy)
	}
	w.WriteHeader(http.StatusNoContent)
}

func hasEffectConfigValues(values map[string][]string, schema sim.EffectSchema) bool {
	for _, knob := range schema.Knobs {
		if _, ok := values[knob.Key]; ok {
			return true
		}
	}
	return false
}

func hasScenePolicyValues(values map[string][]string) bool {
	for _, key := range []string{sceneMinMinutesKey, sceneMaxMinutesKey, sceneTransitionMinutesKey, sceneVariationKey} {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func parseScenePolicyValues(values map[string][]string, current scenePolicyData) (scenePolicy, error) {
	policy := scenePolicy{
		MinTicks:        current.MinTicks,
		MaxTicks:        current.MaxTicks,
		TransitionTicks: current.TransitionTicks,
		Variation:       current.Variation,
	}
	if raw := strings.TrimSpace(firstValue(values, sceneMinMinutesKey)); raw != "" {
		v, err := parseBoundedFloat(raw, 1, 12*60)
		if err != nil {
			return scenePolicy{}, fmt.Errorf("parse %s: %w", sceneMinMinutesKey, err)
		}
		policy.MinTicks = ticksFor(time.Duration(v * float64(time.Minute)))
	}
	if raw := strings.TrimSpace(firstValue(values, sceneMaxMinutesKey)); raw != "" {
		v, err := parseBoundedFloat(raw, 1, 12*60)
		if err != nil {
			return scenePolicy{}, fmt.Errorf("parse %s: %w", sceneMaxMinutesKey, err)
		}
		policy.MaxTicks = ticksFor(time.Duration(v * float64(time.Minute)))
	}
	if raw := strings.TrimSpace(firstValue(values, sceneTransitionMinutesKey)); raw != "" {
		v, err := parseBoundedFloat(raw, 0, 120)
		if err != nil {
			return scenePolicy{}, fmt.Errorf("parse %s: %w", sceneTransitionMinutesKey, err)
		}
		policy.TransitionTicks = ticksFor(time.Duration(v * float64(time.Minute)))
	}
	if raw := strings.TrimSpace(firstValue(values, sceneVariationKey)); raw != "" {
		v, err := parseBoundedFloat(raw, 0.05, 1)
		if err != nil {
			return scenePolicy{}, fmt.Errorf("parse %s: %w", sceneVariationKey, err)
		}
		policy.Variation = v
	}
	return policy.normalized(), nil
}

func firstValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func parseBoundedFloat(raw string, minValue, maxValue float64) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	if v < minValue {
		v = minValue
	}
	if v > maxValue {
		v = maxValue
	}
	return v, nil
}

func serveSharedTrigger(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !controlAuth.require(w, req) {
		return
	}
	event := strings.Trim(strings.TrimPrefix(req.URL.Path, "/trigger/"), "/")
	if event == "" || strings.Contains(event, "/") {
		http.Error(w, "usage: /trigger/<event>?effect=<name>", http.StatusBadRequest)
		return
	}
	if _, _, err := sharedEffectSchema(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !shared.triggerEvent(event) {
		http.Error(w, "unknown event: "+event, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func serveSharedNextEffect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !controlAuth.require(w, req) {
		return
	}
	if shared == nil {
		http.Error(w, "shared atmosphere unavailable", http.StatusServiceUnavailable)
		return
	}
	if !shared.rotateToNextEffect() {
		http.Error(w, "no alternate effect available", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
