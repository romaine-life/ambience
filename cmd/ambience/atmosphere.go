// atmosphere coordinates the shared simulation state that clients replicate.
//
// Each atmosphere is a server-side Rain sim that ticks forward — but unlike
// the old pixel-streaming model, the server does not broadcast rendered
// frames. Instead, the atmosphere's role is to DECIDE when discrete events
// fire (downpour, calm, gust, splash) and broadcast those decisions as
// structured commands. Clients run their own local sims and apply the
// commands to stay in rough sync with the atmosphere.
package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/nelsong6/ambience/rngutil"
	"github.com/nelsong6/ambience/sim"
)

// Transition window for config drift at scene boundaries. We don't snap the
// sim.Config on rotation — it looks jarring. Instead, the sim runs with a
// LERP between the previous scene's config and the new one over this many
// ticks, broadcasting the interpolated config every transitionBroadcastEvery
// ticks so clients stay roughly in sync.
//
// Capped by the scene's own DurationTicks / 2 — in test mode with short
// scenes (AMBIENCE_SCENE_TICKS=60) we want the drift to finish before the
// next rotation, so we scale down.
const (
	maxTransitionTicks       = 600 // 60 s at 10 Hz
	transitionBroadcastEvery = 10  // every 1 s during drift
	metricBroadcastEvery     = 50  // every 5 s as a low-rate heartbeat
)

// Command is a single message sent from server to clients.
// Kinds:
//
//	"snapshot"  first message — full state dump (config + active events + scene)
//	"config"    config fields changed; clients apply via SetConfig
//	"trigger"   an event fired (downpour/calm/gust/splash)
//	"scene"     scene rotated; carries new name + duration + next-up name
//	"metric"    periodic status beat (entropy bytes, scene remaining)
type Command struct {
	ID    string          `json:"-"`
	Kind  string          `json:"kind"`
	Tick  int             `json:"tick"`
	Data  json.RawMessage `json:"data,omitempty"`
	Event string          `json:"event,omitempty"`
	Desc  string          `json:"desc,omitempty"`
}

