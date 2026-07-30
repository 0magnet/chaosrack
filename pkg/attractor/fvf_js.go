//go:build js && wasm

package attractor

// FVF ("harmonic wobbulator") mode — a software analog of the
// Frequency→Voltage→Frequency converter with balanced modulator designed at
// bunkerofdoom.com, ported from
// the 0magnet/FVF prototype into wasm-stuff so it reuses the audio source,
// the spectrogram display, and the knob control surface. In this mode the
// live audio is run through the FVF DSP and the PROCESSED result is shown on
// the spectrogram, so you can see (and, once audio-out lands, hear) the
// transformation while tweaking the F→V→F chain with knobs.
//
// Signal flow per sample: crude zero-crossing pitch track (f_in) → clamp(
// gain·f_in + offset, fmin, fmax) = carrier freq → carrier oscillator
// (pulse / square / ÷2 sub) → ring or AM modulation by the original audio.

import (
	"math"
	"strconv"
	"syscall/js"
	"unsafe"
)

const (
	fvfPitchFloor = 40.0
	fvfPitchCeil  = 3000.0
)

var (
	fvfGain   float32 = 1    // f_out = gain·f_in + offset
	fvfOffset float32 = 0    // Hz floor / transposition
	fvfFMin   float32 = 30   // Hz; carrier never stops (V/F can't do 0 Hz)
	fvfFMax   float32 = 6000 // Hz ceiling
	fvfDuty   float32 = 0.12 // pulse duty (brightness)
	fvfMix    float32 = 1    // 0 = dry .. 1 = fully processed
	fvfGlide  float32 = 0.25 // pitch smoothing (low = snappy/glitchy = faithful)
	fvfWave   int     = 1    // 0 square, 1 pulse, 2 sub (÷2)
	fvfMod    int     = 0    // 0 ring, 1 AM
	fvfBypass bool           // true = pass raw input through (FX switch off)
	fvfProc   *fvfProcessor
)

func init() {
	attractorParams["fvf"] = []paramDef{
		{"fvf-gain", "gain", &fvfGain, 1, 0.01, 20, 0.01},
		{"fvf-offset", "offset", &fvfOffset, 0, 0, 4000, 10},
		{"fvf-fmin", "fmin", &fvfFMin, 30, 10, 2000, 10},
		{"fvf-fmax", "fmax", &fvfFMax, 6000, 500, 12000, 50},
		{"fvf-duty", "duty", &fvfDuty, 0.12, 0.01, 0.5, 0.01},
		{"fvf-mix", "mix", &fvfMix, 1, 0, 1, 0.01},
		{"fvf-glide", "glide", &fvfGlide, 0.25, 0.01, 1, 0.01},
	}
	attractorDescriptions["fvf"] = "FVF — Harmonic Wobbulator. A software analog of the " +
		"Frequency→Voltage→Frequency converter with balanced modulator designed at bunkerofdoom.com " +
		"(hardware built 1984). The live audio's " +
		"pitch is tracked, scaled/offset into a new carrier frequency, and ring- or AM-modulated back by " +
		"the original signal — the metallic, glitchy 'very strange' timbre. Shown here as the processed " +
		"spectrogram."
}

// fvfProcessor is a single-channel FVF engine; it reads the live fvf* control
// vars each sample so the knobs adjust it in real time.
type fvfProcessor struct {
	sampleRate        float64
	lastPos           bool
	sinceCross        int
	fIn, phase, subPh float64
}

func ensureFVFProc() {
	sr := 24000.0
	if audioSource != nil && audioSource.SampleRate() > 0 {
		sr = float64(audioSource.SampleRate())
	}
	if fvfProc == nil || fvfProc.sampleRate != sr {
		fvfProc = &fvfProcessor{sampleRate: sr}
	}
}

