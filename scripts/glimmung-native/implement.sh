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
#   ui_hint        — {menu_label, route} as a JSON string, when the
#                    implementation declared one. For feature types with a
#                    standing verification case (AMBIENCE_FEATURE_TYPE, e.g.
#                    "effect") a passing implementation MUST declare it: the
#                    verifier needs it to find the new surface. The hint is a
#                    discovery aid only — judgment criteria stay the issue
#                    text, so this is navigation knowledge flowing forward,
#                    not evaluation knowledge.

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

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
WORKFLOW_REF="$(native_workflow_checkout_ref)"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"
PROVIDER_API_PROXY_NAMESPACE="${GLIMMUNG_INPUT_PROVIDER_API_PROXY_NAMESPACE:-${GLIMMUNG_PROVIDER_API_PROXY_NAMESPACE:-glimmung-runs}}"
GITHUB_POLICY_CA_SECRET="${GLIMMUNG_GITHUB_POLICY_CA_SECRET:-glimmung-provider-api-proxy-ca}"
GITHUB_POLICY_CA_CONFIGMAP="${GLIMMUNG_GITHUB_POLICY_CA_CONFIGMAP:-glimmung-provider-api-proxy-ca}"
GITHUB_POLICY_PROXY_SERVICE="${GLIMMUNG_GITHUB_POLICY_PROXY_SERVICE:-github-git-policy-proxy}"
# Name-based api.github.com egress for the locked-down agent runs through the
# agent-egress Envoy Gateway (TLS passthrough). EG provisions its data-plane
# Service in ENVOY_GATEWAY_SYSTEM_NAMESPACE; resolve that Service's clusterIP by
# the EG owning-gateway labels and hostAlias api.github.com onto it.
AGENT_EGRESS_GATEWAY_NAME="${GLIMMUNG_AGENT_EGRESS_GATEWAY_NAME:-agent-egress}"
AGENT_EGRESS_GATEWAY_NAMESPACE="${GLIMMUNG_AGENT_EGRESS_GATEWAY_NAMESPACE:-${PROVIDER_API_PROXY_NAMESPACE}}"
ENVOY_GATEWAY_SYSTEM_NAMESPACE="${GLIMMUNG_ENVOY_GATEWAY_SYSTEM_NAMESPACE:-envoy-gateway-system}"
GITHUB_EGRESS_IP_FILE="/tmp/implementation-github-egress-ip"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
IMAGE_TAG="${GLIMMUNG_INPUT_IMAGE_TAG}"
BRANCH_NAME="$(native_implementation_branch_name)"
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
EVIDENCE_DIR="${AMBIENCE_EVIDENCE_DIR:-/tmp/evidence}"
IMPLEMENTATION_CONTRACT_JSON="${EVIDENCE_DIR}/implementation-contract.json"
POD_LOG="/tmp/agent-pod.log"
IMPL_EXIT_CODE_FILE="/tmp/implementation-exit-code"
PROXY_IP_FILE="/tmp/implementation-proxy-ip"
GITHUB_PROXY_IP_FILE="/tmp/implementation-github-proxy-ip"
PR_NUMBER_FILE="/tmp/implementation-pr-number"
PR_URL_FILE="/tmp/implementation-pr-url"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
printf '0\n' >"$IMPL_EXIT_CODE_FILE"
mkdir -p "$EVIDENCE_DIR/screenshots" "$EVIDENCE_DIR/videos"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" "$WORKFLOW_REF" "$BRANCH_NAME"
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

copy_github_policy_ca() {
  kubectl -n "$PROVIDER_API_PROXY_NAMESPACE" get secret "$GITHUB_POLICY_CA_SECRET" -o json \
    | NAMESPACE="$NAMESPACE" CONFIGMAP="$GITHUB_POLICY_CA_CONFIGMAP" jq '
        {
          apiVersion: "v1",
          kind: "ConfigMap",
          metadata: {
            name: env.CONFIGMAP,
            namespace: env.NAMESPACE
          },
          data: {
            "ca.crt": (.data["ca.crt"] | @base64d)
          }
        }
      ' \
    | kubectl apply -f -
}

