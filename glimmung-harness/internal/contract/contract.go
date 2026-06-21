// Package contract ports the runner-owned implementation-contract logic from
// the retired scripts/agent/contracts/{generate,validate,effect}.sh and the
// implement.sh ui_hint gate. Contract generation declares the repo touchpoints
// for a feature type; validation checks the implementation's changed files
// against that contract; the ui_hint gate fails a passing effect implementation
// that omits its discovery hint BEFORE any verify spend. All three are pure
// (the git-diff that produces the changed-file set lives in the handler), so
// they port to typed, table-tested Go.
package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed effect_contract.json
var effectContractJSON []byte

var devRouteRe = regexp.MustCompile(`^/dev/[a-z0-9_-]`)

// Generate returns the implementation contract JSON for a feature type,
// mirroring scripts/agent/contracts/generate.sh. "effect" returns the embedded
// effect contract; "" and "generic" return the generic contract; any other
// type returns an explicit "unsupported" contract.
func Generate(featureType string) []byte {
	switch strings.TrimSpace(featureType) {
	case "effect":
		// Re-marshal to guarantee compact, well-formed output regardless of the
		// embedded file's formatting.
		var v any
		_ = json.Unmarshal(effectContractJSON, &v)
		out, _ := json.Marshal(v)
		return out
	case "", "generic":
		out, _ := json.Marshal(map[string]any{
			"schema_version":      1,
			"kind":                "implementation_contract",
			"project":             "ambience",
			"feature_type":        "generic",
			"summary":             "Generic ambience implementation contract. Follow the issue and repo docs; no feature-type-specific touchpoint rules are declared.",
			"read_first":          []string{"AGENTS.md", "CLAUDE.md"},
			"required_outputs":    []string{"implementation"},
			"validation_commands": []string{"git diff --check"},
			"checks":              []any{},
		})
		return out
	default:
		out, _ := json.Marshal(map[string]any{
			"schema_version":      1,
			"kind":                "implementation_contract",
			"project":             "ambience",
			"feature_type":        strings.TrimSpace(featureType),
			"status":              "unsupported",
			"summary":             "No ambience implementation contract generator is registered for this feature_type yet.",
			"read_first":          []string{"AGENTS.md", "CLAUDE.md"},
			"required_outputs":    []string{"implementation"},
			"validation_commands": []string{"git diff --check"},
			"checks":              []any{},
		})
		return out
	}
}

// Contract is the subset of the implementation contract the validator reads.
type Contract struct {
	FeatureType           string `json:"feature_type"`
	RequiredFileTemplates []string
	RequiredTouchpoints   []string
	ForbiddenPaths        []ForbiddenPath
}

// ForbiddenPath is a forbidden touchpoint pattern with its rationale.
type ForbiddenPath struct {
	Pattern string
	Reason  string
}

