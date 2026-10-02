//go:build js && wasm

package attractor

import (
	"slices"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dotmatrix"
)

// ── The knob bank ────────────────────────────────────────────────────────
//
// A category row used to carry a card per model, every card always on show:
// fourteen of them for the attractors, across three bays, to show the three
// knobs of the one model running. A bank is what a synthesizer does with the
// same problem. It has a FIXED set of knobs — as many as the category's
// biggest model needs — and a small character display over each one, and
// choosing a patch reprograms the displays. Nothing on the panel moves.
//
// So a bank position is always a knob, whichever model the bay is set to.
// Where the model has a constant for it, the position shows that constant's
// control, with the constant's name on the display; where it has none, it
// shows an unassigned knob of the same shape, parked, with its display dark.
// Under the panel every parameter still has its own control and its own id —
// the MIDI map, the permalink and Reset All address it as they always have —
// and what the MODEL knob changes is which of them each position is showing.
//
// Knobs, selectors and switches are banked separately, knobs first: a
// position cannot be reprogrammed from a knob into a switch, so a category
// whose models mix them has a section of each, each as big as its biggest.

// bankCategories are the rows drawn as a bank rather than as cards: a row
// label, which for a merged row is its group's (see rackRows).
// One row, one bank for every model (see rackRowGroups).
var bankCategories = map[string]bool{"Visual": true}

// bankChars is how many characters a bank's displays hold: its longest
// legend, but never fewer than bankMinChars nor more than bankMaxChars. Every
// display in a bank is the same size, whatever it is showing, because it is
// one part fitted to every position.
var bankChars = bankMinChars

// bankControls is how many of its bay's controls each bank model has.
var bankControls = map[string]int{}

const (
	bankMinChars = 3
	bankMaxChars = 6 // "offset", "radius", "stacks"
)

// buildBankRow is a bank category's one bay: its head and its bank. A family
// dial, if the category has one, is on the head's MODEL knob (buildBayRotary).
//
// The step is programmable too. Every model's step cell sits in the one
// position the head keeps for it, with a display rather than a printed "dt":
// a bay that someday holds a model with no step, or a step by another name,
// shows that on the same part.
func buildBankRow(label string, own []string, cells map[string][]js.Value, steps map[string]js.Value, shared, hidden, tail []js.Value) []js.Value {
	// One display size for the whole bay, head and bank alike.
	bankChars = bankMinChars
	measure := func(c js.Value) {
		if n := c.Call("getAttribute", "data-chars"); n.Truthy() { // a placeholder (banklazy_js.go)
			if v, err := strconv.Atoi(n.String()); err == nil {
				bankChars = max(bankChars, min(bankMaxChars, v))
			}
			return
		}
		ls := c.Call("querySelectorAll", ".u-lbl, .plabel, .twoway-name, option")
		for i := range ls.Get("length").Int() {
			bankChars = max(bankChars, min(bankMaxChars, len([]rune(ls.Index(i).Get("textContent").String()))))
		}
	}
	for _, mode := range own {
		for _, c := range cells[mode] {
			measure(c)
		}
		if s, ok := steps[mode]; ok {
			measure(s)
		}
	}

	var some js.Value
	var free []string
	for _, mode := range own {
		if s, ok := steps[mode]; ok {
			programLegends(s)
			makeTurning(s)
			some = s
		} else {
			free = append(free, mode)
		}
	}
	// The cells several models share: family dials first, then shared
	// parameters, each at one position for all of its models (below).
	var common []js.Value
	// A family's dial is one of its members' controls: a selector in the bank,
	// one cell shown for whichever member the bay is on.
	for _, f := range modelFamilies {
		var members []string
		for _, m := range own {
			if familyOf(m) == f {
				members = append(members, m)
			}
		}
		if len(members) == 0 {
			continue
		}
		fc := buildFamilyCell(f, true)
		fc.Call("setAttribute", "data-bank-for", strings.Join(members, " "))
		common = append(common, fc)
	}
	// So is every parameter several of its models share: in the bank, at the
	// same position for each of them, rather than in the head where it would
	// stand for models it does nothing to. One position per shared cell, the
	// same for every model it serves (bankSharedSlots); each model's own
	// cells fill the positions around them.
	common = append(common, shared...)
	serves := make([][]string, len(common))
	for i, c := range common {
		serves[i] = strings.Fields(c.Call("getAttribute", "data-bank-for").String())
	}
	pos := bankSharedSlots(serves)
	// The step's position, for a model with no step: its first own cell,
	// rather than a blank between its first and second shared positions. A
	// cell of its own is no other model's, so taking it out of the order moves
	// nothing another model relies on. Only with a second own cell, so the
	// bank is built for it to go into.
	stepFill := map[string]js.Value{}
	for _, m := range free {
		if len(cells[m]) > 1 {
			stepFill[m] = cells[m][0]
		}
	}
	for _, m := range own {
		first := 0
		if _, ok := stepFill[m]; ok {
			first = 1
		}
		var laid []js.Value
		for _, s := range bankModelSlots(m, serves, pos, len(cells[m])-first) {
			switch {
			case s.Shared >= 0:
				laid = append(laid, common[s.Shared])
			case s.Own >= 0:
				laid = append(laid, cells[m][s.Own+first])
			default:
				laid = append(laid, js.Undefined()) // kept for a shared cell of other models
			}
		}
		cells[m] = laid
	}
	bankTail(own, cells, tail)
	// How many of the bay's controls each model has, for the MODEL list
	// (bayPicker): its step, or the cell standing in for one, and every
	// position that holds one of its controls, a family's dial included.
	for _, m := range own {
		n := 0
		if _, ok := steps[m]; ok {
			n++
		} else if _, ok := stepFill[m]; ok {
			n++
		}
		for _, c := range cells[m] {
			if c.Truthy() {
				n++
			}
		}
		bankControls[m] = n
	}
	// The bank first: the head's MODEL knob and step go into it.
	bank := buildBankModule(label, own, cells)
	var bankGrid js.Value
	if bank.Truthy() {
		bankGrid = bank.Call("querySelector", ".catbank")
	}
	head := buildBayHead(label, 0, own, steps, nil, bankGrid)
	var blankFor []string
	for _, m := range free {
		c, ok := stepFill[m]
		if !ok || !bankGrid.Truthy() {
			blankFor = append(blankFor, m)
			continue
		}
		c.Get("classList").Call("add", "bankcell")
		c.Call("setAttribute", "data-bank", m)
		if !bankIsPlaceholder(c) {
			bankPosition(c)
		}
		at(c, bankGrid, bankStepRow, 1, 1, 1)
	}
	if len(blankFor) > 0 && some.Truthy() {
		blank := unassignedControl(some)
		blank.Get("classList").Call("remove", "stepcell")
		blank.Call("setAttribute", "data-bank-for", strings.Join(blankFor, " "))
		if bankGrid.Truthy() {
			at(blank, bankGrid, bankStepRow, 1, 1, 1)
		} else {
			at(blank, head.Call("querySelector", ".cathead"), 1+catMonitorRows, 1, 2, 1)
		}
	}
	out := []js.Value{head}
	if bank.Truthy() {
		out = append(out, bank)
		g := bankGrid
		// The parameters a position's buttons set: in the panel, so they are
		// still controls to reset, the permalink and MIDI, but not on it.
		for _, c := range hidden {
			c.Get("style").Set("display", "none")
			g.Call("appendChild", c)
		}
	}
	wireTrios()
	syncTrios()
	return out
}

