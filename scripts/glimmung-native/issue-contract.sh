#!/usr/bin/env bash
#
# Glimmung phase: prepare / issue-contract job
#
# Spawns a read-only LLM pod that canonicalizes the issue target before the
# parallel test-plan and implement jobs run.
#
# Outputs emitted to glimmung:
#   issue_contract — issue-agent-contract.json serialised as a JSON string.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_VALIDATION_NAMESPACE

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-${CLAUDE_NAMESPACE:-tank-operator}}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

NAMESPACE="${GLIMMUNG_VALIDATION_NAMESPACE}"
VALIDATION_SLOT_INDEX="${GLIMMUNG_NATIVE_SLOT_INDEX:-}"
if [ -n "$VALIDATION_SLOT_INDEX" ]; then
  VALIDATION_HOST="${AMBIENCE_STANDBY_HOST_PREFIX:-ambience-slot-}${VALIDATION_SLOT_INDEX}.ambience.dev.romaine.life"
else
  VALIDATION_HOST="${NAMESPACE}.ambience.dev.romaine.life"
fi
VALIDATION_URL="https://${VALIDATION_HOST}"

RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
JOB_NAME="agent-${RUN_SLUG}-ic-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-issue-contract"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

CONTRACT_EXIT_CODE=0
CONTRACT_JSON="/tmp/issue-contract.json"
CONTRACT_MD="/tmp/issue-contract.md"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
CONTRACT_EXIT_CODE_FILE="/tmp/issue-contract-exit-code"
PROXY_IP_FILE="/tmp/issue-contract-proxy-ip"
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

  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    --from-file=prompt-issue-contract.md="${REPO_DIR}/.github/agent/prompt-issue-contract.md" \
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
      --agent-container-image "$(native_agent_container_image)" \
      --repo-slug "$REPO_SLUG" \
      --stage "issue-contract" \
      --config-map-name "$CONFIG_MAP_NAME" \
      --agent-runtime-json "${GLIMMUNG_AGENT_RUNTIME_JSON:-}"
    native_emit_inner_job_marker "$NAMESPACE" "$JOB_NAME" helper issue-contract-agent
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_ISSUE_CONTRACT_TIMEOUT_SECONDS:-900}"
  )
}

run_llm_record() {
  native_record_exit_code "$CONTRACT_EXIT_CODE_FILE" run_llm
  CONTRACT_EXIT_CODE="$(native_read_exit_code "$CONTRACT_EXIT_CODE_FILE")"
}

contract_exit_code() {
  native_read_exit_code "$CONTRACT_EXIT_CODE_FILE"
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
  if [ -f "${EVIDENCE_DIR}/issue-agent-contract.json" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-contract.json" "$CONTRACT_JSON"
  fi
  if [ -f "${EVIDENCE_DIR}/issue-agent-contract.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-contract.md" "$CONTRACT_MD"
    cp "$CONTRACT_MD" "$SUMMARY_MD"
  fi
  if [ ! -s "$CONTRACT_JSON" ]; then
    local exit_code
    exit_code="$(contract_exit_code)"
    if [ "$exit_code" -ne 0 ]; then
      jq -n --arg reason "issue-contract pod exited with ${exit_code}" \
        '{schema_version:1,status:"fail",abort_reason:$reason}' >"$CONTRACT_JSON"
    else
      jq -n '{schema_version:1,status:"fail",abort_reason:"issue-contract pod produced no output"}' \
        >"$CONTRACT_JSON"
    fi
  fi
}

emit() {
  local contract_str
  contract_str="$(cat "$CONTRACT_JSON")"
  local outputs
  outputs="$(jq -nc --argjson v "$contract_str" '{issue_contract: ($v | tostring)}')"
  local summary
  summary="$(cat "$SUMMARY_MD" 2>/dev/null || true)"
  native_completed "$outputs" "null" "" "$summary"
  local exit_code
  exit_code="$(contract_exit_code)"
  if native_selected_step && [ "$exit_code" -ne 0 ]; then
    native_failed "issue-contract pod exited with ${exit_code}"
    return "$exit_code"
  fi
}

if native_selected_step; then
  native_run_selected_step \
    "clone" clone_repo \
    "prepare" prepare_context \
    "run-issue-contract" run_llm_record \
    "collect" collect_evidence \
    "finalize" finalize \
    "emit" emit
  exit $?
fi

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step "run-issue-contract" run_llm_record
native_step "collect" collect_evidence
native_step "finalize" finalize
CONTRACT_EXIT_CODE="$(contract_exit_code)"
if [ "$CONTRACT_EXIT_CODE" -ne 0 ]; then
  native_failed "issue-contract pod exited with ${CONTRACT_EXIT_CODE}"
  exit "$CONTRACT_EXIT_CODE"
fi
native_step "emit" emit
