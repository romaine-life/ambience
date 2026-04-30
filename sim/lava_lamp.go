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

// LavaLamp is a dedicated sim type for the lava-lamp effect. Visible blob
// motion is rendered client-side; the server only owns coarse rhythm
// (rise/merge/split/pop/quiet) so multiple clients agree on phase.
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
	"intro_dur":      60,
	"intro_glow":     0.12,
	"intro_warmup":   24,
	"ending_dur":     70,
	"ending_linger":  28,
	"ending_glow":    0.08,
	"bottle_x":       0.5,
	"bottle_top":     0.10,
	"bottle_bottom":  0.92,
	"bottle_width":   0.32,
	"bottle_neck":    0.18,
	"liquid_hue":     14,
	"liquid_sat":     0.42,
	"liquid_light":   0.22,
	"hue":            6,
	"hue_sp":         16,
	"sat":            0.86,
	"lmin":           0.26,
	"lmax":           0.92,
	"glow":           0.55,
	"blob_count":     4,
	"blob_size":      4.5,
	"blob_size_sp":   1.6,
	"rise_speed":     0.18,
	"viscosity":      0.78,
	"wobble":         0.55,
	"blob_rise_p":    0.0,
	"blob_merge_p":   0.0,
	"blob_split_p":   0.0,
	"surface_pop_p":  0.0,
	"quiet_flow_p":   0.0,
	"blob_rise_dur":  60,
	"blob_merge_dur": 36,
	"blob_split_dur": 32,
	"surface_pop_dur": 28,
	"quiet_flow_dur":  140,
	"quiet_flow_mult": 0.45,
}

