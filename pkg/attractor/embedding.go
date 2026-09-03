package attractor

import "math"

// Choosing the delay embedding's two numbers by MEASURING the signal instead
// of guessing them.
//
// Takens' theorem says a delay vector reconstructs the attractor; it does not
// say which delay or how many coordinates. The τ knob's help text used to
// offer "a quarter period of the dominant frequency", which is a rule of
// thumb for a sine wave and nothing else — it has no dominant frequency to
// quarter when the input is speech, a drum kit, or a chaotic circuit. The two
// standard answers are here instead:
//
//   - τ from the FIRST MINIMUM of average mutual information (Fraser & Swinney
//     1986). Autocorrelation would only find the delay at which the signal is
//     LINEARLY unrelated to itself; mutual information finds where it is
//     unrelated at all, which is the right question for a nonlinear system.
//     The first minimum, not the global one: MI keeps falling toward the noise
//     floor forever, so the smallest delay that already decorrelates is what
//     is wanted — a longer one folds the attractor over itself.
//
//   - the dimension from FALSE NEAREST NEIGHBORS (Kennel, Brown & Abarbanel
//     1992). Two points can sit next to each other in an m-dimensional
//     projection only because the projection collapsed a direction they
//     differ in; add a coordinate and they fly apart. The dimension at which
//     that stops happening is the one where the attractor is unfolded.
//
// MEASURED ON DEMAND, NEVER PER FRAME. This is the constraint that shapes the
// whole file. Automatic scaling was removed from the Takens mode because it
// made the figure appear to zoom in and out with the music: anything that
// re-tunes every frame moves the picture in time with the audio, and the eye
// reads that motion as the signal rather than as the visualizer fidgeting.
// τ is a static property of the signal, so it is measured once, when the user
// asks, and written into the knob as a starting point that then holds still
// and can be turned by hand. Do NOT call these from a render loop — not for
// performance (a run is milliseconds) but because a knob that moves by itself
// sixty times a second is the exact behavior that was taken out.
//
// Untagged so the native tests can drive the estimators with signals of known
// answer — a sine's first MI minimum is a quarter period, Lorenz needs three
// dimensions — the way lyapunov.go is tested against known chaotic systems.

// EmbeddingResult is one measurement of a signal's embedding parameters.
type EmbeddingResult struct {
	Tau int     // delay, in samples of the input series
	MI  float64 // average mutual information at Tau, bits
	Dim int     // embedding dimension
	FNN float64 // false-neighbor fraction at Dim (0 = fully unfolded)
	OK  bool    // false when the signal was too short or too flat to measure
}

const (
	// amiBins is the histogram resolution for the mutual-information estimate.
	// Fraser & Swinney use an adaptive partition; a fixed equidistant one is
	// biased upward (empty cells inflate the estimate) but the bias is smooth
	// in τ, and only the LOCATION of the minimum is used here, not its height.
	// 16 bins over a few thousand points keeps ~10 counts per occupied cell.
	amiBins = 16

	// fnnRtol: a neighbor whose extra coordinate differs by more than this
	// multiple of the distance between the two points was never a neighbor,
	// only a projection artifact. Kennel's paper measures the plateau to start
	// around 10 and reports the answer insensitive between 10 and 50.
	fnnRtol = 15.0

	// fnnAtol guards the other failure: on a signal with structure at very
	// different scales the ratio test passes for pairs that are already far
	// apart relative to the attractor. A pair that ends up further than twice
	// the signal's standard deviation is false regardless of the ratio.
	fnnAtol = 2.0

	// fnnAccept is the fraction of false neighbors below which the attractor
	// counts as unfolded. Real data never reaches exactly zero — noise creates
	// neighbors that are genuinely spurious at every dimension.
	fnnAccept = 0.05

	// embedMinSamples is the shortest series worth measuring. Below this the
	// histogram is mostly empty cells and the answer is the bin count, not the
	// signal.
	embedMinSamples = 256
)

