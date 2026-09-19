//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
)

// The rack scope's front panel and its tube.
//
// The tube is its OWN canvas with a 2-D context, not a corner of the WebGL
// one. That is the whole design: this instrument does not share the model's
// pipeline, its camera, its palette or its mode, so it does not go dark when
// the MODEL knob moves to a polyhedron and it does not need the grid, the
// sweep or the phosphor shader to agree with it. A scope you have to put the
// rack into a particular state to read is not an instrument in the rack, it
// is a skin on the main view.
//
// Persistence is done the way a tube does it rather than the way a canvas
// does: the frame is not cleared, it is painted over with a low-alpha black
// so the last few sweeps fade out underneath the new one. That IS the
// afterglow, and it is why the INTENSITY knob changes how long the trace
// hangs around as well as how bright it is.

// scopeFuncs is the panel's own js.Func arena: its dials are built once,
// on a different schedule from the parameter panel's.
var scopeFuncs []js.Func

// scopeState is the front panel's settings — where every knob is.
//
// Held here rather than read out of the DOM each frame because the draw runs
// sixty times a second and a DOM read per knob per frame is the one way this
// module could cost anything. The listeners write in; the draw reads.
type scopeState struct {
	voltsIdx int     // detent on the VOLTS/DIV switch
	timeIdx  int     // detent on the TIME/DIV switch
	vpos     float64 // vertical POSITION, in divisions
	hpos     float64 // horizontal POSITION, in divisions
	trigLvl  float64 // TRIGGER LEVEL, in full scale
	rising   bool    // SLOPE
	trigAuto bool    // MODE: auto sweeps when nothing crosses, norm waits
	intens   float64 // INTENSITY: beam brightness and afterglow
	focus    float64 // FOCUS: spot tightness
	beam     bool    // BEAM: the tube is on
	chanSel  int     // SOURCE: which signal is on the vertical
}

// The SOURCE positions. X-Y is last because it is the one that stops being a
// timebase at all — the horizontal comes off the other channel instead of
// off the sweep, which is how a goniometer is made out of a scope.
const (
	scopeChanL = iota
	scopeChanR
	scopeChanMid
	scopeChanXY
)

var scopeChanNames = []string{"CH 1", "CH 2", "MID", "X-Y"}

// scopeUI is the panel. Defaults are a scope you could hand to someone: a
// range that fits a normalized signal, a sweep slow enough to see a waveform
// on, auto trigger so silence still draws a baseline, and the beam on.
var scopeUI = scopeState{
	voltsIdx: scopeNearestStep(scopeVoltsDivs, 0.5),
	timeIdx:  scopeNearestStep(scopeTimebases, 2e-3),
	rising:   true,
	trigAuto: true,
	intens:   0.6,
	focus:    0.55,
	beam:     true,
}

// scopeCanvas and scopeCtx are the tube, looked up once.
var (
	scopeCanvas js.Value
	scopeCtx    js.Value
	// scopeSampL and scopeSampR are the capture buffers, grown rather than
	// reallocated: the slowest timebase is half a second a division, which is
	// five seconds of audio across the screen.
	scopeSampL, scopeSampR []float32
)

func scopeSecPerDiv() float64 {
	if len(scopeTimebases) == 0 {
		return 1e-3
	}
	return scopeTimebases[clampIdx(scopeUI.timeIdx, len(scopeTimebases))]
}

func scopeVoltsPerDiv() float64 {
	if len(scopeVoltsDivs) == 0 {
		return 0.5
	}
	return scopeVoltsDivs[clampIdx(scopeUI.voltsIdx, len(scopeVoltsDivs))]
}

