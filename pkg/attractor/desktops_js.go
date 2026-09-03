//go:build js && wasm

package attractor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// Four 3-D desktops, reproduced.
//
// "The only window manager with a 3-D scene" would have been a nice claim and
// it is not true: people have been putting windows in three dimensions since
// 2003, and the good ideas are theirs. These are reproductions of four of them,
// over a desk whose wallpaper happens to be an attractor being integrated.
//
// WHAT MAKES THIS POSSIBLE IS CSS 3-D TRANSFORMS, and it is worth saying why
// after the trouble taken elsewhere. A window cannot be drawn into the WebGL
// scene, because a window is not only its pane: the title, the buttons and the
// border are DOM, and DOM cannot be sampled into a texture. But DOM can be
// TRANSFORMED. rotateY on an element is a real perspective projection performed
// by the compositor, the element stays live, and the browser hit-tests it at
// the place it appears — so a window tilted forty degrees is still a window you
// can type into. That is the same trick Metisse played on X11 in 2004, by
// redirecting windows to offscreen pixmaps and texturing them; here the browser
// does the redirection and there is nothing to texture.
//
// So these are not pictures of the originals. The windows are real, the shells
// in them are running, and the tilted ones still take the keyboard.
var (
	deskStyle    = deskFlat
	deskTicking  bool
	deskTickFunc js.Func
	deskCubeYaw  float64 // degrees, the cube's rotation about Y
)

// buildDeskStyleSelect fills the selector from the tables above.
//
// The options were written out in the panel's HTML first, which is two lists of
// the same five things that no compiler checks against each other — the same
// shape as the category tooltip that had quietly stopped mentioning Maps. One
// list now, in Go, and the markup is an empty select.
func buildDeskStyleSelect() {
	sel := doc.Call("getElementById", "desk-style")
	if !sel.Truthy() {
		return
	}
	sel.Set("innerHTML", "")
	for _, k := range deskStyleOrder {
		o := doc.Call("createElement", "option")
		o.Set("value", k)
		o.Set("textContent", deskStyleLabel[k])
		sel.Call("appendChild", o)
	}
	sel.Set("value", deskStyle)
	// A rotary with a name readout, like the phosphor selector, rather than the
	// bare dropdown this used to be. Five named options is exactly the case
	// that knob is for — too many for a label ring, too few to need a list —
	// and a dropdown in a rack panel reads as a browser widget somebody forgot
	// to finish. The select stays as the state and the knob is a view of it.
	if holder := doc.Call("getElementById", "desk-style-stack"); holder.Truthy() &&
		!holder.Get("firstChild").Truthy() {
		holder.Call("appendChild", selectorKnobReadout(sel))
	}
}

// deskWindows returns the live windows, in the DOM order winbox keeps them.
func deskWindows() []js.Value {
	if !deskEl.Truthy() {
		return nil
	}
	list := deskEl.Call("querySelectorAll", ".winbox")
	out := make([]js.Value, 0, list.Get("length").Int())
	for i := 0; i < list.Get("length").Int(); i++ {
		out = append(out, list.Index(i))
	}
	return out
}

// setDeskStyle switches desktops.
func setDeskStyle(s string) {
	if _, ok := deskStyleLabel[s]; !ok {
		s = deskFlat
	}
	deskStyle = s
	clearDeskTransforms()
	teardownCube()

	root := deskEl
	if !root.Truthy() {
		return
	}
	// The perspective lives on the ROOT rather than on each window, so every
	// window shares one vanishing point. Per-element perspective gives each its
	// own, and a row of windows then looks like a row of separate photographs
	// rather than objects in one room.
	if s == deskFlat {
		root.Get("style").Set("perspective", "")
		root.Get("style").Set("perspectiveOrigin", "")
	} else {
		root.Get("style").Set("perspective", "1400px")
		root.Get("style").Set("perspectiveOrigin", "50% 40%")
	}

	if s == deskCube {
		buildCube()
	}
	if s == deskBump {
		seedBump()
	}
	deskStyleTick() // one frame immediately, so switching is not a wait
	setDeskTicking(s != deskFlat)
}

