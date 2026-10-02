//go:build js && wasm

package attractor

// The bank's cells, built when their model is first shown rather than all at
// once. The bank holds every model's constants — seventy models, a few
// thousand cells, two thirds of every element on the page — and building
// them all at boot, then carrying them hidden through every style and layout
// pass after, was most of the rack's start.
//
// A model's own cells start as placeholders: one empty element each, which
// stands in the bank's grid where the cell will (the bank's plan only counts
// positions), and says how many characters its name takes (bankChars). The
// first time the model is shown (syncBankCells), each is replaced by the
// cell, built from the parameter's value as it is then — which is where a
// model's settings live while another is showing — and given what every
// bank cell gets (bankPosition). Shared cells, steps and the first model are
// built at once: the bank's unassigned positions are copies of one of them.

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// bankLazy is each model's placeholders not built yet, with how to build
// each, in the order they were made.
var bankLazy = map[string][]bankLazyCell{}

type bankLazyCell struct {
	ph    js.Value
	build func() js.Value
}

// bankPlaceholder stands in for the cell of model mode's parameter p.
func bankPlaceholder(mode string, p paramDef) js.Value {
	ph := dom.Doc.Call("createElement", "div")
	ph.Set("className", "punit bankwait")
	// The name its display will need: its label, or the longest a selector's
	// positions take, which is the bank's widest.
	n := len([]rune(p.Label))
	if len(paramLabels[p.ID]) > 0 {
		n = bankMaxChars
	}
	ph.Call("setAttribute", "data-chars", strconv.Itoa(n))
	bankLazy[mode] = append(bankLazy[mode], bankLazyCell{ph, func() js.Value { return newPUnit(mode, p) }})
	return ph
}

// bankIsPlaceholder reports whether c is a placeholder.
func bankIsPlaceholder(c js.Value) bool {
	return c.Get("classList").Call("contains", "bankwait").Bool()
}

// bankMaterialize builds model's cells in place of their placeholders, if
// they are not built yet.
func bankMaterialize(model string) {
	lazy := bankLazy[model]
	if len(lazy) == 0 {
		return
	}
	delete(bankLazy, model)
	for _, l := range lazy {
		c := l.build()
		c.Get("classList").Call("add", "bankcell")
		c.Call("setAttribute", "data-bank", model)
		// Where the placeholder stood, and whether it was showing.
		ps, cs := l.ph.Get("style"), c.Get("style")
		cs.Set("gridRow", ps.Get("gridRow"))
		cs.Set("gridColumn", ps.Get("gridColumn"))
		cs.Set("display", ps.Get("display"))
		if parent := l.ph.Get("parentNode"); parent.Truthy() {
			parent.Call("replaceChild", c, l.ph)
		}
		bankPosition(c)
	}
	wireTrios()
	syncTrios()
}
