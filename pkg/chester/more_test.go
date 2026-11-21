package chester

import (
	"math"
	"testing"
)

// volume of a convex solid, by a fan from its corners' mean.
func volume(p Poly) float64 {
	var o V
	for _, v := range p.V {
		o = o.add(v)
	}
	o = o.mul(1 / float64(len(p.V)))
	vol := 0.0
	for _, f := range p.F {
		for i := 1; i+1 < len(f); i++ {
			a, b, c := p.V[f[0]].sub(o), p.V[f[i]].sub(o), p.V[f[i+1]].sub(o)
			vol += math.Abs(cross(b, c).dot(a)) / 6
		}
	}
	return vol
}

func sidesOf(p Poly) map[int]int {
	s := map[int]int{}
	for _, f := range p.F {
		s[len(f)]++
	}
	return s
}

// Frank's dualization: the half cut is the thirteen-faced decatria.
func TestTheDecatriaIsTheHalfCut(t *testing.T) {
	if p := Dualized(0); len(p.F) != 7 || len(p.V) != 7 {
		t.Errorf("uncut: V F = %d %d, want the chestahedron's 7 7", len(p.V), len(p.F))
	}
	p := Dualized(1)
	s := sidesOf(p)
	if len(p.F) != 13 || s[5] != 9 || s[6] != 3 || s[3] != 1 {
		t.Errorf("decatria: %d faces by sides %v; want 13: 9 pentagons, 3 hexagons, 1 triangle", len(p.F), s)
	}
	if len(p.V)-len(p.Edges())+len(p.F) != 2 {
		t.Errorf("decatria V E F = %d %d %d, not a sphere", len(p.V), len(p.Edges()), len(p.F))
	}
	flat(t, p, "decatria")
	// Inside the chestahedron, as a dual is.
	for _, pl := range facePlanes(Chestahedron()) {
		for i, v := range p.V {
			if pl.n.dot(v) > pl.d+1e-9 {
				t.Errorf("decatria corner %d is outside the chestahedron", i)
			}
		}
	}
	// Fully cut, only the seven cuts bound it: the dual heptahedron, its
	// base corners' cuts (four-cornered in the solid) now its kites.
	d := Dualized(2)
	ds := sidesOf(d)
	if len(d.F) != 7 || len(d.V) != 7 || ds[3] != 4 || ds[4] != 3 {
		t.Errorf("dual: V F = %d %d by sides %v; want 7 7, 4 triangles and 3 quadrilaterals", len(d.V), len(d.F), ds)
	}
	for m := 0.1; m < 2; m += 0.1 {
		if p := Dualized(m); len(p.F) < 7 || len(p.V)-len(p.Edges())+len(p.F) != 2 {
			t.Errorf("m=%.1f: a broken solid, V E F = %d %d %d", m, len(p.V), len(p.Edges()), len(p.F))
		}
	}
}

