//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/led"
)

// The Waterfall mode — cumulative spectral decay, as a surface in the 3-D
// pipeline.
//
// The analysis is in pkg/acoustics and checked against a synthetic
// resonance and a decay of known T60. This is the picture, and it is the one
// display here that could only exist in this app: every other analyzer in the
// rack draws its own flat panel, and this one is a genuine three-dimensional
// surface, so it goes through the SAME vertex pipeline the attractors do and
// gets the camera, the drag-to-rotate, the gradient, the persist painting and
// Model Out for nothing.
//
// Frequency runs left to right (logarithmically, because hearing is organized
// in ratios), level up, and TIME INTO THE SCREEN. Each line is one slice: the
// spectrum of what is left of the impulse response from a moment onwards. A
// flat loudspeaker's surface falls away evenly; a resonance is a ridge running
// back into the screen at one frequency, which is the thing a frequency
// response cannot show and this exists for.
//
// ── IT IS A MEASUREMENT, SO IT HOLDS STILL ───────────────────────────────
//
// Unlike every other audio mode here, the picture does not move with the sound.
// An impulse response is a measurement of a system rather than a view of a
// signal: it is made from one sweep pass and then it is the answer until
// another is made. So the surface is recomputed when a pass completes and holds
// between them, which is also what makes it something you can rotate and look
// at rather than something you have to freeze first.
//
// The surface, its knobs and its measurement are Waterfall
// (waterfallsurface.go), which a native rack draws too. This is the page's
// side: reading the tap, the vertices, the camera, and the RT60 readout.

// wfallFFTRing labels the FFT knob's positions, wfallFFTSizes.
var wfallFFTRing = []string{"1k", "2k", "4k", "8k"}

// waterfall is the Waterfall mode: the surface, the sweep it captures, the
// live surface's window of the tap, and the readout.
type waterfall struct {
	Waterfall
	cursor  int
	refBuf  []float32 // the reference: what went out
	measBuf []float32 // the measurement: what came back
	fill    int

	// The live surface's own tap cursor and window of audio.
	liveCursor int
	liveBuf    []float32
	liveFill   int

	// fitted is the one-shot camera fit, as the audio embeddings have.
	fitted bool
	rtEl   js.Value
	rtTx   string
}

var wfall = waterfall{
	cursor:     tapUnjoined,
	liveCursor: tapUnjoined,
	Waterfall:  *NewWaterfall(),
}

func init() {
	registerGenerate("waterfall", wfall.generate)
	attractorParams["waterfall"] = []paramDef{
		{"wfall-src", "src", &wfall.srcF, 0, 0, 1, 1},
		{"wfall-chan", "chan", &wfall.chanF, 0, 0, float32(len(tapChanNames) - 1), 1},
		{"wfall-lines", "line", &wfall.linesF, 16, 4, 64, 1},
		{"wfall-step", "step", &wfall.stepF, 5, 1, 100, 1},
		{"wfall-fft", "fft", &wfall.fftF, 1, 0, float32(len(wfallFFTSizes) - 1), 1},
		{"wfall-top", "top", &wfall.topF, 0, -60, 20, 1},
		{"wfall-range", "rnge", &wfall.rangeF, 40, 10, 80, 5},
		{"wfall-depth", "dpth", &wfall.depthF, 1, 0.2, 3, 0.1},
		{"wfall-swap", "ref", &wfall.swapF, 0, 0, 1, 1},
	}
}

// applyDefaults moves LINE, STEP and FFT to what d wants, where nobody chose
// otherwise (Waterfall.useDefaults), turning the panel's knobs.
func (w *waterfall) applyDefaults(d wfallDefaults) { w.useDefaults(d, setWfallKnob) }

// setWfallKnob writes a value into a knob, rather than only into the variable
// behind it: the hidden range input is the value, and its input event is what
// repaints the dial and the LED. Writing the variable alone would draw the new
// surface under a knob still showing the old number.
func setWfallKnob(id string, ptr *float32, v float32) {
	*ptr = v
	el := dom.Doc.Call("getElementById", id)
	if !el.Truthy() {
		return
	}
	el.Set("value", strconv.FormatFloat(float64(v), 'f', -1, 32))
	dom.Fire(el, "input")
}

