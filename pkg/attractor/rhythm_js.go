//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// The rhythm section's sound, clock and panel. The patterns themselves are in
// rhythm.go, untagged, where they can be read and checked on the host.
//
// THE DRUMS ARE SYNTHESIZED, not sampled, for the same reason the rest of the
// audio here is: a sample is a file to ship and a decision nobody can see
// inside. Each voice is two or three Web Audio nodes and its character is in
// the envelope — a bass drum is a sine swept down fast, a snare is noise and a
// tone together, a hat is that same noise with everything below 6 kHz taken
// away and a decay measured in hundredths. That is roughly how the organs did
// it too: a handful of transistors per voice, not a memory chip.
//
// SCHEDULED AHEAD OF THE AUDIO CLOCK, exactly as the tonematrix does and for
// the reason it gives: rAF is not a clock. Frames arrive when the compositor
// feels like it, so a drum triggered on the frame it falls due is early or late
// by however long that frame took, and the ear hears that as a limp. Each step
// is placed on the audio context's own timeline a lookahead in advance, which
// is the one clock in the page that rendering cannot drag around.
var (
	rhythmOn      bool
	rhythmRunning bool
	rhythmPreset  = rhythmDefaultPreset

	rhythmCtx    js.Value
	rhythmMaster js.Value
	rhythmPan    js.Value
	rhythmNext   float64 // audio-clock time of the next step to be scheduled
	rhythmStep   int     // index of that step, counting up without wrapping
	rhythmLampAt = -1    // which beat lamp is currently lit
)

// rhythmLookahead is how far ahead steps are placed, in seconds — the same
// figure the tonematrix uses, for the same reason: a few frames of headroom.
const rhythmLookahead = 0.12

// rhythmEnsureGraph acquires the shared context and builds the output chain,
// the gain-into-panner-into-destination shape every voice module here has.
func rhythmEnsureGraph() {
	ctx := acquireAudioCtx("rhythm")
	if !ctx.Truthy() {
		return
	}
	if !rhythmMaster.Truthy() {
		rhythmMaster = ctx.Call("createGain")
		rhythmPan = ctx.Call("createStereoPanner")
		rhythmMaster.Call("connect", rhythmPan)
		rhythmPan.Call("connect", ctx.Get("destination"))
	}
	rhythmCtx = ctx
	rhythmUpdateRouting()
}

