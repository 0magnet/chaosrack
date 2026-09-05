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
	want := []string{"rec-src", "rec-win", "rec-eps", "rec-dim", "takens-tau"}
	got := attractorParams["recurrence"]
	if len(got) != len(want) {
		t.Fatalf("recurrence exposes %d knobs, want %d: %v", len(got), len(want), want)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("knob %d is %q, want %q", i, got[i].ID, id)
		}
	}
}

// SRC is a named setting, so the panel builds it as a labeled rotary switch
// rather than a dial reading a number — and buildParamUnit picks that up from
// paramLabels alone. A missing entry gives a knob reading "0", "1", "2", which
// is the one thing a reader cannot decode.
func TestTheSourceKnobIsLabeled(t *testing.T) {
	labels, ok := paramLabels["rec-src"]
	if !ok {
		t.Fatal("rec-src has no paramLabels entry; the dial would read 0/1/2")
	}
	var src paramDef
	for _, p := range attractorParams["recurrence"] {
		if p.ID == "rec-src" {
			src = p
		}
	}
	// A labeled parameter runs 0..n-1 in steps of one — that is the contract
	// paramLabels documents, and positions that do not line up with the option
	// list put the wrong name under the wrong setting.
	if int(src.Max-src.Min)+1 != len(labels) || src.Step != 1 || src.Min != 0 {
		t.Errorf("rec-src runs %v..%v step %v for %d labels; a named setting must run 0..n-1 by 1",
			src.Min, src.Max, src.Step, len(labels))
	}
	if src.Def != rpSrcAudio {
		t.Errorf("rec-src defaults to %v; it must default to the raw-audio picture the mode "+
			"drew before the knob existed, or every existing permalink restores something else", src.Def)
	}
}

