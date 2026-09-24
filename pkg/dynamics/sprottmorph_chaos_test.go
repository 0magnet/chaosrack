package dynamics

import (
	"math"
	"testing"
)

// The morph is the one dynamical mode LyapunovFor cannot measure.
//
// It returns "n/a" for sprottmorph, so TestEveryDynamicalModeMeasuresChaotic
// skips it — and that is exactly how a completely static default shipped. The
// morph's flow is a blended coefficient table rather than a named deriv, so it
// has no entry in the registry the Lyapunov estimator walks.
//
// This covers the gap with the weaker property the estimator would have caught
// anyway: at every position on the catalog dial the trajectory has to MOVE.
// Chaos is not asserted here — the individual systems are checked for that in
// TestSprottCatalogDefaultsAreChaotic — but a fixed point is not an attractor
// at any tolerance.
func TestSprottMorphMovesAtEveryPosition(t *testing.T) {
	sys := SprottMorphSystems()
	if len(sys) == 0 {
		t.Fatal("no morph systems")
	}
	for m := 0.0; m < float64(len(sys)); m += 0.5 {
		x, y, z, ok := morphRun(sys, m, 4000)
		if !ok {
			t.Errorf("m=%.1f: trajectory left the bounds or went NaN", m)
			continue
		}
		// The span of the last part of the run, which is what is drawn.
		if span := math.Max(x, math.Max(y, z)); span < 1e-6 {
			i, _, _, _, _ := morphBlendAt(sys, m)
			t.Errorf("m=%.1f (%s): the trajectory spans %g — it is a fixed point, not an attractor",
				m, sys[i].Letter, span)
		}
	}
}

// The default the panel comes up on, called out separately: this is the one a
// user sees without touching anything.
func TestSprottMorphDefaultIsNotAFixedPoint(t *testing.T) {
	sys := SprottMorphSystems()
	const def = 3 // paramdefs_js.go: smorph-sys defaults to 3
	x, y, z, ok := morphRun(sys, def, 4000)
	if !ok {
		t.Fatal("the default position diverged")
	}
	if span := math.Max(x, math.Max(y, z)); span < 1e-6 {
		t.Errorf("the default (sys=%d) spans %g: the model is static on arrival", def, span)
	}
}

// morphRun integrates the blend at m from the SAME starting state the page
// does — the zero value — and reports the per-axis span of the second half.
//
// Starting from zero is the point. Seeding it with the system's own IC here
// would test a trajectory the app never runs, and would have passed happily
// while the app sat on the origin.
func morphRun(sys []SprottMorphSys, m float64, steps int) (sx, sy, sz float64, ok bool) {
	c, dt, _, _, _ := SprottMorphBlend(sys, m)
	var x, y, z float64
	reseed := func() {
		i := int(m) % len(sys)
		x = float64(sys[i].IC[0]) + 0.01
		y = float64(sys[i].IC[1]) + 0.01
		z = float64(sys[i].IC[2]) + 0.01
	}
	minv := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	maxv := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for s := 0; s < steps; s++ {
		dx, dy, dz := EvalQuad(&c, x, y, z)
		if dx == 0 && dy == 0 && dz == 0 {
			reseed()
			continue
		}
		x += dx * dt
		y += dy * dt
		z += dz * dt
		if x != x || y != y || z != z || math.Abs(x) > 60 || math.Abs(y) > 60 || math.Abs(z) > 60 {
			reseed()
			continue
		}
		if s > steps/2 {
			for k, v := range [3]float64{x, y, z} {
				if v < minv[k] {
					minv[k] = v
				}
				if v > maxv[k] {
					maxv[k] = v
				}
			}
		}
	}
	if math.IsInf(minv[0], 1) {
		return 0, 0, 0, false
	}
	return maxv[0] - minv[0], maxv[1] - minv[1], maxv[2] - minv[2], true
}

// morphBlendAt is SprottMorphBlend with the indices first, for a message.
func morphBlendAt(sys []SprottMorphSys, m float64) (i, j int, frac, dt float64, n int) {
	_, d, a, b, f := SprottMorphBlend(sys, m)
	return a, b, f, d, len(sys)
}
