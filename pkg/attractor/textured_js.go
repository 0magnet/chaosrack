//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
	"syscall/js"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
)

// Textured rendering pipeline. A second shader program (texProgram) draws
// geometry with a sampled 2D texture instead of the attractor's gradient
// coloring, reusing the same P/V/M matrices so textured models rotate,
// zoom, and auto-rotate exactly like every other model. It backs both the
// spectrogram plane-model (Stage 2) and the "spectrogram skin" on surface
// models (Stage 3). All state lives here so the attractor pipeline in
// render.go stays untouched.

const texVertShaderSrc = `
	attribute vec3 aPos;
	attribute vec2 aUV;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	varying vec2 vUV;
	void main(void) {
		gl_Position = Pmatrix * Vmatrix * Mmatrix * vec4(aPos, 1.0);
		vUV = aUV;
	}
`

const texFragShaderSrc = `
	precision mediump float;
	varying vec2 vUV;
	uniform sampler2D uSampler;
	uniform float uOffset;
	void main(void) {
		// uOffset scrolls the time axis so the newest column sits at the
		// right edge (u=1); wrap keeps the ring-buffer texture seamless.
		float u = mod(vUV.x + uOffset, 1.0);
		gl_FragColor = texture2D(uSampler, vec2(u, vUV.y));
	}
`

// texturedPipe is the textured program the spectrogram, recurrence plot and
// terminals are drawn with: its locations, its planes, and the matrix
// scratch.
type texturedPipe struct {
	program     js.Value
	posLoc      js.Value
	uvLoc       js.Value
	uSamplerLoc js.Value
	uOffsetLoc  js.Value
	pmatLoc     js.Value
	vmatLoc     js.Value
	mmatLoc     js.Value
	ready       bool

	// Unit plane (two triangles, TRIANGLE_STRIP) with interleaved
	// pos(x,y,z) + uv(u,v). Half-extents give a ~5:3 landscape rectangle;
	// v=0 is the bottom edge so it lines up with the spectrogram's 0 Hz.
	planeBuf   js.Value
	planeReady bool

	// The same plane at 1:1. A spectrogram is time against frequency and the
	// landscape rectangle suits it; a recurrence matrix is time against the
	// SAME time, and on a stretched quad its 45° diagonal — the one line that
	// is always lit, and the reference every other feature is read against —
	// would not come out at 45°.
	sqBuf   js.Value
	sqReady bool

	// Persistent 64-byte scratch for matrix uniform uploads — created once, so
	// per-frame matrix uploads allocate no JS objects (same pattern as the
	// gpu.vertU8/gpu.vertF32 vertex scratch). WebGL copies uniform data during
	// the uniformMatrix4fv call, so reusing one buffer across consecutive uploads
	// in a frame is safe.
	matU8  js.Value
	matF32 js.Value

	// aspectQuads holds one quad buffer per shape, keyed by aspect × 1000.
	//
	// Cached because a quad is a GL buffer, and building one per frame would leak
	// a buffer per frame. There are only ever a handful of distinct shapes: one per
	// source canvas.
	aspectQuads map[int]js.Value
}

var texp = texturedPipe{
	aspectQuads: map[int]js.Value{},
}

var (

	// frameNowMs is the current frame's rAF timestamp, published by
	// renderLoop so generateForMode-driven modes (spectrogram) can pace
	// themselves by wall-clock time.
	frameNowMs float64
)

const (
	planeHalfW = 2.5
	planeHalfH = 1.5
)

func (t *texturedPipe) setupTexShaders() {
	vs := glctx.GL.Call("createShader", glctx.Types.VertexShader)
	glctx.GL.Call("shaderSource", vs, texVertShaderSrc)
	glctx.GL.Call("compileShader", vs)
	fs := glctx.GL.Call("createShader", glctx.Types.FragmentShader)
	glctx.GL.Call("shaderSource", fs, texFragShaderSrc)
	glctx.GL.Call("compileShader", fs)

	t.program = glctx.GL.Call("createProgram")
	glctx.GL.Call("attachShader", t.program, vs)
	glctx.GL.Call("attachShader", t.program, fs)
	glctx.GL.Call("linkProgram", t.program)

	t.posLoc = glctx.GL.Call("getAttribLocation", t.program, "aPos")
	t.uvLoc = glctx.GL.Call("getAttribLocation", t.program, "aUV")
	t.uSamplerLoc = glctx.GL.Call("getUniformLocation", t.program, "uSampler")
	t.uOffsetLoc = glctx.GL.Call("getUniformLocation", t.program, "uOffset")
	t.pmatLoc = glctx.GL.Call("getUniformLocation", t.program, "Pmatrix")
	t.vmatLoc = glctx.GL.Call("getUniformLocation", t.program, "Vmatrix")
	t.mmatLoc = glctx.GL.Call("getUniformLocation", t.program, "Mmatrix")
	t.ready = true
}

