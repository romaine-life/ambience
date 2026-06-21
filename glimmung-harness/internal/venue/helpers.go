package venue

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	evidenceTarStart = "===EVIDENCE-TAR-START==="
	evidenceTarEnd   = "===EVIDENCE-TAR-END==="
)

// InnerJobMarker is the writer EmitInnerJobMarker prints to; nil = os.Stdout.
// Exposed for tests.
var InnerJobMarker io.Writer

// EmitInnerJobMarker prints the inner-Job registration line the Glimmung
// runner parses (===GLIMMUNG-INNER-JOB=== <json>), so Glimmung records the
// child agent Job alongside the outer phase Job and prices its usage lines.
// Mirrors native_emit_inner_job_marker exactly. Intent is the closed enum
// {verification_agent|helper|tooling|unknown}.
func EmitInnerJobMarker(namespace, jobName, intent, label string) error {
	w := InnerJobMarker
	if w == nil {
		w = os.Stdout
	}
	payload := map[string]string{
		"namespace": namespace,
		"job_name":  jobName,
		"intent":    intent,
	}
	if strings.TrimSpace(label) != "" {
		payload["label"] = label
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode inner-job marker: %w", err)
	}
	_, err = fmt.Fprintf(w, "===GLIMMUNG-INNER-JOB=== %s\n", encoded)
	return err
}

// ExtractEvidenceTar mirrors collect_evidence's tar extraction: pull the
// base64 block between the EVIDENCE-TAR markers out of a pod log, decode, and
// untar (gzip) it into evidenceDir. Missing markers is not an error (the agent
// may have produced no evidence); a malformed block returns an error the caller
// logs and continues past, matching the shell's best-effort behavior.
func ExtractEvidenceTar(podLog string, evidenceDir string) error {
	start := strings.Index(podLog, evidenceTarStart)
	if start < 0 {
		return nil
	}
	rest := podLog[start+len(evidenceTarStart):]
	end := strings.Index(rest, evidenceTarEnd)
	if end < 0 {
		return fmt.Errorf("evidence tar start marker without end marker")
	}
	b64 := strings.TrimSpace(rest[:end])
	// The shell sed prints the lines BETWEEN markers; join non-empty lines.
	var compact strings.Builder
	for _, line := range strings.Split(b64, "\n") {
		compact.WriteString(strings.TrimSpace(line))
	}
	raw, err := base64.StdEncoding.DecodeString(compact.String())
	if err != nil {
		return fmt.Errorf("decode evidence base64: %w", err)
	}
	return untarGz(bytes.NewReader(raw), evidenceDir)
}

func untarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		// Guard against path traversal in archive entries.
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}

// AzureLogin mirrors native_azure_login: workload-identity service-principal
// login from the projected federated token, then optional subscription select.
func (e Exec) AzureLogin(ctx context.Context) error {
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	tokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	if clientID == "" || tenantID == "" || tokenFile == "" {
		return fmt.Errorf("azure workload identity env is missing; cannot login")
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		return fmt.Errorf("read federated token: %w", err)
	}
	if err := e.Run(ctx, "", "az", "login", "--service-principal",
		"--username", clientID,
		"--tenant", tenantID,
		"--federated-token", strings.TrimSpace(string(token)),
		"--allow-no-subscriptions"); err != nil {
		return err
	}
	if sub := firstNonEmpty(os.Getenv("AZURE_SUBSCRIPTION_ID"), os.Getenv("ARM_SUBSCRIPTION_ID")); sub != "" {
		return e.Run(ctx, "", "az", "account", "set", "--subscription", sub)
	}
	return nil
}

// InstallPreviewPackage mirrors native_install_preview_package: a no-op when
// ambience_preview.cli already imports, otherwise a user pip install of the mcp
// package directory.
func (e Exec) InstallPreviewPackage(ctx context.Context, packageDir string) error {
	if _, err := e.OutputQuiet(ctx, "", "python3", "-c", "import ambience_preview.cli"); err == nil {
		return nil
	}
	if strings.TrimSpace(packageDir) == "" {
		return fmt.Errorf("ambience preview package directory not found: %q", packageDir)
	}
	if _, err := os.Stat(packageDir); err != nil {
		return fmt.Errorf("ambience preview package directory not found: %s", packageDir)
	}
	if err := e.Run(ctx, "", "python3", "-m", "pip", "install", "--user", "--upgrade", "pip"); err != nil {
		return err
	}
	return e.Run(ctx, "", "python3", "-m", "pip", "install", "--user", packageDir)
}

// PreviewCLI runs `python3 -m ambience_preview.cli <args...>` in dir.
func (e Exec) PreviewCLI(ctx context.Context, dir string, args ...string) error {
	full := append([]string{"-m", "ambience_preview.cli"}, args...)
	return e.Run(ctx, dir, "python3", full...)
}

// PreviewCLIOutput runs the preview CLI capturing stdout.
func (e Exec) PreviewCLIOutput(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-m", "ambience_preview.cli"}, args...)
	return e.Output(ctx, dir, "python3", full...)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
