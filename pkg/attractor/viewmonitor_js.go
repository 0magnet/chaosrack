//go:build js && wasm

package attractor

// The Monitor at the head of bay 2: the picture on a screen of its own,
// beside the knobs that turn, move and zoom it (View, Grid, Display), as the
// Record monitor opens bay 3 and the model's monitor bay 1. It is the same
// screen as Record's without what Record burns into its picture: a copy of
// the whole canvas, fitted, every tick, and nothing while it is switched off
// or out of sight (screenPower).

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// viewMonitor is the screen and the context it is drawn with.
var viewMonitor struct {
	cv, ctx js.Value
}

// wireViewMonitor finds the screen and starts it. Safe to call before the
// panel exists: it does nothing, and can be called again.
func wireViewMonitor() {
	viewMonitor.cv = dom.Doc.Call("getElementById", "view-preview")
	if !viewMonitor.cv.Truthy() {
		return
	}
	viewMonitor.ctx = viewMonitor.cv.Call("getContext", "2d")
	tick := dom.FuncOf(func(js.Value, []js.Value) any {
		drawViewMonitor()
		return nil
	})
	js.Global().Call("setInterval", tick, 1000/recPreviewFPS)
}

// drawViewMonitor copies the picture onto the screen, fitted and centered.
func drawViewMonitor() {
	cv, ctx := viewMonitor.cv, viewMonitor.ctx
	if !ctx.Truthy() {
		return
	}
	pw, ph := cv.Get("width").Float(), cv.Get("height").Float()
	if !viewScreenPower.on(cv) {
		if viewScreenPower.needsBlank() {
			ctx.Set("fillStyle", "#05070a")
			ctx.Call("fillRect", 0, 0, pw, ph)
			viewScreenPower.markBlanked()
		}
		return
	}
	ctx.Set("fillStyle", "#05070a")
	ctx.Call("fillRect", 0, 0, pw, ph)
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	// Record's region: the part of the picture that is seen, not the canvas
	// behind a docked panel, so the two monitors frame it alike.
	sx, sy, sw, sh := recRegionRect(canvas)
	if sw < 1 || sh < 1 {
		return
	}
	k := min(pw/sw, ph/sh)
	dw, dh := sw*k, sh*k
	ctx.Call("drawImage", near.captureCanvas(canvas), sx, sy, sw, sh, (pw-dw)/2, (ph-dh)/2, dw, dh)
}
