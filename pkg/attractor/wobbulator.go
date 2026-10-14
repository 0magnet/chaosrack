package attractor

import "math"

// The FVF wobbulator's knobs and its signal processing, shared by the page
// (fvf_js.go, where the file comment says what it is a model of) and a
// native rack. Where the samples come from and where the processed ones go
// are each side's own.
//
// Signal flow per sample: crude zero-crossing pitch track (f_in) → clamp(
// gain·f_in + offset, fmin, fmax) = carrier freq → carrier oscillator
// (pulse / square / ÷2 sub) → ring or AM modulation by the original audio.

const (
	fvfPitchFloor = 40.0
	fvfPitchCeil  = 3000.0
)

// Wobbulator is the FVF harmonic wobbulator's knobs and switches.
type Wobbulator struct {
	gain   float32 // f_out = gain·f_in + offset
	offset float32 // Hz floor / transposition
	fMin   float32 // Hz; carrier never stops (V/F can't do 0 Hz)
	fMax   float32 // Hz ceiling
	duty   float32 // pulse duty (brightness)
	mix    float32 // 0 = dry .. 1 = fully processed
	glide  float32 // pitch smoothing (low = snappy/glitchy = faithful)
	wave   int     // 0 square, 1 pulse, 2 sub (÷2)
	mod    int     // 0 ring, 1 AM
	bypass bool    // true = pass raw input through (FX switch off)
}

// NewWobbulator is a wobbulator at the page's defaults.
func NewWobbulator() *Wobbulator {
	return &Wobbulator{gain: 1, fMin: 30, fMax: 6000, duty: 0.12, mix: 1, glide: 0.25, wave: 1}
}

// WobbulatorKnobs are the wobbulator's knobs and selectors, by the page's ids.
var WobbulatorKnobs = []string{"fvf-gain", "fvf-offset", "fvf-fmin", "fvf-fmax", "fvf-duty", "fvf-mix", "fvf-glide", "fvf-wave", "fvf-mod"}

// Set turns knob id to v, reporting whether the wobbulator has it.
func (w *Wobbulator) Set(id string, v float32) bool {
	switch id {
	case "fvf-gain":
		w.gain = v
	case "fvf-offset":
		w.offset = v
	case "fvf-fmin":
		w.fMin = v
	case "fvf-fmax":
		w.fMax = v
	case "fvf-duty":
		w.duty = v
	case "fvf-mix":
		w.mix = v
	case "fvf-glide":
		w.glide = v
	case "fvf-wave":
		w.wave = int(v + 0.5)
	case "fvf-mod":
		w.mod = int(v + 0.5)
	default:
		return false
	}
	return true
}

// Bypassed reports whether the FX switch is off: the input passes as it is.
func (w *Wobbulator) Bypassed() bool { return w.bypass }

// SetBypass throws the FX switch: off passes the input through.
func (w *Wobbulator) SetBypass(b bool) { w.bypass = b }

// FVFProcessor is one stream through the wobbulator: its pitch track and
// its carrier's phase. It reads the wobbulator's knobs each sample, so they
// adjust it in real time.
type FVFProcessor struct {
	w                 *Wobbulator
	sampleRate        float64
	lastPos           bool
	sinceCross        int
	fIn, phase, subPh float64
}

// Processor is a fresh stream through w at sample rate sr.
func (w *Wobbulator) Processor(sr float64) *FVFProcessor {
	return &FVFProcessor{w: w, sampleRate: sr}
}

// Rate is the stream's sample rate.
func (p *FVFProcessor) Rate() float64 { return p.sampleRate }

// SetRate changes it, for a source whose rate is only known later.
func (p *FVFProcessor) SetRate(sr float64) { p.sampleRate = sr }

// Process is one sample through the wobbulator.
func (p *FVFProcessor) Process(x float32) float32 {
	w := p.w
	// Bypass = pass the raw incoming audio straight through, so the "FX"
	// switch is an instant A/B between the untouched signal and the
	// wobbulated one (independent of the mix knob's position).
	if w.bypass {
		return x
	}
	xf := float64(x)

	// Limiter + F/V as a zero-crossing pitch tracker (deliberately crude —
	// the wobble and octave jumps are the character).
	pos := xf >= 0
	p.sinceCross++
	if pos && !p.lastPos && p.sinceCross > 1 {
		f := p.sampleRate / float64(p.sinceCross)
		if f >= fvfPitchFloor && f <= fvfPitchCeil {
			p.fIn += float64(w.glide) * (f - p.fIn)
		}
		p.sinceCross = 0
	}
	p.lastPos = pos

	// gain·f + offset, clamped so the carrier never dies.
	fOut := float64(w.gain)*p.fIn + float64(w.offset)
	if fOut < float64(w.fMin) {
		fOut = float64(w.fMin)
	}
	if fOut > float64(w.fMax) {
		fOut = float64(w.fMax)
	}

	p.phase += fOut / p.sampleRate
	if p.phase >= 1 {
		p.phase -= math.Floor(p.phase)
	}
	p.subPh += (fOut * 0.5) / p.sampleRate
	if p.subPh >= 1 {
		p.subPh -= math.Floor(p.subPh)
	}

	var carrier float64
	switch w.wave {
	case 1: // variable-duty pulse (bright, harmonic-rich)
		duty := float64(w.duty)
		if duty < 0.001 {
			duty = 0.001
		} else if duty > 0.5 {
			duty = 0.5
		}
		if p.phase < duty {
			carrier = 1
		} else {
			carrier = -1
		}
	case 2: // ÷2 sub-octave square
		if p.subPh < 0.5 {
			carrier = 1
		} else {
			carrier = -1
		}
	default: // 50% square
		if p.phase < 0.5 {
			carrier = 1
		} else {
			carrier = -1
		}
	}

	var wet float64
	if w.mod == 1 { // AM / balanced
		wet = xf * (1 + carrier) * 0.5
	} else { // ring (four-quadrant)
		wet = xf * carrier
	}
	out := float64(w.mix)*wet + (1-float64(w.mix))*xf
	if out > 1 {
		out = 1
	} else if out < -1 {
		out = -1
	}
	return float32(out)
}
