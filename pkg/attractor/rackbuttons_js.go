//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/conway"
	"github.com/0magnet/chaosrack/pkg/dom"
)

// ── Three buttons beside every bank knob ──────────────────────────────────
//
// A knob is for a range. A setting with two or three states is not one, and
// a whole bank position spent on it is a knob that is really a switch — the
// globe had two of them, par (rings or a spiral) and dir (which way the
// spiral winds), and dir did nothing at all unless par was on spiral.
//
// So every position carries a column of three small lit buttons against its
// right-hand rule, printed + 0 −, the way its legend stands against the left
// one. They are the same part on every position, and like the knob they are
// programmed by the model: on the globe's lat they are spiral one way, rings,
// spiral the other. Where a model gives a position no such setting they are
// there and dark, and pressing them does nothing — the part does not come and
// go with the model, which is the bank's whole rule.

// trioKeys are the three buttons' printed legends, top to bottom.
var trioKeys = [3]string{"+", "0", "−"}

// trioProgram is what a column of buttons does for one parameter: the bank's
// three beside every position, or a control's own few (the scope's SLOPE and
// MODE beside its TRIG and TIME knobs).
type trioProgram struct {
	keys  []string    // the printed legends, top to bottom; trioKeys when nil
	help  []string    // what each button does, for its tooltip
	press func(i int) // i indexes the legends
	lit   func() int  // which button is on, or -1
	drive []string    // the hidden controls it sets, so a change to them relights it
}

