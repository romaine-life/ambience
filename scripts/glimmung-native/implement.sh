#!/usr/bin/env bash
# Glimmung phase: implement.
#
# Code-editing LLM phase. Works directly from the issue spec, parallel to
# test-plan. Commits + pushes the implementation branch, then rebuilds the
# validation env so verify can run against it.
#
# Outputs emitted to glimmung:
#   implementation — issue-agent-implementation.json as a JSON string.
#   branch_name    — the pushed branch name (for verify to clone).

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_INPUT_VALIDATION_URL \
  GLIMMUNG_INPUT_NAMESPACE \
  GLIMMUNG_INPUT_IMAGE_TAG

REPO_SLUG="${AMBIENCE_REPO_SLUG:-nelsong6/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-latest}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
IMAGE_TAG="${GLIMMUNG_INPUT_IMAGE_TAG}"
BRANCH_NAME="${GLIMMUNG_WORK_CONTEXT_BRANCH:-glimmung/${GLIMMUNG_RUN_ID}}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
JOB_NAME="agent-${RUN_SLUG}-im-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-implement"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

IMPL_EXIT_CODE=0
IMPLEMENTATION_JSON="/tmp/implementation.json"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
mkdir -p "$EVIDENCE_DIR/screenshots"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main "$BRANCH_NAME"
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
  copy_claude_ca

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP

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

  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    --from-file=prompt-implementation.md="${REPO_DIR}/.github/agent/prompt-implementation.md" \
    --dry-run=client -o yaml | kubectl apply -f -
}

run_llm() {
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
      --branch-name "$BRANCH_NAME" \
      --proxy-ip "$PROXY_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --repo-slug "$REPO_SLUG" \
      --stage "implement" \
      --config-map-name "$CONFIG_MAP_NAME"
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_IMPLEMENT_TIMEOUT_SECONDS:-2400}"
  )
}

# The implement pod commits + pushes the branch itself. Confirm it's reachable.
push_branch() {
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  if git -c "http.extraHeader=${auth_header}" \
      ls-remote --exit-code "https://github.com/${REPO_SLUG}.git" "refs/heads/${BRANCH_NAME}" >/dev/null; then
    echo "branch ${BRANCH_NAME} is present on the remote"
    return 0
  fi
  echo "branch ${BRANCH_NAME} is absent — implement pod did not complete the push step" >&2
  return 1
}

rebuild_env() {
  local rebuild_tag="${IMAGE_TAG}-r2"
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli rebuild-validation-image \
      --namespace "$NAMESPACE" \
      --branch "$BRANCH_NAME" \
      --image-tag "$rebuild_tag" \
      --repo-slug "$REPO_SLUG"
    python3 -m ambience_preview.cli wait-public-preview --url "$VALIDATION_URL"
  )
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
  if [ -f "${EVIDENCE_DIR}/issue-agent-implementation.json" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-implementation.json" "$IMPLEMENTATION_JSON"
  fi
  if [ -f "${EVIDENCE_DIR}/issue-agent-implementation.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-implementation.md" "$SUMMARY_MD"
  fi
  if [ ! -s "$IMPLEMENTATION_JSON" ]; then
    jq -n '{schema_version:1,status:"fail",abort_reason:"implement pod produced no output"}' \
      >"$IMPLEMENTATION_JSON"
  fi
}

emit() {
  local impl_str
  impl_str="$(cat "$IMPLEMENTATION_JSON")"
  local outputs
  outputs="$(jq -nc \
    --argjson v "$impl_str" \
    --arg branch "$BRANCH_NAME" \
    '{implementation: ($v | tostring), branch_name: $branch}')"
  local summary
  summary="$(cat "$SUMMARY_MD" 2>/dev/null || true)"
  native_completed "$outputs" "null" "" "$summary"
}

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step_allow_failure "llm" run_llm || IMPL_EXIT_CODE=$?
native_step "collect" collect_evidence
if [ "$IMPL_EXIT_CODE" -ne 0 ]; then
  native_step "finalize" finalize
  native_failed "implement pod exited with ${IMPL_EXIT_CODE}"
  exit "$IMPL_EXIT_CODE"
fi
native_step "push-branch" push_branch
native_step "rebuild-env" rebuild_env
native_step "finalize" finalize
native_step "emit" emit
native_assert_resume_satisfied
