#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"${TMP_DIR}/curl" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

: "${NATIVE_CONTRACT_CURL_CAPTURE:?set NATIVE_CONTRACT_CURL_CAPTURE}"

data=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -d)
      shift
      data="${1:-}"
      ;;
    -H|--retry|--retry-delay|-X)
      shift
      ;;
    --retry-all-errors|-fsS)
      ;;
    *)
      url="$1"
      ;;
  esac
  shift || true
done

printf '%s\n' "$url" >"${NATIVE_CONTRACT_CURL_CAPTURE}.url"
printf '%s\n' "$data" >"${NATIVE_CONTRACT_CURL_CAPTURE}.body"
SH
chmod +x "${TMP_DIR}/curl"

cat >"${TMP_DIR}/python3" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

: "${NATIVE_CONTRACT_PYTHON_CAPTURE:?set NATIVE_CONTRACT_PYTHON_CAPTURE}"

if [ "${1:-}" = "-c" ]; then
  exit 1
fi

printf '%s\n' "$*" >>"$NATIVE_CONTRACT_PYTHON_CAPTURE"
SH
chmod +x "${TMP_DIR}/python3"

unset GLIMMUNG_FAILED_URL
export GLIMMUNG_ATTEMPT_TOKEN="contract-token"
export GLIMMUNG_EVENTS_URL="http://glimmung.test/v1/run-callbacks/cb/native/events"
export GLIMMUNG_COMPLETED_URL="http://glimmung.test/v1/run-callbacks/cb/native/completed"
export GLIMMUNG_GITHUB_TOKEN_URL="http://glimmung.test/v1/run-callbacks/cb/native/github-token"
export GLIMMUNG_JOB_ID="env-prep"
export GLIMMUNG_RUN_ID="run-1"
export NATIVE_CONTRACT_CURL_CAPTURE="${TMP_DIR}/native-failed"
export NATIVE_CONTRACT_PYTHON_CAPTURE="${TMP_DIR}/python.calls"
export PATH="${TMP_DIR}:${PATH}"

# shellcheck source=glimmung-native/lib.sh
source "${SCRIPT_DIR}/glimmung-native/lib.sh"

REVISION="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
[ "$(native_image_tag_for_revision "$REVISION")" = "git-${REVISION}" ]
if native_image_tag_for_revision "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" >/dev/null 2>&1; then
  echo "native_image_tag_for_revision must reject non-40-character revisions" >&2
  exit 1
fi
unset AGENT_CONTAINER_IMAGE AGENT_CONTAINER_TAG
if native_agent_container_image >/dev/null 2>&1; then
  echo "native_agent_container_image must reject missing image inputs" >&2
  exit 1
fi
AGENT_CONTAINER_TAG="native-runner-test"
[ "$(native_agent_container_image)" = "romainecr.azurecr.io/ambience-agent-runner:native-runner-test" ]

unset AMBIENCE_IMPLEMENTATION_BRANCH GLIMMUNG_WORK_CONTEXT_BRANCH GLIMMUNG_ISSUE_NUMBER
[ "$(native_implementation_branch_name)" = "glimmung/run-1" ]
GLIMMUNG_WORK_CONTEXT_BRANCH="glimmung/work-context"
[ "$(native_implementation_branch_name)" = "glimmung/work-context" ]
GLIMMUNG_ISSUE_NUMBER="168"
[ "$(native_implementation_branch_name)" = "glimmung/issue-168/run-1" ]
[ "$(native_issue_branch_prefix)" = "glimmung/issue-168/" ]
AMBIENCE_IMPLEMENTATION_BRANCH="glimmung/manual-branch"
[ "$(native_implementation_branch_name)" = "glimmung/manual-branch" ]
unset AMBIENCE_IMPLEMENTATION_BRANCH GLIMMUNG_WORK_CONTEXT_BRANCH GLIMMUNG_ISSUE_NUMBER
AGENT_CONTAINER_IMAGE="romainecr.azurecr.io/custom:tag"
[ "$(native_agent_container_image)" = "romainecr.azurecr.io/custom:tag" ]
unset AGENT_CONTAINER_IMAGE AGENT_CONTAINER_TAG
unset GLIMMUNG_RUN_INPUT_GIT_REF AMBIENCE_WORKFLOW_REF
[ "$(native_workflow_checkout_ref)" = "main" ]
AMBIENCE_WORKFLOW_REF="ambience-workflow-ref"
[ "$(native_workflow_checkout_ref)" = "ambience-workflow-ref" ]
GLIMMUNG_RUN_INPUT_GIT_REF="codex/lifecycle-observe"
[ "$(native_workflow_checkout_ref)" = "codex/lifecycle-observe" ]
unset GLIMMUNG_RUN_INPUT_GIT_REF AMBIENCE_WORKFLOW_REF

mkdir -p "${TMP_DIR}/repo/mcp"
native_install_preview_package "${TMP_DIR}/repo/mcp"
grep -Fx -- "-m pip install --user --upgrade pip" "$NATIVE_CONTRACT_PYTHON_CAPTURE" >/dev/null
grep -Fx -- "-m pip install --user ${TMP_DIR}/repo/mcp" "$NATIVE_CONTRACT_PYTHON_CAPTURE" >/dev/null

native_init
native_failed "contract failure"

