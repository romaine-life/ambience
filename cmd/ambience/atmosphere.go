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
	"sync"
	"time"

	"github.com/nelsong6/ambience/sim"
)

// Command is a single message sent from server to clients.
// Kinds:
//
//	"snapshot"  first message — full state dump (config + active events)
//	"config"    config fields changed; clients apply via SetConfig
//	"trigger"   an event fired (downpour/calm/gust/splash)
type Command struct {
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
	Type         string     `json:"type"`
	Tick         int        `json:"tick"`
	Config       sim.Config `json:"config"`
	Seed         int64      `json:"seed"`
	DownpourLeft int        `json:"downpourLeft"`
	DownpourMult float64    `json:"downpourMult"`
	CalmLeft     int        `json:"calmLeft"`
	GustLeft     int        `json:"gustLeft"`
	GustWind     float64    `json:"gustWind"`
}

type atmosphere struct {
	mu        sync.Mutex
	sim       *sim.Rain
	cfg       sim.Config
	seed      int64
	listeners map[chan Command]struct{}
	lastSeen  time.Time
	cancel    context.CancelFunc
}

func newAtmosphere(cfg sim.Config) *atmosphere {
	seed := time.Now().UnixNano()
	return &atmosphere{
		sim:       sim.NewRain(gridW, gridH, seed, cfg),
		cfg:       cfg,
		seed:      seed,
		listeners: make(map[chan Command]struct{}),
		lastSeen:  time.Now(),
	}
}

// run ticks the atmosphere forever; fired events become broadcast Commands.
func (a *atmosphere) run(ctx context.Context) {
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.sim.Step()
			for _, e := range a.sim.DrainLog() {
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
	defer a.mu.Unlock()
	for ch := range a.listeners {
		select {
		case ch <- cmd:
		default:
		}
	}
}

func (a *atmosphere) snapshot() snapshotData {
	a.mu.Lock()
	seed := a.seed
	a.mu.Unlock()
	// Use the sim's effective config (defaults applied), not our raw stored cfg.
	cfg := a.sim.EffectiveConfig()
	s := a.sim.SnapshotState()
	return snapshotData{
		Type:         "rain",
		Tick:         s.Tick,
		Config:       cfg,
		Seed:         seed,
		DownpourLeft: s.DownpourTicks,
		DownpourMult: s.DownpourMult,
		CalmLeft:     s.CalmTicks,
		GustLeft:     s.GustTicks,
		GustWind:     s.GustWind,
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
	a.mu.Unlock()
	a.sim.PerturbRNG(acc)
}

func (a *atmosphere) setConfig(cfg sim.Config) {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.sim.SetConfig(cfg)
	data, _ := json.Marshal(cfg)
	a.broadcast(Command{
		Kind: "config",
		Tick: a.sim.CurrentTick(),
		Data: data,
	})
}

func (a *atmosphere) triggerEvent(event string) bool {
	// TriggerEvent writes to the log; the run loop picks it up and broadcasts.
	return a.sim.TriggerEvent(event)
}

// Per-session dev atmospheres.
var devAtmospheres sync.Map // string → *atmosphere

func getOrCreateDevAtmosphere(id string) *atmosphere {
	if v, ok := devAtmospheres.Load(id); ok {
		a := v.(*atmosphere)
		a.mu.Lock()
		a.lastSeen = time.Now()
		a.mu.Unlock()
		return a
	}
	a := newAtmosphere(sim.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	actual, loaded := devAtmospheres.LoadOrStore(id, a)
	if loaded {
		cancel()
		return actual.(*atmosphere)
	}
	go a.run(ctx)
	return a
}

func sweepDevAtmospheres() {
	t := time.NewTicker(devSessionSweep)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		devAtmospheres.Range(func(k, v interface{}) bool {
			a := v.(*atmosphere)
			a.mu.Lock()
			empty := len(a.listeners) == 0
			idle := now.Sub(a.lastSeen) > devSessionIdle
			a.mu.Unlock()
			if empty && idle {
				if a.cancel != nil {
					a.cancel()
				}
				devAtmospheres.Delete(k)
			}
			return true
		})
	}
}
