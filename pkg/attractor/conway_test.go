package attractor

import (
	"fmt"
	"math"
	"testing"
)

// vef is the shape of a solid, which is what identifies it: a solid with 24
// vertices, 36 edges and 14 faces IS a truncated cube, whatever the
// coordinates came out as.
func vef(p polyhedron) (int, int, int) { return len(p.Verts), len(p.Edges()), len(p.Faces) }

func name(s, o int) string { return conwayName(s, o) }

// Euler's formula is the one test that says a result is a solid rather than
// a bag of polygons: V - E + F = 2 for anything sphere-like, and every
// operator here has to preserve it.
func TestEveryGeneratedSolidSatisfiesEuler(t *testing.T) {
	for s := range polySeeds {
		for o := range polyOps {
			p := conwaySolid(s, o)
			if got := p.Euler(); got != 2 {
				v, e, f := vef(p)
				t.Errorf("%s: V-E+F = %d-%d+%d = %d, want 2", name(s, o), v, e, f, got)
			}
		}
	}
}

// The seeds are the solids they are named after.
func TestTheSeedsAreThePlatonicSolids(t *testing.T) {
	want := map[string][3]int{
		"T": {4, 6, 4}, "C": {8, 12, 6}, "O": {6, 12, 8},
		"D": {20, 30, 12}, "I": {12, 30, 20},
	}
	for s, seed := range polySeeds {
		p := conwaySolid(s, 0)
		v, e, f := vef(p)
		if got, w := [3]int{v, e, f}, want[seed.Letter]; got != w {
			t.Errorf("%s (%s) is V%d E%d F%d, want V%d E%d F%d",
				seed.Letter, seed.Name, v, e, f, w[0], w[1], w[2])
		}
	}
}

// The operators produce the Archimedean solids they are supposed to. These
// counts are the definition of each solid, so this is the test that says
// the operators are implemented and not merely plausible.
func TestTheOperatorsProduceTheNamedSolids(t *testing.T) {
	idx := func(letter string, list []string) int {
		for i, l := range list {
			if l == letter {
				return i
			}
		}
		return -1
	}
	seeds := make([]string, len(polySeeds))
	for i, s := range polySeeds {
		seeds[i] = s.Letter
	}
	ops := make([]string, len(polyOps))
	for i, o := range polyOps {
		ops[i] = o.Letter
	}

	for _, c := range []struct {
		op, seed string
		v, e, f  int
		solid    string
	}{
		{"d", "C", 6, 12, 8, "octahedron"},
		{"d", "O", 8, 12, 6, "cube"},
		{"d", "T", 4, 6, 4, "tetrahedron (self-dual)"},
		{"d", "I", 20, 30, 12, "dodecahedron"},
		{"d", "D", 12, 30, 20, "icosahedron"},
		{"a", "C", 12, 24, 14, "cuboctahedron"},
		{"a", "O", 12, 24, 14, "cuboctahedron"},
		{"a", "D", 30, 60, 32, "icosidodecahedron"},
		{"a", "T", 6, 12, 8, "octahedron"},
		{"t", "C", 24, 36, 14, "truncated cube"},
		{"t", "O", 24, 36, 14, "truncated octahedron"},
		{"t", "T", 12, 18, 8, "truncated tetrahedron"},
		{"t", "I", 60, 90, 32, "truncated icosahedron (the football)"},
		{"t", "D", 60, 90, 32, "truncated dodecahedron"},
		{"e", "C", 24, 48, 26, "rhombicuboctahedron"},
		{"b", "C", 48, 72, 26, "truncated cuboctahedron"},
	} {
		si, oi := idx(c.seed, seeds), idx(c.op, ops)
		if si < 0 || oi < 0 {
			t.Fatalf("no seed %q or operator %q", c.seed, c.op)
		}
		p := conwaySolid(si, oi)
		v, e, f := vef(p)
		if v != c.v || e != c.e || f != c.f {
			t.Errorf("%s is V%d E%d F%d, want V%d E%d F%d (%s)",
				c.op+c.seed, v, e, f, c.v, c.e, c.f, c.solid)
		}
	}
}

