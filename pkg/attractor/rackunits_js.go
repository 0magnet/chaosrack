//go:build js && wasm

package attractor

// Building the three containers: the frame, the units in it, and the
// openings the plug-in modules go in. pkg/racksurface says which module lands
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
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"strconv"
	"strings"
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

// unitHidden is the put-away set, shared by every opening's rack.
//
// One map, because being switched out is a fact about the MODULE and
// not about the unit it happens to be packed into — and packing moves
// modules between units whenever a width changes.
var unitHidden = map[string]bool{}

// instrumentUnits are the units that are a front panel rather than an
// opening, by the id of the panel element they wrap. They are appended
// after the subracks, which is a v1 simplification: a real frame lets you
// bolt a unit anywhere in the stack, and that wants a unit order of its own
// to persist.
var instrumentUnits = []struct{ panelID, title string }{
	{"scope-panel", "Scope"},
}

// rackFrame is the 19-inch frame: the element the units are bolted into.
func rackFrame() js.Value { return dom.Doc.Call("querySelector", rackFrameSel) }

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
	host := dom.Doc.Call("querySelector", ".modules")
	if !host.Truthy() {
		return js.Undefined()
	}
	// .modules becomes the FRAME. Its children become the first opening's.
	host.Get("classList").Call("add", "rack-frame")
	open := dom.Doc.Call("createElement", "div")
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
	u := dom.Doc.Call("createElement", "div")
	u.Set("className", unitClass)
	u.Call("appendChild", unitEar())
	u.Call("appendChild", open)
	u.Call("appendChild", unitEar())
	return u
}

// unitEar is the frame either side of the opening, with the holes the unit
// is bolted through. A 19-inch frame has (482.6 - 426.72)/2 of ear per
// side, which is what the handles bolt to.
//
// Three holes and a handle, and the rack style decides which show: bare
// ears have the three holes, and an ear with a handle trades the middle
// hole for it, the handle's feet bolted between the other two.
func unitEar() js.Value {
	e := dom.Doc.Call("createElement", "div")
	e.Set("className", unitEarCls)
	e.Call("appendChild", dom.Doc.Call("createElement", "i"))
	mid := dom.Doc.Call("createElement", "i")
	mid.Set("className", "ear-mid")
	e.Call("appendChild", mid)
	e.Call("appendChild", rackHandle())
	e.Call("appendChild", dom.Doc.Call("createElement", "i"))
	return e
}

// numberBay stamps a bay's number on its left ear, above the top hole —
// the same number `chaosrack rack` prints beside the bay, so the page and
// the drawing can be read against each other.
func numberBay(open js.Value, n int) {
	ear := open.Get("parentNode").Get("firstElementChild")
	if !ear.Truthy() || !ear.Get("classList").Call("contains", unitEarCls).Bool() {
		return
	}
	s := strconv.Itoa(n)
	if ear.Call("getAttribute", "data-bay").String() != s {
		ear.Call("setAttribute", "data-bay", s)
	}
}

// rackHandle is the grab handle bolted to an ear: two feet and a grip, the
// shape meshstl.RackHandle extrudes.
func rackHandle() js.Value {
	h := dom.Doc.Call("createElement", "span")
	h.Set("className", "rack-handle")
	for _, part := range []string{"rh-foot", "rh-grip", "rh-foot"} {
		p := dom.Doc.Call("createElement", "span")
		p.Set("className", part)
		h.Call("appendChild", p)
	}
	return h
}

