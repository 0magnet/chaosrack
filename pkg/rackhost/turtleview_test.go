package rackhost

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// turned is p turned by angles as rasterview turns a model: Rz, then Ry,
// then Rx.
func turned(p, a [3]float64) [3]float64 {
	sx, cx := math.Sincos(a[0])
	sy, cy := math.Sincos(a[1])
	sz, cz := math.Sincos(a[2])
	x1, y1 := p[0]*cz-p[1]*sz, p[0]*sz+p[1]*cz
	x2, z2 := x1*cy+p[2]*sy, -x1*sy+p[2]*cy
	return [3]float64{x2, y1*cx - z2*sx, y1*sx + z2*cx}
}

func sameDir(a, b [3]float64) bool {
	return math.Abs(a[0]-b[0]) < 1e-9 && math.Abs(a[1]-b[1]) < 1e-9 && math.Abs(a[2]-b[2]) < 1e-9
}

// The angles turn from onto to, whichever two directions they are, opposite
// ones included.
func TestAimAnglesTurnOneDirectionOntoAnother(t *testing.T) {
	dirs := [][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {0, 0, -1}, {-1, 0, 0}, {0, -1, 0}, {1, 2, 3}, {-3, 1, 0.5}, {0.2, -0.1, -5}}
	for _, from := range dirs {
		for _, to := range dirs {
			if got := turned(unit(from), aimAngles(from, to)); !sameDir(got, unit(to)) {
				t.Errorf("%v onto %v: turned to %v", from, to, got)
			}
		}
	}
}

// VIEW at end turns the Turtle's axis toward the viewer and parks
// auto-rotate; at free, auto-rotate comes back.
func TestTheTurtlesViewAimsItsAxis(t *testing.T) {
	r, c := idleRack(t, "turtle")
	set(t, r, "auto-rotate", "1")
	watch(t, r, c, 0.2)
	r.mu.Lock()
	axis, ok := r.analyzers.scope.turtle.Axis()
	r.mu.Unlock()
	if !ok {
		t.Skip("the default walk has no axis")
	}
	set(t, r, "turtle-view", "1")
	watch(t, r, c, 0.1)
	r.scene.mu.Lock()
	a := r.scene.angle
	r.scene.mu.Unlock()
	v := rasterview.View{AngleX: a[0], AngleY: a[1], AngleZ: a[2]}
	if d := v.Depth(unit(axis)); d < 0.99 {
		t.Errorf("end left the axis at depth %.3f, want 1 (toward the viewer)", d)
	}
	r.mu.Lock()
	spin := r.value("auto-rotate")
	r.mu.Unlock()
	if spin != "0" {
		t.Errorf("auto-rotate is %q while a view is held", spin)
	}
	set(t, r, "turtle-view", "0")
	watch(t, r, c, 0.1)
	r.mu.Lock()
	spin = r.value("auto-rotate")
	r.mu.Unlock()
	if spin != "1" {
		t.Errorf("auto-rotate is %q after free, want it back", spin)
	}
}
