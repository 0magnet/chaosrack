//go:build js && wasm

package attractor

// The rack: what holds the modules, and how wide each one is.
//
// This used to be about four hundred lines here — a slot quantizer, the
// show/hide switches, and header-drag reordering — none of which is about
// attractors. It is now github.com/0magnet/rack-go, and this file is the
// adapter: it tells the rack what this panel's elements are called and leaves
// the rest of the app calling the same four functions it always did.
//
// Keeping the old names as delegates rather than rewriting twenty call sites is
// deliberate. quantizeModuleWidths is called from a dozen places for a dozen
// good reasons — a mode changed, a module was built, the interface was scaled —
// and each of those call sites is still correct. Only the implementation moved.

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/racklayout"
	"github.com/0magnet/chaosrack/pkg/racksurface"

	"github.com/0magnet/rack-go"
)

// panelRack is gone: there is no single rack any more. unitRacks (see
// rackunits_js.go) holds one per unit opening, and the functions below
// aggregate across them so the dozen call sites elsewhere did not have to
// learn that the rack grew a level.

// rackModuleContent is where a module's real width is measured from.
//
// NOT #params: that is a display:contents row whose only child is the
// full-width grid, so measuring it reports the width the module was widened to
// rather than the width its content needs.
const rackModuleContent = ".punit-grid, .recgrid, .toprow, .row:not(#params)"

// ensureRack builds the rack over the panel's module container the first time
// the container exists, and returns nil until then. Several things ask for a
// re-quantize before the panel has been built — the interface scale is restored
// from storage on boot, for one — and they should be no-ops rather than
// ordering constraints.
func ensureRack() *rack.Rack {
	if f := ensureRackFrame(); !f.Truthy() {
		return nil
	}
	syncUnitRacks()
	if len(unitRacks) == 0 {
		return nil
	}
	return unitRacks[0]
}

// newOpeningRack builds the rack that manages ONE unit's opening.
//
// One per opening rather than one for the panel, because rack-go
// enumerates its container's direct children and an opening is a real
// container now. The options are identical for every opening: they are
// all the same kind of subrack, and a rack whose slots were a different
// pitch from its neighbor's would not be a rack.
func newOpeningRack(container js.Value) *rack.Rack {
	return rack.New(rack.Options{
		Container: container,
		// This panel's own names, so the rack manages the elements the rest of
		// the app and 700 lines of CSS already know by these names.
		ModuleClass:     "sect",
		HeaderClass:     "sect-hdr",
		ContentSelector: rackModuleContent,
		// This panel styles its switches as input.sw inside label.grp.
		SwitchClass:      "sw",
		SwitchLabelClass: "grp",
		SlotWidth:        moduleSlot,
		Gap:              moduleGap,
		Scale:            layout.scale,
		Pinned:           modulePinned,
		// Every way the user can rearrange the rack ends at the same record.
		// The rack fires these only for a real change — SetOrder and SetHidden,
		// which is how the record is restored, deliberately do not — so there
		// is no boot-time loop of reading a layout and writing it straight back.
		// Every opening is a drop target for every other, so a module can be
		// carried up to the row above. Without it a rack held you inside
		// whichever unit you picked the module up in.
		Siblings: func() []*rack.Rack { return unitRacks },
		// And ONE put-away set between them. A module repacked into another
		// unit used to leave its "hidden" record behind in the rack it came
		// from, so it stayed off-screen with its switch saying it was on.
		Hidden: unitHidden,
		OnReorder: func([]string) {
			saveRackLayout()
			// A module dropped into another unit changes what fits in both,
			// and the flat order it was dropped into is what packing reads.
			quantizeModuleWidthsSoon()
		},
		OnVisibility: func(string, bool) {
			saveRackLayout()
			// A module going in or out changes what fits in a unit, so the
			// rack is repacked. Coalesced onto the next frame: the switch
			// handler has not finished changing the DOM yet, and measuring
			// it now measures the state being left behind.
			quantizeModuleWidthsSoon()
		},
	})
}