func clampIdx(i, n int) int {
	if n <= 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// buildRackScope fills the panel's dials and wires them. Called once, after
// the control panel exists.
func buildRackScope() {
	rebuildInto(&scopeFuncs, func() {
		// The two range switches, from the sequences themselves — a hand-typed
		// option list is a second copy of the spec, and the one that goes stale.
		buildScopeDial("scope-volts", scopeVoltsDivs, scopeFormatVolts, scopeUI.voltsIdx,
			func(i int) { scopeUI.voltsIdx = i })
		buildScopeDial("scope-time", scopeTimebases, scopeFormatTime, scopeUI.timeIdx,
			func(i int) { scopeUI.timeIdx = i })
		buildScopeNameDial("scope-chan", scopeChanNames, scopeUI.chanSel,
			func(i int) { scopeUI.chanSel = i })
		buildScopeNameDial("scope-tmode", []string{"AUTO", "NORM"}, 0,
			func(i int) { scopeUI.trigAuto = i == 0 })

		wireScopeRange("scope-vpos", func(v float64) { scopeUI.vpos = v })
		wireScopeRange("scope-hpos", func(v float64) { scopeUI.hpos = v })
		wireScopeRange("scope-trig", func(v float64) { scopeUI.trigLvl = v })
		wireScopeRange("scope-intens", func(v float64) { scopeUI.intens = v })
		wireScopeRange("scope-focus", func(v float64) { scopeUI.focus = v })
		wireScopeSwitch("scope-slope", func(on bool) { scopeUI.rising = on })
		wireScopeSwitch("scope-beam", func(on bool) { scopeUI.beam = on })
	})
}

// buildScopeDial rings a detented range switch, labeled with the values it
// actually selects.
func buildScopeDial(id string, steps []float64, label func(float64) string, at int, set func(int)) {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = label(s)
	}
	buildScopeNameDial(id, names, at, set)
}

// buildScopeNameDial rings a switch whose positions are named rather than
// numbered. Both dials go through soloKnob and addSelectorLabels, so a scope
// knob is the same object as every other knob in the rack — it turns the
// same way, scrolls the same way, and is the same size.
func buildScopeNameDial(id string, names []string, at int, set func(int)) {
	sel := doc.Call("getElementById", id)
	holder := doc.Call("getElementById", id+"-stack")
	if !sel.Truthy() || !holder.Truthy() || len(names) == 0 {
		return
	}
	sel.Set("innerHTML", "")
	// One loop fills the options and the ring labels together. Two lists
	// bound by index, written out separately, is the shape of every bug this
	// panel has had.
	ring := make([]string, len(names))
	for i, n := range names {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", n)
		sel.Call("appendChild", opt)
		ring[i] = n
	}
	sel.Set("value", strconv.Itoa(clampIdx(at, len(names))))
	sel.Get("style").Set("display", "none")

	holder.Set("innerHTML", "")
	stack := soloKnob(sel)
	addSelectorLabels(stack, ring, sel).Set("id", id+"-ring")
	holder.Call("appendChild", stack)

	sel.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		if n, err := strconv.Atoi(sel.Get("value").String()); err == nil {
			set(n)
			setScopeReadout(id, names, n)
		}
		return nil
	}))
	setScopeReadout(id, names, clampIdx(at, len(names)))
}

// setScopeReadout puts the switch's current position in the window under
// the knob. A range switch prints every position around its skirt, which
// says what it COULD be set to; the window says what it IS, and on an
// instrument being read at a glance that is the one that matters.
func setScopeReadout(id string, names []string, i int) {
	el := doc.Call("getElementById", id+"-read")
	if !el.Truthy() || len(names) == 0 {
		return
	}
	el.Set("textContent", names[clampIdx(i, len(names))])
}

// wireScopeRange turns a slider into a knob and reports its value.
//
// The knob goes in the stack span the markup leaves for it rather than
// beside the hidden slider, because these controls are laid out in
// labeled columns on a faceplate and not in the parameter grid's cells.
// makeKnob is the same one every other knob in the rack is made by, so a
// scope knob drags, scrolls and looks exactly like the rest.
func wireScopeRange(id string, set func(float64)) {
	el := doc.Call("getElementById", id)
	holder := doc.Call("getElementById", id+"-stack")
	if !el.Truthy() {
		return
	}
	el.Get("style").Set("display", "none")
	if holder.Truthy() {
		holder.Set("innerHTML", "")
		holder.Call("appendChild", makeKnob(el, js.Undefined(), true, true, false))
	}
	read := func() {
		if v, err := strconv.ParseFloat(el.Get("value").String(), 64); err == nil {
			set(v)
		}
	}
	el.Call("addEventListener", "input", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		read()
		return nil
	}))
	read()
}
func wireScopeSwitch(id string, set func(bool)) {
	el := doc.Call("getElementById", id)
	if !el.Truthy() {
		return
	}
	el.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		set(el.Get("checked").Bool())
		return nil
	}))
	set(el.Get("checked").Bool())
}

