package attractor

import "testing"

// The key labels each source by what its ends mean: fixed marks for the
// absolute ones, the live span for the auto-ranged ones, and the axis by name.
func TestTheColorKeySaysWhatTheColorsMean(t *testing.T) {
	ranged := AudioColor{ranged: true, rlo: 0.05, rhi: 0.25, sr: 24000}
	flat := AudioColor{}
	for _, c := range []struct {
		name  string
		src   int
		model string
		axes  int
		a     *AudioColor
		want  colorKey
	}{
		{"corr", gradientSourceCorr, "stereo", 0, &flat, colorKey{"L/R correlation", "−1 inverted", "0", "+1 same"}},
		{"balance", gradientSourceBalance, "stereo", 0, &flat, colorKey{"balance", "R", "center", "L"}},
		{"dB", gradientSourceDB, "takens", 0, &flat, colorKey{"level", "−60 dB", "−30", "0 dB"}},
		{"centroid live", gradientSourceAudio, "takens", 0, &ranged, colorKey{"spectral centroid", "600 Hz", "", "3.0 kHz"}},
		{"centroid flat", gradientSourceAudio, "lorenz", 0, &flat, colorKey{"spectral centroid", "darker", "", "brighter"}},
		{"Z on Takens", 2, "takens", 0, &flat, colorKey{"Z · s(t−2τ)", "low", "", "high"}},
		{"Z on a goniometer", 2, "stereo", 1, &flat, colorKey{"Z · time", "older", "", "newer"}},
		{"Z on Lorenz", 2, "lorenz", 0, &flat, colorKey{"Z", "low", "", "high"}},
		{"trail", 3, "lorenz", 0, &flat, colorKey{"trail age", "older", "", "newer"}},
		{"corr on one channel", gradientSourceCorr, "takens", 0, &flat, colorKey{"L/R correlation", "", "needs two channels", ""}},
	} {
		got, ok := colorKeyFor(c.src, c.model, c.axes, c.a)
		if !ok || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.name, got, ok, c.want)
		}
	}
	if _, ok := colorKeyFor(GradientSourceOff, "lorenz", 0, &flat); ok {
		t.Error("OFF has a key")
	}
}

// A ranged level is labeled in dBFS of the span it is mapped across.
func TestTheKeyLabelsALevelInDecibels(t *testing.T) {
	a := AudioColor{ranged: true, rlo: 0.01, rhi: 0.5}
	k, _ := colorKeyFor(gradientSourceLevel, "takens", 0, &a)
	if k.Lo != "−40 dB" || k.Hi != "−6 dB" {
		t.Errorf("level key %q..%q, want −40 dB..−6 dB", k.Lo, k.Hi)
	}
}
