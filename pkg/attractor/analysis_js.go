//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"
)

// The Analysis module: the largest Lyapunov exponent of whatever is on screen.
//
// The app has always been able to measure this — the estimator guarded the
// mode defaults against periodic windows — but only the test suite ever saw
// the answer. Which is a strange thing for a chaos visualizer to keep to
// itself: λ is the number that says whether the thing you are looking at is
// actually chaotic, and it is the difference between an attractor and a
// closed loop that merely looks complicated.
//
// It earns its place most in Custom mode. Type a system, and instead of
// guessing from the picture whether you have found chaos, read it.
//
// Measured on DEMAND, not per frame. A run is a few hundred thousand
// integration steps — some milliseconds — which is nothing once, and a frame
// killer sixty times a second. It re-measures when the model changes or a
// parameter is edited, both of which change the answer, and does so on a
// short delay so dragging a knob does not queue a run per pixel.

var (
	lyapLEDEl    js.Value
	lyapVerdEl   js.Value
	lyapOn       bool
	lyapPending  bool   // a re-measure is scheduled
	lyapLastMode string // what the displayed number belongs to
	lyapTimer    js.Value
)

// analysisModuleVisible shows or hides the module.
func analysisModuleVisible(on bool) {
	if sect := doc.Call("getElementById", "analysis-module"); sect.Truthy() {
		if on {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
}

func wireAnalysisModule() {
	lyapLEDEl = doc.Call("getElementById", "lyap-led")
	lyapVerdEl = doc.Call("getElementById", "lyap-verdict")
	sw := doc.Call("getElementById", "analysis-on")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		lyapOn = sw.Get("checked").Bool()
		analysisModuleVisible(lyapOn)
		if lyapOn {
			scheduleLyapunov(0)
		}
		return nil
	}))
	if btn := doc.Call("getElementById", "lyap-remeasure"); btn.Truthy() {
		btn.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			scheduleLyapunov(0)
			return nil
		}))
	}
}

// scheduleLyapunov debounces the measurement. delayMs of 0 still defers to a
// timer so the click that asked for it can finish painting first: the run
// blocks the single wasm thread, and a button that appears to hang while it
// works looks broken even when it is only busy.
func scheduleLyapunov(delayMs int) {
	if !lyapOn {
		return
	}
	if lyapPending && lyapTimer.Truthy() {
		js.Global().Call("clearTimeout", lyapTimer)
	}
	lyapPending = true
	showLyapunov("measuring…", "", false)
	if delayMs < 30 {
		delayMs = 30
	}
	lyapTimer = js.Global().Call("setTimeout", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		lyapPending = false
		runLyapunov()
		return nil
	}), delayMs)
}

// runLyapunov measures the current mode and paints the result.
func runLyapunov() {
	mode := selectedMode
	lyapLastMode = mode
	r := LyapunovFor(mode)
	if r.Verdict == "n/a" {
		// Not a dynamical system. Saying so is the honest readout; printing
		// 0.0000 beside a dodecahedron would be a category error with a
		// decimal point.
		showLyapunov("  --.--", "no dynamics", false)
		return
	}
	if !r.OK {
		showLyapunov("  --.--", r.Verdict, false)
		return
	}
	unit := "/t"
	if r.PerStep {
		unit = "/n" // per iterate: a map has no dt, so per-time is meaningless
	}
	showLyapunov(formatLyap(r.Lambda)+unit, r.Verdict, r.Verdict == "chaotic")
}

// formatLyap renders the exponent at a fixed width so the readout does not
// jitter as the value changes sign or crosses a decade.
func formatLyap(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "  --.--"
	}
	s := strconv.FormatFloat(v, 'f', 4, 64)
	if v >= 0 {
		s = "+" + s
	}
	return s
}

func showLyapunov(val, verdict string, chaotic bool) {
	if lyapLEDEl.Truthy() {
		lyapLEDEl.Set("textContent", val)
	}
	if lyapVerdEl.Truthy() {
		lyapVerdEl.Set("textContent", verdict)
		cl := lyapVerdEl.Get("classList")
		if cl.Truthy() {
			if chaotic {
				cl.Call("add", "lyap-chaotic")
			} else {
				cl.Call("remove", "lyap-chaotic")
			}
		}
	}
}

// syncAnalysisModule runs on every panel rebuild: a mode change or a
// parameter edit both invalidate the displayed number, because both change
// the system being measured.
func syncAnalysisModule(mode string) {
	if !lyapOn {
		return
	}
	if mode != lyapLastMode {
		scheduleLyapunov(120)
	}
}

// lyapInvalidate is the parameter-edit path: the exponent belongs to the
// coefficients that produced it, so an edited knob makes it stale. Debounced
// generously — a knob drag fires this continuously.
func lyapInvalidate() {
	if lyapOn {
		scheduleLyapunov(400)
	}
}
