package dynamics

import (
	"slices"
	"testing"
)

// Sprott's search found B to E with five terms and two nonlinearities, and F
// to S with six terms and one; the anatomy has to read his catalog back.
func TestAnatomyReadsSprottsCatalog(t *testing.T) {
	for _, c := range SprottCases {
		a := c.Anatomy()
		terms, nl := 6, 1
		if c.Name <= "Sprott E" {
			terms, nl = 5, 2
		}
		if a.Terms != terms || len(a.Nonlinear) != nl {
			t.Errorf("%s: %d terms, nonlinear %v; want %d and %d", c.Name, a.Terms, a.Nonlinear, terms, nl)
		}
	}
}

func TestAnatomyOfSprottB(t *testing.T) {
	a := SprottCases[0].Anatomy()
	if !slices.Equal(a.Nonlinear, []string{"yz", "xy"}) || a.Div != [4]float64{-1, 0, 0, 0} || !a.HalfTurnZ {
		t.Errorf("Sprott B: %+v", a)
	}
	// K's divergence depends on y: dx/dt = xy − z gives ∂/∂x = y.
	k := SprottCases[9]
	if a := k.Anatomy(); k.Name != "Sprott K" || a.Div != [4]float64{-0.7, 0, 1, 0} || a.HalfTurnZ {
		t.Errorf("%s: %+v", k.Name, a)
	}
}
