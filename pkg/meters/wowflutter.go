package meters

import "math"

// Wow and flutter — speed stability, measured off a test tone.
//
// The measurement a turntable, a tape deck or a cassette is judged by, and one
// nothing else here can make: every other analyzer asks about amplitude, and
// this asks whether the TIME AXIS is steady. A deck that runs at the wrong
// speed plays everything at the wrong pitch; one whose speed wobbles smears
// every note, and the ear is remarkably good at hearing it on a piano and
// nearly blind to it on a drum.
//
// The convention is a 3150 Hz tone, which is what the test records carry (DIN
// 45507, IEC 60386, and the same figure on every JIS and NAB alignment disc),
// because speed error shows as frequency modulation of a known carrier and
// 3150 Hz sits where both the ear and the measurement are sensitive. The Test
// module generates it, so a deck can be measured by playing that through it and
// capturing the result — or, with no deck involved at all, by watching this
// report 0.000% on the generator itself, which is the check that the instrument
// is not inventing the number.
//
// ── WHAT IS REPORTED ─────────────────────────────────────────────────────
//
//	SPEED    the mean frequency error as a percentage. A turntable running at
//	         33.4 rpm instead of 33⅓ is +0.2% here, and it is a different fault
//	         from wow: a constant error is a pitch shift and a varying one is a
//	         warble.
//	WOW      the slow modulation, 0.5–6 Hz — once-per-revolution faults. An
//	         off-center spindle hole is wow at exactly the platter's rate.
//	FLUTTER  the fast modulation, 6–100 Hz — capstan and idler faults, and the
//	         range the ear hears as roughness rather than as pitch movement.
//	W&F      the DIN-weighted figure, which is the one a specification quotes:
//	         the deviation through a filter peaked at 4 Hz, where the ear is
//	         most sensitive to pitch movement, read as a quasi-peak.
//
// ── HOW ──────────────────────────────────────────────────────────────────
//
// Quadrature demodulation, not zero-crossing counting. The carrier is
// multiplied by a complex exponential at its own nominal frequency, which
// brings it to baseband; low-passing away the image leaves an I/Q pair whose
// argument is the carrier's phase, and the derivative of that phase is its
// instantaneous frequency. Counting zero crossings would work and is what the
// analog instruments did, but its resolution is one sample per crossing and the
// deviations being measured here are parts in ten thousand.

const (
	// WfCarrier is the standard test tone.
	WfCarrier = 3150.0

	// wfWowLo and wfWowHi bound the slow band, wfFlutterHi the fast one. The
	// 6 Hz split between wow and flutter is the conventional one and is about
	// where a listener stops hearing pitch movement and starts hearing
	// roughness.
	wfWowLo     = 0.5
	wfWowHi     = 6.0
	wfFlutterHi = 100.0

	// wfDemodRate is the rate the deviation signal is decimated to before it is
	// filtered. The fastest thing being measured is 100 Hz, so a few hundred is
	// ample and it makes the very low-frequency filters — a 0.5 Hz high-pass —
	// numerically reasonable, which they are not at 48 kHz: a biquad with its
	// corner a hundred-thousandth of the sample rate has coefficients that
	// differ from 1 in the eleventh decimal place and loses the signal in the
	// rounding.
	wfDemodRate = 480.0
)

// WowFlutterResult is one measurement, all four figures as percentages.
type WowFlutterResult struct {
	OK          bool
	Carrier     float64 // the measured carrier, Hz
	SpeedPct    float64 // mean frequency error, %
	WowPct      float64 // 0.5–6 Hz, RMS, %
	FlutterPct  float64 // 6–100 Hz, RMS, %
	WeightedPct float64 // DIN-weighted quasi-peak, %
}

