//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/encoder"
	"github.com/0magnet/chaosrack/pkg/skirt"
	"math"
	"strconv"
	"syscall/js"
)

// Bounded control knobs for the parameter/camera sliders — the tactile
// equivalent of a panel potentiometer (limited travel with end stops),
// distinct from the endless rotation knobs. Each knob DRIVES an existing
// <input type=range>: dragging turns the pointer over a ~270° sweep mapped
// to [min,max], writes slider.value, and dispatches 'input' so the slider's
// own handler does the real work. The knob also tracks the slider's 'input'
// event, so numeric-box / reset / permalink changes move the pointer too.
//
// An optional nested inner disc gives fine trim (lower drag sensitivity), the
// way scope/lab gear stacks a fine knob inside the coarse one.

// Shared drag state (only one knob turns at a time). Document-level move/up
// listeners (initKnobDrag) drive whichever knob grabbed last.
// kb is the active knob-drag gesture — one grouped singleton instead of ten
// loose globals (the grouping pattern for per-feature state clusters).
var kb struct {
	slider   js.Value
	knobEl   js.Value // the knob element being turned (for the grab highlight)
	min      float64
	max      float64
	fine     bool
	sweep    float64 // degrees the knob turns across its range (knobSweep)
	cx, cy   float64
	prevAng  float64
	active   bool
	dragInit bool

	// A bank position is an encoder, not a potentiometer: turning it is
	// detents, each one step of whatever it is programmed to, accelerated
	// by how fast they come (pkg/encoder). Every other knob maps its sweep
	// onto its range, as a pot does.
	encoder bool
	enc     encoder.Encoder
	step    float64 // the coarse step one detent moves an encoder knob
}

// fineRatio is the fine disc's step and drag sensitivity as a fraction of
// the knob's coarse step: fine moves in tenths. It used to be a rack-wide
// setting (Display's Fine ×, with a Step × beside it that scaled every coarse
// step); a bank position has a step of its own, which that fought with.
const fineRatio = 0.1

// switchPots are the knobs that are switch-pots (switchpot.go), by slider id.
var switchPots = map[string]switchPot{}

// oneTurn are the knobs that sweep a whole turn rather than the standard
// 270°: the View module's position knobs, which sit beside the angle knobs
// and wear the same degree dial. One turn, not endless: the middle is the
// default, and either end is half a turn away, at the bottom.
var oneTurn = map[string]bool{"pan-x": true, "pan-y": true, "camera-zoom": true}

// knobSweep is how many degrees the knob for slider id turns across its range.
func knobSweep(id string) float64 {
	if oneTurn[id] {
		return 360
	}
	return skirt.SweepDeg
}

func knobAngleForValue(v, lo, hi, sweep float64) float64 {
	if hi <= lo {
		return 0
	}
	t := (v - lo) / (hi - lo)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return -sweep/2 + sweep*t
}

