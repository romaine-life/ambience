package sim

// Schema types: a portable description of an effect's tunable knobs. Used by
// the dev UI to render controls dynamically so every effect can reuse the
// same panel layout without hardcoded HTML.

// KnobType is the interpretation of a knob's numeric range.
type KnobType string

const (
	KnobFloat KnobType = "float"
	KnobInt   KnobType = "int"
)

// KnobSlot maps a knob to its role in the 5-slot effect template. This tells
// the UI *when/how* the knob is applied, which controls grouping and visual
// ordering in the panel.
type KnobSlot string

const (
	// SlotSpawn — value fixed at particle/entity spawn time.
	SlotSpawn KnobSlot = "spawn"
	// SlotLever — continuous drift/tuning while the effect runs.
	SlotLever KnobSlot = "lever"
	// SlotEvent — frequency/probability of a discrete event firing.
	SlotEvent KnobSlot = "event"
	// SlotEventMod — per-event-instance randomization (duration, intensity).
	SlotEventMod KnobSlot = "event-mod"
	// SlotEnd — natural termination conditions (e.g. tetris lose-state).
	SlotEnd KnobSlot = "end"
)

// Knob describes one tunable parameter.
type Knob struct {
	// Key is the URL-query-param / JSON field name (no spaces).
	Key string `json:"key"`
	// Label is the human-readable name for the UI.
	Label string `json:"label"`
	// Slot categorizes the knob per the 5-slot template.
	Slot KnobSlot `json:"slot"`
	// Type controls how the raw value is parsed.
	Type KnobType `json:"type"`
	// Min / Max / Step / Default configure the slider control.
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	Default float64 `json:"default"`
	// Group is an optional sub-label within the slot for visual clustering
	// (e.g. "motion", "color"). Empty = ungrouped.
	Group string `json:"group,omitempty"`
	// Description is a short blurb (< 120 chars) explaining what the knob
	// does. Rendered as a hover tooltip in the dev UI.
	Description string `json:"description,omitempty"`
	// Trigger names an event that can be fired immediately via
	// POST /dev/trigger/<session>/<trigger>. Empty = no trigger button.
	// Used on SlotEvent knobs so the UI can render a "fire now" button
	// beside the probability slider.
	Trigger string `json:"trigger,omitempty"`
}

// EffectSchema is an effect's full control surface. Served from
// GET /effects/<name>/schema and consumed by the dev UI.
type EffectSchema struct {
	Name  string `json:"name"`
	Knobs []Knob `json:"knobs"`
}
