//go:build js && wasm && rainonly

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/romaine-life/ambience/sim"
)

// Rain-scoped effect registry, selected with `-tags rainonly`. This builds the
// client artifact (ambience-rain.wasm) that single-effect consumers vendor —
// e.g. the chess main menu, which subscribes to the rain-pinned /chess world.
// Because this is the only makeRuntime in the build, the Go linker
// dead-code-eliminates every sim.New* except those referenced here, so the
// artifact is a fraction of the full all-effects bundle.
//
// A client built this way supports only these effects. The world's snapshot
// servedEffects + the consumer handshake guarantee it is only ever paired with
// a world it can render; a mismatch fails loudly rather than rendering nothing.

func supportedEffects(js.Value, []js.Value) any {
	return js.ValueOf([]any{
		"rain",
		"rain-on-window",
	})
}

func makeRuntime(kind string, w, h int, seed int64, cfg json.RawMessage) (*runtime, error) {
	switch kind {
	case "rain":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewRain)
	case "rain-on-window":
		return makeTypedRuntime(kind, w, h, seed, cfg, sim.NewRainOnWindow)
	default:
		return nil, fmt.Errorf("unsupported effect %q (rain-only build)", kind)
	}
}
