package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/romaine-life/ambience/sim"
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
	data, err := parseEffectConfig(req.URL.Query(), schema)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := shared.setConfigRaw(data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
