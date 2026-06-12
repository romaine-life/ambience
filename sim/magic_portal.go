package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

const (
	MagicPortalEventPulse      = "pulse"
	MagicPortalEventPowerSurge = "power-surge"
	MagicPortalEventEmberBurst = "ember-burst"
	MagicPortalEventRuneShift  = "rune-shift"
	MagicPortalEventQuietGate  = "quiet-gate"
	MagicPortalEventIntro      = "intro"
	MagicPortalEventEnding     = "ending"
)

// MagicPortalConfig tunes the magic-portal effect used in isolated dev
// sessions. Hue and pulse period are the main scene-level levers.
type MagicPortalConfig struct {
	// INTRODUCTION
	IntroResolve int `json:"intro_resolve"`
	IntroFull    int `json:"intro_full"`
	// ENDING
	EndingDur       int     `json:"ending_dur"`
	FinalSurgeDur   int     `json:"final_surge_dur"`
	FinalSurgeInt   float64 `json:"final_surge_int"`
	EndingEmberTail int     `json:"ember_tail"`
	// LEVERS - silhouette / pulse
	RingScale   float64 `json:"ring_scale"`
	RingWidth   float64 `json:"ring_width"`
	RuneCount   int     `json:"rune_count"`
	PulsePeriod int     `json:"pulse_period"`
	PulseAmp    float64 `json:"pulse_amp"`
	Glow        float64 `json:"glow"`
	EmberRate   int     `json:"ember_rate"`
	// LEVERS - color
	Hue      float64 `json:"hue"`
	Sat      float64 `json:"sat"`
	LightMin float64 `json:"lmin"`
	LightMax float64 `json:"lmax"`
	// EVENT CHANCES
	SurgeChance      float64 `json:"surge_p"`
	EmberBurstChance float64 `json:"burst_p"`
	RuneShiftChance  float64 `json:"shift_p"`
	QuietChance      float64 `json:"quiet_p"`
	// EVENT MODIFIERS
	PulsePeakDur int     `json:"pulse_peak_dur"`
	SurgeDur     int     `json:"surge_dur"`
	SurgeMult    float64 `json:"surge_mult"`
	BurstEmbers  int     `json:"burst_embers"`
	RuneShiftDur int     `json:"rune_shift_dur"`
	QuietDur     int     `json:"quiet_dur"`
	QuietMult    float64 `json:"quiet_mult"`
	EmberLife    int     `json:"ember_life"`
}

