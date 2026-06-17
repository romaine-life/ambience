# ambience

Shared-world ambient pixel-art effect system. Server coordinates state;
every client (browser canvas, terminal sixel) runs its own sim replica
that receives config + event broadcasts via SSE. Conceptually independent
of fzt — the name only matches the domain (`ambience.romaine.life`).
Read `D:/shell-config/setup/codex/AGENTS.md` for global Codex config.

## Start Here

If you only read one section in this file, read this one.

1. Re-open the open Glimmung issues and GitHub PRs at the start of a fresh
   session, then pick one concrete, bounded slice from the real backlog.
2. Treat `https://ambience.dev.romaine.life` as the default test target.
   Do not spin up or rely on localhost unless the user explicitly asks
   for a local repro.
3. For browser assets, authority Go, or runtime image inputs, validate with a
   CI-built image deployed through Glimmung test slots using
   `deploy_image_to_test_slot`.
4. For direct dev-environment rollouts outside test slots, use
   `powershell -ExecutionPolicy Bypass -File scripts/dev-deploy.ps1 -Component all`
   by default. Use `edge` or `authority` only when the change is truly
   one-sided.
5. Validate the result on the assigned test-slot URL or
   `ambience.dev.romaine.life`, usually on
   `/dev/<effect>` for effect work, before treating it as ready.
6. Only after dev validation should the change move into the manual
   production promotion flow: build/push the real image, bump the prod
   Helm values file, commit that desired-state change, and let ArgoCD
   reconcile it.

## Container Build Verification

Agent pods are not expected to have Docker. Do not report missing local Docker
as a blocker. Run available repo checks first, then use PR CI as the normal
container build gate: `.github/workflows/docker-build-check.yml` computes
image fingerprints and reuses or pushes ACR proof images for trusted PRs,
falling back to `push: false` for fork PRs. If image-packaging feedback is
needed before a PR is ready, manually dispatch that workflow with `git_ref`.
Release/deploy workflows publish the fingerprint-tagged images used by deploys.

## Architecture

```
cmd/ambience (Go)     HTTP server. Runs the shared atmosphere goroutine
  │                   at 60 Hz — decides when discrete events fire
  │                   (downpour/calm/gust/splash) and broadcasts state
  │                   commands. Does NOT stream pixel frames.
  │                   Embeds web/ and serves it.
  │
  ├─► /                  Live broadcast monitor — canvas running the shared
  │                      replica sim + status panel overlay (current scene,
  │                      remaining, up next, entropy counter with togglable
  │                      capture, rolling event log). Canonical consumer view.
  ├─► /sim.js            `AmbienceSim` namespace/helpers
  ├─► /wasm_runtime.js   Go/WASM sim loader; registers Go sim constructors
  ├─► /ambience.wasm     Go `sim` package compiled for browser runtime
  ├─► /controls.js       Shared schema-driven control panel helper
  ├─► /client.js         Shared auto-init browser client (see "Consumer
  │                      integration" below)
  ├─► /ambience.js       Older helper (pre-client.js); still served
  ├─► /dev               Dev page with per-session effect switching,
  │                      presets, and live knobs (NOT connected to the
  │                      shared broadcast)
  ├─► /snapshot          JSON: current shared atmosphere state (incl. scene)
  ├─► /events            SSE: atmosphere commands
  │                      (snapshot/config/trigger/scene/metric)
  ├─► /entropy           POST bytes — folded into the shared RNG
  ├─► /effects/<effect>/schema  JSON schema for an effect's knobs
  └─► /dev/{snapshot,events,config,randomize,trigger}
                         Per-session dev atmospheres

sim/             Pure Go simulation logic. Rain is the live shared-world
                 effect today; Fireflies, Waterfall, and Dust also exist
                 as isolated dev effects. No I/O; consumed by
                 cmd/ambience (server-side) and by terminal/
                 (client-side sixel renderer).

terminal/        Go package — SSE subscriber + local sim replica + sixel
                 renderer. Consumed by fzt-automate via its
                 PostFrameHook. See `docs/terminal-integration-status.md`
                 for open issues (flicker, opacity, accumulation).

tools/conpty-capture/  Python/pywinpty ConPTY capture tool — records full
                       Windows terminal byte stream including sixel DCS
                       blocks, writes asciinema-compatible .cast.
                       Closes the diagnostic gap PowerSession leaves
                       (PowerSession doesn't capture CONOUT$ writes).

chart/ambience/  Helm chart used by ArgoCD for prod and held in reserve
                 for dev. `values-prod.yaml` drives the live app at
                 `ambience.romaine.life`; `values-dev.yaml` is the
                 declared state for `ambience.dev.romaine.life` but is
                 not currently wired to an ArgoCD app — restore by
                 adding a sibling `infra-bootstrap/k8s/apps/ambience-dev.yaml`
                 if continuous dev hosting becomes useful again. The live
                 ArgoCD Application is at
                 `infra-bootstrap/k8s/apps/ambience.yaml`.
                 CI is manual / session-driven:
                 `.github/workflows/build-and-deploy.yml` has no automatic
                 triggers and only builds + pushes
                 `romainecr.azurecr.io/ambience:<sha>` when manually
                 dispatched. The follow-up image-tag bump commit is done
                 from the session by editing the appropriate Helm values
                 file, then ArgoCD picks up that committed chart change
                 and rolls out. Local k8s-first dev work uses
                 `scripts/dev-deploy.ps1`, which patches live
                 `edge`, `authority`, or `all` workload images for a
                 fast inner loop without treating dev as a manual Helm
                 release. The script prefers local Docker when it is
                 available, but can fall back to `az acr build` and
                 verifies the image tag exists before patching, so local
                 Docker is optional for the dev path. Test-slot validation
                 should deploy the CI-built image with
                 `deploy_image_to_test_slot` so browser assets and Go code
                 are exercised from the same artifact that PR CI proved.
```

