// Command glimmung-ambience is the single-faced, in-cluster run-harness binary
// for ambience's Glimmung workflow. It replaces the retired
// scripts/glimmung-native/*.sh fork end to end.
//
// Unlike spirelens (one global pod registry of unique slugs), ambience's phases
// reuse slugs (clone/prepare/finalize/emit recur across phases with different
// behavior), so dispatch is two-level: the first argument selects the PHASE
// registry, then GLIMMUNG_STEP_SLUG selects the step within it. The workflow
// re-registration binds each (phase, slug) to:
//
//	cd glimmung-harness && go build -o /tmp/glimmung-ambience ./cmd/glimmung-ambience \
//	  && exec /tmp/glimmung-ambience <phase> <slug>
//
// There is no host face and no remote venue: the model runs in a child k8s Job
// (see internal/agentjob), not as an in-pod child of the harness.
package main

import (
	"fmt"
	"os"

	"github.com/romaine-life/ambience/glimmung-harness/internal/envdestroy"
	"github.com/romaine-life/ambience/glimmung-harness/internal/envprep"
	"github.com/romaine-life/ambience/glimmung-harness/internal/implement"
	"github.com/romaine-life/ambience/glimmung-harness/internal/testplan"
	"github.com/romaine-life/glimmung/harness/step"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: glimmung-ambience <env-prep|test-plan|implement|env-destroy> [step-slug]")
		os.Exit(2)
	}
	phase := args[0]
	// Belt-and-suspenders: an explicit slug arg overrides GLIMMUNG_STEP_SLUG
	// (the runner normally sets the env), mirroring spirelens's `pod <slug>`.
	if len(args) >= 2 && args[1] != "" {
		_ = os.Setenv("GLIMMUNG_STEP_SLUG", args[1])
	}
	registry, ok := registryFor(phase)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown phase %q (want env-prep|test-plan|implement|env-destroy)\n", phase)
		os.Exit(2)
	}
	step.Main(registry)
}

// registryFor maps a phase name to its step registry.
func registryFor(phase string) (*step.Registry, bool) {
	switch phase {
	case "env-prep":
		return envprep.Registry(), true
	case "test-plan":
		return testplan.Registry(), true
	case "implement":
		return implement.Registry(), true
	case "env-destroy":
		return envdestroy.Registry(), true
	default:
		return nil, false
	}
}
