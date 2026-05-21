package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/nelsong6/ambience/rngutil"
)

type DistantStormConfig = ProceduralConfig
type DistantStormState = ProceduralState
type DistantStormSnapshot = ProceduralSnapshot
type DistantStormPersistedState = ProceduralPersistedState

// DistantStorm is a seascape effect: a dark cloud bank squats over the
// horizon, the sea below rolls in slow swells, and silent lightning
// occasionally flickers inside the cloud bank. The storm stays out at
// the horizon, so the foreground remains quiet.
type DistantStorm struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng    *rngutil.RNG
	cfg    DistantStormConfig
	tick   int
	timers map[string]int
	values map[string]float64
	log    []LogEntry
}

var distantStormDefaultsLocal = DistantStormConfig{
	"intro_dur":            55,
	"intro_storm":          0.20,
	"ending_dur":           70,
	"ending_linger":        20,
	"ending_storm":         0.10,
	"horizon":              0.72,
	"cloud_height":         0.34,
	"cloud_density":        0.62,
	"cloud_drift":          0.06,
	"sea_swell":            2.6,
	"sea_speed":            0.12,
	"glow":                 0.16,
	"hue":                  220,
	"hue_sp":               14,
	"sat":                  0.32,
	"lmin":                 0.10,
	"lmax":                 0.78,
	"lightning_p":          0.0,
	"afterglow_p":          0.0,
	"cloud_drift_p":        0.0,
	"distant_thunder_p":    0.0,
	"quiet_horizon_p":      0.0,
	"lightning_dur":        5,
	"lightning_mult":       2.5,
	"afterglow_dur":        34,
	"afterglow_mult":       1.35,
	"cloud_drift_dur":      90,
	"cloud_drift_mult":     2.15,
	"distant_thunder_dur":  48,
	"distant_thunder_mult": 1.45,
	"quiet_horizon_dur":    150,
	"quiet_horizon_mult":   0.58,
}

