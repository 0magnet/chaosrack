//go:build js && wasm

package attractor

// The live λ readout: a dedicated probe pair, advanced a slice at a time out
// of the render loop, feeding the accumulator in lyaplive.go.
//
// The measurement was already on screen as a PICTURE — Trace > Twin draws two
// copies of the flow ε apart and lets you watch them come apart — and a
// picture of divergence is not a rate. What this adds is the number, and the
// number is what distinguishes an attractor from a closed loop that merely
// looks complicated at 60 frames a second.
//
// The probe is deliberately NOT the visible pair. The visible pair is left to
// separate as far as it likes, because that is what makes the picture; a pair
// yanked back to ε every time unit would draw two trajectories that stay
// together and show nothing. The probe is the opposite: renormalized on a
// fixed schedule so it never leaves the linear regime, where the log of the
// separation is a rate rather than a report of the attractor's diameter.
//
// It also runs whether or not the Twin switch is on. λ is a property of the
// system, not of a drawing choice, and hanging the measurement off the switch
// meant the panel could only tell you how chaotic the model was while you were
// also asking it to draw two of them.

import (
	"math"
	"strconv"
	"syscall/js"
)

// How many probe sub-steps a frame pays for. The interpreted (equation-engine)
// systems get fewer for the reason twinTick splits its budget the same way: an
// AST walk is about ten times a compiled derivative, and the probe must not be
// what makes Custom mode stutter. The consequence is only that the readout
// settles later on those systems, never that it settles somewhere else.
const (
	lyapLiveProbeCompiled    = 1024
	lyapLiveProbeInterpreted = 256
)

var (
	lyapLiveState liveLyapunov
	lyapLiveA     [4]float64 // probe reference
	lyapLiveB     [4]float64 // probe copy, held d0 away
	lyapLiveMode  string     // the mode the pair belongs to; "" = unseeded

	lyapLiveEl    js.Value // the readout cell in the parameter grid
	lyapLiveText  string   // last text written to it
	lyapLiveTrace string   // last text written to the Trace row's LED
)

// lyapLiveSystem answers whether a mode has a continuous flow to measure, and
// hands back the system if so.
//
// The class test is not redundant with flowFor4. flowFor4 also answers from
// integrate3D's per-frame capture, so any mode that has ever reached that loop
// can name itself as a flow afterwards — the same trap the bifurcation and
// Poincaré source tracking has to step around by name. Going through modeInfo
// first means the answer is the mode's declared identity and not a side effect
// of what was on screen a moment ago.
//
// Maps are excluded on purpose rather than by accident: a map has no dt, so an
// exponent per unit time is not a smaller version of the right answer, it is a
// category error. LyapunovForMap measures those per ITERATE, and the Analysis
// module is where that is reported. Geometry and the audio displays have no
// dynamics at all.
func lyapLiveSystem(mode string) (flowSys4, bool) {
	switch modeInfo[mode].Class {
	case ClassFlow3D, ClassFlow4D:
	default:
		return flowSys4{}, false
	}
	return flowFor4(mode)
}

// lyapLiveInvalidate restarts the measurement, because the system it belongs
// to has changed. An exponent is a property of a set of coefficients; carrying
// an average across a knob edit would report a system that is no longer
// running, and would keep reporting it for as long as the old samples
// outweighed the new ones.
func lyapLiveInvalidate() { lyapLiveMode = "" }

func lyapLiveSeed(mode string, sys flowSys4) {
	ic := initCondFor(mode)
	// w0, the on-attractor seed, not sys.w(), which is wherever the renderer
	// has got to. LyapunovForFlow4 makes the same distinction for the same
	// reason: a fresh trajectory started from the running w is started from a
	// state that belongs to a different trajectory.
	lyapLiveA = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w0}
	lyapLiveB = lyapLiveA
	lyapLiveB[0] += lyapLiveD0
	lyapLiveState.reset()
	lyapLiveMode = mode
}

// lyapLiveTick advances the probe by one frame's slice and refreshes the
// readout. Called from twinTick, which generateForMode reaches every frame for
// every mode that has a trajectory at all — the modes it does not reach are
// the spectrogram surfaces, the recurrence plot and the audio scopes, which
// are exactly the modes with no exponent to measure.
func lyapLiveTick(mode string) {
	sys, ok := lyapLiveSystem(mode)
	if !ok {
		lyapLiveMode = ""
		lyapLiveShow("")
		return
	}
	if lyapLiveMode != mode {
		lyapLiveSeed(mode, sys)
	}
	// The dt the app is ACTUALLY running: the mode's own knob times the Speed
	// scale. Both belong in it — see lyaplive.go on why the exponent depends
	// on dt rather than merely being reached sooner or later because of it.
	dt := sys.dt() * float64(speedScale)
	if dt <= 0 {
		lyapLiveShow(lyapLiveReadout())
		return
	}
	// The same integrator the mode's own render loop uses, chosen the same way
	// the Poincaré section chooses it. Measuring an RK4 system with Euler at
	// its own timestep measures a different system, and lyapunov.go mislabeled
	// a third of the Sprott catalog before it learned that.
	step := sectAdvancer(mode, sys, dt)
	n := lyapLiveProbeCompiled
	if sys.interpreted {
		n = lyapLiveProbeInterpreted
	}
	for i := 0; i < n; i++ {
		step(&lyapLiveA)
		step(&lyapLiveB)
		if twinDiverged(lyapLiveA) || twinDiverged(lyapLiveB) {
			// Reseed AND restart the average. What has been accumulated
			// belongs to a trajectory that left the attractor, and an
			// over-modulated system should read "no answer yet" rather than
			// keep quoting the last one it managed to blow up from.
			lyapLiveSeed(mode, sys)
			break
		}
		var d2 float64
		for k := 0; k < 4; k++ {
			e := lyapLiveB[k] - lyapLiveA[k]
			d2 += e * e
		}
		sc, renormed := lyapLiveState.advance(dt, math.Sqrt(d2))
		if renormed {
			for k := 0; k < 4; k++ {
				lyapLiveB[k] = lyapLiveA[k] + (lyapLiveB[k]-lyapLiveA[k])*sc
			}
		}
	}
	lyapLiveShow(lyapLiveReadout())
}

