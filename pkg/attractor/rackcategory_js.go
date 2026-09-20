//go:build js && wasm

package attractor

import (
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
//	Attractors 55   Scope 50   Audio 40   Sprott 21   Maps 19
//	Geometry   13   Sequences 10   Analysis 4   Polyhedra 1
//	Solids      0   Custom     0                       = 213
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

// categoryOffLabel is the rotary's first position.
const categoryOffLabel = "off"

// The row's geometry, in grid columns. A bay is twelve, a column holds
// three cells, and the head of the first module spends two columns on the
// monitor and one on the model rotary.
const (
	catCellsPerCol  = 3
	catColsPerBay   = 12
	catMonitorCols  = 2
	catRotaryCols   = 1
	catMonitorWidth = 232
	catMonitorHigh  = 174
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
	if !doc.Truthy() {
		return
	}
	host := doc.Call("getElementById", "params-module")
	if !host.Truthy() {
		return
	}
	parent := host.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	if doc.Call("getElementById", categoryModuleID(modelCategories()[0], 0)).Truthy() {
		return // already built
	}
	claimed := map[string]bool{}
	for _, label := range modelCategories() {
		for _, mod := range buildCategoryRow(label, claimed) {
			parent.Call("insertBefore", mod, host)
		}
	}
	syncCategoryRotaries()
}

// categoryModuleID is a row's nth module.
func categoryModuleID(label string, n int) string {
	return "cat-" + categorySlug(label) + "-" + strconv.Itoa(n) + "-module"
}

// categorySelectID is a category's rotary: the hidden select the knob drives.
func categorySelectID(label string) string { return "cat-" + categorySlug(label) + "-sel" }

// categoryMonitorID and categoryMonitorSwitchID are the row's screen and the
// switch that powers it.
func categoryMonitorID(label string) string   { return "cat-" + categorySlug(label) + "-mon" }
func categoryMonSwitchID(label string) string { return categoryMonitorID(label) + "-on" }

// modelGroup is one model's parameters, kept together and named.
//
// Fourteen of the Attractors' models have a step size and all fourteen are
// labelled "dt"; seven have a constant labelled "a". A row that puts them
// side by side with nothing between them is a row of knobs that read as one
// knob drawn over and over.
//
// The CONSTANTS cannot be merged, and it is worth being exact about why,
// because the labels suggest otherwise: chen-a runs 10..50 and rossler-a
// runs 0.01..1 — the same letter naming two unrelated quantities. A knob
// that silently meant a different quantity depending on what was playing
// would be worse than fourteen honest ones. So each model's constants take
// whole columns under a header carrying the model's name.
//
// The STEP is the other case, and it is one control. dt is not a constant of
// any system; it is how finely the integrator walks it, the same setting
// with the same meaning in every one of them. Its range does differ per
// model — chen-dt is 0.0001..0.05 against lorenz-dt's 0.001..0.05, a stiffer
// system needing finer steps — but a range is something a knob is given when
// the model changes, not a reason for fourteen knobs. So the row has ONE
// step cell, and it is whichever model the row is set to.
//
// Implemented by building every model's step cell and showing one, rather
// than by one cell rebound on each change. The cells are already correct —
// each has its own range, its own default and its own id — and that id is
// what the permalink, the MIDI map and Reset All address. A single rebound
// knob would have to fake all of that; a hidden cell simply keeps it.
// Sprott's row goes from seven columns of identical-looking dt to one.
type modelGroup struct {
	mode    string
	name    string
	cells   []js.Value
	stacked bool // all cells in one place, one of them shown
}

// cols is how many grid columns the group needs, three cells to a column —
// or one, when the cells are stacked and only ever one is visible.
func (g modelGroup) cols() int {
	if g.stacked {
		return 1
	}
	return (len(g.cells) + catCellsPerCol - 1) / catCellsPerCol
}

// buildCategoryRow builds one category's modules: the first carries the
// monitor and the rotary, then as many model groups as a bay has room for,
// and any that are left over continue into further modules.
//
// claimed is shared across rows so a parameter that two categories' models
// both declare is built once, by the first row that reaches it.
func buildCategoryRow(label string, claimed map[string]bool) []js.Value {
	var groups []modelGroup
	// Every model's step cell, stacked in one place. One of them shows.
	steps := modelGroup{name: "step  dt", stacked: true}
	for _, mode := range categoryModes(label) {
		if categoryOf(mode) != label {
			continue // listed here but filed under the category that had it first
		}
		g := modelGroup{mode: mode, name: modeLabel(mode)}
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			if strings.HasSuffix(p.ID, "-dt") {
				steps.cells = append(steps.cells, buildCategoryStepCell(mode, p))
				continue
			}
			g.cells = append(g.cells, buildCategoryParamCell(mode, p))
		}
		if len(g.cells) > 0 {
			groups = append(groups, g)
		}
	}
	// Last, so a row reads constants-then-steps: the constants are what the
	// model IS and the step is how finely it is walked.
	if len(steps.cells) > 0 {
		groups = append(groups, steps)
	}

	var mods []js.Value
	for n := 0; ; n++ {
		grid := doc.Call("createElement", "div")
		grid.Set("className", "punit-grid catgrid")
		col := 1
		if n == 0 {
			placeCategoryHead(grid, label)
			col += catMonitorCols + catRotaryCols
		}
		// As many whole groups as the bay has room for. A group is never
		// split across modules: half a model's front panel at the end of one
		// bay and the rest at the start of the next is the thing the headers
		// are here to prevent.
		took := 0
		for _, g := range groups {
			if took > 0 && col+g.cols() > catColsPerBay+1 {
				break
			}
			placeModelGroup(grid, g, col, took+n)
			col += g.cols()
			took++
		}
		groups = groups[took:]

		title := label
		if n > 0 {
			title = label + " " + strconv.Itoa(n+1)
		}
		mods = append(mods, wrapCategoryModule(label, n, title, grid))
		if len(groups) == 0 {
			return mods
		}
	}
}

