// Package implement is the typed port of scripts/glimmung-native/implement.sh:
// the code-editing LLM phase. It opens a draft PR so CI is visible during the
// agent loop, spawns the implementation agent inner Job, confirms the pushed
// branch, validates the implementation contract, waits the deterministic PR-CI
// gate, rebuilds the validation env from the new commit, enforces the ui_hint
// discovery contract, and emits the implementation + branch + PR + ui_hint
// phase outputs.
//
// The deterministic gates that protect downstream verify spend are the
// implementation-contract validation (the model's changed files must match the
// effect touchpoints) and the ui_hint gate (a passing effect implementation
// must declare its discovery hint). Both fail the phase, named, BEFORE verify —
// ambience's equivalent of the spirelens pre-agent gate, applied to the model's
// output.
package implement

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/romaine-life/ambience/glimmung-harness/internal/agentjob"
	"github.com/romaine-life/ambience/glimmung-harness/internal/contract"
	"github.com/romaine-life/ambience/glimmung-harness/internal/naming"
	"github.com/romaine-life/ambience/glimmung-harness/internal/venue"
	"github.com/romaine-life/glimmung/harness/runcallbacks"
	"github.com/romaine-life/glimmung/harness/step"
)

const (
	implementationJSONPath = "/tmp/implementation.json"
	summaryMDPath          = "/tmp/summary.md"
	exitCodePath           = "/tmp/implementation-exit-code"
	proxyIPPath            = "/tmp/implementation-proxy-ip"
	githubProxyIPPath      = "/tmp/implementation-github-proxy-ip"
	githubEgressIPPath     = "/tmp/implementation-github-egress-ip"
	prNumberPath           = "/tmp/implementation-pr-number"
	prURLPath              = "/tmp/implementation-pr-url"
)

// Registry returns the implement phase step registry.
func Registry() *step.Registry {
	return step.NewRegistry().Register(
		step.HandlerFunc{StepSlug: "clone", Fn: cloneRepo},
		step.HandlerFunc{StepSlug: "prepare", Fn: prepare},
		step.HandlerFunc{StepSlug: "prepare-draft-pr-branch", Fn: prepareDraftPRBranch},
		step.HandlerFunc{StepSlug: "ensure-draft-pr", Fn: ensureDraftPR},
		step.HandlerFunc{StepSlug: "run-implementation", Fn: runImplementation},
		step.HandlerFunc{StepSlug: "collect", Fn: collect},
		step.HandlerFunc{StepSlug: "push-branch", Fn: pushBranch},
		step.HandlerFunc{StepSlug: "wait-pr-checks", Fn: waitPRChecks},
		step.HandlerFunc{StepSlug: "rebuild-env", Fn: rebuildEnv},
		step.HandlerFunc{StepSlug: "finalize", Fn: finalize},
		step.HandlerFunc{StepSlug: "emit", Fn: emit},
	)
}

type config struct {
	repoSlug                  string
	repoDir                   string
	workflowRef               string
	claudeNamespace           string
	claudeCANamespace         string
	providerAPIProxyNamespace string
	githubPolicyCASecret      string
	githubPolicyCAConfigMap   string
	githubPolicyProxyService  string
	agentEgressGatewayName    string
	agentEgressGatewayNS      string
	envoyGatewaySystemNS      string
	validationURL             string
	namespace                 string
	imageTag                  string
	branchName                string
	jobName                   string
	configMap                 string
	agentContainerTag         string
	agentContainerImage       string
	agentRuntimeJSON          string
	featureType               string
	evidenceDir               string
	prBase                    string
	issue                     agentjob.IssueContext
}

