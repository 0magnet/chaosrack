//go:build js && wasm

package attractor

import (
	_ "embed"
	"strconv"
	"syscall/js"
)

// ── UI helpers ───────────────────────────────────────────────────────────────

// sizeLEDField fixes a numeric input's width to the widest value it can show
// (sign + max integer digits + dot + dec) and right-aligns it, so it never
// resizes and unsigned/positive values reserve the sign column as a blank.
func sizeLEDField(el js.Value, min, max float64, dec int, signed bool) {
	chars := intDigits(max)
	if d := intDigits(min); d > chars {
		chars = d
	}
	if dec > 0 {
		chars += 1 + dec // decimal point + fraction
	}
	if signed {
		chars++ // sign column
	}
	// DSEG7 is fixed-width, so size in ch (glyph widths) plus the field's own
	// box-model overhead. The inputs are border-box with ~3px padding + ~1px
	// border per side (~8px total), so the added slack must cover that or the
	// widest value clips (e.g. a 7-digit "20480.0"); 9px clears it with a hair to
	// spare.
	st := el.Get("style")
	st.Set("width", "calc("+strconv.Itoa(chars)+"ch + 9px)")
	st.Set("textAlign", "right")
}

// wheelNudge makes scrolling over an LED readout step the paired (usually
// hidden) slider by `step`, clamped to [mn,mx]; the slider's own input handler
// then reformats the readout, so the LED stays formatted.
func wheelNudge(readout, slider js.Value, step, mn, mx float64) {
	if step == 0 {
		step = 1
	}
	readout.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		if e.Get("deltaY").Float() < 0 {
			v += step
		} else {
			v -= step
		}
		if v < mn {
			v = mn
		}
		if v > mx {
			v = mx
		}
		slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))
}

