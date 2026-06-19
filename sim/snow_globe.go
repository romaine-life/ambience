package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	snowGlobeTicksPerSecond = 60
	snowGlobeSceneCabin     = 0
	snowGlobeScenePine      = 1
	snowGlobeSceneVillage   = 2
	snowGlobeSceneBeacon    = 3
)

// SnowGlobeConfig tunes the contained snow-globe effect. The scene knob is an
// integer enum: 0 cabin, 1 lone pine, 2 village, 3 lighthouse.
type SnowGlobeConfig struct {
	SettleRate      float64 `json:"settleRate"`
	SnowVolume      float64 `json:"snowVolume"`
	ShakeCadence    float64 `json:"shakeCadence"`
	SwirlTurbulence float64 `json:"swirlTurbulence"`
	Scene           int     `json:"scene"`
	IntroDur        int     `json:"introDur"`
	EndingDur       int     `json:"endingDur"`
}

func (c SnowGlobeConfig) withDefaults() SnowGlobeConfig {
	if c.SettleRate <= 0 {
		c.SettleRate = 1.0
	}
	c.SettleRate = math.Max(0.35, math.Min(2.5, c.SettleRate))
	if c.SnowVolume <= 0 {
		c.SnowVolume = 0.72
	}
	c.SnowVolume = math.Max(0.2, math.Min(1.6, c.SnowVolume))
	if c.ShakeCadence <= 0 {
		c.ShakeCadence = 45
	}
	c.ShakeCadence = math.Max(5, math.Min(120, c.ShakeCadence))
	if c.SwirlTurbulence <= 0 {
		c.SwirlTurbulence = 1.0
	}
	c.SwirlTurbulence = math.Max(0.25, math.Min(3.0, c.SwirlTurbulence))
	if c.Scene < snowGlobeSceneCabin {
		c.Scene = snowGlobeSceneCabin
	}
	if c.Scene > snowGlobeSceneBeacon {
		c.Scene = snowGlobeSceneBeacon
	}
	if c.IntroDur <= 0 {
		c.IntroDur = 90
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 300
	}
	return c
}

// NormalizeSnowGlobeConfig returns cfg with defaults and clamps applied.
func NormalizeSnowGlobeConfig(cfg SnowGlobeConfig) SnowGlobeConfig {
	return cfg.withDefaults()
}

// SnowGlobeState is the server/client state for the snow-globe effect. The
// lifecycle field is derived at snapshot time and ignored on restore.
type SnowGlobeState struct {
	Tick      int                `json:"tick"`
	Lifecycle Lifecycle          `json:"lifecycle"`
	Timers    map[string]int     `json:"timers,omitempty"`
	Values    map[string]float64 `json:"values,omitempty"`
	Ended     bool               `json:"ended,omitempty"`
}

// SnowGlobeSnapshot is the wire shape returned by Snapshot().
type SnowGlobeSnapshot struct {
	SnowGlobeState
	RNGState uint64 `json:"rngState,omitempty"`
}

// SnowGlobePersistedState is the on-disk shape returned by
// SnapshotPersistedState().
type SnowGlobePersistedState struct {
	SnowGlobeState
	RNGState uint64 `json:"rngState"`
}

// SnowGlobe is a contained, mostly still snow scene whose only motion comes
// from rare shake events.
type SnowGlobe struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    SnowGlobeConfig
	tick   int
	timers map[string]int
	values map[string]float64
	ended  bool
	log    []LogEntry
}

