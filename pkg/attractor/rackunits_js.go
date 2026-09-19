//go:build js && wasm

package attractor

// Building the three containers: the frame, the units in it, and the
// openings the plug-in modules go in. rackunit.go says which module lands
// in which unit; this puts them there.
//
// The frame is .rack. A unit is .runit, which carries its ears and rails
// and is a whole panel width. A SUBRACK unit has a .runit-open, which is
// the opening, and one rack-go instance manages it — one per opening, not
// one for the panel, because rack-go enumerates its container's direct
// children and an opening is a real container now rather than a rectangle
// drawn behind some of them.
//
// An INSTRUMENT unit has a .runit-panel instead: one piece of front panel
// with its own controls, bolted to the rails like any other unit. The
// scope is the first of those. A unit with BOTH — an instrument that takes
// plug-ins, which is what a Tek 7000 or an HP 180 is — needs nothing new
// here: give it a .runit-open as well and it gets an opening.

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/rack-go"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// rackFrameSel and the unit classes, in one place because the CSS and three
// functions here have to agree about them.
const (
	rackFrameSel = ".rack-frame"
	unitClass    = "runit"
	unitOpenCls  = "runit-open"
	unitPanelCls = "runit-panel"
	unitEarCls   = "runit-ear"
	unitBlankCls = "runit-blank"
)

// unitRacks is one rack-go instance per subrack opening, in unit order.
//
// The panel's flat order — which is what is saved, and what packing reads —
// is these racks' orders concatenated. Keeping the flat list as the truth
// and DERIVING which unit a module is in means the two can never disagree,
// which a stored unit number would eventually manage.
var unitRacks []*rack.Rack

// instrumentUnits are the units that are a front panel rather than an
// opening, by the id of the panel element they wrap. They are appended
// after the subracks, which is a v1 simplification: a real frame lets you
// bolt a unit anywhere in the stack, and that wants a unit order of its own
// to persist.
var instrumentUnits = []struct{ panelID, title string }{
	{"scope-panel", "Scope"},
}

// rackFrame is the 19-inch frame: the element the units are bolted into.
func rackFrame() js.Value { return doc.Call("querySelector", rackFrameSel) }

// ensureRackFrame turns the flat .modules container into a frame with one
// subrack unit in it, moving the modules into that unit's opening.
//
// Done once, in place, rather than by changing the markup: .modules is
// named by 700 lines of CSS and by the template, and the modules are
// already in it in the right order when this runs.
func ensureRackFrame() js.Value {
	if f := rackFrame(); f.Truthy() {
		return f
	}
	host := doc.Call("querySelector", ".modules")
	if !host.Truthy() {
		return js.Undefined()
	}
	// .modules becomes the FRAME. Its children become the first opening's.
	host.Get("classList").Call("add", "rack-frame")
	open := doc.Call("createElement", "div")
	open.Set("className", unitOpenCls)
	for host.Get("firstChild").Truthy() {
		c := host.Get("firstChild")
		if c.Get("nodeType").Int() == 1 &&
			c.Get("classList").Call("contains", "rack-frame").Bool() {
			break // paranoia: never move the frame into itself
		}
		open.Call("appendChild", c)
	}
	host.Call("appendChild", newSubrackUnit(open))
	return host
}

// newSubrackUnit wraps an opening in a unit: ears either side, rails above
// and below, all of it the unit's own chrome rather than something drawn
// behind a row of modules.
func newSubrackUnit(open js.Value) js.Value {
	u := doc.Call("createElement", "div")
	u.Set("className", unitClass)
	u.Call("appendChild", unitEar())
	u.Call("appendChild", open)
	u.Call("appendChild", unitEar())
	return u
}

// unitEar is the frame either side of the opening, with the holes the unit
// is bolted through. A 19-inch frame has (482.6 - 426.72)/2 of ear per
// side, which is what the handles bolt to.
func unitEar() js.Value {
	e := doc.Call("createElement", "div")
	e.Set("className", unitEarCls)
	for i := 0; i < 3; i++ {
		e.Call("appendChild", doc.Call("createElement", "i"))
	}
	return e
}

