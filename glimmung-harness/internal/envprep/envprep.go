// Package envprep is the typed port of scripts/glimmung-native/env-prep.sh: the
// prepare phase that builds, deploys, and health-checks the per-run validation
// environment, then emits its coordinates as phase outputs for the downstream
// llm-work and verify phases.
//
// The venue is in-cluster: every operation runs against the current cluster via
// kubectl/helm and the ambience_preview Python CLI (image build + helm deploy).
// There is no remote host. Cross-step state (the resolved image tag, the
// preview image reference) lives on the shared pod filesystem under /tmp,
// exactly as the shell harness passed it between managed per-step invocations.
package envprep

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/romaine-life/ambience/glimmung-harness/internal/naming"
	"github.com/romaine-life/ambience/glimmung-harness/internal/venue"
	"github.com/romaine-life/glimmung/harness/runcallbacks"
	"github.com/romaine-life/glimmung/harness/step"
)

const (
	imageJSONPath = "/tmp/ambience-preview-image.json"
	imageFilePath = "/tmp/ambience-preview-image.txt"
)

// Registry returns the env-prep phase step registry.
func Registry() *step.Registry {
	return step.NewRegistry().Register(
		step.HandlerFunc{StepSlug: "clone-repo", Fn: cloneRepo},
		step.HandlerFunc{StepSlug: "build-validation-image", Fn: buildValidationImage},
		step.HandlerFunc{StepSlug: "push-validation-image", Fn: pushValidationImage},
		step.HandlerFunc{StepSlug: "reap-slot-conflicts", Fn: reapSlotConflicts},
		step.HandlerFunc{StepSlug: "deploy-validation-env", Fn: deployValidationEnv},
		step.HandlerFunc{StepSlug: "check-validation-env", Fn: checkValidationEnv},
		step.HandlerFunc{StepSlug: "emit-env-outputs", Fn: emitEnvOutputs},
	)
}

type config struct {
	repoSlug          string
	repoDir           string
	workflowRef       string
	claudeNamespace   string
	claudeCANamespace string
	namespace         string
	slotIndex         string
	preprovisioned    bool
	validationHost    string
	releaseName       string
	validationURL     string
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
		repoSlug:          envOr(c, "AMBIENCE_REPO_SLUG", "romaine-life/ambience"),
		repoDir:           envOr(c, "AMBIENCE_REPO_DIR", "/workspace/ambience"),
		workflowRef:       naming.WorkflowCheckoutRef(c.Env("GLIMMUNG_RUN_INPUT_GIT_REF"), c.Env("AMBIENCE_WORKFLOW_REF")),
		claudeNamespace:   envOr(c, "CLAUDE_NAMESPACE", "tank-operator"),
		claudeCANamespace: envOr(c, "CLAUDE_CA_NAMESPACE", "tank-operator-sessions"),
		namespace:         ns,
		slotIndex:         strings.TrimSpace(c.Env("GLIMMUNG_RUNNER_SLOT_INDEX")),
	}
	if cfg.slotIndex != "" {
		cfg.preprovisioned = true
		cfg.validationHost = envOr(c, "AMBIENCE_STANDBY_HOST_PREFIX", "ambience-slot-") + cfg.slotIndex + ".ambience.dev.romaine.life"
		cfg.releaseName = envOr(c, "AMBIENCE_VALIDATION_RELEASE", ns+"-hot")
	} else {
		cfg.validationHost = ns + ".ambience.dev.romaine.life"
		cfg.releaseName = envOr(c, "AMBIENCE_VALIDATION_RELEASE", "ambience-agent")
	}
	cfg.validationURL = "https://" + cfg.validationHost
	return cfg, nil
}

func (cfg config) git(c *step.Context) (venue.Git, *step.LayeredError) {
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return venue.Git{}, lerr
	}
	return venue.Git{Exec: venue.Exec{}, Token: token}, nil
}

