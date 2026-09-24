//go:build js && wasm

package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/takens"
)

// τ IS A DELAY, SO IT IS A TIME. The knob counts samples at a fixed reference
// rate, and the delay in source samples is derived from the live rate — so one
// knob position is the same duration on the 48 kHz microphone and on the 24 kHz
// server feed, which is what it was not when the number was read as raw samples.
func TestTauIsTheSameDurationOnEverySource(t *testing.T) {
	for _, knob := range []float32{1, 32, takens.TauDef, 200, takens.TauMax} {
		want := float64(tauMS(knob))
		for _, sr := range []int{24000, 44100, 48000, 96000} {
			got := float64(takens.TauSamples(knob, sr)) / float64(sr) * 1000
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
		if got := takens.TauSamples(knob, takens.RefRate); got != int(knob) {
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
	for range 40 {
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
		if got := takens.TauSamples(c.tau, c.sr); got < 1 {
			t.Errorf("takens.TauSamples(%v, %d) = %d, want at least 1", c.tau, c.sr, got)
		}
	}
}

// A rate of zero means the source has not reported one yet, and the knob's own
// number is the best guess available — not a collapse to the floor.
func TestTauFallsBackToTheKnobWhenTheRateIsUnknown(t *testing.T) {
	if got := takens.TauSamples(72, 0); got != 72 {
		t.Errorf("with no sample rate yet, τ=72 became %d; want the knob's own 72", got)
	}
}

// The default is what a permalink that never mentions τ restores, and what the
// mode is judged on in its first second. It must not be back in the range where
// the figure collapses toward the diagonal.
func TestTauDefaultIsLongEnoughToBeAnEmbedding(t *testing.T) {
	ms := tauMS(takens.TauDef)
	if ms < 1.0 || ms > 3.0 {
		t.Errorf("default τ is %.2f ms; outside 1–3 ms it is either a streak or folded", ms)
	}
	if takens.TauDef > takens.TauMax || takens.TauDef < 1 {
		t.Errorf("default τ %v is outside the knob's own range 1..%v", takens.TauDef, takens.TauMax)
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
		{"stereo", "stereo-tau"}, {"polar", "takens-tau"},
	} {
		p, ok := find(c.mode, c.id)
		if !ok {
			t.Fatalf("%s has no %s row", c.mode, c.id)
		}
		if p.Def != takens.TauDef || p.Min != 1 || p.Max != takens.TauMax || p.Step != 1 {
			t.Errorf("%s/%s is %v..%v/%v def %v, want 1..%v/1 def %v",
				c.mode, c.id, p.Min, p.Max, p.Step, p.Def, takens.TauMax, takens.TauDef)
		}
	}
}

// The recurrence ring is sized from rpMaxLookback, which has to cover the
// deepest lookback any SAMPLE RATE can produce — not the deepest the knob's
// number looks like. A source above the reference rate turns the same knob
// position into more real samples.
func TestRecurrenceRingCoversTheFastestSource(t *testing.T) {
	const fastest = 96000
	if got := takens.TauSamples(takens.TauMax, fastest); got > rpMaxTauSamples {
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
	oldDone, oldRing, oldW := emb.autoDone, emb.ring, emb.w
	t.Cleanup(func() { emb.autoDone, emb.ring, emb.w = oldDone, oldRing, oldW })

	emb.ring = make([]float32, takensEstMax)

	// Short of a full window it is not due — measuring the first samples of a
	// source that has just opened is measuring its fade-in.
	emb.autoDone = false
	emb.w = takensEstMax - 1
	if emb.autoDue() {
		t.Error("due on a window shorter than takensEstMax")
	}

	// A full window is due.
	emb.w = takensEstMax
	if !emb.autoDue() {
		t.Fatal("a full window is not due")
	}

	// Having run, it is never due again — which is what every later frame asks.
	emb.autoDone = true
	if emb.autoDue() {
		t.Error("still due after the one-shot has fired")
	}
	emb.w = takensEstMax * 4
	if emb.autoDue() {
		t.Error("more audio re-opened the one-shot")
	}

	// Only an explicit re-arm brings it back.
	emb.armAutoMeasure()
	if !emb.autoDue() {
		t.Error("takensArmAutoMeasure did not re-arm the one-shot")
	}
}

// The automatic measurement must never take the knob away from the person
// using it. A τ set by hand — or carried by a permalink, which records a
// parameter only when it differs from the default — stands, and leaving the
// mode and coming back does not quietly replace it.
func TestAutoMeasureDefersToAChosenTau(t *testing.T) {
	oldDone, oldSet, oldTau := emb.autoDone, emb.autoSet, emb.tau
	oldRing, oldW := emb.ring, emb.w
	t.Cleanup(func() {
		emb.autoDone, emb.autoSet, emb.tau = oldDone, oldSet, oldTau
		emb.ring, emb.w = oldRing, oldW
	})
	emb.ring = make([]float32, takensEstMax)
	emb.w = takensEstMax

	// The default is nobody's choice, so it is due.
	emb.autoDone, emb.autoSet, emb.tau = false, 0, takens.TauDef
	if !emb.autoDue() {
		t.Error("not due at the default τ")
	}

	// A hand-set τ is a choice, and re-arming must not undo it.
	emb.tau = 300
	emb.armAutoMeasure()
	if emb.autoDue() {
		t.Error("due over a τ that was set by hand")
	}

	// The value a previous automatic measurement wrote is not a choice, so a
	// new source measures again rather than keeping an answer about the old one.
	emb.autoSet = 43
	emb.tau = 43
	emb.armAutoMeasure()
	if !emb.autoDue() {
		t.Error("not due over the value the last automatic measurement wrote")
	}
}

// TestTakensSmoothStaysInsideTheVertexBudget is the safety property of putting
// the beam smoothing on a knob.
//
// The window arithmetic and the vertex count are two halves of one invariant:
// takensWindow spends budget/smooth on source points and takensVerts draws
// (n-1)*smooth+1 of them. If the two ever read different values of smooth — or
// if a modulator drives it to zero, since it is a DIVISOR — the figure either
// overruns the buffer or divides by zero. Neither is a wrong picture; both are
// a crash.
func TestTakensSmoothStaysInsideTheVertexBudget(t *testing.T) {
	saved := takensSmoothF
	t.Cleanup(func() { takensSmoothF = saved })

	for _, knob := range []float32{-1e9, -1, 0, 0.4, 1, 4, 16, 17, 1e9} {
		takensSmoothF = knob
		sm := takensSmooth()
		if sm < 1 || sm > 16 {
			t.Errorf("smth %v clamped to %d, want 1..16", knob, sm)
			continue
		}
		for _, budget := range []int{64, 2048, 20000} {
			for _, winMS := range []float32{5, 85, 500} {
				n, stride := takensWindow(winMS, 24000, budget)
				if n < 2 || stride < 1 {
					t.Fatalf("smth %v budget %d win %v: n=%d stride=%d", knob, budget, winMS, n, stride)
				}
				if v := takensVerts(n); v > budget {
					t.Errorf("smth %v budget %d win %v: %d vertices overruns the budget",
						knob, budget, winMS, v)
				}
			}
		}
	}
}

// A shared knob has to be shared STORAGE, not merely a matching row.
//
// TestEveryTauRowAgrees above checks that the rows offer the same range and
// default, which is what "shared" looked like before Polar actually shared:
// it had a polar-tau of its own with identical numbers, so that test passed
// while turning one knob left the other mode's delay where it was. The rack
// builds one element per id, so two modes declaring the same id and DIFFERENT
// pointers is worse than two knobs — it is one knob that writes to whichever
// variable the last-built row happened to bind.
//
// Stereo is deliberately not in this list. It keeps a tau per view instance
// because the view grid exists so two cells can be set differently, and a
// shared one would move all sixteen at once.
func TestTheSharedTauIsOneVariable(t *testing.T) {
	ptrOf := func(mode, id string) *float32 {
		t.Helper()
		for _, p := range attractorParams[mode] {
			if p.ID == id {
				return p.Value
			}
		}
		t.Fatalf("%s has no %s row", mode, id)
		return nil
	}
	want := ptrOf("takens", "takens-tau")
	if want != &emb.tau {
		t.Fatalf("takens-tau does not point at takensTau")
	}
	for _, mode := range []string{"polar", "recurrence"} {
		if got := ptrOf(mode, "takens-tau"); got != want {
			t.Errorf("%s/takens-tau points at %p, takens points at %p — same knob, two variables", mode, got, want)
		}
	}
	// And the retired row is really gone: an id nothing declares is an id the
	// permalink and the MIDI map can no longer address.
	for _, p := range attractorParams["polar"] {
		if p.ID == "polar-tau" {
			t.Error("polar-tau still declared; it was replaced by the shared takens-tau")
		}
	}
}