// NewSnowGlobe builds a snow-globe sim with deterministic timers and RNG.
func NewSnowGlobe(w, h int, seed int64, cfg SnowGlobeConfig) *SnowGlobe {
	g := &SnowGlobe{
		rng:    rngutil.New(seed),
		cfg:    cfg.withDefaults(),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
	g.Resize(w, h)
	g.mu.Lock()
	g.scheduleNextShakeLocked()
	g.mu.Unlock()
	return g
}

func SnowGlobeSchema() EffectSchema {
	return EffectSchema{
		Name:           "snow-globe",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "snowVolume", Label: "snow volume", Slot: SlotSpawn, Group: "globe", Type: KnobFloat, Min: 0.25, Max: 1.35, Step: 0.05, Default: 0.72,
				Description: "Amount of settled snow banked at the bottom of the globe."},
			{Key: "settleRate", Label: "settle rate", Slot: SlotLever, Group: "motion", Type: KnobFloat, Min: 0.5, Max: 1.8, Step: 0.05, Default: 1.0,
				Description: "How quickly airborne flakes resettle after a shake."},
			{Key: "shakeCadence", Label: "shake cadence", Slot: SlotEvent, Group: "shake", Type: KnobFloat, Min: 30, Max: 60, Step: 1, Default: 45, Trigger: "shake",
				Description: "Approximate seconds between autonomous shakes."},
			{Key: "swirlTurbulence", Label: "swirl turbulence", Slot: SlotEventMod, Group: "shake", Type: KnobFloat, Min: 0.35, Max: 2.2, Step: 0.05, Default: 1.0,
				Description: "How wild the contained snow spiral becomes after a shake."},
			{Key: "scene", Label: "scene", Slot: SlotLever, Group: "inside", Type: KnobInt, Min: 0, Max: 3, Step: 1, Default: 0,
				Description: "Inner scene: 0 cabin, 1 lone pine, 2 village, 3 lighthouse."},
			{Key: "introDur", Label: "intro dur", Slot: SlotSpawn, Group: "lifecycle", Type: KnobInt, Min: 30, Max: 240, Step: 5, Default: 90, Trigger: "intro",
				Description: "Ticks spent setting the globe down as the last flakes settle."},
			{Key: "endingDur", Label: "ending dur", Slot: SlotEnd, Group: "lifecycle", Type: KnobInt, Min: 120, Max: 600, Step: 10, Default: 300, Trigger: "ending",
				Description: "Ticks spent resolving to a fully settled terminal globe."},
		},
	}
}

func (g *SnowGlobe) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if w == g.W && h == g.H {
		return
	}
	g.W = w
	g.H = h
	g.Grid = make([][]Pixel, h)
	for y := range g.Grid {
		g.Grid[y] = make([]Pixel, w)
	}
}

func (g *SnowGlobe) SetConfig(cfg SnowGlobeConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cfg = cfg.withDefaults()
	if g.lifecycleLocked() == LifecycleRunning && g.timers["next_shake"] <= 0 && g.timers["swirl"] <= 0 && g.timers["settle"] <= 0 {
		g.scheduleNextShakeLocked()
	}
}

func (g *SnowGlobe) EffectiveConfig() SnowGlobeConfig {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cfg
}

func (g *SnowGlobe) Snapshot() SnowGlobeSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	return SnowGlobeSnapshot{
		SnowGlobeState: g.snapshotStateLocked(),
		RNGState:       g.rng.State(),
	}
}

func (g *SnowGlobe) RestoreSnapshot(snap SnowGlobeSnapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.restoreStateLocked(snap.SnowGlobeState)
	if snap.RNGState != 0 {
		g.rng.SetState(snap.RNGState)
	}
}

func (g *SnowGlobe) SnapshotPersistedState() SnowGlobePersistedState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return SnowGlobePersistedState{
		SnowGlobeState: g.snapshotStateLocked(),
		RNGState:       g.rng.State(),
	}
}

func (g *SnowGlobe) RestorePersistedState(state SnowGlobePersistedState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.restoreStateLocked(state.SnowGlobeState)
	if state.RNGState != 0 {
		g.rng.SetState(state.RNGState)
	}
}

func (g *SnowGlobe) snapshotStateLocked() SnowGlobeState {
	return SnowGlobeState{
		Tick:      g.tick,
		Lifecycle: g.lifecycleLocked(),
		Timers:    cloneTimerMap(g.timers),
		Values:    cloneValueMap(g.values),
		Ended:     g.ended,
	}
}

func (g *SnowGlobe) restoreStateLocked(state SnowGlobeState) {
	g.tick = state.Tick
	g.timers = cloneTimerMap(state.Timers)
	if g.timers == nil {
		g.timers = make(map[string]int)
	}
	g.values = cloneValueMap(state.Values)
	if g.values == nil {
		g.values = make(map[string]float64)
	}
	g.ended = state.Ended
}

