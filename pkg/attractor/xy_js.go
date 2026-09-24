//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// The xy scope mode draws (L[i], R[i]) as a line strip on the shared
// #gocanvas — classic oscilloscope-style Lissajous. Mono sources use a
// lag on the same channel to still produce a useful figure (pure
// diagonal on a raw mono signal is not useful; a lagged copy adds an
// orbit).
//
// Own shader program (pass-through vertex + solid-color fragment) with a
// dedicated dynamic-vertex buffer sized to the sample window.
//
// ── THE CONTROLS ─────────────────────────────────────────────────────────
//
// This mode had none. Its window, its deflection, its mono lag and its beam
// smoothing were compile-time constants, and the Parameters module did not even
// appear for it — which is an odd place to end up for the display with the
// highest name recognition in the app, and the one an audio engineer arrives
// looking for. Every one of those constants is now a knob, and the two things
// a hardware goniometer has that this did not — a basis switch and a
// persistence control — are knobs beside them.
//
//	BASIS   L/R or mid/side. M=(L+R)/2 and S=(L−R)/2 are the same plane turned
//	        45°, which is the orientation broadcast goniometers ship in: center
//	        content lies along one axis and difference content along the other,
//	        so "how wide is this" is an extent rather than the eccentricity of a
//	        tilted ellipse. The ½ is what keeps switching basis from resizing
//	        the figure — see stereoChanValue, whose reasoning this shares.
//
//	GAIN    A multiplier on the deflection. The scope drew everything at 0.9 of
//	        the screen and nothing could be done about it, so a quiet source was
//	        a dot in the middle. Clipping is not a hazard the way it is on a
//	        tube: a figure driven past the edge is simply drawn outside the
//	        viewport, and turning the knob back brings it in.
//
//	WIN     The time base, in milliseconds, replacing a fixed 2048 samples.
//	        Short is a single cycle of something low and the instantaneous phase
//	        relationship; long is many cycles overlaid, which is how the width
//	        of a whole mix reads.
//
//	PERSIST The afterglow. A hardware vectorscope's most-used control and the
//	        one this could not do: the mode clears every frame, so a phosphor
//	        selected in the Style module gave the trace its COLOR and no glow at
//	        all. At zero the frame is cleared as before; above it the frame is
//	        multiplied down each frame instead, so the trace decays rather than
//	        vanishing and a transient leaves a trail that can actually be read.
//
//	LAG     How far a mono source is delayed against itself, in milliseconds
//	        rather than in samples, so it means the same thing on every source.
//
//	SMOOTH  The Catmull-Rom upsample. An analog beam is slew-limited and never
//	        draws a corner; this is how much of that to imitate.
//
// CORR is the correlation meter — the number beside every hardware goniometer,
// and the one reading the figure alone cannot give you, because a thin ellipse
// and a line are the same picture at a glance. It shares stereoCorrelation with
// the Stereo Embedding rather than computing its own: it is the same quantity
// over the same kind of window, and two implementations of Pearson's r would be
// two things to keep in step.

const xyVertShaderSrc = `
	attribute vec2 aPos;
	uniform vec2 uOffset;
	void main() {
		gl_Position = vec4(aPos + uOffset, 0.0, 1.0);
	}
`

const xyFragShaderSrc = `
	precision mediump float;
	uniform vec3 uColor;
	uniform float uAlpha;
	void main(void) {
		gl_FragColor = vec4(uColor, uAlpha);
	}
`

// xyDeflection is the per-axis scale from a full-scale sample to clip space,
// and it is TWO numbers because the canvas is not square.
//
// The vertex shader writes gl_Position straight from the sample pair, so a
// coordinate of 1 is the edge of the viewport in whichever direction it is
// written — which on a 1916x998 canvas is 958 pixels across and 499 pixels up.
// A goniometer whose two deflection sensitivities differ by 1.92x is not
// telling the truth about anything it exists to show: the L=R diagonal that
// every mastering engineer reads as "mono" lands at atan(998/1916) = 27.5
// degrees instead of 45, the circle a 90-degree phase difference draws comes
// out as a wide ellipse, and the eccentricity that means "phase" is
// indistinguishable from the eccentricity the window shape put there.
//
// So the scope is squared against the SHORTER side: the full range stays on
// screen in both directions and the surplus on the long axis is left as
// margin, rather than cropping the trace to fill it. Every other mode is
// already isotropic — they go through mgl32.Perspective, which takes the
// aspect ratio — so this is the one display that had to be told.
func xyDeflection() (sx, sy float32) {
	if gpu.width <= 0 || gpu.height <= 0 {
		return xyScale, xyScale
	}
	if gpu.width > gpu.height {
		return xyScale * float32(gpu.height) / float32(gpu.width), xyScale
	}
	return xyScale, xyScale * float32(gpu.width) / float32(gpu.height)
}

