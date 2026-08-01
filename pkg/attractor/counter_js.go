//go:build js && wasm

package attractor

import (
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

var (
	counterOn      bool
	counterBuf     []float32
	counterCycles  int
	counterSamples int
	counterState   int // +1 above trigger, −1 below, 0 unarmed
	counterLEDEl   js.Value
	counterGateEl  js.Value
	counterGateOn  bool
)

// counterTick runs every frame from the render loop. It drains the shared
// source into the cycle counter and latches the readout each time the gate
// window's worth of samples has been counted.
func counterTick() {
	if !counterOn {
		return
	}
	src := ensureAudioSource()
	if src == nil || !src.Ready() || src.SampleRate() <= 0 {
		return
	}
	if counterBuf == nil {
		counterBuf = make([]float32, 16384)
	}
	n := src.Drain(counterBuf)
	th := fgFloat(doc.Call("getElementById", "counter-trig")) / 100
	if th < 1e-4 { // trig at 0 still needs hysteresis or it chatters
		th = 1e-4
	}
	for i := 0; i < n; i++ {
		s := float64(counterBuf[i])
		switch {
		case s >= th:
			if counterState < 0 {
				counterCycles++ // one full swing = one cycle
			}
			counterState = 1
		case s <= -th:
			if counterState == 0 {
				counterState = -1 // arm on first low excursion
			} else if counterState > 0 {
				counterState = -1
			}
		}
	}
	counterSamples += n
	gate, _ := strconv.ParseFloat(doc.Call("getElementById", "counter-gatesel").Get("value").String(), 64)
	if gate <= 0 {
		gate = 1
	}
	if counterSamples >= int(gate*float64(src.SampleRate())) {
		// Latch: cycles over the ACTUAL window (sample-exact, not wall time).
		hz := float64(counterCycles) / (float64(counterSamples) / float64(src.SampleRate()))
		if counterLEDEl.Truthy() {
			counterLEDEl.Set("textContent", formatLED(hz, 5, 1, false))
		}
		counterCycles, counterSamples = 0, 0
		// The gate lamp toggles at each latch — the classic heartbeat.
		counterGateOn = !counterGateOn
		if counterGateEl.Truthy() {
			counterGateEl.Get("classList").Call("toggle", "lit", counterGateOn)
		}
	}
}

// wireCounterModule builds the gate selector and trigger knobs and wires
// the Window-group switch. Called once from Run.
func wireCounterModule() {
	counterLEDEl = doc.Call("getElementById", "counter-led")
	counterGateEl = doc.Call("getElementById", "counter-gate")
	gatesel := doc.Call("getElementById", "counter-gatesel")
	trig := doc.Call("getElementById", "counter-trig")
	trigLED := doc.Call("getElementById", "counter-trig-led")
	gstack := doc.Call("getElementById", "counter-gstack")
	tstack := doc.Call("getElementById", "counter-tstack")
	sw := doc.Call("getElementById", "counter-on")
	if !gatesel.Truthy() || !gstack.Truthy() {
		return
	}
	gstack.Call("appendChild", singleSelectorKnob(gatesel, []string{"0.1", "0.5", "1", "2"}, 50))
	trigLED.Set("value", formatLED(fgFloat(trig), intDigits(30), 0, false))
	sizeLEDField(trigLED, 0, 30, 0, false)
	tstack.Call("appendChild", makeKnob(trig, js.Undefined(), true, false, true))
	trig.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		trigLED.Set("value", formatLED(fgFloat(trig), intDigits(30), 0, false))
		return nil
	}))
	trigLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(trigLED.Get("value").String(), 64); err == nil {
			trig.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			trig.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))
	if sw.Truthy() {
		sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			counterOn = sw.Get("checked").Bool()
			if sect := doc.Call("getElementById", "counter-module"); sect.Truthy() {
				if counterOn {
					sect.Get("style").Set("display", "")
					// We're inside a user gesture: start the source now so the
					// mic prompt / ws connect happens on the flip, not the
					// first gate.
					ensureAudioSource()
					counterCycles, counterSamples, counterState = 0, 0, 0
				} else {
					sect.Get("style").Set("display", "none")
				}
			}
			quantizeModuleWidths()
			return nil
		}))
	}
}
