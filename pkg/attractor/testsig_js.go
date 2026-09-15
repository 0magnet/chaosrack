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

	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "testsig-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 50,
		LEDID: "testsig-lvl-led", ResetID: "rst-testsig-lvl",
		Apply: func(v float64) { fg().SetTestLevel(v / 100) },
	})
	adoptDescControl(ControlDesc{
		ID: "testsig-sel", Label: "sig", IsSelect: true, SelectDef: "0", PermaKey: "tv",
		ResetID:     "rst-testsig-sel",
		SelectApply: func(string) { applyTestSignal() },
	})
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
