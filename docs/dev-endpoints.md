# Dev endpoints

The `cmd/ambience` server exposes a handful of `/dev/...` routes for
per-session, isolated testing of effects without disturbing the
shared broadcast.

## Routes

| Route | Method | Purpose |
| --- | --- | --- |
| `/dev/<effect>` | GET | Per-session dev page. Loads the named effect and renders it via the shared client. Each new browser session gets its own isolated atmosphere. |
| `/dev/snapshot` | GET | JSON snapshot of the calling session's dev atmosphere. Includes `appliedEvents` — a bounded ring of `{tick,event}` for events actually applied to this session's sim — and a top-level `lifecycle` (`intro\|running\|ending\|ended`), the effect-generic arc contract every intro/ending-capable effect publishes. Use them to mechanically confirm a fired trigger reached the session you are observing, rather than inferring it from a single frame (a pristine, never-triggered sim can coincidentally match a resting look). |
| `/dev/events` | GET (SSE) | Per-session command stream (snapshot/config/trigger/scene/metric). |
| `/dev/config` | POST | Override the dev atmosphere's config. Body is the effect-specific config JSON or query-string-encoded knobs depending on the registered config parser. Returns 204 on success. |
| `/dev/trigger/<session>/<event>?effect=<effect>` | POST | Fire a discrete event in the named session. `session` is the dev-session identifier. The `effect` query param is required in practice: sessions are keyed by (effect, session), so a trigger without it routes to the default effect's session and is rejected with `unknown event` when the event belongs to another effect. To observe the effect, load the page with the matching `?session=<session>` — the dev page honors that query param so an external driver addresses the same session it triggers (a fired trigger only affects the page when both sides agree on the id). `event` is the effect-specific event name (e.g. `lightning-flash`, `ignite`, `gust`, `downpour`). |
| `/dev/observe` | POST | Verification-only lifecycle observer. It can trigger an event, wait for a lifecycle log marker and/or a `lifecycle=<intro\|running\|ending\|ended>` contract predicate, require it to hold for `hold_ticks`, then store a frozen grid frame and return a JSON trace with `appliedEvents`, ticks, config, final state, `observationId`, and `frameUrl`. Raw state-path probing was retired (ambience#174): observers assert the lifecycle contract, not effect-internal field names. |
| `/dev/frame` | GET | Returns an `image/png` for a frozen observation frame from `/dev/observe` via `?session=<session>&effect=<effect>&observation=<observationId>`. |
| `/effects/<effect>/schema` | GET | JSON schema for the effect's dev-panel knobs (used by the dev page to render controls). |

## Verification recipe

Browser evidence is captured **only** through Glimmung's central MCP capture
tools (`capture_video` / `capture_screenshot`), which connect to the leased
slot browser, record/snapshot the page, upload the artifact, and return a
`ref`/`url`. There is no per-repo Playwright script and no local capture file.
Everything that is not a browser capture — pinning, triggering, reading
lifecycle — is plain `curl` HTTP.

When validating a new effect through the agent flow, the typical sequence is:

```sh
# 0. Pin the session to a deterministic config over plain HTTP. Dev sessions
#    are created with randomized knob values; verification claims are judged
#    against schema defaults (+ the case's session_config overrides), so pin
#    BEFORE capturing the page or firing triggers. The verify wrapper re-checks
#    the pin and fails the case when the session does not match it.
#
#    Lifecycle: sessions are reaped after 60s with no listeners
#    (devSessionIdle) and a later read lazily recreates them RANDOMIZED.
#    The verify wrapper holds a background SSE listener on the case's
#    session (/dev/events) from prepare to emit so the pinned session
#    survives the whole case; ad hoc runs that pause longer than 60s
#    between pin and capture must hold their own listener.
SESSION=test1
EFFECT=<effect>
# Fetch the schema, set every knob to its default (or session_config override),
# and POST them to /dev/config. /dev/config creates the session if absent.
SCHEMA="$(curl -fsS "$VALIDATION_URL/effects/$EFFECT/schema")"
QS="$(printf '%s' "$SCHEMA" | jq -r '
  [ (.knobs // [])[]
    | "\(.key)=\(if .type == "int" then (.default | floor) else .default end)" ]
  | join("&")')"
curl -fsS -X POST "$VALIDATION_URL/dev/config?session=$SESSION&effect=$EFFECT&$QS" >/dev/null
curl -fsS "$VALIDATION_URL/dev/snapshot?session=$SESSION&effect=$EFFECT" | jq '.config'

# 1. Record default behavior (pinned session) — call the capture_video tool:
#      {"url": "$VALIDATION_URL/dev/<effect>?session=test1",
#       "wait_ms": 6000, "label": "dev <effect> default"}
#    It uploads the WebM and returns its ref/url. There is no local file.

# 2. Trigger a discrete event in the same named session and record it. Pass the
#    trigger_url to capture_video so the event is POSTed once the page is
#    visible and recording has started. The trigger URL requires ?effect= —
#    sessions are keyed by (effect, session), so an effect-less trigger lands on
#    the default effect and is rejected with `unknown event`:
#      {"url": "$VALIDATION_URL/dev/<effect>?session=test1",
#       "wait_ms": 6000, "label": "dev <effect> flash",
#       "trigger_url": "$VALIDATION_URL/dev/trigger/test1/lightning-flash?effect=<effect>"}

# 3. Override config to a known good preset (plain HTTP)
curl -s -X POST -H 'Content-Type: application/json' \
  --data-binary '@preset.json' \
  "$VALIDATION_URL/dev/config?session=$SESSION"

# 4. Optional still-frame evidence — call the capture_screenshot tool:
#      {"url": "$VALIDATION_URL/dev/<effect>?session=test1",
#       "label": "dev <effect>", "full_page": true, "wait_ms": 1000}

# 5. Terminal lifecycle proof over HTTP: trigger, then poll /dev/snapshot for
#    the lifecycle contract value, then capture the frozen look with the
#    capture_screenshot tool. Use `ended` only when the effect schema declares
#    ending_terminal: true; non-terminal effects resume, so their post-outro
#    claim is `running`. (The verify wrapper proves the hold mechanically via
#    /dev/observe; ad hoc verification confirms it with the snapshot poll.)
curl -fsS -X POST "$VALIDATION_URL/dev/trigger/$SESSION/ending?effect=$EFFECT" >/dev/null
for _ in $(seq 1 40); do
  snap="$(curl -fsS "$VALIDATION_URL/dev/snapshot?session=$SESSION&effect=$EFFECT")"
  [ "$(printf '%s' "$snap" | jq -r '.lifecycle // ""')" = "ended" ] \
    && printf '%s' "$snap" | jq -e '[.appliedEvents[]?.event] | index("ending") != null' >/dev/null \
    && break
  sleep 0.5
done
# Then call capture_screenshot against $VALIDATION_URL/dev/$EFFECT?session=$SESSION.
```

`/dev/observe` is the wrapper's mechanical proof path for terminal lifecycle
claims (the verify wrapper drives it over HTTP and enforces
applied/observed/hold from the trace). A video captured by `capture_video` can
accompany the run so reviewers see motion, but terminal correctness comes from
the lifecycle observation: the trigger reached the named session, the lifecycle
contract value was observed, it held for the requested ticks, and the frozen
PNG came from that observed state. Do not infer terminal correctness from an
arbitrary video timestamp when a lifecycle predicate is available.

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

## Captured-video integrity

There is no local video-decode step. The central `capture_video` tool connects
to the leased slot browser and starts recording only after the page has
rendered — its server-side blank-frame gate owns "did it render" — then uploads
the WebM and returns a `ref`/`url`. By the time a ref exists, render and upload
are already proven, so the verifier judges the look of the uploaded artifact
directly and does not re-decode it locally. The retired per-repo decode gate
was removed with the other browser scripts rather than reimplemented. Do not
start a local browser or static server to play evidence videos back through
`127.0.0.1`; browser evidence comes only from the central capture tools.