// rhythmUpdateRouting pushes the out ring and level knob into the master chain.
func rhythmUpdateRouting() {
	if !rhythmMaster.Truthy() {
		return
	}
	lvl := fgFloat(doc.Call("getElementById", "rhythm-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch doc.Call("getElementById", "rhythm-out").Get("value").String() {
	case "l":
		gain, pan = lvl, -1
	case "r":
		gain, pan = lvl, 1
	case "both":
		gain, pan = lvl, 0
	}
	// Headroom: a samba puts four voices on some steps, and a bass drum alone
	// already peaks at 0.9.
	rhythmMaster.Get("gain").Set("value", gain*0.35)
	rhythmPan.Get("pan").Set("value", pan)
}

func rhythmTempo() float64 {
	bpm := fgFloat(doc.Call("getElementById", "rhythm-tempo"))
	if bpm < 40 {
		bpm = 100
	}
	return bpm
}

// rhythmTick runs every frame from the render loop and is a no-op unless the
// section is running.
func rhythmTick() {
	if !rhythmOn || !rhythmRunning || !rhythmCtx.Truthy() {
		return
	}
	pat, ok := rhythmPatternByName(rhythmPreset)
	if !ok {
		return
	}
	dur := rhythmStepSeconds(rhythmTempo(), pat)
	if dur <= 0 {
		return
	}
	now := rhythmCtx.Get("currentTime").Float()
	// A long gap — a hidden tab, a stalled frame — resynchronizes instead of
	// racing to catch up. A drum machine that plays sixty steps at once to make
	// up lost time is worse than one that simply carries on from here.
	if rhythmNext == 0 || rhythmNext < now-0.5 {
		rhythmNext = now + 0.05
	}
	for rhythmNext < now+rhythmLookahead {
		rhythmScheduleStep(pat, rhythmStep, rhythmNext)
		rhythmStep++
		rhythmNext += dur
	}
	rhythmUpdateLamps(pat, now, dur)
}

// rhythmUpdateLamps lights the beat the listener is HEARING, not the one being
// scheduled.
//
// Those are a lookahead apart, and a lookahead is a tenth of a second — enough
// that lamps driven off the scheduling counter run visibly ahead of the sound
// and read as a machine that is out of time with itself. So the audible step is
// worked back from the audio clock: rhythmStep is the index of the step at
// rhythmNext, so however many step-durations rhythmNext is in the future is how
// far back the ear currently is.
func rhythmUpdateLamps(p rhythmPattern, now, dur float64) {
	beats := rhythmBeatsPerBar(p)
	perBeat := p.Steps / beats
	if perBeat <= 0 {
		return
	}
	ahead := int((rhythmNext-now)/dur + 0.5)
	heard := rhythmStep - ahead
	if heard < 0 {
		return
	}
	beat := (heard / perBeat) % beats
	if beat == rhythmLampAt {
		return
	}
	rhythmLampAt = beat
	lamps := doc.Call("getElementById", "rhythm-beats")
	if !lamps.Truthy() {
		return
	}
	kids := lamps.Get("children")
	for i := 0; i < kids.Get("length").Int(); i++ {
		kids.Index(i).Get("classList").Call("toggle", "lit", i == beat)
	}
}

// rhythmScheduleStep places whatever plays on this step onto the audio clock.
func rhythmScheduleStep(p rhythmPattern, step int, t float64) {
	for v := 0; v < rhythmVoiceCount; v++ {
		if rhythmHit(p, v, step) {
			rhythmVoice(v, t)
		}
	}
}

// rhythmVoice builds one drum at time t and lets it fall away.
//
// Nodes are made per hit and left to be collected once they have stopped, which
// is how Web Audio is meant to be driven: a node is a note, not an instrument.
func rhythmVoice(v int, t float64) {
	ctx := rhythmCtx
	g := ctx.Call("createGain")
	gain := g.Get("gain")
	g.Call("connect", rhythmMaster)

	switch v {
	case voiceBass:
		// A sine swept 150 → 45 Hz in a twentieth of a second. The sweep IS the
		// sound: held at one pitch this is an organ note, not a drum.
		osc := ctx.Call("createOscillator")
		osc.Set("type", "sine")
		f := osc.Get("frequency")
		f.Call("setValueAtTime", 150, t)
		f.Call("exponentialRampToValueAtTime", 45, t+0.05)
		gain.Call("setValueAtTime", 0.9, t)
		gain.Call("exponentialRampToValueAtTime", 0.001, t+0.28)
		osc.Call("connect", g)
		osc.Call("start", t)
		osc.Call("stop", t+0.3)

	case voiceSnare:
		// Noise for the wires and a triangle for the head, together: either one
		// on its own reads as a hiss or as a tom.
		n := ctx.Call("createBufferSource")
		n.Set("buffer", genNoiseBuffer(ctx))
		n.Set("loop", true)
		hp := ctx.Call("createBiquadFilter")
		hp.Set("type", "highpass")
		hp.Get("frequency").Set("value", 1200)
		n.Call("connect", hp)
		hp.Call("connect", g)
		tone := ctx.Call("createOscillator")
		tone.Set("type", "triangle")
		tone.Get("frequency").Set("value", 190)
		tg := ctx.Call("createGain")
		tg.Get("gain").Set("value", 0.4)
		tone.Call("connect", tg)
		tg.Call("connect", g)
		gain.Call("setValueAtTime", 0.7, t)
		gain.Call("exponentialRampToValueAtTime", 0.001, t+0.18)
		n.Call("start", t)
		n.Call("stop", t+0.2)
		tone.Call("start", t)
		tone.Call("stop", t+0.2)

	case voiceHat, voiceCymbal:
		// The same noise twice, told apart by how much of it is left and how
		// long it lasts: a hat is a tick, a cymbal is a wash.
		n := ctx.Call("createBufferSource")
		n.Set("buffer", genNoiseBuffer(ctx))
		n.Set("loop", true)
		hp := ctx.Call("createBiquadFilter")
		hp.Set("type", "highpass")
		decay, peak := 0.05, 0.35
		if v == voiceCymbal {
			hp.Get("frequency").Set("value", 4000)
			decay, peak = 0.45, 0.25
		} else {
			hp.Get("frequency").Set("value", 6500)
		}
		n.Call("connect", hp)
		hp.Call("connect", g)
		gain.Call("setValueAtTime", peak, t)
		gain.Call("exponentialRampToValueAtTime", 0.001, t+decay)
		n.Call("start", t)
		n.Call("stop", t+decay+0.02)
	}
}

// setRhythmPreset picks a pattern and interlocks the tabs, as the row of tabs
// on the organ did: pressing one popped the last one out.
func setRhythmPreset(name string) {
	if _, ok := rhythmPatternByName(name); !ok {
		return
	}
	rhythmPreset = name
	tabs := doc.Call("getElementById", "rhythm-tabs")
	if tabs.Truthy() {
		kids := tabs.Get("children")
		for i := 0; i < kids.Get("length").Int(); i++ {
			el := kids.Index(i)
			el.Get("classList").Call("toggle", "down", el.Call("getAttribute", "data-rp").String() == name)
		}
	}
	if sel := doc.Call("getElementById", "rhythm-preset"); sel.Truthy() {
		sel.Set("value", name)
	}
	// The bar restarts on a change of pattern rather than continuing from
	// whatever step the old one had reached: a bossa that begins halfway
	// through its bar is not a bossa.
	rhythmRestart()
	rhythmBuildLamps()
}

// rhythmRestart drops the schedule so the next tick begins a fresh bar.
func rhythmRestart() {
	rhythmNext = 0
	rhythmStep = 0
	rhythmLampAt = -1
}

// rhythmBuildLamps puts one lamp per beat of the current bar.
//
// Three for a waltz and four for a march, because the count is the thing being
// shown — a fixed four lamps under a waltz would be counting a bar the pattern
// does not have.
func rhythmBuildLamps() {
	host := doc.Call("getElementById", "rhythm-beats")
	if !host.Truthy() {
		return
	}
	p, ok := rhythmPatternByName(rhythmPreset)
	if !ok {
		return
	}
	host.Set("innerHTML", "")
	for i := 0; i < rhythmBeatsPerBar(p); i++ {
		d := doc.Call("createElement", "span")
		d.Set("className", "rhythm-beat")
		host.Call("appendChild", d)
	}
}

// setRhythmRunning starts or stops the section.
func setRhythmRunning(on bool) {
	rhythmRunning = on
	rhythmRestart()
	if on {
		rhythmEnsureGraph()
		return
	}
	if lamps := doc.Call("getElementById", "rhythm-beats"); lamps.Truthy() {
		kids := lamps.Get("children")
		for i := 0; i < kids.Get("length").Int(); i++ {
			kids.Index(i).Get("classList").Call("remove", "lit")
		}
	}
}

// setRhythmOn shows or hides the module.
//
// Unlike the Matrix switch this does NOT change the model. The tonematrix
// switches the view to the spectrogram because its pads are the picture — the
// point of painting them is watching them play. A drum machine has no picture
// to show, so taking over the user's model to open it would be seizing
// something it has no use for.
func setRhythmOn(on bool) {
	rhythmOn = on
	if sect := doc.Call("getElementById", "rhythm-module"); sect.Truthy() {
		if on {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	rhythmRestart()
	if on {
		rhythmEnsureGraph() // the switch flip is our user gesture
	} else if rhythmCtx.Truthy() {
		// Hand the shared context back, as every other voice module does when
		// it closes: the lease is what keeps it from being suspended, and a
		// module nobody can see holding one open would keep the audio hardware
		// awake for nothing.
		releaseAudioCtx("rhythm")
		rhythmCtx = js.Undefined()
	}
	quantizeModuleWidths()
}

// wireRhythmModule builds the control cells and the tab bank. Called once from
// Run, BEFORE the permalink is applied, so the hidden preset select already has
// its options when a link tries to set one.
func wireRhythmModule() {
	tempo := doc.Call("getElementById", "rhythm-tempo")
	tempoLED := doc.Call("getElementById", "rhythm-tempo-led")
	lvl := doc.Call("getElementById", "rhythm-lvl")
	lvlLED := doc.Call("getElementById", "rhythm-lvl-led")
	out := doc.Call("getElementById", "rhythm-out")
	sel := doc.Call("getElementById", "rhythm-preset")
	tabs := doc.Call("getElementById", "rhythm-tabs")
	tstack := doc.Call("getElementById", "rhythm-tstack")
	lstack := doc.Call("getElementById", "rhythm-lstack")
	ostack := doc.Call("getElementById", "rhythm-ostack")
	if !tempo.Truthy() || !tabs.Truthy() || !tstack.Truthy() {
		return
	}

	// Tempo cell: value knob + editable LED.
	tempoLED.Set("value", formatLED(fgFloat(tempo), intDigits(240), 0, false))
	sizeLEDField(tempoLED, 40, 240, 0, false)
	tstack.Call("appendChild", makeKnob(tempo, js.Undefined(), true, false, true))
	tempo.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		tempoLED.Set("value", formatLED(fgFloat(tempo), intDigits(240), 0, false))
		return nil
	}))
	tempoLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(tempoLED.Get("value").String(), 64); err == nil {
			tempo.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			tempo.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))

	// Level cell: value knob + LED.
	lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
	sizeLEDField(lvlLED, 0, 100, 1, false)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
		rhythmUpdateRouting()
		return nil
	}))
	lvlLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlLED.Get("value").String(), 64); err == nil {
			lvl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			lvl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))

	// Out cell: the same routing ring every voice module has.
	ostk := makeSelectorKnob(out)
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out, 50)
	ostack.Call("appendChild", ostk)
	out.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		rhythmUpdateRouting()
		return nil
	}))

	// The tab bank, and the hidden select that carries it in a link. Both are
	// built from rhythmPatterns so there is ONE list of what the presets are —
	// a tab with no matching option would be a preset no link could describe.
	for _, p := range rhythmPatterns {
		opt := doc.Call("createElement", "option")
		opt.Set("value", p.Name)
		opt.Set("textContent", p.Name)
		sel.Call("appendChild", opt)

		name := p.Name
		tab := doc.Call("createElement", "div")
		tab.Set("className", "rhythm-tab")
		tab.Call("setAttribute", "data-rp", name)
		tab.Set("textContent", name)
		tab.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			setRhythmPreset(name)
			// Pressing a tab starts the section, as it did on the organ: the
			// tabs WERE the start control there. Run stays the way to stop it.
			if run := doc.Call("getElementById", "rhythm-run"); run.Truthy() && !run.Get("checked").Bool() {
				run.Set("checked", true)
				run.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			}
			return nil
		}))
		tabs.Call("appendChild", tab)
	}
	// The select is what a permalink writes to; the tabs follow it.
	sel.Set("value", rhythmPreset)
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		setRhythmPreset(sel.Get("value").String())
		return nil
	}))

	if run := doc.Call("getElementById", "rhythm-run"); run.Truthy() {
		run.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			setRhythmRunning(run.Get("checked").Bool())
			return nil
		}))
	}
	if sw := doc.Call("getElementById", "rhythm-on"); sw.Truthy() {
		sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			setRhythmOn(sw.Get("checked").Bool())
			return nil
		}))
	}
	setRhythmPreset(rhythmPreset)
}
