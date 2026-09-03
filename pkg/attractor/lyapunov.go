package attractor

import "math"

// The largest Lyapunov exponent — how fast two nearby trajectories separate,
// which is the number that says whether what is on screen is actually chaotic.
//
// This lived in chaos_test.go, where it guarded the mode defaults against
// settling into periodic windows, and was never shown to anyone. It is the
// same estimator; it is only in a file the app can also reach now, so the
// measurement the test suite trusts is the measurement the panel displays.
// There is deliberately one implementation: a readout that agreed with the
// tests only by coincidence would be worse than none.
//
// Method: two-trajectory renormalization. Integrate a reference and a copy
// perturbed by d0, let them run for a fixed interval, take ln(separation/d0),
// pull the copy back to d0 along the separation, repeat, average. Positive λ
// is chaos (nearby states diverge, prediction has a horizon); λ ≈ 0 is
// periodic or quasi-periodic; negative is a sink.
//
// Untagged so both the native tests and the js build use it.

// lyapDefaultD0 is the perturbation size: above float32 round-off, far below
// the scale of any attractor here, so the separation grows in the linear
// regime rather than folding around the attractor before it is measured.
const lyapDefaultD0 = 1e-4

// LyapunovResult carries the estimate with enough context to say what it
// means. Verdict is the plain-language reading — the point of showing this at
// all is that "0.906" means nothing to most people and "chaotic" does.
type LyapunovResult struct {
	Lambda  float64 // largest exponent, per unit time (flows) or per iterate (maps)
	Verdict string  // "chaotic" | "periodic" | "converging" | "diverged"
	PerStep bool    // true when Lambda is per iterate rather than per time unit
	OK      bool    // false when the system could not be measured at all
}

// classify turns an exponent into the reading. The thresholds are not
// arbitrary: the estimator's own noise floor on a periodic orbit is around
// 1e-3 (a closed orbit's neighbors neither separate nor converge, and what is
// left is round-off), so anything inside that band is "not measurably
// exponential" rather than "exactly zero".
func classify(lam float64) string {
	switch {
	case math.IsNaN(lam) || math.IsInf(lam, 0):
		return "diverged"
	case lam > 5e-3:
		return "chaotic"
	case lam < -5e-3:
		return "converging"
	default:
		return "periodic"
	}
}

// lyapunovStep is the general estimator over an arbitrary state advance. Both
// the flow and map forms below are this function with a different step: a flow
// advances by dt, a map advances by one iterate, and the only difference to the
// arithmetic is what "per unit" means in the answer.
//
// n is how many advances make one renormalization interval, and intervals is
// how many of those to average over.
func lyapunovStep(step func([]float64) []float64, ic []float64, transient, n, intervals int) float64 {
	if n < 1 || intervals < 1 || len(ic) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), ic...)
	for i := 0; i < transient; i++ {
		s = step(s)
		if !finiteSlice(s) {
			return math.NaN()
		}
	}
	p := append([]float64(nil), s...)
	p[0] += lyapDefaultD0

	var sum float64
	var count int
	for k := 0; k < intervals; k++ {
		for i := 0; i < n; i++ {
			s = step(s)
			p = step(p)
		}
		if !finiteSlice(s) || !finiteSlice(p) {
			return math.NaN()
		}
		var d2 float64
		for i := range s {
			d := p[i] - s[i]
			d2 += d * d
		}
		d := math.Sqrt(d2)
		if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return math.NaN()
		}
		sum += math.Log(d / lyapDefaultD0)
		count++
		// Pull the copy back to d0 along the separation, keeping the direction
		// it has grown into — that direction converges on the most unstable
		// one, which is what makes this the LARGEST exponent and not just some
		// exponent.
		sc := lyapDefaultD0 / d
		for i := range p {
			p[i] = s[i] + (p[i]-s[i])*sc
		}
	}
	if count == 0 {
		return math.NaN()
	}
	return sum / float64(count)
}

