//go:build js && wasm

package attractor

// The Grid module, built from P-units: the one part every position of a
// model bank is (rackbank_js.go), so the Grid reads and turns as bay 1 does —
// the legend display, the setting's display (a click on it lists them all),
// the LED ring, the step and reset, and the three buttons.
//
// The controls under them are the Grid's own, by the ids they always had, so
// the permalink, MIDI and the sweep code drive them as before. The sweep and
// focus dials are rebuilt with the model and the grid; each mounts its knob in
// its P-unit's .knobhold (selectorReadout).

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// gridSweepStep is FROM and TO's step, as their markup and ControlDescs say.
const gridSweepStep = "0.01"

// buildGridBank makes the Grid's cells P-units. Called once the fixed knobs
// are made (FROM and TO are knobifyFixed's) and before the dials are filled.
func buildGridBank() {
	bank := dom.Doc.Call("getElementById", "grid-bank")
	if !bank.Truthy() {
		return
	}
	for _, id := range []string{"sweep-lo", "sweep-hi"} {
		sl := dom.Doc.Call("getElementById", id)
		if !sl.Truthy() {
			continue
		}
		c := sl.Call("closest", ".punit")
		label := ""
		if l := c.Call("querySelector", ".u-lbl"); l.Truthy() {
			label = l.Get("textContent").String()
		}
		// The step the markup gives, not the slider's now: the fine ring has
		// divided that.
		c.Call("appendChild", buildStepField(sl, label, gridSweepStep))
	}
	cs := bank.Call("querySelectorAll", ":scope>.punit")
	for i := range cs.Length() {
		bankPosition(cs.Index(i))
	}
	// The one dial nothing rebuilds: mounted here, as the others are by
	// their own builds.
	if sel, h := dom.Doc.Call("getElementById", "view-n"), dom.Doc.Call("getElementById", "view-n-stack"); sel.Truthy() && h.Truthy() {
		selectorReadout(h, sel, viewCountRing)
	}
}

// stepSelect moves the select id by d positions, or to position 0 when d is
// zero, as its knob would: through its change event.
func stepSelect(id string, d int) {
	sel := dom.Doc.Call("getElementById", id)
	if !sel.Truthy() {
		return
	}
	n := sel.Get("options").Get("length").Int()
	i := 0
	if d != 0 {
		i = clampIndex(sel.Get("selectedIndex").Int()+d, n)
	}
	if i == sel.Get("selectedIndex").Int() {
		return
	}
	sel.Set("selectedIndex", i)
	sel.Call("dispatchEvent", js.Global().Get("Event").New("change", map[string]any{"bubbles": true}))
}

// selectAt is the position select id is at.
func selectAt(id string) int {
	if sel := dom.Doc.Call("getElementById", id); sel.Truthy() {
		return sel.Get("selectedIndex").Int()
	}
	return -1
}

// gridTrio is the three buttons of a selector P-unit on the Grid: + the next
// position, 0 the first, − the one before, lit on 0 when it is there.
func gridTrio(id, key string) trioProgram {
	return trioProgram{
		help: []string{doc(key + "=0"), doc(key + "=1"), doc(key + "=2")},
		press: func(i int) {
			stepSelect(id, []int{1, 0, -1}[i])
		},
		lit: func() int {
			if selectAt(id) == 0 {
				return 1
			}
			return -1
		},
		drive: []string{id},
	}
}

func init() {
	trioPrograms["view-n"] = gridTrio("view-n", "trio.view-n")
	trioPrograms["sweep-p"] = gridTrio("sweep-p", "trio.sweep-p")
	trioPrograms["sweep2-p"] = gridTrio("sweep2-p", "trio.sweep-p")
	// Focus's 0 is Link: every cell on one set of knobs, which is what
	// focus is not. + and − take the knobs to the next cell or the one
	// before, which unlinks them — a focus is only followed unlinked.
	trioPrograms["focus-n"] = trioProgram{
		help: []string{doc("trio.focus-n=0"), doc("trio.focus-n=1"), doc("trio.focus-n=2")},
		press: func(i int) {
			sw := dom.Doc.Call("getElementById", "link-sw")
			if !sw.Truthy() {
				return
			}
			link := i == 1
			if sw.Get("checked").Bool() != link {
				sw.Set("checked", link)
				sw.Call("dispatchEvent", js.Global().Get("Event").New("change", map[string]any{"bubbles": true}))
			}
			if !link {
				stepSelect("focus-n", []int{1, 0, -1}[i])
			}
		},
		lit: func() int {
			if sw := dom.Doc.Call("getElementById", "link-sw"); sw.Truthy() && sw.Get("checked").Bool() {
				return 1
			}
			return -1
		},
		drive: []string{"link-sw", "focus-n"},
	}
}
