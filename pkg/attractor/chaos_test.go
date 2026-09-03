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
// The estimator itself lives in lyapunov.go, not here. It used to be defined
// in this file, which meant the app had no way to reach it: the code that
// could say whether a system was chaotic could only ever tell the test suite.
// It is exported now and the panel shows it, and these guards call the SAME
// functions, so the number on screen is the number the tests trust.

func TestClassicDefaultsAreChaotic(t *testing.T) {
	if len(classicSystems) == 0 {
		t.Fatal("classic registry is empty")
	}
	// LyapunovForFlow picks Euler or RK4 per mode, from the same registry the
	// renderer uses, so this loop no longer has to know which is which. It did
	// once, and the reason is worth keeping: measuring an RK4 system with Euler
	// at its own timestep reports a system the app does not run — Sprott M came
	// out at λ≈0.0008 that way and would have been called periodic.
	for mode := range classicSystems {
		r := LyapunovForFlow(mode)
		t.Logf("%-14s λ ≈ %+.4f", mode, r.Lambda)
		if !r.OK {
			t.Errorf("%s: trajectory diverged or degenerated during the Lyapunov estimate", mode)
			continue
		}
		if r.Verdict != "chaotic" {
			t.Errorf("%s: largest Lyapunov exponent %.4f reads %q — defaults are NOT chaotic (periodic window or sink)",
				mode, r.Lambda, r.Verdict)
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
		r := LyapunovForFlow(c.key)
		t.Logf("%-14s λ ≈ %+.4f", c.key, r.Lambda)
		if !r.OK {
			t.Errorf("%s: trajectory diverged or degenerated during the Lyapunov estimate", c.key)
			continue
		}
		if r.Verdict != "chaotic" {
			t.Errorf("%s: largest Lyapunov exponent %.4f reads %q — defaults are NOT chaotic (periodic window or sink)",
				c.key, r.Lambda, r.Verdict)
		}
	}
}

// The hyper-Rössler render loop is forward Euler on the shared hyperDeriv, so
// the guard is too. Two positive exponents in the ideal system; the largest
// must survive the app's dt.
func TestHyperRosslerDefaultIsChaotic(t *testing.T) {
	r := LyapunovForFlow4("hyperrossler")
	t.Logf("hyperrossler λ ≈ %+.4f", r.Lambda)
	if !r.OK || r.Verdict != "chaotic" {
		t.Errorf("hyperrossler: largest Lyapunov exponent %.4f reads %q — defaults are NOT chaotic",
			r.Lambda, r.Verdict)
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
