#!/usr/bin/env bash
#
# Publish the managed implementation agent's current branch and show the
# deterministic CI result for that exact commit.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/repo}"
BRANCH_NAME="${GLIMMUNG_WORK_CONTEXT_BRANCH:-glimmung/${GLIMMUNG_RUN_ID:?GLIMMUNG_RUN_ID required}}"
BASE_REF="${AMBIENCE_PR_BASE:-main}"
WORKFLOW_FILE="${AMBIENCE_PR_CHECK_WORKFLOW:-docker-build-check.yaml}"

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

assert_branch() {
  local current
  current="$(git -C "$REPO_DIR" branch --show-current)"
  if [ "$current" != "$BRANCH_NAME" ]; then
    echo "refusing to publish from branch ${current}; expected ${BRANCH_NAME}" >&2
    return 1
  fi
}

commit_if_needed() {
  cd "$REPO_DIR"
  local blocked
  blocked="$(git status --porcelain -- .github/workflows .github/agent .mcp.json 2>/dev/null || true)"
  if [ -n "$blocked" ]; then
    echo "runner-local config changes are not publishable from the implementation agent:" >&2
    echo "$blocked" >&2
    return 1
  fi
  git add -A
  if git diff --cached --quiet; then
    echo "no staged changes to commit"
    return 0
  fi
  git commit -m "agent: address ${GLIMMUNG_ISSUE_NUMBER:-${GLIMMUNG_RUN_ID}}"
}

push_branch() {
  local token auth_header
  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  git -C "$REPO_DIR" remote set-url origin "https://github.com/${REPO_SLUG}.git"
  git -C "$REPO_DIR" -c "http.extraHeader=${auth_header}" fetch origin "$BASE_REF"
  git -C "$REPO_DIR" rebase "origin/${BASE_REF}"
  git -C "$REPO_DIR" -c "http.extraHeader=${auth_header}" push origin "HEAD:${BRANCH_NAME}"
}

dispatch_checks() {
  local owner repo workflow_path body
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  workflow_path="$(printf '%s' "$WORKFLOW_FILE" | jq -sRr @uri)"
  body="$(jq -nc --arg ref "$BRANCH_NAME" '{ref:$ref, inputs:{git_ref:$ref}}')"
  github_api POST "/repos/${owner}/${repo}/actions/workflows/${workflow_path}/dispatches" "$body" >/dev/null
  echo "dispatched ${WORKFLOW_FILE} for ${BRANCH_NAME}"
}

print_checks() {
  local owner repo sha
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  github_api GET "/repos/${owner}/${repo}/commits/${sha}/check-runs?per_page=100" \
    | jq -r '
        if (.check_runs // [] | length) == 0 then
          "no check runs recorded yet"
        else
          (.check_runs // [])[]
          | "- " + (.name // "check") + ": " + (.status // "unknown") + (if .conclusion then " / " + .conclusion else "" end) + " " + (.html_url // "")
        end
      '
}

wait_checks() {
  local owner repo sha deadline now checks_json pending failing
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  deadline="$(($(date +%s) + ${AMBIENCE_AGENT_CI_TIMEOUT_SECONDS:-1800}))"
  while true; do
    checks_json="$(github_api GET "/repos/${owner}/${repo}/commits/${sha}/check-runs?per_page=100")"
    pending="$(printf '%s' "$checks_json" | jq -r '(.check_runs // []) | map(select(.status != "completed")) | length')"
    failing="$(printf '%s' "$checks_json" | jq -r '
      (.check_runs // [])
      | map(. as $run | select($run.status == "completed" and ((["success", "neutral", "skipped"] | index($run.conclusion // "")) | not)))
      | length
    ')"
    print_checks
    if [ "$failing" -gt 0 ]; then
      return 1
    fi
    if [ "$(printf '%s' "$checks_json" | jq -r '(.check_runs // []) | length')" -gt 0 ] && [ "$pending" -eq 0 ]; then
      return 0
    fi
    now="$(date +%s)"
    if [ "$now" -ge "$deadline" ]; then
      echo "timed out waiting for CI checks" >&2
      return 1
    fi
    sleep "${AMBIENCE_AGENT_CI_POLL_SECONDS:-20}"
  done
}

case "${1:-status}" in
  publish)
    assert_branch
    commit_if_needed
    push_branch
    ;;
  dispatch)
    dispatch_checks
    ;;
  status)
    print_checks
    ;;
  wait)
    wait_checks
    ;;
  publish-and-wait)
    assert_branch
    commit_if_needed
    push_branch
    dispatch_checks
    wait_checks
    ;;
  *)
    echo "usage: $0 {publish|dispatch|status|wait|publish-and-wait}" >&2
    exit 2
    ;;
esac
