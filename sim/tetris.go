package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// Tetris is an ambient tetromino effect. Pieces descend slowly into a fixed
// playfield and settle on contact; lines never clear, so the well gradually
// fills until a new piece can't spawn — the natural lose-state ends the
// scene before the well resets via the intro lifecycle. Lateral movement
// and player-controlled rotation are intentionally absent: each piece picks
// a random column + rotation at spawn time and falls straight down.
//
// Authority and browser sims must agree on the piece sequence; both use the
// shared mulberry32 helper seeded from RNGState so the queue stays in sync
// across the SSE snapshot/config/trigger envelope.

// tetrisBoardW / tetrisBoardH are the default playfield dimensions.
// Classic Tetris is 10 × 20; we keep that for the unmistakable read.
const (
	tetrisBoardW = 10
	tetrisBoardH = 20
)

// Piece type codes. 0 is reserved for empty cells in the board grid.
const (
	tetrisPieceI byte = 1 + iota
	tetrisPieceO
	tetrisPieceT
	tetrisPieceS
	tetrisPieceZ
	tetrisPieceJ
	tetrisPieceL
)

// tetrisShapes lists the (row, col) offsets of each tetromino's filled cells
// per rotation. Anchor is the top-left of the piece's bounding box.
var tetrisShapes = [8][][][2]int{
	tetrisPieceI: {
		{{0, 0}, {0, 1}, {0, 2}, {0, 3}},
		{{0, 0}, {1, 0}, {2, 0}, {3, 0}},
	},
	tetrisPieceO: {
		{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
	},
	tetrisPieceT: {
		{{0, 0}, {0, 1}, {0, 2}, {1, 1}},
		{{0, 0}, {1, 0}, {1, 1}, {2, 0}},
		{{0, 1}, {1, 0}, {1, 1}, {1, 2}},
		{{0, 1}, {1, 0}, {1, 1}, {2, 1}},
	},
	tetrisPieceS: {
		{{0, 1}, {0, 2}, {1, 0}, {1, 1}},
		{{0, 0}, {1, 0}, {1, 1}, {2, 1}},
	},
	tetrisPieceZ: {
		{{0, 0}, {0, 1}, {1, 1}, {1, 2}},
		{{0, 1}, {1, 0}, {1, 1}, {2, 0}},
	},
	tetrisPieceJ: {
		{{0, 0}, {1, 0}, {1, 1}, {1, 2}},
		{{0, 0}, {0, 1}, {1, 0}, {2, 0}},
		{{0, 0}, {0, 1}, {0, 2}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 0}, {2, 1}},
	},
	tetrisPieceL: {
		{{0, 2}, {1, 0}, {1, 1}, {1, 2}},
		{{0, 0}, {1, 0}, {2, 0}, {2, 1}},
		{{0, 0}, {0, 1}, {0, 2}, {1, 0}},
		{{0, 0}, {0, 1}, {1, 1}, {2, 1}},
	},
}

// Hue offsets per piece type, anchored so cfg.Hue corresponds to the I-piece
// (cyan in the classic palette). Rotating cfg.Hue rotates the whole palette.
var tetrisPieceHues = [8]float64{
	tetrisPieceI: 0,
	tetrisPieceO: -120, // yellow
	tetrisPieceT: 105,  // purple
	tetrisPieceS: -60,  // green
	tetrisPieceZ: 180,  // red
	tetrisPieceJ: 45,   // blue
	tetrisPieceL: -150, // orange
}

// TetrisConfig tunes the slow-Tetris ambient effect.
type TetrisConfig struct {
	// INTRODUCTION
	IntroDur    int     `json:"intro_dur"`
	IntroHeight float64 `json:"intro_h"`
	IntroFirst  int     `json:"intro_first"`
	// ENDING
	EndingDur    int `json:"ending_dur"`
	EndingLinger int `json:"ending_linger"`
	// LEVERS
	BoardW     int     `json:"board_w"`
	BoardH     int     `json:"board_h"`
	FallEvery  int     `json:"fall_every"`
	SpawnPause int     `json:"spawn_pause"`
	LockDelay  int     `json:"lock_delay"`
	Hue        float64 `json:"hue"`
	HueSpread  float64 `json:"hue_sp"`
	Saturation float64 `json:"sat"`
	LightMin   float64 `json:"lmin"`
	LightMax   float64 `json:"lmax"`
	GhostAlpha float64 `json:"ghost"`
	// EVENT CHANCES
	LullChance float64 `json:"lull_p"`
	// EVENT MODIFIERS
	LullDur int `json:"lull_dur"`
	// END CONDITIONS
	FillThreshold float64 `json:"fill_thresh"`
}

