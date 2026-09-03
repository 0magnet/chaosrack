package attractor

import (
	"math"
	"testing"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The local allocation-free FFT must be a drop-in for the upstream
// ComputeFFT (Hann window, magnitudes of the first n/2 bins).
func TestComputeFFTMagsMatchesUpstream(t *testing.T) {
	for _, n := range []int{64, 1024} {
		input := make([]float32, n)
		seed := uint32(12345)
		for i := range input {
			seed = seed*1664525 + 1013904223
			input[i] = float32(seed%20000)/10000 - 1
		}
		want := sg.ComputeFFT(input)
		got := computeFFTMags(input)
		if len(got) != len(want) {
			t.Fatalf("n=%d: len %d, want %d", n, len(got), len(want))
		}
		for i := range want {
			if d := math.Abs(got[i] - want[i]); d > 1e-9*(1+math.Abs(want[i])) {
				t.Fatalf("n=%d bin %d: got %.12g want %.12g (Δ=%g)", n, i, got[i], want[i], d)
			}
		}
	}
}

func TestComputeFFTMagsRejectsNonPow2(t *testing.T) {
	if computeFFTMags(make([]float32, 1000)) != nil {
		t.Error("non-power-of-two length must return nil")
	}
	if computeFFTMags(nil) != nil {
		t.Error("empty input must return nil")
	}
}

func BenchmarkComputeFFTMags(b *testing.B) {
	input := make([]float32, 1024)
	for i := range input {
		input[i] = float32(math.Sin(float64(i) * 0.1))
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		computeFFTMags(input)
	}
}

func BenchmarkUpstreamComputeFFT(b *testing.B) {
	input := make([]float32, 1024)
	for i := range input {
		input[i] = float32(math.Sin(float64(i) * 0.1))
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sg.ComputeFFT(input)
	}
}
