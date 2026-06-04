#!/usr/bin/env bash
# Glimmung phase: verify.
#
# Runs after both test-plan and implement. Each verify-case-NN job selects
# one required_evidence item from the test-plan JSON, mounts that single case
# in the verifier pod's configmap, and runs the verification LLM against the
# rebuilt validation env for that case only.
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
AGENT_CONTAINER_TAG="${AGENT_CONTAINER_TAG:-latest}"
CLAUDE_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_NAMESPACE:-tank-operator}"
CLAUDE_CA_NAMESPACE="${GLIMMUNG_INPUT_CLAUDE_CA_NAMESPACE:-${CLAUDE_CA_NAMESPACE:-tank-operator-sessions}}"

VALIDATION_URL="${GLIMMUNG_INPUT_VALIDATION_URL}"
NAMESPACE="${GLIMMUNG_INPUT_NAMESPACE}"
BRANCH_NAME="${GLIMMUNG_INPUT_BRANCH_NAME}"
RUN_SLUG="$(printf '%s' "$GLIMMUNG_RUN_ID" | tr '[:upper:]' '[:lower:]')"
ATTEMPT_INDEX="${GLIMMUNG_ATTEMPT_INDEX:-0}"
VERIFY_CASE_JOB_ID="${GLIMMUNG_JOB_ID:-}"
case "$VERIFY_CASE_JOB_ID" in
  verify-case-[0-9][0-9]) ;;
  *)
    echo "GLIMMUNG_JOB_ID must be verify-case-NN for bounded verification, got '${VERIFY_CASE_JOB_ID:-unset}'" >&2
    exit 1
    ;;
esac
VERIFY_CASE_NUMBER="$((10#${VERIFY_CASE_JOB_ID#verify-case-}))"
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
SCREENSHOTS_MD="/tmp/screenshots.md"
SUMMARY_MD="/tmp/summary.md"
EVENTS_LOG="/tmp/agent-events.jsonl"
EVIDENCE_DIR="/tmp/evidence"
POD_LOG="/tmp/agent-pod.log"
VERIFY_EXIT_CODE_FILE="/tmp/verification-exit-code"
PROXY_IP_FILE="/tmp/verification-proxy-ip"
VERIFICATION_CASE_FILE="${EVIDENCE_DIR}/verification-case.json"
VERIFICATION_CASE_STATUS_FILE="/tmp/verification-case-status"
: >"$VERIFICATION_REASONS"
: >"$EVIDENCE_REFS"
printf '[]\n' >"$EVIDENCE_ARTIFACTS"
: >"$SCREENSHOTS_MD"
: >"$SUMMARY_MD"
: >"$EVENTS_LOG"
printf 'active\n' >"$VERIFICATION_CASE_STATUS_FILE"
mkdir -p "$EVIDENCE_DIR/screenshots" "$EVIDENCE_DIR/videos"

# Stage handoff files — written by prepare_context from glimmung inputs.
ISSUE_CONTRACT_FILE="${EVIDENCE_DIR}/issue-agent-contract.json"
TEST_PLAN_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.json"
TEST_PLAN_MD_FILE="${EVIDENCE_DIR}/issue-agent-test-plan.md"
IMPL_FILE="${EVIDENCE_DIR}/issue-agent-implementation.json"
IMPL_MD_FILE="${EVIDENCE_DIR}/issue-agent-implementation.md"