// generate runs whichever surface SRC names, and draws it.
func (w *waterfall) generate() {
	defer w.showRT()
	if w.Switched(setWfallKnob) {
		w.liveFill = 0
		w.armFit()
	}
	if w.Live() {
		w.liveTick()
	} else {
		w.capture()
	}
	w.draw()
}

// liveTick drains the tap into the live window and pushes a slice of it when
// one is due (Waterfall.PushLive).
func (w *waterfall) liveTick() {
	if len(w.liveBuf) != w.fftLen() {
		w.liveBuf = make([]float32, w.fftLen())
		w.liveFill = 0
		w.DropSurface()
	}
	// Drain the tap into the window, keeping the newest LiveFFT samples.
	// Drained EVERY frame even though a slice is only pushed every 40 ms: the
	// ring is finite, and a reader that only reads when it wants a slice falls
	// behind and then jumps, which is a surface built from audio that is not
	// adjacent to itself.
	var blk [4096]float32
	for {
		n := tap.readChan(&w.liveCursor, blk[:], w.channel())
		if n <= 0 {
			break
		}
		if n >= len(w.liveBuf) {
			copy(w.liveBuf, blk[n-len(w.liveBuf):n])
			w.liveFill = len(w.liveBuf)
		} else {
			copy(w.liveBuf, w.liveBuf[n:])
			copy(w.liveBuf[len(w.liveBuf)-n:], blk[:n])
			if w.liveFill += n; w.liveFill > len(w.liveBuf) {
				w.liveFill = len(w.liveBuf)
			}
		}
		if n < len(blk) {
			break
		}
	}
	if w.liveFill < len(w.liveBuf) {
		return
	}
	w.PushLive(w.liveBuf, takensSourceRate(), frameNowMs)
}

// capture keeps the rolling stereo buffer and measures each time the
// generator's sweep comes round (Waterfall.MeasureDue).
func (w *waterfall) capture() {
	sr := takensSourceRate()
	want := sr * WaterfallCaptureSec
	if len(w.refBuf) != want {
		w.refBuf = make([]float32, want)
		w.measBuf = make([]float32, want)
		w.fill = 0
	}
	var sl, srr [4096]float32
	for {
		n := tap.readStereo(&w.cursor, sl[:], srr[:])
		if n <= 0 {
			break
		}
		ref, meas := sl[:n], srr[:n]
		if w.Swapped() {
			ref, meas = srr[:n], sl[:n]
		}
		if n >= want {
			copy(w.refBuf, ref[n-want:])
			copy(w.measBuf, meas[n-want:])
			w.fill = want
		} else {
			copy(w.refBuf, w.refBuf[n:])
			copy(w.measBuf, w.measBuf[n:])
			copy(w.refBuf[want-n:], ref)
			copy(w.measBuf[want-n:], meas)
			if w.fill += n; w.fill > want {
				w.fill = want
			}
		}
		if n < len(sl) {
			break
		}
	}
	if w.fill < want {
		return
	}
	if w.MeasureDue(aud.rackBus().SweepPosition(), frameNowMs) {
		w.Measure(w.refBuf, w.measBuf, sr)
	}
}

