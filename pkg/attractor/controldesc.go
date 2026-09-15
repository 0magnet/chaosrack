//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

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

// builtControls holds Controls constructed from descriptors (persistent, unlike
// paramControls which is cleared per param-panel rebuild). buildControlModel
// will reuse these so reset / permalink / tooltip read owned metadata rather
// than re-deriving from the DOM.
var builtControls []*Control

// buildDescControl builds one control from its descriptor: a hidden range
// slider (the value source of truth), an LED numeric readout with a
// single-sourced format, wheel-to-step, and a reset button — all owned by a
// Control registered in builtControls. The knob and the cell's placement are
// the caller's job (compose with makeKnob(ctl.slider) etc.), so this stays
// layout-agnostic. Returns the Control and the numeric-readout element.
//
// This is the one place that decides a control's LED format, reset behavior,
// and value-change plumbing, so those can never again drift between call sites.
//
//nolint:unused // the CREATE path of the registry — dormant until modules are built from descriptors
func buildDescControl(d ControlDesc) (*Control, js.Value) {
	dec := ledDecimals(d.Step)
	intDig := ledIntDigits(d.Min, d.Max)

	slider := doc.Call("createElement", "input")
	slider.Set("type", "range")
	slider.Set("id", d.ID)
	slider.Set("min", strconv.FormatFloat(d.Min, 'g', -1, 64))
	slider.Set("max", strconv.FormatFloat(d.Max, 'g', -1, 64))
	slider.Set("step", strconv.FormatFloat(d.Step, 'g', -1, 64)) // before value so the thumb isn't snapped
	slider.Set("value", strconv.FormatFloat(d.Def, 'g', -1, 64))
	slider.Set("style", "display:none;")

	ctl := &Control{
		module: "", kind: kindGeneric, slider: slider, def: float32(d.Def),
		ledInt: intDig, ledDec: dec, ledSign: d.Signed, permaKey: d.PermaKey,
	}

	led := doc.Call("createElement", "input")
	led.Set("type", "text")
	led.Set("inputmode", "decimal")
	led.Set("className", "numin u-val")
	led.Set("value", ctl.formatValue(d.Def))
	sizeLEDField(led, d.Min, d.Max, dec, d.Signed)

	apply := func(v float64) {
		led.Set("value", ctl.formatValue(v))
		if d.Apply != nil {
			d.Apply(v)
		}
	}
	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			apply(v)
		}
		return nil
	}))
	led.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(led.Get("value").String(), 64); err == nil {
			slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			apply(v)
		}
		return nil
	}))
	wheelNudge(led, slider, d.Step, d.Min, d.Max)

	builtControls = append(builtControls, ctl)
	return ctl, led
}

// adoptDescControl wires an EXISTING template-declared control (slider + LED
// readout + reset button already in the DOM) onto a descriptor, making the
// descriptor the single owner of its behavior: LED format, slider→state
// plumbing, typed LED entry, wheel-to-step, and reset (button + Reset All via
// builtControls). The template keeps owning layout/labels/tooltips; this owns
// everything that used to be spread across attachSliderInput,
// linkNumToSlider, a bespoke reset handler, and an onResetAll literal — each
// of which could (and did) silently miss a control.
func adoptDescControl(d ControlDesc) *Control { //nolint:unparam // callers will use the Control as migration continues
	slider := doc.Call("getElementById", d.ID)
	if !slider.Truthy() {
		return nil
	}
	// LED format derives from the DISPLAY domain (differs from the slider
	// domain only for mapped controls like Speed's log slider).
	ledStep, ledMin, ledMax := d.Step, d.Min, d.Max
	if d.LEDStep != 0 {
		ledStep = d.LEDStep
	}
	if d.LEDMax != 0 {
		ledMin, ledMax = d.LEDMin, d.LEDMax
	}
	dec := ledDecimals(ledStep)
	intDig := ledIntDigits(ledMin, ledMax)

	ctl := &Control{
		module: "", kind: kindGeneric, slider: slider, def: float32(d.Def),
		ledInt: intDig, ledDec: dec, ledSign: d.Signed, permaKey: d.PermaKey,
		resetHook: d.ResetExtra,
	}

	disp := func(s float64) float64 {
		if d.SliderToVal != nil {
			return d.SliderToVal(s)
		}
		return s
	}

	led := doc.Call("getElementById", d.LEDID)
	if led.Truthy() {
		led.Set("type", "text") // LED display: free-form so +/- and trailing zeros stick
		sizeLEDField(led, ledMin, ledMax, dec, d.Signed)
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			led.Set("value", ctl.formatValue(disp(v)))
		}
	}

	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			if d.Apply != nil {
				d.Apply(v)
			}
			if led.Truthy() {
				led.Set("value", ctl.formatValue(disp(v)))
			}
		}
		return nil
	}))
	if led.Truthy() {
		// Typed entry commits on Enter/blur ("change", not "input", so the
		// slider handler's formatted write-back doesn't fight typing).
		led.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if v, err := strconv.ParseFloat(led.Get("value").String(), 64); err == nil {
				if d.ValToSlider != nil {
					v = d.ValToSlider(v)
				}
				slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
				slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
			return nil
		}))
		wheelNudge(led, slider, d.Step, d.Min, d.Max)
	}
	if rb := doc.Call("getElementById", d.ResetID); rb.Truthy() {
		rb.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			ctl.resetToDefault()
			return nil
		}))
	}

	builtControls = append(builtControls, ctl)
	return ctl
}
