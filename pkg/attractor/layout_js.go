//go:build js && wasm

package attractor

import (
	_ "embed"
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// panelLayout is the control panel's geometry: its size, where it is docked
// or floating, and its scale.
type panelLayout struct {
	standalone   bool
	dockEdge     string
	dockSizeH    float64 // panel height (px) when docked bottom/top
	dockSizeW    float64 // panel width (px) when docked left/right
	resizeHandle js.Value
	resizing     bool

	// scale mirrors the CSS --kscale (interface size). Both the Size knob and
	// the bottom/top dock resize-drag drive it, so "resize the panel" scales the
	// whole control interface — which works even though every module is a
	// fixed-height 3-row grid (a plain height drag could only clip it).
	scale float64

	// Float mode geometry, persisted across sessions. The window itself is
	// winbox-go now (panelwindow_js.go); these are what it is created with and what
	// its move and resize callbacks write back.
	floatX float64
	floatY float64
	floatW float64
	floatH float64

	// hostFooter is the host page's <footer> element, when one exists — the
	// "footer" dock edge appends the panel inline into it (below the host's own
	// content, e.g. a store's cart links).
	hostFooter js.Value
}

var layout = panelLayout{
	dockEdge:  "bottom",
	dockSizeW: 360.0,
	scale:     1.0,
	floatX:    floatDefX,
	floatY:    floatDefY,
	floatW:    floatDefW,
	floatH:    floatDefH,
}

// setKScale sets the interface size: clamped, persisted, and told to
// every unit's rack so the slot pitch follows.
//
// It used to take a flag saying whether the change came from the rack
// bay, because the bay shrank the interface to make a whole 84 HP row
// fit the window and had to put the size back afterwards. The frame is
// a fixed 84 HP now and a window too narrow for it scrolls, so there is
// nothing that changes the size behind the user's back and no size to
// remember on their behalf.

func (pa *panelLayout) setKScale(v float64) {
	if v < 0.6 {
		v = 0.6
	} else if v > 2.2 {
		v = 2.2
	}
	pa.scale = v
	dom.Doc.Get("documentElement").Get("style").Call("setProperty", "--kscale", strconv.FormatFloat(v, 'f', 3, 64))
	// --kscale drives the CSS; the rack needs the same number told to it,
	// because the slot pitch it snaps modules to scales with the interface.
	// rackSetScale re-quantizes, so there is no separate call here.
	rackSetScale(v)
	// The modules are a different size, so what fits in a unit has
	// changed and the frame is a different width.
	layoutRackHandles()
	layoutSkirts() // every input to the skirt geometry scales with the interface
	pa.positionResizeHandle()
	lsSet("wasmstuff-kscale", strconv.FormatFloat(v, 'f', 3, 64))
}

func pxStr(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }

// clampFloatPos keeps the float's title bar reachable within the current
// viewport. Lives inside positionFloat so EVERY path that moves the float —
// dragging, a persisted position restored into a smaller window, a window
// resize — goes through the one clamp; a stale localStorage position can
// never strand the panel off-screen.
func (pa *panelLayout) clampFloatPos() {
	if pa.floatX < 8-pa.floatW+120 {
		pa.floatX = 8 - pa.floatW + 120
	}
	if pa.floatX > winW()-40 {
		pa.floatX = winW() - 40
	}
	if pa.floatY < 0 {
		pa.floatY = 0
	}
	if pa.floatY > winH()-30 {
		pa.floatY = winH() - 30
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
func (pa *panelLayout) healFloatGeom() {
	if pa.floatW >= floatMinW && pa.floatH >= floatMinH {
		return
	}
	pa.floatX, pa.floatY = floatDefX, floatDefY
	pa.floatW, pa.floatH = floatDefW, floatDefH
	pa.saveFloatGeom()
}

func (pa *panelLayout) saveFloatGeom() {
	lsSet("wasmstuff-floatX", pxStr(pa.floatX))
	lsSet("wasmstuff-floatY", pxStr(pa.floatY))
	lsSet("wasmstuff-floatW", pxStr(pa.floatW))
	lsSet("wasmstuff-floatH", pxStr(pa.floatH))
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

// applyDock positions the controls panel against a window edge: bottom/top
// become a fixed horizontal strip (height dockSizeH), left/right a vertical
// sidebar (width dockSizeW), float a draggable window — and "footer" appends
// the panel INLINE into the host page's footer, below its existing content.
// The edge + sizes persist in localStorage.
func (pa *panelLayout) applyDock(edge string) {
	shell := dom.Doc.Call("getElementById", "panel-shell")
	p := dom.Doc.Call("getElementById", "controls-panel")
	if !shell.Truthy() || !p.Truthy() {
		return
	}
	if edge == "footer" && !pa.hostFooter.Truthy() {
		edge = "bottom" // no host footer to dock into
	}
	if pa.dockSizeH <= 0 {
		// Default tall enough to show a full module row (~545px at scale 1)
		// without clipping, capped so it never eats the whole viewport.
		pa.dockSizeH = math.Min(560, winH()*0.9)
	}
	hpx := strconv.FormatFloat(pa.dockSizeH, 'f', 0, 64) + "px"
	wpx := strconv.FormatFloat(pa.dockSizeW, 'f', 0, 64) + "px"

	// The SHELL is placed; the panel fills it. Which way round matters for the
	// furniture: the resize bar and the dock cluster are the shell's children,
	// so if the shell hugs the panel exactly then CSS placing them against the
	// shell's edges puts them on the panel's edges, with nothing to recompute.
	//
	// For the horizontal docks the shell is left to size itself to the panel
	// and the panel carries the height, rather than the shell taking a height
	// and the panel filling it. A percentage height against an auto-height
	// parent resolves to auto, so the other way round the panel would grow past
	// the shell and the bar would sit somewhere in the middle of it.
	//
	// An explicit height and not a max-height: a drawer is the size it was
	// pulled to. Under max-height the panel stopped at whatever its CONTENT
	// happened to need, so pulling past that did nothing at all and the
	// drawer could not be opened to cover the page — the drag was moving a
	// number the layout had already ignored. Height with overflow-y:auto IS
	// the drawer: too small and the modules scroll inside it, too large and
	// it is simply open that far. The vertical docks always did it this way.
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
		panelCSS = "width:100%;height:" + hpx + ";overflow-y:auto;border-bottom:1px solid #333;"
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
		panelCSS = "width:100%;height:" + hpx + ";overflow-y:auto;border-top:1px solid #333;"
	}

	if float {
		// Inside a window's body, so the window owns the geometry and the shell
		// just fills it. Left fixed here it would ignore the window entirely and
		// sit against the viewport while the window moved around it.
		pa.standalone = true
		shell.Get("style").Set("cssText",
			"position:relative;width:100%;height:100%;box-sizing:border-box;pointer-events:auto;")
		p.Get("style").Set("cssText", panelLook+panelCSS)
	} else if edge == "footer" {
		pa.standalone = false
		// position:relative (NOT static): the inline shell must participate in
		// z stacking, or the positioned canvas (z 3) paints over it and the
		// Front switch appears dead — static elements ignore z-index.
		shell.Get("style").Set("cssText",
			"position:relative;z-index:var(--z-panel);box-sizing:border-box;width:100%;")
		p.Get("style").Set("cssText", panelLook+
			"display:block;position:relative;width:100%;max-height:"+hpx+
			";overflow:auto;border-top:1px solid #333;")
		if !shell.Get("parentElement").Equal(pa.hostFooter) {
			pa.hostFooter.Call("appendChild", shell)
		}
	} else {
		pa.standalone = true
		if !shell.Get("parentElement").Equal(dom.Body) {
			dom.Body.Call("appendChild", shell)
		}
		shell.Get("style").Set("cssText", shellBase+shellCSS)
		p.Get("style").Set("cssText", panelLook+panelCSS)
	}
	if wasHidden {
		p.Get("style").Set("display", "none")
	}
	pa.dockEdge = edge

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
	layoutSkirts() // a re-dock may be the first time the panel has a size
	pa.positionResizeHandle()
	for _, e := range []string{"top", "bottom", "left", "right", "float", "footer"} {
		if b := dom.Doc.Call("getElementById", "dock-"+e); b.Truthy() {
			if e == edge {
				b.Get("classList").Call("add", "active")
			} else {
				b.Get("classList").Call("remove", "active")
			}
		}
	}
	lsSet("wasmstuff-dock", edge)
	lsSet("wasmstuff-dockH", strconv.FormatFloat(pa.dockSizeH, 'f', 0, 64))
	lsSet("wasmstuff-dockW", strconv.FormatFloat(pa.dockSizeW, 'f', 0, 64))
	// Mid-drag the widths are re-snapped at most once a frame: a drag fires
	// pointermove far faster than that, and re-measuring every module on each
	// one is what cost the model a frame. Settled synchronously on release.
	if pa.resizing {
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
func (pa *panelLayout) positionResizeHandle() {
	if !pa.resizeHandle.Truthy() {
		return
	}
	p := dom.Doc.Call("getElementById", "controls-panel")
	hidden := !p.Truthy() || p.Get("style").Get("display").String() == "none"
	if hidden || pa.dockEdge == "float" {
		pa.resizeHandle.Get("style").Set("display", "none")
	} else {
		pa.resizeHandle.Get("style").Set("display", "")
	}
	pa.positionAudioMeters()
}

// positionAudioMeters keeps the top-left audio-feature meter overlay clear of
// the control panel: it shifts right of a left sidebar or below a top strip,
// and returns to the corner for bottom/right docks or when the panel is hidden.
func (pa *panelLayout) positionAudioMeters() {
	if !af.overlay.Truthy() {
		return
	}
	top, left := 8.0, 8.0
	if pa.standalone {
		if p := dom.Doc.Call("getElementById", "controls-panel"); p.Truthy() && p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch pa.dockEdge {
			case "left":
				left = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "top":
				top = r.Get("bottom").Float() + 8
			}
		}
	}
	st := af.overlay.Get("style")
	st.Set("top", strconv.FormatFloat(top, 'f', 0, 64)+"px")
	st.Set("left", strconv.FormatFloat(left, 'f', 0, 64)+"px")
}

// initDockResize creates the resize bar and its drag handlers (document-level
// so the drag continues past the thin bar).
func (pa *panelLayout) initDockResize() {
	// Declared in the shell's furniture now rather than built here: it has to
	// be a child of the shell for CSS to place it against the dock edge.
	pa.resizeHandle = dom.Doc.Call("getElementById", "dock-resize")
	if !pa.resizeHandle.Truthy() {
		return
	}
	pa.resizeHandle.Call("addEventListener", "pointerdown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		pa.resizing = true
		return nil
	}))
	// The "DOCK" label doubles as a resize grip (a bigger, obvious touch target
	// than the thin bar). Dragging it resizes exactly like the bar.
	if dl := dom.Doc.Call("querySelector", "#dock-controls .dock-lbl"); dl.Truthy() {
		dl.Get("style").Set("cursor", "grab")
		dl.Get("style").Set("touchAction", "none")
		dl.Set("title", "DOCK — drag this label to resize the panel; the arrow buttons choose the dock edge or floating mode")
		dl.Call("addEventListener", "pointerdown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			a[0].Call("stopPropagation")
			pa.resizing = true
			return nil
		}))
	}
	onPointerMove(func(e js.Value) {
		if !pa.resizing {
			return
		}
		switch pa.dockEdge {
		case "bottom", "footer":
			pa.dockSizeH = winH() - e.Get("clientY").Float()
		case "top":
			pa.dockSizeH = e.Get("clientY").Float()
		case "left":
			pa.dockSizeW = e.Get("clientX").Float()
		case "right":
			pa.dockSizeW = winW() - e.Get("clientX").Float()
		}
		// The panel travels the WHOLE edge: shut at one end, covering the
		// page at the other.
		//
		// It used to stop 120px short of shut and 4% short of full, and both
		// ends were wrong for the same reason — a drawer that will not close
		// and will not open all the way is a drawer arguing with the hand on
		// it. The old floor was guarding something real, that a panel pulled
		// to nothing cannot be pulled back, and guarding it in the wrong
		// place: the grip is its own element pinned to the dock EDGE, not to
		// the panel's content, so it stays reachable at any size and the
		// floor only has to keep the grip itself on screen.
		grip := pa.dockGripPx()
		pa.dockSizeH = clampDock(pa.dockSizeH, grip, winH())
		pa.dockSizeW = clampDock(pa.dockSizeW, grip, winW())
		pa.applyDock(pa.dockEdge)
	})
	dom.Doc.Call("addEventListener", "pointerup", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if pa.resizing {
			pa.resizing = false
			// Settle exactly, now that the once-a-frame path is done with, and
			// let the monitor start drawing again.
			quantizeModuleWidths()
			pa.positionResizeHandle()
		}
		return nil
	}))
	// Keep the bar on the panel's edge as its content height changes
	// (audio-mod rows, section collapse, mode switches, window resize).
	if ro := js.Global().Get("ResizeObserver"); ro.Truthy() {
		obs := ro.New(dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			pa.positionResizeHandle()
			return nil
		}))
		if p := dom.Doc.Call("getElementById", "controls-panel"); p.Truthy() {
			obs.Call("observe", p)
		}
	}
	js.Global().Call("addEventListener", "resize", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if pa.dockEdge == "float" {
			reclampPanelWindow() // a saved position must not strand it off-screen
		}
		pa.positionResizeHandle()
		quantizeModuleWidths()
		return nil
	}))
}

