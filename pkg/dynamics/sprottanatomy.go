package dynamics

import "math"

// Anatomy is what one of Sprott's systems is made of, read off its
// coefficients rather than written down beside them: the term counts are what
// his search minimized, and the divergence says how the flow treats volume.
type Anatomy struct {
	Terms     int        // nonzero terms across the three equations
	Nonlinear []string   // the quadratic ones, e.g. "yz", "x²"
	Div       [4]float64 // ∇·F = Div[0] + Div[1]·x + Div[2]·y + Div[3]·z
	HalfTurnZ bool       // unchanged by (x, y, z) → (−x, −y, z)
}

var quadNames = [quadTerms]string{"1", "x", "y", "z", "x²", "y²", "z²", "xy", "xz", "yz"}

// Anatomy reads the case's structure from its vector field.
func (c Case) Anatomy() Anatomy {
	// The coefficients come from probing the field, so a sum like 2.7 + 0.7
	// leaves a few ulps where a term is absent.
	const eps = 1e-9
	clean := func(v float64) float64 {
		if math.Abs(v) < eps {
			return 0
		}
		return v
	}
	co := quadExtract(c.Deriv)
	var a Anatomy
	for eq := range 3 {
		for i := range quadTerms {
			if clean(co[eq*quadTerms+i]) != 0 {
				a.Terms++
				if i >= 4 {
					a.Nonlinear = append(a.Nonlinear, quadNames[i])
				}
			}
		}
	}
	f, g, h := co[:quadTerms], co[quadTerms:2*quadTerms], co[2*quadTerms:]
	a.Div = [4]float64{
		clean(f[1] + g[2] + h[3]),
		clean(2*f[4] + g[7] + h[8]),
		clean(f[7] + 2*g[5] + h[9]),
		clean(f[8] + g[9] + 2*h[6]),
	}
	a.HalfTurnZ = true
	for _, p := range [][3]float64{{0.3, -0.7, 1.1}, {-1.3, 0.2, 0.5}, {2, 1, -0.4}} {
		x, y, z := c.Deriv(p[0], p[1], p[2])
		mx, my, mz := c.Deriv(-p[0], -p[1], p[2])
		if math.Abs(mx+x) > eps || math.Abs(my+y) > eps || math.Abs(mz-z) > eps {
			a.HalfTurnZ = false
		}
	}
	return a
}
