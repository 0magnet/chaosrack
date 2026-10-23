package dynamics

import (
	"math"
	"testing"
)

// TestMoreFlowsStayOnTheirAttractors: a system can look bounded and chaotic
// for thousands of time units and then leave — Sakarya's two scrolls did,
// after about thirty thousand, for a regime thirty times the size, and the
// page, integrating a whole trail a frame, got there in under half a minute
// and drew a line running off the screen. Each system here stays within
// three times the reach of its first two thousand time units for fifty
// thousand, from its initial condition, at its default step.
func TestMoreFlowsStayOnTheirAttractors(t *testing.T) {
	for _, f := range MoreFlows {
		h := float64(*f.DT)
		x, y, z := float64(f.IC[0]), float64(f.IC[1]), float64(f.IC[2])
		var early, late float64
		for i := range int(50000 / h) {
			x, y, z = RK4(f.Deriv, h, x, y, z)
			r := math.Max(math.Abs(x), math.Max(math.Abs(y), math.Abs(z)))
			if !(r < 1e6) {
				t.Errorf("%s escaped at t=%.0f", f.Key, float64(i)*h)
				break
			}
			if float64(i)*h < 2000 {
				early = math.Max(early, r)
			} else {
				late = math.Max(late, r)
			}
		}
		if late > 3*early {
			t.Errorf("%s: reached %.1f after t=2000, against %.1f before", f.Key, late, early)
		}
	}
}