// The book's sequence in the cube lands on the solids it names.
func TestTheSpinsStages(t *testing.T) {
	near := func(p Poly, want []V) bool {
		for _, w := range want {
			found := false
			for _, v := range p.V {
				if v.sub(w).len() < 1e-9 {
					found = true
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	tet := []V{{1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	oct := []V{{1, 0.5, 0.5}, {0.5, 1, 0.5}, {0.5, 0.5, 1}, {0.5, 0.5, 0}, {0.5, 0, 0.5}, {0, 0.5, 0.5}} // the face centers
	dual := []V{{0, 0, 0}, {0, 1, 1}, {1, 0, 1}, {1, 1, 0}}
	if !near(Spin(0), tet) {
		t.Error("spin 0 is not the tetrahedron on alternate corners")
	}
	if p := Spin(1); !near(p, oct) || len(p.V) != 6 || len(p.F) != 8 {
		t.Errorf("spin 1 is not the octahedron (V F %d %d)", len(p.V), len(p.F))
	}
	if !near(Spin(2), dual) {
		t.Error("spin 2 is not the other tetrahedron")
	}
	// At ½ it is the chestahedron as it sits in the earth-study cube, that
	// cube made the unit one.
	sp := Spin(0.5)
	exact := Chestahedron()
	for i := range exact.V {
		if exact.V[i].mul(1/exactC).sub(sp.V[i]).len() > 1e-9 {
			t.Fatal("spin ½ is not the chestahedron in its cube")
		}
	}
	for m := 0.05; m < 2; m += 0.05 {
		p := Spin(m)
		if m <= 0.5 || m >= 1.5 { // the drawn way between is not flat-faced
			flat(t, p, "spin")
		}
		for _, v := range p.V {
			for _, x := range v {
				if x < -1e-9 || x > 1+1e-9 {
					t.Fatalf("spin %.2f leaves the cube: %v", m, v)
				}
			}
		}
	}
}

// His first form: four pieces of 3 kites and 4 triangles, round a tetrahedron.
func TestTheFirstPieces(t *testing.T) {
	pieces, inner := FirstPieces(0)
	total := volume(inner)
	for i, p := range pieces {
		total += volume(p)
		s := sidesOf(p)
		if len(p.V) != 7 || len(p.Edges()) != 12 || s[4] != 3 || s[3] != 4 {
			t.Errorf("piece %d: V E = %d %d by sides %v; want 7 12, 3 kites and 4 triangles", i, len(p.V), len(p.Edges()), s)
		}
		flat(t, p, "piece")
		// Each kite is two 30-60-90 triangles: 60° at the corner, 90° at the
		// midpoints, 120° at the face's center.
		k := p.F[0]
		ang := func(o, x, y int) float64 {
			u, v := p.V[x].sub(p.V[o]), p.V[y].sub(p.V[o])
			return math.Acos(u.dot(v)/u.len()/v.len()) * 180 / math.Pi
		}
		got := [4]float64{ang(k[0], k[3], k[1]), ang(k[1], k[0], k[2]), ang(k[2], k[1], k[3]), ang(k[3], k[2], k[0])}
		want := [4]float64{60, 90, 120, 90}
		for j := range got {
			if math.Abs(got[j]-want[j]) > 1e-9 {
				t.Errorf("piece %d kite angles %v, want %v", i, got, want)
				break
			}
		}
	}
	if math.Abs(total-1.0/3) > 1e-9 { // the tetrahedron on alternate corners of the unit cube
		t.Errorf("pieces and inner tetrahedron fill %.6f, the tetrahedron is 1/3", total)
	}
}

func TestTheRollLiesOnItsCone(t *testing.T) {
	if p := Roll(0.7, 1.1, 1); math.Abs(p[1]) > 1e-12 {
		t.Errorf("unrolled, the disk is not flat: %v", p)
	}
	for _, k := range []float64{2, 3, 4} {
		// The first turn lies exactly on the cone of half-angle asin(1/k).
		for _, phi := range []float64{0.1, 0.5, 1.2} {
			p := Roll(0.8, phi/k, k)
			half := math.Atan2(math.Hypot(p[0], p[2]), p[1])
			if math.Abs(math.Sin(half)-1/k) > 1e-9 {
				t.Errorf("k=%v: off the cone (sin %.6f)", k, math.Sin(half))
			}
		}
	}
}

func TestTheEarthCubeHoldsTheSolid(t *testing.T) {
	c := exactC
	ch := Chestahedron()
	for i, v := range ch.V {
		for _, x := range v {
			if x < -1e-12 || x > c+1e-12 {
				t.Errorf("corner %d outside [0,c]³: %v", i, v)
			}
		}
	}
	// Peak in the corner, base corners on the edges from the origin.
	if ch.V[vPeak].sub(V{c, c, c}).len() > 1e-12 {
		t.Error("the peak is not in the cube's corner")
	}
}

func TestTheBooksCubeIsTheHalfStarsShadow(t *testing.T) {
	small, outline := Hexagrams(1)
	if small != 0.5 || math.Abs(outline-1/(2*math.Sqrt(3))) > 1e-15 {
		t.Errorf("hexagrams of a unit band: %v %v", small, outline)
	}
	// The core sphere, √3 × the cube's edge across, inside the solid's
	// sphere; the cone's, sin 13°.
	if c := InnerCore(); c <= 0 || c > 0.5 {
		t.Errorf("inner core %v", c)
	}
	if c := ConeCore(); math.Abs(c-0.2249510543) > 1e-9 {
		t.Errorf("cone core %v, want sin 13°", c)
	}
}