func (g *SnowGlobe) lifecycleLocked() Lifecycle {
	switch {
	case g.timers["intro"] > 0:
		return LifecycleIntro
	case g.timers["ending"] > 0:
		return LifecycleEnding
	case g.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (g *SnowGlobe) CurrentTick() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tick
}

func (g *SnowGlobe) PerturbRNG(delta int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rng.Mix(delta)
}

func (g *SnowGlobe) DrainLog() []LogEntry {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.log) == 0 {
		return nil
	}
	out := g.log
	g.log = nil
	return out
}

func (g *SnowGlobe) TriggerEvent(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch name {
	case "intro":
		g.startIntroLocked()
	case "ending":
		g.startEndingLocked()
	case "shake", "swirl":
		if g.ended {
			return false
		}
		g.startShakeLocked("triggered")
	case "settle":
		if g.ended {
			return false
		}
		g.startSettleLocked("triggered")
	case "still":
		g.clearMotionLocked()
		g.scheduleNextShakeLocked()
		g.appendLogLocked("still", "snow fully resettled")
	default:
		return false
	}
	return true
}

func (g *SnowGlobe) Step() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.tick++
	prevSwirl := g.timers["swirl"]
	prevSettle := g.timers["settle"]
	prevIntro := g.timers["intro"]
	prevEnding := g.timers["ending"]
	for key, value := range g.timers {
		if value > 0 {
			g.timers[key] = value - 1
		}
	}

	if prevSwirl > 0 && g.timers["swirl"] <= 0 && g.timers["settle"] <= 0 && !g.ended {
		g.startSettleLocked("settle")
	}
	if prevSettle > 0 && g.timers["settle"] <= 0 && g.timers["ending"] <= 0 && !g.ended {
		g.appendLogLocked("still", "snow fully resettled")
		g.scheduleNextShakeLocked()
	}
	if prevIntro > 0 && g.timers["intro"] <= 0 && g.timers["settle"] <= 0 && !g.ended {
		g.scheduleNextShakeLocked()
	}
	if prevEnding > 0 && g.timers["ending"] <= 0 {
		g.clearMotionLocked()
		g.ended = true
		g.appendLogLocked("still", "terminal globe settled")
	}
	g.pruneTimersLocked()

	if g.lifecycleLocked() == LifecycleRunning && g.timers["swirl"] <= 0 && g.timers["settle"] <= 0 {
		if g.timers["next_shake"] <= 0 {
			g.startShakeLocked("started")
		}
	}
}

func (g *SnowGlobe) GridCopy() [][]Pixel {
	g.mu.Lock()
	defer g.mu.Unlock()
	return renderProceduralGrid("snow-globe", g.W, g.H, g.tick, snowGlobeProceduralConfig(g.cfg), g.timers, g.values, g.rng.State())
}

func (g *SnowGlobe) Frame() [][]Pixel {
	return g.GridCopy()
}

func (g *SnowGlobe) startIntroLocked() {
	g.ended = false
	g.clearMotionLocked()
	g.startSettleLocked("intro")
	g.timers["intro"] = max(max(1, g.cfg.IntroDur), g.timers["settle"])
	g.values["intro_total"] = float64(g.timers["intro"])
	g.appendLogLocked("intro", fmt.Sprintf("started (dur=%d)", g.timers["intro"]))
}

func (g *SnowGlobe) startEndingLocked() {
	g.ended = false
	g.timers["intro"] = 0
	g.timers["next_shake"] = 0
	g.timers["swirl"] = 0
	g.startSettleLocked("ending")
	ending := max(g.cfg.EndingDur, g.timers["settle"])
	g.timers["ending"] = ending
	g.values["ending_total"] = float64(ending)
	g.pruneTimersLocked()
	g.appendLogLocked("ending", fmt.Sprintf("started (dur=%d)", ending))
}

