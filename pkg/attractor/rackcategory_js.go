//go:build js && wasm

package attractor

import (
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
	if doc.Call("getElementById", categoryHeadID(modelCategories()[0])).Truthy() {
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

// categorySelectID is a category's rotary: the hidden select the knob drives.
func categorySelectID(label string) string { return "cat-" + categorySlug(label) + "-sel" }

// categoryMonitorID and categoryMonitorSwitchID are the row's screen and the
// switch that powers it.
func categoryMonitorID(label string) string   { return "cat-" + categorySlug(label) + "-mon" }
func categoryMonSwitchID(label string) string { return categoryMonitorID(label) + "-on" }

// A CATEGORY IS A CAGE OF CARDS.
//
// Each model that has constants of its own is a card with its NAME on it —
// Lorenz, Chua, Aizawa — holding that model's constants and nothing else.
// The category's own module sits at the head of the run with the monitor,
// the rotary that chooses which card is playing, the step size, and any
// control that belongs to the category rather than to one of its models.
// The bay silkscreen marks the whole run.
//
// This replaces one wide grid per category with a silkscreened band over
// each model's columns. The band was the right idea and the wrong object: a
// group of controls with a name over it, sharing a panel with eleven other
// groups, IS a card — so it should be one, and then it has an honest width,
// it flows across bays the way any section too wide for one always has, and
// it can be dragged and put away like everything else in the rack.
//
// It also ends the continuation modules. "Attractors 2" existed because a
// module cannot overflow a bay and a category's grid could; no equipment has
// a panel called that. Fourteen cards called after fourteen systems is what
// the thing actually is.
//
// Two kinds of parameter do NOT go on a card:
//
//   - The step size. dt is not a constant of any system, it is how finely
//     the integrator walks it, so it belongs with the category's controls.
//     Every model's step cell is built and stacked in one place and the head
//     shows the one the row is set to. Sprott is the case that proves it:
//     nineteen of its twenty models have a step and nothing else, so on cards
//     they would be nineteen one-knob panels.
//
//   - A parameter several models declare. poly-op is the Conway operator
//     applied to whichever Platonic seed is chosen and takens-smooth is
//     shared by three of the Scope displays; neither is a property of any one
//     model, and putting it on the first card that happened to claim it would
//     say it was.

// buildCategoryRow builds one category's modules: its head, then a card for
// every model in it that has constants of its own.
//
// claimed is shared across rows so a parameter that two categories' models
// both declare is built once, by the first row that reaches it.
func buildCategoryRow(label string, claimed map[string]bool) []js.Value {
	own := categoryOwnModes(label)

	// Which parameters are the category's rather than a model's: the ones
	// more than one of its models declares.
	shared := map[string]int{}
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			shared[p.ID]++
		}
	}

	head := doc.Call("createElement", "div")
	head.Set("className", "punit-grid")
	head.Call("appendChild", buildCategoryMonitor(label))
	head.Call("appendChild", buildCategoryRotary(label))

	mods := []js.Value{}
	var cards []js.Value
	for _, mode := range own {
		card := doc.Call("createElement", "div")
		card.Set("className", "punit-grid")
		n := 0
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			switch {
			case strings.HasSuffix(p.ID, "-dt"):
				head.Call("appendChild", buildCategoryStepCell(mode, p))
			case shared[p.ID] > 1:
				head.Call("appendChild", buildCategoryParamCell(mode, p))
			default:
				card.Call("appendChild", buildCategoryParamCell(mode, p))
				n++
			}
		}
		if n == 0 {
			continue // nothing of its own: the head carries what it has
		}
		cards = append(cards, wrapCategoryModule(label, cardModuleID(mode), modeLabel(mode), card,
			modeInfo[mode].Label+" — this model's own constants. The step size and the model "+
				"knob are on the "+label+" panel at the head of the row."))
	}

	mods = append(mods, wrapCategoryModule(label, categoryHeadID(label), label, head, ""))
	return append(mods, cards...)
}

// categoryOwnModes is the models filed under this category — the ones whose
// cards and constants the row carries.
func categoryOwnModes(label string) []string {
	var out []string
	for _, mode := range categoryModes(label) {
		if categoryOf(mode) == label {
			out = append(out, mode)
		}
	}
	return out
}

// categoryHeadID and cardModuleID are the row's head panel and one model's
// card.
func categoryHeadID(label string) string { return "cat-" + categorySlug(label) + "-module" }
func cardModuleID(mode string) string    { return "card-" + categorySlug(mode) + "-module" }

// wrapCategoryModule puts a grid in a module of its own.
func wrapCategoryModule(label, id, title string, grid js.Value, tip string) js.Value {
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect catmodule")
	mod.Set("id", id)
	// The bay this module is in, declared rather than looked up from the
	// header: a card is named after its model, and the Analysis category
	// collides by name with the Analysis meter.
	mod.Call("setAttribute", "data-cat", label)

	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	if tip == "" {
		what := catTooltips[label]
		if what == "" {
			what = label
		}
		tip = what + "\n\nThe head of the row: the monitor, the knob that chooses which of " +
			"this category's models is playing, its integration step, and anything that " +
			"belongs to the category rather than to one model. Each model with constants " +
			"of its own has a card of its own further along the row."
	}
	h.Set("title", tip)
	mod.Call("appendChild", h)
	mod.Call("appendChild", grid)
	return mod
}

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
// Labeled by MODEL, not by parameter. Inside a block already headed "step"
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