// ── the tube ────────────────────────────────────────────────────────────

// drawRackScope paints one frame of the scope's screen. Called from the
// render loop, and a no-op when the module is not on screen — a rack scope
// switched out of the rack must not cost a capture and a canvas paint per
// frame for a tube nobody can see.
func drawRackScope() {
	if !scopeVisible() {
		return
	}
	if !scopeCtx.Truthy() {
		scopeCanvas = doc.Call("getElementById", "scope-screen")
		if !scopeCanvas.Truthy() {
			return
		}
		scopeCtx = scopeCanvas.Call("getContext", "2d")
		if !scopeCtx.Truthy() {
			return
		}
	}
	w := scopeCanvas.Get("width").Float()
	h := scopeCanvas.Get("height").Float()
	if !(w > 0 && h > 0) {
		return
	}

	if !scopeUI.beam {
		// Switched off: the glass goes dark and STAYS dark. Not a frozen last
		// frame — a scope with no beam shows nothing, and leaving the trace up
		// would say the instrument was still measuring.
		scopeCtx.Set("globalAlpha", 1.0)
		scopeCtx.Set("fillStyle", "#05070a")
		scopeCtx.Call("fillRect", 0, 0, w, h)
		drawScopeFaceGrat(w, h)
		return
	}

	// The afterglow. Painting over rather than clearing is what a phosphor
	// does, and the INTENSITY knob sets how fast it gives up: a bright beam
	// on a long-persistence tube holds several sweeps at once.
	fade := 0.12 + 0.5*(1-scopeUI.intens)
	scopeCtx.Set("globalAlpha", fade)
	scopeCtx.Set("fillStyle", "#05070a")
	scopeCtx.Call("fillRect", 0, 0, w, h)
	scopeCtx.Set("globalAlpha", 1.0)

	drawScopeFaceGrat(w, h)
	drawScopeTrace(w, h)
}

// scopeVisible reports whether the tube is on screen at all: the module
// exists and the rack has not switched it out.

// drawScopeFaceGrat draws the etched face — the same figure the model's
// graticule uses, so the two are one instrument.

func snapHalf(v float64) float64 { return float64(int(v)) + 0.5 }

// drawScopeTrace captures the live audio and sweeps it across the face.
func drawScopeTrace(w, h float64) {
	src := ensureAudioSource()
	if src == nil || !src.Ready() {
		return
	}
	sr := src.SampleRate()
	if sr <= 0 {
		sr = 24000
	}
	span := scopeSweepSamples(scopeSecPerDiv(), sr)
	// A margin behind the window for the trigger to search in — one screen's
	// worth, so an edge anywhere in the last two screens can be found.
	need := span * 2
	if len(scopeSampL) < need {
		scopeSampL = make([]float32, need+need/2)
		scopeSampR = make([]float32, len(scopeSampL))
	}
	l, r := scopeSampL[:need], scopeSampR[:need]
	src.TimeDomainStereo(l, r)

	vert := scopeVertical(l, r)
	start := span // the newest whole window, which is what a free run shows
	if i := scopeTriggerIndex(vert, float32(scopeUI.trigLvl), scopeUI.rising, span); i >= 0 {
		start = i
	} else if !scopeUI.trigAuto {
		// NORM: no edge, no sweep. The tube keeps whatever was on it and
		// fades, which is exactly what a scope waiting for a trigger does.
		return
	}
	if start+span > len(vert) {
		start = len(vert) - span
	}
	if start < 0 {
		start = 0
	}

	px := w / float64(gratDivX)
	py := h / float64(gratDivY)
	cx, cy := w/2, h/2
	vpd := scopeVoltsPerDiv()

	// The beam. shadowBlur is the halo a real spot has; FOCUS tightens both
	// the line and the halo, which is what the knob does on the tube.
	line := 1.0 + 2.2*(1-scopeUI.focus)
	scopeCtx.Set("lineWidth", line)
	scopeCtx.Set("lineJoin", "round")
	scopeCtx.Set("lineCap", "round")
	scopeCtx.Set("shadowBlur", 4+10*(1-scopeUI.focus))
	scopeCtx.Set("shadowColor", "rgba(120,255,170,0.9)")
	scopeCtx.Set("strokeStyle", scopeBeamColor())
	// Built as ONE path string and handed over once. A moveTo/lineTo per
	// sample is a Go/JS boundary crossing per sample, which at a few
	// hundred samples a frame is the bulk of what this instrument costs.
	scopeTracePath.Reset()

	if scopeUI.chanSel == scopeChanXY {
		// X-Y: the horizontal comes off the other channel and the timebase is
		// out of circuit entirely. This is the goniometer, made the way a
		// scope makes one.
		for i := 0; i < span && start+i < len(l); i++ {
			x := cx + scopeYDiv(l[start+i], vpd, scopeUI.hpos)*px
			y := cy - scopeYDiv(r[start+i], vpd, scopeUI.vpos)*py
			scopeLineTo(i == 0, x, y)
		}
	} else {
		// The sweep: one screen width in span samples, so x is the fraction
		// of the way across and y is the deflection.
		for i := 0; i < span && start+i < len(vert); i++ {
			frac := float64(i) / float64(span-1)
			x := cx + (frac-0.5+scopeUI.hpos/float64(gratDivX))*w
			y := cy - scopeYDiv(vert[start+i], vpd, scopeUI.vpos)*py
			scopeLineTo(i == 0, x, y)
		}
	}
	if p := js.Global().Get("Path2D"); p.Truthy() {
		scopeCtx.Call("stroke", p.New(scopeTracePath.String()))
	}
	scopeCtx.Set("shadowBlur", 0)
}

