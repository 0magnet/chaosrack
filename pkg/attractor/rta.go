package attractor

import "math"

// Fractional-octave analysis — the RTA a room is measured with.
//
// A spectrogram shows every bin, which is the right display for watching sound
// move and the wrong one for asking what a room is doing to it: 8193 linearly
// spaced bins put nine-tenths of the picture above 2 kHz, where almost nothing
// interesting is, and squeeze the bass into the first few pixels. An RTA splits
// the band into a small number of FRACTIONAL-OCTAVE bands instead — equal
// ratios, so equal widths on screen — and shows the level in each.
//
// That is the display every room-correction and PA-tuning workflow is built on,
// and the reason pink noise is the stimulus for it: fractional-octave bands get
// wider in Hz as they go up, so equal power per octave is what reads FLAT. A
// flat-reading RTA on pink noise is the definition of a flat system, and the
// two facts have to be true together or neither is worth anything (see
// rta_test.go, which checks exactly that against the Test module's pink noise).
//
// ── THE BAND CENTERS ARE THE STANDARD ONES ───────────────────────────────
//
// ISO 266 and ANSI S1.11 place band centers on the base-ten system: f = 1000 ·
// 10^(n/(10·b)) for a 1/b octave band. Not the base-two system (1000 · 2^(n/b)),
// which is off by a fraction of a percent and which every published
// third-octave table disagrees with — the familiar 31.5, 63, 125, 250, 500,
// 1k, 2k, 4k, 8k, 16k row is the base-ten one rounded, and an analyzer whose
// bands sit somewhere else cannot be compared with anybody's measurement.

// rtaWindowKind is the window the analysis runs through. Blackman-Harris for the
// distortion analyzer's reason: a band's level is only as clean as the leakage
// from the loud band beside it, and on a room measurement the bands beside each
// other differ by tens of decibels.
const rtaWindowKind = winBlackmanHarris

// rtaFractions are the band widths offered, as the b in "1/b octave".
//
// 1/1 is the ten-band display on a hi-fi graphic equalizer, 1/3 is what room
// measurement and every acoustics standard uses, and the two finer settings are
// for finding a single narrow resonance — a 1/12-octave band is about 6% wide,
// which is roughly the ear's own resolution in the midrange.
var rtaFractions = []int{1, 3, 6, 12}

// rtaFractionNames and rtaFractionRing label the knob.
var (
	rtaFractionNames = []string{"1/1 octave", "1/3 octave", "1/6 octave", "1/12 octave"}
	rtaFractionRing  = []string{"1/1", "1/3", "1/6", "1/12"}
)

// rtaLo and rtaHi bound the analysis. 20 Hz to 20 kHz is the audible band, and
// bands whose center falls outside it are not built: a display that draws a
// band nobody can hear is spending width on it.
//
// rtaEdgeTol is why the comparison is not against those two numbers exactly.
// The band everybody calls "20 Hz" has an EXACT center of 19.953 Hz — 20 is the
// preferred number it is printed as, not the frequency it sits at — so a bound
// of exactly 20.0 drops the bottom band and the top one, and the third-octave
// series comes out with thirty bands where every acoustics table prints
// thirty-one. One percent admits that rounding and nothing else: the next band
// out is 12% away at third-octave and 6% away at 1/12.
const (
	rtaLo      = 20.0
	rtaHi      = 20000.0
	rtaEdgeTol = 1.01
)

// RTABand is one band of the analysis.
type RTABand struct {
	Center float64 // Hz, the standard center
	Lo, Hi float64 // Hz, the band edges
}

// RTABands builds the bands for a 1/b octave analysis, low to high.
//
// The edges are the center times 2^(±1/2b), which is what makes the bands
// PARTITION the spectrum: one band's upper edge is exactly the next one's
// lower, so every hertz is counted once and none is counted twice. An analyzer
// whose bands overlap reports more total power than went in, and one with gaps
// loses whatever falls between them — both show as a level that depends on the
// band width, which is the one thing a fractional-octave display exists to
// remove.
func RTABands(b int) []RTABand {
	if b < 1 {
		b = 1
	}
	// ONE OCTAVE IS 10^0.3 in the base-ten system, not 2 — that is the whole of
	// what "base-ten" means here, and it is 0.3% away from a factor of two. A
	// 1/b octave step is therefore 10^(0.3/b) and a band's half-width 10^(0.15/b).
	// Written as 10^(1/(10b)) first, which is a third of that: the third-octave
	// series came out with sixty-one bands between 20 Hz and 20 kHz instead of
	// the thirty-one every acoustics table prints.
	half := math.Pow(10, 0.15/float64(b))
	var out []RTABand
	// n indexes the standard series about 1 kHz, running out far enough to
	// cover the band from either end whatever the fraction.
	for n := -20 * b; n <= 20*b; n++ {
		c := 1000 * math.Pow(10, 0.3*float64(n)/float64(b))
		if c < rtaLo/rtaEdgeTol || c > rtaHi*rtaEdgeTol {
			continue
		}
		out = append(out, RTABand{Center: c, Lo: c / half, Hi: c * half})
	}
	return out
}

