package attractor

import (
	"math"
	"math/rand"
	"testing"
)

// The estimators are tested against signals whose answer is known from theory
// rather than from a previous run of this code: a sine's mutual information
// first minimizes at a quarter of its period, and the Lorenz attractor's
// published embedding dimension is 3.
//
// The tones carry a little noise, deliberately. A mathematically pure sine
// sampled at an integer period visits only T distinct sample values however
// long it runs, so the histogram behind the mutual-information estimate is
// counting the same handful of points over and over and the estimate becomes a
// jagged function of τ — measured here, the "first minimum" of a noiseless
// integer-period tone lands at T/20. Nothing off a microphone, a line input or
// even a signal generator is that clean, and 5% noise is still an obviously
// pure tone.

func sineWithNoise(n int, period, noise float64, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // a deterministic test signal, not a secret
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Sin(2*math.Pi*float64(i)/period) + noise*rng.NormFloat64()
	}
	return x
}

// lorenzSeries is one observable of a Lorenz trajectory — the x coordinate,
// which is the case the delay-embedding literature works with. Sampled at
// 0.05 time units, close to the rate the app's own render loop draws it at.
func lorenzSeries(t *testing.T) []float64 {
	t.Helper()
	p := Trajectory("lorenz", TrajectoryOptions{Transient: 40, Duration: 200, MaxPoints: 4000})
	if len(p) < 2000 {
		t.Fatalf("the Lorenz trajectory came back %d points long; the test needs a few thousand", len(p))
	}
	x := make([]float64, len(p))
	for i, v := range p {
		x[i] = v[0]
	}
	return x
}

// A signal tells the most about itself at zero delay, and less as the delay
// grows. Anything else and the quantity is not mutual information.
func TestMutualInformationIsMaximalAtZeroDelay(t *testing.T) {
	x := sineWithNoise(4000, 100, 0.05, 1)
	at0 := MutualInformation(x, 0, amiBins)
	for _, tau := range []int{1, 5, 25, 100} {
		if got := MutualInformation(x, tau, amiBins); got >= at0 {
			t.Errorf("MI at τ=%d is %.3f, not below the %.3f at τ=0", tau, got, at0)
		}
	}
	// Two independent signals share nothing: the estimate should sit at its
	// small-sample floor rather than at anything a reader would call a bit.
	noise := sineWithNoise(4000, 1e9, 1, 2) // period far beyond the series: pure noise
	if got := MutualInformation(noise, 37, amiBins); got > 5*miNoiseFloor(len(noise)) {
		t.Errorf("white noise reports %.3f bits of self-information at τ=37; floor is %.3f",
			got, miNoiseFloor(len(noise)))
	}
	// A constant has no information to share with anything, including itself.
	flat := make([]float64, 1000)
	if got := MutualInformation(flat, 3, amiBins); got != 0 {
		t.Errorf("a constant signal reports %v bits", got)
	}
}

// The textbook case: for a sinusoid the first minimum of average mutual
// information is at a quarter period, where the delay coordinates trace the
// fattest ellipse (a circle) and are least able to predict each other. Held to
// ±10% of the period, which is far tighter than the difference the estimate is
// there to prevent — the guess it replaces is off by whole periods on anything
// that is not a single tone.
func TestFirstMinimumOfATonesMutualInformationIsAQuarterPeriod(t *testing.T) {
	for _, period := range []float64{40, 97.3, 100, 256} {
		x := sineWithNoise(6000, period, 0.05, 11)
		tau, mi, ok := FirstMinimumTau(x, int(period))
		if !ok {
			t.Errorf("period %.1f: no first minimum found", period)
			continue
		}
		ratio := float64(tau) / period
		if ratio < 0.15 || ratio > 0.35 {
			t.Errorf("period %.1f: first minimum at τ=%d (%.3f of a period), want ≈0.25",
				period, tau, ratio)
		}
		if mi <= 0 {
			t.Errorf("period %.1f: minimum MI reported as %v", period, mi)
		}
	}
}

// τ is a delay in SAMPLES, so the same tone at half the sample rate must come
// back with half the delay. This is the property that makes the measurement
// usable as a knob value at whatever rate the audio source happens to run.
func TestDelayScalesWithTheSampleRate(t *testing.T) {
	slow, _, ok1 := FirstMinimumTau(sineWithNoise(6000, 64, 0.05, 5), 64)
	fast, _, ok2 := FirstMinimumTau(sineWithNoise(6000, 128, 0.05, 5), 128)
	if !ok1 || !ok2 {
		t.Fatalf("no first minimum: %v %v", ok1, ok2)
	}
	if r := float64(fast) / float64(slow); r < 1.6 || r > 2.4 {
		t.Errorf("doubling the period took τ from %d to %d (×%.2f), want ×2", slow, fast, r)
	}
}

