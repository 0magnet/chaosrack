//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"strconv"
	"strings"
	"syscall/js"
)

// A hierarchy of types for the control panel. The panel is, conceptually:
//
//	Module (a .sect: "Position", "View", "Palette"…)
//	  └─ Control (a knob cell: "Zoom", "X spin rate", "start color"…)
//	       └─ elements (label, LED readout, knob, slider, reset, swatch…)
//
// Historically this hierarchy existed only implicitly in the DOM (identified by
// header text / classes / ids) and every cross-cutting concern (tooltips, CRT
// dimming, …) re-derived it independently. These types make it explicit: the
// registry is (re)built from the panel once per rebuild, and each concern
// iterates typed values instead of re-walking the DOM. Builders will construct
// Controls directly in a later pass; for now the model is derived from the DOM,
// but the *ownership* of behavior (a Control annotates itself, knows whether a
// phosphor overrides it, …) now lives on the type.

// controlKind selects how a Control's elements map to the module/control/element
// tooltip hierarchy — the cell layouts differ enough to need it.
type controlKind int

const (
	kindGeneric  controlKind = iota // label + knob + readout(s) (params, view, display, colors, style…)
	kindRotation                    // View axis: angle (top) + spin-rate ω (bottom)
	kindPalette                     // color swatch + hex + Hue/Level knobs
)

// Control is one labeled control cell within a Module.
type Control struct {
	module      string      // owning module name ("Position")
	kind        controlKind // element layout
	cell        js.Value    // the .pcell / .punit element
	crtOverride bool        // dimmed while a phosphor (CRT mode) overrides it
	tipIdx      int         // position in buildControlModel's cell enumeration, for the batched tooltip pass

	// Element references and value metadata — populated when a builder
	// constructs the Control (buildParamUnit). DOM-derived Controls leave these
	// zero; the fields let a Control own its own reset / LED format / permalink
	// key instead of that logic living in scattered per-id handlers and tables.
	slider    js.Value // hidden <input type=range> — the value source of truth
	def       float32  // reset-to value
	ledInt    int      // LED integer digits
	ledDec    int      // LED decimal places
	ledSign   bool     // LED shows a sign
	permaKey  string   // permalink key (e.g. "p.sigma")
	resetHook func()   // extra work a reset needs beyond restoring the value

	// A SELECTOR-backed control instead of a slider-backed one: sel is the
	// <select> that holds the value and selDef the option value a reset returns
	// to. Exactly one of slider and sel is ever set.
	//
	// Kept as its own field rather than overloading slider, because the two are
	// read differently everywhere it matters — a slider's value is a float to
	// compare with a tolerance, a select's is a string to compare exactly, and
	// the event that commits one is "input" where the other is "change".
	sel    js.Value
	selDef string

	// skipResetAll excludes this control from the Reset All sweep (see
	// ControlDesc.SkipResetAll); its own reset button still drives it.
	skipResetAll bool
}

// formatValue renders a value for this control's LED readout using its owned
// integer/decimal/sign format.
func (c *Control) formatValue(v float64) string {
	return led.Format(v, c.ledInt, c.ledDec, c.ledSign)
}

// resetToDefault sets the control's value back to its default (drives the
// hidden slider, whose input handler does the real work). No-op for Controls
// without a slider reference (e.g. purely DOM-derived ones).
func (c *Control) resetToDefault() {
	if c.sel.Truthy() {
		c.sel.Set("value", c.selDef)
		c.sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		if c.resetHook != nil {
			c.resetHook()
		}
		return
	}
	if !c.slider.Truthy() {
		return
	}
	c.slider.Set("value", strconv.FormatFloat(float64(c.def), 'g', -1, 32))
	c.slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	if c.resetHook != nil {
		c.resetHook()
	}
}

// Module is one .sect section holding an ordered set of Controls.
type Module struct {
	name  string
	sect  js.Value
	ctrls []*Control
}

// panelModules is the current control model, rebuilt by buildControlModel after
// every panel (re)build.
var panelModules []*Module

