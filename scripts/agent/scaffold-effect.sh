#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  echo "usage: scripts/agent/scaffold-effect.sh <effect-slug>" >&2
  echo "example: scripts/agent/scaffold-effect.sh paper-lanterns" >&2
}

if [ "$#" -ne 1 ]; then
  usage
  exit 2
fi

slug="$1"
if ! [[ "$slug" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
  echo "effect slug must be lowercase kebab-case: ${slug}" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REPO_DIR="${AMBIENCE_REPO_DIR:-$DEFAULT_REPO_DIR}"

snake="${slug//-/_}"
pascal="$(
  awk -v s="$slug" '
    BEGIN {
      n = split(s, parts, "-")
      for (i = 1; i <= n; i++) {
        out = out toupper(substr(parts[i], 1, 1)) substr(parts[i], 2)
      }
      print out
    }
  '
)"
lower_ident="$(printf '%s%s' "$(printf '%s' "${pascal:0:1}" | tr '[:upper:]' '[:lower:]')" "${pascal:1}")"
title="$(
  awk -v s="$slug" '
    BEGIN {
      n = split(s, parts, "-")
      for (i = 1; i <= n; i++) {
        if (i > 1) out = out " "
        out = out toupper(substr(parts[i], 1, 1)) substr(parts[i], 2)
      }
      print out
    }
  '
)"

sim_file="${REPO_DIR}/sim/${snake}.go"
test_file="${REPO_DIR}/sim/${snake}_test.go"
runtime_file="${REPO_DIR}/cmd/ambience/effect_${snake}.go"

mkdir -p "$(dirname "$sim_file")" "$(dirname "$runtime_file")"

for target in "$sim_file" "$test_file" "$runtime_file"; do
  if [ -e "$target" ]; then
    echo "refusing to overwrite existing file: ${target}" >&2
    exit 1
  fi
done

render_template() {
  local target="$1"
  sed \
    -e "s/__SLUG__/${slug}/g" \
    -e "s/__SNAKE__/${snake}/g" \
    -e "s/__PASCAL__/${pascal}/g" \
    -e "s/__LOWER_IDENT__/${lower_ident}/g" \
    -e "s/__TITLE__/${title}/g" \
    >"$target"
}

render_template "$sim_file" <<'GO'
package sim

import (
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/romaine-life/ambience/rngutil"
)

// __PASCAL__Config tunes the __SLUG__ effect. Replace these starter knobs
// with the issue-owned aesthetic controls before publishing the effect.
type __PASCAL__Config struct {
	IntroDur  int     `json:"intro_dur"`
	EndingDur int     `json:"ending_dur"`
	PulseDur  int     `json:"pulse_dur"`
	Hue       float64 `json:"hue"`
	Sat       float64 `json:"sat"`
	Light     float64 `json:"light"`
}

func (c __PASCAL__Config) withDefaults() __PASCAL__Config {
	if c.IntroDur <= 0 {
		c.IntroDur = 80
	}
	if c.EndingDur <= 0 {
		c.EndingDur = 80
	}
	if c.PulseDur <= 0 {
		c.PulseDur = 36
	}
	if c.Hue == 0 {
		c.Hue = 42
	}
	if c.Sat <= 0 {
		c.Sat = 0.72
	}
	if c.Light <= 0 {
		c.Light = 0.58
	}
	return c
}

// __PASCAL__Snapshot is the server/client wire state for __SLUG__.
type __PASCAL__Snapshot struct {
	Tick        int       `json:"tick"`
	Lifecycle  Lifecycle `json:"lifecycle"`
	IntroTicks int       `json:"introTicks,omitempty"`
	EndingTicks int      `json:"endingTicks,omitempty"`
	PulseTicks int       `json:"pulseTicks,omitempty"`
	Ended      bool      `json:"ended,omitempty"`
	RNGState   uint64    `json:"rngState,omitempty"`
}

