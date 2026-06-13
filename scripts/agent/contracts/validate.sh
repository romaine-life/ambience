#!/usr/bin/env bash
set -Eeuo pipefail

contract="${1:?usage: validate.sh <contract.json> <implementation.json> [repo_dir] [base_ref] [head_ref]}"
implementation="${2:?usage: validate.sh <contract.json> <implementation.json> [repo_dir] [base_ref] [head_ref]}"
repo_dir="${3:-$(pwd)}"
base_ref="${4:-origin/main}"
head_ref="${5:-HEAD}"

fail() {
  local reason="$1"
  local detail="$2"
  jq -n \
    --arg reason "$reason" \
    --arg detail "$detail" \
    '{status:"fail", abort_reason:$reason, detail:$detail}'
  printf 'implementation contract failed: %s: %s\n' "$reason" "$detail" >&2
  exit 1
}

[ -s "$contract" ] || fail "missing_contract" "contract file not found: $contract"
[ -s "$implementation" ] || fail "missing_implementation" "implementation file not found: $implementation"
jq . "$contract" >/dev/null || fail "invalid_contract" "contract is not valid JSON: $contract"
jq . "$implementation" >/dev/null || fail "invalid_implementation" "implementation is not valid JSON: $implementation"

feature_type="$(jq -r '.feature_type // ""' "$contract")"
impl_status="$(jq -r '.status // ""' "$implementation")"

if [ "$impl_status" != "pass" ]; then
  jq -n '{status:"pass", skipped:"implementation status is not pass"}'
  exit 0
fi

if [ "$feature_type" != "effect" ]; then
  jq -n --arg feature_type "$feature_type" '{status:"pass", skipped:"no validator for feature_type", feature_type:$feature_type}'
  exit 0
fi

route="$(jq -r '.ui_hint.route // ""' "$implementation")"
case "$route" in
  /dev/[a-z0-9_-]*)
    effect_slug="${route#/dev/}"
    ;;
  *)
    fail "missing_ui_hint" "effect contracts require ui_hint.route shaped as /dev/<effect>; got ${route:-<empty>}"
    ;;
esac

effect_snake="$(printf '%s' "$effect_slug" | tr '-' '_')"
if [ -z "$effect_snake" ]; then
  fail "missing_ui_hint" "could not derive effect_snake from route $route"
fi

if ! git -C "$repo_dir" rev-parse --verify "$base_ref" >/dev/null 2>&1; then
  fail "missing_base_ref" "base ref $base_ref is not available in $repo_dir"
fi
if ! git -C "$repo_dir" rev-parse --verify "$head_ref" >/dev/null 2>&1; then
  fail "missing_head_ref" "head ref $head_ref is not available in $repo_dir"
fi

changed="$(git -C "$repo_dir" diff --name-only "${base_ref}...${head_ref}" | sort -u)"
if [ -z "$changed" ]; then
  fail "empty_change" "no files changed relative to $base_ref"
fi

missing=()
while IFS= read -r template; do
  [ -n "$template" ] || continue
  path="${template//\{effect_snake\}/$effect_snake}"
  if ! printf '%s\n' "$changed" | grep -Fx -- "$path" >/dev/null; then
    missing+=("$path")
  fi
done < <(jq -r '.required_file_templates[]? // empty' "$contract")

while IFS= read -r path; do
  [ -n "$path" ] || continue
  if ! printf '%s\n' "$changed" | grep -Fx -- "$path" >/dev/null; then
    missing+=("$path")
  fi
done < <(jq -r '.required_touchpoints[]?.path // empty' "$contract")

if [ "${#missing[@]}" -gt 0 ]; then
  printf '%s\n' "${missing[@]}" >"/tmp/contract-missing.$$"
  detail="$(jq -Rsc 'split("\n")[:-1]' "/tmp/contract-missing.$$")"
  rm -f "/tmp/contract-missing.$$"
  fail "missing_required_touchpoints" "$detail"
fi

forbidden=()
while IFS=$'\t' read -r pattern reason; do
  [ -n "$pattern" ] || continue
  while IFS= read -r path; do
    case "$path" in
      $pattern)
        forbidden+=("${path}: ${reason}")
        ;;
    esac
  done <<<"$changed"
done < <(jq -r '.forbidden_paths[]? | [.pattern, (.reason // "")] | @tsv' "$contract")

if [ "${#forbidden[@]}" -gt 0 ]; then
  printf '%s\n' "${forbidden[@]}" >"/tmp/contract-forbidden.$$"
  detail="$(jq -Rsc 'split("\n")[:-1]' "/tmp/contract-forbidden.$$")"
  rm -f "/tmp/contract-forbidden.$$"
  fail "forbidden_touchpoint" "$detail"
fi

jq -n \
  --arg feature_type "$feature_type" \
  --arg effect_slug "$effect_slug" \
  --argjson changed "$(printf '%s\n' "$changed" | jq -Rsc 'split("\n")[:-1]')" \
  '{status:"pass", feature_type:$feature_type, effect_slug:$effect_slug, changed_files:$changed}'
