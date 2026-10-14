package rackhost

import (
	"math"
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

// The bifurcation explorer sweeps the last flow shown: its diagram grows as
// it runs, across the swept parameter's range.
func TestTheBifurcationDiagramGrows(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "bifurcation")
	early := shownFigure(t, r, c, 0.2)
	later := shownFigure(t, r, c, 2)
	if len(later) <= len(early) || len(later) < 1000 {
		t.Fatalf("the diagram has %d points after 0.2 s and %d after 2 s", len(early), len(later))
	}
	lo, hi := later[0][0], later[0][0]
	for _, p := range later {
		lo, hi = min(lo, p[0]), max(hi, p[0])
	}
	if hi-lo < 10 {
		t.Errorf("the diagram spans %.1f..%.1f across: the sweep did not move", lo, hi)
	}
}

// The Poincaré model is the last flow's crossings of its plane: in place,
// all at the plane's z; face on, flat; as a return map, with its guide.
func TestThePoincareSection(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "poincare")
	pts := shownFigure(t, r, c, 1)
	if len(pts) < 100 {
		t.Fatalf("the section has %d crossings", len(pts))
	}
	for _, p := range pts {
		if math.Abs(p[2]-pts[0][2]) > 1e-3 {
			t.Fatalf("a crossing at z %.3f, another at %.3f: not one plane", p[2], pts[0][2])
		}
	}
	set(t, r, "sect-view", "1")
	for _, p := range shownFigure(t, r, c, 0.1) {
		if p[2] != 0 {
			t.Fatalf("face on, a crossing is off the plane: %v", p)
		}
	}
	set(t, r, "sect-view", "2")
	if m := shownFigure(t, r, c, 0.1); len(m) < 64 {
		t.Errorf("the return map is %d points", len(m))
	}
}

// LINE S draws the section over a running flow, in gold.
func TestLineSOverlaysTheSection(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	set(t, r, "sect-sw", "1")
	watch(t, r, c, 1)
	img, err := r.Scene(160, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	gold := 0
	for i := 0; i+2 < len(img.Px); i += 3 {
		if near(img.Px[i:i+3], [3]uint8{255, 204, 38}) {
			gold++
		}
	}
	if gold < 10 {
		t.Errorf("LINE S drew %d gold pixels", gold)
	}
}

// SWEEP is the last flow's parameters: the diagram sweeps the one it is on.
func TestTheBifurcationSweepsWhatSweepPicks(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "bifurcation")
	watch(t, r, c, 0.1)
	r.mu.Lock()
	sw := r.ctls[r.byID["bif-sweep"]]
	r.mu.Unlock()
	if len(sw.Labels) != 4 || sw.Labels[2] != "ρ" {
		t.Fatalf("SWEEP offers %v on Lorenz", sw.Labels)
	}
	set(t, r, "bif-sweep", "2")
	pts := shownFigure(t, r, c, 1)
	if len(pts) < 100 {
		t.Errorf("sweeping ρ drew %d points", len(pts))
	}
}

// The Turtle walks natively: its figure is drawn with the tinter's colors,
// and CYCLE steps the modulus on, turning the MOD knob.
func TestTheTurtleWalks(t *testing.T) {
	r, c := idleRack(t, "turtle")
	watch(t, r, c, 0.5)
	r.scene.mu.Lock()
	n, ages := len(r.scene.fig.Points), append([]float32(nil), r.scene.fig.Ages...)
	r.scene.mu.Unlock()
	if n < 20 || len(ages) != n {
		t.Fatalf("the Turtle drew %d points with %d tints", n, len(ages))
	}
	tints := map[float32]bool{}
	for _, a := range ages {
		tints[a] = true
	}
	if len(tints) < 2 {
		t.Errorf("the walk is one tint: %v", ages[:3])
	}
	set(t, r, "turtle-cycle", "1")
	watch(t, r, c, 2.2)
	if got := r.value("turtle-mod"); got == "25" {
		t.Error("CYCLE did not step the modulus")
	}
}
