package audiosrc

import (
	"math"
	"testing"
)

const testSR = 48000.0

// gen runs a stimulus for n samples and returns both channels.
func gen(sig TestSignal, n int) (l, r []float64) {
	g := newTestGen()
	g.sig = sig
	g.level = 1
	l, r = make([]float64, n), make([]float64, n)
	for i := range l {
		a, b := g.next(testSR)
		l[i], r[i] = float64(a), float64(b)
	}
	return l, r
}

func rms(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}

func mean(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func peak(x []float64) float64 {
	m := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > m {
			m = a
		}
	}
	return m
}

// corr is Pearson's r, the same quantity the goniometer's meter shows.
func corr(a, b []float64) float64 {
	ma, mb := mean(a), mean(b)
	var saa, sbb, sab float64
	for i := range a {
		x, y := a[i]-ma, b[i]-mb
		saa += x * x
		sbb += y * y
		sab += x * y
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

// A stimulus that leaves the converter is a stimulus that was clipped, and a
// clipped test signal measures the clipping rather than the system. Every one
// of them has to stay inside ±1 at full level.
func TestEveryStimulusStaysInsideFullScale(t *testing.T) {
	for sig := TestSignal(1); sig < testSignalCount; sig++ {
		l, r := gen(sig, int(testSR*5))
		if p := peak(l); p > 1 {
			t.Errorf("%s: left peaks at %.4f", TestSignalNames[sig], p)
		}
		if p := peak(r); p > 1 {
			t.Errorf("%s: right peaks at %.4f", TestSignalNames[sig], p)
		}
	}
}

// DC is not a test signal. An offset drives a woofer off center, biases every
// RMS and correlation measured from it, and is invisible on every display here
// — so it is worth pinning rather than trusting.
func TestNoStimulusCarriesDC(t *testing.T) {
	for sig := TestSignal(1); sig < testSignalCount; sig++ {
		l, _ := gen(sig, int(testSR*4)) // a whole number of sweep passes
		if m := mean(l); math.Abs(m) > 0.02 {
			t.Errorf("%s: mean is %.4f, which is a DC offset", TestSignalNames[sig], m)
		}
	}
}

// Every stimulus has to actually make a signal. A position on the dial that
// delivers silence is worse than no position at all: it looks like a broken
// input rather than a missing feature.
func TestEveryStimulusIsAudible(t *testing.T) {
	for sig := TestSignal(1); sig < testSignalCount; sig++ {
		l, r := gen(sig, int(testSR*2))
		if rms(l) < 0.01 && rms(r) < 0.01 {
			t.Errorf("%s: both channels are silent", TestSignalNames[sig])
		}
	}
}

// ── The stereo relationships, which are the whole point of three of them ──

func TestStereoRelationshipsAreWhatTheyClaim(t *testing.T) {
	cases := []struct {
		sig  TestSignal
		want float64
		why  string
	}{
		{TestWhite, +1, "the same noise in both channels is the diagonal"},
		{TestOutOfPhase, -1, "one channel inverted is the other diagonal, and nothing in mono"},
		{TestWide, 0, "two independent streams are a round cloud"},
	}
	for _, c := range cases {
		l, r := gen(c.sig, int(testSR*2))
		got := corr(l, r)
		if math.Abs(got-c.want) > 0.05 {
			t.Errorf("%s: correlation %.3f, want %.1f — %s", TestSignalNames[c.sig], got, c.want, c.why)
		}
	}
}

// The out-of-polarity pair must actually VANISH when summed. That is the
// mono-compatibility failure this signal exists to demonstrate, and a version
// that merely reads −1 on the meter without canceling would demonstrate
// nothing.
func TestOutOfPhaseSumsToSilence(t *testing.T) {
	l, r := gen(TestOutOfPhase, int(testSR))
	sum := make([]float64, len(l))
	for i := range sum {
		sum[i] = (l[i] + r[i]) * 0.5
	}
	if got := rms(sum); got > 1e-9 {
		t.Errorf("summed to mono the pair still has an RMS of %.3g", got)
	}
}

// The channel-identification signals must be in ONE channel and silent in the
// other, or they identify nothing.
func TestChannelIdentificationIsOneSided(t *testing.T) {
	l, r := gen(TestLeftOnly, int(testSR))
	if rms(l) < 0.1 || rms(r) != 0 {
		t.Errorf("left-only: L rms %.3f, R rms %.3f", rms(l), rms(r))
	}
	l, r = gen(TestRightOnly, int(testSR))
	if rms(r) < 0.1 || rms(l) != 0 {
		t.Errorf("right-only: L rms %.3f, R rms %.3f", rms(l), rms(r))
	}
}

// ── The tones ────────────────────────────────────────────────────────────

// zeroCrossRate estimates a sine's frequency by counting upward zero crossings,
// which is independent of the phase arithmetic being tested.
func zeroCrossRate(x []float64, sr float64) float64 {
	n := 0
	for i := 1; i < len(x); i++ {
		if x[i-1] <= 0 && x[i] > 0 {
			n++
		}
	}
	return float64(n) * sr / float64(len(x))
}

func TestReferenceTonesAreAtTheirNamedFrequency(t *testing.T) {
	for _, c := range []struct {
		sig TestSignal
		hz  float64
	}{{TestTone1k, 1000}, {TestTone3150, 3150}, {TestLeftOnly, 1000}} {
		l, _ := gen(c.sig, int(testSR*2))
		if got := zeroCrossRate(l, testSR); math.Abs(got-c.hz) > 2 {
			t.Errorf("%s: measured %.1f Hz, want %.0f", TestSignalNames[c.sig], got, c.hz)
		}
	}
}

// A sine's RMS is its amplitude over √2, and a reference tone whose level is
// not what it says is a reference to nothing.
func TestReferenceToneLevelIsExact(t *testing.T) {
	l, _ := gen(TestTone1k, int(testSR*2))
	if got, want := rms(l), 1/math.Sqrt2; math.Abs(got-want) > 0.005 {
		t.Errorf("a full-scale 1 kHz tone has an RMS of %.4f, want %.4f", got, want)
	}
}

// ── The sweep ────────────────────────────────────────────────────────────

// The sweep has to BE logarithmic, because that is the only reason to prefer it
// to a linear one: equal time per octave, so the bottom of the band gets as many
// cycles as the top.
//
// Checked as a CYCLE COUNT rather than as a frequency. Counting zero crossings
// over a fixed window and calling the answer a frequency works at the top of
// the sweep and is hopeless at the bottom: at 40 Hz a 2048-sample window holds
// under two cycles, so the estimate is quantized to steps of 23 Hz and disagrees
// with the truth by a third before anything is wrong. The number of cycles the
// window should contain is exact at every frequency — it is the phase advanced
// across the window, over 2π — and the only error left is the ±1 crossing that
// falls where the window happens to start.
func TestSweepIsLogarithmicAndCoversTheBand(t *testing.T) {
	g := newTestGen()
	g.sig = TestSweep
	g.level = 1

	const win = 2048
	k := math.Log(sweepHi / sweepLo)
	// The phase the generator integrates, so the expectation is the definition
	// rather than a restatement of the implementation's arithmetic.
	phase := func(pos float64) float64 {
		return 2 * math.Pi * sweepLo * sweepSeconds * (math.Exp(pos*k) - 1) / k
	}
	for _, at := range []float64{0.05, 0.1, 0.3, 0.5, 0.7, 0.9} {
		g.sweep = at
		buf := make([]float64, win)
		for i := range buf {
			buf[i] = g.nextSweep(testSR)
		}
		end := at + win/(sweepSeconds*testSR)
		want := (phase(end) - phase(at)) / (2 * math.Pi)
		got := 0.0
		for i := 1; i < len(buf); i++ {
			if buf[i-1] <= 0 && buf[i] > 0 {
				got++
			}
		}
		if math.Abs(got-want) > 1.5 {
			t.Errorf("at %.0f%% through the sweep (%.0f Hz) the window holds %.0f cycles, want %.2f",
				at*100, sweepFreqAt(at), got, want)
		}
	}
	// ...and the frequencies those cycle counts correspond to really do span
	// the band logarithmically, which is the property the counts are evidence
	// for: an octave takes the same time wherever it is.
	for _, pair := range [][2]float64{{0.1, 0.2}, {0.5, 0.6}, {0.8, 0.9}} {
		ratio := sweepFreqAt(pair[1]) / sweepFreqAt(pair[0])
		if want := math.Pow(sweepHi/sweepLo, 0.1); math.Abs(ratio-want)/want > 1e-9 {
			t.Errorf("a tenth of the pass from %.1f multiplies the frequency by %.4f, want %.4f",
				pair[0], ratio, want)
		}
	}
}
func TestSweepEndsCoverTheAudibleBand(t *testing.T) {
	if lo := sweepFreqAt(0); math.Abs(lo-sweepLo) > 0.001 {
		t.Errorf("the sweep starts at %.2f Hz, not %.0f", lo, sweepLo)
	}
	if hi := sweepFreqAt(1); math.Abs(hi-sweepHi) > 0.1 {
		t.Errorf("the sweep ends at %.1f Hz, not %.0f", hi, sweepHi)
	}
}

// The pass has to come round: a stimulus that runs off the end and stops is one
// that measures once and then reports silence.
func TestSweepRepeats(t *testing.T) {
	g := newTestGen()
	g.sig = TestSweep
	g.level = 1
	for range int(sweepSeconds*testSR) + 100 {
		g.nextSweep(testSR)
	}
	if p := g.SweepPosition(); p < 0 || p >= 1 {
		t.Errorf("after a full pass the sweep position is %v, outside 0..1", p)
	}
	if p := g.SweepPosition(); p > 0.5 {
		t.Errorf("after a full pass and a little the sweep is at %v, so it did not wrap", p)
	}
}

// ── Pink noise ───────────────────────────────────────────────────────────

// psdIn estimates the POWER SPECTRAL DENSITY — power per Hz — inside a band,
// by averaging plain Goertzel periodograms over probe frequencies spread
// log-uniformly through it and over several segments of the signal.
//
// Both kinds of averaging are needed and for the same reason: a single
// periodogram bin of a random signal is chi-squared with two degrees of
// freedom, which scatters by several dB on its own. A first version of this
// averaged twelve probes over one segment and reported adjacent octaves of pink
// noise differing by 5.4 dB when the answer is 3 — the noise was in the
// measurement, not the stimulus. Segments times probes is what brings the
// scatter under a decibel.
//
// DENSITY, not band power: the probes sample the spectrum rather than
// integrating it, so a band twice as wide does not read twice as high. White
// noise is therefore flat here and pink falls at 3 dB per octave, which is the
// definition of each and what the two tests below check.
func psdIn(x []float64, lo, hi float64) float64 {
	const sr = testSR
	const probes, segs = 24, 8
	seg := len(x) / segs
	var total float64
	for s := range segs {
		part := x[s*seg : (s+1)*seg]
		for i := range probes {
			f := lo * math.Pow(hi/lo, (float64(i)+0.5)/probes)
			w := 2 * math.Pi * f / sr
			var re, im float64
			for n, v := range part {
				re += v * math.Cos(w*float64(n))
				im += v * math.Sin(w*float64(n))
			}
			total += (re*re + im*im) / float64(len(part)) / float64(len(part))
		}
	}
	return total / float64(probes*segs)
}

// PINK IS EQUAL POWER PER OCTAVE. That is the property the whole convention of
// room measurement rests on: a flat system reads flat on a fractional-octave
// analyzer only because the stimulus is pink. A generator that called white
// noise pink would make every RTA reading a rising slope nobody could explain.
//
// Measured as a DENSITY, where "equal power per octave" is a 3 dB per octave
// FALL: an octave twice as wide carries the same power only if the power per Hz
// in it is half. That is the same statement read the other way round, and it is
// the one a periodogram can check without integrating anything.
func TestPinkNoiseFallsThreeDBPerOctave(t *testing.T) {
	l, _ := gen(TestPink, 1<<17)
	// Octaves well inside the band, away from the filter's ends.
	a := psdIn(l, 250, 500)
	b := psdIn(l, 500, 1000)
	c := psdIn(l, 1000, 2000)
	for _, p := range []struct {
		name string
		x, y float64
	}{{"250–500 over 500–1k", a, b}, {"500–1k over 1k–2k", b, c}} {
		db := 10 * math.Log10(p.x/p.y)
		if math.Abs(db-3) > 1.2 {
			t.Errorf("%s is %.2f dB; pink noise falls 3 dB per octave", p.name, db)
		}
	}
}

// ...and white noise does NOT, which is what makes the two different positions.
// Its density is flat, so the same measurement reads zero — and if these two
// ever agree, one of them is mislabeled.
func TestWhiteNoiseDensityIsFlat(t *testing.T) {
	l, _ := gen(TestWhite, 1<<17)
	a := psdIn(l, 250, 500)
	b := psdIn(l, 1000, 2000)
	if db := 10 * math.Log10(a/b); math.Abs(db) > 1.2 {
		t.Errorf("white noise falls %.2f dB across two octaves; its density is flat, "+
			"and if it slopes then it is not white", db)
	}
}

// ── The polarity pulse ───────────────────────────────────────────────────

// ASYMMETRY IS THE SIGNAL. A waveform that is its own inverse cannot show
// polarity, which is why a tone will not do — the pulse has to have a
// recognizable direction.
func TestPolarityPulseIsAsymmetric(t *testing.T) {
	l, _ := gen(TestPolarity, int(testSR))
	pos, neg := 0.0, 0.0
	for _, v := range l {
		if v > pos {
			pos = v
		}
		if v < neg {
			neg = v
		}
	}
	if pos < 0.9 {
		t.Errorf("the spike only reaches %.3f; it should be near full scale", pos)
	}
	if ratio := pos / -neg; ratio < 3 {
		t.Errorf("peak %+.3f against %+.3f is a ratio of %.2f — too symmetric to read a direction from",
			pos, neg, ratio)
	}
}

// Equal areas, so the pulse train carries no offset — a polarity signal that
// pushed a woofer off center would be measuring the amplifier's coupling.
func TestPolarityPulseHasNoNetOffset(t *testing.T) {
	l, _ := gen(TestPolarity, int(testSR)) // a whole number of pulse periods
	if m := mean(l); math.Abs(m) > 0.01 {
		t.Errorf("the pulse train has a mean of %.4f", m)
	}
}

// ── The tables ───────────────────────────────────────────────────────────

// A dial whose ring does not match its options is discarded whole by the panel
// builder, which then falls back to the full names and draws the knob over the
// top of them.
func TestSignalTablesLineUp(t *testing.T) {
	if len(TestSignalNames) != TestSignalCount {
		t.Errorf("%d names for %d signals", len(TestSignalNames), TestSignalCount)
	}
	if len(TestSignalRing) != TestSignalCount {
		t.Errorf("%d ring labels for %d signals", len(TestSignalRing), TestSignalCount)
	}
	if len(TestSignalDescs) != TestSignalCount {
		t.Errorf("%d descriptions for %d signals", len(TestSignalDescs), TestSignalCount)
	}
	// Each dial position gets its OWN tooltip. Two positions sharing one
	// description is the failure this is here to catch: a label with nothing of
	// its own to say falls through to the knob's tooltip, and eleven positions
	// that all explain the knob explain none of themselves.
	seen := map[string]int{}
	for i, d := range TestSignalDescs {
		if d == "" {
			t.Errorf("signal %d (%q) has no description", i, TestSignalNames[i])
		}
		if j, dup := seen[d]; dup {
			t.Errorf("signals %d and %d share the description %q", j, i, d)
		}
		seen[d] = i
	}
	for i, s := range TestSignalRing {
		if len(s) > 5 {
			t.Errorf("ring label %d %q is %d runes; five is what fits around the dial", i, s, len(s))
		}
	}
}

// The zero value has to be "off", so a generator nobody has told about test
// signals goes on being three oscillators.
func TestZeroValueIsOff(t *testing.T) {
	var s TestSignal
	if s != TestOff {
		t.Error("the zero TestSignal is not TestOff")
	}
	l, r := gen(TestOff, 256)
	if rms(l) != 0 || rms(r) != 0 {
		t.Error("the off position synthesizes something")
	}
}
