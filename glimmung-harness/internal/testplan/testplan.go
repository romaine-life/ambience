// Package testplan is the typed port of scripts/glimmung-native/test-plan.sh:
// the read-only LLM phase that produces an evidence specification
// (issue-agent-test-plan.json) the verify phase later captures against. It runs
// in parallel with implement and never touches code.
//
// This is the GENERATED-plan path only. Feature types with a repo-versioned
// standing case (e.g. effect) skip this job at the PLATFORM level — the workflow
// registration's `when` gate means Glimmung never creates the pod.
//
// The deterministic plan-validation gates (planlint) are ambience's
// pre-verify-spend guard: a malformed or undecidable plan fails HERE, named,
// before any verify token is spent — the in-cluster analogue of spirelens's
// deterministic unit-test gate, applied to the model's emitted plan.
package testplan

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/romaine-life/ambience/glimmung-harness/internal/agentjob"
	"github.com/romaine-life/ambience/glimmung-harness/internal/naming"
	"github.com/romaine-life/ambience/glimmung-harness/internal/planlint"
	"github.com/romaine-life/ambience/glimmung-harness/internal/venue"
	"github.com/romaine-life/glimmung/harness/runcallbacks"
	"github.com/romaine-life/glimmung/harness/step"
)

const (
	planJSONPath  = "/tmp/test-plan.json"
	planMDPath    = "/tmp/test-plan.md"
	summaryMDPath = "/tmp/summary.md"
	exitCodePath  = "/tmp/test-plan-exit-code"
	proxyIPPath   = "/tmp/test-plan-proxy-ip"
)

// Registry returns the test-plan phase step registry.
func Registry() *step.Registry {
	return step.NewRegistry().Register(
		step.HandlerFunc{StepSlug: "clone", Fn: cloneRepo},
		step.HandlerFunc{StepSlug: "prepare", Fn: prepare},
		step.HandlerFunc{StepSlug: "run-test-plan", Fn: runTestPlan},
		step.HandlerFunc{StepSlug: "collect", Fn: collect},
		step.HandlerFunc{StepSlug: "finalize", Fn: finalize},
		step.HandlerFunc{StepSlug: "emit", Fn: emit},
	)
}

type config struct {
	repoSlug            string
	repoDir             string
	workflowRef         string
	claudeNamespace     string
	claudeCANamespace   string
	namespace           string
	validationURL       string
	jobName             string
	configMap           string
	agentContainerTag   string
	agentContainerImage string
	agentRuntimeJSON    string
	evidenceDir         string
}

func envOr(c *step.Context, key, def string) string {
	if v := strings.TrimSpace(c.Env(key)); v != "" {
		return v
	}
	return def
}

func newConfig(c *step.Context) (config, *step.LayeredError) {
	if strings.TrimSpace(c.RunID()) == "" {
		return config{}, step.HarnessError("missing_env", "GLIMMUNG_RUN_ID is not set", nil)
	}
	validationURL, err := c.Input("validation_url")
	if err != nil {
		return config{}, err.(*step.LayeredError)
	}
	namespace, err := c.Input("namespace")
	if err != nil {
		return config{}, err.(*step.LayeredError)
	}
	image, imgErr := naming.AgentContainerImage(c.Env("AGENT_CONTAINER_IMAGE"), c.Env("AGENT_CONTAINER_TAG"))
	if imgErr != nil {
		return config{}, step.HarnessError("agent_image", imgErr.Error(), nil)
	}
	runSlug := strings.ToLower(c.RunID())
	attempt := envOr(c, "GLIMMUNG_ATTEMPT_INDEX", "0")
	return config{
		repoSlug:            envOr(c, "AMBIENCE_REPO_SLUG", "romaine-life/ambience"),
		repoDir:             envOr(c, "AMBIENCE_REPO_DIR", "/workspace/ambience"),
		workflowRef:         naming.WorkflowCheckoutRef(c.Env("GLIMMUNG_RUN_INPUT_GIT_REF"), c.Env("AMBIENCE_WORKFLOW_REF")),
		claudeNamespace:     envOr(c, "GLIMMUNG_INPUT_CLAUDE_NAMESPACE", "tank-operator"),
		claudeCANamespace:   firstNonEmpty(c.Env("GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE"), c.Env("CLAUDE_CA_NAMESPACE"), "tank-operator-sessions"),
		namespace:           namespace,
		validationURL:       validationURL,
		jobName:             fmt.Sprintf("agent-%s-tp-%s", runSlug, attempt),
		configMap:           "agent-config-test-plan",
		agentContainerTag:   c.Env("AGENT_CONTAINER_TAG"),
		agentContainerImage: image,
		agentRuntimeJSON:    c.Env("GLIMMUNG_AGENT_RUNTIME_JSON"),
		evidenceDir:         envOr(c, "AMBIENCE_EVIDENCE_DIR", "/tmp/evidence"),
	}, nil
}

