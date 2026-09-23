//go:build js && wasm

package attractor

import (
	"math"
	"runtime"
	"syscall/js"
	"unsafe"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

func updateGradientRange(vertices []float32) {
	stride := gradientStride
	if !shadersReady || len(vertices) < stride {
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
	glctx.GL.Call("uniform1f", uMinXLoc, float64(minX))
	glctx.GL.Call("uniform1f", uMaxXLoc, float64(maxX))
	glctx.GL.Call("uniform1f", uMinYLoc, float64(minY))
	glctx.GL.Call("uniform1f", uMaxYLoc, float64(maxY))
	glctx.GL.Call("uniform1f", uMinZLoc, float64(minZ))
	glctx.GL.Call("uniform1f", uMaxZLoc, float64(maxZ))
}

// uploadVerticesOnly uploads vertex data and draws with drawArrays (no index buffer).
// Subtracts a stable centerOffset (computed once on mode change) so rotations work naturally.
// Uses persistent JS typed arrays for zero per-frame JS allocation.
// lastDrawnCount is how many vertices the last drawArrays actually asked for.
// Pausing redraws without regenerating, and most modes fill the whole trail
// buffer so `steps` is the same number — but a mode that draws fewer (a turtle
// path shorter than its trail, a scatter still filling up) would otherwise have
// the paused frame draw whatever stale vertices were left beyond its own.
var lastDrawnCount int

func uploadVerticesOnly(vertices []float32, drawMode js.Value, count int) {
	n := len(vertices) / 4
	if n > 0 {
		if !centerReady {
			centerWarmup++
			var cx, cy, cz float32
			for i := 0; i < len(vertices); i += 4 {
				cx += vertices[i]
				cy += vertices[i+1]
				cz += vertices[i+2]
			}
			inv := 1.0 / float32(n)
			centerOffset = [3]float32{cx * inv, cy * inv, cz * inv}
			if centerWarmup >= 30 {
				centerReady = true
			}
		}
		for i := 0; i < len(vertices); i += 4 {
			vertices[i] -= centerOffset[0]
			vertices[i+1] -= centerOffset[1]
			vertices[i+2] -= centerOffset[2]
		}
	}
	attractorVertices = vertices
	vertexUploadSeq++
	gradientStride = 4
	// Set stride-4 attribute pointers for interleaved data
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, attractorVertexBuffer)
	glctx.GL.Call("vertexAttribPointer", positionLoc, 3, glctx.Types.Float, false, 16, 0)
	glctx.GL.Call("enableVertexAttribArray", positionLoc)
	glctx.GL.Call("vertexAttribPointer", aTrailTLoc, 1, glctx.Types.Float, false, 16, 12)
	glctx.GL.Call("enableVertexAttribArray", aTrailTLoc)
	js.CopyBytesToJS(jsVertUint8, sliceToByteSlice(vertices))
	runtime.KeepAlive(vertices)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, jsVertFloat, glctx.Types.StaticDraw)
	uploadDwell(vertices, n)
	glctx.GL.Call("uniform1f", uTrailHeadLoc, 0) // scan frames are head-less (ring mode sets its own)
	// Audio-modulated trail length: draw only the most-recent frac·count points
	// (a shorter line-strip tail) — no buffer realloc. frac==1 draws it all.
	first := 0
	drawN := count
	if trailModFrac < 0.999 && count > 2 {
		drawN = int(float32(count) * trailModFrac)
		if drawN < 2 {
			drawN = 2
		}
		first = count - drawN
	}
	lastDrawnCount = drawN
	glctx.GL.Call("drawArrays", drawMode, first, drawN)
}

// Beam-dwell exposure: per-vertex brightness ∝ how long the beam lingered
// there — mean step distance over local step distance, soft-clamped, so the
// trail reads like a real CRT trace (slow arcs glow, fast excursions ghost).
var (
	dwellBuf   []float32
	dwellGL    js.Value
	jsDwellU8  js.Value
	jsDwellF32 js.Value
)

func uploadDwell(vertices []float32, n int) {
	if n < 2 {
		return
	}
	if len(dwellBuf) != n {
		dwellBuf = make([]float32, n)
		jsDwellU8 = js.Global().Get("Uint8Array").New(n * 4)
		jsDwellF32 = js.Global().Get("Float32Array").New(jsDwellU8.Get("buffer"), 0, n)
	}
	if dwellGL.IsUndefined() {
		dwellGL = glctx.GL.Call("createBuffer")
	}
	// mean step distance (squared math avoided: one sqrt per point)
	var total float32
	for i := 1; i < n; i++ {
		a, b := (i-1)*4, i*4
		dx := vertices[b] - vertices[a]
		dy := vertices[b+1] - vertices[a+1]
		dz := vertices[b+2] - vertices[a+2]
		d := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
		dwellBuf[i] = d
		total += d
	}
	mean := total / float32(n-1)
	if mean <= 0 {
		mean = 1e-6
	}
	dwellBuf[0] = 1
	for i := 1; i < n; i++ {
		w := mean / (dwellBuf[i] + mean*0.15) // 1.0 at mean speed, ≤~6.7 when parked
		if w > 1.8 {
			w = 1.8
		} else if w < 0.25 {
			w = 0.25
		}
		dwellBuf[i] = w
	}
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, dwellGL)
	js.CopyBytesToJS(jsDwellU8, sliceToByteSlice(dwellBuf))
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, jsDwellF32, glctx.Types.DynamicDraw)
	glctx.GL.Call("vertexAttribPointer", aDwellLoc, 1, glctx.Types.Float, false, 0, 0)
	glctx.GL.Call("enableVertexAttribArray", aDwellLoc)
	// leave ARRAY_BUFFER bound to the vertex buffer for any later subdata
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, attractorVertexBuffer)
}

