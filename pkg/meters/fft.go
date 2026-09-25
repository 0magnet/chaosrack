package meters

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

// ── The window set, and why it is larger than the spectrogram's ──────────
//
// The spectrogram offers audioprism's four (Hann, Hamming, Bartlett,
// rectangular) and those stay exactly as they were. MEASUREMENT needs windows
// those four do not include, because a measurement is limited by the window
// before it is limited by anything in the signal:
//
//	Hann's sidelobes are −31 dB and fall as the cube of the distance, which is
//	fine for a picture and hopeless for a distortion floor. Measured, a
//	synthesized pure tone read 0.013% THD+N through a Hann window with the
//	fundamental notched sixteen bins wide, and every bit of that was the window.
//
//	Blackman-Harris (4-term) has −92 dB sidelobes for a main lobe twice as wide.
//	Width costs resolution, which a distortion measurement has in abundance —
//	the harmonics are octaves apart — and buys a floor two orders of magnitude
//	lower. This is the window a distortion analyzer wants.
//
//	Flat-top is the amplitude window: its main lobe is deliberately broad and
//	flat-crested, so a tone anywhere between two bins reads its true amplitude
//	to about 0.01 dB where Hann can be 1.4 dB low. It is what a level readout
//	wants, and it is useless for anything needing to resolve two close tones.
//
//	Nuttall sits between Blackman-Harris and Hann: −93 dB sidelobes with a
//	slightly narrower lobe than Blackman-Harris, and a continuous first
//	derivative, which makes it the better choice when the signal is not
//	stationary across the window.

// WinKind is the local enum. The first four values are audioprism's four in
// audioprism's order, so mapping is a conversion rather than a table — but it
// is its own type on purpose, because the extra four are not settings the
// spectrogram offers and must not appear on its dial.
type WinKind int

// The window functions. The first four are audioprism's, in its order.
const (
	WinHann WinKind = iota
	WinHamming
	WinBartlett
	WinRectangular
	winBlackman
	WinBlackmanHarris
	winNuttall
	winFlatTop
)

// winKindOf maps the spectrogram's setting onto the local enum.
func winKindOf(wf sg.WindowFunc) WinKind {
	switch wf {
	case sg.WindowHamming:
		return WinHamming
	case sg.WindowBartlett:
		return WinBartlett
	case sg.WindowRectangular:
		return WinRectangular
	default:
		return WinHann
	}
}

// fillWindow writes the window's coefficients.
//
// The denominators are n−1 rather than n, which is go-dsp's convention and
// therefore audioprism's; matching it matters more than which convention is
// nicer, because the spectrogram's four have to keep producing exactly what
// they produced before. The cosine-sum windows added here use the same
// denominator for consistency — at the sizes this app uses, the difference
// between the symmetric and periodic forms is under a thousandth of a bin.
func fillWindow(win []float64, wk WinKind) {
	n := len(win)
	den := float64(n - 1)
	// cosSum evaluates a cosine-sum window Σ (−1)^k a_k cos(2πkx/den), which is
	// the shape every window below the triangular ones has. One routine because
	// they differ only in their coefficients, and coefficients written out
	// beside their own loop is how one of them comes to be subtly wrong.
	cosSum := func(a []float64) {
		for i := range win {
			x := 2 * math.Pi * float64(i) / den
			v := 0.0
			for k, ak := range a {
				if k%2 == 0 {
					v += ak * math.Cos(float64(k)*x)
				} else {
					v -= ak * math.Cos(float64(k)*x)
				}
			}
			win[i] = v
		}
	}
	switch wk {
	case WinHamming:
		cosSum([]float64{0.54, 0.46})
	case WinBartlett:
		for i := range win {
			win[i] = 1 - math.Abs((float64(i)-den/2)/(den/2))
		}
	case WinRectangular:
		for i := range win {
			win[i] = 1
		}
	case winBlackman:
		cosSum([]float64{0.42, 0.5, 0.08})
	case WinBlackmanHarris:
		// Harris 1978, the 4-term minimum-sidelobe set: −92 dB.
		cosSum([]float64{0.35875, 0.48829, 0.14128, 0.01168})
	case winNuttall:
		// Nuttall's 4-term continuous-first-derivative set: −93 dB.
		cosSum([]float64{0.355768, 0.487396, 0.144232, 0.012604})
	case winFlatTop:
		// The SRS/HP flat-top coefficients: amplitude flat to about 0.01 dB
		// across a bin, at the cost of a main lobe five bins wide. Note that
		// these sum to something near zero at the ends and the window goes
		// NEGATIVE either side of its shoulders, which is correct and is why a
		// flat-top's coherent gain is only 0.216.
		cosSum([]float64{1, 1.93, 1.29, 0.388, 0.028})
	default: // WinHann
		cosSum([]float64{0.5, 0.5})
	}
}

