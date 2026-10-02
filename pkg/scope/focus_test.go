package scope

import (
	"math"
	"testing"
)

func TestTheBeamDefocusesEitherSideOfBestFocus(t *testing.T) {
	if d := Defocus(FocusBest); d != 0 {
		t.Errorf("Defocus at best focus = %v, want 0", d)
	}
	for _, off := range []float64{0.1, 0.25, 0.5} {
		lo, hi := Defocus(FocusBest-off), Defocus(FocusBest+off)
		if lo <= 0 || math.Abs(lo-hi) > 1e-12 {
			t.Errorf("±%v from best focus: %v and %v, want the same spread, above zero", off, lo, hi)
		}
	}
	if Defocus(0) != 1 || Defocus(1) != 1 {
		t.Errorf("the ends of the travel: %v, %v, want 1", Defocus(0), Defocus(1))
	}
}