func (t *texturedPipe) initTexPlane() {
	if t.planeReady {
		return
	}
	t.planeBuf = newTexQuad(planeHalfW, planeHalfH)
	t.planeReady = true
}

func (t *texturedPipe) initTexSquare() {
	if t.sqReady {
		return
	}
	// Half-extent matched to the plane's height, so switching between the
	// spectrogram and the recurrence plot does not change how much of the
	// viewport the picture fills.
	t.sqBuf = newTexQuad(planeHalfH, planeHalfH)
	t.sqReady = true
}

// newTexQuad builds one TRIANGLE_STRIP quad (BL, BR, TL, TR) of interleaved
// x,y,z,u,v. v=0 is the bottom edge so it lines up with the spectrogram's 0 Hz.
func newTexQuad(hw, hh float32) js.Value {
	verts := []float32{
		-hw, -hh, 0, 0, 0,
		hw, -hh, 0, 1, 0,
		-hw, hh, 0, 0, 1,
		hw, hh, 0, 1, 1,
	}
	buf := glctx.GL.Call("createBuffer")
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, buf)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(verts), glctx.Types.StaticDraw)
	return buf
}

// mat4ToTyped returns a JS Float32Array holding the matrix for uniform
// upload, reusing one persistent typed array.
func (t *texturedPipe) mat4ToTyped(m *mgl32.Mat4) js.Value {
	if t.matU8.IsUndefined() {
		t.matU8 = js.Global().Get("Uint8Array").New(64)
		t.matF32 = js.Global().Get("Float32Array").New(t.matU8.Get("buffer"), 0, 16)
	}
	buf := (*[16]float32)(unsafe.Pointer(m)) //nolint:gosec // reinterpreting a typed slice as its backing bytes to cross into JS without a second copy
	js.CopyBytesToJS(t.matU8, sliceToByteSlice((*buf)[:]))
	return t.matF32
}

// useTexProgram activates texProgram and uploads the current P/V/M
// matrices to it. Call before any textured draw.
func (t *texturedPipe) useTexProgram() {
	glctx.GL.Call("useProgram", t.program)
	glctx.GL.Call("uniformMatrix4fv", t.pmatLoc, false, t.mat4ToTyped(&gpu.proj))
	glctx.GL.Call("uniformMatrix4fv", t.vmatLoc, false, t.mat4ToTyped(&view.viewMat))
	glctx.GL.Call("uniformMatrix4fv", t.mmatLoc, false, t.mat4ToTyped(&view.modelMat))
}

// drawTexturedPlane draws the unit plane with the given texture and scroll
// offset through texProgram (and thus the shared camera/rotation state).
func (t *texturedPipe) drawTexturedPlane(texture js.Value, offset float32) {
	if !t.ready {
		return
	}
	t.initTexPlane()
	t.useTexProgram()

	// "Fill" switch: map the plane's extents straight to clip space (face-on,
	// filling the canvas) instead of the rotatable 3D placement.
	if spect.fill {
		fill := mgl32.Ident4()
		fill[0] = 1.0 / planeHalfW
		fill[5] = 1.0 / planeHalfH
		id := mgl32.Ident4()
		glctx.GL.Call("uniformMatrix4fv", t.pmatLoc, false, t.mat4ToTyped(&fill))
		glctx.GL.Call("uniformMatrix4fv", t.vmatLoc, false, t.mat4ToTyped(&id))
		glctx.GL.Call("uniformMatrix4fv", t.mmatLoc, false, t.mat4ToTyped(&id))
	}

	t.drawTexQuad(t.planeBuf, texture, offset)
}

