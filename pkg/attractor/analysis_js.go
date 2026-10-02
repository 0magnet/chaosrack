//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dom"
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

// lyapunovProbe is the Analysis module's on-demand Lyapunov measurement.
type lyapunovProbe struct {
	ledEl    js.Value
	verdEl   js.Value
	on       bool
	pending  bool                               // a re-measure is scheduled
	lastMode string                             // what the displayed number belongs to
	cache    map[string]analysis.LyapunovResult // by model and parameter values (measure)
	timer    js.Value
}

var lyap lyapunovProbe

// wireAnalysisModule wires the Lyapunov readout. It was a module of its own
// and is a readout on the Visual head now, over the equation, with the model
// it measures (modelparts_js.go).
func (l *lyapunovProbe) wireAnalysisModule() {
	l.ledEl = dom.Doc.Call("getElementById", "lyap-led")
	l.verdEl = dom.Doc.Call("getElementById", "lyap-verdict")
	// Always on: there is no state in which the readout is absent. It is SET
	// rather than a setter being called: the setter was the old switch's
	// behavior — it opened an audio graph and took a context lease — and
	// booting must not do that.
	l.on = true
	l.scheduleLyapunov(0)
	if btn := dom.Doc.Call("getElementById", "lyap-remeasure"); btn.Truthy() {
		btn.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			l.scheduleLyapunov(0)
			return nil
		}))
	}
}

// scheduleLyapunov debounces the measurement. delayMs of 0 still defers to a
// timer so the click that asked for it can finish painting first: the run
// blocks the single wasm thread, and a button that appears to hang while it
// works looks broken even when it is only busy.
func (l *lyapunovProbe) scheduleLyapunov(delayMs int) {
	if !l.on {
		return
	}
	if l.pending && l.timer.Truthy() {
		js.Global().Call("clearTimeout", l.timer)
	}
	l.pending = true
	l.showLyapunov("measuring…", "", false)
	if delayMs < 30 {
		delayMs = 30
	}
	l.timer = js.Global().Call("setTimeout", dom.FuncOf(func(js.Value, []js.Value) any {
		l.pending = false
		l.runLyapunov()
		return nil
	}), delayMs)
}

// runLyapunov measures the current mode and paints the result.
func (l *lyapunovProbe) runLyapunov() {
	mode := run.selectedMode
	l.lastMode = mode
	r := l.measure(mode)
	if r.Verdict == "n/a" {
		// Not a dynamical system. Saying so is the honest readout; printing
		// 0.0000 beside a dodecahedron would be a category error with a
		// decimal point.
		l.showLyapunov("--.--", "n/a", false)
		return
	}
	if !r.OK {
		l.showLyapunov("--.--", r.Verdict, false)
		return
	}
	unit := "/t"
	if r.PerStep {
		unit = "/n" // per iterate: a map has no dt, so per-time is meaningless
	}
	l.showLyapunov(formatLyap(r.Lambda)+unit, r.Verdict, r.Verdict == "chaotic")
}

// formatLyap renders the exponent signed, to three decimals, or two past ten,
// so with its unit ("/t", "/n") it fills a full display and no more.
func formatLyap(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "--.--"
	}
	dec := 3
	if math.Abs(v) >= 10 {
		dec = 2
	}
	s := strconv.FormatFloat(v, 'f', dec, 64)
	if v >= 0 {
		s = "+" + s
	}
	return s
}

func (l *lyapunovProbe) showLyapunov(val, verdict string, chaotic bool) {
	if l.ledEl.Truthy() {
		setDotText(l.ledEl, val)
	}
	if l.verdEl.Truthy() {
		setDotText(l.verdEl, verdictText(verdict))
		cl := l.verdEl.Get("classList")
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
func (l *lyapunovProbe) syncAnalysisModule(mode string) {
	if !l.on {
		return
	}
	if mode != l.lastMode {
		l.scheduleLyapunov(120)
	}
}

// invalidate is the parameter-edit path: the exponent belongs to the
// coefficients that produced it, so an edited knob makes it stale. Debounced
// generously — a knob drag fires this continuously.
func (l *lyapunovProbe) invalidate() {
	if l.on {
		l.scheduleLyapunov(400)
	}
}

// measure is the exponent of mode at its parameters' present values. The
// answer depends on nothing else, so it is kept: going back to a model whose
// knobs have not moved costs no integration at all, where it was a run of a
// few hundred thousand steps on every model change.
func (l *lyapunovProbe) measure(mode string) analysis.LyapunovResult {
	// Not the two whose system is more than their knobs: Custom's is the
	// equations typed into it, and the morph reprograms itself as it runs.
	if mode == "custom" || mode == "sprottmorph" {
		return analysis.LyapunovFor(mode)
	}
	var key strings.Builder
	key.WriteString(mode)
	for _, p := range attractorParams[mode] {
		key.WriteString("|" + strconv.FormatFloat(float64(*p.Value), 'g', -1, 32))
	}
	if r, ok := l.cache[key.String()]; ok {
		return r
	}
	r := analysis.LyapunovFor(mode)
	if l.cache == nil {
		l.cache = map[string]analysis.LyapunovResult{}
	}
	l.cache[key.String()] = r
	return r
}

// verdictText is a verdict as its display shows it: eight characters, so
// "converging" says what the exponent says of it, that the orbit is settling.
func verdictText(v string) string {
	if v == "converging" {
		return "settling"
	}
	return v
}
