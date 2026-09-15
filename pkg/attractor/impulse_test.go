package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

const irSR = 48000

// irWindow has to cover a WHOLE sweep pass. The sweep takes four seconds to
// cross the band, so a shorter window holds only its bottom end — measured with
// 65536 samples, which is a third of a pass and therefore 20 Hz to about
// 200 Hz, the recovered impulse was a five-millisecond smear and the delay tests
// failed because a band-limited impulse cannot resolve a delay shorter than its
// own width. 262144 samples is 5.5 seconds: a full pass with margin.
const irWindow = 1 << 18

// irSweep builds one pass of the log sweep, which is the stimulus these
// measurements are made with. Taken from the Test module's own generator rather
// than rewritten here, so the analysis is checked against the signal the app
// actually sends.
func irSweep(n int) []float32 {
	out := make([]float32, n)
	src := audiosrc.NewTestSource(audiosrc.TestSweep, irSR)
	src.SetLevel(1)
	src.FillMono(out)
	return out
}

// THE IDENTITY CASE. A system that does nothing has an impulse response that is
// a single spike at time zero of height one. If the deconvolution cannot
// recover that, nothing else it produces means anything.
func TestDeconvolvingAPassthroughGivesASpikeAtZero(t *testing.T) {
	const n = irWindow
	x := irSweep(n)
	ir := ImpulseResponse(x, x, 1e-6)
	if ir == nil {
		t.Fatal("no impulse response")
	}
	at, h := IRPeak(ir)
	if at != 0 {
		t.Errorf("the peak is at sample %d, and a passthrough has no delay", at)
	}
	if math.Abs(h-1) > 0.05 {
		t.Errorf("the peak is %.4f, and a passthrough has a gain of one", h)
	}
	// ...and nothing much anywhere else.
	var rest float64
	for i := 8; i < n-8; i++ {
		if a := math.Abs(ir[i]); a > rest {
			rest = a
		}
	}
	if rest > 0.05 {
		t.Errorf("away from the spike the response reaches %.4f; a wire has nothing there", rest)
	}
}

// A DELAY PUTS THE SPIKE WHERE THE DELAY IS, which is the measurement a
// system-tuning rig makes and the one the transfer display fits from the phase.
// Here it is read straight off the impulse response, which is the other way of
// asking the same question.
func TestADelayMovesTheSpike(t *testing.T) {
	const n = irWindow
	x := irSweep(n)
	for _, d := range []int{1, 17, 240, 1000} {
		y := make([]float32, n)
		copy(y[d:], x[:n-d])
		ir := ImpulseResponse(x, y, 1e-6)
		if ir == nil {
			t.Fatalf("%d samples: no impulse response", d)
		}
		at, h := IRPeak(ir)
		if at != d {
			t.Errorf("a %d-sample delay put the spike at %d", d, at)
		}
		if math.Abs(h-1) > 0.05 {
			t.Errorf("a %d-sample delay has a gain of %.4f, want one", d, h)
		}
	}
}

// A gain scales the spike and nothing else.
func TestAGainScalesTheSpike(t *testing.T) {
	const n = irWindow
	x := irSweep(n)
	for _, g := range []float32{0.25, 0.5, 2} {
		y := make([]float32, n)
		for i := range x {
			y[i] = x[i] * g
		}
		_, h := IRPeak(ImpulseResponse(x, y, 1e-6))
		if rel := math.Abs(h-float64(g)) / float64(g); rel > 0.05 {
			t.Errorf("a gain of %.2f measured %.4f", g, h)
		}
	}
}

// ── Reverberation time ───────────────────────────────────────────────────

// decayingNoise builds an impulse response that decays exponentially with a
// known T60: noise shaped by an envelope whose amplitude falls 60 dB in t60
// seconds. The answer is therefore known by construction, which is the only
// way to check a reverberation time without a room.
func decayingNoise(secs, t60 float64) []float64 {
	n := int(secs * irSR)
	out := make([]float64, n)
	rng := uint32(2463534242)
	for i := range out {
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		v := float64(int32(rng)>>8) / (1 << 23)
		// −60 dB over t60 seconds is an amplitude factor of 10^(−3·t/t60).
		out[i] = v * math.Pow(10, -3*float64(i)/irSR/t60)
	}
	return out
}

