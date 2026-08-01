//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// Model-input wiring: the document-level drag-rotate gesture (mouse + touch,
// with native image-drag / text-selection suppression while rotating), the
// canvas wheel zoom, and the wheel-on-control bindings. Extracted from Run()
// verbatim — every referenced name is package state.

// wireModelInput binds drag-rotation and wheel zoom.
func wireModelInput() {
	// isInteractiveDragTarget returns true when the mousedown/touchstart
	// target is something the user is clicking deliberately (button,
	// link, form input, the controls panel itself) — in those cases we
	// must NOT hijack the click to start a drag.
	isInteractiveDragTarget := func(target js.Value) bool {
		if !target.Truthy() {
			return false
		}
		if closest := target.Get("closest"); closest.Type() == js.TypeFunction {
			match := target.Call("closest", "a, button, input, label, select, textarea, #controls-panel, [data-no-drag]")
			if match.Truthy() {
				return true
			}
		}
		return false
	}

	// Scope Pong: dragging on the game screen drives the paddles (side by
	// pointer half), not the camera — mouse on desktop, finger(s) on touch.
	pongPointer := false

	// Event: mouse drag rotation. Bound to document (not canvasEl) so
	// rotation still works when the host page paints other elements
	// (e.g. magnetosphere.net's SVG logo) above the canvas. The target
	// filter above lets clicks on links/buttons/inputs through.
	doc.Call("addEventListener", "mousedown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		if selectedMode == "pong" {
			pongPointer = true
			pongPointerPaddle(e.Get("clientX").Float(), e.Get("clientY").Float())
			return nil
		}
		dragging = true
		beginDrag(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mousemove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if pongPointer {
			if e.Get("buttons").Float() == 0 {
				pongPointer = false
				return nil
			}
			pongPointerPaddle(e.Get("clientX").Float(), e.Get("clientY").Float())
			return nil
		}
		if !dragging {
			return nil
		}
		// Self-heal a missed mouseup (a native drag or an off-window release
		// eats it): no buttons held ⇒ the drag is over, stop rotating.
		if e.Get("buttons").Float() == 0 {
			dragging = false
			return nil
		}
		dragMove(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mouseup", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dragging = false
		pongPointer = false
		return nil
	}))
	// While rotating, kill the browser's native behaviors that hijack the
	// gesture on host pages: dragging an <img>/SVG (magnetosphere.net's logo
	// lifts "in hand" and eats every event until release) and text selection.
	for _, ev := range []string{"dragstart", "selectstart"} {
		doc.Call("addEventListener", ev, trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if dragging {
				args[0].Call("preventDefault")
			}
			return nil
		}))
	}

	// Event: touch drag rotation + two-finger pinch zoom. Same doc-binding
	// rationale as mouse. Do NOT preventDefault here unconditionally — that
	// breaks tap+drag on host-page links/buttons. Only preventDefault when
	// we're actually starting a gesture (target isn't interactive).
	pinching := false
	pinchDist := 0.0
	touchDist := func(touches js.Value) float64 {
		a, b := touches.Index(0), touches.Index(1)
		dx := a.Get("clientX").Float() - b.Get("clientX").Float()
		dy := a.Get("clientY").Float() - b.Get("clientY").Float()
		return dx*dx + dy*dy // squared is fine — only ratios of change matter
	}
	doc.Call("addEventListener", "touchstart", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		e.Call("preventDefault")
		touches := e.Get("touches")
		if selectedMode == "pong" {
			// Every finger drives the paddle on its side — two players on
			// one phone works.
			for i := 0; i < touches.Get("length").Int(); i++ {
				t := touches.Index(i)
				pongPointerPaddle(t.Get("clientX").Float(), t.Get("clientY").Float())
			}
			return nil
		}
		if touches.Get("length").Int() >= 2 {
			dragging = false
			pinching = true
			pinchDist = touchDist(touches)
			return nil
		}
		t := touches.Index(0)
		dragging = true
		beginDrag(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchmove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		touches := e.Get("touches")
		if selectedMode == "pong" && !isInteractiveDragTarget(e.Get("target")) {
			e.Call("preventDefault")
			for i := 0; i < touches.Get("length").Int(); i++ {
				t := touches.Index(i)
				pongPointerPaddle(t.Get("clientX").Float(), t.Get("clientY").Float())
			}
			return nil
		}
		if pinching && touches.Get("length").Int() >= 2 {
			e.Call("preventDefault")
			d := touchDist(touches)
			if pinchDist > 0 {
				// Spread → zoom in. Log-ratio keeps the gesture scale-free.
				applyZoomDelta(float32(-8 * (d/pinchDist - 1)))
			}
			pinchDist = d
			return nil
		}
		if !dragging {
			return nil
		}
		e.Call("preventDefault")
		t := touches.Index(0)
		dragMove(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchend", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if args[0].Get("touches").Get("length").Int() < 2 {
			pinching = false
		}
		dragging = false
		return nil
	}))

	// Event: scroll wheel zoom
	canvasEl.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		// deltaY is ~100–130 per mouse notch; keep the per-notch zoom step
		// small (~2–3) while still scaling gently on fine trackpads.
		applyZoomDelta(float32(e.Get("deltaY").Float()) * 0.02)
		return nil
	}))
}

