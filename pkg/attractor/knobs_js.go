//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"strings"
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
	cx, cy   float64
	prevAng  float64
	active   bool
	dragInit bool
}

// fineRatio: the fine knob's step and drag sensitivity as a fraction of the
// coarse step (0.1 = fine moves in tenths of a coarse step). Adjustable at
// runtime via the "Fine ×" control; read live by drag and wheel.

// coarseRatio scales the coarse step / drag sensitivity of every param knob
// (the "Step ×" control), read live so changes take effect without a rebuild.
var coarseRatio = 1.0

func knobAngleForValue(v, min, max float64) float64 {
	if max <= min {
		return 0
	}
	t := (v - min) / (max - min)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return -knobSweepDeg/2 + knobSweepDeg*t
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
		scale := coarseRatio
		if kb.fine {
			scale = coarseRatio * fineRatio
		}
		v += (d / (knobSweepDeg * math.Pi / 180)) * (kb.max - kb.min) * scale
		if v < kb.min {
			v = kb.min
		}
		if v > kb.max {
			v = kb.max
		}
		kb.slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		kb.slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	})
	release := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		kb.active = false
		if kb.knobEl.Truthy() {
			kb.knobEl.Get("classList").Call("remove", "knob-grab")
		}
		return nil
	})
	doc.Call("addEventListener", "pointerup", release)
	doc.Call("addEventListener", "pointercancel", release)
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

// ── Multi-position selector knobs (rotary encoder over a <select>) ───────────
// Shared drag state + one-time document listeners, so per-parameter selector
// knobs rebuilt with the panel don't accumulate listeners.
var (
	selkActive                        bool
	selkSel, selkKnob                 js.Value
	selkCX, selkCY, selkPrev, selkAcc float64
	selkDragInit                      bool
)

func selkStep(dir int) {
	if !selkSel.Truthy() {
		return
	}
	n := selkSel.Get("options").Get("length").Int()
	if n == 0 {
		return
	}
	idx := selkSel.Get("selectedIndex").Int() + dir
	for idx < 0 {
		idx += n
	}
	idx %= n
	selkSel.Set("selectedIndex", idx)
	selkSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
}

// initSelKnobDrag wires the one-time document move/up listeners that turn the
// active selector knob. Recomputes the center each move and resyncs on a
// panel reflow so one detent = one step. Called once from Run.
func initSelKnobDrag() {
	if selkDragInit {
		return
	}
	selkDragInit = true
	onPointerMove(func(e js.Value) {
		if !selkActive {
			return
		}
		r := selkKnob.Call("getBoundingClientRect")
		cx := r.Get("left").Float() + r.Get("width").Float()/2
		cy := r.Get("top").Float() + r.Get("height").Float()/2
		cur := math.Atan2(e.Get("clientY").Float()-cy, e.Get("clientX").Float()-cx)
		if math.Abs(cx-selkCX) > 1 || math.Abs(cy-selkCY) > 1 {
			selkCX, selkCY, selkPrev = cx, cy, cur
			return
		}
		d := cur - selkPrev
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		selkPrev = cur
		dDeg := d * 180 / math.Pi
		// The pointer isn't turned freely here — it snaps to the selected slot
		// via the select's 'change' handler each time a detent steps it.
		// One full turn = one pass through the options: detent = 360°/N.
		detent := 24.0
		if selkSel.Truthy() {
			if n := selkSel.Get("options").Get("length").Int(); n > 0 {
				detent = 360.0 / float64(n)
			}
		}
		for selkAcc += dDeg; selkAcc >= detent; selkAcc -= detent {
			selkStep(1)
		}
		for ; selkAcc <= -detent; selkAcc += detent {
			selkStep(-1)
		}
	})
	rel := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		selkActive = false
		return nil
	})
	doc.Call("addEventListener", "pointerup", rel)
	doc.Call("addEventListener", "pointercancel", rel)
}

