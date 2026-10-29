package attractor

import "testing"

// Takens and polar come up on audio through turbo, stereo on correlation
// through turbo, the spectrogram family on heat, and everything else where
// the rack rests.
func TestModelsComeUpOnTheirOwnColors(t *testing.T) {
	for _, c := range []struct {
		model string
		want  ModelColors
	}{
		{"takens", ModelColors{"4", "8"}},
		{"stereo", ModelColors{"7", "8"}},
		{"polar", ModelColors{"4", "8"}},
		{"spectrogram", ModelColors{"2", "5"}},
		{"fvf", ModelColors{"2", "5"}},
		{"lorenz", ModelColors{"2", "2"}},
		{"globe", ModelColors{"2", "2"}},
	} {
		if got := ModelColorDefault(c.model); got != c.want {
			t.Errorf("%s comes up on %+v, want %+v", c.model, got, c.want)
		}
	}
}

// A turned knob keeps the rings for the operator through every change of
// model, until Reset All gives them back.
func TestATurnedKnobKeepsTheRings(t *testing.T) {
	var f ColorFollow
	if c, ok := f.For("takens"); !ok || c != ModelColorDefault("takens") {
		t.Fatalf("untouched rings did not follow the model: %+v %v", c, ok)
	}
	f.Choose()
	if _, ok := f.For("lorenz"); ok {
		t.Fatal("a change of model moved rings the operator had turned")
	}
	f.Forget()
	if _, ok := f.For("lorenz"); !ok {
		t.Fatal("Reset All did not give the rings back")
	}
}
