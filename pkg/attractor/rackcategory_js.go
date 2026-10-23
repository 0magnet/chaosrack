//go:build js && wasm

package attractor

import (
	"slices"

	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"strings"
	"syscall/js"
)

// One rack row per model category.
//
// A row is a whole instrument: its own monitor on the left, the rotary that
// picks which of its models is playing, and then the parameters of EVERY
// model in the category — not only the one running. See rackcategory.go for
// why the rows exist.
//
// Every model's knobs, always. That is the point of a rack: an instrument's
// front panel does not appear when you patch it in and vanish when you patch
// it out. It also makes the rows the size of what they hold, which nothing
// else here managed — a row per category with only the running model's panel
// on it was eleven rows of blank panel and one that was half full.
//
// Measured, the union of parameters per category:
//
//	Attractors 76   Scope 50   Audio 40   Maps 19
//	Geometry   13   Sequences 10   Analysis 4   Polyhedra 1
//	Solids      0   Custom     0                  = 213
//
// Attractors is 76 because the Sprott systems were folded into it; it was 55
// and they were 21. The total is unchanged — the same knobs, one heading
// fewer — and it is the widest category by some way, which is what the
// continuation modules below are for.
//
// Three of those are wider than an 84 HP bay, so those categories continue
// into a second module and the section flows into the next bay, which is
// what a section wider than a bay has always done here. A module cannot
// overflow a bay, which is why the continuation is a module and not a
// wrapped grid.
//
// A parameter has ONE knob in the rack. Its id is what the MIDI map, the
// permalink and Reset All address it by, so a second element with the same
// id is not a second view of a value, it is a bug in three features at once.
// Two consequences: the per-mode Parameters module stopped building knobs
// (see buildParamPanel), and where a parameter is shared by models in two
// categories the first row to claim it builds it — one variable, one knob.
//
// The rotaries INTERLOCK, the mechanism the Rhythm module's preset tabs use
// and for the same reason: the rack draws one model, so one row is driving
// it and the rest are showing what they would play. Choosing a model on any
// row puts every other row's rotary to OFF. Choosing OFF on the row that IS
// driving powers the rack down — which is what OFF on the model selector has
// always meant here, it being the first detent of the Console's old category
// knob. It used to snap back instead, and a detent you cannot reach is a
// broken control whatever the argument for it.

// categoryOffLabel is OFF: the category ring's position at the bottom, and the
// first of the model selector's on a bay that has no category ring.
const categoryOffLabel = "off"

// The row's geometry, in grid columns. A bay is twelve columns, a column
// holds three control positions, and a bay's head is two of those columns:
// the monitor across the top two rows, the model rotary and the step knob
// side by side underneath it. That is six positions with nothing blank in
// them, where the head used to be four columns to show three controls.
const (
	catCellsPerCol = 3
	catColsPerBay  = 12
	catHeadCols    = 2
	catMonitorRows = 2
	// The screen fills the two columns it is given, less the bezel.
	catMonitorWidth = 232
	catMonitorHigh  = 250
)

// catRotarySyncing guards the interlock against its own writes: putting the
// other rotaries to OFF dispatches change events, and without this each one
// would try to re-drive the interlock.
var catRotarySyncing bool

// catParamControls are the Controls of the category rows' parameter cells.
//
// Separate from paramControls, which buildParamPanel clears on every mode
// change: these are built once and outlive every rebuild, and clearing them
// would take the whole rack's knobs out of the control model. Kept so
// findBuiltControl can still match a cell to the Control the builder made.
var catParamControls []*Control

// buildCategoryModules creates every row, once.
//
// Inserted before the Parameters module, which is where the running model's
// readouts go and therefore where a reader looks after choosing one.
func buildCategoryModules() {
	if !dom.Doc.Truthy() {
		return
	}
	host := dom.Doc.Call("getElementById", "params-module")
	if !host.Truthy() {
		return
	}
	parent := host.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	if dom.Doc.Call("getElementById", categoryHeadID(rackRows()[0])).Truthy() {
		return // already built
	}
	// The controls a module of their own holds, it holds alone (homedmods_js.go).
	claimed := homedParamIDs()
	for _, label := range rackRows() {
		for _, mod := range buildCategoryRow(label, claimed) {
			parent.Call("insertBefore", mod, host)
		}
	}
	syncCategoryRotaries()
	buildHomedModules()
}

