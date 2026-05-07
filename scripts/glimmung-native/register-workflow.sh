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
      # trigger_label intentionally null. The label-driven trigger is
      # legacy from when glimmung listened for `issues.labeled` webhooks;
      # runs are now started directly via the glimmung UI/API. Sent
      # explicitly so an upsert clears any preserved value from a
      # previous registration. (Was the empty string until the glimmung
      # schema accepted nullable trigger_label — see nelsong6/glimmung#298.)
      trigger_label: null,
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
                {slug: "reap-slot-conflicts", title: "Reap conflicting slot claimants"},
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
                {slug: "run-plan-and-implement", title: "LLM: Plan + implement"},
                {slug: "push-branch", title: "Confirm pushed branch"},
                {slug: "rebuild-validation", title: "Rebuild validation env"},
                {slug: "run-verification", title: "LLM: Verify"},
                {slug: "collect-evidence", title: "Collect evidence"},
                {slug: "summarize-agent", title: "Summarize agent result"},
                {slug: "verify-result", title: "Verify result"},
                {slug: "upload-screenshots", title: "Upload screenshots"},
                {slug: "emit-agent-outputs", title: "Emit verification"}
              ]
            }
          ]
        },
        {
          # Always-run teardown (glimmung#296). Runs on success/abort/fail
          # so the slot namespace is cleaned up immediately rather than
          # leaking until the next env-prep on the same slot reaps it.
          # The env-prep slot-reap (ambience#224) stays in place as
          # belt-and-suspenders for the case where this teardown fails.
          name: "env-destroy",
          kind: "k8s_job",
          always: true,
          jobs: [
            {
              id: "env-destroy",
              name: "Tear down validation environment",
              image: $image,
              command: ["/bin/bash", "/opt/ambience-native/scripts/env-destroy.sh"],
              env: {
                AZURE_SUBSCRIPTION_ID: $subscription
              },
              timeout_seconds: 600,
              steps: [
                {slug: "describe-pre-teardown", title: "Describe pre-teardown state"},
                {slug: "uninstall-helm-release", title: "Uninstall helm release"},
                {slug: "delete-namespace", title: "Delete namespace"}
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