// White noise has no embedding to find, and the estimator has to say so rather
// than return a number. Its mutual information is at the estimator's floor for
// every delay, so there is no first minimum to report — and an arbitrary τ
// written into the knob would look exactly like a measurement.
func TestWhiteNoiseHasNoMeasurableDelay(t *testing.T) {
	rng := rand.New(rand.NewSource(3)) //nolint:gosec // a deterministic test signal, not a secret
	x := make([]float64, 4000)
	for i := range x {
		x[i] = rng.NormFloat64()
	}
	r := EstimateEmbedding(x, 200, 8)
	if r.Tau != 0 || r.OK {
		t.Errorf("white noise measured as %+v; want no answer at all", r)
	}
}

// The Lorenz attractor's published embedding dimension is 3 — it is a 3-D flow
// and one of its coordinates is enough to reconstruct it. This is the whole
// claim of Takens' theorem and the reason the mode exists.
func TestLorenzEmbedsInThreeDimensions(t *testing.T) {
	x := lorenzSeries(t)
	r := EstimateEmbedding(x, len(x)/8, 8)
	if !r.OK {
		t.Fatalf("no embedding measured: %+v", r)
	}
	if r.Dim != 3 {
		t.Errorf("Lorenz measured %d dimensions (%.1f%% false neighbors), want the published 3",
			r.Dim, 100*r.FNN)
	}
	// The dimensions below it must be visibly insufficient, or "3" is just the
	// first number the loop happened to accept.
	if f := FalseNearestFraction(x, r.Tau, 1, 0); f < 0.5 {
		t.Errorf("a single coordinate leaves only %.1f%% false neighbors; the test is not measuring folding", 100*f)
	}
	if f2, f3 := FalseNearestFraction(x, r.Tau, 2, 0), FalseNearestFraction(x, r.Tau, 3, 0); f2 <= f3 {
		t.Errorf("false neighbors did not fall from 2-D (%.3f) to 3-D (%.3f)", f2, f3)
	}
	// And the delay itself: Fraser & Swinney's first minimum for Lorenz is a
	// fifth of a time unit or so, not the several-time-unit delays that fold
	// the reconstruction back over itself.
	if tu := float64(r.Tau) * 200 / float64(len(x)); tu < 0.05 || tu > 0.5 {
		t.Errorf("τ = %.2f time units, outside the published 0.1–0.2 neighborhood", tu)
	}
}

// Every 3-D flow in the catalog reconstructs from one coordinate in three
// dimensions — the same statement as the Lorenz case, made against systems the
// estimator was not tuned on.
func TestThreeDimensionalFlowsEmbedInThree(t *testing.T) {
	for _, mode := range []string{"rossler", "chua", "aizawa", "halvorsen"} {
		p := Trajectory(mode, TrajectoryOptions{Transient: 100, Duration: 400, MaxPoints: 6000})
		if len(p) < 2000 {
			t.Errorf("%s: trajectory only %d points", mode, len(p))
			continue
		}
		x := make([]float64, len(p))
		for i, v := range p {
			x[i] = v[0]
		}
		r := EstimateEmbedding(x, len(x)/8, 8)
		if !r.OK || r.Dim != 3 {
			t.Errorf("%s: measured %+v, want a 3-dimensional embedding", mode, r)
		}
	}
}

// A measurement is a function of the signal and nothing else. This is what
// makes it safe to hang off a button: the answer cannot depend on what was
// measured before it, on how many frames have passed, or on any state the
// package is carrying — which is exactly what a per-frame adaptation would.
func TestEstimateIsAFunctionOfTheSignalAlone(t *testing.T) {
	a := sineWithNoise(4000, 80, 0.05, 7)
	b := lorenzSeries(t)
	first := EstimateEmbedding(a, 200, 8)
	EstimateEmbedding(b, 200, 8) // a different signal in between
	if second := EstimateEmbedding(a, 200, 8); second != first {
		t.Errorf("the same signal measured %+v then %+v", first, second)
	}
}

