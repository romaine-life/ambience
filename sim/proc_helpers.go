package sim

// ProceduralConfig is a lightweight numeric config map used by the browser-first
// scenic prototypes (snow, autumn-leaves, aurora, ...). The server owns event
// timing and snapshot/restore state; the browser owns the richer deterministic
// render derived from tick + seed. Each per-effect file aliases this shape as
// e.g. SnowConfig = ProceduralConfig.
type ProceduralConfig map[string]float64

// ProceduralState is the per-tick mood state shared across the procedural
// prototypes. Each effect tracks its own timers/values inside this shape;
// per-effect files alias this as e.g. SnowState = ProceduralState.
type ProceduralState struct {
	Tick   int                `json:"tick"`
	Timers map[string]int     `json:"timers,omitempty"`
	Values map[string]float64 `json:"values,omitempty"`
}

// ProceduralSnapshot is the wire shape returned by Snapshot() on every
// procedural-prototype effect.
type ProceduralSnapshot struct {
	ProceduralState
	RNGState uint64 `json:"rngState,omitempty"`
}

// ProceduralPersistedState is the on-disk shape returned by
// SnapshotPersistedState() on every procedural-prototype effect (state + RNG).
type ProceduralPersistedState struct {
	ProceduralState
	RNGState uint64 `json:"rngState"`
}

func cloneConfig(src ProceduralConfig) ProceduralConfig {
	if src == nil {
		return ProceduralConfig{}
	}
	out := make(ProceduralConfig, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneTimerMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(src))
	for k, v := range src {
		if v > 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneValueMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