clone_repo() {
  native_clone_repo "$REPO_SLUG" "$REPO_DIR" main
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
  # Stage handoff artifacts from glimmung phase outputs into evidence dir.
  if [ -n "${GLIMMUNG_INPUT_ISSUE_CONTRACT:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" | jq -r . >"$ISSUE_CONTRACT_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_ISSUE_CONTRACT" >"$ISSUE_CONTRACT_FILE"
    echo "staged issue-contract JSON ($(wc -c <"$ISSUE_CONTRACT_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_ISSUE_CONTRACT not set; verify will proceed without issue-contract context"
  fi

  # GLIMMUNG_INPUT_TEST_PLAN is the test-plan JSON string; unwrap it.
  if [ -n "${GLIMMUNG_INPUT_TEST_PLAN:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" | jq -r . >"$TEST_PLAN_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_TEST_PLAN" >"$TEST_PLAN_FILE"
    echo "staged test-plan JSON ($(wc -c <"$TEST_PLAN_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_TEST_PLAN not set; verify will proceed without test-plan context"
  fi

  if [ -n "${GLIMMUNG_INPUT_IMPLEMENTATION:-}" ]; then
    printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" | jq -r . >"$IMPL_FILE" 2>/dev/null \
      || printf '%s' "$GLIMMUNG_INPUT_IMPLEMENTATION" >"$IMPL_FILE"
    echo "staged implementation JSON ($(wc -c <"$IMPL_FILE") bytes)"
  else
    echo "GLIMMUNG_INPUT_IMPLEMENTATION not set; verify will proceed without implementation context"
  fi

  select_verification_case
  if [ "$(verification_case_status)" != "active" ]; then
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

  local dest="/tmp/agent-prompt-context.md"
  : >"$dest"
  {
    echo "# Glimmung issue ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
    echo "URL: ${ISSUE_URL}"
    echo "Validation env: ${VALIDATION_URL}"
    echo "Glimmung run: ${GLIMMUNG_RUN_ID}"
    echo "Glimmung attempt index: ${ATTEMPT_INDEX:-unknown}"
    if [ -n "${GLIMMUNG_ISSUE_BODY:-}" ]; then
      echo ""
      echo "## Issue body"
      echo ""
      printf '%s\n' "$GLIMMUNG_ISSUE_BODY"
    fi
  } >>"$dest"

  local token
  token="$(native_github_token)"
  kubectl -n "$NAMESPACE" create secret generic agent-github-token \
    --from-literal=token="$token" \
    --dry-run=client -o yaml | kubectl apply -f -

  # Build the agent-config configmap with the prompt + all available handoff files.
  local args=(
    --from-file=prompt-verification.md="${REPO_DIR}/.github/agent/prompt-verification.md"
  )
  for f in "$ISSUE_CONTRACT_FILE" "$TEST_PLAN_FILE" "$TEST_PLAN_MD_FILE" "$IMPL_FILE" "$IMPL_MD_FILE" "$VERIFICATION_CASE_FILE"; do
    [ -s "$f" ] || continue
    base="$(basename "$f")"
    args+=(--from-file="${base}=${f}")
  done
  kubectl -n "$NAMESPACE" create configmap "$CONFIG_MAP_NAME" \
    "${args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

verification_case_status() {
  cat "$VERIFICATION_CASE_STATUS_FILE" 2>/dev/null || printf 'active'
}

write_verification_case_file() {
  local status="$1"
  local reason="${2:-}"
  local case_json="${3:-null}"
  printf '%s\n' "$status" >"$VERIFICATION_CASE_STATUS_FILE"
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
      --agent-container-tag "$AGENT_CONTAINER_TAG" \
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

collect_evidence() {
  if [ "$(verification_case_status)" != "active" ]; then
    echo "${VERIFY_CASE_JOB_ID} did not launch a verifier pod; skipping evidence collection"
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

enforce_issue_contract() {
  local contract="${ISSUE_CONTRACT_FILE}"
  if [ ! -s "$contract" ]; then
    add_reason "missing issue-contract JSON; cannot enforce public target contract"
    return 1
  fi

  local failed=0
  local slug dev_route schema_route
  slug="$(jq -r '.canonical_target.slug // ""' "$contract" 2>/dev/null || true)"
  dev_route="$(jq -r '.public_surface.dev_route // ""' "$contract" 2>/dev/null || true)"
  schema_route="$(jq -r '.public_surface.schema_route // ""' "$contract" 2>/dev/null || true)"

  check_get_route() {
    local label="$1"
    local route="$2"
    [ -n "$route" ] || return 0
    local status
    status="$(curl -sS -o /dev/null -w "%{http_code}" "${VALIDATION_URL}${route}" || printf '000')"
    case "$status" in
      2*|3*) ;;
      *)
        add_reason "issue contract: ${label} ${route} returned HTTP ${status}"
        failed=1
        ;;
    esac
  }

  check_get_route "dev route" "$dev_route"
  check_get_route "schema route" "$schema_route"

  if jq -e '(.public_surface.trigger_events // []) | length > 0' "$contract" >/dev/null 2>&1; then
    if [ -z "$slug" ]; then
      add_reason "issue contract: trigger_events declared but canonical_target.slug is empty"
      failed=1
    else
      while IFS= read -r event; do
        [ -n "$event" ] || continue
        local session status
        session="contract-$(printf '%s' "$event" | tr -c 'A-Za-z0-9_-' '-')"
        status="$(curl -sS -o /dev/null -w "%{http_code}" \
          -X POST "${VALIDATION_URL}/dev/trigger/${session}/${event}?effect=${slug}" || printf '000')"
        case "$status" in
          2*|3*) ;;
          *)
            add_reason "issue contract: trigger ${event} for effect ${slug} returned HTTP ${status}"
            failed=1
            ;;
        esac
      done < <(jq -r '.public_surface.trigger_events[]? // empty' "$contract" 2>/dev/null)
    fi
  fi

  while IFS= read -r forbidden; do
    [ -n "$forbidden" ] || continue
    local status
    status="$(curl -sS -o /dev/null -w "%{http_code}" "${VALIDATION_URL}/dev/${forbidden}" || printf '000')"
    case "$status" in
      2*|3*)
        add_reason "issue contract: forbidden public name /dev/${forbidden} unexpectedly exists"
        failed=1
        ;;
    esac
  done < <(jq -r '.forbidden_public_names[]? // empty' "$contract" 2>/dev/null)

  [ "$failed" -eq 0 ]
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
          else empty
          end
      ' || true
  )"
  local missing_files
  missing_files="$(
    jq -r '
      .evidence_results[]?
      | select(.status == "pass")
      | (.video?, .screenshot?)
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

