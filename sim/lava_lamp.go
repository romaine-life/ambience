package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type LavaLampConfig = ProceduralConfig
type LavaLampState = ProceduralState
type LavaLampSnapshot = ProceduralSnapshot
type LavaLampPersistedState = ProceduralPersistedState

// LavaLamp is a dedicated sim type for the lava-lamp effect. The server
// owns lifecycle timers and discrete event rolls; clients run the blob
// physics locally from tick + seed.
type LavaLamp struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    LavaLampConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var lavaLampDefaultsLocal = LavaLampConfig{
	"intro_dur":     60,
	"intro_glow":    0.18,
	"ending_dur":    80,
	"ending_linger": 30,
	"ending_glow":   0.12,
	"horizon":       0.92,
	"bottle_top":    0.08,
	"bottle_width":  0.32,
	"bottle_curve":  0.55,
	"neck_width":    0.18,
	"blob_count":    4,
	"blob_size":     6.5,
	"rise_speed":    0.42,
	"viscosity":     0.55,
	"heat":          0.62,
	"hue":           8,
	"hue_sp":        14,
	"sat":           0.82,
	"lmin":          0.22,
	"lmax":          0.88,
	"bg_hue":        18,
	"bg_sat":        0.32,
	"bg_light":      0.16,
	"rise_p":        0.0,
	"merge_p":       0.0,
	"split_p":       0.0,
	"surface_pop_p": 0.0,
	"quiet_p":       0.0,
	"rise_dur":      80,
	"rise_mult":     1.6,
	"merge_dur":     50,
	"split_dur":     50,
	"surface_dur":   40,
	"quiet_dur":     180,
	"quiet_mult":    0.45,
}

func LavaLampSchema() EffectSchema {
	return EffectSchema{
		Name: "lava-lamp",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent warming the lamp from a dark bottle to its full glow before the first blob detaches."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Starting fraction of the heat glow as the lamp turns on."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent cooling the lamp — heat fades and rising blobs settle to the base."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 180, Step: 5, Default: 30,
				Description: "Extra quiet ticks for blobs to come to rest after the heat is gone."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "Residual heat glow remaining at the base near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.6, Max: 0.98, Step: 0.01, Default: 0.92,
				Description: "Where the bottle base sits in the frame, as a fraction of the canvas height."},
			{Key: "bottle_top", Label: "bottle top", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.0, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Vertical position of the bottle cap as a fraction from the top of the canvas."},
			{Key: "bottle_width", Label: "bottle width", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.15, Max: 0.6, Step: 0.01, Default: 0.32,
				Description: "Horizontal half-width of the bottle's belly relative to the canvas."},
			{Key: "bottle_curve", Label: "bottle curve", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.0, Max: 1.0, Step: 0.02, Default: 0.55,
				Description: "How tapered the bottle silhouette is from belly toward neck (0 = cylindrical, 1 = fully tapered)."},
			{Key: "neck_width", Label: "neck width", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.06, Max: 0.4, Step: 0.01, Default: 0.18,
				Description: "Half-width of the bottle neck near the cap, as a fraction of canvas width."},
			{Key: "blob_count", Label: "blob count", Slot: SlotLever, Group: "blobs", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 4,
				Description: "Number of blobs suspended in the lamp at any given time."},
			{Key: "blob_size", Label: "blob size", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 2.5, Max: 14, Step: 0.5, Default: 6.5,
				Description: "Base radius of each blob in cells before merge/split modifiers."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.05, Default: 0.42,
				Description: "Slow vertical drift rate of the blobs — the lamp's overall rhythm."},
			{Key: "viscosity", Label: "viscosity", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.02, Default: 0.55,
				Description: "How gooey the motion reads — higher values dampen wobble and keep blobs rounder."},
			{Key: "heat", Label: "heat", Slot: SlotLever, Group: "heat", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.02, Default: 0.62,
				Description: "Strength of the warm glow at the base of the bottle."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 8,
				Description: "Base hue of the blobs and heat (0 = red, 120 = green, 200 = blue)."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 14,
				Description: "Variation in hue across the blobs and base glow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.82,
				Description: "Saturation applied to the blobs and warm base."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Minimum lightness, used for the bottle silhouette and dim blob edges."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Maximum lightness, used for the brightest blob cores at peak glow."},
			{Key: "bg_hue", Label: "bg hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 18,
				Description: "Hue of the dim warm liquid suspended inside the bottle."},
			{Key: "bg_sat", Label: "bg sat", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.32,
				Description: "Saturation of the bottle's interior liquid."},
			{Key: "bg_light", Label: "bg light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Lightness of the bottle's interior liquid."},
			{Key: "rise_p", Label: "blob rise", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-rise",
				Description: "Per-tick chance of a fresh blob detaching from the heated base and beginning to rise."},
			{Key: "merge_p", Label: "blob merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-merge",
				Description: "Per-tick chance of two adjacent blobs combining into a larger one."},
			{Key: "split_p", Label: "blob split", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-split",
				Description: "Per-tick chance of a large blob splitting while ascending or descending."},
			{Key: "surface_pop_p", Label: "surface pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surface-pop",
				Description: "Per-tick chance of a blob touching the cap, flattening, and starting to sink back down."},
			{Key: "quiet_p", Label: "quiet flow", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "quiet-flow",
				Description: "Per-tick chance of the lamp settling into a long quiet window with fewer detachments."},
			{Key: "rise_dur", Label: "rise dur", Slot: SlotEventMod, Group: "blob-rise", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80,
				Description: "Duration of a fresh-rise pulse in ticks (jittered by +/-30%)."},
			{Key: "rise_mult", Label: "rise x", Slot: SlotEventMod, Group: "blob-rise", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.6,
				Description: "Glow and motion multiplier applied to the freshly detached blob during its rise."},
			{Key: "merge_dur", Label: "merge dur", Slot: SlotEventMod, Group: "blob-merge", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 50,
				Description: "Duration of a merge — how long two blobs stay visibly fused before settling."},
			{Key: "split_dur", Label: "split dur", Slot: SlotEventMod, Group: "blob-split", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 50,
				Description: "Duration of a split — how long the parent and child stretch before becoming independent."},
			{Key: "surface_dur", Label: "surface dur", Slot: SlotEventMod, Group: "surface-pop", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 40,
				Description: "Duration of a surface-pop flatten before the blob starts sinking again."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobInt, Min: 60, Max: 600, Step: 10, Default: 180,
				Description: "Duration of a quiet-flow window in ticks, during which the lamp settles."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Motion multiplier applied while a quiet-flow window is active (lower = stiller lamp)."},
		},
	}
}

