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

## Physical fall speed (tracked drops)

Tracked-drop fall speed is **derived from real raindrop physics**, not an
arbitrary rows/tick, so the rain reads as real rain rather than uniform synthetic
streaks. Two independent axes are sampled per drop:

- **size** `dMm` — drop diameter, 0.5–4.5mm, biased small (Marshall–Palmer: real
  rain is mostly small drops with a few large ones).
- **distance** `depthT ∈ [0,1]` — how far the drop is from the viewer.

**Terminal velocity** comes from the Gunn–Kinzer (1949) fit
`v(d) = 9.65 − 10.3·e^(−0.6d)` m/s — ~2 m/s at 0.5mm rising to ~9 m/s near 5mm.
*This is the dominant source of realistic speed variety:* big drops genuinely
fall ~4× faster than small ones, which is what gives rain its mix of fast bold
streaks and slow faint drizzle. (A single-axis distance model misses it and
reads sluggish — there are no fast streaks at all.)

Apparent (on-screen) velocity = terminal velocity / distance. The projection to
the grid is physical:

```
effSpeed (rows/tick) = v(dMm) · distFall · (Speed/refSpeedKnob) · H / (worldHeightM · clientFPS)
```

`worldHeightM` (≈4.5m) is the one genuinely artistic choice — how many metres of
falling rain the viewport spans. Smaller = closer/faster rain. Because the
projection multiplies by grid height `H`, **fall speed is automatically
resolution-independent**: a finer grid only makes the pixels thinner, never
slower (multiply by `H` instead of patching a per-grid scale factor).

Measured crossing-time distribution (640×360, nominal scene): fastest big near
drops ~**0.4–0.5s**, on-screen median ~**0.85s**, distant drizzle ~**2.4s** —
matching real rain (a 4.5mm drop at ~8.9 m/s crosses 4.5m in ~0.5s).

## Coherence of the other cues

Everything else derives from the same size+distance so a slow drop is *visibly*
a distant/small one, never arbitrarily slow:

| cue        | driver                                   | physical basis                          |
|------------|------------------------------------------|-----------------------------------------|
| streak len | ∝ apparent velocity (`v/v_ref · distFall`) | motion blur over the exposure         |
| brightness | bigger + nearer = brighter               | drop cross-section + atmospheric haze   |
| width      | `drop_width` cells at the biggest/nearest, tapering to 1 | apparent diameter ∝ size/distance |

`drop_width` defaults to 1, so the default look is thin one-cell rain whose
speed, streak and brightness still track size/distance. Raising it fattens the
largest near drops.

## Resolution independence (lengths)

Streak/sheet/front lengths and splash radius are still in **cells**, calibrated
at a reference grid height of **180** (`refGridH`) and scaled by
`resScale() = max(1, H/180)` so they stay screen-proportional at any grid. (Fall
speed no longer needs this — its projection already includes `H`.) `resScale` is
clamped to ≥1 so small grids — tests, the terminal — keep literal cell values.
Drop **width** is deliberately *not* scaled, so a finer grid = thinner drops.