// uploadBuffersIndexed uploads and draws with drawElements.
// Uses packed stride-0 (xyz only), disabling the trail attribute.
//
// Only does the full upload (bind, attribute setup, bufferData with
// fresh SliceToTypedArray allocations) when staticGeomDirty is set
// — i.e. on mode change, param change, or Reset. For all other
// frames we go straight to drawElements with the still-bound
// buffers, eliminating the per-frame CPU cost of regenerating the
// JS typed arrays and pushing identical data to the GPU.
func uploadBuffersIndexed(vertices []float32, indices []uint16, drawMode js.Value) {
	if staticGeomDirty {
		attractorVertices = vertices
		attractorIndices = indices
		vertexUploadSeq++
		gradientStride = 3
		glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, attractorVertexBuffer)
		// Switch to packed xyz stride for indexed geometry
		glctx.GL.Call("vertexAttribPointer", positionLoc, 3, glctx.Types.Float, false, 0, 0)
		glctx.GL.Call("enableVertexAttribArray", positionLoc)
		glctx.GL.Call("disableVertexAttribArray", aTrailTLoc)
		glctx.GL.Call("disableVertexAttribArray", aDwellLoc)
		glctx.GL.Call("vertexAttrib1f", aDwellLoc, 1.0)
		glctx.GL.Call("vertexAttrib1f", aTrailTLoc, 0.0)
		glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(attractorVertices), glctx.Types.StaticDraw)
		glctx.GL.Call("bindBuffer", glctx.Types.ElementArrayBuffer, attractorIndexBuffer)
		glctx.GL.Call("bufferData", glctx.Types.ElementArrayBuffer, SliceToTypedArray(attractorIndices), glctx.Types.StaticDraw)
		staticGeomDirty = false
	}
	glctx.GL.Call("drawElements", drawMode, len(attractorIndices), glctx.Types.UnsignedShort, 0)
}

// staticGeomCached reports that the geometry already on the GPU is still what
// this frame wants, and draws it.
//
// The static models — the polyhedra, sphere, torus, globe, magnetosphere and a
// loaded STL — are a fixed mesh built from knob values, and every control that
// feeds one sets staticGeomDirty when it moves. uploadBuffersIndexed has always
// known that, and skipped the upload while the flag was clear. What it could
// not skip was the build: the generators handed it a freshly computed mesh on
// every frame and it threw all of them away but the first.
//
// That was the larger half of the cost. Building the globe's mesh is several
// thousand sin/cos and a few thousand appends, sixty times a second, to produce
// bytes identical to the ones already in the buffer. Under TinyGo's collector
// it was also most of the garbage in the program: profiling the browser put
// 45% of all allocation in generateGlobe alone, and the collector then stopped
// the frame for 66-100ms about every 400ms to sweep up after it, which is
// exactly the stutter that could be seen.
//
// So the generators now ask this first and return if the answer is yes.
func staticGeomCached(drawMode js.Value) bool {
	if staticGeomDirty {
		return false
	}
	glctx.GL.Call("drawElements", drawMode, len(attractorIndices), glctx.Types.UnsignedShort, 0)
	return true
}

func sliceToByteSlice(s interface{}) []byte {
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

func SliceToTypedArray(s interface{}) js.Value {
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
func setGradientRange(minX, maxX, minY, maxY, minZ, maxZ float32) {
	if !shadersReady {
		return
	}
	glctx.GL.Call("uniform1f", uMinXLoc, float64(minX-centerOffset[0]))
	glctx.GL.Call("uniform1f", uMaxXLoc, float64(maxX-centerOffset[0]))
	glctx.GL.Call("uniform1f", uMinYLoc, float64(minY-centerOffset[1]))
	glctx.GL.Call("uniform1f", uMaxYLoc, float64(maxY-centerOffset[1]))
	glctx.GL.Call("uniform1f", uMinZLoc, float64(minZ-centerOffset[2]))
	glctx.GL.Call("uniform1f", uMaxZLoc, float64(maxZ-centerOffset[2]))
}

// vertexUploadSeq counts uploads into the attractor vertex buffer.
//
// The gradient's extents are the only reader, and what they need to know is
// whether the buffer they are about to scan belongs to the mode now on screen
// or to the one before it. A mode name cannot answer that — the mode has
// changed and the buffer has not — and emptiness cannot either, because a mode
// that draws nothing on its first frames leaves the previous model's vertices
// in place rather than clearing them. An upload count answers it exactly.
var vertexUploadSeq uint64
