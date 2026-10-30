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

Verify an effect by hand against a local server
(`AMBIENCE_ADDR=127.0.0.1:8080 go run ./cmd/ambience`). Pinning, triggering
and reading lifecycle are plain `curl` HTTP; screenshots come from
`scripts/shot.mjs` (see CLAUDE.md, "Taking screenshots").

```sh
BASE=http://127.0.0.1:8080
SESSION=test1
EFFECT=<effect>

# 0. Pin the session to a deterministic config. Dev sessions are created with
#    randomized knob values, so set every schema knob to its default before
#    judging the look. /dev/config creates the session if absent. Sessions are
#    reaped after 60s with no listeners (devSessionIdle) and a later read
#    recreates them randomized, so keep the dev page open while you work.
SCHEMA="$(curl -fsS "$BASE/effects/$EFFECT/schema")"
QS="$(printf '%s' "$SCHEMA" | jq -r '
  [ (.knobs // [])[]
    | "\(.key)=\(if .type == "int" then (.default | floor) else .default end)" ]
  | join("&")')"
curl -fsS -X POST "$BASE/dev/config?session=$SESSION&effect=$EFFECT&$QS" >/dev/null
curl -fsS "$BASE/dev/snapshot?session=$SESSION&effect=$EFFECT" | jq '.config'

# 1. Screenshot the pinned session.
node scripts/shot.mjs "$BASE/dev/$EFFECT?session=$SESSION" tmp-shots/default.png

# 2. Fire a discrete event in the same session. The trigger URL requires
#    ?effect= — sessions are keyed by (effect, session), so an effect-less
#    trigger lands on the default effect and is rejected with `unknown event`.
curl -fsS -X POST "$BASE/dev/trigger/$SESSION/<event>?effect=$EFFECT" >/dev/null

# 3. Terminal lifecycle proof: trigger `ending`, wait for the lifecycle
#    contract value, hold it, and get a frozen grid frame — all through
#    /dev/observe. Use `ended` only when the effect schema declares
#    ending_terminal: true; non-terminal effects resume, so their post-outro
#    claim is `running`.
scripts/agent/selfcheck-effect.sh "$EFFECT" ending
```

Terminal correctness comes from the lifecycle observation, not from a
screenshot timestamp: the trigger reached the named session, the lifecycle
contract value was observed, it held for the requested ticks, and the frozen
PNG came from that observed state.

## Server readiness

The server takes a few seconds to compile + bind. For a local
readiness signal, poll `/snapshot` until it returns 200; that endpoint is among the first
the server registers and it has no per-session state requirement.

The server also registers first-class `/healthz` (liveness) and `/readyz`
(readiness) endpoints — the chart's edge probes use them — so prefer those
for container/HTTP readiness; `/snapshot` stays a convenient local signal.