func (c MagicPortalConfig) withDefaults() MagicPortalConfig {
	if c.IntroResolve <= 0 {
		c.IntroResolve = 70
	}
	if c.IntroFull <= 0 {
		c.IntroFull = 180
	}
	if c.EndingDur == 0 && c.FinalSurgeDur == 0 && c.FinalSurgeInt == 0 && c.EndingEmberTail == 0 {
		c.EndingDur = 115
		c.FinalSurgeDur = 42
		c.FinalSurgeInt = 1.8
		c.EndingEmberTail = 70
	} else {
		if c.EndingDur <= 0 {
			c.EndingDur = 115
		}
		if c.FinalSurgeDur <= 0 {
			c.FinalSurgeDur = 42
		}
		if c.FinalSurgeInt <= 0 {
			c.FinalSurgeInt = 1.8
		}
		if c.EndingEmberTail < 0 {
			c.EndingEmberTail = 0
		}
	}
	if c.RingScale <= 0 {
		c.RingScale = 1
	}
	if c.RingScale > 1.6 {
		c.RingScale = 1.6
	}
	if c.RingWidth <= 0 {
		c.RingWidth = 2.4
	}
	if c.RingWidth > 8 {
		c.RingWidth = 8
	}
	if c.RuneCount <= 0 {
		c.RuneCount = 14
	}
	if c.RuneCount > 32 {
		c.RuneCount = 32
	}
	if c.PulsePeriod <= 0 {
		c.PulsePeriod = 210
	}
	if c.PulsePeriod < 60 {
		c.PulsePeriod = 60
	}
	if c.PulseAmp <= 0 {
		c.PulseAmp = 0.72
	}
	if c.PulseAmp > 1.4 {
		c.PulseAmp = 1.4
	}
	if c.Glow <= 0 {
		c.Glow = 0.74
	}
	if c.Glow > 1.4 {
		c.Glow = 1.4
	}
	if c.EmberRate <= 0 {
		c.EmberRate = 3
	}
	if c.EmberRate > 24 {
		c.EmberRate = 24
	}
	if c.Hue == 0 && c.Sat == 0 && c.LightMin == 0 && c.LightMax == 0 {
		c.Hue = 208
		c.Sat = 0.72
		c.LightMin = 0.12
		c.LightMax = 0.86
	} else {
		c.Sat = clamp01(c.Sat)
		if c.LightMin <= 0 {
			c.LightMin = 0.12
		}
		if c.LightMax <= 0 {
			c.LightMax = 0.86
		}
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.SurgeChance < 0 {
		c.SurgeChance = 0
	}
	if c.EmberBurstChance < 0 {
		c.EmberBurstChance = 0
	}
	if c.RuneShiftChance < 0 {
		c.RuneShiftChance = 0
	}
	if c.QuietChance < 0 {
		c.QuietChance = 0
	}
	if c.PulsePeakDur <= 0 {
		c.PulsePeakDur = 28
	}
	if c.SurgeDur <= 0 {
		c.SurgeDur = 95
	}
	if c.SurgeMult <= 0 {
		c.SurgeMult = 1.65
	}
	if c.BurstEmbers <= 0 {
		c.BurstEmbers = 11
	}
	if c.BurstEmbers > 80 {
		c.BurstEmbers = 80
	}
	if c.RuneShiftDur <= 0 {
		c.RuneShiftDur = 55
	}
	if c.QuietDur <= 0 {
		c.QuietDur = 280
	}
	if c.QuietMult <= 0 || c.QuietMult > 1 {
		c.QuietMult = 0.32
	}
	if c.EmberLife <= 0 {
		c.EmberLife = 95
	}
	if c.EmberLife > 360 {
		c.EmberLife = 360
	}
	return c
}

// MagicPortalSchema describes the magic-portal effect's tunable knobs for the
// dev UI.
func MagicPortalSchema() EffectSchema {
	return EffectSchema{
		Name: "magic-portal",
		// The ending resolves to a held dark gate (gateDark) until an
		// intro relights it — the catalog's terminal outro.
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_event", Label: "intro", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventIntro,
				Description: "Preview the gate resolving from darkness into its steady pulse."},
			{Key: "intro_resolve", Label: "rune resolve", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 70,
				Description: "Ticks spent flickering the runes into a readable gate silhouette."},
			{Key: "intro_full", Label: "first pulse", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 60, Max: 420, Step: 10, Default: 180,
				Description: "Ticks until the first full-amplitude breathing pulse is allowed."},
			{Key: "ending_event", Label: "ending", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventEnding,
				Description: "Run the outro: pulses decay, one surge cracks, then the gate goes dark."},
			{Key: "ending_dur", Label: "outro decay", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 30, Max: 260, Step: 5, Default: 115,
				Description: "Ticks spent fading the gate pulse toward darkness."},
			{Key: "final_surge_dur", Label: "surge dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 120, Step: 5, Default: 42,
				Description: "Duration of the final crackling surge at the start of the outro."},
			{Key: "final_surge_int", Label: "surge power", Slot: SlotEnd, Group: "ending", Type: KnobFloat, Min: 0.6, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Brightness multiplier for the outro's final surge."},
			{Key: "ember_tail", Label: "ember tail", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 220, Step: 5, Default: 70,
				Description: "Extra ticks for embers to fade before the terminal dark state is reached."},
			{Key: "ring_scale", Label: "ring scale", Slot: SlotLever, Group: "silhouette", Type: KnobFloat, Min: 0.65, Max: 1.5, Step: 0.05, Default: 1,
				Description: "Size of the portal ring/arch silhouette."},
			{Key: "ring_width", Label: "ring width", Slot: SlotLever, Group: "silhouette", Type: KnobFloat, Min: 1, Max: 6, Step: 0.1, Default: 2.4,
				Description: "Thickness of the bright runic ring in pixels."},
			{Key: "rune_count", Label: "runes", Slot: SlotLever, Group: "silhouette", Type: KnobInt, Min: 6, Max: 28, Step: 1, Default: 14,
				Description: "Number of readable runic marks around the gate."},
			{Key: "pulse_period", Label: "pulse period", Slot: SlotLever, Group: "pulse", Type: KnobInt, Min: 90, Max: 420, Step: 5, Default: 210,
				Description: "Ticks per breathing cycle. Higher values make the portal pulse more slowly."},
			{Key: "pulse_amp", Label: "pulse amp", Slot: SlotLever, Group: "pulse", Type: KnobFloat, Min: 0.2, Max: 1.3, Step: 0.05, Default: 0.72,
				Description: "How strongly the central glow expands and contracts."},
			{Key: "glow", Label: "glow", Slot: SlotLever, Group: "pulse", Type: KnobFloat, Min: 0.15, Max: 1.3, Step: 0.05, Default: 0.74,
				Description: "Baseline portal-core glow strength."},
			{Key: "ember_rate", Label: "pulse embers", Slot: SlotLever, Group: "embers", Type: KnobInt, Min: 1, Max: 18, Step: 1, Default: 3,
				Description: "Approximate embers emitted from the ring at each automatic pulse peak."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 359, Step: 1, Default: 208,
				Description: "Scene hue: blue for arcane, red for infernal, amber for ancient, low saturation for dormant."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.02, Default: 0.72,
				Description: "Portal color saturation. Lower values produce a dormant gray relic gate."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.03, Max: 0.35, Step: 0.01, Default: 0.12,
				Description: "Minimum lightness for the gate silhouette and dark background."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.35, Max: 1, Step: 0.01, Default: 0.86,
				Description: "Maximum lightness used by rune peaks, surges, and embers."},
			{Key: "pulse_event", Label: "pulse", Slot: SlotEvent, Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventPulse,
				Description: "Fire one immediate brightness peak and a few embers."},
			{Key: "surge_event", Label: "power surge", Slot: SlotEvent, Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventPowerSurge,
				Description: "Fire a brighter, longer pulse with denser embers."},
			{Key: "burst_event", Label: "ember burst", Slot: SlotEvent, Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventEmberBurst,
				Description: "Emit a compact ember cluster from the ring."},
			{Key: "shift_event", Label: "rune shift", Slot: SlotEvent, Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventRuneShift,
				Description: "Briefly shift one rune out of alignment."},
			{Key: "quiet_event", Label: "quiet gate", Slot: SlotEvent, Type: KnobInt, Min: 0, Max: 1, Step: 1, Default: 0, Trigger: MagicPortalEventQuietGate,
				Description: "Dim the gate into a long suppressed breathing window."},
			{Key: "surge_p", Label: "surge chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.0001, Default: 0.0002,
				Description: "Per-tick chance of a power surge while the gate is active."},
			{Key: "burst_p", Label: "burst chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.001,
				Description: "Per-tick chance of a small ember burst."},
			{Key: "shift_p", Label: "shift chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.01, Step: 0.0002, Default: 0.0008,
				Description: "Per-tick chance that a rune shifts briefly."},
			{Key: "quiet_p", Label: "quiet chance", Slot: SlotEvent, Group: "chance", Type: KnobFloat, Min: 0, Max: 0.003, Step: 0.0001, Default: 0.00015,
				Description: "Per-tick chance of a long quiet-gate suppression window."},
			{Key: "pulse_peak_dur", Label: "pulse peak", Slot: SlotEventMod, Group: "pulse", Type: KnobInt, Min: 8, Max: 70, Step: 2, Default: 28,
				Description: "Envelope duration for a manually fired pulse peak."},
			{Key: "surge_dur", Label: "surge dur", Slot: SlotEventMod, Group: "surge", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 95,
				Description: "Typical power-surge duration in ticks."},
			{Key: "surge_mult", Label: "surge x", Slot: SlotEventMod, Group: "surge", Type: KnobFloat, Min: 1, Max: 3, Step: 0.05, Default: 1.65,
				Description: "Brightness multiplier applied during a power surge."},
			{Key: "burst_embers", Label: "burst count", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 2, Max: 48, Step: 1, Default: 11,
				Description: "Approximate ember count emitted by ember-burst and surge events."},
			{Key: "rune_shift_dur", Label: "shift dur", Slot: SlotEventMod, Group: "runes", Type: KnobInt, Min: 8, Max: 140, Step: 2, Default: 55,
				Description: "Ticks a shifted rune takes to settle back into the ring."},
			{Key: "quiet_dur", Label: "quiet dur", Slot: SlotEventMod, Group: "quiet", Type: KnobInt, Min: 40, Max: 720, Step: 10, Default: 280,
				Description: "Duration of quiet-gate suppression."},
			{Key: "quiet_mult", Label: "quiet x", Slot: SlotEventMod, Group: "quiet", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.05, Default: 0.32,
				Description: "Brightness and event-probability multiplier while quiet-gate is active."},
			{Key: "ember_life", Label: "ember life", Slot: SlotEventMod, Group: "embers", Type: KnobInt, Min: 20, Max: 220, Step: 5, Default: 95,
				Description: "Typical ember lifetime before it fades out."},
		},
	}
}

