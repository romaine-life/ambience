#!/usr/bin/env bash
#
# Glimmung phase: test-plan
#
# Spawns a read-only LLM pod that reads the issue and produces an evidence
# specification (issue-agent-test-plan.json). Runs in parallel with the
# implement phase — does NOT receive or produce code changes.
#
# Outputs emitted to glimmung:
#   test_plan  — issue-agent-test-plan.json serialised as a JSON string.
#                Consumed by the verify phase to know what evidence to capture.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_INPUT_VALIDATION_URL \
  GLIMMUNG_INPUT_NAMESPACE

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-latest}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
JOB_NAME="agent-${RUN_SLUG}-tp-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-test-plan"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

PLAN_EXIT_CODE=0
TEST_PLAN_JSON="/tmp/test-plan.json"
TEST_PLAN_MD="/tmp/test-plan.md"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
PLAN_EXIT_CODE_FILE="/tmp/test-plan-exit-code"
PROXY_IP_FILE="/tmp/test-plan-proxy-ip"
ISSUE_CONTRACT_FILE="${EVIDENCE_DIR}/issue-agent-contract.json"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
mkdir -p "$EVIDENCE_DIR/screenshots" "$EVIDENCE_DIR/videos"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main
}

copy_claude_ca() {
  kubectl -n "$CLAUDE_CA_NAMESPACE" get configmap claude-oauth-ca -o json \
    | NAMESPACE="$NAMESPACE" jq '
        del(
          .metadata.annotations,
          .metadata.uid,
          .metadata.resourceVersion,
          .metadata.generation,
          .metadata.creationTimestamp,
          .metadata.managedFields
        )
        | .metadata.namespace = env.NAMESPACE
      ' \
    | kubectl apply -f -
}

prepare_context() {
  native_azure_login
  native_install_preview_package "${REPO_DIR}/mcp"
  copy_claude_ca
  native_prepare_codex_credentials_secret "$NAMESPACE"

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP
  printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"

  write_prompt_context() {
    local dest="/tmp/agent-prompt-context.md"
    : >"$dest"
    {
      echo "# Glimmung issue ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
      echo "URL: ${ISSUE_URL}"
      echo "Validation env: ${VALIDATION_URL}"
      echo "Glimmung run: ${GLIMMUNG_RUN_ID}"
      echo "Glimmung attempt index: ${ATTEMPT_INDEX:-unknown}"
      if [ -n "${GLIMMUNG_ISSUE_BODY:-}" ]; then
        echo ""
        echo "## Issue body"
        echo ""
        printf '%s\n' "$GLIMMUNG_ISSUE_BODY"
      fi
    } >>"$dest"
  }
  write_prompt_context

  prepare_agent_github_token() {
    local token
    token="$(native_github_token)"
    kubectl -n "$NAMESPACE" create secret generic agent-github-token \
      --from-literal=token="$token" \
      --dry-run=client -o yaml | kubectl apply -f -
  }
  prepare_agent_github_token

  if [ -n "${GLIMMUNG_INPUT_ISSUE_CONTRACT:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" | jq -r . >"$ISSUE_CONTRACT_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" >"$ISSUE_CONTRACT_FILE"
    echo "staged issue-contract JSON ($(wc -c <"$ISSUE_CONTRACT_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_ISSUE_CONTRACT not set; test-plan will proceed from issue context only"
  fi

  local args=(
    --from-file=prompt-test-plan.md="${REPO_DIR}/.github/agent/prompt-test-plan.md"
  )
  [ -s "$ISSUE_CONTRACT_FILE" ] && args+=(--from-file="issue-agent-contract.json=${ISSUE_CONTRACT_FILE}")
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

ensure_proxy_ip() {
  if [ -z "${PROXY_IP:-}" ] && [ -s "$PROXY_IP_FILE" ]; then
    PROXY_IP="$(cat "$PROXY_IP_FILE")"
    export PROXY_IP
  fi
  if [ -z "${PROXY_IP:-}" ]; then
    PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
    if [ -z "$PROXY_IP" ]; then
      echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
      return 1
    fi
    export PROXY_IP
    printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"
  fi
}

run_llm() {
  ensure_proxy_ip
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli apply-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --issue-number "${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}" \
      --issue-title "$ISSUE_TITLE" \
      --issue-url "$ISSUE_URL" \
      --issue-reference "$ISSUE_REFERENCE" \
      --validation-url "$VALIDATION_URL" \
      --branch-name "glimmung/${GLIMMUNG_RUN_ID}" \
      --proxy-ip "$PROXY_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --repo-slug "$REPO_SLUG" \
      --stage "test-plan" \
      --config-map-name "$CONFIG_MAP_NAME" \
      --agent-runtime-json "${GLIMMUNG_AGENT_RUNTIME_JSON:-}"
    native_emit_inner_job_marker "$NAMESPACE" "$JOB_NAME" helper test-plan-agent
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_TEST_PLAN_TIMEOUT_SECONDS:-900}"
  )
}

