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

var panelRack *rack.Rack

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
	if panelRack != nil {
		return panelRack
	}
	container := doc.Call("querySelector", ".modules")
	if !container.Truthy() {
		return nil
	}
	panelRack = rack.New(rack.Options{
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
		OnReorder:    func([]string) { saveRackLayout() },
		OnVisibility: func(string, bool) { saveRackLayout() },
	})
	return panelRack
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
	r := panelRack
	return modulePinnedFrom(false, r.Hidden(key),
		r.Module(key).Get("style").Get("display").String() == "none")
}

// moduleNeverSwitched names the modules that can have no switch whatever their
// current display: the Console, because the switches live in it, and the three
// that a MODE shows and hides by a class on the panel. A switch contradicting
// the mode is only a way to get stuck.
func moduleNeverSwitched(key string) bool {
	r := panelRack
	if r == nil {
		return true
	}
	m := r.Module(key)
	if !m.Truthy() {
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
	if r := ensureRack(); r != nil {
		r.Quantize()
	}
	// The rows may have re-wrapped, and the rack handles are drawn per row.
	// This is the one place every layout change passes through.
	layoutRackHandles()
}

// buildModuleSwitches fills the Console's Modules section with one switch per
// module.
func buildModuleSwitches() {
	r := ensureRack()
	if r == nil {
		return
	}
	host := doc.Call("getElementById", "module-switches")
	if !host.Truthy() {
		return
	}
	r.Switches(host)
}

// applyModuleVisibility puts away what the switches say to put away, and leaves
// everything else alone. Called after a rebuild, which replaces the elements the
// last pass acted on.
func applyModuleVisibility() {
	if r := ensureRack(); r != nil {
		r.Apply()
	}
	layoutRackHandles()
}

// wireModuleDrag exists for the one call site that turns dragging on. The rack
// wires it when it is built, so this only has to make sure it has been.
func wireModuleDrag() { ensureRack() }

// rackSetScale passes the interface scale through to the rack, which sizes its
// slots by it. The panel's own --kscale still drives the CSS; this is the same
// number told to the thing that does the arithmetic.
func rackSetScale(v float64) {
	if r := ensureRack(); r != nil {
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
