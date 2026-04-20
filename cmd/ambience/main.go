// ambience serves the ambient-effect coordination service. Clients run
// their own local sim; the server's job is to broadcast state changes
// (config + discrete events) that the clients apply.
//
// Routes:
//
//	GET  /                      — canonical demo page (browser runs a JS sim)
//	GET  /ambience.js           — shared renderer / SSE helpers
//	GET  /sim.js                — JS port of sim/rain.go (runs in browser)
//	GET  /snapshot              — shared atmosphere init payload (JSON)
//	GET  /events                — shared atmosphere SSE command stream
//	GET  /dev                   — dev page with knob controls
//	GET  /dev/snapshot?session= — dev atmosphere snapshot (JSON, creates if new)
//	GET  /dev/events?session=   — dev atmosphere SSE command stream
//	POST /dev/config?session=   — update the dev atmosphere's config
//	POST /dev/trigger/:session/:event
//	                            — fire a discrete event on the dev atmosphere
//	GET  /effects/rain/schema   — JSON schema for Rain's knob panel
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nelsong6/ambience/sim"
)

const (
	gridW           = 160
	gridH           = 80
	tickRate        = 100 * time.Millisecond // ~10 Hz
	defaultAddr     = ":8080"
	devSessionIdle  = 60 * time.Second
	devSessionSweep = 30 * time.Second
)

//go:embed web
var webFS embed.FS

var shared *atmosphere

func main() {
	shared = newAtmosphere(sim.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go shared.run(ctx)
	go sweepDevAtmospheres()

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(web)))
	http.HandleFunc("/dev", serveDevPage(web))
	http.HandleFunc("/effects/rain/schema", cors(serveSchema))

	// Shared atmosphere — CORS-enabled so third-party pages (fzt-showcase,
	// my-homepage, etc.) can consume the stream. Read-only endpoints; no
	// state mutation risk.
	http.HandleFunc("/snapshot", cors(serveSharedSnapshot))
	http.HandleFunc("/events", cors(serveSharedEvents))
	// Entropy intake — clients POST keystroke-derived bytes here; bytes
	// get folded into the shared atmosphere's RNG. Also CORS-enabled so
	// browser consumers can contribute.
	http.HandleFunc("/entropy", cors(serveEntropy))

	// Dev atmospheres (per-session)
	http.HandleFunc("/dev/snapshot", serveDevSnapshot)
	http.HandleFunc("/dev/events", serveDevEvents)
	http.HandleFunc("/dev/config", serveDevConfig)
	http.HandleFunc("/dev/trigger/", serveDevTrigger)

	// AMBIENCE_ADDR overrides the bind address. For local dev, set it to
	// "127.0.0.1:8080" so Windows Firewall doesn't prompt (loopback skips
	// the firewall). Kubernetes keeps the default ":8080" for pod reachability.
	addr := defaultAddr
	if envAddr := os.Getenv("AMBIENCE_ADDR"); envAddr != "" {
		addr = envAddr
	}
	log.Printf("ambience listening on %s (grid %dx%d, tick %s)", addr, gridW, gridH, tickRate)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// cors wraps a handler to send permissive CORS headers. Safe because the
// wrapped endpoints are read-only broadcast streams — no state mutation
// based on request origin, no cookies/auth consulted.
func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func serveDevPage(web fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/dev" {
			http.NotFound(w, req)
			return
		}
		data, err := fs.ReadFile(web, "dev.html")
		if err != nil {
			http.Error(w, "dev page not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}
}

func serveSchema(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sim.RainSchema())
}

// sseHeaders sets the three standard SSE headers and returns a Flusher.
func sseHeaders(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return flusher, true
}

// writeCommand writes one SSE frame, flushing.
func writeCommand(w http.ResponseWriter, flusher http.Flusher, cmd Command) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// writeSnapshotFrame encodes an initial snapshot and sends it as the first
// SSE frame for a new subscriber.
func writeSnapshotFrame(w http.ResponseWriter, flusher http.Flusher, a *atmosphere) error {
	snap := a.snapshot()
	data, _ := json.Marshal(snap)
	return writeCommand(w, flusher, Command{Kind: "snapshot", Tick: snap.Tick, Data: data})
}

func streamAtmosphere(w http.ResponseWriter, req *http.Request, a *atmosphere) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	ch := a.addListener()
	defer a.removeListener(ch)

	if err := writeSnapshotFrame(w, flusher, a); err != nil {
		return
	}

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case cmd, ok := <-ch:
			if !ok {
				return
			}
			if err := writeCommand(w, flusher, cmd); err != nil {
				return
			}
		}
	}
}