// buildCategoryRow builds one row: its bay head and the bank of programmable
// knobs beside it, one bay for every model in the row (see buildBankRow).
//
// Some parameters are not on a model's positions:
//
//   - The step size. dt is not a constant of any system, it is how finely
//     the integrator walks it, so the head shows the one belonging to the
//     model the bay is set to.
//
//   - A parameter several models declare. poly-op is the Conway operator
//     applied to whichever Platonic seed is chosen and takens-smooth is
//     shared by three of the Scope displays; neither is a property of any one
//     model, so it is one shared cell, shown for the models that declare it.
//
// claimed is shared across rows so a parameter that two rows' models both
// declare is built once, by the first row that reaches it.
func buildCategoryRow(label string, claimed map[string]bool) []js.Value {
	own := categoryOwnModes(label)

	// Which parameters are the row's rather than a model's: the ones more
	// than one of its models declares.
	shared := map[string]int{}
	declarers := map[string][]string{} // which models declare each parameter
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			shared[p.ID]++
			declarers[p.ID] = append(declarers[p.ID], mode)
		}
	}

	cells := map[string][]js.Value{}
	steps := map[string]js.Value{}
	var sharedCells []js.Value
	var hidden []js.Value // set from another position's buttons (bankHidden)
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			switch {
			case bankHidden(p.ID):
				hidden = append(hidden, buildCategoryParamCell(mode, p))
			case strings.HasSuffix(p.ID, "-dt"):
				steps[mode] = buildCategoryStepCell(mode, p)
			case shared[p.ID] > 1:
				// A shared cell, shown for the models that declare it and no
				// others (see buildBankRow).
				sc := newPUnit(mode, p)
				sc.Call("setAttribute", "data-bank-for", strings.Join(declarers[p.ID], " "))
				sharedCells = append(sharedCells, sc)
			case mode != own[0]:
				// Built when the model is first shown (banklazy_js.go); the
				// first model's are built now, for the bank's blanks to copy.
				cells[mode] = append(cells[mode], bankPlaceholder(mode, p))
			default:
				cells[mode] = append(cells[mode], newPUnit(mode, p))
			}
		}
	}

	// With each model's constants, the selectors its own panel used to
	// carry (modelparts_js.go).
	for _, mode := range own {
		cells[mode] = append(cells[mode], modelSelectorCells(mode)...)
	}

	// And Model Out's knobs (sonify_js.go), at the top of the last column
	// for every model with equations or a trail to play (bankTail).
	var heard []string
	for _, m := range own {
		if hasModelOut(m) {
			heard = append(heard, m)
		}
	}
	var mo []js.Value
	if len(heard) > 0 {
		for _, p := range modelOutParams {
			c := newPUnit(heard[0], p)
			c.Call("setAttribute", "data-bank-for", strings.Join(heard, " "))
			c.Set("title", helpFor(p.ID))
			mo = append(mo, c)
		}
	}
	// And Speed, the clock of the models that have one, under them (1.12.3):
	// dark for the rest, which it does nothing to.
	if sc := dom.Doc.Call("getElementById", "speed-cell"); sc.Truthy() && len(mo) > 0 {
		var timed []string
		for _, m := range own {
			if runsOnSpeed(m) {
				timed = append(timed, m)
			}
		}
		if len(timed) > 0 {
			sc.Call("setAttribute", "data-bank-for", strings.Join(timed, " "))
			withStepField(sc)
			sc.Call("remove") // out of the holder: the bank places a shared cell that has no place yet
			mo = append(mo, sc)
		}
	}
	groups, gcells := homedBankGroups(own)
	return buildBankRow(label, own, cells, steps, sharedCells, hidden, mo, groups, gcells)
}

// ── Families ────────────────────────────────────────────────────────────
//
// A family is a run of models that differ only in which member of a
// catalog they are: J. C. Sprott's nineteen simple systems, A to S. On the
// bay's MODEL knob they were nineteen positions of twenty-four, so reaching
// Chen meant turning past all of them. Now they are ONE position, and a
// lettered dial of their own picks which system that position plays.

// modelFamily is one such run.
type modelFamily struct {
	Key     string
	Label   string   // what the MODEL position reads
	Members []string // the models, in dial order
	Letters []string // the dial's ring, one per member

	sel     js.Value   // the family's own dial
	options []js.Value // the MODEL position that stands for it, per bay
	chosen  string     // the member the dial is set to
}

// modelFamilies is every family; Sprott's is derived from the catalog so a
// system added there joins the dial without being listed twice.
var modelFamilies = func() []*modelFamily {
	f := &modelFamily{Key: "sprott", Label: "Sprott A–S"}
	for _, g := range modeGroups {
		for _, m := range g.Keys {
			if len(m) == len("sprotta") && strings.HasPrefix(m, "sprott") && !slices.Contains(f.Members, m) {
				f.Members = append(f.Members, m)
				f.Letters = append(f.Letters, strings.ToUpper(m[len("sprott"):]))
			}
		}
	}
	return []*modelFamily{f}
}()

// familyOf is the family a model belongs to, or nil.
func familyOf(mode string) *modelFamily {
	for _, f := range modelFamilies {
		if slices.Contains(f.Members, mode) {
			return f
		}
	}
	return nil
}

// current is the member the family's MODEL position plays.
func (f *modelFamily) current() string {
	if f.chosen == "" && len(f.Members) > 0 {
		f.chosen = f.Members[0]
	}
	return f.chosen
}

// choose points the family at a member: its dial and every MODEL position
// that stands for it. It does not change what is running.
func (f *modelFamily) choose(mode string) {
	if !slices.Contains(f.Members, mode) {
		return
	}
	f.chosen = mode
	// The MODEL position reads which member it plays: that window is a
	// readout, so it may say "Sprott C" where the ring says nothing.
	text := strings.Fields(f.Label)[0] + " " + f.Letters[slices.Index(f.Members, mode)]
	for _, o := range f.options {
		o.Set("value", mode)
		o.Set("textContent", text)
	}
	if f.sel.Truthy() && f.sel.Get("value").String() != mode {
		f.sel.Set("value", mode)
		dom.Fire(f.sel, "change")
		dom.Fire(f.sel, "input")
	}
}

