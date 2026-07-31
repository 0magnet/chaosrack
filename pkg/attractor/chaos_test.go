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

// lyapunov4 is the 4-state generalization: step advances the full state by
// one dt. Used for the RK4-integrated Sprott catalog (step matches the app's
// integrate3D) and the 4D flows (Euler, matching their render loops).
func lyapunov4(step func([4]float64) [4]float64, ic [4]float64, dt, tTransient, tMeasure float64) float64 {
	if dt <= 0 {
		return math.NaN()
	}
	s := ic
	for t := 0.0; t < tTransient; t += dt {
		s = step(s)
	}
	const d0 = 1e-4
	p := s
	p[0] += d0
	var sum float64
	var n int
	const renorm = 1.0
	for t := 0.0; t < tMeasure; {
		for tt := 0.0; tt < renorm; tt += dt {
			s = step(s)
			p = step(p)
			t += dt
		}
		var d2 float64
		for i := 0; i < 4; i++ {
			d := p[i] - s[i]
			d2 += d * d
		}
		d := math.Sqrt(d2)
		if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return math.NaN()
		}
		sum += math.Log(d / d0)
		n++
		sc := d0 / d
		for i := 0; i < 4; i++ {
			p[i] = s[i] + (p[i]-s[i])*sc
		}
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

// The Sprott catalog integrates with RK4 in the app (integrate3D), so the
// guard steps RK4 too — the integrator is part of "the system the app runs".
func TestSprottCatalogDefaultsAreChaotic(t *testing.T) {
	if len(sprottCases) == 0 {
		t.Fatal("sprott catalog is empty")
	}
	for _, c := range sprottCases {
		dt := float64(c.dt)
		f := c.deriv
		step := func(s [4]float64) [4]float64 {
			x, y, z := s[0], s[1], s[2]
			k1x, k1y, k1z := f(x, y, z)
			k2x, k2y, k2z := f(x+dt/2*k1x, y+dt/2*k1y, z+dt/2*k1z)
			k3x, k3y, k3z := f(x+dt/2*k2x, y+dt/2*k2y, z+dt/2*k2z)
			k4x, k4y, k4z := f(x+dt*k3x, y+dt*k3y, z+dt*k3z)
			return [4]float64{
				x + dt/6*(k1x+2*k2x+2*k3x+k4x),
				y + dt/6*(k1y+2*k2y+2*k3y+k4y),
				z + dt/6*(k1z+2*k2z+2*k3z+k4z),
				0,
			}
		}
		ic := [4]float64{float64(c.ic[0]), float64(c.ic[1]), float64(c.ic[2]), 0}
		lam := lyapunov4(step, ic, dt, 20000, 2000)
		t.Logf("%-14s λ ≈ %+.4f", c.key, lam)
		if math.IsNaN(lam) {
			t.Errorf("%s: trajectory diverged or degenerated during the Lyapunov estimate", c.key)
			continue
		}
		if lam < 0.005 {
			t.Errorf("%s: largest Lyapunov exponent %.4f — defaults are NOT chaotic (periodic window or sink)", c.key, lam)
		}
	}
}

// The hyper-Rössler render loop is forward Euler on the shared hyperDeriv, so
// the guard is too. Two positive exponents in the ideal system; the largest
// must survive the app's dt.
func TestHyperRosslerDefaultIsChaotic(t *testing.T) {
	dt := float64(hyperDT)
	step := func(s [4]float64) [4]float64 {
		dx, dy, dz, dw := hyperDeriv(s[0], s[1], s[2], s[3])
		return [4]float64{s[0] + dt*dx, s[1] + dt*dy, s[2] + dt*dz, s[3] + dt*dw}
	}
	ic := attractorInitCond["hyperrossler"]
	lam := lyapunov4(step, [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), float64(hyperW0)}, dt, 500, 500)
	t.Logf("hyperrossler λ ≈ %+.4f", lam)
	if math.IsNaN(lam) || lam < 0.005 {
		t.Errorf("hyperrossler: largest Lyapunov exponent %.4f — defaults are NOT chaotic", lam)
	}
}