// MutualInformation is the average mutual information, in bits, between the
// series and itself delayed by tau: how much knowing s(t) tells you about
// s(t+τ). It is zero when the two are independent and maximal at τ=0.
func MutualInformation(x []float64, tau, bins int) float64 {
	if bins < 2 {
		bins = amiBins
	}
	n := len(x) - tau
	if tau < 0 || n < 2 {
		return 0
	}
	lo, hi := x[0], x[0]
	for _, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	rng := hi - lo
	if rng <= 0 { // a constant carries no information about anything
		return 0
	}
	scale := float64(bins) / rng
	bin := func(v float64) int {
		b := int((v - lo) * scale)
		if b < 0 {
			b = 0
		} else if b >= bins {
			b = bins - 1
		}
		return b
	}

	joint := make([]int, bins*bins)
	px := make([]int, bins)
	py := make([]int, bins)
	for i := 0; i < n; i++ {
		a, b := bin(x[i]), bin(x[i+tau])
		joint[a*bins+b]++
		px[a]++
		py[b]++
	}

	inv := 1 / float64(n)
	var mi float64
	for a := 0; a < bins; a++ {
		if px[a] == 0 {
			continue
		}
		pa := float64(px[a]) * inv
		for b := 0; b < bins; b++ {
			c := joint[a*bins+b]
			if c == 0 {
				continue
			}
			pab := float64(c) * inv
			pb := float64(py[b]) * inv
			mi += pab * math.Log2(pab/(pa*pb))
		}
	}
	return mi
}

// FirstMinimumTau returns the delay at the first local minimum of average
// mutual information, and the value there.
//
// "First local minimum" cannot be read off a raw comparison of neighbors. The
// binned MI estimate wobbles, and a one-sample wobble stopped the scan at τ=5
// on a tone whose answer is 25. What separates a real turn from a wobble is
// SIZE, so the scan walks forward from each candidate until MI either drops
// below it — in which case the candidate was a wobble and the search moves on
// — or climbs above it by more than the estimator's own noise, in which case
// the candidate was the minimum. No lookahead window and no smoothing window
// to tune: the curve itself decides how far to look.
//
// Measured on Lorenz this lands on τ = 0.20 time units, matching the published
// first minimum for that system; a wobble-tolerant fixed-window rule reported
// 0.60, three times too long, and a delay that long visibly folds the
// reconstruction (its false-neighbor fraction never falls below 4%).
func FirstMinimumTau(x []float64, maxTau int) (int, float64, bool) {
	if len(x) < embedMinSamples || maxTau < 2 {
		return 0, 0, false
	}
	if maxTau > len(x)/4 { // beyond this the estimate is made of few pairs
		maxTau = len(x) / 4
	}
	mi := make([]float64, maxTau+1)
	for t := 0; t <= maxTau; t++ {
		mi[t] = MutualInformation(x, t, amiBins)
	}
	floor := miNoiseFloor(len(x))
	for t := 1; t < maxTau; t++ {
		if mi[t] > mi[t-1] {
			continue // still climbing out of an earlier minimum
		}
		for k := t + 1; k <= maxTau; k++ {
			if mi[k] < mi[t] {
				break // not a minimum: the curve is still on its way down
			}
			if mi[k]-mi[t] > floor {
				return t, mi[t], true
			}
		}
	}
	return 0, 0, false
}

// miNoiseFloor is how much of the MI estimate is an artifact of counting a
// finite sample into a fixed grid rather than a property of the signal: the
// classical (Bx−1)(By−1)/2N nats of bias for a plug-in histogram estimate,
// converted to bits. A rise smaller than this is not evidence of anything. It
// scales with the window, which is what keeps the rule from needing a constant
// per signal length.
func miNoiseFloor(n int) float64 {
	if n < 2 {
		return 0
	}
	dof := float64((amiBins - 1) * (amiBins - 1))
	return dof / (2 * float64(n) * math.Ln2)
}

// FalseNearestFraction is the fraction of points whose nearest neighbor in an
// m-dimensional delay embedding stops being a neighbor when an (m+1)-th
// coordinate is added — the share of the attractor that is still folded over
// itself at this dimension.
//
// theiler excludes neighbors that are merely the next point along the same
// orbit. Without it every "nearest neighbor" is i±1, which is close for
// reasons of continuity rather than of geometry, and the method reports that
// one dimension suffices for everything.
func FalseNearestFraction(x []float64, tau, m, theiler int) float64 {
	if tau < 1 || m < 1 {
		return 1
	}
	n := len(x) - m*tau // vectors that also have the (m+1)-th coordinate
	if n < 32 {
		return 1
	}
	if theiler < 1 {
		theiler = tau * m
	}
	sigma := stdDev(x)
	if sigma <= 0 {
		return 1
	}

	// Cost is O(refs·n·m). Every point as a reference buys nothing here — the
	// answer is a fraction, and a few hundred samples of it are already stable
	// to well inside the 5% acceptance band — while the browser runs this
	// single-threaded on a button press.
	const maxRefs = 512
	step := 1
	if n > maxRefs {
		step = n / maxRefs
	}

	var checked, false0 int
	for i := 0; i < n; i += step {
		best, bestJ := math.Inf(1), -1
		for j := 0; j < n; j++ {
			if j >= i-theiler && j <= i+theiler {
				continue
			}
			var d2 float64
			for k := 0; k < m; k++ {
				d := x[i+k*tau] - x[j+k*tau]
				d2 += d * d
				if d2 >= best {
					break
				}
			}
			if d2 < best {
				best, bestJ = d2, j
			}
		}
		if bestJ < 0 || best <= 0 {
			continue // no admissible neighbor, or a duplicate point
		}
		r := math.Sqrt(best)
		delta := math.Abs(x[i+m*tau] - x[bestJ+m*tau])
		checked++
		if delta/r > fnnRtol || math.Sqrt(best+delta*delta)/sigma > fnnAtol {
			false0++
		}
	}
	if checked == 0 {
		return 1
	}
	return float64(false0) / float64(checked)
}

