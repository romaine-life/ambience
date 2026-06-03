package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

type MysteriousManConfig = ProceduralConfig
type MysteriousManState = ProceduralState
type MysteriousManSnapshot = ProceduralSnapshot
type MysteriousManPersistedState = ProceduralPersistedState

// MysteriousMan is a dedicated sim type for the mysterious-man effect.
type MysteriousMan struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    MysteriousManConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var mysteriousManDefaultsLocal = MysteriousManConfig{
	"intro_dur":          70,
	"intro_glow":         0.10,
	"ending_dur":         85,
	"ending_linger":      24,
	"ending_glow":        0.06,
	"figure_x":           0.5,
	"figure_height":      30.0,
	"figure_width":       11.0,
	"silhouette":         0.92,
	"hat":                1.0,
	"shoulder":           1.0,
	"ember_x":            0.56,
	"ember_y":            0.62,
	"ember_brightness":   0.86,
	"ember_pulse":        0.34,
	"smoke_density":      0.42,
	"smoke_rise":         0.46,
	"smoke_drift":        0.18,
	"smoke_softness":     0.62,
	"hue":                22,
	"hue_sp":             10,
	"sat":                0.72,
	"lmin":               0.06,
	"lmax":               0.86,
	"inhale_p":           0.0,
	"exhale_p":           0.0,
	"ash_fall_p":         0.0,
	"lighter_flick_p":    0.0,
	"inhale_dur":         32,
	"inhale_mult":        1.85,
	"exhale_dur":         60,
	"exhale_plume":       1.40,
	"ash_fall_dur":       28,
	"ash_fall_mult":      1.30,
	"lighter_flick_dur":  20,
	"lighter_flick_mult": 2.40,
}

