//go:build js && wasm

package audiosrc

import "math"

// FuncGen is the rack's four signal generators, made in the tab: four
// independent oscillators (X, Y, Z, V: Gen 1 to 4 on the panel), each with
// its own waveform, frequency, amplitude and envelope. They work with no
// server, mic or network, so the scopes, the audio-reactive features and the
// spectrogram all have a signal on a static build (GitHub Pages) with nothing
// streaming in.
//
// Where each one goes is not its business: the Bus patches them to the rack's
// channels. Nor is whether it is heard: the speakers are a separate player
// (Web Audio, in the attractor package) driven from the same per-oscillator
// parameters.
type FuncGen struct {
	sr     int
	osc    [OscCount]osc // X, Y, Z, V
	shadow [OscCount]osc // each drawn alone for a scope (RenderOsc), its own phase
	solo   int           // the one oscillator heard, or -1 for all of them
}

type osc struct {
	wave  int     // a Wave* constant
	freq  float64 // Hz
	amp   float64 // 0..1
	off   bool    // switched off (SetOn): silent, and still keeping time
	phase float64 // running phase (radians)
	lfsr  uint32  // 15-bit shift register for the noise wave (SN76477-style)
	nz    float64 // held noise output between shift clocks
	stim  *testGen

	// Its envelope (EnvAt): on, the level repeats a rise over atk and a
	// fall over dcy seconds; envT is how far into it the oscillator is.
	envOn    bool
	atk, dcy float64
	envT     float64
}

// Oscillator indices.
const (
	OscX = 0
	OscY = 1
	OscZ = 2
	OscV = 3

	// OscCount is how many oscillators the generator has.
	OscCount = 4
)

// NewFuncGen returns the generators X and Y in a 2:3 ratio, so the two drawn
// against each other are a Lissajous figure, and Z a third tone.
func NewFuncGen() *FuncGen {
	f := &FuncGen{sr: 48000, solo: -1}
	f.osc[OscX] = osc{freq: 196.00, amp: 0.8} // G3
	f.osc[OscY] = osc{freq: 293.66, amp: 0.8} // D4 (a fifth above X)
	f.osc[OscZ] = osc{freq: 146.83, amp: 0.8} // D3
	// V starts silent: another tone in the mix by default would change
	// the spectrogram and every audio feature of a rack nobody has touched.
	f.osc[OscV] = osc{freq: 246.94} // B3
	for i := range f.osc {
		// Seeded apart, so two generators on noise are two noises.
		f.osc[i].stim = newStimGen(0x12345678 + uint32(i)*0x9E3779B9) //nolint:gosec // i < OscCount
	}
	return f
}

// newStimGen is one generator's stimulus source, its noise seeded apart from
// every other generator's: two generators playing noise are then independent
// streams, which is what the widest image (one on L, one on R) needs.
func newStimGen(seed uint32) *testGen {
	g := newTestGen()
	g.rngL = seed
	return g
}

// SetWave sets oscillator i's waveform.
func (f *FuncGen) SetWave(i, w int) { f.osc[i].wave = w }

// SetFreq sets oscillator i's frequency in Hz.
func (f *FuncGen) SetFreq(i int, hz float64) { f.osc[i].freq = hz }

// SetAmp sets oscillator i's amplitude.
func (f *FuncGen) SetAmp(i int, a float64) { f.osc[i].amp = a }

// SetOn switches oscillator i on or off: off, it plays silence, and keeps
// time, so it is in phase the moment it is switched back on.
func (f *FuncGen) SetOn(i int, on bool) { f.osc[i].off = !on }

// SetSampleRate sets the rate the generator synthesizes at.
func (f *FuncGen) SetSampleRate(sr int) { f.sr = sr }

// SetEnv sets oscillator i's envelope: on or off, and its attack and decay
// in seconds.
func (f *FuncGen) SetEnv(i int, on bool, atk, dcy float64) {
	o := &f.osc[i]
	o.envOn, o.atk, o.dcy = on, atk, dcy
}

// SetSolo makes oscillator i the only one let through, or with −1 lets them
// all through again.
func (f *FuncGen) SetSolo(i int) {
	if i < 0 || i >= OscCount {
		i = -1
	}
	f.solo = i
}

