//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// The rack bay: the 19-inch frame the modules are actually in, drawn behind
// each ROW of them.
//
// Per row, and that is the point. The modules wrap: when they run past the
// width of the window the next one starts a new row, and a row is a bay. A
// frame around the whole panel would be one bay however many were on screen.
//
// It draws the frame, not just its handles. Handles on their own floated on
// the background as though bolted to nothing — they are the grab handles of
// a bay, and without the bay they are a picture of a handle.
//
// The leftover width at the end of a row is filled with BLANK PANELS, on the
// same slot pitch as the modules. A rack does not have a ragged gap at the
// end of a row; it has blanks, and they are cut to the same widths.
//
// None of it is in the container's flow. A frame or a blank that was a flex
// item would wrap like one and change which modules landed in which row —
// the layout it exists to draw.

var bayOn bool

// railHeight is the frame member above and below a 3U opening, in mm — the
// same figure meshstl.RailHeight extrudes, so the drawn bay and the modeled
// one are the same frame.
const railHeightMM = 8.0

// earWidth is the frame either side of the row, carrying the handle. A real
// 19-inch frame has (482.6 − 426.72)/2 ≈ 28 mm of ear per side, which is
// exactly what the handles bolt to.
const earWidthMM = (rackspec.PanelWidth19 - rackspec.RowHP*rackspec.HP) / 2

// setRackBay turns the bay on or off and remembers the choice.
//
// Turning it OFF puts the interface size back to whatever it was before the
// bay shrank it to fit a whole row. The bay is a view of the rack, not a
// change to it: leaving the panel smaller after the view is dismissed would
// make it a change to it.
func setRackBay(on bool) {
	bayOn = on
	v := "0"
	if on {
		v = "1"
	}
	lsSet("wasmstuff-handles", v)
	bayWantFit = on
	if !on {
		restoreScaleAfterBay()
	}
	layoutRackHandles()
	layoutRackBaySoon()
}

// bayPrevScaleKey remembers the interface size the bay shrank away from, so
// the restore survives a reload — the bay's own state does, and the two have
// to be put back together or the panel comes back small with no bay to blame.
const bayPrevScaleKey = "wasmstuff-bay-prevscale"

// rememberScaleBeforeBay records the size to come back to, once. A second
// shrink (a window resize while the bay is on) must not overwrite it with a
// size the bay itself chose.
func rememberScaleBeforeBay(s float64) {
	if _, ok := lsGet(bayPrevScaleKey); ok {
		return
	}
	lsSet(bayPrevScaleKey, strconv.FormatFloat(s, 'f', 3, 64))
}

// forgetScaleBeforeBay drops the saved size. Called when the size is changed
// by anything other than the bay — the Size knob, the dock drag — because
// from then on the current size is the one the user asked for, and putting
// the old one back would be undoing their edit rather than the bay's.
func forgetScaleBeforeBay() { lsRemove(bayPrevScaleKey) }

// restoreScaleAfterBay puts the interface size back, if the bay changed it.
func restoreScaleAfterBay() {
	v, ok := lsGet(bayPrevScaleKey)
	if !ok {
		return
	}
	lsRemove(bayPrevScaleKey)
	prev, err := strconv.ParseFloat(v, 64)
	if err != nil || prev <= 0 {
		return
	}
	// Through the bay's own guard, so restoring does not look like a user
	// edit and immediately forget itself.
	setKScaleFrom(prev, true)
}

// modulesHost is the flex container the modules live in.
func modulesHost() js.Value { return doc.Call("querySelector", ".modules") }

// bayRow is one wrapped row of modules: where it starts, where it ends, and
// how far along its last module reaches.
type bayRow struct{ top, bottom, right float64 }

