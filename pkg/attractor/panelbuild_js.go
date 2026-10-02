//go:build js && wasm

package attractor

import (
	"math"
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

	// modelChange marks the scope as a model change (onModeChange), the one
	// kind of request that may find the rack already right: see rackUnmoved.
	modelChange bool
}

var owed layoutDebts

// parseReadout reads a typed-in readout's value, its blank sign slot included
// (led.Blank). By this name where a variable called led is in scope.
var parseReadout = led.Parse

// sizeLEDField sizes a numeric readout for the widest value it can show
// (sign + max integer digits + dot + dec), full or half, and right-aligns it,
// so unsigned and positive values reserve the sign column as a blank.
func sizeLEDField(el js.Value, lo, hi float64, dec int, signed bool) {
	chars := led.IntDigits(lo, hi)
	if dec > 0 {
		chars += 1 + dec // decimal point + fraction
	}
	if signed {
		chars++ // sign column
	}
	// Not sized to its digits: a readout is a full display or a half one
	// (panel.css --disp-full, --disp-half), so two knobs side by side carry
	// the same part whatever their ranges. A full one holds seven digits, a
	// half one three.
	el.Get("style").Set("textAlign", "right")
	el.Call("setAttribute", "data-chars", strconv.Itoa(chars))
	el.Get("classList").Call("toggle", "disp-half", chars <= 3)
}

