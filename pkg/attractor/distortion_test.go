package attractor

import (
	"math"
	"testing"
)

const dtSR = 48000

// distTone builds n samples of a sine at hz with the given amplitude, plus any
// harmonics named as (multiple, amplitude) pairs. Built rather than measured,
// so the distortion in it is known exactly and the analyzer can be checked
// against arithmetic instead of against another measurement.
func distTone(n int, hz, amp float64, harm ...[2]float64) []float32 {
	out := make([]float32, n)
	for i := range out {
		t := float64(i) / dtSR
		v := amp * math.Sin(2*math.Pi*hz*t)
		for _, h := range harm {
			v += h[1] * math.Sin(2*math.Pi*hz*h[0]*t)
		}
		out[i] = float32(v)
	}
	return out
}

// A PURE TONE HAS NO DISTORTION. If the analyzer reports any, it is reporting
// its own arithmetic — spectral leakage from a frequency that does not land on
// a bin centre, most likely — and every other number it produces is built on
// the same error.
//
// The frequencies here are deliberately NOT bin-aligned: 1000 Hz at 48 kHz over
// 16384 samples is bin 341.33, a third of a bin off, which is the case that
// breaks a peak-bin implementation.
func TestAPureToneHasNoDistortion(t *testing.T) {
	for _, hz := range []float64{1000, 997, 440.5, 3150, 7777.7} {
		r := AnalyzeDistortion(distTone(16384, hz, 0.5), dtSR, 10)
		if !r.OK {
			t.Errorf("%.1f Hz: no measurement", hz)
			continue
		}
		if AsPercent(r.THD) > 0.05 {
			t.Errorf("%.1f Hz: a pure tone measured %.4f%% THD", hz, AsPercent(r.THD))
		}
		if AsPercent(r.THDN) > 0.2 {
			t.Errorf("%.1f Hz: a pure tone measured %.4f%% THD+N", hz, AsPercent(r.THDN))
		}
	}
}

// The fundamental has to be found to well under a bin, because the harmonic
// bands are placed from it: half a bin of error at the fundamental is ten bins
// out by the twentieth harmonic, which lands the band on the wrong part of the
// spectrum entirely.
func TestTheFundamentalIsFoundBetweenBins(t *testing.T) {
	binHz := float64(dtSR) / 16384
	for _, hz := range []float64{1000, 997.3, 440.5, 5000.7} {
		r := AnalyzeDistortion(distTone(16384, hz, 0.5), dtSR, 5)
		if !r.OK {
			t.Fatalf("%.1f Hz: no measurement", hz)
		}
		if err := math.Abs(r.Fundamental - hz); err > binHz*0.2 {
			t.Errorf("a %.2f Hz tone was found at %.2f Hz — %.2f bins out",
				hz, r.Fundamental, err/binHz)
		}
	}
}

// THE HEADLINE TEST. A tone with one harmonic added at a known amplitude has a
// THD that is arithmetic: the harmonic over the fundamental. If the analyzer
// cannot recover a number it was handed, nothing else it says is worth reading.
func TestTHDRecoversAKnownHarmonic(t *testing.T) {
	for _, c := range []struct {
		mult, ratio float64
	}{
		{2, 0.10}, {2, 0.01}, {2, 0.001},
		{3, 0.05}, {5, 0.02}, {7, 0.005},
	} {
		const fund = 0.5
		x := distTone(16384, 1000, fund, [2]float64{c.mult, fund * c.ratio})
		r := AnalyzeDistortion(x, dtSR, 12)
		if !r.OK {
			t.Fatalf("H%.0f at %.3f: no measurement", c.mult, c.ratio)
		}
		got := r.THD
		if rel := math.Abs(got-c.ratio) / c.ratio; rel > 0.03 {
			t.Errorf("H%.0f at %.4f%%: measured %.4f%% (%.1f%% out)",
				c.mult, AsPercent(c.ratio), AsPercent(got), rel*100)
		}
	}
}