prepare_context() {
  native_azure_login
  native_install_preview_package "${REPO_DIR}/mcp"
  generate_implementation_contract
  copy_claude_ca
  copy_github_policy_ca
  native_prepare_codex_credentials_secret "$NAMESPACE"

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP
  printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"

  GITHUB_PROXY_IP="$(kubectl -n "$PROVIDER_API_PROXY_NAMESPACE" get svc "$GITHUB_POLICY_PROXY_SERVICE" -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$GITHUB_PROXY_IP" ]; then
    echo "${GITHUB_POLICY_PROXY_SERVICE} Service not found in ${PROVIDER_API_PROXY_NAMESPACE}" >&2
    return 1
  fi
  export GITHUB_PROXY_IP
  printf '%s\n' "$GITHUB_PROXY_IP" >"$GITHUB_PROXY_IP_FILE"

  # Stage the GitHub token the implementation agent Job mounts as
  # GITHUB_TOKEN_FILE — used to push its branch and read its draft PR's CI.
  # Mirrors the other agent stages (test-plan/verify). Branch
  # scoping is enforced by the github-git-policy-proxy via the agent pod's
  # github-policy-{repo,ref} annotations, not by the token; the signed
  # push-policy-token model was retired in glimmung #739.
  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -

  local args=(
    --from-file=prompt-implementation.md="${REPO_DIR}/.github/agent/prompt-implementation.md"
  )
  if [ -s "$IMPLEMENTATION_CONTRACT_JSON" ]; then
    args+=(--from-file=implementation-contract.json="$IMPLEMENTATION_CONTRACT_JSON")
  fi
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

generate_implementation_contract() {
  (
    cd "$REPO_DIR"
    AMBIENCE_IMPLEMENTATION_CONTRACT="$IMPLEMENTATION_CONTRACT_JSON" \
      scripts/agent/contracts/generate.sh "${AMBIENCE_FEATURE_TYPE:-generic}" "$IMPLEMENTATION_CONTRACT_JSON" >/dev/null
  )
  echo "generated implementation contract ${IMPLEMENTATION_CONTRACT_JSON}"
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

ensure_github_proxy_ip() {
  if [ -z "${GITHUB_PROXY_IP:-}" ] && [ -s "$GITHUB_PROXY_IP_FILE" ]; then
    GITHUB_PROXY_IP="$(cat "$GITHUB_PROXY_IP_FILE")"
    export GITHUB_PROXY_IP
  fi
  if [ -z "${GITHUB_PROXY_IP:-}" ]; then
    GITHUB_PROXY_IP="$(kubectl -n "$PROVIDER_API_PROXY_NAMESPACE" get svc "$GITHUB_POLICY_PROXY_SERVICE" -o jsonpath='{.spec.clusterIP}')"
    if [ -z "$GITHUB_PROXY_IP" ]; then
      echo "${GITHUB_POLICY_PROXY_SERVICE} Service not found in ${PROVIDER_API_PROXY_NAMESPACE}" >&2
      return 1
    fi
    export GITHUB_PROXY_IP
    printf '%s\n' "$GITHUB_PROXY_IP" >"$GITHUB_PROXY_IP_FILE"
  fi
}

ensure_github_egress_ip() {
  if [ -z "${GITHUB_EGRESS_IP:-}" ] && [ -s "$GITHUB_EGRESS_IP_FILE" ]; then
    GITHUB_EGRESS_IP="$(cat "$GITHUB_EGRESS_IP_FILE")"
    export GITHUB_EGRESS_IP
  fi
  if [ -z "${GITHUB_EGRESS_IP:-}" ]; then
    GITHUB_EGRESS_IP="$(kubectl -n "$ENVOY_GATEWAY_SYSTEM_NAMESPACE" get svc \
      -l "gateway.envoyproxy.io/owning-gateway-name=${AGENT_EGRESS_GATEWAY_NAME},gateway.envoyproxy.io/owning-gateway-namespace=${AGENT_EGRESS_GATEWAY_NAMESPACE}" \
      -o jsonpath='{.items[0].spec.clusterIP}' 2>/dev/null || true)"
    if [ -z "$GITHUB_EGRESS_IP" ]; then
      # Non-fatal: the agent's live CI self-check (agent-ci-feedback.sh) loses
      # api.github.com this run, but the deterministic wait_pr_checks gate runs
      # in the (non-egress-locked) wrapper and still gates CI. Don't fail the run.
      echo "WARNING: agent-egress gateway data-plane Service not found in ${ENVOY_GATEWAY_SYSTEM_NAMESPACE} for gateway ${AGENT_EGRESS_GATEWAY_NAMESPACE}/${AGENT_EGRESS_GATEWAY_NAME}; agent will not reach api.github.com this run" >&2
      return 0
    fi
    export GITHUB_EGRESS_IP
    printf '%s\n' "$GITHUB_EGRESS_IP" >"$GITHUB_EGRESS_IP_FILE"
  fi
}

run_llm() {
  ensure_proxy_ip
  ensure_github_proxy_ip
  ensure_github_egress_ip
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
      --github-proxy-ip "$GITHUB_PROXY_IP" \
      --github-egress-ip "$GITHUB_EGRESS_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --agent-container-image "$(native_agent_container_image)" \
      --repo-slug "$REPO_SLUG" \
      --stage "implement" \
      --config-map-name "$CONFIG_MAP_NAME" \
      --agent-runtime-json "${GLIMMUNG_AGENT_RUNTIME_JSON:-}"
    native_emit_inner_job_marker "$NAMESPACE" "$JOB_NAME" helper implement-agent
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_IMPLEMENT_TIMEOUT_SECONDS:-2400}"
  )
}