// buildParamUnit builds one parameter "unit": the control (label · knob ·
// value · step · reset) with its MOD/LVL half always beneath it (dimmed when
// Audio mod is off — never reflows). Returns the unit element.
func buildParamUnit(p paramDef) js.Value {
	dec := ledDecimals(float64(p.Step))
	signed := p.Min < 0
	intDig := ledIntDigits(float64(p.Min), float64(p.Max))
	stepStr := strconv.FormatFloat(float64(p.Step), 'g', -1, 32)
	minStr := strconv.FormatFloat(float64(p.Min), 'g', -1, 32)
	maxStr := strconv.FormatFloat(float64(p.Max), 'g', -1, 32)

	labels := paramLabels[p.ID]

	unit := doc.Call("createElement", "div")
	unit.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", labelIsSym(p.Label))) // symbols keep case; words uppercase
	lbl.Set("textContent", p.Label)

	slider := doc.Call("createElement", "input")
	slider.Set("type", "range")
	slider.Set("id", p.ID)
	slider.Set("min", minStr)
	slider.Set("max", maxStr)
	slider.Set("step", stepStr) // set before value so the thumb isn't snapped
	slider.Set("value", strconv.FormatFloat(float64(*p.Value), 'g', -1, 32))
	slider.Set("title", p.Label+" — attractor parameter (range "+minStr+" … "+maxStr+")")
	slider.Set("style", "display:none;")

	// The cell is owned by a Control that holds the value source (slider), its
	// default, and its LED format — so reset and readout formatting are the
	// Control's methods instead of inline closures / scattered handlers.
	ctl := &Control{
		module: "Params", kind: kindGeneric, cell: unit, slider: slider, def: p.Def,
		ledInt: intDig, ledDec: dec, ledSign: signed, permaKey: "p." + paramKey(p.ID),
	}
	paramControls = append(paramControls, ctl)

	numInput := doc.Call("createElement", "input")
	numInput.Set("type", "text") // LED display: keeps +/- and trailing zeros
	numInput.Set("inputmode", "decimal")
	numInput.Set("min", minStr)
	numInput.Set("max", maxStr)
	numInput.Set("step", stepStr)
	numInput.Set("className", "numin u-val")

	// A named setting reads by name. The LED shows the label rather than its
	// index, and a hidden <select> mirrors the same positions so the cell can be
	// built as a labeled rotary switch below — both still driven by the slider,
	// which remains the value.
	sel := js.Undefined()
	selSync := func(int) {}
	if len(labels) > 0 {
		// The dial's own highlighted label is the readout, so the LED would only
		// be a second, worse copy of it — seven segments cannot spell a word.
		numInput.Set("readOnly", true)
		numInput.Get("style").Set("display", "none")
		sel = doc.Call("createElement", "select")
		sel.Set("style", "display:none;")
		for i, l := range labels {
			opt := doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", l)
			sel.Call("appendChild", opt)
		}
		// The dial writes the value, the value writes the dial back (a reset or a
		// permalink moves it without anyone touching the knob). syncing guards
		// the round trip, since the label highlight listens for the same change
		// event this handler is answering.
		syncing := false
		sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if syncing {
				return nil
			}
			slider.Set("value", sel.Get("value").String())
			slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			return nil
		}))
		selSync = func(i int) {
			syncing = true
			sel.Set("value", strconv.Itoa(i))
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			syncing = false
		}
	} else {
		sizeLEDField(numInput, float64(p.Min), float64(p.Max), dec, signed)
	}
	// showValue keeps the readout in step with the value: a quantity is the
	// number, a setting is its position on the dial.
	showValue := func(v float64) {
		if len(labels) > 0 {
			selSync(clampIndex(int(v+0.5), len(labels)))
			return
		}
		numInput.Set("value", ctl.formatValue(v))
	}
	showValue(float64(*p.Value))

	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			showValue(val)
			staticGeomDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
	if len(labels) == 0 {
		numInput.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if val, err := strconv.ParseFloat(numInput.Get("value").String(), 64); err == nil {
				*p.Value = float32(val)
				slider.Set("value", strconv.FormatFloat(val, 'g', -1, 64))
				staticGeomDirty = true
				resetAttractorState()
				refreshGradient()
			}
			return nil
		}))
	}
	// Scroll over the LED readout steps the value (drives the slider, which
	// reformats the readout).
	wheelNudge(numInput, slider, float64(p.Step), float64(p.Min), float64(p.Max))

	rst := doc.Call("createElement", "button")
	rst.Set("className", "rst")
	rst.Set("title", "Reset "+p.Label)
	rst.Set("textContent", "↺")
	rst.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		ctl.resetToDefault()
		return nil
	}))

	stepInput := doc.Call("createElement", "input")
	stepInput.Set("type", "number")
	stepInput.Set("min", "0.0000001")
	stepInput.Set("step", "any")
	stepInput.Set("value", stepStr)
	stepInput.Set("title", "Step size for "+p.Label+" — how much one knob step changes the value")
	stepInput.Set("className", "numin u-step")
	stepInput.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(stepInput.Get("value").String(), 64); err == nil && val > 0 {
			newStep := strconv.FormatFloat(val, 'g', -1, 64)
			slider.Set("step", newStep)
			numInput.Set("step", newStep)
		}
		return nil
	}))

	// Standard cell header: label pinned left, numeric LED centered over the knob,
	// reset pinned right — all on one line above the knob (see .rst CSS).
	top := doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	top.Call("appendChild", lbl)
	top.Call("appendChild", numInput)
	unit.Call("appendChild", top)
	unit.Call("appendChild", slider) // hidden, drives the value
	// Integer-step parameters of geometry models are true COUNTS (latitude /
	// longitude lines, segments, subdivisions) — a fine-trim disc would dial
	// fractional lines, which can't be displayed. Continuous systems keep the
	// fine disc even at step 1 (chen's a=35 is still a real-valued knob).
	// The turtle path is traced in time rather than built, so it is Parametric —
	// but its parameters are counts in exactly the geometry sense: a modulus, a
	// multiplier, a term limit and a set of named settings have no fractional
	// part to trim.
	fine := decimalsForStep(p.Step) > 0 ||
		(modeInfo[selectedMode].Class != ClassGeometry && selectedMode != "turtle")
	if sel.Truthy() {
		ring := paramRingLabels[p.ID]
		if len(ring) != len(labels) {
			ring = labels
		}
		unit.Call("appendChild", sel)
		// TWO POSITIONS IS A SWITCH, not a dial. A rotary that can only be at one
		// end or the other is a switch wearing the wrong clothes: it costs a drag
		// to do what a click does, and the ring around it spends half its labels
		// saying what the thing is NOT. The panel is full of real switches
		// already — Persist, Points, Ring, Fill — and a two-state parameter
		// belongs with them rather than beside the dials.
		//
		// The select stays the value, so the permalink, Reset All and a patch
		// recall all still drive this through exactly the path they drove the
		// dial through.
		if len(labels) == 2 {
			unit.Call("appendChild", buildTwoWaySwitch(sel, labels))
		} else if !ringLabelsFit(ring) {
			// Too many options, or names too long to sit round a dial. This is
			// what selectorKnobReadout exists for and says so -- the Phosphor,
			// Backdrop, Skin and Desk-style knobs all take it. Twenty demo names
			// ringed round a knob would be twenty overlapping words.
			unit.Call("appendChild", selectorKnobReadout(sel))
		} else {
			stack := singleSelectorKnob(sel, ring, 46)
			tips := map[string]string{}
			for i, short := range ring {
				tips[short] = p.Label + " " + labels[i]
			}
			setLabelTooltips(stack, tips)
			unit.Call("appendChild", stack)
		}
	} else {
		unit.Call("appendChild", makeKnob(slider, numInput, fine, false, true))
	}
	unit.Call("appendChild", rst) // pinned top-right by CSS
	unit.Call("appendChild", stepInput)
	return unit
}

