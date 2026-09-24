//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// The Keys module: a playable polyphonic keyboard — the first slice of the
// modular-synth direction. A Window-group switch (Keys) shows a module whose
// left column is three standard cells (range knob, level knob, out/voice
// knob) and whose body is a piano keybed that sizes itself to the selected
// range — anything from 13 keys up to the full 88 (A0–C8). Keys play with
// the mouse (click, or hold and glissando), touch, or the computer keyboard
// using the classic two-row tracker map (Z row = lower octave, Q row =
// upper); the mapped keys carry small letter labels. Voices are plain
// OscillatorNodes (one per held note, attack/release ramps against clicks)
// through a master gain + stereo panner on the shared audio context — the
// same out-cell routing (off / L / R / L+R) and waveform inner knob as the
// Gen oscillators, so the module reads as one of them.

// Two-row computer-keyboard map, offsets in semitones from the anchor C
// (the C nearest the middle of the displayed range). The low row's tail
// (, l .) overlaps the high row's first notes, as on every tracker.
var (
	kbLowKeys  = []string{"z", "s", "x", "d", "c", "v", "g", "b", "h", "n", "j", "m", ",", "l", "."}
	kbHighKeys = []string{"q", "2", "w", "3", "e", "4", "r", "5", "t", "6", "y", "7", "u", "i", "9", "o", "0", "p"}
	kbOffset   = func() map[string]int {
		m := map[string]int{}
		for i, k := range kbLowKeys {
			m[k] = i
		}
		for i, k := range kbHighKeys {
			m[k] = 12 + i
		}
		return m
	}()
)

// noteNames spells midi pitch classes for tooltips (shared with the
// Tonematrix module's row labels).
var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// keyboard is the Keys module: its voices, its audio graph and the key being
// held.
type keyboard struct {
	on        bool
	ctx       js.Value         // shared ctx while the lease is held
	master    js.Value         // master gain (level × routing)
	panNode   js.Value         // stereo panner (routing)
	voices    map[int]js.Value // held midi note → OscillatorNode
	gains     map[int]js.Value // held midi note → its GainNode
	keyEls    map[int]js.Value // midi note → key element (highlight)
	anchor    int              // midi note the Z row's C lands on
	mouseNote int              // note held by the current mouse drag
	touchNote int              // note held by the current touch (glissando)
}

var keys = keyboard{
	voices:    map[int]js.Value{},
	gains:     map[int]js.Value{},
	keyEls:    map[int]js.Value{},
	mouseNote: -1,
	touchNote: -1,
}

// keysRange returns the keybed's midi range from the range knobs: the full
// piano for "88", else span whole octaves C-to-C from the base octave.
func keysRange() (lo, hi int) {
	if dom.Doc.Call("getElementById", "keys-span").Get("value").String() == "88" {
		return 21, 108 // A0..C8
	}
	n, _ := strconv.Atoi(dom.Doc.Call("getElementById", "keys-span").Get("value").String())    //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	base, _ := strconv.Atoi(dom.Doc.Call("getElementById", "keys-base").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	lo = 12 * (base + 1)                                                                       // midi C(base): C1=24 … C5=72
	return lo, lo + 12*n
}

