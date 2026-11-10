//go:build js && wasm

package glctx

import (
	"runtime"
	"syscall/js"
	"unsafe"
)

// Bytes is the bytes that back a numeric slice. One copy: the
// attractor and the STL viewer each had this pair, the viewer through the
// deprecated reflect.SliceHeader.
func Bytes(s any) []byte {
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
		panic("glctx.Bytes: not a numeric slice")
	}
}

// SliceToTypedArray copies a numeric slice into a new JS typed array of the
// matching element type, for handing to WebGL.
func SliceToTypedArray(s any) js.Value {
	switch s := s.(type) {
	case []int8:
		a := js.Global().Get("Uint8Array").New(len(s))
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int8Array").New(buf, a.Get("byteOffset"), a.Get("byteLength"))
	case []int16:
		a := js.Global().Get("Uint8Array").New(len(s) * 2)
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []int32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, Bytes(s))
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
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []uint32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float64:
		a := js.Global().Get("Uint8Array").New(len(s) * 8)
		js.CopyBytesToJS(a, Bytes(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float64Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/8)
	default:
		panic("unexpected value at SliceToTypedArray")
	}
}
