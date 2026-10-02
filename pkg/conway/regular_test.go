package conway

import (
	"math"
	"testing"
)

// The five spherical {p,q} are the five Platonic solids.
func TestTheSphericalSymbolsAreThePlatonicSolids(t *testing.T) {
	want := map[[2]int][3]int{
		{3, 3}: {4, 6, 4}, {4, 3}: {8, 12, 6}, {3, 4}: {6, 12, 8},
		{5, 3}: {20, 30, 12}, {3, 5}: {12, 30, 20},
	}
	for pq, w := range want {
		if c := CurvatureOf(pq[0], pq[1]); c != Spherical {
			t.Errorf("{%d,%d}: curvature %d, want spherical", pq[0], pq[1], c)
		}
		v, e, f := vef(Regular(pq[0], pq[1]))
		if [3]int{v, e, f} != w {
			t.Errorf("{%d,%d}: V,E,F = %d,%d,%d, want %v", pq[0], pq[1], v, e, f, w)
		}
	}
}

func TestCurvature(t *testing.T) {
	for _, c := range []struct {
		p, q int
		want Curvature
	}{{4, 4, Flat}, {3, 6, Flat}, {6, 3, Flat}, {4, 5, Hyperbolic}, {5, 4, Hyperbolic}, {5, 5, Hyperbolic}, {6, 6, Hyperbolic}} {
		if got := CurvatureOf(c.p, c.q); got != c.want {
			t.Errorf("{%d,%d}: %d, want %d", c.p, c.q, got, c.want)
		}
	}
}

// A tiling is {p,q} if q faces meet at its corners. The corners of the
// central face are surrounded by faces in any patch that has grown at all, so
// that is where it is counted. Hyperbolic faces carry their arcs' points as
// vertices too, so a corner is told by being on the central face's corner
// circle.
func TestTheTilingsHaveQFacesAtACorner(t *testing.T) {
	for _, pq := range [][2]int{{4, 4}, {3, 6}, {6, 3}, {4, 5}, {5, 4}, {5, 5}, {3, 7}, {6, 4}} {
		p, q := pq[0], pq[1]
		s := Regular(p, q)
		faces := s.Faces
		if CurvatureOf(p, q) == Hyperbolic {
			faces = faces[:len(faces)-1] // the rim is not a face of the tiling
		}
		center := faces[0]
		step := 1
		if CurvatureOf(p, q) == Hyperbolic {
			step = arcSteps
		}
		for k := 0; k < len(center); k += step {
			v := center[k]
			n := 0
			for _, f := range faces {
				for _, i := range f {
					if i == v {
						n++
						break
					}
				}
			}
			if n != q {
				t.Errorf("{%d,%d}: %d faces at a corner of the central face, want %d", p, q, n, q)
				break
			}
		}
		if len(faces) < 10 {
			t.Errorf("{%d,%d}: only %d faces", p, q, len(faces))
		}
		for _, v := range s.Verts {
			if math.Hypot(v.X, v.Y) > 1+1e-9 || v.Z != 0 {
				t.Errorf("{%d,%d}: vertex %v outside the unit disk", p, q, v)
				break
			}
		}
	}
}

// The morph has no jump where its halves meet: just either side of 1, every
// vertex is next to one of the rectified solid's.
func TestMorphMeetsItselfAtTheRectifiedSolid(t *testing.T) {
	for _, pq := range [][2]int{{3, 3}, {4, 3}, {5, 3}} {
		s := Regular(pq[0], pq[1])
		mid := Morph(s, 1)
		for _, m := range []float64{0.99, 1.01} {
			near := Morph(s, m)
			for _, v := range near.Verts {
				best := math.Inf(1)
				for _, w := range mid.Verts {
					best = min(best, math.Hypot(math.Hypot(v.X-w.X, v.Y-w.Y), v.Z-w.Z))
				}
				if best > 0.02 {
					t.Errorf("{%d,%d} at %.2f: a vertex %.3f from the rectified solid", pq[0], pq[1], m, best)
					break
				}
			}
		}
	}
}

// Every named stop of the morph is a solid.
func TestMorphStopsAreSolids(t *testing.T) {
	for _, pq := range [][2]int{{3, 3}, {4, 3}, {3, 4}, {5, 3}, {3, 5}} {
		for _, m := range MorphStops {
			if e := Morph(Regular(pq[0], pq[1]), m).Euler(); e != 2 {
				t.Errorf("{%d,%d} morph %.2f: Euler %d", pq[0], pq[1], m, e)
			}
		}
	}
}
