#!/usr/bin/env bash
#
# Glimmung phase: test-plan
#
# Spawns a read-only LLM pod that reads the issue and produces an evidence
# specification (issue-agent-test-plan.json). Runs in parallel with the
# implement phase — does NOT receive or produce code changes.
#
# Feature types (AMBIENCE_FEATURE_TYPE): the workflow registration declares
# what kind of feature this workflow delivers, and the type selects the case
# source. A type with a repo-versioned standing case
# (.github/agent/standing-cases/<type>.json) does NOT generate a plan: the
# feature's surface (route, name, schema) does not exist at plan time, so a
# generated plan could only guess it — ambience#167 run 11 is the motivating
# failure (plan-invented session_config knobs the parallel implementation
# never defined). The stage still runs so the workflow shape stays total:
# every step logs SKIPPED and the standing case becomes the plan, validated
# by the exact same lint gates a generated plan must pass. Types without a
# standing case file keep the generated path unchanged.
#
# Outputs emitted to glimmung:
#   test_plan  — issue-agent-test-plan.json serialised as a JSON string.
#                Consumed by the verify phase to know what evidence to capture.
#                In standing mode this is the repo's standing case wrapped in
#                a plan envelope (case_source: "standing").

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

native_init
native_require_env \
  GLIMMUNG_RUN_ID \
  GLIMMUNG_INPUT_VALIDATION_URL \
  GLIMMUNG_INPUT_NAMESPACE

REPO_SLUG="${AMBIENCE_REPO_SLUG:-romaine-life/ambience}"
REPO_DIR="${AMBIENCE_REPO_DIR:-/workspace/ambience}"
WORKFLOW_REF="$(native_workflow_checkout_ref)"
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
JOB_NAME="agent-${RUN_SLUG}-tp-${ATTEMPT_INDEX}"
CONFIG_MAP_NAME="agent-config-test-plan"

# Gestalt cap: at most this many judged cases (cases carrying a non-empty
# must_show) per plan. Verification judgment is a holistic look at a handful
# of moments, not a checklist sweep — every additional judged case dilutes
# attention and adds an independent chance of a prose claim being misread.
# Mechanical-only cases (no must_show) are wrapper-enforced and stay capped
# only by the existing 10-case total.
MAX_JUDGED_CASES=3

# Standing case source. The feature type comes from the workflow registration
# (job env), and the standing case file is read from the WORKFLOW checkout —
# never the implementation branch — so the case the run is judged by cannot be
# edited by the run's own implementation agent.
FEATURE_TYPE="${AMBIENCE_FEATURE_TYPE:-}"
STANDING_CASE_FILE="${AMBIENCE_STANDING_CASE_FILE:-${REPO_DIR}/.github/agent/standing-cases/${FEATURE_TYPE}.json}"

standing_case_source() {
  [ -n "$FEATURE_TYPE" ] && [ -s "$STANDING_CASE_FILE" ]
}

# A declared feature type whose standing case file is absent is a
# misconfiguration, not a license to silently fall back to plan generation.
assert_feature_type_resolvable() {
  if [ -n "$FEATURE_TYPE" ] && [ ! -s "$STANDING_CASE_FILE" ]; then
    echo "feature_type=${FEATURE_TYPE} declares a standing case source but ${STANDING_CASE_FILE} is missing or empty" >&2
    return 1
  fi
  return 0
}

standing_skip() {
  local step="$1"
  echo "SKIPPED ${step} (feature_type=${FEATURE_TYPE}): test-plan generation does not run; the standing case ${STANDING_CASE_FILE#"${REPO_DIR}"/} is the case source"
}