// Too little audio is not a small answer, it is no answer. The Takens ring is
// empty for the first frames after the mode starts, and a τ measured off 40
// samples would be the bin count talking.
func TestShortSignalsAreRefused(t *testing.T) {
	for _, n := range []int{0, 1, 40, embedMinSamples - 1} {
		x := sineWithNoise(n, 20, 0.05, 1)
		if _, _, ok := FirstMinimumTau(x, 50); ok {
			t.Errorf("%d samples produced a delay estimate", n)
		}
		if r := EstimateEmbedding(x, 50, 5); r.OK || r.Tau != 0 {
			t.Errorf("%d samples produced %+v", n, r)
		}
	}
}

// ── Recurrence plot ──────────────────────────────────────────────────────

// The two properties that make the plot readable at all: it is symmetric, and
// its main diagonal is lit (every point recurs with itself).
func TestRecurrenceMatrixIsSymmetricAboutALitDiagonal(t *testing.T) {
	x := sineWithNoise(64, 17, 0.05, 4)
	m := make([]byte, len(x)*len(x))
	RecurrenceMatrix(x, 0.1, m)
	n := len(x)
	for i := 0; i < n; i++ {
		if m[i*n+i] != 255 {
			t.Fatalf("the diagonal is dark at %d", i)
		}
		for j := 0; j < n; j++ {
			if m[i*n+j] != m[j*n+i] {
				t.Fatalf("asymmetric at (%d,%d)", i, j)
			}
		}
	}
}

// A periodic signal's recurrence plot is periodic in the same way — shifting
// both indices by one period lands on the same answer. That invariance is what
// draws the diagonal lines the plot is read for, and it puts a number on the
// claim "a steady tone draws diagonals spaced by its period".
func TestAPeriodicSignalGivesAPeriodicPlot(t *testing.T) {
	const period, n = 16, 128
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Sin(2 * math.Pi * float64(i) / period)
	}
	m := make([]byte, n*n)
	RecurrenceMatrix(x, 0.05, m)
	for i := 0; i+period < n; i++ {
		for j := 0; j+period < n; j++ {
			if m[i*n+j] != m[(i+period)*n+j+period] {
				t.Fatalf("(%d,%d) and (%d,%d) disagree: the plot is not period-%d",
					i, j, i+period, j+period, period)
			}
		}
	}
	// The line one period off the diagonal is lit along its whole length —
	// that is the diagonal a reader measures the period off.
	for i := 0; i+period < n; i++ {
		if m[i*n+i+period] != 255 {
			t.Errorf("the period-%d diagonal is broken at %d", period, i)
		}
	}
}

// ε is the knob, and it has to behave like one: more of it lights more of the
// plot, monotonically, from the bare diagonal up to a solid square.
func TestRecurrenceRateGrowsWithEpsilon(t *testing.T) {
	x := sineWithNoise(200, 23, 0.05, 9)
	prev := -1.0
	for _, eps := range []float64{0, 0.01, 0.05, 0.2, 0.5, 4} {
		r := RecurrenceRate(x, eps)
		if r < prev {
			t.Errorf("ε=%v lit %.3f of the plot, less than the %.3f before it", eps, r, prev)
		}
		prev = r
	}
	if r, want := RecurrenceRate(x, 0), 1/float64(len(x)); math.Abs(r-want) > 1e-12 {
		t.Errorf("ε=0 lit %.4f of the plot, want just the diagonal (%.4f)", r, want)
	}
	if r := RecurrenceRate(x, 4); r != 1 {
		t.Errorf("an ε wider than the signal lit %.4f, want the whole square", r)
	}
}

// The matrix and the rate are two readings of one thing, so they must agree —
// the rate is what the ε knob is turned by and the matrix is what is drawn.
func TestRecurrenceRateMatchesTheMatrix(t *testing.T) {
	x := sineWithNoise(96, 11, 0.1, 6)
	n := len(x)
	m := make([]byte, n*n)
	const eps = 0.15
	RecurrenceMatrix(x, eps, m)
	lit := 0
	for _, v := range m {
		if v != 0 {
			lit++
		}
	}
	// The matrix lights its own diagonal unconditionally; |x_i − x_i| = 0 is
	// not < 0 only when ε is 0, and ε is positive here.
	if got, want := float64(lit)/float64(n*n), RecurrenceRate(x, eps); math.Abs(got-want) > 1e-9 {
		t.Errorf("matrix lit %.4f, rate says %.4f", got, want)
	}
}