// makeSelectorKnob builds a rotary-encoder knob that steps sel's options,
// like the model selector. Returns the knob element to place before sel.
// makeSelectorKnob builds a rotary selector over sel. An optional rot (degrees)
// offsets the pointer so it lines up with labels that were rotated by the same
// amount (e.g. the knob-style ring, staggered off the LED-color dots).
func makeSelectorKnob(sel js.Value, rot ...float64) js.Value {
	ptrRot := 0.0
	if len(rot) > 0 {
		ptrRot = rot[0]
	}
	knob := doc.Call("createElement", "span")
	knob.Set("className", "knob knobsel")
	knob.Call("setAttribute", "data-no-drag", "")
	// Name the knob from the select it drives (single source: set the title on
	// the <select> once and every knob/label built from it inherits it), so each
	// selector knob identifies its own control instead of a generic hint.
	if t := sel.Get("title").String(); t != "" {
		knob.Set("title", t+" — turn to select")
	} else {
		knob.Set("title", "turn to change selection")
	}
	ptr := doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)
	// The pointer snaps to the selected option's slot (270° spread over the
	// options) — it only ever points at a valid position, never in between.
	// Driven off the select's 'change', so drag detents, the wheel, the
	// dropdown, and permalink restores all move it.
	snap := func() {
		n := sel.Get("options").Get("length").Int()
		idx := sel.Get("selectedIndex").Int()
		ang := ptrRot
		if n > 1 {
			ang = -knobSweepDeg/2 + knobSweepDeg*float64(idx)/float64(n-1) + ptrRot
		}
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(ang, 'f', 1, 64)+"deg)")
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		snap()
		return nil
	}))
	snap()
	knob.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		r := knob.Call("getBoundingClientRect")
		selkCX = r.Get("left").Float() + r.Get("width").Float()/2
		selkCY = r.Get("top").Float() + r.Get("height").Float()/2
		selkPrev = math.Atan2(e.Get("clientY").Float()-selkCY, e.Get("clientX").Float()-selkCX)
		selkSel, selkKnob = sel, knob
		selkAcc = 0
		selkActive = true
		return nil
	}))
	// One step of the selection, shared by the wheel and the arrow keys, for
	// the same reason the value knobs share theirs.
	step := func(up bool) {
		idx := sel.Get("selectedIndex").Int()
		n := sel.Get("options").Get("length").Int()
		if up {
			idx--
		} else {
			idx++
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		sel.Set("selectedIndex", idx)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
	// Scroll wheel over the knob steps the selection (like scrolling the
	// select itself), firing change so the bound handler reacts.
	knob.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		step(e.Get("deltaY").Float() < 0)
		return nil
	}))
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
	stack := doc.Call("createElement", "span")
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
	stack := doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	k := makeSelectorKnob(sel)
	k.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", k)
	return stack
}

// setLabelTooltips sets a per-label title on a selector knob's dial labels,
// matched by the label text, so rotary-switch positions get unique tooltips.
func setLabelTooltips(stack js.Value, tips map[string]string) {
	labs := stack.Call("querySelectorAll", ".knob-dial-lab")
	for i := 0; i < labs.Get("length").Int(); i++ {
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
	stack := doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)
	addSelectorLabels(stack, labels, sel)
	return stack
}

// selectorKnobReadout builds a lone rotary-switch knob with a live text readout
// of the current option beneath it, for selectors that have too many options or
// too-long labels for a ring of labels around the dial (e.g. Phosphor). Returns
// a wrapper element to place in the panel.
func selectorKnobReadout(sel js.Value) js.Value {
	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "selk-ro")
	stack := doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)
	readout := doc.Call("createElement", "span")
	readout.Set("className", "selk-readout")
	set := func() {
		idx := sel.Get("selectedIndex").Int()
		if idx < 0 {
			idx = 0
		}
		readout.Set("textContent", sel.Get("options").Index(idx).Get("text").String())
		// The readout describes what it is CURRENTLY showing. Without a title of
		// its own it showed the cell's, which is a paragraph about the knob —
		// the same paragraph whatever the readout said, and available from
		// anywhere else in the cell anyway.
		dialPosTitle(readout, sel, idx)
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { set(); return nil }))
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