// The head of the bank: its first column is the bay's MODEL knob, then the
// first position, then the step (buildBayHead), and the positions carry on
// down the next column. Where the knob and the step stood under the monitor
// is the running model's equation.
const (
	bankRotaryRow = 1
	bankStepRow   = 3
	bankHeadCells = 2 // positions in the first column kept for those two
)

// bankCell is the bank's i-th position, as a grid row and column: down each
// column, around the two kept for the head.
func bankCell(i int) (row, col int) {
	if i == 0 {
		return 2, 1 // under the MODEL knob
	}
	i += bankHeadCells
	return 1 + i%catCellsPerCol, 1 + i/catCellsPerCol
}

// eqSlotID is the place under a bank row's monitor that holds the running
// model's equation (mountEquation).
func eqSlotID(label string) string { return "eqslot-" + categorySlug(label) }

// newPUnit is model mode's parameter p built as a P-unit is (buildParamUnitAs):
// a selector without a card's knob, so nothing is built to be thrown away.
// It is finished by bankPosition when the bank is laid out, not here: every
// display in a bank is one size, the bank's longest legend (bankChars), and
// that is known only once every cell in it is.
func newPUnit(mode string, p paramDef) js.Value { return buildCategoryParamCellAs(mode, p, true) }

// bankPosition makes c a P-unit, one of the bank's positions: the one part, its
// legend display, and its three buttons.
func bankPosition(c js.Value) {
	if c.Get("classList").Call("contains", "pu").Bool() {
		return // one already
	}
	uniformBankCell(c)
	programLegends(c)
	c.Call("appendChild", trioColumn(trioKeys[:]))
	makeTurning(c)
	c.Get("classList").Call("add", "pu")
}

// bankColumns is how many columns a bank has: the rest of its bay, after the
// head. As many as the model with the most controls would need is fewer, and
// a bank that stopped short of the bay's end left a strip of loose modules
// beside it; one that runs to the end is the bay's one panel.
const bankColumns = catColsPerBay - catHeadCols

