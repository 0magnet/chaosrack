//go:build js && wasm

package attractor

// Jam / attract mode (Motion > Jam): the app performs itself — a light
// in-app port of the chaos-monkey demo choreography. Every 12–20 seconds it
// hops to a random flow mode; hops sometimes flip persist on (the CRT paint
// look), and always set fresh gentle spin rates. Everything goes through the
// real controls (select dispatch, slider input events) so the panel, the
// permalink and every registry behavior stay truthful. Speaker-output
// switches are never touched.

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

var (
	jamOn   bool
	jamNext float64 // ms timestamp of the next hop
)

func jamRand() float64 { return js.Global().Get("Math").Call("random").Float() }

// jamTick runs from the render loop; cheap no-op unless armed and due, and
// while the power is off: the loop goes on then, for the scopes, and hops
// nobody could see were rewriting the model, the spin and the link.
func jamTick(nowMs float64) {
	if !jamOn || run.stopped || nowMs < jamNext {
		return
	}
	jamNext = nowMs + JamInterval(jamRand)
	jamHop()
}

func jamHop() {
	// The hop itself is jam.go's, which a native rack plays too; here it is
	// played through the real controls.
	next, persist, y, x, ok := JamHop(jamRand, run.selectedMode)
	if !ok {
		return
	}
	if sel := dom.Doc.Call("getElementById", "mode-select"); sel.Truthy() {
		sel.Set("value", next)
		dom.Fire(sel, "change")
	}
	if p := dom.Doc.Call("getElementById", "persist-trail"); p.Truthy() {
		p.Set("checked", persist)
		dom.Fire(p, "change")
	}
	for axis, v := range map[string]float64{"y": y, "x": x} {
		if sl := dom.Doc.Call("getElementById", "rotation-controls-"+axis); sl.Truthy() {
			sl.Set("value", strconv.FormatFloat(v, 'f', 1, 64))
			dom.Fire(sl, "input")
		}
	}
	syncKnobs()
}

func wireJamSwitch() {
	wireSwitch("jam-sw", func(on bool) {
		jamOn = on
		jamNext = 0 // first hop on the next frame
	})
}
