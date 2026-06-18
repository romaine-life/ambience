# Effect candidates

The canonical, browsable registry of ambience effects: what's **built**, what's
**queued as issues**, and what's still a **candidate**. The live issues
themselves live in Glimmung (`project=ambience`, label `ambient-effects`); this
file is the human-readable overview and promotion record.

> The original rolling list (issue #9 in the retired `nelsong6/ambience`) did not
> survive the move to `romaine-life/ambience`, so promotion status lives here now.

Build references when promoting one of these: [docs/effects-cookbook.md](effects-cookbook.md)
(file pattern + touchpoint checklist) and [docs/dev-endpoints.md](dev-endpoints.md)
(dev triggers / observe endpoints used for verification).

## Selection criteria

The bar for a standalone effect:

- **Small scene** — fits the canvas at low resolution.
- **Natural randomness** — maps to the 5-slot template (spawn / lever / event /
  event-mod / end). Without an event slot a candidate reads as a one-shot
  animation, not an atmosphere.
- **Unobtrusive** — reads as background; doesn't demand attention.
- **Pixel-art friendly** — silhouettes and motion read at low res.
- **At least one scene-variation knob** — intensity, hue, density, speed, rhythm.

The 5-slot template *is* the schema's `KnobSlot` taxonomy (`sim/schema.go`): each
effect declares typed `Knob`s tagged spawn / lever / event / event-mod / end,
implements the `effectRuntime` interface, self-registers via `init()` →
`register()` in `cmd/ambience/effect_<name>.go` (decentralized — no central list
to merge-conflict on), exposes its WASM constructor through `cmd/ambience-wasm`,
and — if it has `intro`/`ending` triggers — surfaces a derived `lifecycle` field
per `sim/lifecycle.go`. Browser effects render through the Go/WASM runtime into a
pixel grid; there is no `cmd/ambience/web/effects/*`.

## Built — live on `main`

33 effects are registered (`sim/<name>.go` + `cmd/ambience/effect_<name>.go`):

aurora · autumn leaves · beach · birds on a wire · bog · burning trees ·
campfire · cave crystals · cottage chimney · distant storm · dust · fireflies ·
lava lamp · lighthouse · magic portal · mysterious man · paper lanterns · pond ·
rain · rain on window · rowboat · sand · slimes · snow · starfield · tetris ·
train · underwater · volcano · water pipe · waterfall · wheat field · windmill

(Some — tetris, dust, sand, water pipe, burning trees — may be utility / playful /
scene-element effects rather than classic atmospheres; registered all the same.)

## Queued — next batch (`ambience#177–204`)

28 effect issues are filed and labeled `ambient-effects`, written to the effect
contract (concept, goals, recommended v1, 5-slot/knob mapping, events, lifecycle).
None dispatched yet.

**Cozy / domestic** — sleeping cat #183 *(or a campfire-#20 layer)* · tea kettle
#186 · wind chimes #187 · rooftop laundry #197
**Fauna / fish** — owl #179 *(prey/swoop hunt loop)* · deer grazing #193 · bats at
dusk #192 · aquarium / fish tank #188 · jellyfish #189
**Sky / cosmic** — drifting fog #185 · moonrise #202 · constellations #204 ·
distant fireworks #180 *(transition track)*
**Earth / water** — dripping cave #181 · hot spring #190 · geyser #191
**Magic / mythic** — sleeping dragon #177 · crystal ball #200 · mushroom grove #201
**Urban / objects** — clock tower #178 · streetlight moths #194 · neon sign #195 ·
ferris wheel #196 · blacksmith forge #198 · spider web #199 · pumpkin patch #182
*(or a seasonal campfire-#20 scene)* · VHS static #184 *(transition track)* ·
snow globe #203 *(transition track)*

## Deferred — build as scene variants, not standalone effects

These fragment the registry if promoted; land them as a scene/preset on the
parent effect, not a new effect issue:

| Candidate | Fold into |
| --- | --- |
| Wandering spirit in a graveyard | fireflies (slower lever + pathing) |
| Will-o'-the-wisps in a swamp | fireflies |
| Comet trails / meteor shower | starfield (event-slot streaks) |
| Sailing ship on the horizon | beach / a generalized horizontal-traversal effect |
| Bioluminescent algae waves | beach |
| Underground mine cart | train (horizontal traversal) |
| Wooden bridge with crossing silhouettes | train (horizontal traversal) |
| Hot air balloon drifting | a sky scene's event-spawn |

## Transition / perturbation track

`VHS static` (#184), `distant fireworks` (#180, at high cadence), and `snow globe`
(#203) are "rare bright/odd event over a static scene" — fine in short bursts,
intrusive as a default atmosphere. Their issues are filed, but they're best wired
as transition / perturbation effects, or kept to a low cadence as default scenes.
Eclipse falls in the same bucket and is intentionally not promoted as a standalone.

## Patterns observed while triaging

- **Many strong candidates are scene variants of effects in flight.** Promote the
  parent first, then expand its scene generator. The deferred table flags these.
- **"Horizontal traversal" deserves a generalized effect.** train (#25), rowboat,
  sailing ship, mine cart, and crossing silhouettes are one shape — a sprite
  traversing a static backdrop. Consider generalizing `train` to host them rather
  than one effect each.
- **"Rare event over a static scene" fights the unobtrusive bar.** See the
  transition track above.
- **Fixed-silhouette candidates rely on palette + tempo for variation.** sleeping
  dragon, clock tower, owl. Not fatal (fireflies/rain lean on palette too), but two
  same-effect scenes feel less distinct — give them a real event beat (owl #179's
  prey/swoop loop is the model).
