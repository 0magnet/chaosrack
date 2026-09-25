//go:build js && wasm

package attractor

import (
	"math"
	"runtime"
	"syscall/js"
	"unsafe"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

func (r *renderer) updateGradientRange(vertices []float32) {
	stride := r.stride
	if !r.ready || len(vertices) < stride {
		return
	}
	minX := float32(math.MaxFloat32)
	maxX := float32(-math.MaxFloat32)
	minY := float32(math.MaxFloat32)
	maxY := float32(-math.MaxFloat32)
	minZ := float32(math.MaxFloat32)
	maxZ := float32(-math.MaxFloat32)
	// Stop on the last full vertex; otherwise vertices[i+1] / [i+2]
	// index past the slice end when len(vertices) isn't a multiple
	// of stride (happens transiently while a buffer is repopulated).
	for i := 0; i+2 < len(vertices); i += stride {
		if vertices[i] < minX {
			minX = vertices[i]
		}
		if vertices[i] > maxX {
			maxX = vertices[i]
		}
		if vertices[i+1] < minY {
			minY = vertices[i+1]
		}
		if vertices[i+1] > maxY {
			maxY = vertices[i+1]
		}
		if vertices[i+2] < minZ {
			minZ = vertices[i+2]
		}
		if vertices[i+2] > maxZ {
			maxZ = vertices[i+2]
		}
	}
	glctx.GL.Call("uniform1f", r.u.minX, float64(minX))
	glctx.GL.Call("uniform1f", r.u.maxX, float64(maxX))
	glctx.GL.Call("uniform1f", r.u.minY, float64(minY))
	glctx.GL.Call("uniform1f", r.u.maxY, float64(maxY))
	glctx.GL.Call("uniform1f", r.u.minZ, float64(minZ))
	glctx.GL.Call("uniform1f", r.u.maxZ, float64(maxZ))
}

// uploadVerticesOnly uploads vertex data and draws with drawArrays (no index buffer).
// Subtracts a stable centerOffset (computed once on mode change) so rotations work naturally.
// Uses persistent JS typed arrays for zero per-frame JS allocation.
func (r *renderer) uploadVerticesOnly(vertices []float32, drawMode js.Value, count int) {
	n := len(vertices) / 4
	if n > 0 {
		if !sim.centerReady {
			sim.centerWarmup++
			var cx, cy, cz float32
			for i := 0; i < len(vertices); i += 4 {
				cx += vertices[i]
				cy += vertices[i+1]
				cz += vertices[i+2]
			}
			inv := 1.0 / float32(n)
			sim.centerOffset = [3]float32{cx * inv, cy * inv, cz * inv}
			if sim.centerWarmup >= 30 {
				sim.centerReady = true
			}
		}
		for i := 0; i < len(vertices); i += 4 {
			vertices[i] -= sim.centerOffset[0]
			vertices[i+1] -= sim.centerOffset[1]
			vertices[i+2] -= sim.centerOffset[2]
		}
	}
	r.verts = vertices
	r.uploadSeq++
	r.stride = 4
	// Set stride-4 attribute pointers for interleaved data
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, r.vbuf)
	glctx.GL.Call("vertexAttribPointer", r.aPosition, 3, glctx.Types.Float, false, 16, 0)
	glctx.GL.Call("enableVertexAttribArray", r.aPosition)
	glctx.GL.Call("vertexAttribPointer", r.aTrailT, 1, glctx.Types.Float, false, 16, 12)
	glctx.GL.Call("enableVertexAttribArray", r.aTrailT)
	js.CopyBytesToJS(r.vertU8, sliceToByteSlice(vertices))
	runtime.KeepAlive(vertices)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, r.vertF32, glctx.Types.StaticDraw)
	gpu.uploadDwell(vertices, n)
	glctx.GL.Call("uniform1f", r.u.trailHead, 0) // scan frames are head-less (ring mode sets its own)
	// Audio-modulated trail length: draw only the most-recent frac·count points
	// (a shorter line-strip tail) — no buffer realloc. frac==1 draws it all.
	first := 0
	drawN := count
	if style.trailModFrac < 0.999 && count > 2 {
		drawN = max(int(float32(count)*style.trailModFrac), 2)
		first = count - drawN
	}
	r.lastDrawn = drawN
	glctx.GL.Call("drawArrays", drawMode, first, drawN)
}

