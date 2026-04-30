package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// RainOnWindow is the "rain seen from inside a window" effect. The framing
// is the window itself — droplets nucleate on a pane of glass, grow on
// contact with neighbors, and once they exceed a critical mass they track
// downward leaving a streak of smaller residue drops behind. A diffuse
// background glow suggests warm interior light or a dim city street.
//
// The authority owns each drop's discrete state and lifecycle so the
// browser and terminal replicas agree on which drops exist, when they
// fall, and when they wipe off the bottom edge. Inner shimmer (light
// catch on each drop) is the only thing the renderer free-runs per
// client.
//
// Per-drop state is one of:
//   ROWEmpty (0)    — slot has no live drop
//   ROWForming (1)  — drop is nucleating; size ramps up
//   ROWHolding (2)  — drop has reached its target size and is clinging
//   ROWFalling (3)  — drop has exceeded critical size and is tracking down
//
// The slot count is fixed at config time; each slot is anchored to a
// stable column on the pane so spatial layout stays coherent across
// clients even when individual drops cycle.

const (
	ROWEmpty   byte = iota // 0
	ROWForming             // 1
	ROWHolding             // 2
	ROWFalling             // 3
)

// RainOnWindowConfig tunes the rain-on-window prototype used in isolated
// dev sessions. See RainOnWindowSchema for the full knob inventory.
type RainOnWindowConfig struct {
	// INTRODUCTION
	IntroDur     int     `json:"intro_dur"`
	IntroBurst   float64 `json:"intro_burst"`
	IntroBgRamp  float64 `json:"intro_bg_ramp"`
	// ENDING
	EndingDur     int     `json:"ending_dur"`
	EndingLinger  int     `json:"ending_linger"`
	EndingResidue float64 `json:"ending_residue"`
	// LEVERS — pane geometry
	SlotCount   int     `json:"slot_count"`
	FormDur     int     `json:"form_dur"`
	HoldDur     int     `json:"hold_dur"`
	FallSpeed   float64 `json:"fall_speed"`
	DropMinSize float64 `json:"drop_min_size"`
	DropMaxSize float64 `json:"drop_max_size"`
	CritSize    float64 `json:"crit_size"`
	TrailLen    float64 `json:"trail_len"`
	FrameThick  float64 `json:"frame_thick"`
	// LEVERS — color (background and drops)
	BgHue       float64 `json:"bg_hue"`
	BgSat       float64 `json:"bg_sat"`
	BgLight     float64 `json:"bg_light"`
	BgGlow      float64 `json:"bg_glow"`
	DropHue     float64 `json:"drop_hue"`
	DropSat     float64 `json:"drop_sat"`
	HighlightL  float64 `json:"hl_light"`
	ShadowL     float64 `json:"sh_light"`
	// EVENT CHANCES
	FormChance float64 `json:"form_p"`
	FallChance float64 `json:"fall_p"`
	GustChance float64 `json:"gust_p"`
	QuietChance float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	GustDur   int     `json:"gust_dur"`
	GustSkew  float64 `json:"gust_skew"`
	QuietDur  int     `json:"quiet_dur"`
	QuietMult float64 `json:"quiet_mult"`
}

