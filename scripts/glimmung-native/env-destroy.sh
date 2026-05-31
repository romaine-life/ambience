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
    "emit" emit
  exit $?
fi

native_step "describe-pre-teardown" describe_pre_teardown
native_step_allow_failure "uninstall-helm-release" uninstall_helm_release
native_step_allow_failure "delete-namespace" delete_namespace
native_assert_resume_satisfied

emit
