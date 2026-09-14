//go:build js && wasm

package attractor

import "testing"

// τ IS A DELAY, SO IT IS A TIME. The knob counts samples at a fixed reference
// rate, and the delay in source samples is derived from the live rate — so one
// knob position is the same duration on the 48 kHz microphone and on the 24 kHz
// server feed, which is what it was not when the number was read as raw samples.
func TestTauIsTheSameDurationOnEverySource(t *testing.T) {
	for _, knob := range []float32{1, 32, takensTauDef, 200, takensTauMax} {
		want := float64(tauMS(knob))
		for _, sr := range []int{24000, 44100, 48000, 96000} {
			got := float64(tauSamples(knob, sr)) / float64(sr) * 1000
			// Rounding to a whole sample is the only error allowed, and it is
			// widest at the lowest rate.
			if tol := 1000.0 / float64(sr); got < want-tol || got > want+tol {
				t.Errorf("τ=%v at %d Hz is %.4f ms, want %.4f ms (±%.4f)", knob, sr, got, want, tol)
			}
		}
	}
}

// At the reference rate the conversion is the identity, which is why an
// ordinary microphone session sees no change at all and why every permalink
// already written still means what it meant.
func TestTauIsUnchangedAtTheReferenceRate(t *testing.T) {
	for _, knob := range []float32{1, 32, 72, 512} {
		if got := tauSamples(knob, tauRefRate); got != int(knob) {
			t.Errorf("τ=%v at the reference rate became %d samples", knob, got)
		}
	}
}

// A zero delay makes all three coordinates the same sample and collapses the
// embedding onto the diagonal; a negative one indexes backwards out of the
// ring. Neither may come out of here whatever arrives — and a modulator riding
// a feature that has gone to zero or to infinity can deliver anything.
func TestTauSamplesNeverReturnsSomethingUnusable(t *testing.T) {
	inf := float32(1)
	for i := 0; i < 40; i++ {
		inf *= 1e10 // +Inf without importing math
	}
	cases := []struct {
		tau float32
		sr  int
	}{
		{0, 48000}, {-5, 48000}, {inf, 48000}, {-inf, 48000}, {inf - inf, 48000}, // NaN
		{32, 0}, {32, -1}, {0.0001, 48000},
	}
	for _, c := range cases {
		if got := tauSamples(c.tau, c.sr); got < 1 {
			t.Errorf("tauSamples(%v, %d) = %d, want at least 1", c.tau, c.sr, got)
		}
	}
}

// A rate of zero means the source has not reported one yet, and the knob's own
// number is the best guess available — not a collapse to the floor.
func TestTauFallsBackToTheKnobWhenTheRateIsUnknown(t *testing.T) {
	if got := tauSamples(72, 0); got != 72 {
		t.Errorf("with no sample rate yet, τ=72 became %d; want the knob's own 72", got)
	}
}

// The default is what a permalink that never mentions τ restores, and what the
// mode is judged on in its first second. It must not be back in the range where
// the figure collapses toward the diagonal.
func TestTauDefaultIsLongEnoughToBeAnEmbedding(t *testing.T) {
	ms := tauMS(takensTauDef)
	if ms < 1.0 || ms > 3.0 {
		t.Errorf("default τ is %.2f ms; outside 1–3 ms it is either a streak or folded", ms)
	}
	if takensTauDef > takensTauMax || takensTauDef < 1 {
		t.Errorf("default τ %v is outside the knob's own range 1..%v", takensTauDef, takensTauMax)
	}
}

// τ is ONE knob shared by four modes, and Reset All walks every mode's
// parameter list — so a disagreement resets one variable to different numbers
// depending on map iteration order. recurrence_js_test.go pins the Takens and
// Recurrence rows together; this pins the other two to the same constants.
func TestEveryTauRowAgrees(t *testing.T) {
	find := func(mode, id string) (paramDef, bool) {
		for _, p := range attractorParams[mode] {
			if p.ID == id {
				return p, true
			}
		}
		return paramDef{}, false
	}
	for _, c := range []struct{ mode, id string }{
		{"takens", "takens-tau"}, {"recurrence", "takens-tau"},
		{"stereo", "stereo-tau"}, {"polar", "polar-tau"},
	} {
		p, ok := find(c.mode, c.id)
		if !ok {
			t.Fatalf("%s has no %s row", c.mode, c.id)
		}
		if p.Def != takensTauDef || p.Min != 1 || p.Max != takensTauMax || p.Step != 1 {
			t.Errorf("%s/%s is %v..%v/%v def %v, want 1..%v/1 def %v",
				c.mode, c.id, p.Min, p.Max, p.Step, p.Def, takensTauMax, takensTauDef)
		}
	}
}