func cloneRepo(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return step.Result{}, lerr
	}
	g := venue.Git{Exec: venue.Exec{}, Token: token}
	if err := g.CloneRepo(c.RunContext(), cfg.repoSlug, cfg.repoDir, cfg.workflowRef, ""); err != nil {
		return step.Result{}, step.HostError("git_clone", "clone repo", err)
	}
	return step.Result{}, nil
}

func prepare(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	ex := venue.Exec{}
	if err := ex.AzureLogin(c.RunContext()); err != nil {
		return step.Result{}, step.HostError("azure_login", "azure login", err)
	}
	if err := ex.InstallPreviewPackage(c.RunContext(), cfg.repoDir+"/mcp"); err != nil {
		return step.Result{}, step.HostError("preview_pkg", "install ambience_preview", err)
	}
	prep := agentjob.Prep{Exec: ex, Namespace: cfg.namespace}
	if err := prep.CopyClaudeCA(c.RunContext(), cfg.claudeCANamespace); err != nil {
		return step.Result{}, step.HostError("copy_claude_ca", "copy provider CA into slot", err)
	}
	fmt.Printf("using Glimmung provider API proxy for Codex in %s; no raw codex-credentials Secret is created\n", cfg.namespace)
	proxyIP, err := prep.ResolveClusterIP(c.RunContext(), cfg.claudeNamespace, "claude-api-proxy")
	if err != nil {
		return step.Result{}, step.HostError("resolve_proxy", "resolve claude-api-proxy clusterIP", err)
	}
	_ = os.WriteFile(proxyIPPath, []byte(proxyIP+"\n"), 0o644)
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return step.Result{}, lerr
	}
	if err := prep.CreateGitHubTokenSecret(c.RunContext(), token); err != nil {
		return step.Result{}, step.HostError("github_secret", "stage agent github token", err)
	}
	if err := prep.CreateConfigMap(c.RunContext(), cfg.configMap, []string{
		"prompt-test-plan.md=" + cfg.repoDir + "/.github/agent/prompt-test-plan.md",
	}); err != nil {
		return step.Result{}, step.HostError("configmap", "stage prompt configmap", err)
	}
	return step.Result{}, nil
}

func (cfg config) ensureProxyIP(c *step.Context) (string, *step.LayeredError) {
	if data, err := os.ReadFile(proxyIPPath); err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data)), nil
	}
	prep := agentjob.Prep{Exec: venue.Exec{}, Namespace: cfg.namespace}
	ip, err := prep.ResolveClusterIP(c.RunContext(), cfg.claudeNamespace, "claude-api-proxy")
	if err != nil {
		return "", step.HostError("resolve_proxy", "resolve claude-api-proxy clusterIP", err)
	}
	_ = os.WriteFile(proxyIPPath, []byte(ip+"\n"), 0o644)
	return ip, nil
}