// setDeskTicking starts or stops the per-frame loop.
//
// Only the styles that move need it. Looking Glass and Metisse are static
// arrangements — they are reapplied when something changes, not sixty times a
// second — but they share the loop because a window can be opened, moved or
// focused at any time and there is no event for "winbox restacked".
func setDeskTicking(on bool) {
	if on == deskTicking {
		return
	}
	deskTicking = on
	if !on {
		return
	}
	if !deskTickFunc.Truthy() {
		deskTickFunc = trackedFuncOf(func(js.Value, []js.Value) interface{} {
			if !deskTicking {
				return nil
			}
			deskStyleTick()
			js.Global().Call("requestAnimationFrame", deskTickFunc)
			return nil
		})
	}
	js.Global().Call("requestAnimationFrame", deskTickFunc)
}

func clearDeskTransforms() {
	for _, w := range deskWindows() {
		st := w.Get("style")
		st.Set("transform", "")
		st.Set("transformStyle", "")
		st.Set("transformOrigin", "")
		st.Set("transition", "")
		// Opacity too. The cube dims the faces that have turned away, and
		// without clearing it a window that happened to be edge-on when the
		// desktop was switched stays at twelve percent for good — visible as
		// "that window went nearly invisible and never came back".
		st.Set("opacity", "")
		removeBackFace(w)
	}
}

func deskStyleTick() {
	switch deskStyle {
	case deskGlass:
		tickGlass()
	case deskCube:
		tickCube()
	case deskMetisse:
		tickMetisse()
	case deskBump:
		tickBump()
	}
}

// --- Sun's Project Looking Glass, 2003 ---
//
// Its signature was that a window was an object with two sides: you could
// rotate one away to a slant and write a note on its BACK. The tilted stack in
// the screenshots everybody remembers is the side effect of that — windows kept
// at an angle so several are legible at once.
//
// The back face here carries the window's title and the shell's own suggestion,
// because a back face with nothing on it is just a dark rectangle and misses
// the point of the original entirely.

func tickGlass() {
	wins := deskWindows()
	for i, w := range wins {
		st := w.Get("style")
		st.Set("transformStyle", "preserve-3d")
		st.Set("transformOrigin", "0% 50%")
		st.Set("transition", "transform 320ms ease")
		if isFlipped(w) {
			// Turned about its MIDDLE, unlike the leaning stack, which hinges
			// on its left edge. Hinging a flip would swing the window a full
			// width to the left and halfway off the screen — the stack wants a
			// hinge and a flip wants a spindle.
			st.Set("transformOrigin", "50% 50%")
			st.Set("transform", "rotateY(180deg)")
			ensureBackFace(w)
			continue
		}
		removeBackFace(w)
		// The frontmost window stands nearly upright and the ones behind lean
		// away, which is what makes several readable at once.
		depth := float64(len(wins)-1-i) + 1
		angle := -18.0 - 7.0*depth
		if angle < -62 {
			angle = -62
		}
		st.Set("transform", fmt.Sprintf("translateZ(%.0fpx) rotateY(%.1fdeg)", -60*depth, angle))
	}
}

func isFlipped(w js.Value) bool { return w.Get("__lgFlipped").Truthy() }

// flipDeskWindow turns a window over. This is the Looking Glass gesture, and it
// is bound to a double click on the title bar because winbox already spends the
// single click on focus and the drag on moving.
func flipDeskWindow(w js.Value) {
	w.Set("__lgFlipped", !isFlipped(w))
	deskStyleTick()
}