func (c TetrisConfig) withDefaults() TetrisConfig {
	if c.IntroDur == 0 && c.IntroHeight == 0 && c.IntroFirst == 0 {
		c.IntroDur = 60
		c.IntroHeight = 0.0
		c.IntroFirst = 8
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 60
		}
		if c.IntroHeight < 0 {
			c.IntroHeight = 0
		}
		if c.IntroFirst <= 0 {
			c.IntroFirst = 8
		}
	}
	if c.IntroHeight > 0.85 {
		c.IntroHeight = 0.85
	}
	if c.EndingDur == 0 && c.EndingLinger == 0 {
		c.EndingDur = 80
		c.EndingLinger = 60
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 80
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
	}
	if c.BoardW <= 0 {
		c.BoardW = tetrisBoardW
	}
	if c.BoardH <= 0 {
		c.BoardH = tetrisBoardH
	}
	if c.BoardW < 6 {
		c.BoardW = 6
	}
	if c.BoardW > 24 {
		c.BoardW = 24
	}
	if c.BoardH < 10 {
		c.BoardH = 10
	}
	if c.BoardH > 32 {
		c.BoardH = 32
	}
	if c.FallEvery <= 0 {
		c.FallEvery = 14
	}
	if c.SpawnPause <= 0 {
		c.SpawnPause = 18
	}
	if c.LockDelay <= 0 {
		c.LockDelay = 6
	}
	if c.Hue == 0 {
		c.Hue = 200
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.55
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.4
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.66
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.GhostAlpha < 0 {
		c.GhostAlpha = 0
	}
	if c.GhostAlpha > 0.6 {
		c.GhostAlpha = 0.6
	}
	if c.LullChance < 0 {
		c.LullChance = 0
	}
	if c.LullDur <= 0 {
		c.LullDur = 80
	}
	if c.FillThreshold <= 0 {
		c.FillThreshold = 0.85
	}
	if c.FillThreshold > 1 {
		c.FillThreshold = 1
	}
	return c
}

// TetrisSchema describes Tetris's tunable knobs for the dev UI.
func TetrisSchema() EffectSchema {
	return EffectSchema{
		Name: "tetris",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks of the opening before the steady cadence begins."},
			{Key: "intro_h", Label: "starting stack", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 0.85, Step: 0.02, Default: 0,
				Description: "Fraction of the well filled with random debris when the intro fires."},
			{Key: "intro_first", Label: "first wait", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 1, Max: 60, Step: 1, Default: 8,
				Description: "Ticks before the first piece spawns after the intro begins."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks the lose-state holds before the well clears and the intro fires."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 240, Step: 5, Default: 60,
				Description: "Extra still ticks after the lose-state before the well resets."},
			{Key: "fill_thresh", Label: "fill thresh", Slot: SlotEnd, Group: "lose state", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.02, Default: 0.85,
				Description: "Lose when this fraction of the well is filled (in addition to the spawn-blocked check)."},
			{Key: "board_w", Label: "board w", Slot: SlotLever, Group: "well", Type: KnobInt, Min: 6, Max: 24, Step: 1, Default: tetrisBoardW,
				Description: "Width of the playfield in cells."},
			{Key: "board_h", Label: "board h", Slot: SlotLever, Group: "well", Type: KnobInt, Min: 10, Max: 32, Step: 1, Default: tetrisBoardH,
				Description: "Height of the playfield in cells."},
			{Key: "fall_every", Label: "fall 1/", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 1, Max: 80, Step: 1, Default: 14,
				Description: "Ticks between drops of one cell. Higher values make the descent more meditative."},
			{Key: "spawn_pause", Label: "spawn pause", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 1, Max: 240, Step: 1, Default: 18, Trigger: "new-piece",
				Description: "Ticks of stillness between piece settle and the next spawn. Fire to skip the pause and spawn now."},
			{Key: "lock_delay", Label: "lock delay", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 1, Max: 60, Step: 1, Default: 6,
				Description: "Ticks the piece sits visible at its resting position before settling into the well."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 200,
				Description: "Base hue. Each piece type rotates around this anchor by a fixed offset."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 0,
				Description: "Per-piece hue jitter applied around each piece type's anchor offset."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.02, Default: 0.55,
				Description: "Saturation of the piece palette."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.15, Max: 0.75, Step: 0.02, Default: 0.4,
				Description: "Lower bound of piece lightness."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 0.95, Step: 0.02, Default: 0.66,
				Description: "Upper bound of piece lightness."},
			{Key: "ghost", Label: "ghost", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0,
				Description: "Opacity of the resting-position outline drawn ahead of the falling piece."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.001, Default: 0, Trigger: "lull",
				Description: "Per-spawn chance that the next pause is extended into a longer lull."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 30, Max: 600, Step: 5, Default: 80,
				Description: "Ticks added to the spawn pause when a lull fires."},
		},
	}
}

