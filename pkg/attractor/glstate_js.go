//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/go-gl/mathgl/mgl32"
	"strconv"
	"syscall/js"
)

// ── WebGL state ──────────────────────────────────────────────────────────────

// renderer is the main line/point pipeline: the program every attractor,
// curve and polyhedron is drawn with, its buffers and the locations it was
// linked with, and the scratch the per-frame upload reuses. The textured,
// vertex-color, water and XY passes are their own programs and keep their
// own state beside the file that draws them.
//
// Nothing in it may be filled at package-init time. When attractor is
// imported as a library (m2/wasm/stl2), package init runs before the host's
// DOM is ready, getElementById("gocanvas") returns null and getContext
// panics with "syscall/js: call of Value.Call on null". gpu.initWebGL, called
// from Run once the canvas exists, fills it.
type renderer struct {
	// width and height are the canvas BACKING STORE size in device pixels,
	// which is not the element's CSS size — see sizeCanvasToViewport.
	width, height int

	program js.Value
	vbuf    js.Value // vertex buffer
	ibuf    js.Value // index buffer, for the indexed geometry path

	// verts and indices are the geometry last uploaded, kept for the
	// gradient-range scan and for anything that re-reads the frame.
	verts   []float32
	indices []uint16

	// stride is the number of floats per vertex in verts, set by whichever
	// upload path last ran: 4 for interleaved attractor data (x,y,z,t) via
	// gpu.uploadVerticesOnly, 3 for packed xyz indexed geometry via
	// uploadBuffersIndexed. updateGradientRange reads it so it scans the
	// right stride instead of assuming 4, which would misread the range for
	// polyhedra and the other indexed modes.
	stride int

	// uploadSeq counts uploads into the vertex buffer.
	//
	// The gradient's extents are the only reader, and what they need to know
	// is whether the buffer they are about to scan belongs to the mode now on
	// screen or to the one before it. A mode name cannot answer that — the
	// mode has changed and the buffer has not — and emptiness cannot either,
	// because a mode that draws nothing on its first frames leaves the
	// previous model's vertices in place rather than clearing them. An upload
	// count answers it exactly.
	uploadSeq uint64

	// lastDrawn is how many vertices the last drawArrays actually asked for.
	// Pausing redraws without regenerating, and most modes fill the whole
	// trail buffer so `steps` is the same number — but a mode that draws
	// fewer (a turtle path shorter than its trail, a scatter still filling
	// up) would otherwise have the paused frame draw whatever stale vertices
	// were left beyond its own.
	lastDrawn int

	// staticDirty is set when a non-attractor mode's geometry needs
	// re-uploading (mode change or param change). uploadBuffersIndexed
	// clears it after the upload; the frames after that skip the
	// SliceToTypedArray and bufferData work and go straight to drawElements
	// with the still-bound buffers.
	staticDirty bool

	// drawMode is the GL draw mode (LineStrip or Points), set once the GL
	// type constants are known.
	drawMode js.Value

	// vertU8 and vertF32 are one JS ArrayBuffer seen two ways, allocated
	// once and reused every frame to avoid GC pressure: CopyBytesToJS writes
	// through the bytes, bufferData reads the floats.
	vertU8  js.Value
	vertF32 js.Value

	// dwell is the beam-dwell attribute; see uploadDwell.
	dwell dwellAttr

	// proj is the projection matrix, kept so the textured program can be
	// fed the one the attractor program uses.
	proj mgl32.Mat4

	aPosition, aTrailT, aDwell js.Value // attribute locations
	u                          uniforms

	// ready is set once the shaders are linked and the uniforms located.
	ready bool
}

// uniforms are the main program's uniform locations, named for the GLSL
// uniform each one addresses (Mmatrix and Vmatrix become model and view).
type uniforms struct {
	baseColor, topColor, midColor js.Value
	minX, maxX                    js.Value
	minY, maxY                    js.Value
	minZ, maxZ                    js.Value
	gradientSource                js.Value
	gradientColors                js.Value
	gradientFreq                  js.Value
	gradientPhase                 js.Value
	gradientReverse               js.Value
	palette, paletteShift         js.Value
	audioLUT                      js.Value
	dashDuty, dashCount           js.Value
	pointSize                     js.Value
	model, view                   js.Value
	trailHead                     js.Value
	splitZ, splitSide             js.Value
}

// dwellAttr is the per-vertex beam-dwell brightness: how long the beam
// lingered at each point — mean step distance over local step distance,
// soft-clamped, so the trail reads like a real CRT trace (slow arcs glow,
// fast excursions ghost).
type dwellAttr struct {
	buf []float32
	gl  js.Value // the GL buffer
	u8  js.Value // the JS copy, as bytes and as floats
	f32 js.Value
}

// gpu is the one main pipeline.
var gpu = renderer{stride: 4, staticDirty: true}

// sizeCanvasToViewport sizes the canvas BACKING STORE to CSS-viewport ×
// devicePixelRatio (capped at 3) while pinning the element's CSS size to CSS
// pixels — so HiDPI displays render the scope traces at native resolution
// instead of a soft 1× upscale. Reports whether a valid size was applied.
// width/height globals are backing-store pixels (aspect and NDC math are
// ratio-based, so both stay correct).
func (r *renderer) sizeCanvasToViewport() bool {
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
	r.width = int(float64(cssW) * d)
	r.height = int(float64(cssH) * d)
	glctx.Canvas.Set("width", r.width)
	glctx.Canvas.Set("height", r.height)
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

func (r *renderer) initWebGL() {
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
	r.sizeCanvasToViewport()
	r.program = glctx.GL.Call("createProgram")
	r.vbuf = glctx.GL.Call("createBuffer")
	r.ibuf = glctx.GL.Call("createBuffer")
}