// paramControls holds the Controls constructed by buildParamUnit for the
// current mode (transient — cleared each buildParamPanel). buildControlModel
// reuses these typed objects for the param cells instead of re-deriving them
// from the DOM, so param identity flows from the builder.
var paramControls []*Control

// crtOverriddenIDs are the cells a selected phosphor overrides (their color /
// trail is taken over by the phosphor), so they dim in CRT mode.
var crtOverriddenIDs = map[string]bool{
	"src-cell": true, "map-cell": true, "grp-cstart": true, "grp-cmid": true, "grp-cend": true, "trail-controls": true,
}

// buildControlModel (re)derives the Module/Control registry from the panel DOM.
func buildControlModel() {
	panelModules = panelModules[:0]
	tipN := 0
	sects := dom.Doc.Call("querySelectorAll", ".modules .sect:not(.template-mod)")
	for i := 0; i < sects.Get("length").Int(); i++ {
		sect := sects.Index(i)
		m := &Module{sect: sect}
		if h := sect.Call("querySelector", ".sect-hdr"); h.Truthy() {
			m.name = titleWord(h.Get("textContent").String())
		}
		cells := sect.Call("querySelectorAll", ".pcell, .punit")
		for j := 0; j < cells.Get("length").Int(); j++ {
			cell := cells.Index(j)
			c := findBuiltControl(cell) // reuse a builder-made Control if this is a param cell
			if c == nil {
				c = &Control{module: m.name, cell: cell, kind: classifyControl(cell)}
			}
			c.module = m.name
			c.tipIdx = tipN
			tipN++
			if id := cell.Get("id").String(); id != "" && crtOverriddenIDs[id] {
				c.crtOverride = true
			}
			m.ctrls = append(m.ctrls, c)
		}
		panelModules = append(panelModules, m)
	}
}

// findBuiltControl returns the builder-constructed Control for a cell, if any
// (matched by DOM identity), so the model reuses it instead of re-deriving.
func findBuiltControl(cell js.Value) *Control {
	// Both lists: paramControls is the running model's readouts and editors,
	// cleared on every rebuild, and catParamControls is every category row's
	// parameter cell, built once and kept.
	for _, c := range paramControls {
		if c.cell.Equal(cell) {
			return c
		}
	}
	for _, c := range catParamControls {
		if c.cell.Equal(cell) {
			return c
		}
	}
	return nil
}

func classifyControl(cell js.Value) controlKind {
	cl := cell.Get("classList")
	switch {
	case cl.Call("contains", "axrot").Bool():
		return kindRotation
	case cl.Call("contains", "pal-cell").Bool():
		return kindPalette
	default:
		return kindGeneric
	}
}

// annotateControlTooltips rebuilds the model and lets every Control tooltip
// itself — the SINGLE SOURCE for "Module / Control / element" tooltips.
func annotateControlTooltips() {
	buildControlModel()
	// The whole panel read once, then every control works out its own
	// titles from that and the writes go back in one crossing. See
	// readPanelCells.
	h := fastDOM()
	tipBatching = h.Truthy() && readPanelCells(h)
	for _, m := range panelModules {
		for _, c := range m.ctrls {
			tipCell = c.tipIdx
			c.annotate()
		}
	}
	flushStamps()
}

