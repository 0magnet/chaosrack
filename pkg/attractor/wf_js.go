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

// wfWindowSec is how much audio each measurement is made over, and wfPeriodMs
// is how often it is remade. Both are on the panel — WINDOW and RATE — and
// this is the module where the difference between them is easiest to see.
//
// The window is the measurement: ten seconds holds five cycles of the slowest
// wow, and two seconds cannot see wow at all, only flutter. It is also the
// cost — the analysis walks the whole window, so it is the single largest
// lump of work in the rack's frame, and shortening it is the only thing that
// makes that lump SMALLER.
//
// The rate is how often that lump lands. Turning it down does not make the
// analysis cheaper, it makes the hesitation rarer, and it is the honest
// trade: a wow-and-flutter reading that settles over ten seconds does not
// need remaking twice a second.
var (
	wfWindowSec         = 10
	wfPeriodMs  float64 = 500
)

var (
	wfCursor  = tapUnjoined
	wfWin     slidingWindow // the newest wfWindowSec seconds
	wfBuf     []float32     // wfWin laid out in order, for the analyzer
	wfNextMs  float64
	wfRes     WowFlutterResult
	wfNominal float32 = wfCarrier

	wfSpeedEl, wfWowEl, wfFlutEl js.Value
	wfWeightedEl, wfCarrierEl    js.Value
)

// wfTick keeps the rolling buffer full and remeasures on its own clock.
func wfTick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !moduleOnScreen("wf-module") {
		return
	}
	sr := takensSourceRate()
	want := sr * wfWindowSec
	wfWin.Resize(want)
	if len(wfBuf) != want {
		wfBuf = make([]float32, want)
	}
	var scratch [4096]float32
	for {
		n := tapRead(&wfCursor, scratch[:])
		if n <= 0 {
			break
		}
		wfWin.Push(scratch[:n])
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
	// Laid out in order HERE, on the timer. Ten seconds at 48 kHz is 1.9 MB,
	// and sliding that on every frame to produce a reading twice a second
	// was about 115 MB/s of memmove. See slidingwindow.go.
	n := wfWin.Linear(wfBuf)
	wfRes = AnalyzeWowFlutter(wfBuf[:n], sr, float64(wfNominal))
	showWowFlutter()
}

// showWowFlutter writes the readouts.
func showWowFlutter() {
	set := func(key string, el js.Value, v float64, signed bool) {
		if !wfRes.OK {
			setLEDText(key, el, "  --.---")
			return
		}
		setLEDText(key, el, formatLED(v, 2, 3, signed))
	}
	set("wf-speed", wfSpeedEl, wfRes.SpeedPct, true)
	set("wf-wow", wfWowEl, wfRes.WowPct, false)
	set("wf-flut", wfFlutEl, wfRes.FlutterPct, false)
	set("wf-wtd", wfWeightedEl, wfRes.WeightedPct, false)
	if wfRes.OK {
		setLEDText("wf-carrier", wfCarrierEl, formatLED(wfRes.Carrier, 5, 1, false))
	} else {
		setLEDText("wf-carrier", wfCarrierEl, "-----.-")
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