// buildFamilyCell is the family's dial as one bank position, built without
// the printed knob, which the P-unit's own replaces.
func buildFamilyCell(f *modelFamily) js.Value {
	sel := buildFamilySelect(f)
	cell := dom.Doc.Call("createElement", "div")
	cell.Set("className", "punit")
	cell.Set("title", docf("fam-cell", "family", f.Label))
	top := dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "u-lbl")
	lbl.Set("textContent", "sys")
	top.Call("appendChild", lbl)
	cell.Call("appendChild", top)
	cell.Call("appendChild", sel)
	return cell
}

// buildBayHead is the bay's own panel: its monitor, the selector that picks
// among the generators in this bay, and the step size.
//
// Two columns, six positions, nothing blank. The monitor takes the top two
// rows across both columns — it is a screen, so the space it is given is
// space it uses — and the rotary and the step knob sit side by side under
// it. It was four columns for the same three controls, because the step had
// a position reserved for every model in the bay while showing one: they are
// stacked in ONE position now, which is what the panel always looked like.
//
// Given a bank's grid, the rotary and the step go to the head of the bank
// instead, at the top of its first column and at its foot, and the row
// under the monitor is the running model's equation (eqSlotID).
func buildBayHead(label string, bay int, modes []string, steps map[string]js.Value, bank js.Value) js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid cathead")
	at(buildCategoryMonitor(label, bay), grid, 1, catMonitorRows, 1, catHeadCols)
	rotary, rotRow, rotCol := grid, 1+catMonitorRows, 1
	stepRow, stepCol := 1+catMonitorRows, 2
	if bank.Truthy() {
		rotary, rotRow, rotCol = bank, bankRotaryRow, 1
		stepRow, stepCol = bankStepRow, 1
		slot := dom.Doc.Call("createElement", "div")
		slot.Set("className", "eqslot")
		slot.Set("id", eqSlotID(label))
		mountModelReadouts(slot) // over the equation: modelparts_js.go
		at(slot, grid, 1+catMonitorRows, 1, 1, catHeadCols)
	}
	at(buildBayRotary(label, bay, modes), rotary, rotRow, 1, rotCol, 1)

	// Every step cell in the bay goes in the same position. Only one is ever
	// shown — the one belonging to the model the bay is set to — so one
	// position is what the panel needs, and a knob that re-ranges with the
	// selector is what it has always been from the front.
	for _, m := range modes {
		if s, ok := steps[m]; ok {
			at(s, rotary, stepRow, 1, stepCol, 1)
		}
	}

	title := label
	if bay > 0 {
		title = label + " " + strconv.Itoa(bay+1)
	}
	mod := wrapCategoryModule(label, categoryHeadID(label)+"-"+strconv.Itoa(bay), title, grid, "")
	// Declared on the module, because the rack packer works on the DOM and
	// has no idea a category was ever divided into bays. Without it the
	// packer refills the bays by width and this panel lands wherever it
	// fits, which for nine of the eleven heads was not the left of a row.
	mod.Call("setAttribute", bayHeadAttr, "1")
	return mod
}

// categoryOwnModes is the models filed under this row — a category, or every
// category of a merged row, in catalog order — whose constants it carries.
func categoryOwnModes(label string) []string {
	var out []string
	for _, cat := range rowCategories(label) {
		for _, mode := range categoryModes(cat) {
			if categoryOf(mode) == cat && !slices.Contains(out, mode) {
				out = append(out, mode)
			}
		}
	}
	return out
}

// categoryHeadID is the row.s head panel.
func categoryHeadID(label string) string { return "cat-" + categorySlug(label) + "-module" }

// wrapCategoryModule puts a grid in a module of its own.
func wrapCategoryModule(label, id, title string, grid js.Value, tip string) js.Value {
	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect")
	mod.Set("id", id)
	// The bay this module is in, declared rather than looked up from the
	// header: a card is named after its model, and the Analysis category
	// collides by name with the Analysis meter.
	mod.Call("setAttribute", "data-cat", label)

	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	if tip == "" {
		what := catTooltips[label]
		if what == "" {
			what = label
		}
		tip = docf("cat-head", "what", what)
	}
	h.Set("title", tip)
	mod.Call("appendChild", h)
	mod.Call("appendChild", grid)
	return mod
}

// belongs to so the row can light the ones that are running.
func buildCategoryParamCell(mode string, p paramDef) js.Value {
	return buildCategoryParamCellAs(mode, p, false)
}

// buildCategoryParamCellAs is buildCategoryParamCell, built for a P-unit
// when pu is set (newPUnit).
func buildCategoryParamCellAs(mode string, p paramDef, pu bool) js.Value {
	before := len(paramControls)
	unit := buildParamUnitAs(mode, p, pu)
	// The Controls the builder just made belong to the rack for good, not to
	// the next panel rebuild, which clears paramControls.
	if len(paramControls) > before {
		catParamControls = append(catParamControls, paramControls[before:]...)
		paramControls = paramControls[:before]
	}
	unit.Call("setAttribute", "data-mode", mode)
	// Which parameter, for the buttons beside it (rackbuttons_js.go).
	unit.Call("setAttribute", "data-param", p.ID)
	// WHICH MODEL's constant this is. A row carries up to fifty-five cells
	// and the label on one of them is a symbol: Chen's a and Lu's a are the
	// same letter over two different knobs, and only the model tells them
	// apart. The name is on the cell rather than in the label because the
	// label is one glyph wide by design.
	name := mode
	if info, ok := modeInfo[mode]; ok && info.Label != "" {
		name = info.Label
	}
	// And what it does, where the parameter has a sentence: the label is one
	// glyph or an abbreviation, and the cell's tooltip is where it is spelled
	// out (the legend display in a bank has none of its own).
	tip := name + " — " + p.Label
	if h := helpFor(p.ID); h != "" {
		tip = name + " — " + h
	}
	unit.Set("title", tip)
	return unit
}