// buildBankModule lays the models' controls into fixed positions — at least
// enough to fill bankColumns, and as many as the model with the most controls
// — filling each column top to bottom. Every position is the same part
// (uniformBankCell), so a model's controls simply take the positions in the
// order it declares them.
func buildBankModule(label string, modes []string, cells map[string][]js.Value) js.Value {
	size := 0
	for _, mode := range modes {
		size = max(size, len(cells[mode]))
	}
	if size == 0 {
		return js.Value{}
	}
	size = max(size, bankColumns*catCellsPerCol-bankHeadCells)
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid catbank")
	type blankAt struct {
		row, col int
		free     []string
	}
	var blanks []blankAt
	for i := range size {
		row, col := bankCell(i)
		var free []string // the models with nothing at this position
		for _, mode := range modes {
			cs := cells[mode]
			if i >= len(cs) || !cs[i].Truthy() { // past its last, or kept for others' shared cell
				free = append(free, mode)
				continue
			}
			c := cs[i]
			// A cell several models share is placed once, and says which
			// models it is for itself (data-bank-for).
			if c.Call("hasAttribute", "data-bank-for").Bool() {
				if !c.Get("parentNode").Truthy() {
					bankPosition(c)
					at(c, grid, row, 1, col, 1)
				}
				continue
			}
			c.Get("classList").Call("add", "bankcell")
			c.Call("setAttribute", "data-bank", mode)
			if !bankIsPlaceholder(c) {
				bankPosition(c)
			}
			at(c, grid, row, 1, col, 1)
		}
		if len(free) > 0 {
			blanks = append(blanks, blankAt{row, col, free})
		}
	}
	// The unassigned positions, all from one part: a copy of the bank's
	// first numeric knob, so every one of them is the same whatever position
	// it stands in and whatever the model beside it has there.
	tpl := grid.Call("querySelector", ".u-val:not(.swhidden)")
	if tpl.Truthy() {
		tpl = tpl.Call("closest", ".punit")
	}
	for _, b := range blanks {
		if !tpl.Truthy() {
			break
		}
		blank := unassignedControl(tpl)
		blank.Call("setAttribute", "data-bank-for", strings.Join(b.free, " "))
		at(blank, grid, b.row, 1, b.col, 1)
	}
	// Not the category's bare name: the rack keys a module by its header (its
	// saved place, its width high-water mark), and that name is the head's.
	//
	// "Model", because it is everything the model on the MODEL knob has to set:
	// its constants, and the pots and selectors that used to be a module of
	// its own (modelparts_js.go).
	return wrapCategoryModule(label, "bank-"+categorySlug(label)+"-module", label+" · Model", grid,
		label+" · Model — a bank of "+strconv.Itoa(size)+" programmable controls: the MODEL knob, "+
			"then every constant, pot and selector of the model it is set to. Each display names "+
			"what the control under it does for that model; turning the MODEL knob reprograms them, "+
			"and each model keeps its own settings while another is showing. With Back on (Layers · "+
			"Colors) they are the backdrop's.")
}

// programLegends puts a character display where each of a cell's printed
// legends was and, on a selector (which a switch is too, by now: see
// uniformBankCell),
// a second display that reads the option it is on: a bank selector serves
// several models with different options, so the ring of printed option names
// it had is not on the panel, and the display says the setting instead.
func programLegends(c js.Value) {
	c.Get("classList").Call("add", "dmdcell")
	if v := c.Call("querySelector", ".u-val"); v.Truthy() {
		// Fixed by the bank's CSS: a readout that sized itself to its
		// parameter changed width as the MODEL knob changed the parameter.
		v.Get("style").Call("removeProperty", "width")
	}
	ls := c.Call("querySelectorAll", ".u-lbl, .plabel")
	for i := range ls.Get("length").Int() {
		l := ls.Index(i)
		l.Get("classList").Call("add", "dmdlbl")
		l.Get("parentNode").Call("insertBefore", dotDisplay(l.Get("textContent").String(), true), l)
	}
	if v := c.Call("querySelector", ".punit-top>.u-val:not(.swhidden):not(.dmdmirror)"); v.Truthy() && v.Get("style").Get("display").String() != "none" {
		mirrorReadout(v)
	}
	sel := c.Call("querySelector", "select")
	top := c.Call("querySelector", ".punit-top")
	if !sel.Truthy() || !top.Truthy() || c.Call("querySelector", ".u-val:not(.swhidden)").Truthy() {
		return
	}
	// A selector whose dial is rebuilt has its display from that build
	// (selectorReadout), which keeps it.
	if top.Call("querySelector", ".dmdval").Truthy() {
		return
	}
	// The setting's short name where it has one — the label written to fit
	// round a dial (paramRingLabels) fits a six-character display too, where
	// the option's full name was cut off mid-word ("left is reference" read
	// "left i") or carried a character the display has no dots for.
	ring := paramRingLabels[c.Call("getAttribute", "data-param").String()]
	optText := func() string {
		if i := sel.Get("selectedIndex").Int(); i >= 0 && i < len(ring) {
			return ring[i]
		}
		if o := sel.Get("selectedOptions"); o.Truthy() && o.Length() > 0 {
			return strings.TrimSpace(o.Index(0).Get("textContent").String())
		}
		return ""
	}
	// The readout box is sized by the bank's CSS, not by its own inline width.
	val := dotDisplayN(optText(), false, bankValChars)
	val.Get("classList").Call("add", "dmdval")
	top.Call("appendChild", val)
	ledPick(val, sel)
	dom.OnAs(sel, "change", "display", func(js.Value, []js.Value) any {
		setDotText(val, optText())
		return nil
	})
}

