package migrationguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to the ambience repository root (the
// directory containing .git).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (.git) from %s", dir)
		}
		dir = parent
	}
}

// The retired shell run-harness paths must stay deleted.
func TestRetiredHarnessPathsAreGone(t *testing.T) {
	root := repoRoot(t)
	// The RUNNER shell harness is retired. scripts/agent/ is a distinct surface
	// (agent-container tooling invoked inside inner Jobs by the Python venue) and
	// is intentionally NOT asserted gone here.
	retired := []string{
		"scripts/glimmung-native",
		"scripts/test-glimmung-native-contract.sh",
	}
	for _, rel := range retired {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("retired shell harness path reappeared: %s", rel)
		}
	}
}

// No retired lib.sh sentinel symbol may reappear in a live shell or CI/workflow
// file. Go source is exempt: package comments documenting what was ported
// legitimately name the retired functions.
func TestNoRetiredSentinelSymbols(t *testing.T) {
	root := repoRoot(t)
	sentinels := []string{
		"native_emit_output",
		"native_emit_json_output",
		"native_emit_abort",
		"native_run_selected_step",
		"native_completed",
		"native_failed",
		"native_step",
		"native_github_token",
		"native_image_tag_for_revision",
		"native_implementation_branch_name",
	}
	scanExt := map[string]bool{".sh": true, ".yml": true, ".yaml": true, ".bash": true}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !scanExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		body := string(data)
		for _, s := range sentinels {
			if strings.Contains(body, s) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("retired sentinel %q reappeared in %s", s, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