// xyScope is the XY scope mode: its program, the stereo window it draws, and
// the correlation readout.
type xyScope struct {
	program js.Value
	buf     js.Value
	aPos    js.Value
	uColor  js.Value
	uAlpha  js.Value
	uOffset js.Value
	ready   bool
	window  int // current window, in samples; derived from the WIN knob
	bufL    []float32
	bufR    []float32
	line    []float32 // interleaved x,y pairs for GL upload (smoothing per sample)
	jsUint8 js.Value  // persistent upload scratch (byte view)
	jsFloat js.Value  // persistent upload scratch (float32 view)

	// The knobs. xyWinMS and xyLagMS are durations rather than sample counts for
	// takens.TauSamples' reason: a sample is not a fixed amount of time, and the same
	// setting would otherwise mean one thing on the 48 kHz microphone and another
	// on the 24 kHz server feed.
	basisF   float32  // 0 = L/R, 1 = mid/side
	gain     float32  // multiplier on the deflection
	winMS    float32  // time base, ms (2048 samples at 48 kHz: what it was)
	persist  float32  // afterglow, 0 = clear every frame as before
	lagMS    float32  // mono self-delay, ms (128 samples at 48 kHz)
	smoothF  float32  // Catmull-Rom steps per sample interval
	corrEl   js.Value // the correlation readout in the parameter grid
	corrText string   // last text written to it (DOM write only on change)
	monoSrc  bool
	corrOK   bool
	corr     float32
}

var xy = xyScope{
	window:  2048,
	gain:    1,
	winMS:   43,
	lagMS:   2.67,
	smoothF: 4,
}

var (
	xyScale float32 = 0.9
)

// xyBasisNames are the dial's positions by name, and xyBasisRing what fits
// around it — the same arrangement as the Stereo Embedding's axes knob, and
// the same reason: for a named setting the ring IS the readout, because seven
// segments cannot spell a word.
var (
	xyBasisNames = []string{"L, R", "mid, side"}
	xyBasisRing  = []string{"LR", "MS"}
)

// xyWinMax is the WIN knob's ceiling, in milliseconds. The window is a SNAPSHOT
// — TimeDomainStereo fills it from the source's ring — so it cannot exceed what
// the ring holds without coming back silently wrapped, which is the trap
// audiosrc.DefaultRingSize is named and exported to describe. 250 ms is 12000
// samples at 48 kHz, comfortably inside the 16384-sample ring, and it is also
// past the point where a phase display is a filled blob whatever the source is
// doing — the Stereo Embedding's WIN tops out here for both of the same reasons.
const xyWinMax = 250

func init() {
	attractorParams["xy"] = []paramDef{
		{"xy-basis", "axes", &xy.basisF, 0, 0, float32(len(xyBasisNames) - 1), 1},
		{"xy-gain", "gain", &xy.gain, 1, 0.1, 8, 0.1},
		{"xy-win", "win", &xy.winMS, 43, 2, xyWinMax, 1},
		{"xy-persist", "glow", &xy.persist, 0, 0, 0.98, 0.02},
		{"xy-lag", "lag", &xy.lagMS, 2.67, 0.1, 25, 0.1},
		{"xy-smooth", "smth", &xy.smoothF, 4, 1, 16, 1},
	}
}

// xySmoothSel is the SMOOTH knob as a step count, clamped. Audio modulation can
// drive any registered parameter, so the value arriving here is not necessarily
// on the dial — and a modulator riding a feature that has gone to zero or to
// infinity can hand over an out-of-range float or a NaN. The range is checked
// BEFORE the conversion, because a float-to-int conversion whose value does not
// fit is implementation-defined in Go (this is stereoAxisSel's argument).
func (x *xyScope) smoothSel() int {
	v := x.smoothF
	if !(v > 1) { // false for NaN too
		return 1
	}
	if v > 16 {
		return 16
	}
	return int(v + 0.5)
}

// xyIsMidSide reports the basis knob's position, by the same clamp.
func (x *xyScope) isMidSide() bool { return x.basisF > 0.5 }

