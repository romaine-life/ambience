#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

feature_type="${AMBIENCE_FEATURE_TYPE:-${1:-generic}}"
output="${AMBIENCE_IMPLEMENTATION_CONTRACT:-${2:-/workspace/evidence/implementation-contract.json}}"

mkdir -p "$(dirname "$output")"

case "$feature_type" in
  effect)
    "${SCRIPT_DIR}/effect.sh" "$output"
    ;;
  ""|generic)
    jq -n \
      --arg project "ambience" \
      --arg feature_type "${feature_type:-generic}" \
      '{
        schema_version: 1,
        kind: "implementation_contract",
        project: $project,
        feature_type: $feature_type,
        summary: "Generic ambience implementation contract. Follow the issue and repo docs; no feature-type-specific touchpoint rules are declared.",
        read_first: ["AGENTS.md", "CLAUDE.md"],
        required_outputs: ["implementation"],
        validation_commands: ["git diff --check"],
        checks: []
      }' >"$output"
    ;;
  *)
    jq -n \
      --arg project "ambience" \
      --arg feature_type "$feature_type" \
      '{
        schema_version: 1,
        kind: "implementation_contract",
        project: $project,
        feature_type: $feature_type,
        status: "unsupported",
        summary: "No ambience implementation contract generator is registered for this feature_type yet.",
        read_first: ["AGENTS.md", "CLAUDE.md"],
        required_outputs: ["implementation"],
        validation_commands: ["git diff --check"],
        checks: []
      }' >"$output"
    ;;
esac

jq . "$output" >/dev/null
printf '%s\n' "$output"
