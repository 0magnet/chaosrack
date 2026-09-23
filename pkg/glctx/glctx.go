//go:build js && wasm

// Package glctx owns the WebGL context and the canvas it draws on.
//
// The same kind of thing as pkg/dom, and extracted for the same reason: GL is
// touched by 35 of the app's browser files — the camera, the palette, the
// spectrogram, the terminal, the desk, the stereo scope — because in a program
// whose whole job is drawing, "the thing that draws" is not a subsystem with a
// boundary. It is ambient, like the document.
//
// That is worth stating because the obvious reading is the other one: that gl,
// the shader program and the vertex buffers are a Renderer, and a Renderer
// should be a type with methods. The file list says otherwise. A type would
// have to be handed to two thirds of the package, which is a global with extra
// steps and more typing. What these actually are is the platform: acquired
// once when the page is ready, used from anywhere, never replaced.
package glctx

import "syscall/js"

// The context and the surface it draws on.
//
// Assigned by Init rather than at package-var time, for the reason the
// original comment gave: when this is imported as a library the host's DOM may
// not exist yet, and calling getContext on a null canvas panics rather than
// returning nothing.
var (
	GL     js.Value
	Canvas js.Value
	Types  GLTypes
)

// Init takes the context from a canvas element and reports whether there was
// one. A page with no canvas, or a browser with no WebGL, is a thing the
// caller has to cope with rather than an error to panic on.
//
// preserveDrawingBuffer because the frame has to survive being drawn: the PNG
// and GIF capture read the canvas back, and without it the buffer's contents
// are undefined by the time anything looks. experimental-webgl is the old
// spelling, still the only one some drivers answer to.
//
// Types is filled here rather than by the caller because the constants come
// off the context — a GLTypes built against a context that has since been
// replaced holds handles into the wrong one.
func Init(canvas js.Value) bool {
	if !canvas.Truthy() {
		return false
	}
	opts := js.Global().Get("Object").New()
	opts.Set("preserveDrawingBuffer", true)
	ctx := canvas.Call("getContext", "webgl", opts)
	if ctx.IsUndefined() {
		ctx = canvas.Call("getContext", "experimental-webgl", opts)
	}
	if ctx.IsUndefined() || !ctx.Truthy() {
		Canvas = canvas // the surface exists even where the context does not
		return false
	}
	Canvas, GL = canvas, ctx
	Types.New(ctx)
	return true
}

// Ready reports whether there is a context to draw with.
func Ready() bool { return GL.Truthy() }
