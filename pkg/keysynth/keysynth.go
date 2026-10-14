// Package keysynth is the arithmetic of the synthesizer the Keys play, shared
// by the page, which builds each note of Web Audio nodes, and the native
// rack, which makes it a sample at a time: the settings the Synth bay's knobs
// hold, the presets, and what a note of a setting is made of.
package keysynth

import "math"

// Params is the engine's settings, one field a knob.
type Params struct {
	Str, Part, Tilt, Inhm, Strk, Uni, Damp float64 // STRING
	FM, Rato, Indx, Idec                   float64 // FM
	Wave                                   float64 // WAVE level
	Shape                                  string  // WAVE shape: an OscillatorNode type, "organ" or "noise"
	Nse, Tone, Ndec                        float64 // NOISE
	Brgt, Dark, Fdec, Reso                 float64 // FILTER
	Atk, Dec, Ktrk, Sus, Rel               float64 // AMP
}

// Field is the setting the knob id holds, or nil: the level and the two
// selectors are not numbers of the engine's.
func (p *Params) Field(id string) *float64 {
	switch id {
	case "syn-str":
		return &p.Str
	case "syn-part":
		return &p.Part
	case "syn-tilt":
		return &p.Tilt
	case "syn-inhm":
		return &p.Inhm
	case "syn-strk":
		return &p.Strk
	case "syn-uni":
		return &p.Uni
	case "syn-damp":
		return &p.Damp
	case "syn-fm":
		return &p.FM
	case "syn-rato":
		return &p.Rato
	case "syn-indx":
		return &p.Indx
	case "syn-idec":
		return &p.Idec
	case "syn-wave":
		return &p.Wave
	case "syn-nse":
		return &p.Nse
	case "syn-tone":
		return &p.Tone
	case "syn-ndec":
		return &p.Ndec
	case "syn-brgt":
		return &p.Brgt
	case "syn-dark":
		return &p.Dark
	case "syn-fdec":
		return &p.Fdec
	case "syn-reso":
		return &p.Reso
	case "syn-atk":
		return &p.Atk
	case "syn-dec":
		return &p.Dec
	case "syn-ktrk":
		return &p.Ktrk
	case "syn-sus":
		return &p.Sus
	case "syn-rel":
		return &p.Rel
	}
	return nil
}

// Hz is midi note's pitch, A4 at 440.
func Hz(midi int) float64 { return 440 * math.Pow(2, float64(midi-69)/12) }

// Tau is how long a note sustains, the time constant of its fall: DEC at C3,
// and as far as KTRK says, an octave less every eighteen semitones up — a
// piano's strings.
func (p *Params) Tau(midi int) float64 {
	return math.Max(0.02, math.Min(30, p.Dec*math.Pow(2, -p.Ktrk*float64(midi-48)/18)))
}

// Partial is one sine of the string: its pitch, its peak and its fall.
type Partial struct{ Hz, Amp, Tau float64 }

// Partials is the string layer of midi note at a Nyquist of nyq: overtones
// run sharp by B·n² (stiffer at both ends of the keyboard), weighted by where
// the hammer strikes, the high ones falling faster, and the lowest three from
// two strings UNI cents apart.
func (p *Params) Partials(midi int, nyq float64) []Partial {
	if p.Str <= 0 {
		return nil
	}
	hz, tau := Hz(midi), p.Tau(midi)
	d := float64(midi-60) / 30
	b := p.Inhm * 1e-4 * (1 + d*d)
	var out []Partial
	for n := 1; n <= int(p.Part); n++ {
		nf := float64(n)
		f := hz * nf * math.Sqrt(1+b*nf*nf)
		if f > math.Min(nyq*0.9, 16000) {
			break
		}
		a := p.Str * 1.5 / math.Pow(nf, p.Tilt)
		if p.Strk > 0 {
			a *= 0.35 + 0.65*math.Abs(math.Sin(math.Pi*nf*p.Strk))
		}
		td := tau / (1 + p.Damp*(nf-1))
		if n > 3 || p.Uni == 0 {
			out = append(out, Partial{f, a, td})
			continue
		}
		for _, c := range []float64{-p.Uni, p.Uni} {
			out = append(out, Partial{f * math.Pow(2, c/1200), a / 2, td * (1 + c/6)})
		}
	}
	return out
}

