#!/usr/bin/env bash

# Register the agent-run workflow with glimmung from the canonical
# .glimmung/workflows/agent-run.yaml file. This is the legacy push
# path; once the upstream-sync UI ships in glimmung (#299), prefer
# clicking "Install" in the workflow detail view, or calling the
# /v1/projects/{project}/workflows/{name}/sync endpoint via MCP.
# Kept for bootstrap and for environments where the UI/MCP path
# isn't yet wired up.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GLIMMUNG_BASE="${GLIMMUNG_BASE:-https://glimmung.romaine.life}"
GLIMMUNG_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
GLIMMUNG_WORKFLOW="${GLIMMUNG_WORKFLOW:-agent-run}"
WORKFLOW_FILE="${WORKFLOW_FILE:-${REPO_ROOT}/.glimmung/workflows/${GLIMMUNG_WORKFLOW}.yaml}"

if [ ! -f "$WORKFLOW_FILE" ]; then
  echo "workflow file not found: $WORKFLOW_FILE" >&2
  exit 1
fi

admin_token() {
  if [ -n "${GLIMMUNG_ADMIN_TOKEN:-}" ]; then
    printf '%s' "$GLIMMUNG_ADMIN_TOKEN"
    return 0
  fi
  if [ -n "${GLIMMUNG_ADMIN_TOKEN_FILE:-}" ]; then
    tr -d '\n' <"$GLIMMUNG_ADMIN_TOKEN_FILE"
    return 0
  fi
  tr -d '\n' </var/run/secrets/kubernetes.io/serviceaccount/token
}

admin_curl() {
  local method="$1"
  local path="$2"
  local body="$3"
  local token
  token="$(admin_token)"
  curl -fsS \
    -X "$method" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "${GLIMMUNG_BASE}${path}"
}

# Convert YAML to JSON and inject project + name from the call site
# (file location is authoritative — see .glimmung/workflows/*.yaml
# header). Any project / name embedded in the file is ignored.
workflow_payload="$(
  python3 - <<PY
import json, sys, yaml
with open(${WORKFLOW_FILE@Q}) as f:
    body = yaml.safe_load(f)
body['project'] = ${GLIMMUNG_PROJECT@Q}
body['name'] = ${GLIMMUNG_WORKFLOW@Q}
print(json.dumps(body))
PY
)"

if [ "${GLIMMUNG_REGISTER_DRY_RUN:-}" = "1" ]; then
  printf '%s\n' "$workflow_payload"
  exit 0
fi

project_payload="$(
  jq -nc \
    --arg name "$GLIMMUNG_PROJECT" \
    --arg repo "nelsong6/ambience" \
    '{
      name: $name,
      github_repo: $repo,
      metadata: {
        runner: "native-k8s",
        native_webapp: true,
        native_standby_dns: {
          enabled: true,
          record_base: "ambience.dev.romaine.life",
          slot_prefix: "ambience-slot"
        }
      }
    }'
)"
admin_curl POST /v1/projects "$project_payload" >/dev/null
echo "registered project ${GLIMMUNG_PROJECT}"

admin_curl POST /v1/workflows "$workflow_payload"
echo
echo "registered ${GLIMMUNG_PROJECT}/${GLIMMUNG_WORKFLOW} from ${WORKFLOW_FILE}"
