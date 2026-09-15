//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// The Wow & Flutter module — speed stability off a test tone.
//
// The analysis is in wowflutter.go, untagged and checked against frequency-
// modulated signals whose deviation is known by construction. This is the panel
// around it.
//
// ── IT NEEDS SECONDS, NOT MILLISECONDS ───────────────────────────────────
//
// The slowest thing being measured is 0.5 Hz, which is two seconds a cycle, and
// a wow figure taken over less than several cycles is a figure made of one
// lurch. So this keeps a rolling buffer of the last wfWindowSec seconds and
// re-measures a few times a second over the whole of it — unlike the distortion
// analyzer, whose window is a fifth of a second and which can afford to look at
// the newest one.

const (
	// wfWindowSec is how much audio each measurement is made over. Ten seconds
	// holds five cycles of the slowest wow and is about as long as anybody
	// wants to wait for a reading to settle after adjusting a deck.
	wfWindowSec = 10

	// wfPeriodMs is how often it is remeasured. The analysis walks ten seconds
	// of audio, so this is the one measurement here that is genuinely worth
	// pacing.
	wfPeriodMs = 500
)

var (
	wfCursor  = tapUnjoined
	wfBuf     []float32
	wfFill    int
	wfNextMs  float64
	wfRes     WowFlutterResult
	wfNominal float32 = wfCarrier

	wfSpeedEl, wfWowEl, wfFlutEl js.Value
	wfWeightedEl, wfCarrierEl    js.Value
)

// wfTick keeps the rolling buffer full and remeasures on its own clock.
func wfTick(nowMs float64) {
	mod := doc.Call("getElementById", "wf-module")
	if !mod.Truthy() || !mod.Get("offsetParent").Truthy() {
		return
	}
	sr := takensSourceRate()
	want := sr * wfWindowSec
	if len(wfBuf) != want {
		wfBuf = make([]float32, want)
		wfFill = 0
	}
	var scratch [4096]float32
	for {
		n := tapRead(&wfCursor, scratch[:])
		if n <= 0 {
			break
		}
		if n >= len(wfBuf) {
			copy(wfBuf, scratch[n-len(wfBuf):n])
			wfFill = len(wfBuf)
		} else {
			copy(wfBuf, wfBuf[n:])
			copy(wfBuf[len(wfBuf)-n:], scratch[:n])
			if wfFill += n; wfFill > len(wfBuf) {
				wfFill = len(wfBuf)
			}
		}
		if n < len(scratch) {
			break
		}
	}
	if nowMs < wfNextMs {
		return
	}
	wfNextMs = nowMs + wfPeriodMs
	// Measured over whatever has arrived rather than waiting for the whole ten
	// seconds: a partial buffer gives a usable flutter figure long before it
	// gives a usable wow one, and AnalyzeWowFlutter refuses anything too short
	// to mean something.
	if wfFill < len(wfBuf) {
		wfRes = AnalyzeWowFlutter(wfBuf[len(wfBuf)-wfFill:], sr, float64(wfNominal))
	} else {
		wfRes = AnalyzeWowFlutter(wfBuf, sr, float64(wfNominal))
	}
	showWowFlutter()
}

// showWowFlutter writes the readouts.
func showWowFlutter() {
	set := func(el js.Value, v float64, signed bool) {
		if !el.Truthy() {
			return
		}
		if !wfRes.OK {
			el.Set("textContent", "  --.---")
			return
		}
		el.Set("textContent", formatLED(v, 2, 3, signed))
	}
	set(wfSpeedEl, wfRes.SpeedPct, true)
	set(wfWowEl, wfRes.WowPct, false)
	set(wfFlutEl, wfRes.FlutterPct, false)
	set(wfWeightedEl, wfRes.WeightedPct, false)
	if wfCarrierEl.Truthy() {
		if wfRes.OK {
			wfCarrierEl.Set("textContent", formatLED(wfRes.Carrier, 5, 1, false))
		} else {
			wfCarrierEl.Set("textContent", "-----.-")
		}
	}
}

// wireWowFlutterModule finds the readouts and wires the nominal knob.
func wireWowFlutterModule() {
	wfSpeedEl = doc.Call("getElementById", "wf-speed-led")
	wfWowEl = doc.Call("getElementById", "wf-wow-led")
	wfFlutEl = doc.Call("getElementById", "wf-flutter-led")
	wfWeightedEl = doc.Call("getElementById", "wf-weighted-led")
	wfCarrierEl = doc.Call("getElementById", "wf-carrier-led")
	nom := doc.Call("getElementById", "wf-nom")
	nstack := doc.Call("getElementById", "wf-nstack")
	if !nom.Truthy() {
		return
	}
	if nstack.Truthy() {
		nstack.Call("appendChild", makeKnob(nom, js.Undefined(), true, false, true))
	}
	// Step 10 already gives whole hertz through ledDecimals, so no LEDStep is
	// needed here. The LED gains the zero padding every other readout in the
	// rack has — it was the one built with strconv.Itoa rather than formatLED,
	// so 3150 showed unpadded where 03150 is the house style.
	adoptDescControl(ControlDesc{
		ID: "wf-nom", Label: "nom", Min: 0, Max: 20000, Step: 10, Def: 3150,
		LEDID: "wf-nom-led", ResetID: "rst-wf-nom",
		Apply: func(v float64) {
			wfNominal = float32(v)
			if lbl := doc.Call("getElementById", "wf-nom-lbl"); lbl.Truthy() {
				lbl.Set("textContent", formatLED(v, 5, 1, false))
			}
		},
	})
	showWowFlutter()
}