// modulePinned names the modules that get no show/hide switch.
//
// The Console, because it is where the switches are: every other module can be
// put away precisely because the way to bring one back is always in the same
// place, and a switch that could hide the switches would be a door that locks
// from the outside.
//
// The rest answer to switches of their own. Mod, EQ and Physics are shown and
// hidden by a class on the panel rather than by a style of their own, so they
// look visible here even when they are not, and a module a mode has taken away
// has that mode's reasons. A second switch contradicting the first is only a
// way to get stuck.
// The rule itself is modulePinnedFrom, in racklayout.go; this gathers the three
// facts it needs. Measured before the middle one was there: thirteen switches
// before a reload, twelve after, and the missing one was the module that had
// just been put away.
func modulePinned(key string) bool {
	if moduleNeverSwitched(key) {
		return true
	}
	r, m := rackFor(key)
	if r == nil {
		return true // not in any opening yet: no switch to offer
	}
	return racklayout.ModulePinnedFrom(false, r.Hidden(key),
		m.Get("style").Get("display").String() == "none")
}

// moduleNeverSwitched names the modules that can have no switch whatever their
// current display: the Console, because the switches live in it, and the three
// that a MODE shows and hides by a class on the panel. A switch contradicting
// the mode is only a way to get stuck.
func moduleNeverSwitched(key string) bool {
	r, m := rackFor(key)
	if r == nil || !m.Truthy() {
		return true
	}
	cl := m.Get("classList")
	for _, c := range []string{"console", "modmodule", "eqmodule", "physmodule"} {
		if cl.Call("contains", c).Bool() {
			return true
		}
	}
	return false
}

// quantizeModuleWidths snaps every module to a whole number of slots.
func quantizeModuleWidths() {
	if owed.deferred {
		owed.quantize = true
		return
	}
	if ensureRack() == nil {
		return
	}
	// What the panels must be at least as wide as, before asking how wide
	// they want to be: a legend ring bigger than its column is a part
	// bolted to the panel, not content that flows.
	fitModulesToTheirParts()
	// Every opening, then repack: a module's slot count is the input to
	// packing, so it has to be settled before anything is moved.
	//
	// All sixteen bays in one pass. Quantizing them one at a time made the
	// browser recompute style and layout for the whole page between every
	// pair — see rack.QuantizeAll.
	rack.QuantizeAll(unitRacks)
	latchModuleWidths()
	relayoutUnits()
	// A second pass, but only when the first one can have missed something.
	//
	// A module is measured by stretching it to 3000px and reading its
	// content back, which is a width that depends on what is ON the panel
	// and not on which bay it is in — so re-measuring a module the repack
	// has merely MOVED gives the width it already has, and costs two full
	// style-and-layout passes over the page to say so. Verified rather than
	// assumed: six models and four interface sizes give a byte-identical
	// rack either way.
	//
	// What the first pass can miss is a module that was not in any opening
	// when it ran, because QuantizeAll measures racks and a rack is an
	// opening. relayoutUnits has just put every module in one and says
	// whether it found any outside.
	if strayBeforeRepack {
		rack.QuantizeAll(unitRacks)
		latchModuleWidths()
	}
	layoutRackHandles()
	// Skirts are MEASURED, so they are sized after the layout that gives
	// them a size. Here because this is the one funnel every layout
	// change already passes through: a rebuild, a resize, a reorder, an
	// interface-scale change. A skirt sized against a detached or
	// zero-width knob is a skirt that never gets sized at all.
	layoutSkirts()
	// And if that gave a ring a size the pass above did not know — the
	// first build of a module, where every skirt was an estimate — the
	// panel it is on may have to be wider. Once, and no further: the
	// skirts are measured now, so a third pass would size them the same.
	if fitModulesToTheirParts() {
		rack.QuantizeAll(unitRacks)
		latchModuleWidths()
		relayoutUnits()
		layoutRackHandles()
	}
}

// applyModuleVisibility puts away what the switches say to put away, and leaves
// everything else alone. Called after a rebuild, which replaces the elements the
// last pass acted on.
func applyModuleVisibility() {
	if ensureRack() != nil {
		rack.ApplyAll(unitRacks)
	}
	// A module just switched IN changes what fits in a unit, so the rack
	// has to be repacked — not merely redrawn. Without this a module
	// switched on was appended to whatever unit it was last in and ran
	// off the side of the frame, since an opening does not wrap.
	quantizeModuleWidths()
}

