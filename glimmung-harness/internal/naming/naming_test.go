package naming

import "testing"

// Ported from scripts/test-glimmung-native-contract.sh cases for
// native_image_tag_for_revision.
func TestImageTagForRevision(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got, err := ImageTagForRevision(sha)
	if err != nil {
		t.Fatalf("valid 40-char SHA rejected: %v", err)
	}
	if got != "git-"+sha {
		t.Fatalf("ImageTagForRevision=%q want git-%s", got, sha)
	}
	if _, err := ImageTagForRevision("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("39-char revision must be rejected")
	}
	if _, err := ImageTagForRevision(""); err == nil {
		t.Fatal("empty revision must be rejected")
	}
	if _, err := ImageTagForRevision("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"); err == nil {
		t.Fatal("non-hex revision must be rejected")
	}
	// Uppercase normalizes to lowercase before validation.
	up := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	got, err = ImageTagForRevision(up)
	if err != nil || got != "git-"+sha {
		t.Fatalf("uppercase SHA: got %q err %v", got, err)
	}
}

// Ported from native_agent_container_image cases.
func TestAgentContainerImage(t *testing.T) {
	if _, err := AgentContainerImage("", ""); err == nil {
		t.Fatal("missing both image and tag must be rejected")
	}
	got, err := AgentContainerImage("", "native-runner-test")
	if err != nil || got != "romainecr.azurecr.io/ambience-agent-runner:native-runner-test" {
		t.Fatalf("tag path: got %q err %v", got, err)
	}
	got, err = AgentContainerImage("romainecr.azurecr.io/custom:tag", "ignored")
	if err != nil || got != "romainecr.azurecr.io/custom:tag" {
		t.Fatalf("image precedence: got %q err %v", got, err)
	}
}

// Ported from native_implementation_branch_name cases (preference order).
func TestImplementationBranchName(t *testing.T) {
	tests := []struct {
		name string
		in   BranchInputs
		want string
	}{
		{"run-only", BranchInputs{RunID: "run-1"}, "glimmung/run-1"},
		{"work-context-over-run", BranchInputs{RunID: "run-1", WorkContextBranch: "glimmung/work-context"}, "glimmung/work-context"},
		{"issue-and-run", BranchInputs{RunID: "run-1", IssueNumber: "168", WorkContextBranch: "glimmung/work-context"}, "glimmung/issue-168/run-1"},
		{"explicit-branch-wins", BranchInputs{RunID: "run-1", IssueNumber: "168", WorkContextBranch: "glimmung/work-context", AmbienceImplementationBranch: "glimmung/manual-branch"}, "glimmung/manual-branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ImplementationBranchName(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ImplementationBranchName=%q want %q", got, tt.want)
			}
		})
	}
	if _, err := ImplementationBranchName(BranchInputs{}); err == nil {
		t.Fatal("no run id and no overrides must error")
	}
}

// Ported from native_issue_branch_prefix cases.
func TestIssueBranchPrefix(t *testing.T) {
	got, err := IssueBranchPrefix("168")
	if err != nil || got != "glimmung/issue-168/" {
		t.Fatalf("IssueBranchPrefix=%q err %v", got, err)
	}
	if _, err := IssueBranchPrefix(""); err == nil {
		t.Fatal("missing issue number must error")
	}
}

// Ported from native_workflow_checkout_ref cases (preference order).
func TestWorkflowCheckoutRef(t *testing.T) {
	if got := WorkflowCheckoutRef("", ""); got != "main" {
		t.Fatalf("default=%q want main", got)
	}
	if got := WorkflowCheckoutRef("", "ambience-workflow-ref"); got != "ambience-workflow-ref" {
		t.Fatalf("ambience ref=%q", got)
	}
	if got := WorkflowCheckoutRef("codex/lifecycle-observe", "ambience-workflow-ref"); got != "codex/lifecycle-observe" {
		t.Fatalf("run-input ref precedence=%q", got)
	}
}

func TestSanitizeRefSegment(t *testing.T) {
	tests := map[string]string{
		"Run-1":           "run-1",
		"feature/Foo Bar": "feature-foo-bar",
		"  ":              "unknown",
		"--a--b--":        "a-b", // leading/trailing '-' trimmed, internal runs collapsed
		"...dotted...":    "dotted",
		"keep_underscore": "keep_underscore",
	}
	for in, want := range tests {
		if got := SanitizeRefSegment(in); got != want {
			t.Errorf("SanitizeRefSegment(%q)=%q want %q", in, got, want)
		}
	}
}
