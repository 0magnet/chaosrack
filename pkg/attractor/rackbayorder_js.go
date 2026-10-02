//go:build js && wasm

package attractor

// Moving a bay by its screws: a click on either of a bay's top screws moves
// it up a place, on a bottom screw down one, and the top bay pushed up goes
// to the bottom, the bottom one pushed down to the top (bayorder.go). The
// order is kept in this browser, and packing applies it (arrangeUnits), so
// every address on a moved bay follows it.
//
// Only while the metalwork is drawn (Console ▸ Rack bay): without it the
// screws are not there to be seen, and a click on bare panel edge moving
// the rack would be a trap.

import (
	"slices"
	"strings"
	"syscall/js"
)

const bayOrderKey = "wasmstuff-bayorder"

// bayOrderNow is the bays as packing last laid them out, by key, top first.
var bayOrderNow []string

// savedBayOrder is the order the screws left, or nothing.
func savedBayOrder() []string {
	v, ok := lsGet(bayOrderKey)
	if !ok || v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// arrangeUnits puts the packed bays, units, in the saved order.
func arrangeUnits(items []packItem, units [][]int) [][]int {
	secs := make([]string, len(units))
	for i, idx := range units {
		secs[i] = unitSection(items, idx)
	}
	keys := bayKeys(secs)
	order := arrangeBays(keys, savedBayOrder())
	out := make([][]int, 0, len(units))
	for _, k := range order {
		out = append(out, units[slices.Index(keys, k)])
	}
	bayOrderNow = order
	return out
}

// bayScrewsWired is set once the frame listens for its screws.
var bayScrewsWired bool

// wireBayScrews has the frame f move a bay when one of its screws is clicked.
func wireBayScrews(f js.Value) {
	if bayScrewsWired {
		return
	}
	bayScrewsWired = true
	f.Call("addEventListener", "click", js.FuncOf(func(_ js.Value, a []js.Value) any {
		t := a[0].Get("target")
		if !f.Get("classList").Call("contains", "with-bay").Bool() ||
			!t.Call("matches", "."+unitEarCls+" > i:not(.ear-mid)").Bool() {
			return nil
		}
		dir := 1 // a bottom screw
		if t.Equal(t.Get("parentNode").Get("firstElementChild")) {
			dir = -1
		}
		open := t.Call("closest", "."+unitClass).Call("querySelector", "."+unitOpenCls)
		i := slices.IndexFunc(unitOpenings(), func(o js.Value) bool { return o.Equal(open) })
		if i < 0 || i >= len(bayOrderNow) {
			return nil
		}
		moved := bayOrderNow[i]
		lsSet(bayOrderKey, strings.Join(moveBay(bayOrderNow, i, dir), ","))
		relayoutUnits()
		layoutRackHandles()
		lastRack.sig = fastDOM().Call("rackSig").String()
		// The bay that moved, kept in view: it is where the hand is.
		if at := slices.Index(bayOrderNow, moved); at >= 0 {
			if opens := unitOpenings(); at < len(opens) {
				opens[at].Call("closest", "."+unitClass).Call("scrollIntoView", map[string]any{"block": "nearest"})
			}
		}
		return nil
	}))
}