// wireModuleDrag exists for the one call site that turns dragging on. The rack
// wires it when it is built, so this only has to make sure it has been.
func wireModuleDrag() { ensureRack() }

// rackSetScale passes the interface scale through to the rack, which sizes its
// slots by it. The panel's own --kscale still drives the CSS; this is the same
// number told to the thing that does the arithmetic.
func rackSetScale(v float64) {
	if ensureRack() == nil {
		return
	}
	// Every panel is re-milled at a new interface size, so the widths
	// latched at the old one mean nothing.
	moduleWidthHighWater = map[string]int{}
	for _, r := range unitRacks {
		r.SetScale(v)
	}
}

// quantizePending is set while a coalesced re-quantize is waiting for the next
// animation frame.
var quantizePending bool

// quantizeModuleWidthsSoon re-snaps the module widths at most once per frame.
//
// Quantizing is not cheap and is not supposed to be: every module is widened
// out of the way, measured from its content's own children, and set back, which
// is three forced layouts of the whole rack. That is fine when something
// changed — a mode switch, a module built, the interface rescaled.
//
// It is not fine on pointermove. Dragging the panel's edge fires those as fast
// as the pointer reports, and each one was re-measuring fourteen modules; the
// model lost a frame every time the work landed across a redraw. Once per
// animation frame is as often as the result can be seen anyway.
//
// The synchronous quantizeModuleWidths stays for callers that need the widths
// correct on the next line rather than the next frame.
func quantizeModuleWidthsSoon() {
	if quantizePending {
		return
	}
	quantizePending = true
	var fn js.Func
	fn = js.FuncOf(func(js.Value, []js.Value) interface{} {
		quantizePending = false
		fn.Release()
		quantizeModuleWidths()
		return nil
	})
	js.Global().Call("requestAnimationFrame", fn)
}

// requantizeAfterFonts re-measures the module widths once the panel's fonts
// have actually been applied.
//
// The widths are measured from the content, and the content is mostly TEXT, so
// the measurement is only as good as the font it was taken in. injectFonts adds
// the @font-face rules at startup and the browser applies them asynchronously —
// base64 in the rule rather than a network fetch does not change that — so the
// first quantize can easily run against the fallback font and come out with a
// module one slot too narrow. Nothing re-measured afterwards, so it stayed
// narrow for the session: the Colors module came up at half its width, and
// undocking and redocking the panel fixed it because docking re-quantizes.
//
// One extra measurement per session, on a promise that is already resolved by
// the time it is asked on a warm cache — which is the case this has to handle
// as well. Resolved immediately, the callback runs on the next microtask,
// which is still before the panel has been laid out once: measured then, the
// widest category rows came out a slot short and stayed short, clipping 35px
// of their last column, and a window resize put them right. So the measure is
// taken two animation frames after the promise, which is after the first
// paint on a warm cache and no later than it already was on a cold one.
func requantizeAfterFonts() {
	fonts := dom.Doc.Get("fonts")
	if !fonts.Truthy() {
		return
	}
	ready := fonts.Get("ready")
	if !ready.Truthy() || ready.Type() != js.TypeObject {
		return
	}
	var fn js.Func
	fn = js.FuncOf(func(js.Value, []js.Value) interface{} {
		fn.Release()
		afterTwoFrames(func() {
			// Forget what the panels measured in the wrong font.
			//
			// latchModuleWidths stops a module ever getting NARROWER, which is
			// right for a knob that changes what is on a panel and wrong for
			// this one: the first pass measures in the fallback face, before
			// the panel's own has been applied, and whatever it got is then
			// held for the rest of the session. Analysis is two half-height
			// cells that stack into one column and want ONE slot; measured
			// early they sat side by side, latched at two, and the module
			// stayed twice as wide as anything on it.
			//
			// Clearing the marks here is the argument rackSetScale already
			// makes when it clears them: measuring again in a different font
			// re-mills every panel, so the old marks describe a rack that no
			// longer exists.
			moduleWidthHighWater = map[string]int{}
			quantizeModuleWidths()
		})
		return nil
	})
	ready.Call("then", fn)
}