// __PASCAL__PersistedState is the restart-safe state for __SLUG__.
type __PASCAL__PersistedState struct {
	Tick        int    `json:"tick"`
	IntroTicks int    `json:"introTicks,omitempty"`
	EndingTicks int   `json:"endingTicks,omitempty"`
	PulseTicks int    `json:"pulseTicks,omitempty"`
	Ended      bool   `json:"ended,omitempty"`
	RNGState   uint64 `json:"rngState"`
}

// __PASCAL__ is a starter pixel-grid simulation for __SLUG__.
type __PASCAL__ struct {
	mu sync.Mutex

	W, H int
	Grid [][]Pixel

	rng *rngutil.RNG
	cfg __PASCAL__Config

	tick        int
	introTicks  int
	endingTicks int
	pulseTicks  int
	ended       bool
	log         []LogEntry
}

func New__PASCAL__(w, h int, seed int64, cfg __PASCAL__Config) *__PASCAL__ {
	e := &__PASCAL__{
		rng: rngutil.New(seed),
		cfg: cfg.withDefaults(),
	}
	e.Resize(w, h)
	return e
}

// __PASCAL__Schema describes the __SLUG__ effect's tunable knobs.
func __PASCAL__Schema() EffectSchema {
	return EffectSchema{
		Name:           "__SLUG__",
		EndingTerminal: true,
		Knobs: []Knob{
			{Key: "intro_dur", Label: "intro dur", Slot: SlotSpawn, Group: "lifecycle", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80, Trigger: "intro",
				Description: "Ticks spent ramping from dark into the resting frame."},
			{Key: "ending_dur", Label: "ending dur", Slot: SlotEnd, Group: "lifecycle", Type: KnobInt, Min: 20, Max: 240, Step: 5, Default: 80, Trigger: "ending",
				Description: "Ticks spent resolving into the terminal frame."},
			{Key: "pulse_dur", Label: "pulse dur", Slot: SlotEventMod, Group: "pulse", Type: KnobInt, Min: 6, Max: 120, Step: 2, Default: 36,
				Description: "Duration of the starter transient pulse event."},
			{Key: "hue", Label: "hue", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 360, Step: 1, Default: 42,
				Description: "Base hue for the starter frame."},
			{Key: "sat", Label: "sat", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0, Max: 1, Step: 0.01, Default: 0.72,
				Description: "Starter frame color saturation."},
			{Key: "light", Label: "light", Slot: SlotLever, Group: "color", Type: KnobFloat, Min: 0.05, Max: 1, Step: 0.01, Default: 0.58, Trigger: "pulse",
				Description: "Starter frame brightness; pulse previews the transient event path."},
		},
	}
}

func (e *__PASCAL__) Resize(w, h int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e.W = w
	e.H = h
	e.Grid = make([][]Pixel, h)
	for y := range e.Grid {
		e.Grid[y] = make([]Pixel, w)
	}
	e.renderLocked()
}

func (e *__PASCAL__) EffectiveConfig() __PASCAL__Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *__PASCAL__) SetConfig(cfg __PASCAL__Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg.withDefaults()
	e.renderLocked()
}

func (e *__PASCAL__) Snapshot() __PASCAL__Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return __PASCAL__Snapshot{
		Tick:        e.tick,
		Lifecycle:  e.lifecycleLocked(),
		IntroTicks: e.introTicks,
		EndingTicks: e.endingTicks,
		PulseTicks: e.pulseTicks,
		Ended:      e.ended,
		RNGState:   e.rng.State(),
	}
}

func (e *__PASCAL__) RestoreSnapshot(snap __PASCAL__Snapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick = snap.Tick
	e.introTicks = snap.IntroTicks
	e.endingTicks = snap.EndingTicks
	e.pulseTicks = snap.PulseTicks
	e.ended = snap.Ended || snap.Lifecycle == LifecycleEnded
	if snap.RNGState != 0 {
		e.rng.SetState(snap.RNGState)
	}
	e.renderLocked()
}

