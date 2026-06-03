package terminal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/romaine-life/ambience/sim"
)

func newTestClient() *Client {
	c := New(Config{
		GridW:      20,
		GridH:      10,
		TickRate:   100 * time.Millisecond,
		DelayTicks: 5,
	})
	now := time.Unix(100, 0)
	c.playbackClock.now = func() time.Time { return now }
	return c
}

func marshalCommand(t *testing.T, kind string, tick int, event string, data any) string {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := json.Marshal(commandWire{
		Kind:  kind,
		Tick:  tick,
		Event: event,
		Data:  raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(cmd)
}

func snapshotPayload(t *testing.T, tick int, cfg sim.Config, state sim.RainSnapshot) snapshotWire {
	t.Helper()
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return snapshotWire{
		Type:   "rain",
		Tick:   tick,
		Config: cfgRaw,
		State:  stateRaw,
	}
}

func applySnapshot(t *testing.T, c *Client, tick int, cfg sim.Config) {
	t.Helper()
	c.applyCommand(marshalCommand(t, "snapshot", tick, "", snapshotPayload(t, tick, cfg, sim.RainSnapshot{
		State: sim.State{Tick: tick},
	})))
}

func TestSnapshotInitializesRainStateAndReadiness(t *testing.T) {
	c := newTestClient()
	state := sim.RainSnapshot{
		State: sim.State{Tick: 10, GustTicks: 3, GustWind: 1.25},
		Drops: []sim.Drop{{
			Row:       2,
			Col:       4,
			Color:     sim.RGB{R: 120, G: 140, B: 255},
			VRow:      1,
			StreakLen: 3,
		}},
	}
	c.applyCommand(marshalCommand(t, "snapshot", 10, "", snapshotPayload(t, 10, sim.Config{Speed: 1.4}, state)))

	debug := c.DebugState()
	if !debug.Ready || debug.EffectType != "rain" || !debug.RainOnly {
		t.Fatalf("debug readiness = %+v, want ready rain-only rain", debug)
	}
	if got := c.sim.CurrentTick(); got != 10 {
		t.Fatalf("sim tick = %d, want 10", got)
	}
	if got := len(c.sim.DropsCopy()); got != 1 {
		t.Fatalf("drops = %d, want restored snapshot drop", got)
	}
}

func TestClockSamplesAdvanceDelayedPlaybackTarget(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 10, sim.Config{})
	c.applyCommand(marshalCommand(t, "clock", 20, "", map[string]int{"tick": 20}))

	debug := c.DebugState()
	if debug.AuthorityTick != 20 || debug.PlaybackTick != 15 || debug.DriftTicks != 5 {
		t.Fatalf("debug clock state = %+v, want authority=20 playback=15 drift=5", debug)
	}
	if !debug.HaveAuthoritySample {
		t.Fatal("expected authority sample")
	}
}

func TestFutureConfigIsQueuedAndNotAppliedEarly(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 10, sim.Config{Speed: 1, SpawnEvery: 1000})
	c.applyCommand(marshalCommand(t, "config", 12, "", sim.Config{Speed: 2, SpawnEvery: 1000}))

	if got := c.sim.EffectiveConfig().Speed; got != 1 {
		t.Fatalf("config applied early: speed = %v, want 1", got)
	}
	c.applyCommand(marshalCommand(t, "clock", 16, "", map[string]int{"tick": 16}))
	c.stepTowardAuthorityClock()
	if got := c.sim.EffectiveConfig().Speed; got != 1 {
		t.Fatalf("config applied before playback tick 12: speed = %v, want 1", got)
	}
	c.applyCommand(marshalCommand(t, "clock", 17, "", map[string]int{"tick": 17}))
	c.stepTowardAuthorityClock()
	if got := c.sim.EffectiveConfig().Speed; got != 2 {
		t.Fatalf("config not applied at playback tick 12: speed = %v, want 2", got)
	}
}

func TestFutureTriggerIsQueuedAndAppliedAtDelayedPlaybackTick(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 10, sim.Config{SpawnEvery: 1000, IntroDur: 7, IntroSeed: 1})
	c.applyCommand(marshalCommand(t, "trigger", 12, "intro", nil))

	c.applyCommand(marshalCommand(t, "clock", 16, "", map[string]int{"tick": 16}))
	c.stepTowardAuthorityClock()
	if got := c.sim.SnapshotState().IntroTicks; got != 0 {
		t.Fatalf("trigger applied early: intro ticks = %d, want 0", got)
	}

	c.applyCommand(marshalCommand(t, "clock", 17, "", map[string]int{"tick": 17}))
	c.stepTowardAuthorityClock()
	if got := c.sim.SnapshotState().IntroTicks; got == 0 {
		t.Fatal("trigger was not applied at delayed playback tick")
	}
}

func TestSparseLaterClockSamplesUseBoundedCatchup(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 0, sim.Config{SpawnEvery: 1000})
	c.applyCommand(marshalCommand(t, "clock", 200, "", map[string]int{"tick": 200}))
	c.stepTowardAuthorityClock()

	if got := c.sim.CurrentTick(); got != 5 {
		t.Fatalf("sim tick = %d, want bounded 5-step catch-up", got)
	}
}

func TestFreshSnapshotClearsStaleQueuedCommands(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 5, sim.Config{})
	c.applyCommand(marshalCommand(t, "config", 9, "", sim.Config{Speed: 1.5}))
	c.applyCommand(marshalCommand(t, "trigger", 10, "gust", nil))
	c.applyCommand(marshalCommand(t, "config", 12, "", sim.Config{Speed: 2}))

	c.applyCommand(marshalCommand(t, "snapshot", 10, "", snapshotPayload(t, 10, sim.Config{Speed: 1}, sim.RainSnapshot{
		State: sim.State{Tick: 10},
	})))

	debug := c.DebugState()
	if debug.QueuedCommands != 1 || debug.NextQueuedCommandTick == nil || *debug.NextQueuedCommandTick != 12 {
		t.Fatalf("queue after snapshot = %+v, want only tick 12 command", debug)
	}
}

func TestDebugTelemetryReportsQueueAndClockState(t *testing.T) {
	c := newTestClient()
	applySnapshot(t, c, 10, sim.Config{})
	c.applyCommand(marshalCommand(t, "clock", 20, "", map[string]int{"tick": 20}))
	c.applyCommand(marshalCommand(t, "config", 17, "", sim.Config{Speed: 1.5}))
	c.applyCommand(marshalCommand(t, "trigger", 22, "gust", nil))

	debug := c.DebugState()
	if debug.QueuedCommands != 2 {
		t.Fatalf("queued commands = %d, want 2", debug.QueuedCommands)
	}
	if debug.NextQueuedCommandTick == nil || *debug.NextQueuedCommandTick != 17 {
		t.Fatalf("next queued tick = %v, want 17", debug.NextQueuedCommandTick)
	}
	if debug.MaxQueuedCommandTick == nil || *debug.MaxQueuedCommandTick != 22 {
		t.Fatalf("max queued tick = %v, want 22", debug.MaxQueuedCommandTick)
	}
	if debug.BufferedAheadTicks != 5 || debug.DelayTicks != 5 || debug.DriftTicks != 7 {
		t.Fatalf("debug telemetry = %+v, want buffered=5 delay=5 drift=7", debug)
	}
	if !debug.HaveAuthoritySample {
		t.Fatal("expected authority sample")
	}
}
