#!/usr/bin/env bash

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
BRANCH_NAME="glimmung/${GLIMMUNG_RUN_ID}"
JOB_NAME="agent-${IMAGE_TAG}"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
GITHUB_ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
if [ -n "$GITHUB_ISSUE_NUMBER" ] && [ -n "${GLIMMUNG_ISSUE_REPO:-}" ]; then
  ISSUE_REFERENCE="#${GITHUB_ISSUE_NUMBER}"
  ISSUE_URL="https://github.com/${GLIMMUNG_ISSUE_REPO}/issues/${GITHUB_ISSUE_NUMBER}"
else
  ISSUE_REFERENCE="${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_REFERENCE}"
fi

AGENT_EXIT_CODE=0
VERIFICATION_JSON="/tmp/verification.json"
VERIFICATION_REASONS="/tmp/verification-reasons.txt"
EVIDENCE_REFS="/tmp/evidence-refs.txt"
SCREENSHOTS_MD="/tmp/screenshots.md"
: >"$VERIFICATION_REASONS"
: >"$EVIDENCE_REFS"
: >"$SCREENSHOTS_MD"

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

write_agent_prompt() {
  local dest="/tmp/agent-prompt.md"
  : >"$dest"
  cat "${REPO_DIR}/.github/agent/prompt.md" >>"$dest"
  {
    echo ""
    echo "---"
    echo ""
    if [ -n "$GITHUB_ISSUE_NUMBER" ]; then
      echo "# Issue #${GITHUB_ISSUE_NUMBER}: ${ISSUE_TITLE}"
    else
      echo "# Glimmung issue ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
    fi
    echo "URL: ${ISSUE_URL}"
    echo "Validation env: ${VALIDATION_URL}"
    echo "Glimmung run: ${GLIMMUNG_RUN_ID}"
    echo "Glimmung attempt index: ${GLIMMUNG_ATTEMPT_INDEX:-unknown}"
    if [ -n "${GLIMMUNG_ISSUE_BODY:-}" ]; then
      echo ""
      echo "## Issue body"
      echo ""
      printf '%s\n' "$GLIMMUNG_ISSUE_BODY"
    fi
  } >>"$dest"
  echo "agent prompt size: $(wc -l <"$dest") lines, $(wc -c <"$dest") bytes"
}

prepare_agent_context() {
  local token
  native_azure_login
  install_preview_package
  copy_claude_ca

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP

  write_agent_prompt
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create configmap agent-config \
    --from-file=prompt.md=/tmp/agent-prompt.md \
    --dry-run=client -o yaml | kubectl apply -f -
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -
}

run_agent() {
  (
    cd "$REPO_DIR"
    args=(
      --namespace "$NAMESPACE"
      --job-name "$JOB_NAME"
      --issue-number "${GITHUB_ISSUE_NUMBER:-${ISSUE_REFERENCE}}"
      --issue-title "$ISSUE_TITLE"
      --issue-url "$ISSUE_URL"
      --issue-reference "$ISSUE_REFERENCE"
      --github-issue-number "$GITHUB_ISSUE_NUMBER"
      --validation-url "$VALIDATION_URL"
      --branch-name "$BRANCH_NAME"
      --proxy-ip "$PROXY_IP"
      --agent-container-tag "$AGENT_CONTAINER_TAG"
      --repo-slug "$REPO_SLUG"
    )
    python3 -m ambience_preview.cli apply-agent-job "${args[@]}"
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_JOB_TIMEOUT_SECONDS:-1800}"
  )
}

collect_evidence() {
  mkdir -p /tmp/evidence/screenshots
  local pod=""
  pod="$(kubectl -n "$NAMESPACE" get pods -l "job-name=${JOB_NAME}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "$pod" ]; then
    echo "no pod found for job ${JOB_NAME}; skipping event capture"
    : >/tmp/agent-pod.log
    : >/tmp/agent-events.jsonl
    return 0
  fi

  kubectl -n "$NAMESPACE" logs "$pod" >/tmp/agent-pod.log || true
  grep -E '^\{' /tmp/agent-pod.log >/tmp/agent-events.jsonl || true
  echo "captured $(wc -l </tmp/agent-events.jsonl) event lines from ${pod}"

  if grep -q '===EVIDENCE-TAR-START===' /tmp/agent-pod.log; then
    if ! sed -n '/===EVIDENCE-TAR-START===/,/===EVIDENCE-TAR-END===/{//!p;}' \
        /tmp/agent-pod.log \
        | base64 -d 2>/tmp/extract.err \
        | tar -xzf - -C /tmp/evidence 2>>/tmp/extract.err; then
      echo "evidence tarball extraction failed; continuing without in-pod evidence" >&2
      cat /tmp/extract.err >&2 || true
      rm -rf /tmp/evidence
      mkdir -p /tmp/evidence/screenshots
    fi
  else
    echo "no evidence tar markers found in agent pod log"
  fi
}