first_edit_status() {
  git -C "$REPO_DIR" status --porcelain --untracked-files=normal 2>/dev/null || true
}

first_edit_files_json() {
  awk '
    NF > 0 {
      path = substr($0, 4)
      if (path != "") print path
    }
  ' | head -10 | jq -Rsc 'split("\n")[:-1]'
}

monitor_first_edit_latency() {
  local started_at poll_seconds status_json changed elapsed metadata
  started_at="$(date +%s)"
  poll_seconds="${AMBIENCE_FIRST_EDIT_POLL_SECONDS:-5}"
  case "$poll_seconds" in
    ''|*[!0-9]*) poll_seconds="5" ;;
    0) poll_seconds="1" ;;
  esac
  while true; do
    status_json="$(first_edit_status)"
    if [ -n "$status_json" ]; then
      elapsed="$(($(date +%s) - started_at))"
      changed="$(printf '%s\n' "$status_json" | first_edit_files_json)"
      metadata="$(
        jq -nc \
          --argjson seconds "$elapsed" \
          --argjson files "$changed" \
          '{metric:"first_edit_latency_seconds", value:$seconds, first_changed_files:$files}'
      )"
      native_event "metric" "run-implementation" "first repo edit after ${elapsed}s" "" "$metadata" || true
      echo "first_edit_latency_seconds=${elapsed} first_changed_files=$(jq -cr . <<<"$changed")"
      return 0
    fi
    sleep "$poll_seconds"
  done
}

run_llm_record() {
  local monitor_pid
  monitor_first_edit_latency &
  monitor_pid=$!
  native_record_exit_code "$IMPL_EXIT_CODE_FILE" run_llm
  if kill "$monitor_pid" >/dev/null 2>&1; then
    wait "$monitor_pid" 2>/dev/null || true
  else
    wait "$monitor_pid" 2>/dev/null || true
  fi
  IMPL_EXIT_CODE="$(native_read_exit_code "$IMPL_EXIT_CODE_FILE")"
}