func runTestPlan(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	proxyIP, lerr := cfg.ensureProxyIP(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	spawn := agentjob.Spawn{
		Exec:                venue.Exec{},
		RepoDir:             cfg.repoDir,
		Namespace:           cfg.namespace,
		JobName:             cfg.jobName,
		Stage:               "test-plan",
		ConfigMap:           cfg.configMap,
		Issue:               agentjob.DeriveIssueContext(c),
		ValidationURL:       cfg.validationURL,
		BranchName:          "glimmung/" + c.RunID(),
		ProxyIP:             proxyIP,
		AgentContainerTag:   cfg.agentContainerTag,
		AgentContainerImage: cfg.agentContainerImage,
		RepoSlug:            cfg.repoSlug,
		AgentRuntimeJSON:    cfg.agentRuntimeJSON,
		MarkerIntent:        "helper",
		MarkerLabel:         "test-plan-agent",
	}
	// A failure to even render/create the agent Job is a venue (host) failure —
	// attribute it honestly rather than laundering it through the plan exit code.
	if err := spawn.Apply(c.RunContext()); err != nil {
		return step.Result{}, step.HostError("apply_agent_job", "render test-plan agent job", err)
	}
	timeout := envOr(c, "AGENT_TEST_PLAN_TIMEOUT_SECONDS", "900")
	// A nonzero agent Job is the model failing to deliver — record it and let
	// finalize/emit produce the named verdict (the $0-misattribution boundary).
	if err := spawn.Wait(c.RunContext(), timeout); err != nil {
		_ = venue.WriteExitCode(exitCodePath, 1)
		fmt.Fprintf(os.Stderr, "test-plan agent job failed: %v\n", err)
		return step.Result{}, nil
	}
	_ = venue.WriteExitCode(exitCodePath, 0)
	return step.Result{}, nil
}

func collect(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	prep := agentjob.Prep{Exec: venue.Exec{}, Namespace: cfg.namespace}
	podLog, _ := prep.CollectPodLog(c.RunContext(), cfg.jobName)
	if podLog == "" {
		fmt.Printf("no pod found for job %s; skipping evidence capture\n", cfg.jobName)
		return step.Result{}, nil
	}
	if err := venue.ExtractEvidenceTar(podLog, cfg.evidenceDir); err != nil {
		fmt.Fprintf(os.Stderr, "evidence tarball extraction failed; continuing: %v\n", err)
	}
	return step.Result{}, nil
}

func finalize(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	copyIfPresent(cfg.evidenceDir+"/issue-agent-test-plan.json", planJSONPath)
	if copyIfPresent(cfg.evidenceDir+"/issue-agent-test-plan.md", planMDPath) {
		copyIfPresent(planMDPath, summaryMDPath)
	}

	data, _ := os.ReadFile(planJSONPath)
	if len(strings.TrimSpace(string(data))) == 0 {
		reason := "test-plan pod produced no output"
		if venue.ReadExitCode(exitCodePath) != 0 {
			reason = fmt.Sprintf("test-plan pod exited with %d", venue.ReadExitCode(exitCodePath))
		}
		writeJSON(planJSONPath, map[string]any{"schema_version": 1, "status": "fail", "abort_reason": reason})
		return step.Result{}, nil
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err == nil && status(doc) == "pass" {
		outcome := planlint.Lint(data)
		if outcome.Pass {
			_ = os.WriteFile(planJSONPath, outcome.Normalized, 0o644)
		} else {
			writeJSON(planJSONPath, outcome.Verdict)
			_ = venue.WriteExitCode(exitCodePath, 1)
		}
	}
	return step.Result{}, nil
}

func emit(c *step.Context) (step.Result, error) {
	data, _ := os.ReadFile(planJSONPath)
	testPlan := strings.TrimSpace(string(data))
	caseCount := planlint.CaseCount(data)
	if err := c.EmitOutput("test_plan", testPlan); err != nil {
		return step.Result{}, err
	}
	if err := c.EmitOutput("test_cases_count", strconv.Itoa(caseCount)); err != nil {
		return step.Result{}, err
	}
	if summary, err := os.ReadFile(summaryMDPath); err == nil {
		c.SetSummaryMarkdown(strings.TrimSpace(string(summary)))
	}
	if code := venue.ReadExitCode(exitCodePath); code != 0 {
		return step.Result{}, step.ModelError("test_plan_rejected",
			fmt.Sprintf("test-plan phase failed (exit %d); see emitted test_plan verdict", code), nil)
	}
	return step.Result{}, nil
}

func status(doc map[string]any) string {
	if s, ok := doc["status"].(string); ok {
		return s
	}
	return ""
}

func copyIfPresent(src, dst string) bool {
	data, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	return os.WriteFile(dst, data, 0o644) == nil
}

func writeJSON(path string, v any) {
	b, _ := json.Marshal(v)
	_ = os.WriteFile(path, append(b, '\n'), 0o644)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