// layoutRackHandles rebuilds the bays for the current row layout. Cheap
// enough to call whenever the layout could have changed — a rebuild, a
// resize, a reorder, a scale change — and idempotent.
func layoutRackHandles() {
	host := modulesHost()
	if !host.Truthy() {
		return
	}
	// Clear whatever the last pass left; the rows may have changed entirely.
	for _, sel := range []string{".rack-bay", ".rack-blank"} {
		old := host.Call("querySelectorAll", sel)
		for i := 0; i < old.Get("length").Int(); i++ {
			el := old.Index(i)
			if p := el.Get("parentNode"); p.Truthy() {
				p.Call("removeChild", el)
			}
		}
	}
	cl := host.Get("classList")
	if !bayOn {
		cl.Call("remove", "with-bay")
		// Hand the width back to the layout, or the panel stays the width
		// of a bay after the bay is gone.
		off := host.Get("style")
		off.Set("width", "")
		off.Set("marginLeft", "")
		off.Set("marginRight", "")
		return
	}
	cl.Call("add", "with-bay")

	// A bay is a WHOLE number of slots wide. Left to the window's width the
	// row ended wherever it ended and the leftover was whatever was left —
	// not a slot, so not anything a rack could hold, which is the ragged gap
	// between the last module and the right ear.
	//
	// Clear the width first so the natural one can be read, then set it to
	// the largest whole number of slots that fits, capped at the 84 HP row.
	// The container is border-box while the bay is on, so the width set here
	// INCLUDES the ears — set as a content width, content-box added them on
	// top and the bay came out two ears too wide with the difference showing
	// as a gap at the end of the row.
	st := host.Get("style")
	st.Set("width", "")
	if fitBayScale(host) {
		// The scale changed, which re-quantizes every module and calls this
		// function again with the new geometry. Nothing to draw on this pass.
		return
	}
	slot := moduleSlot * panelScale
	pitch := slot + moduleGap*panelScale
	ear := earWidthMM * rackspec.PxPerMM * panelScale
	n := baySlots(host.Get("clientWidth").Float(), pitch, slot, ear)
	inner := float64(n)*pitch - moduleGap*panelScale
	st.Set("width", pxFine(inner+2*ear))
	st.Set("marginLeft", "auto")
	st.Set("marginRight", "auto")

	rows := measureBayRows(host)
	for _, r := range rows {
		host.Call("appendChild", bayEl(r))
		for _, b := range blankPanels(r, inner) {
			host.Call("appendChild", b)
		}
	}
}

// baySlots is how many module slots a bay of the given natural width holds:
// as many whole ones as fit inside the ears, and never more than the 84 HP
// row a 19-inch frame actually has.
func baySlots(natural, pitch, slot, ear float64) int {
	if pitch <= 0 {
		return 1
	}
	// The last slot needs no gap after it, so the usable width is one gap
	// more than a whole number of pitches.
	n := int((natural - 2*ear + (pitch - slot)) / pitch)
	if n > rackspec.SlotsPerRow() {
		n = rackspec.SlotsPerRow()
	}
	if n < 1 {
		n = 1
	}
	return n
}

// measureBayRows groups the modules into rows by the top they landed on.
// Wrapping is the browser's decision, so this reads it back rather than
// trying to predict it.
func measureBayRows(host js.Value) []bayRow {
	var rows []bayRow
	kids := host.Get("children")
	for i := 0; i < kids.Get("length").Int(); i++ {
		el := kids.Index(i)
		c := el.Get("classList")
		if c.Call("contains", "rack-bay").Bool() || c.Call("contains", "rack-blank").Bool() {
			continue
		}
		h := el.Get("offsetHeight").Float()
		if h <= 0 {
			continue // a module that is switched off
		}
		top := el.Get("offsetTop").Float()
		right := el.Get("offsetLeft").Float() + el.Get("offsetWidth").Float()
		placed := false
		for j := range rows {
			// Same row if the tops are within a few pixels: modules of equal
			// height start at the same offset, and the tolerance covers one
			// with a different border.
			if d := rows[j].top - top; d < 4 && d > -4 {
				if top+h > rows[j].bottom {
					rows[j].bottom = top + h
				}
				if right > rows[j].right {
					rows[j].right = right
				}
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, bayRow{top: top, bottom: top + h, right: right})
		}
	}
	return rows
}

