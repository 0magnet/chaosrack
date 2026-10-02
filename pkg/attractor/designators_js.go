//go:build js && wasm

package attractor

import (
	"regexp"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rackspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Designators: every control on the rack has an address,
// bay.module.position.
//
//   - bay is the number on the bay's left ear;
//   - module is the module's place in the bay, 1 from the left;
//   - position is the cell of the module's grid the control stands in, a
//     slot across and a row down, numbered down each column and then across
//     whether or not the cells before it hold anything — the bay's three
//     rows are 1 to 3 in the first column, 4 to 6 in the second.
//
// So 3.2.4 is the top of the second column of bay 3's second module, and
// 3.2 the module itself. An address is a place, as a part number on a
// drawing is: a module laid out differently keeps its positions, and an
// empty one keeps its number. Two or more controls in one position — a knob
// and the readout over it, a column of switches — are lettered, top to
// bottom and then left to right: 3.2.4.a, 3.2.4.b.
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
var addressPrefix = regexp.MustCompile(`^(?:[0-9S]+\.\d+(?:\.\d+(?:\.[a-z])?)? · )+`)

// stripAddress is a tooltip without the address designate gave it, for code
// that builds one tooltip out of another element's.
func stripAddress(title string) string { return addressPrefix.ReplaceAllString(title, "") }

// designate addresses every control on the rack now.
func designate() {
	h, f := fastDOM(), rackFrame()
	if !h.Truthy() || !f.Truthy() {
		return
	}
	h.Call("designate", f, (moduleSlot+moduleGap)*layout.scale,
		racksurface.UnitCapacity(), moduleGap/(moduleSlot+moduleGap), rackspec.RowsPerPanel())
	// The manual lists controls by address, so an open one is rewritten
	// with the addresses as they now are.
	if manualShown {
		manualRefresh()
	}
	if infoShown() {
		infoManualKey = ""
		updateInfoOverlay()
	}
}
