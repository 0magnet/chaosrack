//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
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
	lvl := fgFloat(dom.Doc.Call("getElementById", "rhythm-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch dom.Doc.Call("getElementById", "rhythm-out").Get("value").String() {
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
	bpm := fgFloat(dom.Doc.Call("getElementById", "rhythm-tempo"))
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
	lamps := dom.Doc.Call("getElementById", "rhythm-beats")
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
	tabs := dom.Doc.Call("getElementById", "rhythm-tabs")
	if tabs.Truthy() {
		kids := tabs.Get("children")
		for i := 0; i < kids.Get("length").Int(); i++ {
			el := kids.Index(i)
			el.Get("classList").Call("toggle", "down", el.Call("getAttribute", "data-rp").String() == name)
		}
	}
	if sel := dom.Doc.Call("getElementById", "rhythm-preset"); sel.Truthy() {
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
	host := dom.Doc.Call("getElementById", "rhythm-beats")
	if !host.Truthy() {
		return
	}
	p, ok := rhythmPatternByName(rhythmPreset)
	if !ok {
		return
	}
	host.Set("innerHTML", "")
	for i := 0; i < rhythmBeatsPerBar(p); i++ {
		d := dom.Doc.Call("createElement", "span")
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
	if lamps := dom.Doc.Call("getElementById", "rhythm-beats"); lamps.Truthy() {
		kids := lamps.Get("children")
		for i := 0; i < kids.Get("length").Int(); i++ {
			kids.Index(i).Get("classList").Call("remove", "lit")
		}
	}
}

// wireRhythmModule builds the control cells and the tab bank. Called once from
// Run, BEFORE the permalink is applied, so the hidden preset select already has
// its options when a link tries to set one.
func wireRhythmModule() {
	tempo := dom.Doc.Call("getElementById", "rhythm-tempo")
	lvl := dom.Doc.Call("getElementById", "rhythm-lvl")
	out := dom.Doc.Call("getElementById", "rhythm-out")
	sel := dom.Doc.Call("getElementById", "rhythm-preset")
	tabs := dom.Doc.Call("getElementById", "rhythm-tabs")
	tstack := dom.Doc.Call("getElementById", "rhythm-tstack")
	lstack := dom.Doc.Call("getElementById", "rhythm-lstack")
	ostack := dom.Doc.Call("getElementById", "rhythm-ostack")
	if !tempo.Truthy() || !tabs.Truthy() || !tstack.Truthy() {
		return
	}

	// Tempo cell: value knob, LED and reset from the descriptor. LEDStep 10
	// keeps it at whole BPM.
	tstack.Call("appendChild", makeKnob(tempo, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "rhythm-tempo", Label: "tempo", Min: 40, Max: 240, Step: 1, Def: 100,
		LEDID: "rhythm-tempo-led", ResetID: "rst-rhythm-tempo", LEDStep: 10,
	})

	// Level cell: value knob, LED and reset from the descriptor.
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "rhythm-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 80,
		LEDID: "rhythm-lvl-led", ResetID: "rst-rhythm-lvl",
		Apply: func(float64) { rhythmUpdateRouting() },
	})

	// Out cell: the same routing ring every voice module has.
	ostk := makeSelectorKnob(out)
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out)
	ostack.Call("appendChild", ostk)
	// Another orphan: no reset, no Reset All, no permalink.
	adoptDescControl(ControlDesc{
		ID: "rhythm-out", Label: "out", IsSelect: true, SelectDef: "both", PermaKey: "ho",
		ResetID: "rst-rhythm-out", SelectApply: func(string) { rhythmUpdateRouting() },
	})

	// The tab bank, and the hidden select that carries it in a link. Both are
	// built from rhythmPatterns so there is ONE list of what the presets are —
	// a tab with no matching option would be a preset no link could describe.
	for _, p := range rhythmPatterns {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", p.Name)
		opt.Set("textContent", p.Name)
		sel.Call("appendChild", opt)

		name := p.Name
		tab := dom.Doc.Call("createElement", "div")
		tab.Set("className", "rhythm-tab")
		tab.Call("setAttribute", "data-rp", name)
		tab.Set("textContent", name)
		tab.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			setRhythmPreset(name)
			// Pressing a tab starts the section, as it did on the organ: the
			// tabs WERE the start control there. Run stays the way to stop it.
			if run := dom.Doc.Call("getElementById", "rhythm-run"); run.Truthy() && !run.Get("checked").Bool() {
				run.Set("checked", true)
				run.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			}
			return nil
		}))
		tabs.Call("appendChild", tab)
	}
	// The select is what a permalink writes to; the tabs follow it.
	sel.Set("value", rhythmPreset)
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		setRhythmPreset(sel.Get("value").String())
		return nil
	}))

	if run := dom.Doc.Call("getElementById", "rhythm-run"); run.Truthy() {
		run.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			setRhythmRunning(run.Get("checked").Bool())
			return nil
		}))
	}
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	rhythmOn = true
	setRhythmPreset(rhythmPreset)
}
