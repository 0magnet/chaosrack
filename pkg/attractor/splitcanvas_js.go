//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"strconv"
	"syscall/js"
)

// The canvas on the near side of the panel.
//
// The model is WebGL and the rack is DOM, so "some of the model in front of the
// controls and some behind" cannot be one canvas: there is no way to put DOM
// between two halves of a single drawing. It takes a surface on each side of
// the panel in the stacking order.
//
// This one is a plain 2-D canvas rather than a second WebGL context, and that
// is deliberate. A second context would double every buffer, every program and
// every texture, and contexts are a capped resource — a browser allows a page
// somewhere near sixteen and evicts the oldest when asked for one past the cap,
// which is a bug this project has already been bitten by. So the scene is drawn
// twice into the ONE context and the near half is copied out with drawImage,
// which costs a frame copy and no context at all.

// nearCanvas is the canvas on the near side of the panel and the capture
// canvas that feeds it.
type nearCanvas struct {
	front    js.Value // 2-D canvas stacked above the panel
	frontCtx js.Value

	// captureCanvas is what anything recording the picture should read from.
	//
	// While the model is split there is no single canvas holding it: the far half
	// is on the GL canvas and the near half is on the 2-D one above the panel. A
	// capture that took either alone would take part of the model and call it the
	// picture — measured, a still taken mid-knob had 235 lit pixels where the whole
	// thing had 659, so two thirds of the model went missing from the file while
	// the screen looked right.
	//
	// Unsplit, this hands back the very canvas it was given, so the ordinary path
	// keeps capturing the canvas directly with no copy at all.
	capture    js.Value
	captureCtx js.Value
}

var near nearCanvas

// ensureFrontCanvas builds the near-side canvas on first use.
//
// pointer-events are off: it covers the whole viewport including the controls,
// and a canvas that swallowed clicks would make the rack unusable the moment
// any of the model was in front of it. Drag-to-rotate is bound to the document
// and so keeps working through it, which is the same reason the Front switch
// could raise the main canvas without breaking the panel.
func (n *nearCanvas) ensure() bool {
	if n.front.Truthy() {
		return true
	}
	if !dom.Doc.Truthy() {
		return false
	}
	n.front = dom.Doc.Call("createElement", "canvas")
	n.front.Set("id", "gocanvas-front")
	n.front.Get("style").Set("cssText",
		"position:fixed;left:0;top:0;pointer-events:none;z-index:var(--z-canvas-front);")
	dom.Body.Call("appendChild", n.front)
	n.frontCtx = n.front.Call("getContext", "2d")
	n.size()
	return n.frontCtx.Truthy()
}

// sizeFrontCanvas matches the near canvas to the main one, in both the backing
// store and the CSS box, so a copy between them is one-to-one and needs no
// scaling. Called from the same place the main canvas is sized.
func (n *nearCanvas) size() {
	if !n.front.Truthy() || !glctx.Canvas.Truthy() {
		return
	}
	n.front.Set("width", gpu.width)
	n.front.Set("height", gpu.height)
	st := glctx.Canvas.Get("style")
	fs := n.front.Get("style")
	fs.Set("width", st.Get("width"))
	fs.Set("height", st.Get("height"))
}

// showFrontCanvas hides the near canvas outright when nothing is on that side.
// An empty transparent canvas over the whole page costs a composite every frame
// for nothing, and this is the common case: the knob spends most of its life at
// one end or the other.
func (n *nearCanvas) show(on bool) {
	if !n.front.Truthy() {
		return
	}
	if on {
		n.front.Get("style").Set("display", "")
		return
	}
	// Cleared as well as hidden. A hidden canvas keeps its pixels, so showing
	// it again would flash whatever was last drawn on it -- a half of a model
	// that may not even be the current one.
	if n.frontCtx.Truthy() {
		n.frontCtx.Call("clearRect", 0, 0, gpu.width, gpu.height)
	}
	n.front.Get("style").Set("display", "none")
}

// copyNearPassToFront lifts what the GL canvas currently holds onto the near
// canvas, replacing what was there.
//
// clearRect first because the copy is drawn with source-over: without it, the
// previous frame's near half would show through wherever this one is
// transparent, which is most of the picture.
//
// The GL context is created with preserveDrawingBuffer, so reading the canvas
// after drawing is defined rather than a race with the compositor.
func (n *nearCanvas) copyNearPassToFront() {
	if !n.frontCtx.Truthy() {
		return
	}
	n.frontCtx.Call("clearRect", 0, 0, gpu.width, gpu.height)
	n.frontCtx.Call("drawImage", glctx.Canvas, 0, 0)
}

// frontCanvasPx reports the CSS size the near canvas is showing at, for tests
// that want to prove it tracks the main one.
func (n *nearCanvas) frontCanvasPx() string {
	if !n.front.Truthy() {
		return ""
	}
	return n.front.Get("style").Get("width").String() + "x" +
		n.front.Get("style").Get("height").String() +
		" @" + strconv.Itoa(gpu.width) + "x" + strconv.Itoa(gpu.height)
}

func (n *nearCanvas) captureCanvas(main js.Value) js.Value {
	if !splitDrawing() || !n.front.Truthy() || !main.Truthy() {
		return main
	}
	if !n.capture.Truthy() {
		n.capture = dom.Doc.Call("createElement", "canvas")
		n.captureCtx = n.capture.Call("getContext", "2d")
	}
	if !n.captureCtx.Truthy() {
		return main
	}
	w, h := main.Get("width").Int(), main.Get("height").Int()
	if n.capture.Get("width").Int() != w {
		n.capture.Set("width", w)
	}
	if n.capture.Get("height").Int() != h {
		n.capture.Set("height", h)
	}
	// Cleared, then far half, then near half — the same order the screen
	// composites them in, so what is recorded is what was on screen rather than
	// a picture with the halves the wrong way round.
	n.captureCtx.Call("clearRect", 0, 0, w, h)
	n.captureCtx.Call("drawImage", main, 0, 0)
	n.captureCtx.Call("drawImage", n.front, 0, 0)
	return n.capture
}

// modelCanvas is the canvas the model is drawn on.
//
// It exists because eight places used to ask for `querySelector("canvas")`,
// which is not a name but a POSITION: whichever canvas the document happens to
// list first. That was true of the model's canvas by luck rather than by
// design, and the page has been growing canvases — the near-side one this
// feature added, a preview monitor in the Record module, one per terminal. Any
// of them landing earlier in the document would have quietly redirected
// recording, region selection and stills at the wrong picture.
//
// canvasEl is the same element the renderer resolved by id at start-up, so this
// asks the question by name and gets the answer the renderer is using.
func modelCanvas() js.Value { return glctx.Canvas }