// THE HEADLINE TEST. A decay built to fall 60 dB in a known time has to measure
// as that time, and both the T20 and the T30 fits have to agree — they
// extrapolate from different stretches of the same line, so a disagreement
// means the line is not straight and the measurement is not a reverberation
// time.
func TestReverbTimeRecoversAKnownDecay(t *testing.T) {
	for _, t60 := range []float64{0.3, 0.8, 1.5} {
		ir := decayingNoise(t60*2.5, t60)
		decay := SchroederDecay(ir)
		t20, ok20 := ReverbTime(decay, irSR, -5, -25)
		t30, ok30 := ReverbTime(decay, irSR, -5, -35)
		if !ok20 || !ok30 {
			t.Fatalf("T60 %.1f s: T20 ok=%v, T30 ok=%v", t60, ok20, ok30)
		}
		for _, c := range []struct {
			name string
			got  float64
		}{{"T20", t20}, {"T30", t30}} {
			if rel := math.Abs(c.got-t60) / t60; rel > 0.08 {
				t.Errorf("a %.2f s decay measured %s = %.3f s", t60, c.name, c.got)
			}
		}
	}
}

// Schroeder's integration is what makes the curve fittable at all: the decay of
// one impulse response is a noisy ring, and the backward energy integral is the
// ensemble average of infinitely many of them. So the curve has to be
// monotonic — energy remaining can only fall as the start moves later.
func TestSchroederDecayIsMonotonic(t *testing.T) {
	decay := SchroederDecay(decayingNoise(2, 0.8))
	for i := 1; i < len(decay); i++ {
		if decay[i] > decay[i-1]+1e-9 {
			t.Fatalf("the decay rose at sample %d: %.4f after %.4f", i, decay[i], decay[i-1])
		}
	}
	if decay[0] != 0 {
		t.Errorf("the decay starts at %.4f dB, and it is normalized to start at 0", decay[0])
	}
}

// A measurement whose decay never reaches the range asked for cannot produce a
// reverberation time, and extrapolating one from three decibels of fall would
// be inventing it.
func TestReverbTimeRefusesAnInsufficientDecay(t *testing.T) {
	if _, ok := ReverbTime(nil, irSR, -5, -25); ok {
		t.Error("an empty decay produced a reverberation time")
	}
	if _, ok := ReverbTime(SchroederDecay(decayingNoise(2, 0.5)), irSR, -35, -5); ok {
		t.Error("the fit points the wrong way round produced a reverberation time")
	}
}

// ── The waterfall ────────────────────────────────────────────────────────

// A RESONANCE IS WHAT A WATERFALL IS FOR. A frequency response cannot tell a
// resonance from a broad lift — they look identical in magnitude — and they
// look nothing alike in time, which is exactly what this surface shows.
//
// The impulse response here is a decaying sine at one frequency: a pure
// resonance. That frequency has to still be there in the later slices while
// everything else has gone.
func TestTheWaterfallShowsAResonanceRingingOn(t *testing.T) {
	const n, res = 1 << 14, 1000.0
	ir := make([]float64, n)
	for i := range ir {
		tt := float64(i) / irSR
		ir[i] = math.Exp(-tt/0.05) * math.Sin(2*math.Pi*res*tt)
	}
	freqs := LogFreqPoints(100, 10000, 96)
	slices := CSD(ir, irSR, 8, 5, 4096, freqs)
	if len(slices) < 4 {
		t.Fatalf("only %d slices", len(slices))
	}
	// The nearest point to the resonance, and one well away from it.
	near, far := 0, 0
	for i, f := range freqs {
		if math.Abs(math.Log(f/res)) < math.Abs(math.Log(freqs[near]/res)) {
			near = i
		}
		if math.Abs(math.Log(f/5000)) < math.Abs(math.Log(freqs[far]/5000)) {
			far = i
		}
	}
	last := slices[len(slices)-1]
	if last.DB[near] < last.DB[far]+20 {
		t.Errorf("after %.0f ms the resonance is at %.1f dB and 5 kHz at %.1f; "+
			"the resonance should still be ringing", last.TimeMS, last.DB[near], last.DB[far])
	}
	// ...and it decays, rather than the surface being flat in time.
	if slices[0].DB[near]-last.DB[near] < 6 {
		t.Errorf("the resonance fell only %.1f dB across the whole surface",
			slices[0].DB[near]-last.DB[near])
	}
}