func ensureBackFace(w js.Value) {
	if w.Get("__lgBack").Truthy() {
		return
	}
	title := "window"
	if t := w.Call("querySelector", ".wb-title"); t.Truthy() {
		title = t.Get("textContent").String()
	}
	back := doc.Call("createElement", "div")
	back.Set("className", "lg-back")
	// z-index above the window's own chrome: without it the title bar shows
	// through the back panel, mirrored, which reads as a rendering fault
	// rather than as the back of something.
	back.Set("style", "position:absolute;inset:0;z-index:9;transform:rotateY(180deg);"+
		"backface-visibility:hidden;background:#141821;color:#9fb4c8;"+
		"border:1px solid #2a3340;box-sizing:border-box;padding:14px 16px;"+
		"font:12px/1.7 'B612 Mono',ui-monospace,monospace;pointer-events:none;")
	back.Set("innerHTML", "<b style=\"color:#c9d6e2\">"+title+"</b><br><br>"+
		"the back of a window.<br><br>"+
		"<span style=\"color:#6b7f92\">Sun's Project Looking Glass, 2003 — a window was an "+
		"object with two sides, and this is the side you wrote notes on. "+
		"Double-click the title bar to turn it back over.</span>")
	w.Call("appendChild", back)
	w.Set("__lgBack", back)
}

func removeBackFace(w js.Value) {
	if b := w.Get("__lgBack"); b.Truthy() {
		b.Call("remove")
		w.Set("__lgBack", js.Undefined())
	}
}

// --- Compiz's desktop cube, 2006 ---
//
// Four workspaces on the faces of a cube you spin. Compiz drew the cube by
// texturing each workspace onto a face; this hangs the actual windows on the
// faces instead, so the workspace you are looking at is one you can use, and
// the ones edge-on are still running.
//
// The windows are not reparented. Moving a window into another element would
// make winbox's coordinates meaningless and break dragging; each window is
// placed on its face by transform alone.

func buildCube()    { deskCubeYaw = 0 }
func teardownCube() {}

// faceOf assigns a window to a workspace. Round-robin by open order, which is
// what an unconfigured Compiz did with new windows too.

func tickCube() {
	wins := deskWindows()
	if len(wins) == 0 {
		return
	}
	// ONE AXIS FOR ALL OF THEM, which is the whole difference between a cube
	// and four windows each swinging about its own middle. Windows cannot be
	// reparented into face elements — winbox positions them with left and top
	// and moving them under another element would make dragging meaningless —
	// so instead every window is given a transform-origin at the SAME point in
	// the page, expressed in its own coordinates. Rotating about a shared
	// origin is rotating about a shared axis.
	cx := js.Global().Get("innerWidth").Float() / 2
	cy := deskFloor() / 2

	for i, w := range wins {
		st := w.Get("style")
		st.Set("transformStyle", "preserve-3d")
		st.Set("transition", "")
		left := w.Get("offsetLeft").Float()
		top := w.Get("offsetTop").Float()
		st.Set("transformOrigin", fmt.Sprintf("%.1fpx %.1fpx", cx-left, cy-top))

		face := float64(faceOf(i)) * (360.0 / deskFaces)
		// The sandwich is what puts the front face where the window already
		// is: out to the face, back by the same radius. Without the first
		// translateZ the whole cube sits a radius closer to the camera and the
		// front face arrives magnified.
		st.Set("transform", fmt.Sprintf(
			"translateZ(%.0fpx) rotateY(%.2fdeg) translateZ(%.0fpx)",
			-cubeRadius, deskCubeYaw+face, cubeRadius))

		// Faces past the edge dim rather than vanish, so a spin reads as one
		// object turning instead of windows blinking out.
		rel := math.Mod(math.Abs(deskCubeYaw+face), 360)
		if rel > 180 {
			rel = 360 - rel
		}
		op := 1.0 - rel/150
		if op < 0.12 {
			op = 0.12
		}
		st.Set("opacity", strconv.FormatFloat(op, 'f', 2, 64))
	}
}

// cubeRadius is half the cube's edge. Fixed rather than derived from the
// viewport: the faces hold windows of different sizes, so there is no width
// that makes them meet, and a radius that changes with the window would make
// the cube breathe as the page resizes.
const cubeRadius = 520.0