// placeCategoryHead puts the row's screen and its model rotary in the first
// three columns, each under a label of its own.
func placeCategoryHead(grid js.Value, label string) {
	at(groupHeader("screen"), grid, 1, 1, 1, catMonitorCols)
	at(buildCategoryMonitor(label), grid, 2, catCellsPerCol, 1, catMonitorCols)
	at(groupHeader("model"), grid, 1, 1, 1+catMonitorCols, catRotaryCols)
	at(buildCategoryRotary(label), grid, 2, catCellsPerCol, 1+catMonitorCols, catRotaryCols)
}

// placeModelGroup puts one model's header and cells into their columns.
func placeModelGroup(grid js.Value, g modelGroup, col, tint int) {
	at(groupHeader(g.name), grid, 1, 1, col, g.cols())
	for i, c := range g.cells {
		// Alternating ground per group as well as the header, so the extent
		// of a group is readable at a glance and not only at its start —
		// Woodson & Conover's "area color patterning" beside their "marked
		// outlines around each group" (§2-133).
		c.Call("setAttribute", "data-mgroup", strconv.Itoa(tint%2))
		if g.stacked {
			// All in the one cell. syncStepCells shows whichever model the
			// row is set to; the rest are display:none, which costs no layout
			// and no paint while keeping their ids addressable.
			at(c, grid, 2, 1, col, 1)
			continue
		}
		at(c, grid, 2+i%catCellsPerCol, 1, col+i/catCellsPerCol, 1)
	}
}

// groupHeader is the silkscreen over a group of columns.
func groupHeader(name string) js.Value {
	h := doc.Call("createElement", "span")
	h.Set("className", "pghdr")
	h.Set("textContent", name)
	h.Set("title", name)
	return h
}

// at places an element in the grid: a row, a starting column, and how many
// of each it spans.
//
// Placed explicitly rather than flowed. The grid used to be auto-flow
// column, which fills three cells and moves right — fine for one model's
// panel and wrong for eleven, because a group then starts wherever the
// previous one happened to end and no header can sit over it.
func at(el, grid js.Value, row, rowSpan, col, colSpan int) {
	st := el.Get("style")
	st.Set("gridRow", strconv.Itoa(row)+" / span "+strconv.Itoa(rowSpan))
	st.Set("gridColumn", strconv.Itoa(col)+" / span "+strconv.Itoa(colSpan))
	grid.Call("appendChild", el)
}

