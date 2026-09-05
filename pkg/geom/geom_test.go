package geom

import (
	"math"
	"testing"
)

// The globe the attractor's Globe mode uploads: 17 latitude circles and
// 36 meridians of 60 points, all on the unit sphere.
func TestGlobeShape(t *testing.T) {
	l := Globe(18, 36, 60)
	wantVerts := (17 + 36) * 61 * 3
	if len(l.Vertices) != wantVerts {
		t.Errorf("vertices = %d floats, want %d", len(l.Vertices), wantVerts)
	}
	wantIdx := (17 + 36) * 60 * 2
	if len(l.Indices) != wantIdx {
		t.Errorf("indices = %d, want %d", len(l.Indices), wantIdx)
	}
	for i := 0; i+2 < len(l.Vertices); i += 3 {
		x, y, z := float64(l.Vertices[i]), float64(l.Vertices[i+1]), float64(l.Vertices[i+2])
		if r := math.Sqrt(x*x + y*y + z*z); math.Abs(r-1) > 1e-5 {
			t.Fatalf("vertex %d is at radius %v, not on the unit sphere", i/3, r)
		}
	}
}

// The vertex count is held as an int and the index widened to meet it,
// rather than the count being narrowed to uint16. Narrowing it makes the
// check vacuous in exactly the case it exists for: past 65535 vertices
// the count wraps too, so a wrapped index compares against a wrapped
// bound and every assertion passes.
func TestSphereIndicesInRange(t *testing.T) {
	v, idx := Sphere(1, 16, 16, 0)
	n := len(v) / 3
	for _, i := range idx {
		if int(i) >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}

func TestTorusIndicesInRange(t *testing.T) {
	v, idx := Torus(1.5, 0.5, 30, 30, 0)
	n := len(v) / 3
	for _, i := range idx {
		if int(i) >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}

func TestMagnetosphereIndicesInRange(t *testing.T) {
	l := Magnetosphere()
	if len(l.Vertices) == 0 || len(l.Indices) == 0 {
		t.Fatal("empty magnetosphere")
	}
	n := len(l.Vertices) / 3
	for _, i := range l.Indices {
		if int(i) >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}

// The largest models the callers can ask for. The attractor's own globe
// is knob-driven — parallels 2..60 and meridians 1..90, in
// pkg/splitwasm/control.go — and that package is js-tagged, so its
// generator cannot be reached from a native test. Its geometry is the
// same shape as Globe's, so checking Globe at those maxima is what says
// the knob ranges are still inside the budget. Widen a knob past this and
// this test is where it shows up.
func TestKnobMaximaFitTheIndexSpace(t *testing.T) {
	const (
		maxParallels = 60
		maxMeridians = 90
	)
	l := Globe(maxParallels, maxMeridians, 60)
	n := len(l.Vertices) / 3
	if n > MaxVertices {
		t.Errorf("parallels=%d meridians=%d gives %d vertices, past the %d "+
			"a uint16 index can address", maxParallels, maxMeridians, n, MaxVertices)
	}
	// The count fitting is necessary but not sufficient: Globe restarts
	// its base index at each circle, so what has to hold is that every
	// index still names the vertex it was built for.
	for i, idx := range l.Indices {
		if int(idx) >= n {
			t.Fatalf("parallels=%d meridians=%d: index %d at position %d "+
				"addresses vertex %d of %d — the conversion wrapped",
				maxParallels, maxMeridians, idx, i, idx, n)
		}
	}
}

// Globe had no index check at all — TestGlobeShape counts them and puts
// every vertex on the unit sphere, but never follows an index to the
// vertex it names. Globe is also the one generator the attractor drives
// from knobs, so it is the one where a parameter can change.
func TestGlobeIndicesInRange(t *testing.T) {
	l := Globe(18, 36, 60)
	n := len(l.Vertices) / 3
	for _, i := range l.Indices {
		if int(i) >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}
