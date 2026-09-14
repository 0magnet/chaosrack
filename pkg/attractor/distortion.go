package attractor

import "math"

// Harmonic distortion analysis — THD, THD+N, SINAD and ENOB.
//
// The measurement every piece of audio equipment is specified by, and the one
// this app had no way to make: feed a clean tone in, and ask how much of what
// comes back is NOT that tone. Everything here is a ratio of powers taken out
// of one spectrum, so the whole of it is arithmetic and it lives untagged with
// a native test beside it — the answers are checkable against signals whose
// distortion is known exactly because they were built that way.
//
// ── WHAT THE FOUR NUMBERS ARE ────────────────────────────────────────────
//
//	THD    the harmonics alone, as a fraction of the fundamental. Power summed
//	       over 2f, 3f, 4f… and square-rooted. It ignores noise entirely, which
//	       is what makes it the number a distortion spec quotes and also what
//	       makes it flattering: a hissy amplifier can post a fine THD.
//
//	THD+N  everything that is not the fundamental, as a fraction of it — the
//	       harmonics, the noise, the hum, the intermodulation, the lot. It is
//	       the honest one, it is always the larger of the two, and the gap
//	       between them is how much of the rubbish is noise rather than
//	       distortion.
//
//	SINAD  the same ratio the other way up and in decibels: signal-plus-noise-
//	       and-distortion over noise-and-distortion. Converters are specified
//	       in it. It is −20·log₁₀(THD+N) to within the fundamental's own
//	       contribution, which is why both are here rather than one.
//
//	ENOB   the effective number of bits, (SINAD − 1.76)/6.02. A perfect n-bit
//	       converter quantizing a full-scale sine has a SINAD of 6.02n + 1.76
//	       dB, so running that backwards says what an ideal converter would
//	       have to be to sound this clean. It is the most legible of the four:
//	       "14.2 bits" means something to people who would have to think about
//	       what −87 dB is.
//
// ── WHY THE POWER IS SUMMED OVER A BAND RATHER THAN READ FROM A BIN ──────
//
// A Hann window spreads a pure tone over about three bins, and the tone is
// almost never exactly on a bin centre — the generator's frequency and the FFT's
// bin spacing have no reason to divide. Reading the peak bin alone therefore
// loses a scalloping-dependent fraction of the power, up to 1.4 dB with Hann,
// and the loss is DIFFERENT for the fundamental and for each harmonic. That
// turns into distortion that changes as the tone is tuned, which is an
// instrument reporting its own arithmetic.
//
// Summing the band around each peak recovers the power whatever the alignment.
// The bands have to be wide enough to hold the window's main lobe and narrow
// enough not to swallow the neighbours, which for Hann is ±3 bins.

// DistortionResult is one measurement.
type DistortionResult struct {
	OK bool // false when there is no tone to measure against

	Fundamental float64 // Hz, interpolated between bins
	Level       float64 // fundamental amplitude, 0..1 relative to full scale

	THD   float64   // harmonics over fundamental, as a fraction (0.01 = 1%)
	THDN  float64   // everything-but-the-fundamental over fundamental, a fraction
	SINAD float64   // dB
	ENOB  float64   // bits
	Harm  []float64 // harmonic amplitudes relative to the fundamental, H2 first
}

// thdHalfWidth is how many bins either side of a peak are counted as part of
// it. Three is the Hann window's main lobe: its first null is two bins out, and
// the third bin catches the shoulder a non-integer frequency pushes across.
const thdHalfWidth = 3

// thdNotchWidth is how many bins either side of the fundamental are EXCLUDED
// when measuring everything-but-the-fundamental, and it is much wider than
// thdHalfWidth on purpose.
//
// The two widths answer different questions. thdHalfWidth is "how much power is
// in this tone", and the main lobe is all of it. This is "where does the tone
// stop", and a window's skirts reach a great deal further than its main lobe:
// Hann falls away as the cube of the distance, so three bins out it is still
// only about 30 dB down. Measured with a three-bin exclusion, a synthesized
// pure tone read 0.67% THD+N — a distortion figure made entirely of the
// window, and a noise floor no measurement could see past. A real THD+N meter
// has the same problem and the same answer: it notches the fundamental out with
// a filter far wider than the tone.
//
// Sixteen bins is 47 Hz on a 16384-point window at 48 kHz, which is narrow
// enough that nothing anybody would call noise is inside it, and puts the
// leakage floor below −90 dB.
const thdNotchWidth = 16

// thdMinLevel is the quietest fundamental worth measuring, as an amplitude.
// Below about −60 dBFS the harmonics of a real signal are under the noise and
// the answer would be a measurement of the noise floor wearing a distortion
// label. Silence is the common case — nobody is playing a tone — and reporting
// "no tone" is the honest output for it.
const thdMinLevel = 1e-3