// wrapCategoryModule puts a grid in a module of its own.
func wrapCategoryModule(label string, n int, title string, grid js.Value) js.Value {
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect catmodule")
	mod.Set("id", categoryModuleID(label, n))
	// The bay this module is in, declared rather than looked up from the
	// header: two modules of the same row carry different headers, and the
	// Analysis category collides by name with the Analysis meter.
	mod.Call("setAttribute", "data-cat", label)

	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	what := catTooltips[label]
	if what == "" {
		what = label
	}
	h.Set("title", what+"\n\nThe row is the whole category: its own monitor, the rotary that "+
		"chooses which of its models is playing, and the constants of every model in it. "+
		"The knobs of the model that is running are lit; the rest are the panels of "+
		"instruments that are patched out, which is why they are still here to set.")
	mod.Call("appendChild", h)
	mod.Call("appendChild", grid)
	return mod
}

// buildCategoryParamCell is one parameter's cell, tagged with the model it
// belongs to so the row can light the ones that are running.
func buildCategoryParamCell(mode string, p paramDef) js.Value {
	before := len(paramControls)
	unit := buildParamUnit(mode, p)
	// The Controls the builder just made belong to the rack for good, not to
	// the next panel rebuild, which clears paramControls.
	if len(paramControls) > before {
		catParamControls = append(catParamControls, paramControls[before:]...)
		paramControls = paramControls[:before]
	}
	unit.Call("setAttribute", "data-mode", mode)
	// WHICH MODEL's constant this is. A row carries up to fifty-five cells
	// and the label on one of them is a symbol: Chen's a and Lu's a are the
	// same letter over two different knobs, and only the model tells them
	// apart. The name is on the cell rather than in the label because the
	// label is one glyph wide by design.
	name := mode
	if info, ok := modeInfo[mode]; ok && info.Label != "" {
		name = info.Label
	}
	unit.Set("title", name+" — "+p.Label)
	return unit
}

// buildCategoryStepCell is a model's integration step, for the block that
// gathers every model's.
//
// Labelled by MODEL, not by parameter. Inside a block already headed "step"
// the useful word on a cell is which system's step it is; "dt" written on
// every one of them is exactly what made the row read as one knob repeated.
func buildCategoryStepCell(mode string, p paramDef) js.Value {
	unit := buildCategoryParamCell(mode, p)
	unit.Get("classList").Call("add", "stepcell")
	if l := unit.Call("querySelector", ".u-lbl"); l.Truthy() {
		l.Set("textContent", categoryTag(modeLabel(mode)))
		// The model name is words, and a cell label is set upright down the
		// left edge, which is right for "dt" and wrong for "Newton-Leipnik".
		l.Get("classList").Call("add", "wordlbl")
	}
	return unit
}

// buildCategoryMonitor is the row's screen: the model, when this row is the
// one driving it, and dark when it is not.
func buildCategoryMonitor(label string) js.Value {
	unit := doc.Call("createElement", "span")
	unit.Set("className", "punit monunit")
	unit.Call("setAttribute", "data-no-drag", "")
	unit.Set("title", "Monitor — this row's screen. It shows the model while this row is the "+
		"one driving the rack, and stands by when another row is: only one model is drawn, "+
		"so only one screen can have a picture. The switch under it cuts the copy out of "+
		"the drawing buffer that the picture costs.")

	bez := doc.Call("createElement", "span")
	bez.Set("className", "monbezel")
	cv := doc.Call("createElement", "canvas")
	cv.Set("id", categoryMonitorID(label))
	cv.Set("width", catMonitorWidth)
	cv.Set("height", catMonitorHigh)
	bez.Call("appendChild", cv)
	unit.Call("appendChild", bez)

	lab := doc.Call("createElement", "label")
	lab.Set("className", "grp")
	lab.Get("style").Set("cursor", "pointer")
	sw := doc.Call("createElement", "input")
	sw.Set("type", "checkbox")
	sw.Set("className", "sw")
	sw.Set("id", categoryMonSwitchID(label))
	sw.Set("checked", true)
	lab.Call("appendChild", sw)
	lab.Call("appendChild", doc.Call("createTextNode", " Screen"))
	unit.Call("appendChild", lab)
	return unit
}