// scopeVertical is the signal on the vertical axis, per the SOURCE switch.
// X-Y has no single vertical — it uses both channels directly — so it reads
// as CH 1 here and the caller takes the other branch.
func scopeVertical(l, r []float32) []float32 {
	switch scopeUI.chanSel {
	case scopeChanR:
		return r
	case scopeChanMid:
		// Summed into the left buffer's tail is not safe (the caller still
		// wants l for X-Y), so mid gets its own.
		if len(scopeMidBuf) < len(l) {
			scopeMidBuf = make([]float32, len(l))
		}
		m := scopeMidBuf[:len(l)]
		for i := range l {
			m[i] = (l[i] + r[i]) * 0.5
		}
		return m
	default:
		return l
	}
}

var scopeMidBuf []float32

// scopeBeamColor is the phosphor. P31 green by default, and it follows the
// rack's own phosphor selection when one is set, so the scope in the rack
// and the scope look on the model are the same tube.
func scopeBeamColor() string {
	if phosphorIdx > 0 && phosphorIdx < len(phosphors) {
		p := phosphors[phosphorIdx]
		return phColorCSS(p.tr, p.tg, p.tb)
	}
	// P31, the Tektronix standard, when nothing else is chosen: this tube
	// has a phosphor whether or not the rack's CRT look is switched on.
	p := phosphors[1]
	return phColorCSS(p.tr, p.tg, p.tb)
}

// The face is drawn from three cached Path2D objects, one per line weight.
//
// It used to walk the graticule and issue a moveTo and a lineTo per line —
// about three hundred crossings of the Go/JS boundary every frame, on top of
// rebuilding the figure three times. syscall/js pays for each of those
// crossings, and together they cost the model a third of its frame rate and
// a visible hitch about once a second. A Path2D is built once and handed to
// stroke, so a frame is three calls instead of three hundred.
var (
	scopeGratPaths [3]js.Value
	scopeGratW     float64
	scopeGratH     float64
)

// scopeGratWeights is the order the face is drawn in: ticks first, so the
// heavier lines land on top of them where they cross.
var scopeGratWeights = [3]gratWeight{gratWeightTick, gratWeightDiv, gratWeightAxis}

// scopeGratStroke is how each weight is painted. A real graticule is not one
// uniform grid — the center axes are cut heavier than the division lines and
// the ticks are hairlines — and drawing them alike is what makes a rendered
// one look like a spreadsheet.
var scopeGratStroke = [3]struct {
	color string
	width float64
}{
	{"rgba(120,155,185,0.20)", 1.0},
	{"rgba(120,155,185,0.30)", 1.0},
	{"rgba(150,190,220,0.55)", 1.2},
}