// unitOpenings is every subrack opening in the frame, in order.
func unitOpenings() []js.Value {
	f := rackFrame()
	if !f.Truthy() {
		return nil
	}
	els := f.Call("querySelectorAll", "."+unitOpenCls)
	out := make([]js.Value, 0, els.Get("length").Int())
	for i := 0; i < els.Get("length").Int(); i++ {
		out = append(out, els.Index(i))
	}
	return out
}

// syncUnitRacks makes unitRacks match the openings that exist.
func syncUnitRacks() {
	opens := unitOpenings()
	// A rack whose opening is gone has to be released, or its drag and
	// switch listeners outlive the DOM they were attached to.
	for _, r := range unitRacks {
		still := false
		for _, o := range opens {
			if r.Root().Equal(o) {
				still = true
				break
			}
		}
		if !still {
			r.Release()
		}
	}
	next := make([]*rack.Rack, 0, len(opens))
	for _, o := range opens {
		found := false
		for _, r := range unitRacks {
			if r.Root().Equal(o) {
				next = append(next, r)
				found = true
				break
			}
		}
		if !found {
			next = append(next, newOpeningRack(o))
		}
	}
	unitRacks = next
}

// moduleSlots is how many slots a module currently occupies, from the width
// rack-go quantized it to.
//
// Read back rather than recomputed: rack-go owns the measurement, and a
// second implementation of "how wide is this module" is a second answer.
func moduleSlots(m js.Value) int {
	pitch := (moduleSlot + moduleGap) * panelScale
	if pitch <= 0 {
		return 1
	}
	w := m.Get("offsetWidth").Float()
	if w <= 0 {
		return 1
	}
	n := int((w+moduleGap*panelScale)/pitch + 0.5)
	if n < 1 {
		return 1
	}
	return n
}

// relayoutUnits is the whole of the new level: measure, pack, and put each
// module in the unit it belongs to.
//
// Idempotent, and safe to call whenever the arrangement could have changed.
// It does NOT re-quantize — the caller does that first, because a module's
// slot count is the input here and asking for it again after moving things
// would be a second measurement of the same thing.
func relayoutUnits() {
	f := rackFrame()
	if !f.Truthy() {
		return
	}
	// Every module in the frame, in document order — which is the flat
	// order, because the units are in order and each opening is in order.
	els := f.Call("querySelectorAll", ".sect")
	var mods []js.Value
	var slots []int
	for i := 0; i < els.Get("length").Int(); i++ {
		m := els.Index(i)
		// A module that is switched off takes no slots but keeps its
		// PLACE: packed at width zero, so putting it back does not move
		// it to the front of the rack, and it reserves nothing in the
		// opening while it is out.
		w := moduleSlots(m)
		if m.Get("offsetParent").IsNull() && m.Get("style").Get("display").String() == "none" {
			w = 0
		}
		mods = append(mods, m)
		slots = append(slots, w)
	}
	if len(mods) == 0 {
		return
	}
	units := packUnits(slots, unitCapacitySlots())

	// Make the frame hold exactly that many subrack units, before any
	// instrument unit. Reused rather than rebuilt: recreating them every
	// pass would throw away each opening's rack and its drag listeners on
	// every resize.
	opens := unitOpenings()
	for len(opens) < len(units) {
		open := doc.Call("createElement", "div")
		open.Set("className", unitOpenCls)
		u := newSubrackUnit(open)
		f.Call("appendChild", u)
		opens = append(opens, open)
	}
	for len(opens) > len(units) {
		last := opens[len(opens)-1]
		// Rescue whatever is still in it FIRST. A unit is removed with
		// its children, and on this pass the surplus unit still holds
		// last pass's modules — they have not been re-parented yet,
		// because the re-parenting is below. Removing the unit without
		// emptying it deletes them from the document for good.
		for last.Get("firstChild").Truthy() {
			opens[0].Call("appendChild", last.Get("firstChild"))
		}
		if u := last.Call("closest", "."+unitClass); u.Truthy() {
			u.Call("remove")
		}
		opens = opens[:len(opens)-1]
	}

	// Blanks first: they are the previous pass's and belong to nothing now.
	clearUnitBlanks(f)
	for ui, idx := range units {
		open := opens[ui]
		for _, mi := range idx {
			m := mods[mi]
			if !m.Get("parentNode").Equal(open) {
				open.Call("appendChild", m)
			} else {
				open.Call("appendChild", m) // keep the order within the unit
			}
		}
		// A rack does not have a ragged gap at the end of a row; it has
		// blank panels, cut to the same widths.
		for b := unitBlankSlots(slots, idx, unitCapacitySlots()); b > 0; b-- {
			open.Call("appendChild", unitBlank())
		}
	}
	syncUnitRacks()
	relayoutInstrumentUnits(f)
}

