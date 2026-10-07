//go:build js && wasm

package stlview

import (
	"errors"
	"math"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// A Globe is the viewer's default picture — the wireframe sphere turning at
// a random speed, with the same renderer, shaders and colors Run draws into
// #gocanvas — on any canvas, with nothing of the page around it: no element
// ids, no sliders (Interact gives it a hand instead). It is for a caller
// that wants the site's globe somewhere else, such as over a terminal's
// cells.
type Globe struct {
	r         Renderer
	frame     js.Func
	raf       js.Value
	stopped   bool
	zoom      float32
	listeners []listener
	// mesh is the lines the globe draws, in the space Pose describes: its
	// sphere is built with a random tilt of its own, so drawing any other
	// sphere at Pose comes out turned by that tilt.
	meshV []float32
	meshI []uint16
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
	g := &Globe{r: r, zoom: 3} // Run's starting zoom
	g.meshV, g.meshI = lineMesh(config.Vertices, config.Indices)
	g.r.SetZoom(g.zoom)
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
	for _, l := range g.listeners {
		l.el.Call("removeEventListener", l.name, l.fn)
		l.fn.Release()
	}
	js.Global().Call("cancelAnimationFrame", g.raf)
	g.frame.Release()
	g.r.Release()
}

// Turn rotates the globe as a hand on it would: dx about the screen's
// vertical, dy about its horizontal, in radians, ahead of its own spin.
func (g *Globe) Turn(dx, dy float32) {
	v := g.view()
	vt := v.Transpose()
	t := vt.Mul3(mgl32.Rotate3DY(dx)).Mul3(mgl32.Rotate3DX(dy)).Mul3(v).Mat4()
	if g.r.turn != nil {
		t = t.Mul4(*g.r.turn)
	}
	g.r.turn = &t
}

// Zoom moves the camera by f: under 1 nearer, over 1 farther.
func (g *Globe) Zoom(f float32) {
	g.zoom = min(max(g.zoom*f, minZoom), maxZoom)
	g.r.SetZoom(g.zoom)
}

const (
	minZoom = 1.2 // the sphere just inside the near plane
	maxZoom = 30
)

// view is the camera's rotation: Run's camera looks at the globe from the
// diagonal, however far.
func (g *Globe) view() mgl32.Mat3 {
	return mgl32.LookAtV(mgl32.Vec3{1, 1, 1}, mgl32.Vec3{}, mgl32.Vec3{0, 1, 0}).Mat3()
}

// Pose is how the camera sees the globe now — its spin, the person's turning
// and the camera's angle — as the angles of Rx·Ry·Rz, x right, y up and z
// toward the viewer: rasterview.View's pose. It is for drawing something in
// step with the globe, such as its shadow.
func (g *Globe) Pose() (ax, ay, az float64) {
	m := g.view().Mul3(g.r.movMatrix.Mat3())
	ay = math.Asin(float64(min(max(m.At(0, 2), -1), 1)))
	ax = math.Atan2(float64(-m.At(1, 2)), float64(m.At(2, 2)))
	az = math.Atan2(float64(-m.At(0, 1)), float64(m.At(0, 0)))
	return ax, ay, az
}

// Mesh is the globe as it is drawn, for drawing it again in step with it:
// vertices (xyz) and line endpoint pairs in the globe's own model space,
// which Pose turns and Distance views. The page colors each point by its y,
// red at -1 to blue at +1.
func (g *Globe) Mesh() (vertices []float32, indices []uint16) { return g.meshV, g.meshI }

// Distance is how far the camera is from the globe's center, in its radii.
func (g *Globe) Distance() float64 { return float64(g.zoom) * math.Sqrt(3) }

// Radius is the globe's apparent radius as a share of half the canvas's
// height, at the camera's distance now.
func (g *Globe) Radius() float64 {
	// A unit sphere seen from zoom·√3 away under a 45° vertical field.
	return 1 / (float64(g.zoom) * math.Sqrt(3) * math.Tan(math.Pi/8))
}

// Interact lets the person turn the globe by dragging on el and zoom it
// with the wheel.
func (g *Globe) Interact(el js.Value) {
	var lastX, lastY float64
	down := false
	on := func(name string, f func(e js.Value)) {
		fn := js.FuncOf(func(_ js.Value, args []js.Value) any {
			f(args[0])
			return nil
		})
		el.Call("addEventListener", name, fn, map[string]any{"passive": false})
		g.listeners = append(g.listeners, listener{el, name, fn})
	}
	el.Get("style").Set("touchAction", "none")
	el.Get("style").Set("cursor", "grab")
	on("pointerdown", func(e js.Value) {
		down = true
		lastX, lastY = e.Get("clientX").Float(), e.Get("clientY").Float()
		el.Call("setPointerCapture", e.Get("pointerId"))
		el.Get("style").Set("cursor", "grabbing")
	})
	on("pointermove", func(e js.Value) {
		if !down {
			return
		}
		x, y := e.Get("clientX").Float(), e.Get("clientY").Float()
		// Half a turn across the element's height.
		k := math.Pi / max(el.Get("clientHeight").Float(), 1)
		g.Turn(float32((x-lastX)*k), float32((y-lastY)*k))
		lastX, lastY = x, y
	})
	up := func(js.Value) {
		down = false
		el.Get("style").Set("cursor", "grab")
	}
	on("pointerup", up)
	on("pointercancel", up)
	on("wheel", func(e js.Value) {
		e.Call("preventDefault")
		g.Zoom(float32(math.Pow(1.0015, e.Get("deltaY").Float())))
	})
}

type listener struct {
	el   js.Value
	name string
	fn   js.Func
}

// lineMesh is what Render draws for a sphere: a line loop through every
// vertex in order (drawArrays LINE_LOOP), then the index pairs (LINES).
func lineMesh(v []float32, idx []uint32) ([]float32, []uint16) {
	n := len(v) / 3
	if n == 0 || n > 1<<16 {
		return nil, nil
	}
	ind := make([]uint16, 0, 2*n+len(idx))
	for k := range n {
		ind = append(ind, uint16(k), uint16((k+1)%n)) //nolint:gosec // n is under 1<<16, checked above
	}
	for _, i := range idx {
		ind = append(ind, uint16(i)) //nolint:gosec // an index into the same n vertices
	}
	return append([]float32(nil), v...), ind
}
