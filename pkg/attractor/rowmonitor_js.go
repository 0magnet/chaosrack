//go:build js && wasm

package attractor

// The model row's own monitor.
//
// Each model category has a row, and the row whose category the running model
// is in holds that model's Parameters and its own panels (see
// rackcategory.go). What it was missing is the thing those knobs are turning:
// a picture. Turning a constant while watching a number is not operating an
// instrument.
//
// So the row carries a monitor, which is what a rack of instruments has: the
// one that is patched to the output shows a picture, and it shows it beside
// its own controls rather than across the room. The main canvas is behind the
// panel and is frequently covered by it, scrolled away from, or letterboxed
// into a corner by the drawer — the point of a panel-mounted screen is that
// it is where your hands are.
//
// It is a MONITOR and not a second renderer. The picture is a copy out of the
// model's own drawing buffer at ten frames a second, the same rate and the
// same mechanism the Record monitor uses, for the same reason given there: a
// confidence display must not cost the model frames to look at. It is also
// behind a power switch and a viewport check, because a screen nobody is
// looking at should cost nothing at all — see screenpower_js.go.

import "syscall/js"

// rowMonFPS is how often the monitor redraws. Ten, as the Record monitor's
// does, and for the reason recPreviewFPS gives.
const rowMonFPS = 10

var (
	rowMon    js.Value
	rowMonCtx js.Value
)

// wireRowMonitor starts the row monitor. Safe to call before the panel
// exists, and safe to call again.
func wireRowMonitor() {
	rowMon = doc.Call("getElementById", "rowmon")
	if !rowMon.Truthy() {
		return
	}
	rowMonCtx = rowMon.Call("getContext", "2d")
	tick := trackedFuncOf(func(js.Value, []js.Value) interface{} {
		drawRowMonitor()
		return nil
	})
	js.Global().Call("setInterval", tick, 1000/rowMonFPS)
	drawRowMonitor()
}

// drawRowMonitor paints one frame of the model onto the row's screen.
func drawRowMonitor() {
	if !rowMonCtx.Truthy() {
		return
	}
	if !rowMonScreenPower.on(rowMon) {
		if rowMonScreenPower.needsBlank() {
			rowMonBlank()
			rowMonScreenPower.markBlanked()
		}
		return
	}
	pw := rowMon.Get("width").Float()
	ph := rowMon.Get("height").Float()
	rowMonCtx.Set("fillStyle", "#05070a")
	rowMonCtx.Call("fillRect", 0, 0, pw, ph)

	canvas := modelCanvas()
	if !canvas.Truthy() {
		rowMonID(ph)
		return
	}
	sw := canvas.Get("width").Float()
	sh := canvas.Get("height").Float()
	if sw < 1 || sh < 1 {
		rowMonID(ph)
		return
	}
	// Fit, not fill. A monitor that stretched its picture would make every
	// model the shape of the monitor, which is the one thing a picture of a
	// three-dimensional figure must not do — the Record monitor letterboxes
	// for the same reason and says so there.
	k := pw / sw
	if ky := ph / sh; ky < k {
		k = ky
	}
	dw, dh := sw*k, sh*k
	rowMonCtx.Call("drawImage", captureCanvas(canvas),
		0, 0, sw, sh, (pw-dw)/2, (ph-dh)/2, dw, dh)
	rowMonID(ph)
}

// rowMonBlank paints the dark face of a screen that is switched off. Once,
// not every frame — repainting it is the cost the switch exists to remove.
func rowMonBlank() {
	pw := rowMon.Get("width").Float()
	ph := rowMon.Get("height").Float()
	rowMonCtx.Set("fillStyle", "#05070a")
	rowMonCtx.Call("fillRect", 0, 0, pw, ph)
}

// rowMonID burns the running model's name into the bottom of the picture.
//
// A studio monitor carries a source ID for the reason this one does: a wall
// of them all showing pictures is unreadable without one, and this monitor
// sits in a rack of eleven rows that each name a category and not a model.
// Burned into the picture rather than printed beside it, the way an
// under-monitor display is, because it belongs to the signal and not to the
// furniture — drawRecOSD's own argument.
func rowMonID(ph float64) {
	name := selectedMode
	if info, ok := modeInfo[selectedMode]; ok && info.Label != "" {
		name = info.Label
	}
	if name == "" {
		return
	}
	ctx := rowMonCtx
	ctx.Set("font", "9px ui-monospace, monospace")
	ctx.Set("textBaseline", "alphabetic")
	w := ctx.Call("measureText", name).Get("width").Float()
	ctx.Set("fillStyle", "rgba(0,0,0,0.55)")
	ctx.Call("fillRect", 0, ph-13, w+10, 13)
	ctx.Set("fillStyle", "#8fe3b0")
	ctx.Call("fillText", name, 5, ph-4)
}
