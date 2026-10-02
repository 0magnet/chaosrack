//go:build js && wasm

package attractor

import "syscall/js"

// Handing the beam over as numbers instead of as text.
//
// The sweep used to be built as an SVG path string — "M120.5 47.3L120.9
// 48.1…" — one strconv.FormatFloat per coordinate, two per point, then the
// whole string crossed into JS and handed to Path2D, which parsed the
// decimal back into the floats Go had a moment earlier. A 2400-point sweep
// is a 28 KB string built and re-parsed sixty times a second.
//
// The choice was deliberate and the reasoning was right as far as it went: a
// moveTo/lineTo per sample is a Go→JS crossing per sample, and crossings are
// what this codebase has learned to count. But the encoding was the trap
// fastdom_js.go's comment warns about from the other direction — formatting
// cost more than the crossings it saved. Measured in the page: building the
// string is 1.3 ms a frame and Path2D's parse another 0.18, against 0.14 ms
// for a moveTo/lineTo loop driven from JS over a typed array.
//
// So the points go over as a Float32Array — one memcpy, one crossing — and
// the helper's scopeStroke (fastdom_js.go) walks it.

// scopeBeam is the buffer the points travel in, and the two views onto it:
// Go copies bytes through the Uint8Array, JS reads floats through the
// Float32Array. Grown rather than reallocated — a fresh pair per frame is two
// finalized js.Values per frame for no reason.
type scopeBeam struct {
	ptsF32 js.Value
	ptsU8  js.Value
	ptsCap int
}

var sfast scopeBeam

// ptsArrays returns the Float32Array and Uint8Array views, big enough for n
// floats.
func (s *scopeBeam) ptsArrays(n int) (f32, u8 js.Value) {
	if s.ptsCap >= n && s.ptsF32.Truthy() {
		return s.ptsF32, s.ptsU8
	}
	// Headroom so turning the TIME/DIV knob does not reallocate on every
	// detent on the way round.
	capacity := n + n/2
	buf := js.Global().Get("ArrayBuffer").New(capacity * 4)
	s.ptsF32 = js.Global().Get("Float32Array").New(buf)
	s.ptsU8 = js.Global().Get("Uint8Array").New(buf)
	s.ptsCap = capacity
	return s.ptsF32, s.ptsU8
}

// strokeScopePoints draws pts (x,y pairs) as one polyline.
func strokeScopePoints(ctx js.Value, pts []float32) {
	if !ctx.Truthy() || len(pts) < 4 {
		return
	}
	f32, u8 := sfast.ptsArrays(len(pts))
	js.CopyBytesToJS(u8, sliceToByteSlice(pts))
	fastDOM().Call("scopeStroke", ctx, f32, len(pts))
}
