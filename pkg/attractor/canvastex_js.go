//go:build js && wasm

package attractor

import "syscall/js"

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
		*tex = gl.Call("createTexture")
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), *tex)
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
	}
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), *tex)
	gl.Call("pixelStorei", gl.Get("UNPACK_FLIP_Y_WEBGL"), true)
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"),
		gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), src)
	gl.Call("pixelStorei", gl.Get("UNPACK_FLIP_Y_WEBGL"), false)
}
