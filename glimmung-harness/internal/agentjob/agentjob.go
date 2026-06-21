// Package agentjob holds the shared in-cluster agent inner-Job venue the
// test-plan and implement phases both use: deriving the issue context, copying
// the provider CA into the slot namespace, resolving proxy cluster IPs, staging
// the per-attempt GitHub token Secret and the prompt ConfigMap, and spawning +
// waiting on the agent Job (rendered by the ambience_preview Python venue).
//
// This is the in-cluster analogue of spirelens's remote-host agent venue: the
// model does NOT run as an in-pod child of the harness step (so
// harness/agent.Invoke does not apply); it runs in an isolated child k8s Job
// that never mounts the real provider OAuth secret. The agent's usage lines are
// priced through Glimmung's inner-Job observation, primed by the
// ===GLIMMUNG-INNER-JOB=== marker EmitInnerJobMarker writes. See the SDK gap
// flagged for the hub: an "inner-job agent venue" the SDK could own so model
// layer attribution is typed across the pod boundary.
package agentjob

import (
	"context"
	"fmt"
	"strings"

	"github.com/romaine-life/ambience/glimmung-harness/internal/venue"
	"github.com/romaine-life/glimmung/harness/step"
)

// IssueContext is the issue identity both LLM phases derive identically from the
// GLIMMUNG_* environment.
type IssueContext struct {
	Title     string
	Number    string
	Project   string
	Reference string
	URL       string
}

// DeriveIssueContext mirrors the ISSUE_* derivation at the top of test-plan.sh
// and implement.sh.
func DeriveIssueContext(c *step.Context) IssueContext {
	env := func(k string) string { return strings.TrimSpace(c.Env(k)) }
	runID := c.RunID()
	issueID := env("GLIMMUNG_ISSUE_ID")
	number := env("GLIMMUNG_ISSUE_NUMBER")
	project := env("GLIMMUNG_PROJECT")
	if project == "" {
		project = "ambience"
	}
	title := env("GLIMMUNG_ISSUE_TITLE")
	if title == "" {
		idForTitle := issueID
		if idForTitle == "" {
			idForTitle = runID
		}
		title = "Glimmung issue " + idForTitle
	}
	refID := number
	if refID == "" {
		refID = issueID
	}
	if refID == "" {
		refID = runID
	}
	reference := project + "#" + refID
	base := env("GLIMMUNG_BASE_URL")
	if base == "" {
		base = "https://glimmung.romaine.life"
	}
	var url string
	if number != "" {
		url = fmt.Sprintf("%s/projects/%s/issues/%s", base, project, number)
	} else {
		idForURL := issueID
		if idForURL == "" {
			idForURL = runID
		}
		url = fmt.Sprintf("%s/issues/%s/%s", base, project, idForURL)
	}
	return IssueContext{Title: title, Number: number, Project: project, Reference: reference, URL: url}
}

// Prep carries the namespace venue prep operations shared by both LLM phases.
type Prep struct {
	Exec      venue.Exec
	Namespace string
}

// CopyClaudeCA mirrors copy_claude_ca: prefer the locally mounted provider CA,
// else clone the claude-oauth-ca ConfigMap from the CA namespace into the slot.
func (p Prep) CopyClaudeCA(ctx context.Context, claudeCANamespace string) error {
	if fileNonEmpty("/etc/glimmung-provider-api-proxy-ca/ca.crt") {
		yaml, err := p.Exec.Output(ctx, "", "kubectl", "-n", p.Namespace, "create", "configmap", "claude-oauth-ca",
			"--from-file=ca.crt=/etc/glimmung-provider-api-proxy-ca/ca.crt", "--dry-run=client", "-o", "yaml")
		if err != nil {
			return err
		}
		return p.apply(ctx, yaml)
	}
	src, err := p.Exec.Output(ctx, "", "kubectl", "-n", claudeCANamespace, "get", "configmap", "claude-oauth-ca", "-o", "json")
	if err != nil {
		return err
	}
	rewritten, err := rewriteConfigMapNamespace(src, p.Namespace)
	if err != nil {
		return err
	}
	return p.apply(ctx, rewritten)
}

// ResolveClusterIP returns a Service's clusterIP, failing when absent.
func (p Prep) ResolveClusterIP(ctx context.Context, namespace, service string) (string, error) {
	ip, err := p.Exec.Output(ctx, "", "kubectl", "-n", namespace, "get", "svc", service, "-o", "jsonpath={.spec.clusterIP}")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ip) == "" {
		return "", fmt.Errorf("%s Service not found in %s", service, namespace)
	}
	return strings.TrimSpace(ip), nil
}

