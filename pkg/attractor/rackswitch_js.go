//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/controlspec"
)

// The switches, which the registry did not have.
//
// adoptDescControl records a control as it wires it, and every knob and rotary
// on the panel goes through it — 72 of them. The switches do not: they are
// plain checkboxes wired one at a time by fifteen different functions
// (wireScopeSwitch, wireMIDISwitch, wireRecordSwitch, the row switches, the
// per-module ones…), and none of those ever told the registry.
//
// Measured on the running page: 72 controls listed, 61 switches not listed.
// Anything driving the rack from outside — the terminal panel, the cable, a
// command line — was blind to nearly half the surface, and the half it could
// not see is the half that turns things ON.
//
// Swept rather than registered at each site, deliberately. Fifteen call sites
// would be fifteen chances to forget the sixteenth, and the sweep cannot go
// out of date: it reports what the panel HAS. It belongs in a js file because
// it is a DOM question, which is the same reason the registry itself is pure
// and only its builders are tagged.

// switchControls is every two-state control on the panel, as plain data.
func switchControls() []controlspec.ControlInfo {
	if !dom.Doc.Truthy() {
		return nil
	}
	els := dom.Doc.Call("querySelectorAll", "input[type=checkbox]")
	n := els.Get("length").Int()
	out := make([]controlspec.ControlInfo, 0, n)
	for i := 0; i < n; i++ {
		el := els.Index(i)
		id := el.Get("id").String()
		if id == "" {
			continue // unaddressable: nothing outside could name it anyway
		}
		out = append(out, controlspec.ControlInfo{
			ID:       id,
			Label:    switchLabel(el, id),
			IsSwitch: true,
			Def:      switchDefault(el),
			Module:   moduleOfControl(id),
		})
	}
	return out
}

// switchLabel is what is written beside the switch.
//
// A switch is marked up as the checkbox INSIDE its own label — see the Console
// markup — so the text is the parent's, with the input's own (empty) text
// removed by the trim. Falling back to the id keeps a switch listable even
// when it is unlabeled, which is better than hiding it again.
func switchLabel(el js.Value, id string) string {
	lab := el.Call("closest", "label")
	if lab.Truthy() {
		if t := strings.TrimSpace(lab.Get("textContent").String()); t != "" {
			return t
		}
	}
	if t := el.Call("getAttribute", "title"); t.Truthy() {
		if s := strings.TrimSpace(t.String()); s != "" {
			return s
		}
	}
	return id
}

// switchDefault is the position the panel is built with, which is the one
// "reset" means. Read from the ATTRIBUTE rather than the property: the
// attribute is what the markup asked for and the property is where the user
// has since put it.
func switchDefault(el js.Value) float64 {
	if el.Call("hasAttribute", "checked").Bool() {
		return 1
	}
	return 0
}

// controlValueOf reads a control, whatever kind it is.
//
// The whole reason this exists: a checkbox's .value is the string "on" whether
// it is checked or not — that is the HTML form value, not the state — so
// reading .value off a switch reports "on" for a switch that is off. `ctl
// audio-mod` said "on" with modulation plainly disabled.
func controlValueOf(el js.Value) string {
	if isSwitchEl(el) {
		if el.Get("checked").Bool() {
			return "1"
		}
		return "0"
	}
	return el.Get("value").String()
}

// setControlValue writes a control, whatever kind it is, and returns whether
// anything was written.
//
// The write half of the same bug: assigning .value on a checkbox sets the form
// value and leaves the state alone, so `ctl audio-mod=1` changed nothing at
// all except what a later read would report.
func setControlValue(el js.Value, v string) {
	if isSwitchEl(el) {
		el.Set("checked", switchOn(v))
		return
	}
	el.Set("value", v)
}

// switchOn reads a switch position written as text. Generous on purpose: a
// command line is typed by a person, and 1/on/true/yes all plainly mean on.
func switchOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes", "y":
		return true
	}
	return false
}

func isSwitchEl(el js.Value) bool {
	return el.Truthy() &&
		strings.EqualFold(el.Get("tagName").String(), "input") &&
		el.Get("type").String() == "checkbox"
}

// dispatchControlEvents fires what the panel listens for. Both, because a
// range reports dragging as "input" and settling as "change", and different
// controls were wired to different ones.
func dispatchControlEvents(el js.Value) {
	ev := js.Global().Get("Event")
	for _, kind := range []string{"input", "change"} {
		opt := js.Global().Get("Object").New()
		opt.Set("bubbles", true)
		el.Call("dispatchEvent", ev.New(kind, opt))
	}
}

// rackControls is the whole surface: the knobs the registry recorded as it
// wired them, and the switches swept off the panel.
func rackControls() []controlspec.ControlInfo {
	reg := ControlRegistry()
	sw := switchControls()
	out := make([]controlspec.ControlInfo, 0, len(reg)+len(sw))
	out = append(out, reg...)
	// Only the ones the registry does not already have. A switch that is
	// adopted properly one day should not then appear twice.
	have := make(map[string]bool, len(reg))
	for _, c := range reg {
		have[c.ID] = true
	}
	for _, c := range sw {
		if !have[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// wireSwitch attaches a switch to the thing it turns on, and applies the
// position it is already in.
//
// It was wireScopeSwitch, in rackscope_js.go, parameterized exactly like this
// and used by the scope. Four others — jam, sect, twin, MIDI — were the same
// function written again with the id inlined and the body in place of the
// closure, which is how the registry came to be missing them: each site was
// its own chance to forget to record one.
//
// The trailing set() is the part the copies left out. A switch that starts
// closed — because the markup says so, or a permalink restored it — otherwise
// never runs its effect until a hand moves it, and the Go state disagrees with
// the panel until then. Every switch in the rack currently starts open, so
// that was a hazard rather than a fault; applying it makes the two agree by
// construction instead of by coincidence.
func wireSwitch(id string, set func(bool)) {
	el := dom.Doc.Call("getElementById", id)
	if !el.Truthy() {
		return
	}
	el.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
		set(el.Get("checked").Bool())
		return nil
	}))
	set(el.Get("checked").Bool())
}
