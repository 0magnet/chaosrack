//go:build js && wasm

package attractor

import (
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

var (
	tmOn      bool
	tmRun     = true
	tmCtx     js.Value // shared ctx while the lease is held
	tmMaster  js.Value // master gain (level × routing)
	tmPanNode js.Value // stereo panner (routing)

	tmPat   [tmMaxSteps][tmRows]bool
	tmCells [tmMaxSteps][tmRows]js.Value
	tmCols  []js.Value

	tmStep  int     // next column to schedule
	tmNext  float64 // ctx time the next column sounds at
	tmPHCol = -1    // column currently highlighted as the playhead

	// Scheduled-but-not-yet-sounding columns (audio runs ~a lookahead ahead
	// of the display; the playhead advances when a column's time arrives).
	tmDue []tmDueCol

	tmPaint   = -1    // pad state being painted by the current drag (-1 = none)
	tmTouchAt float64 // performance.now() of the last pad touch — a tap's
	// synthesized compatibility mousedown must not re-toggle the pad

	tmPrevMode string // model showing before the Matrix took the display
)

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
	n, _ := strconv.Atoi(doc.Call("getElementById", "tm-steps").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	if n < 1 || n > tmMaxSteps {
		n = 16
	}
	return n
}

// tmRootMidi reads the root knob: midi C of the chosen octave.
func tmRootMidi() int {
	oct, _ := strconv.Atoi(doc.Call("getElementById", "tm-root").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return 12 * (oct + 1)                                                               // C1=24 … C4=60
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
// their pattern state across rebuilds (it lives in tmPat, not the DOM).
func buildTMGrid() {
	grid := doc.Call("getElementById", "tm-grid")
	if !grid.Truthy() {
		return
	}
	grid.Set("innerHTML", "")
	tmCols = tmCols[:0]
	tmSetPH(-1)
	steps := tmStepCount()
	noteRow := make([]string, tmRows)
	for r := 0; r < tmRows; r++ {
		m := tmMidiFor(r)
		noteRow[r] = noteNames[m%12] + strconv.Itoa(m/12-1)
	}
	for c := 0; c < steps; c++ {
		col := doc.Call("createElement", "span")
		cls := "tm-col"
		if c > 0 && c%4 == 0 {
			cls += " tm-beat" // a breath every four columns, like bar lines
		}
		col.Set("className", cls)
		for r := 0; r < tmRows; r++ {
			cc, rr := c, r
			cell := doc.Call("createElement", "span")
			cell.Set("className", "tm-cell")
			cell.Set("title", "Tonematrix pad — step "+strconv.Itoa(c+1)+", "+noteRow[r]+" (click to toggle, drag to paint)")
			cell.Call("setAttribute", "data-tmc", strconv.Itoa(c))
			cell.Call("setAttribute", "data-tmr", strconv.Itoa(r))
			if tmPat[c][r] {
				cell.Get("classList").Call("add", "on")
			}
			cell.Call("addEventListener", "mousedown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				a[0].Call("preventDefault")
				if js.Global().Get("performance").Call("now").Float()-tmTouchAt < 800 {
					return nil
				}
				on := !tmPat[cc][rr]
				tmSetPad(cc, rr, on)
				tmPaint = 0
				if on {
					tmPaint = 1
				}
				tmEnsureGraph() // user gesture: unlock audio for the loop
				return nil
			}))
			cell.Call("addEventListener", "mouseenter", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				if tmPaint < 0 {
					return nil
				}
				if int(a[0].Get("buttons").Float())&1 == 0 {
					tmPaint = -1
					return nil
				}
				tmSetPad(cc, rr, tmPaint == 1)
				return nil
			}))
			cell.Call("addEventListener", "touchstart", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				a[0].Call("preventDefault")
				tmTouchAt = js.Global().Get("performance").Call("now").Float()
				on := !tmPat[cc][rr]
				tmSetPad(cc, rr, on)
				tmPaint = 0
				if on {
					tmPaint = 1
				}
				tmEnsureGraph()
				return nil
			}))
			tmCells[c][r] = cell
			col.Call("appendChild", cell)
		}
		tmCols = append(tmCols, col)
		grid.Call("appendChild", col)
	}
	// Touch paint: touchmove keeps targeting the starting pad, so follow the
	// finger with elementFromPoint (the keybed glissando pattern).
	grid.Call("addEventListener", "touchmove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		e.Call("preventDefault")
		if tmPaint < 0 {
			return nil
		}
		t := e.Get("touches").Index(0)
		el := doc.Call("elementFromPoint", t.Get("clientX").Float(), t.Get("clientY").Float())
		if !el.Truthy() {
			return nil
		}
		cAttr, rAttr := el.Call("getAttribute", "data-tmc"), el.Call("getAttribute", "data-tmr")
		if !cAttr.Truthy() || !rAttr.Truthy() {
			return nil
		}
		c, _ := strconv.Atoi(cAttr.String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		r, _ := strconv.Atoi(rAttr.String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		tmSetPad(c, r, tmPaint == 1)
		return nil
	}))
	for _, ev := range []string{"touchend", "touchcancel"} {
		grid.Call("addEventListener", ev, trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			tmPaint = -1
			return nil
		}))
	}
	if tmStep >= steps {
		tmStep = 0
	}
}