// buildCategoryStepCell is a model's integration step, for the position in
// the bay head that shows the step of whichever model the bay is set to.
//
// Labeled "dt", the same on every model's cell. It was labeled with the
// model's name, which made the legend change as the MODEL knob turned — and
// a legend is printed on the panel: only a readout may change. Which model's
// step it is, the MODEL knob and the monitor already say.
func buildCategoryStepCell(mode string, p paramDef) js.Value {
	unit := buildCategoryParamCell(mode, p)
	unit.Get("classList").Call("add", "stepcell")
	return unit
}

// ── Bays ──────────────────────────────────────────────────────────────────
//
// A bay is the unit of selection now, not a category. It has one monitor and
// one selector, and the selector offers the generators mounted in that bay.
// A category with more generators than a bay holds simply takes more bays,
// and every one of them is silkscreened with the category's name.
//
// That is what a rack of this kind looks like: a row of units with a screen
// and a selector at the left of it, and the selector switches between what
// is IN that row. It also fixes the thing a category-wide selector could not
// — with fourteen attractors on one knob, reaching the last of them means
// turning past thirteen.

// bayRecord is one bay: which category it belongs to, its index within that
// category, and the generators mounted in it.
type bayRecord struct {
	Label string
	N     int
	Modes []string
}

// racksBays is every bay in the rack, in the order they are built.
var rackBays []bayRecord

// bayID names a bay's elements.
func bayID(label string, n int) string { return "bay-" + categorySlug(label) + "-" + strconv.Itoa(n) }

// bayMonitorID, bayMonSwitchID and baySelectID are the bay's screen, the
// switch that powers it, and the selector that picks what it plays.
func bayMonitorID(label string, n int) string   { return bayID(label, n) + "-mon" }
func bayMonSwitchID(label string, n int) string { return bayMonitorID(label, n) + "-on" }
func baySelectID(label string, n int) string    { return bayID(label, n) + "-sel" }

// bayOf is the bay a model is mounted in, or nil.
func bayOf(mode string) *bayRecord {
	for i := range rackBays {
		if slices.Contains(rackBays[i].Modes, mode) {
			return &rackBays[i]
		}
	}
	return nil
}

// bayModel is what each bay is set to, remembered while another bay drives.
//
// The selectors interlock — the rack draws one model — so a bay that is not
// driving reads OFF and cannot say which of its generators it is on. It
// still has one: it is what comes back when the bay is turned on, and it is
// what that bay's step cell and monitor caption follow.
var bayModel = map[string]string{}

// bayModelOf is what a bay is set to: what it was last turned to, or its
// first generator before it has been turned at all.
func bayModelOf(b bayRecord) string {
	if m := bayModel[bayID(b.Label, b.N)]; m != "" {
		return m
	}
	if len(b.Modes) > 0 {
		return b.Modes[0]
	}
	return ""
}

// buildCategoryMonitor is a bay's screen: the model, while this bay is the
// one driving the rack, and standing by when another is.
func buildCategoryMonitor(label string, bay int) js.Value {
	unit := dom.Doc.Call("createElement", "span")
	unit.Set("className", "punit monunit")
	unit.Call("setAttribute", "data-no-drag", "")

	// The tooltip is the screen's and not the unit's: the screen is a part
	// with an address of its own (designate), and the switches under it
	// have theirs.
	bez := dom.Doc.Call("createElement", "span")
	bez.Set("className", "monbezel")
	bez.Set("title", doc("bay-monitor"))
	cv := dom.Doc.Call("createElement", "canvas")
	cv.Set("id", bayMonitorID(label, bay))
	cv.Set("width", catMonitorWidth)
	cv.Set("height", catMonitorHigh)
	bez.Call("appendChild", cv)
	// The screen also carries the running model's Lyapunov exponent, as a
	// scope carries its measurements: a readout of the picture on the
	// picture, rather than a module of its own holding one number.
	ro := dom.Doc.Call("createElement", "span")
	ro.Set("className", "monread")
	ro.Set("id", bayMonitorID(label, bay)+"-lyap")
	bez.Call("appendChild", ro)
	unit.Call("appendChild", bez)

	row := dom.Doc.Call("createElement", "span")
	row.Set("className", "monsw")
	row.Call("appendChild", screenSwitch(bayMonSwitchID(label, bay)))
	buildModelSwitches(row) // what each does is the model's: modelparts_js.go
	unit.Call("appendChild", row)
	return unit
}

