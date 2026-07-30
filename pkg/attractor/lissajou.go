//go:build js && wasm

package attractor

import "math"

var lissajouA, lissajouB, lissajouC float32 = 3, 2, 5

// The beam runs CONTINUOUSLY, like the attractor integrators and the xy
// scope: lissajouT is the persistent parameter time, advanced every frame at
// a rate the Speed knob scales, and the trail is the trailing window of the
// beam's path — index steps-1 is the beam head (newest), matching the
// attractors' convention so the gradient head visibly sweeps the figure.
// With integer a:b:c the curve is closed, so the FIGURE holds still while
// the beam runs it — exactly how a real scope behaves with locked ratios —
// and the slow relative-phase drift (lissajouPhase) precesses it on top,
// standing in for the detune that rotates a real scope's figure.
var lissajouT float64
var lissajouPhase float32

func generateLissajou() {
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	// The visible window spans a fixed 2 base periods (enough to always show
	// the whole closed figure); it no longer stretches with the Speed knob —
	// Speed now scales the BEAM RATE, like it scales the integrators.
	const cycles = 2
	delta := 2 * math.Pi * cycles / float64(steps)
	// Base period ≈ 2 s at speed 1 (sub-steps and dt-scale both speed it up).
	lissajouT += (2 * math.Pi / 120) * float64(speedScale) * float64(speedSteps)
	lissajouPhase += 0.004 * speedScale
	ph := float64(lissajouPhase)
	a, b, c := float64(lissajouA), float64(lissajouB), float64(lissajouC)
	for i := 0; i < steps; i++ {
		t := lissajouT - float64(steps-1-i)*delta
		j := i * 4
		vertices[j] = float32(math.Sin(a*t + ph))
		vertices[j+1] = float32(math.Sin(b * t))
		vertices[j+2] = float32(math.Sin(c*t + ph*0.5))
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}
