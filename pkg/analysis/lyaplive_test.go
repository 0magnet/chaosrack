package analysis

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The accumulator is deliberately separable from the flow, the stepper and the
// DOM: everything below runs on the host, with no GL context and no browser.

// liveReadyTime is the model time a driven run needs before Lambda() will
// answer. It is not warmup + threshold exactly: an interval closes on the
// first step that CROSSES liveInterval, so each one overshoots by up to a
// dt and the last partial one is never counted. Ten intervals of slack covers
// that for any dt these systems use.
const liveReadyTime = liveWarmup + LiveMinTime + 10*liveInterval

// rateFeeder drives the accumulator with a separation growing at exactly rate
// per unit of model time, which is the one case where the answer is known in
// closed form. The separation is state on the feeder rather than a local, so
// that a run split into two phases at different rates is CONTINUOUS across the
// split — restarting d at d0 mid-interval would put a partly-grown separation
// against a full interval's time and cost a few parts in ten thousand, which
// is exactly the size of the leak these tests are looking for.
type rateFeeder struct{ d float64 }

func newRateFeeder() *rateFeeder { return &rateFeeder{d: LiveD0} }

func (f *rateFeeder) feed(l *LiveLyapunov, rate, dt, until float64) {
	for t := 0.0; t < until; t += dt {
		f.d *= math.Exp(rate * dt)
		sc, _ := l.Advance(dt, f.d)
		f.d *= sc
	}
}

// lyapTestStep is the model-time step every test feeds at. Small enough that
// the exponent estimate is not dominated by the integration error, and the
// same for all of them so their run lengths are comparable.
const lyapTestStep = 0.01

// feedRate is the single-phase case.
func feedRate(l *LiveLyapunov, rate, until float64) {
	newRateFeeder().feed(l, rate, lyapTestStep, until)
}

func TestLiveLyapunovConstantRate(t *testing.T) {
	for _, rate := range []float64{0.9, 0, -0.4} {
		var l LiveLyapunov
		l.Reset()
		feedRate(&l, rate, liveWarmup+LiveMinTime+50)
		lam, ok := l.Lambda()
		if !ok {
			t.Fatalf("rate %v: not ready after %v model time", rate, liveWarmup+LiveMinTime+50)
		}
		if math.Abs(lam-rate) > 1e-9 {
			t.Errorf("rate %v: lambda = %v, want %v", rate, lam, rate)
		}
	}
}

// The guard is the point of the readout: until enough model time has gone by,
// there is no number, only a dash.
func TestLiveLyapunovNotReadyBeforeMinTime(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	// One interval short of the threshold, warmup included.
	feedRate(&l, 0.9, liveWarmup+LiveMinTime-2*liveInterval)
	if lam, ok := l.Lambda(); ok {
		t.Fatalf("reported %v with only %v model time accumulated; want not-yet-meaningful", lam, l.time)
	}
	feedRate(&l, 0.9, 4*liveInterval)
	if _, ok := l.Lambda(); !ok {
		t.Fatalf("still not ready after %v model time, threshold is %v", l.time, LiveMinTime)
	}
}

// A fresh accumulator reports nothing at all, rather than 0/0 or a zero that
// would read as "periodic".
func TestLiveLyapunovZeroTimeReportsNothing(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	if _, ok := l.Lambda(); ok {
		t.Fatal("a reset accumulator reported a value")
	}
	if _, ok := (&LiveLyapunov{}).Lambda(); ok {
		t.Fatal("a zero accumulator reported a value")
	}
}

