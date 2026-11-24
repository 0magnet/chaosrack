//go:build js && wasm

package attractor

// MEASURE: pick points on the figure and measure between them. It is a
// position of the model bank (1.12.3) under the figures of lines.
//
// MODE arms it — dist takes two points, angle three, the middle one the
// vertex — and while it is armed a press that lands near a point that
// matters takes that point instead of turning the view: a figure's centers,
// where its circles and lines meet (solved exactly), its corners, where two
// of its lines cross in space (pkg/measure). A press on empty space still
// turns the view, so the figure can be turned between picks. A ring follows
// the cursor to show which point a press would take.
//
// The first distance measured is the unit, and every one after it is read
// as a ratio to it and named when it is a number worth naming: measure the
// vesica's line of centers and then its chord, and the chord says √3. U
// makes the last distance the unit, 1 forgets the unit, C clears.
//
// What is measured is drawn over the figure on a canvas of its own, each
// frame, through the same matrix as the figure, so it turns with it.

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/measure"
	"github.com/go-gl/mathgl/mgl32"
)

// measureResult is one measurement: two points (a distance) or three (an
// angle at the middle one).
type measureResult struct {
	pts []measure.P
}

var meas struct {
	scene     measure.Scene
	sceneSeq  uint64
	sceneMode string
	sceneOK   bool

	pending []measure.P
	results []measureResult
	unit    float64
	unitTag string // which measurement the unit is

	hover    measure.Snapped
	hovering bool

	overlay js.Value
	ctx     js.Value
	drawn   bool // whether the overlay has anything on it to clear
	w, h    float64
}

// measureSnapPx is how near a point a press must land to take it.
const measureSnapPx = 14

// measureMode is MODE's position: 0 off, 1 distance, 2 angle.
func measureMode() int {
	if !measurableModel(run.selectedMode) {
		return 0 // armed, but under a model it does not serve: off
	}
	return max(selectAt("measure-mode"), 0)
}

// measurePoints is how many points MODE's measurement takes.
func measurePoints() int {
	if measureMode() == 2 {
		return 3
	}
	return 2
}

// measureScene is the figure as measurable, rebuilt when the figure was.
// Only a figure of lines is: a flow's trail is a path, not a construction.
func measureScene() (*measure.Scene, bool) {
	if !gpu.indexMode.Truthy() || !gpu.indexMode.Equal(glctx.Types.Line) || len(gpu.indices) < 2 || len(gpu.verts) < 6 {
		return nil, false
	}
	if meas.sceneOK && meas.sceneSeq == gpu.uploadSeq && meas.sceneMode == run.selectedMode {
		return &meas.scene, true
	}
	v, idx := gpu.verts, gpu.indices
	at := func(i uint16) measure.P {
		k := int(i) * 3
		return measure.P{float64(v[k]), float64(v[k+1]), float64(v[k+2])}
	}
	s := measure.Scene{}
	for i := 0; i+1 < len(idx); i += 2 {
		if int(idx[i])*3+2 < len(v) && int(idx[i+1])*3+2 < len(v) {
			s.Segs = append(s.Segs, [2]measure.P{at(idx[i]), at(idx[i+1])})
		}
	}
	s.Prepare()
	meas.scene, meas.sceneSeq, meas.sceneMode, meas.sceneOK = s, gpu.uploadSeq, run.selectedMode, true
	return &meas.scene, true
}

// measureProject is a point of the figure on the canvas, in CSS pixels,
// through the matrix the figure is drawn with.
func measureProject(w, h float64) measure.Project {
	mvp := mvpNow()
	return func(p measure.P) (float64, float64, bool) {
		c := mvp.Mul4x1(mgl32.Vec4{float32(p[0]), float32(p[1]), float32(p[2]), 1})
		if c.W() <= 0 {
			return 0, 0, false
		}
		x, y := float64(c.X()/c.W()), float64(c.Y()/c.W())
		return (x + 1) / 2 * w, (1 - y) / 2 * h, true
	}
}

