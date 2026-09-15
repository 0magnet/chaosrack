package attractor

import "math"

// Impulse response, reverberation time and cumulative spectral decay.
//
// The measurement a room or a loudspeaker is characterised by, and the one
// every other display here is a slice through: the impulse response contains
// the whole linear behaviour of whatever it was measured through. The frequency
// response is its spectrum, the delay is where its peak is, the reverberation
// time is how its energy decays, and the waterfall is what it looks like when
// the decay is taken one frequency at a time.
//
// It is measured with the log sweep the Test module generates, because a sweep
// puts far more energy into a room than a click can without deafening anybody,
// and because its harmonic distortion lands BEFORE the linear response in the
// deconvolved result rather than smeared through it.
//
// ── DECONVOLUTION ────────────────────────────────────────────────────────
//
// H(f) = Y(f)·conj(X(f)) / (|X(f)|² + ε), inverse-transformed. That is the same
// cross-spectral ratio the transfer function display computes, with the
// division regularized — and the regularization is not optional. A sweep leaves
// almost no energy above 20 kHz and below 20 Hz, so |X|² there is nearly zero,
// and dividing by it turns whatever noise is in Y into an enormous response at
// exactly the frequencies nobody can hear. Undamped, that noise inverse-
// transforms into a burst of hash spread across the whole impulse response and
// buries the thing being measured.

// ImpulseResponse deconvolves a measured signal against the reference that
// produced it. ref and meas must be the same power-of-two length; the result is
// that long too.
//
// The regularization ε is relative to the reference's own mean power, so it
// scales with the measurement rather than being a number that happens to suit
// one recording level.
func ImpulseResponse(ref, meas []float32, epsilon float64) []float64 {
	n := len(ref)
	if n != len(meas) || n == 0 || n&(n-1) != 0 {
		return nil
	}
	half := n/2 + 1
	xr := make([]float64, half)
	xi := make([]float64, half)
	yr := make([]float64, half)
	yi := make([]float64, half)
	// RECTANGULAR, deliberately. Every other analysis here windows its input to
	// stop a tone leaking across the spectrum; a deconvolution must not, because
	// a window is a multiplication in time and therefore a convolution in
	// frequency — it would smear the very response being recovered, and the
	// sweep is already the whole buffer rather than a slice out of a longer
	// signal, so there is no discontinuity at the ends to suppress.
	if !computeFFTComplex(ref, winRectangular, xr, xi) {
		return nil
	}
	if !computeFFTComplex(meas, winRectangular, yr, yi) {
		return nil
	}
	var meanPow float64
	for i := 0; i < half; i++ {
		meanPow += xr[i]*xr[i] + xi[i]*xi[i]
	}
	meanPow /= float64(half)
	if meanPow <= 0 {
		return nil
	}
	if epsilon <= 0 {
		epsilon = 1e-4
	}
	eps := epsilon * meanPow
	hr := make([]float64, half)
	hi := make([]float64, half)
	for i := 0; i < half; i++ {
		den := xr[i]*xr[i] + xi[i]*xi[i] + eps
		// Y · conj(X) / (|X|² + ε)
		hr[i] = (yr[i]*xr[i] + yi[i]*xi[i]) / den
		hi[i] = (yi[i]*xr[i] - yr[i]*xi[i]) / den
	}
	out := make([]float64, n)
	if !inverseFFTReal(hr, hi, n, out) {
		return nil
	}
	return out
}

// SchroederDecay is the backward energy integration of an impulse response, in
// dB relative to its total, one value per sample.
//
// Schroeder's method (1965), and the reason every reverberation measurement
// uses it: the decay of a single impulse response is a noisy thing to fit a
// line to, because the response is a random-looking ring rather than a smooth
// fall. Integrating the energy BACKWARDS from the end gives the ensemble
// average of infinitely many measurements, exactly, from one — so the curve to
// fit is smooth and the answer does not depend on which impulse was caught.
func SchroederDecay(ir []float64) []float64 {
	n := len(ir)
	if n == 0 {
		return nil
	}
	out := make([]float64, n)
	var sum float64
	for i := n - 1; i >= 0; i-- {
		sum += ir[i] * ir[i]
		out[i] = sum
	}
	total := out[0]
	if total <= 0 {
		for i := range out {
			out[i] = -200
		}
		return out
	}
	for i := range out {
		if out[i] <= 0 {
			out[i] = -200
			continue
		}
		out[i] = 10 * math.Log10(out[i]/total)
	}
	return out
}

