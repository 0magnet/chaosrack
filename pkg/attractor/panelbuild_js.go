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
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64)
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
	numInput.Set("value", ctl.formatValue(float64(*p.Value)))
	numInput.Set("className", "numin u-val")
	sizeLEDField(numInput, float64(p.Min), float64(p.Max), dec, signed)

	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			numInput.Set("value", ctl.formatValue(val))
			staticGeomDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
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
	unit.Call("appendChild", makeKnob(slider, numInput, true, false, true))
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
	updateCRTOverlay()

	// The Equation module only exists in Custom mode (buildCustomPanel makes it).
	if mode != "custom" {
		if em := doc.Call("getElementById", "eqn-module"); em.Truthy() {
			em.Get("parentNode").Call("removeChild", em)
		}
	}

	if mode == "custom" {
		buildCustomPanel(paramsDiv)
		if rebindParamWheel != nil {
			rebindParamWheel()
		}
		quantizeModuleWidths() // Equation + Parameters modules
		return
	}

	params, ok := attractorParams[mode]
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
		{"params", pTargets},
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
	makeMod := func(cls, title string, cards []js.Value) js.Value {
		mod := doc.Call("createElement", "div")
		mod.Set("className", "sect "+cls)
		h := doc.Call("createElement", "div")
		h.Set("className", "sect-hdr")
		h.Set("textContent", title)
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
		modMod := makeMod("modmodule", "mod", modCards)
		eqMod := makeMod("eqmodule", "eq", eqCards)
		parent := primary.Get("parentNode")
		parent.Call("insertBefore", modMod, primary.Get("nextSibling"))
		parent.Call("insertBefore", eqMod, modMod.Get("nextSibling"))
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
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return float32(r) / 255.0, float32(g) / 255.0, float32(b) / 255.0
}
