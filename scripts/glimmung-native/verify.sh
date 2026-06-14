#!/usr/bin/env bash
# Glimmung phase: verify.
#
# Runs after both test-plan and implement. Each verify-case-NN job selects
# one required_evidence item from the test-plan JSON. Judged cases (non-empty
# must_show) mount that single case in the verifier pod's configmap and run
# the verification LLM against the rebuilt validation env for that case only.
# Mechanical-only cases (no must_show) never launch an LLM: the wrapper pins,
# triggers, observes, and captures itself (run_mechanical_case) and
# synthesizes the same verifier-output files.
#
# Also enforces the selected required_evidence item against the verifier's
# evidence_results before emitting pass.
#
# Completion emitted to glimmung:
#   verification — typed per-case verification JSON. Glimmung aggregates all
#                  verify-case jobs and synthesizes the phase output consumed
#                  by the downstream evidence_verification_gate phase.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_INPUT_VALIDATION_URL \
  GLIMMUNG_INPUT_NAMESPACE \
  GLIMMUNG_INPUT_BRANCH_NAME

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
BRANCH_NAME="${GLIMMUNG_INPUT_BRANCH_NAME}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
if [ -n "${GLIMMUNG_DYNAMIC_CASE_INDEX:-}" ]; then
  VERIFY_CASE_NUMBER="$((10#${GLIMMUNG_DYNAMIC_CASE_INDEX}))"
  VERIFY_CASE_JOB_ID="$(printf 'verify-case-%02d' "$VERIFY_CASE_NUMBER")"
else
  VERIFY_CASE_JOB_ID="${GLIMMUNG_JOB_ID:-}"
  case "$VERIFY_CASE_JOB_ID" in
    verify-case-[0-9][0-9]) ;;
    *)
      echo "GLIMMUNG_JOB_ID must be verify-case-NN for bounded verification, got '${VERIFY_CASE_JOB_ID:-unset}'" >&2
      exit 1
      ;;
  esac
  VERIFY_CASE_NUMBER="$((10#${VERIFY_CASE_JOB_ID#verify-case-}))"
fi
if [ "$VERIFY_CASE_NUMBER" -lt 1 ] || [ "$VERIFY_CASE_NUMBER" -gt 10 ]; then
  echo "verification case ${VERIFY_CASE_JOB_ID} is outside supported range verify-case-01..verify-case-10" >&2
  exit 1
fi
VERIFY_CASE_INDEX="$((VERIFY_CASE_NUMBER - 1))"
JOB_NAME="agent-${RUN_SLUG}-vc${VERIFY_CASE_NUMBER}-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-${VERIFY_CASE_JOB_ID}"

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

VERIFY_EXIT_CODE=0
VERIFICATION_JSON="/tmp/verification.json"
VERIFICATION_REASONS="/tmp/verification-reasons.txt"
EVIDENCE_REFS="/tmp/evidence-refs.txt"
EVIDENCE_ARTIFACTS="/tmp/evidence-artifacts.json"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="${AMBIENCE_EVIDENCE_DIR:-/tmp/evidence}"
# Cross-step markdown lives under EVIDENCE_DIR, not /tmp scratch: in managed
# per-step invocation mode every step re-runs this script from the top, so a
# /tmp file truncated here is empty by the time emit reads it. EVIDENCE_DIR
# persists across the case's steps and is torn down by emit after the
# completion payload is built.
SCREENSHOTS_MD="${EVIDENCE_DIR}/screenshots.md"
SUMMARY_MD="${EVIDENCE_DIR}/summary.md"
POD_LOG="/tmp/agent-pod.log"
VERIFY_EXIT_CODE_FILE="/tmp/verification-exit-code"
PROXY_IP_FILE="/tmp/verification-proxy-ip"
CODEX_PROXY_IP_FILE="/tmp/verification-codex-proxy-ip"
VERIFICATION_CASE_FILE="${EVIDENCE_DIR}/verification-case.json"
: >"$VERIFICATION_REASONS"
: >"$EVIDENCE_REFS"
printf '[]\n' >"$EVIDENCE_ARTIFACTS"
: >"$EVENTS_LOG"
mkdir -p "$EVIDENCE_DIR/screenshots" "$EVIDENCE_DIR/videos" "$EVIDENCE_DIR/observations"

# Stage handoff files — written by prepare_context from glimmung inputs.
TEST_PLAN_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.json"
TEST_PLAN_MD_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.md"
IMPL_FILE="${EVIDENCE_DIR}/issue-agent-implementation.json"
IMPL_MD_FILE="${EVIDENCE_DIR}/issue-agent-implementation.md"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" "$BRANCH_NAME"
}

copy_claude_ca() {
  if [ -s /etc/glimmung-provider-api-proxy-ca/ca.crt ]; then
    kubectl -n "$NAMESPACE" create configmap claude-oauth-ca \
      --from-file=ca.crt=/etc/glimmung-provider-api-proxy-ca/ca.crt \
      --dry-run=client -o yaml | kubectl apply -f -
    return 0
  fi
  kubectl -n "$CLAUDE_CA_NAMESPACE" get configmap claude-oauth-ca -o json \
    | NAMESPACE="$NAMESPACE" jq '
        del(
          .metadata.annotations,
          .metadata.uid,
          .metadata.resourceVersion,
          .metadata.generation,
          .metadata.creationTimestamp,
          .metadata.managedFields
        )
        | .metadata.namespace = env.NAMESPACE
      ' \
    | kubectl apply -f -
}

