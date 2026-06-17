# ambience

Shared-world ambient pixel-art effects. A 60 Hz server decides which
effect is active, which scene/config is current, and when discrete
events fire, then broadcasts those commands via SSE. Browser consumers
run their own delayed Go/WASM sim replicas and apply the commands against
an authority clock. Terminal sixel support participates in the same
authority-clock buffering for rain-only mode; all-effect terminal support
is still separate future work. The server, browser, terminal, and
generated social images share the same Go `sim` package as their pixel
simulation source.

The long-term target is clock-like lock-step rendering across browser
consumers:
if `ambience.romaine.life` is open on one machine and a subscriber such
as `homepage.romaine.life` is open on another, they should appear to be
displaying the same shared world at the same moment, modulo ordinary
network and browser scheduling jitter. The server still avoids streaming
raw pixels; the engineering challenge is keeping semantic snapshots,
ticks, RNG state, scene transitions, and client replay tight enough that
subscribers feel synchronized rather than merely thematically similar.

Canonical live view: <https://ambience.romaine.life>.

## Quick start

```sh
./scripts/build-web-wasm.sh
go run ./cmd/ambience
# open http://localhost:8080/
```

The Docker image runs `build-web-wasm.sh` automatically before embedding web
assets. Local `go run` needs that script once per checkout or after WASM
bridge changes so `/ambience.wasm` and `/wasm_exec.js` exist under
`cmd/ambience/web/`.

`/` renders the current effect full-screen. `/dev` opens a per-session
effect-tuning page with an effect switcher, presets when the active
effect defines them, randomized starting stats, a `randomize` button,
and live adjustable knobs.

## Consumer integration

Any web page can drop ambience in as a background overlay:

```html
<canvas data-ambience></canvas>
<script src="https://ambience.romaine.life/sim.js"></script>
<script src="https://ambience.romaine.life/client.js"></script>
```

`client.js` auto-initializes on any `<canvas data-ambience>`, loads the
Go/WASM runtime, subscribes to the shared atmosphere, runs the sim locally,
and posts keystroke-derived entropy back so typing subtly steers the world.
Configure via
`data-ambience-*` attributes on the canvas (server URL, grid dims,
transparent-vs-opaque render, entropy on/off).

For terminal consumers, `github.com/romaine-life/ambience/terminal` is a Go
package that subscribes + renders rain via sixel. It applies snapshots
immediately, queues future config/trigger commands behind the delayed
authority playback tick, and exposes `Client.DebugState()` telemetry for
rain-only sync debugging. It does not yet instantiate every live effect;
that needs a separate terminal runtime/registry design. See
[`docs/terminal-integration-status.md`](docs/terminal-integration-status.md).

## Architecture

```
cmd/ambience/    HTTP server + atmosphere goroutine. Decides event
                 timing (downpour/calm/gust/splash) and broadcasts
                 state commands. Does NOT stream pixel frames.
  web/           Embedded static: index.html (demo), sim.js
                 (AmbienceSim namespace/helpers), wasm_runtime.js
                 (Go/WASM sim loader), controls.js (shared control helper),
                 client.js (auto-init shim for consumers), dev.html
                 (knob-tuning page).

cmd/ambience-wasm/
                 Go/WASM bridge that exposes sim runtimes to browser JS.

sim/             Pure Go simulation logic. No I/O. Consumed by the
                 server and by the terminal client.

terminal/        Rain-only SSE subscriber + delayed authority-clock local
                 sim replica + sixel renderer as a Go package. Consumed by
                 fzt-automate; all-effect terminal support is future work.

tools/conpty-capture/
                 Python/pywinpty tool that captures Windows terminal
                 byte streams including sixel DCS blocks (which
                 PowerSession misses). For debugging sixel rendering
                 inside fzt-automate.

chart/ambience/  Helm chart used by ArgoCD for both prod and dev.
                 `values-prod.yaml` serves `ambience.romaine.life`;
                 `values-dev.yaml` serves `ambience.dev.romaine.life`.
                 The dev Argo app is intentionally manual-sync so
                 direct session-driven image rolls are not immediately
                 reconciled away.
scripts/dev-deploy.ps1
                  Local k8s-first dev helper that builds, pushes, and
                  patches just `edge`, just `authority`, or `all` in the
                  live dev namespace. Prefers local Docker when available,
                  but can fall back to `az acr build` so a local Docker
                  daemon is not required for the dev loop. When rolling
                  `all`, it updates authority before edge so edge
                  readiness does not wait on a simultaneously replacing
                  authority pod.
Glimmung test slots
                  Deploy the CI-built image for the pushed ref with
                  `deploy_image_to_test_slot`. Browser assets, authority
                  code, and edge static serving are validated together from
                  the same image artifact that PR CI proved.
```

