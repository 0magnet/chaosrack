//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"syscall/js"
)

// The Counter module — a frequency counter for the rack, in the spirit of
// the glensstuff.com NAND-gate counter. It measures the way the discrete
// original did: open a gate, COUNT the cycles that cross the trigger
// threshold, close the gate, latch the count onto the display. No FFT, no
// autocorrelation — cycles per second, literally. The input is whatever
// the shared audio source is delivering (mic, ws stream, or the signal
// generator), pulled through Drain so every sample is counted exactly
// once. A hysteresis trigger (the trig knob, % of full scale) rejects
// noise around zero the way a real counter's trigger-level pot does.

// freqCounter is the Counter module: its gate, its count and its LEDs.
type freqCounter struct {
	on      bool
	buf     []float32
	cycles  int
	samples int
	state   int // +1 above trigger, −1 below, 0 unarmed
	ledEl   js.Value
	gateEl  js.Value
	gateOn  bool
}

var counter freqCounter

// tick runs every frame from the render loop. It drains the shared
// source into the cycle counter and latches the readout each time the gate
// window's worth of samples has been counted.
func (f *freqCounter) tick() {
	if !f.on {
		return
	}
	// Same reason as the other analyzers: it counts crossings every frame,
	// and the drawer usually has it scrolled away. See moduleOnScreen.
	if !onScreen.moduleOnScreen("counter-module") {
		return
	}
	src := aud.ensureAudioSource()
	if src == nil || !src.Ready() || src.SampleRate() <= 0 {
		return
	}
	if f.buf == nil {
		f.buf = make([]float32, 16384)
	}
	n := tapRead(&counterCursor, f.buf)
	th := fgFloat(dom.Doc.Call("getElementById", "counter-trig")) / 100
	if th < 1e-4 { // trig at 0 still needs hysteresis or it chatters
		th = 1e-4
	}
	for i := range n {
		s := float64(f.buf[i])
		switch {
		case s >= th:
			if f.state < 0 {
				f.cycles++ // one full swing = one cycle
			}
			f.state = 1
		case s <= -th:
			if f.state == 0 {
				f.state = -1 // arm on first low excursion
			} else if f.state > 0 {
				f.state = -1
			}
		}
	}
	f.samples += n
	gate, _ := strconv.ParseFloat(dom.Doc.Call("getElementById", "counter-gatesel").Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	if gate <= 0 {
		gate = 1
	}
	if f.samples >= int(gate*float64(src.SampleRate())) {
		// Latch: cycles over the ACTUAL window (sample-exact, not wall time).
		hz := float64(f.cycles) / (float64(f.samples) / float64(src.SampleRate()))
		owed.readouts.Set("counter-hz", f.ledEl, led.Format(hz, 5, 1, false))
		f.cycles, f.samples = 0, 0
		// The gate lamp toggles at each latch — the classic heartbeat.
		f.gateOn = !f.gateOn
		if f.gateEl.Truthy() {
			f.gateEl.Get("classList").Call("toggle", "lit", f.gateOn)
		}
	}
}

// wireCounterModule builds the gate selector and trigger knobs and wires
// the Window-group switch. Called once from Run.
func (f *freqCounter) wireCounterModule() {
	f.ledEl = dom.Doc.Call("getElementById", "counter-led")
	f.gateEl = dom.Doc.Call("getElementById", "counter-gate")
	gatesel := dom.Doc.Call("getElementById", "counter-gatesel")
	trig := dom.Doc.Call("getElementById", "counter-trig")
	gstack := dom.Doc.Call("getElementById", "counter-gstack")
	tstack := dom.Doc.Call("getElementById", "counter-tstack")
	if !gatesel.Truthy() || !gstack.Truthy() {
		return
	}
	gstack.Call("appendChild", singleSelectorKnob(gatesel, []string{"0.1", "0.5", "1", "2"}))
	// The gate had no reset, was not restored by Reset All and was not in the
	// permalink: turn it and there was no way back but a page reload.
	adoptDescControl(ControlDesc{
		ID: "counter-gatesel", Label: "gate", IsSelect: true, SelectDef: "1", PermaKey: "cq",
		ResetID: "rst-counter-gate",
	})
	tstack.Call("appendChild", makeKnob(trig, js.Undefined(), true, false, true))
	// The LED, its typed entry, the wheel nudge and the reset all come from the
	// descriptor. LEDStep 10 keeps it reading whole percent, which is what it
	// read before: led.Decimals works from step × fineRatio, so a step of 1 asks
	// for a decimal this value never has.
	adoptDescControl(ControlDesc{
		ID: "counter-trig", Label: "trig", Min: 0, Max: 30, Step: 1, Def: 4,
		LEDID: "counter-trig-led", ResetID: "rst-counter-trig", LEDStep: 10,
	})
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	// The audio source is NOT started here. It used to be, because the flip
	// was a user gesture and the mic prompt belonged on it; a boot is not a
	// gesture, and a page that asks for the microphone before anybody has
	// touched anything is the thing that rule existed to avoid. The counter
	// reads whatever source is already running and shows nothing until one is.
	f.on = true
	f.cycles, f.samples, f.state = 0, 0, 0
}