// resolveBaseImage reads the cloned repo's HEAD and derives the validation image
// tag, mirroring resolve_base_image_identity. It clones first if the working
// tree is absent (mirrors the shell's lazy clone).
func (cfg config) resolveBaseImage(c *step.Context) (baseRevision, imageTag string, lerr *step.LayeredError) {
	ex := venue.Exec{}
	if _, err := os.Stat(cfg.repoDir + "/.git"); err != nil {
		if _, cloneErr := cloneRepo(c); cloneErr != nil {
			return "", "", asLayered(cloneErr)
		}
	}
	rev, err := ex.Output(c.RunContext(), cfg.repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", step.HostError("git_rev_parse", "resolve repo HEAD", err)
	}
	tag, err := naming.ImageTagForRevision(rev)
	if err != nil {
		return "", "", step.HarnessError("image_tag", err.Error(), nil)
	}
	return rev, tag, nil
}

func asLayered(err error) *step.LayeredError {
	if le, ok := err.(*step.LayeredError); ok {
		return le
	}
	if err == nil {
		return nil
	}
	return step.HarnessError("internal", err.Error(), err)
}

func cloneRepo(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if err := g.CloneRepo(c.RunContext(), cfg.repoSlug, cfg.repoDir, cfg.workflowRef, ""); err != nil {
		return step.Result{}, step.HostError("git_clone", "clone repo", err)
	}
	return step.Result{}, nil
}

func buildValidationImage(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.AzureLogin(c.RunContext()); err != nil {
		return step.Result{}, step.HostError("azure_login", "azure workload-identity login", err)
	}
	if err := ex.InstallPreviewPackage(c.RunContext(), cfg.repoDir+"/mcp"); err != nil {
		return step.Result{}, step.HostError("preview_pkg", "install ambience_preview", err)
	}
	baseRevision, imageTag, lerr := cfg.resolveBaseImage(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	out, err := ex.PreviewCLIOutput(c.RunContext(), cfg.repoDir,
		"build-preview-image", "--image-tag", imageTag, "--source-revision", baseRevision)
	if err != nil {
		return step.Result{}, step.HostError("build_preview_image", "build preview image", err)
	}
	if err := os.WriteFile(imageJSONPath, []byte(out+"\n"), 0o644); err != nil {
		return step.Result{}, step.HarnessError("write_image_json", "persist image json", err)
	}
	var doc struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || strings.TrimSpace(doc.Image) == "" {
		return step.Result{}, step.HostError("build_preview_image", "build-preview-image did not return an .image", err)
	}
	if err := os.WriteFile(imageFilePath, []byte(doc.Image+"\n"), 0o644); err != nil {
		return step.Result{}, step.HarnessError("write_image_file", "persist image ref", err)
	}
	return step.Result{}, nil
}

func pushValidationImage(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.AzureLogin(c.RunContext()); err != nil {
		return step.Result{}, step.HostError("azure_login", "azure workload-identity login", err)
	}
	_, imageTag, lerr := cfg.resolveBaseImage(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	got, err := ex.Output(c.RunContext(), "", "az", "acr", "repository", "show-tags",
		"--name", "romainecr", "--repository", "ambience",
		"--query", fmt.Sprintf("[?@=='%s'] | [0]", imageTag), "--output", "tsv")
	if err != nil {
		return step.Result{}, step.HostError("acr_show_tags", "query ACR for image tag", err)
	}
	if strings.TrimSpace(got) != imageTag {
		return step.Result{}, step.HostError("image_missing",
			fmt.Sprintf("image tag %s was not found in romainecr.azurecr.io/ambience", imageTag), nil)
	}
	fmt.Printf("verified romainecr.azurecr.io/ambience:%s\n", imageTag)
	return step.Result{}, nil
}

func ensureImageFile(c *step.Context, cfg config) (string, *step.LayeredError) {
	if data, err := os.ReadFile(imageFilePath); err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data)), nil
	}
	_, imageTag, lerr := cfg.resolveBaseImage(c)
	if lerr != nil {
		return "", lerr
	}
	ref := "romainecr.azurecr.io/ambience:" + imageTag
	_ = os.WriteFile(imageFilePath, []byte(ref+"\n"), 0o644)
	return ref, nil
}

type httproute struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Hostnames []string `json:"hostnames"`
	} `json:"spec"`
}