if [ "$(cat "${NATIVE_CONTRACT_CURL_CAPTURE}.url")" != "$GLIMMUNG_COMPLETED_URL" ]; then
  echo "native_failed must post to GLIMMUNG_COMPLETED_URL" >&2
  exit 1
fi

jq -e '
  .conclusion == "failure"
  and .job_id == "env-prep"
  and .summary_markdown == "contract failure"
' "${NATIVE_CONTRACT_CURL_CAPTURE}.body" >/dev/null

export GLIMMUNG_MANAGED_RUNNER=1
export GLIMMUNG_OUTPUT_FILE="${TMP_DIR}/managed-output.jsonl"
export GLIMMUNG_COMPLETION_FILE="${TMP_DIR}/managed-completion.json"
rm -f "$GLIMMUNG_OUTPUT_FILE" "$GLIMMUNG_COMPLETION_FILE" "${NATIVE_CONTRACT_CURL_CAPTURE}.url" "${NATIVE_CONTRACT_CURL_CAPTURE}.body"

native_completed \
  '{"validation_url":"https://preview.example"}' \
  '{"status":"pass","reasons":["ok"]}' \
  '![screen](https://example.test/screen.png)' \
  'managed summary' \
  '[{"kind":"video","ref":"videos/demo.webm","content_type":"video/webm"}]'

jq -e '.validation_url == "https://preview.example"' "$GLIMMUNG_OUTPUT_FILE" >/dev/null
jq -e '
  .verification.status == "pass"
  and .screenshots_markdown == "![screen](https://example.test/screen.png)"
  and .summary_markdown == "managed summary"
  and .evidence[0].kind == "video"
  and .evidence[0].ref == "videos/demo.webm"
' "$GLIMMUNG_COMPLETION_FILE" >/dev/null

if [ -e "${NATIVE_CONTRACT_CURL_CAPTURE}.url" ]; then
  echo "managed native_completed must not post callbacks" >&2
  exit 1
fi

native_failed "managed failure"
jq -e '.summary_markdown == "managed failure"' "$GLIMMUNG_COMPLETION_FILE" >/dev/null

export GLIMMUNG_STEP_SLUG="selected"
SELECTED_MARKER="${TMP_DIR}/selected-marker"
unselected_step() {
  echo "unselected step should not run" >&2
  exit 1
}
selected_step() {
  printf 'ran\n' >"$SELECTED_MARKER"
}
native_run_selected_step \
  "unselected" unselected_step \
  "selected" selected_step
grep -Fx "ran" "$SELECTED_MARKER" >/dev/null

EXIT_CODE_FILE="${TMP_DIR}/exit-code"
failing_step() {
  return 7
}
native_record_exit_code "$EXIT_CODE_FILE" failing_step
[ "$(native_read_exit_code "$EXIT_CODE_FILE")" = "7" ]

PLAN_EVIDENCE_DIR="${TMP_DIR}/plan-evidence"
mkdir -p "$PLAN_EVIDENCE_DIR"
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-demo",
      "kind": "webm",
      "url_path": "/dev/demo",
      "must_show": "demo animates"
    }
  ]
}
JSON
AMBIENCE_EVIDENCE_DIR="$PLAN_EVIDENCE_DIR" \
AMBIENCE_TEST_PLAN_VALIDATE_ONLY=1 \
GLIMMUNG_INPUT_VALIDATION_URL="https://preview.test" \
GLIMMUNG_INPUT_NAMESPACE="preview-ns" \
bash "${SCRIPT_DIR}/glimmung-native/test-plan.sh" >"${TMP_DIR}/validated-plan.json"
jq -e '.status == "pass" and .required_evidence[0].kind == "video"' "${TMP_DIR}/validated-plan.json" >/dev/null

cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "tests-demo",
      "kind": "go-test",
      "command": "go test ./..."
    }
  ]
}
JSON
set +e
AMBIENCE_EVIDENCE_DIR="$PLAN_EVIDENCE_DIR" \
AMBIENCE_TEST_PLAN_VALIDATE_ONLY=1 \
GLIMMUNG_INPUT_VALIDATION_URL="https://preview.test" \
GLIMMUNG_INPUT_NAMESPACE="preview-ns" \
bash "${SCRIPT_DIR}/glimmung-native/test-plan.sh" >"${TMP_DIR}/invalid-plan.json"
PLAN_VALIDATE_RC=$?
set -e
[ "$PLAN_VALIDATE_RC" -eq 1 ]
jq -e '.status == "fail" and .abort_reason == "unsupported_required_evidence_kind"' "${TMP_DIR}/invalid-plan.json" >/dev/null

# --- closed claim vocabulary: test-plan wrapper guards ---

run_plan_validate() {
  set +e
  AMBIENCE_EVIDENCE_DIR="$PLAN_EVIDENCE_DIR" \
  AMBIENCE_TEST_PLAN_VALIDATE_ONLY=1 \
  GLIMMUNG_INPUT_VALIDATION_URL="https://preview.test" \
  GLIMMUNG_INPUT_NAMESPACE="preview-ns" \
  bash "${SCRIPT_DIR}/glimmung-native/test-plan.sh" >"$1"
  PLAN_VALIDATE_RC=$?
  set -e
}

expect_plan_reject() {
  local out="$1" reason="$2"
  run_plan_validate "$out"
  if [ "$PLAN_VALIDATE_RC" -ne 1 ]; then
    echo "expected plan validation to exit 1 (${reason}), got ${PLAN_VALIDATE_RC}" >&2
    exit 1
  fi
  jq -e --arg reason "$reason" '.status == "fail" and .abort_reason == $reason' "$out" >/dev/null
}