func (e *__PASCAL__) SnapshotPersistedState() __PASCAL__PersistedState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return __PASCAL__PersistedState{
		Tick:        e.tick,
		IntroTicks: e.introTicks,
		EndingTicks: e.endingTicks,
		PulseTicks: e.pulseTicks,
		Ended:      e.ended,
		RNGState:   e.rng.State(),
	}
}

func (e *__PASCAL__) RestorePersistedState(state __PASCAL__PersistedState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick = state.Tick
	e.introTicks = state.IntroTicks
	e.endingTicks = state.EndingTicks
	e.pulseTicks = state.PulseTicks
	e.ended = state.Ended
	if state.RNGState != 0 {
		e.rng.SetState(state.RNGState)
	}
	e.renderLocked()
}

func (e *__PASCAL__) CurrentTick() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick
}

func (e *__PASCAL__) PerturbRNG(delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rng.Mix(delta)
}

func (e *__PASCAL__) DrainLog() []LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.log) == 0 {
		return nil
	}
	out := e.log
	e.log = nil
	return out
}

func (e *__PASCAL__) TriggerEvent(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch name {
	case "intro":
		e.ended = false
		e.endingTicks = 0
		e.introTicks = e.cfg.IntroDur
		e.appendLogLocked("intro", fmt.Sprintf("started (dur=%d)", e.introTicks))
	case "ending":
		e.introTicks = 0
		e.pulseTicks = 0
		e.endingTicks = e.cfg.EndingDur
		e.appendLogLocked("ending", fmt.Sprintf("started (dur=%d)", e.endingTicks))
	case "pulse":
		if e.ended {
			return false
		}
		e.pulseTicks = e.cfg.PulseDur
		e.appendLogLocked("pulse", fmt.Sprintf("started (dur=%d)", e.pulseTicks))
	default:
		return false
	}
	e.renderLocked()
	return true
}

func (e *__PASCAL__) Step() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tick++
	if e.introTicks > 0 {
		e.introTicks--
	}
	if e.pulseTicks > 0 {
		e.pulseTicks--
	}
	if e.endingTicks > 0 {
		e.endingTicks--
		if e.endingTicks == 0 {
			e.ended = true
		}
	}
	e.renderLocked()
}

func (e *__PASCAL__) GridCopy() [][]Pixel {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]Pixel, len(e.Grid))
	for y := range e.Grid {
		out[y] = make([]Pixel, len(e.Grid[y]))
		copy(out[y], e.Grid[y])
	}
	return out
}

func (e *__PASCAL__) lifecycleLocked() Lifecycle {
	switch {
	case e.introTicks > 0:
		return LifecycleIntro
	case e.endingTicks > 0:
		return LifecycleEnding
	case e.ended:
		return LifecycleEnded
	default:
		return LifecycleRunning
	}
}

func (e *__PASCAL__) appendLogLocked(kind, desc string) {
	e.log = append(e.log, LogEntry{Tick: e.tick, Type: kind, Desc: desc})
	if len(e.log) > 200 {
		e.log = e.log[len(e.log)-200:]
	}
}

func (e *__PASCAL__) renderLocked() {
	for y := range e.Grid {
		for x := range e.Grid[y] {
			e.Grid[y][x] = Pixel{}
		}
	}
	if e.ended {
		return
	}

	level := 1.0
	if e.introTicks > 0 {
		level = 1 - float64(e.introTicks)/float64(max(1, e.cfg.IntroDur))
	}
	if e.endingTicks > 0 {
		level = float64(e.endingTicks) / float64(max(1, e.cfg.EndingDur))
	}
	if e.pulseTicks > 0 {
		level = math.Min(1.35, level+0.35*float64(e.pulseTicks)/float64(max(1, e.cfg.PulseDur)))
	}

	cx := float64(e.W-1) / 2
	cy := float64(e.H-1) / 2
	radius := math.Max(2, math.Min(float64(e.W), float64(e.H))*0.28)
	for y := 0; y < e.H; y++ {
		for x := 0; x < e.W; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Hypot(dx, dy)
			if dist > radius {
				continue
			}
			wave := 0.75 + 0.25*math.Sin(float64(e.tick)*0.08+dist*0.55)
			light := clamp01(e.cfg.Light * level * wave)
			e.Grid[y][x] = Pixel{
				Filled: true,
				C:     hslToRGB(e.cfg.Hue+dist*2, clamp01(e.cfg.Sat), light),
			}
		}
	}
}

