# ambience

Shared-world ambient pixel-art effects. A 10 Hz server decides when
discrete events fire (downpour, calm, gust, splash) and broadcasts those
as commands via SSE; every consumer (browser canvas, terminal sixel)
runs its own sim replica and applies the commands in sync. Rain is the
only effect in the shared live world today; Fireflies, Waterfall, and
Dust are available in isolated dev sessions.

Canonical live view: <https://ambience.romaine.life>.

## Quick start

```sh
go run ./cmd/ambience
# open http://localhost:8080/
```

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

`client.js` auto-initializes on any `<canvas data-ambience>`, subscribes
to the shared atmosphere, runs the sim locally, and posts keystroke-
derived entropy back so typing subtly steers the world. Configure via
`data-ambience-*` attributes on the canvas (server URL, grid dims,
transparent-vs-opaque render, entropy on/off).

For terminal consumers, `github.com/nelsong6/ambience/terminal` is a Go
package that subscribes + renders via sixel. Used by fzt-automate;
currently has platform-specific rendering issues on Windows Terminal —
see [`docs/terminal-integration-status.md`](docs/terminal-integration-status.md).

## Architecture

```
cmd/ambience/    HTTP server + atmosphere goroutine. Decides event
                 timing (downpour/calm/gust/splash) and broadcasts
                 state commands. Does NOT stream pixel frames.
  web/           Embedded static: index.html (demo), sim.js (JS port
                 of the sim), controls.js (shared control helper),
                 client.js (auto-init shim for consumers), dev.html
                 (knob-tuning page).

sim/             Pure Go simulation logic. No I/O. Consumed by the
                 server and by the terminal client.

terminal/        SSE subscriber + local sim replica + sixel renderer
                 as a Go package. Consumed by fzt-automate.

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
                 daemon is not required for the dev loop.
scripts/dev-loop.ps1
                 Fast static-web dev helper for the dev environment.
                 Syncs `cmd/ambience/web` straight into the live edge
                 pod's override directory so `/dev` changes can be
                 tested on `ambience.dev.romaine.life` without a local
                 runtime or a full image rebuild.
```

## Atmosphere model

The server does not broadcast pixel frames. Each atmosphere is a
server-side sim running at 10 Hz whose job is to decide when discrete
events fire. Clients run their own sims locally and apply five kinds of
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

Clients do not roll for discrete events — only the server does. Each
client's RNG drifts from the server's after the initial snapshot, but
event timing stays in sync.

## Terminology

- **Effect** means the simulation mechanism selected from the
  `AmbienceSim.effects` registry and carried on the wire as snapshot
  `type` (`rain`, `sand`, `volcano`, ...).
- **Scene** means the viewer-facing "what is showing right now?" state
  within an effect, carried as `currentScene` / `nextScene`.
- In Rain today, a scene is a generated config variant with a duration
  and lookahead. In simpler effects, scene data can just be the display
  label surface for the active effect.

Short version: effect answers "which sim is running?", while scene
answers "what version of that sim is on screen right now?"

## Effects model