// initKnobDrag wires the one-time document listeners that turn the active
// bounded knob. Called from Run after the DOM exists.
func initKnobDrag() {
	if kb.dragInit {
		return
	}
	kb.dragInit = true
	onPointerMove(func(e js.Value) {
		if !kb.active {
			return
		}
		cur := math.Atan2(e.Get("clientY").Float()-kb.cy, e.Get("clientX").Float()-kb.cx)
		d := cur - kb.prevAng
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		kb.prevAng = cur
		v, _ := strconv.ParseFloat(kb.slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		scale := 1.0
		if kb.fine {
			scale = fineRatio
		}
		if kb.encoder {
			n := kb.enc.Turn(d, js.Global().Get("performance").Call("now").Float())
			if n == 0 {
				return
			}
			v += float64(n) * kb.step * scale
		} else {
			v += (d / (kb.sweep * math.Pi / 180)) * (kb.max - kb.min) * scale
		}
		// An encoder with laps goes where its laps go (turns.go); anything
		// else stops at the ends of its range.
		if spec, endless := turnSpecs[kb.slider.Get("id").String()]; endless && kb.encoder {
			v = spec.clamp(v)
		} else {
			v = math.Max(kb.min, math.Min(kb.max, v))
		}
		kb.slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		dom.Fire(kb.slider, "input")
	})
	release := dom.FuncOf(func(this js.Value, args []js.Value) any {
		// A switch-pot let go in the gap comes to rest where its pointer
		// already shows it: in the detent or at the bottom of the range.
		if kb.active && kb.slider.Truthy() {
			if sp, ok := switchPots[kb.slider.Get("id").String()]; ok {
				v, _ := strconv.ParseFloat(kb.slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
				kb.slider.Set("value", strconv.FormatFloat(sp.snap(v), 'g', -1, 64))
			}
		}
		kb.active = false
		if kb.knobEl.Truthy() {
			kb.knobEl.Get("classList").Call("remove", "knob-grab")
		}
		return nil
	})
	dom.Doc.Call("addEventListener", "pointerup", release)
	dom.Doc.Call("addEventListener", "pointercancel", release)
}

// knobSyncers holds pointer-refresh closures for the persistent (fixed)
// knobs, so syncKnobs can move them after a programmatic value change
// (Reset All, permalink restore) that doesn't fire 'input'. Param-panel
// knobs are rebuilt fresh each panel build and so aren't registered.
var knobSyncers []func()

func syncKnobs() {
	for _, f := range knobSyncers {
		f()
	}
}

// selectorKnob is the selector-knob drag in progress.
type selectorKnob struct {
	// ── Multi-position selector knobs (rotary encoder over a <select>) ───────────
	// Shared drag state + one-time document listeners, so per-parameter selector
	// knobs rebuilt with the panel don't accumulate listeners.
	active            bool
	sel, knob         js.Value
	cx, cy, prev, acc float64
	dragInit          bool
	// parked: the drag landed on no position (a merged bay's model ring at
	// OFF) and turns nothing more until it is let go.
	parked bool
}

var selk selectorKnob

func (s *selectorKnob) step(dir int) {
	if s.sel.Truthy() {
		selStep(s.sel, dir, true)
	}
}

// initSelKnobDrag wires the one-time document move/up listeners that turn the
// active selector knob. Recomputes the center each move and resyncs on a
// panel reflow so one detent = one step. Called once from Run.
func (s *selectorKnob) initSelKnobDrag() {
	if s.dragInit {
		return
	}
	s.dragInit = true
	onPointerMove(func(e js.Value) {
		if !s.active || s.parked {
			return
		}
		r := s.knob.Call("getBoundingClientRect")
		cx := r.Get("left").Float() + r.Get("width").Float()/2
		cy := r.Get("top").Float() + r.Get("height").Float()/2
		cur := math.Atan2(e.Get("clientY").Float()-cy, e.Get("clientX").Float()-cx)
		if math.Abs(cx-s.cx) > 1 || math.Abs(cy-s.cy) > 1 {
			s.cx, s.cy, s.prev = cx, cy, cur
			return
		}
		d := cur - s.prev
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		s.prev = cur
		dDeg := d * 180 / math.Pi
		// The pointer isn't turned freely here — it snaps to the selected slot
		// via the select's 'change' handler each time a detent steps it.
		// One full turn = one pass through the options: detent = 360°/N.
		detent := 24.0
		if s.sel.Truthy() {
			if n := s.sel.Get("options").Get("length").Int(); n > 0 {
				detent = 360.0 / float64(n)
			}
		}
		// No position — a merged bay's model ring at OFF, between its last
		// model and its first — is a stop: a drag that lands on it stays there
		// until it is let go, as a switch-pot's OFF holds the hand, so OFF can
		// be reached by turning and not only passed through.
		land := func() bool {
			if s.sel.Get("selectedIndex").Int() < 0 {
				s.parked, s.acc = true, 0
			}
			return s.parked
		}
		for s.acc += dDeg; s.acc >= detent; s.acc -= detent {
			s.step(1)
			if land() {
				return
			}
		}
		for ; s.acc <= -detent; s.acc += detent {
			s.step(-1)
			if land() {
				return
			}
		}
	})
	rel := dom.FuncOf(func(this js.Value, args []js.Value) any {
		s.active, s.parked = false, false
		return nil
	})
	dom.Doc.Call("addEventListener", "pointerup", rel)
	dom.Doc.Call("addEventListener", "pointercancel", rel)
}

// makeSelectorKnob builds a rotary-encoder knob that steps sel's options,
// like the model selector. Returns the knob element to place before sel.
func (s *selectorKnob) makeSelectorKnob(sel js.Value) js.Value {
	knob := dom.Doc.Call("createElement", "span")
	knob.Set("className", "knob knobsel")
	knob.Call("setAttribute", "data-no-drag", "")
	// Name the knob from the select it drives (single source: set the title on
	// the <select> once and every knob/label built from it inherits it), so each
	// selector knob identifies its own control instead of a generic hint.
	// With no title on the select, none on the knob either: the cell it sits
	// in says what it is (the scope's VOLTS/DIV, say), and a generic "turn to
	// change selection" on the knob hid that sentence under the hand.
	if t := sel.Get("title").String(); t != "" {
		knob.Set("title", t+" — turn to select")
	}
	ptr := dom.Doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)
	// The pointer snaps to the selected option's slot (270° spread over the
	// options) — it only ever points at a position, never in between, or with
	// none selected at the bottom, where OFF is.
	// Driven off the select's 'change', so drag detents, the wheel, the
	// dropdown, and permalink restores all move it.
	snap := func() {
		n := sel.Get("options").Get("length").Int()
		idx := sel.Get("selectedIndex").Int()
		ang := 0.0
		switch {
		case idx < 0:
			// On no position: a merged bay's model ring with the bay off. It
			// points at the bottom, the gap between its last position and its
			// first, which is where OFF is on the category ring round it: the two
			// rings say OFF together.
			ang = 180
		case selEndless(sel) && n > 0:
			ang = endlessDeg(idx, n)
		case n > 1:
			ang = -skirt.SweepDeg/2 + skirt.SweepDeg*float64(idx)/float64(n-1)
		}
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(ang, 'f', 1, 64)+"deg)")
	}
	// Keyed: a selector has one knob at a time, and one built to replace
	// another (a bank remounting a family's dial) replaces its listener too.
	dom.OnAs(sel, "change", "selknob", func(this js.Value, args []js.Value) any {
		snap()
		return nil
	})
	snap()
	dom.On(knob, "pointerdown", func(this js.Value, args []js.Value) any {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		r := knob.Call("getBoundingClientRect")
		s.cx = r.Get("left").Float() + r.Get("width").Float()/2
		s.cy = r.Get("top").Float() + r.Get("height").Float()/2
		s.prev = math.Atan2(e.Get("clientY").Float()-s.cy, e.Get("clientX").Float()-s.cx)
		s.sel, s.knob = sel, knob
		s.acc = 0
		s.active = true
		return nil
	})
	// One step of the selection, shared by the wheel and the arrow keys, for
	// the same reason the value knobs share theirs.
	step := func(up bool) {
		dir := 1
		if up {
			dir = -1
		}
		selStep(sel, dir, false)
	}
	// Scroll wheel over the knob steps the selection (like scrolling the
	// select itself), firing change so the bound handler reacts.
	dom.On(knob, "wheel", func(this js.Value, args []js.Value) any {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		step(e.Get("deltaY").Float() < 0)
		return nil
	})
	// Up steps the same way scrolling up does — towards the earlier option —
	// so the two agree about which direction "up" is on a detented ring.
	registerKnobHover(knob, step)
	return knob
}

