package attractor

import "math"

// The running largest-Lyapunov accumulator behind the panel's live λ readout.
//
// lyapunov.go already measures the exponent, but it measures it the way a test
// does: seed, discard a long transient, run a few hundred thousand steps,
// answer once. The Analysis module runs exactly that, on demand, for exactly
// that reason — it is a few milliseconds, which is nothing once and a frame
// killer sixty times a second.
//
// This is the other half. The Trace > Twin switch has always DRAWN the same
// measurement — two copies of the flow ε apart, separating at the attractor's
// own rate — and a picture of divergence is not a number. What was missing was
// a resumable accumulation: one that owes each frame only a slice, so it can
// run continuously beside the render loop instead of stopping it.
//
// UNITS — the thing to get wrong here. λ is per unit of MODEL time: the time
// the integrator counts, dt per step. It is NOT per second of wall clock, and
// nothing in this file reads a clock. A frame that integrates twice as far
// advances model time twice as far, and the quotient is unchanged; a dropped
// frame does not move the answer. That is the property that makes the readout
// stable while the browser is busy.
//
// It follows that the value DEPENDS ON dt, and that this is not an artifact to
// be apologized for. What the app runs is not the flow, it is the discrete map
// s → s + dt·f(s) (or the RK4 step), and that map's exponent moves as dt moves
// — which is why lyapunov.go integrates each mode with the scheme its own
// render loop uses, and why the Speed knob, which multiplies dt, genuinely
// changes the number rather than merely reaching it sooner.
//
// Reading it: positive means chaos — nearby states separate exponentially, so
// prediction has a horizon of about 1/λ time units. Near zero means a limit
// cycle or a torus: neighbors neither separate nor converge. Negative means a
// fixed point or a decaying transient. classify() in lyapunov.go puts the
// thresholds on that, and the readout uses the same ones, because two verdicts
// that disagreed about the same system would be worse than one.

// lyapLiveD0 is the probe pair's separation, in state-space units — the
// live twin of lyapDefaultD0 and the same value for the same reason: far
// enough above float64 round-off at attractor coordinates to be a real
// distance, far enough below the attractor's own extent that the pair stays
// in the linear regime instead of folding around the attractor before the
// interval is up.
const lyapLiveD0 = 1e-4

// lyapLiveInterval is the model time between renormalizations, matching the
// offline estimator's one time unit. It is a compromise the estimator has
// already made: long enough that the separation grows measurably above
// round-off, short enough that it has not saturated across the attractor,
// where the log would report the attractor's diameter and not a rate.
const lyapLiveInterval = 1.0

// lyapLiveWarmup is the model time discarded before anything is accumulated.
// Two transients have to die in it, not one: the trajectory's approach ONTO
// the attractor, and the separation direction's convergence onto the most
// unstable one. The second is what makes this the LARGEST exponent rather
// than some exponent, and it only happens while the pair is being
// renormalized — so the warmup renormalizes and throws the logs away, it does
// not simply wait.
const lyapLiveWarmup = 60.0

// lyapLiveMinTime is how much model time must be averaged before the readout
// shows a number at all. Below it the answer is mostly transient, and a
// confident "+0.31" that will be "+0.90" a second later is worse than "--":
// it is wrong in the one direction the readout exists to be right in, since
// 0.31 and 0.90 are both "chaotic" but 0.02 and 0.90 are not.
//
// 300 is where the Lorenz estimate stops moving in the third decimal
// (TestLiveLyapunovLorenzConvergence measures it). At the default speed the
// probe covers that in well under two seconds, so the cost of the honesty is
// a dash for about a second after a mode change.
const lyapLiveMinTime = 300.0

// liveLyapunov accumulates ln(separation/d0) against the model time it took,
// one renormalization interval at a time. Zero value is not usable — reset()
// installs the warmup.
type liveLyapunov struct {
	sum  float64 // Σ ln(d/d0) over completed, post-warmup intervals
	time float64 // model time those intervals span
	tau  float64 // model time since the last renormalization
	warm float64 // model time still to discard
}

// reset restarts the accumulation, e.g. because the mode or a coefficient
// changed and the exponent now belongs to a different system.
func (l *liveLyapunov) reset() { *l = liveLyapunov{warm: lyapLiveWarmup} }

// advance records that dt of model time has passed with the pair currently d
// apart, and folds the interval in once a full one has elapsed. It returns the
// factor the caller must scale the separation VECTOR by to pull the copy back
// to d0 — the direction is kept, which is the whole trick — and whether a
// renormalization happened at all.
//
// The caller passes d rather than the two states so that this stays testable
// without a flow, a stepper or a GL context: the arithmetic that can be got
// wrong is here, and integrating is the caller's business.
func (l *liveLyapunov) advance(dt, d float64) (scale float64, renormed bool) {
	if !(dt > 0) {
		return 1, false
	}
	l.tau += dt
	if l.tau < lyapLiveInterval {
		return 1, false
	}
	tau := l.tau
	l.tau = 0
	if !(d > 0) || math.IsInf(d, 0) {
		// The pair collapsed or blew up. Drop the interval rather than
		// logging a zero or an infinity into a running sum that has no way
		// back out of one; the caller reseeds if the states themselves went.
		return 1, false
	}
	if l.warm > 0 {
		l.warm -= tau
		return lyapLiveD0 / d, true
	}
	l.sum += math.Log(d / lyapLiveD0)
	l.time += tau
	return lyapLiveD0 / d, true
}

// lambda is the exponent per unit of model time, and whether enough model time
// has accumulated for it to mean anything.
//
// Σlog / Σtime, not the mean of the per-interval rates. The two agree only
// when every interval is the same length, and they are not: an interval ends
// on the first step that crosses lyapLiveInterval, so it overshoots by up to
// one dt, and a mode's dt moves with its own knob and with Speed. Averaging
// rates would then weight a short interval the same as a long one, which is
// the wrong quotient — the exponent is a total growth over a total time.
func (l *liveLyapunov) lambda() (float64, bool) {
	if l.time < lyapLiveMinTime {
		return 0, false
	}
	lam := l.sum / l.time
	if math.IsNaN(lam) || math.IsInf(lam, 0) {
		return 0, false
	}
	return lam, true
}
