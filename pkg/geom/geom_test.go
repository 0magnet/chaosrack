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

func TestSphereIndicesInRange(t *testing.T) {
	v, idx := Sphere(1, 16, 16, 0)
	n := uint16(len(v) / 3)
	for _, i := range idx {
		if i >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}

func TestTorusIndicesInRange(t *testing.T) {
	v, idx := Torus(1.5, 0.5, 30, 30, 0)
	n := uint16(len(v) / 3)
	for _, i := range idx {
		if i >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}

func TestMagnetosphereIndicesInRange(t *testing.T) {
	l := Magnetosphere()
	if len(l.Vertices) == 0 || len(l.Indices) == 0 {
		t.Fatal("empty magnetosphere")
	}
	n := uint16(len(l.Vertices) / 3)
	for _, i := range l.Indices {
		if i >= n {
			t.Fatalf("index %d out of range (%d vertices)", i, n)
		}
	}
}