// mirrorReadout puts a character display over a bank position's number
// field. The field stays, under it, as what a click types into; the display
// reads its value. Seven segments cannot spell the settings beside it, and a
// bank of one part should not show a number in one kind of display and a
// setting in another.
//
// The value is written from many places (the knob, a reset, a permalink, a
// model change, audio modulation), none of which fires an event, so the
// display is redrawn from the field's own value setter: every writer passes
// through it.
func mirrorReadout(in js.Value) {
	in.Get("classList").Call("add", "dmdmirror")
	win := dotDisplayN(readoutText(in.Get("value").String()), false, bankValChars)
	win.Get("classList").Call("add", "dmdval", "dmdnum")
	in.Get("parentNode").Call("insertBefore", win, in)
	show := func() { setDotText(win, readoutText(in.Get("value").String())) }
	obj := js.Global().Get("Object")
	native := obj.Call("getOwnPropertyDescriptor", js.Global().Get("HTMLInputElement").Get("prototype"), "value")
	set := native.Get("set")
	desc := obj.New()
	desc.Set("configurable", true)
	desc.Set("get", native.Get("get"))
	desc.Set("set", dom.FuncOf(func(this js.Value, a []js.Value) any {
		set.Call("call", this, a[0])
		show()
		return nil
	}))
	obj.Call("defineProperty", in, "value", desc)
	redraw := dom.FuncOf(func(js.Value, []js.Value) any { show(); return nil })
	in.Call("addEventListener", "input", redraw)
	in.Call("addEventListener", "blur", redraw)
}

// setDotText rewrites what a display shows, in the orientation and the
// number of characters it was built with. A display written into the markup
// as a bare span (a readout the page declares, with its data-chars) is given
// its dots on the first write, so code that looked it up by id before then
// still holds the display.
func setDotText(win js.Value, text string) {
	n := bankChars
	if c, err := strconv.Atoi(win.Call("getAttribute", "data-chars").String()); err == nil && c > 0 {
		n = c
	}
	vertical := win.Get("classList").Call("contains", "dmdv").Bool()
	svg := win.Call("querySelector", "svg")
	if !svg.Truthy() {
		win.Set("textContent", "")
		win.Get("classList").Call("add", "dmdwin")
		win.Get("classList").Call("toggle", "disp-half", !vertical && n <= dispHalfChars)
		win.Call("appendChild", dotSVG(text, vertical, n))
		return
	}
	lit, dark := dotPaths(text, vertical, n)
	svg.Call("setAttribute", "aria-label", text) // what it says, to a screen reader
	if on := win.Call("querySelector", ".dmd-on"); on.Truthy() {
		on.Call("setAttribute", "d", lit)
	}
	if off := win.Call("querySelector", ".dmd-off"); off.Truthy() {
		off.Call("setAttribute", "d", dark)
	}
}

// dotPaths draws text across a display of n characters, or down one.
func dotPaths(text string, vertical bool, n int) (lit, dark string) {
	if vertical {
		return dotmatrix.PathsVertical(text, n)
	}
	return dotmatrix.Paths(text, n)
}

// dotDisplay is one character display: bankChars characters of 5×7 dots,
// every dot drawn, the lit ones in the LED color. A legend's display is
// vertical, down the left edge of its cell where a printed label was, so the
// readout keeps the top of the cell to itself, centered over the knob; a
// setting's readout is horizontal, in the readout's own place.
func dotDisplay(text string, vertical bool) js.Value {
	return dotDisplayN(text, vertical, bankChars)
}

// dotDisplayN is dotDisplay with n characters, for a display outside a bank:
// a readout that has words to say as well as numbers.
func dotDisplayN(text string, vertical bool, n int) js.Value {
	win := dom.Doc.Call("createElement", "span")
	win.Set("className", "dmdwin")
	if vertical {
		win.Get("classList").Call("add", "dmdv")
	}
	// No title of its own: hovering the display shows its cell's, which
	// says what the legend abbreviates. Its own would only repeat the legend.
	win.Call("setAttribute", "data-chars", strconv.Itoa(n))
	if !vertical && n <= dispHalfChars {
		win.Get("classList").Call("add", "disp-half")
	}
	win.Call("appendChild", dotSVG(text, vertical, n))
	return win
}

