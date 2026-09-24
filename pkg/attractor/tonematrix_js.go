//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"strconv"
	"syscall/js"
)

// The Tonematrix module: a pentatonic step sequencer — the drum-patterning
// slice of the modular-synth direction. A Window-group switch (Matrix) shows
// a module whose left column is standard cells (tempo, steps/root, level,
// out/voice) plus Run and Clear, and whose body is a grid of lit pads: 16
// pitch rows (major pentatonic, so any pattern consonates — the tonematrix
// trick) by 8/16/32 time columns. A playhead sweeps the columns at the
// tempo, sounding every lit pad in the column as a short ping through the
// same master gain → panner chain the Keys module uses on the shared audio
// context, with the Gen-style out ring and waveform inner knob. Pads paint
// with click-drag or touch-drag; the pattern survives a steps change (the
// hidden columns keep their pads).

// tonematrix is the Tonematrix module: the cells, the pattern, the clock and
// the audio graph.
type tonematrix struct {
	on      bool
	run     bool
	ctx     js.Value // shared ctx while the lease is held
	master  js.Value // master gain (level × routing)
	panNode js.Value // stereo panner (routing)
	pat     [tmMaxSteps][tmRows]bool
	cells   [tmMaxSteps][tmRows]js.Value
	cols    []js.Value
	step    int     // next column to schedule
	next    float64 // ctx time the next column sounds at
	phCol   int     // column currently highlighted as the playhead

	// Scheduled-but-not-yet-sounding columns (audio runs ~a lookahead ahead
	// of the display; the playhead advances when a column's time arrives).
	due     []tmDueCol
	paint   int     // pad state being painted by the current drag (-1 = none)
	touchAt float64 // performance.now() of the last pad touch — a tap's
}

var tm = tonematrix{
	run:   true,
	phCol: -1,
	paint: -1,
}

type tmDueCol struct {
	step int
	t    float64
}

const (
	tmRows     = 16
	tmMaxSteps = 32
	tmLookah   = 0.12 // scheduling horizon, s (a few frames of jitter headroom)
)

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

// buildTMGrid (re)renders the pad grid for the current step count and root:
// column-major spans so the playhead is one class toggle per step. Pads keep
// their pattern state across rebuilds (it lives in tm.pat, not the DOM).
func (to *tonematrix) buildTMGrid() {
	grid := dom.Doc.Call("getElementById", "tm-grid")
	if !grid.Truthy() {
		return
	}
	grid.Set("innerHTML", "")
	to.cols = to.cols[:0]
	to.setPH(-1)
	steps := tmStepCount()
	noteRow := make([]string, tmRows)
	for r := 0; r < tmRows; r++ {
		m := tmMidiFor(r)
		noteRow[r] = noteNames[m%12] + strconv.Itoa(m/12-1)
	}
	for c := 0; c < steps; c++ {
		col := dom.Doc.Call("createElement", "span")
		cls := "tm-col"
		if c > 0 && c%4 == 0 {
			cls += " tm-beat" // a breath every four columns, like bar lines
		}
		col.Set("className", cls)
		for r := 0; r < tmRows; r++ {
			cc, rr := c, r
			cell := dom.Doc.Call("createElement", "span")
			cell.Set("className", "tm-cell")
			cell.Set("title", "Tonematrix pad — step "+strconv.Itoa(c+1)+", "+noteRow[r]+" (click to toggle, drag to paint)")
			cell.Call("setAttribute", "data-tmc", strconv.Itoa(c))
			cell.Call("setAttribute", "data-tmr", strconv.Itoa(r))
			if to.pat[c][r] {
				cell.Get("classList").Call("add", "on")
			}
			cell.Call("addEventListener", "mousedown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
				a[0].Call("preventDefault")
				if js.Global().Get("performance").Call("now").Float()-to.touchAt < 800 {
					return nil
				}
				on := !to.pat[cc][rr]
				to.setPad(cc, rr, on)
				to.paint = 0
				if on {
					to.paint = 1
				}
				to.ensureGraph() // user gesture: unlock audio for the loop
				return nil
			}))
			cell.Call("addEventListener", "mouseenter", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
				if to.paint < 0 {
					return nil
				}
				if int(a[0].Get("buttons").Float())&1 == 0 {
					to.paint = -1
					return nil
				}
				to.setPad(cc, rr, to.paint == 1)
				return nil
			}))
			cell.Call("addEventListener", "touchstart", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
				a[0].Call("preventDefault")
				to.touchAt = js.Global().Get("performance").Call("now").Float()
				on := !to.pat[cc][rr]
				to.setPad(cc, rr, on)
				to.paint = 0
				if on {
					to.paint = 1
				}
				to.ensureGraph()
				return nil
			}))
			to.cells[c][r] = cell
			col.Call("appendChild", cell)
		}
		to.cols = append(to.cols, col)
		grid.Call("appendChild", col)
	}
	// Touch paint: touchmove keeps targeting the starting pad, so follow the
	// finger with elementFromPoint (the keybed glissando pattern).
	grid.Call("addEventListener", "touchmove", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		e.Call("preventDefault")
		if to.paint < 0 {
			return nil
		}
		t := e.Get("touches").Index(0)
		el := dom.Doc.Call("elementFromPoint", t.Get("clientX").Float(), t.Get("clientY").Float())
		if !el.Truthy() {
			return nil
		}
		cAttr, rAttr := el.Call("getAttribute", "data-tmc"), el.Call("getAttribute", "data-tmr")
		if !cAttr.Truthy() || !rAttr.Truthy() {
			return nil
		}
		c, _ := strconv.Atoi(cAttr.String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		r, _ := strconv.Atoi(rAttr.String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		to.setPad(c, r, to.paint == 1)
		return nil
	}))
	for _, ev := range []string{"touchend", "touchcancel"} {
		grid.Call("addEventListener", ev, dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			to.paint = -1
			return nil
		}))
	}
	if to.step >= steps {
		to.step = 0
	}
}

