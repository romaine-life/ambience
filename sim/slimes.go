package sim

import (
	"fmt"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// Slimes is a meadow with a row of pixel-art slime blobs sitting on the grass
// line. Individual slimes hop in short ballistic arcs, squash briefly when
// they land, and otherwise drift gently back toward their resting column.
//
// The server is authoritative about *which* slime hops, *when*, and *how
// hard*. Clients run a local replica using the same per-slime state so hops
// stay in time even though the background grass shimmer drifts per client.
//
// Per-slime state is one of:
//   sitting (0)   — resting on the grass, eligible to start a hop
//   hopping (1)   — ballistic arc above the ground, gravity pulling down
//   squashing (2) — landed, flattened, counting down before sitting again

const (
	SlimeStateSitting byte = iota
	SlimeStateHopping
	SlimeStateSquashing
)

// SlimesConfig tunes the slimes-in-a-meadow prototype used in isolated dev
// sessions. See SlimesSchema for the full knob inventory.
type SlimesConfig struct {
	// INTRODUCTION
	IntroDur    int     `json:"intro_dur"`
	IntroGrowth float64 `json:"intro_growth"`
	// ENDING
	EndingDur    int `json:"ending_dur"`
	EndingLinger int `json:"ending_linger"`
	// LEVERS — meadow
	SlimeCount  int     `json:"slime_count"`
	SlimeSize   float64 `json:"slime_size"`
	SizeJitter  float64 `json:"size_jit"`
	Baseline    float64 `json:"baseline"`
	GrassHeight float64 `json:"grass_h"`
	GrassDens   float64 `json:"grass_dens"`
	// LEVERS — physics
	HopPower    float64 `json:"hop_power"`
	HopJitter   float64 `json:"hop_jit"`
	HopForward  float64 `json:"hop_fwd"`
	Gravity     float64 `json:"gravity"`
	HopChance   float64 `json:"hop_p"`
	SquashDur   int     `json:"squash_dur"`
	DriftHome   float64 `json:"drift_home"`
	// LEVERS — color
	SlimeHue   float64 `json:"slime_hue"`
	HueSpread  float64 `json:"hue_sp"`
	MeadowHue  float64 `json:"meadow_hue"`
	SkyHue     float64 `json:"sky_hue"`
	Saturation float64 `json:"sat"`
	LightMin   float64 `json:"lmin"`
	LightMax   float64 `json:"lmax"`
	// EVENT CHANCES
	WaveChance   float64 `json:"wave_p"`
	CalmChance   float64 `json:"calm_p"`
	BigHopChance float64 `json:"big_hop_p"`
	// EVENT MODIFIERS
	WaveDur     int     `json:"wave_dur"`
	WaveSpacing int     `json:"wave_spacing"`
	CalmDur     int     `json:"calm_dur"`
	CalmMult    float64 `json:"calm_mult"`
	BigHopDur   int     `json:"big_hop_dur"`
	BigHopMult  float64 `json:"big_hop_mult"`
}

func (c SlimesConfig) withDefaults() SlimesConfig {
	if c.IntroDur <= 0 {
		c.IntroDur = 60
	}
	if c.IntroGrowth <= 0 {
		c.IntroGrowth = 0.25
	}
	c.IntroGrowth = clamp01(c.IntroGrowth)
	if c.EndingDur <= 0 {
		c.EndingDur = 80
	}
	if c.EndingLinger < 0 {
		c.EndingLinger = 0
	}
	if c.SlimeCount <= 0 {
		c.SlimeCount = 7
	}
	if c.SlimeCount > 24 {
		c.SlimeCount = 24
	}
	if c.SlimeSize <= 0 {
		c.SlimeSize = 3
	}
	if c.SizeJitter < 0 {
		c.SizeJitter = 0
	}
	c.SizeJitter = clamp01(c.SizeJitter)
	if c.Baseline <= 0 {
		c.Baseline = 0.78
	}
	if c.GrassHeight <= 0 {
		c.GrassHeight = 3
	}
	if c.GrassDens < 0 {
		c.GrassDens = 0
	}
	c.GrassDens = clamp01(c.GrassDens)
	if c.HopPower <= 0 {
		c.HopPower = 1.6
	}
	if c.HopJitter < 0 {
		c.HopJitter = 0
	}
	if c.HopForward < 0 {
		c.HopForward = 0
	}
	if c.Gravity <= 0 {
		c.Gravity = 0.18
	}
	if c.HopChance < 0 {
		c.HopChance = 0
	}
	if c.SquashDur <= 0 {
		c.SquashDur = 10
	}
	if c.DriftHome < 0 {
		c.DriftHome = 0
	}
	if c.SlimeHue == 0 {
		c.SlimeHue = 118
	}
	if c.HueSpread < 0 {
		c.HueSpread = 0
	}
	if c.MeadowHue == 0 {
		c.MeadowHue = 95
	}
	if c.SkyHue == 0 {
		c.SkyHue = 205
	}
	if c.Saturation <= 0 {
		c.Saturation = 0.55
	}
	if c.LightMin <= 0 {
		c.LightMin = 0.18
	}
	if c.LightMax <= 0 {
		c.LightMax = 0.78
	}
	if c.LightMax < c.LightMin {
		c.LightMin, c.LightMax = c.LightMax, c.LightMin
	}
	if c.WaveChance < 0 {
		c.WaveChance = 0
	}
	if c.CalmChance < 0 {
		c.CalmChance = 0
	}
	if c.BigHopChance < 0 {
		c.BigHopChance = 0
	}
	if c.WaveDur <= 0 {
		c.WaveDur = 60
	}
	if c.WaveSpacing <= 0 {
		c.WaveSpacing = 4
	}
	if c.CalmDur <= 0 {
		c.CalmDur = 90
	}
	if c.CalmMult <= 0 || c.CalmMult > 1 {
		c.CalmMult = 0.3
	}
	if c.BigHopDur <= 0 {
		c.BigHopDur = 50
	}
	if c.BigHopMult <= 0 {
		c.BigHopMult = 1.8
	}
	return c
}

// SlimesSchema describes the slimes effect's tunable knobs for the dev UI.
func SlimesSchema() EffectSchema {
	return EffectSchema{
		Name: "slimes",
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "introduction", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 60, Trigger: "intro",
				Description: "Ticks the row of slimes spends fading in from sprouts before the meadow is settled."},
			{Key: "intro_growth", Label: "intro growth", Slot: SlotSpawn, Group: "introduction", Type: KnobFloat, Min: 0.05, Max: 0.9, Step: 0.05, Default: 0.25,
				Description: "Starting size fraction during the intro before each slime finishes growing in."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent shrinking the slimes back into the meadow."},
			{Key: "ending_linger", Label: "ending linger", Slot: SlotEnd, Group: "ending", Type: KnobInt, Min: 0, Max: 160, Step: 5, Default: 25,
				Description: "Extra quiet ticks after the slimes are gone so the meadow holds the frame."},
			{Key: "slime_count", Label: "slime count", Slot: SlotLever, Group: "meadow", Type: KnobInt, Min: 3, Max: 24, Step: 1, Default: 7,
				Description: "Number of slimes resting along the meadow. Lower reads as a clearing; higher as a colony."},
			{Key: "slime_size", Label: "slime size", Slot: SlotLever, Group: "meadow", Type: KnobFloat, Min: 1.5, Max: 6, Step: 0.25, Default: 3,
				Description: "Base body radius for each slime, in pixels."},
			{Key: "size_jit", Label: "size jitter", Slot: SlotLever, Group: "meadow", Type: KnobFloat, Min: 0, Max: 0.6, Step: 0.05, Default: 0.25,
				Description: "Per-slime size variation as a fraction of the base."},
			{Key: "baseline", Label: "baseline", Slot: SlotLever, Group: "meadow", Type: KnobFloat, Min: 0.5, Max: 0.95, Step: 0.01, Default: 0.78,
				Description: "Vertical position of the grass line as a fraction of the frame height."},
			{Key: "grass_h", Label: "grass height", Slot: SlotLever, Group: "meadow", Type: KnobFloat, Min: 1, Max: 8, Step: 0.5, Default: 3,
				Description: "How tall the grass tufts rise above the soil."},
			{Key: "grass_dens", Label: "grass density", Slot: SlotLever, Group: "meadow", Type: KnobFloat, Min: 0, Max: 1, Step: 0.05, Default: 0.45,
				Description: "Fraction of horizontal cells with a grass tuft above the meadow line."},
			{Key: "hop_power", Label: "hop power", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0.5, Max: 4, Step: 0.1, Default: 1.6,
				Description: "Initial upward velocity each slime gets when it starts a hop, in pixels per tick."},
			{Key: "hop_jit", Label: "hop jitter", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0, Max: 0.8, Step: 0.05, Default: 0.3,
				Description: "Per-hop random variation in hop power as a fraction of base."},
			{Key: "hop_fwd", Label: "hop fwd", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0, Max: 1.5, Step: 0.05, Default: 0.5,
				Description: "Sideways drift added per hop. 0 = hops straight up; higher = slimes wander along the row."},
			{Key: "gravity", Label: "gravity", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0.05, Max: 0.6, Step: 0.01, Default: 0.18,
				Description: "Downward acceleration applied per tick to a hopping slime."},
			{Key: "hop_p", Label: "hop chance", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0, Max: 0.1, Step: 0.002, Default: 0.014, Trigger: "hop",
				Description: "Per-tick chance each sitting slime decides to start a new hop. Fire button hops a single random slime immediately."},
			{Key: "squash_dur", Label: "squash dur", Slot: SlotLever, Group: "physics", Type: KnobInt, Min: 2, Max: 30, Step: 1, Default: 10,
				Description: "Ticks a slime spends in the flattened landing pose before it can hop again."},
			{Key: "drift_home", Label: "drift home", Slot: SlotLever, Group: "physics", Type: KnobFloat, Min: 0, Max: 0.5, Step: 0.01, Default: 0.08,
				Description: "How strongly a sitting slime nudges back toward its starting column each tick."},
			{Key: "slime_hue", Label: "slime hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 60, Max: 200, Step: 1, Default: 118,
				Description: "Base slime body hue. Lower values lean yellow-green; higher lean blue-green."},
			{Key: "hue_sp", Label: "hue spread", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 60, Step: 1, Default: 18,
				Description: "Per-slime hue variation in degrees so each slime reads as a slightly different color."},
			{Key: "meadow_hue", Label: "meadow hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 60, Max: 160, Step: 1, Default: 95,
				Description: "Base hue of the meadow grass."},
			{Key: "sky_hue", Label: "sky hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 160, Max: 260, Step: 1, Default: 205,
				Description: "Base hue of the sky above the meadow."},
			{Key: "sat", Label: "saturation", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.2, Max: 1, Step: 0.02, Default: 0.55,
				Description: "Overall scene saturation."},
			{Key: "lmin", Label: "light min", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 0.5, Step: 0.01, Default: 0.18,
				Description: "Minimum lightness used for the darkest shadow under each slime and the lowest soil."},
			{Key: "lmax", Label: "light max", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.45, Max: 1, Step: 0.01, Default: 0.78,
				Description: "Maximum lightness used for slime highlights and the bright sky band."},
			{Key: "wave_p", Label: "wave", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "wave",
				Description: "Per-tick chance of a coordinated hop wave rippling across the row."},
			{Key: "calm_p", Label: "calm", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "calm",
				Description: "Per-tick chance of a quieter window where the slimes barely hop."},
			{Key: "big_hop_p", Label: "big hop", Slot: SlotEvent, Type: KnobFloat, Min: 0, Max: 0.02, Step: 0.0005, Default: 0, Trigger: "big-hop",
				Description: "Per-tick chance of a brief window where every hop is unusually powerful."},
			{Key: "wave_dur", Label: "wave dur", Slot: SlotEventMod, Group: "wave", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 60,
				Description: "Total ticks the hop wave takes to travel across the row."},
			{Key: "wave_spacing", Label: "wave spacing", Slot: SlotEventMod, Group: "wave", Type: KnobInt, Min: 1, Max: 20, Step: 1, Default: 4,
				Description: "Per-slime delay along the wave, in ticks. Higher = a slower, more readable ripple."},
			{Key: "calm_dur", Label: "calm dur", Slot: SlotEventMod, Group: "calm", Type: KnobInt, Min: 10, Max: 240, Step: 5, Default: 90,
				Description: "Duration of a quiet window in which the slimes settle, in ticks."},
			{Key: "calm_mult", Label: "calm x", Slot: SlotEventMod, Group: "calm", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.05, Default: 0.3,
				Description: "Hop-chance multiplier applied while a calm is active."},
			{Key: "big_hop_dur", Label: "big hop dur", Slot: SlotEventMod, Group: "big-hop", Type: KnobInt, Min: 10, Max: 200, Step: 5, Default: 50,
				Description: "Duration of the big-hop window in ticks."},
			{Key: "big_hop_mult", Label: "big hop x", Slot: SlotEventMod, Group: "big-hop", Type: KnobFloat, Min: 1.05, Max: 3, Step: 0.05, Default: 1.8,
				Description: "Hop-power multiplier applied to every hop while big-hop is active."},
		},
	}
}

