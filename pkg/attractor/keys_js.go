//go:build js && wasm

package attractor

import (
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

var (
	keysOn        bool
	keysCtx       js.Value             // shared ctx while the lease is held
	keysMaster    js.Value             // master gain (level × routing)
	keysPanNode   js.Value             // stereo panner (routing)
	keysVoices    = map[int]js.Value{} // held midi note → OscillatorNode
	keysGains     = map[int]js.Value{} // held midi note → its GainNode
	keysKeyEls    = map[int]js.Value{} // midi note → key element (highlight)
	keysAnchor    int                  // midi note the Z row's C lands on
	keysMouseNote = -1                 // note held by the current mouse drag
)

// keysRange returns the keybed's midi range from the range knobs: the full
// piano for "88", else span whole octaves C-to-C from the base octave.
func keysRange() (lo, hi int) {
	if doc.Call("getElementById", "keys-span").Get("value").String() == "88" {
		return 21, 108 // A0..C8
	}
	n, _ := strconv.Atoi(doc.Call("getElementById", "keys-span").Get("value").String())
	base, _ := strconv.Atoi(doc.Call("getElementById", "keys-base").Get("value").String())
	lo = 12 * (base + 1) // midi C(base): C1=24 … C5=72
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
func buildKeysBed() {
	bed := doc.Call("getElementById", "keys-bed")
	if !bed.Truthy() {
		return
	}
	keysAllOff()
	bed.Set("innerHTML", "")
	keysKeyEls = map[int]js.Value{}
	lo, hi := keysRange()

	// Anchor the Z row on the C nearest the middle so both mapped octaves
	// sit in the played range; slide down an octave if the Q row would
	// spill off the top, but never below the low end.
	mid := (lo + hi) / 2
	keysAnchor = mid - mid%12
	if keysAnchor+24 > hi && keysAnchor-12 >= lo {
		keysAnchor -= 12
	}
	if keysAnchor < lo {
		keysAnchor = lo + (12-lo%12)%12
	}

	whitesTotal := 0
	for m := lo; m <= hi; m++ {
		if !keysIsBlack(m) {
			whitesTotal++
		}
	}
	bed.Get("style").Set("width", "calc("+strconv.Itoa(whitesTotal)+" * 16px * var(--kscale,1))")

	whites := doc.Call("createElement", "span")
	whites.Set("className", "pk-whites")
	blacks := doc.Call("createElement", "span")
	blacks.Set("className", "pk-blacks")
	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	blackW := 62.0 / float64(whitesTotal) // % of bed width, ~0.62 white keys

	whitesBefore := 0
	for m := lo; m <= hi; m++ {
		midi := m
		el := doc.Call("createElement", "span")
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
		if off := midi - keysAnchor; off >= 0 {
			lab := ""
			if off >= 12 && off-12 < len(kbHighKeys) {
				lab = kbHighKeys[off-12]
			} else if off < 12 {
				lab = kbLowKeys[off]
			}
			if lab != "" {
				sp := doc.Call("createElement", "span")
				sp.Set("className", "pk-kb")
				sp.Set("textContent", lab)
				el.Call("appendChild", sp)
			}
		}
		keysKeyEls[midi] = el

		el.Call("addEventListener", "mousedown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			e := a[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			keysMouseNote = midi
			keysNoteOn(midi)
			return nil
		}))
		el.Call("addEventListener", "mouseenter", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			e := a[0]
			// Glissando: only while a bed drag is in progress with the button
			// still down (buttons==0 heals a mouseup we never saw).
			if keysMouseNote < 0 {
				return nil
			}
			if int(e.Get("buttons").Float())&1 == 0 {
				keysNoteOff(keysMouseNote)
				keysMouseNote = -1
				return nil
			}
			if keysMouseNote != midi {
				keysNoteOff(keysMouseNote)
				keysMouseNote = midi
				keysNoteOn(midi)
			}
			return nil
		}))
		el.Call("addEventListener", "touchstart", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			keysNoteOn(midi)
			return nil
		}))
		for _, ev := range []string{"touchend", "touchcancel"} {
			el.Call("addEventListener", ev, trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
				keysNoteOff(midi)
				return nil
			}))
		}
	}
	bed.Call("appendChild", whites)
	bed.Call("appendChild", blacks)
}