// DistantStormSchema describes the distant-storm effect's tunable knobs for
// the dev UI.
func DistantStormSchema() EffectSchema {
	return EffectSchema{
		Name: "distant-storm",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 55, Trigger: "intro",
				Description: "Ticks spent bringing the cloud bank in from a near-clear horizon."},
			{Key: "intro_storm", Label: "intro storm", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.20,
				Description: "Starting fraction of the full storm presence before the cloud bank settles in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 220, Step: 5, Default: 70, Trigger: "ending",
				Description: "Ticks spent thinning the cloud bank and letting the last glow fade."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 20,
				Description: "Extra quiet ticks after the storm has mostly cleared."},
			{Key: "ending_storm", Label: "ending storm", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.10,
				Description: "Residual cloud presence near the end of the outro."},
			{Key: "horizon", Label: "horizon", Slot: SlotLever, Group: "scene", Type: KnobFloat, Min: 0.55, Max: 0.86, Step: 0.01, Default: 0.72,
				Description: "Height of the sea horizon in the frame. Lower values give more sky, higher values more sea."},
			{Key: "cloud_height", Label: "cloud height", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.12, Max: 0.6, Step: 0.01, Default: 0.34,
				Description: "Vertical thickness of the cloud bank above the horizon as a fraction of the frame."},
			{Key: "cloud_density", Label: "cloud density", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.15, Max: 1.0, Step: 0.02, Default: 0.62,
				Description: "Opacity of the cloud bank; lower values let more sky show through."},
			{Key: "cloud_drift", Label: "cloud drift", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.0, Max: 0.4, Step: 0.01, Default: 0.06,
				Description: "Horizontal drift speed of the cloud bank along the horizon."},
			{Key: "sea_swell", Label: "sea swell", Slot: SlotLever, Group: "sea", Type: KnobFloat, Min: 0.5, Max: 8.0, Step: 0.1, Default: 2.6,
				Description: "Vertical amplitude of the long sea swells rolling under the storm."},
			{Key: "sea_speed", Label: "sea speed", Slot: SlotLever, Group: "sea", Type: KnobFloat, Min: 0.02, Max: 0.4, Step: 0.01, Default: 0.12,
				Description: "How quickly the sea swells advance across the frame."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "sky", Type: KnobFloat, Min: 0.02, Max: 0.5, Step: 0.01, Default: 0.16,
				Description: "Residual horizon glow under the cloud bank."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 190, Max: 250, Step: 1, Default: 220,
				Description: "Base storm hue. Lower values lean teal-grey; higher values lean deep blue-violet."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 30, Step: 1, Default: 14,
				Description: "Variation between cloud bank, horizon glow, and sea tones."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.7, Step: 0.01, Default: 0.32,
				Description: "Overall scene saturation for sky and sea."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.04, Max: 0.4, Step: 0.01, Default: 0.10,
				Description: "Minimum lightness used for the darkest clouds and deepest sea."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.4, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for horizon glow, foam, and lightning."},
			{Key: "lightning_p", Label: "lightning", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.04, Step: 0.0005, Default: 0, Trigger: "lightning-flash",
				Description: "Per-tick chance of a brief lightning flash illuminating the cloud bank."},
			{Key: "afterglow_p", Label: "afterglow", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.03, Step: 0.0005, Default: 0, Trigger: "afterglow",
				Description: "Per-tick chance of a soft residual glow inside the cloud bank."},
			{Key: "cloud_drift_p", Label: "drift shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "cloud-drift",
				Description: "Per-tick chance of the cloud edge stretching and drifting faster."},
			{Key: "distant_thunder_p", Label: "thunder pulse", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "distant-thunder",
				Description: "Per-tick chance of a slow, dim pulse through the cloud bank."},
			{Key: "quiet_horizon_p", Label: "quiet horizon", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "quiet-horizon",
				Description: "Per-tick chance of a quiet interval with no new flashes."},
			{Key: "lightning_dur", Label: "flash dur", Slot: SlotEventMod, Group: "lightning", Type: KnobInt, Min: 2, Max: 30, Step: 1, Default: 6,
				Description: "Duration in ticks of a single lightning flash."},
			{Key: "lightning_mult", Label: "flash x", Slot: SlotEventMod, Group: "lightning", Type: KnobFloat, Min: 1.2, Max: 4, Step: 0.1, Default: 2.4,
				Description: "Brightness multiplier applied to the cloud bank during a lightning flash."},
			{Key: "afterglow_dur", Label: "afterglow dur", Slot: SlotEventMod, Group: "afterglow", Type: KnobInt, Min: 5, Max: 120, Step: 5, Default: 34,
				Description: "Duration of the soft glow tail after a flash."},
			{Key: "afterglow_mult", Label: "afterglow x", Slot: SlotEventMod, Group: "afterglow", Type: KnobFloat, Min: 1.05, Max: 2.2, Step: 0.05, Default: 1.35,
				Description: "Cloud-bank brightness multiplier for the glow tail."},
			{Key: "cloud_drift_dur", Label: "drift dur", Slot: SlotEventMod, Group: "cloud drift", Type: KnobInt, Min: 15, Max: 220, Step: 5, Default: 90,
				Description: "Duration of a faster cloud-drift interval."},
			{Key: "cloud_drift_mult", Label: "drift x", Slot: SlotEventMod, Group: "cloud drift", Type: KnobFloat, Min: 1.05, Max: 4, Step: 0.05, Default: 2.15,
				Description: "Drift-speed multiplier while the cloud-drift event is active."},
			{Key: "distant_thunder_dur", Label: "thunder dur", Slot: SlotEventMod, Group: "distant thunder", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 48,
				Description: "Duration of the slow visual thunder pulse."},
			{Key: "distant_thunder_mult", Label: "thunder x", Slot: SlotEventMod, Group: "distant thunder", Type: KnobFloat, Min: 1.05, Max: 2.5, Step: 0.05, Default: 1.45,
				Description: "Cloud-bank pulse multiplier for distant thunder."},
			{Key: "quiet_horizon_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet horizon", Type: KnobInt, Min: 20, Max: 300, Step: 5, Default: 150,
				Description: "Duration of the no-flash quiet interval."},
			{Key: "quiet_horizon_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet horizon", Type: KnobFloat, Min: 0.1, Max: 1, Step: 0.05, Default: 0.58,
				Description: "Storm intensity multiplier while quiet horizon is active."},
		},
	}
}

func defaultDistantStormConfig() DistantStormConfig { return cloneConfig(distantStormDefaultsLocal) }

