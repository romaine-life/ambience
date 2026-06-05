#!/usr/bin/env bash
# Local visual self-check for an effect, for the implementation stage.
#
# Builds the in-progress code, serves it on localhost (never the shared
# validation env), records a WebM of /dev/<effect> while optionally firing a
# lifecycle/event trigger, and inspects the final frame. Use this to confirm a
# visual or temporal claim — especially a terminal lifecycle state like
# `ending` ("the gate goes dark and stays dark") — before writing a passing
# implementation JSON. `go build`/`go test` cannot see pixels.
#
# Usage:
#   scripts/agent/selfcheck-effect.sh <effect> [trigger-event] [record-ms]
# Examples:
#   scripts/agent/selfcheck-effect.sh magic-portal ending
#   scripts/agent/selfcheck-effect.sh magic-portal            # default render only
#
# Output (under /tmp, not committed):
#   /tmp/selfcheck-<effect>.webm        recorded clip
#   /tmp/selfcheck-<effect>-final.png   final frame to eyeball vs the contract
# Inspect the final frame against the issue contract's lifecycle resting_state.

set -Eeuo pipefail

EFFECT="${1:?usage: selfcheck-effect.sh <effect> [trigger-event] [record-ms]}"
EVENT="${2:-}"
RECORD_MS="${3:-12000}"
PORT="${SELFCHECK_PORT:-8765}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SESSION="selfcheck"
BASE="http://127.0.0.1:${PORT}"
WEBM="/tmp/selfcheck-${EFFECT}.webm"
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

capture_args=(
  --url "${BASE}/dev/${EFFECT}?session=${SESSION}"
  --output "${WEBM}"
  --wait-ms "${RECORD_MS}"
)
if [ -n "${EVENT}" ]; then
  capture_args+=(
    --trigger-url "${BASE}/dev/trigger/${SESSION}/${EVENT}?effect=${EFFECT}"
    --trigger-delay-ms 1500
  )
  echo "[selfcheck] recording ${RECORD_MS}ms, firing '${EVENT}' at 1.5s..."
else
  echo "[selfcheck] recording ${RECORD_MS}ms (default render, no trigger)..."
fi

node scripts/agent/capture-video.mjs "${capture_args[@]}"
node scripts/agent/inspect-video.mjs --file "${WEBM}" --screenshot "${SHOT}" --min-duration-ms "$((RECORD_MS - 1500))"

echo "[selfcheck] done."
echo "[selfcheck] clip:        ${WEBM}"
echo "[selfcheck] final frame:  ${SHOT}"
echo "[selfcheck] Eyeball the final frame against the contract's lifecycle resting_state for '${EVENT:-default}'."