// measureSnap is the point a press at the client position would take.
func measureSnap(clientX, clientY float64) (measure.Snapped, bool) {
	if measureMode() == 0 || selectAt("view-n") > 0 {
		return measure.Snapped{}, false
	}
	s, ok := measureScene()
	if !ok {
		return measure.Snapped{}, false
	}
	cx, cy, w, h, ok := canvasPoint(clientX, clientY)
	if !ok {
		return measure.Snapped{}, false
	}
	return s.Snap(float64(cx), float64(cy), measureProject(float64(w), float64(h)), measureSnapPx)
}

// measurePick takes the point under a press, if MODE is armed and one is
// near, and reports whether it did, so the press does not also turn the view.
func measurePick(clientX, clientY float64) bool {
	p, ok := measureSnap(clientX, clientY)
	if !ok {
		return false
	}
	meas.pending = append(meas.pending, p.P)
	if len(meas.pending) >= measurePoints() {
		r := measureResult{pts: meas.pending}
		meas.results = append(meas.results, r)
		meas.pending = nil
		if len(r.pts) == 2 && meas.unit == 0 {
			if d := measure.Dist(r.pts[0], r.pts[1]); d > 0 {
				meas.unit, meas.unitTag = d, string(rune('A'+(len(meas.results)-1)%26))
			}
		}
	}
	return true
}

// measureHover moves the ring that shows which point a press would take.
func measureHover(clientX, clientY float64) {
	if measureMode() == 0 {
		meas.hovering = false
		return
	}
	meas.hover, meas.hovering = measureSnap(clientX, clientY)
	if meas.hovering {
		setCanvasCursor("crosshair")
	} else {
		setCanvasCursor("")
	}
}

// measureClear forgets every measurement and the unit.
func measureClear() {
	meas.pending, meas.results, meas.unit = nil, nil, 0
}

// measureModelChanged forgets the measurements: they were of another figure.
func measureModelChanged() {
	if meas.sceneMode != "" && meas.sceneMode != run.selectedMode {
		measureClear()
	}
}

// measureText is a measurement as it reads: a distance as a ratio to the
// unit, named when it has a name; an angle in degrees, named likewise.
func measureText(r measureResult) string {
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', 4, 64) }
	if len(r.pts) == 3 {
		a := measure.Angle(r.pts[0], r.pts[1], r.pts[2])
		if name, ok := measure.RecognizeAngle(a); ok {
			return name
		}
		return strconv.FormatFloat(a, 'f', 2, 64) + "°"
	}
	d := measure.Dist(r.pts[0], r.pts[1])
	if meas.unit <= 0 {
		return f(d)
	}
	x := d / meas.unit
	if name, ok := measure.Recognize(x); ok {
		if name == "1" {
			return "1 (unit)"
		}
		return name + "  " + f(x)
	}
	return f(x)
}

