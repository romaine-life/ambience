package sim

import "testing"

func TestBurningTreesIgnitionFlow(t *testing.T) {
	bt := NewBurningTrees(160, 80, 1, BurningTreesConfig{
		TreeCount: 6,
		IgniteDur: 4,
		BurnDur:   6,
		AshDur:    3,
	})
	if !bt.TriggerEvent("ignite") {
		t.Fatalf("expected ignite event to be accepted")
	}
	snap := bt.Snapshot()
	burning := -1
	for i, s := range snap.States {
		if s == BTreeStateIgniting {
			burning = i
			break
		}
	}
	if burning < 0 {
		t.Fatalf("expected one tree in igniting state after trigger, got %v", snap.States)
	}
	// March through ignite -> burn -> ash -> ash linger transitions.
	for i := 0; i < 30; i++ {
		bt.Step()
	}
	post := bt.Snapshot()
	if post.States[burning] != BTreeStateAsh {
		t.Fatalf("expected tree %d to be ash after burn cycle, got state %d", burning, post.States[burning])
	}
}

func TestBurningTreesIntroResetsRow(t *testing.T) {
	bt := NewBurningTrees(160, 80, 7, BurningTreesConfig{TreeCount: 4})
	bt.TriggerEvent("ignite")
	bt.TriggerEvent("intro")
	snap := bt.Snapshot()
	for i, s := range snap.States {
		if s != BTreeStateAlive {
			t.Fatalf("tree %d unexpectedly non-alive after intro: state %d", i, s)
		}
	}
	if snap.IntroTicks <= 0 {
		t.Fatalf("expected intro_ticks > 0 after intro trigger, got %d", snap.IntroTicks)
	}
}

func TestBurningTreesSchemaContainsTriggers(t *testing.T) {
	schema := BurningTreesSchema()
	if schema.Name != "burning-trees" {
		t.Fatalf("schema name mismatch: %q", schema.Name)
	}
	want := map[string]bool{"ignite": true, "flare": true, "lull": true, "intro": true, "ending": true}
	for _, k := range schema.Knobs {
		if k.Trigger != "" {
			delete(want, k.Trigger)
		}
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}