// buildModCard builds one card for the Modulation module: the target's name
// above its MOD/LVL control (channel + level). All modulation controls live in
// the dedicated Modulation module, never mixed into other modules.
func buildModCard(id, label string, sym bool) js.Value {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", buildModUnit(id, label))
	return card
}

func buildParamPanel(mode string) {
	// Free the previous build's listener closures, then collect this build's
	// (see funcarena_js.go) — the wipe below kills their DOM in the same
	// synchronous pass.
	releasePanelFuncs()
	panelCollect = true
	defer func() { panelCollect = false }()

	paramsDiv := doc.Call("getElementById", "params")
	paramsDiv.Set("innerHTML", "")
	paramsDiv.Set("className", "row")
	paramControls = paramControls[:0] // rebuilt below by buildParamUnit
	// The panel's am-on/am-off class shows/hides the adjacent Modulation module.
	if panel := doc.Call("getElementById", "controls-panel"); panel.Truthy() {
		cl := panel.Get("classList")
		if audioMod {
			cl.Call("add", "am-on")
			cl.Call("remove", "am-off")
		} else {
			cl.Call("add", "am-off")
			cl.Call("remove", "am-on")
		}
	}

	// Mode-scoped scope extras (run before any early return so they clean up on
	// every mode change): GA waveform switches + the CRT overlay.
	syncGAWaveSwitches(mode)
	syncPongExtras(mode)
	syncScopeTextExtras(mode)
	syncBounceExtras(mode)
	syncSprottMorphExtras(mode)
	syncSTLFileExtras(mode)
	syncMapExtras(mode)
	syncDeskExtras(mode)
	syncTermAnimExtras(mode)
	syncLayersModule(mode)
	syncSpectroModule(mode)
	syncDeskModel(mode)
	syncAnalysisModule(mode)
	clearTurtlePhysModule()
	clearSectionModule()
	updateCRTOverlay()

	// The Equation module only exists in Custom mode (buildCustomPanel makes it).
	if mode != "custom" {
		if em := doc.Call("getElementById", "eqn-module"); em.Truthy() {
			em.Get("parentNode").Call("removeChild", em)
		}
	}

	// Patchbay rebuilds with the panel so its matrix columns track the mode
	// (and so pin edits resync the MOD knobs by rebuilding everything).
	buildPatchbayModule(paramsDiv.Call("closest", ".sect"))

	// The Section module, when the Poincaré overlay is switched on. Before the
	// mode branches below, because those return early for the modes with no
	// parameter grid of their own — and the overlay works on every flow, not
	// only the ones that happen to have knobs.
	buildSectionModule(mode, paramsDiv)

	if mode == "custom" {
		buildCustomPanel(paramsDiv)
		if rebindParamWheel != nil {
			rebindParamWheel()
		}
		quantizeModuleWidths() // Equation + Parameters modules
		return
	}

	if mode == "bifurcation" {
		buildBifPanel(paramsDiv)
		quantizeModuleWidths()
		return
	}

	// A module with nothing in it reads as broken rather than as empty. Six
	// models have no tunable constants at all -- the desk, both terminals, the
	// STL viewer, the magnetosphere, the bifurcation plot -- and each of them
	// showed a Parameters module with a header and a void under it. Their
	// settings, where they have any, live in modules of their own.
	params, ok := attractorParams[mode]
	showParamsModule(len(params) > 0)
	if !ok || len(params) == 0 {
		return
	}

	// Lay the params out as units in a grid: vertical groups of 3 (3 rows),
	// flowing into as many columns as there are params. The enclosing section
	// is the equipment module (its header names it).
	grid := doc.Call("createElement", "div")
	gridCls := "punit-grid"
	if len(params) > 3 { // wraps into 2 columns → draw a divider between them
		gridCls += " two-col"
	}
	grid.Set("className", gridCls)
	for _, p := range params {
		grid.Call("appendChild", buildParamUnit(p))
	}
	paramsDiv.Call("appendChild", grid)

	buildTurtlePhysModule(mode, paramsDiv)
	applyModuleVisibility() // a rebuild puts back what the switches took away

	if mode == "takens" {
		// Into the grid for the same reason the FVF selectors are: #params
		// stacks below the height-bounded grid and gets clipped.
		appendTakensEstimate(grid)
	}

	if mode == "recurrence" {
		// Same placement, same reason. The RQA cell is also what publishes the
		// readout element, so the per-frame scan knows whether anything is
		// displaying its result — a panel rebuild replaces the element, and
		// this is where the new one is handed over.
		appendRecurrenceRQA(grid)
		// ...and the history of those same three numbers, in the cell after
		// them, which is the reading RQA is actually for. Last, because it
		// spans a whole column group of the grid and everything appended after
		// a full-height item flows into the columns past it.
		appendRecurrenceSeries(grid)
	}

	if mode == "stereo" {
		// The correlation readout, into the grid for the same reason.
		appendStereoReadout(grid)
	}

	if mode == "fvf" {
		// Into the GRID, not #params: the grid is the height-bounded
		// column-wrap container, so extra cells flow into a new column and the
		// width quantizer widens the module. Appended to #params they stacked
		// BELOW the grid and were clipped by the module's fixed height.
		appendFVFSelectors(grid) // wave + modulator selector knobs + FX/Listen
	}

	// Audio-modulation controls live in per-group MOD + EQ modules, each pair
	// inserted right after the primary module it modulates and shown only while
	// Audio mod is on (CSS: .am-off hides .modmodule/.eqmodule).
	buildModEQModules(params)
	if rebindParamWheel != nil {
		rebindParamWheel()
	}
	quantizeModuleWidths()    // param count changed → re-snap module widths
	annotateControlTooltips() // role-aware tooltips (labels/readouts/swatches)
}