// ParseContract decodes the contract fields the validator needs.
func ParseContract(data []byte) (Contract, error) {
	var raw struct {
		FeatureType           string   `json:"feature_type"`
		RequiredFileTemplates []string `json:"required_file_templates"`
		RequiredTouchpoints   []struct {
			Path string `json:"path"`
		} `json:"required_touchpoints"`
		ForbiddenPaths []struct {
			Pattern string `json:"pattern"`
			Reason  string `json:"reason"`
		} `json:"forbidden_paths"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Contract{}, err
	}
	c := Contract{FeatureType: raw.FeatureType, RequiredFileTemplates: raw.RequiredFileTemplates}
	for _, t := range raw.RequiredTouchpoints {
		if strings.TrimSpace(t.Path) != "" {
			c.RequiredTouchpoints = append(c.RequiredTouchpoints, t.Path)
		}
	}
	for _, f := range raw.ForbiddenPaths {
		if strings.TrimSpace(f.Pattern) != "" {
			c.ForbiddenPaths = append(c.ForbiddenPaths, ForbiddenPath{Pattern: f.Pattern, Reason: f.Reason})
		}
	}
	return c, nil
}

// ValidationResult is the validate.sh verdict.
type ValidationResult struct {
	Status       string   `json:"status"`
	AbortReason  string   `json:"abort_reason,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	Skipped      string   `json:"skipped,omitempty"`
	FeatureType  string   `json:"feature_type,omitempty"`
	EffectSlug   string   `json:"effect_slug,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
}

func failResult(reason, detail string) ValidationResult {
	return ValidationResult{Status: "fail", AbortReason: reason, Detail: detail}
}

// ValidateImplementation mirrors scripts/agent/contracts/validate.sh against an
// already-computed changedFiles set (the handler runs the git diff). It returns
// a pass result with skip metadata when the implementation isn't a passing
// effect, and a fail result naming the gate otherwise.
func ValidateImplementation(contractJSON, implementationJSON []byte, changedFiles []string) ValidationResult {
	c, err := ParseContract(contractJSON)
	if err != nil {
		return failResult("invalid_contract", "contract is not valid JSON")
	}
	var impl struct {
		Status string `json:"status"`
		UIHint struct {
			Route string `json:"route"`
		} `json:"ui_hint"`
	}
	if err := json.Unmarshal(implementationJSON, &impl); err != nil {
		return failResult("invalid_implementation", "implementation is not valid JSON")
	}

	if impl.Status != "pass" {
		return ValidationResult{Status: "pass", Skipped: "implementation status is not pass"}
	}
	if c.FeatureType != "effect" {
		return ValidationResult{Status: "pass", Skipped: "no validator for feature_type", FeatureType: c.FeatureType}
	}

	route := strings.TrimSpace(impl.UIHint.Route)
	if !devRouteRe.MatchString(route) {
		return failResult("missing_ui_hint", fmt.Sprintf("effect contracts require ui_hint.route shaped as /dev/<effect>; got %s", orEmpty(route)))
	}
	effectSlug := strings.TrimPrefix(route, "/dev/")
	effectSnake := strings.ReplaceAll(effectSlug, "-", "_")
	if effectSnake == "" {
		return failResult("missing_ui_hint", "could not derive effect_snake from route "+route)
	}

	if len(changedFiles) == 0 {
		return failResult("empty_change", "no committed or worktree files changed relative to base")
	}
	changed := map[string]bool{}
	for _, f := range changedFiles {
		changed[f] = true
	}

	var missing []string
	for _, tmpl := range c.RequiredFileTemplates {
		path := strings.ReplaceAll(tmpl, "{effect_snake}", effectSnake)
		if !changed[path] {
			missing = append(missing, path)
		}
	}
	for _, path := range c.RequiredTouchpoints {
		if !changed[path] {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		detail, _ := json.Marshal(missing)
		return failResult("missing_required_touchpoints", string(detail))
	}

	var forbidden []string
	for _, fp := range c.ForbiddenPaths {
		re := globToRegexp(fp.Pattern)
		for _, path := range changedFiles {
			if re.MatchString(path) {
				forbidden = append(forbidden, path+": "+fp.Reason)
			}
		}
	}
	if len(forbidden) > 0 {
		detail, _ := json.Marshal(forbidden)
		return failResult("forbidden_touchpoint", string(detail))
	}

	return ValidationResult{
		Status:       "pass",
		FeatureType:  c.FeatureType,
		EffectSlug:   effectSlug,
		ChangedFiles: changedFiles,
	}
}

// globToRegexp converts a shell case-glob (where '*' matches any run including
// '/', as bash case patterns do) into an anchored regexp, matching the shell's
// `case "$path" in $pattern)` semantics in validate.sh.
func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func orEmpty(s string) string {
	if s == "" {
		return "<empty>"
	}
	return s
}

// UIHintOutcome is the result of the implement-phase ui_hint gate.
type UIHintOutcome struct {
	OK          bool   // true when the gate passes (or doesn't apply)
	AbortReason string // set on failure ("missing_ui_hint")
	MenuLabel   string
	Route       string
}

// EnforceUIHint mirrors implement.sh's enforce_ui_hint_contract. For a feature
// type with a standing verification case, a PASSING implementation must declare
// a usable ui_hint {menu_label, route:/dev/<effect>}; otherwise the gate fails
// here, named, before the expensive verify phase binds the standing case.
//
// It mutates implementation (a decoded JSON object) in place on failure to set
// status=fail / abort_reason=missing_ui_hint / an appended summary, matching the
// shell, and returns the outcome.
func EnforceUIHint(implementation map[string]any, featureType string) UIHintOutcome {
	if strings.TrimSpace(featureType) == "" {
		return UIHintOutcome{OK: true}
	}
	if str(implementation["status"]) != "pass" {
		return UIHintOutcome{OK: true}
	}
	label, route := "", ""
	if hint, ok := implementation["ui_hint"].(map[string]any); ok {
		label = strings.TrimSpace(str(hint["menu_label"]))
		route = strings.TrimSpace(str(hint["route"]))
	}
	if !devRouteRe.MatchString(route) {
		route = ""
	}
	if label == "" || route == "" {
		got := "null"
		if hint, ok := implementation["ui_hint"]; ok {
			if b, err := json.Marshal(hint); err == nil {
				got = string(b)
			}
		}
		implementation["status"] = "fail"
		implementation["abort_reason"] = "missing_ui_hint"
		prev := str(implementation["summary"])
		implementation["summary"] = strings.TrimSpace(prev) +
			" [wrapper: feature_type requires ui_hint {menu_label, route:/dev/<effect>} on a passing implementation; got " + got + "]"
		return UIHintOutcome{OK: false, AbortReason: "missing_ui_hint"}
	}
	return UIHintOutcome{OK: true, MenuLabel: label, Route: route}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
