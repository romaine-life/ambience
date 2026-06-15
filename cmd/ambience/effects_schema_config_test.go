package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"testing"

	"github.com/romaine-life/ambience/sim"
)

// schemaConfigPinEpsilon mirrors the float tolerance the verification pin
// helper (the pin_check curl/jq path the Glimmung verify agent step runs)
// uses when it compares the live /dev/snapshot config against the pinned
// schema defaults. Int knobs compare after rounding, exactly like the helper.
const schemaConfigPinEpsilon = 1e-6

// schemaConfigGuardTicks is the capture-window stand-in: after the pin lands,
// the verify agent step records evidence for a while and then re-checks the
// pin (enforce_session_config_pinned -> pin_check). A config that
// only matches at the instant it is applied — because Step() drifts or
// rewrites config fields — fails that post-capture re-check just as surely as
// a missing knob does, so the guard holds the session for a window of ticks
// too.
const schemaConfigGuardTicks = 200

// reservedDevConfigParams are query parameters POST /dev/config (and the
// other /dev endpoints) interpret before knob parsing. A schema knob keyed
// like one of these could never be pinned over HTTP: the value would be
// consumed as session routing, not as a knob.
var reservedDevConfigParams = map[string]bool{
	"session": true,
	"effect":  true,
	"gridW":   true,
	"gridH":   true,
}

// TestEveryEffectSchemaKnobRoundTripsThroughDevConfig is the registry guard
// for the verification pin contract: every knob an effect declares in its
// schema must be settable through the /dev/config path and must read back
// from the /dev/snapshot config at its schema default, knob-for-knob.
//
// That contract is what verification runs against. Dev sessions are created
// with RANDOMIZED knob values, so before capturing any evidence the verifier
// pins the session over plain HTTP (the pin_session curl/jq path the Glimmung
// verify agent step runs): POST /dev/config with every schema knob
// set to its default, then poll /dev/snapshot until the live config matches,
// and re-check the pin again after capture. A knob
// that exists in the schema but never surfaces in the snapshot config makes
// that loop unsatisfiable — `Number(undefined)` never equals the default.
//
// ambience#167 run 10.1 is the motivating failure: the generated
// paper-lanterns implementation declared schema knobs (intro_event,
// ending_event, fade_event) that never appeared in the live dev config
// snapshot after POST /dev/config, so the pin could not land and the run
// aborted only after verification tokens were already spent. This test runs
// the same pin loop server-side, at the same seam the HTTP handlers use
// (setConfigQuery == POST /dev/config, snapshot == GET /dev/snapshot),
// against the real effect registry — so that class of implementation dies in
// PR CI (the implementation stage gates on the draft PR's go-test check
// before any verify case runs), the way effects_lifecycle_test.go made
// lifecycle violations unrepresentable.
func TestEveryEffectSchemaKnobRoundTripsThroughDevConfig(t *testing.T) {
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
			if len(schema.Knobs) == 0 {
				// The verification pin refuses a knobless schema
				// ("declares no knobs; nothing to pin"), which makes every
				// claim against the effect unverifiable.
				t.Fatal("schema declares no knobs; the verification pin contract needs at least one knob")
			}

			seen := make(map[string]bool, len(schema.Knobs))
			for _, knob := range schema.Knobs {
				if knob.Key == "" {
					t.Fatalf("schema knob %q has an empty key", knob.Label)
				}
				if reservedDevConfigParams[knob.Key] {
					t.Fatalf("schema knob %q collides with a reserved /dev/config query parameter", knob.Key)
				}
				if seen[knob.Key] {
					t.Fatalf("schema declares knob key %q twice; the pin would be ambiguous", knob.Key)
				}
				seen[knob.Key] = true
				if knob.Type != sim.KnobInt && knob.Type != sim.KnobFloat {
					t.Fatalf("schema knob %q has type %q; /dev/config can only pin int/float knobs", knob.Key, knob.Type)
				}
				lo, hi := knob.Min, knob.Max
				if hi < lo {
					lo, hi = hi, lo
				}
				if pinned := pinnedKnobDefault(knob); pinned < lo || pinned > hi {
					t.Fatalf("schema knob %q default %v is outside its own range [%v, %v]; the pinned baseline would be unrepresentable", knob.Key, pinned, lo, hi)
				}
			}

			// Create the session the way the /dev endpoints do. Its starting
			// config is randomized by design — which is exactly why the
			// verifier must be able to pin it to schema defaults.
			s, err := newDevSessionWithGrid(effectType, 96, 54)
			if err != nil {
				t.Fatalf("new dev session: %v", err)
			}

			pin := url.Values{}
			for _, knob := range schema.Knobs {
				pin.Set(knob.Key, formatPinnedKnobDefault(knob))
			}

			// (a) Pin schema defaults through the same internal path
			// POST /dev/config uses; every schema knob must now be present
			// in the snapshot config at its default value.
			if err := s.setConfigQuery(pin); err != nil {
				t.Fatalf("pin schema defaults via the /dev/config path: %v", err)
			}
			assertSnapshotConfigPinnedToSchemaDefaults(t, schema, s, "after pinning schema defaults")
			if t.Failed() {
				return
			}

			// The pin must hold across the capture window, not just at the
			// instant it lands — the wrapper re-checks it after evidence
			// capture.
			for i := 0; i < schemaConfigGuardTicks; i++ {
				s.stepAndBroadcast()
			}
			assertSnapshotConfigPinnedToSchemaDefaults(t, schema, s, fmt.Sprintf("after %d ticks under the pinned config", schemaConfigGuardTicks))
			if t.Failed() {
				return
			}

			// (b) Re-applying the same explicit defaults must round-trip
			// every knob key/value again — pinning is idempotent, so a
			// verifier can always re-pin a live session.
			if err := s.setConfigQuery(pin); err != nil {
				t.Fatalf("re-pin schema defaults via the /dev/config path: %v", err)
			}
			assertSnapshotConfigPinnedToSchemaDefaults(t, schema, s, "after re-applying schema defaults")
		})
	}
}