## Atmosphere model

The server does not broadcast pixel frames. Each atmosphere is a
server-side sim running at 60 Hz whose job is to choose the active
effect, own scene/config transitions, and decide when discrete events
fire. Clients run their own sims locally and apply five kinds of
commands:

- **`snapshot`** — state dump on connect. The outer envelope is
  effect-generic: `{type, tick, config, state, seed, gridW, gridH,
  currentScene, nextScene, entropyBytes, sceneRemaining}`. `config` and
  `state` are effect-owned blobs; `type` tells clients which constructor
  to instantiate.
- **`config`** — sim config changed; clients apply via `setConfig`.
- **`trigger`** — a discrete event fired; clients apply its effects.
- **`scene`** — scene rotation metadata for the live monitor.
- **`metric`** — entropy and scene-progress heartbeat data.
- **`clock`** — sparse authority tick samples. Clients use these like a
  clock signal and render behind the authority by a small delay buffer, so
  independent consumers can seek the same playback tick without streaming
  dense per-frame state.

Clients do not roll for discrete events — only the server does. The goal
is for clients to treat the authority stream like a clock signal: restore
from snapshots, advance by authoritative ticks, and converge on the same
visible phase after reconnects or effect changes. Any client-side RNG
use should be deterministic from authority-provided state or limited to
visual details that do not make subscribers visibly diverge.

The browser client exposes `window.AmbienceClient.getDebugState()` when it
is running. That returns the estimated authority tick, delayed playback
tick, local sim tick, drift, queue depth, and active effect type; use it to
compare browser subscribers such as `ambience.romaine.life` and
`homepage.romaine.life` without adding transport volume. Terminal sixel
exposes comparable rain-only telemetry through `Client.DebugState()`.

Example browser sync telemetry:

```js
window.AmbienceClient.getDebugState()
// {
//   effectType: "rain",
//   ready: true,
//   authorityTick: 24810,
//   playbackTick: 24760,
//   simTick: 24759,
//   driftTicks: 1,
//   delayTicks: 300,
//   bufferedAheadTicks: 8,
//   queuedCommands: 2,
//   nextQueuedCommandTick: 24762,
//   maxQueuedCommandTick: 24768,
//   haveAuthoritySample: true
// }
```

`authorityTick` is the latest estimated server tick from sparse `clock`
samples. `playbackTick` is the delayed target tick the browser is trying
to render. `driftTicks` is `playbackTick - simTick`; small positive values
mean the local sim is catching up, while zero means it is on the delayed
target. `bufferedAheadTicks` is `maxQueuedCommandTick - playbackTick`,
clamped at zero; it only describes queued authority commands already
received by the browser, not future commands that have not arrived.
`nextQueuedCommandTick` and `maxQueuedCommandTick` are `null` when the
command queue is empty.

The deterministic client-side sync harness lives at
`scripts/test-client-sync.mjs`. It runs two isolated browser-client
instances against the same scripted authority stream and checks that they
stay aligned through initial connect, buffered playback, scene/metric
metadata, queued config/trigger commands, effect rotation, a resume-style
catch-up, a fresh snapshot convergence path, and unsupported effect
registry handling. It is still a browser-client harness; HTTP
Last-Event-ID replay is covered by server-side direct-authority and edge
mirror tests. Terminal rain-only authority-clock buffering is covered by
Go tests in `terminal/`.

## Effects model

Every effect fills a 5-slot template:

1. **Spawn config** — random init params
2. **Continuous levers** — micro-drift fed by entropy
3. **Discrete events** — periodic bursts
4. **Event modifiers** — per-event randomization
5. **End conditions** — natural conclusions (optional)

New effects plug in through Go types in `sim/`. Browser clients load
`/wasm_runtime.js`, which loads the Go/WASM runtime and registers one
constructor per supported effect in `AmbienceSim.effects`. Consumer pages
that vendor the scripts, such as `my-homepage`, must refresh the vendored
bundle whenever the shared sim/client changes so they remain full
subscribers to every live effect. The `/dev` page reads the same registry
to switch effects without page-specific wiring.

