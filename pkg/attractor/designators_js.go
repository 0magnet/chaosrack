//go:build js && wasm

package attractor

import (
	"regexp"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// Designators: every control on the rack has an address, bay.row.slot.
//
//   - bay is the number on the bay's left ear;
//   - row counts the rows of controls down the module the control is in;
//   - slot is where the control's center is, 1 to 12 across the bay.
//
// So 3.2.4 is the second row down, in the fourth slot of bay 3, and two
// controls sharing a row and a slot are 3.2.4a and 3.2.4b, left to right.
// The slot is the bay's rather than the module's so that an address does
// not depend on where one module ends and the next begins; the row is the
// module's because rows are what a module's own layout makes, and counting
// them across a bay of differently laid-out modules would be counting
// something nobody can see.
//
// The point is to name a control in a sentence. It is in every tooltip, and
// uitool reads it back (data-loc), so an address in a message is a control
// that can be found. Not printed on the panel: a mark over every control was
// clutter, and a real panel does not carry its service manual's part numbers.
//
// Measured from the page after the rack is packed, not planned in Go: an
// address is where the control IS, and the page is the only thing that knows
// that. One crossing (see fastdom_js.go), at most once a frame.

var designateState struct {
	pending bool
	fn      js.Func
}

// scheduleDesignate readdresses the rack on the next frame. Called by
// anything that moves controls, shows or hides them, or rewrites their
// tooltips; several calls in one frame are one pass.
func scheduleDesignate() {
	if designateState.pending || !dom.Doc.Truthy() {
		return
	}
	designateState.pending = true
	if designateState.fn.IsUndefined() {
		// For the life of the page, and so js.FuncOf rather than dom.FuncOf:
		// made during a panel build, the arena would free it with the panel.
		designateState.fn = js.FuncOf(func(js.Value, []js.Value) any {
			designateState.pending = false
			designate()
			return nil
		})
	}
	js.Global().Call("requestAnimationFrame", designateState.fn)
}

// addressPrefix is what designate puts in front of a tooltip.
var addressPrefix = regexp.MustCompile(`^(?:[0-9S]+\.\d+\.\d+[a-z]? · )+`)

// stripAddress is a tooltip without the address designate gave it, for code
// that builds one tooltip out of another element's.
func stripAddress(title string) string { return addressPrefix.ReplaceAllString(title, "") }

// designate addresses every control on the rack now.
func designate() {
	h, f := fastDOM(), rackFrame()
	if !h.Truthy() || !f.Truthy() {
		return
	}
	h.Call("designate", f, (moduleSlot+moduleGap)*layout.scale)
}
