package venue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub is a minimal GitHub REST client authenticated with the per-attempt
// token, replacing the curl-based github_api helper in the shell harness.
type GitHub struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string // defaults to https://api.github.com
}

func (g GitHub) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (g GitHub) base() string {
	if strings.TrimSpace(g.BaseURL) != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return "https://api.github.com"
}

// Do performs a GitHub REST call and decodes the JSON response into out (which
// may be nil). A response status >= 400 is an error carrying the body.
func (g GitHub) Do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base()+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github %s %s returned HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	if out != nil && buf.Len() > 0 {
		if err := json.Unmarshal(buf.Bytes(), out); err != nil {
			return fmt.Errorf("decode github response: %w", err)
		}
	}
	return nil
}

// PullRequest is the subset of a PR object the harness reads.
type PullRequest struct {
	Number   int    `json:"number"`
	HTMLURL  string `json:"html_url"`
	MergedAt string `json:"merged_at"`
}

// FindOpenPR returns the first open PR for owner:branch, or (nil) when none.
func (g GitHub) FindOpenPR(ctx context.Context, owner, repo, branch string) (*PullRequest, error) {
	head := url.QueryEscape(owner + ":" + branch)
	var prs []PullRequest
	if err := g.Do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls?head=%s&state=open&per_page=1", owner, repo, head), nil, &prs); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

// CreatePR opens a draft PR and returns it.
func (g GitHub) CreatePR(ctx context.Context, owner, repo, title, head, base, body string) (*PullRequest, error) {
	var pr PullRequest
	req := map[string]any{"title": title, "head": head, "base": base, "body": body, "draft": true}
	if err := g.Do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), req, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePR patches a PR's title and body.
func (g GitHub) UpdatePR(ctx context.Context, owner, repo string, number int, title, body string) error {
	return g.Do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), map[string]any{"title": title, "body": body}, nil)
}

// BranchHasMergedPR reports whether any PR for owner:branch (any state) merged,
// mirroring env-destroy.sh's branch_has_merged_pr.
func (g GitHub) BranchHasMergedPR(ctx context.Context, owner, repo, branch string) (bool, error) {
	head := url.QueryEscape(owner + ":" + branch)
	var prs []PullRequest
	if err := g.Do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls?head=%s&state=all&per_page=20", owner, repo, head), nil, &prs); err != nil {
		return false, err
	}
	for _, pr := range prs {
		if strings.TrimSpace(pr.MergedAt) != "" {
			return true, nil
		}
	}
	return false, nil
}

// CheckRun is one GitHub check-run.
type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type checkRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}

// CommitStatus is the combined commit status.
type CommitStatus struct {
	State    string `json:"state"`
	Statuses []struct {
		Context     string `json:"context"`
		State       string `json:"state"`
		TargetURL   string `json:"target_url"`
		Description string `json:"description"`
	} `json:"statuses"`
}

// ChecksSnapshot is the combined check-runs + commit-status view for one commit.
type ChecksSnapshot struct {
	CheckRuns     []CheckRun
	CombinedState string
	StatusCount   int
}

// CommitChecks fetches the check-runs and combined status for a commit SHA.
func (g GitHub) CommitChecks(ctx context.Context, owner, repo, sha string) (ChecksSnapshot, error) {
	var runs checkRunsResponse
	if err := g.Do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=100", owner, repo, sha), nil, &runs); err != nil {
		return ChecksSnapshot{}, err
	}
	var status CommitStatus
	if err := g.Do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/commits/%s/status", owner, repo, sha), nil, &status); err != nil {
		return ChecksSnapshot{}, err
	}
	state := status.State
	if state == "" {
		state = "pending"
	}
	return ChecksSnapshot{CheckRuns: runs.CheckRuns, CombinedState: state, StatusCount: len(status.Statuses)}, nil
}

// CheckVerdict classifies a checks snapshot the way implement.sh's
// wait_pr_checks loop does.
type CheckVerdict int

const (
	// CheckPending: no terminal verdict yet, keep polling.
	CheckPending CheckVerdict = iota
	// CheckPassed: every check completed successfully (or there were commit
	// statuses all green and no pending check-runs).
	CheckPassed
	// CheckFailed: at least one check-run completed non-success/neutral/skipped,
	// or the combined status is failure/error.
	CheckFailed
)

// Classify mirrors the wait_pr_checks decision: failing wins; otherwise pass
// once there is at least one signal, no pending check-runs, and the combined
// status is success or absent.
func (s ChecksSnapshot) Classify() CheckVerdict {
	pending := 0
	failing := 0
	for _, run := range s.CheckRuns {
		if run.Status != "completed" {
			pending++
			continue
		}
		switch run.Conclusion {
		case "success", "neutral", "skipped":
		default:
			failing++
		}
	}
	if failing > 0 || s.CombinedState == "failure" || s.CombinedState == "error" {
		return CheckFailed
	}
	if len(s.CheckRuns) > 0 || s.StatusCount > 0 {
		if pending == 0 && (s.CombinedState == "success" || s.StatusCount == 0) {
			return CheckPassed
		}
	}
	return CheckPending
}
