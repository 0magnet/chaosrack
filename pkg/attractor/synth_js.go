//go:build js && wasm

package attractor

// The Synth bay: the knobs of the synthesizer the keyboard plays
// (keysvoice_js.go), above the keyboard. Six modules — SYNTH, the preset and
// where it goes; then STRING, FM, WAVE · NOISE, FILTER and AMP, the engine's
// parts — and every knob a standard cell, so it has its readout, its reset,
// its place in a link and in Reset All like any other.
//
// A preset is a starting point, not a mode: it sets every knob, and any of
// them turned after it changes the sound from there. The piano is the
// default, so it is what every knob's reset returns to.

import (
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/keysynth"
)

// synth is the engine's settings, written by the knobs.
var synth keysynth.Params

// synthKnob is one value knob on the bay.
type synthKnob struct {
	id, label           string
	min, max, step, def float64
	key, help           string
}

// synthSelect is one selector on the bay.
type synthSelect struct {
	id, label, help, def, key string
	opts                      []synthOpt
	readout                   bool // its name on a display rather than printed round it
	apply                     func(string)
}

// synthOpt is one position of a selector: its value, what is printed or
// displayed for it, and what it is.
type synthOpt struct{ value, text, help string }

// synthModule is one module of the bay, its cells in order.
type synthModule struct {
	title, help string
	cells       []any // synthKnob or synthSelect
}

// synthModules is the bay, left to right: three cells to a column.
var synthModules = []synthModule{
	{"Synth", doc("synth-synth"), []any{
		synthSelect{id: "syn-preset", label: "prst", def: "piano", readout: true,
			help: doc("syn-preset"),
			opts: synthPresetOpts()}, // applied by applySynthPreset, which reads this table
		synthKnob{id: "keys-lvl", label: "lvl", min: 0, max: 100, step: 1, def: 80,
			help: doc("keys-lvl")},
	}},
	{"String", doc("synth-string"), []any{
		synthKnob{id: "syn-str", label: "str", min: 0, max: 1, step: 0.01, def: 1, key: "ys", help: doc("syn-str")},
		synthKnob{id: "syn-part", label: "part", min: 1, max: 16, step: 1, def: 12, key: "yp", help: doc("syn-part")},
		synthKnob{id: "syn-tilt", label: "tilt", min: 0.3, max: 3, step: 0.05, def: 1.15, key: "yt", help: doc("syn-tilt")},
		synthKnob{id: "syn-inhm", label: "inhm", min: 0, max: 20, step: 0.1, def: 2.5, key: "yi", help: doc("syn-inhm")},
		synthKnob{id: "syn-strk", label: "strk", min: 0, max: 0.5, step: 0.005, def: 0.125, key: "yk", help: doc("syn-strk")},
		synthKnob{id: "syn-uni", label: "uni", min: 0, max: 5, step: 0.1, def: 0.6, key: "yu", help: doc("syn-uni")},
		synthKnob{id: "syn-damp", label: "damp", min: 0, max: 2, step: 0.05, def: 0.45, key: "yd", help: doc("syn-damp")},
	}},
	{"FM", doc("synth-fm"), []any{
		synthKnob{id: "syn-fm", label: "fm", min: 0, max: 1, step: 0.01, def: 0, key: "yf", help: doc("syn-fm")},
		synthKnob{id: "syn-rato", label: "rato", min: 0.5, max: 16, step: 0.5, def: 1, key: "yr", help: doc("syn-rato")},
		synthKnob{id: "syn-indx", label: "indx", min: 0, max: 10, step: 0.1, def: 2.6, key: "yx", help: doc("syn-indx")},
		synthKnob{id: "syn-idec", label: "idec", min: 0.01, max: 3, step: 0.01, def: 0.3, key: "ye", help: doc("syn-idec")},
	}},
	{"Wave · Noise", doc("synth-wave"), []any{
		synthSelect{id: "syn-shape", label: "shp", def: "sine", key: "yw",
			help: doc("syn-shape"),
			opts: []synthOpt{
				{"sine", "sine", doc("syn-shape=sine")},
				{"triangle", "tri", doc("syn-shape=triangle")},
				{"square", "sqr", doc("syn-shape=square")},
				{"sawtooth", "saw", doc("syn-shape=sawtooth")},
				{"organ", "org", doc("syn-shape=organ")},
				{"noise", "nse", doc("syn-shape=noise")},
			}, apply: setSynthShape},
		synthKnob{id: "syn-wave", label: "wave", min: 0, max: 1, step: 0.01, def: 0, key: "yv", help: doc("syn-wave")},
		synthKnob{id: "syn-nse", label: "nse", min: 0, max: 1, step: 0.01, def: 0.5, key: "yn", help: doc("syn-nse")},
		synthKnob{id: "syn-tone", label: "tone", min: 1, max: 16, step: 0.5, def: 4, key: "yo", help: doc("syn-tone")},
		synthKnob{id: "syn-ndec", label: "ndec", min: 0.002, max: 0.2, step: 0.001, def: 0.008, key: "yc", help: doc("syn-ndec")},
	}},
	{"Filter", doc("synth-filter"), []any{
		synthKnob{id: "syn-brgt", label: "brgt", min: 1, max: 48, step: 0.5, def: 48, key: "yb", help: doc("syn-brgt")},
		synthKnob{id: "syn-dark", label: "dark", min: 1, max: 48, step: 0.5, def: 48, key: "yg", help: doc("syn-dark")},
		synthKnob{id: "syn-fdec", label: "fdec", min: 0.01, max: 5, step: 0.01, def: 0.5, key: "yh", help: doc("syn-fdec")},
		synthKnob{id: "syn-reso", label: "reso", min: 0.1, max: 12, step: 0.1, def: 0.7, key: "yq", help: doc("syn-reso")},
	}},
	{"Amp", doc("synth-amp"), []any{
		synthKnob{id: "syn-atk", label: "atk", min: 0.001, max: 1, step: 0.001, def: 0.002, key: "ya", help: doc("syn-atk")},
		synthKnob{id: "syn-dec", label: "dec", min: 0.05, max: 10, step: 0.05, def: 3.2, key: "yy", help: doc("syn-dec")},
		synthKnob{id: "syn-ktrk", label: "ktrk", min: 0, max: 2, step: 0.05, def: 1, key: "yj", help: doc("syn-ktrk")},
		synthKnob{id: "syn-sus", label: "sus", min: 0, max: 1, step: 0.01, def: 0, key: "yl", help: doc("syn-sus")},
		synthKnob{id: "syn-rel", label: "rel", min: 0.01, max: 3, step: 0.01, def: 0.25, key: "ym", help: doc("syn-rel")},
	}},
}

