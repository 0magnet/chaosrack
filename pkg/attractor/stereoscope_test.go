package attractor

import (
	"math"
	"slices"
	"testing"
)

// stereoTone is n samples of a tone of period samples on both channels.
func stereoTone(n int, period float64) (l, r []float32) {
	l, r = make([]float32, n), make([]float32, n)
	for i := range l {
		l[i] = float32(0.5 * math.Sin(2*math.Pi*float64(i)/period+1))
		r[i] = l[i]
	}
	return l, r
}

// A frame's start is the window it drew: what the color walks read to color
// the trail by the audio under it. Triggered on a rising edge, the first
// vertex is the sample just past the crossing.
func TestStereoFrameStartsWhereItDrew(t *testing.T) {
	s := NewStereoScope()
	s.trig = stereoTrigRising
	const sr, budget, smooth = 48000, 20000, 4
	snap, nv := s.Window(sr, budget, smooth)
	l, r := stereoTone(snap, 109.3)
	v := make([]float32, 4*nv)
	if !s.Frame(l, r, sr, budget, smooth, v) {
		t.Fatal("auto mode held a frame")
	}
	f := s.frame(sr, budget, smooth)
	if s.start == f.margin {
		t.Fatal("the trigger found no edge in a steady tone")
	}
	i := f.baseL + s.start + f.tau
	if !(l[i-1] < 0 && l[i] >= 0) {
		t.Errorf("the window starts at %v after %v, not at a rising crossing", l[i], l[i-1])
	}
	if want := l[i] * s.gain * s.vgain; v[0] != want {
		t.Errorf("the first vertex is %v; the sample at the start is %v", v[0], want)
	}
	s.trig = stereoTrigOff
	s.Frame(l, r, sr, budget, smooth, v)
	if s.start != 0 {
		t.Errorf("untriggered, the window starts %d past the base", s.start)
	}
}

// Every knob in StereoKnobs is one Set takes, the trigger's switches among
// them, so a native rack and a linked view can turn each.
func TestStereoSetTakesEveryKnob(t *testing.T) {
	s := NewStereoScope()
	for _, id := range StereoKnobs {
		if !s.Set(id, 0.5) {
			t.Errorf("Set does not take %s", id)
		}
	}
	for _, id := range []string{"stereo-tsrc", "stereo-tcpl", "stereo-trun"} {
		if !slices.Contains(StereoKnobs, id) || s.field(id) == nil {
			t.Errorf("%s is not a knob of the scope", id)
		}
	}
	if s.Set("takens-tau", 1) {
		t.Error("Set took another model's knob")
	}
}

// The graticule is the lines at the signal's extent: both axes, and in
// full the diagonals and the box.
func TestStereoGraticuleFigure(t *testing.T) {
	s := NewStereoScope()
	if g := s.Graticule(); len(g.Edges) != 0 {
		t.Errorf("the grid off draws %d lines", len(g.Edges)/2)
	}
	s.grat, s.vgain = gratAxes, 2
	if g := s.Graticule(); len(g.Edges) != 4 || g.Points[0][0] != -20 {
		t.Errorf("the axes are %d endpoints from %v; want 4 from -20", len(g.Edges), g.Points[0])
	}
	s.grat = gratFull
	if g := s.Graticule(); len(g.Edges) != 16 || int(g.Edges[15]) != len(g.Points)-1 {
		t.Errorf("the full grid is %d endpoints over %d points", len(g.Edges), len(g.Points))
	}
}