prepare_context() {
  # Stage handoff artifacts from glimmung phase outputs into evidence dir.
  # GLIMMUNG_INPUT_TEST_PLAN is the test-plan JSON string; unwrap it. An
  # EMPTY input means the llm-test-plan job was when-skipped at the platform
  # (skipped legs publish nothing; their outputs resolve empty): the case
  # source is then the repo's standing case for the workflow's feature type,
  # read from the WORKFLOW checkout — never the implementation branch — so
  # the case a run is judged by cannot be edited by the run's own
  # implementation agent. The standing file is CI-linted by the contract
  # harness with the same claim-vocabulary gates a generated plan must pass.
  if [ -n "${GLIMMUNG_INPUT_TEST_PLAN:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" | jq -r . >"$TEST_PLAN_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" >"$TEST_PLAN_FILE"
    echo "staged test-plan JSON ($(wc -c <"$TEST_PLAN_FILE") bytes)"
  elif ! stage_standing_test_plan; then
    return 1
  fi

  if [ -n "${GLIMMUNG_INPUT_IMPLEMENTATION:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" | jq -r . >"$IMPL_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" >"$IMPL_FILE"
    echo "staged implementation JSON ($(wc -c <"$IMPL_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_IMPLEMENTATION not set; verify will proceed without implementation context"
  fi

  select_verification_case
  resolve_standing_case
  if [ "$(verification_case_status)" != "active" ]; then
    return 0
  fi

  # Standing cases are judged against the issue text itself (the standing
  # must_show is the gestalt umbrella; the issue body is the criteria). Stage
  # the body so the verifier prompt can carry it. Generated cases keep their
  # plan-authored claims and do not receive this block.
  if selected_case_is_standing && [ -n "${GLIMMUNG_ISSUE_BODY:-}" ]; then
    printf '%s\n' "$GLIMMUNG_ISSUE_BODY" >"${EVIDENCE_DIR}/issue-body.md"
    echo "staged issue body for standing-case judgment ($(wc -c <"${EVIDENCE_DIR}/issue-body.md") bytes)"
  fi

  start_session_keepalive_and_pin

  if verification_case_is_mechanical; then
    # Agentless mechanical case: no inner verifier Job is launched, so the
    # agent-job preparation (provider proxy IPs, CA copy, codex credentials,
    # GitHub token Secret, prompt configmap) is skipped — a mechanical case
    # must not fail because agent infrastructure is unavailable. Azure login
    # still runs: upload-screenshots authenticates with it.
    native_azure_login
    echo "${VERIFY_CASE_JOB_ID} is mechanical-only (no must_show); skipped verifier-agent preparation"
    return 0
  fi

  native_azure_login
  native_install_preview_package "${REPO_DIR}/mcp"
  copy_claude_ca
  native_prepare_codex_credentials_secret "$NAMESPACE"

  PROXY_IP="${GLIMMUNG_PROVIDER_API_PROXY_CLAUDE_IP:-}"
  if [ -z "$PROXY_IP" ]; then
    PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  fi
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP
  printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"
  CODEX_PROXY_IP="${GLIMMUNG_PROVIDER_API_PROXY_CODEX_IP:-}"
  if [ -z "$CODEX_PROXY_IP" ]; then
    CODEX_PROXY_IP="$PROXY_IP"
  fi
  export CODEX_PROXY_IP
  printf '%s\n' "$CODEX_PROXY_IP" >"$CODEX_PROXY_IP_FILE"

  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -

  # Build the agent-config configmap with the prompt + all available handoff files.
  local args=(
    --from-file=prompt-verification.md="${REPO_DIR}/.github/agent/prompt-verification.md"
  )
  for f in "$TEST_PLAN_FILE" "$TEST_PLAN_MD_FILE" "$IMPL_FILE" "$IMPL_MD_FILE" "$VERIFICATION_CASE_FILE" "${EVIDENCE_DIR}/issue-body.md"; do
    [ -s "$f" ] || continue
    base="$(basename "$f")"
    args+=(--from-file="${base}=${f}")
  done
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

# The case status lives in the durable case file, not /tmp scratch: managed
# per-step mode re-invokes this script per step, so a /tmp status would reset
# to a default between prepare and finalize and silently disable the
# skipped/plan_error branches.
verification_case_status() {
  jq -r '.status // "active"' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf 'active'
}

# A case carrying a non-empty must_show is a judged case: an inner
# verification LLM looks at the artifact and judges the claim. A case
# without one is mechanical-only: every declared expectation
# (session_config pin, trigger_event application, terminal_lifecycle hold)
# is wrapper-checkable, so no LLM job is launched and the wrapper produces
# the evidence and verdict itself (run_mechanical_case). Routing fails
# toward the judged path: when the case file cannot be read, the inner LLM
# job still runs — an agentless mis-route would synthesize an unjudged pass
# (fail open), while a judged mis-route merely costs an LLM invocation.
verification_case_is_mechanical() {
  [ "$(
    jq -r 'if ((.required_evidence.must_show // "") == "") then "mechanical" else "judged" end' \
      "$VERIFICATION_CASE_FILE" 2>/dev/null || printf 'judged'
  )" = "mechanical" ]
}

write_verification_case_file() {
  local status="$1"
  local reason="${2:-}"
  local case_json="${3:-null}"
  jq -n \
    --arg schema_version "1" \
    --arg slot_id "$VERIFY_CASE_JOB_ID" \
    --argjson slot_index "$VERIFY_CASE_INDEX" \
    --arg status "$status" \
    --arg reason "$reason" \
    --argjson required_evidence "$case_json" \
    '{
      schema_version: ($schema_version | tonumber),
      slot_id: $slot_id,
      slot_index: $slot_index,
      status: $status,
      reason: $reason,
      required_evidence: $required_evidence
    }' >"$VERIFICATION_CASE_FILE"
}

select_verification_case() {
  if [ ! -s "$TEST_PLAN_FILE" ]; then
    add_reason "missing test-plan JSON; cannot select verification case"
    write_verification_case_file "plan_error" "missing test-plan JSON"
    return 0
  fi

  local total
  total="$(jq -r '(.required_evidence // []) | length' "$TEST_PLAN_FILE" 2>/dev/null || printf 'invalid')"
  if [ "$total" = "invalid" ]; then
    add_reason "test-plan JSON is not parseable; cannot select verification case"
    write_verification_case_file "plan_error" "test-plan JSON is not parseable"
    return 0
  fi
  if [ "$total" -gt 10 ]; then
    if [ "$VERIFY_CASE_INDEX" -eq 0 ]; then
      add_reason "test plan has ${total} required_evidence items; maximum is 10"
      write_verification_case_file "plan_error" "test plan exceeds 10 required_evidence items"
    else
      write_verification_case_file "skipped" "test plan overflow reported by verify-case-01"
    fi
    return 0
  fi
  if [ "$VERIFY_CASE_INDEX" -ge "$total" ]; then
    write_verification_case_file "skipped" "no required_evidence item for this slot"
    return 0
  fi

  local case_json
  case_json="$(jq -c --argjson idx "$VERIFY_CASE_INDEX" '.required_evidence[$idx]' "$TEST_PLAN_FILE")"
  write_verification_case_file "active" "" "$case_json"
  echo "${VERIFY_CASE_JOB_ID} selected required_evidence[$VERIFY_CASE_INDEX]: $(printf '%s' "$case_json" | jq -r '.id // "unnamed"')"
}

# stage_standing_test_plan synthesizes the test-plan envelope from the repo's
# standing case when the test-plan leg was when-skipped. A declared feature
# type whose standing case file is absent or malformed is a misconfiguration,
# not a license to run with no cases — fail loudly with the path named.
stage_standing_test_plan() {
  local feature_type standing_file
  feature_type="${AMBIENCE_FEATURE_TYPE:-}"
  if [ -z "$feature_type" ]; then
    echo "GLIMMUNG_INPUT_TEST_PLAN is empty and no AMBIENCE_FEATURE_TYPE declares a standing case source" >&2
    return 1
  fi
  standing_file="${AMBIENCE_STANDING_CASE_FILE:-${REPO_DIR}/.github/agent/standing-cases/${feature_type}.json}"
  if [ ! -s "$standing_file" ] || ! jq -e . "$standing_file" >/dev/null 2>&1; then
    echo "feature_type=${feature_type} declares a standing case source but ${standing_file} is missing or malformed" >&2
    return 1
  fi
  jq -n \
    --arg feature_type "$feature_type" \
    --arg path "${standing_file#"${REPO_DIR}"/}" \
    --slurpfile standing "$standing_file" \
    '{
      schema_version: 1,
      status: "pass",
      case_source: "standing",
      feature_type: $feature_type,
      summary: ("Standing acceptance case for feature_type=" + $feature_type
        + " from " + $path
        + ". The test-plan leg was when-skipped: the feature surface does not exist at plan time, so the verifier binds this case to the implementation ui_hint and judges the capture against the issue text."),
      required_evidence: [$standing[0]]
    }' >"$TEST_PLAN_FILE"
  echo "staged standing test plan from ${standing_file#"${REPO_DIR}"/} (feature_type=${feature_type})"
}

selected_case_is_standing() {
  [ "$(jq -r '.required_evidence.source // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '')" = "standing" ]
}

# Standing cases are repo-versioned acceptance cases for feature types whose
# public surface does not exist at plan time (a new effect has no route, name,
# or schema until the implementation lands). The implementation stage emits a
# ui_hint phase output naming the new surface ({menu_label, route}); this
# binds the standing case to it: url_path = hint route + a deterministic
# session, and the hint is embedded for the verifier. Bright line: the hint is
# a DISCOVERY aid only — it tells the verifier where to look, never what
# success looks like. Judgment criteria stay the issue text.
resolve_standing_case() {
  if [ "$(verification_case_status)" != "active" ] || ! selected_case_is_standing; then
    return 0
  fi
  local hint_file hint_route hint_label case_id session resolved
  hint_file="${EVIDENCE_DIR}/ui-hint.json"
  if [ -n "${GLIMMUNG_INPUT_UI_HINT:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_UI_HINT" | jq -r 'if type == "string" then . else tojson end' >"$hint_file" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_UI_HINT" >"$hint_file"
  else
    : >"$hint_file"
  fi
  hint_route="$(jq -r '.route // ""' "$hint_file" 2>/dev/null || printf '')"
  hint_label="$(jq -r '.menu_label // ""' "$hint_file" 2>/dev/null || printf '')"
  case "$hint_route" in
    /dev/[a-z0-9_-]*) ;;
    *) hint_route="" ;;
  esac
  if [ -z "$hint_route" ] || [ -z "$hint_label" ]; then
    local detail
    detail="standing case: implementation ui_hint is missing or invalid (need {menu_label, route:/dev/<effect>}); got: $(head -c 200 "$hint_file" 2>/dev/null || printf '<unset>')"
    echo "$detail"
    # The reason rides the durable case file: /tmp reasons are truncated by
    # every managed step re-invocation, and finalize folds this back in.
    write_verification_case_file "plan_error" "$detail"
    return 0
  fi
  case_id="$(jq -r '.required_evidence.id // "standing-case"' "$VERIFICATION_CASE_FILE")"
  session="$(printf '%s' "$case_id" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-' | sed 's/-*$//')"
  resolved="$(
    jq -c \
      --arg url_path "${hint_route}?session=${session}" \
      --arg route "$hint_route" \
      --arg label "$hint_label" \
      '.required_evidence
       | .url_path = $url_path
       | .ui_hint = {menu_label: $label, route: $route}' \
      "$VERIFICATION_CASE_FILE"
  )"
  write_verification_case_file "active" "" "$resolved"
  echo "${VERIFY_CASE_JOB_ID} standing case ${case_id} bound to ui_hint: route=${hint_route} menu_label=${hint_label} session=${session}"
}

ensure_proxy_ip() {
  if [ -z "${PROXY_IP:-}" ] && [ -s "$PROXY_IP_FILE" ]; then
    PROXY_IP="$(cat "$PROXY_IP_FILE")"
    export PROXY_IP
  fi
  if [ -z "${PROXY_IP:-}" ]; then
    PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
    if [ -z "$PROXY_IP" ]; then
      echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
      return 1
    fi
    export PROXY_IP
    printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"
  fi
  if [ -z "${CODEX_PROXY_IP:-}" ] && [ -s "$CODEX_PROXY_IP_FILE" ]; then
    CODEX_PROXY_IP="$(cat "$CODEX_PROXY_IP_FILE")"
    export CODEX_PROXY_IP
  fi
  if [ -z "${CODEX_PROXY_IP:-}" ]; then
    CODEX_PROXY_IP="${GLIMMUNG_PROVIDER_API_PROXY_CODEX_IP:-${PROXY_IP:-}}"
    export CODEX_PROXY_IP
    printf '%s\n' "$CODEX_PROXY_IP" >"$CODEX_PROXY_IP_FILE"
  fi
}

run_llm() {
  case "$(verification_case_status)" in
    active) ;;
    skipped)
      echo "${VERIFY_CASE_JOB_ID} has no required_evidence item; skipping verifier agent"
      return 0
      ;;
    *)
      echo "${VERIFY_CASE_JOB_ID} cannot launch verifier agent: $(jq -r '.reason // "case selection failed"' "$VERIFICATION_CASE_FILE" 2>/dev/null || echo "case selection failed")"
      return 0
      ;;
  esac
  if verification_case_is_mechanical; then
    run_mechanical_case
    return $?
  fi
  ensure_proxy_ip
  (
    cd "$REPO_DIR"
    python3 -m ambience_preview.cli apply-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --issue-number "${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}" \
      --issue-title "$ISSUE_TITLE" \
      --issue-url "$ISSUE_URL" \
      --issue-reference "$ISSUE_REFERENCE" \
      --validation-url "$VALIDATION_URL" \
      --branch-name "$BRANCH_NAME" \
      --proxy-ip "$PROXY_IP" \
      --codex-proxy-ip "$CODEX_PROXY_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --agent-container-image "$(native_agent_container_image)" \
      --repo-slug "$REPO_SLUG" \
      --stage "verify" \
      --config-map-name "$CONFIG_MAP_NAME" \
      --agent-runtime-json "${GLIMMUNG_AGENT_RUNTIME_JSON:-}"
    native_emit_inner_job_marker "$NAMESPACE" "$JOB_NAME" verification_agent verify-agent
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_VERIFY_TIMEOUT_SECONDS:-1800}"
  )
}

run_llm_record() {
  native_record_exit_code "$VERIFY_EXIT_CODE_FILE" run_llm
  VERIFY_EXIT_CODE="$(native_read_exit_code "$VERIFY_EXIT_CODE_FILE")"
}

verify_exit_code() {
  native_read_exit_code "$VERIFY_EXIT_CODE_FILE"
}