func LavaLampSchema() EffectSchema {
	return EffectSchema{
		Name: "lava-lamp",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks spent kindling the heat source before the first blob detaches."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.12,
				Description: "Starting fraction of the warm base glow before the lamp settles into rhythm."},
			{Key: "intro_warmup", Label: "warmup", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 160, Step: 2, Default: 24,
				Description: "Extra quiet ticks after the heat catches before the first detachment fires."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent fading the heat source so blobs settle back to the base."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 200, Step: 5, Default: 28,
				Description: "Extra quiet ticks for residual blobs to drift back to the floor."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.08,
				Description: "Residual base glow that remains while the lamp shuts down."},
			{Key: "bottle_x", Label: "bottle x", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.01, Default: 0.5,
				Description: "Horizontal position of the bottle silhouette as a fraction of frame width."},
			{Key: "bottle_top", Label: "bottle top", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Top of the bottle silhouette as a fraction of frame height."},
			{Key: "bottle_bottom", Label: "bottle bottom", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.6, Max: 0.98, Step: 0.01, Default: 0.92,
				Description: "Bottom of the bottle silhouette as a fraction of frame height."},
			{Key: "bottle_width", Label: "bottle width", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.12, Max: 0.6, Step: 0.01, Default: 0.32,
				Description: "Maximum bottle width relative to the frame."},
			{Key: "bottle_neck", Label: "bottle neck", Slot: SlotLever, Group: "bottle", Type: KnobFloat, Min: 0.05, Max: 0.45, Step: 0.01, Default: 0.18,
				Description: "Width of the narrow neck and base relative to the bottle's belly."},
			{Key: "liquid_hue", Label: "liquid hue", Slot: SlotLever, Group: "liquid", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 14,
				Description: "Hue of the warm liquid behind the blobs."},
			{Key: "liquid_sat", Label: "liquid sat", Slot: SlotLever, Group: "liquid", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.42,
				Description: "Saturation of the warm liquid background."},
			{Key: "liquid_light", Label: "liquid light", Slot: SlotLever, Group: "liquid", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.22,
				Description: "Lightness of the warm liquid background."},
			{Key: "hue", Label: "blob hue", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 6,
				Description: "Base hue of the blobs. The scene knob — preset palettes shift this dramatically."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 16,
				Description: "Hue variation across blobs and within the bright core to dim rim."},
			{Key: "sat", Label: "blob sat", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Overall saturation of the blob bodies."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.26,
				Description: "Minimum lightness used at blob rims and dim halves."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.92,
				Description: "Maximum lightness used at brightest blob cores under the heat source."},
			{Key: "glow", Label: "base glow", Slot: SlotLever, Group: "heat", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.55,
				Description: "Strength of the warm glow at the base of the bottle."},
			{Key: "blob_count", Label: "blob count", Slot: SlotLever, Group: "blobs", Type: KnobInt, Min: 1, Max: 9, Step: 1, Default: 4,
				Description: "Target number of active blobs floating in the bottle."},
			{Key: "blob_size", Label: "blob size", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 1.5, Max: 9, Step: 0.1, Default: 4.5,
				Description: "Average blob radius in grid cells."},
			{Key: "blob_size_sp", Label: "size spread", Slot: SlotLever, Group: "blobs", Type: KnobFloat, Min: 0, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Variation between large and small blobs."},
			{Key: "rise_speed", Label: "rise speed", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.04, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "How quickly heated blobs rise through the liquid."},
			{Key: "viscosity", Label: "viscosity", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Resistance of the liquid — higher reads as gooier and slower changes in direction."},
			{Key: "wobble", Label: "wobble", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0, Max: 1.4, Step: 0.02, Default: 0.55,
				Description: "Sideways drift while a blob is rising or sinking."},
			{Key: "blob_rise_p", Label: "blob rise", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.05, Step: 0.0005, Default: 0, Trigger: "blob-rise",
				Description: "Per-tick chance of a blob detaching from the base and starting its ascent."},
			{Key: "blob_merge_p", Label: "blob merge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.04, Step: 0.0005, Default: 0, Trigger: "blob-merge",
				Description: "Per-tick chance of two adjacent blobs combining into a larger one."},
			{Key: "blob_split_p", Label: "blob split", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.03, Step: 0.0005, Default: 0, Trigger: "blob-split",
				Description: "Per-tick chance of a large blob splitting while in motion."},
			{Key: "surface_pop_p", Label: "surface pop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.04, Step: 0.0005, Default: 0, Trigger: "surface-pop",
				Description: "Per-tick chance of a blob touching the top, flattening, and starting to sink."},
			{Key: "quiet_flow_p", Label: "quiet flow", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-flow",
				Description: "Per-tick chance of the lamp settling into a long suppressed-detachment window."},
			{Key: "blob_rise_dur", Label: "rise dur", Slot: SlotEventMod, Group: "blob-rise", Type: KnobInt, Min: 12, Max: 200, Step: 4, Default: 60,
				Description: "Duration of an active rising-blob impulse."},
			{Key: "blob_merge_dur", Label: "merge dur", Slot: SlotEventMod, Group: "blob-merge", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 36,
				Description: "Visual duration of a merge — the absorbing blob brightens for this long."},
			{Key: "blob_split_dur", Label: "split dur", Slot: SlotEventMod, Group: "blob-split", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 32,
				Description: "Visual duration of a split — the donor blob shrinks while the new one accelerates."},
			{Key: "surface_pop_dur", Label: "pop dur", Slot: SlotEventMod, Group: "surface-pop", Type: KnobInt, Min: 8, Max: 120, Step: 2, Default: 28,
				Description: "Duration of a flattening surface-pop before the blob starts sinking again."},
			{Key: "quiet_flow_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobInt, Min: 30, Max: 360, Step: 10, Default: 140,
				Description: "Duration of a quiet-flow window where the lamp settles."},
			{Key: "quiet_flow_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet-flow", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Heat multiplier applied while the lamp is in a quiet-flow window."},
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
	if out["intro_warmup"] < 0 {
		out["intro_warmup"] = 0
	}
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = lavaLampDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	out["bottle_x"] = clamp01(out["bottle_x"])
	out["bottle_top"] = clamp01(out["bottle_top"])
	out["bottle_bottom"] = clamp01(out["bottle_bottom"])
	if out["bottle_bottom"] <= out["bottle_top"] {
		out["bottle_bottom"] = math.Min(0.98, out["bottle_top"]+0.4)
	}
	if out["bottle_width"] <= 0 {
		out["bottle_width"] = lavaLampDefaultsLocal["bottle_width"]
	}
	if out["bottle_neck"] <= 0 {
		out["bottle_neck"] = lavaLampDefaultsLocal["bottle_neck"]
	}
	if out["liquid_sat"] < 0 {
		out["liquid_sat"] = 0
	}
	if out["liquid_light"] <= 0 {
		out["liquid_light"] = lavaLampDefaultsLocal["liquid_light"]
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
	if out["glow"] <= 0 {
		out["glow"] = lavaLampDefaultsLocal["glow"]
	}
	if out["blob_count"] <= 0 {
		out["blob_count"] = lavaLampDefaultsLocal["blob_count"]
	}
	if out["blob_size"] <= 0 {
		out["blob_size"] = lavaLampDefaultsLocal["blob_size"]
	}
	if out["blob_size_sp"] < 0 {
		out["blob_size_sp"] = 0
	}
	if out["rise_speed"] <= 0 {
		out["rise_speed"] = lavaLampDefaultsLocal["rise_speed"]
	}
	if out["viscosity"] <= 0 {
		out["viscosity"] = lavaLampDefaultsLocal["viscosity"]
	}
	if out["wobble"] < 0 {
		out["wobble"] = 0
	}
	if out["blob_rise_dur"] <= 0 {
		out["blob_rise_dur"] = lavaLampDefaultsLocal["blob_rise_dur"]
	}
	if out["blob_merge_dur"] <= 0 {
		out["blob_merge_dur"] = lavaLampDefaultsLocal["blob_merge_dur"]
	}
	if out["blob_split_dur"] <= 0 {
		out["blob_split_dur"] = lavaLampDefaultsLocal["blob_split_dur"]
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
		l.startBlobRiseLocked("triggered")
	case "blob-merge":
		l.startBlobMergeLocked("triggered")
	case "blob-split":
		l.startBlobSplitLocked("triggered")
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

func (l *LavaLamp) startBlobRiseLocked(verb string) {
	l.timers["blob_rise"] = jitterInt(l.rng, l.intCfg("blob_rise_dur"), 0.3)
	l.values["blob_rise_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-rise", fmt.Sprintf("%s (dur=%d)", verb, l.timers["blob_rise"]))
}

func (l *LavaLamp) startBlobMergeLocked(verb string) {
	l.timers["blob_merge"] = jitterInt(l.rng, l.intCfg("blob_merge_dur"), 0.3)
	l.values["blob_merge_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-merge", fmt.Sprintf("%s (dur=%d)", verb, l.timers["blob_merge"]))
}

func (l *LavaLamp) startBlobSplitLocked(verb string) {
	l.timers["blob_split"] = jitterInt(l.rng, l.intCfg("blob_split_dur"), 0.3)
	l.values["blob_split_seed"] = l.rng.Float64() * 1024
	l.appendLog("blob-split", fmt.Sprintf("%s (dur=%d)", verb, l.timers["blob_split"]))
}

func (l *LavaLamp) startSurfacePopLocked(verb string) {
	l.timers["surface_pop"] = jitterInt(l.rng, l.intCfg("surface_pop_dur"), 0.3)
	l.values["surface_pop_seed"] = l.rng.Float64() * 1024
	l.appendLog("surface-pop", fmt.Sprintf("%s (dur=%d)", verb, l.timers["surface_pop"]))
}

func (l *LavaLamp) startQuietFlowLocked(verb string) {
	l.timers["quiet_flow"] = jitterInt(l.rng, l.intCfg("quiet_flow_dur"), 0.3)
	l.appendLog("quiet-flow", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, l.timers["quiet_flow"], l.cfg["quiet_flow_mult"]))
}

func (l *LavaLamp) startIntroLocked() {
	l.timers["blob_rise"] = 0
	l.timers["blob_merge"] = 0
	l.timers["blob_split"] = 0
	l.timers["surface_pop"] = 0
	l.timers["quiet_flow"] = 0
	l.timers["ending"] = 0
	l.timers["intro"] = l.intCfg("intro_dur") + max(0, l.intCfg("intro_warmup"))
	l.values["intro_total"] = float64(l.timers["intro"])
	l.values["intro_warmup"] = float64(max(0, l.intCfg("intro_warmup")))
}

func (l *LavaLamp) startEndingLocked() {
	l.timers["intro"] = 0
	l.timers["blob_rise"] = 0
	l.timers["blob_merge"] = 0
	l.timers["blob_split"] = 0
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
	if l.timers["intro"] <= 0 {
		delete(l.values, "intro_total")
		delete(l.values, "intro_warmup")
	}
	if l.timers["ending"] <= 0 {
		delete(l.values, "ending_total")
	}
	if l.timers["intro"] > 0 || l.timers["ending"] > 0 {
		return
	}
	if l.timers["quiet_flow"] <= 0 && l.cfg["quiet_flow_p"] > 0 && l.rng.Float64() < l.cfg["quiet_flow_p"] {
		l.startQuietFlowLocked("started")
	}
	// During quiet flow, suppress detachments but still allow gentle merges to settle blobs.
	quiet := l.timers["quiet_flow"] > 0
	rise := l.cfg["blob_rise_p"]
	if quiet {
		rise *= l.cfg["quiet_flow_mult"]
	}
	if l.timers["blob_rise"] <= 0 && rise > 0 && l.rng.Float64() < rise {
		l.startBlobRiseLocked("started")
	}
	if l.timers["blob_merge"] <= 0 && l.cfg["blob_merge_p"] > 0 && l.rng.Float64() < l.cfg["blob_merge_p"] {
		l.startBlobMergeLocked("started")
	}
	split := l.cfg["blob_split_p"]
	if quiet {
		split *= l.cfg["quiet_flow_mult"]
	}
	if l.timers["blob_split"] <= 0 && split > 0 && l.rng.Float64() < split {
		l.startBlobSplitLocked("started")
	}
	pop := l.cfg["surface_pop_p"]
	if quiet {
		pop *= l.cfg["quiet_flow_mult"]
	}
	if l.timers["surface_pop"] <= 0 && pop > 0 && l.rng.Float64() < pop {
		l.startSurfacePopLocked("started")
	}
}