// EmbeddingDimension is the smallest m at which the false-neighbor fraction
// has dropped to fnnAccept — the number of delay coordinates needed to unfold
// the attractor without self-intersections.
func EmbeddingDimension(x []float64, tau, maxDim int) (int, float64, bool) {
	if maxDim < 1 {
		return 0, 1, false
	}
	for m := 1; m <= maxDim; m++ {
		f := FalseNearestFraction(x, tau, m, 0)
		if f <= fnnAccept {
			return m, f, true
		}
	}
	return maxDim, FalseNearestFraction(x, tau, maxDim, 0), false
}

// EstimateEmbedding measures both parameters of a delay reconstruction of x.
// One call, one answer, from a button — see the file comment on why this is
// not something to run per frame.
func EstimateEmbedding(x []float64, maxTau, maxDim int) EmbeddingResult {
	tau, mi, ok := FirstMinimumTau(x, maxTau)
	if !ok {
		return EmbeddingResult{}
	}
	dim, fnn, dimOK := EmbeddingDimension(x, tau, maxDim)
	return EmbeddingResult{Tau: tau, MI: mi, Dim: dim, FNN: fnn, OK: dimOK}
}

// stdDev is the sample standard deviation, the scale FNN's second test
// measures "too far apart" against.
func stdDev(x []float64) float64 {
	if len(x) < 2 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	mean := sum / float64(len(x))
	var ss float64
	for _, v := range x {
		d := v - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(x)-1))
}

// ── Recurrence plot ──────────────────────────────────────────────────────
//
// R[i][j] = 1 when |x_i − x_j| < ε (Eckmann, Kamphorst & Ruelle 1987). The
// picture is the whole point: a periodic signal draws diagonal lines spaced by
// its period, a chaotic one draws short broken diagonals, drift shows as the
// lit region fading away from the main diagonal, and a sudden change of state
// is a square block. It is the one view here that shows the TIME structure of
// the signal as a shape rather than as a moving trace.
//
// Untagged so the matrix itself can be tested against signals whose plot is
// known — a sine's diagonals must land on its period — with no GL context.

// RecurrenceMatrix fills dst, an n×n row-major byte image, with 255 where
// |x_i − x_j| < eps and 0 elsewhere. dst must hold len(x)² bytes.
//
// The matrix is symmetric and its main diagonal is always lit, which is why
// only the upper triangle is computed and mirrored: the plot is 65536 cells at
// the size drawn here and gets rebuilt as audio arrives.
func RecurrenceMatrix(x []float64, eps float64, dst []byte) {
	n := len(x)
	if n == 0 || len(dst) < n*n {
		return
	}
	for i := range dst[:n*n] {
		dst[i] = 0
	}
	for i := 0; i < n; i++ {
		dst[i*n+i] = 255
		for j := i + 1; j < n; j++ {
			if math.Abs(x[i]-x[j]) < eps {
				dst[i*n+j] = 255
				dst[j*n+i] = 255
			}
		}
	}
}

// RecurrenceRate is the fraction of the matrix that is lit — the density of
// the plot, and the number to turn ε by. Around 1–5% is the usual working
// range: below it the structure is too sparse to read, above it the plot
// saturates into a white square.
func RecurrenceRate(x []float64, eps float64) float64 {
	n := len(x)
	if n == 0 {
		return 0
	}
	var lit int
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			// i == j is lit by convention rather than by the test, exactly as
			// RecurrenceMatrix draws it: a point recurs with itself, and the
			// two functions describing the same picture must not disagree
			// about the one line every reader measures everything against.
			if i == j || math.Abs(x[i]-x[j]) < eps {
				lit++
			}
		}
	}
	return float64(lit) / float64(n*n)
}
