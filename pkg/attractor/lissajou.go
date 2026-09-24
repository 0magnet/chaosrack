//go:build js && wasm

package attractor

import "math"

// lissajousBeam is the Lissajous beam's running phase and its knobs.
type lissajousBeam struct {
	a, b, c float32

	// The beam runs CONTINUOUSLY, like the attractor integrators and the xy
	// scope: t is the persistent parameter time, advanced every frame at
	// a rate the Speed knob scales, and the trail is the trailing window of the
	// beam's path — index steps-1 is the beam head (newest), matching the
	// attractors' convention so the gradient head visibly sweeps the figure.
	// With integer a:b:c the curve is closed, so the FIGURE holds still while
	// the beam runs it — exactly how a real scope behaves with locked ratios —
	// and the slow relative-phase drift (phase) precesses it on top,
	// standing in for the detune that rotates a real scope's figure.
	t     float64
	phase float32
}

var liss = lissajousBeam{
	a: 3,
	b: 2,
	c: 5,
}

func (l *lissajousBeam) generateLissajou() {
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	// The visible window spans a fixed 2 base periods (enough to always show
	// the whole closed figure); it no longer stretches with the Speed knob —
	// Speed now scales the BEAM RATE, like it scales the integrators.
	const cycles = 2
	delta := 2 * math.Pi * cycles / float64(sim.steps)
	// Base period ≈ 2 s at speed 1 (sub-steps and dt-scale both speed it up).
	l.t += (2 * math.Pi / 120) * float64(sim.speedScale) * float64(sim.speedSteps)
	l.phase += 0.004 * sim.speedScale
	ph := float64(l.phase)
	a, b, c := float64(l.a), float64(l.b), float64(l.c)
	for i := 0; i < sim.steps; i++ {
		t := l.t - float64(sim.steps-1-i)*delta
		j := i * 4
		vertices[j] = float32(math.Sin(a*t + ph))
		vertices[j+1] = float32(math.Sin(b * t))
		vertices[j+2] = float32(math.Sin(c*t + ph*0.5))
		vertices[j+3] = float32(i) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}