// deskFloor is the top of the rack, which is as far down as the desk goes.
func deskFloor() float64 {
	if p := doc.Call("getElementById", "controls-panel"); p.Truthy() {
		if t := p.Call("getBoundingClientRect").Get("top").Float(); t > 0 {
			return t
		}
	}
	return js.Global().Get("innerHeight").Float()
}

// spinCube turns the cube by d degrees. Bound to the arrow keys while the cube
// desktop is on, which is what Compiz used (with ctrl+alt held).
func spinCube(d float64) {
	deskCubeYaw = math.Mod(deskCubeYaw+d, 360)
	deskStyleTick()
}

// --- Metisse, 2004 ---
//
// The research one, and the most general: every window freely rotatable and
// still interactive, so you could turn a window aside to see what was behind it
// without unfocusing it. Metisse achieved that by redirecting X windows to
// offscreen pixmaps and texturing them; the browser gives it away for nothing,
// and the windows stay live for the same reason they did there.
//
// Shift-drag a title bar to turn a window. Plain drag still moves it, because
// taking that away to make room for a demo would be a bad trade.

// num reads a number stashed on an element, treating anything else as zero.
//
// js.Value.Float() PANICS on undefined — it does not return NaN, which is what
// I assumed and what cost an afternoon: the first frame of Metisse read an
// unset property, panicked, and took the whole Go program down with it. Every
// window froze, and because a dead program only complains when something calls
// into it, the failure looked like "the transform is empty" rather than like a
// crash.
func num(v js.Value) float64 {
	if v.Type() != js.TypeNumber {
		return 0
	}
	f := v.Float()
	if math.IsNaN(f) {
		return 0
	}
	return f
}

func tickMetisse() {
	for _, w := range deskWindows() {
		st := w.Get("style")
		st.Set("transformStyle", "preserve-3d")
		st.Set("transformOrigin", "50% 50%")
		st.Set("transition", "")
		rx := num(w.Get("__mtRX"))
		ry := num(w.Get("__mtRY"))
		st.Set("transform", fmt.Sprintf("rotateX(%.2fdeg) rotateY(%.2fdeg)", rx, ry))
	}
}

func metisseTurn(w js.Value, dx, dy float64) {
	rx := num(w.Get("__mtRX"))
	ry := num(w.Get("__mtRY"))
	// Clamped short of ninety degrees: edge-on, a window is a line, and a
	// window you cannot see is one you cannot turn back.
	rx = clampDeg(rx-dy*0.4, 75)
	ry = clampDeg(ry+dx*0.4, 75)
	w.Set("__mtRX", rx)
	w.Set("__mtRY", ry)
	deskStyleTick()
}

// --- BumpTop, 2009 ---
//
// The one that treated windows as physical objects: they had mass, they fell,
// they piled against the edges of the desk. It was bought by Google in 2010 and
// disappeared.
//
// This is a small rigid-body loop rather than a real solver — gravity, a floor,
// and separation between overlapping windows — because what made BumpTop feel
// like BumpTop was not the fidelity of its physics but that windows landed on
// each other instead of overlapping. Windows keep their left/top from winbox;
// the fall is a transform, so dragging one still works and dropping it lets it
// fall from wherever it was let go.

const (
	bumpGravity = 0.55
	bumpDamp    = 0.72 // restitution on landing
	bumpFloorPd = 8.0  // how far above the panel a window comes to rest
)

func seedBump() {
	for _, w := range deskWindows() {
		w.Set("__bpY", 0.0)
		w.Set("__bpV", 0.0)
		w.Set("__bpA", 0.0)  // tilt, degrees
		w.Set("__bpAV", 0.0) // angular velocity
	}
}

// bumpScale shrinks the windows on the way into the pile.
//
// BumpTop did this too, and here it is also the difference between a pile and
// no pile: a window is nearly as tall as the space above the rack, so at full
// size the first one to land fills the floor and nothing can stack on it. At a
// third, three of them fit with room to fall.
const bumpScale = 0.34