// Every face is a closed polygon of at least three DISTINCT vertices. A
// face that repeats a vertex is a fold, and it draws as a spike.
func TestNoFaceIsDegenerate(t *testing.T) {
	for s := range polySeeds {
		for o := range polyOps {
			p := conwaySolid(s, o)
			for fi, f := range p.Faces {
				if len(f) < 3 {
					t.Errorf("%s face %d has %d vertices", name(s, o), fi, len(f))
					continue
				}
				seen := map[int]bool{}
				for _, v := range f {
					if v < 0 || v >= len(p.Verts) {
						t.Errorf("%s face %d references vertex %d of %d", name(s, o), fi, v, len(p.Verts))
						continue
					}
					if seen[v] {
						t.Errorf("%s face %d visits vertex %d twice", name(s, o), fi, v)
					}
					seen[v] = true
				}
			}
		}
	}
}

// Every edge is shared by exactly two faces. One face means a hole, three
// means the winding is wrong somewhere.
func TestEveryEdgeIsSharedByTwoFaces(t *testing.T) {
	for s := range polySeeds {
		for o := range polyOps {
			p := conwaySolid(s, o)
			count := map[[2]int]int{}
			for _, f := range p.Faces {
				for i := range f {
					a, b := f[i], f[(i+1)%len(f)]
					if a > b {
						a, b = b, a
					}
					count[[2]int{a, b}]++
				}
			}
			for e, n := range count {
				if n != 2 {
					t.Errorf("%s: edge %v is on %d faces, want 2", name(s, o), e, n)
				}
			}
		}
	}
}

// Normalizing puts every vertex on the unit sphere, which is what makes a
// generated solid look like the one it names.
func TestNormalizePutsEveryVertexOnTheSphere(t *testing.T) {
	for s := range polySeeds {
		for o := range polyOps {
			p := conwaySolid(s, o)
			for i, v := range p.Verts {
				d := math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
				if math.Abs(d-1) > 1e-9 {
					t.Errorf("%s vertex %d is at radius %.6f", name(s, o), i, d)
				}
			}
		}
	}
}

// Edges come back once each, in a stable order, so the wireframe's index
// buffer does not change between builds of the same solid.
func TestEdgesAreUniqueAndStable(t *testing.T) {
	p := conwaySolid(1, 3) // tC
	a, b := p.Edges(), p.Edges()
	if len(a) != len(b) {
		t.Fatalf("two calls gave %d and %d edges", len(a), len(b))
	}
	seen := map[[2]int]bool{}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("edge %d differs between calls: %v vs %v", i, a[i], b[i])
		}
		if a[i][0] >= a[i][1] {
			t.Errorf("edge %v is not in low-high order", a[i])
		}
		if seen[a[i]] {
			t.Errorf("edge %v appears twice", a[i])
		}
		seen[a[i]] = true
	}
}

// The notation is Conway's, read right to left with the seed last.
func TestTheNameIsConwayNotation(t *testing.T) {
	for _, c := range []struct {
		s, o int
		want string
	}{
		{1, 0, "C"}, {1, 3, "tC"}, {1, 2, "aC"}, {3, 2, "aD"},
	} {
		if got := conwayName(c.s, c.o); got != c.want {
			t.Errorf("seed %d op %d gives %q, want %q", c.s, c.o, got, c.want)
		}
	}
	// Out-of-range comes back as something buildable rather than panicking:
	// a knob position that does not exist must not take the rack down.
	if got := conwayName(99, 99); got == "" {
		t.Error("an out-of-range seed and operator gave no name")
	}
	if p := conwaySolid(-1, -1); p.Euler() != 2 {
		t.Error("an out-of-range seed and operator gave something that is not a solid")
	}
}

// What the row is worth: the five seeds under these operators reach this
// many distinct solids, counting by shape rather than by name.
func TestTheGeneratorReachesMoreSolidsThanTheSelectorHad(t *testing.T) {
	shapes := map[string]string{}
	for s := range polySeeds {
		for o := range polyOps {
			v, e, f := vef(conwaySolid(s, o))
			shapes[fmt.Sprintf("%d-%d-%d", v, e, f)] = name(s, o)
		}
	}
	// The selector it replaces offered six fixed models.
	if len(shapes) <= 6 {
		t.Errorf("the generator reaches %d distinct solids, no better than the six-model selector", len(shapes))
	}
	t.Logf("%d distinct solids from %d seeds x %d operators", len(shapes), len(polySeeds), len(polyOps))
}
