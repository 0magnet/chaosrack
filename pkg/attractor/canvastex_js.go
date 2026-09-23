//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

// uploadCanvasTexture copies a canvas into a GL texture, creating the texture
// on first use.
//
// Three modes draw a canvas that some other program is rendering — the Terminal
// (xterm-go), the desk (its WebGL compositor), and the spectrogram's own
// surface — and they all need exactly this. It was written twice before it was
// written once.
//
// Straight from the canvas every frame: texImage2D takes the element and the
// driver does the copy, which is the cheap path and the reason any of this
// works. UNPACK_FLIP_Y is set because a canvas's origin is top-left and a
// texture's is bottom-left, so without it the picture is upside down.
func uploadCanvasTexture(tex *js.Value, src js.Value) {
	if !src.Truthy() {
		return
	}
	if !tex.Truthy() {
		*tex = glctx.GL.Call("createTexture")
		glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), *tex)
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MIN_FILTER"), glctx.GL.Get("LINEAR"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MAG_FILTER"), glctx.GL.Get("LINEAR"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_S"), glctx.GL.Get("CLAMP_TO_EDGE"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_T"), glctx.GL.Get("CLAMP_TO_EDGE"))
	}
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), *tex)
	glctx.GL.Call("pixelStorei", glctx.GL.Get("UNPACK_FLIP_Y_WEBGL"), true)
	glctx.GL.Call("texImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"),
		glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), src)
	glctx.GL.Call("pixelStorei", glctx.GL.Get("UNPACK_FLIP_Y_WEBGL"), false)
}
