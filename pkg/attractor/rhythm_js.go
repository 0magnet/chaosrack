//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rhythm"
	"syscall/js"
)

// rhythmSection is the rhythm section's sound, clock and panel state.
type rhythmSection struct {
	// The rhythm section's sound, clock and panel. The patterns themselves are in
	// pkg/rhythm, where they can be read and checked on the host.
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
	on      bool
	running bool
	preset  string
	ctx     js.Value
	master  js.Value
	pan     js.Value
	next    float64 // audio-clock time of the next step to be scheduled
	step    int     // index of that step, counting up without wrapping
	lampAt  int     // which beat lamp is currently lit
}

var rhy = rhythmSection{
	preset: rhythm.DefaultPreset,
	lampAt: -1,
}

// rhythmLookahead is how far ahead steps are placed, in seconds — the same
// figure the tonematrix uses, for the same reason: a few frames of headroom.
const rhythmLookahead = 0.12

// ensureGraph acquires the shared context and builds the output chain,
// the gain-into-panner-into-destination shape every voice module here has.
func (r *rhythmSection) ensureGraph() {
	ctx := acquireAudioCtx("rhythm")
	if !ctx.Truthy() {
		return
	}
	if !r.master.Truthy() {
		r.master = ctx.Call("createGain")
		r.pan = ctx.Call("createStereoPanner")
		r.master.Call("connect", r.pan)
		r.pan.Call("connect", ctx.Get("destination"))
	}
	r.ctx = ctx
	r.updateRouting()
}

