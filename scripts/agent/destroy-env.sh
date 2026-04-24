#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <namespace> [release]" >&2
  exit 1
fi

namespace="$1"
release="${2:-ambience-agent}"

helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
kubectl delete namespace "$namespace" --ignore-not-found=true --wait=false >/dev/null 2>&1 || true