// dotSVG is a display's dots: every one drawn, dark, and the lit ones over
// them.
func dotSVG(text string, vertical bool, n int) js.Value {
	const ns = "http://www.w3.org/2000/svg"
	svg := dom.Doc.Call("createElementNS", ns, "svg")
	svg.Call("setAttribute", "class", "dmd")
	w, h := dotmatrix.Width(n), dotmatrix.Rows
	if vertical {
		w, h = dotmatrix.Cols, dotmatrix.Height(n)
	}
	svg.Call("setAttribute", "viewBox", "0 0 "+strconv.Itoa(w)+" "+strconv.Itoa(h))
	svg.Call("setAttribute", "aria-label", text)
	lit, dark := dotPaths(text, vertical, n)
	for _, p := range [][2]string{{"dmd-off", dark}, {"dmd-on", lit}} {
		path := dom.Doc.Call("createElementNS", ns, "path")
		path.Call("setAttribute", "class", p[0])
		path.Call("setAttribute", "d", p[1])
		svg.Call("appendChild", path)
	}
	return svg
}

// unassignedControl is a position with nothing to do for this model: a copy
// of a real control of the same kind, so it is exactly the same shape, with
// everything that made it one particular parameter taken off it. Its display
// is dark, its knob parked at the bottom of its travel, and it answers
// nothing, because a clone carries no listeners.
func unassignedControl(model js.Value) js.Value {
	c := model.Call("cloneNode", true)
	c.Get("classList").Call("remove", "bankcell", "live")
	c.Get("classList").Call("add", "bankblank")
	// data-param too: a copy is no parameter's position, and its buttons
	// must not answer for the one it was copied from (see trioOf).
	for _, a := range []string{"id", "data-mode", "data-bank", "data-bank-for", "data-param", "title"} {
		c.Call("removeAttribute", a)
	}
	strip := func(sel string, f func(js.Value)) {
		ns := c.Call("querySelectorAll", sel)
		for i := range ns.Get("length").Int() {
			f(ns.Index(i))
		}
	}
	strip("[id]", func(e js.Value) { e.Call("removeAttribute", "id") })
	strip("[title]", func(e js.Value) { e.Call("removeAttribute", "title") })
	strip(".gtag", func(e js.Value) { e.Call("remove") })
	strip(".dmdwin", func(e js.Value) {
		n, err := strconv.Atoi(e.Call("getAttribute", "data-chars").String())
		if err != nil || n < 1 {
			n = bankChars
		}
		d := dotDisplayN("", e.Get("classList").Call("contains", "dmdv").Bool(), n)
		for _, k := range []string{"dmdval", "dmdnum"} {
			if e.Get("classList").Call("contains", k).Bool() {
				d.Get("classList").Call("add", k)
			}
		}
		e.Call("replaceWith", d)
	})
	strip("input", func(e js.Value) {
		e.Set("disabled", true)
		e.Set("value", "")
		e.Set("checked", false)
	})
	// An unassigned position's ring is dark: there is no value to show.
	strip(".value-dial", func(e js.Value) { e.Call("removeAttribute", "data-lit") })
	strip(".knob-ptr", func(e js.Value) {
		e.Get("style").Set("transform", "translate(-50%, -100%) rotate(-135deg)")
	})
	// Its resets stay on the panel, as on every position, and say why they
	// do nothing.
	strip(".rst", func(e js.Value) {
		e.Set("disabled", true)
		e.Set("title", doc("bank-rst-none"))
	})
	return c
}

// bankTrioSel is the buttons of model's positions in a bay: its own, its step
// position's, and the shared and unassigned positions that name it.
func bankTrioSel(model string) string {
	return `.bankcell[data-bank="` + model + `"] .trio, .stepcell[data-mode="` + model + `"] .trio, [data-bank-for~="` + model + `"] .trio`
}

// syncBankCells shows, in every bay, the step cell and the bank controls of
// the model that bay is set to, and the unassigned controls where that model
// has none.
//
// Each row by its own bay's model. A model can be in two rows — stereo is an
// embedding and an audio display — and a rule that showed a model's cells
// wherever ANY bay was set to it put stereo's controls on top of the
// Embeddings bank's own whenever the Audio bay played stereo.
func syncBankCells() {
	if !dom.Doc.Truthy() {
		return
	}
	h := fastDOM()
	type trioRoot struct {
		el  js.Value
		sel string
	}
	var trioRoots []trioRoot
	for _, b := range rackBays {
		model := bayModelOf(b)
		// The row's head (its step position) and its bank.
		parts := js.Global().Get("Array").New()
		if mon := dom.Doc.Call("getElementById", bayMonitorID(b.Label, b.N)); mon.Truthy() {
			if head := mon.Call("closest", ".sect"); head.Truthy() {
				parts.Call("push", head)
				trioRoots = append(trioRoots, trioRoot{head, bankTrioSel(model)})
			}
		}
		if bank := dom.Doc.Call("getElementById", "bank-"+categorySlug(b.Label)+"-module"); bank.Truthy() {
			parts.Call("push", bank)
			trioRoots = append(trioRoots, trioRoot{bank, bankTrioSel(model)})
		}
		bankMaterialize(model) // its cells, the first time it is shown
		// An unassigned position still answers a hover: with what it is not.
		h.Call("bankShow", parts, model, "Unassigned — "+modeLabel(model)+" has no control in this position.")
	}
	// The head's switches and readouts follow the same model.
	syncModelParts(editMode())
	// What is showing in each position changed, so what each address is,
	// and what each position's buttons do: the buttons of the positions now
	// showing, and nothing else, since nothing else in the rack is programmed
	// by the model and a hidden position is relit when it is shown.
	for _, r := range trioRoots {
		syncTriosIn(r.el, r.sel)
	}
	scheduleDesignate()
}

