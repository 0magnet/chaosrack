//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
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
	// width and height are the canvas BACKING STORE size in device pixels,
	// which is not the element's CSS size — see sizeCanvasToViewport. They
	// stay here rather than moving to pkg/glctx with the context because they
	// are not the platform's, they are this app's idea of how big to draw.
	width  int
	height int

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
)

// sizeCanvasToViewport sizes the canvas BACKING STORE to CSS-viewport ×
// devicePixelRatio (capped at 3) while pinning the element's CSS size to CSS
// pixels — so HiDPI displays render the scope traces at native resolution
// instead of a soft 1× upscale. Reports whether a valid size was applied.
// width/height globals are backing-store pixels (aspect and NDC math are
// ratio-based, so both stay correct).
func sizeCanvasToViewport() bool {
	// The VIEWPORT, not the body.
	//
	// documentElement.clientWidth/Height is the viewport minus any scrollbars,
	// which is exactly what a full-window backdrop wants. body.clientHeight is
	// the body's own content height, and the two are only equal while the page
	// cannot scroll. On a host whose document is taller than the window — a
	// catalog under the model — the body measurement sized the drawing buffer to
	// the whole document: a 2364x7908 canvas, six times the pixels needed, for a
	// picture that is only ever a window tall.
	cssW := dom.Doc.Get("documentElement").Get("clientWidth").Int()
	cssH := dom.Doc.Get("documentElement").Get("clientHeight").Int()
	if cssW <= 0 || cssH <= 0 {
		// A document with no layout yet; the body is the older fallback.
		cssW = dom.Doc.Get("body").Get("clientWidth").Int()
		cssH = dom.Doc.Get("body").Get("clientHeight").Int()
	}
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
	glctx.Canvas.Set("width", width)
	glctx.Canvas.Set("height", height)
	st := glctx.Canvas.Get("style")
	st.Set("width", strconv.Itoa(cssW)+"px")
	st.Set("height", strconv.Itoa(cssH)+"px")
	// The near-side canvas is the same picture on the other side of the panel,
	// so it is sized here rather than anywhere else — one place decides how big
	// a frame is, and a copy between two canvases of different sizes would be a
	// scale nobody asked for.
	sizeFrontCanvas()
	return true
}

func initWebGL() {
	dom.Init()
	if !glctx.Init(dom.Doc.Call("getElementById", "gocanvas")) {
		// A missing canvas and a missing context are different failures. No
		// canvas means this is not the page we draw on — say nothing. A canvas
		// with no context means the browser cannot do it, which the user needs
		// telling about, because the alternative is a blank rectangle.
		if glctx.Canvas.Truthy() {
			js.Global().Call("alert", "browser might not support webgl")
		}
		return
	}
	sizeCanvasToViewport()
	shaderProgram = glctx.GL.Call("createProgram")
	attractorVertexBuffer = glctx.GL.Call("createBuffer")
	attractorIndexBuffer = glctx.GL.Call("createBuffer")
}