# digit in must_show → unverifiable_must_show
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-cluster",
      "kind": "video",
      "url_path": "/dev/lanterns",
      "must_show": "a cluster of 5 lanterns rises together"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-digit.json" "unverifiable_must_show"
jq -e '
  (.unverifiable_must_show_cases | any(test("dev-lanterns-cluster")))
  and (.unverifiable_must_show_cases | any(test("digit")))
  and (.summary | test("judge the look"))
' "${TMP_DIR}/plan-digit.json" >/dev/null

# comparator phrase in must_show → unverifiable_must_show
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-few",
      "kind": "video",
      "url_path": "/dev/lanterns",
      "must_show": "at least a few lanterns drift upward"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-comparator.json" "unverifiable_must_show"
jq -e '
  .unverifiable_must_show_cases
  | any(test("dev-lanterns-few") and test("comparator") and test("at least"))
' "${TMP_DIR}/plan-comparator.json" >/dev/null

# camelCase identifier token in must_show → unverifiable_must_show
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-cadence",
      "kind": "video",
      "url_path": "/dev/lanterns",
      "must_show": "the emitEvery cadence visibly calms"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-camelcase.json" "unverifiable_must_show"
jq -e '
  .unverifiable_must_show_cases
  | any(test("dev-lanterns-cadence") and test("camelCase") and test("emitEvery"))
' "${TMP_DIR}/plan-camelcase.json" >/dev/null

# valid plan: gestalt judged cases (kebab-case trigger names allowed in
# prose), trigger cases pinned to a quiescent baseline, and one
# mechanical-only case — passes whole
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-default",
      "kind": "webm",
      "url_path": "/dev/lanterns",
      "must_show": "warm lantern glows drifting upward against a dark sky"
    },
    {
      "id": "dev-lanterns-release",
      "kind": "screenshot",
      "url_path": "/dev/lanterns",
      "must_show": "a release-pulse burst reads as one bright clustered moment",
      "trigger_event": "release-pulse",
      "session_config": {"emit_every": 9999, "release_pulse_p": 0}
    },
    {
      "id": "dev-lanterns-ending",
      "kind": "screenshot",
      "url_path": "/dev/lanterns",
      "trigger_event": "ending",
      "terminal_lifecycle": "ended",
      "hold_ticks": 12,
      "session_config": {"emit_every": 9999}
    }
  ]
}
JSON
run_plan_validate "${TMP_DIR}/plan-vocab-valid.json"
if [ "$PLAN_VALIDATE_RC" -ne 0 ]; then
  echo "expected mixed judged/mechanical plan to validate, got ${PLAN_VALIDATE_RC}" >&2
  cat "${TMP_DIR}/plan-vocab-valid.json" >&2
  exit 1
fi
jq -e '
  .status == "pass"
  and (.required_evidence | length == 3)
  and (.required_evidence[0].kind == "video")
  and ((.required_evidence[2].must_show // "") == "")
' "${TMP_DIR}/plan-vocab-valid.json" >/dev/null

# four judged cases → too_many_judged_cases names the limit and the ids
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {"id": "dev-judged-a", "kind": "video", "url_path": "/dev/lanterns", "must_show": "a calm warm glow"},
    {"id": "dev-judged-b", "kind": "video", "url_path": "/dev/lanterns", "must_show": "a cool dim sky"},
    {"id": "dev-judged-c", "kind": "video", "url_path": "/dev/lanterns", "must_show": "a bright moon rising"},
    {"id": "dev-judged-d", "kind": "video", "url_path": "/dev/lanterns", "must_show": "a soft drifting haze"}
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-too-many-judged.json" "too_many_judged_cases"
jq -e '
  .max_judged_cases == 3
  and (.judged_cases | length == 4)
  and (.judged_cases | index("dev-judged-d") != null)
  and (.summary | test("MAX_JUDGED_CASES=3"))
' "${TMP_DIR}/plan-too-many-judged.json" >/dev/null

# mechanical-only plan (no must_show anywhere) is accepted
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-quiet-baseline",
      "kind": "screenshot",
      "url_path": "/dev/lanterns",
      "session_config": {"emit_every": 9999}
    }
  ]
}
JSON
run_plan_validate "${TMP_DIR}/plan-mechanical-only.json"
if [ "$PLAN_VALIDATE_RC" -ne 0 ]; then
  echo "expected mechanical-only plan to validate, got ${PLAN_VALIDATE_RC}" >&2
  cat "${TMP_DIR}/plan-mechanical-only.json" >&2
  exit 1
fi
jq -e '.status == "pass" and (.required_evidence | length == 1)' \
  "${TMP_DIR}/plan-mechanical-only.json" >/dev/null

# video without must_show → video_requires_judgment
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-motion",
      "kind": "video",
      "url_path": "/dev/lanterns",
      "terminal_lifecycle": "running"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-video-mechanical.json" "video_requires_judgment"
jq -e '.video_without_judgment_cases | index("dev-lanterns-motion") != null' \
  "${TMP_DIR}/plan-video-mechanical.json" >/dev/null

# trigger_event without a session_config baseline → trigger_case_without_baseline
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-pulse",
      "kind": "screenshot",
      "url_path": "/dev/lanterns",
      "must_show": "a single bright pulse against a quiet sky",
      "trigger_event": "release-pulse"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-trigger-no-baseline.json" "trigger_case_without_baseline"