func (to *tonematrix) setPad(c, r int, on bool) {
	if to.pat[c][r] == on {
		return
	}
	to.pat[c][r] = on
	if el := to.cells[c][r]; el.Truthy() {
		if on {
			el.Get("classList").Call("add", "on")
		} else {
			el.Get("classList").Call("remove", "on")
		}
	}
}

// setPH moves the playhead highlight to col (-1 = off).
func (to *tonematrix) setPH(col int) {
	if to.phCol == col {
		return
	}
	if to.phCol >= 0 && to.phCol < len(to.cols) {
		to.cols[to.phCol].Get("classList").Call("remove", "ph")
	}
	to.phCol = col
	if col >= 0 && col < len(to.cols) {
		to.cols[col].Get("classList").Call("add", "ph")
	}
}

// ── Voice engine ─────────────────────────────────────────────────────────

// ensureGraph acquires the shared context (call from a user gesture so
// the autoplay policy lets it start) and lazily builds master gain → panner.
// The context it acquired is tm.ctx, which stays unset if the acquire failed.
func (to *tonematrix) ensureGraph() {
	ctx := acquireAudioCtx("tmx")
	if !ctx.Truthy() {
		return
	}
	if !to.master.Truthy() {
		to.master = ctx.Call("createGain")
		to.panNode = ctx.Call("createStereoPanner")
		to.master.Call("connect", to.panNode)
		to.panNode.Call("connect", ctx.Get("destination"))
	}
	to.ctx = ctx
	to.updateRouting()
}

