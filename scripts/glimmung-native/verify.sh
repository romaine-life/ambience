#!/usr/bin/env bash
# Glimmung phase: verify.
#
# Runs after both test-plan and implement. Receives the test-plan JSON
# (evidence specification) and implementation JSON from glimmung phase
# outputs, mounts them in the verify pod's configmap, and runs the
# verification LLM against the rebuilt validation env.
#
# Also enforces the test plan's required_evidence contract against the
# verifier's evidence_results before emitting pass.
#
# Outputs emitted to glimmung:
#   verification — issue-agent-verification.json as a JSON string.
#                  Consumed by the downstream evidence_verification_gate phase.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_INPUT_VALIDATION_URL \
  GLIMMUNG_INPUT_NAMESPACE \
  GLIMMUNG_INPUT_BRANCH_NAME

REPO_SLUG="${AMBIENCE_REPO_SLUG:-nelsong6/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-latest}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
BRANCH_NAME="${GLIMMUNG_INPUT_BRANCH_NAME}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
JOB_NAME="agent-${RUN_SLUG}-ve-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-verify"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

VERIFY_EXIT_CODE=0
VERIFICATION_JSON="/tmp/verification.json"
VERIFICATION_REASONS="/tmp/verification-reasons.txt"
EVIDENCE_REFS="/tmp/evidence-refs.txt"
SCREENSHOTS_MD="/tmp/screenshots.md"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
: >"$VERIFICATION_REASONS"
: >"$EVIDENCE_REFS"
: >"$SCREENSHOTS_MD"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
mkdir -p "$EVIDENCE_DIR/screenshots"