// The recurrence ring is sized from rpMaxLookback, which has to cover the
// deepest lookback any SAMPLE RATE can produce — not the deepest the knob's
// number looks like. A source above the reference rate turns the same knob
// position into more real samples.
func TestRecurrenceRingCoversTheFastestSource(t *testing.T) {
	const fastest = 96000
	if got := tauSamples(takensTauMax, fastest); got > rpMaxTauSamples {
		t.Errorf("τ at the top of the knob is %d samples at %d Hz, past the %d the ring is sized for",
			got, fastest, rpMaxTauSamples)
	}
	if rpMaxLookback < (rpMaxDim-1)*rpMaxTauSamples {
		t.Error("rpMaxLookback no longer covers rpMaxDim delays of the longest τ")
	}
}

// The automatic measurement is a ONE-SHOT. It is reached from the frame loop,
// so the thing that keeps it from being the per-frame auto-tuning the mode
// exists not to have is the guard — and the guard is what this pins.
func TestAutoMeasureIsAOneShot(t *testing.T) {
	oldDone, oldRing, oldW := takensAutoDone, takensRing, takensW
	t.Cleanup(func() { takensAutoDone, takensRing, takensW = oldDone, oldRing, oldW })

	takensRing = make([]float32, takensEstMax)

	// Short of a full window it is not due — measuring the first samples of a
	// source that has just opened is measuring its fade-in.
	takensAutoDone = false
	takensW = takensEstMax - 1
	if takensAutoDue() {
		t.Error("due on a window shorter than takensEstMax")
	}

	// A full window is due.
	takensW = takensEstMax
	if !takensAutoDue() {
		t.Fatal("a full window is not due")
	}

	// Having run, it is never due again — which is what every later frame asks.
	takensAutoDone = true
	if takensAutoDue() {
		t.Error("still due after the one-shot has fired")
	}
	takensW = takensEstMax * 4
	if takensAutoDue() {
		t.Error("more audio re-opened the one-shot")
	}

	// Only an explicit re-arm brings it back.
	takensArmAutoMeasure()
	if !takensAutoDue() {
		t.Error("takensArmAutoMeasure did not re-arm the one-shot")
	}
}

// The automatic measurement must never take the knob away from the person
// using it. A τ set by hand — or carried by a permalink, which records a
// parameter only when it differs from the default — stands, and leaving the
// mode and coming back does not quietly replace it.
func TestAutoMeasureDefersToAChosenTau(t *testing.T) {
	oldDone, oldSet, oldTau := takensAutoDone, takensAutoSet, takensTau
	oldRing, oldW := takensRing, takensW
	t.Cleanup(func() {
		takensAutoDone, takensAutoSet, takensTau = oldDone, oldSet, oldTau
		takensRing, takensW = oldRing, oldW
	})
	takensRing = make([]float32, takensEstMax)
	takensW = takensEstMax

	// The default is nobody's choice, so it is due.
	takensAutoDone, takensAutoSet, takensTau = false, 0, takensTauDef
	if !takensAutoDue() {
		t.Error("not due at the default τ")
	}

	// A hand-set τ is a choice, and re-arming must not undo it.
	takensTau = 300
	takensArmAutoMeasure()
	if takensAutoDue() {
		t.Error("due over a τ that was set by hand")
	}

	// The value a previous automatic measurement wrote is not a choice, so a
	// new source measures again rather than keeping an answer about the old one.
	takensAutoSet = 43
	takensTau = 43
	takensArmAutoMeasure()
	if !takensAutoDue() {
		t.Error("not due over the value the last automatic measurement wrote")
	}
}
