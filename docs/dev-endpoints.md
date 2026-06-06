# Dev endpoints

The `cmd/ambience` server exposes a handful of `/dev/...` routes for
per-session, isolated testing of effects without disturbing the
shared broadcast.

## Routes

| Route | Method | Purpose |
| --- | --- | --- |
| `/dev/<effect>` | GET | Per-session dev page. Loads the named effect and renders it via the shared client. Each new browser session gets its own isolated atmosphere. |
| `/dev/snapshot` | GET | JSON snapshot of the calling session's dev atmosphere. Includes `appliedEvents` — a bounded ring of `{tick,event}` for events actually applied to this session's sim. Use it to mechanically confirm a fired trigger reached the session you are observing, rather than inferring it from a single frame (a pristine, never-triggered sim can coincidentally match a resting look). |
| `/dev/events` | GET (SSE) | Per-session command stream (snapshot/config/trigger/scene/metric). |
| `/dev/config` | POST | Override the dev atmosphere's config. Body is the effect-specific config JSON or query-string-encoded knobs depending on the registered config parser. Returns 204 on success. |
| `/dev/trigger/<session>/<event>` | POST | Fire a discrete event in the named session. `session` is the dev-session identifier. To observe the effect, load the page with the matching `?session=<session>` — the dev page honors that query param so an external driver addresses the same session it triggers (a fired trigger only affects the page when both sides agree on the id). `event` is the effect-specific event name (e.g. `lightning-flash`, `ignite`, `gust`, `downpour`). |
| `/dev/observe` | POST | Verification-only lifecycle observer. It can trigger an event, wait for a lifecycle log marker and/or state predicate, require the predicate to hold for `hold_ticks`, then store a frozen grid frame and return a JSON trace with `appliedEvents`, ticks, config, final state, `observationId`, and `frameUrl`. |
| `/dev/frame` | GET | Returns an `image/png` for a frozen observation frame from `/dev/observe` via `?session=<session>&effect=<effect>&observation=<observationId>`. |
| `/effects/<effect>/schema` | GET | JSON schema for the effect's dev-panel knobs (used by the dev page to render controls). |

## Verification recipe

When validating a new effect through the agent flow, the typical
sequence is:

```sh
# 1. Record default behavior
node scripts/agent/capture-video.mjs \
  --url "$VALIDATION_URL/dev/<effect>" \
  --output /workspace/evidence/videos/dev-<effect>.webm \
  --wait-ms 6000
node scripts/agent/inspect-video.mjs \
  --file /workspace/evidence/videos/dev-<effect>.webm \
  --min-duration-ms 6000 \
  --screenshot /workspace/evidence/screenshots/dev-<effect>-frame.png

# 2. Trigger a discrete event in a named session and record it
SESSION=test1
node scripts/agent/capture-video.mjs \
  --url "$VALIDATION_URL/dev/<effect>?session=$SESSION" \
  --output /workspace/evidence/videos/dev-<effect>-flash.webm \
  --trigger-url "$VALIDATION_URL/dev/trigger/$SESSION/lightning-flash" \
  --wait-ms 6000
node scripts/agent/inspect-video.mjs \
  --file /workspace/evidence/videos/dev-<effect>-flash.webm \
  --min-duration-ms 6000 \
  --screenshot /workspace/evidence/screenshots/dev-<effect>-flash-frame.png

# 3. Override config to a known good preset
curl -s -X POST -H 'Content-Type: application/json' \
  --data-binary '@preset.json' \
  "$VALIDATION_URL/dev/config?session=$SESSION"

# 4. Optional still-frame evidence when useful
node scripts/agent/capture-screenshot.mjs \
  --url "$VALIDATION_URL/dev/<effect>?session=$SESSION" \
  --output /workspace/evidence/screenshots/dev-<effect>-flash.png \
  --full-page --wait-ms 1000

# 5. Terminal lifecycle proof: trigger, wait for state, then freeze a frame
node scripts/agent/capture-observation.mjs \
  --base-url "$VALIDATION_URL" \
  --effect "<effect>" \
  --session "$SESSION" \
  --trigger ending \
  --state-path endingTicks \
  --state-equals 0 \
  --hold-ticks 12 \
  --output /workspace/evidence/observations/dev-<effect>-ending.json \
  --screenshot /workspace/evidence/screenshots/dev-<effect>-ending-terminal.png
```

`/dev/observe` is the preferred proof path for terminal lifecycle claims. A
video can still accompany the run so reviewers see motion, but terminal
correctness should come from the observer trace: the trigger reached the named
session, the state predicate became true, it held for the requested ticks, and
the frozen PNG came from that observed state. Do not infer terminal correctness
from an arbitrary video timestamp when a state predicate is available.

## Server readiness

The server takes a few seconds to compile + bind. The current verify
flow uses `wait-public-preview` (in `mcp/ambience_preview/`) which
polls until the validation URL responds 200. If you need a local
readiness signal (running `go run ./cmd/ambience` in-pod), poll
`/snapshot` until it returns 200; that endpoint is among the first
the server registers and it has no per-session state requirement.

The server also registers first-class `/healthz` (liveness) and `/readyz`
(readiness) endpoints — the chart's edge probes use them — so prefer those
for container/HTTP readiness; `/snapshot` stays a convenient local signal.

## Captured-video inspection

Verification must inspect captured WebMs with
`scripts/agent/inspect-video.mjs`. The helper reads the local file bytes into
a controlled Playwright media page, checks that Chromium can decode them,
enforces the requested minimum duration, and writes a sampled-frame PNG when
requested.

Do not start a local static server to play evidence videos back through
`127.0.0.1`. Playback inspection is a repo-owned helper so the verifier job
has one deterministic tool path and fails at that boundary when the artifact is
bad.
