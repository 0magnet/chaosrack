package analysis

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// longRunFloor is the least exponent any window may read.
const longRunFloor = 0.005

// conservative flows have no attractor to contract onto: Sprott A, the
// Nosé–Hoover oscillator, fills a volume rather than collapsing onto a set.
var conservative = map[string]bool{"sprotta": true}

// Every flow, at its defaults, is chaotic as the equations are, not as one
// integrator makes it: under RK4, late in a long run as well as early, at
// its own step and at a quarter of it. An exponent that changes with the
// step belongs to the integrator, and one that falls away after the first
// window belongs to a transient; either way the page shows something other
// than a strange attractor, sooner or later.
func TestEveryFlowStaysChaotic(t *testing.T) {
	if testing.Short() {
		t.Skip("long runs")
	}
	for _, k := range dynamics.Keys() {
		for _, scale := range []float64{1, 0.25} {
			r := MeasureLongRun(k, scale)
			t.Logf("%-14s dt×%-4g λ %.3f  div %+.3f", k, scale, r.Lambda, r.Divergence)
			if !r.Chaotic(longRunFloor, !conservative[k]) {
				t.Errorf("%s at dt×%g: λ by window %.3f, divergence %+.3f — not chaotic for the whole run (an exponent below %g, or below 0.2 of the first), or not an attractor",
					k, scale, r.Lambda, r.Divergence, longRunFloor)
			}
		}
	}
}
