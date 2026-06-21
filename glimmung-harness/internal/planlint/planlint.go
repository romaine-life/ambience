// Package planlint ports the test-plan normalization and validation gates from
// the retired scripts/glimmung-native/test-plan.sh finalize step. These gates
// are ambience's evidence-contract: a generated test plan is the spec the later
// (LLM) verify phase captures against, so a malformed or undecidable plan must
// fail HERE — before any verify spend — with a named abort_reason. The logic is
// pure (parse → normalize → guard), so it ports directly to typed, table-tested
// Go with byte-for-byte equivalent verdicts.
//
// Guard order is load-bearing and mirrors the shell exactly: the first failing
// guard wins. See Lint.
package planlint

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MaxJudgedCases is the gestalt cap on judged cases (cases with a non-empty
// must_show) per plan, mirroring test-plan.sh's MAX_JUDGED_CASES.
const MaxJudgedCases = 3

var (
	digitRe      = regexp.MustCompile(`[0-9]+`)
	comparatorRe = regexp.MustCompile(`(?i)(?:>=|<=)|\b(?:no more than|at least|at most|more than|less than|exactly|within|between)\b`)
	camelRe      = regexp.MustCompile(`[a-z]+[A-Z][A-Za-z]*`)
	devPathRe    = regexp.MustCompile(`^/dev/[A-Za-z0-9_-]+`)
	hasSessionRe = regexp.MustCompile(`[?&]session=`)
	nonSessionCh = regexp.MustCompile(`[^a-z0-9_-]`)
)

var terminalLifecycleEnum = map[string]bool{
	"intro": true, "running": true, "ending": true, "ended": true,
}

// Outcome is the result of linting a status=="pass" plan. When Pass is true,
// Normalized holds the normalized plan (kind canonicalized, deterministic
// ?session= appended) to write back. When Pass is false, Verdict holds the fail
// document the wrapper writes (schema_version/status/abort_reason + the
// case-specific diagnostic arrays) and AbortReason names the gate.
type Outcome struct {
	Pass        bool
	Normalized  []byte
	Verdict     map[string]any
	AbortReason string
}

func failOutcome(reason string, doc map[string]any) Outcome {
	doc["schema_version"] = 1
	doc["status"] = "fail"
	doc["abort_reason"] = reason
	return Outcome{Pass: false, Verdict: doc, AbortReason: reason}
}

// Lint normalizes and validates a passing test plan. It assumes the caller has
// already confirmed the plan's status is "pass" (the shell only ran this block
// then). An unparseable plan yields the malformed_test_plan_json verdict.
func Lint(planJSON []byte) Outcome {
	var plan map[string]any
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return failOutcome("malformed_test_plan_json", map[string]any{
			"summary": "Test plan JSON could not be parsed by the native wrapper.",
		})
	}

	items := evidenceItems(plan)

	// 1. Normalize kind + inject deterministic session into /dev/<effect> URLs.
	for _, it := range items {
		it["kind"] = normalizeKind(str(it["kind"]))
		withSession(it)
	}
	plan["required_evidence"] = toAnySlice(items)

	// 2. Case-count bounds.
	if len(items) > 10 {
		return failOutcome("too_many_required_evidence", map[string]any{
			"summary":                  "Test plan produced too many required evidence cases for the bounded verifier.",
			"required_evidence_count":  len(items),
		})
	}
	if len(items) == 0 {
		return failOutcome("no_required_media_evidence", map[string]any{
			"summary": "Test plan passed without any browser media evidence. Ambience LLM verification requires at least one screenshot or video case.",
		})
	}

	// 3. Only screenshot/video media kinds.
	var unsupported []string
	for _, it := range items {
		k := str(it["kind"])
		if k != "video" && k != "screenshot" {
			id := idOr(it)
			kk := k
			if kk == "" {
				kk = "<missing-kind>"
			}
			unsupported = append(unsupported, id+":"+kk)
		}
	}
	if len(unsupported) > 0 {
		return failOutcome("unsupported_required_evidence_kind", map[string]any{
			"summary":                      "Test plan included non-media verification cases. Ambience LLM verification only accepts screenshot and video evidence; deterministic checks belong in PR CI.",
			"unsupported_required_evidence": unsupported,
		})
	}

	// 4. Closed claim vocabulary on must_show.
	var unverifiable []string
	for _, it := range items {
		id := idOr(it)
		raw, present := it["must_show"]
		if !present || raw == nil {
			continue
		}
		ms, isStr := raw.(string)
		if !isStr {
			unverifiable = append(unverifiable, id+": must_show is not a string ("+jqType(raw)+")")
			continue
		}
		if ms == "" {
			continue
		}
		if m := digitRe.FindString(ms); m != "" {
			unverifiable = append(unverifiable, id+`: digit "`+m+`"`)
			continue
		}
		if m := comparatorRe.FindString(ms); m != "" {
			unverifiable = append(unverifiable, id+`: comparator "`+m+`"`)
			continue
		}
		if m := camelRe.FindString(ms); m != "" {
			unverifiable = append(unverifiable, id+`: camelCase identifier "`+m+`"`)
			continue
		}
	}
	if len(unverifiable) > 0 {
		return failOutcome("unverifiable_must_show", map[string]any{
			"summary":                      "Test plan must_show clauses contain digits, comparator phrases, or camelCase identifier tokens. must_show is a single gestalt judgment: judge the look; don't measure — move decidable claims to structured fields (session_config, trigger_event, terminal_lifecycle).",
			"unverifiable_must_show_cases": unverifiable,
		})
	}

	// 5. Video cases must carry a must_show to judge.
	var videoNoJudgment []string
	for _, it := range items {
		if str(it["kind"]) == "video" && str(it["must_show"]) == "" {
			videoNoJudgment = append(videoNoJudgment, idOr(it))
		}
	}
	if len(videoNoJudgment) > 0 {
		return failOutcome("video_requires_judgment", map[string]any{
			"summary":                     "Test plan declared video cases without a must_show. Video evidence exists to be judged; a case with nothing to judge is mechanical-only and uses screenshot/observation evidence.",
			"video_without_judgment_cases": videoNoJudgment,
		})
	}

	// 6. A case must declare at least one verifiable expectation.
	var emptyCases []string
	for _, it := range items {
		if str(it["must_show"]) == "" &&
			str(it["trigger_event"]) == "" &&
			str(it["terminal_lifecycle"]) == "" &&
			sessionConfigLen(it) == 0 {
			emptyCases = append(emptyCases, idOr(it))
		}
	}
	if len(emptyCases) > 0 {
		return failOutcome("empty_case", map[string]any{
			"summary":     "Test plan declared cases with no must_show and no mechanical expectation. A mechanical-only case must declare at least one of trigger_event, terminal_lifecycle, or session_config so the wrapper has something to enforce.",
			"empty_cases": emptyCases,
		})
	}

	// 7. Gestalt cap on judged cases.
	var judged []string
	for _, it := range items {
		if str(it["must_show"]) != "" {
			judged = append(judged, idOr(it))
		}
	}
	if len(judged) > MaxJudgedCases {
		count := len(judged)
		return failOutcome("too_many_judged_cases", map[string]any{
			"summary":           "Test plan has too many judged cases (non-empty must_show); the limit is MAX_JUDGED_CASES=3. Judgment is a gestalt look at a few key moments — keep at most 3 judged cases and express the rest as mechanical-only cases (trigger_event / terminal_lifecycle / session_config, no must_show).",
			"max_judged_cases":  MaxJudgedCases,
			"judged_case_count": itoa(count),
			"judged_cases":      judged,
		})
	}

	// 8. A trigger case needs a quiescent session_config baseline.
	var triggerNoBaseline []string
	for _, it := range items {
		if str(it["trigger_event"]) != "" && sessionConfigLen(it) == 0 {
			triggerNoBaseline = append(triggerNoBaseline, idOr(it))
		}
	}
	if len(triggerNoBaseline) > 0 {
		return failOutcome("trigger_case_without_baseline", map[string]any{
			"summary":                       "Test plan declared trigger_event cases without a session_config baseline. A trigger judged against default background cadence is undecidable; pin a quiescent baseline (suppress competing cadence knobs: emission rates near zero, pulse probabilities 0) so the triggered behavior is isolated.",
			"trigger_cases_without_baseline": triggerNoBaseline,
		})
	}

	// 9. terminal_lifecycle must use the closed enum; retired fields are rejected.
	var invalidTerminal []string
	for _, it := range items {
		_, hasPath := it["terminal_state_path"]
		_, hasEquals := it["terminal_state_equals"]
		bad := hasPath || hasEquals
		if tl, has := it["terminal_lifecycle"]; has {
			s, isStr := tl.(string)
			if !isStr || !terminalLifecycleEnum[s] {
				bad = true
			}
		}
		if bad {
			invalidTerminal = append(invalidTerminal, idOr(it))
		}
	}
	if len(invalidTerminal) > 0 {
		return failOutcome("invalid_terminal_lifecycle", map[string]any{
			"summary":                          "Test plan used retired terminal_state_path/terminal_state_equals or an invalid terminal_lifecycle. Terminal claims assert the lifecycle contract: terminal_lifecycle one of intro|running|ending|ended.",
			"invalid_terminal_lifecycle_cases": invalidTerminal,
		})
	}

	// 10. session_config must be a flat object of numeric knob overrides.
	var invalidSessionConfig []string
	for _, it := range items {
		raw, has := it["session_config"]
		if !has {
			continue
		}
		obj, isObj := raw.(map[string]any)
		if !isObj || !allNumeric(obj) {
			invalidSessionConfig = append(invalidSessionConfig, idOr(it))
		}
	}
	if len(invalidSessionConfig) > 0 {
		return failOutcome("invalid_session_config", map[string]any{
			"summary":                       "Test plan declared session_config that is not a flat object of numeric knob overrides.",
			"invalid_session_config_cases":  invalidSessionConfig,
		})
	}

	normalized, err := json.Marshal(plan)
	if err != nil {
		return failOutcome("malformed_test_plan_json", map[string]any{
			"summary": "Test plan JSON could not be re-encoded by the native wrapper.",
		})
	}
	return Outcome{Pass: true, Normalized: normalized}
}

