//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"
)

// Arrow keys turn the knob under the pointer.
//
// Every knob already responds to the wheel, which quietly assumes a wheel:
// a trackpad without one, a trackball, a tablet, or anyone driving the panel
// from the keyboard had no way to nudge a value by exactly one step. Dragging
// works but is a coarse instrument for a control whose whole point is a
// precise increment.
//
// It is scoped to HOVER rather than focus because that is how the wheel
// already behaves — you point at a knob and turn it, without clicking first —
// and because these knobs are divs wrapping a hidden input, so there is
// nothing natural to focus. Hovering also makes the binding unambiguous: at
// most one knob is under the pointer, so there is never a question of which
// control the key meant.
//
// Up and Right increase, Down and Left decrease, matching a native range
// input. The step is the SAME step the wheel uses, read from the same closure,
// so the two can never drift apart — including the live "Fine ×" ratio when
// the pointer is over a nested knob's inner disc.

// hoverNudge is the knob currently under the pointer, or nil. It is a function
// rather than an element because what a nudge means belongs to the knob that
// registered it: a value knob steps its slider, a selector knob steps its
// options, and the inner disc of a nested knob steps by the fine ratio.
var hoverNudge func(up bool)

// registerKnobHover wires a knob element so that pointing at it makes its
// nudge the live one. Both the outer knob and a nested fine disc call this
// with their own nudge, so the inner disc wins while the pointer is over it —
// which is the same precedence the wheel has, since the disc sits on top.
func registerKnobHover(el js.Value, nudge func(up bool)) {
	if !el.Truthy() {
		return
	}
	el.Call("addEventListener", "pointerenter", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		hoverNudge = nudge
		return nil
	}))
	el.Call("addEventListener", "pointerleave", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		// Only clear if this knob is still the live one. A pointerleave on the
		// outer knob arrives when the pointer crosses onto the inner disc,
		// which has already claimed the nudge — clearing unconditionally would
		// disarm the disc the instant it was pointed at.
		if hoverNudge != nil {
			hoverNudge = nil
		}
		return nil
	}))
}

// wireKnobArrowKeys installs the one document-level key listener.
func wireKnobArrowKeys() {
	doc.Call("addEventListener", "keydown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if hoverNudge == nil {
			return nil
		}
		e := a[0]
		// Never take a key away from somewhere text can be typed: the equation
		// editor, the banner text field, the LED number boxes. A native input
		// has its own arrow behavior and it has to win.
		if t := e.Get("target"); t.Truthy() {
			switch strings.ToLower(t.Get("tagName").String()) {
			case "input", "select", "textarea":
				return nil
			}
			if t.Get("isContentEditable").Truthy() {
				return nil
			}
		}
		// A modifier means the key belongs to the browser or the window
		// manager, not to us.
		if e.Get("ctrlKey").Truthy() || e.Get("metaKey").Truthy() || e.Get("altKey").Truthy() {
			return nil
		}
		var up bool
		switch e.Get("key").String() {
		case "ArrowUp", "ArrowRight":
			up = true
		case "ArrowDown", "ArrowLeft":
			up = false
		default:
			return nil
		}
		// Only now: preventDefault would otherwise stop the page scrolling for
		// keys we are not handling, and would take the arrows from Scope Pong
		// whenever the pointer happened to rest anywhere other than a knob.
		e.Call("preventDefault")
		e.Call("stopPropagation")
		hoverNudge(up)
		return nil
	}))
}