// screenSwitch is the monitor's power switch: the same part as the model's
// switches beside it, a dot-matrix legend over a switch, so the row under
// the screen reads as one row of switches.
func screenSwitch(id string) js.Value {
	lab := dom.Doc.Call("createElement", "label")
	lab.Set("className", "modsw")
	lab.Call("setAttribute", "data-no-drag", "")
	lab.Set("title", doc("bay-screen"))
	lab.Call("appendChild", dotDisplayN("scrn", false, legendChars))
	sw := dom.Doc.Call("createElement", "input")
	sw.Set("type", "checkbox")
	sw.Set("className", "sw")
	sw.Set("id", id)
	sw.Set("checked", true)
	lab.Call("appendChild", sw)
	return lab
}

// buildBayRotary is the cell that picks which of this bay's generators plays.
func buildBayRotary(label string, bay int, modes []string) js.Value {
	rackBays = append(rackBays, bayRecord{Label: label, N: bay, Modes: modes})

	cell := dom.Doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell catcell")
	cell.Call("setAttribute", "data-no-drag", "")
	cell.Set("title", doc("bay-model-cell"))

	top := dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "plabel")
	lbl.Set("textContent", "model")
	top.Call("appendChild", lbl)
	cell.Call("appendChild", top)

	sel := dom.Doc.Call("createElement", "select")
	sel.Set("id", baySelectID(label, bay))
	sel.Set("className", "selwin")
	sel.Set("title", doc("bay-model"))

	bay2 := dom.Doc.Call("createElement", "span")
	bay2.Set("className", "grp vmbay")
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "catsel")

	// A knob AND a list, which is what the Console's model selector was. A
	// dial alone is the wrong control for a long list: reaching the last of
	// them means dragging through all the others while the rack re-renders
	// at each. The list is how you go somewhere; the knob is how you walk.
	knob := selk.makeSelectorKnob(sel)
	var stack js.Value
	if cats := rowCategories(label); len(cats) > 1 {
		// A merged row: the outer ring picks the category, the inner ring the
		// model within it. The inner ring only ever holds one category's
		// models, so walking it is walking a short list, and the outer ring's
		// legends are printed because the categories never change.
		catSel := dom.Doc.Call("createElement", "select")
		catSel.Set("id", bayCatSelectID(label, bay))
		catSel.Set("title", doc("bay-category"))
		catSel.Get("style").Set("display", "none")
		// OFF is the category ring's, at the bottom, and the ring is endless
		// (selEndless): turned on past the last category it comes round to
		// OFF and on to the first. OFF was the inner ring's first position,
		// where it sat among the models as though it were one.
		catSel.Call("setAttribute", "data-endless", "")
		tags := []string{categoryOffLabel}
		off := dom.Doc.Call("createElement", "option")
		off.Set("value", "")
		off.Set("textContent", categoryOffLabel)
		catSel.Call("appendChild", off)
		for _, c := range cats {
			o := dom.Doc.Call("createElement", "option")
			o.Set("value", c)
			o.Set("textContent", c)
			catSel.Call("appendChild", o)
			tags = append(tags, categoryRingTag(c))
		}
		catSel.Set("value", cats[0])
		fillBayOptions(sel, modesOfCategory(modes, cats[0]), false)
		stack = stackKnobs(selk.makeSelectorKnob(catSel), knob)
		addSelectorLabels(stack, tags, catSel)
		stack.Call("setAttribute", "title", doc("bay-rings"))
		wrap.Call("appendChild", catSel)
		lb, nb := label, bay
		selOverflow[baySelectID(label, bay)] = func(dir int) { onBayModelOverflow(lb, nb, dir) }
		dom.On(catSel, "change", func(js.Value, []js.Value) any {
			onBayCategory(lb, nb)
			return nil
		})
	} else {
		fillBayOptions(sel, modes, true)
		stack = dom.Doc.Call("createElement", "span")
		stack.Set("className", "knobstack")
		stack.Call("setAttribute", "data-no-drag", "")
		knob.Get("classList").Call("add", "knob-ring")
		stack.Call("appendChild", knob)
	}
	wrap.Call("appendChild", stack)
	wrap.Call("appendChild", sel)
	// A merged bay's display opens every model in the bay, by category
	// (bayPicker); the knob's own select, one category's worth, is hidden.
	picker := sel
	if cats := rowCategories(label); len(cats) > 1 {
		picker = bayPicker(label, bay, sel, cats, modes)
		sel.Get("style").Set("display", "none")
		wrap.Call("appendChild", picker)
	}
	bay2.Call("appendChild", wrap)
	cell.Call("appendChild", bay2)
	attachSelMarquee(sel, picker)

	lb, nb := label, bay
	dom.On(sel, "change", func(js.Value, []js.Value) any {
		onBayRotary(lb, nb)
		return nil
	})
	return cell
}

// bayCatSelectID is a merged bay's category ring.
func bayCatSelectID(label string, n int) string { return bayID(label, n) + "-cat" }

// categoryRingTag is a category's name as the category ring prints it: four
// letters, which is what fits between two detents.
func categoryRingTag(cat string) string {
	r := []rune(strings.ToLower(categoryTag(cat)))
	return string(r[:min(4, len(r))])
}

// modesOfCategory is the models of modes filed under cat, in their order.
func modesOfCategory(modes []string, cat string) []string {
	var out []string
	for _, m := range modes {
		if categoryOf(m) == cat {
			out = append(out, m)
		}
	}
	return out
}

