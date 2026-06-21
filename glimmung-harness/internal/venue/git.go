package venue

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitAuthHeader mirrors native_git_auth_header: an HTTP Basic header carrying
// "x-access-token:<token>" base64-encoded, used as git http.extraHeader so the
// token never lands in git config.
func GitAuthHeader(token string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return "Authorization: Basic " + encoded
}

// Git runs git in repoDir with the per-attempt auth header injected via
// http.extraHeader for this invocation only.
type Git struct {
	Exec  Exec
	Token string
}

func (g Git) run(ctx context.Context, dir string, args ...string) error {
	full := append([]string{"-c", "http.extraHeader=" + GitAuthHeader(g.Token)}, args...)
	return g.Exec.Run(ctx, dir, "git", full...)
}

func (g Git) output(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-c", "http.extraHeader=" + GitAuthHeader(g.Token)}, args...)
	return g.Exec.Output(ctx, dir, "git", full...)
}

// CloneRepo mirrors native_clone_repo: idempotently init repoDir, set origin,
// fetch all heads with auth, set the bot identity, and check out either a fresh
// branch off base or a detached base.
func (g Git) CloneRepo(ctx context.Context, repoSlug, repoDir, baseRef, branchName string) error {
	remote := "https://github.com/" + repoSlug + ".git"
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return fmt.Errorf("mkdir repo parent: %w", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		if err := g.Exec.Run(ctx, "", "git", "init", repoDir); err != nil {
			return err
		}
		if err := g.Exec.Run(ctx, repoDir, "git", "remote", "add", "origin", remote); err != nil {
			return err
		}
	}
	if err := g.Exec.Run(ctx, repoDir, "git", "remote", "set-url", "origin", remote); err != nil {
		return err
	}
	if err := g.run(ctx, repoDir, "fetch", "--force", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	if err := g.Exec.Run(ctx, repoDir, "git", "config", "user.name", "ambience-agent[bot]"); err != nil {
		return err
	}
	if err := g.Exec.Run(ctx, repoDir, "git", "config", "user.email", "ambience-agent@romaine.life"); err != nil {
		return err
	}
	if strings.TrimSpace(branchName) != "" {
		return g.Exec.Run(ctx, repoDir, "git", "checkout", "-B", branchName, "origin/"+baseRef)
	}
	return g.Exec.Run(ctx, repoDir, "git", "checkout", "--detach", "origin/"+baseRef)
}

// PushBranch mirrors native_push_branch: push HEAD to the named remote branch.
func (g Git) PushBranch(ctx context.Context, repoSlug, repoDir, branchName string) error {
	remote := "https://github.com/" + repoSlug + ".git"
	if err := g.Exec.Run(ctx, repoDir, "git", "remote", "set-url", "origin", remote); err != nil {
		return err
	}
	return g.run(ctx, repoDir, "push", "origin", "HEAD:"+branchName)
}

// RemoteBranchRevision mirrors native_remote_branch_revision: the SHA the named
// branch points at on the remote, or an error if the branch is absent.
func (g Git) RemoteBranchRevision(ctx context.Context, repoSlug, branchName string) (string, error) {
	remote := "https://github.com/" + repoSlug + ".git"
	out, err := g.output(ctx, "", "ls-remote", remote, "refs/heads/"+branchName)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.TrimSpace(fields[0]) != "" {
			return strings.TrimSpace(fields[0]), nil
		}
	}
	return "", fmt.Errorf("branch %s is absent on %s", branchName, repoSlug)
}

// RemoteBranchExists reports whether the branch exists on the remote.
func (g Git) RemoteBranchExists(ctx context.Context, repoSlug, branchName string) bool {
	_, err := g.RemoteBranchRevision(ctx, repoSlug, branchName)
	return err == nil
}

// Fetch fetches the given refspecs with auth.
func (g Git) Fetch(ctx context.Context, repoDir string, refspecs ...string) error {
	args := append([]string{"fetch", "--force", "origin"}, refspecs...)
	return g.run(ctx, repoDir, args...)
}

// RevParse returns the resolved SHA for a ref in repoDir.
func (g Git) RevParse(ctx context.Context, repoDir, ref string) (string, error) {
	return g.Exec.Output(ctx, repoDir, "git", "rev-parse", ref)
}

// CommitAllowEmpty creates an empty commit with the given messages.
func (g Git) CommitAllowEmpty(ctx context.Context, repoDir string, messages ...string) error {
	args := []string{"commit", "--allow-empty"}
	for _, m := range messages {
		args = append(args, "-m", m)
	}
	return g.Exec.Run(ctx, repoDir, "git", args...)
}

// DeleteRemoteBranch deletes a branch on the remote.
func (g Git) DeleteRemoteBranch(ctx context.Context, repoSlug, branchName string) error {
	remote := "https://github.com/" + repoSlug + ".git"
	return g.run(ctx, "", "push", remote, "--delete", branchName)
}

// ListRemoteHeads returns the branch names under refs/heads/<prefix>* on the
// remote (matching env-destroy.sh's cleanup ls-remote).
func (g Git) ListRemoteHeads(ctx context.Context, repoSlug, prefix string) ([]string, error) {
	remote := "https://github.com/" + repoSlug + ".git"
	out, err := g.output(ctx, "", "ls-remote", "--heads", remote, "refs/heads/"+prefix+"*")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		branches = append(branches, strings.TrimPrefix(fields[1], "refs/heads/"))
	}
	return branches, nil
}

// ChangedFiles mirrors validate.sh's change set: the union of the base...head
// diff, the worktree diff, the staged diff, and untracked files, sorted unique.
func (g Git) ChangedFiles(ctx context.Context, repoDir, baseRef, headRef string) ([]string, error) {
	seen := map[string]bool{}
	add := func(out string) {
		for _, f := range strings.Split(out, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				seen[f] = true
			}
		}
	}
	out, err := g.Exec.Output(ctx, repoDir, "git", "diff", "--name-only", baseRef+"..."+headRef)
	if err != nil {
		return nil, err
	}
	add(out)
	if out, err := g.Exec.OutputQuiet(ctx, repoDir, "git", "diff", "--name-only"); err == nil {
		add(out)
	}
	if out, err := g.Exec.OutputQuiet(ctx, repoDir, "git", "diff", "--name-only", "--cached"); err == nil {
		add(out)
	}
	if out, err := g.Exec.OutputQuiet(ctx, repoDir, "git", "ls-files", "--others", "--exclude-standard"); err == nil {
		add(out)
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sortStrings(files)
	return files, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
