// Package naming holds the pure identifier-derivation logic ported verbatim
// from the retired scripts/glimmung-native/lib.sh fork: image tags, branch
// names, issue-branch prefixes, the workflow checkout ref, and the agent
// container image. These were the most-tested seams of the shell harness
// (scripts/test-glimmung-native-contract.sh) and they carry no I/O, so they
// move to typed, table-tested Go functions with identical behavior.
package naming

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	dashRun = regexp.MustCompile(`-+`)
	hex40   = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// SanitizeRefSegment mirrors native_sanitize_ref_segment: lowercase, replace
// every character outside [a-z0-9._-] with '-', collapse runs of '-', trim
// leading/trailing '.' and '-', and fall back to "unknown" when empty.
func SanitizeRefSegment(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := dashRun.ReplaceAllString(b.String(), "-")
	s = strings.Trim(s, ".-")
	if s == "" {
		return "unknown"
	}
	return s
}

// ImageTagForRevision mirrors native_image_tag_for_revision: the revision must
// be a 40-character lowercase git SHA; the tag is "git-<sha>".
func ImageTagForRevision(revision string) (string, error) {
	rev := strings.ToLower(strings.TrimSpace(revision))
	if !hex40.MatchString(rev) {
		return "", fmt.Errorf("source revision must be a 40-character git SHA: %s", revision)
	}
	return "git-" + rev, nil
}

// BranchInputs are the environment values native_implementation_branch_name
// consults, in the SAME preference order.
type BranchInputs struct {
	AmbienceImplementationBranch string // AMBIENCE_IMPLEMENTATION_BRANCH
	IssueNumber                  string // GLIMMUNG_ISSUE_NUMBER
	RunID                        string // GLIMMUNG_RUN_ID
	WorkContextBranch            string // GLIMMUNG_WORK_CONTEXT_BRANCH
}

// ImplementationBranchName mirrors native_implementation_branch_name's
// preference order:
//  1. AMBIENCE_IMPLEMENTATION_BRANCH (verbatim)
//  2. glimmung/issue-<issue>/<run> when issue+run are both set
//  3. GLIMMUNG_WORK_CONTEXT_BRANCH (verbatim)
//  4. glimmung/<run> when only the run id is set
//
// A missing run id (with none of the higher-precedence values) is an error,
// matching the shell's `exit 2`.
func ImplementationBranchName(in BranchInputs) (string, error) {
	if v := strings.TrimSpace(in.AmbienceImplementationBranch); v != "" {
		return v, nil
	}
	issue := strings.TrimSpace(in.IssueNumber)
	run := strings.TrimSpace(in.RunID)
	if issue != "" && run != "" {
		return fmt.Sprintf("glimmung/issue-%s/%s", SanitizeRefSegment(issue), SanitizeRefSegment(run)), nil
	}
	if v := strings.TrimSpace(in.WorkContextBranch); v != "" {
		return v, nil
	}
	if run != "" {
		return "glimmung/" + SanitizeRefSegment(run), nil
	}
	return "", fmt.Errorf("GLIMMUNG_RUN_ID is required to derive the implementation branch")
}

// IssueBranchPrefix mirrors native_issue_branch_prefix: glimmung/issue-<issue>/
// when the issue number is set, otherwise an error (the shell returned 1).
func IssueBranchPrefix(issueNumber string) (string, error) {
	issue := strings.TrimSpace(issueNumber)
	if issue == "" {
		return "", fmt.Errorf("GLIMMUNG_ISSUE_NUMBER is not set")
	}
	return fmt.Sprintf("glimmung/issue-%s/", SanitizeRefSegment(issue)), nil
}

// WorkflowCheckoutRef mirrors native_workflow_checkout_ref:
// GLIMMUNG_RUN_INPUT_GIT_REF, then AMBIENCE_WORKFLOW_REF, then "main".
func WorkflowCheckoutRef(runInputGitRef, ambienceWorkflowRef string) string {
	if v := strings.TrimSpace(runInputGitRef); v != "" {
		return v
	}
	if v := strings.TrimSpace(ambienceWorkflowRef); v != "" {
		return v
	}
	return "main"
}

// AgentContainerImage mirrors native_agent_container_image:
// AGENT_CONTAINER_IMAGE wins; otherwise AGENT_CONTAINER_TAG selects
// romainecr.azurecr.io/ambience-agent-runner:<tag>; otherwise an error.
func AgentContainerImage(image, tag string) (string, error) {
	if v := strings.TrimSpace(image); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(tag); v != "" {
		return "romainecr.azurecr.io/ambience-agent-runner:" + v, nil
	}
	return "", fmt.Errorf("AGENT_CONTAINER_IMAGE or AGENT_CONTAINER_TAG must be set for inner agent jobs")
}