// ── Voice engine ─────────────────────────────────────────────────────────

// keysEnsureGraph acquires the shared context (we're inside a user gesture:
// a key click or keydown) and lazily builds the master gain → panner chain.
func keysEnsureGraph() js.Value {
	ctx := acquireAudioCtx("keys")
	if !ctx.Truthy() {
		return js.Undefined()
	}
	if !keysMaster.Truthy() {
		keysMaster = ctx.Call("createGain")
		keysPanNode = ctx.Call("createStereoPanner")
		keysMaster.Call("connect", keysPanNode)
		keysPanNode.Call("connect", ctx.Get("destination"))
	}
	keysCtx = ctx
	keysUpdateRouting()
	return ctx
}

// keysUpdateRouting pushes the out ring + level knob into the master chain.
func keysUpdateRouting() {
	if !keysMaster.Truthy() {
		return
	}
	lvl := fgFloat(doc.Call("getElementById", "keys-lvl")) / 100
	gain, pan := 0.0, 0.0
	switch doc.Call("getElementById", "keys-out").Get("value").String() {
	case "l":
		gain, pan = lvl, -1
	case "r":
		gain, pan = lvl, 1
	case "both":
		gain, pan = lvl, 0
	}
	keysMaster.Get("gain").Set("value", gain*0.25) // headroom for chords
	keysPanNode.Get("pan").Set("value", pan)
}

