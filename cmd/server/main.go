// ambience-server serves the ambient effect sim over SSE plus a demo HTML
// page at /. Single binary, embeds the web assets.
//
// Routes:
//
//	GET /              — demo HTML page (full-screen canvas)
//	GET /ambience.js   — browser renderer module
//	GET /stream        — SSE stream of grid snapshots (~10 Hz)
//
// Run from repo root: `go run ./cmd/server` then open http://localhost:8080/.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	r := sim.NewRain(gridW, gridH, time.Now().UnixNano())
	bc := newBroadcaster()

	go tickLoop(r, bc)

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(web)))
	http.HandleFunc("/stream", func(w http.ResponseWriter, req *http.Request) {
		handleSSE(w, req, bc)
	})

	log.Printf("ambience listening on %s (grid %dx%d, tick %s)", addr, gridW, gridH, tickRate)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func tickLoop(r *sim.Rain, bc *broadcaster) {
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

func handleSSE(w http.ResponseWriter, req *http.Request, bc *broadcaster) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

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