// WinMetrics are the two sums every amplitude and noise scaling needs.
//
// coherent gain (Σw/n) is what a TONE's peak is multiplied by, and Energy
// (Σw²/n) is what NOISE power is multiplied by. Their ratio is the window's
// noise-equivalent bandwidth, which is how many bins' worth of noise a single
// bin of this window collects — the number that turns a spectrum into a noise
// density. Getting an amplitude out of a spectrum without them is how a readout
// comes to be wrong by a fixed factor nobody notices.
type WinMetrics struct {
	CoherentGain float64 // Σw / n
	Energy       float64 // Σw² / n
	ENBW         float64 // n·Σw² / (Σw)² — bins
}

var winMetricsCache = map[fftKey]WinMetrics{}

// WindowMetrics returns the metrics for the window an FFT size is using.
func WindowMetrics(n int, wk WinKind) WinMetrics {
	k := fftKey{n, wk}
	if m, ok := winMetricsCache[k]; ok {
		return m
	}
	s := fftScratchFor(n, wk)
	var sum, sum2 float64
	for _, w := range s.win {
		sum += w
		sum2 += w * w
	}
	m := WinMetrics{CoherentGain: sum / float64(n), Energy: sum2 / float64(n)}
	if sum != 0 {
		m.ENBW = float64(n) * sum2 / (sum * sum)
	}
	winMetricsCache[k] = m
	return m
}

type fftKey struct {
	n  int
	wf WinKind
}

var fftCache = map[fftKey]*fftScratch{}

func fftScratchFor(n int, wf WinKind) *fftScratch {
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
	fillWindow(s.win, wf)
	bits := 0
	for 1<<bits < n {
		bits++
	}
	for i := range n {
		r := 0
		for b := range bits {
			r = r<<1 | (i>>b)&1
		}
		s.rev[i] = r
	}
	for i := range n / 2 {
		ang := -2 * math.Pi * float64(i) / float64(n)
		s.cosT[i] = math.Cos(ang)
		s.sinT[i] = math.Sin(ang)
	}
	fftCache[fftKey{n, wf}] = s
	return s
}

// ComputeFFTMags windows the input (Hann), runs an in-place radix-2 FFT and
// returns its n/2+1 magnitudes. The returned slice is the scratch's —
// valid until the next call with the same size. n must be a power of two.
func ComputeFFTMags(input []float32) []float64 {
	return ComputeFFTMagsWindow(input, sg.WindowHann)
}

// ComputeFFTMagsWindow is ComputeFFTMags with the window function named, for
// the spectrogram, which lets it be chosen. Everything else wants Hann and says
// so by calling the plain one.
func ComputeFFTMagsWindow(input []float32, wf sg.WindowFunc) []float64 {
	return ComputeFFTMagsKind(input, winKindOf(wf))
}

