#!/usr/bin/env bash

set -Eeuo pipefail

GLIMMUNG_BASE="${GLIMMUNG_BASE:-https://glimmung.romaine.life}"
GLIMMUNG_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
GLIMMUNG_WORKFLOW="${GLIMMUNG_WORKFLOW:-agent-run}"
AMBIENCE_NATIVE_RUNNER_IMAGE="${AMBIENCE_NATIVE_RUNNER_IMAGE:?set AMBIENCE_NATIVE_RUNNER_IMAGE to the pushed native runner image}"
AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID:-aee0cbd2-8074-4001-b610-0f8edb4eaa3c}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-latest}"
AGENT_SCREENSHOT_STORAGE_ACCOUNT="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
AGENT_SCREENSHOT_CONTAINER="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
AGENT_SCREENSHOT_CONTAINER_URL="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"

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

workflow_payload="$(
  jq -nc \
    --arg project "$GLIMMUNG_PROJECT" \
    --arg workflow "$GLIMMUNG_WORKFLOW" \
    --arg image "$AMBIENCE_NATIVE_RUNNER_IMAGE" \
    --arg subscription "$AZURE_SUBSCRIPTION_ID" \
    --arg agent_container_tag "$AGENT_CONTAINER_TAG" \
    --arg screenshot_account "$AGENT_SCREENSHOT_STORAGE_ACCOUNT" \
    --arg screenshot_container "$AGENT_SCREENSHOT_CONTAINER" \
    --arg screenshot_url "$AGENT_SCREENSHOT_CONTAINER_URL" \
    '{
      project: $project,
      name: $workflow,
      trigger_label: "issue-agent",
      budget: {total: 25.0},
      default_requirements: {},
      phases: [
        {
          name: "env-prep",
          kind: "k8s_job",
          outputs: [
            "validation_url",
            "validation_slot_index",
            "namespace",
            "image_tag",
            "claude_namespace",
            "claude_ca_namespace"
          ],
          jobs: [
            {
              id: "env-prep",
              name: "Build and deploy validation environment",
              image: $image,
              command: ["/bin/bash", "/opt/ambience-native/scripts/env-prep.sh"],
              env: {
                AZURE_SUBSCRIPTION_ID: $subscription
              },
              timeout_seconds: 2400,
              steps: [
                {slug: "clone-repo", title: "Clone repository"},
                {slug: "build-validation-image", title: "Build validation image"},
                {slug: "push-validation-image", title: "Verify validation image"},
                {slug: "deploy-validation-env", title: "Deploy validation environment"},
                {slug: "check-validation-env", title: "Check validation environment"},
                {slug: "emit-env-outputs", title: "Emit phase outputs"}
              ]
            }
          ]
        },
        {
          name: "agent-execute",
          kind: "k8s_job",
          inputs: {
            validation_url: "${{ phases.env-prep.outputs.validation_url }}",
            validation_slot_index: "${{ phases.env-prep.outputs.validation_slot_index }}",
            namespace: "${{ phases.env-prep.outputs.namespace }}",
            image_tag: "${{ phases.env-prep.outputs.image_tag }}",
            claude_namespace: "${{ phases.env-prep.outputs.claude_namespace }}",
            claude_ca_namespace: "${{ phases.env-prep.outputs.claude_ca_namespace }}"
          },
          verify: true,
          # Recycle disabled (max_attempts: 0) — any verify_fail /
          # verify_malformed aborts the run immediately. Restore a
          # real cap once the verifier is trustworthy enough to retry
          # on without spawning runaway recycle children.
          recycle_policy: {
            max_attempts: 0,
            on: ["verify_fail", "verify_malformed"],
            lands_at: "self"
          },
          jobs: [
            {
              id: "agent-execute",
              name: "Run agent and verify result",
              image: $image,
              command: ["/bin/bash", "/opt/ambience-native/scripts/agent-execute.sh"],
              env: {
                AZURE_SUBSCRIPTION_ID: $subscription,
                AGENT_CONTAINER_TAG: $agent_container_tag,
                AGENT_SCREENSHOT_STORAGE_ACCOUNT: $screenshot_account,
                AGENT_SCREENSHOT_CONTAINER: $screenshot_container,
                AGENT_SCREENSHOT_CONTAINER_URL: $screenshot_url
              },
              timeout_seconds: 5400,
              steps: [
                {slug: "clone-repo", title: "Clone repository"},
                {slug: "prepare-agent-context", title: "Prepare agent context"},
                {slug: "run-agent", title: "LLM: Run agent job"},
                {slug: "collect-evidence", title: "Collect evidence"},
                {slug: "summarize-agent", title: "Summarize agent result"},
                {slug: "verify-result", title: "Verify result"},
                {slug: "push-branch", title: "Confirm pushed branch"},
                {slug: "emit-agent-outputs", title: "Emit verification"}
              ]
            }
          ]
        }
      ],
      pr: {
        enabled: true,
        recycle_policy: {
          max_attempts: 3,
          on: ["pr_review_changes_requested"],
          lands_at: "agent-execute"
        }
      }
    }'
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
echo "registered ${GLIMMUNG_PROJECT}/${GLIMMUNG_WORKFLOW} -> ${AMBIENCE_NATIVE_RUNNER_IMAGE}"