# Synthesize the two verifier-output files for an agentless mechanical case
# in the exact schema the verifier agent writes (prompt-verification.md):
# issue-agent-verification.json drives finalize's verdict + enforcement and
# write_verification, issue-agent-verification.md feeds write_summary and the
# durable reports/ upload. Downstream steps cannot tell wrapper-synthesized
# output from agent-written output, so finalize / upload-screenshots / emit
# run unchanged.
write_mechanical_verifier_output() {
  local status="$1" abort_reason="$2" case_id="$3" expected="$4" observed_text="$5" screenshot_ref="$6" observation_ref="$7"
  jq -n \
    --arg status "$status" \
    --arg abort_reason "$abort_reason" \
    --arg id "${case_id:-<missing-id>}" \
    --arg expected "$expected" \
    --arg observed_text "$observed_text" \
    --arg screenshot "$screenshot_ref" \
    --arg observation "$observation_ref" \
    '{
      schema_version: 1,
      status: $status,
      abort_reason: $abort_reason,
      failure: (if $status == "pass" then null else {
        expected: $expected,
        observed: $observed_text,
        where: "wrapper-mechanical",
        suspected_cause: null,
        cause_detail: null
      } end),
      evidence: (
        (if $screenshot != "" then
          [{kind: "screenshot", ref: $screenshot, label: ($id + " mechanical frame"), content_type: "image/png"}]
        else [] end)
        + (if $observation != "" then
          [{kind: "artifact", ref: $observation, label: ($id + " mechanical observation"), content_type: "application/json"}]
        else [] end)
      ),
      evidence_results: [
        ({
          id: $id,
          status: (if $status == "pass" then "pass" else "fail" end),
          observed_text: $observed_text
        }
        + (if $screenshot != "" then {screenshot: $screenshot} else {} end)
        + (if $observation != "" then {observation: $observation} else {} end))
      ]
    }' >"${EVIDENCE_DIR}/issue-agent-verification.json"

  {
    printf '## Mechanical verification — %s\n\n' "${case_id:-<missing-id>}"
    printf -- '- **What I observed** — %s\n' "$observed_text"
    printf -- '- **Test process** — agentless mechanical case (no must_show, so no verification LLM was launched); the verify wrapper executed and checked the declared mechanical facts itself. Expected: %s\n' "$expected"
    if [ "$status" = "pass" ]; then
      printf -- '- **Observed deviations** — none\n'
    else
      printf -- '- **Observed deviations** — %s (%s)\n' "$observed_text" "$abort_reason"
    fi
  } >"${EVIDENCE_DIR}/issue-agent-verification.md"
}

# Agentless verification for a mechanical-only case (no must_show). The
# wrapper performs the case itself, in plan order: re-assert the session pin
# (the same pin path prepare used — finalize's enforce_session_config_pinned
# stays the authoritative re-check), fire the declared trigger through the
# documented flow (POST /dev/trigger/<session>/<event>?effect=<effect>, then
# require the event to register in snapshot.appliedEvents — accepted is not
# applied), prove a declared terminal_lifecycle with the lifecycle observer,
# and freeze a frame for screenshot cases. Every step fails closed: any
# mechanical step that cannot be completed writes a fail verdict with the
# literal failure as observed_text. The function itself returns 0 — the
# verdict travels in issue-agent-verification.json, exactly as it does for
# the agent path, and all existing finalize enforcement applies identically.
run_mechanical_case() {
  local case_id kind effect session trigger_event terminal_lifecycle hold_ticks max_ticks safe_id
  local overrides expected observed screenshot_ref observation_ref pin_out observe_out
  case_id="$(jq -r '.required_evidence.id // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '')"
  kind="$(selected_required_evidence_kind)"
  effect="$(selected_case_dev_effect)"
  session="$(selected_case_session)"
  trigger_event="$(jq -r '.required_evidence.trigger_event // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '')"
  terminal_lifecycle="$(jq -r '.required_evidence.terminal_lifecycle // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '')"
  hold_ticks="$(jq -r '.required_evidence.hold_ticks // 0 | tonumber? // 0 | floor' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '0')"
  max_ticks="$(jq -r '.required_evidence.max_ticks // 0 | tonumber? // 0 | floor' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '0')"
  safe_id="$(printf '%s' "${case_id:-mechanical-case}" | tr -c 'A-Za-z0-9_.-' '-')"
  screenshot_ref=""
  observation_ref=""
  observed="mechanical:"

  expected="mechanical contract: session_config pinned"
  [ -n "$trigger_event" ] && expected="${expected}; trigger ${trigger_event} applied"
  [ -n "$terminal_lifecycle" ] && expected="${expected}; lifecycle ${terminal_lifecycle} held ${hold_ticks} ticks"
  [ "$kind" = "screenshot" ] && expected="${expected}; frozen frame captured"

  mechanical_fail() {
    local fail_reason="$1" detail="$2"
    echo "${VERIFY_CASE_JOB_ID} mechanical case ${case_id:-<missing-id>} failed (${fail_reason}): ${detail}"
    write_mechanical_verifier_output "fail" "$fail_reason" "$case_id" "$expected" "mechanical: ${detail}" "$screenshot_ref" "$observation_ref"
    return 0
  }

  echo "${VERIFY_CASE_JOB_ID} case ${case_id:-<missing-id>} is mechanical-only (no must_show): wrapper verification, no verifier agent"

  if [ "$kind" = "video" ]; then
    mechanical_fail "mechanical_case_invalid" "kind=video requires judgment (non-empty must_show); the test-plan gate rejects this shape (video_requires_judgment)"
    return 0
  fi
  if [ -z "$effect" ] || [ -z "$session" ]; then
    mechanical_fail "mechanical_case_invalid" "mechanical cases require a /dev/<effect> url_path with a session param; got url_path=$(jq -r '.required_evidence.url_path // "<missing>"' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '<unreadable>')"
    return 0
  fi

  overrides="$(jq -c '.required_evidence.session_config // {}' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '{}')"
  if ! pin_out="$(pin_session "$effect" "$session" "$overrides" 2>&1)"; then
    mechanical_fail "session_pin_failed" "session pin failed for effect=${effect} session=${session}: $(printf '%s' "$pin_out" | tail -1)"
    return 0
  fi
  observed="${observed} pin verified (${pin_out})"

  if [ -n "$trigger_event" ]; then
    local trigger_status applied attempt
    trigger_status="$(curl -sS -o /dev/null -w "%{http_code}" \
      -X POST "${VALIDATION_URL}/dev/trigger/${session}/${trigger_event}?effect=${effect}" || printf '000')"
    case "$trigger_status" in
      2*|3*) ;;
      *)
        mechanical_fail "trigger_not_observed" "trigger ${trigger_event} for effect ${effect} returned HTTP ${trigger_status}"
        return 0
        ;;
    esac
    applied=0
    for attempt in 1 2 3 4 5 6 7 8 9 10; do
      if curl -sS "${VALIDATION_URL}/dev/snapshot?session=${session}&effect=${effect}" 2>/dev/null \
          | jq -e --arg ev "$trigger_event" '(.appliedEvents // []) | any(.event == $ev)' >/dev/null 2>&1; then
        applied=1
        break
      fi
      sleep 0.3
    done
    if [ "$applied" -ne 1 ]; then
      mechanical_fail "trigger_not_observed" "trigger ${trigger_event} was accepted (HTTP ${trigger_status}) but never registered on session ${session} (snapshot.appliedEvents)"
      return 0
    fi
    observed="${observed}; trigger ${trigger_event} accepted and applied"
  fi

  # Lifecycle observation runs without a trigger arg: the trigger (when
  # declared) was already fired above, and /dev/observe treats `applied` as
  # vacuously true when it fires no trigger itself —
  # enforce_terminal_observation_artifact still requires observed/lifecycle/hold
  # from the trace.
  if [ -n "$terminal_lifecycle" ]; then
    local shot_arg=""
    observation_ref="observations/${safe_id}.json"
    if [ "$kind" = "screenshot" ]; then
      screenshot_ref="screenshots/${safe_id}.png"
      shot_arg="${EVIDENCE_DIR}/${screenshot_ref}"
    fi
    if ! observe_out="$(dev_observe "$effect" "$session" "$terminal_lifecycle" "$hold_ticks" "$max_ticks" \
        "${EVIDENCE_DIR}/${observation_ref}" "$shot_arg" 2>&1)"; then
      observation_ref=""
      screenshot_ref=""
      mechanical_fail "lifecycle_not_observed" "lifecycle ${terminal_lifecycle} (hold ${hold_ticks} ticks) was not observed: $(printf '%s' "$observe_out" | tail -1)"
      return 0
    fi
    observed="${observed}; lifecycle ${terminal_lifecycle} held $(jq -r '.holdTicks // 0' "${EVIDENCE_DIR}/${observation_ref}" 2>/dev/null || printf '?') ticks"
  fi

  # Screenshot cases that did not get a frame from the lifecycle observation
  # still owe one (enforce_evidence_contract requires it). /dev/observe is
  # the only frozen-frame source (/dev/frame needs an observationId) and it
  # requires a predicate, so echo the session's current lifecycle back as a
  # self-satisfying predicate — dev snapshots always report one (effects
  # without the field surface the running fallback).
  if [ "$kind" = "screenshot" ] && [ -z "$screenshot_ref" ]; then
    local current_lifecycle
    current_lifecycle="$(
      curl -sS "${VALIDATION_URL}/dev/snapshot?session=${session}&effect=${effect}" 2>/dev/null \
        | jq -r '.lifecycle // ""' 2>/dev/null || printf ''
    )"
    case "$current_lifecycle" in
      intro|running|ending|ended) ;;
      *)
        mechanical_fail "screenshot_missing" "cannot freeze a frame: session ${session} snapshot did not report a usable lifecycle (got \"${current_lifecycle}\")"
        return 0
        ;;
    esac
    observation_ref="observations/${safe_id}-frame.json"
    screenshot_ref="screenshots/${safe_id}.png"
    if ! observe_out="$(dev_observe "$effect" "$session" "$current_lifecycle" "" "" \
      "${EVIDENCE_DIR}/${observation_ref}" "${EVIDENCE_DIR}/${screenshot_ref}" 2>&1)"; then
      observation_ref=""
      screenshot_ref=""
      mechanical_fail "screenshot_missing" "frame freeze via /dev/observe (lifecycle=${current_lifecycle}) failed: $(printf '%s' "$observe_out" | tail -1)"
      return 0
    fi
    observed="${observed}; frame frozen at lifecycle ${current_lifecycle}"
  fi

  write_mechanical_verifier_output "pass" "" "$case_id" "$expected" "$observed" "$screenshot_ref" "$observation_ref"
  echo "${VERIFY_CASE_JOB_ID} mechanical case ${case_id:-<missing-id>}: ${observed}"
}

