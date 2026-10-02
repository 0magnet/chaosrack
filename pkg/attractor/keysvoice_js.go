//go:build js && wasm

package attractor

// The synthesizer the keyboard plays: one engine, every voice a setting of
// it, and every setting a knob on the Synth bay (synth_js.go). It is built
// out of plain Web Audio nodes, so a key sounds the moment it is pressed —
// nothing is rendered or loaded first.
//
// A note is up to four layers, summed into a lowpass filter, then the note's
// own release gain:
//
//   - STRING, additive partials, which is the piano: overtones not quite
//     harmonic (a stiff string's run sharp, by B·n², most of why a piano does
//     not sound like an organ), each falling away at its own rate, the high
//     ones faster (DAMP), so the tone darkens as it sustains; weighted by
//     where the hammer or quill strikes (STRK), which a string cannot sound
//     at a node; the lowest from two strings a few cents apart (UNI), whose
//     slow beating is a piano's unison.
//   - FM, a sine modulated by another at RATO times its frequency, the depth
//     (INDX) fading over IDEC: the electric piano's bark fading to a pure tone.
//   - WAVE, one oscillator: a generator waveform, the organ's drawbars, or the
//     tuned shift-register noise.
//   - NOISE, a burst of filtered noise at TONE times the note: a hammer, a
//     quill, a tine's click.
//
// AMP shapes every layer alike: an attack, then a fall towards SUS with
// time constant DEC, longer in the bass than the treble as far as KTRK says,
// and REL to let go. The filter opens at BRGT times the note and closes
// towards DARK times it over FDEC — a plucked string brightest at the pluck.

import (
	"math"
	"syscall/js"
)

// keyVoice is one sounding note: what it is made of, and how it ends.
type keyVoice struct {
	g    js.Value   // its output, into the master chain: the release, and glide's re-attack
	srcs []js.Value // every source it started, stopped together
	rel  float64    // how long letting go takes, in seconds
	osc  js.Value   // a plain waveform's oscillator, which the shape knob retypes live
}

// release lets the note go at audio time t: its output falls away over rel,
// and its sources stop once it has.
func (v *keyVoice) release(t float64, now bool) {
	g := v.g.Get("gain")
	if now {
		g.Call("cancelScheduledValues", t)
		g.Call("setValueAtTime", g.Get("value"), t)
	}
	g.Call("setTargetAtTime", 0, t, v.rel/4)
	for _, s := range v.srcs {
		s.Call("stop", t+v.rel+0.05)
	}
}

// envelope gives gain param p the amp envelope at time t0: up to peak over
// atk, then towards peak·sus with time constant tau.
func envelope(p js.Value, peak, atk, sus, tau, t0 float64) {
	p.Call("setValueAtTime", 0, t0)
	p.Call("linearRampToValueAtTime", peak, t0+atk)
	p.Call("setTargetAtTime", peak*sus, t0+atk, tau)
}