// TetrisActivePiece is the in-flight tetromino. When kind == 0 there is no
// active piece (the sim is in spawn-pause / intro / ending stillness).
type TetrisActivePiece struct {
	Kind     byte    `json:"kind"`
	Rotation int     `json:"rot"`
	Row      int     `json:"row"`
	Col      int     `json:"col"`
	Hue      float64 `json:"hue"`
	Locking  bool    `json:"locking"`
	LockTick int     `json:"lockTick"`
}

type TetrisState struct {
	Tick           int                `json:"tick"`
	BoardW         int                `json:"boardW"`
	BoardH         int                `json:"boardH"`
	Cells          []byte             `json:"cells"`
	Hues           []float64          `json:"hues"`
	Active         *TetrisActivePiece `json:"active,omitempty"`
	NextKind       byte               `json:"nextKind"`
	FallTimer      int                `json:"fallTimer"`
	SpawnPause     int                `json:"spawnPause"`
	LullPause      int                `json:"lullPause"`
	PieceIndex     int                `json:"pieceIndex"`
	IntroTicks     int                `json:"introTicks"`
	IntroTotal     int                `json:"introTotal"`
	IntroFirstWait int                `json:"introFirstWait"`
	EndingTicks    int                `json:"endingTicks"`
	EndingTotal    int                `json:"endingTotal"`
	EndingFade     int                `json:"endingFade"`
	RNGState       uint32             `json:"rngState"`
}

type TetrisSnapshot struct {
	TetrisState
}

type TetrisPersistedState struct {
	TetrisState
}

// Tetris is a slow ambient tetromino effect.
type Tetris struct {
	mu sync.Mutex

	// Rendering grid dimensions; Tetris doesn't paint into a Pixel grid, so
	// these only mirror the canvas size advertised in snapshots.
	W, H int

	rng *rngutil.RNG
	cfg TetrisConfig

	// Piece-sequence RNG. mulberry32 state is mirrored into JS so both ends
	// agree on piece types, columns, and rotations without a parallel
	// floating-point RNG.
	seqState uint32

	tick int

	boardW int
	boardH int
	cells  []byte
	hues   []float64

	active     *TetrisActivePiece
	nextKind   byte
	fallTimer  int
	spawnPause int
	lullPause  int
	pieceIndex int

	introTicks     int
	introTotal     int
	introFirstWait int
	endingTicks    int
	endingTotal    int
	endingFade     int

	log []LogEntry
}

func NewTetris(w, h int, seed int64, cfg TetrisConfig) *Tetris {
	t := &Tetris{
		W:        w,
		H:        h,
		rng:      rngutil.New(seed),
		cfg:      cfg.withDefaults(),
		seqState: tetrisSeqState(uint32(uint64(seed) ^ uint64(seed>>32))),
	}
	t.boardW = t.cfg.BoardW
	t.boardH = t.cfg.BoardH
	t.cells = make([]byte, t.boardW*t.boardH)
	t.hues = make([]float64, t.boardW*t.boardH)
	t.spawnPause = t.cfg.IntroFirst
	t.nextKind = t.rollPieceKind()
	return t
}

func (t *Tetris) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.W = w
	t.H = h
}

func (t *Tetris) SetConfig(cfg TetrisConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	next := cfg.withDefaults()
	if next.BoardW != t.boardW || next.BoardH != t.boardH {
		t.boardW = next.BoardW
		t.boardH = next.BoardH
		t.cells = make([]byte, t.boardW*t.boardH)
		t.hues = make([]float64, t.boardW*t.boardH)
		t.active = nil
		t.spawnPause = next.IntroFirst
		t.fallTimer = 0
	}
	t.cfg = next
}

func (t *Tetris) EffectiveConfig() TetrisConfig {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cfg
}

