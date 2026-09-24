//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"
)

// A per-vertex-color program for the flat analyzers.
//
// The xy scope's program carries ONE color in a uniform, which is right for a
// scope trace and wrong for a display whose whole point is that different parts
// of it mean different things. An RTA's bars say more when their color is the
// level as well as the height, and a transfer function's magnitude curve says
// far more when its color is the COHERENCE — that is how a system-tuning rig
// shows you which parts of the curve to believe, and it is the reading that
// otherwise needs a second glance at a second lane.
//
// ── IT USES THE COLORS MODULE'S OWN PALETTE ──────────────────────────────
//
// No new knob. palette_js.go's argument is that a value should paint the same
// color in the spectrogram and on the trail, so that the displays are readable
// against each other rather than each having a private language; a third and
// fourth private language here would be exactly the thing it argues against.
// So these read the gradient palette the Colors module already sets, and when
// it names one of the six colormaps they use it — heat, blue, grayscale, turbo,
// viridis, magma, the same six and in the same order.
//
// When it does NOT — mono, two-color, three-color or the rainbow, which are
// about mixing swatches rather than mapping a scalar — they fall back to the
// single trace color they had before. A two-swatch gradient has no meaning for
// "how coherent is this band", and inventing one would be worse than the plain
// green.

const vcVertShaderSrc = `
	attribute vec2 aPos;
	attribute vec3 aCol;
	uniform vec2 uOffset;
	varying vec3 vCol;
	void main() {
		vCol = aCol;
		gl_Position = vec4(aPos + uOffset, 0.0, 1.0);
	}
`

const vcFragShaderSrc = `
	precision mediump float;
	varying vec3 vCol;
	uniform float uAlpha;
	void main(void) {
		gl_FragColor = vec4(vCol, uAlpha);
	}
`

var (
	vcProgram js.Value
	vcBuf     js.Value
	vcAPos    js.Value
	vcACol    js.Value
	vcUAlpha  js.Value
	vcUOffset js.Value
	vcReady   bool
	vcData    []float32 // interleaved x,y,r,g,b
	vcU8      js.Value
	vcF32     js.Value
)

// vcStride is the floats per vertex: two of position and three of color.
const vcStride = 5

func initVColor() {
	if vcReady {
		return
	}
	vs := glctx.GL.Call("createShader", glctx.Types.VertexShader)
	glctx.GL.Call("shaderSource", vs, vcVertShaderSrc)
	glctx.GL.Call("compileShader", vs)
	fs := glctx.GL.Call("createShader", glctx.Types.FragmentShader)
	glctx.GL.Call("shaderSource", fs, vcFragShaderSrc)
	glctx.GL.Call("compileShader", fs)
	vcProgram = glctx.GL.Call("createProgram")
	glctx.GL.Call("attachShader", vcProgram, vs)
	glctx.GL.Call("attachShader", vcProgram, fs)
	glctx.GL.Call("linkProgram", vcProgram)
	vcAPos = glctx.GL.Call("getAttribLocation", vcProgram, "aPos")
	vcACol = glctx.GL.Call("getAttribLocation", vcProgram, "aCol")
	vcUAlpha = glctx.GL.Call("getUniformLocation", vcProgram, "uAlpha")
	vcUOffset = glctx.GL.Call("getUniformLocation", vcProgram, "uOffset")
	vcBuf = glctx.GL.Call("createBuffer")
	vcReady = true
}

// vcFit sizes the interleaved buffer and its upload scratch for n vertices.
func vcFit(n int) {
	need := n * vcStride
	if len(vcData) >= need {
		return
	}
	vcData = make([]float32, need+need/2)
	vcU8 = js.Global().Get("Uint8Array").New(len(vcData) * 4)
	vcF32 = js.Global().Get("Float32Array").New(vcU8.Get("buffer"), 0, len(vcData))
}

// vcPut writes one vertex at index i.
func vcPut(i int, x, y float32, c [3]float32) {
	o := i * vcStride
	vcData[o], vcData[o+1] = x, y
	vcData[o+2], vcData[o+3], vcData[o+4] = c[0], c[1], c[2]
}

// vcUpload binds the program and hands the buffer over. Called once before a
// run of vcSpan calls, so the geometry is uploaded once however many passes are
// drawn from it.
func vcUpload(n int) {
	if n <= 0 {
		return
	}
	glctx.GL.Call("useProgram", vcProgram)
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, vcBuf)
	js.CopyBytesToJS(vcU8, sliceToByteSlice(vcData))
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, vcF32, glctx.Types.DynamicDraw)
	glctx.GL.Call("enableVertexAttribArray", vcAPos)
	glctx.GL.Call("vertexAttribPointer", vcAPos, 2, glctx.Types.Float, false, vcStride*4, 0)
	glctx.GL.Call("enableVertexAttribArray", vcACol)
	glctx.GL.Call("vertexAttribPointer", vcACol, 3, glctx.Types.Float, false, vcStride*4, 2*4)
}

// vcSpan draws count vertices starting at first, offset by (dx, dy) in clip
// space.
//
// The offset is a UNIFORM rather than a shift applied to the vertices, which is
// how the widening passes are done without touching the geometry. Written the
// other way first — adding dx to every x between passes and subtracting it back
// afterwards — it was both slower and wrong, because the shifts accumulated
// asymmetrically and the halo ended up on one side.
func vcSpan(mode js.Value, first, count int, alpha, dx, dy float32) {
	if count <= 0 {
		return
	}
	glctx.GL.Call("uniform1f", vcUAlpha, alpha)
	glctx.GL.Call("uniform2f", vcUOffset, dx, dy)
	glctx.GL.Call("drawArrays", mode, first, count)
}

// vcDone releases the color attribute, which the other programs do not have
// and would otherwise inherit as a stale binding.
func vcDone() { glctx.GL.Call("disableVertexAttribArray", vcACol) }

// analyzerPalette reports the colormap the Colors module currently names, if it
// names one at all.
//
// The same selection the trail and the spectrogram read, so a level that paints
// amber on one paints amber on the others. When the palette is one of the
// swatch-mixing ones instead, ok is false and the caller keeps its single
// color — a two-swatch gradient is not a scalar map and pretending otherwise
// would put arbitrary colors on a measurement.
func analyzerPalette() (int, bool) { return colormap.Index(gradientColors) }

// analyzerColorAt samples that colormap at a 0..1 value, as three floats.
func analyzerColorAt(idx int, v float64) [3]float32 {
	c := colormap.At(idx, v)
	r, g, b, _ := c.RGBA()
	return [3]float32{float32(r) / 65535, float32(g) / 65535, float32(b) / 65535}
}

// analyzerTraceColor is the single color a flat analyzer draws in when no
// colormap is selected: the phosphor's, if one is, and the scope green
// otherwise — which is what these displays did before they could be colored.
func analyzerTraceColor() [3]float32 {
	if phosphorActive() {
		p := phosphors[phosphorIdx]
		return [3]float32{float32(p.tr), float32(p.tg), float32(p.tb)}
	}
	return [3]float32{0.4, 1.0, 0.45}
}
