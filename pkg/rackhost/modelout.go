package rackhost

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// modelVoice is Model Out: the model as a signal generator, its x, y and z
// the outputs an analog computer solving the same equations would have, run
// at audio rate. The page's (attractor's sonify_js.go) ported for the models
// with equations; a parametric curve the page plays by scanning its drawn
// trail, and a solid by its beam, which are the page's still.
//
// SPD is integrator steps a sample, in semitones: 0 is one step a sample and
// every 12 up twice as fast. LVL is the level. Each output is centered and
// scaled to its own swing, because an attractor's coordinates are arbitrary
// in offset and size: Lorenz's z lives around 25.
type modelVoice struct {
	want         string  // the model chosen
	speed, level float64 // SPD as steps a sample, LVL as 0..1

	mode       string // the model the state was seeded for
	x, y, z, w float64
	px, py, pz float64 // the state a step ago, to interpolate between
	acc        float64 // fractional steps owed
	cen, span  [3]float64
}

// plays reports whether the chosen model has equations to play.
func (v *modelVoice) plays() bool {
	_, ok := dynamics.FlowFor4(v.want)
	return ok
}

// render fills out[c][:n] with n samples of x, y and z: silence for a model
// with no equations.
func (v *modelVoice) render(out *[3][]float32, n int) {
	for c := range out {
		if cap(out[c]) < n {
			out[c] = make([]float32, n)
		}
		out[c] = out[c][:n]
	}
	sys, ok := dynamics.FlowFor4(v.want)
	if !ok {
		for c := range out {
			clear(out[c])
		}
		return
	}
	v.flow(out, n, sys)
	v.normalize(out, n)
}

// flow integrates the model's own equations, speed steps a sample.
func (v *modelVoice) flow(out *[3][]float32, n int, sys dynamics.FlowSys4) {
	reseed := func() {
		ic := dynamics.InitCondFor(v.want)
		v.x, v.y, v.z = float64(ic[0]), float64(ic[1]), float64(ic[2])
		if v.x == 0 && v.y == 0 && v.z == 0 {
			v.x = 0.1 // not stranded at a fixed point
		}
		v.w = sys.W()
		v.px, v.py, v.pz = v.x, v.y, v.z
		v.acc = 0
	}
	if v.mode != v.want {
		v.mode = v.want
		reseed()
	}
	dt := sys.Dt()
	const lim = 1e5
	for i := range n {
		v.acc += v.speed
		for v.acc >= 1 {
			v.acc--
			v.px, v.py, v.pz = v.x, v.y, v.z
			dx, dy, dz, dw := sys.F(v.x, v.y, v.z, v.w)
			v.x += dt * dx
			v.y += dt * dy
			v.z += dt * dz
			v.w += dt * dw
			if !(math.Abs(v.x) < lim && math.Abs(v.y) < lim && math.Abs(v.z) < lim && math.Abs(v.w) < lim) {
				reseed()
			}
		}
		t := v.acc // between the previous state and this one
		out[0][i] = float32(v.px + (v.x-v.px)*t)
		out[1][i] = float32(v.py + (v.y-v.py)*t)
		out[2][i] = float32(v.pz + (v.z-v.pz)*t)
	}
}

// normalize centers each output on its swing and scales it to the level,
// the swing tracked fast when it grows and slowly when it shrinks, so a
// suddenly larger orbit does not clip and a quiet one is not pumped.
func (v *modelVoice) normalize(out *[3][]float32, n int) {
	for c := range out {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, s := range out[c][:n] {
			lo, hi = math.Min(lo, float64(s)), math.Max(hi, float64(s))
		}
		if hi > lo {
			mid, half := (hi+lo)/2, (hi-lo)/2
			v.cen[c] += (mid - v.cen[c]) * 0.05
			if half > v.span[c] {
				v.span[c] = half
			} else {
				v.span[c] += (half - v.span[c]) * 0.01
			}
		}
		span := math.Max(v.span[c], 1e-6)
		for i, s := range out[c][:n] {
			out[c][i] = float32(math.Max(-1, math.Min(1, (float64(s)-v.cen[c])*v.level/span)))
		}
	}
}
