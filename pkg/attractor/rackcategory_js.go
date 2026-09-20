//go:build js && wasm

package attractor

import "syscall/js"

// Building the model selectors.
//
// Two modules, not eleven. Each category gets a rotary naming the models in
// it, generated from modeGroups rather than written out, so a category added
// to the selector gets its knob with no second list to keep in step.
//
// It WAS eleven: a module per category, each one a header, a single rotary,
// and two thirds of a panel with nothing on it. Measured, every one of them
// took a full 135px slot to carry one knob — eleven slots, nearly a whole
// 84 HP bay, for eleven controls that fit in four. A module is a card in a
// subrack; a card with one knob on it is a card that should have been three
// controls in somebody else's.
//
// So the idle rotaries are one BANK — eleven cells flowing three to a column
// the way every other multi-control module in the rack lays out, which is
// also Woodson & Conover's answer for six or more related controls (§2-132:
// "if there are groups of six or more, use rows or columns"). The rotary of
// the category that is RUNNING is not in the bank: it moves to the model's
// own row, beside the parameters and the monitor it belongs with, which is
// the same paragraph's first rule — a control belongs close to the display
// it affects. One knob moves per model change.
//
// The rotaries INTERLOCK, which is the mechanism the Rhythm module's preset
// tabs already use and for the same reason: the rack draws one model, so one
// rotary is driving it and the rest show what they would play. Choosing a
// model anywhere puts every other rotary to OFF. Choosing OFF on the one
// that is driving snaps back, because a rack with nothing selected is not a
// state the instrument has.

// categoryOffLabel is the rotary's first position.
const categoryOffLabel = "off"

// catRotarySyncing guards the interlock against its own writes: putting the
// other rotaries to OFF dispatches change events, and without this each one
// would try to re-drive the interlock.
var catRotarySyncing bool

// The two modules the rotaries live in, and the rows inside them that hold
// the cells.
const (
	modelsModuleID = "models-module" // the bank of idle rotaries
	modelsRowID    = "models-row"
	modelModuleID  = "model-module" // the running model's own rotary
	modelRowID     = "model-row"
)

// buildCategoryModules creates the two selector modules and every rotary in
// them, once.
//
// Inserted before the Parameters module, which is where the model's own
// panel goes and therefore where a reader looks after choosing one.
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
	if doc.Call("getElementById", modelsModuleID).Truthy() {
		return // already built
	}

	bank := buildSelectorModule(modelsModuleID, modelsRowID, "Models",
		"Models — a rotary per category of the catalog, each naming the models in it. "+
			"The rack draws one model, so the rotaries interlock: choosing a model here puts "+
			"every other category to off. The one you choose LEAVES this bank and appears on "+
			"the model's own row, beside its parameters and its monitor — which is where a "+
			"control belongs once it is the one doing something.")
	one := buildSelectorModule(modelModuleID, modelRowID, "Model",
		"Model — the rotary of the category that is running, on that category's own row "+
			"beside the parameters it selects and the monitor showing it. Turn it to reach "+
			"another model in the same category; the other categories are in the Models bank.")

	parent.Call("insertBefore", bank, host)
	parent.Call("insertBefore", one, host)

	bankRow := doc.Call("getElementById", modelsRowID)
	for _, label := range modelCategories() {
		bankRow.Call("appendChild", buildCategoryCell(label))
	}
	syncCategoryRotaries()
}

// buildSelectorModule is an empty module with one control row in it.
func buildSelectorModule(id, rowID, title, tip string) js.Value {
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect catmodule")
	mod.Set("id", id)

	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	h.Set("title", tip)
	mod.Call("appendChild", h)

	row := doc.Call("createElement", "div")
	row.Set("className", "row vmrow")
	row.Set("id", rowID)
	mod.Call("appendChild", row)
	return mod
}

// categorySelectID is a category's rotary: the hidden select the knob drives.
func categorySelectID(label string) string { return "cat-" + categorySlug(label) + "-sel" }

// categoryCellID is the cell the rotary sits in, which is what moves between
// the bank and the running model's row.
func categoryCellID(label string) string { return "cat-" + categorySlug(label) + "-cell" }

