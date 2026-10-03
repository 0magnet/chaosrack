//go:build js && wasm

package attractor

// Reading and writing the rack layout, which is the only part of it that needs
// a browser. The record itself, and the ordering arithmetic, are in
// racklayout.go where they can be tested without one.

import (
	"github.com/0magnet/chaosrack/pkg/racklayout"
)

// readRackLayout loads the saved arrangement, or an empty one.
func readRackLayout() racklayout.Layout {
	v, ok := lsGet(racklayout.LayoutKey)
	if !ok {
		return racklayout.Layout{}
	}
	return racklayout.Decode(v)
}

// saveRackLayout writes the arrangement as it stands.
//
// Called from the rack's own OnReorder / OnVisibility callbacks, so every way
// a module can move ends up here and there is no arrangement that is only
// half-remembered.
func saveRackLayout() {
	r := ensureRack()
	if r == nil {
		return
	}
	// Hidden is left empty on purpose. Nothing takes a module out of the
	// rack now, so there is nothing to write — and writing it would be a
	// record of a state the panel can no longer be in.
	l := racklayout.Layout{Order: rackOrder()}
	lsSet(racklayout.LayoutKey, l.Encode())
}

// restoreRackLayout puts the modules back where they were.
//
// Must run BEFORE buildModuleSwitches: the rack builds each switch checked or
// not from its own hidden set, so a module restored as hidden afterward would
// come back with its switch saying it was in.
func restoreRackLayout() {
	r := ensureRack()
	if r == nil {
		return
	}
	l := readRackLayout()
	if len(l.Order) > 0 {
		rackSetOrder(l.Order)
	}
	// The saved Hidden set is deliberately IGNORED, not merely no longer
	// written. A record from a build that had module switches names modules
	// this one cannot bring back: honoring it would take a module out of the
	// rack permanently, on this load and every later one, with no control
	// anywhere that could undo it. The order is still worth restoring; what
	// was put away is not.
	// Only where something is put away: SetHidden ends in a quantize, a
	// full layout, and on a rack with nothing hidden it would change nothing.
	for _, u := range unitRacks {
		if len(u.HiddenKeys()) > 0 {
			rackSetHidden(nil)
			break
		}
	}
}