// setParamSlider sets a parameter through its own control, the way the knob
// would: reset, the permalink and MIDI keep seeing one control per parameter.
func setParamSlider(id string, v float64) {
	s := dom.Doc.Call("getElementById", id)
	if !s.Truthy() {
		return
	}
	s.Set("value", strconv.FormatFloat(v, 'f', -1, 64))
	s.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

// trioPrograms are keyed by the parameter of the knob the buttons stand
// beside.
var trioPrograms = map[string]trioProgram{
	// The globe's parallels: + and − wind one spiral from pole to pole, each
	// way round; 0 draws them as separate rings. lat is how many either way.
	"globe-lat": {
		help: []string{
			doc("p.globe-lat=0"),
			doc("p.globe-lat=1"),
			doc("p.globe-lat=2"),
		},
		press: func(i int) {
			switch i {
			case 0:
				setParamSlider("globe-par", 1)
				setParamSlider("globe-rev", 0)
			case 1:
				setParamSlider("globe-par", 0)
			case 2:
				setParamSlider("globe-par", 1)
				setParamSlider("globe-rev", 1)
			}
		},
		lit: func() int {
			switch {
			case !globe.spiral():
				return 1
			case globe.revF >= 0.5:
				return 2
			default:
				return 0
			}
		},
		drive: []string{"globe-par", "globe-rev"},
	},
}

// The Polyhedron's (conway_js.go). Set in init rather than in the table
// above because they read the model's own knobs, and those are package
// variables the table's initializer would otherwise have to be ordered after.
func init() {
	// MORPH's named solids are stops along it; + and − walk to the next one
	// either way, 0 goes back to the solid. The knob still reaches everything
	// in between.
	trioPrograms["polyhedron-morph"] = trioProgram{
		help: []string{
			"the next named solid along the morph: truncated, rectified, the dual's truncation, the dual",
			"back to the solid itself",
			"the previous named solid along the morph",
		},
		press: func(i int) {
			m, v := float64(poly.morphF), 0.0
			switch i {
			case 0:
				v = conway.MorphStops[len(conway.MorphStops)-1]
				for _, s := range conway.MorphStops {
					if s > m+1e-3 {
						v = s
						break
					}
				}
			case 2:
				for _, s := range conway.MorphStops {
					if s < m-1e-3 {
						v = s
					}
				}
			}
			setParamSlider("polyhedron-morph", v)
		},
		lit: func() int {
			if poly.morphF < 1e-3 {
				return 1
			}
			return -1
		},
	}
	// KIS: pyramids out, none, pyramids in — Conway's own height either way.
	trioPrograms["polyhedron-kis"] = trioProgram{
		help: []string{
			"raise a pyramid on every face",
			"flat faces, no pyramids",
			"press a pyramid into every face",
		},
		press: func(i int) { setParamSlider("polyhedron-kis", []float64{0.25, 0, -0.25}[i]) },
		lit: func() int {
			switch {
			case poly.kisF > 1e-3:
				return 0
			case poly.kisF < -1e-3:
				return 2
			default:
				return 1
			}
		},
	}
}

// bankHidden are parameters another position's buttons set. They keep their
// control, hidden, so they are still one control each to everything that
// reads controls, but they are not a position of their own.
var bankHidden = func() map[string]bool {
	out := map[string]bool{}
	for _, p := range trioPrograms {
		for _, id := range p.drive {
			out[id] = true
		}
	}
	return out
}()

// trioColumn is the part: a column of lit buttons, one per legend. The
// legend is on the button, and is what lights: a label over each button
// took as much room again as the button, and left it too small to press.
func trioColumn(keys []string) js.Value {
	col := dom.Doc.Call("createElement", "span")
	col.Set("className", "trio")
	for i, k := range keys {
		btn := dom.Doc.Call("createElement", "button")
		btn.Set("className", "trio-btn")
		btn.Call("setAttribute", "data-trio", strconv.Itoa(i))
		lamp := dom.Doc.Call("createElement", "i")
		lamp.Set("className", "trio-lamp")
		lamp.Set("textContent", k)
		btn.Call("appendChild", lamp)
		col.Call("appendChild", btn)
	}
	return col
}

// trioOf is the program for the position a button is in, if it has one.
func trioOf(el js.Value) (trioProgram, bool) {
	c := el.Call("closest", "[data-param]")
	if !c.Truthy() {
		return trioProgram{}, false
	}
	p, ok := trioPrograms[c.Call("getAttribute", "data-param").String()]
	return p, ok
}

// lightTrios relights the buttons of one parameter's positions, and nothing
// else: a press is answered by its own column, not by a pass over the rack.
func lightTrios(param string) {
	p, ok := trioPrograms[param]
	if !ok {
		return
	}
	on := p.lit()
	btns := dom.Doc.Call("querySelectorAll", `[data-param="`+param+`"] .trio-btn`)
	for j := range btns.Length() {
		b := btns.Index(j)
		i, _ := strconv.Atoi(b.Call("getAttribute", "data-trio").String()) //nolint:errcheck // set from an int by trioColumn
		b.Get("classList").Call("toggle", "trio-on", i == on)
	}
}

// syncTrios lights each position's buttons for what its setting is now, and
// says in each tooltip what the button does there, or that it does nothing.
// A pass over every position on the rack, so it runs when what the positions
// ARE changes (a model change, a build) and never on a press: run on every
// press, it rewrote every tooltip and set off a re-address of the whole rack,
// which is a forced layout, and the model stuttered each time a button that
// does nothing was pressed.
func syncTrios() { syncTriosIn(dom.Doc, ".trio") }

// syncTriosIn relights the trios under root that sel matches.
func syncTriosIn(root js.Value, sel string) {
	if !dom.Doc.Truthy() {
		return
	}
	cols := root.Call("querySelectorAll", sel)
	for i := range cols.Length() {
		col := cols.Index(i)
		p, ok := trioOf(col)
		on, keys := -1, trioKeys[:]
		if ok {
			on, keys = p.lit(), p.legends()
		}
		col.Get("classList").Call("toggle", "trio-live", ok)
		btns := col.Call("querySelectorAll", ".trio-btn")
		for j := range btns.Length() {
			b := btns.Index(j)
			b.Get("classList").Call("toggle", "trio-on", j == on)
			if j >= len(keys) || (ok && j >= len(p.help)) {
				continue
			}
			tip := keys[j] + " — unassigned for this model"
			if ok {
				tip = keys[j] + " — " + p.help[j]
			}
			b.Set("title", tip)
		}
	}
}

// wireTrios hangs one listener on the document for every trio there is or
// will be: the bank's unassigned positions are copies, and a copy does not
// carry its original's listeners. Page-lifetime, so js.FuncOf.
var trioWired bool

func wireTrios() {
	if trioWired || !dom.Doc.Truthy() {
		return
	}
	trioWired = true
	dom.Doc.Call("addEventListener", "click", js.FuncOf(func(_ js.Value, a []js.Value) any {
		b := a[0].Get("target").Call("closest", ".trio-btn")
		if !b.Truthy() {
			return nil
		}
		// Unassigned: nothing to do, and nothing done.
		c := b.Call("closest", "[data-param]")
		if !c.Truthy() {
			return nil
		}
		param := c.Call("getAttribute", "data-param").String()
		p, ok := trioPrograms[param]
		if !ok {
			return nil
		}
		if i, err := strconv.Atoi(b.Call("getAttribute", "data-trio").String()); err == nil {
			p.press(i)
		}
		lightTrios(param)
		return nil
	}))
	// A parameter with buttons changed some other way — its own knob, a reset,
	// a permalink, MIDI — or one that a position's buttons drive did: relight
	// those positions. Every other input is two map lookups and nothing more.
	// A selector says so with change rather than input, so both.
	relight := js.FuncOf(func(_ js.Value, a []js.Value) any {
		idv := a[0].Get("target").Get("id")
		if !idv.Truthy() {
			return nil
		}
		id := idv.String()
		if _, ok := trioPrograms[id]; ok {
			lightTrios(id)
		}
		for _, param := range trioDriven()[id] {
			lightTrios(param)
		}
		return nil
	})
	dom.Doc.Call("addEventListener", "input", relight)
	dom.Doc.Call("addEventListener", "change", relight)
}

// trioDrivenBy is, by control, the parameters whose buttons set it: made the
// first time it is asked for, once every init has added its programs.
var trioDrivenBy map[string][]string

func trioDriven() map[string][]string {
	if trioDrivenBy == nil {
		trioDrivenBy = map[string][]string{}
		for param, p := range trioPrograms {
			for _, id := range p.drive {
				if id != param {
					trioDrivenBy[id] = append(trioDrivenBy[id], param)
				}
			}
		}
	}
	return trioDrivenBy
}

// legends are the program's printed keys: its own, or the bank's + 0 −.
func (p trioProgram) legends() []string {
	if p.keys != nil {
		return p.keys
	}
	return trioKeys[:]
}