// stackKnobs nests inner concentrically inside a larger outer ring, the way
// scope / lab gear stacks two controls on one shaft. outer is a plain knob
// element (a .knob, e.g. from makeSelectorKnob); inner is a full knob widget
// (a .knobwrap from makeKnob, or another .knob). The inner sits centered on
// top with a higher z-index, so its own drag/wheel handlers (which
// stopPropagation) win within its footprint; the outer ring's exposed annulus
// turns the outer control. Returns the stack element to place in the panel.
func stackKnobs(outer, inner js.Value) js.Value {
	stack := dom.Doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	outer.Get("classList").Call("add", "knob-ring")
	inner.Get("classList").Call("add", "knob-inner")
	stack.Call("appendChild", outer)
	stack.Call("appendChild", inner)
	return stack
}

// soloKnob is stackKnobs' single-knob form: one selector knob in a knobstack,
// for a cell that holds one ring rather than two concentric ones.
func soloKnob(sel js.Value) js.Value {
	stack := dom.Doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	k := selk.makeSelectorKnob(sel)
	k.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", k)
	return stack
}

// setLabelTooltips sets a per-label title on a selector knob's dial labels,
// matched by the label text, so rotary-switch positions get unique tooltips.
func setLabelTooltips(stack js.Value, tips map[string]string) {
	labs := stack.Call("querySelectorAll", ".knob-dial-lab")
	for i := range labs.Get("length").Int() {
		l := labs.Index(i)
		if t, ok := tips[l.Get("textContent").String()]; ok {
			l.Set("title", t)
		}
	}
}

// singleSelectorKnob builds a lone rotary-switch knob (not concentric) wrapped
// in a stack so it gets a label ring, for standalone selectors like Size / Knob
// style. Returns the stack element.
func singleSelectorKnob(sel js.Value, labels []string) js.Value {
	stack := dom.Doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := selk.makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)
	addSelectorLabels(stack, labels, sel)
	return stack
}