write_evidence_artifacts() {
  local artifacts_out="$1"
  local refs_out="$2"
  local file_artifacts="/tmp/evidence-files.jsonl"
  local verifier="${EVIDENCE_DIR}/issue-agent-verification.json"
  local empty_verifier="/tmp/empty-verifier.json"
  : >"$file_artifacts"
  printf '{}\n' >"$empty_verifier"

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
    done < <(find "${EVIDENCE_DIR}/videos" -maxdepth 1 -type f \( -name '*.webm' -o -name '*.mp4' -o -name '*.mov' \) | sort)
  fi

  if [ ! -s "$verifier" ]; then
    verifier="$empty_verifier"
  fi

  jq -s --slurpfile verifier "$verifier" '
    def normalize_ref($ref): ($ref | tostring | sub("^/workspace/evidence/"; "") | sub("^/tmp/evidence/"; ""));
    def first_ref($item): normalize_ref($item.ref // $item.artifact_path // $item.url // "");
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
          (($v.evidence_results // [])[]? | select((.screenshot // "") != "") | {kind:"screenshot", ref:.screenshot, label:(.label // .id // ""), content_type:"image/png"})
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
  local cost
  cost="$(verification_cost)"
  write_evidence_artifacts "$EVIDENCE_ARTIFACTS" "$EVIDENCE_REFS"
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
    '{
      schema_version: 1,
      status: $status,
      reasons: $reasons,
      evidence_refs: $evidence_refs,
      evidence: $evidence,
      cost_usd: $cost_usd,
      prompt_version: "ambience-native-staged-v1",
      metadata: {
        run_id: $run_id,
        branch: $branch,
        validation_url: $validation_url,
        verification_case: {
          job_id: $verification_case,
          index: $verification_case_index,
          status: $verification_case_status
        }
      }
    }' >"$VERIFICATION_JSON"
  cat "$VERIFICATION_JSON"
}

finalize() {
  case "$(verification_case_status)" in
    skipped)
      write_verification "pass"
      return 0
      ;;
    plan_error)
      write_verification "fail"
      return 0
      ;;
  esac

  VERIFY_EXIT_CODE="$(verify_exit_code)"
  if [ "$VERIFY_EXIT_CODE" -ne 0 ]; then
    add_reason "verify pod exited with ${VERIFY_EXIT_CODE}; see native step logs"
    if [ -s "$POD_LOG" ]; then
      grep -E "::error::|Job failed|FATAL|panic:|aborted:|forbidden|exited without writing" "$POD_LOG" \
        | head -5 >>"$VERIFICATION_REASONS" || true
    fi
    write_verification "fail"
    return 0
  fi

  local verifier_status
  verifier_status="$(jq -r '.status // "missing"' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo missing)"
  if [ "$verifier_status" != "pass" ]; then
    add_reason "verifier reported status=${verifier_status} reason=$(jq -r '.abort_reason // ""' "${EVIDENCE_DIR}/issue-agent-verification.json" 2>/dev/null || echo "")"
    write_verification "fail"
    return 0
  fi

  if [ "$VERIFY_CASE_INDEX" -eq 0 ] && ! enforce_issue_contract; then
    write_verification "fail"
    return 0
  fi

  if ! enforce_evidence_contract; then
    write_verification "fail"
    return 0
  fi

  write_verification "pass"
  write_summary
}

