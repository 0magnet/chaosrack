//go:build js && wasm

package dom

import "syscall/js"

// Fire tells el's listeners that its value changed, as a hand on it would:
// an event named name ("input" or "change"), bubbling. It bubbles because the
// rack answers more and more of its controls from one listener on the
// document (a P-unit's buttons, the setting lists), and an event that does
// not bubble reaches the control's own listeners and nothing above it — so a
// value set by a link, a reset or MIDI left those answers stale.
func Fire(el js.Value, name string) {
	if !el.Truthy() {
		return
	}
	if !bubbles.Truthy() {
		bubbles = js.Global().Get("Object").New()
		bubbles.Set("bubbles", true)
	}
	el.Call("dispatchEvent", js.Global().Get("Event").New(name, bubbles))
}

// bubbles is the init every Fire uses, made once.
var bubbles js.Value
