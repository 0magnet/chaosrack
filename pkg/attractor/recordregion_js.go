//go:build js && wasm

package attractor

// Choosing the part of the canvas to record, by dragging on it.
//
// Two elements do this. A capture layer sits over the canvas while the Region
// switch is on and takes the pointer events, so a drag draws a rectangle
// instead of rotating the model; and an outline that stays visible afterwards
// so the chosen area is not something you have to remember.
//
// The capture layer is why the switch is a mode rather than a one-shot "drag
// now" prompt. The canvas already uses drag for the camera, and there is no
// gesture free to mean "select" instead — so the choice is between stealing
// drag for as long as the switch is on, which is visible and reversible, and
// inventing a modifier chord, which is neither.
//
// The layer is taken away as soon as a selection is made, so the camera comes
// back immediately and the outline stays to show what was chosen. Switching
// back to "full" is what clears the region — stopRegionSelect zeroes it and
// hides the outline — and that is the only thing that does, which is why Reset
// All resets this switch too.
//
// The region is stored in CANVAS pixels, not CSS pixels. The two differ by the
// device pixel ratio and by whatever size the canvas is being displayed at, and
// the recorder crops the backing store — a region in the wrong units records
// the wrong part of the picture, and on a HiDPI screen it would be half of it.

import "syscall/js"

var (
	regionOn      bool
	regionLayer   js.Value // takes pointer events while selecting
	regionOutline js.Value // shows the chosen area
	regionDrag    bool
	regionX0      float64
	regionY0      float64
)

// wireRegionSwitch turns selection mode on and off.
func wireRegionSwitch() {
	sw := doc.Call("getElementById", "rec-region-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, _ []js.Value) interface{} {
		if this.Get("checked").Bool() {
			startRegionSelect()
		} else {
			stopRegionSelect()
		}
		return nil
	}))
	// The outline is placed from the stored region, so it has to be re-placed
	// whenever the canvas moves or changes size under it.
	js.Global().Call("addEventListener", "resize", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		placeRegionOutline()
		placeRegionLayer()
		return nil
	}))
}

// canvasScale is how many canvas pixels there are per CSS pixel. It is not the
// device pixel ratio: the canvas may also be displayed at a size other than its
// backing store, and only the rendered rectangle knows the whole story.
func canvasScale(canvas js.Value) (sx, sy float64, rect js.Value) {
	rect = canvas.Call("getBoundingClientRect")
	w := rect.Get("width").Float()
	h := rect.Get("height").Float()
	if w <= 0 || h <= 0 {
		return 1, 1, rect
	}
	return canvas.Get("width").Float() / w, canvas.Get("height").Float() / h, rect
}

func startRegionSelect() {
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	regionOn = true
	ensureRegionLayer()
	placeRegionLayer()
	regionLayer.Get("style").Set("display", "block")
	placeRegionOutline()
}

// stopRegionSelect ends region recording: the switch off means the whole
// canvas, so the region goes with it.
//
// The switch means "record a region", not "I am selecting one" — those come
// apart the moment a drag ends, which is why the capture layer is dropped then
// (see disarmRegionLayer) while the switch stays on and the outline stays up.
// Leaving the region set after the switch was turned off would mean a later
// recording silently came out cropped to something no longer on screen.
func stopRegionSelect() {
	regionOn = false
	regionDrag = false
	disarmRegionLayer()
	recRegion.x, recRegion.y, recRegion.w, recRegion.h = 0, 0, 0, 0
	if regionOutline.Truthy() {
		regionOutline.Get("style").Set("display", "none")
	}
}

// disarmRegionLayer takes the capture layer away, giving the canvas its drag
// back. Called as soon as a selection is made: holding the pointer hostage for
// as long as the region is set would mean choosing between seeing the region
// and moving the model.
func disarmRegionLayer() {
	if regionLayer.Truthy() {
		regionLayer.Get("style").Set("display", "none")
	}
}

// ensureRegionLayer builds the capture layer and the outline once.
func ensureRegionLayer() {
	if !regionLayer.Truthy() {
		regionLayer = doc.Call("createElement", "div")
		regionLayer.Set("id", "rec-region-layer")
		regionLayer.Get("style").Set("cssText",
			"position:fixed;z-index:var(--z-grip);cursor:crosshair;display:none;"+
				"touch-action:none;background:rgba(0,0,0,0.12);")
		body.Call("appendChild", regionLayer)
		wireRegionDrag()
	}
	if !regionOutline.Truthy() {
		regionOutline = doc.Call("createElement", "div")
		regionOutline.Set("id", "rec-region-outline")
		regionOutline.Get("style").Set("cssText",
			"position:fixed;z-index:var(--z-grip);pointer-events:none;display:none;"+
				"border:1px dashed #6cf;box-shadow:0 0 0 9999px rgba(0,0,0,0.35);")
		body.Call("appendChild", regionOutline)
	}
}

