//go:build js && wasm

package attractor

import (
	_ "embed"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/rackspec"
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
// setKScale sets the interface size as a USER edit.
func setKScale(v float64) { setKScaleFrom(v, false) }

// setKScaleFrom sets the interface size, saying where the change came from.
//
// The origin matters because the rack bay shrinks the size to fit a whole
// 84 HP row and puts it back when it is switched off. A size change from
// anywhere else is the user's: from then on the current size is the one they
// asked for, and the bay must not put an older one back over it.
func setKScaleFrom(v float64, fromBay bool) {
	if !fromBay {
		forgetScaleBeforeBay()
	}
	if v < 0.6 {
		v = 0.6
	} else if v > 2.2 {
		v = 2.2
	}
	panelScale = v
	doc.Get("documentElement").Get("style").Call("setProperty", "--kscale", strconv.FormatFloat(v, 'f', 3, 64))
	// --kscale drives the CSS; the rack needs the same number told to it,
	// because the slot pitch it snaps modules to scales with the interface.
	// rackSetScale re-quantizes, so there is no separate call here.
	rackSetScale(v)
	// The modules are a different size, so the rows may have re-wrapped and
	// the bay drawn behind them is stale.
	layoutRackHandles()
	positionResizeHandle()
	lsSet("wasmstuff-kscale", strconv.FormatFloat(v, 'f', 3, 64))
}

// Float mode geometry, persisted across sessions. The window itself is
// winbox-go now (panelwindow_js.go); these are what it is created with and what
// its move and resize callbacks write back.
var (
	floatX = floatDefX
	floatY = floatDefY
	floatW = floatDefW
	floatH = floatDefH
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

// Float window minimums, the same pair the window is built with. Named because
// two places have to agree about them: the window that refuses to be dragged
// smaller, and the restore that refuses to believe a smaller one was saved.
const (
	floatMinW = 240.0
	floatMinH = 160.0
	floatDefX = 24.0
	floatDefY = 48.0
	floatDefW = 384.0
	floatDefH = 720.0
)

// healFloatGeom repairs a stored geometry that could not have been arranged.
//
// A version of this saved winbox's minimize slot as the float geometry — see
// the OnResize guard in panelwindow_js.go — and the damage OUTLIVED the fix,
// because the wrong numbers were already in localStorage and are read back on
// every load. Nothing below the window's own minimum can be produced by
// dragging, so a stored size under it is that bug's residue.
//
// ALL FOUR GO BACK, not the offending dimension. The slot's numbers were
// written by one event, and the rest of them are individually plausible while
// being just as wrong: the parked width came out at 251, eleven pixels above
// the minimum and so invisible to any floor, and the parked y put the window
// on the bottom edge, where restoring its real height hangs it off the screen.
// Repairing only what fails the test leaves a panel that is technically within
// its limits and practically unusable.
func healFloatGeom() {
	if floatW >= floatMinW && floatH >= floatMinH {
		return
	}
	floatX, floatY = floatDefX, floatDefY
	floatW, floatH = floatDefW, floatDefH
	saveFloatGeom()
}

func saveFloatGeom() {
	lsSet("wasmstuff-floatX", pxStr(floatX))
	lsSet("wasmstuff-floatY", pxStr(floatY))
	lsSet("wasmstuff-floatW", pxStr(floatW))
	lsSet("wasmstuff-floatH", pxStr(floatH))
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

// hostFooter is the host page's <footer> element, when one exists — the
// "footer" dock edge appends the panel inline into it (below the host's own
// content, e.g. a store's cart links).
var hostFooter js.Value

// applyDock positions the controls panel against a window edge: bottom/top
// become a fixed horizontal strip (height dockSizeH), left/right a vertical
// sidebar (width dockSizeW), float a draggable window — and "footer" appends
// the panel INLINE into the host page's footer, below its existing content.
// The edge + sizes persist in localStorage.
func applyDock(edge string) {
	shell := doc.Call("getElementById", "panel-shell")
	p := doc.Call("getElementById", "controls-panel")
	if !shell.Truthy() || !p.Truthy() {
		return
	}
	if edge == "footer" && !hostFooter.Truthy() {
		edge = "bottom" // no host footer to dock into
	}
	if dockSizeH <= 0 {
		// Default tall enough to show a full module row (~545px at scale 1)
		// without clipping, capped so it never eats the whole viewport.
		dockSizeH = math.Min(560, winH()*0.9)
	}
	hpx := strconv.FormatFloat(dockSizeH, 'f', 0, 64) + "px"
	wpx := strconv.FormatFloat(dockSizeW, 'f', 0, 64) + "px"

	// The SHELL is placed; the panel fills it. Which way round matters for the
	// furniture: the resize bar and the dock cluster are the shell's children,
	// so if the shell hugs the panel exactly then CSS placing them against the
	// shell's edges puts them on the panel's edges, with nothing to recompute.
	//
	// For the horizontal docks the shell is left to size itself to the panel
	// and the panel keeps the max-height, rather than the shell taking a height
	// and the panel filling it. A percentage max-height against an auto-height
	// parent resolves to none, so the other way round the panel would grow past
	// the shell and the bar would sit somewhere in the middle of it.
	const shellBase = "position:fixed;box-sizing:border-box;pointer-events:auto;z-index:var(--z-panel);"
	// NB: no z-index on the panel — its base z lives in CSS (#controls-panel →
	// var(--z-panel)) so that clearing an inline override falls back to a value
	// above the canvas, never auto(0). Inline z is only ever set as a temporary
	// recovery-raise (see panelToggle / the Front handler).
	const panelLook = "background:rgba(0,0,0,0.85);padding:8px 12px;font-family:'B612 Mono',monospace;" +
		"font-size:12px;color:#aaa;pointer-events:auto;box-sizing:border-box;" +
		"-webkit-overflow-scrolling:touch;overscroll-behavior:contain;touch-action:pan-x pan-y;"

	// The panel may be hidden (PanelStartHidden / the ▤ toggle); re-docking must
	// not reveal it as a side effect of rewriting cssText.
	wasHidden := p.Get("style").Get("display").String() == "none"

	var shellCSS, panelCSS string
	float := false
	switch edge {
	case "top":
		shellCSS = "left:0;right:0;top:0;"
		panelCSS = "width:100%;max-height:" + hpx + ";overflow-y:auto;border-bottom:1px solid #333;"
	case "left":
		shellCSS = "top:0;bottom:0;left:0;width:" + wpx + ";"
		panelCSS = "width:100%;height:100%;overflow-y:auto;border-right:1px solid #333;"
	case "right":
		shellCSS = "top:0;bottom:0;right:0;width:" + wpx + ";"
		panelCSS = "width:100%;height:100%;overflow-y:auto;border-left:1px solid #333;"
	case "float":
		float = true
		// Geometry (left/top/width/height) is written onto the shell by
		// positionFloat.
		shellCSS = ""
		panelCSS = "width:100%;height:100%;overflow-y:auto;border:1px solid #2a3a4a;" +
			"border-radius:8px;box-shadow:0 10px 34px rgba(0,0,0,0.6);"
	case "footer":
		// Handled below: inline in the host's document flow, not fixed to the
		// window, so the shell is not positioned at all.
	default:
		edge = "bottom"
		shellCSS = "left:0;right:0;bottom:0;"
		panelCSS = "width:100%;max-height:" + hpx + ";overflow-y:auto;border-top:1px solid #333;"
	}

	if float {
		// Inside a window's body, so the window owns the geometry and the shell
		// just fills it. Left fixed here it would ignore the window entirely and
		// sit against the viewport while the window moved around it.
		standalonePanel = true
		shell.Get("style").Set("cssText",
			"position:relative;width:100%;height:100%;box-sizing:border-box;pointer-events:auto;")
		p.Get("style").Set("cssText", panelLook+panelCSS)
	} else if edge == "footer" {
		standalonePanel = false
		// position:relative (NOT static): the inline shell must participate in
		// z stacking, or the positioned canvas (z 3) paints over it and the
		// Front switch appears dead — static elements ignore z-index.
		shell.Get("style").Set("cssText",
			"position:relative;z-index:var(--z-panel);box-sizing:border-box;width:100%;")
		p.Get("style").Set("cssText", panelLook+
			"display:block;position:relative;width:100%;max-height:"+hpx+
			";overflow:auto;border-top:1px solid #333;")
		if !shell.Get("parentElement").Equal(hostFooter) {
			hostFooter.Call("appendChild", shell)
		}
	} else {
		standalonePanel = true
		if !shell.Get("parentElement").Equal(body) {
			body.Call("appendChild", shell)
		}
		shell.Get("style").Set("cssText", shellBase+shellCSS)
		p.Get("style").Set("cssText", panelLook+panelCSS)
	}
	if wasHidden {
		p.Get("style").Set("display", "none")
	}
	dockEdge = edge

	// (The legacy "rack" horizontal-strip layout is superseded by the module
	// system, which lays out the same in every dock mode.)
	p.Get("classList").Call("remove", "rack")
	// Tag BOTH with the edge. The shell needs it because the furniture it holds
	// is placed against that edge; the panel needs it because 700 lines of
	// stylesheet already select on #controls-panel.dk-left and there is no
	// reason to rewrite them.
	for _, e := range []string{"top", "bottom", "left", "right", "float", "footer"} {
		p.Get("classList").Call("remove", "dk-"+e)
		shell.Get("classList").Call("remove", "dk-"+e)
	}
	p.Get("classList").Call("add", "dk-"+edge)
	shell.Get("classList").Call("add", "dk-"+edge)

	// Floating is a winbox window around the shell; every other mode is the
	// shell placed against an edge, with no title bar — docked modes have no
	// top chrome, because the dock cluster clips onto the resize bar instead.
	if float {
		floatPanelWindow()
	} else {
		rememberPreFloatEdge(edge)
		unfloatPanelWindow()
	}
	positionResizeHandle()
	for _, e := range []string{"top", "bottom", "left", "right", "float", "footer"} {
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			if e == edge {
				b.Get("classList").Call("add", "active")
			} else {
				b.Get("classList").Call("remove", "active")
			}
		}
	}
	lsSet("wasmstuff-dock", edge)
	lsSet("wasmstuff-dockH", strconv.FormatFloat(dockSizeH, 'f', 0, 64))
	lsSet("wasmstuff-dockW", strconv.FormatFloat(dockSizeW, 'f', 0, 64))
	// Mid-drag the widths are re-snapped at most once a frame: a drag fires
	// pointermove far faster than that, and re-measuring every module on each
	// one is what cost the model a frame. Settled synchronously on release.
	if resizing {
		quantizeModuleWidthsSoon()
	} else {
		quantizeModuleWidths()
	}
}

// moduleSlot is the base card-slot width. Like rack cards plugged into a
// uniformly-spaced backplane, every module occupies an INTEGER number of slots
// (its content width rounded up), so modules always butt up cleanly.
//
// It is a 7 HP panel, milled 0.5 mm narrow for the seam — derived from
// rackspec rather than written out, so the arithmetic here and the stylesheet
// that draws it cannot disagree about what a slot is.
const moduleSlot = rackspec.SlotWidth * rackspec.PxPerMM

// moduleGap matches the .modules flex gap (px) — the seam between two
// adjacent panels. Multi-slot modules add (N-1) of these so their right edge
// lines up with N separate 1-slot modules, on the whole-HP grid.
const moduleGap = rackspec.Seam * rackspec.PxPerMM

// positionResizeHandle is what is left of two functions that used to place the
// resize bar and the dock cluster in viewport coordinates, recomputed in Go
// every time the panel moved — about ninety lines between them, most of it a
// switch over the dock edge repeated twice.
//
// Both are now children of #panel-shell and placed by CSS off the shell's
// dk-<edge> class, so there is nothing to position. What is left is the part
// CSS cannot do: the bar is not shown when the panel is hidden or floating
// (floating has its own corner grip), and the two overlays that keep out of the
// panel's way have to be told the panel moved.
//
// The name stays because a dozen call sites say it and each of them still means
// "the panel's geometry changed".
func positionResizeHandle() {
	if !resizeHandle.Truthy() {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	hidden := !p.Truthy() || p.Get("style").Get("display").String() == "none"
	if hidden || dockEdge == "float" {
		resizeHandle.Get("style").Set("display", "none")
	} else {
		resizeHandle.Get("style").Set("display", "")
	}
	positionAudioMeters()
	positionInfoOverlay()
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
	// Declared in the shell's furniture now rather than built here: it has to
	// be a child of the shell for CSS to place it against the dock edge.
	resizeHandle = doc.Call("getElementById", "dock-resize")
	if !resizeHandle.Truthy() {
		return
	}
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
		dl.Set("title", "DOCK — drag this label to resize the panel; the arrow buttons choose the dock edge or floating mode")
		dl.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			a[0].Call("stopPropagation")
			resizing = true
			return nil
		}))
	}
	onPointerMove(func(e js.Value) {
		if !resizing {
			return
		}
		switch dockEdge {
		case "bottom", "footer":
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
	})
	doc.Call("addEventListener", "pointerup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if resizing {
			resizing = false
			// Settle exactly, now that the once-a-frame path is done with, and
			// let the monitor start drawing again.
			quantizeModuleWidths()
			positionResizeHandle()
		}
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
			reclampPanelWindow() // a saved position must not strand it off-screen
		}
		positionResizeHandle()
		quantizeModuleWidths()
		return nil
	}))
}

func wireDockButtons() {
	for _, e := range []string{"top", "bottom", "left", "right", "float", "footer"} {
		edge := e
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				applyDock(edge)
				return nil
			}))
		}
	}
	// The footer dock target only exists on host pages that have a <footer>.
	if fb := doc.Call("getElementById", "dock-footer"); fb.Truthy() && !hostFooter.Truthy() {
		fb.Get("style").Set("display", "none")
	}
}

func readDockPref() string {
	if v, ok := lsGet("wasmstuff-dockH"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			dockSizeH = n
		}
	}
	if v, ok := lsGet("wasmstuff-dockW"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			dockSizeW = n
		}
	}
	for key, dst := range map[string]*float64{
		"wasmstuff-floatX": &floatX, "wasmstuff-floatY": &floatY,
		"wasmstuff-floatW": &floatW, "wasmstuff-floatH": &floatH,
	} {
		if v, ok := lsGet(key); ok {
			if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	healFloatGeom()
	if v, ok := lsGet("wasmstuff-dock"); ok {
		return v
	}
	return "bottom"
}

// updateGradientUI shows only the color controls relevant to the current
// palette: monochrome → one color; two-color → start+end; three-color →
// start+mid+end; rainbow → no fixed colors, show the period knob instead.
