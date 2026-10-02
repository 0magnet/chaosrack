//go:build js && wasm

package audiosrc

import (
	"math"
	"testing"
)

// run is n samples of the mix.
func run(t *testing.T, b *Bus, n int) (l, r []float32) {
	t.Helper()
	l, r = make([]float32, n), make([]float32, n)
	if got := b.DrainStereo(l, r); got != n {
		t.Fatalf("DrainStereo gave %d of %d with no capture in the mix", got, n)
	}
	return l, r
}

// mixed is a bus with generator i alone, at full level on wave, at gain gl
// on RACK L and gr on RACK R.
func mixed(i, wave int, gl, gr float32) *Bus {
	f := NewFuncGen()
	f.SetWave(i, wave)
	f.SetAmp(i, 1)
	b := NewBus(f)
	b.Mix[0][InGen+In(i)] = gl
	b.Mix[1][InGen+In(i)] = gr
	return b
}

func energy(s []float32) float64 {
	var e float64
	for _, v := range s {
		e += float64(v) * float64(v)
	}
	return e
}

// A source is on the side its pin is on, and on neither without one.
func TestThePinIsTheSide(t *testing.T) {
	l, r := run(t, mixed(OscZ, WaveSine, 0, 1), 4800)
	if energy(l) != 0 || energy(r) == 0 {
		t.Errorf("Z on R: left energy %.1f, right %.1f", energy(l), energy(r))
	}
	l, r = run(t, mixed(OscV, WaveSine, 1, 0), 4800)
	if energy(l) == 0 || energy(r) != 0 {
		t.Errorf("V on L: left energy %.1f, right %.1f", energy(l), energy(r))
	}
	l, r = run(t, mixed(OscY, WaveSine, 0, 0), 4800)
	if energy(l)+energy(r) != 0 {
		t.Error("Y with no pin is in the mix")
	}
}

// A pin's gain scales the source, and a negative one inverts it: at +1 on L
// and −1 on R a generator is L−R, nothing in the mono mix.
func TestANegativePinInverts(t *testing.T) {
	b := mixed(OscX, WavePink, 1, -1)
	l, r := run(t, b, 4800)
	for i := range l {
		if l[i] != -r[i] {
			t.Fatalf("sample %d: L %.4f, R %.4f", i, l[i], r[i])
		}
	}
	m := make([]float32, 4800)
	b.Drain(m)
	if e := energy(m); e != 0 {
		t.Errorf("mono of L−R has energy %.4f", e)
	}
	h := mixed(OscX, WaveSquare, 0.5, 1)
	l, r = run(t, h, 4800)
	if got := energy(l) / energy(r); math.Abs(got-0.25) > 1e-3 {
		t.Errorf("half gain on L carries %.3f of R's energy, want 0.25", got)
	}
}

// Two sources on one side are summed: that is what a mixer is for.
func TestTwoSourcesOnOneSideAreSummed(t *testing.T) {
	b := mixed(OscX, WaveSquare, 1, 0)
	b.Gen.SetWave(OscY, WaveSquare)
	b.Gen.SetAmp(OscY, 1)
	b.Mix[0][InGen+OscY] = 1
	sum, _ := run(t, b, 4800)

	x, _ := run(t, mixed(OscX, WaveSquare, 1, 0), 4800)
	y, _ := run(t, mixed(OscY, WaveSquare, 1, 0), 4800)
	for i := range sum {
		if d := sum[i] - (x[i] + y[i]); d > 1e-5 || d < -1e-5 {
			t.Fatalf("sample %d: %.4f, want %.4f + %.4f", i, sum[i], x[i], y[i])
		}
	}
}

// Two generators on white noise are two noises: one on each side is the
// widest image, correlation near 0.
func TestTwoNoisesAreIndependent(t *testing.T) {
	b := mixed(OscX, WaveWhite, 1, 0)
	b.Gen.SetWave(OscY, WaveWhite)
	b.Gen.SetAmp(OscY, 1)
	b.Mix[1][InGen+OscY] = 1
	l, r := run(t, b, 48000)
	var lr, ll, rr float64
	for i := range l {
		lr += float64(l[i]) * float64(r[i])
		ll += float64(l[i]) * float64(l[i])
		rr += float64(r[i]) * float64(r[i])
	}
	if c := lr / math.Sqrt(ll*rr); math.Abs(c) > 0.05 {
		t.Errorf("correlation %.3f, want about 0", c)
	}
}

// Solo lets one generator through and keeps the rest as they were set.
func TestSoloIsOneGenerator(t *testing.T) {
	b := NewBus(NewFuncGen())
	b.Mix[0][InGen+OscX] = 1
	b.Mix[1][InGen+OscY] = 1
	b.Gen.SetSolo(OscY)
	l, r := run(t, b, 4800)
	if energy(l) != 0 || energy(r) == 0 {
		t.Errorf("solo Y: left energy %.1f, right %.1f", energy(l), energy(r))
	}
	b.Gen.SetSolo(-1)
	l, _ = run(t, b, 4800)
	if energy(l) == 0 {
		t.Error("X is still silent after the solo is released")
	}
}

