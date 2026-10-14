package rackhost

import (
	"math"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/keysynth"
)

// The Keys: the synthesizer the keyboard plays, as the page builds each note
// of Web Audio nodes (keysvoice_js.go), made here a sample at a time. A note
// is up to four layers — the string's partials, FM, one wave, a burst of
// noise — summed into a lowpass that closes as the note sounds, then the
// note's own gain, which lets it go. Every setting is a knob on the Synth bay
// (keysynth.Params), read off the controls as they turn.

// keysHeadroom is the master gain under the level knob: room for chords.
const keysHeadroom = 0.25

// maxKeyVoices is as many notes as sound at once; a glissando's passing
// notes past it take the place of the oldest let go.
const maxKeyVoices = 64

// cutEvery is how many samples the filter's cutoff holds between moves.
const cutEvery = 16

// hammer is the noise layer's burst, played once from the top every note.
var hammer = keysynth.HammerNoise(sampleRate)

// organTable is one period of the organ's wave, the drawbars summed and
// scaled to a peak of one, as a periodic wave is.
var organTable = func() []float64 {
	const n = 4096
	out := make([]float64, n)
	peak := 0.0
	for i := range out {
		x := float64(i) / n
		for h, a := range keysynth.Drawbars {
			out[i] += a * math.Sin(2*math.Pi*float64(h)*x)
		}
		peak = math.Max(peak, math.Abs(out[i]))
	}
	for i := range out {
		out[i] /= peak
	}
	return out
}()

// organAt is the organ's wave at phase ph (0..1), read between its samples.
func organAt(ph float64) float64 {
	p := ph * float64(len(organTable))
	i := int(p)
	f := p - float64(i)
	a, b := organTable[i%len(organTable)], organTable[(i+1)%len(organTable)]
	return a + (b-a)*f
}

// adEnv is the amp envelope one layer's gain follows: from nothing up to
// peak over the attack, then towards peak·sus with time constant tau.
type adEnv struct {
	peak, sus float64
	atk       float64 // the attack, in samples
	d, k      float64 // how far the fall has still to go, and its step
}

func newEnv(peak, atk, sus, tau float64) adEnv {
	return adEnv{peak: peak, sus: sus, atk: atk * sampleRate, d: 1, k: math.Exp(-1 / (tau * sampleRate))}
}

// at is the envelope n samples into the note, n counting up by one a call.
func (e *adEnv) at(n int) float64 {
	if t := float64(n); t < e.atk {
		return e.peak * t / e.atk
	}
	g := e.peak * (e.sus + (1-e.sus)*e.d)
	e.d *= e.k
	return g
}

// partial is one of the string's sines: a phasor turned a step a sample,
// which costs a few multiplies where a sine costs a call.
type partial struct {
	c, s, cw, sw float64
	env          adEnv
}

// keyVoice is one sounding note.
type keyVoice struct {
	midi int
	wait int // samples before it starts
	n    int // samples since it started
	hz   float64

	parts []partial

	fm               bool
	fmEnv            adEnv
	dev0, devD, devK float64 // the deviation's floor, how far above it, its fall
	rato             float64 // the modulator over the note
	carPh, modPh     float64

	wave    bool
	shape   string
	plain   int // a plain shape's wave, audiosrc's numbering
	waveEnv adEnv
	ph      float64

	nse         bool
	nseG, nseK  float64 // the burst's peak, then its gain as it falls, and its fall
	nseStop     int
	bp          biquad
	lp          biquad
	reso        float64
	cutTo, cutD float64 // where the cutoff falls to, how far above it
	cutK        float64 // its fall, a cutEvery block
	gain, relK  float64
	releaseAt   int // n at which it is let go, or -1
	releasedAt  int // n at which it was, or -1
	stopAfter   int // samples after the release that it stops
}

// plainWave is a plain shape's wave, or -1 for the organ and the noise.
func plainWave(shape string) int {
	switch shape {
	case "sine":
		return audiosrc.WaveSine
	case "triangle":
		return audiosrc.WaveTriangle
	case "square":
		return audiosrc.WaveSquare
	case "sawtooth":
		return audiosrc.WaveSaw
	}
	return -1
}