// CreateGitHubTokenSecret stages the agent-github-token Secret the agent Job
// mounts as GITHUB_TOKEN_FILE.
func (p Prep) CreateGitHubTokenSecret(ctx context.Context, token string) error {
	yaml, err := p.Exec.Output(ctx, "", "kubectl", "-n", p.Namespace, "create", "secret", "generic", "agent-github-token",
		"--from-literal=token="+token, "--dry-run=client", "-o", "yaml")
	if err != nil {
		return err
	}
	return p.apply(ctx, yaml)
}

// CreateConfigMap stages a ConfigMap from --from-file specs ("key=path").
func (p Prep) CreateConfigMap(ctx context.Context, name string, fromFiles []string) error {
	args := []string{"-n", p.Namespace, "create", "configmap", name}
	for _, f := range fromFiles {
		args = append(args, "--from-file="+f)
	}
	args = append(args, "--dry-run=client", "-o", "yaml")
	yaml, err := p.Exec.Output(ctx, "", "kubectl", args...)
	if err != nil {
		return err
	}
	return p.apply(ctx, yaml)
}

func (p Prep) apply(ctx context.Context, manifest string) error {
	return p.Exec.RunInput(ctx, "", "kubectl", strings.NewReader(manifest), "apply", "-f", "-")
}

// Spawn renders and waits on one agent inner Job through the ambience_preview
// Python venue.
type Spawn struct {
	Exec                venue.Exec
	RepoDir             string
	Namespace           string
	JobName             string
	Stage               string
	ConfigMap           string
	Issue               IssueContext
	ValidationURL       string
	BranchName          string
	ProxyIP             string
	GitHubProxyIP       string
	GitHubEgressIP      string
	AgentContainerTag   string
	AgentContainerImage string
	RepoSlug            string
	AgentRuntimeJSON    string
	MarkerIntent        string
	MarkerLabel         string
}

// Apply spawns the agent Job and emits the inner-Job observation marker.
func (s Spawn) Apply(ctx context.Context) error {
	number := s.Issue.Number
	if number == "" {
		number = s.Issue.Reference
	}
	args := []string{
		"apply-agent-job",
		"--namespace", s.Namespace,
		"--job-name", s.JobName,
		"--issue-number", number,
		"--issue-title", s.Issue.Title,
		"--issue-url", s.Issue.URL,
		"--issue-reference", s.Issue.Reference,
		"--validation-url", s.ValidationURL,
		"--branch-name", s.BranchName,
		"--proxy-ip", s.ProxyIP,
	}
	if strings.TrimSpace(s.GitHubProxyIP) != "" {
		args = append(args, "--github-proxy-ip", s.GitHubProxyIP)
	}
	if strings.TrimSpace(s.GitHubEgressIP) != "" {
		args = append(args, "--github-egress-ip", s.GitHubEgressIP)
	}
	args = append(args,
		"--agent-container-tag", s.AgentContainerTag,
		"--agent-container-image", s.AgentContainerImage,
		"--repo-slug", s.RepoSlug,
		"--stage", s.Stage,
		"--config-map-name", s.ConfigMap,
		"--agent-runtime-json", s.AgentRuntimeJSON,
	)
	if err := s.Exec.PreviewCLI(ctx, s.RepoDir, args...); err != nil {
		return err
	}
	intent := s.MarkerIntent
	if intent == "" {
		intent = "helper"
	}
	return venue.EmitInnerJobMarker(s.Namespace, s.JobName, intent, s.MarkerLabel)
}

// Wait blocks on the agent Job and returns its terminal error (nil on success).
func (s Spawn) Wait(ctx context.Context, timeoutSeconds string) error {
	return s.Exec.PreviewCLI(ctx, s.RepoDir, "wait-agent-job",
		"--namespace", s.Namespace, "--job-name", s.JobName, "--timeout-seconds", timeoutSeconds)
}

// CollectPodLog returns the agent Job pod's logs, or "" when no pod exists yet.
func (p Prep) CollectPodLog(ctx context.Context, jobName string) (string, error) {
	pod, err := p.Exec.OutputQuiet(ctx, "", "kubectl", "-n", p.Namespace, "get", "pods",
		"-l", "job-name="+jobName, "-o", "jsonpath={.items[0].metadata.name}")
	if err != nil || strings.TrimSpace(pod) == "" {
		return "", nil
	}
	logs, err := p.Exec.OutputQuiet(ctx, "", "kubectl", "-n", p.Namespace, "logs", strings.TrimSpace(pod))
	if err != nil {
		return logs, nil // best-effort, mirrors `|| true`
	}
	return logs, nil
}