// buildCategoryRotary is the cell that picks which of the category's models
// is playing.
func buildCategoryRotary(label string) js.Value {
	cell := doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell catcell")
	cell.Call("setAttribute", "data-no-drag", "")
	cell.Set("title", "Model — which of this category's models is playing. Off means another "+
		"row is driving the rack; the knobs on this row stay where you set them either way.")

	top := doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", "plabel")
	lbl.Set("textContent", "model")
	top.Call("appendChild", lbl)
	cell.Call("appendChild", top)

	sel := doc.Call("createElement", "select")
	sel.Set("id", categorySelectID(label))
	sel.Set("className", "selwin")
	sel.Set("title", "Model — pick one from the list, or turn the knob above it")
	add := func(value, text string) {
		o := doc.Call("createElement", "option")
		o.Set("value", value)
		o.Set("textContent", text)
		sel.Call("appendChild", o)
	}
	add("", categoryOffLabel)
	for _, m := range categoryModes(label) {
		name := m
		if info, ok := modeInfo[m]; ok && info.Label != "" {
			name = info.Label
		}
		add(m, name)
	}

	bay := doc.Call("createElement", "span")
	bay.Set("className", "grp vmbay")
	// A KNOB AND A LIST, which is what the Console's model selector was.
	//
	// A dial alone is the wrong control for this: Sprott has twenty positions
	// and the Scope ten, and reaching the last of them means dragging through
	// all the others while the rack re-renders at each one. The list is how
	// you GO somewhere; the knob is how you walk. The same select drives
	// both, so the two can never disagree.
	//
	// The names are not ringed round the dial either way: ringLabelsFit
	// allows eight labels of five characters, and these are up to twenty of
	// "Chirikov Standard Map". Ringed anyway they ran out of the cell and
	// across the parameters beside them. The list is the readout now.
	stack := doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)

	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "catsel")
	wrap.Call("appendChild", stack)
	wrap.Call("appendChild", sel)
	bay.Call("appendChild", wrap)
	cell.Call("appendChild", bay)
	// After the select is in the document: the marquee wraps it in place, and
	// it is the same treatment the Console's dropdowns had — the native text
	// is hidden and a readout overlays it, scrolling when a name overflows,
	// because "Chirikov Standard Map" does not fit a cell either.
	attachSelMarquee(sel, "#7fe0a0")

	sel.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		onCategoryRotary(label)
		return nil
	}))
	return cell
}

// onCategoryRotary drives the model from a row's own rotary.
func onCategoryRotary(label string) {
	if catRotarySyncing {
		return
	}
	sel := doc.Call("getElementById", categorySelectID(label))
	if !sel.Truthy() {
		return
	}
	mode := sel.Get("value").String()
	if mode == "" {
		// OFF, and it means off.
		//
		// It used to snap back, on the argument that a rack with no model
		// selected is not a state the instrument has. That was wrong twice
		// over: the instrument does have the state — it is what the Power
		// switch puts it in — and a detent you cannot reach is a broken
		// control whatever the argument for it. OFF on the model selector
		// has always meant this here; it was the first detent of the
		// Console's category knob, and it called setPowerState too.
		setPowerState(false)
		setPowerSwitch(false)
		syncCategoryRotaries()
		return
	}
	rowModel[label] = mode
	// Choosing a model powers the rack back up, including when it is the
	// model already selected — which is how a row comes back on after being
	// switched off without having to pass through a different model first.
	setPowerState(true)
	setPowerSwitch(true)
	if mode == selectedMode {
		syncCategoryRotaries()
		return
	}
	ms := doc.Call("getElementById", "mode-select")
	if !ms.Truthy() {
		return
	}
	ms.Set("value", mode)
	ms.Call("dispatchEvent", js.Global().Get("Event").New("change"))
}

