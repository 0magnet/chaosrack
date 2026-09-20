//go:build js && wasm

package attractor

import (
	"strconv"
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
// row puts every other row's rotary to OFF; choosing OFF on the row that is
// driving snaps back, because a rack with nothing selected is not a state
// the instrument has.

// categoryOffLabel is the rotary's first position.
const categoryOffLabel = "off"

// How much of a bay a category's first module spends before its parameters:
// the monitor is two columns wide and the rotary is one cell. A column is
// three cells and a bay is twelve columns, so the first module has ten
// columns left and the rotary takes a cell out of one of them.
const (
	catCellsPerCol  = 3
	catColsPerBay   = 12
	catMonitorCols  = 2
	catFirstCells   = (catColsPerBay - catMonitorCols) * catCellsPerCol // 30
	catLaterCells   = catColsPerBay * catCellsPerCol                    // 36
	catChunkMargin  = 3                                                 // leave a column spare for module chrome
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

// buildCategoryRow builds one category's modules: the first carries the
// monitor and the rotary, and as many parameters as a bay has room for; any
// that are left over continue into further modules.
//
// claimed is shared across rows so a parameter that two categories' models
// both declare is built once, by the first row that reaches it.
func buildCategoryRow(label string, claimed map[string]bool) []js.Value {
	// The cells, in model order, so a model's constants stay together and a
	// reader can see where one instrument's panel ends and the next begins.
	var cells []js.Value
	group := 0
	for _, mode := range categoryModes(label) {
		if categoryOf(mode) != label {
			continue // listed here but filed under the category that had it first
		}
		n := 0
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			c := buildCategoryParamCell(mode, p)
			// Alternating tint per model, so the eye can see where one
			// instrument's panel ends and the next begins. The grid flows by
			// column and a model's cells can start halfway down one, so a rule
			// between groups would fall in the middle of a column — Woodson &
			// Conover's other answer for the same job is "area color
			// patterning" (§2-133), which does not care where the break falls.
			c.Call("setAttribute", "data-mgroup", strconv.Itoa(group%2))
			cells = append(cells, c)
			n++
		}
		if n > 0 {
			group++
		}
	}

	var mods []js.Value
	n, first := 0, catFirstCells-catChunkMargin
	for {
		grid := doc.Call("createElement", "div")
		grid.Set("className", "punit-grid")
		if n == 0 {
			grid.Call("appendChild", buildCategoryMonitor(label))
			grid.Call("appendChild", buildCategoryRotary(label))
		}
		room := catLaterCells - catChunkMargin
		if n == 0 {
			room = first
		}
		take := room
		if take > len(cells) {
			take = len(cells)
		}
		for _, c := range cells[:take] {
			grid.Call("appendChild", c)
		}
		cells = cells[take:]

		title := label
		if n > 0 {
			title = label + " " + strconv.Itoa(n+1)
		}
		mods = append(mods, wrapCategoryModule(label, n, title, grid))
		n++
		if len(cells) == 0 {
			break
		}
	}
	return mods
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
	sel.Get("style").Set("display", "none")
	labels := []string{categoryOffLabel}
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
		labels = append(labels, name)
	}

	bay := doc.Call("createElement", "span")
	bay.Set("className", "grp vmbay")
	// Ringed round the dial only if the names will go round it, which for
	// model names they almost never do: ringLabelsFit allows eight labels of
	// five characters, and a category holds up to twenty with names like
	// "Chirikov Standard Map". Ringed anyway they run out of the cell and
	// across the parameters beside them. selectorKnobReadout is what that
	// case is for, and what the parameter cells already do with their own
	// long lists: the dial keeps its detent action and the setting is named
	// once, underneath, with the width of the cell to be read in.
	if ringLabelsFit(labels) {
		bay.Call("appendChild", singleSelectorKnob(sel, labels))
	} else {
		bay.Call("appendChild", selectorKnobReadout(sel))
	}
	cell.Call("appendChild", bay)
	cell.Call("appendChild", sel)

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
	if mode == "" || mode == selectedMode {
		// OFF on the row that is driving, or the model it is already on. A
		// rack displays something, so the knob comes back to where it was
		// rather than leaving the instrument with no model — the way an
		// interlocking tab cannot be released except by pressing another.
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
		want := ""
		if label == activeCategory {
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
		live := c.Call("getAttribute", "data-mode").String() == selectedMode
		c.Get("classList").Call("toggle", "live", live)
	}
}