// ── The P-unit: one part in every position ───────────────────────────────
//
// A bank position is the same part whatever it is programmed to: a knob in a
// tick dial, a legend display and a readout over it, a step readout and its
// small knob under it. The parameters behind them are not alike — a
// constant, a list of settings, an on/off — but a synthesizer's encoders are
// not either, and they are one part: what differs is what the display says.
//
// So a selector is mounted as a detented knob of the same size in the same
// dial, a switch as a knob with two detents, and the readout of either is a
// display naming the setting. The select or checkbox underneath is still the
// control — its id, its listeners, the permalink and the MIDI map are as they
// were — and the knob only turns it.

// uniformBankCell makes c the bank's one part, whatever it was built as.
func uniformBankCell(c js.Value) {
	sel := c.Call("querySelector", "select")
	cb := c.Call("querySelector", "input[type=checkbox]")
	// Built bare (newPUnit, buildFamilyCell, the markup's cells): a selector
	// comes with no knob, and the one it gets is this one. A switch is a
	// selector with two positions.
	if !sel.Truthy() && cb.Truthy() {
		if sel = switchAsSelect(c, cb); sel.Truthy() {
			c.Call("appendChild", sel)
		}
	}
	if sel.Truthy() {
		// Its setting is read on a display (programLegends), not a number.
		if v := c.Call("querySelector", ".u-val"); v.Truthy() {
			v.Get("classList").Call("add", "swhidden")
		}
		// A selector whose dial is rebuilt has a place for its knob, and its
		// own build mounts it there (selectorReadout).
		if !c.Call("querySelector", ".knobhold").Truthy() {
			c.Call("appendChild", ledSelector(sel))
		}
		// No step: a setting has positions, not a resolution. The readout,
		// its knob and its reset are there, dark, because they are there on
		// every position of the bank.
		step := dom.Doc.Call("createElement", "input")
		step.Set("className", "numin u-step")
		step.Set("disabled", true)
		c.Call("appendChild", step)
		sk := dom.Doc.Call("createElement", "span")
		sk.Set("className", "stepknob stepnone")
		sk.Call("appendChild", dom.Doc.Call("createElement", "i"))
		c.Call("appendChild", sk)
		sr := dom.Doc.Call("createElement", "button")
		sr.Set("className", "rst steprst stepnone")
		sr.Set("disabled", true)
		sr.Set("title", doc("bank-step-none"))
		sr.Set("textContent", "↺")
		c.Call("appendChild", sr)
	}
	// Every knob carries the fine-trim ring; where the parameter moves in
	// whole steps it is there and turns nothing, as on the other positions.
	if k := c.Call("querySelector", ".knob.knobb"); k.Truthy() && !k.Call("querySelector", ".knob-fine").Truthy() {
		fine := dom.Doc.Call("createElement", "span")
		fine.Set("className", "knob-fine finenone")
		k.Call("appendChild", fine)
	}
}

// switchAsSelect is a two-position select that stands for a switch: turning
// it sets the checkbox, and the checkbox moving moves it. Its options are the
// switch's two position names.
func switchAsSelect(c, cb js.Value) js.Value {
	var labels []string
	if r := c.Call("querySelector", "input[type=range][id]"); r.Truthy() {
		labels = paramLabels[r.Get("id").String()]
	}
	if len(labels) != 2 {
		labels = []string{"off", "on"}
	}
	sel := dom.Doc.Call("createElement", "select")
	sel.Get("style").Set("display", "none")
	for i, l := range labels {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", strconv.Itoa(i))
		o.Set("textContent", l)
		sel.Call("appendChild", o)
	}
	at := func() int {
		if cb.Get("checked").Bool() {
			return 1
		}
		return 0
	}
	sel.Set("selectedIndex", at())
	// Each side only answers a change that changes something, so the two
	// do not chase each other.
	dom.On(sel, "change", func(js.Value, []js.Value) any {
		if want := sel.Get("selectedIndex").Int() == 1; cb.Get("checked").Bool() != want {
			cb.Set("checked", want)
			dom.Fire(cb, "change")
		}
		return nil
	})
	dom.On(cb, "change", func(js.Value, []js.Value) any {
		if i := at(); sel.Get("selectedIndex").Int() != i {
			sel.Set("selectedIndex", i)
			dom.Fire(sel, "change")
		}
		return nil
	})
	return sel
}