impl_exit_code() {
  native_read_exit_code "$IMPL_EXIT_CODE_FILE"
}

implementation_failed() {
  [ "$(impl_exit_code)" -ne 0 ]
}

# The implement pod commits + pushes the branch itself. Confirm it's reachable.
push_branch() {
  if implementation_failed; then
    echo "implementation pod failed; skipping branch confirmation"
    return 0
  fi
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  if git -c "http.extraHeader=${auth_header}" \
      ls-remote --exit-code "https://github.com/${REPO_SLUG}.git" "refs/heads/${BRANCH_NAME}" >/dev/null; then
    echo "branch ${BRANCH_NAME} is present on the remote"
    validate_implementation_contract
    return 0
  fi
  echo "branch ${BRANCH_NAME} is absent — implement pod did not complete the push step" >&2
  return 1
}

validate_implementation_contract() {
  if implementation_failed; then
    echo "implementation pod failed; skipping implementation contract validation"
    return 0
  fi
  if [ ! -s "$IMPLEMENTATION_CONTRACT_JSON" ]; then
    echo "no implementation contract present; skipping validation"
    return 0
  fi
  if [ ! -s "$IMPLEMENTATION_JSON" ]; then
    if [ -f "${EVIDENCE_DIR}/issue-agent-implementation.json" ]; then
      cp "${EVIDENCE_DIR}/issue-agent-implementation.json" "$IMPLEMENTATION_JSON"
    fi
  fi
  if [ ! -s "$IMPLEMENTATION_JSON" ]; then
    echo "implementation contract validation cannot run without implementation JSON" >&2
    return 1
  fi

  local token auth_header branch_revision
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  git -C "$REPO_DIR" \
    -c "http.extraHeader=${auth_header}" \
    fetch --force origin \
      "+refs/heads/main:refs/remotes/origin/main" \
      "+refs/heads/${BRANCH_NAME}:refs/remotes/origin/${BRANCH_NAME}"
  branch_revision="$(git -C "$REPO_DIR" rev-parse "origin/${BRANCH_NAME}")"
  "${SCRIPT_DIR}/../agent/contracts/validate.sh" \
    "$IMPLEMENTATION_CONTRACT_JSON" \
    "$IMPLEMENTATION_JSON" \
    "$REPO_DIR" \
    "origin/main" \
    "$branch_revision" >/tmp/implementation-contract-validation.json
  echo "implementation contract satisfied: $(jq -c '{feature_type, effect_slug, changed_files}' /tmp/implementation-contract-validation.json)"
}

prepare_draft_pr_branch() {
  if native_remote_branch_revision "$REPO_SLUG" "$BRANCH_NAME" >/dev/null 2>&1; then
    echo "implementation branch ${BRANCH_NAME} already exists"
    return 0
  fi
  git -C "$REPO_DIR" commit --allow-empty \
    -m "agent: start ${ISSUE_REFERENCE}" \
    -m "Glimmung run ${GLIMMUNG_RUN_ID} opened this branch before implementation so CI feedback is visible during the agent loop."
  native_push_branch "$REPO_SLUG" "$REPO_DIR" "$BRANCH_NAME"
  echo "created initial implementation branch ${BRANCH_NAME}"
}

github_api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local token
  token="$(native_github_token)"
  if [ -n "$body" ]; then
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "https://api.github.com${path}"
  else
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "https://api.github.com${path}"
  fi
}