// MagicPortalEmberSnap is one ember persisted in snapshots.
type MagicPortalEmberSnap struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	VX        float64 `json:"vx"`
	VY        float64 `json:"vy"`
	Age       int     `json:"age"`
	Life      int     `json:"life"`
	HueOffset float64 `json:"h"`
}

// MagicPortalState is the wire/persisted snapshot for magic-portal.
type MagicPortalState struct {
	Tick            int                    `json:"tick"`
	IntroTicks      int                    `json:"introTicks"`
	IntroTotal      int                    `json:"introTotal"`
	EndingTicks     int                    `json:"endingTicks"`
	EndingTotal     int                    `json:"endingTotal"`
	FinalSurgeTicks int                    `json:"finalSurgeTicks"`
	FinalSurgeTotal int                    `json:"finalSurgeTotal"`
	SurgeTicks      int                    `json:"surgeTicks"`
	SurgeTotal      int                    `json:"surgeTotal"`
	PulseTicks      int                    `json:"pulseTicks"`
	PulseTotal      int                    `json:"pulseTotal"`
	QuietTicks      int                    `json:"quietTicks"`
	RuneShiftTicks  int                    `json:"runeShiftTicks"`
	RuneShiftTotal  int                    `json:"runeShiftTotal"`
	ShiftedRune     int                    `json:"shiftedRune"`
	LastPulseBeat   int                    `json:"lastPulseBeat"`
	GateDark        bool                   `json:"gateDark"`
	Lifecycle       Lifecycle              `json:"lifecycle"`
	Embers          []MagicPortalEmberSnap `json:"embers,omitempty"`
	RNGState        uint64                 `json:"rngState,omitempty"`
}

type MagicPortalSnapshot struct {
	MagicPortalState
}

type MagicPortalPersistedState struct {
	MagicPortalState
}

type magicPortalEmber struct {
	x         float64
	y         float64
	vx        float64
	vy        float64
	age       int
	life      int
	hueOffset float64
}

