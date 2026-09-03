//go:build js && wasm

package play

import (
	"syscall/js"

	xterm "github.com/0magnet/xterm-go"
)

// FastRenderer moves a terminal off the DOM renderer.
//
// The DOM renderer rebuilds a span per cell, which is fine for a shell and far
// too slow for anything that animates: a demo redrawing a full window every
// frame pegged the renderer and stopped the page answering at all. WebGL draws
// the same cells from a glyph atlas instead.
//
// NewTerminal calls this already. It is exported for callers that build their
// own terminal — an animation in a terminal still on the DOM renderer is the
// one way to make a page stop responding, so it is worth being able to ask for
// this explicitly.
//
// Failure is not fatal. Software WebGL, or none, still leaves a working
// terminal — slower, and only the animating demos will notice.
func FastRenderer(t *xterm.Terminal) {
	if err := t.EnableWebGL(); err != nil {
		js.Global().Get("console").Call("log", "tuiwasm: webgl unavailable, using the DOM renderer: "+err.Error())
	}
}