func MysteriousManSchema() EffectSchema {
	return EffectSchema{
		Name: "mysterious-man",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "intro",
				Description: "Ticks spent revealing the figure from full darkness as the first ember catches."},
			{Key: "intro_glow", Label: "intro glow", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.10,
				Description: "Starting fraction of the final ember intensity before the silhouette resolves."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 85, Trigger: "ending",
				Description: "Ticks spent fading the ember and silhouette back toward darkness."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 24,
				Description: "Extra quiet ticks after the ember has mostly faded so the last smoke plume can thin out."},
			{Key: "ending_glow", Label: "ending glow", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.06,
				Description: "Residual ember and silhouette presence near the end of the outro."},
			{Key: "figure_x", Label: "figure x", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.2, Max: 0.8, Step: 0.01, Default: 0.5,
				Description: "Horizontal position of the figure as a fraction of the frame width."},
			{Key: "figure_height", Label: "figure height", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 18, Max: 48, Step: 1, Default: 30,
				Description: "Vertical extent of the silhouette from shoulders to feet."},
			{Key: "figure_width", Label: "figure width", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 6, Max: 18, Step: 1, Default: 11,
				Description: "Horizontal extent of the silhouette body and shoulders."},
			{Key: "silhouette", Label: "silhouette", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0.4, Max: 1.0, Step: 0.02, Default: 0.92,
				Description: "How dark the figure reads against the scene. Lower values let some shape detail show through."},
			{Key: "hat", Label: "hat", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 1.0,
				Description: "Adds a brimmed hat to the silhouette outline. Set to 0 to drop the hat."},
			{Key: "shoulder", Label: "shoulders", Slot: SlotLever, Group: "figure", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 1.0,
				Description: "Adds a coat-shoulder bulge to the silhouette outline."},
			{Key: "ember_x", Label: "ember x", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.3, Max: 0.85, Step: 0.01, Default: 0.56,
				Description: "Horizontal position of the cigarette ember as a fraction of the frame width."},
			{Key: "ember_y", Label: "ember y", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.3, Max: 0.85, Step: 0.01, Default: 0.62,
				Description: "Vertical position of the cigarette ember as a fraction of the frame height."},
			{Key: "ember_brightness", Label: "ember bright", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0.2, Max: 1.2, Step: 0.02, Default: 0.86,
				Description: "Steady-state brightness of the cigarette ember."},
			{Key: "ember_pulse", Label: "ember pulse", Slot: SlotLever, Group: "cigarette", Type: KnobFloat, Min: 0, Max: 0.9, Step: 0.02, Default: 0.34,
				Description: "How much the ember breathes between dim and bright in the steady loop."},
			{Key: "smoke_density", Label: "smoke", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.05, Max: 1.0, Step: 0.02, Default: 0.42,
				Description: "Number of smoke puffs in the air at any one time."},
			{Key: "smoke_rise", Label: "rise speed", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.1, Max: 1.2, Step: 0.02, Default: 0.46,
				Description: "How quickly smoke puffs lift away from the ember."},
			{Key: "smoke_drift", Label: "drift", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: -0.6, Max: 0.6, Step: 0.02, Default: 0.18,
				Description: "Sideways carry on the smoke. Positive drifts right, negative drifts left."},
			{Key: "smoke_softness", Label: "softness", Slot: SlotLever, Group: "smoke", Type: KnobFloat, Min: 0.2, Max: 1.2, Step: 0.02, Default: 0.62,
				Description: "How softly smoke puffs fade. Higher values blur the edges into the dark."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 22,
				Description: "Base ember hue. Lower values lean redder; higher values lean orange-warm."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 10,
				Description: "Variation across ember core, halo, and reflected warmth on the figure."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Saturation of the ember and warm reflected accents."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.0, Max: 0.4, Step: 0.01, Default: 0.06,
				Description: "Minimum lightness used for the dark scene and silhouette body."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used for the brightest ember pixel."},
			{Key: "inhale_p", Label: "inhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "inhale",
				Description: "Per-tick chance of an inhale brightening the ember while smoke briefly compresses."},
			{Key: "exhale_p", Label: "exhale", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "exhale",
				Description: "Per-tick chance of an exhale releasing a visible smoke plume from the figure."},
			{Key: "ash_fall_p", Label: "ash fall", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "ash-fall",
				Description: "Per-tick chance of a small ash fleck breaking off the cigarette."},
			{Key: "lighter_flick_p", Label: "lighter flick", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0, Trigger: "lighter-flick",
				Description: "Per-tick chance of a rare brighter ember catch like a lighter flicking on."},
			{Key: "inhale_dur", Label: "inhale dur", Slot: SlotEventMod, Group: "inhale", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 32,
				Description: "Duration of an inhale event in ticks."},
			{Key: "inhale_mult", Label: "inhale x", Slot: SlotEventMod, Group: "inhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.85,
				Description: "Ember brightness multiplier applied while an inhale is active."},
			{Key: "exhale_dur", Label: "exhale dur", Slot: SlotEventMod, Group: "exhale", Type: KnobInt, Min: 10, Max: 160, Step: 2, Default: 60,
				Description: "Duration of an exhale plume in ticks."},
			{Key: "exhale_plume", Label: "plume size", Slot: SlotEventMod, Group: "exhale", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.4,
				Description: "Size and density multiplier of the exhaled smoke plume."},
			{Key: "ash_fall_dur", Label: "ash dur", Slot: SlotEventMod, Group: "ash-fall", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 28,
				Description: "How many ticks an ash fleck remains visible as it falls."},
			{Key: "ash_fall_mult", Label: "ash x", Slot: SlotEventMod, Group: "ash-fall", Type: KnobFloat, Min: 1.05, Max: 2.5, Step: 0.05, Default: 1.3,
				Description: "Brightness multiplier on the ash fleck while it falls."},
			{Key: "lighter_flick_dur", Label: "flick dur", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobInt, Min: 8, Max: 60, Step: 2, Default: 20,
				Description: "Duration of a lighter-flick brighter catch in ticks."},
			{Key: "lighter_flick_mult", Label: "flick x", Slot: SlotEventMod, Group: "lighter-flick", Type: KnobFloat, Min: 1.2, Max: 4, Step: 0.05, Default: 2.4,
				Description: "Ember brightness multiplier applied during a lighter flick."},
		},
	}
}

