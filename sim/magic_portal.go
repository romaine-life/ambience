package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	magicPortalActive  = 0
	magicPortalDormant = 1
)

type magicPortalEmber struct {
	Row, Col   float64
	VRow, VCol float64
	Life       int
	MaxLife    int
	HueOffset  float64
	Brightness float64
}

// MagicPortalConfig tunes the magic-portal prototype used in isolated dev
// sessions. Hue and pulse period are the primary scene knobs.
type MagicPortalConfig struct {
	// INTRODUCTION
	IntroResolve int `json:"intro_resolve"`
	IntroPulse   int `json:"intro_pulse"`
	// ENDING
	EndingDecay    int     `json:"ending_decay"`
	EndingSurge    int     `json:"ending_surge"`
	EmberTail      int     `json:"ember_tail"`
	FinalSurgeMult float64 `json:"final_surge"`
	// LEVERS - pulse and form
	PulsePeriod int     `json:"pulse_period"`
	PulseAmp    float64 `json:"pulse_amp"`
	PulseExpand float64 `json:"pulse_expand"`
	RingRadius  float64 `json:"radius"`
	RingWidth   float64 `json:"ring_width"`
	RuneCount   int     `json:"runes"`
	MaxEmbers   int     `json:"max_embers"`
	// LEVERS - color
	Hue      float64 `json:"hue"`
	Sat      float64 `json:"sat"`
	LightMin float64 `json:"lmin"`
	LightMax float64 `json:"lmax"`
	Glow     float64 `json:"glow"`
	// EVENT CHANCES
	EmberPeakChance float64 `json:"ember_peak_p"`
	SurgeChance     float64 `json:"surge_p"`
	BurstChance     float64 `json:"burst_p"`
	RuneShiftChance float64 `json:"rune_shift_p"`
	QuietChance     float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	PulseDur     int     `json:"pulse_dur"`
	SurgeDur     int     `json:"surge_dur"`
	SurgeMult    float64 `json:"surge_mult"`
	BurstCount   int     `json:"burst_count"`
	RuneShiftDur int     `json:"rune_shift_dur"`
	QuietDur     int     `json:"quiet_dur"`
	QuietMult    float64 `json:"quiet_mult"`
	EmberLife    int     `json:"ember_life"`
	EmberDrift   float64 `json:"ember_drift"`
}

