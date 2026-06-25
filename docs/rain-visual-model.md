# Rain Visual Model

Rain uses three perceptual depth layers rather than one physical particle
field.

- **Front plane** models rain crossing the viewer's screen/window plane. This
  is anchored to monitor-height intuition: near drops cross in only a few
  frames, so they render as sparse, short-lived swept streaks instead of
  tracked drops falling through the whole viewport. These are event-like
  streaks, not persistent streams: a drop that exits the viewport should not
  wrap back into the same screen-space path. Their visual length includes
  exposure from the distance swept during a tick so high-speed rain reads as
  motion rather than a harsh flash.
- **Mid/background tracked drops** are falling streaks inside the visible scene
  volume. Each drop is assigned a synthetic **distance** and every perceptual
  cue derives from it together (see "Depth coherence" below).
- **Sheet rain** is a dense distant weather texture. It fills the atmosphere and
  preserves rain continuity without implying individually trackable drops.

## Resolution independence

Fall speed and all streak/sheet/front lengths are expressed in **rows per tick**
and **cells**, calibrated against a reference grid height of **180** rows
(`refGridH`). At that reference a tracked-drop speed of ~1.8 rows/tick crosses
the viewport in ~1.7s, which reads as mid-scene rain; the front plane runs much
faster (~40–72 rows/tick, crossing 180 rows in ~2.5–4.5 frames).

Because clients adopt the broadcast grid (currently **640×360**), the sim scales
every screen-relative quantity by `resScale() = max(1, H/180)`:

- **scaled with resolution** (so screen-relative motion/proportions are constant
  at any grid): tracked-drop speed, tracked-drop streak length, sheet speed +
  length, front speed + length + exposure, splash radius.
- **NOT scaled** (stays in cells, so a finer grid means *thinner* drops — the
  "smaller pixels" look): drop width.

This is the invariant: **raising the grid only makes the pixels finer. It must
never slow the rain or shorten the streaks.** (Bumping 180→360 without this made
every drop fall 2× too slow — a real regression. `resScale` is clamped to ≥1 so
small grids — tests, the terminal — keep the config's literal values.)

## Depth coherence (why slow ⇒ far)

A tracked drop's speed must be **correlated with its synthetic distance**, or
slow drops read as arbitrarily slow rather than distant. When layering is on
(`layers ≥ 2`) each drop draws a distance `t ∈ [0,1]` (0 near, 1 far, `sqrt`-
biased toward the far field) and all cues derive from it **together**:

| cue        | near (t=0) | far (t=1) | physical basis                                   |
|------------|-----------|-----------|--------------------------------------------------|
| fall speed | ×1.0      | ×0.6      | apparent angular velocity ∝ v/distance           |
| streak len | ×1.0      | ×0.35     | apparent size + motion-blur ∝ 1/distance         |
| brightness | ×1.0      | ×0.5      | atmospheric haze with distance                   |
| width      | `drop_width` cells | 1 cell | apparent size ∝ 1/distance (only when `drop_width > 1`) |

`drop_width` defaults to 1, so the default look is thin one-cell rain whose
*speed, length and brightness* still track distance — a slow drop is visibly a
small, dim, short drop. Raising `drop_width` additionally fattens near drops.
The physical drift: near drops are both closer and tend to be the larger,
faster-falling drops, so they appear big/fast/long/bright; far drops the
opposite.