// AnalyzeWowFlutter measures a captured test tone.
//
// nominal is the frequency the tone is supposed to be — 3150 Hz unless
// something else was played — and is what the speed error is measured against.
// A nominal of zero takes the measured carrier as its own reference, which
// reports wow and flutter without a speed figure: the right answer when the
// tone's true frequency is not known.
func AnalyzeWowFlutter(x []float32, sampleRate int, nominal float64) WowFlutterResult {
	var res WowFlutterResult
	if len(x) < sampleRate/2 || sampleRate <= 0 {
		return res // under half a second cannot hold a cycle of the slowest wow
	}
	sr := float64(sampleRate)

	// The carrier, from the signal itself rather than assumed: a deck running
	// 2% fast puts the tone at 3213 Hz, and mixing against 3150 would leave a
	// 63 Hz beat that the demodulator would report as enormous flutter.
	carrier := wfEstimateCarrier(x, sampleRate, nominal)
	if carrier <= 0 {
		return res
	}
	res.Carrier = carrier

	// Quadrature mix down to baseband, low-passed and decimated in one pass.
	dev := wfDemodulate(x, sr, carrier)
	if len(dev) < 8 {
		return res
	}

	// dev is the instantaneous frequency in Hz relative to the mixing carrier.
	// Its MEAN is the residual speed error and its variation is the wow and
	// flutter, so the two are separated here and never confused again.
	var mean float64
	for _, v := range dev {
		mean += v
	}
	mean /= float64(len(dev))
	actual := carrier + mean
	if nominal > 0 {
		res.SpeedPct = (actual - nominal) / nominal * 100
	}
	res.Carrier = actual

	// Everything below is a fraction OF THE CARRIER, which is what makes the
	// figures percentages that can be compared between decks and between test
	// tones.
	for i := range dev {
		dev[i] = (dev[i] - mean) / actual
	}

	res.WowPct = 100 * wfBandRMS(dev, wfDemodRate, wfWowLo, wfWowHi)
	res.FlutterPct = 100 * wfBandRMS(dev, wfDemodRate, wfWowHi, wfFlutterHi)
	res.WeightedPct = 100 * wfWeightedPeak(dev, wfDemodRate)
	res.OK = true
	return res
}

// wfEstimateCarrier finds the tone's frequency.
//
// From a windowed FFT with sub-bin interpolation, the same estimator the
// distortion analyzer uses — and searched near the nominal when one is given,
// so a strong harmonic or a hum component cannot be mistaken for the carrier.
func wfEstimateCarrier(x []float32, sampleRate int, nominal float64) float64 {
	n := 1
	for n*2 <= len(x) && n < 1<<15 {
		n *= 2
	}
	if n < 1024 {
		return 0
	}
	mags := ComputeFFTMagsKind(x[len(x)-n:], WinBlackmanHarris)
	binHz := float64(sampleRate) / float64(n)
	lo, hi := 1, len(mags)-1
	if nominal > 0 {
		// Within ±10% of nominal: no deck anybody is measuring is further out
		// than that, and a wider search is an invitation to lock onto rumble.
		lo = int(nominal * 0.9 / binHz)
		hi = int(nominal * 1.1 / binHz)
		if lo < 1 {
			lo = 1
		}
		if hi >= len(mags) {
			hi = len(mags) - 1
		}
		if lo >= hi {
			return 0
		}
	}
	peak := lo
	for i := lo; i <= hi; i++ {
		if mags[i] > mags[peak] {
			peak = i
		}
	}
	if mags[peak] <= 0 {
		return 0
	}
	return (float64(peak) + parabolicPeak(mags, peak)) * binHz
}

// wfDemodulate mixes the carrier to baseband and returns the instantaneous
// frequency deviation in Hz, decimated to wfDemodRate.
func wfDemodulate(x []float32, sr, carrier float64) []float64 {
	decim := max(int(sr/wfDemodRate), 1)
	// FOUR cascaded one-poles on each quadrature arm, not one.
	//
	// Mixing puts an image at twice the carrier — 6300 Hz for the standard tone
	// — and whatever survives the filter aliases back into the measurement band
	// when the signal is decimated, where it is indistinguishable from flutter.
	// A single pole at 120 Hz rejects 6300 Hz by only 34 dB, and measured on a
	// perfectly steady tone that leak read as 0.022% flutter and 0.037%
	// weighted: an instrument whose own floor is within a factor of two of what
	// a good deck is specified at, which is no instrument at all.
	//
	// Four poles at 400 Hz reject the image by 96 dB and cost 1.1 dB at the top
	// of the flutter band, which is the trade worth making: the band edge is a
	// convention and the floor is not.
	const wfLPPoles = 4
	cutoff := 400.0
	a := math.Exp(-2 * math.Pi * cutoff / sr)
	var fi, fq [wfLPPoles]float64
	var out []float64
	prevPhase := 0.0
	havePrev := false
	w := 2 * math.Pi * carrier / sr
	// Nothing is emitted until the filters have settled. Until then fi and fq
	// are still climbing out of zero, and the argument of a vector that is
	// almost the origin is noise — atan2 of two tiny numbers swings wildly, and
	// differentiated it becomes an enormous apparent frequency excursion. The
	// quasi-peak detector downstream holds a peak for a second and a half, so
	// one startup spike was still showing as 0.035% weighted W&F on a perfectly
	// steady tone six seconds later.
	settle := int(0.05 * sr)
	for i, v := range x {
		ph := w * float64(i)
		s := float64(v)
		xi, xq := s*math.Cos(ph), -s*math.Sin(ph)
		for k := range wfLPPoles {
			fi[k] = (1-a)*xi + a*fi[k]
			fq[k] = (1-a)*xq + a*fq[k]
			xi, xq = fi[k], fq[k]
		}
		if i < settle || i%decim != 0 {
			continue
		}
		p := math.Atan2(fq[wfLPPoles-1], fi[wfLPPoles-1])
		if !havePrev {
			prevPhase, havePrev = p, true
			continue
		}
		d := p - prevPhase
		// Unwrapped: the phase walks continuously and atan2 does not.
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		prevPhase = p
		// dφ/dt over 2π is the frequency, in Hz, relative to the mixer.
		out = append(out, d*wfDemodRate/(2*math.Pi))
	}
	return out
}

