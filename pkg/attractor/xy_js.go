//go:build js && wasm

package attractor

import (
	"syscall/js"
)

// The xy scope mode draws (L[i], R[i]) as a line strip on the shared
// #gocanvas — classic oscilloscope-style Lissajous. Mono sources use a
// lag on the same channel to still produce a useful figure (pure
// diagonal on a raw mono signal is not useful; a lagged copy adds an
// orbit).
//
// Own shader program (pass-through vertex + solid-color fragment) with a
// dedicated dynamic-vertex buffer sized to the sample window.

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

// xySmooth is the beam-smoothing upsample factor: each sample-to-sample
// segment is rendered as this many Catmull-Rom spline steps, so the trace
// curves through the samples instead of turning a hard corner at each one —
// an analog scope's beam is slew-limited by the amplifier and never draws
// sharp angles.
const xySmooth = 4

var (
	xyProgram js.Value
	xyBuf     js.Value
	xyAPos    js.Value
	xyUColor  js.Value
	xyUAlpha  js.Value
	xyUOffset js.Value
	xyReady   bool
	xyWindow  = 2048
	xyBufL    []float32
	xyBufR    []float32
	xyLine    []float32 // interleaved x,y pairs for GL upload (xySmooth per sample)
	xyJsUint8 js.Value  // persistent upload scratch (byte view)
	xyJsFloat js.Value  // persistent upload scratch (float32 view)
	xyScale   float32   = 0.9
	xyMonoLag int       = 128
)

func initXY() {
	if xyReady {
		return
	}
	vs := gl.Call("createShader", glTypes.VertexShader)
	gl.Call("shaderSource", vs, xyVertShaderSrc)
	gl.Call("compileShader", vs)
	fs := gl.Call("createShader", glTypes.FragmentShader)
	gl.Call("shaderSource", fs, xyFragShaderSrc)
	gl.Call("compileShader", fs)

	xyProgram = gl.Call("createProgram")
	gl.Call("attachShader", xyProgram, vs)
	gl.Call("attachShader", xyProgram, fs)
	gl.Call("linkProgram", xyProgram)

	xyAPos = gl.Call("getAttribLocation", xyProgram, "aPos")
	xyUColor = gl.Call("getUniformLocation", xyProgram, "uColor")
	xyUAlpha = gl.Call("getUniformLocation", xyProgram, "uAlpha")
	xyUOffset = gl.Call("getUniformLocation", xyProgram, "uOffset")

	xyBuf = gl.Call("createBuffer")
	xyBufL = make([]float32, xyWindow)
	xyBufR = make([]float32, xyWindow)
	xyLine = make([]float32, xyWindow*xySmooth*2)
	// Persistent upload scratch — the trace re-uploads every frame, and a
	// fresh typed array per frame is steady GC pressure (same pattern as
	// jsVertUint8/jsVertFloat).
	xyJsUint8 = js.Global().Get("Uint8Array").New(len(xyLine) * 4)
	xyJsFloat = js.Global().Get("Float32Array").New(xyJsUint8.Get("buffer"), 0, len(xyLine))
	xyReady = true
}

// renderXYFrame is the full-screen xy-scope MODE: clear the canvas, then draw.
func renderXYFrame() { drawXYScope(true) }

