package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

import (
	"image"
	"testing"
)

// Every model that can be drawn without a browser must draw SOMETHING.
//
// This is the check TestEveryDynamicalModeMeasuresChaotic cannot make. That
// one asks each mode for a Lyapunov exponent, and a mode whose flow is not a
// registered deriv answers "n/a" and is skipped — which is exactly how the
// Sprott morph came to draw a single motionless point at ten of its nineteen
// positions without a test noticing. An exponent needs a vector field; an
// extent needs only the trajectory, so nothing can opt out of it.
func TestEveryFlowDrawsSomething(t *testing.T) {
	keys := dynamics.Keys()
	if len(keys) < 20 {
		t.Fatalf("only %d flows registered; the registry is probably not loaded", len(keys))
	}
	for _, k := range keys {
		pts := dynamics.Trajectory(k, dynamics.DefaultTrajectory())
		if len(pts) == 0 {
			t.Errorf("%s: integrated to nothing — the trajectory diverged", k)
			continue
		}
		dx, dy, dz := Extent(pts)
		if dx == 0 && dy == 0 && dz == 0 {
			t.Errorf("%s: the trajectory has no extent — a fixed point, not an attractor", k)
		}
	}
}

// A trail is a polyline, so the renderer needs an edge between each
// consecutive pair. Handing it nil draws a correct picture of no edges — a
// background and nothing else — which is indistinguishable from a dead model.
func TestTrailIndicesJoinConsecutivePoints(t *testing.T) {
	got := TrailIndices(4)
	want := []uint16{0, 1, 1, 2, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if TrailIndices(1) != nil || TrailIndices(0) != nil {
		t.Error("a trail of fewer than two points has no segments")
	}
}

// The index type is uint16, so a long trail cannot be joined end to end.
// Stopping is right; wrapping would draw a line from the end of the attractor
// back to its start, straight through the middle of the picture.
func TestTrailIndicesDoNotWrapAround(t *testing.T) {
	idx := TrailIndices(90000)
	for _, v := range idx {
		_ = v // a uint16 cannot exceed its own range; the count is the check
	}
	if len(idx) != (65535-1)*2 {
		t.Errorf("got %d indices for 90000 points, want %d", len(idx), (65535-1)*2)
	}
}

// Centering is what makes the fit usable: the renderer sizes the picture by
// the furthest point from the ORIGIN, and an attractor is not modeled around
// its own middle — Lorenz lives around z = 27.
func TestCenteredPutsTheBoxOnTheOrigin(t *testing.T) {
	pts := [][3]float64{{10, 20, 30}, {20, 40, 60}}
	got := Centered(pts)
	for a := 0; a < 3; a++ {
		if got[0][a] != -got[1][a] {
			t.Errorf("axis %d: %v and %v are not symmetric about 0", a, got[0][a], got[1][a])
		}
	}
	if Centered(nil) != nil {
		t.Error("centering nothing should give nothing")
	}
}

// The shift must not change the colors, or recentering would repaint the
// model: the gradient normalizes against the trajectory's own min and max,
// and moving both ends equally leaves every point where it was between them.
func TestCenteringDoesNotMoveAPointWithinItsRange(t *testing.T) {
	pts := [][3]float64{{10, 0, 0}, {14, 0, 0}, {20, 0, 0}}
	c := Centered(pts)
	frac := func(p [][3]float64, i int) float64 {
		return (p[i][0] - p[0][0]) / (p[2][0] - p[0][0])
	}
	if frac(pts, 1) != frac(c, 1) {
		t.Errorf("the middle point moved from %v to %v of the range", frac(pts, 1), frac(c, 1))
	}
}

// Draw must paint a background. rasterview deliberately does not clear its
// destination, and a fresh RGBA is transparent black — so a caller that
// forgets produces a picture that is invisible wherever transparency shows as
// white, which is the same picture a dead model draws.
func TestDrawPaintsABackground(t *testing.T) {
	img := Draw([][3]float64{{0, 0, 0}, {1, 1, 1}}, DrawOptions{Width: 16, Height: 16})
	if got := img.RGBAAt(0, 0); got.A != 255 {
		t.Errorf("the corner is %+v: the background was not painted", got)
	}
}

func TestDrawIsTheRequestedSize(t *testing.T) {
	img := Draw([][3]float64{{0, 0, 0}, {1, 1, 1}}, DrawOptions{Width: 40, Height: 25})
	if got, want := img.Bounds(), image.Rect(0, 0, 40, 25); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
