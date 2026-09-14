package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// THE BAND CENTRES ARE THE PUBLISHED ONES. An analyzer whose bands sit
// somewhere else cannot be compared with anybody's measurement, and the
// third-octave row is the one every acoustics table in the world prints.
func TestThirdOctaveCentresAreTheStandardOnes(t *testing.T) {
	bands := RTABands(3)
	// ISO 266's preferred numbers, which are the exact centres rounded.
	want := []float64{20, 25, 31.5, 40, 50, 63, 80, 100, 125, 160, 200, 250, 315,
		400, 500, 630, 800, 1000, 1250, 1600, 2000, 2500, 3150, 4000, 5000,
		6300, 8000, 10000, 12500, 16000, 20000}
	if len(bands) != len(want) {
		t.Fatalf("%d third-octave bands between %.0f and %.0f Hz, want %d",
			len(bands), rtaLo, rtaHi, len(want))
	}
	for i, w := range want {
		// Within 2%, which is the gap between the exact centre and the
		// preferred number it is printed as.
		if rel := math.Abs(bands[i].Center-w) / w; rel > 0.02 {
			t.Errorf("band %d is centred at %.2f Hz, and the standard says %.1f", i, bands[i].Center, w)
		}
	}
}

// One thousand hertz is a band centre at every fraction, because the whole
// series is defined about it.
func TestEveryFractionIsCentredOnAKilohertz(t *testing.T) {
	for _, b := range rtaFractions {
		bands := RTABands(b)
		found := false
		for _, band := range bands {
			if math.Abs(band.Center-1000) < 0.01 {
				found = true
			}
		}
		if !found {
			t.Errorf("1/%d octave has no band at 1 kHz", b)
		}
	}
}

// THE BANDS PARTITION THE SPECTRUM: one band's top edge is the next one's
// bottom. Overlapping bands report more power than went in and gapped ones lose
// whatever falls between, and both show up as a level that depends on the band
// width — the one thing this display exists to remove.
func TestBandsPartitionWithoutGapsOrOverlap(t *testing.T) {
	for _, b := range rtaFractions {
		bands := RTABands(b)
		for i := 1; i < len(bands); i++ {
			prev, cur := bands[i-1], bands[i]
			if rel := math.Abs(prev.Hi-cur.Lo) / cur.Lo; rel > 1e-9 {
				t.Errorf("1/%d octave: band %d ends at %.4f Hz and band %d starts at %.4f",
					b, i-1, prev.Hi, i, cur.Lo)
			}
		}
	}
}

// A band an octave wide is a band whose edges are a factor of two apart, and a
// third-octave band the cube root of two. If the widths are wrong the centres
// being right does not save it.
func TestBandWidthsAreTheFractionTheyClaim(t *testing.T) {
	for _, b := range rtaFractions {
		want := math.Pow(10, 0.3/float64(b)) // the base-ten system's 1/b octave ratio
		for _, band := range RTABands(b) {
			if rel := math.Abs(band.Hi/band.Lo-want) / want; rel > 1e-9 {
				t.Errorf("1/%d octave: a band spans %.6f, want %.6f", b, band.Hi/band.Lo, want)
			}
		}
	}
}

// rtaOf runs a signal through the analysis the way the display does.
func rtaOf(x []float32, sr, b int) ([]RTABand, []float64) {
	bands := RTABands(b)
	levels := make([]float64, len(bands))
	mags := computeFFTMagsKind(x, rtaWindowKind)
	RTALevels(mags, len(x), sr, bands, rtaWindowKind, levels)
	return bands, levels
}

// inBand reports the levels for bands whose centre lies between lo and hi,
// which is how the tests below avoid the ends of the spectrum where a band can
// be narrower than a bin.
func inBand(bands []RTABand, levels []float64, lo, hi float64) []float64 {
	var out []float64
	for i, b := range bands {
		if b.Center >= lo && b.Center <= hi {
			out = append(out, levels[i])
		}
	}
	return out
}

func spreadDB(v []float64) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return hi - lo
}