// xyWindowSamples converts the WIN knob into a sample count, clamped to what
// one snapshot may ask the source for. The clamp is on the DURATION, so the
// only casualty is milliseconds that were asked for and cannot be had.
func xyWindowSamples(winMS float32, sr int) int {
	if sr <= 0 {
		sr = 48000
	}
	if maxMS := float32(xySpanMax) / float32(sr) * 1000; winMS > maxMS {
		winMS = maxMS
	}
	n := int(winMS / 1000 * float32(sr))
	if n < 64 {
		n = 64
	}
	return n
}

// xySpanMax is the most samples one snapshot may ask for, named against the
// source's own constant rather than repeating the number so the two cannot
// drift apart.
const xySpanMax = audiosrc.DefaultRingSize

// xyLagSamples is the LAG knob in source samples, at least one — a zero lag is
// the raw mono diagonal this exists to avoid.
func xyLagSamples(lagMS float32, sr int) int {
	if sr <= 0 {
		sr = 48000
	}
	if !(lagMS > 0) {
		return 1
	}
	n := int(lagMS / 1000 * float32(sr))
	if n < 1 {
		n = 1
	}
	return n
}

func (x *xyScope) initXY() {
	if x.ready {
		return
	}
	vs := glctx.GL.Call("createShader", glctx.Types.VertexShader)
	glctx.GL.Call("shaderSource", vs, xyVertShaderSrc)
	glctx.GL.Call("compileShader", vs)
	fs := glctx.GL.Call("createShader", glctx.Types.FragmentShader)
	glctx.GL.Call("shaderSource", fs, xyFragShaderSrc)
	glctx.GL.Call("compileShader", fs)

	x.program = glctx.GL.Call("createProgram")
	glctx.GL.Call("attachShader", x.program, vs)
	glctx.GL.Call("attachShader", x.program, fs)
	glctx.GL.Call("linkProgram", x.program)

	x.aPos = glctx.GL.Call("getAttribLocation", x.program, "aPos")
	x.uColor = glctx.GL.Call("getUniformLocation", x.program, "uColor")
	x.uAlpha = glctx.GL.Call("getUniformLocation", x.program, "uAlpha")
	x.uOffset = glctx.GL.Call("getUniformLocation", x.program, "uOffset")

	x.buf = glctx.GL.Call("createBuffer")
	x.ready = true
	x.fitBuffers(x.window, x.smoothSel())
}

// xyFitBuffers sizes the sample buffers, the line buffer and the upload scratch
// for a window and a smoothing factor, and does nothing when they already fit.
//
// The buffers used to be allocated once at the size of a constant. Both numbers
// are knobs now, and the upload scratch is a JS typed array over a Go-side
// buffer — so growing the line without rebuilding the pair leaves the scratch
// describing a shorter run of memory than the copy writes, which is not a
// resize bug that shows up as a small figure. They grow by half again so that
// dragging the WIN dial does not reallocate on every pixel of the drag, and
// they never shrink: the largest window a session has used is a fair guess at
// what it will use again, and the buffer at the knob's ceiling is 1.5 MB.
func (x *xyScope) fitBuffers(win, smooth int) {
	if win < 2 {
		win = 2
	}
	if len(x.bufL) < win {
		grown := win + win/2
		x.bufL = make([]float32, grown)
		x.bufR = make([]float32, grown)
	}
	if need := win * smooth * 2; len(x.line) < need {
		x.line = make([]float32, need+need/2)
		// Persistent upload scratch — the trace re-uploads every frame, and a
		// fresh typed array per frame is steady GC pressure (same pattern as
		// gpu.vertU8/gpu.vertF32).
		x.jsUint8 = js.Global().Get("Uint8Array").New(len(x.line) * 4)
		x.jsFloat = js.Global().Get("Float32Array").New(x.jsUint8.Get("buffer"), 0, len(x.line))
	}
}

// renderXYFrame is the full-screen xy-scope MODE: clear the canvas, then draw.
func renderXYFrame() { xy.drawXYScope(true) }

