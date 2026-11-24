package attractor

import (
	"math"
	"testing"
)

// flatKnobs is a rack's knobs at their defaults with model's DIM at 2.
func flatKnobs(model string, vals map[string]float64) func(string) float64 {
	return func(id string) float64 {
		if id == model+"-dim" {
			return 2
		}
		return vals[id]
	}
}

// radii is the smallest and largest distance of a figure's points from the
// center, and whether every point lies in z = 0.
func radii(f Figure) (lo, hi float64, flat bool) {
	lo, flat = math.Inf(1), true
	for _, p := range f.Points {
		r := math.Hypot(p[0], p[1])
		lo, hi = math.Min(lo, r), math.Max(hi, r)
		if p[2] != 0 {
			flat = false
		}
	}
	return lo, hi, flat
}

// Each geometry model's DIM 2 is flat, and the size of the solid it comes from.
func TestDimTwoIsFlat(t *testing.T) {
	cases := []struct {
		model  string
		knobs  map[string]float64
		lo, hi float64 // the radii the figure must reach, to 1e-6
	}{
		// The circle: the rim at 1, the north pole at the center.
		{"globe", map[string]float64{"globe-lat": 18, "globe-lon": 36}, 0, 1},
		// The annulus from R − r to R + r.
		{"torus", map[string]float64{"torus-R": 1.5, "torus-r": 0.5, "torus-stacks": 30, "torus-slices": 30}, 1, 2},
		// The squares' corners at √2 and √2/2.
		{"nestedcube", nil, math.Sqrt2 / 2, math.Sqrt2},
		// The triangle on the unit circle.
		{"polyhedron", map[string]float64{"polyhedron-p": 3, "polyhedron-q": 3}, 1, 1},
	}
	for _, c := range cases {
		f, ok := ShapeFigure(c.model, flatKnobs(c.model, c.knobs), 0)
		if !ok || len(f.Points) == 0 {
			t.Fatalf("%s: no figure", c.model)
		}
		lo, hi, flat := radii(f)
		if !flat {
			t.Errorf("%s: DIM 2 has a point off the plane", c.model)
		}
		if math.Abs(lo-c.lo) > 1e-6 || math.Abs(hi-c.hi) > 1e-6 {
			t.Errorf("%s: radii %v..%v, want %v..%v", c.model, lo, hi, c.lo, c.hi)
		}
	}
}

// The globe's circle is drawn whatever the knobs say: with no parallels and
// no meridians it is the circle alone.
func TestGlobeFlatIsTheCircleAlone(t *testing.T) {
	v, i := GlobeFlatMesh(0, 0, false, false, 0, nil, nil)
	if len(v)/3 != globePoints || len(i)/2 != globePoints {
		t.Fatalf("%d points, %d segments: want the rim alone, %d of each", len(v)/3, len(i)/2, globePoints)
	}
}

// A torus whose tube is wider than its hole leaves no hole: the band starts
// at the center.
func TestTorusFlatWithNoHole(t *testing.T) {
	v, _ := TorusFlatMesh(0.5, 1, 4, 3, 0)
	for k := 0; k < len(v); k += 3 {
		if r := math.Hypot(float64(v[k]), float64(v[k+1])); r > 1.5+1e-6 {
			t.Fatalf("a point at %v, past R + r", r)
		}
	}
}

// Without the knob (a rack whose catalog predates it reads it as 0) each
// model keeps its solid.
func TestDimMissingKeepsTheSolid(t *testing.T) {
	zero := func(id string) float64 {
		return map[string]float64{"globe-lat": 18, "globe-lon": 36, "polyhedron-p": 4, "polyhedron-q": 3}[id]
	}
	for _, m := range []string{"globe", "polyhedron", "nestedcube"} {
		f, ok := ShapeFigure(m, zero, 0)
		if !ok {
			t.Fatalf("%s: no figure", m)
		}
		if _, _, flat := radii(f); flat {
			t.Errorf("%s: drawn flat with no DIM knob", m)
		}
	}
}
