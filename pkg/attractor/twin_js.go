//go:build js && wasm

package attractor

// Twin-trajectory divergence (the Trace > Twin switch): draw TWO copies of
// the current flow from initial conditions ε apart and watch sensitive
// dependence do its thing — the defining property of chaos, live. Trajectory
// A keeps the normal gradient; trajectory B draws in a fixed contrast color.
// Both integrate with the SAME generic stepper (via dynamics.FlowFor4), so their
// separation reflects the dynamics, never an integrator mismatch.
//
// The λ measurement that used to live in this file has moved to pkg/analysis
// and lyaplive_js.go. It is the same arithmetic — a probe pair renormalized on
// a fixed schedule while the VISIBLE pair is left alone, so the picture stays
// honest and the number stays in the linear regime — and two things changed.
//
// It runs for every flow mode now, not only while this switch is on: λ is a
// property of the system, and hanging the measurement off a drawing choice
// meant the panel could only say how chaotic the model was while you were also
// asking it to draw two of them. And it says nothing until it has averaged
// enough model time to be worth saying, which the old per-frame LED did not —
// it published its first estimate one time unit in, when the average was still
// almost entirely the approach onto the attractor.
//
// What stays here is the Trace row's LED, which annotates the two trajectories
// on screen with the rate at which they are coming apart. lyapLive.show writes
// it.

import (
	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// twinTrail is the Twin divergence trail: the two trajectories and the
// readout.
type twinTrail struct {
	on       bool
	seeded   string     // mode the visible pair was seeded for
	a        [4]float64 // visible reference trajectory
	b        [4]float64 // visible perturbed trajectory
	buf      []float32  // trajectory B's vertex scratch (vertBuf holds A)
	lambdaEl js.Value   // the TWIN P-unit's display, which reads λ while Twin is on
	eps      float64    // the visible pair's initial separation (the TWIN knob); 0 is twinD0
	turned   float64    // when the TWIN knob was last turned, ms: its display reads ε for a moment
	shown    string     // what the TWIN display was last given
}

var twin twinTrail

// twinD0 is the visible pair's initial separation until the TWIN knob sets
// another: the probe's d0, so by default the ε the eye watches grow is the ε
// the exponent is measured against. λ does not depend on it (it is a rate,
// measured by its own probe pair, lyaplive_js.go); a larger ε only brings the
// pair apart sooner, and a smaller one later.
const twinD0 = analysis.LiveD0

// d0 is the visible pair's initial separation.
func (t *twinTrail) d0() float64 {
	if t.eps > 0 {
		return t.eps
	}
	return twinD0
}

func (t *twinTrail) invalidate() { t.seeded = "" }

// twinStep advances one state by a single Euler sub-step.
func twinStep(sys dynamics.FlowSys4, s *[4]float64, dt float64) {
	dx, dy, dz, dw := sys.F(s[0], s[1], s[2], s[3])
	s[0] += dt * dx
	s[1] += dt * dy
	s[2] += dt * dz
	s[3] += dt * dw
}

func twinDiverged(s [4]float64) bool {
	const lim = 1e4
	return !(s[0] > -lim && s[0] < lim && s[1] > -lim && s[1] < lim &&
		s[2] > -lim && s[2] < lim && s[3] > -lim && s[3] < lim)
}

func (t *twinTrail) seed(mode string, sys dynamics.FlowSys4) {
	ic := dynamics.InitCondFor(mode)
	t.a = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W()}
	t.b = t.a
	t.b[0] += t.d0()
	t.seeded = mode
}

// tick draws both trajectories. Returns false when the normal scan
// generator should run instead (twin off / no flow).
//
// It is also where the live λ probe is advanced, which is not where such a
// thing belongs and is where it has to go. generateForMode reaches this call
// every frame for every mode that has a trajectory at all, BEFORE any of the
// switch-conditional branches, so it is the one per-frame hook a measurement
// can hang off without editing the render loop — and the modes it does not
// reach (the spectrogram surfaces, the recurrence plot, the audio scopes) are
// exactly the modes with no exponent to measure. The call is first, above the
// switch test, precisely so the measurement does not depend on the switch.
func (t *twinTrail) tick(mode string) bool {
	lyapLive.tick(mode)
	if !t.on {
		return false
	}
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return false
	}
	if t.seeded != mode {
		t.seed(mode, sys)
	}
	budget := frameBudgetCompiled
	if sys.Interpreted {
		budget = frameBudgetInterpreted
	}
	// Two visible trajectories + the λ probe pair share the frame budget.
	sub := effSubSteps(sim.speedSteps, sim.steps, budget/2)
	dt := sys.Dt() * float64(sim.speedScale)
	scale := sys.Scale
	invN := float32(1) / float32(sim.steps-1)

	if len(t.buf) < sim.steps*4 {
		t.buf = make([]float32, cap(sim.vertBuf))
	}
	trace := func(s *[4]float64, out []float32) {
		for i := range sim.steps {
			for range sub {
				twinStep(sys, s, dt)
				if twinDiverged(*s) {
					ic := dynamics.InitCondFor(mode)
					*s = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W()}
				}
			}
			j := i * 4
			out[j] = float32(s[0]) * scale
			out[j+1] = float32(s[1]) * scale
			out[j+2] = float32(s[2]) * scale
			out[j+3] = float32(i) * invN
		}
	}
	vertices := sim.vertBuf[:sim.steps*4]
	trace(&t.a, vertices)
	trace(&t.b, t.buf[:sim.steps*4])

	// Keep the app-wide integrator state following trajectory A so the
	// permalink, Model Out SCAN and a later twin-off continue seamlessly.
	sim.x, sim.y, sim.z = float32(t.a[0]), float32(t.a[1]), float32(t.a[2])
	sim.x64, sim.y64, sim.z64 = t.a[0], t.a[1], t.a[2]
	sys.SetW(t.a[3])

	// Draw A with the normal gradient, then B in a fixed contrast color via
	// the monochrome override (restored right after).
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, 1)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, 0.15, 1.0, 0.45)
	gpu.uploadVerticesOnly(t.buf[:sim.steps*4], gpu.drawMode, sim.steps)
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, gradientColorsUniform())

	return true
}

// wireTwinSwitch hooks up Twin, a button of the TWIN P-unit (Display).
func (t *twinTrail) wireTwinSwitch() {
	wireSwitch("twin-sw", func(on bool) {
		t.on = on
		t.invalidate()
		// The switch does NOT restart the measurement — the exponent belongs
		// to the system and the system has not changed. The display changes
		// over to it, or back to ε.
		t.showReading(lyapLive.text)
	})
}

// twinShowFor is how long, in ms, the TWIN display reads ε after its knob
// is turned.
const twinShowFor = 1500

// showReading puts λ (s, the live estimate) on the TWIN P-unit's display
// while Twin is on and its knob has not just been turned, and the knob's ε
// otherwise.
func (t *twinTrail) showReading(s string) {
	if !t.lambdaEl.Truthy() {
		t.lambdaEl = dom.Doc.Call("querySelector", "#twin-cell .dmdval")
		if !t.lambdaEl.Truthy() {
			return
		}
	}
	text := ""
	if t.on && s != "" && frameNowMs-t.turned > twinShowFor {
		text = "λ" + s
	} else if in := dom.Doc.Call("getElementById", "slider-value-teps"); in.Truthy() {
		text = readoutText(in.Get("value").String())
	}
	if text != t.shown {
		t.shown = text
		setDotText(t.lambdaEl, text)
	}
}