// selectorKnobReadout builds a lone rotary-switch knob with a character display
// of the current option beneath it, for selectors that have too many options or
// too-long labels for a ring of labels around the dial (e.g. Phosphor). Returns
// a wrapper element to place in the panel.
func selectorKnobReadout(sel js.Value) js.Value {
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "selk-ro")
	stack := dom.Doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := selk.makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)
	readout := dotDisplayN("", false, optionChars(sel))
	readout.Get("classList").Call("add", "selk-readout")
	set := func() {
		idx := max(sel.Get("selectedIndex").Int(), 0)
		setDotText(readout, displayText(sel.Get("options").Index(idx).Get("text").String()))
		// The readout describes what it is CURRENTLY showing. Without a title of
		// its own it showed the cell's, which is a paragraph about the knob —
		// the same paragraph whatever the readout said, and available from
		// anywhere else in the cell anyway.
		dialPosTitle(readout, sel, idx)
	}
	dom.On(sel, "change", func(this js.Value, a []js.Value) any { set(); return nil })
	set()
	wrap.Call("appendChild", stack)
	wrap.Call("appendChild", readout)
	return wrap
}

// dialLabelPos returns left/top CSS percentages for a dial label at pointer
// angle deg (clockwise from straight-up) and radial offset offPct (percent of
// the dial's half-size from center).
func dialLabelPos(deg, offPct float64) (string, string) {
	r := deg * math.Pi / 180
	x := 50 + offPct*math.Sin(r)
	y := 50 - offPct*math.Cos(r)
	return strconv.FormatFloat(x, 'f', 1, 64) + "%", strconv.FormatFloat(y, 'f', 1, 64) + "%"
}

// angleDialLabelOff is where addAngleDial's numbers sit, in dialLabelPos
// percent: outside the ticks, which hug the knob (.axknob-box, panel.css).
const angleDialLabelOff = 45

// addAngleDial draws an analog degree dial around a stack's outer ring: tick
// marks every 30° with 0/90/180/270 labels, like a clock but in degrees.
// Decorative (pointer-events:none) and behind the knob, so only the part
// outside the ring shows. The numbers stand outside the ticks and face the
// knob, foot inward, which is what lets the side ones fit the cell: on end,
// "270" is one line of type wide.
func addAngleDial(stack js.Value) {
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	ticks := dom.Doc.Call("createElement", "span")
	ticks.Set("className", "angle-dial-ticks")
	dial.Call("appendChild", ticks)
	for _, d := range []int{0, 90, 180, 270} {
		l, t := dialLabelPos(float64(d), angleDialLabelOff)
		lab := dom.Doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", strconv.Itoa(d))
		// The dial is decorative, but the label still needs a title: without one
		// it shows the CELL's tooltip, so all four degree marks explained the
		// axis rather than the quarter turn each of them marks.
		lab.Set("title", docf("knob-quarter", "deg", strconv.Itoa(d)))
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", t)
		lab.Get("style").Set("transform", "translate(-50%,-50%) rotate("+strconv.Itoa(d)+"deg)")
		dial.Call("appendChild", lab)
	}
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
}

// fmtDialNum formats a value for a knob's numeric scale: compact, no sci
// notation (1000→"1k", 0.001→"0.001", -95→"-95").
func fmtDialNum(v float64) string {
	if v == 0 {
		return "0"
	}
	if math.Abs(v) >= 1000 {
		return strconv.FormatFloat(v/1000, 'g', 3, 64) + "k"
	}
	return strconv.FormatFloat(v, 'g', 3, 64)
}