// ReverbTime fits a line to a stretch of the Schroeder decay and extrapolates
// it to 60 dB, which is what "RT60" means.
//
// from and to are the decibel points the fit runs between, and the usual
// choices are not 0 to −60: a real measurement's noise floor arrives long
// before 60 dB of decay, so T20 fits −5 to −25 and T30 fits −5 to −35, each
// extrapolated. Starting at −5 rather than 0 skips the direct sound, which is
// not part of the decay.
//
// ok is false when the decay never reaches the range asked for, which is the
// common case on a short or noisy measurement — and reporting a reverberation
// time extrapolated from three decibels of decay would be inventing one.
//
// WHAT THIS DOES NOT DO, stated because it is a real limitation rather than an
// oversight: it does not find the noise floor. A Schroeder integral of a
// TRUNCATED response always falls away at its end, because the energy remaining
// after the last sample is zero however loud the signal was — so a recording
// that stops before the reverberation has, or one whose tail is buried in noise,
// shows a decay that is partly the recording's edge rather than the room's. Even
// a signal that does not decay at all produces a curve here, and a line fitted
// to it returns a number.
//
// The usual answer is Lundeby's method: find where the response meets the noise
// floor, truncate there, and compensate for the energy beyond. That is worth
// having and is not here.
//
// AND THE OBVIOUS CHECK DOES NOT CATCH IT. A T20 and a T30 that disagree are
// usually taken as the sign that a fit has run into the noise floor, and
// measured against a pure truncation ramp they agree to within 6% — both
// returning about 1.5 seconds from a signal with no decay whatsoever. So the
// two fits being consistent is NOT evidence that the number is real. What is:
// a response whose tail is well above its noise floor for the whole fitted
// range, which is what Lundeby's method establishes and what nothing here
// currently does.
func ReverbTime(decay []float64, sampleRate int, from, to float64) (float64, bool) {
	// from is the SHALLOWER point and to the deeper one, both negative: a T20
	// runs from −5 to −25. The pair the wrong way round is a caller asking for a
	// decay that rises, which no measurement has.
	if len(decay) < 16 || sampleRate <= 0 || from <= to {
		return 0, false
	}
	start, end := -1, -1
	for i, v := range decay {
		if start < 0 && v <= from {
			start = i
		}
		if v <= to {
			end = i
			break
		}
	}
	if start < 0 || end < 0 || end <= start+8 {
		return 0, false
	}
	// Least squares of dB against time over the fitted stretch.
	var st, sd, stt, std float64
	n := float64(end - start + 1)
	for i := start; i <= end; i++ {
		t := float64(i) / float64(sampleRate)
		st += t
		sd += decay[i]
		stt += t * t
		std += t * decay[i]
	}
	den := n*stt - st*st
	if den == 0 {
		return 0, false
	}
	slope := (n*std - st*sd) / den // dB per second, negative
	if slope >= 0 {
		return 0, false
	}
	return -60 / slope, true
}

// IRPeak is where the impulse response's largest excursion is, in samples, and
// how big it is. That index is the system's bulk delay and the height is its
// broadband gain.
func IRPeak(ir []float64) (int, float64) {
	at, best := 0, 0.0
	for i, v := range ir {
		if a := math.Abs(v); a > best {
			at, best = i, a
		}
	}
	return at, best
}

// ── Cumulative spectral decay ────────────────────────────────────────────

// CSDSlice is one time slice of the waterfall: the spectrum of what is left of
// the impulse response from a moment onwards.
type CSDSlice struct {
	TimeMS float64
	DB     []float64 // one per frequency point, relative to the loudest point of the whole surface
}

