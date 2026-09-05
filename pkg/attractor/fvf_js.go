//go:build js && wasm

package attractor

// FVF ("harmonic wobbulator") mode — a software analog of the
// Frequency→Voltage→Frequency converter with balanced modulator designed at
// bunkerofdoom.com, ported from
// the 0magnet/FVF prototype into chaosrack so it reuses the audio source,
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
	if src := activeAudioSource(); src != nil && src.SampleRate() > 0 {
		sr = float64(src.SampleRate())
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

// appendFVFSelectors adds the FVF-specific cells to the Parameters grid,
// using the SAME anatomy as every other module: standard .punit cards with an
// u-lbl on top, a labeled selector-ring knob (singleSelectorKnob) over a
// hidden <select> for wave/mod, and labeled switch cards for FX / Listen.
func appendFVFSelectors(grid js.Value) {
	mkSelCard := func(label, tip string, opts, ringLabels []string, cur int, onChange func(int)) js.Value {
		card := doc.Call("createElement", "div")
		card.Set("className", "punit")
		lbl := doc.Call("createElement", "span")
		lbl.Set("className", symClass("u-lbl", false))
		lbl.Set("textContent", label)
		card.Call("appendChild", lbl)
		sel := doc.Call("createElement", "select")
		sel.Set("title", tip)
		sel.Set("style", "display:none;")
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
		card.Call("appendChild", sel)
		grp := doc.Call("createElement", "span")
		grp.Set("className", "grp")
		// Two positions is a switch here for the same reason it is in a
		// parameter cell (buildParamUnit): a rotary that can only sit at one end
		// or the other costs a drag to do what a click does. These cards are
		// built by hand rather than through buildParamUnit, so the rule has to
		// be applied here too — found by sweeping every model for two-option
		// selects still wearing a knob, which turned up exactly this one.
		if len(opts) == 2 {
			grp.Call("appendChild", buildTwoWaySwitch(sel, opts))
		} else {
			grp.Call("appendChild", singleSelectorKnob(sel, ringLabels, 46))
		}
		card.Call("appendChild", grp)
		return card
	}
	grid.Call("appendChild", mkSelCard("wave",
		"Wave — carrier waveform (square / pulse / sub-octave ÷2)",
		[]string{"square", "pulse", "sub ÷2"}, []string{"sqr", "pls", "sub"},
		fvfWave, func(v int) { fvfWave = v }))
	grid.Call("appendChild", mkSelCard("mod",
		"Mod — modulator topology (ring = four-quadrant, AM = balanced)",
		[]string{"ring", "AM"}, []string{"ring", "AM"},
		fvfMod, func(v int) { fvfMod = v }))

	mkSwCard := func(label, tip string, checked bool, onChange func(bool)) js.Value {
		card := doc.Call("createElement", "div")
		card.Set("className", "punit")
		lbl := doc.Call("createElement", "span")
		lbl.Set("className", symClass("u-lbl", false))
		lbl.Set("textContent", label)
		card.Call("appendChild", lbl)
		row := doc.Call("createElement", "label")
		row.Set("className", "grp")
		row.Get("style").Set("cursor", "pointer")
		row.Get("style").Set("justifyContent", "center")
		chk := doc.Call("createElement", "input")
		chk.Set("type", "checkbox")
		chk.Set("className", "sw")
		chk.Set("checked", checked)
		chk.Set("title", tip)
		chk.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			onChange(chk.Get("checked").Bool())
			return nil
		}))
		row.Call("appendChild", chk)
		card.Call("appendChild", row)
		return card
	}
	grid.Call("appendChild", mkSwCard("FX",
		"FX — on: wobbulated (processed) audio; off: the raw incoming audio straight through (instant A/B, independent of the MIX knob; affects both sound and spectrogram)",
		!fvfBypass, func(on bool) { fvfBypass = !on }))
	grid.Call("appendChild", mkSwCard("listen",
		"Listen — play the wobbulated audio out the speakers (mic: use headphones; music: see the null-sink setup)",
		fvfListen, func(on bool) { setFVFListen(on) }))

	// The routing switch, offered only by a server that can actually do it
	// (chaosrack --audio on a machine with pactl). It is unlike every other
	// switch on this panel: FX and Listen change what the page does, this one
	// changes how the MACHINE is wired, which is why it asks the server what is
	// true instead of starting from a default of its own.
	if js.Global().Get("__crWobbulate").Truthy() {
		card := mkSwCard("route", fvfRouteTip, false, func(on bool) { setFVFRoute(on) })
		grid.Call("appendChild", card)
		fvfRouteSw = card.Call("querySelector", "input.sw")
		syncFVFRoute()
	}
}

