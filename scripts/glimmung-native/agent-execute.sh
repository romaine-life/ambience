#!/usr/bin/env bash

# Two-pod issue-agent flow. Replaces the prior monolithic single-pod
# shape. See docs/issue-agent-stage-split.md and
# tank-operator/docs/agent-llm-task-splitting.md for the principle.
#
# Pod 1 — plan-and-implement: two `claude --print` calls in sequence
#   (test-plan, implementation). Pod 1 commits + pushes the branch.
# Wrapper rebuilds the validation env onto the pushed branch.
# Pod 2 — verify: one `claude --print` call that reads the prior
#   stages' handoff JSON+MD (re-mounted via configmap) and captures
#   evidence against the rebuilt env.
#
# The wrapper enforces the test plan's required_evidence contract
# against the verifier's claimed pass before emitting `pass`.

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
JOB_NAME_BASE="agent-${RUN_SLUG}-${ATTEMPT_INDEX}"
PLAN_JOB_NAME="${JOB_NAME_BASE}-plan"
VERIFY_JOB_NAME="${JOB_NAME_BASE}-verify"

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
VERIFY_EXIT_CODE=0
VERIFICATION_JSON="/tmp/verification.json"
VERIFICATION_REASONS="/tmp/verification-reasons.txt"
EVIDENCE_REFS="/tmp/evidence-refs.txt"
SCREENSHOTS_MD="/tmp/screenshots.md"
SUMMARY_MD="/tmp/summary.md"
EVIDENCE_DIR="/tmp/evidence"
STAGE1_LOG="/tmp/agent-pod-stage1.log"
STAGE2_LOG="/tmp/agent-pod-stage2.log"
COMBINED_EVENTS="/tmp/agent-events.jsonl"
: >"$VERIFICATION_REASONS"
: >"$EVIDENCE_REFS"
: >"$SCREENSHOTS_MD"
: >"$SUMMARY_MD"
mkdir -p "$EVIDENCE_DIR/screenshots"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main "$BRANCH_NAME"
}

install_preview_package() {
  python3 -m pip install --user --upgrade pip
  python3 -m pip install --user "${REPO_DIR}/mcp"
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

# Build /tmp/agent-prompt-context.md with the issue heading + body.
# Both stages use this as a context block appended after their stage prompt.
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
  echo "agent prompt context size: $(wc -l <"$dest") lines, $(wc -c <"$dest") bytes"
}

# Write/refresh the agent-config configmap. Pod 1 needs the test-plan and
# implementation prompts; pod 2 needs the verification prompt plus the
# prior-stage handoff JSON+MD files (extracted from pod 1's evidence tar).
prepare_agent_config_plan_impl() {
  kubectl -n "$NAMESPACE" create configmap agent-config \
    --from-file=prompt-test-plan.md="${REPO_DIR}/.github/agent/prompt-test-plan.md" \
    --from-file=prompt-implementation.md="${REPO_DIR}/.github/agent/prompt-implementation.md" \
    --dry-run=client -o yaml | kubectl apply -f -
}

prepare_agent_config_verify() {
  local args=(
    --from-file=prompt-verification.md="${REPO_DIR}/.github/agent/prompt-verification.md"
  )
  for f in issue-agent-test-plan.json issue-agent-test-plan.md \
           issue-agent-implementation.json issue-agent-implementation.md; do
    if [ -f "${EVIDENCE_DIR}/${f}" ]; then
      args+=(--from-file="${f}=${EVIDENCE_DIR}/${f}")
    fi
  done
  kubectl -n "$NAMESPACE" create configmap agent-config \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

prepare_agent_github_token() {
  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -
}

prepare_agent_context() {
  native_azure_login
  install_preview_package
  copy_claude_ca

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP

  write_prompt_context
  prepare_agent_github_token
  prepare_agent_config_plan_impl
}

ensure_repo_for_resume() {
  if [ ! -d "${REPO_DIR}/.git" ]; then
    clone_repo
  fi
}

ensure_context_for_resume() {
  if [ -z "${PROXY_IP:-}" ]; then
    PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
    if [ -z "$PROXY_IP" ]; then
      echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
      return 1
    fi
    export PROXY_IP
  fi
  if ! kubectl -n "$NAMESPACE" get configmap agent-config >/dev/null 2>&1 \
      || ! kubectl -n "$NAMESPACE" get secret agent-github-token >/dev/null 2>&1; then
    ensure_repo_for_resume
    prepare_agent_context
  fi
}

# Apply + wait on a stage's agent Job pod. Streams claude events to the
# Glimmung step log via the existing log streamer (lib.sh strips the
# evidence-tar markers so the base64 stream doesn't bloat events).
run_stage_pod() {
  local job_name="$1"
  local stage="$2"
  local timeout="${3:-1800}"

  ensure_repo_for_resume
  ensure_context_for_resume
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli apply-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$job_name" \
      --issue-number "${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}" \
      --issue-title "$ISSUE_TITLE" \
      --issue-url "$ISSUE_URL" \
      --issue-reference "$ISSUE_REFERENCE" \
      --validation-url "$VALIDATION_URL" \
      --branch-name "$BRANCH_NAME" \
      --proxy-ip "$PROXY_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --repo-slug "$REPO_SLUG" \
      --stage "$stage"
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$job_name" \
      --timeout-seconds "$timeout"
  )
}