func (t *Tetris) CurrentTick() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tick
}

func (t *Tetris) PerturbRNG(delta int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rng.Mix(delta)
	t.seqState ^= uint32(uint64(delta) ^ uint64(delta>>32))
	if t.seqState == 0 {
		t.seqState = tetrisSeqState(1)
	}
}

func (t *Tetris) DrainLog() []LogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.log) == 0 {
		return nil
	}
	out := t.log
	t.log = nil
	return out
}

func (t *Tetris) appendLog(kind, desc string) {
	t.log = append(t.log, LogEntry{Tick: t.tick, Type: kind, Desc: desc})
	if len(t.log) > 200 {
		t.log = t.log[len(t.log)-200:]
	}
}

func (t *Tetris) TriggerEvent(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch name {
	case "intro":
		t.startIntroLocked()
		fillPct := t.cfg.IntroHeight * 100
		t.appendLog("intro", fmt.Sprintf("started (dur=%d, fill=%.0f%%)", t.introTotal, fillPct))
		return true
	case "ending":
		t.startEndingLocked()
		t.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", t.endingFade, t.endingTotal-t.endingFade))
		return true
	case "new-piece":
		t.spawnPause = 0
		t.lullPause = 0
		spawned := t.spawnNextLocked()
		if spawned && t.active != nil {
			t.appendLog("new-piece", fmt.Sprintf("%s @ col %d", tetrisPieceName(t.active.Kind), t.active.Col))
		}
		return true
	case "lull":
		t.lullPause = t.cfg.LullDur
		t.appendLog("lull", fmt.Sprintf("queued (+%d ticks)", t.cfg.LullDur))
		return true
	}
	return false
}

func (t *Tetris) Step() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tick++

	if t.endingTicks > 0 {
		t.endingTicks--
		if t.endingTicks == 0 {
			t.startIntroLocked()
			t.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, fill=%.0f%%)", t.introTotal, t.cfg.IntroHeight*100))
		}
		return
	}

	if t.introTicks > 0 {
		t.introTicks--
	}

	if t.active == nil {
		if t.lullPause > 0 {
			t.lullPause--
			return
		}
		if t.spawnPause > 0 {
			t.spawnPause--
			return
		}
		if !t.spawnNextLocked() {
			// spawn collision = lose state
			t.startEndingLocked()
			t.appendLog("ending", "spawn blocked — well full")
		}
		return
	}

	if t.active.Locking {
		t.active.LockTick++
		if t.active.LockTick < t.cfg.LockDelay {
			return
		}
		t.lockActiveLocked()
		t.fallTimer = 0
		t.spawnPause = t.cfg.SpawnPause
		if t.cfg.LullChance > 0 && t.rng.Float64() < t.cfg.LullChance {
			t.lullPause = t.cfg.LullDur
			t.appendLog("lull", fmt.Sprintf("rolled (+%d ticks)", t.cfg.LullDur))
		}
		if t.fillRatioLocked() >= t.cfg.FillThreshold {
			t.startEndingLocked()
			t.appendLog("ending", fmt.Sprintf("fill %.0f%% reached threshold", t.fillRatioLocked()*100))
		}
		return
	}

	t.fallTimer++
	if t.fallTimer < t.cfg.FallEvery {
		return
	}
	t.fallTimer = 0

	if t.canPlace(t.active.Kind, t.active.Rotation, t.active.Row+1, t.active.Col) {
		t.active.Row++
		return
	}
	t.active.Locking = true
	t.active.LockTick = 0
}

func (t *Tetris) startIntroLocked() {
	t.cells = make([]byte, t.boardW*t.boardH)
	t.hues = make([]float64, t.boardW*t.boardH)
	t.active = nil
	t.spawnPause = t.cfg.IntroFirst
	t.lullPause = 0
	t.fallTimer = 0
	t.introTotal = t.cfg.IntroDur
	if t.introTotal <= 0 {
		t.introTotal = 60
	}
	t.introTicks = t.introTotal
	t.introFirstWait = t.cfg.IntroFirst
	t.endingTicks = 0
	t.endingTotal = 0
	t.endingFade = 0
	if t.cfg.IntroHeight > 0 {
		t.fillStartingStackLocked(t.cfg.IntroHeight)
	}
	t.nextKind = t.rollPieceKind()
}

