//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"
)

// Living under a host page that has its own content.
//
// The attractor started as the whole page and grew into a backdrop behind one.
// Three of its habits are correct for the first case and wrong for the second,
// and all three were found on the same host in the same afternoon: it locked the
// document's scroll, it sized its drawing buffer from the body rather than the
// window, and it claimed the wheel everywhere. A page that is a catalog with a
// model behind it needs to scroll, needs a window-sized buffer, and needs the
// wheel to mean "scroll" over the catalog and "zoom" only over the model.
//
// The knobs here are all off by default, so a host that IS the whole page keeps
// every one of the old behaviors unchanged.

// LockHostScroll injects html,body{overflow:hidden} so the controls panel cannot
// grow the document and leave a sliver of scroll behind it.
//
// True is right when the attractor IS the page. Set it false on a host whose
// document is meant to scroll: the rule is !important and beats the host's own
// stylesheet, and a host that scrolls will simply stop scrolling once the wasm
// boots — which reads as the page freezing a second after it loads, not as a
// style being applied.
var LockHostScroll = true

// ZoomTargetSelector names the elements over which the wheel zooms the camera.
//
// Empty (the default) keeps the original binding: the wheel is claimed on the
// canvas element itself and zooms wherever that element receives the event.
// That is unusable on a host that puts pointer-events:none on the canvas so its
// own links stay clickable, because then the canvas receives no wheel event at
// all and the model can never be zoomed.
//
// When set, the wheel is watched on the document instead and zooms only when the
// event's target is inside a match — everything else keeps native scrolling.
// A host names the model's visible stand-in, e.g. "#logo, #gocanvas", rather
// than the whole page: a rule that zoomed over any bare background would turn
// the wheel into a zoom in every margin of a scrolling document, which is worse
// than not zooming at all.
var ZoomTargetSelector string

// CenterOnSelector centers the model on a host element instead of on the window.
//
// The model is drawn at the middle of the canvas, and the canvas is the window,
// so the model sits at the middle of the window. When the host has put a picture
// over it — a logo the model is supposed to be behind — the two only coincide if
// that picture happens to be centered in the window too, and a page with a
// header, a footer and text around the logo does not put it there. Measured on
// magnetosphere.net: the model at (1182, 610), the logo it stands behind at
// (1192, 544), so 66px of daylight above the model and 10px to the side.
//
// Rather than move the picture, which is in the layout for a reason, this moves
// the backdrop: the canvas container is translated by the offset between the
// window's center and the element's, and re-translated whenever the layout
// could have changed. Nothing about the drawing changes, so the model's size,
// projection and hit-testing are all untouched — only where the canvas sits.
var CenterOnSelector string

// hostOwnsWheel reports whether a wheel event belongs to the host page.
//
// The mousedown filter's list is reused deliberately: an element that must not
// have its click turned into a drag must not have its wheel turned into a zoom
// either, and keeping one list means the two gestures cannot drift apart. The
// panel is on it, which matters — its own knobs bind wheel handlers of their
// own and stopPropagation, but a knob the pointer merely passes over must not
// zoom the camera.
func hostOwnsWheel(target js.Value) bool {
	if !target.Truthy() {
		return false
	}
	if closest := target.Get("closest"); closest.Type() != js.TypeFunction {
		return false
	}
	return target.Call("closest",
		"a, button, input, label, select, textarea, #controls-panel, [data-no-drag]").Truthy()
}

// wireHostWheel installs the document-level wheel zoom for ZoomTargetSelector.
// Returns false when no selector is set, so the caller keeps the canvas binding.
func wireHostWheel() bool {
	if ZoomTargetSelector == "" {
		return false
	}
	doc.Call("addEventListener", "wheel", trackedFuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		e := args[0]
		t := e.Get("target")
		if hostOwnsWheel(t) {
			return nil
		}
		if closest := t.Get("closest"); closest.Type() != js.TypeFunction {
			return nil
		}
		if !t.Call("closest", ZoomTargetSelector).Truthy() {
			return nil // host content: let the page scroll
		}
		// Only now is this ours, and only now may the scroll be swallowed.
		e.Call("preventDefault")
		applyZoomDelta(float32(e.Get("deltaY").Float()) * 0.02)
		return nil
	}), map[string]any{"passive": false})
	return true
}

// backdropCenterOffset is the translation that puts the canvas center on the target
// element's center, in CSS pixels. Zero when there is no target.
func backdropCenterOffset() (dx, dy float64) {
	if CenterOnSelector == "" {
		return 0, 0
	}
	el := doc.Call("querySelector", CenterOnSelector)
	if !el.Truthy() {
		return 0, 0
	}
	r := el.Call("getBoundingClientRect")
	w, h := r.Get("width").Float(), r.Get("height").Float()
	if w <= 0 || h <= 0 {
		return 0, 0 // not laid out (or display:none): leave the backdrop alone
	}
	tx := r.Get("left").Float() + w/2
	ty := r.Get("top").Float() + h/2
	vw := doc.Get("documentElement").Get("clientWidth").Float()
	vh := doc.Get("documentElement").Get("clientHeight").Float()
	return tx - vw/2, ty - vh/2
}

// applyCenterOn translates the canvas container so the model lands on the
// target. A no-op unless CenterOnSelector is set.
//
// The translate goes on the container rather than the canvas because the canvas
// has its width and height rewritten on every resize and a transform there
// would be one more thing to keep re-applying. Nothing else in the package
// writes the container's transform, so this can own it outright.
func applyCenterOn() {
	if CenterOnSelector == "" {
		return
	}
	cont := doc.Call("getElementById", "gocanvas-container")
	if !cont.Truthy() {
		return
	}
	dx, dy := backdropCenterOffset()
	st := cont.Get("style")
	if dx == 0 && dy == 0 {
		st.Call("removeProperty", "transform")
		return
	}
	st.Set("transform", "translate("+ftoa(dx)+"px,"+ftoa(dy)+"px)")
}

// ftoa formats a CSS pixel offset. Whole pixels: the offset comes from a layout
// measurement whose sub-pixel part is noise, and a stable string keeps the
// browser from re-compositing the backdrop on every scroll.
func ftoa(v float64) string {
	return strconv.Itoa(int(math.Floor(v)))
}

// initHostPage wires the host-page accommodations that need a listener. Called
// late in Run, after the canvas and the panel exist.
func initHostPage() {
	if CenterOnSelector == "" {
		return
	}
	applyCenterOn()
	// The offset is a layout measurement, so anything that reflows invalidates
	// it. resize covers the window changing; the :target views this host uses
	// for navigation change the page's height without a resize, and hashchange
	// is what those are.
	for _, ev := range []string{"resize", "hashchange", "load"} {
		js.Global().Call("addEventListener", ev, trackedFuncOf(func(js.Value, []js.Value) interface{} {
			applyCenterOn()
			return nil
		}))
	}
}