// startKey is a note of midi as p is set, starting wait samples from now.
func startKey(p *keysynth.Params, midi, wait int) *keyVoice {
	const nyq = sampleRate / 2
	hz := keysynth.Hz(midi)
	tau := p.Tau(midi)
	v := &keyVoice{midi: midi, wait: wait, hz: hz, gain: 1, releaseAt: -1, releasedAt: -1, reso: p.Reso}
	from, to := p.Cutoffs(hz, nyq)
	v.cutTo, v.cutD = to, from-to
	v.cutK = math.Exp(-cutEvery / (math.Max(p.Fdec, 1e-4) * sampleRate))
	v.lp.lowpass(from, p.Reso)
	rel := math.Max(p.Rel, 1e-3)
	v.relK = math.Exp(-1 / (rel / 4 * sampleRate))
	v.stopAfter = int((rel + 0.05) * sampleRate)

	for _, pt := range p.Partials(midi, nyq) {
		w := 2 * math.Pi * pt.Hz / sampleRate
		v.parts = append(v.parts, partial{c: 1, cw: math.Cos(w), sw: math.Sin(w),
			env: newEnv(pt.Amp, p.Atk, p.Sus, pt.Tau)})
	}
	if p.FM > 0 {
		v.fm = true
		v.fmEnv = newEnv(p.FM*0.9, p.Atk, p.Sus, tau)
		d := p.FMDepth(hz)
		v.dev0, v.devD = d*keysynth.FMFloor, d*(1-keysynth.FMFloor)
		v.rato = p.Rato
		v.devK = math.Exp(-1 / (math.Max(p.Idec, 1e-4) * sampleRate))
	}
	if p.Wave > 0 {
		v.wave = true
		v.shape, v.plain = p.Shape, plainWave(p.Shape)
		peak := p.Wave * 0.8
		if p.Shape == "noise" {
			peak *= 0.6
		}
		v.waveEnv = newEnv(peak, p.Atk, p.Sus, tau)
	}
	if p.Nse > 0 {
		v.nse = true
		v.nseG = p.Nse * 0.5
		v.nseK = math.Exp(-1 / (math.Max(p.Ndec, 1e-4) * sampleRate))
		v.nseStop = int(p.NoiseStop() * sampleRate)
		v.bp = newBandpass(math.Min(hz*p.Tone, nyq*0.9), 0.8)
	}
	return v
}

// release lets the note go now, or as soon as it starts.
func (v *keyVoice) release() {
	if v.releaseAt < 0 || v.releaseAt > v.n {
		v.releaseAt = v.n
	}
}

// next is the voice's next sample, and false once it has stopped.
func (v *keyVoice) next() (float64, bool) {
	if v.wait > 0 {
		v.wait--
		return 0, true
	}
	n := v.n
	v.n++
	if v.releasedAt < 0 && v.releaseAt >= 0 && n >= v.releaseAt {
		v.releasedAt = n
	}
	if v.releasedAt >= 0 {
		if n-v.releasedAt >= v.stopAfter {
			return 0, false
		}
		v.gain *= v.relK
	}

	x := 0.0
	for i := range v.parts {
		pt := &v.parts[i]
		x += pt.s * pt.env.at(n)
		c, s := pt.c*pt.cw-pt.s*pt.sw, pt.s*pt.cw+pt.c*pt.sw
		k := 1.5 - 0.5*(c*c+s*s) // keeps the phasor on the circle
		pt.c, pt.s = c*k, s*k
	}
	if v.fm {
		x += v.fmEnv.at(n) * math.Sin(2*math.Pi*v.carPh)
		dev := v.dev0 + v.devD
		v.devD *= v.devK
		f := v.hz + dev*math.Sin(2*math.Pi*v.modPh)
		v.carPh = frac(v.carPh + f/sampleRate)
		v.modPh = frac(v.modPh + v.hz*v.rato/sampleRate)
	}
	if v.wave {
		g := v.waveEnv.at(n)
		switch {
		case v.shape == "noise":
			x += g * noiseAt(n, v.hz*32/sampleRate)
		case v.shape == "organ":
			// Built on the 16′, an octave under the key.
			x += g * organAt(v.ph)
			v.ph = frac(v.ph + v.hz/2/sampleRate)
		default:
			dt := v.hz / sampleRate
			x += g * osc(v.plain, v.ph, dt)
			v.ph = frac(v.ph + dt)
		}
	}
	if v.nse && n < v.nseStop && n < len(hammer) {
		// Up in a millisecond, then falling away over NDEC.
		const rise = sampleRate / 1000
		g := v.nseG * float64(n) / rise
		if n >= rise {
			g = v.nseG
			v.nseG *= v.nseK
		}
		x += g * v.bp.next(float64(hammer[n]))
	}

	if n%cutEvery == 0 && n > 0 {
		v.cutD *= v.cutK
		v.lp.lowpass(v.cutTo+v.cutD, v.reso)
	}
	return v.gain * v.lp.next(x), true
}

