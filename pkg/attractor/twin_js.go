//go:build js && wasm

package attractor

// Twin-trajectory divergence (the Trace > Twin switch): draw TWO copies of
// the current flow from initial conditions ε apart and watch sensitive
// dependence do its thing — the defining property of chaos, live. Trajectory
// A keeps the normal gradient; trajectory B draws in a fixed contrast color.
// Both integrate with the SAME generic stepper (via flowFor4), so their
// separation reflects the dynamics, never an integrator mismatch.
//
// The λ LED shows a running largest-Lyapunov-exponent estimate from a
// dedicated probe pair advanced a fixed number of steps per frame and
// renormalized every time unit (the same math as the native chaos guard) —
// the VISIBLE pair is left unrenormalized so the on-screen separation stays
// honest, while the probe keeps the measurement in the linear regime.

import (
	"fmt"
	"math"
	"syscall/js"
)

var (
	twinOn     bool
	twinSeeded string     // mode the visible pair was seeded for
	twinA      [4]float64 // visible reference trajectory
	twinB      [4]float64 // visible perturbed trajectory
	twinBuf    []float32  // trajectory B's vertex scratch (vertBuf holds A)

	// λ probe pair + renormalization accumulators.
	twinPA, twinPB   [4]float64
	twinTau          float64 // integrated time since last renorm
	twinLSum         float64
	twinLN           int
	twinLambdaEl     js.Value
	twinLambdaFrames int
)

const twinD0 = 1e-4 // initial/renormalized separation

func twinInvalidate() { twinSeeded = "" }

// twinStep advances one state by a single Euler sub-step.
func twinStep(sys flowSys4, s *[4]float64, dt float64) {
	dx, dy, dz, dw := sys.f(s[0], s[1], s[2], s[3])
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

func twinSeed(mode string, sys flowSys4) {
	ic := initCondFor(mode)
	twinA = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w()}
	twinB = twinA
	twinB[0] += twinD0
	twinPA, twinPB = twinA, twinB
	twinTau, twinLSum, twinLN = 0, 0, 0
	twinSeeded = mode
}

// twinTick draws both trajectories and updates the λ LED. Returns false when
// the normal scan generator should run instead (twin off / no flow).
func twinTick(mode string) bool {
	if !twinOn {
		return false
	}
	sys, ok := flowFor4(mode)
	if !ok {
		return false
	}
	if twinSeeded != mode {
		twinSeed(mode, sys)
	}
	budget := frameBudgetCompiled
	if sys.interpreted {
		budget = frameBudgetInterpreted
	}
	// Two visible trajectories + the probe pair share the frame budget.
	sub := effSubSteps(speedSteps, steps, budget/2)
	dt := sys.dt() * float64(speedScale)
	scale := sys.scale
	invN := float32(1) / float32(steps-1)

	if len(twinBuf) < steps*4 {
		twinBuf = make([]float32, cap(vertBuf))
	}
	trace := func(s *[4]float64, out []float32) {
		for i := 0; i < steps; i++ {
			for k := 0; k < sub; k++ {
				twinStep(sys, s, dt)
				if twinDiverged(*s) {
					ic := initCondFor(mode)
					*s = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w()}
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
	sys.setW(twinA[3])

	// Draw A with the normal gradient, then B in a fixed contrast color via
	// the monochrome override (restored right after).
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
	gl.Call("uniform1i", uGradientColorsLoc, 1)
	gl.Call("uniform3f", uBaseColorLoc, 0.15, 1.0, 0.45)
	uploadVerticesOnly(twinBuf[:steps*4], attractorDrawMode, steps)
	gl.Call("uniform1i", uGradientColorsLoc, gradientColors)

	twinProbe(sys, dt)
	return true
}

// twinProbe advances the measurement pair a fixed slice per frame with
// 1-time-unit renormalization and refreshes the λ LED.
func twinProbe(sys flowSys4, dt float64) {
	if dt <= 0 {
		return
	}
	const probeSteps = 1024
	for i := 0; i < probeSteps; i++ {
		twinStep(sys, &twinPA, dt)
		twinStep(sys, &twinPB, dt)
		twinTau += dt
		if twinDiverged(twinPA) || twinDiverged(twinPB) {
			twinPA = twinA
			twinPB = twinPA
			twinPB[0] += twinD0
			twinTau = 0
			continue
		}
		if twinTau >= 1 {
			var d2 float64
			for k := 0; k < 4; k++ {
				dd := twinPB[k] - twinPA[k]
				d2 += dd * dd
			}
			if d2 > 0 {
				d := math.Sqrt(d2)
				twinLSum += math.Log(d/twinD0) / twinTau
				twinLN++
				sc := twinD0 / d
				for k := 0; k < 4; k++ {
					twinPB[k] = twinPA[k] + (twinPB[k]-twinPA[k])*sc
				}
			}
			twinTau = 0
		}
	}
	twinLambdaFrames++
	if twinLambdaFrames%15 == 0 && twinLambdaEl.Truthy() && twinLN > 0 {
		twinLambdaEl.Set("textContent", fmt.Sprintf("λ%+.2f", twinLSum/float64(twinLN)))
	}
}

// wireTwinSwitch hooks up the Trace > Twin checkbox and the λ readout.
func wireTwinSwitch() {
	twinLambdaEl = doc.Call("getElementById", "twin-lambda")
	sw := doc.Call("getElementById", "twin-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		twinOn = sw.Get("checked").Bool()
		twinInvalidate()
		if twinLambdaEl.Truthy() {
			if twinOn {
				twinLambdaEl.Set("textContent", "λ --")
			} else {
				twinLambdaEl.Set("textContent", "")
			}
		}
		return nil
	}))
}