// drawMeasureOverlay draws the measurements over the figure, each frame
// while there are any or MODE is armed.
func drawMeasureOverlay() {
	active := measureMode() != 0 || len(meas.results) > 0 || len(meas.pending) > 0
	if !active && !meas.drawn {
		return
	}
	measureModelChanged()
	if !meas.overlay.Truthy() {
		c := dom.Doc.Call("createElement", "canvas")
		c.Set("id", "measure-overlay")
		c.Get("style").Set("cssText", "position:fixed;left:0;top:0;pointer-events:none;z-index:var(--z-crt);")
		dom.Doc.Get("body").Call("appendChild", c)
		meas.overlay, meas.ctx = c, c.Call("getContext", "2d")
	}
	if !glctx.Canvas.Truthy() {
		return
	}
	r := glctx.Canvas.Call("getBoundingClientRect")
	left, top := r.Get("left").Float(), r.Get("top").Float()
	w, h := r.Get("width").Float(), r.Get("height").Float()
	dpr := js.Global().Get("devicePixelRatio").Float()
	st := meas.overlay.Get("style")
	if w != meas.w || h != meas.h {
		meas.overlay.Set("width", int(w*dpr))
		meas.overlay.Set("height", int(h*dpr))
		st.Set("width", strconv.FormatFloat(w, 'f', 1, 64)+"px")
		st.Set("height", strconv.FormatFloat(h, 'f', 1, 64)+"px")
		meas.w, meas.h = w, h
	}
	st.Set("transform", "translate("+strconv.FormatFloat(left, 'f', 1, 64)+"px,"+strconv.FormatFloat(top, 'f', 1, 64)+"px)")
	g := meas.ctx
	g.Call("setTransform", dpr, 0, 0, dpr, 0, 0)
	g.Call("clearRect", 0, 0, w, h)
	meas.drawn = active
	if !active {
		return
	}
	proj := measureProject(w, h)
	g.Set("lineWidth", 1.5)
	g.Set("font", "13px ui-monospace, monospace")
	g.Set("textBaseline", "middle")
	dot := func(p measure.P, rad float64, color string, fill bool) {
		x, y, ok := proj(p)
		if !ok {
			return
		}
		g.Call("beginPath")
		g.Call("arc", x, y, rad, 0, 2*math.Pi)
		if fill {
			g.Set("fillStyle", color)
			g.Call("fill")
		} else {
			g.Set("strokeStyle", color)
			g.Call("stroke")
		}
	}
	label := func(x, y float64, text, color string) {
		g.Set("fillStyle", "rgba(0,0,0,0.65)")
		tw := g.Call("measureText", text).Get("width").Float()
		g.Call("fillRect", x-3, y-9, tw+6, 18)
		g.Set("fillStyle", color)
		g.Call("fillText", text, x, y)
	}
	const (
		cDist  = "#5ee7ff"
		cAngle = "#ffd166"
		cPend  = "#ff8a5c"
	)
	var lines []string
	for i, res := range meas.results {
		text := measureText(res)
		tag := string(rune('A' + i%26))
		lines = append(lines, tag+"  "+text)
		color := cDist
		if len(res.pts) == 3 {
			color = cAngle
		}
		g.Set("strokeStyle", color)
		g.Call("beginPath")
		ok := true
		for k, p := range res.pts {
			x, y, in := proj(p)
			ok = ok && in
			if k == 0 {
				g.Call("moveTo", x, y)
			} else {
				g.Call("lineTo", x, y)
			}
		}
		if !ok {
			continue
		}
		g.Call("stroke")
		for _, p := range res.pts {
			dot(p, 3, color, true)
		}
		if len(res.pts) == 3 {
			// The angle's arc at its vertex, on screen.
			bx, by, _ := proj(res.pts[1])
			ax, ay, _ := proj(res.pts[0])
			cx, cy, _ := proj(res.pts[2])
			a0, a1 := math.Atan2(ay-by, ax-bx), math.Atan2(cy-by, cx-bx)
			d := math.Remainder(a1-a0, 2*math.Pi)
			g.Call("beginPath")
			g.Call("arc", bx, by, 22, a0, a0+d, d < 0)
			g.Call("stroke")
			label(bx+26*math.Cos(a0+d/2), by+26*math.Sin(a0+d/2), tag+" "+text, color)
			continue
		}
		ax, ay, _ := proj(res.pts[0])
		bx, by, _ := proj(res.pts[1])
		// Beside the line rather than on its middle, where two lines that
		// cross (the vesica's chord and its line of centers) would put
		// both labels in one place: off to the line's right-hand or upper
		// side, along its normal.
		nx, ny := -(by - ay), bx-ax
		if l := math.Hypot(nx, ny); l > 0 {
			nx, ny = nx/l, ny/l
		}
		if ny > 0 || (ny == 0 && nx < 0) {
			nx, ny = -nx, -ny
		}
		if nx < -0.3 {
			nx, ny = -nx, -ny
		}
		label((ax+bx)/2+nx*16+2, (ay+by)/2+ny*16, tag+" "+text, color)
	}
	for _, p := range meas.pending {
		dot(p, 4, cPend, true)
	}
	if meas.hovering && measureMode() != 0 {
		dot(meas.hover.P, 7, "#ffffff", false)
		if x, y, ok := proj(meas.hover.P); ok {
			label(x+10, y+12, measure.KindNames[meas.hover.Kind], "#cccccc")
		}
	}
	// The list, top left: every measurement, the unit, and what MODE is
	// waiting for.
	head := []string{"MEASURE  " + []string{"off", "distance: 2 points", "angle: 3 points, the vertex second"}[measureMode()]}
	if meas.unit > 0 {
		head = append(head, "unit = "+meas.unitTag)
	}
	if n := len(meas.pending); n > 0 {
		head = append(head, strconv.Itoa(n)+" of "+strconv.Itoa(measurePoints())+" picked")
	}
	for i, l := range append(head, lines...) {
		label(12, 20+float64(i)*20, l, map[bool]string{true: "#ffffff", false: "#bfe9ff"}[i < len(head)])
	}
}

