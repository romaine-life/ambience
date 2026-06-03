package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

func TestLiveEffectReplayAudit(t *testing.T) {
	effects := liveEffectTypesForReplayAudit()
	if len(effects) == 0 {
		t.Fatal("effect registry is empty")
	}

	for _, effectType := range effects {
		t.Run(effectType, func(t *testing.T) {
			const seed int64 = 0x5eed1234
			cfg := replayAuditConfig(t, effectType)

			authority := mustNewReplayAuditRuntime(t, effectType, seed, cfg)
			replica := mustNewReplayAuditRuntime(t, effectType, seed, cfg)

			for i := 0; i < 7; i++ {
				authority.Step()
			}
			if len(cfg) > 0 {
				if err := authority.ApplyConfig(cfg); err != nil {
					t.Fatalf("apply config: %v", err)
				}
			}
			authority.AddEntropy(37)
			for _, trigger := range replayAuditTriggers(t, effectType) {
				if !authority.Trigger(trigger) {
					t.Fatalf("schema trigger %q was not accepted", trigger)
				}
			}
			for i := 0; i < 9; i++ {
				authority.Step()
			}

			snap, err := authority.Snapshot()
			if err != nil {
				t.Fatalf("snapshot authority: %v", err)
			}
			if !snapshotCarriesRNGState(snap.State) {
				t.Fatalf("snapshot state does not carry rngState: %s", snap.State)
			}
			if err := replica.Restore(snap); err != nil {
				t.Fatalf("restore snapshot: %v", err)
			}
			assertReplayRuntimeEqual(t, authority, replica, "after restore")

			for i := 0; i < 15; i++ {
				authority.Step()
				replica.Step()
				assertReplayRuntimeEqual(t, authority, replica, fmt.Sprintf("after post-restore step %d", i+1))
			}
		})
	}
}

func liveEffectTypesForReplayAudit() []string {
	out := make([]string, 0, len(effectRegistry))
	for effectType := range effectRegistry {
		out = append(out, effectType)
	}
	sort.Strings(out)
	return out
}

func mustNewReplayAuditRuntime(t *testing.T, effectType string, seed int64, cfg json.RawMessage) effectRuntime {
	t.Helper()
	rt, err := newEffectRuntime(effectType, 80, 48, seed, cfg)
	if err != nil {
		t.Fatalf("new %s runtime: %v", effectType, err)
	}
	return rt
}

func replayAuditConfig(t *testing.T, effectType string) json.RawMessage {
	t.Helper()
	schema, ok := schemaForEffect(effectType)
	if !ok {
		t.Fatalf("missing schema for %s", effectType)
	}
	cfg := make(map[string]any, len(schema.Knobs))
	for _, knob := range schema.Knobs {
		switch knob.Type {
		case sim.KnobInt:
			cfg[knob.Key] = int(clampReplayAuditValue(knob.Default+knob.Step, knob.Min, knob.Max))
		case sim.KnobFloat:
			cfg[knob.Key] = clampReplayAuditValue(knob.Default+knob.Step, knob.Min, knob.Max)
		default:
			t.Fatalf("unsupported knob type %q for %s.%s", knob.Type, effectType, knob.Key)
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return data
}

func replayAuditTriggers(t *testing.T, effectType string) []string {
	t.Helper()
	schema, ok := schemaForEffect(effectType)
	if !ok {
		t.Fatalf("missing schema for %s", effectType)
	}
	seen := map[string]bool{}
	var triggers []string
	for _, knob := range schema.Knobs {
		if knob.Trigger == "" || seen[knob.Trigger] {
			continue
		}
		seen[knob.Trigger] = true
		triggers = append(triggers, knob.Trigger)
	}
	sort.Strings(triggers)
	return triggers
}

func clampReplayAuditValue(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func snapshotCarriesRNGState(data json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false
	}
	raw, ok := fields["rngState"]
	if !ok {
		return false
	}
	return len(raw) > 0 && !bytes.Equal(raw, []byte("0"))
}

func assertReplayRuntimeEqual(t *testing.T, authority, replica effectRuntime, phase string) {
	t.Helper()
	if authority.CurrentTick() != replica.CurrentTick() {
		t.Fatalf("%s: tick = %d, want %d", phase, replica.CurrentTick(), authority.CurrentTick())
	}
	if !reflect.DeepEqual(authority.Frame(), replica.Frame()) {
		t.Fatalf("%s: frame mismatch", phase)
	}

	authoritySnap, err := authority.Snapshot()
	if err != nil {
		t.Fatalf("%s: snapshot authority: %v", phase, err)
	}
	replicaSnap, err := replica.Snapshot()
	if err != nil {
		t.Fatalf("%s: snapshot replica: %v", phase, err)
	}
	if !jsonEqual(authoritySnap.Config, replicaSnap.Config) {
		t.Fatalf("%s: config mismatch\nauthority: %s\nreplica: %s", phase, authoritySnap.Config, replicaSnap.Config)
	}
	if !jsonEqual(authoritySnap.State, replicaSnap.State) {
		t.Fatalf("%s: state mismatch\nauthority: %s\nreplica: %s", phase, authoritySnap.State, replicaSnap.State)
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
