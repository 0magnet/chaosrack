//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// The Template module is a labeled legend of every named slot a real module
// can hold. It renders like a native module (same classes / chrome) but each
// slot is named IN PLACE — text slots (header, label, readout) show their own
// name, and round slots (knob, fine, ring, inner) carry a small overlay label
// pinned to the exact element. Every slot's tooltip maps the spatial name to
// the Go struct field it corresponds to (Module.* / Control.*), sharing the
// vocabulary annotate() uses for live-control tooltips. Toggled by the
// "Template" switch; excluded from buildControlModel so it is never annotated
// over.
//
// Two cells cover the anatomy:
//   1. a continuous control — label + LED readout (top) + value knob (fine,
//      dial, dial-label) + reset.
//   2. a selector control — label + concentric knob (ring + inner) + a
//      foot-readout BELOW the knob (the other readout position).

var tplSect js.Value

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
	o := tel("span", "tpl-ov")
	o.Set("textContent", text)
	o.Set("title", note)
	o.Get("style").Set("top", strconv.FormatFloat(topPct, 'f', 0, 64)+"%")
	return o
}

func buildTemplateModule() {
	modules := doc.Call("querySelector", ".modules")
	if !modules.Truthy() {
		return
	}
	sect := tel("div", "sect template-mod")
	sect.Get("style").Set("display", "none")

	hdr := tel("div", "sect-hdr")
	hdr.Set("textContent", "header")
	hdr.Set("title", "header — Module.name (.sect-hdr) · the section title")
	sect.Call("appendChild", hdr)

	row := tel("div", "row vmrow")

	// ── Cell 1: a continuous control (label + LED readout + value knob) ──
	c1 := tel("span", "pcell axcol vmcell tpl-cell")
	c1.Set("title", "cell — a Control (.pcell) · one labeled control in the module")
	top1 := tel("span", "punit-top")
	lbl1 := tel("span", "plabel")
	lbl1.Set("textContent", "label")
	lbl1.Set("title", "label — the Control's name (.plabel)")
	ro1 := tel("span", "led tpl-led")
	ro1.Set("textContent", "readout")
	ro1.Set("title", "readout — the value LED (.led); format = Control.ledInt / ledDec / ledSign")
	top1.Call("appendChild", lbl1)
	top1.Call("appendChild", ro1)
	c1.Call("appendChild", top1)

	bay1 := tel("span", "grp vmbay")
	wrap := tel("span", "knobwrap has-dial")
	dial := tel("div", "knob-dial value-dial")
	for i := 0; i <= 8; i++ {
		deg := -knobSweepDeg/2 + knobSweepDeg*float64(i)/8
		cls := "vdial-tick"
		if i%4 == 0 {
			cls += " major"
		}
		tk := tel("span", cls)
		l, tp := dialLabelPos(deg, 41)
		st := tk.Get("style")
		st.Set("left", l)
		st.Set("top", tp)
		st.Set("transform", "translate(-50%,-50%) rotate("+strconv.FormatFloat(deg, 'f', 1, 64)+"deg)")
		dial.Call("appendChild", tk)
	}
	dl := tel("span", "knob-dial-lab")
	dl.Set("textContent", "dial")
	dl.Set("title", "dial — tick / scale ring (.knob-dial); its numbers are dial-labels (.knob-dial-lab)")
	dlL, dlT := dialLabelPos(135, 50) // lower-right, where a real scale number sits
	dl.Get("style").Set("left", dlL)
	dl.Get("style").Set("top", dlT)
	dial.Call("appendChild", dl)
	wrap.Call("appendChild", dial)

	knob := tel("div", "knob knobb")
	knob.Call("appendChild", tel("i", "knob-ptr"))
	fine := tel("div", "knob-fine")
	knob.Call("appendChild", fine)
	knob.Call("appendChild", tplOv("knob", "knob — the main control (.knob.knobb), driven by Control.slider", 32))
	knob.Call("appendChild", tplOv("fine", "fine — fine-trim disc (.knob-fine) · sub-step nudge", 54))
	wrap.Call("appendChild", knob)
	bay1.Call("appendChild", wrap)
	c1.Call("appendChild", bay1)

	rst := tel("span", "rst")
	rst.Set("textContent", "↺")
	rst.Set("title", "reset — restore the default (.rst → Control.def / resetToDefault())")
	c1.Call("appendChild", rst)
	row.Call("appendChild", c1)

	// ── Cell 2: a selector control (concentric knob + foot-readout below) ──
	c2 := tel("span", "pcell axcol vmcell tpl-cell")
	c2.Set("title", "cell — a Control (.pcell) · concentric-knob variant")
	top2 := tel("span", "punit-top")
	lbl2 := tel("span", "plabel")
	lbl2.Set("textContent", "label")
	lbl2.Set("title", "label — the Control's name (.plabel)")
	top2.Call("appendChild", lbl2)
	c2.Call("appendChild", top2)

	bay2 := tel("span", "grp vmbay")
	selkro := tel("span", "selk-ro")
	stack := tel("span", "knobstack")
	ring := tel("div", "knob knobsel knob-ring")
	ring.Call("appendChild", tel("i", "knob-ptr"))
	ring.Call("appendChild", tplOv("ring", "ring — outer concentric selector (.knob-ring)", 24))
	inner := tel("div", "knob knobsel knob-inner")
	inner.Call("appendChild", tel("i", "knob-ptr"))
	inner.Call("appendChild", tplOv("inner", "inner — inner concentric knob (.knob-inner)", 50))
	stack.Call("appendChild", ring)
	stack.Call("appendChild", inner)
	selkro.Call("appendChild", stack)
	foot := tel("span", "selk-readout")
	foot.Set("textContent", "foot")
	foot.Set("title", "foot-readout — a readout BELOW the knob (.selk-ro > .selk-readout), e.g. the phosphor / LED-color name")
	selkro.Call("appendChild", foot)
	bay2.Call("appendChild", selkro)
	c2.Call("appendChild", bay2)
	row.Call("appendChild", c2)

	sect.Call("appendChild", row)

	// Document the alignment grid: every knob cell is one --krow tall and centers
	// its knob, so the Nth knob lines up across all modules by construction.
	note := tel("div", "tpl-note")
	note.Set("innerHTML", "grid — each knob cell is one <b>--krow</b> tall (a fixed row) and centers its knob, so the Nth knob aligns across every module")
	note.Set("title", "--krow / --kcol (panel.css) — the knob-row grid: .vmcell / .axcol.axrot are exactly one row tall, punit-top pinned top, knob centered")
	sect.Call("appendChild", note)

	modules.Call("appendChild", sect)
	tplSect = sect
}

// setTemplate shows or hides the Template module.
func setTemplate(on bool) {
	if !tplSect.Truthy() {
		return
	}
	if on {
		tplSect.Get("style").Set("display", "")
	} else {
		tplSect.Get("style").Set("display", "none")
	}
}