func defaultMysteriousManConfig() MysteriousManConfig {
	return cloneConfig(mysteriousManDefaultsLocal)
}

func mergeMysteriousManDefaults(cfg MysteriousManConfig) MysteriousManConfig {
	out := defaultMysteriousManConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = mysteriousManDefaultsLocal["intro_dur"]
	}
	out["intro_glow"] = clamp01(out["intro_glow"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = mysteriousManDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_glow"] = clamp01(out["ending_glow"])
	if out["figure_x"] <= 0 {
		out["figure_x"] = mysteriousManDefaultsLocal["figure_x"]
	}
	if out["figure_height"] <= 0 {
		out["figure_height"] = mysteriousManDefaultsLocal["figure_height"]
	}
	if out["figure_width"] <= 0 {
		out["figure_width"] = mysteriousManDefaultsLocal["figure_width"]
	}
	out["silhouette"] = clamp01(out["silhouette"])
	if out["silhouette"] <= 0 {
		out["silhouette"] = mysteriousManDefaultsLocal["silhouette"]
	}
	out["hat"] = clamp01(out["hat"])
	out["shoulder"] = clamp01(out["shoulder"])
	if out["ember_x"] <= 0 {
		out["ember_x"] = mysteriousManDefaultsLocal["ember_x"]
	}
	if out["ember_y"] <= 0 {
		out["ember_y"] = mysteriousManDefaultsLocal["ember_y"]
	}
	if out["ember_brightness"] <= 0 {
		out["ember_brightness"] = mysteriousManDefaultsLocal["ember_brightness"]
	}
	if out["ember_pulse"] < 0 {
		out["ember_pulse"] = 0
	}
	if out["smoke_density"] < 0 {
		out["smoke_density"] = 0
	}
	if out["smoke_rise"] <= 0 {
		out["smoke_rise"] = mysteriousManDefaultsLocal["smoke_rise"]
	}
	if out["smoke_softness"] <= 0 {
		out["smoke_softness"] = mysteriousManDefaultsLocal["smoke_softness"]
	}
	if out["hue"] < 0 {
		out["hue"] = mysteriousManDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = mysteriousManDefaultsLocal["sat"]
	}
	if out["lmin"] < 0 {
		out["lmin"] = 0
	}
	if out["lmax"] <= 0 {
		out["lmax"] = mysteriousManDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["inhale_dur"] <= 0 {
		out["inhale_dur"] = mysteriousManDefaultsLocal["inhale_dur"]
	}
	if out["inhale_mult"] <= 0 {
		out["inhale_mult"] = mysteriousManDefaultsLocal["inhale_mult"]
	}
	if out["exhale_dur"] <= 0 {
		out["exhale_dur"] = mysteriousManDefaultsLocal["exhale_dur"]
	}
	if out["exhale_plume"] <= 0 {
		out["exhale_plume"] = mysteriousManDefaultsLocal["exhale_plume"]
	}
	if out["ash_fall_dur"] <= 0 {
		out["ash_fall_dur"] = mysteriousManDefaultsLocal["ash_fall_dur"]
	}
	if out["ash_fall_mult"] <= 0 {
		out["ash_fall_mult"] = mysteriousManDefaultsLocal["ash_fall_mult"]
	}
	if out["lighter_flick_dur"] <= 0 {
		out["lighter_flick_dur"] = mysteriousManDefaultsLocal["lighter_flick_dur"]
	}
	if out["lighter_flick_mult"] <= 0 {
		out["lighter_flick_mult"] = mysteriousManDefaultsLocal["lighter_flick_mult"]
	}
	return out
}

func NewMysteriousMan(w, h int, seed int64, cfg MysteriousManConfig) *MysteriousMan {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &MysteriousMan{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeMysteriousManDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (m *MysteriousMan) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if w == m.W && h == m.H {
		return
	}
	m.W = w
	m.H = h
	m.Grid = make([][]Pixel, h)
	for i := range m.Grid {
		m.Grid[i] = make([]Pixel, w)
	}
}

func (m *MysteriousMan) SetConfig(cfg MysteriousManConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = mergeMysteriousManDefaults(cfg)
}

func (m *MysteriousMan) EffectiveConfig() MysteriousManConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneConfig(m.cfg)
}

func (m *MysteriousMan) Snapshot() MysteriousManSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MysteriousManSnapshot{
		ProceduralState: m.snapshotStateLocked(),
		RNGState:        m.rng.State(),
	}
}

func (m *MysteriousMan) RestoreSnapshot(snap MysteriousManSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		m.rng.SetState(snap.RNGState)
	}
}

