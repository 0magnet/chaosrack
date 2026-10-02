//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
)

// dialTicks is how many ticks a value dial has, and so how many LEDs its
// ring: 21 across the knob's travel, every fifth a major one.
const dialTicks = 21

// lightRing lights LEDs from..to of a value dial's ring.
//
// One attribute, not a class on each tick: the lit set is written as a list
// of indices and the stylesheet lights the ticks it names (panel.css, "THE
// LED RING"). Toggling 21 classes from Go on every movement of every knob is
// 21 crossings into the page where this is one, and the crossing is what a
// DOM write from Go costs.
func lightRing(dial js.Value, from, to int) {
	setLEDList(dial, "data-lit", from, to)
}

// setLEDList writes LEDs from..to into one of a dial's list attributes.
func setLEDList(dial js.Value, attr string, from, to int) {
	var b strings.Builder
	for i := from; i <= to; i++ {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Itoa(i))
	}
	dial.Call("setAttribute", attr, b.String())
}
