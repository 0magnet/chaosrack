//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/skirt"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// Frequency range for the generators: exactly 10 octaves anchored to notes,
// A0 (27.5 Hz, the lowest key on a piano) → A10 (28160 Hz). The knob's hidden
// slider is linear 0..genSemitones with ONE UNIT PER SEMITONE, and frequency
// maps exponentially, so equal turn = equal octaves (a logarithmic knob) and one
// coarse step / scroll click = one semitone. Because the low end is a real note
// in A440 tuning, every detent lands on a concert-pitch note — turning the knob
// steps through every note of the chromatic scale (12 per octave). 120 divides
// the range cleanly, so the slider reaches its max exactly (→ 28160, no short).

// addOctaveDial draws a tick ring around the (log) frequency knob with one tick
// per octave — so equal turn = equal octaves reads at a glance — plus the min /
// max Hz labeled at the sweep ends. Styled like the value dial (behind the
// knob, decorative). The octave ticks land at genFreqLo·2ⁿ, which are evenly
// spaced around the sweep because the knob is logarithmic.
func addOctaveDial(wrap js.Value) {
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial value-dial")
	nOct := int(math.Log2(genFreqHi / genFreqLo)) // whole octaves in range
	for n := 0; n <= nOct; n++ {
		f := genFreqLo * math.Pow(2, float64(n))
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*knobFromFreq(f)/genSemitones
		l, tp := dialLabelPos(deg, 41)
		tk := dom.Doc.Call("createElement", "span")
		cls := "vdial-tick"
		if n%4 == 0 { // a longer tick every 4 octaves for a readable rhythm
			cls += " major"
		}
		tk.Set("className", cls)
		st := tk.Get("style")
		st.Set("left", l)
		st.Set("top", tp)
		st.Set("transform", "translate(-50%,-50%) rotate("+strconv.FormatFloat(deg, 'f', 1, 64)+"deg)")
		dial.Call("appendChild", tk)
	}
	// Note names at the sweep ends (both in the lower half, clear of the LED).
	// Every octave tick is an A, so the ends are A0 and A10; the LED shows exact
	// Hz.
	for _, e := range []struct {
		f float64
		s string
	}{{genFreqLo, "A0"}, {genFreqHi, "A10"}} {
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*knobFromFreq(e.f)/genSemitones
		l, tp := dialLabelPos(deg, 48)
		lab := dom.Doc.Call("createElement", "span")
		lab.Set("className", "knob-dial-lab")
		lab.Set("textContent", e.s)
		// Which end, and what it is in hertz — the ring says A0 and A10 because
		// every tick between them is an A, and the number is the thing a
		// measurement actually needs.
		end := "the lowest"
		if e.f > genFreqLo {
			end = "the highest"
		}
		lab.Set("title", e.s+" — "+strconv.FormatFloat(e.f, 'f', -1, 64)+" Hz, "+end+" this knob goes")
		lab.Get("style").Set("left", l)
		lab.Get("style").Set("top", tp)
		dial.Call("appendChild", lab)
	}
	wrap.Call("insertBefore", dial, wrap.Get("firstChild"))
	wrap.Get("classList").Call("add", "has-dial")
}