// SlimesState is the scalar wire/persisted snapshot of the meadow timers.
type SlimesState struct {
	Tick        int `json:"tick"`
	IntroTicks  int `json:"introTicks"`
	IntroTotal  int `json:"introTotal"`
	EndingTicks int `json:"endingTicks"`
	EndingTotal int `json:"endingTotal"`
	EndingFade  int `json:"endingFade"`
	WaveTicks   int `json:"waveTicks"`
	WaveTotal   int `json:"waveTotal"`
	CalmTicks   int `json:"calmTicks"`
	BigHopTicks int `json:"bigHopTicks"`
}

// Slime is the serializable shape of one slime in the meadow.
type Slime struct {
	Col     float64 `json:"col"`
	Row     float64 `json:"row"`
	VCol    float64 `json:"vCol"`
	VRow    float64 `json:"vRow"`
	State   byte    `json:"state"`
	Phase   int     `json:"phase"`
	PhaseT  int     `json:"phaseT"`
	Size    float64 `json:"size"`
	Hue     float64 `json:"hue"`
	HomeCol float64 `json:"homeCol"`
}

type SlimesSnapshot struct {
	SlimesState
	RNGState uint64  `json:"rngState,omitempty"`
	Slimes   []Slime `json:"slimes"`
}

type SlimesPersistedState struct {
	SlimesState
	RNGState uint64  `json:"rngState"`
	Slimes   []Slime `json:"slimes"`
}

