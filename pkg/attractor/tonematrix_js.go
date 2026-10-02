//go:build js && wasm

package attractor

import (
	"html"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// The Tonematrix module: a pentatonic step sequencer — the drum-patterning
// slice of the modular-synth direction. A Window-group switch (Matrix) shows
// a module whose left column is standard cells (tempo, steps/root, level,
// out/voice) plus Run and Clear, and whose body is a grid of lit pads: 16
// pitch rows (major pentatonic, so any pattern consonates — the tonematrix
// trick) by 64 columns of sixteenths, the width of the row. The steps knob
// is where the loop ends: the playhead sweeps that many columns at the
// tempo, and the ones past it dim but stay, pads and all. Every lit pad in
// the column sounds as a short ping through the same master gain → panner
// chain the Keys module uses on the shared audio context, with the
// Gen-style out ring and waveform inner knob. Pads paint with click-drag or
// touch-drag.
//
// Under the grid are the drum lanes, and they are counted on their own
// subdivision of the SAME beat: four to it like the grid, or three when the
// preset is a shuffle or a swing. That is what lets a triplet beat play
// under a tune in sixteenths — the first version ran the whole grid at the
// preset's subdivision, so loading a shuffle put the tune in triplets too.
// A beat of lanes is drawn as wide as a beat of grid, whichever it holds,
// so the two line up by eye as well as by ear.

// tonematrix is the Tonematrix module: the cells, the pattern, the clock and
// the audio graph.
type tonematrix struct {
	on     bool
	run    bool
	ctx    js.Value // shared ctx while the lease is held
	master js.Value // master gain (the level), into the Mixer's MATRIX
	pat    [tmMaxSteps][tmRows]bool
	// The drum lanes, by lane step: perBeat of them to the beat, so a lane
	// step is a sixteenth or a triplet.
	drum [tmMaxSteps][tmLanes]bool
	// perBeat is the lanes' subdivision and beats the bar they count, for the
	// lamps: both the preset's (loadDrums).
	perBeat, beats int
	cols, dcols    []js.Value // the columns of the grid and of the lanes
	grid, lanes    js.Value   // what they are columns of
	step           int        // next column to schedule
	next           float64    // ctx time the next column sounds at
	phCol, dphCol  int        // grid and lane columns lit as the playhead

	// Scheduled-but-not-yet-sounding columns (audio runs ~a lookahead ahead
	// of the display; the playhead advances when a column's time arrives).
	// The lanes have their own, since their steps fall between the grid's.
	due, ddue []tmDueCol
	paint     int     // pad state being painted by the current drag (-1 = none)
	touchAt   float64 // performance.now() of the last pad touch — a tap's
}

var tm = tonematrix{
	run:     true,
	phCol:   -1,
	dphCol:  -1,
	paint:   -1,
	perBeat: tmPerBeat,
	beats:   4,
}

type tmDueCol struct {
	step int
	t    float64
}

const (
	tmRows     = 16
	tmLanes    = rhythm.VoiceCount // bass drum, snare, hi-hat, cymbal
	tmMaxSteps = 64
	tmPerBeat  = 4 // the grid's columns to the beat: sixteenths
	tmBeats    = tmMaxSteps / tmPerBeat
	tmLookah   = 0.12 // scheduling horizon, s (a few frames of jitter headroom)
)

// tmLoopSteps are where the steps knob can end the loop, in its order: whole
// beats, three to sixteen.
var tmLoopSteps = []int{12, 16, 24, 32, 48, 64}

// Major pentatonic — the tonematrix scale (5 notes per octave, no
// semitone clashes, so every pattern is consonant).
var tmPenta = [5]int{0, 2, 4, 7, 9}

func tmStepCount() int {
	n, _ := strconv.Atoi(dom.Doc.Call("getElementById", "tm-steps").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	if n < 1 || n > tmMaxSteps {
		n = 16
	}
	return n
}

// tmRootMidi reads the root knob: midi C of the chosen octave.
func tmRootMidi() int {
	oct, _ := strconv.Atoi(dom.Doc.Call("getElementById", "tm-root").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return 12 * (oct + 1)                                                                   // C1=24 … C4=60
}

// tmMidiFor maps a grid row (0 = top) to its midi note: pentatonic degrees
// stacked up from the root, highest note on the top row.
func tmMidiFor(row int) int {
	b := tmRows - 1 - row
	return tmRootMidi() + 12*(b/5) + tmPenta[b%5]
}

// ── Grid ─────────────────────────────────────────────────────────────────

// buildTMGrid (re)renders the pad grid and the drum lanes: column-major
// spans so the playhead is one class toggle per step. Every column is built,
// whatever the loop length; markLoop dims the ones past it. Pads keep their
// state across rebuilds (it lives in tm.pat and tm.drum, not the DOM).
//
// Written as one string of markup for each and listened to on their shared
// parent, not built pad by pad: there are thirteen hundred of them, and a pad
// made from Go with three listeners of its own cost about a millisecond
// apiece of the rack's start. A pad says which it is (data-tmc, data-tmr),
// and that is all a press needs.
func (to *tonematrix) buildTMGrid() {
	grid := dom.Doc.Call("getElementById", "tm-grid")
	lanes := dom.Doc.Call("getElementById", "tm-drums")
	if !grid.Truthy() || !lanes.Truthy() {
		return
	}
	// The rest of the row beside the two knob columns, whatever the loop.
	grid.Get("parentElement").Get("style").Set("width", rowRestWidth(2))
	to.setPH(-1)
	to.setDrumPH(-1)
	var rowName [tmRows]string
	for r := range tmRows {
		m := tmMidiFor(r)
		rowName[r] = noteNames[m%12] + strconv.Itoa(m/12-1)
	}
	var b strings.Builder
	for c := range tmMaxSteps {
		b.WriteString(tmColumnOpen(c, tmPerBeat))
		for r := range tmRows {
			b.WriteString(to.pad(c, r, "tm-cell", docf("tm-pad", "step", strconv.Itoa(c+1), "note", rowName[r])))
		}
		b.WriteString(`</span>`)
	}
	grid.Set("innerHTML", b.String())
	unit := "sixteenth"
	if to.perBeat == 3 {
		unit = "triplet"
	}
	b.Reset()
	for d := range tmBeats * to.perBeat {
		b.WriteString(tmColumnOpen(d, to.perBeat))
		for v := range tmLanes {
			b.WriteString(to.pad(d, tmRows+v, "tm-cell tm-drum "+tmLaneClass[v],
				docf("tm-drum-pad", "lane", tmLaneNames[v], "beat", strconv.Itoa(d/to.perBeat+1), "unit", unit, "n", strconv.Itoa(d%to.perBeat+1))))
		}
		b.WriteString(`</span>`)
	}
	lanes.Set("innerHTML", b.String())
	to.grid, to.lanes = grid, lanes
	to.cols = tmChildren(grid)
	to.dcols = tmChildren(lanes)
	if body := grid.Get("parentElement"); body.Truthy() && !body.Call("hasAttribute", "data-tm-wired").Bool() {
		body.Call("setAttribute", "data-tm-wired", "")
		to.wirePads(body)
	}
	to.markLoop()
}

// tmChildren is el's children, in order.
func tmChildren(el js.Value) []js.Value {
	cs := el.Get("children")
	out := make([]js.Value, cs.Length())
	for i := range out {
		out[i] = cs.Index(i)
	}
	return out
}

// tmPadAt is the pad an event happened on, as its step and row, and whether
// it was on one.
func tmPadAt(t js.Value) (c, r int, ok bool) {
	if !t.Truthy() || !t.Get("getAttribute").Truthy() {
		return 0, 0, false
	}
	cAttr, rAttr := t.Call("getAttribute", "data-tmc"), t.Call("getAttribute", "data-tmr")
	if !cAttr.Truthy() || !rAttr.Truthy() {
		return 0, 0, false
	}
	c, err1 := strconv.Atoi(cAttr.String())
	r, err2 := strconv.Atoi(rAttr.String())
	return c, r, err1 == nil && err2 == nil
}

// wirePads listens for presses and painting on every pad, from body, the
// parent the grid and the lanes share: a press toggles a pad and starts a
// paint, moving over a pad with the button down paints it, and a touch
// follows the finger (touchmove keeps targeting the pad it started on, so
// the pad under the finger is asked for, the keybed glissando pattern) and
// can cross from the grid to the lanes.
func (to *tonematrix) wirePads(body js.Value) {
	press := func(c, r int) {
		on := !to.isOn(c, r)
		to.setPad(c, r, on)
		to.paint = 0
		if on {
			to.paint = 1
		}
		to.ensureGraph() // user gesture: unlock audio for the loop
	}
	body.Call("addEventListener", "mousedown", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		c, r, ok := tmPadAt(a[0].Get("target"))
		if !ok {
			return nil
		}
		a[0].Call("preventDefault")
		if js.Global().Get("performance").Call("now").Float()-to.touchAt < 800 {
			return nil
		}
		press(c, r)
		return nil
	}))
	body.Call("addEventListener", "mouseover", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		if to.paint < 0 {
			return nil
		}
		c, r, ok := tmPadAt(a[0].Get("target"))
		if !ok {
			return nil
		}
		if int(a[0].Get("buttons").Float())&1 == 0 {
			to.paint = -1
			return nil
		}
		to.setPad(c, r, to.paint == 1)
		return nil
	}))
	body.Call("addEventListener", "touchstart", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		c, r, ok := tmPadAt(a[0].Get("target"))
		if !ok {
			return nil
		}
		a[0].Call("preventDefault")
		to.touchAt = js.Global().Get("performance").Call("now").Float()
		press(c, r)
		return nil
	}), map[string]any{"passive": false})
	body.Call("addEventListener", "touchmove", dom.FuncOf(func(_ js.Value, a []js.Value) any {
		if to.paint < 0 {
			return nil
		}
		e := a[0]
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		if c, r, ok := tmPadAt(dom.Doc.Call("elementFromPoint", t.Get("clientX").Float(), t.Get("clientY").Float())); ok {
			to.setPad(c, r, to.paint == 1)
		}
		return nil
	}), map[string]any{"passive": false})
	for _, ev := range []string{"touchend", "touchcancel"} {
		body.Call("addEventListener", ev, dom.FuncOf(func(js.Value, []js.Value) any {
			to.paint = -1
			return nil
		}))
	}
}

