package sacred

import (
	"math"
	"testing"
)

// The Egg of Life seen down z is the Seed of Life: of its eight sphere
// centers, two fall on the center and six on a circle of radius one, 60°
// apart.
func TestEggSeenDownItsAxisIsTheSeed(t *testing.T) {
	a := math.Sqrt(1.5) / 2
	center, ring := 0, 0
	var angles []float64
	for i := range 8 {
		p := diagonal(V{float64(i&1*2-1) * a, float64(i>>1&1*2-1) * a, float64(i>>2&1*2-1) * a})
		switch r := math.Hypot(p[0], p[1]); {
		case r < 1e-9:
			center++
		case math.Abs(r-1) < 1e-9:
			ring++
			angles = append(angles, math.Atan2(p[1], p[0]))
		default:
			t.Fatalf("a corner at radius %v down the axis", r)
		}
	}
	if center != 2 || ring != 6 {
		t.Fatalf("%d at the center, %d on the ring; want 2 and 6", center, ring)
	}
	for i := range angles {
		for j := i + 1; j < len(angles); j++ {
			d := math.Mod(math.Abs(angles[i]-angles[j]), math.Pi/3)
			if min(d, math.Pi/3-d) > 1e-9 {
				t.Fatalf("ring corners %v° apart, not a multiple of 60°", (angles[i]-angles[j])*180/math.Pi)
			}
		}
	}
}

// The merkaba seen down z is the hexagram: its eight corners fall two on the
// center and six on one circle.
func TestMerkabaSeenDownItsAxisIsTheHexagram(t *testing.T) {
	ring := 0
	r0 := 0.0
	for _, s := range starTetrahedron(0) {
		for _, p := range s {
			if r := math.Hypot(p[0], p[1]); r > 1e-9 {
				if r0 == 0 {
					r0 = r
				}
				if math.Abs(r-r0) > 1e-9 {
					t.Fatalf("corners at radii %v and %v: not one circle", r0, r)
				}
				ring++
			}
		}
	}
	if ring == 0 {
		t.Fatal("no corners off the axis")
	}
}

// The face-centered cubic packing's shells: twelve round one (the vector
// equilibrium), 55 to two.
func TestPackingShells(t *testing.T) {
	if n := len(fccPoints(1)); n != 13 {
		t.Errorf("one shell: %d spheres, want 13", n)
	}
	if n := len(fccPoints(2)); n != 55 {
		t.Errorf("two shells: %d spheres, want 55", n)
	}
	// Every nearest neighbor is one apart.
	cs := fccPoints(1)
	for _, c := range cs[1:] {
		if d := dist3(c, V{}); math.Abs(d-1) > 1e-9 {
			t.Errorf("a neighbor %v away", d)
		}
	}
}

// The three golden rectangles' corners are the icosahedron's: thirty edges,
// all of one length.
func TestGoldenRectanglesAreTheIcosahedron(t *testing.T) {
	ss := goldenRects(1)
	if n := len(ss) - 3; n != 30 {
		t.Errorf("%d edges, want 30", n)
	}
	for _, s := range ss[3:] {
		if l := dist3(s[0], s[1]); math.Abs(l-2) > 1e-9 {
			t.Errorf("an edge of length %v", l)
		}
	}
}

// Kepler's spheres: each the inradius of the solid outside it — a third for
// the tetrahedron, 1/√3 for the cube and octahedron.
func TestKeplerSpheres(t *testing.T) {
	rs := keplerRadii()
	if len(rs) != len(Planets) {
		t.Fatalf("%d spheres for %d planets", len(rs), len(Planets))
	}
	for i, want := range map[int]float64{0: 1 / math.Sqrt(3), 1: 1.0 / 3, 4: 1 / math.Sqrt(3)} {
		if got := rs[i+1] / rs[i]; math.Abs(got-want) > 1e-9 {
			t.Errorf("%s to %s: %v, want %v", Planets[i+1], Planets[i], got, want)
		}
	}
	// The dodecahedron and icosahedron share one ratio.
	if a, b := rs[3]/rs[2], rs[4]/rs[3]; math.Abs(a-b) > 1e-9 {
		t.Errorf("dodecahedron %v, icosahedron %v", a, b)
	}
}
