//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// The Template module is a live legend of every control type the panel uses.
// Each cell is built with the SAME builders and classes as the real modules
// (makeKnob with dial+fine, stackKnobs+makeSelectorKnob with clickable ring
// labels, the View axis cell markup, the Palette cell markup, buildModCard,
// buildEQCard) — so what you see is exactly the anatomy the live modules
// use; only the slot names and tooltips differ. Every slot is named in
// place, and its tooltip maps the spatial name to the Go struct field /
// builder it corresponds to (the same vocabulary annotate() uses). There are
// no visible sliders anywhere — like the real panel, every range input is
// hidden behind a knob. Toggled by Window > Template; excluded from
// buildControlModel so it is never annotated over.
//
// Six cells cover the whole knob vocabulary:
//   value    — continuous knob: scale dial + fine disc + LED + reset
//              (Parameters / Position / Display cells).
//   selector — concentric ring + inner selector with clickable labels
//              (Colors src, Gen out/wave, Model Out map/mode, Style).
//   angle    — View-axis cell: pose knob (nested rate disc) + degree LED
//              + rate value row.
//   color    — Palette cell: hue ring + level disc around a swatch.
//   mod      — the audio-mod channel/depth card (Mod module).
//   eq       — the graphic-EQ band painter card (EQ module).

var (
	tplSect    js.Value
	tplDemoVal float32 = 5 //nolint:unused // written by the demo knob
)

func tel(tag, cls string) js.Value {
	e := doc.Call("createElement", tag)
	if cls != "" {
		e.Set("className", cls)
	}
	return e
}

// tplOv is a small overlay label pinned onto a round slot (knob / ring / …)
// that can't hold its own text, at topPct% down the element, with the struct
// mapping on hover.
func tplOv(text, note string, topPct float64) js.Value {
	_ = note // the element beneath carries the tooltip (overlays are pointer-events:none)
	o := tel("span", "tpl-ov")
	o.Set("textContent", text)
	o.Get("style").Set("top", strconv.FormatFloat(topPct, 'f', 0, 64)+"%")
	return o
}

// tplHiddenRange makes the standard hidden range input that backs a knob —
// the panel never shows sliders; knobs drive them.
func tplHiddenRange(min, max, step, val string) js.Value {
	sl := tel("input", "")
	sl.Set("type", "range")
	sl.Set("min", min)
	sl.Set("max", max)
	sl.Set("step", step)
	sl.Set("value", val)
	sl.Get("style").Set("display", "none")
	return sl
}