// fillBayOptions puts a bay selector's positions in: OFF when withOff (a bay
// with no category ring, which has nowhere else for it), then the models,
// a family standing as one position whose value is the member its dial is
// set to (see modelFamily).
func fillBayOptions(sel js.Value, modes []string, withOff bool) {
	sel.Set("innerHTML", "")
	for _, f := range modelFamilies {
		f.options = slices.DeleteFunc(f.options, func(o js.Value) bool { return !o.Get("parentNode").Truthy() })
	}
	add := func(value, text string) js.Value {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", value)
		o.Set("textContent", text)
		sel.Call("appendChild", o)
		return o
	}
	if withOff {
		add("", categoryOffLabel)
	}
	seen := map[*modelFamily]bool{}
	for _, m := range modes {
		if f := familyOf(m); f != nil {
			if !seen[f] {
				seen[f] = true
				o := add(f.current(), f.Label)
				o.Call("setAttribute", "data-family", f.Key)
				f.options = append(f.options, o)
				f.choose(f.current()) // the position reads its member
			}
			continue
		}
		add(m, modeLabel(m))
	}
}

// bayCatModel is the model each category of a merged bay was last set to, so
// turning the category ring away and back returns to it.
var bayCatModel = map[string]string{}

// onBayCategory follows a merged bay's category ring: the inner ring gets
// that category's models and the bay switches to the one it was last on.
func onBayCategory(label string, bay int) {
	if catRotarySyncing {
		return
	}
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	if !catSel.Truthy() || !sel.Truthy() {
		return
	}
	cat := catSel.Get("value").String()
	if cat == "" {
		// OFF, at the bottom of the ring, and it means off: the rack powers
		// down, which is what OFF on the model selector has always meant.
		setPowerState(false)
		setPowerSwitch(false)
		syncCategoryRotaries()
		return
	}
	var modes []string
	for _, r := range rackBays {
		if r.Label == label && r.N == bay {
			modes = modesOfCategory(r.Modes, cat)
		}
	}
	if len(modes) == 0 {
		return
	}
	fillBayOptions(sel, modes, false)
	m := bayCatModel[bayID(label, bay)+"/"+cat]
	if m == "" {
		m = modes[0]
		if f := familyOf(m); f != nil {
			m = f.current()
		}
	}
	sel.Set("value", m)
	dom.Fire(sel, "change")
	dom.Fire(sel, "input")
}

// onBayModelOverflow is a merged bay's model ring turned past its last model
// (dir +1) or before its first (-1): it goes on into the next category's first
// model, or the previous one's last, as the category ring follows. Past the
// last category, or back before the first, it comes to OFF, where the category
// ring has it, and from OFF on to the first model of the first category or the
// last of the last: the two rings walk one order.
func onBayModelOverflow(label string, bay int, dir int) {
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	if !catSel.Truthy() {
		return
	}
	var bayModes []string
	for _, r := range rackBays {
		if r.Label == label && r.N == bay {
			bayModes = r.Modes
		}
	}
	cats := rowCategories(label)
	at := slices.Index(cats, catSel.Get("value").String())
	if at < 0 && dir < 0 {
		at = len(cats) // from OFF, backwards, to the last category
	}
	for range cats {
		at += dir
		if at < 0 || at >= len(cats) {
			catSel.Set("value", "") // OFF (onBayCategory powers down)
			dom.Fire(catSel, "change")
			dom.Fire(catSel, "input")
			return
		}
		modes := modesOfCategory(bayModes, cats[at])
		if len(modes) == 0 {
			continue
		}
		m := modes[0]
		if dir < 0 {
			m = modes[len(modes)-1]
		}
		if f := familyOf(m); f != nil {
			m = f.current()
		}
		// Where onBayCategory puts the model ring, as it would a category's
		// remembered model.
		bayCatModel[bayID(label, bay)+"/"+cats[at]] = m
		catSel.Set("value", cats[at])
		dom.Fire(catSel, "change")
		dom.Fire(catSel, "input")
		return
	}
}

// setBayCategory turns a merged bay's category ring to cat without driving
// anything, and gives the inner ring that category's models.
func setBayCategory(b bayRecord, cat string) (refilled bool) {
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(b.Label, b.N))
	if !catSel.Truthy() || catSel.Get("value").String() == cat {
		return false
	}
	catSel.Set("value", cat)
	dom.Fire(catSel, "change")
	dom.Fire(catSel, "input")
	if sel := dom.Doc.Call("getElementById", baySelectID(b.Label, b.N)); sel.Truthy() {
		fillBayOptions(sel, modesOfCategory(b.Modes, cat), false)
	}
	return true
}

// onBayRotary drives the model from a bay's own selector.
func onBayRotary(label string, bay int) {
	if catRotarySyncing {
		return
	}
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	if !sel.Truthy() {
		return
	}
	mode := sel.Get("value").String()
	if mode == "" {
		// OFF, and it means off: the rack powers down, which is what OFF on
		// a model selector has always meant here.
		setPowerState(false)
		setPowerSwitch(false)
		syncCategoryRotaries()
		return
	}
	bayModel[bayID(label, bay)] = mode
	bayCatModel[bayID(label, bay)+"/"+categoryOf(mode)] = mode
	setPowerState(true)
	setPowerSwitch(true)
	if mode == editMode() {
		syncCategoryRotaries()
		return
	}
	ms := dom.Doc.Call("getElementById", "mode-select")
	if !ms.Truthy() {
		return
	}
	ms.Set("value", mode)
	dom.Fire(ms, "change")
}