// MagicPortal is an authoritative pixel-grid sim for a pulsing runic gate.
type MagicPortal struct {
	mu sync.Mutex

	W, H int
	rng  *rngutil.RNG
	cfg  MagicPortalConfig
	tick int

	introTicks      int
	introTotal      int
	endingTicks     int
	endingTotal     int
	finalSurgeTicks int
	finalSurgeTotal int
	surgeTicks      int
	surgeTotal      int
	pulseTicks      int
	pulseTotal      int
	quietTicks      int
	runeShiftTicks  int
	runeShiftTotal  int
	shiftedRune     int
	lastPulseBeat   int
	gateDark        bool

	embers []magicPortalEmber
	log    []LogEntry
}

func NewMagicPortal(w, h int, seed int64, cfg MagicPortalConfig) *MagicPortal {
	return &MagicPortal{
		W:             w,
		H:             h,
		rng:           rngutil.New(seed),
		cfg:           cfg.withDefaults(),
		shiftedRune:   -1,
		lastPulseBeat: -1,
	}
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
	scaleX := float64(w) / math.Max(1, float64(m.W))
	scaleY := float64(h) / math.Max(1, float64(m.H))
	m.W = w
	m.H = h
	for i := range m.embers {
		m.embers[i].x *= scaleX
		m.embers[i].y *= scaleY
		m.embers[i].vx *= scaleX
		m.embers[i].vy *= scaleY
	}
}

func (m *MagicPortal) SetConfig(cfg MagicPortalConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg.withDefaults()
	if m.cfg.RuneCount > 0 && m.shiftedRune >= m.cfg.RuneCount {
		m.shiftedRune = m.cfg.RuneCount - 1
	}
}

func (m *MagicPortal) EffectiveConfig() MagicPortalConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
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

func (m *MagicPortal) appendLog(kind, desc string) {
	m.log = append(m.log, LogEntry{Tick: m.tick, Type: kind, Desc: desc})
	if len(m.log) > 200 {
		m.log = m.log[len(m.log)-200:]
	}
}

func (m *MagicPortal) Snapshot() MagicPortalSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MagicPortalSnapshot{MagicPortalState: m.snapshotStateLocked()}
}

func (m *MagicPortal) RestoreSnapshot(s MagicPortalSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(s.MagicPortalState)
}

func (m *MagicPortal) SnapshotPersistedState() MagicPortalPersistedState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MagicPortalPersistedState{MagicPortalState: m.snapshotStateLocked()}
}

func (m *MagicPortal) RestorePersistedState(s MagicPortalPersistedState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreStateLocked(s.MagicPortalState)
}

func (m *MagicPortal) snapshotStateLocked() MagicPortalState {
	out := MagicPortalState{
		Tick:            m.tick,
		IntroTicks:      m.introTicks,
		IntroTotal:      m.introTotal,
		EndingTicks:     m.endingTicks,
		EndingTotal:     m.endingTotal,
		FinalSurgeTicks: m.finalSurgeTicks,
		FinalSurgeTotal: m.finalSurgeTotal,
		SurgeTicks:      m.surgeTicks,
		SurgeTotal:      m.surgeTotal,
		PulseTicks:      m.pulseTicks,
		PulseTotal:      m.pulseTotal,
		QuietTicks:      m.quietTicks,
		RuneShiftTicks:  m.runeShiftTicks,
		RuneShiftTotal:  m.runeShiftTotal,
		ShiftedRune:     m.shiftedRune,
		LastPulseBeat:   m.lastPulseBeat,
		GateDark:        m.gateDark,
		Lifecycle:       m.lifecycleLocked(),
		RNGState:        m.rng.State(),
	}
	if len(m.embers) > 0 {
		out.Embers = make([]MagicPortalEmberSnap, len(m.embers))
		for i, e := range m.embers {
			out.Embers[i] = MagicPortalEmberSnap{
				X:         e.x,
				Y:         e.y,
				VX:        e.vx,
				VY:        e.vy,
				Age:       e.age,
				Life:      e.life,
				HueOffset: e.hueOffset,
			}
		}
	}
	return out
}

func (m *MagicPortal) restoreStateLocked(s MagicPortalState) {
	m.tick = s.Tick
	m.introTicks = s.IntroTicks
	m.introTotal = s.IntroTotal
	m.endingTicks = s.EndingTicks
	m.endingTotal = s.EndingTotal
	m.finalSurgeTicks = s.FinalSurgeTicks
	m.finalSurgeTotal = s.FinalSurgeTotal
	m.surgeTicks = s.SurgeTicks
	m.surgeTotal = s.SurgeTotal
	m.pulseTicks = s.PulseTicks
	m.pulseTotal = s.PulseTotal
	m.quietTicks = s.QuietTicks
	m.runeShiftTicks = s.RuneShiftTicks
	m.runeShiftTotal = s.RuneShiftTotal
	m.shiftedRune = s.ShiftedRune
	m.lastPulseBeat = s.LastPulseBeat
	m.gateDark = s.GateDark
	if s.RNGState != 0 {
		m.rng.SetState(s.RNGState)
	}
	m.embers = m.embers[:0]
	for _, e := range s.Embers {
		m.embers = append(m.embers, magicPortalEmber{
			x:         e.X,
			y:         e.Y,
			vx:        e.VX,
			vy:        e.VY,
			age:       e.Age,
			life:      e.Life,
			hueOffset: e.HueOffset,
		})
	}
}

