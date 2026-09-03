package attractor

import (
	"math"
	"testing"
)

// The coefficient extraction must reproduce every catalog deriv EXACTLY
// (quadratic probing has no truncation error on quadratics) — this is what
// lets Sprott Morph claim it integrates the same systems the app renders.

func TestQuadExtractReproducesCatalog(t *testing.T) {
	probes := [][3]float64{
		{0.3, -0.7, 1.2}, {-1.1, 0.4, -0.2}, {2.0, 1.5, -1.8}, {0, 0, 0}, {-0.5, -0.5, -0.5},
	}
	check := func(name string, deriv func(x, y, z float64) (float64, float64, float64)) {
		c := quadExtract(deriv)
		for _, p := range probes {
			wx, wy, wz := deriv(p[0], p[1], p[2])
			gx, gy, gz := evalQuad(&c, p[0], p[1], p[2])
			for _, d := range []float64{gx - wx, gy - wy, gz - wz} {
				if math.Abs(d) > 1e-9 {
					t.Fatalf("%s: extracted flow differs from deriv at %v by %g", name, p, d)
				}
			}
		}
	}
	check("Sprott A", sprottADeriv)
	for _, sc := range sprottCases {
		check(sc.name, sc.deriv)
	}
}

func TestMorphBlendEndpoints(t *testing.T) {
	sys := sprottMorphSystems()
	if len(sys) != 19 {
		t.Fatalf("catalog: %d systems, want 19 (A–S)", len(sys))
	}
	// At integer positions the blend must BE that system.
	for i, s := range sys {
		c, dt, gi, _, frac := morphBlend(sys, float64(i))
		if gi != i || frac != 0 {
			t.Fatalf("blend(%d): landed on %d+%.2f", i, gi, frac)
		}
		if c != s.coefs || dt != s.dt {
			t.Fatalf("blend(%d): coefficients differ from system %s", i, s.letter)
		}
	}
	// Halfway between D (3) and E (4) every coefficient is the average.
	c, _, _, _, _ := morphBlend(sys, 3.5)
	for k := range c {
		want := (sys[3].coefs[k] + sys[4].coefs[k]) / 2
		if math.Abs(c[k]-want) > 1e-12 {
			t.Fatalf("blend(3.5)[%d] = %g, want %g", k, c[k], want)
		}
	}
	// Position wraps: 19.25 ≡ 0.25, and S→A blends across the seam.
	c1, _, i1, j1, f1 := morphBlend(sys, 18.5)
	if i1 != 18 || j1 != 0 || math.Abs(f1-0.5) > 1e-12 {
		t.Fatalf("blend(18.5): got %d→%d %.2f, want 18→0 0.50", i1, j1, f1)
	}
	_ = c1
}
