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

// LavaLamp is the authority half of the lava-lamp effect. The server owns
// event timing and intro/outro phase, the browser owns the metaball blob
// rendering — blob positions are derived from seed+tick+id rather than
// streamed, so there's no per-blob persisted state on the server.
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
	"intro_dur":        60,
	"intro_glow":       0.14,
	"ending_dur":       70,
	"ending_linger":    24,
	"ending_glow":      0.10,
	"bottle_x":         0.5,
	"bottle_top":       0.10,
	"bottle_bottom":    0.86,
	"bottle_width":     34,
	"neck_width":       12,
	"base_height":      6,
	"blob_count":       4,
	"rise_speed":       0.018,
	"viscosity":        0.55,
	"min_radius":       3.0,
	"max_radius":       6.5,
	"glow":             0.62,
	"hue":              8,
	"hue_sp":           18,
	"sat":              0.78,
	"lmin":             0.18,
	"lmax":             0.92,
	"rise_p":           0.0,
	"merge_p":          0.0,
	"split_p":          0.0,
	"surface_pop_p":    0.0,
	"quiet_flow_p":     0.0,
	"rise_dur":         60,
	"merge_dur":        20,
	"split_dur":        20,
	"surface_pop_dur":  18,
	"quiet_flow_dur":   140,
	"quiet_flow_mult":  0.35,
}

