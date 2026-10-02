//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// turnSpecs is every knob that turns without end, by its slider's id: what
// its laps are (turns.go). A knob not in it has stops.
var turnSpecs = map[string]turnSpec{}

// knobRefresh redraws a knob's pointer and ring from its slider, by the
// slider's id: for when what the ring means changes under a knob that has
// not moved.
var knobRefresh = map[string]func(){}

// makeTurning makes a bank position's knob an endless one, when its
// parameter has laps: the spec registered, the slider under it widened so it
// does not clamp at the old top, and the dial redrawn as a full ring of one
// LED per detent.
//
// At the bank, not where the knob is made: a bank position is an encoder,
// and only an encoder can turn round and round. The same parameter on a knob
// with stops keeps its stops.
func makeTurning(c js.Value) {
	if c.Call("querySelector", "select").Truthy() || c.Call("querySelector", "input[type=checkbox]").Truthy() {
		return // a setting: its detents are its values
	}
	slider := c.Call("querySelector", "input[type=range][id]")
	if !slider.Truthy() {
		return
	}
	id := slider.Get("id").String()
	num := func(attr string) float64 {
		f, _ := strconv.ParseFloat(slider.Get(attr).String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the fallback
		return f
	}
	lo, hi, step := num("min"), num("max"), num("step")
	if s, ok := turnSpecs[id]; ok {
		lo, hi = s.lo, s.hi // made once already: the slider is the widened one
	}
	label := ""
	if l := c.Call("querySelector", ".u-lbl"); l.Truthy() {
		label = l.Get("textContent").String()
	}
	kind := paramTurns(id, label, lo, hi, step, false)
	if kind == turnBounded {
		return
	}
	spec := turnSpec{kind, lo, hi}
	turnSpecs[id] = spec
	dlo, dhi := spec.domain()
	slider.Set("min", strconv.FormatFloat(dlo, 'g', -1, 64))
	slider.Set("max", strconv.FormatFloat(dhi, 'g', -1, 64))
	if dial := c.Call("querySelector", ".value-dial"); dial.Truthy() {
		fullRing(dial, spec)
	}
	if f := knobRefresh[id]; f != nil {
		f()
	}
}

// fullRing redraws a value dial as an endless knob's: one LED per detent all
// knob has no ends. The first LED, at the top, is where every lap starts;
// the color the ring lights in is which lap (paintRing).
// and its color is which lap (panel.css, "LAPS").
func fullRing(dial js.Value, spec turnSpec) {
	dial.Set("innerHTML", "")
	dial.Get("classList").Call("add", "full-ring")
	for i := range ringLEDs {
		deg := 360 * float64(i) / ringLEDs
		l, tp := dialLabelPos(deg, 41)
		tk := dom.Doc.Call("createElement", "span")
		cls := "vdial-tick"
		if i%(ringLEDs/4) == 0 {
			cls += " major"
		}
		tk.Set("className", cls)
		st := tk.Get("style")
		st.Set("left", l)
		st.Set("top", tp)
		st.Set("transform", "translate(-50%,-50%) rotate("+strconv.FormatFloat(deg, 'f', 1, 64)+"deg)")
		dial.Call("appendChild", tk)
	}
	what := map[turnKind]string{
		turnLinear: "one turn is " + fmtDialNum(spec.lo) + " to " + fmtDialNum(spec.hi) + ", and each turn past it another range's worth",
		turnLog:    "one turn is " + fmtDialNum(spec.lo) + " to " + fmtDialNum(spec.hi) + ", and each turn past it ten times the one before",
		turnWrap:   "one turn is " + fmtDialNum(spec.lo) + " to " + fmtDialNum(spec.hi) + ", and it comes round again",
	}[spec.kind]
	dial.Set("title", docf("knob-endless", "what", what))
}

// paintRing lights an endless knob's ring for v (turnSpec.paint): this lap's
// LEDs in this lap's color and the rest still in the last lap's. The colors
// are the rack's LED colors, from the one the Style knob chose, in the
// order that knob lists them (ledColorDefs).
//
// As custom properties on the dial, which its LEDs inherit (panel.css,
// "LAPS"), written only when the lap changes: turning within a lap is the
// two lists and nothing else.
func paintRing(dial js.Value, spec turnSpec, v float64) {
	p := spec.paint(v)
	setLEDList(dial, "data-lit", p.from, p.to)
	setLEDList(dial, "data-under", p.uFrom, p.uTo)
	base := ledColorIndex()
	key := strconv.Itoa(p.lap) + ":" + strconv.Itoa(base)
	if dial.Call("getAttribute", "data-lap").String() == key {
		return
	}
	dial.Call("setAttribute", "data-lap", key)
	st := dial.Get("style")
	c, u := ledColorDefs[lapColor(base, p.lap, len(ledColorDefs))], ledColorDefs[lapColor(base, p.uLap, len(ledColorDefs))]
	st.Call("setProperty", "--lap-col", c.col)
	st.Call("setProperty", "--lap-glow", c.glow)
	st.Call("setProperty", "--under-col", u.col)
	st.Call("setProperty", "--under-glow", u.glow)
}

// ledColorIndex is where the Style knob's LED color is in ledColorDefs.
func ledColorIndex() int {
	if sel := dom.Doc.Call("getElementById", "led-color"); sel.Truthy() {
		name := sel.Get("value").String()
		for i, d := range ledColorDefs {
			if d.name == name {
				return i
			}
		}
	}
	return 0
}

// repaintTurning redraws every endless knob's ring: for when the LED color
// they count their laps from has changed.
func repaintTurning() {
	for id := range turnSpecs {
		if f := knobRefresh[id]; f != nil {
			f()
		}
	}
}