func mergeDistantStormDefaults(cfg DistantStormConfig) DistantStormConfig {
	out := defaultDistantStormConfig()
	for k, v := range cfg {
		out[k] = v
	}
	if out["intro_dur"] <= 0 {
		out["intro_dur"] = distantStormDefaultsLocal["intro_dur"]
	}
	out["intro_storm"] = clamp01(out["intro_storm"])
	if out["ending_dur"] <= 0 {
		out["ending_dur"] = distantStormDefaultsLocal["ending_dur"]
	}
	if out["ending_linger"] < 0 {
		out["ending_linger"] = 0
	}
	out["ending_storm"] = clamp01(out["ending_storm"])
	if out["horizon"] <= 0 {
		out["horizon"] = distantStormDefaultsLocal["horizon"]
	}
	if out["cloud_height"] <= 0 {
		out["cloud_height"] = distantStormDefaultsLocal["cloud_height"]
	}
	if out["cloud_density"] <= 0 {
		out["cloud_density"] = distantStormDefaultsLocal["cloud_density"]
	}
	if out["cloud_drift"] < 0 {
		out["cloud_drift"] = 0
	}
	if out["sea_swell"] <= 0 {
		out["sea_swell"] = distantStormDefaultsLocal["sea_swell"]
	}
	if out["sea_speed"] <= 0 {
		out["sea_speed"] = distantStormDefaultsLocal["sea_speed"]
	}
	if out["glow"] <= 0 {
		out["glow"] = distantStormDefaultsLocal["glow"]
	}
	if out["hue"] == 0 {
		out["hue"] = distantStormDefaultsLocal["hue"]
	}
	if out["hue_sp"] < 0 {
		out["hue_sp"] = 0
	}
	if out["sat"] <= 0 {
		out["sat"] = distantStormDefaultsLocal["sat"]
	}
	if out["lmin"] <= 0 {
		out["lmin"] = distantStormDefaultsLocal["lmin"]
	}
	if out["lmax"] <= 0 {
		out["lmax"] = distantStormDefaultsLocal["lmax"]
	}
	if out["lmax"] < out["lmin"] {
		out["lmin"], out["lmax"] = out["lmax"], out["lmin"]
	}
	if out["lightning_dur"] <= 0 {
		out["lightning_dur"] = distantStormDefaultsLocal["lightning_dur"]
	}
	if out["lightning_mult"] <= 0 {
		out["lightning_mult"] = distantStormDefaultsLocal["lightning_mult"]
	}
	for _, key := range []string{"afterglow_dur", "cloud_drift_dur", "distant_thunder_dur", "quiet_horizon_dur"} {
		if out[key] <= 0 {
			out[key] = distantStormDefaultsLocal[key]
		}
	}
	for _, key := range []string{"afterglow_mult", "cloud_drift_mult", "distant_thunder_mult", "quiet_horizon_mult"} {
		if out[key] <= 0 {
			out[key] = distantStormDefaultsLocal[key]
		}
	}
	return out
}

func NewDistantStorm(w, h int, seed int64, cfg DistantStormConfig) *DistantStorm {
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return &DistantStorm{
		W:      w,
		H:      h,
		Grid:   grid,
		rng:    rngutil.New(seed),
		cfg:    mergeDistantStormDefaults(cfg),
		timers: make(map[string]int),
		values: make(map[string]float64),
	}
}

func (d *DistantStorm) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if w == d.W && h == d.H {
		return
	}
	d.W = w
	d.H = h
	d.Grid = make([][]Pixel, h)
	for i := range d.Grid {
		d.Grid[i] = make([]Pixel, w)
	}
}

func (d *DistantStorm) SetConfig(cfg DistantStormConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = mergeDistantStormDefaults(cfg)
}

func (d *DistantStorm) EffectiveConfig() DistantStormConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cloneConfig(d.cfg)
}

func (d *DistantStorm) Snapshot() DistantStormSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DistantStormSnapshot{
		ProceduralState: d.snapshotStateLocked(),
		RNGState:        d.rng.State(),
	}
}

func (d *DistantStorm) RestoreSnapshot(snap DistantStormSnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restoreStateLocked(snap.ProceduralState)
	if snap.RNGState != 0 {
		d.rng.SetState(snap.RNGState)
	}
}

func (d *DistantStorm) SnapshotPersistedState() DistantStormPersistedState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DistantStormPersistedState{
		ProceduralState: d.snapshotStateLocked(),
		RNGState:        d.rng.State(),
	}
}

