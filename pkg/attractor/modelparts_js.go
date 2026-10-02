//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// ── A model's own parts, in the Visual head and bank ────────────────────
//
// Seven models used to bring a module of their own: Pong's Scoreboard, the
// Banner, the Patch readout, the Launcher, the Loader, the Animation picker
// and the Desk. Only one model runs at a time, so at most one of them ever
// had anything to say, and together they took a bay and a half. What they
// held is now in the places every model already has:
//
//   - a knob or a selector is a BANK position, beside the model's constants:
//     Pong's paddle pots and the Launcher's height (parameters, paramdefs_js),
//     the desk's style, the animation and the built-in solid (the cells in
//     the #model-parts holder, marked data-bank-of);
//   - a readout or the banner's text is on the line over the equation under
//     the monitor (.mro, marked data-for);
//   - a switch or a button is one of the head's PROGRAMMABLE SWITCHES, next to
//     Screen, whose dot-matrix legends name what each does for the model on
//     the MODEL knob — as the bank's displays name its knobs.

// modelSwitchCount is how many programmable switches the head has: as many
// as the model with the most (Custom's iterate, surface and 4D).
const modelSwitchCount = 3

// legendChars is a switch legend's size: the Visual head's switches, a half
// display each, so every legend is four characters: "scrn", "lstn".
const legendChars = dispHalfChars

// modelSwitchChars is how many characters each switch's legend shows.
const modelSwitchChars = legendChars

// modelSwitch is what one programmable switch does for a model.
type modelSwitch struct {
	legend, tip string
	on          func() bool // where it stands; nil for a momentary one
	set         func(bool)  // a latching switch thrown
	press       func()      // a momentary switch pressed
	live        func() bool // whether it applies now; nil for always
}

// clickPart presses one of the holder's hidden buttons, which keep the
// listeners those models wired to them.
func clickPart(id string) func() {
	return func() {
		if b := dom.Doc.Call("getElementById", id); b.Truthy() {
			b.Call("click")
		}
	}
}

// modelSwitchesFor is what the head's switches do for mode, in order.
func modelSwitchesFor(mode string) []modelSwitch {
	switch mode {
	case "custom":
		c := &custom
		return []modelSwitch{
			{legend: "iter", tip: doc("sw.custom.iter"),
				on: func() bool { return c.iterate },
				set: func(v bool) {
					c.setFlavor(func() {
						c.iterate = v
						if v {
							c.surface = false // one flavor at a time
						}
					})
				}},
			{legend: "surf", tip: doc("sw.custom.surf"),
				on: func() bool { return c.surface },
				set: func(v bool) {
					c.setFlavor(func() {
						c.surface = v
						if v {
							c.iterate = false
						}
					})
				}},
			// A map has no hidden 4th state here: the 3-D map machinery cannot
			// carry one, and the Lyapunov estimator runs two copies of the step
			// side by side, which a package-var w would have them share. The
			// typed dw/dt is kept, just not compiled, so flipping back restores it.
			{legend: "4d w", tip: doc("sw.custom.4d"),
				on:   func() bool { return c.useW },
				set:  func(v bool) { c.setFlavor(func() { c.useW = v }) },
				live: func() bool { return !c.iterate && !c.surface }},
		}
	case "desk":
		return []modelSwitch{{legend: "pass",
			tip: doc("sw.desk.pass"),
			on:  func() bool { return deskPassOn },
			set: func(v bool) {
				if cb := dom.Doc.Call("getElementById", "desk-pass"); cb.Truthy() {
					cb.Set("checked", v)
					cb.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				}
			}}}
	case "pong":
		return []modelSwitch{{legend: "rset", tip: doc("sw.pong.rset"), press: clickPart("pong-restart")}}
	case "bounceball":
		return []modelSwitch{{legend: "drop", tip: doc("sw.bounceball.drop"), press: clickPart("bounce-drop")}}
	case "fvf":
		return fvf.fvfSwitches()
	case "recurrence":
		return []modelSwitch{{legend: "trnd", tip: rqaTrendTip,
			on:  func() bool { return rqaTrendOn },
			set: func(v bool) { rqaTrendOn = v; rqa.paint() }}}
	case "stlfile":
		return []modelSwitch{{legend: "load", tip: doc("sw.stlfile.load"), press: clickPart("stlfile-load")}}
	}
	return nil
}

// setFlavor changes Custom's flavor or its 4D switch and rebuilds what
// depends on it. Reparsed before the rebuild: the panel's mode-scoped syncs
// run ahead of buildCustomPanel, and IsMap("custom") has to be true by the
// time syncMapExtras asks — otherwise a switch to iterate poses the (plane)
// figure face-on one rebuild late.
func (c *customEquation) setFlavor(apply func()) {
	apply()
	c.parseCustom()
	resetAttractorState()
	buildParamPanel("custom")
	perma.syncPermalinkNow()
}

// modelSwitchID is the i-th programmable switch.
func modelSwitchID(i int) string { return "model-sw-" + strconv.Itoa(i) }