func (pa *panelLayout) wireDockButtons() {
	for _, e := range []string{"top", "bottom", "left", "right", "float", "footer"} {
		edge := e
		if b := dom.Doc.Call("getElementById", "dock-"+e); b.Truthy() {
			b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
				pa.applyDock(edge)
				return nil
			}))
		}
	}
	// The footer dock target only exists on host pages that have a <footer>.
	if fb := dom.Doc.Call("getElementById", "dock-footer"); fb.Truthy() && !pa.hostFooter.Truthy() {
		fb.Get("style").Set("display", "none")
	}
}

func (pa *panelLayout) readDockPref() string {
	if v, ok := lsGet("wasmstuff-dockH"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			pa.dockSizeH = n
		}
	}
	if v, ok := lsGet("wasmstuff-dockW"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			pa.dockSizeW = n
		}
	}
	for key, dst := range map[string]*float64{
		"wasmstuff-floatX": &pa.floatX, "wasmstuff-floatY": &pa.floatY,
		"wasmstuff-floatW": &pa.floatW, "wasmstuff-floatH": &pa.floatH,
	} {
		if v, ok := lsGet(key); ok {
			if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	pa.healFloatGeom()
	if v, ok := lsGet("wasmstuff-dock"); ok {
		return v
	}
	return "bottom"
}

// updateGradientUI shows only the color controls relevant to the current
// palette: monochrome → one color; two-color → start+end; three-color →
// start+mid+end; rainbow → no fixed colors, show the period knob instead.

// dockGripPx is how much of the edge the resize grip needs to stay on
// screen — the floor a shut drawer stops at, so there is always something
// to pull it back out by.
//
// Measured rather than a constant because the grip is sized in CSS and
// scales with the interface Size ring: a number written here would be the
// right floor at one setting and would swallow the grip at another.
func (pa *panelLayout) dockGripPx() float64 {
	const fallback = 10.0 // before layout, or if the grip is display:none
	if !pa.resizeHandle.Truthy() {
		return fallback
	}
	r := pa.resizeHandle.Call("getBoundingClientRect")
	grip := r.Get("height").Float()
	if w := r.Get("width").Float(); w > 0 && w < grip {
		grip = w // the vertical edges: the bar is tall and thin
	}
	if !(grip > 0) {
		return fallback
	}
	return grip
}

// clampDock holds a dock size between the grip floor and the full window.
func clampDock(v, lo, hi float64) float64 {
	if lo > hi {
		lo = hi
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
