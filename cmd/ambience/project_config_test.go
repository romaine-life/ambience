package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The committed .glimmung/project.yaml is the declarative source of truth for
// ambience's durable Glimmung project row; the reconcile workflow syncs it on
// merge. Because a sync REPLACES authored config wholesale, the file must stay
// complete — dropping a field here would wipe it from the runtime row. This
// guard fails if the authored config loses a load-bearing block or the
// static hot-swap contract.
//
// It is a deliberately shallow byte check (no YAML dep, no glimmung import):
// the authoritative validation is glimmung's own parseProjectYAML, exercised
// by check_project_updates / the reconcile job. This just stops an obvious
// regression from landing.
func TestGlimmungProjectConfigStaysComplete(t *testing.T) {
	path := filepath.Join("..", "..", ".glimmung", "project.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)

	for _, want := range []string{
		"name: ambience",
		"github_repo: romaine-life/ambience",
		// Authored config that a wholesale sync would otherwise drop.
		"native_standby_dns:",
		"test_slot_helm:",
		// The static hot-swap contract this file exists to register.
		"test_slot_hot_swap:",
		"static:",
		"source: cmd/ambience/web",
		"target: /var/run/ambience-web-override",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf(".glimmung/project.yaml missing %q — a wholesale sync would drop it from the durable row", want)
		}
	}

	// Reconciler-owned status must never be AUTHORED here as a config key (the
	// sync strips it, but committing it is a category error). Scan for it as a
	// real mapping key, ignoring the explanatory comment that names these
	// examples.
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, forbidden := range []string{
			"managed_auth_origin_status",
			"native_standby_workload_identity_status",
		} {
			if strings.HasPrefix(trimmed, forbidden+":") {
				t.Fatalf(".glimmung/project.yaml authors reconciler-owned status key %q; remove it", forbidden)
			}
		}
	}
}