# Stage handoff files — written by prepare_context from glimmung inputs.
TEST_PLAN_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.json"
TEST_PLAN_MD_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.md"
IMPL_FILE="${EVIDENCE_DIR}/issue-agent-implementation.json"
IMPL_MD_FILE="${EVIDENCE_DIR}/issue-agent-implementation.md"

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

  # Stage handoff artifacts from glimmung phase outputs into evidence dir.
  # GLIMMUNG_INPUT_TEST_PLAN is the test-plan JSON string; unwrap it.
  if [ -n "${GLIMMUNG_INPUT_TEST_PLAN:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" | jq -r . >"$TEST_PLAN_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" >"$TEST_PLAN_FILE"
    echo "staged test-plan JSON ($(wc -c <"$TEST_PLAN_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_TEST_PLAN not set; verify will proceed without test-plan context"
  fi

  if [ -n "${GLIMMUNG_INPUT_IMPLEMENTATION:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" | jq -r . >"$IMPL_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" >"$IMPL_FILE"
    echo "staged implementation JSON ($(wc -c <"$IMPL_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_IMPLEMENTATION not set; verify will proceed without implementation context"
  fi

  # Build the agent-config configmap with the prompt + all available handoff files.
  local args=(
    --from-file=prompt-verification.md="${REPO_DIR}/.github/agent/prompt-verification.md"
  )
  for f in "$TEST_PLAN_FILE" "$TEST_PLAN_MD_FILE" "$IMPL_FILE" "$IMPL_MD_FILE"; do
    [ -s "$f" ] || continue
    base="$(basename "$f")"
    args+=(--from-file="${base}=${f}")
  done
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
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
      --stage "verify" \
      --config-map-name "$CONFIG_MAP_NAME"
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_VERIFY_TIMEOUT_SECONDS:-1800}"
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

add_reason() {
  printf '%s\n' "$1" >>"$VERIFICATION_REASONS"
}

verification_cost() {
  if [ -s "$EVENTS_LOG" ]; then
    jq -r 'select(.type=="result") | .total_cost_usd // 0' "$EVENTS_LOG" \
      | awk '{s+=$1} END {if (NR>0) printf "%.4f", s; else printf "0"}'
  else
    printf '0'
  fi
}

enforce_evidence_contract() {
  local plan="${TEST_PLAN_FILE}"
  local verify="${EVIDENCE_DIR}/issue-agent-verification.json"
  if [ ! -s "$plan" ] || [ ! -s "$verify" ]; then
    add_reason "missing handoff JSON; cannot enforce evidence contract"
    return 1
  fi
  local missing
  missing="$(
    jq -nr \
      --slurpfile plan "$plan" \
      --slurpfile verify "$verify" \
      '
        ($plan[0].required_evidence // []) as $req
        | ($verify[0].evidence_results // []) as $res
        | $req[]
        | . as $r
        | ($res | map(select(.id == $r.id))) as $match
        | if ($match | length) == 0 then "missing:" + ($r.id // "")
          elif $match[0].status != "pass" then "not_pass:" + ($r.id // "")
          else empty
          end
      ' || true
  )"
  if [ -n "$missing" ]; then
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      add_reason "evidence contract: $line"
    done <<<"$missing"
    return 1
  fi
  return 0
}

write_verification() {
  local status="$1"
  local cost
  cost="$(verification_cost)"
  find "${EVIDENCE_DIR}/screenshots" -maxdepth 1 -type f -name '*.png' \
    -printf 'screenshots/%f\n' 2>/dev/null | sort >"$EVIDENCE_REFS" || true
  jq -n \
    --arg status "$status" \
    --argjson reasons "$(jq -Rs 'split("\n")[:-1]' "$VERIFICATION_REASONS")" \
    --argjson evidence_refs "$(jq -Rs 'split("\n")[:-1]' "$EVIDENCE_REFS")" \
    --argjson cost_usd "${cost:-0}" \
    --arg run_id "$GLIMMUNG_RUN_ID" \
    --arg branch "$BRANCH_NAME" \
    --arg validation_url "$VALIDATION_URL" \
    '{
      schema_version: 1,
      status: $status,
      reasons: $reasons,
      evidence_refs: $evidence_refs,
      cost_usd: $cost_usd,
      prompt_version: "ambience-native-staged-v1",
      metadata: {
        run_id: $run_id,
        branch: $branch,
        validation_url: $validation_url
      }
    }' >"$VERIFICATION_JSON"
  cat "$VERIFICATION_JSON"
}

finalize() {
  if [ "$VERIFY_EXIT_CODE" -ne 0 ]; then
    add_reason "verify pod exited with ${VERIFY_EXIT_CODE}; see native step logs"
    if [ -s "$POD_LOG" ]; then
      grep -E "::error::|Job failed|FATAL|panic:|aborted:|forbidden|exited without writing" "$POD_LOG" \
        | head -5 >>"$VERIFICATION_REASONS" || true
    fi
    write_verification "fail"
    return 0
  fi

  local verifier_status
  verifier_status="$(jq -r '.status // "missing"' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo missing)"
  if [ "$verifier_status" != "pass" ]; then
    add_reason "verifier reported status=${verifier_status} reason=$(jq -r '.abort_reason // ""' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo "")"
    write_verification "fail"
    return 0
  fi

  if ! enforce_evidence_contract; then
    write_verification "fail"
    return 0
  fi

  write_verification "pass"
  write_summary
}

upload_screenshots() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"

  if ! compgen -G "${EVIDENCE_DIR}/screenshots/*.png" >/dev/null; then
    return 0
  fi

  local prefix staging total taken upload_ok
  prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/screenshots"
  staging="$(mktemp -d)"
  total=0
  taken=0
  while IFS= read -r file; do
    total=$((total + 1))
    if [ "$taken" -lt "$max_screenshots" ]; then
      cp "$file" "$staging/"
      taken=$((taken + 1))
    fi
  done < <(find "${EVIDENCE_DIR}/screenshots" -maxdepth 1 -type f -name '*.png' | sort)

  upload_ok=true
  if ! az storage blob upload-batch \
      --account-name "$storage_account" \
      --destination "$container" \
      --destination-path "$prefix" \
      --source "$staging" \
      --auth-mode login \
      --overwrite true; then
    upload_ok=false
    echo "screenshot upload failed; report body will point at native logs"
  fi

  {
    echo "## Screenshots"
    echo ""
    if [ "$upload_ok" = "false" ]; then
      echo "_Screenshot upload failed; see the Glimmung native run logs._"
      echo ""
    else
      if [ "$total" -gt "$taken" ]; then
        echo "_Showing first ${taken} of ${total} screenshots._"
        echo ""
      fi
      for file in "$staging"/*.png; do
        [ -e "$file" ] || continue
        base="$(basename "$file")"
        echo "### ${base%.png}"
        echo ""
        echo "![${base%.png}](${container_url}/${prefix}/${base})"
        echo ""
      done
    fi
  } >"$SCREENSHOTS_MD"
  rm -rf "$staging"
}

write_summary() {
  if [ -s "${EVIDENCE_DIR}/issue-agent-verification.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-verification.md" "$SUMMARY_MD"
    return 0
  fi
  if [ -s "${EVIDENCE_DIR}/issue-agent-implementation.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-implementation.md" "$SUMMARY_MD"
    return 0
  fi
  if [ -s "${EVIDENCE_DIR}/issue-agent-test-plan.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.md" "$SUMMARY_MD"
  fi
}

emit() {
  if [ ! -s "$VERIFICATION_JSON" ]; then
    add_reason "verification result was not produced"
    write_verification "error"
  fi
  cat "$VERIFICATION_JSON"

  local verification_outputs
  verification_outputs="$(jq -nc --slurpfile v "$VERIFICATION_JSON" '{verification: ($v[0] | tostring)}')"
  native_completed \
    "$verification_outputs" \
    "$(cat "$VERIFICATION_JSON")" \
    "$(cat "$SCREENSHOTS_MD")" \
    "$(cat "$SUMMARY_MD")"
}

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step_allow_failure "llm" run_llm || VERIFY_EXIT_CODE=$?
native_step "collect" collect_evidence
native_step "finalize" finalize
native_step_allow_failure "upload-screenshots" upload_screenshots || true
native_step "emit" emit
native_assert_resume_satisfied
