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

(
  FIRST_EDIT_REPO="${TMP_DIR}/first-edit-repo"
  git init -q "$FIRST_EDIT_REPO"
  git -C "$FIRST_EDIT_REPO" config user.name "contract-test"
  git -C "$FIRST_EDIT_REPO" config user.email "contract-test@example.invalid"
  printf 'base\n' >"${FIRST_EDIT_REPO}/base.txt"
  git -C "$FIRST_EDIT_REPO" add base.txt
  git -C "$FIRST_EDIT_REPO" commit -q -m "base"

  unset GLIMMUNG_MANAGED_RUNNER GLIMMUNG_STEP_SLUG
  export GLIMMUNG_JOB_ID="llm-work"
  export GLIMMUNG_RUN_ID="run-1"
  export GLIMMUNG_INPUT_VALIDATION_URL="https://preview.example"
  export GLIMMUNG_INPUT_NAMESPACE="preview-ns"
  export GLIMMUNG_INPUT_IMAGE_TAG="app-test"
  export GLIMMUNG_EVENTS_URL="http://glimmung.test/v1/run-callbacks/cb/native/events"
  export GLIMMUNG_COMPLETED_URL="http://glimmung.test/v1/run-callbacks/cb/native/completed"
  export GLIMMUNG_GITHUB_TOKEN_URL="http://glimmung.test/v1/run-callbacks/cb/native/github-token"
  export AMBIENCE_REPO_DIR="$FIRST_EDIT_REPO"
  export AMBIENCE_IMPLEMENT_SOURCE_ONLY=1
  export AMBIENCE_FIRST_EDIT_POLL_SECONDS=1
  export NATIVE_CONTRACT_CURL_CAPTURE="${TMP_DIR}/first-edit-event"
  rm -f "${NATIVE_CONTRACT_CURL_CAPTURE}.url" "${NATIVE_CONTRACT_CURL_CAPTURE}.body"
  source "${SCRIPT_DIR}/glimmung-native/implement.sh"

  monitor_first_edit_latency >"${TMP_DIR}/first-edit-monitor.out" &
  FIRST_EDIT_MONITOR_PID=$!
  sleep 2
  printf 'changed\n' >"${FIRST_EDIT_REPO}/simulated-effect.go"
  wait "$FIRST_EDIT_MONITOR_PID"

  grep -q 'first_edit_latency_seconds=' "${TMP_DIR}/first-edit-monitor.out"
  [ "$(cat "${NATIVE_CONTRACT_CURL_CAPTURE}.url")" = "$GLIMMUNG_EVENTS_URL" ]
  jq -e '
    .event == "metric"
    and .job_id == "llm-work"
    and .step_slug == "run-implementation"
    and .metadata.metric == "first_edit_latency_seconds"
    and (.metadata.value >= 1)
    and (.metadata.first_changed_files | index("simulated-effect.go") != null)
  ' "${NATIVE_CONTRACT_CURL_CAPTURE}.body" >/dev/null
)

(
  AGENT_CI_ORIGIN="${TMP_DIR}/agent-ci-origin.git"
  AGENT_CI_SEED="${TMP_DIR}/agent-ci-seed"
  AGENT_CI_BOOTSTRAP="${TMP_DIR}/agent-ci-bootstrap"
  AGENT_CI_WORK="${TMP_DIR}/agent-ci-work"
  AGENT_CI_BRANCH="glimmung/issue-168/run-1"

  git init -q "$AGENT_CI_SEED"
  git -C "$AGENT_CI_SEED" config user.name "contract-test"
  git -C "$AGENT_CI_SEED" config user.email "contract-test@example.invalid"
  printf 'base\n' >"${AGENT_CI_SEED}/effect.txt"
  git -C "$AGENT_CI_SEED" add effect.txt
  git -C "$AGENT_CI_SEED" commit -q -m "base"
  git -C "$AGENT_CI_SEED" branch -M main
  git clone -q --bare "$AGENT_CI_SEED" "$AGENT_CI_ORIGIN"

  git clone -q "$AGENT_CI_ORIGIN" "$AGENT_CI_BOOTSTRAP"
  git -C "$AGENT_CI_BOOTSTRAP" checkout -q -b "$AGENT_CI_BRANCH" origin/main
  git -C "$AGENT_CI_BOOTSTRAP" config user.name "contract-test"
  git -C "$AGENT_CI_BOOTSTRAP" config user.email "contract-test@example.invalid"
  git -C "$AGENT_CI_BOOTSTRAP" commit -q --allow-empty \
    -m "agent: start ambience#168"
  git -C "$AGENT_CI_BOOTSTRAP" push -q origin "HEAD:${AGENT_CI_BRANCH}"
  AGENT_CI_BOOTSTRAP_SHA="$(git -C "$AGENT_CI_BOOTSTRAP" rev-parse HEAD)"

  git clone -q "$AGENT_CI_ORIGIN" "$AGENT_CI_WORK"
  git -C "$AGENT_CI_WORK" checkout -q -b "$AGENT_CI_BRANCH" origin/main
  git -C "$AGENT_CI_WORK" config user.name "contract-test"
  git -C "$AGENT_CI_WORK" config user.email "contract-test@example.invalid"
  printf 'agent work\n' >"${AGENT_CI_WORK}/effect.txt"
  git -C "$AGENT_CI_WORK" add effect.txt
  git -C "$AGENT_CI_WORK" commit -q -m "agent: address 168"
  AGENT_CI_WORK_SHA="$(git -C "$AGENT_CI_WORK" rev-parse HEAD)"

  export AMBIENCE_AGENT_CI_FEEDBACK_SOURCE_ONLY=1
  export AMBIENCE_GIT_REMOTE_URL="$AGENT_CI_ORIGIN"
  export AMBIENCE_REPO_DIR="$AGENT_CI_WORK"
  export AMBIENCE_REPO_SLUG="romaine-life/ambience"
  export BRANCH_NAME="$AGENT_CI_BRANCH"
  source "${SCRIPT_DIR}/glimmung-native/agent-ci-feedback.sh"
  push_branch

  AGENT_CI_REMOTE_SHA="$(
    git --git-dir="$AGENT_CI_ORIGIN" rev-parse "refs/heads/${AGENT_CI_BRANCH}"
  )"
  [ "$AGENT_CI_REMOTE_SHA" = "$AGENT_CI_WORK_SHA" ]
  [ "$AGENT_CI_REMOTE_SHA" != "$AGENT_CI_BOOTSTRAP_SHA" ]
)