// annotate stamps this control's elements with the module/control/element
// hierarchy, per its kind.
func (c *Control) annotate() {
	// What the read pass found for this cell, or nil when there was none.
	f := c.cellFacts()
	switch c.kind {
	case kindRotation:
		axis := ""
		if f != nil {
			axis = strings.TrimSpace(f.Axis)
		} else if l := c.cell.Call("querySelector", ".toprow .plabel"); l.Truthy() {
			axis = strings.TrimSpace(l.Get("textContent").String())
		}
		angle := c.module + sep + axis + " angle"
		rate := c.module + sep + axis + " spin rate (ω)"
		stampAll(c.cell, ".toprow .plabel", angle+sep+"label")
		stampAll(c.cell, ".toprow .led", angle+sep+"LED readout")
		stampAll(c.cell, ".knob:not(.knob-fine)", angle+sep+"knob")
		stampAll(c.cell, ".knob-fine", angle+sep+"fine-trim knob")
		// knobifyFixed nests the spin-rate knob inside the angle stack as
		// .knobwrap.knob-inner > .knob; name it for the rate so the two knobs
		// never share a title.
		stampAll(c.cell, ".knobwrap.knob-inner .knob:not(.knob-fine)", rate+sep+"knob (nested inner disc)")
		// The rate sub-row has its own knob + fine disc (knobifyFixed) — name
		// them for the rate, not the angle, so the two knobs stay distinct.
		stampAll(c.cell, ".axsub .knob:not(.knob-fine)", rate+sep+"knob")
		stampAll(c.cell, ".axsub .knob-fine", rate+sep+"fine-trim knob")
		stampAll(c.cell, ".botrow .plabel", rate+sep+"ω label") // carries the ω glyph
		stampAll(c.cell, ".botrow .numin", rate+sep+"value field")
		stampAll(c.cell, ".axsub .axlbl", rate+sep+"label")
		stampAll(c.cell, ".axsub .numin", rate+sep+"value field")
		stampAll(c.cell, "input[type=range]", rate+sep+"slider")
		stampAll(c.cell, ".rst", c.module+sep+axis+sep+"reset (angle + spin)")

	case kindPalette:
		ctl := cellCtl(f, c.cell, c.module) + " color"
		stampAll(c.cell, ".plabel", ctl+sep+"label")
		stampAll(c.cell, "input[type=color]", ctl+sep+"color swatch")
		stampAll(c.cell, ".pal-hex", ctl+sep+"hex readout")
		stampAll(c.cell, ".rst", ctl+sep+"reset")
		stampAll(c.cell, ".hueknob", ctl+sep+"hue knob (outer)")
		stampAll(c.cell, ".colorknob .knob:not(.hueknob)", ctl+sep+"level knob (inner)")

	default: // kindGeneric
		// The step/fine dual cell holds TWO stacked controls; naming both
		// rows from the first label made every element's tooltip collide.
		cellID := ""
		if f != nil {
			cellID = f.ID
		} else {
			cellID = c.cell.Get("id").String()
		}
		if cellID == "stepfine-grp" {
			stepC := c.module + sep + "step ×"
			fineC := c.module + sep + "fine ×"
			stampAll(c.cell, ".sf-hdr .plabel", stepC+sep+"label")
			stampAll(c.cell, "#step-led", stepC+sep+"LED readout")
			stampAll(c.cell, ".sf-ftr .plabel", fineC+sep+"label")
			stampAll(c.cell, "#fine-led", fineC+sep+"LED readout")
			stampSelectorKnobs(f, c.cell, c.module, c.module+sep+"step / fine")
			return
		}
		ctl := cellCtl(f, c.cell, c.module)
		help := cellHelp(f, c.cell)
		stampAll(c.cell, ".plabel:not(.ledcolor-lbl), .u-lbl", withHelp(ctl+sep+"label", help))
		stampLEDs(f, c.cell, c.module, ctl, help)
		stampAll(c.cell, "input[type=range]", ctl+sep+"slider")
		stampAll(c.cell, ".rst", ctl+sep+"reset")
		stampAll(c.cell, ".eqstrip", ctl+sep+"audio EQ (drag to pick frequency bands)")
		stampAll(c.cell, ".knob-fine", ctl+sep+"fine-trim knob")
		numRole := func(step bool) string {
			if step {
				return "step-size field"
			}
			return "value field"
		}
		if f != nil {
			for i, step := range f.Nums {
				queueStamp(".numin", ctl+sep+numRole(step), i)
			}
		} else {
			nums := c.cell.Call("querySelectorAll", ".numin")
			for i := 0; i < nums.Get("length").Int(); i++ {
				n := nums.Index(i)
				n.Set("title", ctl+sep+numRole(n.Get("classList").Call("contains", "u-step").Bool()))
			}
		}
		stampAll(c.cell, ".knob:not(.knobsel):not(.knob-fine)", withHelp(ctl+sep+"knob", help))
		hasKnobSel := false
		if f != nil {
			hasKnobSel = f.NKnob > 0
		} else {
			hasKnobSel = c.cell.Call("querySelector", ".knobsel").Truthy()
		}
		if hasKnobSel {
			stampSelectorKnobs(f, c.cell, c.module, ctl)
		}
	}
}