collect_evidence() {
  if [ "$(verification_case_status)" != "active" ]; then
    echo "${VERIFY_CASE_JOB_ID} did not launch a verifier pod; skipping evidence collection"
    return 0
  fi
  if verification_case_is_mechanical; then
    echo "${VERIFY_CASE_JOB_ID} ran agentless (mechanical case); no verifier pod logs to collect"
    return 0
  fi
  local pod=""
  pod="$(kubectl -n "$NAMESPACE" get pods -l "job-name=${JOB_NAME}" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "$pod" ]; then
    echo "no pod found for job ${JOB_NAME}; skipping evidence capture"
    : >"$POD_LOG"
    return 0
  fi
  kubectl -n "$NAMESPACE" logs "$pod" >"$POD_LOG" || true
  echo "captured $(wc -l <"$POD_LOG") log lines from ${pod}"

  if grep -q '===EVIDENCE-TAR-START===' "$POD_LOG"; then
    if ! sed -n '/===EVIDENCE-TAR-START===/,/===EVIDENCE-TAR-END===/{//!p;}' \
        "$POD_LOG" \
        | base64 -d 2>/tmp/extract.err \
        | tar -xzf - -C "$EVIDENCE_DIR" 2>>/tmp/extract.err; then
      echo "evidence tarball extraction failed; continuing" >&2
      cat /tmp/extract.err >&2 || true
    fi
  else
    echo "no evidence tar markers found for ${JOB_NAME}"
  fi

  grep -E '^\{' "$POD_LOG" >>"$EVENTS_LOG" || true
}

add_reason() {
  printf '%s\n' "$1" >>"$VERIFICATION_REASONS"
}

verification_cost() {
  if [ -s "$EVENTS_LOG" ]; then
    jq -r 'select(.type=="result") | .total_cost_usd // 0' "$EVENTS_LOG" \
      | awk '{s+=$1} END {if (NR>0) printf "%.4f", s; else printf "0"}'
  else
    printf '0'
  fi
}