// addAngleDial draws an analog degree dial around a stack's outer ring: tick
// marks every 30° with 0/90/180/270 labels, like a clock but in degrees.
// Decorative (pointer-events:none) and behind the knob, so only the part
// outside the ring shows.
func addAngleDial(stack js.Value) {
	dial := doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	ticks := doc.Call("createElement", "span")
	ticks.Set("className", "angle-dial-ticks")
	dial.Call("appendChild", ticks)
	for _, d := range []int{0, 90, 180, 270} {
		l, t := dialLabelPos(float64(d), 44)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", strconv.Itoa(d))
		// The dial is decorative, but the label still needs a title: without one
		// it shows the CELL's tooltip, so all four degree marks explained the
		// axis rather than the quarter turn each of them marks.
		lab.Set("title", strconv.Itoa(d)+"° — a quarter-turn mark on the angle scale")
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", t)
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
func addValueDial(wrap js.Value, min, max float64) {
	dial := doc.Call("createElement", "span")
	dial.Set("className", "knob-dial value-dial")
	// Discrete tick marks spanning ONLY the knob's 270° travel (−135°→+135°),
	// not the full circle — so the scale matches how far the knob actually
	// turns. A major (longer) tick every quarter aligns with where the min/max
	// numbers sit; minor ticks fill in between.
	const nTicks = 20 // 21 marks across the sweep; every 5th is a major
	for i := 0; i <= nTicks; i++ {
		t := float64(i) / float64(nTicks)
		deg := -knobSweepDeg/2 + knobSweepDeg*t
		major := i%5 == 0
		l, tp := dialLabelPos(deg, 41)
		tk := doc.Call("createElement", "span")
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
		deg := -knobSweepDeg/2 + knobSweepDeg*t
		l, tp := dialLabelPos(deg, 48)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		v := fmtDialNum(min + (max-min)*t)
		lab.Set("textContent", v)
		if i == 0 {
			lab.Set("title", v+" — the lowest this knob goes; turned fully counter-clockwise")
		} else {
			lab.Set("title", v+" — the highest this knob goes; turned fully clockwise")
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
	dial := doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	circle := doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	dotEls := make([]js.Value, n)
	for i, col := range colors {
		deg := 0.0
		if n > 1 {
			deg = -knobSweepDeg/2 + knobSweepDeg*float64(i)/float64(n-1)
		}
		l, t := dialLabelPos(deg, off)
		dot := doc.Call("createElement", "span")
		dot.Set("className", "knob-dial-dot clickable")
		dot.Get("style").Set("left", l)
		dot.Get("style").Set("top", t)
		dot.Get("style").Set("background", col)
		dialPosTitle(dot, sel, i)
		dotEls[i] = dot
		if sel.Truthy() {
			idx := i
			dot.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				sel.Set("selectedIndex", idx)
				sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				return nil
			}))
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
		sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { hi(); return nil }))
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
	min, _ := strconv.ParseFloat(slider.Get("min").String(), 64)         //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	max, _ := strconv.ParseFloat(slider.Get("max").String(), 64)         //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	coarseStep, _ := strconv.ParseFloat(slider.Get("step").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	if coarseStep <= 0 {
		coarseStep = (max - min) / 100
	}
	// Let the slider carry values far finer than one coarse step, so the fine
	// knob/wheel can nudge sub-step at any fineRatio; the coarse control still
	// moves by whole coarse steps. Knobs built WITHOUT a fine disc keep the
	// authored step — integer-domain controls (lat/lon line counts, polygon
	// subdivisions) must snap to whole values.
	if withFine {
		slider.Set("step", strconv.FormatFloat(coarseStep*0.001, 'g', -1, 64))
	}

	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "knobwrap")
	wrap.Call("setAttribute", "data-no-drag", "")

	knob := doc.Call("createElement", "span")
	knob.Set("className", "knob knobb")
	// Name the knob from its slider's title so hovering identifies the control.
	if t := slider.Get("title").String(); t != "" {
		knob.Set("title", t)
	}
	ptr := doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)
	wrap.Call("appendChild", knob)

	var fine js.Value
	if withFine {
		fine = doc.Call("createElement", "span")
		fine.Set("className", "knob-fine")
		// Fine-trim disc: name it from the control it trims (the slider title) so
		// it isn't a generic "fine" on every knob.
		if t := slider.Get("title").String(); t != "" {
			fine.Set("title", "Fine trim — "+t)
		} else {
			fine.Set("title", "fine trim")
		}
		knob.Call("appendChild", fine)
	}

	update := func() {
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		ang := knobAngleForValue(v, min, max)
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(ang, 'f', 1, 64)+"deg)")
	}
	update()
	upd := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
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
		return trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			r := knob.Call("getBoundingClientRect")
			kb.cx = r.Get("left").Float() + r.Get("width").Float()/2
			kb.cy = r.Get("top").Float() + r.Get("height").Float()/2
			kb.prevAng = math.Atan2(e.Get("clientY").Float()-kb.cy, e.Get("clientX").Float()-kb.cx)
			kb.slider, kb.min, kb.max, kb.fine, kb.active = slider, min, max, fineMode, true
			kb.knobEl = el
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
	// fine=false → step by one coarse step; fine=true → coarseStep·fineRatio
	// (read live, so the "Fine ×" control takes effect without a rebuild).
	// One definition of "one step of this knob", shared by the wheel and the
	// arrow keys. Keeping it in a single closure is the point: the two inputs
	// cannot drift apart, and both pick up the live fine ratio.
	nudge := func(fineMode bool) func(up bool) {
		return func(up bool) {
			stepv := coarseStep * coarseRatio
			if fineMode {
				stepv = coarseStep * coarseRatio * fineRatio
			}
			v, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
			if up {
				v += stepv
			} else {
				v -= stepv
			}
			if v < min {
				v = min
			}
			if v > max {
				v = max
			}
			slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
	}
	wheel := func(fineMode bool) js.Func {
		step := nudge(fineMode)
		return trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
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
		addValueDial(wrap, min, max)
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
// It no longer takes a radius. The radius is derived — see skirt.go — from
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
	return addSelectorLabelsRot(stack, labels, sel, 0)
}

// addSelectorLabelsRot is the same with the whole ring turned by rot
// degrees.
//
// A rotation is NOT derivable the way the radius is: it exists to
// stagger one ring off another's markings — the style labels off the
// LED-color dots at the same detents — which is a fact about the other
// ring, not about this one's geometry.
func addSelectorLabelsRot(stack js.Value, labels []string, sel js.Value, rot float64) js.Value {
	dial := doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	// A thin guide circle at this ring's radius; the labels (opaque
	// background) sit on it, breaking it into an arc with small gaps —
	// visually tying each label ring to its concentric knob. Sized by
	// layoutSkirts along with everything else.
	circle := doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dial.Call("appendChild", circle)

	labEls := make([]js.Value, len(labels))
	for i, txt := range labels {
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", txt)
		lab.Call("setAttribute", "data-deg",
			strconv.FormatFloat(skirtAngles(len(labels), knobSweepDeg)[i]+rot, 'f', 2, 64))
		dialPosTitle(lab, sel, i)
		labEls[i] = lab
		if sel.Truthy() {
			lab.Get("classList").Call("add", "clickable")
			idx := i
			lab.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				sel.Set("selectedIndex", idx)
				sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				return nil
			}))
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
		sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { hi(); return nil }))
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
func skirtGapPx() float64 { return 3.0 * panelScale }