func (c MagicPortalConfig) withDefaults() MagicPortalConfig {
	zero := c == MagicPortalConfig{}
	if c.IntroResolve <= 0 {
		c.IntroResolve = 36
	}
	if c.IntroPulse <= 0 {
		c.IntroPulse = 42
	}
	if c.EndingDecay <= 0 {
		c.EndingDecay = 48
	}
	if c.EndingSurge <= 0 {
		c.EndingSurge = 20
	}
	if c.EmberTail <= 0 {
		c.EmberTail = 42
	}
	if c.FinalSurgeMult <= 0 {
		c.FinalSurgeMult = 1.8
	}
	if c.PulsePeriod <= 0 {
		c.PulsePeriod = 42
	}
	if c.PulsePeriod < 12 {
		c.PulsePeriod = 12
	}
	if c.PulseAmp <= 0 {
		c.PulseAmp = 0.42
	}
	c.PulseAmp = clamp01(c.PulseAmp)
	if c.PulseExpand < 0 {
		c.PulseExpand = 0
	}
	if zero && c.PulseExpand == 0 {
		c.PulseExpand = 1.6
	}
	if c.RingRadius <= 0 {
		c.RingRadius = 0.285
	}
	if c.RingRadius > 0.48 {
		c.RingRadius = 0.48
	}
	if c.RingWidth <= 0 {
		c.RingWidth = 2.2
	}
	if c.RuneCount <= 0 {
		c.RuneCount = 14
	}
	if c.RuneCount > 36 {
		c.RuneCount = 36
	}
	if c.MaxEmbers <= 0 {
		c.MaxEmbers = 80
	}
	if c.MaxEmbers > 220 {
		c.MaxEmbers = 220
	}
	if zero {
		c.Hue = 205
	}
	c.Hue = math.Mod(c.Hue+360, 360)
	if c.Sat <= 0 {
		c.Sat = 0.78
	}
	c.Sat = clamp01(c.Sat)
	if c.LightMin <= 0 {
		c.LightMin = 0.08
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.88
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	c.LightMin = clamp01(c.LightMin)
	c.LightMax = clamp01(c.LightMax)
	if zero && c.Glow == 0 {
		c.Glow = 0.54
	}
	if c.Glow < 0 {
		c.Glow = 0
	}
	c.Glow = clamp01(c.Glow)
	if zero {
		c.EmberPeakChance = 0.48
		c.SurgeChance = 0.0007
		c.BurstChance = 0.001
		c.RuneShiftChance = 0.0008
		c.QuietChance = 0.0004
	}
	if c.EmberPeakChance < 0 {
		c.EmberPeakChance = 0
	}
	if c.SurgeChance < 0 {
		c.SurgeChance = 0
	}
	if c.BurstChance < 0 {
		c.BurstChance = 0
	}
	if c.RuneShiftChance < 0 {
		c.RuneShiftChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.PulseDur <= 0 {
		c.PulseDur = 18
	}
	if c.SurgeDur <= 0 {
		c.SurgeDur = 46
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 2.0
	}
	if c.BurstCount <= 0 {
		c.BurstCount = 10
	}
	if c.RuneShiftDur <= 0 {
		c.RuneShiftDur = 24
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 110
	}
	if c.QuietMult <= 0 || c.QuietMult > 1 {
		c.QuietMult = 0.28
	}
	if c.EmberLife <= 0 {
		c.EmberLife = 54
	}
	if c.EmberDrift <= 0 {
		c.EmberDrift = 0.38
	}
	return c
}

// MagicPortalSchema describes the MagicPortal effect's tunable knobs for the
// dev UI.
func MagicPortalSchema() EffectSchema {
	return EffectSchema{
		Name: "magic-portal",
		Knobs: []Knob{
			{Key: "intro_resolve", Label: "resolve", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 8, Max: 120, Step: 4, Default: 36, Trigger: "intro",
				Description: "Ticks spent flickering dark runes into a stable portal ring."},
			{Key: "intro_pulse", Label: "first pulse", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 12, Max: 120, Step: 4, Default: 42,
				Description: "Ticks for the first full-amplitude breath after the runes resolve."},
			{Key: "ending_decay", Label: "decay", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 160, Step: 5, Default: 48, Trigger: "ending",
				Description: "Ticks spent dimming the gate after the final crackling surge."},
			{Key: "ending_surge", Label: "final surge", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 4, Max: 80, Step: 2, Default: 20,
				Description: "Ticks for the bright crackle at the start of the outro."},
			{Key: "ember_tail", Label: "ember tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 8, Max: 140, Step: 4, Default: 42,
				Description: "How long embers keep fading after the gate has mostly gone dark."},
			{Key: "final_surge", Label: "surge x", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 1, Max: 4, Step: 0.05, Default: 1.8,
				Description: "Brightness multiplier for the outro's final pulse."},
			{Key: "pulse_period", Label: "pulse period", Slot: SlotLever, Group: "pulse", Type: KnobInt, Min: 18, Max: 90, Step: 2, Default: 42,
				Description: "Ticks per breathing pulse. Higher values make the gate breathe slower."},
			{Key: "pulse_amp", Label: "pulse amp", Slot: SlotLever, Group: "pulse", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.01, Default: 0.42,
				Description: "How much the ring brightens at the top of each pulse."},
			{Key: "pulse_expand", Label: "expand", Slot: SlotLever, Group: "pulse", Type: KnobFloat, Min: 0, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Pixel radius added as the ring reaches each pulse peak."},
			{Key: "radius", Label: "radius", Slot: SlotLever, Group: "gate", Type: KnobFloat, Min: 0.18, Max: 0.42, Step: 0.005, Default: 0.285,
				Description: "Portal ring radius as a fraction of the shorter frame dimension."},
			{Key: "ring_width", Label: "ring width", Slot: SlotLever, Group: "gate", Type: KnobFloat, Min: 1, Max: 5, Step: 0.1, Default: 2.2,
				Description: "Thickness of the lit runic gate ring in pixels."},
			{Key: "runes", Label: "runes", Slot: SlotLever, Group: "gate", Type: KnobInt, Min: 6, Max: 28, Step: 1, Default: 14,
				Description: "Number of runic marks spaced around the ring."},
			{Key: "max_embers", Label: "max embers", Slot: SlotLever, Group: "embers", Type: KnobInt, Min: 8, Max: 180, Step: 1, Default: 80,
				Description: "Cap on live embers drifting outward from the ring."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 205,
				Description: "Scene hue: blue arcane, red infernal, amber ancient, or desaturated gray."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.78,
				Description: "Overall portal saturation. Lower values give a dormant stone-gray gate."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.02, Max: 0.35, Step: 0.01, Default: 0.08,
				Description: "Minimum lightness for the dark stone and low glow."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.35, Max: 1, Step: 0.01, Default: 0.88,
				Description: "Maximum lightness for rune peaks and ember cores."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.54,
				Description: "Strength of the central portal glow inside the ring."},
			{Key: "ember_peak_p", Label: "peak embers", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.48, Trigger: "pulse",
				Description: "Chance that a steady pulse peak releases a small ember."},
			{Key: "surge_p", Label: "surge", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0007, Trigger: "power-surge",
				Description: "Per-tick chance of a brighter, longer pulse with extra embers."},
			{Key: "burst_p", Label: "ember burst", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0.001, Trigger: "ember-burst",
				Description: "Per-tick chance of a small ember cluster firing from the ring."},
			{Key: "rune_shift_p", Label: "rune shift", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0.0008, Trigger: "rune-shift",
				Description: "Per-tick chance that one rune briefly shifts around the gate."},
			{Key: "quiet_p", Label: "quiet gate", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0004, Trigger: "quiet-gate",
				Description: "Per-tick chance of a long dim window with subtle pulses."},
			{Key: "pulse_dur", Label: "pulse dur", Slot: SlotEventMod, Group: "pulse", Type: KnobInt, Min: 6, Max: 60, Step: 2, Default: 18,
				Description: "Duration of a manually triggered single brightness peak."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 46,
				Description: "Duration of a power-surge pulse."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1, Max: 4, Step: 0.05, Default: 2,
				Description: "Brightness and ember-density multiplier during a power surge."},
			{Key: "burst_count", Label: "burst count", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 2, Max: 36, Step: 1, Default: 10,
				Description: "Approximate ember count released by ember-burst and surge events."},
			{Key: "rune_shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "runes", Type: KnobInt, Min: 6, Max: 80, Step: 2, Default: 24,
				Description: "Duration of the selected rune's temporary rotation."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet", Type: KnobInt, Min: 20, Max: 260, Step: 5, Default: 110,
				Description: "Duration of a quiet-gate suppression window."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.28,
				Description: "Brightness and ember-rate multiplier while quiet-gate is active."},
			{Key: "ember_life", Label: "ember life", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 12, Max: 140, Step: 4, Default: 54,
				Description: "Ticks an ember drifts before fading out."},
			{Key: "ember_drift", Label: "ember drift", Slot: SlotEventMod, Group: "embers", Type: KnobFloat, Min: 0.08, Max: 1.2, Step: 0.02, Default: 0.38,
				Description: "Outward speed applied to embers spawned from the gate ring."},
		},
	}
}

type MagicPortalState struct {
	Tick           int `json:"tick"`
	LifeState      int `json:"lifeState"`
	IntroTicks     int `json:"introTicks"`
	IntroTotal     int `json:"introTotal"`
	EndingTicks    int `json:"endingTicks"`
	EndingTotal    int `json:"endingTotal"`
	EndingSurge    int `json:"endingSurge"`
	PulseTicks     int `json:"pulseTicks"`
	PulseTotal     int `json:"pulseTotal"`
	SurgeTicks     int `json:"surgeTicks"`
	SurgeTotal     int `json:"surgeTotal"`
	QuietTicks     int `json:"quietTicks"`
	RuneShiftTicks int `json:"runeShiftTicks"`
	RuneShiftTotal int `json:"runeShiftTotal"`
	RuneShiftIndex int `json:"runeShiftIndex"`
}

type MagicPortalEmber struct {
	Row        float64 `json:"row"`
	Col        float64 `json:"col"`
	VRow       float64 `json:"vRow"`
	VCol       float64 `json:"vCol"`
	Life       int     `json:"life"`
	MaxLife    int     `json:"maxLife"`
	HueOffset  float64 `json:"hueOffset"`
	Brightness float64 `json:"brightness"`
}

type MagicPortalSnapshot struct {
	MagicPortalState
	RNGState uint64             `json:"rngState,omitempty"`
	Embers   []MagicPortalEmber `json:"embers,omitempty"`
}

type MagicPortalPersistedState struct {
	MagicPortalState
	RNGState uint64             `json:"rngState"`
	Embers   []MagicPortalEmber `json:"embers,omitempty"`
}

// MagicPortal is a centered runic gate with a breathing pulse and rare embers.
type MagicPortal struct {
	mu sync.Mutex

	W, H   int
	Grid   [][]Pixel
	embers []magicPortalEmber
	rng    *rngutil.RNG
	cfg    MagicPortalConfig
	tick   int

	lifeState      int
	introTicks     int
	introTotal     int
	endingTicks    int
	endingTotal    int
	endingSurge    int
	pulseTicks     int
	pulseTotal     int
	surgeTicks     int
	surgeTotal     int
	quietTicks     int
	runeShiftTicks int
	runeShiftTotal int
	runeShiftIndex int

	log []LogEntry
}

func NewMagicPortal(w, h int, seed int64, cfg MagicPortalConfig) *MagicPortal {
	m := &MagicPortal{
		W:              w,
		H:              h,
		Grid:           makePixelGrid(w, h),
		rng:            rngutil.New(seed),
		cfg:            cfg.withDefaults(),
		lifeState:      magicPortalActive,
		runeShiftIndex: -1,
	}
	m.rebuildGridLocked()
	return m
}

func (m *MagicPortal) Resize(w, h int) {
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
	m.Grid = makePixelGrid(w, h)
	m.rebuildGridLocked()
}

func (m *MagicPortal) SetConfig(cfg MagicPortalConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg.withDefaults()
	if m.runeShiftIndex >= m.cfg.RuneCount {
		m.runeShiftIndex = m.cfg.RuneCount - 1
	}
	if len(m.embers) > m.cfg.MaxEmbers {
		m.embers = m.embers[len(m.embers)-m.cfg.MaxEmbers:]
	}
	m.rebuildGridLocked()
}

func (m *MagicPortal) EffectiveConfig() MagicPortalConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *MagicPortal) SnapshotState() MagicPortalState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotStateLocked()
}

func (m *MagicPortal) Snapshot() MagicPortalSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MagicPortalSnapshot{
		MagicPortalState: m.snapshotStateLocked(),
		RNGState:         m.rng.State(),
		Embers:           m.copyEmbersLocked(),
	}
}