func buildTemplateModule() {
	modules := doc.Call("querySelector", ".modules")
	if !modules.Truthy() {
		return
	}
	sect := tel("div", "sect template-mod")
	sect.Get("style").Set("display", "none")

	hdr := tel("div", "sect-hdr")
	hdr.Set("textContent", "Template")
	hdr.Set("title", "Template — a live legend built with the real cell builders: hover any slot for the Go struct field / builder it maps to (this header = Module.name, .sect-hdr)")
	sect.Call("appendChild", hdr)

	row := tel("div", "row vmrow")

	// ── value: a real continuous cell (hidden slider → makeKnob dial+fine) ──
	vcell := tel("span", "pcell axcol vmcell tpl-cell")
	vcell.Set("title", "value cell — one continuous Control (.pcell): label + LED + knob + reset (buildParamUnit pattern)")
	vtop := tel("span", "punit-top")
	vlbl := tel("span", "plabel")
	vlbl.Set("textContent", "value")
	vlbl.Set("title", "label — the Control's name (.plabel / .u-lbl)")
	vtop.Call("appendChild", vlbl)
	vslider := tplHiddenRange("0", "10", "0.1", "5")
	vled := tel("input", "numin u-val")
	vled.Set("type", "text")
	vled.Set("value", "05.0")
	vled.Set("title", "readout — the value LED (.numin.u-val); format = Control.ledInt / ledDec / ledSign")
	vslider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(vslider.Get("value").String(), 64); err == nil {
			tplDemoVal = float32(v)
			vled.Set("value", strconv.FormatFloat(v, 'f', 1, 64))
		}
		return nil
	}))
	vtop.Call("appendChild", vled)
	vcell.Call("appendChild", vtop)
	vcell.Call("appendChild", vslider)
	vknob := makeKnob(vslider, vled, true, false, true)
	vknob.Call("appendChild", tplOv("knob", "knob — the main control (.knob from makeKnob), driven by the hidden Control.slider", 30))
	vknob.Call("appendChild", tplOv("fine", "fine — fine-trim disc (.knob-fine) · drags at the Fine × fraction of a step", 78))
	vcell.Call("appendChild", vknob)
	vrst := tel("button", "rst")
	vrst.Set("textContent", "↺")
	vrst.Set("title", "reset — restore the default (.rst → Control.def / resetToDefault())")
	vcell.Call("appendChild", vrst)
	row.Call("appendChild", vcell)

	// ── selector: a real concentric dual selector (Colors src pattern) ──
	mkSel := func(opts []string) js.Value {
		sel := tel("select", "")
		for _, o := range opts {
			op := tel("option", "")
			op.Set("textContent", o)
			sel.Call("appendChild", op)
		}
		sel.Get("style").Set("display", "none")
		return sel
	}
	selA := mkSel([]string{"A", "B", "C", "trl"})
	selB := mkSel([]string{"1", "2", "3", "∞"})
	scell := tel("span", "pcell axcol tpl-cell")
	scell.Set("title", "selector cell — a concentric dual selector: each ring drives a hidden <select> (stackKnobs + makeSelectorKnob)")
	sbay := tel("span", "grp vmbay")
	sgrp := tel("span", "grp knoblbl")
	slbl := tel("span", "plabel")
	slbl.Set("textContent", "selector")
	slbl.Set("title", "label — the Control's name (.plabel)")
	sgrp.Call("appendChild", slbl)
	sstack := stackKnobs(makeSelectorKnob(selA), makeSelectorKnob(selB))
	addSelectorLabels(sstack, []string{"A", "B", "C", "trl"}, selA, 43)
	addSelectorLabels(sstack, []string{"1", "2", "3", "∞"}, selB, 31)
	if ring := sstack.Call("querySelector", ".knob-ring"); ring.Truthy() {
		ring.Set("title", "ring — outer concentric selector (.knob-ring), one detent per <option>; its ring labels are clickable")
		ring.Call("appendChild", tplOv("ring", "ring — outer concentric selector (.knob-ring), one detent per <option> (makeSelectorKnob)", 16))
	}
	if inner := sstack.Call("querySelector", ".knob-inner"); inner.Truthy() {
		inner.Set("title", "inner — inner concentric selector (.knob-inner), an independent second select")
		inner.Call("appendChild", tplOv("inner", "inner — inner concentric selector (.knob-inner), an independent second select", 50))
	}
	scell.Call("appendChild", selA)
	scell.Call("appendChild", selB)
	sgrp.Call("appendChild", sstack)
	sbay.Call("appendChild", sgrp)
	scell.Call("appendChild", sbay)
	row.Call("appendChild", scell)

	// ── angle: the View-axis cell (pose knob + nested rate disc + LEDs) ──
	acell := tel("span", "pcell axcol axrot tpl-cell")
	acell.Set("title", "angle cell — a View axis (.axcol.axrot, kindRotation): pose knob with the spin-rate knob nested inside, degree LED, rate value row")
	albl := tel("span", "plabel")
	albl.Set("textContent", "angle")
	albl.Set("title", "label — the axis name (.plabel)")
	acell.Call("appendChild", albl)
	agrp := tel("span", "grp")
	aknob := tel("div", "knob")
	aknob.Call("appendChild", tel("i", "knob-ptr"))
	aknob.Set("title", "angle knob — absolute pose about one axis; the nested disc is the SPIN-RATE knob (knobifyFixed nests it inside)")
	aknob.Call("appendChild", tel("div", "knob-fine"))
	aknob.Call("appendChild", tplOv("angle", "angle knob — absolute pose about one axis (kindRotation; euler angles rebuilt per frame)", 30))
	aknob.Call("appendChild", tplOv("rate", "rate knob — the spin-rate control, nested as the inner disc (knobifyFixed)", 78))
	agrp.Call("appendChild", aknob)
	aled := tel("span", "led")
	aled.Set("textContent", "000°")
	aled.Set("title", "degree LED — the axis angle readout (.led)")
	agrp.Call("appendChild", aled)
	acell.Call("appendChild", agrp)
	asub := tel("span", "grp axsub")
	asublbl := tel("span", "axlbl")
	asublbl.Set("textContent", "rate")
	asublbl.Set("title", "rate label — the spin-rate sub-row (.axsub .axlbl)")
	asub.Call("appendChild", asublbl)
	asub.Call("appendChild", tplHiddenRange("-1", "1", "0.1", "0")) // hidden, knob-driven like the real row
	anum := tel("input", "numin")
	anum.Set("type", "number")
	anum.Set("value", "0")
	anum.Set("title", "rate value field — typed entry for the spin rate (.axsub .numin)")
	asub.Call("appendChild", anum)
	arst := tel("button", "rst")
	arst.Set("textContent", "↺")
	arst.Set("title", "reset — zero the angle AND the spin rate (ControlDesc.ResetExtra)")
	asub.Call("appendChild", arst)
	acell.Call("appendChild", asub)
	row.Call("appendChild", acell)

	// ── color: the Palette cell markup (swatch + hue/level color knob) ──
	ccell := tel("span", "pcell axcol vmcell pal-cell tpl-cell")
	ccell.Set("title", "color cell — a Palette color (.pal-cell, kindPalette): swatch + reset above the hue/level color knob")
	ctop := tel("span", "punit-top")
	clbl := tel("span", "plabel")
	clbl.Set("textContent", "color")
	clbl.Set("title", "label — the color's role (.plabel): start / mid / end / bg")
	ctop.Call("appendChild", clbl)
	csw := tel("span", "cswatch")
	cinput := tel("input", "")
	cinput.Set("type", "color")
	cinput.Set("value", "#35e06a")
	cinput.Set("title", "swatch — the native color input the knob drives (input[type=color])")
	csw.Call("appendChild", cinput)
	crst := tel("button", "rst")
	crst.Set("textContent", "↺")
	crst.Set("title", "reset — restore this color's default (.rst)")
	csw.Call("appendChild", crst)
	ctop.Call("appendChild", csw)
	ccell.Call("appendChild", ctop)
	cholder := tel("span", "ckholder")
	cholder.Call("appendChild", buildColorKnob(cinput))
	if hue := cholder.Call("querySelector", ".hueknob"); hue.Truthy() {
		hue.Set("title", "hue ring — outer ring sets the hue (buildColorKnob)")
		hue.Call("appendChild", tplOv("hue", "hue ring — outer ring sets the hue (buildColorKnob)", 14))
	}
	if lvl := cholder.Call("querySelector", ".colorknob .knob:not(.hueknob)"); lvl.Truthy() {
		lvl.Set("title", "level knob — inner disc sets the level / brightness")
		lvl.Call("appendChild", tplOv("lvl", "level knob — inner disc sets the level / brightness (buildColorKnob)", 50))
	}
	ccell.Call("appendChild", cholder)
	row.Call("appendChild", ccell)

	// ── mod + eq: the REAL audio-mod cards (same builders as the Mod / EQ
	//    modules; the demo routing id is never applied or serialized) ──
	mcard := buildModCard("tpl-demo", "mod", false)
	mcard.Get("classList").Call("add", "tpl-cell")
	mcard.Set("title", "mod card — routes an audio channel (outer ring) at a depth (inner disc) onto its control (buildModCard → paramMods)")
	if l := mcard.Call("querySelector", ".u-lbl"); l.Truthy() {
		l.Set("title", "mod card label — the modulated control's name (.u-lbl)")
	}
	// makeKnob copies the hidden range's title onto the knob; outside the
	// annotate pass (the template is excluded) that pair would collide.
	if r := mcard.Call("querySelector", "input[type=range]"); r.Truthy() {
		r.Set("title", "depth value source — the hidden range the depth knob drives")
	}
	row.Call("appendChild", mcard)

	ecard := buildEQCard("tpl-demo", "eq", false)
	ecard.Get("classList").Call("add", "tpl-cell")
	ecard.Set("title", "eq card — paint per-band weights on the strip; the weighted band energy drives the MOD route (buildEQCard → paramMod.bands)")
	if l := ecard.Call("querySelector", ".u-lbl"); l.Truthy() {
		l.Set("title", "eq card label — the band-painted control's name (.u-lbl)")
	}
	row.Call("appendChild", ecard)

	sect.Call("appendChild", row)

	// Document the alignment grid: every knob cell is one --krow tall and centers
	// its knob, so the Nth knob lines up across all modules by construction.
	note := tel("div", "tpl-note")
	note.Set("innerHTML", "grid — each cell is one <b>--krow</b> tall and centers its control, so the Nth knob aligns across every module; module widths quantize to whole <b>--kcol</b> slots")
	note.Set("title", "--krow / --kcol (panel.css) — the knob grid: .vmcell / .axcol.axrot are exactly one row tall, punit-top pinned top, knob centered; quantizeModuleWidths sizes modules in whole slots")
	sect.Call("appendChild", note)

	modules.Call("appendChild", sect)
	tplSect = sect
}

// setTemplate shows or hides the Template module. The width quantizer
// measures a hidden module as zero, so re-quantize after showing.
func setTemplate(on bool) {
	if !tplSect.Truthy() {
		return
	}
	if on {
		tplSect.Get("style").Set("display", "")
	} else {
		tplSect.Get("style").Set("display", "none")
	}
	quantizeModuleWidths()
}
