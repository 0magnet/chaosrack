//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The stage: the part of the window the docked panel leaves uncovered, which
// is where the model is drawn. The canvas is the whole window and the panel
// lies over it, 85% opaque, so a model centered on the canvas was centered
// behind the panel's top edge: at power-up the drawer covers the bottom 560
// pixels and a first visit saw the top half of a globe. The projection now
// fits the model to the stage and centers it there (stageProjection), and
// a Grid lays its cells out in it (viewRects). Backdrops still fill the
// window, behind the panel as before.
//
// Fractions of the window, so a resize that keeps the dock where it is keeps
// the stage too; the whole window while the panel floats, is hidden, or is
// docked in a host's footer.
var stage = struct{ x0, y0, x1, y1 float64 }{0, 0, 1, 1}

// measureStage reads where the panel is and makes the stage the rest of the
// window, then gives the trace program the projection for it.
func measureStage() {
	x0, y0, x1, y1 := 0.0, 0.0, 1.0, 1.0
	if left, top, right, bottom, ok := panelCover(); ok {
		const edge, across = 0.02, 0.9
		switch {
		case right-left >= across && bottom >= 1-edge && top > 0:
			y1 = top // along the bottom
		case right-left >= across && top <= edge && bottom < 1:
			y0 = bottom // along the top
		case bottom-top >= across && left <= edge && right < 1:
			x0 = right // down the left
		case bottom-top >= across && right >= 1-edge && left > 0:
			x1 = left // down the right
		}
	}
	// A drawer pulled over nearly everything leaves too little to draw in:
	// the model goes back to the window then, behind the panel.
	if x1-x0 < 0.15 || y1-y0 < 0.15 {
		x0, y0, x1, y1 = 0, 0, 1, 1
	}
	if stage.x0 == x0 && stage.y0 == y0 && stage.x1 == x1 && stage.y1 == y1 {
		return
	}
	stage.x0, stage.y0, stage.x1, stage.y1 = x0, y0, x1, y1
	if gpu.ready {
		setupMatrices()
	}
}

// panelCover is the part of the window the panel covers, as fractions of it:
// the panel on screen (the page's own, docked, or the one a terminal's window
// holds, .cp-ctx) with the frames around it, out to the first one that is
// the whole window, and clipped to the window. The panel kept off screen
// while the terminal's window has the rack is not it. ok is false when no
// panel is on screen.
func panelCover() (left, top, right, bottom float64, ok bool) {
	w, h := winW(), winH()
	els := dom.Doc.Call("querySelectorAll", "#controls-panel, .cp-ctx")
	for i := range els.Length() {
		el := els.Index(i)
		r := el.Call("getBoundingClientRect")
		if r.Get("right").Float() <= 0 || r.Get("left").Float() >= w || r.Get("width").Float() == 0 || r.Get("height").Float() == 0 {
			continue
		}
		l, t, rt, b := w, h, 0.0, 0.0
		for e := scrollBox(el); e.Truthy() && !e.Equal(dom.Body); e = e.Get("parentElement") {
			// Clipped to the window first: the panel's own box is as tall as
			// everything it scrolls.
			r := e.Call("getBoundingClientRect")
			cl, ct := max(r.Get("left").Float(), 0), max(r.Get("top").Float(), 0)
			cr, cb := min(r.Get("right").Float(), w), min(r.Get("bottom").Float(), h)
			if cr-cl >= 0.95*w && cb-ct >= 0.95*h {
				break
			}
			l, t = min(l, cl), min(t, ct)
			rt, b = max(rt, cr), max(b, cb)
		}
		if rt <= l || b <= t {
			continue
		}
		return l / w, t / h, rt / w, b / h, true
	}
	return 0, 0, 0, 0, false
}

// wireStage measures the stage now, on a resize, and twice a second: the
// panel moves and resizes by more hands than one (the page's DOCK buttons and
// drag bar, a terminal laying its window out again) and measuring is cheaper
// than following each of them.
func wireStage() {
	measureStage()
	dom.On(js.Global(), "resize", func(js.Value, []js.Value) any {
		measureStage()
		return nil
	})
	js.Global().Call("setInterval", dom.FuncOf(func(js.Value, []js.Value) any {
		measureStage()
		return nil
	}), 500)
}

// stageProjection is the projection for the whole canvas, w by h pixels: the
// 45° camera on the stage's shape, carried from the middle of the canvas to
// the middle of the stage and scaled to its size. Clip space is the canvas,
// so the carry and the scale act after the perspective, on clip coordinates.
func stageProjection(w, h int) mgl32.Mat4 {
	sx, sy := float32(stage.x1-stage.x0), float32(stage.y1-stage.y0)
	aspect := float32(w) * sx / (float32(max(h, 1)) * sy)
	p := mgl32.Perspective(mgl32.DegToRad(45.0), aspect, 1, 1500.0)
	if sx == 1 && sy == 1 {
		return p
	}
	cx := float32(stage.x0+stage.x1) - 1
	cy := 1 - float32(stage.y0+stage.y1)
	return mgl32.Translate3D(cx, cy, 0).Mul4(mgl32.Scale3D(sx, sy, 1)).Mul4(p)
}

// stageRect is the stage in GL's pixels: x, y from the bottom, width, height.
func stageRect(w, h int) [4]int {
	x := int(stage.x0 * float64(w))
	y := int((1 - stage.y1) * float64(h))
	return [4]int{x, y, max(int(stage.x1*float64(w))-x, 1), max(int((1-stage.y0)*float64(h))-y, 1)}
}

// stageOn is the stage on a canvas cw by ch pixels, from its top left: the
// part a picture of the model takes, the monitors' and a recording's when no
// region was chosen. Taken whole, it was mostly the panel's shadow.
func stageOn(cw, ch float64) (x, y, w, h float64) {
	return stage.x0 * cw, stage.y0 * ch, (stage.x1 - stage.x0) * cw, (stage.y1 - stage.y0) * ch
}

// scrollBox is el or the nearest frame around it that scrolls: the box the
// panel is seen through. The panel's contents are as tall as all its bays, so
// scrolled down their box starts above the window, and measured from there it
// read as filling the window: the stage went to the whole window and the model
// jumped in size with every scroll of the panel.
func scrollBox(el js.Value) js.Value {
	cs := js.Global().Get("getComputedStyle")
	for e := el; e.Truthy() && !e.Equal(dom.Body); e = e.Get("parentElement") {
		switch cs.Invoke(e).Get("overflowY").String() {
		case "auto", "scroll":
			return e
		}
	}
	return el
}