func (m *MagicPortal) RestoreState(s MagicPortalState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(s)
	m.rebuildGridLocked()
}

func (m *MagicPortal) RestoreSnapshot(s MagicPortalSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(s.MagicPortalState)
	if s.RNGState != 0 {
		m.rng.SetState(s.RNGState)
	}
	m.restoreEmbersLocked(s.Embers)
	m.rebuildGridLocked()
}

func (m *MagicPortal) SnapshotPersistedState() MagicPortalPersistedState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MagicPortalPersistedState{
		MagicPortalState: m.snapshotStateLocked(),
		RNGState:         m.rng.State(),
		Embers:           m.copyEmbersLocked(),
	}
}

func (m *MagicPortal) RestorePersistedState(s MagicPortalPersistedState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(s.MagicPortalState)
	if s.RNGState != 0 {
		m.rng.SetState(s.RNGState)
	}
	m.restoreEmbersLocked(s.Embers)
	m.rebuildGridLocked()
}

func (m *MagicPortal) CurrentTick() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tick
}

func (m *MagicPortal) PerturbRNG(delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rng.Mix(delta)
}

func (m *MagicPortal) TriggerEvent(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch name {
	case "intro":
		m.startIntroLocked()
		m.appendLog("intro", fmt.Sprintf("started (resolve=%d, pulse=%d)", m.cfg.IntroResolve, m.cfg.IntroPulse))
	case "ending":
		m.startEndingLocked()
		m.appendLog("ending", fmt.Sprintf("started (surge=%d, decay=%d, tail=%d)", m.endingSurge, m.cfg.EndingDecay, m.cfg.EmberTail))
	case "pulse":
		m.startPulseLocked()
		m.spawnEmberBurstLocked(2, 1)
		m.appendLog("pulse", fmt.Sprintf("triggered (dur=%d)", m.pulseTotal))
	case "power-surge":
		m.startSurgeLocked(m.cfg.SurgeMult)
		m.appendLog("power-surge", fmt.Sprintf("triggered (dur=%d, x%.2f)", m.surgeTotal, m.cfg.SurgeMult))
	case "ember-burst":
		count := jitterInt(m.rng, m.cfg.BurstCount, 0.25)
		m.spawnEmberBurstLocked(count, 1.05)
		m.appendLog("ember-burst", fmt.Sprintf("triggered (%d embers)", count))
	case "rune-shift":
		m.startRuneShiftLocked()
		m.appendLog("rune-shift", fmt.Sprintf("triggered (rune=%d, dur=%d)", m.runeShiftIndex, m.runeShiftTotal))
	case "quiet-gate":
		m.quietTicks = jitterInt(m.rng, m.cfg.QuietDur, 0.25)
		m.appendLog("quiet-gate", fmt.Sprintf("triggered (dur=%d, x%.2f)", m.quietTicks, m.cfg.QuietMult))
	default:
		return false
	}
	m.rebuildGridLocked()
	return true
}