// buildModelSwitches is the head's programmable switches, for the row
// under the monitor beside Screen: each a switch with a dot-matrix legend
// over it, programmed by syncModelParts.
func buildModelSwitches(row js.Value) {
	for i := range modelSwitchCount {
		lab := dom.Doc.Call("createElement", "label")
		lab.Set("className", "modsw")
		lab.Call("setAttribute", "data-no-drag", "")
		lab.Call("appendChild", dotDisplayN("", false, modelSwitchChars))
		sw := dom.Doc.Call("createElement", "input")
		sw.Set("type", "checkbox")
		sw.Set("className", "sw")
		sw.Set("id", modelSwitchID(i))
		sw.Set("disabled", true)
		n := i
		sw.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
			ss := modelSwitchesFor(editMode())
			if n >= len(ss) {
				sw.Set("checked", false)
				return nil
			}
			s := ss[n]
			if s.press != nil {
				// Momentary: pressed while the switch is thrown — inside the
				// click, so a file picker may open — and it springs back.
				if sw.Get("checked").Bool() {
					s.press()
					js.Global().Call("setTimeout", dom.FuncOf(func(js.Value, []js.Value) any {
						sw.Set("checked", false)
						return nil
					}), 180)
				}
				return nil
			}
			s.set(sw.Get("checked").Bool())
			syncModelParts(editMode())
			return nil
		}))
		lab.Call("appendChild", sw)
		row.Call("appendChild", lab)
	}
}

// modelSelectorCells are mode's bank positions from the holder: selectors
// filled at run time, which have no parameter behind them.
func modelSelectorCells(mode string) []js.Value {
	var out []js.Value
	cs := dom.Doc.Call("querySelectorAll", `#model-parts [data-bank-of="`+mode+`"]`)
	for i := range cs.Length() {
		c := cs.Index(i)
		c.Call("setAttribute", "data-mode", mode)
		out = append(out, c)
	}
	return out
}

// mountModelReadouts puts the holder's readouts on a line of their own at the
// top of the place under the monitor, over the equation.
func mountModelReadouts(slot js.Value) {
	line := dom.Doc.Call("createElement", "div")
	line.Set("className", "mroline")
	line.Set("id", "model-readouts")
	rs := dom.Doc.Call("querySelectorAll", "#model-parts .mro")
	for i := range rs.Length() {
		line.Call("appendChild", rs.Index(i))
	}
	// Every readout here is a character display: the markup declares each
	// one's size, and one its model has not written yet shows what the
	// markup says it starts at.
	ds := line.Call("querySelectorAll", ".mrodmd:not(.dmdwin)")
	for i := range ds.Length() {
		d := ds.Index(i)
		setDotText(d, d.Get("textContent").String())
	}
	slot.Call("appendChild", line)
}

// liveReadout puts a measured readout on host: its label and a character
// display of n characters (full or half), side by side like the Lyapunov
// readout's, so every model's line is the same height whatever it measures.
// It returns the display, which setDotText writes. A host that is not the
// readout line (the Parameters grid) gets the label and display as a card.
func liveReadout(host js.Value, label string, n int, text, title string) js.Value {
	win := dotDisplayN(text, false, n)
	win.Get("classList").Call("add", "mrodmd")
	win.Set("title", title)
	if !host.Get("classList").Call("contains", "mro").Bool() {
		card, top := newPunitCard(label)
		top.Call("appendChild", win)
		host.Call("appendChild", card)
		return win
	}
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "mro-lbl")
	lbl.Set("textContent", label)
	host.Call("appendChild", lbl)
	host.Call("appendChild", win)
	return win
}

// syncModelParts programs the head's switches and shows the readouts for
// mode: the model the panel is on.
func syncModelParts(mode string) {
	if !dom.Doc.Truthy() {
		return
	}
	ss := modelSwitchesFor(mode)
	for i := range modelSwitchCount {
		sw := dom.Doc.Call("getElementById", modelSwitchID(i))
		if !sw.Truthy() {
			continue
		}
		lab := sw.Call("closest", "label")
		win := lab.Call("querySelector", ".dmdwin")
		if i >= len(ss) {
			sw.Set("disabled", true)
			sw.Set("checked", false)
			setDotText(win, "")
			lab.Get("classList").Call("add", "modsw-off")
			lab.Set("title", docf("sw.unassigned", "mode", modeLabel(mode)))
			continue
		}
		s := ss[i]
		live := s.live == nil || s.live()
		sw.Set("disabled", !live)
		sw.Set("checked", s.on != nil && live && s.on())
		setDotText(win, s.legend)
		lab.Get("classList").Call("toggle", "modsw-off", !live)
		lab.Set("title", s.tip)
	}
	rs := dom.Doc.Call("querySelectorAll", "#model-readouts .mro")
	shown := false
	for i := range rs.Length() {
		r := rs.Index(i)
		// "*" is a readout of whatever model is RUNNING (the Lyapunov
		// exponent): not while the panel is on the backdrop.
		who := r.Call("getAttribute", "data-for").String()
		on := who == mode || (who == "*" && !back.editing)
		shown = shown || on
		r.Get("style").Set("display", map[bool]string{true: "", false: "none"}[on])
	}
	if line := dom.Doc.Call("getElementById", "model-readouts"); line.Truthy() {
		line.Get("style").Set("display", map[bool]string{true: "", false: "none"}[shown])
	}
}

// liveReadoutHost is where the panel build puts mode's measured readouts —
// the correlation, the delay, the rt60 — built fresh with each panel: a place
// on the line over the equation, the previous build's taken away. Where the
// row has no such line, the Parameters grid, as before.
func liveReadoutHost(mode string, fallback js.Value) js.Value {
	line := dom.Doc.Call("getElementById", "model-readouts")
	if !line.Truthy() {
		return fallback
	}
	olds := line.Call("querySelectorAll", ".mro-live")
	for i := range olds.Length() {
		olds.Index(i).Call("remove")
	}
	host := dom.Doc.Call("createElement", "span")
	host.Set("className", "mro mro-live")
	host.Call("setAttribute", "data-for", mode)
	line.Call("appendChild", host)
	line.Get("style").Set("display", "")
	return host
}
