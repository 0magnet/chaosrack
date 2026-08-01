//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// ── WebGL state ──────────────────────────────────────────────────────────────

// These are package-level for convenience (lots of helper functions
// across the package reach for them), but they MUST NOT be initialized
// at package-var time. When attractor is imported as a library (e.g.
// from m2/wasm/stl2), package-var init runs before the host's DOM is
// ready, so getElementById("gocanvas") returns null and the subsequent
// canvasEl.Call("getContext", "webgl") panics with
// "syscall/js: call of Value.Call on null". initWebGL(), called from
// Run() once the canvas exists, populates them.
var (
	doc      js.Value
	body     js.Value
	canvasEl js.Value
	width    int
	height   int
	gl       js.Value

	shaderProgram         js.Value
	attractorVertexBuffer js.Value
	attractorIndexBuffer  js.Value
	attractorVertices     []float32
	attractorIndices      []uint16

	// gradientStride is the number of floats per vertex in
	// attractorVertices, set by whichever upload path last ran:
	// 4 for interleaved attractor data (x,y,z,t) via
	// uploadVerticesOnly, 3 for packed xyz indexed geometry via
	// uploadBuffersIndexed. updateGradientRange reads it so it scans
	// the right stride instead of assuming 4 (which would misread the
	// min/max range for polyhedra and other indexed modes).
	gradientStride = 4

	glTypes GLTypes
)

// sizeCanvasToViewport sizes the canvas BACKING STORE to CSS-viewport ×
// devicePixelRatio (capped at 3) while pinning the element's CSS size to CSS
// pixels — so HiDPI displays render the scope traces at native resolution
// instead of a soft 1× upscale. Reports whether a valid size was applied.
// width/height globals are backing-store pixels (aspect and NDC math are
// ratio-based, so both stay correct).
func sizeCanvasToViewport() bool {
	cssW := doc.Get("body").Get("clientWidth").Int()
	cssH := doc.Get("body").Get("clientHeight").Int()
	if cssW <= 0 || cssH <= 0 {
		return false
	}
	d := js.Global().Get("devicePixelRatio").Float()
	if d < 1 {
		d = 1
	} else if d > 3 {
		d = 3 // beyond 3× the fill cost outruns any visible sharpness gain
	}
	width = int(float64(cssW) * d)
	height = int(float64(cssH) * d)
	canvasEl.Set("width", width)
	canvasEl.Set("height", height)
	st := canvasEl.Get("style")
	st.Set("width", strconv.Itoa(cssW)+"px")
	st.Set("height", strconv.Itoa(cssH)+"px")
	return true
}

func initWebGL() {
	doc = js.Global().Get("document")
	body = doc.Get("body")
	canvasEl = doc.Call("getElementById", "gocanvas")
	if canvasEl.IsUndefined() || canvasEl.IsNull() {
		return
	}
	opts := js.Global().Get("Object").New()
	opts.Set("preserveDrawingBuffer", true)
	gl = canvasEl.Call("getContext", "webgl", opts)
	sizeCanvasToViewport()
	if gl.IsUndefined() {
		gl = canvasEl.Call("getContext", "experimental-webgl", opts)
	}
	if gl.IsUndefined() {
		js.Global().Call("alert", "browser might not support webgl")
		return
	}
	shaderProgram = gl.Call("createProgram")
	attractorVertexBuffer = gl.Call("createBuffer")
	attractorIndexBuffer = gl.Call("createBuffer")
}
