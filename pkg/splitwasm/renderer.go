//go:build js && wasm

package splitwasm

import (
	"math"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"syscall/js"
)

// The shaders are chaosrack's, cut down to what a wireframe globe needs: the
// trail/dwell attributes and the gradient uniforms have no part in the frame
// budget question and only widen the renderer's static data.
const vertSrc = `
	attribute vec3 position;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	varying vec3 vPosition;
	void main(void) {
		gl_Position = Pmatrix * Vmatrix * Mmatrix * vec4(position, 1.0);
		vPosition = position;
	}
`

const fragSrc = `
	precision mediump float;
	uniform vec3 uColor;
	varying vec3 vPosition;
	void main(void) {
		// A little depth shading so the far side of the cage reads as far.
		float d = 0.55 + 0.45 * (vPosition.z * 0.5 + 0.5);
		gl_FragColor = vec4(uColor * d, 1.0);
	}
`

type renderer struct {
	gl                 js.Value
	canvas             js.Value
	prog               js.Value
	vbuf, ibuf         js.Value
	uP, uV, uM, uColor js.Value
	posLoc             js.Value
	width, height      int
	idxCount           int

	// Retained geometry buffers. Rebuilt only when the control plane bumps
	// PGeomSeq, never per frame — a frame that appends to these would allocate,
	// and allocation is what triggers the collections this whole exercise is
	// about.
	verts []float32
	idx   []uint16

	shared   js.Value // the JS Float32Array
	sharedU8 js.Value // a Uint8Array view of it, for the bulk copy
	params   [ParamCount]float32
	raw      []byte // params as bytes, the destination of the copy

	angleX, angleY, angleZ float32
	lastSeq                float32
	frameFn                js.Func
}

// readParams pulls the whole parameter block across in one go. This is the only
// boundary crossing the frame makes for control values.
func (r *renderer) readParams() {
	js.CopyBytesToGo(r.raw, r.sharedU8)
}

// buildGlobe fills the vertex and index buffers for the globe.
//
// The index buffer is UNSIGNED_SHORT, so it cannot address a vertex past
// 65535, and lat and lon come from knobs — the one place in this program
// where geometry size is driven by something a person can turn. At
// pts=60 the vertex count is 61*(lat-1+lon), so the knob maxima in
// control.go (parallels 60, meridians 90) give 9089 and the budget is
// nowhere near spent. Raise either range far enough and the base index
// below wraps, which does not fail — it draws a scrambled globe.
//
// pkg/geom carries the same arithmetic and TestKnobMaximaFitTheIndexSpace
// pins those maxima there, since this package is js-tagged and cannot be
// reached from a native test.
func (r *renderer) buildGlobe(lat, lon int, twist float64) {
	if lat < 2 {
		lat = 2
	}
	if lon < 1 {
		lon = 1
	}
	const pts = 60
	v := r.verts[:0]
	ix := r.idx[:0]
	for i := 1; i < lat; i++ {
		phi := float64(i) * math.Pi / float64(lat)
		base := uint16(len(v) / 3) //nolint:gosec // G115: bounded by the knob maxima; see buildGlobe
		for j := 0; j <= pts; j++ {
			th := float64(j) * 2 * math.Pi / float64(pts)
			v = append(v,
				float32(math.Sin(phi)*math.Cos(th)),
				float32(math.Sin(phi)*math.Sin(th)),
				float32(math.Cos(phi)))
			if j > 0 {
				ix = append(ix, base+uint16(j-1), base+uint16(j))
			}
		}
	}
	for j := 0; j < lon; j++ {
		th0 := float64(j) * 2 * math.Pi / float64(lon)
		base := uint16(len(v) / 3) //nolint:gosec // G115: bounded by the knob maxima; see buildGlobe
		for i := 0; i <= pts; i++ {
			phi := float64(i) * math.Pi / float64(pts)
			th := th0 + twist*phi
			v = append(v,
				float32(math.Sin(phi)*math.Cos(th)),
				float32(math.Sin(phi)*math.Sin(th)),
				float32(math.Cos(phi)))
			if i > 0 {
				ix = append(ix, base+uint16(i-1), base+uint16(i))
			}
		}
	}
	r.verts, r.idx = v, ix
	r.idxCount = len(ix)

	gl := r.gl
	gl.Call("bindBuffer", gl.Get("ARRAY_BUFFER"), r.vbuf)
	gl.Call("bufferData", gl.Get("ARRAY_BUFFER"), f32Array(v), gl.Get("STATIC_DRAW"))
	gl.Call("bindBuffer", gl.Get("ELEMENT_ARRAY_BUFFER"), r.ibuf)
	gl.Call("bufferData", gl.Get("ELEMENT_ARRAY_BUFFER"), u16Array(ix), gl.Get("STATIC_DRAW"))
	gl.Call("vertexAttribPointer", r.posLoc, 3, gl.Get("FLOAT"), false, 0, 0)
	gl.Call("enableVertexAttribArray", r.posLoc)
}