// Off, a generator is silent wherever it is pinned.
func TestAGeneratorSwitchedOffIsSilent(t *testing.T) {
	b := mixed(OscX, WaveSaw, 1, 0)
	b.Gen.SetOn(OscX, false)
	if l, _ := run(t, b, 4800); energy(l) != 0 {
		t.Errorf("off, X has energy %.1f", energy(l))
	}
	b.Gen.SetOn(OscX, true)
	if l, _ := run(t, b, 4800); energy(l) == 0 {
		t.Error("on again, X is silent")
	}
}

// The sweep's position is the generator playing it in the mix, and nothing
// when none is: the waterfall waits for its wrap.
func TestSweepPositionFollowsTheSweepingGenerator(t *testing.T) {
	b := mixed(OscZ, WaveSweep, 1, 1)
	run(t, b, 48000)
	if p := b.SweepPosition(); p < 0.2 || p > 0.3 {
		t.Errorf("a second into a four-second sweep reads %.3f", p)
	}
	b.Mix = [2][InCount]float32{}
	if p := b.SweepPosition(); p != 0 {
		t.Errorf("a sweep out of the mix reads %.3f", p)
	}
}

// The envelope shapes the rack's signal: silent at the start of the attack,
// full at its end, and back to nothing at the end of the decay.
func TestTheEnvelopeShapesTheSignal(t *testing.T) {
	b := mixed(OscX, WaveSquare, 1, 0)
	b.Gen.SetFreq(OscX, 4800) // ten samples a cycle, so a window's peak is its level
	b.Gen.SetEnv(OscX, true, 0.01, 0.01)
	l, _ := run(t, b, 960) // the attack, then the decay: 480 samples each
	peak := func(a, c int) float64 {
		var p float64
		for _, v := range l[a:c] {
			p = math.Max(p, math.Abs(float64(v)))
		}
		return p
	}
	if p := peak(0, 10); p > 0.05 {
		t.Errorf("start of the attack peaks at %.3f", p)
	}
	if p := peak(470, 490); p < 0.95 {
		t.Errorf("top of the envelope peaks at %.3f", p)
	}
	if p := peak(950, 960); p > 0.05 {
		t.Errorf("end of the decay peaks at %.3f", p)
	}
}

// fakeCapture is a capture with n samples waiting, l on the left and r on
// the right, at rate sr (24000 when 0).
type fakeCapture struct {
	n    int
	l, r float32
	sr   int
}

func (c *fakeCapture) DrainStereo(l, r []float32) int {
	n := min(c.n, len(l))
	for k := range n {
		l[k], r[k] = c.l, c.r
	}
	c.n -= n
	return n
}
func (c *fakeCapture) TimeDomainStereo(l, r []float32) {
	for k := range l {
		l[k], r[k] = c.l, c.r
	}
}
func (c *fakeCapture) TimeDomain(d []float32) []float32 { return d }
func (c *fakeCapture) Drain([]float32) int              { return 0 }
func (c *fakeCapture) SampleRate() int {
	if c.sr == 0 {
		return 24000
	}
	return c.sr
}
func (c *fakeCapture) Channels() int { return 2 }
func (c *fakeCapture) Ready() bool   { return true }
func (c *fakeCapture) Err() error    { return nil }
func (c *fakeCapture) Close()        {}

// With the capture in the mix, the capture is the clock: the generator on
// the other side is made to the same count, at the capture's rate.
func TestTheCaptureIsTheClock(t *testing.T) {
	c := &fakeCapture{n: 300, l: 0.5, r: 0.25}
	b := NewBus(NewFuncGen())
	b.Mix[0][InCaptureR] = 1
	b.Mix[1][InGen+OscX] = 1
	b.Capture = func() Source { return c }
	if sr := b.SampleRate(); sr != 24000 {
		t.Errorf("rate %d, want the capture's 24000", sr)
	}
	l, r := make([]float32, 1024), make([]float32, 1024)
	if n := b.DrainStereo(l, r); n != 300 {
		t.Fatalf("drained %d, want the capture's 300", n)
	}
	if l[0] != 0.25 || l[299] != 0.25 {
		t.Errorf("RACK L is the capture's right, 0.25: got %.3f", l[0])
	}
	if energy(r[:300]) == 0 {
		t.Error("Gen X on RACK R is silent")
	}
}