// updateRouting pushes the out ring and level knob into the master chain.
func (r *rhythmSection) updateRouting() {
	if !r.master.Truthy() {
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
	r.master.Get("gain").Set("value", gain*0.35)
	r.pan.Get("pan").Set("value", pan)
}

func rhythmTempo() float64 {
	bpm := fgFloat(dom.Doc.Call("getElementById", "rhythm-tempo"))
	if bpm < 40 {
		bpm = 100
	}
	return bpm
}

// tick runs every frame from the render loop and is a no-op unless the
// section is running.
func (r *rhythmSection) tick() {
	if !r.on || !r.running || !r.ctx.Truthy() {
		return
	}
	pat, ok := rhythm.ByName(r.preset)
	if !ok {
		return
	}
	dur := rhythm.StepSeconds(rhythmTempo(), pat)
	if dur <= 0 {
		return
	}
	now := r.ctx.Get("currentTime").Float()
	// A long gap — a hidden tab, a stalled frame — resynchronizes instead of
	// racing to catch up. A drum machine that plays sixty steps at once to make
	// up lost time is worse than one that simply carries on from here.
	if r.next == 0 || r.next < now-0.5 {
		r.next = now + 0.05
	}
	for r.next < now+rhythmLookahead {
		rhythmScheduleStep(pat, r.step, r.next)
		r.step++
		r.next += dur
	}
	r.updateLamps(pat, now, dur)
}

// updateLamps lights the beat the listener is HEARING, not the one being
// scheduled.
//
// Those are a lookahead apart, and a lookahead is a tenth of a second — enough
// that lamps driven off the scheduling counter run visibly ahead of the sound
// and read as a machine that is out of time with itself. So the audible step is
// worked back from the audio clock: rhy.step is the index of the step at
// rhy.next, so however many step-durations rhy.next is in the future is how
// far back the ear currently is.
func (r *rhythmSection) updateLamps(p rhythm.Pattern, now, dur float64) {
	beats := rhythm.BeatsPerBar(p)
	perBeat := p.Steps / beats
	if perBeat <= 0 {
		return
	}
	ahead := int((r.next-now)/dur + 0.5)
	heard := r.step - ahead
	if heard < 0 {
		return
	}
	beat := (heard / perBeat) % beats
	if beat == r.lampAt {
		return
	}
	r.lampAt = beat
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
func rhythmScheduleStep(p rhythm.Pattern, step int, t float64) {
	for v := 0; v < rhythm.VoiceCount; v++ {
		if rhythm.Hit(p, v, step) {
			rhy.voice(v, t)
		}
	}
}

// voice builds one drum at time t and lets it fall away.
//
// Nodes are made per hit and left to be collected once they have stopped, which
// is how Web Audio is meant to be driven: a node is a note, not an instrument.
func (r *rhythmSection) voice(v int, t float64) {
	ctx := r.ctx
	g := ctx.Call("createGain")
	gain := g.Get("gain")
	g.Call("connect", r.master)

	switch v {
	case rhythm.Bass:
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

	case rhythm.Snare:
		// Noise for the wires and a triangle for the head, together: either one
		// on its own reads as a hiss or as a tom.
		n := ctx.Call("createBufferSource")
		n.Set("buffer", gen.noiseBuffer(ctx))
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

	case rhythm.Hat, rhythm.Cymbal:
		// The same noise twice, told apart by how much of it is left and how
		// long it lasts: a hat is a tick, a cymbal is a wash.
		n := ctx.Call("createBufferSource")
		n.Set("buffer", gen.noiseBuffer(ctx))
		n.Set("loop", true)
		hp := ctx.Call("createBiquadFilter")
		hp.Set("type", "highpass")
		decay, peak := 0.05, 0.35
		if v == rhythm.Cymbal {
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
func (r *rhythmSection) setRhythmPreset(name string) {
	if _, ok := rhythm.ByName(name); !ok {
		return
	}
	r.preset = name
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
	r.restart()
	r.buildLamps()
}

// restart drops the schedule so the next tick begins a fresh bar.
func (r *rhythmSection) restart() {
	r.next = 0
	r.step = 0
	r.lampAt = -1
}

// buildLamps puts one lamp per beat of the current bar.
//
// Three for a waltz and four for a march, because the count is the thing being
// shown — a fixed four lamps under a waltz would be counting a bar the pattern
// does not have.
func (r *rhythmSection) buildLamps() {
	host := dom.Doc.Call("getElementById", "rhythm-beats")
	if !host.Truthy() {
		return
	}
	p, ok := rhythm.ByName(r.preset)
	if !ok {
		return
	}
	host.Set("innerHTML", "")
	for i := 0; i < rhythm.BeatsPerBar(p); i++ {
		d := dom.Doc.Call("createElement", "span")
		d.Set("className", "rhythm-beat")
		host.Call("appendChild", d)
	}
}

// setRhythmRunning starts or stops the section.
func (r *rhythmSection) setRhythmRunning(on bool) {
	r.running = on
	r.restart()
	if on {
		r.ensureGraph()
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
func (r *rhythmSection) wireRhythmModule() {
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
		Apply: func(float64) { r.updateRouting() },
	})

	// Out cell: the same routing ring every voice module has.
	ostk := selk.makeSelectorKnob(out)
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out)
	ostack.Call("appendChild", ostk)
	// Another orphan: no reset, no Reset All, no permalink.
	adoptDescControl(ControlDesc{
		ID: "rhythm-out", Label: "out", IsSelect: true, SelectDef: "both", PermaKey: "ho",
		ResetID: "rst-rhythm-out", SelectApply: func(string) { r.updateRouting() },
	})

	// The tab bank, and the hidden select that carries it in a link. Both are
	// built from rhythm.Patterns so there is ONE list of what the presets are —
	// a tab with no matching option would be a preset no link could describe.
	for _, p := range rhythm.Patterns {
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
			r.setRhythmPreset(name)
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
	sel.Set("value", r.preset)
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		r.setRhythmPreset(sel.Get("value").String())
		return nil
	}))

	if run := dom.Doc.Call("getElementById", "rhythm-run"); run.Truthy() {
		run.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			r.setRhythmRunning(run.Get("checked").Bool())
			return nil
		}))
	}
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	r.on = true
	r.setRhythmPreset(r.preset)
}