// setPowerSwitch moves the Console's Power switch without firing it, so the
// switch and the rotaries always say the same thing about whether the rack
// is running.
func setPowerSwitch(on bool) {
	if sw := doc.Call("getElementById", "power-sw"); sw.Truthy() {
		sw.Set("checked", on)
	}
}

// syncCategoryRotaries puts every rotary where the current model says it
// should be: the model's own category shows it, every other shows OFF.
//
// Called after any change of model, from wherever: a permalink, a preset, the
// jam performer, another row's rotary.
func syncCategoryRotaries() {
	if !doc.Truthy() {
		return
	}
	catRotarySyncing = true
	defer func() { catRotarySyncing = false }()
	setActiveCategory(selectedMode)
	for _, label := range modelCategories() {
		sel := doc.Call("getElementById", categorySelectID(label))
		if !sel.Truthy() {
			continue
		}
		// Powered down, every rotary reads off: no row is driving, because
		// nothing is being drawn. The knobs keep their settings, the models
		// keep theirs, and choosing one anywhere powers back up.
		want := ""
		if label == activeCategory && !stopped {
			want = selectedMode
		}
		if sel.Get("value").String() == want {
			continue
		}
		sel.Set("value", want)
		// Both events. The knob's pointer follows 'input'; the readout under
		// a rotary too long to ring its labels follows 'change', and without
		// it a row put to off still read out the model it used to be on.
		// Dispatching 'change' is safe because catRotarySyncing is exactly
		// what stops the interlock from answering its own writes.
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		sel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	if activeCategory != "" && selectedMode != "" {
		rowModel[activeCategory] = selectedMode
	}
	syncStepCells()
	lightLiveParamCells()
}

// lightLiveParamCells marks the cells of the model that is running.
//
// Two hundred and thirteen knobs are on the rack and three of them are doing
// something. Without this the rows are a reference rather than an instrument:
// you can read every model's settings and not see which ones are live.
func lightLiveParamCells() {
	cells := doc.Call("querySelectorAll", ".punit[data-mode]")
	for i := 0; i < cells.Get("length").Int(); i++ {
		c := cells.Index(i)
		// Nothing is lit while the rack is powered down, which is the same
		// answer the rotaries give: no model is running, so no front panel
		// on the rack is the one in the signal path.
		live := !stopped && c.Call("getAttribute", "data-mode").String() == selectedMode
		c.Get("classList").Call("toggle", "live", live)
	}
}

// ── Putting a whole row away ───────────────────────────────────────────────
//
// Every model's knobs, always, is 213 parameter cells and 11 screens, and
// that is most of the panel's cost: the DOM went from 3748 nodes to 10966
// when the rows were filled, the relayout sweep from 0.30ms to 2.1ms, and
// the panel from about a tenth of the frame budget to about a fifth.
//
// A switch per row is the answer a rack already has for this. Taking a row
// out is display:none on its modules, which costs the browser nothing to
// keep — no layout, no paint, no measurement — so a rack cut down to the
// three categories somebody actually uses is as cheap as the rack was
// before the rows were filled. The models in a row that is out still play:
// this is putting the front panel away, not unplugging the instrument.
//
// The switches are on the CONSOLE and not on the rows, because a switch
// that goes away with the thing it hides cannot bring it back.

// rowHiddenKey is where the put-away rows are remembered. Its own record
// rather than the rack layout's: that one lists which switches are ON, so an
// empty record means everything off, and the right default here is that
// every row is in the rack.
const rowHiddenKey = "wasmstuff-rackrows-out"

// rowSwitchID is a category's row switch.
func rowSwitchID(label string) string { return "row-" + categorySlug(label) + "-sw" }

// buildRowSwitches fills the Console's Rows group, one switch per category.
func buildRowSwitches() {
	host := doc.Call("getElementById", "row-switches")
	if !host.Truthy() {
		return
	}
	out := readHiddenRows()
	for _, label := range modelCategories() {
		lab := doc.Call("createElement", "label")
		lab.Set("className", "grp")
		lab.Get("style").Set("cursor", "pointer")
		what := catTooltips[label]
		if what == "" {
			what = label
		}
		lab.Set("title", what+"\n\nIn the rack, or put away. A row that is out costs nothing "+
			"to keep — no layout, no paint, no measurement — which is what the switch is "+
			"for: every model's knobs, always, is 213 controls and most of the panel's "+
			"cost. The models in a row that is out still play; this puts the front panel "+
			"away, not the instrument.")
		sw := doc.Call("createElement", "input")
		sw.Set("type", "checkbox")
		sw.Set("className", "sw")
		sw.Set("id", rowSwitchID(label))
		sw.Set("checked", !out[label])
		lab.Call("appendChild", sw)
		lab.Call("appendChild", doc.Call("createTextNode", " "+categoryTag(label)))
		host.Call("appendChild", lab)

		sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			applyRowVisibility()
			saveHiddenRows()
			quantizeModuleWidths() // the rack is a different size now
			return nil
		}))
	}
	applyRowVisibility()
}

