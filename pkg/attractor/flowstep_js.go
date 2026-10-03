//go:build js && wasm

package attractor

// The generic flow stepper the Grid's start sweep, the Poincaré section, the
// bifurcation explorer and the live λ probe all integrate with, so that what
// they show differs only by the dynamics, never by the integrator.

import "github.com/0magnet/chaosrack/pkg/dynamics"

// flowStep advances one state by a single Euler sub-step.
func flowStep(sys dynamics.FlowSys4, s *[4]float64, dt float64) {
	dx, dy, dz, dw := sys.F(s[0], s[1], s[2], s[3])
	s[0] += dt * dx
	s[1] += dt * dy
	s[2] += dt * dz
	s[3] += dt * dw
}

// flowDiverged reports whether a state has run off towards infinity and
// must start again.
func flowDiverged(s [4]float64) bool {
	const lim = 1e4
	return !(s[0] > -lim && s[0] < lim && s[1] > -lim && s[1] < lim &&
		s[2] > -lim && s[2] < lim && s[3] > -lim && s[3] < lim)
}