run_plan_and_implement() {
  run_stage_pod "$PLAN_JOB_NAME" "plan-and-implement" "${AGENT_PLAN_JOB_TIMEOUT_SECONDS:-2400}"
}

run_verification() {
  # Re-mount agent-config with the prior stages' handoffs before pod 2 starts.
  prepare_agent_config_verify
  run_stage_pod "$VERIFY_JOB_NAME" "verify" "${AGENT_VERIFY_JOB_TIMEOUT_SECONDS:-1800}"
}

# Capture pod logs + extract the base64 evidence tar emitted between the
# ===EVIDENCE-TAR-START===/END=== markers. Pod 1's tar contains the
# test-plan + implementation handoff JSON+MD; pod 2's contains the
# verification JSON+MD plus screenshots/.
collect_pod_evidence() {
  local job_name="$1"
  local log_path="$2"
  local pod=""
  pod="$(kubectl -n "$NAMESPACE" get pods -l "job-name=${job_name}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "$pod" ]; then
    echo "no pod found for job ${job_name}; skipping evidence capture"
    : >"$log_path"
    return 0
  fi
  kubectl -n "$NAMESPACE" logs "$pod" >"$log_path" || true
  echo "captured $(wc -l <"$log_path") log lines from ${pod}"

  if grep -q '===EVIDENCE-TAR-START===' "$log_path"; then
    if ! sed -n '/===EVIDENCE-TAR-START===/,/===EVIDENCE-TAR-END===/{//!p;}' \
        "$log_path" \
        | base64 -d 2>/tmp/extract.err \
        | tar -xzf - -C "$EVIDENCE_DIR" 2>>/tmp/extract.err; then
      echo "evidence tarball extraction failed for ${job_name}; continuing" >&2
      cat /tmp/extract.err >&2 || true
    fi
  else
    echo "no evidence tar markers found for ${job_name}"
  fi
}

collect_evidence() {
  collect_pod_evidence "$PLAN_JOB_NAME" "$STAGE1_LOG"
  if [ "$VERIFY_EXIT_CODE" -ne 99 ]; then
    # 99 = stage 2 was skipped because stage 1 failed. Otherwise extract
    # whatever the verify pod produced (even if claude inside it failed,
    # the bash tail still emits a tar with whatever JSON was written).
    collect_pod_evidence "$VERIFY_JOB_NAME" "$STAGE2_LOG"
  fi

  : >"$COMBINED_EVENTS"
  for f in "$STAGE1_LOG" "$STAGE2_LOG"; do
    [ -f "$f" ] || continue
    grep -E '^\{' "$f" >>"$COMBINED_EVENTS" || true
  done
  echo "combined $(wc -l <"$COMBINED_EVENTS") event lines from both stages"
}

# Push-branch step: pod 1 already pushed the branch on its own. We just
# confirm the branch is reachable on the remote. If it isn't, pod 1 must
# have failed before push — surface that.
push_branch() {
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  if git -c "http.extraHeader=${auth_header}" \
      ls-remote --exit-code "https://github.com/${REPO_SLUG}.git" "refs/heads/${BRANCH_NAME}" >/dev/null; then
    echo "branch ${BRANCH_NAME} is present on the remote"
    return 0
  fi
  echo "branch ${BRANCH_NAME} is absent — pod 1 did not complete the push step" >&2
  return 1
}