// applyCRTDim dims (or restores) this control if a phosphor overrides it.
func (c *Control) applyCRTDim(crt bool) {
	c.cell.Get("classList").Call("toggle", "crt-dim", c.crtOverride && crt)
}

// stampLEDs names each readout in a cell, one at a time.
//
// A cell can carry TWO readouts. The Loudness module shows momentary beside
// short-term, integrated beside loudness range, target beside the distance
// from it; Distortion shows SINAD beside ENOB; Wow & Flutter shows wow beside
// flutter. Named from the cell's single label — which is what stamping them
// all at once did — the second one claimed to be the first: the short-term
// readout's tooltip said "Loudness / M / LED readout". That is worse than no
// tooltip, and it is the same bug the step/fine cell above is special-cased
// for.
//
// So a readout with a label of its own is named by it, and the description
// the markup gave it is kept rather than replaced. Those descriptions are the
// only place the panel says what ENOB is, or which window "S" averages over,
// and the stamp was destroying every one of them.
func stampLEDs(f *cellRead, cell js.Value, module, ctl, help string) {
	const sel = ".led:not(.pal-hex)"
	if f != nil {
		for i, l := range f.LEDs {
			name := ctl
			if own := strings.TrimSpace(l.Own); own != "" {
				name = module + sep + own
			}
			desc, keep := ledDescriptionFrom(l.Help, l.Title, help)
			if keep {
				// The description the markup gave this readout, written
				// down before the stamp below overwrites the title it
				// lives in.
				queueAttr(sel, "data-help", desc, i)
			}
			queueStamp(sel, withHelp(name+sep+"LED readout", desc), i)
		}
		return
	}
	leds := cell.Call("querySelectorAll", sel)
	for i := 0; i < leds.Get("length").Int(); i++ {
		l := leds.Index(i)
		name := ctl
		if own := ledOwnLabel(l); own != "" {
			name = module + sep + own
		}
		l.Set("title", withHelp(name+sep+"LED readout", ledDescription(l, help)))
	}
}

// ledDescriptionFrom is ledDescription over what the read pass found: the
// description already memoized, the readout's current tooltip, and the cell's
// fallback sentence. Reports whether the description is newly discovered and
// so has to be written down before the stamp overwrites the title it came
// from.
func ledDescriptionFrom(dataHelp, title, fallback string) (string, bool) {
	if dataHelp != "" {
		return dataHelp, false
	}
	t := strings.TrimSpace(title)
	if t == "" || strings.Contains(t, sep+"LED readout") {
		return fallback, false // nothing authored, or already stamped by an earlier pass
	}
	return t, true
}

// ledOwnLabel is the readout's own label, where it has one.
func ledOwnLabel(led js.Value) string {
	prev := led.Get("previousElementSibling")
	if prev.Truthy() && prev.Get("classList").Call("contains", "ledlbl").Bool() {
		return strings.TrimSpace(prev.Get("textContent").String())
	}
	return ""
}

// ledDescription is what the markup said this readout means.
//
// Captured on the first stamp, because the stamp is what overwrites it, and
// annotate runs again on every panel rebuild.
func ledDescription(led js.Value, fallback string) string {
	if d := led.Call("getAttribute", "data-help"); d.Truthy() {
		if s := d.String(); s != "" {
			return s
		}
	}
	t := strings.TrimSpace(led.Get("title").String())
	if t == "" || strings.Contains(t, sep+"LED readout") {
		return fallback // nothing authored, or already stamped by an earlier pass
	}
	led.Call("setAttribute", "data-help", t)
	return t
}
