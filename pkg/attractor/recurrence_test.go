package attractor

import (
	"math"
	"math/rand"
	"testing"
)

// The recurrence math, tested without a GL context — the reason it lives in an
// untagged file at all. What is checked here is not "the code runs" but the
// handful of properties that make the picture READABLE, each of which has a
// known answer from theory rather than from a previous run of this code: a
// periodic signal's plot is periodic, ε behaves like a threshold, the vector
// form measures a real distance rather than a per-coordinate one, and the two
// normalizers keep the plot's density where it can be read.
//
// (The first four moved here with the functions when the recurrence math came
// out of embedding.go; the helpers sineWithNoise and lorenzSeries are still in
// embedding_test.go, which is the same package.)

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

// ── The vector form ──────────────────────────────────────────────────────

// RecurrenceMatrix is now a one-dimensional call into RecurrenceMatrixVec, so
// the scalar picture the audio source has always drawn has to be bit-identical
// to what the general routine produces — including at ε = 0, where the squared
// comparison the general routine uses could have differed from the |·| one it
// replaced.
func TestTheVectorFormReproducesTheScalarPictureExactly(t *testing.T) {
	x := sineWithNoise(80, 13, 0.08, 21)
	n := len(x)
	for _, eps := range []float64{0, 0.01, 0.15, 3} {
		a := make([]byte, n*n)
		b := make([]byte, n*n)
		RecurrenceMatrix(x, eps, a)
		RecurrenceMatrixVec(x, 1, eps, b)
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("ε=%v: cell %d is %d scalar, %d vector", eps, i, a[i], b[i])
			}
		}
	}
}

// A phase-space recurrence is a EUCLIDEAN neighborhood, not a box. The bug
// this pins is the easy one to write and impossible to see in the picture:
// testing each coordinate against ε separately, which is the Chebyshev norm
// and lights a square neighborhood instead of a round one. The two disagree by
// up to √dim, so the plot would simply be denser than ε says — plausible,
// wrong, and silent.
func TestVectorRecurrenceIsEuclideanAndNotPerCoordinate(t *testing.T) {
	// Two points a coordinate-wise 0.9 apart in both axes: 0.9 under any
	// per-coordinate test, 1.2728 apart in fact.
	pts := []float64{0, 0, 0.9, 0.9}
	m := make([]byte, 4)
	RecurrenceMatrixVec(pts, 2, 1.0, m)
	if m[1] != 0 {
		t.Error("ε=1.0 lit a pair 1.27 apart: the distance is being taken per coordinate, not as a norm")
	}
	RecurrenceMatrixVec(pts, 2, 1.3, m)
	if m[1] != 255 {
		t.Error("ε=1.3 left a pair 1.27 apart dark")
	}
}

// The diameter is the normalizer for a trajectory, so it has to be the widest
// pair and not, say, the widest spread on any one axis.
func TestDiameterIsTheWidestPair(t *testing.T) {
	// A right triangle with legs 3 and 4: the hypotenuse, 5, is the diameter,
	// and no single axis spans more than 4.
	pts := []float64{0, 0, 3, 0, 0, 4}
	if got := RecurrenceDiameter(pts, 2); math.Abs(got-5) > 1e-12 {
		t.Errorf("diameter %v, want 5", got)
	}
	if got := RecurrenceDiameter([]float64{1, 2, 3}, 3); got != 0 {
		t.Errorf("a single point has diameter %v, want 0", got)
	}
}

// delayEmbed builds n delay vectors of dim coordinates at delay tau, the way
// the audio path does it, as a flat series.
func delayEmbed(x []float64, dim, tau int) []float64 {
	base := (dim - 1) * tau
	n := len(x) - base
	if n < 1 {
		return nil
	}
	out := make([]float64, n*dim)
	for i := 0; i < n; i++ {
		for c := 0; c < dim; c++ {
			out[i*dim+c] = x[base+i-c*tau]
		}
	}
	return out
}

