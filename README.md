# ambience

Shared-world ambient pixel-art effects. A 10 Hz server decides when
discrete events fire (downpour, calm, gust, splash) and broadcasts those
as commands via SSE; every consumer (browser canvas, terminal sixel)
runs its own sim replica and applies the commands in sync. Rain is the
only effect live today; more are planned.

Canonical live view: <https://ambience.romaine.life>.

## Quick start

```sh
go run ./cmd/ambience
# open http://localhost:8080/
```

`/` renders the current effect full-screen. `/dev` opens a per-session
knob-tuning page with 27 live-adjustable parameters.

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
                 of the sim), client.js (auto-init shim for consumers),
                 dev.html (knob-tuning page).

sim/             Pure Go simulation logic. No I/O. Consumed by the
                 server and by the terminal client.

terminal/        SSE subscriber + local sim replica + sixel renderer
                 as a Go package. Consumed by fzt-automate.

tools/conpty-capture/
                 Python/pywinpty tool that captures Windows terminal
                 byte streams including sixel DCS blocks (which
                 PowerSession misses). For debugging sixel rendering
                 inside fzt-automate.

k8s/             Kubernetes manifests. Deployed via ArgoCD watching
                 this path on `main`.
```

## Atmosphere model

The server does not broadcast pixel frames. Each atmosphere is a
server-side sim running at 10 Hz whose job is to decide when discrete
events fire. Clients run their own sims locally and apply three kinds
of commands:

- **`snapshot`** — state dump on connect: `{type, tick, config, seed,
  downpourLeft, ...}`. `type` is the effect name — clients use it to
  pick which sim constructor to instantiate.
- **`config`** — sim config changed; clients apply via `setConfig`.
- **`trigger`** — a discrete event fired; clients apply its effects.

Clients do not roll for discrete events — only the server does. Each
client's RNG drifts from the server's after the initial snapshot, but
event timing stays in sync.

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
no consumer-side update.

## Entropy

Client-side keystroke capture feeds bytes into `POST /entropy`. The
server folds them into the shared atmosphere's seed and re-seeds the
sim RNG via `Rain.PerturbRNG(delta)`. Future random decisions drift —
typing subtly steers the pattern. Cheap and lossy on purpose; this is
aesthetic perturbation, not secure randomness.

## Endpoints

All broadcast endpoints set permissive CORS for cross-origin consumers.

- `GET  /` — demo page
- `GET  /dev` — dev-knob page
- `GET  /sim.js`, `/client.js` — consumer scripts
- `GET  /snapshot` — current atmosphere state (JSON)
- `GET  /events` — atmosphere command stream (SSE)
- `POST /entropy` — raw bytes folded into the RNG (max 4KB/req)
- `GET  /effects/rain/schema` — knob schema for the dev UI

## Deploying

```sh
az acr build --registry romainecr --image ambience:latest .
kubectl rollout restart deployment/ambience -n ambience
```

The ArgoCD Application lives in `infra-bootstrap/k8s/apps/ambience.yaml`
and watches this repo's `k8s/` path on `main`. Manifest changes sync
automatically; image updates need an explicit rollout because the tag
is `:latest` with `imagePullPolicy: Always`.

## Status

Rain effect live. Consumers: ambience's own demo, fzt-showcase (DOS
terminal), my-homepage (bookmark terminal). Entropy flow wired.

Terminal integration via fzt-automate is tabled pending platform
rendering work — see issues
[#11–#15](https://github.com/nelsong6/ambience/issues?q=is%3Aopen+label%3Aterminal-client).

Repo migration `nelsong6/` → `romaine-life/` tracked by
[#10](https://github.com/nelsong6/ambience/issues/10).
