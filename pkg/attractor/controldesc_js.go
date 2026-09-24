//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"syscall/js"
)

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
	dec := led.Decimals(d.Step, fineRatio)
	intDig := led.IntDigits(d.Min, d.Max)

	slider := dom.Doc.Call("createElement", "input")
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

	led := dom.Doc.Call("createElement", "input")
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
	slider.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			apply(v)
		}
		return nil
	}))
	led.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
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
	// The module it is mounted in, read off the panel as it is adopted: the
	// descriptor does not name one, and the DOM is the only thing that knows.
	registerControl(d, moduleOfControl(d.ID)) // see ControlRegistry
	if d.IsSelect {
		return adoptSelectControl(d)
	}
	slider := dom.Doc.Call("getElementById", d.ID)
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
	dec := led.Decimals(ledStep, fineRatio)
	intDig := led.IntDigits(ledMin, ledMax)

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

	led := dom.Doc.Call("getElementById", d.LEDID)
	if led.Truthy() {
		// A readout is free-form text so +/- and trailing zeros stick. The
		// markup declares it type=number with min/max/step, which is valid
		// there and invalid the moment the type changes — 105 of the
		// validator's complaints were these, left behind by this line. The
		// range lives on the slider this readout mirrors, so nothing reads
		// them here.
		led.Set("type", "text")
		for _, a := range []string{"min", "max", "step"} {
			led.Call("removeAttribute", a)
		}
		sizeLEDField(led, ledMin, ledMax, dec, d.Signed)
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			led.Set("value", ctl.formatValue(disp(v)))
		}
	}

	slider.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
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
		led.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
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
	if rb := dom.Doc.Call("getElementById", d.ResetID); rb.Truthy() {
		rb.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			ctl.resetToDefault()
			return nil
		}))
	}

	builtControls = append(builtControls, ctl)
	return ctl
}

// adoptSelectControl is adoptDescControl's selector half: it takes over a
// <select> that already exists in the markup, wires its effect, and registers a
// Control so the reset button, Reset All and the permalink all reach it.
//
// Selectors were the last controls wired entirely by hand, and it showed in a
// way nothing else did. Eight of them — the test signal, the counter's gate,
// the Keys range and output, the Matrix step count and output, the Rhythm
// output, the distortion channel — had no reset button, were not touched by
// Reset All, and were not in the permalink. Turn one and there was no way back
// short of reloading the page, and no way to share the view you had made. That
// is not a missing convenience; it is state with no way home.
//
// The value is the select's own, exactly as for a slider-backed control: the
// element is the source of truth and everything else follows it, so a permalink
// restore and a reset and a click on the ring all take the same path.
func adoptSelectControl(d ControlDesc) *Control {
	sel := dom.Doc.Call("getElementById", d.ID)
	if !sel.Truthy() {
		return nil
	}
	ctl := &Control{
		module: "", kind: kindGeneric,
		sel: sel, selDef: d.SelectDef,
		skipResetAll: d.SkipResetAll,
		permaKey:     d.PermaKey, resetHook: d.ResetExtra,
	}
	if d.SelectApply != nil {
		sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			d.SelectApply(sel.Get("value").String())
			return nil
		}))
	}
	if rb := dom.Doc.Call("getElementById", d.ResetID); rb.Truthy() {
		rb.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			ctl.resetToDefault()
			return nil
		}))
	}
	builtControls = append(builtControls, ctl)
	return ctl
}

// moduleOfControl is the panel a control sits in, by the name the rack knows
// it by — its header text. Empty for a control that is not in a module, which
// the Console's own switches are.
func moduleOfControl(id string) string {
	if id == "" {
		return ""
	}
	el := dom.Doc.Call("getElementById", id)
	if !el.Truthy() {
		return ""
	}
	sect := el.Call("closest", ".sect")
	if !sect.Truthy() {
		return ""
	}
	return moduleKeyOf(sect)
}