ISSUE_TITLE="${GLIMMUNG_ISSUE_TITLE:-Glimmung issue ${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
ISSUE_NUMBER="${GLIMMUNG_ISSUE_NUMBER:-}"
ISSUE_PROJECT="${GLIMMUNG_PROJECT:-ambience}"
ISSUE_REFERENCE="${ISSUE_PROJECT}#${ISSUE_NUMBER:-${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}}"
if [ -n "$ISSUE_NUMBER" ]; then
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/projects/${ISSUE_PROJECT}/issues/${ISSUE_NUMBER}"
else
  ISSUE_URL="${GLIMMUNG_BASE_URL:-https://glimmung.romaine.life}/issues/${ISSUE_PROJECT}/${GLIMMUNG_ISSUE_ID:-${GLIMMUNG_RUN_ID}}"
fi

PLAN_EXIT_CODE=0
TEST_PLAN_JSON="/tmp/test-plan.json"
TEST_PLAN_MD="/tmp/test-plan.md"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="${AMBIENCE_EVIDENCE_DIR:-/tmp/evidence}"
POD_LOG="/tmp/agent-pod.log"
PLAN_EXIT_CODE_FILE="/tmp/test-plan-exit-code"
PROXY_IP_FILE="/tmp/test-plan-proxy-ip"
ISSUE_CONTRACT_FILE="${EVIDENCE_DIR}/issue-agent-contract.json"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
printf '0\n' >"$PLAN_EXIT_CODE_FILE"
mkdir -p "$EVIDENCE_DIR/screenshots" "$EVIDENCE_DIR/videos"

clone_repo() {
  assert_feature_type_resolvable
  if standing_case_source; then
    standing_skip "clone (managed checkout already provides the workflow ref)"
    return 0
  fi
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" "$WORKFLOW_REF"
}

copy_claude_ca() {
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
  assert_feature_type_resolvable
  if standing_case_source; then
    standing_skip "prepare (no agent infrastructure is provisioned)"
    return 0
  fi
  native_azure_login
  native_install_preview_package "${REPO_DIR}/mcp"
  copy_claude_ca
  native_prepare_codex_credentials_secret "$NAMESPACE"

  PROXY_IP="$(kubectl -n "$CLAUDE_NAMESPACE" get svc claude-api-proxy -o jsonpath='{.spec.clusterIP}')"
  if [ -z "$PROXY_IP" ]; then
    echo "claude-api-proxy Service not found in ${CLAUDE_NAMESPACE}" >&2
    return 1
  fi
  export PROXY_IP
  printf '%s\n' "$PROXY_IP" >"$PROXY_IP_FILE"

  prepare_agent_github_token() {
    local token
    token="$(native_github_token)"
    kubectl -n "$NAMESPACE" create secret generic agent-github-token \
      --from-literal=token="$token" \
      --dry-run=client -o yaml | kubectl apply -f -
  }
  prepare_agent_github_token

  if [ -n "${GLIMMUNG_INPUT_ISSUE_CONTRACT:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" | jq -r . >"$ISSUE_CONTRACT_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" >"$ISSUE_CONTRACT_FILE"
    echo "staged issue-contract JSON ($(wc -c <"$ISSUE_CONTRACT_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_ISSUE_CONTRACT not set; test-plan will proceed from issue context only"
  fi

  local args=(
    --from-file=prompt-test-plan.md="${REPO_DIR}/.github/agent/prompt-test-plan.md"
  )
  [ -s "$ISSUE_CONTRACT_FILE" ] && args+=(--from-file="issue-agent-contract.json=${ISSUE_CONTRACT_FILE}")
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
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
}

run_llm() {
  assert_feature_type_resolvable
  if standing_case_source; then
    standing_skip "run-test-plan (no test-plan agent is launched)"
    return 0
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
      --branch-name "glimmung/${GLIMMUNG_RUN_ID}" \
      --proxy-ip "$PROXY_IP" \
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
      --agent-container-image "$(native_agent_container_image)" \
      --repo-slug "$REPO_SLUG" \
      --stage "test-plan" \
      --config-map-name "$CONFIG_MAP_NAME" \
      --agent-runtime-json "${GLIMMUNG_AGENT_RUNTIME_JSON:-}"
    native_emit_inner_job_marker "$NAMESPACE" "$JOB_NAME" helper test-plan-agent
    python3 -m ambience_preview.cli wait-agent-job \
      --namespace "$NAMESPACE" \
      --job-name "$JOB_NAME" \
      --timeout-seconds "${AGENT_TEST_PLAN_TIMEOUT_SECONDS:-900}"
  )
}