// clearUnitBlanks removes every blank panel in the frame.
func clearUnitBlanks(f js.Value) {
	els := f.Call("querySelectorAll", "."+unitBlankCls)
	for i := 0; i < els.Get("length").Int(); i++ {
		els.Index(i).Call("remove")
	}
}

// unitBlank is one blank panel, one slot wide.
func unitBlank() js.Value {
	b := doc.Call("createElement", "div")
	b.Set("className", unitBlankCls)
	b.Get("style").Set("width", strconv.FormatFloat(moduleSlot*panelScale, 'f', 2, 64)+"px")
	return b
}

// relayoutInstrumentUnits keeps each instrument's panel in a unit of its
// own at the bottom of the frame.
//
// The panel element itself is declared in the markup and only MOVED here,
// so the scope's face stays one piece of HTML that can be styled and tested
// without knowing it is in a rack.
func relayoutInstrumentUnits(f js.Value) {
	for _, iu := range instrumentUnits {
		panel := doc.Call("getElementById", iu.panelID)
		if !panel.Truthy() {
			continue
		}
		// Found by the PANEL WRAPPER, not by any unit: until it has been
		// re-homed the panel is still sitting in a subrack opening, and
		// closest(.runit) there finds the SUBRACK — which would re-append
		// that whole unit and leave the instrument in an opening for ever.
		u := js.Value{}
		if w := panel.Call("closest", "."+unitPanelCls); w.Truthy() {
			u = w.Call("closest", "."+unitClass)
		}
		if !u.Truthy() {
			u = newInstrumentUnit(panel)
		}
		// Last, always: instrument units sit below the subracks until a
		// unit order of their own is worth persisting.
		f.Call("appendChild", u)
	}
}

// newInstrumentUnit bolts one front panel into a unit of its own: ears
// either side, and the panel where a subrack has its opening.
//
// A unit that is BOTH — an instrument that takes plug-ins, which is what
// a Tek 7000 or an HP 180 is — needs nothing new: give it a .runit-open
// beside the panel and it has an opening, and the packing above will
// fill it like any other.
func newInstrumentUnit(panel js.Value) js.Value {
	u := doc.Call("createElement", "div")
	u.Set("className", unitClass+" runit-instr")
	u.Call("appendChild", unitEar())
	w := doc.Call("createElement", "div")
	w.Set("className", unitPanelCls)
	w.Call("appendChild", panel)
	u.Call("appendChild", w)
	u.Call("appendChild", unitEar())
	return u
}

// unitFrameWidthPx is how wide the frame is: a whole 19-inch panel at the
// current interface scale.
//
// A fixed number and not the window's width, which is the change. The
// opening is 84 HP whatever the browser is doing, so a module's unit is a
// function of the order it is in and how wide it is — and a window too
// narrow for a rack scrolls a rack, the way a rack too big for a room is
// still a rack.
func unitFrameWidthPx() float64 {
	return rackspec.PanelWidth19 * rackspec.PxPerMM * panelScale
}

// ── the frame as a view: ears and rails on or off ───────────────────────