// updateRouting pushes the out ring + level knob into the master chain.
func (to *tonematrix) updateRouting() {
	if !to.master.Truthy() {
		return
	}
	lvl := fgFloat(dom.Doc.Call("getElementById", "tm-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch dom.Doc.Call("getElementById", "tm-out").Get("value").String() {
	case "l":
		gain, pan = lvl, -1
	case "r":
		gain, pan = lvl, 1
	case "both":
		gain, pan = lvl, 0
	}
	to.master.Get("gain").Set("value", gain*0.3) // headroom for full columns
	to.panNode.Get("pan").Set("value", pan)
}

// tmStepDur returns one column's duration: columns are sixteenths, four to
// the beat, so a 16-step loop is one bar at the tempo knob's BPM.
func tmStepDur() float64 {
	bpm := fgFloat(dom.Doc.Call("getElementById", "tm-tempo"))
	if bpm < 40 {
		bpm = 120
	}
	return 60 / bpm / 4
}

// scheduleCol sounds every lit pad in the column at ctx time t: a short
// ping (fast attack, exponential decay) per pad, fire-and-forget nodes.
func (to *tonematrix) scheduleCol(c int, t float64) {
	w, _ := strconv.Atoi(dom.Doc.Call("getElementById", "tm-wave").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	dur := tmStepDur() * 2
	if dur < 0.2 {
		dur = 0.2
	}
	if dur > 0.5 {
		dur = 0.5
	}
	for r := 0; r < tmRows; r++ {
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
		to.due = to.due[:0]
	}
	steps := tmStepCount()
	for to.next < now+tmLookah {
		to.scheduleCol(to.step, to.next)
		to.due = append(to.due, tmDueCol{to.step, to.next})
		to.step = (to.step + 1) % steps
		to.next += tmStepDur()
	}
	for len(to.due) > 0 && to.due[0].t <= now {
		to.setPH(to.due[0].step)
		to.due = to.due[1:]
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
	out := dom.Doc.Call("getElementById", "tm-out")
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

	// Steps cell: outer ring = column count, inner knob = root octave.
	sstk := stackKnobs(selk.makeSelectorKnob(stepsSel), selk.makeSelectorKnob(root))
	addSelectorLabels(sstk, []string{"8", "16", "32"}, stepsSel)
	addSelectorLabels(sstk, []string{"C1", "C2", "C3", "C4"}, root)
	sstack.Call("appendChild", sstk)
	rebuildTM := func() {
		to.buildTMGrid()
		quantizeModuleWidths()
	}
	// The step count and root share the steps cell's reset button, as the
	// routing and waveform share the out cell's. All four were orphans: no
	// reset, not restored by Reset All, not in the permalink.
	adoptDescControl(ControlDesc{
		ID: "tm-steps", Label: "steps", IsSelect: true, SelectDef: "16", PermaKey: "ms",
		ResetID: "rst-tm-steps", SelectApply: func(string) { rebuildTM() },
	})
	adoptDescControl(ControlDesc{
		ID: "tm-root", Label: "root", IsSelect: true, SelectDef: "3", PermaKey: "mr",
		ResetID: "rst-tm-steps", SelectApply: func(string) { rebuildTM() },
	})

	// Level cell: standard value knob, LED and reset from the descriptor.
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "tm-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 80,
		LEDID: "tm-lvl-led", ResetID: "rst-tm-lvl",
		Apply: func(float64) { to.updateRouting() },
	})

	// Out cell: Gen-oscillator anatomy — routing ring, waveform inner knob.
	ostk := stackKnobs(selk.makeSelectorKnob(out), selk.makeSelectorKnob(wave))
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out)
	addSelectorWaveDial(ostk, wave, 38)
	ostack.Call("appendChild", ostk)
	adoptDescControl(ControlDesc{
		ID: "tm-out", Label: "out", IsSelect: true, SelectDef: "both", PermaKey: "mo",
		ResetID: "rst-tm-out", SelectApply: func(string) { to.updateRouting() },
	})
	adoptDescControl(ControlDesc{
		ID: "tm-wave", Label: "wave", IsSelect: true, SelectDef: "0", PermaKey: "mv",
	})

	if run := dom.Doc.Call("getElementById", "tm-run"); run.Truthy() {
		run.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			to.run = run.Get("checked").Bool()
			to.next = 0 // restart cleanly rather than racing to catch up
			to.due = to.due[:0]
			if to.run {
				to.ensureGraph()
			} else {
				to.setPH(-1)
			}
			return nil
		}))
	}
	if b := dom.Doc.Call("getElementById", "tm-clear"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			for c := 0; c < tmMaxSteps; c++ {
				for r := 0; r < tmRows; r++ {
					to.setPad(c, r, false)
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
	dom.Doc.Call("addEventListener", "mouseup", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		to.paint = -1
		return nil
	}))

	to.buildTMGrid()
}