// wfBiquadBandpass is a constant-skirt-gain bandpass, unity at its center.
func wfBiquadBandpass(f0, q, sr float64) biquad {
	w := 2 * math.Pi * f0 / sr
	alpha := math.Sin(w) / (2 * q)
	a0 := 1 + alpha
	return biquad{
		b0: alpha / a0, b1: 0, b2: -alpha / a0,
		a1: -2 * math.Cos(w) / a0,
		a2: (1 - alpha) / a0,
	}
}

// wfBandRMS is the RMS of a deviation signal inside a band.
//
// Built from a bandpass centered on the band's geometric mean with a Q that
// spans it, which is a gentler shape than a brick wall and is what the analog
// instruments had. The bands are a decade wide or more, so the skirts overlap a
// little — wow and flutter are conventions about where one becomes the other
// rather than disjoint physical phenomena, and a measurement that pretended
// otherwise would be claiming a precision the convention does not have.
func wfBandRMS(dev []float64, sr, lo, hi float64) float64 {
	f0 := math.Sqrt(lo * hi)
	q := f0 / (hi - lo)
	c := wfBiquadBandpass(f0, q, sr)
	var st biquadState
	// Run twice, forward only, to steepen the skirts. The filter's own settling
	// is skipped rather than measured: a bandpass handed a step at t=0 rings,
	// and that ring is not in the signal.
	skip := min(int(sr*2/lo), len(dev)/2)
	var sum float64
	var n int
	for i, v := range dev {
		y := st.step(c, v)
		if i < skip {
			continue
		}
		sum += y * y
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}

// wfWeightedPeak is the DIN-style weighted figure: the deviation through a
// filter peaked at 4 Hz, read as a quasi-peak.
//
// 4 Hz because that is where the ear is most sensitive to pitch movement — the
// weighting is a psychoacoustic curve, not a measurement bandwidth, and it is
// what makes the quoted W&F of two decks comparable. This is a two-pole
// approximation of the published curve rather than the tabulated response: it
// has the peak in the right place and the right rough width, and it is called
// an approximation here rather than a compliance claim because a table read off
// a graph would be one too.
//
// A quasi-peak rather than an RMS or a true peak: DIN reads the meter's
// deflection, which rises quickly and falls slowly, so a single lurch counts
// for more than its Energy would suggest and a steady warble is not
// under-reported.
func wfWeightedPeak(dev []float64, sr float64) float64 {
	c := wfBiquadBandpass(4, 0.6, sr)
	var st biquadState
	skip := len(dev) / 4
	// Quasi-peak ballistics: rise in about 10 ms, fall in about 1.5 s, which is
	// the shape of a mechanical meter movement.
	riseA := 1 - math.Exp(-1/(0.010*sr))
	fallA := 1 - math.Exp(-1/(1.500*sr))
	var meter, peak float64
	for i, v := range dev {
		y := math.Abs(st.step(c, v))
		if y > meter {
			meter += (y - meter) * riseA
		} else {
			meter += (y - meter) * fallA
		}
		if i >= skip && meter > peak {
			peak = meter
		}
	}
	return peak
}
