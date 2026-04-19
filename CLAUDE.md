# ambience

Shared-world ambient pixel-art effect system. Server runs the simulation; every client (browser canvas, terminal sixel) is a thin renderer reading the same SSE stream. Conceptually independent of fzt — the name only matches the domain (`ambience.romaine.life`). Read `D:/shell-config/setup/claude/CLAUDE.md` for global Claude config.

## Architecture

```
cmd/server  (Go)   Runs the active effect as a goroutine at ~10 Hz.
   │               Broadcasts { w, h, pixels[] } frames via SSE.
   │               Embeds web/ (index.html + ambience.js) and serves them.
   │
   ├─► /stream        SSE endpoint: one JSON frame per tick
   ├─► /ambience.js   Browser canvas renderer module
   ├─► /              Full-screen demo HTML page
   └─► /entropy       (future) POST keystroke bits that reseed the RNG

sim/                  Pure Go simulation logic. Grid + Step(). No I/O.
terminal/   (future)  Go package: sixel renderer + SSE subscriber, consumed by fzt-automate.
```

The sim and the server are separated so the harness / tests / future terminal client can all use the same simulation code without pulling in HTTP.

## Effect Template (5 slots)

Every effect fills these:

1. **Spawn config** — params randomized at effect start (palette, scene layout, base rates)
2. **Continuous levers** — micro-drift fed by keystroke entropy (hue shift, rate jitter)
3. **Discrete events** — periodic bursts/transitions (downpour, container spawn, ignition)
4. **Event modifiers** — per-event randomization (duration, intensity, target hue)
5. **End conditions** — natural conclusions (e.g., tetris lose-state); optional

See [#1 architecture issue](https://github.com/nelsong6/ambience/issues/1).

## Why ambient, not responsive

Keystrokes don't directly spawn visible elements — they feed entropy bits into the sim's RNG. The sim always runs; typing subtly steers the pattern. Because there's no immediate "response moment," cross-tab SSE roundtrip latency is invisible and tabs stay frame-perfect.

## First-class standalone

`ambience.romaine.life` root URL IS the canonical live view. Opening it in a browser renders the exact grid every other consumer is rendering. No fzt UI, no framing — just the ambience.

## Consumers (planned)

- **my-homepage** (browser): imports `ambience.js` from CDN, points it at `ambience.romaine.life`
- **fzt-automate** (terminal): imports `github.com/nelsong6/ambience/terminal` Go package, renders via sixel
- **ambience itself** (demo page at `/`): same `ambience.js`, same stream

## Decisions settled

- Name: `ambience` (matches `ambience.romaine.life`)
- Language: Go for server + sim + terminal client; JS for browser
- First effect: Rain ([#2](https://github.com/nelsong6/ambience/issues/2))
- Shared state is global across all consumers/profiles
- K8s-native deploy — first app on the per-app deployment pattern (see infra-bootstrap)
- Repo migration from `nelsong6/` → `romaine-life/` tracked by [#10](https://github.com/nelsong6/ambience/issues/10) (May 2026)

## Status

Pre-MVP. `go run ./cmd/server` serves a local Rain loop end-to-end; no entropy input yet, no persistence, no k8s manifests.