func defaultLavaLampConfig() LavaLampConfig { return cloneConfig(lavaLampDefaultsLocal) }

func mergeLavaLampDefaults(cfg LavaLampConfig) LavaLampConfig {
	out := defaultLavaLampConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = lavaLampDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = lavaLampDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["horizon"] <= 0 {
		out["horizon"] = lavaLampDefaultsLocal["horizon"]
	}
	out["horizon"] = clamp01(out["horizon"])
	if out["bottle_top"] < 0 {
		out["bottle_top"] = 0
	}
	out["bottle_top"] = clamp01(out["bottle_top"])
	if out["bottle_width"] <= 0 {
		out["bottle_width"] = lavaLampDefaultsLocal["bottle_width"]
	}
	out["bottle_curve"] = clamp01(out["bottle_curve"])
	if out["neck_width"] <= 0 {
		out["neck_width"] = lavaLampDefaultsLocal["neck_width"]
	}
	if out["blob_count"] < 1 {
		out["blob_count"] = 1
	}
	if out["blob_size"] <= 0 {
		out["blob_size"] = lavaLampDefaultsLocal["blob_size"]
	}
	if out["rise_speed"] <= 0 {
		out["rise_speed"] = lavaLampDefaultsLocal["rise_speed"]
	}
	if out["viscosity"] <= 0 {
		out["viscosity"] = lavaLampDefaultsLocal["viscosity"]
	}
	if out["heat"] <= 0 {
		out["heat"] = lavaLampDefaultsLocal["heat"]
	}
	if out["hue"] < 0 {
		out["hue"] = math.Mod(out["hue"], 360) + 360
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = lavaLampDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = lavaLampDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = lavaLampDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["bg_sat"] < 0 {
		out["bg_sat"] = 0
	}
	if out["bg_light"] <= 0 {
		out["bg_light"] = lavaLampDefaultsLocal["bg_light"]
	}
	if out["rise_dur"] <= 0 {
		out["rise_dur"] = lavaLampDefaultsLocal["rise_dur"]
	}
	if out["rise_mult"] <= 0 {
		out["rise_mult"] = lavaLampDefaultsLocal["rise_mult"]
	}
	if out["merge_dur"] <= 0 {
		out["merge_dur"] = lavaLampDefaultsLocal["merge_dur"]
	}
	if out["split_dur"] <= 0 {
		out["split_dur"] = lavaLampDefaultsLocal["split_dur"]
	}
	if out["surface_dur"] <= 0 {
		out["surface_dur"] = lavaLampDefaultsLocal["surface_dur"]
	}
	if out["quiet_dur"] <= 0 {
		out["quiet_dur"] = lavaLampDefaultsLocal["quiet_dur"]
	}
	if out["quiet_mult"] <= 0 {
		out["quiet_mult"] = lavaLampDefaultsLocal["quiet_mult"]
	}
	return out
}

func NewLavaLamp(w, h int, seed int64, cfg LavaLampConfig) *LavaLamp {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &LavaLamp{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeLavaLampDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (l *LavaLamp) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if w == l.W && h == l.H {
		return
	}
	l.W = w
	l.H = h
	l.Grid = make([][]Pixel, h)
	for i := range l.Grid {
		l.Grid[i] = make([]Pixel, w)
	}
}

func (l *LavaLamp) SetConfig(cfg LavaLampConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg = mergeLavaLampDefaults(cfg)
}

func (l *LavaLamp) EffectiveConfig() LavaLampConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	return cloneConfig(l.cfg)
}

func (l *LavaLamp) Snapshot() LavaLampSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LavaLampSnapshot{ProceduralState: l.snapshotStateLocked()}
}