func (c RainOnWindowConfig) withDefaults() RainOnWindowConfig {
	if c.IntroDur == 0 && c.IntroBurst == 0 && c.IntroBgRamp == 0 {
		c.IntroDur = 50
		c.IntroBurst = 0.55
		c.IntroBgRamp = 0.35
	} else {
		if c.IntroDur <= 0 {
			c.IntroDur = 50
		}
		if c.IntroBurst < 0 {
			c.IntroBurst = 0
		}
		if c.IntroBgRamp < 0 {
			c.IntroBgRamp = 0
		}
	}
	c.IntroBurst = clamp01(c.IntroBurst)
	c.IntroBgRamp = clamp01(c.IntroBgRamp)
	if c.EndingDur == 0 && c.EndingLinger == 0 && c.EndingResidue == 0 {
		c.EndingDur = 80
		c.EndingLinger = 30
		c.EndingResidue = 0.18
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 80
		}
		if c.EndingLinger < 0 {
			c.EndingLinger = 0
		}
		if c.EndingResidue < 0 {
			c.EndingResidue = 0
		}
	}
	c.EndingResidue = clamp01(c.EndingResidue)
	if c.SlotCount <= 0 {
		c.SlotCount = 36
	}
	if c.SlotCount > 96 {
		c.SlotCount = 96
	}
	if c.FormDur <= 0 {
		c.FormDur = 70
	}
	if c.HoldDur <= 0 {
		c.HoldDur = 220
	}
	if c.FallSpeed <= 0 {
		c.FallSpeed = 0.7
	}
	if c.DropMinSize <= 0 {
		c.DropMinSize = 1.2
	}
	if c.DropMaxSize <= 0 {
		c.DropMaxSize = 3.4
	}
	if c.DropMaxSize < c.DropMinSize {
		c.DropMinSize, c.DropMaxSize = c.DropMaxSize, c.DropMinSize
	}
	if c.CritSize <= 0 {
		c.CritSize = 2.6
	}
	if c.TrailLen <= 0 {
		c.TrailLen = 18
	}
	if c.FrameThick < 0 {
		c.FrameThick = 0
	}
	if c.FrameThick == 0 {
		c.FrameThick = 2
	}
	if c.BgHue == 0 {
		c.BgHue = 30
	}
	if c.BgSat <= 0 {
		c.BgSat = 0.45
	}
	if c.BgLight <= 0 {
		c.BgLight = 0.22
	}
	if c.BgGlow <= 0 {
		c.BgGlow = 0.6
	}
	if c.DropHue == 0 {
		c.DropHue = 210
	}
	if c.DropSat <= 0 {
		c.DropSat = 0.18
	}
	if c.HighlightL <= 0 {
		c.HighlightL = 0.85
	}
	if c.ShadowL <= 0 {
		c.ShadowL = 0.18
	}
	if c.FormChance < 0 {
		c.FormChance = 0
	}
	if c.FallChance < 0 {
		c.FallChance = 0
	}
	if c.GustChance < 0 {
		c.GustChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.GustDur <= 0 {
		c.GustDur = 50
	}
	if c.GustSkew == 0 {
		c.GustSkew = 0.6
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 90
	}
	if c.QuietMult <= 0 {
		c.QuietMult = 0.25
	}
	return c
}

