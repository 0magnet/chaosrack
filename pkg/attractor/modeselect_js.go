//go:build js && wasm

package attractor

import (
	_ "embed"
	"strconv"
	"syscall/js"
)

var selWindow js.Value

// updateSelWindow shows the current dropdown selection's label in the
// selector-knob window. Called on every mode change.
func updateSelWindow() {
	if !selWindow.Truthy() {
		return
	}
	sel := doc.Call("getElementById", "mode-select")
	if !sel.Truthy() {
		return
	}
	opts := sel.Get("selectedOptions")
	if opts.Truthy() && opts.Get("length").Int() > 0 {
		selWindow.Set("textContent", opts.Index(0).Get("textContent").String())
	}
}

// ---- Nested model selector (category ring + model knob) ----

// modelOpt is one selectable model: its #mode-select value and display label.
type modelOpt struct{ value, label string }

var (
	nestedCatOrder  []string              // category display order
	nestedCatModels map[string][]modelOpt // category -> its models
	nestedModeCat   map[string]string     // model value -> category
	nestedSyncing   bool                  // guards sync from re-propagating
)

// catShortLabel is the short tag printed around the outer selector ring.
func catShortLabel(cat string) string {
	switch cat {
	case "Attractors":
		return "ATTR"
	case "Polyhedra":
		return "POLY"
	case "Geometry":
		return "GEO"
	case "Audio":
		return "AUD"
	case "Scope":
		return "SCOPE"
	case "Custom":
		return "CUST"
	case "OFF":
		return "OFF"
	}
	return cat
}

// nestedOffCat is the synthetic 6th outer-knob category: turning to it powers
// the model off (replacing the old PWR switch). It holds no models.
const nestedOffCat = "OFF"

// setPowerState stops or resumes the render loop. Off blanks the canvas but
// keeps the panel; on restarts the loop only if it was actually stopped (so
// switching categories while already running doesn't spawn a second RAF loop).
func setPowerState(on bool) {
	if on {
		if stopped {
			stopped = false
			js.Global().Call("requestAnimationFrame", renderFrame)
		}
		return
	}
	stopped = true
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", glTypes.ColorBufferBit)
	gl.Call("clear", glTypes.DepthBufferBit)
}

// optgroupCategory folds the finer #mode-select optgroups into the top-level
// categories the outer knob switches between (Sprott systems join Attractors).
func optgroupCategory(label string) string {
	switch label {
	case "Polyhedra", "Geometry", "Audio", "Scope":
		return label
	}
	// Attractors + Sprott systems + (Custom, now folded in) → "Attractors".
	return "Attractors"
}

// buildNestedModelSelector reads the hidden #mode-select's optgroups into the
// category/model tables, fills the two visible dropdowns, and wires the
// concentric selector knobs (outer ring = category, inner = model). The hidden
// #mode-select stays the single source of truth: the dropdowns write to it and
// dispatch its change; syncNestedFromMode reads it back.
// attachSelMarquee caps a Console <select> to one unit and overlays a marquee
// readout of the current option: the native text is hidden (see CSS), a
// click-through overlay shows the name, and long names that overflow scroll
// back and forth so they stay readable.
func attachSelMarquee(sel js.Value, colorHex string) {
	parent := sel.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "selwrap")
	parent.Call("insertBefore", wrap, sel)
	wrap.Call("appendChild", sel)
	marq := doc.Call("createElement", "span")
	marq.Set("className", "selmarq")
	if colorHex != "" {
		marq.Get("style").Set("color", colorHex)
	}
	inner := doc.Call("createElement", "span")
	marq.Call("appendChild", inner)
	wrap.Call("appendChild", marq)
	upd := func() {
		idx := sel.Get("selectedIndex").Int()
		txt := ""
		if idx >= 0 {
			txt = sel.Get("options").Index(idx).Get("text").String()
		}
		inner.Set("textContent", txt)
		over := inner.Get("offsetWidth").Int() - marq.Get("clientWidth").Int()
		if over > 4 {
			marq.Get("style").Call("setProperty", "--marq-shift", "-"+strconv.Itoa(over+6)+"px")
			marq.Get("classList").Call("add", "scroll")
		} else {
			marq.Get("classList").Call("remove", "scroll")
		}
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
	upd()
}

