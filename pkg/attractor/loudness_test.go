package attractor

import (
	"math"
	"testing"
)

const ldSR = 48000

// THE FILTER IS THE MEASUREMENT. Every LUFS number is the mean square of the
// K-weighted signal, so a weighting whose corners have moved is a meter that is
// wrong everywhere and says nothing about it.
//
// BS.1770-4 prints the coefficients at 48 kHz. This app derives them from the
// analog prototype at whatever rate the audio is arriving at, because the
// printed numbers are only correct at 48 kHz and a meter that used them at 44.1
// would be weighting by a different filter — quietly. Reproducing the published
// numbers exactly at 48 kHz is what makes the derivation the same filter rather
// than a similar one.
func TestKWeightingMatchesThePublishedCoefficients(t *testing.T) {
	// ITU-R BS.1770-4, Tables 1 and 2.
	wantShelf := biquad{
		b0: 1.53512485958697, b1: -2.69169618940638, b2: 1.19839281085285,
		a1: -1.69065929318241, a2: 0.73248077421585,
	}
	wantHP := biquad{
		b0: 1.0, b1: -2.0, b2: 1.0,
		a1: -1.99004745483398, a2: 0.99007225036621,
	}
	got := kWeightingShelf(ldSR)
	for _, c := range []struct {
		name     string
		got, wnt float64
	}{
		{"shelf b0", got.b0, wantShelf.b0}, {"shelf b1", got.b1, wantShelf.b1},
		{"shelf b2", got.b2, wantShelf.b2}, {"shelf a1", got.a1, wantShelf.a1},
		{"shelf a2", got.a2, wantShelf.a2},
	} {
		if math.Abs(c.got-c.wnt) > 1e-11 {
			t.Errorf("%s = %.14f, BS.1770 says %.14f", c.name, c.got, c.wnt)
		}
	}
	gotHP := kWeightingHighpass(ldSR)
	for _, c := range []struct {
		name     string
		got, wnt float64
	}{
		{"hp b0", gotHP.b0, wantHP.b0}, {"hp b1", gotHP.b1, wantHP.b1},
		{"hp b2", gotHP.b2, wantHP.b2},
		{"hp a1", gotHP.a1, wantHP.a1}, {"hp a2", gotHP.a2, wantHP.a2},
	} {
		if math.Abs(c.got-c.wnt) > 1e-10 {
			t.Errorf("%s = %.14f, BS.1770 says %.14f", c.name, c.got, c.wnt)
		}
	}
}

// ...and the filter has to keep its shape at other rates, which is the whole
// reason for deriving it. The shelf's lift and the high-pass's corner are
// properties of the analog prototype, not of the sample rate.
func TestKWeightingKeepsItsShapeAtEveryRate(t *testing.T) {
	for _, sr := range []float64{44100, 48000, 88200, 96000} {
		// The shelf is about +4 dB well above its corner...
		if g := biquadGainDB(kWeightingShelf(sr), 10000, sr); math.Abs(g-kShelfG) > 0.15 {
			t.Errorf("%.0f Hz: the shelf reaches %.3f dB at 10 kHz, want %.3f", sr, g, kShelfG)
		}
		// ...and the high-pass is at its own Q at the corner frequency, which for
		// this nearly-critically-damped section is −6 dB and not the −3 a
		// first version of this test assumed. A second-order high-pass evaluated
		// at its corner has |H| = Q exactly, so with Q = 0.5003 the corner sits
		// 6.02 dB down and the −3 dB point is somewhere else entirely. Checking
		// it against the design parameter is checking the filter against its own
		// specification rather than against a rule of thumb for a different Q.
		wantDB := 20 * math.Log10(kHPQ)
		if g := biquadGainDB(kWeightingHighpass(sr), kHPF0, sr); math.Abs(g-wantDB) > 0.1 {
			t.Errorf("%.0f Hz: the high-pass is %.3f dB at its own corner, want %.3f (its Q)",
				sr, g, wantDB)
		}
	}
}

