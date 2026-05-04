#!/usr/bin/env bash

set -Eeuo pipefail

native_require_env() {
  local missing=()
  for name in "$@"; do
    if [ -z "${!name:-}" ]; then
      missing+=("$name")
    fi
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    printf 'missing required env: %s\n' "${missing[*]}" >&2
    exit 2
  fi
}

native_init() {
  native_require_env \
    GLIMMUNG_ATTEMPT_TOKEN \
    GLIMMUNG_EVENTS_URL \
    GLIMMUNG_COMPLETED_URL \
    GLIMMUNG_FAILED_URL \
    GLIMMUNG_GITHUB_TOKEN_URL \
    GLIMMUNG_JOB_ID \
    GLIMMUNG_RUN_ID

  NATIVE_SEQ_FILE="${NATIVE_SEQ_FILE:-/tmp/glimmung-native-seq}"
  if [ ! -f "$NATIVE_SEQ_FILE" ]; then
    printf '0\n' >"$NATIVE_SEQ_FILE"
  fi
  NATIVE_RESUME_FROM_JOB="${GLIMMUNG_ENTRYPOINT_JOB_ID:-}"
  NATIVE_RESUME_FROM_STEP="${GLIMMUNG_RESUME_FROM_STEP:-${GLIMMUNG_ENTRYPOINT_STEP_SLUG:-}}"
  NATIVE_RESUME_SEEN=""
  if [ -z "$NATIVE_RESUME_FROM_JOB" ] && [ -z "$NATIVE_RESUME_FROM_STEP" ]; then
    NATIVE_RESUME_SEEN="1"
  elif [ -n "$NATIVE_RESUME_FROM_JOB" ] && [ "$NATIVE_RESUME_FROM_JOB" != "$GLIMMUNG_JOB_ID" ]; then
    NATIVE_RESUME_SEEN="skip-job"
  elif [ -z "$NATIVE_RESUME_FROM_STEP" ]; then
    NATIVE_RESUME_SEEN="1"
  fi
}

native_next_seq() {
  local current next
  current="$(cat "$NATIVE_SEQ_FILE" 2>/dev/null || printf '0')"
  next=$((current + 1))
  printf '%s\n' "$next" >"$NATIVE_SEQ_FILE"
  printf '%s' "$next"
}

native_post_json() {
  local url="$1"
  local payload="$2"
  curl -fsS \
    --retry 5 \
    --retry-delay 1 \
    --retry-all-errors \
    -H "Content-Type: application/json" \
    -H "X-Glimmung-Attempt-Token: ${GLIMMUNG_ATTEMPT_TOKEN}" \
    -d "$payload" \
    "$url" >/dev/null
}

native_event() {
  local event="$1"
  local step_slug="${2:-}"
  local message="${3:-}"
  local exit_code="${4:-}"
  local metadata_json="${5:-{}}"
  local seq exit_json payload
  seq="$(native_next_seq)"
  if [ -n "$exit_code" ]; then
    exit_json="$exit_code"
  else
    exit_json="null"
  fi
  payload="$(
    jq -nc \
      --arg job_id "$GLIMMUNG_JOB_ID" \
      --argjson seq "$seq" \
      --arg event "$event" \
      --arg step_slug "$step_slug" \
      --arg message "$message" \
      --argjson exit_code "$exit_json" \
      --argjson metadata "$metadata_json" \
      '{
        job_id: $job_id,
        seq: $seq,
        event: $event,
        metadata: $metadata
      }
      + (if $step_slug != "" then {step_slug: $step_slug} else {} end)
      + (if $message != "" then {message: $message} else {} end)
      + (if $exit_code != null then {exit_code: $exit_code} else {} end)'
  )"
  native_post_json "$GLIMMUNG_EVENTS_URL" "$payload"
}

native_log() {
  local step_slug="$1"
  local message="$2"
  if [ -n "$message" ]; then
    native_event "log" "$step_slug" "$message"
  fi
}

native_log_file() {
  local step_slug="$1"
  local file="$2"
  local chunk_dir part
  if [ ! -s "$file" ]; then
    return 0
  fi
  chunk_dir="$(mktemp -d)"
  split -b 12000 "$file" "${chunk_dir}/chunk-"
  for part in "${chunk_dir}"/chunk-*; do
    [ -f "$part" ] || continue
    native_log "$step_slug" "$(cat "$part")"
  done
  rm -rf "$chunk_dir"
}

native_should_skip_step() {
  local step_slug="$1"
  if [ "$NATIVE_RESUME_SEEN" = "skip-job" ]; then
    return 0
  fi
  if [ -n "$NATIVE_RESUME_SEEN" ]; then
    return 1
  fi
  if [ "$step_slug" = "$NATIVE_RESUME_FROM_STEP" ]; then
    NATIVE_RESUME_SEEN="1"
    return 1
  fi
  return 0
}

native_step() {
  local step_slug="$1"
  shift
  native_step_run "$step_slug" "fatal" "$@"
}

native_step_allow_failure() {
  local step_slug="$1"
  shift
  native_step_run "$step_slug" "allow" "$@"
}

