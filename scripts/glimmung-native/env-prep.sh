#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env GLIMMUNG_VALIDATION_NAMESPACE GLIMMUNG_RUN_ID

REPO_SLUG="${AMBIENCE_REPO_SLUG:-nelsong6/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
RELEASE_NAME="${AMBIENCE_VALIDATION_RELEASE:-ambience-agent}"
CLAUDE_NAMESPACE="${CLAUDE_NAMESPACE:-tank-operator}"

NAMESPACE="${GLIMMUNG_VALIDATION_NAMESPACE}"
VALIDATION_HOST="${NAMESPACE}.ambience.dev.romaine.life"
VALIDATION_URL="https://${VALIDATION_HOST}"
IMAGE_TAG="${NAMESPACE}"
IMAGE_JSON="/tmp/ambience-preview-image.json"
IMAGE_FILE="/tmp/ambience-preview-image.txt"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main
}

install_preview_package() {
  python3 -m pip install --user --upgrade pip
  python3 -m pip install --user "${REPO_DIR}/mcp"
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

deploy_validation_env() {
  local image
  image="$(cat "$IMAGE_FILE")"
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli deploy-validation-preview \
      --namespace "$NAMESPACE" \
      --image "$image" \
      --release "$RELEASE_NAME" \
      --public-host "$VALIDATION_HOST" \
      --no-create-namespace
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
    --arg namespace "$NAMESPACE" \
    --arg image_tag "$IMAGE_TAG" \
    --arg claude_namespace "$CLAUDE_NAMESPACE" \
    '{
      validation_url: $validation_url,
      namespace: $namespace,
      image_tag: $image_tag,
      claude_namespace: $claude_namespace
    }' >/tmp/ambience-env-outputs.json
  cat /tmp/ambience-env-outputs.json
}

native_step "clone-repo" clone_repo
native_step "build-validation-image" build_validation_image
native_step "push-validation-image" push_validation_image
native_step "deploy-validation-env" deploy_validation_env
native_step "check-validation-env" check_validation_env
native_step "emit-env-outputs" emit_env_outputs
native_assert_resume_satisfied

native_completed "$(cat /tmp/ambience-env-outputs.json)"
