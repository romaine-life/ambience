// ambience serves the ambient-effect coordination service. Clients run
// their own local sim; the server's job is to broadcast state changes
// (config + discrete events) that the clients apply.
//
// Routes:
//
//	GET  /                      — canonical demo page (browser runs a JS sim)
//	GET  /ambience.js           — shared renderer / SSE helpers
//	GET  /sim.js                — JS port of sim/rain.go (runs in browser)
//	GET  /controls.js           — shared schema-driven control panel helper
//	GET  /snapshot              — shared atmosphere init payload (JSON)
//	GET  /events                — shared atmosphere SSE command stream
//	POST /config?effect=&...    — mutate the shared atmosphere config
//	POST /trigger/:event        — fire a discrete event on the shared atmosphere
//	GET  /dev                   — dev page with knob controls (defaults to rain)
//	GET  /dev/<effect>          — effect-specific dev page (e.g. /dev/fireflies)
//	GET  /dev/snapshot?session=&effect=
//	GET  /dev/events?session=&effect=
//	POST /dev/config?session=&effect=
//	POST /dev/randomize?session=&effect=
//	POST /dev/trigger/:session/:event?effect=
//	                            — fire a discrete event on the dev atmosphere
//	GET  /effects/<effect>/schema — JSON schema for the dev knob panel
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
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
	role           appRole
	addr           string
	authorityURL   string
	shutdownDrain  time.Duration
	webOverrideDir string
}

type lifecycleState struct {
	draining atomic.Bool
}

type effectLookup func(effect string) (bool, error)

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
	static := newStaticAssets(web, cfg.webOverrideDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lifecycle := &lifecycleState{}

	mux := http.NewServeMux()
	baseReady := func() bool { return true }

	switch cfg.role {
	case roleAll, roleAuthority:
		if err := bootAuthority(ctx); err != nil {
			log.Fatal(err)
		}
		registerStaticRoutes(mux, static, localEffectLookup)
		registerSchemaRoute(mux)
		registerAuthorityRoutes(mux)
		registerDevRoutes(mux)
	case roleEdge:
		proxy, err := newAuthorityProxy(ctx, cfg.authorityURL)
		if err != nil {
			log.Fatal(err)
		}
		baseReady = proxy.ready
		registerStaticRoutes(mux, static, proxy.effectSchemaExists)
		registerEdgeRoutes(mux, proxy)
	default:
		log.Fatalf("unsupported ambience role %q", cfg.role)
	}
	registerCommonRoutes(mux, func() bool {
		return !lifecycle.draining.Load() && baseReady()
	})

	// AMBIENCE_ADDR overrides the bind address. For local dev, set it to
	// "127.0.0.1:8080" so Windows Firewall doesn't prompt (loopback skips
	// the firewall). Kubernetes keeps the default ":8080" for pod reachability.
	log.Printf("ambience listening on %s (role=%s, grid %dx%d, tick %s)", cfg.addr, cfg.role, gridW, gridH, tickRate)
	srv := &http.Server{Addr: cfg.addr, Handler: mux}
	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatal(err)
		}
	case <-sigCtx.Done():
		lifecycle.draining.Store(true)
		if cfg.shutdownDrain > 0 {
			log.Printf("ambience draining for %s before shutdown", cfg.shutdownDrain)
			time.Sleep(cfg.shutdownDrain)
		}
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		if err := <-serverErr; err != nil {
			log.Fatal(err)
		}
	}
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
	if rawDrain := strings.TrimSpace(os.Getenv("AMBIENCE_SHUTDOWN_DRAIN")); rawDrain != "" {
		d, err := time.ParseDuration(rawDrain)
		if err != nil {
			return appConfig{}, fmt.Errorf("parse AMBIENCE_SHUTDOWN_DRAIN: %w", err)
		}
		if d < 0 {
			return appConfig{}, fmt.Errorf("AMBIENCE_SHUTDOWN_DRAIN must be >= 0")
		}
		cfg.shutdownDrain = d
	}
	cfg.webOverrideDir = strings.TrimSpace(os.Getenv("AMBIENCE_WEB_OVERRIDE_DIR"))
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
	go sweepDevSessions()
	return nil
}