// ldTone builds a stereo sine.
func ldTone(n int, hz, amp float64, rightAmp float64) ([]float32, []float32) {
	l := make([]float32, n)
	r := make([]float32, n)
	for i := range l {
		v := math.Sin(2 * math.Pi * hz * float64(i) / ldSR)
		l[i] = float32(amp * v)
		r[i] = float32(rightAmp * v)
	}
	return l, r
}

// feed runs a signal through a fresh meter.
func feed(l, r []float32) LoudnessResult {
	m := NewLoudnessMeter(ldSR)
	m.Add(l, r)
	return m.Result()
}

// THE ABSOLUTE ANCHOR. BS.1770's loudness is −0.691 + 10·log₁₀ of the summed
// weighted mean square, so for a sine of known amplitude at a frequency whose
// K-weighting gain is known, the answer is arithmetic — and the weighting gain
// comes from the filter's own coefficients rather than from a table, so this
// checks the integrator and the filter against each other.
func TestASineReadsTheLoudnessTheFormulaPredicts(t *testing.T) {
	const secs = 10
	for _, c := range []struct{ hz, amp float64 }{
		{1000, 0.5}, {1000, 0.1}, {100, 0.25}, {5000, 0.3}, {997, 0.7},
	} {
		l, r := ldTone(ldSR*secs, c.hz, c.amp, c.amp)
		got := feed(l, r).Integrated
		// The weighted mean square of one channel is (A²/2)·|H|², and both
		// channels carry it with a weight of 1.
		g := math.Pow(10, KWeightingGainDB(c.hz, ldSR)/20)
		ms := 2 * (c.amp * c.amp / 2) * g * g
		want := loudnessOffset + 10*math.Log10(ms)
		if math.Abs(got-want) > 0.1 {
			t.Errorf("%.0f Hz at %.2f: measured %.2f LUFS, the formula says %.2f", c.hz, c.amp, got, want)
		}
	}
}

// Doubling the amplitude is exactly 6.02 louder, because loudness is a power
// ratio in decibels and nothing in the chain is nonlinear.
func TestDoublingAmplitudeIsSixDecibels(t *testing.T) {
	a, _ := ldTone(ldSR*8, 1000, 0.2, 0.2)
	b, _ := ldTone(ldSR*8, 1000, 0.4, 0.4)
	la := feed(a, a).Integrated
	lb := feed(b, b).Integrated
	if d := lb - la; math.Abs(d-6.0206) > 0.02 {
		t.Errorf("doubling the amplitude changed the loudness by %.4f LU, want 6.0206", d)
	}
}

// The same signal in both channels is 3.01 louder than in one, because the
// channel powers are SUMMED and not averaged. Getting this backwards is how a
// meter comes to read every stereo program 3 dB quiet.
func TestBothChannelsAreThreeDecibelsLouderThanOne(t *testing.T) {
	l, r := ldTone(ldSR*8, 1000, 0.3, 0.3)
	both := feed(l, r).Integrated
	silent := make([]float32, len(r))
	one := feed(l, silent).Integrated
	if d := both - one; math.Abs(d-3.0103) > 0.02 {
		t.Errorf("two channels measured %.4f LU above one, want 3.0103", d)
	}
}

// THE GATE IS WHAT MAKES THE INTEGRATED NUMBER MEAN ANYTHING. Silence between
// passages must not drag a programme's loudness down, so quiet blocks are
// thrown away before the average — and a measurement that included them would
// report a number nobody could hit by mixing.
func TestSilenceDoesNotDragTheIntegratedLoudnessDown(t *testing.T) {
	const secs = 6
	loud, _ := ldTone(ldSR*secs, 1000, 0.5, 0.5)
	alone := feed(loud, loud).Integrated

	// The same program with an equal length of silence after it.
	l := append(append([]float32{}, loud...), make([]float32, ldSR*secs)...)
	withGap := feed(l, l).Integrated

	// A tenth of a decibel of movement is expected and correct: the gating
	// blocks are 400 ms with a 100 ms step, so three of them straddle the
	// boundary and are genuinely part tone and part silence. They carry
	// three quarters, a half and a quarter of the power, all well above the
	// relative gate, and three such blocks among sixty pull the mean down by
	// 10·log₁₀(58.5/60) = 0.11 dB. That is the standard working, not the
	// meter failing — an ungated average over the same audio would have moved
	// it by three decibels.
	if math.Abs(withGap-alone) > 0.25 {
		t.Errorf("adding six seconds of silence moved the integrated loudness from %.2f to %.2f LUFS",
			alone, withGap)
	}
	// ...and an UNGATED average would have moved it by about 3 dB, which is
	// what the gate is for. The momentary reading, which is ungated by
	// definition, does follow the silence down.
	if m := feed(l, l).Momentary; m > absGateLUFS {
		t.Errorf("the momentary reading at the end of six seconds of silence is %.1f LUFS", m)
	}
}