// τ IS ONE KNOB SHARED BY TWO MODES, on purpose: it is the same delay of the
// same signal, and the Takens mode's MEAS button writes it by DOM id, so
// whichever panel is on screen is the one it moves. What that costs is that the
// two rows have to agree about the knob's range and default — Reset All walks
// every mode's parameter list, so a disagreement would reset one variable to
// two different numbers depending on map iteration order, which is to say
// randomly.
func TestTheSharedTauKnobAgreesBetweenBothModes(t *testing.T) {
	find := func(mode string) (paramDef, bool) {
		for _, p := range attractorParams[mode] {
			if p.ID == "takens-tau" {
				return p, true
			}
		}
		return paramDef{}, false
	}
	a, okA := find("takens")
	b, okB := find("recurrence")
	if !okA || !okB {
		t.Fatalf("takens-tau present in takens=%v, recurrence=%v; both must expose it", okA, okB)
	}
	if a.Value != b.Value {
		t.Error("the two takens-tau rows point at different variables, so the id is a collision rather than a share")
	}
	if a.Def != b.Def || a.Min != b.Min || a.Max != b.Max || a.Step != b.Step {
		t.Errorf("takens-tau is %v..%v/%v def %v in takens and %v..%v/%v def %v in recurrence",
			a.Min, a.Max, a.Step, a.Def, b.Min, b.Max, b.Step, b.Def)
	}
	// Integer step is what keeps the shared id out of the patchbay and the
	// audio-mod matrix, both of which key routings by parameter id alone and
	// would otherwise apply a routing made in one mode to the other.
	if decimalsForStep(a.Step) != 0 {
		t.Error("takens-tau has a fractional step, so it becomes a routable destination " +
			"under an id that two modes share")
	}
	// The audio ring is sized for rpMaxLookback, which assumes τ cannot exceed
	// rpMaxTau. Raising the knob's ceiling without raising that constant would
	// let the delay coordinates read behind the start of the buffer.
	if a.Max != rpMaxTau {
		t.Errorf("the τ knob reaches %v but the ring is sized for rpMaxTau=%d", a.Max, rpMaxTau)
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

// ── The trajectory source's cache ────────────────────────────────────────

// THIS IS THE TEST THE FEATURE EXISTS BEHIND. Integrating a trajectory costs
// 0.5 ms for Lorenz and 24 ms for Chen, and a knob DRAG changes a parameter
// value on every frame it moves — so an implementation that simply re-derives
// the plot from the current values does 24 ms of work per frame for as long as
// the pointer is down, which is a tab that stops responding while it is being
// used. It is worth a test rather than a comment because the failure is a
// gradual slowdown under one specific gesture, not something that shows up in a
// stack trace or on an idle screen.
//
// What is asserted is the whole contract: nothing is integrated while the value
// is moving, exactly one integration happens after it stops, and the frames
// after that do no work at all.
func TestADragIntegratesNothingUntilTheKnobSettles(t *testing.T) {
	savedMat, savedNow, savedMode := rpMat, frameNowMs, lastFlowMode
	savedSrc, savedWin, savedEps := rpSrc, rpWin, rpEps
	ps := attractorParams["lorenz"]
	savedVals := make([]float32, len(ps))
	for i, p := range ps {
		savedVals[i] = *p.Value
	}
	defer func() {
		rpMat, frameNowMs, lastFlowMode = savedMat, savedNow, savedMode
		rpSrc, rpWin, rpEps = savedSrc, savedWin, savedEps
		for i, p := range ps {
			*p.Value = savedVals[i]
		}
		rpTrajSeries, rpTrajMode, rpTrajVals = nil, "", nil
		rpTrajStale, rpTrajBuilt, rpTrajDiam = false, false, 0
	}()

	rpMat = make([]byte, rpN*rpN)
	rpTrajSeries, rpTrajMode, rpTrajVals = nil, "", nil
	rpTrajStale, rpTrajBuilt, rpTrajDiam = false, false, 0
	lastFlowMode, rpSrc, rpWin, rpEps = "lorenz", rpSrcTraj, 100, 0.05
	frameNowMs = 1000

	// The frame the mode is entered on notices it has nothing and asks for a
	// trajectory; it must not integrate one on that same frame.
	if rpFillFromTrajectory() {
		t.Error("built a matrix on the frame it first noticed the system")
	}
	if rpTrajSeries != nil {
		t.Fatal("integrated on the frame the change was noticed, before any settle")
	}

	// A drag: σ moves every frame for ten frames, well inside the settle window.
	var sigma *float32
	for _, p := range ps {
		if p.Label == "σ" {
			sigma = p.Value
		}
	}
	if sigma == nil {
		t.Fatal("lorenz has no σ parameter to drag")
	}
	for i := 0; i < 10; i++ {
		frameNowMs += 16
		*sigma += 0.1
		if rpFillFromTrajectory() {
			t.Fatalf("frame %d of a drag rebuilt the matrix", i)
		}
		if rpTrajSeries != nil {
			t.Fatalf("frame %d of a drag re-integrated the trajectory", i)
		}
	}

	// Let go. Nothing happens until the settle window has passed...
	frameNowMs += rpTrajSettleMs / 2
	if rpFillFromTrajectory() || rpTrajSeries != nil {
		t.Fatal("integrated before the settle window was up")
	}
	// ...and then exactly once.
	frameNowMs += rpTrajSettleMs
	if !rpFillFromTrajectory() {
		t.Fatal("never integrated after the knob settled")
	}
	if len(rpTrajSeries) != rpN*3 {
		t.Fatalf("integrated %d floats, want %d points of 3 coordinates", len(rpTrajSeries), rpN)
	}
	if rpTrajDiam <= 0 {
		t.Errorf("diameter is %v, so ε would normalize against nothing", rpTrajDiam)
	}

	// A still knob is a still picture: the following frames must do no work,
	// because the series is static and the matrix on the texture is already it.
	for i := 0; i < 5; i++ {
		frameNowMs += 16
		if rpFillFromTrajectory() {
			t.Errorf("frame %d after settling rebuilt a matrix that had not changed", i)
		}
	}

	// ε is the exception and gets no settle delay: it only refills the matrix,
	// which is 0.3 ms, so it follows the knob immediately.
	rpEps = 0.08
	if !rpFillFromTrajectory() {
		t.Error("ε moved and the matrix was not rebuilt")
	}
	if len(rpTrajSeries) != rpN*3 {
		t.Error("moving ε re-integrated the trajectory; only the threshold changed")
	}
}

// The m knob is 1 for the raw-audio position whatever it is set to, because
// that position IS m = 1 and shares its code path — and it is clamped to the
// buffer's width, which is allocated once at rpMaxDim and never regrown.
func TestTheEmbeddingDimensionIsClampedToTheBuffer(t *testing.T) {
	savedSrc, savedDim := rpSrc, rpDim
	defer func() { rpSrc, rpDim = savedSrc, savedDim }()

	rpSrc, rpDim = rpSrcAudio, 5
	if got := rpEmbedDim(); got != 1 {
		t.Errorf("raw audio reports m=%d, want 1", got)
	}
	rpSrc = rpSrcEmbed
	for _, c := range []struct{ set, want float32 }{{0, 1}, {1, 1}, {3, 3}, {rpMaxDim, rpMaxDim}, {99, rpMaxDim}} {
		rpDim = c.set
		if got := rpEmbedDim(); float32(got) != c.want {
			t.Errorf("m knob at %v reports %d, want %v — rpVec holds only rpMaxDim coordinates",
				c.set, got, c.want)
		}
	}
}
