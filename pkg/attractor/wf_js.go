//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
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

// wowFlutter is the Wow & Flutter module: its window of audio, the last
// result and its LEDs.
type wowFlutter struct {
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
	windowSec              int
	periodMs               float64
	cursor                 int
	win                    meters.SlidingWindow // the newest wfWindowSec seconds
	buf                    []float32            // wfWin laid out in order, for the analyzer
	nextMs                 float64
	res                    meters.WowFlutterResult
	nominal                float32
	speedEl, wowEl, flutEl js.Value
	weightedEl, carrierEl  js.Value
}

var wow = wowFlutter{
	windowSec: 10,
	periodMs:  500,
	cursor:    tapUnjoined,
	nominal:   meters.WfCarrier,
}

// wfTick keeps the rolling buffer full and remeasures on its own clock.
func (w *wowFlutter) tick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !moduleOnScreen("wf-module") {
		return
	}
	sr := takensSourceRate()
	want := sr * w.windowSec
	w.win.Resize(want)
	if len(w.buf) != want {
		w.buf = make([]float32, want)
	}
	var scratch [4096]float32
	for {
		n := tapRead(&w.cursor, scratch[:])
		if n <= 0 {
			break
		}
		w.win.Push(scratch[:n])
		if n < len(scratch) {
			break
		}
	}
	if nowMs < w.nextMs {
		return
	}
	w.nextMs = nowMs + w.periodMs
	// Measured over whatever has arrived rather than waiting for the whole ten
	// seconds: a partial buffer gives a usable flutter figure long before it
	// gives a usable wow one, and meters.AnalyzeWowFlutter refuses anything too short
	// to mean something.
	// Laid out in order HERE, on the timer. Ten seconds at 48 kHz is 1.9 MB,
	// and sliding that on every frame to produce a reading twice a second
	// was about 115 MB/s of memmove. See slidingwindow.go.
	n := w.win.Linear(w.buf)
	w.res = meters.AnalyzeWowFlutter(w.buf[:n], sr, float64(w.nominal))
	w.showWowFlutter()
}

// showWowFlutter writes the readouts.
func (w *wowFlutter) showWowFlutter() {
	set := func(key string, el js.Value, v float64, signed bool) {
		if !w.res.OK {
			readouts.Set(key, el, "  --.---")
			return
		}
		readouts.Set(key, el, led.Format(v, 2, 3, signed))
	}
	set("wf-speed", w.speedEl, w.res.SpeedPct, true)
	set("wf-wow", w.wowEl, w.res.WowPct, false)
	set("wf-flut", w.flutEl, w.res.FlutterPct, false)
	set("wf-wtd", w.weightedEl, w.res.WeightedPct, false)
	if w.res.OK {
		readouts.Set("wf-carrier", w.carrierEl, led.Format(w.res.Carrier, 5, 1, false))
	} else {
		readouts.Set("wf-carrier", w.carrierEl, "-----.-")
	}
}

// wireWowFlutterModule finds the readouts and wires the nominal knob.
func (w *wowFlutter) wireWowFlutterModule() {
	w.speedEl = dom.Doc.Call("getElementById", "wf-speed-led")
	w.wowEl = dom.Doc.Call("getElementById", "wf-wow-led")
	w.flutEl = dom.Doc.Call("getElementById", "wf-flutter-led")
	w.weightedEl = dom.Doc.Call("getElementById", "wf-weighted-led")
	w.carrierEl = dom.Doc.Call("getElementById", "wf-carrier-led")
	nom := dom.Doc.Call("getElementById", "wf-nom")
	nstack := dom.Doc.Call("getElementById", "wf-nstack")
	if !nom.Truthy() {
		return
	}
	if nstack.Truthy() {
		nstack.Call("appendChild", makeKnob(nom, js.Undefined(), true, false, true))
	}
	// Step 10 already gives whole hertz through led.Decimals, so no LEDStep is
	// needed here. The LED gains the zero padding every other readout in the
	// rack has — it was the one built with strconv.Itoa rather than led.Format,
	// so 3150 showed unpadded where 03150 is the house style.
	adoptDescControl(ControlDesc{
		ID: "wf-nom", Label: "nom", Min: 0, Max: 20000, Step: 10, Def: 3150,
		LEDID: "wf-nom-led", ResetID: "rst-wf-nom",
		Apply: func(v float64) {
			w.nominal = float32(v)
			if lbl := dom.Doc.Call("getElementById", "wf-nom-lbl"); lbl.Truthy() {
				lbl.Set("textContent", led.Format(v, 5, 1, false))
			}
		},
	})
	w.showWowFlutter()
}