// AnalyzeDistortion measures the spectrum of one window of samples.
//
// maxHarm caps how many harmonics are summed; harmonics past Nyquist are not
// counted at all, because they are not there — a 1 kHz tone at 48 kHz has 23
// harmonics inside the band and asking for 30 would be asking about frequencies
// that alias rather than exist.
func AnalyzeDistortion(samples []float32, sampleRate int, maxHarm int) DistortionResult {
	var res DistortionResult
	n := len(samples)
	if n == 0 || n&(n-1) != 0 || sampleRate <= 0 {
		return res
	}
	mags := computeFFTMags(samples)
	if len(mags) < 8 {
		return res
	}
	if maxHarm < 2 {
		maxHarm = 2
	}
	binHz := float64(sampleRate) / float64(n)

	// The fundamental is the largest peak ABOVE DC. The first few bins are the
	// window's own leakage of any DC offset, and on a signal with one they are
	// the biggest thing in the spectrum — a fundamental found there would make
	// every harmonic a multiple of nothing.
	lo := thdHalfWidth + 1
	peak := lo
	for i := lo; i < len(mags); i++ {
		if mags[i] > mags[peak] {
			peak = i
		}
	}
	// Sub-bin position by parabolic interpolation on the log magnitudes, which
	// is the standard estimator for a windowed peak and good to a few hundredths
	// of a bin. It matters: the harmonic bands are placed from this, and an
	// error of half a bin at the fundamental is 10 bins out by the 20th harmonic.
	delta := parabolicPeak(mags, peak)
	res.Fundamental = (float64(peak) + delta) * binHz

	total := bandPowerSum(mags, lo, len(mags)-1)
	fund := bandPowerSum(mags, peak-thdHalfWidth, peak+thdHalfWidth)
	if total <= 0 || fund <= 0 {
		return res
	}
	// Band power back to the amplitude of the tone that made it.
	//
	// A tone of amplitude A windowed by w puts (A/2)·W(k−k₀) into the bins
	// around it, and by Parseval the window's transform carries Σ|W|² = n·Σw²,
	// so the band holds A²·n·Σw²/4. Inverting that is the line below. Derived
	// from the window the FFT actually applies rather than from a constant,
	// because a constant fitted to one window is an amplitude readout that is
	// silently wrong by a fixed factor under any other — this read every tone
	// 22% high when it was written as a bare 4/n.
	if we := windowEnergy(n); we > 0 {
		res.Level = 2 * math.Sqrt(fund/(float64(n)*we))
	}
	if res.Level < thdMinLevel {
		return res
	}

	var harmPower float64
	nyq := float64(sampleRate) / 2
	for k := 2; k <= maxHarm; k++ {
		f := res.Fundamental * float64(k)
		if f >= nyq {
			break
		}
		c := int(math.Round(f / binHz))
		p := bandPowerSum(mags, c-thdHalfWidth, c+thdHalfWidth)
		harmPower += p
		res.Harm = append(res.Harm, math.Sqrt(p/fund))
	}

	// Everything that is not the fundamental. Not the sum of the harmonics —
	// that is THD — but the whole spectrum with the fundamental's band removed,
	// which is what catches hum, hiss, intermodulation and anything else.
	rest := total - bandPowerSum(mags, peak-thdNotchWidth, peak+thdNotchWidth)
	if rest < 0 {
		rest = 0
	}

	res.THD = math.Sqrt(harmPower / fund)
	res.THDN = math.Sqrt(rest / fund)
	// SINAD is everything over everything-but-the-signal. Guarded because a
	// synthesized tone with no noise at all makes rest exactly zero, and a
	// converter that good has an infinite SINAD rather than a NaN one.
	if rest > 0 {
		res.SINAD = 10 * math.Log10(total/rest)
	} else {
		res.SINAD = 999
	}
	// The ideal-converter relation, run backwards. Clamped at zero because a
	// SINAD under 1.76 dB is a signal buried in its own noise, and "−0.3 bits"
	// is not a reading anybody can use.
	res.ENOB = (res.SINAD - 1.76) / 6.02
	if res.ENOB < 0 {
		res.ENOB = 0
	}
	res.OK = true
	return res
}

// bandPowerSum adds the squared magnitudes over an inclusive bin range,
// clipping to the spectrum.
func bandPowerSum(mags []float64, lo, hi int) float64 {
	if lo < 0 {
		lo = 0
	}
	if hi >= len(mags) {
		hi = len(mags) - 1
	}
	var s float64
	for i := lo; i <= hi; i++ {
		s += mags[i] * mags[i]
	}
	return s
}

// parabolicPeak estimates a peak's sub-bin offset from the three bins around
// it, in ±0.5 bins.
//
// On the LOG magnitudes, because a windowed tone's main lobe is close to a
// parabola in decibels and nothing like one in linear amplitude. The zero guard
// is not decoration: a synthesized tone landing exactly on a bin centre leaves
// its neighbours at the window's null, which is a true zero, and log(0) would
// put a NaN into the frequency and from there into every harmonic band.
func parabolicPeak(mags []float64, i int) float64 {
	if i <= 0 || i >= len(mags)-1 {
		return 0
	}
	const floor = 1e-20
	a := math.Log(math.Max(mags[i-1], floor))
	b := math.Log(math.Max(mags[i], floor))
	c := math.Log(math.Max(mags[i+1], floor))
	den := a - 2*b + c
	if den == 0 {
		return 0
	}
	d := 0.5 * (a - c) / den
	if d > 0.5 {
		d = 0.5
	} else if d < -0.5 {
		d = -0.5
	}
	return d
}

// AsPercent renders a distortion ratio the way a specification sheet does.
func AsPercent(ratio float64) float64 { return ratio * 100 }

// AsDB renders a distortion ratio in decibels, which is how a converter's is
// quoted. A ratio of zero is silence rather than −∞ dB.
func AsDB(ratio float64) float64 {
	if ratio <= 0 {
		return -999
	}
	return 20 * math.Log10(ratio)
}
