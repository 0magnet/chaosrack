//go:build js && wasm

package attractor

import (
	"math"
	"runtime"
	"syscall/js"
	"unsafe"
)

// GLTypes holds WebGL constant values.
type GLTypes struct {
	StaticDraw         js.Value
	ArrayBuffer        js.Value
	ElementArrayBuffer js.Value
	VertexShader       js.Value
	FragmentShader     js.Value
	Float              js.Value
	DepthTest          js.Value
	ColorBufferBit     js.Value
	DepthBufferBit     js.Value
	Triangles          js.Value
	UnsignedShort      js.Value
	LEqual             js.Value
	LineLoop           js.Value
	Line               js.Value
	LineStrip          js.Value
	Lines              js.Value
	Points             js.Value
	DynamicDraw        js.Value
}

func (types *GLTypes) New(gl js.Value) {
	types.StaticDraw = gl.Get("STATIC_DRAW")
	types.ArrayBuffer = gl.Get("ARRAY_BUFFER")
	types.ElementArrayBuffer = gl.Get("ELEMENT_ARRAY_BUFFER")
	types.VertexShader = gl.Get("VERTEX_SHADER")
	types.FragmentShader = gl.Get("FRAGMENT_SHADER")
	types.Float = gl.Get("FLOAT")
	types.DepthTest = gl.Get("DEPTH_TEST")
	types.ColorBufferBit = gl.Get("COLOR_BUFFER_BIT")
	types.Triangles = gl.Get("TRIANGLES")
	types.UnsignedShort = gl.Get("UNSIGNED_SHORT")
	types.LEqual = gl.Get("LEQUAL")
	types.DepthBufferBit = gl.Get("DEPTH_BUFFER_BIT")
	types.LineLoop = gl.Get("LINE_LOOP")
	types.Line = gl.Get("LINES")
	types.LineStrip = gl.Get("LINE_STRIP")
	types.Lines = gl.Get("LINES")
	types.Points = gl.Get("POINTS")
	types.DynamicDraw = gl.Get("DYNAMIC_DRAW")
}

// updateGradientRange scans vertices and sets min/max uniforms for x, y, and z.
// Stride is gradientStride floats per vertex: 4 for interleaved
// attractor data (x,y,z,t), 3 for packed indexed geometry (x,y,z).
// Only called on mode/param change, NOT per frame.
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
	gl.Call("uniform1f", uMinXLoc, float64(minX))
	gl.Call("uniform1f", uMaxXLoc, float64(maxX))
	gl.Call("uniform1f", uMinYLoc, float64(minY))
	gl.Call("uniform1f", uMaxYLoc, float64(maxY))
	gl.Call("uniform1f", uMinZLoc, float64(minZ))
	gl.Call("uniform1f", uMaxZLoc, float64(maxZ))
}

// uploadVerticesOnly uploads vertex data and draws with drawArrays (no index buffer).
// Subtracts a stable centerOffset (computed once on mode change) so rotations work naturally.
// Uses persistent JS typed arrays for zero per-frame JS allocation.
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
	gradientStride = 4
	// Set stride-4 attribute pointers for interleaved data
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
	gl.Call("vertexAttribPointer", positionLoc, 3, glTypes.Float, false, 16, 0)
	gl.Call("enableVertexAttribArray", positionLoc)
	gl.Call("vertexAttribPointer", aTrailTLoc, 1, glTypes.Float, false, 16, 12)
	gl.Call("enableVertexAttribArray", aTrailTLoc)
	js.CopyBytesToJS(jsVertUint8, sliceToByteSlice(vertices))
	runtime.KeepAlive(vertices)
	gl.Call("bufferData", glTypes.ArrayBuffer, jsVertFloat, glTypes.StaticDraw)
	uploadDwell(vertices, n)
	gl.Call("uniform1f", uTrailHeadLoc, 0) // scan frames are head-less (ring mode sets its own)
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
	gl.Call("drawArrays", drawMode, first, drawN)
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
		dwellGL = gl.Call("createBuffer")
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
	gl.Call("bindBuffer", glTypes.ArrayBuffer, dwellGL)
	js.CopyBytesToJS(jsDwellU8, sliceToByteSlice(dwellBuf))
	gl.Call("bufferData", glTypes.ArrayBuffer, jsDwellF32, glTypes.DynamicDraw)
	gl.Call("vertexAttribPointer", aDwellLoc, 1, glTypes.Float, false, 0, 0)
	gl.Call("enableVertexAttribArray", aDwellLoc)
	// leave ARRAY_BUFFER bound to the vertex buffer for any later subdata
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
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
		gradientStride = 3
		gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
		// Switch to packed xyz stride for indexed geometry
		gl.Call("vertexAttribPointer", positionLoc, 3, glTypes.Float, false, 0, 0)
		gl.Call("enableVertexAttribArray", positionLoc)
		gl.Call("disableVertexAttribArray", aTrailTLoc)
		gl.Call("disableVertexAttribArray", aDwellLoc)
		gl.Call("vertexAttrib1f", aDwellLoc, 1.0)
		gl.Call("vertexAttrib1f", aTrailTLoc, 0.0)
		gl.Call("bufferData", glTypes.ArrayBuffer, SliceToTypedArray(attractorVertices), glTypes.StaticDraw)
		gl.Call("bindBuffer", glTypes.ElementArrayBuffer, attractorIndexBuffer)
		gl.Call("bufferData", glTypes.ElementArrayBuffer, SliceToTypedArray(attractorIndices), glTypes.StaticDraw)
		staticGeomDirty = false
	}
	gl.Call("drawElements", drawMode, len(attractorIndices), glTypes.UnsignedShort, 0)
}

func sliceToByteSlice(s interface{}) []byte {
	switch s := s.(type) {
	case []int8:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s))
	case []int16:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2)
	case []int32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4)
	case []int64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8)
	case []uint8:
		return s
	case []uint16:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2)
	case []uint32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4)
	case []uint64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8)
	case []float32:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4)
	case []float64:
		return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8)
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
