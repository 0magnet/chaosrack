//go:build js && wasm

package attractor

import (
	"syscall/js"
	"time"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// Mini knobs: the trimmers a cell carries under its main knob, a step
// knob's size, for the settings that go with the main one — a generator's
// attack and decay beside its level, the beam's focus and illumination beside
// its intensity, the trace's position beside the tube. They used to be rings
// on the main knob's shaft, three to a cell, and which ring did what could
// not be told by looking.
//
// A mini knob has no scale and no display of its own. Its name is printed on
// its cap, and turns with it, the way a trimmer's legend is molded into the
// knob; where there is room, the cell's display reads it while it is turned
// (lendReadout).

// miniKnob is a mini knob driving slider, with legend on its cap. dial puts a
// scale of ticks round it, without numbers: there is no room for them, and
// the cell's display reads the knob while it turns. register is makeKnob's:
// keep it in step with its value from outside the panel build.
func miniKnob(slider js.Value, legend string, register, dial bool) js.Value {
	k := makeKnob(slider, js.Undefined(), false, register, dial)
	k.Get("classList").Call("add", "miniknob")
	if knob := k.Call("querySelector", ".knob"); knob.Truthy() {
		knob.Set("title", slider.Get("title").String())
	}
	// Inside the pointer, at its pivot, which is the knob's center: the
	// pointer's turn is the legend's.
	if ptr := k.Call("querySelector", ".knob-ptr"); ptr.Truthy() {
		c := dom.Doc.Call("createElement", "b")
		c.Set("className", "knob-cap")
		c.Set("textContent", legend)
		ptr.Call("appendChild", c)
	}
	return k
}

// miniRow stands mini knobs side by side under the cell's main knob.
func miniRow(knobs ...js.Value) js.Value {
	col := dom.Doc.Call("createElement", "span")
	col.Set("className", "minirow")
	for _, k := range knobs {
		col.Call("appendChild", k)
	}
	return col
}

// readoutLoan is a cell's display lent to its mini knobs: while one is turned
// by hand the display reads it, its legend saying which, and a moment after
// the hand leaves it goes back to the main knob.
//
// The legend changes, so it is a character display, not print: the bank's
// vertical legend, the same part, where the printed label stood.
type readoutLoan struct {
	col      js.Value // the minis
	led      js.Value // the display
	legend   js.Value // its legend display
	home     string   // the legend the display has when it is not lent
	giveBack func()   // rewrites the main knob's value into the display
	touched  float64  // when a hand was last on the minis (frameNowMs)
	timer    *time.Timer
}

// lendReadout lends cell's display (led, whose legend is the .plabel beside
// it) to the minis in col.
func lendReadout(col, led js.Value, giveBack func()) *readoutLoan {
	l := &readoutLoan{col: col, led: led, giveBack: giveBack}
	if led.Truthy() {
		if lbl := led.Get("parentNode").Call("querySelector", ".plabel"); lbl.Truthy() {
			l.home = lbl.Get("textContent").String()
			// The printed label stays, hidden, as the cell's name for its
			// tooltips; the display is what is seen.
			lbl.Get("classList").Call("add", "loansrc")
			l.legend = dotDisplayN(l.home, true, bankMaxChars)
			l.legend.Get("classList").Call("add", "loanlbl")
			lbl.Get("parentNode").Call("insertBefore", l.legend, lbl)
		}
	}
	// Capture phase: a knob stops the pointer events it answers.
	touch := dom.FuncOf(func(js.Value, []js.Value) any {
		l.touched = frameNowMs
		return nil
	})
	opts := map[string]any{"capture": true, "passive": true}
	for _, ev := range []string{"pointerdown", "wheel"} {
		col.Call("addEventListener", ev, touch, opts)
	}
	col.Call("addEventListener", "pointermove", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		if a[0].Get("buttons").Int() != 0 {
			l.touched = frameNowMs
		}
		return nil
	}), opts)
	return l
}

// show puts legend and value on the display, if a hand is on the minis: a
// reset or a restored link turns them without anyone looking at the cell.
func (l *readoutLoan) show(legend, value string) {
	turning := kb.active && kb.knobEl.Truthy() && l.col.Call("contains", kb.knobEl).Bool()
	if !l.led.Truthy() || !l.legend.Truthy() || !turning && (l.touched == 0 || frameNowMs-l.touched > 1500) {
		return
	}
	setDotText(l.legend, legend)
	l.led.Set("value", value)
	if l.timer != nil {
		l.timer.Stop()
	}
	l.timer = time.AfterFunc(1500*time.Millisecond, func() {
		setDotText(l.legend, l.home)
		l.giveBack()
	})
}