func keysIsBlack(midi int) bool {
	switch midi % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// buildKeysBed (re)renders the keybed for the current range: white keys as
// a flex row, black keys absolutely positioned on their white-key
// boundaries, letter labels on the computer-keyboard-mapped span. The bed's
// width tracks the interface size (16px per white key × --kscale) so the
// module quantizes to more slots as the range grows.
func (k *keyboard) buildKeysBed() {
	bed := dom.Doc.Call("getElementById", "keys-bed")
	if !bed.Truthy() {
		return
	}
	k.allOff()
	bed.Set("innerHTML", "")
	k.keyEls = map[int]js.Value{}
	lo, hi := keysRange()

	// Anchor the Z row on the C nearest the middle so both mapped octaves
	// sit in the played range; slide down an octave if the Q row would
	// spill off the top, but never below the low end.
	mid := (lo + hi) / 2
	k.anchor = mid - mid%12
	if k.anchor+24 > hi && k.anchor-12 >= lo {
		k.anchor -= 12
	}
	if k.anchor < lo {
		k.anchor = lo + (12-lo%12)%12
	}

	whitesTotal := 0
	for m := lo; m <= hi; m++ {
		if !keysIsBlack(m) {
			whitesTotal++
		}
	}
	bed.Get("style").Set("width", "calc("+strconv.Itoa(whitesTotal)+" * 16px * var(--kscale,1))")

	whites := dom.Doc.Call("createElement", "span")
	whites.Set("className", "pk-whites")
	blacks := dom.Doc.Call("createElement", "span")
	blacks.Set("className", "pk-blacks")
	blackW := 62.0 / float64(whitesTotal) // % of bed width, ~0.62 white keys

	whitesBefore := 0
	for m := lo; m <= hi; m++ {
		midi := m
		el := dom.Doc.Call("createElement", "span")
		name := noteNames[midi%12] + strconv.Itoa(midi/12-1)
		el.Set("title", "Keys — play "+name+" (hold and slide for glissando)")
		if keysIsBlack(midi) {
			el.Set("className", "pk-key pk-black")
			center := float64(whitesBefore) / float64(whitesTotal) * 100
			el.Get("style").Set("left", strconv.FormatFloat(center-blackW/2, 'f', 3, 64)+"%")
			el.Get("style").Set("width", strconv.FormatFloat(blackW, 'f', 3, 64)+"%")
			blacks.Call("appendChild", el)
		} else {
			el.Set("className", "pk-key pk-white")
			whites.Call("appendChild", el)
			whitesBefore++
		}
		// Letter label where the computer keyboard lands (Q row wins on the
		// overlap so every label is a distinct physical key).
		if off := midi - k.anchor; off >= 0 {
			lab := ""
			if off >= 12 && off-12 < len(kbHighKeys) {
				lab = kbHighKeys[off-12]
			} else if off < 12 {
				lab = kbLowKeys[off]
			}
			if lab != "" {
				sp := dom.Doc.Call("createElement", "span")
				sp.Set("className", "pk-kb")
				sp.Set("textContent", lab)
				el.Call("appendChild", sp)
			}
		}
		k.keyEls[midi] = el
		el.Call("setAttribute", "data-midi", strconv.Itoa(midi))

		el.Call("addEventListener", "mousedown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			e := a[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			k.mouseNote = midi
			k.noteOn(midi)
			return nil
		}))
		el.Call("addEventListener", "mouseenter", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			e := a[0]
			// Glissando: only while a bed drag is in progress with the button
			// still down (buttons==0 heals a mouseup we never saw).
			if k.mouseNote < 0 {
				return nil
			}
			if int(e.Get("buttons").Float())&1 == 0 {
				k.noteOff(k.mouseNote)
				k.mouseNote = -1
				return nil
			}
			if k.mouseNote != midi {
				k.noteOff(k.mouseNote)
				k.mouseNote = midi
				k.noteOn(midi)
			}
			return nil
		}))
		el.Call("addEventListener", "touchstart", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			k.touchNote = midi
			k.noteOn(midi)
			return nil
		}))
		for _, ev := range []string{"touchend", "touchcancel"} {
			el.Call("addEventListener", ev, dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
				// The touch may have slid to another key (glissando below) —
				// release whichever note the finger ended on, and the origin.
				k.noteOff(midi)
				if k.touchNote >= 0 && k.touchNote != midi {
					k.noteOff(k.touchNote)
				}
				k.touchNote = -1
				return nil
			}))
		}
	}
	bed.Call("appendChild", whites)
	bed.Call("appendChild", blacks)
	// Touch glissando: touchmove keeps targeting the starting key, so track
	// the finger with elementFromPoint and slide the sounding note.
	bed.Call("addEventListener", "touchmove", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		el := dom.Doc.Call("elementFromPoint", t.Get("clientX").Float(), t.Get("clientY").Float())
		if !el.Truthy() {
			return nil
		}
		m := el.Call("getAttribute", "data-midi")
		if !m.Truthy() {
			return nil
		}
		midi, err := strconv.Atoi(m.String())
		if err != nil || midi == k.touchNote {
			return nil
		}
		if k.touchNote >= 0 {
			k.noteOff(k.touchNote)
		}
		k.touchNote = midi
		k.noteOn(midi)
		return nil
	}))
}

// ── Voice engine ─────────────────────────────────────────────────────────

// keysEnsureGraph acquires the shared context (we're inside a user gesture:
// a key click or keydown) and lazily builds the master gain → panner chain.
func (k *keyboard) ensureGraph() js.Value {
	ctx := acquireAudioCtx("keys")
	if !ctx.Truthy() {
		return js.Undefined()
	}
	if !k.master.Truthy() {
		k.master = ctx.Call("createGain")
		k.panNode = ctx.Call("createStereoPanner")
		k.master.Call("connect", k.panNode)
		k.panNode.Call("connect", ctx.Get("destination"))
	}
	k.ctx = ctx
	k.updateRouting()
	return ctx
}

