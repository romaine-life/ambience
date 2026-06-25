//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/romaine-life/ambience/sim"
)

type runtime struct {
	kind    string
	effect  wasmEffect
	set     func(json.RawMessage) error
	restore func(effectEnvelope) error
}

type wasmEffect interface {
	Step()
	CurrentTick() int
	TriggerEvent(string) bool
	GridCopy() [][]sim.Pixel
}

type resizable interface {
	Resize(int, int)
}

type effectEnvelope struct {
	Tick   int             `json:"tick"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	Seed   int64           `json:"seed"`
	GridW  int             `json:"gridW"`
	GridH  int             `json:"gridH"`
}

var (
	nextID   = 1
	runtimes = map[int]*runtime{}
)

func main() {
	js.Global().Set("ambienceWasm", map[string]any{
		"supportedEffects": js.FuncOf(supportedEffects),
		"newRuntime":       js.FuncOf(newRuntime),
		"destroy":          js.FuncOf(destroy),
		"setConfig":        js.FuncOf(setConfig),
		"restoreSnapshot":  js.FuncOf(restoreSnapshot),
		"triggerEvent":     js.FuncOf(triggerEvent),
		"step":             js.FuncOf(step),
		"tick":             js.FuncOf(tick),
		"width":            js.FuncOf(width),
		"height":           js.FuncOf(height),
		"frame":            js.FuncOf(frame),
		"overlayFrame":     js.FuncOf(overlayFrame),
	})
	select {}
}

func newRuntime(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("usage: newRuntime(kind, w, h, seed?, configJSON?)")
	}
	kind := args[0].String()
	w := args[1].Int()
	h := args[2].Int()
	seed := int64(1)
	if len(args) > 3 {
		seed = jsInt64(args[3])
		if seed == 0 {
			seed = 1
		}
	}
	configJSON := json.RawMessage("{}")
	if len(args) > 4 && args[4].Type() == js.TypeString {
		configJSON = json.RawMessage(args[4].String())
	}
	rt, err := makeRuntime(kind, w, h, seed, configJSON)
	if err != nil {
		return fail(err.Error())
	}
	id := nextID
	nextID++
	runtimes[id] = rt
	return id
}

type typedEffect[C any, S any] interface {
	wasmEffect
	resizable
	SetConfig(C)
	RestoreSnapshot(S)
}

func makeTypedRuntime[C any, S any, T typedEffect[C, S]](kind string, w, h int, seed int64, cfgRaw json.RawMessage, ctor func(int, int, int64, C) T) (*runtime, error) {
	var cfg C
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return nil, fmt.Errorf("decode %s config: %w", kind, err)
		}
	}
	effect := ctor(w, h, seed, cfg)
	rt := &runtime{
		kind:   kind,
		effect: effect,
	}
	rt.set = func(raw json.RawMessage) error {
		var cfg C
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return fmt.Errorf("decode %s config: %w", kind, err)
			}
		}
		effect.SetConfig(cfg)
		return nil
	}
	rt.restore = func(env effectEnvelope) error {
		if len(env.Config) > 0 {
			if err := rt.set(env.Config); err != nil {
				return err
			}
		}
		if env.GridW > 0 && env.GridH > 0 {
			effect.Resize(env.GridW, env.GridH)
		}
		if len(env.State) == 0 {
			return nil
		}
		var snap S
		if err := json.Unmarshal(env.State, &snap); err != nil {
			return fmt.Errorf("decode %s snapshot: %w", kind, err)
		}
		effect.RestoreSnapshot(snap)
		return nil
	}
	return rt, nil
}

func destroy(_ js.Value, args []js.Value) any {
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		delete(runtimes, args[0].Int())
	}
	return true
}

func setConfig(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return false
	}
	if len(args) < 2 || args[1].Type() != js.TypeString {
		return fail("setConfig requires config JSON")
	}
	if err := rt.set(json.RawMessage(args[1].String())); err != nil {
		return fail(err.Error())
	}
	return true
}

func restoreSnapshot(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return false
	}
	if len(args) < 2 || args[1].Type() != js.TypeString {
		return fail("restoreSnapshot requires snapshot JSON")
	}
	var env effectEnvelope
	if err := json.Unmarshal([]byte(args[1].String()), &env); err != nil {
		return fail(fmt.Sprintf("decode snapshot envelope: %v", err))
	}
	if err := rt.restore(env); err != nil {
		return fail(err.Error())
	}
	return true
}

func triggerEvent(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil || len(args) < 2 {
		return false
	}
	return rt.effect.TriggerEvent(args[1].String())
}

func step(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return false
	}
	steps := 1
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		steps = max(1, args[1].Int())
	}
	for range steps {
		rt.effect.Step()
	}
	return true
}

func tick(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	return rt.effect.CurrentTick()
}

func width(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	_, w := gridBounds(rt.effect.GridCopy())
	return w
}

func height(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	h, _ := gridBounds(rt.effect.GridCopy())
	return h
}

func frame(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return js.Global().Get("Uint8ClampedArray").New(0)
	}
	buf := flattenGrid(rt.effect.GridCopy())
	out := js.Global().Get("Uint8ClampedArray").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

// overlayEffect is the optional capability of exposing a near/overlay frame
// (only rain implements it today). Effects without it return an empty frame, so
// a consumer asking for the overlay layer of a non-overlay effect draws nothing.
type overlayEffect interface {
	OverlayGridCopy() [][]sim.Pixel
}

func overlayFrame(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return js.Global().Get("Uint8ClampedArray").New(0)
	}
	oe, ok := rt.effect.(overlayEffect)
	if !ok {
		return js.Global().Get("Uint8ClampedArray").New(0)
	}
	buf := flattenGrid(oe.OverlayGridCopy())
	out := js.Global().Get("Uint8ClampedArray").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func flattenGrid(grid [][]sim.Pixel) []byte {
	h, w := gridBounds(grid)
	buf := make([]byte, h*w*3)
	for y, row := range grid {
		for x, p := range row {
			if !p.Filled {
				continue
			}
			i := (y*w + x) * 3
			buf[i] = p.C.R
			buf[i+1] = p.C.G
			buf[i+2] = p.C.B
		}
	}
	return buf
}

func gridBounds(grid [][]sim.Pixel) (int, int) {
	w := 0
	for _, row := range grid {
		if len(row) > w {
			w = len(row)
		}
	}
	return len(grid), w
}

func lookup(args []js.Value) *runtime {
	if len(args) == 0 || args[0].Type() != js.TypeNumber {
		return nil
	}
	return runtimes[args[0].Int()]
}

func jsInt64(v js.Value) int64 {
	switch v.Type() {
	case js.TypeString:
		var out int64
		if err := json.Unmarshal([]byte(v.String()), &out); err == nil {
			return out
		}
		return 0
	case js.TypeNumber:
		return int64(v.Float())
	default:
		return 0
	}
}

func fail(message string) any {
	js.Global().Get("console").Call("error", "ambience wasm:", message)
	return -1
}