// tmColumnOpen opens one column of pads, with a breath before it when it
// starts a beat, as bar lines do.
func tmColumnOpen(c, perBeat int) string {
	if c > 0 && c%perBeat == 0 {
		return `<span class="tm-col tm-beat">`
	}
	return `<span class="tm-col">`
}

// pad is one pad's markup: step c of row r, where a row past the pitches is
// a drum lane and c counts that lane's steps.
func (to *tonematrix) pad(c, r int, cls, title string) string {
	if to.isOn(c, r) {
		cls += " on"
	}
	return `<span class=` + strconv.Quote(cls) + ` title="` + html.EscapeString(title) + `" data-tmc="` + strconv.Itoa(c) + `" data-tmr="` + strconv.Itoa(r) + `"></span>`
}

// loopBeats is how many beats the loop runs: the steps knob, in sixteenths.
func loopBeats() int { return max(1, tmStepCount()/tmPerBeat) }

// markLoop dims the columns past the loop, in the grid and in the lanes, and
// brings the playhead back inside it.
func (to *tonematrix) markLoop() {
	steps := tmStepCount()
	for c, col := range to.cols {
		col.Get("classList").Call("toggle", "tm-past", c >= steps)
	}
	lane := loopBeats() * to.perBeat
	for d, col := range to.dcols {
		col.Get("classList").Call("toggle", "tm-past", d >= lane)
	}
	if to.step >= steps {
		to.step = 0
	}
}