// THE CLAIM RecurrenceVectorScale EXISTS TO MAKE. Raising the embedding
// dimension must not change how dense the plot is by itself — otherwise the
// user turns the m knob, sees the picture thin out, and reads a change of
// setting as a change in the audio.
//
// Without the √m normalizer this fails plainly: the same ε has to reach across
// a space whose diagonal grew by √m, so the density falls away with dimension.
// With it, the same knob position holds the density flat to within a few
// percent over m = 2..8 — measured at 0.0194..0.0202 on this signal.
//
// m = 1 IS EXCLUDED FROM THE BAND, and honestly rather than to make the test
// pass. At m = 1 the plotted object is not the reconstructed curve at all; it
// is the raw signal's interval, where a sine piles up at its turning points, so
// the same ε lights three times as much (0.0606 here). No scale factor can
// reconcile those, because the difference is the shape of the object and not
// the size of the space it sits in — which is the same reason the mode offers
// raw audio and an embedding as separate settings rather than as m = 1 and
// m > 1 of one setting.
func TestVectorScaleHoldsDensityAcrossEmbeddingDimensions(t *testing.T) {
	x := sineWithNoise(1200, 40, 0.03, 5)
	const frac = 0.05
	rr := func(dim int) float64 {
		v := delayEmbed(x, dim, 10)
		n := len(v) / dim
		m := make([]byte, n*n)
		RecurrenceMatrixVec(v, dim, frac*RecurrenceVectorScale(dim), m)
		return RQA(m, n).RR
	}
	lo, hi := math.Inf(1), 0.0
	for dim := 2; dim <= 8; dim++ {
		r := rr(dim)
		t.Logf("m=%d: RR=%.4f", dim, r)
		lo, hi = math.Min(lo, r), math.Max(hi, r)
	}
	t.Logf("m=1 (raw signal, not a reconstruction): RR=%.4f", rr(1))
	if lo <= 0 {
		t.Fatal("some dimension lit nothing but the diagonal")
	}
	if hi/lo > 1.2 {
		t.Errorf("density ranges %.4f..%.4f over m=2..8, a factor of %.2f — "+
			"the dimension is moving the picture on its own", lo, hi, hi/lo)
	}
}

// ── RQA ──────────────────────────────────────────────────────────────────

// RR is the same quantity RecurrenceRate computes, read off the matrix instead
// of off the series. Two routes to one number, and the readout would be a lie
// if they disagreed.
func TestRQARateAgreesWithRecurrenceRate(t *testing.T) {
	x := sineWithNoise(120, 19, 0.1, 31)
	n := len(x)
	m := make([]byte, n*n)
	const eps = 0.2
	RecurrenceMatrix(x, eps, m)
	if got, want := RQA(m, n).RR, RecurrenceRate(x, eps); math.Abs(got-want) > 1e-12 {
		t.Errorf("RQA says RR=%.6f, RecurrenceRate says %.6f", got, want)
	}
}

// DET IS THE NUMBER THAT SEPARATES A SYSTEM FROM NOISE, and it can only do
// that if the line of identity is kept out of it. A periodic orbit's recurrence
// points lie on long diagonals and noise's are isolated specks; counting the
// main diagonal in would hand noise a perfect unbroken line n cells long and
// float its DET to something confident and meaningless.
//
// Both signals are plotted EMBEDDED, and the raw-scalar numbers are logged
// beside them because the gap is the argument for the embedding. A scalar
// signal is not injective — sin(t) = sin(T/2 − t) — so a raw tone's plot
// carries ANTI-diagonals as strong as its diagonals, and an anti-diagonal
// crosses the diagonals this statistic scans one cell at a time. The measured
// cost is a tone reading DET = 0.50 raw against 0.99 embedded: the plot is
// still perfectly structured, and half of that structure is at right angles to
// where DET looks for it. Which is the honest reason the mode's src knob offers
// an embedding and not only raw samples.
func TestDeterminismSeparatesAnOrbitFromNoise(t *testing.T) {
	const n = 400
	raw := make([]float64, n+16)
	for i := range raw {
		raw[i] = math.Sin(2 * math.Pi * float64(i) / 32)
	}
	rng := rand.New(rand.NewSource(77)) //nolint:gosec // a deterministic test signal, not a secret
	noise := make([]float64, n+16)
	for i := range noise {
		noise[i] = rng.NormFloat64()
	}

	// Matched densities, so the comparison is about STRUCTURE and not about one
	// plot simply being fuller than the other: ε is bisected per signal until
	// each lights about 5% of its square.
	det := func(v []float64, dim int) (float64, float64) {
		pts := len(v) / dim
		m := make([]byte, pts*pts)
		lo, hi := 1e-6, 100.0
		for k := 0; k < 50; k++ {
			mid := (lo + hi) / 2
			RecurrenceMatrixVec(v, dim, mid, m)
			if RQA(m, pts).RR < 0.05 {
				lo = mid
			} else {
				hi = mid
			}
		}
		RecurrenceMatrixVec(v, dim, hi, m)
		r := RQA(m, pts)
		return r.DET, r.RR
	}

	dt, rt := det(delayEmbed(raw, 2, 8), 2)
	dn, rn := det(delayEmbed(noise, 2, 8), 2)
	rawDet, _ := det(raw[:n], 1)
	t.Logf("embedded: tone DET=%.3f (RR=%.3f), noise DET=%.3f (RR=%.3f); raw tone DET=%.3f",
		dt, rt, dn, rn, rawDet)
	if dt < 0.9 {
		t.Errorf("an embedded tone reads DET=%.3f; a periodic orbit's recurrences are all on diagonals", dt)
	}
	if dn > 0.5 {
		t.Errorf("white noise reads DET=%.3f — the line of identity is leaking into the statistic", dn)
	}
	if dt <= dn {
		t.Errorf("DET does not separate the two at all: tone %.3f, noise %.3f", dt, dn)
	}
}

