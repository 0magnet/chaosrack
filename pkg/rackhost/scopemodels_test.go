package rackhost

import (
	"testing"
)

// shownFigure is the figure the scene drew last.
func shownFigure(t *testing.T, r *Rack, c *clock, secs float64) [][3]float64 {
	t.Helper()
	watch(t, r, c, secs)
	r.scene.mu.Lock()
	defer r.scene.mu.Unlock()
	return append([][3]float64(nil), r.scene.fig.Points...)
}

// The Graphic Artist drifts as it runs, and its harmonics reshape it.
func TestTheGraphicArtistDriftsAndTakesItsKnobs(t *testing.T) {
	r, c := idleRack(t, "graphicartist")
	a := shownFigure(t, r, c, 0.1)
	b := shownFigure(t, r, c, 0.5)
	if len(a) == 0 || a[0] == b[0] {
		t.Fatal("the Graphic Artist did not drift")
	}
	set(t, r, "pause-sw", "1")
	held := shownFigure(t, r, c, 0.1)
	set(t, r, "ga-hc", "5")
	reshaped := shownFigure(t, r, c, 0.1)
	if held[len(held)/3] == reshaped[len(reshaped)/3] {
		t.Error("HmC at 5 drew the same wireframe")
	}
}

// The scope clock draws a face, flat on the screen.
func TestTheScopeClockDrawsAFace(t *testing.T) {
	r, c := idleRack(t, "scopeclock")
	pts := shownFigure(t, r, c, 0.1)
	if len(pts) < 100 {
		t.Fatalf("the clock drew %d points", len(pts))
	}
	for _, p := range pts {
		if p[2] != 0 {
			t.Fatalf("the clock's face is not flat: %v", p)
		}
	}
}

// Fourier Text draws its banner a stroke at a time, never joining one stroke
// to the next, and its harmonics knob changes the glyphs.
func TestFourierTextDrawsItsBanner(t *testing.T) {
	r, c := idleRack(t, "scopetext")
	watch(t, r, c, 0.1)
	r.scene.mu.Lock()
	n, edges := len(r.scene.fig.Points), len(r.scene.fig.Edges)/2
	first := r.scene.fig.Points[n/2]
	r.scene.mu.Unlock()
	if n < 1000 || edges == 0 || edges >= n-1 {
		t.Fatalf("the banner is %d points joined by %d lines: want strokes apart", n, edges)
	}
	set(t, r, "stext-harm", "4")
	watch(t, r, c, 0.1)
	r.scene.mu.Lock()
	again := r.scene.fig.Points[len(r.scene.fig.Points)/2]
	r.scene.mu.Unlock()
	if again == first {
		t.Error("4 harmonics drew the same glyphs as 24")
	}
}