// TriggerEvent fires a named portal event. Intro reactivates the gate; ending
// is terminal and leaves gateDark=true after the outro completes.
func (m *MagicPortal) TriggerEvent(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch name {
	case MagicPortalEventPulse:
		m.startPulseLocked("triggered")
	case MagicPortalEventPowerSurge:
		m.startPowerSurgeLocked("triggered")
	case MagicPortalEventEmberBurst:
		m.startEmberBurstLocked("triggered")
	case MagicPortalEventRuneShift:
		m.startRuneShiftLocked("triggered")
	case MagicPortalEventQuietGate:
		m.startQuietLocked("triggered")
	case MagicPortalEventIntro:
		m.startIntroLocked()
		m.appendLog(MagicPortalEventIntro, fmt.Sprintf("started (resolve=%d, full=%d)", m.cfg.IntroResolve, m.cfg.IntroFull))
	case MagicPortalEventEnding:
		m.startEndingLocked()
		m.appendLog(MagicPortalEventEnding, fmt.Sprintf("started (decay=%d, tail=%d)", m.cfg.EndingDur, m.cfg.EndingEmberTail))
	default:
		return false
	}
	return true
}

func (m *MagicPortal) Step() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tick++
	m.decTimersLocked()
	m.stepEmbersLocked()

	if m.endingTicks == 0 && m.endingTotal > 0 {
		m.finishEndingLocked()
		return
	}
	if m.gateDark {
		return
	}
	if m.endingTicks > 0 {
		return
	}
	if m.introTicks > 0 {
		return
	}

	period := max(1, m.cfg.PulsePeriod)
	beat := m.tick / period
	if m.tick > 0 && m.tick%period == 0 && beat != m.lastPulseBeat {
		m.lastPulseBeat = beat
		m.startPulseLocked("steady")
	}

	chanceMult := 1.0
	if m.quietTicks > 0 {
		chanceMult = m.cfg.QuietMult
	}
	if m.surgeTicks <= 0 && m.cfg.SurgeChance > 0 && m.rng.Float64() < m.cfg.SurgeChance*chanceMult {
		m.startPowerSurgeLocked("started")
	}
	if m.cfg.EmberBurstChance > 0 && m.rng.Float64() < m.cfg.EmberBurstChance*chanceMult {
		m.startEmberBurstLocked("started")
	}
	if m.runeShiftTicks <= 0 && m.cfg.RuneShiftChance > 0 && m.rng.Float64() < m.cfg.RuneShiftChance*chanceMult {
		m.startRuneShiftLocked("started")
	}
	if m.quietTicks <= 0 && m.cfg.QuietChance > 0 && m.rng.Float64() < m.cfg.QuietChance {
		m.startQuietLocked("started")
	}
}

func (m *MagicPortal) decTimersLocked() {
	dec := func(v *int) {
		if *v > 0 {
			*v--
		}
	}
	dec(&m.introTicks)
	dec(&m.endingTicks)
	dec(&m.finalSurgeTicks)
	dec(&m.surgeTicks)
	dec(&m.pulseTicks)
	dec(&m.quietTicks)
	dec(&m.runeShiftTicks)
	if m.runeShiftTicks == 0 {
		m.shiftedRune = -1
	}
	if m.pulseTicks == 0 {
		m.pulseTotal = 0
	}
	if m.surgeTicks == 0 {
		m.surgeTotal = 0
	}
	if m.finalSurgeTicks == 0 {
		m.finalSurgeTotal = 0
	}
	if m.introTicks == 0 {
		m.introTotal = 0
	}
}

func (m *MagicPortal) stepEmbersLocked() {
	if len(m.embers) == 0 {
		return
	}
	dst := m.embers[:0]
	for _, e := range m.embers {
		e.age++
		if e.life <= 0 || e.age >= e.life {
			continue
		}
		e.x += e.vx
		e.y += e.vy
		e.vx *= 1.006
		e.vy *= 1.006
		dst = append(dst, e)
	}
	m.embers = dst
}

func (m *MagicPortal) startPulseLocked(verb string) {
	dur := jitterInt(m.rng, m.cfg.PulsePeakDur, 0.18)
	m.pulseTicks = dur
	m.pulseTotal = dur
	count := m.cfg.EmberRate
	if m.rng.Float64() < 0.35 {
		count++
	}
	m.spawnEmbersLocked(count, 1)
	m.appendLog(MagicPortalEventPulse, fmt.Sprintf("%s (embers=%d)", verb, count))
}

func (m *MagicPortal) startPowerSurgeLocked(verb string) {
	dur := jitterInt(m.rng, m.cfg.SurgeDur, 0.25)
	m.surgeTicks = dur
	m.surgeTotal = dur
	pulseDur := max(m.cfg.PulsePeakDur, dur/2)
	m.pulseTicks = pulseDur
	m.pulseTotal = pulseDur
	count := max(m.cfg.BurstEmbers, m.cfg.EmberRate*3)
	m.spawnEmbersLocked(count, 1.55)
	m.appendLog(MagicPortalEventPowerSurge, fmt.Sprintf("%s (dur=%d, embers=%d)", verb, dur, count))
}

func (m *MagicPortal) startEmberBurstLocked(verb string) {
	count := jitterInt(m.rng, m.cfg.BurstEmbers, 0.35)
	m.spawnEmbersLocked(count, 1.25)
	m.appendLog(MagicPortalEventEmberBurst, fmt.Sprintf("%s (embers=%d)", verb, count))
}

