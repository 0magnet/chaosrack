//go:build js && wasm

package attractor

// The model rows' monitors.
//
// Every category row has a screen on its left (rackcategory_js.go). The rack
// draws ONE model, so at most one of those screens can have a picture: the
// row that is driving shows the model, and the rest stand by. That is not a
// compromise, it is what a rack of instruments looks like — the one patched
// to the output has a picture and the others are powered and idle.
//
// The picture is beside the knobs it answers to, which is the point. The main
// canvas is behind the panel and is usually covered by it, scrolled away
// from, or letterboxed into a corner by the drawer; a panel-mounted screen is
// where your hands are.
//
// It is a MONITOR and not a second renderer: a copy out of the model's own
// drawing buffer ten times a second, the rate and the mechanism the Record
// monitor uses and for the reason given there. Every screen is also behind a
// power switch and a viewport check, so one nobody is looking at costs
// nothing — see screenpower_js.go.

import "syscall/js"

// rowMonFPS is how often the live monitor redraws. Ten, as the Record
// monitor's does, and for the reason recPreviewFPS gives.
const rowMonFPS = 10

// rowMonPower is a screen's power state, one per category, keyed by label.
// Built lazily because the rows are built after the boot wiring runs.
var rowMonPower = map[string]*screenPower{}

// rowMonBlanked remembers which standby screens have had their one dark
// frame, so a row that is not driving costs one paint and then nothing.
var rowMonBlanked = map[string]bool{}

// wireRowMonitors starts the monitors. Safe to call before the rows exist,
// and safe to call again.
func wireRowMonitors() {
	if !doc.Truthy() {
		return
	}
	for _, label := range modelCategories() {
		id := categoryMonSwitchID(label)
		p := &screenPower{switchID: id}
		rowMonPower[label] = p
		if sw := doc.Call("getElementById", id); sw.Truthy() {
			sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
				p.invalidate()
				rowMonBlanked[label] = false
				return nil
			}))
		}
	}
	tick := trackedFuncOf(func(js.Value, []js.Value) interface{} {
		drawRowMonitors()
		return nil
	})
	js.Global().Call("setInterval", tick, 1000/rowMonFPS)
	drawRowMonitors()
}

// drawRowMonitors paints one frame: the driving row's screen gets the model,
// and any screen that has just stopped driving gets its one dark frame.
func drawRowMonitors() {
	if !doc.Truthy() {
		return
	}
	active := activeCategory
	for _, label := range modelCategories() {
		cv := doc.Call("getElementById", categoryMonitorID(label))
		if !cv.Truthy() {
			continue
		}
		p := rowMonPower[label]
		if p == nil {
			continue
		}
		// A standby screen is not off — it is powered and showing nothing,
		// which is one paint and then silence until it is driving again. The
		// whole rack powered down puts every screen here, including the one
		// whose row was driving.
		if stopped || label != active || !p.on(cv) {
			if !rowMonBlanked[label] {
				rowMonStandby(cv, label)
				rowMonBlanked[label] = true
			}
			continue
		}
		rowMonBlanked[label] = false
		drawRowMonitor(cv)
	}
}

// rowMonDark is the face of a screen with no picture on it.
const rowMonDark = "#05070a"

// drawRowMonitor paints the model onto one screen.
func drawRowMonitor(cv js.Value) {
	ctx := cv.Call("getContext", "2d")
	if !ctx.Truthy() {
		return
	}
	pw := cv.Get("width").Float()
	ph := cv.Get("height").Float()
	ctx.Set("fillStyle", rowMonDark)
	ctx.Call("fillRect", 0, 0, pw, ph)

	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	sw := canvas.Get("width").Float()
	sh := canvas.Get("height").Float()
	if sw < 1 || sh < 1 {
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
	ctx.Call("drawImage", captureCanvas(canvas),
		0, 0, sw, sh, (pw-dw)/2, (ph-dh)/2, dw, dh)
	rowMonID(ctx, ph, modeLabel(selectedMode))
}

// rowMonStandby paints the dark face of a screen that is not driving, with
// the model its row would play burned in — which is the whole reason a
// standby screen is worth having rather than blank glass.
func rowMonStandby(cv js.Value, label string) {
	ctx := cv.Call("getContext", "2d")
	if !ctx.Truthy() {
		return
	}
	pw := cv.Get("width").Float()
	ph := cv.Get("height").Float()
	ctx.Set("fillStyle", rowMonDark)
	ctx.Call("fillRect", 0, 0, pw, ph)

	name := ""
	if sel := doc.Call("getElementById", categorySelectID(label)); sel.Truthy() {
		name = modeLabel(sel.Get("value").String())
	}
	ctx.Set("font", "10px ui-monospace, monospace")
	ctx.Set("textAlign", "center")
	ctx.Set("textBaseline", "middle")
	ctx.Set("fillStyle", "#2a4a3a")
	ctx.Call("fillText", "STANDBY", pw/2, ph/2-7)
	if name != "" {
		ctx.Set("fillStyle", "#3a6a52")
		ctx.Call("fillText", name, pw/2, ph/2+8)
	}
	ctx.Set("textAlign", "left")
}

// rowMonID burns the running model's name into the bottom of the picture.
//
// A studio monitor carries a source ID for the reason this one does: a wall
// of them all showing pictures is unreadable without one, and these sit in a
// rack of rows that each name a category and not a model. Burned into the
// picture rather than printed beside it, the way an under-monitor display
// is, because it belongs to the signal and not to the furniture —
// drawRecOSD's own argument.
func rowMonID(ctx js.Value, ph float64, name string) {
	if name == "" {
		return
	}
	ctx.Set("font", "9px ui-monospace, monospace")
	ctx.Set("textBaseline", "alphabetic")
	w := ctx.Call("measureText", name).Get("width").Float()
	ctx.Set("fillStyle", "rgba(0,0,0,0.55)")
	ctx.Call("fillRect", 0, ph-13, w+10, 13)
	ctx.Set("fillStyle", "#8fe3b0")
	ctx.Call("fillText", name, 5, ph-4)
}

// modeLabel is a model's display name, or its key when it has none.
func modeLabel(mode string) string {
	if mode == "" {
		return ""
	}
	if info, ok := modeInfo[mode]; ok && info.Label != "" {
		return info.Label
	}
	return mode
}