func (g *SnowGlobe) startShakeLocked(verb string) {
	g.timers["next_shake"] = 0
	g.timers["settle"] = 0
	g.timers["swirl"] = g.swirlDurationLocked()
	g.values["swirl_total"] = float64(g.timers["swirl"])
	g.values["swirl_start_tick"] = float64(g.tick)
	g.values["swirl_seed"] = float64(g.rng.Uint64() % 1_000_000)
	dir := 1.0
	if g.rng.Float64() < 0.5 {
		dir = -1
	}
	g.values["swirl_dir"] = dir
	g.pruneTimersLocked()
	g.appendLogLocked("shake", fmt.Sprintf("%s (swirl=%d, turbulence=%.2f)", verb, g.timers["swirl"], g.cfg.SwirlTurbulence))
}

func (g *SnowGlobe) startSettleLocked(verb string) {
	settle := g.settleDurationLocked()
	g.timers["settle"] = settle
	g.values["settle_total"] = float64(settle)
	g.values["settle_start_tick"] = float64(g.tick)
	if _, ok := g.values["swirl_seed"]; !ok {
		g.values["swirl_seed"] = float64(g.rng.Uint64() % 1_000_000)
	}
	g.appendLogLocked("settle", fmt.Sprintf("%s (dur=%d)", verb, settle))
}

func (g *SnowGlobe) scheduleNextShakeLocked() {
	if g.ended {
		return
	}
	base := max(1, int(math.Round(g.cfg.ShakeCadence*snowGlobeTicksPerSecond)))
	g.timers["next_shake"] = jitterInt(g.rng, base, 0.33)
}

func (g *SnowGlobe) clearMotionLocked() {
	for _, key := range []string{"intro", "ending", "next_shake", "swirl", "settle"} {
		g.timers[key] = 0
	}
	for _, key := range []string{"intro_total", "ending_total", "swirl_total", "swirl_start_tick", "swirl_seed", "swirl_dir", "settle_total", "settle_start_tick"} {
		delete(g.values, key)
	}
	g.pruneTimersLocked()
}

func (g *SnowGlobe) pruneTimersLocked() {
	for key, value := range g.timers {
		if value <= 0 {
			delete(g.timers, key)
		}
	}
	if g.timers["intro"] <= 0 {
		delete(g.values, "intro_total")
	}
	if g.timers["ending"] <= 0 {
		delete(g.values, "ending_total")
	}
	if g.timers["swirl"] <= 0 {
		delete(g.values, "swirl_total")
		delete(g.values, "swirl_start_tick")
	}
	if g.timers["settle"] <= 0 {
		delete(g.values, "settle_total")
		delete(g.values, "settle_start_tick")
	}
	if g.timers["swirl"] <= 0 && g.timers["settle"] <= 0 {
		delete(g.values, "swirl_seed")
		delete(g.values, "swirl_dir")
	}
}

func (g *SnowGlobe) swirlDurationLocked() int {
	turb := math.Sqrt(math.Max(0.25, g.cfg.SwirlTurbulence))
	return max(90, min(240, int(math.Round(float64(snowGlobeTicksPerSecond)*(1.7+0.8*turb)))))
}

func (g *SnowGlobe) settleDurationLocked() int {
	return max(180, min(600, int(math.Round(float64(snowGlobeTicksPerSecond)*5.0/g.cfg.SettleRate))))
}

func (g *SnowGlobe) appendLogLocked(kind, desc string) {
	g.log = append(g.log, LogEntry{Tick: g.tick, Type: kind, Desc: desc})
	if len(g.log) > 200 {
		g.log = g.log[len(g.log)-200:]
	}
}

func snowGlobeProceduralConfig(cfg SnowGlobeConfig) ProceduralConfig {
	cfg = cfg.withDefaults()
	return ProceduralConfig{
		"settleRate":      cfg.SettleRate,
		"snowVolume":      cfg.SnowVolume,
		"shakeCadence":    cfg.ShakeCadence,
		"swirlTurbulence": cfg.SwirlTurbulence,
		"scene":           float64(cfg.Scene),
		"introDur":        float64(cfg.IntroDur),
		"endingDur":       float64(cfg.EndingDur),
	}
}
