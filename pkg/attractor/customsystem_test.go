package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// TestACustomFlowCanStepWithRK4: the Custom template, Lorenz, at a step
// forward Euler cannot take. Registered as Euler it diverges, no
// attractor at all; chosen RK4, it measures chaotic — which is the system
// the page then draws, and the registry's Euler mark is what tells the
// readouts so.
func TestACustomFlowCanStepWithRK4(t *testing.T) {
	c := NewCustomSystem()
	c.SetDt(0.03)
	c.Compile()
	defer dynamics.Unregister4(dynamics.CustomKey)
	if s, _ := dynamics.FlowFor4(dynamics.CustomKey); !s.Euler {
		t.Fatal("a Custom flow registered as RK4 before RK4 was chosen")
	}
	euler := analysis.LyapunovFor(dynamics.CustomKey)
	c.SetRK4(true)
	if s, _ := dynamics.FlowFor4(dynamics.CustomKey); s.Euler {
		t.Fatal("choosing RK4 left the Custom flow registered as Euler")
	}
	rk4 := analysis.LyapunovFor(dynamics.CustomKey)
	t.Logf("dt 0.03: euler %+v, rk4 %+v", euler, rk4)
	if rk4.Verdict != "chaotic" {
		t.Errorf("Lorenz under RK4 at dt 0.03 measured %q (λ %.3f), want chaotic", rk4.Verdict, rk4.Lambda)
	}
	if euler.Verdict != "diverged" {
		t.Errorf("Lorenz under Euler at dt 0.03 measured %q (λ %.3f), want diverged: the step no longer tells the integrators apart", euler.Verdict, euler.Lambda)
	}
}