// drawXYScope fills the sample window and draws the Lissajous line strip. When
// clear is true (xy MODE) it clears the canvas first; when false (xy used as a
// BACKGROUND behind an attractor) it draws onto whatever is already there, so
// the caller controls clearing and the attractor can be layered on top.
func (x *xyScope) drawXYScope(clear bool) {
	if !x.ready {
		x.initXY()
	}
	src := aud.ensureAudioSource()
	sr := 48000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	smooth := x.smoothSel()
	x.window = xyWindowSamples(x.winMS, sr)
	x.fitBuffers(x.window, smooth)
	drawn := x.window * smooth

	if src != nil && src.Ready() {
		l, r := x.bufL[:x.window], x.bufR[:x.window]
		src.TimeDomainStereo(l, r)
		// If the source only has one channel, fall back to a lagged
		// pseudo-stereo so the trace isn't a straight diagonal.
		mono := src.Channels() < 2
		if mono {
			lag := xyLagSamples(x.lagMS, sr)
			// Backwards, so a sample is read before the same index is written:
			// forwards, the lagged copy would be built out of samples this loop
			// had already replaced, which for a lag shorter than the window is
			// a feedback comb rather than a delay.
			for i := x.window - 1; i >= 0; i-- {
				if j := i - lag; j < 0 {
					r[i] = 0
				} else {
					r[i] = l[j]
				}
			}
		}
		// The correlation before the basis rotation, because it is a property
		// of the CHANNELS: in mid/side the two axes are constructed to be
		// uncorrelated for centered material, so measuring there would report
		// the rotation rather than the source. A mono source has nothing to
		// correlate and says so instead.
		if mono {
			x.noteState(true, false, 0)
		} else {
			corr, ok := stereoCorrelation(l, r)
			x.noteState(false, ok, corr)
		}
		// Catmull-Rom upsample: `smooth` spline steps per sample segment, per
		// axis, so the beam curves through the samples (analog slew) instead
		// of cornering at every one.
		clampIdx := func(i int) int {
			if i < 0 {
				return 0
			}
			if i >= x.window {
				return x.window - 1
			}
			return i
		}
		// The basis is applied to the SAMPLES, before the spline, so the spline
		// interpolates the axes actually being drawn. Rotating afterwards would
		// give the same answer here — the rotation is linear and Catmull-Rom is
		// a linear combination of its control points, so the two commute — but
		// only while the basis stays linear, and doing it first costs nothing.
		ms := x.isMidSide()
		ax := func(i int) (float32, float32) {
			a, b := l[i], r[i]
			if ms {
				return (a + b) * 0.5, (a - b) * 0.5
			}
			return a, b
		}
		sx, sy := xyDeflection()
		sx *= x.gain
		sy *= x.gain
		o := 0
		for i := 0; i < x.window; i++ {
			l0, r0 := ax(clampIdx(i - 1))
			l1, r1 := ax(i)
			l2, r2 := ax(clampIdx(i + 1))
			l3, r3 := ax(clampIdx(i + 2))
			for s := 0; s < smooth; s++ {
				t := float32(s) / float32(smooth)
				x.line[o] = catmullRom(l0, l1, l2, l3, t) * sx
				x.line[o+1] = catmullRom(r0, r1, r2, r3, t) * sy
				o += 2
			}
		}
	} else {
		// Blank the line so we don't draw stale data.
		for i := 0; i < drawn*2; i++ {
			x.line[i] = 0
		}
		x.noteState(false, false, 0)
	}

	// Draw as a flat 2D trace (no depth), so as a background it never occludes
	// or z-fights the attractor layered on top.
	glctx.GL.Call("disable", glctx.Types.DepthTest)
	if clear {
		if k := x.persistK(); k > 0 {
			// PERSIST: multiply the frame down instead of clearing it, so the
			// trace decays over several frames the way a phosphor does. Only on
			// the self-clearing path — as a BACKDROP the scope draws onto a
			// buffer somebody else owns and the model on top of it is redrawn
			// whole every frame, so fading here would smear that instead.
			phos.drawFadeQuad(k, k, k)
			glctx.GL.Call("disable", glctx.Types.DepthTest)
		} else {
			// Transparent clear (alpha 0), like every other mode — so with "Front" on
			// (canvas layered over the panel) the scope shows its trace over the
			// controls instead of an opaque black block that hides the panel and can't
			// be undone.
			glctx.GL.Call("clearColor", 0, 0, 0, 0)
			glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
		}
	}

	glctx.GL.Call("useProgram", x.program)
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, x.buf)
	js.CopyBytesToJS(x.jsUint8, sliceToByteSlice(x.line))
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, x.jsFloat, glctx.Types.DynamicDraw)
	glctx.GL.Call("enableVertexAttribArray", x.aPos)
	glctx.GL.Call("vertexAttribPointer", x.aPos, 2, glctx.Types.Float, false, 0, 0)

	col := [3]float32{0.4, 1.0, 0.45}
	if phos.active() { // scope mode → trace in the selected phosphor color
		p := phosphors[phos.index]
		col = [3]float32{float32(p.tr), float32(p.tg), float32(p.tb)}
	}
	glctx.GL.Call("uniform3f", x.uColor, col[0], col[1], col[2])

	// Additive multi-pass "beam": one bright center line plus dim sub-pixel-
	// offset copies around it, so the 1px GL line reads as a thicker, soft-edged
	// (antialiased) glowing trace like the attractor's lines — WebGL can't set
	// lineWidth reliably, so we fake width + AA with offset passes.
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE")) // additive glow
	dx := float32(1.4) / float32(gpu.width)
	dy := float32(1.4) / float32(gpu.height)
	halo := [][3]float32{ // x-offset, y-offset, alpha
		{dx, 0, 0.35}, {-dx, 0, 0.35}, {0, dy, 0.35}, {0, -dy, 0.35},
		{dx, dy, 0.22}, {-dx, -dy, 0.22}, {dx, -dy, 0.22}, {-dx, dy, 0.22},
	}
	for _, h := range halo {
		glctx.GL.Call("uniform2f", x.uOffset, h[0], h[1])
		glctx.GL.Call("uniform1f", x.uAlpha, h[2])
		glctx.GL.Call("drawArrays", glctx.Types.LineStrip, 0, drawn)
	}
	// Bright center pass last so it sits on top of the halo.
	glctx.GL.Call("uniform2f", x.uOffset, 0, 0)
	glctx.GL.Call("uniform1f", x.uAlpha, 1.0)
	glctx.GL.Call("drawArrays", glctx.Types.LineStrip, 0, drawn)
	glctx.GL.Call("disable", glctx.GL.Get("BLEND"))
}

