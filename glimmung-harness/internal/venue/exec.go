// Package venue holds the in-cluster execution helpers the ambience run-harness
// phase handlers orchestrate: command execution, git over the per-attempt
// GitHub token, the GitHub REST surface, Azure workload-identity login, the
// preview-package install, evidence-tar extraction, and the inner-Job
// observation marker. These are the I/O seams of the retired
// scripts/glimmung-native/lib.sh fork, re-expressed as typed Go that the SDK
// step handlers call.
//
// ambience's venue is IN-CLUSTER and single-faced: there is no remote host and
// no host subcommand. The model runs in a child k8s Job (rendered by the
// ambience_preview Python venue tooling, which this package shells out to),
// registered for Glimmung observation via EmitInnerJobMarker — which is how the
// agent's usage lines are priced, the in-cluster analogue of spirelens's
// line-streamed RunSelf over ssh.
package venue

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Exec runs external commands, streaming their combined output to Stdout/Stderr
// so the runner's log stream observes every line. The zero value streams to the
// process stdout/stderr.
type Exec struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (e Exec) stdout() io.Writer {
	if e.Stdout != nil {
		return e.Stdout
	}
	return os.Stdout
}

func (e Exec) stderr() io.Writer {
	if e.Stderr != nil {
		return e.Stderr
	}
	return os.Stderr
}

// Run executes name+args in dir (empty = current), streaming output. A non-nil
// error carries the command line and exit status.
func (e Exec) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = e.stdout()
	cmd.Stderr = e.stderr()
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

// RunInput executes a command feeding stdin from r, streaming output.
func (e Exec) RunInput(ctx context.Context, dir, name string, r io.Reader, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = e.stdout()
	cmd.Stderr = e.stderr()
	cmd.Stdin = r
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

// Output executes a command and returns its trimmed stdout. Stderr streams to
// the runner log so failures stay visible; a non-nil error carries the command
// line and exit status.
func (e Exec) Output(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = e.stderr()
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return strings.TrimSpace(out.String()), nil
}

// OutputQuiet is Output but swallows stderr (for best-effort probes the shell
// ran with 2>/dev/null).
func (e Exec) OutputQuiet(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func commandLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}