## Pixel-world contract

The product is a persistent shared ambient pixel world, not a browser-only
canvas scene. Do not add effect visuals that bypass the pixel grid.

The intended contract is:

1. The authority/server coordinates state, effect/scene timing, config,
   discrete events, entropy, and snapshots.
2. Clients already have the effect code loaded. They locally simulate from
   snapshots/config/events such as "wind gust", "downpour", "intro", or
   "ending" instead of receiving streamed pixel frames.
3. Pixel streaming is intentionally avoided. It is too heavy for the goal and
   solves responsiveness when this project is mainly about persistence across
   clients and sessions.
4. Every effect owns a low-resolution pixel grid and updates that grid with
   effect-specific pixel math. Browser canvas, terminal sixel, and social PNG
   preview are output adapters for the grid.
5. Browser code must not draw ambience visuals with canvas-native gradients,
   paths, arcs, strokes, or other scene-rendering APIs. The shared renderer may
   use `fillRect` to scale grid cells to the canvas; effects should write
   pixels into their Go grids.
6. Entropy should be felt later, not as immediate frame noise. A good use is
   biasing future generated configurations/scenes while clients continue to
   converge from the event stream and their playback buffer.

If an effect needs richer art direction, refine its grid math. Do not add a
parallel non-grid rendering layer.

## Preferred iteration loop

When the deployment/platform foundation is already stable, prefer using
that stability to ship a scoped product slice instead of starting another
infra refactor.

For future Codex sessions, the default loop should be:

1. Re-open the open Glimmung issues and open GitHub PRs at the start of a fresh
   session so the next slice is chosen from the real backlog, not memory.
2. Pick one concrete, bounded feature. Bias toward effect work or
   effect-adjacent enhancements when the existing registry, `/dev`
   switcher, presets, and schema-driven controls already make that cheap.
3. Treat `ambience.dev.romaine.life` as the default test target. Do not
   spin up or rely on a local runtime unless the user explicitly asks for
   a localhost repro.
4. For browser-only static work in `cmd/ambience/web`, use the image deploy
   loop; browser assets are baked into the app image.
5. For effect work that changes authority-side Go together with
   `cmd/ambience/web`, patch the dev environment with the combined image.