// drawXYScope fills the sample window and draws the Lissajous line strip. When
// clear is true (xy MODE) it clears the canvas first; when false (xy used as a
// BACKGROUND behind an attractor) it draws onto whatever is already there, so
// the caller controls clearing and the attractor can be layered on top.
func drawXYScope(clear bool) {
	if !xyReady {
		initXY()
	}
	src := ensureAudioSource()

	if src != nil && src.Ready() {
		src.TimeDomainStereo(xyBufL, xyBufR)
		// If the source only has one channel, fall back to a lagged
		// pseudo-stereo so the trace isn't a straight diagonal.
		if src.Channels() < 2 {
			for i := 0; i < xyWindow; i++ {
				j := i - xyMonoLag
				if j < 0 {
					xyBufR[i] = 0
				} else {
					xyBufR[i] = xyBufL[j]
				}
			}
		}
		// Catmull-Rom upsample: xySmooth spline steps per sample segment, per
		// channel, so the beam curves through the samples (analog slew) instead
		// of cornering at every one.
		clampIdx := func(i int) int {
			if i < 0 {
				return 0
			}
			if i >= xyWindow {
				return xyWindow - 1
			}
			return i
		}
		o := 0
		for i := 0; i < xyWindow; i++ {
			l0, l1 := xyBufL[clampIdx(i-1)], xyBufL[i]
			l2, l3 := xyBufL[clampIdx(i+1)], xyBufL[clampIdx(i+2)]
			r0, r1 := xyBufR[clampIdx(i-1)], xyBufR[i]
			r2, r3 := xyBufR[clampIdx(i+1)], xyBufR[clampIdx(i+2)]
			for s := 0; s < xySmooth; s++ {
				t := float32(s) / xySmooth
				xyLine[o] = catmullRom(l0, l1, l2, l3, t) * xyScale
				xyLine[o+1] = catmullRom(r0, r1, r2, r3, t) * xyScale
				o += 2
			}
		}
	} else {
		// Blank the line so we don't draw stale data.
		for i := range xyLine {
			xyLine[i] = 0
		}
	}

	// Draw as a flat 2D trace (no depth), so as a background it never occludes
	// or z-fights the attractor layered on top.
	gl.Call("disable", glTypes.DepthTest)
	if clear {
		// Transparent clear (alpha 0), like every other mode — so with "Front" on
		// (canvas layered over the panel) the scope shows its trace over the
		// controls instead of an opaque black block that hides the panel and can't
		// be undone.
		gl.Call("clearColor", 0, 0, 0, 0)
		gl.Call("clear", glTypes.ColorBufferBit)
	}

	gl.Call("useProgram", xyProgram)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, xyBuf)
	js.CopyBytesToJS(xyJsUint8, sliceToByteSlice(xyLine))
	gl.Call("bufferData", glTypes.ArrayBuffer, xyJsFloat, glTypes.DynamicDraw)
	gl.Call("enableVertexAttribArray", xyAPos)
	gl.Call("vertexAttribPointer", xyAPos, 2, glTypes.Float, false, 0, 0)

	col := [3]float32{0.4, 1.0, 0.45}
	if phosphorActive() { // scope mode → trace in the selected phosphor color
		p := phosphors[phosphorIdx]
		col = [3]float32{float32(p.tr), float32(p.tg), float32(p.tb)}
	}
	gl.Call("uniform3f", xyUColor, col[0], col[1], col[2])

	// Additive multi-pass "beam": one bright center line plus dim sub-pixel-
	// offset copies around it, so the 1px GL line reads as a thicker, soft-edged
	// (antialiased) glowing trace like the attractor's lines — WebGL can't set
	// lineWidth reliably, so we fake width + AA with offset passes.
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE")) // additive glow
	dx := float32(1.4) / float32(width)
	dy := float32(1.4) / float32(height)
	halo := [][3]float32{ // x-offset, y-offset, alpha
		{dx, 0, 0.35}, {-dx, 0, 0.35}, {0, dy, 0.35}, {0, -dy, 0.35},
		{dx, dy, 0.22}, {-dx, -dy, 0.22}, {dx, -dy, 0.22}, {-dx, dy, 0.22},
	}
	for _, h := range halo {
		gl.Call("uniform2f", xyUOffset, h[0], h[1])
		gl.Call("uniform1f", xyUAlpha, h[2])
		gl.Call("drawArrays", glTypes.LineStrip, 0, xyWindow*xySmooth)
	}
	// Bright center pass last so it sits on top of the halo.
	gl.Call("uniform2f", xyUOffset, 0, 0)
	gl.Call("uniform1f", xyUAlpha, 1.0)
	gl.Call("drawArrays", glTypes.LineStrip, 0, xyWindow*xySmooth)
	gl.Call("disable", gl.Get("BLEND"))
}
