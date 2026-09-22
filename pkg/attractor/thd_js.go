//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"
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

	// thdPeriodMs is how often the measurement runs.
	thdPeriodMs = 400
)

var (
	thdCursor = tapUnjoined
	thdWin    slidingWindow // the newest thdWindow samples
	thdBuf    []float32     // thdWin laid out in order, for the analyzer
	thdNextMs float64
	thdRes    DistortionResult

	thdLED, thdnLED, thdSinadLED, thdEnobLED js.Value
	thdFundLED, thdLevelLED                  js.Value
	thdChanSel                               js.Value
	thdHarmF                                 float32 = 10
)

// thdTick accumulates audio and runs the measurement when its period is up.
// Called once a frame from the render loop, and returns immediately on all but
// a few of those calls.
func thdTick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !moduleOnScreen("thd-module") {
		return
	}
	thdWin.Resize(thdWindow)
	if thdBuf == nil {
		thdBuf = make([]float32, thdWindow)
	}
	// Drain into the window, oldest first, keeping the newest thdWindow
	// samples. The tap hands each sample over once, so this cannot double-count
	// and cannot miss any that arrived while another consumer was reading.
	ch := tapMix
	if thdChanSel.Truthy() {
		if i, err := strconv.Atoi(thdChanSel.Get("value").String()); err == nil {
			ch = tapChanSel(float32(i))
		}
	}
	var scratch [4096]float32
	for {
		n := tapReadChan(&thdCursor, scratch[:], ch)
		if n <= 0 {
			break
		}
		thdWin.Push(scratch[:n])
		if n < len(scratch) {
			break
		}
	}
	if !thdWin.Full() || nowMs < thdNextMs {
		return
	}
	thdNextMs = nowMs + thdPeriodMs
	// Laid out in order HERE, on the timer, not on the frame: putting the
	// window in order is the only part that costs the window's length, and
	// it is needed four hundred milliseconds apart rather than sixty times
	// a second. See slidingwindow.go.
	thdWin.Linear(thdBuf)
	thdRes = AnalyzeDistortion(thdBuf, takensSourceRate(), int(thdHarmF))
	showDistortion()
}

// showDistortion writes the readouts. A measurement that came back OK=false is
// written as dashes rather than as a stale number: the last reading of a tone
// that is no longer playing is the most misleading thing the module could show.
func showDistortion() {
	set := func(key string, el js.Value, s string) {
		setLEDText(key, el, s)
	}
	if !thdRes.OK {
		set("thd-thd", thdLED, "  --.---")
		set("thd-thdn", thdnLED, "  --.---")
		set("thd-sinad", thdSinadLED, "  --.-")
		set("thd-enob", thdEnobLED, "--.--")
		set("thd-fund", thdFundLED, "-----.-")
		set("thd-level", thdLevelLED, "  --.-")
		return
	}
	set("thd-thd", thdLED, formatLED(AsPercent(thdRes.THD), 2, 3, false))
	set("thd-thdn", thdnLED, formatLED(AsPercent(thdRes.THDN), 2, 3, false))
	// A SINAD of 999 is the sentinel for "nothing but the fundamental in the
	// window", which a synthesized tone with no noise really does produce. It
	// is not a number to print — an infinite SINAD is a claim no measurement
	// can make — so it is shown as over-range.
	if thdRes.SINAD >= 900 {
		set("thd-sinad", thdSinadLED, "  >99.9")
		set("thd-enob", thdEnobLED, ">16.0")
	} else {
		set("thd-sinad", thdSinadLED, formatLED(thdRes.SINAD, 3, 1, false))
		set("thd-enob", thdEnobLED, formatLED(thdRes.ENOB, 2, 2, false))
	}
	set("thd-fund", thdFundLED, formatLED(thdRes.Fundamental, 5, 1, false))
	set("thd-level", thdLevelLED, formatLED(20*math.Log10(math.Max(thdRes.Level, 1e-9)), 3, 1, true))
}

// wireDistortionModule builds the two knobs and finds the readouts. Called once
// from Run.
func wireDistortionModule() {
	thdLED = doc.Call("getElementById", "thd-led")
	thdnLED = doc.Call("getElementById", "thdn-led")
	thdSinadLED = doc.Call("getElementById", "thd-sinad-led")
	thdEnobLED = doc.Call("getElementById", "thd-enob-led")
	thdFundLED = doc.Call("getElementById", "thd-fund-led")
	thdLevelLED = doc.Call("getElementById", "thd-level-led")
	thdChanSel = doc.Call("getElementById", "thd-chan")
	harm := doc.Call("getElementById", "thd-harm")
	cstack := doc.Call("getElementById", "thd-chanstack")
	hstack := doc.Call("getElementById", "thd-hstack")
	if !thdChanSel.Truthy() || !harm.Truthy() {
		return
	}
	for i, name := range tapChanNames {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", name)
		if i < len(tapChanDescs) {
			opt.Set("title", tapChanDescs[i])
		}
		thdChanSel.Call("appendChild", opt)
	}
	thdChanSel.Set("value", "0")
	cstack.Call("appendChild", singleSelectorKnob(thdChanSel, tapChanRing))

	hstack.Call("appendChild", makeKnob(harm, js.Undefined(), true, false, true))
	// LEDStep 10 to keep whole harmonics, as above.
	adoptDescControl(ControlDesc{
		ID: "thd-harm", Label: "harm", Min: 2, Max: 20, Step: 1, Def: 10,
		LEDID: "thd-harm-led", ResetID: "rst-thd-harm", LEDStep: 10,
		Apply: func(v float64) { thdHarmF = float32(v) },
	})
	// A change of channel is a change of signal, so the window it was measuring
	// no longer describes what is being asked about.
	adoptDescControl(ControlDesc{
		ID: "thd-chan", Label: "src", IsSelect: true, SelectDef: "0", PermaKey: "dc",
		ResetID: "rst-thd-chan",
		SelectApply: func(string) {
			// A change of channel is a change of signal, so the window it was
			// measuring no longer describes what is being asked about.
			thdWin.Reset()
			thdRes = DistortionResult{}
			showDistortion()
		},
	})
	showDistortion()
}