enforce_evidence_contract() {
  local plan="${TEST_PLAN_FILE}"
  local verify="${EVIDENCE_DIR}/issue-agent-verification.json"
  if [ ! -s "$plan" ] || [ ! -s "$verify" ]; then
    add_reason "missing handoff JSON; cannot enforce evidence contract"
    return 1
  fi
  local missing
  missing="$(
    jq -nr \
      --slurpfile plan "$plan" \
      --slurpfile verify "$verify" \
      --slurpfile selected_case "$VERIFICATION_CASE_FILE" \
      '
        def kind($value):
          ($value // "" | ascii_downcase) as $k
          | if $k == "animation" or $k == "webm" or $k == "movie" or $k == "recording" then "video"
            elif $k == "image" or $k == "still" then "screenshot"
            else $k
            end;
        (if (($selected_case[0].required_evidence // null) == null) then
          ($plan[0].required_evidence // [])
        else
          [$selected_case[0].required_evidence]
        end) as $req
        | ($verify[0].evidence_results // []) as $res
        | $req[]
        | . as $r
        | ($res | map(select(.id == $r.id))) as $match
        | if ($match | length) == 0 then "missing:" + ($r.id // "")
          elif $match[0].status != "pass" then "not_pass:" + ($r.id // "")
          elif kind($r.kind) == "video" and (($match[0].video // "") == "") then "missing_video:" + ($r.id // "")
          elif kind($r.kind) == "screenshot" and (($match[0].screenshot // "") == "") then "missing_screenshot:" + ($r.id // "")
          elif (($r.terminal_lifecycle // "") != "") and (($match[0].observation // "") == "") then "missing_observation:" + ($r.id // "")
          else empty
          end
      ' || printf 'jq_error:evidence_contract'
  )"
  local missing_files
  missing_files="$(
    jq -r '
      .evidence_results[]?
      | select(.status == "pass")
      | (.video?, .screenshot?, .observation?)
      | select(type == "string" and length > 0)
    ' "$verify" 2>/dev/null \
      | while IFS= read -r ref; do
          case "$ref" in
            http://*|https://*|blob://*|/v1/artifacts/*) ;;
            *)
              if [ ! -f "${EVIDENCE_DIR}/${ref}" ]; then
                printf 'missing_file:%s\n' "$ref"
              fi
              ;;
          esac
        done
  )"
  if [ -n "$missing_files" ]; then
    if [ -n "$missing" ]; then
      missing="${missing}
${missing_files}"
    else
      missing="$missing_files"
    fi
  fi
  if [ -n "$missing" ]; then
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      add_reason "evidence contract: $line"
    done <<<"$missing"
    return 1
  fi
  return 0
}

enforce_terminal_observation_artifact() {
  local verify="${EVIDENCE_DIR}/issue-agent-verification.json"
  if [ ! -s "$VERIFICATION_CASE_FILE" ] || [ ! -s "$verify" ]; then
    add_reason "terminal observation: missing handoff JSON; cannot inspect selected observation"
    return 1
  fi
  if ! jq -e '(.required_evidence.terminal_lifecycle // "") != ""' "$VERIFICATION_CASE_FILE" >/dev/null 2>&1; then
    return 0
  fi

  local evidence_id ref observation_path expected_lifecycle expected_hold failures
  evidence_id="$(jq -r '.required_evidence.id // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || true)"
  expected_lifecycle="$(jq -r '.required_evidence.terminal_lifecycle // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || true)"
  expected_hold="$(jq -r '.required_evidence.hold_ticks // 0 | tonumber? // 0' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '0')"
  ref="$(
    jq -r --arg id "$evidence_id" '
      .evidence_results[]?
      | select((.id // "") == $id and (.status // "") == "pass")
      | .observation // empty
    ' "$verify" 2>/dev/null | head -1
  )"
  if [ -z "$ref" ]; then
    add_reason "terminal observation: selected evidence ${evidence_id:-unknown} did not report an observation path"
    return 1
  fi
  case "$ref" in
    http://*|https://*|blob://*|/v1/artifacts/*)
      add_reason "terminal observation: selected evidence ${evidence_id:-unknown} reported non-local observation ref ${ref}"
      return 1
      ;;
  esac
  observation_path="$(evidence_ref_path "$ref")"
  if [ ! -s "$observation_path" ]; then
    add_reason "terminal observation: selected evidence ${evidence_id:-unknown} points at missing or empty observation ${ref}"
    return 1
  fi

  failures="$(
    jq -r \
      --arg lifecycle "$expected_lifecycle" \
      --argjson hold "${expected_hold:-0}" \
      '
        if (.applied != true) then "not_applied"
        elif (.observed != true) then "not_observed"
        elif ((.lifecycle // "") != $lifecycle) then "wrong_lifecycle:" + (.lifecycle // "")
        elif (((.holdTicks // 0) | tonumber? // 0) < $hold) then "hold_too_short:" + ((.holdTicks // 0) | tostring)
        elif (((.heldUntilTick // 0) | tonumber? // 0) < (((.observedTick // 0) | tonumber? // 0) + $hold)) then "held_until_before_required_tick"
        else empty
        end
      ' "$observation_path" 2>/dev/null || printf 'invalid_json'
  )"
  if [ -n "$failures" ]; then
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      add_reason "terminal observation: ${evidence_id:-unknown}: $line"
    done <<<"$failures"
    return 1
  fi

  echo "inspected terminal observation ${evidence_id:-unknown}: ${ref}"
  return 0
}

selected_required_evidence_kind() {
  jq -r '
    def kind($value):
      ($value // "" | ascii_downcase) as $k
      | if $k == "animation" or $k == "webm" or $k == "movie" or $k == "recording" then "video"
        elif $k == "image" or $k == "still" then "screenshot"
        else $k
        end;
    kind(.required_evidence.kind)
  ' "$VERIFICATION_CASE_FILE" 2>/dev/null || true
}

selected_case_dev_effect() {
  # /dev/<effect>[?query] -> <effect>; empty for non-dev pages.
  jq -r '
    (.required_evidence.url_path // "")
    | split("?")[0]
    | if startswith("/dev/") then (ltrimstr("/dev/") | split("/")[0]) else "" end
  ' "$VERIFICATION_CASE_FILE" 2>/dev/null || true
}

selected_case_session() {
  jq -r '
    (.required_evidence.url_path // "")
    | (split("?")[1] // "")
    | split("&")
    | map(select(startswith("session=")))
    | (first // "")
    | ltrimstr("session=")
  ' "$VERIFICATION_CASE_FILE" 2>/dev/null || true
}

# --- session pinning over plain HTTP -----------------------------------------
#
# Dev sessions are created with RANDOMIZED knob values (a /dev product
# feature). Verification claims are written against the PINNED contract:
# schema defaults overridden by the case's optional session_config. Pinning
# is a sequence of plain HTTP calls (GET schema, POST /dev/config, GET
# /dev/snapshot) — no browser — so the wrapper drives it with curl + jq
# directly (the vendored per-repo pin script was deleted with the other
# browser scripts): the comparison rules are identical to the
# Go-side contract in cmd/ambience/effects_schema_config_test.go (int knobs
# round; float knobs compare within 1e-6; overrides must name real knobs and
# sit inside [min, max]).

# pin_expected_config_json <effect> <overrides-json>
# Emits the expected pinned config object {knob: value, ...} on success.
# Fails (nonzero) with a one-line reason on stderr when the schema is
# unreachable/knobless or an override is unknown / non-numeric / out of range.
pin_expected_config_json() {
  # NOTE: never default with ${2:-{}} — bash parses the trailing `}}` so the
  # brace default leaks a literal `}` onto the value and corrupts the JSON.
  local effect="$1" overrides="${2:-}" schema
  [ -z "$overrides" ] && overrides='{}'
  if ! schema="$(curl -fsS "${VALIDATION_URL}/effects/${effect}/schema" 2>/dev/null)"; then
    echo "could not fetch effect schema for ${effect}" >&2
    return 1
  fi
  # Validate the overrides and build the expected pinned-config map in one
  # pass: defaults overridden by session_config, int knobs floored. jq's
  # error() short-circuits with a named-knob message that surfaces on stderr.
  printf '%s' "$schema" | jq -e \
    --argjson ov "$overrides" '
      (.knobs // []) as $knobs
      | if ($knobs | length) == 0 then
          error("effect schema declares no knobs; nothing to pin")
        else . end
      | ($knobs | map({(.key): .}) | add) as $byKey
      | ($ov | to_entries | map(
          .key as $k | .value as $v
          | if ($byKey[$k] == null) then error("unknown knob \($k)")
            elif ($v | type) != "number" then error("knob \($k) override is not a finite number")
            elif ($v < ($byKey[$k].min) or $v > ($byKey[$k].max)) then
              error("knob \($k)=\($v) outside schema range [\($byKey[$k].min), \($byKey[$k].max)]")
            else true end)) as $checked
      | [ $knobs[]
          | .key as $k
          | (if ($ov[$k] != null) then $ov[$k] else .default end) as $raw
          | {key: $k, value: (if .type == "int" then ($raw | floor) else $raw end)} ]
      | from_entries
    ' 2>/tmp/pin-jq-err || { sed -n 's/^jq: error[^:]*: //p; s/^jq: error: //p' /tmp/pin-jq-err 2>/dev/null | head -1 >&2; return 1; }
}

# pin_query_string <expected-config-json> -> knob=value&knob=value...
pin_query_string() {
  printf '%s' "$1" | jq -r 'to_entries | map("\(.key)=\(.value)") | join("&")'
}

# pin_session <effect> <session> <overrides-json> [timeout-seconds]
# Pins the session over HTTP and polls /dev/snapshot until the live config
# matches. Returns 0 and echoes "<knob-count> knobs" on success; nonzero with
# a one-line reason on stderr otherwise.
pin_session() {
  local effect="$1" session="$2" overrides="${3:-}" timeout="${4:-15}"
  local expected qs knob_count deadline live mismatches status
  [ -z "$overrides" ] && overrides='{}'
  if ! expected="$(pin_expected_config_json "$effect" "$overrides" 2>/tmp/pin-err)"; then
    cat /tmp/pin-err >&2 2>/dev/null || true
    return 1
  fi
  qs="$(pin_query_string "$expected")"
  knob_count="$(printf '%s' "$expected" | jq -r 'length')"
  status="$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "${VALIDATION_URL}/dev/config?session=${session}&effect=${effect}&${qs}" 2>/dev/null || printf '000')"
  case "$status" in
    2*|3*) ;;
    *) echo "POST /dev/config -> HTTP ${status}" >&2; return 1 ;;
  esac
  deadline=$(( $(date +%s) + timeout ))
  while :; do
    if live="$(curl -fsS "${VALIDATION_URL}/dev/snapshot?session=${session}&effect=${effect}" 2>/dev/null)"; then
      mismatches="$(pin_config_mismatches "$effect" "$expected" "$(printf '%s' "$live" | jq -c '.config // {}')")"
      if [ -z "$mismatches" ]; then
        echo "${knob_count} knobs"
        return 0
      fi
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "pinned config did not become live before timeout: ${mismatches:-no snapshot}" >&2
      return 1
    fi
    sleep 0.4
  done
}

# pin_config_mismatches <effect> <expected-json> <live-config-json>
# Echoes a comma-joined "key expected=.. actual=.." list of mismatches, empty
# when the live config matches the expected pinned config knob-for-knob. Int
# knobs round; float knobs compare within 1e-6 (mirrors the Go contract test).
pin_config_mismatches() {
  local effect="$1" expected="$2" live="$3" schema
  schema="$(curl -fsS "${VALIDATION_URL}/effects/${effect}/schema" 2>/dev/null || printf '{}')"
  jq -nr \
    --argjson expected "$expected" \
    --argjson live "$live" \
    --argjson schema "$schema" '
      ($schema.knobs // []) as $knobs
      | ($knobs | map({(.key): .type}) | add // {}) as $types
      | [ $expected | to_entries[]
          | .key as $k | .value as $want
          | ($live[$k]) as $got
          | (if ($got == null) then "\($k) expected=\($want) actual=null"
             elif (($types[$k] // "float") == "int") then
               (if (($got | floor) != ($want | floor)) then "\($k) expected=\($want) actual=\($got)" else empty end)
             else
               (if (($got - $want) | fabs) > 1e-6 then "\($k) expected=\($want) actual=\($got)" else empty end)
             end) ]
      | join(", ")
    '
}

# pin_check <effect> <session> <overrides-json>
# Read-only contract check used by enforce_session_config_pinned. Returns 0
# when the live session config matches the pinned contract; nonzero with a
# one-line detail (including the session tick) on stderr otherwise.
pin_check() {
  local effect="$1" session="$2" overrides="${3:-}" expected snap live tick mismatches
  [ -z "$overrides" ] && overrides='{}'
  if ! expected="$(pin_expected_config_json "$effect" "$overrides" 2>/tmp/pin-err)"; then
    cat /tmp/pin-err >&2 2>/dev/null || true
    return 1
  fi
  if ! snap="$(curl -fsS "${VALIDATION_URL}/dev/snapshot?session=${session}&effect=${effect}" 2>/dev/null)"; then
    echo "could not fetch session snapshot" >&2
    return 1
  fi
  live="$(printf '%s' "$snap" | jq -c '.config // {}')"
  tick="$(printf '%s' "$snap" | jq -r '.tick // "?"')"
  mismatches="$(pin_config_mismatches "$effect" "$expected" "$live")"
  if [ -n "$mismatches" ]; then
    # A low tick relative to the case duration means the session was reaped and
    # lazily recreated (randomized) after the pin — a session lifecycle problem,
    # not config drift. The keepalive listener exists to prevent this; surface
    # the tick so the failure reads correctly.
    echo "${mismatches} [session tick=${tick} — a low tick means the session was reaped and recreated randomized; check the keepalive listener]" >&2
    return 1
  fi
  return 0
}

# dev_observe <effect> <session> <lifecycle> <hold-ticks> <max-ticks> <out-json> [shot-png]
# Drives the verification-only /dev/observe lifecycle observer over plain HTTP
# (POST /dev/observe with the lifecycle predicate, GET /dev/frame for the
# frozen grid frame). The vendored per-repo observation script was deleted with
# the other browser scripts: /dev/observe and /dev/frame are
# JSON/PNG HTTP endpoints, so no browser is involved. The trace JSON is written
# to <out-json>; when <shot-png> is given, the observation's frameUrl PNG is
# fetched to it. Returns 0 on a 2xx observe response; nonzero with a one-line
# reason on stderr otherwise. The caller enforces applied/observed/hold from
# the written trace, exactly as enforce_terminal_observation_artifact does.
dev_observe() {
  local effect="$1" session="$2" lifecycle="$3" hold_ticks="${4:-}" max_ticks="${5:-}" out_json="$6" shot_png="${7:-}"
  local url status body frame_url frame_status
  url="${VALIDATION_URL}/dev/observe?effect=${effect}&session=${session}"
  [ -n "$lifecycle" ] && url="${url}&lifecycle=${lifecycle}"
  [ -n "$hold_ticks" ] && [ "$hold_ticks" != "0" ] && url="${url}&hold_ticks=${hold_ticks}"
  [ -n "$max_ticks" ] && [ "$max_ticks" != "0" ] && url="${url}&max_ticks=${max_ticks}"
  mkdir -p "$(dirname "$out_json")"
  status="$(curl -sS -X POST "$url" -o "$out_json" -w '%{http_code}' 2>/dev/null || printf '000')"
  case "$status" in
    2*|3*) ;;
    *)
      body="$(tail -c 200 "$out_json" 2>/dev/null | tr '\n' ' ')"
      echo "observe failed ${status}: ${body}" >&2
      return 1
      ;;
  esac
  if [ -n "$shot_png" ]; then
    frame_url="$(jq -r '.frameUrl // ""' "$out_json" 2>/dev/null || printf '')"
    if [ -z "$frame_url" ]; then
      echo "observe response did not include frameUrl" >&2
      return 1
    fi
    case "$frame_url" in
      http://*|https://*) ;;
      /*) frame_url="${VALIDATION_URL}${frame_url}" ;;
      *) frame_url="${VALIDATION_URL}/${frame_url}" ;;
    esac
    mkdir -p "$(dirname "$shot_png")"
    frame_status="$(curl -sS "$frame_url" -o "$shot_png" -w '%{http_code}' 2>/dev/null || printf '000')"
    case "$frame_status" in
      2*|3*) ;;
      *) echo "frame fetch failed ${frame_status}" >&2; return 1 ;;
    esac
  fi
  return 0
}

# Dev sessions are reaped after 60s with no listeners (devSessionIdle in
# cmd/ambience/main.go), and the gap between the verifier's capture and the
# wrapper's finalize enforcement is minutes. Without a listener the pinned
# session would die mid-case and every snapshot read after that would lazily
# create a fresh RANDOMIZED session — turning enforcement into a guaranteed
# false failure. The product contract is "a session lives while someone
# listens", so the wrapper is that someone: hold one background SSE listener
# on the case's session from prepare until emit teardown. Pin immediately
# after opening it so the session is deterministic from (near) birth — the
# verifier agent's own pin step is then an idempotent re-assert.
SESSION_KEEPALIVE_PID_FILE="/tmp/session-keepalive.pid"

start_session_keepalive_and_pin() {
  local effect session overrides pin_out
  effect="$(selected_case_dev_effect)"
  session="$(selected_case_session)"
  if [ -z "$effect" ] || [ -z "$session" ]; then
    return 0
  fi
  stop_session_keepalive
  nohup curl -sS -N --max-time 7200 \
    "${VALIDATION_URL}/dev/events?session=${session}&effect=${effect}" \
    >/dev/null 2>&1 &
  printf '%s\n' "$!" >"$SESSION_KEEPALIVE_PID_FILE"
  disown || true
  echo "session keepalive listener started for effect=${effect} session=${session} (pid $(cat "$SESSION_KEEPALIVE_PID_FILE"))"

  overrides="$(jq -c '.required_evidence.session_config // {}' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '{}')"
  if pin_out="$(pin_session "$effect" "$session" "$overrides" 2>&1)"; then
    echo "session pinned at prepare: ${pin_out}"
  else
    # Do not fail prepare: the verifier agent pins again before capture,
    # and finalize enforcement is the authoritative gate. This logs why
    # an early pin could not land (e.g. env still warming).
    echo "session pin at prepare failed (verifier will pin before capture): $(printf '%s' "$pin_out" | tail -1)"
  fi
}

stop_session_keepalive() {
  if [ -s "$SESSION_KEEPALIVE_PID_FILE" ]; then
    kill "$(cat "$SESSION_KEEPALIVE_PID_FILE")" 2>/dev/null || true
    rm -f "$SESSION_KEEPALIVE_PID_FILE"
  fi
}

# Dev sessions start with randomized knob values by design. Verification
# claims are written against the pinned contract — schema defaults plus the
# case's optional session_config — so the session the evidence was captured
# from must actually carry that config. The session is pinned at prepare and
# re-asserted by the verifier agent before capture; this check makes the pin
# a contract instead of a convention. Glimmung issue ambience#167 run 5 is
# the motivating failure: a "5-10 lantern cluster" claim judged against a
# session whose randomized cluster_min/cluster_max was 2/23.
enforce_session_config_pinned() {
  local effect session evidence_id overrides detail expected knob_count
  effect="$(selected_case_dev_effect)"
  if [ -z "$effect" ]; then
    # Non-/dev surface (e.g. the shared monitor page): no per-session config
    # contract exists there; claims must be config-agnostic by plan rule.
    return 0
  fi
  evidence_id="$(jq -r '.required_evidence.id // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || true)"
  session="$(selected_case_session)"
  if [ -z "$session" ]; then
    add_reason "session config: case ${evidence_id:-unknown} url_path has no session param; test-plan normalization must assign one so the session can be pinned"
    return 1
  fi
  overrides="$(jq -c '.required_evidence.session_config // {}' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '{}')"
  if ! detail="$(pin_check "$effect" "$session" "$overrides" 2>&1)"; then
    add_reason "session config: case ${evidence_id:-unknown} session ${session} (effect ${effect}) does not match the pinned contract (schema defaults + session_config): ${detail:-pin check failed}"
    return 1
  fi
  knob_count="?"
  if expected="$(pin_expected_config_json "$effect" "$overrides" 2>/dev/null)"; then
    knob_count="$(printf '%s' "$expected" | jq -r 'length' 2>/dev/null || printf '?')"
  fi
  echo "session config pinned for ${evidence_id:-unknown}: effect=${effect} session=${session} (${knob_count} knobs)"
  return 0
}

# enforce_declared_surface mechanically verifies the implementation-declared
# surface (the standing case's bound ui_hint): the dev route serves and the
# derived /effects/<slug>/schema route serves. Declaration replaces the
# pre-implementation contract's prediction; what was declared must exist.
enforce_declared_surface() {
  local route slug status failed=0
  route="$(jq -r '.required_evidence.ui_hint.route // ""' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf '')"
  if [ -z "$route" ]; then
    add_reason "declared surface: standing case has no bound ui_hint route to enforce"
    return 1
  fi
  slug="${route#/dev/}"
  for probe in "$route" "/effects/${slug}/schema"; do
    status="$(curl -sS -o /dev/null -w "%{http_code}" "${VALIDATION_URL}${probe}" || printf '000')"
    case "$status" in
      2*|3*) ;;
      *)
        add_reason "declared surface: ${probe} returned HTTP ${status}"
        failed=1
        ;;
    esac
  done
  if [ "$failed" -eq 0 ]; then
    echo "declared surface verified: ${route} and /effects/${slug}/schema serve"
  fi
  [ "$failed" -eq 0 ]
}

evidence_ref_path() {
  local ref="$1"
  case "$ref" in
    /workspace/evidence/*) printf '%s/%s\n' "$EVIDENCE_DIR" "${ref#/workspace/evidence/}" ;;
    /tmp/evidence/*) printf '%s\n' "$ref" ;;
    /*) printf '%s\n' "$ref" ;;
    *) printf '%s/%s\n' "$EVIDENCE_DIR" "$ref" ;;
  esac
}

# Video evidence is captured by Glimmung's central capture_video tool, which
# connects to the leased slot browser, records only after the page has
# rendered (its server-side blank-frame gate owns "did it render"), and uploads
# the WebM — returning a remote ref/url, not a local file. There is therefore
# no local decode/duration step here: the retired per-repo video-decode gate
# was removed rather than reimplemented, since the central tool already proves
# render + upload before the ref exists.

write_evidence_artifacts() {
  local artifacts_out="$1"
  local refs_out="$2"
  local file_artifacts="/tmp/evidence-files.jsonl"
  local verifier="${EVIDENCE_DIR}/issue-agent-verification.json"
  local empty_verifier="/tmp/empty-verifier.json"
  : >"$file_artifacts"
  printf '{}\n' >"$empty_verifier"

  if [ ! -s "$verifier" ]; then
    verifier="$empty_verifier"
  fi

  if compgen -G "${EVIDENCE_DIR}/screenshots/*.png" >/dev/null; then
    while IFS= read -r file; do
      local base size
      base="$(basename "$file")"
      size="$(wc -c <"$file" | tr -d ' ')"
      jq -nc \
        --arg ref "screenshots/$base" \
        --arg label "${base%.png}" \
        --argjson size "${size:-0}" \
        '{kind:"screenshot", ref:$ref, label:$label, content_type:"image/png", size_bytes:$size}' \
        >>"$file_artifacts"
    done < <(find "${EVIDENCE_DIR}/screenshots" -maxdepth 1 -type f -name '*.png' | sort)
  fi

  if compgen -G "${EVIDENCE_DIR}/videos/*" >/dev/null; then
    while IFS= read -r file; do
      local base ext size content_type
      base="$(basename "$file")"
      ext="${base##*.}"
      size="$(wc -c <"$file" | tr -d ' ')"
      case "$(printf '%s' "$ext" | tr '[:upper:]' '[:lower:]')" in
        webm) content_type="video/webm" ;;
        mp4) content_type="video/mp4" ;;
        mov) content_type="video/quicktime" ;;
        *) content_type="video/webm" ;;
      esac
      jq -nc \
        --arg ref "videos/$base" \
        --arg label "${base%.*}" \
        --arg content_type "$content_type" \
        --argjson size "${size:-0}" \
        '{kind:"video", ref:$ref, label:$label, content_type:$content_type, size_bytes:$size}' \
        >>"$file_artifacts"
    # Curated video evidence now lives as a remote ref on the verifier output
    # (the central capture_video tool uploads the WebM and returns its ref),
    # folded in below from $verifier. This local glob stays only to surface any
    # stray local clip a future flow might drop here; the agent self-capture
    # path that produced raw page@<hash>.webm byproducts is gone.
    done < <(find "${EVIDENCE_DIR}/videos" -maxdepth 1 -type f \( -name '*.webm' -o -name '*.mp4' -o -name '*.mov' \) ! -name 'page@*' | sort)
  fi

  if compgen -G "${EVIDENCE_DIR}/observations/*.json" >/dev/null; then
    while IFS= read -r file; do
      local base size
      base="$(basename "$file")"
      size="$(wc -c <"$file" | tr -d ' ')"
      jq -nc \
        --arg ref "observations/$base" \
        --arg label "${base%.json}" \
        --argjson size "${size:-0}" \
        '{kind:"artifact", ref:$ref, label:$label, content_type:"application/json", size_bytes:$size}' \
        >>"$file_artifacts"
    done < <(find "${EVIDENCE_DIR}/observations" -maxdepth 1 -type f -name '*.json' | sort)
  fi

  jq -s \
    --arg run_prefix "runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}" \
    --slurpfile verifier "$verifier" '
    def normalize_ref($ref): ($ref | tostring | sub("^/workspace/evidence/"; "") | sub("^/tmp/evidence/"; ""));
    def run_scoped_ref($ref):
      if ($ref | startswith("observations/")) then $run_prefix + "/" + $ref
      else $ref
      end;
    def first_ref($item): normalize_ref($item.ref // $item.artifact_path // $item.url // "") | run_scoped_ref(.);
    def norm_kind($kind; $ref):
      ($kind // "" | ascii_downcase) as $k
      | ($ref // "" | ascii_downcase) as $r
      | if $k == "screenshot" or $k == "image" or $k == "still" then "screenshot"
        elif $k == "video" or $k == "animation" or $k == "webm" or $k == "movie" or $k == "recording" then "video"
        elif ($r | test("\\.(webm|mp4|mov|m4v)([?#].*)?$")) then "video"
        elif ($r | test("\\.(png|jpg|jpeg|webp|gif)([?#].*)?$")) then "screenshot"
        else "artifact"
        end;
    def clean:
      first_ref(.) as $ref
      | norm_kind(.kind; $ref) as $kind
      | {
          kind: $kind,
          ref: ($ref | tostring),
          label: ((.label // .id // ($ref | split("/")[-1] // "")) | tostring),
          content_type: ((.content_type // (if $kind == "video" then "video/webm" elif $kind == "screenshot" then "image/png" else "" end)) | tostring),
          size_bytes: ((.size_bytes // 0) | tonumber? // 0),
          duration_ms: ((.duration_ms // 0) | tonumber? // 0)
        }
      | with_entries(select(.value != "" and .value != 0));
    ($verifier[0] // {}) as $v
    | (
        [
          ($v.evidence // [])[]?,
          ($v.evidence_artifacts // [])[]?,
          (($v.evidence_results // [])[]? | select((.video // "") != "") | {kind:"video", ref:.video, label:(.label // .id // ""), content_type:"video/webm", duration_ms:(.duration_ms // 0)}),
          (($v.evidence_results // [])[]? | select((.screenshot // "") != "") | {kind:"screenshot", ref:.screenshot, label:(.label // .id // ""), content_type:"image/png"}),
          (($v.evidence_results // [])[]? | select((.observation // "") != "") | {kind:"artifact", ref:.observation, label:(.label // .id // ""), content_type:"application/json"})
        ] + .
      )
    | map(clean)
    | map(select(.ref != ""))
    | unique_by(.kind + "\u0000" + .ref)
  ' "$file_artifacts" >"$artifacts_out"

  jq -r '.[].ref' "$artifacts_out" >"$refs_out"
}

write_verification() {
  local status="$1"
  local cost verifier_file case_file
  cost="$(verification_cost)"
  write_evidence_artifacts "$EVIDENCE_ARTIFACTS" "$EVIDENCE_REFS"
  verifier_file="${EVIDENCE_DIR}/issue-agent-verification.json"
  if [ ! -s "$verifier_file" ] || ! jq -e . "$verifier_file" >/dev/null 2>&1; then
    verifier_file="/dev/null"
  fi
  case_file="$VERIFICATION_CASE_FILE"
  if [ ! -s "$case_file" ] || ! jq -e . "$case_file" >/dev/null 2>&1; then
    case_file="/dev/null"
  fi
  # The emitted verification JSON is the durable per-case verdict glimmung
  # stores and renders. It carries the WHY, not just the enum: the selected
  # case definition (what was being verified), the verifier's per-evidence
  # observed_text, and the structured failure block (expected / observed /
  # where / suspected_cause / cause_detail). When the verifier failed to
  # produce a failure block — or the failure was detected by wrapper
  # enforcement after a verifier pass — synthesize one from the case
  # definition and the first recorded reason so downstream surfaces always
  # have expected-vs-observed to show.
  jq -n \
    --arg status "$status" \
    --argjson reasons "$(jq -Rs 'split("\n")[:-1]' "$VERIFICATION_REASONS")" \
    --argjson evidence_refs "$(jq -Rs 'split("\n")[:-1]' "$EVIDENCE_REFS")" \
    --argjson evidence "$(cat "$EVIDENCE_ARTIFACTS")" \
    --argjson cost_usd "${cost:-0}" \
    --arg run_id "$GLIMMUNG_RUN_ID" \
    --arg branch "$BRANCH_NAME" \
    --arg validation_url "$VALIDATION_URL" \
    --arg verification_case "$VERIFY_CASE_JOB_ID" \
    --argjson verification_case_index "$VERIFY_CASE_INDEX" \
    --arg verification_case_status "$(verification_case_status)" \
    --slurpfile verifier_doc_raw "$verifier_file" \
    --slurpfile case_doc_raw "$case_file" \
    '
    ($verifier_doc_raw[0] // {}) as $verifier
    | ($case_doc_raw[0].required_evidence // {}) as $case
    | (($case.url_path // "")
        | (split("?")[1] // "") | split("&")
        | map(select(startswith("session="))) | (first // "")
        | ltrimstr("session=")) as $session
    | (if ($verifier.failure | type) == "object" then $verifier.failure
       elif $status != "pass" then
         {
           expected: ($case.must_show // ""),
           observed: (
             [
               ($verifier.evidence_results // [])[]
               | select((.status // "") != "pass")
               | .observed_text // empty
             ] + $reasons
             | map(select(. != "")) | first // ""
           ),
           where: "wrapper-synthesized",
           suspected_cause: null,
           cause_detail: null
         }
       else null
       end) as $failure
    | {
      schema_version: 1,
      status: $status,
      reasons: $reasons,
      failure: $failure,
      evidence_refs: $evidence_refs,
      evidence: $evidence,
      cost_usd: $cost_usd,
      prompt_version: "ambience-native-staged-v2",
      verifier: (if $verifier == {} then null else {
        status: ($verifier.status // null),
        abort_reason: ($verifier.abort_reason // null),
        evidence_results: ($verifier.evidence_results // [])
      } end),
      metadata: {
        run_id: $run_id,
        branch: $branch,
        validation_url: $validation_url,
        verification_case: {
          job_id: $verification_case,
          index: $verification_case_index,
          status: $verification_case_status,
          id: ($case.id // null),
          kind: ($case.kind // null),
          must_show: ($case.must_show // null),
          url_path: ($case.url_path // null),
          trigger_event: ($case.trigger_event // null),
          session: (if $session == "" then null else $session end),
          session_config: ($case.session_config // null)
        }
      }
    }' >"$VERIFICATION_JSON"
  jq -c . "$VERIFICATION_JSON"
}

finalize() {
  # The verifier's markdown (What I observed / Test process / Observed
  # deviations) is the human why-channel for every outcome — a failed case
  # needs it more than a passing one. Build the summary before any verdict
  # branch; emit rebuilds it as well since managed per-step invocations do
  # not share /tmp truncation state.
  write_summary

  case "$(verification_case_status)" in
    skipped)
      write_verification "pass"
      return 0
      ;;
    plan_error)
      # The selection/resolution failure detail rides the case file (durable
      # across managed step invocations); fold it into the reasons channel so
      # the verdict says WHY, not just that selection failed.
      add_reason "case selection: $(jq -r '.reason // "plan_error"' "$VERIFICATION_CASE_FILE" 2>/dev/null || printf 'plan_error')"
      write_verification "fail"
      return 0
      ;;
  esac

  local verifier_status
  verifier_status="$(jq -r '.status // "missing"' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo missing)"
  if [ "$verifier_status" != "pass" ]; then
    VERIFY_EXIT_CODE="$(verify_exit_code)"
    if [ "$VERIFY_EXIT_CODE" -ne 0 ]; then
      add_reason "verify pod exited with ${VERIFY_EXIT_CODE}; see native step logs"
      if [ -s "$POD_LOG" ]; then
        grep -E "::error::|Job failed|FATAL|panic:|aborted:|forbidden|exited without writing|did not reach a terminal Job condition" "$POD_LOG" \
          | head -5 >>"$VERIFICATION_REASONS" || true
      fi
    fi
    add_reason "verifier reported status=${verifier_status} reason=$(jq -r '.abort_reason // ""' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo "")"
    append_verifier_failure_reasons
    write_verification "fail"
    return 0
  fi

  # The issue-contract stage is retired: public names settle by the
  # implementation's declaration. The declared surface (bound ui_hint route
  # + derived schema route) is the only mechanical surface gate.
  if selected_case_is_standing && ! enforce_declared_surface; then
    write_verification "fail"
    return 0
  fi

  if ! enforce_evidence_contract; then
    write_verification "fail"
    return 0
  fi

  if ! enforce_session_config_pinned; then
    write_verification "fail"
    return 0
  fi

  if ! enforce_terminal_observation_artifact; then
    write_verification "fail"
    return 0
  fi

  write_verification "pass"
}

# Distill the verifier's structured failure block into the reasons channel so
# the one-line step failure message answers "why", not just "which enum".
# A non-pass verifier verdict without a failure block is itself a contract
# violation worth naming — the prompt requires expected/observed/cause.
append_verifier_failure_reasons() {
  local verifier="${EVIDENCE_DIR}/issue-agent-verification.json"
  [ -s "$verifier" ] || return 0
  if jq -e '(.failure | type) == "object"' "$verifier" >/dev/null 2>&1; then
    while IFS= read -r line; do
      [ -n "$line" ] && add_reason "$line"
    done < <(
      jq -r '
        def trunc: tostring | if length > 400 then .[:400] + "…" else . end;
        .failure
        | (if (.expected // "") != "" then "expected: " + (.expected | trunc) else empty end),
          (if (.observed // "") != "" then
            "observed: " + (.observed | trunc)
            + (if (.where // "") != "" then " [" + .where + "]" else "" end)
          else empty end),
          (if (.suspected_cause // "") != "" then
            "suspected cause: " + .suspected_cause
            + (if (.cause_detail // "") != "" then " — " + (.cause_detail | trunc) else "" end)
          else empty end)
      ' "$verifier" 2>/dev/null || true
    )
  else
    local observed
    observed="$(
      jq -r '
        def trunc: tostring | if length > 400 then .[:400] + "…" else . end;
        [.evidence_results[]? | select((.status // "") != "pass") | .observed_text // empty]
        | map(select(. != "")) | first // "" | if . == "" then empty else "observed: " + trunc end
      ' "$verifier" 2>/dev/null || true
    )"
    [ -n "$observed" ] && add_reason "$observed"
    add_reason "verifier omitted the required failure block (expected/observed/where/suspected_cause); see issue-agent-verification.md"
  fi
  return 0
}

upload_evidence() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"
  local max_videos="${MAX_VIDEOS:-10}"
  local max_observations="${MAX_OBSERVATIONS:-10}"
  local screenshot_prefix video_prefix observation_prefix report_prefix screenshot_staging video_staging observation_staging screenshot_total screenshot_taken video_total video_taken observation_total observation_taken report_taken upload_ok
  screenshot_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/screenshots"
  video_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/videos"
  observation_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/observations"
  report_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/reports"
  screenshot_staging="$(mktemp -d)"
  video_staging="$(mktemp -d)"
  observation_staging="$(mktemp -d)"
  screenshot_total=0
  screenshot_taken=0
  video_total=0
  video_taken=0
  observation_total=0
  observation_taken=0
  report_taken=0
  upload_ok=true

  if compgen -G "${EVIDENCE_DIR}/screenshots/*.png" >/dev/null; then
    while IFS= read -r file; do
      screenshot_total=$((screenshot_total + 1))
      if [ "$screenshot_taken" -lt "$max_screenshots" ]; then
        cp "$file" "$screenshot_staging/"
        screenshot_taken=$((screenshot_taken + 1))
      fi
    done < <(find "${EVIDENCE_DIR}/screenshots" -maxdepth 1 -type f -name '*.png' | sort)

    if ! az storage blob upload-batch \
        --account-name "$storage_account" \
        --destination "$container" \
        --destination-path "$screenshot_prefix" \
        --source "$screenshot_staging" \
        --auth-mode login \
        --overwrite true; then
      upload_ok=false
      echo "screenshot upload failed; report body will point at native logs"
    fi
  fi

  if compgen -G "${EVIDENCE_DIR}/videos/*" >/dev/null; then
    while IFS= read -r file; do
      video_total=$((video_total + 1))
      if [ "$video_taken" -lt "$max_videos" ]; then
        cp "$file" "$video_staging/"
        video_taken=$((video_taken + 1))
      fi
    # Video evidence is normally a remote ref from the central capture_video
    # tool (uploaded by Glimmung, not staged here). This local upload path stays
    # only for any stray local clip; the agent self-capture byproducts
    # (page@<hash>.webm) it used to filter no longer exist.
    done < <(find "${EVIDENCE_DIR}/videos" -maxdepth 1 -type f \( -name '*.webm' -o -name '*.mp4' -o -name '*.mov' \) ! -name 'page@*' | sort)

    while IFS= read -r file; do
      local base content_type
      [ -e "$file" ] || continue
      base="$(basename "$file")"
      case "$(printf '%s' "${base##*.}" | tr '[:upper:]' '[:lower:]')" in
        webm) content_type="video/webm" ;;
        mp4) content_type="video/mp4" ;;
        mov) content_type="video/quicktime" ;;
        *) content_type="video/webm" ;;
      esac
      if ! az storage blob upload \
          --account-name "$storage_account" \
          --container-name "$container" \
          --name "${video_prefix}/${base}" \
          --file "$file" \
          --auth-mode login \
          --overwrite true \
          --content-type "$content_type"; then
        upload_ok=false
        echo "video upload failed for ${base}; report body will point at native logs"
      fi
    done < <(find "$video_staging" -maxdepth 1 -type f | sort)
  fi

  if compgen -G "${EVIDENCE_DIR}/observations/*.json" >/dev/null; then
    while IFS= read -r file; do
      observation_total=$((observation_total + 1))
      if [ "$observation_taken" -lt "$max_observations" ]; then
        cp "$file" "$observation_staging/"
        observation_taken=$((observation_taken + 1))
      fi
    done < <(find "${EVIDENCE_DIR}/observations" -maxdepth 1 -type f -name '*.json' | sort)

    while IFS= read -r file; do
      local base
      [ -e "$file" ] || continue
      base="$(basename "$file")"
      if ! az storage blob upload \
          --account-name "$storage_account" \
          --container-name "$container" \
          --name "${observation_prefix}/${base}" \
          --file "$file" \
          --auth-mode login \
          --overwrite true \
          --content-type "application/json"; then
        upload_ok=false
        echo "observation upload failed for ${base}; report body will point at native logs"
      fi
    done < <(find "$observation_staging" -maxdepth 1 -type f | sort)
  fi

  # The verifier's report files are evidence too — they carry the why
  # (What I observed / Test process / Observed deviations, plus the
  # structured failure block). Upload them durably, prefixed by the case
  # slot so multi-case runs do not overwrite each other.
  local report_file report_base report_blob report_content_type
  for report_file in \
    "${EVIDENCE_DIR}/issue-agent-verification.json" \
    "${EVIDENCE_DIR}/issue-agent-verification.md" \
    "$VERIFICATION_CASE_FILE"; do
    [ -s "$report_file" ] || continue
    report_base="$(basename "$report_file")"
    report_blob="${VERIFY_CASE_JOB_ID}-${report_base}"
    case "$report_base" in
      *.json) report_content_type="application/json" ;;
      *) report_content_type="text/markdown" ;;
    esac
    if az storage blob upload \
        --account-name "$storage_account" \
        --container-name "$container" \
        --name "${report_prefix}/${report_blob}" \
        --file "$report_file" \
        --auth-mode login \
        --overwrite true \
        --content-type "$report_content_type"; then
      report_taken=$((report_taken + 1))
    else
      upload_ok=false
      echo "report upload failed for ${report_blob}; report body will point at native logs"
    fi
  done

  if [ "$screenshot_total" -eq 0 ] && [ "$video_total" -eq 0 ] && [ "$observation_total" -eq 0 ] && [ "$report_taken" -eq 0 ]; then
    rm -rf "$screenshot_staging" "$video_staging" "$observation_staging"
    return 0
  fi

  {
    echo "## Evidence"
    echo ""
    if [ "$upload_ok" = "false" ]; then
      echo "_Evidence upload failed; see the Glimmung native run logs._"
      echo ""
    fi

    if [ "$report_taken" -gt 0 ]; then
      echo "### Verification reports"
      echo ""
      for report_file in \
        "${EVIDENCE_DIR}/issue-agent-verification.md" \
        "${EVIDENCE_DIR}/issue-agent-verification.json" \
        "$VERIFICATION_CASE_FILE"; do
        [ -s "$report_file" ] || continue
        report_base="$(basename "$report_file")"
        echo "- [${VERIFY_CASE_JOB_ID}-${report_base}](${container_url}/${report_prefix}/${VERIFY_CASE_JOB_ID}-${report_base})"
      done
      echo ""
    fi

    if [ "$video_taken" -gt 0 ]; then
      echo "### Videos"
      echo ""
      if [ "$video_total" -gt "$video_taken" ]; then
        echo "_Showing first ${video_taken} of ${video_total} videos._"
        echo ""
      fi
      for file in "$video_staging"/*; do
        [ -e "$file" ] || continue
        base="$(basename "$file")"
        echo "- [${base}](${container_url}/${video_prefix}/${base})"
      done
      echo ""
    fi

    if [ "$observation_taken" -gt 0 ]; then
      echo "### Observations"
      echo ""
      if [ "$observation_total" -gt "$observation_taken" ]; then
        echo "_Showing first ${observation_taken} of ${observation_total} observations._"
        echo ""
      fi
      for file in "$observation_staging"/*.json; do
        [ -e "$file" ] || continue
        base="$(basename "$file")"
        echo "- [${base}](${container_url}/${observation_prefix}/${base})"
      done
      echo ""
    fi

    if [ "$screenshot_taken" -gt 0 ]; then
      echo "### Screenshots"
      echo ""
      if [ "$screenshot_total" -gt "$screenshot_taken" ]; then
        echo "_Showing first ${screenshot_taken} of ${screenshot_total} screenshots._"
        echo ""
      fi
      for file in "$screenshot_staging"/*.png; do
        [ -e "$file" ] || continue
        base="$(basename "$file")"
        echo "#### ${base%.png}"
        echo ""
        echo "![${base%.png}](${container_url}/${screenshot_prefix}/${base})"
        echo ""
      done
    fi
  } >"$SCREENSHOTS_MD"
  rm -rf "$screenshot_staging" "$video_staging" "$observation_staging"
}

write_summary() {
  if [ -s "${EVIDENCE_DIR}/issue-agent-verification.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-verification.md" "$SUMMARY_MD"
    return 0
  fi
  if [ -s "${EVIDENCE_DIR}/issue-agent-implementation.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-implementation.md" "$SUMMARY_MD"
    return 0
  fi
  if [ -s "${EVIDENCE_DIR}/issue-agent-test-plan.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.md" "$SUMMARY_MD"
  fi
}

# Human-readable digest of the case verdict for the step log. The structured
# JSON still flows to glimmung via native_completed — this is what a person
# scanning the step page reads first: what was being verified, what was
# observed, and why it failed (when it did). One fact per line.
emit_verification_digest() {
  jq -r '
    def trunc: tostring | if length > 500 then .[:500] + "…" else . end;
    (.metadata.verification_case // {}) as $case
    | "=== verification case \($case.id // $case.job_id // "unknown"): \((.status // "?") | ascii_upcase) ===",
      (if ($case.must_show // "") != "" then "expected (must_show): \($case.must_show | trunc)" else empty end),
      (if ($case.url_path // "") != "" then
        "surface: \($case.url_path)\(if ($case.trigger_event // "") != "" then "  trigger: \($case.trigger_event)" else "" end)"
      else empty end),
      (if (.failure.observed // "") != "" then
        "observed: \(.failure.observed | trunc)\(if (.failure.where // "") != "" then "  [\(.failure.where)]" else "" end)"
      else empty end),
      (if (.failure.suspected_cause // "") != "" then
        "suspected cause: \(.failure.suspected_cause)\(if (.failure.cause_detail // "") != "" then " — \(.failure.cause_detail | trunc)" else "" end)"
      else empty end),
      (if ((.reasons // []) | length) > 0 then "reasons:", ((.reasons // [])[] | "  - \(trunc)") else empty end),
      (if ((.evidence_refs // []) | length) > 0 then "evidence:", ((.evidence_refs // [])[] | "  - \(.)") else empty end)
  ' "$VERIFICATION_JSON" 2>/dev/null || true
}

emit() {
  if [ ! -s "$VERIFICATION_JSON" ]; then
    add_reason "verification result was not produced"
    write_verification "error"
  fi
  # Rebuild cross-step outputs from durable evidence state: managed per-step
  # mode runs emit in its own invocation, so /tmp scratch written by finalize
  # or upload-screenshots is not visible here. EVIDENCE_DIR is.
  write_summary
  write_evidence_artifacts "$EVIDENCE_ARTIFACTS" "$EVIDENCE_REFS"
  emit_verification_digest
  jq -c . "$VERIFICATION_JSON"

  native_completed \
    "{}" \
    "$(cat "$VERIFICATION_JSON")" \
    "$(cat "$SCREENSHOTS_MD" 2>/dev/null || true)" \
    "$(cat "$SUMMARY_MD" 2>/dev/null || true)" \
    "$(cat "$EVIDENCE_ARTIFACTS")"

  # Per-case teardown. The verdict + evidence are already emitted/uploaded,
  # so free this case's working set before the next case reuses the
  # long-lived outer pod. Without this the single verify pod accumulates
  # all N cases' videos, extracted evidence tarballs, and pod logs and is
  # evicted (ephemeral storage) or OOM-killed — the run 14.1 failure mode.
  # The top-of-script mkdir re-creates the structure for the next case.
  stop_session_keepalive
  rm -rf "${EVIDENCE_DIR:?}/"* 2>/dev/null || true
  : >"$POD_LOG" 2>/dev/null || true
  : >"$EVENTS_LOG" 2>/dev/null || true
}

# Test seam: the contract harness sources this file to exercise the case
# functions directly against stubbed tools (see
# scripts/test-glimmung-native-contract.sh). Sourcing must define the
# functions and globals without running any step.
if [ "${AMBIENCE_VERIFY_SOURCE_ONLY:-}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

if native_selected_step; then
  case "${GLIMMUNG_STEP_SLUG}" in
    *-case-[0-9][0-9])
      export GLIMMUNG_STEP_SLUG="${GLIMMUNG_STEP_SLUG%-case-[0-9][0-9]}"
      ;;
  esac
  native_run_selected_step \
    "clone" clone_repo \
    "prepare" prepare_context \
    "run-verification" run_llm_record \
    "collect" collect_evidence \
    "finalize" finalize \
    "upload-screenshots" upload_evidence \
    "emit" emit
  exit $?
fi

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step "run-verification" run_llm_record
native_step "collect" collect_evidence
native_step "finalize" finalize
native_step_allow_failure "upload-screenshots" upload_evidence || true
native_step "emit" emit
native_assert_resume_satisfied