func tickBump() {
	wins := deskWindows()
	floor := deskFloor() - 6
	// Windows pile ON the rack rather than at the bottom of the viewport,
	// which is the right floor for this desk and a better joke besides.

	type placed struct{ top, left, right float64 }
	var boxes []placed

	for _, w := range wins {
		y := num(w.Get("__bpY"))
		v := num(w.Get("__bpV"))
		a := num(w.Get("__bpA"))
		av := num(w.Get("__bpAV"))

		// LAYOUT coordinates, not getBoundingClientRect: the rect already has
		// this transform in it, so reading it back and adding to it is a loop
		// that feeds itself. offsetTop is where winbox put the window and does
		// not move when it is transformed.
		left := w.Get("offsetLeft").Float()
		top := w.Get("offsetTop").Float()
		width := w.Get("offsetWidth").Float()
		height := w.Get("offsetHeight").Float()

		// Scaled from the TOP edge, not the bottom. Scaling about the bottom
		// leaves the bottom where the layout put it — and the layout already
		// puts a full-height window below the rack, so every window would be
		// resting before it started and the pile would be a shove upward. From
		// the top, a shrunk window hangs where its title bar is and has the
		// whole floor to fall to.
		hEff := height * bumpScale
		wEff := width * bumpScale
		lEff := left + (width-wEff)/2
		rEff := lEff + wEff

		v += bumpGravity
		y += v
		a += av
		av *= 0.96

		// The floor, and the top of anything already resting under this one.
		limit := floor - hEff - top
		for _, b := range boxes {
			if rEff < b.left || lEff > b.right {
				continue // no horizontal overlap; it falls past
			}
			if l := b.top - hEff - top; l < limit {
				limit = l
			}
		}
		// A pile taller than the room overflows upward rather than off the
		// screen: the last window rests against the top edge instead of
		// disappearing above it, which is wrong physics and right behavior.
		if limit < -top {
			limit = -top
		}
		if y >= limit {
			if v > 3 {
				// A landing worth noticing takes a little spin from the
				// impact, which is what made BumpTop's piles look dropped
				// rather than stacked.
				av += (v * 0.05) * bumpSign(w)
			}
			y = limit
			v = -v * bumpDamp
			if math.Abs(v) < 1.5 {
				v = 0
				av *= 0.4
			}
		}
		a = clampDeg(a, 12)

		w.Set("__bpY", y)
		w.Set("__bpV", v)
		w.Set("__bpA", a)
		w.Set("__bpAV", av)

		st := w.Get("style")
		st.Set("transformStyle", "")
		st.Set("transformOrigin", "50% 0%")
		st.Set("transition", "")
		st.Set("transform", fmt.Sprintf("translateY(%.1fpx) rotate(%.2fdeg) scale(%.3f)", y, a, bumpScale))

		boxes = append(boxes, placed{top: top + y, left: lEff, right: rEff})
	}
}

// bumpSign gives each window a stable direction to tip in, so a pile leans
// both ways instead of every window rolling the same side.
func bumpSign(w js.Value) float64 {
	if id := w.Get("id"); id.Truthy() && len(id.String())%2 == 0 {
		return 1
	}
	return -1
}

// --- the gestures ---
//
// Each original had one, and they are kept because the gesture is most of what
// the thing was. What they must not do is take input the rest of the app is
// already using, so each is qualified twice: by the desktop being on, and by
// the style that owns it being selected.

var deskGesturesWired bool