run_llm_record() {
  native_record_exit_code "$PLAN_EXIT_CODE_FILE" run_llm
  PLAN_EXIT_CODE="$(native_read_exit_code "$PLAN_EXIT_CODE_FILE")"
}

plan_exit_code() {
  native_read_exit_code "$PLAN_EXIT_CODE_FILE"
}

collect_evidence() {
  local pod=""
  pod="$(kubectl -n "$NAMESPACE" get pods -l "job-name=${JOB_NAME}" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "$pod" ]; then
    echo "no pod found for job ${JOB_NAME}; skipping evidence capture"
    : >"$POD_LOG"
    return 0
  fi
  kubectl -n "$NAMESPACE" logs "$pod" >"$POD_LOG" || true
  echo "captured $(wc -l <"$POD_LOG") log lines from ${pod}"

  if grep -q '===EVIDENCE-TAR-START===' "$POD_LOG"; then
    if ! sed -n '/===EVIDENCE-TAR-START===/,/===EVIDENCE-TAR-END===/{//!p;}' \
        "$POD_LOG" \
        | base64 -d 2>/tmp/extract.err \
        | tar -xzf - -C "$EVIDENCE_DIR" 2>>/tmp/extract.err; then
      echo "evidence tarball extraction failed; continuing" >&2
      cat /tmp/extract.err >&2 || true
    fi
  else
    echo "no evidence tar markers found for ${JOB_NAME}"
  fi

  grep -E '^\{' "$POD_LOG" >>"$EVENTS_LOG" || true
}

finalize() {
  if [ -f "${EVIDENCE_DIR}/issue-agent-test-plan.json" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.json" "$TEST_PLAN_JSON"
  fi
  if [ -f "${EVIDENCE_DIR}/issue-agent-test-plan.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.md" "$TEST_PLAN_MD"
    cp "$TEST_PLAN_MD" "$SUMMARY_MD"
  fi
  if [ ! -s "$TEST_PLAN_JSON" ]; then
    local exit_code
    exit_code="$(plan_exit_code)"
    if [ "$exit_code" -ne 0 ]; then
      jq -n --arg reason "test-plan pod exited with ${exit_code}" \
        '{schema_version:1,status:"fail",abort_reason:$reason}' >"$TEST_PLAN_JSON"
    else
      jq -n '{schema_version:1,status:"fail",abort_reason:"test-plan pod produced no output"}' \
        >"$TEST_PLAN_JSON"
    fi
  fi
}

emit() {
  local test_plan_str
  test_plan_str="$(cat "$TEST_PLAN_JSON")"
  local outputs
  outputs="$(jq -nc --argjson v "$test_plan_str" '{test_plan: ($v | tostring)}')"
  local summary
  summary="$(cat "$SUMMARY_MD" 2>/dev/null || true)"
  native_completed "$outputs" "null" "" "$summary"
  local exit_code
  exit_code="$(plan_exit_code)"
  if native_selected_step && [ "$exit_code" -ne 0 ]; then
    native_failed "test-plan pod exited with ${exit_code}"
    return "$exit_code"
  fi
}

if native_selected_step; then
  native_run_selected_step \
    "clone" clone_repo \
    "prepare" prepare_context \
    "run-test-plan" run_llm_record \
    "collect" collect_evidence \
    "finalize" finalize \
    "emit" emit
  exit $?
fi

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step "run-test-plan" run_llm_record
native_step "collect" collect_evidence
native_step "finalize" finalize
PLAN_EXIT_CODE="$(plan_exit_code)"
if [ "$PLAN_EXIT_CODE" -ne 0 ]; then
  native_failed "test-plan pod exited with ${PLAN_EXIT_CODE}"
  exit "$PLAN_EXIT_CODE"
fi
native_step "emit" emit
native_assert_resume_satisfied
