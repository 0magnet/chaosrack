//go:build js && wasm

package attractor

import "syscall/js"

// Building the per-category model rows.
//
// One module per category, generated from modeGroups rather than written
// out, so a category added to the selector gets a row without a second list
// to keep in step. See rackcategory.go for why the rows exist and for the
// measurement that says each one fits an 84 HP bay.
//
// The rotaries INTERLOCK, which is the same mechanism the Rhythm module's
// preset tabs already use and for the same reason: the rack draws one model,
// so one row is driving it and the rest are showing what they would play.
// Choosing a model on any row puts every other row's rotary to OFF. Choosing
// OFF on the row that is driving snaps back, because a rack with nothing
// selected is not a state the instrument has.

// categoryOffLabel is the rotary's first position.
const categoryOffLabel = "off"

// catRotarySyncing guards the interlock against its own writes: putting the
// other rotaries to OFF dispatches change events, and without this each one
// would try to re-drive the interlock.
var catRotarySyncing bool

// buildCategoryModules creates the category rows, once.
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
	for _, label := range modelCategories() {
		if doc.Call("getElementById", categoryModuleID(label)).Truthy() {
			continue // already built
		}
		parent.Call("insertBefore", buildCategoryModule(label), host)
	}
	syncCategoryRotaries()
}

// categoryModuleID is the row's element id.
func categoryModuleID(label string) string { return "cat-" + categorySlug(label) + "-module" }

// categorySelectID is the row's rotary, the hidden select the knob drives.
func categorySelectID(label string) string { return "cat-" + categorySlug(label) + "-sel" }

// buildCategoryModule is one row: a rotary naming every model in the
// category, and OFF.
func buildCategoryModule(label string) js.Value {
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect catmodule")
	mod.Set("id", categoryModuleID(label))
	mod.Call("setAttribute", "data-cat", label)

	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", label)
	// What this category IS comes from the table beside the catalog, so the
	// header says something about the models rather than repeating the same
	// paragraph about rows eleven times. The paragraph follows it, once.
	what := catTooltips[label]
	if what == "" {
		what = label
	}
	h.Set("title", what+"\n\nEvery category has a row of its own, so the model you are running "+
		"is chosen where its controls are rather than from a single knob on the Console that "+
		"changed what the whole instrument was. The rotaries interlock: choosing a model here "+
		"puts the other rows to off, because the rack draws one model at a time. The row "+
		"driving the rack carries that model's parameters and its monitor.")
	mod.Call("appendChild", h)

	row := doc.Call("createElement", "div")
	row.Set("className", "row vmrow")

	cell := doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell")
	cell.Call("setAttribute", "data-no-drag", "")
	cell.Set("title", "Model — which of this category's models the row is set to. "+
		"OFF means another row is driving the display.")

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
	// "Chirikov Standard Map". Ringed anyway they ran clean out of the module
	// and across its neighbor's — eleven of these rotaries side by side in
	// the selector bay, each one's labels lying over the next one's knob.
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

	row.Call("appendChild", cell)
	mod.Call("appendChild", row)
	return mod
}

// onCategoryRotary drives the model from a row's own knob.
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
		// OFF on the row that is driving. A rack displays something, so the
		// knob comes back to where it was rather than leaving the instrument
		// with no model — the same way an interlocking tab cannot be
		// released except by pressing another one.
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

// syncCategoryRotaries puts every row's knob where the current model says it
// should be: the model's own row shows it, every other row shows OFF.
//
// Called after any change of model, from wherever — a permalink, a preset,
// the jam performer, another row's knob.
func syncCategoryRotaries() {
	if !doc.Truthy() {
		return
	}
	catRotarySyncing = true
	defer func() { catRotarySyncing = false }()
	// Which row is the instrument. Set here as well as in onModeChange so
	// that a path which syncs the knobs without going through it — the boot
	// pass, a recall — files the model's panels into the right row too.
	setActiveCategory(selectedMode)
	active := activeCategory
	for _, label := range modelCategories() {
		sel := doc.Call("getElementById", categorySelectID(label))
		if !sel.Truthy() {
			continue
		}
		want := ""
		if label == active {
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
}
