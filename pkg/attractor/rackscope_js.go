//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/scope"
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

// rackScope is one of the rack's scopes: its canvas, the trace and graticule
// it draws, and the sample windows.
type rackScope struct {
	// p is its id prefix (scopePrefix), and n which scope it is, from 0.
	p string
	n int

	// funcs is the panel's own js.Func arena: its dials are built once,
	// on a different schedule from the parameter panel's.
	funcs []js.Func

	// ui is the panel. Defaults are a scope you could hand to someone: a
	// range that fits a normalized signal, a sweep slow enough to see a waveform
	// on, auto trigger so silence still draws a baseline, and the beam on.
	ui scopeState

	// power is the tube's switch, SCALE ILLUM's OFF detent.
	power screenPower

	// canvas and ctx are the tube, looked up once.
	canvas js.Value
	ctx    js.Value

	// sampL and sampR are the capture buffers, grown rather than
	// reallocated: the slowest timebase is half a second a division, which is
	// five seconds of audio across the screen.
	sampL, sampR []float32
	midBuf       []float32

	// The face is drawn from three cached Path2D objects, one per line weight.
	//
	// It used to walk the graticule and issue a moveTo and a lineTo per line —
	// about three hundred crossings of the Go/JS boundary every frame, on top of
	// rebuilding the figure three times. syscall/js pays for each of those
	// crossings, and together they cost the model a third of its frame rate and
	// a visible hitch about once a second. A Path2D is built once and handed to
	// stroke, so a frame is three calls instead of three hundred.
	gratPaths [3]js.Value
	gratW     float64
	gratH     float64

	// The sweep, as x,y pairs. Both reused between frames: they are rewritten
	// sixty times a second and a fresh allocation each time is garbage the
	// collector has to come back for.
	tracePts []float32 // the points handed to the canvas
	envBuf   []float32 // the column min/max pairs they are built from
}

// id is the scope's own id for a part the first scope's markup calls
// "scope-"+part.
func (ra *rackScope) id(part string) string { return ra.p + "-" + part }

// The VOLTS/DIV and TIME/DIV switches' positions at power-on, and where their
// resets put them.
var (
	scopeVoltsDef = scope.NearestStep(scope.VoltsDivs, 0.5)
	scopeTimeDef  = scope.NearestStep(scope.Timebases, 2e-3)
)

// rscopes are the rack's scopes, in order.
var rscopes = func() (s [rackScopeCount]*rackScope) {
	for n := range s {
		ra := &rackScope{
			p: scopePrefix(n), n: n,
			ui: scopeState{
				voltsIdx: scopeVoltsDef,
				timeIdx:  scopeTimeDef,
				rising:   true,
				trigAuto: true,
				power:    true,
				illum:    0.5,
				intens:   0.6,
				focus:    scope.FocusBest,
				in:       scopeInDefaults[n],
			},
		}
		ra.power = screenPower{powered: func() bool { return ra.ui.power }}
		s[n] = ra
	}
	return s
}()

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
	power    bool    // SCALE ILLUM out of its OFF detent: the scope is on
	illum    float64 // SCALE ILLUM: how brightly the graticule is lit, 0–1
	intens   float64 // INTENSITY: beam brightness and afterglow
	focus    float64 // FOCUS: spot tightness
	chanSel  int     // SOURCE: which signal is on the vertical
	in       [2]int  // INPUT 1 and 2: what each channel is fed from (scopeInNames)
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

func (ra *rackScope) secPerDiv() float64 {
	if len(scope.Timebases) == 0 {
		return 1e-3
	}
	return scope.Timebases[clampIdx(ra.ui.timeIdx, len(scope.Timebases))]
}

