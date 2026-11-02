package attractor

import (
	"math"
	"testing"
)

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

// TestTheBandTableIsTheFormula: bands summed through the table are what the
// logarithm per bin made, bin for bin, at every rate and size in use, and
// switching between them picks the right table.
func TestTheBandTableIsTheFormula(t *testing.T) {
	formula := func(mags []float64, sr int) []float32 {
		nyq := float64(sr) / 2
		out := make([]float32, numEQBands)
		if nyq <= 30 {
			return out
		}
		logMin, logMax := math.Log(30), math.Log(nyq)
		for i, m := range mags {
			f := float64(i) / float64(len(mags)) * nyq
			if f < 30 {
				continue
			}
			b := min(max(int((math.Log(f)-logMin)/(logMax-logMin)*float64(numEQBands)), 0), numEQBands-1)
			out[b] += float32(m)
		}
		return out
	}
	for round := range 2 {
		for _, sr := range []int{48000, 44100, 24000, 60} {
			for _, n := range []int{513, 1025, 2049} {
				mags := make([]float64, n)
				for i := range mags {
					mags[i] = 1 + float64(i%7)
				}
				got, want := computeBands(mags, sr), formula(mags, sr)
				for b := range want {
					if got[b] != want[b] {
						t.Fatalf("round %d, %d Hz, %d bins: band %d = %v, want %v", round, sr, n, b, got[b], want[b])
					}
				}
			}
		}
	}
}
