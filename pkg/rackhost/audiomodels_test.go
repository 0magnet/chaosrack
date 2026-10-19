package rackhost

import (
	"math"
	"testing"
)

// toneRack is a rack drawing model from generator 1 alone, on both RACK
// rows: the same tone on left and right.
func toneRack(t *testing.T, model string) (*Rack, *clock) {
	t.Helper()
	r, c := idleRack(t, model)
	set(t, r, "gen-x-wave", "0", "mix-rl-cl", "0", "mix-rr-cr", "0", "mix-rl-g1", "1", "mix-rr-g1", "1", "gen-x-lvl", "60")
	looks(t, r, c, 0.3)
	return r, c
}

// spans is how far a figure's points reach on each axis.
func spans(pts [][3]float64) (s [3]float64) {
	for _, p := range pts {
		for a := range 3 {
			s[a] = math.Max(s[a], math.Abs(p[a]))
		}
	}
	return s
}

// The XY's AXES knob turns the scope to mid and side: the same tone on both
// channels is the diagonal on L, R and lies flat along mid.
func TestTheXYsAxesKnobTurnsItToMidAndSide(t *testing.T) {
	r, _ := toneRack(t, "xy")
	fig, _, _ := r.audioFigure("xy")
	for _, p := range fig.Points {
		if math.Abs(p[0]-p[1]) > 1e-6 {
			t.Fatalf("one tone on L and R strays off the diagonal at %v", p)
		}
	}
	set(t, r, "xy-basis", "1")
	fig, _, _ = r.audioFigure("xy")
	if s := spans(fig.Points); s[0] < 0.3 || s[1] > 1e-6 {
		t.Errorf("on mid and side the tone reaches %.3f on mid and %.3f on side; want it all on mid", s[0], s[1])
	}
}

// The smoothing knobs set how many steps the beam takes between samples.
func TestTheSmoothingKnobsSetTheBeamsSteps(t *testing.T) {
	r, _ := toneRack(t, "xy")
	set(t, r, "xy-smooth", "1")
	one, _, _ := r.audioFigure("xy")
	set(t, r, "xy-smooth", "8")
	eight, _, _ := r.audioFigure("xy")
	if len(eight.Points) != 8*len(one.Points) {
		t.Errorf("the XY draws %d points at smoothing 1 and %d at 8", len(one.Points), len(eight.Points))
	}
	set(t, r, r.selector, "takens", "takens-smooth", "1")
	one, _, _ = r.audioFigure("takens")
	set(t, r, "takens-smooth", "16")
	sixteen, _, _ := r.audioFigure("takens")
	if len(sixteen.Points) <= len(one.Points) {
		t.Errorf("Takens draws %d points at smoothing 1 and %d at 16", len(one.Points), len(sixteen.Points))
	}
}

// Takens' SRC knob picks the channel it embeds: the side of a tone on both
// channels is silence.
func TestTakensEmbedsTheChannelItsSourceKnobPicks(t *testing.T) {
	r, _ := toneRack(t, "takens")
	fig, _, _ := r.audioFigure("takens")
	if s := spans(fig.Points); s[0] < 0.3 {
		t.Fatalf("the mix of the tone reaches only %.3f", s[0])
	}
	set(t, r, "takens-chan", "4") // side
	fig, _, _ = r.audioFigure("takens")
	if s := spans(fig.Points); s[0] > 1e-6 {
		t.Errorf("the side of one tone on both channels reaches %.3f", s[0])
	}
}

// Polar's MAP knob bends the vector's length: the unit map puts every point
// on the sphere.
func TestPolarsMapKnobBendsTheLength(t *testing.T) {
	r, _ := toneRack(t, "polar")
	set(t, r, "polar-map", "3") // unit
	fig, _, _ := r.audioFigure("polar")
	for _, p := range fig.Points {
		if n := math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2]); n > 1e-3 && math.Abs(n-1) > 1e-3 {
			t.Fatalf("the unit map drew a point %.4f from the center", n)
		}
	}
}

