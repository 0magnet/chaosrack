//go:build js && wasm

package attractor

import (
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

const knobSweepDeg = 270.0 // total pointer travel, centered on straight-up

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
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !kb.active {
			return nil
		}
		e := args[0]
		cur := math.Atan2(e.Get("clientY").Float()-kb.cy, e.Get("clientX").Float()-kb.cx)
		d := cur - kb.prevAng
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		kb.prevAng = cur
		v, _ := strconv.ParseFloat(kb.slider.Get("value").String(), 64)
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
		return nil
	}))
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
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !selkActive {
			return nil
		}
		e := args[0]
		r := selkKnob.Call("getBoundingClientRect")
		cx := r.Get("left").Float() + r.Get("width").Float()/2
		cy := r.Get("top").Float() + r.Get("height").Float()/2
		cur := math.Atan2(e.Get("clientY").Float()-cy, e.Get("clientX").Float()-cx)
		if math.Abs(cx-selkCX) > 1 || math.Abs(cy-selkCY) > 1 {
			selkCX, selkCY, selkPrev = cx, cy, cur
			return nil
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
		return nil
	}))
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
	knob := doc.Call("createElement", "div")
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
	// Scroll wheel over the knob steps the selection (like scrolling the
	// select itself), firing change so the bound handler reacts.
	knob.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		idx := sel.Get("selectedIndex").Int()
		n := sel.Get("options").Get("length").Int()
		if e.Get("deltaY").Float() > 0 {
			idx++
		} else {
			idx--
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		sel.Set("selectedIndex", idx)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
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
func singleSelectorKnob(sel js.Value, labels []string, offset float64) js.Value {
	stack := doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)
	addSelectorLabels(stack, labels, sel, offset)
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
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial")
	ticks := doc.Call("createElement", "div")
	ticks.Set("className", "angle-dial-ticks")
	dial.Call("appendChild", ticks)
	for _, d := range []int{0, 90, 180, 270} {
		l, t := dialLabelPos(float64(d), 44)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", strconv.Itoa(d))
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
	dial := doc.Call("createElement", "div")
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
	for _, t := range []float64{0, 1} {
		deg := -knobSweepDeg/2 + knobSweepDeg*t
		l, tp := dialLabelPos(deg, 48)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", fmtDialNum(min+(max-min)*t))
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
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial")
	circle := doc.Call("createElement", "div")
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

// addSelectorLabels places the option labels around a selector knob at radius
// offset[0]. offset[1], if given, rotates the whole label ring by that many
// degrees — used to stagger the style labels off the LED-color dots (which sit
// at the same detent angles on the inner ring) so the two rings don't collide.
func addSelectorLabels(stack js.Value, labels []string, sel js.Value, offset ...float64) {
	off := 46.0
	rot := 0.0
	if len(offset) > 0 {
		off = offset[0]
	}
	if len(offset) > 1 {
		rot = offset[1]
	}
	n := len(labels)
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial")
	// A thin guide circle at this ring's radius; the labels (opaque background)
	// sit on it, breaking it into an arc with small gaps — visually tying each
	// label ring to its concentric knob.
	circle := doc.Call("createElement", "div")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	labEls := make([]js.Value, n)
	for i, txt := range labels {
		deg := rot
		if n > 1 {
			deg = -knobSweepDeg/2 + knobSweepDeg*float64(i)/float64(n-1) + rot
		}
		l, t := dialLabelPos(deg, off)
		lab := doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", txt)
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", t)
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
}

// makeKnob builds a bounded knob assembly that drives slider. mirror, if
// truthy, is an extra element (e.g. the numeric box) whose 'input' also
// refreshes the pointer. withFine adds a nested fine-trim disc. register
// adds the knob to syncKnobs (use for persistent, not rebuilt-per-panel,
// knobs). Returns the wrapper element to insert into the panel.
func makeKnob(slider, mirror js.Value, withFine, register, valueDial bool) js.Value {
	min, _ := strconv.ParseFloat(slider.Get("min").String(), 64)
	max, _ := strconv.ParseFloat(slider.Get("max").String(), 64)
	coarseStep, _ := strconv.ParseFloat(slider.Get("step").String(), 64)
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

	knob := doc.Call("createElement", "div")
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
		fine = doc.Call("createElement", "div")
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
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64)
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
	wheel := func(fineMode bool) js.Func {
		return trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			stepv := coarseStep * coarseRatio
			if fineMode {
				stepv = coarseStep * coarseRatio * fineRatio
			}
			v, _ := strconv.ParseFloat(slider.Get("value").String(), 64)
			if e.Get("deltaY").Float() < 0 {
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
			return nil
		})
	}
	knob.Call("addEventListener", "wheel", wheel(false))
	if withFine {
		fine.Call("addEventListener", "wheel", wheel(true))
	}
	if valueDial {
		addValueDial(wrap, min, max)
	}
	return wrap
}
