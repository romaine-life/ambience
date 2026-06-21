// Package envdestroy is the typed port of scripts/glimmung-native/env-destroy.sh:
// the always-run teardown phase that runs after the verification gate however it
// resolved, so a failed verify-loop never leaves its slot namespace claiming the
// public hostname. It is idempotent: a missing helm release or namespace is
// logged and skipped, never an error.
package envdestroy

import (
	"fmt"
	"os"
	"strings"

	"github.com/romaine-life/ambience/glimmung-harness/internal/naming"
	"github.com/romaine-life/ambience/glimmung-harness/internal/venue"
	"github.com/romaine-life/glimmung/harness/runcallbacks"
	"github.com/romaine-life/glimmung/harness/step"
)

// Registry returns the env-destroy phase step registry.
func Registry() *step.Registry {
	return step.NewRegistry().Register(
		step.HandlerFunc{StepSlug: "describe-pre-teardown", Fn: describePreTeardown},
		step.HandlerFunc{StepSlug: "uninstall-helm-release", Fn: uninstallHelmRelease},
		step.HandlerFunc{StepSlug: "delete-namespace", Fn: deleteNamespace},
		step.HandlerFunc{StepSlug: "cleanup-issue-branches", Fn: cleanupIssueBranches},
		step.HandlerFunc{StepSlug: "emit", Fn: emit},
	)
}

type config struct {
	repoSlug       string
	namespace      string
	slotIndex      string
	preprovisioned bool
	releaseName    string
}

func envOr(c *step.Context, key, def string) string {
	if v := strings.TrimSpace(c.Env(key)); v != "" {
		return v
	}
	return def
}

func newConfig(c *step.Context) (config, *step.LayeredError) {
	ns := strings.TrimSpace(c.Env("GLIMMUNG_VALIDATION_NAMESPACE"))
	if ns == "" {
		return config{}, step.HarnessError("missing_env", "GLIMMUNG_VALIDATION_NAMESPACE is not set", nil)
	}
	if strings.TrimSpace(c.RunID()) == "" {
		return config{}, step.HarnessError("missing_env", "GLIMMUNG_RUN_ID is not set", nil)
	}
	cfg := config{
		repoSlug:  envOr(c, "AMBIENCE_REPO_SLUG", "romaine-life/ambience"),
		namespace: ns,
		slotIndex: strings.TrimSpace(c.Env("GLIMMUNG_RUNNER_SLOT_INDEX")),
	}
	if cfg.slotIndex != "" {
		cfg.preprovisioned = true
		cfg.releaseName = envOr(c, "AMBIENCE_VALIDATION_RELEASE", ns+"-hot")
	} else {
		cfg.releaseName = envOr(c, "AMBIENCE_VALIDATION_RELEASE", "ambience-agent")
	}
	return cfg, nil
}

func describePreTeardown(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	fmt.Printf("namespace: %s\nrelease:   %s\n", cfg.namespace, cfg.releaseName)
	fmt.Println("--- helm releases in namespace ---")
	_ = ex.Run(c.RunContext(), "", "helm", "list", "--namespace", cfg.namespace)
	fmt.Println("--- httproutes in namespace ---")
	_ = ex.Run(c.RunContext(), "", "kubectl", "get", "httproute", "--namespace", cfg.namespace)
	fmt.Println("--- pods in namespace ---")
	_ = ex.Run(c.RunContext(), "", "kubectl", "get", "pods", "--namespace", cfg.namespace)
	return step.Result{}, nil
}

// uninstallHelmRelease is idempotent and best-effort (the shell ran it as
// native_step_allow_failure): a missing namespace or release is logged, never
// an error.
func uninstallHelmRelease(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.Run(c.RunContext(), "", "kubectl", "get", "namespace", cfg.namespace); err != nil {
		fmt.Printf("namespace %s already gone; nothing to uninstall\n", cfg.namespace)
		return step.Result{}, nil
	}
	if err := ex.Run(c.RunContext(), "", "helm", "status", cfg.releaseName, "--namespace", cfg.namespace); err != nil {
		fmt.Printf("helm release %s not found in %s; nothing to uninstall\n", cfg.releaseName, cfg.namespace)
		return step.Result{}, nil
	}
	if err := ex.Run(c.RunContext(), "", "helm", "uninstall", cfg.releaseName, "--namespace", cfg.namespace, "--wait=false"); err != nil {
		fmt.Fprintf(os.Stderr, "helm uninstall failed (continuing teardown): %v\n", err)
	}
	return step.Result{}, nil
}

// deleteNamespace is best-effort; pre-provisioned slots keep their warm
// namespace.
func deleteNamespace(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if cfg.preprovisioned {
		fmt.Printf("pre-provisioned slot namespace %s; leaving warm resources in place\n", cfg.namespace)
		return step.Result{}, nil
	}
	if err := (venue.Exec{}).Run(c.RunContext(), "", "kubectl", "delete", "namespace", cfg.namespace, "--ignore-not-found=true", "--wait=false"); err != nil {
		fmt.Fprintf(os.Stderr, "namespace delete failed (continuing teardown): %v\n", err)
	}
	return step.Result{}, nil
}

// cleanupIssueBranches deletes implementation branches under the issue prefix,
// but only once a PR under that prefix has merged (mirrors the shell's refusal
// to delete unmerged work). Best-effort: teardown never blocks on it.
func cleanupIssueBranches(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if strings.TrimSpace(c.Phase()) != "cleanup_final" {
		fmt.Printf("phase %s is not cleanup_final; skipping issue branch cleanup\n", orUnknown(c.Phase()))
		return step.Result{}, nil
	}
	prefix, err := naming.IssueBranchPrefix(c.Env("GLIMMUNG_ISSUE_NUMBER"))
	if err != nil {
		fmt.Println("GLIMMUNG_ISSUE_NUMBER is not set; skipping issue branch cleanup")
		return step.Result{}, nil
	}
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return step.Result{}, lerr
	}
	g := venue.Git{Exec: venue.Exec{}, Token: token}
	gh := venue.GitHub{Token: token}
	owner, repo := splitSlug(cfg.repoSlug)

	branches, err := g.ListRemoteHeads(c.RunContext(), cfg.repoSlug, prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list issue branches failed (continuing teardown): %v\n", err)
		return step.Result{}, nil
	}
	if len(branches) == 0 {
		fmt.Printf("no issue branches found for %s\n", prefix)
		return step.Result{}, nil
	}
	mergedSeen := false
	for _, b := range branches {
		if merged, err := gh.BranchHasMergedPR(c.RunContext(), owner, repo, b); err == nil && merged {
			mergedSeen = true
			break
		}
	}
	if !mergedSeen {
		fmt.Printf("no merged PR found under %s; refusing to delete implementation branches\n", prefix)
		return step.Result{}, nil
	}
	fmt.Printf("deleting %d issue branch(es) under %s\n", len(branches), prefix)
	for _, b := range branches {
		fmt.Printf("deleting %s\n", b)
		if err := g.DeleteRemoteBranch(c.RunContext(), cfg.repoSlug, b); err != nil {
			fmt.Fprintf(os.Stderr, "delete branch %s failed (continuing): %v\n", b, err)
		}
	}
	return step.Result{}, nil
}

func emit(c *step.Context) (step.Result, error) {
	return step.Result{}, nil
}

func splitSlug(slug string) (string, string) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return slug, ""
	}
	return parts[0], parts[1]
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}