(
  SCAFFOLD_REPO="${TMP_DIR}/scaffold-worktree"
  SCAFFOLD_LOG="${TMP_DIR}/scaffold-effect.log"
  git -C "${SCRIPT_DIR}/.." worktree add -q --detach "$SCAFFOLD_REPO" HEAD

  set +e
  (
    set -Eeuo pipefail
    AMBIENCE_REPO_DIR="$SCAFFOLD_REPO" \
      bash "${SCRIPT_DIR}/agent/scaffold-effect.sh" test-orbit \
      >"${TMP_DIR}/scaffold-effect.out"

    grep -F 'sim/test_orbit.go' "${TMP_DIR}/scaffold-effect.out" >/dev/null
    grep -F 'cmd/ambience/effect_test_orbit.go' "${TMP_DIR}/scaffold-effect.out" >/dev/null
    grep -F 'sim.NewTestOrbit' "${TMP_DIR}/scaffold-effect.out" >/dev/null
    grep -F 'func NewTestOrbit' "${SCAFFOLD_REPO}/sim/test_orbit.go" >/dev/null
    grep -F 'func TestTestOrbitEndingHoldsTerminalState' \
      "${SCAFFOLD_REPO}/sim/test_orbit_test.go" >/dev/null
    grep -F 'newTestOrbitRuntime' \
      "${SCAFFOLD_REPO}/cmd/ambience/effect_test_orbit.go" >/dev/null

    git -C "$SCAFFOLD_REPO" diff --check
    go -C "$SCAFFOLD_REPO" test ./sim -run TestTestOrbit
    go -C "$SCAFFOLD_REPO" test ./cmd/ambience -run '^$'

    set +e
    AMBIENCE_REPO_DIR="$SCAFFOLD_REPO" \
      bash "${SCRIPT_DIR}/agent/scaffold-effect.sh" test-orbit \
      >"${TMP_DIR}/scaffold-effect-repeat.out" 2>&1
    REPEAT_RC=$?
    set -e
    [ "$REPEAT_RC" -ne 0 ]
    grep -F 'refusing to overwrite existing file' \
      "${TMP_DIR}/scaffold-effect-repeat.out" >/dev/null

    set +e
    AMBIENCE_REPO_DIR="$SCAFFOLD_REPO" \
      bash "${SCRIPT_DIR}/agent/scaffold-effect.sh" BadSlug \
      >"${TMP_DIR}/scaffold-effect-bad-slug.out" 2>&1
    BAD_SLUG_RC=$?
    set -e
    [ "$BAD_SLUG_RC" -eq 2 ]
    grep -F 'lowercase kebab-case' \
      "${TMP_DIR}/scaffold-effect-bad-slug.out" >/dev/null
  ) >"$SCAFFOLD_LOG" 2>&1
  SCAFFOLD_RC=$?
  set -e

  git -C "${SCRIPT_DIR}/.." worktree remove -f "$SCAFFOLD_REPO" >/dev/null 2>&1 || true
  if [ "$SCAFFOLD_RC" -ne 0 ]; then
    cat "$SCAFFOLD_LOG" >&2
    exit "$SCAFFOLD_RC"
  fi
)

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
# claim-vocabulary clean. The harness wraps the file in the same standing
# envelope used at runtime and runs it through the plan gates.
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
printf 'package main\n// paper-lanterns\n' >"${CONTRACT_REPO}/cmd/ambience-wasm/main.go"
write_effect_impl_json
bash "${SCRIPT_DIR}/agent/contracts/validate.sh" \
  "${CONTRACT_EVIDENCE_DIR}/implementation-contract.json" \
  "${CONTRACT_EVIDENCE_DIR}/issue-agent-implementation.json" \
  "$CONTRACT_REPO" \
  origin/main \
  HEAD >"${TMP_DIR}/contract-valid-worktree.json"
jq -e '
  .status == "pass"
  and .feature_type == "effect"
  and .effect_slug == "paper-lanterns"
  and (.changed_files | index("cmd/ambience-wasm/main.go") != null)
  and (.changed_files | index("sim/paper_lanterns.go") != null)
' "${TMP_DIR}/contract-valid-worktree.json" >/dev/null

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