func keysNoteOn(midi int) {
	if _, held := keysVoices[midi]; held {
		return
	}
	if el, ok := keysKeyEls[midi]; ok {
		el.Get("classList").Call("add", "held")
	}
	ctx := keysEnsureGraph()
	if !ctx.Truthy() {
		return
	}
	hz := 440 * math.Pow(2, float64(midi-69)/12)
	w, _ := strconv.Atoi(doc.Call("getElementById", "keys-wave").Get("value").String())
	var osc js.Value
	if w == 4 {
		// Noise voice: the DCSG shift-register loop, pitched by playback rate
		// so the keyboard plays tuned noise (chiff/percussion territory).
		osc = ctx.Call("createBufferSource")
		osc.Set("buffer", genNoiseBuffer(ctx))
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
	g.Call("connect", keysMaster)
	osc.Call("start")
	keysVoices[midi] = osc
	keysGains[midi] = g
}

func keysNoteOff(midi int) {
	if el, ok := keysKeyEls[midi]; ok {
		el.Get("classList").Call("remove", "held")
	}
	osc, held := keysVoices[midi]
	if !held {
		return
	}
	g := keysGains[midi].Get("gain")
	delete(keysVoices, midi)
	delete(keysGains, midi)
	now := keysCtx.Get("currentTime").Float()
	g.Call("cancelScheduledValues", now)
	g.Call("setValueAtTime", g.Get("value"), now)
	g.Call("linearRampToValueAtTime", 0, now+0.12)
	osc.Call("stop", now+0.16)
}

func keysAllOff() {
	for m := range keysVoices {
		keysNoteOff(m)
	}
	keysMouseNote = -1
}

// setKeysOn shows/hides the module; hiding silences everything and drops
// the audio-context lease (the graph stays for the next resume).
func setKeysOn(on bool) {
	keysOn = on
	if sect := doc.Call("getElementById", "keys-module"); sect.Truthy() {
		if on {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if !on {
		keysAllOff()
		if keysCtx.Truthy() {
			releaseAudioCtx("keys")
			keysCtx = js.Undefined()
		}
	}
	quantizeModuleWidths()
}

// ── Wiring ───────────────────────────────────────────────────────────────

// wireKeysModule builds the three control cells (range, level, out/voice),
// renders the keybed, and installs the global play listeners. Called once
// from Run.
func wireKeysModule() {
	span := doc.Call("getElementById", "keys-span")
	base := doc.Call("getElementById", "keys-base")
	lvl := doc.Call("getElementById", "keys-lvl")
	lvlLED := doc.Call("getElementById", "keys-lvl-led")
	out := doc.Call("getElementById", "keys-out")
	wave := doc.Call("getElementById", "keys-wave")
	sw := doc.Call("getElementById", "keys-on")
	rstack := doc.Call("getElementById", "keys-rstack")
	lstack := doc.Call("getElementById", "keys-lstack")
	ostack := doc.Call("getElementById", "keys-ostack")
	sizeLED := doc.Call("getElementById", "keys-size-led")
	if !span.Truthy() || !rstack.Truthy() {
		return
	}

	// Range cell: outer ring = key count, inner knob = starting octave.
	rstk := stackKnobs(makeSelectorKnob(span), makeSelectorKnob(base))
	addSelectorLabels(rstk, []string{"13", "25", "37", "49", "61", "85", "88"}, span, 50)
	addSelectorLabels(rstk, []string{"C1", "C2", "C3", "C4", "C5"}, base, 36)
	rstack.Call("appendChild", rstk)

	// Level cell: standard value knob + LED.
	lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
	sizeLEDField(lvlLED, 0, 100, 1, false)
	lstack.Call("appendChild", makeKnob(lvl, js.Undefined(), true, false, true))

	// Out cell: same anatomy as the Gen oscillators — outer ring = speaker
	// routing, inner knob = waveform with the glyph dial.
	ostk := stackKnobs(makeSelectorKnob(out), makeSelectorKnob(wave))
	addSelectorLabels(ostk, []string{"off", "L", "R", "L+R"}, out, 50)
	addSelectorWaveDial(ostk, wave, 38)
	ostack.Call("appendChild", ostk)

	refreshSize := func() {
		txt := "88"
		if v := span.Get("value").String(); v != "88" {
			n, _ := strconv.Atoi(v)
			txt = strconv.Itoa(12*n + 1)
		}
		sizeLED.Set("textContent", txt)
	}
	rebuild := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		refreshSize()
		buildKeysBed()
		quantizeModuleWidths()
		return nil
	})
	span.Call("addEventListener", "change", rebuild)
	base.Call("addEventListener", "change", rebuild)

	out.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		keysUpdateRouting()
		return nil
	}))
	wave.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		w, _ := strconv.Atoi(wave.Get("value").String())
		if w == 4 {
			return nil // node-type change applies to NEW notes; held ones finish as-is
		}
		for _, osc := range keysVoices { // retype held periodic voices live
			if osc.Get("type").Truthy() { // BufferSource (noise) has no type
				osc.Set("type", waveTypeName(w))
			}
		}
		return nil
	}))
	lvl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		lvlLED.Set("value", formatLED(fgFloat(lvl), intDigits(100), 1, false))
		keysUpdateRouting()
		return nil
	}))
	lvlLED.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(lvlLED.Get("value").String(), 64); err == nil {
			lvl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
			lvl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		return nil
	}))

	if sw.Truthy() {
		sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			setKeysOn(sw.Get("checked").Bool())
			return nil
		}))
	}

	// Computer keyboard: two tracker rows anchored near the range's middle
	// C. Only while the module is shown, never while typing in a field.
	doc.Call("addEventListener", "keydown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		if !keysOn || e.Get("repeat").Bool() ||
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
		midi := keysAnchor + off
		if lo, hi := keysRange(); midi < lo || midi > hi {
			return nil
		}
		e.Call("preventDefault")
		keysNoteOn(midi)
		return nil
	}))
	doc.Call("addEventListener", "keyup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if off, ok := kbOffset[strings.ToLower(a[0].Get("key").String())]; ok {
			keysNoteOff(keysAnchor + off)
		}
		return nil
	}))
	// Release the mouse-drag note anywhere; silence everything on tab blur
	// so no note can stick when focus leaves.
	doc.Call("addEventListener", "mouseup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if keysMouseNote >= 0 {
			keysNoteOff(keysMouseNote)
			keysMouseNote = -1
		}
		return nil
	}))
	js.Global().Call("addEventListener", "blur", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		keysAllOff()
		return nil
	}))

	refreshSize()
	buildKeysBed()
}