// addPianoKeys builds a one-octave piano strip under the frequency knob and
// lights the key of the note the knob is currently on (pitch class, A=0 to match
// semitones above A0). Clicking a key sets that note in the octave currently
// shown. It's a live note indicator for the semitone-stepped log knob.
func addPianoKeys(freq js.Value) js.Value {
	// Key tooltips carry the owning oscillator so the three keyboards (and
	// their twelve notes each) stay globally unique.
	owner := "Gen"
	if id := freq.Get("id").String(); len(id) > 4 && id[:4] == "gen-" {
		owner = "Gen " + strings.ToUpper(id[4:5])
	}
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "gen-piano")
	wrap.Call("setAttribute", "data-no-drag", "")
	whites := dom.Doc.Call("createElement", "span")
	whites.Set("className", "pk-whites")
	blacks := dom.Doc.Call("createElement", "span")
	blacks.Set("className", "pk-blacks")

	// Pitch classes with A=0 (0=A,1=A#,2=B,3=C,…). White keys C..B left→right;
	// black keys sit on the white-key boundaries that have them.
	whitePC := []int{3, 5, 7, 8, 10, 0, 2}
	whiteName := []string{"C", "D", "E", "F", "G", "A", "B"}
	blackPC := []int{4, 6, 9, 11, 1}
	blackName := []string{"C#", "D#", "F#", "G#", "A#"}
	blackCenter := []float64{1.0 / 7, 2.0 / 7, 4.0 / 7, 5.0 / 7, 6.0 / 7} // white-boundary fractions
	const blackW = 9.0                                                    // % of strip width

	var keyEls []js.Value
	mk := func(pc int, name string, black bool, leftPct float64) js.Value {
		el := dom.Doc.Call("createElement", "span")
		if black {
			el.Set("className", "pk-key pk-black")
			el.Get("style").Set("left", strconv.FormatFloat(leftPct, 'f', 2, 64)+"%")
		} else {
			el.Set("className", "pk-key pk-white")
		}
		el.Call("setAttribute", "data-pc", strconv.Itoa(pc))
		el.Set("title", owner+" — set note "+name+" (in the octave currently shown)")
		el.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			// Set this note in the octave currently shown on the keyboard (the C..B
			// register the current note is in), snapping out any detune.
			cur := math.Round(fgFloat(freq))           // current note, semitones above A0
			off := ((pc-3)%12 + 12) % 12               // this key's offset above the register's C
			regC := 3 + 12*int(math.Floor((cur-3)/12)) // C of the current note's octave
			s := float64(regC + off)
			for s < 0 {
				s += 12
			}
			for s > genSemitones {
				s -= 12
			}
			freq.Set("value", strconv.FormatFloat(s, 'f', 0, 64))
			freq.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			return nil
		}))
		keyEls = append(keyEls, el)
		return el
	}
	for i, pc := range whitePC {
		whites.Call("appendChild", mk(pc, whiteName[i], false, 0))
	}
	for i, pc := range blackPC {
		blacks.Call("appendChild", mk(pc, blackName[i], true, blackCenter[i]*100-blackW/2))
	}
	wrap.Call("appendChild", whites)
	wrap.Call("appendChild", blacks)

	highlight := func() {
		// The hidden slider is in semitones, so the fractional part IS the detune:
		// off ∈ [-0.5,+0.5] semitone = ∓50…+50 cents from the nearest note.
		sv := fgFloat(freq)
		nearest := math.Round(sv)
		off := sv - nearest
		pc := ((int(nearest) % 12) + 12) % 12
		want := strconv.Itoa(pc)
		// Tuning gradient: full green at dead-center; as you detune, red grows on
		// the side you're heading toward — right when sharp (off>0), left when flat
		// (off<0) — flipping at the ±50¢ boundary where the next note takes over.
		a := math.Min(1, math.Abs(off)*2) // 0 in-tune … 1 at the boundary
		var grad string
		if off >= 0 { // sharp → green left, red grows right
			grad = "linear-gradient(90deg,#16f06a 0%,#16f06a " + strconv.FormatFloat((1-a)*100, 'f', 0, 64) + "%,#ff2410 100%)"
		} else { // flat → red grows left, green right
			grad = "linear-gradient(90deg,#ff2410 0%,#16f06a " + strconv.FormatFloat(a*100, 'f', 0, 64) + "%,#16f06a 100%)"
		}
		for _, el := range keyEls {
			lit := el.Call("getAttribute", "data-pc").String() == want
			el.Get("classList").Call("toggle", "lit", lit)
			if lit {
				el.Get("style").Set("background", grad)
			} else {
				el.Get("style").Set("background", "")
			}
		}
	}
	highlight()
	freq.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any {
		highlight()
		return nil
	}))
	return wrap
}

// The Generator modules: four independent oscillators, Gen 1 to 4, each a
// frequency knob with a Hz LED, a level with its envelope, and a waveform
// knob with buttons 1 to 4 for the channels it is patched to, and inv. Each
// drives the shared FuncGen, which the bus patches into the rack's signal, and
// a parallel Web Audio graph for the speakers.