// Several harmonics add in POWER, not in amplitude: the total is the root of
// the sum of squares. A version that summed amplitudes would read high by up to
// the number of harmonics, which on a real amplifier is most of the answer.
func TestHarmonicsAddInPower(t *testing.T) {
	const fund = 0.5
	x := distTone(16384, 1000, fund,
		[2]float64{2, fund * 0.03},
		[2]float64{3, fund * 0.04})
	r := AnalyzeDistortion(x, dtSR, 10)
	if !r.OK {
		t.Fatal("no measurement")
	}
	want := math.Hypot(0.03, 0.04) // 0.05 exactly
	if rel := math.Abs(r.THD-want) / want; rel > 0.03 {
		t.Errorf("3%% and 4%% together measured %.4f%%, want %.4f%% (root sum of squares)",
			AsPercent(r.THD), AsPercent(want))
	}
}

// Each harmonic is reported on its own as well as summed, because "3% THD" and
// "3% all of it second harmonic" are different facts about an amplifier.
func TestPerHarmonicLevelsAreReported(t *testing.T) {
	const fund = 0.5
	x := distTone(16384, 1000, fund,
		[2]float64{2, fund * 0.10},
		[2]float64{4, fund * 0.02})
	r := AnalyzeDistortion(x, dtSR, 6)
	if len(r.Harm) < 3 {
		t.Fatalf("only %d harmonics reported", len(r.Harm))
	}
	// Harm[0] is H2, Harm[2] is H4.
	if math.Abs(r.Harm[0]-0.10) > 0.005 {
		t.Errorf("H2 reported at %.4f, want 0.10", r.Harm[0])
	}
	if math.Abs(r.Harm[2]-0.02) > 0.003 {
		t.Errorf("H4 reported at %.4f, want 0.02", r.Harm[2])
	}
	if r.Harm[1] > 0.005 {
		t.Errorf("H3 reported at %.4f, and nothing was put there", r.Harm[1])
	}
}

// THD+N counts everything that is not the fundamental, so it is ALWAYS at least
// THD. A measurement where it came out lower would mean noise had been
// subtracted from distortion, which is not a thing.
func TestTHDNIsNeverLessThanTHD(t *testing.T) {
	const fund = 0.5
	for _, noise := range []float64{0, 1e-4, 1e-3, 1e-2} {
		x := distTone(16384, 1000, fund, [2]float64{3, fund * 0.01})
		rng := uint32(12345)
		for i := range x {
			rng = rng*1664525 + 1013904223
			x[i] += float32(noise * (float64(int32(rng)>>8) / (1 << 23)))
		}
		r := AnalyzeDistortion(x, dtSR, 10)
		if !r.OK {
			t.Fatal("no measurement")
		}
		if r.THDN < r.THD-1e-9 {
			t.Errorf("noise %.0e: THD+N %.5f is under THD %.5f", noise, r.THDN, r.THD)
		}
	}
}

// Noise moves THD+N and leaves THD alone. That difference is the whole reason
// both numbers are quoted: the gap between them is how much of the rubbish is
// hiss rather than distortion.
//
// Asserted against the ARITHMETIC rather than against "it went up", because
// what the two should be is calculable. Uniform noise of amplitude a has an RMS
// of a/√3; the fundamental's is 0.5/√2; and noise and distortion are
// uncorrelated, so THD+N is the quadrature sum of the two ratios. A first
// version of this test asked only that noise double THD+N, which the numbers do
// not oblige: 1% distortion under 1.63% noise is 1.91%, not 2%. The threshold
// was a guess where a calculation was available.
func TestNoiseMovesTHDNAndNotTHD(t *testing.T) {
	const fund = 0.5
	const dist = 0.01 // the third harmonic put in below
	mk := func(noise float64) DistortionResult {
		x := distTone(16384, 1000, fund, [2]float64{3, fund * dist})
		rng := uint32(999)
		for i := range x {
			rng = rng*1664525 + 1013904223
			x[i] += float32(noise * (float64(int32(rng)>>8) / (1 << 23)))
		}
		return AnalyzeDistortion(x, dtSR, 10)
	}
	for _, noise := range []float64{0, 0.002, 0.01, 0.05} {
		r := mk(noise)
		if !r.OK {
			t.Fatalf("noise %.3f: no measurement", noise)
		}
		// THD counts the harmonic and almost nothing else. Not NOTHING else: the
		// harmonic's band is seven bins wide and the noise in those seven bins is
		// inside it, which is why a real analyzer averages or narrows its
		// bandwidth to push a distortion figure below the hiss. At 5% noise that
		// is worth 0.7% on its own, so the tight claim is made where it holds and
		// the loose one everywhere.
		if noise <= 0.01 {
			if rel := math.Abs(r.THD-dist) / dist; rel > 0.08 {
				t.Errorf("noise %.3f: THD reads %.4f%%, and %.4f%% was put in",
					noise, AsPercent(r.THD), AsPercent(dist))
			}
		} else if r.THD > dist*2 {
			t.Errorf("noise %.3f: THD reads %.4f%% against %.4f%% put in — it should be far "+
				"less sensitive to noise than THD+N is", noise, AsPercent(r.THD), AsPercent(dist))
		}
		// THD+N counts both, in quadrature.
		nRatio := (noise / math.Sqrt(3)) / (fund / math.Sqrt2)
		want := math.Hypot(dist, nRatio)
		if rel := math.Abs(r.THDN-want) / want; rel > 0.08 {
			t.Errorf("noise %.3f: THD+N reads %.4f%%, want %.4f%% (%.4f%% distortion and %.4f%% noise)",
				noise, AsPercent(r.THDN), AsPercent(want), AsPercent(dist), AsPercent(nRatio))
		}
	}
}