// The saturated case, where every definition has to agree: an ε wider than the
// data lights the whole square, which is one unbroken line per column and one
// per diagonal, so the picture is entirely lines.
//
// DET is 0.996 rather than 1, and the two cells it is short of are the ones in
// the far corners: the diagonals of offset ±(n−1) are one cell long, and a line
// of one is a coincidence rather than a line (RQALMin). Pinned as a bound
// rather than as equality so the arithmetic is described honestly — an exact
// 1.0 here would mean the corners had been special-cased to produce it.
func TestASolidSquareReadsAsEntirelyDeterministic(t *testing.T) {
	const n = 24
	m := make([]byte, n*n)
	for i := range m {
		m[i] = 255
	}
	r := RQA(m, n)
	if r.RR != 1 || r.LAM != 1 {
		t.Errorf("a solid square reads RR=%.3f LAM=%.3f, want 1 and 1", r.RR, r.LAM)
	}
	// n²−n cells off the diagonal, of which the two single-cell corners cannot
	// be on a line.
	if want := float64(n*n-n-2) / float64(n*n-n); math.Abs(r.DET-want) > 1e-12 {
		t.Errorf("DET=%.4f, want %.4f (everything but the two one-cell corners)", r.DET, want)
	}
	if r.Lit != n*n {
		t.Errorf("Lit=%d, want %d", r.Lit, n*n)
	}
}

// A bare diagonal — ε = 0 — is the other end: the picture is nothing but the
// line of identity, which the diagonal statistic excludes, so DET has nothing
// to be a fraction OF and must report 0 rather than divide by zero.
func TestABareDiagonalHasNoDeterminismToReport(t *testing.T) {
	const n = 16
	m := make([]byte, n*n)
	for i := 0; i < n; i++ {
		m[i*n+i] = 255
	}
	r := RQA(m, n)
	if r.Lit != n {
		t.Fatalf("Lit=%d, want %d", r.Lit, n)
	}
	if r.DET != 0 {
		t.Errorf("DET=%v with nothing off the diagonal, want 0", r.DET)
	}
	if math.Abs(r.RR-1/float64(n)) > 1e-12 {
		t.Errorf("RR=%v, want %v", r.RR, 1/float64(n))
	}
}

// ── The trajectory source ────────────────────────────────────────────────

// The matrix is a fixed-size texture, so the series that fills it has to be
// exactly the size asked for — a short one would leave part of the square
// unwritten and a long one would read past it.
func TestTrajectorySeriesIsExactlyTheLengthAsked(t *testing.T) {
	for _, mode := range FlowKeys() {
		span := RecurrenceSpan(mode, 10, 256)
		if span <= 0 {
			t.Errorf("%s: no span at all", mode)
			continue
		}
		s := TrajectorySeries(mode, 256, span)
		if len(s) != 256*3 {
			t.Errorf("%s: %d floats for 256 points of 3 coordinates", mode, len(s))
			continue
		}
		for i, v := range s {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("%s: coordinate %d is %v", mode, i, v)
				break
			}
		}
	}
}

