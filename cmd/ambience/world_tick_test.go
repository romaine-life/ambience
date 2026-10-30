package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

func drainCommands(ch chan Command) []Command {
	var out []Command
	for {
		select {
		case cmd := <-ch:
			out = append(out, cmd)
		default:
			return out
		}
	}
}

// TestWorldTickContinuesAcrossRotation pins the clock clients follow: the
// effect runtime restarts at tick 0 on a rotation, but every broadcast stays on
// the world tick, which keeps counting.
func TestWorldTickContinuesAcrossRotation(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	a.setRotationPolicy(rotationPolicy{Enabled: true, CadenceTicks: 10, Allowed: []string{"rain", "aurora"}})
	for i := 0; i < 25; i++ {
		a.effect.Step()
	}
	before := a.effect.CurrentTick()
	ch := a.addListener()
	defer a.removeListener(ch)

	if !a.rotateToEffect(before, "aurora") {
		t.Fatal("rotation did not fire")
	}
	if got := a.effect.CurrentTick(); got != 0 {
		t.Fatalf("new effect runtime tick = %d, want 0", got)
	}
	var snapCmd *Command
	for _, cmd := range drainCommands(ch) {
		if cmd.Tick < before {
			t.Errorf("%s broadcast at world tick %d, before the rotation tick %d", cmd.Kind, cmd.Tick, before)
		}
		if cmd.Kind == "snapshot" {
			c := cmd
			snapCmd = &c
		}
	}
	if snapCmd == nil {
		t.Fatal("no snapshot broadcast")
	}
	if snapCmd.Tick != before {
		t.Fatalf("rotation snapshot world tick = %d, want %d", snapCmd.Tick, before)
	}
	var snap snapshotData
	if err := json.Unmarshal(snapCmd.Data, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Tick != 0 || snap.WorldTick != before {
		t.Fatalf("snapshot tick/worldTick = %d/%d, want 0/%d", snap.Tick, snap.WorldTick, before)
	}

	a.effect.Step()
	a.broadcastClock(a.effect.CurrentTick())
	cmds := drainCommands(ch)
	if len(cmds) != 1 || cmds[0].Kind != "clock" {
		t.Fatalf("want one clock command, got %+v", cmds)
	}
	var clk clockData
	if err := json.Unmarshal(cmds[0].Data, &clk); err != nil {
		t.Fatal(err)
	}
	if cmds[0].Tick != before+1 || clk.Tick != before+1 {
		t.Fatalf("clock after rotation at %d (data %d), want world tick %d", cmds[0].Tick, clk.Tick, before+1)
	}
	if clk.SuggestedDelayTicks != playbackDelayTicks {
		t.Fatalf("suggested delay = %d, want %d", clk.SuggestedDelayTicks, playbackDelayTicks)
	}
}

func TestWorldTickSurvivesPersistRoundTrip(t *testing.T) {
	a := newAtmosphere(sim.Config{})
	for i := 0; i < 7; i++ {
		a.effect.Step()
	}
	if !a.rotateToEffect(a.effect.CurrentTick(), "aurora") {
		t.Fatal("rotation did not fire")
	}
	for i := 0; i < 3; i++ {
		a.effect.Step()
	}
	want := a.snapshot().WorldTick

	store := &memoryStore{}
	if err := store.Save(context.Background(), a.persistedState()); err != nil {
		t.Fatalf("save: %v", err)
	}
	restored := restoreSharedAtmosphere(context.Background(), store)
	if got := restored.snapshot().WorldTick; got != want {
		t.Fatalf("world tick after restore = %d, want %d", got, want)
	}
}
