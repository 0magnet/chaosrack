//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// The density view (Display, POINTS' D button on a map): instead of a
// frame's iterates as points, how often the orbit has landed in each cell of
// the figure's plane, counted since the view was switched on and shown on a
// log scale in the trace's colors. The counts are kept in Go
// (dynamics.Density) and drawn as one texture on a quad over the figure's
// box, through the textured program, so the view still turns and zooms with
// the camera.
var density struct {
	on    bool
	d     dynamics.Density
	key   string // the map the counts are for; "" counts afresh
	frame int

	tex    js.Value
	pix    []byte
	levels []byte
	// refit asks for the camera to be fitted again on the next frame drawn,
	// to the box (density on) or to the points (density off).
	refit bool
	pixJS js.Value
	quad  js.Value
	quadV [20]float32
	// corners are the box's corners, uploaded as the frame's vertices so the
	// camera fits and centers on the box. Drawn as none of them.
	corners [16]float32
	lut     [256][3]float32
}

// densityItersPerFrame is how many iterates a frame counts per TRAIL point:
// a point plot shows TRAIL's count, and counting is cheap enough that the
// figure can fill in several times faster than it draws.
const densityItersPerFrame = 10

// densityToneEvery is how often, in frames, the counts are turned into the
// image and uploaded: the image changes slowly once it has filled in, and a
// 768-cell-square upload every frame would cost more than the counting.
const densityToneEvery = 4

// densityInvalidate starts the counts over, on a parameter change: a count is
// only meaningful for the parameters that produced it.
func densityInvalidate() { density.key = "" }

// generateDensity counts this frame's iterates of map m and draws the
// density image.
func generateDensity(mode string, m dynamics.MapSys) {
	orbits := m.Count()
	mapOrbits.Ensure(mode, m)
	if density.key != mode {
		lo, hi := dynamics.DensityBox(m, 100000)
		density.d.Reset(lo, hi)
		density.key, density.frame = mode, 0
	}
	n := max(sim.steps, 2) * densityItersPerFrame
	per := max(n/orbits, 1)
	for o := range orbits {
		p := mapOrbits.At(o)
		for range per {
			x, y, z := m.Step(p[0], p[1], p[2])
			if !dynamics.Bounded(x, y, z) {
				p = m.Start(o, orbits)
				continue
			}
			p = [3]float64{x, y, z}
			density.d.Add(x, y)
		}
		mapOrbits.Set(o, p)
	}

	// The box's corners, for the camera's fit and centering; count 0 draws none.
	lo, hi := density.d.Lo, density.d.Hi
	c := &density.corners
	*c = [16]float32{
		float32(lo[0]), float32(lo[1]), 0, 0,
		float32(hi[0]), float32(lo[1]), 0, 1. / 3,
		float32(lo[0]), float32(hi[1]), 0, 2. / 3,
		float32(hi[0]), float32(hi[1]), 0, 1,
	}
	gpu.uploadVerticesOnly(c[:], glctx.Types.Points, 0)
	if centerWarm.on {
		return
	}
	// Switched on: fit the camera to the box, which takes in the sparse outer
	// reaches a frame of points does not.
	if density.refit {
		density.refit = false
		view.autoFitCamera()
	}
	if density.frame%densityToneEvery == 0 {
		densityTone()
	}
	density.frame++
	drawDensityQuad()
}

// densityTone turns the counts into the image: each cell's log-scaled level
// through the trace's colors, and dimmed by it, so an empty cell is black and
// a rarely visited one faint.
func densityTone() {
	d := &density.d
	densityLUT()
	if need := d.W * d.H * 4; len(density.pix) != need {
		density.pix = make([]byte, need)
		density.pixJS = js.Global().Get("Uint8Array").New(need)
		density.levels = make([]byte, d.W*d.H)
	}
	// Each level's final color, once, then one lookup per cell: a color and a
	// square root per cell was most of the frame.
	var rgb [256][3]byte
	for lv := 1; lv < 256; lv++ {
		col, b := density.lut[lv], float64(lv)/255
		for a := range 3 {
			rgb[lv][a] = byte(float64(col[a])*b*255 + 0.5)
		}
	}
	d.LevelBytes(density.levels)
	pix := density.pix
	for k, lv := range density.levels {
		c, j := rgb[lv], k*4
		pix[j], pix[j+1], pix[j+2], pix[j+3] = c[0], c[1], c[2], 255
	}
	js.CopyBytesToJS(density.pixJS, pix)
	gl := glctx.GL
	if density.tex.IsUndefined() {
		density.tex = gl.Call("createTexture")
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), density.tex)
		// Not a power of two: WebGL 1 needs clamping and no mipmaps for it.
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
	}
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), density.tex)
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), d.W, d.H, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), density.pixJS)
}