// placeRegionLayer lays the capture layer exactly over the canvas.
func placeRegionLayer() {
	if !regionLayer.Truthy() {
		return
	}
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	r := canvas.Call("getBoundingClientRect")
	st := regionLayer.Get("style")
	st.Set("left", pxStr(r.Get("left").Float()))
	st.Set("top", pxStr(r.Get("top").Float()))
	st.Set("width", pxStr(r.Get("width").Float()))
	st.Set("height", pxStr(r.Get("height").Float()))
}

// placeRegionOutline draws the stored region, converting back from canvas
// pixels to where it appears on screen.
func placeRegionOutline() {
	if !regionOutline.Truthy() {
		return
	}
	if recRegion.w <= 1 || recRegion.h <= 1 {
		regionOutline.Get("style").Set("display", "none")
		return
	}
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	sx, sy, r := canvasScale(canvas)
	st := regionOutline.Get("style")
	st.Set("display", "block")
	st.Set("left", pxStr(r.Get("left").Float()+recRegion.x/sx))
	st.Set("top", pxStr(r.Get("top").Float()+recRegion.y/sy))
	st.Set("width", pxStr(recRegion.w/sx))
	st.Set("height", pxStr(recRegion.h/sy))
}

func wireRegionDrag() {
	regionLayer.Call("addEventListener", "pointerdown", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) == 0 || !regionOn {
			return nil
		}
		e := a[0]
		e.Call("preventDefault")
		regionDrag = true
		regionX0, regionY0 = e.Get("clientX").Float(), e.Get("clientY").Float()
		// Captured on the layer so a drag that leaves the canvas still steers,
		// and so the release is heard wherever it happens.
		if regionLayer.Get("setPointerCapture").Type() == js.TypeFunction {
			regionLayer.Call("setPointerCapture", e.Get("pointerId"))
		}
		setRegionFrom(regionX0, regionY0, regionX0, regionY0)
		return nil
	}))
	regionLayer.Call("addEventListener", "pointermove", trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
		if !regionDrag || len(a) == 0 {
			return nil
		}
		e := a[0]
		setRegionFrom(regionX0, regionY0, e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	for _, ev := range []string{"pointerup", "pointercancel"} {
		regionLayer.Call("addEventListener", ev, trackedFuncOf(func(_ js.Value, a []js.Value) interface{} {
			if !regionDrag {
				return nil
			}
			regionDrag = false
			if len(a) > 0 && regionLayer.Get("releasePointerCapture").Type() == js.TypeFunction {
				id := a[0].Get("pointerId")
				if regionLayer.Call("hasPointerCapture", id).Bool() {
					regionLayer.Call("releasePointerCapture", id)
				}
			}
			// A click with no drag means the whole canvas, and leaves the layer
			// armed so the next drag still selects — the alternative is that a
			// misclick disarms and the region has to be re-armed to try again.
			if recRegion.w <= 4 || recRegion.h <= 4 {
				recRegion.x, recRegion.y, recRegion.w, recRegion.h = 0, 0, 0, 0
				placeRegionOutline()
				return nil
			}
			// A real selection: show it and hand the pointer back to the canvas.
			// Re-selecting is turning the switch off and on again, which also
			// says plainly that the old region is gone.
			placeRegionOutline()
			disarmRegionLayer()
			return nil
		}))
	}
}

// setRegionFrom converts a drag in client coordinates into the canvas-pixel
// region, clamped to the canvas, and redraws the outline.
func setRegionFrom(x0, y0, x1, y1 float64) {
	canvas := modelCanvas()
	if !canvas.Truthy() {
		return
	}
	sx, sy, r := canvasScale(canvas)
	left, top := r.Get("left").Float(), r.Get("top").Float()

	// Normalized, so dragging up or left works the same as down or right.
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	cx0 := (x0 - left) * sx
	cy0 := (y0 - top) * sy
	cx1 := (x1 - left) * sx
	cy1 := (y1 - top) * sy

	cw := canvas.Get("width").Float()
	ch := canvas.Get("height").Float()
	cx0 = clampPx(cx0, cw)
	cx1 = clampPx(cx1, cw)
	cy0 = clampPx(cy0, ch)
	cy1 = clampPx(cy1, ch)

	recRegion.x, recRegion.y = cx0, cy0
	recRegion.w, recRegion.h = cx1-cx0, cy1-cy0
	placeRegionOutline()
}

func clampPx(v, hi float64) float64 {
	const lo = 0
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