func (l *LavaLamp) RestoreSnapshot(snap LavaLampSnapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(snap.ProceduralState)
}

func (l *LavaLamp) SnapshotPersistedState() LavaLampPersistedState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LavaLampPersistedState{
		ProceduralState: l.snapshotStateLocked(),
		RNGState:        l.rng.State(),
	}
}

func (l *LavaLamp) RestorePersistedState(ps LavaLampPersistedState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		l.rng.SetState(ps.RNGState)
	}
}

func (l *LavaLamp) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   l.tick,
		Timers: cloneTimerMap(l.timers),
		Values: cloneValueMap(l.values),
	}
}

func (l *LavaLamp) restoreStateLocked(state ProceduralState) {
	l.tick = state.Tick
	l.timers = cloneTimerMap(state.Timers)
	if l.timers == nil {
		l.timers = make(map[string]int)
	}
	l.values = cloneValueMap(state.Values)
	if l.values == nil {
		l.values = make(map[string]float64)
	}
}

func (l *LavaLamp) CurrentTick() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tick
}

func (l *LavaLamp) PerturbRNG(delta int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rng.Mix(delta)
}

func (l *LavaLamp) DrainLog() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.log) == 0 {
		return nil
	}
	out := l.log
	l.log = nil
	return out
}

func (l *LavaLamp) appendLog(kind, desc string) {
	l.log = append(l.log, LogEntry{Tick: l.tick, Type: kind, Desc: desc})
	if len(l.log) > 200 {
		l.log = l.log[len(l.log)-200:]
	}
}

func (l *LavaLamp) intCfg(key string) int {
	return int(math.Round(l.cfg[key]))
}

func (l *LavaLamp) TriggerEvent(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch name {
	case "blob-rise":
		l.startRiseLocked("triggered")
	case "blob-merge":
		l.startMergeLocked("triggered")
	case "blob-split":
		l.startSplitLocked("triggered")
	case "surface-pop":
		l.startSurfacePopLocked("triggered")
	case "quiet-flow":
		l.startQuietLocked("triggered")
	case "intro":
		l.startIntroLocked()
		l.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", l.timers["intro"], l.cfg["intro_glow"]))
	case "ending":
		l.startEndingLocked()
		l.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", l.intCfg("ending_dur"), l.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (l *LavaLamp) Step() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tick++
	for key, value := range l.timers {
		if value > 0 {
			l.timers[key] = value - 1
		}
	}
	l.stepLocked()
}

func (l *LavaLamp) startRiseLocked(verb string) {
	l.timers["rise"] = jitterInt(l.rng, l.intCfg("rise_dur"), 0.3)
	l.values["rise_gain"] = l.cfg["rise_mult"] * (0.85 + l.rng.Float64()*0.3)
	// Pick which blob slot the fresh detachment animates around.
	count := l.intCfg("blob_count")
	if count < 1 {
		count = 1
	}
	l.values["rise_slot"] = float64(l.rng.Intn(count))
	l.appendLog("blob-rise", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["rise"], l.values["rise_gain"]))
}