// CSD computes the cumulative spectral decay of an impulse response.
//
// Take the impulse response, cut off everything before a moment, transform what
// is left; repeat for later and later moments. What the surface shows is how
// long each frequency goes on ringing after the impulse — which is the question
// "does this loudspeaker have a resonance", and one a frequency response cannot
// answer, because a resonance and a broad lift look identical in magnitude and
// nothing alike in time.
//
// The remaining tail is windowed on its leading edge rather than simply cut.
// A hard cut is a step, a step has energy at every frequency, and the surface
// would then show a ridge that is the knife rather than the loudspeaker.
func CSD(ir []float64, sampleRate int, slices int, sliceMS float64, fftLen int,
	freqs []float64) []CSDSlice {
	if len(ir) == 0 || sampleRate <= 0 || slices < 1 || fftLen <= 0 || fftLen&(fftLen-1) != 0 {
		return nil
	}
	peak, _ := IRPeak(ir)
	step := int(sliceMS / 1000 * float64(sampleRate))
	if step < 1 {
		step = 1
	}
	buf := make([]float32, fftLen)
	out := make([]CSDSlice, 0, slices)
	// The rise is a raised cosine over a tenth of the window, which is long
	// enough to have no step in it and short enough not to hide the very decay
	// being measured.
	rise := fftLen / 10
	if rise < 1 {
		rise = 1
	}
	loEdge, hiEdge := logBandEdges(freqs)
	best := math.Inf(-1)
	for s := 0; s < slices; s++ {
		start := peak + s*step
		if start >= len(ir) {
			break
		}
		for i := range buf {
			j := start + i
			if j >= len(ir) {
				buf[i] = 0
				continue
			}
			w := 1.0
			if i < rise {
				w = 0.5 * (1 - math.Cos(math.Pi*float64(i)/float64(rise)))
			}
			buf[i] = float32(ir[j] * w)
		}
		mags := computeFFTMagsKind(buf, winBlackmanHarris)
		db := make([]float64, len(freqs))
		binHz := float64(sampleRate) / float64(fftLen)
		for k, f := range freqs {
			// The whole band the point stands for. Three bins around it was the
			// rule here first, and it leaves most of the spectrum read by nothing
			// at all once the points are further apart than three bins — which is
			// everywhere above a few hundred hertz. A narrow resonance falling
			// between two points is exactly what this display exists to show and
			// exactly what that missed. See logBandEdges.
			p := bandPower(mags, binHz, loEdge[k], hiEdge[k], f)
			if p <= 0 {
				db[k] = -200
				continue
			}
			db[k] = 10 * math.Log10(p)
			if db[k] > best {
				best = db[k]
			}
		}
		out = append(out, CSDSlice{
			TimeMS: float64(s*step) / float64(sampleRate) * 1000,
			DB:     db,
		})
	}
	// Normalized to the loudest point of the whole surface, so the front slice
	// sits at 0 dB and everything is read as decay from it.
	if !math.IsInf(best, -1) {
		for i := range out {
			for k := range out[i].DB {
				out[i].DB[k] -= best
				if out[i].DB[k] < -200 {
					out[i].DB[k] = -200
				}
			}
		}
	}
	return out
}

// LogFreqPoints builds n frequencies spaced logarithmically from lo to hi —
// the x axis a waterfall is read on, because hearing is organised in ratios.
func LogFreqPoints(lo, hi float64, n int) []float64 {
	if n < 2 || lo <= 0 || hi <= lo {
		return nil
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = lo * math.Pow(hi/lo, float64(i)/float64(n-1))
	}
	return out
}

// SpectrumPoints fills out with the level at each of freqs, in dB relative to
// full scale, from one block of audio.
//
// The same reading a CSD slice is made of, taken from the SIGNAL rather than
// from an impulse response — which is what a live waterfall is: a stack of
// these, one per moment, instead of one per moment of a decay. Sharing the
// sampling means the two surfaces have the same frequency axis and the same
// "a few bins either side" rule, so one can be compared with the other rather
// than each having its own idea of where 1 kHz is.
//
// ABSOLUTE, not normalized to its own loudest point. A CSD is a decay and is
// read as dB below the impulse, so normalizing it is the measurement; a live
// surface is a level and has to stay where it is put, or a quiet passage rises
// to fill the display and the waterfall stops saying anything about loudness.
//
// The scaling is the one in rta.go: a tone of amplitude A puts A²n²·energy/4
// into its bin, so 4/(n²·energy) turns summed bin power back into an amplitude
// squared and a full-scale tone reads 0 dB.
func SpectrumPoints(buf []float32, sampleRate int, freqs []float64, winK winKind, out []float64) bool {
	n := len(buf)
	if n == 0 || n&(n-1) != 0 || sampleRate <= 0 || len(out) < len(freqs) {
		return false
	}
	mags := computeFFTMagsKind(buf, winK)
	if mags == nil {
		return false
	}
	m := windowMetrics(n, winK)
	norm := 4 / (float64(n) * float64(n) * m.energy)
	binHz := float64(sampleRate) / float64(n)
	lo, hi := logBandEdges(freqs)
	for k, f := range freqs {
		p := bandPower(mags, binHz, lo[k], hi[k], f)
		if p <= 0 {
			out[k] = spectrumFloorDB
			continue
		}
		out[k] = 10 * math.Log10(p*norm)
		if out[k] < spectrumFloorDB {
			out[k] = spectrumFloorDB
		}
	}
	return true
}

