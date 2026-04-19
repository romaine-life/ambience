// ambience serves the ambient effect sim over SSE plus a demo HTML page at /.
// Single binary, embeds the web assets.
//
// Routes:
//
//	GET /                      — canonical demo page (full-screen canvas)
//	GET /ambience.js           — browser renderer module
//	GET /stream                — SSE stream of the shared global sim (~10 Hz)
//	GET /dev                   — dev page with sliders for tweaking effect params
//	GET /dev/stream?session=X  — SSE stream of a per-session private sim.
//	                             URL-param changes on reconnect UPDATE the
//	                             running sim in place rather than recreating it.
//	POST /dev/trigger/:session/:event
//	                           — fire a discrete event immediately on that
//	                             session's sim (bypasses probability).
//	GET /effects/rain/schema   — JSON knob schema for Rain.
//
// Run from repo root: `go run ./cmd/ambience`, then open http://localhost:8080/.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nelsong6/ambience/sim"
)

const (
	gridW           = 160
	gridH           = 80
	tickRate        = 100 * time.Millisecond // ~10 Hz
	addr            = ":8080"
	devSessionIdle  = 60 * time.Second       // session is GC'd after this long without a listener
	devSessionSweep = 30 * time.Second       // GC pass period
)

//go:embed web
var webFS embed.FS

type filledPixel struct {
	X int   `json:"x"`
	Y int   `json:"y"`
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

type frame struct {
	W      int            `json:"w"`
	H      int            `json:"h"`
	Pixels []filledPixel  `json:"pixels"`
	Log    []sim.LogEntry `json:"log,omitempty"`
}

// broadcaster is a fan-out for the shared global stream.
type broadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *broadcaster) subscribe() chan []byte {
	ch := make(chan []byte, 4)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *broadcaster) broadcast(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

// devSession is a per-sessionID private sim that persists across SSE
// reconnects. When the dev UI tweaks a knob it reconnects with new URL
// params; the existing session picks up the new config without resetting
// drops/splashes/tick state.
type devSession struct {
	mu        sync.Mutex
	sim       *sim.Rain
	listeners map[chan []byte]struct{}
	lastSeen  time.Time
	cancel    context.CancelFunc
}

var devSessions sync.Map // string → *devSession

func getOrCreateDevSession(id string, cfg sim.Config) *devSession {
	if v, ok := devSessions.Load(id); ok {
		s := v.(*devSession)
		s.sim.SetConfig(cfg)
		s.mu.Lock()
		s.lastSeen = time.Now()
		s.mu.Unlock()
		return s
	}
	s := &devSession{
		sim:       sim.NewRain(gridW, gridH, time.Now().UnixNano(), cfg),
		listeners: make(map[chan []byte]struct{}),
		lastSeen:  time.Now(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	actual, loaded := devSessions.LoadOrStore(id, s)
	if loaded {
		// Another goroutine beat us; use theirs, cancel ours.
		cancel()
		other := actual.(*devSession)
		other.sim.SetConfig(cfg)
		return other
	}
	go s.run(ctx)
	return s
}

func (s *devSession) run(ctx context.Context) {
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sim.Step()
			data := snapshotWithLog(s.sim)
			s.mu.Lock()
			for ch := range s.listeners {
				select {
				case ch <- data:
				default:
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *devSession) addListener() chan []byte {
	ch := make(chan []byte, 8)
	s.mu.Lock()
	s.listeners[ch] = struct{}{}
	s.lastSeen = time.Now()
	s.mu.Unlock()
	return ch
}

func (s *devSession) removeListener(ch chan []byte) {
	s.mu.Lock()
	delete(s.listeners, ch)
	s.lastSeen = time.Now()
	s.mu.Unlock()
	close(ch)
}

func sweepDevSessions() {
	t := time.NewTicker(devSessionSweep)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		devSessions.Range(func(k, v interface{}) bool {
			s := v.(*devSession)
			s.mu.Lock()
			empty := len(s.listeners) == 0
			idle := now.Sub(s.lastSeen) > devSessionIdle
			s.mu.Unlock()
			if empty && idle {
				s.cancel()
				devSessions.Delete(k)
			}
			return true
		})
	}
}

func main() {
	shared := sim.NewRain(gridW, gridH, time.Now().UnixNano(), sim.Config{})
	bc := newBroadcaster()

	go sharedTickLoop(shared, bc)
	go sweepDevSessions()

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(web)))
	http.HandleFunc("/stream", func(w http.ResponseWriter, req *http.Request) {
		handleSharedSSE(w, req, bc)
	})
	http.HandleFunc("/dev", func(w http.ResponseWriter, req *http.Request) {
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
	})
	http.HandleFunc("/dev/stream", handleDevSSE)
	http.HandleFunc("/dev/trigger/", handleDevTrigger)
	http.HandleFunc("/effects/rain/schema", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sim.RainSchema())
	})

	log.Printf("ambience listening on %s (grid %dx%d, tick %s)", addr, gridW, gridH, tickRate)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func sharedTickLoop(r *sim.Rain, bc *broadcaster) {
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for range t.C {
		r.Step()
		bc.broadcast(snapshot(r))
	}
}

// snapshot builds a plain frame JSON (no log). Used by the shared global sim.
func snapshot(r *sim.Rain) []byte {
	f := frame{W: r.W, H: r.H, Pixels: []filledPixel{}}
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			p := r.Grid[y][x]
			if !p.Filled {
				continue
			}
			f.Pixels = append(f.Pixels, filledPixel{X: x, Y: y, R: p.C.R, G: p.C.G, B: p.C.B})
		}
	}
	b, err := json.Marshal(f)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// snapshotWithLog builds a frame including drained log entries. Used by dev
// sessions so the UI can show a live event log.
func snapshotWithLog(r *sim.Rain) []byte {
	f := frame{W: r.W, H: r.H, Pixels: []filledPixel{}}
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			p := r.Grid[y][x]
			if !p.Filled {
				continue
			}
			f.Pixels = append(f.Pixels, filledPixel{X: x, Y: y, R: p.C.R, G: p.C.G, B: p.C.B})
		}
	}
	f.Log = r.DrainLog()
	b, err := json.Marshal(f)
	if err != nil {
		return []byte("{}")
	}
	return b
}

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

func handleSharedSSE(w http.ResponseWriter, req *http.Request, bc *broadcaster) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	ch := bc.subscribe()
	defer bc.unsubscribe(ch)
	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleDevSSE connects a listener to the per-session sim. If the session
// doesn't exist yet, it's created from the query params. Otherwise the
// existing sim's config is updated in place (drops/events/timers preserved).
func handleDevSSE(w http.ResponseWriter, req *http.Request) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	sessionID := req.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "session param required", http.StatusBadRequest)
		return
	}
	cfg := parseDevConfig(req)
	s := getOrCreateDevSession(sessionID, cfg)
	ch := s.addListener()
	defer s.removeListener(ch)

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleDevTrigger fires an event on a session's sim immediately, bypassing
// probability. URL form: /dev/trigger/<sessionID>/<event>.
func handleDevTrigger(w http.ResponseWriter, req *http.Request) {
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
	v, ok := devSessions.Load(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	s := v.(*devSession)
	if !s.sim.TriggerEvent(event) {
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
