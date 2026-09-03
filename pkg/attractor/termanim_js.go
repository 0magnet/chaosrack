//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/tuiwasm/demos"
	"github.com/0magnet/tuiwasm/play"
)

// A terminal animation, as a model.
//
// The Terminal model puts a SHELL on a quad. This puts one of tuiwasm's demos
// there instead — twenty-one of them animations, drawn as half-block pixels or
// as falling glyphs, and every one of them a program that draws into a terminal
// and nothing else.
//
// It works for the same reason the Terminal model does, and the reason is worth
// keeping in one place: a terminal rendered as DOM cannot be sampled into a
// WebGL texture, but xterm-go's WebGL renderer draws the grid as instanced
// quads into a canvas it owns, and texImage2D takes an HTMLCanvasElement as a
// texture source directly. So the animation renders itself on the GPU and this
// copies that canvas into a texture. See terminal_js.go, which explains the
// canvas-by-class lookup and why the host is parked offscreen rather than
// hidden.
//
// WHAT IS DIFFERENT from that model is only what is running: play.Mount builds
// the terminal and starts the demo in it. tuiwasm's play package exists for
// exactly this — its own doc names "a texture on a quad in a 3D scene" as one
// of the places a demo has to be embeddable — so there is no adapter here, just
// a call.
//
// NO KEYBOARD is wired to it, unlike the Terminal model. These draw; they do
// not read. The animations quit on q or Escape, and a model that could be
// dismissed by a stray keystroke and not restarted would be a worse model than
// one that simply runs.

var (
	animHost    js.Value // the offscreen div the terminal is mounted in
	animCanvas  js.Value // xterm-go's WebGL canvas, the texture source
	animTexture js.Value
	animSession *play.Session
	animRunning string // the demo currently mounted, if any
	animFailed  string // why there is no animation, if there is not
)

// animDefault is what the model shows before anything is picked.
//
// matrix rather than one of the pixel effects: it is made of glyphs, so it
// reads as a terminal at a glance and on a rotated quad — which is the thing
// this model is for. A field of half-block pixels turning in space is a pretty
// texture that says nothing about where it came from.
const animDefault = "matrix"

// animPick is the demo the panel's selector last asked for.
var animPick = animDefault

// ensureTermAnim mounts the demo named by animPick, replacing whatever was
// running. It returns false when there is nothing to draw.
func ensureTermAnim() bool {
	if animSession != nil && animRunning == animPick {
		return animFailed == ""
	}

	// Switching demos: the old one goes first. Closing the session finalizes
	// its screen, which is what ends the animation's own goroutine — see
	// canvas.Run in termanim, which returns when the event queue closes.
	if animSession != nil {
		animSession.Close()
		animSession = nil
	}
	if animHost.Truthy() {
		animHost.Call("remove")
		animHost = js.Undefined()
	}
	animFailed = ""

	d, ok := demos.Lookup(animPick)
	if !ok {
		animFailed = "no demo called " + animPick
		return false
	}

	// Offscreen, but with a box: the renderer sizes itself from the element,
	// and a detached node has no box, so the terminal would come up zero by
	// zero and the texture would be empty.
	animHost = doc.Call("createElement", "div")
	style := animHost.Get("style")
	style.Set("position", "fixed")
	style.Set("left", "-10000px")
	style.Set("top", "0")
	style.Set("width", "900px")
	style.Set("height", "560px")
	doc.Get("body").Call("appendChild", animHost)

	s, err := play.Mount(d, animHost)
	if err != nil {
		animFailed = "the demo would not start: " + err.Error()
		return false
	}
	animSession = s
	animRunning = animPick

	animCanvas = animHost.Call("querySelector", "canvas.xterm-webgl-canvas")
	if !animCanvas.Truthy() {
		// Without the WebGL renderer there is no canvas to sample, and a
		// DOM-rendered terminal cannot be a texture — which is the only thing
		// this mode wants from it.
		animFailed = "no WebGL renderer — a DOM-rendered terminal cannot be a texture"
		return false
	}
	return true
}

// setTermAnim asks for a different demo. The switch happens on the next frame,
// in ensureTermAnim, so that tearing one down and building another never
// happens in the middle of a draw.
func setTermAnim(name string) {
	if name == "" {
		name = animDefault
	}
	animPick = name
}

func generateTermAnim() {
	if !ensureTermAnim() {
		showTermAnimTrouble()
		return
	}
	uploadCanvasTexture(&animTexture, animCanvas)
	// Its canvas is not square, and drawing it on a square quad squashes the
	// glyphs — which on a terminal is the whole of what there is to look at.
	drawTexturedAspect(animTexture, canvasAspect(animCanvas))
}

// showTermAnimTrouble reports why there is nothing to see, through the same
// overlay the audio modes use for the same purpose. Drawing nothing and saying
// nothing is the one outcome worth ruling out.
func showTermAnimTrouble() {
	if animFailed == "" {
		animFailed = "the animation could not be started"
	}
	showAudioStatus(animFailed)
}

// termAnimTexture is what the backdrop asks for to paint an animation behind
// another model. It returns a texture only when there is one, so the backdrop
// falls through to drawing nothing rather than binding a texture never filled.
func termAnimTexture() (js.Value, bool) {
	if !ensureTermAnim() {
		return js.Undefined(), false
	}
	uploadCanvasTexture(&animTexture, animCanvas)
	return animTexture, true
}

// syncTermAnimExtras shows this model's panel section when it is the model, or
// is what the backdrop is drawing, and fills the selector the first time.
//
// The list is built from tuiwasm's registry rather than written into the panel
// HTML, so it cannot drift from what is actually registered — the same reason
// the STL module builds its catalog at runtime.
func syncTermAnimExtras(mode string) {
	sect := doc.Call("getElementById", "termanim-module")
	if !sect.Truthy() {
		return
	}
	if mode == "termanim" || bgVisual == "termanim" {
		sect.Get("style").Set("display", "")
	} else {
		sect.Get("style").Set("display", "none")
	}
	fillTermAnimPicker()
	quantizeModuleWidths()
}

var animPickerFilled bool

func fillTermAnimPicker() {
	if animPickerFilled {
		return
	}
	sel := doc.Call("getElementById", "termanim-pick")
	if !sel.Truthy() {
		return
	}
	for _, d := range demos.All() {
		opt := doc.Call("createElement", "option")
		opt.Set("value", d.Name)
		opt.Set("textContent", d.Name)
		opt.Set("title", d.Desc)
		if d.Name == animPick {
			opt.Set("selected", true)
		}
		sel.Call("appendChild", opt)
	}
	sel.Call("addEventListener", "change", js.FuncOf(func(this js.Value, _ []js.Value) any {
		setTermAnim(this.Get("value").String())
		return nil
	}))
	animPickerFilled = true
}

// drawTermAnimBackground paints the animation behind whatever model is on
// screen, the way the terminal and the spectrogram can be. Face-on and filling
// the canvas, with no clear, so it layers under the model rather than replacing
// it — an animation is a better backdrop than a shell, since it is moving and
// has nothing to read.
func drawTermAnimBackground() {
	tex, ok := termAnimTexture()
	if !ok {
		return // nothing running: draw nothing rather than a black square
	}
	savedFill := spectFill
	spectFill = true
	gl.Call("disable", glTypes.DepthTest)
	drawTexturedPlane(tex, 0)
	spectFill = savedFill
}

func init() {
	registerGenerate("termanim", generateTermAnim)
}