// genOscSpec is one oscillator of the signal generator: which DOM ids belong to it,
// which slot of the function generator it drives, and where its two knobs sit
// when nothing has been touched.
//
// One type because the same three oscillators were written out three times —
// genOscIDs for the ids, genOscIdx for the index, and an anonymous struct
// inside onResetAll carrying the defaults that the markup also carries. Three
// places to keep in step, and the defaults were already stated twice: once as
// the range input's value attribute and once in the reset. A ControlDesc's Def
// is now the only copy, and Reset All restores through the control rather than
// by writing remembered numbers back into the DOM.
type genOscSpec struct {
	id   string  // DOM id prefix: "gen-x" → gen-x-freq, gen-x-lvl, gen-x-wave…
	idx  int     // slot in the function generator
	freq float64 // default frequency knob position, in semitones above A0
	lvl  float64 // default level, 0..100
	wave string  // where its waveform knob starts: "0", a sine, or "off"
}

// genOscs are the oscillators, by their index in the function generator
// (genOscs[i].idx == i: the audio graph looks them up by index).
var genOscs = []genOscSpec{
	// Playing from the start and patched nowhere, so each scope beside one
	// shows it, and nothing reaches the channels until somebody patches it.
	{id: "gen-x", idx: 0, freq: 34, lvl: 80, wave: "0"},
	{id: "gen-y", idx: 1, freq: 41, lvl: 80, wave: "0"},
	{id: "gen-z", idx: 2, freq: 29, lvl: 80, wave: "0"},
	// V starts silent (audiosrc.NewFuncGen says why), on B3: with X, Y and
	// Z, a G-major chord.
	{id: "gen-v", idx: 3, freq: 38, lvl: 0, wave: "off"},
}

// key is the link key of the generator's part named p: g1f is Gen 1's
// frequency.
func (o genOscSpec) key(p string) string { return "g" + o.letter() + p }

// letter is the oscillator's name on the panel, "1" for gen-x: the generators
// are Gen 1 to 4, by their index, and keep their old letters in their ids.
func (o genOscSpec) letter() string { return strconv.Itoa(o.idx + 1) }

// genWaveOff is which generators have their waveform knob at OFF: playing
// nothing, which a scope probing one of them shows (rackScope.feed).
var genWaveOff [audiosrc.OscCount]bool

// genWaves are the waveform ring's positions, index = audiosrc.Wave*: the
// short name on the readout and what each is. The four stimuli's say what
// the library says they are for (audiosrc.TestSignalDescs).
var genWaves = func() []struct{ name, help string } {
	w := []struct{ name, help string }{
		{"sine", doc("gen-wave=sine")},
		{"tri", doc("gen-wave=tri")},
		{"sqr", doc("gen-wave=sqr")},
		{"saw", doc("gen-wave=saw")},
		{"noise", doc("gen-wave=noise")},
		{"white", ""}, {"pink", ""}, {"swp", ""}, {"pulse", ""},
	}
	for i := range w {
		if sig, ok := audiosrc.WaveStim(i); ok {
			w[i].help = audiosrc.TestSignalDescs[sig] + ". The freq knob does not change it"
		}
	}
	return w
}()

// fillSelect gives a hidden select its options.
func fillSelect(sel js.Value, def string, vals, texts, titles []string) {
	for i, v := range vals {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", v)
		opt.Set("textContent", texts[i])
		opt.Set("title", titles[i])
		if v == def {
			opt.Set("selected", true)
		}
		sel.Call("appendChild", opt)
	}
	sel.Set("value", def)
}