// Solo is the soloed oscillator, or −1.
func (f *FuncGen) Solo() int { return f.solo }

// Audible reports whether oscillator i is let through by the solo.
func (f *FuncGen) Audible(i int) bool { return f.solo < 0 || f.solo == i }

// Wave is oscillator i's waveform.
func (f *FuncGen) Wave(i int) int { return f.osc[i].wave }

// Freq is oscillator i's frequency in Hz.
func (f *FuncGen) Freq(i int) float64 { return f.osc[i].freq }

// Amp is oscillator i's amplitude.
func (f *FuncGen) Amp(i int) float64 { return f.osc[i].amp }

// SampleRate is the rate the generator synthesizes at.
func (f *FuncGen) SampleRate() int { return f.sr }

func waveform(kind int, ph float64) float64 {
	ph = math.Mod(ph, 2*math.Pi)
	if ph < 0 {
		ph += 2 * math.Pi
	}
	switch kind {
	case WaveTriangle:
		return 2 / math.Pi * math.Asin(math.Sin(ph))
	case WaveSquare:
		if math.Sin(ph) >= 0 {
			return 1
		}
		return -1
	case WaveSaw:
		return ph/math.Pi - 1
	default: // sine
		return math.Sin(ph)
	}
}

// step runs the oscillator forward one sample at rate sr, envelope and all,
// and returns its value.
func (o *osc) step(sr float64) float64 {
	v := o.raw(sr)
	if o.off {
		v = 0
	}
	if o.envOn {
		v *= EnvAt(o.envT, o.atk, o.dcy)
		o.envT += 1 / sr
		if p := o.atk + o.dcy; p > 0 && o.envT >= p {
			o.envT -= p
		}
	}
	return v
}

// raw is the oscillator's next sample before its envelope.
func (o *osc) raw(sr float64) float64 {
	if sig, ok := WaveStim(o.wave); ok {
		// A stimulus runs at its own rate; the frequency knob is not its pitch.
		return o.stim.stimMono(sig, sr) * o.amp
	}
	if o.wave == WaveLFSR {
		// Shift-register noise, the Complex Sound Generator way: a 15-bit
		// LFSR clocked at 32× the frequency knob and HELD between clocks, so
		// the knob audibly tunes the noise from rumble to hiss.
		if o.lfsr == 0 {
			o.lfsr = 0x4001
		}
		o.phase += 2 * math.Pi * o.freq * 32 / sr
		for o.phase >= 2*math.Pi {
			o.phase -= 2 * math.Pi
			bit := (o.lfsr ^ (o.lfsr >> 1)) & 1
			o.lfsr = (o.lfsr >> 1) | (bit << 14)
			if o.lfsr&1 == 1 {
				o.nz = 1
			} else {
				o.nz = -1
			}
		}
		return o.nz * o.amp
	}
	v := waveform(o.wave, o.phase) * o.amp
	o.phase += 2 * math.Pi * o.freq / sr
	if o.phase > 2*math.Pi {
		o.phase -= 2 * math.Pi
	}
	return v
}

// RenderOsc fills dst with oscillator i alone, as its own knobs set it and
// before its out ring: what a probe on that generator's output would show,
// routed anywhere or nowhere. It is drawn from a shadow of the oscillator
// with its own phase, so a scope watching a generator does not move the one
// the rack's signal is made of.
func (f *FuncGen) RenderOsc(i int, dst []float32) {
	o, s := &f.osc[i], &f.shadow[i]
	if s.stim == nil {
		s.stim = newStimGen(0x2545F491 + uint32(i)*0x9E3779B9) //nolint:gosec // i < OscCount
	}
	if s.wave != o.wave {
		s.phase, s.envT = 0, 0
	}
	s.wave, s.freq, s.amp, s.off = o.wave, o.freq, o.amp, o.off
	s.envOn, s.atk, s.dcy = o.envOn, o.atk, o.dcy
	sr := float64(f.sr)
	for k := range dst {
		dst[k] = float32(s.step(sr))
	}
}