ensure_draft_pr() {
  local owner repo base head_query existing pr_body create_body pr_number pr_url title body
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  base="${AMBIENCE_PR_BASE:-main}"
  head_query="$(printf '%s:%s' "$owner" "$BRANCH_NAME" | jq -sRr @uri)"
  existing="$(github_api GET "/repos/${owner}/${repo}/pulls?head=${head_query}&state=open&per_page=1")"
  title="Glimmung ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
  body="$(
    cat <<EOF
Draft PR opened by the Ambience Glimmung implementation wrapper.

- Glimmung run: ${GLIMMUNG_RUN_ID}
- Issue: ${ISSUE_URL}
- Branch: ${BRANCH_NAME}

Deterministic CI checks must pass before LLM verification starts. The later Glimmung touchpoint is a human review boundary; it does not merge this PR.
EOF
  )"

  if [ "$(printf '%s' "$existing" | jq 'length')" -gt 0 ]; then
    pr_number="$(printf '%s' "$existing" | jq -r '.[0].number')"
    pr_url="$(printf '%s' "$existing" | jq -r '.[0].html_url')"
    pr_body="$(jq -nc --arg title "$title" --arg body "$body" '{title:$title, body:$body}')"
    github_api PATCH "/repos/${owner}/${repo}/pulls/${pr_number}" "$pr_body" >/dev/null
    echo "updated existing implementation PR ${pr_url}"
  else
    create_body="$(jq -nc \
      --arg title "$title" \
      --arg head "$BRANCH_NAME" \
      --arg base "$base" \
      --arg body "$body" \
      '{title:$title, head:$head, base:$base, body:$body, draft:true}')"
    pr_body="$(github_api POST "/repos/${owner}/${repo}/pulls" "$create_body")"
    pr_number="$(printf '%s' "$pr_body" | jq -r '.number')"
    pr_url="$(printf '%s' "$pr_body" | jq -r '.html_url')"
    echo "opened draft implementation PR ${pr_url}"
  fi

  printf '%s\n' "$pr_number" >"$PR_NUMBER_FILE"
  printf '%s\n' "$pr_url" >"$PR_URL_FILE"
}

