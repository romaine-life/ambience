# Effect candidates

In-repo companion for the canonical rolling list in Glimmung. This doc
gives the backlog a place to grow with a little more shape than a flat
bullet line: a fit note against the 5-slot template, a one-line reason it
feels ambient, and current promotion status.

When a candidate is promoted, leave the row in place but mark it
`promoted: ambience#N` so the historical reasoning stays browsable next to
the implementation issue.

## Selection criteria

Same bar as #9, restated for convenience:

- **Small scene** — fits the effect canvas at low resolution.
- **Natural randomness** — maps cleanly to the 5-slot template
  (spawn / lever / event / event-mod / end). Without an event slot the
  candidate tends to read as a one-shot animation, not an atmosphere.
- **Unobtrusive** — reads as background; doesn't demand attention.
- **Pixel-art friendly** — silhouettes and motion read at low res, not
  reliant on fine gradients or subpixel detail.
- **At least one scene-variation knob** — intensity, hue, density, speed,
  rhythm. Without it, two scenes of the same effect feel identical.

## Starter list — status

Tracked here so the original backlog stays visible after promotion.
See #9's comment thread for the canonical promotion record.

| Candidate                          | Status                         |
| ---------------------------------- | ------------------------------ |
| Waterfall + mist + pool            | promoted: #21 (live)           |
| Rowboat on a lake                  | promoted: #22                  |
| Campfire + embers                  | promoted: #20                  |
| Windmill rotating                  | promoted: #23                  |
| Lighthouse beam sweeping           | promoted: #24                  |
| Train passing horizontally         | promoted: #25                  |
| Starfield with parallax layers     | promoted: #19                  |
| Snowfall in a pine forest          | promoted: #18                  |
| Beach with tide                    | promoted: #26                  |
| Wheat field waving                 | promoted: #27                  |
| Falling autumn leaves              | promoted: #28                  |
| Fireflies in a dark field          | promoted: #29 (live)           |
| Aurora borealis                    | promoted: #30                  |
| Volcano with occasional eruption   | promoted: #31                  |
| Underwater scene                   | promoted: #32                  |
| Mysterious man smoking             | promoted: #34                  |
| Pixel fish tank                    | candidate (likely #32 variant) |
| Sleeping dragon                    | candidate                      |
| Rain on a window                   | candidate                      |
| Lava lamp                          | candidate                      |
| VHS static / channel flip          | candidate                      |
| Snow globe                         | candidate (likely #18 variant) |
| Clock tower at dusk                | candidate                      |
| Constellations forming             | candidate (likely #19 variant) |

## New candidates

Grouped by feel. Each entry includes the game(s) that make the trope
recognizable and a one-line 5-slot fit note.

### Cozy / domestic

- **Cottage chimney smoke at night** — Stardew Valley, Harvest Moon. A
  warm-lit window plus a slow rising smoke plume that varies with wind.
  Spawn = puff emit at chimney; lever = wind drift; event = stronger
  gust pulse; event-mod = puff scatter; end = fade at top edge. The
  window-glow is the scene-palette knob.
- **Sleeping cat by a fireplace** — Stardew, Animal Crossing. Tail flick
  + breath chest rise. Lever = breath rate; event = ear twitch or tail
  flick; mostly a metronomic effect. Probably better as a layered
  foreground over a campfire (#20) scene than its own effect.
- **Owl on a branch in moonlight** — Stardew, Don't Starve at night.
  Mostly motionless silhouette with rare blink + occasional head turn.
  Strong silhouette, low attention demand. Scene variation comes from
  moon phase / cloud cover.

### Festival / celebration

- **Distant fireworks over a city skyline** — RPG epilogues, festival
  scenes. Spawn = launch streak; event = burst; event-mod = ember
  shower; end = fade. Hue palette per scene; cadence is the rhythm
  knob. Reads loud at high cadence — needs a cadence ceiling to stay
  unobtrusive.
- **Paper-lantern release** — Tangled-style, Stardew Spirit's Eve vibe.
  Lanterns drift up from the bottom in occasional clusters and fade at
  altitude. Spawn = lantern emit; lever = wind drift; event = release
  pulse; end = fade. Naturally calmer than fireworks at the same
  cadence.
- **Pumpkin patch with flickering jack-o-lanterns** — Stardew Spirit's
  Eve, seasonal events. Pumpkins as static silhouettes with flame-flicker
  inside the eyes. Lever = flicker rate; event = sync flicker on
  multiple pumpkins; rare event = a candle goes out. Reads strongly
  themed; might work better as a seasonal scene of campfire (#20).

### Magic / mythic

- **Magic portal pulse / runic gate** — Hades, Hollow Knight. A static
  portal silhouette that pulses, with rare embers spawning from the
  ring. Spawn = ember emit on pulse; lever = pulse phase; event = power
  surge (brighter pulse); event-mod = surrounding particles caught in
  draft; end = ember fade. Scene knobs = hue + pulse rate.
- **Wandering spirit drifting through a graveyard** — Spiritfarer, Hades
  underworld. A dim glow drifting along a contour with occasional pause
  + bob. Probably a fireflies (#29) variant with a slower lever and a
  pathing constraint, not its own effect.
- **Will-o'-the-wisps in swamp** — many fantasy games. Bigger / slower
  than fireflies; drifts along an invisible terrain contour. Same call
  as the spirit candidate above — likely a #29 scene variant.

### Wild / fauna

- **Slimes hopping in a meadow** — Dragon Quest, Stardew, JRPG basics.
  Two or three slime silhouettes idle on grass; each fires occasional
  small hops. Spawn = slime placement at scene start; lever = idle
  squish; event = hop; event-mod = landing dust; end = idle resume. The
  slime palette per scene is the scene knob.
- **Pond with circling ducks** — many cozy games. Ducks tracing slow
  elliptical paths, occasional dive-and-resurface event with ripple.
  Trivial 5-slot fit; reads peaceful; pairs well with lily-pad pond.
- **Bog with rising methane bubbles** — Don't Starve, Loop Hero. Slow
  large bubbles surface and pop with a small ripple. Spawn = bubble
  emit; lever = rise; event = surface pop; event-mod = ripple; end =
  ripple decay. Sister to the underwater (#32) bubble system but at the
  surface.

### Sky / horizon

- **Distant storm at sea horizon** — many adventure games' establishing
  shots. Silhouette ocean line at bottom, occasional silent lightning
  flashes behind a cloud bank. Spawn = nothing; lever = cloud drift;
  event = strike; event-mod = afterglow tail; end = decay. Calm vs.
  storm-cell scenes give natural variation.
- **Hot air balloon drifting** — Stardew Valley fair, JRPG world maps.
  A single balloon drifts horizontally across the scene with mild bob.
  Mostly metronomic — its randomness slot is the rare burner-flame puff
  that briefly raises altitude. Reads cleaner as an event-spawn inside
  a sky scene than its own effect.
- **Comet trails / meteor shower** — countless. Same starfield bones as
  #19 with an event slot for streak spawns. Likely a #19 scene variant
  rather than its own effect; promotion-time call.
- **Sailing ship on ocean horizon** — Wind Waker, Sea of Stars. Distant
  ship silhouette traverses the scene with mild sway, occasional
  lantern flicker. Pairs with the storm-at-horizon candidate; could be
  a scene variant of beach (#26).

### Cave / underground

- **Crystals slowly forming on a cave floor** — Terraria, Hollow Knight
  hidden rooms. Crystals grow over the scene's lifetime, then reset at
  scene rotation. Spawn = crystal nucleus; lever = growth rate; event =
  large crystal pop-in; event-mod = sparkle burst; end = scene rotation
  resets the field. Long-arc growth is unusual for the registry —
  worth verifying it composes with scene drift.
- **Dripping cave** — many platformers. Stalactites at the top, slow
  drips fall to a puddle that ripples on impact. Spawn = drip emit at
  stalactite tip; lever = fall; event = puddle splash; event-mod =
  ripple; end = ripple decay. Quieter sister to waterfall (#21).
- **Underground mine cart crossing** — Donkey Kong Country, Stardew
  mines. Rare event: a mine cart traverses a track silhouette across
  the bottom edge. Mostly an event-driven effect; probably best as a
  scene variant of train (#25) sharing the track-traversal mechanic.

### Quirky / one-knob

- **Bioluminescent algae waves** — Loop Hero, Subnautica beach scenes.
  Wave crests trigger glow trails that fade. Beach (#26) scene variant
  if the tide effect lands first.
- **Power line with birds landing/leaving** — peaceful urban scene
  staple. Spawn = bird arrival; lever = idle bob on wire; event =
  group takeoff; event-mod = wing-beat scatter; end = empty wire.
  Distinctive silhouette; probably its own effect.
- **Wooden bridge with crossing silhouettes** — RPG world maps. Rare
  small figure walks across a bridge silhouette in the foreground.
  Good event-driven layer; probably better as a #25 train-style
  effect generalized to "horizontal traversal" than a separate one.

## Patterns observed while triaging

A few shapes show up repeatedly and are worth weighing before promoting
the next batch:

- **Many strong candidates are scene variants of effects in flight.**
  Promoting them as their own issues fragments the registry. Better to
  land the parent effect first, then expand its scene generator. This
  doc tries to flag those calls in the entry itself.
- **"Horizontal traversal" deserves a generalized effect.** Train (#25),
  rowboat (#22), sailing ship, mine cart, and crossing silhouettes are
  all the same shape: a sprite slowly traverses the scene against a
  static backdrop. Worth considering whether #25 generalizes to host
  them all rather than each becoming its own effect.
- **"Rare event over a static scene" candidates fight the unobtrusive
  bar.** Eclipse, VHS static, snow-globe shake, fireworks at high
  cadence. Pleasing in short bursts, intrusive when ran as a default
  scene. Likely better as transition or perturbation effects per #8
  than as standalone atmospheres.
- **Fixed-silhouette candidates rely on palette + tempo for variation.**
  Sleeping dragon, clock tower, owl on a branch. The constraint isn't
  fatal — fireflies and rain also lean heavily on palette — but two
  same-effect scenes will feel less distinct than for a free-spawning
  effect. Worth weighing before promotion.