// ── PERSIST, and the correlation meter ───────────────────────────────────

// xyPersistK is the PERSIST knob as a per-frame retention factor, clamped to
// [0, 0.98]. Zero means "clear", which is what the mode always did.
//
// The ceiling is not 1. At a retention of exactly 1 the frame never decays at
// all, so the display fills in and then stays filled — a trace that cannot be
// erased is not a long persistence, it is a stuck picture, and the only way out
// of it would be to leave the mode. 0.98 is about a 2.5-second decay at 60 Hz,
// which is longer than the longest phosphor in the Style module (P33 at 0.985
// there is a decay per frame of the same kind), and it still goes away.
func (x *xyScope) persistK() float32 {
	v := x.persist
	if !(v > 0) { // false for NaN too
		return 0
	}
	if v > 0.98 {
		return 0.98
	}
	return v
}

// xyNoteState records this frame's measurement and updates the readout.
// stereoReadout renders it, because it is the same reading of the same quantity
// and two wordings of "mono" would be two things to keep in step.
func (x *xyScope) noteState(monoSrc, ok bool, corr float32) {
	x.monoSrc, x.corrOK, x.corr = monoSrc, ok, corr
	s := stereoReadout(monoSrc, ok, corr)
	if s == x.corrText {
		// The correlation moves continuously and the DOM does not need to hear
		// about every frame of it — showStereoReadout's argument, and the same
		// trap: a two-decimal readout re-rendered sixty times a second is
		// unreadable even when it is correct.
		return
	}
	x.corrText = s
	if x.corrEl.Truthy() {
		x.corrEl.Set("textContent", s)
	}
}

// appendXYReadout adds the CORR cell to the XY Scope's parameter grid. Into the
// grid rather than #params, for appendStereoReadout's reason: #params stacks
// below the height-bounded grid and gets clipped.
func (x *xyScope) appendXYReadout(grid js.Value) {
	card, top := newPunitCard("corr")

	x.corrEl = dom.Doc.Call("createElement", "span")
	x.corrEl.Set("className", "led counter-led")
	x.corrEl.Set("title", "Correlation between the two channels over the displayed window, as a goniometer's "+
		"correlation meter reads it: +1.00 means the channels are identical and the figure is the diagonal "+
		"line, 0 means they are unrelated and the figure is a round cloud, −1.00 means one is the other's "+
		"polarity inverted — and that is the content that disappears if the mix is summed to mono. "+
		"Measured on L and R whatever the axes knob is set to, because it is a property of the channels "+
		"rather than of the way they are drawn. "+
		"\"mono\" means the source has only one channel, so there is no stereo relationship to read. "+
		"\"r --\" means silence, or one dead channel: nothing to correlate.")
	// Seeded from the last measurement rather than from a placeholder: the
	// panel is rebuilt on every module toggle, and a cell that came back
	// reading "r --" over a live stereo source would be reporting a silence
	// that is not there. xyCorrText is cleared so the next frame writes into
	// the NEW element instead of skipping it as unchanged.
	x.corrText = ""
	x.corrEl.Set("textContent", stereoReadout(x.monoSrc, x.corrOK, x.corr))
	top.Call("appendChild", x.corrEl)

	grid.Call("appendChild", card)
}