func (m *MagicPortal) startRuneShiftLocked(verb string) {
	dur := jitterInt(m.rng, m.cfg.RuneShiftDur, 0.25)
	m.runeShiftTicks = dur
	m.runeShiftTotal = dur
	if m.cfg.RuneCount > 0 {
		m.shiftedRune = m.rng.Intn(m.cfg.RuneCount)
	} else {
		m.shiftedRune = -1
	}
	m.appendLog(MagicPortalEventRuneShift, fmt.Sprintf("%s (rune=%d, dur=%d)", verb, m.shiftedRune, dur))
}

func (m *MagicPortal) startQuietLocked(verb string) {
	dur := jitterInt(m.rng, m.cfg.QuietDur, 0.25)
	m.quietTicks = dur
	m.appendLog(MagicPortalEventQuietGate, fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, m.cfg.QuietMult))
}

// lifecycleLocked derives the effect-generic lifecycle contract value.
// Magic-portal's outro is terminal: finishEndingLocked sets gateDark and the
// gate holds its dark terminal look until an intro trigger relights it, so
// the schema declares ending_terminal: true.
func (m *MagicPortal) lifecycleLocked() Lifecycle {
	switch {
	case m.introTicks > 0:
		return LifecycleIntro
	case m.endingTicks > 0:
		return LifecycleEnding
	case m.gateDark:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (m *MagicPortal) startIntroLocked() {
	total := max(m.cfg.IntroResolve, m.cfg.IntroFull)
	if total < 1 {
		total = 1
	}
	m.gateDark = false
	m.introTicks = total
	m.introTotal = total
	m.endingTicks = 0
	m.endingTotal = 0
	m.finalSurgeTicks = 0
	m.finalSurgeTotal = 0
	m.surgeTicks = 0
	m.surgeTotal = 0
	m.quietTicks = 0
	m.pulseTicks = 0
	m.pulseTotal = 0
	m.embers = m.embers[:0]
	m.lastPulseBeat = m.tick / max(1, m.cfg.PulsePeriod)
}

func (m *MagicPortal) startEndingLocked() {
	m.gateDark = false
	m.introTicks = 0
	m.introTotal = 0
	m.surgeTicks = 0
	m.surgeTotal = 0
	m.quietTicks = 0
	m.runeShiftTicks = 0
	m.runeShiftTotal = 0
	m.shiftedRune = -1
	total := max(1, m.cfg.EndingDur+max(0, m.cfg.EndingEmberTail))
	m.endingTicks = total
	m.endingTotal = total
	dur := max(1, m.cfg.FinalSurgeDur)
	m.finalSurgeTicks = dur
	m.finalSurgeTotal = dur
	m.spawnEmbersLocked(max(m.cfg.BurstEmbers*2, m.cfg.EmberRate*4), 1.8)
}

func (m *MagicPortal) finishEndingLocked() {
	m.gateDark = true
	m.endingTotal = 0
	m.finalSurgeTicks = 0
	m.finalSurgeTotal = 0
	m.surgeTicks = 0
	m.surgeTotal = 0
	m.pulseTicks = 0
	m.pulseTotal = 0
	m.quietTicks = 0
	m.runeShiftTicks = 0
	m.runeShiftTotal = 0
	m.shiftedRune = -1
	m.embers = m.embers[:0]
	m.appendLog(MagicPortalEventEnding, "complete (gateDark=true)")
}

func (m *MagicPortal) spawnEmbersLocked(count int, speedBoost float64) {
	if count <= 0 || m.W <= 0 || m.H <= 0 {
		return
	}
	cx, cy, rx, ry := m.portalGeometryLocked(1)
	for i := 0; i < count; i++ {
		angle := m.rng.Float64() * math.Pi * 2
		rimX := cx + math.Cos(angle)*rx
		rimY := cy + math.Sin(angle)*ry
		speed := (0.18 + m.rng.Float64()*0.34) * speedBoost
		life := jitterInt(m.rng, m.cfg.EmberLife, 0.35)
		m.embers = append(m.embers, magicPortalEmber{
			x:         rimX,
			y:         rimY,
			vx:        math.Cos(angle)*speed + (m.rng.Float64()*2-1)*0.05,
			vy:        math.Sin(angle)*speed + (m.rng.Float64()*2-1)*0.05,
			life:      life,
			hueOffset: (m.rng.Float64()*2 - 1) * 18,
		})
	}
	const maxEmbers = 160
	if len(m.embers) > maxEmbers {
		m.embers = m.embers[len(m.embers)-maxEmbers:]
	}
}

func (m *MagicPortal) portalGeometryLocked(scale float64) (cx, cy, rx, ry float64) {
	minDim := math.Min(float64(m.W), float64(m.H))
	base := math.Min(float64(m.W)*0.22, float64(m.H)*0.35)
	if base < minDim*0.18 {
		base = minDim * 0.18
	}
	if base < 3 {
		base = 3
	}
	rx = base * m.cfg.RingScale * scale
	ry = base * 1.2 * m.cfg.RingScale * scale
	cx = float64(m.W-1) * 0.5
	cy = float64(m.H-1) * 0.53
	if bottom := cy + ry; bottom > float64(m.H)-3 {
		cy -= bottom - (float64(m.H) - 3)
	}
	if cy-ry < 2 {
		cy = ry + 2
	}
	return cx, cy, rx, ry
}

func (m *MagicPortal) GridCopy() [][]Pixel {
	m.mu.Lock()
	defer m.mu.Unlock()

	grid := make([][]Pixel, m.H)
	for y := range grid {
		grid[y] = make([]Pixel, m.W)
	}
	if m.W <= 0 || m.H <= 0 {
		return grid
	}

	m.paintBackgroundLocked(grid)
	if m.gateDark {
		return grid
	}

	level, runeLevel := m.lifecycleLevelsLocked()
	if level <= 0 {
		return grid
	}
	wave := m.pulseWaveLocked()
	pulseEnv := magicPortalEnvelope(m.pulseTotal, m.pulseTicks)
	surgeEnv := magicPortalEnvelope(m.surgeTotal, m.surgeTicks)
	finalEnv := magicPortalEnvelope(m.finalSurgeTotal, m.finalSurgeTicks) * m.cfg.FinalSurgeInt
	if m.quietTicks > 0 {
		level *= m.cfg.QuietMult
		wave *= m.cfg.QuietMult
		pulseEnv *= m.cfg.QuietMult
	}
	brightness := clamp01(0.24 + m.cfg.PulseAmp*0.52*wave + 0.28*pulseEnv + 0.24*m.cfg.SurgeMult*surgeEnv + 0.24*finalEnv)
	scale := 1 + level*(0.035*wave+0.04*surgeEnv+0.045*finalEnv)
	cx, cy, rx, ry := m.portalGeometryLocked(scale)

	m.paintCoreLocked(grid, cx, cy, rx, ry, level, brightness)
	m.paintRingLocked(grid, cx, cy, rx, ry, level, brightness)
	m.paintRunesLocked(grid, cx, cy, rx, ry, level*runeLevel, brightness)
	m.paintEmbersLocked(grid, level)
	return grid
}

func (m *MagicPortal) lifecycleLevelsLocked() (level, runeLevel float64) {
	level = 1
	runeLevel = 1
	if m.introTicks > 0 && m.introTotal > 0 {
		elapsed := m.introTotal - m.introTicks
		runeLevel = clamp01(float64(elapsed) / math.Max(1, float64(m.cfg.IntroResolve)))
		pulseLevel := clamp01(float64(elapsed) / math.Max(1, float64(m.cfg.IntroFull)))
		level *= 0.08 + 0.92*pulseLevel
	}
	if m.endingTicks > 0 && m.endingTotal > 0 {
		elapsed := m.endingTotal - m.endingTicks
		fade := 1 - clamp01(float64(elapsed)/math.Max(1, float64(m.cfg.EndingDur)))
		level *= fade
		runeLevel *= fade
	}
	return clamp01(level), clamp01(runeLevel)
}

func (m *MagicPortal) pulseWaveLocked() float64 {
	period := math.Max(1, float64(m.cfg.PulsePeriod))
	phase := math.Mod(float64(m.tick), period) / period
	return 0.5 + 0.5*math.Cos(phase*math.Pi*2)
}

func magicPortalEnvelope(total, left int) float64 {
	if total <= 0 || left <= 0 {
		return 0
	}
	p := phaseProgress(total, left)
	return math.Sin(math.Pi * p)
}

func (m *MagicPortal) paintBackgroundLocked(grid [][]Pixel) {
	hue := math.Mod(m.cfg.Hue+236, 360)
	sat := clamp01(m.cfg.Sat * 0.12)
	for y := 0; y < m.H; y++ {
		t := float64(y) / math.Max(1, float64(m.H-1))
		light := 0.012 + 0.024*(1-t)
		if !m.gateDark {
			light += 0.012 * (1 - math.Abs(t-0.55))
		}
		c := hslToRGB(hue, sat, clamp01(light))
		for x := 0; x < m.W; x++ {
			grid[y][x] = Pixel{Filled: true, C: c}
		}
	}
}

func (m *MagicPortal) paintCoreLocked(grid [][]Pixel, cx, cy, rx, ry, level, brightness float64) {
	hue := math.Mod(m.cfg.Hue+360, 360)
	coreRX := rx * (0.62 + 0.16*brightness)
	coreRY := ry * (0.72 + 0.16*brightness)
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			dx := (float64(x) - cx) / math.Max(1, coreRX)
			dy := (float64(y) - cy) / math.Max(1, coreRY)
			d2 := dx*dx + dy*dy
			if d2 > 1.35 {
				continue
			}
			alpha := math.Exp(-d2*2.35) * m.cfg.Glow * level * (0.42 + 0.65*brightness)
			light := clamp01(m.cfg.LightMin + (m.cfg.LightMax-m.cfg.LightMin)*(0.28+0.55*brightness))
			c := hslToRGB(hue, clamp01(m.cfg.Sat*0.82), light)
			portalBlendPixel(grid, x, y, c, alpha)
		}
	}
}