add_reason() {
  printf '%s\n' "$1" >>"$VERIFICATION_REASONS"
}

verification_cost() {
  if [ -s /tmp/agent-events.jsonl ]; then
    jq -r 'select(.type=="result") | .total_cost_usd // 0' /tmp/agent-events.jsonl \
      | awk '{s+=$1} END {if (NR>0) printf "%.4f", s; else printf "0"}'
  else
    printf '0'
  fi
}

write_verification() {
  local status="$1"
  local cost
  cost="$(verification_cost)"
  find /tmp/evidence/screenshots -maxdepth 1 -type f -name '*.png' \
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
      prompt_version: "ambience-native-v1",
      metadata: {
        run_id: $run_id,
        branch: $branch,
        validation_url: $validation_url
      }
    }' >"$VERIFICATION_JSON"
  cat "$VERIFICATION_JSON"
}

fetch_agent_branch() {
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  git -C "$REPO_DIR" \
    -c "http.extraHeader=${auth_header}" \
    fetch origin "refs/heads/${BRANCH_NAME}:refs/remotes/origin/${BRANCH_NAME}"
  git -C "$REPO_DIR" checkout -B "$BRANCH_NAME" "origin/${BRANCH_NAME}"
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

capture_screenshots() {
  if [ ! -f "${REPO_DIR}/screenshot-pages.json" ]; then
    echo "no screenshot-pages.json; skipping screenshot pass"
    return 0
  fi
  (
    cd "$REPO_DIR"
    mkdir -p /tmp/evidence/screenshots
    if [ -d /opt/ambience-agent/node_modules/playwright ]; then
      mkdir -p node_modules
      ln -sfn /opt/ambience-agent/node_modules/playwright node_modules/playwright
    else
      npm init -y >/dev/null
      npm install --no-save --silent playwright@1
      npx --yes playwright install chromium
    fi
    BASE_URL="$VALIDATION_URL" \
      PAGES_JSON=screenshot-pages.json \
      OUT_DIR=/tmp/evidence/screenshots \
      GLIMMUNG_RUN_ID="$GLIMMUNG_RUN_ID" \
      node scripts/screenshot-pages.mjs
  )
}

upload_screenshots() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"
  local prefix staging total taken upload_ok

  if ! compgen -G "/tmp/evidence/screenshots/*.png" >/dev/null; then
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
  done < <(find /tmp/evidence/screenshots -maxdepth 1 -type f -name '*.png' | sort)

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

verify_result() {
  if [ "$AGENT_EXIT_CODE" -ne 0 ]; then
    add_reason "agent job exited with ${AGENT_EXIT_CODE}; see native step logs"
    if [ -s /tmp/agent-pod.log ]; then
      grep -E "::error::|Job failed|FATAL|panic:|agent produced no changes" /tmp/agent-pod.log \
        | head -5 >>"$VERIFICATION_REASONS" || true
    fi
    write_verification "fail"
    return 0
  fi

  if ! fetch_agent_branch; then
    add_reason "agent completed but branch ${BRANCH_NAME} was not found"
    write_verification "fail"
    return 0
  fi

  if ! rebuild_validation_env; then
    add_reason "rebuilt validation environment failed to deploy"
    write_verification "fail"
    return 0
  fi

  if ! capture_screenshots; then
    add_reason "screenshot capture failed against ${VALIDATION_URL}"
    write_verification "fail"
    return 0
  fi

  upload_screenshots || true
  write_verification "pass"
}

push_branch() {
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  if git -c "http.extraHeader=${auth_header}" \
      ls-remote --exit-code "https://github.com/${REPO_SLUG}.git" "refs/heads/${BRANCH_NAME}" >/dev/null; then
    echo "branch ${BRANCH_NAME} is present"
    return 0
  fi
  if jq -e '.status == "pass"' "$VERIFICATION_JSON" >/dev/null 2>&1; then
    echo "verification passed but branch ${BRANCH_NAME} is missing" >&2
    return 1
  fi
  echo "branch ${BRANCH_NAME} is absent after failed verification"
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
native_step_allow_failure "run-agent" run_agent || AGENT_EXIT_CODE=$?
native_step "collect-evidence" collect_evidence
native_step "verify-result" verify_result
native_step "push-branch" push_branch
native_step "emit-agent-outputs" emit_agent_outputs
native_assert_resume_satisfied

native_completed "null" "$(cat "$VERIFICATION_JSON")" "$(cat "$SCREENSHOTS_MD")"
