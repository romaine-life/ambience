#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
registry_name="${REGISTRY_NAME:-romainecr}"
image_repository="${IMAGE_REPOSITORY:-ambience}"
registry_server="${REGISTRY_SERVER:-${registry_name}.azurecr.io}"
tag="${1:-}"

if [[ -z "$tag" ]]; then
  short_sha="$(git -C "$repo_root" rev-parse --short=7 HEAD)"
  stamp="$(date -u +%Y%m%d%H%M%S)"
  tag="agent-${short_sha}-${stamp}"
fi

image="${registry_server}/${image_repository}:${tag}"

existing_tag="$(az acr repository show-tags \
  --name "$registry_name" \
  --repository "$image_repository" \
  --query "[?@=='${tag}'] | [0]" \
  --output tsv || true)"

if [[ "$existing_tag" == "$tag" ]]; then
  echo "Image tag '${tag}' already exists in ${registry_server}/${image_repository}; skipping build." >&2
else
  az acr build \
    --registry "$registry_name" \
    --image "${image_repository}:${tag}" \
    "$repo_root"
fi

verified_tag="$(az acr repository show-tags \
  --name "$registry_name" \
  --repository "$image_repository" \
  --query "[?@=='${tag}'] | [0]" \
  --output tsv || true)"

if [[ "$verified_tag" != "$tag" ]]; then
  echo "Image tag '${tag}' was not found in ${registry_server}/${image_repository} after build." >&2
  exit 1
fi

printf 'TAG=%s\n' "$tag"
printf 'IMAGE=%s\n' "$image"
printf 'REGISTRY_SERVER=%s\n' "$registry_server"