func envOr(c *step.Context, key, def string) string {
	if v := strings.TrimSpace(c.Env(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func newConfig(c *step.Context) (config, *step.LayeredError) {
	if strings.TrimSpace(c.RunID()) == "" {
		return config{}, step.HarnessError("missing_env", "GLIMMUNG_RUN_ID is not set", nil)
	}
	validationURL, err := c.Input("validation_url")
	if err != nil {
		return config{}, toLayered(err)
	}
	namespace, err := c.Input("namespace")
	if err != nil {
		return config{}, toLayered(err)
	}
	imageTag, err := c.Input("image_tag")
	if err != nil {
		return config{}, toLayered(err)
	}
	image, imgErr := naming.AgentContainerImage(c.Env("AGENT_CONTAINER_IMAGE"), c.Env("AGENT_CONTAINER_TAG"))
	if imgErr != nil {
		return config{}, step.HarnessError("agent_image", imgErr.Error(), nil)
	}
	branch, brErr := naming.ImplementationBranchName(naming.BranchInputs{
		AmbienceImplementationBranch: c.Env("AMBIENCE_IMPLEMENTATION_BRANCH"),
		IssueNumber:                  c.Env("GLIMMUNG_ISSUE_NUMBER"),
		RunID:                        c.RunID(),
		WorkContextBranch:            c.Env("GLIMMUNG_WORK_CONTEXT_BRANCH"),
	})
	if brErr != nil {
		return config{}, step.HarnessError("branch_name", brErr.Error(), nil)
	}
	providerNS := firstNonEmpty(c.Env("GLIMMUNG_INPUT_PROVIDER_API_PROXY_NAMESPACE"), c.Env("GLIMMUNG_PROVIDER_API_PROXY_NAMESPACE"), "glimmung-runs")
	runSlug := strings.ToLower(c.RunID())
	attempt := envOr(c, "GLIMMUNG_ATTEMPT_INDEX", "0")
	return config{
		repoSlug:                  envOr(c, "AMBIENCE_REPO_SLUG", "romaine-life/ambience"),
		repoDir:                   envOr(c, "AMBIENCE_REPO_DIR", "/workspace/ambience"),
		workflowRef:               naming.WorkflowCheckoutRef(c.Env("GLIMMUNG_RUN_INPUT_GIT_REF"), c.Env("AMBIENCE_WORKFLOW_REF")),
		claudeNamespace:           envOr(c, "GLIMMUNG_INPUT_CLAUDE_NAMESPACE", "tank-operator"),
		claudeCANamespace:         firstNonEmpty(c.Env("GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE"), c.Env("CLAUDE_CA_NAMESPACE"), "tank-operator-sessions"),
		providerAPIProxyNamespace: providerNS,
		githubPolicyCASecret:      envOr(c, "GLIMMUNG_GITHUB_POLICY_CA_SECRET", "glimmung-provider-api-proxy-ca"),
		githubPolicyCAConfigMap:   envOr(c, "GLIMMUNG_GITHUB_POLICY_CA_CONFIGMAP", "glimmung-provider-api-proxy-ca"),
		githubPolicyProxyService:  envOr(c, "GLIMMUNG_GITHUB_POLICY_PROXY_SERVICE", "github-git-policy-proxy"),
		agentEgressGatewayName:    envOr(c, "GLIMMUNG_AGENT_EGRESS_GATEWAY_NAME", "agent-egress"),
		agentEgressGatewayNS:      firstNonEmpty(c.Env("GLIMMUNG_AGENT_EGRESS_GATEWAY_NAMESPACE"), providerNS),
		envoyGatewaySystemNS:      envOr(c, "GLIMMUNG_ENVOY_GATEWAY_SYSTEM_NAMESPACE", "envoy-gateway-system"),
		validationURL:             validationURL,
		namespace:                 namespace,
		imageTag:                  imageTag,
		branchName:                branch,
		jobName:                   fmt.Sprintf("agent-%s-im-%s", runSlug, attempt),
		configMap:                 "agent-config-implement",
		agentContainerTag:         c.Env("AGENT_CONTAINER_TAG"),
		agentContainerImage:       image,
		agentRuntimeJSON:          c.Env("GLIMMUNG_AGENT_RUNTIME_JSON"),
		featureType:               c.Env("AMBIENCE_FEATURE_TYPE"),
		evidenceDir:               envOr(c, "AMBIENCE_EVIDENCE_DIR", "/tmp/evidence"),
		prBase:                    envOr(c, "AMBIENCE_PR_BASE", "main"),
		issue:                     agentjob.DeriveIssueContext(c),
	}, nil
}

func toLayered(err error) *step.LayeredError {
	if le, ok := err.(*step.LayeredError); ok {
		return le
	}
	return step.HarnessError("internal", err.Error(), err)
}

func (cfg config) git(c *step.Context) (venue.Git, *step.LayeredError) {
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return venue.Git{}, lerr
	}
	return venue.Git{Exec: venue.Exec{}, Token: token}, nil
}

func (cfg config) github(c *step.Context) (venue.GitHub, *step.LayeredError) {
	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return venue.GitHub{}, lerr
	}
	return venue.GitHub{Token: token}, nil
}

func implementationFailed() bool { return venue.ReadExitCode(exitCodePath) != 0 }

func cloneRepo(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if err := g.CloneRepo(c.RunContext(), cfg.repoSlug, cfg.repoDir, cfg.workflowRef, cfg.branchName); err != nil {
		return step.Result{}, step.HostError("git_clone", "clone repo", err)
	}
	_ = os.MkdirAll(cfg.evidenceDir+"/screenshots", 0o755)
	_ = os.MkdirAll(cfg.evidenceDir+"/videos", 0o755)
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
	// Generate the implementation contract from the feature type (runner-owned).
	contractPath := cfg.evidenceDir + "/implementation-contract.json"
	_ = os.MkdirAll(cfg.evidenceDir, 0o755)
	if err := os.WriteFile(contractPath, contract.Generate(cfg.featureType), 0o644); err != nil {
		return step.Result{}, step.HarnessError("contract", "write implementation contract", err)
	}
	fmt.Printf("generated implementation contract %s\n", contractPath)

	prep := agentjob.Prep{Exec: ex, Namespace: cfg.namespace}
	if err := prep.CopyClaudeCA(c.RunContext(), cfg.claudeCANamespace); err != nil {
		return step.Result{}, step.HostError("copy_claude_ca", "copy provider CA", err)
	}
	if err := prep.CopyGitHubPolicyCA(c.RunContext(), cfg.providerAPIProxyNamespace, cfg.githubPolicyCASecret, cfg.githubPolicyCAConfigMap); err != nil {
		return step.Result{}, step.HostError("copy_github_policy_ca", "copy github policy CA", err)
	}
	fmt.Printf("using Glimmung provider API proxy for Codex in %s; no raw codex-credentials Secret is created\n", cfg.namespace)

	proxyIP, err := prep.ResolveClusterIP(c.RunContext(), cfg.claudeNamespace, "claude-api-proxy")
	if err != nil {
		return step.Result{}, step.HostError("resolve_proxy", "resolve claude-api-proxy clusterIP", err)
	}
	_ = os.WriteFile(proxyIPPath, []byte(proxyIP+"\n"), 0o644)

	githubProxyIP, err := prep.ResolveClusterIP(c.RunContext(), cfg.providerAPIProxyNamespace, cfg.githubPolicyProxyService)
	if err != nil {
		return step.Result{}, step.HostError("resolve_github_proxy", "resolve github policy proxy clusterIP", err)
	}
	_ = os.WriteFile(githubProxyIPPath, []byte(githubProxyIP+"\n"), 0o644)

	token, lerr := runcallbacks.FromContext(c).MintGitHubToken(c.RunContext())
	if lerr != nil {
		return step.Result{}, lerr
	}
	if err := prep.CreateGitHubTokenSecret(c.RunContext(), token); err != nil {
		return step.Result{}, step.HostError("github_secret", "stage agent github token", err)
	}

	files := []string{"prompt-implementation.md=" + cfg.repoDir + "/.github/agent/prompt-implementation.md"}
	if info, err := os.Stat(contractPath); err == nil && info.Size() > 0 {
		files = append(files, "implementation-contract.json="+contractPath)
	}
	if err := prep.CreateConfigMap(c.RunContext(), cfg.configMap, files); err != nil {
		return step.Result{}, step.HostError("configmap", "stage prompt configmap", err)
	}
	return step.Result{}, nil
}

func prepareDraftPRBranch(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if g.RemoteBranchExists(c.RunContext(), cfg.repoSlug, cfg.branchName) {
		fmt.Printf("implementation branch %s already exists\n", cfg.branchName)
		return step.Result{}, nil
	}
	if err := g.CommitAllowEmpty(c.RunContext(), cfg.repoDir,
		"agent: start "+cfg.issue.Reference,
		fmt.Sprintf("Glimmung run %s opened this branch before implementation so CI feedback is visible during the agent loop.", c.RunID()),
	); err != nil {
		return step.Result{}, step.HostError("git_commit", "open implementation branch", err)
	}
	if err := g.PushBranch(c.RunContext(), cfg.repoSlug, cfg.repoDir, cfg.branchName); err != nil {
		return step.Result{}, step.HostError("git_push", "push initial implementation branch", err)
	}
	fmt.Printf("created initial implementation branch %s\n", cfg.branchName)
	return step.Result{}, nil
}

const prBodyTemplate = `Draft PR opened by the Ambience Glimmung implementation wrapper.

- Glimmung run: %s
- Issue: %s
- Branch: %s

Deterministic CI checks must pass before LLM verification starts. The later Glimmung touchpoint is a human review boundary; it does not merge this PR.`

func ensureDraftPR(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	gh, lerr := cfg.github(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	owner, repo := splitSlug(cfg.repoSlug)
	title := fmt.Sprintf("Glimmung %s: %s", cfg.issue.Reference, cfg.issue.Title)
	body := fmt.Sprintf(prBodyTemplate, c.RunID(), cfg.issue.URL, cfg.branchName)

	existing, err := gh.FindOpenPR(c.RunContext(), owner, repo, cfg.branchName)
	if err != nil {
		return step.Result{}, step.HostError("github_pr", "look up existing PR", err)
	}
	var number int
	var url string
	if existing != nil {
		if err := gh.UpdatePR(c.RunContext(), owner, repo, existing.Number, title, body); err != nil {
			return step.Result{}, step.HostError("github_pr", "update draft PR", err)
		}
		number, url = existing.Number, existing.HTMLURL
		fmt.Printf("updated existing implementation PR %s\n", url)
	} else {
		pr, err := gh.CreatePR(c.RunContext(), owner, repo, title, cfg.branchName, cfg.prBase, body)
		if err != nil {
			return step.Result{}, step.HostError("github_pr", "open draft PR", err)
		}
		number, url = pr.Number, pr.HTMLURL
		fmt.Printf("opened draft implementation PR %s\n", url)
	}
	_ = os.WriteFile(prNumberPath, []byte(strconv.Itoa(number)+"\n"), 0o644)
	_ = os.WriteFile(prURLPath, []byte(url+"\n"), 0o644)
	return step.Result{}, nil
}

func runImplementation(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	proxyIP := readFileTrim(proxyIPPath)
	githubProxyIP := readFileTrim(githubProxyIPPath)
	githubEgressIP := cfg.resolveGitHubEgressIP(c)

	spawn := agentjob.Spawn{
		Exec:                venue.Exec{},
		RepoDir:             cfg.repoDir,
		Namespace:           cfg.namespace,
		JobName:             cfg.jobName,
		Stage:               "implement",
		ConfigMap:           cfg.configMap,
		Issue:               cfg.issue,
		ValidationURL:       cfg.validationURL,
		BranchName:          cfg.branchName,
		ProxyIP:             proxyIP,
		GitHubProxyIP:       githubProxyIP,
		GitHubEgressIP:      githubEgressIP,
		AgentContainerTag:   cfg.agentContainerTag,
		AgentContainerImage: cfg.agentContainerImage,
		RepoSlug:            cfg.repoSlug,
		AgentRuntimeJSON:    cfg.agentRuntimeJSON,
		MarkerIntent:        "helper",
		MarkerLabel:         "implement-agent",
	}
	if err := spawn.Apply(c.RunContext()); err != nil {
		return step.Result{}, step.HostError("apply_agent_job", "render implementation agent job", err)
	}
	timeout := envOr(c, "AGENT_IMPLEMENT_TIMEOUT_SECONDS", "2400")
	if err := spawn.Wait(c.RunContext(), timeout); err != nil {
		_ = venue.WriteExitCode(exitCodePath, 1)
		fmt.Fprintf(os.Stderr, "implementation agent job failed: %v\n", err)
		return step.Result{}, nil
	}
	_ = venue.WriteExitCode(exitCodePath, 0)
	return step.Result{}, nil
}

// resolveGitHubEgressIP mirrors ensure_github_egress_ip: best-effort resolution
// of the agent-egress gateway data-plane clusterIP; a miss is non-fatal (the
// deterministic wait-pr-checks gate still gates CI).
func (cfg config) resolveGitHubEgressIP(c *step.Context) string {
	if ip := readFileTrim(githubEgressIPPath); ip != "" {
		return ip
	}
	ex := venue.Exec{}
	selector := fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s,gateway.envoyproxy.io/owning-gateway-namespace=%s",
		cfg.agentEgressGatewayName, cfg.agentEgressGatewayNS)
	ip, err := ex.OutputQuiet(c.RunContext(), "", "kubectl", "-n", cfg.envoyGatewaySystemNS, "get", "svc",
		"-l", selector, "-o", "jsonpath={.items[0].spec.clusterIP}")
	ip = strings.TrimSpace(ip)
	if err != nil || ip == "" {
		fmt.Fprintf(os.Stderr, "WARNING: agent-egress gateway data-plane Service not found; agent will not reach api.github.com this run\n")
		return ""
	}
	_ = os.WriteFile(githubEgressIPPath, []byte(ip+"\n"), 0o644)
	return ip
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

func pushBranch(c *step.Context) (step.Result, error) {
	if implementationFailed() {
		fmt.Println("implementation pod failed; skipping branch confirmation")
		return step.Result{}, nil
	}
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	if !g.RemoteBranchExists(c.RunContext(), cfg.repoSlug, cfg.branchName) {
		return step.Result{}, step.ModelError("branch_absent",
			fmt.Sprintf("branch %s is absent — the implementation agent did not complete its push", cfg.branchName), nil)
	}
	fmt.Printf("branch %s is present on the remote\n", cfg.branchName)
	return validateImplementationContract(c, cfg, g)
}

func validateImplementationContract(c *step.Context, cfg config, g venue.Git) (step.Result, error) {
	contractPath := cfg.evidenceDir + "/implementation-contract.json"
	contractJSON, err := os.ReadFile(contractPath)
	if err != nil || len(strings.TrimSpace(string(contractJSON))) == 0 {
		fmt.Println("no implementation contract present; skipping validation")
		return step.Result{}, nil
	}
	implJSON := readImplementationJSON(cfg)
	if len(strings.TrimSpace(string(implJSON))) == 0 {
		return step.Result{}, step.HarnessError("missing_impl", "implementation contract validation cannot run without implementation JSON", nil)
	}
	if err := g.Fetch(c.RunContext(), cfg.repoDir,
		"+refs/heads/main:refs/remotes/origin/main",
		"+refs/heads/"+cfg.branchName+":refs/remotes/origin/"+cfg.branchName); err != nil {
		return step.Result{}, step.HostError("git_fetch", "fetch branch for contract validation", err)
	}
	branchRev, err := g.RevParse(c.RunContext(), cfg.repoDir, "origin/"+cfg.branchName)
	if err != nil {
		return step.Result{}, step.HostError("git_rev_parse", "resolve branch revision", err)
	}
	changed, err := g.ChangedFiles(c.RunContext(), cfg.repoDir, "origin/main", branchRev)
	if err != nil {
		return step.Result{}, step.HostError("git_diff", "compute changed files", err)
	}
	res := contract.ValidateImplementation(contractJSON, implJSON, changed)
	writeJSON("/tmp/implementation-contract-validation.json", res)
	if res.Status == "fail" {
		return step.Result{}, step.ModelError(res.AbortReason,
			fmt.Sprintf("implementation contract failed: %s: %s", res.AbortReason, res.Detail), nil)
	}
	fmt.Printf("implementation contract satisfied: feature_type=%s effect_slug=%s\n", res.FeatureType, res.EffectSlug)
	return step.Result{}, nil
}

func waitPRChecks(c *step.Context) (step.Result, error) {
	if implementationFailed() {
		fmt.Println("implementation pod failed; skipping PR checks")
		return step.Result{}, nil
	}
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	gh, lerr := cfg.github(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	branchRev, err := g.RemoteBranchRevision(c.RunContext(), cfg.repoSlug, cfg.branchName)
	if err != nil {
		return step.Result{}, step.HostError("git_rev", "resolve branch revision", err)
	}
	owner, repo := splitSlug(cfg.repoSlug)
	pollSeconds := atoiOr(envOr(c, "AMBIENCE_PR_CHECK_POLL_SECONDS", "30"), 30)
	timeoutSeconds := atoiOr(envOr(c, "AMBIENCE_PR_CHECK_TIMEOUT_SECONDS", "3600"), 3600)
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	fmt.Printf("waiting for PR checks on %s@%s\n", cfg.branchName, branchRev)
	for {
		snap, err := gh.CommitChecks(c.RunContext(), owner, repo, branchRev)
		if err != nil {
			return step.Result{}, step.HostError("github_checks", "read PR checks", err)
		}
		switch snap.Classify() {
		case venue.CheckFailed:
			return step.Result{}, step.ModelError("pr_checks_failed", "PR CI checks failed for the implementation branch", nil)
		case venue.CheckPassed:
			fmt.Printf("PR checks passed for %s@%s\n", cfg.branchName, branchRev)
			return step.Result{}, nil
		}
		if time.Now().After(deadline) {
			return step.Result{}, step.HostError("pr_checks_timeout", "timed out waiting for PR checks", nil)
		}
		select {
		case <-c.RunContext().Done():
			return step.Result{}, step.HarnessError("cancelled", "context cancelled waiting for PR checks", c.RunContext().Err())
		case <-time.After(time.Duration(pollSeconds) * time.Second):
		}
	}
}

func rebuildEnv(c *step.Context) (step.Result, error) {
	if implementationFailed() {
		fmt.Println("implementation pod failed; skipping validation rebuild")
		return step.Result{}, nil
	}
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	g, lerr := cfg.git(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	branchRev, err := g.RemoteBranchRevision(c.RunContext(), cfg.repoSlug, cfg.branchName)
	if err != nil {
		return step.Result{}, step.HostError("git_rev", "resolve branch revision", err)
	}
	rebuildTag, tagErr := naming.ImageTagForRevision(branchRev)
	if tagErr != nil {
		return step.Result{}, step.HarnessError("image_tag", tagErr.Error(), nil)
	}
	fmt.Printf("rebuilding validation image from %s@%s\n", cfg.branchName, branchRev)
	ex := venue.Exec{}
	if err := ex.PreviewCLI(c.RunContext(), cfg.repoDir, "rebuild-validation-image",
		"--namespace", cfg.namespace, "--branch", cfg.branchName, "--image-tag", rebuildTag,
		"--source-revision", branchRev, "--repo-slug", cfg.repoSlug); err != nil {
		return step.Result{}, step.HostError("rebuild_image", "rebuild validation image", err)
	}
	if err := ex.PreviewCLI(c.RunContext(), cfg.repoDir, "wait-public-preview", "--url", cfg.validationURL); err != nil {
		return step.Result{}, step.HostError("wait_public_preview", "validation env did not become ready after rebuild", err)
	}
	return step.Result{}, nil
}

func finalize(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	copyIfPresent(cfg.evidenceDir+"/issue-agent-implementation.json", implementationJSONPath)
	copyIfPresent(cfg.evidenceDir+"/issue-agent-implementation.md", summaryMDPath)

	data, _ := os.ReadFile(implementationJSONPath)
	if len(strings.TrimSpace(string(data))) == 0 {
		reason := "implement pod produced no output"
		if code := venue.ReadExitCode(exitCodePath); code != 0 {
			reason = fmt.Sprintf("implement pod exited with %d", code)
		}
		writeJSON(implementationJSONPath, map[string]any{"schema_version": 1, "status": "fail", "abort_reason": reason})
		return step.Result{}, nil
	}

	// ui_hint gate: a passing implementation under a standing feature type must
	// declare {menu_label, route:/dev/<effect>} or the phase fails HERE, named,
	// before verify binds the standing case.
	var impl map[string]any
	if err := json.Unmarshal(data, &impl); err != nil {
		return step.Result{}, step.HarnessError("impl_json", "implementation JSON is malformed", err)
	}
	out := contract.EnforceUIHint(impl, cfg.featureType)
	if !out.OK {
		writeJSON(implementationJSONPath, impl) // persist the mutated status=fail doc
		return step.Result{}, step.ModelError(out.AbortReason,
			fmt.Sprintf("missing_ui_hint: feature_type=%s requires a passing implementation to declare ui_hint {menu_label, route:/dev/<effect>}", cfg.featureType), nil)
	}
	if out.MenuLabel != "" {
		fmt.Printf("ui_hint contract satisfied: menu_label=%s route=%s\n", out.MenuLabel, out.Route)
	}
	return step.Result{}, nil
}

func emit(c *step.Context) (step.Result, error) {
	cfg, lerr := newConfig(c)
	if lerr != nil {
		return step.Result{}, lerr
	}
	implStr := strings.TrimSpace(string(readImplementationJSON(cfg)))
	if err := c.EmitOutput("implementation", implStr); err != nil {
		return step.Result{}, err
	}
	if err := c.EmitOutput("branch_name", cfg.branchName); err != nil {
		return step.Result{}, err
	}
	if pr := readFileTrim(prNumberPath); pr != "" {
		if err := c.EmitOutput("pr_number", pr); err != nil {
			return step.Result{}, err
		}
	}
	if u := readFileTrim(prURLPath); u != "" {
		if err := c.EmitOutput("pr_url", u); err != nil {
			return step.Result{}, err
		}
	}
	if hint := uiHintString(implStr); hint != "" {
		if err := c.EmitOutput("ui_hint", hint); err != nil {
			return step.Result{}, err
		}
	}
	if summary, err := os.ReadFile(summaryMDPath); err == nil {
		c.SetSummaryMarkdown(strings.TrimSpace(string(summary)))
	}
	if code := venue.ReadExitCode(exitCodePath); code != 0 {
		return step.Result{}, step.ModelError("implementation_failed",
			fmt.Sprintf("implement phase failed (exit %d); see emitted implementation verdict", code), nil)
	}
	return step.Result{}, nil
}

// --- helpers ---

func readImplementationJSON(cfg config) []byte {
	if data, err := os.ReadFile(implementationJSONPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return data
	}
	if data, err := os.ReadFile(cfg.evidenceDir + "/issue-agent-implementation.json"); err == nil {
		_ = os.WriteFile(implementationJSONPath, data, 0o644)
		return data
	}
	return nil
}

func uiHintString(implStr string) string {
	var impl map[string]any
	if err := json.Unmarshal([]byte(implStr), &impl); err != nil {
		return ""
	}
	hint, ok := impl["ui_hint"]
	if !ok || hint == nil {
		return ""
	}
	b, err := json.Marshal(hint)
	if err != nil {
		return ""
	}
	return string(b)
}

func splitSlug(slug string) (owner, repo string) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return slug, ""
	}
	return parts[0], parts[1]
}

func readFileTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return n
	}
	return def
}
