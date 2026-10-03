//go:build js && wasm

package attractor

// The Mod module (bay 7): every audio-modulation route on one pin matrix,
// and the selected route's depth and EQ under it.
//
// Each row is a destination: the running model's parameters, then the view's
// routable controls, in blocks of nine standing side by side. Each column is
// a source (modChannels). A pin routes its column's source to its row; a row
// holds one route, so pressing another pin in it moves the route there, and
// pressing a lit one takes it away.
//
// The matrix is one size whatever model runs. There are as many parameter
// rows as the model with the most parameters has, and a model with fewer
// leaves the rest blank, so nothing in the module changes with the model but
// the names. Each name is on a character display, because a row is a
// different parameter for each model.
//
// It is built once. A model change rewrites the names, which rows are blank
// and DEST's list (modMxRetarget); every pin and name reads which control its
// row is for when it is pressed, so none of them is rewired.
//
// It replaces two surfaces that edited the same pmod.params: the Patchbay's
// pins and a Mod/EQ pair of modules per destination group. The routes, and
// the m./vm. keys a link carries them in, are unchanged.

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
)

// modMxViewCols are the view's routable controls, in the panel's order:
// View's spins and position, then the Colors module's period, shift and
// trail. Not viewModTargets' order, which MIDI's CC map fixes.
var modMxViewCols = []struct{ id, label string }{
	{"view-spinx", "spin X"}, {"view-spiny", "spin Y"}, {"view-spinz", "spin Z"},
	{"view-panx", "pan X"}, {"view-pany", "pan Y"}, {"view-zoom", "zoom"},
	{"view-rfreq", "period"}, {"view-pshift", "shift"}, {"view-trail", "trail"},
}

// modMxDefDepth is the depth a route is made with when it had none.
const modMxDefDepth = 0.4

// modMxBlockRows is how many destinations stand in one block of the matrix:
// a block is a column of names, each with its row of pins, and the blocks
// stand side by side, so the matrix is as tall as one block whatever the
// model. The view's controls are exactly one block.
const modMxBlockRows = 9

// modMxParamCols is how many places the parameters get: as many as the model
// with the most has, in whole blocks (modMxBlockRows), so every model fits
// and no model changes what is in the module. Custom is left out: its
// parameters are whatever its equations name, and it gets as many as fit.
func modMxParamCols() int {
	n := 0
	for m, ps := range attractorParams {
		if m != "custom" {
			n = max(n, len(ps))
		}
	}
	return (n + modMxBlockRows - 1) / modMxBlockRows * modMxBlockRows
}

// modMxCol is what one row routes to: its pmod key and name, or blank.
type modMxCol struct{ id, label string }

// modMxRow is one row's parts: its name display and its pins, in source
// order.
type modMxRow struct {
	legend js.Value
	pins   []js.Value
	named  bool // its name has been written at least once
	// lit is what each pin was last lit as ("" dark, else its opacity),
	// so modMxLight touches only the pins that change: lit from scratch
	// it was four crossings a pin, a thousand on every model change.
	lit []string
}

// modMx is the module's live parts.
var modMx struct {
	sel     string     // the selected row's pmod key
	nParams int        // how many rows are the parameters'
	cols    []modMxCol // what each row routes to now, blanks included
	rows    []modMxRow // each row's parts
	dest    js.Value   // the hidden select the DEST knob turns
	depth   js.Value   // the hidden range behind the DPTH knob
	depthRO js.Value   // its readout
	eqHold  js.Value   // where the selected route's EQ strip goes
	eqFuncs []js.Func  // the strip's listeners, rebuilt with the selection
	funcs   []js.Func  // the module's own listeners, kept for its life
}

// buildModMatrix points the Mod module at params, the running model's
// parameters, building it first, after the Parameters module (the rack's
// packer puts it in the mod bay), if it is not there yet.
func buildModMatrix(params []paramDef) {
	if !dom.Doc.Call("getElementById", "mod-matrix").Truthy() {
		at := dom.Doc.Call("getElementById", "params-module")
		if !at.Truthy() {
			return
		}
		dom.RebuildInto(&modMx.funcs, func() {
			at.Get("parentNode").Call("insertBefore", modMxModule(), at.Get("nextSibling"))
		})
	}
	modMxRetarget(params)
}

// modMxModule builds the module: the matrix, and the selected route's cells
// under it.
func modMxModule() js.Value {
	modMx.nParams = modMxParamCols()
	n := modMx.nParams + len(modMxViewCols)
	modMx.cols = make([]modMxCol, n)
	modMx.rows = make([]modMxRow, n)

	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect")
	mod.Set("id", "mod-matrix")
	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", "Mod")
	h.Call("setAttribute", "data-doc", "mod-matrix")
	h.Set("title", doc("mod-matrix"))
	mod.Call("appendChild", h)
	body := dom.Doc.Call("createElement", "div")
	body.Set("className", "row modmx")
	body.Call("appendChild", modMxGrid())
	body.Call("appendChild", modMxEditRow())
	mod.Call("appendChild", body)
	return mod
}