// applySynthPreset sets every knob on the bay to preset name: its own values,
// and the piano's for the rest.
func applySynthPreset(name string) {
	p, ok := keysynth.PresetByName(name)
	if !ok {
		return
	}
	for _, m := range synthModules {
		for _, c := range m.cells {
			kn, ok := c.(synthKnob)
			if !ok || synth.Field(kn.id) == nil {
				continue
			}
			v, ok := p.Set[kn.id]
			if !ok {
				v = kn.def
			}
			setParamSlider(kn.id, v)
		}
	}
	setSelect("syn-shape", p.Shape)
}

// synthPresetOpts is the preset selector's positions.
func synthPresetOpts() []synthOpt {
	out := make([]synthOpt, len(keysynth.Presets))
	for i, p := range keysynth.Presets {
		out[i] = synthOpt{p.Name, p.Text, doc("syn-preset=" + p.Name)}
	}
	return out
}

// setSynthShape is the wave shape knob: the next note's oscillator, and a
// held plain waveform's at once.
func setSynthShape(v string) {
	synth.Shape = v
	switch v {
	case "sine", "triangle", "square", "sawtooth":
		for _, kv := range keys.voices {
			if kv.osc.Truthy() {
				kv.osc.Set("type", v)
			}
		}
	}
}

// buildSynthBay builds the bay's modules before the keyboard's and wires
// their knobs.
func buildSynthBay() {
	keysMod := dom.Doc.Call("getElementById", "keys-module")
	if !keysMod.Truthy() {
		return
	}
	for _, m := range synthModules {
		mod, row := newCellModule("synth-"+strings.ToLower(strings.Fields(m.title)[0]), m.title, m.help)
		keysMod.Get("parentNode").Call("insertBefore", mod, keysMod)
		for _, c := range m.cells {
			switch c := c.(type) {
			case synthKnob:
				synthKnobCell(row, c)
			case synthSelect:
				synthSelectCell(row, c)
			}
		}
	}
}

