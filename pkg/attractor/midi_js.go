//go:build js && wasm

package attractor

// WebMIDI (Audio > MIDI): hardware knobs drive the panel. Fixed map, no
// learn step:
//   - CC 1..N → the current mode's float parameters, in panel order (a CC's
//     0–127 spans the parameter's full knob range);
//   - CC 21..28 → the view/motion targets (zoom, pan X/Y, spin X/Y/Z,
//     rainbow, trail) — the classic "knob row two" on compact controllers;
//   - any note-on → hop to that note's flow mode (pitch mod the catalog).
// Everything goes through the real sliders (input events), so LEDs, knobs,
// permalink and modulation stay truthful.

import (
	"strconv"
	"syscall/js"
)

var (
	midiOn     bool
	midiAccess js.Value
	midiMsgFn  js.Func
)

func midiSetSlider(id string, min, max float32, v127 float64) {
	sl := doc.Call("getElementById", id)
	if !sl.Truthy() {
		return
	}
	val := float64(min) + v127/127*float64(max-min)
	sl.Set("value", strconv.FormatFloat(val, 'g', 6, 64))
	sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

func midiHandle(this js.Value, args []js.Value) interface{} {
	if !midiOn || len(args) == 0 {
		return nil
	}
	data := args[0].Get("data")
	if !data.Truthy() || data.Get("length").Int() < 3 {
		return nil
	}
	status := data.Index(0).Int()
	d1 := data.Index(1).Int()
	d2 := data.Index(2).Int()
	switch status & 0xF0 {
	case 0xB0: // control change
		v := float64(d2)
		switch {
		case d1 >= 21 && d1 < 21+len(viewModTargets):
			vt := viewModTargets[d1-21]
			midiSetSlider(vt.anchor, vt.min, vt.max, v)
		case d1 >= 1:
			params := attractorParams[selectedMode]
			idx := 0
			for _, pd := range params {
				if decimalsForStep(pd.Step) == 0 {
					continue
				}
				idx++
				if idx == d1 {
					midiSetSlider(pd.ID, pd.Min, pd.Max, v)
					break
				}
			}
		}
	case 0x90: // note on
		if d2 == 0 {
			return nil
		}
		keys := ModeKeys(ClassFlow3D, ClassFlow4D)
		if len(keys) == 0 {
			return nil
		}
		next := keys[d1%len(keys)]
		if next != selectedMode {
			if sel := doc.Call("getElementById", "mode-select"); sel.Truthy() {
				sel.Set("value", next)
				sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			}
		}
	}
	return nil
}

func midiBindInputs() {
	if !midiAccess.Truthy() {
		return
	}
	inputs := midiAccess.Get("inputs")
	fn := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Set("onmidimessage", midiMsgFn)
		return nil
	})
	inputs.Call("forEach", fn)
}

func startMIDI() {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() || !nav.Get("requestMIDIAccess").Truthy() {
		return
	}
	if midiMsgFn.IsUndefined() {
		midiMsgFn = trackedFuncOf(midiHandle)
	}
	if midiAccess.Truthy() {
		midiBindInputs()
		return
	}
	then := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		midiAccess = a[0]
		midiBindInputs()
		// New devices plugged in later bind too.
		midiAccess.Set("onstatechange", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			midiBindInputs()
			return nil
		}))
		return nil
	})
	nav.Call("requestMIDIAccess").Call("then", then)
}

func wireMIDISwitch() {
	sw := doc.Call("getElementById", "midi-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		midiOn = sw.Get("checked").Bool()
		if midiOn {
			startMIDI()
		}
		return nil
	}))
}
