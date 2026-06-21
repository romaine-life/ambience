package planlint

import (
	"encoding/json"
	"strings"
	"testing"
)

func lint(t *testing.T, plan map[string]any) Outcome {
	t.Helper()
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	return Lint(b)
}

func ev(fields ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(fields); i += 2 {
		m[fields[i].(string)] = fields[i+1]
	}
	return m
}

func normalizedItems(t *testing.T, o Outcome) []map[string]any {
	t.Helper()
	var plan map[string]any
	if err := json.Unmarshal(o.Normalized, &plan); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	raw, _ := plan["required_evidence"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		out = append(out, e.(map[string]any))
	}
	return out
}

// 13a / 13f: a valid mixed judged+mechanical plan passes; webm normalizes to
// video; the mechanical case keeps an empty must_show.
func TestLint_ValidMixedPlanPasses(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-lanterns-rise", "kind", "webm", "url_path", "/dev/paper-lanterns",
				"must_show", "a calm field of lanterns drifting upward"),
			ev("id", "dev-lanterns-pulse", "kind", "screenshot", "trigger_event", "pulse",
				"session_config", map[string]any{"emit_every": 0}),
			ev("id", "dev-lanterns-end", "kind", "screenshot", "terminal_lifecycle", "ended"),
		},
	})
	if !o.Pass {
		t.Fatalf("expected pass, got fail %q: %v", o.AbortReason, o.Verdict)
	}
	items := normalizedItems(t, o)
	if len(items) != 3 {
		t.Fatalf("want 3 cases, got %d", len(items))
	}
	if items[0]["kind"] != "video" {
		t.Errorf("webm not normalized to video: %v", items[0]["kind"])
	}
	if str(items[2]["must_show"]) != "" {
		t.Errorf("mechanical case must_show should be empty: %v", items[2]["must_show"])
	}
	// Deterministic session appended to /dev/<effect> url.
	if u := str(items[0]["url_path"]); !strings.Contains(u, "?session=dev-lanterns-rise") {
		t.Errorf("session not injected: %q", u)
	}
}

// 13h: mechanical-only plan (no must_show anywhere) passes.
func TestLint_MechanicalOnlyPasses(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-only", "kind", "screenshot", "session_config", map[string]any{"spawn": 1.0}),
		},
	})
	if !o.Pass {
		t.Fatalf("expected pass, got %q", o.AbortReason)
	}
}

// 13b: non-media kinds are rejected.
func TestLint_UnsupportedKind(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-unit", "kind", "go-test"),
		},
	})
	if o.Pass || o.AbortReason != "unsupported_required_evidence_kind" {
		t.Fatalf("want unsupported_required_evidence_kind, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
	cases, _ := o.Verdict["unsupported_required_evidence"].([]string)
	if len(cases) != 1 || !strings.Contains(cases[0], "dev-unit:go-test") {
		t.Errorf("unexpected unsupported list: %v", cases)
	}
}

// 13c/13d/13e: closed claim vocabulary on must_show.
func TestLint_UnverifiableMustShow(t *testing.T) {
	tests := []struct {
		name, mustShow, wantToken string
	}{
		{"digit", "a cluster of 5 lanterns rises", "digit"},
		{"comparator", "at least a few lanterns", "comparator"},
		{"camelCase", "the emitEvery cadence visibly calms", "camelCase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := lint(t, map[string]any{
				"status": "pass",
				"required_evidence": []any{
					ev("id", "dev-bad", "kind", "video", "must_show", tt.mustShow),
				},
			})
			if o.Pass || o.AbortReason != "unverifiable_must_show" {
				t.Fatalf("want unverifiable_must_show, got pass=%v reason=%q", o.Pass, o.AbortReason)
			}
			cases, _ := o.Verdict["unverifiable_must_show_cases"].([]string)
			if len(cases) != 1 || !strings.Contains(cases[0], "dev-bad") || !strings.Contains(cases[0], tt.wantToken) {
				t.Errorf("case %q missing id/token %q", cases, tt.wantToken)
			}
		})
	}
	// specific quoted-token checks mirroring contract test 13d/13e
	o := lint(t, map[string]any{"status": "pass", "required_evidence": []any{
		ev("id", "x", "kind", "video", "must_show", "at least a few lanterns")}})
	if cs, _ := o.Verdict["unverifiable_must_show_cases"].([]string); !strings.Contains(cs[0], `"at least"`) {
		t.Errorf("comparator should quote 'at least': %v", cs)
	}
	o = lint(t, map[string]any{"status": "pass", "required_evidence": []any{
		ev("id", "x", "kind", "video", "must_show", "the emitEvery cadence calms")}})
	if cs, _ := o.Verdict["unverifiable_must_show_cases"].([]string); !strings.Contains(cs[0], `"emitEvery"`) {
		t.Errorf("camelCase should quote 'emitEvery': %v", cs)
	}
}

