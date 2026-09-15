package attractor

import "math"

// The dual-channel transfer function — magnitude, phase and coherence.
//
// Every measurement so far asks about ONE signal: how loud, how distorted, how
// it is spread across the bands. This one asks about the relationship between
// TWO, which is a different question and the one a system is actually tuned by:
// send a signal into a loudspeaker, a room, a filter or a cable, capture what
// comes back, and ask what the thing in between did to it. That is what Smaart
// and every system-tuning rig does, and chaosrack already carries two channels
// end to end, so the hard part was already built.
//
//	MAGNITUDE  |H| in dB — the frequency response. Flat is a system that
//	           changed nothing.
//	PHASE      arg(H) in degrees — how far the output lags the input at each
//	           frequency. A pure DELAY is a phase that falls linearly with
//	           frequency, and its slope IS the delay, which is what makes this
//	           display a way to measure one.
//	COHERENCE  0..1 — how much of the output is linearly explained by the
//	           input. It is the number that says whether to believe the other
//	           two: noise, a second uncorrelated source, a nonlinearity or a
//	           system that moved during the measurement all drive it down.
//
// ── AVERAGING IS NOT OPTIONAL, AND COHERENCE IS WHY ──────────────────────
//
// From a SINGLE window, coherence is exactly 1 at every frequency — always,
// for any two signals whatever, including two independent noises. It falls
// straight out of the definition: |Sxy|² = |X|²|Y|² for one pair of complex
// numbers, so the ratio against Sxx·Syy is 1 identically. A coherence display
// built on one window is a row of perfect scores that means nothing.
//
// The quantity only becomes real once the spectra are averaged over several
// windows, because then the cross-spectrum's phase either lines up between
// windows (a real relationship, and the average keeps its magnitude) or points
// in different directions (noise, and the average cancels toward zero). So this
// accumulates, and a Result taken before it has enough windows says so rather
// than reporting a perfect score it cannot support.
//
// Everything here is arithmetic over spectra, so it is untagged and the native
// test can hand it signals whose relationship is known exactly: a gain, a
// delay, and two independent noises.

// xfWindowKind is the window the transfer measurement runs through. Hann rather
// than the low-leakage windows the other analyzers use: this one averages the
// CROSS-spectrum over many windows, which suppresses leakage by itself wherever
// the phase does not line up, and Hann's narrower main lobe keeps the response
// curve's resolution — a wide lobe smears a notch into a dip.
const xfWindowKind = winHann

// transferMinAvg is the fewest windows a result is given for. Below about eight
// the coherence is still biased high — its expected value on pure noise is
// 1/count — and a measurement that reports 0.25 for two unrelated signals
// invites believing a response that is not there.
const transferMinAvg = 8

// TransferResult is one measurement, band by band.
type TransferResult struct {
	OK        bool
	Averages  int
	Bands     []RTABand
	MagDB     []float64 // |H| in dB
	PhaseDeg  []float64 // arg(H), wrapped to ±180
	Coherence []float64 // 0..1

	// RefDB is how much of the REFERENCE was in each band, in dB relative to the
	// loudest band. It is not a property of the system under test and it is the
	// number that says whether to look at the others at all.
	//
	// Coherence does not cover this case, which is why both are here. Feed a
	// single tone through a perfect wire and every band reports 0 dB at
	// coherence 1 — correctly, because a wire really is flat and the window's
	// leakage really is passing through it unchanged. The answer is true and
	// worthless: nothing excited those bands, so nothing was learned about them.
	// A response curve drawn across them is a measurement of the stimulus's
	// spectrum wearing the system's name, which is why an analyzer's display
	// greys out the bands the signal did not reach.
	RefDB []float64
}

// TransferAccum accumulates the three spectra a transfer function needs.
//
// Sxx and Syy are the two auto-spectra (real, the power in each channel) and
// Sxy the CROSS-spectrum (complex, and where the phase relationship lives).
// Kept as running sums rather than as a list of windows, so the memory does not
// grow with the averaging count.
type TransferAccum struct {
	n     int
	sxx   []float64
	syy   []float64
	sxyRe []float64
	sxyIm []float64
	count int

	xre, xim []float64 // scratch for the two spectra
	yre, yim []float64
}