// The warmup has to be DISCARDED, not merely survived: a transient that
// separated ten times faster than the attractor does must leave no trace in
// the average.
func TestLiveLyapunovWarmupIsDiscarded(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	f := newRateFeeder()
	f.feed(&l, 9.0, 0.01, liveWarmup)
	if l.sum != 0 || l.time != 0 {
		t.Fatalf("warmup accumulated sum=%v time=%v, want both zero", l.sum, l.time)
	}
	f.feed(&l, 0.9, 0.01, LiveMinTime+10)
	lam, ok := l.Lambda()
	if !ok {
		t.Fatal("not ready after the warmup plus a full averaging window")
	}
	// Not exact equality, and the difference is not slop. Whatever growth
	// happened AFTER the warmup's last renormalization is still in the
	// separation when the first counted interval closes, so up to one dt of
	// the transient's rate can survive — here at most 9.0 × 0.01 of log,
	// spread over the whole averaging window. Asserting equality instead
	// would be asserting that the phase change happened to land on an
	// interval boundary, which is a fact about the test.
	leak := 9.0 * 0.01 / l.time
	if leak > 0.001 {
		t.Fatalf("the bound itself is %v — too loose to be evidence of anything", leak)
	}
	if math.Abs(lam-0.9) > leak+1e-12 {
		t.Errorf("lambda = %v, want 0.9 ± %v — the warmup leaked into the average", lam, leak)
	}
}

// dt is not constant in the app (the dt knob and Speed both move it), so the
// quotient must be total-growth over total-time and not a mean of rates.
func TestLiveLyapunovUnequalIntervalsWeightByTime(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	l.warm = 0
	// One interval at dt = 1 (exactly one time unit) and one at dt = 3 (three
	// time units in a single step), both growing at rate 0.5. A mean of rates
	// and a time-weighted quotient agree on the answer here only because the
	// rate is the same in both — so also check the denominator.
	for _, dt := range []float64{1, 3} {
		sc, ok := l.Advance(dt, LiveD0*math.Exp(0.5*dt))
		if !ok {
			t.Fatalf("dt %v did not close an interval", dt)
		}
		_ = sc
	}
	if math.Abs(l.time-4) > 1e-12 {
		t.Errorf("accumulated time = %v, want 4 — intervals are being counted, not timed", l.time)
	}
	if math.Abs(l.sum/l.time-0.5) > 1e-12 {
		t.Errorf("lambda = %v, want 0.5", l.sum/l.time)
	}
}

// A collapsed or blown-up pair contributes nothing rather than ±Inf, which a
// running sum has no way back out of.
func TestLiveLyapunovRejectsDegenerateSeparation(t *testing.T) {
	for _, d := range []float64{0, -1, math.Inf(1), math.NaN()} {
		var l LiveLyapunov
		l.Reset()
		l.warm = 0
		if _, ok := l.Advance(liveInterval, d); ok {
			t.Errorf("separation %v renormalized; want the interval dropped", d)
		}
		if l.sum != 0 || l.time != 0 {
			t.Errorf("separation %v accumulated sum=%v time=%v", d, l.sum, l.time)
		}
		if lam, ok := l.Lambda(); ok {
			t.Errorf("separation %v reported %v", d, lam)
		}
	}
}

// A non-advancing clock must not close intervals — the render loop calls this
// per sub-step and a paused or zero-dt system would otherwise pile up
// renormalizations of a separation that never grew.
func TestLiveLyapunovIgnoresNonPositiveDT(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	l.warm = 0
	for i := 0; i < 1000; i++ {
		if _, ok := l.Advance(0, LiveD0*2); ok {
			t.Fatal("dt = 0 closed an interval")
		}
	}
	if l.tau != 0 || l.time != 0 {
		t.Errorf("dt = 0 advanced the clock: tau=%v time=%v", l.tau, l.time)
	}
}