func (p *fvfProcessor) Process(x float32) float32 {
	// Bypass = pass the raw incoming audio straight through, so the "FX"
	// switch is an instant A/B between the untouched signal and the
	// wobbulated one (independent of the mix knob's position).
	if fvfBypass {
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
			p.fIn += float64(fvfGlide) * (f - p.fIn)
		}
		p.sinceCross = 0
	}
	p.lastPos = pos

	// gain·f + offset, clamped so the carrier never dies.
	fOut := float64(fvfGain)*p.fIn + float64(fvfOffset)
	if fOut < float64(fvfFMin) {
		fOut = float64(fvfFMin)
	}
	if fOut > float64(fvfFMax) {
		fOut = float64(fvfFMax)
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
	switch fvfWave {
	case 1: // variable-duty pulse (bright, harmonic-rich)
		duty := float64(fvfDuty)
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
	if fvfMod == 1 { // AM / balanced
		wet = xf * (1 + carrier) * 0.5
	} else { // ring (four-quadrant)
		wet = xf * carrier
	}
	out := float64(fvfMix)*wet + (1-float64(fvfMix))*xf
	if out > 1 {
		out = 1
	} else if out < -1 {
		out = -1
	}
	return float32(out)
}

// appendFVFSelectors adds the waveform and modulator selector knobs to the
// FVF control panel (buildParamPanel calls this after the float-param knobs).
func appendFVFSelectors(paramsDiv js.Value) {
	makeSel := func(label string, opts []string, cur int, onChange func(int)) js.Value {
		cell := doc.Call("createElement", "span")
		cell.Set("className", "pcell")
		lbl := doc.Call("createElement", "span")
		lbl.Set("className", "plabel")
		lbl.Set("textContent", label)
		cell.Call("appendChild", lbl)
		grp := doc.Call("createElement", "span")
		grp.Set("className", "grp")
		sel := doc.Call("createElement", "select")
		sel.Set("style", "background:#222;color:#ccc;border:1px solid #555;font-family:monospace;font-size:12px;padding:2px 4px;")
		for i, o := range opts {
			opt := doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", o)
			if i == cur {
				opt.Set("selected", true)
			}
			sel.Call("appendChild", opt)
		}
		sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if v, err := strconv.Atoi(sel.Get("value").String()); err == nil {
				onChange(v)
			}
			return nil
		}))
		grp.Call("appendChild", makeSelectorKnob(sel))
		grp.Call("appendChild", sel)
		cell.Call("appendChild", grp)
		return cell
	}
	paramsDiv.Call("appendChild", makeSel("wave", []string{"square", "pulse", "sub ÷2"}, fvfWave, func(v int) { fvfWave = v }))
	paramsDiv.Call("appendChild", makeSel("mod", []string{"ring", "AM"}, fvfMod, func(v int) { fvfMod = v }))

	// FX switch — instant A/B between the raw incoming audio (off) and the
	// wobbulated signal (on). Independent of the mix knob, and it affects
	// both what you hear and the spectrogram, so raw looks/sounds raw.
	fc := doc.Call("createElement", "span")
	fc.Set("className", "pcell")
	flbl := doc.Call("createElement", "span")
	flbl.Set("className", "plabel")
	flbl.Set("textContent", "🎛 FX")
	fc.Call("appendChild", flbl)
	fchk := doc.Call("createElement", "input")
	fchk.Set("type", "checkbox")
	fchk.Set("className", "sw")
	fchk.Set("checked", !fvfBypass)
	fchk.Set("title", "On = wobbulated (processed) audio; off = raw incoming audio, straight through")
	fchk.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		fvfBypass = !fchk.Get("checked").Bool()
		return nil
	}))
	fc.Call("appendChild", fchk)
	paramsDiv.Call("appendChild", fc)

	// Listen switch — route the wobbulated audio out the speakers.
	lc := doc.Call("createElement", "span")
	lc.Set("className", "pcell")
	llbl := doc.Call("createElement", "span")
	llbl.Set("className", "plabel")
	llbl.Set("textContent", "🔊 Listen")
	lc.Call("appendChild", llbl)
	lchk := doc.Call("createElement", "input")
	lchk.Set("type", "checkbox")
	lchk.Set("className", "sw")
	lchk.Set("checked", fvfListen)
	lchk.Set("title", "Play the wobbulated audio out the speakers (mic: use headphones; music: see the null-sink setup)")
	lchk.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		setFVFListen(lchk.Get("checked").Bool())
		return nil
	}))
	lc.Call("appendChild", lchk)
	paramsDiv.Call("appendChild", lc)
}

// ── FVF audio output engine ──────────────────────────────────────────────
// So the wobbulation can be HEARD (not just seen): a WebAudio ScriptProcessor
// created at the source's sample rate (no resampling) drains the current
// audio source, runs the FVF DSP, and plays the result. The processed samples
// also feed the spectrogram (fvfVis) so the display matches the sound. Works
// for any source — the microphone, or the PulseAudio/WebSocket stream (music),
// the latter via a null-sink so the browser's output isn't re-captured.

