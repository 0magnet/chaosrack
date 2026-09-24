//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The Distortion module — THD, THD+N, SINAD and ENOB of the live audio.
//
// The analysis is in distortion.go, untagged and tested against signals whose
// distortion is known by construction. This is the panel around it: where the
// samples come from, how often the measurement runs, and how the four numbers
// are written.
//
// ── IT DOES NOT RUN PER FRAME ────────────────────────────────────────────
//
// A 16384-point FFT is not free, and four numbers a person reads a few times a
// second do not need sixty measurements a second to arrive. It runs on the same
// clock the RQA readout uses and for the same reason — and there is a second,
// better reason here: a distortion figure that flickers through three digits is
// unreadable even when every digit is right. The measurement is an average over
// its window; showing it more often than the window is long is showing the same
// audio twice.
//
// It also does nothing at all while the module is not on screen. offsetParent
// is null for a module the rack has put away, which is the test the record
// preview and the desk monitor already use.

const (
	// thdWindow is the analysis length. 16384 samples is 341 ms at 48 kHz and a
	// 2.9 Hz bin — fine enough to separate a harmonic from the noise beside it,
	// and long enough that the noise floor has settled. It is fixed rather than
	// a knob because every number the module reports depends on it, and a
	// distortion figure whose bandwidth moves under you is not comparable with
	// anything, including itself a moment ago.
	thdWindow = 16384
)

// distortion is the Distortion module: its window of audio, the last result
// and its LEDs.
type distortion struct {
	// periodMs is how often the measurement runs and is shown — for this module
	// those are one number, since showing it more often than the window is long is
	// showing the same audio twice. On the panel as RATE; see meterswitch_js.go.
	//
	// The WINDOW above stays fixed and is deliberately not beside it. Both would
	// be a switch on a real analyzer, but the argument written against it holds:
	// a distortion figure whose bandwidth moves under you is not comparable with
	// itself a moment ago, and RATE does not have that problem — it changes how
	// often you are told, not what you are told.
	periodMs                        float64
	cursor                          int
	win                             meters.SlidingWindow // the newest thdWindow samples
	buf                             []float32            // thd.win laid out in order, for the analyzer
	nextMs                          float64
	res                             meters.DistortionResult
	led, thdnLED, sinadLED, enobLED js.Value
	fundLED, levelLED               js.Value
	chanSel                         js.Value
	harmF                           float32
}

var thd = distortion{
	periodMs: 400,
	cursor:   tapUnjoined,
	harmF:    10,
}

// tick accumulates audio and runs the measurement when its period is up.
// Called once a frame from the render loop, and returns immediately on all but
// a few of those calls.
func (d *distortion) tick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !onScreen.moduleOnScreen("thd-module") {
		return
	}
	d.win.Resize(thdWindow)
	if d.buf == nil {
		d.buf = make([]float32, thdWindow)
	}
	// Drain into the window, oldest first, keeping the newest thdWindow
	// samples. The tap hands each sample over once, so this cannot double-count
	// and cannot miss any that arrived while another consumer was reading.
	ch := tapMix
	if d.chanSel.Truthy() {
		if i, err := strconv.Atoi(d.chanSel.Get("value").String()); err == nil {
			ch = tapChanSel(float32(i))
		}
	}
	var scratch [4096]float32
	for {
		n := tap.readChan(&d.cursor, scratch[:], ch)
		if n <= 0 {
			break
		}
		d.win.Push(scratch[:n])
		if n < len(scratch) {
			break
		}
	}
	if !d.win.Full() || nowMs < d.nextMs {
		return
	}
	d.nextMs = nowMs + d.periodMs
	// Laid out in order HERE, on the timer, not on the frame: putting the
	// window in order is the only part that costs the window's length, and
	// it is needed four hundred milliseconds apart rather than sixty times
	// a second. See slidingwindow.go.
	d.win.Linear(d.buf)
	d.res = meters.AnalyzeDistortion(d.buf, takensSourceRate(), int(d.harmF))
	d.showDistortion()
}