// afterTwoFrames runs f once the browser has had a frame to lay out and a
// frame to paint. One is not enough: the first only gets as far as style and
// layout for whatever was already dirty.
func afterTwoFrames(f func()) {
	var a, b js.Func
	b = js.FuncOf(func(js.Value, []js.Value) interface{} {
		b.Release()
		f()
		return nil
	})
	a = js.FuncOf(func(js.Value, []js.Value) interface{} {
		a.Release()
		js.Global().Call("requestAnimationFrame", b)
		return nil
	})
	js.Global().Call("requestAnimationFrame", a)
}

// rackFor finds the opening a module lives in, and the rack that manages
// it. Zero values when the module is not in the frame — which is every call
// made before the frame is built, and the modules a mode hides by class.
func rackFor(key string) (*rack.Rack, js.Value) {
	for _, r := range unitRacks {
		if m := r.Module(key); m.Truthy() {
			return r, m
		}
	}
	return nil, js.Value{}
}

// rackAllModules is every module in the frame, in flat order: the units in
// order, and each opening's modules in order. This is the list that is
// saved and the list packing reads, and deriving it rather than storing it
// is what keeps the flat order and the unit placement from disagreeing.
func rackAllModules() []js.Value {
	var out []js.Value
	for _, r := range unitRacks {
		out = append(out, r.Modules()...)
	}
	return out
}

// rackOrder is the flat order, by key.
func rackOrder() []string {
	var out []string
	for _, r := range unitRacks {
		out = append(out, r.Order()...)
	}
	return out
}

// rackSetOrder puts the modules back in a saved order.
//
// Applied to the FIRST opening, and the rest follows: every module is moved
// into unit 0 in the given order and then repacked, because which unit a
// module ends up in is derived from the flat order and not stored. Doing it
// per opening instead would need the saved record to say which unit each
// module was in, and a record that says that can disagree with the order it
// also says.
func rackSetOrder(order []string) {
	if len(unitRacks) == 0 {
		return
	}
	first := unitRacks[0]
	for _, m := range rackAllModules() {
		first.Root().Call("appendChild", m)
	}
	syncUnitRacks()
	first.SetOrder(racklayout.MergeModuleOrder(order, first.Order()))
}

// rackSetHidden restores the put-away set. Each opening is given the whole
// list and keeps the keys it owns, which is what SetHidden already does
// with a key it has never heard of.
func rackSetHidden(keys []string) {
	for _, r := range unitRacks {
		r.SetHidden(keys)
	}
}

// ── A panel is milled once ────────────────────────────────────────────────

// moduleWidthHighWater is the widest each module has ever needed to be, in
// slots, at the current interface size.
//
// Reset when the interface size changes (see rackSetScale), because that
// re-mills every panel in the rack.
var moduleWidthHighWater = map[string]int{}

// latchModuleWidths stops a module from ever getting NARROWER.
//
// A module's width is quantized from what is currently on it, which means a
// control that changes how much is on it changes the width of the panel it
// is mounted on — and every module after it in the rack slides. Measured by
// turning each of the panel's 44 selector knobs in turn, seven of them move
// the rack, and the honest ones are the Matrix's step count (16 to 8), the
// Keys span (4 to 1), the view count and the focus count. Turn the sequencer
// down to eight steps and the whole rack below it re-flows.
//
// That is the metaphor breaking. A 19-inch panel is milled once, for the
// widest thing it will ever carry; a knob changes what is ON the panel, not
// how wide the panel is. Nobody's sequencer gets narrower in the rack when
// they select fewer steps.
//
// So a module keeps its high-water width. It grows when it genuinely needs
// to — the first time a mode puts more on it — and never shrinks back, so
// the shift happens at most once per module per session instead of on every
// turn of the knob, and turning the knob back does not move anything at all.
func latchModuleWidths() {
	f := rackFrame()
	if !f.Truthy() {
		return
	}
	els := f.Call("querySelectorAll", ".sect")
	for i := 0; i < els.Get("length").Int(); i++ {
		m := els.Index(i)
		key := moduleKeyOf(m)
		if key == "" || isHiddenModule(m) {
			continue
		}
		n := moduleSlots(m)
		if was := moduleWidthHighWater[key]; n < was {
			m.Get("style").Set("width", pxStr(slotsWidthPx(was)))
			continue
		}
		moduleWidthHighWater[key] = n
	}
}