// wheelNudge makes scrolling over an LED readout step the paired (usually
// hidden) slider by `step`, clamped to [mn,mx]; the slider's own input handler
// then reformats the readout, so the LED stays formatted.
func wheelNudge(readout, slider js.Value, step, mn, mx float64) {
	if step == 0 {
		step = 1
	}
	readout.Call("addEventListener", "wheel", dom.FuncOf(func(this js.Value, args []js.Value) any {
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
		dom.Fire(slider, "input")
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
	slider.Set("title", docf("param-slider", "label", p.Label, "min", minStr, "max", maxStr))
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
		sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, args []js.Value) any {
			if syncing {
				return nil
			}
			slider.Set("value", sel.Get("value").String())
			dom.Fire(slider, "input")
			return nil
		}))
		selSync = func(i int) {
			syncing = true
			sel.Set("value", strconv.Itoa(i))
			dom.Fire(sel, "change")
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

	slider.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) any {
		if val, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			showValue(val)
			if quietParams[p.ID] {
				return nil // a pot the model turns itself: nothing to rebuild
			}
			gpu.staticDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
	if len(labels) == 0 {
		numInput.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) any {
			if val, err := led.Parse(numInput.Get("value").String()); err == nil {
				*p.Value = float32(val)
				slider.Set("value", strconv.FormatFloat(val, 'g', -1, 64))
				if quietParams[p.ID] {
					return nil // a pot the model turns itself, or a sound: nothing to rebuild
				}
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
	rst.Set("title", docf("reset", "label", p.Label))
	rst.Set("textContent", "↺")
	rst.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, args []js.Value) any {
		ctl.resetToDefault()
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
	if len(labels) == 0 {
		unit.Call("appendChild", buildStepField(slider, p.Label, stepStr))
	}
	return unit
}

// buildStepField is a quantity's step size: how far one detent of its knob
// moves the value, and the small knob beside it that sets it. A named setting
// has no such field — its positions are the whole numbers 0..n-1, and the
// field only ever said "1" under it.
//
// Returned as a fragment of the two, both children of the cell, because the
// field is positioned against the cell and so is its knob.
func buildStepField(slider js.Value, label, stepStr string) js.Value {
	stepInput := dom.Doc.Call("createElement", "input")
	stepInput.Set("type", "number")
	stepInput.Set("min", "0.0000001")
	stepInput.Set("step", "any")
	stepInput.Set("value", stepStr)
	stepInput.Set("title", docf("param-step", "label", label))
	stepInput.Set("className", "numin u-step")
	knob := dom.Doc.Call("createElement", "span")
	ptr := dom.Doc.Call("createElement", "i")
	knob.Call("appendChild", ptr)
	knob.Set("className", "stepknob")
	knob.Call("setAttribute", "data-no-drag", "")
	knob.Set("title", docf("param-step-knob", "label", label))

	// The step's travel: from the whole range in one step down to a
	// ten-millionth of it, which is finer than a float32 parameter resolves.
	// min and max are attributes, so strings: Float on one panics.
	smin, _ := strconv.ParseFloat(slider.Get("min").String(), 64) //nolint:errcheck // a bad bound gives the default travel below
	smax, _ := strconv.ParseFloat(slider.Get("max").String(), 64) //nolint:errcheck // likewise
	span := smax - smin
	lo, hi := span*1e-7, span
	if !(span > 0) {
		lo, hi = 1e-7, 1
	}
	show := func(v float64) {
		t := (math.Log10(v) - math.Log10(lo)) / (math.Log10(hi) - math.Log10(lo))
		t = math.Max(0, math.Min(1, t))
		ptr.Get("style").Set("transform", "translate(-50%, -100%) rotate("+strconv.FormatFloat(-135+270*t, 'f', 1, 64)+"deg)")
	}
	set := func(v float64) {
		v = math.Max(lo, math.Min(hi, v))
		stepInput.Set("value", strconv.FormatFloat(v, 'g', 3, 64))
		dom.Fire(stepInput, "input")
	}
	cur := func() float64 {
		v, err := strconv.ParseFloat(stepInput.Get("value").String(), 64)
		if err != nil || !(v > 0) {
			return lo
		}
		return v
	}
	show(cur())

	stepInput.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, args []js.Value) any {
		if val, err := strconv.ParseFloat(stepInput.Get("value").String(), 64); err == nil && val > 0 {
			newStep := strconv.FormatFloat(val, 'g', -1, 64)
			slider.Set("step", newStep)
			show(val)
		}
		return nil
	}))
	knob.Call("addEventListener", "wheel", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		e := a[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		if e.Get("deltaY").Float() < 0 {
			set(cur() * 10)
		} else {
			set(cur() / 10)
		}
		return nil
	}), map[string]any{"passive": false})
	// Dragged: one decade per twelve pixels, up for coarser.
	var startY float64
	var dragging bool
	knob.Call("addEventListener", "pointerdown", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		e := a[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		knob.Call("setPointerCapture", e.Get("pointerId"))
		startY, dragging = e.Get("clientY").Float(), true
		return nil
	}))
	knob.Call("addEventListener", "pointermove", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		if !dragging {
			return nil
		}
		dy := startY - a[0].Get("clientY").Float()
		if math.Abs(dy) >= 12 {
			if dy > 0 {
				set(cur() * 10)
			} else {
				set(cur() / 10)
			}
			startY = a[0].Get("clientY").Float()
		}
		return nil
	}))
	knob.Call("addEventListener", "pointerup", dom.FuncOf(func(js.Value, []js.Value) any {
		dragging = false
		return nil
	}))

	// The step's own reset, back to the step the parameter was built with.
	rst := dom.Doc.Call("createElement", "button")
	rst.Set("className", "rst steprst")
	rst.Set("title", docf("reset-step", "label", label, "step", stepStr))
	rst.Set("textContent", "↺")
	rst.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) any {
		stepInput.Set("value", stepStr)
		dom.Fire(stepInput, "input")
		return nil
	}))

	frag := dom.Doc.Call("createDocumentFragment")
	frag.Call("appendChild", stepInput)
	frag.Call("appendChild", knob)
	frag.Call("appendChild", rst)
	return frag
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
// An empty mode is the one f leaves the rack on.
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
	if mode == "" {
		mode = run.selectedMode // the mode f left it on
	}
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
		if l.modelChange && rackUnmoved(mode) {
			layoutSkirts()
		} else {
			quantizeModuleWidths()
		}
	} else if l.skirts {
		layoutSkirts()
	}
	l.modelChange = false
}

// The rack as the last full pass left it (quantizeModuleWidths): each shown
// module's name and width in order, and the model it was measured under.
var lastRack struct{ sig, mode string }

