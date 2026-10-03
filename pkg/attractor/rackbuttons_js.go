//go:build js && wasm

package attractor

import (
	"encoding/json"
	"strconv"
	"strings"
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
	keys  []string      // the printed legends, top to bottom; trioKeys when nil
	help  []string      // what each button does, for its tooltip
	press func(i int)   // i indexes the legends
	lit   func() int    // which button is on, or -1
	lits  func() []bool // which buttons are on, for buttons that are switches of their own (lit then unused)
	drive []string      // the hidden controls it sets, so a change to them relights it
}

// setParamSlider sets a parameter through its own control, the way the knob
// would: reset, the permalink and MIDI keep seeing one control per parameter.
func setParamSlider(id string, v float64) {
	s := dom.Doc.Call("getElementById", id)
	if !s.Truthy() {
		return
	}
	s.Set("value", strconv.FormatFloat(v, 'f', -1, 64))
	dom.Fire(s, "input")
}

// paramSliderValue is the value of slider id, or -1 where there is none.
func paramSliderValue(id string) float64 {
	s := dom.Doc.Call("getElementById", id)
	if !s.Truthy() {
		return -1
	}
	return parseOr0(s.Get("value").String())
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

// bankHidden reports whether id is a parameter another position's buttons
// set. It keeps its control, hidden, so it is still one control to
// everything that reads controls, but it is not a position of its own.
// Built on first use, as trioDriven is: the programs added in init
// functions are not in the table when package variables are initialized.
func bankHidden(id string) bool {
	if bankHiddenIDs == nil {
		bankHiddenIDs = map[string]bool{}
		for param, p := range trioPrograms {
			for _, d := range p.drive {
				if d != param {
					bankHiddenIDs[d] = true
				}
			}
		}
	}
	return bankHiddenIDs[id]
}

var bankHiddenIDs map[string]bool

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

// lightTrios relights the buttons of one parameter's positions, and nothing
// else: a press is answered by its own column, not by a pass over the rack.
func lightTrios(param string) {
	p, ok := trioPrograms[param]
	if !ok {
		return
	}
	on := p.lighting()
	btns := dom.Doc.Call("querySelectorAll", `[data-param="`+param+`"] .trio-btn`)
	for j := range btns.Length() {
		b := btns.Index(j)
		i, _ := strconv.Atoi(b.Call("getAttribute", "data-trio").String()) //nolint:errcheck // set from an int by trioColumn
		b.Get("classList").Call("toggle", "trio-on", i < len(on) && on[i])
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
	// Decided here once per parameter, read and applied in JavaScript once
	// per call (fastDOM trioParams, trioApply): button by button from Go it
	// was six calls across a button, over every column on the rack.
	h := fastDOM()
	type entry struct {
		Live bool      `json:"live"`
		On   []bool    `json:"on"`
		Dead []bool    `json:"dead"`
		Keys []string  `json:"keys"`
		Tips []*string `json:"tips"` // nil leaves a tooltip as it is
	}
	payload := map[string]entry{}
	for _, param := range strings.Split(h.Call("trioParams", root, sel).String(), "\n") {
		if _, done := payload[param]; done {
			continue
		}
		p, ok := trioPrograms[param]
		e := entry{Live: ok, Keys: trioKeys[:], On: []bool{}}
		if ok {
			e.On, e.Keys = p.lighting(), p.legends()
		}
		for j, k := range e.Keys {
			e.Dead = append(e.Dead, ok && !p.assigned(j))
			// The legend is programmed too: a position's buttons say what
			// they do where it has legends of its own.
			if ok && j >= len(p.help) {
				e.Tips = append(e.Tips, nil)
				continue
			}
			tip := k + " — unassigned for this model"
			if ok {
				tip = k + " — " + p.help[j]
			}
			e.Tips = append(e.Tips, &tip)
		}
		payload[param] = e
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.Call("trioApply", root, sel, string(b))
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
		if i, err := strconv.Atoi(b.Call("getAttribute", "data-trio").String()); err == nil && p.assigned(i) {
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
// assigned reports whether button i does anything: it has a tooltip to say
// what, and a legend to say it with. The rest are inert.
func (p trioProgram) assigned(i int) bool {
	l := p.legends()
	return i >= 0 && i < len(p.help) && i < len(l) && l[i] != ""
}

func (p trioProgram) legends() []string {
	if p.keys != nil {
		return p.keys
	}
	return trioKeys[:]
}

// lighting is which of the program's buttons are on: the one lit names, or
// each that lits says, for buttons that are switches of their own.
func (p trioProgram) lighting() []bool {
	if p.lits != nil {
		return p.lits()
	}
	on := make([]bool, len(p.legends()))
	if i := p.lit(); i >= 0 && i < len(on) {
		on[i] = true
	}
	return on
}

// dimTrioButton dims button i of param's buttons where what it switches does
// not apply to what is on screen, as a switch's label used to dim. It still
// works: a dim button is a hint, not a lock.
func dimTrioButton(param string, i int, dim bool) {
	b := dom.Doc.Call("querySelector", `[data-param="`+param+`"] .trio-btn[data-trio="`+strconv.Itoa(i)+`"]`)
	if b.Truthy() {
		b.Get("classList").Call("toggle", "trio-dim", dim)
	}
}
