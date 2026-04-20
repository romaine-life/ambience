# Terminal integration — status as of 2026-04-19

Tabled in favor of web-first development. This doc captures the state
so the next person (likely future-you or me) can pick it up cleanly.

## Goal

Render ambient rain inside fzt-automate, compositing over the menu TUI
so drops fall behind text and the terminal wallpaper shows through.

## Current state

**Working:**
- `ambience/terminal` Client — subscribes to server SSE, runs local sim,
  emits sixel via `Client.Render(w, col, row)`
- Byte-level capture on Windows via `tools/conpty-capture/` — pywinpty
  under ConPTY, records full output incl. sixel DCS blocks into an
  asciinema-compatible `.cast`
- `cmd/sixel-demo` — isolates the encoder path for testing
- Pb=1 DCS header rewrite (transparency enabled for non-drop pixels)
- Dynamic `Resize` on sim + client; `applyScaledConfig` scales Speed
  and SpawnBurst with grid size (WIP — not aesthetically tuned)
- `fzt-automate` ambience-widget branch: CONOUT$ sixel path, dynamic
  grid sizing from the `PostFrameHook`, `--clock` debug overlay
- `fzt-terminal` ambience-hook branch: removed `screen.Clear()` so
  tcell's differential rendering actually works — reduced terminal
  output ~6× and eliminated the 5 Hz flicker it was causing

**Not working / open:**
- See issues #11–#15 — in brief:
  - #11 — WT renders sixel raster opaquely after some seconds
    (probably a symptom of #12)
  - #12 — sixel pixels accumulate on screen; no clear mechanism
    between frames
  - #13 — drops cluster in top portion; don't reach full grid height
  - #14 — rain physics scaling not aesthetically tuned
  - #15 — sixel renders over TUI text; should render behind

## Why we tabled

The Windows Terminal + sixel + tcell interaction has more friction than
expected. Each fix exposes a new layer of the problem:

1. PowerSession doesn't capture sixels on Windows → built ConPTY tool
2. tcell's Show() was forcing full redraws → removed `screen.Clear()`
3. Now sixel pixels accumulate because nothing clears them → #12
4. Cells with opaque bg block sixel transparency → #11, #15

None of these are bugs — they're a stack of slightly-incompatible
design decisions compounding. The browser has none of them: canvas is
a single compositing surface, z-order is explicit, clearing is a
one-liner.

## Pivoting to web

Browser dev has:
- DOM/canvas for clean layering
- `mcp__Claude_Preview__*` tools for autonomous screenshot + inspect
- No sixel / ConPTY / CONOUT$ / alt-screen interaction to fight

The sim code is shared (Go server + JS port in `cmd/ambience/web/sim.js`),
so aesthetic work done in-browser transfers to any terminal port later.

## Paths back to terminal

When the time comes, options are:

- **Fix the open issues in sequence** — #12 first (clear mechanism),
  then re-evaluate #11, then decide on #15 (masking or accept overlap).
  Probably 1-2 days of focused work against the ConPTY harness.
- **Switch to a custom window** — rather than fighting WT's sixel,
  render into our own window (Tauri/Electron/native). The browser
  build is already most of this — we're ~1 Tauri shell away from a
  standalone ambience widget that could sit on the desktop.
- **Wait for ecosystem** — Kitty / iTerm2-style image protocols are
  gaining ground on Windows; if WT or a replacement supports them
  cleanly, we revisit.

## Tools / artifacts to keep

- `tools/conpty-capture/` — Python capture, still useful
- `cmd/sixel-demo/` — simplest sixel-output binary, keep around
- `terminal/client.go` — the Client is correct conceptually, only the
  scaling in `applyScaledConfig` is WIP
- `fzt-terminal@a7b5e2a` on `ambience-hook` branch — the screen.Clear()
  fix is independently valuable for any sixel-into-tcell work
- Analysis scripts in `/d/workspace/analyze_*.py` — parse cast files
  and surface flicker / density / CUP patterns