// setPowerSwitch moves the Console's Power switch without firing it, so the
// switch and the selectors always say the same thing about whether the rack
// is running.
func setPowerSwitch(on bool) {
	if sw := dom.Doc.Call("getElementById", "power-sw"); sw.Truthy() {
		sw.Set("checked", on)
	}
}

// syncCategoryRotaries puts every bay's selector where the current model
// says it should be: the bay holding it shows it, every other shows OFF.
//
// Called after any change of model, from wherever: a permalink, a preset,
// the jam performer, another bay's selector.
func syncCategoryRotaries() {
	if !dom.Doc.Truthy() {
		return
	}
	catRotarySyncing = true
	defer func() { catRotarySyncing = false }()
	// The model the panel is on: the running one, or the back layer's while
	// that is being edited (backlayer_js.go).
	m := editMode()
	setActiveCategory(m)
	// A running family member points its family at itself first, so the
	// MODEL position that stands for it has the value about to be set.
	if f := familyOf(m); f != nil {
		f.choose(m)
	}
	live := bayOf(m)
	if live != nil && m != "" && !run.stopped {
		bayModel[bayID(live.Label, live.N)] = m
	}
	for _, b := range rackBays {
		sel := dom.Doc.Call("getElementById", baySelectID(b.Label, b.N))
		if !sel.Truthy() {
			continue
		}
		// Powered down, every selector reads off: no bay is driving,
		// because nothing is being drawn.
		refilled := false
		want := ""
		if live != nil && b.Label == live.Label && b.N == live.N && !run.stopped {
			want = m
			// A merged bay turns its category ring to the running model's
			// category first, so the inner ring holds the model to show.
			refilled = setBayCategory(b, categoryOf(m))
		}
		if want == "" {
			// A merged bay says OFF on its category ring, where OFF is; its
			// model ring, which has no OFF, points at nothing and reads off.
			if catSel := dom.Doc.Call("getElementById", bayCatSelectID(b.Label, b.N)); catSel.Truthy() {
				if catSel.Get("value").String() != "" {
					catSel.Set("value", "")
					dom.Fire(catSel, "change")
					dom.Fire(catSel, "input")
				}
				if sel.Get("selectedIndex").Int() >= 0 {
					sel.Set("selectedIndex", -1)
					dom.Fire(sel, "change")
				}
				continue
			}
		}
		// A refilled ring is on its first model already, which the browser
		// selects: when that is the model, it still has to say so, or the
		// display and the pointer stay on the category before.
		if !refilled && sel.Get("value").String() == want {
			continue
		}
		sel.Set("value", want)
		// Both events. The knob's pointer follows 'input'; the marquee
		// readout follows 'change', and without it a bay put to off still
		// reads out the model it used to be on. Dispatching 'change' is safe
		// because catRotarySyncing is what stops the interlock answering its
		// own writes.
		dom.Fire(sel, "change")
		dom.Fire(sel, "input")
	}
	// The model lists follow too: a select already on its model above was
	// skipped, and fired nothing for its list to follow.
	for _, follow := range bayPickerFollow {
		follow()
	}
	syncBankCells()
	lightLiveParamCells()
}

// lightLiveParamCells marks the cells of the model that is running.
//
// Two hundred and thirteen knobs are on the rack and three of them are doing
// something. Without this the rows are a reference rather than an instrument:
// you can read every model's settings and not see which ones are live.
func lightLiveParamCells() {
	// Nothing is lit while the rack is powered down, which is the same answer
	// the rotaries give: no model is running, so no front panel on the rack is
	// the one in the signal path. A bank's shared cell is the running model's
	// too if it is one of the models the cell serves. One pass in the page:
	// from here it was a few calls a cell over every model's cells.
	model := editMode()
	if run.stopped {
		model = ""
	}
	fastDOM().Call("lightLive", model)
}

// at places an element in a module's grid: a row, a starting column, and
// how many of each it spans.
//
// Explicit, not flowed. The grid's own auto-flow fills three cells and moves
// right, which is right for one generator's panel and useless for three:
// each would start wherever the last one ended, and a tile with a name over
// it has to start where the name does.
func at(el, grid js.Value, row, rowSpan, col, colSpan int) {
	st := el.Get("style")
	st.Set("gridRow", strconv.Itoa(row)+" / span "+strconv.Itoa(rowSpan))
	st.Set("gridColumn", strconv.Itoa(col)+" / span "+strconv.Itoa(colSpan))
	grid.Call("appendChild", el)
}