// The front of the surface is 0 dB, because everything is read as decay from
// the loudest point.
func TestTheWaterfallIsNormalizedToItsPeak(t *testing.T) {
	ir := decayingNoise(0.5, 0.2)
	freqs := LogFreqPoints(100, 10000, 64)
	slices := CSD(ir, irSR, 6, 4, 2048, freqs)
	if len(slices) == 0 {
		t.Fatal("no slices")
	}
	best := math.Inf(-1)
	for _, s := range slices {
		for _, v := range s.DB {
			if v > best {
				best = v
			}
		}
	}
	if math.Abs(best) > 1e-9 {
		t.Errorf("the loudest point of the surface is %.6f dB, and it is normalized to 0", best)
	}
}

// Rubbish in is refused rather than panicking or returning a surface made of
// nothing.
func TestCSDRefusesBadInput(t *testing.T) {
	freqs := LogFreqPoints(100, 10000, 32)
	for _, c := range []struct {
		ir     []float64
		sr     int
		slices int
		fft    int
	}{
		{nil, irSR, 4, 1024}, {make([]float64, 100), 0, 4, 1024},
		{make([]float64, 100), irSR, 0, 1024}, {make([]float64, 100), irSR, 4, 1000},
	} {
		if s := CSD(c.ir, c.sr, c.slices, 5, c.fft, freqs); s != nil {
			t.Errorf("CSD accepted ir=%d sr=%d slices=%d fft=%d", len(c.ir), c.sr, c.slices, c.fft)
		}
	}
	if ImpulseResponse(make([]float32, 1000), make([]float32, 1000), 1e-6) != nil {
		t.Error("a non-power-of-two length was accepted")
	}
	if ImpulseResponse(make([]float32, 1024), make([]float32, 512), 1e-6) != nil {
		t.Error("mismatched lengths were accepted")
	}
}

// The frequency axis is logarithmic, because hearing is organised in ratios and
// a linear axis spends nine-tenths of the width above 2 kHz.
func TestLogFreqPointsAreLogarithmic(t *testing.T) {
	f := LogFreqPoints(20, 20000, 100)
	if len(f) != 100 {
		t.Fatalf("%d points", len(f))
	}
	if math.Abs(f[0]-20) > 1e-9 || math.Abs(f[99]-20000) > 1e-6 {
		t.Errorf("the axis runs %.3f..%.1f Hz", f[0], f[99])
	}
	r := f[1] / f[0]
	for i := 2; i < len(f); i++ {
		if rel := math.Abs(f[i]/f[i-1]-r) / r; rel > 1e-9 {
			t.Fatalf("the step from point %d is %.9f, and the first was %.9f", i, f[i]/f[i-1], r)
		}
	}
	if LogFreqPoints(20, 20000, 1) != nil || LogFreqPoints(0, 100, 10) != nil {
		t.Error("a degenerate axis was accepted")
	}
}

// The inverse transform has to undo the forward one, which is what everything
// above rests on.
func TestInverseFFTUndoesTheForward(t *testing.T) {
	const n = 1024
	x := make([]float32, n)
	rng := uint32(12345)
	for i := range x {
		rng = rng*1664525 + 1013904223
		x[i] = float32(float64(int32(rng)>>8) / (1 << 23))
	}
	re := make([]float64, n/2+1)
	im := make([]float64, n/2+1)
	if !computeFFTComplex(x, winRectangular, re, im) {
		t.Fatal("the forward transform was refused")
	}
	back := make([]float64, n)
	if !inverseFFTReal(re, im, n, back) {
		t.Fatal("the inverse transform was refused")
	}
	for i := range x {
		if math.Abs(back[i]-float64(x[i])) > 1e-9 {
			t.Fatalf("sample %d came back as %.12f, was %.12f", i, back[i], float64(x[i]))
		}
	}
}

