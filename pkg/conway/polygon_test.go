package conway

import (
	"math"
	"testing"
)

// {p} has p corners on the unit circle and p edges, flat in z = 0.
func TestPolygonIsTheFace(t *testing.T) {
	for p := 3; p <= 12; p++ {
		s := Polygon(p, 0, 0)
		if len(s.Verts) != p || len(s.Edges()) != p {
			t.Fatalf("{%d}: %d corners, %d edges", p, len(s.Verts), len(s.Edges()))
		}
		for _, v := range s.Verts {
			if v.Z != 0 || math.Abs(math.Hypot(v.X, v.Y)-1) > 1e-12 {
				t.Fatalf("{%d}: corner %v is off the unit circle", p, v)
			}
		}
	}
}

// Morph runs polygon, 2p-gon, rectified (p corners again, turned half a
// step), 2p-gon, dual (p corners, turned): the solids' walk one dimension
// down, with no jump at 1.
func TestPolygonMorph(t *testing.T) {
	const p = 5
	for _, c := range []struct {
		m     float64
		verts int
	}{{0, p}, {0.5, 2 * p}, {1, p}, {1.5, 2 * p}, {2, p}} {
		if n := len(Polygon(p, c.m, 0).Verts); n != c.verts {
			t.Errorf("morph %v: %d corners, want %d", c.m, n, c.verts)
		}
	}
	// Either side of 1 approaches the rectified corners.
	near := func(m float64) Vert { return Polygon(p, m, 0).Verts[1] }
	rect := Polygon(p, 1, 0).Verts[0]
	for _, m := range []float64{0.999, 1.001} {
		v := near(m)
		if m > 1 {
			v = Polygon(p, m, 0).Verts[0]
		}
		if math.Hypot(v.X-rect.X, v.Y-rect.Y) > 0.01 {
			t.Errorf("morph %v: corner %v is not near the rectified %v", m, v, rect)
		}
	}
}

// Kis on a polygon is a star: a point on every edge, outward, or in below
// zero.
func TestPolygonKisIsAStar(t *testing.T) {
	s := Polygon(5, 0, 1)
	if len(s.Verts) != 10 {
		t.Fatalf("%d corners, want 10", len(s.Verts))
	}
	out := math.Hypot(s.Verts[1].X, s.Verts[1].Y)
	if out <= 1 {
		t.Errorf("an outward point at radius %v", out)
	}
	in := Polygon(5, 0, -0.5).Verts[1]
	if r := math.Hypot(in.X, in.Y); r >= math.Cos(math.Pi/5) {
		t.Errorf("an inward point at radius %v, not inside the edge", r)
	}
}

func TestPolygonAngle(t *testing.T) {
	for p, want := range map[int]float64{3: 60, 4: 90, 5: 108, 6: 120} {
		if got := PolygonAngle(p); math.Abs(got-want) > 1e-12 {
			t.Errorf("{%d}: %v°, want %v°", p, got, want)
		}
	}
}