// buildCategoryCell is one category's rotary, in a cell of its own.
func buildCategoryCell(label string) js.Value {
	cell := doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell catcell")
	cell.Set("id", categoryCellID(label))
	cell.Call("setAttribute", "data-no-drag", "")
	// What this category IS comes from the table beside the catalog, so the
	// cell says something about the models rather than repeating the same
	// paragraph about interlocks eleven times.
	what := catTooltips[label]
	if what == "" {
		what = label
	}
	cell.Set("title", what+"\n\nOff means another category is driving the display.")

	top := doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", "plabel")
	// The CATEGORY, not the word "model": eleven cells side by side all
	// labeled "model" would be eleven identical labels over eleven different
	// knobs, and the label is the only thing telling them apart now that they
	// are not eleven modules with eleven headers.
	lbl.Set("textContent", categoryTag(label))
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
	// "Chirikov Standard Map". Ringed anyway they ran clean out of the cell
	// and across its neighbor's — eleven of these side by side, each one's
	// labels lying over the next one's knob.
	//
	// selectorKnobReadout is what that case is for, and what the parameter
	// cells already do with their own long lists (see buildParamUnit): the
	// dial keeps its detent action and the setting is named once, underneath,
	// where the name has the width of the cell to be read in.
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

// onCategoryRotary drives the model from a rotary.
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
		// OFF on the rotary that is driving. A rack displays something, so
		// the knob comes back to where it was rather than leaving the
		// instrument with no model — the same way an interlocking tab cannot
		// be released except by pressing another one.
		syncCategoryRotaries()
		return
	}
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

// homeCategoryCells records which category is running and puts the rotaries
// where that says: the driving one on the model's row, the rest in the bank.
//
// Separate from syncCategoryRotaries, and called BEFORE the panel is rebuilt,
// because the rack is re-measured and re-packed during that rebuild. Done
// afterwards, the pass that lays the rack out still saw the previous model's
// category on the row — so the rotary was silkscreened with the category it
// had just left, and sorted after its own Parameters and Monitor because
// those had already moved and it had not.
//
// It moves elements and sets an attribute; it dispatches nothing, which is
// what makes it safe to call twice in one mode change.
func homeCategoryCells() {
	if !doc.Truthy() {
		return
	}
	setActiveCategory(selectedMode)
	// The row's module declares which bay it is in, so the rack files it —
	// and the Parameters and Monitor that follow the model — into the running
	// category's row, rather than looking the name up from a header that is
	// the same in every mode.
	if mod := doc.Call("getElementById", modelModuleID); mod.Truthy() {
		mod.Call("setAttribute", "data-cat", activeCategory)
	}
	bank := doc.Call("getElementById", modelsRowID)
	row := doc.Call("getElementById", modelRowID)
	for _, label := range modelCategories() {
		cell := doc.Call("getElementById", categoryCellID(label))
		if !cell.Truthy() {
			continue
		}
		want := bank
		if label == activeCategory {
			want = row
		}
		if want.Truthy() && !cell.Get("parentNode").Equal(want) {
			want.Call("appendChild", cell)
		}
	}
}

// syncCategoryRotaries puts every rotary where the current model says it
// should be — the model's own category shows it, every other shows OFF — and
// homes the cells.
//
// Called after any change of model, from wherever: a permalink, a preset, the
// jam performer, another rotary.
func syncCategoryRotaries() {
	if !doc.Truthy() {
		return
	}
	catRotarySyncing = true
	defer func() { catRotarySyncing = false }()
	homeCategoryCells()
	active := activeCategory

	for _, label := range modelCategories() {
		sel := doc.Call("getElementById", categorySelectID(label))
		if !sel.Truthy() {
			continue
		}
		val := ""
		if label == active {
			val = selectedMode
		}
		if sel.Get("value").String() == val {
			continue
		}
		sel.Set("value", val)
		// Both events. The knob's pointer follows 'input'; the readout under
		// a rotary too long to ring its labels follows 'change', and without
		// it a rotary put to off still read out the model it used to be on.
		// Dispatching 'change' is safe because catRotarySyncing is exactly
		// what stops the interlock from answering its own writes.
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		sel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
}