// modTarget is one modulatable control (its paramMods key + display label).
type modTarget struct {
	id, label string
	sym       bool // label is a math parameter symbol (kept lowercase), not a word
}

// buildModEQModules (re)builds, for each primary module that has modulatable
// controls, an adjacent MOD module (channel/level knobs) and EQ module (graphic-
// EQ band painters), aligned row-for-row with the primary. Params come from the
// current mode; the view/camera/color targets are fixed. Only shown when Audio
// mod is on.
func buildModEQModules(params []paramDef) {
	old := doc.Call("querySelectorAll", ".modmodule, .eqmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
	var pTargets []modTarget
	for _, p := range params {
		if decimalsForStep(p.Step) > 0 {
			pTargets = append(pTargets, modTarget{p.ID, p.Label, labelIsSym(p.Label)})
		}
	}
	groups := []struct {
		hdr     string
		targets []modTarget
	}{
		{"Parameters", pTargets},
		{"Colors", []modTarget{{"view-rfreq", "rainbow", false}, {"view-trail", "trail", false}}},
		{"View", []modTarget{{"view-spinx", "spin X", false}, {"view-spiny", "spin Y", false}, {"view-spinz", "spin Z", false}}},
		// Order must match the Position control panel (X, Y, Zoom) so each MOD/EQ
		// row lines up with the control it drives ("Pan" dropped — implied by the
		// Position group).
		{"Position", []modTarget{{"view-panx", "X", false}, {"view-pany", "Y", false}, {"view-zoom", "zoom", false}}},
	}
	findSect := func(hdr string) js.Value {
		s := doc.Call("querySelectorAll", ".modules > .sect")
		for i := 0; i < s.Get("length").Int(); i++ {
			m := s.Index(i)
			if h := m.Call("querySelector", ".sect-hdr"); h.Truthy() && h.Get("textContent").String() == hdr {
				return m
			}
		}
		return js.Undefined()
	}
	makeMod := func(cls, title, tip string, cards []js.Value) js.Value {
		mod := doc.Call("createElement", "div")
		mod.Set("className", "sect "+cls)
		h := doc.Call("createElement", "div")
		h.Set("className", "sect-hdr")
		h.Set("textContent", title)
		h.Set("title", tip)
		mod.Call("appendChild", h)
		g := doc.Call("createElement", "div")
		gc := "punit-grid"
		if len(cards) > 3 {
			gc += " two-col"
		}
		g.Set("className", gc)
		for _, c := range cards {
			g.Call("appendChild", c)
		}
		mod.Call("appendChild", g)
		return mod
	}
	for _, grp := range groups {
		if len(grp.targets) == 0 {
			continue
		}
		primary := findSect(grp.hdr)
		if !primary.Truthy() {
			continue
		}
		var modCards, eqCards []js.Value
		for _, t := range grp.targets {
			modCards = append(modCards, buildModCard(t.id, t.label, t.sym))
			eqCards = append(eqCards, buildEQCard(t.id, t.label, t.sym))
		}
		// Short, single-line headers so the module content starts at the same Y
		// as its primary (a wrapped 2-line header would push the knobs down).
		modMod := makeMod("modmodule", "Mod",
			"Modulation routing for the "+grp.hdr+" module — a channel + depth card per control", modCards)
		eqMod := makeMod("eqmodule", "EQ",
			"Graphic-EQ band weights for the "+grp.hdr+" module's modulation — paint which frequency bands drive each control", eqCards)
		parent := primary.Get("parentNode")
		parent.Call("insertBefore", modMod, primary.Get("nextSibling"))
		parent.Call("insertBefore", eqMod, modMod.Get("nextSibling"))
	}
}

// buildTurtlePhysModule gives the weight controls a module of their own, which
// the Physics switch shows and hides. They are not parameters of the figure —
// the figure is the same figure whatever it weighs — and a module that appears
// when the switch is thrown says that better than four more knobs that do
// nothing until it is.
func buildTurtlePhysModule(mode string, paramsDiv js.Value) {
	clearTurtlePhysModule()
	if mode != "turtle" {
		return
	}
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect physmodule")
	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", "Physics")
	h.Set("title", "The figure as a rigid body in the plane of the screen, inside a room whose walls are the edges of the picture. GRAV pulls either way up; FRIC is how much the surfaces bite; BOUNCE is how much of the speed a wall gives back; SPIN is how readily it turns.")
	mod.Call("appendChild", h)
	g := doc.Call("createElement", "div")
	g.Set("className", "punit-grid two-col")
	for _, p := range turtlePhysParams {
		g.Call("appendChild", buildParamUnit(p))
	}
	mod.Call("appendChild", g)
	if primary := paramsDiv.Call("closest", ".sect"); primary.Truthy() {
		primary.Get("parentNode").Call("insertBefore", mod, primary.Get("nextSibling"))
	}
}

// buildSectionModule gives the Poincaré overlay's plane controls a module of
// their own, alongside the source system's Parameters module rather than
// inside it. The plane is not a parameter of the Lorenz system — turning it
// changes what is being LOOKED AT, not what is running — and mixing the two
// grids would say otherwise.
//
// It is the turtle Physics module's shape, for the same reason: controls that
// appear with a switch and go away with it. The Sect switch rebuilds the panel
// so this runs, exactly as the Patchbay switch does.
//
// Not in the Poincaré MODEL's own mode, where the same paramDefs are already in
// attractorParams and the generic grid has built them — two grids driving the
// same variables would be two DOM elements with the same id, and the second
// one's dial would silently drive the first one's slider.
func buildSectionModule(mode string, paramsDiv js.Value) {
	clearSectionModule()
	if !sectOn || mode == "poincare" {
		return
	}
	if _, isFlow := flowFor4(mode); !isFlow {
		// Nothing to section. The switch stays on — it is a preference about
		// flows, and hopping through a dodecahedron on the way to another
		// attractor should not turn it off.
		return
	}
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect sectmodule")
	h := doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", "Section")
	h.Set("title", "Where the Poincaré section's plane sits, and which way through it counts. "+
		"AXIS and POS place it — POS as a fraction of the attractor's own reach along that axis, "+
		"so 0 is through the middle whatever the system's size. DIR one way is the default: a "+
		"bounded flow that goes up through a plane must come back down through it, so counting "+
		"both superimposes two different sections. The crossings draw in gold where they "+
		"physically are; Analysis → Poincaré Section is the same section as a picture of its "+
		"own, with the return map.")
	mod.Call("appendChild", h)
	g := doc.Call("createElement", "div")
	g.Set("className", "punit-grid")
	for _, p := range sectPlaneParams {
		g.Call("appendChild", buildParamUnit(p))
	}
	mod.Call("appendChild", g)
	if primary := paramsDiv.Call("closest", ".sect"); primary.Truthy() {
		primary.Get("parentNode").Call("insertBefore", mod, primary.Get("nextSibling"))
	}
}

// clearSectionModule takes it away again, on every panel build, before the
// early returns — so the module cannot outlive the switch or the mode.
func clearSectionModule() {
	old := doc.Call("querySelectorAll", ".sectmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
}

// clearTurtlePhysModule takes the module away. It runs on every panel build,
// before the early returns for the modes that have no parameter grid at all,
// so the module cannot outlive the mode it belongs to.
func clearTurtlePhysModule() {
	old := doc.Call("querySelectorAll", ".physmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
}

// buildEQCard is one graphic-EQ band-painter card for the EQ module.
func buildEQCard(id, label string, sym bool) js.Value {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", makeEQStrip(id))
	return card
}

func hexToRGB(hex string) (float32, float32, float32) {
	if len(hex) < 7 {
		return 1, 1, 1
	}
	r, _ := strconv.ParseInt(hex[1:3], 16, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	g, _ := strconv.ParseInt(hex[3:5], 16, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	b, _ := strconv.ParseInt(hex[5:7], 16, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return float32(r) / 255.0, float32(g) / 255.0, float32(b) / 255.0
}

// buildTwoWaySwitch renders a two-option setting as a switch with the current
// option named beside it.
//
// The name matters: a bare switch says on/off, and these settings are not
// on/off — "logarithmic" and "linear" are two things, neither of which is the
// absence of the other. Writing the live option next to the switch keeps that
// readable without a label ring, and it doubles as the readout the LED would
// have been (buildParamUnit hides the LED for named settings, because seven
// segments cannot spell a word).
//
// The select remains the value. Everything that drives one of these — the
// permalink, Reset All, a patch recall — moves the select and this follows.
func buildTwoWaySwitch(sel js.Value, labels []string) js.Value {
	wrap := doc.Call("createElement", "label")
	wrap.Set("className", "grp twoway")
	wrap.Get("style").Set("cursor", "pointer")

	box := doc.Call("createElement", "input")
	box.Set("type", "checkbox")
	box.Set("className", "sw")

	name := doc.Call("createElement", "span")
	name.Set("className", "twoway-name")

	show := func() {
		i := sel.Get("selectedIndex").Int()
		if i < 0 {
			i = 0
		}
		box.Set("checked", i == 1)
		name.Set("textContent", labels[clampIndex(i, len(labels))])
	}
	box.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		idx := 0
		if box.Get("checked").Bool() {
			idx = 1
		}
		sel.Set("selectedIndex", idx)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
	// The select can move without the switch being touched, and then the switch
	// has to catch up or it is lying about the state it controls.
	sel.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		show()
		return nil
	}))
	show()

	wrap.Call("appendChild", box)
	wrap.Call("appendChild", name)
	wrap.Call("appendChild", sel)
	return wrap
}

// showParamsModule hides the Parameters module for a model that has no
// parameters, instead of leaving a titled empty box on the rack.
func showParamsModule(on bool) {
	sect := doc.Call("getElementById", "params-module")
	if !sect.Truthy() {
		return
	}
	if on {
		sect.Get("style").Set("display", "")
		return
	}
	sect.Get("style").Set("display", "none")
}
