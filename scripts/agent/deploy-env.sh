#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <namespace> <image> [release]" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
chart_path="${CHART_PATH:-${repo_root}/chart/ambience}"
namespace="$1"
image="$2"
release="${3:-ambience-agent}"
timeout="${HELM_TIMEOUT:-10m}"
rollout_timeout="${ROLLOUT_TIMEOUT:-180s}"
image_repository="${image%:*}"
image_tag="${image##*:}"
service_name="${SERVICE_NAME:-ambience}"
service_url="http://${service_name}.${namespace}.svc.cluster.local"

helm upgrade --install "$release" "$chart_path" \
  --namespace "$namespace" \
  --create-namespace \
  --wait \
  --timeout "$timeout" \
  --set "image.repository=${image_repository}" \
  --set "image.tag=${image_tag}" \
  --set "image.pullPolicy=Always" \
  --set "edge.replicas=1" \
  --set "authority.replicas=1" \
  --set "pdb.enabled=false" \
  --set "edge.shutdownDrain=1s" \
  --set "edge.terminationGracePeriodSeconds=3" \
  --set "authority.terminationGracePeriodSeconds=5" \
  --set "route.enabled=false" \
  --set "certificate.enabled=false" \
  --set "gateway.listenerSetEnabled=false"

kubectl rollout status deployment/ambience-edge -n "$namespace" --timeout="$rollout_timeout"
kubectl rollout status statefulset/ambience-authority -n "$namespace" --timeout="$rollout_timeout"

printf 'NAMESPACE=%s\n' "$namespace"
printf 'RELEASE=%s\n' "$release"
printf 'SERVICE_URL=%s\n' "$service_url"
