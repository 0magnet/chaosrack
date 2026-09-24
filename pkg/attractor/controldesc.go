package attractor

import "github.com/0magnet/chaosrack/pkg/controlspec"

// The control surface, described once and rendered by whoever is rendering.
//
// This file is UNTAGGED and the builders beside it are not, which is the whole
// point of the split. A ControlDesc says what a control IS — its range, its
// step, its default, whether it is a switch or a dial, what it does when it
// moves, how it is spelled in a permalink. None of that is a fact about the
// DOM, and none of it needs a browser to be true.
//
// What the DOM half does with it (controldesc_js.go) is one way of drawing it:
// a hidden range input, an LED, a knob, a reset button. A terminal is another,
// and the rack's layout is already renderable that way — see rackascii.go, and
// racksection.go, pkg/racksurface and pkg/rackspec, which have always been pure.
// The controls were the last piece still welded to one front end.

// ControlDesc is the single, declarative description of one panel control.
//
// It is the foundation of the "control registry" refactor (initiative A of the
// architecture audit): today a *param* control needs one table row
// (attractorParams) which already drives its knob, LED format, reset, and
// permalink — but every *fixed* control (Zoom, Pan, Speed, Line, Trail, the
// spin rates, colors, selectors) is wired imperatively across ~8 sites in 3
// files (HTML declaration, knobifyFixed, attachSliderInput's LED-format
// re-derivation, linkNumToSlider, a bespoke reset handler, an onResetAll
// literal, a permaCtls row, a viewModTargets entry). Each omission is a distinct
// silent bug (LED clip, not-shareable, not-reset, not-modulatable).
//
// One ControlDesc per control is meant to generate ALL of that from one place:
// the slider + LED readout, the value↔LED↔knob sync, the reset, the permalink
// (serialize + restore), the audio-mod target, and the tooltip. buildDescControl
// below is the generic builder, modeled on the proven buildParamUnit path.
//
// NOTE: this type + builder are the scaffolding; wiring the existing fixed
// controls onto it happens module-by-module with live verification (they are
// unused until then, which is intentional).
type ControlDesc struct {
	ID        string          // DOM id of the (hidden) <input type=range> value source
	Label     string          // short cell label
	Min, Max  float64         // value range
	Step      float64         // coarse step (also drives LED decimal places)
	Def       float64         // default: reset target, and "omit if equal" for the permalink
	Signed    bool            // LED shows an explicit sign
	PermaKey  string          // short permalink key (e.g. "z"); "" ⇒ not serialized
	ModTarget bool            // exposed as an audio-mod routing target
	Apply     func(v float64) // the control-specific effect to run when the value changes

	// Adopt-path fields (adoptDescControl): ids of the template-declared
	// elements the descriptor takes ownership of, plus any extra work a reset
	// needs beyond restoring the value (e.g. Zoom also recenters the camera).
	// A SELECTOR-backed control. IsSelect is what tells adoptDescControl which
	// kind this is, and SelectDef is the option value a reset returns to —
	// Min/Max/Step/Def and the LED fields are all meaningless for a select,
	// whose value is one of a named set rather than a number on a scale.
	//
	// IsSelect is a field of its own rather than "SelectDef is non-empty",
	// which is what it used to be, because the empty string is a perfectly good
	// option value: the Backdrop and Skin rings both default to OFF, whose
	// value is "". Under the old rule those two read as numeric controls, took
	// the slider path, found no slider, and silently registered nothing — the
	// exact class of silent omission this type exists to end.
	//
	// Apply still receives a float64 for a numeric control; a selector's effect
	// goes in SelectApply, which gets the option value as the string it is.
	IsSelect    bool
	SelectDef   string
	SelectApply func(v string)

	// SkipResetAll keeps this control out of the Reset All sweep while still
	// giving it a reset button of its own. Exactly one control wants that: the
	// interface Size ring is a display preference, like the dock edge, and
	// resizing somebody's whole panel because they asked for a fresh view of the
	// model is not what that button is for. Having its own reset is still right
	// — a size you cannot get back out of is state with no way home.
	SkipResetAll bool

	LEDID      string // existing numeric-readout element id
	ResetID    string // existing reset-button id
	ResetExtra func()

	// Display mapping, for controls whose slider runs in a different domain
	// than the value the LED shows (Speed: log10 slider -2..2, LED 0.01..100).
	// Min/Max/Step/Def above are always the SLIDER domain (attributes, wheel
	// steps, reset target); these describe the DISPLAYED value. All optional —
	// identity controls leave everything nil/zero.
	SliderToVal func(s float64) float64 // slider raw → displayed value
	ValToSlider func(v float64) float64 // typed display value → slider raw
	LEDMin      float64                 // display range for LED sizing (used when LEDMax != 0)
	LEDMax      float64
	LEDStep     float64 // display step driving LED decimals (0 ⇒ Step)
}

// ── The surface, enumerable ──────────────────────────────────────────────

// ControlInfo is a control as plain data. The type lives in pkg/controlspec
// so that a front end and the rack can both have it without either importing
// the other — which is what putting a terminal panel inside the rack needs.
type ControlInfo = controlspec.ControlInfo

// Info is the plain-data half of a descriptor.
func (d ControlDesc) Info() ControlInfo {
	return ControlInfo{
		ID: d.ID, Label: d.Label,
		Min: d.Min, Max: d.Max, Step: d.Step, Def: d.Def,
		IsSelect: d.IsSelect, SelectDef: d.SelectDef,
		PermaKey: d.PermaKey, ModTarget: d.ModTarget,
	}
}

// controlRegistry is every control the rack has adopted, in the order it was
// built. Recorded as the panel is wired rather than declared separately: a
// second list would be a list that can be wrong, and the failure would be a
// control missing from a front end with nothing to say so.
var controlRegistry []ControlInfo

// registerControl records a control as it is adopted. Idempotent by id, so a
// panel rebuilt in place does not double the surface.
func registerControl(d ControlDesc, module string) {
	if d.ID == "" {
		return
	}
	info := d.Info()
	info.Module = module
	for i, c := range controlRegistry {
		if c.ID == d.ID {
			controlRegistry[i] = info
			return
		}
	}
	controlRegistry = append(controlRegistry, info)
}

// ControlRegistry is every control in the rack, as plain data. The order is
// the order the panel was wired in, which is the order the rack reads.
func ControlRegistry() []ControlInfo {
	out := make([]ControlInfo, len(controlRegistry))
	copy(out, controlRegistry)
	return out
}
