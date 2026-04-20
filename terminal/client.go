// Package terminal is an ambience client for terminal-resident consumers
// (e.g., fzt-automate). It subscribes to an ambience server's SSE command
// stream, runs a local Rain sim replica, and emits sixel output via Render.
//
// Usage:
//
//	c := terminal.New(terminal.Config{ServerURL: "https://ambience.romaine.life"})
//	c.Start(ctx)
//	defer c.Stop()
//
//	// inside your render loop, after tcell.Screen.Show():
//	c.Render(os.Stdout, col, row)  // 1-based cell coords
package terminal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sixel"
	"github.com/nelsong6/ambience/sim"
)

// Config tunes a Client.
type Config struct {
	// ServerURL is the base URL of the ambience server, e.g.
	// "https://ambience.romaine.life" or "http://localhost:8080".
	ServerURL string
	// GridW, GridH control the local sim's grid dimensions in pixels. Larger
	// grids = more drops + wider visible strip. Defaults suit a narrow strip.
	GridW, GridH int
	// TickRate — how often the local sim advances. Defaults to 100ms (10Hz),
	// matching the server's atmosphere tick.
	TickRate time.Duration
	// OnError is an optional callback for logging transient errors (e.g.
	// disconnected SSE). If nil, errors are silent.
	OnError func(err error)
	// RecordDir, if non-empty, is a directory where the client will save a
	// PNG of the grid every RecordEvery ticks. Useful for debugging without
	// a TTY — a headless consumer can run the client and inspect what the
	// sim is producing frame-by-frame.
	RecordDir string
	// RecordEvery — save one frame every N ticks. 0 disables recording
	// even when RecordDir is set.
	RecordEvery int
}

// snapshotWire matches the server's snapshotData struct (see atmosphere.go).
// Only the subset the client needs is declared.
type snapshotWire struct {
	Tick         int        `json:"tick"`
	Config       sim.Config `json:"config"`
	Seed         int64      `json:"seed"`
	DownpourLeft int        `json:"downpourLeft"`
	DownpourMult float64    `json:"downpourMult"`
	CalmLeft     int        `json:"calmLeft"`
	GustLeft     int        `json:"gustLeft"`
	GustWind     float64    `json:"gustWind"`
}

type commandWire struct {
	Kind  string          `json:"kind"`
	Tick  int             `json:"tick"`
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Client is an ambience subscriber + local sim + sixel renderer.
type Client struct {
	cfg Config

	// The local sim replica. Owned by the tick goroutine; other goroutines
	// only call its thread-safe methods (SetConfig, TriggerEvent, RestoreState).
	sim *sim.Rain

	// Most recent grid snapshot. Produced by the tick goroutine after each
	// Step via sim.GridCopy(); read by Render.
	gridMu sync.Mutex
	grid   [][]sim.Pixel

	cancel context.CancelFunc

	// Render-time scratch buffers.
	renderMu sync.Mutex
	img      *image.RGBA
	buf      bytes.Buffer
}

// New builds a Client. Apply defaults for any zero fields in cfg.
func New(cfg Config) *Client {
	if cfg.GridW <= 0 {
		cfg.GridW = 160
	}
	if cfg.GridH <= 0 {
		cfg.GridH = 20
	}
	if cfg.TickRate <= 0 {
		cfg.TickRate = 100 * time.Millisecond
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "https://ambience.romaine.life"
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	grid := make([][]sim.Pixel, cfg.GridH)
	for i := range grid {
		grid[i] = make([]sim.Pixel, cfg.GridW)
	}

	return &Client{
		cfg:  cfg,
		sim:  sim.NewRain(cfg.GridW, cfg.GridH, time.Now().UnixNano(), sim.Config{}),
		grid: grid,
		img:  image.NewRGBA(image.Rect(0, 0, cfg.GridW, cfg.GridH)),
	}
}

// Start launches the SSE subscriber + local tick loop goroutines. Returns
// immediately. Call Stop() or cancel the parent context to shut down.
func (c *Client) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	go c.subscribeLoop(ctx)
	go c.tickLoop(ctx)
}

// Stop cancels the Client's goroutines.
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// subscribeLoop maintains an SSE connection to the server, reconnecting with
// exponential backoff on errors. Each streamed command is applied to the sim.
func (c *Client) subscribeLoop(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.runOneConnection(ctx)
		if ctx.Err() != nil {
			return
		}
		c.reportError(err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) runOneConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.ServerURL+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ambience /events: HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<16), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		c.applyCommand(strings.TrimPrefix(line, "data: "))
	}
	return scanner.Err()
}

