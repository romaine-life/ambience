package rngutil

const defaultState uint64 = 0x9e3779b97f4a7c15

// RNG is a tiny deterministic PRNG with explicit serializable state.
// We use it where ambience needs future randomness to survive process restarts.
type RNG struct {
	state uint64
}

func New(seed int64) *RNG {
	return NewFromState(uint64(seed))
}

func NewFromState(state uint64) *RNG {
	if state == 0 {
		state = defaultState
	}
	return &RNG{state: state}
}

func (r *RNG) State() uint64 {
	return r.state
}

func (r *RNG) SetState(state uint64) {
	if state == 0 {
		state = defaultState
	}
	r.state = state
}

// Mix folds an external value into the RNG state.
func (r *RNG) Mix(delta int64) {
	r.state ^= uint64(delta)
	if r.state == 0 {
		r.state = defaultState
	}
}

func (r *RNG) Uint64() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *RNG) Int63() int64 {
	return int64(r.Uint64() >> 1)
}

func (r *RNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Uint64() % uint64(n))
}

func (r *RNG) Float64() float64 {
	return float64(r.Uint64()>>11) * (1.0 / (1 << 53))
}