func tmSetPad(c, r int, on bool) {
	if tmPat[c][r] == on {
		return
	}
	tmPat[c][r] = on
	if el := tmCells[c][r]; el.Truthy() {
		if on {
			el.Get("classList").Call("add", "on")
		} else {
			el.Get("classList").Call("remove", "on")
		}
	}
}

// tmSetPH moves the playhead highlight to col (-1 = off).
func tmSetPH(col int) {
	if tmPHCol == col {
		return
	}
	if tmPHCol >= 0 && tmPHCol < len(tmCols) {
		tmCols[tmPHCol].Get("classList").Call("remove", "ph")
	}
	tmPHCol = col
	if col >= 0 && col < len(tmCols) {
		tmCols[col].Get("classList").Call("add", "ph")
	}
}

// ── Voice engine ─────────────────────────────────────────────────────────

// tmEnsureGraph acquires the shared context (call from a user gesture so
// the autoplay policy lets it start) and lazily builds master gain → panner.
// The context it acquired is tmCtx, which stays unset if the acquire failed.
func tmEnsureGraph() {
	ctx := acquireAudioCtx("tmx")
	if !ctx.Truthy() {
		return
	}
	if !tmMaster.Truthy() {
		tmMaster = ctx.Call("createGain")
		tmPanNode = ctx.Call("createStereoPanner")
		tmMaster.Call("connect", tmPanNode)
		tmPanNode.Call("connect", ctx.Get("destination"))
	}
	tmCtx = ctx
	tmUpdateRouting()
}