func f32Bytes(s []float32) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*4) //nolint:gosec // G103: reinterpreting the slice is the only way to hand it to js.CopyBytesToJS without copying every element per frame
}

func u16Bytes(s []uint16) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*2) //nolint:gosec // G103: as f32Bytes
}

func f32Array(s []float32) js.Value {
	b := js.Global().Get("Uint8Array").New(len(s) * 4)
	js.CopyBytesToJS(b, f32Bytes(s))
	return js.Global().Get("Float32Array").New(b.Get("buffer"), 0, len(s))
}

func u16Array(s []uint16) js.Value {
	b := js.Global().Get("Uint8Array").New(len(s) * 2)
	js.CopyBytesToJS(b, u16Bytes(s))
	return js.Global().Get("Uint16Array").New(b.Get("buffer"), 0, len(s))
}

func compile(gl js.Value, kind js.Value, src string) js.Value {
	s := gl.Call("createShader", kind)
	gl.Call("shaderSource", s, src)
	gl.Call("compileShader", s)
	return s
}

// StartRenderer brings up WebGL on #gocanvas and runs the frame loop. It is the
// whole of the renderer module: no panel, no knobs, no audio, no persistence.
func StartRenderer() {
	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "gocanvas")
	if canvas.IsNull() || canvas.IsUndefined() {
		return
	}
	gl := canvas.Call("getContext", "webgl")
	if gl.IsNull() || gl.IsUndefined() {
		return
	}
	r := &renderer{gl: gl, canvas: canvas}

	prog := gl.Call("createProgram")
	gl.Call("attachShader", prog, compile(gl, gl.Get("VERTEX_SHADER"), vertSrc))
	gl.Call("attachShader", prog, compile(gl, gl.Get("FRAGMENT_SHADER"), fragSrc))
	gl.Call("linkProgram", prog)
	gl.Call("useProgram", prog)
	r.prog = prog
	r.uP = gl.Call("getUniformLocation", prog, "Pmatrix")
	r.uV = gl.Call("getUniformLocation", prog, "Vmatrix")
	r.uM = gl.Call("getUniformLocation", prog, "Mmatrix")
	r.uColor = gl.Call("getUniformLocation", prog, "uColor")
	r.posLoc = gl.Call("getAttribLocation", prog, "position")
	r.vbuf = gl.Call("createBuffer")
	r.ibuf = gl.Call("createBuffer")

	r.shared = EnsureShared()
	r.sharedU8 = js.Global().Get("Uint8Array").New(r.shared.Get("buffer"), 0, ParamCount*4)
	r.raw = f32Bytes(r.params[:])
	r.readParams()

	r.verts = make([]float32, 0, 3*61*128)
	r.idx = make([]uint16, 0, 2*61*128)
	r.buildGlobe(int(r.params[PLat]), int(r.params[PLon]), float64(r.params[PTwist]))
	r.lastSeq = r.params[PGeomSeq]

	resize := func() {
		r.width = js.Global().Get("innerWidth").Int()
		r.height = js.Global().Get("innerHeight").Int()
		canvas.Set("width", r.width)
		canvas.Set("height", r.height)
		gl.Call("viewport", 0, 0, r.width, r.height)
	}
	js.Global().Call("addEventListener", "resize", js.FuncOf(func(js.Value, []js.Value) interface{} {
		resize()
		return nil
	}))
	resize()
	gl.Call("enable", gl.Get("DEPTH_TEST"))

	// Matrices are held once and rewritten in place; the typed arrays handed to
	// uniformMatrix4fv are allocated here and never again.
	var pm, vm, mm [16]float32
	pj, vj, mj := f32Array(pm[:]), f32Array(vm[:]), f32Array(mm[:])
	pjU8 := js.Global().Get("Uint8Array").New(pj.Get("buffer"))
	vjU8 := js.Global().Get("Uint8Array").New(vj.Get("buffer"))
	mjU8 := js.Global().Get("Uint8Array").New(mj.Get("buffer"))

	allocKB := frameAllocKBFromQuery()
	r.frameFn = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		allocFrame(allocKB / 2)
		allocFragmented(allocKB / 2)
		r.readParams()

		// Geometry is rebuilt only when the control plane says so, which is what
		// keeps the frame from allocating.
		if r.params[PGeomSeq] != r.lastSeq {
			r.lastSeq = r.params[PGeomSeq]
			r.buildGlobe(int(r.params[PLat]), int(r.params[PLon]), float64(r.params[PTwist]))
		}

		r.angleX += r.params[PRotX] / 60
		r.angleY += r.params[PRotY] / 60
		if r.params[PAuto] >= 0.5 {
			r.angleY += 0.1 / 6
		}
		r.angleZ += r.params[PRotZ] / 60

		asp := float32(1.6)
		if r.height > 0 {
			asp = float32(r.width) / float32(r.height)
		}
		p := mgl32.Perspective(mgl32.DegToRad(45), asp, 0.1, 1000)
		dist := 4.0 - r.params[PZoom]
		if dist < 0.5 {
			dist = 0.5
		}
		v := mgl32.LookAtV(
			mgl32.Vec3{r.params[PPanX], r.params[PPanY], dist},
			mgl32.Vec3{r.params[PPanX], r.params[PPanY], 0},
			mgl32.Vec3{0, 1, 0})
		m := mgl32.HomogRotate3DY(r.angleY).
			Mul4(mgl32.HomogRotate3DX(r.angleX)).
			Mul4(mgl32.HomogRotate3DZ(r.angleZ))
		pm, vm, mm = p, v, m

		js.CopyBytesToJS(pjU8, f32Bytes(pm[:]))
		js.CopyBytesToJS(vjU8, f32Bytes(vm[:]))
		js.CopyBytesToJS(mjU8, f32Bytes(mm[:]))
		gl.Call("uniformMatrix4fv", r.uP, false, pj)
		gl.Call("uniformMatrix4fv", r.uV, false, vj)
		gl.Call("uniformMatrix4fv", r.uM, false, mj)
		gl.Call("uniform3f", r.uColor, r.params[PR], r.params[PG], r.params[PB])

		gl.Call("clearColor", 0.02, 0.03, 0.06, 1)
		gl.Call("clear", gl.Get("COLOR_BUFFER_BIT"))
		gl.Call("clear", gl.Get("DEPTH_BUFFER_BIT"))
		gl.Call("drawElements", gl.Get("LINES"), r.idxCount, gl.Get("UNSIGNED_SHORT"), 0)

		js.Global().Call("requestAnimationFrame", r.frameFn)
		return nil
	})
	js.Global().Call("requestAnimationFrame", r.frameFn)
}
