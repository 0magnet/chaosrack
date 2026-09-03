//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/websh/web"
)

// A terminal, as a model.
//
// The spectrogram, the FVF wobbulator and the recurrence plot are all a
// texture on a quad drawn through the shared 3-D pipeline, so they rotate,
// zoom and pose like anything else. This is a fourth, and what is on the
// texture is a live terminal.
//
// WHY THIS IS POSSIBLE AT ALL, because it very nearly is not: a terminal
// rendered as DOM cannot be sampled into a WebGL texture, and there is no
// cheap way to make it one — rasterizing DOM every frame is the sort of thing
// that ends in html2canvas. xterm-go has an optional WebGL2 renderer which
// draws the grid as instanced quads into a CANVAS it owns, and WebGL's
// texImage2D accepts an HTMLCanvasElement directly as a texture source. So the
// terminal renders itself on the GPU, and this mode copies that canvas into a
// texture. Without EnableWebGL there is nothing here to draw.
//
// THE CANVAS IS FOUND BY CLASS, which is worth explaining rather than leaving
// as a mystery selector. xterm-go's renderer keeps its canvas unexported and
// offers no accessor, but it inserts it into the terminal's own screen element
// with the class name "xterm-webgl-canvas". That is a stable, load-bearing
// part of its DOM — its stylesheet targets it — so reading it back is not
// reaching into a private field, and it avoids needing a change upstream
// before this could be tried at all.
//
// THERE IS A SHELL BEHIND IT: websh, a Bash interpreter compiled to wasm over
// an in-memory filesystem. It is the same shell the desk runs, and it is what
// makes this a terminal rather than a picture of one — you can rotate a running
// `for` loop.
//
// INPUT, and why it turned out to cost nothing. This app binds the keyboard
// three ways: Scope Pong drives its paddles from the arrows, the Keys module
// plays the computer keyboard, and the arrows turn whatever knob the pointer is
// over. A terminal taking focus looked like it would fight all three. It does
// not, because all three already refuse to act when the event target is an
// input, a select or a textarea — a guard they have for the LED number boxes
// and the banner field — and xterm-go captures keys on a hidden textarea. So
// focusing the terminal silences chaosrack's bindings by a rule that was
// already there, with nothing to coordinate.
//
// Focus is taken on a DOUBLE click and released with Escape. Not a single
// click: dragging on the canvas rotates the model, and a gesture that both
// rotates and steals the keyboard would make the model impossible to turn
// without also typing into it.

var (
	termSession *web.Session
	termHost    js.Value // the offscreen div the terminal is mounted in
	termCanvas  js.Value // xterm-go's WebGL canvas, the texture source
	termTexture js.Value
	termTried   bool   // mount attempted; do not retry every frame
	termFailed  string // why there is no terminal, if there is not
	termWired   bool   // the focus listeners are attached
)

// termGreeting is deliberately EMPTY: the terminal comes up as a bare prompt.
//
// It used to open with four lines saying what this is and how to type into it.
// They are not wrong, and on a rotated plane they are the most legible thing on
// screen — which is the problem. A terminal that greets you with a paragraph is
// a demo of a terminal; one that shows a prompt is a terminal.
//
// Nothing is lost by dropping them. The Info overlay for this model already
// says double-click the canvas to type and Esc to give the keyboard back (see
// descriptions.go), which is the place a person looks for what a model does.
const termGreeting = ""

// ensureTerminal mounts the terminal once, offscreen, with the WebGL renderer
// on. It is mounted into the document rather than a detached node because the
// renderer sizes itself from the element's box, and a detached element has no
// box — the terminal would come up zero by zero and the texture would be
// empty. Offscreen and one pixel is enough to have a box without being seen.
func ensureTerminal() bool {
	if termTried {
		return termFailed == "" && termSession != nil
	}
	termTried = true

	termHost = doc.Call("createElement", "div")
	style := termHost.Get("style")
	style.Set("position", "fixed")
	style.Set("left", "-10000px") // offscreen, not display:none — see above
	style.Set("top", "0")
	style.Set("width", "900px")
	style.Set("height", "560px")
	doc.Get("body").Call("appendChild", termHost)

	// Nil FS: websh makes and seeds its own in-memory filesystem, which is the
	// right one here. This terminal is a model in a visualiser, not a file
	// manager, and it has nowhere to persist to.
	s, err := web.NewSession(termHost, web.Options{
		Host:     "chaosrack",
		Greeting: termGreeting,
	})
	if err != nil {
		termFailed = "the shell would not start: " + err.Error()
		return false
	}
	termSession = s

	termCanvas = termHost.Call("querySelector", "canvas.xterm-webgl-canvas")
	if !termCanvas.Truthy() {
		// websh falls back to the DOM renderer where WebGL2 is missing, and a
		// DOM terminal cannot be a texture — which is the only thing this mode
		// wants from it.
		termFailed = "no WebGL renderer — a DOM-rendered terminal cannot be a texture"
		return false
	}
	wireTerminalFocus()
	return true
}

