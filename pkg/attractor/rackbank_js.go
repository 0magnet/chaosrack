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
// One so far, while the look is settled.
var bankCategories = map[string]bool{"Visual": true}

// bankChars is how many characters a bank's displays hold: its longest
// legend, but never fewer than bankMinChars nor more than bankMaxChars. Every
// display in a bank is the same size, whatever it is showing, because it is
// one part fitted to every position.
var bankChars = bankMinChars

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
func buildBankRow(label string, own []string, cells map[string][]js.Value, steps map[string]js.Value, shared []js.Value) []js.Value {
	// One display size for the whole bay, head and bank alike.
	bankChars = bankMinChars
	measure := func(c js.Value) {
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
			some = s
		} else {
			free = append(free, mode)
		}
	}
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
		fc := buildFamilyCell(f)
		fc.Call("setAttribute", "data-bank-for", strings.Join(members, " "))
		for _, m := range members {
			cells[m] = append([]js.Value{fc}, cells[m]...)
		}
	}
	// So is every parameter several of its models share: in the bank, at the
	// same position for each of them (first, in the same order), rather than
	// in the head where it would stand for models it does nothing to.
	for i := len(shared) - 1; i >= 0; i-- {
		sc := shared[i]
		for _, m := range strings.Fields(sc.Call("getAttribute", "data-bank-for").String()) {
			cells[m] = append([]js.Value{sc}, cells[m]...)
		}
	}
	head := buildBayHead(label, 0, own, steps, nil)
	if len(free) > 0 && some.Truthy() {
		blank := unassignedControl(some)
		blank.Get("classList").Call("remove", "stepcell")
		blank.Call("setAttribute", "data-bank-for", strings.Join(free, " "))
		at(blank, head.Call("querySelector", ".cathead"), 1+catMonitorRows, 1, 2, 1)
	}
	out := []js.Value{head}
	grids := []js.Value{head.Call("querySelector", ".cathead")}
	if bank := buildBankModule(label, own, cells); bank.Truthy() {
		out = append(out, bank)
		grids = append(grids, bank.Call("querySelector", ".catbank"))
	}
	bankReadoutWidth(grids...)
	return out
}

// buildBankModule lays the models' controls into fixed positions — as many
// as the model with the most controls — filling each column top to bottom.
// Every position is the same part (uniformBankCell), so a model's controls
// simply take the positions in the order it declares them.
func buildBankModule(label string, modes []string, cells map[string][]js.Value) js.Value {
	size := 0
	for _, mode := range modes {
		size = max(size, len(cells[mode]))
	}
	if size == 0 {
		return js.Value{}
	}
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid catbank")
	type blankAt struct {
		row, col int
		free     []string
	}
	var blanks []blankAt
	for i := range size {
		row, col := 1+i%catCellsPerCol, 1+i/catCellsPerCol
		var free []string // the models with nothing at this position
		for _, mode := range modes {
			cs := cells[mode]
			if i >= len(cs) {
				free = append(free, mode)
				continue
			}
			c := cs[i]
			// A cell several models share is placed once, and says which
			// models it is for itself (data-bank-for).
			if c.Call("hasAttribute", "data-bank-for").Bool() {
				if !c.Get("parentNode").Truthy() {
					uniformBankCell(c)
					programLegends(c)
					at(c, grid, row, 1, col, 1)
				}
				continue
			}
			c.Get("classList").Call("add", "bankcell")
			c.Call("setAttribute", "data-bank", mode)
			uniformBankCell(c)
			programLegends(c)
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
	return wrapCategoryModule(label, "bank-"+categorySlug(label)+"-module", label+" · Constants", grid,
		label+" — a bank of "+strconv.Itoa(size)+" programmable controls, as many as the "+
			"biggest of its models needs. Each display names what the control under it does for "+
			"the model this bay is set to; turning the MODEL knob reprograms them, and each model "+
			"keeps its own settings while another is showing.")
}

// bankReadoutWidth sizes every readout in a bank's bay, head and bank, value
// and step alike, to the widest value any of its models shows. One width for
// all of them, because the MODEL knob changes what a position reads: a
// readout sized to one model's parameter overflowed on the next one's (a
// signed "+0.000" scrolled inside a box sized for "03.0", cutting its last
// digit off and moving as it changed).
// In pixels, not the ch sizeLEDField uses: the reset and the step knob are
// placed from this width too, and a ch on them is a character of THEIR font,
// a third narrower than a seven-segment digit.
//
// dseg7Advance is a DSEG7 Classic bold digit at the readouts' 11px, measured
// in the browser (canvas measureText of "0"), and + 1 its letter-spacing;
// + 9px is sizeLEDField's slack.
const dseg7Advance = 8.98

func bankReadoutWidth(grids ...js.Value) {
	chars := 0
	for _, g := range grids {
		if !g.Truthy() {
			continue
		}
		vs := g.Call("querySelectorAll", ".u-val[data-chars]")
		for i := range vs.Length() {
			if n, err := strconv.Atoi(vs.Index(i).Call("getAttribute", "data-chars").String()); err == nil && n > chars {
				chars = n
			}
		}
	}
	for _, g := range grids {
		if g.Truthy() && chars > 0 {
			g.Get("style").Call("setProperty", "--bank-led", strconv.FormatFloat(float64(chars)*(dseg7Advance+1)+9, 'f', 2, 64)+"px")
		}
	}
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
	sel := c.Call("querySelector", "select")
	top := c.Call("querySelector", ".punit-top")
	if !sel.Truthy() || !top.Truthy() || c.Call("querySelector", ".u-val:not(.swhidden)").Truthy() {
		return
	}
	optText := func() string {
		if o := sel.Get("selectedOptions"); o.Truthy() && o.Length() > 0 {
			return strings.TrimSpace(o.Index(0).Get("textContent").String())
		}
		return ""
	}
	// The readout box is sized by the bank's CSS, not by its own inline width.
	val := dotDisplay(optText(), false)
	val.Get("classList").Call("add", "dmdval")
	top.Call("appendChild", val)
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		setDotText(val, optText())
		return nil
	}))
}

