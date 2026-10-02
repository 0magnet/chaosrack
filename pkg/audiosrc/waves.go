package audiosrc

import "math"

// The generator's waveforms, in the order of the waveform ring.
//
// The first five are oscillators: the frequency knob is their pitch (the
// shift-register noise's clock). The last four are the stimuli a test record
// carries (testsig.go), played by a generator like any other wave, so a
// measurement's signal is set up on the generators, routed and leveled like
// the rest, rather than in a module of its own that replaced them all.
const (
	WaveSine = iota
	WaveTriangle
	WaveSquare
	WaveSaw
	WaveLFSR // 15-bit shift-register noise, clocked from the frequency knob
	WaveWhite
	WavePink
	WaveSweep // log sweep, 20 Hz–20 kHz in sweepSeconds
	WavePulse // the polarity pulse, polarityHz times a second
	WaveCount
)

// WaveStim is the stimulus a wave plays, for the four that are one.
func WaveStim(w int) (TestSignal, bool) {
	switch w {
	case WaveWhite:
		return TestWhite, true
	case WavePink:
		return TestPink, true
	case WaveSweep:
		return TestSweep, true
	case WavePulse:
		return TestPolarity, true
	}
	return TestOff, false
}

// stimNoiseSeconds is how long a loop of noise is for a player that repeats
// one (StimLoop). Long enough that the repeat is not heard as a rhythm.
const stimNoiseSeconds = 6.0

// StimLoop is one loop of a stimulus wave, for a player that repeats a buffer:
// the speakers play the generators through Web Audio, and this is how they get
// the same signal the rack's own synthesis makes. The sweep and the pulse
// loop on their own period, the noises on stimNoiseSeconds. Nil for a wave
// that is not a stimulus.
func StimLoop(w, sampleRate int) []float32 {
	sig, ok := WaveStim(w)
	if !ok {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	sr := float64(sampleRate)
	n := int(sr * stimNoiseSeconds)
	switch sig {
	case TestSweep:
		n = int(math.Round(sweepSeconds * sr))
	case TestPolarity:
		n = int(math.Round(sr / polarityHz))
	}
	g := newTestGen()
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(g.stimMono(sig, sr))
	}
	return out
}

// EnvAt is a generator's repeating envelope, t seconds into it: a linear rise
// over atk seconds and a linear fall over dcy, over and over, 0..1. The rack's
// signal (FuncGen) steps it by samples and the speakers by the audio clock,
// from this one definition.
func EnvAt(t, atk, dcy float64) float64 {
	atk, dcy = math.Max(atk, 0.001), math.Max(dcy, 0.001)
	p := math.Mod(t, atk+dcy)
	if p < 0 {
		p += atk + dcy
	}
	if p < atk {
		return p / atk
	}
	return 1 - (p-atk)/dcy
}