// bayEl builds the frame for one row: rails above and below, an ear each side,
// and a grab handle on each ear.
func bayEl(r bayRow) js.Value {
	el := doc.Call("createElement", "div")
	el.Set("className", "rack-bay")
	st := el.Get("style")
	st.Set("top", pxFine(r.top-railHeightMM*rackspec.PxPerMM*panelScale))
	st.Set("height", pxFine(r.bottom-r.top+2*railHeightMM*rackspec.PxPerMM*panelScale))
	el.Set("title", "Rack bay — one 3U opening of a 19-inch frame. Each row of modules is its own bay.")

	for _, part := range []string{"rb-rail rb-top", "rb-rail rb-bot", "rb-ear rb-l", "rb-ear rb-r"} {
		p := doc.Call("createElement", "div")
		p.Set("className", part)
		if part == "rb-ear rb-l" || part == "rb-ear rb-r" {
			p.Call("appendChild", handleEl())
		}
		el.Call("appendChild", p)
	}
	return el
}

// handleEl is the grab handle bolted to an ear: two feet and a grip, the
// shape meshstl.RackHandle extrudes.
func handleEl() js.Value {
	h := doc.Call("createElement", "div")
	h.Set("className", "rack-handle")
	for _, part := range []string{"rh-foot", "rh-grip", "rh-foot"} {
		p := doc.Call("createElement", "div")
		p.Set("className", part)
		h.Call("appendChild", p)
	}
	return h
}

// blankPanels fills the leftover at the end of a row, in whole slots.
//
// The gap between the last module and the end of the bay was whatever the
// wrap happened to leave — not a module width, and so not anything a rack
// could contain. Blanks are cut to the same slot pitch as everything else, so
// what is left over after them is under one slot wide and reads as the frame
// it is.
func blankPanels(r bayRow, inner float64) []js.Value {
	slot := moduleSlot * panelScale
	pitch := slot + moduleGap*panelScale
	ear := earWidthMM * rackspec.PxPerMM * panelScale

	var out []js.Value
	for _, left := range blankOffsets(r.right, inner, ear, slot, pitch) {
		el := doc.Call("createElement", "div")
		el.Set("className", "rack-blank")
		st := el.Get("style")
		st.Set("left", pxFine(left))
		st.Set("top", pxFine(r.top))
		st.Set("width", pxFine(slot))
		st.Set("height", pxFine(r.bottom-r.top))
		el.Set("title", "Blank panel — a slot with nothing in it")
		out = append(out, el)
	}
	return out
}

// blankOffsets is where the blanks of a row go: the arithmetic, separated
// from the DOM so it can be tested.
//
// rowRight is how far the row's last module reaches; inner is the usable
// width of the bay, which is a whole number of slots. Everything from the end
// of the last module to the end of the bay is filled, so no gap wider than a
// slot is ever left.
func blankOffsets(rowRight, inner, ear, slot, pitch float64) []float64 {
	if pitch <= 0 || slot <= 0 {
		return nil
	}
	// Blanks go on the slot GRID, not at a measured offset from the last
	// module. offsetLeft and offsetWidth are integers, so a module's right
	// edge reads up to a pixel out; chaining the blanks off it carried that
	// error to the end of the row and left a pixel and a half against the
	// ear. Only the COUNT is taken from the measurement, and a count is
	// exactly the thing rounding is safe for.
	gap := pitch - slot
	total := int(math.Round((inner + gap) / pitch))
	used := int(math.Round((rowRight - ear + gap) / pitch))
	if used < 0 {
		used = 0
	}
	var out []float64
	for k := used; k < total; k++ {
		out = append(out, ear+float64(k)*pitch)
	}
	return out
}

