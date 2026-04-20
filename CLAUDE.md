# ambience

Shared-world ambient pixel-art effect system. Server coordinates state;
every client (browser canvas, terminal sixel) runs its own sim replica
that receives config + event broadcasts via SSE. Conceptually independent
of fzt — the name only matches the domain (`ambience.romaine.life`).
Read `D:/shell-config/setup/claude/CLAUDE.md` for global Claude config.

## Architecture

```
cmd/ambience (Go)     HTTP server. Runs the shared atmosphere goroutine
  │                   at 10 Hz — decides when discrete events fire
  │                   (downpour/calm/gust/splash) and broadcasts state
  │                   commands. Does NOT stream pixel frames.
  │                   Embeds web/ and serves it.
  │
  ├─► /                  Canonical demo page (runs local JS sim)
  ├─► /sim.js            JS port of sim/rain.go, `AmbienceSim` global
  ├─► /client.js         Shared auto-init browser client (see "Consumer
  │                      integration" below)
  ├─► /ambience.js       Older helper (pre-client.js); still served
  ├─► /dev               Dev-knob page for tuning Rain config live
  ├─► /snapshot          JSON: current shared atmosphere state
  ├─► /events            SSE: atmosphere commands (snapshot/config/trigger)
  ├─► /entropy           POST bytes — folded into the shared RNG
  ├─► /effects/rain/schema  JSON schema for Rain's 27 knobs (dev-panel UI)
  └─► /dev/{snapshot,events,config,trigger}  Per-session dev atmospheres

sim/             Pure Go simulation logic. Rain is the only effect today.
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

k8s/             Deployment manifests. ArgoCD Application lives in
                 infra-bootstrap/k8s/apps/ambience.yaml and watches this
                 path on main. Image tag is pinned by
                 `k8s/kustomization.yaml`; push-to-main CI
                 (`.github/workflows/build-and-deploy.yml`) builds
                 `romainecr.azurecr.io/ambience:<sha>`, bumps the kustomize
                 tag, commits back with [skip ci]. ArgoCD picks up the
                 kustomization change and rolls out.
```

## Atmosphere model

The server does NOT broadcast pixel frames. Instead, each atmosphere is a
server-side Rain sim running at 10 Hz whose job is to DECIDE when discrete
events fire. Clients run their own sims locally and apply three kinds of
commands:

- **`snapshot`** — initial state dump on connect: `{type, tick, config, seed,
  downpourLeft, downpourMult, calmLeft, gustLeft, gustWind}`. `type` is the
  effect name ("rain" for now; future effects carry their own type).
- **`config`** — sim config changed; clients call `setConfig`
- **`trigger`** — an event fired (downpour/calm/gust/splash); clients apply

Clients do NOT roll for discrete events — only the server does. Clients
advance timers and physics locally. Frame-level sync is not guaranteed
(each client RNG drifts after initial snapshot), but event timing
(downpour starts/ends, calm windows) stays in sync.

## Effect registry

`sim.js` exports `AmbienceSim.effects = { rain: Rain }`. The shared
`client.js` reads `snapshotData.Type` and looks up the constructor there
— adding a new effect means registering one entry in `effects`, no
client-side change, no per-consumer change. The 5-slot effect template
(spawn / lever / event / event-mod / end) is in the ambience repo
issues: [#1](https://github.com/nelsong6/ambience/issues/1).

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

- **`/`** — repo's own demo page, `<canvas id="c">` full-screen, not using
  the data-ambience auto-init (pre-dates it; uses sim.js directly).
- **`/dev`** — per-session dev-atmosphere knob-tuning page.
- **fzt-showcase** — `<canvas data-ambience>` behind the WASM DOS terminal.
- **my-homepage** — `<canvas data-ambience>` behind the fzt bookmark terminal.
- **fzt-automate** (terminal) — imports `github.com/nelsong6/ambience/terminal`,
  paints sixel via `tui.PostFrameHook`. Currently has known rendering
  issues tracked in #11–#15; on pause.

## Decisions settled

- Name: `ambience` (matches `ambience.romaine.life`)
- Language: Go server + sim + terminal client; JS for browser consumers
- First effect: Rain ([#2](https://github.com/nelsong6/ambience/issues/2))
- Shared state is global across all consumers/profiles
- K8s-native deploy — first app on the per-app deployment pattern
  (see `infra-bootstrap/k8s/apps/ambience.yaml`)
- Effects plug in via `AmbienceSim.effects` registry; server broadcasts
  `snapshotData.Type` so clients know which constructor to pick. Adding
  Sand/Fire/Tetris requires zero client changes.
- Repo migration from `nelsong6/` → `romaine-life/` tracked by
  [#10](https://github.com/nelsong6/ambience/issues/10) (May 2026)

## Status

Rain MVP live at `ambience.romaine.life`. Consumers: fzt-showcase +
my-homepage integrated via shared client. Entropy intake wired.
Terminal integration tabled, see `docs/terminal-integration-status.md`.
Future effects ready to plug in via the registry.
