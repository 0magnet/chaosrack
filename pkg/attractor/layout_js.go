//go:build js && wasm

package attractor

import (
	_ "embed"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

var standalonePanel bool
var dockEdge = "bottom"
var dockSizeH float64 // panel height (px) when docked bottom/top
var dockSizeW = 360.0 // panel width (px) when docked left/right
var resizeHandle js.Value
var resizing bool

// panelScale mirrors the CSS --kscale (interface size). Both the Size knob and
// the bottom/top dock resize-drag drive it, so "resize the panel" scales the
// whole control interface — which works even though every module is a
// fixed-height 3-row grid (a plain height drag could only clip it).
var panelScale = 1.0

// setKScale sets the interface scale (clamped), persists it, and re-snaps the
// module widths + resize bar to the new size.
func setKScale(v float64) {
	if v < 0.6 {
		v = 0.6
	} else if v > 2.2 {
		v = 2.2
	}
	panelScale = v
	doc.Get("documentElement").Get("style").Call("setProperty", "--kscale", strconv.FormatFloat(v, 'f', 3, 64))
	quantizeModuleWidths()
	positionResizeHandle()
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-kscale", strconv.FormatFloat(v, 'f', 3, 64))
	}
}

// Float mode: a detached, draggable, resizable portrait window (phone-shaped by
// default) that can sit anywhere over the full-screen model.
var (
	floatX               = 24.0
	floatY               = 48.0
	floatW               = 384.0
	floatH               = 720.0
	floatTitle           js.Value
	floatGrip            js.Value
	floatDragging        bool
	floatResizing        bool
	floatOffX, floatOffY float64
)

func pxStr(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }

// clampFloatPos keeps the float's title bar reachable within the current
// viewport. Lives inside positionFloat so EVERY path that moves the float —
// dragging, a persisted position restored into a smaller window, a window
// resize — goes through the one clamp; a stale localStorage position can
// never strand the panel off-screen.
func clampFloatPos() {
	if floatX < 8-floatW+120 {
		floatX = 8 - floatW + 120
	}
	if floatX > winW()-40 {
		floatX = winW() - 40
	}
	if floatY < 0 {
		floatY = 0
	}
	if floatY > winH()-30 {
		floatY = winH() - 30
	}
}

// positionFloat writes the floating panel's geometry (and its resize grip)
// without rebuilding the whole style or touching localStorage — cheap enough
// to call on every pointermove.
func positionFloat() {
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	clampFloatPos()
	st := p.Get("style")
	st.Set("left", pxStr(floatX))
	st.Set("top", pxStr(floatY))
	st.Set("width", pxStr(floatW))
	st.Set("height", pxStr(floatH))
	if floatGrip.Truthy() {
		gs := floatGrip.Get("style")
		gs.Set("left", pxStr(floatX+floatW-16))
		gs.Set("top", pxStr(floatY+floatH-16))
	}
}

func saveFloatGeom() {
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-floatX", pxStr(floatX))
		ls.Call("setItem", "wasmstuff-floatY", pxStr(floatY))
		ls.Call("setItem", "wasmstuff-floatW", pxStr(floatW))
		ls.Call("setItem", "wasmstuff-floatH", pxStr(floatH))
	}
}

func winH() float64 {
	if v := js.Global().Get("innerHeight").Float(); v > 0 {
		return v
	}
	return 800
}
func winW() float64 {
	if v := js.Global().Get("innerWidth").Float(); v > 0 {
		return v
	}
	return 1200
}