func (d *DistantStorm) RestorePersistedState(ps DistantStormPersistedState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restoreStateLocked(ps.ProceduralState)
	if ps.RNGState != 0 {
		d.rng.SetState(ps.RNGState)
	}
}

func (d *DistantStorm) snapshotStateLocked() ProceduralState {
	return ProceduralState{
		Tick:   d.tick,
		Timers: cloneTimerMap(d.timers),
		Values: cloneValueMap(d.values),
	}
}

func (d *DistantStorm) restoreStateLocked(state ProceduralState) {
	d.tick = state.Tick
	d.timers = cloneTimerMap(state.Timers)
	if d.timers == nil {
		d.timers = make(map[string]int)
	}
	d.values = cloneValueMap(state.Values)
	if d.values == nil {
		d.values = make(map[string]float64)
	}
}

func (d *DistantStorm) CurrentTick() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tick
}

func (d *DistantStorm) PerturbRNG(delta int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rng.Mix(delta)
}

func (d *DistantStorm) DrainLog() []LogEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.log) == 0 {
		return nil
	}
	out := d.log
	d.log = nil
	return out
}

func (d *DistantStorm) appendLog(kind, desc string) {
	d.log = append(d.log, LogEntry{Tick: d.tick, Type: kind, Desc: desc})
	if len(d.log) > 200 {
		d.log = d.log[len(d.log)-200:]
	}
}

func (d *DistantStorm) intCfg(key string) int {
	return int(math.Round(d.cfg[key]))
}

func (d *DistantStorm) TriggerEvent(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch name {
	case "lightning-flash":
		d.startLightningLocked("triggered")
	case "afterglow":
		d.startAfterglowLocked("triggered")
	case "cloud-drift":
		d.startCloudDriftLocked("triggered")
	case "distant-thunder":
		d.startDistantThunderLocked("triggered")
	case "quiet-horizon":
		d.startQuietHorizonLocked("triggered")
	case "intro":
		d.startIntroLocked()
		d.appendLog("intro", fmt.Sprintf("started (dur=%d, storm=%.2f)", d.timers["intro"], d.cfg["intro_storm"]))
	case "ending":
		d.startEndingLocked()
		d.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", d.intCfg("ending_dur"), d.intCfg("ending_linger")))
	default:
		return false
	}
	return true
}

func (d *DistantStorm) Step() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tick++
	for key, value := range d.timers {
		if value > 0 {
			d.timers[key] = value - 1
		}
	}
	d.stepLocked()
}

func (d *DistantStorm) startLightningLocked(verb string) {
	d.timers["lightning"] = jitterInt(d.rng, d.intCfg("lightning_dur"), 0.4)
	d.values["lightning_gain"] = d.cfg["lightning_mult"] * (0.85 + d.rng.Float64()*0.45)
	d.values["lightning_seed"] = float64(d.rng.Int63() & 0xffff)
	d.startAfterglowLocked("tail")
	d.appendLog("lightning-flash", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, d.timers["lightning"], d.values["lightning_gain"]))
}

func (d *DistantStorm) startAfterglowLocked(verb string) {
	d.timers["afterglow"] = max(d.timers["afterglow"], jitterInt(d.rng, d.intCfg("afterglow_dur"), 0.25))
	d.values["afterglow_total"] = float64(d.timers["afterglow"])
	d.values["afterglow_gain"] = d.cfg["afterglow_mult"] * (0.92 + d.rng.Float64()*0.18)
	d.appendLog("afterglow", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, d.timers["afterglow"], d.values["afterglow_gain"]))
}

func (d *DistantStorm) startCloudDriftLocked(verb string) {
	d.timers["cloud-drift"] = jitterInt(d.rng, d.intCfg("cloud_drift_dur"), 0.25)
	d.values["cloud_drift_total"] = float64(d.timers["cloud-drift"])
	d.values["cloud_drift_bias"] = (d.rng.Float64()*2 - 1) * 8
	d.appendLog("cloud-drift", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, d.timers["cloud-drift"], d.cfg["cloud_drift_mult"]))
}