func buildNestedModelSelector() {
	mode := doc.Call("getElementById", "mode-select")
	catSel := doc.Call("getElementById", "cat-select")
	modelSel := doc.Call("getElementById", "model-select")
	holder := doc.Call("getElementById", "modelknob-holder")
	if !mode.Truthy() || !catSel.Truthy() || !modelSel.Truthy() || !holder.Truthy() {
		return
	}

	nestedCatOrder = nil
	nestedCatModels = map[string][]modelOpt{}
	nestedModeCat = map[string]string{}
	groups := mode.Call("querySelectorAll", "optgroup")
	for i := 0; i < groups.Get("length").Int(); i++ {
		g := groups.Index(i)
		cat := optgroupCategory(g.Get("label").String())
		if _, seen := nestedCatModels[cat]; !seen {
			nestedCatOrder = append(nestedCatOrder, cat)
		}
		opts := g.Call("querySelectorAll", "option")
		for j := 0; j < opts.Get("length").Int(); j++ {
			o := opts.Index(j)
			mo := modelOpt{o.Get("value").String(), o.Get("textContent").String()}
			nestedCatModels[cat] = append(nestedCatModels[cat], mo)
			nestedModeCat[mo.value] = cat
		}
	}
	// Synthetic FIRST category = OFF (power). Placing it before Attractors means
	// one detent counter-clockwise from Attractors reaches OFF (no need to spin
	// all the way around past Custom). No models; selecting it stops.
	nestedCatOrder = append([]string{nestedOffCat}, nestedCatOrder...)

	catSel.Set("innerHTML", "")
	for _, cat := range nestedCatOrder {
		o := doc.Call("createElement", "option")
		o.Set("value", cat)
		o.Set("textContent", cat)
		catSel.Call("appendChild", o)
	}

	// Concentric knobs: outer ring = category (labeled), inner = model.
	catSel.Set("title", "Model category — OFF / Attractors / Scope / Polyhedra / Geometry / Audio / Custom")
	modelSel.Set("title", "Model — the specific system within the selected category")
	stack := stackKnobs(makeSelectorKnob(catSel), makeSelectorKnob(modelSel))
	short := make([]string, len(nestedCatOrder))
	for i, c := range nestedCatOrder {
		short[i] = catShortLabel(c)
	}
	addSelectorLabels(stack, short, catSel, 40)
	// Unique, concise tooltip per category label (turn/click to that category).
	setLabelTooltips(stack, map[string]string{
		"OFF":   "Power off — stop rendering and clear the display",
		"ATTR":  "Attractors — chaotic systems (Lorenz, Rössler…) plus Custom",
		"SCOPE": "Scope — Lissajous, Graphic Artist and XY oscilloscope figures",
		"POLY":  "Polyhedra — wireframe Platonic solids",
		"GEO":   "Geometry — sphere, torus, globe and other surfaces",
		"AUD":   "Audio — spectrogram and FVF wobbulator displays",
	})
	holder.Call("appendChild", stack)
	attachSelMarquee(catSel, "#8fd0ff")   // category (blue)
	attachSelMarquee(modelSel, "#7fe0a0") // model (green) — scrolls long names

	// Category change -> repopulate models, pick the first, drive #mode-select.
	// The OFF category is the power switch: it stops the model and empties the
	// model dropdown without touching #mode-select (so the last model returns
	// when the knob leaves OFF).
	catSel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if nestedSyncing {
			return nil
		}
		if catSel.Get("value").String() == nestedOffCat {
			setPowerState(false)
			populateModelSelect(nestedOffCat, "")
			modelSel.Call("dispatchEvent", js.Global().Get("Event").New("change")) // snap inner knob
			return nil
		}
		setPowerState(true) // leaving OFF (or a normal switch) powers back on
		populateModelSelect(catSel.Get("value").String(), "")
		mode.Set("value", modelSel.Get("value").String())
		mode.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
	// Model change -> drive #mode-select.
	modelSel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if nestedSyncing {
			return nil
		}
		mode.Set("value", modelSel.Get("value").String())
		mode.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))

	syncNestedFromMode()
}

