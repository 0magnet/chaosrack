//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/rackspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// The Keys module: a playable polyphonic keyboard — the first slice of the
// modular-synth direction. A Window-group switch (Keys) shows a module whose
// left column is two standard cells (level knob, out/voice knob) and whose
// body is the whole piano, 88 keys from A0 to C8, across the rest of the
// row. Keys play with
// the mouse (click, or hold and glissando), touch, or the computer keyboard
// using the classic two-row tracker map (Z row = lower octave, Q row =
// upper); the mapped keys carry small letter labels. Each held note is a voice
// (keysvoice_js.go): an instrument — piano, electric piano, harpsichord,
// organ — or one of the generators' waveforms, through a master gain +
// stereo panner on the shared audio context, with the same out-cell routing
// (off / L / R / L+R) as the Gen oscillators, so the module reads as one of
// them.

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
	ctx       js.Value          // shared ctx while the lease is held
	master    js.Value          // master gain (the level), into the Mixer's KEYS
	voices    map[int]*keyVoice // held midi note → its voice (keysvoice_js.go)
	keyEls    map[int]js.Value  // midi note → key element (highlight)
	anchor    int               // midi note the Z row's C lands on
	mouseNote int               // note held by the current mouse drag
	touchNote int               // note held by the current touch (glissando)
	// Where and when the drag or touch was last seen, for the glide between
	// that and the next event (glide).
	lastX, lastY, lastT float64
	// Made once and shared by every note: the organ's waveform, and the
	// burst of noise that is the piano's hammer.
	organWave, hammerBuf js.Value
}

var keys = keyboard{
	voices:    map[int]*keyVoice{},
	keyEls:    map[int]js.Value{},
	mouseNote: -1,
	touchNote: -1,
}

// keysRange returns the keybed's midi range: the whole piano, A0..C8. It was
// a range knob, from 13 keys up; with a bay to itself the bed has room for
// all of them, and a knob that took keys away was only a way to have fewer.
func keysRange() (lo, hi int) {
	return 21, 108
}

// rowRestWidth is the CSS width of what fills a module's row beside its knob
// columns: every slot of the row but the taken ones, less the filler's own
// margins and two millimeters of play, so rounding cannot tip the module
// into a slot more than the row has. The Keys bed and the Matrix grid.
func rowRestWidth(taken int) string {
	n := racksurface.UnitCapacity() - taken
	px := float64(n)*moduleSlot - 12 - 2*rackspec.PxPerMM
	return "calc(" + strconv.FormatFloat(px, 'f', 1, 64) + "px * var(--kscale,1) + " + strconv.FormatFloat(float64(n-1)*moduleGap, 'f', 1, 64) + "px)"
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
// boundaries, letter labels on the computer-keyboard-mapped span. The bed is
// the rest of the row (rowRestWidth), so a white key is as wide as the
// row has room for.
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
	bed.Get("style").Set("width", rowRestWidth(0)) // the whole bay: its knobs are the Synth bay's

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
		el.Set("title", docf("keys-key", "name", name))
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

		dom.On(el, "mousedown", func(this js.Value, a []js.Value) any {
			e := a[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			k.mouseNote = midi
			k.markDrag(e)
			k.noteOn(midi)
			return nil
		})
		dom.On(el, "touchstart", func(this js.Value, a []js.Value) any {
			a[0].Call("preventDefault")
			k.touchNote = midi
			k.markDrag(a[0].Get("touches").Index(0))
			k.noteOn(midi)
			return nil
		})
		for _, ev := range []string{"touchend", "touchcancel"} {
			dom.On(el, ev, func(this js.Value, a []js.Value) any {
				// The touch may have slid to another key (glissando below) —
				// release whichever note the finger ended on, and the origin.
				k.noteOff(midi)
				if k.touchNote >= 0 && k.touchNote != midi {
					k.noteOff(k.touchNote)
				}
				k.touchNote = -1
				return nil
			})
		}
	}
	bed.Call("appendChild", whites)
	bed.Call("appendChild", blacks)
	// Glissando, by the bed rather than by each key: a key only hears about
	// the pointer when it lands on it, and a fast drag lands on few of the
	// keys it crosses (glide). Mouse only while a drag is in progress with the
	// button still down (buttons==0 heals a mouseup we never saw); touchmove
	// keeps targeting the starting key, so the finger is tracked the same way.
	dom.On(bed, "mousemove", func(this js.Value, a []js.Value) any {
		e := a[0]
		if k.mouseNote < 0 {
			return nil
		}
		if int(e.Get("buttons").Float())&1 == 0 {
			k.noteOff(k.mouseNote)
			k.mouseNote = -1
			return nil
		}
		k.glide(e.Get("clientX").Float(), e.Get("clientY").Float(), &k.mouseNote)
		return nil
	})
	dom.On(bed, "touchmove", func(this js.Value, a []js.Value) any {
		e := a[0]
		e.Call("preventDefault")
		if k.touchNote < 0 {
			return nil
		}
		t := e.Get("touches").Index(0)
		k.glide(t.Get("clientX").Float(), t.Get("clientY").Float(), &k.touchNote)
		return nil
	})
}