// SINAD and THD+N are the same ratio read two ways, so they have to agree.
// Two numbers that can drift apart are two chances to be wrong.
func TestSINADAgreesWithTHDN(t *testing.T) {
	const fund = 0.5
	x := distTone(16384, 1000, fund, [2]float64{2, fund * 0.02}, [2]float64{3, fund * 0.01})
	rng := uint32(4242)
	for i := range x {
		rng = rng*1664525 + 1013904223
		x[i] += float32(1e-3 * (float64(int32(rng)>>8) / (1 << 23)))
	}
	r := AnalyzeDistortion(x, dtSR, 10)
	if !r.OK {
		t.Fatal("no measurement")
	}
	// SINAD is (S+N+D)/(N+D) and THD+N is (N+D)/S, so the two differ only by
	// the fundamental's own place in the numerator: 10·log10(1 + 1/thdn²).
	want := 10 * math.Log10(1+1/(r.THDN*r.THDN))
	if math.Abs(r.SINAD-want) > 0.2 {
		t.Errorf("SINAD %.2f dB against THD+N %.5f, which implies %.2f dB", r.SINAD, r.THDN, want)
	}
}

// ENOB IS THE IDEAL-CONVERTER RELATION RUN BACKWARDS, so a signal quantized to
// a known number of bits has to come back as that number. This is the one test
// here that checks the analyzer against physics rather than against its own
// definitions: quantization noise is not something the test puts in at a chosen
// level, it is what falls out of rounding.
func TestENOBRecoversAQuantizedSignal(t *testing.T) {
	for _, bits := range []int{8, 10, 12} {
		x := distTone(16384, 1000.7, 0.98) // near full scale, off a bin centre
		step := 2.0 / math.Pow(2, float64(bits))
		for i := range x {
			x[i] = float32(math.Round(float64(x[i])/step) * step)
		}
		r := AnalyzeDistortion(x, dtSR, 20)
		if !r.OK {
			t.Fatalf("%d-bit: no measurement", bits)
		}
		if math.Abs(r.ENOB-float64(bits)) > 0.6 {
			t.Errorf("a %d-bit signal measured %.2f effective bits (SINAD %.1f dB)",
				bits, r.ENOB, r.SINAD)
		}
	}
}

// Silence is the common case — nobody is playing a tone — and the honest output
// for it is "no tone", not a measurement of the noise floor wearing a
// distortion label.
func TestSilenceIsReportedAsNoMeasurement(t *testing.T) {
	if r := AnalyzeDistortion(make([]float32, 16384), dtSR, 10); r.OK {
		t.Errorf("silence measured %.4f%% THD", AsPercent(r.THD))
	}
	// ...and so is a signal far below anything real.
	if r := AnalyzeDistortion(distTone(16384, 1000, 1e-5), dtSR, 10); r.OK {
		t.Error("a signal 100 dB down was measured rather than refused")
	}
}