func (m *MagicPortal) DrainLog() []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.log) == 0 {
		return nil
	}
	out := m.log
	m.log = nil
	return out
}

func (m *MagicPortal) Step() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tick++
	endingActive := m.endingTicks > 0
	activeGate := m.lifeState == magicPortalActive || m.introTicks > 0 || endingActive

	if activeGate && !endingActive {
		if m.isPulsePeakLocked() {
			chance := m.cfg.EmberPeakChance
			if m.quietTicks > 0 {
				chance *= m.cfg.QuietMult
			}
			if m.rng.Float64() < chance {
				m.spawnEmberBurstLocked(1+m.rng.Intn(2), 0.8)
			}
		}
		if m.surgeTicks == 0 && m.quietTicks == 0 && m.rng.Float64() < m.cfg.SurgeChance {
			m.startSurgeLocked(m.cfg.SurgeMult)
			m.appendLog("power-surge", fmt.Sprintf("started (dur=%d, x%.2f)", m.surgeTotal, m.cfg.SurgeMult))
		}
		if m.rng.Float64() < m.cfg.BurstChance {
			count := max(1, jitterInt(m.rng, m.cfg.BurstCount, 0.25))
			m.spawnEmberBurstLocked(count, 1)
			m.appendLog("ember-burst", fmt.Sprintf("started (%d embers)", count))
		}
		if m.runeShiftTicks == 0 && m.rng.Float64() < m.cfg.RuneShiftChance {
			m.startRuneShiftLocked()
			m.appendLog("rune-shift", fmt.Sprintf("started (rune=%d, dur=%d)", m.runeShiftIndex, m.runeShiftTotal))
		}
		if m.quietTicks == 0 && m.rng.Float64() < m.cfg.QuietChance {
			m.quietTicks = jitterInt(m.rng, m.cfg.QuietDur, 0.25)
			m.appendLog("quiet-gate", fmt.Sprintf("started (dur=%d, x%.2f)", m.quietTicks, m.cfg.QuietMult))
		}
	}

	if endingActive && m.endingElapsedLocked() < m.endingSurge && m.rng.Float64() < 0.45 {
		m.spawnEmberBurstLocked(1+m.rng.Intn(3), 1.35)
	}
	if m.surgeTicks > 0 && m.rng.Float64() < 0.18 {
		m.spawnEmberBurstLocked(1+m.rng.Intn(2), 1.2)
	}

	m.stepEmbersLocked()
	m.decrementTimersLocked()
	m.rebuildGridLocked()
}

func (m *MagicPortal) appendLog(kind, desc string) {
	m.log = append(m.log, LogEntry{Tick: m.tick, Type: kind, Desc: desc})
	if len(m.log) > 200 {
		m.log = m.log[len(m.log)-200:]
	}
}