var (
	fvfListen       bool
	fvfAudioCtx     js.Value
	fvfAudioNode    js.Value
	fvfAudioFn      js.Func
	fvfAudioProc    *fvfProcessor
	fvfAudioActive  bool
	fvfDrainScratch []float32
	fvfVis          = newFVFRing(48000) // ~1s of processed samples for the spectrogram
)

// fvfRing is a minimal single-producer/single-consumer float32 ring.
type fvfRing struct {
	buf  []float32
	w, r int // monotonic counters
}

func newFVFRing(n int) *fvfRing { return &fvfRing{buf: make([]float32, n)} }

func (rr *fvfRing) write(s []float32) {
	n := len(rr.buf)
	for _, v := range s {
		rr.buf[rr.w%n] = v
		rr.w++
	}
}

func (rr *fvfRing) drain(dst []float32) int {
	n := len(rr.buf)
	if rr.w-rr.r > n {
		rr.r = rr.w - n
	}
	c := rr.w - rr.r
	if c > len(dst) {
		c = len(dst)
	}
	for i := 0; i < c; i++ {
		dst[i] = rr.buf[(rr.r+i)%n]
	}
	rr.r += c
	return c
}

// setFVFListen starts/stops the audio-output engine.
func setFVFListen(on bool) {
	fvfListen = on
	if on {
		startFVFAudio()
	} else {
		stopFVFAudio()
	}
}

func startFVFAudio() {
	src := ensureAudioSource()
	if src == nil {
		return
	}
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	if fvfAudioActive {
		if fvfAudioCtx.Truthy() {
			fvfAudioCtx.Call("resume")
		}
		return
	}
	ac := js.Global().Get("AudioContext")
	if ac.IsUndefined() {
		ac = js.Global().Get("webkitAudioContext")
	}
	if ac.IsUndefined() {
		return
	}
	opts := js.Global().Get("Object").New()
	opts.Set("sampleRate", sr) // context at the source rate → no resampling
	fvfAudioCtx = ac.New(opts)
	fvfAudioProc = &fvfProcessor{sampleRate: float64(sr)}
	const bufSize = 2048
	fvfDrainScratch = make([]float32, bufSize)
	fvfAudioNode = fvfAudioCtx.Call("createScriptProcessor", bufSize, 1, 1)
	fvfAudioFn = trackedFuncOf(fvfAudioProcess)
	fvfAudioNode.Set("onaudioprocess", fvfAudioFn)
	fvfAudioNode.Call("connect", fvfAudioCtx.Get("destination"))
	fvfAudioActive = true
	fvfAudioCtx.Call("resume")
}

func stopFVFAudio() {
	if !fvfAudioActive {
		return
	}
	if fvfAudioNode.Truthy() {
		fvfAudioNode.Set("onaudioprocess", js.Null())
		fvfAudioNode.Call("disconnect")
	}
	if fvfAudioCtx.Truthy() {
		fvfAudioCtx.Call("close")
	}
	fvfAudioNode, fvfAudioCtx = js.Undefined(), js.Undefined()
	fvfAudioFn.Release() // symmetric with startFVFAudio's FuncOf (was leaked per Listen toggle)
	fvfAudioActive = false
}

// fvfAudioProcess is the ScriptProcessor callback (runs in Go): drain the
// source, run FVF per sample, write the result to the output buffer and to
// fvfVis (for the spectrogram). Underflow samples are processed as silence so
// the carrier keeps running.
func fvfAudioProcess(_ js.Value, args []js.Value) interface{} {
	if !fvfAudioActive || fvfAudioProc == nil {
		return nil
	}
	outData := args[0].Get("outputBuffer").Call("getChannelData", 0)
	n := outData.Get("length").Int()
	if n > len(fvfDrainScratch) {
		n = len(fvfDrainScratch)
	}
	got := 0
	if audioSource != nil && audioSource.Ready() {
		got = audioSource.Drain(fvfDrainScratch[:n])
	}
	for i := 0; i < n; i++ {
		var x float32
		if i < got {
			x = fvfDrainScratch[i]
		}
		fvfDrainScratch[i] = fvfAudioProc.Process(x)
	}
	fvfVis.write(fvfDrainScratch[:n])
	b := unsafe.Slice((*byte)(unsafe.Pointer(&fvfDrainScratch[0])), n*4)
	u8 := js.Global().Get("Uint8Array").New(outData.Get("buffer"))
	js.CopyBytesToJS(u8, b)
	return nil
}