// The step budget is what keeps a knob turn from wedging the tab, and the
// systems it has to protect against are the ones with tiny timesteps: Chen
// integrates at dt = 0.0005, so its span is capped an order of magnitude below
// Lorenz's for the same cost.
func TestSpanNeverBuysMoreStepsThanTheBudget(t *testing.T) {
	for _, mode := range FlowKeys() {
		sys, ok := flowFor4(mode)
		if !ok {
			t.Errorf("%s is in FlowKeys but has no 4D form", mode)
			continue
		}
		dt := sys.dt()
		span := RecurrenceSpan(mode, 1e9, 256)
		if steps := (recTrajTransient + span) / dt; steps > recTrajStepBudget+1 {
			t.Errorf("%s: an unbounded request bought %.0f steps, over the %d budget",
				mode, steps, recTrajStepBudget)
		}
	}
}

// The other end of the clamp, and the one a plausible-looking implementation
// gets wrong: a span short enough that the integrator produces fewer points
// than the plot has columns. Thomas runs at dt = 0.05, where the mode's default
// span of 10 time units is 200 steps against a 256-column square — that came
// back nil, the texture kept the previous frame, and the mode looked hung.
func TestSpanIsRaisedUntilItCanFillTheColumns(t *testing.T) {
	const n = 256
	for _, mode := range FlowKeys() {
		sys, _ := flowFor4(mode)
		span := RecurrenceSpan(mode, 10, n)
		if steps := span / sys.dt(); steps < n {
			t.Errorf("%s: a span of %v buys %.0f steps for a %d-column plot", mode, span, steps, n)
		}
		// A request that is already long enough must come back untouched, or
		// the floor is quietly rewriting every span rather than the short ones.
		if want := 200.0; want/sys.dt() >= n {
			if got := RecurrenceSpan(mode, want, n); got != want &&
				got != float64(recTrajStepBudget)*sys.dt()-recTrajTransient {
				t.Errorf("%s: a %v-unit span came back as %v", mode, want, got)
			}
		}
	}
}

// THE "USEFUL WITHOUT FIDDLING" CLAIM, PINNED. ε is one knob with one default,
// and it has to give a readable plot on every registered system without being
// retuned per model — which is exactly what a raw distance cannot do, since
// these attractors differ in width by more than an order of magnitude. Taking
// it as a fraction of each system's own diameter is what makes one number work
// everywhere, and "works" means the density lands somewhere a reader can see
// structure in: too sparse and there is nothing but the diagonal, too full and
// the square is white.
func TestTheDefaultEpsilonIsReadableOnEveryRegisteredSystem(t *testing.T) {
	const frac = 0.05 // the ε knob's default
	const n = 256
	m := make([]byte, n*n)
	worstLo, worstHi := "", ""
	lo, hi := 1.0, 0.0
	for _, mode := range FlowKeys() {
		s := TrajectorySeries(mode, n, RecurrenceSpan(mode, 10, n))
		if s == nil {
			t.Errorf("%s: no trajectory", mode)
			continue
		}
		d := RecurrenceDiameter(s, 3)
		if d <= 0 {
			t.Errorf("%s: zero diameter", mode)
			continue
		}
		RecurrenceMatrixVec(s, 3, frac*d, m)
		r := RQA(m, n)
		t.Logf("%-14s diam=%8.2f RR=%.4f DET=%.3f LAM=%.3f", mode, d, r.RR, r.DET, r.LAM)
		if r.RR < lo {
			lo, worstLo = r.RR, mode
		}
		if r.RR > hi {
			hi, worstHi = r.RR, mode
		}
	}
	// The band is wide on purpose. It is not a claim that every attractor
	// looks the same at one ε — they should not, since density IS information
	// — only that none of them comes up blank or saturated at the default.
	if lo < 0.002 {
		t.Errorf("%s lit only %.4f of the square at the default ε: nothing but the diagonal", worstLo, lo)
	}
	if hi > 0.5 {
		t.Errorf("%s lit %.4f of the square at the default ε: a white square", worstHi, hi)
	}
}