// RainOnWindowSchema describes the rain-on-window effect's tunable knobs
// for the dev UI.
func RainOnWindowSchema() EffectSchema {
	return EffectSchema{
		Name: "rain-on-window",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 50, Trigger: "intro",
				Description: "Ticks spent kicking off rapid initial nucleation before the steady cadence settles in."},
			{Key: "intro_burst", Label: "intro burst", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.55,
				Description: "Fraction of pane slots seeded with starter drops the moment the intro fires."},
			{Key: "intro_bg_ramp", Label: "intro bg ramp", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Starting background glow level during the intro before it eases up to the configured value."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent suppressing new drops while existing tracks finish running off the pane."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 30,
				Description: "Extra still ticks after the fade so residual streaks can dim before the cycle restarts."},
			{Key: "ending_residue", Label: "ending residue", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.18,
				Description: "Fraction of drop highlights that linger as the outro completes."},
			{Key: "slot_count", Label: "slot count", Slot: SlotLever, Group: "pane", Type: KnobInt, Min: 8, Max: 96, Step: 1, Default: 36,
				Description: "Maximum live drops on the pane at once. Higher values feel busier; lower values feel sparser."},
			{Key: "form_dur", Label: "form dur", Slot: SlotLever, Group: "pane", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 70,
				Description: "Ticks a drop spends nucleating up to its target size before it stabilizes."},
			{Key: "hold_dur", Label: "hold dur", Slot: SlotLever, Group: "pane", Type: KnobInt, Min: 30, Max: 800, Step: 10, Default: 220,
				Description: "Ticks a settled drop clings to the glass before crossing the critical-size threshold."},
			{Key: "fall_speed", Label: "fall speed", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 0.1, Max: 2.5, Step: 0.05, Default: 0.7,
				Description: "How fast a drop tracks downward in cells per tick once it starts falling."},
			{Key: "drop_min_size", Label: "drop min", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.2,
				Description: "Smallest stable drop radius in pane cells."},
			{Key: "drop_max_size", Label: "drop max", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 1, Max: 6, Step: 0.1, Default: 3.4,
				Description: "Largest stable drop radius in pane cells."},
			{Key: "crit_size", Label: "crit size", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 1, Max: 6, Step: 0.1, Default: 2.6,
				Description: "Drop radius at which a held drop trips over its critical mass and starts running."},
			{Key: "trail_len", Label: "trail len", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 4, Max: 60, Step: 1, Default: 18,
				Description: "How many cells of residue track each falling drop leaves behind it."},
			{Key: "frame_thick", Label: "frame thick", Slot: SlotLever, Group: "pane", Type: KnobFloat, Min: 0, Max: 8, Step: 0.5, Default: 2,
				Description: "Width of the suggested window-frame silhouette around the pane."},
			{Key: "bg_hue", Label: "bg hue", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 30,
				Description: "Background light hue. Warm values (10–60) read as interior; cool values (190–230) as city night."},
			{Key: "bg_sat", Label: "bg sat", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Saturation of the background glow."},
			{Key: "bg_light", Label: "bg light", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.22,
				Description: "Base lightness of the background glow."},
			{Key: "bg_glow", Label: "bg glow", Slot: SlotLever, Group: "light", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.6,
				Description: "Strength of the diffuse halo behind the pane."},
			{Key: "drop_hue", Label: "drop hue", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 210,
				Description: "Hue used inside drops when refracting the background light."},
			{Key: "drop_sat", Label: "drop sat", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.18,
				Description: "Saturation of the drop bodies and their refraction."},
			{Key: "hl_light", Label: "highlight", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.02, Default: 0.85,
				Description: "Lightness of the bright catch-light pixels on each drop."},
			{Key: "sh_light", Label: "shadow", Slot: SlotLever, Group: "drops", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.02, Default: 0.18,
				Description: "Lightness of the rim shadow that anchors each drop on the pane."},
			{Key: "form_p", Label: "drop-form", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.0005, Default: 0, Trigger: "drop-form",
				Description: "Per-tick chance of nucleating a fresh drop somewhere on the pane."},
			{Key: "fall_p", Label: "drop-fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "drop-fall",
				Description: "Per-tick chance of forcing one settled drop past the critical size into a run."},
			{Key: "gust_p", Label: "wind-gust", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wind-gust",
				Description: "Per-tick chance of a brief sideways gust skewing falling drops."},
			{Key: "quiet_p", Label: "quiet-pane", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-pane",
				Description: "Per-tick chance of a long suppression window where new drops are rare."},
			{Key: "gust_dur", Label: "gust dur", Slot: SlotEventMod, Group: "wind-gust", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 50,
				Description: "Duration of an active wind-gust in ticks."},
			{Key: "gust_skew", Label: "gust skew", Slot: SlotEventMod, Group: "wind-gust", Type: KnobFloat, Min: 0.05, Max: 1.5, Step: 0.05, Default: 0.6,
				Description: "How far falling drops are deflected sideways while a gust is active."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-pane", Type: KnobInt, Min: 20, Max: 400, Step: 10, Default: 90,
				Description: "Duration of a quiet-pane window in ticks."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-pane", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Spawn-rate multiplier applied while quiet-pane is active."},
		},
	}
}

