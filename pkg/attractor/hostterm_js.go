//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/desk/panes/hostterm"
)

// The machine's own shell, as a model.
//
// The Terminal model beside this one is websh: a shell compiled into the wasm,
// over a filesystem that exists only in the page. This is the other kind — a
// real shell on the machine serving the page, through the same agent the desk's
// host pane uses.
//
// IT EXISTED ALREADY, one level in. hostterm is a desk PANE, so the only way to
// reach a host shell was to turn on the desk and open it there, which also
// brings a window manager, a panel and a taskbar to look at a terminal through.
// The pane was always compositable — it enables the WebGL renderer on mount and
// offers Canvas() for exactly this — so the model is the pane mounted offscreen
// and its canvas used as a texture, which is what the Terminal model does with
// websh's.
//
// WITHOUT --shell there is still something to draw. hostterm mounts its
// terminal whether or not an agent answers and writes into it the reason and
// the flag that fixes it, which is a better answer than an empty rectangle and
// is why nothing here checks for an agent first.
var (
	hostTermPane   *hostterm.Pane
	hostTermHost   js.Value // the offscreen div the pane is mounted in
	hostTermTex    js.Value
	hostTermTried  bool
	hostTermFailed string
)

// ensureHostTerm mounts the pane once, offscreen, for the same reason the websh
// terminal is mounted rather than built detached: the renderer sizes itself
// from the element's box, and a detached element has no box.
func ensureHostTerm() bool {
	if hostTermTried {
		return hostTermFailed == "" && hostTermPane != nil
	}
	hostTermTried = true

	hostTermHost = doc.Call("createElement", "div")
	style := hostTermHost.Get("style")
	style.Set("position", "fixed")
	style.Set("left", "-10000px") // offscreen, not display:none — it needs a box
	style.Set("top", "0")
	style.Set("width", "900px")
	style.Set("height", "560px")
	doc.Get("body").Call("appendChild", hostTermHost)

	p := hostterm.New()
	if err := p.Mount(hostTermHost); err != nil {
		hostTermFailed = "the host terminal would not start: " + err.Error()
		return false
	}
	hostTermPane = p
	wireTerminalFocus() // the same double-click / Escape pair the websh one uses
	return true
}

// hostTermCanvas is the pane's own canvas, or nothing.
//
// Canvas() returning nothing is how the pane reports that it is drawing through
// the DOM instead — which a texture cannot sample — so this is asked EVERY
// frame rather than cached: the renderer can come up after the first frame.
func hostTermCanvas() js.Value {
	if hostTermPane == nil {
		return js.Value{}
	}
	return hostTermPane.Canvas()
}

func hostTermOnScreen() bool {
	return selectedMode == "hostterm" || bgVisual == "hostterm"
}

func hostTermTextarea() js.Value {
	if !hostTermHost.Truthy() {
		return js.Value{}
	}
	return hostTermHost.Call("querySelector", "textarea.xterm-helper-textarea")
}

func generateHostTerm() {
	if !ensureHostTerm() {
		if hostTermFailed == "" {
			hostTermFailed = "the host terminal could not be started"
		}
		showAudioStatus(hostTermFailed)
		return
	}
	cv := hostTermCanvas()
	if !cv.Truthy() {
		showAudioStatus("no WebGL renderer — a DOM-rendered terminal cannot be a texture")
		return
	}
	uploadCanvasTexture(&hostTermTex, cv)
	drawTexturedAspect(hostTermTex, canvasAspect(cv))
}

func init() {
	registerGenerate("hostterm", generateHostTerm)
}
