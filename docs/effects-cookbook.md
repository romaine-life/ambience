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

## Intros and outros

If your effect has lifecycle (intro, ending, transitions), define
those as named events your sim's `TriggerEvent` knows about. The
authority server fires them at scene boundaries; the dev page
exposes manual triggers via `/dev/trigger/<session>/<event>` (see
`docs/dev-endpoints.md`).

## Validation flow

For each new effect, the agent flow's verification stage will:

1. Hit `/dev/<effect>` against the rebuilt validation env to capture
   a "default" screenshot.
2. POST to `/dev/trigger/<session>/<event>` for any lifecycle events
   of interest and re-screenshot.
3. Run `go test ./sim/ -run <Effect>`.

These show up as `required_evidence` items in the test-plan stage
output — see `docs/issue-agent-stage-split.md`.
