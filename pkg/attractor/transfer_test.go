package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

const xfSR = 48000

// xfRun feeds `windows` windows of a stimulus through a system and measures it.
// system is handed the reference and fills the measurement, which is how a gain,
// a delay or a filter is expressed for these tests.
func xfRun(sig audiosrc.TestSignal, windows, n int, system func(ref, meas []float32)) TransferResult {
	src := audiosrc.NewTestSource(sig, xfSR)
	src.SetLevel(0.8)
	var a TransferAccum
	// A continuous stream cut into windows, because a delay reaches BACK past
	// the start of a window and a test that generated each window separately
	// would be measuring a delay into silence.
	total := make([]float32, windows*n+n)
	src.FillMono(total)
	ref := make([]float32, n)
	meas := make([]float32, n)
	for w := 0; w < windows; w++ {
		copy(ref, total[w*n+n:(w+1)*n+n])
		system(ref, meas)
		a.Add(ref, meas, xfWindowKind)
	}
	return a.Result(xfSR, 6)
}

// passthrough is the identity system: what went in came back.
func passthrough(ref, meas []float32) { copy(meas, ref) }

// gainOf builds a system that multiplies by g.
func gainOf(g float32) func(ref, meas []float32) {
	return func(ref, meas []float32) {
		for i := range ref {
			meas[i] = ref[i] * g
		}
	}
}

// usable collects the bands whose coherence says the numbers mean something,
// inside a frequency range the window can actually resolve.
func usable(r TransferResult, lo, hi, minCoh float64) []int {
	var out []int
	for i, b := range r.Bands {
		if b.Center >= lo && b.Center <= hi && r.Coherence[i] >= minCoh {
			out = append(out, i)
		}
	}
	return out
}

// A SYSTEM THAT DOES NOTHING measures as nothing: 0 dB, 0°, coherence 1. If the
// instrument cannot report the identity correctly then every reading it gives
// of a real system is that error plus the system.
func TestPassthroughMeasuresFlatAndInPhase(t *testing.T) {
	r := xfRun(audiosrc.TestPink, 24, 4096, passthrough)
	if !r.OK {
		t.Fatalf("no result after %d windows", r.Averages)
	}
	idx := usable(r, 100, 10000, 0.9)
	if len(idx) < 10 {
		t.Fatalf("only %d usable bands", len(idx))
	}
	for _, i := range idx {
		if math.Abs(r.MagDB[i]) > 0.1 {
			t.Errorf("%.0f Hz: passthrough measured %.3f dB", r.Bands[i].Center, r.MagDB[i])
		}
		if math.Abs(r.PhaseDeg[i]) > 1 {
			t.Errorf("%.0f Hz: passthrough measured %.2f°", r.Bands[i].Center, r.PhaseDeg[i])
		}
		if r.Coherence[i] < 0.99 {
			t.Errorf("%.0f Hz: passthrough coherence %.4f", r.Bands[i].Center, r.Coherence[i])
		}
	}
}

// A known gain has to come back as that gain, at every frequency.
func TestAGainMeasuresAsThatGain(t *testing.T) {
	for _, g := range []float32{0.5, 2, 0.1} {
		r := xfRun(audiosrc.TestPink, 24, 4096, gainOf(g))
		want := 20 * math.Log10(float64(g))
		for _, i := range usable(r, 100, 10000, 0.9) {
			if math.Abs(r.MagDB[i]-want) > 0.1 {
				t.Errorf("gain %.2f at %.0f Hz: measured %.3f dB, want %.3f",
					g, r.Bands[i].Center, r.MagDB[i], want)
			}
		}
	}
}

// THE TEST THIS DISPLAY EXISTS FOR. A pure delay is a phase that falls linearly
// with frequency, and the slope is the delay — which is how a system-tuning rig
// measures the offset between a loudspeaker and a microphone. Recovering a
// delay that was put in is the whole claim.
func TestADelayIsRecoveredFromThePhaseSlope(t *testing.T) {
	for _, samples := range []int{16, 48, 120} {
		wantMS := float64(samples) / xfSR * 1000
		// The delayed copy reaches back before the window, which is why xfRun
		// generates one continuous stream and hands out overlapping slices.
		var stream []float32
		src := audiosrc.NewTestSource(audiosrc.TestPink, xfSR)
		src.SetLevel(0.8)
		const n, windows = 4096, 24
		stream = make([]float32, windows*n+n)
		src.FillMono(stream)
		var a TransferAccum
		ref := make([]float32, n)
		meas := make([]float32, n)
		for w := 0; w < windows; w++ {
			base := w*n + n
			copy(ref, stream[base:base+n])
			copy(meas, stream[base-samples:base-samples+n])
			a.Add(ref, meas, xfWindowKind)
		}
		r := a.Result(xfSR, 6)
		if !r.OK {
			t.Fatalf("%d samples: no result", samples)
		}
		got, ok := TransferDelayMS(r, 0.9)
		if !ok {
			t.Fatalf("%d samples: no delay estimate", samples)
		}
		if math.Abs(got-wantMS) > 0.05 {
			t.Errorf("a %d-sample delay (%.4f ms) measured as %.4f ms", samples, wantMS, got)
		}
	}
}

