# Effects cookbook

Pattern for adding a new ambient effect to ambience. The current
canonical example is `burning-trees`.

## Three-files-only pattern

Every effect lands as **three new files** plus a small set of edits
to a fixed list of registry files. The edits are short and identical
in shape across all effects.

### New files

- `sim/<effect>.go` — pure-Go simulation type. Owns the pixel grid,
  the per-tick step, snapshot/restore, and `TriggerEvent`. No I/O.
  Compare to `sim/burning_trees.go`.
- `sim/<effect>_test.go` — schema test plus a couple of behavior
  tests (event flow, snapshot round-trip). Compare to
  `sim/burning_trees_test.go`.
- `cmd/ambience/effect_<effect>.go` — small runtime adapter that
  registers the effect via `register(effectDefinition{...})` in an
  `init()`. Compare to `cmd/ambience/effect_burning_trees.go`.

### Touchpoint edits

Every new effect must edit each of these:

- `cmd/ambience/effect_frames.go` — add a one-line `Frame()` method.
- `sim/gridcopy.go` — add a one-line `GridCopy()` method.
- `cmd/ambience-wasm/main.go` — add the effect to the slice + switch.

That is the full list. There are no other shared files to touch for
a fresh effect — if you find yourself editing
`procedural_renderers.go` or any of the older shared renderer code,
you are doing the *legacy* shape; prefer self-contained.

## Helpers available

The shared helpers live in `sim/rain.go` and `sim/proc_helpers.go`
(despite the names — these are not rain-specific):

- `hslToRGB(h, s, l)` — color conversion
- `clamp01(v)` — value clamping
- `phaseProgress(total, left)` — eased phase progress
- `jitterInt(rng, base, spread)` — randomized int

`rngutil.RNG` is the seeded RNG. `rngutil.New(seed int64)` for
construction; `Uint64`, `Int63`, `Intn`, `Float64` are the methods.

## Schema

Every effect exposes an `EffectSchema` (used by the dev-knob page).
Compare to `sim.BurningTreesSchema()`. Keys are knob names; types
use the standard `KnobType` set in `sim/schema.go`.

## Lifecycle states vs transient events

Every `TriggerEvent` your sim handles is one of two kinds, and the
distinction is load-bearing — get it wrong and the effect looks correct
at a glance but fails verification:

- **Transient event** (e.g. `pulse`, `power-surge`, `ember-burst`):
  fires, peaks, then **decays back to the prior resting state**. The
  world returns to exactly what it was before. Implement as a bounded
  envelope — a timer that expires and leaves no lasting change.

- **Terminal lifecycle state** (e.g. `intro`, `ending`): changes the
  world's **resting state** and *holds it* until another lifecycle
  trigger moves it. `intro` ignites from dark to the normal resting
  look; `ending` resolves to its terminal look — for a portal, "the gate
  goes dark" — **and stays there.** It must not revert to normal
  breathing once the outro animation finishes.

**Invariant: after a terminal lifecycle trigger completes, the resting
frame is its terminal state.** The classic bug is implementing `ending`
as a transient envelope (dim, then snap back to full brightness when the
timer expires) — that reverts the world instead of ending it, and a
verifier inspecting the final frame reports the claimed end state was
not observed. Drive the resting look from a **persisted lifecycle state**
(carried through snapshot/restore so it survives an authority restart),
not only from a soon-to-expire timer.

Wire lifecycle triggers as named events your sim's `TriggerEvent` knows
about. The authority server fires them at scene boundaries; the dev page
exposes manual triggers via `/dev/trigger/<session>/<event>` (see
`docs/dev-endpoints.md`). The issue contract classifies each trigger as
transient or terminal and names the terminal resting state — treat that
classification as binding, and verify your terminal states actually hold
(see "Self-checking visual behavior" below).

## Self-checking visual behavior

`go build` and `go test` cannot see pixels, so a visual or temporal
claim ("the gate goes dark", "the surge brightens then fades") is not
proven by a green Go build. Prove it two ways before claiming pass:

1. **Deterministic end-state assertion (Go).** Step the sim through a
   trigger and *past* the outro/envelope expiry, then assert the resting
   state via `GridCopy()` — a terminal `ending` leaves the gate at/below
   its terminal brightness and stays there; a transient event returns to
   baseline. This makes "the world reverted" a failing test.
2. **Visual self-check (localhost browser).** Build and run your
   in-progress code locally and watch it with the same capture/inspect
   tooling the verification stage uses. See the implementation prompt for
   the exact localhost recipe.

## Validation flow

For each new effect, the agent flow's verification stage will:

1. Hit `/dev/<effect>` against the rebuilt validation env to record
   a default WebM video.
2. POST to `/dev/trigger/<session>/<event>` for any lifecycle events
   of interest and record another WebM.
3. Run `go test ./sim/ -run <Effect>`.

These show up as `required_evidence` items in the test-plan stage
output. Screenshots can supplement video when a still frame needs
separate inspection — see `docs/issue-agent-stage-split.md`.
