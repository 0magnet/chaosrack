//go:build js && wasm

package attractor

import (
	"encoding/json"
	"syscall/js"
)

// The rack, drivable from outside it.
//
// Every control in the panel is a hidden <input> with an id and a change
// listener — that is what adoptDescControl builds — so "turn this knob" has
// always been one assignment and one dispatched event away. What was missing
// was the LIST: nothing outside the wasm could say which controls exist, what
// they are called, or what range they run over, so anything driving the rack
// had to know the ids in advance and hope.
//
// ControlRegistry is that list, recorded as the panel is wired. Handing it out
// here turns the rack into something a terminal can drive: list the surface,
// read a value, set a value. It is a remote control and not a headless rack —
// the instrument is still wasm in a page, and this is the cable to it — but it
// is the difference between "write a bespoke probe, build it, dial CDP, eval
// some JS, parse the result" and "ask".
//
// The same list is what a second front end would render. A tcell panel and
// this cable are the same question asked twice, which is why the registry
// lives in untagged Go and only this shim is tagged.

// exposeRackControl installs window.rackctl. Called once from Run.
func exposeRackControl() {
	o := js.Global().Get("Object").New()

	// list() → every control, as JSON. Plain data, so the caller does not
	// need a Go type to read it.
	o.Set("list", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		b, err := json.Marshal(ControlRegistry())
		if err != nil {
			return "[]"
		}
		return string(b)
	}))

	// get(id) → the control's current value as a string, or null if the rack
	// has no such control. The element is the source of truth: a descriptor
	// says what a control CAN be, the input says what it is.
	o.Set("get", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) == 0 {
			return nil
		}
		el := doc.Call("getElementById", a[0].String())
		if !el.Truthy() {
			return nil
		}
		return el.Get("value")
	}))

	// set(id, v) → drives the control the way a hand would: write the value
	// and dispatch the events the panel listens for. Both, because a range
	// input reports dragging as "input" and settling as "change", and
	// different controls were wired to different ones.
	o.Set("set", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) < 2 {
			return false
		}
		el := doc.Call("getElementById", a[0].String())
		if !el.Truthy() {
			return false
		}
		el.Set("value", a[1])
		ev := js.Global().Get("Event")
		for _, kind := range []string{"input", "change"} {
			opt := js.Global().Get("Object").New()
			opt.Set("bubbles", true)
			el.Call("dispatchEvent", ev.New(kind, opt))
		}
		return true
	}))

	js.Global().Set("rackctl", o)
}