// RTALevels fills levels with the power in each band, in dB relative to full
// scale, from a magnitude spectrum.
//
// mags is a windowed FFT's magnitudes and winK the window it was taken with,
// because the scaling depends on it: a band's power has to be divided by the
// window's energy to mean anything, and by its noise-equivalent bandwidth to be
// a DENSITY rather than a sum over however many bins happened to fall inside.
//
// The bands do not all contain the same number of bins — a 1/3-octave band at
// 20 Hz holds one and at 16 kHz holds several hundred — which is exactly why
// the sum is normalized. Without it the display would rise 3 dB per octave on a
// flat input, which is the slope of the bin count and not of the signal.
func RTALevels(mags []float64, n, sampleRate int, bands []RTABand, winK winKind, levels []float64) {
	if len(levels) < len(bands) || len(mags) == 0 || sampleRate <= 0 {
		return
	}
	m := windowMetrics(n, winK)
	binHz := float64(sampleRate) / float64(n)
	// A tone of amplitude A puts A²·n²·energy/4 into its band (see the same
	// derivation in distortion.go), so this is the factor that turns summed
	// power back into an amplitude squared.
	norm := 4 / (float64(n) * float64(n) * m.energy)
	for i, b := range bands {
		lo := int(math.Ceil(b.Lo / binHz))
		hi := int(math.Floor(b.Hi / binHz))
		if lo < 1 {
			lo = 1 // never DC: it is the window's own leakage of any offset
		}
		if hi >= len(mags) {
			hi = len(mags) - 1
		}
		var p float64
		count := 0
		for k := lo; k <= hi; k++ {
			p += mags[k] * mags[k]
			count++
		}
		if count == 0 {
			// A band narrower than one bin, which happens at the bottom of a
			// fine fraction on a short window. Read the nearest bin rather than
			// reporting silence — the energy is there, the resolution is not,
			// and an empty bar would say the opposite.
			k := int(math.Round(b.Center / binHz))
			if k >= 1 && k < len(mags) {
				p = mags[k] * mags[k]
				count = 1
			}
		}
		if p <= 0 || count == 0 {
			levels[i] = rtaFloorDB
			continue
		}
		// BAND POWER, not density: the sum over the band as it stands, with no
		// division by how many bins fell inside it.
		//
		// That division is what a noise DENSITY needs, and putting it here is the
		// mistake this display exists to avoid. A fractional-octave band gets
		// wider in hertz as it goes up, so summing it whole is what makes pink
		// noise — equal power per octave — read FLAT, which is the convention
		// every room measurement in the world is done in. Divided by the count it
		// reads as a 3 dB per octave fall instead, and measured that way pink
		// noise spanned 21 dB across the band and white noise read flat: the two
		// stimuli swapped, which is precisely backwards.
		//
		// A tone is unaffected either way — all of its energy is in one band
		// whatever the width — which is why the tone test alone would not have
		// caught it.
		v := p * norm
		levels[i] = 10 * math.Log10(v)
		if levels[i] < rtaFloorDB {
			levels[i] = rtaFloorDB
		}
	}
}

// rtaFloorDB is the bottom of the scale: a band with nothing in it reads here
// rather than at negative infinity, which is not a bar height.
const rtaFloorDB = -120.0

// RTASmooth mixes a new set of levels into a held set, as a meter's ballistics
// do. fast is the rise coefficient and slow the fall.
//
// Asymmetric on purpose, and it is the same shape as every level meter ever
// built: a peak that is there and gone inside one window still has to move the
// display, so the rise is quick; a display that fell as fast would flicker at
// the frame rate and be unreadable. This is what "fast" and "slow" mean on a
// real analyzer's averaging switch.
func RTASmooth(held, fresh []float64, rise, fall float64) {
	for i := range held {
		if i >= len(fresh) {
			return
		}
		if fresh[i] > held[i] {
			held[i] += (fresh[i] - held[i]) * rise
		} else {
			held[i] += (fresh[i] - held[i]) * fall
		}
	}
}

// RTAPeakHold updates a peak-hold set: it takes any new maximum at once and
// otherwise decays by decayDB per call.
//
// The peak hold is what makes an RTA usable on music rather than on noise. A
// room's response is a property of the room, but music only excites part of the
// band at a time, so the instantaneous display jumps about and the peaks are the
// envelope that accumulates into the answer.
func RTAPeakHold(peaks, levels []float64, decayDB float64) {
	for i := range peaks {
		if i >= len(levels) {
			return
		}
		if levels[i] > peaks[i] {
			peaks[i] = levels[i]
		} else if peaks[i] -= decayDB; peaks[i] < rtaFloorDB {
			peaks[i] = rtaFloorDB
		}
	}
}