func (m *MagicPortal) snapshotStateLocked() MagicPortalState {
	return MagicPortalState{
		Tick:           m.tick,
		LifeState:      m.lifeState,
		IntroTicks:     m.introTicks,
		IntroTotal:     m.introTotal,
		EndingTicks:    m.endingTicks,
		EndingTotal:    m.endingTotal,
		EndingSurge:    m.endingSurge,
		PulseTicks:     m.pulseTicks,
		PulseTotal:     m.pulseTotal,
		SurgeTicks:     m.surgeTicks,
		SurgeTotal:     m.surgeTotal,
		QuietTicks:     m.quietTicks,
		RuneShiftTicks: m.runeShiftTicks,
		RuneShiftTotal: m.runeShiftTotal,
		RuneShiftIndex: m.runeShiftIndex,
	}
}

func (m *MagicPortal) restoreStateLocked(s MagicPortalState) {
	m.tick = s.Tick
	if s.LifeState == magicPortalDormant {
		m.lifeState = magicPortalDormant
	} else {
		m.lifeState = magicPortalActive
	}
	m.introTicks = max(0, s.IntroTicks)
	m.introTotal = max(0, s.IntroTotal)
	m.endingTicks = max(0, s.EndingTicks)
	m.endingTotal = max(0, s.EndingTotal)
	m.endingSurge = max(0, s.EndingSurge)
	m.pulseTicks = max(0, s.PulseTicks)
	m.pulseTotal = max(0, s.PulseTotal)
	m.surgeTicks = max(0, s.SurgeTicks)
	m.surgeTotal = max(0, s.SurgeTotal)
	m.quietTicks = max(0, s.QuietTicks)
	m.runeShiftTicks = max(0, s.RuneShiftTicks)
	m.runeShiftTotal = max(0, s.RuneShiftTotal)
	m.runeShiftIndex = s.RuneShiftIndex
	if m.runeShiftIndex >= m.cfg.RuneCount {
		m.runeShiftIndex = m.cfg.RuneCount - 1
	}
	if m.runeShiftIndex < -1 {
		m.runeShiftIndex = -1
	}
}

func (m *MagicPortal) copyEmbersLocked() []MagicPortalEmber {
	out := make([]MagicPortalEmber, len(m.embers))
	for i, e := range m.embers {
		out[i] = MagicPortalEmber{
			Row:        e.Row,
			Col:        e.Col,
			VRow:       e.VRow,
			VCol:       e.VCol,
			Life:       e.Life,
			MaxLife:    e.MaxLife,
			HueOffset:  e.HueOffset,
			Brightness: e.Brightness,
		}
	}
	return out
}

func (m *MagicPortal) restoreEmbersLocked(list []MagicPortalEmber) {
	m.embers = make([]magicPortalEmber, 0, min(len(list), m.cfg.MaxEmbers))
	for _, e := range list {
		if e.Life <= 0 || e.MaxLife <= 0 {
			continue
		}
		m.embers = append(m.embers, magicPortalEmber{
			Row:        e.Row,
			Col:        e.Col,
			VRow:       e.VRow,
			VCol:       e.VCol,
			Life:       e.Life,
			MaxLife:    e.MaxLife,
			HueOffset:  e.HueOffset,
			Brightness: e.Brightness,
		})
		if len(m.embers) >= m.cfg.MaxEmbers {
			break
		}
	}
}

func (m *MagicPortal) startIntroLocked() {
	m.lifeState = magicPortalActive
	m.introTotal = max(1, m.cfg.IntroResolve+m.cfg.IntroPulse)
	m.introTicks = m.introTotal
	m.endingTicks = 0
	m.endingTotal = 0
	m.endingSurge = 0
	m.quietTicks = 0
	m.surgeTicks = 0
	m.surgeTotal = 0
	m.runeShiftTicks = 0
	m.runeShiftIndex = -1
	m.embers = nil
	m.startPulseLocked()
}

func (m *MagicPortal) startEndingLocked() {
	m.lifeState = magicPortalActive
	m.introTicks = 0
	m.introTotal = 0
	m.quietTicks = 0
	m.pulseTicks = 0
	m.pulseTotal = 0
	m.surgeTicks = 0
	m.surgeTotal = 0
	m.endingSurge = max(1, m.cfg.EndingSurge)
	m.endingTotal = max(1, m.endingSurge+m.cfg.EndingDecay+m.cfg.EmberTail)
	m.endingTicks = m.endingTotal
	m.spawnEmberBurstLocked(max(4, m.cfg.BurstCount), 1.45)
}

func (m *MagicPortal) startPulseLocked() {
	m.pulseTotal = max(1, jitterInt(m.rng, m.cfg.PulseDur, 0.2))
	m.pulseTicks = m.pulseTotal
}

func (m *MagicPortal) startSurgeLocked(mult float64) {
	m.surgeTotal = max(1, jitterInt(m.rng, m.cfg.SurgeDur, 0.2))
	m.surgeTicks = m.surgeTotal
	count := int(math.Round(float64(m.cfg.BurstCount) * math.Max(1, mult) * 0.85))
	m.spawnEmberBurstLocked(max(3, count), math.Max(1, mult*0.6))
}

func (m *MagicPortal) startRuneShiftLocked() {
	m.runeShiftTotal = max(1, jitterInt(m.rng, m.cfg.RuneShiftDur, 0.25))
	m.runeShiftTicks = m.runeShiftTotal
	m.runeShiftIndex = m.rng.Intn(max(1, m.cfg.RuneCount))
}