// tmLaneNames are the drum lanes, in rhythm's voice order.
var tmLaneNames = [tmLanes]string{"bass drum", "snare", "hi-hat", "cymbal"}

// tmLaneClass colors each lane (panel.css): written out, so the stylesheet's
// names can be found in the source.
var tmLaneClass = [tmLanes]string{"tm-lane-0", "tm-lane-1", "tm-lane-2", "tm-lane-3"}

// isOn is whether pad r of column c is lit: a pitch row, or below them a
// drum lane, where c is the lane's step.
func (to *tonematrix) isOn(c, r int) bool {
	if r >= tmRows {
		return to.drum[c][r-tmRows]
	}
	return to.pat[c][r]
}

func (to *tonematrix) setPad(c, r int, on bool) {
	if to.isOn(c, r) == on {
		return
	}
	cols, row := to.cols, r
	if r >= tmRows {
		to.drum[c][r-tmRows] = on
		cols, row = to.dcols, r-tmRows
	} else {
		to.pat[c][r] = on
	}
	if c < len(cols) {
		if el := cols[c].Get("children").Index(row); el.Truthy() {
			el.Get("classList").Call("toggle", "on", on)
		}
	}
}

// beatsPerBar is how many beats the bar is counted in, for the lamps.
func (to *tonematrix) beatsPerBar() int { return max(1, to.beats) }