// showDistortion writes the readouts. A measurement that came back OK=false is
// written as dashes rather than as a stale number: the last reading of a tone
// that is no longer playing is the most misleading thing the module could show.
func (d *distortion) showDistortion() {
	set := func(key string, el js.Value, s string) {
		owed.readouts.Set(key, el, s)
	}
	if !d.res.OK {
		set("thd-thd", d.led, "  --.---")
		set("thd-thdn", d.thdnLED, "  --.---")
		set("thd-sinad", d.sinadLED, "  --.-")
		set("thd-enob", d.enobLED, "--.--")
		set("thd-fund", d.fundLED, "-----.-")
		set("thd-level", d.levelLED, "  --.-")
		return
	}
	set("thd-thd", d.led, led.Format(meters.AsPercent(d.res.THD), 2, 3, false))
	set("thd-thdn", d.thdnLED, led.Format(meters.AsPercent(d.res.THDN), 2, 3, false))
	// A SINAD of 999 is the sentinel for "nothing but the fundamental in the
	// window", which a synthesized tone with no noise really does produce. It
	// is not a number to print — an infinite SINAD is a claim no measurement
	// can make — so it is shown as over-range.
	if d.res.SINAD >= 900 {
		set("thd-sinad", d.sinadLED, "  >99.9")
		set("thd-enob", d.enobLED, ">16.0")
	} else {
		set("thd-sinad", d.sinadLED, led.Format(d.res.SINAD, 3, 1, false))
		set("thd-enob", d.enobLED, led.Format(d.res.ENOB, 2, 2, false))
	}
	set("thd-fund", d.fundLED, led.Format(d.res.Fundamental, 5, 1, false))
	set("thd-level", d.levelLED, led.Format(20*math.Log10(math.Max(d.res.Level, 1e-9)), 3, 1, true))
}

// wireDistortionModule builds the two knobs and finds the readouts. Called once
// from Run.
func (d *distortion) wireDistortionModule() {
	d.led = dom.Doc.Call("getElementById", "thd-led")
	d.thdnLED = dom.Doc.Call("getElementById", "thdn-led")
	d.sinadLED = dom.Doc.Call("getElementById", "thd-sinad-led")
	d.enobLED = dom.Doc.Call("getElementById", "thd-enob-led")
	d.fundLED = dom.Doc.Call("getElementById", "thd-fund-led")
	d.levelLED = dom.Doc.Call("getElementById", "thd-level-led")
	d.chanSel = dom.Doc.Call("getElementById", "thd-chan")
	harm := dom.Doc.Call("getElementById", "thd-harm")
	cstack := dom.Doc.Call("getElementById", "thd-chanstack")
	hstack := dom.Doc.Call("getElementById", "thd-hstack")
	if !d.chanSel.Truthy() || !harm.Truthy() {
		return
	}
	for i, name := range tapChanNames {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", name)
		if i < len(tapChanDescs) {
			opt.Set("title", tapChanDescs[i])
		}
		d.chanSel.Call("appendChild", opt)
	}
	d.chanSel.Set("value", "0")
	cstack.Call("appendChild", singleSelectorKnob(d.chanSel, tapChanRing))

	hstack.Call("appendChild", makeKnob(harm, js.Undefined(), true, false, true))
	// LEDStep 10 to keep whole harmonics, as above.
	adoptDescControl(ControlDesc{
		ID: "thd-harm", Label: "harm", Min: 2, Max: 20, Step: 1, Def: 10,
		LEDID: "thd-harm-led", ResetID: "rst-thd-harm", LEDStep: 10,
		Apply: func(v float64) { d.harmF = float32(v) },
	})
	// A change of channel is a change of signal, so the window it was measuring
	// no longer describes what is being asked about.
	adoptDescControl(ControlDesc{
		ID: "thd-chan", Label: "src", IsSelect: true, SelectDef: "0", PermaKey: "dc",
		ResetID: "rst-thd-chan",
		SelectApply: func(string) {
			// A change of channel is a change of signal, so the window it was
			// measuring no longer describes what is being asked about.
			d.win.Reset()
			d.res = meters.DistortionResult{}
			d.showDistortion()
		},
	})
	d.showDistortion()
}