func count__PASCAL__Filled(grid [][]Pixel) int {
	count := 0
	for _, row := range grid {
		for _, p := range row {
			if p.Filled && p.C != (color.RGBA{}) {
				count++
			}
		}
	}
	return count
}
GO

render_template "$test_file" <<'GO'
package sim

import (
	"reflect"
	"testing"
)

func Test__PASCAL__SchemaContainsLifecycleTriggers(t *testing.T) {
	schema := __PASCAL__Schema()
	if schema.Name != "__SLUG__" {
		t.Fatalf("schema name = %q, want __SLUG__", schema.Name)
	}
	if !schema.EndingTerminal {
		t.Fatal("__SLUG__ ending should hold a terminal state")
	}
	want := map[string]bool{"intro": true, "ending": true, "pulse": true}
	for _, knob := range schema.Knobs {
		delete(want, knob.Trigger)
	}
	if len(want) != 0 {
		t.Fatalf("schema missing triggers: %v", want)
	}
}

func Test__PASCAL__DefaultFrameHasContent(t *testing.T) {
	e := New__PASCAL__(48, 32, 1, __PASCAL__Config{})
	if filled := count__PASCAL__Filled(e.GridCopy()); filled == 0 {
		t.Fatal("default frame is empty")
	}
}

func Test__PASCAL__SnapshotRoundTrip(t *testing.T) {
	a := New__PASCAL__(48, 32, 99, __PASCAL__Config{PulseDur: 12})
	if !a.TriggerEvent("pulse") {
		t.Fatal("pulse trigger rejected")
	}
	for i := 0; i < 5; i++ {
		a.Step()
	}

	b := New__PASCAL__(48, 32, 1, __PASCAL__Config{PulseDur: 12})
	b.RestoreSnapshot(a.Snapshot())
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("snapshot mismatch immediately after restore")
	}
	if !reflect.DeepEqual(a.GridCopy(), b.GridCopy()) {
		t.Fatal("grid mismatch immediately after restore")
	}
}

func Test__PASCAL__EndingHoldsTerminalState(t *testing.T) {
	e := New__PASCAL__(48, 32, 44, __PASCAL__Config{EndingDur: 6})
	if !e.TriggerEvent("ending") {
		t.Fatal("ending trigger rejected")
	}
	for i := 0; i < 12; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("lifecycle = %q, want ended", got)
	}
	if filled := count__PASCAL__Filled(e.GridCopy()); filled != 0 {
		t.Fatalf("terminal frame has %d filled pixels, want 0", filled)
	}
	for i := 0; i < 8; i++ {
		e.Step()
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleEnded {
		t.Fatalf("terminal lifecycle did not hold, got %q", got)
	}
	if !e.TriggerEvent("intro") {
		t.Fatal("intro trigger rejected from terminal state")
	}
	if got := e.Snapshot().Lifecycle; got != LifecycleIntro {
		t.Fatalf("intro restart lifecycle = %q, want intro", got)
	}
}
GO

render_template "$runtime_file" <<'GO'
package main

import (
	"encoding/json"
	"fmt"

	"github.com/romaine-life/ambience/sim"
)

func init() {
	register(effectDefinition{
		Type:       "__SLUG__",
		Schema:     sim.__PASCAL__Schema,
		NewRuntime: new__PASCAL__Runtime,
	})
}

type __LOWER_IDENT__Runtime struct {
	sim *sim.__PASCAL__
}

func new__PASCAL__Runtime(w, h int, seed int64, cfg json.RawMessage) (effectRuntime, error) {
	var parsed sim.__PASCAL__Config
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return nil, fmt.Errorf("decode __SLUG__ config: %w", err)
		}
	}
	return &__LOWER_IDENT__Runtime{sim: sim.New__PASCAL__(w, h, seed, parsed)}, nil
}

