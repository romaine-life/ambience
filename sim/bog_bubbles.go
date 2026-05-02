package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

// BogBubbles is the authority side of the bog-bubbles effect: a glassy bog
// surface where slow, large methane bubbles rise, surface, and pop with a
// brief ripple. The server owns lifecycle (intro/ending) plus the
// methane-burst / quiet-bog event timers; the per-bubble motion is rendered
// deterministically client-side from tick + seed + the shared timer values
// (sister to underwater (#32) but the action is at the surface).
type BogBubblesConfig = ProceduralConfig
type BogBubblesState = ProceduralState
type BogBubblesSnapshot = ProceduralSnapshot
type BogBubblesPersistedState = ProceduralPersistedState

type BogBubbles struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    BogBubblesConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var bogBubblesDefaultsLocal = BogBubblesConfig{
	"intro_dur":     80,
	"intro_first":   30,
	"ending_dur":    90,
	"ending_linger": 60,
	"spawn_rate":    1.6,
	"rise_speed":    0.18,
	"bubble_size":   3.5,
	"viscosity":     0.6,
	"water_level":   0.55,
	"mist":          0.18,
	"ripple_life":   26,
	"ripple_size":   8,
	"hue":           110,
	"hue_sp":        14,
	"sat":           0.42,
	"lmin":          0.10,
	"lmax":          0.78,
	"burst_p":       0,
	"quiet_p":       0,
	"burst_dur":     40,
	"burst_mult":    2.6,
	"quiet_dur":     100,
	"quiet_mult":    0.25,
}

