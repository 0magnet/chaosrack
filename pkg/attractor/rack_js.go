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
		Scale:            panelScale,
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
	return modulePinnedFrom(false, r.Hidden(key),
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
	if ensureRack() == nil {
		return
	}
	// Every opening, then repack: a module's slot count is the input to
	// packing, so it has to be settled before anything is moved.
	for _, r := range unitRacks {
		r.Quantize()
	}
	relayoutUnits()
	// The widths changed, so the modules a unit holds may have; quantize
	// the openings that now exist.
	for _, r := range unitRacks {
		r.Quantize()
	}
	layoutRackHandles()
	// Skirts are MEASURED, so they are sized after the layout that gives
	// them a size. Here because this is the one funnel every layout
	// change already passes through: a rebuild, a resize, a reorder, an
	// interface-scale change. A skirt sized against a detached or
	// zero-width knob is a skirt that never gets sized at all.
	layoutSkirts()
}

// applyModuleVisibility puts away what the switches say to put away, and leaves
// everything else alone. Called after a rebuild, which replaces the elements the
// last pass acted on.
func applyModuleVisibility() {
	if ensureRack() != nil {
		for _, r := range unitRacks {
			r.Apply()
		}
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
// the time it is asked on a warm cache.
func requantizeAfterFonts() {
	fonts := doc.Get("fonts")
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
		quantizeModuleWidths()
		return nil
	})
	ready.Call("then", fn)
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
	first.SetOrder(mergeModuleOrder(order, first.Order()))
}

// rackSetHidden restores the put-away set. Each opening is given the whole
// list and keeps the keys it owns, which is what SetHidden already does
// with a key it has never heard of.
func rackSetHidden(keys []string) {
	for _, r := range unitRacks {
		r.SetHidden(keys)
	}
}