func (ra *rackScope) voltsPerDiv() float64 {
	if len(scope.VoltsDivs) == 0 {
		return 0.5
	}
	return scope.VoltsDivs[clampIdx(ra.ui.voltsIdx, len(scope.VoltsDivs))]
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

// buildRackScopes fills every scope's dials and wires them. Called once,
// after the control panel exists.
func buildRackScopes() {
	for _, ra := range rscopes {
		ra.buildRackScope()
	}
}

// buildRackScope fills the scope's dials and wires them.
func (ra *rackScope) buildRackScope() {
	dom.RebuildInto(&ra.funcs, func() {
		// The range switches, from the sequences themselves — a hand-typed
		// option list is a second copy of the spec, and the one that goes stale.
		buildScopeDial(ra.id("volts"), scope.VoltsDivs, scope.FormatVolts, ra.ui.voltsIdx, scopeVoltsDef,
			func(i int) { ra.ui.voltsIdx = i })
		buildScopeDial(ra.id("time"), scope.Timebases, scope.FormatTime, ra.ui.timeIdx, scopeTimeDef,
			func(i int) { ra.ui.timeIdx = i })

		wireScopeRange(ra.id("vpos"), func(v float64) { ra.ui.vpos = v })
		wireScopeRange(ra.id("hpos"), func(v float64) { ra.ui.hpos = v })
		wireScopeRange(ra.id("trig"), func(v float64) { ra.ui.trigLvl = v })
		ra.wireScopeBeam()
		ra.wireScopeInputs()

		// SLOPE and MODE are two-way settings, so they are buttons on the
		// trigger LEVEL's cell rather than a switch or a knob of their own:
		// the trigger's three settings in one place. Each column is its
		// parameter's position (trioOf), so two share a cell. SOURCE is four
		// buttons beside the tube, which is what it chooses the picture for,
		// between the H and V trimmers.
		cols := []struct{ id, in string }{
			{ra.id("slope"), "#" + ra.id("trig-cell")},
			{ra.id("tmode"), "#" + ra.id("trig-cell")},
			{ra.id("chan"), "#" + ra.id("chan-slot")},
		}
		for _, c := range cols {
			if in := dom.Doc.Call("querySelector", c.in); in.Truthy() {
				col := trioColumn(trioPrograms[c.id].legends())
				col.Call("setAttribute", "data-param", c.id)
				in.Call("appendChild", col)
			}
		}
		syncTrios()
	})
}

// wireScopeBeam is the first cell under the tube: INTENSITY, and under it on
// mini knobs SCALE ILLUM, a switch-pot whose OFF is the scope's power, and
// FOCUS. The three were one shaft, and could not be told apart.
func (ra *rackScope) wireScopeBeam() {
	illum := dom.Doc.Call("getElementById", ra.id("illum"))
	intens := dom.Doc.Call("getElementById", ra.id("intens"))
	focus := dom.Doc.Call("getElementById", ra.id("focus"))
	holder := dom.Doc.Call("getElementById", ra.id("intens-stack"))
	if !illum.Truthy() || !intens.Truthy() || !focus.Truthy() || !holder.Truthy() {
		return
	}
	lo, _ := strconv.ParseFloat(illum.Get("min").String(), 64) //nolint:errcheck // the markup's own number
	sp := switchPot{off: lo}
	switchPots[ra.id("illum")] = sp
	holder.Set("innerHTML", "")
	holder.Call("appendChild", makeKnob(intens, js.Undefined(), false, true, true))
	col := miniRow(miniKnob(illum, "I", true, true), miniKnob(focus, "F", true, true))
	if cell := holder.Call("closest", ".pcell"); cell.Truthy() {
		cell.Call("appendChild", col)
	}
	led := dom.Doc.Call("getElementById", ra.id("intens-led"))
	loan := lendReadout(col, led, func() {
		dom.Fire(intens, "input")
	})
	two := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	apply := func() {
		v, _ := strconv.ParseFloat(illum.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute
		v = sp.snap(v)
		power := !sp.isOff(v)
		ra.ui.illum = max(v, 0)
		if power != ra.ui.power {
			ra.ui.power = power
			ra.power.invalidate()
		}
		if power {
			loan.show("illum", two(v))
		} else {
			loan.show("off", "0")
		}
	}
	dom.On(illum, "input", func(js.Value, []js.Value) any {
		apply()
		return nil
	})
	// The cell's one reset puts all three back, as a generator's level reset
	// does its envelope.
	adoptDescControl(ControlDesc{
		ID: ra.id("focus"), Label: "focus", Min: 0, Max: 1, Step: 0.01, Def: scope.FocusBest,
		ResetID: "rst-" + ra.id("intens"),
		Apply: func(v float64) {
			ra.ui.focus = v
			loan.show("focus", two(v))
		},
	})
	adoptDescControl(ControlDesc{
		ID: ra.id("illum"), Label: "illum", Min: lo, Max: 1, Step: 0.01, Def: 0.5,
		ResetID: "rst-" + ra.id("intens"),
	})
	ra.ui.focus, _ = strconv.ParseFloat(focus.Get("value").String(), 64) //nolint:errcheck // the markup's own number
	apply()
	adoptDescControl(ControlDesc{
		ID: ra.id("intens"), Label: "intens", Min: 0, Max: 1, Step: 0.01, Def: 0.6,
		LEDStep: 0.1, LEDID: ra.id("intens-led"), ResetID: "rst-" + ra.id("intens"),
		Apply: func(v float64) { ra.ui.intens = v },
	})
	ra.ui.intens, _ = strconv.ParseFloat(intens.Get("value").String(), 64) //nolint:errcheck // the markup's own number
}

// The SLOPE, MODE and SOURCE buttons, every scope's.
func init() {
	for _, ra := range rscopes {
		ra.registerTrios()
	}
}

func (ra *rackScope) registerTrios() {
	trioPrograms[ra.id("slope")] = trioProgram{
		keys: []string{"+", "−"},
		help: []string{
			"slope: the sweep starts on the signal rising through the trigger level",
			"slope: the sweep starts on the signal falling through the trigger level",
		},
		press: func(i int) { ra.ui.rising = i == 0 },
		lit: func() int {
			if ra.ui.rising {
				return 0
			}
			return 1
		},
	}
	trioPrograms[ra.id("chan")] = trioProgram{
		keys: scopeChanNames,
		help: []string{
			"the beam draws channel 1, what input 1 feeds it",
			"the beam draws channel 2, what input 2 feeds it",
			"the beam draws the two channels summed",
			"the beam draws channel 1 against channel 2 instead of against time, which is a goniometer",
		},
		press: func(i int) { ra.ui.chanSel = i },
		lit:   func() int { return ra.ui.chanSel },
	}
	trioPrograms[ra.id("tmode")] = trioProgram{
		keys: []string{"auto", "norm"},
		help: []string{
			"sweep mode: sweeps anyway when nothing crosses the level, so silence shows a baseline",
			"sweep mode: sweeps only on a trigger, and holds the last trace otherwise",
		},
		press: func(i int) { ra.ui.trigAuto = i == 0 },
		lit: func() int {
			if ra.ui.trigAuto {
				return 0
			}
			return 1
		},
	}
}

// wireScopeInputs is the two INPUT mini knobs under VOLTS: a detent per
// source, ticks round them to count by, and the source each is on named on
// the face, where the eye already is (drawInputNames). Not on the cell's
// display as the beam's minis are: its tall lent legend runs into the VOLTS
// ring's labels, which the beam's knob does not have. The cell's reset puts them back with VOLTS, as the
// beam's puts its minis back with INTENSITY.
func (ra *rackScope) wireScopeInputs() {
	volts := dom.Doc.Call("getElementById", ra.id("volts"))
	cell := volts.Call("closest", ".pcell")
	if !volts.Truthy() || !cell.Truthy() {
		return
	}
	tips := make([]string, scopeInCount)
	for i, name := range scopeInNames {
		tips[i] = strconv.Itoa(i) + " " + name + ": " + scopeInHelp(i)
	}
	var knobs, els []js.Value
	for ch := range 2 {
		el := dom.Doc.Call("getElementById", ra.id("in"+strconv.Itoa(ch+1)))
		if !el.Truthy() {
			return
		}
		el.Set("max", scopeInCount-1)
		el.Set("value", scopeInDefaults[ra.n][ch])
		el.Set("title", el.Get("title").String()+"\n\n"+strings.Join(tips, "\n"))
		els = append(els, el)
		knobs = append(knobs, miniKnob(el, strconv.Itoa(ch+1), true, true))
	}
	cell.Call("appendChild", miniRow(knobs...))
	for ch, el := range els {
		def := float64(scopeInDefaults[ra.n][ch])
		adoptDescControl(ControlDesc{
			ID: el.Get("id").String(), Label: "in " + strconv.Itoa(ch+1),
			Min: 0, Max: scopeInCount - 1, Step: 1, Def: def,
			ResetID: "rst-" + ra.id("volts"), PermaKey: scopeInKey(ra.n, ch),
			Apply: func(v float64) {
				ra.ui.in[ch] = clampIdx(int(v+0.5), scopeInCount)
			},
		})
	}
}

// scopeLabel is a scope control's name without its scope: "volts" for
// scope3-volts.
func scopeLabel(id string) string { return id[strings.IndexByte(id, '-')+1:] }

// buildScopeDial rings a detented range switch, labeled with the values it
// actually selects: in full in the window, and round the skirt as numbers
// with each band's unit once (scope.SkirtLabels).
func buildScopeDial(id string, steps []float64, label func(float64) string, at, def int, set func(int)) {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = label(s)
	}
	buildScopeRing(id, names, scope.SkirtLabels(steps, label), at, def, set)
}

// buildScopeRing rings a detented switch over its select: names are the
// options and the window, ring what is printed round the knob. It goes
// through soloKnob and addSelectorLabels, so a scope knob is the same object
// as every other knob in the rack — it turns the same way, scrolls the same
// way, and is the same size.
func buildScopeRing(id string, names, ring []string, at, def int, set func(int)) {
	sel := dom.Doc.Call("getElementById", id)
	holder := dom.Doc.Call("getElementById", id+"-stack")
	if !sel.Truthy() || !holder.Truthy() || len(names) == 0 || len(ring) != len(names) {
		return
	}
	sel.Set("innerHTML", "")
	for i, n := range names {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", n)
		sel.Call("appendChild", opt)
	}
	sel.Set("value", strconv.Itoa(clampIdx(at, len(names))))
	sel.Get("style").Set("display", "none")

	holder.Set("innerHTML", "")
	stack := soloKnob(sel)
	addSelectorLabels(stack, ring, sel).Set("id", id+"-ring")
	holder.Call("appendChild", stack)

	dom.On(sel, "change", func(js.Value, []js.Value) any {
		if n, err := strconv.Atoi(sel.Get("value").String()); err == nil {
			set(n)
			setScopeReadout(id, names, n)
		}
		return nil
	})
	setScopeReadout(id, names, clampIdx(at, len(names)))
	adoptDescControl(ControlDesc{
		ID: id, Label: scopeLabel(id), IsSelect: true,
		SelectDef: strconv.Itoa(clampIdx(def, len(names))), ResetID: "rst-" + id,
	})
}

// setScopeReadout puts the switch's current position in the window under
// the knob. A range switch prints every position around its skirt, which
// says what it COULD be set to; the window says what it IS, and on an
// instrument being read at a glance that is the one that matters.
//
// The window is a character display, the part every other worded readout on
// the rack is: the markup leaves a place for it, and the first call puts
// the display there.
func setScopeReadout(id string, names []string, i int) {
	el := dom.Doc.Call("getElementById", id+"-read")
	if !el.Truthy() || len(names) == 0 {
		return
	}
	if !el.Get("classList").Call("contains", "dmdwin").Bool() {
		d := dotDisplayN("", false, scopeWindowChars)
		d.Set("id", id+"-read")
		el.Call("replaceWith", d)
		el = d
	}
	setDotText(el, names[clampIdx(i, len(names))])
}

// scopeWindowChars is a scope window's size: a full display, which holds its
// longest setting, "500 mFS".
const scopeWindowChars = dispFullChars

// wireScopeRange turns a slider into a knob and reports its value.
//
// The knob goes in the stack span the markup leaves for it, in the cell a
// knob has in every module, and the cell is the generators' in every part:
// makeKnob with its tick ring, and the descriptor's readout, typed entry,
// wheel and reset, from the range the markup gives the slider.
func wireScopeRange(id string, set func(float64)) {
	el := dom.Doc.Call("getElementById", id)
	holder := dom.Doc.Call("getElementById", id+"-stack")
	if !el.Truthy() {
		return
	}
	if holder.Truthy() {
		holder.Set("innerHTML", "")
		if mini := holder.Call("closest", ".scope-mini"); mini.Truthy() {
			// A trimmer beside the tube: no scale, no fine ring, and no
			// display — where the trace is shows it.
			k := miniKnob(el, mini.Call("getAttribute", "data-cap").String(), true, false)
			k.Call("querySelector", ".knob").Set("title", mini.Get("title").String())
			holder.Call("appendChild", k)
		} else {
			holder.Call("appendChild", makeKnob(el, js.Undefined(), true, true, true))
		}
	}
	attr := func(a string) float64 {
		v, _ := strconv.ParseFloat(el.Call("getAttribute", a).String(), 64) //nolint:errcheck // the markup's own numbers; zero if one is ever missing
		return v
	}
	adoptDescControl(ControlDesc{
		ID: id, Label: scopeLabel(id),
		Min: attr("min"), Max: attr("max"), Step: attr("step"), Def: attr("value"),
		Signed: attr("min") < 0,
		// Two places: the finest of these knobs moves in hundredths.
		LEDStep: 0.1,
		LEDID:   id + "-led", ResetID: "rst-" + id,
		Apply: set,
	})
	set(attr("value"))
}

// ── the tube ────────────────────────────────────────────────────────────

// scopeBus is this frame's window on the rack's signal and the capture,
// taken once for all four scopes (audiosrc.Bus.Window): two scopes on one
// source draw the same signal, and reading it does not move the rack's
// signal on.
var scopeBus struct {
	win   audiosrc.BusWindow
	sr    int          // the rate the scopes sweep at
	model [3][]float32 // the model's outputs, for a scope probing one
}

// drawRackScopes paints a frame of every scope. Called from the render loop.
func drawRackScopes() {
	b := aud.ensureAudioSource().(*audiosrc.Bus)
	scopeBus.sr = b.SampleRate()
	need := 0
	for _, ra := range rscopes {
		if !ra.ui.power {
			continue
		}
		for _, in := range ra.ui.in {
			if in <= scopeInCapR {
				need = max(need, 2*scope.SweepSamples(ra.secPerDiv(), scopeBus.sr))
			}
		}
	}
	if need > 0 {
		scopeBus.win = b.Window(need)
	}
	for _, ra := range rscopes {
		ra.drawRackScope()
	}
}

// drawRackScope paints one frame of the scope's screen.
func (ra *rackScope) drawRackScope() {
	// Powered by the OFF detent of its SCALE ILLUM knob. Off, the
	// tube is painted dark ONCE and then costs nothing — no capture, no path, no
	// layout read. The rack shows every module in a bay now, so "nobody
	// can see it" has stopped being what turns this off.
	if !ra.power.on(ra.canvasEl()) {
		if ra.power.needsBlank() && ra.blankFace() {
			ra.power.markBlanked()
		}
		return
	}
	if !ra.ctx.Truthy() {
		ra.canvas = dom.Doc.Call("getElementById", ra.id("screen"))
		if !ra.canvas.Truthy() {
			return
		}
		ra.ctx = ra.canvas.Call("getContext", "2d")
		if !ra.ctx.Truthy() {
			return
		}
	}
	w := ra.canvas.Get("width").Float()
	h := ra.canvas.Get("height").Float()
	if !(w > 0 && h > 0) {
		return
	}

	// The afterglow. Painting over rather than clearing is what a phosphor
	// does, and the INTENSITY knob sets how fast it gives up: a bright beam
	// on a long-persistence tube holds several sweeps at once.
	fade := 0.12 + 0.5*(1-ra.ui.intens)
	ra.ctx.Set("globalAlpha", fade)
	ra.ctx.Set("fillStyle", "#05070a")
	ra.ctx.Call("fillRect", 0, 0, w, h)
	ra.ctx.Set("globalAlpha", 1.0)

	ra.drawScopeFaceGrat(w, h, ra.ui.illum)
	ra.drawScopeTrace(w, h)
	ra.drawInputNames(h)
}

// drawInputNames prints what the two channels are fed from at the top left
// of the face, the way a scope with on-screen readouts prints its settings:
// in the beam's color, and over the afterglow, so it holds still while the
// trace fades under it.
func (ra *rackScope) drawInputNames(h float64) {
	px := h / 13
	ra.ctx.Set("font", strconv.FormatFloat(px, 'f', 0, 64)+"px ui-monospace, monospace")
	ra.ctx.Set("textBaseline", "top")
	ra.ctx.Set("fillStyle", scopeBeamColor())
	ra.ctx.Set("globalAlpha", 0.35+0.5*ra.ui.intens)
	txt := "1 " + scopeInNames[ra.ui.in[0]] + "   2 " + scopeInNames[ra.ui.in[1]]
	ra.ctx.Call("fillText", txt, px*0.6, px*0.5)
	ra.ctx.Set("globalAlpha", 1.0)
}

func snapHalf(v float64) float64 { return float64(int(v)) + 0.5 }

// drawScopeTrace takes the two inputs and sweeps them across the face.
func (ra *rackScope) drawScopeTrace(w, h float64) {
	sr := scopeBus.sr
	span := scope.SweepSamples(ra.secPerDiv(), sr)
	// A margin behind the window for the trigger to search in — one screen's
	// worth, so an edge anywhere in the last two screens can be found.
	need := span * 2
	if len(ra.sampL) < need {
		ra.sampL = make([]float32, need+need/2)
		ra.sampR = make([]float32, len(ra.sampL))
	}
	l, r := ra.sampL[:need], ra.sampR[:need]
	if !ra.feed(ra.ui.in[0], l) || !ra.feed(ra.ui.in[1], r) {
		return
	}

	vert := ra.vertical(l, r)
	start := span // the newest whole window, which is what a free run shows
	if i := scope.TriggerIndex(vert, float32(ra.ui.trigLvl), ra.ui.rising, span); i >= 0 {
		start = i
	} else if !ra.ui.trigAuto {
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

	px := w / float64(scope.DivX)
	py := h / float64(scope.DivY)
	cx, cy := w/2, h/2
	vpd := ra.voltsPerDiv()

	// The beam. shadowBlur is the halo a real spot has; FOCUS tightens both
	// the line and the halo, sharpest part-way round its travel and spreading
	// either side of it, as a tube's does (scope.Defocus).
	blur := scope.Defocus(ra.ui.focus)
	line := 1.0 + 2.2*blur
	ra.ctx.Set("lineWidth", line)
	ra.ctx.Set("lineJoin", "round")
	ra.ctx.Set("lineCap", "round")
	// INTENSITY is how bright the beam is, spot and halo alike: a dim trace
	// at the bottom of the knob, full at the top. It used to set only how long
	// the afterglow held, which barely changed the trace itself.
	ra.ctx.Set("globalAlpha", 0.12+0.88*ra.ui.intens)
	ra.ctx.Set("shadowBlur", (4+10*blur)*(0.25+0.75*ra.ui.intens))
	ra.ctx.Set("shadowColor", "rgba(120,255,170,0.9)")
	ra.ctx.Set("strokeStyle", scopeBeamColor())
	// Collected as x,y pairs and handed over in ONE crossing. A moveTo/lineTo
	// per sample is a Go/JS boundary crossing per sample, which is what this
	// used to avoid by building a path string instead — but the formatting
	// cost more than the crossings did. See scopefast_js.go.
	ra.tracePts = ra.tracePts[:0]

	switch {
	case ra.ui.chanSel == scopeChanXY:
		// X-Y: the horizontal comes off the other channel and the timebase is
		// out of circuit entirely. This is the goniometer, made the way a
		// scope makes one.
		//
		// No column envelope here, and there cannot be one: the figure is not
		// a function of x, so "the samples in this column" is not a slice of
		// it. A Lissajous pattern reduced to one vertical bar per column is a
		// different figure.
		for i := 0; i < span && start+i < len(l); i++ {
			x := cx + scope.YDiv(l[start+i], vpd, ra.ui.hpos)*px
			y := cy - scope.YDiv(r[start+i], vpd, ra.ui.vpos)*py
			ra.tracePts = append(ra.tracePts, float32(x), float32(y))
		}
	case span > 2*scope.TraceCols(w):
		// More samples than the face has columns: draw the envelope, which is
		// what the dense trace looks like anyway. See pkg/scope/trace.go.
		cols := scope.TraceCols(w)
		if len(ra.envBuf) < cols*2 {
			ra.envBuf = make([]float32, cols*2)
		}
		seg := vert[start:]
		if span < len(seg) {
			seg = seg[:span]
		}
		n := scope.TraceEnvelope(ra.envBuf, seg, cols)
		for c := range n {
			frac := float64(c) / float64(n-1)
			x := float32(cx + (frac-0.5+ra.ui.hpos/float64(scope.DivX))*w)
			// The lowest sample in the column is the lowest point on the
			// screen, the deflection being affine in the sample value.
			lo := float32(cy - scope.YDiv(ra.envBuf[c*2], vpd, ra.ui.vpos)*py)
			hi := float32(cy - scope.YDiv(ra.envBuf[c*2+1], vpd, ra.ui.vpos)*py)
			// Alternate which end the column is entered from, so the join to
			// the next one runs along the edge of the band rather than back
			// across it. Same figure, half the diagonal.
			if c%2 == 0 {
				ra.tracePts = append(ra.tracePts, x, lo, x, hi)
			} else {
				ra.tracePts = append(ra.tracePts, x, hi, x, lo)
			}
		}
	default:
		// The sweep: one screen width in span samples, so x is the fraction
		// of the way across and y is the deflection.
		for i := 0; i < span && start+i < len(vert); i++ {
			frac := float64(i) / float64(span-1)
			x := cx + (frac-0.5+ra.ui.hpos/float64(scope.DivX))*w
			y := cy - scope.YDiv(vert[start+i], vpd, ra.ui.vpos)*py
			ra.tracePts = append(ra.tracePts, float32(x), float32(y))
		}
	}
	strokeScopePoints(ra.ctx, ra.tracePts)
	ra.ctx.Set("shadowBlur", 0)
	ra.ctx.Set("globalAlpha", 1.0)
}

// vertical is the signal on the vertical axis, per the SOURCE switch.
// X-Y has no single vertical — it uses both channels directly — so it reads
// as CH 1 here and the caller takes the other branch.
func (ra *rackScope) vertical(l, r []float32) []float32 {
	switch ra.ui.chanSel {
	case scopeChanR:
		return r
	case scopeChanMid:
		// Summed into the left buffer's tail is not safe (the caller still
		// wants l for X-Y), so mid gets its own.
		if len(ra.midBuf) < len(l) {
			ra.midBuf = make([]float32, len(l))
		}
		m := ra.midBuf[:len(l)]
		for i := range l {
			m[i] = (l[i] + r[i]) * 0.5
		}
		return m
	default:
		return l
	}
}

// feed fills dst with what input in carries, the newest sample last.
func (ra *rackScope) feed(in int, dst []float32) bool {
	switch {
	case in <= scopeInCapR:
		w := [...][]float32{scopeBus.win.L, scopeBus.win.R, scopeBus.win.CapL, scopeBus.win.CapR}[in]
		if len(w) < len(dst) {
			return false
		}
		copy(dst, w[len(w)-len(dst):])
	case in >= scopeInGen1 && in < scopeInGen1+audiosrc.OscCount:
		// A probe on the generator's own output: nothing while its knob
		// is at OFF, whatever it was last playing.
		aud.fg().RenderOsc(in-scopeInGen1, dst)
	default:
		// A probe on Model Out's output, as on a generator's: the scopes'
		// copy of the model, the same window every scope on it draws.
		w := &scopeBus.model
		busModel(w, len(dst), false)
		copy(dst, w[in-scopeInModelX])
	}
	return true
}

// scopeBeamColor is the phosphor. P31 green by default, and it follows the
// rack's own phosphor selection when one is set, so the scope in the rack
// and the scope look on the model are the same tube.
func scopeBeamColor() string {
	if phos.index > 0 && phos.index < len(phosphors) {
		p := phosphors[phos.index]
		return phColorCSS(p.tr, p.tg, p.tb)
	}
	// P31, the Tektronix standard, when nothing else is chosen: this tube
	// has a phosphor whether or not the rack's CRT look is switched on.
	p := phosphors[1]
	return phColorCSS(p.tr, p.tg, p.tb)
}

// scopeGratWeights is the order the face is drawn in: ticks first, so the
// heavier lines land on top of them where they cross.
var scopeGratWeights = [3]scope.Weight{scope.WeightTick, scope.WeightDiv, scope.WeightAxis}

// scopeGratStroke is how each weight is painted. A real graticule is not one
// uniform grid — the center axes are cut heavier than the division lines and
// the ticks are hairlines — and drawing them alike is what makes a rendered
// one look like a spreadsheet.
//
// These are the face fully lit. SCALE ILLUM dims all three together, and its
// default, halfway, is the face as it was before the knob existed.
var scopeGratStroke = [3]struct {
	color string
	width float64
}{
	{"rgba(120,155,185,0.40)", 1.0},
	{"rgba(120,155,185,0.60)", 1.0},
	{"rgba(150,190,220,1.00)", 1.2},
}

// scopeGratUnlit is how the face shows with the scope off: the lines are
// cut into the glass, and they are still there, faintly, with no light
// behind them.
const scopeGratUnlit = 0.25

// buildScopeGratPaths rebuilds the three paths for a canvas of this size.
func (ra *rackScope) buildScopeGratPaths(w, h float64) {
	p2d := js.Global().Get("Path2D")
	if !p2d.Truthy() {
		return
	}
	px := w / float64(scope.DivX)
	py := h / float64(scope.DivY)
	cx, cy := w/2, h/2
	var b strings.Builder
	for i, weight := range scopeGratWeights {
		b.Reset()
		for _, l := range scope.Graticule() {
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
		ra.gratPaths[i] = p2d.New(b.String())
	}
	ra.gratW, ra.gratH = w, h
}

// drawScopeFaceGrat strokes the face — the same figure the model's graticule
// uses, so the two are one instrument — lit to lit, 0 to 1.
func (ra *rackScope) drawScopeFaceGrat(w, h, lit float64) {
	if ra.gratW != w || ra.gratH != h || !ra.gratPaths[0].Truthy() {
		ra.buildScopeGratPaths(w, h)
	}
	ra.ctx.Set("globalAlpha", lit)
	for i := range scopeGratWeights {
		p := ra.gratPaths[i]
		if !p.Truthy() {
			continue
		}
		ra.ctx.Set("strokeStyle", scopeGratStroke[i].color)
		ra.ctx.Set("lineWidth", scopeGratStroke[i].width)
		ra.ctx.Call("stroke", p)
	}
	ra.ctx.Set("globalAlpha", 1.0)
}

// appendNum writes a coordinate with one decimal, which is as fine as a
// canvas path needs and keeps the string short.
func appendNum(b *strings.Builder, v float64) {
	b.WriteString(strconv.FormatFloat(v, 'f', 1, 64))
}

// canvasEl is the tube's canvas, looked up lazily.
func (ra *rackScope) canvasEl() js.Value {
	if !ra.canvas.Truthy() {
		ra.canvas = dom.Doc.Call("getElementById", ra.id("screen"))
	}
	return ra.canvas
}

// blankFace paints the dark tube once, for a scope whose beam is off.
//
// A scope that is switched off should LOOK switched off — dark glass with
// the graticule still faintly etched on it, because the graticule is
// printed on the face and does not go anywhere when the beam does.
// Returns false if there is nothing to paint on yet, so the caller knows
// it still owes the blank.
func (ra *rackScope) blankFace() bool {
	c := ra.canvasEl()
	if !c.Truthy() {
		return false
	}
	if !ra.ctx.Truthy() {
		ra.ctx = c.Call("getContext", "2d")
		if !ra.ctx.Truthy() {
			return false
		}
	}
	w := c.Get("width").Float()
	h := c.Get("height").Float()
	if !(w > 0 && h > 0) {
		return false
	}
	ra.ctx.Set("globalAlpha", 1.0)
	ra.ctx.Set("fillStyle", "#05070a")
	ra.ctx.Call("fillRect", 0, 0, w, h)
	ra.drawScopeFaceGrat(w, h, scopeGratUnlit)
	return true
}