// drawTexturedSquare draws the 1:1 quad with the given texture and no scroll
// offset — for a texture whose two axes are the same axis (pkg/recurrence).
func (t *texturedPipe) drawTexturedSquare(texture js.Value) {
	if !t.ready {
		return
	}
	t.initTexSquare()
	t.useTexProgram()
	t.drawTexQuad(t.sqBuf, texture, 0)
}

// drawTexturedAspect draws a texture on a quad of ITS OWN SHAPE.
//
// drawTexturedSquare puts everything on a 1:1 quad, which is right for a
// recurrence plot — whose two axes are the same axis — and wrong for anything
// that is a picture of something. A 1920×999 desktop on a square quad is a
// desktop squashed to half its width, and the giveaway is the text: letters go
// tall and narrow before anything else looks wrong.
//
// The height is kept and the width follows, so switching between these modes
// does not change how much of the viewport is filled vertically.
func (t *texturedPipe) drawTexturedAspect(texture js.Value, aspect float32) {
	if !t.ready {
		return
	}
	if aspect <= 0 || math.IsInf(float64(aspect), 0) || math.IsNaN(float64(aspect)) {
		t.drawTexturedSquare(texture)
		return
	}
	key := int(aspect*1000 + 0.5)
	buf, ok := t.aspectQuads[key]
	if !ok {
		buf = newTexQuad(planeHalfH*aspect, planeHalfH)
		t.aspectQuads[key] = buf
	}
	t.useTexProgram()
	t.drawTexQuad(buf, texture, 0)
}

// canvasAspect is a canvas's width over its height, or zero if it has neither.
func canvasAspect(cv js.Value) float32 {
	if !cv.Truthy() {
		return 0
	}
	w := cv.Get("width").Float()
	h := cv.Get("height").Float()
	if w <= 0 || h <= 0 {
		return 0
	}
	return float32(w / h)
}

// drawTexQuad binds one quad buffer and the texture and draws it. Assumes
// texProgram is current and its matrices are already uploaded.
func (t *texturedPipe) drawTexQuad(buf, texture js.Value, offset float32) {
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, buf)
	// stride 20 bytes: 3 floats pos + 2 floats uv
	glctx.GL.Call("vertexAttribPointer", t.posLoc, 3, glctx.Types.Float, false, 20, 0)
	glctx.GL.Call("enableVertexAttribArray", t.posLoc)
	glctx.GL.Call("vertexAttribPointer", t.uvLoc, 2, glctx.Types.Float, false, 20, 12)
	glctx.GL.Call("enableVertexAttribArray", t.uvLoc)

	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), texture)
	glctx.GL.Call("uniform1i", t.uSamplerLoc, 0)
	glctx.GL.Call("uniform1f", t.uOffsetLoc, float64(offset))

	glctx.GL.Call("drawArrays", glctx.GL.Get("TRIANGLE_STRIP"), 0, 4)
}

// drawTexturedMesh draws an indexed triangle mesh (interleaved pos+uv,
// stride 20) with the given texture and scroll offset through texProgram,
// so it shares the camera/rotation state. Used for the spectrogram skin
// on surface models.
func (t *texturedPipe) drawTexturedMesh(vertBuf, idxBuf js.Value, idxCount int, texture js.Value, offset float32) {
	if !t.ready || idxCount == 0 {
		return
	}
	t.useTexProgram()

	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, vertBuf)
	glctx.GL.Call("vertexAttribPointer", t.posLoc, 3, glctx.Types.Float, false, 20, 0)
	glctx.GL.Call("enableVertexAttribArray", t.posLoc)
	glctx.GL.Call("vertexAttribPointer", t.uvLoc, 2, glctx.Types.Float, false, 20, 12)
	glctx.GL.Call("enableVertexAttribArray", t.uvLoc)
	glctx.GL.Call("bindBuffer", glctx.Types.ElementArrayBuffer, idxBuf)

	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), texture)
	glctx.GL.Call("uniform1i", t.uSamplerLoc, 0)
	glctx.GL.Call("uniform1f", t.uOffsetLoc, float64(offset))

	glctx.GL.Call("drawElements", glctx.GL.Get("TRIANGLES"), idxCount, glctx.Types.UnsignedShort, 0)
}