func (l *LavaLamp) startMergeLocked(verb string) {
	l.timers["merge"] = jitterInt(l.rng, l.intCfg("merge_dur"), 0.3)
	count := l.intCfg("blob_count")
	if count < 1 {
		count = 1
	}
	l.values["merge_slot"] = float64(l.rng.Intn(count))
	l.appendLog("blob-merge", fmt.Sprintf("%s (dur=%d)", verb, l.timers["merge"]))
}

func (l *LavaLamp) startSplitLocked(verb string) {
	l.timers["split"] = jitterInt(l.rng, l.intCfg("split_dur"), 0.3)
	count := l.intCfg("blob_count")
	if count < 1 {
		count = 1
	}
	l.values["split_slot"] = float64(l.rng.Intn(count))
	l.appendLog("blob-split", fmt.Sprintf("%s (dur=%d)", verb, l.timers["split"]))
}

func (l *LavaLamp) startSurfacePopLocked(verb string) {
	l.timers["surface"] = jitterInt(l.rng, l.intCfg("surface_dur"), 0.3)
	count := l.intCfg("blob_count")
	if count < 1 {
		count = 1
	}
	l.values["surface_slot"] = float64(l.rng.Intn(count))
	l.appendLog("surface-pop", fmt.Sprintf("%s (dur=%d)", verb, l.timers["surface"]))
}

func (l *LavaLamp) startQuietLocked(verb string) {
	l.timers["quiet"] = jitterInt(l.rng, l.intCfg("quiet_dur"), 0.2)
	l.appendLog("quiet-flow", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["quiet"], l.cfg["quiet_mult"]))
}

func (l *LavaLamp) startIntroLocked() {
	l.timers["rise"] = 0
	l.timers["merge"] = 0
	l.timers["split"] = 0
	l.timers["surface"] = 0
	l.timers["quiet"] = 0
	l.timers["ending"] = 0
	l.values["rise_gain"] = 1
	l.timers["intro"] = l.intCfg("intro_dur")
	l.values["intro_total"] = float64(l.timers["intro"])
}

func (l *LavaLamp) startEndingLocked() {
	l.timers["intro"] = 0
	l.timers["rise"] = 0
	l.timers["merge"] = 0
	l.timers["split"] = 0
	l.timers["surface"] = 0
	l.timers["quiet"] = 0
	l.values["rise_gain"] = 1
	endingTotal := l.intCfg("ending_dur") + max(0, l.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, l.intCfg("ending_dur"))
	}
	l.timers["ending"] = endingTotal
	l.values["ending_total"] = float64(endingTotal)
}

func (l *LavaLamp) stepLocked() {
	if l.timers["rise"] <= 0 {
		l.values["rise_gain"] = 1
	}
	if l.timers["intro"] <= 0 {
		delete(l.values, "intro_total")
	}
	if l.timers["ending"] <= 0 {
		delete(l.values, "ending_total")
	}
	if l.timers["intro"] > 0 || l.timers["ending"] > 0 {
		return
	}
	if l.timers["rise"] <= 0 && l.cfg["rise_p"] > 0 && l.rng.Float64() < l.cfg["rise_p"] {
		l.startRiseLocked("started")
	}
	if l.timers["merge"] <= 0 && l.cfg["merge_p"] > 0 && l.rng.Float64() < l.cfg["merge_p"] {
		l.startMergeLocked("started")
	}
	if l.timers["split"] <= 0 && l.cfg["split_p"] > 0 && l.rng.Float64() < l.cfg["split_p"] {
		l.startSplitLocked("started")
	}
	if l.timers["surface"] <= 0 && l.cfg["surface_pop_p"] > 0 && l.rng.Float64() < l.cfg["surface_pop_p"] {
		l.startSurfacePopLocked("started")
	}
	if l.timers["quiet"] <= 0 && l.cfg["quiet_p"] > 0 && l.rng.Float64() < l.cfg["quiet_p"] {
		l.startQuietLocked("started")
	}
}