func BogBubblesSchema() EffectSchema {
	return EffectSchema{
		Name: "bog-bubbles",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "intro",
				Description: "Ticks easing the bog out of glassy still into the rhythmic bubble loop."},
			{Key: "intro_first", Label: "first bubble", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 30,
				Description: "Ticks of completely still surface at the start of intro before the first bubble rises."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 90, Trigger: "ending",
				Description: "Ticks during which new bubbles stop spawning and in-flight bubbles finish their rise."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 60,
				Description: "Extra still ticks after spawns stop so the final ripples can fully decay."},
			{Key: "spawn_rate", Label: "bubble rate", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 0.2, Max: 6, Step: 0.1, Default: 1.6,
				Description: "Average bubbles surfacing per ~100 ticks. Lower values feel sparse and unsettling."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Cells per tick a bubble climbs. Keep slow for the viscous methane feel."},
			{Key: "bubble_size", Label: "bubble size", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 1.5, Max: 7, Step: 0.5, Default: 3.5,
				Description: "Max bubble diameter in cells. Larger reads as a fat belch from deep mud."},
			{Key: "viscosity", Label: "viscosity", Slot: SlotLever, Group: "bubbles", Type: KnobFloat, Min: 0, Max: 1.5, Step: 0.05, Default: 0.6,
				Description: "How much each bubble wobbles and distorts the surface as it climbs."},
			{Key: "water_level", Label: "water level", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0.3, Max: 0.8, Step: 0.01, Default: 0.55,
				Description: "Vertical position of the bog surface as a fraction of frame height."},
			{Key: "mist", Label: "mist", Slot: SlotLever, Group: "water", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Strength of the haze drifting above the water surface."},
			{Key: "ripple_life", Label: "ripple life", Slot: SlotLever, Group: "ripples", Type: KnobInt, Min: 6, Max: 100, Step: 2, Default: 26,
				Description: "Ticks each surface-pop ripple lasts before fully decaying."},
			{Key: "ripple_size", Label: "ripple size", Slot: SlotLever, Group: "ripples", Type: KnobFloat, Min: 2, Max: 18, Step: 0.5, Default: 8,
				Description: "Max ripple radius in cells from a popped bubble."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 30, Max: 220, Step: 1, Default: 110,
				Description: "Base bog hue. Lower values lean tar/amber; higher values lean swampy green or icy."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 36, Step: 1, Default: 14,
				Description: "Variation across the surface, mist, and bubble shading."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.02, Default: 0.42,
				Description: "Overall bog saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.04, Max: 0.5, Step: 0.01, Default: 0.10,
				Description: "Minimum lightness used for bog depth and silhouettes."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.3, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for bubble highlights and surface mist."},
			{Key: "burst_p", Label: "methane burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "methane-burst",
				Description: "Per-tick chance of a rare cluster of 3–5 bubbles surfacing in quick succession."},
			{Key: "quiet_p", Label: "quiet bog", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-bog",
				Description: "Per-tick chance of a long suppression window where the surface barely moves."},
			{Key: "burst_dur", Label: "burst dur", Slot: SlotEventMod, Group: "methane-burst", Type: KnobInt, Min: 8, Max: 200, Step: 4, Default: 40,
				Description: "Duration of a methane-burst window."},
			{Key: "burst_mult", Label: "burst x", Slot: SlotEventMod, Group: "methane-burst", Type: KnobFloat, Min: 1.05, Max: 5, Step: 0.05, Default: 2.6,
				Description: "Bubble spawn multiplier applied during a methane burst."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-bog", Type: KnobInt, Min: 20, Max: 400, Step: 5, Default: 100,
				Description: "Duration of a quiet-bog suppression window."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-bog", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.25,
				Description: "Bubble spawn multiplier applied while a quiet-bog window is active."},
		},
	}
}

func defaultBogBubblesConfig() BogBubblesConfig { return cloneConfig(bogBubblesDefaultsLocal) }

func mergeBogBubblesDefaults(cfg BogBubblesConfig) BogBubblesConfig {
	out := defaultBogBubblesConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = bogBubblesDefaultsLocal["intro_dur"]
	}
	if out["intro_first"] < 0 {
		out["intro_first"] = 0
	}
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = bogBubblesDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	if out["spawn_rate"] <= 0 {
		out["spawn_rate"] = bogBubblesDefaultsLocal["spawn_rate"]
	}
	if out["rise_speed"] <= 0 {
		out["rise_speed"] = bogBubblesDefaultsLocal["rise_speed"]
	}
	if out["bubble_size"] <= 0 {
		out["bubble_size"] = bogBubblesDefaultsLocal["bubble_size"]
	}
	if out["viscosity"] < 0 {
		out["viscosity"] = 0
	}
	if out["water_level"] <= 0 {
		out["water_level"] = bogBubblesDefaultsLocal["water_level"]
	}
	if out["mist"] < 0 {
		out["mist"] = 0
	}
	if out["ripple_life"] <= 0 {
		out["ripple_life"] = bogBubblesDefaultsLocal["ripple_life"]
	}
	if out["ripple_size"] <= 0 {
		out["ripple_size"] = bogBubblesDefaultsLocal["ripple_size"]
	}
	if out["hue"] == 0 {
		out["hue"] = bogBubblesDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = bogBubblesDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = bogBubblesDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = bogBubblesDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["burst_p"] < 0 {
		out["burst_p"] = 0
	}
	if out["quiet_p"] < 0 {
		out["quiet_p"] = 0
	}
	if out["burst_dur"] <= 0 {
		out["burst_dur"] = bogBubblesDefaultsLocal["burst_dur"]
	}
	if out["burst_mult"] <= 0 {
		out["burst_mult"] = bogBubblesDefaultsLocal["burst_mult"]
	}
	if out["quiet_dur"] <= 0 {
		out["quiet_dur"] = bogBubblesDefaultsLocal["quiet_dur"]
	}
	if out["quiet_mult"] < 0 {
		out["quiet_mult"] = 0
	}
	return out
}

func NewBogBubbles(w, h int, seed int64, cfg BogBubblesConfig) *BogBubbles {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &BogBubbles{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeBogBubblesDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (b *BogBubbles) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if w == b.W && h == b.H {
		return
	}
	b.W = w
	b.H = h
	b.Grid = make([][]Pixel, h)
	for i := range b.Grid {
		b.Grid[i] = make([]Pixel, w)
	}
}

func (b *BogBubbles) SetConfig(cfg BogBubblesConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cfg = mergeBogBubblesDefaults(cfg)
}

func (b *BogBubbles) EffectiveConfig() BogBubblesConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneConfig(b.cfg)
}

func (b *BogBubbles) Snapshot() BogBubblesSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BogBubblesSnapshot{ProceduralState: b.snapshotStateLocked()}
}

func (b *BogBubbles) RestoreSnapshot(snap BogBubblesSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(snap.ProceduralState)
}

func (b *BogBubbles) SnapshotPersistedState() BogBubblesPersistedState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BogBubblesPersistedState{
		ProceduralState: b.snapshotStateLocked(),
		RNGState:        b.rng.State(),
	}
}

func (b *BogBubbles) RestorePersistedState(ps BogBubblesPersistedState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		b.rng.SetState(ps.RNGState)
	}
}