// snapshotData is carried in the initial "snapshot" command and in response
// to GET /snapshot.
//
// Type identifies which effect this atmosphere is currently running ("rain"
// for now; future: "sand", "fire", etc.). Clients look the type up in
// AmbienceSim.effects[type] to pick the renderer constructor — so adding a
// new effect doesn't require any client-side change.
type snapshotData struct {
	Type   string          `json:"type"`
	Tick   int             `json:"tick"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	Seed   int64           `json:"seed"`
	GridW  int             `json:"gridW"`
	GridH  int             `json:"gridH"`
	// Scene + entropy status — used by the / live monitor. Panels update
	// via periodic "metric" commands between full snapshot re-requests.
	CurrentScene   Scene `json:"currentScene"`
	NextScene      Scene `json:"nextScene"`
	EntropyBytes   int64 `json:"entropyBytes"`
	SceneRemaining int   `json:"sceneRemaining"`
}

type atmosphere struct {
	mu sync.Mutex
	// Sim + explicit scene RNG. Entropy POSTs flow into both so future event
	// rolls and future generated scenes survive restarts coherently.
	effect       effectRuntime
	cfg          sim.Config
	seed         int64
	sceneRNG     *rngutil.RNG
	current      Scene
	next         Scene
	entropyBytes int64 // cumulative entropy bytes received since boot
	// Transition state: when a scene rotates, we don't apply the new config
	// instantly. Instead we LERP from transitionFrom to transitionTo over
	// transitionDur ticks starting at transitionStart. transitionDur == 0
	// means "no transition in progress."
	transitionFrom  sim.Config
	transitionTo    sim.Config
	transitionStart int
	transitionDur   int
	commandSeq      int64
	listeners       map[chan Command]struct{}
	lastSeen        time.Time
	cancel          context.CancelFunc
}

func newAtmosphere(_ sim.Config) *atmosphere {
	// Note: seed parameter on newAtmosphere historically accepted a config
	// override, but with scene generation we always start from a fresh
	// generated scene, so the argument is ignored. Kept in the signature to
	// avoid a cascading change in callers (dev atmospheres, tests).
	seed := time.Now().UnixNano()
	sceneRNG := rngutil.New(seed ^ 0x6d0f27bd0b5a3c11)
	first := generateScene(sceneRNG, 0)
	// Pre-generate the next scene too — the "single-slot lookahead" model.
	// StartedAtTick is set when it's promoted to current.
	nxt := generateScene(sceneRNG, 0)
	cfgData, _ := json.Marshal(first.Config)
	return &atmosphere{
		effect:    mustNewEffectRuntime("rain", gridW, gridH, seed, cfgData),
		cfg:       first.Config,
		seed:      seed,
		sceneRNG:  sceneRNG,
		current:   first,
		next:      nxt,
		listeners: make(map[chan Command]struct{}),
		lastSeen:  time.Now(),
	}
}

// run ticks the atmosphere forever. Per tick:
//  1. sim.Step() — advance physics + roll event chances
//  2. transition drift — if a config transition is in progress, apply the
//     interpolated config to the sim + periodically broadcast it
//  3. scene-expired check — if current scene's duration elapsed, promote
//     next → current, generate new next, start a fresh transition
//  4. drain sim log → broadcast trigger commands for fired events
//
// No periodic metric broadcast — that's event-driven now, fired from
// AddEntropy and rotateScene.
func (a *atmosphere) run(ctx context.Context) {
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.effect.Step()
			cur := a.effect.CurrentTick()

			a.applyTransition(cur)

			a.mu.Lock()
			expired := cur >= a.current.StartedAtTick+a.current.DurationTicks
			a.mu.Unlock()
			if expired {
				a.rotateScene(cur)
			}
			if cur%metricBroadcastEvery == 0 {
				a.broadcastMetric(cur)
			}

			for _, e := range a.effect.DrainLog() {
				a.broadcast(Command{
					Kind:  "trigger",
					Tick:  e.Tick,
					Event: e.Type,
					Desc:  e.Desc,
				})
			}
		}
	}
}

// applyTransition drives the LERP during a config-drift window. Called every
// tick; no-op when transitionDur == 0. Broadcasts the interpolated config at
// transitionBroadcastEvery cadence so clients' local sims stay near sync.
// On completion, broadcasts the final target config for exact sync + clears
// the transition state.
func (a *atmosphere) applyTransition(cur int) {
	a.mu.Lock()
	if a.transitionDur == 0 {
		a.mu.Unlock()
		return
	}
	elapsed := cur - a.transitionStart
	if elapsed >= a.transitionDur {
		final := a.transitionTo
		a.transitionDur = 0
		a.transitionStart = 0
		a.cfg = final
		a.mu.Unlock()
		data, _ := json.Marshal(final)
		_ = a.effect.ApplyConfig(data)
		a.broadcast(Command{Kind: "config", Tick: cur, Data: data})
		return
	}
	from := a.transitionFrom
	to := a.transitionTo
	dur := a.transitionDur
	a.mu.Unlock()

	progress := easeInOutCubic(float64(elapsed) / float64(dur))
	lerped := lerpConfig(from, to, progress)
	data, _ := json.Marshal(lerped)
	_ = a.effect.ApplyConfig(data)
	if elapsed%transitionBroadcastEvery == 0 {
		a.broadcast(Command{Kind: "config", Tick: cur, Data: data})
	}
}

// rotateScene promotes next → current, generates a new next, and sets up a
// fresh transition from the previous scene's config to the new one. The
// actual config application happens in applyTransition tick-by-tick — we
// don't call sim.SetConfig here. Broadcasts the "scene" command for panel
// updates; config drift starts broadcasting on the next tick.
func (a *atmosphere) rotateScene(tick int) {
	a.mu.Lock()
	fromCfg := a.cfg
	promoted := a.next
	promoted.StartedAtTick = tick
	a.current = promoted
	a.next = generateScene(a.sceneRNG, 0)
	currentCopy := a.current
	nextName := a.next.Name
	// Transition cap: keep drift bounded by the new scene's duration so we
	// never drift across a scene boundary. Half the scene, max 60 s.
	dur := maxTransitionTicks
	if half := promoted.DurationTicks / 2; half < dur {
		dur = half
	}
	a.transitionFrom = fromCfg
	a.transitionTo = promoted.Config
	a.transitionStart = tick
	a.transitionDur = dur
	a.mu.Unlock()

	sceneData, _ := json.Marshal(map[string]interface{}{
		"name":            currentCopy.Name,
		"durationTicks":   currentCopy.DurationTicks,
		"startedAtTick":   currentCopy.StartedAtTick,
		"nextName":        nextName,
		"transitionTicks": dur,
	})
	a.broadcast(Command{Kind: "scene", Tick: tick, Data: sceneData})

	// Push an immediate metric so panels see the new scene name + fresh
	// remaining without waiting for the next entropy event.
	a.broadcastMetric(tick)
}

// broadcastMetric pushes the current entropy total + scene progress. Called
// whenever an event changes one of these fields (entropy POST, scene
// rotation). No periodic timer.
func (a *atmosphere) broadcastMetric(tick int) {
	a.mu.Lock()
	entropyBytes := a.entropyBytes
	currentCopy := a.current
	nextName := a.next.Name
	a.mu.Unlock()

	data, _ := json.Marshal(map[string]interface{}{
		"entropyBytes":   entropyBytes,
		"sceneRemaining": currentCopy.Remaining(tick),
		"currentName":    currentCopy.Name,
		"nextName":       nextName,
	})
	a.broadcast(Command{Kind: "metric", Tick: tick, Data: data})
}

// lerpConfig linearly interpolates every continuous field of sim.Config,
// plus integer fields via rounded interpolation. Hue uses angular LERP so
// transitions cross the 0°/360° seam cleanly (e.g. 340° → 20° doesn't
// sweep backward through cyan).
func lerpConfig(a, b sim.Config, t float64) sim.Config {
	lf := func(x, y float64) float64 { return x + (y-x)*t }
	li := func(x, y int) int { return x + int(float64(y-x)*t+0.5) }
	return sim.Config{
		Wind:           lf(a.Wind, b.Wind),
		WindJitter:     lf(a.WindJitter, b.WindJitter),
		Speed:          lf(a.Speed, b.Speed),
		SpeedJitter:    lf(a.SpeedJitter, b.SpeedJitter),
		StreakLen:      li(a.StreakLen, b.StreakLen),
		FadeFactor:     lf(a.FadeFactor, b.FadeFactor),
		SpawnEvery:     li(a.SpawnEvery, b.SpawnEvery),
		SpawnBurst:     li(a.SpawnBurst, b.SpawnBurst),
		Hue:            lerpAngle(a.Hue, b.Hue, t),
		HueSpread:      lf(a.HueSpread, b.HueSpread),
		Saturation:     lf(a.Saturation, b.Saturation),
		LightnessMin:   lf(a.LightnessMin, b.LightnessMin),
		LightnessMax:   lf(a.LightnessMax, b.LightnessMax),
		Layers:         li(a.Layers, b.Layers),
		LayerBalance:   lf(a.LayerBalance, b.LayerBalance),
		HueDriftAmp:    lf(a.HueDriftAmp, b.HueDriftAmp),
		WindDriftAmp:   lf(a.WindDriftAmp, b.WindDriftAmp),
		DownpourChance: lf(a.DownpourChance, b.DownpourChance),
		CalmChance:     lf(a.CalmChance, b.CalmChance),
		GustChance:     lf(a.GustChance, b.GustChance),
		SplashChance:   lf(a.SplashChance, b.SplashChance),
		// Event modifier fields (durations/multipliers) are discrete per-event
		// values applied when events fire; no need to interpolate between
		// scenes — the fired event just picks whatever the current scene has.
		DownpourDur:  b.DownpourDur,
		DownpourMult: b.DownpourMult,
		CalmDur:      b.CalmDur,
		GustDur:      b.GustDur,
		GustStrength: b.GustStrength,
		SplashSize:   b.SplashSize,
	}
}

// easeInOutCubic smooths the transition progress so drift speeds up from 0%
// and decelerates toward 100% — no abrupt start or stop.
func easeInOutCubic(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	if t < 0.5 {
		return 4 * t * t * t
	}
	f := 2*t - 2
	return 1 + f*f*f/2
}

// lerpAngle interpolates around the hue circle along the shortest arc.
// Keeps result in [0, 360).
func lerpAngle(a, b, t float64) float64 {
	diff := b - a
	if diff > 180 {
		diff -= 360
	} else if diff < -180 {
		diff += 360
	}
	r := a + diff*t
	if r < 0 {
		r += 360
	}
	if r >= 360 {
		r -= 360
	}
	return r
}

func (a *atmosphere) addListener() chan Command {
	ch := make(chan Command, 32)
	a.mu.Lock()
	a.listeners[ch] = struct{}{}
	a.lastSeen = time.Now()
	a.mu.Unlock()
	return ch
}

func (a *atmosphere) removeListener(ch chan Command) {
	a.mu.Lock()
	delete(a.listeners, ch)
	a.lastSeen = time.Now()
	a.mu.Unlock()
	close(ch)
}

func (a *atmosphere) broadcast(cmd Command) {
	a.mu.Lock()
	if cmd.ID == "" {
		a.commandSeq++
		cmd.ID = strconv.FormatInt(a.commandSeq, 10)
	}
	defer a.mu.Unlock()
	for ch := range a.listeners {
		select {
		case ch <- cmd:
		default:
		}
	}
}

func (a *atmosphere) currentCommandID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strconv.FormatInt(a.commandSeq, 10)
}

func (a *atmosphere) snapshot() snapshotData {
	a.mu.Lock()
	seed := a.seed
	current := a.current
	next := a.next
	entropyBytes := a.entropyBytes
	a.mu.Unlock()
	effectSnap, err := a.effect.Snapshot()
	if err != nil {
		return snapshotData{
			Type:           a.effect.Type(),
			Seed:           seed,
			CurrentScene:   current,
			NextScene:      next,
			EntropyBytes:   entropyBytes,
			SceneRemaining: current.Remaining(0),
		}
	}
	return snapshotData{
		Type:           a.effect.Type(),
		Tick:           effectSnap.Tick,
		Config:         cloneRaw(effectSnap.Config),
		State:          cloneRaw(effectSnap.State),
		Seed:           seed,
		GridW:          effectSnap.GridW,
		GridH:          effectSnap.GridH,
		CurrentScene:   current,
		NextScene:      next,
		EntropyBytes:   entropyBytes,
		SceneRemaining: current.Remaining(effectSnap.Tick),
	}
}

// AddEntropy folds external entropy bytes into the atmosphere's RNG state.
// The bytes are summed into the current seed and reshuffled via the sim's
// rng — effectively nudging future random decisions. Cheap; not
// cryptographically strong, which is fine — this is ambient aesthetic
// perturbation, not security.
func (a *atmosphere) AddEntropy(b []byte) {
	if len(b) == 0 {
		return
	}
	var acc int64
	for _, x := range b {
		acc = (acc*31 + int64(x)) & 0x7fffffffffffffff
	}
	a.mu.Lock()
	a.seed ^= acc
	a.entropyBytes += int64(len(b))
	a.sceneRNG.Mix(acc)
	a.mu.Unlock()
	a.effect.AddEntropy(acc)
	// Push a metric broadcast on every entropy event so the / live monitor's
	// counter updates immediately (no 30 s polling cadence needed). Sub-
	// second latency matches the client-side entropy buffer flush (2 s).
	a.broadcastMetric(a.effect.CurrentTick())
}

func (a *atmosphere) setConfigRaw(data json.RawMessage) error {
	if err := a.effect.ApplyConfig(data); err != nil {
		return err
	}
	effectType := a.effect.Type()
	var cfg sim.Config
	hasRainConfig := effectType == "rain" && json.Unmarshal(data, &cfg) == nil
	if hasRainConfig {
		cfg = sim.NormalizeConfig(cfg)
	}
	a.mu.Lock()
	// A manual live edit should take over immediately instead of getting
	// overwritten by the previous scene transition on the next tick.
	a.transitionDur = 0
	a.transitionStart = 0
	if hasRainConfig {
		a.cfg = cfg
		a.current.Config = cfg
	}
	a.mu.Unlock()
	a.broadcast(Command{
		Kind: "config",
		Tick: a.effect.CurrentTick(),
		Data: cloneRaw(data),
	})
	return nil
}

func (a *atmosphere) triggerEvent(event string) bool {
	// TriggerEvent writes to the log; the run loop picks it up and broadcasts.
	return a.effect.Trigger(event)
}