// applyRowVisibility puts each row in or out to match its switch.
func applyRowVisibility() {
	for _, label := range modelCategories() {
		sw := doc.Call("getElementById", rowSwitchID(label))
		in := !sw.Truthy() || sw.Get("checked").Bool()
		mods := doc.Call("querySelectorAll", "[data-cat]")
		for i := 0; i < mods.Get("length").Int(); i++ {
			m := mods.Index(i)
			if m.Call("getAttribute", "data-cat").String() != label {
				continue
			}
			if in {
				m.Get("style").Set("display", "")
			} else {
				m.Get("style").Set("display", "none")
			}
		}
	}
}

// readHiddenRows is the set of categories that were put away.
func readHiddenRows() map[string]bool {
	out := map[string]bool{}
	v, ok := lsGet(rowHiddenKey)
	if !ok || v == "" {
		return out
	}
	for _, s := range strings.Split(v, ",") {
		if s != "" {
			out[s] = true
		}
	}
	return out
}

// saveHiddenRows records it, by label, so a category renamed comes back
// rather than staying out under a name nothing matches.
func saveHiddenRows() {
	var out []string
	for _, label := range modelCategories() {
		sw := doc.Call("getElementById", rowSwitchID(label))
		if sw.Truthy() && !sw.Get("checked").Bool() {
			out = append(out, label)
		}
	}
	lsSet(rowHiddenKey, strings.Join(out, ","))
}

// ── Which step cell shows ──────────────────────────────────────────────────

// rowModel is the model each row is set to, remembered even while the row is
// not the one driving the rack.
//
// The rotaries interlock, so a row that is not driving reads OFF and cannot
// say which of its models it is on. The row still has one — it is what comes
// back when you turn the row on — and the step cell has to follow it, or a
// row that is off shows the step of whatever model happens to be first.
var rowModel = map[string]string{}

// rowModelOf is the model a row is set to: what it was last turned to, or
// the first model in the category before it has been turned at all.
func rowModelOf(label string) string {
	if m := rowModel[label]; m != "" {
		return m
	}
	if ms := categoryModes(label); len(ms) > 0 {
		return ms[0]
	}
	return ""
}

// syncStepCells shows one step cell per row: the one belonging to the model
// that row is set to.
func syncStepCells() {
	if !doc.Truthy() {
		return
	}
	want := make(map[string]string, len(modeGroups))
	for _, label := range modelCategories() {
		want[label] = rowModelOf(label)
	}
	cells := doc.Call("querySelectorAll", ".stepcell[data-mode]")
	for i := 0; i < cells.Get("length").Int(); i++ {
		c := cells.Index(i)
		mode := c.Call("getAttribute", "data-mode").String()
		if mode == want[categoryOf(mode)] {
			c.Get("style").Set("display", "")
			continue
		}
		c.Get("style").Set("display", "none")
	}
}
