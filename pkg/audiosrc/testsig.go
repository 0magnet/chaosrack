package audiosrc

import "math"

// Test signals — the known stimuli a measurement needs.
//
// Every analyzer in this app compares what came back against what went out, and
// until now there was nothing defined to send: the generator has three
// oscillators, which is a fine instrument and not a reference. A distortion
// figure means nothing without a clean tone to make it from, an RTA means
// nothing without pink noise, an impulse response needs a sweep, and "which
// speaker is the left one" needs a signal that is only in one channel.
//
// These are the signals a test record or a measurement CD carries, synthesized
// rather than played back, so they are exact and always to hand.
//
// Untagged and free of syscall/js on purpose: this is arithmetic, and
// arithmetic that a native test can check against its own definition is worth
// far more than arithmetic only a browser can run. The spectra, the sweep's
// instantaneous frequency and the correlation of each stereo pair are all
// pinned in testsig_test.go.

// TestSignal names one stimulus. TestOff is the zero value and means the three
// oscillators, so a generator that has never been told otherwise behaves
// exactly as it always did.
type TestSignal int

const (
	TestOff TestSignal = iota
	TestWhite
	TestPink
	TestSweep
	TestTone1k
	TestTone3150
	TestLeftOnly
	TestRightOnly
	TestPolarity
	TestWide
	TestOutOfPhase
	testSignalCount
)

// TestSignalNames are the positions of the selector, in knob order, and
// TestSignalRing what fits around such a dial.
//
// Ordered by what they are FOR rather than alphabetically: the two noises that
// feed a spectrum analyzer, the sweep that feeds an impulse response, the two
// reference tones, then the three stereo checks and the two correlation
// extremes. A dial is read in order, so the order is a grouping.
var TestSignalNames = []string{
	"off",
	"white noise",
	"pink noise",
	"log sweep 20 Hz–20 kHz",
	"1 kHz reference",
	"3150 Hz (wow & flutter)",
	"left channel only",
	"right channel only",
	"polarity pulse",
	"uncorrelated noise (wide)",
	"out-of-polarity noise",
}

// TestSignalRing is five runes at most per position: for a named setting the
// ring IS the readout, because seven segments cannot spell a word.
var TestSignalRing = []string{
	"off", "whit", "pink", "swp", "1k", "3150", "L", "R", "pol", "wide", "oop",
}

// TestSignalCount is how many positions the selector has.
const TestSignalCount = int(testSignalCount)

// sweepSeconds is one pass of the log sweep, low to high, before it restarts.
//
// Four seconds is long enough that the sweep spends a usable number of cycles
// in the bass — a 20 Hz tone is 50 ms per cycle, and a sweep that crosses the
// bottom octave in a tenth of a second has excited it with two cycles and
// measured nothing — and short enough that a repeating stimulus is a display
// that moves rather than one you wait for.
const sweepSeconds = 4.0

// sweepLo and sweepHi bound the sweep: the audible band, near enough.
const (
	sweepLo = 20.0
	sweepHi = 20000.0
)

// polarityHz is the repetition rate of the polarity pulse — slow enough that
// the individual pulses are separate events on a scope and on a loudspeaker
// cone, rather than a tone with a timbre.
const polarityHz = 8.0

// testGen holds the state the stimuli carry between buffers: the noise
// generator, the pink filter's poles, and the phases.
//
// The noise is a deterministic xorshift rather than the runtime's random
// source, which is what lets a test assert a spectrum: the same seed is the
// same noise, so "is this pink" is a question with a repeatable answer. Two
// independent generators, because the uncorrelated-noise position needs two
// streams that really are independent — running one generator twice per sample
// and calling the halves L and R correlates them through the shared state.
type testGen struct {
	sig   TestSignal
	level float64

	rngL, rngR uint32
	pinkL      pinkState
	pinkR      pinkState

	phase float64 // oscillator phase for the fixed tones, radians
	sweep float64 // position through one sweep pass, 0..1
	pol   float64 // polarity pulse phase, 0..1
}

func newTestGen() *testGen {
	return &testGen{level: 0.5, rngL: 0x12345678, rngR: 0x9E3779B9}
}

