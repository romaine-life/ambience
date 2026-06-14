#!/usr/bin/env bash
# Local lifecycle self-check for an effect, for the implementation stage.
#
# Builds the in-progress code, serves it on localhost (never the shared
# validation env), and drives a terminal-lifecycle event through plain HTTP
# via /dev/observe — no browser, no Playwright. /dev/observe triggers the
# event, waits for the lifecycle contract predicate, holds it for hold-ticks,
# and returns a JSON trace plus a frozen grid frame. Use this to confirm a
# visual or temporal claim — especially a terminal lifecycle state like
# `ending` ("the gate goes dark and stays dark") — before writing a passing
# implementation JSON. `go build`/`go test` cannot see pixels; /dev/observe
# reads the grid directly without rendering a page.
#
# Browser evidence (video/screenshots against the rebuilt env) is the
# verification stage's job, captured through Glimmung's central capture tools.
# The leased slot browser cannot reach this pod-local server, so this self-check
# is intentionally browserless and stays on localhost HTTP.
#
# Usage:
#   scripts/agent/selfcheck-effect.sh <effect> <lifecycle-event> [lifecycle] [hold-ticks]
# Examples:
#   scripts/agent/selfcheck-effect.sh magic-portal ending           # lifecycle defaults to "ended"
#   scripts/agent/selfcheck-effect.sh magic-portal ending ended 12
#
# Output (under /tmp, not committed):
#   /tmp/selfcheck-<effect>-observe.json   the /dev/observe trace
#   /tmp/selfcheck-<effect>-final.png      frozen grid frame to eyeball vs the contract
# Confirm `applied` and `observed` are true and `lifecycle` matches the issue's
# described terminal resting state.

set -Eeuo pipefail

EFFECT="${1:?usage: selfcheck-effect.sh <effect> <lifecycle-event> [lifecycle] [hold-ticks]}"
EVENT="${2:?usage: selfcheck-effect.sh <effect> <lifecycle-event> [lifecycle] [hold-ticks]}"
LIFECYCLE="${3:-ended}"
HOLD_TICKS="${4:-12}"
PORT="${SELFCHECK_PORT:-8765}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SESSION="selfcheck"
BASE="http://127.0.0.1:${PORT}"
TRACE="/tmp/selfcheck-${EFFECT}-observe.json"
SHOT="/tmp/selfcheck-${EFFECT}-final.png"

cd "$ROOT"

echo "[selfcheck] building web wasm into the embedded asset dir..."
bash scripts/build-web-wasm.sh

echo "[selfcheck] starting local server on ${BASE} (in-progress build)..."
AMBIENCE_ADDR=":${PORT}" go run ./cmd/ambience >/tmp/selfcheck-server.log 2>&1 &
SERVER_PID=$!
cleanup() { kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; }
trap cleanup EXIT

echo "[selfcheck] waiting for readiness..."
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "${BASE}/"; then ready=1; break; fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "[selfcheck] server exited early; log:" >&2; cat /tmp/selfcheck-server.log >&2; exit 1
  fi
  sleep 0.5
done
[ "${ready:-}" = "1" ] || { echo "[selfcheck] server never became ready" >&2; cat /tmp/selfcheck-server.log >&2; exit 1; }

echo "[selfcheck] firing '${EVENT}' and waiting for lifecycle='${LIFECYCLE}' (hold ${HOLD_TICKS} ticks) via /dev/observe..."
observe_url="${BASE}/dev/observe?effect=${EFFECT}&session=${SESSION}&trigger=${EVENT}&lifecycle=${LIFECYCLE}&hold_ticks=${HOLD_TICKS}"
if ! curl -fsS -X POST "$observe_url" >"$TRACE"; then
  echo "[selfcheck] /dev/observe request failed; server log:" >&2; cat /tmp/selfcheck-server.log >&2; exit 1
fi

jq -e '.applied == true and .observed == true' "$TRACE" >/dev/null || {
  echo "[selfcheck] lifecycle '${LIFECYCLE}' was not observed/held — fix the code; trace:" >&2
  jq -c '{applied, observed, lifecycle, holdTicks, observedTick, heldUntilTick}' "$TRACE" >&2 || cat "$TRACE" >&2
  exit 1
}

# Pull the frozen grid frame named in the trace so the final state can be eyeballed.
observation_id="$(jq -r '.observationId // ""' "$TRACE")"
if [ -n "$observation_id" ]; then
  curl -fsS "${BASE}/dev/frame?effect=${EFFECT}&session=${SESSION}&observation=${observation_id}" -o "$SHOT" || true
fi

echo "[selfcheck] done."
echo "[selfcheck] trace:        ${TRACE}"
echo "[selfcheck] observed: $(jq -c '{applied, observed, lifecycle, holdTicks}' "$TRACE")"
[ -s "$SHOT" ] && echo "[selfcheck] final frame:  ${SHOT}"
echo "[selfcheck] Confirm the trace lifecycle and frozen frame match the contract's resting_state for '${EVENT}'."