// buildScopeGratPaths rebuilds the three paths for a canvas of this size.
func buildScopeGratPaths(w, h float64) {
	p2d := js.Global().Get("Path2D")
	if !p2d.Truthy() {
		return
	}
	px := w / float64(gratDivX)
	py := h / float64(gratDivY)
	cx, cy := w/2, h/2
	var b strings.Builder
	for i, weight := range scopeGratWeights {
		b.Reset()
		for _, l := range scopeGraticule() {
			if l.W != weight {
				continue
			}
			// +0.5 so a one-pixel line lands ON a pixel instead of across two
			// of them, which is the difference between a ruled face and a
			// blurred one.
			b.WriteString("M")
			appendNum(&b, snapHalf(cx+float64(l.X0)*px))
			b.WriteString(" ")
			appendNum(&b, snapHalf(cy-float64(l.Y0)*py))
			b.WriteString("L")
			appendNum(&b, snapHalf(cx+float64(l.X1)*px))
			b.WriteString(" ")
			appendNum(&b, snapHalf(cy-float64(l.Y1)*py))
		}
		scopeGratPaths[i] = p2d.New(b.String())
	}
	scopeGratW, scopeGratH = w, h
}

// drawScopeFaceGrat strokes the face.
func drawScopeFaceGrat(w, h float64) {
	if scopeGratW != w || scopeGratH != h || !scopeGratPaths[0].Truthy() {
		buildScopeGratPaths(w, h)
	}
	for i := range scopeGratWeights {
		p := scopeGratPaths[i]
		if !p.Truthy() {
			continue
		}
		scopeCtx.Set("strokeStyle", scopeGratStroke[i].color)
		scopeCtx.Set("lineWidth", scopeGratStroke[i].width)
		scopeCtx.Call("stroke", p)
	}
}

// appendNum writes a coordinate with one decimal, which is as fine as a
// canvas path needs and keeps the string short.
func appendNum(b *strings.Builder, v float64) {
	b.WriteString(strconv.FormatFloat(v, 'f', 1, 64))
}

// scopeTracePath is the sweep, accumulated as an SVG path. Reused between
// frames: it is rewritten sixty times a second and a fresh builder each
// time is garbage the collector has to come back for.
var scopeTracePath strings.Builder

// scopeLineTo appends one point to the trace.
func scopeLineTo(first bool, x, y float64) {
	if first {
		scopeTracePath.WriteString("M")
	} else {
		scopeTracePath.WriteString("L")
	}
	appendNum(&scopeTracePath, x)
	scopeTracePath.WriteString(" ")
	appendNum(&scopeTracePath, y)
}

// scopeVisible reports whether the tube is on screen at all.
//
// Asked a few times a second rather than every frame. The answer needs
// offsetParent, and reading that forces the browser to settle style and
// layout before it can reply — sixty of those a second, interleaved with
// the model's own rendering, is a cost paid on every frame whether the
// scope is in the rack or not. It changes when a switch is flipped or the
// panel is re-laid-out, which is nowhere near frame rate.
var (
	scopeVisCached bool
	scopeVisAt     float64
)

// scopeVisEveryMs is how stale the answer is allowed to get. A quarter of a
// second is four layout reads a second instead of sixty, and is far below
// the time it takes to notice a panel has changed.
const scopeVisEveryMs = 250

func scopeVisible() bool {
	if frameNowMs-scopeVisAt < scopeVisEveryMs && scopeVisAt != 0 {
		return scopeVisCached
	}
	scopeVisAt = frameNowMs
	scopeVisCached = scopeOnScreen()
	return scopeVisCached
}

// scopeOnScreen is the real answer, measured.
func scopeOnScreen() bool {
	p := doc.Call("getElementById", "scope-panel")
	if !p.Truthy() {
		return false
	}
	// offsetParent is null for anything inside a hidden ancestor, which is
	// how a unit taken out of the frame, a collapsed drawer or a put-away
	// rack all read here.
	return p.Get("offsetParent").Truthy()
}

// invalidateScopeVisible forces the next frame to measure again, for the
// moments when the answer has just changed and waiting a quarter second to
// notice would be visible.
func invalidateScopeVisible() { scopeVisAt = 0 }