// rackUnmoved reports whether a model change has left the rack as the last
// full pass measured it, so the pass can be skipped. The rack is one
// instrument whatever the model — every module is on it for every model and
// none changes size with it — and walking all seventy-one models with the
// pass measuring before and after found it changing nothing: stretching every
// module to 3000px and reading it back, twice, only to put each where it was.
//
// Not when something a model change rebuilds can change size with the model:
// the Section and Physics modules, and Custom's editor. (The Mod module is
// one size for every model.) And not when the shown modules or their widths
// differ from the last pass's, which is the check that the rest holds.
func rackUnmoved(mode string) bool {
	if sect.on || physOn() || mode == "custom" || lastRack.mode == "custom" || lastRack.sig == "" {
		return false
	}
	return fastDOM().Call("rackSig").String() == lastRack.sig
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
	// A new model is a new rack, not the old one with a knob turned: every
	// module's width starts again from what is on it. The latch is there so a
	// KNOB does not move the rack (latchModuleWidths), and a model change
	// moves it anyway. Latched across models, the widths only ever added up:
	// one model's twenty-odd leftovers held Parameters at 22 slots under every
	// model after it, and a Console three slots wide on one model and a
	// Patchbay two wide on another pushed the Patchbay into a bay of its own
	// on any model visited after both.
	clear(moduleWidthHighWater)
	// Free the previous build's listener closures, then collect this build's
	// — the wipe below kills their DOM in the same synchronous pass.
	defer dom.StartPanelBuild()()

	paramsDiv := dom.Doc.Call("getElementById", "params")
	paramsDiv.Set("innerHTML", "")
	// Custom's bank cells were built with its panel, and their listeners go
	// with it (bankCustomCells); another model's build takes them out.
	if mode != "custom" {
		clearCustomBankCells()
	}
	paramsDiv.Set("className", "row")
	paramControls = paramControls[:0] // rebuilt below by buildParamUnit

	// Mode-scoped scope extras (run before any early return so they clean up on
	// every mode change): GA waveform switches + the CRT overlay.
	pong.syncPongExtras(mode)
	ftext.syncScopeTextExtras(mode)
	ball.syncBounceExtras(mode)
	morph.syncSprottMorphExtras(mode)
	syncMapExtras(mode)
	syncTermAnimExtras(mode)
	syncLayersModule(mode)
	syncSpectroModule(mode)
	syncDeskModel(mode)
	lyap.syncAnalysisModule(mode)
	clearTurtlePhysModule()
	clearSectionModule()
	phos.updateCRTOverlay()

	// The Equation module: Custom's editor in Custom, the running model's
	// own system in a bank bay (buildEquationView).
	// Every row is a bank, so the module is always there — a model with no
	// equations shows it disabled rather than taking it away, which would
	// slide every module after it in the bay on each change of model.
	switch {
	case mode == "custom":
		// buildCustomPanel makes it, below.
	case bankCategories[rowOf(mode)]:
		custom.buildEquationView(mode, paramsDiv)
	default:
		if em := dom.Doc.Call("getElementById", "eqn-module"); em.Truthy() {
			em.Get("parentNode").Call("removeChild", em)
		}
	}

	// The patch memories in Presets.
	buildPatchBank()

	// The Section module, when the Poincaré overlay is switched on. Before the
	// mode branches below, because those return early for the modes with no
	// parameter grid of their own — and the overlay works on every flow, not
	// only the ones that happen to have knobs.
	buildSectionModule(mode, paramsDiv)

	// The Mod module goes with the panel, so they are rebuilt on every
	// path out of it, the early return included. Skipped there, the last
	// model's modules stayed on the page — its cards, beside a model they do
	// not drive — with every listener on them already freed by the arena.
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
	// What is left for this module is the Custom editor, and a readout of the
	// running model's where its row has no readout line (liveReadoutHost). The
	// readouts, the selectors and the switches a model used to add here are on
	// the readout line, in the bank and on the head's switches. The grid below
	// is the fallback container; if nothing goes into it the module is hidden,
	// because a module with a header and a void under it reads as broken rather
	// than as empty.
	params := attractorParams[mode]
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid")
	paramsDiv.Call("appendChild", grid)
	// Decided at the end, once the extras below have had their chance at it.
	defer func() { showParamsModule(grid.Get("childElementCount").Int() > 0) }()

	buildTurtlePhysModule(mode, paramsDiv)
	applyModuleVisibility() // a rebuild puts back what the switches took away

	if mode == "takens" {
		// A measurement, so on the line over the equation (liveReadoutHost)
		// rather than a Parameters module of its own; the grid where the
		// row has no such line, because #params stacks below the
		// height-bounded grid and gets clipped.
		emb.appendTakensEstimate(liveReadoutHost(mode, grid))
	}

	if mode == "recurrence" {
		// RR, DET and LAM on the line over the equation, like takens'. The
		// readouts are also what tells the per-frame scan anything is
		// displaying its result: a rebuild replaces them, and this is where the
		// new ones are handed over. Their history is on the head's screen
		// (rqaseries_js.go).
		rp.appendRecurrenceRQA(liveReadoutHost(mode, grid))
	}

	if mode == "bifurcation" {
		// Where the audio drive puts the swept parameter, on the same line.
		bif.appendCursorReadout(liveReadoutHost(mode, grid))
		bif.syncSweepCell()
		bif.syncDepthCell()
	}

	if mode == "fvf" {
		syncFVFRoute() // the machine may have been rewired while it was away
	}

	if mode == "stereo" {
		// The correlation readout, on the line over the equation like takens'.
		stereo.appendReadout(liveReadoutHost(mode, grid))
	}

	if mode == "xfer" {
		// The fitted bulk delay, on the line over the equation like takens'.
		xf.appendTransferReadout(liveReadoutHost(mode, grid))
	}

	if mode == "waterfall" {
		// The reverberation time, which the surface is far too shallow to show.
		wfall.appendReadout(liveReadoutHost(mode, grid))
	}

	if mode == "xy" {
		// The goniometer's own correlation meter — the number every hardware
		// one carries beside the tube, and the one the figure cannot give you,
		// because a thin ellipse and a line are the same picture at a glance.
		xy.appendXYReadout(liveReadoutHost(mode, grid))
	}

	// Every monitor readout goes blank first: the model it measured may
	// not be the one running now, and a number left on a screen is a claim.
	if ros := dom.Doc.Call("querySelectorAll", ".monread"); ros.Truthy() {
		for i := range ros.Get("length").Int() {
			ros.Index(i).Set("textContent", "")
		}
	}
	if _, isFlow := lyapLiveSystem(mode); isFlow {
		// The live Lyapunov exponent, same placement and same reason. Only on
		// the continuous flows, and lyapLiveSystem is what draws that line —
		// from the mode's declared class rather than from whether dynamics.FlowFor4
		// happens to answer, for the reason spelled out there. A map's
		// exponent is per iterate and a polyhedron has none; both belong to
		// the Analysis module, which can say so in words, rather than to a
		// cell in this grid that could only print a number or a dash.
		if ro := lyapMonitorFor(mode); ro.Truthy() {
			lyapLive.attachMonitorReadout(ro)
		} else {
			lyapLive.appendLyapunovReadout(grid)
		}
	}

	// The Mod module, always shown.
	buildModMatrix(params)
	if rebindParamWheel != nil {
		rebindParamWheel()
	}
	quantizeModuleWidths()    // param count changed → re-snap module widths
	annotateControlTooltips() // role-aware tooltips (labels/readouts/swatches)
	syncSweptMarks()          // a rebuilt row has lost its swept marking
	syncLinkMarks()           // and its per-control link badges
	layoutSkirts()            // skirts are measured, so they are sized once the rows exist
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
	h.Set("title", doc("phys-module"))
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
	h.Set("title", doc("sect-module"))
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
		i := max(sel.Get("selectedIndex").Int(), 0)
		box.Set("checked", i == 1)
		name.Set("textContent", labels[clampIndex(i, len(labels))])
		// What it is now, and what the next click gives — which is the question
		// a two-position control actually raises, and one the wrapper's tooltip
		// cannot answer because it does not change.
		name.Set("title", labels[clampIndex(i, len(labels))]+" — click or scroll for "+labels[clampIndex(1-i, len(labels))])
	}
	box.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		idx := 0
		if box.Get("checked").Bool() {
			idx = 1
		}
		sel.Set("selectedIndex", idx)
		dom.Fire(sel, "change")
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
	wrap.Call("addEventListener", "wheel", dom.FuncOf(func(_ js.Value, args []js.Value) any {
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
		dom.Fire(sel, "change")
		return nil
	}))
	// The select can move without the switch being touched, and then the switch
	// has to catch up or it is lying about the state it controls.
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
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
