package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// A world is a named effect-set the authority hosts as an independent
// atmosphere. The default world (name "") is the public, rotating world served
// at the bare paths (/snapshot, /events, /entropy), backed by the global
// `shared` atmosphere. Named worlds are curated sets served under /{name}/ with
// their own atmosphere — e.g. "chess" serves only rain and never rotates.
//
// A consumer subscribes to exactly one world. The world's snapshot carries
// servedEffects (its effect-set); the consumer's vendored runtime handshakes
// that it supports every entry. This is why a world is a first-class concept
// rather than a flag: each is an independent, separately-addressable broadcast
// with its own capability contract.
//
// Named worlds are deliberately a narrow public contract — a read-only stream
// (snapshot + events) plus entropy intake. Live mutation, dev sessions,
// og-image, and persistence remain default-world concerns.

// worldDef is the static declaration of a named world.
type worldDef struct {
	Name     string         // route prefix segment; non-empty for named worlds
	Effect   string         // seed effect the atmosphere starts on
	Rotation rotationPolicy // served effect-set (Allowed) + whether it rotates
}

// namedWorldDefs returns the worlds the authority hosts alongside the default
// rotating world. Add an entry here to host another curated effect-set.
func namedWorldDefs() []worldDef {
	return []worldDef{
		{
			// Chess Tactics main-menu world: pinned rain, never rotates.
			// servedEffects resolves to ["rain"], so a rain-only vendored
			// client handshakes cleanly.
			Name:     "chess",
			Effect:   "rain",
			Rotation: rotationPolicy{Enabled: false, Allowed: []string{"rain"}},
		},
	}
}

// liveWorld binds a worldDef to its running atmosphere.
type liveWorld struct {
	def        worldDef
	atmosphere *atmosphere
}

// namedWorlds holds the running named worlds. The default world is not here —
// it stays addressable as the global `shared`. Populated by bootAuthority.
var namedWorlds []*liveWorld

// startNamedWorlds constructs and runs one atmosphere per named world. Named
// worlds are not persisted (they are pinned/curated, not the resumable shared
// world), so there is no Cosmos interaction and nothing to collide with the
// default world's persisted document.
func startNamedWorlds(ctx context.Context) {
	for _, def := range namedWorldDefs() {
		a := newAtmosphereWithEffect(def.Effect)
		a.setRotationPolicy(def.Rotation)
		go a.run(ctx)
		namedWorlds = append(namedWorlds, &liveWorld{def: def, atmosphere: a})
	}
}

// The handlers below are bound to a specific atmosphere so both the default
// world (passing `shared`) and named worlds share one implementation.

func serveAtmosphereSnapshot(a *atmosphere) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.snapshot())
	}
}

func serveAtmosphereEvents(a *atmosphere) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		streamAtmosphere(w, req, a)
	}
}

// serveAtmosphereEntropy accepts raw bytes (POSTed by a consumer on a throttled
// keystroke cadence) and folds them into this world's RNG. Cheap, lossy,
// intentional — aesthetic perturbation, not secure randomness. A small size cap
// keeps the endpoint to short entropy bursts.
func serveAtmosphereEntropy(a *atmosphere) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		const maxBytes = 4096
		req.Body = http.MaxBytesReader(w, req.Body, maxBytes)
		buf := make([]byte, maxBytes)
		n, _ := io.ReadFull(req.Body, buf)
		if n > 0 {
			a.AddEntropy(buf[:n])
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
