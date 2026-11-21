package chester

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// flat reports whether every face of p lies in one plane.
func flat(t *testing.T, p Poly, what string) {
	t.Helper()
	for fi, face := range p.F {
		if len(face) < 4 {
			continue
		}
		a, b, c := p.V[face[0]], p.V[face[1]], p.V[face[2]]
		n := cross(b.sub(a), c.sub(a))
		if n.len() < 1e-12 {
			continue // a degenerate corner of a shrinking face
		}
		n = n.unit()
		for _, v := range face[3:] {
			if d := math.Abs(p.V[v].sub(a).dot(n)); d > 1e-9 {
				t.Errorf("%s: face %d is not flat (%.3g off)", what, fi, d)
			}
		}
	}
}

func TestTheSevenFacesHaveOneArea(t *testing.T) {
	p := Chestahedron()
	want := math.Sqrt(3) / 2 // an equilateral triangle of edge √2
	for f := range p.F {
		if a := p.Area(f); !near(a, want, 1e-12) {
			t.Errorf("face %d (%d corners): area %.15f, want %.15f", f, len(p.F[f]), a, want)
		}
	}
	flat(t, p, "chestahedron")
	if len(p.V) != 7 || len(p.Edges()) != 12 || len(p.F) != 7 {
		t.Errorf("V E F = %d %d %d, want 7 12 7", len(p.V), len(p.Edges()), len(p.F))
	}
}

func TestTheTrianglesAreEquilateral(t *testing.T) {
	p := Chestahedron()
	for _, f := range p.F {
		if len(f) != 3 {
			continue
		}
		for i := range f {
			if d := p.V[f[i]].sub(p.V[f[(i+1)%3]]).len(); !near(d, math.Sqrt2, 1e-12) {
				t.Errorf("triangle %v has an edge %.15f", f, d)
			}
		}
	}
}

// The numbers in the package comment and the model's Equation lines.
func TestTheExactNumbers(t *testing.T) {
	a, b, c := Exact()
	if !near(a, 1.246702669, 1e-9) || !near(b, 0.620379116, 1e-9) || !near(c, 1.359185993, 1e-9) {
		t.Errorf("a b c = %.12f %.12f %.12f", a, b, c)
	}
}

func TestTheGoldenRelationsAreNearMisses(t *testing.T) {
	kb, kp, kt := KiteAngles()
	if !near(kb, 36.4998, 1e-3) || !near(kp, 71.7501, 1e-3) || !near(kt, 125.8751, 1e-3) {
		t.Errorf("kite angles %.4f %.4f %.4f", kb, kp, kt)
	}
	if math.Abs(kb-36) < 0.4 {
		t.Errorf("the kite's base angle %.4f is a pentagram's: the near miss is gone", kb)
	}
	h := HexagonAt()
	if !near(h, 0.614884, 1e-6) || math.Abs(h-1/phi) < 0.003 {
		t.Errorf("hexagon at %.9f (1/φ = %.9f)", h, 1/phi)
	}
	if r := StarKiteRatio(); !near(r, 0.9823, 1e-3) {
		t.Errorf("a pentagram kite over a triangle = %.5f", r)
	}
}

func TestTheBandIsARegularHexagon(t *testing.T) {
	b := Band(Chestahedron(), HexagonAt())
	side := b[0].sub(b[1]).len()
	for i := range b {
		if d := b[i].sub(b[(i+1)%6]).len(); !near(d, side, 1e-9) {
			t.Errorf("side %d is %.12f, side 0 is %.12f", i, d, side)
		}
	}
}