// assertSnapshotConfigPinnedToSchemaDefaults reads the session snapshot the
// way GET /dev/snapshot does and checks every schema knob is present in its
// config at the pinned schema default. Mismatches report per-knob so a
// violating effect lists every unsatisfiable knob at once; the session seed
// is included for reproducibility.
func assertSnapshotConfigPinnedToSchemaDefaults(t *testing.T, schema sim.EffectSchema, s *devSession, context string) {
	t.Helper()
	snap := s.snapshot()
	if len(snap.Config) == 0 {
		t.Fatalf("%s: snapshot config is empty (seed %d)", context, snap.Seed)
	}
	var live map[string]any
	if err := json.Unmarshal(snap.Config, &live); err != nil {
		t.Fatalf("%s: snapshot config is not a JSON object: %v (seed %d)", context, err, snap.Seed)
	}
	for _, knob := range schema.Knobs {
		raw, present := live[knob.Key]
		if !present {
			t.Errorf("%s: schema knob %q is missing from the snapshot config — the verification pin contract is unsatisfiable (seed %d)", context, knob.Key, snap.Seed)
			continue
		}
		got, isNumber := raw.(float64)
		if !isNumber {
			t.Errorf("%s: schema knob %q is %T in the snapshot config, want a JSON number (seed %d)", context, knob.Key, raw, snap.Seed)
			continue
		}
		if !pinnedKnobDefaultMatches(knob, got) {
			t.Errorf("%s: schema knob %q = %v in the snapshot config, want schema default %v (seed %d)", context, knob.Key, got, pinnedKnobDefault(knob), snap.Seed)
		}
	}
}

// pinnedKnobDefault is the value the pin helper sends and expects back for a
// knob: the schema default, rounded for int knobs.
func pinnedKnobDefault(knob sim.Knob) float64 {
	if knob.Type == sim.KnobInt {
		return math.Round(knob.Default)
	}
	return knob.Default
}

// pinnedKnobDefaultMatches mirrors the verification pin comparison
// (pin_config_mismatches in the Glimmung verify agent step): int knobs
// match after rounding, float knobs within schemaConfigPinEpsilon.
func pinnedKnobDefaultMatches(knob sim.Knob, got float64) bool {
	if knob.Type == sim.KnobInt {
		return math.Round(got) == pinnedKnobDefault(knob)
	}
	return math.Abs(got-pinnedKnobDefault(knob)) <= schemaConfigPinEpsilon
}

// formatPinnedKnobDefault renders a knob's pinned default the way the pin
// helper does (String(value)) into the query parameter parseEffectConfig
// reads.
func formatPinnedKnobDefault(knob sim.Knob) string {
	if knob.Type == sim.KnobInt {
		return strconv.FormatInt(int64(math.Round(knob.Default)), 10)
	}
	return strconv.FormatFloat(knob.Default, 'g', -1, 64)
}
