package attractor

// Allocation-free real-FFT magnitudes for the audio pipeline. The upstream
// audioprism ComputeFFT allocates four slices per call (window, windowed
// copy, FFT output, magnitudes) and recomputes the Hann window every time —
// per-frame garbage under audio modulation (audiofeatures runs it twice a
// frame, the spectrogram once per scrolled column). This path caches
// everything per FFT size and reuses one scratch set; the app only ever uses
// the default Hann window, so that's the only window implemented.
//
// Untagged: fft_test.go proves the output matches the vendored
// go-dsp FFTReal magnitudes bit-for-bit within float64 rounding.

import "math"

type fftScratch struct {
	n    int
	win  []float64 // Hann window
	re   []float64
	im   []float64
	rev  []int     // bit-reversal permutation
	cosT []float64 // twiddle tables, quarter-resolution per stage reuse
	sinT []float64
	mags []float64 // n/2 output magnitudes (reused — copy if you keep it)
}

var fftCache = map[int]*fftScratch{}

func fftScratchFor(n int) *fftScratch {
	if s, ok := fftCache[n]; ok {
		return s
	}
	s := &fftScratch{
		n:    n,
		win:  make([]float64, n),
		re:   make([]float64, n),
		im:   make([]float64, n),
		rev:  make([]int, n),
		cosT: make([]float64, n/2),
		sinT: make([]float64, n/2),
		mags: make([]float64, n/2),
	}
	for i := 0; i < n; i++ {
		// Hann as go-dsp defines it: 0.5·(1−cos(2πi/(n−1))).
		s.win[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	bits := 0
	for 1<<bits < n {
		bits++
	}
	for i := 0; i < n; i++ {
		r := 0
		for b := 0; b < bits; b++ {
			r = r<<1 | (i>>b)&1
		}
		s.rev[i] = r
	}
	for i := 0; i < n/2; i++ {
		ang := -2 * math.Pi * float64(i) / float64(n)
		s.cosT[i] = math.Cos(ang)
		s.sinT[i] = math.Sin(ang)
	}
	fftCache[n] = s
	return s
}

// computeFFTMags windows the input (Hann), runs an in-place radix-2 FFT and
// returns the first n/2 magnitudes. The returned slice is the scratch's —
// valid until the next call with the same size. n must be a power of two.
func computeFFTMags(input []float32) []float64 {
	n := len(input)
	if n == 0 || n&(n-1) != 0 {
		return nil
	}
	s := fftScratchFor(n)
	for i := 0; i < n; i++ {
		s.re[s.rev[i]] = float64(input[i]) * s.win[i]
		s.im[s.rev[i]] = 0
	}
	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		tstep := n / size
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				c, sn := s.cosT[k*tstep], s.sinT[k*tstep]
				i0, i1 := start+k, start+k+half
				tr := s.re[i1]*c - s.im[i1]*sn
				ti := s.re[i1]*sn + s.im[i1]*c
				s.re[i1] = s.re[i0] - tr
				s.im[i1] = s.im[i0] - ti
				s.re[i0] += tr
				s.im[i0] += ti
			}
		}
	}
	for i := 0; i < n/2; i++ {
		s.mags[i] = math.Hypot(s.re[i], s.im[i])
	}
	return s.mags
}