func (d *DistantStorm) startDistantThunderLocked(verb string) {
	d.timers["distant-thunder"] = jitterInt(d.rng, d.intCfg("distant_thunder_dur"), 0.25)
	d.values["distant_thunder_total"] = float64(d.timers["distant-thunder"])
	d.values["distant_thunder_gain"] = d.cfg["distant_thunder_mult"] * (0.9 + d.rng.Float64()*0.2)
	d.appendLog("distant-thunder", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, d.timers["distant-thunder"], d.values["distant_thunder_gain"]))
}

func (d *DistantStorm) startQuietHorizonLocked(verb string) {
	d.timers["quiet-horizon"] = jitterInt(d.rng, d.intCfg("quiet_horizon_dur"), 0.25)
	d.timers["lightning"] = 0
	d.values["lightning_gain"] = 1
	delete(d.values, "lightning_seed")
	d.appendLog("quiet-horizon", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, d.timers["quiet-horizon"], d.cfg["quiet_horizon_mult"]))
}

func (d *DistantStorm) startIntroLocked() {
	d.timers["lightning"] = 0
	d.timers["afterglow"] = 0
	d.timers["cloud-drift"] = 0
	d.timers["distant-thunder"] = 0
	d.timers["quiet-horizon"] = 0
	d.timers["ending"] = 0
	d.values["lightning_gain"] = 1
	d.values["afterglow_gain"] = 1
	d.values["distant_thunder_gain"] = 1
	d.values["cloud_drift_bias"] = 0
	d.timers["intro"] = d.intCfg("intro_dur")
	d.values["intro_total"] = float64(d.timers["intro"])
}

func (d *DistantStorm) startEndingLocked() {
	d.timers["intro"] = 0
	d.timers["lightning"] = 0
	d.timers["cloud-drift"] = 0
	d.timers["distant-thunder"] = 0
	d.timers["quiet-horizon"] = 0
	d.values["lightning_gain"] = 1
	d.values["distant_thunder_gain"] = 1
	d.values["cloud_drift_bias"] = 0
	endingTotal := d.intCfg("ending_dur") + max(0, d.intCfg("ending_linger"))
	if endingTotal < 1 {
		endingTotal = max(1, d.intCfg("ending_dur"))
	}
	d.timers["ending"] = endingTotal
	d.values["ending_total"] = float64(endingTotal)
}

func (d *DistantStorm) stepLocked() {
	if d.timers["lightning"] <= 0 {
		d.values["lightning_gain"] = 1
		delete(d.values, "lightning_seed")
	}
	if d.timers["afterglow"] <= 0 {
		d.values["afterglow_gain"] = 1
		delete(d.values, "afterglow_total")
	}
	if d.timers["cloud-drift"] <= 0 {
		d.values["cloud_drift_bias"] = 0
		delete(d.values, "cloud_drift_total")
	}
	if d.timers["distant-thunder"] <= 0 {
		d.values["distant_thunder_gain"] = 1
		delete(d.values, "distant_thunder_total")
	}
	if d.timers["intro"] <= 0 {
		delete(d.values, "intro_total")
	}
	if d.timers["ending"] <= 0 {
		delete(d.values, "ending_total")
	}
	if d.timers["intro"] > 0 || d.timers["ending"] > 0 {
		return
	}
	if d.timers["lightning"] <= 0 && d.timers["quiet-horizon"] <= 0 && d.cfg["lightning_p"] > 0 && d.rng.Float64() < d.cfg["lightning_p"] {
		d.startLightningLocked("started")
	}
	if d.timers["afterglow"] <= 0 && d.cfg["afterglow_p"] > 0 && d.rng.Float64() < d.cfg["afterglow_p"] {
		d.startAfterglowLocked("started")
	}
	if d.timers["cloud-drift"] <= 0 && d.cfg["cloud_drift_p"] > 0 && d.rng.Float64() < d.cfg["cloud_drift_p"] {
		d.startCloudDriftLocked("started")
	}
	if d.timers["distant-thunder"] <= 0 && d.cfg["distant_thunder_p"] > 0 && d.rng.Float64() < d.cfg["distant_thunder_p"] {
		d.startDistantThunderLocked("started")
	}
	if d.timers["quiet-horizon"] <= 0 && d.cfg["quiet_horizon_p"] > 0 && d.rng.Float64() < d.cfg["quiet_horizon_p"] {
		d.startQuietHorizonLocked("started")
	}
}
