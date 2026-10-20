package analysis

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// A positive exponent says chaotic, not ATTRACTING. An attractor also
// contracts volume: the flow's divergence, averaged along the trajectory, is
// negative. A conservative system — Sprott A, the Nosé–Hoover oscillator —
// has a positive exponent and no attractor, and passes a test of λ alone.

// meanDivergence is the divergence (the Jacobian's trace, by central
// differences) averaged along a trajectory stepped by adv, after a transient.
func meanDivergence(f dynamics.Deriv, adv func(dst, s []float64), ic []float64) float64 {
	s := append([]float64(nil), ic...)
	nx := make([]float64, 3)
	const h = 1e-6
	sum, n := 0.0, 0
	for k := range 40000 {
		adv(nx, s)
		s, nx = nx, s
		if k < 10000 {
			continue
		}
		x, y, z := s[0], s[1], s[2]
		a, _, _ := f(x+h, y, z)
		b, _, _ := f(x-h, y, z)
		_, c, _ := f(x, y+h, z)
		_, d, _ := f(x, y-h, z)
		_, _, e := f(x, y, z+h)
		_, _, g := f(x, y, z-h)
		sum += (a-b)/(2*h) + (c-d)/(2*h) + (e-g)/(2*h)
		n++
	}
	return sum / float64(n)
}

// stepper is how the app steps flow key, as LyapunovForFlow steps it.
func stepper(key string) (dynamics.Deriv, func(dst, s []float64), bool) {
	dt, f, ok := dynamics.FlowFor(key)
	if !ok {
		return nil, nil, false
	}
	euler := dynamics.IsClassic(key)
	return f, func(dst, s []float64) {
		if euler {
			dx, dy, dz := f(s[0], s[1], s[2])
			dst[0], dst[1], dst[2] = s[0]+dt*dx, s[1]+dt*dy, s[2]+dt*dz
			return
		}
		dst[0], dst[1], dst[2] = dynamics.RK4(f, dt, s[0], s[1], s[2])
	}, true
}

func TestTheAttractorsContractVolume(t *testing.T) {
	keys := dynamics.ClassicKeys()
	for _, c := range dynamics.SprottCases {
		keys = append(keys, c.Key)
	}
	for _, k := range keys {
		f, adv, ok := stepper(k)
		if !ok {
			t.Errorf("%s: not a registered flow", k)
			continue
		}
		ic := dynamics.InitCondFor(k)
		div := meanDivergence(f, adv, []float64{float64(ic[0]), float64(ic[1]), float64(ic[2])})
		t.Logf("%-14s mean divergence %+.4f", k, div)
		if !(div < -0.05) {
			t.Errorf("%s: mean divergence %+.4f — it does not contract volume, so it is not an attractor", k, div)
		}
	}
}

// Every whole position of the Sprott Morph is chaotic as the machine steps
// it (RK4, from that system's IC): it is what the rate knob plays.
func TestEverySprottMorphSystemIsChaotic(t *testing.T) {
	sys := dynamics.SprottMorphSystems()
	for i, s := range sys {
		c, dt, _, _, _ := dynamics.SprottMorphBlend(sys, float64(i))
		f := func(x, y, z float64) (float64, float64, float64) { return dynamics.EvalQuad(&c, x, y, z) }
		adv := func(dst, v []float64) { dst[0], dst[1], dst[2] = dynamics.RK4(f, dt, v[0], v[1], v[2]) }
		n := max(int(1.0/dt+0.5), 1)
		lam := lyapunovStep(adv, []float64{float64(s.IC[0]), float64(s.IC[1]), float64(s.IC[2])}, int(200.0/dt), n, 1500)
		t.Logf("%s λ ≈ %+.4f", s.Letter, lam)
		if Classify(lam) != "chaotic" {
			t.Errorf("morph system %s: λ %.4f reads %q", s.Letter, lam, Classify(lam))
		}
	}
}