// applyZoomDelta nudges the camera-zoom control by -delta (wheel notches
// and pinch spread both funnel here), clamped to the slider range, firing
// 'input' so the zoom knob pointer + numeric box track the gesture.
func applyZoomDelta(delta float32) {
	zoomVal := float32(js.Global().Get("parseFloat").Invoke(cameraControl.Get("value")).Float())
	zoomVal -= delta
	if zoomVal < -95 {
		zoomVal = -95
	}
	if zoomVal > 95 {
		zoomVal = 95
	}
	cameraControl.Set("value", strconv.FormatFloat(float64(zoomVal), 'f', 0, 64))
	cachedZoom = zoomVal // render loop reads the cache, not the DOM
	cameraControl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

// wireWheelBindings makes the wheel adjust controls (ranges, numerics,
// selects) instead of scrolling, and installs the param-panel rebinder
// buildParamPanel re-invokes after each rebuild.
func wireWheelBindings() {
	// Wheel-on-input: scroll over a range/number input adjusts its
	// value by `step` instead of scrolling the host page. Fires the
	// input event so the existing listener pipelines react.
	bindWheelEl := func(el js.Value) {
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			deltaY := e.Get("deltaY").Float()
			step, _ := strconv.ParseFloat(el.Get("step").String(), 64)
			if step == 0 {
				step = 1
			}
			cur, _ := strconv.ParseFloat(el.Get("value").String(), 64)
			if deltaY < 0 {
				cur += step
			} else {
				cur -= step
			}
			if minS := el.Get("min").String(); minS != "" {
				if mn, err := strconv.ParseFloat(minS, 64); err == nil && cur < mn {
					cur = mn
				}
			}
			if maxS := el.Get("max").String(); maxS != "" {
				if mx, err := strconv.ParseFloat(maxS, 64); err == nil && cur > mx {
					cur = mx
				}
			}
			el.Set("value", strconv.FormatFloat(cur, 'f', -1, 64))
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("input", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToInput := func(id string) {
		bindWheelEl(doc.Call("getElementById", id))
	}
	for _, id := range []string{
		"camera-zoom", "rotation-controls-x", "rotation-controls-y",
		"rotation-controls-z", "speed-slider", "trail-slider", "line-width",
	} {
		bindWheelToInput(id)
	}

	// Wheel-on-select: hover-scroll over a <select> cycles its
	// selectedIndex (option groups skipped automatically — they
	// aren't part of .options). Fires the change event so existing
	// listeners (mode-select onModeChange, gradient-type handler)
	// react as if the user clicked.
	bindWheelToSelect := func(id string) {
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			idx := el.Get("selectedIndex").Int()
			n := el.Get("options").Get("length").Int()
			if e.Get("deltaY").Float() > 0 {
				idx++
			} else {
				idx--
			}
			if idx < 0 {
				idx = 0
			}
			if idx >= n {
				idx = n - 1
			}
			el.Set("selectedIndex", idx)
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("change", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToSelect("cat-select")
	bindWheelToSelect("model-select")
	bindWheelToSelect("gradient-source")
	bindWheelToSelect("gradient-colors")
	// Also wheel-bind every numeric input the param panel builds.
	// Re-invoke after each buildParamPanel so newly-added inputs get
	// wired too — wrap the existing helper into a package-level
	// rebinder we can call from buildParamPanel.
	rebindParamWheel = func() {
		params := doc.Call("getElementById", "params")
		if !params.Truthy() {
			return
		}
		inputs := params.Call("querySelectorAll", "input[type=range], input[type=number]")
		for i := 0; i < inputs.Length(); i++ {
			bindWheelEl(inputs.Index(i)) // by element — the number boxes have no id
		}
	}
	rebindParamWheel()
}
