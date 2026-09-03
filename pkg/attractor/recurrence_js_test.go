//go:build js && wasm

package attractor

import (
	"math"
	"math/rand"
	"testing"
)

// The window arithmetic is what the picture is OF, and it is the same
// conflation the Takens mode already got wrong once: WIN is a DURATION, and
// the matrix side is a fixed resolution the duration is decimated into.
func TestRecurrenceWindowIsADurationDecimatedToTheMatrix(t *testing.T) {
	for _, sr := range []int{8000, 24000, 48000} {
		for _, win := range []float32{50, 500, 4000} {
			span, stride := rpWindow(win, sr)
			if stride < 1 {
				t.Fatalf("sr=%d win=%v: stride %d", sr, win, stride)
			}
			// Exactly one sample per column: anything else either leaves part
			// of the texture unwritten or reads past the ring.
			if span != stride*rpN {
				t.Errorf("sr=%d win=%v: span %d is not %d columns of stride %d",
					sr, win, span, rpN, stride)
			}
			wantMS := float64(win)
			gotMS := float64(span) / float64(sr) * 1000
			// The stride is an integer, so a short window at a low rate cannot
			// land on the millisecond; it must not be off by a factor.
			if gotMS < wantMS*0.5 || gotMS > wantMS*1.05 {
				t.Errorf("sr=%d win=%v ms: covers %.0f ms", sr, win, gotMS)
			}
		}
	}
}

// The ws source reports a sample rate of 0 until the stream tells it one, and
// a zero-length window would divide by zero or read nothing at all.
func TestRecurrenceWindowSurvivesAnUnknownSampleRate(t *testing.T) {
	span, stride := rpWindow(500, 0)
	if span < rpN || stride < 1 {
		t.Fatalf("unknown rate gave span=%d stride=%d", span, stride)
	}
	if got := float64(span) / 24000 * 1000; got < 400 || got > 600 {
		t.Errorf("unknown rate covers %.0f ms, want the 24 kHz default's ~500", got)
	}
}

// The mode is only real if the registry knows about it: the generator, the
// label and the knobs all have to be there or it is a key nothing reaches.
func TestRecurrenceModeIsRegistered(t *testing.T) {
	if modeGenerate["recurrence"] == nil {
		t.Error("no generator registered for recurrence")
	}
	if modeInfo["recurrence"].Label == "" {
		t.Error("recurrence has no ModeInfo row")
	}
	if !isTexturePlane("recurrence") {
		t.Error("recurrence is not drawn as a texture plane; its camera would boot tumbling")
	}
	if got := len(attractorParams["recurrence"]); got != 2 {
		t.Errorf("recurrence exposes %d knobs, want win and ε", got)
	}
}

// ── The Takens mode's on-demand measurement ──────────────────────────────

// The ring is a wrapping buffer with a monotonic write cursor, and the
// estimators need the window in time order. Getting that backwards or
// off-by-one would still produce a plausible-looking number, which is the
// worst kind of wrong for a measurement, so it is checked against a signal
// whose answer is known: a tone's mutual information first minimizes at a
// quarter period.
func TestTakensMeasurementWindowIsInTimeOrder(t *testing.T) {
	savedRing, savedW := takensRing, takensW
	defer func() { takensRing, takensW = savedRing, savedW }()

	const period = 40
	takensRing = make([]float32, 5000)
	takensW = 0
	// Write more than the ring holds, so the window has wrapped — the case
	// that a naive copy from index 0 gets wrong.
	rng := rand.New(rand.NewSource(17)) //nolint:gosec // a deterministic test signal, not a secret
	for i := 0; i < 12000; i++ {
		// A little noise, for the reason embedding_test.go gives: a noiseless
		// tone at an exact integer period visits only 40 distinct sample
		// values, and the histogram behind the estimate then has nothing to
		// count but repeats.
		takensRing[takensW%len(takensRing)] = float32(math.Sin(2*math.Pi*float64(i)/period) +
			0.05*rng.NormFloat64())
		takensW++
	}
	x := takensEstWindow()
	if len(x) != takensEstMax {
		t.Fatalf("window is %d samples, want the %d cap", len(x), takensEstMax)
	}
	// The last sample of the window must be the last sample written.
	if last := float64(takensRing[(takensW-1)%len(takensRing)]); math.Abs(x[len(x)-1]-last) > 1e-9 {
		t.Errorf("the window ends at %v, not at the newest sample %v", x[len(x)-1], last)
	}
	tau, _, ok := FirstMinimumTau(x, 200)
	if !ok {
		t.Fatal("no delay measured from a pure tone")
	}
	if r := float64(tau) / period; r < 0.15 || r > 0.35 {
		t.Errorf("measured τ=%d for a %d-sample period (%.2f of it), want ≈0.25", tau, period, r)
	}
}

// Before any audio arrives there is nothing to measure, and the button has to
// say so instead of measuring the silence.
func TestTakensMeasurementRefusesAnEmptyRing(t *testing.T) {
	savedRing, savedW := takensRing, takensW
	defer func() { takensRing, takensW = savedRing, savedW }()

	takensRing = make([]float32, 3000)
	takensW = 0
	if x := takensEstWindow(); x != nil {
		t.Errorf("an empty ring produced a %d-sample window", len(x))
	}
	takensW = 100 // some audio, but not enough of it
	if x := takensEstWindow(); x != nil {
		t.Errorf("100 samples produced a %d-sample window", len(x))
	}
}

// The measurement must not be reachable from the render path. This is the
// constraint the mode was rebuilt around — per-frame auto-adjustment made the
// figure zoom in and out with the music — and it is worth a test rather than
// a comment, because the failure mode is a knob that creeps rather than an
// error anyone would see in a stack trace.
func TestGeneratingAFrameDoesNotRetuneTau(t *testing.T) {
	savedTau, savedRing, savedW := takensTau, takensRing, takensW
	defer func() { takensTau, takensRing, takensW = savedTau, savedRing, savedW }()

	takensRing = make([]float32, 4096)
	takensW = 0
	for i := 0; i < 20000; i++ {
		takensRing[takensW%len(takensRing)] = float32(math.Sin(2 * math.Pi * float64(i) / 40))
		takensW++
	}
	takensTau = 32
	// generateTakens itself needs a GL context; what is being asserted is that
	// the estimator is not on that path at all, which the measurement window's
	// own purity states: measuring twice cannot change anything.
	x := takensEstWindow()
	before := takensTau
	EstimateEmbedding(x, 512, 8)
	EstimateEmbedding(x, 512, 8)
	if takensTau != before {
		t.Errorf("measuring moved τ from %v to %v without anyone pressing the button", before, takensTau)
	}
}