// tmUpdateRouting pushes the out ring + level knob into the master chain.
func tmUpdateRouting() {
	if !tmMaster.Truthy() {
		return
	}
	lvl := fgFloat(doc.Call("getElementById", "tm-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch doc.Call("getElementById", "tm-out").Get("value").String() {
	case "l":
		gain, pan = lvl, -1
	case "r":
		gain, pan = lvl, 1
	case "both":
		gain, pan = lvl, 0
	}
	tmMaster.Get("gain").Set("value", gain*0.3) // headroom for full columns
	tmPanNode.Get("pan").Set("value", pan)
}

// tmStepDur returns one column's duration: columns are sixteenths, four to
// the beat, so a 16-step loop is one bar at the tempo knob's BPM.
func tmStepDur() float64 {
	bpm := fgFloat(doc.Call("getElementById", "tm-tempo"))
	if bpm < 40 {
		bpm = 120
	}
	return 60 / bpm / 4
}

// tmScheduleCol sounds every lit pad in the column at ctx time t: a short
// ping (fast attack, exponential decay) per pad, fire-and-forget nodes.
func tmScheduleCol(c int, t float64) {
	w, _ := strconv.Atoi(doc.Call("getElementById", "tm-wave").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	dur := tmStepDur() * 2
	if dur < 0.2 {
		dur = 0.2
	}
	if dur > 0.5 {
		dur = 0.5
	}
	for r := 0; r < tmRows; r++ {
		if !tmPat[c][r] {
			continue
		}
		hz := 440 * math.Pow(2, float64(tmMidiFor(r)-69)/12)
		var osc js.Value
		if w == 4 {
			// Noise pad: the DCSG shift-register loop pitched by playback rate
			// (snare/hat territory — higher rows = brighter bursts).
			osc = tmCtx.Call("createBufferSource")
			osc.Set("buffer", genNoiseBuffer(tmCtx))
			osc.Set("loop", true)
			osc.Get("playbackRate").Set("value", hz*32/tmCtx.Get("sampleRate").Float())
		} else {
			osc = tmCtx.Call("createOscillator")
			osc.Set("type", waveTypeName(w))
			osc.Get("frequency").Set("value", hz)
		}
		g := tmCtx.Call("createGain")
		gg := g.Get("gain")
		gg.Call("setValueAtTime", 0, t)
		gg.Call("linearRampToValueAtTime", 1, t+0.006)
		gg.Call("exponentialRampToValueAtTime", 0.001, t+dur)
		osc.Call("connect", g)
		g.Call("connect", tmMaster)
		osc.Call("start", t)
		osc.Call("stop", t+dur+0.02)
	}
}

// tmTick runs every frame from the render loop: schedule columns a small
// lookahead ahead of the audio clock, and advance the playhead as each
// scheduled column's time arrives. A long gap (hidden tab froze rAF)
// resynchronizes instead of racing to catch up.
func tmTick() {
	if !tmOn || !tmRun || !tmCtx.Truthy() {
		return
	}
	now := tmCtx.Get("currentTime").Float()
	if tmNext == 0 || tmNext < now-0.5 {
		tmNext = now + 0.05
		tmDue = tmDue[:0]
	}
	steps := tmStepCount()
	for tmNext < now+tmLookah {
		tmScheduleCol(tmStep, tmNext)
		tmDue = append(tmDue, tmDueCol{tmStep, tmNext})
		tmStep = (tmStep + 1) % steps
		tmNext += tmStepDur()
	}
	for len(tmDue) > 0 && tmDue[0].t <= now {
		tmSetPH(tmDue[0].step)
		tmDue = tmDue[1:]
	}
}

// ── Wiring ───────────────────────────────────────────────────────────────

// tmSwitchMode drives the model selector programmatically so every
// mode-change side effect (panel rebuild, camera, permalink) applies.
func tmSwitchMode(mode string) {
	if sel := doc.Call("getElementById", "mode-select"); sel.Truthy() {
		sel.Set("value", mode)
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
}

// setTonematrixOn shows/hides the module; hiding parks the playhead and
// drops the audio-context lease (the graph stays for the next resume).
// While the Matrix runs the display shows the audio, not the attractor:
// switching on hands the screen to the spectrogram (the scrolling spectrum
// is the pattern itself in frequency × time), switching off restores the
// model that was showing — unless the user picked another mode meanwhile.
func setTonematrixOn(on bool) {
	tmOn = on
	if sect := doc.Call("getElementById", "tm-module"); sect.Truthy() {
		if on {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if on {
		tmEnsureGraph() // the switch flip is our user gesture
		tmNext = 0
		if selectedMode != "spectrogram" && selectedMode != "xy" {
			tmPrevMode = selectedMode
			tmSwitchMode("spectrogram")
		}
	} else {
		tmSetPH(-1)
		tmNext = 0
		tmDue = tmDue[:0]
		if tmCtx.Truthy() {
			releaseAudioCtx("tmx")
			tmCtx = js.Undefined()
		}
		if tmPrevMode != "" && selectedMode == "spectrogram" {
			tmSwitchMode(tmPrevMode)
		}
		tmPrevMode = ""
	}
	quantizeModuleWidths()
}

// wireTonematrixModule builds the control cells, renders the pad grid, and
// wires the Run/Clear controls. Called once from Run.
func wireTonematrixModule() {
	tempo := doc.Call("getElementById", "tm-tempo")
	tempoLED := doc.Call("getElementById", "tm-tempo-led")
	stepsSel := doc.Call("getElementById", "tm-steps")
	root := doc.Call("getElementById", "tm-root")
	lvl := doc.Call("getElementById", "tm-lvl")
	lvlLED := doc.Call("getElementById", "tm-lvl-led")
	out := doc.Call("getElementById", "tm-out")
	wave := doc.Call("getElementById", "tm-wave")
	sw := doc.Call("getElementById", "tm-on")
	tstack := doc.Call("getElementById", "tm-tstack")
	sstack := doc.Call("getElementById", "tm-sstack")
	lstack := doc.Call("getElementById", "tm-lstack")
	ostack := doc.Call("getElementById", "tm-ostack")
	if !tempo.Truthy() || !tstack.Truthy() {
		return
	}

	// Tempo cell: standard value knob + editable LED.
	tempoLED.Set("value", formatLED(fgFloat(tempo), intDigits(300), 0, false))
	sizeLEDField(tempoLED, 40, 300, 0, false)
	tstack.Call("appendChild", makeKnob(tempo, js.Undefined(), true, false, true))
	tempo.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		tempoLED.Set("value", formatLED(fgFloat(tempo), intDigits(300), 0, false))
		return nil
	}))
	tempoLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(tempoLED.Get("value").String(), 64); err == nil {
			tempo.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			tempo.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))

	// Steps cell: outer ring = column count, inner knob = root octave.
	sstk := stackKnobs(makeSelectorKnob(stepsSel), makeSelectorKnob(root))
	addSelectorLabels(sstk, []string{"8", "16", "32"}, stepsSel, 50)
	addSelectorLabels(sstk, []string{"C1", "C2", "C3", "C4"}, root, 36)
	sstack.Call("appendChild", sstk)
	rebuild := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		buildTMGrid()
		quantizeModuleWidths()
		return nil
	})
	stepsSel.Call("addEventListener", "change", rebuild)
	root.Call("addEventListener", "change", rebuild)

	// Level cell: standard value knob + LED.
	lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
	sizeLEDField(lvlLED, 0, 100, 1, false)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
		tmUpdateRouting()
		return nil
	}))
	lvlLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlLED.Get("value").String(), 64); err == nil {
			lvl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			lvl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))

	// Out cell: Gen-oscillator anatomy — routing ring, waveform inner knob.
	ostk := stackKnobs(makeSelectorKnob(out), makeSelectorKnob(wave))
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out, 50)
	addSelectorWaveDial(ostk, wave, 38)
	ostack.Call("appendChild", ostk)
	out.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		tmUpdateRouting()
		return nil
	}))

	if run := doc.Call("getElementById", "tm-run"); run.Truthy() {
		run.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			tmRun = run.Get("checked").Bool()
			tmNext = 0 // restart cleanly rather than racing to catch up
			tmDue = tmDue[:0]
			if tmRun {
				tmEnsureGraph()
			} else {
				tmSetPH(-1)
			}
			return nil
		}))
	}
	if b := doc.Call("getElementById", "tm-clear"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			for c := 0; c < tmMaxSteps; c++ {
				for r := 0; r < tmRows; r++ {
					tmSetPad(c, r, false)
				}
			}
			return nil
		}))
	}
	if sw.Truthy() {
		sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			setTonematrixOn(sw.Get("checked").Bool())
			return nil
		}))
	}
	// Release a pad paint-drag wherever the mouse comes up.
	doc.Call("addEventListener", "mouseup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		tmPaint = -1
		return nil
	}))

	buildTMGrid()
}
