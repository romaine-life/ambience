# Dev endpoints

The `cmd/ambience` server exposes a handful of `/dev/...` routes for
per-session, isolated testing of effects without disturbing the
shared broadcast.

## Routes

| Route | Method | Purpose |
| --- | --- | --- |
| `/dev/<effect>` | GET | Per-session dev page. Loads the named effect and renders it via the shared client. Each new browser session gets its own isolated atmosphere. |
| `/dev/snapshot` | GET | JSON snapshot of the calling session's dev atmosphere. |
| `/dev/events` | GET (SSE) | Per-session command stream (snapshot/config/trigger/scene/metric). |
| `/dev/config` | POST | Override the dev atmosphere's config. Body is the effect-specific config JSON or query-string-encoded knobs depending on the registered config parser. Returns 204 on success. |
| `/dev/trigger/<session>/<event>` | POST | Fire a discrete event in the named session. `session` is the dev-session identifier (a header or query param the page assigns). `event` is the effect-specific event name (e.g. `lightning-flash`, `ignite`, `gust`, `downpour`). |
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

# 2. Trigger a discrete event in a named session and record it
SESSION=test1
node scripts/agent/capture-video.mjs \
  --url "$VALIDATION_URL/dev/<effect>?session=$SESSION" \
  --output /workspace/evidence/videos/dev-<effect>-flash.webm \
  --trigger-url "$VALIDATION_URL/dev/trigger/$SESSION/lightning-flash" \
  --wait-ms 6000

# 3. Override config to a known good preset
curl -s -X POST -H 'Content-Type: application/json' \
  --data-binary '@preset.json' \
  "$VALIDATION_URL/dev/config?session=$SESSION"

# 4. Optional still-frame evidence when useful
node scripts/agent/capture-screenshot.mjs \
  --url "$VALIDATION_URL/dev/<effect>?session=$SESSION" \
  --output /workspace/evidence/screenshots/dev-<effect>-flash.png \
  --full-page --wait-ms 1000
```

## Server readiness

The server takes a few seconds to compile + bind. The current verify
flow uses `wait-public-preview` (in `mcp/ambience_preview/`) which
polls until the validation URL responds 200. If you need a local
readiness signal (running `go run ./cmd/ambience` in-pod), poll
`/snapshot` until it returns 200; that endpoint is among the first
the server registers and it has no per-session state requirement.

A first-class `/healthz` endpoint is tracked separately as a small
follow-up.
