#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env GLIMMUNG_VALIDATION_NAMESPACE GLIMMUNG_RUN_ID

REPO_SLUG="${AMBIENCE_REPO_SLUG:-nelsong6/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
CLAUDE_NAMESPACE="${CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}"

NAMESPACE="${GLIMMUNG_VALIDATION_NAMESPACE}"
VALIDATION_SLOT_INDEX="${GLIMMUNG_NATIVE_SLOT_INDEX:-}"
PREPROVISIONED_PUBLIC_HOST=""
if [ -n "$VALIDATION_SLOT_INDEX" ]; then
  VALIDATION_HOST="${AMBIENCE_STANDBY_HOST_PREFIX:-ambience-slot-}${VALIDATION_SLOT_INDEX}.ambience.dev.romaine.life"
  PREPROVISIONED_PUBLIC_HOST="1"
else
  VALIDATION_HOST="${NAMESPACE}.ambience.dev.romaine.life"
fi
if [ -n "$PREPROVISIONED_PUBLIC_HOST" ]; then
  RELEASE_NAME="${AMBIENCE_VALIDATION_RELEASE:-${NAMESPACE}-hot}"
else
  RELEASE_NAME="${AMBIENCE_VALIDATION_RELEASE:-ambience-agent}"
fi
VALIDATION_URL="https://${VALIDATION_HOST}"
IMAGE_TAG="${NAMESPACE}"
IMAGE_JSON="/tmp/ambience-preview-image.json"
IMAGE_FILE="/tmp/ambience-preview-image.txt"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main
}

install_preview_package() {
  native_install_preview_package "${REPO_DIR}/mcp"
}

build_validation_image() {
  native_azure_login
  install_preview_package
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli build-preview-image --image-tag "$IMAGE_TAG" >"$IMAGE_JSON"
  )
  jq -r '.image' "$IMAGE_JSON" >"$IMAGE_FILE"
}

push_validation_image() {
  local tag
  tag="$(
    az acr repository show-tags \
      --name romainecr \
      --repository ambience \
      --query "[?@=='${IMAGE_TAG}'] | [0]" \
      --output tsv
  )"
  if [ "$tag" != "$IMAGE_TAG" ]; then
    echo "image tag ${IMAGE_TAG} was not found in romainecr.azurecr.io/ambience" >&2
    return 1
  fi
  echo "verified romainecr.azurecr.io/ambience:${IMAGE_TAG}"
}

# Older aborted runs could leak their glim-run-* namespace before teardown
# existed. Multiple leaked namespaces all claiming the same ambience-slot-N
# hostname leave Envoy Gateway picking the oldest by creationTimestamp, so a
# fresh verify-loop can be routed at a stale image. Defensively reap any peer
# namespace whose HTTPRoute claims our slot host before we install ours.
reap_conflicting_slot() {
  if [ -z "$VALIDATION_SLOT_INDEX" ]; then
    echo "no slot index set; nothing to reap"
    return 0
  fi
  local conflicts
  conflicts="$(
    kubectl get httproute --all-namespaces -o json \
      | jq -r --arg host "$VALIDATION_HOST" --arg ns "$NAMESPACE" '
          .items[]
          | select(.spec.hostnames | index($host))
          | select(.metadata.namespace != $ns)
          | "\(.metadata.namespace)\t\(.metadata.name)"
        '
  )"
  if [ -z "$conflicts" ]; then
    echo "no peer claims ${VALIDATION_HOST}"
    return 0
  fi
  while IFS=$'\t' read -r conflict_ns conflict_name; do
    [ -n "$conflict_ns" ] || continue
    case "$conflict_ns" in
      glim-run-*) ;;
      *)
        echo "skipping reap of ${conflict_ns}/${conflict_name}: not a glim-run-* namespace" >&2
        continue
        ;;
    esac
    echo "reaping ${VALIDATION_HOST} claimant ${conflict_ns}/${conflict_name}"
    # Delete the HTTPRoute first so Envoy reroutes before we proceed, then
    # tear down the helm release and namespace asynchronously.
    kubectl delete httproute "$conflict_name" --namespace "$conflict_ns" --ignore-not-found=true >/dev/null 2>&1 || true
    helm uninstall "$RELEASE_NAME" --namespace "$conflict_ns" >/dev/null 2>&1 || true
    kubectl delete namespace "$conflict_ns" --ignore-not-found=true --wait=false >/dev/null 2>&1 || true
  done <<<"$conflicts"
}

deploy_validation_env() {
  local image
  local -a args
  image="$(cat "$IMAGE_FILE")"
  args=(
    deploy-validation-preview
    --namespace "$NAMESPACE"
    --image "$image"
    --release "$RELEASE_NAME"
    --public-host "$VALIDATION_HOST"
    --no-create-namespace
  )
  if [ -n "$PREPROVISIONED_PUBLIC_HOST" ]; then
    args+=(--skip-external-dns)
    args+=(--render-mode hot --test-env-slot-name "$NAMESPACE")
  fi
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli "${args[@]}"
  )
}

check_validation_env() {
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli wait-public-preview --url "$VALIDATION_URL"
  )
}

emit_env_outputs() {
  jq -nc \
    --arg validation_url "$VALIDATION_URL" \
    --arg validation_slot_index "$VALIDATION_SLOT_INDEX" \
    --arg namespace "$NAMESPACE" \
    --arg image_tag "$IMAGE_TAG" \
    --arg claude_namespace "$CLAUDE_NAMESPACE" \
    --arg claude_ca_namespace "$CLAUDE_CA_NAMESPACE" \
    '{
      validation_url: $validation_url,
      validation_slot_index: $validation_slot_index,
      namespace: $namespace,
      image_tag: $image_tag,
      claude_namespace: $claude_namespace,
      claude_ca_namespace: $claude_ca_namespace
    }' >/tmp/ambience-env-outputs.json
  cat /tmp/ambience-env-outputs.json
}

if native_selected_step; then
  native_run_selected_step \
    "clone-repo" clone_repo \
    "build-validation-image" build_validation_image \
    "push-validation-image" push_validation_image \
    "reap-slot-conflicts" reap_conflicting_slot \
    "deploy-validation-env" deploy_validation_env \
    "check-validation-env" check_validation_env \
    "emit-env-outputs" emit_env_outputs
  exit $?
fi

native_step "clone-repo" clone_repo
native_step "build-validation-image" build_validation_image
native_step "push-validation-image" push_validation_image
native_step "reap-slot-conflicts" reap_conflicting_slot
native_step "deploy-validation-env" deploy_validation_env
native_step "check-validation-env" check_validation_env
native_step "emit-env-outputs" emit_env_outputs
native_assert_resume_satisfied

native_completed "$(cat /tmp/ambience-env-outputs.json)"
