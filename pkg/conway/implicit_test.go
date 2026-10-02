package conway

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/equation"
)

// The equation is the solid: every vertex on it, the center inside, a point
// far away outside — read back through the same parser Custom uses.
func TestTheImplicitEquationIsTheSolid(t *testing.T) {
	check := func(name string, s Solid) {
		eq, ok := Implicit(s)
		if !ok {
			t.Errorf("%s: not convex", name)
			return
		}
		e, err := equation.ParseExpr(eq)
		if err != nil {
			t.Errorf("%s: %q does not parse: %v", name, eq, err)
			return
		}
		st := make([]float64, e.StackNeed()+2)
		f := func(x, y, z float64) float64 { return e.Eval([5]float64{x, y, z}, nil, st) }
		for _, v := range s.Verts {
			if d := f(v.X, v.Y, v.Z); math.Abs(d) > 2e-3 {
				t.Errorf("%s: %q is %.4f at a vertex", name, eq, d)
				break
			}
		}
		if f(0, 0, 0) >= 0 || f(9, 9, 9) <= 0 {
			t.Errorf("%s: %q does not have the center inside and the far outside", name, eq)
		}
	}
	for _, pq := range [][2]int{{3, 3}, {4, 3}, {3, 4}, {5, 3}, {3, 5}} {
		s := Regular(pq[0], pq[1])
		check("regular", s)
		for _, m := range MorphStops {
			check("morph", Morph(s, m))
		}
	}
}

// The abs terms make the regular solids read the way they are written by hand.
func TestTheRegularEquationsAreShort(t *testing.T) {
	if eq, _ := Implicit(Regular(3, 4)); eq != "abs(x) + abs(y) + abs(z) - 0.577" && eq != "abs(x) + abs(y) + abs(z) - 1" {
		// Scaled so the first coefficient is 1; the octahedron's vertices are
		// on the unit sphere, so its faces are at 1/√3 of the way along
		// (1,1,1)/√3 — the right-hand side is 1.
		t.Errorf("octahedron: %q", eq)
	}
	if eq, _ := Implicit(Regular(4, 3)); eq != "max(abs(x), abs(y), abs(z)) - 0.577" {
		t.Errorf("cube: %q", eq)
	}
}

// Pyramids pressed in make a solid no single max of planes describes.
func TestANonConvexSolidHasNoSingleMax(t *testing.T) {
	if _, ok := Implicit(Regular(4, 3).Kis(-0.3)); ok {
		t.Error("a cube with its faces pressed in reported convex")
	}
}