// COHERENCE IS THE NUMBER THAT SAYS WHETHER TO BELIEVE THE OTHERS. Two
// independent signals have no relationship, so it has to fall to near zero —
// and if it does not, a response invented out of noise will be reported with
// full confidence.
func TestIndependentSignalsHaveNoCoherence(t *testing.T) {
	left := audiosrc.NewTestSource(audiosrc.TestWide, xfSR)
	left.SetLevel(0.8)
	const n, windows = 4096, 32
	var a TransferAccum
	l := make([]float32, n)
	rr := make([]float32, n)
	for w := 0; w < windows; w++ {
		left.Fill(l, rr) // the "wide" stimulus is two independent streams
		a.Add(l, rr, xfWindowKind)
	}
	r := a.Result(xfSR, 6)
	if !r.OK {
		t.Fatal("no result")
	}
	worst := 0.0
	for i, b := range r.Bands {
		if b.Center >= 100 && b.Center <= 10000 {
			worst = math.Max(worst, r.Coherence[i])
		}
	}
	// The expected coherence of unrelated signals is about 1/averages, and the
	// scatter above that is what the averaging count buys down.
	if worst > 0.5 {
		t.Errorf("two independent noises reach a coherence of %.3f over %d averages", worst, windows)
	}
}

// THE TRAP THIS WHOLE DESIGN IS ARRANGED AROUND. From one window, coherence is
// exactly 1 — it falls straight out of the definition, |Sxy|² = |X|²|Y|² for a
// single pair of complex numbers — so a display built on one window is a row of
// perfect scores that means nothing, for ANY two signals whatever.
//
// Checked at the BIN, which is where the identity lives. Summing a band is
// itself a kind of averaging: several bins of one window behave like several
// windows of one bin, so a band-summed coherence from one window is already
// below 1 wherever the band holds more than a bin or two. That is a useful
// property and it is not the identity, so this checks the identity where it is.
func TestOneWindowHasCoherenceOneAtEveryBin(t *testing.T) {
	src := audiosrc.NewTestSource(audiosrc.TestWide, xfSR)
	src.SetLevel(0.8)
	const n = 4096
	var a TransferAccum
	l := make([]float32, n)
	rr := make([]float32, n)
	src.Fill(l, rr) // two INDEPENDENT streams: no relationship at all
	if !a.Add(l, rr, xfWindowKind) {
		t.Fatal("the window was refused")
	}
	for k := 10; k < len(a.sxx)-10; k += 37 {
		c := (a.sxyRe[k]*a.sxyRe[k] + a.sxyIm[k]*a.sxyIm[k]) / (a.sxx[k] * a.syy[k])
		if math.Abs(c-1) > 1e-9 {
			t.Fatalf("bin %d of a single window has coherence %.12f; the identity says 1", k, c)
		}
	}
}

// ...which is why a result is refused until there are enough windows for the
// number to mean something.
func TestASingleWindowIsRefused(t *testing.T) {
	src := audiosrc.NewTestSource(audiosrc.TestWide, xfSR)
	const n = 4096
	var a TransferAccum
	l := make([]float32, n)
	rr := make([]float32, n)
	for w := 0; w < transferMinAvg-1; w++ {
		src.Fill(l, rr)
		if r := a.Result(xfSR, 6); r.OK {
			t.Fatalf("a result was given after %d windows, and %d are needed", w, transferMinAvg)
		}
		a.Add(l, rr, xfWindowKind)
	}
}

// A BAND THE REFERENCE NEVER REACHED cannot be reported on, and coherence does
// not catch it: a single tone through a perfect wire reports 0 dB at coherence
// 1 in every band, correctly — the wire really is flat and the window's leakage
// really does pass through unchanged. True, and worthless, because nothing
// excited those bands. RefDB is what says so.
func TestABandWithNoInputIsMarkedByTheReferenceLevel(t *testing.T) {
	const n, windows = 4096, 16
	var a TransferAccum
	ref := distTone(n, 1000, 0.5)
	for w := 0; w < windows; w++ {
		a.Add(ref, ref, xfWindowKind)
	}
	r := a.Result(xfSR, 3)
	if !r.OK {
		t.Fatal("no result")
	}
	var atTone, away float64 = -999, 0
	for i, b := range r.Bands {
		switch {
		case b.Center > 900 && b.Center < 1100:
			atTone = math.Max(atTone, r.RefDB[i])
		case b.Center > 100 && b.Center < 500, b.Center > 4000 && b.Center < 10000:
			away = math.Min(away, r.RefDB[i])
		}
	}
	if atTone > -1 == false {
		t.Errorf("the band holding the tone reports a reference level of %.1f dB", atTone)
	}
	if away > -40 {
		t.Errorf("a band the tone never reached reports a reference level of %.1f dB; "+
			"nothing marks it as unmeasured", away)
	}
	// ...and the delay fit must not use them.
	if _, ok := TransferDelayMS(r, 0.9); ok {
		t.Error("a delay was fitted through bands the stimulus never excited")
	}
}

// Reset drops the average, because an average taken across two different
// systems describes neither.
func TestResetDropsTheAverage(t *testing.T) {
	const n = 4096
	var a TransferAccum
	ref := distTone(n, 1000, 0.5)
	for w := 0; w < transferMinAvg+4; w++ {
		a.Add(ref, ref, xfWindowKind)
	}
	if r := a.Result(xfSR, 3); !r.OK {
		t.Fatal("no result before the reset")
	}
	a.Reset()
	if r := a.Result(xfSR, 3); r.OK {
		t.Error("a result survived the reset")
	}
}

// Rubbish in has to be refused rather than panicking or inventing a spectrum.
func TestTransferRefusesBadWindows(t *testing.T) {
	var a TransferAccum
	if a.Add(make([]float32, 1000), make([]float32, 1000), xfWindowKind) {
		t.Error("a non-power-of-two window was accepted")
	}
	if a.Add(make([]float32, 4096), make([]float32, 2048), xfWindowKind) {
		t.Error("mismatched channel lengths were accepted")
	}
	if a.Add(nil, nil, xfWindowKind) {
		t.Error("an empty window was accepted")
	}
}