// measureReady is whether the cell's dial, buttons and control are made:
// once, however often the bay is built.
var measureReady bool

// measureCell is MEASURE's cell, ready for the bank: it is a position of
// the model bank, at 1.12.3 under the figures of lines, where Speed is
// under the models with a clock (rackcategory_js.go places it).
func measureCell() js.Value {
	c := dom.Doc.Call("getElementById", "measure-mode-cell")
	if !c.Truthy() || measureReady {
		return c
	}
	measureReady = true
	registerMeasureTrio()
	if sel, h := dom.Doc.Call("getElementById", "measure-mode"), dom.Doc.Call("getElementById", "measure-mode-stack"); sel.Truthy() && h.Truthy() {
		selectorReadout(h, sel, measureModeRing)
	}
	registerControl(ControlDesc{ID: "measure-mode", Label: "measure", IsSelect: true, SelectDef: "0"}, moduleOfControl("measure-mode"))
	// Turning it off ends a half-picked measurement; the finished ones stay
	// until C or a change of model.
	dom.On(dom.Doc.Call("getElementById", "measure-mode"), "change", func(js.Value, []js.Value) any {
		meas.pending = nil
		if measureMode() == 0 {
			meas.hovering = false
			setCanvasCursor("")
		}
		return nil
	})
	return c
}

// measurableModel is whether MEASURE serves a model: the figures of lines,
// the geometry models, which have no clock and so no Speed.
func measurableModel(mode string) bool { return modeInfo[mode].Class == ClassGeometry }

// measureModeRing is MODE's positions round its dial.
var measureModeRing = []string{"off", "dist", "ang"}

// registerMeasureTrio sets MODE's three buttons, before its cell is wired.
func registerMeasureTrio() {
	// U: the last distance is the unit. 1: forget the unit (the next
	// distance becomes it). C: clear everything measured.
	trioPrograms["measure-mode"] = trioProgram{
		keys: []string{"U", "1", "C"},
		help: []string{doc("trio.measure-mode=0"), doc("trio.measure-mode=1"), doc("trio.measure-mode=2")},
		press: func(i int) {
			switch i {
			case 0:
				for k := len(meas.results) - 1; k >= 0; k-- {
					if r := meas.results[k]; len(r.pts) == 2 {
						meas.unit, meas.unitTag = measure.Dist(r.pts[0], r.pts[1]), string(rune('A'+k%26))
						break
					}
				}
			case 1:
				meas.unit = 0
			default:
				measureClear()
			}
		},
		lit: func() int {
			if meas.unit > 0 {
				return 0
			}
			return -1
		},
		drive: []string{"measure-mode"},
	}
}