// The relative gate goes further: a quiet passage that is not silence is still
// excluded, so a programme's number describes its program material rather
// than an average with the intro in it.
func TestAQuietPassageIsGatedOut(t *testing.T) {
	const secs = 6
	loud, _ := ldTone(ldSR*secs, 1000, 0.5, 0.5)
	quiet, _ := ldTone(ldSR*secs, 1000, 0.5*math.Pow(10, -20.0/20), 0.5*math.Pow(10, -20.0/20))
	alone := feed(loud, loud).Integrated
	l := append(append([]float32{}, loud...), quiet...)
	together := feed(l, l).Integrated
	if math.Abs(together-alone) > 0.3 {
		t.Errorf("a passage 20 LU quieter moved the integrated loudness from %.2f to %.2f LUFS",
			alone, together)
	}
}

// LRA IS THE RANGE, so a signal that never changes level has none...
func TestLRAOfAConstantSignalIsNothing(t *testing.T) {
	l, r := ldTone(ldSR*20, 1000, 0.4, 0.4)
	if lra := feed(l, r).LRA; lra > 1 {
		t.Errorf("a constant tone has a loudness range of %.2f LU", lra)
	}
}

// ...and one that alternates between two levels has about the difference
// between them.
func TestLRAFindsTheSpreadBetweenTwoLevels(t *testing.T) {
	const secs, spread = 6, 15.0
	hi, _ := ldTone(ldSR*secs, 1000, 0.5, 0.5)
	lo, _ := ldTone(ldSR*secs, 1000, 0.5*math.Pow(10, -spread/20), 0.5*math.Pow(10, -spread/20))
	var l []float32
	for i := 0; i < 3; i++ {
		l = append(l, hi...)
		l = append(l, lo...)
	}
	got := feed(l, l).LRA
	if math.Abs(got-spread) > 3 {
		t.Errorf("two levels %.0f LU apart gave a loudness range of %.2f LU", spread, got)
	}
}

// ── True peak ────────────────────────────────────────────────────────────

// THE CASE TRUE PEAK EXISTS FOR. A full-scale sine at a quarter of the sample
// rate, landing halfway between samples, has every sample at 0.7071 and reaches
// 1.0 in between — so a sample peak meter reads −3 dBFS while the converter is
// clipping. A delivery spec's ceiling is a TRUE peak ceiling because of exactly
// this signal.
func TestTruePeakFindsTheOvershootBetweenSamples(t *testing.T) {
	n := 4096
	x := make([]float32, n)
	for i := range x {
		// fs/4 with a 45° offset: samples land on ±√½ and never on the crest.
		x[i] = float32(math.Sin(2*math.Pi*0.25*float64(i) + math.Pi/4))
	}
	samplePeak := 0.0
	for _, v := range x {
		if a := math.Abs(float64(v)); a > samplePeak {
			samplePeak = a
		}
	}
	if math.Abs(samplePeak-math.Sqrt2/2) > 1e-3 {
		t.Fatalf("the test signal's sample peak is %.4f, and the case needs it at 0.7071", samplePeak)
	}
	tp := TruePeak(x)
	if tp < 0.98 || tp > 1.05 {
		t.Errorf("true peak measured %.4f (%.2f dBTP); the signal reaches 1.0 between samples",
			tp, 20*math.Log10(tp))
	}
	if tp <= samplePeak+0.05 {
		t.Errorf("true peak %.4f is no higher than the sample peak %.4f — the oversampling "+
			"found nothing, which is the one thing it is for", tp, samplePeak)
	}
}

