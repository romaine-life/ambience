#!/usr/bin/env bash
# Shared functions for the glimmung-native agent phase scripts.
# Source this file after native_init and after setting the common vars
# (NAMESPACE, REPO_DIR, REPO_SLUG, VALIDATION_URL, BRANCH_NAME, etc.).

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
  echo "agent prompt context: $(wc -l <"$dest") lines, $(wc -c <"$dest") bytes"
}

prepare_agent_github_token() {
  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -
}

ensure_repo_for_resume() {
  if [ ! -d "${REPO_DIR}/.git" ]; then
    native_clone_repo "$REPO_SLUG" "$REPO_DIR" main "$BRANCH_NAME"
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
    prepare_common_context
  fi
}

# Called by each phase's prepare step to set up shared k8s resources.
# The caller is responsible for creating the agent-config configmap
# with the right prompt files BEFORE calling run_stage_pod.
prepare_common_context() {
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
}

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
    # Emit the inner-Job registration marker so the outer glimmung
    # runner records this child Job alongside its own. verify is the
    # verification_agent; everything else is a helper.
    local marker_intent="helper"
    if [ "$stage" = "verify" ]; then
      marker_intent="verification_agent"
    fi
    native_emit_inner_job_marker "$NAMESPACE" "$job_name" "$marker_intent" "${stage}-agent"
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$job_name" \
      --timeout-seconds "$timeout"
  )
}

# Extract the base64-encoded evidence tar emitted by a stage pod between
# ===EVIDENCE-TAR-START=== / ===EVIDENCE-TAR-END=== markers in its logs.
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

rebuild_validation_env() {
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

upload_screenshots() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"
  local prefix staging total taken upload_ok screenshots_md_out="${1:-/tmp/screenshots.md}"

  : >"$screenshots_md_out"

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
  } >"$screenshots_md_out"
  rm -rf "$staging"
}