func wireDeskGestures() {
	if deskGesturesWired {
		return
	}
	deskGesturesWired = true

	// Looking Glass: turn a window over. A double click, because winbox has
	// already spent the single one on focus and the drag on moving.
	doc.Call("addEventListener", "dblclick", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if deskStyle != deskGlass || len(a) == 0 {
			return nil
		}
		// The TITLE BAR only. Anywhere-in-the-window would mean a double
		// click on a word in a terminal — the ordinary way to select one —
		// flipping the window over instead.
		t := a[0].Get("target")
		if !inTitleBar(t) {
			return nil
		}
		if w := closestWinbox(t); w.Truthy() {
			flipDeskWindow(w)
		}
		return nil
	}))

	// Metisse: shift-drag a title bar to turn the window. Plain drag still
	// moves it — taking that away to make room for a demo would be a bad
	// trade, and Metisse itself kept the ordinary drag too.
	var turning js.Value
	var lastX, lastY float64
	doc.Call("addEventListener", "mousedown", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if deskStyle != deskMetisse || len(a) == 0 || !a[0].Get("shiftKey").Truthy() {
			return nil
		}
		w := closestWinbox(a[0].Get("target"))
		if !w.Truthy() {
			return nil
		}
		turning = w
		lastX, lastY = a[0].Get("clientX").Float(), a[0].Get("clientY").Float()
		a[0].Call("preventDefault")
		a[0].Call("stopPropagation")
		return nil
	}), map[string]any{"capture": true})
	doc.Call("addEventListener", "mousemove", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if !turning.Truthy() || len(a) == 0 {
			return nil
		}
		x, y := a[0].Get("clientX").Float(), a[0].Get("clientY").Float()
		metisseTurn(turning, x-lastX, y-lastY)
		lastX, lastY = x, y
		return nil
	}), map[string]any{"capture": true})
	doc.Call("addEventListener", "mouseup", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		turning = js.Value{}
		return nil
	}), map[string]any{"capture": true})

	// Compiz: the arrows spin the cube. Guarded the same way every other key
	// binding here is — a focused input, select or textarea keeps its arrows,
	// which is what lets a terminal in one of these windows still work.
	doc.Call("addEventListener", "keydown", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if deskStyle != deskCube || len(a) == 0 {
			return nil
		}
		// A KNOB UNDER THE POINTER OWNS THE ARROWS. Both bindings are on the
		// document and neither used to know about the other, so with the cube up
		// and the pointer resting anywhere on the panel — which is most of the
		// screen — one press of Right nudged a parameter AND spun the cube.
		// Measured: speed 0.90 to 0.92 and the cube 7.5 to 15 degrees, from a
		// single keystroke. Steering the cube quietly edited the model.
		//
		// The hovered knob wins because it is the specific target and the cube
		// is the ambient one; hoverNudge is exactly "the pointer is on a knob".
		if hoverNudge != nil {
			return nil
		}
		e := a[0]
		if t := e.Get("target"); t.Truthy() {
			switch strings.ToLower(t.Get("tagName").String()) {
			case "input", "select", "textarea":
				return nil
			}
		}
		switch e.Get("key").String() {
		case "ArrowLeft":
			spinCube(-90.0 / 12)
		case "ArrowRight":
			spinCube(90.0 / 12)
		default:
			return nil
		}
		e.Call("preventDefault")
		return nil
	}))
}

// inTitleBar reports whether an event happened on a window's title.
//
// The title rather than the whole header: the header also holds the minimize,
// maximize and close buttons, and a gesture that fires on those would run
// alongside whatever winbox already does with them.
func inTitleBar(t js.Value) bool {
	if !t.Truthy() || t.Get("closest").Type() != js.TypeFunction {
		return false
	}
	return t.Call("closest", ".wb-title").Truthy()
}

// closestWinbox walks up to the window an event happened in.
func closestWinbox(t js.Value) js.Value {
	if !t.Truthy() || t.Get("closest").Type() != js.TypeFunction {
		return js.Value{}
	}
	w := t.Call("closest", ".winbox")
	if !w.Truthy() {
		return js.Value{}
	}
	// Only windows on this desk; the page has no others, but a selector that
	// says what it means survives someone adding some.
	if deskEl.Truthy() && !deskEl.Call("contains", w).Bool() {
		return js.Value{}
	}
	return w
}