// RowDrop is the wire shape of a single drop slot.
type RowDrop struct {
	State    byte    `json:"s"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Size     float64 `json:"sz"`
	Target   float64 `json:"tgt"`
	PhaseLeft int    `json:"pl"`
	PhaseTotal int   `json:"pt"`
	StartY   float64 `json:"sy"`
	VelX     float64 `json:"vx"`
}

// RainOnWindowState is the wire/persisted snapshot of the pane.
type RainOnWindowState struct {
	Tick         int       `json:"tick"`
	Drops        []RowDrop `json:"drops"`
	IntroTicks   int       `json:"introTicks"`
	IntroTotal   int       `json:"introTotal"`
	EndingTicks  int       `json:"endingTicks"`
	EndingTotal  int       `json:"endingTotal"`
	EndingFade   int       `json:"endingFade"`
	GustTicks    int       `json:"gustTicks"`
	GustTotal    int       `json:"gustTotal"`
	GustDir      float64   `json:"gustDir"`
	QuietTicks   int       `json:"quietTicks"`
	QuietTotal   int       `json:"quietTotal"`
	RNGState     uint64    `json:"rngState,omitempty"`
}

type RainOnWindowSnapshot struct {
	RainOnWindowState
}

type RainOnWindowPersistedState struct {
	RainOnWindowState
}

// RainOnWindow is the authoritative server-side pane sim.
type RainOnWindow struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  RainOnWindowConfig
	tick int

	drops []RowDrop

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int

	gustTicks int
	gustTotal int
	gustDir   float64

	quietTicks int
	quietTotal int

	log []LogEntry
}

func NewRainOnWindow(w, h int, seed int64, cfg RainOnWindowConfig) *RainOnWindow {
	r := &RainOnWindow{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	r.drops = make([]RowDrop, r.cfg.SlotCount)
	return r
}

func (r *RainOnWindow) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.W = w
	r.H = h
}

func (r *RainOnWindow) SetConfig(cfg RainOnWindowConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := cfg.withDefaults()
	if next.SlotCount != r.cfg.SlotCount {
		r.cfg = next
		r.drops = make([]RowDrop, next.SlotCount)
		return
	}
	r.cfg = next
}

func (r *RainOnWindow) EffectiveConfig() RainOnWindowConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *RainOnWindow) CurrentTick() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tick
}

func (r *RainOnWindow) PerturbRNG(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Mix(delta)
}

func (r *RainOnWindow) DrainLog() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) == 0 {
		return nil
	}
	out := r.log
	r.log = nil
	return out
}

func (r *RainOnWindow) appendLog(kind, desc string) {
	r.log = append(r.log, LogEntry{Tick: r.tick, Type: kind, Desc: desc})
	if len(r.log) > 200 {
		r.log = r.log[len(r.log)-200:]
	}
}

func (r *RainOnWindow) Snapshot() RainOnWindowSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainOnWindowSnapshot{RainOnWindowState: r.snapshotStateLocked(false)}
}

func (r *RainOnWindow) RestoreSnapshot(s RainOnWindowSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(s.RainOnWindowState)
}

func (r *RainOnWindow) SnapshotPersistedState() RainOnWindowPersistedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RainOnWindowPersistedState{RainOnWindowState: r.snapshotStateLocked(true)}
}

func (r *RainOnWindow) RestorePersistedState(s RainOnWindowPersistedState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreStateLocked(s.RainOnWindowState)
	if s.RNGState != 0 {
		r.rng.SetState(s.RNGState)
	}
}

func (r *RainOnWindow) snapshotStateLocked(includeRNG bool) RainOnWindowState {
	drops := make([]RowDrop, len(r.drops))
	copy(drops, r.drops)
	out := RainOnWindowState{
		Tick:        r.tick,
		Drops:       drops,
		IntroTicks:  r.introTicks,
		IntroTotal:  r.introTotal,
		EndingTicks: r.endingTicks,
		EndingTotal: r.endingTotal,
		EndingFade:  r.endingFade,
		GustTicks:   r.gustTicks,
		GustTotal:   r.gustTotal,
		GustDir:     r.gustDir,
		QuietTicks:  r.quietTicks,
		QuietTotal:  r.quietTotal,
	}
	if includeRNG {
		out.RNGState = r.rng.State()
	}
	return out
}

func (r *RainOnWindow) restoreStateLocked(s RainOnWindowState) {
	r.tick = s.Tick
	if len(s.Drops) > 0 {
		r.drops = make([]RowDrop, len(s.Drops))
		copy(r.drops, s.Drops)
	} else if len(r.drops) != r.cfg.SlotCount {
		r.drops = make([]RowDrop, r.cfg.SlotCount)
	}
	r.introTicks = s.IntroTicks
	r.introTotal = s.IntroTotal
	r.endingTicks = s.EndingTicks
	r.endingTotal = s.EndingTotal
	r.endingFade = s.EndingFade
	r.gustTicks = s.GustTicks
	r.gustTotal = s.GustTotal
	r.gustDir = s.GustDir
	r.quietTicks = s.QuietTicks
	r.quietTotal = s.QuietTotal
}

// TriggerEvent fires a discrete event by name. Returns true if the event
// is known to this effect.
func (r *RainOnWindow) TriggerEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "drop-form":
		idx, ok := r.pickEmptySlotLocked()
		if !ok {
			r.appendLog("drop-form", "skipped (pane full)")
			return true
		}
		r.spawnDropLocked(idx, false)
		r.appendLog("drop-form", fmt.Sprintf("slot %d", idx+1))
	case "drop-fall":
		idx, ok := r.pickHoldingLocked()
		if !ok {
			r.appendLog("drop-fall", "skipped (no settled drops)")
			return true
		}
		r.startFallLocked(idx)
		r.appendLog("drop-fall", fmt.Sprintf("slot %d", idx+1))
	case "drop-merge":
		a, b, ok := r.pickMergePairLocked()
		if !ok {
			r.appendLog("drop-merge", "skipped (no neighbor pair)")
			return true
		}
		r.mergeDropsLocked(a, b)
		r.appendLog("drop-merge", fmt.Sprintf("slots %d+%d", a+1, b+1))
	case "wind-gust":
		r.startGustLocked("triggered")
	case "quiet-pane":
		r.startQuietLocked("triggered")
	case "intro":
		r.startIntroLocked()
		r.appendLog("intro", fmt.Sprintf("started (dur=%d, burst=%.2f)", r.introTotal, r.cfg.IntroBurst))
	case "ending":
		r.startEndingLocked()
		r.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", r.endingFade, r.endingTotal-r.endingFade))
	default:
		return false
	}
	return true
}

func (r *RainOnWindow) pickEmptySlotLocked() (int, bool) {
	open := make([]int, 0, len(r.drops))
	for i, d := range r.drops {
		if d.State == ROWEmpty {
			open = append(open, i)
		}
	}
	if len(open) == 0 {
		return 0, false
	}
	return open[r.rng.Intn(len(open))], true
}

func (r *RainOnWindow) pickHoldingLocked() (int, bool) {
	candidates := make([]int, 0, len(r.drops))
	for i, d := range r.drops {
		if d.State == ROWHolding {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		// Fall back to any forming drop near critical so the trigger always
		// reads visibly rather than silently skipping.
		for i, d := range r.drops {
			if d.State == ROWForming && d.Size >= r.cfg.CritSize*0.6 {
				candidates = append(candidates, i)
			}
		}
		if len(candidates) == 0 {
			return 0, false
		}
	}
	return candidates[r.rng.Intn(len(candidates))], true
}

// pickMergePairLocked finds two drops whose pane positions are within a
// few cells of each other so the merge reads as physical rather than
// random.
func (r *RainOnWindow) pickMergePairLocked() (int, int, bool) {
	type pair struct{ a, b int }
	pairs := make([]pair, 0, 8)
	radius := math.Max(3, r.cfg.DropMaxSize*1.4)
	for i := range r.drops {
		di := r.drops[i]
		if di.State != ROWForming && di.State != ROWHolding {
			continue
		}
		for j := i + 1; j < len(r.drops); j++ {
			dj := r.drops[j]
			if dj.State != ROWForming && dj.State != ROWHolding {
				continue
			}
			if math.Abs(di.X-dj.X) > radius {
				continue
			}
			if math.Abs(di.Y-dj.Y) > radius {
				continue
			}
			pairs = append(pairs, pair{i, j})
		}
	}
	if len(pairs) == 0 {
		return 0, 0, false
	}
	p := pairs[r.rng.Intn(len(pairs))]
	return p.a, p.b, true
}

func (r *RainOnWindow) spawnDropLocked(idx int, fromIntro bool) {
	if idx < 0 || idx >= len(r.drops) {
		return
	}
	maxSize := math.Max(r.cfg.DropMinSize, r.cfg.DropMaxSize)
	target := r.cfg.DropMinSize + r.rng.Float64()*(maxSize-r.cfg.DropMinSize)
	formDur := jitterInt(r.rng, r.cfg.FormDur, 0.3)
	if fromIntro {
		// Stagger intro starts so they don't all land on the same tick.
		formDur = jitterInt(r.rng, r.cfg.FormDur/2, 0.4)
	}
	frame := r.cfg.FrameThick + 1
	usableW := math.Max(4, float64(r.W)-2*frame)
	usableH := math.Max(8, float64(r.H)*0.7-2*frame)
	x := frame + r.rng.Float64()*usableW
	y := frame + r.rng.Float64()*usableH
	if r.gustTicks > 0 {
		// Bias new spawns toward the windward edge.
		bias := r.gustDir * r.cfg.GustSkew
		x = clamp01((x-frame)/usableW+bias*0.18)
		x = frame + clamp01(x)*usableW
	}
	r.drops[idx] = RowDrop{
		State:      ROWForming,
		X:          x,
		Y:          y,
		Size:       math.Max(0.4, r.cfg.DropMinSize*0.4),
		Target:     target,
		PhaseLeft:  formDur,
		PhaseTotal: formDur,
		StartY:     y,
	}
}

func (r *RainOnWindow) startFallLocked(idx int) {
	if idx < 0 || idx >= len(r.drops) {
		return
	}
	d := r.drops[idx]
	if d.State == ROWEmpty || d.State == ROWFalling {
		return
	}
	// Bump small drops up to a runable mass so the trigger doesn't dribble.
	if d.Size < r.cfg.CritSize {
		d.Size = r.cfg.CritSize
	}
	d.State = ROWFalling
	d.PhaseLeft = 0
	d.PhaseTotal = 0
	d.StartY = d.Y
	d.VelX = 0
	if r.gustTicks > 0 {
		d.VelX = r.gustDir * r.cfg.GustSkew * (0.6 + r.rng.Float64()*0.4)
	}
	r.drops[idx] = d
}

func (r *RainOnWindow) mergeDropsLocked(a, b int) {
	if a == b || a < 0 || b < 0 || a >= len(r.drops) || b >= len(r.drops) {
		return
	}
	da := r.drops[a]
	db := r.drops[b]
	combined := math.Sqrt(da.Size*da.Size + db.Size*db.Size)
	keep := a
	gone := b
	if db.Y > da.Y {
		keep = b
		gone = a
	}
	merged := r.drops[keep]
	other := r.drops[gone]
	merged.Size = math.Min(combined, math.Max(r.cfg.DropMaxSize, r.cfg.CritSize*1.2))
	merged.X = (da.X*da.Size + db.X*db.Size) / math.Max(0.001, da.Size+db.Size)
	if merged.State == ROWForming {
		merged.PhaseLeft = jitterInt(r.rng, r.cfg.FormDur/3, 0.3)
		merged.PhaseTotal = merged.PhaseLeft
	}
	r.drops[keep] = merged
	r.drops[gone] = RowDrop{State: ROWEmpty}
	_ = other
	if merged.Size >= r.cfg.CritSize {
		r.startFallLocked(keep)
	}
}

func (r *RainOnWindow) startGustLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.GustDur, 0.3)
	r.gustTicks = dur
	r.gustTotal = dur
	if r.rng.Float64() < 0.5 {
		r.gustDir = -1
	} else {
		r.gustDir = 1
	}
	for i := range r.drops {
		if r.drops[i].State == ROWFalling {
			r.drops[i].VelX += r.gustDir * r.cfg.GustSkew * (0.5 + r.rng.Float64()*0.4)
		}
	}
	r.appendLog("wind-gust", fmt.Sprintf("%s (dur=%d, dir=%+.0f)", verb, dur, r.gustDir))
}

func (r *RainOnWindow) startQuietLocked(verb string) {
	dur := jitterInt(r.rng, r.cfg.QuietDur, 0.3)
	r.quietTicks = dur
	r.quietTotal = dur
	r.appendLog("quiet-pane", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, r.cfg.QuietMult))
}

func (r *RainOnWindow) startIntroLocked() {
	r.introTotal = r.cfg.IntroDur
	if r.introTotal <= 0 {
		r.introTotal = 50
	}
	r.introTicks = r.introTotal
	r.endingTicks = 0
	r.endingTotal = 0
	r.endingFade = 0
	r.gustTicks = 0
	r.quietTicks = 0
	for i := range r.drops {
		r.drops[i] = RowDrop{State: ROWEmpty}
	}
	// Seed initial drops so the trope is visible immediately.
	burst := int(math.Round(float64(len(r.drops)) * clamp01(r.cfg.IntroBurst)))
	if burst > len(r.drops) {
		burst = len(r.drops)
	}
	for i := 0; i < burst; i++ {
		r.spawnDropLocked(i, true)
	}
}

func (r *RainOnWindow) startEndingLocked() {
	r.gustTicks = 0
	r.quietTicks = 0
	r.introTicks = 0
	r.introTotal = 0
	r.endingFade = r.cfg.EndingDur
	if r.endingFade <= 0 {
		r.endingFade = 80
	}
	linger := r.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	r.endingTotal = r.endingFade + linger
	if r.endingTotal < 1 {
		r.endingTotal = r.endingFade
	}
	r.endingTicks = r.endingTotal
}

func (r *RainOnWindow) Step() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++

	if len(r.drops) != r.cfg.SlotCount {
		r.drops = make([]RowDrop, r.cfg.SlotCount)
	}

	if r.endingTicks > 0 {
		r.endingTicks--
		if r.endingTicks == 0 {
			r.startIntroLocked()
			r.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, burst=%.2f)", r.introTotal, r.cfg.IntroBurst))
		}
	} else if r.introTicks > 0 {
		r.introTicks--
	}

	if r.gustTicks > 0 {
		r.gustTicks--
		if r.gustTicks == 0 {
			r.gustDir = 0
			r.gustTotal = 0
		}
	}
	if r.quietTicks > 0 {
		r.quietTicks--
		if r.quietTicks == 0 {
			r.quietTotal = 0
		}
	}

	r.advanceDropsLocked()

	if r.endingTicks > 0 {
		return
	}

	r.rollSpawnsLocked()

	if r.cfg.FormChance > 0 && r.rng.Float64() < r.cfg.FormChance {
		if idx, ok := r.pickEmptySlotLocked(); ok {
			r.spawnDropLocked(idx, false)
			r.appendLog("drop-form", fmt.Sprintf("slot %d (rolled)", idx+1))
		}
	}
	if r.cfg.FallChance > 0 && r.rng.Float64() < r.cfg.FallChance {
		if idx, ok := r.pickHoldingLocked(); ok {
			r.startFallLocked(idx)
			r.appendLog("drop-fall", fmt.Sprintf("slot %d (rolled)", idx+1))
		}
	}
	if r.cfg.GustChance > 0 && r.gustTicks == 0 && r.rng.Float64() < r.cfg.GustChance {
		r.startGustLocked("started")
	}
	if r.cfg.QuietChance > 0 && r.quietTicks == 0 && r.rng.Float64() < r.cfg.QuietChance {
		r.startQuietLocked("started")
	}

	// Adjacent forming drops have a small chance to coalesce on contact.
	r.rollMergeLocked()
}

func (r *RainOnWindow) rollSpawnsLocked() {
	// Steady spawn pressure so empty slots refill at roughly the form-dur
	// cadence, scaled by lifecycle/quiet modifiers.
	rate := 1.0 / float64(max(20, r.cfg.FormDur))
	if r.quietTicks > 0 {
		rate *= r.cfg.QuietMult
	}
	if r.introTicks > 0 {
		// Higher pressure during intro for a quick fill.
		progress := phaseProgress(r.introTotal, r.introTicks)
		rate *= 1 + (1-progress)*1.5
	}
	for i, d := range r.drops {
		if d.State != ROWEmpty {
			continue
		}
		if r.rng.Float64() < rate {
			r.spawnDropLocked(i, false)
		}
	}
}

func (r *RainOnWindow) advanceDropsLocked() {
	for i := range r.drops {
		d := r.drops[i]
		switch d.State {
		case ROWForming:
			if d.PhaseLeft > 0 {
				d.PhaseLeft--
			}
			progress := 1 - float64(d.PhaseLeft)/math.Max(1, float64(d.PhaseTotal))
			d.Size = math.Max(0.4, r.cfg.DropMinSize*0.4) + (d.Target-math.Max(0.4, r.cfg.DropMinSize*0.4))*clamp01(progress)
			if d.PhaseLeft <= 0 {
				d.State = ROWHolding
				holdDur := jitterInt(r.rng, r.cfg.HoldDur, 0.4)
				d.PhaseLeft = holdDur
				d.PhaseTotal = holdDur
				d.Size = d.Target
			}
		case ROWHolding:
			if d.PhaseLeft > 0 {
				d.PhaseLeft--
			}
			// While holding, drop slowly grows toward the critical mass.
			if d.Size < r.cfg.CritSize {
				d.Size += (r.cfg.CritSize - d.Size) * 0.0035
			}
			if d.PhaseLeft <= 0 || d.Size >= r.cfg.CritSize {
				r.drops[i] = d
				r.startFallLocked(i)
				continue
			}
		case ROWFalling:
			d.Y += r.cfg.FallSpeed * (0.85 + math.Max(0, d.Size-r.cfg.CritSize)*0.45)
			d.X += d.VelX
			d.VelX *= 0.92
			if r.gustTicks > 0 {
				d.X += r.gustDir * r.cfg.GustSkew * 0.05
			}
			frame := r.cfg.FrameThick + 1
			leftEdge := frame
			rightEdge := math.Max(leftEdge+1, float64(r.W)-frame)
			if d.X < leftEdge {
				d.X = leftEdge
				d.VelX = math.Abs(d.VelX) * 0.5
			}
			if d.X > rightEdge {
				d.X = rightEdge
				d.VelX = -math.Abs(d.VelX) * 0.5
			}
			if d.Y > float64(r.H)-frame {
				d.State = ROWEmpty
				d.Size = 0
			}
		}
		r.drops[i] = d
	}
}

func (r *RainOnWindow) rollMergeLocked() {
	radius := math.Max(2, r.cfg.DropMaxSize*0.9)
	for i := 0; i < len(r.drops); i++ {
		di := r.drops[i]
		if di.State != ROWForming && di.State != ROWHolding {
			continue
		}
		for j := i + 1; j < len(r.drops); j++ {
			dj := r.drops[j]
			if dj.State != ROWForming && dj.State != ROWHolding {
				continue
			}
			dx := di.X - dj.X
			dy := di.Y - dj.Y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}
			// Small per-encounter chance so adjacent drops occasionally
			// catch each other rather than collapsing en masse.
			if r.rng.Float64() > 0.04 {
				continue
			}
			r.mergeDropsLocked(i, j)
			r.appendLog("drop-merge", fmt.Sprintf("slots %d+%d", i+1, j+1))
			break
		}
	}
}