// newCellModule is an empty module of standard cells, titled, and the row its
// cells go in.
func newCellModule(id, title, help string) (mod, row js.Value) {
	mod = dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect gen-osc")
	mod.Set("id", id)
	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	h.Set("title", help)
	mod.Call("appendChild", h)
	row = dom.Doc.Call("createElement", "div")
	row.Set("className", "row vmrow")
	mod.Call("appendChild", row)
	return mod, row
}

// synthCell is a standard cell with label, holding place for a knob, and
// reset, in row.
func synthCell(row js.Value, id, label, help string) (cell, top, stack js.Value) {
	cell = dom.Doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell gen-cell")
	cell.Set("title", help)
	top = dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	l := dom.Doc.Call("createElement", "span")
	l.Set("className", "plabel")
	l.Set("textContent", label)
	top.Call("appendChild", l)
	cell.Call("appendChild", top)
	bay := dom.Doc.Call("createElement", "span")
	bay.Set("className", "grp vmbay")
	stack = dom.Doc.Call("createElement", "span")
	bay.Call("appendChild", stack)
	cell.Call("appendChild", bay)
	rst := dom.Doc.Call("createElement", "button")
	rst.Set("className", "rst")
	rst.Set("id", "rst-"+id)
	rst.Set("title", docf("reset", "label", label))
	rst.Set("textContent", "↺")
	cell.Call("appendChild", rst)
	row.Call("appendChild", cell)
	return cell, top, stack
}

// synthKnobCell is a value knob's cell: readout, knob with its scale, reset.
func synthKnobCell(row js.Value, k synthKnob) {
	cell, top, stack := synthCell(row, k.id, k.label, k.help)
	led := dom.Doc.Call("createElement", "input")
	led.Set("type", "number")
	led.Set("className", "numin")
	led.Set("id", k.id+"-led")
	led.Set("title", k.help)
	top.Call("appendChild", led)
	sl := dom.Doc.Call("createElement", "input")
	sl.Set("type", "range")
	sl.Set("id", k.id)
	for _, a := range [][2]any{{"min", k.min}, {"max", k.max}, {"step", k.step}, {"value", k.def}} {
		sl.Call("setAttribute", a[0], a[1])
	}
	sl.Set("title", k.help)
	sl.Get("style").Set("display", "none")
	cell.Call("insertBefore", sl, top.Get("nextSibling"))
	stack.Call("appendChild", makeKnob(sl, led, true, false, true))
	apply := func(v float64) { keys.updateRouting() } // LVL
	if p := synth.Field(k.id); p != nil {
		*p = k.def
		apply = func(v float64) { *p = v }
	}
	adoptDescControl(ControlDesc{
		ID: k.id, Label: k.label, Min: k.min, Max: k.max, Step: k.step, Def: k.def,
		PermaKey: k.key, LEDID: k.id + "-led", ResetID: "rst-" + k.id, Apply: apply,
	})
}

// synthSelectCell is a selector's cell: a ring of printed positions, or a
// knob with the setting's name on a display.
func synthSelectCell(row js.Value, s synthSelect) {
	cell, _, stack := synthCell(row, s.id, s.label, s.help)
	sel := dom.Doc.Call("createElement", "select")
	sel.Set("id", s.id)
	sel.Set("title", s.help)
	sel.Get("style").Set("display", "none")
	var texts []string
	for _, o := range s.opts {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", o.value)
		opt.Set("textContent", o.text)
		opt.Set("title", o.help)
		sel.Call("appendChild", opt)
		texts = append(texts, o.text)
	}
	sel.Set("value", s.def)
	cell.Call("appendChild", sel)
	if s.readout {
		stack.Call("appendChild", selectorKnobReadout(sel))
	} else {
		stack.Call("appendChild", singleSelectorKnob(sel, texts))
	}
	if s.id == "syn-shape" {
		synth.Shape = s.def
	}
	apply := s.apply
	if s.id == "syn-preset" {
		apply = applySynthPreset
	}
	adoptDescControl(ControlDesc{
		ID: s.id, Label: s.label, IsSelect: true, SelectDef: s.def, PermaKey: s.key,
		ResetID: "rst-" + s.id, SelectApply: apply,
	})
}