// Every "Edit equations" seed with a natively-reachable vector field must
// produce the SAME derivatives as that field — this is the transcription-
// drift guard (it caught the sprott seed still carrying the pre-fix periodic
// parameters, and aizawa/dadras seeds using a parameter named e, which the
// engine reads as Euler's constant).
func TestBuiltinEquationSeedsMatchNativeDerivs(t *testing.T) {
	// Deterministic sample points spread over the typical attractor ranges.
	pts := [][4]float64{
		{0.3, -0.7, 1.2, 0.4}, {-1.5, 0.9, -0.4, -1.1}, {2.0, 1.3, 0.8, 0.2},
		{-0.2, -1.8, 1.9, 1.5}, {1.1, 0.4, -1.6, -0.6},
	}
	evalSeed := func(be builtinEq, p [4]float64) ([4]float64, error) {
		var out [4]float64
		for i := 0; i < 4; i++ {
			if be.eq[i] == "" {
				continue
			}
			e, err := ParseExpr(be.eq[i])
			if err != nil {
				return out, err
			}
			pv := make([]float64, len(e.Params))
			for k, name := range e.Params {
				pv[k] = float64(be.params[name])
			}
			stack := make([]float64, len(e.rpn)+2)
			out[i] = e.Eval([5]float64{p[0], p[1], p[2], p[3], 0}, pv, stack)
		}
		return out, nil
	}
	approx := func(a, b, tol float64) bool {
		return math.Abs(a-b) <= tol*(1+math.Abs(a)+math.Abs(b))
	}
	checked := 0
	for mode, be := range builtinEquations {
		var native func(p [4]float64) ([4]float64, bool)
		var tol float64
		switch {
		case mode == "hyperrossler":
			native = func(p [4]float64) ([4]float64, bool) {
				dx, dy, dz, dw := hyperDeriv(p[0], p[1], p[2], p[3])
				return [4]float64{dx, dy, dz, dw}, true
			}
			tol = 1e-6 // float32 params inside hyperDeriv
		case func() bool { _, ok := sprottCaseIndex[mode]; return ok }():
			c := sprottCases[sprottCaseIndex[mode]]
			native = func(p [4]float64) ([4]float64, bool) {
				dx, dy, dz := c.deriv(p[0], p[1], p[2])
				return [4]float64{dx, dy, dz, 0}, true
			}
			tol = 1e-9 // pure float64 both sides
		default:
			sys, ok := flowSystems[mode]
			if !ok {
				continue // native deriv lives behind the js build tag — not comparable here
			}
			native = func(p [4]float64) ([4]float64, bool) {
				dx, dy, dz := sys.f(p[0], p[1], p[2])
				return [4]float64{dx, dy, dz, 0}, true
			}
			tol = 2e-3 // native path rounds through float32
		}
		checked++
		for _, p := range pts {
			want, _ := native(p)
			got, err := evalSeed(be, p)
			if err != nil {
				t.Errorf("%s: seed does not parse: %v", mode, err)
				break
			}
			for i := 0; i < 4; i++ {
				if !approx(got[i], want[i], tol) {
					t.Errorf("%s: seed deriv[%d] at %v = %g, native = %g — seed table drifted from the real system",
						mode, i, p, got[i], want[i])
				}
			}
		}
	}
	if checked < 25 {
		t.Errorf("only %d seeds were comparable — expected the classics + sprott catalog + hyperrossler", checked)
	}
	t.Logf("%d builtin-equation seeds verified against native derivatives", checked)
}

// The ring beam and Model Out FLOW consume flows through flowFor4 — assert
// the lookups that gate those features: native 4D registration (hyper), the
// w≡0 lift for 3D classics, and the display scale/hidden-state plumbing.
func TestFlowFor4Coverage(t *testing.T) {
	s, ok := flowFor4("hyperrossler")
	if !ok {
		t.Fatal("hyperrossler missing from the 4D flow registry")
	}
	if s.scale != hyperScale {
		t.Errorf("hyperrossler scale = %v, want %v", s.scale, hyperScale)
	}
	dx, dy, dz, dw := s.f(-10, -6, 0, 10)
	edx, edy, edz, edw := hyperDeriv(-10, -6, 0, 10)
	if dx != edx || dy != edy || dz != edz || dw != edw {
		t.Error("hyperrossler flowFor4 does not dispatch to hyperDeriv")
	}
	s3, ok := flowFor4("lorenz")
	if !ok {
		t.Fatal("3D classics must lift into flowFor4")
	}
	if _, _, _, dw := s3.f(1, 1, 1, 5); dw != 0 {
		t.Errorf("lifted 3D flow must hold dw≡0, got %v", dw)
	}
	if s3.scale != 1 {
		t.Errorf("lifted 3D flow scale = %v, want 1", s3.scale)
	}
	if _, ok := flowFor4("globe"); ok {
		t.Error("geometry modes must not report a flow")
	}
}