// addValueDial draws an analog tick ring + numeric scale around a value knob
// (the .knobwrap from makeKnob), so a bare numeric knob shows its range at a
// glance like lab gear. The knob sweeps 270° (min at −135°/lower-left → max at
// +135°/lower-right); the min/max are labeled at the sweep ends (both in the
// lower half, clear of the numeric LED that sits above the knob) and the tick
// ring conveys the gradations between. Decorative (pointer-events:none),
// behind the knob.
func addValueDial(wrap js.Value, lo, hi float64) {
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial value-dial")
	// Discrete tick marks spanning ONLY the knob's 270° travel (−135°→+135°),
	// not the full circle — so the scale matches how far the knob actually
	// turns. A major (longer) tick every quarter aligns with where the min/max
	// numbers sit; minor ticks fill in between.
	const nTicks = dialTicks - 1 // 21 marks across the sweep; every 5th is a major
	for i := 0; i <= nTicks; i++ {
		t := float64(i) / float64(nTicks)
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*t
		major := i%5 == 0
		l, tp := dialLabelPos(deg, 41)
		tk := dom.Doc.Call("createElement", "span")
		cls := "vdial-tick"
		if major {
			cls += " major"
		}
		tk.Set("className", cls)
		st := tk.Get("style")
		st.Set("left", l)
		st.Set("top", tp)
		st.Set("transform", "translate(-50%,-50%) rotate("+strconv.FormatFloat(deg, 'f', 1, 64)+"deg)")
		dial.Call("appendChild", tk)
	}
	// Numbers at the two sweep ends (major ticks), both in the lower half so they
	// stay clear of the numeric LED above the knob.
	// Each end says which end it is. Without a title of its own a label shows
	// its nearest titled ancestor's tooltip, which here is the whole cell — so
	// hovering the "20" at the end of the palette-period scale explained what
	// palette period means, and so did hovering the "0.05" at the other end.
	for i, t := range []float64{0, 1} {
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*t
		l, tp := dialLabelPos(deg, 48)
		lab := dom.Doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		v := fmtDialNum(lo + (hi-lo)*t)
		lab.Set("textContent", v)
		if i == 0 {
			lab.Set("title", docf("knob-min", "v", v))
		} else {
			lab.Set("title", docf("knob-max", "v", v))
		}
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", tp)
		dial.Call("appendChild", lab)
	}
	wrap.Call("insertBefore", dial, wrap.Get("firstChild"))
	wrap.Get("classList").Call("add", "has-dial")
}

// addSelectorLabels places short labels around a selector-knob stack at the
// pointer's snap positions (the same 270° spread makeSelectorKnob snaps to), so
// a multi-position knob reads like a labeled rotary switch.
// addSelectorLabels places short labels around a selector-knob stack. If sel is
// truthy, each label is CLICKABLE (jumps the select to that option and turns the
// knob) and the label matching the current selection is highlighted.
// addSelectorDotLabels is addSelectorLabels for a color selector: instead of a
// text label at each detent it places a small filled dot in that option's
// color, so the ring reads as a swatch palette. The active option's dot gets a
// ring highlight. Used by the LED-color knob.
func addSelectorDotLabels(stack js.Value, colors []string, sel js.Value, offset ...float64) {
	off := 31.0
	if len(offset) > 0 {
		off = offset[0]
	}
	n := len(colors)
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	circle := dom.Doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	dotEls := make([]js.Value, n)
	for i, col := range colors {
		deg := 0.0
		if n > 1 {
			deg = -skirt.SweepDeg/2 + skirt.SweepDeg*float64(i)/float64(n-1)
		}
		l, t := dialLabelPos(deg, off)
		dot := dom.Doc.Call("createElement", "span")
		dot.Set("className", "knob-dial-dot clickable")
		dot.Get("style").Set("left", l)
		dot.Get("style").Set("top", t)
		dot.Get("style").Set("background", col)
		dialPosTitle(dot, sel, i)
		dotEls[i] = dot
		if sel.Truthy() {
			idx := i
			dom.On(dot, "click", func(this js.Value, a []js.Value) any {
				sel.Set("selectedIndex", idx)
				dom.Fire(sel, "change")
				return nil
			})
		}
		dial.Call("appendChild", dot)
	}
	if sel.Truthy() {
		hi := func() {
			ci := sel.Get("selectedIndex").Int()
			for j, d := range dotEls {
				if j == ci {
					d.Get("classList").Call("add", "dot-active")
				} else {
					d.Get("classList").Call("remove", "dot-active")
				}
			}
		}
		dom.On(sel, "change", func(this js.Value, a []js.Value) any { hi(); return nil })
		hi()
	}
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
}