func meanDB(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// rtaAveraged runs a stimulus through the analysis over several consecutive
// windows and averages the band POWER, which is what the display's smoothing
// does over time.
//
// Averaging is not a convenience here, it is part of the measurement. A single
// window's estimate of a noise band is chi-squared with two degrees of freedom
// per bin, and a 1/12-octave band down at 100 Hz holds only a handful of bins —
// so one snapshot scatters by several dB however correct the arithmetic is.
// Measured, one window of pink noise spanned 6.2 dB at 1/12 octave and 2 dB at
// third. That is the variance every real analyzer answers with an averaging
// switch, and asserting flatness on a single window would be asserting that
// noise is not noisy.
func rtaAveraged(sig audiosrc.TestSignal, windows, n, sr, b int) ([]RTABand, []float64) {
	bands := RTABands(b)
	acc := make([]float64, len(bands))
	one := make([]float64, len(bands))
	src := audiosrc.NewTestSource(sig, sr)
	src.SetLevel(1)
	buf := make([]float32, n)
	for w := 0; w < windows; w++ {
		src.FillMono(buf)
		RTALevels(computeFFTMagsKind(buf, rtaWindowKind), n, sr, bands, rtaWindowKind, one)
		for i, db := range one {
			acc[i] += math.Pow(10, db/10) // average the power, not the decibels
		}
	}
	for i := range acc {
		acc[i] = 10 * math.Log10(acc[i]/float64(windows))
	}
	return bands, acc
}

// THE TEST THAT TIES THE WHOLE THING TOGETHER. Pink noise is equal power per
// octave; a fractional-octave analyzer's bands are equal ratios; so pink noise
// has to read FLAT. That is the convention every room measurement in the world
// is done in, and if the two halves of this app disagree about it then one of
// them is wrong and neither is usable.
//
// The stimulus comes from the Test module's own generator rather than from a
// filter written here, so this checks the two features against each other.
func TestPinkNoiseReadsFlat(t *testing.T) {
	for _, b := range rtaFractions {
		bands, levels := rtaAveraged(audiosrc.TestPink, 16, 1<<14, 48000, b)
		// From 100 Hz to 10 kHz: below that a band can be narrower than a bin
		// on this window, and the very top runs into Nyquist.
		v := inBand(bands, levels, 100, 10000)
		if len(v) < 5 {
			t.Fatalf("1/%d octave: only %d bands in the measured range", b, len(v))
		}
		if s := spreadDB(v); s > 5 {
			t.Errorf("1/%d octave: pink noise spans %.1f dB across the band; it should read flat", b, s)
		}
	}
}

// ...and white noise must NOT, because it is equal power per HERTZ and the
// bands get wider. It has to rise 3 dB per octave, which is the difference
// between the two stimuli and the reason pink is the one used.
func TestWhiteNoiseRisesThreeDBPerOctaveOnTheRTA(t *testing.T) {
	bands, levels := rtaAveraged(audiosrc.TestWhite, 16, 1<<14, 48000, 3)
	lo := meanDB(inBand(bands, levels, 100, 200))
	hi := meanDB(inBand(bands, levels, 1600, 3200))
	// Four octaves between the two, at 3 dB each.
	if got := hi - lo; math.Abs(got-12) > 3 {
		t.Errorf("white noise rises %.1f dB over four octaves, want about 12", got)
	}
}

// A tone reads at its own level in the band that holds it, whatever the band
// width — that is what the ENBW normalization is for, and getting it wrong is
// how a display comes to depend on the knob rather than on the signal.
func TestAToneReadsItsOwnLevelAtEveryBandWidth(t *testing.T) {
	const amp = 0.5
	want := 20 * math.Log10(amp)
	x := distTone(1<<14, 1000.7, amp)
	for _, b := range rtaFractions {
		bands, levels := rtaOf(x, dtSR, b)
		peak := math.Inf(-1)
		for i := range bands {
			peak = math.Max(peak, levels[i])
		}
		if math.Abs(peak-want) > 1.5 {
			t.Errorf("1/%d octave: a %.1f dBFS tone peaks at %.1f dB", b, want, peak)
		}
	}
}

// Silence is the floor, not negative infinity: a bar has to have a height.
func TestSilenceReadsTheFloor(t *testing.T) {
	bands, levels := rtaOf(make([]float32, 1<<14), dtSR, 3)
	for i := range bands {
		if levels[i] != rtaFloorDB {
			t.Errorf("band %d of silence reads %.2f dB, want the floor %.0f", i, levels[i], rtaFloorDB)
		}
	}
}

// Meter ballistics: quick to rise, slow to fall. A display that fell as fast as
// it rises flickers at the frame rate and cannot be read.
func TestSmoothingRisesFasterThanItFalls(t *testing.T) {
	held := []float64{-60}
	RTASmooth(held, []float64{-20}, 0.5, 0.1)
	up := held[0]
	held = []float64{-20}
	RTASmooth(held, []float64{-60}, 0.5, 0.1)
	down := held[0]
	if up-(-60) <= (-20)-down {
		t.Errorf("a 40 dB step up moved %.1f dB and a step down %.1f dB; rise should be quicker",
			up-(-60), (-20)-down)
	}
}

// Peak hold takes a new maximum at once and lets go slowly, which is what makes
// an RTA readable on music rather than only on noise.
func TestPeakHoldTakesMaximaAtOnceAndDecays(t *testing.T) {
	peaks := []float64{-60}
	RTAPeakHold(peaks, []float64{-10}, 1)
	if peaks[0] != -10 {
		t.Errorf("a new peak of -10 was held at %.1f", peaks[0])
	}
	RTAPeakHold(peaks, []float64{-80}, 1)
	if peaks[0] != -11 {
		t.Errorf("after one decay step the hold is %.1f, want -11", peaks[0])
	}
	for i := 0; i < 1000; i++ {
		RTAPeakHold(peaks, []float64{-200}, 1)
	}
	if peaks[0] != rtaFloorDB {
		t.Errorf("the hold decayed to %.1f rather than stopping at the floor", peaks[0])
	}
}

// The knob's labels have to match its positions, or the dial names a band width
// it does not measure.
func TestRTAFractionTablesLineUp(t *testing.T) {
	if len(rtaFractions) != len(rtaFractionNames) || len(rtaFractions) != len(rtaFractionRing) {
		t.Errorf("%d fractions, %d names, %d ring labels",
			len(rtaFractions), len(rtaFractionNames), len(rtaFractionRing))
	}
}

// pinkFromGenerator and whiteFromGenerator take the stimulus from the Test
// module's own library, so these tests check the two features against each
// other rather than against a second implementation written to agree.
func pinkFromGenerator(n int) []float32  { return genStimulus(audiosrc.TestPink, n) }
func whiteFromGenerator(n int) []float32 { return genStimulus(audiosrc.TestWhite, n) }

func genStimulus(sig audiosrc.TestSignal, n int) []float32 {
	out := make([]float32, n)
	audiosrc.NewTestSource(sig, 48000).FillMono(out)
	return out
}