6. For edge or shared image-backed changes, patch the dev
   environment with
   `powershell -ExecutionPolicy Bypass -File scripts/dev-deploy.ps1 -Component all`.
   If the change is only edge-side or only authority-side, use `edge` or
   `authority` respectively.
7. Validate on `ambience.dev.romaine.life`, usually via
   `/dev/<effect>` for new or changed dev effects, before treating the
   slice as ready for promotion.
8. Only after the dev environment looks right should the change move into
   the manual production promotion flow: build/push the real image, bump
   the Helm values file, commit that desired-state change, and let ArgoCD
   reconcile it.

This is the preferred default for future iterations unless the backlog
item is explicitly infra-only or the current platform work is still
blocking product progress.

## Atmosphere model

The server does NOT broadcast pixel frames. Instead, each atmosphere is a
server-side sim running at 60 Hz whose job is to DECIDE when discrete
events fire and when to rotate scenes. Clients run their own sims locally
and apply five kinds of commands:

- **`snapshot`** — initial state dump on connect. The outer envelope is
  effect-generic:
  `{type, tick, config, state, seed, gridW, gridH, currentScene,
  nextScene, entropyBytes, sceneRemaining}`. `config` and `state` are
  effect-owned blobs; `type` tells clients which constructor to use.
- **`config`** — sim config changed; clients call `setConfig`. Broadcast on
  entry to scene transitions (at ~1 Hz during drift) and on transition
  completion (final target for exact sync).
- **`trigger`** — an event fired (downpour/calm/gust/splash); clients apply.
- **`scene`** — scene rotated. Carries `{name, durationTicks, startedAtTick,
  nextName, transitionTicks}` so UI panels update + interpolated tick
  interpolation works.
- **`metric`** — entropy bytes + scene-remaining snapshot. Event-driven,
  not periodic — fires on every `AddEntropy` call and on scene rotation.

Clients do NOT roll for discrete events — only the server does. Clients
advance timers and physics locally. Frame-level sync is not guaranteed
(each client RNG drifts after initial snapshot), but event timing
(downpour starts/ends, calm windows, scene changes) stays in sync.

## Scene rotation

Rain runs with generated scenes rather than a single fixed config.
Scenes have a 1–4 h duration, two-slot lookahead (current + next, next
pre-generated at rotation time using the atmosphere's RNG — entropy
perturbs the RNG, so keystrokes bias future scenes with a one-scene
delay). Scene names are auto-derived descriptors (`warm-fast-drizzle`,
`cool-calm-downpour`) from hue/speed/spawn buckets.

Transitions between scenes are a config DRIFT — `lerpConfig` over
`min(60 s, sceneDur/2)` ticks with `easeInOutCubic` easing. Hue uses
angular LERP so the 0/360° seam is crossed along the shortest arc.
Server applies interpolated configs to its sim each tick and broadcasts
every 10 ticks during drift (~1 Hz), so client replicas stay near-sync
without overwhelming the SSE stream. The larger cross-effect transition
design is tracked in Glimmung; the current work is the within-effect
half. Vocabulary (Scene vs Effect) is tracked in Glimmung.

For local testing, `AMBIENCE_SCENE_TICKS=60` env var shortens scene
duration to 6 s so rotations fire visibly without a 90-minute wait.
Production ignores the var (falls back to 1–4 h random).

## Effect registry

`wasm_runtime.js` loads `ambience.wasm` and registers Go-backed
constructors in `AmbienceSim.effects` for every supported effect. The shared
`client.js` reads `snapshotData.Type` and looks up the constructor there.
Adding a new active effect means adding the Go `sim` runtime and exposing it
through `cmd/ambience-wasm`; no per-consumer wiring should be needed. The
5-slot effect template (spawn / lever / event / event-mod / end) is tracked
in Glimmung.

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

## Entropy flow