// applyDock positions the standalone controls panel against a window edge:
// bottom/top become a horizontal strip (height dockSizeH), left/right a
// vertical sidebar (width dockSizeW; the flex rows wrap into a narrow
// scrollable column). The edge + sizes persist in localStorage. No-op when
// the panel lives inside a host footer.
func applyDock(edge string) {
	if !standalonePanel {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	if dockSizeH <= 0 {
		// Default tall enough to show a full module row (~545px at scale 1)
		// without clipping, capped so it never eats the whole viewport.
		dockSizeH = math.Min(560, winH()*0.9)
	}
	// NB: no z-index here — the panel's base z lives in CSS (#controls-panel →
	// var(--z-panel)) so that clearing an inline override falls back to a value
	// above the canvas, never auto(0). Inline z is only ever set as a temporary
	// recovery-raise (see panelToggle / the Front handler).
	const base = "background:rgba(0,0,0,0.85);padding:8px 12px;font-family:'B612 Mono',monospace;" +
		"font-size:12px;color:#aaa;pointer-events:auto;position:fixed;box-sizing:border-box;" +
		"-webkit-overflow-scrolling:touch;overscroll-behavior:contain;touch-action:pan-x pan-y;"
	hpx := strconv.FormatFloat(dockSizeH, 'f', 0, 64) + "px"
	wpx := strconv.FormatFloat(dockSizeW, 'f', 0, 64) + "px"
	var edgeCSS string
	float := false
	switch edge {
	case "top":
		edgeCSS = "left:0;right:0;top:0;max-height:" + hpx + ";overflow-y:auto;border-bottom:1px solid #333;"
	case "left":
		edgeCSS = "top:0;bottom:0;left:0;width:" + wpx + ";overflow-y:auto;border-right:1px solid #333;"
	case "right":
		edgeCSS = "top:0;bottom:0;right:0;width:" + wpx + ";overflow-y:auto;border-left:1px solid #333;"
	case "float":
		float = true
		// portrait window; geometry (left/top/width/height) set by positionFloat.
		edgeCSS = "overflow-y:auto;border:1px solid #2a3a4a;border-radius:8px;box-shadow:0 10px 34px rgba(0,0,0,0.6);"
	default:
		edge = "bottom"
		edgeCSS = "left:0;right:0;bottom:0;max-height:" + hpx + ";overflow-y:auto;border-top:1px solid #333;"
	}
	p.Get("style").Set("cssText", base+edgeCSS)
	dockEdge = edge
	// (The legacy "rack" horizontal-strip layout is superseded by the module
	// system, which lays out the same in every dock mode.)
	p.Get("classList").Call("remove", "rack")
	// Tag the panel with its edge so CSS can adapt (e.g. sidebars stack modules
	// in a single column rather than flowing them side-by-side).
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		p.Get("classList").Call("remove", "dk-"+e)
	}
	p.Get("classList").Call("add", "dk-"+edge)
	// The title bar (drag handle + label) shows ONLY when floating; docked modes
	// have no top chrome — the dock controls clip onto the resize bar instead.
	if floatTitle.Truthy() {
		if float {
			floatTitle.Get("classList").Call("add", "floating")
			floatTitle.Get("style").Set("display", "")
		} else {
			floatTitle.Get("classList").Call("remove", "floating")
			floatTitle.Get("style").Set("display", "none")
		}
	}
	if floatGrip.Truthy() {
		fdisp := "none"
		if float {
			fdisp = ""
		}
		floatGrip.Get("style").Set("display", fdisp)
	}
	if float {
		positionFloat()
	}
	positionResizeHandle()
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			if e == edge {
				b.Get("classList").Call("add", "active")
			} else {
				b.Get("classList").Call("remove", "active")
			}
		}
	}
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-dock", edge)
		ls.Call("setItem", "wasmstuff-dockH", strconv.FormatFloat(dockSizeH, 'f', 0, 64))
		ls.Call("setItem", "wasmstuff-dockW", strconv.FormatFloat(dockSizeW, 'f', 0, 64))
	}
	quantizeModuleWidths()
}

// moduleSlot is the base card-slot width. Like rack cards plugged into a
// uniformly-spaced backplane, every module occupies an INTEGER number of slots
// (its content width rounded up), so modules always butt up cleanly.
const moduleSlot = 132.0

