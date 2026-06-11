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

ARTIFACTS_JSON="${TMP_DIR}/artifacts.json"
VERIFICATION_JSON="${TMP_DIR}/verification.json"
REFS_TXT="${TMP_DIR}/refs.txt"
cat >"$ARTIFACTS_JSON" <<'JSON'
[
  {"kind":"screenshot","ref":"screenshots/frame.png","content_type":"image/png"},
  {"kind":"video","ref":"/workspace/evidence/videos/demo.webm","content_type":"video/webm"},
  {"kind":"artifact","ref":"observations/state.json","content_type":"application/json"},
  {"kind":"video","ref":"/v1/artifacts/runs/ambience/run-1/videos/already.webm","content_type":"video/webm"}
]
JSON
cat >"$VERIFICATION_JSON" <<'JSON'
{"status":"fail","reasons":["example"],"evidence_refs":[],"evidence":[]}
JSON
native_rewrite_uploaded_evidence_refs \
  "https://glimmung.romaine.life/v1/artifacts/" \
  "runs/ambience/run-1/screenshots" \
  "runs/ambience/run-1/videos" \
  "runs/ambience/run-1/observations" \
  "$ARTIFACTS_JSON" \
  "$VERIFICATION_JSON" \
  "$REFS_TXT"
jq -e '
  .[0].ref == "https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/screenshots/frame.png"
  and .[1].ref == "https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/videos/demo.webm"
  and .[2].ref == "https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/observations/state.json"
  and .[3].ref == "https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/videos/already.webm"
' "$ARTIFACTS_JSON" >/dev/null
jq -e '
  (.evidence | length) == 4
  and (.evidence_refs | length) == 4
  and (.evidence_refs[] | startswith("https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/"))
' "$VERIFICATION_JSON" >/dev/null
grep -Fx "https://glimmung.romaine.life/v1/artifacts/runs/ambience/run-1/videos/demo.webm" "$REFS_TXT" >/dev/null

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