jq -e '
  (.trigger_cases_without_baseline | index("dev-lanterns-pulse") != null)
  and (.summary | test("quiescent baseline"))
' "${TMP_DIR}/plan-trigger-no-baseline.json" >/dev/null

# no must_show and no mechanical expectation → empty_case
cat >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {
      "id": "dev-lanterns-empty",
      "kind": "screenshot",
      "url_path": "/dev/lanterns"
    }
  ]
}
JSON
expect_plan_reject "${TMP_DIR}/plan-empty-case.json" "empty_case"
jq -e '.empty_cases | index("dev-lanterns-empty") != null' \
  "${TMP_DIR}/plan-empty-case.json" >/dev/null

# --- retired surface: issue-contract stage must not return ---
# Migration policy: the pre-implementation contract stage is deleted end to
# end (public names settle by the implementation's declaration). This guard
# fails if the retired stage's files or wrapper reads are reintroduced.
if [ -e "${SCRIPT_DIR}/glimmung-native/issue-contract.sh" ] || [ -e "${SCRIPT_DIR}/../.github/agent/prompt-issue-contract.md" ]; then
  echo "retired issue-contract stage files have been reintroduced" >&2
  exit 1
fi
if grep -l "GLIMMUNG_INPUT_ISSUE_CONTRACT" "${SCRIPT_DIR}"/glimmung-native/*.sh >/dev/null 2>&1; then
  echo "retired issue-contract input reads have been reintroduced in native scripts" >&2
  exit 1
fi

# --- feature types: standing case source (verify-side, when-skipped plan leg) ---

REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# The REAL repo standing case must pass the exact same lint pipeline a
# generated plan does — this is the CI guard that keeps the standing case
# claim-vocabulary clean. The harness wraps the file in the same envelope
# verify.sh synthesizes at runtime and runs it through the plan gates.
jq -n --slurpfile standing "${REPO_ROOT}/.github/agent/standing-cases/effect.json" \
  '{schema_version: 1, status: "pass", case_source: "standing", feature_type: "effect", required_evidence: [$standing[0]]}' \
  >"${PLAN_EVIDENCE_DIR}/issue-agent-test-plan.json"
run_plan_validate "${TMP_DIR}/standing-plan.json"
if [ "$PLAN_VALIDATE_RC" -ne 0 ]; then
  echo "expected the repo standing effect case to pass the plan lint gates, got ${PLAN_VALIDATE_RC}" >&2
  cat "${TMP_DIR}/standing-plan.json" >&2
  exit 1
fi
jq -e '
  .status == "pass"
  and .case_source == "standing"
  and (.required_evidence | length == 1)
  and .required_evidence[0].source == "standing"
  and .required_evidence[0].id == "effect-acceptance"
  and .required_evidence[0].kind == "video"
  and ((.required_evidence[0].must_show // "") != "")
' "${TMP_DIR}/standing-plan.json" >/dev/null

# --- feature-type implementation contracts ---

CONTRACT_EVIDENCE_DIR="${TMP_DIR}/contract-evidence"
mkdir -p "$CONTRACT_EVIDENCE_DIR"
AMBIENCE_FEATURE_TYPE="effect" \
AMBIENCE_IMPLEMENTATION_CONTRACT="${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" \
bash "${SCRIPT_DIR}/agent/contracts/generate.sh" effect "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" >/dev/null
jq -e '
  .schema_version == 1
  and .kind == "implementation_contract"
  and .feature_type == "effect"
  and (.required_file_templates | index("sim/{effect_snake}.go") != null)
  and (.required_touchpoints | any(.path == "cmd/ambience-wasm/main.go"))
  and (.forbidden_paths | any(.pattern == "cmd/ambience/web/effects/*"))
' "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" >/dev/null

CONTRACT_REPO="${TMP_DIR}/contract-repo"
mkdir -p "$CONTRACT_REPO"
git -C "$CONTRACT_REPO" init -q
git -C "$CONTRACT_REPO" config user.name "contract-test"
git -C "$CONTRACT_REPO" config user.email "contract-test@example.invalid"
mkdir -p "${CONTRACT_REPO}/cmd/ambience-wasm"
printf 'package main\n' >"${CONTRACT_REPO}/cmd/ambience-wasm/main.go"
git -C "$CONTRACT_REPO" add .
git -C "$CONTRACT_REPO" commit -q -m "base"
git -C "$CONTRACT_REPO" update-ref refs/remotes/origin/main HEAD

write_effect_impl_json() {
  cat >"${CONTRACT_EVIDENCE_DIR}/issue-agent-implementation.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "summary": "added paper lanterns",
  "ui_hint": {"menu_label": "paper-lanterns", "route": "/dev/paper-lanterns"}
}
JSON
}

reset_contract_repo() {
  git -C "$CONTRACT_REPO" reset -q --hard origin/main
  git -C "$CONTRACT_REPO" clean -qfdx
}

reset_contract_repo
mkdir -p "${CONTRACT_REPO}/sim" "${CONTRACT_REPO}/cmd/ambience"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns.go"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns_test.go"
printf 'package main\n' >"${CONTRACT_REPO}/cmd/ambience/effect_paper_lanterns.go"
printf 'package main\n// paper-lanterns\n' >"${CONTRACT_REPO}/cmd/ambience-wasm/main.go"
git -C "$CONTRACT_REPO" add .
git -C "$CONTRACT_REPO" commit -q -m "valid effect"
write_effect_impl_json
bash "${SCRIPT_DIR}/agent/contracts/validate.sh" \
  "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" \
  "${CONTRACT_EVIDENCE_DIR}/issue-agent-implementation.json" \
  "$CONTRACT_REPO" \
  origin/main \
  HEAD >"${TMP_DIR}/contract-valid.json"
jq -e '
  .status == "pass"
  and .feature_type == "effect"
  and .effect_slug == "paper-lanterns"
  and (.changed_files | index("cmd/ambience-wasm/main.go") != null)
' "${TMP_DIR}/contract-valid.json" >/dev/null

reset_contract_repo
mkdir -p "${CONTRACT_REPO}/sim" "${CONTRACT_REPO}/cmd/ambience"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns.go"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns_test.go"
printf 'package main\n' >"${CONTRACT_REPO}/cmd/ambience/effect_paper_lanterns.go"
git -C "$CONTRACT_REPO" add .
git -C "$CONTRACT_REPO" commit -q -m "missing wasm touchpoint"
set +e
bash "${SCRIPT_DIR}/agent/contracts/validate.sh" \
  "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" \
  "${CONTRACT_EVIDENCE_DIR}/issue-agent-implementation.json" \
  "$CONTRACT_REPO" \
  origin/main \
  HEAD >"${TMP_DIR}/contract-missing.json" 2>"${TMP_DIR}/contract-missing.err"
CONTRACT_VALIDATE_RC=$?
set -e
[ "$CONTRACT_VALIDATE_RC" -eq 1 ]
jq -e '
  .status == "fail"
  and .abort_reason == "missing_required_touchpoints"
  and (.detail | fromjson | index("cmd/ambience-wasm/main.go") != null)
' "${TMP_DIR}/contract-missing.json" >/dev/null

reset_contract_repo
mkdir -p "${CONTRACT_REPO}/sim" "${CONTRACT_REPO}/cmd/ambience" "${CONTRACT_REPO}/cmd/ambience/web/effects"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns.go"
printf 'package sim\n' >"${CONTRACT_REPO}/sim/paper_lanterns_test.go"
printf 'package main\n' >"${CONTRACT_REPO}/cmd/ambience/effect_paper_lanterns.go"
printf 'package main\n// paper-lanterns\n' >"${CONTRACT_REPO}/cmd/ambience-wasm/main.go"
printf 'export class PaperLanterns {}\n' >"${CONTRACT_REPO}/cmd/ambience/web/effects/paper-lanterns.js"
git -C "$CONTRACT_REPO" add .
git -C "$CONTRACT_REPO" commit -q -m "legacy browser effect"
set +e
bash "${SCRIPT_DIR}/agent/contracts/validate.sh" \
  "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" \
  "${CONTRACT_EVIDENCE_DIR}/issue-agent-implementation.json" \
  "$CONTRACT_REPO" \
  origin/main \
  HEAD >"${TMP_DIR}/contract-forbidden.json" 2>"${TMP_DIR}/contract-forbidden.err"
CONTRACT_VALIDATE_RC=$?
set -e
[ "$CONTRACT_VALIDATE_RC" -eq 1 ]
jq -e '
  .status == "fail"
  and .abort_reason == "forbidden_touchpoint"
  and (.detail | fromjson | any(test("cmd/ambience/web/effects/paper-lanterns.js")))
' "${TMP_DIR}/contract-forbidden.json" >/dev/null

# --- verify.sh agentless mechanical cases ---
# Source verify.sh (AMBIENCE_VERIFY_SOURCE_ONLY) and drive the mechanical
# path through stubbed curl/node, in a subshell so the sourced globals and
# stub PATH do not leak back into this harness.
(
  VERIFY_STUB_DIR="${TMP_DIR}/verify-stubs"
  VERIFY_EVIDENCE_DIR="${TMP_DIR}/verify-evidence"
  mkdir -p "$VERIFY_STUB_DIR" "$VERIFY_EVIDENCE_DIR"

  cat >"${VERIFY_STUB_DIR}/curl" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

url=""
write_out=""
expect=""
for arg in "$@"; do
  if [ -n "$expect" ]; then
    if [ "$expect" = "w" ]; then
      write_out="$arg"
    fi
    expect=""
    continue
  fi
  case "$arg" in
    -w) expect="w" ;;
    -o|-d|-H|-X|--retry|--retry-delay|--max-time) expect="skip" ;;
    -*) ;;
    *) url="$arg" ;;
  esac
done

case "$url" in
  */dev/trigger/*)
    if [ -n "$write_out" ]; then
      printf '%s' "${NATIVE_CONTRACT_TRIGGER_HTTP:-204}"
    fi
    ;;
  */dev/snapshot*)
    snapshot_json="${NATIVE_CONTRACT_SNAPSHOT_JSON:-}"
    if [ -z "$snapshot_json" ]; then
      snapshot_json='{}'
    fi
    printf '%s' "$snapshot_json"
    ;;
  */dev/events*)
    ;;
  *)
    # Surface probes (-o /dev/null -w %{http_code}) get the configured
    # status; other calls just record the URL.
    if [ -n "$write_out" ]; then
      printf '%s' "${NATIVE_CONTRACT_SURFACE_HTTP:-200}"
    elif [ -n "${NATIVE_CONTRACT_CURL_CAPTURE:-}" ]; then
      printf '%s\n' "$url" >"${NATIVE_CONTRACT_CURL_CAPTURE}.url"
    fi
    ;;