// waveSVG holds a tiny glyph per waveform (index = audiosrc.Wave*: sine,
// triangle, square, saw, shift-register noise, then the stimuli white, pink,
// sweep and pulse), stroked in currentColor so CSS can dim the ring and light
// the active one.
var waveSVG = []string{
	`<svg viewBox="0 0 24 12"><path d="M1,6 C3.2,1 5.8,1 8,6 C10.2,11 12.8,11 15,6 C17.2,1 19.8,1 22,6" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L7,2 L12,10 L17,2 L22,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L2,3 L8.5,3 L8.5,10 L15,10 L15,3 L21.5,3 L21.5,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M2,10 L8,3 L8,10 L14,3 L14,10 L20,3 L20,10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
	`<svg viewBox="0 0 24 12"><path d="M1,7 L3,7 L3,3 L5,3 L5,9 L8,9 L8,4 L10,4 L10,2 L13,2 L13,8 L15,8 L15,5 L18,5 L18,10 L20,10 L20,6 L23,6" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>`,
	// white: dense and even
	`<svg viewBox="0 0 24 12"><path d="M1,6 L2,3 L3,9 L4,2 L5,8 L6,4 L7,10 L8,3 L9,7 L10,1 L11,9 L12,5 L13,11 L14,2 L15,8 L16,4 L17,9 L18,3 L19,7 L20,2 L21,10 L22,5 L23,6" fill="none" stroke="currentColor" stroke-width="1" stroke-linejoin="round"/></svg>`,
	// pink: the same, its highs rolled off
	`<svg viewBox="0 0 24 12"><path d="M1,7 C3,2 4,10 6,5 S9,2 11,7 S14,10 16,6 S19,1 21,5 S22,8 23,6" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>`,
	// sweep: a sine rising in pitch
	`<svg viewBox="0 0 24 12"><path d="M1,6 C2.5,0 5,0 7,6 C8.5,12 10,12 11,6 C12,1 13,1 14,6 C14.8,11 15.6,11 16.3,6 C16.9,2 17.5,2 18,6 C18.4,10 18.9,10 19.3,6 C19.7,3 20.1,3 20.4,6 C20.7,9 21,9 21.3,6 C21.6,4 21.9,4 22.2,6" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>`,
	// pulse: a spike one way, a long shallow recovery the other
	`<svg viewBox="0 0 24 12"><path d="M1,8 L4,8 L5,1 L6,8 Q9,11 12,8 L13,8 L14,1 L15,8 Q18,11 21,8 L23,8" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>`,
}

// addSelectorWaveDial adds a ring of waveform glyphs around a selector stack
// (one per option, at the knob's detent angles, all the way round for an
// endless one), lighting the active one and clickable to select — a graphic
// counterpart to addSelectorLabels for a waveform knob.
func addSelectorWaveDial(stack, sel js.Value, off float64) {
	// One glyph per OPTION (not per known waveform) — a select offering only
	// the periodic waves must not grow a phantom noise detent. An option that
	// is not a waveform (OFF) is its name in type.
	opts := sel.Get("options")
	n := min(opts.Get("length").Int(), len(waveSVG)+1)
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial")
	circle := dom.Doc.Call("createElement", "span")
	circle.Set("className", "knob-ring-circle")
	dia := strconv.FormatFloat(2*off, 'f', 1, 64) + "%"
	circle.Get("style").Set("width", dia)
	circle.Get("style").Set("height", dia)
	dial.Call("appendChild", circle)
	els := make([]js.Value, n)
	for i := range n {
		l, t := dialLabelPos(labelDeg(sel, i, n), off)
		ic := dom.Doc.Call("createElement", "span")
		if w, err := strconv.Atoi(opts.Index(i).Get("value").String()); err == nil && w >= 0 && w < len(waveSVG) {
			ic.Set("className", "knob-dial-wave clickable")
			ic.Set("innerHTML", waveSVG[w])
		} else {
			ic.Set("className", "knob-dial-lab clickable")
			ic.Set("textContent", opts.Index(i).Get("text").String())
		}
		ic.Get("style").Set("left", l)
		ic.Get("style").Set("top", t)
		dialPosTitle(ic, sel, i)
		els[i] = ic
		idx := i
		ic.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			sel.Set("selectedIndex", idx)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			return nil
		}))
		dial.Call("appendChild", ic)
	}
	hi := func() {
		ci := sel.Get("selectedIndex").Int()
		for j, e := range els {
			e.Get("classList").Call("toggle", "wave-active", j == ci)
			e.Get("classList").Call("toggle", "lab-active", j == ci)
		}
	}
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any { hi(); return nil }))
	hi()
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
}

// waveTypeName maps our waveform index to the Web Audio OscillatorNode type.
func waveTypeName(w int) string {
	switch w {
	case 1:
		return "triangle"
	case 2:
		return "square"
	case 3:
		return "sawtooth"
	default:
		return "sine"
	}
}

// buildGeneratorModule wires the per-oscillator modules. Each fills its
// 1-unit module with three cells: a frequency knob (log, equal turn per octave)
// with a Hz LED, a level knob with a % LED, and the waveform knob with the
// channel buttons and inv under it and solo, to hear only it, beside it.
func buildGeneratorModule() {
	for _, osc := range genOscs {
		id, idx := osc.id, osc.idx
		freq := dom.Doc.Call("getElementById", id+"-freq")
		wave := dom.Doc.Call("getElementById", id+"-wave")
		fstack := dom.Doc.Call("getElementById", id+"-fstack")
		lvl := dom.Doc.Call("getElementById", id+"-lvl")
		lstack := dom.Doc.Call("getElementById", id+"-lstack")
		ostack := dom.Doc.Call("getElementById", id+"-ostack")
		if !freq.Truthy() || !fstack.Truthy() {
			continue
		}

		// Freq cell: single log-frequency knob with an octave tick ring. The hidden
		// slider is in semitones (step 1 = one semitone), so scrolling the knob or
		// the LED steps note-by-note (12 per octave); dragging still sweeps
		// continuously and the fine disc trims sub-semitone. The LED is zero-padded
		// to the widest value (so 20480 never clips to "2048") with a fixed decimal,
		// like every other readout — all of which the descriptor below sets up,
		// along with the typed entry and the wheel nudge that used to be written
		// out here beside every other cell that wanted them.
		fknob := makeKnob(freq, js.Undefined(), true, false, false)
		addOctaveDial(fknob)
		fstack.Call("appendChild", fknob)
		// One-octave piano strip under the knob, lighting the current note.
		fstack.Get("parentNode").Get("parentNode").Call("appendChild", addPianoKeys(freq))

		// Level cell: the level, its envelope's ATTACK and DECAY minis under it
		// (genLevelStack), and between the two the env button that repeats it.
		lstack.Call("appendChild", genLevelStack(osc, lvl))
		lcell := lstack.Get("parentNode").Get("parentNode")
		lcell.Get("classList").Call("add", "gen-lvl-cell")
		ecol := trioColumn(trioPrograms[id+"-env"].legends())
		ecol.Call("setAttribute", "data-param", id+"-env")
		if row := lcell.Call("querySelector", ".minirow"); row.Truthy() {
			row.Call("insertBefore", ecol, row.Get("lastChild"))
		}

		// Out cell: the waveform on one knob, OFF at the bottom and the
		// waves round the ring as glyphs, and beside it solo. Where the
		// generator goes is the Mixer's.
		wv, wt := []string{"off"}, []string{"off"}
		wh := []string{"OFF — Gen " + osc.letter() + " plays nothing"}
		for i, w := range genWaves {
			wv, wt, wh = append(wv, strconv.Itoa(i)), append(wt, w.name), append(wh, w.help)
		}
		fillSelect(wave, osc.wave, wv, wt, wh)
		ostk := soloKnob(wave)
		addSelectorWaveDial(ostk, wave, 44)
		ostack.Call("appendChild", ostk)
		cell := ostack.Get("parentNode").Get("parentNode")
		col := trioColumn(trioPrograms[id+"-solo"].legends())
		col.Call("setAttribute", "data-param", id+"-solo")
		cell.Call("appendChild", col)
		// Turning a generator's knob while the Mixer has it nowhere pins it
		// (mixAutoPin). In the capture phase, because a knob stops its own
		// pointer and wheel events from going any further up than itself.
		if m := dom.Doc.Call("getElementById", id+"-module"); m.Truthy() {
			for _, ev := range []string{"pointerdown", "wheel"} {
				m.Call("addEventListener", ev, dom.FuncOf(func(_ js.Value, a []js.Value) any {
					if a[0].Get("target").Call("closest", genTouchParts).Truthy() &&
						!genWaveOff[idx] && aud.fg().Amp(idx) > 0 && mixAutoPin(idx) {
						aud.showAudioStatus(docf("gen-autopin", "gen", osc.letter()))
					}
					return nil
				}), map[string]any{"passive": true, "capture": true})
			}
		}

		// The two value knobs go through the descriptor path, which is what
		// gives them their LED formatting, typed entry, wheel nudge, reset and
		// the Control that Reset All drives — rather than each re-implementing
		// all of it beside the next one. Everything specific to an oscillator
		// is the Apply closure; the rest is the same machinery every parameter
		// cell in the rack already used.
		//
		// The frequency mapping is the one sonify_js.go already defines and,
		// until now, used exactly once: the slider is in semitones and the LED
		// is in hertz, which is precisely what SliderToVal / ValToSlider are
		// for.
		adoptDescControl(ControlDesc{
			ID: id + "-freq", Label: "freq", Min: 0, Max: float64(genSemitones), Step: 1, Def: osc.freq,
			LEDID: id + "-led", ResetID: "rst-" + id + "-freq",
			PermaKey: osc.key("f"),
			Apply: func(v float64) {
				aud.fg().SetFreq(idx, freqFromKnob(v))
				gen.audioUpdate(idx)
			},
			SliderToVal: sonifyFreqFromSlider,
			ValToSlider: sonifySliderFromFreq,
			LEDMin:      genFreqLo, LEDMax: genFreqHi, LEDStep: 1,
		})
		adoptDescControl(ControlDesc{
			ID: id + "-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: osc.lvl,
			LEDID: id + "-lvl-led", ResetID: "rst-" + id + "-lvl",
			PermaKey: osc.key("l"),
			Apply: func(v float64) {
				aud.fg().SetAmp(idx, v/100)
				gen.audioUpdate(idx)
			},
		})
		// The waveform knob goes through the descriptor path for the same
		// reason the two knobs above do. It has no reset of its own: OFF is
		// a position on its ring, at the bottom, and Reset All still puts it
		// back.
		adoptDescControl(ControlDesc{
			ID: id + "-wave", Label: "wave", IsSelect: true, SelectDef: osc.wave,
			PermaKey: osc.key("w"),
			SelectApply: func(v string) {
				genWaveOff[idx] = v == "off"
				aud.fg().SetOn(idx, v != "off")
				if w, err := strconv.Atoi(v); err == nil {
					aud.fg().SetWave(idx, w)
				}
				gen.audioSync()
			},
		})
		// Push the markup's defaults into the generator.
		aud.fg().SetFreq(idx, freqFromKnob(fgFloat(freq)))
		aud.fg().SetAmp(idx, fgFloat(lvl)/100)
	}
	adoptDescControl(ControlDesc{
		ID: "gen-solo", Label: "solo", IsSelect: true, SelectDef: "",
		PermaKey: "gso",
		SelectApply: func(v string) {
			solo := -1
			for _, o := range genOscs {
				if o.id == "gen-"+v {
					solo = o.idx
				}
			}
			aud.fg().SetSolo(solo)
			gen.audioSync()
			for _, o := range genOscs {
				lightTrios(o.id + "-solo")
			}
		},
	})
	syncTrios()
}

// genHeard reports whether generator i reaches the speakers: pinned to one
// on the Mixer, switched on, and not silenced by another's solo.
func genHeard(i int) bool {
	if genWaveOff[i] || !aud.fg().Audible(i) {
		return false
	}
	return mixOnSpeakers(mixSrcIndex("g" + strconv.Itoa(i+1)))
}

// setSelect sets a hidden select the way its knob or button would.
func setSelect(id, v string) {
	if s := dom.Doc.Call("getElementById", id); s.Truthy() {
		s.Set("value", v)
		s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
}

// The solo button, a column of its own (trioColumn).
func init() {
	for _, o := range genOscs {
		letter := o.id[len("gen-"):] // the solo select's value
		trioPrograms[o.id+"-solo"] = trioProgram{
			keys: []string{"solo"},
			help: []string{docf("gen-solo-btn", "gen", o.letter())},
			press: func(int) {
				v := letter
				if s := dom.Doc.Call("getElementById", "gen-solo"); s.Truthy() && s.Get("value").String() == letter {
					v = ""
				}
				setSelect("gen-solo", v)
			},
			lit: func() int {
				if s := dom.Doc.Call("getElementById", "gen-solo"); s.Truthy() && s.Get("value").String() == letter {
					return 0
				}
				return -1
			},
		}
	}
}

// genTouchParts are the parts of a generator that a hand turns or presses.
const genTouchParts = ".knob, .knobstack, .gen-piano, .numin, .knob-dial-wave, .knob-dial-lab"

// generator is the signal generator's audio graph: the speakers' copy of the
// generators. The rack's own signal is audiosrc.Bus's; this plays the same
// parameters out loud, each generator into its column of the Mixer, which
// puts it on the speakers its pins say.
type generator struct {
	ctx      js.Value
	osc      [audiosrc.OscCount]js.Value
	kind     [audiosrc.OscCount]int // nodeKind of gen.osc[i], -1 for none
	gain     [audiosrc.OscCount]js.Value
	env      [audiosrc.OscCount]js.Value // each one's envelope (genEnvTick), level → env → the Mixer
	noiseBuf js.Value                    // shared 2-s LFSR noise loop
	stimBufs map[int]js.Value            // one loop per stimulus wave, made on first use
	running  bool
}

var gen generator

// audioSync starts the Web Audio graph if any oscillator is heard (genHeard),
// stops it if none is, and otherwise just refreshes the running nodes.
func (g *generator) audioSync() {
	found := false
	for i := range genOscs {
		found = found || genHeard(i)
	}
	switch {
	case found && !g.running:
		g.audioStart()
	case !found && g.running:
		g.audioStop()
	case g.running:
		for i := range audiosrc.OscCount {
			g.audioUpdate(i)
		}
	}
}

func fgFloat(el js.Value) float64 {
	v, _ := strconv.ParseFloat(el.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
	return v
}

// ── Web Audio output ──────────────────────────────────────────────────────

// noiseBuffer builds (once) the shift-register noise loop the noise wave
// plays through an AudioBufferSourceNode — the same 15-bit LFSR the FuncGen
// analysis path steps, so what you hear is what the scope sees. The freq
// knob maps to playbackRate, sweeping the noise from rumble to hiss.
func (g *generator) noiseBuffer(ctx js.Value) js.Value {
	if g.noiseBuf.Truthy() {
		return g.noiseBuf
	}
	sr := int(ctx.Get("sampleRate").Float())
	buf := ctx.Call("createBuffer", 1, sr*2, sr)
	data := buf.Call("getChannelData", 0)
	lfsr := uint32(0x4001)
	for i := range sr * 2 {
		bit := (lfsr ^ (lfsr >> 1)) & 1
		lfsr = (lfsr >> 1) | (bit << 14)
		v := -1.0
		if lfsr&1 == 1 {
			v = 1
		}
		data.SetIndex(i, v)
	}
	g.noiseBuf = buf
	return buf
}

// stimBuffer builds (once per wave) a loop of a stimulus wave, synthesized by
// the same library the rack's signal plays it from (audiosrc.StimLoop), so
// the speakers and the scope have the one definition of pink noise.
func (g *generator) stimBuffer(ctx js.Value, w int) js.Value {
	if b, ok := g.stimBufs[w]; ok {
		return b
	}
	sr := int(ctx.Get("sampleRate").Float())
	pts := audiosrc.StimLoop(w, sr)
	buf := ctx.Call("createBuffer", 1, len(pts), sr)
	f32 := js.Global().Get("Float32Array").New(len(pts))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(f32.Get("buffer")), sliceToByteSlice(pts))
	buf.Call("copyToChannel", f32, 0)
	if g.stimBufs == nil {
		g.stimBufs = map[int]js.Value{}
	}
	g.stimBufs[w] = buf
	return buf
}

// ensureNode makes gen.osc[i] the right node for wave w — an OscillatorNode
// for the periodic waves, a looped AudioBufferSourceNode of LFSR noise or of
// a stimulus for the others — replacing the node when it was made for
// another kind.
func (g *generator) ensureNode(i, w int) {
	k := nodeKind(w)
	if g.kind[i] == k && g.osc[i].Truthy() {
		return
	}
	if g.osc[i].Truthy() {
		g.osc[i].Call("stop")
		g.osc[i].Call("disconnect")
	}
	var node js.Value
	_, stim := audiosrc.WaveStim(w)
	switch {
	case w == audiosrc.WaveLFSR:
		node = g.ctx.Call("createBufferSource")
		node.Set("buffer", g.noiseBuffer(g.ctx))
		node.Set("loop", true)
	case stim:
		node = g.ctx.Call("createBufferSource")
		node.Set("buffer", g.stimBuffer(g.ctx, w))
		node.Set("loop", true)
	default:
		node = g.ctx.Call("createOscillator")
	}
	node.Call("connect", g.gain[i])
	node.Call("start")
	g.osc[i], g.kind[i] = node, k
}

// nodeKind is which node a wave plays through: the periodic waves share an
// OscillatorNode, told its type; every other wave has a loop of its own.
func nodeKind(w int) int {
	if w < audiosrc.WaveLFSR {
		return audiosrc.WaveSine
	}
	return w
}

// audioStart builds the Web Audio graph (one node per generator → level →
// the Envelope → its column of the Mixer) and starts it, mirroring the
// generators' params.
func (g *generator) audioStart() {
	if g.running {
		return
	}
	// We're inside a user-gesture handler, so the acquire's resume is allowed
	// under the browser autoplay policy.
	g.ctx = mixAcquire()
	if !g.ctx.Truthy() {
		return
	}
	for i := range audiosrc.OscCount {
		gain := g.ctx.Call("createGain")
		env := g.ctx.Call("createGain") // its envelope (genEnvTick)
		gain.Call("connect", env)
		env.Call("connect", mixIn("g"+strconv.Itoa(i+1)))
		g.gain[i], g.env[i] = gain, env
		g.kind[i] = -1
		g.ensureNode(i, aud.fg().Wave(i))
	}
	g.running = true
	for i := range audiosrc.OscCount {
		g.audioUpdate(i)
	}
}

func (g *generator) audioStop() {
	if !g.running {
		return
	}
	for i := range audiosrc.OscCount {
		if g.osc[i].Truthy() {
			g.osc[i].Call("stop")
			g.osc[i].Call("disconnect")
		}
		for _, n := range []js.Value{g.gain[i], g.env[i]} {
			if n.Truthy() {
				n.Call("disconnect")
			}
		}
		g.osc[i], g.gain[i], g.env[i] = js.Undefined(), js.Undefined(), js.Undefined()
		g.kind[i] = -1
	}
	g.ctx = js.Undefined()
	g.running = false
	mixRelease()
}

// audioUpdate pushes oscillator i's waveform, frequency and level to its
// Web Audio nodes.
func (g *generator) audioUpdate(i int) {
	if !g.running || !g.osc[i].Truthy() {
		return
	}
	w := aud.fg().Wave(i)
	g.ensureNode(i, w)
	_, stim := audiosrc.WaveStim(w)
	switch {
	case w == audiosrc.WaveLFSR:
		// The LFSR loop's clock tracks the freq knob via playback rate, the
		// same 32× mapping the analysis path uses.
		rate := aud.fg().Freq(i) * 32 / g.ctx.Get("sampleRate").Float()
		g.osc[i].Get("playbackRate").Set("value", rate)
	case stim:
		// A stimulus plays at its own rate.
	default:
		g.osc[i].Set("type", waveTypeName(w))
		g.osc[i].Get("frequency").Set("value", aud.fg().Freq(i))
	}
	level := 0.0
	if genHeard(i) {
		level = aud.fg().Amp(i) * 0.3 // headroom
	}
	g.gain[i].Get("gain").Set("value", level)
}