// ── the routing switch ───────────────────────────────────────────────────
// Hearing the wobbulator on SPEAKERS (rather than headphones) needs the
// machine's audio re-routed, because the processed output would otherwise be
// captured and wobbulated again, one buffer later, forever. The server does
// that with a null sink (pkg/audioroute); this is the switch for it.
//
// It used to be a flag on a different binary, which meant switching it meant
// stopping one server, starting another and reloading the tab -- for a change
// that takes about as long as a click. The server's half is a POST; this is the
// click.

const fvfRouteURL = "/audio/wobbulate"

const fvfRouteTip = "Route — send ALL system audio through a temporary null sink on the server's machine, " +
	"so the wobbulated result can be played out the speakers without being captured and wobbulated again. " +
	"Turn it on, play something in any app, then turn on Listen. Off restores the previous default sink; " +
	"so does stopping the server. Only a page on the same machine can switch it."

// fvfRouteSw is the checkbox itself, kept so the server's answer can move it.
// The routing is not this page's state to remember: another tab, the --wobbulate
// flag, or the operator's own pactl can all have changed it.
var fvfRouteSw js.Value

// setFVFRoute asks the server to install or remove the routing.
func setFVFRoute(on bool) {
	body := `{"on":false}`
	if on {
		body = `{"on":true}`
	}
	headers := js.Global().Get("Object").New()
	// Not decoration: an application/json body is what makes this a request the
	// browser has to ask permission for before sending it cross-site. The server
	// answers no preflight, so no other page can flip the machine's audio.
	headers.Set("Content-Type", "application/json")
	opts := js.Global().Get("Object").New()
	opts.Set("method", "POST")
	opts.Set("headers", headers)
	opts.Set("body", body)
	fetchJSONOnce(fvfRouteURL, opts, func(ok bool, b js.Value) { applyFVFRoute(ok, b, on) })
}

// syncFVFRoute puts the switch where the machine actually is, at panel-build
// time.
func syncFVFRoute() {
	fetchJSONOnce(fvfRouteURL, js.Undefined(), func(ok bool, b js.Value) { applyFVFRoute(ok, b, false) })
}

// applyFVFRoute moves the switch to whatever the server reports, and puts it
// BACK when a request was refused.
//
// Back matters. A switch that stays where the click left it claims a routing
// that was never installed, and the symptom of believing that is silence with
// no explanation -- the exact failure this whole feature exists to avoid.
func applyFVFRoute(ok bool, body js.Value, wanted bool) {
	if !fvfRouteSw.Truthy() {
		return
	}
	if ok && body.Truthy() {
		fvfRouteSw.Set("checked", body.Get("on").Bool())
		fvfRouteSw.Set("title", fvfRouteTip)
		return
	}
	fvfRouteSw.Set("checked", !wanted)
	msg := "no answer from the server"
	if body.Truthy() && body.Get("error").Truthy() {
		msg = body.Get("error").String()
	}
	fvfRouteSw.Set("title", fvfRouteTip+"\n\nlast attempt failed: "+msg)
	js.Global().Get("console").Call("warn", "[chaosrack] audio routing: "+msg)
}

// fetchJSONOnce runs one fetch and calls done exactly once with the decoded
// body, whichever way the promise settles.
//
// The callbacks are released by hand rather than through trackedFuncOf because
// a promise settles on its own schedule: the panel can be rebuilt (and every
// tracked func released) between the request and the answer, and a released func
// that JavaScript then calls is a runtime error, not a no-op. Releasing in the
// handler that fires ties their lifetime to the request instead of the panel.
func fetchJSONOnce(url string, opts js.Value, done func(ok bool, body js.Value)) {
	var onResp, onJSON, onErr js.Func
	released := false
	settle := func(ok bool, body js.Value) {
		if released {
			return
		}
		released = true
		onResp.Release()
		onJSON.Release()
		onErr.Release()
		done(ok, body)
	}
	respOK := false
	onErr = js.FuncOf(func(js.Value, []js.Value) interface{} {
		settle(false, js.Undefined())
		return nil
	})
	onJSON = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		body := js.Undefined()
		if len(args) > 0 {
			body = args[0]
		}
		settle(respOK, body)
		return nil
	})
	onResp = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			settle(false, js.Undefined())
			return nil
		}
		// A refusal still carries JSON saying why, so the body is read either
		// way and "ok" is remembered rather than inferred from it.
		respOK = args[0].Get("ok").Bool()
		args[0].Call("json").Call("then", onJSON).Call("catch", onErr)
		return nil
	})
	js.Global().Call("fetch", url, opts).Call("then", onResp).Call("catch", onErr)
}

// ── FVF audio output engine ──────────────────────────────────────────────
// So the wobbulation can be HEARD (not just seen): a WebAudio ScriptProcessor
// on the SHARED context drains the current audio source, runs the FVF DSP per
// source-rate sample, and linearly upsamples the processed stream to the
// context rate for playback (the shared context runs at the hardware rate, so
// a 24 kHz ws feed can't get a private matched-rate context any more). The
// processed source-rate samples also feed the spectrogram (fvfVis) so the
// display matches the sound. Works for any source — the microphone, or the
// PulseAudio/WebSocket stream (music), the latter via a null-sink so the
// browser's output isn't re-captured.