For backlog candidates not yet promoted to their own implementation
issue, see [`docs/effect-candidates.md`](docs/effect-candidates.md), with
5-slot fit notes and promotion status per candidate.

## Guiding principle

One of ambience's guiding principles is to copy the *boundaries* that
make simulation-heavy systems like Noita compelling, without trying to
clone their exact engine.

- Keep authoritative world truth compact and semantic. The server should
  decide important events, phases, and state transitions; clients should
  do the expensive local replay/render work.
- Keep the transport generic at the envelope level and effect-owned on
  the inside. Snapshot/config/trigger/schema are shared seams; each
  effect owns its own inner state and tuning knobs.
- Aggregate secondary systems instead of mirroring every particle.
  Persistence, logs, metrics, entropy, and future audio should describe
  meaningful simulation state, not become a raw firehose of per-pixel
  updates.
- Prefer stable control seams over effect-specific special cases. New
  effects should plug into the registry, schema, snapshot/restore, and
  trigger paths instead of requiring consumer-specific wiring.
- Treat cross-consumer synchronization as a product goal, not a best
  effort side effect. Effect implementations should preserve enough
  state in snapshots and deterministic replay paths that independent
  clients can render the same phase of the same scene like synchronized
  clocks.
- Before an effect becomes live/promotable, `go test ./sim ./cmd/ambience`
  must pass the replay audit. Client-facing snapshots need the current
  tick, config, visible particle/timer state, and RNG cursor (`rngState`);
  restore must repaint an immediately comparable frame, accept schema
  triggers deterministically, and keep post-restore frames and snapshot
  state equal after future random draws.

## Entropy

Client-side keystroke capture feeds bytes into `POST /entropy`. The
server folds them into the shared atmosphere's seed and re-seeds the
sim RNG via `Rain.PerturbRNG(delta)`. Future random decisions drift —
typing subtly steers the pattern. Cheap and lossy on purpose; this is
aesthetic perturbation, not secure randomness.

## Endpoints

All broadcast endpoints set permissive CORS for cross-origin consumers.

- `GET  /` — demo page
- `GET  /dev`, `/dev/<effect>` — dev page with effect switcher, presets,
  randomized per-session configs, and per-effect knobs
- `GET  /sim.js`, `/wasm_runtime.js`, `/wasm_exec.js`, `/ambience.wasm`,
  `/controls.js`, `/client.js` — consumer scripts/runtime
- `GET  /snapshot` — current atmosphere state (JSON)
- `GET  /events` — atmosphere command stream (SSE). Browser `EventSource`
  reconnects use `Last-Event-ID`; the authority and edge mirror replay
  missed commands while their bounded replay buffers still cover the ID,
  otherwise they send a fresh snapshot frame.
- `POST /config?effect=&...` — mutate the shared atmosphere config
- `POST /trigger/:event` — fire a discrete event on the shared atmosphere
- `POST /entropy` — raw bytes folded into the RNG (max 4KB/req)
- `POST /dev/randomize?session=&effect=` — roll a new config for the
  active dev session
- `GET  /effects/:effect/schema` — knob schema for the dev UI

The user-facing static routes are intentionally exact: use `/` and
`/dev/<effect>`, not `.html` URLs. `/index.html` and unknown static paths
return 404.

## Roles

The binary supports three runtime roles via `AMBIENCE_ROLE`:

- `all` — backward-compatible single-process mode; serves static pages and
  runs the shared authority locally
- `authority` — runs the shared sim + dev atmospheres and owns persistence
- `edge` — serves the web UI and proxies `/snapshot`, `/events`, `/entropy`,
  and `/dev/*` to the authority given by `AMBIENCE_AUTHORITY_URL`

Edge mode also buffers entropy best-effort in memory if the authority is
briefly unavailable, then retries forwarding on a short cadence.

## Deploying

`.github/workflows/build-and-deploy.yml` publishes the canonical
`romainecr.azurecr.io/ambience:app-<fingerprint>` image. On pushes to
main it commits the matching tag into the prod and base Helm values files.
Manual dispatch can prebuild a requested `git_ref` without changing chart
desired state.

The intended production deploy loop is:

1. Merge or push the code change to `main`.
2. Let the build workflow publish the `app-<fingerprint>` image and commit
   the chart tag bump.
3. Let ArgoCD reconcile the committed chart change, or manually refresh/sync
   it if you want a faster rollout.

A dev environment under `ambience.dev.romaine.life` shares the wildcard
listener at `*.ambience.dev.romaine.life` with Glimmung test slots. The
continuously synced prod release owns that shared listener and certificate
so slot HTTPS works without per-slot certificate requests. `values-dev.yaml`
still supports a standalone `ambience-dev` install if continuous dev hosting
becomes useful again.

Unless a task explicitly calls for localhost, the default test target
should be a preview slot URL or `https://ambience.dev.romaine.life`,
not a local runtime.

Use the dev helpers like this:

1. Test-slot validation for browser assets, authority Go, edge static
   serving, or chart/runtime image inputs:
   push the ref, wait for CI to prove the `app-<fingerprint>` image and
   publish its run-scoped lookup tag, then deploy it with Glimmung
   `deploy_image_to_test_slot`.
2. Direct dev-environment rollouts that need a new image on edge,
   authority, or both:
   run `powershell -ExecutionPolicy Bypass -File scripts/dev-deploy.ps1 -Component edge`
   or swap `edge` for `authority` / `all`. The script builds and pushes a
   temporary image tag, then patches the live `ambience-edge` and/or
   `ambience-authority` image fields with `kubectl set image`.
3. If local Docker is unavailable, `scripts/dev-deploy.ps1` automatically
   falls back to `az acr build` and verifies the tag exists in ACR before
   patching, so the session can stay k8s-first without relying on a local
   Docker daemon.

Glimmung native test slots are validated by deploying the CI-built image for a
pushed ref with `deploy_image_to_test_slot`. Browser assets and authority code
ship together in that image, so the slot runs the same artifact that PR CI
proved and main will deploy.

Keep image rollout for edge binary changes, dependency changes, chart changes,
and edits that change runtime image inputs.

The recommended feature-iteration loop is:

1. Start a fresh session by re-checking the open issues and open PRs so
   the next slice comes from the current backlog.
2. Choose one bounded feature, preferably effect work or an
   effect-adjacent enhancement when the registry and `/dev` tooling
   already support it cleanly.
3. Validate it on `ambience.dev.romaine.life` first, not localhost, unless
   the task explicitly needs a local-only repro.
4. Deploy the pushed ref's CI image with `deploy_image_to_test_slot` for
   browser assets and authority-side Go. Use `scripts/dev-deploy.ps1
   -Component all` only for direct dev-environment image rollouts.
5. Verify the result on `/dev/<effect>` or the relevant dev route before
   promoting it.

When a dev image should become declared state again, update
`chart/ambience/values-dev.yaml` to that image tag and commit the bump.
If/when an `ambience-dev` ArgoCD app is wired up, it will sync from
`chart/ambience/` using that values file.

The ArgoCD Application at
`infra-bootstrap/k8s/apps/ambience.yaml`
watches this repo's `chart/ambience/` path on `main`; committed Helm
values changes feed the desired state, and prod autosyncs.

The shipped Kubernetes manifests now split the app into one internal
`authority` StatefulSet and a public `edge` Deployment. The authority
upserts the shared atmosphere document every 30s into the `ambience`
Cosmos database (container `atmosphere`, document id `shared`) and
reads it back on startup, so pod restarts resume the live world. Auth
is via Azure workload identity: the `ambience-identity` user-assigned
identity (provisioned in `tofu/`) is federated to the prod namespace's
`default` ServiceAccount, scoped at the Cosmos data plane to
`dbs/ambience` only. Preview slots leave `authority.cosmos.endpoint`
unset and run without persistence.

## Status

Rain effect live. Dev sessions now support Rain, Fireflies, Waterfall,
and Dust with randomized starting stats plus a `randomize` button on
`/dev/<effect>`. Consumers: ambience's own demo, fzt-showcase (DOS
terminal), my-homepage (bookmark terminal). Entropy flow wired.

Terminal rain-only sync is implemented in the `terminal` package, but
fzt-automate rendering integration is still tabled pending platform
rendering work tracked in Glimmung.

Repo migration `nelsong6/` to `romaine-life/` is tracked in Glimmung.