// ComputeFFTMagsKind is the real entry point, taking the local window enum so
// that the measurement paths can ask for a window the spectrogram does not
// offer. The two wrappers above are the callers that only ever want Hann or one
// of audioprism's four.
func ComputeFFTMagsKind(input []float32, wf WinKind) []float64 {
	n := len(input)
	if n == 0 || n&(n-1) != 0 {
		return nil
	}
	s := fftScratchFor(n, wf)
	for i := range n {
		s.re[s.rev[i]] = float64(input[i]) * s.win[i]
		s.im[s.rev[i]] = 0
	}
	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		tstep := n / size
		for start := 0; start < n; start += size {
			for k := range half {
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

// ComputeFFTComplex is ComputeFFTMagsKind keeping the PHASE, into caller-owned
// buffers.
//
// Magnitudes are enough for a spectrum, a spectrogram and a distortion figure,
// which is why nothing here needed the complex result until now. A transfer
// function is a phase measurement — its whole subject is how far the output
// lags the input — so throwing the phase away is throwing away the answer.
//
// It writes into re and im rather than returning the scratch's own slices,
// because a transfer function needs TWO spectra at once and the scratch has one
// of each per size: the second call would overwrite the first, and the
// measurement would come out as a signal correlated with itself. The copy is
// n/2+1 pairs and happens a few times a second.
func ComputeFFTComplex(input []float32, wf WinKind, re, im []float64) bool {
	n := len(input)
	if n == 0 || n&(n-1) != 0 || len(re) < n/2+1 || len(im) < n/2+1 {
		return false
	}
	// The magnitudes are not wanted, but the transform is, and it leaves its
	// result in the scratch — so this runs the same one rather than repeating
	// the butterflies here, where the two copies would drift apart.
	s := fftScratchFor(n, wf)
	ComputeFFTMagsKind(input, wf)
	copy(re, s.re[:n/2+1])
	copy(im, s.im[:n/2+1])
	return true
}

// InverseFFTReal reconstructs a real signal from a Hermitian half-spectrum —
// the n/2+1 complex bins a real transform produces — into dst, which must hold
// n samples.
//
// Done with the FORWARD transform rather than a second butterfly loop, through
// the identity IFFT(X) = conj(FFT(conj(X)))/n. One transform in the package is
// one transform to keep correct; a separate inverse would be a second copy of
// the same butterflies, differing only in a sign, which is exactly the kind of
// near-duplicate that drifts.
//
// The half-spectrum is mirrored back to full length first: bins n/2+1..n-1 are
// the conjugates of n/2-1 down to 1, which is what "Hermitian" means and what
// makes the result real.
func InverseFFTReal(re, im []float64, n int, dst []float64) bool {
	half := n/2 + 1
	if n == 0 || n&(n-1) != 0 || len(re) < half || len(im) < half || len(dst) < n {
		return false
	}
	s := fftScratchFor(n, WinRectangular)
	// conj(X), mirrored to full length, bit-reversed into place.
	for i := range n {
		var xr, xi float64
		if i < half {
			// half is n/2 and re/im are n/2+1 long, so i < half indexes inside
			// them by construction — gosec cannot follow the bound from the
			// loop condition to the slice length.
			xr, xi = re[i], -im[i] //nolint:gosec // i < half <= len(re)-1
		} else {
			j := n - i
			xr, xi = re[j], im[j] // conj of the conjugate is the value itself
		}
		s.re[s.rev[i]] = xr
		s.im[s.rev[i]] = xi
	}
	for size := 2; size <= n; size <<= 1 {
		hs := size >> 1
		tstep := n / size
		for start := 0; start < n; start += size {
			for k := range hs {
				// k < hs and tstep = n/size, so k*tstep < n/2, which is the length
				// of both tables by construction. gosec cannot follow that through
				// the two loop bounds; a runtime check in the innermost line of an
				// FFT is not the place to prove it.
				c, sn := s.cosT[k*tstep], s.sinT[k*tstep] //nolint:gosec // bounded by hs and tstep above
				i0, i1 := start+k, start+k+hs
				tr := s.re[i1]*c - s.im[i1]*sn
				ti := s.re[i1]*sn + s.im[i1]*c
				s.re[i1] = s.re[i0] - tr
				s.im[i1] = s.im[i0] - ti
				s.re[i0] += tr
				s.im[i0] += ti
			}
		}
	}
	// conj again and scale: the real part is the answer, and the imaginary part
	// is rounding for a properly Hermitian input.
	inv := 1 / float64(n)
	for i := range n {
		dst[i] = s.re[i] * inv
	}
	return true
}