// lyapLiveReadout is the value text, WITHOUT a λ on the front — the panel cell
// has a λ label of its own and read "λλ+0.96" the first time this was put in
// front of a browser. The Trace row's LED has no label, so lyapLiveShow puts
// the symbol back for that one.
//
// Two decimals, not the Analysis module's four. They are different readouts:
// that one runs a few hundred thousand steps on demand and can stand behind
// its fourth decimal, this one averages over a window three hundred times
// shorter and can stand behind its second — which is what lyapLiveMinTime is
// calibrated to. Printing four here would be printing two digits of noise.
func lyapLiveReadout() string {
	lam, ok := lyapLiveState.lambda()
	if !ok {
		// Not "+0.00". Below the threshold the average is mostly the approach
		// onto the attractor, and a small number there reads as "periodic" —
		// the one verdict this readout exists to distinguish from "chaotic".
		// A dash says the measurement is not ready; a number would say the
		// system is a limit cycle. It is the idiom the Stereo mode's "r --"
		// already uses for exactly this situation.
		return " --"
	}
	s := strconv.FormatFloat(lam, 'f', 2, 64)
	if lam >= 0 {
		s = "+" + s
	}
	return s
}

// lyapLiveShow writes the text to both places it appears, and only when it has
// changed — the exponent drifts in the third decimal every frame, the DOM does
// not need to hear about that, and a cell that re-renders sixty times a second
// is unreadable anyway. It is the rule showStereoReadout keeps.
func lyapLiveShow(s string) {
	if s != lyapLiveText {
		lyapLiveText = s
		if lyapLiveEl.Truthy() {
			lyapLiveEl.Set("textContent", s)
		}
	}
	// The Trace row's LED belongs to the Twin switch and says nothing while
	// the switch is off: it annotates the two trajectories on screen with the
	// rate at which they are coming apart, and with no trajectories drawn
	// there is nothing there for it to annotate. The panel cell is the one
	// that is always right. It carries the λ itself, having no label beside
	// it to say what the number is.
	t := ""
	if twinOn && s != "" {
		t = "λ" + s
	}
	if t != lyapLiveTrace {
		lyapLiveTrace = t
		if twinLambdaEl.Truthy() {
			twinLambdaEl.Set("textContent", t)
		}
	}
}

// appendLyapunovReadout adds the λ cell to a flow mode's parameter grid. Into
// the grid and not #params, for the reason appendStereoReadout and
// appendTakensEstimate are: #params stacks below the height-bounded grid and
// gets clipped by the module's fixed height.
func appendLyapunovReadout(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", true))
	lbl.Set("textContent", "λ")
	card.Call("appendChild", lbl)

	lyapLiveEl = doc.Call("createElement", "span")
	lyapLiveEl.Set("className", "led counter-led")
	lyapLiveEl.Set("title", "Largest Lyapunov exponent, measured live from a pair of trajectories started "+
		"a hair apart: how fast two nearby states of THIS system, at these coefficients, separate. "+
		"Positive is chaos — prediction has a horizon of roughly 1/λ — and the bigger it is the shorter "+
		"that horizon. About zero is a limit cycle or a torus. Negative is settling to a fixed point. "+
		"Per unit of MODEL time, not per second: it does not change when the browser is busy, and it "+
		"does change with dt and with Speed, because the thing being integrated changes with them. "+
		"\"λ --\" means not enough model time has been averaged yet for the number to mean anything; "+
		"it clears itself after a second or so. Analysis → lyap is the same quantity measured at "+
		"length on demand, to four decimals.")
	// Cleared so the next frame writes into the NEW element: the panel is
	// rebuilt on every mode change and every module toggle, and the
	// write-on-change guard would otherwise skip the fresh cell as unchanged
	// and leave it empty until the value happened to move.
	lyapLiveText = ""
	lyapLiveEl.Set("textContent", lyapLiveReadout())
	card.Call("appendChild", lyapLiveEl)

	grid.Call("appendChild", card)
}
