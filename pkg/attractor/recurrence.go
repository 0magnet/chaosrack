package attractor

import "math"

// Recurrence plots and the RQA scalars read off them — the picture of when a
// system returns to where it has already been (J.-P. Eckmann, S. O. Kamphorst
// and D. Ruelle, "Recurrence Plots of Dynamical Systems", Europhys. Lett. 4
// (1987) 973). R[i][j] is lit when the two states are within ε of each other,
// so the main diagonal is always lit and everything off it says how the system
// repeats: a periodic orbit draws unbroken diagonals spaced by its period, a
// chaotic one breaks them into short segments, a laminar passage is a solid
// block, and a change of regime is an edge running across the square.
//
// UNTAGGED, so the properties that make the picture legible are pinned by
// native tests rather than by looking at it — a periodic signal's plot must be
// invariant under shifting both indices by its period, and the matrix and the
// rate computed from it must agree. This file was split out of embedding.go,
// which is about ESTIMATING a delay embedding (τ by mutual information, m by
// false nearest neighbors); a recurrence plot CONSUMES an embedding, and with
// the vector form, the two normalizers and RQA it is three times the code the
// scalar version was.
//
// ── The scale problem, which is the whole reason this is not one function ──
//
// ε is a distance, and a distance is only meaningful against something. The
// three things this plots have wildly different scales: an audio sample is
// bounded to ±1, a Lorenz state ranges over about fifty units, a Chua state
// over five. One fixed ε is useful for exactly one of them.
//
// The answer here is deliberately NOT "measure the spread each frame and
// divide by it". That is adaptive normalization, and this repo has already
// paid for it once: the Takens mode auto-scaled its figure from the current
// audio level, and the result was a picture that zoomed in and out in time
// with the music, which the eye reads as structure in the signal. For a
// recurrence plot the same mistake shows up as DENSITY — the fraction of lit
// cells pulsing with the level — and a plot whose density is a function of
// something other than the dynamics is not a measurement of anything.
//
// So the normalizer must hold still, and there are two ways to make one:
//
//   - For a signal with a KNOWN BOUND, use the bound. Audio samples are bounded
//     to ±1, so a delay vector of m of them lives in a cube of side 2, and
//     RecurrenceVectorScale returns √m — that cube's half-diagonal — so ε as a
//     fraction of full scale means the same thing at every m. Nothing about the
//     current audio enters it, which is the point.
//
//   - For a trajectory with no bound, use the object's OWN diameter — noting
//     that a trajectory here is a STATIC object, integrated once from the
//     system's initial condition and recomputed only when the system changes.
//     Its diameter is therefore as still as the picture is. This is also the
//     convention the RQA literature quotes thresholds in (a few percent of the
//     maximum phase-space diameter), so 0.05 on the knob reads as "5% of the
//     attractor's width" and lands in the usable band without fiddling.
//
// The distinction is worth stating because it looks like an inconsistency and
// is not: both scales are constants of the thing being plotted, and neither is
// a function of what happens to be arriving this frame.

// RecurrenceMatrix fills dst, an n×n row-major byte image, with 255 where
// |x_i − x_j| < eps and 0 elsewhere. dst must hold len(x)² bytes.
//
// The scalar case of RecurrenceMatrixVec, kept under its own name because it is
// what the raw-audio path calls and what most of the tests are written against.
func RecurrenceMatrix(x []float64, eps float64, dst []byte) {
	RecurrenceMatrixVec(x, 1, eps, dst)
}

