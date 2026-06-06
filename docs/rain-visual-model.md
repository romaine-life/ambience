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
- **Mid/background drops** are tracked falling streaks inside the visible scene
  volume. Foreground tracked drops move faster; background tracked drops move
  slower, dimmer, and with shorter streaks for parallax.
- **Sheet rain** is a dense distant weather texture. It fills the atmosphere and
  preserves rain continuity without implying individually trackable drops.

The default browser surface is 320 by 180 cells at 60 Hz. A tracked drop speed
of 1.8 rows per tick crosses the viewport in roughly 1.7 seconds, which reads
as mid-scene rain. The front-plane layer intentionally runs much faster:
roughly 40-72 rows per tick in generated scenes, crossing the 180-row viewport
in about 2.5-4.5 frames.