// bayOn is whether the frame is drawn as a frame — ears, rails, blanks —
// rather than as a bare flow of modules.
//
// A view of the rack and not a change to it: the units, the openings and
// which module is in which one are the same either way. This only says
// whether the metalwork is painted.
var bayOn bool

// setRackBay turns the frame's metalwork on or off and remembers it.
func setRackBay(on bool) {
	bayOn = on
	v := "0"
	if on {
		v = "1"
	}
	lsSet("wasmstuff-handles", v)
	layoutRackHandles()
}

// restoreRackBay puts the stored choice back at boot.
func restoreRackBay() {
	if v, ok := lsGet("wasmstuff-handles"); ok {
		bayOn = v == "1"
	}
	if sw := doc.Call("getElementById", "handles-on"); sw.Truthy() {
		sw.Set("checked", bayOn)
	}
	layoutRackHandles()
}

// layoutRackHandles sizes the frame and says whether its metalwork shows.
//
// This used to be two hundred lines: it measured which modules the browser
// had wrapped onto which row, then drew a bay behind each row and blank
// panels in the gap at the end of it. None of that is needed now, and its
// absence is the point — a row was an accident of the window's width, so
// the bay could only ever be painted after the fact. A unit is a declared
// 84 HP of opening, it knows its own ears, and its blanks are real children
// put there by relayoutUnits. What is left here is the frame's width.
func layoutRackHandles() {
	f := rackFrame()
	if !f.Truthy() {
		return
	}
	cl := f.Get("classList")
	st := f.Get("style")
	if !bayOn {
		cl.Call("remove", "with-bay")
		st.Set("width", "")
		st.Set("marginLeft", "")
		st.Set("marginRight", "")
		return
	}
	cl.Call("add", "with-bay")
	// A whole 19-inch panel, always — not as many slots as the window
	// happens to fit. A window narrower than a rack scrolls a rack.
	st.Set("width", strconv.FormatFloat(unitFrameWidthPx(), 'f', 2, 64)+"px")
	// The ear is a real dimension, not a look: (482.6 − 426.72)/2 of a
	// 19-inch panel either side of the 84 HP opening, which is exactly
	// what the handles bolt to. Published to the CSS rather than written
	// there, so the drawn frame and the modeled one cannot disagree.
	st.Call("setProperty", "--ear-w",
		strconv.FormatFloat(unitEarWidthPx(), 'f', 2, 64)+"px")
	st.Set("marginLeft", "auto")
	st.Set("marginRight", "auto")
}

// setScopeUnit bolts the scope's unit into the frame or takes it out.
//
// Its own switch, not a module switch, because it is not a module: the
// rack's Modules list is what is in the openings, and a unit is a peer of
// an opening rather than a thing inside one. This is also why it is
// independent of the MODEL knob — nothing about which model is on the main
// canvas has any bearing on whether an instrument is in the rack.
func setScopeUnit(on bool) {
	p := doc.Call("getElementById", "scope-panel")
	if !p.Truthy() {
		return
	}
	u := p.Call("closest", "."+unitClass)
	if !u.Truthy() {
		u = p
	}
	if on {
		u.Get("style").Set("display", "")
	} else {
		u.Get("style").Set("display", "none")
	}
}

// wireScopeUnit hooks the Scope switch up and applies its stored state.
func wireScopeUnit() {
	sw := doc.Call("getElementById", "scope-on")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		setScopeUnit(sw.Get("checked").Bool())
		saveRackLayout()
		return nil
	}))
	setScopeUnit(sw.Get("checked").Bool())
}

// unitEarWidthPx is the frame either side of the opening, in pixels at the
// current interface scale.
//
// (PanelWidth19 − RowHP×HP)/2: a 19-inch panel is 482.6 mm and the opening
// is 84 HP of it, and what is left is the two ears. Derived rather than
// chosen, so the frame drawn on screen is the frame rackspec describes.
func unitEarWidthPx() float64 {
	ear := (rackspec.PanelWidth19 - rackspec.RowHP*rackspec.HP) / 2
	return ear * rackspec.PxPerMM * panelScale
}
