//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"syscall/js"

	"github.com/0magnet/desk/panes/hostterm"
)

// hostTerminal is the machine's own shell shown as a model.
type hostTerminal struct {
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
	pane   *hostterm.Pane
	host   js.Value // the offscreen div the pane is mounted in
	tex    js.Value
	tried  bool
	failed string
}

var hostTerm hostTerminal

// ensureHostTerm mounts the pane once, offscreen, for the same reason the websh
// terminal is mounted rather than built detached: the renderer sizes itself
// from the element's box, and a detached element has no box.
func (h *hostTerminal) ensureHostTerm() bool {
	if h.tried {
		return h.failed == "" && h.pane != nil
	}
	h.tried = true

	h.host = dom.Doc.Call("createElement", "div")
	style := h.host.Get("style")
	style.Set("position", "fixed")
	style.Set("left", "-10000px") // offscreen, not display:none — it needs a box
	style.Set("top", "0")
	style.Set("width", "900px")
	style.Set("height", "560px")
	dom.Doc.Get("body").Call("appendChild", h.host)

	p := hostterm.New()
	if err := p.Mount(h.host); err != nil {
		h.failed = "the host terminal would not start: " + err.Error()
		return false
	}
	h.pane = p
	termPane.wireTerminalFocus() // the same double-click / Escape pair the websh one uses
	return true
}

// canvas is the pane's own canvas, or nothing.
//
// Canvas() returning nothing is how the pane reports that it is drawing through
// the DOM instead — which a texture cannot sample — so this is asked EVERY
// frame rather than cached: the renderer can come up after the first frame.
func (h *hostTerminal) canvas() js.Value {
	if h.pane == nil {
		return js.Value{}
	}
	return h.pane.Canvas()
}

func hostTermOnScreen() bool {
	return run.selectedMode == "hostterm" || bgVisual == "hostterm"
}

func (h *hostTerminal) textarea() js.Value {
	if !h.host.Truthy() {
		return js.Value{}
	}
	return h.host.Call("querySelector", "textarea.xterm-helper-textarea")
}

func (h *hostTerminal) generateHostTerm() {
	if !h.ensureHostTerm() {
		if h.failed == "" {
			h.failed = "the host terminal could not be started"
		}
		aud.showAudioStatus(h.failed)
		return
	}
	cv := h.canvas()
	if !cv.Truthy() {
		aud.showAudioStatus("no WebGL renderer — a DOM-rendered terminal cannot be a texture")
		return
	}
	uploadCanvasTexture(&h.tex, cv)
	texp.drawTexturedAspect(h.tex, canvasAspect(cv))
}

func init() {
	registerGenerate("hostterm", hostTerm.generateHostTerm)
}
