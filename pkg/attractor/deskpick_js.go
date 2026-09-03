//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"

	"github.com/0magnet/desk"
	"github.com/go-gl/mathgl/mgl32"
)

// Reaching into the desk while it is a model.
//
// The keyboard needed nothing: a key goes wherever the DOM focus is, and the
// windows are still in the DOM. The mouse is the half that has a position, so
// it needs the position translating — from a point on the screen, through a
// quad that may be turned to any angle, to the point on the desk that is drawn
// there.
//
// THE CHAIN IS SHORTER THAN IT LOOKS, and I said it was impractical twice
// before working it out.
//
//  1. Screen point to a ray, and the ray to the quad. Done in MODEL space,
//     where the quad is the plane z=0 and the arithmetic is one division:
//     unproject two points through the inverse of P·V·M and intersect.
//  2. Quad coordinates to canvas pixels. The quad's v runs bottom-up and the
//     texture is uploaded with UNPACK_FLIP_Y, so v=1 is the canvas's top row.
//  3. Canvas pixels to a PAGE point — and this is the step that makes the rest
//     unnecessary. The compositor's canvas is a 1:1 picture of the desk's own
//     rectangle on the page, so a canvas pixel IS a page point, give or take
//     the device pixel ratio. Which means there is nothing to look up: hand
//     that point to elementFromPoint and the real element comes back, hidden
//     though it is, and a synthesized event on it is handled by winbox's own
//     code with no help from here.
//
// WHY THE REAL EVENTS HAVE TO BE STOPPED. winbox drags a window by listening
// for mousemove, and those movements would be the cursor's ACTUAL position —
// out on the canvas, nowhere near where the window is drawn. So a passthrough
// drag swallows the real events and re-dispatches mapped ones. The synthetic
// events are ignored on the way back in by their isTrusted flag, which is
// false for anything script made.
//
// THE LISTENERS ARE ON window, NOT document, and that is not interchangeable.
// winbox attaches its drag handlers to window, which is OUTSIDE document in
// the propagation path: first in the capture phase and last in the bubble
// phase. Stopping an event at document-capture therefore does not reliably
// keep it from winbox — and the symptom was precise rather than vague. A
// purely horizontal drag moved the window diagonally, to exactly the position
// the CURSOR would have implied: 577 minus the grab offset, on both axes. Both
// events were arriving and the real one, being last, won.
//
// So: window, capture phase, and stopImmediatePropagation.

// deskPassOn is the Pass-through switch: which gesture is the cheap one.
var deskPassOn bool

// deskPassDrag is set while a passthrough drag is in flight, so the moves and
// the release that follow a press go to the same place the press did.
var deskPassDrag bool

// wantsDeskMouse decides whether this gesture belongs to the desk.
//
// The switch chooses the default and CONTROL INVERTS IT, rather than control
// meaning one fixed thing. A modifier that always meant "reach the desk" would
// be dead weight once the switch was on; inverting keeps both gestures
// available in both modes, which is the persistent-tool-and-momentary-modifier
// arrangement every drawing program uses.
func wantsDeskMouse(e js.Value) bool {
	return deskPassOn != e.Get("ctrlKey").Truthy()
}

// deskQuadHit maps a point on the screen to a point on the page inside the
// desk, or reports that the ray missed the quad.
func deskQuadHit(cx, cy float64) (px, py float64, ok bool) {
	cv := desk.Canvas()
	if !canvasEl.Truthy() || !cv.Truthy() {
		return 0, 0, false
	}
	cw, ch := cv.Get("width").Float(), cv.Get("height").Float()
	if cw <= 0 || ch <= 0 {
		return 0, 0, false
	}
	box := canvasEl.Call("getBoundingClientRect")
	vw, vh := box.Get("width").Float(), box.Get("height").Float()
	if vw <= 0 || vh <= 0 {
		return 0, 0, false
	}

	// Screen to normalized device coordinates. y is flipped because the page
	// counts down from the top and clip space counts up from the middle.
	ndcX := float32(2*(cx-box.Get("left").Float())/vw - 1)
	ndcY := float32(1 - 2*(cy-box.Get("top").Float())/vh)

	inv := projMatrix.Mul4(viewMatrix).Mul4(movMatrix).Inv()
	near := perspDivide(inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, -1, 1}))
	far := perspDivide(inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, 1, 1}))
	dir := far.Sub(near)
	if math.Abs(float64(dir.Z())) < 1e-6 {
		return 0, 0, false // the ray runs along the plane; no single crossing
	}
	t := -near.Z() / dir.Z()
	hit := near.Add(dir.Mul(t))

	hh := float64(planeHalfH)
	hw := hh * (cw / ch)
	u := (float64(hit.X()) + hw) / (2 * hw)
	v := (float64(hit.Y()) + hh) / (2 * hh)
	if u < 0 || u > 1 || v < 0 || v > 1 {
		return 0, 0, false
	}

	// The texture is uploaded with UNPACK_FLIP_Y, so v=1 is the canvas's TOP
	// row. Without this the desk would answer to the mouse upside down.
	canvasX := u * cw
	canvasY := (1 - v) * ch

	deskBox := deskEl.Call("getBoundingClientRect")
	dh := deskBox.Get("height").Float()
	if dh <= 0 {
		return 0, 0, false
	}
	dpr := ch / dh // the compositor sizes its canvas to the desk times the dpr
	return deskBox.Get("left").Float() + canvasX/dpr,
		deskBox.Get("top").Float() + canvasY/dpr, true
}