// densityLUT fills the level-to-color table from the trace's colors: a named
// colormap where one is chosen, and otherwise the gradient's own colors, as
// the trace's shader mixes them.
func densityLUT() {
	idx, named := colormap.Index(style.gradientColors)
	for i := range density.lut {
		t := float64(i) / 255
		if style.gradientReverse {
			t = 1 - t
		}
		var c [3]float32
		switch {
		case named:
			r, g, b, _ := colormap.At(idx, t).RGBA()
			c = [3]float32{float32(r) / 65535, float32(g) / 65535, float32(b) / 65535}
		case style.gradientColors == 1:
			c = style.baseColor
		case style.gradientColors == 3:
			if t < 0.5 {
				c = mixColor(style.baseColor, style.midColor, float32(t*2))
			} else {
				c = mixColor(style.midColor, style.topColor, float32(t*2-1))
			}
		case style.gradientColors == 4:
			c = hueColor(t)
		default:
			c = mixColor(style.baseColor, style.topColor, float32(t))
		}
		density.lut[i] = c
	}
}

func mixColor(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

// hueColor is a fully saturated, full-value color at hue h (0..1), the hue
// sweep's colors.
func hueColor(h float64) [3]float32 {
	h = math.Mod(h, 1) * 6
	x := float32(1 - math.Abs(math.Mod(h, 2)-1))
	switch int(h) {
	case 0:
		return [3]float32{1, x, 0}
	case 1:
		return [3]float32{x, 1, 0}
	case 2:
		return [3]float32{0, 1, x}
	case 3:
		return [3]float32{0, x, 1}
	case 4:
		return [3]float32{x, 0, 1}
	}
	return [3]float32{1, 0, x}
}

// drawDensityQuad draws the image on the box, through the textured program
// and so with the same camera, added to what is behind it: black adds
// nothing, so the backdrop shows round the figure.
func drawDensityQuad() {
	t := &texp
	if !t.ready || density.tex.IsUndefined() {
		return
	}
	gl := glctx.GL
	lo, hi, off := density.d.Lo, density.d.Hi, sim.centerOffset
	x0, y0 := float32(lo[0])-off[0], float32(lo[1])-off[1]
	x1, y1 := float32(hi[0])-off[0], float32(hi[1])-off[1]
	z := -off[2]
	density.quadV = [20]float32{
		x0, y0, z, 0, 0,
		x1, y0, z, 1, 0,
		x0, y1, z, 0, 1,
		x1, y1, z, 1, 1,
	}
	if density.quad.IsUndefined() {
		density.quad = gl.Call("createBuffer")
	}
	t.useTexProgram()
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, density.quad)
	gl.Call("bufferData", glctx.Types.ArrayBuffer, glctx.SliceToTypedArray(density.quadV[:]), gl.Get("DYNAMIC_DRAW"))
	gl.Call("vertexAttribPointer", t.posLoc, 3, glctx.Types.Float, false, 20, 0)
	gl.Call("enableVertexAttribArray", t.posLoc)
	gl.Call("vertexAttribPointer", t.uvLoc, 2, glctx.Types.Float, false, 20, 12)
	gl.Call("enableVertexAttribArray", t.uvLoc)
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), density.tex)
	gl.Call("uniform1i", t.uSamplerLoc, 0)
	gl.Call("uniform1f", t.uOffsetLoc, 0)
	gl.Call("uniform1f", t.uLUTOnLoc, 0)
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("ONE"), gl.Get("ONE"))
	gl.Call("drawArrays", gl.Get("TRIANGLE_STRIP"), 0, 4)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
	// Back to the trace's program, which everything after this assumes.
	// Both of its arrays off: left on, the quad's four-vertex buffer stayed
	// bound to an attribute the next model's first draw could read past.
	gl.Call("disableVertexAttribArray", t.posLoc)
	gl.Call("disableVertexAttribArray", t.uvLoc)
	gl.Call("useProgram", gpu.program)
}

// wireDensity follows the density view's switch. Every switch-on counts
// afresh.
func wireDensity() {
	sw := dom.Doc.Call("getElementById", "density-sw")
	if !sw.Truthy() {
		return
	}
	dom.On(sw, "change", func(this js.Value, _ []js.Value) any {
		density.on = sw.Get("checked").Bool()
		density.refit = true
		densityInvalidate()
		return nil
	})
}

// densityShowing reports whether mode is drawn as its density image now.
func densityShowing(mode string) bool {
	return density.on && dynamics.IsMap(mode)
}