// ── Voice engine ─────────────────────────────────────────────────────────

// ensureGraph acquires the shared context (we're inside a user gesture:
// a key click or keydown) and lazily builds the master gain into the Mixer.
func (k *keyboard) ensureGraph() js.Value {
	ctx := acquireAudioCtx("keys")
	if !ctx.Truthy() {
		return js.Undefined()
	}
	if !k.master.Truthy() {
		k.master = ctx.Call("createGain")
		mixEnsure(ctx)
		k.master.Call("connect", mixIn("ky"))
	}
	k.ctx = ctx
	k.updateRouting()
	return ctx
}

// updateRouting pushes the level knob into the master gain; where it goes is
// the Mixer's.
func (k *keyboard) updateRouting() {
	if !k.master.Truthy() {
		return
	}
	lvl := fgFloat(dom.Doc.Call("getElementById", "keys-lvl")) / 100
	k.master.Get("gain").Set("value", lvl*0.25) // headroom for chords
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
	k.voices[midi] = k.startVoice(ctx, midi, ctx.Get("currentTime").Float())
}

// passLitWhite and passLitBlack are a held key's look (panel.css .held), for
// the moment a passing note sounds.
var (
	passLitWhite = map[string]any{"background": "linear-gradient(#c8ffe2,#5fd894)", "boxShadow": "0 0 8px rgba(70,240,140,.55)"}
	passLitBlack = map[string]any{"background": "linear-gradient(#2f9a5e,#0b3a20)", "boxShadow": "0 0 8px rgba(70,240,140,.55)"}
)

// passNote sounds a key the drag went over on its way to the one it is on:
// started at t0 and released after dur, on the audio clock, with the key lit
// for as long. Not a held voice — nothing lets go of it but its own release.
func (k *keyboard) passNote(midi int, t0, dur float64) {
	ctx := k.ensureGraph()
	if !ctx.Truthy() {
		return
	}
	k.startVoice(ctx, midi, t0).release(t0+dur, false)
	// Lit by the Web Animations API, on the note's own schedule, rather than
	// by a timer per key: nothing on the Go side to call back and free.
	if el, ok := k.keyEls[midi]; ok {
		lit := passLitWhite
		if keysIsBlack(midi) {
			lit = passLitBlack
		}
		lead := (t0 - ctx.Get("currentTime").Float()) * 1000
		el.Call("animate", js.ValueOf([]any{lit, lit}), js.ValueOf(map[string]any{
			"delay": max(lead, 0), "duration": max(dur*1000, 60),
		}))
	}
}

