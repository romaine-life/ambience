//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/nelsong6/ambience/sim"
)

type runtime struct {
	kind string
	rain *sim.Rain
}

type rainEnvelope struct {
	Tick   int             `json:"tick"`
	Config sim.Config      `json:"config"`
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
		"newRuntime":      js.FuncOf(newRuntime),
		"destroy":         js.FuncOf(destroy),
		"setConfig":       js.FuncOf(setConfig),
		"restoreSnapshot": js.FuncOf(restoreSnapshot),
		"triggerEvent":    js.FuncOf(triggerEvent),
		"step":            js.FuncOf(step),
		"tick":            js.FuncOf(tick),
		"width":           js.FuncOf(width),
		"height":          js.FuncOf(height),
		"frame":           js.FuncOf(frame),
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
		parsed, err := jsInt64(args[3])
		if err == nil && parsed != 0 {
			seed = parsed
		}
	}
	configJSON := "{}"
	if len(args) > 4 && args[4].Type() == js.TypeString {
		configJSON = args[4].String()
	}
	switch kind {
	case "rain":
		var cfg sim.Config
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return fail(fmt.Sprintf("decode rain config: %v", err))
		}
		id := nextID
		nextID++
		runtimes[id] = &runtime{kind: kind, rain: sim.NewRain(w, h, seed, cfg)}
		return id
	default:
		return fail("unsupported effect: " + kind)
	}
}

func destroy(_ js.Value, args []js.Value) any {
	if rt := lookup(args); rt != nil {
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
	switch rt.kind {
	case "rain":
		var cfg sim.Config
		if err := json.Unmarshal([]byte(args[1].String()), &cfg); err != nil {
			return fail(fmt.Sprintf("decode rain config: %v", err))
		}
		rt.rain.SetConfig(cfg)
		return true
	default:
		return false
	}
}

func restoreSnapshot(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return false
	}
	if len(args) < 2 || args[1].Type() != js.TypeString {
		return fail("restoreSnapshot requires snapshot JSON")
	}
	switch rt.kind {
	case "rain":
		var env rainEnvelope
		raw := []byte(args[1].String())
		if err := json.Unmarshal(raw, &env); err != nil {
			return fail(fmt.Sprintf("decode rain envelope: %v", err))
		}
		if env.GridW > 0 && env.GridH > 0 {
			rt.rain.Resize(env.GridW, env.GridH)
		}
		rt.rain.SetConfig(env.Config)
		stateRaw := env.State
		if len(stateRaw) == 0 {
			stateRaw = raw
		}
		var snap sim.RainSnapshot
		if err := json.Unmarshal(stateRaw, &snap); err != nil {
			return fail(fmt.Sprintf("decode rain state: %v", err))
		}
		if snap.Tick == 0 && env.Tick > 0 {
			snap.Tick = env.Tick
		}
		rt.rain.RestoreSnapshot(snap)
		return true
	default:
		return false
	}
}

func triggerEvent(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil || len(args) < 2 {
		return false
	}
	switch rt.kind {
	case "rain":
		return rt.rain.TriggerEvent(args[1].String())
	default:
		return false
	}
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
	switch rt.kind {
	case "rain":
		for range steps {
			rt.rain.Step()
		}
		return true
	default:
		return false
	}
}

func tick(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	switch rt.kind {
	case "rain":
		return rt.rain.CurrentTick()
	default:
		return 0
	}
}

func width(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	switch rt.kind {
	case "rain":
		return rt.rain.W
	default:
		return 0
	}
}

func height(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return 0
	}
	switch rt.kind {
	case "rain":
		return rt.rain.H
	default:
		return 0
	}
}

func frame(_ js.Value, args []js.Value) any {
	rt := lookup(args)
	if rt == nil {
		return js.Global().Get("Uint8ClampedArray").New(0)
	}
	var grid [][]sim.Pixel
	switch rt.kind {
	case "rain":
		grid = rt.rain.GridCopy()
	default:
		return js.Global().Get("Uint8ClampedArray").New(0)
	}
	buf := flattenGrid(grid)
	out := js.Global().Get("Uint8ClampedArray").New(len(buf))
	js.CopyBytesToJS(out, buf)
	return out
}

func flattenGrid(grid [][]sim.Pixel) []byte {
	w := 0
	for _, row := range grid {
		if len(row) > w {
			w = len(row)
		}
	}
	buf := make([]byte, len(grid)*w*3)
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

func lookup(args []js.Value) *runtime {
	if len(args) == 0 || args[0].Type() != js.TypeNumber {
		return nil
	}
	return runtimes[args[0].Int()]
}

func jsInt64(v js.Value) (int64, error) {
	switch v.Type() {
	case js.TypeString:
		return strconv.ParseInt(v.String(), 10, 64)
	case js.TypeNumber:
		return int64(v.Float()), nil
	default:
		return 0, fmt.Errorf("unsupported int64 value")
	}
}

func fail(message string) any {
	js.Global().Get("console").Call("error", "ambience wasm:", message)
	return -1
}
