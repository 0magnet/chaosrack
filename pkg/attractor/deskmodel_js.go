//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/desk"
)

// The desk, as a model.
//
// ONE OF THE TWO WAYS TO HAVE THE DESK. This one puts the whole desktop INSIDE
// the scene, on the same quad the spectrogram, the recurrence plot and the
// Terminal model use — rotate it, zoom it, put it behind an attractor. The
// other is the Desk switch, which turns the arrangement inside out: the desk
// becomes the environment this app runs in, with the rack floating on it as a
// window. Windows are opened from the desk's own Applications menu in both.
//
// WHY THIS TOOK A DETOUR THROUGH desk. The obvious attempt — point a texture at
// the desk's DOM — cannot work, and the reason is the same one that has come up
// every time: DOM cannot be sampled. A window is more than its pane, and its
// title, buttons and border are DOM even when its pane is a canvas. Texturing
// the panes alone gives a desk of frameless rectangles.
//
// So desk grew a way to draw the frames itself: with DrawChrome the compositor
// rasterizes each title bar with Canvas2D — text, buttons and all — into the
// same canvas it draws the panes into. That canvas is a complete picture of the
// desk, and a canvas is a texture source. This mode asks for it and draws it.
//
// THE MOUSE IS THE HARD PART, not the keyboard. A window under a rotation is a
// quad in a scene, so a click would have to be raycast through it to a texture
// coordinate and synthesized back into a DOM event at a place nothing is. What
// exists instead is ctrl-drag, which reaches the desk and moves a window while
// an ordinary drag still turns the model (Pass-thru swaps the two) — and aiming
// is its honest limitation, because a title bar is not where the pointer says
// it is. The KEYBOARD does reach it: double-click the canvas to type, Esc to
// give it back, the same pair the Terminal and Host Shell models use.
//
// So: the Desk switch is what you want if you mean to WORK in the windows, and
// this is what you want if you mean to look at them. Flatten to work, rotate to
// admire.

var (
	deskModelTexture js.Value
	deskModelOn      bool // the compositor is being borrowed for a texture
)

// enterDeskModel borrows the desk's compositor.
//
// The desk is switched on if it was not: a model that drew nothing until you
// found an unrelated switch would look broken. Compositing is asked for WITH
// chrome, because a desk without its frames is not the thing being asked for.
func enterDeskModel() bool {
	ensureDesk()
	if !deskEl.Truthy() {
		return false
	}
	deskEl.Get("style").Set("display", "")
	// A BACKGROUND, because without one the desk is a window floating in
	// nothing and the rotation is unreadable: the quad turns about the middle
	// of the DESK, and a window sitting off-center in the texture then appears
	// to pivot about an axis that is not through it. Filling the desktop makes
	// the surface visible and the motion obvious. It is desk's own #10131a.
	if err := desk.EnableCompositingOpts(desk.CompositingOptions{
		DrawChrome: true,
		Background: [4]float32{0.063, 0.075, 0.102, 1},
	}); err != nil {
		showAudioStatus("the desk cannot be drawn here: " + err.Error())
		return false
	}
	// The compositor's own canvas is a full-page layer above everything. It
	// has to keep RENDERING — it is the texture — but it must not also be
	// visible on the page, or the desk appears twice: once flat on top, once
	// on the quad. opacity rather than display:none, because a canvas that is
	// not laid out is a canvas the compositor may size to nothing.
	if cv := desk.Canvas(); cv.Truthy() {
		cv.Get("style").Set("opacity", "0")
	}
	// See the stylesheet in ensureDesk: this is what stops the hidden windows
	// from intercepting the drag that is supposed to turn the model.
	deskEl.Get("classList").Call("add", "as-model")
	deskModelOn = true
	return true
}

// syncDeskModel runs on every panel rebuild, beside the other mode-scoped
// syncs. Leaving the desk model gives the compositor back — without it the
// compositor's canvas stays at opacity 0 for the rest of the session, so
// switching the Desk switch on afterwards shows nothing at all and looks like
// the desk has broken.
func syncDeskModel(mode string) {
	// The skin counts too: it borrows the same compositor, so releasing it here
	// while a desk is painted on a torus would take the picture away and let the
	// next frame ask for it back, once per frame, forever.
	if mode != "desk" && bgVisual != "desk" && skinSource != "desk" {
		leaveDeskModel()
	}
}

// leaveDeskModel gives the compositor back.
func leaveDeskModel() {
	if !deskModelOn {
		return
	}
	deskModelOn = false
	deskEl.Get("classList").Call("remove", "as-model")
	if cv := desk.Canvas(); cv.Truthy() {
		cv.Get("style").Set("opacity", "")
	}
	// Compositing is left ON rather than switched off. Turning it off would
	// put every window back on the DOM path mid-gesture, and the Desk switch
	// is perfectly happy with a composited desk — it is the same desk either
	// way. What is restored is only what this mode changed.
	//nolint:errcheck // already running, so this only flips the flags
	_ = desk.EnableCompositingOpts(desk.CompositingOptions{DrawChrome: false})
	if !deskOnSwitchIsSet() {
		deskEl.Get("style").Set("display", "none")
	}
}