// A signal whose peaks ARE on samples must not be inflated: an oversampler that
// overshoots on everything is a meter that fails every delivery spec.
func TestTruePeakDoesNotInventAnOvershoot(t *testing.T) {
	n := 4096
	x := make([]float32, n)
	for i := range x {
		x[i] = float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/ldSR))
	}
	if tp := TruePeak(x); tp > 0.53 {
		t.Errorf("a 0.5-amplitude tone measured a true peak of %.4f", tp)
	}
}

// Silence has no peak and no loudness, and both have to come back as the floor
// rather than as a NaN or a negative infinity nobody can put on a readout.
func TestSilenceIsTheFloorEverywhere(t *testing.T) {
	z := make([]float32, ldSR*5)
	r := feed(z, z)
	if r.Momentary != LoudnessFloor || r.ShortTerm != LoudnessFloor {
		t.Errorf("silence read M %.1f S %.1f", r.Momentary, r.ShortTerm)
	}
	if r.OK {
		t.Errorf("silence produced an integrated reading of %.1f LUFS", r.Integrated)
	}
	if r.TruePeak != LoudnessFloor {
		t.Errorf("silence has a true peak of %.1f dBTP", r.TruePeak)
	}
	if tp := TruePeak(z); tp != 0 {
		t.Errorf("TruePeak of silence is %v", tp)
	}
}

// A measurement shorter than one gating block cannot produce an integrated
// number, and saying so beats reporting one made of half a block.
func TestTooShortIsRefused(t *testing.T) {
	l, r := ldTone(ldSR/10, 1000, 0.5, 0.5) // 100 ms: one block, four are needed
	if res := feed(l, r); res.OK {
		t.Errorf("100 ms produced an integrated reading of %.2f LUFS", res.Integrated)
	}
}

// The meter has to retune when the source's rate changes, or it goes on
// weighting by a filter built for a rate that is no longer arriving.
func TestResetRetunesTheFilter(t *testing.T) {
	m := NewLoudnessMeter(48000)
	before := m.shelf
	m.Reset(44100)
	if m.SampleRate() != 44100 {
		t.Errorf("the meter reports %d Hz after a reset to 44100", m.SampleRate())
	}
	if m.shelf == before {
		t.Error("the filter coefficients did not change with the sample rate")
	}
	if want := kWeightingShelf(44100); m.shelf != want {
		t.Error("the retuned filter is not the one the derivation gives for 44100")
	}
}

// THE STANDARD'S OWN CALIBRATION POINT, and the neatest check there is that the
// filter and the offset belong to each other.
//
// K-weighting's gain at 997 Hz is +0.691 dB, and the loudness equation's offset
// is −0.691 dB. They are the same number because the standard chose the offset
// to cancel the weighting at its reference frequency — so a 997 Hz sine of
// amplitude A in BOTH channels reads exactly 20·log₁₀(A) LUFS, with the two
// channels' 3.01 dB and the half in a sine's mean square canceling as well.
//
// Every part of the chain has to be right for that to come out: the filter, the
// offset, the channel summing and the mean square. It is also why 997 Hz rather
// than 1000 appears throughout BS.1770 — at 1 kHz the weighting is +0.698 dB
// and the identity is off by seven thousandths of a decibel.
func TestA997HzSineReadsItsOwnAmplitudeInLUFS(t *testing.T) {
	if g := KWeightingGainDB(997, ldSR); math.Abs(g+loudnessOffset) > 0.002 {
		t.Errorf("K-weighting at 997 Hz is %+.4f dB and the loudness offset is %+.4f; "+
			"the two are meant to cancel", g, loudnessOffset)
	}
	for _, amp := range []float64{0.5, 0.25, 0.1} {
		l, r := ldTone(ldSR*8, 997, amp, amp)
		got := feed(l, r).Integrated
		want := 20 * math.Log10(amp)
		if math.Abs(got-want) > 0.05 {
			t.Errorf("a 997 Hz sine of amplitude %.3f reads %.3f LUFS, want %.3f (its own dBFS)",
				amp, got, want)
		}
	}
}