// Reset drops the accumulated average. Called whenever the thing being measured
// changes — a different channel, a different window length — because an average
// over two different systems describes neither.
func (a *TransferAccum) Reset() { a.count = 0; a.clear() }

func (a *TransferAccum) clear() {
	for i := range a.sxx {
		a.sxx[i], a.syy[i], a.sxyRe[i], a.sxyIm[i] = 0, 0, 0, 0
	}
}

func (a *TransferAccum) size(n int) {
	if a.n == n && a.sxx != nil {
		return
	}
	half := n/2 + 1
	a.n = n
	a.sxx = make([]float64, half)
	a.syy = make([]float64, half)
	a.sxyRe = make([]float64, half)
	a.sxyIm = make([]float64, half)
	a.xre, a.xim = make([]float64, half), make([]float64, half)
	a.yre, a.yim = make([]float64, half), make([]float64, half)
	a.count = 0
}

// Add folds one window of the two channels into the average. ref is the signal
// that went out and meas what came back; the result describes what happened
// between them.
func (a *TransferAccum) Add(ref, meas []float32, wk winKind) bool {
	n := len(ref)
	if n != len(meas) || n == 0 || n&(n-1) != 0 {
		return false
	}
	a.size(n)
	if !computeFFTComplex(ref, wk, a.xre, a.xim) {
		return false
	}
	if !computeFFTComplex(meas, wk, a.yre, a.yim) {
		return false
	}
	for i := range a.sxx {
		xr, xi := a.xre[i], a.xim[i]
		yr, yi := a.yre[i], a.yim[i]
		a.sxx[i] += xr*xr + xi*xi
		a.syy[i] += yr*yr + yi*yi
		// Sxy = conj(X)·Y, so its argument is arg(Y) − arg(X): how far the
		// output leads the input, which for a delay is negative and falls with
		// frequency.
		a.sxyRe[i] += xr*yr + xi*yi
		a.sxyIm[i] += xr*yi - xi*yr
	}
	a.count++
	return true
}

// Result reduces the accumulated spectra into fractional-octave bands.
//
// The band reduction sums the SPECTRA and divides once, rather than averaging
// per-bin transfer functions. Those are different answers and only the first is
// right: a per-bin H in a band where the input has almost no energy is a ratio
// of two small noisy numbers, and averaging it lets that bin shout as loudly as
// one carrying the signal. Summing first weights every bin by how much input
// was actually in it, which is what "the response at this band" means.
func (a *TransferAccum) Result(sampleRate, frac int) TransferResult {
	var r TransferResult
	if a.count < transferMinAvg || a.n == 0 || sampleRate <= 0 {
		r.Averages = a.count
		return r
	}
	r.Bands = RTABands(frac)
	r.MagDB = make([]float64, len(r.Bands))
	r.PhaseDeg = make([]float64, len(r.Bands))
	r.Coherence = make([]float64, len(r.Bands))
	r.RefDB = make([]float64, len(r.Bands))
	refPow := make([]float64, len(r.Bands))
	binHz := float64(sampleRate) / float64(a.n)
	for i, b := range r.Bands {
		lo := int(math.Ceil(b.Lo / binHz))
		hi := int(math.Floor(b.Hi / binHz))
		if lo < 1 {
			lo = 1
		}
		if hi >= len(a.sxx) {
			hi = len(a.sxx) - 1
		}
		if lo > hi {
			// A band narrower than a bin: read the nearest one rather than
			// reporting nothing, as the RTA does.
			k := int(math.Round(b.Center / binHz))
			if k < 1 || k >= len(a.sxx) {
				r.MagDB[i], r.PhaseDeg[i], r.Coherence[i] = transferFloorDB, 0, 0
				continue
			}
			lo, hi = k, k
		}
		var sxx, syy, re, im float64
		for k := lo; k <= hi; k++ {
			sxx += a.sxx[k]
			syy += a.syy[k]
			re += a.sxyRe[k]
			im += a.sxyIm[k]
		}
		refPow[i] = sxx
		if sxx <= 0 {
			// Nothing went in here, so nothing can be said about what came
			// out. A magnitude invented from a zero denominator is the one
			// number a response display must never show.
			r.MagDB[i], r.PhaseDeg[i], r.Coherence[i] = transferFloorDB, 0, 0
			continue
		}
		mag := math.Hypot(re, im) / sxx
		r.MagDB[i] = 20 * math.Log10(math.Max(mag, 1e-12))
		if r.MagDB[i] < transferFloorDB {
			r.MagDB[i] = transferFloorDB
		}
		r.PhaseDeg[i] = math.Atan2(im, re) * 180 / math.Pi
		if syy > 0 {
			c := (re*re + im*im) / (sxx * syy)
			if c > 1 {
				c = 1 // rounding only
			}
			r.Coherence[i] = c
		}
	}
	// The reference level, relative to the band that got the most. Relative
	// rather than absolute because what matters is whether a band was excited
	// COMPARED WITH the rest of the measurement, not how loud the whole thing
	// was.
	peak := 0.0
	for _, p := range refPow {
		if p > peak {
			peak = p
		}
	}
	for i, p := range refPow {
		if peak <= 0 || p <= 0 {
			r.RefDB[i] = transferFloorDB
			continue
		}
		r.RefDB[i] = 10 * math.Log10(p/peak)
		if r.RefDB[i] < transferFloorDB {
			r.RefDB[i] = transferFloorDB
		}
	}
	r.Averages = a.count
	r.OK = true
	return r
}