func (r *__LOWER_IDENT__Runtime) Type() string { return "__SLUG__" }

func (r *__LOWER_IDENT__Runtime) Schema() sim.EffectSchema { return sim.__PASCAL__Schema() }

func (r *__LOWER_IDENT__Runtime) Snapshot() (effectEnvelope, error) {
	configData, err := json.Marshal(r.sim.EffectiveConfig())
	if err != nil {
		return effectEnvelope{}, err
	}
	snap := r.sim.Snapshot()
	stateData, err := json.Marshal(snap)
	if err != nil {
		return effectEnvelope{}, err
	}
	return effectEnvelope{
		Tick:   snap.Tick,
		Config: configData,
		State:  stateData,
		GridW:  r.sim.W,
		GridH:  r.sim.H,
	}, nil
}

func (r *__LOWER_IDENT__Runtime) Restore(s effectEnvelope) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.__PASCAL__Snapshot
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode __SLUG__ snapshot: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestoreSnapshot(state)
	return nil
}

func (r *__LOWER_IDENT__Runtime) Persisted() (persistedEffectState, error) {
	configData, err := json.Marshal(r.sim.EffectiveConfig())
	if err != nil {
		return persistedEffectState{}, err
	}
	stateData, err := json.Marshal(r.sim.SnapshotPersistedState())
	if err != nil {
		return persistedEffectState{}, err
	}
	return persistedEffectState{
		Config: configData,
		State:  stateData,
		GridW:  r.sim.W,
		GridH:  r.sim.H,
	}, nil
}

func (r *__LOWER_IDENT__Runtime) RestorePersisted(s persistedEffectState) error {
	if len(s.Config) > 0 {
		if err := r.ApplyConfig(s.Config); err != nil {
			return err
		}
	}
	if len(s.State) == 0 {
		return nil
	}
	var state sim.__PASCAL__PersistedState
	if err := json.Unmarshal(s.State, &state); err != nil {
		return fmt.Errorf("decode __SLUG__ persisted state: %w", err)
	}
	if s.GridW > 0 && s.GridH > 0 && (r.sim.W != s.GridW || r.sim.H != s.GridH) {
		r.sim.Resize(s.GridW, s.GridH)
	}
	r.sim.RestorePersistedState(state)
	return nil
}

func (r *__LOWER_IDENT__Runtime) Trigger(name string) bool { return r.sim.TriggerEvent(name) }

func (r *__LOWER_IDENT__Runtime) Frame() [][]sim.Pixel { return r.sim.GridCopy() }

func (r *__LOWER_IDENT__Runtime) Step() { r.sim.Step() }

func (r *__LOWER_IDENT__Runtime) CurrentTick() int { return r.sim.CurrentTick() }

func (r *__LOWER_IDENT__Runtime) DrainLog() []sim.LogEntry { return r.sim.DrainLog() }

func (r *__LOWER_IDENT__Runtime) ApplyConfig(data json.RawMessage) error {
	var cfg sim.__PASCAL__Config
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode __SLUG__ config: %w", err)
		}
	}
	r.sim.SetConfig(cfg)
	return nil
}

func (r *__LOWER_IDENT__Runtime) AddEntropy(delta int64) { r.sim.PerturbRNG(delta) }
GO

gofmt -w "$sim_file" "$test_file" "$runtime_file"

printf 'scaffolded %s:\n' "$slug"
printf '  %s\n' "${sim_file#${REPO_DIR}/}"
printf '  %s\n' "${test_file#${REPO_DIR}/}"
printf '  %s\n' "${runtime_file#${REPO_DIR}/}"
printf '\nrequired next edit:\n'
printf '  cmd/ambience-wasm/main.go: add "%s" to supportedEffects and add:\n' "$slug"
printf '  case "%s": return makeTypedRuntime(kind, w, h, seed, cfg, sim.New%s)\n' "$slug" "$pascal"