func (m *MysteriousMan) SnapshotPersistedState() MysteriousManPersistedState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MysteriousManPersistedState{
		ProceduralState: m.snapshotStateLocked(),
		RNGState:        m.rng.State(),
	}
}

func (m *MysteriousMan) RestorePersistedState(ps MysteriousManPersistedState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		m.rng.SetState(ps.RNGState)
	}
}

func (m *MysteriousMan) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   m.tick,
		Timers: cloneTimerMap(m.timers),
		Values: cloneValueMap(m.values),
	}
}

func (m *MysteriousMan) restoreStateLocked(state ProceduralState) {
	m.tick = state.Tick
	m.timers = cloneTimerMap(state.Timers)
	if m.timers == nil {
		m.timers = make(map[string]int)
	}
	m.values = cloneValueMap(state.Values)
	if m.values == nil {
		m.values = make(map[string]float64)
	}
}

func (m *MysteriousMan) CurrentTick() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tick
}

func (m *MysteriousMan) PerturbRNG(delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rng.Mix(delta)
}

func (m *MysteriousMan) DrainLog() []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.log) == 0 {
		return nil
	}
	out := m.log
	m.log = nil
	return out
}

func (m *MysteriousMan) appendLog(kind, desc string) {
	m.log = append(m.log, LogEntry{Tick: m.tick, Type: kind, Desc: desc})
	if len(m.log) > 200 {
		m.log = m.log[len(m.log)-200:]
	}
}

func (m *MysteriousMan) intCfg(key string) int {
	return int(math.Round(m.cfg[key]))
}

func (m *MysteriousMan) TriggerEvent(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch name {
	case "inhale":
		m.startInhaleLocked("triggered")
	case "exhale":
		m.startExhaleLocked("triggered")
	case "ash-fall":
		m.startAshFallLocked("triggered")
	case "lighter-flick":
		m.startLighterFlickLocked("triggered")
	case "intro":
		m.startIntroLocked()
		m.appendLog("intro", fmt.Sprintf("started (dur=%d, glow=%.2f)", m.timers["intro"], m.cfg["intro_glow"]))
	case "ending":
		m.startEndingLocked()
		m.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", m.intCfg("ending_dur"), m.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (m *MysteriousMan) Step() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tick++
	for key, value := range m.timers {
		if value > 0 {
			m.timers[key] = value - 1
		}
	}
	m.stepLocked()
}

func (m *MysteriousMan) startInhaleLocked(verb string) {
	m.timers["inhale"] = jitterInt(m.rng, m.intCfg("inhale_dur"), 0.25)
	m.values["inhale_gain"] = m.cfg["inhale_mult"] * (0.8 + m.rng.Float64()*0.4)
	m.appendLog("inhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, m.timers["inhale"], m.values["inhale_gain"]))
}

func (m *MysteriousMan) startExhaleLocked(verb string) {
	m.timers["exhale"] = jitterInt(m.rng, m.intCfg("exhale_dur"), 0.25)
	m.values["exhale_gain"] = m.cfg["exhale_plume"] * (0.85 + m.rng.Float64()*0.35)
	m.values["exhale_seed"] = m.rng.Float64() * 1024
	m.appendLog("exhale", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, m.timers["exhale"], m.values["exhale_gain"]))
}

func (m *MysteriousMan) startAshFallLocked(verb string) {
	m.timers["ash-fall"] = jitterInt(m.rng, m.intCfg("ash_fall_dur"), 0.3)
	m.values["ash_gain"] = m.cfg["ash_fall_mult"] * (0.85 + m.rng.Float64()*0.3)
	m.values["ash_seed"] = m.rng.Float64() * 1024
	m.appendLog("ash-fall", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, m.timers["ash-fall"], m.values["ash_gain"]))
}