upload_evidence() {
  local storage_account="${AGENT_SCREENSHOT_STORAGE_ACCOUNT:-romaineglimmungartifacts}"
  local container="${AGENT_SCREENSHOT_CONTAINER:-artifacts}"
  local container_url="${AGENT_SCREENSHOT_CONTAINER_URL:-https://glimmung.romaine.life/v1/artifacts}"
  local max_screenshots="${MAX_SCREENSHOTS:-20}"
  local max_videos="${MAX_VIDEOS:-10}"
  local screenshot_prefix video_prefix screenshot_staging video_staging screenshot_total screenshot_taken video_total video_taken upload_ok
  screenshot_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/screenshots"
  video_prefix="runs/${GLIMMUNG_PROJECT}/${GLIMMUNG_RUN_ID}/videos"
  screenshot_staging="$(mktemp -d)"
  video_staging="$(mktemp -d)"
  screenshot_total=0
  screenshot_taken=0
  video_total=0
  video_taken=0
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
    done < <(find "${EVIDENCE_DIR}/videos" -maxdepth 1 -type f \( -name '*.webm' -o -name '*.mp4' -o -name '*.mov' \) | sort)

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

  if [ "$screenshot_total" -eq 0 ] && [ "$video_total" -eq 0 ]; then
    rm -rf "$screenshot_staging" "$video_staging"
    return 0
  fi

  {
    echo "## Evidence"
    echo ""
    if [ "$upload_ok" = "false" ]; then
      echo "_Evidence upload failed; see the Glimmung native run logs._"
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
  rm -rf "$screenshot_staging" "$video_staging"
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

emit() {
  if [ ! -s "$VERIFICATION_JSON" ]; then
    add_reason "verification result was not produced"
    write_verification "error"
  fi
  cat "$VERIFICATION_JSON"

  native_completed \
    "{}" \
    "$(cat "$VERIFICATION_JSON")" \
    "$(cat "$SCREENSHOTS_MD")" \
    "$(cat "$SUMMARY_MD")" \
    "$(cat "$EVIDENCE_ARTIFACTS")"
}

if native_selected_step; then
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