// layoutSkirts sizes every skirt on the panel.
//
// Run after a build rather than during one: a label has no width until it
// is in the document, and most of these are built in a detached subtree.
// Run again whenever the interface size changes, since every input to the
// geometry — grip, label, gap — scales with it.
func layoutSkirts() {
	stacks := doc.Call("querySelectorAll", ".has-dial")
	for i := 0; i < stacks.Get("length").Int(); i++ {
		layoutSkirtsIn(stacks.Index(i))
	}
}

// layoutSkirtsIn sizes the skirts on one knob, nesting them outward.
//
// Outward in DOM order, because a concentric control carries concentric
// skirts: the inner knob's positions are engraved inside the outer knob's,
// and each ring has to clear not just the grip but everything already
// placed around it. That is what the 43-and-31 pairs at the old call sites
// were doing by hand.
func layoutSkirtsIn(stack js.Value) {
	if !stack.Truthy() {
		return
	}
	clear := gripRadiusPx(stack)
	if clear <= 0 {
		// Not laid out yet — a detached subtree, a module switched out, a
		// panel not yet shown. ESTIMATE rather than bail: a skirt that is
		// never laid out has no positions at all, and every one of its
		// labels sits on the origin in a heap. A rough ring is wrong by a
		// pixel or two; no ring is wrong by the width of the knob, and it
		// was the larger half of what the audit found still broken.
		clear = estGripRadiusPx()
	}
	gap := skirtGapPx()
	dials := stack.Call("querySelectorAll", ":scope > .knob-dial")
	for i := 0; i < dials.Get("length").Int(); i++ {
		// Only the first ring is sitting on the knob. For the ones outside
		// it "clear" is the previous ring's outer edge, not a grip, so there
		// is no grip for them to take room from — see layoutOneSkirt.
		clear = layoutOneSkirt(dials.Index(i), clear, gap, i == 0)
	}
}

