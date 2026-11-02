package meters

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
		got := ComputeFFTMags(input)
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
	if ComputeFFTMags(make([]float32, 1000)) != nil {
		t.Error("non-power-of-two length must return nil")
	}
	if ComputeFFTMags(nil) != nil {
		t.Error("empty input must return nil")
	}
}

func BenchmarkComputeFFTMags(b *testing.B) {
	input := make([]float32, 1024)
	for i := range input {
		input[i] = float32(math.Sin(float64(i) * 0.1))
	}
	b.ReportAllocs()
	for range b.N {
		ComputeFFTMags(input)
	}
}

func BenchmarkUpstreamComputeFFT(b *testing.B) {
	input := make([]float32, 1024)
	for i := range input {
		input[i] = float32(math.Sin(float64(i) * 0.1))
	}
	b.ReportAllocs()
	for range b.N {
		sg.ComputeFFT(input)
	}
}

// TestAPairIsTwoTransforms: both windows' spectra out of one complex
// transform are each one's own, to rounding, at every bin from DC to Nyquist,
// for a pair as unlike as a tone and noise.
func TestAPairIsTwoTransforms(t *testing.T) {
	for _, n := range []int{64, 1024} {
		a, b := make([]float32, n), make([]float32, n)
		seed := uint32(777)
		for i := range a {
			seed = seed*1664525 + 1013904223
			a[i] = float32(math.Sin(2 * math.Pi * 37.3 * float64(i) / float64(n)))
			b[i] = float32(seed%20000)/10000 - 1
		}
		wantA := append([]float64(nil), ComputeFFTMags(a)...)
		wantB := append([]float64(nil), ComputeFFTMags(b)...)
		gotA, gotB := ComputeFFTMagsPair(a, b)
		peak := 0.0
		for i := range wantA {
			peak = max(peak, wantA[i], wantB[i])
		}
		for i := range wantA {
			if d := math.Abs(gotA[i] - wantA[i]); d > 1e-12*peak {
				t.Fatalf("n=%d bin %d: a %.12g, want %.12g", n, i, gotA[i], wantA[i])
			}
			if d := math.Abs(gotB[i] - wantB[i]); d > 1e-12*peak {
				t.Fatalf("n=%d bin %d: b %.12g, want %.12g", n, i, gotB[i], wantB[i])
			}
		}
	}
	if a, b := ComputeFFTMagsPair(make([]float32, 64), make([]float32, 32)); a != nil || b != nil {
		t.Error("windows of two lengths must return nil")
	}
}

func BenchmarkComputeFFTMagsTwice(b *testing.B) {
	l, r := make([]float32, sg.FFTSize), make([]float32, sg.FFTSize)
	for b.Loop() {
		ComputeFFTMags(l)
		ComputeFFTMags(r)
	}
}

func BenchmarkComputeFFTMagsPair(b *testing.B) {
	l, r := make([]float32, sg.FFTSize), make([]float32, sg.FFTSize)
	for b.Loop() {
		ComputeFFTMagsPair(l, r)
	}
}
