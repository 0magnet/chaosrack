//go:build js && wasm

package attractor

import (
	_ "embed"
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/racklayout"
)

// ── UI helpers ───────────────────────────────────────────────────────────────

// layoutDebts is the layout work deferred to the next frame.
type layoutDebts struct {
	// readouts puts the meters' and controls' text on their LEDs, skipping
	// writes that would not change them. Rebuilding the panel forgets it.
	readouts led.Readouts

	// Deferring the two things a control handler does that cost the whole rack.
	//
	// commitBuiltControls fires each control.s own event once, so that every
	// control applies its value the way it would if you had touched it. Sixty-
	// eight controls, and several of their handlers rebuild the parameter panel
	// or re-quantize the rack — a rebuild is a second of work now, and a
	// quantize stretches all seventy-eight modules to 3000px and measures them
	// back, which is a full layout of ten thousand nodes. Measured, the sweep
	// ran twenty-three quantizes and three panel rebuilds and took six seconds
	// of a thirteen-second boot.
	//
	// commitBuiltControls fires each control's own event once so that every
	// control applies its value the way it would if you had touched it. Several
	// of those handlers rebuild the parameter panel, and a rebuild is now a
	// second of work: the Behind and On selectors between them spent four and a
	// half seconds of a thirteen-second boot rebuilding a panel that nothing had
	// changed, twice. The sweep does not need the panel rebuilt between two
	// controls — it needs it right once at the end — so the requests are
	// collected and paid for once.
	// None of that is wrong between two controls — it is only wrong to do it
	// sixty-eight times when the rack is read once at the end. So the requests
	// are collected and paid for once.
	deferred, paramPanel, quantize, skirts bool
}

var owed layoutDebts

