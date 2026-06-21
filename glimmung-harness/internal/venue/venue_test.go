package venue

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitAuthHeader(t *testing.T) {
	got := GitAuthHeader("secret-token")
	want := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:secret-token"))
	if got != want {
		t.Fatalf("GitAuthHeader=%q want %q", got, want)
	}
}

func TestEmitInnerJobMarker(t *testing.T) {
	var buf bytes.Buffer
	InnerJobMarker = &buf
	defer func() { InnerJobMarker = nil }()

	if err := EmitInnerJobMarker("glim-run-x", "agent-x-im-0", "helper", "implement-agent"); err != nil {
		t.Fatal(err)
	}
	line := buf.String()
	if !strings.HasPrefix(line, "===GLIMMUNG-INNER-JOB=== ") {
		t.Fatalf("missing marker prefix: %q", line)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(line), "===GLIMMUNG-INNER-JOB=== ")), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if payload["namespace"] != "glim-run-x" || payload["job_name"] != "agent-x-im-0" ||
		payload["intent"] != "helper" || payload["label"] != "implement-agent" {
		t.Fatalf("payload fields wrong: %v", payload)
	}

	// Label omitted when empty (mirrors the shell's two-branch jq).
	buf.Reset()
	if err := EmitInnerJobMarker("ns", "job", "verification_agent", ""); err != nil {
		t.Fatal(err)
	}
	var noLabel map[string]string
	_ = json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(buf.String()), "===GLIMMUNG-INNER-JOB=== ")), &noLabel)
	if _, ok := noLabel["label"]; ok {
		t.Fatalf("label should be omitted when empty: %v", noLabel)
	}
}

func TestExtractEvidenceTar(t *testing.T) {
	// Build a gzip tar with one evidence file, base64 it between markers.
	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	content := []byte("a fake screenshot")
	_ = tw.WriteHeader(&tar.Header{Name: "screenshots/effect.png", Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gz.Close()
	b64 := base64.StdEncoding.EncodeToString(tarBuf.Bytes())

	podLog := "noise before\n" + evidenceTarStart + "\n" + b64 + "\n" + evidenceTarEnd + "\nnoise after\n"

	dir := t.TempDir()
	if err := ExtractEvidenceTar(podLog, dir); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "screenshots", "effect.png"))
	if err != nil {
		t.Fatalf("evidence file missing: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("evidence content mismatch: %q", got)
	}

	// No markers is a no-op, not an error.
	if err := ExtractEvidenceTar("nothing here", dir); err != nil {
		t.Fatalf("no markers should be a no-op: %v", err)
	}
}

func TestChecksSnapshotClassify(t *testing.T) {
	tests := []struct {
		name string
		snap ChecksSnapshot
		want CheckVerdict
	}{
		{"no-signal", ChecksSnapshot{CombinedState: "pending"}, CheckPending},
		{"failing-run", ChecksSnapshot{CheckRuns: []CheckRun{{Status: "completed", Conclusion: "failure"}}, CombinedState: "pending"}, CheckFailed},
		{"failing-status", ChecksSnapshot{CombinedState: "failure", StatusCount: 1}, CheckFailed},
		{"pending-run", ChecksSnapshot{CheckRuns: []CheckRun{{Status: "in_progress"}}, CombinedState: "pending"}, CheckPending},
		{"all-green-runs", ChecksSnapshot{CheckRuns: []CheckRun{{Status: "completed", Conclusion: "success"}}, CombinedState: "pending"}, CheckPassed},
		{"green-with-status", ChecksSnapshot{CheckRuns: []CheckRun{{Status: "completed", Conclusion: "success"}}, CombinedState: "success", StatusCount: 1}, CheckPassed},
		{"neutral-skipped-ok", ChecksSnapshot{CheckRuns: []CheckRun{{Status: "completed", Conclusion: "neutral"}, {Status: "completed", Conclusion: "skipped"}}, CombinedState: "pending"}, CheckPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.snap.Classify(); got != tt.want {
				t.Fatalf("Classify=%d want %d", got, tt.want)
			}
		})
	}
}