// With the return in the mix and no capture, the return is the clock, and
// what it carries is on the sides it was sent to.
func TestTheReturnIsTheClockWithoutACapture(t *testing.T) {
	ret := &fakeCapture{n: 200, l: 0.5, r: -0.5, sr: 48000}
	b := NewBus(NewFuncGen())
	b.Return = func() Source { return ret }
	b.ReturnOn = true
	l, r := make([]float32, 1024), make([]float32, 1024)
	if n := b.DrainStereo(l, r); n != 200 {
		t.Fatalf("drained %d, want the return's 200", n)
	}
	if l[0] != 0.5 || r[199] != -0.5 {
		t.Errorf("the return reads %.2f / %.2f, want 0.5 / −0.5", l[0], r[199])
	}
}

// Beside a capture at half the return's rate, the return is resampled to
// the capture's: twice as many of its samples go into each block.
func TestTheReturnFollowsTheCapturesRate(t *testing.T) {
	c := &fakeCapture{n: 100}
	ret := &fakeCapture{n: 1000, l: 0.25, r: 0.25, sr: 48000}
	b := NewBus(NewFuncGen())
	b.Mix[0][InCaptureL] = 1
	b.Capture = func() Source { return c }
	b.Return = func() Source { return ret }
	b.ReturnOn = true
	l, r := make([]float32, 512), make([]float32, 512)
	if n := b.DrainStereo(l, r); n != 100 {
		t.Fatalf("drained %d, want the capture's 100", n)
	}
	if ret.n != 800 {
		t.Errorf("the return gave %d samples for 100 at half its rate, want 200", 1000-ret.n)
	}
	if l[50] != 0.25 {
		t.Errorf("the return reads %.3f in the mix, want 0.25", l[50])
	}
}

// The capture alone in the mix, not yet delivering, is not ready; anything
// else in the mix is.
func TestReadyWaitsOnlyForACaptureAlone(t *testing.T) {
	b := NewBus(NewFuncGen())
	b.Mix[0][InCaptureL], b.Mix[1][InCaptureR] = 1, 1
	b.Capture = func() Source { return nil }
	if b.Ready() {
		t.Error("ready with only an absent capture in the mix")
	}
	b.Mix[1][InGen+OscY] = 1
	if !b.Ready() {
		t.Error("not ready with a generator in the mix")
	}
}

// A window draws the mix without moving it on: the generators are drawn
// from their shadows. The capture is in it whether the mix has it or not.
func TestAWindowLeavesTheMixAlone(t *testing.T) {
	b := mixed(OscX, WaveSine, 1, 0)
	b.Capture = func() Source { return &fakeCapture{l: 0.5, r: 0.25} }
	before := b.Gen.osc[OscX].phase
	w := b.Window(512)
	if energy(w.L) == 0 {
		t.Error("the window on RACK L is silent")
	}
	if b.Gen.osc[OscX].phase != before {
		t.Error("drawing the window moved Gen X")
	}
	if w.CapL[0] != 0.5 || w.CapR[0] != 0.25 {
		t.Errorf("the capture out of the mix reads %.2f / %.2f in the window", w.CapL[0], w.CapR[0])
	}
}

// The model's three outputs come from one call that moves all three, and
// each lands where its pin puts it: z on L and x on R here.
func TestTheModelsOutputsAreWhereTheirPinsAre(t *testing.T) {
	b := NewBus(NewFuncGen())
	b.Mix[0][InModelZ] = 1
	b.Mix[1][InModelX] = 1
	calls := 0
	b.Model = func(dst *[3][]float32, n int, advance bool) {
		calls++
		for c := range dst {
			for k := range n {
				dst[c][k] = float32(c + 1) // x is 1, y 2, z 3
			}
		}
	}
	l, r := run(t, b, 64)
	if l[0] != 3 || r[0] != 1 {
		t.Errorf("RACK L reads %.0f (want z, 3), RACK R %.0f (want x, 1)", l[0], r[0])
	}
	if calls != 1 {
		t.Errorf("the model was run %d times for one block", calls)
	}
}

// The sends are a mix of their own: a generator on MOD A and on nothing
// else is in MOD A's window, not in the rack's signal, and reading the sends
// moves nothing on.
func TestTheModSendsAreAMixOfTheirOwn(t *testing.T) {
	b := mixed(OscY, WaveSine, 0, 0)
	b.ModMix[0][InGen+OscY] = 1
	before := b.Gen.osc[OscY].phase
	a, c := b.ModWindow(512)
	if energy(a) == 0 || energy(c) != 0 {
		t.Errorf("Gen Y on MOD A: A energy %.1f, B %.1f", energy(a), energy(c))
	}
	if b.Gen.osc[OscY].phase != before {
		t.Error("reading the sends moved Gen Y")
	}
	if !b.ModOn() {
		t.Error("a pin on MOD A, and the sends read as empty")
	}
	if l, r := run(t, b, 512); energy(l)+energy(r) != 0 {
		t.Error("Gen Y on MOD A alone is in the rack's signal")
	}
}