// moduleGap matches the .modules flex gap (px). Multi-slot modules add (N-1)
// of these so their right edge lines up with N separate 1-slot modules.
const moduleGap = 4.0

// quantizeModuleWidths snaps every module's width to a whole number of slots.
func quantizeModuleWidths() {
	if !standalonePanel {
		return
	}
	mods := doc.Call("querySelectorAll", ".modules > .sect")
	for i := 0; i < mods.Get("length").Int(); i++ {
		m := mods.Index(i)
		st := m.Get("style")
		// Widen the module so its column-wrap content forms its natural,
		// height-driven columns with nothing clipped, then measure the real
		// content span from the item positions (flex column-wrap misreports both
		// max-content and scrollWidth).
		st.Set("width", "3000px")
		// The actual controls grid — NOT #params (a display:contents .row whose
		// only child is the full-width grid, which would misreport 3000px).
		content := m.Call("querySelector", ".punit-grid, .toprow, .row:not(#params)")
		w := 0.0
		if content.Truthy() {
			items := content.Get("children")
			minL, maxR, any := 1e9, -1e9, false
			for j := 0; j < items.Get("length").Int(); j++ {
				it := items.Index(j)
				// Skip out-of-flow items (e.g. the dock-controls cluster, which is
				// position:fixed) so they don't inflate the module's measured width.
				if js.Global().Get("getComputedStyle").Invoke(it).Get("position").String() == "fixed" {
					continue
				}
				r := it.Call("getBoundingClientRect")
				l, rr := r.Get("left").Float(), r.Get("right").Float()
				if rr-l <= 0 {
					continue // skip hidden/zero-size items
				}
				any = true
				if l < minL {
					minL = l
				}
				if rr > maxR {
					maxR = rr
				}
			}
			if any {
				w = maxR - minL
			}
		}
		slot := moduleSlot * panelScale // slot grows with the interface scale
		slots := int(math.Ceil((w + 13) / slot))
		if slots < 1 {
			slots = 1
		}
		// An N-slot module also spans the (N-1) inter-module gaps it covers, so its
		// right edge lines up with N separate 1-slot modules — corners align across
		// wrapped rows. (moduleGap matches the .modules flex gap.)
		width := float64(slots)*slot + float64(slots-1)*moduleGap
		st.Set("width", strconv.FormatFloat(width, 'f', 0, 64)+"px")
	}
}