// THE TRUNCATION RAMP, recorded rather than hidden. A Schroeder integral of a
// truncated response always falls away at its end — the energy remaining after
// the last sample is zero however loud the signal was — so even a signal that
// does not decay produces a curve, and a line fitted to it returns a number.
//
// This is the limitation ReverbTime documents and does not solve; the usual
// answer is Lundeby's method, finding the noise floor and truncating there. The
// test exists so that the behaviour is a known quantity rather than a surprise,
// and so that a future fix has something to change.
//
// It also records something worse than the limitation itself: the obvious check
// does not catch it. A T20 and a T30 that disagree are usually taken as the sign
// that a fit has run into the noise floor, and on a pure truncation ramp they
// AGREE — so consistency between them is not evidence that the number is real.
// That was asserted the other way round when this test was written, and the test
// is what showed it to be false.
func TestATruncatedSignalShowsASpuriousDecay(t *testing.T) {
	flat := make([]float64, irSR)
	for i := range flat {
		flat[i] = 1
	}
	decay := SchroederDecay(flat)
	t20, ok20 := ReverbTime(decay, irSR, -5, -25)
	t30, ok30 := ReverbTime(decay, irSR, -5, -35)
	if !ok20 || !ok30 {
		t.Skip("the truncation ramp did not reach the fit range on this length")
	}
	t.Logf("a constant signal yields T20 %.3f s and T30 %.3f s from its truncation ramp alone",
		t20, t30)
	if t20 <= 0 || t30 <= 0 {
		t.Errorf("the truncation ramp gave T20 %.3f, T30 %.3f; the point is that it gives "+
			"plausible numbers", t20, t30)
	}
	// THE UNCOMFORTABLE PART: they agree, so agreement proves nothing.
	if rel := math.Abs(t20-t30) / t30; rel > 0.15 {
		t.Errorf("T20 %.3f and T30 %.3f differ by %.1f%% on a truncation ramp. If that is "+
			"now true, the disagreement HAS become a usable warning and this test and "+
			"ReverbTime's comment should both say so", t20, t30, rel*100)
	}
	// ...and a genuine decay also has them agreeing, which is the whole problem:
	// the two cases are indistinguishable by this check.
	real20, _ := ReverbTime(SchroederDecay(decayingNoise(2, 0.6)), irSR, -5, -25)
	real30, _ := ReverbTime(SchroederDecay(decayingNoise(2, 0.6)), irSR, -5, -35)
	if rel := math.Abs(real20-real30) / real30; rel > 0.08 {
		t.Errorf("on a genuine 0.6 s decay T20 %.3f and T30 %.3f differ by %.1f%%",
			real20, real30, rel*100)
	}
}

// TestSpectrumPointsLevel checks the live surface reads an absolute level: a
// full-scale tone at 0 dB, half of it 6 dB down, and nothing where there is
// nothing.
//
// The absolute part is the point of the test rather than a detail of it. A CSD
// is normalized to its own loudest point because it is read as decay; a live
// waterfall is read as LEVEL, and one that normalized itself would draw a quiet
// passage at the same height as a loud one.
func TestSpectrumPointsLevel(t *testing.T) {
	const (
		n  = 4096
		sr = 48000
	)
	freqs := LogFreqPoints(20, 20000, 96)
	// The axis point nearest 1 kHz, and then the BIN CENTRE nearest that point,
	// which is the tone the test uses.
	//
	// Both steps matter and neither is the function being lenient. The frequency
	// axis is logarithmic, so 1000 Hz is not on it — the nearest point is 1015 —
	// and a tone read three bins around the wrong centre is 1.2 dB light, which
	// is scalloping loss and is a true fact about a 96-point axis over a 4096-bin
	// transform rather than an error in the scaling this test is checking.
	near := 0
	for i, v := range freqs {
		if math.Abs(math.Log(v/1000)) < math.Abs(math.Log(freqs[near]/1000)) {
			near = i
		}
	}
	binHz := float64(sr) / float64(n)
	f := math.Round(freqs[near]/binHz) * binHz
	out := make([]float64, len(freqs))
	for _, amp := range []float64{1.0, 0.5} {
		buf := make([]float32, n)
		for i := range buf {
			buf[i] = float32(amp * math.Sin(2*math.Pi*f*float64(i)/sr))
		}
		if !SpectrumPoints(buf, sr, freqs, winHann, out) {
			t.Fatalf("SpectrumPoints refused a %d-sample block", n)
		}
		// AMPLITUDE-referenced, which is the convention rta.go already reads in and
		// the one digital metering uses: a full-scale sine is 0 dBFS, not the
		// -3.01 its RMS power would give. The two displays have to agree or a band
		// that reads -20 on the RTA reads -23 on the waterfall beside it.
		want := 20 * math.Log10(amp)
		if got := out[near]; math.Abs(got-want) > 0.6 {
			t.Errorf("amplitude %.2f: %.2f dB at %.0f Hz, want %.2f", amp, got, freqs[near], want)
		}
		// Well away from the tone there is nothing. Three octaves down, which is
		// past any window's skirt.
		far := 0
		for i, v := range freqs {
			if v > f/8 {
				break
			}
			far = i
		}
		if out[far] > -60 {
			t.Errorf("amplitude %.2f: %.2f dB at %.0f Hz, want silence", amp, out[far], freqs[far])
		}
	}
}

