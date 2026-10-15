package rackhost

import "github.com/0magnet/chaosrack/pkg/attractor"

// lissTrail is the Lissajous beam's trail now, as the page draws it
// (attractor/lissajou.go): run on by a frame at the Speed knob unless the
// rack is paused, Trail points long, framed to the whole closed figure fig.
// Called with s.mu held and not r.mu.
func (s *scene) lissTrail(fig attractor.Figure, knob func(string) float64, paused bool, run int, speed float64, dots bool) (attractor.Figure, [][3]float64) {
	s.liss.SetRatios(float32(knob("lissajou-a")), float32(knob("lissajou-b")), float32(knob("lissajou-c")))
	if !paused {
		sub, scale := speedSteps(speed)
		s.liss.Advance(float32(scale), sub)
	}
	n := max(run, 2)
	pts := make([][3]float64, n)
	for i := range pts {
		x, y, z := s.liss.At(i, n)
		pts[i] = [3]float64{float64(x), float64(y), float64(z)}
	}
	out := attractor.Figure{Kind: attractor.FigurePath, Points: pts}
	if dots {
		out.Kind = attractor.FigurePoints
	}
	return out, fig.Points
}

// running is fig as it runs this frame, and what frames it: a flow's live
// trail, the Lissajous beam, or fig as it stands. knobs reads the model's
// knobs, for the beam. Called with s.mu held and not r.mu.
func (s *scene) running(fig attractor.Figure, fit [][3]float64, key string, knobs func() func(string) float64, paused bool, run int, speed float64, dots bool) (attractor.Figure, [][3]float64) {
	switch {
	case key == "lissajou":
		return s.lissTrail(fig, knobs(), paused, run, speed, dots)
	case fig.Kind == attractor.FigurePath:
		return s.liveTrail(fig, fit, key, paused, run, speed, dots)
	}
	return fig, fit
}