// driveFlow runs the accumulator against a real registered flow, integrating
// the way the app integrates that mode — the distinction lyapunov.go had to
// learn, and the reason the live probe consults dynamics.IsClassic too.
func driveFlow(mode string, modelTime float64) (*LiveLyapunov, bool) {
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return nil, false
	}
	dt := sys.Dt()
	if dt <= 0 {
		return nil, false
	}
	euler := dynamics.IsClassic(mode)
	step := func(s *[4]float64) {
		if euler || sys.Euler {
			dx, dy, dz, dw := sys.F(s[0], s[1], s[2], s[3])
			s[0] += dt * dx
			s[1] += dt * dy
			s[2] += dt * dz
			s[3] += dt * dw
			return
		}
		*s = dynamics.RK4x4(sys.F, dt, *s)
	}
	ic := dynamics.InitCondFor(mode)
	a := [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W0}
	b := a
	b[0] += LiveD0

	l := &LiveLyapunov{}
	l.Reset()
	for t := 0.0; t < modelTime; t += dt {
		step(&a)
		step(&b)
		var d2 float64
		for k := 0; k < 4; k++ {
			e := b[k] - a[k]
			d2 += e * e
		}
		sc, renormed := l.Advance(dt, math.Sqrt(d2))
		if renormed {
			for k := 0; k < 4; k++ {
				b[k] = a[k] + (b[k]-a[k])*sc
			}
		}
	}
	return l, true
}

// The live estimate has to agree with the offline one, or the panel and the
// Analysis module would show two different exponents for the same system on
// screen at the same time. Lorenz because its exponent is a published number
// (~0.9) that both are checked against elsewhere.
func TestLiveLyapunovAgreesWithOfflineLorenz(t *testing.T) {
	l, ok := driveFlow("lorenz", liveReadyTime)
	if !ok {
		t.Skip("lorenz flow not registered in this build")
	}
	live, ready := l.Lambda()
	if !ready {
		t.Fatalf("not ready after %v model time (accumulated %v)", liveReadyTime, l.time)
	}
	off := LyapunovForFlow("lorenz")
	if !off.OK {
		t.Skip("offline estimator could not measure lorenz")
	}
	if Classify(live) != Classify(off.Lambda) {
		t.Errorf("live %v (%s) and offline %v (%s) disagree on the verdict",
			live, Classify(live), off.Lambda, Classify(off.Lambda))
	}
	// A tenth of an exponent: the live estimate averages over a window three
	// hundred times shorter than the offline one and is expected to sit a
	// little off it, but not far enough to matter to anybody reading it.
	if math.Abs(live-off.Lambda) > 0.1 {
		t.Errorf("live lambda %v vs offline %v, difference %v exceeds 0.1",
			live, off.Lambda, math.Abs(live-off.Lambda))
	}
}

// LiveMinTime is a claim about where the estimate settles, so measure it
// rather than asserting it: past the threshold the value must stay inside the
// band the readout displays to two decimals.
func TestLiveLyapunovLorenzConvergence(t *testing.T) {
	sys, ok := dynamics.FlowFor4("lorenz")
	if !ok {
		t.Skip("lorenz flow not registered in this build")
	}
	_ = sys
	base, _ := driveFlow("lorenz", liveReadyTime)
	ref, ready := base.Lambda()
	if !ready {
		t.Fatal("threshold reached without a reading")
	}
	for _, extra := range []float64{300, 900, 2700} {
		l, _ := driveFlow("lorenz", liveReadyTime+extra)
		lam, ok := l.Lambda()
		if !ok {
			t.Fatalf("+%v: not ready", extra)
		}
		if math.Abs(lam-ref) > 0.02 {
			t.Errorf("at the threshold lambda = %.4f, after %v more model time %.4f "+
				"— still moving by more than the readout's own resolution", ref, extra, lam)
		}
	}
}

// A periodic system must read as periodic, not merely as "small": that is the
// distinction the readout exists to draw, and the one a too-short window
// destroys first.
func TestLiveLyapunovPeriodicReadsPeriodic(t *testing.T) {
	var l LiveLyapunov
	l.Reset()
	feedRate(&l, 0, liveWarmup+LiveMinTime+10)
	lam, ok := l.Lambda()
	if !ok {
		t.Fatal("not ready")
	}
	if Classify(lam) != "periodic" {
		t.Errorf("lambda %v classified %q, want periodic", lam, Classify(lam))
	}
}