// glide moves a drag's note to the key under (x, y), sounding every key on
// the straight line from where the pointer was last seen.
//
// One key per event was what played. A pointer moving faster than a key per
// frame crosses several between two events, and only the one it landed on
// ever heard about it: drag fast and the run skipped keys. So the line is
// walked a few pixels at a time, every key on it is collected in order, and
// the ones before the last are played as passing notes spread across the time
// since the last event — a run, not a chord, at the speed the hand went.
func (k *keyboard) glide(x, y float64, note *int) {
	now := js.Global().Get("performance").Call("now").Float() / 1000
	x0, y0, dt := k.lastX, k.lastY, now-k.lastT
	k.lastX, k.lastY, k.lastT = x, y, now
	var path []int
	steps := int(math.Hypot(x-x0, y-y0)/3) + 1
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		m, ok := k.midiAt(x0+(x-x0)*f, y0+(y-y0)*f)
		if !ok || m == *note || (len(path) > 0 && path[len(path)-1] == m) {
			continue
		}
		path = append(path, m)
	}
	if len(path) == 0 {
		return
	}
	last := path[len(path)-1]
	if *note >= 0 {
		k.noteOff(*note)
	}
	*note = last
	k.noteOn(last)
	passing := path[:len(path)-1]
	v, held := k.voices[last]
	if len(passing) == 0 || !k.ctx.Truthy() || !held {
		return
	}
	dt = min(max(dt, 0.02), 0.15)
	per := dt / float64(len(path))
	t := k.ctx.Get("currentTime").Float()
	for i, m := range passing {
		k.passNote(m, t+float64(i)*per, per)
	}
	// The key it lands on is heard after the run, not under it: its attack
	// moved to the end of the run, on the audio clock.
	at := t + float64(len(passing))*per
	gg := v.g.Get("gain")
	gg.Call("cancelScheduledValues", t)
	gg.Call("setValueAtTime", 0, t)
	gg.Call("setValueAtTime", 0, at)
	gg.Call("linearRampToValueAtTime", 1, at+0.008)
}

// markDrag records where a drag or touch began, which is where the first
// glide is walked from.
func (k *keyboard) markDrag(p js.Value) {
	k.lastX, k.lastY = p.Get("clientX").Float(), p.Get("clientY").Float()
	k.lastT = js.Global().Get("performance").Call("now").Float() / 1000
}

// midiAt is the key under a point on the page.
func (k *keyboard) midiAt(x, y float64) (int, bool) {
	el := dom.Doc.Call("elementFromPoint", x, y)
	if !el.Truthy() {
		return 0, false
	}
	if !el.Call("hasAttribute", "data-midi").Bool() {
		el = el.Call("closest", "[data-midi]")
		if !el.Truthy() {
			return 0, false
		}
	}
	m, err := strconv.Atoi(el.Call("getAttribute", "data-midi").String())
	return m, err == nil
}

func (k *keyboard) noteOff(midi int) {
	if el, ok := k.keyEls[midi]; ok {
		el.Get("classList").Call("remove", "held")
	}
	v, held := k.voices[midi]
	if !held {
		return
	}
	delete(k.voices, midi)
	v.release(k.ctx.Get("currentTime").Float(), true)
}

func (k *keyboard) allOff() {
	for m := range k.voices {
		k.noteOff(m)
	}
	k.mouseNote = -1
}

// ── Wiring ───────────────────────────────────────────────────────────────

// wireKeysModule builds the Synth bay, renders the keybed, and installs the
// global play listeners. Called once from Run.
func (k *keyboard) wireKeysModule() {
	// What the keyboard sounds like, how loud and where: the Synth bay above
	// it (synth_js.go), built before the keys so its knobs are there to play.
	buildSynthBay()

	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	k.on = true

	// Computer keyboard: two tracker rows anchored near the range's middle
	// C. Only while the module is shown, never while typing in a field.
	dom.On(dom.Doc, "keydown", func(this js.Value, a []js.Value) any {
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
	})
	dom.On(dom.Doc, "keyup", func(this js.Value, a []js.Value) any {
		if off, ok := kbOffset[strings.ToLower(a[0].Get("key").String())]; ok {
			k.noteOff(k.anchor + off)
		}
		return nil
	})
	// Release the mouse-drag note anywhere; silence everything on tab blur
	// so no note can stick when focus leaves.
	dom.On(dom.Doc, "mouseup", func(this js.Value, a []js.Value) any {
		if k.mouseNote >= 0 {
			k.noteOff(k.mouseNote)
			k.mouseNote = -1
		}
		return nil
	})
	dom.On(js.Global(), "blur", func(this js.Value, a []js.Value) any {
		k.allOff()
		return nil
	})

	k.buildKeysBed()
}