// sizeLEDField fixes a numeric input's width to the widest value it can show
// (sign + max integer digits + dot + dec) and right-aligns it, so it never
// resizes and unsigned/positive values reserve the sign column as a blank.
func sizeLEDField(el js.Value, min, max float64, dec int, signed bool) {
	chars := led.IntDigits(min, max)
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
	readout.Call("addEventListener", "wheel", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
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
//
// The MODE is passed because a cell is no longer necessarily the running
// model's: every category row carries the parameters of every model in it,
// so a cell has to be built the way ITS model wants rather than the way
// whatever happens to be playing does. See the fine-trim decision below,
// which reads the mode's class.
func buildParamUnit(mode string, p paramDef) js.Value {
	dec := led.Decimals(float64(p.Step), fineRatio)
	signed := p.Min < 0
	intDig := led.IntDigits(float64(p.Min), float64(p.Max))
	stepStr := strconv.FormatFloat(float64(p.Step), 'g', -1, 32)
	minStr := strconv.FormatFloat(float64(p.Min), 'g', -1, 32)
	maxStr := strconv.FormatFloat(float64(p.Max), 'g', -1, 32)

	labels := paramLabels[p.ID]

	unit := dom.Doc.Call("createElement", "div")
	unit.Set("className", "punit")

	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", labelIsSym(p.Label))) // symbols keep case; words uppercase
	lbl.Set("textContent", p.Label)

	slider := dom.Doc.Call("createElement", "input")
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

	numInput := dom.Doc.Call("createElement", "input")
	numInput.Set("type", "text") // LED display: keeps +/- and trailing zeros
	numInput.Set("inputmode", "decimal")
	// No min/max/step here. They are only meaningful on a numeric input, and
	// this is type=text so it can hold the LED's sign and trailing zeros —
	// the validator counts 756 of them across the panel, and not one is read:
	// the wheel over a readout goes through wheelNudge, which takes the range
	// as Go arguments, and bindWheelEl binds only range and number inputs.
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
		sel = dom.Doc.Call("createElement", "select")
		sel.Set("style", "display:none;")
		for i, l := range labels {
			opt := dom.Doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", l)
			sel.Call("appendChild", opt)
		}
		// The dial writes the value, the value writes the dial back (a reset or a
		// permalink moves it without anyone touching the knob). syncing guards
		// the round trip, since the label highlight listens for the same change
		// event this handler is answering.
		syncing := false
		sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
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

	slider.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			showValue(val)
			gpu.staticDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
	if len(labels) == 0 {
		numInput.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if val, err := strconv.ParseFloat(numInput.Get("value").String(), 64); err == nil {
				*p.Value = float32(val)
				slider.Set("value", strconv.FormatFloat(val, 'g', -1, 64))
				gpu.staticDirty = true
				resetAttractorState()
				refreshGradient()
			}
			return nil
		}))
	}
	// Scroll over the LED readout steps the value (drives the slider, which
	// reformats the readout).
	wheelNudge(numInput, slider, float64(p.Step), float64(p.Min), float64(p.Max))

	rst := dom.Doc.Call("createElement", "button")
	rst.Set("className", "rst")
	rst.Set("title", "Reset "+p.Label)
	rst.Set("textContent", "↺")
	rst.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ctl.resetToDefault()
		return nil
	}))

	stepInput := dom.Doc.Call("createElement", "input")
	stepInput.Set("type", "number")
	stepInput.Set("min", "0.0000001")
	stepInput.Set("step", "any")
	stepInput.Set("value", stepStr)
	stepInput.Set("title", "Step size for "+p.Label+" — how much one knob step changes the value")
	stepInput.Set("className", "numin u-step")
	stepInput.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(stepInput.Get("value").String(), 64); err == nil && val > 0 {
			newStep := strconv.FormatFloat(val, 'g', -1, 64)
			slider.Set("step", newStep)
		}
		return nil
	}))

	// Standard cell header: label pinned left, numeric LED centered over the knob,
	// reset pinned right — all on one line above the knob (see .rst CSS).
	top := dom.Doc.Call("createElement", "span")
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
	fine := led.StepDecimals(p.Step) > 0 ||
		(modeInfo[mode].Class != ClassGeometry && mode != "turtle")
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
			unit.Call("appendChild", buildTwoWaySwitch(sel, labels, p.Label))
		} else if !racklayout.RingLabelsFit(ring) {
			// Too many options, or names too long to sit round a dial. This is
			// what selectorKnobReadout exists for and says so -- the Phosphor,
			// Backdrop, Skin and Desk-style knobs all take it. Twenty demo names
			// ringed round a knob would be twenty overlapping words.
			unit.Call("appendChild", selectorKnobReadout(sel))
		} else {
			stack := singleSelectorKnob(sel, ring)
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
	card := dom.Doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", buildModUnit(id, label))
	return card
}

// withDeferredLayout runs f with panel rebuilds and rack quantizes
// collected, then does each once if anything asked for it.
//
// The rebuild is paid for with the collecting still ON, and that is the
// point rather than a detail: building the parameter panel asks for four
// quantizes of its own — one directly, one from applyModuleVisibility, one
// from the desk extras, one from the terminal-animation extras — so a flush
// that switched collecting off first bought one pass and then paid for four.
// Measured on a mode change before this: five full passes over the rack,
// 2435ms of a 2903ms switch, all five computing the same answer.
//
// A nested call is the outer one's business. A handler that defers is often
// reached from a sweep that already has, and an inner scope that reset the
// flags on its way out would hand the rest of the outer scope's work back to
// the unbatched path.
func (l *layoutDebts) withDeferredLayout(mode string, f func()) {
	if l.deferred {
		f()
		return
	}
	l.deferred, l.paramPanel, l.quantize, l.skirts = true, false, false, false
	f()
	// At most twice. A rebuild that asks for another rebuild is a loop
	// rather than a request; the second pass is for the one honest case,
	// a builder that only learns it needs the panel again from something
	// the first build put on it.
	for n := 0; l.paramPanel && n < 2; n++ {
		l.paramPanel = false
		buildParamPanelNow(mode)
	}
	l.deferred = false
	if l.quantize {
		// Which ends in a skirt pass of its own, so the owed one is paid.
		quantizeModuleWidths()
	} else if l.skirts {
		layoutSkirts()
	}
}

func buildParamPanel(mode string) {
	if owed.deferred {
		owed.paramPanel = true
		return
	}
	buildParamPanelNow(mode)
}