// Slimes is the authoritative server-side meadow simulation.
type Slimes struct {
	mu sync.Mutex

	W, H   int
	rng    *rngutil.RNG
	cfg    SlimesConfig
	tick   int
	slimes []Slime

	introTicks  int
	introTotal  int
	endingTicks int
	endingTotal int
	endingFade  int
	waveTicks   int
	waveTotal   int
	calmTicks   int
	bigHopTicks int

	log []LogEntry
}

func NewSlimes(w, h int, seed int64, cfg SlimesConfig) *Slimes {
	s := &Slimes{
		W:   w,
		H:   h,
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	s.resetSlimesLocked()
	return s
}

func (s *Slimes) Resize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if w == s.W && h == s.H {
		return
	}
	xScale := 1.0
	yScale := 1.0
	if s.W > 0 {
		xScale = float64(w) / float64(s.W)
	}
	if s.H > 0 {
		yScale = float64(h) / float64(s.H)
	}
	s.W = w
	s.H = h
	for i := range s.slimes {
		s.slimes[i].Col *= xScale
		s.slimes[i].Row *= yScale
		s.slimes[i].HomeCol *= xScale
	}
}

func (s *Slimes) SetConfig(cfg SlimesConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cfg.withDefaults()
	prevCount := s.cfg.SlimeCount
	s.cfg = next
	if next.SlimeCount != prevCount || len(s.slimes) != next.SlimeCount {
		s.resetSlimesLocked()
	}
}

