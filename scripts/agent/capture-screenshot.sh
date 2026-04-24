#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: $0 <namespace> <page-path> <output-path>" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
namespace="$1"
page_path="$2"
output_path="$3"
service_name="${SERVICE_NAME:-ambience}"
local_port="${LOCAL_PORT:-18080}"
wait_ms="${WAIT_MS:-5000}"
health_path="${HEALTH_PATH:-/healthz}"

mkdir -p "$(dirname "$output_path")"

cleanup() {
  if [[ -n "${port_forward_pid:-}" ]]; then
    kill "$port_forward_pid" >/dev/null 2>&1 || true
    wait "$port_forward_pid" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

kubectl port-forward -n "$namespace" "service/${service_name}" "${local_port}:80" >/tmp/ambience-port-forward.log 2>&1 &
port_forward_pid="$!"

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${local_port}${health_path}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

if ! curl -fsS "http://127.0.0.1:${local_port}${health_path}" >/dev/null 2>&1; then
  echo "Timed out waiting for port-forward to become ready." >&2
  exit 1
fi

node "${repo_root}/scripts/agent/capture-screenshot.mjs" \
  --url "http://127.0.0.1:${local_port}${page_path}" \
  --output "$output_path" \
  --wait-ms "$wait_ms" \
  --full-page
