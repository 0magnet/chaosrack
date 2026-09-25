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
// So the points go over as a Float32Array — one memcpy, one crossing — and a
// four-line JS loop walks it. The profile's appendNum, FormatFloat, ftoa64
// and loadString entries all belonged to this and all of them go away.
const scopeFastSource = `(function () {
  return {
    // stroke walks n floats of the buffer as x,y pairs. beginPath through
    // stroke stays on this side so the whole sweep is one crossing.
    stroke: function (ctx, pts, n) {
      if (n < 4) return;
      ctx.beginPath();
      ctx.moveTo(pts[0], pts[1]);
      for (var i = 2; i < n; i += 2) ctx.lineTo(pts[i], pts[i + 1]);
      ctx.stroke();
    }
  };
})()`

// scopeBeam is the beam handed to JavaScript as numbers.
type scopeBeam struct {
	helper js.Value
	tried  bool

	// The shared buffer the points travel in, and the two views onto it:
	// Go copies bytes through the Uint8Array, JS reads floats through the
	// Float32Array. Grown rather than reallocated — a fresh pair per frame
	// is two finalized js.Values per frame for no reason.
	ptsF32 js.Value
	ptsU8  js.Value
	ptsCap int
}

var sfast scopeBeam

// fast is the JS helper, or a zero Value on a page that will not
// evaluate it. Tried once; the recover is the point, because a
// Content-Security-Policy that forbids eval reaches Go as a panic and the
// caller still has its path-string route. See fastDOM, which does the same.
func (s *scopeBeam) fast() (v js.Value) {
	if s.tried {
		return s.helper
	}
	s.tried = true
	defer func() {
		if recover() != nil {
			s.helper, v = js.Value{}, js.Value{}
		}
	}()
	s.helper = js.Global().Call("eval", scopeFastSource)
	return s.helper
}

// ptsArrays returns the Float32Array and Uint8Array views, big enough
// for n floats, or false if this page cannot make them.
func (s *scopeBeam) ptsArrays(n int) (f32, u8 js.Value, ok bool) {
	if n <= 0 {
		return js.Value{}, js.Value{}, false
	}
	if s.ptsCap >= n && s.ptsF32.Truthy() {
		return s.ptsF32, s.ptsU8, true
	}
	ab := js.Global().Get("ArrayBuffer")
	f32c := js.Global().Get("Float32Array")
	u8c := js.Global().Get("Uint8Array")
	if !ab.Truthy() || !f32c.Truthy() || !u8c.Truthy() {
		return js.Value{}, js.Value{}, false
	}
	// Headroom so turning the TIME/DIV knob does not reallocate on every
	// detent on the way round.
	capacity := n + n/2
	buf := ab.New(capacity * 4)
	s.ptsF32 = f32c.New(buf)
	s.ptsU8 = u8c.New(buf)
	s.ptsCap = capacity
	return s.ptsF32, s.ptsU8, true
}

// strokeScopePoints draws pts (x,y pairs) as one polyline. Reports whether
// it ran; false means the caller owes the stroke by its own route.
func strokeScopePoints(ctx js.Value, pts []float32) bool {
	h := sfast.fast()
	if !h.Truthy() || !ctx.Truthy() || len(pts) < 4 {
		return false
	}
	f32, u8, ok := sfast.ptsArrays(len(pts))
	if !ok {
		return false
	}
	js.CopyBytesToJS(u8, sliceToByteSlice(pts))
	h.Call("stroke", ctx, f32, len(pts))
	return true
}