func perspDivide(v mgl32.Vec4) mgl32.Vec3 {
	if v.W() == 0 {
		return mgl32.Vec3{v.X(), v.Y(), v.Z()}
	}
	return mgl32.Vec3{v.X() / v.W(), v.Y() / v.W(), v.Z() / v.W()}
}

// sendToDesk delivers one mapped mouse event to whatever is at that point.
func sendToDesk(kind string, px, py float64, src js.Value) {
	// The windows are inert while the desk is a model — that is what stops
	// invisible windows swallowing the drag meant to turn the model — so they
	// have to be made hittable for exactly as long as it takes to find one.
	deskEl.Get("classList").Call("remove", "as-model")
	el := doc.Call("elementFromPoint", px, py)
	deskEl.Get("classList").Call("add", "as-model")
	js.Global().Set("__pick", map[string]any{"kind": kind, "px": px, "py": py,
		"el": func() string {
			if el.Truthy() {
				return el.Get("className").String()
			}
			return "(none)"
		}()})
	if !el.Truthy() {
		return
	}
	ev := js.Global().Get("MouseEvent").New(kind, map[string]any{
		"bubbles":    true,
		"cancelable": true,
		"clientX":    px,
		"clientY":    py,
		"button":     src.Get("button").Int(),
		"buttons":    src.Get("buttons").Int(),
		"detail":     src.Get("detail").Int(),
		// The modifier is DROPPED. It was the routing decision, not something
		// the desk asked for, and a control-click arriving at a window is a
		// context menu nobody wanted.
		"ctrlKey":  false,
		"shiftKey": src.Get("shiftKey").Truthy(),
		"altKey":   src.Get("altKey").Truthy(),
	})
	el.Call("dispatchEvent", ev)
}

// wireDeskPassthrough routes the mouse while the desk is drawn as a model.
func wireDeskPassthrough() {
	capture := map[string]any{"capture": true}

	handle := func(kind string) js.Func {
		return trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
			if len(a) == 0 {
				return nil
			}
			e := a[0]
			// Anything script made — including everything below — comes back
			// through here, and re-handling it would be a loop.
			if !e.Get("isTrusted").Truthy() {
				return nil
			}
			if !deskModelOn || selectedMode != "desk" {
				return nil
			}
			switch kind {
			case "mousedown":
				if !wantsDeskMouse(e) {
					return nil // the model's gesture; let the app turn it
				}
				px, py, ok := deskQuadHit(e.Get("clientX").Float(), e.Get("clientY").Float())
				if !ok {
					return nil // missed the quad; treat it as the model's
				}
				deskPassDrag = true
				e.Call("stopImmediatePropagation")
				e.Call("preventDefault")
				sendToDesk("mousedown", px, py, e)
			case "mousemove":
				if !deskPassDrag {
					return nil
				}
				e.Call("stopImmediatePropagation")
				px, py, ok := deskQuadHit(e.Get("clientX").Float(), e.Get("clientY").Float())
				if !ok {
					// Off the quad mid-drag. The event is still swallowed —
					// letting it through would start turning the model in the
					// middle of dragging a window — but nothing is delivered.
					return nil
				}
				sendToDesk("mousemove", px, py, e)
			case "mouseup":
				if !deskPassDrag {
					return nil
				}
				deskPassDrag = false
				e.Call("stopImmediatePropagation")
				if px, py, ok := deskQuadHit(e.Get("clientX").Float(), e.Get("clientY").Float()); ok {
					sendToDesk("mouseup", px, py, e)
					sendToDesk("click", px, py, e)
				}
			}
			return nil
		})
	}

	for _, kind := range []string{"mousedown", "mousemove", "mouseup"} {
		js.Global().Call("addEventListener", kind, handle(kind), capture)
	}
}