func (m *MagicPortal) paintRingLocked(grid [][]Pixel, cx, cy, rx, ry, level, brightness float64) {
	hue := math.Mod(m.cfg.Hue+360, 360)
	width := math.Max(1, m.cfg.RingWidth)
	minR := math.Max(1, math.Min(rx, ry))
	outerGlow := width * 2.2
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			dx := (float64(x) - cx) / math.Max(1, rx)
			dy := (float64(y) - cy) / math.Max(1, ry)
			dist := math.Sqrt(dx*dx + dy*dy)
			bandPx := math.Abs(dist-1) * minR
			if bandPx > outerGlow {
				continue
			}
			alpha := 0.0
			if bandPx <= width {
				alpha = (1 - bandPx/width) * level * (0.55 + 0.65*brightness)
			} else {
				alpha = (1 - (bandPx-width)/(outerGlow-width)) * level * brightness * 0.22
			}
			light := clamp01(m.cfg.LightMin + (m.cfg.LightMax-m.cfg.LightMin)*(0.42+0.48*brightness))
			if bandPx > width {
				light *= 0.62
			}
			c := hslToRGB(hue, clamp01(m.cfg.Sat), light)
			portalBlendPixel(grid, x, y, c, alpha)
		}
	}

	baseY := int(math.Round(cy + ry*0.92))
	baseC := hslToRGB(hue, clamp01(m.cfg.Sat*0.72), clamp01(m.cfg.LightMin+(m.cfg.LightMax-m.cfg.LightMin)*0.24*level))
	for dx := int(math.Round(-rx * 0.8)); dx <= int(math.Round(rx*0.8)); dx++ {
		portalBlendPixel(grid, int(math.Round(cx))+dx, baseY, baseC, 0.55*level)
	}
}