// isHiddenModule reports whether a module is switched out, in which case its
// width is meaningless and must not be latched.
func isHiddenModule(m js.Value) bool {
	return m.Get("style").Get("display").String() == "none"
}

// slotsWidthPx is what rack-go makes an n-slot module: n panels and the n-1
// seams between them. The seam is unscaled there, so it is unscaled here —
// the two have to agree or a latched module is a different size from a
// quantized one.
func slotsWidthPx(n int) float64 {
	if n < 1 {
		n = 1
	}
	return float64(n)*moduleSlot*layout.scale + float64(n-1)*moduleGap
}

// fitModulesToTheirParts widens a module that carries a control bigger than
// the column it is mounted in.
//
// A module's width is quantized from its content, and its content is a grid
// of 29 mm columns. A legend ring is not: it is sized from the legends
// engraved on it (see pkg/skirt), and a rotary with long enough ones needs a
// ring wider than the column its knob sits in. The ring is absolutely
// positioned, so it adds nothing to max-content — the panel is milled to the
// column, and the ring draws past its edge and is clipped away by .sect's
// overflow:hidden. Model Out's source ring is 150 px across on a 140 px
// panel: five pixels of the legend were simply not there.
//
// A panel is at least as wide as the widest part bolted to it. skirt.Fit
// tries the two cheaper answers first — a smaller grip, then smaller
// lettering — and Model Out's off/CAM/XY/XZ/YZ ring is the one that spends
// both and still overhangs. Past that the honest remedy is a bigger panel,
// which is the same answer the scope tube gets.
//
// One minimum per MODULE and not per cell, because only the module that
// cannot contain its own ring has to grow: widening the CELL would widen a
// whole column of a multi-column module to suit one control in it.
//
// Reports whether any minimum changed, because a ring that has only just
// been measured can want a panel the pass before it did not know about.
func fitModulesToTheirParts() bool {
	f := rackFrame()
	if !f.Truthy() {
		return false
	}
	// Measured, decided and written in three phases where the page allows
	// it — see fitModulesFast. The loop below alternates a read with a
	// write per module, which is what made this expensive.
	if h := fastDOM(); h.Truthy() {
		if changed, ok := fitModulesFast(h, f); ok {
			return changed
		}
	}
	changed := false
	els := f.Call("querySelectorAll", ".sect")
	for i := 0; i < els.Get("length").Int(); i++ {
		m := els.Index(i)
		if isHiddenModule(m) {
			continue
		}
		widest := 0.0
		ds := m.Call("querySelectorAll", ".knob-dial")
		for j := 0; j < ds.Get("length").Int(); j++ {
			if w := ds.Index(j).Get("offsetWidth").Float(); w > widest {
				widest = w
			}
		}
		// One slot is the floor already — .sect carries min-width:--mod-w
		// — so a module that fits says nothing, rather than saying the
		// same thing twice in two places that can drift apart.
		want := ""
		if n := slotsForWidthPx(widest + moduleEdgePx(m)); widest > 0 && n > 1 {
			want = pxStr(slotsWidthPx(n))
		}
		if m.Get("style").Get("minWidth").String() != want {
			m.Get("style").Set("minWidth", want)
			changed = true
		}
	}
	return changed
}

// moduleEdgePx is what a ring may not reach past: the panel's own border.
//
// The PADDING is deliberately not counted. A legend ring is allowed to
// reach a little way past the cell it is centered in — skirtCellGapPx, and
// the note there says in as many words that the module's padding is what
// absorbs it. Charging a ring for padding it is entitled to use milled the
// Grid module two slots wide for a ring eight tenths of a pixel over.
func moduleEdgePx(m js.Value) float64 {
	cs := js.Global().Call("getComputedStyle", m)
	return parsePx(cs.Get("borderLeftWidth").String()) +
		parsePx(cs.Get("borderRightWidth").String())
}

// slotsForWidthPx is the narrowest panel that holds w. A whole number of
// slots, because a module is milled to the rack's pitch and there is no such
// thing as a slot and a half; and never wider than a bay, because a module
// that does not fit the rack is a different problem from a module that needs
// a second slot.
func slotsForWidthPx(w float64) int {
	n := 1
	for slotsWidthPx(n) < w && n < racksurface.UnitCapacity() {
		n++
	}
	return n
}