func (s *Slimes) EffectiveConfig() SlimesConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Slimes) CurrentTick() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

func (s *Slimes) PerturbRNG(delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rng.Mix(delta)
}

func (s *Slimes) DrainLog() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return nil
	}
	out := s.log
	s.log = nil
	return out
}

func (s *Slimes) appendLog(kind, desc string) {
	s.log = append(s.log, LogEntry{Tick: s.tick, Type: kind, Desc: desc})
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *Slimes) Snapshot() SlimesSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SlimesSnapshot{
		SlimesState: s.snapshotStateLocked(),
		RNGState:    s.rng.State(),
		Slimes:      s.copySlimesLocked(),
	}
}

func (s *Slimes) RestoreSnapshot(snap SlimesSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.SlimesState)
	if snap.RNGState != 0 {
		s.rng.SetState(snap.RNGState)
	}
	s.restoreSlimesLocked(snap.Slimes)
}

func (s *Slimes) SnapshotPersistedState() SlimesPersistedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SlimesPersistedState{
		SlimesState: s.snapshotStateLocked(),
		RNGState:    s.rng.State(),
		Slimes:      s.copySlimesLocked(),
	}
}

func (s *Slimes) RestorePersistedState(snap SlimesPersistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restoreStateLocked(snap.SlimesState)
	if snap.RNGState != 0 {
		s.rng.SetState(snap.RNGState)
	}
	s.restoreSlimesLocked(snap.Slimes)
}

