package sim

import (
	"fmt"
	"math"
)

const (
	tetrisBoardCols = 10
	tetrisBoardRows = 20
)

type tetrisPoint struct {
	X int
	Y int
}

var tetrisDefaults = ProceduralConfig{
	"intro_dur":     60,
	"intro_stack":   0,
	"intro_cadence": 0.55,
	"ending_dur":    70,
	"ending_linger": 28,
	"ending_fill":   0.84,
	"well_x":        0.18,
	"well_y":        0.13,
	"cell_size":     3,
	"spawn_every":   42,
	"fall_every":    8,
	"lock_delay":    4,
	"glow":          0.18,
	"ghost":         0.14,
	"hue":           208,
	"hue_sp":        24,
	"sat":           0.56,
	"lmin":          0.10,
	"lmax":          0.92,
	"new_piece_p":   0.0,
	"lull_p":        0.0,
	"new_piece_dur": 14,
	"new_piece_cut": 0.25,
	"lull_dur":      80,
	"lull_mult":     1.8,
}

var tetrisPieceNames = []string{"I", "J", "L", "O", "S", "T", "Z"}

var tetrisPieceRotations = [][][]tetrisPoint{
	{
		{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		{{2, 0}, {2, 1}, {2, 2}, {2, 3}},
		{{0, 2}, {1, 2}, {2, 2}, {3, 2}},
		{{1, 0}, {1, 1}, {1, 2}, {1, 3}},
	},
	{
		{{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {2, 2}},
		{{1, 0}, {1, 1}, {0, 2}, {1, 2}},
	},
	{
		{{2, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {1, 1}, {1, 2}, {2, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {0, 2}},
		{{0, 0}, {1, 0}, {1, 1}, {1, 2}},
	},
	{
		{{0, 0}, {1, 0}, {0, 1}, {1, 1}},
		{{0, 0}, {1, 0}, {0, 1}, {1, 1}},
		{{0, 0}, {1, 0}, {0, 1}, {1, 1}},
		{{0, 0}, {1, 0}, {0, 1}, {1, 1}},
	},
	{
		{{1, 0}, {2, 0}, {0, 1}, {1, 1}},
		{{1, 0}, {1, 1}, {2, 1}, {2, 2}},
		{{1, 1}, {2, 1}, {0, 2}, {1, 2}},
		{{0, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	{
		{{1, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {1, 2}},
		{{1, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	{
		{{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		{{2, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {1, 2}, {2, 2}},
		{{1, 0}, {0, 1}, {1, 1}, {0, 2}},
	},
}

func TetrisSchema() EffectSchema {
	return EffectSchema{
		Name: "tetris",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent easing from an empty well into the normal spawn rhythm."},
			{Key: "intro_stack", Label: "intro stack", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 8, Step: 1, Default: 0,
				Description: "Approximate starting stack height in rows before the first active piece appears."},
			{Key: "intro_cadence", Label: "intro cadence", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Fraction of the normal piece cadence used near the start of the intro."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent dimming the board after it reaches a quiet lose-state."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 28,
				Description: "Extra quiet ticks where the filled board remains on screen before restart."},
			{Key: "ending_fill", Label: "ending fill", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.6, Max: 0.98, Step: 0.01, Default: 0.84,
				Description: "Board-fill ratio that triggers the ambient lose-state even before a blocked spawn."},
			{Key: "well_x", Label: "well x", Slot: SlotLever, Group: "layout", Type: KnobFloat, Min: 0.08, Max: 0.4, Step: 0.01, Default: 0.18,
				Description: "Horizontal placement of the Tetris well in the scene."},
			{Key: "well_y", Label: "well y", Slot: SlotLever, Group: "layout", Type: KnobFloat, Min: 0.05, Max: 0.25, Step: 0.01, Default: 0.13,
				Description: "Vertical placement of the well's top edge in the scene."},
			{Key: "cell_size", Label: "cell size", Slot: SlotLever, Group: "layout", Type: KnobFloat, Min: 2, Max: 4, Step: 0.25, Default: 3,
				Description: "Rendered size of each board cell in scene-grid units."},
			{Key: "spawn_every", Label: "spawn every", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 16, Max: 160, Step: 2, Default: 42,
				Description: "Base pause between settled pieces before the next tetromino appears."},
			{Key: "fall_every", Label: "fall every", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 3, Max: 20, Step: 1, Default: 8,
				Description: "Ticks between each downward step of the active tetromino."},
			{Key: "lock_delay", Label: "lock delay", Slot: SlotLever, Group: "cadence", Type: KnobInt, Min: 1, Max: 12, Step: 1, Default: 4,
				Description: "Extra ticks a resting piece waits before it settles into the stack."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Strength of the board halo and settled-stack atmosphere."},
			{Key: "ghost", Label: "ghost", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Opacity of the landing ghost beneath the active tetromino."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 180, Max: 260, Step: 1, Default: 208,
				Description: "Base palette anchor for the cool neon board background."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 8, Max: 40, Step: 1, Default: 24,
				Description: "How far the tetromino colors spread away from the base hue."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.01, Default: 0.56,
				Description: "Overall saturation of the board, blocks, and glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.05, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Minimum lightness used for the well interior and background."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "palette", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Maximum lightness used for active-piece highlights and board accents."},
			{Key: "new_piece_p", Label: "new piece", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "new-piece",
				Description: "Per-tick chance of shortening the wait until the next tetromino arrives."},
			{Key: "lull_p", Label: "lull", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "lull",
				Description: "Per-tick chance of a longer-than-usual pause between pieces."},
			{Key: "new_piece_dur", Label: "new piece dur", Slot: SlotEventMod, Group: "new-piece", Type: KnobInt, Min: 4, Max: 40, Step: 1, Default: 14,
				Description: "Duration of the shortened-spawn window created by a new-piece event."},
			{Key: "new_piece_cut", Label: "new piece x", Slot: SlotEventMod, Group: "new-piece", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Fraction of the normal spawn delay kept while a new-piece window is active."},
			{Key: "lull_dur", Label: "lull dur", Slot: SlotEventMod, Group: "lull", Type: KnobInt, Min: 20, Max: 200, Step: 5, Default: 80,
				Description: "Duration of the extended ambient pause."},
			{Key: "lull_mult", Label: "lull x", Slot: SlotEventMod, Group: "lull", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Spawn-delay multiplier applied while a lull is active."},
		},
	}
}

func mergeTetrisDefaults(cfg ProceduralConfig) ProceduralConfig {
	out := cloneProceduralConfig(tetrisDefaults)
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = tetrisDefaults["intro_dur"]
	}
	if out["intro_stack"] < 0 {
		out["intro_stack"] = 0
	}
	if out["intro_stack"] > 8 {
		out["intro_stack"] = 8
	}
	out["intro_cadence"] = clamp01(out["intro_cadence"])
	if out["intro_cadence"] < 0.2 {
		out["intro_cadence"] = tetrisDefaults["intro_cadence"]
	}
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = tetrisDefaults["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_fill"] = clamp01(out["ending_fill"])
	if out["ending_fill"] < 0.6 {
		out["ending_fill"] = tetrisDefaults["ending_fill"]
	}
	if out["well_x"] <= 0 {
		out["well_x"] = tetrisDefaults["well_x"]
	}
	if out["well_y"] <= 0 {
		out["well_y"] = tetrisDefaults["well_y"]
	}
	if out["cell_size"] <= 0 {
		out["cell_size"] = tetrisDefaults["cell_size"]
	}
	if out["spawn_every"] <= 0 {
		out["spawn_every"] = tetrisDefaults["spawn_every"]
	}
	if out["fall_every"] <= 0 {
		out["fall_every"] = tetrisDefaults["fall_every"]
	}
	if out["lock_delay"] < 1 {
		out["lock_delay"] = tetrisDefaults["lock_delay"]
	}
	if out["glow"] <= 0 {
		out["glow"] = tetrisDefaults["glow"]
	}
	if out["ghost"] < 0 {
		out["ghost"] = 0
	}
	if out["hue"] <= 0 {
		out["hue"] = tetrisDefaults["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = tetrisDefaults["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = tetrisDefaults["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = tetrisDefaults["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["new_piece_dur"] <= 0 {
		out["new_piece_dur"] = tetrisDefaults["new_piece_dur"]
	}
	if out["new_piece_cut"] <= 0 {
		out["new_piece_cut"] = tetrisDefaults["new_piece_cut"]
	}
	if out["lull_dur"] <= 0 {
		out["lull_dur"] = tetrisDefaults["lull_dur"]
	}
	if out["lull_mult"] <= 1 {
		out["lull_mult"] = tetrisDefaults["lull_mult"]
	}
	return out
}

func tetrisMakeRNG(seed uint32) func() float64 {
	state := seed
	return func() float64 {
		state += 0x6D2B79F5
		t := state
		t = uint32(int32(t^(t>>15)) * int32(t|1))
		t ^= t + uint32(int32(t^(t>>7))*int32(t|61))
		return float64((t^(t>>14))>>0) / 4294967296.0
	}
}

func tetrisJitterInt(rng func() float64, base int, spread float64) int {
	f := float64(base) * (1 + spread*(rng()*2-1))
	return max(1, int(math.Round(f)))
}

func (p *Procedural) tetrisSeededRNGLocked(key int) func() float64 {
	seed := uint32(p.seed) ^ uint32(uint32(key)*2654435761)
	return tetrisMakeRNG(seed)
}

func (p *Procedural) tetrisEventRNGLocked(salt int) func() float64 {
	return p.tetrisSeededRNGLocked(p.tick + salt)
}

func tetrisPieceName(kind int) string {
	if kind < 1 || kind > len(tetrisPieceNames) {
		return "?"
	}
	return tetrisPieceNames[kind-1]
}

func tetrisCellKey(row, col int) string {
	return fmt.Sprintf("cell_%02d_%02d", row, col)
}

func tetrisPiecePoints(kind, rot int) []tetrisPoint {
	if kind < 1 || kind > len(tetrisPieceRotations) {
		return nil
	}
	rots := tetrisPieceRotations[kind-1]
	if len(rots) == 0 {
		return nil
	}
	rot = ((rot % len(rots)) + len(rots)) % len(rots)
	return rots[rot]
}

func (p *Procedural) tetrisGetCellLocked(row, col int) int {
	if row < 0 || row >= tetrisBoardRows || col < 0 || col >= tetrisBoardCols {
		return 0
	}
	return int(math.Round(p.values[tetrisCellKey(row, col)]))
}

func (p *Procedural) tetrisSetCellLocked(row, col, value int) {
	if row < 0 || row >= tetrisBoardRows || col < 0 || col >= tetrisBoardCols {
		return
	}
	key := tetrisCellKey(row, col)
	if value <= 0 {
		delete(p.values, key)
		return
	}
	p.values[key] = float64(value)
}

func (p *Procedural) tetrisClearBoardLocked() {
	for row := 0; row < tetrisBoardRows; row++ {
		for col := 0; col < tetrisBoardCols; col++ {
			delete(p.values, tetrisCellKey(row, col))
		}
	}
}

func (p *Procedural) tetrisSeedIntroStackLocked() {
	rows := p.intCfg("intro_stack")
	if rows <= 0 {
		return
	}
	for col := 0; col < tetrisBoardCols; col++ {
		rng := p.tetrisSeededRNGLocked(400 + col*7)
		height := rows - 1 + int(math.Floor(rng()*3))
		if height < 0 {
			height = 0
		}
		if height > rows {
			height = rows
		}
		for depth := 0; depth < height; depth++ {
			row := tetrisBoardRows - 1 - depth
			kind := 1 + (col+depth+int(math.Floor(rng()*float64(len(tetrisPieceRotations)))))%len(tetrisPieceRotations)
			p.tetrisSetCellLocked(row, col, kind)
		}
	}
}

func (p *Procedural) tetrisClearCurrentPieceLocked() {
	p.values["piece_alive"] = 0
	delete(p.values, "piece_kind")
	delete(p.values, "piece_rot")
	delete(p.values, "piece_x")
	delete(p.values, "piece_y")
	delete(p.values, "fall_cd")
	delete(p.values, "lock_cd")
}

func (p *Procedural) tetrisCurrentPieceLocked() (kind, rot, x, y int, alive bool) {
	alive = p.values["piece_alive"] > 0.5
	if !alive {
		return 0, 0, 0, 0, false
	}
	kind = int(math.Round(p.values["piece_kind"]))
	rot = int(math.Round(p.values["piece_rot"]))
	x = int(math.Round(p.values["piece_x"]))
	y = int(math.Round(p.values["piece_y"]))
	return kind, rot, x, y, true
}

func (p *Procedural) tetrisSetCurrentPieceLocked(kind, rot, x, y int) {
	p.values["piece_alive"] = 1
	p.values["piece_kind"] = float64(kind)
	p.values["piece_rot"] = float64(rot)
	p.values["piece_x"] = float64(x)
	p.values["piece_y"] = float64(y)
}

func (p *Procedural) tetrisCollisionLocked(kind, rot, x, y int) bool {
	for _, pt := range tetrisPiecePoints(kind, rot) {
		col := x + pt.X
		row := y + pt.Y
		if col < 0 || col >= tetrisBoardCols || row < 0 || row >= tetrisBoardRows {
			return true
		}
		if p.tetrisGetCellLocked(row, col) > 0 {
			return true
		}
	}
	return false
}

func (p *Procedural) tetrisLockPieceLocked(kind, rot, x, y int) {
	for _, pt := range tetrisPiecePoints(kind, rot) {
		p.tetrisSetCellLocked(y+pt.Y, x+pt.X, kind)
	}
}

func (p *Procedural) tetrisBoardStatsLocked() (filled, stack int) {
	top := tetrisBoardRows
	for row := 0; row < tetrisBoardRows; row++ {
		for col := 0; col < tetrisBoardCols; col++ {
			if p.tetrisGetCellLocked(row, col) <= 0 {
				continue
			}
			filled++
			if row < top {
				top = row
			}
		}
	}
	if filled == 0 {
		return 0, 0
	}
	return filled, tetrisBoardRows - top
}

func (p *Procedural) tetrisUpdateBoardStatsLocked() {
	filled, stack := p.tetrisBoardStatsLocked()
	p.values["fill_ratio"] = float64(filled) / float64(tetrisBoardCols*tetrisBoardRows)
	p.values["stack_height"] = float64(stack)
}

func (p *Procedural) tetrisSpawnDelayLocked() int {
	delay := max(1, p.intCfg("spawn_every"))
	if p.timers["intro"] > 0 {
		total := int(math.Round(p.values["intro_total"]))
		progress := proceduralPhaseProgress(total, p.timers["intro"])
		cadence := p.cfg["intro_cadence"] + (1-p.cfg["intro_cadence"])*progress
		delay = max(1, int(math.Round(float64(delay)*math.Max(0.2, cadence))))
	}
	if p.timers["lull"] > 0 {
		gain := p.values["lull_gain"]
		if gain <= 1 {
			gain = p.cfg["lull_mult"]
		}
		delay = max(1, int(math.Round(float64(delay)*math.Max(1.05, gain))))
	}
	if p.timers["new-piece"] > 0 {
		cut := p.values["new_piece_cut"]
		if cut <= 0 {
			cut = p.cfg["new_piece_cut"]
		}
		delay = max(1, int(math.Round(float64(delay)*math.Max(0.05, cut))))
	}
	return delay
}

func (p *Procedural) tetrisFallDelayLocked() int {
	delay := max(1, p.intCfg("fall_every"))
	if p.timers["new-piece"] > 0 {
		delay = max(1, int(math.Round(float64(delay)*0.6)))
	}
	return delay
}

func (p *Procedural) startTetrisNewPieceLocked(verb string) {
	rng := p.tetrisEventRNGLocked(182)
	p.timers["lull"] = 0
	p.timers["new-piece"] = tetrisJitterInt(rng, p.intCfg("new_piece_dur"), 0.25)
	p.values["lull_gain"] = 1
	p.values["new_piece_cut"] = math.Max(0.05, p.cfg["new_piece_cut"]*(0.8+rng()*0.35))
	if _, _, _, _, alive := p.tetrisCurrentPieceLocked(); alive {
		if fall := int(math.Round(p.values["fall_cd"])); fall > 1 {
			p.values["fall_cd"] = 1
		}
		if lock := int(math.Round(p.values["lock_cd"])); lock > 1 {
			p.values["lock_cd"] = 1
		}
	} else if spawn := int(math.Round(p.values["spawn_cd"])); spawn > 1 {
		p.values["spawn_cd"] = float64(max(1, int(math.Round(float64(spawn)*p.values["new_piece_cut"]))))
	}
	p.appendLog("new-piece", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["new-piece"], p.values["new_piece_cut"]))
}

func (p *Procedural) startTetrisLullLocked(verb string) {
	rng := p.tetrisEventRNGLocked(177)
	p.timers["new-piece"] = 0
	p.timers["lull"] = tetrisJitterInt(rng, p.intCfg("lull_dur"), 0.25)
	p.values["new_piece_cut"] = p.cfg["new_piece_cut"]
	p.values["lull_gain"] = math.Max(1.05, p.cfg["lull_mult"]*(0.9+rng()*0.3))
	if _, _, _, _, alive := p.tetrisCurrentPieceLocked(); !alive {
		delay := int(math.Round(p.values["spawn_cd"]))
		if delay < 1 {
			delay = p.intCfg("spawn_every")
		}
		target := int(math.Round(float64(delay) * p.values["lull_gain"]))
		if target > delay {
			p.values["spawn_cd"] = float64(target)
		}
	}
	p.appendLog("lull", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, p.timers["lull"], p.values["lull_gain"]))
}

func (p *Procedural) startTetrisIntroLocked() {
	p.timers["new-piece"] = 0
	p.timers["lull"] = 0
	p.timers["ending"] = 0
	p.values["lull_gain"] = 1
	p.values["new_piece_cut"] = p.cfg["new_piece_cut"]
	delete(p.values, "ended")
	delete(p.values, "ending_total")
	p.tetrisClearCurrentPieceLocked()
	p.tetrisClearBoardLocked()
	p.tetrisSeedIntroStackLocked()
	p.values["piece_counter"] = 0
	p.timers["intro"] = p.intCfg("intro_dur")
	p.values["intro_total"] = float64(p.timers["intro"])
	p.values["spawn_cd"] = float64(max(1, int(math.Round(float64(p.intCfg("spawn_every"))*math.Max(0.2, p.cfg["intro_cadence"])))))
	p.tetrisUpdateBoardStatsLocked()
}

func (p *Procedural) startTetrisEndingLocked(reason string) {
	p.timers["intro"] = 0
	p.timers["new-piece"] = 0
	p.timers["lull"] = 0
	p.values["lull_gain"] = 1
	delete(p.values, "intro_total")
	p.tetrisClearCurrentPieceLocked()
	endingTotal := p.intCfg("ending_dur") + max(0, p.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, p.intCfg("ending_dur"))
	}
	p.timers["ending"] = endingTotal
	p.values["ending_total"] = float64(endingTotal)
	p.values["ended"] = 1
	p.tetrisUpdateBoardStatsLocked()
	if reason != "" {
		p.appendLog("ending", reason)
	}
}

func (p *Procedural) tetrisSpawnPieceLocked(verb string) bool {
	counter := int(math.Round(p.values["piece_counter"]))
	rng := p.tetrisSeededRNGLocked(800 + counter*17)
	kind := 1 + int(math.Floor(rng()*float64(len(tetrisPieceRotations))))
	rot := int(math.Floor(rng() * 4))
	x := 3
	y := 0
	if p.tetrisCollisionLocked(kind, rot, x, y) {
		p.startTetrisEndingLocked("started (board blocked)")
		return false
	}
	p.values["piece_counter"] = float64(counter + 1)
	p.tetrisSetCurrentPieceLocked(kind, rot, x, y)
	p.values["fall_cd"] = float64(p.tetrisFallDelayLocked())
	p.values["lock_cd"] = float64(max(1, p.intCfg("lock_delay")))
	p.values["spawn_cd"] = 0
	if p.timers["new-piece"] > 0 {
		p.timers["new-piece"] = 0
	}
	p.appendLog("new-piece", fmt.Sprintf("%s (%s)", verb, tetrisPieceName(kind)))
	return true
}

func (p *Procedural) stepTetrisLocked() {
	if p.timers["lull"] <= 0 {
		p.values["lull_gain"] = 1
	}
	if p.timers["new-piece"] <= 0 {
		delete(p.values, "new_piece_cut")
	}
	if p.timers["intro"] <= 0 {
		delete(p.values, "intro_total")
	}
	if p.timers["ending"] <= 0 {
		delete(p.values, "ending_total")
		if p.values["ended"] > 0.5 {
			p.startTetrisIntroLocked()
			p.appendLog("intro", "restarted after lose-state")
			return
		}
	}
	if p.timers["ending"] > 0 {
		p.tetrisUpdateBoardStatsLocked()
		return
	}

	kind, rot, x, y, alive := p.tetrisCurrentPieceLocked()
	if !alive {
		spawnCD := int(math.Round(p.values["spawn_cd"]))
		if spawnCD > 0 {
			spawnCD--
			p.values["spawn_cd"] = float64(spawnCD)
		}
		if spawnCD <= 0 {
			p.tetrisSpawnPieceLocked("spawned")
		}
	} else {
		fallCD := int(math.Round(p.values["fall_cd"]))
		lockCD := int(math.Round(p.values["lock_cd"]))
		if p.timers["new-piece"] > 0 && fallCD > 1 {
			fallCD = 1
		}
		if fallCD > 0 {
			fallCD--
			p.values["fall_cd"] = float64(fallCD)
		} else if !p.tetrisCollisionLocked(kind, rot, x, y+1) {
			y++
			p.values["piece_y"] = float64(y)
			p.values["fall_cd"] = float64(p.tetrisFallDelayLocked())
			p.values["lock_cd"] = float64(max(1, p.intCfg("lock_delay")))
		} else if lockCD > 0 {
			lockCD--
			p.values["lock_cd"] = float64(lockCD)
		} else {
			p.tetrisLockPieceLocked(kind, rot, x, y)
			p.tetrisClearCurrentPieceLocked()
			p.values["spawn_cd"] = float64(p.tetrisSpawnDelayLocked())
			p.tetrisUpdateBoardStatsLocked()
			if p.values["fill_ratio"] >= math.Max(0.6, p.cfg["ending_fill"]) {
				p.startTetrisEndingLocked(fmt.Sprintf("started (fill=%.2f)", p.values["fill_ratio"]))
				return
			}
		}
	}

	p.tetrisUpdateBoardStatsLocked()
	if p.timers["new-piece"] <= 0 && p.timers["lull"] <= 0 && p.cfg["new_piece_p"] > 0 && p.rng.Float64() < p.cfg["new_piece_p"] {
		p.startTetrisNewPieceLocked("started")
		return
	}
	if p.timers["lull"] <= 0 && p.timers["new-piece"] <= 0 && p.cfg["lull_p"] > 0 && p.rng.Float64() < p.cfg["lull_p"] {
		p.startTetrisLullLocked("started")
	}
}
