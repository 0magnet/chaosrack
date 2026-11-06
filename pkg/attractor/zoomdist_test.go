package attractor

import "testing"

// A far fit keeps the knob's units, so saved links mean what they did.
func TestAFarFitZoomsInUnits(t *testing.T) {
	if d := zoomedDist(150, 60); d != 90 {
		t.Errorf("fit 150 zoom 60 is %.1f units away, want 90", d)
	}
	if d := zoomedDist(150, -40); d != 190 {
		t.Errorf("fit 150 zoom -40 is %.1f units away, want 190", d)
	}
}

// A near fit is never zoomed through: every zoom in brings the camera nearer
// and leaves it in front of the model.
func TestANearFitIsNeverZoomedThrough(t *testing.T) {
	for _, fit := range []float32{5, 6, 20, 99} {
		last := zoomedDist(fit, -95)
		for z := float32(-90); z <= 95; z += 5 {
			d := zoomedDist(fit, z)
			if d <= 0 {
				t.Fatalf("fit %.0f zoom %.0f puts the camera %.2f units away: through the model", fit, z, d)
			}
			if d >= last {
				t.Fatalf("fit %.0f: zoom %.0f is no nearer than the step before (%.2f >= %.2f)", fit, z, d, last)
			}
			last = d
		}
	}
}