// draw builds the surface's vertices and hands them to the normal pipeline.
//
// As LINES rather than a line strip: the slices are separate curves, and a
// strip would draw a diagonal from the end of each one back to the start of the
// next — sixteen bright diagonals across a surface, which is the retrace a
// scope shows and not something a measurement should.
func (w *waterfall) draw() {
	n := w.Slices()
	if n == 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Lines, 0)
		return
	}
	bins := len(wfallFreqs)
	need := n * (bins - 1) * 2
	if need*4 > len(sim.vertBuf) {
		// More than the trail budget allows. The budget is a knob, so this is
		// a real possibility rather than a theoretical one, and drawing fewer
		// slices is a better answer than drawing a corrupt surface.
		n = len(sim.vertBuf) / 4 / ((bins - 1) * 2)
		if n < 1 {
			gpu.uploadVerticesOnly(sim.vertBuf[:0], glctx.Types.Lines, 0)
			return
		}
		need = n * (bins - 1) * 2
	}
	v := sim.vertBuf[:need*4]
	o := 0
	w.eachSegment(n, func(x, y, z, age float32) {
		v[o], v[o+1], v[o+2], v[o+3] = x, y, z, age
		o += 4
	})
	gpu.uploadVerticesOnly(v, glctx.Types.Lines, need)
	// The surface's bounds are exact rather than measured: x is the frequency
	// axis end to end, y is the whole of TOP..TOP-RNGE because the level is
	// clamped into it, and z is the depth DPTH asked for. Setting them is what
	// makes the colormap span the surface instead of whatever range the model
	// drawn before this one happened to occupy.
	//
	// Y is the source worth turning the Colors ring to here, and the reason is
	// this line: with the extents exact, Y colors by LEVEL across exactly the
	// decibels the scale shows, so a ridge is the hot end of the map and the
	// floor is the cold end. Z and trail both color by age, which is worth
	// having and is the default. X repeats the frequency axis. AUDIO is the one
	// to avoid: its table is a short-time centroid along the trail and only
	// takens, stereo and polar fill it, so on this mode it is one flat tint.
	span, depth := float32(WaterfallSpan), float32(w.Depth())
	gpu.setGradientRange(-span, span, -span/2, span/2, -depth/2, depth/2)
	if !w.fitted {
		// Fitted to the surface's OWN half-span, which is exactly what its
		// largest coordinate reaches — autoFitCamera measures extent as
		// max|coordinate| and the frequency axis is the widest of the three. The
		// 1.2 that used to pad it was reserving room for nothing: the bound here
		// is exact rather than a worst case, unlike the audio embeddings where
		// the figure's size depends on the signal.
		//
		// The surface still leaves room either side, because the camera frames a
		// SPHERE of that radius and the window is wider than it is tall. That is
		// how every model here is framed and the zoom is how to fill the width.
		w.fitted = true
		view.fitOverride = WaterfallSpan
		view.autoFitCamera()
	}
}

// armFit re-arms it, for a mode change.
func (w *waterfall) armFit() { w.fitted = false }

// showRT writes the reverberation time beside the knobs.
//
// The one number the surface does not show and cannot: it is eighty
// milliseconds deep, which is where a loudspeaker's resonances live, and a
// room's decay is measured over seconds. It was being computed from the same
// impulse response and then thrown away, so the README described a readout
// that was not on screen.
//
// T20 extrapolated, from -5 dB to -25 dB: the first 5 dB is the direct sound
// and the last of a real decay is in the noise, so the standard measures the
// straight part between them and multiplies up. Blank in the live surface,
// which is spectra of a signal and has no impulse to decay from.
func (w *waterfall) showRT() {
	s := "--- s"
	if rt, ok := w.RT60(); ok {
		s = led.Format(rt, 1, 2, false) + " s"
	}
	if s == w.rtTx {
		return
	}
	w.rtTx = s
	if w.rtEl.Truthy() {
		setDotText(w.rtEl, s)
	}
}

// appendReadout adds the RT60 readout to the model's readout line
// (liveReadoutHost).
func (w *waterfall) appendReadout(host js.Value) {
	w.rtTx = ""
	w.rtEl = liveReadout(host, "rt60", dispFullChars, "--- s", doc("ro.rt60"))
}

// wfallFFTLabels is the FFT knob's per-position tooltip: the size and what it
// buys, because the number alone does not say which way the trade runs.
var wfallFFTLabels = []string{
	doc("p.wfall-fft=0"),
	doc("p.wfall-fft=1"),
	doc("p.wfall-fft=2"),
	doc("p.wfall-fft=3"),
}
