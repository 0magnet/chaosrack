package attractor

import (
	"math"
	"testing"
)

// The defaults of every registered flow must actually be CHAOTIC — this is
// the "the attractor settled into a stable orbit" bug class, caught in the
// wild when the Sprott mode's default parameters turned out to sit in a
// periodic window (its gallery GIF collapsed onto a single closed loop).
//
// The test estimates the largest Lyapunov exponent by the two-trajectory
// renormalization method: integrate a reference and a perturbed copy with
// the mode's own dt and initial condition, renormalize the separation every
// time unit, and average ln(growth). Positive λ ⇒ chaotic; λ ≈ 0 ⇒ periodic
// (the reported failure mode); negative ⇒ sinks to a fixed point.

// lyapunov estimates the largest exponent for a registered classic system.
func lyapunov(sys flowSys, ic [3]float32, tTransient, tMeasure float64) float64 {
	dt := sys.dt()
	if dt <= 0 {
		return math.NaN()
	}
	x, y, z := float64(ic[0]), float64(ic[1]), float64(ic[2])
	step := func(x, y, z float64) (float64, float64, float64) {
		dx, dy, dz := sys.f(x, y, z)
		return x + dt*dx, y + dt*dy, z + dt*dz
	}
	// transient: settle onto the attractor
	for t := 0.0; t < tTransient; t += dt {
		x, y, z = step(x, y, z)
	}
	const d0 = 1e-4 // above float32 round-off, far below attractor scale
	px, py, pz := x+d0, y, z
	var sum float64
	var n int
	renorm := 1.0 // renormalize every 1 time unit
	for t := 0.0; t < tMeasure; {
		for tt := 0.0; tt < renorm; tt += dt {
			x, y, z = step(x, y, z)
			px, py, pz = step(px, py, pz)
			t += dt
		}
		dx, dy, dz := px-x, py-y, pz-z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return math.NaN()
		}
		sum += math.Log(d / d0)
		n++
		// pull the perturbed copy back to distance d0 along the separation
		s := d0 / d
		px, py, pz = x+dx*s, y+dy*s, z+dz*s
	}
	return sum / (float64(n) * renorm)
}

func TestClassicDefaultsAreChaotic(t *testing.T) {
	if len(flowSystems) == 0 {
		t.Fatal("flow registry is empty")
	}
	for mode, sys := range flowSystems {
		// The transient must be LONG: the original Sprott defaults ran
		// chaotically for thousands of time units before collapsing into a
		// periodic orbit (collapse horizon ≈ t=10⁴, verified numerically), so
		// a short-window estimate happily measures the doomed transient.
		lam := lyapunov(sys, initCondFor(mode), 20000, 2000)
		t.Logf("%-14s λ ≈ %+.4f", mode, lam)
		if math.IsNaN(lam) {
			t.Errorf("%s: trajectory diverged or degenerated during the Lyapunov estimate", mode)
			continue
		}
		if lam < 0.005 {
			t.Errorf("%s: largest Lyapunov exponent %.4f — defaults are NOT chaotic (periodic window or sink)", mode, lam)
		}
	}
}
