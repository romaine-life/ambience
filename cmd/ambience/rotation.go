// Cross-effect rotation policy for the shared atmosphere.
//
// PR #110 (issue #8) shipped the client-side crossfade — when an SSE
// snapshot arrives whose `type` differs from the running effect, the old
// sim fades into the new one. But ambience itself never broadcasts such a
// snapshot today: the dropdown soft-switch on /dev was the only trigger.
// This file is the authority-side half — it picks a new effect every
// `cadenceTicks` and broadcasts a fresh snapshot so the live monitor at /
// crossfades on its own without a human in the loop.
package main

import (
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/romaine-life/ambience/rngutil"
)

// defaultRotationCadenceTicks is 10 minutes at 10 Hz — the middle of the
// 5–15 minute window the issue called out as "long enough to feel coherent,
// short enough to see variety."
const defaultRotationCadenceTicks = 6000

type rotationPolicy struct {
	Enabled      bool
	CadenceTicks int
	// Allowed is the configured pool of effect types. Empty means
	// "everything in the registry" — resolvedAllowedEffects() applies that
	// fallback at lookup time so a registry add/remove takes effect without
	// changing the policy.
	Allowed []string
}

func loadRotationPolicyFromEnv() rotationPolicy {
	p := rotationPolicy{
		Enabled:      true,
		CadenceTicks: defaultRotationCadenceTicks,
	}
	if raw := strings.TrimSpace(os.Getenv("AMBIENCE_ROTATION_ENABLED")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			log.Printf("rotation: invalid AMBIENCE_ROTATION_ENABLED %q; defaulting to true", raw)
		} else {
			p.Enabled = v
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AMBIENCE_ROTATION_CADENCE")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			log.Printf("rotation: invalid AMBIENCE_ROTATION_CADENCE %q; using default %s", raw, time.Duration(defaultRotationCadenceTicks)*tickRate)
		} else {
			ticks := int(d / tickRate)
			if ticks < 1 {
				ticks = 1
			}
			p.CadenceTicks = ticks
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AMBIENCE_ROTATION_EFFECTS")); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			name := strings.TrimSpace(strings.ToLower(item))
			if name != "" {
				p.Allowed = append(p.Allowed, name)
			}
		}
	}
	return p
}

// resolvedAllowedEffects expands the configured pool against the registry,
// dropping unknown entries. With no configured pool, returns every
// registered effect. Result is sorted so picks stay deterministic given the
// same RNG state across restarts.
func (p rotationPolicy) resolvedAllowedEffects() []string {
	if len(p.Allowed) > 0 {
		out := make([]string, 0, len(p.Allowed))
		for _, name := range p.Allowed {
			if _, ok := lookupEffectDefinition(name); ok {
				out = append(out, name)
			} else {
				log.Printf("rotation: ignoring unknown effect %q", name)
			}
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(effectRegistry))
	for name := range effectRegistry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// pickNextEffect chooses an effect from pool, preferring not to repeat the
// current one. Returns current unchanged when no other option exists, so
// callers can detect a no-op rotation and just slide the timer forward.
func pickNextEffect(rng *rngutil.RNG, pool []string, current string) string {
	if len(pool) == 0 {
		return current
	}
	candidates := make([]string, 0, len(pool))
	for _, name := range pool {
		if name != current {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return current
	}
	return candidates[rng.Intn(len(candidates))]
}
