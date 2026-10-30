# ambience

Shared-world ambient pixel-art effect system. Server coordinates state;
every client (browser canvas, terminal sixel) runs its own sim replica
that receives config + event broadcasts via SSE. Conceptually independent
of fzt — the name only matches the domain (`ambience.romaine.life`).
Read `D:/shell-config/setup/claude/CLAUDE.md` for global Claude config.

**The goal** (AMBIENCE-1, AMBIENCE-50): one shared world that every open
screen shows at the same moment, like synchronized displays rather than
separate animations that receive the same events, without streaming pixels.
It is a slow background, so clients play a little behind the authority to buy
that sync, and playback must still look smooth: never frozen, never sped up.

## Taking screenshots (read this before trying to screenshot the app)

**The in-editor preview/screenshot tool's *capture* step hangs on this machine —
every grab times out at ~30s, even on a blank page. The dev server is fine; only
the pixel grab is broken.** Do NOT retry it, and do not tell the user screenshots
are impossible. Use `scripts/shot.mjs` (headless Chrome over the DevTools
Protocol — dependency-free on the installed Node), then Read the PNG:

```
# 1. server running locally (preview-managed or: AMBIENCE_ADDR=127.0.0.1:8080 go run ./cmd/ambience)
# 2. capture a deep-linked route, then Read the PNG it writes
node scripts/shot.mjs "http://127.0.0.1:8080/dev?effect=rain&render=vector" tmp-shots/vec.png 1280x720 5000
```

Args: `<url> [outPath=tmp-shots/shot.png] [WxH=1280x720] [waitMs=4500]`. `/dev`
takes effect + knobs as query params (see `docs/dev-endpoints.md`), so any
effect/config/render-mode is reachable as a deep link — no clicking needed.
`tmp-shots/` is gitignored. Two ambience-specific reasons the naive
`chrome --screenshot` / `--virtual-time-budget` approaches fail (and this helper
handles): the user's normal Chrome is usually already running (a plain launch
no-ops — the helper uses a throwaway `--user-data-dir`), and ambience pages hold
a persistent `/events` SSE stream so the page never reaches network-idle
(`--virtual-time-budget` hangs forever — the helper waits a fixed real-time delay
instead). `waitMs` is that settle delay: bump it if the sim hasn't accumulated a
representative frame.

## Container Build Verification

Local Docker is not required. Do not report missing local Docker as a
blocker. Run available repo checks first, then use PR CI as the normal
container build gate: `.github/workflows/docker-build-check.yaml` computes
the app image fingerprint and reuses or pushes an ACR proof image for trusted
PRs, falling back to `push: false` for fork PRs. If image-packaging feedback is needed before a
PR is ready, manually dispatch that workflow with `git_ref`. Release/deploy
workflows publish the fingerprint-tagged images used by deploys.

## Testing

There is no autonomous agent pipeline: Glimmung, which ran one for this repo,
is gone (AMBIENCE-69). Effects are verified by hand on a local server, using
the `/dev` endpoints, `scripts/shot.mjs` screenshots and
`scripts/agent/selfcheck-effect.sh` for lifecycle claims; the recipe is in
[docs/dev-endpoints.md](docs/dev-endpoints.md).