// gripRadiusPx is the radius of the largest knob on this stack — what the
// first skirt has to clear.
func gripRadiusPx(stack js.Value) float64 {
	els := stack.Call("querySelectorAll", ".knob, .knob-ring")
	max := 0.0
	for i := 0; i < els.Get("length").Int(); i++ {
		if w := els.Index(i).Get("offsetWidth").Float(); w/2 > max {
			max = w / 2
		}
	}
	return max
}

// layoutOneSkirt places one ring and returns how far out it reaches, for
// the next ring to clear.
func layoutOneSkirt(dial js.Value, clear, gap float64, onGrip bool) float64 {
	els := dial.Call("querySelectorAll", ".knob-dial-lab")
	n := els.Get("length").Int()
	labs := make([]skirtLabel, 0, n)
	kept := make([]js.Value, 0, n)
	for i := 0; i < n; i++ {
		el := els.Index(i)
		deg, err := strconv.ParseFloat(el.Call("getAttribute", "data-deg").String(), 64)
		if err != nil {
			continue // not one of ours (the angle dial's tick labels)
		}
		w := el.Get("offsetWidth").Float()
		h := el.Get("offsetHeight").Float()
		if w <= 0 || h <= 0 {
			// Same reason as the grip above: estimated from the text, so an
			// unmeasurable label still gets a place on the ring.
			w, h = estLabelBoxPx(el.Get("textContent").String())
		}
		labs = append(labs, skirtLabel{W: w, H: h, Deg: deg})
		kept = append(kept, el)
	}
	if len(labs) == 0 {
		return clear
	}

	// Fit the ring to the cell before placing it. A skirt sized only by its
	// legends can reach past the control cell and into the next control's
	// space — Model Out's off/CAM/XY/XZ/YZ ring did, by 25px. skirtFit takes
	// the room out of the grip first and the legend only after that; see the
	// note in skirt.go for why that order.
	// A ring outside another one has no grip to take room from, so its floor
	// is the radius it already has and the legend carries the whole
	// reduction.
	minGrip := clear
	if onGrip {
		minGrip = clear * skirtMinGripFrac
	}
	// Less the gap, because the box drawn below is 2*(out+gap): fitting to
	// the bare room left every ring exactly one gap wider than the space it
	// was fitted into, which is the 6px Model Out had left over.
	room := skirtRoomPx(dial)
	if room > 0 {
		room -= gap
	}
	useGrip, scale := skirtFit(clear, minGrip, gap, room, labs)
	if scale < 1 {
		labs = skirtScaleLabels(labs, scale)
		for _, el := range kept {
			el.Get("style").Set("font-size", pxStr(skirtLabelBasePx*panelScale*scale))
		}
	}
	if useGrip < clear {
		shrinkGrip(dial, useGrip/clear)
	}
	clear = useGrip

	r := skirtRadius(clear, gap, labs)
	out := skirtOuter(r, labs)

	// The box has to contain the labels, or the element that exists to hold
	// them is the thing clipping them.
	box := 2 * (out + gap)
	st := dial.Get("style")
	st.Set("width", pxStr(box))
	st.Set("height", pxStr(box))
	for i, el := range kept {
		x := box/2 + r*math.Sin(labs[i].Deg*math.Pi/180)
		y := box/2 - r*math.Cos(labs[i].Deg*math.Pi/180)
		es := el.Get("style")
		es.Set("left", pxStr(x))
		es.Set("top", pxStr(y))
	}
	if c := dial.Call("querySelector", ".knob-ring-circle"); c.Truthy() {
		cs := c.Get("style")
		cs.Set("width", pxStr(2*r))
		cs.Set("height", pxStr(2*r))
	}
	return out
}