func registerCommonRoutes(mux *http.ServeMux, ready func() bool) {
	mux.HandleFunc("/healthz", serveHealthz)
	mux.HandleFunc("/readyz", serveReadyz(ready))
}

func registerStaticRoutes(mux *http.ServeMux, static staticAssets, lookup effectLookup) {
	handler := serveDevPageWithEffectLookup(static, lookup)
	mux.HandleFunc("/dev", handler)
	mux.HandleFunc("/dev/", handler)
	// /sim.js is sim.js (namespace + helpers + ProceduralScene base)
	// concatenated with every web/effects/*.js file. New effects drop a
	// file in web/effects/ and get picked up on the next request — no
	// shared file gets edited. See serveSimBundle.
	mux.HandleFunc("/sim.js", serveSimBundle(static))
	mux.HandleFunc("/controls.js", serveStaticFile(static, "controls.js"))
	mux.HandleFunc("/client.js", serveStaticFile(static, "client.js"))
	mux.HandleFunc("/ambience.js", serveStaticFile(static, "ambience.js"))
	mux.HandleFunc("/", serveExactStaticFile(static, "/", "index.html"))
}

func registerSchemaRoute(mux *http.ServeMux) {
	mux.HandleFunc("/effects/", cors(serveEffectSchema))
}

func registerAuthorityRoutes(mux *http.ServeMux) {
	// Shared atmosphere — CORS-enabled so third-party pages (fzt-showcase,
	// my-homepage, etc.) can consume the stream.
	mux.HandleFunc("/snapshot", cors(serveSharedSnapshot))
	mux.HandleFunc("/events", cors(serveSharedEvents))
	// Shared live controls stay same-origin only. They intentionally do not
	// opt into permissive CORS because they mutate the shared atmosphere.
	mux.HandleFunc("/config", serveSharedConfig)
	mux.HandleFunc("/trigger/", serveSharedTrigger)
	// Entropy intake — clients POST keystroke-derived bytes here; bytes
	// get folded into the shared atmosphere's RNG.
	mux.HandleFunc("/entropy", cors(serveEntropy))
}

func registerDevRoutes(mux *http.ServeMux) {
	// Dev atmospheres (per-session)
	mux.HandleFunc("/dev/snapshot", serveDevSessionSnapshot)
	mux.HandleFunc("/dev/events", serveDevSessionEvents)
	mux.HandleFunc("/dev/config", serveDevSessionConfig)
	mux.HandleFunc("/dev/randomize", serveDevSessionRandomize)
	mux.HandleFunc("/dev/trigger/", serveDevSessionTrigger)
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

func localEffectLookup(effect string) (bool, error) {
	_, ok := schemaForEffect(effect)
	return ok, nil
}

func serveDevPage(static staticAssets) http.HandlerFunc {
	return serveDevPageWithEffectLookup(static, localEffectLookup)
}

func serveDevPageWithEffectLookup(static staticAssets, lookup effectLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		effect, ok := devPageEffectCandidateFromPath(req.URL.Path)
		if !ok {
			http.NotFound(w, req)
			return
		}
		if lookup != nil {
			exists, err := lookup(effect)
			if err != nil {
				http.Error(w, "effect lookup unavailable", http.StatusServiceUnavailable)
				return
			}
			if !exists {
				http.NotFound(w, req)
				return
			}
		}
		data, err := static.readFile("dev.html")
		if err != nil {
			http.Error(w, "dev page not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}
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
	if cmd.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", cmd.ID); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSnapshotDataFrame(w http.ResponseWriter, flusher http.Flusher, snap snapshotData, id string) error {
	data, _ := json.Marshal(snap)
	return writeCommand(w, flusher, Command{ID: id, Kind: "snapshot", Tick: snap.Tick, Data: data})
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
	return writeSnapshotDataFrame(w, flusher, a.snapshot(), a.currentCommandID())
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