run_llm_record() {
  native_record_exit_code "$PLAN_EXIT_CODE_FILE" run_llm
  PLAN_EXIT_CODE="$(native_read_exit_code "$PLAN_EXIT_CODE_FILE")"
}

plan_exit_code() {
  native_read_exit_code "$PLAN_EXIT_CODE_FILE"
}

collect_evidence() {
  if standing_case_source; then
    standing_skip "collect (no verifier pod exists)"
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

finalize() {
  assert_feature_type_resolvable
  if standing_case_source; then
    # Synthesize the plan envelope from the repo's standing case and stage it
    # exactly where the test-plan agent would have written its output. The
    # whole lint pipeline below then applies verbatim — the standing case is
    # held to the same claim-vocabulary gates as a generated plan, so a
    # repo edit that breaks the case fails this step with the named lint.
    # (stderr: validate-only mode reserves stdout for the plan JSON.)
    standing_skip "finalize input (standing case becomes the plan)" >&2
    if ! jq -e . "$STANDING_CASE_FILE" >/dev/null 2>&1; then
      jq -n --arg path "${STANDING_CASE_FILE#"${REPO_DIR}"/}" \
        '{schema_version:1,status:"fail",abort_reason:"malformed_standing_case",
          summary:("standing case file is not valid JSON: " + $path)}' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi
    jq -n \
      --arg feature_type "$FEATURE_TYPE" \
      --arg path "${STANDING_CASE_FILE#"${REPO_DIR}"/}" \
      --slurpfile standing "$STANDING_CASE_FILE" \
      '{
        schema_version: 1,
        status: "pass",
        case_source: "standing",
        feature_type: $feature_type,
        summary: ("Standing acceptance case for feature_type=" + $feature_type
          + " from " + $path
          + ". Test-plan generation was skipped: the feature surface does not exist at plan time, so the verifier binds this case to the implementation ui_hint and judges the capture against the issue text."),
        required_evidence: [$standing[0]]
      }' >"${EVIDENCE_DIR}/issue-agent-test-plan.json"
    {
      printf '## Standing case source\n\n'
      printf -- '- feature_type: `%s`\n' "$FEATURE_TYPE"
      printf -- '- case: `%s`\n' "${STANDING_CASE_FILE#"${REPO_DIR}"/}"
      printf -- '- Test-plan generation skipped by the workflow feature type; the verifier judges the capture against the issue text, using the implementation ui_hint for discovery only.\n'
    } >"${EVIDENCE_DIR}/issue-agent-test-plan.md"
  fi
  if [ -f "${EVIDENCE_DIR}/issue-agent-test-plan.json" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.json" "$TEST_PLAN_JSON"
  fi
  if [ -f "${EVIDENCE_DIR}/issue-agent-test-plan.md" ]; then
    cp "${EVIDENCE_DIR}/issue-agent-test-plan.md" "$TEST_PLAN_MD"
    cp "$TEST_PLAN_MD" "$SUMMARY_MD"
  fi
  if [ ! -s "$TEST_PLAN_JSON" ]; then
    local exit_code
    exit_code="$(plan_exit_code)"
    if [ "$exit_code" -ne 0 ]; then
      jq -n --arg reason "test-plan pod exited with ${exit_code}" \
        '{schema_version:1,status:"fail",abort_reason:$reason}' >"$TEST_PLAN_JSON"
    else
      jq -n '{schema_version:1,status:"fail",abort_reason:"test-plan pod produced no output"}' \
        >"$TEST_PLAN_JSON"
    fi
  elif [ "$(jq -r '.status // "missing"' "$TEST_PLAN_JSON" 2>/dev/null || echo missing)" = "pass" ]; then
    local normalized_json unsupported_media_count unsupported_media
    normalized_json="$(mktemp)"
    if ! jq '
        def normalized_kind:
          (.kind // "" | ascii_downcase) as $kind
          | if (["animation", "webm", "movie", "recording"] | index($kind)) then "video"
            elif (["image", "still"] | index($kind)) then "screenshot"
            else $kind
            end;
        # Every /dev/<effect> case runs in a named, isolated dev session so
        # the verifier can pin its config (dev sessions randomize knobs on
        # creation). When the plan omits ?session=, derive a deterministic
        # one from the case id so capture, trigger, and pin enforcement all
        # address the same session.
        def session_for_id:
          ((.id // "case") | ascii_downcase | gsub("[^a-z0-9_-]"; "-"));
        def with_session:
          if ((.url_path // "") | test("^/dev/[A-Za-z0-9_-]+"))
             and (((.url_path // "") | test("[?&]session=")) | not)
          then
            .url_path = (.url_path
              + (if ((.url_path // "") | contains("?")) then "&" else "?" end)
              + "session=" + session_for_id)
          else .
          end;
        .required_evidence = ((.required_evidence // []) | map(.kind = normalized_kind | with_session))
      ' "$TEST_PLAN_JSON" >"$normalized_json"; then
      jq -n '{
        schema_version: 1,
        status: "fail",
        abort_reason: "malformed_test_plan_json",
        summary: "Test plan JSON could not be parsed by the native wrapper."
      }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      rm -f "$normalized_json"
      return 0
    fi
    mv "$normalized_json" "$TEST_PLAN_JSON"

    local evidence_count
    evidence_count="$(jq -r '(.required_evidence // []) | length' "$TEST_PLAN_JSON" 2>/dev/null || echo 0)"
    if [ "$evidence_count" -gt 10 ]; then
      jq -n \
        --argjson evidence_count "$evidence_count" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "too_many_required_evidence",
          summary: "Test plan produced too many required evidence cases for the bounded verifier.",
          required_evidence_count: $evidence_count
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi
    if [ "$evidence_count" -eq 0 ]; then
      jq -n '{
        schema_version: 1,
        status: "fail",
        abort_reason: "no_required_media_evidence",
        summary: "Test plan passed without any browser media evidence. Ambience LLM verification requires at least one screenshot or video case."
      }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    unsupported_media="$(
      jq -r '
        (.required_evidence // [])[]
        | select(.kind != "video" and .kind != "screenshot")
        | ((.id // "<missing-id>") + ":" + (.kind // "<missing-kind>"))
      ' "$TEST_PLAN_JSON" 2>/dev/null || true
    )"
    unsupported_media_count="$(printf '%s\n' "$unsupported_media" | sed '/^$/d' | wc -l | tr -d ' ')"
    if [ "${unsupported_media_count:-0}" -gt 0 ]; then
      jq -n \
        --argjson unsupported "$(printf '%s\n' "$unsupported_media" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "unsupported_required_evidence_kind",
          summary: "Test plan included non-media verification cases. Ambience LLM verification only accepts screenshot and video evidence; deterministic checks belong in PR CI.",
          unsupported_required_evidence: $unsupported
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # Closed claim vocabulary (ambience#167 runs 9.1/10.1). must_show is one
    # gestalt judgment clause — what the artifact should LOOK like — not a
    # measurement protocol. Digits, comparator phrases, and camelCase
    # identifier tokens each make a prose claim decidable only by counting,
    # measuring, or reaching into effect-internal state, which an LLM judge
    # cannot do reliably (and which the parallel, mutually-blind
    # implementation never promised). Decidable expectations belong in the
    # structured fields the wrapper enforces mechanically: session_config,
    # trigger_event, terminal_lifecycle. Kebab-case trigger names are prose-
    # safe and allowed.
    local unverifiable_must_show
    unverifiable_must_show="$(
      jq -r '
        (.required_evidence // [])[]
        | (.id // "<missing-id>") as $id
        | (.must_show // null) as $ms
        | if $ms == null then empty
          elif ($ms | type) != "string" then
            $id + ": must_show is not a string (" + ($ms | type) + ")"
          elif $ms == "" then empty
          elif ($ms | test("[0-9]")) then
            $id + ": digit \"" + ($ms | match("[0-9]+").string) + "\""
          elif ($ms | test("(?:>=|<=)|\\b(?:no more than|at least|at most|more than|less than|exactly|within|between)\\b"; "i")) then
            $id + ": comparator \"" + ($ms | match("(?:>=|<=)|\\b(?:no more than|at least|at most|more than|less than|exactly|within|between)\\b"; "i").string) + "\""
          elif ($ms | test("[a-z]+[A-Z][A-Za-z]*")) then
            $id + ": camelCase identifier \"" + ($ms | match("[a-z]+[A-Z][A-Za-z]*").string) + "\""
          else empty
          end
      ' "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:must_show_vocabulary_guard'
    )"
    if [ -n "$(printf '%s\n' "$unverifiable_must_show" | sed '/^$/d')" ]; then
      jq -n \
        --arg summary "Test plan must_show clauses contain digits, comparator phrases, or camelCase identifier tokens. must_show is a single gestalt judgment: judge the look; don't measure — move decidable claims to structured fields (session_config, trigger_event, terminal_lifecycle)." \
        --argjson invalid "$(printf '%s\n' "$unverifiable_must_show" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "unverifiable_must_show",
          summary: $summary,
          unverifiable_must_show_cases: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # Video exists to be judged over time; a video case without a must_show
    # has no claim for the verifier to judge. Mechanical-only cases use
    # screenshot/observation evidence instead.
    local video_without_judgment
    video_without_judgment="$(
      jq -r '
        (.required_evidence // [])[]
        | select((.kind == "video") and ((.must_show // "") == ""))
        | (.id // "<missing-id>")
      ' "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:video_judgment_guard'
    )"
    if [ -n "$(printf '%s\n' "$video_without_judgment" | sed '/^$/d')" ]; then
      jq -n \
        --arg summary "Test plan declared video cases without a must_show. Video evidence exists to be judged; a case with nothing to judge is mechanical-only and uses screenshot/observation evidence." \
        --argjson invalid "$(printf '%s\n' "$video_without_judgment" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "video_requires_judgment",
          summary: $summary,
          video_without_judgment_cases: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # must_show is optional: a case without one is mechanical-only and is
    # verified entirely by the wrapper. It must declare at least one
    # mechanical expectation (trigger_event / terminal_lifecycle /
    # session_config) or there is nothing to verify at all.
    local empty_cases
    empty_cases="$(
      jq -r '
        (.required_evidence // [])[]
        | select(
            ((.must_show // "") == "")
            and ((.trigger_event // "") == "")
            and ((.terminal_lifecycle // "") == "")
            and (((.session_config // {}) | (if type == "object" then length else 0 end)) == 0)
          )
        | (.id // "<missing-id>")
      ' "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:empty_case_guard'
    )"
    if [ -n "$(printf '%s\n' "$empty_cases" | sed '/^$/d')" ]; then
      jq -n \
        --arg summary "Test plan declared cases with no must_show and no mechanical expectation. A mechanical-only case must declare at least one of trigger_event, terminal_lifecycle, or session_config so the wrapper has something to enforce." \
        --argjson invalid "$(printf '%s\n' "$empty_cases" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "empty_case",
          summary: $summary,
          empty_cases: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # Gestalt cap: at most MAX_JUDGED_CASES judged cases per plan. Fails
    # closed when the judged-case count cannot be computed.
    local judged_count judged_case_ids
    judged_count="$(
      jq -r '[(.required_evidence // [])[] | select((.must_show // "") != "")] | length' \
        "$TEST_PLAN_JSON" 2>/dev/null || printf 'invalid'
    )"
    case "$judged_count" in
      ''|*[!0-9]*) judged_count="invalid" ;;
    esac
    if [ "$judged_count" = "invalid" ] || [ "$judged_count" -gt "$MAX_JUDGED_CASES" ]; then
      judged_case_ids="$(
        jq -r '(.required_evidence // [])[] | select((.must_show // "") != "") | (.id // "<missing-id>")' \
          "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:judged_case_guard'
      )"
      jq -n \
        --arg summary "Test plan has ${judged_count} judged cases (non-empty must_show); the limit is MAX_JUDGED_CASES=${MAX_JUDGED_CASES}. Judgment is a gestalt look at a few key moments — keep at most ${MAX_JUDGED_CASES} judged cases and express the rest as mechanical-only cases (trigger_event / terminal_lifecycle / session_config, no must_show)." \
        --arg judged_count "$judged_count" \
        --argjson max_judged_cases "$MAX_JUDGED_CASES" \
        --argjson judged "$(printf '%s\n' "$judged_case_ids" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "too_many_judged_cases",
          summary: $summary,
          max_judged_cases: $max_judged_cases,
          judged_case_count: $judged_count,
          judged_cases: $judged
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # A trigger claim judged against the default background cadence is
    # undecidable — ambience#167 run 9.1: a "single lantern, no cluster"
    # claim drowned in the session's default emit_every/release_pulse_p
    # activity, so the judge could not tell the triggered lantern from the
    # ambient ones. Every case that fires a trigger_event must pin a
    # quiescent baseline via session_config (suppress the competing cadence
    # knobs: emission rates near zero, pulse probabilities 0) so the
    # triggered behavior is isolated from background noise.
    local trigger_without_baseline
    trigger_without_baseline="$(
      jq -r '
        (.required_evidence // [])[]
        | select(
            ((.trigger_event // "") != "")
            and (((.session_config // {}) | (if type == "object" then length else 0 end)) == 0)
          )
        | (.id // "<missing-id>")
      ' "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:trigger_baseline_guard'
    )"
    if [ -n "$(printf '%s\n' "$trigger_without_baseline" | sed '/^$/d')" ]; then
      jq -n \
        --arg summary "Test plan declared trigger_event cases without a session_config baseline. A trigger judged against default background cadence is undecidable; pin a quiescent baseline (suppress competing cadence knobs: emission rates near zero, pulse probabilities 0) so the triggered behavior is isolated." \
        --argjson invalid "$(printf '%s\n' "$trigger_without_baseline" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "trigger_case_without_baseline",
          summary: $summary,
          trigger_cases_without_baseline: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # Terminal lifecycle claims assert the closed lifecycle contract
    # (ambience#174). The retired terminal_state_path/terminal_state_equals
    # fields probed effect-internal state names and are rejected outright;
    # terminal_lifecycle must be one of the enum values.
    local invalid_terminal
    invalid_terminal="$(
      jq -r '
        (.required_evidence // [])[]
        | (.terminal_lifecycle // null) as $tl
        | select(
            has("terminal_state_path") or has("terminal_state_equals")
            or (has("terminal_lifecycle") and (
                 ($tl | type) != "string"
                 or ((["intro", "running", "ending", "ended"] | index($tl)) == null)
               ))
          )
        | (.id // "<missing-id>")
      ' "$TEST_PLAN_JSON" 2>/dev/null || printf 'jq_error:terminal_lifecycle_guard'
    )"
    if [ -n "$(printf '%s\n' "$invalid_terminal" | sed '/^$/d')" ]; then
      jq -n \
        --argjson invalid "$(printf '%s\n' "$invalid_terminal" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "invalid_terminal_lifecycle",
          summary: "Test plan used retired terminal_state_path/terminal_state_equals or an invalid terminal_lifecycle. Terminal claims assert the lifecycle contract: terminal_lifecycle one of intro|running|ending|ended.",
          invalid_terminal_lifecycle_cases: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
      return 0
    fi

    # session_config must be a flat object of numeric knob overrides. Knob
    # names are validated against the live effect schema at verify time;
    # shape problems fail here so a malformed plan never reaches a verifier.
    local invalid_session_config
    invalid_session_config="$(
      jq -r '
        (.required_evidence // [])[]
        | select(has("session_config"))
        | select(
            ((.session_config | type) != "object")
            or ((.session_config | to_entries | map(.value | type == "number") | all) | not)
          )
        | (.id // "<missing-id>")
      ' "$TEST_PLAN_JSON" 2>/dev/null || true
    )"
    if [ -n "$(printf '%s\n' "$invalid_session_config" | sed '/^$/d')" ]; then
      jq -n \
        --argjson invalid "$(printf '%s\n' "$invalid_session_config" | sed '/^$/d' | jq -R . | jq -s .)" \
        '{
          schema_version: 1,
          status: "fail",
          abort_reason: "invalid_session_config",
          summary: "Test plan declared session_config that is not a flat object of numeric knob overrides.",
          invalid_session_config_cases: $invalid
        }' >"$TEST_PLAN_JSON"
      printf '1\n' >"$PLAN_EXIT_CODE_FILE"
    fi
  fi
}

if [ "${AMBIENCE_TEST_PLAN_VALIDATE_ONLY:-}" = "1" ]; then
  finalize
  cat "$TEST_PLAN_JSON"
  exit "$(plan_exit_code)"
fi

emit() {
  local test_plan_str
  test_plan_str="$(cat "$TEST_PLAN_JSON")"
  # Emit the verification case count as a first-class phase output so the
  # verify phase sizes its sequential per-case job set from the plan,
  # rather than recomputing it inside verify. Bounded to [0,10].
  local case_count
  case_count="$(printf '%s' "$test_plan_str" | jq -r '((.required_evidence // .cases // .test_cases // []) | length)' 2>/dev/null || echo 0)"
  case "$case_count" in ''|*[!0-9]*) case_count=0 ;; esac
  if [ "$case_count" -gt 10 ]; then case_count=10; fi
  local outputs
  outputs="$(jq -nc --argjson v "$test_plan_str" --argjson n "$case_count" '{test_plan: ($v | tostring), test_cases_count: $n}')"
  local summary
  summary="$(cat "$SUMMARY_MD" 2>/dev/null || true)"
  native_completed "$outputs" "null" "" "$summary"
  local exit_code
  exit_code="$(plan_exit_code)"
  if native_selected_step && [ "$exit_code" -ne 0 ]; then
    native_failed "test-plan pod exited with ${exit_code}"
    return "$exit_code"
  fi
}

if native_selected_step; then
  native_run_selected_step \
    "clone" clone_repo \
    "prepare" prepare_context \
    "run-test-plan" run_llm_record \
    "collect" collect_evidence \
    "finalize" finalize \
    "emit" emit
  exit $?
fi

native_step "clone" clone_repo
native_step "prepare" prepare_context
native_step "run-test-plan" run_llm_record
native_step "collect" collect_evidence
native_step "finalize" finalize
PLAN_EXIT_CODE="$(plan_exit_code)"
if [ "$PLAN_EXIT_CODE" -ne 0 ]; then
  native_failed "test-plan pod exited with ${PLAN_EXIT_CODE}"
  exit "$PLAN_EXIT_CODE"
fi
native_step "emit" emit
native_assert_resume_satisfied