func (t *Tetris) startEndingLocked() {
	t.active = nil
	t.spawnPause = 0
	t.lullPause = 0
	t.fallTimer = 0
	t.introTicks = 0
	t.introTotal = 0
	t.endingFade = t.cfg.EndingDur
	if t.endingFade <= 0 {
		t.endingFade = 80
	}
	linger := t.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	t.endingTotal = t.endingFade + linger
	if t.endingTotal < 1 {
		t.endingTotal = t.endingFade
	}
	t.endingTicks = t.endingTotal
}

// fillStartingStackLocked drops random debris into the bottom rows so the
// scene boots into something with visible architecture.
func (t *Tetris) fillStartingStackLocked(height float64) {
	rows := int(math.Round(height * float64(t.boardH)))
	if rows <= 0 {
		return
	}
	if rows > t.boardH-3 {
		rows = t.boardH - 3
	}
	for r := t.boardH - rows; r < t.boardH; r++ {
		holes := 1 + t.rng.Intn(2)
		holeCols := map[int]bool{}
		for h := 0; h < holes; h++ {
			holeCols[t.rng.Intn(t.boardW)] = true
		}
		for c := 0; c < t.boardW; c++ {
			if holeCols[c] {
				continue
			}
			kind := byte(1 + t.rng.Intn(7))
			idx := r*t.boardW + c
			t.cells[idx] = kind
			t.hues[idx] = t.pieceHue(kind)
		}
	}
}

func (t *Tetris) spawnNextLocked() bool {
	kind := t.nextKind
	if kind == 0 {
		kind = t.rollPieceKind()
	}
	t.nextKind = t.rollPieceKind()
	t.pieceIndex++
	rotations := tetrisShapes[kind]
	rot := 0
	if len(rotations) > 1 {
		rot = int(tetrisSeqUint32(&t.seqState) % uint32(len(rotations)))
	}
	bw, _ := tetrisShapeExtent(kind, rot)
	maxCol := t.boardW - bw
	if maxCol < 0 {
		maxCol = 0
	}
	col := 0
	if maxCol > 0 {
		col = int(tetrisSeqUint32(&t.seqState) % uint32(maxCol+1))
	}
	if !t.canPlace(kind, rot, 0, col) {
		t.active = nil
		return false
	}
	t.active = &TetrisActivePiece{
		Kind:     kind,
		Rotation: rot,
		Row:      0,
		Col:      col,
		Hue:      t.pieceHue(kind),
	}
	t.fallTimer = 0
	return true
}

func (t *Tetris) rollPieceKind() byte {
	return byte(1 + tetrisSeqUint32(&t.seqState)%7)
}

func (t *Tetris) pieceHue(kind byte) float64 {
	if kind == 0 {
		return t.cfg.Hue
	}
	base := t.cfg.Hue + tetrisPieceHues[kind]
	if t.cfg.HueSpread > 0 {
		jitter := (t.rng.Float64()*2 - 1) * t.cfg.HueSpread
		base += jitter
	}
	for base < 0 {
		base += 360
	}
	for base >= 360 {
		base -= 360
	}
	return base
}

func (t *Tetris) canPlace(kind byte, rot, row, col int) bool {
	rotations := tetrisShapes[kind]
	if rot < 0 || rot >= len(rotations) {
		return false
	}
	for _, off := range rotations[rot] {
		r := row + off[0]
		c := col + off[1]
		if r < 0 {
			continue
		}
		if r >= t.boardH || c < 0 || c >= t.boardW {
			return false
		}
		if t.cells[r*t.boardW+c] != 0 {
			return false
		}
	}
	return true
}

func (t *Tetris) lockActiveLocked() {
	if t.active == nil {
		return
	}
	rotations := tetrisShapes[t.active.Kind]
	if t.active.Rotation < 0 || t.active.Rotation >= len(rotations) {
		t.active = nil
		return
	}
	for _, off := range rotations[t.active.Rotation] {
		r := t.active.Row + off[0]
		c := t.active.Col + off[1]
		if r < 0 || r >= t.boardH || c < 0 || c >= t.boardW {
			continue
		}
		idx := r*t.boardW + c
		t.cells[idx] = t.active.Kind
		t.hues[idx] = t.active.Hue
	}
	t.active = nil
}

func (t *Tetris) fillRatioLocked() float64 {
	if len(t.cells) == 0 {
		return 0
	}
	filled := 0
	for _, c := range t.cells {
		if c != 0 {
			filled++
		}
	}
	return float64(filled) / float64(len(t.cells))
}

func (t *Tetris) Snapshot() TetrisSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TetrisSnapshot{TetrisState: t.snapshotStateLocked()}
}