When developing a new effect, the relevant references are
[docs/effects-cookbook.md](docs/effects-cookbook.md) (file pattern +
touchpoint checklist) and [docs/dev-endpoints.md](docs/dev-endpoints.md)
(server endpoints used for verification videos/screenshots and event
triggers).

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
  ├─► /client.js         Shared auto-init browser client (see "Consumer
  │                      integration" below)
  ├─► /ambience.js       Older helper (pre-client.js); still served
  ├─► /dev               Dev-knob page for tuning Rain config live (per-
  │                      session dev atmosphere, NOT connected to broadcast)
  ├─► /snapshot          JSON: current shared atmosphere state (incl. scene)
  ├─► /events            SSE: atmosphere commands
  │                      (snapshot/config/trigger/scene/metric)
  ├─► /entropy           POST bytes — folded into the shared RNG
  ├─► /effects/:effect/schema  JSON schema for an effect's dev-panel knobs
  └─► /dev/{snapshot,events,config,trigger}  Per-session dev atmospheres

sim/             Pure Go simulation logic, one file per effect.
                 No I/O; consumed by cmd/ambience (server-side) and by
                 terminal/ (client-side sixel renderer).

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
                 `.github/workflows/build-and-deploy.yml` builds + pushes
                 `romainecr.azurecr.io/ambience:app-<fingerprint>`. On
                 pushes to main it commits the corresponding fingerprint
                 tag into the prod and base Helm values files, then ArgoCD
                 picks up that committed chart change and rolls out. Manual
                 dispatch can prebuild a requested `git_ref` without
                 changing chart desired state.
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

## Atmosphere model

The server does NOT broadcast pixel frames. Instead, each atmosphere is a
server-side sim running at 60 Hz whose job is to DECIDE when discrete
events fire and when to rotate scenes. Clients run their own sims locally
and apply five kinds of commands:

- **`snapshot`** — initial state dump on connect. Full game-save:
  `{type, tick, config, state, seed, gridW, gridH, currentScene,
  nextScene, entropyBytes, sceneRemaining}`. `type` is the effect name
  ("rain" for now); `config` and `state` are the effect-specific payloads.
- **`config`** — sim config changed; clients call `setConfig`. Broadcast on
  entry to scene transitions (at ~1 Hz during drift) and on transition
  completion (final target for exact sync).
- **`trigger`** — an event fired (downpour/calm/gust/splash); clients apply.
- **`scene`** — scene rotated. Carries `{name, durationTicks, startedAtTick,
  nextName, transitionTicks}` so UI panels update + interpolated tick
  interpolation works.
- **`metric`** — entropy bytes + scene-remaining snapshot. Event-driven,
  not periodic — fires on every `AddEntropy` call and on scene rotation.

Clients do NOT roll for discrete events — only the server does. Every
command is stamped with the world tick, which keeps counting across effect
rotations; clients play that tick 1.5 s behind the authority so each command
lands on the same tick on every screen, correcting drift by running a few
percent fast or slow and closing larger gaps in one undrawn frame (README,
"Browser playback"). Pixel-identical frames are not the goal; the same
world at the same moment is.

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
without overwhelming the SSE stream.

For local testing, `AMBIENCE_SCENE_TICKS=60` env var shortens scene
duration to 6 s so rotations fire visibly without a 90-minute wait.
Production ignores the var (falls back to 1–4 h random).

## Effect registry

`wasm_runtime.js` loads `ambience.wasm` and registers Go-backed
constructors in `AmbienceSim.effects` for every supported effect. The shared
`client.js` reads `snapshotData.Type` and looks up the constructor there.
Adding a new active effect means adding the Go `sim` runtime and exposing it
through `cmd/ambience-wasm`; no per-consumer wiring should be needed.

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
  replica sim through Go/WASM, played by `client.js`'s shared
  `AmbiencePlayback` engine (loaded with `data-ambience-library`; the page
  handles `scene`/`metric` itself for its panel). Status-panel overlay shows current scene
  name, remaining time, up-next, tick, entropy counter with togglable
  capture, and a rolling event log. Dev-ish tools here are a
  fallback — `/dev` remains the real knob-tuning surface.
- **`/dev`** — per-session dev-atmosphere knob-tuning page. NOT connected to
  the shared broadcast; each session gets its own isolated atmosphere for
  testing effects/configs without interfering with prod state.
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
- Repo moved from `nelsong6/` to `romaine-life/` (May 2026)

## Status

Rain MVP live at `ambience.romaine.life` with scene rotation + smooth
drift transitions + live monitor panel at `/`. Consumers: fzt-showcase +
my-homepage integrated via shared client (unaffected by scene/metric
commands — client.js ignores unknown kinds). Entropy intake wired,
visible on the `/` panel. Terminal integration tabled, see
`docs/terminal-integration-status.md`. Shared atmosphere state persists
across authority restarts via Cosmos DB (database `ambience`, container
`atmosphere`, document id `shared`) using workload-identity auth — the
`ambience-identity` user-assigned identity is federated to the prod
namespace's `default` ServiceAccount with data-plane scope narrowed to
`dbs/ambience`. Leaving `authority.cosmos.endpoint` unset runs without
persistence. Future effects are ready to plug in via the
registry.
