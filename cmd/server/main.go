// ambience-server serves the ambient effect sim over SSE plus a demo HTML
// page at /. Single binary, embeds the web assets.
//
// Routes:
//
//	GET /              — canonical demo page (full-screen canvas, shared world)
//	GET /ambience.js   — browser renderer module
//	GET /stream        — SSE stream of the shared global sim (~10 Hz)
//	GET /dev           — dev page with sliders for tweaking effect parameters
//	GET /dev/stream    — SSE stream of a PRIVATE sim configured via query params,
//	                     isolated from the shared global state
//
// Dev-stream query params (all optional, sensible defaults):
//
//	wind=0.0       Wind (continuous spectrum, typical range [-2, 2])
//	spawn=N        SpawnEvery (new drop every 1/N probability)
//	fade=0.65      FadeFactor (0..1)
//	hue=210        Hue (base hue in degrees [0, 360))
//
// Run from repo root: `go run ./cmd/server`, then open http://localhost:8080/.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/nelsong6/ambience/sim"
)

const (
	gridW    = 160
	gridH    = 80
	tickRate = 100 * time.Millisecond // ~10 Hz
	addr     = ":8080"
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
	W      int           `json:"w"`
	H      int           `json:"h"`
	Pixels []filledPixel `json:"pixels"`
}

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
		default: // drop frame for slow clients rather than block
		}
	}
}

func main() {
	shared := sim.NewRain(gridW, gridH, time.Now().UnixNano(), sim.Config{})
	bc := newBroadcaster()

	go sharedTickLoop(shared, bc)

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(web)))
	http.HandleFunc("/stream", func(w http.ResponseWriter, req *http.Request) {
		handleSharedSSE(w, req, bc)
	})
	// Serve the dev page at a clean /dev URL (not /dev.html) by reading the
	// embedded file and streaming it as HTML.
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

func snapshot(r *sim.Rain) []byte {
	f := frame{W: r.W, H: r.H, Pixels: []filledPixel{}}
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			p := r.Grid[y][x]
			if !p.Filled {
				continue
			}
			f.Pixels = append(f.Pixels, filledPixel{
				X: x, Y: y,
				R: p.C.R, G: p.C.G, B: p.C.B,
			})
		}
	}
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

// handleDevSSE runs a private sim for this single connection, configured via
// query params. When the client disconnects, the sim is garbage-collected.
func handleDevSSE(w http.ResponseWriter, req *http.Request) {
	flusher, ok := sseHeaders(w)
	if !ok {
		return
	}
	cfg := parseDevConfig(req)
	r := sim.NewRain(gridW, gridH, time.Now().UnixNano(), cfg)

	ctx := req.Context()
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Step()
			fmt.Fprintf(w, "data: %s\n\n", snapshot(r))
			flusher.Flush()
		}
	}
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