// spectrumFloorDB is where an empty point reads, rather than at negative
// infinity — which is not a height on a surface.
const spectrumFloorDB = -200.0

// logBandEdges gives each point of a logarithmic frequency axis the BAND it
// stands for: from the geometric midpoint below it to the geometric midpoint
// above.
//
// ── WHY A BAND AND NOT A FEW BINS ────────────────────────────────────────
//
// Reading a log axis by taking the nearest bin and its neighbours is the
// obvious thing and it is wrong, in a way that is invisible on noise and total
// on a tone. The axis points are a fixed RATIO apart — ninety-six of them over
// three decades is 7.5% — while the bins are a fixed NUMBER OF HERTZ apart, so
// above a few hundred hertz the points are many bins apart and three bins
// around each one covers a fraction of the spectrum. Everything in between is
// read by nothing at all.
//
// A steady 3150 Hz tone drew a completely flat surface: the nearest points are
// 3018 and 3243 Hz, the tone is eleven bins from either, and a three-bin window
// around each saw nothing. 1 kHz happened to land close enough to a point to
// leak into it, which is worse than failing outright — it looked like it worked.
//
// The edges are the geometric midpoints, which is what makes the bands
// PARTITION the axis: one band's top is exactly the next one's bottom, so every
// bin is counted once and none twice. Same rule and the same reason as the
// fractional-octave bands in rta.go, and it gives the same convention: band
// POWER, so equal power per constant-ratio band — pink noise — reads flat.
func logBandEdges(freqs []float64) (lo, hi []float64) {
	n := len(freqs)
	if n == 0 {
		return nil, nil
	}
	lo, hi = make([]float64, n), make([]float64, n)
	for i, f := range freqs {
		if i == 0 {
			// The end bands run out to half the step, so the first and last
			// points are as wide as their neighbours rather than half as wide.
			if n > 1 {
				lo[i] = f * f / math.Sqrt(f*freqs[1])
			} else {
				lo[i] = f
			}
		} else {
			lo[i] = math.Sqrt(freqs[i-1] * f)
		}
		if i == n-1 {
			if n > 1 {
				hi[i] = f * f / math.Sqrt(f*freqs[n-2])
			} else {
				hi[i] = f
			}
		} else {
			hi[i] = math.Sqrt(f * freqs[i+1])
		}
	}
	return lo, hi
}

// bandPower sums the power in mags between two frequencies.
//
// Falls back to the nearest bin when the band is narrower than one, which is
// what happens at the bottom of the axis: a 7.5% band at 20 Hz is 1.5 Hz wide
// and a 4096-point transform at 48 kHz has 11.7 Hz bins. The energy is there
// and the resolution is not, and reporting silence would say the opposite.
func bandPower(mags []float64, binHz, lo, hi, center float64) float64 {
	a := int(math.Ceil(lo / binHz))
	b := int(math.Floor(hi / binHz))
	if a < 1 {
		a = 1 // never DC: it is the window's leakage of any offset
	}
	if b >= len(mags) {
		b = len(mags) - 1
	}
	var p float64
	for k := a; k <= b; k++ {
		p += mags[k] * mags[k]
	}
	if b < a {
		k := int(math.Round(center / binHz))
		if k >= 1 && k < len(mags) {
			p = mags[k] * mags[k]
		}
	}
	return p
}
