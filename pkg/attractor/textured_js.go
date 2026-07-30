//go:build js && wasm

package attractor

import (
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

var (
	// Matrices are cached here (pkg-level) so texProgram can be fed the
	// same values the attractor program uses. movMatrix already lives in
	// main.go; these two are populated by setupMatrices/updateViewMatrix.
	projMatrix mgl32.Mat4
	viewMatrix mgl32.Mat4

	// frameNowMs is the current frame's rAF timestamp, published by
	// renderLoop so generateForMode-driven modes (spectrogram) can pace
	// themselves by wall-clock time.
	frameNowMs float64

	texProgram     js.Value
	texPosLoc      js.Value
	texUVLoc       js.Value
	texUSamplerLoc js.Value
	texUOffsetLoc  js.Value
	texPmatLoc     js.Value
	texVmatLoc     js.Value
	texMmatLoc     js.Value
	texReady       bool

	// Unit plane (two triangles, TRIANGLE_STRIP) with interleaved
	// pos(x,y,z) + uv(u,v). Half-extents give a ~5:3 landscape rectangle;
	// v=0 is the bottom edge so it lines up with the spectrogram's 0 Hz.
	texPlaneBuf   js.Value
	texPlaneReady bool
)

const (
	planeHalfW = 2.5
	planeHalfH = 1.5
)

func setupTexShaders() {
	vs := gl.Call("createShader", glTypes.VertexShader)
	gl.Call("shaderSource", vs, texVertShaderSrc)
	gl.Call("compileShader", vs)
	fs := gl.Call("createShader", glTypes.FragmentShader)
	gl.Call("shaderSource", fs, texFragShaderSrc)
	gl.Call("compileShader", fs)

	texProgram = gl.Call("createProgram")
	gl.Call("attachShader", texProgram, vs)
	gl.Call("attachShader", texProgram, fs)
	gl.Call("linkProgram", texProgram)

	texPosLoc = gl.Call("getAttribLocation", texProgram, "aPos")
	texUVLoc = gl.Call("getAttribLocation", texProgram, "aUV")
	texUSamplerLoc = gl.Call("getUniformLocation", texProgram, "uSampler")
	texUOffsetLoc = gl.Call("getUniformLocation", texProgram, "uOffset")
	texPmatLoc = gl.Call("getUniformLocation", texProgram, "Pmatrix")
	texVmatLoc = gl.Call("getUniformLocation", texProgram, "Vmatrix")
	texMmatLoc = gl.Call("getUniformLocation", texProgram, "Mmatrix")
	texReady = true
}

func initTexPlane() {
	if texPlaneReady {
		return
	}
	hw, hh := float32(planeHalfW), float32(planeHalfH)
	// interleaved x,y,z,u,v — TRIANGLE_STRIP: BL, BR, TL, TR
	verts := []float32{
		-hw, -hh, 0, 0, 0,
		hw, -hh, 0, 1, 0,
		-hw, hh, 0, 0, 1,
		hw, hh, 0, 1, 1,
	}
	texPlaneBuf = gl.Call("createBuffer")
	gl.Call("bindBuffer", glTypes.ArrayBuffer, texPlaneBuf)
	gl.Call("bufferData", glTypes.ArrayBuffer, SliceToTypedArray(verts), glTypes.StaticDraw)
	texPlaneReady = true
}

// Persistent 64-byte scratch for matrix uniform uploads — created once, so
// per-frame matrix uploads allocate no JS objects (same pattern as the
// jsVertUint8/jsVertFloat vertex scratch). WebGL copies uniform data during
// the uniformMatrix4fv call, so reusing one buffer across consecutive uploads
// in a frame is safe.
var (
	jsMatUint8 js.Value
	jsMatFloat js.Value
)

// mat4ToTyped returns a JS Float32Array holding the matrix for uniform
// upload, reusing one persistent typed array.
func mat4ToTyped(m *mgl32.Mat4) js.Value {
	if jsMatUint8.IsUndefined() {
		jsMatUint8 = js.Global().Get("Uint8Array").New(64)
		jsMatFloat = js.Global().Get("Float32Array").New(jsMatUint8.Get("buffer"), 0, 16)
	}
	buf := (*[16]float32)(unsafe.Pointer(m))
	js.CopyBytesToJS(jsMatUint8, sliceToByteSlice((*buf)[:]))
	return jsMatFloat
}

// useTexProgram activates texProgram and uploads the current P/V/M
// matrices to it. Call before any textured draw.
func useTexProgram() {
	gl.Call("useProgram", texProgram)
	gl.Call("uniformMatrix4fv", texPmatLoc, false, mat4ToTyped(&projMatrix))
	gl.Call("uniformMatrix4fv", texVmatLoc, false, mat4ToTyped(&viewMatrix))
	gl.Call("uniformMatrix4fv", texMmatLoc, false, mat4ToTyped(&movMatrix))
}

// drawTexturedPlane draws the unit plane with the given texture and scroll
// offset through texProgram (and thus the shared camera/rotation state).
func drawTexturedPlane(texture js.Value, offset float32) {
	if !texReady {
		return
	}
	initTexPlane()
	useTexProgram()

	// "Fill" switch: map the plane's extents straight to clip space (face-on,
	// filling the canvas) instead of the rotatable 3D placement.
	if spectFill {
		fill := mgl32.Ident4()
		fill[0] = 1.0 / planeHalfW
		fill[5] = 1.0 / planeHalfH
		id := mgl32.Ident4()
		gl.Call("uniformMatrix4fv", texPmatLoc, false, mat4ToTyped(&fill))
		gl.Call("uniformMatrix4fv", texVmatLoc, false, mat4ToTyped(&id))
		gl.Call("uniformMatrix4fv", texMmatLoc, false, mat4ToTyped(&id))
	}

	gl.Call("bindBuffer", glTypes.ArrayBuffer, texPlaneBuf)
	// stride 20 bytes: 3 floats pos + 2 floats uv
	gl.Call("vertexAttribPointer", texPosLoc, 3, glTypes.Float, false, 20, 0)
	gl.Call("enableVertexAttribArray", texPosLoc)
	gl.Call("vertexAttribPointer", texUVLoc, 2, glTypes.Float, false, 20, 12)
	gl.Call("enableVertexAttribArray", texUVLoc)

	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), texture)
	gl.Call("uniform1i", texUSamplerLoc, 0)
	gl.Call("uniform1f", texUOffsetLoc, float64(offset))

	gl.Call("drawArrays", gl.Get("TRIANGLE_STRIP"), 0, 4)
}

// drawTexturedMesh draws an indexed triangle mesh (interleaved pos+uv,
// stride 20) with the given texture and scroll offset through texProgram,
// so it shares the camera/rotation state. Used for the spectrogram skin
// on surface models.
func drawTexturedMesh(vertBuf, idxBuf js.Value, idxCount int, texture js.Value, offset float32) {
	if !texReady || idxCount == 0 {
		return
	}
	useTexProgram()

	gl.Call("bindBuffer", glTypes.ArrayBuffer, vertBuf)
	gl.Call("vertexAttribPointer", texPosLoc, 3, glTypes.Float, false, 20, 0)
	gl.Call("enableVertexAttribArray", texPosLoc)
	gl.Call("vertexAttribPointer", texUVLoc, 2, glTypes.Float, false, 20, 12)
	gl.Call("enableVertexAttribArray", texUVLoc)
	gl.Call("bindBuffer", glTypes.ElementArrayBuffer, idxBuf)

	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), texture)
	gl.Call("uniform1i", texUSamplerLoc, 0)
	gl.Call("uniform1f", texUOffsetLoc, float64(offset))

	gl.Call("drawElements", gl.Get("TRIANGLES"), idxCount, glTypes.UnsignedShort, 0)
}
