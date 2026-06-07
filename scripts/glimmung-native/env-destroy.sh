#!/usr/bin/env bash

# Always-run teardown phase (glimmung#296). Runs after the verification gate
# regardless of how that gate resolved — success, abort, or fail —
# so a failed verify-loop no longer leaves its slot namespace claiming
# the public hostname for the next 24h. Idempotent: missing
# helm release or namespace is fine, we log and move on.
#
# The env-prep slot reap (ambience#224) stays in place as belt-and-
# suspenders for the case where this teardown itself fails — both
# can run; the reap is cheap when there's nothing to reap.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env GLIMMUNG_VALIDATION_NAMESPACE GLIMMUNG_RUN_ID

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
NAMESPACE="${GLIMMUNG_VALIDATION_NAMESPACE}"
VALIDATION_SLOT_INDEX="${GLIMMUNG_NATIVE_SLOT_INDEX:-}"
PREPROVISIONED_SLOT=""
if [ -n "$VALIDATION_SLOT_INDEX" ]; then
  PREPROVISIONED_SLOT="1"
fi
if [ -n "$PREPROVISIONED_SLOT" ]; then
  RELEASE_NAME="${AMBIENCE_VALIDATION_RELEASE:-${NAMESPACE}-hot}"
else
  RELEASE_NAME="${AMBIENCE_VALIDATION_RELEASE:-ambience-agent}"
fi

describe_pre_teardown() {
  echo "namespace: ${NAMESPACE}"
  echo "release:   ${RELEASE_NAME}"
  echo "--- helm releases in namespace ---"
  helm list --namespace "$NAMESPACE" 2>&1 || echo "(helm list failed, namespace may already be gone)"
  echo "--- httproutes in namespace ---"
  kubectl get httproute --namespace "$NAMESPACE" 2>&1 || true
  echo "--- pods in namespace ---"
  kubectl get pods --namespace "$NAMESPACE" 2>&1 || true
}

uninstall_helm_release() {
  if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
    echo "namespace ${NAMESPACE} already gone; nothing to uninstall"
    return 0
  fi
  if ! helm status "$RELEASE_NAME" --namespace "$NAMESPACE" >/dev/null 2>&1; then
    echo "helm release ${RELEASE_NAME} not found in ${NAMESPACE}; nothing to uninstall"
    return 0
  fi
  # --wait=false: namespace deletion will reap the rest. The HTTPRoute
  # delete that releases the slot hostname is what callers care about
  # most, and `helm uninstall` deletes that synchronously.
  helm uninstall "$RELEASE_NAME" --namespace "$NAMESPACE" --wait=false
}

delete_namespace() {
  if [ -n "$PREPROVISIONED_SLOT" ]; then
    echo "pre-provisioned slot namespace ${NAMESPACE}; leaving warm resources in place"
    return 0
  fi
  kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --wait=false
}

github_api() {
  local method="$1"
  local path="$2"
  local token
  token="$(native_github_token)"
  curl -fsS \
    -X "$method" \
    -H "Authorization: Bearer ${token}" \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com${path}"
}

branch_has_merged_pr() {
  local branch="$1"
  local owner repo head_query pulls
  owner="${REPO_SLUG%%/*}"
  repo="${REPO_SLUG#*/}"
  head_query="$(jq -nr --arg head "${owner}:${branch}" '$head | @uri')"
  pulls="$(github_api GET "/repos/${owner}/${repo}/pulls?head=${head_query}&state=all&per_page=20")"
  jq -e 'any(.[]?; .merged_at != null)' >/dev/null <<<"$pulls"
}

cleanup_issue_branches() {
  local prefix token auth_header remote branches branch merged_seen
  if [ "${GLIMMUNG_PHASE:-}" != "cleanup_final" ]; then
    echo "phase ${GLIMMUNG_PHASE:-unknown} is not cleanup_final; skipping issue branch cleanup"
    return 0
  fi
  if ! prefix="$(native_issue_branch_prefix)"; then
    echo "GLIMMUNG_ISSUE_NUMBER is not set; skipping issue branch cleanup"
    return 0
  fi

  token="$(native_github_token)"
  auth_header="$(native_git_auth_header "$token")"
  remote="https://github.com/${REPO_SLUG}.git"
  mapfile -t branches < <(
    git -c "http.extraHeader=${auth_header}" \
      ls-remote --heads "$remote" "refs/heads/${prefix}*" \
      | awk '{sub(/^refs\/heads\//, "", $2); print $2}'
  )

  if [ "${#branches[@]}" -eq 0 ]; then
    echo "no issue branches found for ${prefix}"
    return 0
  fi

  merged_seen=""
  for branch in "${branches[@]}"; do
    if branch_has_merged_pr "$branch"; then
      merged_seen="1"
      break
    fi
  done
  if [ -z "$merged_seen" ]; then
    echo "no merged PR found under ${prefix}; refusing to delete implementation branches"
    return 0
  fi

  echo "deleting ${#branches[@]} issue branch(es) under ${prefix}"
  for branch in "${branches[@]}"; do
    echo "deleting ${branch}"
    git -c "http.extraHeader=${auth_header}" push "$remote" --delete "$branch"
  done
}

uninstall_helm_release_allow_failure() {
  uninstall_helm_release || true
}

delete_namespace_allow_failure() {
  delete_namespace || true
}

emit() {
  native_completed "{}"
}

if native_selected_step; then
  native_run_selected_step \
    "describe-pre-teardown" describe_pre_teardown \
    "uninstall-helm-release" uninstall_helm_release_allow_failure \
    "delete-namespace" delete_namespace_allow_failure \
    "cleanup-issue-branches" cleanup_issue_branches \
    "emit" emit
  exit $?
fi

native_step "describe-pre-teardown" describe_pre_teardown
native_step_allow_failure "uninstall-helm-release" uninstall_helm_release
native_step_allow_failure "delete-namespace" delete_namespace
native_step_allow_failure "cleanup-issue-branches" cleanup_issue_branches
native_assert_resume_satisfied

emit