func LavaLampSchema() EffectSchema {
	return EffectSchema{
		Name: "lava-lamp",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent warming the heat source before the first blob detaches."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.14,
				Description: "Starting fraction of the base glow before the lamp settles into rhythm."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent cooling — heat source fades, blobs settle to the base."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 24,
				Description: "Extra quiet ticks for the bottle silhouette to dim after the heat fades."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Residual base glow that remains near the end of the outro."},
			{Key: "bottle_x", Label: "bottle x", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.01, Default: 0.5,
				Description: "Horizontal position of the bottle centerline (0 = left edge, 1 = right edge)."},
			{Key: "bottle_top", Label: "bottle top", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.04, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Top of the bottle silhouette as a fraction of the frame height."},
			{Key: "bottle_bottom", Label: "bottle bottom", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.6, Max: 0.96, Step: 0.01, Default: 0.86,
				Description: "Base of the bottle silhouette as a fraction of the frame height."},
			{Key: "bottle_width", Label: "bottle width", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 16, Max: 60, Step: 1, Default: 34,
				Description: "Width of the bottle at its widest point."},
			{Key: "neck_width", Label: "neck width", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 6, Max: 28, Step: 1, Default: 12,
				Description: "Width of the bottle at the top — narrower than the body for the classic silhouette."},
			{Key: "base_height", Label: "base height", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 2, Max: 16, Step: 1, Default: 6,
				Description: "Vertical thickness of the heat-source pedestal at the bottle base."},
			{Key: "blob_count", Label: "blob count", Slot: SlotLever, Group: "blobs", Type: KnobInt, Min: 1, Max: 8, Step: 1, Default: 4,
				Description: "Steady-state number of blobs suspended in the bottle."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.004, Max: 0.06, Step: 0.001, Default: 0.018,
				Description: "Base rate at which blobs cycle from base to surface — smaller is more languid."},
			{Key: "viscosity", Label: "viscosity", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.1, Max: 1.0, Step: 0.01, Default: 0.55,
				Description: "How gooey the motion feels — higher values bias toward slower, smoother arcs."},
			{Key: "min_radius", Label: "min radius", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 1.5, Max: 6, Step: 0.1, Default: 3.0,
				Description: "Smallest blob radius (in pixels) before merging or splitting."},
			{Key: "max_radius", Label: "max radius", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 3, Max: 14, Step: 0.1, Default: 6.5,
				Description: "Largest blob radius (in pixels) before merging or splitting."},
			{Key: "glow", Label: "base glow", Slot: SlotLever, Group: "heat", Type: KnobFloat, Min: 0.05, Max: 1.0, Step: 0.01, Default: 0.62,
				Description: "Strength of the warm glow rising from the heat source at the bottle's base."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 8,
				Description: "Base blob hue. Classic-red is around 8; blue-cool around 220; green-goo around 130."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Per-blob hue variation — small values keep the lamp monochromatic."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Overall saturation of the blobs and base glow."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the bottle silhouette and dim liquid."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Maximum lightness used for the brightest blob cores at peak heat."},
			{Key: "rise_p", Label: "rise", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-rise",
				Description: "Per-tick chance of marking a blob to detach from the base and begin rising."},
			{Key: "merge_p", Label: "merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-merge",
				Description: "Per-tick chance of two adjacent blobs combining into a larger one."},
			{Key: "split_p", Label: "split", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "blob-split",
				Description: "Per-tick chance of a large blob splitting while ascending or descending."},
			{Key: "surface_pop_p", Label: "surface pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "surface-pop",
				Description: "Per-tick chance of a blob touching the top, flattening, and starting to sink."},
			{Key: "quiet_flow_p", Label: "quiet flow", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "quiet-flow",
				Description: "Per-tick chance of a long suppression window where the lamp settles."},
			{Key: "rise_dur", Label: "rise dur", Slot: SlotEventMod, Group: "blob-rise", Type: KnobInt, Min: 20, Max: 200, Step: 5, Default: 60,
				Description: "Duration of a rise highlight in ticks (jittered by +/-30%)."},
			{Key: "merge_dur", Label: "merge dur", Slot: SlotEventMod, Group: "blob-merge", Type: KnobInt, Min: 6, Max: 60, Step: 2, Default: 20,
				Description: "Duration of a merge flash."},
			{Key: "split_dur", Label: "split dur", Slot: SlotEventMod, Group: "blob-split", Type: KnobInt, Min: 6, Max: 60, Step: 2, Default: 20,
				Description: "Duration of a split flash."},
			{Key: "surface_pop_dur", Label: "pop dur", Slot: SlotEventMod, Group: "surface-pop", Type: KnobInt, Min: 6, Max: 60, Step: 2, Default: 18,
				Description: "Duration of a surface pop while the topmost blob flattens."},
			{Key: "quiet_flow_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobInt, Min: 30, Max: 400, Step: 10, Default: 140,
				Description: "Duration of the quiet-flow suppression window."},
			{Key: "quiet_flow_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.35,
				Description: "Speed multiplier applied to blob motion while quiet-flow is active."},
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
	if out["bottle_x"] <= 0 {
		out["bottle_x"] = lavaLampDefaultsLocal["bottle_x"]
	}
	out["bottle_x"] = clamp01(out["bottle_x"])
	if out["bottle_top"] <= 0 {
		out["bottle_top"] = lavaLampDefaultsLocal["bottle_top"]
	}
	out["bottle_top"] = clamp01(out["bottle_top"])
	if out["bottle_bottom"] <= 0 {
		out["bottle_bottom"] = lavaLampDefaultsLocal["bottle_bottom"]
	}
	out["bottle_bottom"] = clamp01(out["bottle_bottom"])
	if out["bottle_bottom"] <= out["bottle_top"]+0.05 {
		out["bottle_bottom"] = math.Min(0.96, out["bottle_top"]+0.4)
	}
	if out["bottle_width"] <= 0 {
		out["bottle_width"] = lavaLampDefaultsLocal["bottle_width"]
	}
	if out["neck_width"] <= 0 {
		out["neck_width"] = lavaLampDefaultsLocal["neck_width"]
	}
	if out["neck_width"] > out["bottle_width"] {
		out["neck_width"] = out["bottle_width"]
	}
	if out["base_height"] <= 0 {
		out["base_height"] = lavaLampDefaultsLocal["base_height"]
	}
	if out["blob_count"] < 1 {
		out["blob_count"] = 1
	}
	if out["rise_speed"] <= 0 {
		out["rise_speed"] = lavaLampDefaultsLocal["rise_speed"]
	}
	if out["viscosity"] <= 0 {
		out["viscosity"] = lavaLampDefaultsLocal["viscosity"]
	}
	out["viscosity"] = clamp01(out["viscosity"])
	if out["min_radius"] <= 0 {
		out["min_radius"] = lavaLampDefaultsLocal["min_radius"]
	}
	if out["max_radius"] <= 0 {
		out["max_radius"] = lavaLampDefaultsLocal["max_radius"]
	}
	if out["max_radius"] < out["min_radius"] {
		out["min_radius"], out["max_radius"] = out["max_radius"], out["min_radius"]
	}
	if out["glow"] <= 0 {
		out["glow"] = lavaLampDefaultsLocal["glow"]
	}
	if out["hue"] < 0 {
		out["hue"] = lavaLampDefaultsLocal["hue"]
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
	if out["rise_dur"] <= 0 {
		out["rise_dur"] = lavaLampDefaultsLocal["rise_dur"]
	}
	if out["merge_dur"] <= 0 {
		out["merge_dur"] = lavaLampDefaultsLocal["merge_dur"]
	}
	if out["split_dur"] <= 0 {
		out["split_dur"] = lavaLampDefaultsLocal["split_dur"]
	}
	if out["surface_pop_dur"] <= 0 {
		out["surface_pop_dur"] = lavaLampDefaultsLocal["surface_pop_dur"]
	}
	if out["quiet_flow_dur"] <= 0 {
		out["quiet_flow_dur"] = lavaLampDefaultsLocal["quiet_flow_dur"]
	}
	if out["quiet_flow_mult"] <= 0 {
		out["quiet_flow_mult"] = lavaLampDefaultsLocal["quiet_flow_mult"]
	}
	out["quiet_flow_mult"] = clamp01(out["quiet_flow_mult"])
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
		l.startQuietFlowLocked("triggered")
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
	maxBlobs := l.intCfg("blob_count")
	if maxBlobs < 1 {
		maxBlobs = 1
	}
	l.values["rise_blob"] = float64(l.rng.Intn(maxBlobs))
	l.values["rise_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-rise", fmt.Sprintf("%s (dur=%d, blob=%d)", verb, l.timers["rise"], int(l.values["rise_blob"])))
}

func (l *LavaLamp) startMergeLocked(verb string) {
	l.timers["merge"] = jitterInt(l.rng, l.intCfg("merge_dur"), 0.3)
	maxBlobs := l.intCfg("blob_count")
	if maxBlobs < 2 {
		maxBlobs = 2
	}
	a := l.rng.Intn(maxBlobs)
	b := (a + 1 + l.rng.Intn(max(1, maxBlobs-1))) % maxBlobs
	l.values["merge_a"] = float64(a)
	l.values["merge_b"] = float64(b)
	l.values["merge_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-merge", fmt.Sprintf("%s (dur=%d, %d+%d)", verb, l.timers["merge"], a, b))
}

func (l *LavaLamp) startSplitLocked(verb string) {
	l.timers["split"] = jitterInt(l.rng, l.intCfg("split_dur"), 0.3)
	maxBlobs := l.intCfg("blob_count")
	if maxBlobs < 1 {
		maxBlobs = 1
	}
	l.values["split_blob"] = float64(l.rng.Intn(maxBlobs))
	l.values["split_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-split", fmt.Sprintf("%s (dur=%d, blob=%d)", verb, l.timers["split"], int(l.values["split_blob"])))
}

func (l *LavaLamp) startSurfacePopLocked(verb string) {
	l.timers["surface_pop"] = jitterInt(l.rng, l.intCfg("surface_pop_dur"), 0.3)
	maxBlobs := l.intCfg("blob_count")
	if maxBlobs < 1 {
		maxBlobs = 1
	}
	l.values["surface_pop_blob"] = float64(l.rng.Intn(maxBlobs))
	l.values["surface_pop_seed"] = l.rng.Float64() * 1024
	l.appendLog("surface-pop", fmt.Sprintf("%s (dur=%d, blob=%d)", verb, l.timers["surface_pop"], int(l.values["surface_pop_blob"])))
}

func (l *LavaLamp) startQuietFlowLocked(verb string) {
	l.timers["quiet_flow"] = jitterInt(l.rng, l.intCfg("quiet_flow_dur"), 0.3)
	l.values["quiet_flow_mult"] = l.cfg["quiet_flow_mult"]
	l.appendLog("quiet-flow", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["quiet_flow"], l.values["quiet_flow_mult"]))
}

func (l *LavaLamp) startIntroLocked() {
	l.timers["rise"] = 0
	l.timers["merge"] = 0
	l.timers["split"] = 0
	l.timers["surface_pop"] = 0
	l.timers["quiet_flow"] = 0
	l.timers["ending"] = 0
	l.timers["intro"] = l.intCfg("intro_dur")
	l.values["intro_total"] = float64(l.timers["intro"])
}

func (l *LavaLamp) startEndingLocked() {
	l.timers["intro"] = 0
	l.timers["rise"] = 0
	l.timers["merge"] = 0
	l.timers["split"] = 0
	l.timers["surface_pop"] = 0
	l.timers["quiet_flow"] = 0
	endingTotal := l.intCfg("ending_dur") + max(0, l.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, l.intCfg("ending_dur"))
	}
	l.timers["ending"] = endingTotal
	l.values["ending_total"] = float64(endingTotal)
}

func (l *LavaLamp) stepLocked() {
	if l.timers["quiet_flow"] <= 0 {
		delete(l.values, "quiet_flow_mult")
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
	if l.timers["surface_pop"] <= 0 && l.cfg["surface_pop_p"] > 0 && l.rng.Float64() < l.cfg["surface_pop_p"] {
		l.startSurfacePopLocked("started")
	}
	if l.timers["quiet_flow"] <= 0 && l.cfg["quiet_flow_p"] > 0 && l.rng.Float64() < l.cfg["quiet_flow_p"] {
		l.startQuietFlowLocked("started")
	}
}
