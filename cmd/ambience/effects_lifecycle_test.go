package main

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

// lifecycleGuardMaxTicks bounds how long the guard will step an effect while
// waiting for a lifecycle transition under default config. Default intro and
// outro durations are hundreds of ticks; anything that cannot resolve within
// this bound is broken for verification purposes too (/dev/observe caps at
// 20000 ticks).
const lifecycleGuardMaxTicks = 20000

// TestEveryLifecycleEffectHonorsTheLifecycleContract is the migration guard
// for the lifecycle observer contract (ambience#174): every effect that
// registers intro/ending triggers must publish a `lifecycle` field in its
// snapshot state and walk the closed enum correctly under default config:
//
//	intro trigger  -> "intro"   -> resolves to "running"
//	ending trigger -> "ending"  -> resolves to "ended" when the schema
//	                  declares ending_terminal, else back to "running"
//
// Effects without intro/ending triggers must report "running" always. The
// test runs against the real registry so a new effect cannot land without
// honoring the contract — verification plans assert lifecycle values, never
// effect-internal state fields, and this is what keeps that assertion
// decidable. ambience#167 run 8.1 (a correct implementation failed because a
// plan guessed introTicks semantics) is the motivating failure.
func TestEveryLifecycleEffectHonorsTheLifecycleContract(t *testing.T) {
	types := make([]string, 0, len(effectRegistry))
	for effectType := range effectRegistry {
		types = append(types, effectType)
	}
	sort.Strings(types)
	if len(types) == 0 {
		t.Fatal("effect registry is empty")
	}

	for _, effectType := range types {
		def := effectRegistry[effectType]
		t.Run(effectType, func(t *testing.T) {
			schema := def.Schema()
			rt, err := def.NewRuntime(96, 54, 4242, nil)
			if err != nil {
				t.Fatalf("new runtime: %v", err)
			}
			lifecycle := func() sim.Lifecycle {
				snap, err := rt.Snapshot()
				if err != nil {
					t.Fatalf("snapshot: %v", err)
				}
				return lifecycleFromState(snap.State)
			}
			stepUntil := func(want sim.Lifecycle, context string) {
				for i := 0; i < lifecycleGuardMaxTicks; i++ {
					if lifecycle() == want {
						return
					}
					rt.Step()
				}
				t.Fatalf("%s: lifecycle never reached %q within %d ticks (last %q)", context, want, lifecycleGuardMaxTicks, lifecycle())
			}

			hasIntro := schemaDeclaresTrigger(schema, "intro")
			hasEnding := schemaDeclaresTrigger(schema, "ending")

			if !hasIntro && !hasEnding {
				if schema.EndingTerminal {
					t.Fatal("schema declares ending_terminal without an ending trigger")
				}
				for i := 0; i < 25; i++ {
					if got := lifecycle(); got != sim.LifecycleRunning {
						t.Fatalf("steady-only effect reported lifecycle %q at step %d, want running", got, i)
					}
					rt.Step()
				}
				return
			}

			// A lifecycle-capable effect must publish the field from birth —
			// an empty value would read as the fail-open "running" default
			// and hide non-compliance, so probe the raw state for presence.
			snap, err := rt.Snapshot()
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if !stateDeclaresLifecycle(snap.State) {
				t.Fatal("effect registers intro/ending triggers but its snapshot state has no lifecycle field")
			}

			if hasIntro {
				if !rt.Trigger("intro") {
					t.Fatal("intro trigger rejected")
				}
				if got := lifecycle(); got != sim.LifecycleIntro {
					t.Fatalf("after intro trigger lifecycle = %q, want intro", got)
				}
				stepUntil(sim.LifecycleRunning, "intro resolution")
			}

			if hasEnding {
				if !rt.Trigger("ending") {
					t.Fatal("ending trigger rejected")
				}
				if got := lifecycle(); got != sim.LifecycleEnding {
					t.Fatalf("after ending trigger lifecycle = %q, want ending", got)
				}
				if schema.EndingTerminal {
					stepUntil(sim.LifecycleEnded, "terminal ending resolution")
					for i := 0; i < 50; i++ {
						rt.Step()
						if got := lifecycle(); got != sim.LifecycleEnded {
							t.Fatalf("terminal ending did not hold: lifecycle %q after %d extra ticks", got, i+1)
						}
					}
					if hasIntro {
						// The terminal hold must release through intro.
						if !rt.Trigger("intro") {
							t.Fatal("intro trigger rejected from ended state")
						}
						if got := lifecycle(); got != sim.LifecycleIntro {
							t.Fatalf("intro from ended state: lifecycle = %q, want intro", got)
						}
						stepUntil(sim.LifecycleRunning, "post-ended intro resolution")
					}
				} else {
					stepUntil(sim.LifecycleRunning, "non-terminal ending resolution")
				}
			}
		})
	}
}

func schemaDeclaresTrigger(schema sim.EffectSchema, trigger string) bool {
	for _, knob := range schema.Knobs {
		if knob.Trigger == trigger {
			return true
		}
	}
	return false
}

func stateDeclaresLifecycle(state []byte) bool {
	var probe map[string]any
	if len(state) == 0 || json.Unmarshal(state, &probe) != nil {
		return false
	}
	value, ok := probe["lifecycle"].(string)
	return ok && value != ""
}