rebuild_validation_env() {
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

add_reason() {
  printf '%s\n' "$1" >>"$VERIFICATION_REASONS"
}

verification_cost() {
  if [ -s "$COMBINED_EVENTS" ]; then
    jq -r 'select(.type=="result") | .total_cost_usd // 0' "$COMBINED_EVENTS" \
      | awk '{s+=$1} END {if (NR>0) printf "%.4f", s; else printf "0"}'
  else
    printf '0'
  fi
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

# Prefer the verifier's own MD when present; fall back to the
# implementation MD; fall back to the test-plan MD; fall back to the
# last `result` event from the combined stream.
write_agent_summary() {
  for f in issue-agent-verification.md issue-agent-implementation.md issue-agent-test-plan.md; do
    if [ -s "${EVIDENCE_DIR}/${f}" ]; then
      cp "${EVIDENCE_DIR}/${f}" "$SUMMARY_MD"
      return 0
    fi
  done
  if [ -s "$COMBINED_EVENTS" ]; then
    jq -sr '
      map(
        select(.type == "result" and (.result? | type == "string") and (.result | length > 0))
        | .result
      )
      | last // empty
    ' "$COMBINED_EVENTS" >"$SUMMARY_MD" || true
  fi
}

# Walk the test plan's required_evidence and confirm every item has a
# matching evidence_results entry with status=pass. A verifier-claimed
# pass that misses any required item flips to fail.
enforce_evidence_contract() {
  local plan="${EVIDENCE_DIR}/issue-agent-test-plan.json"
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

verify_result() {
  if [ "$PLAN_EXIT_CODE" -ne 0 ]; then
    add_reason "plan-and-implement pod exited with ${PLAN_EXIT_CODE}; see native step logs"
    if [ -s "$STAGE1_LOG" ]; then
      grep -E "::error::|Job failed|FATAL|panic:|aborted:|forbidden|exited without writing" "$STAGE1_LOG" \
        | head -5 >>"$VERIFICATION_REASONS" || true
    fi
    write_verification "fail"
    return 0
  fi
  if [ "$VERIFY_EXIT_CODE" -ne 0 ]; then
    add_reason "verify pod exited with ${VERIFY_EXIT_CODE}; see native step logs"
    if [ -s "$STAGE2_LOG" ]; then
      grep -E "::error::|Job failed|FATAL|panic:|aborted:|forbidden|exited without writing" "$STAGE2_LOG" \
        | head -5 >>"$VERIFICATION_REASONS" || true
    fi
    write_verification "fail"
    return 0
  fi

  # Verifier's own claim
  local verifier_status
  verifier_status="$(jq -r '.status // "missing"' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo missing)"
  if [ "$verifier_status" != "pass" ]; then
    add_reason "verifier reported status=${verifier_status} reason=$(jq -r '.abort_reason // ""' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo "")"
    write_verification "fail"
    return 0
  fi

  # Wrapper-side recheck against the test plan's required_evidence.
  if ! enforce_evidence_contract; then
    write_verification "fail"
    return 0
  fi

  write_verification "pass"
}

upload_screenshots() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"
  local prefix staging total taken upload_ok

  if ! compgen -G "${EVIDENCE_DIR}/screenshots/*.png" >/dev/null; then
    return 0
  fi

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

emit_agent_outputs() {
  if [ ! -s "$VERIFICATION_JSON" ]; then
    add_reason "verification result was not produced"
    write_verification "error"
  fi
  cat "$VERIFICATION_JSON"
}

native_step "clone-repo" clone_repo
native_step "prepare-agent-context" prepare_agent_context
native_step_allow_failure "run-plan-and-implement" run_plan_and_implement || PLAN_EXIT_CODE=$?
if [ "$PLAN_EXIT_CODE" -ne 0 ]; then
  VERIFY_EXIT_CODE=99
  native_step_allow_failure "push-branch" /bin/false || true
  native_step_allow_failure "rebuild-validation" /bin/false || true
  native_step_allow_failure "run-verification" /bin/false || true
else
  native_step "push-branch" push_branch
  native_step "rebuild-validation" rebuild_validation_env
  native_step_allow_failure "run-verification" run_verification || VERIFY_EXIT_CODE=$?
fi
native_step "collect-evidence" collect_evidence
native_step "summarize-agent" write_agent_summary
native_step "verify-result" verify_result
native_step_allow_failure "upload-screenshots" upload_screenshots || true
native_step "emit-agent-outputs" emit_agent_outputs
native_assert_resume_satisfied

native_completed "null" "$(cat "$VERIFICATION_JSON")" "$(cat "$SCREENSHOTS_MD")" "$(cat "$SUMMARY_MD")"