func (s *Slimes) snapshotStateLocked() SlimesState {
	return SlimesState{
		Tick:        s.tick,
		IntroTicks:  s.introTicks,
		IntroTotal:  s.introTotal,
		EndingTicks: s.endingTicks,
		EndingTotal: s.endingTotal,
		EndingFade:  s.endingFade,
		WaveTicks:   s.waveTicks,
		WaveTotal:   s.waveTotal,
		CalmTicks:   s.calmTicks,
		BigHopTicks: s.bigHopTicks,
	}
}

func (s *Slimes) restoreStateLocked(state SlimesState) {
	s.tick = state.Tick
	s.introTicks = state.IntroTicks
	s.introTotal = state.IntroTotal
	s.endingTicks = state.EndingTicks
	s.endingTotal = state.EndingTotal
	s.endingFade = state.EndingFade
	s.waveTicks = state.WaveTicks
	s.waveTotal = state.WaveTotal
	s.calmTicks = state.CalmTicks
	s.bigHopTicks = state.BigHopTicks
}

func (s *Slimes) copySlimesLocked() []Slime {
	out := make([]Slime, len(s.slimes))
	copy(out, s.slimes)
	return out
}

func (s *Slimes) restoreSlimesLocked(list []Slime) {
	if len(list) == 0 {
		s.resetSlimesLocked()
		return
	}
	s.slimes = make([]Slime, len(list))
	copy(s.slimes, list)
}

func (s *Slimes) resetSlimesLocked() {
	n := s.cfg.SlimeCount
	if n <= 0 {
		n = 1
	}
	s.slimes = make([]Slime, n)
	baseline := s.baselineRowLocked()
	step := 1.0
	if n > 0 {
		step = float64(s.W) / float64(n+1)
	}
	for i := 0; i < n; i++ {
		home := float64(i+1) * step
		size := s.cfg.SlimeSize
		if s.cfg.SizeJitter > 0 {
			size *= 1 + s.cfg.SizeJitter*(s.rng.Float64()*2-1)
		}
		if size < 1 {
			size = 1
		}
		hue := s.cfg.SlimeHue
		if s.cfg.HueSpread > 0 {
			hue += s.cfg.HueSpread * (s.rng.Float64()*2 - 1)
		}
		hue = math.Mod(hue+360, 360)
		s.slimes[i] = Slime{
			Col:     home,
			Row:     float64(baseline),
			State:   SlimeStateSitting,
			Size:    size,
			Hue:     hue,
			HomeCol: home,
		}
	}
}

func (s *Slimes) baselineRowLocked() int {
	row := int(math.Round(float64(s.H-1) * s.cfg.Baseline))
	if row < 1 {
		row = 1
	}
	if row >= s.H {
		row = s.H - 1
	}
	return row
}