// unitOpenings is every subrack opening in the frame, in order.
func unitOpenings() []js.Value {
	f := rackFrame()
	if !f.Truthy() {
		return nil
	}
	els := f.Call("querySelectorAll", "."+unitOpenCls)
	out := make([]js.Value, 0, els.Get("length").Int())
	for i := range els.Get("length").Int() {
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
	pitch := (moduleSlot + moduleGap) * layout.scale
	if pitch <= 0 {
		return 1
	}
	w := m.Get("offsetWidth").Float()
	if w <= 0 {
		return 1
	}
	n := int((w+moduleGap*layout.scale)/pitch + 0.5)
	if n < 1 {
		return 1
	}
	return n
}

// bayHeadAttr marks a module that has to be the first one in its bay. It is
// an attribute rather than a class because it is a fact about packing and
// not about appearance, and because the drag code rewrites className.
const bayHeadAttr = "data-bayhead"

// bayScreens are the modules OUTSIDE the model rows that carry a screen,
// and so lead a bay for the same reason a model row's head does.
//
// Two of them, and a test keeps the list honest: every module in the rack
// with a canvas in it is either a row head or named here. It is a list and
// not a query because the packer runs on slot counts before anything has
// been laid out, and because a module acquiring a canvas should be a
// decision about where it goes rather than a silent relayout.
var bayScreens = map[string]bool{
	"desk":   true, // the desk monitor
	"record": true, // the capture preview
}

// relayoutUnits is the whole of the new level: measure, pack, and put each
// module in the unit it belongs to.
//
// Idempotent, and safe to call whenever the arrangement could have changed.
// It does NOT re-quantize — the caller does that first, because a module's
// slot count is the input here and asking for it again after moving things
// would be a second measurement of the same thing.
// strayBeforeRepack records that relayoutUnits found a module outside every
// opening, and so one that the quantize before it did not measure.
var strayBeforeRepack bool

func relayoutUnits() {
	strayBeforeRepack = false
	f := rackFrame()
	if !f.Truthy() {
		return
	}
	// Every module in the frame, in document order — which is the flat
	// order, because the units are in order and each opening is in order.
	els := f.Call("querySelectorAll", ".sect")
	var mods []js.Value
	var slots []int
	var items []packItem
	for i := range els.Get("length").Int() {
		m := els.Index(i)
		// A module that is switched off takes no slots but keeps its
		// PLACE: packed at width zero, so putting it back does not move
		// it to the front of the rack, and it reserves nothing in the
		// opening while it is out.
		w := moduleSlots(m)
		// offsetWidth and not offsetParent, which is the same test here and
		// a much cheaper one. A js.Value wrapping a JS OBJECT gets a runtime
		// finalizer so the JS-side reference can be released, and attaching
		// one is the single most expensive thing this package does per DOM
		// read; a js.Value wrapping a NUMBER gets none. offsetParent hands
		// back an element, offsetWidth a float, and display:none zeroes the
		// width just as surely as it clears the offset parent. The style
		// read after it is only reached for a module that measured zero.
		//
		// The COMPUTED display, not the inline one: the Mod and EQ modules
		// are hidden by a class on the panel while audio mod is off, and
		// read through style.display they were eight modules of one slot
		// each, filling a row of nothing and pushing Presets onto its own.
		// Zero width alone is not enough — a panel that is itself hidden
		// measures every module at zero — so it is the module's own display.
		if m.Get("offsetWidth").Float() == 0 &&
			js.Global().Call("getComputedStyle", m).Get("display").String() == "none" {
			w = 0
		}
		// A module sitting outside every opening has never been quantized:
		// QuantizeAll measures the racks, and a rack is an opening. A panel
		// just built and appended to the frame is the case. Noting it here
		// is what lets the second quantize below be skipped in the usual
		// one, where every module was already in a bay and re-measuring
		// gives the width it already has.
		if !m.Get("parentNode").Get("classList").Call("contains", unitOpenCls).Bool() {
			strayBeforeRepack = true
		}
		mods = append(mods, m)
		slots = append(slots, w)
		key := moduleKeyOf(m)
		// A model's own panel is in the running model's row, and says so on the
		// element as a category card does, so anything that reads the rack
		// off the page (chaosrack rack, the terminal panel) files it in the
		// same bay rather than guessing without knowing what is running.
		if moduleSections[key] == secModel {
			if activeCategory != "" {
				m.Call("setAttribute", "data-cat", activeCategory)
			} else {
				m.Call("removeAttribute", "data-cat")
			}
		}
		items = append(items, packItem{
			Key:     key,
			Slots:   w,
			Section: sectionOfModule(m),
			Lead:    m.Call("getAttribute", bayHeadAttr).Truthy() || bayScreens[key],
		})
	}
	if len(mods) == 0 {
		return
	}
	// GROUPED before packing, not merely sorted once at boot. A module
	// built at runtime — the patchbay, the template — appears after any
	// boot-time sort has run, so an order that was in signal order
	// stopped being in it and the rack grew a second MODULATION bay with
	// a GENERATOR bay wedged between the two. The table decides which bay
	// a module is in; dragging decides its order WITHIN that bay.
	items, mods = groupBySection(items, mods)
	// slots has to follow the grouping, or the blank count for a bay is
	// computed from the widths of whichever modules USED to be at those
	// indices.
	slots = slots[:0]
	for _, it := range items {
		slots = append(slots, it.Slots)
	}
	units := packBySection(items, racksurface.UnitCapacity(), bayMonitorSlots)

	// Make the frame hold exactly that many subrack units, before any
	// instrument unit. Reused rather than rebuilt: recreating them every
	// pass would throw away each opening's rack and its drag listeners on
	// every resize.
	opens := unitOpenings()
	for len(opens) < len(units) {
		open := dom.Doc.Call("createElement", "div")
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
		numberBay(open, ui+1)
		for _, mi := range idx {
			m := mods[mi]
			// Appended even when it is already here: appending moves it to the end,
			// which is what keeps the order within the unit.
			open.Call("appendChild", m)
		}
		// A rack does not have a ragged gap at the end of a row; it has
		// blank panels, cut to the same widths.
		for b := racksurface.BlankSlots(slots, idx, racksurface.UnitCapacity()); b > 0; b-- {
			open.Call("appendChild", unitBlank())
		}
	}
	// Labels in a SECOND pass, after every module has been re-parented.
	// Their positions are measured off the modules they name, and while
	// the first pass is still running the bays it has not reached yet
	// hold last layout's modules — so a label measured then is placed
	// against a layout that is about to change. Measured: every label
	// in the rack offset by exactly the span of the runs before it, and
	// two of them computing a non-positive width and being dropped.
	// And in three phases of their own: take the old ones away, measure
	// every bay, then draw. Measuring is a read of the modules and drawing
	// is a write, so a loop that finished one bay before starting the next
	// forced the browser to re-lay out the whole rack sixteen times. See
	// drawRunLabels.
	// In JavaScript where the page allows it: a read and a write on every
	// module in the rack is exactly the shape that costs more in the
	// crossing than in the work. See fastdom_js.go. The Go pass below is
	// the same thing and stays for a page that will not evaluate it.
	if !layoutBayLabels(f, units, items) {
		for ui := range units {
			if ui < len(opens) {
				clearRunLabels(opens[ui])
			}
		}
		var labels []runLabel
		for ui, idx := range units {
			if ui < len(opens) {
				labels = append(labels, planRunLabels(opens[ui], sectionRuns(items, idx), mods, idx)...)
			}
		}
		drawRunLabels(labels)
	}
	syncUnitRacks()
	relayoutInstrumentUnits(f)
	hideEmptyUnits(f)
}

// clearUnitBlanks removes every blank panel in the frame.
func clearUnitBlanks(f js.Value) {
	els := f.Call("querySelectorAll", "."+unitBlankCls)
	for i := range els.Get("length").Int() {
		els.Index(i).Call("remove")
	}
}

// unitBlank is one blank panel, one slot wide.
func unitBlank() js.Value {
	b := dom.Doc.Call("createElement", "div")
	b.Set("className", unitBlankCls)
	b.Get("style").Set("width", strconv.FormatFloat(moduleSlot*layout.scale, 'f', 2, 64)+"px")
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
		panel := dom.Doc.Call("getElementById", iu.panelID)
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
	u := dom.Doc.Call("createElement", "div")
	u.Set("className", unitClass+" runit-instr")
	u.Call("appendChild", unitEar())
	w := dom.Doc.Call("createElement", "div")
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
	return rackspec.PanelWidth19 * rackspec.PxPerMM * layout.scale
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

// rackStyle is what the metalwork carries when it is drawn: "bare",
// "handles", "screws" or "full". The Rack bay switch says whether there is
// a frame; this says what kind.
var rackStyle = "full"

// wireRackStyle binds the Size knob's inner ring to the frame, restoring
// the stored style first.
func wireRackStyle(sel js.Value) {
	if v, ok := lsGet("wasmstuff-rackstyle"); ok {
		sel.Set("value", v)
		if sel.Get("selectedIndex").Int() < 0 {
			sel.Set("value", rackStyle)
		}
	}
	rackStyle = sel.Get("value").String()
	sel.Get("style").Set("display", "none")
	// SkipResetAll for the same reason as Size: it is how the rack looks,
	// not a setting of the instrument, and Reset All leaves those alone.
	adoptDescControl(ControlDesc{
		ID: "bay-style", Label: "Rack", IsSelect: true, SelectDef: "full",
		ResetID: "rst-knob-size", SkipResetAll: true,
		SelectApply: func(v string) {
			rackStyle = v
			lsSet("wasmstuff-rackstyle", v)
			layoutRackHandles()
		},
	})
}

// restoreRackBay puts the stored choice back at boot.
func restoreRackBay() {
	if v, ok := lsGet("wasmstuff-handles"); ok {
		bayOn = v == "1"
	}
	if sw := dom.Doc.Call("getElementById", "handles-on"); sw.Truthy() {
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
	// The class is the PAINT and nothing else. The geometry below runs
	// either way, so flipping the switch does not move a single module —
	// it used to shed the frame width, the centering and the ears all at
	// once and shift the whole rack 69px left.
	if bayOn {
		cl.Call("add", "with-bay")
	} else {
		cl.Call("remove", "with-bay")
	}
	cl.Call("toggle", "rs-handles", rackStyle == "handles" || rackStyle == "full")
	cl.Call("toggle", "rs-screws", rackStyle == "screws" || rackStyle == "full")
	// A whole 19-inch panel, always — not as many slots as the window
	// happens to fit. A window narrower than that gets the rack DRAWN
	// smaller, not cropped: see fitFrameToWidth below.
	st.Set("width", strconv.FormatFloat(unitFrameWidthPx(), 'f', 2, 64)+"px")
	// The opening is exactly its capacity, and the ears are what is left
	// of the nineteen inches — in that order. Sizing the ears first and
	// giving the opening the remainder is what made the opening too
	// narrow to hold the twelve slots it claims.
	st.Call("setProperty", "--open-w",
		strconv.FormatFloat(unitOpeningWidthPx(), 'f', 2, 64)+"px")
	ear := (unitFrameWidthPx() - unitOpeningWidthPx()) / 2
	if ear < 0 {
		ear = 0
	}
	st.Call("setProperty", "--ear-w", strconv.FormatFloat(ear, 'f', 2, 64)+"px")
	st.Set("marginLeft", "auto")
	st.Set("marginRight", "auto")
	fitFrameToWidth(f)
}

// setScopeUnit bolts the scope's unit into the frame or takes it out.
//
// Its own switch, not a module switch, because it is not a module: the
// rack's Modules list is what is in the openings, and a unit is a peer of
// an opening rather than a thing inside one. This is also why it is
// independent of the MODEL knob — nothing about which model is on the main
// canvas has any bearing on whether an instrument is in the rack.
func setScopeUnit(on bool) {
	p := dom.Doc.Call("getElementById", "scope-panel")
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
	scopeScreenPower.invalidate() // the answer just changed; do not wait to notice
}

// wireScopeUnit hooks the Scope switch up and applies its stored state.
func wireScopeUnit() {
	sw := dom.Doc.Call("getElementById", "scope-on")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
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
	return ear * rackspec.PxPerMM * layout.scale
}

// unitOpeningWidthPx is how wide a unit's opening is: exactly the capacity
// it claims.
//
// Derived and set explicitly rather than left to flex, because the opening
// declares it holds twelve slots and must therefore BE twelve slots. It was
// sized as "whatever is left over after the ears", which came out a few
// pixels narrower than twelve slots need — and since the opening does not
// wrap, a unit filled to its stated capacity ran its last module off the
// side of the rack. An opening that cannot hold what it says it holds is
// the declared-capacity model failing at the one thing it is for.
//
// N slots span N pitches less the trailing seam, which belongs to the next
// module along and not to this row.
func unitOpeningWidthPx() float64 {
	pitch := (moduleSlot + moduleGap) * layout.scale
	return float64(racksurface.UnitCapacity())*pitch - moduleGap*layout.scale
}

// fitFrameToWidth draws the whole rack smaller when the window is narrower
// than nineteen inches.
//
// The frame is ALWAYS a whole 19-inch panel with an 84 HP opening — that is
// the declared-capacity model and it does not bend. What bends is how big
// it is drawn: a rack too large for the room is photographed smaller, not
// cropped, and not left hanging out of the frame on a scrollbar. A window
// fifty pixels short of a rack should not get a scrollbar for those fifty
// pixels.
//
// A transform rather than the interface size, which is what the old bay
// shrank. The Size ring is the operator's preference; changing it to make
// something fit means remembering what it used to be and putting it back,
// which is a whole mechanism (and was). Scaling the frame touches nothing
// else and needs nothing remembered.
//
// Never larger than 1: a rack in a wide window stays its own size and sits
// in the middle, the way it does on a bench.
func fitFrameToWidth(f js.Value) {
	st := f.Get("style")
	st.Set("transform", "")
	st.Set("transformOrigin", "")
	st.Set("marginBottom", "")

	avail := frameAvailWidthPx(f)
	want := unitFrameWidthPx()
	if avail <= 0 || want <= 0 || avail >= want {
		return
	}
	k := avail / want
	// Below this the legends stop being readable and a scrollbar is the
	// lesser evil — the same floor the interface size has.
	if k < 0.6 {
		k = 0.6
	}
	// Origin at the left, and the space the frame no longer fills taken
	// back with negative margins.
	//
	// A transform scales the PIXELS and not the layout box, so a scaled
	// frame still reserved its full nineteen inches and the panel still
	// scrolled — the thing being fixed. Shrinking the box by what the
	// scale took off is what actually removes the scrollbar. Centering
	// goes with it, and does not matter: scaling only happens when the
	// rack does not fit, and then it fills the width anyway.
	st.Set("transformOrigin", "top left")
	st.Set("transform", "scale("+strconv.FormatFloat(k, 'f', 4, 64)+")")
	// A transform does not change the space the element takes in the flow,
	// so a scaled rack would leave the height it was NOT drawn at as a gap
	// underneath. Take it back.
	st.Set("marginLeft", "0")
	st.Set("marginRight", strconv.FormatFloat(-want*(1-k), 'f', 2, 64)+"px")
	if h := f.Get("offsetHeight").Float(); h > 0 {
		st.Set("marginBottom", strconv.FormatFloat(-h*(1-k), 'f', 2, 64)+"px")
	}
}

// frameAvailWidthPx is how much width the frame's container actually offers.
func frameAvailWidthPx(f js.Value) float64 {
	p := f.Get("parentElement")
	if !p.Truthy() {
		return 0
	}
	avail := p.Get("clientWidth").Float()
	cs := js.Global().Call("getComputedStyle", p)
	for _, side := range []string{"paddingLeft", "paddingRight"} {
		if v, err := strconv.ParseFloat(
			strings.TrimSuffix(cs.Get(side).String(), "px"), 64); err == nil {
			avail -= v
		}
	}
	return avail
}

// moduleKeyOf is a module's key: its header text, lowercased, which is what
// rack-go names it by and what moduleSections is keyed on.
func moduleKeyOf(m js.Value) string {
	h := m.Call("querySelector", ".sect-hdr")
	if !h.Truthy() {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(h.Get("textContent").String()))
}

// Silkscreening each section's name over the modules it covers: measured in
// planRunLabels, drawn in drawRunLabels, and split in two for the reason
// given there.
//
// Per run and not per bay, because a bay carries several sections now: one
// label on a row holding three groups would be two-thirds wrong. This is
// Woodson & Conover's way of identifying a group inside a row rather than
// by giving it a row of its own — "adequate spacing of display or control
// groups... marked outlines around each group... area color patterning"
// (§2-133). The label is the name, the tinted rule under it is the
// outline, and the section color is the patterning.
//
// Positioned over the run's own modules, measured after they have been
// placed, so a label sits above what it names whatever the widths are.

// runLabel is one bay label, measured but not yet drawn.
type runLabel struct {
	open    js.Value
	section string
	title   string
	left    float64
	width   float64
}

// clearRunLabels takes away a bay's labels from the previous pass.
func clearRunLabels(open js.Value) {
	old := open.Call("querySelectorAll", ":scope > .runit-label")
	for i := range old.Get("length").Int() {
		old.Index(i).Call("remove")
	}
}

// planRunLabels measures where a bay's labels go, and draws nothing.
//
// Reading and writing are split across the whole rack — see drawRunLabels —
// so this touches no element it does not measure.
func planRunLabels(open js.Value, runs []sectionRun, mods []js.Value, idx []int) []runLabel {
	var out []runLabel
	for _, r := range runs {
		title := sectionTitleOf(r.Section)
		if title == "" || r.Count < 1 {
			continue
		}
		// The FIRST AND LAST VISIBLE module of the run, not the first and
		// last of it. A run carries the modules that are switched out too
		// — they pack at zero width and keep their place — and those have
		// no position at all, so a run ending in one measured a negative
		// width and was silently dropped. GENERATOR and UTILITY lost their
		// labels that way, both being sections whose tail is mode-specific
		// modules that are usually off.
		a, b := js.Value{}, js.Value{}
		for n := r.From; n < r.From+r.Count && n < len(idx); n++ {
			m := mods[idx[n]]
			// Zero width is what a switched-out module looks like, and
			// reading a number rather than the element offsetParent hands
			// back saves a finalizer per module — see the note in
			// relayoutUnits.
			if !m.Truthy() || m.Get("offsetWidth").Float() == 0 {
				continue
			}
			if !a.Truthy() {
				a = m
			}
			b = m
		}
		if !a.Truthy() || !b.Truthy() {
			continue // every module in this run is switched out
		}
		left := a.Get("offsetLeft").Float()
		width := b.Get("offsetLeft").Float() + b.Get("offsetWidth").Float() - left
		if width <= 0 {
			continue
		}
		out = append(out, runLabel{open: open, section: r.Section, title: title, left: left, width: width})
	}
	return out
}

// drawRunLabels puts every bay's labels in, having measured them all first.
//
// One pass over the rack rather than one per bay, and that is the whole
// reason the function is separate from the measuring.
//
// A label is measured off the modules it names — offsetLeft, offsetWidth,
// offsetParent — and appending one is a write that dirties the layout, so a
// loop that labeled each bay in turn made the browser re-lay out the whole
// ten-thousand-element rack before every bay after the first. Sixteen bays,
// sixteen forced layouts, and labelRuns came to 83% of a relayout and about
// a fifth of the entire model change.
//
// The labels are position:absolute, so nothing here moves a module and
// deferring the writes changes no geometry — only how many times the
// browser is asked to compute it.
func drawRunLabels(ls []runLabel) {
	for _, l := range ls {
		el := dom.Doc.Call("createElement", "div")
		el.Set("className", "runit-label")
		el.Get("dataset").Set("section", l.section)
		el.Set("textContent", l.title)
		el.Set("title", l.title+" — one of the sections this bay carries. "+
			"See docs/signal-flow.md: the bays run in signal order, and a "+
			"control sits in the same row as the thing it affects.")
		st := el.Get("style")
		st.Set("left", pxStr(l.left))
		st.Set("width", pxStr(l.width))
		l.open.Call("appendChild", el)
	}
}

// groupBySection reorders the modules so each section's are contiguous,
// keeping their relative order inside it.
//
// This is what makes a label reliable: a section appears in exactly one
// run, so there is never a second label with the same name further down
// the rack. It also means an empty section produces no label at all,
// rather than a name over a stretch of blank panel.
func groupBySection(items []packItem, mods []js.Value) ([]packItem, []js.Value) {
	order := sectionOrderOf(items)
	outItems := make([]packItem, len(order))
	outMods := make([]js.Value, len(order))
	for i, j := range order {
		outItems[i], outMods[i] = items[j], mods[j]
	}
	return outItems, outMods
}

// hideEmptyUnits puts away a bay with nothing switched on in it.
//
// A bay whose every module is switched out would otherwise draw as a row
// of twelve blank panels with nothing in it. The modules stay parented
// there, so switching one back on brings the bay back with it.
func hideEmptyUnits(f js.Value) {
	us := f.Call("querySelectorAll", ":scope > ."+unitClass)
	for i := range us.Get("length").Int() {
		u := us.Index(i)
		open := u.Call("querySelector", ":scope > ."+unitOpenCls)
		if !open.Truthy() {
			continue // an instrument unit has no opening and is never empty
		}
		mods := open.Call("querySelectorAll", ":scope > .sect")
		live := false
		for j := range mods.Get("length").Int() {
			// The module's OWN display, not offsetParent: offsetParent is
			// null for anything inside a hidden ancestor, so once a bay was
			// hidden every module in it read as switched off and the bay
			// could never come back. A latch, and it took Parameters with it.
			if mods.Index(j).Get("style").Get("display").String() != "none" {
				live = true
				break
			}
		}
		if live {
			u.Get("style").Set("display", "")
		} else {
			u.Get("style").Set("display", "none")
		}
	}
}

// sectionOfModule is the bay a module belongs to.
//
// A category row says so itself, on the element, rather than being looked up
// by its header text. Two of them collide: there is an Analysis MODULE (the
// Lyapunov measurement, which belongs in the metering bay) and an Analysis
// CATEGORY (the measurement models, which is a row of its own). Keyed by
// header they are the same string, and the category row was being filed into
// the metering bay with the meter — two modules called Analysis in the rack
// and no row for the category at all.
func sectionOfModule(m js.Value) string {
	if c := m.Call("getAttribute", "data-cat"); c.Truthy() {
		if s := c.String(); s != "" {
			return categorySection(s)
		}
	}
	return moduleSection(moduleKeyOf(m))
}