func serveSharedSnapshot(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(shared.snapshot())
}

func serveSharedEvents(w http.ResponseWriter, req *http.Request) {
	streamAtmosphere(w, req, shared)
}

// serveEntropy accepts raw bytes from clients (POSTed by client.js on a
// throttled keystroke cadence) and folds them into the shared atmosphere's
// RNG. Cheap, lossy, intentional — this is aesthetic perturbation, not
// secure randomness. A small request-size cap keeps the endpoint from
// being used for anything but short entropy bursts.
func serveEntropy(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	const maxBytes = 4096
	req.Body = http.MaxBytesReader(w, req.Body, maxBytes)
	buf := make([]byte, maxBytes)
	n, _ := io.ReadFull(req.Body, buf)
	if n > 0 {
		shared.AddEntropy(buf[:n])
	}
	w.WriteHeader(http.StatusNoContent)
}

func devAtmosphereFromRequest(req *http.Request) (*atmosphere, string, error) {
	sessionID := req.URL.Query().Get("session")
	if sessionID == "" {
		return nil, "", fmt.Errorf("session param required")
	}
	return getOrCreateDevAtmosphere(sessionID), sessionID, nil
}

func serveDevSnapshot(w http.ResponseWriter, req *http.Request) {
	a, _, err := devAtmosphereFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.snapshot())
}

func serveDevEvents(w http.ResponseWriter, req *http.Request) {
	a, _, err := devAtmosphereFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	streamAtmosphere(w, req, a)
}

func serveDevConfig(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	a, _, err := devAtmosphereFromRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg := parseDevConfig(req)
	a.setConfig(cfg)
	w.WriteHeader(http.StatusNoContent)
}

func serveDevTrigger(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(req.URL.Path, "/dev/trigger/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "usage: /dev/trigger/<session>/<event>", http.StatusBadRequest)
		return
	}
	sessionID, event := parts[0], parts[1]
	v, ok := devAtmospheres.Load(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	a := v.(*atmosphere)
	if !a.triggerEvent(event) {
		http.Error(w, "unknown event: "+event, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseDevConfig(req *http.Request) sim.Config {
	q := req.URL.Query()
	cfg := sim.Config{}
	getFloat(q, "wind", &cfg.Wind)
	getFloat(q, "wind_jit", &cfg.WindJitter)
	getFloat(q, "speed", &cfg.Speed)
	getFloat(q, "speed_jit", &cfg.SpeedJitter)
	getInt(q, "streak", &cfg.StreakLen)
	getFloat(q, "fade", &cfg.FadeFactor)
	getInt(q, "spawn", &cfg.SpawnEvery)
	getInt(q, "burst", &cfg.SpawnBurst)
	getFloat(q, "hue", &cfg.Hue)
	getFloat(q, "hue_sp", &cfg.HueSpread)
	getFloat(q, "sat", &cfg.Saturation)
	getFloat(q, "lmin", &cfg.LightnessMin)
	getFloat(q, "lmax", &cfg.LightnessMax)
	getInt(q, "layers", &cfg.Layers)
	getFloat(q, "lbal", &cfg.LayerBalance)
	getFloat(q, "hue_drift", &cfg.HueDriftAmp)
	getFloat(q, "wind_drift", &cfg.WindDriftAmp)
	getFloat(q, "downpour_p", &cfg.DownpourChance)
	getFloat(q, "calm_p", &cfg.CalmChance)
	getFloat(q, "gust_p", &cfg.GustChance)
	getFloat(q, "splash_p", &cfg.SplashChance)
	getInt(q, "downpour_dur", &cfg.DownpourDur)
	getFloat(q, "downpour_mult", &cfg.DownpourMult)
	getInt(q, "calm_dur", &cfg.CalmDur)
	getInt(q, "gust_dur", &cfg.GustDur)
	getFloat(q, "gust_str", &cfg.GustStrength)
	getInt(q, "splash_size", &cfg.SplashSize)
	return cfg
}

func getFloat(q map[string][]string, key string, dst *float64) {
	if v, ok := q[key]; ok && len(v) > 0 && v[0] != "" {
		if f, err := strconv.ParseFloat(v[0], 64); err == nil {
			*dst = f
		}
	}
}

func getInt(q map[string][]string, key string, dst *int) {
	if v, ok := q[key]; ok && len(v) > 0 && v[0] != "" {
		if n, err := strconv.Atoi(v[0]); err == nil {
			*dst = n
		}
	}
}
