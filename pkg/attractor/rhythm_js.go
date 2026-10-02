//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// rhythmSection is the rhythm section's sound and presets. The patterns are
// in pkg/rhythm, where they can be read and checked on the host.
//
// It is the Matrix's drum lanes now, not a machine of its own. It had its own
// tempo knob, its own Run and its own clock, beside a Matrix with all three —
// two sequencers that could be set to the same BPM and still never lock, since
// each placed its steps on the audio clock from a start of its own. The drums
// are four lanes at the foot of the Matrix's grid, played by the Matrix's
// clock (tonematrix_js.go); a preset tab loads a pattern into them, where it
// can be edited, which the organ's tabs never allowed.
//
// THE DRUMS ARE SYNTHESIZED, not sampled, for the same reason the rest of the
// audio here is: a sample is a file to ship and a decision nobody can see
// inside. Each voice is two or three Web Audio nodes and its character is in
// the envelope — a bass drum is a sine swept down fast, a snare is noise and a
// tone together, a hat is that same noise with everything below 6 kHz taken
// away and a decay measured in hundredths. That is roughly how the organs did
// it too: a handful of transistors per voice, not a memory chip.
type rhythmSection struct {
	preset string
	ctx    js.Value
	master js.Value // the drums' level, into the Mixer's DRUMS
	lampAt int      // which beat lamp is currently lit
}

var rhy = rhythmSection{
	preset: rhythm.DefaultPreset,
	lampAt: -1,
}

// ensureGraph acquires the shared context and builds the drums' output: a
// gain, the level, into the Mixer's DRUMS column.
func (r *rhythmSection) ensureGraph() {
	ctx := acquireAudioCtx("rhythm")
	if !ctx.Truthy() {
		return
	}
	if !r.master.Truthy() {
		r.master = ctx.Call("createGain")
		mixEnsure(ctx)
		r.master.Call("connect", mixIn("dr"))
	}
	r.ctx = ctx
	r.updateRouting()
}

// updateRouting pushes the drum level into the drums' gain; where they go
// is the Mixer's.
func (r *rhythmSection) updateRouting() {
	if !r.master.Truthy() {
		return
	}
	lvl := fgFloat(dom.Doc.Call("getElementById", "rhythm-lvl")) / 100
	// Headroom: a samba puts four voices on some steps, and a bass drum alone
	// already peaks at 0.9.
	r.master.Get("gain").Set("value", lvl*0.35)
}

// lightLamp lights the lamp of the beat being heard.
func (r *rhythmSection) lightLamp(beat int) {
	if beat == r.lampAt {
		return
	}
	r.lampAt = beat
	lamps := dom.Doc.Call("getElementById", "rhythm-beats")
	if !lamps.Truthy() {
		return
	}
	kids := lamps.Get("children")
	for i := range kids.Get("length").Int() {
		kids.Index(i).Get("classList").Call("toggle", "lit", i == beat)
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

// setRhythmPreset loads a pattern into the Matrix's drum lanes and interlocks
// the tabs, as the row of tabs on the organ did: pressing one popped the last
// one out.
//
// The loop takes the pattern's length and meter with it: a waltz is a bar of
// twelve in three, a shuffle twelve in four, and a Matrix left at sixteen
// would play either with four steps of silence at the end of every bar.
func (r *rhythmSection) setRhythmPreset(name string) {
	p, ok := rhythm.ByName(name)
	if !ok {
		return
	}
	r.preset = name
	tabs := dom.Doc.Call("getElementById", "rhythm-tabs")
	if tabs.Truthy() {
		kids := tabs.Get("children")
		for i := range kids.Get("length").Int() {
			el := kids.Index(i)
			el.Get("classList").Call("toggle", "down", el.Call("getAttribute", "data-rp").String() == name)
		}
	}
	if sel := dom.Doc.Call("getElementById", "rhythm-preset"); sel.Truthy() {
		sel.Set("value", name)
	}
	tm.loadDrums(p)
	r.buildLamps()
}

// buildLamps puts one lamp per beat of the Matrix's bar.
//
// Three for a waltz and four for a march, because the count is the thing being
// shown — a fixed four lamps under a waltz would be counting a bar the pattern
// does not have.
func (r *rhythmSection) buildLamps() {
	host := dom.Doc.Call("getElementById", "rhythm-beats")
	if !host.Truthy() {
		return
	}
	host.Set("innerHTML", "")
	for range tm.beatsPerBar() {
		d := dom.Doc.Call("createElement", "span")
		d.Set("className", "rhythm-beat")
		host.Call("appendChild", d)
	}
	r.lampAt = -1
}

// wireRhythmModule builds the drum level cell and the tab bank. Called once
// from Run, BEFORE the permalink is applied, so the hidden preset select
// already has its options when a link tries to set one.
func (r *rhythmSection) wireRhythmModule() {
	lvl := dom.Doc.Call("getElementById", "rhythm-lvl")
	sel := dom.Doc.Call("getElementById", "rhythm-preset")
	tabs := dom.Doc.Call("getElementById", "rhythm-tabs")
	lstack := dom.Doc.Call("getElementById", "rhythm-lstack")
	if !lvl.Truthy() || !tabs.Truthy() || !lstack.Truthy() {
		return
	}

	// Level cell: value knob, LED and reset from the descriptor.
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "rhythm-lvl", Label: "drums", Min: 0, Max: 100, Step: 1, Def: 80,
		LEDID: "rhythm-lvl-led", ResetID: "rst-rhythm-lvl",
		Apply: func(float64) { r.updateRouting() },
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
		tab.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			r.setRhythmPreset(name)
			// Pressing a tab starts the loop, as it did on the organ: the
			// tabs WERE the start control there. Run stays the way to stop it.
			if run := dom.Doc.Call("getElementById", "tm-run"); run.Truthy() && !run.Get("checked").Bool() {
				run.Set("checked", true)
				dom.Fire(run, "change")
			}
			tm.ensureGraph() // a user gesture: unlock the audio for the loop
			return nil
		}))
		tabs.Call("appendChild", tab)
	}
	// The select is what a permalink writes to; the tabs follow it.
	sel.Set("value", r.preset)
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any {
		r.setRhythmPreset(sel.Get("value").String())
		return nil
	}))
	r.setRhythmPreset(r.preset)
}