// The family runs from the regular tetrahedron to the unit cube, every face
// flat on the way.
func TestTheFamilyEnds(t *testing.T) {
	tet := Family(0)
	pts := []V{tet.V[vE1], tet.V[vE2], tet.V[vE3], tet.V[vPeak]}
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if d := pts[i].sub(pts[j]).len(); !near(d, math.Sqrt2, 1e-9) {
				t.Errorf("tetrahedron edge %d-%d = %.12f", i, j, d)
			}
		}
	}
	for _, tip := range []int{vP12, vP13, vP23} {
		if tet.V[tip].sub(tet.V[vPeak]).len() > 1e-9 {
			t.Errorf("tetrahedron: tip %d is not closed onto the peak", tip)
		}
	}
	cube := Family(2)
	corners := map[V]bool{}
	for _, v := range cube.V {
		r := V{math.Round(v[0]*1e9) / 1e9, math.Round(v[1]*1e9) / 1e9, math.Round(v[2]*1e9) / 1e9}
		for _, x := range r {
			if x != 0 && x != 1 {
				t.Fatalf("cube corner %v is not a unit cube's", v)
			}
		}
		corners[r] = true
	}
	if len(corners) != 8 {
		t.Errorf("the cube has %d distinct corners", len(corners))
	}
	for m := 0.05; m < 2; m += 0.05 {
		flat(t, Family(m), "family")
	}
}

func TestTheStarFoldsRigidlyAndCloses(t *testing.T) {
	for s := 0.0; s <= 1; s += 0.125 {
		peak, cs, ps := StarFold(s)
		for j := range ps {
			if !near(cs[j].sub(peak).len(), pentR, 1e-12) {
				t.Errorf("s=%v: spoke %d stretched", s, j)
			}
			if !near(cs[j].sub(cs[j+1]).len(), 1, 1e-9) {
				t.Errorf("s=%v: kite %d's diagonal is %.9f, a pentagon side is 1", s, j, cs[j].sub(cs[j+1]).len())
			}
			for _, c := range []V{cs[j], cs[j+1]} {
				if !near(ps[j].sub(c).len(), phi, 1e-9) {
					t.Errorf("s=%v: star point %d is %.9f from its corner, want φ", s, j, ps[j].sub(c).len())
				}
			}
		}
	}
	_, cs, _ := StarFold(1)
	if cs[3].sub(cs[0]).len() > 1e-9 {
		t.Errorf("folded, the last spoke misses the first by %.3g", cs[3].sub(cs[0]).len())
	}
	_, cs, _ = StarFold(0)
	for _, c := range cs {
		if math.Abs(c[1]) > 1e-12 {
			t.Errorf("unfolded, a corner is off the plane: %v", c)
		}
	}
}

func TestEversionReflectsEachPyramid(t *testing.T) {
	p := Chestahedron()
	o := p.Centroid()
	half, out := Everted(p, 0.5), Everted(p, 1)
	for i, face := range p.F {
		a, b, c := p.V[face[0]], p.V[face[1]], p.V[face[2]]
		n := cross(b.sub(a), c.sub(a)).unit()
		depth := math.Abs(o.sub(a).dot(n))
		if d := math.Abs(half[i].sub(a).dot(n)); d > 1e-12 {
			t.Errorf("face %d: at ½ the apex is %.3g off the face", i, d)
		}
		if d := math.Abs(out[i].sub(a).dot(n)); !near(d, depth, 1e-12) {
			t.Errorf("face %d: everted apex %.6f from the face, the center was %.6f", i, d, depth)
		}
		if out[i].sub(o).dot(a.sub(o)) <= 0 {
			t.Errorf("face %d: everted apex is not outside", i)
		}
	}
}

func TestFormsFitTheIndexBuffer(t *testing.T) {
	for f := range NumForms {
		for _, m := range []float64{0, 0.5, 1, 1.5, 2} {
			for d := 0; d <= MaxDetail; d++ {
				l := Form(f, m, float64(d))
				if len(l.P) == 0 || len(l.P) > 65535 {
					t.Errorf("%s m=%v d=%d: %d points", FormNames[f], m, d, len(l.P))
				}
				for _, e := range l.E {
					if e[0] >= len(l.P) || e[1] >= len(l.P) {
						t.Fatalf("%s: an edge past the points", FormNames[f])
					}
				}
				for _, p := range l.P {
					if p.len() > 3 || math.IsNaN(p.len()) {
						t.Errorf("%s m=%v d=%d: a point at %v", FormNames[f], m, d, p)
						break
					}
				}
			}
		}
	}
	if len(FormNames) != NumForms {
		t.Errorf("%d names for %d forms", len(FormNames), NumForms)
	}
}