func (r *renderer) uploadDwell(vertices []float32, n int) {
	if n < 2 {
		return
	}
	if len(r.dwell.buf) != n {
		r.dwell.buf = make([]float32, n)
		r.dwell.u8 = js.Global().Get("Uint8Array").New(n * 4)
		r.dwell.f32 = js.Global().Get("Float32Array").New(r.dwell.u8.Get("buffer"), 0, n)
	}
	if r.dwell.gl.IsUndefined() {
		r.dwell.gl = glctx.GL.Call("createBuffer")
	}
	// mean step distance (squared math avoided: one sqrt per point)
	var total float32
	for i := 1; i < n; i++ {
		a, b := (i-1)*4, i*4
		dx := vertices[b] - vertices[a]
		dy := vertices[b+1] - vertices[a+1]
		dz := vertices[b+2] - vertices[a+2]
		d := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
		r.dwell.buf[i] = d
		total += d
	}
	mean := total / float32(n-1)
	if mean <= 0 {
		mean = 1e-6
	}
	r.dwell.buf[0] = 1
	for i := 1; i < n; i++ {
		w := mean / (r.dwell.buf[i] + mean*0.15) // 1.0 at mean speed, ≤~6.7 when parked
		if w > 1.8 {
			w = 1.8
		} else if w < 0.25 {
			w = 0.25
		}
		r.dwell.buf[i] = w
	}
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, r.dwell.gl)
	js.CopyBytesToJS(r.dwell.u8, sliceToByteSlice(r.dwell.buf))
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, r.dwell.f32, glctx.Types.DynamicDraw)
	glctx.GL.Call("vertexAttribPointer", r.aDwell, 1, glctx.Types.Float, false, 0, 0)
	glctx.GL.Call("enableVertexAttribArray", r.aDwell)
	// leave ARRAY_BUFFER bound to the vertex buffer for any later subdata
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, r.vbuf)
}

// uploadBuffersIndexed uploads and draws with drawElements.
// Uses packed stride-0 (xyz only), disabling the trail attribute.
//
// Only does the full upload (bind, attribute setup, bufferData with
// fresh SliceToTypedArray allocations) when gpu.staticDirty is set
// — i.e. on mode change, param change, or Reset. For all other
// frames we go straight to drawElements with the still-bound
// buffers, eliminating the per-frame CPU cost of regenerating the
// JS typed arrays and pushing identical data to the GPU.
func (r *renderer) uploadBuffersIndexed(vertices []float32, indices []uint16, drawMode js.Value) {
	if r.staticDirty {
		r.verts = vertices
		r.indices = indices
		r.uploadSeq++
		r.stride = 3
		glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, r.vbuf)
		// Switch to packed xyz stride for indexed geometry
		glctx.GL.Call("vertexAttribPointer", r.aPosition, 3, glctx.Types.Float, false, 0, 0)
		glctx.GL.Call("enableVertexAttribArray", r.aPosition)
		glctx.GL.Call("disableVertexAttribArray", r.aTrailT)
		glctx.GL.Call("disableVertexAttribArray", r.aDwell)
		glctx.GL.Call("vertexAttrib1f", r.aDwell, 1.0)
		glctx.GL.Call("vertexAttrib1f", r.aTrailT, 0.0)
		glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(r.verts), glctx.Types.StaticDraw)
		glctx.GL.Call("bindBuffer", glctx.Types.ElementArrayBuffer, r.ibuf)
		glctx.GL.Call("bufferData", glctx.Types.ElementArrayBuffer, SliceToTypedArray(r.indices), glctx.Types.StaticDraw)
		r.staticDirty = false
	}
	glctx.GL.Call("drawElements", drawMode, len(r.indices), glctx.Types.UnsignedShort, 0)
}