func (m *MysteriousMan) startLighterFlickLocked(verb string) {
	m.timers["lighter-flick"] = jitterInt(m.rng, m.intCfg("lighter_flick_dur"), 0.25)
	m.values["flick_gain"] = m.cfg["lighter_flick_mult"] * (0.85 + m.rng.Float64()*0.3)
	m.appendLog("lighter-flick", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, m.timers["lighter-flick"], m.values["flick_gain"]))
}

func (m *MysteriousMan) startIntroLocked() {
	m.timers["inhale"] = 0
	m.timers["exhale"] = 0
	m.timers["ash-fall"] = 0
	m.timers["lighter-flick"] = 0
	m.timers["ending"] = 0
	m.values["inhale_gain"] = 1
	m.values["exhale_gain"] = 1
	m.values["ash_gain"] = 1
	m.values["flick_gain"] = 1
	m.timers["intro"] = m.intCfg("intro_dur")
	m.values["intro_total"] = float64(m.timers["intro"])
}

func (m *MysteriousMan) startEndingLocked() {
	m.timers["intro"] = 0
	m.timers["inhale"] = 0
	m.timers["exhale"] = 0
	m.timers["ash-fall"] = 0
	m.timers["lighter-flick"] = 0
	m.values["inhale_gain"] = 1
	m.values["exhale_gain"] = 1
	m.values["ash_gain"] = 1
	m.values["flick_gain"] = 1
	endingTotal := m.intCfg("ending_dur") + max(0, m.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, m.intCfg("ending_dur"))
	}
	m.timers["ending"] = endingTotal
	m.values["ending_total"] = float64(endingTotal)
}

func (m *MysteriousMan) stepLocked() {
	if m.timers["inhale"] <= 0 {
		m.values["inhale_gain"] = 1
	}
	if m.timers["exhale"] <= 0 {
		m.values["exhale_gain"] = 1
	}
	if m.timers["ash-fall"] <= 0 {
		m.values["ash_gain"] = 1
	}
	if m.timers["lighter-flick"] <= 0 {
		m.values["flick_gain"] = 1
	}
	if m.timers["intro"] <= 0 {
		delete(m.values, "intro_total")
	}
	if m.timers["ending"] <= 0 {
		delete(m.values, "ending_total")
	}
	if m.timers["intro"] > 0 || m.timers["ending"] > 0 {
		return
	}
	if m.timers["inhale"] <= 0 && m.cfg["inhale_p"] > 0 && m.rng.Float64() < m.cfg["inhale_p"] {
		m.startInhaleLocked("started")
	}
	if m.timers["exhale"] <= 0 && m.cfg["exhale_p"] > 0 && m.rng.Float64() < m.cfg["exhale_p"] {
		m.startExhaleLocked("started")
	}
	if m.timers["ash-fall"] <= 0 && m.cfg["ash_fall_p"] > 0 && m.rng.Float64() < m.cfg["ash_fall_p"] {
		m.startAshFallLocked("started")
	}
	if m.timers["lighter-flick"] <= 0 && m.cfg["lighter_flick_p"] > 0 && m.rng.Float64() < m.cfg["lighter_flick_p"] {
		m.startLighterFlickLocked("started")
	}
}