// The estimates a skirt falls back to when nothing can be measured yet.
//
// Both track the stylesheet: the knob is 38px at scale 1 (.knobb) and a
// label is 'B612 Mono' at 8px (.knob-dial-lab). They are deliberately a
// little generous — an estimate that is too small puts a label back on the
// grip, which is the fault being fixed, while one that is too large only
// leaves a slightly wide ring until the measured pass corrects it.
func estGripRadiusPx() float64 { return 19.0 * panelScale }

func estLabelBoxPx(text string) (w, h float64) {
	const px = 8.0      // .knob-dial-lab font-size at scale 1
	const perChar = 5.2 // B612 Mono advance at that size, rounded up
	n := len([]rune(text))
	if n < 1 {
		n = 1
	}
	return float64(n) * perChar * panelScale, px * panelScale
}

// skirtRoomPx is how far this ring may reach from its center before it is in
// the next control's space: half the control cell it sits in, plus half the
// gap between cells, which is the ring's own share of the space between two
// of them.
//
// Zero when there is no cell to measure or it has not been laid out. That is
// "unconstrained" rather than "no room": skirtFit reads it that way, and the
// alternative is shrinking every knob on a panel nobody has shown yet.
func skirtRoomPx(dial js.Value) float64 {
	cell := dial.Call("closest", ".pcell")
	if !cell.Truthy() {
		return 0
	}
	w := cell.Get("clientWidth").Float()
	if w <= 0 {
		return 0
	}
	cs := js.Global().Call("getComputedStyle", cell)
	pad := parsePx(cs.Get("paddingLeft").String()) + parsePx(cs.Get("paddingRight").String())
	return (w - pad + skirtCellGapPx*panelScale) / 2
}

// shrinkGrip scales the knob this ring sits on, so the room the ring needed
// comes out of the grip. Scaling rather than resizing keeps the pointer, the
// shading and the guide circle in proportion with no second set of numbers to
// keep in step.
//
// Only the LARGEST knob on the stack, because that is the one gripRadiusPx
// measured and therefore the one the radius was computed against. Scaling
// every knob in a concentric stack moved the inner one for no reason.
//
// The centering translate has to be carried. A .knob-ring is placed with
// left/top 50% and transform:translate(-50%,-50%); writing a bare scale()
// over that is not a smaller knob, it is a knob half its own width down and
// to the right — which is exactly what it looked like.
func shrinkGrip(dial js.Value, f float64) {
	if f <= 0 || f >= 1 {
		return
	}
	stack := dial.Get("parentElement")
	if !stack.Truthy() {
		return
	}
	knobs := stack.Call("querySelectorAll", ":scope > .knob, :scope > .knob-ring")
	var biggest js.Value
	max := 0.0
	for i := 0; i < knobs.Get("length").Int(); i++ {
		k := knobs.Index(i)
		if w := k.Get("offsetWidth").Float(); w > max {
			max, biggest = w, k
		}
	}
	if !biggest.Truthy() {
		return
	}
	s := "scale(" + strconv.FormatFloat(f, 'f', 3, 64) + ")"
	if biggest.Get("classList").Call("contains", "knob-ring").Bool() {
		s = "translate(-50%,-50%) " + s
	}
	st := biggest.Get("style")
	st.Set("transform", s)
	st.Set("transformOrigin", "center center")
}

// parsePx reads a computed length like "7px". Anything unparseable is zero,
// which is the right answer for "auto" and for an empty string.
func parsePx(v string) float64 {
	v = strings.TrimSuffix(strings.TrimSpace(v), "px")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
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
