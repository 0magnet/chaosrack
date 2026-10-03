//go:build js && wasm

package attractor

import (
	"regexp"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rackspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Designators: every control on the rack has an address, bay.column.row.
//
//   - bay is the number on the bay's left ear;
//   - column is the slot of the bay the control stands in, counted from the
//     bay's left: a bay is twelve of the narrowest module wide, so 1 to 12;
//   - row is the bay's row of controls it is in, 1 to 3.
//
// So the control to the right of 1.3.1 is 1.4.1, whatever module either is
// in, and a module is addressed by its first column (the Visual monitor is
// two slots, 1.1, so the bank beside it is 1.3). An address is a place, as a
// grid reference on a drawing is: an empty cell keeps its number. Two or
// more controls in one cell — a knob and the readout over it, a column of
// switches — are lettered, top to bottom and then left to right: 3.2.1.a,
// 3.2.1.b.
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
	pending  bool
	fn       js.Func
	idleOpts js.Value // {timeout: designateWithinMs}, made once
}

// scheduleDesignate readdresses the rack once the page is idle. Called by
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
	// When the page is idle, and within half a second: the pass measures
	// every control on the rack, a sixth of a second, and on the next frame
	// it was that frame -- the first one a new model or a fresh rack draws.
	// Addresses are for reading, and nothing reads them that fast.
	if ric := js.Global().Get("requestIdleCallback"); ric.Type() == js.TypeFunction {
		if designateState.idleOpts.IsUndefined() {
			designateState.idleOpts = js.ValueOf(map[string]any{"timeout": designateWithinMs})
		}
		js.Global().Call("requestIdleCallback", designateState.fn, designateState.idleOpts)
		return
	}
	js.Global().Call("requestAnimationFrame", designateState.fn)
}

// designateWithinMs is how long the address pass may wait for an idle
// moment.
const designateWithinMs = 500

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
