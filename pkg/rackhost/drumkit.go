package rackhost

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// The Matrix's voices: a pad's ping and the four drums, each what the page
// builds of Web Audio nodes for one hit (tonematrix_js.go's scheduleCol,
// rhythm_js.go's voice), made here a sample at a time. A voice is a note, as
// a node is there: made for the hit, and gone when it stops.

// noise is the page's noise buffer: two seconds of a 15-bit shift register
// clocked once a sample, the Complex Sound Generator's noise. A drum plays it
// from the top every hit, so every snare is the same snare, as the page's is.
var noise = func() []float32 {
	out := make([]float32, 2*sampleRate)
	lfsr := uint32(0x4001)
	for i := range out {
		bit := (lfsr ^ (lfsr >> 1)) & 1
		lfsr = (lfsr >> 1) | (bit << 14)
		out[i] = -1
		if lfsr&1 == 1 {
			out[i] = 1
		}
	}
	return out
}()

// noiseAt is the noise buffer played from the top at rate buffer samples a
// sample, n samples in: looped, and read between its samples linearly, as a
// buffer source plays at a rate other than one.
func noiseAt(n int, rate float64) float64 {
	p := float64(n) * rate
	i := int(p)
	f := p - float64(i)
	a, b := noise[i%len(noise)], noise[(i+1)%len(noise)]
	return float64(a) + (float64(b)-float64(a))*f
}

// biquad is a Web Audio biquad filter node, its coefficients as the spec's
// cookbook gives them.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

// newHighpass is a highpass at its default Q (1 dB): the filter the page
// puts between the noise and a snare, a hat or a cymbal.
func newHighpass(hz float64) biquad {
	w := 2 * math.Pi * hz / sampleRate
	cw, alpha := math.Cos(w), math.Sin(w)/(2*math.Pow(10, 1.0/20))
	a0 := 1 + alpha
	return biquad{
		b0: (1 + cw) / 2 / a0, b1: -(1 + cw) / a0, b2: (1 + cw) / 2 / a0,
		a1: -2 * cw / a0, a2: (1 - alpha) / a0,
	}
}

// lowpass tunes h to a lowpass at hz, Q in decibels, as a node's frequency
// moves under a sound: the coefficients change, what the filter holds stays.
func (h *biquad) lowpass(hz, qdB float64) {
	w := 2 * math.Pi * hz / sampleRate
	cw, alpha := math.Cos(w), math.Sin(w)/(2*math.Pow(10, qdB/20))
	a0 := 1 + alpha
	h.b0, h.b1, h.b2 = (1-cw)/2/a0, (1-cw)/a0, (1-cw)/2/a0
	h.a1, h.a2 = -2*cw/a0, (1-alpha)/a0
}

// newBandpass is a bandpass at hz, of quality q (a plain ratio, not
// decibels, for this type).
func newBandpass(hz, q float64) biquad {
	w := 2 * math.Pi * hz / sampleRate
	cw, alpha := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + alpha
	return biquad{b0: alpha / a0, b2: -alpha / a0, a1: -2 * cw / a0, a2: (1 - alpha) / a0}
}

func (h *biquad) next(x float64) float64 {
	y := h.b0*x + h.b1*h.x1 + h.b2*h.x2 - h.a1*h.y1 - h.a2*h.y2
	h.x2, h.x1, h.y2, h.y1 = h.x1, x, h.y1, y
	return y
}

// expRamp is a Web Audio exponential ramp from v0 at t0 to v1 at t1, held at
// v1 after it.
func expRamp(t, t0, v0, t1, v1 float64) float64 {
	switch {
	case t <= t0:
		return v0
	case t >= t1:
		return v1
	}
	return v0 * math.Pow(v1/v0, (t-t0)/(t1-t0))
}