func (b *BogBubbles) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   b.tick,
		Timers: cloneTimerMap(b.timers),
		Values: cloneValueMap(b.values),
	}
}

func (b *BogBubbles) restoreStateLocked(state ProceduralState) {
	b.tick = state.Tick
	b.timers = cloneTimerMap(state.Timers)
	if b.timers == nil {
		b.timers = make(map[string]int)
	}
	b.values = cloneValueMap(state.Values)
	if b.values == nil {
		b.values = make(map[string]float64)
	}
}

func (b *BogBubbles) CurrentTick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tick
}

func (b *BogBubbles) PerturbRNG(delta int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rng.Mix(delta)
}

func (b *BogBubbles) DrainLog() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) == 0 {
		return nil
	}
	out := b.log
	b.log = nil
	return out
}

func (b *BogBubbles) appendLog(kind, desc string) {
	b.log = append(b.log, LogEntry{Tick: b.tick, Type: kind, Desc: desc})
	if len(b.log) > 200 {
		b.log = b.log[len(b.log)-200:]
	}
}

func (b *BogBubbles) intCfg(key string) int {
	return int(math.Round(b.cfg[key]))
}

func (b *BogBubbles) TriggerEvent(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch name {
	case "methane-burst":
		b.startBurstLocked("triggered")
	case "quiet-bog":
		b.startQuietLocked("triggered")
	case "intro":
		b.startIntroLocked()
		b.appendLog("intro", fmt.Sprintf("started (dur=%d, first=%d)", b.timers["intro"], b.intCfg("intro_first")))
	case "ending":
		b.startEndingLocked()
		b.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", b.intCfg("ending_dur"), b.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (b *BogBubbles) Step() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tick++
	for key, value := range b.timers {
		if value > 0 {
			b.timers[key] = value - 1
		}
	}
	b.stepLocked()
}

func (b *BogBubbles) startBurstLocked(verb string) {
	b.timers["methane-burst"] = jitterInt(b.rng, b.intCfg("burst_dur"), 0.3)
	b.timers["quiet-bog"] = 0
	b.values["spawn_gain"] = b.cfg["burst_mult"] * (0.85 + b.rng.Float64()*0.3)
	b.appendLog("methane-burst", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, b.timers["methane-burst"], b.values["spawn_gain"]))
}

func (b *BogBubbles) startQuietLocked(verb string) {
	b.timers["quiet-bog"] = jitterInt(b.rng, b.intCfg("quiet_dur"), 0.3)
	b.timers["methane-burst"] = 0
	b.values["spawn_gain"] = 1
	b.appendLog("quiet-bog", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, b.timers["quiet-bog"], b.cfg["quiet_mult"]))
}

func (b *BogBubbles) startIntroLocked() {
	b.timers["methane-burst"] = 0
	b.timers["quiet-bog"] = 0
	b.timers["ending"] = 0
	b.values["spawn_gain"] = 1
	b.timers["intro"] = b.intCfg("intro_dur")
	b.values["intro_total"] = float64(b.timers["intro"])
}

func (b *BogBubbles) startEndingLocked() {
	b.timers["intro"] = 0
	b.timers["methane-burst"] = 0
	b.timers["quiet-bog"] = 0
	b.values["spawn_gain"] = 1
	endingTotal := b.intCfg("ending_dur") + max(0, b.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, b.intCfg("ending_dur"))
	}
	b.timers["ending"] = endingTotal
	b.values["ending_total"] = float64(endingTotal)
}

func (b *BogBubbles) stepLocked() {
	if b.timers["methane-burst"] <= 0 {
		b.values["spawn_gain"] = 1
	}
	if b.timers["intro"] <= 0 {
		delete(b.values, "intro_total")
	}
	if b.timers["ending"] <= 0 {
		delete(b.values, "ending_total")
	}
	if b.timers["intro"] > 0 || b.timers["ending"] > 0 {
		return
	}
	if b.timers["methane-burst"] <= 0 && b.timers["quiet-bog"] <= 0 && b.cfg["burst_p"] > 0 && b.rng.Float64() < b.cfg["burst_p"] {
		b.startBurstLocked("started")
	}
	if b.timers["quiet-bog"] <= 0 && b.timers["methane-burst"] <= 0 && b.cfg["quiet_p"] > 0 && b.rng.Float64() < b.cfg["quiet_p"] {
		b.startQuietLocked("started")
	}
}
