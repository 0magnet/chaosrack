// WebGL type bindings + slice-to-typed-array helpers.
package stlview

import (
	r "reflect"
	"runtime"
	"syscall/js"
	u "unsafe"
)

// GLTypes provides WebGL bindings.
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
	UnsignedInt        js.Value
	LEqual             js.Value
	LineLoop           js.Value
	Line               js.Value
}

// New grabs the WebGL bindings from a GL context.
func (types *GLTypes) New(gl js.Value) js.Value {
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
	enabled := gl.Call("getExtension", "OES_element_index_uint")
	if !enabled.Truthy() {
		return js.Global().Get("Error").New("missing extension: OES_element_index_uint")
	}
	types.UnsignedInt = gl.Get("UNSIGNED_INT")
	return js.Null()
}

func sliceToByteSlice(s interface{}) []byte {
	switch s := s.(type) {
	case []int8:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		return *(*[]byte)(u.Pointer(h))      // nolint
	case []int16:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 2
		h.Cap *= 2
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []int32:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []int64:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []uint8:
		return s
	case []uint16:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 2
		h.Cap *= 2
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []uint32:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []uint64:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []float32:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(u.Pointer(h)) // nolint
	case []float64:
		h := (*r.SliceHeader)(u.Pointer(&s)) // nolint
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(u.Pointer(h)) // nolint
	default:
		//		panic("jsutil: unexpected value at sliceToBytesSlice: " + r.TypeOf(s).String())
		panic("jsutil: unexpected value at sliceToBytesSlice: ")
	}
}

const (
	bo  = "byteOffset"
	bl  = "byteLength"
	bb  = "buffer"
	u8a = "Uint8Array"
)

// S2TA converts Slice To TypedArray
func S2TA(s interface{}) js.Value {
	switch s := s.(type) {
	case []int8:
		a := js.Global().Get(u8a).New(len(s))
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Int8Array").New(buf, a.Get(bo), a.Get(bl))
	case []int16:
		a := js.Global().Get(u8a).New(len(s) * 2)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Int16Array").New(buf, a.Get(bo), a.Get(bl).Int()/2)
	case []int32:
		a := js.Global().Get(u8a).New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Int32Array").New(buf, a.Get(bo), a.Get(bl).Int()/4)
	case []uint8:
		a := js.Global().Get(u8a).New(len(s))
		js.CopyBytesToJS(a, s)
		runtime.KeepAlive(s)
		return a
	case []uint16:
		a := js.Global().Get(u8a).New(len(s) * 2)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Uint16Array").New(buf, a.Get(bo), a.Get(bl).Int()/2)
	case []uint32:
		a := js.Global().Get(u8a).New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Uint32Array").New(buf, a.Get(bo), a.Get(bl).Int()/4)
	case []float32:
		a := js.Global().Get(u8a).New(len(s) * 4)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Float32Array").New(buf, a.Get(bo), a.Get(bl).Int()/4)
	case []float64:
		a := js.Global().Get(u8a).New(len(s) * 8)
		js.CopyBytesToJS(a, sliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get(bb)
		return js.Global().Get("Float64Array").New(buf, a.Get(bo), a.Get(bl).Int()/8)
	default:
		//		panic("jsutil: unexpected value at S2TA: " + r.TypeOf(s).String())
		panic("jsutil: unexpected value at S2TA: ")
	}
}