// beatOf is the beat of the bar grid column c sounds on.
func (to *tonematrix) beatOf(c int) int {
	return (c / tmPerBeat) % to.beatsPerBar()
}

// tmLoopFor is the loop length a preset of this many beats to the bar wants:
// the current one if it holds whole bars, else the nearest that does.
func tmLoopFor(steps, beats int) int {
	if beats < 1 || (steps/tmPerBeat)%beats == 0 {
		return steps
	}
	best := steps
	for _, n := range tmLoopSteps {
		if (n/tmPerBeat)%beats != 0 {
			continue
		}
		if best == steps || abs(n-steps) < abs(best-steps) {
			best = n
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// loadDrums puts a rhythm preset into the drum lanes, its bar repeated
// across all of them, in the subdivision it is written in; and moves the
// loop to a whole number of its bars, from the top.
func (to *tonematrix) loadDrums(p rhythm.Pattern) {
	to.beats = rhythm.BeatsPerBar(p)
	perBeat := max(1, p.Steps/to.beats)
	if perBeat != to.perBeat && tmBeats*perBeat <= tmMaxSteps {
		to.perBeat = perBeat
		to.buildTMGrid() // a triplet beat has three lane columns, not four
	}
	for d := range tmBeats * to.perBeat {
		for v := range tmLanes {
			to.setPad(d, tmRows+v, rhythm.Hit(p, v, d))
		}
	}
	if sel := dom.Doc.Call("getElementById", "tm-steps"); sel.Truthy() {
		if n := tmLoopFor(tmStepCount(), to.beats); strconv.Itoa(n) != sel.Get("value").String() {
			sel.Set("value", strconv.Itoa(n))
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	to.markLoop()
	to.step, to.next = 0, 0
	to.due, to.ddue = to.due[:0], to.ddue[:0]
}

// setPH moves the playhead highlight to col (-1 = off).
func (to *tonematrix) setPH(col int) {
	to.phCol = movePH(to.cols, to.phCol, col)
}

// setDrumPH moves the lanes' playhead to lane step d (-1 = off).
func (to *tonematrix) setDrumPH(d int) {
	to.dphCol = movePH(to.dcols, to.dphCol, d)
}

// movePH moves the "ph" class from column from to column to, and returns to.
func movePH(cols []js.Value, from, to int) int {
	if from == to {
		return to
	}
	if from >= 0 && from < len(cols) {
		cols[from].Get("classList").Call("remove", "ph")
	}
	if to >= 0 && to < len(cols) {
		cols[to].Get("classList").Call("add", "ph")
	}
	return to
}

// ── Voice engine ─────────────────────────────────────────────────────────

// ensureGraph acquires the shared context (call from a user gesture so
// the autoplay policy lets it start) and lazily builds the master gain into
// the Mixer.
// The context it acquired is tm.ctx, which stays unset if the acquire failed.
func (to *tonematrix) ensureGraph() {
	ctx := acquireAudioCtx("tmx")
	if !ctx.Truthy() {
		return
	}
	rhy.ensureGraph() // the drum lanes' own chain, on the same context
	if !to.master.Truthy() {
		to.master = ctx.Call("createGain")
		mixEnsure(ctx)
		to.master.Call("connect", mixIn("tm"))
	}
	to.ctx = ctx
	to.updateRouting()
}

// updateRouting pushes the level knob into the master gain; where it goes is
// the Mixer's.
func (to *tonematrix) updateRouting() {
	if !to.master.Truthy() {
		return
	}
	lvl := fgFloat(dom.Doc.Call("getElementById", "tm-lvl")) / 100
	to.master.Get("gain").Set("value", lvl*0.3) // headroom for full columns
}

// stepDur returns one grid column's duration: a sixteenth, a quarter of a
// beat at the tempo knob's BPM, whatever the drums are counting in.
func (to *tonematrix) stepDur() float64 {
	bpm := fgFloat(dom.Doc.Call("getElementById", "tm-tempo"))
	if bpm < 40 {
		bpm = 120
	}
	return 60 / bpm / tmPerBeat
}

// scheduleCol sounds every lit pad in the column at ctx time t: a short
// ping (fast attack, exponential decay) per pad, fire-and-forget nodes.
func (to *tonematrix) scheduleCol(c int, t float64) {
	w, _ := strconv.Atoi(dom.Doc.Call("getElementById", "tm-wave").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	dur := to.stepDur() * 2
	if dur < 0.2 {
		dur = 0.2
	}
	if dur > 0.5 {
		dur = 0.5
	}
	for r := range tmRows {
		if !to.pat[c][r] {
			continue
		}
		hz := 440 * math.Pow(2, float64(tmMidiFor(r)-69)/12)
		var osc js.Value
		if w == 4 {
			// Noise pad: the DCSG shift-register loop pitched by playback rate
			// (snare/hat territory — higher rows = brighter bursts).
			osc = to.ctx.Call("createBufferSource")
			osc.Set("buffer", gen.noiseBuffer(to.ctx))
			osc.Set("loop", true)
			osc.Get("playbackRate").Set("value", hz*32/to.ctx.Get("sampleRate").Float())
		} else {
			osc = to.ctx.Call("createOscillator")
			osc.Set("type", waveTypeName(w))
			osc.Get("frequency").Set("value", hz)
		}
		g := to.ctx.Call("createGain")
		gg := g.Get("gain")
		gg.Call("setValueAtTime", 0, t)
		gg.Call("linearRampToValueAtTime", 1, t+0.006)
		gg.Call("exponentialRampToValueAtTime", 0.001, t+dur)
		osc.Call("connect", g)
		g.Call("connect", to.master)
		osc.Call("start", t)
		osc.Call("stop", t+dur+0.02)
	}
	// The drum lanes, a beat at a time: on the column that starts a beat, that
	// beat's lane steps, spread across it at the lanes' own subdivision. So a
	// triplet lands between the grid's columns, and the next beat starts both
	// together.
	if c%tmPerBeat != 0 {
		return
	}
	beat := to.stepDur() * tmPerBeat
	for k := range to.perBeat {
		d := c/tmPerBeat*to.perBeat + k
		td := t + beat*float64(k)/float64(to.perBeat)
		if rhy.ctx.Truthy() {
			for v := range tmLanes {
				if to.drum[d][v] {
					rhy.voice(v, td)
				}
			}
		}
		to.ddue = append(to.ddue, tmDueCol{d, td})
	}
}

// tick runs every frame from the render loop: schedule columns a small
// lookahead ahead of the audio clock, and advance the playhead as each
// scheduled column's time arrives. A long gap (hidden tab froze rAF)
// resynchronizes instead of racing to catch up.
func (to *tonematrix) tick() {
	if !to.on || !to.run || !to.ctx.Truthy() {
		return
	}
	now := to.ctx.Get("currentTime").Float()
	if to.next == 0 || to.next < now-0.5 {
		to.next = now + 0.05
		to.due, to.ddue = to.due[:0], to.ddue[:0]
	}
	steps := tmStepCount()
	for to.next < now+tmLookah {
		to.scheduleCol(to.step, to.next)
		to.due = append(to.due, tmDueCol{to.step, to.next})
		to.step = (to.step + 1) % steps
		to.next += to.stepDur()
	}
	for len(to.due) > 0 && to.due[0].t <= now {
		to.setPH(to.due[0].step)
		rhy.lightLamp(to.beatOf(to.due[0].step))
		to.due = to.due[1:]
	}
	for len(to.ddue) > 0 && to.ddue[0].t <= now {
		to.setDrumPH(to.ddue[0].step)
		to.ddue = to.ddue[1:]
	}
}

// ── Wiring ───────────────────────────────────────────────────────────────

// wireTonematrixModule builds the control cells, renders the pad grid, and
// wires the Run/Clear controls. Called once from Run.
func (to *tonematrix) wireTonematrixModule() {
	tempo := dom.Doc.Call("getElementById", "tm-tempo")
	stepsSel := dom.Doc.Call("getElementById", "tm-steps")
	root := dom.Doc.Call("getElementById", "tm-root")
	lvl := dom.Doc.Call("getElementById", "tm-lvl")
	wave := dom.Doc.Call("getElementById", "tm-wave")
	tstack := dom.Doc.Call("getElementById", "tm-tstack")
	sstack := dom.Doc.Call("getElementById", "tm-sstack")
	lstack := dom.Doc.Call("getElementById", "tm-lstack")
	ostack := dom.Doc.Call("getElementById", "tm-ostack")
	if !tempo.Truthy() || !tstack.Truthy() {
		return
	}

	// Tempo cell: standard value knob, LED and reset from the descriptor.
	// LEDStep 10 keeps it at whole BPM.
	tstack.Call("appendChild", makeKnob(tempo, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "tm-tempo", Label: "tempo", Min: 40, Max: 300, Step: 1, Def: 120,
		LEDID: "tm-tempo-led", ResetID: "rst-tm-tempo", LEDStep: 10,
	})

	// Steps cell: outer ring = where the loop ends, inner knob = root octave.
	sstk := stackKnobs(selk.makeSelectorKnob(stepsSel), selk.makeSelectorKnob(root))
	addSelectorLabels(sstk, []string{"12", "16", "24", "32", "48", "64"}, stepsSel)
	addSelectorLabels(sstk, []string{"C1", "C2", "C3", "C4"}, root)
	sstack.Call("appendChild", sstk)
	// The step count and root share the steps cell's reset button, as the
	// routing and waveform share the out cell's. All four were orphans: no
	// reset, not restored by Reset All, not in the permalink.
	//
	// The steps are where the loop ends, not how many columns there are:
	// nothing is rebuilt, and the module keeps its width.
	adoptDescControl(ControlDesc{
		ID: "tm-steps", Label: "steps", IsSelect: true, SelectDef: "16", PermaKey: "ms",
		ResetID: "rst-tm-steps", SelectApply: func(string) { to.markLoop() },
	})
	adoptDescControl(ControlDesc{
		ID: "tm-root", Label: "root", IsSelect: true, SelectDef: "3", PermaKey: "mr",
		ResetID: "rst-tm-steps", SelectApply: func(string) { to.buildTMGrid() },
	})

	// Level cell: standard value knob, LED and reset from the descriptor.
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "tm-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 80,
		LEDID: "tm-lvl-led", ResetID: "rst-tm-lvl",
		Apply: func(float64) { to.updateRouting() },
	})

	// Wave cell: the waveform knob, as a generator's. Where the Matrix is
	// heard is the Mixer's.
	wstk := soloKnob(wave)
	addSelectorWaveDial(wstk, wave, 38)
	ostack.Call("appendChild", wstk)
	adoptDescControl(ControlDesc{
		ID: "tm-wave", Label: "wave", IsSelect: true, SelectDef: "0", PermaKey: "mv",
		ResetID: "rst-tm-wave",
	})

	if run := dom.Doc.Call("getElementById", "tm-run"); run.Truthy() {
		run.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any {
			to.run = run.Get("checked").Bool()
			to.next = 0 // restart cleanly rather than racing to catch up
			to.due, to.ddue = to.due[:0], to.ddue[:0]
			if to.run {
				to.ensureGraph()
			} else {
				to.setPH(-1)
				to.setDrumPH(-1)
				rhy.lightLamp(-1)
			}
			return nil
		}))
	}
	if b := dom.Doc.Call("getElementById", "tm-clear"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			for c := range tmMaxSteps {
				for r := range tmRows + tmLanes {
					to.setPad(c, r, false) // a lane's steps number no more than the grid's
				}
			}
			return nil
		}))
	}
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	to.on = true
	// Release a pad paint-drag wherever the mouse comes up.
	dom.Doc.Call("addEventListener", "mouseup", dom.FuncOf(func(this js.Value, a []js.Value) any {
		to.paint = -1
		return nil
	}))

	to.buildTMGrid()
}