// buildFamilySelect is the family's dial as a control: a select of its
// members, lettered, that switches to the member it lands on when a member is
// running and otherwise only sets which one the MODEL position will play.
// Mounted as a module of its own beside a card row, or as the inner ring of
// a bank bay's MODEL knob.
func buildFamilySelect(f *modelFamily) js.Value {
	sel := dom.Doc.Call("createElement", "select")
	sel.Set("id", "fam-"+f.Key)
	sel.Set("title", docf("fam-select", "family", f.Label))
	sel.Get("style").Set("display", "none")
	for i, m := range f.Members {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", m)
		o.Set("textContent", f.Letters[i])
		o.Set("title", modeLabel(m))
		sel.Call("appendChild", o)
	}
	sel.Set("value", f.current())
	f.sel = sel
	// A control of the rack, as the MODEL knob is: without it neither ctl, a
	// panel in a terminal nor the native rack could reach a member past the
	// first. Its module is the bank's, where its cell is drawn.
	registerControl(ControlDesc{ID: "fam-" + f.Key, Label: "sys", IsSelect: true, SelectDef: f.Members[0]}, "visual · model")
	dom.On(sel, "change", func(js.Value, []js.Value) any {
		if catRotarySyncing {
			return nil
		}
		m := sel.Get("value").String()
		f.choose(m)
		if em := editMode(); familyOf(em) == f && m != em && !run.stopped {
			if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
				ms.Set("value", m)
				dom.Fire(ms, "change")
			}
		}
		return nil
	})

	return sel
}

// bayPickerFollow is each model list's way of catching up with its knob.
var bayPickerFollow = map[string]func(){}

// bayPicker is the list a merged bay's model display opens: every model in
// the bay under its category's name, the categories' names separators that
// cannot be chosen, and beside each model how many of the bay's controls it
// has (bankControls), so a model can be weighed before it is switched to.
// Choosing one turns both rings to it, as the knob would; sel is the knob's
// own select, which it follows.
func bayPicker(label string, bay int, sel js.Value, cats, modes []string) js.Value {
	p := dom.Doc.Call("createElement", "select")
	p.Set("id", baySelectID(label, bay)+"-all")
	p.Set("className", "selwin")
	p.Set("title", doc("bay-model-all"))
	// OFF first, as on the rings: the bay driving nothing.
	off := dom.Doc.Call("createElement", "option")
	off.Set("value", "")
	off.Set("textContent", categoryOffLabel)
	p.Call("appendChild", off)
	for _, c := range cats {
		g := dom.Doc.Call("createElement", "optgroup")
		g.Set("label", c)
		fillBayOptions(g, modesOfCategory(modes, c), false)
		p.Call("appendChild", g)
	}
	// Follows the knob, whichever moved it; a family's entry follows the
	// member it is on, and so does its count. syncCategoryRotaries calls it
	// too, for the moves that fire nothing.
	bayPickerCounts(p, false)
	follow := func() {
		bayPickerCounts(p, true)
		if sel.Truthy() && sel.Get("selectedIndex").Int() >= 0 {
			p.Set("value", sel.Get("value").String())
		} else {
			p.Set("value", "") // OFF
		}
	}
	follow()
	bayPickerFollow[p.Get("id").String()] = follow
	if sel.Truthy() {
		dom.On(sel, "change", func(js.Value, []js.Value) any {
			follow()
			return nil
		})
	}
	dom.On(p, "change", func(js.Value, []js.Value) any {
		onBayPick(label, bay, p.Get("value").String())
		return nil
	})
	// In the registry, as every other control is: it is the one that says
	// what the bay plays, and without it neither ctl, a panel in a terminal
	// nor the native rack's catalog could change the model.
	registerControl(ControlDesc{ID: p.Get("id").String(), Label: "Model", IsSelect: true}, strings.ToLower(label))
	return p
}

// bayPickerCounts writes each model's control count after its name, the
// names padded to one width so the counts stand in a column. A family's
// entry is whichever member its dial is on, so its count can change; with
// families, only theirs are rewritten.
func bayPickerCounts(p js.Value, families bool) {
	opts := p.Call("querySelectorAll", "optgroup option")
	if !families {
		// The first time: every name kept, and the width they pad to.
		w := 0
		for i := range opts.Length() {
			o := opts.Index(i)
			name := o.Get("textContent").String()
			o.Call("setAttribute", "data-name", name)
			w = max(w, len([]rune(name)))
		}
		p.Call("setAttribute", "data-w", strconv.Itoa(w))
	} else {
		opts = p.Call("querySelectorAll", "option[data-family]")
	}
	w, _ := strconv.Atoi(p.Call("getAttribute", "data-w").String()) //nolint:errcheck // written just above, from an int
	for i := range opts.Length() {
		o := opts.Index(i)
		name := o.Call("getAttribute", "data-name").String()
		pad := strings.Repeat(" ", w-len([]rune(name))+2)
		o.Set("textContent", name+pad+strconv.Itoa(bankControls[o.Get("value").String()]))
	}
}

// onBayPick turns a merged bay to model m from its list: the category ring
// to m's category, which brings the model ring with it to m; or, for OFF,
// the category ring to OFF, which powers the rack down.
func onBayPick(label string, bay int, m string) {
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	if !sel.Truthy() || !catSel.Truthy() {
		return
	}
	cat := ""
	if m != "" {
		cat = categoryOf(m)
		bayCatModel[bayID(label, bay)+"/"+cat] = m
	}
	if catSel.Get("value").String() != cat {
		catSel.Set("value", cat)
		dom.Fire(catSel, "change")
		dom.Fire(catSel, "input")
		return
	}
	sel.Set("value", m)
	dom.Fire(sel, "change")
}