// TriggerEvent fires a discrete event by name. Returns true if the event is
// known to this effect.
func (s *Slimes) TriggerEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "hop":
		idx, ok := s.pickSittingSlimeLocked()
		if !ok {
			s.appendLog("hop", "skipped (no sitting slimes)")
			return true
		}
		s.startHopLocked(idx, 1.0, true)
		s.appendLog("hop", fmt.Sprintf("slime %d (of %d)", idx+1, len(s.slimes)))
	case "wave":
		s.startWaveLocked("triggered")
	case "calm":
		s.startCalmLocked("triggered")
	case "big-hop":
		s.startBigHopLocked("triggered")
	case "intro":
		s.startIntroLocked()
		s.appendLog("intro", fmt.Sprintf("started (dur=%d, growth=%.2f)", s.introTotal, s.cfg.IntroGrowth))
	case "ending":
		s.startEndingLocked()
		s.appendLog("ending", fmt.Sprintf("started (fade=%d, linger=%d)", s.endingFade, s.endingTotal-s.endingFade))
	default:
		return false
	}
	return true
}

func (s *Slimes) pickSittingSlimeLocked() (int, bool) {
	sitters := make([]int, 0, len(s.slimes))
	for i := range s.slimes {
		if s.slimes[i].State == SlimeStateSitting {
			sitters = append(sitters, i)
		}
	}
	if len(sitters) == 0 {
		return 0, false
	}
	return sitters[s.rng.Intn(len(sitters))], true
}

// startHopLocked transitions a sitting slime into the hopping state with
// initial velocity scaled by power. When `signed` is true the horizontal drift
// is chosen randomly so manual triggers feel lively; otherwise the wave event
// fixes a left-to-right ripple.
func (s *Slimes) startHopLocked(idx int, power float64, signed bool) {
	if idx < 0 || idx >= len(s.slimes) {
		return
	}
	sl := &s.slimes[idx]
	if sl.State != SlimeStateSitting {
		return
	}
	jitter := 1.0
	if s.cfg.HopJitter > 0 {
		jitter = 1 + s.cfg.HopJitter*(s.rng.Float64()*2-1)
	}
	if jitter < 0.2 {
		jitter = 0.2
	}
	power *= jitter
	if s.bigHopTicks > 0 {
		power *= s.cfg.BigHopMult
	}
	vRow := -s.cfg.HopPower * power
	vCol := 0.0
	if s.cfg.HopForward > 0 {
		dir := 1.0
		if signed && s.rng.Float64() < 0.5 {
			dir = -1.0
		}
		vCol = dir * s.cfg.HopForward * power
	}
	sl.VRow = vRow
	sl.VCol = vCol
	sl.State = SlimeStateHopping
	sl.Phase = 0
	sl.PhaseT = 0
}

func (s *Slimes) startWaveLocked(verb string) {
	dur := jitterInt(s.rng, s.cfg.WaveDur, 0.25)
	s.waveTicks = dur
	s.waveTotal = dur
	s.appendLog("wave", fmt.Sprintf("%s (dur=%d, spacing=%d)", verb, dur, s.cfg.WaveSpacing))
}

func (s *Slimes) startCalmLocked(verb string) {
	dur := jitterInt(s.rng, s.cfg.CalmDur, 0.3)
	s.calmTicks = dur
	s.appendLog("calm", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, s.cfg.CalmMult))
}

func (s *Slimes) startBigHopLocked(verb string) {
	dur := jitterInt(s.rng, s.cfg.BigHopDur, 0.3)
	s.bigHopTicks = dur
	s.appendLog("big-hop", fmt.Sprintf("%s (dur=%d, x%.2f)", verb, dur, s.cfg.BigHopMult))
}

func (s *Slimes) startIntroLocked() {
	s.resetSlimesLocked()
	s.introTotal = s.cfg.IntroDur
	if s.introTotal <= 0 {
		s.introTotal = 60
	}
	s.introTicks = s.introTotal
	s.endingTicks = 0
	s.endingTotal = 0
	s.endingFade = 0
	s.waveTicks = 0
	s.waveTotal = 0
	s.calmTicks = 0
	s.bigHopTicks = 0
}

func (s *Slimes) startEndingLocked() {
	s.introTicks = 0
	s.introTotal = 0
	s.waveTicks = 0
	s.calmTicks = 0
	s.bigHopTicks = 0
	s.endingFade = s.cfg.EndingDur
	if s.endingFade <= 0 {
		s.endingFade = 80
	}
	linger := s.cfg.EndingLinger
	if linger < 0 {
		linger = 0
	}
	s.endingTotal = s.endingFade + linger
	if s.endingTotal < 1 {
		s.endingTotal = s.endingFade
	}
	s.endingTicks = s.endingTotal
}