Browsers capture `keydown` events and POST a byte per keystroke
(`key.charCodeAt(0) ^ Date.now() & 0xff`) to `/entropy` every 2s
(throttled, max 4KB per request). The server folds bytes into the
shared atmosphere's seed and re-seeds the sim's RNG via
`Rain.PerturbRNG(delta)`. Future random decisions drift — typing subtly
steers the world. Cheap, lossy, intentional — this is aesthetic
perturbation, not secure randomness.

## Consumer integration

The shared auto-init pattern. Drop this into any page:

```html
<canvas data-ambience></canvas>
<script src="https://ambience.romaine.life/sim.js"></script>
<script src="https://ambience.romaine.life/client.js"></script>
```

Then CSS (optional, for layered overlay):

```css
#ambience-canvas { position: fixed; inset: 0; z-index: 0; pointer-events: none; }
body.ambience-on { --fzt-bg: transparent; }  /* or other opacity overrides */
```

`client.js` adds `body.ambience-on` on successful init — consumer CSS
can conditionally adapt so the page still renders correctly if ambience
JS fails to fetch. Configuration via `data-ambience-*` attrs on the
canvas: `url`, `grid-w`, `grid-h`, `transparent`, `entropy`.

Consumers:

- **`/`** — live broadcast monitor. Full-screen canvas running the shared
  replica sim through Go/WASM (via inline EventSource + `wasm_runtime.js`,
  not `client.js` — the page consumes all five command kinds including
  `scene`/`metric` which `client.js` doesn't know about). Status-panel overlay shows current scene
  name, remaining time, up-next, tick, entropy counter with togglable
  capture, and a rolling event log. Dev-ish tools here are a
  fallback — `/dev` remains the real knob-tuning surface.
- **`/dev`** — per-session dev-atmosphere knob-tuning page. NOT connected to
  the shared broadcast; each session gets its own isolated atmosphere for
  testing effects/configs without interfering with prod state. The page
  supports effect switching via `/dev/<effect>`, effect-defined presets,
  randomized per-session starting configs, and a `randomize` button for
  another quick stat roll on the active effect.
- **fzt-showcase** — `<canvas data-ambience>` behind the WASM DOS terminal.
- **my-homepage** — `<canvas data-ambience>` behind the fzt bookmark terminal.
- **fzt-automate** (terminal) — imports `github.com/romaine-life/ambience/terminal`,
  paints sixel via `tui.PostFrameHook`. Currently has known rendering
  issues tracked in #11–#15; on pause.

## Decisions settled

- Name: `ambience` (matches `ambience.romaine.life`)
- Language: Go server + sim + terminal client; JS for browser consumers
- First effect: Rain
- Shared state is global across all consumers/profiles
- K8s-native deploy — first app on the per-app deployment pattern
  (see `infra-bootstrap/k8s/apps/ambience.yaml`)
- Effects plug in via `AmbienceSim.effects` registry; server broadcasts
  `snapshotData.Type` so clients know which constructor to pick. Adding
  Sand/Fire/Tetris requires zero client changes.
- Repo migration from `nelsong6/` to `romaine-life/` is tracked in
  Glimmung (May 2026)

## Status

Rain MVP live at `ambience.romaine.life` with scene rotation + smooth
drift transitions + live monitor panel at `/`. The dev environment at
`ambience.dev.romaine.life` is not currently deployed — its values file
(`chart/ambience/values-dev.yaml`) and wildcard listener remain in
reserve. `/dev` now supports effect switching across Rain,
Fireflies, Waterfall, and Dust, including randomized per-session starting
stats, a `randomize` button, and a bottom-anchored event log that behaves
like a live tail instead of pinning the newest entry off-screen. The web
surface is tightened around clean routes: use `/` and `/dev/<effect>`,
not `.html` paths, and unknown static routes now 404. Consumers:
fzt-showcase + my-homepage integrated via shared client (unaffected by
scene/metric commands — client.js ignores unknown kinds). Entropy intake
wired, visible on the `/` panel. Shared atmosphere state now persists
across authority restarts via Cosmos DB (database `ambience`, container
`atmosphere`, document id `shared`) using workload-identity auth.
Terminal integration tabled, see `docs/terminal-integration-status.md`.
Future effects ready to plug in via the registry.