// next returns one stereo sample of the current stimulus.
func (g *testGen) next(sr float64) (float32, float32) {
	if sr <= 0 {
		sr = 48000
	}
	a := g.level
	switch g.sig {
	case TestWhite:
		// The SAME noise in both channels: correlated, so a goniometer draws
		// the diagonal and a correlation meter reads +1. The uncorrelated case
		// is its own position, because the two are different tests and telling
		// them apart is most of what a phase display is for.
		v := a * g.whiteL()
		return float32(v), float32(v)

	case TestPink:
		v := a * g.pinkL.next(g.whiteL())
		return float32(v), float32(v)

	case TestWide:
		// Two independent streams: correlation 0, and a round cloud on the
		// goniometer. This is what "wide" looks like at its limit.
		return float32(a * g.whiteL()), float32(a * g.whiteR())

	case TestOutOfPhase:
		// One stream, one channel inverted: correlation −1, the figure on the
		// other diagonal, and NOTHING LEFT when the two are summed. That last
		// is the point — it is the mono-compatibility failure, made on purpose
		// so the displays can be seen reporting it.
		v := a * g.whiteL()
		return float32(v), float32(-v)

	case TestSweep:
		v := a * g.nextSweep(sr)
		return float32(v), float32(v)

	case TestTone1k:
		v := a * g.nextTone(sr, 1000)
		return float32(v), float32(v)

	case TestTone3150:
		// 3150 Hz is the wow-and-flutter tone: it is what the test records
		// carry (DIN 45507 / IEC 60386), because speed error shows as
		// frequency modulation of a known carrier and 3150 Hz sits where the
		// ear and the meter are both sensitive.
		v := a * g.nextTone(sr, 3150)
		return float32(v), float32(v)

	case TestLeftOnly:
		return float32(a * g.nextTone(sr, 1000)), 0

	case TestRightOnly:
		return 0, float32(a * g.nextTone(sr, 1000))

	case TestPolarity:
		v := a * g.nextPolarity(sr)
		return float32(v), float32(v)
	}
	return 0, 0
}

// whiteL and whiteR are the two independent noise streams, in ±1.
//
// xorshift32, and the top bits are the ones used: the low bit of a xorshift has
// a short period of its own, which on a spectrum shows as a tone rather than as
// noise.
func (g *testGen) whiteL() float64 { g.rngL = xorshift(g.rngL); return noiseOf(g.rngL) }
func (g *testGen) whiteR() float64 { g.rngR = xorshift(g.rngR); return noiseOf(g.rngR) }

func xorshift(x uint32) uint32 {
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	return x
}

// noiseOf maps a xorshift word to a sample in ±1.
//
// int32(x) >> 8, not int32(x >> 8): the shift has to happen on the SIGNED value
// so that it sign-extends and the result straddles zero. Shifting the unsigned
// word first throws the sign bit away and leaves 0..2^24, which through the
// same divisor is "noise" running from 0 to 2 — a signal with a large DC
// offset and half the intended swing. It sounds like noise and it is not one:
// every correlation, every spectrum and every level built on it would be wrong
// by that offset.
func noiseOf(x uint32) float64 { return float64(int32(x)>>8) / (1 << 23) }

// pinkState is Paul Kellet's refined pink-noise filter: six one-pole sections
// summed, which tracks a −3 dB/octave slope to about ±0.05 dB over ten octaves.
//
// Pink rather than white because a spectrum analyzer's bands are FRACTIONAL
// octaves — each band is wider than the one below it in Hz, so white noise
// (equal power per Hz) reads as a rising slope and tells you nothing about the
// room. Pink is equal power per octave, so a flat system reads flat, which is
// the whole convention room measurement is done in.
type pinkState struct{ b0, b1, b2, b3, b4, b5, b6 float64 }

func (p *pinkState) next(white float64) float64 {
	p.b0 = 0.99886*p.b0 + white*0.0555179
	p.b1 = 0.99332*p.b1 + white*0.0750759
	p.b2 = 0.96900*p.b2 + white*0.1538520
	p.b3 = 0.86650*p.b3 + white*0.3104856
	p.b4 = 0.55000*p.b4 + white*0.5329522
	p.b5 = -0.7616*p.b5 - white*0.0168980
	out := p.b0 + p.b1 + p.b2 + p.b3 + p.b4 + p.b5 + p.b6 + white*0.5362
	p.b6 = white * 0.115926
	// The sum runs a little over unity on peaks; 0.11 brings the RMS to about
	// the same place the other stimuli sit so that switching between them is
	// not a jump in level.
	return out * 0.11
}

// nextTone advances a plain sine at a fixed frequency.
func (g *testGen) nextTone(sr, hz float64) float64 {
	v := math.Sin(g.phase)
	g.phase += 2 * math.Pi * hz / sr
	if g.phase > 2*math.Pi {
		g.phase -= 2 * math.Pi
	}
	return v
}

