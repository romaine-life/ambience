package sim

// Lifecycle is the closed, effect-generic description of where an effect is
// in its intro/steady/outro arc. It exists so external observers — the
// verification harness above all — can assert lifecycle claims against a
// CONTRACT instead of guessing effect-internal counter names and semantics.
// ambience#167 run 8.1 is the motivating failure: a test plan asserted
// `introTicks == 0` (the countdown idiom other effects use) against an
// implementation whose introTicks counted up, and a correct implementation
// failed verification over a private variable's semantics. The lifecycle
// enum is the public surface; internal counters stay private and free to
// change.
//
// Semantics:
//
//   - LifecycleIntro:   an intro sequence is in progress.
//   - LifecycleRunning: steady-state. Effects with no intro/outro report
//     this always.
//   - LifecycleEnding:  an outro sequence is in progress.
//   - LifecycleEnded:   the outro completed and the effect is holding its
//     terminal look (typically until an `intro` trigger restarts it).
//
// Every effect that registers `intro`/`ending` triggers MUST surface a
// `lifecycle` field in its snapshot state, computed from its own internals
// at snapshot time (it is derived state — never restored). The registry
// test in cmd/ambience enforces the transitions for every such effect.
type Lifecycle string

const (
	LifecycleIntro   Lifecycle = "intro"
	LifecycleRunning Lifecycle = "running"
	LifecycleEnding  Lifecycle = "ending"
	LifecycleEnded   Lifecycle = "ended"
)