// ── Custom's constants in the bank ────────────────────────────────────────
//
// Every other model's constants are built into its row's bank once, at
// startup. Custom's are whatever its equation declares — a knob for each
// letter that is not x, y, z or w — so they are built with its panel, on every
// parse, into the positions a model with that many constants would have, and
// taken out again when the panel is built for another model. Until then they
// stood in a Parameters module of their own a bay away, the one model in the
// row whose knobs were not where the MODEL knob's other models keep theirs.

// customBankMark is on a blank Custom's cell has taken the position from.
const customBankMark = "data-custom-took"

// clearCustomBankCells takes Custom's cells out of the bank and gives the
// positions back to the blanks they took them from.
func clearCustomBankCells() {
	if !dom.Doc.Truthy() {
		return
	}
	olds := dom.Doc.Call("querySelectorAll", `.catbank [data-bank="custom"]`)
	for i := range olds.Length() {
		olds.Index(i).Call("remove")
	}
	took := dom.Doc.Call("querySelectorAll", ".catbank ["+customBankMark+"]")
	for i := range took.Length() {
		b := took.Index(i)
		b.Call("removeAttribute", customBankMark)
		b.Call("setAttribute", "data-bank-for", strings.TrimSpace(b.Call("getAttribute", "data-bank-for").String()+" custom"))
	}
}

// bankCustomCells builds Custom's constants into its row's bank, and says
// whether there was a bank to build them into.
func bankCustomCells(defs []paramDef) bool {
	clearCustomBankCells()
	bank := dom.Doc.Call("getElementById", "bank-"+categorySlug(rowOf("custom"))+"-module")
	if !bank.Truthy() {
		return false
	}
	grid := bank.Call("querySelector", ".catbank")
	if !grid.Truthy() {
		return false
	}
	// The blanks Custom is shown at, by the grid position they stand in.
	blanks := map[string]js.Value{}
	bs := grid.Call("querySelectorAll", ".bankblank[data-bank-for]")
	for i := range bs.Length() {
		b := bs.Index(i)
		if slices.Contains(strings.Fields(b.Call("getAttribute", "data-bank-for").String()), "custom") {
			st := b.Get("style")
			blanks[st.Get("gridRow").String()+"|"+st.Get("gridColumn").String()] = b
		}
	}
	// Its dt is a step, in the position every flow's step is in; the rest
	// are its constants, in order from the first.
	i := 0
	for _, d := range defs {
		c := buildParamUnit("custom", d)
		c.Get("classList").Call("add", "bankcell")
		c.Call("setAttribute", "data-bank", "custom")
		c.Call("setAttribute", "data-mode", "custom")
		c.Call("setAttribute", "data-param", d.ID)
		tip := "Custom equation — " + d.Label
		if h := helpFor(d.ID); h != "" {
			tip = "Custom equation — " + h
		}
		c.Set("title", tip)
		row, col := bankStepRow, 1
		if d.ID == "custom-dt" {
			uniformBankCell(c) // the bank's part, without the buttons a step has none of
			programLegends(c)
			makeTurning(c)
		} else {
			bankPosition(c)
			row, col = bankCell(i)
			i++
		}
		at(c, grid, row, 1, col, 1)
		st := c.Get("style")
		if b, ok := blanks[st.Get("gridRow").String()+"|"+st.Get("gridColumn").String()]; ok {
			var rest []string
			for _, m := range strings.Fields(b.Call("getAttribute", "data-bank-for").String()) {
				if m != "custom" {
					rest = append(rest, m)
				}
			}
			b.Call("setAttribute", "data-bank-for", strings.Join(rest, " "))
			b.Call("setAttribute", customBankMark, "")
		}
	}
	syncBankCells()
	// Built after the pass that lights the running model's cells, so they
	// are lit here: unlit, Custom's own knobs were dimmed as patched out
	// while Custom was the model running.
	lightLiveParamCells()
	return true
}

// ledSelector is a selector knob with no printed positions: a ring of LED
// dots, the one at its setting lit, its detents spread over the ring's travel
// as printed labels would be. What the setting is called goes on a character
// display beside it (the bank's programLegends, or selectorReadout). It is
// the part for a selector whose options are not fixed — they change with the
// model, or with the grid — since a printed ring would change under the knob.
func ledSelector(sel js.Value) js.Value {
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "knobwrap")
	wrap.Call("setAttribute", "data-no-drag", "")
	knob := selk.makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knobb")
	wrap.Call("appendChild", knob)
	addValueDial(wrap, 0, 1)
	if dial := wrap.Call("querySelector", ".value-dial"); dial.Truthy() {
		light := func() {
			d := ringDot(sel.Get("selectedIndex").Int(), sel.Get("options").Length(), dialTicks)
			lightRing(dial, d, d)
		}
		light()
		dom.OnAs(sel, "change", "ledring", func(js.Value, []js.Value) any {
			light()
			return nil
		})
	}
	return wrap
}