// reapSlotConflicts mirrors reap_conflicting_slot: when running on a
// pre-provisioned slot, delete any peer glim-run-* namespace whose HTTPRoute
// claims our slot host before we install, so Envoy never routes the slot host at
// a stale image.
func reapSlotConflicts(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if cfg.slotIndex == "" {
		fmt.Println("no slot index set; nothing to reap")
		return step.Result{}, nil
	}
	ex := venue.Exec{}
	out, err := ex.Output(c.RunContext(), "", "kubectl", "get", "httproute", "--all-namespaces", "-o", "json")
	if err != nil {
		return step.Result{}, step.HostError("kubectl_get_httproute", "list httproutes", err)
	}
	var list struct {
		Items []httproute `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return step.Result{}, step.HarnessError("parse_httproute", "parse httproute json", err)
	}
	reaped := 0
	for _, r := range list.Items {
		if r.Metadata.Namespace == cfg.namespace || !containsStr(r.Spec.Hostnames, cfg.validationHost) {
			continue
		}
		if !strings.HasPrefix(r.Metadata.Namespace, "glim-run-") {
			fmt.Fprintf(os.Stderr, "skipping reap of %s/%s: not a glim-run-* namespace\n", r.Metadata.Namespace, r.Metadata.Name)
			continue
		}
		fmt.Printf("reaping %s claimant %s/%s\n", cfg.validationHost, r.Metadata.Namespace, r.Metadata.Name)
		_ = ex.Run(c.RunContext(), "", "kubectl", "delete", "httproute", r.Metadata.Name, "--namespace", r.Metadata.Namespace, "--ignore-not-found=true")
		_ = ex.Run(c.RunContext(), "", "helm", "uninstall", cfg.releaseName, "--namespace", r.Metadata.Namespace)
		_ = ex.Run(c.RunContext(), "", "kubectl", "delete", "namespace", r.Metadata.Namespace, "--ignore-not-found=true", "--wait=false")
		reaped++
	}
	if reaped == 0 {
		fmt.Printf("no peer claims %s\n", cfg.validationHost)
	}
	return step.Result{}, nil
}

func deployValidationEnv(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.InstallPreviewPackage(c.RunContext(), cfg.repoDir+"/mcp"); err != nil {
		return step.Result{}, step.HostError("preview_pkg", "install ambience_preview", err)
	}
	// Re-verify the image is present (push_validation_image) before deploy.
	if _, err := pushValidationImage(c); err != nil {
		return step.Result{}, err
	}
	image, lerr := ensureImageFile(c, cfg)
	if lerr != nil {
		return step.Result{}, lerr
	}
	args := []string{
		"deploy-validation-preview",
		"--namespace", cfg.namespace,
		"--image", image,
		"--release", cfg.releaseName,
		"--public-host", cfg.validationHost,
		"--no-create-namespace",
	}
	if cfg.preprovisioned {
		args = append(args, "--skip-external-dns", "--render-mode", "hot", "--test-env-slot-name", cfg.namespace)
	}
	if err := ex.PreviewCLI(c.RunContext(), cfg.repoDir, args...); err != nil {
		return step.Result{}, step.HostError("deploy_validation_preview", "deploy validation env", err)
	}
	return step.Result{}, nil
}

func checkValidationEnv(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.InstallPreviewPackage(c.RunContext(), cfg.repoDir+"/mcp"); err != nil {
		return step.Result{}, step.HostError("preview_pkg", "install ambience_preview", err)
	}
	if err := ex.PreviewCLI(c.RunContext(), cfg.repoDir, "wait-public-preview", "--url", cfg.validationURL); err != nil {
		return step.Result{}, step.HostError("wait_public_preview", "validation env did not become ready", err)
	}
	return step.Result{}, nil
}

func emitEnvOutputs(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	baseRevision, imageTag, lerr := cfg.resolveBaseImage(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	return step.Result{Outputs: map[string]string{
		"validation_url":        cfg.validationURL,
		"validation_slot_index": cfg.slotIndex,
		"namespace":             cfg.namespace,
		"image_tag":             imageTag,
		"base_revision":         baseRevision,
		"claude_namespace":      cfg.claudeNamespace,
		"claude_ca_namespace":   cfg.claudeCANamespace,
	}}, nil
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
