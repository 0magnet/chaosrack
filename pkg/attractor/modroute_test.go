package attractor

import "testing"

// The wave shape is the signal itself, either side of the setting, where the
// other shapes read an envelope.
func TestTheWaveShapeFollowsTheSignalBothWays(t *testing.T) {
	var s ModShaper
	for _, c := range []struct{ in, want float32 }{{0.5, 0.5}, {-0.25, -0.25}, {3, 1}, {-3, -1}} {
		if got := s.Signal(ModShapeWave, "L", c.in, 1); got != c.want {
			t.Errorf("wave of %v = %v, want %v", c.in, got, c.want)
		}
	}
	// A coordinate is centered, as under every shape.
	if got := s.Signal(ModShapeWave, modSrcModelX, 0.75, 1); got != 0.5 {
		t.Errorf("wave of head x 0.75 = %v, want 0.5", got)
	}
	if !IsModShape(ModShapeWave) {
		t.Error("a link cannot store the wave shape")
	}
}

// ModWave holds each channel's newest sample.
func TestModWaveIsTheNewestSample(t *testing.T) {
	var w ModWave
	if w.Value("L") != 0 {
		t.Error("a channel never set is not silent")
	}
	w.Set("L", Newest([]float32{0.1, 0.2, -0.7}))
	if got := w.Value("L"); got != -0.7 {
		t.Errorf("L = %v, want the last sample, -0.7", got)
	}
	if Newest(nil) != 0 {
		t.Error("an empty window is not silent")
	}
}