// initFloatWindow builds the floating panel's drag title-bar and corner resize
// grip (once), with document-level move/up handlers so a drag continues off the
// small targets. Hidden until float mode is active (applyDock toggles them).
func initFloatWindow() {
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	// Float mode shows a title bar (drag handle + label). The dock controls no
	// longer live on it — they're a body-level fixed cluster that clips onto the
	// end of the resize bar in dock modes (positionDockControls), so no chrome
	// bar steals panel space when docked.
	floatTitle = doc.Call("createElement", "div")
	floatTitle.Set("id", "float-title")
	floatTitle.Set("className", "float-title")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", "float-title-lbl")
	lbl.Set("textContent", "◈ WASM-STUFF")
	floatTitle.Call("appendChild", lbl)
	if dc := doc.Call("getElementById", "dock-controls"); dc.Truthy() {
		body.Call("appendChild", dc) // detach: positioned onto the resize bar's end
	}
	// (power switch stays inside the Console module — not moved to the top bar)
	p.Call("insertBefore", floatTitle, p.Get("firstChild"))

	floatGrip = doc.Call("createElement", "div")
	floatGrip.Set("id", "float-grip")
	floatGrip.Set("title", "Drag to resize")
	floatGrip.Set("style", "display:none;position:fixed;z-index:var(--z-grip);width:22px;height:22px;cursor:nwse-resize;touch-action:none;"+
		"background:linear-gradient(135deg,transparent 40%,rgba(140,170,200,0.7) 40%,rgba(140,170,200,0.7) 55%,transparent 55%,transparent 68%,rgba(140,170,200,0.7) 68%,rgba(140,170,200,0.7) 83%,transparent 83%);")
	body.Call("appendChild", floatGrip)

	floatTitle.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		// Only drag when floating, and not when pressing a dock button.
		if dockEdge != "float" {
			return nil
		}
		if t := e.Get("target"); t.Truthy() && t.Call("closest", "button,select,input").Truthy() {
			return nil
		}
		e.Call("preventDefault")
		floatDragging = true
		floatOffX = e.Get("clientX").Float() - floatX
		floatOffY = e.Get("clientY").Float() - floatY
		return nil
	}))
	floatGrip.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		floatResizing = true
		return nil
	}))
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if !floatDragging && !floatResizing {
			return nil
		}
		e := a[0]
		if floatDragging {
			floatX = e.Get("clientX").Float() - floatOffX
			floatY = e.Get("clientY").Float() - floatOffY
			// (positionFloat clamps — the one clamp for every move path)
		}
		if floatResizing {
			floatW = e.Get("clientX").Float() - floatX
			floatH = e.Get("clientY").Float() - floatY
			if floatW < 240 {
				floatW = 240
			}
			if floatH < 200 {
				floatH = 200
			}
		}
		positionFloat()
		return nil
	}))
	up := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if floatDragging || floatResizing {
			saveFloatGeom()
		}
		floatDragging, floatResizing = false, false
		return nil
	})
	doc.Call("addEventListener", "pointerup", up)
	doc.Call("addEventListener", "pointercancel", up)
}

// positionResizeHandle places the drag bar along the panel's inner edge (the
// one facing the page center) so dragging it resizes the panel.
func positionResizeHandle() {
	positionDockControls()
	if !resizeHandle.Truthy() {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	// Hidden panel (panel-toggle) or floating (its own corner grip): hide the
	// edge bar and let the meter return to the corner.
	if p.Get("style").Get("display").String() == "none" || dockEdge == "float" {
		resizeHandle.Get("style").Set("display", "none")
		positionAudioMeters()
		positionInfoOverlay()
		return
	}
	resizeHandle.Get("style").Set("display", "")
	// Track the panel's actual rendered inner edge (the side facing the page
	// center) so the bar stays on the edge regardless of content height.
	r := p.Call("getBoundingClientRect")
	px := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }
	// Thicker bar (12px) with touch-action:none so touch-drags resize instead of
	// scrolling; a lighter center stripe reads as a grip.
	const common = "position:fixed;z-index:var(--z-dock);touch-action:none;background:" +
		"linear-gradient(rgba(120,150,180,0.12),rgba(150,180,210,0.55),rgba(120,150,180,0.12));"
	const commonV = "position:fixed;z-index:var(--z-dock);touch-action:none;background:" +
		"linear-gradient(to right,rgba(120,150,180,0.12),rgba(150,180,210,0.55),rgba(120,150,180,0.12));"
	var s string
	switch dockEdge {
	case "top":
		s = common + "left:0;right:0;top:" + px(r.Get("bottom").Float()-6) + ";height:12px;cursor:ns-resize;"
	case "left":
		s = commonV + "top:0;bottom:0;left:" + px(r.Get("right").Float()-6) + ";width:12px;cursor:ew-resize;"
	case "right":
		s = commonV + "top:0;bottom:0;left:" + px(r.Get("left").Float()-6) + ";width:12px;cursor:ew-resize;"
	default:
		s = common + "left:0;right:0;top:" + px(r.Get("top").Float()-6) + ";height:12px;cursor:ns-resize;"
	}
	resizeHandle.Get("style").Set("cssText", s)
	positionAudioMeters()
	positionInfoOverlay()
}