// A DC offset is not a fundamental. The window's leakage of one sits in the
// first few bins and on a signal with a large offset it is the biggest thing in
// the spectrum — a fundamental found there would make every harmonic a multiple
// of nothing.
func TestADCOffsetIsNotMistakenForTheFundamental(t *testing.T) {
	x := distTone(16384, 1000, 0.3)
	for i := range x {
		x[i] += 0.6
	}
	r := AnalyzeDistortion(x, dtSR, 10)
	if !r.OK {
		t.Fatal("no measurement")
	}
	if math.Abs(r.Fundamental-1000) > 5 {
		t.Errorf("with a DC offset twice the tone, the fundamental was found at %.1f Hz", r.Fundamental)
	}
}

// Harmonics past Nyquist are not there, and counting them would be counting
// aliases. A 7 kHz tone at 48 kHz has three harmonics inside the band.
func TestHarmonicsPastNyquistAreNotCounted(t *testing.T) {
	r := AnalyzeDistortion(distTone(16384, 7000, 0.5), dtSR, 20)
	if !r.OK {
		t.Fatal("no measurement")
	}
	if len(r.Harm) != 2 { // 14 kHz and 21 kHz; 28 kHz is past Nyquist
		t.Errorf("a 7 kHz tone reported %d harmonics; only 2 fit under Nyquist", len(r.Harm))
	}
}

// The level readout is what tells you the tone is actually near full scale,
// which is where a distortion spec is quoted. A Hann window's coherent gain is
// a half, and forgetting it reads every tone 6 dB low.
func TestLevelIsRelativeToFullScale(t *testing.T) {
	for _, amp := range []float64{1.0, 0.5, 0.1} {
		r := AnalyzeDistortion(distTone(16384, 1000.5, amp), dtSR, 5)
		if !r.OK {
			t.Fatalf("amp %.2f: no measurement", amp)
		}
		if rel := math.Abs(r.Level-amp) / amp; rel > 0.02 {
			t.Errorf("a tone of amplitude %.2f read as %.4f", amp, r.Level)
		}
	}
}

// A window that is not a power of two, or a nonsense rate, has to come back as
// "no measurement" rather than as a panic or an invented number.
func TestBadInputIsRefused(t *testing.T) {
	for _, c := range []struct {
		n  int
		sr int
	}{{0, dtSR}, {1000, dtSR}, {16384, 0}, {16384, -1}, {4, dtSR}} {
		x := make([]float32, c.n)
		for i := range x {
			x[i] = 0.5
		}
		if r := AnalyzeDistortion(x, c.sr, 10); r.OK {
			t.Errorf("n=%d sr=%d was measured rather than refused", c.n, c.sr)
		}
	}
}

// THE INSTRUMENT'S OWN FLOOR, stated as a test so it cannot quietly get worse.
//
// A synthesized pure tone has no distortion and no noise, so whatever the
// analyzer reports for one is the window's leakage getting past the notch —
// the quietest thing this instrument can see, and the number the module's
// tooltip and the README quote. Through a Hann window it was 0.013%
// (77.8 dB SINAD, 12.6 effective bits) and the analyzer could read nothing
// below it, which is above the distortion of genuinely good equipment.
func TestTheMeasurementFloorIsWhereItIsClaimed(t *testing.T) {
	worstTHDN, worstSINAD, bestENOB := 0.0, 999.0, 0.0
	for _, hz := range []float64{100, 440.5, 997, 1000, 3150, 7777.7, 12000.3} {
		r := AnalyzeDistortion(distTone(16384, hz, 0.9), dtSR, 20)
		if !r.OK {
			t.Fatalf("%.1f Hz: no measurement", hz)
		}
		worstTHDN = math.Max(worstTHDN, r.THDN)
		worstSINAD = math.Min(worstSINAD, r.SINAD)
		bestENOB = math.Max(bestENOB, r.ENOB)
	}
	t.Logf("floor: %.5f%% THD+N, %.1f dB SINAD, %.1f ENOB", AsPercent(worstTHDN), worstSINAD, bestENOB)
	// The claim. Loose enough not to be a trip-wire on an arithmetic change,
	// tight enough that dropping back to a Hann window — which would land at
	// 0.013% — fails it immediately.
	if AsPercent(worstTHDN) > 0.002 {
		t.Errorf("the floor is %.5f%% THD+N; the README claims better than 0.002%%",
			AsPercent(worstTHDN))
	}
	if worstSINAD < 94 {
		t.Errorf("the floor is %.1f dB SINAD; the README claims better than 94 dB", worstSINAD)
	}
}