// CaseCount returns the bounded [0,10] verification case count emitted as the
// test_cases_count phase output, mirroring test-plan.sh emit().
func CaseCount(planJSON []byte) int {
	var plan map[string]any
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return 0
	}
	n := len(evidenceItems(plan))
	if n > 10 {
		return 10
	}
	return n
}

func evidenceItems(plan map[string]any) []map[string]any {
	raw, ok := plan["required_evidence"].([]any)
	if !ok {
		// emit() also tolerates .cases / .test_cases for the count; the lint
		// path is required_evidence-only, matching finalize().
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func toAnySlice(items []map[string]any) []any {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "animation", "webm", "movie", "recording":
		return "video"
	case "image", "still":
		return "screenshot"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func withSession(it map[string]any) {
	url := str(it["url_path"])
	if url == "" {
		return
	}
	if !devPathRe.MatchString(url) || hasSessionRe.MatchString(url) {
		return
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	it["url_path"] = url + sep + "session=" + sessionForID(it)
}

func sessionForID(it map[string]any) string {
	id := str(it["id"])
	if id == "" {
		id = "case"
	}
	return nonSessionCh.ReplaceAllString(strings.ToLower(id), "-")
}

func sessionConfigLen(it map[string]any) int {
	obj, ok := it["session_config"].(map[string]any)
	if !ok {
		return 0
	}
	return len(obj)
}

func allNumeric(obj map[string]any) bool {
	for _, v := range obj {
		if _, ok := v.(float64); !ok {
			if _, ok := v.(json.Number); !ok {
				return false
			}
		}
	}
	return true
}

func idOr(it map[string]any) string {
	if id := str(it["id"]); id != "" {
		return id
	}
	return "<missing-id>"
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func jqType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, json.Number:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