// TestSpectrumPointsRejects checks the sizes it will not accept, because a
// non-power-of-two block is a silent wrong answer from the FFT rather than an
// error.
func TestSpectrumPointsRejects(t *testing.T) {
	freqs := LogFreqPoints(20, 20000, 16)
	out := make([]float64, len(freqs))
	if SpectrumPoints(make([]float32, 1000), 48000, freqs, winHann, out) {
		t.Error("accepted a block that is not a power of two")
	}
	if SpectrumPoints(make([]float32, 1024), 0, freqs, winHann, out) {
		t.Error("accepted a zero sample rate")
	}
	if SpectrumPoints(make([]float32, 1024), 48000, freqs, winHann, out[:4]) {
		t.Error("accepted an output shorter than the frequency axis")
	}
}

// TestSpectrumPointsTonesBetweenAxisPoints is the regression for a tone that
// vanished.
//
// A steady 3150 Hz sine drew a perfectly flat live surface. The axis points
// either side of it are 3018 and 3243 Hz, the tone is eleven bins from either,
// and the sampling read three bins around each point — so nothing on the axis
// was looking anywhere near it. 1 kHz worked, which was the trap: it lands
// close enough to a point to leak into it, so the display looked correct and
// was reading almost nothing.
//
// Every tone the app can generate has to raise its own band well clear of the
// rest of the axis, wherever it happens to fall between the points.
func TestSpectrumPointsTonesBetweenAxisPoints(t *testing.T) {
	const (
		n  = 4096
		sr = 48000
	)
	freqs := LogFreqPoints(20, 20000, 96)
	out := make([]float64, len(freqs))
	for _, f := range []float64{100, 440, 1000, 3150, 5000, 9000} {
		buf := make([]float32, n)
		for i := range buf {
			buf[i] = float32(math.Sin(2 * math.Pi * f * float64(i) / sr))
		}
		if !SpectrumPoints(buf, sr, freqs, winHann, out) {
			t.Fatalf("%.0f Hz: SpectrumPoints refused the block", f)
		}
		near, peak := 0, math.Inf(-1)
		for i, v := range out {
			if v > peak {
				near, peak = i, v
			}
		}
		// The peak of the axis has to be at the tone, within one point either
		// side — the tone is between two points and either may hold more of it.
		want := 0
		for i, v := range freqs {
			if math.Abs(math.Log(v/f)) < math.Abs(math.Log(freqs[want]/f)) {
				want = i
			}
		}
		if near < want-1 || near > want+1 {
			t.Errorf("%.0f Hz: the axis peaks at %.0f Hz, want near %.0f Hz",
				f, freqs[near], freqs[want])
		}
		// And it has to be a PEAK, not a rounding error: a full-scale tone is
		// the whole of the signal, so everything else on the axis is leakage
		// and sits far below it.
		if peak < -3 {
			t.Errorf("%.0f Hz: peak reads %.1f dB, want a full-scale tone near 0", f, peak)
		}
		// Everything more than a third of an octave away, which is the window's
		// own skirt plus the points that share a bin with the tone. At the bottom
		// of the axis the points are closer together than the transform can
		// resolve — 7.5% at 100 Hz is 7.5 Hz and the bins are 11.7 — so several
		// adjacent points legitimately read the same tone. That is the resolution
		// saying so, not the sampling missing it.
		var second float64 = -1e9
		for i, v := range out {
			if math.Abs(math.Log2(freqs[i]/f)) < 1.0/3 {
				continue
			}
			if v > second {
				second = v
			}
		}
		if peak-second < 30 {
			t.Errorf("%.0f Hz: tone is only %.1f dB above the rest of the axis", f, peak-second)
		}
	}
}