// staticGeomCached reports that the geometry already on the GPU is still what
// this frame wants, and draws it.
//
// The static models — the polyhedra, sphere, torus, globe, magnetosphere and a
// loaded STL — are a fixed mesh built from knob values, and every control that
// feeds one sets gpu.staticDirty when it moves. uploadBuffersIndexed has always
// known that, and skipped the upload while the flag was clear. What it could
// not skip was the build: the generators handed it a freshly computed mesh on
// every frame and it threw all of them away but the first.
//
// That was the larger half of the cost. Building the globe's mesh is several
// thousand sin/cos and a few thousand appends, sixty times a second, to produce
// bytes identical to the ones already in the buffer. Under TinyGo's collector
// it was also most of the garbage in the program: profiling the browser put
// 45% of all allocation in globe.generate alone, and the collector then stopped
// the frame for 66-100ms about every 400ms to sweep up after it, which is
// exactly the stutter that could be seen.
//
// So the generators now ask this first and return if the answer is yes.
func (r *renderer) staticGeomCached(drawMode js.Value) bool {
	if r.staticDirty {
		return false
	}
	glctx.GL.Call("drawElements", drawMode, len(r.indices), glctx.Types.UnsignedShort, 0)
	return true
}

func sliceToByteSlice(s any) []byte {
	switch s := s.(type) {
	case []int8:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []int16:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []int32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []int64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []uint8:
		return s
	case []uint16:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []uint32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []uint64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []float32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	case []float64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8) //nolint:gosec // reinterpreting a typed slice as the bytes that back it, to hand to WebGL; the length is exactly the element size times the count
	default:
		panic("unexpected value at sliceToByteSlice")
	}
}

// SliceToTypedArray copies a numeric slice into a new JS typed array of the
// matching element type, for handing to WebGL.
func SliceToTypedArray(s any) js.Value {
	switch s := s.(type) {
	case []int8:
		a := js.Global().Get("Uint8Array").New(len(s))
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int8Array").New(buf, a.Get("byteOffset"), a.Get("byteLength"))
	case []int16:
		a := js.Global().Get("Uint8Array").New(len(s) * 2)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []int32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []uint8:
		a := js.Global().Get("Uint8Array").New(len(s))
		js.CopyBytesToJS(a, s)
		runtime.KeepAlive(s)
		return a
	case []uint16:
		a := js.Global().Get("Uint8Array").New(len(s) * 2)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []uint32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float64:
		a := js.Global().Get("Uint8Array").New(len(s) * 8)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float64Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/8)
	default:
		panic("unexpected value at SliceToTypedArray")
	}
}

// setGradientRange sets the gradient's normalizing extents directly, for
// geometry whose bounds are known BY CONSTRUCTION rather than found by
// scanning it.
//
// updateGradientRange scans the vertex buffer, and its doc comment says why it
// is only called on a mode or parameter change: it is an O(n) pass over up to
// twenty thousand points and it is not per-frame work. That is fine for a model
// whose shape is settled by the time the mode is entered, and wrong for one
// built from live audio — the scan runs on the PREVIOUS model's vertices,
// because the new mode has not drawn yet, and the gradient then normalizes the
// new geometry against the old one's bounds. A waterfall entered from an
// attractor was colored across whatever slice of the colormap its coordinates
// happened to fall in inside the attractor's range, which is how six distinct
// colormaps all came out looking like one flat tint.
//
// A surface with known bounds does not need the scan at all. centerOffset is
// subtracted because uploadVerticesOnly subtracts it from the vertices, so the
// bounds have to move with them or they describe a figure that is no longer
// where it was.
func (r *renderer) setGradientRange(minX, maxX, minY, maxY, minZ, maxZ float32) {
	if !r.ready {
		return
	}
	glctx.GL.Call("uniform1f", r.u.minX, float64(minX-sim.centerOffset[0]))
	glctx.GL.Call("uniform1f", r.u.maxX, float64(maxX-sim.centerOffset[0]))
	glctx.GL.Call("uniform1f", r.u.minY, float64(minY-sim.centerOffset[1]))
	glctx.GL.Call("uniform1f", r.u.maxY, float64(maxY-sim.centerOffset[1]))
	glctx.GL.Call("uniform1f", r.u.minZ, float64(minZ-sim.centerOffset[2]))
	glctx.GL.Call("uniform1f", r.u.maxZ, float64(maxZ-sim.centerOffset[2]))
}
