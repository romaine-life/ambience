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
	sseHeartbeat    = 10 * time.Second
	defaultAddr     = ":8080"
	devSessionIdle  = 60 * time.Second
	devSessionSweep = 30 * time.Second
)

type appRole string

const (
	roleAll       appRole = "all"
	roleAuthority appRole = "authority"
	roleEdge      appRole = "edge"
)

type appConfig struct {
	role         appRole
	addr         string
	authorityURL string
}

//go:embed web
var webFS embed.FS

var shared *atmosphere

func main() {
	cfg, err := loadAppConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	readyCheck := func() bool { return true }

	switch cfg.role {
	case roleAll, roleAuthority:
		if err := bootAuthority(ctx); err != nil {
			log.Fatal(err)
		}
		registerStaticRoutes(mux, web)
		registerSchemaRoute(mux)
		registerAuthorityRoutes(mux)
		registerDevRoutes(mux)
	case roleEdge:
		proxy, err := newAuthorityProxy(ctx, cfg.authorityURL)
		if err != nil {
			log.Fatal(err)
		}
		readyCheck = proxy.ready
		registerStaticRoutes(mux, web)
		registerSchemaRoute(mux)
		registerEdgeRoutes(mux, proxy)
	default:
		log.Fatalf("unsupported ambience role %q", cfg.role)
	}
	registerCommonRoutes(mux, readyCheck)

	// AMBIENCE_ADDR overrides the bind address. For local dev, set it to
	// "127.0.0.1:8080" so Windows Firewall doesn't prompt (loopback skips
	// the firewall). Kubernetes keeps the default ":8080" for pod reachability.
	log.Printf("ambience listening on %s (role=%s, grid %dx%d, tick %s)", cfg.addr, cfg.role, gridW, gridH, tickRate)
	log.Fatal(http.ListenAndServe(cfg.addr, mux))
}

func loadAppConfigFromEnv() (appConfig, error) {
	cfg := appConfig{
		role: roleAll,
		addr: defaultAddr,
	}
	if envRole := strings.TrimSpace(strings.ToLower(os.Getenv("AMBIENCE_ROLE"))); envRole != "" {
		cfg.role = appRole(envRole)
	}
	switch cfg.role {
	case roleAll, roleAuthority, roleEdge:
	default:
		return appConfig{}, fmt.Errorf("AMBIENCE_ROLE must be one of %q, %q, %q", roleAll, roleAuthority, roleEdge)
	}
	if envAddr := os.Getenv("AMBIENCE_ADDR"); envAddr != "" {
		cfg.addr = envAddr
	}
	cfg.authorityURL = strings.TrimRight(strings.TrimSpace(os.Getenv("AMBIENCE_AUTHORITY_URL")), "/")
	if cfg.role == roleEdge && cfg.authorityURL == "" {
		return appConfig{}, fmt.Errorf("AMBIENCE_AUTHORITY_URL is required when AMBIENCE_ROLE=%q", roleEdge)
	}
	return cfg, nil
}

func bootAuthority(ctx context.Context) error {
	store, persistInterval, err := newPersistenceStoreFromEnv()
	if err != nil {
		return err
	}
	shared = restoreSharedAtmosphere(ctx, store)
	go shared.run(ctx)
	if store != nil {
		go persistLoop(ctx, persistInterval, store, shared)
	}
	go sweepDevAtmospheres()
	return nil
}

func registerCommonRoutes(mux *http.ServeMux, ready func() bool) {
	mux.HandleFunc("/healthz", serveHealthz)
	mux.HandleFunc("/readyz", serveReadyz(ready))
}

func registerStaticRoutes(mux *http.ServeMux, web fs.FS) {
	mux.HandleFunc("/dev", serveDevPage(web))
	mux.Handle("/", http.FileServer(http.FS(web)))
}

func registerSchemaRoute(mux *http.ServeMux) {
	mux.HandleFunc("/effects/rain/schema", cors(serveSchema))
}

func registerAuthorityRoutes(mux *http.ServeMux) {
	// Shared atmosphere — CORS-enabled so third-party pages (fzt-showcase,
	// my-homepage, etc.) can consume the stream.
	mux.HandleFunc("/snapshot", cors(serveSharedSnapshot))
	mux.HandleFunc("/events", cors(serveSharedEvents))
	// Entropy intake — clients POST keystroke-derived bytes here; bytes
	// get folded into the shared atmosphere's RNG.
	mux.HandleFunc("/entropy", cors(serveEntropy))
}

func registerDevRoutes(mux *http.ServeMux) {
	// Dev atmospheres (per-session)
	mux.HandleFunc("/dev/snapshot", serveDevSnapshot)
	mux.HandleFunc("/dev/events", serveDevEvents)
	mux.HandleFunc("/dev/config", serveDevConfig)
	mux.HandleFunc("/dev/trigger/", serveDevTrigger)
}

func serveHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func serveReadyz(ready func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if ready != nil && !ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// cors wraps a handler to send permissive CORS headers. Safe because the
// wrapped endpoints are read-only broadcast streams — no state mutation
// based on request origin, no cookies/auth consulted.
func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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

func writeSnapshotDataFrame(w http.ResponseWriter, flusher http.Flusher, snap snapshotData) error {
	data, _ := json.Marshal(snap)
	return writeCommand(w, flusher, Command{Kind: "snapshot", Tick: snap.Tick, Data: data})
}

func writeSSEComment(w http.ResponseWriter, flusher http.Flusher, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// writeSnapshotFrame encodes an initial snapshot and sends it as the first
// SSE frame for a new subscriber.
func writeSnapshotFrame(w http.ResponseWriter, flusher http.Flusher, a *atmosphere) error {
	return writeSnapshotDataFrame(w, flusher, a.snapshot())
}

func streamAtmosphere(w http.ResponseWriter, req *http.Request, a *atmosphere) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	ch := a.addListener()
	defer a.removeListener(ch)
	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	if err := writeSnapshotFrame(w, flusher, a); err != nil {
		return
	}

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if err := writeSSEComment(w, flusher, "keep-alive"); err != nil {
				return
			}
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
