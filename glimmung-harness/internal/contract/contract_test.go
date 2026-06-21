package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

// 16a: generate the effect implementation contract.
func TestGenerateEffectContract(t *testing.T) {
	c, err := ParseContract(Generate("effect"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.FeatureType != "effect" {
		t.Fatalf("feature_type=%q", c.FeatureType)
	}
	if !contains(c.RequiredFileTemplates, "sim/{effect_snake}.go") {
		t.Errorf("missing required_file_template sim/{effect_snake}.go: %v", c.RequiredFileTemplates)
	}
	if !contains(c.RequiredTouchpoints, "cmd/ambience-wasm/main.go") {
		t.Errorf("missing required touchpoint cmd/ambience-wasm/main.go: %v", c.RequiredTouchpoints)
	}
	foundForbidden := false
	for _, fp := range c.ForbiddenPaths {
		if fp.Pattern == "cmd/ambience/web/effects/*" {
			foundForbidden = true
		}
	}
	if !foundForbidden {
		t.Errorf("missing forbidden pattern cmd/ambience/web/effects/*: %v", c.ForbiddenPaths)
	}
	// well-formed JSON and tagged as implementation_contract
	var raw map[string]any
	if err := json.Unmarshal(Generate("effect"), &raw); err != nil {
		t.Fatalf("effect contract not valid JSON: %v", err)
	}
	if raw["kind"] != "implementation_contract" {
		t.Errorf("kind=%v", raw["kind"])
	}
}

func TestGenerateGenericAndUnsupported(t *testing.T) {
	var generic map[string]any
	_ = json.Unmarshal(Generate(""), &generic)
	if generic["feature_type"] != "generic" {
		t.Errorf("empty feature type should map to generic, got %v", generic["feature_type"])
	}
	var unsupported map[string]any
	_ = json.Unmarshal(Generate("stats-display"), &unsupported)
	if unsupported["status"] != "unsupported" || unsupported["feature_type"] != "stats-display" {
		t.Errorf("unsupported contract wrong: %v", unsupported)
	}
}

func passingImpl(t *testing.T, route string) []byte {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"status":         "pass",
		"ui_hint":        map[string]any{"menu_label": "paper-lanterns", "route": route},
	})
	return b
}

// 16b: a passing implementation that hits every required touchpoint and avoids
// forbidden paths validates.
func TestValidateImplementation_Pass(t *testing.T) {
	changed := []string{
		"sim/paper_lanterns.go",
		"sim/paper_lanterns_test.go",
		"cmd/ambience/effect_paper_lanterns.go",
		"cmd/ambience-wasm/main.go",
	}
	res := ValidateImplementation(Generate("effect"), passingImpl(t, "/dev/paper-lanterns"), changed)
	if res.Status != "pass" {
		t.Fatalf("want pass, got %+v", res)
	}
	if res.EffectSlug != "paper-lanterns" {
		t.Errorf("effect_slug=%q", res.EffectSlug)
	}
	if !contains(res.ChangedFiles, "cmd/ambience-wasm/main.go") {
		t.Errorf("changed_files missing touchpoint: %v", res.ChangedFiles)
	}
}

// 16c: missing required touchpoint is rejected.
func TestValidateImplementation_MissingTouchpoint(t *testing.T) {
	changed := []string{
		"sim/paper_lanterns.go",
		"sim/paper_lanterns_test.go",
		"cmd/ambience/effect_paper_lanterns.go",
		// no cmd/ambience-wasm/main.go
	}
	res := ValidateImplementation(Generate("effect"), passingImpl(t, "/dev/paper-lanterns"), changed)
	if res.AbortReason != "missing_required_touchpoints" {
		t.Fatalf("want missing_required_touchpoints, got %q", res.AbortReason)
	}
	var detail []string
	_ = json.Unmarshal([]byte(res.Detail), &detail)
	if !contains(detail, "cmd/ambience-wasm/main.go") {
		t.Errorf("detail missing touchpoint: %v", detail)
	}
}

// 16d: touching a forbidden path is rejected.
func TestValidateImplementation_ForbiddenPath(t *testing.T) {
	changed := []string{
		"sim/paper_lanterns.go",
		"sim/paper_lanterns_test.go",
		"cmd/ambience/effect_paper_lanterns.go",
		"cmd/ambience-wasm/main.go",
		"cmd/ambience/web/effects/paper-lanterns.js",
	}
	res := ValidateImplementation(Generate("effect"), passingImpl(t, "/dev/paper-lanterns"), changed)
	if res.AbortReason != "forbidden_touchpoint" {
		t.Fatalf("want forbidden_touchpoint, got %q", res.AbortReason)
	}
	var detail []string
	_ = json.Unmarshal([]byte(res.Detail), &detail)
	found := false
	for _, d := range detail {
		if strings.HasPrefix(d, "cmd/ambience/web/effects/paper-lanterns.js") {
			found = true
		}
	}
	if !found {
		t.Errorf("detail missing forbidden file: %v", detail)
	}
}

func TestValidateImplementation_NonPassSkipped(t *testing.T) {
	impl, _ := json.Marshal(map[string]any{"status": "fail"})
	res := ValidateImplementation(Generate("effect"), impl, nil)
	if res.Status != "pass" || res.Skipped == "" {
		t.Fatalf("non-pass impl should skip validation: %+v", res)
	}
}

// 17a: passing implementation with a valid ui_hint clears the gate.
func TestEnforceUIHint_Pass(t *testing.T) {
	impl := map[string]any{
		"status":  "pass",
		"ui_hint": map[string]any{"menu_label": "paper-lanterns", "route": "/dev/paper-lanterns"},
	}
	out := EnforceUIHint(impl, "effect")
	if !out.OK || out.MenuLabel != "paper-lanterns" || out.Route != "/dev/paper-lanterns" {
		t.Fatalf("expected pass, got %+v", out)
	}
	if impl["status"] != "pass" {
		t.Errorf("status mutated on pass: %v", impl["status"])
	}
}

// 17b: passing implementation WITHOUT ui_hint under a standing feature type
// fails, named.
func TestEnforceUIHint_MissingFails(t *testing.T) {
	impl := map[string]any{"status": "pass"}
	out := EnforceUIHint(impl, "effect")
	if out.OK || out.AbortReason != "missing_ui_hint" {
		t.Fatalf("expected missing_ui_hint, got %+v", out)
	}
	if impl["status"] != "fail" || impl["abort_reason"] != "missing_ui_hint" {
		t.Errorf("implementation not mutated to fail: %v", impl)
	}
}

// 17c: a non-/dev route is rejected just like a missing hint.
func TestEnforceUIHint_NonDevRouteFails(t *testing.T) {
	impl := map[string]any{
		"status":  "pass",
		"ui_hint": map[string]any{"menu_label": "x", "route": "https://evil.example/dev/paper-lanterns"},
	}
	out := EnforceUIHint(impl, "effect")
	if out.OK || out.AbortReason != "missing_ui_hint" {
		t.Fatalf("non-/dev route should fail, got %+v", out)
	}
}

// 17d: with no feature type, a missing ui_hint is not an error.
func TestEnforceUIHint_NoFeatureTypeNoOp(t *testing.T) {
	impl := map[string]any{"status": "pass"}
	out := EnforceUIHint(impl, "")
	if !out.OK {
		t.Fatalf("no feature type should be a no-op, got %+v", out)
	}
	if impl["status"] != "pass" {
		t.Errorf("status mutated: %v", impl["status"])
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