Every effect fills a 5-slot template — see
[issue #1](https://github.com/nelsong6/ambience/issues/1):

1. **Spawn config** — random init params
2. **Continuous levers** — micro-drift fed by entropy
3. **Discrete events** — periodic bursts
4. **Event modifiers** — per-event randomization
5. **End conditions** — natural conclusions (optional)

New effects plug in via the `AmbienceSim.effects` registry in `sim.js`.
Browser clients look up the constructor by the `type` the server
broadcasts — so adding Sand / Fire / Tetris is a sim-side change with
no consumer-side update. The `/dev` page reads the same registry to
switch effects without page-specific wiring.

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
- `GET  /sim.js`, `/controls.js`, `/client.js` — consumer scripts
- `GET  /snapshot` — current atmosphere state (JSON)
- `GET  /events` — atmosphere command stream (SSE)
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

This repo now uses manual, session-driven CI. `.github/workflows/build-and-deploy.yml`
has no automatic triggers; it only runs when manually dispatched.

The intended production deploy loop is:

1. Push the code change you want built.
2. Manually dispatch the workflow to build and push
   `romainecr.azurecr.io/ambience:<sha>`.
3. Update `chart/ambience/values-prod.yaml` to that image tag.
4. Commit and push the tag bump from the session.
5. Let ArgoCD reconcile the committed chart change, or manually refresh/sync
   it if you want a faster rollout.

The dev environment is also declared through ArgoCD at
`infra-bootstrap/k8s/apps/ambience-dev.yaml`, but that app intentionally
leaves automated sync off. That keeps Argo as the owner of the base dev
environment without immediately reverting fast live image swaps during a
session.

Unless a task explicitly calls for localhost, the default test target
should be `https://ambience.dev.romaine.life`, not a local runtime.

Use the dev helpers like this:

1. Static browser-only work in `cmd/ambience/web/`:
   run `powershell -ExecutionPolicy Bypass -File scripts/dev-loop.ps1 -Once`
   to sync the web files into the live dev edge pod without a Docker
   build. Do not run the background watcher form unless you explicitly
   want it.
2. Go/runtime changes that need a new binary or image:
   run `powershell -ExecutionPolicy Bypass -File scripts/dev-deploy.ps1 -Component edge`
   or swap `edge` for `authority` / `all`. The script builds and pushes a
   temporary image tag, then patches the live `ambience-edge` and/or
   `ambience-authority` image fields with `kubectl set image`.
3. If local Docker is unavailable, `scripts/dev-deploy.ps1` automatically
   falls back to `az acr build` and verifies the tag exists in ACR before
   patching, so the session can stay k8s-first without relying on a local
   Docker daemon.

The web sync path is backed by `AMBIENCE_WEB_OVERRIDE_DIR` on the dev
edge deployment: the main container reads static files from the shared
override directory first, then falls back to embedded assets from the
image. That keeps static `/dev` iteration fast without changing the
authority workload.

The recommended feature-iteration loop is:

1. Start a fresh session by re-checking the open issues and open PRs so
   the next slice comes from the current backlog.
2. Choose one bounded feature, preferably effect work or an
   effect-adjacent enhancement when the registry and `/dev` tooling
   already support it cleanly.
3. Validate it on `ambience.dev.romaine.life` first, not localhost, unless
   the task explicitly needs a local-only repro.
4. Use `scripts/dev-loop.ps1 -Once` for browser-only static work in
   `cmd/ambience/web/`; do not default to the long-lived watcher form.
5. Use `scripts/dev-deploy.ps1 -Component all` for changes that touch both
   browser assets and authority/runtime code, which is the common case for
   new effects. Use `edge` or `authority` only when the change is truly
   one-sided.
6. Verify the result on `/dev/<effect>` or the relevant dev route before
   promoting it.

When a dev image should become declared state again, update
`chart/ambience/values-dev.yaml` to that image tag, commit the bump, and
manually sync the `ambience-dev` Argo app.

The ArgoCD Applications at
`infra-bootstrap/k8s/apps/{ambience,ambience-dev}.yaml`
watch this repo's `chart/ambience/` path on `main`; committed Helm values
changes feed the desired state, with prod autosyncing and dev syncing on
demand.

The shipped Kubernetes manifests now split the app into one internal
`authority` StatefulSet and a public `edge` Deployment. The authority
snapshots the shared atmosphere every 30s to
`AMBIENCE_PERSIST_PATH`; the manifests mount a PVC at `/data` and persist
to `/data/shared-atmosphere.json`, so authority restarts resume the live
world instead of resetting it.

## Status

Rain effect live. Dev sessions now support Rain, Fireflies, Waterfall,
and Dust with randomized starting stats plus a `randomize` button on
`/dev/<effect>`. Consumers: ambience's own demo, fzt-showcase (DOS
terminal), my-homepage (bookmark terminal). Entropy flow wired.

Terminal integration via fzt-automate is tabled pending platform
rendering work — see issues
[#11–#15](https://github.com/nelsong6/ambience/issues?q=is%3Aopen+label%3Aterminal-client).

Repo migration `nelsong6/` → `romaine-life/` tracked by
[#10](https://github.com/nelsong6/ambience/issues/10).