// restoreRackBay reads the persisted choice at boot.
func restoreRackBay() {
	v, _ := lsGet("wasmstuff-handles")
	on := v == "1"
	if sw := doc.Call("getElementById", "handles-on"); sw.Truthy() {
		sw.Set("checked", on)
	}
	bayOn = on
	layoutRackHandles()
	layoutRackBaySoon()
}

// layoutRackBaySoon re-runs the layout on the next frame.
//
// Turning the bay on adds the ears' width to the container's padding, which
// narrows the row and can re-wrap it — so the first pass measures the layout
// the bay is about to invalidate. That is how a blank panel came to be drawn
// on top of the View module: the row's right edge was read before the module
// had moved.
func layoutRackBaySoon() {
	js.Global().Call("requestAnimationFrame", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		layoutRackHandles()
		return nil
	}))
}

// bayWantFit is set when the bay is switched ON, and cleared as soon as it has
// fitted once.
//
// Without it the fit would run on every layout, which means it would undo the
// Size knob: pick L while the bay is on and the next layout would shrink it
// straight back to whatever makes twelve slots fit, leaving the knob dead.
// Fitting is what turning the bay ON does; after that the size is yours and
// the bay simply draws as many whole slots as that size allows.
var bayWantFit bool

// bayWidth1 is what a full 84 HP bay measures at interface scale 1: twelve
// slots, the eleven seams between them, and an ear at each end.
func bayWidth1() float64 {
	n := float64(rackspec.SlotsPerRow())
	return n*(moduleSlot+moduleGap) - moduleGap + 2*earWidthMM*rackspec.PxPerMM
}

// fitBayScale shrinks the interface so a WHOLE 84 HP row fits the window, and
// reports whether it changed anything.
//
// A 19-inch rack is 482.6 mm, which at the panel's own 4 px/mm is 1930 px —
// wider than a 1920-pixel screen has room for once the window's own furniture
// is taken out. So at scale 1 the bay could only ever hold eleven of its
// twelve slots, and the twelfth showed up as a gap at the end of the row.
//
// It only ever shrinks. Growing would fight the Size knob, which is the
// user's; making the rack smaller so that all of it is a rack is the thing
// they asked for.
func fitBayScale(host js.Value) bool {
	if !bayWantFit {
		return false
	}
	avail := availableWidth(host)
	if avail <= 0 {
		return false
	}
	want := avail / bayWidth1()
	// Only shrink, and only when it is worth a redraw.
	if want >= panelScale-0.004 {
		return false
	}
	if want < 0.6 { // setKScale's floor; below it the panel is unreadable
		want = 0.6
	}
	if want >= panelScale-0.004 {
		return false
	}
	bayWantFit = false // before the call: setKScale re-enters this layout
	rememberScaleBeforeBay(panelScale)
	setKScaleFrom(want, true)
	return true
}

// availableWidth is the content width the modules container has to work with,
// read from its PARENT — the container's own clientWidth is whatever width
// this code last set, which would make the fit depend on its own output.
func availableWidth(host js.Value) float64 {
	p := host.Get("parentElement")
	if !p.Truthy() {
		return 0
	}
	w := p.Get("clientWidth").Float()
	cs := js.Global().Call("getComputedStyle", p)
	if !cs.Truthy() {
		return w
	}
	return w - cssPx(cs.Get("paddingLeft")) - cssPx(cs.Get("paddingRight"))
}

// cssPx reads a computed "12px" as 12.
func cssPx(v js.Value) float64 {
	if !v.Truthy() {
		return 0
	}
	f := js.Global().Get("parseFloat").Invoke(v)
	if f.Type() != js.TypeNumber || js.Global().Get("isNaN").Invoke(f).Bool() {
		return 0
	}
	return f.Float()
}

// pxFine is pxStr with sub-pixel precision.
//
// The bay's own geometry is arithmetic on a fractional slot pitch (142.24 px
// at scale 1), and rounding each piece of it to whole pixels left a pixel or
// two between the last blank and the ear — a smaller version of exactly the
// gap this was all meant to remove.
func pxFine(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) + "px" }