// selectorReadout puts an ledSelector in holder and the name of its setting
// on a character display in its cell's top row, where a knob's readout goes.
// names are what the display says for each option.
func selectorReadout(holder, sel js.Value, names []string) {
	holder.Set("innerHTML", "")
	holder.Call("appendChild", ledSelector(sel))
	cell := holder.Call("closest", ".pcell, .punit")
	if !cell.Truthy() {
		return
	}
	cell.Get("classList").Call("add", "ledsel")
	top := cell.Call("querySelector", ".punit-top")
	if !top.Truthy() {
		return
	}
	name := func() string {
		if i := sel.Get("selectedIndex").Int(); i >= 0 && i < len(names) {
			return names[i]
		}
		return ""
	}
	win := top.Call("querySelector", ".dmdval")
	if !win.Truthy() {
		win = dotDisplayN("", false, bankValChars)
		win.Get("classList").Call("add", "dmdval")
		top.Call("appendChild", win)
		ledPick(win, sel)
	}
	ledPickFor(win, sel)
	setDotText(win, name())
	dom.OnAs(sel, "change", "display", func(js.Value, []js.Value) any {
		setDotText(win, name())
		return nil
	})
}

// bankTail puts tail, cells several models share that belong to no model
// (Model Out's), at the top of the bank's last column, the same place for
// every model they are for (data-bank-for) — after all of each model's own
// controls, so they never split a model's. Should a model reach that far,
// they go after its last instead, for all of them alike.
func bankTail(own []string, cells map[string][]js.Value, tail []js.Value) {
	if len(tail) == 0 {
		return
	}
	at := (bankColumns-1)*catCellsPerCol - bankHeadCells
	var fors [][]string
	for _, c := range tail {
		fors = append(fors, strings.Fields(c.Call("getAttribute", "data-bank-for").String()))
	}
	for i := range tail {
		for _, m := range fors[i] {
			at = max(at, len(cells[m]))
		}
	}
	for i, c := range tail {
		for _, m := range fors[i] {
			if !slices.Contains(own, m) {
				continue
			}
			for len(cells[m]) < at+i {
				cells[m] = append(cells[m], js.Undefined())
			}
			cells[m] = append(cells[m], c)
		}
	}
}

// ledPick makes win, the character display naming a selector's setting, a
// way to choose one: a click on it opens the selector's whole list, as the
// MODEL knob's display does (attachSelMarquee), and the choice is made on
// sel as though the knob had been turned to it. A knob alone shows one
// position at a time; a list is how you see what there is and go straight
// to it. The list is read from sel each time it opens, since what a selector
// offers can change with the model. A display kept while its selector is
// built again (selectorReadout) is pointed at the new one with ledPickFor.
func ledPick(win, sel js.Value) {
	parent := win.Get("parentNode")
	if !parent.Truthy() || !sel.Truthy() {
		return
	}
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "ledpickwrap")
	parent.Call("insertBefore", wrap, win)
	wrap.Call("appendChild", win)
	pick := dom.Doc.Call("createElement", "select")
	pick.Set("className", "ledpick")
	wrap.Call("appendChild", pick)
	ledPickFor(win, sel)
	ledPickWire()
}

// ledPickWire answers every list on the page, once: a display outlives the
// panel build that made it, and a listener of its own would be freed with
// that build (dom.FuncOf).
func ledPickWire() {
	if ledPickWired {
		return
	}
	ledPickWired = true
	sel := func(e js.Value) (js.Value, js.Value) {
		p := e.Get("target")
		if !p.Truthy() || !p.Get("classList").Truthy() || !p.Get("classList").Call("contains", "ledpick").Bool() {
			return js.Null(), js.Null()
		}
		return p, p.Get("previousElementSibling").Get("ledSel")
	}
	fill := js.FuncOf(func(_ js.Value, a []js.Value) any {
		if pick, s := sel(a[0]); s.Truthy() {
			ledPickTitle(pick, s)
			pick.Set("innerHTML", s.Get("innerHTML"))
			pick.Set("selectedIndex", s.Get("selectedIndex"))
		}
		return nil
	})
	doc := dom.Doc
	doc.Call("addEventListener", "mousedown", fill, true)
	doc.Call("addEventListener", "focusin", fill)
	doc.Call("addEventListener", "change", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if pick, s := sel(a[0]); s.Truthy() {
			s.Set("value", pick.Get("value"))
			dom.Fire(s, "change")
		}
		return nil
	}))
}

// ledPickWired is set once ledPickWire has run.
var ledPickWired bool

// ledPickFor points the list on display win at selector sel.
func ledPickFor(win, sel js.Value) {
	win.Set("ledSel", sel)
	if p := win.Get("nextElementSibling"); p.Truthy() && p.Get("classList").Call("contains", "ledpick").Bool() {
		ledPickTitle(p, sel)
	}
}

// ledPickTitle gives the list its selector's tooltip, or none, so that the
// cell's own shows through: an empty title would hide it.
func ledPickTitle(pick, sel js.Value) {
	if t := sel.Get("title").String(); t != "" {
		pick.Set("title", t)
	} else {
		pick.Call("removeAttribute", "title")
	}
}
