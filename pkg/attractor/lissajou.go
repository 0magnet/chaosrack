package attractor

import "math"

// LissajousBeam is the Lissajous model's beam: its ratios, and where it has
// run to. Shared by the page (lissajou_js.go, which draws it) and a native
// rack.
//
// The beam runs CONTINUOUSLY, like the attractor integrators and the xy
// scope: t is the persistent parameter time, advanced every frame at a rate
// the Speed knob scales, and the trail is the trailing window of the beam's
// path — index steps-1 is the beam head (newest), matching the attractors'
// convention so the gradient head visibly sweeps the figure. With integer
// a:b:c the curve is closed, so the FIGURE holds still while the beam runs
// it — exactly how a real scope behaves with locked ratios — and the slow
// relative-phase drift (phase) precesses it on top, standing in for the
// detune that rotates a real scope's figure.
type LissajousBeam struct {
	a, b, c float32
	t       float64
	phase   float32
}

// NewLissajousBeam is the beam at the page's 3:2:5.
func NewLissajousBeam() LissajousBeam { return LissajousBeam{a: 3, b: 2, c: 5} }

// SetRatios sets the beam's a:b:c, its knobs.
func (l *LissajousBeam) SetRatios(a, b, c float32) { l.a, l.b, l.c = a, b, c }

// Advance runs the beam on by one of the page's frames at the Speed knob's
// sub-steps and step scale.
func (l *LissajousBeam) Advance(speedScale float32, speedSteps int) {
	// Base period ≈ 2 s at speed 1 (sub-steps and dt-scale both speed it up).
	l.t += (2 * math.Pi / 120) * float64(speedScale) * float64(speedSteps)
	l.phase += 0.004 * speedScale
}

// At is point i of a trail steps long, its head at steps-1. The visible
// window spans a fixed 2 base periods (enough to always show the whole
// closed figure); Speed scales the beam's rate, not the window.
func (l *LissajousBeam) At(i, steps int) (x, y, z float32) {
	const cycles = 2
	delta := 2 * math.Pi * cycles / float64(steps)
	ph := float64(l.phase)
	a, b, c := float64(l.a), float64(l.b), float64(l.c)
	t := l.t - float64(steps-1-i)*delta
	return float32(math.Sin(a*t + ph)), float32(math.Sin(b * t)), float32(math.Sin(c*t + ph*0.5))
}