func frac(x float64) float64 { return x - math.Floor(x) }

// keyBank is the keyboard's sound: the settings, the level, and every note
// sounding.
type keyBank struct {
	p      keysynth.Params
	lvl    float64
	voices []*keyVoice
	held   map[int]*keyVoice // the notes whose keys are down
	edits  int               // counts the keys going down and up, for the picture
}

func newKeyBank() keyBank { return keyBank{held: map[int]*keyVoice{}} }

// add sounds v, in the place of the oldest note let go if there are too
// many.
func (k *keyBank) add(v *keyVoice) {
	if len(k.voices) >= maxKeyVoices {
		at := 0
		for i, o := range k.voices {
			if o.releaseAt >= 0 {
				at = i
				break
			}
		}
		k.voices = append(k.voices[:at], k.voices[at+1:]...)
	}
	k.voices = append(k.voices, v)
}

// down sounds midi until up, starting wait samples from now.
func (k *keyBank) down(midi, wait int) {
	if _, ok := k.held[midi]; ok {
		return
	}
	v := startKey(&k.p, midi, wait)
	k.held[midi] = v
	k.add(v)
	k.edits++
}

// up lets midi go.
func (k *keyBank) up(midi int) {
	v, ok := k.held[midi]
	if !ok {
		return
	}
	delete(k.held, midi)
	v.release()
	k.edits++
}

// pass sounds midi as a glissando passes it: from wait samples on, for dur.
func (k *keyBank) pass(midi, wait, dur int) {
	v := startKey(&k.p, midi, wait)
	v.releaseAt = dur
	k.add(v)
	k.edits++
}

// lit is every key sounding as if pressed: held, or passed and not yet let
// go.
func (k *keyBank) lit() map[int]bool {
	out := map[int]bool{}
	for _, v := range k.voices {
		if v.releasedAt < 0 && (v.releaseAt < 0 || v.n < v.releaseAt) {
			out[v.midi] = true
		}
	}
	return out
}

// setShape is the wave shape knob: the next note's, and a held plain
// waveform's at once, as the page retypes its oscillator.
func (k *keyBank) setShape(shape string) {
	k.p.Shape = shape
	w := plainWave(shape)
	if w < 0 {
		return
	}
	for _, v := range k.voices {
		if v.wave && v.plain >= 0 {
			v.shape, v.plain = shape, w
		}
	}
}

// render adds n samples of every note into out, at the level, and lets go
// of the notes that have stopped.
func (k *keyBank) render(out []float32, n int) {
	clear(out[:n])
	g := k.lvl * keysHeadroom
	live := k.voices[:0]
	for _, v := range k.voices {
		on := true
		for j := 0; j < n && on; j++ {
			var x float64
			x, on = v.next()
			out[j] += float32(g * x)
		}
		if on {
			live = append(live, v)
		}
	}
	clear(k.voices[len(live):])
	k.voices = live
}

// isKeysControl reports whether id is one of the Synth bay's.
func isKeysControl(id string) bool { return strings.HasPrefix(id, "syn-") || id == "keys-lvl" }

// keysSet reads one of the Synth bay's controls into the keys, and says what
// other controls follow: a preset sets every knob, to its own value or the
// piano's, which is the knob's default. Called with r.mu and the sound's
// lock held.
func (r *Rack) keysSet(id string) (more [][2]string) {
	k := &r.sound.keys
	switch id {
	case "keys-lvl":
		k.lvl = r.number(id) / 100
	case "syn-shape":
		k.setShape(r.value(id))
	case "syn-preset":
		p, ok := keysynth.PresetByName(r.value(id))
		if !ok {
			return nil
		}
		for _, c := range r.ctls {
			if k.p.Field(c.ID) == nil {
				continue
			}
			v, ok := p.Set[c.ID]
			if !ok {
				v = c.Def
			}
			if r.number(c.ID) != v {
				more = append(more, [2]string{c.ID, strconv.FormatFloat(v, 'f', -1, 64)})
			}
		}
		if r.value("syn-shape") != p.Shape {
			more = append(more, [2]string{"syn-shape", p.Shape})
		}
	default:
		if f := k.p.Field(id); f != nil {
			*f = r.number(id)
		}
	}
	return more
}