// modMxGrid is the pin matrix: the rows in blocks of modMxBlockRows, the
// parameters' first and then the view's, each block a column of names on
// character displays with a row of pins beside each, under a row of the
// sources' keys.
func modMxGrid() js.Value {
	blocks := dom.Doc.Call("createElement", "div")
	blocks.Set("className", "modmx-blocks")
	var grid js.Value
	for i := range modMx.rows {
		if i%modMxBlockRows == 0 || i == modMx.nParams {
			grid = dom.Doc.Call("createElement", "div")
			grid.Set("className", "mxgrid")
			grid.Call("setAttribute", "data-doc", "mod-grid")
			grid.Set("title", doc("mod-grid"))
			grid.Get("style").Set("gridTemplateColumns", "auto repeat("+strconv.Itoa(len(modChannels))+",auto)")
			// The groups over their columns, RACK and HEAD, as the Mixer
			// heads its own: HEAD is the trail, not Model Out's sound.
			grid.Call("appendChild", mxAxis("from ▸", "mx-from"))
			for s := 0; s < len(modChannels); {
				e := s
				for e < len(modChannels) && modChannels[e].group == modChannels[s].group {
					e++
				}
				g := dom.Doc.Call("createElement", "span")
				g.Set("className", "mxlbl mixgrp")
				g.Set("textContent", modChannels[s].group)
				g.Call("setAttribute", "data-doc", "mod-group="+modChannels[s].group)
				g.Set("title", doc("mod-group="+modChannels[s].group))
				g.Get("style").Set("gridColumn", "span "+strconv.Itoa(e-s))
				grid.Call("appendChild", g)
				s = e
			}
			grid.Call("appendChild", mxAxis("to ▾", "mx-to"))
			for _, src := range modChannels {
				l := dom.Doc.Call("createElement", "span")
				l.Set("className", "mxlbl")
				l.Set("textContent", src.key)
				l.Call("setAttribute", "data-doc", "mod-src="+src.key)
				l.Set("title", doc("mod-src="+src.key))
				grid.Call("appendChild", l)
			}
			blocks.Call("appendChild", grid)
		}
		row := i
		lg := dotDisplayN("", false, dispFullChars)
		lg.Get("classList").Call("add", "mxleg")
		dom.On(lg, "click", func(js.Value, []js.Value) any {
			modMxSelect(modMx.cols[row].id)
			return nil
		})
		grid.Call("appendChild", lg)
		modMx.rows[i].legend = lg
		for _, src := range modChannels {
			pin := dom.Doc.Call("createElement", "span")
			pin.Set("className", "mxpin")
			pin.Call("setAttribute", "data-src", src.name)
			if i >= modMx.nParams {
				pin.Call("setAttribute", "data-view", "")
			}
			grid.Call("appendChild", pin)
			modMx.rows[i].pins = append(modMx.rows[i].pins, pin)
			ch := src.name
			dom.On(pin, "click", func(js.Value, []js.Value) any {
				id := modMx.cols[row].id
				if id == "" {
					return nil
				}
				m := pmod.params[id]
				if m.channel == ch && m.level != 0 {
					m.channel = ""
				} else {
					m.channel = ch
					if m.level == 0 {
						m.level = modMxDefDepth
					}
				}
				pmod.params[id] = m
				syncAudioMod()
				perma.syncPermalinkNow()
				modMxSelect(id)
				modMxLight(row)
				return nil
			})
			pin.Call("addEventListener", "wheel", dom.FuncOf(func(_ js.Value, a []js.Value) any {
				e := a[0]
				id := modMx.cols[row].id
				if m := pmod.params[id]; id == "" || m.channel != ch || m.level == 0 {
					return nil
				}
				e.Call("preventDefault")
				modMxSelect(id)
				step := 0.05
				if e.Get("deltaY").Float() > 0 {
					step = -step
				}
				v, _ := strconv.ParseFloat(modMx.depth.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
				modMx.depth.Set("value", strconv.FormatFloat(v+step, 'g', -1, 64))
				dom.Fire(modMx.depth, "input")
				return nil
			}), map[string]any{"passive": false})
		}
	}
	return blocks
}

// modMxEditRow is the selected route under the matrix: DEST, a knob that
// steps through the rows with the selected one's name on a display; DPTH,
// its depth; and EQ, the bands that drive it.
func modMxEditRow() js.Value {
	row := dom.Doc.Call("createElement", "div")
	row.Set("className", "modmx-edit")
	cell := func(label, title string) (js.Value, js.Value) {
		c := dom.Doc.Call("createElement", "span")
		c.Set("className", "pcell axcol vmcell")
		c.Set("title", title)
		top := dom.Doc.Call("createElement", "span")
		top.Set("className", "punit-top")
		l := dom.Doc.Call("createElement", "span")
		l.Set("className", "plabel")
		l.Set("textContent", label)
		top.Call("appendChild", l)
		c.Call("appendChild", top)
		row.Call("appendChild", c)
		return c, top
	}

	// DEST: its positions are this model's rows, so they change with the
	// model; it is the lit-dot selector (ledSelector) with the selected one's
	// name on a display rather than a printed ring. Built once: the knob
	// counts its positions as it turns, and modMxDest refills the list.
	dc, dtop := cell("dest", doc("mod-dest"))
	sel := dom.Doc.Call("createElement", "select")
	sel.Set("style", "display:none")
	dc.Call("appendChild", sel)
	hold := dom.Doc.Call("createElement", "span")
	hold.Set("className", "grp vmbay")
	hold.Call("appendChild", ledSelector(sel))
	dc.Call("appendChild", hold)
	dc.Get("classList").Call("add", "ledsel")
	name := dotDisplayN("", false, dispFullChars)
	name.Get("classList").Call("add", "dmdval")
	dtop.Call("appendChild", name)
	ledPick(name, sel)
	dom.On(sel, "change", func(js.Value, []js.Value) any {
		if i := sel.Get("selectedIndex").Int(); i >= 0 {
			setDotText(name, sel.Get("options").Index(i).Get("textContent").String())
		}
		// Turned by hand; modMxSelect's own change is already selected.
		if v := sel.Get("value").String(); v != modMx.sel {
			modMxSelect(v)
		}
		return nil
	})
	modMx.dest = sel

	// DPTH: signed, ±4, as the old per-control knob was. The modulation is
	// depth × the source × the control's range, and real music averages well
	// under full scale, so ±1 could only overdrive on rare peaks.
	pc, ptop := cell("dpth", doc("mod-depth"))
	rng := dom.Doc.Call("createElement", "input")
	rng.Set("type", "range")
	rng.Set("id", "modmx-depth")
	rng.Set("min", "-4")
	rng.Set("max", "4")
	rng.Set("step", "0.01")
	rng.Set("value", "0")
	rng.Set("style", "display:none")
	rng.Set("title", doc("mod-depth"))
	ro := dom.Doc.Call("createElement", "input")
	ro.Set("type", "text")
	ro.Set("inputmode", "decimal")
	ro.Set("className", "numin")
	ro.Set("title", doc("mod-depth.field"))
	ptop.Call("appendChild", ro)
	sizeLEDField(ro, -4, 4, 2, true)
	pc.Call("appendChild", rng)
	pc.Call("appendChild", makeKnob(rng, ro, true, false, true))
	dom.On(rng, "input", func(js.Value, []js.Value) any {
		v, err := strconv.ParseFloat(rng.Get("value").String(), 64)
		if err != nil || modMx.sel == "" {
			return nil
		}
		if v > -0.005 && v < 0.005 {
			v = 0
		}
		m := pmod.params[modMx.sel]
		m.level = float32(v)
		pmod.params[modMx.sel] = m
		ro.Set("value", led.Format(v, 1, 2, true))
		modMxLight(modMxRowOf(modMx.sel))
		syncAudioMod()
		perma.syncPermalinkNow()
		return nil
	})
	dom.On(ro, "change", func(js.Value, []js.Value) any {
		if v, err := led.Parse(ro.Get("value").String()); err == nil {
			rng.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			dom.Fire(rng, "input")
		}
		return nil
	})
	modMx.depth, modMx.depthRO = rng, ro

	// EQ: the selected route's band curve.
	ec, _ := cell("eq", doc("mod-eq"))
	modMx.eqHold = dom.Doc.Call("createElement", "span")
	modMx.eqHold.Set("className", "grp vmbay")
	ec.Call("appendChild", modMx.eqHold)
	return row
}

// modMxRetarget points every row at the running model: the parameters' rows
// at params, in order, and blank past the last of them; DEST's list; and the
// selection, which stays where it was while its row still routes something
// on this model.
func modMxRetarget(params []paramDef) {
	// Decided here, applied in one crossing (fastDOM mxRetarget): row by
	// row and pin by pin from Go it was a thousand crossings on every
	// model change.
	type rowWrite struct {
		I     int      `json:"i"`
		SVG   string   `json:"svg"`
		Title string   `json:"title"`
		Col   string   `json:"col"`
		Pins  []string `json:"pins"`
	}
	var writes []rowWrite
	for i := range modMx.cols {
		c := modMxCol{}
		switch {
		case i >= modMx.nParams:
			v := modMxViewCols[i-modMx.nParams]
			c = modMxCol{v.id, v.label}
		case i < len(params):
			c = modMxCol{params[i].ID, params[i].Label}
		}
		if c == modMx.cols[i] && modMx.rows[i].named {
			continue
		}
		modMx.cols[i] = c
		modMx.rows[i].named = true
		w := rowWrite{I: i, SVG: dotSVG(displayText(c.label), false, dispFullChars), Col: c.id, Title: doc("mod-row.none")}
		if c.id != "" {
			w.Title = docf("mod-row", "control", c.label)
			for j := range modMx.rows[i].pins {
				w.Pins = append(w.Pins, docf("mod-pin", "src", modChannels[j].key, "control", c.label))
			}
		}
		writes = append(writes, w)
	}
	if len(writes) > 0 {
		if b, err := json.Marshal(writes); err == nil {
			fastDOM().Call("mxRetarget", len(modChannels), string(b))
		}
	}
	modMxDest()
	if modMxRowOf(modMx.sel) < 0 {
		modMx.sel = ""
		for _, c := range modMx.cols {
			if m := pmod.params[c.id]; c.id != "" && m.channel != "" && m.level != 0 {
				modMx.sel = c.id
				break
			}
		}
		if modMx.sel == "" {
			for _, c := range modMx.cols {
				if c.id != "" {
					modMx.sel = c.id
					break
				}
			}
		}
	}
	modMxSelect(modMx.sel)
	// The list was rewritten under DEST, so its knob and name catch up even
	// when the selection did not move.
	modMx.dest.Set("value", modMx.sel)
	dom.Fire(modMx.dest, "change")
	for i := range modMx.rows {
		modMxLight(i)
	}
}

// modMxDest refills DEST's list with the rows there are, in one write: it
// is rewritten on every model change.
func modMxDest() {
	var b strings.Builder
	for _, c := range modMx.cols {
		if c.id != "" {
			b.WriteString(`<option value="` + html.EscapeString(c.id) + `">` + html.EscapeString(displayText(c.label)) + `</option>`)
		}
	}
	modMx.dest.Set("innerHTML", b.String())
}

// modMxRowOf is the row routing to id, or -1.
func modMxRowOf(id string) int {
	for i, c := range modMx.cols {
		if id != "" && c.id == id {
			return i
		}
	}
	return -1
}

// modMxSelect makes id the route under the matrix: its row marked, the DEST
// knob on it, its depth on DPTH, and its EQ strip.
func modMxSelect(id string) {
	at := modMxRowOf(id)
	if at < 0 {
		return
	}
	modMx.sel = id
	for i, r := range modMx.rows {
		r.legend.Get("classList").Call("toggle", "mxsel", i == at)
	}
	if modMx.dest.Truthy() && modMx.dest.Get("value").String() != id {
		modMx.dest.Set("value", id)
		dom.Fire(modMx.dest, "change")
	}
	if modMx.depth.Truthy() {
		v := float64(pmod.params[id].level)
		modMx.depth.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		modMx.depthRO.Set("value", led.Format(v, 1, 2, true))
		if f := knobRefresh["modmx-depth"]; f != nil {
			f()
		}
	}
	if modMx.eqHold.Truthy() {
		dom.RebuildInto(&modMx.eqFuncs, func() {
			modMx.eqHold.Set("innerHTML", "")
			modMx.eqHold.Call("appendChild", makeEQStrip(id))
		})
	}
}

// modMxLight lights row i's pins: the one its route comes from, as bright as
// the route is deep.
func modMxLight(i int) {
	if i < 0 || i >= len(modMx.rows) {
		return
	}
	id := modMx.cols[i].id
	m := pmod.params[id]
	r := &modMx.rows[i]
	if len(r.lit) != len(r.pins) {
		r.lit = make([]string, len(r.pins))
		for j := range r.lit {
			r.lit[j] = "?" // never lit: written the first time
		}
	}
	for j, pin := range r.pins {
		on := id != "" && m.level != 0 && m.channel == modChannels[j].name
		op := ""
		if on {
			d := float64(m.level)
			if d < 0 {
				d = -d
			}
			op = strconv.FormatFloat(min(1, 0.45+0.55*d), 'f', 2, 64)
		}
		if r.lit[j] == op {
			continue
		}
		r.lit[j] = op
		pin.Get("classList").Call("toggle", "on", on)
		pin.Get("style").Set("opacity", op)
	}
}
