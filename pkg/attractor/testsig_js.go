//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The Test module — the stimulus library's front panel.
//
// Two controls, because there are only two things to say: which signal, and how
// loud. The list itself lives in audiosrc (testsig.go), where the synthesis and
// its tests are, so the dial cannot come to name a signal the generator does
// not make.
//
// It replaces the X/Y/Z oscillators while it is on rather than mixing with
// them. A measurement wants a DEFINED signal, and a defined signal summed with
// whatever the oscillators happened to be left set to is not one — the
// distortion figure would be measuring the oscillators. The oscillators keep
// their settings and come back untouched at the "off" position.

// buildTestSignalModule fills the selector from the library and wires both
// controls. Called once, from the same place the generator module is built.
func buildTestSignalModule() {
	sel := doc.Call("getElementById", "testsig-sel")
	stack := doc.Call("getElementById", "testsig-stack")
	lvl := doc.Call("getElementById", "testsig-lvl")
	lvlLED := doc.Call("getElementById", "testsig-lvl-led")
	lstack := doc.Call("getElementById", "testsig-lstack")
	if !sel.Truthy() || !stack.Truthy() || !lvl.Truthy() {
		return
	}

	// The options come from the library's own list rather than from the HTML,
	// so adding a stimulus is one edit in one file.
	for i, name := range audiosrc.TestSignalNames {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", name)
		sel.Call("appendChild", opt)
	}
	sel.Set("value", "0")

	knob := makeSelectorKnob(sel)
	stack.Call("appendChild", knob)
	addSelectorLabels(knob, audiosrc.TestSignalRing, sel, 50)

	lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
	sizeLEDField(lvlLED, 0, 100, 1, false)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))

	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		applyTestSignal()
		return nil
	}))
	lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		v := fgFloat(lvl)
		lvlLED.Set("value", formatLED(v, intDigits(100), 1, false))
		fg().SetTestLevel(v / 100)
		return nil
	}))
	lvlLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlLED.Get("value").String(), 64); err == nil {
			lvl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			lvl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))
	applyTestSignal()
	fg().SetTestLevel(fgFloat(lvl) / 100)
}

// testSignalSel is the selector's current position.
func testSignalSel() audiosrc.TestSignal {
	sel := doc.Call("getElementById", "testsig-sel")
	if !sel.Truthy() {
		return audiosrc.TestOff
	}
	i, err := strconv.Atoi(sel.Get("value").String())
	if err != nil || i < 0 || i >= audiosrc.TestSignalCount {
		return audiosrc.TestOff
	}
	return audiosrc.TestSignal(i)
}

// applyTestSignal pushes the selector to the generator, and switches the
// generator ON when a stimulus is picked.
//
// Turning it on is the point: the Test module is in the Console's module list
// beside twenty others, and a signal selected on a generator nobody has enabled
// produces silence and looks broken. Picking a stimulus is an unambiguous
// request for it. Returning to "off" does NOT switch the generator back off —
// that would take away a source somebody may have been using before they came
// here, and leaving it running is the reversible half.
func applyTestSignal() {
	s := testSignalSel()
	fg().SetTestSignal(s)
	if s == audiosrc.TestOff {
		return
	}
	if sw := doc.Call("getElementById", "fg-on"); sw.Truthy() && !sw.Get("checked").Bool() {
		sw.Set("checked", true)
		sw.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
}
