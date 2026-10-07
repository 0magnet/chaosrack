//go:build js && wasm

package stlview

import (
	"errors"
	"syscall/js"
)

// A Globe is the viewer's default picture — the wireframe sphere turning at
// a random speed, with the same renderer, shaders and colors Run draws into
// #gocanvas — on any canvas, with nothing of the page around it: no element
// ids, no controls. It is for a caller that wants the site's globe somewhere
// else, such as over a terminal's cells.
type Globe struct {
	r       Renderer
	frame   js.Func
	raf     js.Value
	stopped bool
}

// NewGlobe starts the globe on canvas, at the canvas's own size, and keeps
// it turning until Stop.
func NewGlobe(canvas js.Value) (*Globe, error) {
	gl := canvas.Call("getContext", "webgl")
	if !gl.Truthy() {
		gl = canvas.Call("getContext", "experimental-webgl")
	}
	if !gl.Truthy() {
		return nil, errors.New("stlview: no webgl")
	}
	config := InitialConfig{
		W:      canvas.Get("width").Int(),
		H:      canvas.Get("height").Int(),
		X:      cryptoRandFloat32() / 20,
		Y:      cryptoRandFloat32() / 20,
		Z:      cryptoRandFloat32() / 20,
		Colors: colorsNative,
		FSC:    fragShaderCode1,
		VSC:    vertShaderCode1,
	}
	config.Vertices, config.Indices = generateSphereVertices(float32(1.0), 30, 30)
	r, jsErr := NewRenderer(gl, config)
	if !jsErr.IsNull() {
		return nil, errors.New("stlview: cannot load webgl")
	}
	g := &Globe{r: r}
	g.r.SetZoom(3) // Run's starting zoom
	g.frame = js.FuncOf(func(this js.Value, args []js.Value) any {
		if g.stopped {
			return nil
		}
		g.r.Render(this, args)
		g.raf = js.Global().Call("requestAnimationFrame", g.frame)
		return nil
	})
	g.raf = js.Global().Call("requestAnimationFrame", g.frame)
	return g, nil
}

// Stop ends the animation and frees the renderer.
func (g *Globe) Stop() {
	if g.stopped {
		return
	}
	g.stopped = true
	js.Global().Call("cancelAnimationFrame", g.raf)
	g.frame.Release()
	g.r.Release()
}