// 13i: video without must_show is rejected.
func TestLint_VideoRequiresJudgment(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-lanterns-motion", "kind", "video", "terminal_lifecycle", "ending"),
		},
	})
	if o.Pass || o.AbortReason != "video_requires_judgment" {
		t.Fatalf("want video_requires_judgment, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
	cases, _ := o.Verdict["video_without_judgment_cases"].([]string)
	if len(cases) != 1 || cases[0] != "dev-lanterns-motion" {
		t.Errorf("unexpected list: %v", cases)
	}
}

// 13k: a case with no judgment and no mechanical expectation is rejected.
func TestLint_EmptyCase(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-lanterns-empty", "kind", "screenshot"),
		},
	})
	if o.Pass || o.AbortReason != "empty_case" {
		t.Fatalf("want empty_case, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
}

// 13g: more than MaxJudgedCases judged cases is rejected.
func TestLint_TooManyJudgedCases(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-judged-a", "kind", "screenshot", "must_show", "a soft glow"),
			ev("id", "dev-judged-b", "kind", "screenshot", "must_show", "a warm haze"),
			ev("id", "dev-judged-c", "kind", "screenshot", "must_show", "a gentle drift"),
			ev("id", "dev-judged-d", "kind", "screenshot", "must_show", "a calm field"),
		},
	})
	if o.Pass || o.AbortReason != "too_many_judged_cases" {
		t.Fatalf("want too_many_judged_cases, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
	if o.Verdict["max_judged_cases"] != MaxJudgedCases {
		t.Errorf("max_judged_cases=%v", o.Verdict["max_judged_cases"])
	}
	if cs, _ := o.Verdict["judged_cases"].([]string); len(cs) != 4 {
		t.Errorf("judged_cases len=%d want 4", len(cs))
	}
	if s, _ := o.Verdict["summary"].(string); !strings.Contains(s, "MAX_JUDGED_CASES=3") {
		t.Errorf("summary missing MAX_JUDGED_CASES=3: %q", s)
	}
}

// 13j: a trigger case without a session_config baseline is rejected.
func TestLint_TriggerWithoutBaseline(t *testing.T) {
	o := lint(t, map[string]any{
		"status": "pass",
		"required_evidence": []any{
			ev("id", "dev-lanterns-pulse", "kind", "screenshot", "trigger_event", "pulse"),
		},
	})
	if o.Pass || o.AbortReason != "trigger_case_without_baseline" {
		t.Fatalf("want trigger_case_without_baseline, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
	if s, _ := o.Verdict["summary"].(string); !strings.Contains(s, "quiescent baseline") {
		t.Errorf("summary missing 'quiescent baseline': %q", s)
	}
}

func TestLint_InvalidTerminalLifecycle(t *testing.T) {
	// invalid enum value
	o := lint(t, map[string]any{"status": "pass", "required_evidence": []any{
		ev("id", "t1", "kind", "screenshot", "must_show", "a steady glow", "terminal_lifecycle", "bogus")}})
	if o.Pass || o.AbortReason != "invalid_terminal_lifecycle" {
		t.Fatalf("want invalid_terminal_lifecycle, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
	// retired field present
	o = lint(t, map[string]any{"status": "pass", "required_evidence": []any{
		ev("id", "t2", "kind", "screenshot", "must_show", "a steady glow", "terminal_state_path", "state.phase")}})
	if o.Pass || o.AbortReason != "invalid_terminal_lifecycle" {
		t.Fatalf("retired field: want invalid_terminal_lifecycle, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
}

func TestLint_InvalidSessionConfig(t *testing.T) {
	o := lint(t, map[string]any{"status": "pass", "required_evidence": []any{
		ev("id", "s1", "kind", "screenshot", "must_show", "a steady glow",
			"session_config", map[string]any{"emit": "fast"})}})
	if o.Pass || o.AbortReason != "invalid_session_config" {
		t.Fatalf("want invalid_session_config, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
}

func TestLint_CaseCountBounds(t *testing.T) {
	o := lint(t, map[string]any{"status": "pass", "required_evidence": []any{}})
	if o.Pass || o.AbortReason != "no_required_media_evidence" {
		t.Fatalf("empty: want no_required_media_evidence, got %q", o.AbortReason)
	}
	many := make([]any, 11)
	for i := range many {
		many[i] = ev("id", "c", "kind", "screenshot", "must_show", "a soft glow")
	}
	o = lint(t, map[string]any{"status": "pass", "required_evidence": many})
	if o.Pass || o.AbortReason != "too_many_required_evidence" {
		t.Fatalf("11 cases: want too_many_required_evidence, got %q", o.AbortReason)
	}
	if o.Verdict["required_evidence_count"] != 11 {
		t.Errorf("required_evidence_count=%v", o.Verdict["required_evidence_count"])
	}
}

func TestLint_Malformed(t *testing.T) {
	o := Lint([]byte("{not json"))
	if o.Pass || o.AbortReason != "malformed_test_plan_json" {
		t.Fatalf("want malformed_test_plan_json, got pass=%v reason=%q", o.Pass, o.AbortReason)
	}
}

// 14a: the repo standing case (.github/agent/standing-cases/effect.json),
// wrapped in a passing plan envelope, satisfies the same gates as a generated
// plan. Mirrors the standing-case CI lint.
func TestLint_StandingEffectCasePasses(t *testing.T) {
	o := lint(t, map[string]any{
		"status":      "pass",
		"case_source": "standing",
		"required_evidence": []any{
			ev("id", "effect-acceptance", "kind", "video", "source", "standing",
				"duration_seconds", 10.0,
				"must_show", "a legible ambient effect on the page that plausibly looks like what the issue describes"),
		},
	})
	if !o.Pass {
		t.Fatalf("standing case should pass, got %q: %v", o.AbortReason, o.Verdict)
	}
	items := normalizedItems(t, o)
	if items[0]["kind"] != "video" || items[0]["source"] != "standing" || items[0]["id"] != "effect-acceptance" {
		t.Errorf("standing case fields not preserved: %v", items[0])
	}
}

func TestCaseCount(t *testing.T) {
	b, _ := json.Marshal(map[string]any{"required_evidence": []any{ev("id", "a"), ev("id", "b")}})
	if got := CaseCount(b); got != 2 {
		t.Fatalf("CaseCount=%d want 2", got)
	}
	many := make([]any, 15)
	for i := range many {
		many[i] = ev("id", "c")
	}
	b, _ = json.Marshal(map[string]any{"required_evidence": many})
	if got := CaseCount(b); got != 10 {
		t.Fatalf("CaseCount=%d want 10 (bounded)", got)
	}
}
