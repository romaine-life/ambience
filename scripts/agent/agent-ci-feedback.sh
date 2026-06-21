#!/usr/bin/env bash
#
# Agent-side helper: publish the implementation agent's current branch and show
# the deterministic CI result for that exact commit. Runs INSIDE the
# implementation agent inner Job (rendered by mcp/ambience_preview), not in the
# Glimmung runner — the runner harness is the Go module under glimmung-harness/.
#
# Self-contained by design: it reads the repo-scoped GitHub token the agent Job
# mounts (GITHUB_TOKEN_FILE) and the BRANCH_NAME the Job exports. It does NOT
# depend on the retired scripts/glimmung-native/lib.sh fork.

set -Eeuo pipefail

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/repo}"
BRANCH_NAME="${BRANCH_NAME:-}"
BASE_REF="${AMBIENCE_PR_BASE:-main}"
REMOTE_URL="${AMBIENCE_GIT_REMOTE_URL:-https://github.com/${REPO_SLUG}.git}"

if [ -z "$BRANCH_NAME" ]; then
  echo "BRANCH_NAME is required for agent CI feedback" >&2
  exit 2
fi

github_api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local token
  # The implementation agent job mounts the repo-scoped token as a file
  # (GITHUB_TOKEN_FILE) — the only GitHub credential the agent gets. There is
  # no fallback path.
  if [ -z "${GITHUB_TOKEN_FILE:-}" ] || [ ! -s "${GITHUB_TOKEN_FILE}" ]; then
    echo "GITHUB_TOKEN_FILE is required for agent CI feedback" >&2
    return 1
  fi
  token="$(cat "${GITHUB_TOKEN_FILE}")"
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
  blocked="$(git status --porcelain -- .github/workflows .github/agent .mcp.json glimmung-harness 2>/dev/null || true)"
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
  git commit -m "agent: address ${GLIMMUNG_ISSUE_NUMBER:-${GLIMMUNG_RUN_ID:-update}}"
}

push_branch() {
  local remote_ref remote_sha
  remote_ref="refs/heads/${BRANCH_NAME}"
  git -C "$REPO_DIR" remote set-url origin "$REMOTE_URL"
  git -C "$REPO_DIR" fetch origin "$BASE_REF"
  if git -C "$REPO_DIR" fetch origin "+${remote_ref}:refs/remotes/origin/${BRANCH_NAME}" >/dev/null 2>&1; then
    remote_sha="$(git -C "$REPO_DIR" rev-parse "origin/${BRANCH_NAME}")"
  else
    remote_sha=""
  fi
  git -C "$REPO_DIR" rebase "origin/${BASE_REF}"
  if [ -n "$remote_sha" ]; then
    git -C "$REPO_DIR" push \
      --force-with-lease="${remote_ref}:${remote_sha}" \
      origin "HEAD:${BRANCH_NAME}"
  else
    git -C "$REPO_DIR" push origin "HEAD:${BRANCH_NAME}"
  fi
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
    wait_checks
    ;;
  *)
    echo "usage: $0 {publish|status|wait|publish-and-wait}" >&2
    exit 2
    ;;
esac
