package attractor

// Allocation-free real-FFT magnitudes for the audio pipeline. The upstream
// audioprism ComputeFFT allocates four slices per call (window, windowed
// copy, FFT output, magnitudes) and recomputes the Hann window every time —
// per-frame garbage under audio modulation (audiofeatures runs it twice a
// frame, the spectrogram once per scrolled column). This path caches everything
// per FFT size and window and reuses one scratch set.
//
// All four of audioprism's window functions are here, because the spectrogram
// offers the choice and a choice that is not applied is not a choice. Everything
// else in the app asks for Hann and says so by calling the plain wrapper.
//
// Untagged: fft_test.go proves the output matches the vendored
// go-dsp FFTReal magnitudes bit-for-bit within float64 rounding.

import (
	"math"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

type fftScratch struct {
	n    int
	win  []float64 // Hann window
	re   []float64
	im   []float64
	rev  []int     // bit-reversal permutation
	cosT []float64 // twiddle tables, quarter-resolution per stage reuse
	sinT []float64
	mags []float64 // n/2+1 output magnitudes (reused — copy if you keep it)
}

// The scratch is keyed on the window as well as the size. audioprism offers
// four window functions and they are not interchangeable — a rectangular window
// leaks all over the spectrum where a Hann does not — so a spectrogram that
// offers the choice has to actually apply it, and each choice needs its own
// precomputed table.
type fftKey struct {
	n  int
	wf sg.WindowFunc
}

var fftCache = map[fftKey]*fftScratch{}

func fftScratchFor(n int, wf sg.WindowFunc) *fftScratch {
	if s, ok := fftCache[fftKey{n, wf}]; ok {
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
		mags: make([]float64, n/2+1),
	}
	// As go-dsp defines them, which is what audioprism-go windows with — the
	// denominators are n−1, not n, and matching that matters more than which
	// convention is nicer.
	den := float64(n - 1)
	for i := 0; i < n; i++ {
		x := float64(i)
		switch wf {
		case sg.WindowHamming:
			s.win[i] = 0.54 - 0.46*math.Cos(2*math.Pi*x/den)
		case sg.WindowBartlett:
			s.win[i] = 1 - math.Abs((x-den/2)/(den/2))
		case sg.WindowRectangular:
			s.win[i] = 1
		default: // sg.WindowHann
			s.win[i] = 0.5 * (1 - math.Cos(2*math.Pi*x/den))
		}
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
	fftCache[fftKey{n, wf}] = s
	return s
}

// computeFFTMags windows the input (Hann), runs an in-place radix-2 FFT and
// returns its n/2+1 magnitudes. The returned slice is the scratch's —
// valid until the next call with the same size. n must be a power of two.
func computeFFTMags(input []float32) []float64 {
	return computeFFTMagsWindow(input, sg.WindowHann)
}

// computeFFTMagsWindow is computeFFTMags with the window function named, for
// the spectrogram, which lets it be chosen. Everything else wants Hann and says
// so by calling the plain one.
func computeFFTMagsWindow(input []float32, wf sg.WindowFunc) []float64 {
	n := len(input)
	if n == 0 || n&(n-1) != 0 {
		return nil
	}
	s := fftScratchFor(n, wf)
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
	// n/2+1 magnitudes: every bin of a real transform from DC up to and
	// including Nyquist, which is what audioprism-go returns and what the
	// column mapping counts on to know the transform size it came from.
	for i := 0; i <= n/2; i++ {
		s.mags[i] = math.Hypot(s.re[i], s.im[i])
	}
	return s.mags
}