// setDotText rewrites what a display shows, in the orientation and the
// number of characters it was built with.
func setDotText(win js.Value, text string) {
	n := bankChars
	if c, err := strconv.Atoi(win.Call("getAttribute", "data-chars").String()); err == nil && c > 0 {
		n = c
	}
	lit, dark := dotPaths(text, win.Get("classList").Call("contains", "dmdv").Bool(), n)
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
	const ns = "http://www.w3.org/2000/svg"
	win := dom.Doc.Call("createElement", "span")
	win.Set("className", "dmdwin")
	if vertical {
		win.Get("classList").Call("add", "dmdv")
	}
	// No title of its own: hovering the display shows its cell's, which
	// says what the legend abbreviates. Its own would only repeat the legend.
	svg := dom.Doc.Call("createElementNS", ns, "svg")
	svg.Call("setAttribute", "class", "dmd")
	win.Call("setAttribute", "data-chars", strconv.Itoa(n))
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
	win.Call("appendChild", svg)
	return win
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
	for _, a := range []string{"id", "data-mode", "data-bank", "data-bank-for", "title"} {
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
		d := dotDisplay("", e.Get("classList").Call("contains", "dmdv").Bool())
		if e.Get("classList").Call("contains", "dmdval").Bool() {
			d.Get("classList").Call("add", "dmdval")
		}
		e.Call("replaceWith", d)
	})
	strip("input", func(e js.Value) {
		e.Set("disabled", true)
		e.Set("value", "")
		e.Set("checked", false)
	})
	strip(".knob-ptr", func(e js.Value) {
		e.Get("style").Set("transform", "translate(-50%, -100%) rotate(-135deg)")
	})
	return c
}

// syncBankCells shows, in every bay, the step cell and the bank controls of
// the model that bay is set to, and the unassigned controls where that model
// has none.
func syncBankCells() {
	if !dom.Doc.Truthy() {
		return
	}
	want := map[string]bool{}
	for _, b := range rackBays {
		if m := bayModelOf(b); m != "" {
			want[m] = true
		}
	}
	show := func(c js.Value, on bool) {
		c.Get("style").Set("display", map[bool]string{true: "", false: "none"}[on])
	}
	for _, q := range [][2]string{{".stepcell[data-mode]", "data-mode"}, {".bankcell[data-bank]", "data-bank"}} {
		cells := dom.Doc.Call("querySelectorAll", q[0])
		for i := range cells.Get("length").Int() {
			c := cells.Index(i)
			show(c, want[c.Call("getAttribute", q[1]).String()])
		}
	}
	blanks := dom.Doc.Call("querySelectorAll", "[data-bank-for]")
	for i := range blanks.Get("length").Int() {
		c := blanks.Index(i)
		modes := strings.Fields(c.Call("getAttribute", "data-bank-for").String())
		at := slices.IndexFunc(modes, func(m string) bool { return want[m] })
		show(c, at >= 0)
		// An unassigned position still answers a hover: with what it is not.
		if at >= 0 && c.Get("classList").Call("contains", "bankblank").Bool() {
			c.Set("title", "Unassigned — "+modeLabel(modes[at])+" has no control in this position.")
		}
	}
	// What is showing in each position changed, so what each address is.
	scheduleDesignate()
}

// ── One part in every position ────────────────────────────────────────────
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
	switch {
	case sel.Truthy():
		if st := c.Call("querySelector", ".knobstack"); st.Truthy() {
			st.Call("remove")
		}
	case cb.Truthy():
		sel = switchAsSelect(c, cb)
		if sel.Truthy() {
			c.Call("appendChild", sel)
		}
	}
	if sel.Truthy() {
		// Its setting is read on a display (programLegends), not a number.
		if v := c.Call("querySelector", ".u-val"); v.Truthy() {
			v.Get("classList").Call("add", "swhidden")
		}
		wrap := dom.Doc.Call("createElement", "span")
		wrap.Set("className", "knobwrap")
		wrap.Call("setAttribute", "data-no-drag", "")
		knob := selk.makeSelectorKnob(sel)
		knob.Get("classList").Call("add", "knobb")
		wrap.Call("appendChild", knob)
		addValueDial(wrap, 0, 1)
		c.Call("appendChild", wrap)
		// No step: a setting has positions, not a resolution. The readout
		// and its knob are there, dark, because they are there on every
		// position of the bank.
		step := dom.Doc.Call("createElement", "input")
		step.Set("className", "numin u-step")
		step.Set("disabled", true)
		c.Call("appendChild", step)
		sk := dom.Doc.Call("createElement", "span")
		sk.Set("className", "stepknob stepnone")
		sk.Call("appendChild", dom.Doc.Call("createElement", "i"))
		c.Call("appendChild", sk)
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
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		if want := sel.Get("selectedIndex").Int() == 1; cb.Get("checked").Bool() != want {
			cb.Set("checked", want)
			cb.Call("dispatchEvent", js.Global().Get("Event").New("change", map[string]any{"bubbles": true}))
		}
		return nil
	}))
	cb.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		if i := at(); sel.Get("selectedIndex").Int() != i {
			sel.Set("selectedIndex", i)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
		return nil
	}))
	return sel
}
