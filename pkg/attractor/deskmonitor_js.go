//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/desk"
)

// The desk, face on, whatever the model is doing.
//
// Drawing the desk as a model is the point of the desk model — it turns, it
// leans, it can be seen from behind — and it is also the thing that makes a
// window hard to work in: the one you are typing into may be edge-on, mirrored,
// or facing away. This is the other view of the same desk, flat and the right
// way round, in the module that owns the desk's settings.
//
// It is the Record module's monitor with a different source, deliberately: same
// low frame rate, same "do not draw into a hidden element" guard, same reason.
// A monitor is a glance, not a second display, and a glance does not need sixty
// frames a second.
//
// THE SOURCE IS THE COMPOSITOR'S OWN CANVAS, which is already a face-on picture
// of the desk — that is what it draws before anything maps it onto a quad. So
// this needs no second rendering path and cannot disagree with what the model
// shows: it is the same pixels, before the rotation.
const deskMonitorFPS = 10

var (
	deskMonitor    js.Value
	deskMonitorCtx js.Value
)

func initDeskMonitor() {
	deskMonitor = doc.Call("getElementById", "desk-monitor")
	if !deskMonitor.Truthy() {
		return
	}
	deskMonitorCtx = deskMonitor.Call("getContext", "2d")
	if !deskMonitorCtx.Truthy() {
		return
	}
	js.Global().Call("setInterval", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		drawDeskMonitor()
		return nil
	}), 1000/deskMonitorFPS)
}

// drawDeskMonitor copies the compositor's canvas into the monitor, letterboxed.
func drawDeskMonitor() {
	if !deskMonitorCtx.Truthy() {
		return
	}
	// Nothing to do if nobody can see it: offsetParent is null while the Desk
	// module is hidden, which is most of the time. Skipped during a panel
	// resize for the reason the Record monitor is — the rack re-measures every
	// module on each pointer move, and a canvas copy in the middle of that is
	// how a drag comes to cost the model a frame.
	if !deskMonitor.Get("offsetParent").Truthy() || resizing {
		return
	}
	w := deskMonitor.Get("width").Float()
	h := deskMonitor.Get("height").Float()
	deskMonitorCtx.Call("clearRect", 0, 0, w, h)

	src := desk.Canvas()
	if !src.Truthy() {
		return // no compositor yet: an empty monitor rather than a stale one
	}
	sw := src.Get("width").Float()
	sh := src.Get("height").Float()
	if sw <= 0 || sh <= 0 {
		return
	}
	// Letterboxed, because a desk squashed to the monitor's aspect would make
	// square windows oblong and the monitor would be lying about the thing it
	// is monitoring.
	scale := w / sw
	if s := h / sh; s < scale {
		scale = s
	}
	dw, dh := sw*scale, sh*scale
	deskMonitorCtx.Call("drawImage", src, (w-dw)/2, (h-dh)/2, dw, dh)
}