esac
SH
  chmod +x "${VERIFY_STUB_DIR}/curl"

  cat >"${VERIFY_STUB_DIR}/node" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

script="${1:-}"
shift || true
out=""
shot=""
prev=""
for arg in "$@"; do
  case "$prev" in
    --output) out="$arg" ;;
    --screenshot) shot="$arg" ;;
  esac
  prev="$arg"
done
printf '%s %s\n' "$(basename "$script")" "$*" >>"${NATIVE_CONTRACT_VERIFY_NODE_LOG:-/dev/null}"

case "$(basename "$script")" in
  pin-session-config.mjs)
    if [ "${NATIVE_CONTRACT_PIN_FAIL:-}" = "1" ]; then
      printf '{"error":"schema fetch failed"}\n'
      exit 1
    fi
    printf '{"knob_count":7,"pinned_at_tick":3}\n'
    ;;
  capture-observation.mjs)
    if [ "${NATIVE_CONTRACT_OBSERVE_FAIL:-}" = "1" ]; then
      printf 'observe failed 408: observe timed out\n'
      exit 1
    fi
    if [ -n "$out" ]; then
      mkdir -p "$(dirname "$out")"
      printf '%s' "${NATIVE_CONTRACT_OBSERVATION_JSON:-}" >"$out"
    fi
    if [ -n "$shot" ]; then
      mkdir -p "$(dirname "$shot")"
      printf 'PNG-STUB' >"$shot"
    fi
    printf '{"kind":"observation","observed":true,"applied":true}\n'
    ;;
  *)
    printf 'unexpected node invocation: %s\n' "$script" >&2
    exit 1
    ;;