// populateModelSelect fills #model-select with the models of cat. If keep names
// a model in that category it stays selected, otherwise the first is chosen.
// Does not dispatch — callers propagate to #mode-select as needed.
func populateModelSelect(cat, keep string) {
	modelSel := doc.Call("getElementById", "model-select")
	if !modelSel.Truthy() {
		return
	}
	modelSel.Set("innerHTML", "")
	for _, mo := range nestedCatModels[cat] {
		o := doc.Call("createElement", "option")
		o.Set("value", mo.value)
		o.Set("textContent", mo.label)
		modelSel.Call("appendChild", o)
	}
	if keep != "" {
		modelSel.Set("value", keep)
		if modelSel.Get("value").String() != keep {
			modelSel.Set("selectedIndex", 0)
		}
	} else {
		modelSel.Set("selectedIndex", 0)
	}
}

// syncNestedFromMode sets the category + model dropdowns (and their knob
// pointers) from the current #mode-select value, without propagating back.
func syncNestedFromMode() {
	mode := doc.Call("getElementById", "mode-select")
	catSel := doc.Call("getElementById", "cat-select")
	modelSel := doc.Call("getElementById", "model-select")
	if !mode.Truthy() || !catSel.Truthy() || !modelSel.Truthy() {
		return
	}
	cat, ok := nestedModeCat[mode.Get("value").String()]
	if !ok {
		return
	}
	nestedSyncing = true
	catSel.Set("value", cat)
	populateModelSelect(cat, mode.Get("value").String())
	// Fire change on both so the selector-knob pointers snap; the guarded
	// handlers skip re-propagation while nestedSyncing is set.
	catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	modelSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	nestedSyncing = false
}

func updateInfoOverlay() {
	overlay := doc.Call("getElementById", "info-overlay")
	if overlay.IsNull() || overlay.IsUndefined() {
		return
	}
	showInfo := doc.Call("getElementById", "show-info")
	if showInfo.IsNull() || showInfo.IsUndefined() || !showInfo.Get("checked").Bool() {
		return
	}
	if desc, ok := attractorDescriptions[selectedMode]; ok {
		overlay.Set("textContent", desc)
	} else {
		overlay.Set("textContent", selectedMode)
	}
}

// updateTrailVisibility shows/hides the Trail slider + Persist
// checkbox depending on whether the current mode renders a trail.
func updateTrailVisibility() {
	el := doc.Call("getElementById", "trail-controls")
	if !el.Truthy() {
		return
	}
	if isAttractorMode(selectedMode) {
		el.Get("style").Set("display", "")
	} else {
		el.Get("style").Set("display", "none")
	}
}

// normalizeOrientation resets the current model to the default identity
// pose and zeroes the per-axis spin rates, so it faces the camera head-on.
// Auto-rotate (if on) still applies afterward.

func onModeChange(this js.Value, args []js.Value) interface{} {
	sel := doc.Call("getElementById", "mode-select")
	if sel.Truthy() {
		selectedMode = sel.Get("value").String()
	}
	// Keep the "Edit eqn" switch in sync with whether we're in Custom mode.
	if sw := doc.Call("getElementById", "edit-eq-sw"); sw.Truthy() {
		sw.Set("checked", selectedMode == "custom")
	}
	// New mode means fresh geometry — force an upload on the next
	// uploadBuffersIndexed for static modes, and a skin-mesh rebuild.
	staticGeomDirty = true
	skinDirty = true
	resetAttractorState()
	buildParamPanel(selectedMode)
	updateInfoOverlay()
	updateSelWindow()
	syncNestedFromMode()
	updateTrailVisibility()
	// Run one frame to populate vertices, then update gradient and fit camera
	generateForMode(selectedMode)
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	} else {
		restoreAutoRotateAfterSpectrogram()
		autoFitCamera()
	}
	// FVF audio-out follows the mode: resume if re-entering FVF with Listen
	// on; stop when leaving so no stray audio plays under other models.
	if selectedMode == "fvf" {
		if fvfListen {
			startFVFAudio()
		}
	} else {
		stopFVFAudio()
	}
	// Model Out likewise: suspend in modes it can't sonify (geometry,
	// spectrogram…) instead of streaming zeros ~23×/s, resume in trail modes.
	sonifyModeSync()
	refreshGradient()
	syncPermalinkNow() // reflect the new mode in the URL immediately
	return nil
}