// deskOnSwitchIsSet reports whether the user actually asked for the desk, as
// opposed to this mode having turned it on to have something to draw.
//
// THERE ARE TWO WAYS TO HAVE THE DESK, not three. It can be the MODEL, drawn
// into the scene and turning with it, or it can be the ENVIRONMENT this app
// runs inside — its panel along the bottom, the rack floating on it as a
// window. Either way its windows are opened from the desk's own Applications
// menu.
//
// There used to be a third: a Desk switch that put the desk's windows on the
// page over the canvas, belonging to neither arrangement — floating windows
// that were not the model and were not the environment either, with the rack
// still docked underneath them. It was the odd one out and it is gone; the
// switch that summons the desk now means the environment, which is what
// setDeskContain does.
func deskOnSwitchIsSet() bool { return deskContain }

// deskOnScreen reports whether the desk is being drawn, as the model or behind
// one. Taking the keyboard when it is not visible would be a keyboard stolen by
// something nobody can see.
func deskOnScreen() bool {
	return selectedMode == "desk" || bgVisual == "desk" || skinSource == "desk"
}

// deskKeyboardTarget is what a keystroke should reach inside the composited
// desk: the focused window's terminal, falling back to the topmost window that
// has one.
//
// winbox marks the focused window with a class, which is the same thing the
// compositor reads to draw its title bar brighter — so what takes the keyboard
// is what looks focused, rather than whichever window happened to open last.
func deskKeyboardTarget() js.Value {
	if !deskEl.Truthy() {
		return js.Value{}
	}
	if ta := deskEl.Call("querySelector", ".winbox.focus textarea.xterm-helper-textarea"); ta.Truthy() {
		return ta
	}
	// No window is focused, or the focused one is not a terminal. The last in
	// DOM order is the one winbox has raised highest.
	all := deskEl.Call("querySelectorAll", ".winbox textarea.xterm-helper-textarea")
	if n := all.Get("length").Int(); n > 0 {
		return all.Index(n - 1)
	}
	return js.Value{}
}

// generateDeskModel draws the desk on the quad.
func generateDeskModel() {
	if !deskModelOn && !enterDeskModel() {
		return
	}
	cv := desk.Canvas()
	if !cv.Truthy() || cv.Get("width").Int() == 0 {
		return // compositing gave up; it reports why itself
	}
	uploadCanvasTexture(&deskModelTexture, cv)
	// On a quad shaped like the desk, not a square: a 1920×999 desktop on a
	// 1:1 quad is a desktop squashed to half its width.
	drawTexturedAspect(deskModelTexture, canvasAspect(cv))
}

// deskModelCanvas is what the backdrop asks for.
func deskModelCanvas() (js.Value, bool) {
	if !deskModelOn && !enterDeskModel() {
		return js.Undefined(), false
	}
	cv := desk.Canvas()
	if !cv.Truthy() || cv.Get("width").Int() == 0 {
		return js.Undefined(), false
	}
	uploadCanvasTexture(&deskModelTexture, cv)
	return deskModelTexture, true
}

// drawDeskBackground paints the desk behind whatever model is on screen, the
// way the spectrogram and the terminal can be.
func drawDeskBackground() {
	tex, ok := deskModelCanvas()
	if !ok {
		return
	}
	savedFill := spectFill
	spectFill = true
	gl.Call("disable", glTypes.DepthTest)
	drawTexturedPlane(tex, 0)
	spectFill = savedFill
}

func init() {
	registerGenerate("desk", generateDeskModel)
}

// syncDeskExtras shows the Desk module whenever the desk is on screen, and puts
// it away with it — the same shape as syncPongExtras and the four other
// per-mode reveals buildParamPanel already calls.
//
// The desk earns a module because it has settings and three ways of being
// present: as the MODEL, as the BACKDROP behind another model, and as the
// ENVIRONMENT this app runs inside. Its style, its pass-through rule and
// Contain used to sit in the Console's Window column, filed with Info and
// Fullscreen — generic window furniture — where they were shown to everybody
// whether or not there was a desk to apply them to, and where nothing tied
// them to the thing they configure.
//
// Desk itself stays in that column deliberately: the switch that summons the
// environment is not a setting OF the desk, it is the thing that brings it
// into being, and it has to be reachable when the module is not there.
func syncDeskExtras(mode string) {
	sect := doc.Call("getElementById", "desk-module")
	if !sect.Truthy() {
		return
	}
	on := mode == "desk" || bgVisual == "desk" || deskContain
	if on {
		sect.Get("style").Set("display", "")
	} else {
		sect.Get("style").Set("display", "none")
	}
	quantizeModuleWidths()
}