// makeKnob builds a bounded knob assembly that drives slider. mirror, if
// truthy, is an extra element (e.g. the numeric box) whose 'input' also
// refreshes the pointer. withFine adds a nested fine-trim disc. register
// adds the knob to syncKnobs (use for persistent, not rebuilt-per-panel,
// knobs). Returns the wrapper element to insert into the panel.
func makeKnob(slider, mirror js.Value, withFine, register, valueDial bool) js.Value {
	// Its range and step from its record (controlspec_js.go), never from the
	// slider's attributes, which this and later builders rewrite.
	cs := specOf(slider)
	lo, hi := cs.lo, cs.hi
	// Let the slider carry values far finer than one coarse step, so the fine
	// knob/wheel can nudge a tenth of a step; the coarse control still
	// moves by whole coarse steps. Knobs built WITHOUT a fine disc keep the
	// authored step — integer-domain controls (lat/lon line counts, polygon
	// subdivisions) must snap to whole values.
	if withFine {
		cs.fine = true
		cs.setStep(slider, cs.step)
	}

	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "knobwrap")
	wrap.Call("setAttribute", "data-no-drag", "")

	knob := dom.Doc.Call("createElement", "span")
	knob.Set("className", "knob knobb")
	// Name the knob from its slider's title so hovering identifies the control.
	if t := slider.Get("title").String(); t != "" {
		knob.Set("title", t)
	}
	ptr := dom.Doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)
	wrap.Call("appendChild", knob)

	var fine js.Value
	if withFine {
		fine = dom.Doc.Call("createElement", "span")
		fine.Set("className", "knob-fine")
		// Fine-trim disc: name it from the control it trims (the slider title) so
		// it isn't a generic "fine" on every knob.
		if t := slider.Get("title").String(); t != "" {
			fine.Set("title", docf("knob-fine", "control", t))
		} else {
			fine.Set("title", doc("knob-fine.plain"))
		}
		knob.Call("appendChild", fine)
	}

	var dial js.Value // the value dial, once there is one: its ticks are an LED ring
	id := slider.Get("id").String()
	sweep := knobSweep(id)
	update := func() {
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		// An endless knob (turns_js.go) points round its lap and lights a
		// full ring; one with stops sweeps 270° across its range.
		spec, endless := turnSpecs[id]
		// A switch-pot's pointer is in its detent or in its range, never
		// in the gap between them (switchpot.go).
		if sp, ok := switchPots[id]; ok {
			v = sp.snap(v)
		}
		ang := knobAngleForValue(v, lo, hi, sweep)
		if endless {
			ang = spec.angle(v)
		}
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(ang, 'f', 1, 64)+"deg)")
		if dial.Truthy() {
			if endless {
				paintRing(dial, spec, v)
			} else {
				from, to := ringLit(v, lo, hi, dialTicks)
				lightRing(dial, from, to)
			}
		}
	}
	if id != "" {
		knobRefresh[id] = update
	}
	update()
	upd := dom.FuncOf(func(this js.Value, args []js.Value) any {
		update()
		return nil
	})
	slider.Call("addEventListener", "input", upd)
	if mirror.Truthy() {
		mirror.Call("addEventListener", "input", upd)
	}
	if register {
		knobSyncers = append(knobSyncers, update)
	}

	grab := func(fineMode bool, el js.Value) js.Func {
		return dom.FuncOf(func(this js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			r := knob.Call("getBoundingClientRect")
			kb.cx = r.Get("left").Float() + r.Get("width").Float()/2
			kb.cy = r.Get("top").Float() + r.Get("height").Float()/2
			kb.prevAng = math.Atan2(e.Get("clientY").Float()-kb.cy, e.Get("clientX").Float()-kb.cx)
			kb.slider, kb.min, kb.max, kb.fine, kb.active = slider, lo, hi, fineMode, true
			kb.sweep = sweep
			kb.knobEl = el
			// In a bank it is an encoder (see kb). Asked at the grab, because
			// the cell is made before the bank it is mounted in.
			kb.encoder = knob.Call("closest", ".dmdcell").Truthy()
			kb.step = cs.step // the step trim's, now
			kb.enc.Reset()
			el.Get("classList").Call("add", "knob-grab")
			return nil
		})
	}
	knob.Call("addEventListener", "pointerdown", grab(false, knob))
	if withFine {
		fine.Call("addEventListener", "pointerdown", grab(true, fine))
	}

	// Scroll wheel over a knob nudges its value: coarse step on the main
	// knob, fine step on the inner disc.
	// fine=false → step by one coarse step; fine=true → a tenth of one.
	// One definition of "one step of this knob", shared by the wheel and the
	// arrow keys, so the two inputs cannot drift apart.
	nudge := func(fineMode bool) func(up bool) {
		return func(up bool) {
			stepv := cs.step
			if fineMode {
				stepv = cs.step * fineRatio
			}
			v, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
			old := v
			if up {
				v += stepv
			} else {
				v -= stepv
			}
			if spec, endless := turnSpecs[id]; endless {
				v = spec.clamp(v)
			} else if sp, ok := switchPots[id]; ok {
				v = math.Max(lo, math.Min(hi, sp.step(old, v)))
			} else {
				v = math.Max(lo, math.Min(hi, v))
			}
			slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			dom.Fire(slider, "input")
		}
	}
	wheel := func(fineMode bool) js.Func {
		step := nudge(fineMode)
		return dom.FuncOf(func(this js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			step(e.Get("deltaY").Float() < 0)
			return nil
		})
	}
	knob.Call("addEventListener", "wheel", wheel(false))
	registerKnobHover(knob, nudge(false))
	if withFine {
		fine.Call("addEventListener", "wheel", wheel(true))
		registerKnobHover(fine, nudge(true))
	}
	if valueDial {
		addValueDial(wrap, lo, hi)
		dial = wrap.Call("querySelector", ".value-dial")
		update()
	}
	return wrap
}

