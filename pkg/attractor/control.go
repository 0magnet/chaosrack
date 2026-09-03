//go:build js && wasm

package attractor

import (
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
}

// formatLED renders a value for this control's LED readout using its owned
// integer/decimal/sign format.
func (c *Control) formatValue(v float64) string {
	return formatLED(v, c.ledInt, c.ledDec, c.ledSign)
}

// resetToDefault sets the control's value back to its default (drives the
// hidden slider, whose input handler does the real work). No-op for Controls
// without a slider reference (e.g. purely DOM-derived ones).
func (c *Control) resetToDefault() {
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
	"src-cell": true, "grp-cstart": true, "grp-cmid": true, "grp-cend": true, "trail-controls": true,
}

// buildControlModel (re)derives the Module/Control registry from the panel DOM.
func buildControlModel() {
	panelModules = panelModules[:0]
	sects := doc.Call("querySelectorAll", ".modules .sect:not(.template-mod)")
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
	for _, c := range paramControls {
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
	for _, m := range panelModules {
		for _, c := range m.ctrls {
			c.annotate()
		}
	}
}

// annotate stamps this control's elements with the module/control/element
// hierarchy, per its kind.
func (c *Control) annotate() {
	switch c.kind {
	case kindRotation:
		axis := ""
		if l := c.cell.Call("querySelector", ".toprow .plabel"); l.Truthy() {
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
		ctl := cellCtl(c.cell, c.module) + " color"
		stampAll(c.cell, ".plabel", ctl+sep+"label")
		stampAll(c.cell, "input[type=color]", ctl+sep+"color swatch")
		stampAll(c.cell, ".pal-hex", ctl+sep+"hex readout")
		stampAll(c.cell, ".rst", ctl+sep+"reset")
		stampAll(c.cell, ".hueknob", ctl+sep+"hue knob (outer)")
		stampAll(c.cell, ".colorknob .knob:not(.hueknob)", ctl+sep+"level knob (inner)")

	default: // kindGeneric
		// The step/fine dual cell holds TWO stacked controls; naming both
		// rows from the first label made every element's tooltip collide.
		if c.cell.Get("id").String() == "stepfine-grp" {
			stepC := c.module + sep + "step ×"
			fineC := c.module + sep + "fine ×"
			stampAll(c.cell, ".sf-hdr .plabel", stepC+sep+"label")
			stampAll(c.cell, "#step-led", stepC+sep+"LED readout")
			stampAll(c.cell, ".sf-ftr .plabel", fineC+sep+"label")
			stampAll(c.cell, "#fine-led", fineC+sep+"LED readout")
			stampSelectorKnobs(c.cell, c.module, c.module+sep+"step / fine")
			return
		}
		ctl := cellCtl(c.cell, c.module)
		stampAll(c.cell, ".plabel:not(.ledcolor-lbl), .u-lbl", ctl+sep+"label")
		stampAll(c.cell, ".led:not(.pal-hex)", ctl+sep+"LED readout")
		stampAll(c.cell, "input[type=range]", ctl+sep+"slider")
		stampAll(c.cell, ".rst", ctl+sep+"reset")
		stampAll(c.cell, ".eqstrip", ctl+sep+"audio EQ (drag to pick frequency bands)")
		stampAll(c.cell, ".knob-fine", ctl+sep+"fine-trim knob")
		nums := c.cell.Call("querySelectorAll", ".numin")
		for i := 0; i < nums.Get("length").Int(); i++ {
			n := nums.Index(i)
			role := "value field"
			if n.Get("classList").Call("contains", "u-step").Bool() {
				role = "step-size field"
			}
			n.Set("title", ctl+sep+role)
		}
		stampAll(c.cell, ".knob:not(.knobsel):not(.knob-fine)", ctl+sep+"knob")
		if c.cell.Call("querySelector", ".knobsel").Truthy() {
			stampSelectorKnobs(c.cell, c.module, ctl)
		}
	}
}

// applyCRTDim dims (or restores) this control if a phosphor overrides it.
func (c *Control) applyCRTDim(crt bool) {
	c.cell.Get("classList").Call("toggle", "crt-dim", c.crtOverride && crt)
}