func (s *Slimes) Step() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tick++

	if len(s.slimes) != s.cfg.SlimeCount {
		s.resetSlimesLocked()
	}

	if s.endingTicks > 0 {
		s.endingTicks--
		if s.endingTicks == 0 {
			s.startIntroLocked()
			s.appendLog("intro", fmt.Sprintf("auto-restart (dur=%d, growth=%.2f)", s.introTotal, s.cfg.IntroGrowth))
		}
	} else if s.introTicks > 0 {
		s.introTicks--
	}

	if s.waveTicks > 0 {
		s.advanceWaveLocked()
		s.waveTicks--
	}
	if s.calmTicks > 0 {
		s.calmTicks--
	}
	if s.bigHopTicks > 0 {
		s.bigHopTicks--
	}

	s.advanceSlimesLocked()

	if s.endingTicks > 0 || s.introTicks > 0 {
		return
	}

	hopChance := s.cfg.HopChance
	if s.calmTicks > 0 {
		hopChance *= s.cfg.CalmMult
	}
	if hopChance > 0 {
		for i := range s.slimes {
			if s.slimes[i].State != SlimeStateSitting {
				continue
			}
			if s.rng.Float64() < hopChance {
				s.startHopLocked(i, 1.0, true)
			}
		}
	}
	if s.cfg.WaveChance > 0 && s.waveTicks == 0 && s.rng.Float64() < s.cfg.WaveChance {
		s.startWaveLocked("started")
	}
	if s.cfg.CalmChance > 0 && s.calmTicks == 0 && s.rng.Float64() < s.cfg.CalmChance {
		s.startCalmLocked("started")
	}
	if s.cfg.BigHopChance > 0 && s.bigHopTicks == 0 && s.rng.Float64() < s.cfg.BigHopChance {
		s.startBigHopLocked("started")
	}
}

// advanceWaveLocked fires per-slime hops along the wave's progress so the
// ripple reads as left-to-right travel rather than every slime jumping at once.
func (s *Slimes) advanceWaveLocked() {
	if len(s.slimes) == 0 || s.waveTotal <= 0 {
		return
	}
	spacing := s.cfg.WaveSpacing
	if spacing < 1 {
		spacing = 1
	}
	elapsed := s.waveTotal - s.waveTicks
	for i := range s.slimes {
		if s.slimes[i].State != SlimeStateSitting {
			continue
		}
		if elapsed == i*spacing {
			s.startHopLocked(i, 1.0, false)
		}
	}
}

func (s *Slimes) advanceSlimesLocked() {
	baseline := float64(s.baselineRowLocked())
	for i := range s.slimes {
		sl := &s.slimes[i]
		switch sl.State {
		case SlimeStateHopping:
			sl.VRow += s.cfg.Gravity
			sl.Row += sl.VRow
			sl.Col += sl.VCol
			if sl.Col < 1 {
				sl.Col = 1
				sl.VCol = -sl.VCol * 0.5
			}
			if sl.Col >= float64(s.W-1) {
				sl.Col = float64(s.W - 1)
				sl.VCol = -sl.VCol * 0.5
			}
			if sl.Row >= baseline {
				sl.Row = baseline
				sl.State = SlimeStateSquashing
				sl.PhaseT = s.cfg.SquashDur
				sl.Phase = sl.PhaseT
				sl.VRow = 0
				sl.VCol *= 0.4
			}
		case SlimeStateSquashing:
			if sl.Phase > 0 {
				sl.Phase--
			}
			if sl.Phase == 0 {
				sl.State = SlimeStateSitting
				sl.PhaseT = 0
				sl.VRow = 0
				sl.VCol = 0
				sl.Row = baseline
			}
		default:
			if s.cfg.DriftHome > 0 {
				delta := sl.HomeCol - sl.Col
				sl.Col += delta * s.cfg.DriftHome
			}
			sl.Row = baseline
		}
	}
}