// positionDockControls clips the dock/float button cluster onto the END of the
// resize bar, protruding into the canvas — so no top chrome bar is needed to
// hold them when docked. Horizontal row for top/bottom docks, vertical column
// for left/right sidebars, and over the title bar's right end when floating.
func positionDockControls() {
	dc := doc.Call("getElementById", "dock-controls")
	if !dc.Truthy() {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() || p.Get("style").Get("display").String() == "none" {
		dc.Get("style").Set("display", "none")
		return
	}
	r := p.Call("getBoundingClientRect")
	px := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }
	const chrome = "display:flex;align-items:center;position:fixed;z-index:var(--z-grip);gap:4px;" +
		"background:linear-gradient(#18242f,#0c1218);border:1px solid #2a3a4a;" +
		"padding:2px 5px;border-radius:5px;box-shadow:0 2px 8px rgba(0,0,0,0.5);"
	col := "flex-direction:row;"
	dc.Get("classList").Call("remove", "dc-vert")
	var pos string
	switch dockEdge {
	case "float":
		// Over the title bar, at its right end (label stays visible on the left).
		pos = "top:" + px(r.Get("top").Float()+4) + ";right:" + px(winW()-r.Get("right").Float()+8) + ";"
	case "top":
		// Bar runs along the panel's bottom edge; hang the cluster below it.
		pos = "top:" + px(r.Get("bottom").Float()+2) + ";right:" + px(winW()-r.Get("right").Float()+16) + ";"
	case "left":
		// Bar runs down the panel's right edge; stack the cluster to its right.
		col = "flex-direction:column;"
		dc.Get("classList").Call("add", "dc-vert")
		pos = "left:" + px(r.Get("right").Float()+2) + ";top:" + px(r.Get("top").Float()+16) + ";"
	case "right":
		// Bar runs down the panel's left edge; stack the cluster to its left.
		col = "flex-direction:column;"
		dc.Get("classList").Call("add", "dc-vert")
		pos = "right:" + px(winW()-r.Get("left").Float()+2) + ";top:" + px(r.Get("top").Float()+16) + ";"
	default: // bottom
		// Bar runs along the panel's top edge; perch the cluster above it.
		pos = "bottom:" + px(winH()-r.Get("top").Float()+2) + ";right:" + px(winW()-r.Get("right").Float()+16) + ";"
	}
	dc.Get("style").Set("cssText", chrome+col+pos)
}

// positionAudioMeters keeps the top-left audio-feature meter overlay clear of
// the control panel: it shifts right of a left sidebar or below a top strip,
// and returns to the corner for bottom/right docks or when the panel is hidden.
func positionAudioMeters() {
	if !afOverlay.Truthy() {
		return
	}
	top, left := 8.0, 8.0
	if standalonePanel {
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() && p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch dockEdge {
			case "left":
				left = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "top":
				top = r.Get("bottom").Float() + 8
			}
		}
	}
	st := afOverlay.Get("style")
	st.Set("top", strconv.FormatFloat(top, 'f', 0, 64)+"px")
	st.Set("left", strconv.FormatFloat(left, 'f', 0, 64)+"px")
}

// positionInfoOverlay keeps the Info text clear of the control panel (offset
// past a left/top sidebar, or below a bottom dock's top edge).
func positionInfoOverlay() {
	ov := doc.Call("getElementById", "info-overlay")
	if !ov.Truthy() || ov.Get("style").Get("display").String() == "none" {
		return
	}
	st := ov.Get("style")
	top, left, right := 70.0, 20.0, 20.0
	if standalonePanel {
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() && p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch dockEdge {
			case "left":
				left = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "right":
				right = winW() - r.Get("left").Float() + 48 // clear the vertical dock-controls tab
			case "top":
				top = r.Get("bottom").Float() + 20
			case "bottom":
				// panel at the bottom; keep the overlay in the upper area
			}
		}
	}
	st.Set("top", strconv.FormatFloat(top, 'f', 0, 64)+"px")
	st.Set("left", strconv.FormatFloat(left, 'f', 0, 64)+"px")
	st.Set("right", strconv.FormatFloat(right, 'f', 0, 64)+"px")
}