wait_pr_checks() {
  if implementation_failed; then
    echo "implementation pod failed; skipping PR checks"
    return 0
  fi

  local owner repo branch_revision deadline now poll_seconds checks_json status_json check_count status_count pending_count failing_count combined_state summary
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  branch_revision="$(native_remote_branch_revision "$REPO_SLUG" "$BRANCH_NAME")"
  poll_seconds="${AMBIENCE_PR_CHECK_POLL_SECONDS:-30}"
  deadline="$(($(date +%s) + ${AMBIENCE_PR_CHECK_TIMEOUT_SECONDS:-3600}))"

  echo "waiting for PR checks on ${BRANCH_NAME}@${branch_revision}"
  # CI fires on the draft PR's pull_request event; the gate only waits for and
  # reads those checks. It never dispatches — a missing check set times out and
  # fails the gate deterministically rather than papering over it.
  while true; do
    checks_json="$(github_api GET "/repos/${owner}/${repo}/commits/${branch_revision}/check-runs?per_page=100")"
    status_json="$(github_api GET "/repos/${owner}/${repo}/commits/${branch_revision}/status")"

    check_count="$(printf '%s' "$checks_json" | jq -r '.total_count // ((.check_runs // []) | length)')"
    status_count="$(printf '%s' "$status_json" | jq -r '(.statuses // []) | length')"
    pending_count="$(printf '%s' "$checks_json" | jq -r '(.check_runs // []) | map(select(.status != "completed")) | length')"
    failing_count="$(printf '%s' "$checks_json" | jq -r '
      (.check_runs // [])
      | map(. as $run | select($run.status == "completed" and ((["success", "neutral", "skipped"] | index($run.conclusion // "")) | not)))
      | length
    ')"
    combined_state="$(printf '%s' "$status_json" | jq -r '.state // "pending"')"

    if [ "$failing_count" -gt 0 ] || [ "$combined_state" = "failure" ] || [ "$combined_state" = "error" ]; then
      summary="$(
        jq -n \
          --argjson checks "$checks_json" \
          --argjson status "$status_json" \
          '{
            failing_check_runs: [
              ($checks.check_runs // [])[]
              | . as $run
              | select($run.status == "completed" and ((["success", "neutral", "skipped"] | index($run.conclusion // "")) | not))
              | {name, status, conclusion, html_url}
            ],
            combined_status_state: ($status.state // "pending"),
            statuses: [
              ($status.statuses // [])[]
              | select((.state // "") != "success")
              | {context, state, target_url, description}
            ]
          }'
      )"
      echo "PR checks failed: ${summary}" >&2
      return 1
    fi

    if [ "$check_count" -gt 0 ] || [ "$status_count" -gt 0 ]; then
      if [ "$pending_count" -eq 0 ] && { [ "$combined_state" = "success" ] || [ "$status_count" -eq 0 ]; }; then
        echo "PR checks passed for ${BRANCH_NAME}@${branch_revision}"
        return 0
      fi
    fi

    now="$(date +%s)"
    if [ "$now" -ge "$deadline" ]; then
      summary="$(
        jq -n \
          --argjson checks "$checks_json" \
          --argjson status "$status_json" \
          '{
            check_runs: [($checks.check_runs // [])[] | {name, status, conclusion, html_url}],
            combined_status_state: ($status.state // "pending"),
            statuses: [($status.statuses // [])[] | {context, state, target_url, description}]
          }'
      )"
      echo "timed out waiting for PR checks: ${summary}" >&2
      return 1
    fi

    echo "checks pending: check_runs=${check_count}, pending=${pending_count}, commit_statuses=${status_count}, combined_status=${combined_state}"
    sleep "$poll_seconds"
  done
}

rebuild_env() {
  if implementation_failed; then
    echo "implementation pod failed; skipping validation rebuild"
    return 0
  fi
  local branch_revision rebuild_tag
  branch_revision="$(native_remote_branch_revision "$REPO_SLUG" "$BRANCH_NAME")"
  rebuild_tag="$(native_image_tag_for_revision "$branch_revision")"
  echo "rebuilding validation image from ${BRANCH_NAME}@${branch_revision}"
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli rebuild-validation-image \
      --namespace "$NAMESPACE" \
      --branch "$BRANCH_NAME" \
      --image-tag "$rebuild_tag" \
      --source-revision "$branch_revision" \
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
    local exit_code
    exit_code="$(impl_exit_code)"
    if [ "$exit_code" -ne 0 ]; then
      jq -n --arg reason "implement pod exited with ${exit_code}" \
        '{schema_version:1,status:"fail",abort_reason:$reason}' \
        >"$IMPLEMENTATION_JSON"
      return 0
    fi
    jq -n '{schema_version:1,status:"fail",abort_reason:"implement pod produced no output"}' \
      >"$IMPLEMENTATION_JSON"
  fi
  enforce_ui_hint_contract
}

# Feature types with a standing verification case bind that case to the
# implementation's ui_hint at verify time. A passing implementation without a
# usable hint would fail one expensive phase later with a much vaguer error —
# fail it HERE, named, before any verify spend. Returning nonzero fails the
# finalize step itself, which is the only failure channel that survives
# managed per-step invocation (the /tmp exit-code file is re-initialized by
# every step's script start). Route shape mirrors the verify-side binding
# check (resolve_standing_case in verify.sh).
enforce_ui_hint_contract() {
  [ "${AMBIENCE_FEATURE_TYPE:-}" = "" ] && return 0
  [ "$(jq -r '.status // ""' "$IMPLEMENTATION_JSON" 2>/dev/null || printf '')" = "pass" ] || return 0
  local label route
  label="$(jq -r '.ui_hint.menu_label // ""' "$IMPLEMENTATION_JSON" 2>/dev/null || printf '')"
  route="$(jq -r '.ui_hint.route // ""' "$IMPLEMENTATION_JSON" 2>/dev/null || printf '')"
  case "$route" in
    /dev/[a-z0-9_-]*) ;;
    *) route="" ;;
  esac
  if [ -z "$label" ] || [ -z "$route" ]; then
    local got
    got="$(jq -c '.ui_hint // null' "$IMPLEMENTATION_JSON" 2>/dev/null || printf 'null')"
    jq --arg got "$got" \
      '.status = "fail"
       | .abort_reason = "missing_ui_hint"
       | .summary = ((.summary // "") + " [wrapper: feature_type requires ui_hint {menu_label, route:/dev/<effect>} on a passing implementation; got " + $got + "]")' \
      "$IMPLEMENTATION_JSON" >"${IMPLEMENTATION_JSON}.tmp" \
      && mv "${IMPLEMENTATION_JSON}.tmp" "$IMPLEMENTATION_JSON"
    echo "missing_ui_hint: feature_type=${AMBIENCE_FEATURE_TYPE} requires a passing implementation to declare ui_hint {menu_label, route:/dev/<effect>}; got ${got}" >&2
    return 1
  fi
  echo "ui_hint contract satisfied: menu_label=${label} route=${route}"
}

emit() {
  local impl_str
  impl_str="$(cat "$IMPLEMENTATION_JSON")"
  local outputs
  local pr_number="" pr_url="" ui_hint=""
  [ -s "$PR_NUMBER_FILE" ] && pr_number="$(cat "$PR_NUMBER_FILE")"
  [ -s "$PR_URL_FILE" ] && pr_url="$(cat "$PR_URL_FILE")"
  ui_hint="$(printf '%s' "$impl_str" | jq -c '.ui_hint // empty' 2>/dev/null || printf '')"
  outputs="$(jq -nc \
    --argjson v "$impl_str" \
    --arg branch "$BRANCH_NAME" \
    --arg pr_number "$pr_number" \
    --arg pr_url "$pr_url" \
    --arg ui_hint "$ui_hint" \
    '{
      implementation: ($v | tostring),
      branch_name: $branch
    }
    + (if $pr_number != "" then {pr_number: ($pr_number | tonumber)} else {} end)
    + (if $pr_url != "" then {pr_url: $pr_url} else {} end)
    + (if $ui_hint != "" then {ui_hint: $ui_hint} else {} end)')"
  local summary
  summary="$(cat "$SUMMARY_MD" 2>/dev/null || true)"
  native_completed "$outputs" "null" "" "$summary"
  local exit_code
  exit_code="$(impl_exit_code)"
  if native_selected_step && [ "$exit_code" -ne 0 ]; then
    native_failed "implement pod exited with ${exit_code}"
    return "$exit_code"
  fi
}

# Test seam: the contract harness sources this file to exercise the
# finalize/emit functions directly against staged files (see
# scripts/test-glimmung-native-contract.sh). Sourcing must define the
# functions and globals without running any step.
if [ "${AMBIENCE_IMPLEMENT_SOURCE_ONLY:-}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

if native_selected_step; then
  native_run_selected_step \
    "clone" clone_repo \
    "prepare" prepare_context \
    "prepare-draft-pr-branch" prepare_draft_pr_branch \
    "ensure-draft-pr" ensure_draft_pr \
    "run-implementation" run_llm_record \
    "collect" collect_evidence \
    "push-branch" push_branch \
    "wait-pr-checks" wait_pr_checks \
    "rebuild-env" rebuild_env \
    "finalize" finalize \
    "emit" emit
  exit $?
fi

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step "prepare-draft-pr-branch" prepare_draft_pr_branch
native_step "ensure-draft-pr" ensure_draft_pr
native_step "run-implementation" run_llm_record
native_step "collect" collect_evidence
IMPL_EXIT_CODE="$(impl_exit_code)"
if [ "$IMPL_EXIT_CODE" -ne 0 ]; then
  native_step "finalize" finalize
  native_failed "implement pod exited with ${IMPL_EXIT_CODE}"
  exit "$IMPL_EXIT_CODE"
fi
native_step "push-branch" push_branch
native_step "wait-pr-checks" wait_pr_checks
native_step "rebuild-env" rebuild_env
native_step "finalize" finalize
native_step "emit" emit
native_assert_resume_satisfied
