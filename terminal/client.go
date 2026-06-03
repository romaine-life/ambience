// Package terminal is a rain-only ambience client for terminal-resident
// consumers (e.g., fzt-automate). It subscribes to an ambience server's SSE
// command stream, runs a local sim replica behind the authority clock, and
// emits sixel output via Render.
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sixel"
	"github.com/romaine-life/ambience/sim"
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
	// DelayTicks is the authority-clock playback delay. Defaults to 50 ticks,
	// matching the browser client.
	DelayTicks int
}

// snapshotWire matches the server's generic snapshot envelope. The terminal
// client still only knows how to render Rain, but it now consumes the
// effect-generic outer shape and decodes Rain's nested state blob.
type snapshotWire struct {
	Type   string          `json:"type"`
	Tick   int             `json:"tick"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
}

type commandWire struct {
	Kind  string          `json:"kind"`
	Tick  int             `json:"tick"`
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type queuedCommand struct {
	cmd  commandWire
	data json.RawMessage
}

type playbackClock struct {
	tickRate          time.Duration
	delayTicks        int
	softCatchupDrift  int
	hardCatchupDrift  int
	maxCatchupSteps   int
	authorityTick     int
	authoritySampleAt time.Time
	haveSample        bool
	now               func() time.Time
}

func newPlaybackClock(tickRate time.Duration, delayTicks int) playbackClock {
	if tickRate <= 0 {
		tickRate = 100 * time.Millisecond
	}
	if delayTicks < 0 {
		delayTicks = 0
	}
	return playbackClock{
		tickRate:         tickRate,
		delayTicks:       delayTicks,
		softCatchupDrift: 20,
		hardCatchupDrift: 100,
		maxCatchupSteps:  5,
		now:              time.Now,
	}
}

func (p *playbackClock) noteAuthorityTick(tick int, sampleAt time.Time) {
	p.authorityTick = tick
	if sampleAt.IsZero() {
		sampleAt = p.now()
	}
	p.authoritySampleAt = sampleAt
	p.haveSample = true
}

func (p *playbackClock) estimatedAuthorityTick(fallback int) int {
	if !p.haveSample {
		if fallback < 0 {
			return 0
		}
		return fallback
	}
	elapsed := 0
	if p.tickRate > 0 {
		elapsed = int(p.now().Sub(p.authoritySampleAt) / p.tickRate)
	}
	if elapsed < 0 {
		elapsed = 0
	}
	return p.authorityTick + elapsed
}

func (p *playbackClock) targetPlaybackTick(fallback int) int {
	target := p.estimatedAuthorityTick(fallback) - p.delayTicks
	if target < 0 {
		return 0
	}
	return target
}

func (p *playbackClock) stepsFor(currentTick int) int {
	target := p.targetPlaybackTick(currentTick)
	drift := target - currentTick
	if drift <= 0 {
		return 0
	}
	if drift > p.hardCatchupDrift {
		return p.maxCatchupSteps
	}
	if drift > 1 {
		if p.maxCatchupSteps < 2 {
			return p.maxCatchupSteps
		}
		return 2
	}
	return 1
}

// DebugState is a point-in-time view of the terminal client's rain-only
// authority-clock playback state.
type DebugState struct {
	EffectType            string `json:"effectType"`
	RainOnly              bool   `json:"rainOnly"`
	Ready                 bool   `json:"ready"`
	AuthorityTick         int    `json:"authorityTick"`
	PlaybackTick          int    `json:"playbackTick"`
	SimTick               int    `json:"simTick"`
	DriftTicks            int    `json:"driftTicks"`
	DelayTicks            int    `json:"delayTicks"`
	BufferedAheadTicks    int    `json:"bufferedAheadTicks"`
	TickMs                int    `json:"tickMs"`
	QueuedCommands        int    `json:"queuedCommands"`
	NextQueuedCommandTick *int   `json:"nextQueuedCommandTick"`
	MaxQueuedCommandTick  *int   `json:"maxQueuedCommandTick"`
	HaveAuthoritySample   bool   `json:"haveAuthoritySample"`
	LastError             string `json:"lastError,omitempty"`
}

// Reference grid dims for which the atmosphere's default config is tuned.
// Clients with grids much larger than this scale Speed + SpawnBurst so a
// full-surface overlay doesn't look like a sparse drizzle.
const (
	refGridW = 40
	refGridH = 30
)

// Client is an ambience subscriber + local sim + sixel renderer.
type Client struct {
	cfg Config

	// The local sim replica. All methods used here are internally synchronized
	// by sim.Rain.
	sim *sim.Rain

	// Last config received from the server (unscaled). We keep it so we can
	// re-apply with updated scale factors when the grid resizes or a new
	// config message arrives. Guarded by configMu.
	configMu      sync.Mutex
	baseConfig    sim.Config
	baseConfigSet bool

	// Most recent grid snapshot. Produced by the tick goroutine after each
	// Step via sim.GridCopy(); read by Render.
	gridMu sync.Mutex
	grid   [][]sim.Pixel

	cancel context.CancelFunc

	stateMu       sync.Mutex
	ready         bool
	effectType    string
	lastError     string
	pending       []queuedCommand
	playbackClock playbackClock

	// Render-time scratch buffers.
	renderMu sync.Mutex
	img      *image.RGBA
	buf      bytes.Buffer
}

// applyScaledConfig applies baseConfig to the sim, scaling Speed by the
// ratio of current grid height to refGridH (so drops fall top-to-bottom
// in roughly the same wall-clock time regardless of canvas height), and
// SpawnBurst by the ratio of grid width to refGridW (so horizontal drop
// density stays roughly constant).
//
// Caller must hold configMu.
func (c *Client) applyScaledConfig() {
	if !c.baseConfigSet {
		return
	}
	cfg := c.baseConfig
	hScale := float64(c.cfg.GridH) / float64(refGridH)
	wScale := float64(c.cfg.GridW) / float64(refGridW)
	if hScale > 1 {
		cfg.Speed *= hScale
	}
	if wScale > 1 {
		// Scale burst instead of SpawnEvery so density grows smoothly
		// rather than in step changes.
		scaled := int(float64(cfg.SpawnBurst) * wScale)
		if scaled < 1 {
			scaled = 1
		}
		cfg.SpawnBurst = scaled
	}
	c.sim.SetConfig(cfg)
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
	if cfg.DelayTicks <= 0 {
		cfg.DelayTicks = 50
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
		cfg:           cfg,
		sim:           sim.NewRain(cfg.GridW, cfg.GridH, time.Now().UnixNano(), sim.Config{}),
		grid:          grid,
		img:           image.NewRGBA(image.Rect(0, 0, cfg.GridW, cfg.GridH)),
		playbackClock: newPlaybackClock(cfg.TickRate, cfg.DelayTicks),
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

// Resize reconfigures the sim + render buffers for a new grid size. Safe to
// call concurrently with tickLoop / Render; the next Step will produce a
// grid at the new dimensions. Also re-applies the scaled config so Speed /
// SpawnBurst track the new canvas size.
func (c *Client) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	c.renderMu.Lock()
	if w == c.cfg.GridW && h == c.cfg.GridH {
		c.renderMu.Unlock()
		return
	}
	c.cfg.GridW = w
	c.cfg.GridH = h
	c.sim.Resize(w, h)
	c.img = image.NewRGBA(image.Rect(0, 0, w, h))
	newGrid := make([][]sim.Pixel, h)
	for i := range newGrid {
		newGrid[i] = make([]sim.Pixel, w)
	}
	c.gridMu.Lock()
	c.grid = newGrid
	c.gridMu.Unlock()
	c.renderMu.Unlock()

	c.configMu.Lock()
	c.applyScaledConfig()
	c.configMu.Unlock()
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
	c.stateMu.Lock()
	c.playbackClock.noteAuthorityTick(cmd.Tick, time.Time{})
	c.stateMu.Unlock()

	data := cmd.Data
	switch cmd.Kind {
	case "snapshot":
		c.applyCommandNow(cmd, data)
	case "clock":
		return
	case "config", "trigger":
		c.queueCommand(cmd, data)
	default:
		return
	}
}

func (c *Client) queueCommand(cmd commandWire, data json.RawMessage) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.pending = append(c.pending, queuedCommand{cmd: cmd, data: append(json.RawMessage(nil), data...)})
	sort.SliceStable(c.pending, func(i, j int) bool {
		return c.pending[i].cmd.Tick < c.pending[j].cmd.Tick
	})
}

func (c *Client) applyDueCommands(playbackTick int) {
	var due []queuedCommand
	c.stateMu.Lock()
	for len(c.pending) > 0 {
		tick := c.pending[0].cmd.Tick
		if tick > playbackTick {
			break
		}
		due = append(due, c.pending[0])
		c.pending = c.pending[1:]
	}
	c.stateMu.Unlock()
	for _, item := range due {
		c.applyCommandNow(item.cmd, item.data)
	}
}

func (c *Client) applyCommandNow(cmd commandWire, data json.RawMessage) {
	switch cmd.Kind {
	case "snapshot":
		var snap snapshotWire
		if err := json.Unmarshal(data, &snap); err != nil {
			c.reportError(err)
			return
		}
		if snap.Type != "" && snap.Type != "rain" {
			c.reportError(fmt.Errorf("terminal ambience client does not support effect type %q", snap.Type))
			return
		}
		var cfg sim.Config
		if len(snap.Config) > 0 {
			if err := json.Unmarshal(snap.Config, &cfg); err != nil {
				c.reportError(err)
				return
			}
		}
		var state sim.RainSnapshot
		if len(snap.State) > 0 {
			if err := json.Unmarshal(snap.State, &state); err != nil {
				c.reportError(err)
				return
			}
		}
		c.configMu.Lock()
		c.baseConfig = cfg
		c.baseConfigSet = true
		c.applyScaledConfig()
		c.configMu.Unlock()
		c.sim.RestoreSnapshot(state)
		c.stateMu.Lock()
		c.ready = true
		c.effectType = "rain"
		c.lastError = ""
		c.discardQueuedThroughLocked(snap.Tick)
		c.stateMu.Unlock()
	case "config":
		var cfg sim.Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			c.reportError(err)
			return
		}
		c.configMu.Lock()
		c.baseConfig = cfg
		c.baseConfigSet = true
		c.applyScaledConfig()
		c.configMu.Unlock()
	case "trigger":
		c.sim.TriggerEvent(cmd.Event)
	}
}

func (c *Client) discardQueuedThroughLocked(tick int) {
	if len(c.pending) == 0 {
		return
	}
	keep := c.pending[:0]
	for _, item := range c.pending {
		if item.cmd.Tick > tick {
			keep = append(keep, item)
		}
	}
	c.pending = keep
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
			c.stepTowardAuthorityClock()
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

func (c *Client) stepTowardAuthorityClock() {
	current := c.sim.CurrentTick()
	c.stateMu.Lock()
	ready := c.ready
	steps := c.playbackClock.stepsFor(current)
	c.stateMu.Unlock()
	if !ready {
		return
	}
	if steps <= 0 {
		c.applyDueCommands(current)
		return
	}
	for i := 0; i < steps; i++ {
		c.applyDueCommands(c.sim.CurrentTick() + 1)
		c.sim.Step()
	}
	c.applyDueCommands(c.sim.CurrentTick())
}

// DebugState returns browser-comparable sync telemetry for the rain-only
// terminal client.
func (c *Client) DebugState() DebugState {
	simTick := c.sim.CurrentTick()
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	var nextTick *int
	var maxTick *int
	for _, item := range c.pending {
		tick := item.cmd.Tick
		if nextTick == nil || tick < *nextTick {
			v := tick
			nextTick = &v
		}
		if maxTick == nil || tick > *maxTick {
			v := tick
			maxTick = &v
		}
	}
	authorityTick := c.playbackClock.estimatedAuthorityTick(simTick)
	playbackTick := c.playbackClock.targetPlaybackTick(simTick)
	bufferedAhead := 0
	if maxTick != nil {
		bufferedAhead = *maxTick - playbackTick
		if bufferedAhead < 0 {
			bufferedAhead = 0
		}
	}
	return DebugState{
		EffectType:            c.effectType,
		RainOnly:              true,
		Ready:                 c.ready,
		AuthorityTick:         authorityTick,
		PlaybackTick:          playbackTick,
		SimTick:               simTick,
		DriftTicks:            playbackTick - simTick,
		DelayTicks:            c.playbackClock.delayTicks,
		BufferedAheadTicks:    bufferedAhead,
		TickMs:                int(c.cfg.TickRate / time.Millisecond),
		QueuedCommands:        len(c.pending),
		NextQueuedCommandTick: nextTick,
		MaxQueuedCommandTick:  maxTick,
		HaveAuthoritySample:   c.playbackClock.haveSample,
		LastError:             c.lastError,
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
				// Alpha=0 → mattn/go-sixel encoder skips this pixel (emits
				// no bit in the sixel data). Combined with Pb=1 in the DCS
				// header (set below), this leaves the underlying terminal
				// content untouched wherever there's no drop.
				c.img.Set(x, y, color.RGBA{0, 0, 0, 0})
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
	// The mattn encoder hardcodes the DCS introducer as "\x1bP0;0;8q", which
	// sets Pb=0 (unset pixels painted with background color — the black
	// rectangle). Rewrite in-place to Pb=1 so unset pixels leave the existing
	// terminal content untouched (wallpaper / TUI bg shows through).
	sixelBytes := c.buf.Bytes()
	const oldHeader = "\x1bP0;0;8q"
	const newHeader = "\x1bP0;1;8q"
	if bytes.HasPrefix(sixelBytes, []byte(oldHeader)) {
		copy(sixelBytes[:len(newHeader)], []byte(newHeader))
	}
	if _, err := fmt.Fprint(w, "\x1b[s"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "\x1b[%d;%dH", row, col); err != nil {
		return err
	}
	if _, err := w.Write(sixelBytes); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "\x1b[u")
	return err
}

func (c *Client) reportError(err error) {
	if err == nil {
		return
	}
	c.stateMu.Lock()
	c.lastError = err.Error()
	c.stateMu.Unlock()
	if c.cfg.OnError != nil {
		c.cfg.OnError(err)
		return
	}
	// Silent by default. Consumers can opt in via OnError.
	_ = log.Output // keep log imported so OnError users can pair with log.Printf
}
