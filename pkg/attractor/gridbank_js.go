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

// buildGridBank makes the Grid's cells P-units. Called once the fixed knobs
// are made (FROM and TO are knobifyFixed's) and before the dials are filled.
func buildGridBank() {
	buildPUnitModule("grid-bank")
	// The one dial nothing rebuilds: mounted here, as the others are by
	// their own builds.
	if sel, h := dom.Doc.Call("getElementById", "view-n"), dom.Doc.Call("getElementById", "view-n-stack"); sel.Truthy() && h.Truthy() {
		selectorReadout(h, sel, viewCountRing)
	}
}

// buildPUnitModule makes every cell of the P-unit grid with id id, written
// in the markup as a label and its control, a P-unit: a number gets the step
// readout and its knob, then each is finished as a bank's position is.
func buildPUnitModule(id string) {
	grid := dom.Doc.Call("getElementById", id)
	if !grid.Truthy() {
		return
	}
	cs := grid.Call("querySelectorAll", ":scope>.punit")
	for i := range cs.Length() {
		c := cs.Index(i)
		withStepField(c)
		bankPosition(c)
	}
}

// withStepField gives a markup cell with a numeric control its step readout
// and knob, if it has none: what buildParamUnit gives a parameter's.
func withStepField(c js.Value) {
	sl := c.Call("querySelector", "input[type=range][id]")
	if !sl.Truthy() || c.Call("querySelector", ".u-step").Truthy() || c.Call("querySelector", "select").Truthy() {
		return
	}
	label := ""
	if l := c.Call("querySelector", ".u-lbl"); l.Truthy() {
		label = l.Get("textContent").String()
	}
	c.Call("appendChild", buildStepField(sl, label, specOf(sl).stepText()))
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
	dom.Fire(sel, "change")
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
	// GRID: T tiles the cells side by side, O overlays them, one over
	// another on the whole screen. One at a time.
	trioPrograms["view-n"] = trioProgram{
		keys: []string{"T", "O", ""},
		help: []string{doc("trio.view-n=0"), doc("trio.view-n=1")},
		press: func(i int) {
			if i < 2 {
				setSwitch("grid-ovl", i == 1)
			}
		},
		lit: func() int {
			if checkedOn("grid-ovl") {
				return 1
			}
			return 0
		},
		drive: []string{"grid-ovl"},
	}
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
				dom.Fire(sw, "change")
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
