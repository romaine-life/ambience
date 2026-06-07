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
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
IMPL_EXIT_CODE_FILE="/tmp/implementation-exit-code"
PROXY_IP_FILE="/tmp/implementation-proxy-ip"
GITHUB_PROXY_IP_FILE="/tmp/implementation-github-proxy-ip"
ISSUE_CONTRACT_FILE="${EVIDENCE_DIR}/issue-agent-contract.json"
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

  local policy_json policy_token policy_branch
  policy_json="$(native_github_push_policy_token_json)"
  policy_token="$(printf '%s' "$policy_json" | jq -r '.token // ""')"
  policy_branch="$(printf '%s' "$policy_json" | jq -r '.branch // ""')"
  if [ -z "$policy_token" ]; then
    echo "GitHub push policy token callback returned no token" >&2
    return 1
  fi
  if [ "$policy_branch" != "$BRANCH_NAME" ]; then
    echo "GitHub push policy token branch ${policy_branch} does not match implementation branch ${BRANCH_NAME}" >&2
    return 1
  fi
  kubectl -n "$NAMESPACE" create secret generic agent-github-policy-token \
    --from-literal=token="$policy_token" \
    --dry-run=client -o yaml | kubectl apply -f -

  if [ -n "${GLIMMUNG_INPUT_ISSUE_CONTRACT:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" | jq -r . >"$ISSUE_CONTRACT_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" >"$ISSUE_CONTRACT_FILE"
    echo "staged issue-contract JSON ($(wc -c <"$ISSUE_CONTRACT_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_ISSUE_CONTRACT not set; implementation will proceed from issue context only"
  fi

  local args=(
    --from-file=prompt-implementation.md="${REPO_DIR}/.github/agent/prompt-implementation.md"
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

run_llm() {
  ensure_proxy_ip
  ensure_github_proxy_ip
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

run_llm_record() {
  native_record_exit_code "$IMPL_EXIT_CODE_FILE" run_llm
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
    return 0
  fi
  echo "branch ${BRANCH_NAME} is absent — implement pod did not complete the push step" >&2
  return 1
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

dispatch_pr_checks() {
  local owner repo workflow_path body
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  workflow_path="$(printf '%s' "${AMBIENCE_PR_CHECK_WORKFLOW:-docker-build-check.yaml}" | jq -sRr @uri)"
  body="$(jq -nc --arg ref "$BRANCH_NAME" '{ref:$ref, inputs:{git_ref:$ref}}')"
  github_api POST "/repos/${owner}/${repo}/actions/workflows/${workflow_path}/dispatches" "$body" >/dev/null
  echo "dispatched ${AMBIENCE_PR_CHECK_WORKFLOW:-docker-build-check.yaml} for ${BRANCH_NAME}"
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
  checks_json="$(github_api GET "/repos/${owner}/${repo}/commits/${branch_revision}/check-runs?per_page=100")"
  if [ "$(printf '%s' "$checks_json" | jq -r '.total_count // ((.check_runs // []) | length)')" -eq 0 ]; then
    dispatch_pr_checks
  fi
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
}

emit() {
  local impl_str
  impl_str="$(cat "$IMPLEMENTATION_JSON")"
  local outputs
  local pr_number="" pr_url=""
  [ -s "$PR_NUMBER_FILE" ] && pr_number="$(cat "$PR_NUMBER_FILE")"
  [ -s "$PR_URL_FILE" ] && pr_url="$(cat "$PR_URL_FILE")"
  outputs="$(jq -nc \
    --argjson v "$impl_str" \
    --arg branch "$BRANCH_NAME" \
    --arg pr_number "$pr_number" \
    --arg pr_url "$pr_url" \
    '{
      implementation: ($v | tostring),
      branch_name: $branch
    }
    + (if $pr_number != "" then {pr_number: ($pr_number | tonumber)} else {} end)
    + (if $pr_url != "" then {pr_url: $pr_url} else {} end)')"
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