func (c *Client) applyCommand(payload string) {
	var cmd commandWire
	if err := json.Unmarshal([]byte(payload), &cmd); err != nil {
		c.reportError(err)
		return
	}
	switch cmd.Kind {
	case "snapshot":
		var snap snapshotWire
		if err := json.Unmarshal(cmd.Data, &snap); err != nil {
			c.reportError(err)
			return
		}
		c.sim.SetConfig(snap.Config)
		c.sim.RestoreState(sim.State{
			Tick:          snap.Tick,
			DownpourTicks: snap.DownpourLeft,
			DownpourMult:  snap.DownpourMult,
			CalmTicks:     snap.CalmLeft,
			GustTicks:     snap.GustLeft,
			GustWind:      snap.GustWind,
		})
	case "config":
		var cfg sim.Config
		if err := json.Unmarshal(cmd.Data, &cfg); err != nil {
			c.reportError(err)
			return
		}
		c.sim.SetConfig(cfg)
	case "trigger":
		c.sim.TriggerEvent(cmd.Event)
	}
}

// tickLoop advances the local sim + refreshes the grid snapshot for Render.
func (c *Client) tickLoop(ctx context.Context) {
	t := time.NewTicker(c.cfg.TickRate)
	defer t.Stop()
	tick := 0
	if c.cfg.RecordDir != "" {
		_ = os.MkdirAll(c.cfg.RecordDir, 0o755)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.sim.Step()
			snap := c.sim.GridCopy()
			c.gridMu.Lock()
			c.grid = snap
			c.gridMu.Unlock()
			tick++
			if c.cfg.RecordDir != "" && c.cfg.RecordEvery > 0 && tick%c.cfg.RecordEvery == 0 {
				if err := c.writeFramePNG(tick, snap); err != nil {
					c.reportError(err)
				}
			}
		}
	}
}

// writeFramePNG saves a snapshot grid to RecordDir/frame_NNNNNN.png so headless
// consumers (and future Claude self-debugging) can see what the sim produced
// without a TTY. Empty cells render as black, filled cells as their color.
func (c *Client) writeFramePNG(tick int, grid [][]sim.Pixel) error {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.GridW, c.cfg.GridH))
	for y := 0; y < c.cfg.GridH && y < len(grid); y++ {
		row := grid[y]
		for x := 0; x < c.cfg.GridW && x < len(row); x++ {
			p := row[x]
			if p.Filled {
				img.Set(x, y, p.C)
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	path := filepath.Join(c.cfg.RecordDir, fmt.Sprintf("frame_%06d.png", tick))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// Render emits a sixel image of the current grid at (col, row) (1-based
// terminal cell coordinates). Wraps the output in CSI s/u cursor guards so
// the caller's cursor isn't disturbed.
//
// Returns early (no bytes written) if no snapshot has been received yet.
func (c *Client) Render(w io.Writer, col, row int) error {
	c.gridMu.Lock()
	grid := c.grid
	c.gridMu.Unlock()
	if len(grid) == 0 {
		return nil
	}

	c.renderMu.Lock()
	defer c.renderMu.Unlock()
	anyFilled := false
	for y := 0; y < c.cfg.GridH && y < len(grid); y++ {
		row := grid[y]
		for x := 0; x < c.cfg.GridW && x < len(row); x++ {
			p := row[x]
			if p.Filled {
				c.img.Set(x, y, p.C)
				anyFilled = true
			} else {
				c.img.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	if !anyFilled {
		// Empty scene — don't bother emitting sixel.
		return nil
	}

	c.buf.Reset()
	enc := sixel.NewEncoder(&c.buf)
	if err := enc.Encode(c.img); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "\x1b[s"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "\x1b[%d;%dH", row, col); err != nil {
		return err
	}
	if _, err := w.Write(c.buf.Bytes()); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "\x1b[u")
	return err
}

func (c *Client) reportError(err error) {
	if err == nil {
		return
	}
	if c.cfg.OnError != nil {
		c.cfg.OnError(err)
		return
	}
	// Silent by default. Consumers can opt in via OnError.
	_ = log.Output // keep log imported so OnError users can pair with log.Printf
}