// wireTerminalFocus gives the terminal the keyboard on a double click and takes
// it back on Escape.
//
// The listeners are on the document rather than the terminal because the
// terminal is parked offscreen: it has a box, so it renders, but nothing can be
// clicked on it. What is on screen is the quad, and clicking that is clicking
// the canvas.
func wireTerminalFocus() {
	if termWired {
		return
	}
	termWired = true

	canvasEl.Call("addEventListener", "dblclick", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		focusModelKeyboard()
		return nil
	}))
	doc.Call("addEventListener", "keydown", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) == 0 || a[0].Get("key").String() != "Escape" {
			return nil
		}
		blurModelKeyboard()
		return nil
	}))
}

// terminalOnScreen reports whether the terminal is actually being drawn, as the
// model or behind one. Taking focus when it is not visible would be a keyboard
// stolen by something nobody can see.
// Three ways to be on screen, and the keyboard follows all of them: as the
// MODEL, as the BACKDROP behind one, or painted ON one as the skin. The skin was
// missed when it arrived -- a terminal wrapped round a torus is as much on
// screen as one on a quad, and typing into it did nothing because this only
// asked the first two questions.
func terminalOnScreen() bool {
	return selectedMode == "terminal" || bgVisual == "terminal" || skinSource == "terminal"
}

func terminalTextarea() js.Value {
	if !termHost.Truthy() {
		return js.Value{}
	}
	return termHost.Call("querySelector", "textarea.xterm-helper-textarea")
}

// focusModelKeyboard gives the keyboard to whatever texture-plane model is on
// screen, if it is one with something to type into.
//
// THE KEYBOARD NEEDS NO RAYCASTING, which is worth saying because the mouse
// does and the two got lumped together. A key goes wherever the DOM focus is,
// and the thing being drawn is still in the DOM — hidden, but present and
// focusable. So typing into a rotated terminal is just focusing it: no
// projection, no hit test, no synthesized events. The pixels are in the scene
// and the focus is on the page, and only the mouse cares about the difference.
func focusModelKeyboard() {
	if ta := modelKeyboardTarget(); ta.Truthy() {
		ta.Call("focus")
	}
}

func blurModelKeyboard() {
	if ta := modelKeyboardTarget(); ta.Truthy() {
		ta.Call("blur")
	}
}

// modelKeyboardTarget is the element a keystroke should reach, or nothing.
func modelKeyboardTarget() js.Value {
	switch {
	case terminalOnScreen():
		return terminalTextarea()
	case hostTermOnScreen():
		return hostTermTextarea()
	case deskOnScreen():
		return deskKeyboardTarget()
	}
	return js.Value{}
}

// generateTerminal keeps the texture current and draws it.
func generateTerminal() {
	if !ensureTerminal() {
		showTerminalTrouble()
		return
	}
	uploadTerminalTexture()
	// The terminal's canvas is not square either — it was drawn on a square
	// quad until the desk made the same squashing obvious.
	drawTexturedAspect(termTexture, canvasAspect(termCanvas))
}

// uploadTerminalTexture copies xterm-go's canvas into a GL texture.
//
// Straight from the canvas every frame: texImage2D takes the element and the
// driver does the copy, which is the cheap path and the reason this works at
// all. UNPACK_FLIP_Y is set because a canvas's origin is top-left and a
// texture's is bottom-left, so without it the terminal is drawn upside down.
func uploadTerminalTexture() { uploadCanvasTexture(&termTexture, termCanvas) }

// showTerminalTrouble reports why there is no terminal, through the same
// overlay the audio modes use for the same purpose. Drawing nothing and
// saying nothing is the one outcome worth ruling out.
func showTerminalTrouble() {
	if termFailed == "" {
		termFailed = "the terminal could not be started"
	}
	showAudioStatus(termFailed)
}

// terminalTexture is what the backdrop asks for when it wants to paint the
// terminal behind another model. It returns a texture only when there is one,
// so the backdrop can fall through to drawing nothing rather than binding a
// texture that was never filled.
func terminalTexture() (js.Value, bool) {
	if !ensureTerminal() {
		return js.Undefined(), false
	}
	uploadTerminalTexture()
	return termTexture, true
}

func init() {
	registerGenerate("terminal", generateTerminal)
}

// drawTerminalBackground paints the terminal behind whatever model is on
// screen, the way the spectrogram can be. Face-on and filling the canvas, with
// no clear, so it layers under the model rather than replacing it.
func drawTerminalBackground() {
	tex, ok := terminalTexture()
	if !ok {
		return // no terminal: draw nothing rather than a black square
	}
	// The same trick the spectrogram background uses: spectFill maps the
	// plane straight to clip space, so it fills the canvas face-on regardless
	// of the pose. A background always fills; the Fill switch governs the MODE.
	savedFill := spectFill
	spectFill = true
	gl.Call("disable", glTypes.DepthTest)
	drawTexturedPlane(tex, 0)
	spectFill = savedFill
}