// polyBLEP takes the step out of a waveform's jump at phase t (0..1) for a
// phase increment dt: an oscillator node's waves are band-limited, and a
// naive saw at the top of the grid is mostly alias.
func polyBLEP(t, dt float64) float64 {
	switch {
	case t < dt:
		t /= dt
		return t + t - t*t - 1
	case t > 1-dt:
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

// osc is an oscillator node's wave at phase ph (0..1) and increment dt:
// sine, triangle, square or sawtooth (audiosrc's numbering).
func osc(wave int, ph, dt float64) float64 {
	switch wave {
	case audiosrc.WaveTriangle:
		return 2 / math.Pi * math.Asin(math.Sin(2*math.Pi*ph))
	case audiosrc.WaveSquare:
		v := 1.0
		if ph >= 0.5 {
			v = -1
		}
		return v + polyBLEP(ph, dt) - polyBLEP(math.Mod(ph+0.5, 1), dt)
	case audiosrc.WaveSaw:
		return 2*ph - 1 - polyBLEP(ph, dt)
	}
	return math.Sin(2 * math.Pi * ph)
}

// Which of the Matrix's outputs a voice is heard on.
const (
	busTM = iota // the pads, the Mixer's TM
	busDR        // the drums, its DR
)

// voice is one hit sounding: a pad's ping or a drum.
type voice struct {
	drum int     // rhythm.Bass … rhythm.Cymbal, or -1 for a ping
	wait int     // samples before it starts
	n    int     // samples since it started
	wave int     // a ping's wave
	hz   float64 // a ping's pitch
	ring float64 // a ping's ring, seconds
	ph   float64 // the oscillator's phase, 0..1
	hp   biquad
}

// ping is a pad's note: hz, in wave, ringing ring seconds.
func ping(wave int, hz, ring float64, wait int) voice {
	return voice{drum: -1, wait: wait, wave: wave, hz: hz, ring: ring}
}

// drum is a hit of drum v.
func drum(v, wait int) voice {
	vc := voice{drum: v, wait: wait}
	switch v {
	case rhythm.Snare:
		vc.hp = newHighpass(1200)
	case rhythm.Hat:
		vc.hp = newHighpass(6500)
	case rhythm.Cymbal:
		vc.hp = newHighpass(4000)
	}
	return vc
}

// bus is the output the voice is heard on.
func (v *voice) bus() int {
	if v.drum < 0 {
		return busTM
	}
	return busDR
}

// next is the voice's next sample, and false once it has stopped.
func (v *voice) next() (float64, bool) {
	if v.wait > 0 {
		v.wait--
		return 0, true
	}
	t := float64(v.n) / sampleRate
	n := v.n
	v.n++
	switch v.drum {
	case -1:
		// A fast attack and an exponential fall, stopped a moment after.
		if t >= v.ring+0.02 {
			return 0, false
		}
		g := t / 0.006
		if t >= 0.006 {
			g = expRamp(t, 0.006, 1, v.ring, 0.001)
		}
		if v.wave == audiosrc.WaveLFSR {
			// The noise pitched by playback rate: higher rows, brighter.
			return g * noiseAt(n, v.hz*32/sampleRate), true
		}
		dt := v.hz / sampleRate
		s := osc(v.wave, v.ph, dt)
		v.ph = math.Mod(v.ph+dt, 1)
		return g * s, true

	case rhythm.Bass:
		// A sine swept 150 → 45 Hz in a twentieth of a second: the sweep is
		// the drum.
		if t >= 0.3 {
			return 0, false
		}
		s := math.Sin(2 * math.Pi * v.ph)
		v.ph = math.Mod(v.ph+expRamp(t, 0, 150, 0.05, 45)/sampleRate, 1)
		return expRamp(t, 0, 0.9, 0.28, 0.001) * s, true

	case rhythm.Snare:
		// The wires, noise above 1.2 kHz, and the head, a 190 Hz triangle.
		if t >= 0.2 {
			return 0, false
		}
		head := 0.4 * osc(audiosrc.WaveTriangle, v.ph, 0)
		v.ph = math.Mod(v.ph+190.0/sampleRate, 1)
		return expRamp(t, 0, 0.7, 0.18, 0.001) * (v.hp.next(noiseAt(n, 1)) + head), true
	}

	// A hat and a cymbal: the same noise, a tick and a wash.
	decay, peak := 0.05, 0.35
	if v.drum == rhythm.Cymbal {
		decay, peak = 0.45, 0.25
	}
	if t >= decay+0.02 {
		return 0, false
	}
	return expRamp(t, 0, peak, decay, 0.001) * v.hp.next(noiseAt(n, 1)), true
}
