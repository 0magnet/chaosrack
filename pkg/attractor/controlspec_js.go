//go:build js && wasm

package attractor

// A control's range and step, kept here rather than read back off its slider.
//
// The slider's min, max and step attributes are not a record of anything:
// a knob with a fine ring divides step by a thousand so the ring can move
// the value between steps (makeKnob), an endless knob widens min and max to
// its laps (makeTurning), and the step trim rewrites step again. Code that
// read them after any of that read what the last writer left — the Grid's
// FROM showed a step of 1E-05 — and the knob kept the step it was built
// with, so turning the step trim changed only where the value snapped to.
//
// So the range and step are recorded once, by the builder that knows them,
// and everything that wants them asks here. The attributes are left to do
// the one thing the browser needs them for: the slider's own domain, and
// the quantum its value is snapped to.

import (
	"strconv"
	"syscall/js"
)

// ctlSpec is a control's range as authored and the step it turns by now.
type ctlSpec struct {
	lo, hi float64
	step   float64 // one knob step; the step trim sets it
	fine   bool    // its knob has a fine ring, which turns a tenth of a step
}

// ctlSpecs are the specs, by slider id.
var ctlSpecs = map[string]*ctlSpec{}

// setSpec records the range and step of the slider with id id, as the
// builder that made it says: a rebuilt control replaces what its last build
// recorded (a custom equation's constants can change range). In place, since
// a knob keeps the record it was made with.
func setSpec(id string, lo, hi, step float64) {
	if id == "" {
		return
	}
	if step <= 0 {
		step = (hi - lo) / 100
	}
	if s, ok := ctlSpecs[id]; ok {
		s.lo, s.hi, s.step = lo, hi, step
		return
	}
	ctlSpecs[id] = &ctlSpec{lo: lo, hi: hi, step: step}
}

// specOf is slider's spec. One no builder recorded is read from its
// attributes the first time it is asked for, which is before anything has
// rewritten them: the markup's own numbers.
func specOf(slider js.Value) *ctlSpec {
	id := slider.Get("id").String()
	if s, ok := ctlSpecs[id]; ok {
		return s
	}
	num := func(attr string) float64 {
		f, _ := strconv.ParseFloat(slider.Get(attr).String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the fallback
		return f
	}
	s := &ctlSpec{lo: num("min"), hi: num("max"), step: num("step")}
	if s.step <= 0 {
		s.step = (s.hi - s.lo) / 100
	}
	if id != "" && slider.Truthy() {
		ctlSpecs[id] = s
	}
	return s
}

// quantum is what the slider's value is snapped to: a step, or a
// thousandth of one under a fine ring, so the ring can move between steps.
func (s *ctlSpec) quantum() float64 {
	if s.fine {
		return s.step * 0.001
	}
	return s.step
}

// setStep makes step the control's step, and the slider's snapping follow.
func (s *ctlSpec) setStep(slider js.Value, step float64) {
	if !(step > 0) {
		return
	}
	s.step = step
	slider.Set("step", strconv.FormatFloat(s.quantum(), 'g', -1, 64))
}

// stepText is the step as a step readout shows it.
func (s *ctlSpec) stepText() string { return strconv.FormatFloat(s.step, 'g', -1, 64) }

// parseOr0 is s as a number, or 0.
func parseOr0(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64) //nolint:errcheck // zero is the fallback
	return f
}
