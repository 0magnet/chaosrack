package meters

import (
	"math"
	"math/rand"
	"testing"
)

// truePeakFull is the unskipped pass — every interior position filtered at
// every phase. The optimized TruePeak must agree with it exactly, because
// the bound it skips on is a real bound and not an approximation.
func truePeakFull(x []float32) float64 {
	if len(x) == 0 {
		return 0
	}
	peak := 0.0
	for _, v := range x {
		if a := math.Abs(float64(v)); a > peak {
			peak = a
		}
	}
	if len(x) <= 2*truePeakTaps {
		return peak
	}
	for p := 1; p < truePeakOversample; p++ {
		k := truePeakKernel(p)
		for i := truePeakTaps; i < len(x)-truePeakTaps; i++ {
			var s float64
			for t := -truePeakTaps; t <= truePeakTaps; t++ {
				s += float64(x[i+t]) * k[t+truePeakTaps]
			}
			if a := math.Abs(s); a > peak {
				peak = a
			}
		}
	}
	return peak
}

func TestTruePeakSkippingChangesNothing(t *testing.T) {
	rng := rand.New(rand.NewSource(20260922)) //nolint:gosec // G404: a fixed seed is the point — the reference pass and the skipping one must see identical samples
	kinds := map[string]func(i, n int) float32{
		"silence":    func(int, int) float32 { return 0 },
		"full-scale": func(i, _ int) float32 { return float32(math.Sin(float64(i) * 0.31)) },
		"quiet":      func(i, _ int) float32 { return float32(0.001 * math.Sin(float64(i)*0.31)) },
		"noise":      func(int, int) float32 { return float32(rng.Float64()*2 - 1) },
		// The case the skip has to get right: near-silence with one loud
		// burst, where most blocks are ruled out and one is not.
		"burst": func(i, n int) float32 {
			if i > n/2 && i < n/2+80 {
				return float32(math.Sin(float64(i) * 1.7))
			}
			return float32(0.0005 * rng.Float64())
		},
		// A ramp up through the buffer: every block beats the one before it,
		// so nothing may be skipped and the running peak keeps moving.
		"ramp": func(i, n int) float32 { return float32(float64(i) / float64(n)) },
	}
	for name, gen := range kinds {
		for _, n := range []int{49, 50, 64, 129, 1000, 4096} {
			x := make([]float32, n)
			for i := range x {
				x[i] = gen(i, n)
			}
			got, want := TruePeak(x), truePeakFull(x)
			if got != want {
				t.Errorf("%s n=%d: TruePeak=%.17g full pass=%.17g", name, n, got, want)
			}
		}
	}
}

func TestTruePeakBoundIsAReadBound(t *testing.T) {
	// The skip is only sound if no interpolated point can exceed the largest
	// sample it is built from times the kernel's L1 norm. Check the norm is
	// what it claims and that a unit impulse cannot beat it.
	g := truePeakMaxGain()
	if g < 1 {
		t.Fatalf("kernel L1 norm %v is under 1 — the kernels sum to 1, so it cannot be", g)
	}
	x := make([]float32, 4*truePeakTaps)
	x[len(x)/2] = 1
	if tp := TruePeak(x); tp > g+1e-12 {
		t.Fatalf("an impulse reconstructed to %v, above the bound %v the skip relies on", tp, g)
	}
}

func benchSignal(amp float64) []float32 {
	const n = 4096 // one tap read, which is what the meter hands over
	x := make([]float32, n)
	for i := range x {
		x[i] = float32(amp * math.Sin(float64(i)*0.31))
	}
	return x
}

func BenchmarkTruePeakFullScale(b *testing.B) {
	x := benchSignal(1.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruePeak(x)
	}
}

func BenchmarkTruePeakQuiet(b *testing.B) {
	x := benchSignal(0.05)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruePeak(x)
	}
}

func BenchmarkTruePeakSilence(b *testing.B) {
	x := make([]float32, 4096)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruePeak(x)
	}
}

func BenchmarkTruePeakFullPassFullScale(b *testing.B) {
	x := benchSignal(1.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = truePeakFull(x)
	}
}

func BenchmarkTruePeakFullPassQuiet(b *testing.B) {
	x := benchSignal(0.05)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = truePeakFull(x)
	}
}