// Cutoffs is where the filter of a note at hz opens and where it closes
// towards, held under the Nyquist nyq.
func (p *Params) Cutoffs(hz, nyq float64) (from, to float64) {
	return math.Min(hz*p.Brgt, nyq*0.9), math.Min(hz*p.Dark, nyq*0.9)
}

// FMDepth is the FM layer's deviation at the strike, in hertz: the index
// times the modulator. It falls to FMFloor of it over IDEC.
func (p *Params) FMDepth(hz float64) float64 { return p.Indx * hz * p.Rato }

// FMFloor is what the FM layer's deviation falls towards, of its strike.
const FMFloor = 0.13

// NoiseStop is when the noise layer's burst ends, in seconds after the
// strike.
func (p *Params) NoiseStop() float64 { return math.Min(0.5, 0.001+p.Ndec*8) }

// Drawbars is the organ's wave by harmonic of the 16′, an octave under the
// key: drawbars 16′ 8′ 5⅓′ 4′ and a little 2⅔′.
var Drawbars = map[int]float64{1: 0.7, 2: 1, 3: 0.65, 4: 0.6, 6: 0.25}

// HammerNoise is the noise layer's burst, sr samples a second: half a second
// of white noise, the same every time.
func HammerNoise(sr int) []float32 {
	out := make([]float32, sr/2)
	seed := uint32(0x9e3779b9) // xorshift: any noise will do, and the same every time
	for i := range out {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		out[i] = float32(float64(seed)/math.MaxUint32*2 - 1)
	}
	return out
}

// Preset is an instrument: the knobs it changes from the piano, which is
// every knob's default, and its wave shape.
type Preset struct {
	Name, Text string
	Set        map[string]float64
	Shape      string
}

// Presets are the instruments, in the selector's order.
var Presets = []Preset{
	{"piano", "piano", nil, "sine"},
	{"epiano", "e-piano", map[string]float64{
		"syn-str": 0, "syn-fm": 1, "syn-rato": 1, "syn-indx": 2.6, "syn-idec": 0.3,
		"syn-nse": 0.12, "syn-tone": 14, "syn-ndec": 0.03,
		"syn-atk": 0.003, "syn-dec": 2.5, "syn-ktrk": 0.75, "syn-rel": 0.35,
	}, "sine"},
	{"hpschd", "hpschd", map[string]float64{
		"syn-part": 16, "syn-tilt": 0.7, "syn-inhm": 0.5, "syn-strk": 0.12, "syn-uni": 0, "syn-damp": 0.1,
		"syn-nse": 0.2, "syn-tone": 8, "syn-ndec": 0.004,
		"syn-brgt": 24, "syn-dark": 3, "syn-fdec": 0.55,
		"syn-atk": 0.001, "syn-dec": 1.6, "syn-ktrk": 0.75, "syn-rel": 0.08,
	}, "sine"},
	{"organ", "organ", map[string]float64{
		"syn-str": 0, "syn-wave": 1, "syn-nse": 0, "syn-atk": 0.005, "syn-sus": 1, "syn-rel": 0.04,
	}, "organ"},
	{"sine", "sine", wavePreset, "sine"},
	{"tri", "tri", wavePreset, "triangle"},
	{"sqr", "sqr", wavePreset, "square"},
	{"saw", "saw", wavePreset, "sawtooth"},
	{"noise", "noise", wavePreset, "noise"},
}

// wavePreset is a plain waveform: the oscillator alone, held while the key
// is down.
var wavePreset = map[string]float64{
	"syn-str": 0, "syn-wave": 1, "syn-nse": 0, "syn-atk": 0.008, "syn-sus": 1, "syn-rel": 0.12,
}

// PresetByName is the preset called name.
func PresetByName(name string) (Preset, bool) {
	for _, p := range Presets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}
