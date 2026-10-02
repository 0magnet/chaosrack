package implicit

import (
	"math"
	"testing"
)

// Every point traced is on the surface, to within the grid's resolution: the
// crossing is interpolated along a cell edge, and a curved surface is off the
// straight edge by at most a fraction of a cell.
func TestTheContoursLieOnTheSurface(t *testing.T) {
	for name, c := range map[string]struct {
		f   Field
		tol float64
	}{
		"sphere": {func(x, y, z float64) float64 { return math.Sqrt(x*x+y*y+z*z) - 1 }, 0.01},
		"cube": {func(x, y, z float64) float64 {
			return math.Max(math.Abs(x), math.Max(math.Abs(y), math.Abs(z))) - 0.8
		}, 1e-9},
	} {
		v := Contours(c.f, 1.5, 12, 64)
		if len(v) == 0 || len(v)%6 != 0 {
			t.Fatalf("%s: %d coordinates", name, len(v))
		}
		for i := 0; i < len(v); i += 3 {
			if d := c.f(float64(v[i]), float64(v[i+1]), float64(v[i+2])); math.Abs(d) > c.tol+1e-6 {
				t.Errorf("%s: a traced point is %.4f off the surface", name, d)
				break
			}
		}
	}
}

// Nothing to trace where nothing crosses zero, and the drawing never passes
// what 16-bit indices can reach, however busy the surface.
func TestEmptyAndBusySurfaces(t *testing.T) {
	if v := Contours(func(x, y, z float64) float64 { return 1 }, 1, 8, 32); len(v) != 0 {
		t.Errorf("a surface that is nowhere traced %d coordinates", len(v))
	}
	gyroid := func(x, y, z float64) float64 {
		return math.Sin(9*x)*math.Cos(9*y) + math.Sin(9*y)*math.Cos(9*z) + math.Sin(9*z)*math.Cos(9*x)
	}
	if v := Contours(gyroid, 2, 40, 160); len(v)/3 > MaxVerts {
		t.Errorf("%d vertices, past %d", len(v)/3, MaxVerts)
	}
}