// startVoice starts one note at audio time t0, as the Synth bay is set.
func (k *keyboard) startVoice(ctx js.Value, midi int, t0 float64) *keyVoice {
	s := &synth
	hz := 440 * math.Pow(2, float64(midi-69)/12)
	nyq := ctx.Get("sampleRate").Float() / 2
	v := &keyVoice{g: ctx.Call("createGain"), rel: s.rel}
	v.g.Call("connect", k.master)

	lp := ctx.Call("createBiquadFilter")
	lp.Set("type", "lowpass")
	lp.Get("Q").Set("value", s.reso)
	lf := lp.Get("frequency")
	lf.Call("setValueAtTime", math.Min(hz*s.brgt, nyq*0.9), t0)
	lf.Call("setTargetAtTime", math.Min(hz*s.dark, nyq*0.9), t0, s.fdec)
	lp.Call("connect", v.g)

	// How long the note sustains: DEC at C3, and as far as KTRK says, an
	// octave less every eighteen semitones up — a piano's strings.
	tau := math.Max(0.02, math.Min(30, s.dec*math.Pow(2, -s.ktrk*float64(midi-48)/18)))
	add := func(o js.Value) { v.srcs = append(v.srcs, o) }

	if s.str > 0 {
		d := float64(midi-60) / 30
		b := s.inhm * 1e-4 * (1 + d*d) // stiffer at both ends of the keyboard
		for n := 1; n <= int(s.part); n++ {
			nf := float64(n)
			f := hz * nf * math.Sqrt(1+b*nf*nf)
			if f > math.Min(nyq*0.9, 16000) {
				break
			}
			a := s.str * 1.5 / math.Pow(nf, s.tilt)
			if s.strk > 0 {
				a *= 0.35 + 0.65*math.Abs(math.Sin(math.Pi*nf*s.strk))
			}
			td := tau / (1 + s.damp*(nf-1))
			if n > 3 || s.uni == 0 {
				add(synthSine(ctx, lp, f, a, s.atk, s.sus, td, t0))
				continue
			}
			for _, c := range []float64{-s.uni, s.uni} {
				add(synthSine(ctx, lp, f*math.Pow(2, c/1200), a/2, s.atk, s.sus, td*(1+c/6), t0))
			}
		}
	}
	if s.fm > 0 {
		car := ctx.Call("createOscillator")
		car.Get("frequency").Set("value", hz)
		mod := ctx.Call("createOscillator")
		mod.Get("frequency").Set("value", hz*s.rato)
		idx := ctx.Call("createGain") // deviation, in hertz: index times the modulator
		ig := idx.Get("gain")
		ig.Call("setValueAtTime", s.indx*hz*s.rato, t0)
		ig.Call("setTargetAtTime", s.indx*hz*s.rato*0.13, t0, s.idec)
		mod.Call("connect", idx)
		idx.Call("connect", car.Get("frequency"))
		amp := ctx.Call("createGain")
		envelope(amp.Get("gain"), s.fm*0.9, s.atk, s.sus, tau, t0)
		car.Call("connect", amp)
		amp.Call("connect", lp)
		car.Call("start", t0)
		mod.Call("start", t0)
		add(car)
		add(mod)
	}
	if s.wave > 0 {
		var o js.Value
		peak := s.wave * 0.8
		switch s.shape {
		case "noise":
			// The shift-register loop, tuned to the key by its playback rate.
			o = ctx.Call("createBufferSource")
			o.Set("buffer", gen.noiseBuffer(ctx))
			o.Set("loop", true)
			o.Get("playbackRate").Set("value", hz*32/ctx.Get("sampleRate").Float())
			peak *= 0.6
		case "organ":
			o = ctx.Call("createOscillator")
			o.Call("setPeriodicWave", k.organDrawbars(ctx))
			o.Get("frequency").Set("value", hz/2) // built on the 16′, an octave under the key
		default:
			o = ctx.Call("createOscillator")
			o.Set("type", s.shape)
			o.Get("frequency").Set("value", hz)
			v.osc = o
		}
		amp := ctx.Call("createGain")
		envelope(amp.Get("gain"), peak, s.atk, s.sus, tau, t0)
		o.Call("connect", amp)
		amp.Call("connect", lp)
		o.Call("start", t0)
		add(o)
	}
	if s.nse > 0 {
		h := ctx.Call("createBufferSource")
		h.Set("buffer", k.hammerNoise(ctx))
		bp := ctx.Call("createBiquadFilter")
		bp.Set("type", "bandpass")
		bp.Get("frequency").Set("value", math.Min(hz*s.tone, nyq*0.9))
		bp.Get("Q").Set("value", 0.8)
		hg := ctx.Call("createGain")
		hgg := hg.Get("gain")
		hgg.Call("setValueAtTime", 0, t0)
		hgg.Call("linearRampToValueAtTime", s.nse*0.5, t0+0.001)
		hgg.Call("setTargetAtTime", 0, t0+0.001, s.ndec)
		h.Call("connect", bp)
		bp.Call("connect", hg)
		hg.Call("connect", lp)
		h.Call("start", t0)
		h.Call("stop", t0+math.Min(0.5, 0.001+s.ndec*8)) // it ends itself
	}
	v.g.Get("gain").Call("setValueAtTime", 1, t0)
	return v
}

// synthSine starts a sine at hz into dst at t0 under the amp envelope, peak
// high, and returns it.
func synthSine(ctx, dst js.Value, hz, peak, atk, sus, tau, t0 float64) js.Value {
	o := ctx.Call("createOscillator")
	o.Get("frequency").Set("value", hz)
	g := ctx.Call("createGain")
	envelope(g.Get("gain"), peak, atk, sus, tau, t0)
	o.Call("connect", g)
	g.Call("connect", dst)
	o.Call("start", t0)
	return o
}

// organDrawbars is the organ's wave, made once: drawbars 16′ 8′ 5⅓′ 4′ and a
// little 2⅔′ — on the 16′, so they are its 1st, 2nd, 3rd, 4th and 6th
// harmonics.
func (k *keyboard) organDrawbars(ctx js.Value) js.Value {
	if !k.organWave.Truthy() {
		re := js.Global().Get("Float32Array").New(7)
		im := js.Global().Get("Float32Array").New(7)
		for h, a := range map[int]float64{1: 0.7, 2: 1, 3: 0.65, 4: 0.6, 6: 0.25} {
			im.SetIndex(h, a)
		}
		k.organWave = ctx.Call("createPeriodicWave", re, im)
	}
	return k.organWave
}

// hammerNoise is a buffer of white noise, made once: long enough for the
// slowest NDEC the noise layer allows.
func (k *keyboard) hammerNoise(ctx js.Value) js.Value {
	if k.hammerBuf.Truthy() {
		return k.hammerBuf
	}
	rate := ctx.Get("sampleRate").Float()
	n := int(rate * 0.5)
	buf := ctx.Call("createBuffer", 1, n, rate)
	ch := buf.Call("getChannelData", 0)
	seed := uint32(0x9e3779b9) // xorshift: any noise will do, and the same every time
	for i := range n {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		ch.SetIndex(i, float64(seed)/math.MaxUint32*2-1)
	}
	k.hammerBuf = buf
	return buf
}