esac
SH
  chmod +x "${VERIFY_STUB_DIR}/node"

  export PATH="${VERIFY_STUB_DIR}:${PATH}"
  export NATIVE_CONTRACT_VERIFY_NODE_LOG="${TMP_DIR}/verify-node.calls"
  export AMBIENCE_VERIFY_SOURCE_ONLY=1
  export AMBIENCE_EVIDENCE_DIR="$VERIFY_EVIDENCE_DIR"
  export AMBIENCE_REPO_DIR="${TMP_DIR}/repo"
  export GLIMMUNG_DYNAMIC_CASE_INDEX=1
  export GLIMMUNG_INPUT_VALIDATION_URL="https://preview.test"
  export GLIMMUNG_INPUT_NAMESPACE="preview-ns"
  export GLIMMUNG_INPUT_BRANCH_NAME="glimmung/run-1"
  unset GLIMMUNG_STEP_SLUG

  # shellcheck source=glimmung-native/verify.sh
  source "${SCRIPT_DIR}/glimmung-native/verify.sh"

  # a judged case (non-empty must_show) must not route to the mechanical path
  write_verification_case_file "active" "" \
    '{"id":"judged-1","kind":"video","url_path":"/dev/lanterns?session=judged-1","must_show":"a warm glow"}'
  if verification_case_is_mechanical; then
    echo "judged case (non-empty must_show) must not route to the mechanical path" >&2
    exit 1
  fi

  # mechanical pass: pin + documented trigger flow + lifecycle observation +
  # frozen frame, synthesized verdict accepted by the existing enforcement
  write_verification_case_file "active" "" \
    '{"id":"mech-ending","kind":"screenshot","url_path":"/dev/lanterns?session=mech-ending","trigger_event":"ending","terminal_lifecycle":"ended","hold_ticks":12,"session_config":{"emit_every":9999}}'
  cat >"${VERIFY_EVIDENCE_DIR}/issue-agent-test-plan.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "required_evidence": [
    {"id": "mech-ending", "kind": "screenshot", "url_path": "/dev/lanterns?session=mech-ending", "trigger_event": "ending", "terminal_lifecycle": "ended", "hold_ticks": 12, "session_config": {"emit_every": 9999}}
  ]
}
JSON
  export NATIVE_CONTRACT_SNAPSHOT_JSON='{"lifecycle":"running","appliedEvents":[{"tick":4,"event":"ending"}]}'
  export NATIVE_CONTRACT_OBSERVATION_JSON='{"applied":true,"observed":true,"lifecycle":"ended","holdTicks":12,"observedTick":40,"heldUntilTick":52}'
  run_llm
  jq -e '
    .schema_version == 1
    and .status == "pass"
    and (.evidence_results | length == 1)
    and .evidence_results[0].id == "mech-ending"
    and .evidence_results[0].status == "pass"
    and (.evidence_results[0].observed_text
      | test("pin verified") and test("trigger ending accepted") and test("lifecycle ended held 12 ticks"))
    and .evidence_results[0].screenshot == "screenshots/mech-ending.png"
    and .evidence_results[0].observation == "observations/mech-ending.json"
  ' "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json" >/dev/null
  [ -s "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.md" ]
  [ -s "${VERIFY_EVIDENCE_DIR}/screenshots/mech-ending.png" ]
  if grep -q "apply-agent-job" "$NATIVE_CONTRACT_PYTHON_CAPTURE" 2>/dev/null; then
    echo "mechanical case must not launch the inner LLM job" >&2
    exit 1
  fi
  # the existing enforcement gates accept the synthesized verdict unchanged
  enforce_evidence_contract
  enforce_session_config_pinned
  enforce_terminal_observation_artifact

  # mechanical fail-closed: trigger accepted but never applied
  rm -f "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json"
  write_verification_case_file "active" "" \
    '{"id":"mech-pulse","kind":"screenshot","url_path":"/dev/lanterns?session=mech-pulse","trigger_event":"release-pulse","session_config":{"emit_every":9999}}'
  export NATIVE_CONTRACT_SNAPSHOT_JSON='{"lifecycle":"running","appliedEvents":[]}'
  run_mechanical_case
  jq -e '
    .status == "fail"
    and .abort_reason == "trigger_not_observed"
    and (.failure | type == "object")
    and .failure.where == "wrapper-mechanical"
    and .evidence_results[0].status == "fail"
  ' "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json" >/dev/null

  # mechanical fail-closed: session pin failure
  rm -f "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json"
  write_verification_case_file "active" "" \
    '{"id":"mech-pin","kind":"screenshot","url_path":"/dev/lanterns?session=mech-pin","session_config":{"emit_every":9999}}'
  export NATIVE_CONTRACT_PIN_FAIL=1
  run_mechanical_case
  unset NATIVE_CONTRACT_PIN_FAIL
  jq -e '.status == "fail" and .abort_reason == "session_pin_failed"' \
    "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json" >/dev/null

  # mechanical config-only screenshot case: frame frozen via the session
  # lifecycle echoed back as a self-satisfying /dev/observe predicate
  rm -f "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json"
  write_verification_case_file "active" "" \
    '{"id":"mech-quiet","kind":"screenshot","url_path":"/dev/lanterns?session=mech-quiet","session_config":{"emit_every":9999}}'
  export NATIVE_CONTRACT_SNAPSHOT_JSON='{"lifecycle":"running","appliedEvents":[]}'
  run_mechanical_case
  jq -e '
    .status == "pass"
    and .evidence_results[0].screenshot == "screenshots/mech-quiet.png"
    and .evidence_results[0].observation == "observations/mech-quiet-frame.json"
    and (.evidence_results[0].observed_text | test("frame frozen at lifecycle running"))
  ' "${VERIFY_EVIDENCE_DIR}/issue-agent-verification.json" >/dev/null
  grep -q -- "--lifecycle running" "$NATIVE_CONTRACT_VERIFY_NODE_LOG"

  # --- standing case binding (resolve_standing_case) ---

  # a standing case binds url_path + session from the implementation ui_hint
  write_verification_case_file "active" "" \
    '{"id":"effect-acceptance","kind":"video","source":"standing","duration_seconds":10,"must_show":"the effect reads as the issue describes"}'
  export GLIMMUNG_INPUT_UI_HINT='{"menu_label":"paper-lanterns","route":"/dev/paper-lanterns"}'
  resolve_standing_case
  jq -e '
    .status == "active"
    and .required_evidence.url_path == "/dev/paper-lanterns?session=effect-acceptance"
    and .required_evidence.ui_hint.menu_label == "paper-lanterns"
    and .required_evidence.ui_hint.route == "/dev/paper-lanterns"
    and .required_evidence.must_show == "the effect reads as the issue describes"
  ' "$VERIFICATION_CASE_FILE" >/dev/null

  # a missing/invalid ui_hint fails the standing case closed, with the
  # detail riding the durable case file (finalize folds it into reasons)
  write_verification_case_file "active" "" \
    '{"id":"effect-acceptance","kind":"video","source":"standing","must_show":"the effect reads as the issue describes"}'
  unset GLIMMUNG_INPUT_UI_HINT
  resolve_standing_case
  jq -e '
    .status == "plan_error"
    and (.reason | test("ui_hint is missing or invalid"))
  ' "$VERIFICATION_CASE_FILE" >/dev/null

  # a route outside /dev/<effect> is rejected the same way
  write_verification_case_file "active" "" \
    '{"id":"effect-acceptance","kind":"video","source":"standing","must_show":"the effect reads as the issue describes"}'
  export GLIMMUNG_INPUT_UI_HINT='{"menu_label":"monitor","route":"https://evil.example/page"}'
  resolve_standing_case
  jq -e '.status == "plan_error"' "$VERIFICATION_CASE_FILE" >/dev/null
  unset GLIMMUNG_INPUT_UI_HINT

  # a generated (non-standing) case passes through resolution untouched
  write_verification_case_file "active" "" \
    '{"id":"gen-1","kind":"video","url_path":"/dev/lanterns?session=gen-1","must_show":"a warm glow"}'
  resolve_standing_case
  jq -e '
    .status == "active"
    and .required_evidence.url_path == "/dev/lanterns?session=gen-1"
    and (.required_evidence | has("ui_hint") | not)
  ' "$VERIFICATION_CASE_FILE" >/dev/null

  # --- verify-side standing sourcing (when-skipped plan leg) ---

  # empty test_plan input + feature type: the standing envelope is staged
  # from the repo checkout
  rm -f "$TEST_PLAN_FILE"
  export AMBIENCE_FEATURE_TYPE="effect"
  export AMBIENCE_STANDING_CASE_FILE="${REPO_ROOT}/.github/agent/standing-cases/effect.json"
  stage_standing_test_plan
  jq -e '
    .case_source == "standing"
    and .required_evidence[0].id == "effect-acceptance"
    and .required_evidence[0].source == "standing"
  ' "$TEST_PLAN_FILE" >/dev/null

  # no feature type: an empty plan input is a hard error, not a silent
  # zero-case run
  unset AMBIENCE_FEATURE_TYPE
  if stage_standing_test_plan 2>/dev/null; then
    echo "empty test_plan without a feature type must fail" >&2
    exit 1
  fi

  # declared type with a missing standing file fails loudly
  export AMBIENCE_FEATURE_TYPE="no-such-type"
  export AMBIENCE_STANDING_CASE_FILE="${TMP_DIR}/does-not-exist.json"
  if stage_standing_test_plan 2>/dev/null; then
    echo "missing standing case file must fail" >&2
    exit 1
  fi
  unset AMBIENCE_FEATURE_TYPE AMBIENCE_STANDING_CASE_FILE

  # --- declared-surface enforcement (contract-less standing flow) ---

  # the bound ui_hint route + derived schema route must serve
  write_verification_case_file "active" "" \
    '{"id":"effect-acceptance","kind":"video","source":"standing","url_path":"/dev/paper-lanterns?session=effect-acceptance","ui_hint":{"menu_label":"paper-lanterns","route":"/dev/paper-lanterns"},"must_show":"the effect reads as the issue describes"}'
  : >"$VERIFICATION_REASONS"
  export NATIVE_CONTRACT_SURFACE_HTTP=200
  enforce_declared_surface
  : >"$VERIFICATION_REASONS"
  export NATIVE_CONTRACT_SURFACE_HTTP=404
  if enforce_declared_surface; then
    echo "a dead declared route must fail enforcement" >&2
    exit 1
  fi
  grep -q "declared surface" "$VERIFICATION_REASONS"
  unset NATIVE_CONTRACT_SURFACE_HTTP

  # a standing case that lost its hint binding cannot pass surface checks
  write_verification_case_file "active" "" \
    '{"id":"effect-acceptance","kind":"video","source":"standing","must_show":"the effect reads as the issue describes"}'
  : >"$VERIFICATION_REASONS"
  if enforce_declared_surface; then
    echo "a standing case without a bound ui_hint must fail surface enforcement" >&2
    exit 1
  fi
)

