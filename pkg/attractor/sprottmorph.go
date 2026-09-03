package attractor

// Sprott Morph — the faithful version of the glensstuff.com Self-Programming
// Analog Computer, which stepped itself through the Sprott catalog by
// re-patching. Every Sprott A–S system is a QUADRATIC 3-D flow, i.e. a
// point in the 30-dimensional coefficient space
//
//	dx_i/dt = c_1 + c_x·x + c_y·y + c_z·z + c_xx·x² + c_yy·y² + c_zz·z²
//	         + c_xy·xy + c_xz·xz + c_yz·yz            (×3 equations)
//
// — the very space Sprott's 1994 computer search ran in, and structurally
// what a patch panel selects. Morphing between systems is linear
// interpolation of the 30-vector while the SAME trajectory keeps
// integrating, so Sprott D visibly melts into Sprott E.
//
// The coefficient vectors are extracted NUMERICALLY from the catalog's own
// deriv closures (sprottdata.go) by exact quadratic probing — no hand
// transcription to drift out of sync. This file is untagged so the native
// tests can verify the extraction reproduces every deriv exactly.

// quadTerms is the per-equation term order of a coefficient row:
// 1, x, y, z, x², y², z², xy, xz, yz.
const quadTerms = 10

// sprottMorphSys is one catalog member in coefficient form.
type sprottMorphSys struct {
	letter string
	coefs  [3 * quadTerms]float64
	dt     float64
	ic     [3]float32
}

// quadExtractEq recovers one equation's 10 quadratic coefficients from a
// black-box evaluation function. Exact for quadratics: second-order finite
// differences at ±1 probes have zero truncation error.
func quadExtractEq(f func(x, y, z float64) float64) [quadTerms]float64 {
	var c [quadTerms]float64
	c0 := f(0, 0, 0)
	c[0] = c0
	// Linear + square terms, one axis at a time.
	ax := [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for i, a := range ax {
		p := f(a[0], a[1], a[2])
		n := f(-a[0], -a[1], -a[2])
		c[1+i] = (p - n) / 2  // x, y, z
		c[4+i] = (p+n)/2 - c0 // x², y², z²
	}
	// Cross terms from four-corner probes.
	cross := func(i, j int) float64 {
		var pp, pn, np, nn [3]float64
		pp[i], pp[j] = 1, 1
		pn[i], pn[j] = 1, -1
		np[i], np[j] = -1, 1
		nn[i], nn[j] = -1, -1
		return (f(pp[0], pp[1], pp[2]) - f(pn[0], pn[1], pn[2]) -
			f(np[0], np[1], np[2]) + f(nn[0], nn[1], nn[2])) / 4
	}
	c[7] = cross(0, 1) // xy
	c[8] = cross(0, 2) // xz
	c[9] = cross(1, 2) // yz
	return c
}

// quadExtract recovers all 30 coefficients of a 3-equation quadratic flow.
func quadExtract(deriv func(x, y, z float64) (float64, float64, float64)) [3 * quadTerms]float64 {
	var out [3 * quadTerms]float64
	for eq := 0; eq < 3; eq++ {
		e := eq
		c := quadExtractEq(func(x, y, z float64) float64 {
			dx, dy, dz := deriv(x, y, z)
			switch e {
			case 0:
				return dx
			case 1:
				return dy
			}
			return dz
		})
		copy(out[eq*quadTerms:], c[:])
	}
	return out
}

// sprottADeriv is Sprott A (it lives outside the B–S catalog table).
func sprottADeriv(x, y, z float64) (float64, float64, float64) {
	return y, -x + y*z, 1 - y*y
}

// sprottMorphSystems builds the full A–S coefficient table from the
// catalog's own equations.
func sprottMorphSystems() []sprottMorphSys {
	out := make([]sprottMorphSys, 0, 1+len(sprottCases))
	out = append(out, sprottMorphSys{
		letter: "A",
		coefs:  quadExtract(sprottADeriv),
		dt:     0.01,
		ic:     [3]float32{0.1, 0.2, 0.3},
	})
	for _, sc := range sprottCases {
		out = append(out, sprottMorphSys{
			letter: sc.name[len(sc.name)-1:],
			coefs:  quadExtract(sc.deriv),
			dt:     float64(sc.dt),
			ic:     sc.ic,
		})
	}
	return out
}

// evalQuad evaluates a 30-coefficient quadratic flow at (x, y, z).
func evalQuad(c *[3 * quadTerms]float64, x, y, z float64) (float64, float64, float64) {
	t := [quadTerms]float64{1, x, y, z, x * x, y * y, z * z, x * y, x * z, y * z}
	var d [3]float64
	for eq := 0; eq < 3; eq++ {
		s := 0.0
		for i := 0; i < quadTerms; i++ {
			s += c[eq*quadTerms+i] * t[i]
		}
		d[eq] = s
	}
	return d[0], d[1], d[2]
}

// morphBlend interpolates the coefficient table at position m ∈ [0, n):
// systems floor(m) and floor(m)+1 (wrapping) blended by the fraction, dt
// blended alongside so integration stays stable across timescale changes.
func morphBlend(systems []sprottMorphSys, m float64) (c [3 * quadTerms]float64, dt float64, i, j int, frac float64) {
	n := len(systems)
	for m < 0 {
		m += float64(n)
	}
	for m >= float64(n) {
		m -= float64(n)
	}
	i = int(m)
	if i >= n {
		i = 0
	}
	j = (i + 1) % n
	frac = m - float64(i)
	for k := range c {
		c[k] = systems[i].coefs[k]*(1-frac) + systems[j].coefs[k]*frac
	}
	dt = systems[i].dt*(1-frac) + systems[j].dt*frac
	return
}