// The Stereo Embedding's trigger starts every frame at the tone's rising
// crossing, so the figure stands still; untriggered it starts wherever the
// newest sample left it.
func TestTheStereoTriggerHoldsAToneStill(t *testing.T) {
	first := func(r *Rack, c *clock) (xs []float64) {
		for range 6 {
			looks(t, r, c, 0.05)
			fig, _, _ := r.audioFigure("stereo")
			if len(fig.Points) == 0 {
				t.Fatal("the stereo embedding drew nothing")
			}
			xs = append(xs, fig.Points[0][0])
		}
		return xs
	}
	r, c := toneRack(t, "stereo")
	free := first(r, c)
	moved := false
	for _, x := range free {
		moved = moved || math.Abs(x) > 1
	}
	if !moved {
		t.Errorf("untriggered, every frame began near zero: %.2f", free)
	}
	set(t, r, "stereo-trig", "1")
	for _, x := range first(r, c) {
		// One sample's climb at the crossing, at gain 10.
		if math.Abs(x) > 0.3 {
			t.Errorf("triggered on the rising edge, a frame began at %.2f", x)
		}
	}
}

// SINGLE catches one frame and keeps it, until the run knob moves.
func TestTheStereoSingleShotHoldsItsFrame(t *testing.T) {
	r, c := toneRack(t, "stereo")
	set(t, r, "stereo-trig", "1", "stereo-trun", "2")
	caught, _, _ := r.audioFigure("stereo")
	looks(t, r, c, 0.137)
	held, _, _ := r.audioFigure("stereo")
	if len(held.Points) != len(caught.Points) || held.Points[len(held.Points)/2] != caught.Points[len(caught.Points)/2] {
		t.Error("the single shot did not hold its frame")
	}
	if g, _, _ := r.audioFigure("stereo"); &g.Points[0] != &held.Points[0] {
		t.Error("the single shot drew a new frame")
	}
}

// The GRID knob puts the graticule up, in its gray, behind the trace.
func TestTheStereoGraticule(t *testing.T) {
	r, _ := toneRack(t, "stereo")
	if _, grat, _ := r.audioFigure("stereo"); len(grat.Edges) != 0 {
		t.Fatal("a graticule with the grid off")
	}
	set(t, r, "stereo-grat", "2")
	_, grat, fit := r.audioFigure("stereo")
	if len(grat.Edges) != 16 || grat.Points[0][0] != fit[0][0] {
		t.Errorf("the full graticule is %d endpoints from %v; want 16 at the gain, %v", len(grat.Edges), grat.Points[0], fit[0])
	}
	img, err := r.Scene(120, 80, 1)
	if err != nil {
		t.Fatal(err)
	}
	gray := 0
	for i := 0; i+2 < len(img.Px); i += 3 {
		if near(img.Px[i:i+3], [3]uint8{56, 66, 82}) {
			gray++
		}
	}
	if gray < 20 {
		t.Errorf("the scene shows %d pixels of graticule", gray)
	}
}

// The XY's GLOW keeps as much of the last frame as the page does at 60
// frames a second, whatever the panel's rate.
func TestTheXYsGlowFadesByTime(t *testing.T) {
	if k := glowKeep(0.9, 1.0/60); math.Abs(k-0.9) > 1e-9 {
		t.Errorf("a 60th of a second keeps %.4f, want 0.9", k)
	}
	if k := glowKeep(0.9, 1.0/30); math.Abs(k-0.81) > 1e-9 {
		t.Errorf("a 30th of a second keeps %.4f, want 0.81", k)
	}
	if glowKeep(0, 1) != 0 || glowKeep(1, 1.0/60) != 0.98 {
		t.Error("GLOW off keeps something, or full keeps more than 0.98")
	}
}

// near reports whether a pixel is within 2 of c on every channel.
func near(p []uint8, c [3]uint8) bool {
	for i := range c {
		if d := int(p[i]) - int(c[i]); d < -2 || d > 2 {
			return false
		}
	}
	return true
}