// nextSweep advances the logarithmic sine sweep.
//
// LOGARITHMIC, not linear: the sweep spends equal time per OCTAVE, so the
// bottom of the band gets as many cycles as the top. A linear sweep crosses
// 20–40 Hz in a thousandth of the time it spends on 10–20 kHz, which is why
// the log sweep is what impulse-response measurement uses — and it has the
// second property that makes it the standard, which is that the harmonic
// distortion it provokes lands BEFORE the linear response in the deconvolved
// impulse, where it can be windowed away instead of contaminating the answer.
//
// The instantaneous frequency is f(t) = lo·(hi/lo)^t for t in 0..1, and the
// phase is its integral: φ(t) = 2π·lo·T·((hi/lo)^t − 1)/ln(hi/lo).
func (g *testGen) nextSweep(sr float64) float64 {
	k := math.Log(sweepHi / sweepLo)
	phase := 2 * math.Pi * sweepLo * sweepSeconds * (math.Exp(g.sweep*k) - 1) / k
	g.sweep += 1 / (sweepSeconds * sr)
	if g.sweep >= 1 {
		g.sweep -= 1
	}
	return math.Sin(phase)
}

// SweepPosition reports how far through one sweep pass the generator is, 0..1.
// A measurement that wants to know which frequency it is looking at, or when a
// pass has come round, asks here rather than counting samples of its own.
func (g *testGen) SweepPosition() float64 { return g.sweep }

// sweepFreqAt is the sweep's instantaneous frequency at a position, in Hz —
// the definition the phase above integrates, kept beside it so the two cannot
// drift apart, and what the test checks the synthesized signal against.
func sweepFreqAt(pos float64) float64 {
	return sweepLo * math.Pow(sweepHi/sweepLo, pos)
}

// nextPolarity advances the polarity test pulse.
//
// ASYMMETRIC ON PURPOSE, and that is the whole signal: a tone tells you nothing
// about polarity because it is the same shape upside down. This is a short
// positive spike followed by a long shallow negative recovery, so on a scope
// the spike unmistakably points one way and on a loudspeaker the cone's first
// movement is outward. Invert the cable and both reverse.
//
// The two parts have equal area, so the signal carries no DC — a pulse train
// with a net offset drives a woofer off center and measures the amplifier's
// coupling rather than the speaker's polarity.
func (g *testGen) nextPolarity(sr float64) float64 {
	const spike = 0.1 // the fraction of the period the positive half occupies
	p := g.pol
	g.pol += polarityHz / sr
	if g.pol >= 1 {
		g.pol -= 1
	}
	if p < spike {
		// A half-sine rather than a square edge: a step has energy above
		// Nyquist, and what comes back from that is the reconstruction filter
		// ringing rather than the thing being measured.
		return math.Sin(math.Pi * p / spike)
	}
	// The recovery, scaled so its area cancels the spike's exactly. A half-sine
	// of height 1 over a width w has area 2w/π; spreading that over (1−w) needs
	// a height of 2w/(π(1−w)).
	const area = 2 * spike / math.Pi
	return -area / (1 - spike) * math.Pi / 2 * math.Sin(math.Pi*(p-spike)/(1-spike))
}

// ── Using the stimuli without a browser ──────────────────────────────────

// TestSource synthesizes one stimulus, and is the way to reach the library
// from anywhere that is not a live page.
//
// FuncGen is the generator the app plays through, and it is js-tagged because
// it is a Source — so without this the stimuli could only be produced inside a
// browser, and anything wanting to check a measurement against a known signal
// natively would have to write its own copy of that signal. A second
// implementation written to agree is not a check; it is the same assumption
// stated twice. This lets a test feed the real pink noise to the real analyzer
// and find out whether the two halves of the app agree.
type TestSource struct {
	g  *testGen
	sr int
}

// NewTestSource returns a generator for one stimulus at a sample rate.
func NewTestSource(sig TestSignal, sampleRate int) *TestSource {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	g := newTestGen()
	if sig >= 0 && sig < testSignalCount {
		g.sig = sig
	}
	g.level = 1
	return &TestSource{g: g, sr: sampleRate}
}

// SetLevel sets the amplitude, 0..1.
func (t *TestSource) SetLevel(v float64) {
	if !(v > 0) {
		v = 0
	} else if v > 1 {
		v = 1
	}
	t.g.level = v
}

// Fill writes the next samples of both channels. len(l) must equal len(r).
func (t *TestSource) Fill(l, r []float32) {
	if len(l) != len(r) {
		panic("audiosrc: TestSource.Fill requires len(l) == len(r)")
	}
	sr := float64(t.sr)
	for i := range l {
		l[i], r[i] = t.g.next(sr)
	}
}

// FillMono writes the next samples of the stimulus folded to one channel —
// the mix, which for the out-of-polarity stimulus is correctly silence.
func (t *TestSource) FillMono(dst []float32) {
	sr := float64(t.sr)
	for i := range dst {
		a, b := t.g.next(sr)
		dst[i] = (a + b) * 0.5
	}
}

// SweepPosition reports how far through one pass of the log sweep it is, 0..1.
func (t *TestSource) SweepPosition() float64 { return t.g.SweepPosition() }
