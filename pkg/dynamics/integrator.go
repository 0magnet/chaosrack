package dynamics

import (
	"fmt"
	"strings"
)

// Integrator is how every flow is stepped: one choice for all of them, the
// page's render loops, the native rack, the analyses and the file outputs
// alike.
//
// RK4 is the default because at these step sizes forward Euler is not an
// approximation of the equations so much as a different system. It follows
// the flow of f − (h/2)·Df·f to within O(h²), where RK4's modified field is
// within O(h⁴) of f; for the Simone attractor, whose field changes at a rate
// of about 11, Euler at dt=0.005 is a 5% change to the equations. That is
// enough to cross a periodic window: under Euler at their own steps, Wang–Sun
// and the three-scroll are chaotic, and as the step shrinks they stop being —
// RK4 says the same at every step, so it is the equations speaking and Euler's
// chaos is its own. Euler also makes the Speed knob change the system rather
// than its pace, since slowing a model shrinks its step.
//
// Euler stays a choice. It is how the page drew the classic systems from its
// beginning, it is a dynamical system worth looking at in its own right, and
// what it does to a flow is something to hear and see.
type Integrator int

const (
	// RK4Integrator is classical fourth-order Runge–Kutta, in float64.
	RK4Integrator Integrator = iota
	// EulerIntegrator is forward Euler: the classics in their own float32,
	// everything else in float64.
	EulerIntegrator
)

// IntegratorKnob is the setting, as every flow's INTG position turns it: 0
// for RK4, 1 for Euler. A knob's value like any constant's, so the panel, a
// link and the native rack move it the way they move a constant.
var IntegratorKnob float32

// SetIntegrator chooses how every flow is stepped from the next step on.
func SetIntegrator(i Integrator) { IntegratorKnob = float32(i) }

// CurrentIntegrator is the integrator in force.
func CurrentIntegrator() Integrator {
	if IntegratorKnob >= 0.5 {
		return EulerIntegrator
	}
	return RK4Integrator
}

// UsingEuler reports whether flows are being stepped by forward Euler.
func UsingEuler() bool { return CurrentIntegrator() == EulerIntegrator }

func (i Integrator) String() string {
	if i == EulerIntegrator {
		return "euler"
	}
	return "rk4"
}

// ParseIntegrator reads "rk4" or "euler", as a flag or a link carries it.
func ParseIntegrator(s string) (Integrator, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "rk4", "":
		return RK4Integrator, nil
	case "euler":
		return EulerIntegrator, nil
	}
	return RK4Integrator, fmt.Errorf("integrator %q: want rk4 or euler", s)
}

// IntegratorNames are the INTG position's settings, in knob order.
var IntegratorNames = []string{"rk4", "euler"}

// IntegratorSuffix ends the id of every flow's INTG position: "lorenz-intg".
const IntegratorSuffix = "-intg"

// integratorParam is mode's INTG position. Every flow has one, after its
// constants, and they all turn the one setting: a position of its own rather
// than one cell shared by every flow, which the bank would put first and
// move every model's constants along by one to make room for.
func integratorParam(mode string) Param {
	return Param{mode + IntegratorSuffix, "intg", &IntegratorKnob, 0, 0, 1, 1}
}