// dialPosTitle gives one position on a selector's dial ring its own tooltip,
// taken from the <option> that position selects: the option's title when it has
// one, and otherwise the option's full text.
//
// Every dial position needs a title of its own, and not for decoration. An
// element with no title of its own shows its nearest titled ANCESTOR's tooltip
// instead, which for a dial label is the knob or the whole cell — so hovering
// "3150" on the Test knob explained what the Test knob is, and hovering "pink"
// beside it said exactly the same thing. Eleven positions, one sentence, and it
// was the sentence you get anyway by hovering anywhere else in the cell.
//
// The option is where the description belongs because the option IS the
// position: same order, same count, one list. A ring of tooltips written out
// beside the ring of labels is a second list of the same things, which is how
// the category ring came to be missing Maps.
func dialPosTitle(el, sel js.Value, i int) {
	if !sel.Truthy() {
		return
	}
	opts := sel.Get("options")
	if i < 0 || i >= opts.Get("length").Int() {
		return
	}
	o := opts.Index(i)
	t := o.Get("title").String()
	if t == "" {
		t = o.Get("text").String()
	}
	if t != "" {
		el.Set("title", t)
	}
}

// addSelectorLabels engraves a rotary switch's positions on a skirt around
// its knob.
//
// It no longer takes a radius. The radius is derived — see pkg/skirt — from
// the grip the skirt has to clear and the size of the labels themselves,
// because the twenty-six numbers this used to be given were chosen by eye
// and 201 of the 261 labels they produced sat on top of their own grip.
//
// The measuring cannot happen here: most callers build the stack detached
// and append it afterwards, so nothing has a size yet. Each label is left
// carrying the angle it belongs at, and layoutSkirts does the geometry once
// the dial is on screen. Returns the ring, so a caller with two on one knob
// can still tell them apart.
func addSelectorLabels(stack js.Value, labels []string, sel js.Value) js.Value {
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	// A thin guide circle at this ring's radius; the labels (opaque
	// background) sit on it, breaking it into an arc with small gaps —
	// visually tying each label ring to its concentric knob. Sized by
	// layoutSkirts along with everything else.
	circle := dom.Doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dial.Call("appendChild", circle)

	labEls := make([]js.Value, len(labels))
	for i, txt := range labels {
		lab := dom.Doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", txt)
		lab.Call("setAttribute", "data-deg",
			strconv.FormatFloat(labelDeg(sel, i, len(labels)), 'f', 2, 64))
		dialPosTitle(lab, sel, i)
		labEls[i] = lab
		if sel.Truthy() {
			lab.Get("classList").Call("add", "clickable")
			idx := i
			dom.On(lab, "click", func(this js.Value, a []js.Value) any {
				sel.Set("selectedIndex", idx)
				dom.Fire(sel, "change")
				return nil
			})
		}
		dial.Call("appendChild", lab)
	}
	if sel.Truthy() {
		hi := func() {
			ci := sel.Get("selectedIndex").Int()
			for j, le := range labEls {
				if j == ci {
					le.Get("classList").Call("add", "lab-active")
				} else {
					le.Get("classList").Call("remove", "lab-active")
				}
			}
		}
		dom.On(sel, "change", func(this js.Value, a []js.Value) any { hi(); return nil })
		hi()
	}
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
	layoutSkirtsIn(stack)
	return dial
}

// skirtGapPx is the daylight between a skirt and what it clears, and
// between two neighboring labels. Scales with the interface, because a
// gap that stayed one pixel would close up as everything around it grew.
func skirtGapPx() float64 { return 3.0 * layout.scale }

// layoutSkirts sizes every skirt on the panel.
//
// Run after a build rather than during one: a label has no width until it
// is in the document, and most of these are built in a detached subtree.
// Run again whenever the interface size changes, since every input to the
// geometry — grip, label, gap — scales with it.
//
// Deferred with the rest of the layout, because it is the tail of one.
// quantizeModuleWidths ends in a skirt pass of its own — the sizes depend
// on the widths it has just settled — and buildParamPanel asks for both, so
// every rebuild sized every skirt on the panel twice over. Collected, the
// second ask costs nothing and the one pass that runs is the later, better
// informed one.
func layoutSkirts() {
	if owed.deferred {
		owed.skirts = true
		return
	}
	layoutSkirtsNow()
}

