//go:build js && wasm

package audiosrc

import "math"

// FuncGen is a client-side function/signal generator: three independent
// oscillators (X, Y, Z), each with its own waveform, frequency and amplitude.
// It implements Source with no server, mic or network, so audio-reactive
// features, the spectrogram and the xy scope all work on a static build
// (GitHub Pages) with nothing streaming in.
//
//   - xy scope: X plots oscillator X, Y plots oscillator Y — a real two-signal
//     Lissajous from two independent generators.
//   - features / spectrogram: the mono mix of the enabled oscillators.
//
// Audio playback over the speakers is handled separately (Web Audio oscillators
// in the attractor package) driven from the same per-oscillator parameters.
type FuncGen struct {
	sr  int
	osc [3]osc // X, Y, Z
}

type osc struct {
	wave  int     // 0 sine, 1 triangle, 2 square, 3 saw, 4 noise (LFSR)
	freq  float64 // Hz
	amp   float64 // 0..1
	phase float64 // running phase (radians)
	lfsr  uint32  // 15-bit shift register for the noise wave (SN76477-style)
	nz    float64 // held noise output between shift clocks
}

// Oscillator indices.
const (
	OscX = 0
	OscY = 1
	OscZ = 2
)

// NewFuncGen returns a generator whose X/Y oscillators are in a 2:3 ratio so the
// xy scope shows an interesting Lissajous by default; Z is a third tone.
func NewFuncGen() *FuncGen {
	f := &FuncGen{sr: 48000}
	f.osc[OscX] = osc{wave: 0, freq: 196.00, amp: 0.8} // G3
	f.osc[OscY] = osc{wave: 0, freq: 293.66, amp: 0.8} // D4 (a fifth above X)
	f.osc[OscZ] = osc{wave: 0, freq: 146.83, amp: 0.8} // D3
	return f
}

func (f *FuncGen) SetWave(i, w int)          { f.osc[i].wave = w }
func (f *FuncGen) SetFreq(i int, hz float64) { f.osc[i].freq = hz }
func (f *FuncGen) SetAmp(i int, a float64)   { f.osc[i].amp = a }
func (f *FuncGen) Wave(i int) int            { return f.osc[i].wave }
func (f *FuncGen) Freq(i int) float64        { return f.osc[i].freq }
func (f *FuncGen) Amp(i int) float64         { return f.osc[i].amp }

func (f *FuncGen) SampleRate() int { return f.sr }
func (f *FuncGen) Channels() int   { return 2 }
func (f *FuncGen) Ready() bool     { return true }
func (f *FuncGen) Err() error      { return nil }
func (f *FuncGen) Close()          {}

func waveform(kind int, ph float64) float64 {
	ph = math.Mod(ph, 2*math.Pi)
	if ph < 0 {
		ph += 2 * math.Pi
	}
	switch kind {
	case 1: // triangle
		return 2 / math.Pi * math.Asin(math.Sin(ph))
	case 2: // square
		if math.Sin(ph) >= 0 {
			return 1
		}
		return -1
	case 3: // saw
		return ph/math.Pi - 1
	default: // sine
		return math.Sin(ph)
	}
}

// advance runs oscillator i forward one sample and returns its value.
func (f *FuncGen) advance(i int) float64 {
	o := &f.osc[i]
	if o.wave == 4 {
		// Shift-register noise, the Complex Sound Generator way: a 15-bit
		// LFSR clocked at 32× the frequency knob and HELD between clocks, so
		// the knob audibly tunes the noise from rumble to hiss.
		if o.lfsr == 0 {
			o.lfsr = 0x4001
		}
		o.phase += 2 * math.Pi * o.freq * 32 / float64(f.sr)
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
	o.phase += 2 * math.Pi * o.freq / float64(f.sr)
	if o.phase > 2*math.Pi {
		o.phase -= 2 * math.Pi
	}
	return v
}

// TimeDomainStereo: X oscillator → left, Y oscillator → right (the xy scope's
// two axes).
func (f *FuncGen) TimeDomainStereo(l, r []float32) {
	for i := range l {
		xv := f.advance(OscX)
		yv := f.advance(OscY)
		f.advance(OscZ) // keep Z phase moving so it's coherent when routed to audio
		l[i] = float32(xv)
		r[i] = float32(yv)
	}
}

// TimeDomain / Drain: mono mix of the three oscillators (features, spectrogram).
func (f *FuncGen) fillMono(dst []float32) {
	for i := range dst {
		m := (f.advance(OscX) + f.advance(OscY) + f.advance(OscZ)) / 3
		dst[i] = float32(m)
	}
}

func (f *FuncGen) TimeDomain(dst []float32) []float32 {
	f.fillMono(dst)
	return dst
}

func (f *FuncGen) Drain(dst []float32) int {
	f.fillMono(dst)
	return len(dst)
}