// initDockResize creates the resize bar and its drag handlers (document-level
// so the drag continues past the thin bar).
func initDockResize() {
	resizeHandle = doc.Call("createElement", "div")
	resizeHandle.Set("id", "dock-resize")
	resizeHandle.Set("title", "Drag to resize the control panel")
	body.Call("appendChild", resizeHandle)
	resizeHandle.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		resizing = true
		return nil
	}))
	// The "DOCK" label doubles as a resize grip (a bigger, obvious touch target
	// than the thin bar). Dragging it resizes exactly like the bar.
	if dl := doc.Call("querySelector", "#dock-controls .dock-lbl"); dl.Truthy() {
		dl.Get("style").Set("cursor", "grab")
		dl.Get("style").Set("touchAction", "none")
		dl.Set("title", "Drag to resize the control panel")
		dl.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			a[0].Call("stopPropagation")
			resizing = true
			return nil
		}))
	}
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if !resizing {
			return nil
		}
		e := a[0]
		switch dockEdge {
		case "bottom":
			dockSizeH = winH() - e.Get("clientY").Float()
		case "top":
			dockSizeH = e.Get("clientY").Float()
		case "left":
			dockSizeW = e.Get("clientX").Float()
		case "right":
			dockSizeW = winW() - e.Get("clientX").Float()
		}
		if dockSizeH < 120 {
			dockSizeH = 120
		} else if dockSizeH > winH()*0.96 {
			dockSizeH = winH() * 0.96
		}
		if dockSizeW < 150 {
			dockSizeW = 150
		} else if dockSizeW > winW()*0.95 {
			dockSizeW = winW() * 0.95
		}
		applyDock(dockEdge)
		return nil
	}))
	doc.Call("addEventListener", "pointerup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		resizing = false
		return nil
	}))
	// Keep the bar on the panel's edge as its content height changes
	// (audio-mod rows, section collapse, mode switches, window resize).
	if ro := js.Global().Get("ResizeObserver"); ro.Truthy() {
		obs := ro.New(trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			positionResizeHandle()
			return nil
		}))
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() {
			obs.Call("observe", p)
		}
	}
	js.Global().Call("addEventListener", "resize", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if dockEdge == "float" {
			positionFloat() // re-clamp into the new viewport
		}
		positionResizeHandle()
		quantizeModuleWidths()
		return nil
	}))
}

func wireDockButtons() {
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		edge := e
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				applyDock(edge)
				return nil
			}))
		}
	}
}

func readDockPref() string {
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		if v := ls.Call("getItem", "wasmstuff-dockH"); v.Truthy() {
			if n, err := strconv.ParseFloat(v.String(), 64); err == nil && n > 0 {
				dockSizeH = n
			}
		}
		if v := ls.Call("getItem", "wasmstuff-dockW"); v.Truthy() {
			if n, err := strconv.ParseFloat(v.String(), 64); err == nil && n > 0 {
				dockSizeW = n
			}
		}
		for key, dst := range map[string]*float64{
			"wasmstuff-floatX": &floatX, "wasmstuff-floatY": &floatY,
			"wasmstuff-floatW": &floatW, "wasmstuff-floatH": &floatH,
		} {
			if v := ls.Call("getItem", key); v.Truthy() {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(v.String(), "px"), 64); err == nil && n > 0 {
					*dst = n
				}
			}
		}
		if v := ls.Call("getItem", "wasmstuff-dock"); v.Truthy() {
			return v.String()
		}
	}
	return "bottom"
}

// updateGradientUI shows only the color controls relevant to the current
// palette: monochrome → one color; two-color → start+end; three-color →
// start+mid+end; rainbow → no fixed colors, show the period knob instead.
