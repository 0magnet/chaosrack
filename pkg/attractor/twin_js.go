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
// on screen with the rate at which they are coming apart. lyapLiveShow writes
// it.

import (
	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

var (
	twinOn       bool
	twinSeeded   string     // mode the visible pair was seeded for
	twinA        [4]float64 // visible reference trajectory
	twinB        [4]float64 // visible perturbed trajectory
	twinBuf      []float32  // trajectory B's vertex scratch (vertBuf holds A)
	twinLambdaEl js.Value   // the λ LED in the Trace row
)

// twinD0 is the visible pair's initial separation, and it is deliberately the
// probe's d0 rather than a second constant that happens to match: the picture
// and the number are of the same thing, so the ε the eye watches grow is the ε
// the exponent is measured against.
const twinD0 = analysis.LiveD0

func twinInvalidate() { twinSeeded = "" }

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

func twinSeed(mode string, sys dynamics.FlowSys4) {
	ic := dynamics.InitCondFor(mode)
	twinA = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W()}
	twinB = twinA
	twinB[0] += twinD0
	twinSeeded = mode
}

// twinTick draws both trajectories. Returns false when the normal scan
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
func twinTick(mode string) bool {
	lyapLiveTick(mode)
	if !twinOn {
		return false
	}
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return false
	}
	if twinSeeded != mode {
		twinSeed(mode, sys)
	}
	budget := frameBudgetCompiled
	if sys.Interpreted {
		budget = frameBudgetInterpreted
	}
	// Two visible trajectories + the λ probe pair share the frame budget.
	sub := effSubSteps(speedSteps, steps, budget/2)
	dt := sys.Dt() * float64(speedScale)
	scale := sys.Scale
	invN := float32(1) / float32(steps-1)

	if len(twinBuf) < steps*4 {
		twinBuf = make([]float32, cap(vertBuf))
	}
	trace := func(s *[4]float64, out []float32) {
		for i := 0; i < steps; i++ {
			for k := 0; k < sub; k++ {
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
	vertices := vertBuf[:steps*4]
	trace(&twinA, vertices)
	trace(&twinB, twinBuf[:steps*4])

	// Keep the app-wide integrator state following trajectory A so the
	// permalink, Model Out SCAN and a later twin-off continue seamlessly.
	x, y, z = float32(twinA[0]), float32(twinA[1]), float32(twinA[2])
	x64, y64, z64 = twinA[0], twinA[1], twinA[2]
	sys.SetW(twinA[3])

	// Draw A with the normal gradient, then B in a fixed contrast color via
	// the monochrome override (restored right after).
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
	glctx.GL.Call("uniform1i", uGradientColorsLoc, 1)
	glctx.GL.Call("uniform3f", uBaseColorLoc, 0.15, 1.0, 0.45)
	uploadVerticesOnly(twinBuf[:steps*4], attractorDrawMode, steps)
	glctx.GL.Call("uniform1i", uGradientColorsLoc, gradientColorsUniform())

	return true
}

// wireTwinSwitch hooks up the Trace > Twin checkbox and the λ LED beside it.
func wireTwinSwitch() {
	twinLambdaEl = dom.Doc.Call("getElementById", "twin-lambda")
	wireSwitch("twin-sw", func(on bool) {
		twinOn = on
		twinInvalidate()
		// The switch does NOT restart the measurement — the exponent belongs
		// to the system and the system has not changed. Only the LED's
		// last-written text is cleared, so the next frame writes the current
		// reading into it (or blanks it) instead of skipping it as unchanged.
		lyapLiveTrace = "\x00"
	})
}