// RecurrenceMatrixVec is the same picture for a PHASE-SPACE trajectory: x holds
// n points of dim coordinates each, laid out flat (point i is x[i*dim:]), and a
// cell is lit when the Euclidean distance between the two points is under eps.
// dst must hold n² bytes, where n = len(x)/dim.
//
// The matrix is symmetric and its main diagonal is always lit, which is why
// only the upper triangle is computed and mirrored. Distances are compared
// SQUARED, so no square root is taken at all: at 256 points that removes 32640
// calls to math.Sqrt from a matrix the audio source rebuilds every frame, and
// the comparison is exactly equivalent for eps ≥ 0 (a negative eps is clamped
// to zero, where both forms light only the diagonal).
func RecurrenceMatrixVec(x []float64, dim int, eps float64, dst []byte) {
	if dim < 1 {
		return
	}
	n := len(x) / dim
	if n == 0 || len(dst) < n*n {
		return
	}
	if eps < 0 {
		eps = 0
	}
	e2 := eps * eps
	for i := range dst[:n*n] {
		dst[i] = 0
	}
	for i := 0; i < n; i++ {
		dst[i*n+i] = 255
		pi := x[i*dim : i*dim+dim]
		for j := i + 1; j < n; j++ {
			pj := x[j*dim : j*dim+dim]
			var d2 float64
			for c := 0; c < dim; c++ {
				d := pi[c] - pj[c]
				d2 += d * d
			}
			if d2 < e2 {
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
//
// The scalar reference implementation, computed straight from the series
// rather than from a matrix, so the matrix can be checked against something
// that does not share its indexing.
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

// RecurrenceVectorScale is the fixed normalizer for a delay vector built from
// samples with a known bound of ±1: the half-diagonal of the cube those vectors
// live in, √m. It exists so ε means "this fraction of full scale" at every
// embedding dimension — without it, raising m from 1 to 8 thins the plot by
// itself, because the same ε has to cover a distance that grew by √8, and the
// user reads a change of setting as a change in the audio.
//
// Nothing about the current signal is measured here, which is the requirement
// rather than an approximation: see the file comment on why the normalizer must
// hold still.
func RecurrenceVectorScale(dim int) float64 {
	if dim < 1 {
		return 1
	}
	return math.Sqrt(float64(dim))
}

// RecurrenceDiameter is the largest distance between any two of the points —
// the attractor's width, and the normalizer ε is a fraction of for a
// trajectory, which has no full scale of its own.
//
// The maximum rather than a standard deviation, because it is the convention
// the RQA literature quotes thresholds in and because it makes the knob's
// number mean something a reader can check by eye: 0.05 lights pairs within a
// twentieth of the figure's width of each other. The honest cost is that one
// near-escape widens the diameter and thins the whole plot; that is what the RR
// readout is for, and Trajectory already drops a run that actually diverges.
//
// O(n²·dim), the same cost as the matrix itself, so it is only ever computed
// where the matrix is — on a trajectory that has just been re-integrated, never
// per frame.
func RecurrenceDiameter(x []float64, dim int) float64 {
	if dim < 1 {
		return 0
	}
	n := len(x) / dim
	var max2 float64
	for i := 0; i < n; i++ {
		pi := x[i*dim : i*dim+dim]
		for j := i + 1; j < n; j++ {
			pj := x[j*dim : j*dim+dim]
			var d2 float64
			for c := 0; c < dim; c++ {
				d := pi[c] - pj[c]
				d2 += d * d
			}
			if d2 > max2 {
				max2 = d2
			}
		}
	}
	return math.Sqrt(max2)
}

// ── Recurrence quantification analysis ───────────────────────────────────
//
// The plot is read by eye; RQA puts three numbers on what the eye is doing
// (N. Marwan, M. C. Romano, M. Thiel and J. Kurths, "Recurrence plots for the
// analysis of complex systems", Phys. Rep. 438 (2007) 237). They belong HERE,
// beside the drawing, rather than behind an on-demand button the way the
// Lyapunov exponent is: they are read off a matrix that has already been built
// in order to be drawn, so there is nothing to re-derive and nothing to wait
// for. The caller still rate-limits the readout — see the js side — because
// "cheap" is not "free" and a number nobody can read at 60 Hz should not be
// computed at 60 Hz.

// RQAResult holds the three scalars, each 0..1.
type RQAResult struct {
	// RR — recurrence rate: the fraction of the square that is lit. The
	// density of the picture, and the number ε is turned by.
	RR float64
	// DET — determinism: the fraction of recurrence points lying on a diagonal
	// line of at least RQALMin cells. A diagonal is a repeated visit to the
	// same stretch of trajectory, so this separates a deterministic system
	// (high) from noise, which recurs in isolated specks and reads near zero.
	DET float64
	// LAM — laminarity: the same for VERTICAL lines, which are states the
	// system sat in rather than passed through. High LAM against lower DET is
	// intermittency — laminar phases interrupted by bursts.
	LAM float64
	// Lit is how many cells the fractions were taken over, so a caller can
	// tell "0.00 because nothing recurred" from "0.00 because nothing lined
	// up".
	Lit int
}

// RQALMin is the shortest run of cells counted as a line. Two is the smallest
// value that means anything — one cell is a coincidence, not a line — and
// raising it is the usual way to suppress the tail of short diagonals noise
// contributes. Fixed rather than a knob: a fourth control on this panel would
// earn less than it costs, and the number that actually needs turning is ε.
const RQALMin = 2

// RQA computes the three scalars from an n×n matrix already filled by
// RecurrenceMatrixVec (lit cells non-zero).
//
// THE MAIN DIAGONAL IS EXCLUDED FROM THE DIAGONAL STATISTIC, and that is not a
// detail. Every point recurs with itself, so the line of identity is n cells
// long and perfectly unbroken; counted in, it makes DET of white noise read
// about 0.3 at the sizes drawn here — a confident number that says nothing
// about the signal and everything about the fact that a point equals itself.
// It is left IN for RR, which is a density and the diagonal is genuinely part
// of the picture, and left in for the vertical statistic, where it contributes
// at most one cell per column and so can only lengthen a real line by one — an
// error of one part in the line length, against the alternative of treating it
// as a break and splitting genuine vertical lines in half.
//
// The two denominators therefore differ, deliberately: DET is over the lit
// cells off the diagonal, LAM over all of them. Both are the standard
// definitions, and each is stated in the field comments above.
//
// ONE ROW-MAJOR PASS, not one walk per line. The obvious implementation walks
// each diagonal end to end, which steps through memory n+1 bytes at a time: at
// n = 256 every one of the 65536 accesses lands on its own cache line, and that
// version measured 469 µs — MORE than filling the matrix cost (292 µs), for
// what is nominally two scans of 64 KB. Walking in row order instead makes the
// scan of the big array sequential and keeps the state in two small ones that
// stay in L1: a run length per diagonal (2n−1 of them) and one per column (n).
// Same definitions, same numbers, 283 µs. (Native amd64, n = 256; wasm is
// slower by roughly the same factor throughout, so the ratio is what matters.)
func RQA(mat []byte, n int) RQAResult {
	var r RQAResult
	if n < 2 || len(mat) < n*n {
		return r
	}
	// dRun[j-i+n-1] and vRun[j] are the lengths of the runs currently open on
	// that diagonal and that column. 3n int32s — 3 KB at the size drawn here,
	// against the 64 KB matrix — which is the whole trick: the sequential scan
	// is over the big array and the random access is over the small ones.
	dRun := make([]int32, 2*n-1)
	vRun := make([]int32, n)
	var dLine, dLit, vLine int64

	for i := 0; i < n; i++ {
		row := mat[i*n : i*n+n]
		for j := 0; j < n; j++ {
			lit := row[j] != 0
			if lit {
				r.Lit++
				vRun[j]++
			} else if vRun[j] >= RQALMin {
				vLine += int64(vRun[j])
				vRun[j] = 0
			} else {
				vRun[j] = 0
			}
			if j == i {
				continue // the line of identity, excluded from the diagonals
			}
			d := j - i + n - 1
			if lit {
				dLit++
				dRun[d]++
			} else if dRun[d] >= RQALMin {
				dLine += int64(dRun[d])
				dRun[d] = 0
			} else {
				dRun[d] = 0
			}
		}
	}
	// Runs still open when the scan ran off the edge of the square are lines
	// too — the plot's structure does not stop because the window did.
	for _, v := range dRun {
		if v >= RQALMin {
			dLine += int64(v)
		}
	}
	for _, v := range vRun {
		if v >= RQALMin {
			vLine += int64(v)
		}
	}

	r.RR = float64(r.Lit) / float64(n*n)
	if dLit > 0 {
		r.DET = float64(dLine) / float64(dLit)
	}
	if r.Lit > 0 {
		r.LAM = float64(vLine) / float64(r.Lit)
	}
	return r
}

// ── A trajectory as a series of phase-space points ───────────────────────

// recTrajTransient is how long a trajectory is integrated before recording, in
// the system's own time units — the run-in from the initial condition onto the
// attractor, which is not part of the attractor. The same value
// DefaultTrajectory uses, for the same reason.
const recTrajTransient = 20

// recTrajStepBudget caps the integration one trajectory may cost. It is the
// difference between a mode that redraws and one that wedges the tab, and the
// numbers are not close: the span asked for is divided by the system's own dt,
// and Chen runs at dt = 0.0005, so a 200-time-unit span is 440000 RK4 steps
// where Lorenz at dt = 0.005 needs 44000 for the same span. 300000 is a run of
// a few tens of milliseconds — a visible hitch when the system changes, which
// is what it is, and nothing at all on the frames in between, because the
// result is cached until the system changes again.
const recTrajStepBudget = 300000

// RecurrenceSpan clamps a requested span, in the system's own time units, to
// the range that produces an n-point plot on that system without costing more
// than recTrajStepBudget. Returns 0 for a mode with no registered flow.
//
// BOTH ENDS, and both are the same fact seen twice: the number of integration
// steps a span buys is span/dt, and dt varies by two orders of magnitude across
// the catalog.
//
//   - The ceiling is cost. Chen runs at dt = 0.0005, so a 200-unit span is
//     440000 steps where Lorenz needs 44000 for the same span.
//   - The floor is that a plot needs n points to fill n columns. Thomas runs at
//     dt = 0.05, so a 10-unit span is 200 steps — and a 256-column plot cannot
//     be drawn from 200 of them. Without this it came back nil and the mode
//     showed the previous frame forever, which looks exactly like a hang.
//
// It CLAMPS rather than refusing, because the alternative is a knob that stops
// doing anything past a point that differs per model with no indication why.
// The plot then covers a different span from the one asked for, which is
// visible in the picture — more or fewer diagonals — rather than silent.
func RecurrenceSpan(mode string, want float64, n int) float64 {
	sys, ok := flowFor4(mode)
	if !ok {
		return 0
	}
	dt := sys.dt()
	if dt <= 0 || want <= 0 || n < 2 {
		return 0
	}
	if floor := float64(n) * dt; want < floor {
		want = floor
	}
	if ceil := float64(recTrajStepBudget)*dt - recTrajTransient; want > ceil {
		if ceil <= 0 {
			return 0
		}
		return ceil
	}
	return want
}

// TrajectorySeries integrates a registered flow and returns exactly n points of
// its (x,y,z) path, flat, spanning span time units past the transient — the
// form RecurrenceMatrixVec takes. Returns nil for a mode with no vector field,
// or for a run that diverged before it had n points.
//
// THINNED BY SELECTION, NOT BY AVERAGING, and the audio path next door does the
// opposite for a reason worth writing down. Decimating a bandlimited SIGNAL by
// keeping one sample in every k aliases, so the audio source box-averages each
// stride first. A trajectory is not a signal: it is a curve in space, and
// averaging k consecutive points on a curve returns their centroid, which lies
// off the curve, inside the bend. For a recurrence plot that is worse than
// aliasing, because two chords cutting the same corner come out closer to each
// other than the arcs they replaced — the plot would gain recurrences the
// system does not have. Selection keeps every plotted point ON the attractor.
// What it costs is that when the span is long enough for consecutive selected
// points to be further apart than the orbit's own scale, the picture stops
// being a picture of an orbit; that is a property of the span asked for rather
// than of the method, and it is visible in the readout, because DET collapses
// as soon as the sampling no longer resolves the orbit.
//
// It integrates at 4n points and selects n of them, so that the curve handed to
// the selection is dense — rather than selecting out of a series Trajectory has
// already thinned to n by its own rule.
func TrajectorySeries(mode string, n int, span float64) []float64 {
	if n < 2 || span <= 0 {
		return nil
	}
	pts := Trajectory(mode, TrajectoryOptions{
		Transient: recTrajTransient,
		Duration:  span,
		MaxPoints: 4 * n,
	})
	m := len(pts)
	if m < n {
		return nil
	}
	out := make([]float64, 3*n)
	for i := 0; i < n; i++ {
		p := pts[i*(m-1)/(n-1)]
		out[3*i], out[3*i+1], out[3*i+2] = p[0], p[1], p[2]
	}
	return out
}