func (t *Tetris) RestoreSnapshot(s TetrisSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.restoreStateLocked(s.TetrisState)
}

func (t *Tetris) SnapshotPersistedState() TetrisPersistedState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TetrisPersistedState{TetrisState: t.snapshotStateLocked()}
}

func (t *Tetris) RestorePersistedState(s TetrisPersistedState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.restoreStateLocked(s.TetrisState)
}

func (t *Tetris) snapshotStateLocked() TetrisState {
	cells := make([]byte, len(t.cells))
	copy(cells, t.cells)
	hues := make([]float64, len(t.hues))
	copy(hues, t.hues)
	var active *TetrisActivePiece
	if t.active != nil {
		clone := *t.active
		active = &clone
	}
	return TetrisState{
		Tick:           t.tick,
		BoardW:         t.boardW,
		BoardH:         t.boardH,
		Cells:          cells,
		Hues:           hues,
		Active:         active,
		NextKind:       t.nextKind,
		FallTimer:      t.fallTimer,
		SpawnPause:     t.spawnPause,
		LullPause:      t.lullPause,
		PieceIndex:     t.pieceIndex,
		IntroTicks:     t.introTicks,
		IntroTotal:     t.introTotal,
		IntroFirstWait: t.introFirstWait,
		EndingTicks:    t.endingTicks,
		EndingTotal:    t.endingTotal,
		EndingFade:     t.endingFade,
		RNGState:       t.seqState,
	}
}

func (t *Tetris) restoreStateLocked(s TetrisState) {
	t.tick = s.Tick
	if s.BoardW > 0 && s.BoardH > 0 {
		t.boardW = s.BoardW
		t.boardH = s.BoardH
	}
	if len(s.Cells) == t.boardW*t.boardH {
		t.cells = make([]byte, len(s.Cells))
		copy(t.cells, s.Cells)
	} else {
		t.cells = make([]byte, t.boardW*t.boardH)
	}
	if len(s.Hues) == t.boardW*t.boardH {
		t.hues = make([]float64, len(s.Hues))
		copy(t.hues, s.Hues)
	} else {
		t.hues = make([]float64, t.boardW*t.boardH)
	}
	if s.Active != nil {
		clone := *s.Active
		t.active = &clone
	} else {
		t.active = nil
	}
	t.nextKind = s.NextKind
	t.fallTimer = s.FallTimer
	t.spawnPause = s.SpawnPause
	t.lullPause = s.LullPause
	t.pieceIndex = s.PieceIndex
	t.introTicks = s.IntroTicks
	t.introTotal = s.IntroTotal
	t.introFirstWait = s.IntroFirstWait
	t.endingTicks = s.EndingTicks
	t.endingTotal = s.EndingTotal
	t.endingFade = s.EndingFade
	if s.RNGState != 0 {
		t.seqState = s.RNGState
	}
}

func tetrisShapeExtent(kind byte, rot int) (w, h int) {
	rotations := tetrisShapes[kind]
	if rot < 0 || rot >= len(rotations) {
		return 0, 0
	}
	for _, off := range rotations[rot] {
		if off[1]+1 > w {
			w = off[1] + 1
		}
		if off[0]+1 > h {
			h = off[0] + 1
		}
	}
	return
}

func tetrisPieceName(kind byte) string {
	switch kind {
	case tetrisPieceI:
		return "I"
	case tetrisPieceO:
		return "O"
	case tetrisPieceT:
		return "T"
	case tetrisPieceS:
		return "S"
	case tetrisPieceZ:
		return "Z"
	case tetrisPieceJ:
		return "J"
	case tetrisPieceL:
		return "L"
	}
	return "?"
}

// tetrisSeqState normalizes a uint32 seed so 0 doesn't collapse the sequence.
func tetrisSeqState(seed uint32) uint32 {
	if seed == 0 {
		return 0x9e3779b9
	}
	return seed
}

// tetrisSeqUint32 advances a mulberry32 state and returns the next 32-bit
// value. The same algorithm is implemented in cmd/ambience/web/sim.js so the
// authority and browser sims agree on piece sequencing without needing the
// full RNG envelope synced.
func tetrisSeqUint32(state *uint32) uint32 {
	*state += 0x6D2B79F5
	z := *state
	z = (z ^ (z >> 15)) * (z | 1)
	z ^= z + (z^(z>>7))*(z|61)
	return z ^ (z >> 14)
}