func layoutSkirtsNow() { layoutSkirtsIn(js.Undefined()) }

// The estimates a skirt falls back to when nothing can be measured yet.
//
// Both track the stylesheet: the knob is 38px at scale 1 (.knobb) and a
// label is 'B612 Mono' at 8px (.knob-dial-lab). They are deliberately a
// little generous — an estimate that is too small puts a label back on the
// grip, which is the fault being fixed, while one that is too large only
// leaves a slightly wide ring until the measured pass corrects it.
func estGripRadiusPx() float64 { return 19.0 * layout.scale }

func estLabelBoxPx(text string) (w, h float64) {
	const px = 8.0      // .knob-dial-lab font-size at scale 1
	const perChar = 5.2 // B612 Mono advance at that size, rounded up
	n := max(len([]rune(text)), 1)
	return float64(n) * perChar * layout.scale, px * layout.scale
}

// The constants the two helpers above read against the stylesheet.
const (
	// skirtLabelBasePx is .knob-dial-lab's font-size at scale 1.
	skirtLabelBasePx = 8.0
	// skirtCellGapPx is how far past its cell a legend ring may reach.
	//
	// The gap between control cells is 20px, and a ring centered in one cell
	// could take half of that before it met the next cell's half. But the
	// cell at the END of a column has no neighbor to share a gutter with —
	// past it is the module's own edge — so a ring sized against the full
	// share put the module's scrollWidth past its width. Eight is what the
	// module's own padding can absorb, and it still leaves the widest
	// legends (CAM, .5) clear of everything.
	skirtCellGapPx = 8.0
)

// optionChars is the display a selector's readout needs: half, when every
// option's name fits four characters, and full otherwise.
func optionChars(sel js.Value) int {
	opts := sel.Get("options")
	for i := range opts.Length() {
		if len([]rune(displayText(opts.Index(i).Get("text").String()))) > dispHalfChars {
			return dispFullChars
		}
	}
	return dispHalfChars
}

// An ENDLESS selector (data-endless on its select) has its positions all the
// way round the knob, the first at the bottom where a stopped knob's gap is,
// and turns through its last position back to its first: the bay's category
// ring, whose OFF is at the bottom between the last category and the first.
func selEndless(sel js.Value) bool {
	return sel.Truthy() && sel.Call("hasAttribute", "data-endless").Bool()
}

// endlessDeg is position i of n on an endless selector, clockwise from the
// top: the first at the bottom, the rest evenly round from there.
func endlessDeg(i, n int) float64 {
	d := math.Mod(180+360*float64(i)/float64(n), 360)
	if d > 180 {
		d -= 360
	}
	return d
}

// selOverflow is what a selector does when it is turned past either end,
// by its select's id, in place of stopping or wrapping: the bay's model ring
// goes on into the next category (onBayModelOverflow). dir is +1 past the
// last position, -1 before the first.
var selOverflow = map[string]func(dir int){}

// selStep turns sel one position by dir: into its overflow if it has one,
// round if it is endless, and otherwise wrapping when wrap is set (a drag)
// or stopping at the end (the wheel and the keys).
func selStep(sel js.Value, dir int, wrap bool) {
	n := sel.Get("options").Get("length").Int()
	if n == 0 {
		return
	}
	at := sel.Get("selectedIndex").Int()
	idx := at + dir
	// On no position (the bay's model ring while the bay is OFF) is past
	// both ends at once: where it goes from there is its overflow's to say.
	if idx < 0 || idx >= n || at < 0 {
		if f := selOverflow[sel.Get("id").String()]; f != nil {
			f(dir)
			return
		}
		switch {
		case wrap || selEndless(sel):
			idx = ((idx % n) + n) % n
		case idx < 0:
			idx = 0
		default:
			idx = n - 1
		}
	}
	sel.Set("selectedIndex", idx)
	dom.Fire(sel, "change")
}

// labelDeg is where legend i of n goes round a selector's ring: over the
// knob's sweep, or all the way round for an endless one.
func labelDeg(sel js.Value, i, n int) float64 {
	if selEndless(sel) {
		return endlessDeg(i, n)
	}
	return skirt.Angles(n, skirt.SweepDeg)[i]
}