func (m *MagicPortal) decrementTimersLocked() {
	if m.introTicks > 0 {
		m.introTicks--
	}
	if m.endingTicks > 0 {
		m.endingTicks--
		if m.endingTicks == 0 {
			m.lifeState = magicPortalDormant
			m.embers = nil
			m.surgeTicks = 0
			m.surgeTotal = 0
			m.pulseTicks = 0
			m.pulseTotal = 0
			m.runeShiftTicks = 0
			m.runeShiftIndex = -1
		}
	}
	if m.pulseTicks > 0 {
		m.pulseTicks--
	}
	if m.surgeTicks > 0 {
		m.surgeTicks--
	}
	if m.quietTicks > 0 {
		m.quietTicks--
	}
	if m.runeShiftTicks > 0 {
		m.runeShiftTicks--
		if m.runeShiftTicks == 0 {
			m.runeShiftIndex = -1
		}
	}
}

func (m *MagicPortal) stepEmbersLocked() {
	if len(m.embers) == 0 {
		return
	}
	alive := m.embers[:0]
	for _, e := range m.embers {
		e.Col += e.VCol
		e.Row += e.VRow
		e.VCol *= 0.985
		e.VRow *= 0.985
		if m.surgeTicks > 0 || (m.endingTicks > 0 && m.endingElapsedLocked() < m.endingSurge) {
			cx, cy := m.centerLocked()
			dx := e.Col - cx
			dy := e.Row - cy
			l := math.Hypot(dx, dy)
			if l > 0 {
				e.VCol += dx / l * 0.012
				e.VRow += dy / l * 0.012
			}
		}
		e.Life--
		if e.Life > 0 && e.Col >= -4 && e.Col < float64(m.W)+4 && e.Row >= -4 && e.Row < float64(m.H)+4 {
			alive = append(alive, e)
		}
	}
	m.embers = alive
}

func (m *MagicPortal) spawnEmberBurstLocked(count int, mult float64) {
	if count <= 0 || m.W <= 0 || m.H <= 0 || m.cfg.MaxEmbers <= 0 {
		return
	}
	if m.lifeState == magicPortalDormant && m.introTicks == 0 && m.endingTicks == 0 {
		return
	}
	cx, cy := m.centerLocked()
	radius := m.baseRadiusLocked()
	for i := 0; i < count && len(m.embers) < m.cfg.MaxEmbers; i++ {
		angle := m.rng.Float64() * math.Pi * 2
		cosA, sinA := math.Cos(angle), math.Sin(angle)
		speed := m.cfg.EmberDrift * (0.55 + m.rng.Float64()*0.8) * math.Max(0.2, mult)
		life := max(4, jitterInt(m.rng, m.cfg.EmberLife, 0.35))
		m.embers = append(m.embers, magicPortalEmber{
			Row:        cy + sinA*radius*(0.96+m.rng.Float64()*0.08),
			Col:        cx + cosA*radius*(0.96+m.rng.Float64()*0.08),
			VRow:       sinA*speed - 0.025*(0.5+m.rng.Float64()),
			VCol:       cosA*speed + (m.rng.Float64()*2-1)*0.05,
			Life:       life,
			MaxLife:    life,
			HueOffset:  (m.rng.Float64()*2 - 1) * 18,
			Brightness: 0.75 + m.rng.Float64()*0.55,
		})
	}
}

func (m *MagicPortal) rebuildGridLocked() {
	if len(m.Grid) != m.H || (m.H > 0 && len(m.Grid[0]) != m.W) {
		m.Grid = makePixelGrid(m.W, m.H)
	}
	m.clearGridLocked()
	if m.W <= 0 || m.H <= 0 {
		return
	}
	m.paintBackgroundLocked()
	level := m.lifecycleLevelLocked()
	if level > 0.006 {
		m.paintGlowLocked(level)
		m.paintRingLocked(level)
		m.paintRunesLocked(level)
	}
	m.paintEmbersLocked()
}

func (m *MagicPortal) clearGridLocked() {
	for y := range m.Grid {
		for x := range m.Grid[y] {
			m.Grid[y][x] = Pixel{}
		}
	}
}