# --- implement.sh ui_hint contract (feature types with a standing case) ---
(
  IMPL_EVIDENCE_DIR="${TMP_DIR}/impl-evidence"
  mkdir -p "$IMPL_EVIDENCE_DIR"
  export AMBIENCE_IMPLEMENT_SOURCE_ONLY=1
  export AMBIENCE_EVIDENCE_DIR="$IMPL_EVIDENCE_DIR"
  export GLIMMUNG_INPUT_VALIDATION_URL="https://preview.test"
  export GLIMMUNG_INPUT_NAMESPACE="preview-ns"
  export GLIMMUNG_INPUT_IMAGE_TAG="git-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  unset GLIMMUNG_STEP_SLUG

  # shellcheck source=glimmung-native/implement.sh
  source "${SCRIPT_DIR}/glimmung-native/implement.sh"

  # passing implementation with a valid ui_hint: finalize succeeds and emit
  # publishes the hint as its own phase output
  cat >"${IMPL_EVIDENCE_DIR}/issue-agent-implementation.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "summary": "added paper lanterns",
  "ui_hint": {"menu_label": "paper-lanterns", "route": "/dev/paper-lanterns"}
}
JSON
  export AMBIENCE_FEATURE_TYPE="effect"
  finalize
  jq -e '.status == "pass"' "$IMPLEMENTATION_JSON" >/dev/null
  rm -f "$GLIMMUNG_OUTPUT_FILE" "$GLIMMUNG_COMPLETION_FILE"
  emit
  jq -e '
    (.ui_hint | fromjson | .menu_label == "paper-lanterns")
    and .branch_name == "glimmung/run-1"
  ' "$GLIMMUNG_OUTPUT_FILE" >/dev/null

  # passing implementation WITHOUT a ui_hint under a standing feature type:
  # finalize fails the step, named, before any verify spend
  cat >"${IMPL_EVIDENCE_DIR}/issue-agent-implementation.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "summary": "added paper lanterns"
}
JSON
  set +e
  finalize
  FINALIZE_RC=$?
  set -e
  [ "$FINALIZE_RC" -ne 0 ]
  jq -e '.status == "fail" and .abort_reason == "missing_ui_hint"' "$IMPLEMENTATION_JSON" >/dev/null

  # a non-/dev route is rejected the same way
  cat >"${IMPL_EVIDENCE_DIR}/issue-agent-implementation.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "summary": "added paper lanterns",
  "ui_hint": {"menu_label": "paper-lanterns", "route": "https://evil.example/dev/paper-lanterns"}
}
JSON
  set +e
  finalize
  FINALIZE_RC=$?
  set -e
  [ "$FINALIZE_RC" -ne 0 ]
  jq -e '.abort_reason == "missing_ui_hint"' "$IMPLEMENTATION_JSON" >/dev/null

  # without a feature type, a missing ui_hint is not an error (generated-path
  # workflows have no standing case to bind)
  unset AMBIENCE_FEATURE_TYPE
  cat >"${IMPL_EVIDENCE_DIR}/issue-agent-implementation.json" <<'JSON'
{
  "schema_version": 1,
  "status": "pass",
  "summary": "non-effect change"
}
JSON
  finalize
  jq -e '.status == "pass"' "$IMPLEMENTATION_JSON" >/dev/null
)