native_step_run() {
  local step_slug="$1"
  local failure_mode="$2"
  shift 2
  local log_file rc metadata

  if native_should_skip_step "$step_slug"; then
    metadata="$(
      jq -nc \
        --arg resume_from_job "$NATIVE_RESUME_FROM_JOB" \
        --arg resume_from_step "$NATIVE_RESUME_FROM_STEP" \
        '{resume_from_job: $resume_from_job, resume_from_step: $resume_from_step}'
    )"
    native_event "step_skipped" "$step_slug" "skipped before resume target ${NATIVE_RESUME_FROM_STEP:-${NATIVE_RESUME_FROM_JOB}}" "" "$metadata"
    return 0
  fi

  native_event "step_started" "$step_slug"
  log_file="$(mktemp)"
  set +e
  "$@" >"$log_file" 2>&1
  rc=$?
  set -e
  cat "$log_file"
  native_log_file "$step_slug" "$log_file"
  rm -f "$log_file"

  if [ "$rc" -eq 0 ]; then
    native_event "step_completed" "$step_slug" "" "0"
    return 0
  fi

  native_event "step_failed" "$step_slug" "step exited ${rc}" "$rc"
  if [ "$failure_mode" = "fatal" ]; then
    native_failed "step ${step_slug} exited ${rc}"
    exit "$rc"
  fi
  return "$rc"
}

native_assert_resume_satisfied() {
  if [ "$NATIVE_RESUME_SEEN" = "skip-job" ]; then
    return 0
  fi
  if [ -n "$NATIVE_RESUME_FROM_STEP" ] && [ -z "$NATIVE_RESUME_SEEN" ]; then
    native_failed "unknown resume step ${NATIVE_RESUME_FROM_STEP}"
    exit 2
  fi
}

native_completed() {
  local outputs_json="${1:-null}"
  local verification_json="${2:-null}"
  local screenshots_markdown="${3:-}"
  local payload
  payload="$(
    jq -nc \
      --arg conclusion "success" \
      --argjson outputs "$outputs_json" \
      --argjson verification "$verification_json" \
      --arg screenshots "$screenshots_markdown" \
      '{
        conclusion: $conclusion
      }
      + (if $outputs != null then {outputs: $outputs} else {} end)
      + (if $verification != null then {verification: $verification} else {} end)
      + (if $screenshots != "" then {screenshots_markdown: $screenshots} else {} end)'
  )"
  native_post_json "$GLIMMUNG_COMPLETED_URL" "$payload"
}

native_failed() {
  local reason="$1"
  local payload
  payload="$(jq -nc --arg reason "$reason" '{reason: $reason}')"
  native_post_json "$GLIMMUNG_FAILED_URL" "$payload" || true
}

native_azure_login() {
  if [ -z "${AZURE_CLIENT_ID:-}" ] || [ -z "${AZURE_TENANT_ID:-}" ] || [ -z "${AZURE_FEDERATED_TOKEN_FILE:-}" ]; then
    echo "Azure workload identity env is missing; cannot login" >&2
    return 1
  fi
  az login \
    --service-principal \
    --username "$AZURE_CLIENT_ID" \
    --tenant "$AZURE_TENANT_ID" \
    --federated-token "$(cat "$AZURE_FEDERATED_TOKEN_FILE")" \
    --allow-no-subscriptions >/dev/null
  if [ -n "${AZURE_SUBSCRIPTION_ID:-${ARM_SUBSCRIPTION_ID:-}}" ]; then
    az account set --subscription "${AZURE_SUBSCRIPTION_ID:-${ARM_SUBSCRIPTION_ID}}"
  fi
}

native_github_token() {
  curl -fsS \
    --retry 5 \
    --retry-delay 1 \
    --retry-all-errors \
    -X POST \
    -H "X-Glimmung-Attempt-Token: ${GLIMMUNG_ATTEMPT_TOKEN}" \
    "$GLIMMUNG_GITHUB_TOKEN_URL" \
    | jq -r '.token'
}

native_git_auth_header() {
  local token="$1"
  local encoded
  encoded="$(printf 'x-access-token:%s' "$token" | base64 | tr -d '\n')"
  printf 'Authorization: Basic %s' "$encoded"
}

native_clone_repo() {
  local repo_slug="$1"
  local repo_dir="$2"
  local base_ref="${3:-main}"
  local branch_name="${4:-}"
  local token auth_header

  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  mkdir -p "$(dirname "$repo_dir")"
  if [ ! -d "${repo_dir}/.git" ]; then
    git init "$repo_dir" >/dev/null
    git -C "$repo_dir" remote add origin "https://github.com/${repo_slug}.git"
  fi

  git -C "$repo_dir" remote set-url origin "https://github.com/${repo_slug}.git"
  git -C "$repo_dir" \
    -c "http.extraHeader=${auth_header}" \
    fetch --force origin "+refs/heads/*:refs/remotes/origin/*"
  git -C "$repo_dir" config user.name "ambience-agent[bot]"
  git -C "$repo_dir" config user.email "ambience-agent@romaine.life"

  if [ -n "$branch_name" ]; then
    git -C "$repo_dir" checkout -B "$branch_name" "origin/${base_ref}"
  else
    git -C "$repo_dir" checkout --detach "origin/${base_ref}"
  fi
}

native_push_branch() {
  local repo_slug="$1"
  local repo_dir="$2"
  local branch_name="$3"
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  git -C "$repo_dir" remote set-url origin "https://github.com/${repo_slug}.git"
  git -C "$repo_dir" \
    -c "http.extraHeader=${auth_header}" \
    push origin "HEAD:${branch_name}"
}