func (m *MagicPortal) paintBackgroundLocked() {
	hue := math.Mod(m.cfg.Hue+228, 360)
	sat := clamp01(m.cfg.Sat * 0.18)
	for y := 0; y < m.H; y++ {
		t := float64(y) / math.Max(1, float64(m.H-1))
		light := clamp01(m.cfg.LightMin * (0.11 + 0.18*t))
		c := hslToRGB(hue, sat, light)
		for x := 0; x < m.W; x++ {
			m.Grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (m *MagicPortal) paintGlowLocked(level float64) {
	cx, cy := m.centerLocked()
	radius := m.currentRadiusLocked()
	inner := math.Max(2, radius*0.83)
	glowC := hslToRGB(m.cfg.Hue, clamp01(m.cfg.Sat*0.94), clamp01(m.cfg.LightMax*0.72))
	for y := int(math.Floor(cy - inner)); y <= int(math.Ceil(cy+inner)); y++ {
		for x := int(math.Floor(cx - inner)); x <= int(math.Ceil(cx+inner)); x++ {
			d := math.Hypot((float64(x)-cx)*1.03, float64(y)-cy)
			if d > inner {
				continue
			}
			core := 1 - d/inner
			alpha := math.Pow(core, 1.8) * m.cfg.Glow * level * (0.55 + 0.45*m.breathLocked())
			m.paintAlphaMax(x, y, glowC, alpha)
		}
	}
}

func (m *MagicPortal) paintRingLocked(level float64) {
	cx, cy := m.centerLocked()
	radius := m.currentRadiusLocked()
	width := math.Max(0.5, m.cfg.RingWidth)
	breath := m.breathLocked()
	pulse := m.eventPulseLocked()
	halo := hslToRGB(m.cfg.Hue, clamp01(m.cfg.Sat*0.72), clamp01(m.cfg.LightMin+(m.cfg.LightMax-m.cfg.LightMin)*0.42))
	ring := hslToRGB(m.cfg.Hue, m.cfg.Sat, clamp01(m.cfg.LightMin+(m.cfg.LightMax-m.cfg.LightMin)*(0.52+0.34*breath+0.12*pulse)))
	outer := radius + width*3
	for y := int(math.Floor(cy - outer)); y <= int(math.Ceil(cy+outer)); y++ {
		for x := int(math.Floor(cx - outer)); x <= int(math.Ceil(cx+outer)); x++ {
			d := math.Hypot((float64(x)-cx)*1.03, float64(y)-cy)
			dist := math.Abs(d - radius)
			if dist <= width {
				edge := 1 - dist/width
				alpha := level * (0.52 + 0.46*edge) * (0.86 + 0.14*breath)
				m.paintAlphaMax(x, y, ring, alpha)
				continue
			}
			if dist <= width*3 {
				alpha := level * 0.16 * (1 - (dist-width)/(width*2)) * (0.8 + 0.2*breath)
				m.paintAlphaMax(x, y, halo, alpha)
			}
		}
	}
	stone := hslToRGB(math.Mod(m.cfg.Hue+210, 360), clamp01(m.cfg.Sat*0.16), clamp01(m.cfg.LightMin*1.5))
	baseY := int(math.Round(cy + radius*0.82))
	for dx := -int(radius * 0.72); dx <= int(radius*0.72); dx++ {
		m.paintAlphaMax(int(math.Round(cx))+dx, baseY, stone, level*0.55)
	}
}

func (m *MagicPortal) paintRunesLocked(level float64) {
	count := max(1, m.cfg.RuneCount)
	introProgress := 1.0
	if m.introTicks > 0 && m.introTotal > 0 {
		introProgress = phaseProgress(m.introTotal, m.introTicks)
	}
	cx, cy := m.centerLocked()
	radius := m.currentRadiusLocked() + m.cfg.RingWidth + 1.1
	runeC := hslToRGB(math.Mod(m.cfg.Hue+8, 360), clamp01(m.cfg.Sat*0.94), clamp01(m.cfg.LightMax*0.96))
	for i := 0; i < count; i++ {
		if introProgress < 0.98 {
			threshold := float64(i+1) / float64(count)
			flicker := 0.08 * math.Sin(float64(m.tick+i*17)*0.7)
			if introProgress+flicker < threshold {
				continue
			}
		}
		angle := float64(i)/float64(count)*math.Pi*2 - math.Pi/2
		alpha := level * (0.62 + 0.25*math.Sin(float64(m.tick)*0.11+float64(i)))
		if i == m.runeShiftIndex && m.runeShiftTicks > 0 {
			env := eventEnvelope(m.runeShiftTotal, m.runeShiftTicks)
			angle += env * 0.42
			alpha *= 1.45
		}
		m.paintRuneLocked(cx, cy, radius, angle, runeC, alpha, i)
	}
}

func (m *MagicPortal) paintRuneLocked(cx, cy, radius, angle float64, c color.RGBA, alpha float64, idx int) {
	x := cx + math.Cos(angle)*radius
	y := cy + math.Sin(angle)*radius
	tx := -math.Sin(angle)
	ty := math.Cos(angle)
	nx := math.Cos(angle)
	ny := math.Sin(angle)
	cx0 := int(math.Round(x))
	cy0 := int(math.Round(y))
	m.paintAlphaMax(cx0, cy0, c, alpha)
	switch idx % 4 {
	case 0:
		m.paintAlphaMax(int(math.Round(x+tx)), int(math.Round(y+ty)), c, alpha*0.9)
		m.paintAlphaMax(int(math.Round(x-tx)), int(math.Round(y-ty)), c, alpha*0.9)
	case 1:
		m.paintAlphaMax(int(math.Round(x+nx)), int(math.Round(y+ny)), c, alpha*0.85)
		m.paintAlphaMax(int(math.Round(x-tx)), int(math.Round(y-ty)), c, alpha*0.72)
	case 2:
		m.paintAlphaMax(int(math.Round(x+tx)), int(math.Round(y+ty)), c, alpha*0.8)
		m.paintAlphaMax(int(math.Round(x-nx)), int(math.Round(y-ny)), c, alpha*0.7)
	default:
		m.paintAlphaMax(int(math.Round(x+nx)), int(math.Round(y+ny)), c, alpha*0.75)
		m.paintAlphaMax(int(math.Round(x-nx)), int(math.Round(y-ny)), c, alpha*0.75)
	}
}

func (m *MagicPortal) paintEmbersLocked() {
	for _, e := range m.embers {
		fade := clamp01(float64(e.Life) / math.Max(1, float64(e.MaxLife)))
		if fade <= 0 {
			continue
		}
		hue := math.Mod(m.cfg.Hue+e.HueOffset+18+360, 360)
		c := hslToRGB(hue, clamp01(m.cfg.Sat*0.96), clamp01(m.cfg.LightMax*e.Brightness))
		alpha := math.Pow(fade, 0.65)
		x := int(math.Round(e.Col))
		y := int(math.Round(e.Row))
		m.paintAlphaMax(x, y, c, alpha)
		tail := hslToRGB(hue, clamp01(m.cfg.Sat*0.72), clamp01(m.cfg.LightMax*0.55*e.Brightness))
		m.paintAlphaMax(int(math.Round(e.Col-e.VCol*1.4)), int(math.Round(e.Row-e.VRow*1.4)), tail, alpha*0.42)
		if e.Brightness > 1.05 && fade > 0.35 {
			m.paintAlphaMax(x+1, y, tail, alpha*0.28)
			m.paintAlphaMax(x-1, y, tail, alpha*0.22)
		}
	}
}

func (m *MagicPortal) paintAlphaMax(x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= m.H || x < 0 || x >= m.W {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	c.R = uint8(float64(c.R) * alpha)
	c.G = uint8(float64(c.G) * alpha)
	c.B = uint8(float64(c.B) * alpha)
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return
	}
	cur := m.Grid[y][x]
	if !cur.Filled {
		m.Grid[y][x] = Pixel{Filled: true, C: color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}}
		return
	}
	if c.R > cur.C.R {
		cur.C.R = c.R
	}
	if c.G > cur.C.G {
		cur.C.G = c.G
	}
	if c.B > cur.C.B {
		cur.C.B = c.B
	}
	cur.C.A = 255
	cur.Filled = true
	m.Grid[y][x] = cur
}

func (m *MagicPortal) lifecycleLevelLocked() float64 {
	if m.lifeState == magicPortalDormant && m.introTicks == 0 && m.endingTicks == 0 {
		return 0
	}
	level := 1.0
	if m.introTicks > 0 && m.introTotal > 0 {
		p := phaseProgress(m.introTotal, m.introTicks)
		level *= math.Pow(p, 1.25)
	}
	if m.endingTicks > 0 && m.endingTotal > 0 {
		p := phaseProgress(m.endingTotal, m.endingTicks)
		level *= math.Pow(1-p, 1.55)
	}
	if m.quietTicks > 0 {
		level *= m.cfg.QuietMult
	}
	return clamp01(level)
}

func (m *MagicPortal) breathLocked() float64 {
	period := max(1, m.cfg.PulsePeriod)
	phase := float64(m.tick%period) / float64(period)
	return 0.5 - 0.5*math.Cos(phase*math.Pi*2)
}

func (m *MagicPortal) eventPulseLocked() float64 {
	pulse := m.cfg.PulseAmp * m.breathLocked()
	if m.pulseTicks > 0 {
		pulse += 0.62 * eventEnvelope(m.pulseTotal, m.pulseTicks)
	}
	if m.surgeTicks > 0 {
		pulse += math.Max(0, m.cfg.SurgeMult-1) * eventEnvelope(m.surgeTotal, m.surgeTicks)
	}
	if m.endingTicks > 0 && m.endingSurge > 0 {
		elapsed := m.endingElapsedLocked()
		if elapsed < m.endingSurge {
			left := m.endingSurge - elapsed
			pulse += math.Max(0, m.cfg.FinalSurgeMult-1) * eventEnvelope(m.endingSurge, left)
		}
	}
	if m.quietTicks > 0 {
		pulse *= 0.42 + 0.58*m.cfg.QuietMult
	}
	return math.Max(0, pulse)
}

func (m *MagicPortal) currentRadiusLocked() float64 {
	return m.baseRadiusLocked() + m.cfg.PulseExpand*m.eventPulseLocked()*0.55
}

func (m *MagicPortal) baseRadiusLocked() float64 {
	return math.Max(3, float64(min(m.W, m.H))*m.cfg.RingRadius)
}

func (m *MagicPortal) centerLocked() (float64, float64) {
	return float64(m.W-1) * 0.5, float64(m.H-1) * 0.52
}

func (m *MagicPortal) isPulsePeakLocked() bool {
	period := max(1, m.cfg.PulsePeriod)
	return m.tick%period == period/2
}

func (m *MagicPortal) endingElapsedLocked() int {
	if m.endingTotal <= 0 {
		return 0
	}
	return max(0, m.endingTotal-m.endingTicks)
}

func eventEnvelope(total, left int) float64 {
	if total <= 1 || left <= 0 {
		return 0
	}
	elapsed := total - left
	if elapsed < 0 {
		elapsed = 0
	}
	p := clamp01(float64(elapsed) / float64(total-1))
	return math.Sin(math.Pi * p)
}

func makePixelGrid(w, h int) [][]Pixel {
	if h < 0 {
		h = 0
	}
	if w < 0 {
		w = 0
	}
	grid := make([][]Pixel, h)
	for i := range grid {
		grid[i] = make([]Pixel, w)
	}
	return grid
}