// transferFloorDB is the bottom of the magnitude scale: a band with no input
// reads here rather than at negative infinity, which is not a curve.
const transferFloorDB = -80.0

// TransferDelayMS estimates the bulk delay between the two channels from the
// slope of the phase, in milliseconds, and reports whether the estimate is
// worth anything.
//
// A pure delay τ has phase −2πfτ, so the slope of phase against frequency IS
// the delay. This is how a system-tuning rig measures the offset between a
// speaker and a microphone, and it is the measurement that turns "the phase
// curve is a slope" into a number to dial into a delay line.
//
// Fitted only over bands where the coherence says the phase means something.
// The fit is on the UNWRAPPED phase: arg() returns ±180, so a real delay of any
// size wraps many times across the band, and a slope fitted to the wrapped
// curve is a slope fitted to a sawtooth.
func TransferDelayMS(r TransferResult, minCoherence float64) (float64, bool) {
	if !r.OK {
		return 0, false
	}
	// Bands the reference barely reached are excluded whatever their coherence
	// says: see RefDB, where a perfect wire reports coherence 1 in bands nothing
	// excited, and a delay fitted through them is fitted through the stimulus.
	const minRefDB = -40
	var fs, ps []float64
	prev, turns := 0.0, 0.0
	first := true
	for i := range r.Bands {
		if r.Coherence[i] < minCoherence || r.RefDB[i] < minRefDB {
			// A gap in the usable bands breaks the unwrapping: the phase may
			// have turned any number of times while nobody was watching, so
			// the run has to start again rather than carry a wrong offset.
			first = true
			continue
		}
		p := r.PhaseDeg[i]
		if first {
			turns, first = 0, false
		} else {
			d := p - prev
			for d > 180 {
				d -= 360
				turns--
			}
			for d < -180 {
				d += 360
				turns++
			}
		}
		prev = p
		fs = append(fs, r.Bands[i].Center)
		ps = append(ps, p+turns*360)
	}
	if len(fs) < 4 {
		return 0, false
	}
	// Least squares through the unwrapped phase against frequency. The slope is
	// degrees per hertz; a delay of τ seconds is −360·τ degrees per hertz.
	var sf, sp, sff, sfp float64
	n := float64(len(fs))
	for i := range fs {
		sf += fs[i]
		sp += ps[i]
		sff += fs[i] * fs[i]
		sfp += fs[i] * ps[i]
	}
	den := n*sff - sf*sf
	if den == 0 {
		return 0, false
	}
	slope := (n*sfp - sf*sp) / den
	return -slope / 360 * 1000, true
}