// buildParamPanelNow is the rebuild itself, with no collecting check. The one
// caller that runs it while layout is deferred is the flush above, which is
// deferring precisely so that this function's own quantizes are collected
// instead of paid for one at a time.
func buildParamPanelNow(mode string) {
	// The sweep is over THIS model's parameters; a mode change makes the
	// dial's list wrong before anything else in the panel is rebuilt.
	syncSweepDialMode(mode)
	// Free the previous build's listener closures, then collect this build's
	// — the wipe below kills their DOM in the same synchronous pass.
	defer dom.StartPanelBuild()()

	paramsDiv := dom.Doc.Call("getElementById", "params")
	paramsDiv.Set("innerHTML", "")
	paramsDiv.Set("className", "row")
	paramControls = paramControls[:0] // rebuilt below by buildParamUnit
	// The panel's am-on/am-off class shows/hides the adjacent Modulation module.
	if panel := dom.Doc.Call("getElementById", "controls-panel"); panel.Truthy() {
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
	ga.syncGAWaveSwitches(mode)
	pong.syncPongExtras(mode)
	ftext.syncScopeTextExtras(mode)
	ball.syncBounceExtras(mode)
	morph.syncSprottMorphExtras(mode)
	syncSTLFileExtras(mode)
	syncMapExtras(mode)
	syncDeskExtras(mode)
	syncTermAnimExtras(mode)
	syncLayersModule(mode)
	syncSpectroModule(mode)
	syncDeskModel(mode)
	lyap.syncAnalysisModule(mode)
	clearTurtlePhysModule()
	clearSectionModule()
	phos.updateCRTOverlay()

	// The Equation module only exists in Custom mode (buildCustomPanel makes it).
	if mode != "custom" {
		if em := dom.Doc.Call("getElementById", "eqn-module"); em.Truthy() {
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
		// Shown explicitly: these two build an editor into the module rather
		// than knobs, and a mode before them may have left it hidden.
		showParamsModule(true)
		custom.buildCustomPanel(paramsDiv)
		if rebindParamWheel != nil {
			rebindParamWheel()
		}
		quantizeModuleWidths() // Equation + Parameters modules
		return
	}

	if mode == "bifurcation" {
		showParamsModule(true)
		bif.buildBifPanel(paramsDiv)
		quantizeModuleWidths()
		return
	}

	// The KNOBS are not built here any more.
	//
	// Every model's parameters are on its category's row, all of them, all the
	// time — see rackcategory_js.go. A model's constants are a property of the
	// model and not of what is playing, so they are built once and stay built.
	// This module used to throw the running model's away and build the next
	// one's on every mode change, which is why there was only ever a single
	// panel of them and why it had to live wherever that panel was rather than
	// on the row of the model it belongs to.
	//
	// A parameter also has exactly ONE knob in the rack. Building them here as
	// well would give every id in attractorParams[mode] a second element with
	// the same id, and the MIDI map, the permalink and Reset All each address
	// a parameter by that id.
	//
	// What is left in this module is what genuinely belongs to the RUNNING
	// model and cannot be built ahead of time: the readouts that measure it
	// (the Lyapunov exponent, RQA, correlation, the fitted delay), the
	// selectors a mode adds to its own panel, and the Custom and Bifurcation
	// panels, which are editors rather than knob grids. The grid below is
	// their container; if nothing goes into it the module is hidden, because a
	// module with a header and a void under it reads as broken rather than as
	// empty.
	params := attractorParams[mode]
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid")
	paramsDiv.Call("appendChild", grid)
	// Decided at the end, once the extras below have had their chance at it.
	defer func() { showParamsModule(grid.Get("childElementCount").Int() > 0) }()

	buildTurtlePhysModule(mode, paramsDiv)
	applyModuleVisibility() // a rebuild puts back what the switches took away

	if mode == "takens" {
		// Into the grid for the same reason the FVF selectors are: #params
		// stacks below the height-bounded grid and gets clipped.
		emb.appendTakensEstimate(grid)
	}

	if mode == "recurrence" {
		// Same placement, same reason. The RQA cell is also what publishes the
		// readout element, so the per-frame scan knows whether anything is
		// displaying its result — a panel rebuild replaces the element, and
		// this is where the new one is handed over.
		rp.appendRecurrenceRQA(grid)
		// ...and the history of those same three numbers, in the cell after
		// them, which is the reading RQA is actually for. Last, because it
		// spans a whole column group of the grid and everything appended after
		// a full-height item flows into the columns past it.
		rqa.appendRecurrenceSeries(grid)
	}

	if mode == "stereo" {
		// The correlation readout, into the grid for the same reason.
		stereo.appendReadout(grid)
	}

	if mode == "xfer" {
		// The fitted bulk delay, into the grid for appendStereoReadout's reason.
		xf.appendTransferReadout(grid)
	}

	if mode == "waterfall" {
		// The reverberation time, which the surface is far too shallow to show.
		wfall.appendReadout(grid)
	}

	if mode == "xy" {
		// The goniometer's own correlation meter — the number every hardware
		// one carries beside the tube, and the one the figure cannot give you,
		// because a thin ellipse and a line are the same picture at a glance.
		xy.appendXYReadout(grid)
	}

	if _, isFlow := lyapLiveSystem(mode); isFlow {
		// The live Lyapunov exponent, same placement and same reason. Only on
		// the continuous flows, and lyapLiveSystem is what draws that line —
		// from the mode's declared class rather than from whether dynamics.FlowFor4
		// happens to answer, for the reason spelled out there. A map's
		// exponent is per iterate and a polyhedron has none; both belong to
		// the Analysis module, which can say so in words, rather than to a
		// cell in this grid that could only print a number or a dash.
		lyapLive.appendLyapunovReadout(grid)
	}

	if mode == "fvf" {
		// Into the GRID, not #params: the grid is the height-bounded
		// column-wrap container, so extra cells flow into a new column and the
		// width quantizer widens the module. Appended to #params they stacked
		// BELOW the grid and were clipped by the module's fixed height.
		fvf.appendFVFSelectors(grid) // wave + modulator selector knobs + FX/Listen
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
	syncSweptMarks()          // a rebuilt row has lost its swept marking
	syncLinkMarks()           // and its per-control link badges
	layoutSkirts()            // skirts are measured, so they are sized once the rows exist
}

// modTarget is one modulatable control (its pmod.params key + display label).
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
	old := dom.Doc.Call("querySelectorAll", ".modmodule, .eqmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
	// One card per parameter, integer ones included — the third copy of the
	// rule applyAudioModulation and matrixDests keep, and the copy the user
	// actually touches. Offering the pin in the Patchbay while withholding the
	// MOD/LVL knob would make routing a line count a thing only reachable from
	// one of the two surfaces that exist for it.
	//
	// Row-for-row alignment is the other half. These cards are laid into a grid
	// beside the primary module's, which has a cell for EVERY parameter; while
	// the integer ones were skipped here, a mode with a count in the middle of
	// its parameter list (turtle, the geometry models) had every card below it
	// sitting one row off the control it belongs to.
	pTargets := make([]modTarget, 0, len(params))
	for _, p := range params {
		pTargets = append(pTargets, modTarget{p.ID, p.Label, labelIsSym(p.Label)})
	}
	groups := []struct {
		hdr     string
		targets []modTarget
	}{
		{"Parameters", pTargets},
		// In the Colors module's own order (period, shift, trail) so each card
		// sits beside the knob it drives — NOT in viewModTargets order, which
		// is fixed by the MIDI CC map and has the shift appended at the end.
		{"Colors", []modTarget{{"view-rfreq", "period", false}, {"view-pshift", "shift", false}, {"view-trail", "trail", false}}},
		{"View", []modTarget{{"view-spinx", "spin X", false}, {"view-spiny", "spin Y", false}, {"view-spinz", "spin Z", false}}},
		// Order must match the Position control panel (X, Y, Zoom) so each MOD/EQ
		// row lines up with the control it drives ("Pan" dropped — implied by the
		// Position group).
		{"Position", []modTarget{{"view-panx", "X", false}, {"view-pany", "Y", false}, {"view-zoom", "zoom", false}}},
	}
	// A DESCENDANT selector, not a child one, and the difference silently cost
	// the whole feature.
	//
	// Modules used to be direct children of .modules. Bay packing wraps them —
	// .modules > .runit > .runit-open > .sect — so `.modules > .sect` matched
	// NOTHING, every group below took its `continue`, and audio-mod built no
	// MOD or EQ modules at all. Nothing failed loudly: the checkbox still
	// flipped the panel's am-on class and the CSS that hides .modmodule still
	// worked, so there was simply never anything there to hide.
	//
	// The insert below goes through the primary's own parentNode, so the pair
	// still lands beside the module it modulates, in whatever bay that is.
	findSect := func(hdr string) js.Value {
		s := dom.Doc.Call("querySelectorAll", moduleSelector)
		for i := 0; i < s.Get("length").Int(); i++ {
			m := s.Index(i)
			if h := m.Call("querySelector", ".sect-hdr"); h.Truthy() && h.Get("textContent").String() == hdr {
				return m
			}
		}
		return js.Undefined()
	}
	makeMod := func(cls, title, tip string, cards []js.Value) js.Value {
		mod := dom.Doc.Call("createElement", "div")
		mod.Set("className", "sect "+cls)
		h := dom.Doc.Call("createElement", "div")
		h.Set("className", "sect-hdr")
		h.Set("textContent", title)
		h.Set("title", tip)
		mod.Call("appendChild", h)
		g := dom.Doc.Call("createElement", "div")
		g.Set("className", "punit-grid")
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
	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect physmodule")
	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", "Physics")
	h.Set("title", "The figure as a rigid body in the plane of the screen, inside a room whose walls are the edges of the picture. GRAV pulls either way up; FRIC is how much the surfaces bite; BOUNCE is how much of the speed a wall gives back; SPIN is how readily it turns.")
	mod.Call("appendChild", h)
	g := dom.Doc.Call("createElement", "div")
	g.Set("className", "punit-grid")
	for _, p := range turtlePhysParams {
		g.Call("appendChild", buildParamUnit(run.selectedMode, p))
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
	if !sect.on || mode == "poincare" {
		return
	}
	if _, isFlow := dynamics.FlowFor4(mode); !isFlow {
		// Nothing to section. The switch stays on — it is a preference about
		// flows, and hopping through a dodecahedron on the way to another
		// attractor should not turn it off.
		return
	}
	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect sectmodule")
	h := dom.Doc.Call("createElement", "div")
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
	g := dom.Doc.Call("createElement", "div")
	g.Set("className", "punit-grid")
	for _, p := range sectPlaneParams {
		g.Call("appendChild", buildParamUnit(run.selectedMode, p))
	}
	mod.Call("appendChild", g)
	if primary := paramsDiv.Call("closest", ".sect"); primary.Truthy() {
		primary.Get("parentNode").Call("insertBefore", mod, primary.Get("nextSibling"))
	}
}

// clearSectionModule takes it away again, on every panel build, before the
// early returns — so the module cannot outlive the switch or the mode.
func clearSectionModule() {
	old := dom.Doc.Call("querySelectorAll", ".sectmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
}

// clearTurtlePhysModule takes the module away. It runs on every panel build,
// before the early returns for the modes that have no parameter grid at all,
// so the module cannot outlive the mode it belongs to.
func clearTurtlePhysModule() {
	old := dom.Doc.Call("querySelectorAll", ".physmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
}

// buildEQCard is one graphic-EQ band-painter card for the EQ module.
func buildEQCard(id, label string, sym bool) js.Value {
	card := dom.Doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", makeEQStrip(id))
	return card
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
func buildTwoWaySwitch(sel js.Value, labels []string, label string) js.Value {
	// The switch and the select it drives are SIBLINGS, inside a wrapper that
	// is display:contents so the panel's grid still lays out the switch
	// itself rather than a box around it.
	//
	// The select used to sit inside the label. A label may contain at most
	// one labelable descendant, and a checkbox plus a select is two: which
	// control the label names is then undefined, and so is what a click on
	// the name does. It renders correctly, which is why it stood — this is
	// the kind of fault only a validator finds.
	outer := dom.Doc.Call("createElement", "span")
	outer.Set("className", "twoway-wrap")

	wrap := dom.Doc.Call("createElement", "label")
	wrap.Set("className", "grp twoway")
	wrap.Get("style").Set("cursor", "pointer")
	// The switch had no tooltip anywhere on it — not the box, not the name, not
	// this wrapper — so it was the one parameter control in the panel that
	// explained nothing when you hovered it. The dial it replaced had a label
	// ring whose every position said what that position was.
	wrap.Set("title", label+" — a two-position switch: "+labels[0]+" or "+labels[1])

	box := dom.Doc.Call("createElement", "input")
	box.Set("type", "checkbox")
	box.Set("className", "sw")

	name := dom.Doc.Call("createElement", "span")
	name.Set("className", "twoway-name")

	show := func() {
		i := sel.Get("selectedIndex").Int()
		if i < 0 {
			i = 0
		}
		box.Set("checked", i == 1)
		name.Set("textContent", labels[clampIndex(i, len(labels))])
		// What it is now, and what the next click gives — which is the question
		// a two-position control actually raises, and one the wrapper's tooltip
		// cannot answer because it does not change.
		name.Set("title", labels[clampIndex(i, len(labels))]+" — click or scroll for "+labels[clampIndex(1-i, len(labels))])
	}
	box.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
		idx := 0
		if box.Get("checked").Bool() {
			idx = 1
		}
		sel.Set("selectedIndex", idx)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
	// The wheel steps it, because every other control in the rack answers the
	// wheel and these sit in the same grid as knobs that do. A two-position
	// parameter is still a parameter; that it is drawn as a switch rather than a
	// dial is a decision about which gesture is CHEAPEST (a click, not a drag),
	// not a decision to answer fewer gestures than its neighbors.
	//
	// Up towards the earlier option, matching makeSelectorKnob, so the two agree
	// about which way "up" is on a detented control.
	wrap.Call("addEventListener", "wheel", dom.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		idx := 1
		if e.Get("deltaY").Float() < 0 {
			idx = 0
		}
		if sel.Get("selectedIndex").Int() == idx {
			return nil
		}
		sel.Set("selectedIndex", idx)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
	// The select can move without the switch being touched, and then the switch
	// has to catch up or it is lying about the state it controls.
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
		show()
		return nil
	}))
	show()

	wrap.Call("appendChild", box)
	wrap.Call("appendChild", name)
	outer.Call("appendChild", wrap)
	outer.Call("appendChild", sel)
	return outer
}

// showParamsModule hides the Parameters module for a model that has no
// parameters, instead of leaving a titled empty box on the rack.
func showParamsModule(on bool) {
	sect := dom.Doc.Call("getElementById", "params-module")
	if !sect.Truthy() {
		return
	}
	if on {
		sect.Get("style").Set("display", "")
		return
	}
	sect.Get("style").Set("display", "none")
}

// newPunitCard builds a parameter cell with the anatomy buildParamUnit gives
// every cell it makes: a .punit whose .punit-top carries the label, and beside
// it whatever readout the cell wants.
//
// Shared because it was not, and the drift showed. Ten places built this card by
// hand — the readouts (corr, dly, meas, rqa, rt60, λ), the FVF selector cards —
// and every one of them appended the label straight into the .punit with no
// .punit-top around it. That matches neither of the two rules in panel.css that
// pin a label to its cell's top-left corner, so they fell through to
// .punit{align-items:center} and CENTERED their labels over the knob while
// every cell beside them in the same grid row pinned theirs to the left edge.
//
// The wrapper is what puts the readout in the label's row as well, centered over
// the knob's axis, which is the other half of the standard cell: a bare LED
// under a bare label stacks two centered things where the panel everywhere else
// has a left label with the value beside it.
//
// Returns the card and its top row, so the caller appends the readout to the
// row and the control to the card.
func newPunitCard(label string) (card, top js.Value) {
	card = dom.Doc.Call("createElement", "div")
	card.Set("className", "punit")
	top = dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", labelIsSym(label)))
	lbl.Set("textContent", label)
	top.Call("appendChild", lbl)
	card.Call("appendChild", top)
	return card, top
}
