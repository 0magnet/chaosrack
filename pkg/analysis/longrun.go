package analysis

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// LongRun is what a flow does over a long run under RK4: whether it is still
// chaotic late, not only at first, and whether it is an attractor at all.
//
// One exponent over the first stretch is how the guards used to ask, and it
// passed systems the page then showed collapsing. Some flows are chaotic for
// thousands of time units and then settle — the Simone attractor's exponent
// is 1.1 to t≈2000 and 0.14 by t=3000, and what was a tangle on screen is a
// single loop. So the exponent is measured in windows spread along the run,
// as far as a gallery loop of it reaches, and the late ones are held to the
// early one.
type LongRun struct {
	// Lambda is the largest exponent in each window of LongRunWindows.
	Lambda []float64
	// Divergence is the mean of the field's divergence along the run: below
	// zero, the flow contracts volume, which an attractor does. A conservative
	// flow can have a positive exponent and no attractor at all.
	Divergence float64
	// OK is false when the run diverged or degenerated.
	OK bool
}

// LongRunWindows are where the exponent is measured, in time units, each
// LongRunSpan long: early, middle, and past the 9600 a gallery loop runs to.
var LongRunWindows = []float64{500, 3000, 9000}

// LongRunSpan is how long each window is.
const LongRunSpan = 300.0

// Min is the smallest of the windows' exponents.
func (l LongRun) Min() float64 {
	m := math.Inf(1)
	for _, v := range l.Lambda {
		m = min(m, v)
	}
	return m
}

// Chaotic reports whether the run is chaotic in every window, keeps a fifth
// of its early exponent at least, and, if it is to be an attractor, contracts
// volume: floor is the least exponent allowed in any window. A conservative
// flow (Sprott A, the Nosé–Hoover oscillator) is chaotic without one.
func (l LongRun) Chaotic(floor float64, attractor bool) bool {
	if !l.OK || len(l.Lambda) == 0 || attractor && !(l.Divergence < 0) {
		return false
	}
	return l.Min() >= floor && l.Min() >= 0.2*l.Lambda[0]
}

// MeasureLongRun runs flow key with RK4 at its own step times dtScale, from
// its initial condition, to the end of the last window.
func MeasureLongRun(key string, dtScale float64) LongRun {
	s, ok := dynamics.FlowFor4(key)
	if !ok {
		return LongRun{}
	}
	dt := s.Dt() * dtScale
	if dt <= 0 {
		return LongRun{}
	}
	ic := dynamics.InitCondFor(key)
	a := [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), s.W0}
	var b [4]float64
	step := func(v [4]float64) [4]float64 { return dynamics.RK4x4(s.F, dt, v) }
	finite := func(v [4]float64) bool {
		for _, x := range v {
			if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e6 {
				return false
			}
		}
		return true
	}
	renorm := max(int(1/dt+0.5), 1) // one time unit between renormalizations
	out := LongRun{OK: true}
	var divSum float64
	var divN int
	// The divergence, by central differences: sampled once a time unit over
	// the whole run past the first window's start, since a system whose
	// divergence is a near-cancellation (the Dequan Li's is −18.2 + 0.16·z)
	// reads either sign from the few hundred samples of one window.
	const h = 1e-6
	sampleDiv := func(v [4]float64) {
		var div float64
		for i := range 4 {
			p, m := v, v
			p[i] += h
			m[i] -= h
			var fp, fm [4]float64
			fp[0], fp[1], fp[2], fp[3] = s.F(p[0], p[1], p[2], p[3])
			fm[0], fm[1], fm[2], fm[3] = s.F(m[0], m[1], m[2], m[3])
			div += (fp[i] - fm[i]) / (2 * h)
		}
		divSum += div
		divN++
	}
	t := 0.0
	since := 0
	for _, start := range LongRunWindows {
		for ; t < start; t += dt {
			a = step(a)
			since++
			if since >= renorm && t >= LongRunWindows[0] {
				since = 0
				sampleDiv(a)
			}
		}
		if !finite(a) {
			return LongRun{}
		}
		b = a
		b[0] += lyapDefaultD0
		var sum float64
		n := 0
		for k := 0; float64(k) < LongRunSpan; k++ {
			for range renorm {
				a, b = step(a), step(b)
			}
			t += float64(renorm) * dt
			if !finite(a) || !finite(b) {
				return LongRun{}
			}
			var d2 float64
			for i := range a {
				d2 += (b[i] - a[i]) * (b[i] - a[i])
			}
			d := math.Sqrt(d2)
			if d == 0 {
				return LongRun{}
			}
			sum += math.Log(d / lyapDefaultD0)
			n++
			for i := range b {
				b[i] = a[i] + (b[i]-a[i])*lyapDefaultD0/d
			}
			sampleDiv(a)
		}
		out.Lambda = append(out.Lambda, sum/(float64(n)*float64(renorm)*dt))
	}
	out.Divergence = divSum / float64(divN)
	return out
}