var (
	fvfListen       bool
	fvfAudioCtx     js.Value
	fvfAudioNode    js.Value
	fvfAudioFn      js.Func
	fvfAudioProc    *fvfProcessor
	fvfAudioActive  bool
	fvfDrainScratch []float32           // source-rate: drained input, then processed output
	fvfOutScratch   []float32           // context-rate: upsampled playback samples
	fvfSrcAcc       float64             // fractional source samples owed to the resampler
	fvfResampLast   float32             // last processed sample (interp continuity across callbacks)
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
		acquireAudioCtx("fvf")
		return
	}
	fvfAudioCtx = acquireAudioCtx("fvf")
	if !fvfAudioCtx.Truthy() {
		return
	}
	fvfAudioProc = &fvfProcessor{sampleRate: float64(sr)}
	const bufSize = 2048
	// The source-rate scratch must hold bufSize·(srcRate/ctxRate) samples;
	// 2× covers any plausible rate pair (e.g. 96 kHz source on a 48 kHz ctx).
	fvfDrainScratch = make([]float32, 2*bufSize)
	fvfOutScratch = make([]float32, bufSize)
	fvfSrcAcc, fvfResampLast = 0, 0
	fvfAudioNode = fvfAudioCtx.Call("createScriptProcessor", bufSize, 1, 1)
	fvfAudioFn = trackedFuncOf(fvfAudioProcess)
	fvfAudioNode.Set("onaudioprocess", fvfAudioFn)
	fvfAudioNode.Call("connect", fvfAudioCtx.Get("destination"))
	fvfAudioActive = true
}

func stopFVFAudio() {
	if !fvfAudioActive {
		return
	}
	if fvfAudioNode.Truthy() {
		fvfAudioNode.Set("onaudioprocess", js.Null())
		fvfAudioNode.Call("disconnect")
	}
	fvfAudioNode, fvfAudioCtx = js.Undefined(), js.Undefined()
	fvfAudioFn.Release() // symmetric with startFVFAudio's FuncOf (was leaked per Listen toggle)
	fvfAudioActive = false
	releaseAudioCtx("fvf")
}

// fvfAudioProcess is the ScriptProcessor callback (runs in Go): drain the
// source, run FVF per source-rate sample, then upsample the processed block
// to the context rate for playback. fvfVis gets the source-rate stream (the
// spectrogram's scroll pacing is derived from the source rate). Underflow
// samples are processed as silence so the carrier keeps running.
func fvfAudioProcess(_ js.Value, args []js.Value) interface{} {
	if !fvfAudioActive || fvfAudioProc == nil {
		return nil
	}
	out := args[0].Get("outputBuffer")
	outData := out.Call("getChannelData", 0)
	n := outData.Get("length").Int()
	if n > len(fvfOutScratch) {
		n = len(fvfOutScratch)
	}

	// How many source-rate samples this context-rate block spans. The source
	// rate can settle late (ws connect after Listen), so track it live.
	srcRate := 24000.0
	if src := activeAudioSource(); src != nil && src.SampleRate() > 0 {
		srcRate = float64(src.SampleRate())
	}
	fvfAudioProc.sampleRate = srcRate
	ctxRate := out.Get("sampleRate").Float()
	fvfSrcAcc += srcRate / ctxRate * float64(n)
	m := int(fvfSrcAcc)
	if m > len(fvfDrainScratch) {
		m = len(fvfDrainScratch)
	}
	fvfSrcAcc -= float64(m)

	got := 0
	if src := activeAudioSource(); src != nil && src.Ready() {
		got = src.Drain(fvfDrainScratch[:m])
	}
	for i := 0; i < m; i++ {
		var x float32
		if i < got {
			x = fvfDrainScratch[i]
		}
		fvfDrainScratch[i] = fvfAudioProc.Process(x)
	}
	fvfVis.write(fvfDrainScratch[:m])

	// Linear-interpolate the m processed samples up to n output samples,
	// with fvfResampLast carrying continuity across callback boundaries.
	seq := func(k int) float32 {
		if k <= 0 {
			return fvfResampLast
		}
		return fvfDrainScratch[k-1]
	}
	step := float64(m) / float64(n)
	for i := 0; i < n; i++ {
		pos := float64(i) * step
		j := int(pos)
		frac := float32(pos - float64(j))
		a := seq(j)
		fvfOutScratch[i] = a + frac*(seq(j+1)-a)
	}
	if m > 0 {
		fvfResampLast = fvfDrainScratch[m-1]
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(&fvfOutScratch[0])), n*4) //nolint:gosec // reinterpreting a typed slice as its backing bytes to cross into JS without a second copy
	u8 := js.Global().Get("Uint8Array").New(outData.Get("buffer"))
	js.CopyBytesToJS(u8, b)
	return nil
}