// keysUpdateRouting pushes the out ring + level knob into the master chain.
func (k *keyboard) updateRouting() {
	if !k.master.Truthy() {
		return
	}
	lvl := fgFloat(dom.Doc.Call("getElementById", "keys-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch dom.Doc.Call("getElementById", "keys-out").Get("value").String() {
	case "l":
		gain, pan = lvl, -1
	case "r":
		gain, pan = lvl, 1
	case "both":
		gain, pan = lvl, 0
	}
	k.master.Get("gain").Set("value", gain*0.25) // headroom for chords
	k.panNode.Get("pan").Set("value", pan)
}

func (k *keyboard) noteOn(midi int) {
	if _, held := k.voices[midi]; held {
		return
	}
	if el, ok := k.keyEls[midi]; ok {
		el.Get("classList").Call("add", "held")
	}
	ctx := k.ensureGraph()
	if !ctx.Truthy() {
		return
	}
	hz := 440 * math.Pow(2, float64(midi-69)/12)
	w, _ := strconv.Atoi(dom.Doc.Call("getElementById", "keys-wave").Get("value").String()) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	var osc js.Value
	if w == 4 {
		// Noise voice: the DCSG shift-register loop, pitched by playback rate
		// so the keyboard plays tuned noise (chiff/percussion territory).
		osc = ctx.Call("createBufferSource")
		osc.Set("buffer", gen.noiseBuffer(ctx))
		osc.Set("loop", true)
		osc.Get("playbackRate").Set("value", hz*32/ctx.Get("sampleRate").Float())
	} else {
		osc = ctx.Call("createOscillator")
		osc.Set("type", waveTypeName(w))
		osc.Get("frequency").Set("value", hz)
	}
	g := ctx.Call("createGain")
	now := ctx.Get("currentTime").Float()
	g.Get("gain").Call("setValueAtTime", 0, now)
	g.Get("gain").Call("linearRampToValueAtTime", 1, now+0.008)
	osc.Call("connect", g)
	g.Call("connect", k.master)
	osc.Call("start")
	k.voices[midi] = osc
	k.gains[midi] = g
}

func (k *keyboard) noteOff(midi int) {
	if el, ok := k.keyEls[midi]; ok {
		el.Get("classList").Call("remove", "held")
	}
	osc, held := k.voices[midi]
	if !held {
		return
	}
	g := k.gains[midi].Get("gain")
	delete(k.voices, midi)
	delete(k.gains, midi)
	now := k.ctx.Get("currentTime").Float()
	g.Call("cancelScheduledValues", now)
	g.Call("setValueAtTime", g.Get("value"), now)
	g.Call("linearRampToValueAtTime", 0, now+0.12)
	osc.Call("stop", now+0.16)
}

func (k *keyboard) allOff() {
	for m := range k.voices {
		k.noteOff(m)
	}
	k.mouseNote = -1
}

// ── Wiring ───────────────────────────────────────────────────────────────

// wireKeysModule builds the three control cells (range, level, out/voice),
// renders the keybed, and installs the global play listeners. Called once
// from Run.
func (k *keyboard) wireKeysModule() {
	span := dom.Doc.Call("getElementById", "keys-span")
	base := dom.Doc.Call("getElementById", "keys-base")
	lvl := dom.Doc.Call("getElementById", "keys-lvl")
	out := dom.Doc.Call("getElementById", "keys-out")
	wave := dom.Doc.Call("getElementById", "keys-wave")
	rstack := dom.Doc.Call("getElementById", "keys-rstack")
	lstack := dom.Doc.Call("getElementById", "keys-lstack")
	ostack := dom.Doc.Call("getElementById", "keys-ostack")
	sizeLED := dom.Doc.Call("getElementById", "keys-size-led")
	if !span.Truthy() || !rstack.Truthy() {
		return
	}

	// Range cell: outer ring = key count, inner knob = starting octave.
	rstk := stackKnobs(selk.makeSelectorKnob(span), selk.makeSelectorKnob(base))
	addSelectorLabels(rstk, []string{"13", "25", "37", "49", "61", "85", "88"}, span)
	addSelectorLabels(rstk, []string{"C1", "C2", "C3", "C4", "C5"}, base)
	rstack.Call("appendChild", rstk)

	// Level cell: standard value knob, and the descriptor owns everything
	// around it — LED, typed entry, wheel, reset.
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))
	adoptDescControl(ControlDesc{
		ID: "keys-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 80,
		LEDID: "keys-lvl-led", ResetID: "rst-keys-lvl",
		Apply: func(float64) { k.updateRouting() },
	})

	// Out cell: same anatomy as the Gen oscillators — outer ring = speaker
	// routing, inner knob = waveform with the glyph dial.
	ostk := stackKnobs(selk.makeSelectorKnob(out), selk.makeSelectorKnob(wave))
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out)
	addSelectorWaveDial(ostk, wave, 38)
	ostack.Call("appendChild", ostk)

	refreshSize := func() {
		txt := "88"
		if v := span.Get("value").String(); v != "88" {
			n, _ := strconv.Atoi(v) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
			txt = strconv.Itoa(12*n + 1)
		}
		sizeLED.Set("textContent", txt)
	}
	rebuildKeys := func() {
		refreshSize()
		k.buildKeysBed()
		quantizeModuleWidths()
	}
	// The four selectors, through the descriptor path. All four were orphans:
	// no reset button, not restored by Reset All, not in the permalink — so a
	// range or a routing you had chosen could not be undone or shared.
	//
	// span and base share the range cell's one reset button, as out and wave
	// share the output cell's: two descriptors naming the same ResetID each add
	// a listener to it, so one click resets the pair the cell holds.
	adoptDescControl(ControlDesc{
		ID: "keys-span", Label: "range", IsSelect: true, SelectDef: "4", PermaKey: "kp",
		ResetID: "rst-keys-range", SelectApply: func(string) { rebuildKeys() },
	})
	adoptDescControl(ControlDesc{
		ID: "keys-base", Label: "base", IsSelect: true, SelectDef: "2", PermaKey: "kc",
		ResetID: "rst-keys-range", SelectApply: func(string) { rebuildKeys() },
	})
	adoptDescControl(ControlDesc{
		ID: "keys-out", Label: "out", IsSelect: true, SelectDef: "both", PermaKey: "ko",
		ResetID: "rst-keys-out", SelectApply: func(string) { k.updateRouting() },
	})
	adoptDescControl(ControlDesc{
		ID: "keys-wave", Label: "wave", IsSelect: true, SelectDef: "0", PermaKey: "kv",
		SelectApply: func(v string) {
			w, _ := strconv.Atoi(v) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
			if w == 4 {
				return // node-type change applies to NEW notes; held ones finish as-is
			}
			for _, osc := range k.voices { // retype held periodic voices live
				if osc.Get("type").Truthy() { // BufferSource (noise) has no type
					osc.Set("type", waveTypeName(w))
				}
			}
		},
	})
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	k.on = true

	// Computer keyboard: two tracker rows anchored near the range's middle
	// C. Only while the module is shown, never while typing in a field.
	dom.Doc.Call("addEventListener", "keydown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		if !k.on || e.Get("repeat").Bool() ||
			e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool() || e.Get("altKey").Bool() {
			return nil
		}
		if t := e.Get("target"); t.Truthy() {
			switch strings.ToLower(t.Get("tagName").String()) {
			case "input", "select", "textarea":
				return nil
			}
			if t.Get("isContentEditable").Truthy() && t.Get("isContentEditable").Bool() {
				return nil
			}
		}
		off, ok := kbOffset[strings.ToLower(e.Get("key").String())]
		if !ok {
			return nil
		}
		midi := k.anchor + off
		if lo, hi := keysRange(); midi < lo || midi > hi {
			return nil
		}
		e.Call("preventDefault")
		k.noteOn(midi)
		return nil
	}))
	dom.Doc.Call("addEventListener", "keyup", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if off, ok := kbOffset[strings.ToLower(a[0].Get("key").String())]; ok {
			k.noteOff(k.anchor + off)
		}
		return nil
	}))
	// Release the mouse-drag note anywhere; silence everything on tab blur
	// so no note can stick when focus leaves.
	dom.Doc.Call("addEventListener", "mouseup", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		if k.mouseNote >= 0 {
			k.noteOff(k.mouseNote)
			k.mouseNote = -1
		}
		return nil
	}))
	js.Global().Call("addEventListener", "blur", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		k.allOff()
		return nil
	}))

	refreshSize()
	k.buildKeysBed()
}