func (m *MagicPortal) paintRunesLocked(grid [][]Pixel, cx, cy, rx, ry, level, brightness float64) {
	if level <= 0 || m.cfg.RuneCount <= 0 {
		return
	}
	hue := math.Mod(m.cfg.Hue+360, 360)
	shiftEnv := magicPortalEnvelope(m.runeShiftTotal, m.runeShiftTicks)
	mark := int(math.Max(1, math.Round(math.Min(rx, ry)/22)))
	for i := 0; i < m.cfg.RuneCount; i++ {
		angle := (float64(i) / float64(m.cfg.RuneCount)) * math.Pi * 2
		out := 1.11
		alpha := level * (0.45 + 0.65*brightness)
		if i == m.shiftedRune && shiftEnv > 0 {
			out += 0.1 * shiftEnv
			alpha *= 1.4
			angle += 0.08 * shiftEnv
		}
		x := int(math.Round(cx + math.Cos(angle)*rx*out))
		y := int(math.Round(cy + math.Sin(angle)*ry*out))
		light := clamp01(m.cfg.LightMin + (m.cfg.LightMax-m.cfg.LightMin)*(0.56+0.38*brightness))
		if i == m.shiftedRune && shiftEnv > 0 {
			light = clamp01(m.cfg.LightMax)
		}
		c := hslToRGB(hue, clamp01(m.cfg.Sat*0.9), light)
		switch i % 4 {
		case 0:
			for d := -mark; d <= mark; d++ {
				portalBlendPixel(grid, x+d, y, c, alpha)
			}
		case 1:
			for d := -mark; d <= mark; d++ {
				portalBlendPixel(grid, x, y+d, c, alpha)
			}
		case 2:
			for d := -mark; d <= mark; d++ {
				portalBlendPixel(grid, x+d, y+d, c, alpha)
			}
		default:
			portalBlendPixel(grid, x, y, c, alpha)
			portalBlendPixel(grid, x+1, y, c, alpha*0.75)
			portalBlendPixel(grid, x-1, y, c, alpha*0.75)
		}
	}
}

func (m *MagicPortal) paintEmbersLocked(grid [][]Pixel, level float64) {
	if len(m.embers) == 0 {
		return
	}
	baseHue := math.Mod(m.cfg.Hue+24+360, 360)
	for _, e := range m.embers {
		if e.life <= 0 {
			continue
		}
		fade := clamp01(1 - float64(e.age)/float64(e.life))
		if fade <= 0 {
			continue
		}
		hue := math.Mod(baseHue+e.hueOffset+360, 360)
		light := clamp01(m.cfg.LightMin + (m.cfg.LightMax-m.cfg.LightMin)*(0.62+0.36*fade))
		c := hslToRGB(hue, clamp01(m.cfg.Sat*0.85), light)
		x := int(math.Round(e.x))
		y := int(math.Round(e.y))
		portalBlendPixel(grid, x, y, c, fade*level)
		portalBlendPixel(grid, int(math.Round(e.x-e.vx*1.8)), int(math.Round(e.y-e.vy*1.8)), c, fade*level*0.35)
	}
}

func portalBlendPixel(grid [][]Pixel, x, y int, c color.RGBA, alpha float64) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	alpha = clamp01(alpha)
	if alpha <= 0 {
		return
	}
	dst := grid[y][x].C
	inv := 1 - alpha
	grid[y][x] = Pixel{
		Filled: true,
		C: color.RGBA{
			R: uint8(math.Round(float64(dst.R)*inv + float64(c.R)*alpha)),
			G: uint8(math.Round(float64(dst.G)*inv + float64(c.G)*alpha)),
			B: uint8(math.Round(float64(dst.B)*inv + float64(c.B)*alpha)),
			A: 255,
		},
	}
}