func finiteSlice(v []float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

// LyapunovForMap estimates the exponent of a registered discrete map, per
// ITERATE — a map has no dt, so per-time would be meaningless.
func LyapunovForMap(key string) LyapunovResult {
	step, ic, ok := MapStep(key)
	if !ok {
		return LyapunovResult{Verdict: "unknown", PerStep: true}
	}
	adv := func(s []float64) []float64 {
		x, y, z := step(s[0], s[1], s[2])
		return []float64{x, y, z}
	}
	// One iterate per interval: for a map the natural unit IS the iterate, and
	// renormalizing every step keeps the separation firmly in the linear
	// regime, where a chaotic map would otherwise saturate across the whole
	// attractor within a few dozen steps and report a floor instead of a rate.
	lam := lyapunovStep(adv, []float64{ic[0], ic[1], ic[2]}, 20000, 1, 20000)
	return LyapunovResult{
		Lambda:  lam,
		Verdict: classify(lam),
		PerStep: true,
		OK:      !math.IsNaN(lam) && !math.IsInf(lam, 0),
	}
}

// LyapunovForFlow estimates the exponent of a registered flow, per unit time,
// integrating the way the flow registry does.
func LyapunovForFlow(key string) LyapunovResult {
	sys, ok := flowSystems[key]
	if !ok {
		return LyapunovResult{Verdict: "unknown"}
	}
	dt := sys.dt()
	if dt <= 0 {
		return LyapunovResult{Verdict: "unknown"}
	}
	// Integrate the way THE APP integrates this particular system, which is
	// not one scheme for all of them: the classics run a bespoke forward-Euler
	// render loop, everything else goes through the shared RK4 loop. Measuring
	// an RK4 system with Euler at its own timestep measures a different system
	// — Sprott M reads λ≈0.0005 that way and would be labeled "periodic" on
	// screen while visibly filling an attractor. The guard test learned this
	// and split its two cases; the readout has to make the same distinction or
	// it will confidently mislabel a third of the Sprott catalog.
	_, euler := classicSystems[key]
	adv := func(s []float64) []float64 {
		if euler {
			dx, dy, dz := sys.f(s[0], s[1], s[2])
			return []float64{s[0] + dt*dx, s[1] + dt*dy, s[2] + dt*dz}
		}
		x, y, z := rk4(sys.f, dt, s[0], s[1], s[2])
		return []float64{x, y, z}
	}
	ic := initCondFor(key)
	// One time unit per renormalization, as the guard test uses.
	n := int(1.0/dt + 0.5)
	if n < 1 {
		n = 1
	}
	lam := lyapunovStep(adv, []float64{float64(ic[0]), float64(ic[1]), float64(ic[2])},
		int(200.0/dt), n, 2000)
	return LyapunovResult{
		Lambda:  lam,
		Verdict: classify(lam),
		OK:      !math.IsNaN(lam) && !math.IsInf(lam, 0),
	}
}

// LyapunovForFlow4 handles the flows with a hidden fourth state (the
// hyper-Rössler, and the equation engine when a w term is in use). They are a
// separate registry rather than a special case, and they matter here for a
// reason beyond completeness: a HYPERchaotic system has more than one positive
// exponent, and projecting it to three dimensions would measure the wrong
// object. The seed is w0, the on-attractor value — not the running renderer's
// w, which outside a browser is zero, and the hyper-Rössler diverges from
// there.
func LyapunovForFlow4(key string) LyapunovResult {
	s, ok := flowSystems4[key]
	if !ok {
		return LyapunovResult{Verdict: "unknown"}
	}
	dt := s.dt()
	if dt <= 0 {
		return LyapunovResult{Verdict: "unknown"}
	}
	adv := func(v []float64) []float64 {
		if s.euler {
			dx, dy, dz, dw := s.f(v[0], v[1], v[2], v[3])
			return []float64{v[0] + dt*dx, v[1] + dt*dy, v[2] + dt*dz, v[3] + dt*dw}
		}
		out := rk4x4(s.f, dt, [4]float64{v[0], v[1], v[2], v[3]})
		return []float64{out[0], out[1], out[2], out[3]}
	}
	ic := initCondFor(key)
	n := int(1.0/dt + 0.5)
	if n < 1 {
		n = 1
	}
	lam := lyapunovStep(adv,
		[]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), s.w0},
		int(200.0/dt), n, 2000)
	return LyapunovResult{
		Lambda:  lam,
		Verdict: classify(lam),
		OK:      !math.IsNaN(lam) && !math.IsInf(lam, 0),
	}
}

// LyapunovFor measures whichever kind the mode is, or reports that the mode is
// not a dynamical system at all — a polyhedron has no exponent, and saying so
// is better than printing a number for it.
func LyapunovFor(mode string) LyapunovResult {
	// 4-D before 3-D: a native 4-D system is also reachable through the 3-D
	// lookup (the registry lifts and projects), and measuring the projection
	// would measure a system that does not exist.
	if _, ok := flowSystems4[mode]; ok {
		return LyapunovForFlow4(mode)
	}
	switch {
	case IsMap(mode):
		return LyapunovForMap(mode)
	case HasFlow(mode):
		return LyapunovForFlow(mode)
	}
	return LyapunovResult{Verdict: "n/a"}
}
